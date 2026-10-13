package model

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// EventKind classifies a transcript Event delivered on a Stream's channel.
// Its zero value is EventPartial.
type EventKind struct{ value int }

var eventKinds = make(map[string]EventKind)

var (
	// EventPartial is a tentative, revisable re-decode of the current
	// un-committed audio window. Its Text is the COMPLETE hypothesis for
	// that window (not a delta): each Partial SUPERSEDES the previous one,
	// so a consumer should REPLACE its pending-text buffer with Text. This
	// is what produces the "words change as you keep talking" rewrite
	// effect seen in whisper.cpp's stream example. Partials may be dropped
	// under load.
	EventPartial = newEventKind("partial", 0)

	// EventFinal commits the current un-committed region: it will not be
	// revised. Its Text is the authoritative text for that region. A
	// consumer should APPEND Text to its permanent transcript and clear
	// its pending-partial buffer. Finals are never dropped.
	EventFinal = newEventKind("final", 1)

	// EventReset is emitted after Reset completes, only when the stream
	// was opened WithEmitResetEvent. Text and Segments are empty.
	EventReset = newEventKind("reset", 2)

	// EventError is terminal. Err is set; the Events channel closes next.
	EventError = newEventKind("error", 3)
)

func newEventKind(name string, value int) EventKind {
	k := EventKind{value: value}
	eventKinds[name] = k
	eventKinds[strconv.Itoa(value)] = k
	return k
}

// Int returns the integer value.
func (k EventKind) Int() int { return k.value }

// String returns the event kind name.
func (k EventKind) String() string {
	switch k.value {
	case 0:
		return "partial"
	case 1:
		return "final"
	case 2:
		return "reset"
	case 3:
		return "error"
	default:
		return ""
	}
}

// Equal provides support for comparison and testing.
func (k EventKind) Equal(other EventKind) bool { return k.value == other.value }

// IsZero reports whether the kind is the zero-value partial kind.
func (k EventKind) IsZero() bool { return k.value == 0 }

// MarshalText provides support for logging and serialization.
func (k EventKind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

// UnmarshalText parses a known event kind.
func (k *EventKind) UnmarshalText(data []byte) error {
	parsed, err := ParseEventKind(string(data))
	if err != nil {
		return err
	}
	*k = parsed
	return nil
}

// MarshalJSON preserves the numeric wire representation.
func (k EventKind) MarshalJSON() ([]byte, error) { return json.Marshal(k.value) }

// UnmarshalJSON parses a known numeric event kind.
func (k *EventKind) UnmarshalJSON(data []byte) error {
	value := k.value
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return k.UnmarshalText([]byte(strconv.Itoa(value)))
}

// MarshalYAML preserves the numeric wire representation.
func (k EventKind) MarshalYAML() (any, error) { return k.value, nil }

// UnmarshalYAML parses a known numeric event kind.
func (k *EventKind) UnmarshalYAML(unmarshal func(any) error) error {
	var value int
	if err := unmarshal(&value); err != nil {
		return err
	}
	return k.UnmarshalText([]byte(strconv.Itoa(value)))
}

// ParseEventKind parses a known name or numeric value, defaulting empty to partial.
func ParseEventKind(value string) (EventKind, error) {
	if value == "" {
		return EventKind{}, nil
	}
	k, exists := eventKinds[value]
	if !exists {
		return EventKind{}, fmt.Errorf("invalid event kind %q", value)
	}
	return k, nil
}

// MustParseEventKind parses a kind and panics for an unknown value.
func MustParseEventKind(value string) EventKind {
	k, err := ParseEventKind(value)
	if err != nil {
		panic(err)
	}
	return k
}
