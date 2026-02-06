package kronk

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

type nonStreamingFunc[T any] func(llama *model.Model) (T, error)

func nonStreaming[T any](ctx context.Context, krn *Kronk, f nonStreamingFunc[T]) (T, error) {
	var zero T

	mdl, err := krn.acquireModel(ctx)
	if err != nil {
		return zero, err
	}
	defer krn.releaseModel()

	return f(mdl)
}

// =============================================================================

type streamingFunc[T any] func(llama *model.Model) <-chan T
type errorFunc[T any] func(err error) T

func streaming[T any](ctx context.Context, krn *Kronk, f streamingFunc[T], ef errorFunc[T]) (<-chan T, error) {
	mdl, err := krn.acquireModel(ctx)
	if err != nil {
		return nil, err
	}

	ch := make(chan T, 1)

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				sendError(ch, ef, rec)
			}

			close(ch)
			krn.releaseModel()
		}()

		lch := f(mdl)

		var cancelled bool
		for msg := range lch {
			if err := sendMessage(ctx, ch, msg); err != nil {
				cancelled = true
				break
			}
		}

		if cancelled {
			sendError(ch, ef, ctx.Err())
		}
	}()

	return ch, nil
}

func sendMessage[T any](ctx context.Context, ch chan T, msg T) error {
	// I want to try and send this message before we check the context.
	// Remember the user code might not be trying to receive on this
	// channel anymore.
	select {
	case ch <- msg:
		return nil
	default:
	}

	// Now randonly wait for the channel to be ready or the context to be done.
	select {
	case <-ctx.Done():
		return ctx.Err()

	case ch <- msg:
		return nil
	}
}

func sendError[T any](ch chan T, ef errorFunc[T], rec any) {
	select {
	case ch <- ef(fmt.Errorf("%v", rec)):
	case <-time.After(100 * time.Millisecond):
	}
}

// =============================================================================

type streamProcessor[T, U any] struct {
	Start    func() []U
	Process  func(T) []U
	Complete func(T) []U
}

func streamingWith[T, U any](ctx context.Context, krn *Kronk, f streamingFunc[T], p streamProcessor[T, U], ef errorFunc[U]) (<-chan U, error) {
	mdl, err := krn.acquireModel(ctx)
	if err != nil {
		return nil, err
	}

	ch := make(chan U, 1)

	go func() {
		var cancelled bool

		defer func() {
			if rec := recover(); rec != nil {
				sendError(ch, ef, rec)
			}

			if cancelled {
				sendError(ch, ef, ctx.Err())
			}

			close(ch)
			krn.releaseModel()
		}()

		for _, msg := range p.Start() {
			if err := sendMessage(ctx, ch, msg); err != nil {
				cancelled = true
				return
			}
		}

		lch := f(mdl)

		var lastChunk T
		for chunk := range lch {
			lastChunk = chunk
			for _, msg := range p.Process(chunk) {
				if err := sendMessage(ctx, ch, msg); err != nil {
					cancelled = true
					return
				}
			}
		}

		for _, msg := range p.Complete(lastChunk) {
			if err := sendMessage(ctx, ch, msg); err != nil {
				cancelled = true
				return
			}
		}
	}()

	return ch, nil
}
