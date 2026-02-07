package mid

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
)

// Metrics updates program counters.
func Metrics() web.MidFunc {
	m := func(next web.HandlerFunc) web.HandlerFunc {
		h := func(ctx context.Context, r *http.Request) web.Encoder {
			resp := next(ctx, r)

			n := metrics.AddRequests()

			if n%10 == 0 {
				metrics.UpdateGoroutines()
			}

			if checkIsError(resp) != nil {
				metrics.AddErrors()
			}

			return resp
		}

		return h
	}

	return m
}
