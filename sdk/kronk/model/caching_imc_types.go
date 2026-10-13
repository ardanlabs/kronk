package model

import "fmt"

// IMCSessionState describes an allocated IMC entry. Its zero value is unset.
type IMCSessionState struct{ value string }

var imcSessionStates = make(map[string]IMCSessionState)
var (
	// IMCSessionStateActive means a request holds the entry's reservation.
	IMCSessionStateActive = newIMCSessionState("active")
	// IMCSessionStateIdle means the entry contains a reusable snapshot.
	IMCSessionStateIdle = newIMCSessionState("idle")
	// IMCSessionStateEmpty means the entry has no snapshot.
	IMCSessionStateEmpty = newIMCSessionState("empty")
)

func newIMCSessionState(value string) IMCSessionState {
	t := IMCSessionState{value: value}
	imcSessionStates[value] = t
	return t
}

// String returns the state name.
func (t IMCSessionState) String() string { return t.value }

// Equal reports whether the values match.
func (t IMCSessionState) Equal(other IMCSessionState) bool { return t == other }

// IsZero reports whether the state is unset.
func (t IMCSessionState) IsZero() bool { return t.value == "" }

// ParseIMCSessionState parses a known state or an unset value.
func ParseIMCSessionState(value string) (IMCSessionState, error) {
	if value == "" {
		return IMCSessionState{}, nil
	}
	t, ok := imcSessionStates[value]
	if !ok {
		return IMCSessionState{}, fmt.Errorf("invalid IMC session state %q", value)
	}
	return t, nil
}

// MustParseIMCSessionState parses a name and panics if it is invalid.
func MustParseIMCSessionState(value string) IMCSessionState {
	t, err := ParseIMCSessionState(value)
	if err != nil {
		panic(err)
	}
	return t
}

// MarshalText serializes the state name.
func (t IMCSessionState) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText parses a known state or an unset value.
func (t *IMCSessionState) UnmarshalText(data []byte) error {
	v, err := ParseIMCSessionState(string(data))
	if err != nil {
		return err
	}
	*t = v
	return nil
}
