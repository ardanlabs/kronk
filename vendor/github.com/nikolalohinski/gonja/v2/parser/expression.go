package parser

import (
	"fmt"

	"github.com/nikolalohinski/gonja/v2/logging"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/tokens"
	log "github.com/sirupsen/logrus"
)

// ParseFilterExpression parses an optionnal filter chain for a node
func (p *Parser) ParseFilterExpression(expr nodes.Expression) (nodes.Expression, error) {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"current":    p.Current(),
			"expression": expr,
		}).Trace("ParseFilterExpression")
	}

	if p.Current(tokens.Pipe) != nil {

		filtered := &nodes.FilteredExpression{
			Expression: expr,
		}
		for p.Match(tokens.Pipe) != nil {

			filter, err := p.ParseFilter()
			if err != nil {
				return nil, err
			}

			filtered.Filters = append(filtered.Filters, filter)
		}
		expr = filtered
	}

	if logging.Enabled() {
		log.WithFields(log.Fields{
			"expr": expr,
		}).Trace("ParseFilterExpression return")
	}
	return expr, nil
}

// ParseExpression parses an expression with optional filters
// Nested expression should call this method
func (p *Parser) ParseExpression() (nodes.Expression, error) {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"current": p.Current(),
		}).Trace("ParseExpression")
	}
	var expr nodes.Expression

	expr, err := p.ParseLogicalExpression()
	if err != nil {
		return nil, err
	}

	expr, err = p.ParseFilterExpression(expr)
	if err != nil {
		return nil, err
	}

	if logging.Enabled() {
		log.WithFields(log.Fields{
			"expr": expr,
		}).Trace("ParseExpression return")
	}
	return expr, nil
}

func (p *Parser) ParseCondition() (nodes.Expression, nodes.Expression, error) {
	var returnedCondition, returnedAlternative nodes.Expression
	if p.MatchName("if") != nil {
		condition, err := p.ParseExpression()
		if err != nil {
			return nil, nil, err
		}
		if condition == nil {
			return nil, nil, p.Error("Expected a condition", p.Current())
		}
		returnedCondition = condition

		if p.MatchName("else") != nil {
			alternative, err := p.ParseExpression()
			if err != nil {
				return nil, nil, err
			}
			if alternative == nil {
				return nil, nil, p.Error("Expected an alternative", p.Current())
			}
			returnedAlternative = alternative
		}
	}

	return returnedCondition, returnedAlternative, nil
}

func (p *Parser) ParseExpressionNode() (nodes.Node, error) {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"current": p.Current(),
		}).Trace("ParseExpressionNode")
	}

	tok := p.Match(tokens.VariableBegin)
	if tok == nil {
		return nil, p.Error(fmt.Sprintf("'%s' expected here", p.Config.VariableStartString), p.Current())
	}

	node := &nodes.Output{
		Start: tok,
	}

	expr, err := p.ParseExpression()
	if err != nil {
		return nil, err
	}
	if expr == nil {
		return nil, p.Error("Expected an expression.", p.Current())
	}
	node.Expression = expr

	condition, alternative, err := p.ParseCondition()
	if err != nil {
		return nil, err
	}
	if condition != nil {
		node.Condition = condition
	}
	if alternative != nil {
		node.Alternative = alternative
	}
	tok = p.Match(tokens.VariableEnd)
	if tok == nil {
		return nil, p.Error(fmt.Sprintf("'%s' expected here", p.Config.VariableEndString), p.Current())
	}
	node.End = tok
	if data := p.Current(tokens.Data); data != nil {
		data.Trim = data.Trim || len(node.End.Val) > 0 && node.End.Val[0] == '-'
	}

	if logging.Enabled() {
		log.WithFields(log.Fields{
			"node": node,
		}).Trace("parseExpressionNode return")
	}
	return node, nil
}
