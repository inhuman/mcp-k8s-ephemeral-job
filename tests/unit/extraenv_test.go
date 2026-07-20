package unit

import (
	"testing"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/executor"
)

// Server-level extra env is merged UNDER the caller's env: cluster config
// (e.g. GEM_MIRROR) reaches every job, but an explicit Input.Env key wins.
func TestExtraEnvMergedUnderCallerEnv(t *testing.T) {
	fake := &executor.Fake{Result: executor.Result{Status: executor.StatusSucceeded}}
	_ = fake // Fake bypasses K8s; merge lives in K8s.Run — test via exported helper instead.
	merged := executor.MergeExtraEnv(
		map[string]string{"GEM_MIRROR": "https://nexus/gems", "SHARED": "server"},
		map[string]string{"DELTA": "a.go", "SHARED": "caller"},
	)
	if merged["GEM_MIRROR"] != "https://nexus/gems" || merged["DELTA"] != "a.go" {
		t.Fatalf("merge lost keys: %+v", merged)
	}
	if merged["SHARED"] != "caller" {
		t.Fatalf("a caller key must override the server one: %+v", merged)
	}
	if got := executor.MergeExtraEnv(nil, map[string]string{"A": "1"}); got["A"] != "1" {
		t.Fatalf("nil server env: %+v", got)
	}
}
