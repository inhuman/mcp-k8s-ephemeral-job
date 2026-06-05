package unit

import (
	"testing"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/runjob"
)

func TestResolveLimitsDefaults(t *testing.T) {
	got, err := runjob.ResolveLimits(nil, "1", "512Mi")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.CPU != "1" || got.Memory != "512Mi" {
		t.Fatalf("defaults not applied: %+v", got)
	}
}

func TestResolveLimitsOverride(t *testing.T) {
	got, err := runjob.ResolveLimits(&runjob.ResourceLimits{CPU: "500m", Memory: "256Mi"}, "1", "512Mi")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.CPU != "500m" || got.Memory != "256Mi" {
		t.Fatalf("override not applied: %+v", got)
	}
}

func TestResolveLimitsPartialOverride(t *testing.T) {
	got, err := runjob.ResolveLimits(&runjob.ResourceLimits{Memory: "1Gi"}, "1", "512Mi")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.CPU != "1" || got.Memory != "1Gi" {
		t.Fatalf("partial override wrong: %+v", got)
	}
}

func TestResolveLimitsInvalid(t *testing.T) {
	if _, err := runjob.ResolveLimits(&runjob.ResourceLimits{CPU: "abc"}, "1", "512Mi"); err == nil {
		t.Fatal("invalid cpu must error")
	}
	if _, err := runjob.ResolveLimits(&runjob.ResourceLimits{Memory: "12quux"}, "1", "512Mi"); err == nil {
		t.Fatal("invalid memory must error")
	}
}
