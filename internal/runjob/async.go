package runjob

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// Async wraps a Tool with a submit/fetch pair so a long job can run while the
// caller does other work: an LLM agent submits the static-analysis battery,
// keeps reading diffs, and collects the result when it actually needs it —
// instead of idling for the job's whole wall time inside one synchronous call.
// Jobs live in memory (the server is single-replica by design); a restart loses
// pending handles, which is acceptable for ephemeral runs — the caller simply
// resubmits.
type Async struct {
	tool *Tool
	log  *zap.Logger

	mu   sync.Mutex
	jobs map[string]*asyncJob
}

type asyncJob struct {
	done    chan struct{}
	out     Output
	err     error
	created time.Time
}

// jobTTL bounds how long a finished (or abandoned) job's result is retained.
const jobTTL = 60 * time.Minute

// fetchMaxWaitS caps the long-poll of fetch_job.
const fetchMaxWaitS = 120

// SubmitInput is the submit_job argument set — identical to run_job's Input.
type SubmitInput struct {
	Input
}

// SubmitOutput hands back the token the caller uses to fetch the result.
type SubmitOutput struct {
	JobToken string `json:"job_token"`
	Status   string `json:"status"` // always "submitted"
}

// FetchInput asks for a submitted job's result, optionally long-polling.
type FetchInput struct {
	JobToken string `json:"job_token" jsonschema:"token returned by submit_job"`
	WaitS    int    `json:"wait_s,omitempty" jsonschema:"long-poll: wait up to this many seconds for completion before answering (0 = answer immediately, max 120)"`
}

// FetchOutput is either the finished job's Output or a still-running marker.
type FetchOutput struct {
	Status string  `json:"status"` // running | done | error
	Error  string  `json:"error,omitempty"`
	Result *Output `json:"result,omitempty"`
}

// NewAsync builds the submit/fetch pair over an existing Tool.
func NewAsync(tool *Tool, log *zap.Logger) *Async {
	return &Async{tool: tool, log: log, jobs: make(map[string]*asyncJob)}
}

// Register adds submit_job and fetch_job to the server, alongside run_job.
func (a *Async) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "submit_job",
		Description: "Start a job like run_job but return IMMEDIATELY with a job_token instead of waiting. " +
			"Use when you have other work to do while the job runs; collect the result with fetch_job. " +
			"Same arguments as run_job.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SubmitInput) (*mcp.CallToolResult, SubmitOutput, error) {
		out, err := a.Submit(ctx, in)
		return nil, out, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name: "fetch_job",
		Description: "Fetch the result of a job started with submit_job. status=running means not finished yet — " +
			"pass wait_s to long-poll instead of hammering; status=done carries the same result shape as run_job.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in FetchInput) (*mcp.CallToolResult, FetchOutput, error) {
		out, err := a.Fetch(ctx, in)
		return nil, out, err
	})
}

// Submit validates and starts the job, returning its token immediately.
func (a *Async) Submit(_ context.Context, in SubmitInput) (SubmitOutput, error) {
	// Validate up front so an invalid call fails the SUBMIT, not the fetch —
	// the caller must learn about a bad argument while it can still fix it.
	if err := Validate(in.Input, a.tool.resolver, a.tool.opts.MaxTimeoutS); err != nil {
		return SubmitOutput{}, fmt.Errorf("invalid arguments: %w", err)
	}
	token, err := newJobToken()
	if err != nil {
		return SubmitOutput{}, err
	}
	j := &asyncJob{done: make(chan struct{}), created: time.Now()}
	a.mu.Lock()
	a.sweepLocked()
	a.jobs[token] = j
	a.mu.Unlock()

	// The job outlives the submit call by design → detached context with its
	// own ceiling: the job's own timeout (or the server max) plus scheduling
	// slack, so a stuck executor cannot leak the goroutine forever.
	timeout := in.TimeoutS
	if timeout <= 0 {
		timeout = a.tool.opts.DefaultTimeoutS
	}
	runCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second+2*time.Minute)
	go func() {
		defer cancel()
		out, err := a.tool.Execute(runCtx, in.Input)
		a.mu.Lock()
		j.out, j.err = out, err
		a.mu.Unlock()
		close(j.done)
		if err != nil {
			a.log.Warn("async job failed", zap.Error(err))
		}
	}()
	return SubmitOutput{JobToken: token, Status: "submitted"}, nil
}

// Fetch returns a submitted job's result, long-polling up to WaitS seconds.
func (a *Async) Fetch(ctx context.Context, in FetchInput) (FetchOutput, error) {
	a.mu.Lock()
	j, ok := a.jobs[in.JobToken]
	a.mu.Unlock()
	if !ok {
		return FetchOutput{}, fmt.Errorf("unknown job_token (expired, already collected after TTL, or the server restarted) — resubmit the job")
	}

	wait := in.WaitS
	if wait < 0 {
		wait = 0
	}
	if wait > fetchMaxWaitS {
		wait = fetchMaxWaitS
	}
	if wait > 0 {
		timer := time.NewTimer(time.Duration(wait) * time.Second)
		defer timer.Stop()
		select {
		case <-j.done:
		case <-timer.C:
		case <-ctx.Done():
		}
	}

	select {
	case <-j.done:
	default:
		return FetchOutput{Status: "running"}, nil
	}

	a.mu.Lock()
	out, err := j.out, j.err
	a.mu.Unlock()
	if err != nil {
		return FetchOutput{Status: "error", Error: err.Error()}, nil
	}
	return FetchOutput{Status: "done", Result: &out}, nil
}

// sweepLocked drops jobs past TTL. Called under mu on each submit — lazy
// housekeeping, no background goroutine to manage.
func (a *Async) sweepLocked() {
	cutoff := time.Now().Add(-jobTTL)
	for tok, j := range a.jobs {
		if j.created.Before(cutoff) {
			delete(a.jobs, tok)
		}
	}
}

func newJobToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "job-" + hex.EncodeToString(b), nil
}
