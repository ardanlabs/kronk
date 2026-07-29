package errs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	buckyffmpeg "github.com/ardanlabs/kronk/sdk/bucky/ffmpeg"
	buckypool "github.com/ardanlabs/kronk/sdk/bucky/pool"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/hf"
	"github.com/ardanlabs/kronk/sdk/kronk/jsonrepair"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	kronkpool "github.com/ardanlabs/kronk/sdk/kronk/pool"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	"github.com/ardanlabs/kronk/sdk/tools/github"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
)

func TestFromSDK(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code ErrCode
	}{
		{name: "context canceled", err: context.Canceled, code: Canceled},
		{name: "context deadline", err: context.DeadlineExceeded, code: DeadlineExceeded},
		{name: "file inputs unsupported", err: model.ErrFileInputsUnsupported, code: InvalidArgument},
		{name: "messages missing", err: model.ErrMessagesMissing, code: InvalidArgument},
		{name: "messages invalid", err: model.ErrMessagesInvalid, code: InvalidArgument},
		{name: "kronk pool server busy", err: kronkpool.ErrServerBusy, code: Unavailable},
		{name: "kronk pool no capacity", err: kronkpool.ErrNoCapacity, code: ResourceExhausted},
		{name: "admission timeout", err: kronk.ErrAdmissionTimeout, code: ResourceExhausted},
		{name: "resource manager no capacity", err: resman.ErrNoCapacity, code: ResourceExhausted},
		{name: "resource manager unknown device", err: resman.ErrUnknownDevice, code: Internal},
		{name: "resource manager invalid plan", err: resman.ErrInvalidPlan, code: Internal},
		{name: "resource manager duplicate key", err: resman.ErrDuplicateKey, code: Internal},
		{name: "resource manager no GPUs", err: resman.ErrNoGPUs, code: Internal},
		{name: "hugging face not found", err: hf.ErrNotFound, code: NotFound},
		{name: "hugging face throttled", err: hf.ErrThrottled, code: TooManyRequests},
		{name: "irrecoverable JSON", err: jsonrepair.ErrIrrecoverable, code: Internal},
		{name: "bucky pool server busy", err: buckypool.ErrServerBusy, code: Unavailable},
		{name: "ffmpeg not installed", err: buckyffmpeg.ErrNotInstalled, code: Internal},
		{name: "llama libraries read only", err: libs.ErrReadOnly, code: Internal},
		{name: "bucky libraries read only", err: buckylibs.ErrReadOnly, code: Internal},
		{name: "github rate limited", err: github.ErrRateLimited, code: TooManyRequests},
		{name: "unknown", err: errors.New("unknown"), code: Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", tt.err)
			got := FromSDK(err)

			if !got.Code.Equal(tt.code) {
				t.Errorf("Code: got %s, want %s", got.Code, tt.code)
			}
			if got.Message != err.Error() {
				t.Errorf("Message: got %q, want %q", got.Message, err)
			}
			if !strings.Contains(got.FuncName, "TestFromSDK") {
				t.Errorf("FuncName: got %q, want TestFromSDK caller", got.FuncName)
			}
			if !strings.Contains(got.FileName, "errs_test.go:") {
				t.Errorf("FileName: got %q, want errs_test.go caller", got.FileName)
			}
		})
	}
}
