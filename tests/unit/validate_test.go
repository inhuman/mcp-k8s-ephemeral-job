package unit

import (
	"strings"
	"testing"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/runjob"
)

func TestValidate(t *testing.T) {
	allowed := []string{"python:3.12-slim", "golang:1.26"}

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
			wantErr: "is not allowed",
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
			err := runjob.Validate(tc.in, allowed, 600)
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
		nil, 600,
	)
	if err == nil {
		t.Fatal("empty allowlist must deny any image")
	}
}
