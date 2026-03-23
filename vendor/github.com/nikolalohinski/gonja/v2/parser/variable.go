package parser

import (
	// "fmt"

	"strconv"
	"strings"

	"github.com/nikolalohinski/gonja/v2/logging"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/tokens"
	log "github.com/sirupsen/logrus"
)

func (p *Parser) parseNumber() (nodes.Expression, error) {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"current": p.Current(),
		}).Trace("parseNumber")
	}
	t := p.Match(tokens.Integer, tokens.Float)
	if t == nil {
		return nil, p.Error("Expected a number", t)
	}

	if t.Type == tokens.Integer {
		i, err := strconv.Atoi(t.Val)
		if err != nil {
			return nil, p.Error(err.Error(), t)
		}
		nr := &nodes.Integer{
			Location: t,
			Val:      i,
		}
		return nr, nil
	} else {
		f, err := strconv.ParseFloat(t.Val, 64)
		if err != nil {
			return nil, p.Error(err.Error(), t)
		}
		fr := &nodes.Float{
			Location: t,
			Val:      f,
		}
		return fr, nil
	}
}

func (p *Parser) parseString() (nodes.Expression, error) {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"current": p.Current(),
		}).Trace("parseString")
	}
	t := p.Match(tokens.String)
	if t == nil {
		return nil, p.Error("Expected a string", t)
	}
	str := strconv.Quote(t.Val)
	replaced := strings.Replace(str, `\\`, "\\", -1)
	newstr, err := strconv.Unquote(replaced)
	if err != nil {
		return nil, p.Error(err.Error(), t)
	}
	sr := &nodes.String{
		Location: t,
		Val:      newstr,
	}
	return sr, nil
}

func (p *Parser) parseCollectionOrExpression() (nodes.Expression, error) {
	switch p.Current().Type {
	case tokens.LeftBracket:
		return p.parseList()
	case tokens.LeftParenthesis:
		return p.parseTupleOrExpression()
	case tokens.LeftBrace:
		return p.parseDict()
	default:
		return nil, nil
	}
}

func (p *Parser) parseList() (nodes.Expression, error) {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"current": p.Current(),
		}).Trace("parseList")
	}
	t := p.Match(tokens.LeftBracket)
	if t == nil {
		return nil, p.Error("Expected [", t)
	}

	if p.Match(tokens.RightBracket) != nil {
		// Empty list
		return &nodes.List{Location: t, Val: []nodes.Expression{}}, nil
	}

	expr, err := p.ParseExpression()
	if err != nil {
		return nil, err
	}
	list := []nodes.Expression{expr}

	for p.Match(tokens.Comma) != nil {
		if p.Current(tokens.RightBracket) != nil {
			// Trailing coma
			break
		}
		expr, err := p.ParseExpression()
		if err != nil {
			return nil, err
		}
		if expr == nil {
			return nil, p.Error("Expected a value", p.Current())
		}
		list = append(list, expr)
	}

	if p.Match(tokens.RightBracket) == nil {
		return nil, p.Error("Expected ]", p.Current())
	}

	return &nodes.List{Location: t, Val: list}, nil
}

func (p *Parser) parseTupleOrExpression() (nodes.Expression, error) {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"current": p.Current(),
		}).Trace("parseTuple")
	}
	t := p.Match(tokens.LeftParenthesis)
	if t == nil {
		return nil, p.Error("Expected (", t)
	}
	if p.Match(tokens.RightParenthesis) != nil {
		return &nodes.Tuple{Location: t, Val: []nodes.Expression{}}, nil
	}
	expression, err := p.ParseExpression()
	if err != nil {
		return nil, err
	}
	list := []nodes.Expression{expression}

	trailingComa := false

	// If it's a tuple
	for p.Match(tokens.Comma) != nil {
		if p.Current(tokens.RightParenthesis) != nil {
			// Trailing coma
			trailingComa = true
			break
		}
		expr, err := p.ParseExpression()
		if err != nil {
			return nil, err
		}
		if expr == nil {
			return nil, p.Error("Expected a value", p.Current())
		}
		list = append(list, expr)
	}

	if p.Match(tokens.RightParenthesis) == nil {
		return nil, p.Error("Unbalanced parenthesis", t)
	}

	if len(list) > 1 || trailingComa {
		expression = &nodes.Tuple{Location: t, Val: list}
	}
	if t := p.Match(tokens.Dot, tokens.LeftBracket); t != nil {
		return p.ParseGetter(t, expression)
	}
	return expression, nil
}

func (p *Parser) parsePair() (*nodes.Pair, error) {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"current": p.Current(),
		}).Trace("parsePair")
	}
	key, err := p.ParseExpression()
	if err != nil {
		return nil, err
	}

	if p.Match(tokens.Colon) == nil {
		return nil, p.Error("Expected \":\"", p.Current())
	}
	value, err := p.ParseExpression()
	if err != nil {
		return nil, err
	}
	return &nodes.Pair{
		Key:   key,
		Value: value,
	}, nil
}

func (p *Parser) parseDict() (nodes.Expression, error) {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"current": p.Current(),
		}).Trace("parseDict")
	}
	t := p.Match(tokens.LeftBrace)
	if t == nil {
		return nil, p.Error("Expected {", t)
	}

	dict := &nodes.Dict{
		Token: t,
		Pairs: []*nodes.Pair{},
	}

	if p.Current(tokens.RightBrace) == nil {
		pair, err := p.parsePair()
		if err != nil {
			return nil, err
		}
		dict.Pairs = append(dict.Pairs, pair)
	}

	for p.Match(tokens.Comma) != nil {
		pair, err := p.parsePair()
		if err != nil {
			return nil, err
		}
		dict.Pairs = append(dict.Pairs, pair)
	}

	if p.Match(tokens.RightBrace) == nil {
		return nil, p.Error("Expected }", p.Current())
	}

	return dict, nil
}

func (p *Parser) ParseVariable() (nodes.Expression, error) {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"current": p.Current(),
		}).Trace("ParseVariable")
	}

	t := p.Match(tokens.Name)
	if t == nil {
		return nil, p.Error("Expected an identifier.", t)
	}

	switch t.Val {
	case "true", "True":
		br := &nodes.Bool{
			Location: t,
			Val:      true,
		}
		return br, nil
	case "nil", "None":
		br := &nodes.None{
			Location: t,
		}
		return br, nil
	case "false", "False":
		br := &nodes.Bool{
			Location: t,
			Val:      false,
		}
		return br, nil
	}

	return &nodes.Name{Name: t}, nil
}

func (p *Parser) ParseGetter(accessor *tokens.Token, from nodes.Expression) (nodes.Expression, error) {
	if accessor.Type == tokens.Dot {
		getAttributeNode := &nodes.GetAttribute{
			Location: accessor,
			Node:     from,
		}
		tok := p.Match(tokens.Name, tokens.Integer)
		if tok == nil {
			return nil, p.Error("expected name or integer", p.Current())
		}
		switch tok.Type {
		case tokens.Name:
			getAttributeNode.Attribute = tok.Val
		case tokens.Integer:
			i, err := strconv.Atoi(tok.Val)
			if err != nil {
				return nil, p.Error(err.Error(), tok)
			}
			getAttributeNode.Index = i
		default:
			return nil, p.Error("this token is not allowed within a variable name", p.Current())
		}
		return getAttributeNode, nil
	} else if accessor.Type == tokens.LeftBracket {
		var argument nodes.Node
		if p.Current(tokens.Colon, tokens.RightBracket) == nil {
			expression, err := p.ParseExpression()
			if err != nil {
				return nil, p.Error("invalid expression", p.Current())
			}
			argument = expression
		}
		if p.Match(tokens.RightBracket) != nil {
			return &nodes.GetItem{
				Location: accessor,
				Node:     from,
				Arg:      argument,
			}, nil
		}
		if p.Match(tokens.Colon) != nil {
			var secondArgument nodes.Node
			if p.Current(tokens.RightBracket) == nil {
				expression, err := p.ParseExpression()
				if err != nil {
					return nil, p.Error("Invalid expression", p.Current())
				}
				secondArgument = expression
			}
			if p.Match(tokens.RightBracket) == nil {
				return nil, p.Error("unbalanced bracket", accessor)
			}
			return &nodes.GetSlice{
				Location: accessor,
				Node:     from,
				Start:    argument,
				End:      secondArgument,
			}, nil
		}
		return nil, p.Error("unbalanced bracket", accessor)
	}

	return nil, p.Error("unknown accessor for defining a getter", accessor)
}

// ParseVariableOrLiteral parses IDENT | IDENT.(IDENT|NUMBER)...
func (p *Parser) ParseVariableOrLiteral() (nodes.Expression, error) {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"current": p.Current(),
		}).Trace("ParseVariableOrLiteral")
	}
	t := p.Current()

	if t == nil {
		return nil, p.Error("unexpected EOF, expected a number, string, keyword or identifier.", p.Current())
	}
	var ident nodes.Node
	var err error
	switch t.Type {
	case tokens.Integer, tokens.Float:
		ident, err = p.parseNumber()
	case tokens.String:
		ident, err = p.parseString()
	case tokens.LeftParenthesis, tokens.LeftBrace, tokens.LeftBracket:
		ident, err = p.parseCollectionOrExpression()
	case tokens.Name:
		ident, err = p.ParseVariable()
	default:
		return nil, p.Error("expected either a number, string, keyword or identifier.", t)
	}
	if err != nil {
		return nil, err
	}

	var parent nodes.Node
	for !p.Stream().EOF() {
		if accessor := p.Match(tokens.Dot, tokens.LeftBracket); accessor != nil {
			getter, err := p.ParseGetter(accessor, ident)
			if err != nil {
				return nil, err
			}
			parent = ident
			ident = getter
			continue
		} else if leftParenthesis := p.Match(tokens.LeftParenthesis); leftParenthesis != nil {
			call := &nodes.Call{
				Location: leftParenthesis,
				Func:     ident,
				Args:     []nodes.Expression{},
				Kwargs:   map[string]nodes.Expression{},
				Parent:   parent,
			}

			for p.Match(tokens.Comma) != nil || p.Match(tokens.RightParenthesis) == nil {
				v, err := p.ParseExpression()
				if err != nil {
					return nil, err
				}

				if p.Match(tokens.Assign) != nil {
					key := v.Position().Val
					value, errValue := p.ParseExpression()
					if errValue != nil {
						return nil, errValue
					}
					call.Kwargs[key] = value
				} else {
					call.Args = append(call.Args, v)
				}
			}
			ident = call
			continue
		}
		break
	}

	return ident, nil
}
