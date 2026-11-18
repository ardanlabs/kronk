package parsers

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// D carries tool declarations and their nested schema objects.
type D map[string]any

// MapToD recursively normalizes JSON-decoded maps and arrays into D values.
// Arrays containing only maps become []D, including empty arrays.
func MapToD(m map[string]any) D {
	d := make(D, len(m))
	for k, v := range m {
		d[k] = convertValue(v)
	}
	return d
}

func convertValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return MapToD(val)
	case []any:
		allMaps := true
		for _, elem := range val {
			if _, ok := elem.(map[string]any); !ok {
				allMaps = false
				break
			}
		}
		if allMaps {
			result := make([]D, len(val))
			for i, elem := range val {
				result[i] = convertValue(elem).(D)
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

// ToolCallArguments marshals as a JSON string and accepts strings or objects
// while decoding, preserving numeric values as json.Number.
type ToolCallArguments map[string]any

// MarshalJSON encodes nil arguments as an empty object inside a JSON string.
func (a ToolCallArguments) MarshalJSON() ([]byte, error) {
	if a == nil {
		a = ToolCallArguments{}
	}
	inner, err := json.Marshal(map[string]any(a))
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(inner))
}

// UnmarshalJSON decodes an object or a JSON-encoded object string.
func (a *ToolCallArguments) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*a = nil
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s == "" {
			*a = nil
			return nil
		}
		var m map[string]any
		if err := decodeJSONWithNumber(s, &m); err != nil {
			return err
		}
		*a = m
		return nil
	}
	var m map[string]any
	if err := decodeJSONWithNumber(string(data), &m); err != nil {
		return err
	}
	*a = m
	return nil
}

func decodeJSONWithNumber(data string, value any) error {
	dec := json.NewDecoder(strings.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(value); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("unexpected data after JSON value")
		}
		return err
	}
	return nil
}

// ResponseToolCallFunction carries a function name and parsed arguments.
type ResponseToolCallFunction struct {
	Name      string            `json:"name"`
	Arguments ToolCallArguments `json:"arguments"`
}

// ResponseToolCall carries a parsed call or an atomic parsing failure.
type ResponseToolCall struct {
	ID       string                   `json:"id"`
	Index    int                      `json:"-"`
	Type     string                   `json:"type"`
	Function ResponseToolCallFunction `json:"function"`
	Status   int                      `json:"status,omitempty"`
	Raw      string                   `json:"raw,omitempty"`
	Error    string                   `json:"error,omitempty"`
}

// ResponseToolCallDeltaFunction carries incremental function-call content.
type ResponseToolCallDeltaFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments"`
}

// ResponseToolCallDelta carries an incremental tool-call activity delta.
type ResponseToolCallDelta struct {
	ID       string                        `json:"id,omitempty"`
	Index    int                           `json:"index"`
	Type     string                        `json:"type,omitempty"`
	Function ResponseToolCallDeltaFunction `json:"function"`
}
