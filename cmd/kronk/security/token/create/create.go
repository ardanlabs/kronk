// Package create provides the token create command code.
package create

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/kronk/security/sec"
	"github.com/ardanlabs/kronk/sdk/security/auth"
)

type config struct {
	AdminToken string
	Endpoints  map[string]auth.RateLimit
	Duration   time.Duration
}

func runWeb(cfg config) error {
	fmt.Println("Token create")
	fmt.Printf("  Duration: %s\n", cfg.Duration)
	fmt.Printf("  Endpoints: %v\n", cfg.Endpoints)

	url, err := client.DefaultURL("/v1/security/token/create")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	req := client.D{
		"admin":     false,
		"endpoints": cfg.Endpoints,
		"duration":  cfg.Duration,
	}

	cln := client.New(
		client.FmtLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var resp struct {
		Token string `json:"token"`
	}
	if err := cln.Do(ctx, http.MethodPost, url, req, &resp); err != nil {
		return fmt.Errorf("do: unable to create token: %w", err)
	}

	fmt.Println("TOKEN:")
	fmt.Println(resp.Token)

	return nil
}

func runLocal(cfg config) error {
	fmt.Println("Token create")
	fmt.Printf("  Duration: %s\n", cfg.Duration)
	fmt.Printf("  Endpoints: %v\n", cfg.Endpoints)

	token, err := sec.Security.GenerateToken(false, cfg.Endpoints, cfg.Duration)
	if err != nil {
		return fmt.Errorf("generate-token: %w", err)
	}

	fmt.Println("TOKEN:")
	fmt.Println(token)

	return nil
}

// =============================================================================

// parseEndpoints parses endpoint specifications in the format:
// "endpoint:limit/window" or "endpoint" (defaults to unlimited)
// Examples:
//   - "chat-completions" -> unlimited
//   - "chat-completions:1000/day" -> 1000 requests per day
//   - "embeddings:unlimited" -> unlimited
func parseEndpoints(specs []string) (map[string]auth.RateLimit, error) {
	result := make(map[string]auth.RateLimit)

	for _, spec := range specs {
		name, rateLimit, err := parseEndpointSpec(spec)
		if err != nil {
			return nil, fmt.Errorf("invalid endpoint spec %q: %w", spec, err)
		}

		result[name] = rateLimit
	}

	return result, nil
}

func parseEndpointSpec(spec string) (string, auth.RateLimit, error) {
	parts := strings.SplitN(spec, ":", 2)
	name := strings.TrimSpace(parts[0])

	if name == "" {
		return "", auth.RateLimit{}, fmt.Errorf("empty endpoint name")
	}

	if len(parts) == 1 {
		return name, auth.RateLimit{Limit: 0, Window: auth.RateUnlimited}, nil
	}

	limitSpec := strings.TrimSpace(parts[1])

	if limitSpec == "unlimited" {
		return name, auth.RateLimit{Limit: 0, Window: auth.RateUnlimited}, nil
	}

	limitParts := strings.SplitN(limitSpec, "/", 2)
	if len(limitParts) != 2 {
		return "", auth.RateLimit{}, fmt.Errorf("expected format limit/window (e.g., 1000/day)")
	}

	limit, err := strconv.Atoi(strings.TrimSpace(limitParts[0]))
	if err != nil {
		return "", auth.RateLimit{}, fmt.Errorf("invalid limit: %w", err)
	}

	window, err := parseWindow(strings.TrimSpace(limitParts[1]))
	if err != nil {
		return "", auth.RateLimit{}, err
	}

	return name, auth.RateLimit{Limit: limit, Window: window}, nil
}

func parseWindow(s string) (auth.RateWindow, error) {
	switch strings.ToLower(s) {
	case "day":
		return auth.RateDay, nil

	case "month":
		return auth.RateMonth, nil

	case "year":
		return auth.RateYear, nil

	case "unlimited":
		return auth.RateUnlimited, nil

	default:
		return "", fmt.Errorf("invalid window %q: must be day, month, year, or unlimited", s)
	}
}
