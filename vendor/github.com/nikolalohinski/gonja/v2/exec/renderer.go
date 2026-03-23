package exec

import (
	"io"
	"strings"

	"github.com/pkg/errors"

	"github.com/nikolalohinski/gonja/v2/config"
	"github.com/nikolalohinski/gonja/v2/loaders"
	"github.com/nikolalohinski/gonja/v2/nodes"
)

// Renderer is a node visitor in charge of rendering
type Renderer struct {
	Config      *config.Config
	Environment *Environment
	Loader      loaders.Loader
	Template    *Template
	RootNode    *nodes.Template
	Output      io.Writer
}

// NewRenderer initializes a new renderer
func NewRenderer(environment *Environment, wr io.Writer, config *config.Config, loader loaders.Loader, template *Template) *Renderer {
	r := &Renderer{
		Config:      config.Inherit(),
		Environment: environment,
		Template:    template,
		RootNode:    template.root,
		Output:      wr,
		Loader:      loader,
	}
	r.Environment.Context.Set("self", Self(r))
	return r
}

// Inherit creates a new sub renderer
func (r *Renderer) Inherit() *Renderer {
	sub := &Renderer{
		Config: r.Config.Inherit(),
		Environment: &Environment{
			Context:           r.Environment.Context.Inherit(),
			Tests:             r.Environment.Tests,
			Filters:           r.Environment.Filters,
			ControlStructures: r.Environment.ControlStructures,
			Methods:           r.Environment.Methods,
		},
		Template: r.Template,
		RootNode: r.RootNode,
		Output:   r.Output,
		Loader:   r.Loader,
	}
	return sub
}

// Visit implements the nodes.Visitor interface
func (r *Renderer) Visit(node nodes.Node) (nodes.Visitor, error) {
	switch n := node.(type) {
	case *nodes.Comment:
		return nil, nil
	case *nodes.Data:
		output := n.Data.Val
		if n.RemoveFirstLineReturn {
			output = strings.TrimSuffix(output, "\n")
			output = strings.TrimSuffix(output, "\r\n")
		}
		if n.Trim.Left {
			output = strings.TrimLeft(output, " \r\n\t")
		}
		if n.Trim.Right {
			output = strings.TrimRight(output, " \r\n\t")
		}
		if n.RemoveTrailingWhiteSpaceFromLastLine {
			lines := strings.Split(output, "\n")
			lines = append(lines[0:len(lines)-1], strings.TrimRight(lines[len(lines)-1], " \n\t\r"))
			output = strings.Join(lines, "\n")
		}
		_, err := io.WriteString(r.Output, output)
		return nil, err
	case *nodes.Output:
		var value *Value
		if n.Condition != nil {
			condition := r.Eval(n.Condition)
			if condition.IsError() {
				return nil, errors.Wrapf(condition, `Unable to render condition at line %d: %s`, n.Condition.Position().Line, n.Condition)
			}
			if !condition.IsNil() && condition.IsTrue() {
				value = r.Eval(n.Expression)
			} else if condition.IsNil() || !condition.IsTrue() {
				if n.Alternative != nil {
					value = r.Eval(n.Alternative)
				} else {
					return nil, nil
				}
			} else {
				return nil, errors.Wrapf(condition, `Unable to evaluation condition as boolean at line %d: %s`, n.Condition.Position().Line, n.Condition)
			}
		} else {
			value = r.Eval(n.Expression)
		}
		if value.IsError() {
			return nil, errors.Wrapf(value, `Unable to render expression at line %d: %s`, n.Expression.Position().Line, n.Expression)
		}
		var err error
		if r.Config.AutoEscape && value.IsString() && !value.Safe {
			_, err = io.WriteString(r.Output, value.Escaped())
		} else {
			_, err = io.WriteString(r.Output, value.String())
		}
		return nil, err
	case *nodes.ControlStructureBlock:
		controlStructure, ok := n.ControlStructure.(ControlStructure)
		if ok {
			if err := controlStructure.Execute(r, n); err != nil {
				return nil, errors.Wrapf(err, `Unable to execute controlStructure at line %d: %s`, n.ControlStructure.Position().Line, n.ControlStructure)
			}
		}
		return nil, nil
	default:
		return r, nil
	}
}

// ExecuteWrapper wraps the nodes.Wrapper execution logic
func (r *Renderer) ExecuteWrapper(wrapper *nodes.Wrapper) error {
	return nodes.Walk(r.Inherit(), wrapper)
}

// ExecuteIfWrapper wraps the nodes.Wrapper execution logic and updates the parent context
func (r *Renderer) ExecuteIfWrapper(wrapper *nodes.Wrapper) error {
	sub := r.Inherit()
	if err := nodes.Walk(sub, wrapper); err != nil {
		return err
	}
	r.Environment.Context.Update(sub.Environment.Context)
	return nil
}

func (r *Renderer) Execute() error {
	// Determine the parent to be executed (for template inheritance)
	root := r.RootNode
	for root.Parent != nil {
		root = root.Parent
	}

	return nodes.Walk(r, root)
}

func (r *Renderer) Evaluator() *Evaluator {
	return &Evaluator{
		Environment: r.Environment,
		Config:      r.Config,
		Loader:      r.Template.parser.Loader,
	}
}

func (r *Renderer) Eval(node nodes.Expression) *Value {
	e := r.Evaluator()
	return e.Eval(node)
}
