package config

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Chat template variables are the chat-template-kwargs map of a model entry:
// any name the model's Jinja template reads, with any YAML value. The answer
// is one name=value per line, and each name is its own key in the changes,
// chat-template-kwargs.<name>, so the review shows them one by one.

const keyTemplateVars = "chat-template-kwargs"

var varName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// definedCheck finds "name is defined" and "name is not defined" in a Jinja
// template. A name after a dot, such as message.content, is an attribute and
// not a template variable.
var definedCheck = regexp.MustCompile(`(?:^|[^.\w])([A-Za-z_]\w*)\s+is\s+(?:not\s+)?defined`)

// reservedVars can't be set here: enable_thinking has its own question and
// the server rejects chat_template_kwargs inside itself.
var reservedVars = map[string]string{
	"enable_thinking":      "set it with the thinking question",
	"chat_template_kwargs": "the server rejects it",
}

// parseVars reads name=value lines. Values are YAML: true, 3, text, [a, b].
func parseVars(s string) (map[string]any, error) {
	out := map[string]any{}

	for line := range strings.Lines(s) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		name, value, ok := strings.Cut(line, "=")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		switch {
		case !ok:
			return nil, fmt.Errorf("%q: write name=value", line)
		case !varName.MatchString(name):
			return nil, fmt.Errorf("%q: a name has letters, digits and _", name)
		case reservedVars[name] != "":
			return nil, fmt.Errorf("%s: %s", name, reservedVars[name])
		case value == "":
			return nil, fmt.Errorf("%s: enter a value", name)
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("%s: set twice", name)
		}

		var v any
		if err := yaml.Unmarshal([]byte(value), &v); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out[name] = v
	}

	return out, nil
}

// existingVars rebuilds the variables from the flattened entry, where a map
// value is spread over chat-template-kwargs.<name>.<sub> keys.
func existingVars(existing map[string]any) map[string]any {
	sub := map[string]any{}
	for k, v := range existing {
		if rest, ok := strings.CutPrefix(k, keyTemplateVars+"."); ok {
			sub[rest] = v
		}
	}
	return unflatten(sub)
}

// formatVars writes the variables as the answer text, sorted by name.
func formatVars(vars map[string]any) string {
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(vars)) {
		fmt.Fprintf(&b, "%s=%s\n", name, inlineYAML(vars[name]))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// inlineYAML renders a value on one line: lists and maps in flow style.
func inlineYAML(v any) string {
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

// varChanges compares the answered variables with the ones in the file.
func varChanges(raw string, existing map[string]any) ([]change, error) {
	vars, err := parseVars(raw)
	if err != nil {
		return nil, err
	}
	old := existingVars(existing)

	var out []change
	for _, name := range slices.Sorted(maps.Keys(old)) {
		if _, ok := vars[name]; !ok {
			out = append(out, change{key: keyTemplateVars + "." + name, old: old[name], del: true})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(vars)) {
		prev, had := old[name]
		if had && inlineYAML(prev) == inlineYAML(vars[name]) {
			continue
		}
		c := change{key: keyTemplateVars + "." + name, new: vars[name]}
		if had {
			c.old = prev
		}
		out = append(out, c)
	}

	return out, nil
}

// templateVars lists the names a chat template checks with "x is defined",
// leaving out the ones Kronk or the request fill in.
func templateVars(tpl string) []string {
	skip := map[string]bool{
		"messages": true, "tools": true, "add_generation_prompt": true, "bos_token": true, "eos_token": true,
		"enable_thinking": true, "reasoning_effort": true, "documents": true, "tool_choice": true,
	}

	var out []string
	for _, m := range definedCheck.FindAllStringSubmatch(tpl, -1) {
		name := m[1]
		if skip[name] || slices.Contains(out, name) {
			continue
		}
		out = append(out, name)
	}
	slices.Sort(out)

	return out
}
