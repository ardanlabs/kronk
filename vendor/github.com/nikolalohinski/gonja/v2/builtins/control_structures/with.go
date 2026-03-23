package controlStructures

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/parser"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

type WithControlStructure struct {
	location *tokens.Token
	pairs    map[string]nodes.Expression
	wrapper  *nodes.Wrapper
}

func (wcs *WithControlStructure) Position() *tokens.Token {
	return wcs.location
}
func (wcs *WithControlStructure) String() string {
	t := wcs.Position()
	return fmt.Sprintf("WithControlStructure(Line=%d Col=%d)", t.Line, t.Col)
}

func (wcs *WithControlStructure) Execute(r *exec.Renderer, tag *nodes.ControlStructureBlock) error {
	sub := r.Inherit()

	for key, value := range wcs.pairs {
		val := r.Eval(value)
		if val.IsError() {
			return errors.Wrapf(val, `unable to evaluate parameter %s`, value)
		}
		sub.Environment.Context.Set(key, val)
	}

	return sub.ExecuteWrapper(wcs.wrapper)
}

func withParser(p *parser.Parser, args *parser.Parser) (nodes.ControlStructure, error) {
	cs := &WithControlStructure{
		location: p.Current(),
		pairs:    map[string]nodes.Expression{},
	}

	wrapper, endargs, err := p.WrapUntil("endwith")
	if err != nil {
		return nil, err
	}
	cs.wrapper = wrapper

	if !endargs.End() {
		return nil, endargs.Error("Arguments not allowed here.", nil)
	}

	for !args.End() {
		key := args.Match(tokens.Name)
		if key == nil {
			return nil, args.Error("Expected an identifier", args.Current())
		}
		if args.Match(tokens.Assign) == nil {
			return nil, args.Error("Expected '='.", args.Current())
		}
		value, err := args.ParseExpression()
		if err != nil {
			return nil, err
		}
		cs.pairs[key.Val] = value

		if args.Match(tokens.Comma) == nil {
			break
		}
	}

	if !args.End() {
		return nil, errors.New("")
	}

	return cs, nil
}
