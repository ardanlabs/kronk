package download

import "fmt"

// The set of architectures that can be used.
var (
	AMD64 = newArch("amd64")
	ARM64 = newArch("arm64")
)

// =============================================================================

// Set of known architectures.
var archs = make(map[string]Arch)

// Arch represents a architecture option.
type Arch struct {
	value string
}

func newArch(arch string) Arch {
	a := Arch{arch}
	archs[arch] = a
	return a
}

// String returns the name of the architecture.
func (a Arch) String() string {
	return a.value
}

// Equal provides support for the go-cmp package and testing.
func (a Arch) Equal(a2 Arch) bool {
	return a.value == a2.value
}

// MarshalText provides support for logging and any marshal needs.
func (a Arch) MarshalText() ([]byte, error) {
	return []byte(a.value), nil
}

// =============================================================================

// ParseArch parses the string value and returns an Arch if one exists.
func ParseArch(value string) (Arch, error) {
	arch, exists := archs[value]
	if !exists {
		return Arch{}, fmt.Errorf("invalid architecture %q", value)
	}

	return arch, nil
}

// MustParseArch parses the string value and returns an Arch if one exists. If
// an error occurs the function panics.
func MustParseArch(value string) Arch {
	arch, err := ParseArch(value)
	if err != nil {
		panic(err)
	}

	return arch
}
