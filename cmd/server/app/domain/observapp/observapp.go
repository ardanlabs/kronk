package observapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

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

func (a *app) status(ctx context.Context, r *http.Request) web.Encoder {
	return Status{Enabled: a.sessions != nil}
}

func (a *app) summary(ctx context.Context, r *http.Request) web.Encoder {
	if a.sessions == nil {
		return disabledError()
	}

	summary, err := a.sessions.Summary(ctx)
	if err != nil {
		if errors.Is(err, session.ErrClosed) {
			return errs.New(errs.Unavailable, err)
		}
		return errs.New(errs.Internal, err)
	}

	return Summary(summary)
}

func (a *app) list(ctx context.Context, r *http.Request) web.Encoder {
	if a.sessions == nil {
		return disabledError()
	}

	state, err := parseState(r.PathValue("state"))
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	query, err := parseQuery(r)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	page, err := a.sessions.List(ctx, state, query)
	if err != nil {
		if errors.Is(err, session.ErrClosed) {
			return errs.New(errs.Unavailable, err)
		}
		return errs.New(errs.Internal, err)
	}

	return Page(page)
}

func parseState(value string) (session.State, error) {
	state := session.State(value)
	switch state {
	case session.StateActive, session.StateIdle, session.StateCompleted:
		return state, nil
	default:
		return "", fmt.Errorf("invalid session state %q", value)
	}
}

func parseQuery(r *http.Request) (session.Query, error) {
	values := r.URL.Query()
	query := session.Query{
		ModelID: values.Get("model_id"),
	}

	if value := values.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 100 {
			return session.Query{}, fmt.Errorf("limit must be an integer from 1 through 100")
		}
		query.Limit = limit
	}

	if value := values.Get("offset"); value != "" {
		offset, err := strconv.Atoi(value)
		if err != nil || offset < 0 {
			return session.Query{}, fmt.Errorf("offset must be a non-negative integer")
		}
		query.Offset = offset
	}

	if value := values.Get("min_utilization"); value != "" {
		minimum, err := strconv.ParseFloat(value, 64)
		if err != nil || minimum < 0 {
			return session.Query{}, fmt.Errorf("min_utilization must be a non-negative number")
		}
		query.MinUtilization = minimum
	}

	return query, nil
}

func disabledError() *errs.Error {
	return errs.Errorf(errs.Unimplemented, "context session observability is not enabled")
}
