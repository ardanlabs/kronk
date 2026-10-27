package arch

import (
	"strconv"
	"strings"
)

// Metadata provides architecture adapters with the raw GGUF metadata.
type Metadata struct {
	values map[string]string
}

// NewMetadata wraps metadata for architecture resolution without copying it.
func NewMetadata(values map[string]string) Metadata {
	return Metadata{values: values}
}

func (m Metadata) value(key string) string {
	return m.values[key]
}

func (m Metadata) hasKeyFragment(fragments ...string) bool {
	for key := range m.values {
		key = strings.ToLower(key)
		for _, fragment := range fragments {
			if strings.Contains(key, fragment) {
				return true
			}
		}
	}
	return false
}

func (m Metadata) positiveNextNPredictLayers() int64 {
	var layers int64
	for key, value := range m.values {
		if !strings.Contains(strings.ToLower(key), "nextn_predict_layers") {
			continue
		}

		n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err == nil && n > 0 {
			layers = max(layers, max(int64(n), 1))
		}
	}
	return layers
}
