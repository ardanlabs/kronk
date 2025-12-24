package auth

import "github.com/golang-jwt/jwt/v4"

// RateWindow represents the time period for rate limiting.
type RateWindow string

// Set of rate limit units.
const (
	RateDay       RateWindow = "day"
	RateMonth     RateWindow = "month"
	RateYear      RateWindow = "year"
	RateUnlimited RateWindow = "unlimited"
)

// RateLimit defines the rate limit configuration for an endpoint.
type RateLimit struct {
	Limit  int        `json:"limit"`
	Window RateWindow `json:"window"`
}

// Claims represents the authorization claims transmitted via a JWT.
type Claims struct {
	jwt.RegisteredClaims
	Admin     bool                 `json:"admin"`
	Endpoints map[string]RateLimit `json:"endpoints"`
}
