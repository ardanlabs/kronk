package kronk

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

type nonStreamingFunc[T any] func(llama *model.Model) (T, error)

func nonStreaming[T any](ctx context.Context, krn *Kronk, f nonStreamingFunc[T]) (T, error) {
	var zero T

	llama, err := krn.acquireModel(ctx)
	if err != nil {
		return zero, err
	}
	defer krn.releaseModel(llama)

	return f(llama)
}

// =============================================================================

type streamingFunc[T any] func(llama *model.Model) <-chan T
type errorFunc[T any] func(err error) T

func streaming[T any](ctx context.Context, krn *Kronk, f streamingFunc[T], ef errorFunc[T]) (<-chan T, error) {
	llama, err := krn.acquireModel(ctx)
	if err != nil {
		return nil, err
	}

	ch := make(chan T)

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				sendError(ctx, ch, ef, rec)
			}

			close(ch)
			krn.releaseModel(llama)
		}()

		lch := f(llama)

		for msg := range lch {
			if err := sendMessage(ctx, ch, msg); err != nil {
				break
			}
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

func sendError[T any](ctx context.Context, ch chan T, ef errorFunc[T], rec any) {
	select {
	case <-ctx.Done():
	case ch <- ef(fmt.Errorf("%v", rec)):
	default:
	}
}

// =============================================================================

type streamProcessor[T, U any] struct {
	Start    func() []U
	Process  func(T) []U
	Complete func(T) []U
}

func streamingWith[T, U any](ctx context.Context, krn *Kronk, f streamingFunc[T], p streamProcessor[T, U], ef errorFunc[U]) (<-chan U, error) {
	llama, err := krn.acquireModel(ctx)
	if err != nil {
		return nil, err
	}

	ch := make(chan U)

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				sendError(ctx, ch, ef, rec)
			}

			close(ch)
			krn.releaseModel(llama)
		}()

		for _, msg := range p.Start() {
			if err := sendMessage(ctx, ch, msg); err != nil {
				return
			}
		}

		lch := f(llama)

		var lastChunk T
		for chunk := range lch {
			lastChunk = chunk
			for _, msg := range p.Process(chunk) {
				if err := sendMessage(ctx, ch, msg); err != nil {
					return
				}
			}
		}

		for _, msg := range p.Complete(lastChunk) {
			if err := sendMessage(ctx, ch, msg); err != nil {
				return
			}
		}
	}()

	return ch, nil
}
