package jinja

import (
	"fmt"
	"strings"
)

// Template is a compiled Jinja template ready for rendering.
type Template struct {
	nodes []node
	src   string
}

// Compile parses a Jinja template source string and returns a compiled
// template that can be rendered multiple times with different data.
func Compile(source string) (*Template, error) {
	segs, err := scan(source)
	if err != nil {
		return nil, fmt.Errorf("jinja scan: %w", err)
	}

	nodes, err := parseTemplate(segs)
	if err != nil {
		return nil, fmt.Errorf("jinja parse: %w", err)
	}

	return &Template{nodes: nodes, src: source}, nil
}

// Render executes the template with the provided data and returns the
// rendered string. Each call creates an isolated scope so templates are
// safe to render concurrently after compilation.
func (t *Template) Render(data map[string]any) (string, error) {
	// Convert Go data to Value types.
	vals := make(map[string]Value, len(data))
	for k, v := range data {
		vals[k] = FromGoValue(v)
	}

	return render(t.nodes, vals)
}

// RenderValues is like Render but accepts pre-converted Value data.
func (t *Template) RenderValues(data map[string]Value) (string, error) {
	return render(t.nodes, data)
}

// =============================================================================

// render executes the AST with the given data and returns the result.
func render(nodes []node, data map[string]Value) (string, error) {
	e := evaluator{
		output: strings.Builder{},
	}

	// Create builtins scope as root.
	e.builtins = newScope(nil)
	registerBuiltins(e.builtins)

	// Create user data scope on top of builtins.
	e.scope = newScope(e.builtins)
	for k, v := range data {
		e.scope.set(k, v)
	}

	if err := e.execNodes(nodes); err != nil {
		return "", err
	}

	return e.output.String(), nil
}
