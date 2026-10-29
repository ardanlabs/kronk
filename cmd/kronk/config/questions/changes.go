package questions

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// Diff compares the answers of the questions that were asked with the
// existing entry. Questions that were not asked leave their keys untouched.
func Diff(qs []Question, f source.Facts, a *Answers, existing map[string]any) ([]configfile.Change, error) {
	var out []configfile.Change

	for _, q := range qs {
		if !a.Applies(f, q) {
			continue
		}

		raw := a.Value(q.Key)
		if q.Kind == KindVars {
			vc, err := varChanges(raw, existing)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", q.Key, err)
			}
			out = append(out, vc...)
			continue
		}

		for _, key := range q.Keys() {
			old, had := existing[key]

			if raw == "" {
				if q.sameAs != "" {
					continue
				}
				if had && !q.isSame(old) && !q.Secret {
					out = append(out, configfile.Change{Key: key, Old: old, Del: true})
				}
				continue
			}

			v, err := q.parse(raw)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}

			if had && sameValue(old, v) {
				continue
			}
			out = append(out, configfile.Change{Key: key, Old: old, New: v})
		}
	}

	out = fixContext(f, a, existing, out)

	// Classic speculation runs one request at a time.
	if q, ok := a.ByKey["speculation"]; ok && a.Applies(f, q) && a.Value("speculation") == "classic" {
		out = setChange(out, existing, "nseq-max", 1)
	}

	return lastPerKey(out), nil
}

// lastPerKey keeps the last change to each key, where two questions write
// the same key: value precision after key precision, or a fix after an
// answer.
func lastPerKey(changes []configfile.Change) []configfile.Change {
	var out []configfile.Change
	for i, c := range changes {
		later := slices.ContainsFunc(changes[i+1:], func(n configfile.Change) bool { return n.Key == c.Key })
		if !later {
			out = append(out, c)
		}
	}
	return out
}

// Apply returns the flattened entry after the changes.
func Apply(existing map[string]any, changes []configfile.Change) map[string]any {
	out := maps.Clone(existing)
	if out == nil {
		out = map[string]any{}
	}

	// A change replaces the whole value at its key, so flattened keys under
	// it (a map value spread over key.<sub>) go too.
	for _, c := range changes {
		for k := range out {
			if strings.HasPrefix(k, c.Key+".") {
				delete(out, k)
			}
		}
		if c.Del {
			delete(out, c.Key)
			continue
		}
		out[c.Key] = c.New
	}

	return out
}

// ToModelConfig decodes a flattened entry with the SDK's own YAML tags, so
// an unknown key or a bad enum value is an error here and not at server
// startup.
func ToModelConfig(flat map[string]any) (models.ModelConfig, error) {
	var mc models.ModelConfig
	if err := configfile.DecodeStrict(flat, &mc); err != nil {
		return models.ModelConfig{}, fmt.Errorf("invalid model config: %w", err)
	}
	return mc, nil
}

// sameValue reports whether a and b are written the same way in the file.
func sameValue(a, b any) bool {
	return configfile.InlineYAML(a) == configfile.InlineYAML(b)
}
