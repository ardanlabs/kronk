package parser

import (
	"github.com/nikolalohinski/gonja/v2/tokens"
	"github.com/pkg/errors"
)

func (p *Parser) Error(message string, token *tokens.Token) error {
	if token == nil {
		return errors.New(message)
	}

	return errors.Errorf(`%s (Line: %d Col: %d, near "%s")`, message, token.Line, token.Col, token.Val)
}
