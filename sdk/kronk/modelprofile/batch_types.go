package modelprofile

import "fmt"

// BatchInputMixing describes whether token and embedding rows may share a batch.
// Its zero value is unknown.
type BatchInputMixing struct{ value string }

var batchInputMixings = make(map[string]BatchInputMixing)

var (
	// BatchInputMixingUnknown identifies unvalidated support.
	BatchInputMixingUnknown = newBatchInputMixing("")
	// BatchInputMixingSupported permits mixing token and embedding rows.
	BatchInputMixingSupported = newBatchInputMixing("supported")
	// BatchInputMixingUnsupported requires isolated batches.
	BatchInputMixingUnsupported = newBatchInputMixing("unsupported")
)

func newBatchInputMixing(value string) BatchInputMixing {
	t := BatchInputMixing{value: value}
	batchInputMixings[value] = t
	return t
}

// String returns the wire value.
func (bim BatchInputMixing) String() string { return bim.value }

// Equal reports whether the values match.
func (bim BatchInputMixing) Equal(other BatchInputMixing) bool { return bim == other }

// IsZero reports whether support is unknown.
func (bim BatchInputMixing) IsZero() bool { return bim.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (bim BatchInputMixing) MarshalText() ([]byte, error) { return []byte(bim.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (bim *BatchInputMixing) UnmarshalText(data []byte) error {
	v, err := ParseBatchInputMixing(string(data))
	if err != nil {
		return err
	}
	*bim = v
	return nil
}

// ParseBatchInputMixing parses a registered wire value.
func ParseBatchInputMixing(value string) (BatchInputMixing, error) {
	t, ok := batchInputMixings[value]
	if !ok {
		return BatchInputMixing{}, fmt.Errorf("invalid batch input mixing %q", value)
	}
	return t, nil
}

// MustParseBatchInputMixing parses a value and panics if it is invalid.
func MustParseBatchInputMixing(value string) BatchInputMixing {
	t, err := ParseBatchInputMixing(value)
	if err != nil {
		panic(err)
	}
	return t
}
