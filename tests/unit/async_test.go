package unit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/executor"
	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/runjob"
	"go.uber.org/zap"
)

// blockingExecutor holds Run until released — models a long-running pod so the
// submit/fetch lifecycle (running → done) is observable.
type blockingExecutor struct {
	release chan struct{}
	result  executor.Result
}

func (b *blockingExecutor) Run(ctx context.Context, _ executor.Spec) (executor.Result, error) {
	select {
	case <-b.release:
		return b.result, nil
	case <-ctx.Done():
		return executor.Result{}, ctx.Err()
	}
}

func newAsync(exec executor.Executor) *runjob.Async {
	tool := runjob.NewTool(exec, runjob.Options{
		DefaultTimeoutS: 60,
		MaxTimeoutS:     600,
		DefaultCPU:      "1",
		DefaultMemory:   "512Mi",
		AllowedImages:   []string{"python:3.12-slim"},
	}, zap.NewNop())
	return runjob.NewAsync(tool, zap.NewNop())
}

func TestAsync_SubmitRunningThenDone(t *testing.T) {
	be := &blockingExecutor{
		release: make(chan struct{}),
		result:  executor.Result{Status: executor.StatusSucceeded, Stdout: []byte("battery done")},
	}
	a := newAsync(be)

	sub, err := a.Submit(t.Context(), runjob.SubmitInput{Input: runjob.Input{
		Image: "python:3.12-slim", Command: []string{"python", "x.py"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if sub.Status != "submitted" || !strings.HasPrefix(sub.JobToken, "job-") {
		t.Fatalf("submit: %+v", sub)
	}

	// While the executor is blocked, the job is running.
	f, err := a.Fetch(t.Context(), runjob.FetchInput{JobToken: sub.JobToken})
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != "running" || f.Result != nil {
		t.Fatalf("before completion: %+v", f)
	}

	// Release it; the long poll must wait for done without an external sleep.
	close(be.release)
	f, err = a.Fetch(t.Context(), runjob.FetchInput{JobToken: sub.JobToken, WaitS: 5})
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != "done" || f.Result == nil || f.Result.Stdout != "battery done" {
		t.Fatalf("after completion: %+v", f)
	}

	// Fetching again: the result is not single-use (within its TTL).
	f, err = a.Fetch(t.Context(), runjob.FetchInput{JobToken: sub.JobToken})
	if err != nil || f.Status != "done" {
		t.Fatalf("repeat fetch: %+v err=%v", f, err)
	}
}

func TestAsync_SubmitValidatesUpfront(t *testing.T) {
	a := newAsync(&executor.Fake{})
	_, err := a.Submit(t.Context(), runjob.SubmitInput{Input: runjob.Input{
		Image: "not-allowed:latest", Command: []string{"x"},
	}})
	if err == nil {
		t.Fatal("an invalid image must fail the submit, not the fetch")
	}
}

func TestAsync_UnknownToken(t *testing.T) {
	a := newAsync(&executor.Fake{})
	if _, err := a.Fetch(t.Context(), runjob.FetchInput{JobToken: "job-nope"}); err == nil {
		t.Fatal("an unknown token must be an error hinting at resubmit")
	}
}

func TestAsync_LongPollReturnsEarlyOnCompletion(t *testing.T) {
	be := &blockingExecutor{release: make(chan struct{}), result: executor.Result{Status: executor.StatusSucceeded}}
	a := newAsync(be)
	sub, err := a.Submit(t.Context(), runjob.SubmitInput{Input: runjob.Input{
		Image: "python:3.12-slim", Command: []string{"x"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	go func() { time.Sleep(100 * time.Millisecond); close(be.release) }()
	start := time.Now()
	f, err := a.Fetch(t.Context(), runjob.FetchInput{JobToken: sub.JobToken, WaitS: 30})
	if err != nil || f.Status != "done" {
		t.Fatalf("%+v err=%v", f, err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("the long poll must return as soon as it is done, not sit out wait_s")
	}
}
