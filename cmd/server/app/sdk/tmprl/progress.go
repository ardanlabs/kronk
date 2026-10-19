package tmprl

import (
	"context"
	"sync"
	"time"

	"go.temporal.io/sdk/activity"
)

// ProgressBroker broadcasts process-local updates without blocking activities.
// It keeps only the latest update for slow consumers; durable state remains in
// Temporal. The zero value is ready to use.
type ProgressBroker[K comparable, T any] struct {
	mu          sync.Mutex
	subscribers map[K]map[chan T]struct{}
}

// Subscribe registers a consumer. The returned unsubscribe closes the channel
// and is safe to call more than once.
func (b *ProgressBroker[K, T]) Subscribe(key K) (<-chan T, func()) {
	ch := make(chan T, 1)
	b.mu.Lock()
	if b.subscribers == nil {
		b.subscribers = make(map[K]map[chan T]struct{})
	}
	if b.subscribers[key] == nil {
		b.subscribers[key] = make(map[chan T]struct{})
	}
	b.subscribers[key][ch] = struct{}{}
	b.mu.Unlock()
	return ch, sync.OnceFunc(func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subscribers[key], ch)
		if len(b.subscribers[key]) == 0 {
			delete(b.subscribers, key)
		}
		close(ch)
	})
}

// Publish replaces an unread update rather than waiting for a slow subscriber.
func (b *ProgressBroker[K, T]) Publish(key K, update T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subscribers[key] {
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

// Heartbeat keeps an activity responsive to cancellation even without fresh
// progress. Call stop on every exit; it joins the heartbeat goroutine. Payloads
// must be safe to copy by value, as they are serialized concurrently.
func Heartbeat[T any](ctx context.Context, initial T, publish func(T)) (report func(T), stop func()) {
	var mu sync.Mutex
	latest := initial
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
	return func(update T) {
		mu.Lock()
		latest = update
		mu.Unlock()
		publish(update)
	}, func() { cancel(); <-done }
}
