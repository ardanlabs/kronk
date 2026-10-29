package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kronkserver "github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
	"go.yaml.in/yaml/v3"
)

// newFile is written when the target file doesn't exist yet.
const newFile = "version: 2\n\nmodels: {}\n"

// The sections that hold model entries.
const (
	sectionModels = "models"
	sectionBucky  = "bucky-models"
	sectionMalina = "malina-models"
)

// saveEntry writes the changes into models.<id> of the file at path. Other
// entries, sections and comments are kept. The result is checked with
// modelconfig.Load before it replaces the file, and the previous file is
// kept as <path>.bak. It returns the backup path, empty when there was no
// previous file.
func saveEntry(path string, id string, changes []change) (string, error) {
	return saveSectionEntry(path, sectionModels, id, changes)
}

// saveSectionEntry is saveEntry for <section>.<id>.
func saveSectionEntry(path string, section string, id string, changes []change) (string, error) {
	return saveFile(path, func(data []byte) ([]byte, error) {
		return editSectionEntry(data, section, id, changes)
	})
}

// saveKMS writes the changes into the kms: section of the file at path, the
// same way saveEntry does for a model. The new section is checked against
// the server's config struct first.
func saveKMS(path string, changes []change) (string, error) {
	return saveFile(path, func(data []byte) ([]byte, error) {
		out, err := editKMS(data, changes)
		if err != nil {
			return nil, err
		}

		kms, err := readKMSData(out)
		if err != nil {
			return nil, err
		}
		if err := kronkserver.CheckKMS(unflatten(kms)); err != nil {
			return nil, fmt.Errorf("the server would not accept these settings, nothing was written: %w", err)
		}

		return out, nil
	})
}

// previewEntry returns the lines of models.<id> as saveEntry would write them
// to the file at path, empty when the entry would be removed.
func previewEntry(path string, id string, changes []change) (string, error) {
	return previewSectionEntry(path, sectionModels, id, changes)
}

// previewSectionEntry is previewEntry for <section>.<id>.
func previewSectionEntry(path string, section string, id string, changes []change) (string, error) {
	return preview(path, []string{section, id}, func(data []byte) ([]byte, error) {
		return editSectionEntry(data, section, id, changes)
	})
}

// previewKMS returns the kms: section as saveKMS would write it.
func previewKMS(path string, changes []change) (string, error) {
	return preview(path, []string{"kms"}, func(data []byte) ([]byte, error) {
		return editKMS(data, changes)
	})
}

// preview runs edit on the file at path, or on a new file, and returns the
// lines of the node at keys in the result, with the file's own order,
// indentation and comments.
func preview(path string, keys []string, edit func([]byte) ([]byte, error)) (string, error) {
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		data = []byte(newFile)
	case err != nil:
		return "", err
	}

	out, err := edit(data)
	if err != nil {
		return "", err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(out, &doc); err != nil {
		return "", err
	}

	node := doc.Content[0]
	var key, val *yaml.Node
	for _, k := range keys {
		if node == nil || node.Kind != yaml.MappingNode {
			return "", nil
		}
		key, val = lookup(node, k)
		node = val
	}
	if key == nil {
		return "", nil
	}

	lines := strings.SplitAfter(string(out), "\n")
	return strings.Join(lines[key.Line-1:lastLine(val)], ""), nil
}

// saveFile applies edit to the file at path, or to a new file when it doesn't
// exist, checks that the result loads, keeps the previous file as .bak and
// replaces the file. It returns the backup path, empty for a new file.
func saveFile(path string, edit func([]byte) ([]byte, error)) (string, error) {
	data, err := os.ReadFile(path)
	existed := err == nil
	switch {
	case errors.Is(err, os.ErrNotExist):
		data = []byte(newFile)
	case err != nil:
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	out, err := edit(data)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".model_config-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("write temp file: %w", err)
	}

	if _, err := modelconfig.Load(tmp.Name()); err != nil {
		return "", fmt.Errorf("the new file doesn't load, nothing was written: %w", err)
	}

	var backup string
	if existed {
		backup = path + ".bak"
		if err := os.WriteFile(backup, data, 0644); err != nil {
			return "", fmt.Errorf("write backup: %w", err)
		}
	}

	if err := os.Chmod(tmp.Name(), 0644); err != nil {
		return "", fmt.Errorf("set permissions: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", fmt.Errorf("replace %s: %w", path, err)
	}

	return backup, nil
}

// editKMS applies the changes to the kms: section. Only the lines of that
// section are rewritten. An empty section is written as kms: {}, the way the
// default file has it.
func editKMS(data []byte, changes []change) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}
	if doc.Kind == 0 {
		data = []byte(newFile)
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("config file: top level is not a mapping")
	}

	text := string(data)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	lines := strings.SplitAfter(text, "\n")
	lines = lines[:len(lines)-1]

	kKey, kVal := lookup(root, "kms")

	var end int
	if kVal != nil {
		end = lastLine(kVal)
	}

	section := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if kVal != nil && kVal.Kind == yaml.MappingNode {
		section = kVal
	}
	section.Style = 0

	if err := applyChanges(section, changes); err != nil {
		return nil, err
	}

	rendered := []string{"kms: {}\n"}
	if len(section.Content) > 0 {
		var err error
		rendered, err = renderEntry("kms", section, 0)
		if err != nil {
			return nil, err
		}
	}

	switch {
	case kKey != nil:
		return []byte(strings.Join(splice(lines, kKey.Line-1, end, rendered), "")), nil

	default:
		// No kms: yet. Put it before models: when there is one, so the server
		// settings stay above the model entries.
		at := len(lines)
		if mKey, _ := lookup(root, "models"); mKey != nil {
			at = mKey.Line - 1
			rendered = append(rendered, "\n")
		} else {
			rendered = append([]string{"\n"}, rendered...)
		}
		return []byte(strings.Join(splice(lines, at, at, rendered), "")), nil
	}
}

// editEntry applies the changes to models.<id>.
func editEntry(data []byte, id string, changes []change) ([]byte, error) {
	return editSectionEntry(data, sectionModels, id, changes)
}

// editSectionEntry applies the changes to <section>.<id>, where section is
// models, bucky-models or malina-models. Only the lines of that entry are
// rewritten; the rest of the file stays byte for byte, including blank
// lines, which a full YAML re-encode would drop.
func editSectionEntry(data []byte, section string, id string, changes []change) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if doc.Kind == 0 {
		data = []byte(newFile)
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("config file: top level is not a mapping")
	}

	text := string(data)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	lines := strings.SplitAfter(text, "\n")
	lines = lines[:len(lines)-1]

	mKey, mVal := lookup(root, section)

	var eKey, eVal *yaml.Node
	if mVal != nil && mVal.Kind == yaml.MappingNode {
		eKey, eVal = lookup(mVal, id)
	}

	// The line ranges come from the original nodes, so take them before the
	// changes edit the entry.
	var entryEnd, modelsEnd int
	if eVal != nil {
		entryEnd = lastLine(eVal)
	}
	if mVal != nil {
		modelsEnd = lastLine(mVal)
	}

	entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if eVal != nil && eVal.Kind == yaml.MappingNode {
		entry = eVal
	}
	entry.Style = 0

	if err := applyChanges(entry, changes); err != nil {
		return nil, err
	}

	indent := 2
	switch {
	case eKey != nil:
		indent = eKey.Column - 1
	case mVal != nil && mVal.Kind == yaml.MappingNode && len(mVal.Content) > 0:
		indent = mVal.Content[0].Column - 1
	}

	rendered, err := renderEntry(id, entry, indent)
	if err != nil {
		return nil, err
	}

	var out []string
	switch {
	case eKey != nil:
		out = splice(lines, eKey.Line-1, entryEnd, rendered)

	case mVal != nil && mVal.Kind == yaml.MappingNode && len(mVal.Content) > 0:
		if len(rendered) > 0 {
			rendered = append([]string{"\n"}, rendered...)
		}
		out = splice(lines, modelsEnd, modelsEnd, rendered)

	case mKey != nil && mVal.Line == mKey.Line:
		// "models: {}" or "models:" with nothing under it.
		out = splice(lines, mKey.Line-1, mKey.Line, append([]string{section + ":\n"}, rendered...))

	default:
		out = append(lines, "\n"+section+":\n")
		out = append(out, rendered...)
	}

	return []byte(strings.Join(out, "")), nil
}

func applyChanges(entry *yaml.Node, changes []change) error {
	for _, c := range changes {
		parts := strings.Split(c.key, ".")

		if c.del {
			remove(entry, parts)
			continue
		}

		parent := entry
		for _, p := range parts[:len(parts)-1] {
			parent = child(parent, p, true)
		}

		var val yaml.Node
		if err := val.Encode(c.new); err != nil {
			return fmt.Errorf("encode %s: %w", c.key, err)
		}
		set(parent, parts[len(parts)-1], &val)
	}

	return nil
}

// renderEntry encodes "id: entry" indented to sit under models. An empty
// entry renders nothing, so the entry is removed instead of written as {}.
func renderEntry(id string, entry *yaml.Node, indent int) ([]string, error) {
	if len(entry.Content) == 0 {
		return nil, nil
	}

	wrapper := &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: id},
			entry,
		},
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(wrapper); err != nil {
		return nil, fmt.Errorf("encode entry: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}

	pad := strings.Repeat(" ", indent)
	lines := strings.SplitAfter(buf.String(), "\n")
	lines = lines[:len(lines)-1]
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			lines[i] = pad + l
		}
	}

	return lines, nil
}

// splice replaces lines[from:to] with repl.
func splice(lines []string, from int, to int, repl []string) []string {
	out := make([]string, 0, len(lines)-(to-from)+len(repl))
	out = append(out, lines[:from]...)
	out = append(out, repl...)
	return append(out, lines[to:]...)
}

// lastLine is the last line (1-based) used by n and its children.
func lastLine(n *yaml.Node) int {
	last := n.Line
	if n.Kind == yaml.ScalarNode && (n.Style == yaml.LiteralStyle || n.Style == yaml.FoldedStyle) {
		last += strings.Count(strings.TrimRight(n.Value, "\n"), "\n") + 1
	}
	for _, c := range n.Content {
		last = max(last, lastLine(c))
	}
	return last
}

// lookup returns the key and value nodes for key in mapping m.
func lookup(m *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

// child returns the mapping under key, creating it when create is set. A
// flow mapping such as "models: {}" becomes a block mapping.
func child(m *yaml.Node, key string, create bool) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != key {
			continue
		}

		v := m.Content[i+1]
		if v.Kind != yaml.MappingNode {
			if !create {
				return nil
			}
			*v = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", HeadComment: v.HeadComment, LineComment: v.LineComment}
		}
		if len(v.Content) == 0 {
			v.Style = 0
		}
		return v
	}

	if !create {
		return nil
	}

	k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	v := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	m.Content = append(m.Content, k, v)
	m.Style = 0

	return v
}

func set(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			val.LineComment = m.Content[i+1].LineComment
			m.Content[i+1] = val
			return
		}
	}

	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, val)
	m.Style = 0
}

// remove deletes the key at path and any mapping left empty by it.
func remove(m *yaml.Node, path []string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != path[0] {
			continue
		}

		if len(path) > 1 {
			sub := m.Content[i+1]
			if sub.Kind != yaml.MappingNode {
				return
			}
			remove(sub, path[1:])
			if len(sub.Content) > 0 {
				return
			}
		}

		m.Content = append(m.Content[:i], m.Content[i+2:]...)
		return
	}
}
