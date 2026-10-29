package questions

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"go.yaml.in/yaml/v3"
)

// Chat template variables are the chat-template-kwargs map of a model entry:
// any name the model's Jinja template reads, with any YAML value. The answer
// is one name=value per line, and each name is its own key in the changes,
// chat-template-kwargs.<name>, so the review shows them one by one.

// KeyTemplateVars is the key of the custom template variables question.
const KeyTemplateVars = "chat-template-kwargs"

var varName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// reservedVars can't be set here: enable_thinking has its own question and
// the server rejects chat_template_kwargs inside itself.
var reservedVars = map[string]string{
	"enable_thinking":      "set it with the thinking question",
	"chat_template_kwargs": "the server rejects it",
}

// parseVars reads name=value lines. Values are YAML: true, 3, text, [a, b].
func parseVars(s string) (map[string]any, error) {
	out := map[string]any{}

	for _, line := range SplitList(s, "\n") {
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
		if rest, ok := strings.CutPrefix(k, KeyTemplateVars+"."); ok {
			sub[rest] = v
		}
	}
	return configfile.Unflatten(sub)
}

// formatVars writes the variables as the answer text, sorted by name.
func formatVars(vars map[string]any) string {
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(vars)) {
		fmt.Fprintf(&b, "%s=%s\n", name, configfile.InlineYAML(vars[name]))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// varChanges compares the answered variables with the ones in the file.
func varChanges(raw string, existing map[string]any) ([]configfile.Change, error) {
	vars, err := parseVars(raw)
	if err != nil {
		return nil, err
	}
	old := existingVars(existing)

	var out []configfile.Change
	for _, name := range slices.Sorted(maps.Keys(old)) {
		if _, ok := vars[name]; !ok {
			out = append(out, configfile.Change{Key: KeyTemplateVars + "." + name, Old: old[name], Del: true})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(vars)) {
		prev, had := old[name]
		if had && sameValue(prev, vars[name]) {
			continue
		}
		c := configfile.Change{Key: KeyTemplateVars + "." + name, New: vars[name]}
		if had {
			c.Old = prev
		}
		out = append(out, c)
	}

	return out, nil
}
