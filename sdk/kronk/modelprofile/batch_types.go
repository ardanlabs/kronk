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
func (t BatchInputMixing) String() string { return t.value }

// Equal reports whether the values match.
func (t BatchInputMixing) Equal(other BatchInputMixing) bool { return t == other }

// IsZero reports whether support is unknown.
func (t BatchInputMixing) IsZero() bool { return t.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (t BatchInputMixing) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (t *BatchInputMixing) UnmarshalText(data []byte) error {
	v, err := ParseBatchInputMixing(string(data))
	if err != nil {
		return err
	}
	*t = v
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
