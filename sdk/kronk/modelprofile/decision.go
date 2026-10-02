package modelprofile

import (
	"fmt"
	"strconv"
	"strings"
)

type decisionValue struct {
	key   string
	value string
}

// Decision contains normalized metadata for a decision model.
type Decision struct {
	Type          string
	maxHeadTokens string
	temperatures  map[string]decisionValue
}

// MaxHeadTokens returns the maximum number of tokens accepted by a decision
// head. It returns an error when the metadata is missing or invalid.
func (d Decision) MaxHeadTokens() (int, error) {
	value, err := strconv.Atoi(d.maxHeadTokens)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid max_head_tokens %q", d.maxHeadTokens)
	}

	return value, nil
}

// Temperature returns a calibrated decision temperature by normalized suffix.
// The boolean reports whether the model declares that temperature.
func (d Decision) Temperature(suffix string) (float64, bool, error) {
	value, exists := d.temperatures[suffix]
	if !exists {
		return 0, false, nil
	}

	temperature, err := strconv.ParseFloat(value.value, 64)
	if err != nil || temperature <= 0 {
		return 0, true, fmt.Errorf("invalid decision temperature %s=%q", value.key, value.value)
	}

	return temperature, true, nil
}

func resolveDecision(metadata metadata, architecture string) Decision {
	prefix := architecture + ".decision."
	temperaturePrefix := prefix + "temperature."
	decision := Decision{
		Type:          metadata.value(prefix + "type"),
		maxHeadTokens: metadata.value(prefix + "max_head_tokens"),
		temperatures:  make(map[string]decisionValue),
	}

	for key, value := range metadata.values {
		if suffix, ok := strings.CutPrefix(key, temperaturePrefix); ok {
			decision.temperatures[suffix] = decisionValue{key: key, value: value}
		}
	}

	return decision
}
