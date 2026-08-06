package authclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/authapp"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type authClientStub struct {
	authapp.AuthClient
	response *authapp.AuthenticateResponse
	err      error
	calls    *int
}

func TestTLSCredentials(t *testing.T) {
	t.Run("system roots", func(t *testing.T) {
		creds, err := TLSCredentials("", "auth.example.com")
		if err != nil {
			t.Fatalf("TLSCredentials: %v", err)
		}

		info := creds.Info()
		if info.SecurityProtocol != "tls" {
			t.Errorf("security protocol: got %q, want tls", info.SecurityProtocol)
		}
	})

	t.Run("missing CA file", func(t *testing.T) {
		_, err := TLSCredentials(filepath.Join(t.TempDir(), "missing.pem"), "")
		if err == nil {
			t.Fatal("TLSCredentials: got nil, want error")
		}
	})

	t.Run("invalid CA file", func(t *testing.T) {
		caFile := filepath.Join(t.TempDir(), "ca.pem")
		if err := os.WriteFile(caFile, []byte("not a certificate"), 0600); err != nil {
			t.Fatalf("write CA file: %v", err)
		}

		_, err := TLSCredentials(caFile, "")
		if err == nil {
			t.Fatal("TLSCredentials: got nil, want error")
		}
	})

	t.Run("custom CA handshake", func(t *testing.T) {
		certFile, keyFile := writeTestCertificate(t)
		serverCreds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
		if err != nil {
			t.Fatalf("server TLS credentials: %v", err)
		}

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}

		server := grpc.NewServer(grpc.Creds(serverCreds))
		serveErr := make(chan error, 1)
		go func() {
			serveErr <- server.Serve(listener)
		}()
		t.Cleanup(func() {
			server.Stop()
			if err := <-serveErr; err != nil {
				t.Errorf("serve: %v", err)
			}
			if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				t.Errorf("close listener: %v", err)
			}
		})

		clientCreds, err := TLSCredentials(certFile, "localhost")
		if err != nil {
			t.Fatalf("client TLS credentials: %v", err)
		}
		conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(clientCreds))
		if err != nil {
			t.Fatalf("new client: %v", err)
		}
		defer conn.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = conn.Invoke(ctx, "/test.Service/Missing", &emptypb.Empty{}, &emptypb.Empty{})
		if got := status.Code(err); got != codes.Unimplemented {
			t.Fatalf("Invoke: got %v (%s), want Unimplemented after successful TLS handshake", err, got)
		}
	})
}

func writeTestCertificate(t *testing.T) (string, string) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		DNSNames:              []string{"localhost"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	key, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}

	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatalf("write certificate: %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	return certFile, keyFile
}

func (acs authClientStub) Authenticate(context.Context, *authapp.AuthenticateRequest, ...grpc.CallOption) (*authapp.AuthenticateResponse, error) {
	if acs.calls != nil {
		(*acs.calls)++
	}

	return acs.response, acs.err
}

func TestAuthenticateLocalModes(t *testing.T) {
	subject := uuid.NewString()

	tests := []struct {
		name         string
		local        bool
		enabled      bool
		adminEnabled bool
		admin        bool
		wantCall     bool
		wantSubject  string
	}{
		{name: "external configuration remains authoritative", wantCall: true, wantSubject: subject},
		{name: "local authentication disabled", local: true, wantSubject: uuid.Nil.String()},
		{name: "local admin authentication disabled", local: true, admin: true, wantSubject: uuid.Nil.String()},
		{name: "local admin-only inference bypass", local: true, adminEnabled: true, wantSubject: uuid.Nil.String()},
		{name: "local admin-only admin call", local: true, adminEnabled: true, admin: true, wantCall: true, wantSubject: subject},
		{name: "local authentication enabled", local: true, enabled: true, wantCall: true, wantSubject: subject},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			response := authapp.AuthenticateResponse_builder{Subject: &subject}.Build()
			cln := Client{grpc: authClientStub{response: response, calls: &calls}}
			if tt.local {
				WithLocalAuth(tt.enabled, tt.adminEnabled)(&cln)
			}

			got, err := cln.Authenticate(context.Background(), "", tt.admin, "chat-completions")
			if err != nil {
				t.Fatalf("Authenticate: got error %v, want nil", err)
			}
			if got.Subject != tt.wantSubject {
				t.Errorf("subject: got %q, want %q", got.Subject, tt.wantSubject)
			}

			wantCalls := 0
			if tt.wantCall {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Errorf("calls: got %d, want %d", calls, wantCalls)
			}
		})
	}
}

func TestAuthenticateRequired(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		wantErr error
	}{
		{name: "authenticated", subject: uuid.NewString()},
		{name: "authentication disabled", subject: uuid.Nil.String(), wantErr: errAuthenticationDisabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := authapp.AuthenticateResponse_builder{Subject: &tt.subject}.Build()
			cln := Client{grpc: authClientStub{response: response}}

			_, err := cln.AuthenticateRequired(context.Background(), "", true, "")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("AuthenticateRequired: got error %v, want %v", err, tt.wantErr)
			}
		})
	}
}
