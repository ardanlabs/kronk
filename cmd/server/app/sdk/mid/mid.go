// Package mid provides app level middleware support.
package mid

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

func checkIsError(e web.Encoder) error {
	err, hasError := e.(error)
	if hasError {
		return err
	}

	return nil
}

// =============================================================================

type ctxKey int

const (
	subjectKey ctxKey = iota + 1
)

func setSubject(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, subjectKey, subject)
}

// GetSubject returns the subject from the context.
func GetSubject(ctx context.Context) string {
	v, ok := ctx.Value(subjectKey).(string)
	if !ok {
		return ""
	}
	return v
}
