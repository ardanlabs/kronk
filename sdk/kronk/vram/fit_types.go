package vram

import "fmt"

// FitStatus describes how a memory requirement compares with capacity.
// Its zero value is unset, distinct from FitStatusUnknown.
type FitStatus struct{ value string }

var fitStatuses = make(map[string]FitStatus)

var (
	// FitStatusUnknown indicates that no capacity was supplied.
	FitStatusUnknown = newFitStatus("unknown")
	// FitStatusFits indicates comfortable headroom.
	FitStatusFits = newFitStatus("fits")
	// FitStatusTight indicates that more than 80% of capacity is used.
	FitStatusTight = newFitStatus("tight")
	// FitStatusDoesNotFit indicates that the fit threshold is exceeded.
	FitStatusDoesNotFit = newFitStatus("does_not_fit")
)

func newFitStatus(value string) FitStatus {
	t := FitStatus{value: value}
	fitStatuses[value] = t
	return t
}

// String returns the wire value.
func (t FitStatus) String() string { return t.value }

// Equal reports whether the values match.
func (t FitStatus) Equal(other FitStatus) bool { return t == other }

// IsZero reports whether the value is unset.
func (t FitStatus) IsZero() bool { return t.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (t FitStatus) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (t *FitStatus) UnmarshalText(data []byte) error {
	v, err := ParseFitStatus(string(data))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// ParseFitStatus parses a registered wire value or the unset value.
func ParseFitStatus(value string) (FitStatus, error) {
	if value == "" {
		return FitStatus{}, nil
	}
	t, ok := fitStatuses[value]
	if !ok {
		return FitStatus{}, fmt.Errorf("invalid fit status %q", value)
	}
	return t, nil
}

// MustParseFitStatus parses a value and panics if it is invalid.
func MustParseFitStatus(value string) FitStatus {
	t, err := ParseFitStatus(value)
	if err != nil {
		panic(err)
	}
	return t
}
