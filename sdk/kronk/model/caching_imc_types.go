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
func (iss IMCSessionState) String() string { return iss.value }

// Equal reports whether the values match.
func (iss IMCSessionState) Equal(other IMCSessionState) bool { return iss == other }

// IsZero reports whether the state is unset.
func (iss IMCSessionState) IsZero() bool { return iss.value == "" }

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
func (iss IMCSessionState) MarshalText() ([]byte, error) { return []byte(iss.value), nil }

// UnmarshalText parses a known state or an unset value.
func (iss *IMCSessionState) UnmarshalText(data []byte) error {
	v, err := ParseIMCSessionState(string(data))
	if err != nil {
		return err
	}
	*iss = v
	return nil
}
