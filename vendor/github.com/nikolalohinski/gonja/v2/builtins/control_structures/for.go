package controlStructures

import (
	"fmt"
	"math"

	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/parser"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

type ForControlStructure struct {
	Key             string
	Value           string // only for maps: for key, value in map
	ObjectEvaluator nodes.Expression
	IfCondition     nodes.Expression

	BodyWrapper  *nodes.Wrapper
	EmptyWrapper *nodes.Wrapper
}

func (fcs *ForControlStructure) Position() *tokens.Token {
	return fcs.BodyWrapper.Position()
}
func (fcs *ForControlStructure) String() string {
	t := fcs.Position()
	return fmt.Sprintf("ForControlStructure(Line=%d Col=%d)", t.Line, t.Col)
}

type LoopInfos struct {
	index     int
	index0    int
	revindex  int
	revindex0 int
	first     bool
	last      bool
	PrevItem  *exec.Value
	NextItem  *exec.Value
	lastValue *exec.Value
}

func (li *LoopInfos) Cycle(va *exec.VarArgs) *exec.Value {
	return va.Args[int(math.Mod(float64(li.index0), float64(len(va.Args))))]
}

func (li *LoopInfos) Changed(value *exec.Value) bool {
	same := li.lastValue != nil && value.EqualValueTo(li.lastValue)
	li.lastValue = value
	return !same
}

func (fcs *ForControlStructure) Execute(r *exec.Renderer, tag *nodes.ControlStructureBlock) (forError error) {
	obj := r.Eval(fcs.ObjectEvaluator)
	if obj.IsError() {
		return obj
	}

	// Create loop struct
	items := exec.NewDict()

	// First iteration: filter values to ensure proper LoopInfos
	obj.Iterate(func(idx, count int, key, value *exec.Value) bool {
		sub := r.Inherit()
		ctx := sub.Environment.Context
		pair := &exec.Pair{}

		// There's something to iterate over (correct type and at least 1 item)
		// Update loop infos and public context
		if fcs.Value != "" && !key.IsString() && key.Len() == 2 {
			key.Iterate(func(idx, count int, key, value *exec.Value) bool {
				switch idx {
				case 0:
					ctx.Set(fcs.Key, key)
					pair.Key = key
				case 1:
					ctx.Set(fcs.Value, key)
					pair.Value = key
				}
				return true
			}, func() {})
		} else {
			ctx.Set(fcs.Key, key)
			pair.Key = key
			if value != nil {
				ctx.Set(fcs.Value, value)
				pair.Value = value
			}
		}

		if fcs.IfCondition != nil {
			if !sub.Eval(fcs.IfCondition).IsTrue() {
				return true
			}
		}
		items.Pairs = append(items.Pairs, pair)
		return true
	}, func() {})

	// 2nd pass: all values are defined, render
	length := len(items.Pairs)
	loop := &LoopInfos{
		first:  true,
		index0: -1,
	}
	if len(items.Pairs) == 0 && fcs.EmptyWrapper != nil {
		if err := r.Inherit().ExecuteWrapper(fcs.EmptyWrapper); err != nil {
			return err
		}
	}
	for idx, pair := range items.Pairs {
		sub := r.Inherit()
		ctx := sub.Environment.Context

		ctx.Set(fcs.Key, pair.Key)
		if pair.Value != nil {
			ctx.Set(fcs.Value, pair.Value)
		}

		ctx.Set("loop", loop)
		loop.index0 = idx
		loop.index = loop.index0 + 1
		if idx == 1 {
			loop.first = false
		}
		if idx+1 == length {
			loop.last = true
		}
		loop.revindex = length - idx
		loop.revindex0 = length - (idx + 1)

		if idx == 0 {
			loop.PrevItem = exec.AsValue(nil)
		} else {
			pp := items.Pairs[idx-1]
			if pp.Value != nil {
				loop.PrevItem = exec.AsValue([2]*exec.Value{pp.Key, pp.Value})
			} else {
				loop.PrevItem = pp.Key
			}
		}

		if idx == length-1 {
			loop.NextItem = exec.AsValue(nil)
		} else {
			np := items.Pairs[idx+1]
			if np.Value != nil {
				loop.NextItem = exec.AsValue([2]*exec.Value{np.Key, np.Value})
			} else {
				loop.NextItem = np.Key
			}
		}

		// Render elements with updated context
		err := sub.ExecuteWrapper(fcs.BodyWrapper)
		if err != nil {
			return err
		}
	}

	return forError
}

func forParser(p *parser.Parser, args *parser.Parser) (nodes.ControlStructure, error) {
	cs := &ForControlStructure{}

	// Arguments parsing
	var valueToken *tokens.Token
	keyToken := args.Match(tokens.Name)
	if keyToken == nil {
		return nil, args.Error("Expected an key identifier as first argument for 'for'-tag", nil)
	}

	if args.Match(tokens.Comma) != nil {
		// Value name is provided
		valueToken = args.Match(tokens.Name)
		if valueToken == nil {
			return nil, args.Error("Value name must be an identifier.", nil)
		}
	}

	if args.Match(tokens.In) == nil {
		return nil, args.Error("Expected keyword 'in'.", nil)
	}

	objectEvaluator, err := args.ParseExpression()
	if err != nil {
		return nil, err
	}
	cs.ObjectEvaluator = objectEvaluator
	cs.Key = keyToken.Val
	if valueToken != nil {
		cs.Value = valueToken.Val
	}

	if args.MatchName("if") != nil {
		ifCondition, err := args.ParseExpression()
		if err != nil {
			return nil, err
		}
		cs.IfCondition = ifCondition
	}

	if !args.End() {
		return nil, args.Error("Malformed for-loop args.", nil)
	}

	// Body wrapping
	wrapper, endargs, err := p.WrapUntil("else", "endfor")
	if err != nil {
		return nil, err
	}
	cs.BodyWrapper = wrapper

	if !endargs.End() {
		return nil, endargs.Error("Arguments not allowed here.", nil)
	}

	if wrapper.EndTag == "else" {
		// if there's an else in the if-cs, we need the else-Block as well
		wrapper, endargs, err = p.WrapUntil("endfor")
		if err != nil {
			return nil, err
		}
		cs.EmptyWrapper = wrapper

		if !endargs.End() {
			return nil, endargs.Error("Arguments not allowed here.", nil)
		}
	}

	return cs, nil
}
