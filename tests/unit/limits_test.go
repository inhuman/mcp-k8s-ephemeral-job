package unit

import (
	"testing"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/runjob"
)

// Server defaults are requests; limits are NOT set unless the caller asks for them
// (the namespace LimitRange supplies the ceiling).
func TestResolveResourcesDefaults(t *testing.T) {
	got, err := runjob.ResolveResources(nil, "1", "512Mi")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.CPURequest != "1" || got.MemoryRequest != "512Mi" {
		t.Fatalf("default requests not applied: %+v", got)
	}
	if got.CPULimit != "" || got.MemoryLimit != "" {
		t.Fatalf("limits must stay empty without caller limits: %+v", got)
	}
}

// An explicit limits.memory also raises the memory request (incompressible resource —
// the pod must be scheduled where the memory actually exists); the cpu request stays
// at the default (compressible — throttling, not OOM).
func TestResolveResourcesExplicit(t *testing.T) {
	got, err := runjob.ResolveResources(&runjob.ResourceLimits{CPU: "4", Memory: "10Gi"}, "200m", "512Mi")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.CPULimit != "4" || got.MemoryLimit != "10Gi" {
		t.Fatalf("limits not applied: %+v", got)
	}
	if got.MemoryRequest != "10Gi" {
		t.Fatalf("memory request must follow memory limit: %+v", got)
	}
	if got.CPURequest != "200m" {
		t.Fatalf("cpu request must stay default: %+v", got)
	}
}

func TestResolveResourcesPartial(t *testing.T) {
	got, err := runjob.ResolveResources(&runjob.ResourceLimits{Memory: "1Gi"}, "1", "512Mi")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.CPULimit != "" || got.MemoryLimit != "1Gi" || got.MemoryRequest != "1Gi" {
		t.Fatalf("partial override wrong: %+v", got)
	}
}

func TestResolveResourcesInvalid(t *testing.T) {
	if _, err := runjob.ResolveResources(&runjob.ResourceLimits{CPU: "abc"}, "1", "512Mi"); err == nil {
		t.Fatal("invalid cpu must error")
	}
	if _, err := runjob.ResolveResources(&runjob.ResourceLimits{Memory: "12quux"}, "1", "512Mi"); err == nil {
		t.Fatal("invalid memory must error")
	}
}
