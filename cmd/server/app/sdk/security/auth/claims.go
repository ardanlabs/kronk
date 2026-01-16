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
// The Limit field specifies the maximum number of requests allowed within the
// given Window period. A value of 0 means no requests are allowed. When Window
// is set to RateUnlimited, the Limit field is ignored and unlimited requests
// are permitted.
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
