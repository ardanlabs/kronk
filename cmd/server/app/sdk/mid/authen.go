package mid

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Authenticate calls out to the auth service to authenticate the call.
func Authenticate(client *authclient.Client, admin bool, endpoint string) web.MidFunc {
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
