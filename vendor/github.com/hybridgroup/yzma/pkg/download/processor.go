package download

import "fmt"

// The set of processors that can be used.
var (
	CPU    = newProcessor("cpu")
	CUDA   = newProcessor("cuda")
	Metal  = newProcessor("metal")
	ROCm   = newProcessor("rocm")
	Vulkan = newProcessor("vulkan")
)

// =============================================================================

// Set of known processors.
var processors = make(map[string]Processor)

// Processor represents a hardare option.
type Processor struct {
	value string
}

func newProcessor(processor string) Processor {
	p := Processor{processor}
	processors[processor] = p
	return p
}

// String returns the name of the processor.
func (p Processor) String() string {
	return p.value
}

// Equal provides support for the go-cmp package and testing.
func (p Processor) Equal(r2 Processor) bool {
	return p.value == r2.value
}

// MarshalText provides support for logging and any marshal needs.
func (p Processor) MarshalText() ([]byte, error) {
	return []byte(p.value), nil
}

// =============================================================================

// ParseProcessor parses the string value and returns a processor if one exists.
func ParseProcessor(value string) (Processor, error) {
	processor, exists := processors[value]
	if !exists {
		return Processor{}, fmt.Errorf("invalid processor %q", value)
	}

	return processor, nil
}

// MustParseProcessor parses the string value and returns a processor if one exists. If
// an error occurs the function panics.
func MustParseProcessor(value string) Processor {
	processor, err := ParseProcessor(value)
	if err != nil {
		panic(err)
	}

	return processor
}
