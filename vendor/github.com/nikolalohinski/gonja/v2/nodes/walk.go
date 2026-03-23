package nodes

import (
	"github.com/pkg/errors"
)

type Visitor interface {
	Visit(node Node) (Visitor, error)
}

func Walk(v Visitor, node Node) error {
	v, err := v.Visit(node)
	if err != nil {
		return err
	}
	if v == nil {
		return nil
	}

	switch n := node.(type) {
	case *Template:
		for _, node := range n.Nodes {
			if err := Walk(v, node); err != nil {
				return err
			}
		}
	case *Wrapper:
		for _, node := range n.Nodes {
			if err := Walk(v, node); err != nil {
				return err
			}
		}
	// case *Data:
	// 	return visitor.Data(t)
	// case *Output:
	// 	return visitor.Output(t)
	default:
		return errors.Errorf("Unkown type %T", n)
	}
	return nil
}

type Inspector func(Node) bool

func (f Inspector) Visit(node Node) (Visitor, error) {
	if f(node) {
		return f, nil
	}
	return nil, nil
}

// Inspect traverses an AST in depth-first order: It starts by calling
// f(node); node must not be nil. If f returns true, Inspect invokes f
// recursively for each of the non-nil children of node, followed by a
// call of f(nil).
func Inspect(node Node, f func(Node) bool) {
	Walk(Inspector(f), node)
}

// type NoOpVisitor struct {}

// func (v *NoOpVisitor) Template(node *Template) error {
// 	return nil
// }
// func (v *NoOpVisitor) Comment(node *Template) error {
// 	return nil
// }
// func (v *NoOpVisitor) Data(node *Data) error {
// 	return nil
// }
// func (v *NoOpVisitor) Output(node *Output) error {
// 	return nil
// }
// func (v *NoOpVisitor) ControlStructure(node *ControlStructure) error {
// 	return nil
// }
