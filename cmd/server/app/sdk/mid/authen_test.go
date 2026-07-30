package mid

import (
	"errors"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAuthenticationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code errs.ErrCode
	}{
		{name: "unauthenticated", err: status.Error(codes.Unauthenticated, "failed"), code: errs.Unauthenticated},
		{name: "permission denied", err: status.Error(codes.PermissionDenied, "failed"), code: errs.PermissionDenied},
		{name: "rate limited", err: status.Error(codes.ResourceExhausted, "failed"), code: errs.TooManyRequests},
		{name: "unavailable", err: status.Error(codes.Unavailable, "failed"), code: errs.Unavailable},
		{name: "deadline", err: status.Error(codes.DeadlineExceeded, "failed"), code: errs.DeadlineExceeded},
		{name: "unknown", err: errors.New("failed"), code: errs.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := authenticationError(tt.err)
			if !got.Code.Equal(tt.code) {
				t.Errorf("code: got %s, want %s", got.Code, tt.code)
			}
		})
	}
}
