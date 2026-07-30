package rate_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/rate"
)

func Test_Rate(t *testing.T) {
	limiter, err := rate.New(rate.Config{
		DBPath: t.TempDir(),
	})

	if err != nil {
		t.Fatalf("should be able to construct rate limiter: %s", err)
	}

	defer limiter.Close()

	t.Run("unlimited", unlimited(limiter))
	t.Run("day", day(limiter))
	t.Run("month", month(limiter))
	t.Run("year", year(limiter))
}

func Test_RateConcurrentAdmission(t *testing.T) {
	limiter, err := rate.New(rate.Config{
		DBPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("should be able to construct rate limiter: %s", err)
	}
	defer limiter.Close()

	const (
		calls = 100
		max   = 10
	)

	limit := auth.RateLimit{Limit: max, Window: auth.RateDay}
	var successes atomic.Int64
	var exceeded atomic.Int64
	errCh := make(chan error, calls)
	start := make(chan struct{})

	var wg sync.WaitGroup
	for range calls {
		wg.Go(func() {
			<-start
			err := limiter.Check("concurrent-user", "endpoint", limit)
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, rate.ErrRateLimitExceeded):
				exceeded.Add(1)
			default:
				errCh <- err
			}
		})
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Check: got internal error %v, want ErrRateLimitExceeded", err)
	}
	if got := successes.Load(); got != max {
		t.Errorf("successful calls: got %d, want %d", got, max)
	}
	if got := exceeded.Load(); got != calls-max {
		t.Errorf("rate-limited calls: got %d, want %d", got, calls-max)
	}
}

func unlimited(limiter *rate.Limiter) func(t *testing.T) {
	return func(t *testing.T) {
		limit := auth.RateLimit{
			Limit:  0,
			Window: auth.RateUnlimited,
		}

		for range 100 {
			if err := limiter.Check("user-unlimited", "endpoint", limit); err != nil {
				t.Fatalf("should never exceed unlimited: %s", err)
			}
		}
	}
}

func day(limiter *rate.Limiter) func(t *testing.T) {
	return func(t *testing.T) {
		limit := auth.RateLimit{
			Limit:  3,
			Window: auth.RateDay,
		}

		for i := range 3 {
			if err := limiter.Check("user-day", "endpoint", limit); err != nil {
				t.Fatalf("should not exceed limit on check %d: %s", i+1, err)
			}
		}

		err := limiter.Check("user-day", "endpoint", limit)
		switch {
		case err == nil:
			t.Fatal("should exceed limit after 3 requests")

		case !errors.Is(err, rate.ErrRateLimitExceeded):
			t.Fatalf("should return ErrRateLimitExceeded: %s", err)
		}
	}
}

func month(limiter *rate.Limiter) func(t *testing.T) {
	return func(t *testing.T) {
		limit := auth.RateLimit{
			Limit:  2,
			Window: auth.RateMonth,
		}

		for i := range 2 {
			if err := limiter.Check("user-month", "endpoint", limit); err != nil {
				t.Fatalf("should not exceed limit on check %d: %s", i+1, err)
			}
		}

		err := limiter.Check("user-month", "endpoint", limit)
		switch {
		case err == nil:
			t.Fatal("should exceed limit after 2 requests")

		case !errors.Is(err, rate.ErrRateLimitExceeded):
			t.Fatalf("should return ErrRateLimitExceeded: %s", err)
		}
	}
}

func year(limiter *rate.Limiter) func(t *testing.T) {
	return func(t *testing.T) {
		limit := auth.RateLimit{
			Limit:  5,
			Window: auth.RateYear,
		}

		for i := range 5 {
			if err := limiter.Check("user-year", "endpoint", limit); err != nil {
				t.Fatalf("should not exceed limit on check %d: %s", i+1, err)
			}
		}

		err := limiter.Check("user-year", "endpoint", limit)
		switch {
		case err == nil:
			t.Fatal("should exceed limit after 5 requests")

		case !errors.Is(err, rate.ErrRateLimitExceeded):
			t.Fatalf("should return ErrRateLimitExceeded: %s", err)
		}
	}
}
