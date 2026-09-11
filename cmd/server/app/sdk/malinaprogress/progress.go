// Package malinaprogress provides support for broadcasting process-global
// Malina progress to model-server clients.
package malinaprogress

import "sync"

// Update describes process-global model loading or image generation progress.
type Update struct {
	Step           int
	Steps          int
	SecondsPerStep float32
}

// Broker broadcasts Malina progress without blocking the native callback.
type Broker struct {
	mu          sync.Mutex
	subscribers map[chan Update]struct{}
}

// New constructs a Broker.
func New() *Broker {
	b := Broker{
		subscribers: make(map[chan Update]struct{}),
	}

	return &b
}

// Publish broadcasts the latest process-global Malina progress. A slow
// subscriber loses intermediate updates rather than blocking image generation.
func (b *Broker) Publish(step int, steps int, secondsPerStep float32) {
	update := Update{
		Step:           step,
		Steps:          steps,
		SecondsPerStep: secondsPerStep,
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for subscriber := range b.subscribers {
		select {
		case subscriber <- update:
			continue
		default:
		}

		select {
		case <-subscriber:
		default:
		}
		select {
		case subscriber <- update:
		default:
		}
	}
}

// Subscribe registers a process-global Malina progress subscriber. The caller
// must call unsubscribe when it no longer consumes updates.
func (b *Broker) Subscribe() (<-chan Update, func()) {
	subscriber := make(chan Update, 1)

	b.mu.Lock()
	b.subscribers[subscriber] = struct{}{}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		delete(b.subscribers, subscriber)
		b.mu.Unlock()
	}

	return subscriber, unsubscribe
}
