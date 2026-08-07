package mid

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type authenticatorStub struct {
	calls    int
	admin    bool
	endpoint string
}

func (as *authenticatorStub) Authenticate(_ context.Context, _ string, admin bool, endpoint string) (authclient.AuthenticateReponse, error) {
	as.calls++
	as.admin = admin
	as.endpoint = endpoint
	return authclient.AuthenticateReponse{Subject: "subject"}, nil
}

func TestAccess(t *testing.T) {
	tests := []struct {
		name             string
		mode             auth.Mode
		legacyManagement bool
		middleware       func(Access) web.MidFunc
		wantCall         bool
		wantAdmin        bool
		wantEndpoint     string
	}{
		{name: "legacy discovery", middleware: func(a Access) web.MidFunc { return a.ModelDiscovery() }, wantCall: true},
		{name: "legacy inference", middleware: func(a Access) web.MidFunc { return a.Inference("responses") }, wantCall: true, wantEndpoint: "responses"},
		{name: "legacy management authenticated", middleware: func(a Access) web.MidFunc { return a.Management() }, wantCall: true},
		{name: "legacy management administrator", legacyManagement: true, middleware: func(a Access) web.MidFunc { return a.Management() }, wantCall: true, wantAdmin: true},
		{name: "legacy administration", middleware: func(a Access) web.MidFunc { return a.Administration() }, wantCall: true, wantAdmin: true},
		{name: "legacy playground grant", middleware: func(a Access) web.MidFunc { return a.Playground() }, wantCall: true, wantEndpoint: "playground"},
		{name: "legacy playground administrator", legacyManagement: true, middleware: func(a Access) web.MidFunc { return a.Playground() }, wantCall: true, wantAdmin: true},
		{name: "open discovery", mode: auth.Open, middleware: func(a Access) web.MidFunc { return a.ModelDiscovery() }},
		{name: "open inference", mode: auth.Open, middleware: func(a Access) web.MidFunc { return a.Inference("responses") }},
		{name: "open management", mode: auth.Open, middleware: func(a Access) web.MidFunc { return a.Management() }},
		{name: "open administration", mode: auth.Open, middleware: func(a Access) web.MidFunc { return a.Administration() }},
		{name: "open playground", mode: auth.Open, middleware: func(a Access) web.MidFunc { return a.Playground() }},
		{name: "management discovery", mode: auth.Management, middleware: func(a Access) web.MidFunc { return a.ModelDiscovery() }},
		{name: "management inference", mode: auth.Management, middleware: func(a Access) web.MidFunc { return a.Inference("responses") }},
		{name: "management route", mode: auth.Management, middleware: func(a Access) web.MidFunc { return a.Management() }, wantCall: true, wantAdmin: true},
		{name: "management administration", mode: auth.Management, middleware: func(a Access) web.MidFunc { return a.Administration() }, wantCall: true, wantAdmin: true},
		{name: "management playground", mode: auth.Management, middleware: func(a Access) web.MidFunc { return a.Playground() }, wantCall: true, wantAdmin: true},
		{name: "authenticated discovery", mode: auth.Authenticated, middleware: func(a Access) web.MidFunc { return a.ModelDiscovery() }, wantCall: true},
		{name: "authenticated inference", mode: auth.Authenticated, middleware: func(a Access) web.MidFunc { return a.Inference("responses") }, wantCall: true},
		{name: "authenticated management", mode: auth.Authenticated, middleware: func(a Access) web.MidFunc { return a.Management() }, wantCall: true, wantAdmin: true},
		{name: "full protected discovery", mode: auth.FullProtected, middleware: func(a Access) web.MidFunc { return a.ModelDiscovery() }, wantCall: true},
		{name: "full protected inference", mode: auth.FullProtected, middleware: func(a Access) web.MidFunc { return a.Inference("responses") }, wantCall: true, wantEndpoint: "responses"},
		{name: "full protected management", mode: auth.FullProtected, middleware: func(a Access) web.MidFunc { return a.Management() }, wantCall: true, wantAdmin: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &authenticatorStub{}
			access := Access{client: stub, mode: tt.mode, legacyManagementAccess: tt.legacyManagement}
			middleware := tt.middleware(access)
			handler := middleware(func(context.Context, *http.Request) web.Encoder { return nil })
			handler(context.Background(), httptest.NewRequest("GET", "/", nil))

			wantCalls := 0
			if tt.wantCall {
				wantCalls = 1
			}
			if stub.calls != wantCalls {
				t.Errorf("calls: got %d, want %d", stub.calls, wantCalls)
			}
			if stub.admin != tt.wantAdmin {
				t.Errorf("admin: got %t, want %t", stub.admin, tt.wantAdmin)
			}
			if stub.endpoint != tt.wantEndpoint {
				t.Errorf("endpoint: got %q, want %q", stub.endpoint, tt.wantEndpoint)
			}
		})
	}
}

func TestAuthenticationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code errs.ErrCode
	}{
		{name: "unauthenticated", err: status.Error(codes.Unauthenticated, "failed"), code: errs.Unauthenticated},
		{name: "permission denied", err: status.Error(codes.PermissionDenied, "failed"), code: errs.PermissionDenied},
		{name: "rate limited", err: status.Error(codes.ResourceExhausted, "failed"), code: errs.TooManyRequests},
		{name: "unavailable", err: status.Error(codes.Unavailable, "failed"), code: errs.Unavailable},
		{name: "deadline", err: status.Error(codes.DeadlineExceeded, "failed"), code: errs.DeadlineExceeded},
		{name: "unknown", err: errors.New("failed"), code: errs.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := authenticationError(tt.err)
			if !got.Code.Equal(tt.code) {
				t.Errorf("code: got %s, want %s", got.Code, tt.code)
			}
		})
	}
}
