package download

import "fmt"

// The set of processors that can be used.
var (
	Linux    = newOS("linux")
	Bookworm = newOS("bookworm")
	Trixie   = newOS("trixie")
	Darwin   = newOS("darwin")
	Windows  = newOS("windows")
)

// =============================================================================

// Set of known processors.
var oss = make(map[string]OS)

// OS represents a operating system option.
type OS struct {
	value string
}

func newOS(os string) OS {
	o := OS{os}
	oss[os] = o
	return o
}

// String returns the name of the operating system.
func (o OS) String() string {
	return o.value
}

// Equal provides support for the go-cmp package and testing.
func (o OS) Equal(r2 OS) bool {
	return o.value == r2.value
}

// MarshalText provides support for logging and any marshal needs.
func (o OS) MarshalText() ([]byte, error) {
	return []byte(o.value), nil
}

// =============================================================================

// ParseOS parses the string value and returns a processor if one exists.
func ParseOS(value string) (OS, error) {
	os, exists := oss[value]
	if !exists {
		return OS{}, fmt.Errorf("invalid operating system %q", value)
	}

	return os, nil
}

// MustParseOS parses the string value and returns a processor if one exists. If
// an error occurs the function panics.
func MustParseOS(value string) OS {
	os, err := ParseOS(value)
	if err != nil {
		panic(err)
	}

	return os
}
