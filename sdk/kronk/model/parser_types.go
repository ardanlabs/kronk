package model

import (
	"encoding/json"
	"fmt"
)

// Channel labels emitted tokens. Its zero value is none.
type Channel struct{ value uint8 }

var channels = make(map[string]Channel)
var (
	// ChannelNone identifies a structural marker, counted but not surfaced.
	ChannelNone = newChannel("none", 0)
	// ChannelReasoning identifies reasoning_content deltas.
	ChannelReasoning = newChannel("reasoning", 1)
	// ChannelAnswer identifies content deltas.
	ChannelAnswer = newChannel("answer", 2)
	// ChannelTool identifies content accumulated for ToolCall parsing.
	ChannelTool = newChannel("tool", 3)
)

func newChannel(name string, value uint8) Channel {
	t := Channel{value: value}
	channels[name] = t
	return t
}

// String returns the channel name.
func (t Channel) String() string {
	switch t {
	case ChannelReasoning:
		return "reasoning"
	case ChannelAnswer:
		return "answer"
	case ChannelTool:
		return "tool"
	default:
		return "none"
	}
}

// Equal reports whether the values match.
func (t Channel) Equal(other Channel) bool { return t == other }

// IsZero reports whether the channel is none.
func (t Channel) IsZero() bool { return t.value == 0 }

// ParseChannel parses a registered channel name.
func ParseChannel(value string) (Channel, error) {
	t, ok := channels[value]
	if !ok {
		return Channel{}, fmt.Errorf("invalid channel %q", value)
	}
	return t, nil
}

// MustParseChannel parses a name and panics if it is invalid.
func MustParseChannel(value string) Channel {
	t, err := ParseChannel(value)
	if err != nil {
		panic(err)
	}
	return t
}

// MarshalText serializes the channel name.
func (t Channel) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText parses a registered channel name.
func (t *Channel) UnmarshalText(data []byte) error {
	v, err := ParseChannel(string(data))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// MarshalJSON preserves the numeric wire representation.
func (t Channel) MarshalJSON() ([]byte, error) { return json.Marshal(t.value) }

// UnmarshalJSON validates a numeric wire value.
func (t *Channel) UnmarshalJSON(data []byte) error {
	value := t.value
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return t.fromValue(value)
}
func (t *Channel) fromValue(value uint8) error {
	for _, v := range channels {
		if v.value == value {
			*t = v
			return nil
		}
	}
	return fmt.Errorf("invalid channel %d", value)
}

// MarshalYAML preserves the numeric wire representation.
func (t Channel) MarshalYAML() (any, error) { return t.value, nil }

// UnmarshalYAML validates a numeric wire value.
func (t *Channel) UnmarshalYAML(unmarshal func(any) error) error {
	var value uint8
	if err := unmarshal(&value); err != nil {
		return err
	}
	return t.fromValue(value)
}
