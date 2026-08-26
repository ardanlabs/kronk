package modelprofile

import (
	"strconv"
	"strings"
)

type metadata struct {
	values map[string]string
}

func newMetadata(values map[string]string) metadata {
	return metadata{values: values}
}

func (m metadata) value(key string) string {
	return m.values[key]
}

func (m metadata) hasKeyFragment(fragments ...string) bool {
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

func (m metadata) positiveNextNPredictLayers() int64 {
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
