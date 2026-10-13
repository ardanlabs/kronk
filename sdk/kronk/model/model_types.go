package model

import (
	"encoding/json"
	"fmt"
)

// ModelType represents the model architecture. Its zero value is dense.
type ModelType struct{ value uint8 }

var modelTypes = make(map[string]ModelType)

var (
	// ModelTypeDense uses partial-range state cleanup.
	ModelTypeDense = newModelType("dense", 0)
	// ModelTypeMoE uses expert routing and partial-range state cleanup.
	ModelTypeMoE = newModelType("moe", 1)
	// ModelTypeHybrid requires snapshot/restore for recurrent state.
	ModelTypeHybrid = newModelType("hybrid", 2)
)

func newModelType(name string, value uint8) ModelType {
	t := ModelType{value: value}
	modelTypes[name] = t
	return t
}

// String returns the model architecture name.
func (t ModelType) String() string {
	switch t {
	case ModelTypeDense:
		return "dense"
	case ModelTypeMoE:
		return "moe"
	case ModelTypeHybrid:
		return "hybrid"
	default:
		return "unknown"
	}
}

// Equal reports whether the values match.
func (t ModelType) Equal(other ModelType) bool { return t == other }

// IsZero reports whether the architecture is dense.
func (t ModelType) IsZero() bool { return t.value == 0 }

// ParseModelType parses a registered architecture name.
func ParseModelType(value string) (ModelType, error) {
	t, ok := modelTypes[value]
	if !ok {
		return ModelType{}, fmt.Errorf("invalid model type %q", value)
	}
	return t, nil
}

// MustParseModelType parses a name and panics if it is invalid.
func MustParseModelType(value string) ModelType {
	t, err := ParseModelType(value)
	if err != nil {
		panic(err)
	}
	return t
}

// MarshalText serializes the architecture name.
func (t ModelType) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText parses a registered architecture name.
func (t *ModelType) UnmarshalText(data []byte) error {
	v, err := ParseModelType(string(data))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// MarshalJSON preserves the numeric wire representation.
func (t ModelType) MarshalJSON() ([]byte, error) { return json.Marshal(t.value) }

// UnmarshalJSON validates a numeric wire value.
func (t *ModelType) UnmarshalJSON(data []byte) error {
	value := t.value
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return t.fromValue(value)
}
func (t *ModelType) fromValue(value uint8) error {
	for _, v := range modelTypes {
		if v.value == value {
			*t = v
			return nil
		}
	}
	return fmt.Errorf("invalid model type %d", value)
}

// MarshalYAML preserves the numeric wire representation.
func (t ModelType) MarshalYAML() (any, error) { return t.value, nil }

// UnmarshalYAML validates a numeric wire value.
func (t *ModelType) UnmarshalYAML(unmarshal func(any) error) error {
	var value uint8
	if err := unmarshal(&value); err != nil {
		return err
	}
	return t.fromValue(value)
}
