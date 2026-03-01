// Package github provides HTTP client support for GitHub API calls
// with authentication and rate limit tracking.
package github

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// ErrRateLimited is returned when GitHub responds with 403 or 429 due to
// rate limiting, or when a request is blocked because the tracked rate
// limit is already exhausted.
var ErrRateLimited = errors.New("github rate limited")

// RateLimit contains the current GitHub API rate limit state.
type RateLimit struct {
	Limit     int
	Remaining int
	Used      int
	Reset     time.Time
	Resource  string
}

// Client provides GitHub API access with authentication via the
// GITHUB_TOKEN environment variable and in-memory rate limit tracking.
type Client struct {
	mu        sync.Mutex
	rateLimit RateLimit
}

// New constructs a new GitHub client.
func New() *Client {
	return &Client{}
}

// RateLimitState returns the current rate limit state.
func (c *Client) RateLimitState() RateLimit {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.rateLimit
}

// Do executes the provided HTTP request, adding GitHub authentication
// from the GITHUB_TOKEN environment variable and tracking rate limit
// headers from the response. A 403 or 429 response is treated as rate
// limiting: the response body is closed and ErrRateLimited is returned
// so callers can degrade gracefully.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	rl := c.rateLimit
	c.mu.Unlock()

	if rl.Limit > 0 && rl.Remaining == 0 && time.Now().Before(rl.Reset) {
		return nil, fmt.Errorf("resets at %s: %w", rl.Reset.Format(time.RFC3339), ErrRateLimited)
	}

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	c.updateRateLimit(resp)

	switch resp.StatusCode {
	case http.StatusForbidden, http.StatusTooManyRequests:
		resp.Body.Close()
		return nil, fmt.Errorf("status %d: %w", resp.StatusCode, ErrRateLimited)
	}

	return resp, nil
}

// =============================================================================

func (c *Client) updateRateLimit(resp *http.Response) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if v := resp.Header.Get("x-ratelimit-limit"); v != "" {
		c.rateLimit.Limit, _ = strconv.Atoi(v)
	}

	if v := resp.Header.Get("x-ratelimit-remaining"); v != "" {
		c.rateLimit.Remaining, _ = strconv.Atoi(v)
	}

	if v := resp.Header.Get("x-ratelimit-used"); v != "" {
		c.rateLimit.Used, _ = strconv.Atoi(v)
	}

	if v := resp.Header.Get("x-ratelimit-reset"); v != "" {
		if epoch, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.rateLimit.Reset = time.Unix(epoch, 0)
		}
	}

	if v := resp.Header.Get("x-ratelimit-resource"); v != "" {
		c.rateLimit.Resource = v
	}
}
