// Package client provides support to access an OpenAI-compatible API service.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const version = "v1.0.0"

// ErrUnauthorized is returned when the API refuses to authorize the request.
var ErrUnauthorized = errors.New("api understands the request but refuses to authorize it")

var defaultClient = http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 15 * time.Second,
			DualStack: true,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

// =============================================================================

// Logger represents a function for logging client operations.
type Logger func(context.Context, string, ...any)

// NoopLogger is a logger that discards all log output.
var NoopLogger = func(ctx context.Context, msg string, v ...any) {}

// FmtLogger is a logger that writes to stdout using fmt.
var FmtLogger = func(ctx context.Context, msg string, args ...any) {
	fmt.Print(msg)
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			fmt.Printf(" %v[%v]", args[i], args[i+1])
		}
	}
	fmt.Println()
}

// =============================================================================

// Client provides support for making HTTP requests to an API.
type Client struct {
	log    Logger
	http   *http.Client
	bearer string
}

// New constructs a Client with the provided logger and options.
func New(log Logger, options ...func(cln *Client)) *Client {
	cln := Client{
		log:  log,
		http: &defaultClient,
	}

	for _, option := range options {
		option(&cln)
	}

	return &cln
}

// WithClient sets a custom HTTP client for the Client.
func WithClient(http *http.Client) func(cln *Client) {
	return func(cln *Client) {
		cln.http = http
	}
}

// WithBearer sets a bearer token for API authentication.
func WithBearer(token string) func(cln *Client) {
	return func(cln *Client) {
		cln.bearer = token
	}
}

// Do sends an HTTP request and decodes the JSON response into v.
func (cln *Client) Do(ctx context.Context, method string, endpoint string, body D, v any) error {
	resp, err := do(ctx, cln, method, endpoint, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("client: copy error: %w", err)
	}

	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("client: response: %s, decoding error: %w ", string(data), err)
	}

	return nil
}

// =============================================================================

// SSEClient provides support for Server-Sent Events streaming responses.
type SSEClient[T any] struct {
	*Client
}

// NewSSE constructs an SSEClient with the provided logger and options.
func NewSSE[T any](log Logger, options ...func(cln *Client)) *SSEClient[T] {
	cln := New(log, options...)

	return &SSEClient[T]{
		Client: cln,
	}
}

// Do sends an HTTP request and streams SSE responses to the provided channel.
func (cln *SSEClient[T]) Do(ctx context.Context, method string, endpoint string, body D, ch chan T) error {
	resp, err := do(ctx, cln.Client, method, endpoint, body)
	if err != nil {
		return err
	}

	go func(ctx context.Context) {
		defer func() {
			resp.Body.Close()
			close(ch)
		}()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			if line == "" || line == "data: [DONE]" {
				continue
			}

			var v T
			if err := json.Unmarshal([]byte(line[6:]), &v); err != nil {
				cln.log(ctx, "do:", "Unmarshal", err, "line", line[6:])
				return
			}

			select {
			case ch <- v:

			case <-ctx.Done():
				cln.log(ctx, "do:", "Context", ctx.Err().Error())
				return
			}
		}

		if ctx.Err() != nil {
			cln.log(ctx, "do:", "Context", ctx.Err().Error())
		}
	}(ctx)

	return nil
}

// =============================================================================

func do(ctx context.Context, cln *Client, method string, endpoint string, body any) (*http.Response, error) {
	var statusCode int

	var b bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&b).Encode(body); err != nil {
			return nil, fmt.Errorf("encoding: error: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, &b)
	if err != nil {
		return nil, fmt.Errorf("create request error: %w", err)
	}

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("Ardan Labs AI Training Sample Go Client: %s", version))

	if cln.bearer != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cln.bearer))
	}

	resp, err := cln.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: error: %w", err)
	}

	// Assign for logging the status code at the end of the function call.
	statusCode = resp.StatusCode

	switch statusCode {
	case http.StatusOK, http.StatusNoContent:
		return resp, nil

	default:
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("readall: error: %w", err)
		}

		switch statusCode {
		case http.StatusForbidden:
			return nil, ErrUnauthorized

		default:
			var err Error
			if err := json.Unmarshal(data, &err); err != nil {
				return nil, fmt.Errorf("decoding: response: %s, error: %w ", string(data), err)
			}

			return nil, fmt.Errorf("error: status: %d response: %s", statusCode, err.Err.Message)
		}
	}
}
