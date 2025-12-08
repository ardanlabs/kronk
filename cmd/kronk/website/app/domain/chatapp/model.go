package chatapp

import (
	"github.com/ardanlabs/kronk/model"
)

func toD(m map[string]any) model.D {
	d := make(model.D, len(m))

	for k, v := range m {
		d[k] = convertValue(v)
	}

	return d
}

func convertValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return toD(val)

	case []any:
		allMaps := true
		for _, elem := range val {
			if _, ok := elem.(map[string]any); !ok {
				allMaps = false
				break
			}
		}

		if allMaps {
			result := make([]model.D, len(val))
			for i, elem := range val {
				result[i] = convertValue(elem).(model.D)
			}
			return result
		}

		for i, elem := range val {
			val[i] = convertValue(elem)
		}

		return val

	default:
		return v
	}
}
