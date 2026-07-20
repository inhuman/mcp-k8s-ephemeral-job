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

// Executor runs a single ephemeral job. Implementations: k8s (production) and fake (unit tests only).
type Executor interface {
	Run(ctx context.Context, spec Spec) (Result, error)
}

// Spec holds the normalized run parameters (resources already resolved to k8s
// quantities; requests are always set, limits are optional — "" means the ceiling
// comes from the namespace LimitRange).
type Spec struct {
	Image         string
	Command       []string
	Env           map[string]string
	Files         []File
	CPURequest    string
	MemoryRequest string
	CPULimit      string
	MemoryLimit   string
	Workdir       string
	Timeout       time.Duration
	Clone         *CloneSpec // when set, an init container clones the repo before main starts
}

// CloneSpec requests a repository clone by an init container (privilege separation:
// only the cloner sees the credentials). RepoURL/Ref come from the caller; the secret
// name and image come from the server config, never from the caller.
type CloneSpec struct {
	RepoURL string
	Ref     string
	Subdir  string
}

type File struct {
	Path    string
	Content []byte
	Mode    int64
}

// Result is the outcome of a run. Status is one of the Status* values above.
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
