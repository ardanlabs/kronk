package mcpapp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// statusRecorder wraps http.ResponseWriter to capture the status code that
// the SDK handler writes back, so the access log can include it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// peekJSONRPC pulls the JSON-RPC method (and tool name, when applicable) out
// of an MCP request body without consuming it for the downstream handler.
// Returns ("", "", "") for non-POST or non-JSON requests.
func peekJSONRPC(r *http.Request) (rpcMethod, toolName, rpcID string, err error) {
	if r.Method != http.MethodPost || r.Body == nil {
		return "", "", "", nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", "", "", err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	var env struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return "", "", "", nil // not JSON, leave body intact for handler
	}

	id := ""
	if len(env.ID) > 0 {
		id = string(env.ID)
	}
	return env.Method, env.Params.Name, id, nil
}

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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fuzzy_edit",
		Description: "Edit a file by replacing old_string with new_string. Uses tiered fuzzy matching: exact match, line-ending normalization, then indentation-insensitive matching. Prefer this over the built-in edit tool for more reliable replacements.",
	}, api.fuzzyEdit)

	// Run the streamable HTTP handler in stateful mode with an in-memory
	// event store. Sessions are tracked via the Mcp-Session-Id header and
	// SSE replay buffers live only in process memory, so any time the Kronk
	// service is restarted (deploy, crash, config change) every existing
	// session ID is forgotten. The MCP spec accounts for this: the server
	// will respond to a stale session ID with HTTP 404, and a spec-compliant
	// client (Cline, Kilo, OpenCode, Goose, etc.) is required to react by
	// sending a fresh InitializeRequest without a session ID and continuing
	// with the new ID the server returns. Clients that cache a session ID
	// across our restarts and refuse to re-initialize on 404 are not
	// behaving per the spec and will need to reconnect manually.
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		EventStore: mcp.NewMemoryEventStore(nil),
	})

	logged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rpcMethod, toolName, rpcID, peekErr := peekJSONRPC(r)
		if peekErr != nil {
			cfg.Log.Error(r.Context(), "mcp request", "status", "body read failed", "err", peekErr)
		}

		cfg.Log.Info(r.Context(), "mcp request",
			"http_method", r.Method,
			"path", r.URL.Path,
			"remoteaddr", r.RemoteAddr,
			"accept", r.Header.Get("Accept"),
			"session", r.Header.Get("Mcp-Session-Id"),
			"rpc_method", rpcMethod,
			"rpc_tool", toolName,
			"rpc_id", rpcID,
		)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		handler.ServeHTTP(rec, r)

		cfg.Log.Info(r.Context(), "mcp response",
			"http_method", r.Method,
			"path", r.URL.Path,
			"session", r.Header.Get("Mcp-Session-Id"),
			"resp_session", rec.Header().Get("Mcp-Session-Id"),
			"rpc_method", rpcMethod,
			"rpc_tool", toolName,
			"rpc_id", rpcID,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})

	mux := http.NewServeMux()
	mux.Handle("/mcp", logged)

	api.httpServer = &http.Server{
		Handler: mux,
	}

	go func() {
		if err := api.httpServer.Serve(cfg.Listener); err != nil && err != http.ErrServerClosed {
			api.log.Error(ctx, "mcp server", "status", "mcp server error", "err", err)
		}
	}()

	return api
}
