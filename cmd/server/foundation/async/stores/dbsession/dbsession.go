// Package dbsession provides support for session support.
package dbsession

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/maypok86/otter/v2"
)

type Store struct {
	db          *otter.Cache[string, SessionData]
	callTimeout time.Duration
}

func NewStore(callTimeout time.Duration, timeToLive time.Duration) (*Store, error) {
	opt := otter.Options[string, SessionData]{
		MaximumSize:      100,
		ExpiryCalculator: otter.ExpiryWriting[string, SessionData](timeToLive),
	}

	cache, err := otter.New(&opt)
	if err != nil {
		return nil, fmt.Errorf("constructing cache: %w", err)
	}

	s := Store{
		db:          cache,
		callTimeout: callTimeout,
	}

	return &s, nil
}

func (s *Store) NewSession() (SessionData, error) {
	data := SessionData{
		SessionID:     uuid.New(),
		Status:        New,
		Result:        nil,
		StartedDate:   time.Now(),
		CompletedDate: time.Time{},
	}

	s.db.Set(data.SessionID.String(), data)

	return data, nil
}

func (s *Store) GetSession(ctx context.Context, sessionID uuid.UUID) (SessionData, error) {
	data, found := s.db.GetIfPresent(sessionID.String())
	if !found {
		return SessionData{}, fmt.Errorf("failed to get session from db")
	}

	return data, nil
}

func (s *Store) UpdateSessionStatus(sessionID uuid.UUID, status Status, result []byte) (SessionData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.callTimeout)
	defer cancel()

	data, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return SessionData{}, err
	}

	data.Status = status
	if status == Completed || status == Error {
		data.CompletedDate = time.Now()
	}

	data.Result = result

	s.db.Set(sessionID.String(), data)

	return data, nil
}
