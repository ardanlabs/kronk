package model

import (
	"encoding/json"
	"fmt"
)

// MediaType identifies input media. Its zero value is none.
type MediaType struct{ value int }

var mediaTypes = make(map[string]MediaType)
var (
	// MediaTypeNone identifies text-only input.
	MediaTypeNone = newMediaType("none", 0)
	// MediaTypeVision identifies visual input.
	MediaTypeVision = newMediaType("vision", 1)
	// MediaTypeAudio identifies audio input.
	MediaTypeAudio = newMediaType("audio", 2)
)

func newMediaType(name string, value int) MediaType {
	t := MediaType{value: value}
	mediaTypes[name] = t
	return t
}

// String returns the media name.
func (mt MediaType) String() string {
	switch mt {
	case MediaTypeNone:
		return "none"
	case MediaTypeVision:
		return "vision"
	case MediaTypeAudio:
		return "audio"
	default:
		return "unknown"
	}
}

// Equal reports whether the values match.
func (mt MediaType) Equal(other MediaType) bool { return mt == other }

// IsZero reports whether there is no media.
func (mt MediaType) IsZero() bool { return mt.value == 0 }

// ParseMediaType parses a registered media name.
func ParseMediaType(value string) (MediaType, error) {
	t, ok := mediaTypes[value]
	if !ok {
		return MediaType{}, fmt.Errorf("invalid media type %q", value)
	}
	return t, nil
}

// MustParseMediaType parses a name and panics if it is invalid.
func MustParseMediaType(value string) MediaType {
	t, err := ParseMediaType(value)
	if err != nil {
		panic(err)
	}
	return t
}

// MarshalText serializes the media name.
func (mt MediaType) MarshalText() ([]byte, error) { return []byte(mt.String()), nil }

// UnmarshalText parses a registered media name.
func (mt *MediaType) UnmarshalText(data []byte) error {
	v, err := ParseMediaType(string(data))
	if err != nil {
		return err
	}
	*mt = v
	return nil
}

// MarshalJSON preserves the numeric wire representation.
func (mt MediaType) MarshalJSON() ([]byte, error) { return json.Marshal(mt.value) }

// UnmarshalJSON validates a numeric wire value.
func (mt *MediaType) UnmarshalJSON(data []byte) error {
	value := mt.value
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return mt.fromValue(value)
}
func (mt *MediaType) fromValue(value int) error {
	for _, v := range mediaTypes {
		if v.value == value {
			*mt = v
			return nil
		}
	}
	return fmt.Errorf("invalid media type %d", value)
}

// MarshalYAML preserves the numeric wire representation.
func (mt MediaType) MarshalYAML() (any, error) { return mt.value, nil }

// UnmarshalYAML validates a numeric wire value.
func (mt *MediaType) UnmarshalYAML(unmarshal func(any) error) error {
	var value int
	if err := unmarshal(&value); err != nil {
		return err
	}
	return mt.fromValue(value)
}
