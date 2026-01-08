package rate_test

import (
	"errors"
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
