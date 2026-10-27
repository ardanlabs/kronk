package model

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage"
	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage/ram"
)

// newSessionStore constructs a kvstorage.Store using the factory injected in cfg.
//
// SDK callers inject Config.SessionStoreFactory. A zero-value Config uses the
// built-in RAM factory.
func newSessionStore(ctx context.Context, cfg Config) (kvstorage.Store, error) {
	factory := cfg.SessionStoreFactory
	if factory == nil {
		factory = ram.NewFactory()
	}

	store, err := factory(ctx)
	if err != nil {
		return nil, fmt.Errorf("session-store: create: %w", err)
	}
	if store == nil {
		return nil, fmt.Errorf("session-store: create: factory returned a nil store")
	}

	return store, nil
}

// newSystemCacheStore constructs the RAM-only store used by immutable System
// preloads. Working sessions remain responsible for configured persistence.
func newSystemCacheStore(ctx context.Context) (kvstorage.Store, error) {
	store, err := ram.NewFactory()(ctx)
	if err != nil {
		return nil, fmt.Errorf("system-cache-store: create: %w", err)
	}
	if store == nil {
		return nil, fmt.Errorf("system-cache-store: create: factory returned a nil store")
	}

	return store, nil
}
