package controlStructures

import (
	"fmt"

	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/parser"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

type AutoescapeControlStructure struct {
	Wrapper    *nodes.Wrapper
	Autoescape bool
}

func (acs *AutoescapeControlStructure) Position() *tokens.Token {
	return acs.Wrapper.Position()
}
func (acs *AutoescapeControlStructure) String() string {
	t := acs.Position()
	return fmt.Sprintf("AutoescapeControlStructure(Line=%d Col=%d)", t.Line, t.Col)
}

func (acs *AutoescapeControlStructure) Execute(r *exec.Renderer, tag *nodes.ControlStructureBlock) error {
	sub := r.Inherit()
	sub.Config.AutoEscape = acs.Autoescape

	err := sub.ExecuteWrapper(acs.Wrapper)
	if err != nil {
		return err
	}

	return nil
}

func autoescapeParser(p *parser.Parser, args *parser.Parser) (nodes.ControlStructure, error) {
	cs := &AutoescapeControlStructure{}

	wrapper, _, err := p.WrapUntil("endautoescape")
	if err != nil {
		return nil, err
	}
	cs.Wrapper = wrapper

	modeToken := args.Match(tokens.Name)
	if modeToken == nil {
		return nil, args.Error("A mode is required for autoescape cs.", nil)
	}
	if modeToken.Val == "true" {
		cs.Autoescape = true
	} else if modeToken.Val == "false" {
		cs.Autoescape = false
	} else {
		return nil, args.Error("Only 'true' or 'false' is valid as an autoescape cs.", nil)
	}

	if !args.Stream().End() {
		return nil, args.Error("Malformed autoescape controlStructure args.", nil)
	}

	return cs, nil
}
