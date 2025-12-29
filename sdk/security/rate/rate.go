// Package rate provides rate limiting support using an embedded database.
package rate

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/security/auth"
	"github.com/dgraph-io/badger/v4"
)

// ErrRateLimitExceeded is returned when the rate limit has been exceeded.
var ErrRateLimitExceeded = errors.New("rate limit exceeded")

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
		return nil, fmt.Errorf("opening badger db: %w", err)
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

	key := l.buildKey(subject, endpoint, limit.Window)

	var count int
	err := l.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			count = 0
			return nil
		}

		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			count = int(binary.BigEndian.Uint64(val))
			return nil
		})
	})

	if err != nil {
		return fmt.Errorf("reading rate limit: %w", err)
	}

	if count >= limit.Limit {
		return ErrRateLimitExceeded
	}

	return l.record(key, limit.Window)
}

func (l *Limiter) record(key []byte, window auth.RateWindow) error {
	ttl := l.calculateTTL(window)

	f := func(txn *badger.Txn) error {
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

		count++

		val := make([]byte, 8)
		binary.BigEndian.PutUint64(val, count)

		entry := badger.NewEntry(key, val).WithTTL(ttl)
		return txn.SetEntry(entry)
	}

	if err := l.db.Update(f); err != nil {
		return fmt.Errorf("recording rate limit: %w", err)
	}

	return nil
}

func (l *Limiter) buildKey(subject, endpoint string, window auth.RateWindow) []byte {
	windowStart := l.windowStart(window)
	return fmt.Appendf(nil, "rate:%s:%s:%d", subject, endpoint, windowStart.Unix())
}

func (l *Limiter) windowStart(window auth.RateWindow) time.Time {
	now := time.Now().UTC()

	switch window {
	case auth.RateDay:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	case auth.RateMonth:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	case auth.RateYear:
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)

	default:
		return now
	}
}

func (l *Limiter) calculateTTL(window auth.RateWindow) time.Duration {
	now := time.Now().UTC()
	var end time.Time

	switch window {
	case auth.RateDay:
		end = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)

	case auth.RateMonth:
		end = time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)

	case auth.RateYear:
		end = time.Date(now.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC)

	default:
		return 24 * time.Hour
	}

	return end.Sub(now)
}
