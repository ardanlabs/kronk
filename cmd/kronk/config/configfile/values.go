package configfile

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	yamlv2 "go.yaml.in/yaml/v2"
	"go.yaml.in/yaml/v3"
)

// DecodeStrict decodes a flattened entry into dst, a config struct of the
// SDK, with its own YAML (v2) tags. An unknown key is an error.
func DecodeStrict(flat map[string]any, dst any) error {
	data, err := yaml.Marshal(Unflatten(flat))
	if err != nil {
		return err
	}
	return yamlv2.UnmarshalStrict(data, dst)
}

// FlattenYAML turns an existing entry, a config struct of the SDK, into
// dot-separated keys named by its own YAML (v2) tags.
func FlattenYAML(v any) (map[string]any, error) {
	data, err := yamlv2.Marshal(v)
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

// Unflatten turns dot-separated keys back into nested maps.
func Unflatten(flat map[string]any) map[string]any {
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

// InlineYAML renders a value on one line: lists and maps in flow style.
func InlineYAML(v any) string {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return fmt.Sprint(v)
	}
	setFlow(&n)

	out, err := yaml.Marshal(&n)
	if err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSpace(string(out))
}

func setFlow(n *yaml.Node) {
	if n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode {
		n.Style = yaml.FlowStyle
	}
	for _, c := range n.Content {
		setFlow(c)
	}
}
