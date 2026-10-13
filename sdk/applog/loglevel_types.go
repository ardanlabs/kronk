package applog

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// LogLevel represents the logging level. Its zero value is unset (0).
type LogLevel struct{ value int }

var logLevels = make(map[string]LogLevel)

var (
	// LogSilent disables logging.
	LogSilent = newLogLevel("silent", 1)
	// LogNormal enables normal logging.
	LogNormal = newLogLevel("normal", 2)
)

func newLogLevel(name string, value int) LogLevel {
	ll := LogLevel{value: value}
	logLevels[name] = ll
	logLevels[strconv.Itoa(value)] = ll
	return ll
}

// Int returns the integer value.
func (ll LogLevel) Int() int { return ll.value }

// String returns the logging level name, or empty when unset.
func (ll LogLevel) String() string {
	switch ll.value {
	case 1:
		return "silent"
	case 2:
		return "normal"
	default:
		return ""
	}
}

// Equal provides support for comparison and testing.
func (ll LogLevel) Equal(other LogLevel) bool { return ll.value == other.value }

// IsZero reports whether the level is unset.
func (ll LogLevel) IsZero() bool { return ll.value == 0 }

// MarshalText provides support for logging and serialization.
func (ll LogLevel) MarshalText() ([]byte, error) { return []byte(ll.String()), nil }

// UnmarshalText parses a known logging level.
func (ll *LogLevel) UnmarshalText(data []byte) error {
	parsed, err := ParseLogLevel(string(data))
	if err != nil {
		return err
	}
	*ll = parsed
	return nil
}

// MarshalJSON preserves the numeric wire representation.
func (ll LogLevel) MarshalJSON() ([]byte, error) { return json.Marshal(ll.value) }

// UnmarshalJSON parses a known numeric logging level.
func (ll *LogLevel) UnmarshalJSON(data []byte) error {
	value := ll.value
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return ll.UnmarshalText([]byte(strconv.Itoa(value)))
}

// MarshalYAML preserves the numeric wire representation.
func (ll LogLevel) MarshalYAML() (any, error) { return ll.value, nil }

// UnmarshalYAML parses a known numeric logging level.
func (ll *LogLevel) UnmarshalYAML(unmarshal func(any) error) error {
	var value int
	if err := unmarshal(&value); err != nil {
		return err
	}
	return ll.UnmarshalText([]byte(strconv.Itoa(value)))
}

// ParseLogLevel parses a known name or numeric value, or an unset value.
func ParseLogLevel(value string) (LogLevel, error) {
	if value == "" || value == "0" {
		return LogLevel{}, nil
	}
	ll, exists := logLevels[value]
	if !exists {
		return LogLevel{}, fmt.Errorf("invalid log level %q", value)
	}
	return ll, nil
}

// MustParseLogLevel parses a level and panics for an unknown value.
func MustParseLogLevel(value string) LogLevel {
	ll, err := ParseLogLevel(value)
	if err != nil {
		panic(err)
	}
	return ll
}
