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
	credsVolume    = "git-creds"
	credsMount     = "/git-creds"
	cacheVolume    = "cache"
	MainContainer  = "main"
	ReaderSidecar  = "reader"
	InjectInit     = "inject"
	CloneInit      = "clone"
	// TokenMask replaces the real token in the origin URL after the clone: it stays
	// visible that auth happened, without disclosing the secret itself.
	TokenMask = "xxxxxxxxxxxx"
	// ReadySentinel is the file whose appearance ends the init container and starts the main one.
	ReadySentinel = ".runjob-ready"
)

// GitClone describes a repository clone performed by an init container. ONLY the
// init container sees the credentials (the secret is mounted in its volumeMounts
// alone) — privilege separation. After the clone the real token in .git/config is
// MASKED with TokenMask rather than removed: origin stays visible, the secret does not.
type GitClone struct {
	RepoURL    string // https://host/path(.git) — WITHOUT a token; init injects it from the secret
	Ref        string // branch or sha
	Subdir     string // subdirectory of workdir to clone into (e.g. "src")
	SecretName string // name of the k8s secret holding the token; mounted ONLY on the init cloner
	Image      string // cloner image (one that ships git)
}

// Params holds the primitive manifest parameters (no dependency on other internal packages).
type Params struct {
	Namespace    string
	SidecarImage string
	RunID        string
	TTLSeconds   int32

	Image         string
	Command       []string
	Env           map[string]string
	CPURequest    string
	MemoryRequest string
	CPULimit      string // "" = no limit set; the ceiling comes from the LimitRange default
	MemoryLimit   string // "" = no limit set
	Workdir       string
	Timeout       time.Duration
	HasFiles      bool
	Clone         *GitClone // when set, an init container clones the repo before main starts
	Cache         *Cache    // when set, the PVC is mounted into main+reader at CacheMountPath
}

// Cache is a persistent volume mounted into the pod's main/sidecar containers (but
// NOT into the clone init container — there is nothing to cache there, and it keeps
// the surface for writing credentials into a shared volume minimal). Typical use is
// a Go module cache (/go/pkg/mod): the first run downloads the dependencies, later
// runs read them from the PVC. The PVC must already exist in the namespace
// (provisioned via helm/manifest).
type Cache struct {
	PVCName   string
	MountPath string
}

// buildResources assembles ResourceRequirements: requests always, limits only when
// non-empty (an empty limit means the ceiling comes from the namespace LimitRange).
func buildResources(cpuReq, memReq, cpuLim, memLim string) (corev1.ResourceRequirements, error) {
	out := corev1.ResourceRequirements{Requests: corev1.ResourceList{}}
	parse := func(name, q string) (resource.Quantity, error) {
		v, err := resource.ParseQuantity(q)
		if err != nil {
			return v, fmt.Errorf("%s quantity: %w", name, err)
		}
		return v, nil
	}
	cq, err := parse("cpu request", cpuReq)
	if err != nil {
		return out, err
	}
	mq, err := parse("memory request", memReq)
	if err != nil {
		return out, err
	}
	out.Requests[corev1.ResourceCPU] = cq
	out.Requests[corev1.ResourceMemory] = mq
	if cpuLim != "" || memLim != "" {
		out.Limits = corev1.ResourceList{}
		if cpuLim != "" {
			v, err := parse("cpu limit", cpuLim)
			if err != nil {
				return out, err
			}
			out.Limits[corev1.ResourceCPU] = v
		}
		if memLim != "" {
			v, err := parse("memory limit", memLim)
			if err != nil {
				return out, err
			}
			out.Limits[corev1.ResourceMemory] = v
		}
	}
	return out, nil
}

// Build deterministically assembles the Job from the parameters — the server builds
// the manifest, the caller never supplies raw YAML.
func Build(p Params) (*batchv1.Job, error) {
	workdir := p.Workdir
	if workdir == "" {
		workdir = "/work"
	}

	// Requests must always be set (the scheduler's reservation); limits are optional —
	// an empty value does NOT reach the manifest, and the namespace LimitRange supplies
	// the container's ceiling. Putting the request values into limits is wrong: k8s then
	// copies limits into requests and the LimitRange default stops applying.
	resources, err := buildResources(p.CPURequest, p.MemoryRequest, p.CPULimit, p.MemoryLimit)
	if err != nil {
		return nil, err
	}

	labels := map[string]string{"app": AppLabel, "run-id": p.RunID}
	workMount := corev1.VolumeMount{Name: workVolumeName, MountPath: workdir}
	// mainMounts is what gets mounted into main+reader. The cache (when set) lives
	// alongside workdir so that it survives between runs.
	mainMounts := []corev1.VolumeMount{workMount}
	if p.Cache != nil && p.Cache.PVCName != "" && p.Cache.MountPath != "" {
		mainMounts = append(mainMounts, corev1.VolumeMount{
			Name:      cacheVolume,
			MountPath: p.Cache.MountPath,
		})
	}

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
		VolumeMounts:    mainMounts,
		SecurityContext: hardenedSecurityContext(),
		Resources:       resources,
	}

	deadline := int64(p.Timeout.Seconds())
	reader := corev1.Container{
		Name:            ReaderSidecar,
		Image:           p.SidecarImage,
		Command:         []string{"sleep", fmt.Sprintf("%d", deadline+60)},
		VolumeMounts:    mainMounts,
		SecurityContext: hardenedSecurityContext(),
	}

	podVolumes := []corev1.Volume{{
		Name:         workVolumeName,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}}
	if p.Cache != nil && p.Cache.PVCName != "" && p.Cache.MountPath != "" {
		podVolumes = append(podVolumes, corev1.Volume{
			Name: cacheVolume,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: p.Cache.PVCName,
				},
			},
		})
	}

	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		Volumes:       podVolumes,
		Containers:    []corev1.Container{main, reader},
	}

	// Input file injection: the init container holds /work and waits for the sentinel;
	// the server uploads the files via exec and then writes the sentinel — this is what
	// guarantees "files land before the command starts".
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

	// Repository clone: a separate init container. The token secret is mounted ONLY on it —
	// the main container never sees the credentials (privilege separation).
	if p.Clone != nil {
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: credsVolume,
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
				SecretName: p.Clone.SecretName,
			}},
		})
		podSpec.InitContainers = append(podSpec.InitContainers, corev1.Container{
			Name:  CloneInit,
			Image: p.Clone.Image,
			Env: []corev1.EnvVar{
				{Name: "REPO_URL", Value: p.Clone.RepoURL},
				{Name: "REF", Value: p.Clone.Ref},
				{Name: "DEST", Value: path.Join(workdir, p.Clone.Subdir)},
			},
			Command: []string{"sh", "-c", cloneScript},
			VolumeMounts: []corev1.VolumeMount{
				workMount,
				{Name: credsVolume, MountPath: credsMount, ReadOnly: true},
			},
			SecurityContext: hardenedSecurityContext(),
		})
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

// cloneScript clones REPO_URL@REF into DEST using the token from the mounted secret,
// then MASKS the token in the origin URL and verifies the real token was left nowhere
// under .git. REPO_URL/REF/DEST arrive as env vars (referenced as "$VAR" — no shell
// injection). The token is read from a file and never reaches the command line or
// shell history.
const cloneScript = `set -eu
# https://host/path -> host/path (so basic-auth can be spliced in)
HP="$(printf '%s' "$REPO_URL" | sed -e 's#^https://##' -e 's#^http://##')"
HOST="$(printf '%s' "$HP" | cut -d/ -f1)"
# The token is selected by host: the secret holds one key per git host
# (key = host, value = token), so the cloner can reach different hosts with different creds.
if [ ! -f "` + credsMount + `/$HOST" ]; then echo "FATAL: no credentials for host $HOST" >&2; exit 1; fi
TOKEN="$(cat "` + credsMount + `/$HOST")"
git clone --branch "$REF" "https://oauth2:${TOKEN}@${HP}" "$DEST"
# Mask the token in origin: auth stays visible, the secret does not. main starts
# ONLY after a successful init, so it always sees the already-masked config.
git -C "$DEST" remote set-url origin "https://oauth2:` + TokenMask + `@${HP}"
# Guard: the real token must not survive anywhere under .git.
if grep -rqF "$TOKEN" "$DEST/.git" 2>/dev/null; then echo "FATAL: token leaked into .git" >&2; exit 1; fi
echo "cloned $REPO_URL@$REF -> $DEST"`

func hardenedSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: new(false),
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}
