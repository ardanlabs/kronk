package exec

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"

	"github.com/nikolalohinski/gonja/v2/config"
	"github.com/nikolalohinski/gonja/v2/loaders"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/parser"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

type Template struct {
	source      string
	config      *config.Config
	environment *Environment
	loader      loaders.Loader
	tokens      *tokens.Stream
	parser      *parser.Parser
	root        *nodes.Template
}

// NewTemplate creates a gonja template instance that can be executed with a given context later on
func NewTemplate(identifier string, config *config.Config, loader loaders.Loader, environment *Environment) (*Template, error) {
	input, err := loader.Read(identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to reader template '%s': %s", identifier, err)
	}

	source := new(strings.Builder)
	if _, err := io.Copy(source, input); err != nil {
		return nil, fmt.Errorf("failed to copy '%s' to string buffer: %s", source, err)
	}

	t := &Template{
		source:      source.String(),
		config:      config,
		loader:      loader,
		tokens:      tokens.Lex(source.String(), config),
		environment: environment,
	}

	t.parser = parser.NewParser(identifier, t.tokens, config, loader, environment.ControlStructures)

	root, err := t.parser.Parse()
	if err != nil {
		return nil, fmt.Errorf("failed to parse template '%s': %s", source, err)
	}
	t.root = root

	return t, nil
}

// Execute executes the template and returns the rendered content in the provided writer
func (t *Template) Execute(wr io.Writer, data *Context) error {
	if data == nil {
		data = EmptyContext()
	}

	renderer := NewRenderer(&Environment{
		Tests:             t.environment.Tests,
		Filters:           t.environment.Filters,
		ControlStructures: t.environment.ControlStructures,
		Context:           t.environment.Context.Inherit().Update(data),
		Methods:           t.environment.Methods,
	}, wr, t.config, t.loader, t)

	err := renderer.Execute()
	if err != nil {
		return errors.Wrap(err, "unable to execute template")
	}

	return nil
}

// ExecuteToString executes the template and returns the rendered content as a string
func (t *Template) ExecuteToString(data *Context) (string, error) {
	output := bytes.NewBufferString("")

	if err := t.Execute(output, data); err != nil {
		return "", err
	}

	return output.String(), nil
}

// ExecuteToBytes executes the template and returns the rendered content as bytes
func (t *Template) ExecuteToBytes(data *Context) ([]byte, error) {
	output := bytes.NewBuffer(nil)

	if err := t.Execute(output, data); err != nil {
		return nil, err
	}

	return output.Bytes(), nil
}

// Macros returns all macros available to the template
func (t *Template) Macros() map[string]*nodes.Macro {
	return t.root.Macros
}

// Root returns the root node of the template
func (t *Template) Root() *nodes.Template {
	return t.root
}
