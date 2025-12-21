// Package authapp maintains the auth service handlers.
package authapp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/otel"
	"github.com/ardanlabs/kronk/sdk/security/auth"
	"github.com/ardanlabs/kronk/sdk/tools/security"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Config holds the dependencies for the auth handlers.
type Config struct {
	Log      *logger.Logger
	Security *security.Security
	Listener net.Listener
	Tracer   trace.Tracer
	Enabled  bool
}

// App represents the grpc handlers for the auth service.
type App struct {
	UnimplementedAuthServer
	log      *logger.Logger
	security *security.Security
	lis      net.Listener
	tracer   trace.Tracer
	enabled  bool
	gs       *grpc.Server
}

// New creates a new auth api.
func New(cfg Config) (*App, error) {
	app := App{
		log:      cfg.Log,
		security: cfg.Security,
		lis:      cfg.Listener,
		tracer:   cfg.Tracer,
		enabled:  cfg.Enabled,
	}

	s := grpc.NewServer(
		grpc.UnaryInterceptor(app.authInterceptor),
	)
	app.gs = s

	app.log.Info(context.Background(), "auth service", "status", "register auth server")

	RegisterAuthServer(s, &app)
	reflection.Register(s)

	return &app, nil
}

// Start starts the gRPC service.
func (a *App) Start(ctx context.Context) error {
	a.log.Info(ctx, "startup", "status", "auth server started")
	return a.gs.Serve(a.lis)
}

// Stop stops the gRPC service.
func (a *App) Stop(ctx context.Context) {
	a.log.Info(ctx, "shutdown", "status", "auth server stopped")
	a.gs.GracefulStop()
}

// Authenticate validates a bearer token.
func (a *App) Authenticate(ctx context.Context, req *AuthenticateRequest) (*AuthenticateResponse, error) {
	if !a.enabled {
		a.log.Info(ctx, "***> auth", "status", "authentication disabled")

		arb := AuthenticateResponse_builder{
			TokenId: proto.String("123"),
			Subject: proto.String("no user"),
		}

		return arb.Build(), nil
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("no metadata")
	}

	bearerToken := md.Get("authorization")
	if len(bearerToken) == 0 {
		return nil, fmt.Errorf("unauthorized: no authorization header")
	}

	a.log.Info(ctx, "auth", "method", "authenticate")

	claims, err := a.security.Auth.Authenticate(ctx, bearerToken[0])
	if err != nil {
		a.log.Error(ctx, "authenticate", "err", err)
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}

	a.log.Info(ctx, "auth", "method", "authorize")

	err = a.security.Auth.Authorize(ctx, claims, req.GetAdmin(), req.GetEndpoint())
	if err != nil {
		if errors.Is(err, auth.ErrForbidden) {
			return nil, status.Error(codes.PermissionDenied, "not authorized")
		}

		a.log.Error(ctx, "authorize", "err", err)
		return nil, status.Error(codes.Internal, "authorization failed")
	}

	tokenID := "not-implemented"

	arb := AuthenticateResponse_builder{
		TokenId: proto.String(tokenID),
		Subject: proto.String(claims.Subject),
	}

	return arb.Build(), nil
}

// CreateToken generates a new token.
func (a *App) CreateToken(ctx context.Context, req *CreateTokenRequest) (*CreateTokenResponse, error) {
	duration, err := time.ParseDuration(req.GetDuration())
	if err != nil {
		return nil, fmt.Errorf("parse-duration: %w", err)
	}

	token, err := a.security.GenerateToken(req.GetUserName(), req.GetAdmin(), req.GetEndpoints(), duration)
	if err != nil {
		a.log.Error(ctx, "token", "err", err)
		return nil, status.Error(codes.Internal, "failed to generate token")
	}

	trb := CreateTokenResponse_builder{
		Token: proto.String(token),
	}

	return trb.Build(), nil
}

// =============================================================================

func (a *App) authInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	ctx = otel.InjectTracing(ctx, a.tracer)

	switch info.FullMethod {
	case "/auth.Auth/Token":
		return a.requireAuth(ctx, true, "", req, handler)

	default:
		return handler(ctx, req)
	}
}

func (a *App) requireAuth(ctx context.Context, admin bool, endpoint string, req any, handler grpc.UnaryHandler) (any, error) {
	if !a.enabled {
		a.log.Info(ctx, "***> auth", "status", "authentication disabled")
		return handler(ctx, req)
	}

	arb := AuthenticateRequest_builder{
		Admin:    proto.Bool(admin),
		Endpoint: proto.String(endpoint),
	}

	if _, err := a.Authenticate(ctx, arb.Build()); err != nil {
		return nil, err
	}

	return handler(ctx, req)
}
