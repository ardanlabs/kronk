package modelprofile

import (
	"fmt"
	"math"
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
	scoreBiases   map[string]decisionValue
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

	bits := 64
	switch d.Type {
	case "pplx-decider", "lfm2-d1", "vllm-decision2", "vllm-d3":
		bits = 32
	}
	temperature, err := strconv.ParseFloat(value.value, bits)
	if err != nil || temperature <= 0 || math.IsNaN(temperature) || math.IsInf(temperature, 0) {
		return 0, true, fmt.Errorf("invalid decision temperature %s=%q", value.key, value.value)
	}

	return temperature, true, nil
}

// ScoreBias returns the calibration offsets for a score question's levels.
// Missing offsets are zero. Invalid metadata is rejected, including offsets
// declared for other level counts.
func (d Decision) ScoreBias(levels int) ([]float64, error) {
	bias := make([]float64, levels)
	for suffix, value := range d.scoreBiases {
		a, b, found := strings.Cut(suffix, ".")
		n, errN := strconv.Atoi(a)
		level, errL := strconv.Atoi(b)
		f, err := strconv.ParseFloat(value.value, 32)
		if !found || errN != nil || errL != nil || n < 2 || n > 255 || level < 0 || level >= n || err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("invalid decision score bias %s=%q", value.key, value.value)
		}
		if n == levels {
			bias[level] = f
		}
	}
	return bias, nil
}

func resolveDecision(metadata metadata, architecture string) Decision {
	prefix := architecture + ".decision."
	temperaturePrefix := prefix + "temperature."
	decision := Decision{
		Type:          metadata.value(prefix + "type"),
		maxHeadTokens: metadata.value(prefix + "max_head_tokens"),
		temperatures:  make(map[string]decisionValue),
		scoreBiases:   make(map[string]decisionValue),
	}

	for key, value := range metadata.values {
		if suffix, ok := strings.CutPrefix(key, temperaturePrefix); ok {
			decision.temperatures[suffix] = decisionValue{key: key, value: value}
		}
		if suffix, ok := strings.CutPrefix(key, prefix+"score_bias."); ok {
			decision.scoreBiases[suffix] = decisionValue{key: key, value: value}
		}
	}

	return decision
}
