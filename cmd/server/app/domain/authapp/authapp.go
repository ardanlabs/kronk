// Package authapp maintains the auth service handlers.
package authapp

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/otel"
	"github.com/ardanlabs/kronk/sdk/security/auth"
	"github.com/ardanlabs/kronk/sdk/tools/security"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

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

func newApp(cfg Config) *App {
	return &App{
		log:      cfg.Log,
		security: cfg.Security,
		lis:      cfg.Listener,
		tracer:   cfg.Tracer,
		enabled:  cfg.Enabled,
	}
}

// Shutdown stops the server.
func (a *App) Shutdown(ctx context.Context) {
	a.log.Info(ctx, "shutdown", "status", "auth server stopped")
	a.gs.GracefulStop()
}

// Authenticate validates a bearer token.
func (a *App) Authenticate(ctx context.Context, req *AuthenticateRequest) (*AuthenticateResponse, error) {
	if !a.enabled {
		a.log.Info(ctx, "***> auth", "status", "authentication disabled")

		arb := AuthenticateResponse_builder{
			Subject: proto.String(uuid.Nil.String()),
		}

		return arb.Build(), nil
	}

	a.log.Info(ctx, "auth", "method", "checking authentication")

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("no metadata")
	}

	bearerToken := md.Get("authorization")
	if len(bearerToken) == 0 {
		return nil, fmt.Errorf("unauthorized: no authorization header")
	}

	claims, err := a.security.Authenticate(ctx, bearerToken[0], req.GetAdmin(), req.GetEndpoint())
	if err != nil {
		a.log.Error(ctx, "authenticate", "err", err)
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	arb := AuthenticateResponse_builder{
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

	endpoints := make(map[string]auth.RateLimit)
	for name, rl := range req.GetEndpoints() {
		endpoints[name] = auth.RateLimit{
			Limit:  int(rl.GetLimit()),
			Window: auth.RateWindow(rl.GetWindow()),
		}
	}

	token, err := a.security.GenerateToken(req.GetAdmin(), endpoints, duration)
	if err != nil {
		a.log.Error(ctx, "token", "err", err)
		return nil, status.Error(codes.Internal, "failed to generate token")
	}

	trb := CreateTokenResponse_builder{
		Token: proto.String(token),
	}

	return trb.Build(), nil
}

// ListKeys returns all keys in the system.
func (a *App) ListKeys(ctx context.Context, req *ListKeysRequest) (*ListKeysResponse, error) {
	keys, err := a.security.ListKeys()
	if err != nil {
		a.log.Error(ctx, "listkeys", "err", err)
		return nil, status.Error(codes.Internal, "failed to list keys")
	}

	protoKeys := make([]*Key, len(keys))
	for i, key := range keys {
		kb := Key_builder{
			Id:      proto.String(key.ID),
			Created: proto.String(key.Created.Format("2006-01-02T15:04:05Z07:00")),
		}
		protoKeys[i] = kb.Build()
	}

	lkrb := ListKeysResponse_builder{
		Keys: protoKeys,
	}

	return lkrb.Build(), nil
}

// AddKey adds a new private key to the system.
func (a *App) AddKey(ctx context.Context, req *AddKeyRequest) (*AddKeyResponse, error) {
	if err := a.security.AddPrivateKey(); err != nil {
		a.log.Error(ctx, "addkey", "err", err)
		return nil, status.Error(codes.Internal, "failed to add key")
	}

	return &AddKeyResponse{}, nil
}

// RemoveKey removes a private key from the system.
func (a *App) RemoveKey(ctx context.Context, req *RemoveKeyRequest) (*RemoveKeyResponse, error) {
	keyID := req.GetKeyId()
	if keyID == "" {
		return nil, status.Error(codes.InvalidArgument, "missing key id")
	}

	if err := a.security.DeletePrivateKey(keyID); err != nil {
		a.log.Error(ctx, "removekey", "err", err)
		return nil, status.Error(codes.Internal, "failed to remove key")
	}

	return &RemoveKeyResponse{}, nil
}

// =============================================================================

func (a *App) authInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	ctx = otel.InjectTracing(ctx, a.tracer)

	switch info.FullMethod {
	case "/auth.Auth/CreateToken",
		"/auth.Auth/ListKeys",
		"/auth.Auth/AddKey",
		"/auth.Auth/RemoveKey":
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
