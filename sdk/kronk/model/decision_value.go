package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// MarshalJSON preserves the field order of a DecisionStateValue.
func (o DecisionStateValue) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')

	for i, field := range o {
		if i > 0 {
			buf.WriteByte(',')
		}

		name, err := marshalDecisionJSON(field.Name)
		if err != nil {
			return nil, err
		}
		value, err := marshalDecisionJSON(field.Value)
		if err != nil {
			return nil, err
		}

		buf.Write(name)
		buf.WriteByte(':')
		buf.Write(value)
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func decisionJSON(value any) (string, error) {
	data, err := marshalDecisionJSON(value)
	if err != nil {
		return "", err
	}

	out := make([]byte, 0, len(data)+len(data)/8)
	inString := false
	for i := 0; i < len(data); i++ {
		ch := data[i]
		switch {
		case inString && ch == '\\':
			if i+5 < len(data) && data[i+1] == 'u' && string(data[i+2:i+5]) == "202" && (data[i+5] == '8' || data[i+5] == '9') {
				out = append(out, 0xe2, 0x80, 0xa8+data[i+5]-'8')
				i += 5
				continue
			}
			out = append(out, ch, data[i+1])
			i++
			continue
		case ch == '"':
			inString = !inString
		case !inString && (ch == ',' || ch == ':'):
			out = append(out, ch, ' ')
			continue
		}
		out = append(out, ch)
	}

	return string(out), nil
}

func marshalDecisionJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}

	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}), nil
}

func decisionPythonRepr(value any) (string, error) {
	return pythonRepr(reflect.ValueOf(value))
}

func pythonRepr(value reflect.Value) (string, error) {
	if !value.IsValid() {
		return "None", nil
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "None", nil
		}
		return pythonRepr(value.Elem())
	}

	if value.Type() == reflect.TypeFor[DecisionStateValue]() {
		object := value.Interface().(DecisionStateValue)
		parts := make([]string, 0, len(object))
		for _, field := range object {
			item, err := pythonRepr(reflect.ValueOf(field.Value))
			if err != nil {
				return "", err
			}
			parts = append(parts, pythonQuote(field.Name)+": "+item)
		}
		return "{" + strings.Join(parts, ", ") + "}", nil
	}

	if value.Type() == reflect.TypeFor[json.Number]() {
		return value.Interface().(json.Number).String(), nil
	}

	switch value.Kind() {
	case reflect.String:
		return pythonQuote(value.String()), nil
	case reflect.Bool:
		if value.Bool() {
			return "True", nil
		}
		return "False", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(value.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(value.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		number := value.Float()
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return "", fmt.Errorf("python representation does not support non-finite numbers")
		}
		return strconv.FormatFloat(number, 'g', -1, value.Type().Bits()), nil
	case reflect.Slice, reflect.Array:
		parts := make([]string, value.Len())
		for i := range value.Len() {
			item, err := pythonRepr(value.Index(i))
			if err != nil {
				return "", err
			}
			parts[i] = item
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return "", fmt.Errorf("python representation requires string map keys, got %s", value.Type().Key())
		}
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		parts := make([]string, len(keys))
		for i, key := range keys {
			item, err := pythonRepr(value.MapIndex(key))
			if err != nil {
				return "", err
			}
			parts[i] = pythonQuote(key.String()) + ": " + item
		}
		return "{" + strings.Join(parts, ", ") + "}", nil
	}

	data, err := marshalDecisionJSON(value.Interface())
	if err != nil {
		return "", err
	}

	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return "", err
	}

	return pythonRepr(reflect.ValueOf(decoded))
}

func pythonQuote(value string) string {
	quote := byte('\'')
	if strings.ContainsRune(value, '\'') && !strings.ContainsRune(value, '"') {
		quote = '"'
	}

	var buf strings.Builder
	buf.WriteByte(quote)
	for _, ch := range value {
		switch ch {
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if ch == rune(quote) {
				buf.WriteByte('\\')
				buf.WriteRune(ch)
				continue
			}
			if ch < 0x20 || ch == 0x7f {
				fmt.Fprintf(&buf, `\x%02x`, ch)
				continue
			}
			buf.WriteRune(ch)
		}
	}
	buf.WriteByte(quote)

	return buf.String()
}
