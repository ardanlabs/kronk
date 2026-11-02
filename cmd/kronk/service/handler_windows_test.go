package service

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/sys/windows/svc"
)

func TestHandlerStopWaitsForCleanup(t *testing.T) {
	for _, test := range []struct {
		name        string
		beforeReady bool
		control     svc.Cmd
	}{
		{"stop during startup", true, svc.Stop},
		{"stop while running", false, svc.Stop},
		{"shutdown while running", false, svc.Shutdown},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := make(chan svc.ChangeRequest, 1)
			statuses := make(chan svc.Status, 10)
			cleanup := make(chan struct{})
			defer close(cleanup)
			canceled := make(chan struct{})
			h := handler{run: func(ctx context.Context, ready func()) error {
				if !test.beforeReady {
					ready()
				}
				<-ctx.Done()
				// A late native initialization callback must not resurrect the service.
				ready()
				close(canceled)
				<-cleanup
				return ctx.Err()
			}}
			done := make(chan uint32, 1)
			go func() { _, code := h.Execute(nil, requests, statuses); done <- code }()
			readState(t, statuses, svc.StartPending)
			if !test.beforeReady {
				readState(t, statuses, svc.Running)
			}
			requests <- svc.ChangeRequest{Cmd: test.control}
			readState(t, statuses, svc.StopPending)
			select {
			case <-canceled:
			case <-time.After(2 * time.Second):
				t.Fatal("server context was not canceled")
			}
			requests <- svc.ChangeRequest{Cmd: svc.Interrogate}
			readState(t, statuses, svc.StopPending)
			select {
			case <-done:
				t.Fatal("service exited before cleanup completed")
			default:
			}
			cleanup <- struct{}{}
			select {
			case code := <-done:
				if code != 0 || h.err != nil {
					t.Fatalf("stop: got code=%d error=%v, want successful shutdown", code, h.err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("service did not exit after cleanup")
			}
		})
	}
}

func TestHandlerReportsFailure(t *testing.T) {
	for _, ready := range []bool{false, true} {
		t.Run(map[bool]string{false: "startup", true: "running"}[ready], func(t *testing.T) {
			failure := errors.New("server failed")
			h := handler{run: func(_ context.Context, onReady func()) error {
				if ready {
					onReady()
				}
				return failure
			}}
			specific, code := h.Execute(nil, make(chan svc.ChangeRequest), make(chan svc.Status, 10))
			if !specific || code == 0 || !errors.Is(h.err, failure) {
				t.Fatalf("failure: got specific=%t code=%d error=%v, want service failure", specific, code, h.err)
			}
		})
	}
}

func TestHandlerPendingProgressIsBounded(t *testing.T) {
	for _, stopping := range []bool{false, true} {
		t.Run(map[bool]string{false: "startup", true: "shutdown"}[stopping], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				requests := make(chan svc.ChangeRequest, 1)
				statuses := make(chan svc.Status, 256)
				cleanup := make(chan struct{})
				h := handler{run: func(ctx context.Context, _ func()) error { <-cleanup; return ctx.Err() }}
				done := make(chan struct{})
				go func() { h.Execute(nil, requests, statuses); close(done) }()
				synctest.Wait()
				if stopping {
					requests <- svc.ChangeRequest{Cmd: svc.Stop}
					synctest.Wait()
				}
				time.Sleep(6 * time.Minute)
				count := len(statuses)
				time.Sleep(time.Minute)
				if len(statuses) != count {
					t.Error("hung service kept advertising progress after its pending deadline")
				}
				close(cleanup)
				<-done
			})
		})
	}
}

func readState(t *testing.T, statuses <-chan svc.Status, want svc.State) {
	t.Helper()
	select {
	case state := <-statuses:
		if state.State != want {
			t.Fatalf("state: got %d, want %d", state.State, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for state %d", want)
	}
}
