package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/config"
	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/executor"
	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/runjob"
	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/transport"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

const (
	serverName    = "mcp-k8s-ephemeral-job"
	jobTTLSeconds = 120
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	log, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer func() { _ = log.Sync() }()

	if err := run(log); err != nil {
		log.Fatal("fatal", zap.Error(err))
	}
}

func run(log *zap.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	extraEnv, err := parseJobExtraEnv(cfg.JobExtraEnv)
	if err != nil {
		return fmt.Errorf("MCP_K8S_JOB_EXTRA_ENV: %w", err)
	}
	exec, err := executor.NewK8s(executor.K8sOptions{
		Kubeconfig:     cfg.Kubeconfig,
		Namespace:      cfg.Namespace,
		SidecarImage:   cfg.SidecarImage,
		CloneImage:     cfg.CloneImage,
		CloneSecret:    cfg.CloneSecret,
		CachePVC:       cfg.CachePVC,
		CacheMountPath: cfg.CacheMountPath,
		TTLSeconds:     jobTTLSeconds,
		MaxOutput:      cfg.MaxOutputBytes,
		MaxArtifact:    cfg.MaxArtifactBytes,
		ExtraEnv:       extraEnv,
	}, log)
	if err != nil {
		return err
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: version}, nil)
	tool := runjob.NewTool(exec, runjob.Options{
		DefaultTimeoutS: cfg.DefaultTimeoutS,
		MaxTimeoutS:     cfg.MaxTimeoutS,
		DefaultCPU:      cfg.DefaultCPU,
		DefaultMemory:   cfg.DefaultMemory,
		AllowedImages:   cfg.AllowedImages,
	}, log)
	tool.Register(srv)
	runjob.NewAsync(tool, log).Register(srv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return transport.Serve(ctx, cfg, srv, log)
}

// parseJobExtraEnv parses MCP_K8S_JOB_EXTRA_ENV — a JSON object {"KEY":"value"}.
// Empty = nil. Invalid JSON fails fast at startup rather than silently no-opping.
func parseJobExtraEnv(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("expected a JSON object {\"KEY\":\"value\"}: %w", err)
	}
	return m, nil
}
