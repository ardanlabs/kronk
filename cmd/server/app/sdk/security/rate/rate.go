// Package rate provides rate limiting support using an embedded database.
package rate

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/dgraph-io/badger/v4"
)

// ErrRateLimitExceeded is returned when the rate limit has been exceeded.
var ErrRateLimitExceeded = errors.New("rate limit exceeded")

const conflictRetries = 32

// Config holds the configuration for the rate limiter.
type Config struct {
	DBPath string
}

// Limiter provides rate limiting using an embedded badger database.
type Limiter struct {
	db *badger.DB
}

// New creates a new rate limiter with the specified configuration.
func New(cfg Config) (*Limiter, error) {
	opts := badger.DefaultOptions(cfg.DBPath)
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("new: unable to open badger db: %w", err)
	}

	l := Limiter{
		db: db,
	}

	return &l, nil
}

// Close closes the underlying database.
func (l *Limiter) Close() error {
	return l.db.Close()
}

// Check validates that the rate limit has not been exceeded for the given
// subject and endpoint. If the limit has not been reached, the count is
// incremented. It returns ErrRateLimitExceeded if the limit has been reached,
// nil otherwise. Unlimited endpoints always return nil.
func (l *Limiter) Check(subject string, endpoint string, limit auth.RateLimit) error {
	if limit.Window == auth.RateUnlimited {
		return nil
	}

	windowStart, windowEnd := windowBounds(limit.Window, time.Now().UTC())
	key := fmt.Appendf(nil, "rate:%s:%s:%d", subject, endpoint, windowStart.Unix())
	expiresAt := uint64(windowEnd.Unix())

	update := func(txn *badger.Txn) error {
		var count uint64

		item, err := txn.Get(key)
		switch err {
		case nil:
			err = item.Value(func(val []byte) error {
				count = binary.BigEndian.Uint64(val)
				return nil
			})

			if err != nil {
				return err
			}

		default:
			if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}

		if limit.Limit <= 0 || count >= uint64(limit.Limit) {
			return ErrRateLimitExceeded
		}

		count++

		val := make([]byte, 8)
		binary.BigEndian.PutUint64(val, count)

		entry := badger.NewEntry(key, val)
		entry.ExpiresAt = expiresAt
		return txn.SetEntry(entry)
	}

	for range conflictRetries {
		err := l.db.Update(update)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, ErrRateLimitExceeded):
			return ErrRateLimitExceeded
		case !errors.Is(err, badger.ErrConflict):
			return fmt.Errorf("check: unable to update rate limit: %w", err)
		}
	}

	return fmt.Errorf("check: unable to update rate limit after %d conflicts: %w", conflictRetries, badger.ErrConflict)
}

func windowBounds(window auth.RateWindow, now time.Time) (time.Time, time.Time) {
	switch window {
	case auth.RateDay:
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 0, 1)

	case auth.RateMonth:
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0)

	case auth.RateYear:
		start := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(1, 0, 0)

	default:
		return now, now.Add(24 * time.Hour)
	}
}
