package client

import (
	"strconv"
	"strings"
	"time"
)

// D is a convenience type for building JSON request bodies.
type D map[string]any

// =============================================================================

// Error represents an error response from an OpenAI-compatible API.
type Error struct {
	Err struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Error implements the error interface.
func (err *Error) Error() string {
	return err.Err.Message
}

// =============================================================================

// Time wraps time.Time with custom JSON marshaling for Unix timestamps.
type Time struct {
	time.Time
}

// UnmarshalJSON decodes a Unix timestamp into a Time value.
func (t *Time) UnmarshalJSON(data []byte) error {
	d := strings.Trim(string(data), "\"")

	num, err := strconv.Atoi(d)
	if err != nil {
		return err
	}

	t.Time = time.Unix(int64(num), 0)

	return nil
}

// MarshalJSON encodes a Time value as a Unix timestamp.
func (t Time) MarshalJSON() ([]byte, error) {
	data := strconv.Itoa(int(t.Unix()))
	return []byte(data), nil
}
