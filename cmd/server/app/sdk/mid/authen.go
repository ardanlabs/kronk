package mid

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/security/auth"
)

// Bearer processes JWT authentication logic.
func Bearer(ath *auth.Auth) web.MidFunc {
	m := func(next web.HandlerFunc) web.HandlerFunc {
		h := func(ctx context.Context, r *http.Request) web.Encoder {
			authorizationHeader := r.Header.Get("authorization")
			ctx, err := handleAuthentication(ctx, ath, authorizationHeader)
			if err != nil {
				return err
			}

			return next(ctx, r)
		}

		return h
	}

	return m
}

func Authorize(ath *auth.Auth, requireAdmin bool, endpoint string) web.MidFunc {
	m := func(next web.HandlerFunc) web.HandlerFunc {
		h := func(ctx context.Context, r *http.Request) web.Encoder {
			if err := handleAuthorization(ctx, ath, requireAdmin, endpoint); err != nil {
				return err
			}

			return next(ctx, r)
		}

		return h
	}

	return m
}

// =============================================================================

func handleAuthentication(ctx context.Context, ath *auth.Auth, authorizationHeader string) (context.Context, *errs.Error) {
	if !ath.Enabled() {
		return ctx, nil
	}

	claims, err := ath.Authenticate(ctx, authorizationHeader)
	if err != nil {
		return ctx, errs.New(errs.Unauthenticated, err)
	}

	if claims.Subject == "" {
		return ctx, errs.Errorf(errs.Unauthenticated, "authorize: you are not authorized for that action, no subject")
	}

	ctx = setClaims(ctx, claims)

	return ctx, nil
}

func handleAuthorization(ctx context.Context, ath *auth.Auth, requireAdmin bool, endpoint string) *errs.Error {
	if !ath.Enabled() {
		return nil
	}

	if err := ath.Authorize(ctx, GetClaims(ctx), requireAdmin, endpoint); err != nil {
		return errs.Errorf(errs.Unauthenticated, "authorize: you are not authorized for that action: requireAdmin[%v], endpoint[%s], err[%s]", requireAdmin, endpoint, err)
	}

	return nil
}
