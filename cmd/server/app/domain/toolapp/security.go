package toolapp

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/authapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

func (a *app) listKeys(ctx context.Context, r *http.Request) web.Encoder {
	bearerToken := r.Header.Get("Authorization")

	resp, err := a.authClient.ListKeys(ctx, bearerToken)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toKeys(resp.Keys)
}

func (a *app) createToken(ctx context.Context, r *http.Request) web.Encoder {
	var req TokenRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	bearerToken := r.Header.Get("Authorization")

	endpoints := make(map[string]*authapp.RateLimit)
	for name, rl := range req.Endpoints {
		window := string(rl.Window)
		endpoints[name] = authapp.RateLimit_builder{
			Limit:  new(int32(rl.Limit)),
			Window: &window,
		}.Build()
	}

	resp, err := a.authClient.CreateToken(ctx, bearerToken, req.Admin, endpoints, req.Duration)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return TokenResponse{
		Token: resp.Token,
	}
}

func (a *app) addKey(ctx context.Context, r *http.Request) web.Encoder {
	bearerToken := r.Header.Get("Authorization")

	if err := a.authClient.AddKey(ctx, bearerToken); err != nil {
		return errs.New(errs.Internal, err)
	}

	return nil
}

func (a *app) removeKey(ctx context.Context, r *http.Request) web.Encoder {
	keyID := web.Param(r, "keyid")
	if keyID == "" {
		return errs.Errorf(errs.InvalidArgument, "missing key id")
	}

	bearerToken := r.Header.Get("Authorization")

	if err := a.authClient.RemoveKey(ctx, bearerToken, keyID); err != nil {
		return errs.New(errs.Internal, err)
	}

	return nil
}
