package unit

import (
	"encoding/base64"
	"testing"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/executor"
	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/runjob"
	"go.uber.org/zap"
)

func newTool(fake *executor.Fake) *runjob.Tool {
	return runjob.NewTool(fake, runjob.Options{
		DefaultTimeoutS: 60,
		MaxTimeoutS:     600,
		DefaultCPU:      "1",
		DefaultMemory:   "512Mi",
		AllowedImages:   []string{"python:3.12-slim"},
	}, zap.NewNop())
}

func TestExecuteMapsResultToOutput(t *testing.T) {
	fake := &executor.Fake{Result: executor.Result{
		ExitCode:  0,
		Stdout:    []byte("hello"),
		Status:    executor.StatusSucceeded,
		Artifacts: []executor.Artifact{{Name: "out.png", Size: 3, Content: []byte{1, 2, 3}}},
	}}
	tool := newTool(fake)

	out, err := tool.Execute(t.Context(), runjob.Input{
		Image:   "python:3.12-slim",
		Command: []string{"python", "gen.py"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Status != executor.StatusSucceeded || out.ExitCode != 0 {
		t.Errorf("status/exit = %q/%d", out.Status, out.ExitCode)
	}
	if out.Stdout != "hello" {
		t.Errorf("stdout = %q", out.Stdout)
	}
	if len(out.Artifacts) != 1 || out.Artifacts[0].Name != "out.png" {
		t.Fatalf("artifacts = %+v", out.Artifacts)
	}
	if out.Artifacts[0].ContentB64 != base64.StdEncoding.EncodeToString([]byte{1, 2, 3}) {
		t.Errorf("artifact content not base64-encoded")
	}
}

func TestExecuteDecodesFilesIntoSpec(t *testing.T) {
	fake := &executor.Fake{Result: executor.Result{Status: executor.StatusSucceeded}}
	tool := newTool(fake)

	_, err := tool.Execute(t.Context(), runjob.Input{
		Image:   "python:3.12-slim",
		Command: []string{"python", "gen.py"},
		Files:   []runjob.InputFile{{Path: "gen.py", ContentB64: base64.StdEncoding.EncodeToString([]byte("print(1)"))}},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(fake.LastSpec.Files) != 1 || string(fake.LastSpec.Files[0].Content) != "print(1)" {
		t.Fatalf("files not decoded into spec: %+v", fake.LastSpec.Files)
	}
}

func TestExecuteRejectsBadImage(t *testing.T) {
	tool := newTool(&executor.Fake{})
	if _, err := tool.Execute(t.Context(), runjob.Input{Image: "evil", Command: []string{"sh"}}); err == nil {
		t.Fatal("expected validation error for disallowed image")
	}
}
