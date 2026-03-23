package controlStructures

import (
	"fmt"

	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/parser"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

type ExtendsControlStructure struct {
	location    *tokens.Token
	filename    string
	withContext bool
}

func (ecs *ExtendsControlStructure) Position() *tokens.Token {
	return ecs.location
}

func (ecs *ExtendsControlStructure) String() string {
	t := ecs.Position()
	return fmt.Sprintf("ExtendsControlStructure(Filename=%s Line=%d Col=%d)", ecs.filename, t.Line, t.Col)
}

func (ecs *ExtendsControlStructure) Execute(r *exec.Renderer) error {
	return nil
}

func extendsParser(p *parser.Parser, args *parser.Parser) (nodes.ControlStructure, error) {
	cs := &ExtendsControlStructure{
		location: p.Current(),
	}

	if p.Template.Parent != nil {
		return nil, args.Error("this template has already one parent", args.Current())
	}

	// var filename nodes.Node
	if filename := args.Match(tokens.String); filename != nil {
		cs.filename = filename.Val

		extended, err := p.Extend(cs.filename)
		if err != nil {
			return nil, fmt.Errorf("unable to load template '%s': %s", filename, err)
		}

		p.Template.Parent = extended
	} else {
		return nil, args.Error("tag 'extends' requires a template filename as string", args.Current())
	}

	if tok := args.MatchName("with", "without"); tok != nil {
		if args.MatchName("context") != nil {
			cs.withContext = tok.Val == "with"
		} else {
			args.Stream().Backup()
		}
	}

	if !args.End() {
		return nil, args.Error("tag 'extends' only takes 1 argument", nil)
	}

	return cs, nil
}
