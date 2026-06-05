package manifest

import (
	"fmt"
	"path"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	AppLabel       = "mcp-k8s-ephemeral-job"
	workVolumeName = "work"
	MainContainer  = "main"
	ReaderSidecar  = "reader"
	InjectInit     = "inject"
	// ReadySentinel — файл, по появлению которого init-контейнер завершается и стартует основной.
	ReadySentinel = ".runjob-ready"
)

// Params — примитивные параметры манифеста (без зависимости на другие internal-пакеты).
type Params struct {
	Namespace    string
	SidecarImage string
	RunID        string
	TTLSeconds   int32

	Image    string
	Command  []string
	Env      map[string]string
	CPU      string
	Memory   string
	Workdir  string
	Timeout  time.Duration
	HasFiles bool
}

// Build детерминированно собирает Job из параметров (Принцип III: манифест строит сервер).
func Build(p Params) (*batchv1.Job, error) {
	workdir := p.Workdir
	if workdir == "" {
		workdir = "/work"
	}

	cpu, err := resource.ParseQuantity(p.CPU)
	if err != nil {
		return nil, fmt.Errorf("cpu quantity: %w", err)
	}
	mem, err := resource.ParseQuantity(p.Memory)
	if err != nil {
		return nil, fmt.Errorf("memory quantity: %w", err)
	}

	labels := map[string]string{"app": AppLabel, "run-id": p.RunID}
	workMount := corev1.VolumeMount{Name: workVolumeName, MountPath: workdir}

	env := make([]corev1.EnvVar, 0, len(p.Env))
	for k, v := range p.Env {
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}

	main := corev1.Container{
		Name:            MainContainer,
		Image:           p.Image,
		Command:         p.Command,
		Env:             env,
		WorkingDir:      workdir,
		VolumeMounts:    []corev1.VolumeMount{workMount},
		SecurityContext: hardenedSecurityContext(),
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    cpu,
				corev1.ResourceMemory: mem,
			},
		},
	}

	deadline := int64(p.Timeout.Seconds())
	reader := corev1.Container{
		Name:            ReaderSidecar,
		Image:           p.SidecarImage,
		Command:         []string{"sleep", fmt.Sprintf("%d", deadline+60)},
		VolumeMounts:    []corev1.VolumeMount{workMount},
		SecurityContext: hardenedSecurityContext(),
	}

	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		Volumes: []corev1.Volume{{
			Name:         workVolumeName,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		}},
		Containers: []corev1.Container{main, reader},
	}

	// Инъекция входных файлов: init-контейнер держит /work, ждёт sentinel; сервер заливает файлы
	// через exec и ставит sentinel — гарантия «файлы до старта команды» (FR-002).
	if p.HasFiles {
		sentinel := path.Join(workdir, ReadySentinel)
		podSpec.InitContainers = []corev1.Container{{
			Name:            InjectInit,
			Image:           p.SidecarImage,
			Command:         []string{"sh", "-c", fmt.Sprintf("until [ -f %q ]; do sleep 0.1; done", sentinel)},
			VolumeMounts:    []corev1.VolumeMount{workMount},
			SecurityContext: hardenedSecurityContext(),
		}}
	}

	backoff := int32(0)
	completions := int32(1)
	parallelism := int32(1)
	ttl := p.TTLSeconds
	activeDeadline := deadline + 30

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "ephrun-",
			Namespace:    p.Namespace,
			Labels:       labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			Completions:             &completions,
			Parallelism:             &parallelism,
			TTLSecondsAfterFinished: &ttl,
			ActiveDeadlineSeconds:   &activeDeadline,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       podSpec,
			},
		},
	}
	return job, nil
}

func hardenedSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: new(false),
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}
