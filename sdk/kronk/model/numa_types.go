package model

import "fmt"

// NUMAStrategy controls NUMA placement. Its zero value disables NUMA.
type NUMAStrategy struct{ value string }

var numaStrategies = make(map[string]NUMAStrategy)
var (
	// NUMADisabled disables NUMA placement.
	NUMADisabled = newNUMAStrategy("")
	// NUMADistribute distributes work across nodes.
	NUMADistribute = newNUMAStrategy("distribute")
	// NUMAIsolate restricts work to one node.
	NUMAIsolate = newNUMAStrategy("isolate")
	// NUMANumactl uses the numactl configuration.
	NUMANumactl = newNUMAStrategy("numactl")
	// NUMAMirror mirrors work across nodes.
	NUMAMirror = newNUMAStrategy("mirror")
)

func newNUMAStrategy(value string) NUMAStrategy {
	t := NUMAStrategy{value: value}
	numaStrategies[value] = t
	return t
}

// String returns the strategy name, empty when disabled.
func (ns NUMAStrategy) String() string { return ns.value }

// Equal reports whether the values match.
func (ns NUMAStrategy) Equal(other NUMAStrategy) bool { return ns == other }

// IsZero reports whether NUMA is disabled.
func (ns NUMAStrategy) IsZero() bool { return ns.value == "" }

// ParseNUMAStrategy parses a registered strategy, including disabled empty.
func ParseNUMAStrategy(value string) (NUMAStrategy, error) {
	t, ok := numaStrategies[value]
	if !ok {
		return NUMAStrategy{}, fmt.Errorf("invalid NUMA strategy %q", value)
	}
	return t, nil
}

// MustParseNUMAStrategy parses a name and panics if it is invalid.
func MustParseNUMAStrategy(value string) NUMAStrategy {
	t, err := ParseNUMAStrategy(value)
	if err != nil {
		panic(err)
	}
	return t
}

// MarshalText serializes the strategy name.
func (ns NUMAStrategy) MarshalText() ([]byte, error) { return []byte(ns.value), nil }

// UnmarshalText parses a registered strategy.
func (ns *NUMAStrategy) UnmarshalText(data []byte) error {
	v, err := ParseNUMAStrategy(string(data))
	if err != nil {
		return err
	}
	*ns = v
	return nil
}
