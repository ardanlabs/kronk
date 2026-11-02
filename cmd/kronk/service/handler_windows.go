package service

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sys/windows/svc"
)

type handler struct {
	run func(context.Context, func()) error
	err error
}

func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := svc.Status{State: svc.StartPending, Accepts: svc.AcceptStop | svc.AcceptShutdown, CheckPoint: 1, WaitHint: 30000}
	statuses <- state
	ready := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- h.run(ctx, func() {
			select {
			case ready <- struct{}{}:
			default:
			}
		})
	}()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	// Do not advertise progress forever if native initialization or cleanup hangs.
	pendingDeadline := time.Now().Add(5 * time.Minute)
	for {
		select {
		case err := <-done:
			if ctx.Err() != nil && errors.Is(err, context.Canceled) {
				err = nil
			}
			h.err = err
			if err != nil {
				return true, 1
			}
			return false, 0
		case <-ready:
			if state.State == svc.StartPending {
				state = svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
				statuses <- state
			}
		case request, ok := <-requests:
			if !ok {
				requests = nil
				state = svc.Status{State: svc.StopPending, CheckPoint: 1, WaitHint: 30000}
				pendingDeadline = time.Now().Add(5 * time.Minute)
				cancel()
			} else {
				switch request.Cmd {
				case svc.Stop, svc.Shutdown:
					if state.State != svc.StopPending {
						state = svc.Status{State: svc.StopPending, CheckPoint: 1, WaitHint: 30000}
						pendingDeadline = time.Now().Add(5 * time.Minute)
						cancel()
					}
				case svc.Interrogate:
				}
			}
			statuses <- state
		case <-ticker.C:
			if (state.State == svc.StartPending || state.State == svc.StopPending) && time.Now().Before(pendingDeadline) {
				state.CheckPoint++
				statuses <- state
			}
		}
	}
}
