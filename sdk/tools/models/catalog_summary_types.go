package models

import "fmt"

// MTPSource identifies how a model supplies its MTP drafter.
type MTPSource struct{ value string }

var mtpSources = make(map[string]MTPSource)

var (
	// MTPSourceEmbedded means the MTP head is embedded in the model GGUF.
	MTPSourceEmbedded = newMTPSource("embedded")
	// MTPSourceCompanion means the MTP drafter is a separate GGUF file.
	MTPSourceCompanion = newMTPSource("companion")
)

func newMTPSource(value string) MTPSource {
	t := MTPSource{value: value}
	mtpSources[value] = t
	return t
}

// String returns the canonical name.
func (t MTPSource) String() string { return t.value }

// Equal provides support for comparison and testing.
func (t MTPSource) Equal(other MTPSource) bool { return t.value == other.value }

// IsZero reports whether the value is unset.
func (t MTPSource) IsZero() bool { return t.value == "" }

// MarshalText provides support for logging and serialization.
func (t MTPSource) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText parses a registered value.
func (t *MTPSource) UnmarshalText(data []byte) error {
	parsed, err := ParseMTPSource(string(data))
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// ParseMTPSource parses a registered name or the unset value.
func ParseMTPSource(value string) (MTPSource, error) {
	if value == "" {
		return MTPSource{}, nil
	}
	t, exists := mtpSources[value]
	if !exists {
		return MTPSource{}, fmt.Errorf("invalid MTP source %q", value)
	}
	return t, nil
}

// MustParseMTPSource parses a registered name and panics for an unknown name.
func MustParseMTPSource(value string) MTPSource {
	t, err := ParseMTPSource(value)
	if err != nil {
		panic(err)
	}
	return t
}
