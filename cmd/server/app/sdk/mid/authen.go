package mid

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

// Authenticate calls out to the auth service to authenticate the call.
func Authenticate(enabled bool, client *authclient.Client, admin bool, endpoint string) web.MidFunc {
	m := func(next web.HandlerFunc) web.HandlerFunc {
		h := func(ctx context.Context, r *http.Request) web.Encoder {
			if !enabled {
				return next(ctx, r)
			}

			ar, err := client.Authenticate(ctx, r.Header.Get("authorization"), admin, endpoint)
			if err != nil {
				return errs.New(errs.Unauthenticated, err)
			}

			ctx = setSubject(ctx, ar.Subject)
			ctx = setTokenID(ctx, ar.TokenID)

			return next(ctx, r)
		}

		return h
	}

	return m
}
