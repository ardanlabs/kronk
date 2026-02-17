package mcpapp

import (
	"context"
	"net"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config holds the dependencies for the MCP handlers.
type Config struct {
	Log         *logger.Logger
	Listener    net.Listener
	BraveAPIKey string
}

// Start constructs and starts the MCP server.
func Start(ctx context.Context, cfg Config) *App {
	cfg.Log.Info(ctx, "mcp service", "status", "start mcp server")

	api := newApp(cfg)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "kronk-mcp",
		Version: "1.0.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "web_search",
		Description: "Performs a web search for the given query. Returns a list of relevant web pages with titles, URLs, and descriptions. Use this for general information gathering, research, and finding specific web resources.",
	}, api.webSearch)

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)

	api.httpServer = &http.Server{
		Handler: handler,
	}

	go func() {
		if err := api.httpServer.Serve(cfg.Listener); err != nil && err != http.ErrServerClosed {
			api.log.Error(ctx, "mcp server", "status", "mcp server error", "err", err)
		}
	}()

	return api
}
