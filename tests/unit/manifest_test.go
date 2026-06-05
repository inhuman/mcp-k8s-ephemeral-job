package unit

import (
	"slices"
	"testing"
	"time"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/manifest"
	corev1 "k8s.io/api/core/v1"
)

func baseParams() manifest.Params {
	return manifest.Params{
		Namespace:    "ephemeral",
		SidecarImage: "busybox:1.36",
		RunID:        "abc123",
		TTLSeconds:   120,
		Image:        "python:3.12-slim",
		Command:      []string{"python", "gen.py"},
		CPU:          "500m",
		Memory:       "256Mi",
		Workdir:      "/work",
		Timeout:      30 * time.Second,
	}
}

func findContainer(cs []corev1.Container, name string) *corev1.Container {
	for i := range cs {
		if cs[i].Name == name {
			return &cs[i]
		}
	}
	return nil
}

func TestBuildBasics(t *testing.T) {
	job, err := manifest.Build(baseParams())
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if job.Namespace != "ephemeral" {
		t.Errorf("namespace = %q", job.Namespace)
	}
	if job.GenerateName != "ephrun-" {
		t.Errorf("generateName = %q", job.GenerateName)
	}
	if job.Labels["run-id"] != "abc123" || job.Labels["app"] != manifest.AppLabel {
		t.Errorf("labels = %v", job.Labels)
	}
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 0 {
		t.Errorf("backoffLimit must be 0")
	}
	if job.Spec.TTLSecondsAfterFinished == nil || *job.Spec.TTLSecondsAfterFinished != 120 {
		t.Errorf("ttl not set")
	}
	if job.Spec.ActiveDeadlineSeconds == nil || *job.Spec.ActiveDeadlineSeconds != 60 {
		t.Errorf("activeDeadlineSeconds = %v, want 60 (timeout 30 + grace 30)", job.Spec.ActiveDeadlineSeconds)
	}
	if job.Spec.Template.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("restartPolicy = %q", job.Spec.Template.Spec.RestartPolicy)
	}
}

func TestBuildSidecarAndMain(t *testing.T) {
	job, err := manifest.Build(baseParams())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	containers := job.Spec.Template.Spec.Containers

	main := findContainer(containers, manifest.MainContainer)
	if main == nil {
		t.Fatal("main container missing")
	}
	if main.Image != "python:3.12-slim" {
		t.Errorf("main image = %q", main.Image)
	}
	sc := main.SecurityContext
	if sc == nil || sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Error("main must set allowPrivilegeEscalation=false")
	}
	if sc == nil || !slices.Contains(sc.Capabilities.Drop, corev1.Capability("ALL")) {
		t.Error("main must drop ALL capabilities")
	}

	reader := findContainer(containers, manifest.ReaderSidecar)
	if reader == nil {
		t.Fatal("reader sidecar missing")
	}
	if reader.Image != "busybox:1.36" || reader.Command[0] != "sleep" {
		t.Errorf("reader = %q %v", reader.Image, reader.Command)
	}
}

func TestBuildInitContainerOnlyWithFiles(t *testing.T) {
	noFiles, _ := manifest.Build(baseParams())
	if len(noFiles.Spec.Template.Spec.InitContainers) != 0 {
		t.Error("no init container expected without files")
	}

	p := baseParams()
	p.HasFiles = true
	withFiles, _ := manifest.Build(p)
	inj := findContainer(withFiles.Spec.Template.Spec.InitContainers, manifest.InjectInit)
	if inj == nil {
		t.Fatal("inject init container expected with files")
	}
}

func TestBuildInvalidQuantity(t *testing.T) {
	p := baseParams()
	p.CPU = "not-a-quantity"
	if _, err := manifest.Build(p); err == nil {
		t.Fatal("invalid cpu must error")
	}
}
