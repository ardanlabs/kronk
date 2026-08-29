package auth

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// Set of known rate windows.
var rateWindows = make(map[string]RateWindow)

// RateDay identifies a daily rate-limit window.
var RateDay = newRateWindow("day")

// RateMonth identifies a monthly rate-limit window.
var RateMonth = newRateWindow("month")

// RateYear identifies a yearly rate-limit window.
var RateYear = newRateWindow("year")

// RateUnlimited disables rate limiting.
var RateUnlimited = newRateWindow("unlimited")

// =============================================================================

// RateWindow represents the time period for rate limiting.
type RateWindow struct {
	value string
}

func newRateWindow(value string) RateWindow {
	window := RateWindow{value: value}
	rateWindows[value] = window
	return window
}

// String returns the name of the rate window.
func (rw RateWindow) String() string {
	return rw.value
}

// Equal provides support for the go-cmp package and testing.
func (rw RateWindow) Equal(rw2 RateWindow) bool {
	return rw.value == rw2.value
}

// IsZero reports whether the rate window is unset.
func (rw RateWindow) IsZero() bool {
	return rw.value == ""
}

// MarshalText provides support for logging and serialization.
func (rw RateWindow) MarshalText() ([]byte, error) {
	return []byte(rw.value), nil
}

// UnmarshalText parses serialized text into a known RateWindow.
func (rw *RateWindow) UnmarshalText(data []byte) error {
	window, err := ParseRateWindow(string(data))
	if err != nil {
		return err
	}

	*rw = window
	return nil
}

// =============================================================================

// ParseRateWindow parses value and returns the corresponding RateWindow when
// it exists.
func ParseRateWindow(value string) (RateWindow, error) {
	window, exists := rateWindows[value]
	if !exists {
		return RateWindow{}, fmt.Errorf("invalid rate window %q", value)
	}

	return window, nil
}

// MustParseRateWindow parses value and returns the corresponding RateWindow.
// It panics when value does not identify a known rate window.
func MustParseRateWindow(value string) RateWindow {
	window, err := ParseRateWindow(value)
	if err != nil {
		panic(err)
	}

	return window
}

// =============================================================================

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
