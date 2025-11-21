package kronk

import (
	"context"
	"fmt"
	"sync/atomic"
)

type nonStreamingFunc[T any] func(llama *model) (T, error)
type streamingFunc[T any] func(llama *model) <-chan T

func nonStreaming[T any](ctx context.Context, krn *Kronk, closed *uint32, f nonStreamingFunc[T]) (T, error) {
	var zero T

	if atomic.LoadUint32(closed) == 1 {
		return zero, fmt.Errorf("Kronk has been unloaded")
	}

	select {
	case <-ctx.Done():
		return zero, ctx.Err()

	case llama, ok := <-krn.models:
		if !ok {
			return zero, fmt.Errorf("Kronk has been unloaded")
		}

		krn.wg.Add(1)
		defer func() {
			krn.models <- llama
			krn.wg.Done()
		}()

		return f(llama)
	}
}

func streaming[T any](ctx context.Context, krn *Kronk, closed *uint32, f streamingFunc[T]) (<-chan T, error) {
	var zero chan T

	if atomic.LoadUint32(closed) == 1 {
		return zero, fmt.Errorf("Kronk has been unloaded")
	}

	ch := make(chan T)

	select {
	case <-ctx.Done():
		return zero, ctx.Err()

	case llama, ok := <-krn.models:
		if !ok {
			return zero, fmt.Errorf("Kronk has been unloaded")
		}

		krn.wg.Add(1)
		go func() {
			defer func() {
				close(ch)
				krn.models <- llama
				krn.wg.Done()
			}()

			lch := f(llama)
			for msg := range lch {
				ch <- msg
			}
		}()
	}

	return ch, nil
}
