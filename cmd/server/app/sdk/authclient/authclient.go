// Package authclient provides support to access the auth service.
package authclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/authapp"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Client represents a client that can talk to the auth service.
type Client struct {
	log              *logger.Logger
	url              string
	grpcConn         *grpc.ClientConn
	grpc             authapp.AuthClient
	dialer           func(context.Context, string) (net.Conn, error)
	creds            credentials.TransportCredentials
	localAuth        bool
	authEnabled      bool
	adminAuthEnabled bool
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

	creds := cln.creds
	if creds == nil {
		creds = insecure.NewCredentials()
	}

	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}

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

// TLSCredentials constructs TLS transport credentials for an external auth
// service. An empty CA file uses the host's system certificate pool.
func TLSCredentials(caFile string, serverName string) (credentials.TransportCredentials, error) {
	tlsConfig := tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: serverName,
	}

	if caFile != "" {
		roots := x509.NewCertPool()
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("reading auth CA file: %w", err)
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("reading auth CA file: no certificates found")
		}

		tlsConfig.RootCAs = roots
	}

	return credentials.NewTLS(&tlsConfig), nil
}

// WithDialer sets a custom dialer for in-memory connections (e.g., bufconn).
func WithDialer(dialer func(context.Context, string) (net.Conn, error)) func(cln *Client) {
	return func(cln *Client) {
		cln.dialer = dialer
	}
}

// WithTransportCredentials configures credentials for the auth connection.
func WithTransportCredentials(creds credentials.TransportCredentials) func(cln *Client) {
	return func(cln *Client) {
		cln.creds = creds
	}
}

// WithLocalAuth configures the modes for an embedded auth service so disabled
// requests can bypass the in-process gRPC transport.
func WithLocalAuth(enabled bool, adminEnabled bool) func(cln *Client) {
	return func(cln *Client) {
		cln.localAuth = true
		cln.authEnabled = enabled
		cln.adminAuthEnabled = adminEnabled
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

var errAuthenticationDisabled = errors.New("authentication is disabled by auth service")

// injectTrace passes the web package's trace ID as gRPC metadata.
func injectTrace(ctx context.Context) context.Context {
	traceID := web.GetTraceID(ctx)
	return metadata.AppendToOutgoingContext(ctx, traceIDHeader, traceID)
}

// Authenticate calls the auth service to authenticate the user.
func (cln *Client) Authenticate(ctx context.Context, bearerToken string, admin bool, endpoint string) (AuthenticateReponse, error) {
	if cln.localAuth && !cln.authEnabled && !(admin && cln.adminAuthEnabled) {
		return AuthenticateReponse{Subject: uuid.Nil.String()}, nil
	}

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

// AuthenticateRequired authenticates the user and rejects an auth service
// configured to bypass authentication.
func (cln *Client) AuthenticateRequired(ctx context.Context, bearerToken string, admin bool, endpoint string) (AuthenticateReponse, error) {
	resp, err := cln.Authenticate(ctx, bearerToken, admin, endpoint)
	if err != nil {
		return AuthenticateReponse{}, err
	}
	if resp.Subject == uuid.Nil.String() {
		return AuthenticateReponse{}, errAuthenticationDisabled
	}

	return resp, nil
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
