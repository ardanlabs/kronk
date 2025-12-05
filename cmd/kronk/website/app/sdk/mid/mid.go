// Package mid provides app level middleware support.
package mid

import (
	"context"
	"errors"

	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/auth"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/web"
	"github.com/google/uuid"
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
	claimKey ctxKey = iota + 1
	userIDKey
	userKey
	productKey
	homeKey
	trKey
)

func setClaims(ctx context.Context, claims auth.Claims) context.Context {
	return context.WithValue(ctx, claimKey, claims)
}

// GetClaims returns the claims from the context.
func GetClaims(ctx context.Context) auth.Claims {
	v, ok := ctx.Value(claimKey).(auth.Claims)
	if !ok {
		return auth.Claims{}
	}
	return v
}

// GetSubjectID returns the subject id from the claims.
func GetSubjectID(ctx context.Context) uuid.UUID {
	v := GetClaims(ctx)

	subjectID, err := uuid.Parse(v.Subject)
	if err != nil {
		return uuid.UUID{}
	}

	return subjectID
}

func setUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// GetUserID returns the user id from the context.
func GetUserID(ctx context.Context) (uuid.UUID, error) {
	v, ok := ctx.Value(userIDKey).(uuid.UUID)
	if !ok {
		return uuid.UUID{}, errors.New("user id not found in context")
	}

	return v, nil
}
