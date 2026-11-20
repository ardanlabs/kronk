package moxierun

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets"
	datasetstore "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
)

func TestMoxieOnDemandRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		files, err := datasetstore.New(datasetstore.Config{Root: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		services := Services{Datasets: datasets.New(datasets.Config{Files: files})}
		unavailable := errors.New("dependency unavailable")
		release := make(chan struct{})
		starts, stops := 0, 0
		var lifetime context.Context
		m := newRuntime(t.Context(), func(ctx, serviceCtx context.Context) (Services, func(), error) {
			starts++
			if starts == 1 {
				<-release
				return Services{}, func() {}, unavailable
			}
			lifetime = serviceCtx
			return services, func() { stops++ }, nil
		})
		defer m.Close()
		if starts != 0 {
			t.Fatal("constructing runtime contacted a dependency")
		}
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				if _, err := m.Resolve(t.Context()); !errors.Is(err, unavailable) {
					t.Errorf("unavailable dependency: got %v, want %v", err, unavailable)
				}
			})
		}
		synctest.Wait()
		close(release)
		wg.Wait()
		if starts != 1 {
			t.Fatalf("concurrent initialization: got %d starts, want one shared attempt", starts)
		}
		requestCtx, cancel := context.WithCancel(t.Context())
		if got, err := m.Resolve(requestCtx); err != nil || got != services {
			t.Fatalf("dependency restored: got %+v, %v, want %+v", got, err, services)
		}
		cancel()
		if lifetime.Err() != nil {
			t.Fatal("request cancellation stopped the service")
		}
		if got, err := m.Resolve(t.Context()); err != nil || got != services {
			t.Fatalf("subsequent request: got %+v, %v, want %+v", got, err, services)
		}
		if starts != 2 {
			t.Fatalf("established service was recreated: got %d starts, want 2", starts)
		}
		m.Close()
		if lifetime.Err() == nil || stops != 1 {
			t.Fatalf("shutdown: got lifetime error %v and %d cleanups, want canceled and one cleanup", lifetime.Err(), stops)
		}
		if _, err := m.Resolve(t.Context()); !errors.Is(err, context.Canceled) {
			t.Fatalf("request after shutdown: got %v, want canceled", err)
		}
	})
}

func TestMoxieShutdownDuringInitialization(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stopped := false
		m := newRuntime(t.Context(), func(ctx, serviceCtx context.Context) (Services, func(), error) {
			<-ctx.Done()
			// Exercise shutdown racing with an initializer that just succeeded.
			return Services{}, func() { stopped = true }, nil
		})
		done := make(chan error, 1)
		go func() { _, err := m.Resolve(t.Context()); done <- err }()
		synctest.Wait()
		m.Close()
		if err := <-done; !errors.Is(err, context.Canceled) || !stopped {
			t.Fatalf("shutdown during initialization: got error %v, cleanup %t, want canceled and cleaned up", err, stopped)
		}
	})
}
