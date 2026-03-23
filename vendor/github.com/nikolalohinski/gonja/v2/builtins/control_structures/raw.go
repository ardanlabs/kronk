package controlStructures

import (
	"fmt"
	"io"

	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/parser"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

type RawControlStructure struct {
	data *nodes.Data
}

func (rcs *RawControlStructure) Position() *tokens.Token {
	return rcs.data.Position()
}
func (rcs *RawControlStructure) String() string {
	t := rcs.Position()
	return fmt.Sprintf("RawControlStructure(Line=%d Col=%d)", t.Line, t.Col)
}

func (rcs *RawControlStructure) Execute(r *exec.Renderer, tag *nodes.ControlStructureBlock) error {
	_, err := io.WriteString(r.Output, rcs.data.Data.Val)
	return err
}

func rawParser(p *parser.Parser, args *parser.Parser) (nodes.ControlStructure, error) {
	cs := &RawControlStructure{}

	wrapper, _, err := p.WrapUntil("endraw")
	if err != nil {
		return nil, err
	}
	node := wrapper.Nodes[0]
	data, ok := node.(*nodes.Data)
	if ok {
		cs.data = data
	} else {
		return nil, p.Error("raw controlStructure can only contains a single data node", node.Position())
	}

	if !args.End() {
		return nil, args.Error("raw controlStructure doesn't accept parameters.", args.Current())
	}

	return cs, nil
}
