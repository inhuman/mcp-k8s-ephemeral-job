package main

import (
	"context"
	"os"
	"os/signal"
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

	exec, err := executor.NewK8s(executor.K8sOptions{
		Kubeconfig:   cfg.Kubeconfig,
		Namespace:    cfg.Namespace,
		SidecarImage: cfg.SidecarImage,
		CloneImage:   cfg.CloneImage,
		CloneSecret:  cfg.CloneSecret,
		TTLSeconds:   jobTTLSeconds,
		MaxOutput:    cfg.MaxOutputBytes,
		MaxArtifact:  cfg.MaxArtifactBytes,
	}, log)
	if err != nil {
		return err
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: version}, nil)
	runjob.NewTool(exec, runjob.Options{
		DefaultTimeoutS: cfg.DefaultTimeoutS,
		MaxTimeoutS:     cfg.MaxTimeoutS,
		DefaultCPU:      cfg.DefaultCPU,
		DefaultMemory:   cfg.DefaultMemory,
		AllowedImages:   cfg.AllowedImages,
	}, log).Register(srv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return transport.Serve(ctx, cfg, srv, log)
}
