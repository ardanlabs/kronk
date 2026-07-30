package observapp

import (
	"context"
	"errors"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/session"
)

type app struct {
	sessions *session.Tracker
}

func newApp(sessions *session.Tracker) *app {
	return &app{sessions: sessions}
}

func (a *app) list(ctx context.Context, r *http.Request) web.Encoder {
	if a.sessions == nil {
		return Sessions{Enabled: false, Sessions: []session.Summary{}}
	}

	summaries, err := a.sessions.List()
	if err != nil {
		if errors.Is(err, session.ErrClosed) {
			return errs.New(errs.Unavailable, err)
		}
		return errs.New(errs.Internal, err)
	}

	return Sessions{Enabled: true, Sessions: summaries}
}
