package model

import "fmt"

// ControlType selects how a ControlNet Union model interprets its control image.
// Its zero value leaves task selection unset.
type ControlType struct{ value string }

var controlTypes = make(map[string]ControlType)

var (
	// ControlTypeNone selects no Union task.
	ControlTypeNone = newControlType("none")
	// ControlTypeOpenPose selects pose-skeleton guidance.
	ControlTypeOpenPose = newControlType("openpose")
	// ControlTypeDepth selects depth-map guidance.
	ControlTypeDepth = newControlType("depth")
	// ControlTypeScribble selects scribble or soft-edge guidance.
	ControlTypeScribble = newControlType("scribble", "hed", "softedge", "pidi", "ted")
	// ControlTypeCanny selects edge-map guidance.
	ControlTypeCanny = newControlType("canny", "lineart", "anime_lineart", "mlsd")
	// ControlTypeNormal selects surface-normal guidance.
	ControlTypeNormal = newControlType("normal")
	// ControlTypeSegment selects segmentation-map guidance.
	ControlTypeSegment = newControlType("segment", "seg")
	// ControlTypeTile selects tile guidance for checkpoints that support it.
	ControlTypeTile = newControlType("tile")
	// ControlTypeRepaint selects repaint guidance for checkpoints that support it.
	ControlTypeRepaint = newControlType("repaint", "inpaint", "outpaint")
)

func newControlType(value string, aliases ...string) ControlType {
	ct := ControlType{value: value}
	controlTypes[value] = ct
	for _, alias := range aliases {
		controlTypes[alias] = ct
	}
	return ct
}

// String returns the canonical task name.
func (ct ControlType) String() string { return ct.value }

// Equal reports whether the values match.
func (ct ControlType) Equal(other ControlType) bool { return ct == other }

// IsZero reports whether task selection is unset.
func (ct ControlType) IsZero() bool { return ct.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (ct ControlType) MarshalText() ([]byte, error) { return []byte(ct.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (ct *ControlType) UnmarshalText(data []byte) error {
	parsed, err := ParseControlType(string(data))
	if err != nil {
		return err
	}
	*ct = parsed
	return nil
}

// ParseControlType parses a task name, alias, or the unset value.
func ParseControlType(value string) (ControlType, error) {
	if value == "" {
		return ControlType{}, nil
	}
	ct, ok := controlTypes[value]
	if !ok {
		return ControlType{}, fmt.Errorf("invalid control type %q", value)
	}
	return ct, nil
}

// MustParseControlType parses a task and panics if it is invalid.
func MustParseControlType(value string) ControlType {
	ct, err := ParseControlType(value)
	if err != nil {
		panic(err)
	}
	return ct
}
