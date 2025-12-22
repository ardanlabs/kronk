package authapp

import (
	"context"
	"net"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/sdk/tools/security"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Config holds the dependencies for the auth handlers.
type Config struct {
	Log      *logger.Logger
	Security *security.Security
	Listener net.Listener
	Tracer   trace.Tracer
	Enabled  bool
}

// Start constructs the registers the auth app to the grpc server.
func Start(ctx context.Context, cfg Config) *App {
	cfg.Log.Info(context.Background(), "auth service", "status", "start auth server")

	api := newApp(cfg)

	gs := grpc.NewServer(
		grpc.UnaryInterceptor(api.authInterceptor),
	)

	api.gs = gs

	RegisterAuthServer(gs, api)
	reflection.Register(gs)

	go func() {
		if err := gs.Serve(cfg.Listener); err != nil {
			api.log.Error(ctx, "startup", "status", "auth server error", "err", err)
		}
	}()

	return api
}
