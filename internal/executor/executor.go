package executor

import (
	"context"
	"time"
)

const (
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusTimeout   = "timeout"
	StatusError     = "error"

	DefaultWorkdir = "/work"
)

// Executor исполняет один эфемерный прогон. Реализации: k8s (прод) и fake (только unit-тесты).
type Executor interface {
	Run(ctx context.Context, spec Spec) (Result, error)
}

// Spec — нормализованные параметры прогона (лимиты уже разрешены в k8s quantity).
type Spec struct {
	Image   string
	Command []string
	Env     map[string]string
	Files   []File
	CPU     string
	Memory  string
	Workdir string
	Timeout time.Duration
}

type File struct {
	Path    string
	Content []byte
	Mode    int64
}

// Result — итог прогона. Status — один из runjob.Status* значений.
type Result struct {
	ExitCode  int
	Stdout    []byte
	Stderr    []byte
	Status    string
	Artifacts []Artifact
	Duration  time.Duration

	TruncStdout    bool
	TruncStderr    bool
	TruncArtifacts bool
}

type Artifact struct {
	Name    string
	Size    int64
	Content []byte
}
