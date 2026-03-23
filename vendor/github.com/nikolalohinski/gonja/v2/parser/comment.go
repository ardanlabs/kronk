package parser

import (
	"fmt"

	"github.com/nikolalohinski/gonja/v2/logging"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/tokens"
	log "github.com/sirupsen/logrus"
)

func (p *Parser) ParseComment() (*nodes.Comment, error) {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"current": p.Current(),
		}).Trace("ParseComment")
	}

	tok := p.Match(tokens.CommentBegin)
	if tok == nil {
		msg := fmt.Sprintf(`Expected '%s' , got %s`, p.Config.CommentStartString, p.Current())
		return nil, p.Error(msg, p.Current())
	}

	comment := &nodes.Comment{
		Start: tok,
	}

	tok = p.Match(tokens.Data)
	if tok == nil {
		comment.Text = ""
	} else {
		comment.Text = tok.Val
	}

	tok = p.Match(tokens.CommentEnd)
	if tok == nil {
		msg := fmt.Sprintf(`Expected '%s' , got %s`, p.Config.CommentEndString, p.Current())
		return nil, p.Error(msg, p.Current())
	}
	comment.End = tok
	if data := p.Current(tokens.Data); data != nil {
		data.Trim = data.Trim || len(comment.End.Val) > 0 && comment.End.Val[0] == '-'
	}

	if logging.Enabled() {
		log.WithFields(log.Fields{
			"node": comment,
		}).Trace("ParseComment return")
	}
	return comment, nil
}
