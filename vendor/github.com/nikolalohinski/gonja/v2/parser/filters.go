package parser

import (
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

func (p *Parser) ParseFilter() (*nodes.FilterCall, error) {
	identToken := p.Match(tokens.Name)

	if identToken == nil {
		return nil, p.Error("filter name must be an identifier", p.Current())
	}

	filter := &nodes.FilterCall{
		Token:  identToken,
		Name:   identToken.Val,
		Args:   []nodes.Expression{},
		Kwargs: map[string]nodes.Expression{},
	}

	if p.Match(tokens.LeftParenthesis) != nil {
		if p.Current(tokens.VariableEnd) != nil {
			return nil, p.Error("filter parameter required after '('", p.stream.Current())
		}

		for p.Match(tokens.Comma) != nil || p.Match(tokens.RightParenthesis) == nil {
			// TODO: Handle multiple args and kwargs
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
				filter.Kwargs[key] = value
			} else {
				filter.Args = append(filter.Args, v)
			}
		}
	}

	return filter, nil
}
