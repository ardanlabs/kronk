package authclient

import (
	"context"
	"errors"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/authapp"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

type authClientStub struct {
	authapp.AuthClient
	response *authapp.AuthenticateResponse
	err      error
}

func (acs authClientStub) Authenticate(context.Context, *authapp.AuthenticateRequest, ...grpc.CallOption) (*authapp.AuthenticateResponse, error) {
	return acs.response, acs.err
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
