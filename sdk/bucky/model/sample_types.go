package model

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// SampleEncoding identifies how raw PCM samples are laid out in bytes.
// Only little-endian encodings are listed because every mainstream
// capture stack (CoreAudio, WASAPI, ALSA/PulseAudio/PipeWire, Web
// Audio) emits little-endian on x86 and ARM. Its zero value is Int16LE.
type SampleEncoding struct{ value int }

var sampleEncodings = make(map[string]SampleEncoding)

var (
	// Int16LE is signed 16-bit little-endian PCM (2 bytes/sample). The
	// most universal microphone format. Normalized as v / 32768.
	Int16LE = newSampleEncoding("int16le", 0)

	// Float32LE is 32-bit IEEE-754 little-endian PCM (4 bytes/sample),
	// already in the [-1, 1] range. The native CoreAudio / Web Audio
	// format.
	Float32LE = newSampleEncoding("float32le", 1)
)

func newSampleEncoding(name string, value int) SampleEncoding {
	s := SampleEncoding{value: value}
	sampleEncodings[name] = s
	sampleEncodings[strconv.Itoa(value)] = s
	return s
}

// Int returns the integer value.
func (se SampleEncoding) Int() int { return se.value }

// String returns the sample encoding name.
func (se SampleEncoding) String() string {
	switch se.value {
	case 0:
		return "int16le"
	case 1:
		return "float32le"
	default:
		return ""
	}
}

// Equal provides support for comparison and testing.
func (se SampleEncoding) Equal(other SampleEncoding) bool { return se.value == other.value }

// IsZero reports whether the encoding is the zero-value Int16LE encoding.
func (se SampleEncoding) IsZero() bool { return se.value == 0 }

// MarshalText provides support for logging and serialization.
func (se SampleEncoding) MarshalText() ([]byte, error) { return []byte(se.String()), nil }

// UnmarshalText parses a known sample encoding.
func (se *SampleEncoding) UnmarshalText(data []byte) error {
	parsed, err := ParseSampleEncoding(string(data))
	if err != nil {
		return err
	}
	*se = parsed
	return nil
}

// MarshalJSON preserves the numeric wire representation.
func (se SampleEncoding) MarshalJSON() ([]byte, error) { return json.Marshal(se.value) }

// UnmarshalJSON parses a known numeric sample encoding.
func (se *SampleEncoding) UnmarshalJSON(data []byte) error {
	value := se.value
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return se.UnmarshalText([]byte(strconv.Itoa(value)))
}

// MarshalYAML preserves the numeric wire representation.
func (se SampleEncoding) MarshalYAML() (any, error) { return se.value, nil }

// UnmarshalYAML parses a known numeric sample encoding.
func (se *SampleEncoding) UnmarshalYAML(unmarshal func(any) error) error {
	var value int
	if err := unmarshal(&value); err != nil {
		return err
	}
	return se.UnmarshalText([]byte(strconv.Itoa(value)))
}

// ParseSampleEncoding parses a known name or numeric value, defaulting empty to Int16LE.
func ParseSampleEncoding(value string) (SampleEncoding, error) {
	if value == "" {
		return SampleEncoding{}, nil
	}
	s, exists := sampleEncodings[value]
	if !exists {
		return SampleEncoding{}, fmt.Errorf("invalid sample encoding %q", value)
	}
	return s, nil
}

// MustParseSampleEncoding parses an encoding and panics for an unknown value.
func MustParseSampleEncoding(value string) SampleEncoding {
	s, err := ParseSampleEncoding(value)
	if err != nil {
		panic(err)
	}
	return s
}
