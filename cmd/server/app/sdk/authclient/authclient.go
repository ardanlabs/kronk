// Package authclient provides support to access the auth service.
package authclient

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/authapp"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
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

func (cln *Client) Close() error {
	if cln.grpcConn != nil {
		return cln.grpcConn.Close()
	}

	return nil
}

// Authenticate calls the auth service to authenticate the user.
func (cln *Client) Authenticate(ctx context.Context, bearerToken string, admin bool, endpoint string) (AuthenticateReponse, error) {
	arb := authapp.AuthenticateRequest_builder{
		Admin:    proto.Bool(admin),
		Endpoint: proto.String(endpoint),
	}

	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", bearerToken)

	req, err := cln.grpc.Authenticate(ctx, arb.Build())
	if err != nil {
		return AuthenticateReponse{}, err
	}

	return toAuthenticateReponse(req), nil
}

// CreateToken calls the auth service to create a new token.
func (cln *Client) CreateToken(ctx context.Context, bearerToken string, userName string, admin bool, endpoints []string, duration time.Duration) (CreateTokenResponse, error) {
	arb := authapp.CreateTokenRequest_builder{
		UserName:  proto.String(userName),
		Admin:     proto.Bool(admin),
		Endpoints: endpoints,
		Duration:  proto.String(duration.String()),
	}

	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", bearerToken)

	req, err := cln.grpc.CreateToken(ctx, arb.Build())
	if err != nil {
		return CreateTokenResponse{}, err
	}

	return toCreateTokenResponse(req), nil
}
