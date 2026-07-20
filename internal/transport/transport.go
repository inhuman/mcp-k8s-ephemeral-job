package transport

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

const authHeader = "X-MCP-AUTH"

// Serve starts the server on the configured transport. The tool set is identical on all of them.
func Serve(ctx context.Context, cfg config.Config, srv *mcp.Server, log *zap.Logger) error {
	switch cfg.Transport {
	case "stdio":
		log.Info("serving MCP over stdio")
		if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
			return fmt.Errorf("stdio serve: %w", err)
		}
		return nil
	case "http":
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
		return listen(ctx, cfg, handler, log, "http")
	case "sse":
		handler := mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return srv }, nil)
		return listen(ctx, cfg, handler, log, "sse")
	default:
		return fmt.Errorf("unknown transport %q", cfg.Transport)
	}
}

func listen(ctx context.Context, cfg config.Config, handler http.Handler, log *zap.Logger, kind string) error {
	srv := &http.Server{Addr: cfg.Addr, Handler: withAuth(cfg.AuthToken, handler)}
	context.AfterFunc(ctx, func() {
		_ = srv.Close()
	})
	log.Info("serving MCP", zap.String("transport", kind), zap.String("addr", cfg.Addr), zap.Bool("auth", cfg.AuthToken != ""))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("%s serve: %w", kind, err)
	}
	return nil
}

func withAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get(authHeader))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
