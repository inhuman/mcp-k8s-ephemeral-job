package unit

import (
	"strings"
	"testing"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/runjob"
)

func TestValidate(t *testing.T) {
	resolver := runjob.NewImageResolver([]string{"python:3.12-slim", "golang:1.26"})

	tests := []struct {
		name    string
		in      runjob.Input
		wantErr string
	}{
		{
			name: "valid",
			in:   runjob.Input{Image: "python:3.12-slim", Command: []string{"python", "x.py"}},
		},
		{
			name:    "empty image",
			in:      runjob.Input{Command: []string{"echo"}},
			wantErr: "image is required",
		},
		{
			name:    "image not allowed",
			in:      runjob.Input{Image: "evil:latest", Command: []string{"echo"}},
			wantErr: "is not available",
		},
		{
			name:    "empty command",
			in:      runjob.Input{Image: "golang:1.26"},
			wantErr: "command is required",
		},
		{
			name:    "path traversal",
			in:      runjob.Input{Image: "golang:1.26", Command: []string{"go"}, Files: []runjob.InputFile{{Path: "../etc/passwd", ContentB64: "eA=="}}},
			wantErr: "must not contain ..",
		},
		{
			name:    "absolute path",
			in:      runjob.Input{Image: "golang:1.26", Command: []string{"go"}, Files: []runjob.InputFile{{Path: "/abs", ContentB64: "eA=="}}},
			wantErr: "must be relative",
		},
		{
			name:    "bad env key",
			in:      runjob.Input{Image: "golang:1.26", Command: []string{"go"}, Env: map[string]string{"1BAD": "v"}},
			wantErr: "invalid env key",
		},
		{
			name:    "timeout over max",
			in:      runjob.Input{Image: "golang:1.26", Command: []string{"go"}, TimeoutS: 9999},
			wantErr: "exceeds max",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := runjob.Validate(tc.in, resolver, 600)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateEmptyAllowlistDeniesAll(t *testing.T) {
	err := runjob.Validate(
		runjob.Input{Image: "python:3.12-slim", Command: []string{"python"}},
		runjob.NewImageResolver(nil), 600,
	)
	if err == nil {
		t.Fatal("empty allowlist must deny any image")
	}
}

func TestImageResolver(t *testing.T) {
	r := runjob.NewImageResolver([]string{
		"docker-proxy.t1.cloud/library/busybox:1.36",
		"docker-proxy.t1.cloud/library/python:3.13-slim",
	})
	cases := map[string]string{
		"busybox":                          "docker-proxy.t1.cloud/library/busybox:1.36",
		"busybox:latest":                   "docker-proxy.t1.cloud/library/busybox:1.36",
		"busybox:1.35":                     "docker-proxy.t1.cloud/library/busybox:1.36", // tag ignored
		"docker.io/library/busybox":        "docker-proxy.t1.cloud/library/busybox:1.36",
		"python":                           "docker-proxy.t1.cloud/library/python:3.13-slim",
		"python:3.11-slim":                 "docker-proxy.t1.cloud/library/python:3.13-slim",
		"BusyBox":                          "docker-proxy.t1.cloud/library/busybox:1.36", // case-insensitive
	}
	for req, want := range cases {
		got, ok := r.Resolve(req)
		if !ok || got != want {
			t.Errorf("Resolve(%q) = (%q, %v), want (%q, true)", req, got, ok, want)
		}
	}
	if _, ok := r.Resolve("alpine"); ok {
		t.Error("alpine must not resolve (not in allowlist)")
	}
	if names := r.Names(); len(names) != 2 || names[0] != "busybox" || names[1] != "python" {
		t.Errorf("Names() = %v, want [busybox python]", names)
	}
}

func TestImageResolverMultipleVersions(t *testing.T) {
	r := runjob.NewImageResolver([]string{
		"docker-proxy.t1.cloud/library/busybox:1.36",
		"docker-proxy.t1.cloud/library/busybox:1.35",
	})
	// Bare base name → the FIRST listed (default), not silently shadowed.
	if got, _ := r.Resolve("busybox"); got != "docker-proxy.t1.cloud/library/busybox:1.36" {
		t.Errorf("Resolve(busybox) = %q, want :1.36 default", got)
	}
	// Exact version → that exact one.
	if got, _ := r.Resolve("busybox:1.35"); got != "docker-proxy.t1.cloud/library/busybox:1.35" {
		t.Errorf("Resolve(busybox:1.35) = %q, want :1.35", got)
	}
	if got, _ := r.Resolve("busybox:1.36"); got != "docker-proxy.t1.cloud/library/busybox:1.36" {
		t.Errorf("Resolve(busybox:1.36) = %q, want :1.36", got)
	}
	// Unknown version → fall back to the base default, still runs.
	if got, ok := r.Resolve("busybox:9.9"); !ok || got != "docker-proxy.t1.cloud/library/busybox:1.36" {
		t.Errorf("Resolve(busybox:9.9) = (%q,%v), want :1.36 default", got, ok)
	}
	// Both versions are visible in the description (no silent shadowing).
	if names := r.Names(); len(names) != 2 || names[0] != "busybox:1.35" || names[1] != "busybox:1.36" {
		t.Errorf("Names() = %v, want [busybox:1.35 busybox:1.36]", names)
	}
}
