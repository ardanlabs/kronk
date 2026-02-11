// Package authclient provides support to access the auth service.
package authclient

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/authapp"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Client represents a client that can talk to the auth service.
type Client struct {
	log      *logger.Logger
	url      string
	grpcConn *grpc.ClientConn
	grpc     authapp.AuthClient
	dialer   func(context.Context, string) (net.Conn, error)
}

// New constructs an Auth that can be used to talk with the auth service.
func New(log *logger.Logger, url string, options ...func(cln *Client)) (*Client, error) {
	cln := Client{
		log: log,
		url: url,
	}

	for _, option := range options {
		option(&cln)
	}

	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	if cln.dialer != nil {
		dialOpts = append(dialOpts, grpc.WithContextDialer(cln.dialer))
	}

	grpcConn, err := grpc.NewClient(url, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to auth gRPC service: %w", err)
	}

	cln.grpcConn = grpcConn
	cln.grpc = authapp.NewAuthClient(grpcConn)

	return &cln, nil
}

// WithDialer sets a custom dialer for in-memory connections (e.g., bufconn).
func WithDialer(dialer func(context.Context, string) (net.Conn, error)) func(cln *Client) {
	return func(cln *Client) {
		cln.dialer = dialer
	}
}

// Close is used to close the connections.
func (cln *Client) Close() error {
	if cln.grpcConn != nil {
		return cln.grpcConn.Close()
	}

	return nil
}

const traceIDHeader = "x-trace-id"

// injectTrace passes the web package's trace ID as gRPC metadata.
func injectTrace(ctx context.Context) context.Context {
	traceID := web.GetTraceID(ctx)
	return metadata.AppendToOutgoingContext(ctx, traceIDHeader, traceID)
}

// Authenticate calls the auth service to authenticate the user.
func (cln *Client) Authenticate(ctx context.Context, bearerToken string, admin bool, endpoint string) (AuthenticateReponse, error) {
	arb := authapp.AuthenticateRequest_builder{
		Admin:    &admin,
		Endpoint: &endpoint,
	}

	ctx = injectTrace(ctx)
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", bearerToken)

	req, err := cln.grpc.Authenticate(ctx, arb.Build())
	if err != nil {
		return AuthenticateReponse{}, err
	}

	return toAuthenticateReponse(req), nil
}

// CreateToken calls the auth service to create a new token.
func (cln *Client) CreateToken(ctx context.Context, bearerToken string, admin bool, endpoints map[string]*authapp.RateLimit, duration time.Duration) (CreateTokenResponse, error) {
	protoEndpoints := make(map[string]*authapp.RateLimit)
	for name, rl := range endpoints {
		protoEndpoints[name] = authapp.RateLimit_builder{
			Limit:  new(rl.GetLimit()),
			Window: new(rl.GetWindow()),
		}.Build()
	}

	arb := authapp.CreateTokenRequest_builder{
		Admin:     &admin,
		Endpoints: protoEndpoints,
		Duration:  new(duration.String()),
	}

	ctx = injectTrace(ctx)
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", bearerToken)

	req, err := cln.grpc.CreateToken(ctx, arb.Build())
	if err != nil {
		return CreateTokenResponse{}, err
	}

	return toCreateTokenResponse(req), nil
}

// ListKeys calls the auth service to list all keys.
func (cln *Client) ListKeys(ctx context.Context, bearerToken string) (ListKeysResponse, error) {
	ctx = injectTrace(ctx)
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", bearerToken)

	req, err := cln.grpc.ListKeys(ctx, &authapp.ListKeysRequest{})
	if err != nil {
		return ListKeysResponse{}, err
	}

	return toListKeysResponse(req), nil
}

// AddKey calls the auth service to add a new key.
func (cln *Client) AddKey(ctx context.Context, bearerToken string) error {
	ctx = injectTrace(ctx)
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", bearerToken)

	_, err := cln.grpc.AddKey(ctx, &authapp.AddKeyRequest{})
	return err
}

// RemoveKey calls the auth service to remove a key.
func (cln *Client) RemoveKey(ctx context.Context, bearerToken string, keyID string) error {
	rkb := authapp.RemoveKeyRequest_builder{
		KeyId: &keyID,
	}

	ctx = injectTrace(ctx)
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", bearerToken)

	_, err := cln.grpc.RemoveKey(ctx, rkb.Build())
	return err
}
