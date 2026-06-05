//go:build kind

// Полный жизненный цикл US1 на РЕАЛЬНОМ кластере (kind/k3s): под стартует, исполняет,
// отдаёт stdout+артефакт, удаляется. Запуск:
//
//	go test -tags kind ./tests/integration -run TestRunJobE2E
package integration

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/executor"
	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/runjob"
	"go.uber.org/zap"
)

func TestRunJobE2E(t *testing.T) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("KUBECONFIG not set; kind/k3s cluster required")
	}
	ns := os.Getenv("MCP_K8S_NAMESPACE")
	if ns == "" {
		ns = "default"
	}

	exec, err := executor.NewK8s(executor.K8sOptions{
		Kubeconfig:   kubeconfig,
		Namespace:    ns,
		SidecarImage: "busybox:1.36",
		TTLSeconds:   60,
		MaxOutput:    1 << 20,
		MaxArtifact:  1 << 20,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}

	tool := runjob.NewTool(exec, runjob.Options{
		DefaultTimeoutS: 60, MaxTimeoutS: 600, DefaultCPU: "200m", DefaultMemory: "128Mi",
		AllowedImages: []string{"busybox:1.36"},
	}, zap.NewNop())

	out, err := tool.Execute(t.Context(), runjob.Input{
		Image:   "busybox:1.36",
		Command: []string{"sh", "-c", "echo hi; echo data > out.txt"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Status != executor.StatusSucceeded || out.ExitCode != 0 {
		t.Fatalf("status/exit = %q/%d, stdout=%q", out.Status, out.ExitCode, out.Stdout)
	}

	var found bool
	for _, a := range out.Artifacts {
		if a.Name == "out.txt" {
			content, _ := base64.StdEncoding.DecodeString(a.ContentB64)
			if string(content) != "data\n" {
				t.Errorf("artifact content = %q", content)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("artifact out.txt not returned; got %d artifacts", len(out.Artifacts))
	}
}
