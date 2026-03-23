package controlStructures

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/parser"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

type IncludeControlStructure struct {
	location           *tokens.Token
	filenameExpression nodes.Expression
	ignoreMissing      bool
	withContext        bool
	isEmpty            bool
}

func (ics *IncludeControlStructure) Position() *tokens.Token {
	return ics.location
}

func (ics *IncludeControlStructure) String() string {
	t := ics.Position()
	return fmt.Sprintf("IncludeControlStructure(Filename=%s Line=%d Col=%d)", ics.filenameExpression, t.Line, t.Col)
}

func (ics *IncludeControlStructure) Execute(r *exec.Renderer, tag *nodes.ControlStructureBlock) error {
	if ics.isEmpty {
		return nil
	}

	filenameValue := r.Eval(ics.filenameExpression)
	if filenameValue.IsError() {
		return errors.Wrap(filenameValue, `Unable to evaluate filename`)
	}

	filename, err := r.Loader.Resolve(filenameValue.String())
	if err != nil {
		if ics.ignoreMissing {
			return nil
		} else {
			return errors.Errorf("failed to resolve filename: %s", err)
		}
	}

	loader, err := r.Loader.Inherit(filename)
	if err != nil {
		if ics.ignoreMissing {
			return nil
		} else {
			return errors.Errorf("failed to inherit loader: %s", err)
		}
	}

	included, err := exec.NewTemplate(filename, r.Config, loader, r.Environment)
	if err != nil {
		if ics.ignoreMissing {
			return nil
		} else {
			return fmt.Errorf("unable to load template '%s': %s", filename, err)
		}
	}

	return exec.NewRenderer(r.Environment, r.Output, r.Config.Inherit(), loader, included).Execute()
}

func includeParser(p *parser.Parser, args *parser.Parser) (nodes.ControlStructure, error) {
	cs := &IncludeControlStructure{
		location: p.Current(),
	}

	filenameExpression, err := args.ParseExpression()
	if err != nil {
		return nil, err
	}
	cs.filenameExpression = filenameExpression

	if args.MatchName("ignore") != nil {
		if args.MatchName("missing") != nil {
			cs.ignoreMissing = true
		} else {
			args.Stream().Backup()
		}
	}

	if tok := args.MatchName("with", "without"); tok != nil {
		if args.MatchName("context") != nil {
			cs.withContext = tok.Val == "with"
		} else {
			args.Stream().Backup()
		}
	}

	if !args.End() {
		return nil, args.Error("Malformed 'include'-tag args.", nil)
	}

	return cs, nil
}
