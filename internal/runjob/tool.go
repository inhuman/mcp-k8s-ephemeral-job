package runjob

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/executor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

type Options struct {
	DefaultTimeoutS int
	MaxTimeoutS     int
	DefaultCPU      string
	DefaultMemory   string
	AllowedImages   []string
}

type Tool struct {
	exec executor.Executor
	opts Options
	log  *zap.Logger
}

func NewTool(exec executor.Executor, opts Options, log *zap.Logger) *Tool {
	return &Tool{exec: exec, opts: opts, log: log}
}

// Register добавляет тул run_job на сервер. Схема in/out выводится из типов Input/Output (Принцип III).
func (t *Tool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "run_job",
		Description: t.description(),
	}, t.handle)
}

// description builds the run_job tool description, enumerating the exact allowed
// images so the model copies one verbatim (the allowlist is an exact match, and
// the proxy-prefixed strings are not guessable).
func (t *Tool) description() string {
	d := "Synchronously run a command in an ephemeral Kubernetes pod and return exit code, output and artifacts. " +
		"The `image` MUST be one of the allowed images below, copied EXACTLY."
	if len(t.opts.AllowedImages) > 0 {
		d += " Allowed images: " + strings.Join(t.opts.AllowedImages, ", ") + "."
	} else {
		d += " No images are currently allowed — the tool will reject every call until the operator configures an allowlist."
	}
	return d
}

func (t *Tool) handle(ctx context.Context, _ *mcp.CallToolRequest, in Input) (*mcp.CallToolResult, Output, error) {
	out, err := t.Execute(ctx, in)
	if err != nil {
		return nil, Output{}, err
	}
	return nil, out, nil
}

// Execute прогоняет run_job: валидация → лимиты → спавн → сбор → Output.
// Ошибка возвращается только для невалидного входа/сбоя исполнителя; неуспешный прогон
// (status=failed/timeout/error) — это валидный Output с соответствующим статусом.
func (t *Tool) Execute(ctx context.Context, in Input) (Output, error) {
	if err := Validate(in, t.opts.AllowedImages, t.opts.MaxTimeoutS); err != nil {
		return Output{}, fmt.Errorf("invalid arguments: %w", err)
	}

	limits, err := ResolveLimits(in.Limits, t.opts.DefaultCPU, t.opts.DefaultMemory)
	if err != nil {
		return Output{}, fmt.Errorf("invalid arguments: %w", err)
	}

	files, err := decodeFiles(in.Files)
	if err != nil {
		return Output{}, fmt.Errorf("invalid arguments: %w", err)
	}

	timeoutS := in.TimeoutS
	if timeoutS == 0 {
		timeoutS = t.opts.DefaultTimeoutS
	}

	spec := executor.Spec{
		Image:   in.Image,
		Command: in.Command,
		Env:     in.Env,
		Files:   files,
		CPU:     limits.CPU,
		Memory:  limits.Memory,
		Workdir: in.Workdir,
		Timeout: time.Duration(timeoutS) * time.Second,
	}

	res, err := t.exec.Run(ctx, spec)
	if err != nil {
		t.log.Warn("run_job execution error", zap.String("status", res.Status), zap.Error(err))
		return Output{}, fmt.Errorf("run failed: %w", err)
	}

	out := toOutput(res)
	// Данные вызывающего не логируем (Принцип IX) — только метаданные.
	t.log.Info("run_job completed",
		zap.String("status", out.Status),
		zap.Int("exit_code", out.ExitCode),
		zap.Int64("duration_ms", out.DurationMS),
		zap.Int("stdout_bytes", len(out.Stdout)),
		zap.Int("artifacts", len(out.Artifacts)),
		zap.Bool("truncated_stdout", out.Truncated.Stdout),
		zap.Bool("truncated_artifacts", out.Truncated.Artifacts),
	)
	return out, nil
}

func decodeFiles(in []InputFile) ([]executor.File, error) {
	out := make([]executor.File, 0, len(in))
	for _, f := range in {
		content, err := base64.StdEncoding.DecodeString(f.ContentB64)
		if err != nil {
			return nil, fmt.Errorf("file %q: invalid base64: %w", f.Path, err)
		}
		mode := int64(0o644)
		if f.Mode != "" {
			if _, err := fmt.Sscanf(f.Mode, "%o", &mode); err != nil {
				return nil, fmt.Errorf("file %q: invalid mode %q: %w", f.Path, f.Mode, err)
			}
		}
		out = append(out, executor.File{Path: f.Path, Content: content, Mode: mode})
	}
	return out, nil
}

func toOutput(res executor.Result) Output {
	arts := make([]Artifact, len(res.Artifacts))
	for i, a := range res.Artifacts {
		arts[i] = Artifact{
			Name:       a.Name,
			Size:       a.Size,
			ContentB64: base64.StdEncoding.EncodeToString(a.Content),
		}
	}
	return Output{
		ExitCode:   res.ExitCode,
		Stdout:     string(res.Stdout),
		Stderr:     string(res.Stderr),
		DurationMS: res.Duration.Milliseconds(),
		Status:     res.Status,
		Artifacts:  arts,
		Truncated: Truncated{
			Stdout:    res.TruncStdout,
			Stderr:    res.TruncStderr,
			Artifacts: res.TruncArtifacts,
		},
	}
}
