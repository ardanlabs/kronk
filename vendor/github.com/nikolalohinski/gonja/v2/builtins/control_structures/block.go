package controlStructures

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/parser"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

type BlockControlStructure struct {
	location *tokens.Token
	name     string
}

func (bcs *BlockControlStructure) Position() *tokens.Token {
	return bcs.location
}
func (bcs *BlockControlStructure) String() string {
	t := bcs.Position()
	return fmt.Sprintf("BlockControlStructure(Line=%d Col=%d)", t.Line, t.Col)
}

func (bcs *BlockControlStructure) Execute(r *exec.Renderer, tag *nodes.ControlStructureBlock) error {
	blocks := r.RootNode.GetBlocks(bcs.name)
	block, blocks := blocks[0], blocks[1:]

	if block == nil {
		return errors.Errorf(`Unable to find block "%s"`, bcs.name)
	}

	sub := r.Inherit()
	infos := &BlockInfos{Block: bcs, Renderer: sub, Blocks: blocks}

	sub.Environment.Context.Set("super", infos.super)
	sub.Environment.Context.Set("self", exec.Self(sub))

	err := sub.ExecuteWrapper(block)
	if err != nil {
		return err
	}

	return nil
}

type BlockInfos struct {
	Block    *BlockControlStructure
	Renderer *exec.Renderer
	Blocks   []*nodes.Wrapper
	Root     *nodes.Template
}

func (bi *BlockInfos) super() string {
	if len(bi.Blocks) <= 0 {
		return ""
	}
	r := bi.Renderer
	block, blocks := bi.Blocks[0], bi.Blocks[1:]
	sub := r.Inherit()
	var out strings.Builder
	sub.Output = &out
	infos := &BlockInfos{
		Block:    bi.Block,
		Renderer: sub,
		Blocks:   blocks,
	}
	sub.Environment.Context.Set("self", exec.Self(sub))
	sub.Environment.Context.Set("super", infos.super)
	sub.ExecuteWrapper(block)
	return out.String()
}

func blockParser(p *parser.Parser, args *parser.Parser) (nodes.ControlStructure, error) {
	block := &BlockControlStructure{
		location: p.Current(),
	}
	if args.End() {
		return nil, errors.New("Tag 'block' requires an identifier.")
	}

	name := args.Match(tokens.Name)
	if name == nil {
		return nil, errors.New("First argument for tag 'block' must be an identifier.")
	}

	if !args.End() {
		return nil, errors.New("Tag 'block' takes exactly 1 argument (an identifier).")
	}

	wrapper, endargs, err := p.WrapUntil("endblock")
	if err != nil {
		return nil, err
	}
	if !endargs.End() {
		endName := endargs.Match(tokens.Name)
		if endName != nil {
			if name.Val != endName.Val {
				return nil, errors.Errorf(`Name for 'endblock' must equal to 'block'-tag's name ('%s' != '%s').`,
					name.Val, endName.Val)
			}
		}

		if endName == nil || !endargs.End() {
			return nil, errors.New("Either no or only one argument (identifier) allowed for 'endblock'.")
		}
	}

	if !p.Template.Blocks.Exists(name.Val) {
		p.Template.Blocks.Register(name.Val, wrapper)
	} else {
		return nil, args.Error(fmt.Sprintf("Block named '%s' already defined", name.Val), nil)
	}

	block.name = name.Val
	return block, nil
}
