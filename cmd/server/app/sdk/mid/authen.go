package mid

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Access selects route middleware from an authorization mode. An unset mode
// preserves the legacy authorization settings during migration.
type Access struct {
	client                 authenticator
	mode                   auth.Mode
	legacyManagementAccess bool
}

type authenticator interface {
	Authenticate(ctx context.Context, bearerToken string, admin bool, endpoint string) (authclient.AuthenticateReponse, error)
}

// NewAccess constructs route access middleware for an authorization mode.
func NewAccess(client *authclient.Client, mode auth.Mode, legacyManagementAccess bool) Access {
	return Access{
		client:                 client,
		mode:                   mode,
		legacyManagementAccess: legacyManagementAccess,
	}
}

// ModelDiscovery returns access middleware for model discovery routes.
func (a *Access) ModelDiscovery() web.MidFunc {
	if a.mode.IsZero() {
		return authenticate(a.client, false, "")
	}
	if a.mode.Equal(auth.Open) || a.mode.Equal(auth.Management) {
		return publicAccess()
	}

	return authenticate(a.client, false, "")
}

// Inference returns access middleware for an inference endpoint grant.
func (a Access) Inference(endpoint string) web.MidFunc {
	if a.mode.IsZero() {
		return authenticate(a.client, false, endpoint)
	}
	if a.mode.Equal(auth.Open) || a.mode.Equal(auth.Management) {
		return publicAccess()
	}
	if a.mode.Equal(auth.Authenticated) {
		return authenticate(a.client, false, "")
	}

	return authenticate(a.client, false, endpoint)
}

// Management returns access middleware for management routes. In legacy mode,
// this preserves the existing conditional administrator requirement.
func (a Access) Management() web.MidFunc {
	if a.mode.IsZero() {
		return authenticate(a.client, a.legacyManagementAccess, "")
	}
	if a.mode.Equal(auth.Open) {
		return publicAccess()
	}

	return authenticate(a.client, true, "")
}

// Administration returns access middleware for routes that always used the
// legacy administrator check. Explicit authorization modes classify these as
// management routes.
func (a Access) Administration() web.MidFunc {
	if a.mode.IsZero() {
		return authenticate(a.client, true, "")
	}

	return a.Management()
}

// Playground returns access middleware for playground routes while preserving
// their legacy endpoint grant when no authorization mode is configured.
func (a Access) Playground() web.MidFunc {
	if a.mode.IsZero() {
		if a.legacyManagementAccess {
			return authenticate(a.client, true, "")
		}
		return authenticate(a.client, false, "playground")
	}

	return a.Management()
}

func publicAccess() web.MidFunc {
	return func(next web.HandlerFunc) web.HandlerFunc {
		return next
	}
}

func authenticate(client authenticator, admin bool, endpoint string) web.MidFunc {
	m := func(next web.HandlerFunc) web.HandlerFunc {
		h := func(ctx context.Context, r *http.Request) web.Encoder {
			ar, err := client.Authenticate(ctx, r.Header.Get("authorization"), admin, endpoint)
			if err != nil {
				return authenticationError(err)
			}

			ctx = setSubject(ctx, ar.Subject)

			return next(ctx, r)
		}

		return h
	}

	return m
}

func authenticationError(err error) *errs.Error {
	code := errs.Internal

	switch status.Code(err) {
	case codes.Unauthenticated:
		code = errs.Unauthenticated
	case codes.PermissionDenied:
		code = errs.PermissionDenied
	case codes.ResourceExhausted:
		code = errs.TooManyRequests
	case codes.Unavailable:
		code = errs.Unavailable
	case codes.DeadlineExceeded:
		code = errs.DeadlineExceeded
	}

	return errs.New(code, err)
}
