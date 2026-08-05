package model

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage"
	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage/ram"
)

// SessionStore is the storage contract used for an externalized IMC session.
type SessionStore = kvstorage.Store

// SessionStoreFactory constructs an independent SessionStore.
type SessionStoreFactory = kvstorage.Factory

// newSessionStore constructs a SessionStore using the factory injected in cfg.
//
// SDK callers inject Config.SessionStoreFactory. A zero-value Config uses the
// built-in RAM factory.
func newSessionStore(cfg Config) (SessionStore, error) {
	factory := cfg.SessionStoreFactory
	if factory == nil {
		factory = ram.NewFactory()
	}

	store, err := factory()
	if err != nil {
		return nil, fmt.Errorf("session-store: create: %w", err)
	}
	if store == nil {
		return nil, fmt.Errorf("session-store: create: factory returned a nil store")
	}

	return store, nil
}
