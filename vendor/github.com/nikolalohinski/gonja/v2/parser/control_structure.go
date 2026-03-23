package parser

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/pkg/errors"

	"github.com/nikolalohinski/gonja/v2/logging"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

type ControlStructureParser func(parser *Parser, args *Parser) (nodes.ControlStructure, error)

func (p *Parser) ParseControlStructureBlock() (*nodes.ControlStructureBlock, error) {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"current": p.Current(),
		}).Trace("ParseControlStructureBlock")
	}

	begin := p.Match(tokens.BlockBegin)
	if begin == nil {
		return nil, errors.Errorf(`Expected "%s" got "%s"`, p.Config.BlockStartString, p.Current())
	}

	name := p.Match(tokens.Name)
	if name == nil {
		return nil, p.Error("Expected a controlStructure name here", p.Current())
	}

	controlStructureParser, exists := p.controlStructures.Get(name.Val)
	if !exists {
		return nil, p.Error(fmt.Sprintf("ControlStructure '%s' not found (or beginning not provided)", name.Val), name)
	}

	if logging.Enabled() {
		log.Trace("args")
	}
	var args []*tokens.Token
	for p.Current(tokens.BlockEnd) == nil && !p.Stream().End() {
		if logging.Enabled() {
			log.Trace("for args")
		}
		args = append(args, p.Next())
	}
	if logging.Enabled() {
		log.Trace("loop ended")
	}

	end := p.Match(tokens.BlockEnd)
	if end == nil {
		return nil, p.Error(fmt.Sprintf(`Expected end of block "%s"`, p.Config.BlockEndString), p.Current())
	}
	if data := p.Current(tokens.Data); data != nil {
		data.Trim = data.Trim || len(end.Val) > 0 && end.Val[0] == '-'
		data.RemoveFirstLineReturn = p.Config.TrimBlocks && len(end.Val) > 0 && end.Val[0] != '+'
	}

	if logging.Enabled() {
		log.WithFields(log.Fields{
			"args": args,
		}).Trace("Matched end block")
	}

	stream := tokens.NewStream(args)
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"stream": stream,
		}).Trace("Got stream")
	}
	argParser := NewParser(p.identifier, stream, p.Config, p.Loader, p.controlStructures)
	if logging.Enabled() {
		log.Trace("argparser")
	}

	controlStructure, err := controlStructureParser(p, argParser)
	if err != nil {
		return nil, errors.Wrapf(err, `Unable to parse controlStructure "%s"`, name.Val)
	}
	if logging.Enabled() {
		log.Trace("got controlStructure and return")
	}
	return &nodes.ControlStructureBlock{
		Location:         begin,
		Name:             name.Val,
		ControlStructure: controlStructure,
	}, nil
}
