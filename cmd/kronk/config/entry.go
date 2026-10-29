package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/tools/models"
	yamlv2 "go.yaml.in/yaml/v2"
	"go.yaml.in/yaml/v3"
)

// change is one difference between the file and the answers.
type change struct {
	key string
	old any
	new any
	del bool
}

// diff compares the answers of the questions that were asked with the
// existing entry. Questions that were not asked leave their keys untouched.
func diff(qs []question, f facts, a *answers, existing map[string]any) ([]change, error) {
	var out []change

	for _, q := range qs {
		if !a.applies(f, q) {
			continue
		}

		raw := a.value(q.key)
		if q.kind == kindVars {
			vc, err := varChanges(raw, existing)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", q.key, err)
			}
			out = append(out, vc...)
			continue
		}

		for _, key := range q.keys() {
			old, had := existing[key]

			if raw == "" {
				if q.sameAs != "" {
					continue
				}
				if had && fmt.Sprint(old) != q.same && !q.secret {
					out = append(out, change{key: key, old: old, del: true})
				}
				continue
			}

			v, err := q.parse(raw)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}

			if had && inlineYAML(old) == inlineYAML(v) {
				continue
			}
			out = append(out, change{key: key, old: old, new: v})
		}
	}

	out = fixContext(f, a, existing, out)

	// Classic speculation runs one request at a time.
	if a.value("speculation") == "classic" {
		out = setChange(out, existing, "nseq-max", 1)
	}

	return lastPerKey(out), nil
}

// lastPerKey keeps the last change to each key, where two questions write
// the same key: value precision after key precision, or a fix after an
// answer.
func lastPerKey(changes []change) []change {
	var out []change
	for i, c := range changes {
		later := slices.ContainsFunc(changes[i+1:], func(n change) bool { return n.key == c.key })
		if !later {
			out = append(out, c)
		}
	}
	return out
}

// apply returns the flattened entry after the changes.
func apply(existing map[string]any, changes []change) map[string]any {
	out := maps.Clone(existing)
	if out == nil {
		out = map[string]any{}
	}

	// A change replaces the whole value at its key, so flattened keys under
	// it (a map value spread over key.<sub>) go too.
	for _, c := range changes {
		for k := range out {
			if strings.HasPrefix(k, c.key+".") {
				delete(out, k)
			}
		}
		if c.del {
			delete(out, c.key)
			continue
		}
		out[c.key] = c.new
	}

	return out
}

// toModelConfig decodes a flattened entry with the SDK's own YAML tags, so
// an unknown key or a bad enum value is an error here and not at server
// startup.
func toModelConfig(flat map[string]any) (models.ModelConfig, error) {
	data, err := yaml.Marshal(unflatten(flat))
	if err != nil {
		return models.ModelConfig{}, err
	}

	var mc models.ModelConfig
	if err := yamlv2.UnmarshalStrict(data, &mc); err != nil {
		return models.ModelConfig{}, fmt.Errorf("invalid model config: %w", err)
	}

	return mc, nil
}

// flattenConfig turns an existing entry into dot-separated keys.
func flattenConfig(mc models.ModelConfig) (map[string]any, error) {
	data, err := yamlv2.Marshal(mc)
	if err != nil {
		return nil, err
	}

	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	out := map[string]any{}
	flatten("", m, out)

	return out, nil
}

func flatten(prefix string, m map[string]any, out map[string]any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		if sub, ok := v.(map[string]any); ok {
			flatten(key, sub, out)
			continue
		}
		out[key] = v
	}
}

func unflatten(flat map[string]any) map[string]any {
	out := map[string]any{}

	for _, key := range slices.Sorted(maps.Keys(flat)) {
		parts := strings.Split(key, ".")
		m := out
		for _, p := range parts[:len(parts)-1] {
			sub, ok := m[p].(map[string]any)
			if !ok {
				sub = map[string]any{}
				m[p] = sub
			}
			m = sub
		}
		m[parts[len(parts)-1]] = flat[key]
	}

	return out
}
