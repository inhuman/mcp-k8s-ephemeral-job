package config

import (
	"fmt"

	"github.com/inhuman/config"
)

type Config struct {
	Transport string `env:"MCP_K8S_TRANSPORT" env-default:"stdio"`
	Addr      string `env:"MCP_K8S_ADDR" env-default:":8080"`
	Namespace string `env:"MCP_K8S_NAMESPACE" env-default:"mcp-ephemeral"`

	DefaultTimeoutS int `env:"MCP_K8S_DEFAULT_TIMEOUT_S" env-default:"60"`
	MaxTimeoutS     int `env:"MCP_K8S_MAX_TIMEOUT_S" env-default:"600"`

	MaxOutputBytes   int64 `env:"MCP_K8S_MAX_OUTPUT_BYTES" env-default:"1048576"`
	MaxArtifactBytes int64 `env:"MCP_K8S_MAX_ARTIFACT_BYTES" env-default:"10485760"`

	DefaultCPU    string `env:"MCP_K8S_DEFAULT_CPU" env-default:"1"`
	DefaultMemory string `env:"MCP_K8S_DEFAULT_MEMORY" env-default:"512Mi"`

	MaxConcurrent int `env:"MCP_K8S_MAX_CONCURRENT" env-default:"10"`

	AllowedImages []string `env:"MCP_K8S_ALLOWED_IMAGES" env-separator:","`
	SidecarImage  string   `env:"MCP_K8S_SIDECAR_IMAGE" env-default:"busybox:1.36"`

	// git-clone init container: an image carrying git, plus a k8s secret with a
	// "token" key. When both are set, run_job accepts the `clone` field. The secret
	// is mounted ONLY on the cloner — the main container never sees the credentials.
	CloneImage  string `env:"MCP_K8S_CLONE_IMAGE"`
	CloneSecret string `env:"MCP_K8S_CLONE_SECRET"`

	// JobExtraEnv is a JSON object {"KEY":"value"}: env vars added to EVERY job pod
	// (main container). Cluster-level configuration for the scripts running inside
	// jobs — e.g. a package-mirror URL so installs work without public internet.
	// Caller-supplied keys (Input.Env) OVERRIDE the server ones on collision.
	JobExtraEnv string `env:"MCP_K8S_JOB_EXTRA_ENV"`

	Kubeconfig string `env:"MCP_K8S_KUBECONFIG"`
	AuthToken  string `env:"MCP_K8S_AUTH_TOKEN" mask:"filled"`

	// Persistent cache volume mounted into every ephemeral pod (main + sidecar).
	// Both fields must be set together; either empty → no cache mount.
	// Typical use: Go module cache. CachePVC=go-modcache,
	// CacheMountPath=/go/pkg/mod → all pods share the downloaded modules.
	// The PVC itself is provisioned out-of-band (helm chart / manifest), the
	// server only references it by name.
	CachePVC       string `env:"MCP_K8S_CACHE_PVC"`
	CacheMountPath string `env:"MCP_K8S_CACHE_MOUNT_PATH"`
}

func Load() (Config, error) {
	var c Config
	if err := config.Load(&c); err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	return c, nil
}
