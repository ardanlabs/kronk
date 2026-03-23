package controlStructures

import (
	// "bytes"

	// "github.com/nikolalohinski/gonja/v2/exec"
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"

	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/parser"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

type FilterControlStructure struct {
	position    *tokens.Token
	bodyWrapper *nodes.Wrapper
	filterChain []*nodes.FilterCall
}

func (fcs *FilterControlStructure) Position() *tokens.Token {
	return fcs.position
}
func (fcs *FilterControlStructure) String() string {
	t := fcs.Position()
	return fmt.Sprintf("FilterControlStructure(Line=%d Col=%d)", t.Line, t.Col)
}

func (fcs *FilterControlStructure) Execute(r *exec.Renderer, tag *nodes.ControlStructureBlock) error {
	var out strings.Builder
	sub := r.Inherit()
	sub.Output = &out
	// temp := bytes.NewBuffer(make([]byte, 0, 1024)) // 1 KiB size

	err := sub.ExecuteWrapper(fcs.bodyWrapper)
	if err != nil {
		return err
	}

	value := exec.AsValue(out.String())

	for _, call := range fcs.filterChain {
		value = r.Evaluator().ExecuteFilter(call, value)
		if value.IsError() {
			return errors.Wrapf(value, `Unable to apply filter %s (Line: %d Col: %d, near %s`,
				call.Name, call.Token.Line, call.Token.Col, call.Token.Val)
		}
	}

	_, err = io.WriteString(r.Output, value.String())

	return err
}

func filterParser(p *parser.Parser, args *parser.Parser) (nodes.ControlStructure, error) {
	cs := &FilterControlStructure{
		position: p.Current(),
	}

	wrapper, _, err := p.WrapUntil("endfilter")
	if err != nil {
		return nil, err
	}
	cs.bodyWrapper = wrapper

	for !args.End() {
		filterCall, err := args.ParseFilter()
		if err != nil {
			return nil, err
		}

		cs.filterChain = append(cs.filterChain, filterCall)

		if args.Match(tokens.Pipe) == nil {
			break
		}
	}

	if !args.End() {
		return nil, p.Error("Malformed filter-tag args.", nil)
	}

	return cs, nil
}
