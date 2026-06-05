package runjob

// Контракт тула run_job. Стабилен (Принцип III): несовместимое изменение схемы = MAJOR.

const (
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusTimeout   = "timeout"
	StatusError     = "error"
)

const DefaultWorkdir = "/work"

type Input struct {
	Image    string            `json:"image" jsonschema:"container image repo[:tag|@digest]; must be in the server allowlist"`
	Command  []string          `json:"command" jsonschema:"argv of the main process (no shell interpolation)"`
	Files    []InputFile       `json:"files,omitempty" jsonschema:"input files placed into the working directory before the command runs"`
	Env      map[string]string `json:"env,omitempty" jsonschema:"environment variables for the process"`
	Limits   *ResourceLimits   `json:"limits,omitempty" jsonschema:"CPU/memory resource limits"`
	TimeoutS int               `json:"timeout_s,omitempty" jsonschema:"wall-clock timeout in seconds"`
	Workdir  string            `json:"workdir,omitempty" jsonschema:"working directory in the pod, default /work"`
}

type InputFile struct {
	Path       string `json:"path" jsonschema:"relative path inside the working directory; no leading slash, no .."`
	ContentB64 string `json:"content_b64" jsonschema:"base64-encoded file content"`
	Mode       string `json:"mode,omitempty" jsonschema:"octal file mode, e.g. 0755; default 0644"`
}

type ResourceLimits struct {
	CPU    string `json:"cpu,omitempty" jsonschema:"CPU limit as a k8s quantity, e.g. 500m or 1"`
	Memory string `json:"memory,omitempty" jsonschema:"memory limit as a k8s quantity, e.g. 512Mi"`
}

type Output struct {
	ExitCode int `json:"exit_code"`
	// Stdout carries the container's COMBINED stdout+stderr: Kubernetes pod logs
	// merge the two streams and do not expose them separately.
	Stdout string `json:"stdout" jsonschema:"combined stdout+stderr from the container (Kubernetes merges the streams in pod logs)"`
	// Stderr is reserved and always empty — see Stdout. Kept for forward
	// compatibility so adding stream separation later is not a breaking change.
	Stderr     string     `json:"stderr" jsonschema:"reserved, always empty; Kubernetes pod logs do not separate stderr — read everything from stdout"`
	DurationMS int64      `json:"duration_ms"`
	Status     string     `json:"status"`
	Artifacts  []Artifact `json:"artifacts"`
	Truncated  Truncated  `json:"truncated"`
}

type Artifact struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	ContentB64 string `json:"content_b64"`
}

type Truncated struct {
	Stdout    bool `json:"stdout"`
	Stderr    bool `json:"stderr"`
	Artifacts bool `json:"artifacts"`
}
