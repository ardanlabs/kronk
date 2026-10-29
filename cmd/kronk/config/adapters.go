package config

import (
	"fmt"
	"io/fs"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// LoRA adapters are the adapters list of a model entry. The answer is one
// adapter per line: an ID from the <base>/lora folder (the file name without
// .gguf) or an absolute path, with an optional =scale for its strength.

// parseAdapters reads adapter lines into the list written to the file.
func parseAdapters(s string) ([]any, error) {
	var out []any
	var seen []string
	for line := range strings.Lines(s) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		name, scale, hasScale := strings.Cut(line, "=")
		name = strings.TrimSpace(name)

		entry := map[string]any{}
		switch {
		case filepath.IsAbs(name) && !strings.HasSuffix(name, ".gguf"):
			return nil, fmt.Errorf("%q: an adapter file ends in .gguf", name)
		case filepath.IsAbs(name):
			entry["path"] = name
		case strings.HasPrefix(name, "~") || strings.HasPrefix(name, "."):
			return nil, fmt.Errorf("%q: use an absolute path or an adapter from the lora folder", name)
		default:
			entry["id"] = strings.TrimSuffix(name, ".gguf")
		}

		if hasScale {
			n, err := strconv.ParseFloat(strings.TrimSpace(scale), 64)
			if err != nil || n < 0 || math.IsInf(n, 0) || math.IsNaN(n) {
				return nil, fmt.Errorf("%s: the scale must be a number of 0 or more", name)
			}
			entry["scale"] = n
		}

		if slices.Contains(seen, name) {
			return nil, fmt.Errorf("%s: listed twice", name)
		}
		seen = append(seen, name)
		out = append(out, entry)
	}
	return out, nil
}

// formatAdapters writes the adapters list of the file as answer lines.
func formatAdapters(v any) string {
	list, _ := v.([]any)

	var lines []string
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		name := fmt.Sprint(m["id"])
		if p, ok := m["path"]; ok {
			name = fmt.Sprint(p)
		}
		if scale, ok := m["scale"]; ok {
			name += "=" + fmt.Sprint(scale)
		}
		lines = append(lines, name)
	}
	return strings.Join(lines, "\n")
}

// loraAdapters lists the adapter IDs in <base>/lora: GGUF file paths relative
// to the folder, without .gguf.
func loraAdapters(dir string) []string {
	var out []string
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".gguf") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err == nil {
			out = append(out, filepath.ToSlash(strings.TrimSuffix(rel, ".gguf")))
		}
		return nil
	})
	return out
}
