// Package mcpapp maintains the MCP service handlers.
package mcpapp

import (
	"context"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
)

// App represents the MCP server application.
type App struct {
	log         *logger.Logger
	braveAPIKey string
	httpClient  *http.Client
	httpServer  *http.Server
}

func newApp(cfg Config) *App {
	return &App{
		log:         cfg.Log,
		braveAPIKey: cfg.BraveAPIKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Shutdown stops the MCP server.
func (a *App) Shutdown(ctx context.Context) {
	a.log.Info(ctx, "shutdown", "status", "mcp server stopping")

	if a.httpServer == nil {
		return
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	shutdownComplete := make(chan struct{})

	go func() {
		a.httpServer.Shutdown(shutdownCtx)
		close(shutdownComplete)
	}()

	select {
	case <-shutdownComplete:
		a.log.Info(ctx, "shutdown", "status", "mcp server stopped gracefully")
	case <-shutdownCtx.Done():
		a.log.Info(ctx, "shutdown", "status", "mcp server forcing close")
		a.httpServer.Close()
	}
}
