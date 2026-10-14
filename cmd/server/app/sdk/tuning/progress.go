package tuning

import (
	"context"
	"sync"
	"time"

	"go.temporal.io/sdk/activity"
)

// Progress is a lossy notification, not durable run state. Like malinaprogress,
// slow consumers never block backend execution. Get/Wait remain authoritative.
type progressBroker struct {
	mu          sync.Mutex
	subscribers map[Execution]map[chan Progress]struct{}
}

// Subscribe receives local worker notifications. For a remote worker, poll its
// business API for heartbeat progress. Unsubscribe closes the channel and is
// safe to call more than once.
func (b *progressBroker) Subscribe(execution Execution) (<-chan Progress, func()) {
	ch := make(chan Progress, 1)
	b.mu.Lock()
	if b.subscribers == nil {
		b.subscribers = make(map[Execution]map[chan Progress]struct{})
	}
	if b.subscribers[execution] == nil {
		b.subscribers[execution] = make(map[chan Progress]struct{})
	}
	b.subscribers[execution][ch] = struct{}{}
	b.mu.Unlock()
	unsubscribe := sync.OnceFunc(func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subscribers[execution], ch)
		if len(b.subscribers[execution]) == 0 {
			delete(b.subscribers, execution)
		}
		close(ch)
	})
	return ch, unsubscribe
}

func (b *progressBroker) Publish(update Progress) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subscribers[update.Execution] {
		select {
		case ch <- update:
			continue
		default:
		}
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- update:
		default:
		}
	}
}

// activityProgress heartbeats even without fresh progress, so Temporal can
// deliver cancellation to long-running computation. Stop joins the goroutine.
func activityProgress(ctx context.Context, execution Execution, stage Stage, publish func(Progress)) (func(Progress), func()) {
	var mu sync.Mutex
	latest := Progress{Execution: execution, Stage: stage}
	activity.RecordHeartbeat(ctx, latest)
	heartbeatCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				mu.Lock()
				value := latest
				mu.Unlock()
				activity.RecordHeartbeat(ctx, value)
			}
		}
	}()
	return func(update Progress) {
		update.Execution = execution
		update.Stage = stage
		mu.Lock()
		latest = update
		mu.Unlock()
		publish(update)
	}, func() { cancel(); <-done }
}
