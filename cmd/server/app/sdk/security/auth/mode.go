package auth

import "fmt"

// Set of known authorization modes.
var authorizationModes = make(map[string]Mode)

// The set of authorization modes that can be used.
var (
	// Open leaves discovery, inference, and management endpoints open.
	Open = newMode("open")

	// Management protects management endpoints with administrator access while
	// leaving discovery and inference open.
	Management = newMode("management")

	// Authenticated requires a valid JWT for discovery and inference and
	// administrator access for management endpoints.
	Authenticated = newMode("authenticated")

	// FullProtected requires endpoint grants for inference, a valid JWT for
	// discovery, and administrator access for management endpoints.
	FullProtected = newMode("full-protected")
)

// =============================================================================

// Mode identifies the authorization policy applied to API routes.
type Mode struct {
	value string
}

func newMode(value string) Mode {
	mode := Mode{value: value}
	authorizationModes[value] = mode
	return mode
}

// String returns the name of the mode.
func (m Mode) String() string {
	return m.value
}

// Equal provides support for the go-cmp package and testing.
func (m Mode) Equal(m2 Mode) bool {
	return m.value == m2.value
}

// IsZero reports whether the mode is unset.
func (m Mode) IsZero() bool {
	return m.value == ""
}

// MarshalText provides support for logging and serialization.
func (m Mode) MarshalText() ([]byte, error) {
	return []byte(m.value), nil
}

// UnmarshalText parses serialized text into a known Mode.
func (m *Mode) UnmarshalText(data []byte) error {
	mode, err := ParseMode(string(data))
	if err != nil {
		return err
	}

	*m = mode
	return nil
}

// =============================================================================

// ParseMode parses value and returns the corresponding Mode when it exists.
func ParseMode(value string) (Mode, error) {
	mode, exists := authorizationModes[value]
	if !exists {
		return Mode{}, fmt.Errorf("invalid authorization mode %q", value)
	}

	return mode, nil
}

// MustParseMode parses value and returns the corresponding Mode. It panics
// when value does not identify a known mode.
func MustParseMode(value string) Mode {
	mode, err := ParseMode(value)
	if err != nil {
		panic(err)
	}

	return mode
}
