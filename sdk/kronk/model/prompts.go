// DO NOT CHANGE THIS CODE WITHOUT TALKING TO BILL FIRST!
// THIS CODE IS WORKING WELL WITH TOOL CALLING CONSISTENCY.

package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/hybridgroup/yzma/pkg/mtmd"
	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/builtins"
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/loaders"
)

func (m *Model) applyRequestJinjaTemplate(ctx context.Context, d D) (string, [][]byte, error) {
	// We need to identify if there is media in the request. If there is
	// we want to replace the actual media with a media marker `<__media__>`.
	// We will move the media to it's own slice. The next call that will happen
	// is `processBitmap` which will process the prompt and media.

	// Deep copy and normalize d to ensure all nested maps are plain
	// map[string]any types. This avoids reflection errors in gonja when
	// templates call .items() on nested structures like tool_calls arguments.
	normalized := deepNormalize(d)

	var media [][]byte

	if msgs, ok := normalized["messages"].([]any); ok {
		for _, msg := range msgs {
			if doc, ok := msg.(map[string]any); ok {
				if content, exists := doc["content"]; exists {
					if value, ok := content.([]byte); ok {
						media = append(media, value)
						doc["content"] = fmt.Sprintf("%s\n", mtmd.DefaultMarker())
					}
				}
			}
		}
	}

	prompt, err := m.applyJinjaTemplate(ctx, normalized)
	if err != nil {
		return "", nil, err
	}

	return prompt, media, nil
}

func (m *Model) applyJinjaTemplate(ctx context.Context, d map[string]any) (string, error) {
	messages, _ := d["messages"].([]D)
	m.log(ctx, "applyJinjaTemplate", "template", m.template.FileName, "messages", len(messages))

	if m.template.Script == "" {
		return "", errors.New("apply-jinja-template: no template found")
	}

	gonja.DefaultLoader = &noFSLoader{}

	t, err := newTemplateWithFixedItems(m.template.Script)
	if err != nil {
		return "", fmt.Errorf("apply-jinja-template: failed to parse template: %w", err)
	}

	// Ensure add_generation_prompt is set (default true if not specified).
	// This tells the Jinja template to append the assistant role prefix at the
	// end of the prompt, signaling the model to generate a response. When caching
	// the first message, we set this to false so the cached tokens form a valid
	// prefix that can be extended with additional messages in subsequent requests.
	if _, ok := d["add_generation_prompt"]; !ok {
		d["add_generation_prompt"] = true
	}

	data := exec.NewContext(d)

	s, err := t.ExecuteToString(data)
	if err != nil {
		return "", fmt.Errorf("apply-jinja-template: failed to execute template: %w", err)
	}

	return s, nil
}

// =============================================================================

type noFSLoader struct{}

func (nl *noFSLoader) Read(path string) (io.Reader, error) {
	return nil, errors.New("no-fs-loader-read: filesystem access disabled")
}

func (nl *noFSLoader) Resolve(path string) (string, error) {
	return "", errors.New("no-fs-loader-resolve: filesystem access disabled")
}

func (nl *noFSLoader) Inherit(from string) (loaders.Loader, error) {
	return nil, errors.New("no-fs-loader-inherit: filesystem access disabled")
}

// =============================================================================

// newTemplateWithFixedItems creates a gonja template with a fixed items() method
// that properly returns key-value pairs (the built-in one only returns values).
func newTemplateWithFixedItems(source string) (*exec.Template, error) {
	rootID := fmt.Sprintf("root-%s", string(sha256.New().Sum([]byte(source))))

	loader, err := loaders.NewFileSystemLoader("")
	if err != nil {
		return nil, err
	}

	shiftedLoader, err := loaders.NewShiftedLoader(rootID, bytes.NewReader([]byte(source)), loader)
	if err != nil {
		return nil, err
	}

	// Create custom environment with fixed items() method
	customContext := builtins.GlobalFunctions.Inherit()
	customContext.Set("add_generation_prompt", true)
	customContext.Set("strftime_now", func(format string) string {
		return time.Now().Format("2006-01-02")
	})
	customContext.Set("raise_exception", func(msg string) (string, error) {
		return "", errors.New(msg)
	})
	// Override namespace to unwrap *exec.Value to plain Go values
	customContext.Set("namespace", func(e *exec.Evaluator, params *exec.VarArgs) map[string]any {
		ns := make(map[string]any)
		for key, value := range params.KwArgs {
			ns[key] = value.ToGoSimpleType(true)
		}
		return ns
	})

	customFilters := builtins.Filters.Update(exec.NewFilterSet(map[string]exec.FilterFunction{}))
	customFilters.Register("tojson", func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
		if in.IsError() {
			return in
		}
		// Handle lists specially to avoid unexported field issues
		if in.IsList() {
			inCast := make([]any, in.Len())
			for i := range inCast {
				item := in.Index(i)
				inCast[i] = item.ToGoSimpleType(true)
			}
			in = exec.AsValue(inCast)
		}
		params.ExpectKwArgs([]*exec.KwArg{
			{Name: "ensure_ascii", Default: exec.AsValue(true)},
			{Name: "indent", Default: exec.AsValue(nil)},
		})
		casted := in.ToGoSimpleType(true)
		if err, ok := casted.(error); ok {
			return exec.AsValue(err)
		}
		data, err := json.Marshal(casted)
		if err != nil {
			return exec.AsValue("")
		}
		return exec.AsValue(string(data))
	})
	customFilters.Register("fromjson", func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
		if in.IsError() {
			return in
		}
		if !in.IsString() {
			return in
		}
		var result any
		if err := json.Unmarshal([]byte(in.String()), &result); err != nil {
			return exec.AsValue(err)
		}
		return exec.AsValue(result)
	})
	customFilters.Register("items", func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
		if !in.IsDict() {
			return exec.AsValue([][]any{})
		}
		dict := in.ToGoSimpleType(true)
		if m, ok := dict.(map[string]any); ok {
			items := make([][]any, 0, len(m))
			for key, value := range m {
				// Use ToGoSimpleType to avoid unexported field reflection errors
				v := exec.AsValue(value).ToGoSimpleType(true)
				items = append(items, []any{key, v})
			}
			return exec.AsValue(items)
		}
		return exec.AsValue([][]any{})
	})

	env := exec.Environment{
		Context:           customContext,
		Filters:           customFilters,
		Tests:             builtins.Tests,
		ControlStructures: builtins.ControlStructures,
		Methods: exec.Methods{
			Dict: exec.NewMethodSet(map[string]exec.Method[map[string]any]{
				"keys": func(self map[string]any, selfValue *exec.Value, arguments *exec.VarArgs) (any, error) {
					if err := arguments.Take(); err != nil {
						return nil, err
					}
					keys := make([]string, 0, len(self))
					for key := range self {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					return keys, nil
				},
				"items": func(self map[string]any, selfValue *exec.Value, arguments *exec.VarArgs) (any, error) {
					if err := arguments.Take(); err != nil {
						return nil, err
					}
					// Return [][]any where each inner slice is [key, value]
					// This allows gonja to unpack: for k, v in dict.items()
					// Use ToGoSimpleType to avoid unexported field reflection errors
					items := make([][]any, 0, len(self))
					for key, value := range self {
						v := exec.AsValue(value).ToGoSimpleType(true)
						items = append(items, []any{key, v})
					}
					return items, nil
				},
			}),
			Str:   builtins.Methods.Str,
			List:  builtins.Methods.List,
			Bool:  builtins.Methods.Bool,
			Float: builtins.Methods.Float,
			Int:   builtins.Methods.Int,
		},
	}

	return exec.NewTemplate(rootID, gonja.DefaultConfig, shiftedLoader, &env)
}

func readJinjaTemplate(fileName string) (string, error) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return "", fmt.Errorf("read-jinja-template: failed to read file: %w", err)
	}

	return string(data), nil
}

// =============================================================================

// deepNormalize recursively converts all nested structures to plain Go types
// (map[string]any and []any) to avoid reflection errors in gonja templates.
// Returns plain map[string]any, NOT the named type D.
func deepNormalize(v any) map[string]any {
	switch m := v.(type) {
	case D:
		result := make(map[string]any, len(m))
		for k, val := range m {
			result[k] = deepNormalizeValue(val)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(m))
		for k, val := range m {
			result[k] = deepNormalizeValue(val)
		}
		return result
	}
	return nil
}

func deepNormalizeValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, v := range val {
			result[k] = deepNormalizeValue(v)
		}
		return result
	case D:
		result := make(map[string]any, len(val))
		for k, v := range val {
			result[k] = deepNormalizeValue(v)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, v := range val {
			result[i] = deepNormalizeValue(v)
		}
		return result
	case []D:
		result := make([]any, len(val))
		for i, v := range val {
			result[i] = deepNormalizeValue(v)
		}
		return result
	case []map[string]any:
		result := make([]any, len(val))
		for i, v := range val {
			result[i] = deepNormalizeValue(v)
		}
		return result
	default:
		return v
	}
}
