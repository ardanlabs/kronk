package mid

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/auth"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/web"
	"github.com/google/uuid"
)

// Bearer processes JWT authentication logic.
func Bearer(ath *auth.Auth) web.MidFunc {
	m := func(next web.HandlerFunc) web.HandlerFunc {
		h := func(ctx context.Context, r *http.Request) web.Encoder {
			authorizationHeader := r.Header.Get("authorization")
			ctx, err := HandleAuthentication(ctx, ath, authorizationHeader)
			if err != nil {
				return err
			}

			return next(ctx, r)
		}

		return h
	}

	return m
}

func HandleAuthentication(ctx context.Context, ath *auth.Auth, authorizationHeader string) (context.Context, *errs.Error) {
	if !ath.Enabled() {
		return ctx, nil
	}

	claims, err := ath.Authenticate(ctx, authorizationHeader)
	if err != nil {
		return ctx, errs.New(errs.Unauthenticated, err)
	}

	if claims.Subject == "" {
		return ctx, errs.Errorf(errs.Unauthenticated, "authorize: you are not authorized for that action, no claims")
	}

	subjectID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return ctx, errs.Errorf(errs.Unauthenticated, "parsing subject: %s", err)
	}

	ctx = setUserID(ctx, subjectID)
	ctx = setClaims(ctx, claims)

	return ctx, nil
}
