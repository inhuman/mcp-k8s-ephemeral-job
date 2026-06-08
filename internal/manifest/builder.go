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
	// TokenMask — чем заменяется реальный токен в origin URL после клона: видно, что
	// auth был, но сам секрет не раскрыт.
	TokenMask = "xxxxxxxxxxxx"
	// ReadySentinel — файл, по появлению которого init-контейнер завершается и стартует основной.
	ReadySentinel = ".runjob-ready"
)

// GitClone описывает клон репозитория init-контейнером. Креды видит ТОЛЬКО init
// (секрет смонтирован лишь в его volumeMounts), основной контейнер их не видит —
// разделение привилегий. После клона реальный токен в .git/config МАСКИРУЕТСЯ на
// TokenMask (не удаляется): origin остаётся видимым, секрет — нет (Принцип VII).
type GitClone struct {
	RepoURL    string // https://host/path(.git) — БЕЗ токена, токен подставит init из секрета
	Ref        string // ветка или sha
	Subdir     string // подпапка в workdir, куда клонировать (напр. "src")
	SecretName string // имя k8s-секрета с ключом "token"; монтируется ТОЛЬКО на init-клонер
	Image      string // образ клонера (с git внутри)
}

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
	Clone    *GitClone // если задан — init-контейнер клонирует репо до старта main
	Cache    *Cache    // если задан — PVC монтируется в main+reader на CacheMountPath
}

// Cache — постоянный том, который монтируется во все основные/sidecar контейнеры
// пода (но НЕ в init-clone — кешировать нечего, плюс минимизируем поверхность
// записи кредов в чужой volume). Типовое применение — Go module cache
// (/go/pkg/mod): первый прогон скачивает зависимости, последующие читают из PVC.
// PVC должен существовать в namespace до старта (создаётся через helm/manifest).
type Cache struct {
	PVCName   string
	MountPath string
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
	// mainMounts — то, что монтируется в main+reader. Cache (если задан) живёт
	// рядом с workdir, чтобы между прогонами выживал.
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

	// Клон репозитория: отдельный init-контейнер. Секрет с токеном смонтирован ТОЛЬКО на него —
	// основной контейнер кредов не видит (разделение привилегий, Принцип VII).
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

// cloneScript клонирует REPO_URL@REF в DEST, используя токен из смонтированного
// секрета, затем МАСКИРУЕТ токен в origin URL и убеждается, что реальный токен
// нигде в .git не остался. REPO_URL/REF/DEST приходят как env (через "$VAR" —
// без shell-инъекции). Токен читается из файла, в командную строку/историю не
// попадает (auth через credential helper на одну команду).
const cloneScript = `set -eu
# https://host/path -> host/path (для вставки basic-auth, надёжно для GitLab)
HP="$(printf '%s' "$REPO_URL" | sed -e 's#^https://##' -e 's#^http://##')"
HOST="$(printf '%s' "$HP" | cut -d/ -f1)"
# Токен выбирается по хосту: секрет содержит ключ на каждый GitLab-инстанс
# (ключ = хост, значение = токен). Так клонер ходит на разные гитлабы с разными кредами.
if [ ! -f "` + credsMount + `/$HOST" ]; then echo "FATAL: no credentials for host $HOST" >&2; exit 1; fi
TOKEN="$(cat "` + credsMount + `/$HOST")"
git clone --branch "$REF" "https://oauth2:${TOKEN}@${HP}" "$DEST"
# Маскируем токен в origin: видно, что auth был, но секрет скрыт. main стартует
# ТОЛЬКО после успешного init, т.е. всегда видит уже замаскированный config.
git -C "$DEST" remote set-url origin "https://oauth2:` + TokenMask + `@${HP}"
# Контроль: реального токена не должно остаться нигде в .git.
if grep -rqF "$TOKEN" "$DEST/.git" 2>/dev/null; then echo "FATAL: token leaked into .git" >&2; exit 1; fi
echo "cloned $REPO_URL@$REF -> $DEST"`

func hardenedSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: new(false),
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}
