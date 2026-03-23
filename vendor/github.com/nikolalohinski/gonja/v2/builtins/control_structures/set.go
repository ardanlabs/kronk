package controlStructures

import (
	"fmt"

	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/parser"
	"github.com/nikolalohinski/gonja/v2/tokens"
	"github.com/pkg/errors"
)

type SetControlStructure struct {
	location    *tokens.Token
	target      nodes.Expression
	expression  nodes.Expression
	condition   nodes.Expression
	alternative nodes.Expression
}

func (scs *SetControlStructure) Position() *tokens.Token {
	return scs.location
}
func (scs *SetControlStructure) String() string {
	t := scs.Position()
	return fmt.Sprintf("SetControlStructure(Line=%d Col=%d)", t.Line, t.Col)
}

func (scs *SetControlStructure) Execute(r *exec.Renderer, tag *nodes.ControlStructureBlock) error {
	var value *exec.Value
	// Evaluate expression
	if scs.condition != nil && scs.alternative != nil {
		condition := r.Eval(scs.condition)
		if condition.IsError() {
			return condition
		}
		if condition.Bool() {
			value = r.Eval(scs.expression)
		} else {
			value = r.Eval(scs.alternative)
		}
	} else {
		value = r.Eval(scs.expression)
	}
	if value == nil {
		return errors.Errorf(`Invalid value in 'set' tag: %s`, scs.expression)
	}
	if value.IsError() {
		return value
	}

	switch n := scs.target.(type) {
	case *nodes.Name:
		r.Environment.Context.Set(n.Name.Val, value.Interface())
	case *nodes.GetAttribute:
		target := r.Eval(n.Node)
		if target.IsError() {
			return errors.Wrapf(target, `Unable to evaluate target %s`, n)
		}
		if err := target.Set(exec.AsValue(n.Attribute), value.Interface()); err != nil {
			return errors.Wrapf(err, `Unable to set value on "%s"`, n.Attribute)
		}
	case *nodes.GetItem:
		target := r.Eval(n.Node)
		if target.IsError() {
			return errors.Wrapf(target, `Unable to evaluate target %s`, n)
		}
		arg := r.Eval(n.Arg)
		if arg.IsError() {
			return errors.Wrapf(target, `Unable to evaluate argument %s`, n.Arg)
		}
		if err := target.Set(arg, value.Interface()); err != nil {
			return errors.Wrapf(err, `Unable to set value on "%s"`, n.Arg)
		}
	default:
		return errors.Errorf(`Illegal set target node %s`, n)
	}

	return nil
}

func setParser(p *parser.Parser, args *parser.Parser) (nodes.ControlStructure, error) {
	cs := &SetControlStructure{
		location: p.Current(),
	}

	// Parse variable name
	ident, err := args.ParseVariableOrLiteral()
	if err != nil {
		return nil, errors.Wrap(err, `unable to parse identifier`)
	}
	switch n := ident.(type) {
	case *nodes.Name, *nodes.Call, *nodes.GetItem, *nodes.GetAttribute:
		cs.target = n
	default:
		return nil, errors.Errorf(`unexpected set target %s`, n)
	}

	if args.Match(tokens.Assign) == nil {
		return nil, args.Error("Expected '='.", args.Current())
	}

	// Variable expression
	expr, err := args.ParseExpression()
	if err != nil {
		return nil, err
	}
	cs.expression = expr
	condition, alternative, err := args.ParseCondition()
	if err != nil {
		return nil, err
	}
	if condition != nil {
		cs.condition = condition
		if alternative == nil {
			return nil, args.Error("Malformed 'set' if else condition", args.Current())
		}
		cs.alternative = alternative
	}

	// Remaining arguments
	if !args.End() {
		return nil, args.Error("Malformed 'set' tag args.", args.Current())
	}

	return cs, nil
}
