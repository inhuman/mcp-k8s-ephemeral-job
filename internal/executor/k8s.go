package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/artifacts"
	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/manifest"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

type K8s struct {
	cs           kubernetes.Interface
	rc           *rest.Config
	ns           string
	sidecarImage string
	ttl          int32
	maxOutput    int64
	maxArtifact  int64
	log          *zap.Logger
}

type K8sOptions struct {
	Kubeconfig   string
	Namespace    string
	SidecarImage string
	TTLSeconds   int32
	MaxOutput    int64
	MaxArtifact  int64
}

func NewK8s(opts K8sOptions, log *zap.Logger) (*K8s, error) {
	rc, err := restConfig(opts.Kubeconfig)
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(rc)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}
	return &K8s{
		cs: cs, rc: rc, ns: opts.Namespace, sidecarImage: opts.SidecarImage,
		ttl: opts.TTLSeconds, maxOutput: opts.MaxOutput, maxArtifact: opts.MaxArtifact, log: log,
	}, nil
}

func restConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		rc, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("build kubeconfig: %w", err)
		}
		return rc, nil
	}
	rc, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	return rc, nil
}

func (k *K8s) Run(ctx context.Context, spec Spec) (Result, error) {
	runID := newRunID()
	job, err := manifest.Build(manifest.Params{
		Namespace:    k.ns,
		SidecarImage: k.sidecarImage,
		RunID:        runID,
		TTLSeconds:   k.ttl,
		Image:        spec.Image,
		Command:      spec.Command,
		Env:          spec.Env,
		CPU:          spec.CPU,
		Memory:       spec.Memory,
		Workdir:      spec.Workdir,
		Timeout:      spec.Timeout,
		HasFiles:     len(spec.Files) > 0,
	})
	if err != nil {
		return Result{}, err
	}

	created, err := k.cs.BatchV1().Jobs(k.ns).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return Result{}, fmt.Errorf("create job: %w", err)
	}
	defer k.deleteJob(created.Name)

	start := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, spec.Timeout+30*time.Second)
	defer cancel()

	res, err := k.waitAndCollect(runCtx, runID, spec)
	res.Duration = time.Since(start)
	return res, err
}

func (k *K8s) waitAndCollect(ctx context.Context, runID string, spec Spec) (Result, error) {
	w, err := k.cs.CoreV1().Pods(k.ns).Watch(ctx, metav1.ListOptions{LabelSelector: "run-id=" + runID})
	if err != nil {
		return Result{Status: StatusError}, fmt.Errorf("watch pods: %w", err)
	}
	defer w.Stop()

	injected := false
	for {
		select {
		case <-ctx.Done():
			return Result{Status: StatusTimeout, ExitCode: -1}, nil
		case ev, ok := <-w.ResultChan():
			if !ok {
				return Result{Status: StatusError, ExitCode: -1}, fmt.Errorf("pod watch closed")
			}
			pod, ok := ev.Object.(*corev1.Pod)
			if !ok {
				continue
			}

			if !injected && len(spec.Files) > 0 && initRunning(pod) {
				if err := k.injectFiles(ctx, pod.Name, spec); err != nil {
					return Result{Status: StatusError, ExitCode: -1}, fmt.Errorf("inject files: %w", err)
				}
				injected = true
			}

			if term := mainTerminated(pod); term != nil {
				return k.collect(ctx, pod.Name, workdirOf(spec), int(term.ExitCode))
			}
			if pod.Status.Phase == corev1.PodFailed {
				return Result{Status: StatusError, ExitCode: -1}, nil
			}
		}
	}
}

func (k *K8s) collect(ctx context.Context, podName, workdir string, exitCode int) (Result, error) {
	res := Result{ExitCode: exitCode, Status: StatusFailed}
	if exitCode == 0 {
		res.Status = StatusSucceeded
	}

	stdout, truncOut, err := k.podLogs(ctx, podName)
	if err != nil {
		k.log.Warn("collect logs failed", zap.Error(err))
	}
	res.Stdout = stdout
	res.TruncStdout = truncOut

	var tarBuf bytes.Buffer
	if err := k.exec(ctx, podName, manifest.ReaderSidecar,
		[]string{"tar", "cf", "-", "-C", workdir, "."}, nil, &tarBuf, io.Discard); err != nil {
		k.log.Warn("collect artifacts failed", zap.Error(err))
		return res, nil
	}
	files, truncArt, err := artifacts.CollectFromTar(&tarBuf, k.maxArtifact, manifest.ReadySentinel)
	if err != nil {
		k.log.Warn("parse artifacts failed", zap.Error(err))
		return res, nil
	}
	res.Artifacts = toArtifacts(files)
	res.TruncArtifacts = truncArt
	return res, nil
}

func (k *K8s) injectFiles(ctx context.Context, podName string, spec Spec) error {
	tarBytes, err := artifacts.BuildInputTar(toInputFiles(spec.Files))
	if err != nil {
		return err
	}
	workdir := workdirOf(spec)
	if err := k.exec(ctx, podName, manifest.InjectInit,
		[]string{"tar", "xf", "-", "-C", workdir}, bytes.NewReader(tarBytes), io.Discard, io.Discard); err != nil {
		return fmt.Errorf("stream tar: %w", err)
	}
	sentinel := path.Join(workdir, manifest.ReadySentinel)
	if err := k.exec(ctx, podName, manifest.InjectInit,
		[]string{"touch", sentinel}, nil, io.Discard, io.Discard); err != nil {
		return fmt.Errorf("touch sentinel: %w", err)
	}
	return nil
}

func (k *K8s) podLogs(ctx context.Context, podName string) ([]byte, bool, error) {
	stream, err := k.cs.CoreV1().Pods(k.ns).
		GetLogs(podName, &corev1.PodLogOptions{Container: manifest.MainContainer}).Stream(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("get logs: %w", err)
	}
	defer stream.Close()
	return readCapped(stream, k.maxOutput)
}

func (k *K8s) exec(ctx context.Context, podName, container string, command []string, stdin io.Reader, stdout, stderr io.Writer) error {
	req := k.cs.CoreV1().RESTClient().Post().
		Resource("pods").Name(podName).Namespace(k.ns).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdin:     stdin != nil,
			Stdout:    stdout != nil,
			Stderr:    stderr != nil,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(k.rc, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("spdy executor: %w", err)
	}
	if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{Stdin: stdin, Stdout: stdout, Stderr: stderr}); err != nil {
		return fmt.Errorf("exec stream: %w", err)
	}
	return nil
}

func (k *K8s) deleteJob(name string) {
	fg := metav1.DeletePropagationForeground
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := k.cs.BatchV1().Jobs(k.ns).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &fg}); err != nil {
		k.log.Warn("delete job failed", zap.String("job", name), zap.Error(err))
	}
}

func workdirOf(spec Spec) string {
	if spec.Workdir == "" {
		return DefaultWorkdir
	}
	return spec.Workdir
}

func toInputFiles(files []File) []artifacts.File {
	out := make([]artifacts.File, len(files))
	for i, f := range files {
		out[i] = artifacts.File{Name: f.Path, Mode: f.Mode, Content: f.Content}
	}
	return out
}

func toArtifacts(files []artifacts.File) []Artifact {
	out := make([]Artifact, len(files))
	for i, f := range files {
		out[i] = Artifact{Name: f.Name, Size: f.Size, Content: f.Content}
	}
	return out
}

func initRunning(pod *corev1.Pod) bool {
	for _, cs := range pod.Status.InitContainerStatuses {
		if cs.Name == manifest.InjectInit && cs.State.Running != nil {
			return true
		}
	}
	return false
}

func mainTerminated(pod *corev1.Pod) *corev1.ContainerStateTerminated {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == manifest.MainContainer && cs.State.Terminated != nil {
			return cs.State.Terminated
		}
	}
	return nil
}

func readCapped(r io.Reader, max int64) ([]byte, bool, error) {
	buf, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, false, fmt.Errorf("read: %w", err)
	}
	if int64(len(buf)) > max {
		return buf[:max], true, nil
	}
	return buf, false, nil
}
