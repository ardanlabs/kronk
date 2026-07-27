package mid

import (
	"context"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

// Timeout bounds the time available to the next handler.
func Timeout(duration time.Duration) web.MidFunc {
	return func(next web.HandlerFunc) web.HandlerFunc {
		return func(ctx context.Context, r *http.Request) web.Encoder {
			timeoutCtx, cancel := context.WithTimeout(ctx, duration)
			defer cancel()

			r = r.WithContext(timeoutCtx)
			return next(timeoutCtx, r)
		}
	}
}
