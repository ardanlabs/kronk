package pystring

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/nikolalohinski/gonja/v2/builtins/methods/pyerrors"
)

// AttributeGetter is used to resolve nested kw-args.
type AttributeGetter interface {
	Get(string) (any, bool)
}

// KwArgs adds AttributeGetter interface to map[string]any
type KwArgs map[string]any

func (k KwArgs) Get(key string) (any, bool) {
	v, ok := k[key]
	return v, ok
}

// ListArgs adds AttributeGetter interface to []any.
type ListArgs []any

func (k ListArgs) Get(key string) (any, bool) {
	idx, err := strconv.Atoi(key)
	if err != nil || idx < 0 || idx >= len(k) {
		return "", false
	}
	return k[idx], true
}

func getNestedKwArgs(keys []string, kwarg AttributeGetter) (any, error) {
	if len(keys) == 0 {
		// Shouldn't happen if all other logic is correct
		return "", fmt.Errorf("%w: empty key", pyerrors.ErrInternal)
	}

	key := keys[0]
	tail := keys[1:]

	// Recursion stop case
	if len(tail) == 0 {
		if val, ok := kwarg.Get(key); ok {
			return val, nil
		}
		return "", fmt.Errorf("%w: '%s'", pyerrors.ErrKey, key)
	}

	// Fetch key
	maybeVal, ok := kwarg.Get(key)
	if !ok {
		return "", fmt.Errorf("%w: '%s'", pyerrors.ErrKey, key)
	}

	// See if we can recurse down to sub-keys
	switch val := maybeVal.(type) {
	case AttributeGetter:
		return getNestedKwArgs(tail, val)
	case map[string]any:
		return getNestedKwArgs(tail, KwArgs(val))
	case []any:
		return getNestedKwArgs(tail, ListArgs(val))
	case map[string]string:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]AttributeGetter:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]bool:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]int:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]int8:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]int16:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]int32:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]int64:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]uint:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]uint8:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]uint16:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]uint32:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]uint64:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]float32:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))
	case map[string]float64:
		subKwarg := make(map[string]any)
		for k, v := range val {
			subKwarg[k] = v
		}
		return getNestedKwArgs(tail, KwArgs(subKwarg))

	default:
		// Handle slices and arrays using reflection
		v := reflect.ValueOf(maybeVal)
		if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
			listArgs := make([]any, v.Len())
			for i := 0; i < v.Len(); i++ {
				listArgs[i] = v.Index(i).Interface()
			}
			return getNestedKwArgs(tail, ListArgs(listArgs))
		}

		// Handle structs
		v = reflect.ValueOf(maybeVal)
		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}

		if v.Kind() == reflect.Struct {
			// Check if the struct implements AttributeGetter
			if getter, ok := maybeVal.(AttributeGetter); ok {
				return getNestedKwArgs(tail, getter)
			}

			// Otherwise, use reflection to access the fields
			fieldMap := make(map[string]any)
			for i := 0; i < v.NumField(); i++ {
				field := v.Type().Field(i)
				fieldValue := v.Field(i).Interface()
				fieldMap[field.Name] = fieldValue
			}
			return getNestedKwArgs(tail, KwArgs(fieldMap))
		}

		// Handle maps of structs
		if v.Kind() == reflect.Map && v.Type().Key().Kind() == reflect.String {
			mapField := make(map[string]any)
			for _, key := range v.MapKeys() {
				mapField[key.String()] = v.MapIndex(key).Interface()
			}
			return getNestedKwArgs(tail, KwArgs(mapField))
		}
	}

	return "", fmt.Errorf("%w: '%#v' is not a sub-gettable for key %s", pyerrors.ErrValue, maybeVal, key)
}
