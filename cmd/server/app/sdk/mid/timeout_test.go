package mid

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

func TestTimeout(t *testing.T) {
	parent := t.Context()

	request := httptest.NewRequest("GET", "/", nil)
	var explicitCtx context.Context
	var requestCtx context.Context

	handler := Timeout(time.Hour)(func(ctx context.Context, r *http.Request) web.Encoder {
		explicitCtx = ctx
		requestCtx = r.Context()
		return web.NewNoResponse()
	})

	handler(parent, request)

	if explicitCtx != requestCtx {
		t.Fatal("handler contexts differ")
	}
	if _, ok := explicitCtx.Deadline(); !ok {
		t.Fatal("explicit context has no deadline")
	}
	select {
	case <-explicitCtx.Done():
	default:
		t.Fatal("explicit context was not cancelled after handler returned")
	}
	select {
	case <-requestCtx.Done():
	default:
		t.Fatal("request context was not cancelled after handler returned")
	}
}

func TestTimeoutExpires(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	var explicitErr error
	var requestErr error

	handler := Timeout(time.Millisecond)(func(ctx context.Context, r *http.Request) web.Encoder {
		<-ctx.Done()
		explicitErr = ctx.Err()
		requestErr = r.Context().Err()
		return web.NewNoResponse()
	})

	handler(t.Context(), request)

	if !errors.Is(explicitErr, context.DeadlineExceeded) {
		t.Fatalf("explicit context error: got %v, want %v", explicitErr, context.DeadlineExceeded)
	}
	if !errors.Is(requestErr, context.DeadlineExceeded) {
		t.Fatalf("request context error: got %v, want %v", requestErr, context.DeadlineExceeded)
	}
}

func TestTimeoutParentDeadlineWins(t *testing.T) {
	parentDeadline := time.Now().Add(time.Minute)
	parent, cancel := context.WithDeadline(context.Background(), parentDeadline)
	defer cancel()

	request := httptest.NewRequest("GET", "/", nil)
	var gotDeadline time.Time
	handler := Timeout(time.Hour)(func(ctx context.Context, r *http.Request) web.Encoder {
		var ok bool
		gotDeadline, ok = ctx.Deadline()
		if !ok {
			t.Fatal("explicit context has no deadline")
		}
		requestDeadline, ok := r.Context().Deadline()
		if !ok {
			t.Fatal("request context has no deadline")
		}
		if !requestDeadline.Equal(gotDeadline) {
			t.Fatalf("request deadline: got %v, want %v", requestDeadline, gotDeadline)
		}
		return web.NewNoResponse()
	})

	handler(parent, request)

	if !gotDeadline.Equal(parentDeadline) {
		t.Fatalf("deadline: got %v, want %v", gotDeadline, parentDeadline)
	}
}
