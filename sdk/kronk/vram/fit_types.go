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
func (fs FitStatus) String() string { return fs.value }

// Equal reports whether the values match.
func (fs FitStatus) Equal(other FitStatus) bool { return fs == other }

// IsZero reports whether the value is unset.
func (fs FitStatus) IsZero() bool { return fs.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (fs FitStatus) MarshalText() ([]byte, error) { return []byte(fs.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (fs *FitStatus) UnmarshalText(data []byte) error {
	v, err := ParseFitStatus(string(data))
	if err != nil {
		return err
	}
	*fs = v
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
