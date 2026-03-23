// Package parser provides the template parser implementation.
package parser

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/nikolalohinski/gonja/v2/config"
	"github.com/nikolalohinski/gonja/v2/loaders"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

var (
	lineReturnWithOnlyWhiteSpace = regexp.MustCompile("^(\n|\r\n)[ \t]*$")
)

type ControlStructureGetter interface {
	Get(name string) (ControlStructureParser, bool)
}

// Parser provides a comprehensive and easy tool to
// work with the template document and arguments provided by
// the user for your custom tag.
//
// The parser works on a token list which will be provided by gonja.
// A token is a unit you can work with. Tokens are either of type identifier,
// string, number, keyword, HTML or symbol.
//
// (See Token's documentation for more about tokens)
type Parser struct {
	identifier        string
	stream            *tokens.Stream
	controlStructures ControlStructureGetter

	Config   *config.Config
	Template *nodes.Template
	Loader   loaders.Loader
}

func (p *Parser) Stream() *tokens.Stream {
	return p.stream
}

// NewParser creates a new parser to parse tokens.
// Used inside gonja to parse documents and to provide an easy-to-use
// parser for tag authors
func NewParser(identifier string, stream *tokens.Stream, cfg *config.Config, loader loaders.Loader, controlStructures ControlStructureGetter) *Parser {
	return &Parser{
		identifier:        identifier,
		stream:            stream,
		controlStructures: controlStructures,
		Config:            cfg,
		Loader:            loader,
	}
}

// Consume one token. It will be gone forever.
func (p *Parser) Consume() {
	p.stream.Next()
}

// Next returns and consume the current token
func (p *Parser) Next() *tokens.Token {
	return p.stream.Next()
}

func (p *Parser) End() bool {
	return p.stream.End()
}

// Match returns the CURRENT token if the given type matches.
// Consumes this token on success.
func (p *Parser) Match(types ...tokens.Type) *tokens.Token {
	tok := p.stream.Current()
	if slices.Contains(types, tok.Type) {
		p.stream.Next()
		return tok
	}
	return nil
}

func (p *Parser) MatchName(names ...string) *tokens.Token {
	t := p.Current(tokens.Name)
	if t != nil {
		if slices.Contains(names, t.Val) {
			return p.Pop()
		}
	}
	return nil
}

// Pop returns the current token and advance to the next
func (p *Parser) Pop() *tokens.Token {
	t := p.stream.Current()
	p.stream.Next()
	return t
}

// Current returns the current token without consuming
// it and only if it matches one of the given types
func (p *Parser) Current(types ...tokens.Type) *tokens.Token {
	tok := p.stream.Current()
	if types == nil {
		return tok
	}
	if slices.Contains(types, tok.Type) {
		return tok
	}
	return nil
}

func (p *Parser) Peek(types ...tokens.Type) *tokens.Token {
	tok := p.stream.Peek()
	if types == nil {
		return tok
	}
	if slices.Contains(types, tok.Type) {
		return tok
	}
	return nil
}

func (p *Parser) CurrentName(names ...string) *tokens.Token {
	t := p.Current(tokens.Name)
	if t != nil {
		if slices.Contains(names, t.Val) {
			return t
		}
	}
	return nil
}

// WrapUntil wraps all nodes between starting tag and "{% endtag %}" and provides
// one simple interface to execute the wrapped nodes.
// It returns a parser to process provided arguments to the tag.
func (p *Parser) WrapUntil(names ...string) (*nodes.Wrapper, *Parser, error) {
	wrapper := &nodes.Wrapper{
		Location: p.Current(),
		Trim:     &nodes.Trim{},
	}

	var args []*tokens.Token

	for !p.stream.End() {
		// New tag, check whether we have to stop wrapping here
		if begin := p.Match(tokens.BlockBegin); begin != nil {
			endTag := p.CurrentName(names...)

			if endTag != nil {
				p.Consume()
				for {
					if end := p.Match(tokens.BlockEnd); end != nil {
						wrapper.EndTag = endTag.Val
						if data := p.Current(tokens.Data); data != nil {
							data.Trim = data.Trim || len(end.Val) > 0 && end.Val[0] == '-'
						}
						stream := tokens.NewStream(args)
						return wrapper, NewParser(p.identifier, stream, p.Config, p.Loader, p.controlStructures), nil
					}
					if p.End() || p.Current(tokens.EOF) != nil {
						return nil, nil, p.Error("Unexpected EOF.", p.Current())
					}
					args = append(args, p.Next())
				}
			}
			p.stream.Backup()
		}

		// Otherwise process next element to be wrapped
		node, err := p.parseDocElement()
		if err != nil {
			return nil, nil, err
		}
		wrapper.Nodes = append(wrapper.Nodes, node)
	}

	return nil, nil, p.Error(fmt.Sprintf("Unexpected EOF, expected tag %s.", strings.Join(names, " or ")),
		p.Current())
}

func (p *Parser) parseDocElement() (nodes.Node, error) {
	t := p.Current()
	switch t.Type {
	case tokens.Data:
		n := &nodes.Data{
			Data:                  t,
			RemoveFirstLineReturn: t.RemoveFirstLineReturn,
			Trim: nodes.Trim{
				Left: t.Trim,
			},
		}
		if next := p.Peek(tokens.VariableBegin, tokens.CommentBegin, tokens.BlockBegin); next != nil {
			if len(next.Val) > 0 && next.Val[len(next.Val)-1] == '-' {
				n.Trim.Right = true
			}
		}
		if p.Config.LeftStripBlocks {
			if next := p.Peek(tokens.BlockBegin); next != nil {
				if len(next.Val) == 0 || next.Val[len(next.Val)-1] != '+' {
					n.RemoveTrailingWhiteSpaceFromLastLine = true
				}
			}
		}
		p.Consume()
		return n, nil
	case tokens.EOF:
		p.Consume()
		return nil, nil
	case tokens.CommentBegin:
		return p.ParseComment()
	case tokens.VariableBegin:
		return p.ParseExpressionNode()
	case tokens.BlockBegin:
		node, err := p.ParseControlStructureBlock()
		if err != nil {
			return node, err
		}
		if p.Config.TrimBlocks && !p.End() && p.Peek(tokens.BlockBegin) != nil {
			if data := p.Current(tokens.Data); data != nil && lineReturnWithOnlyWhiteSpace.MatchString(data.Val) {
				p.Consume() // Consume whitespace
			}
		}
		return node, err
	}
	return nil, p.Error("Unexpected token (only HTML/tags/filters in templates allowed)", t)
}

func (p *Parser) Parse() (*nodes.Template, error) {
	tpl := &nodes.Template{
		Identifier: p.identifier,
		Blocks:     nodes.BlockSet{},
		Macros:     map[string]*nodes.Macro{},
	}
	p.Template = tpl

	for !p.Stream().End() {
		node, err := p.parseDocElement()
		if err != nil {
			return nil, err
		}
		if node != nil {
			tpl.Nodes = append(tpl.Nodes, node)
		}
	}
	return tpl, nil
}

func (p *Parser) Extend(identifier string) (*nodes.Template, error) {
	input, err := p.Loader.Read(identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to reader template '%s': %s", identifier, err)
	}

	identifier, err = p.Loader.Resolve(identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve identifier '%s': %s", identifier, err)
	}

	source := new(strings.Builder)
	if _, err := io.Copy(source, input); err != nil {
		return nil, fmt.Errorf("failed to copy '%s' to string buffer: %s", source, err)
	}

	loader, err := p.Loader.Inherit(identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to inherit loader: %s", err)
	}

	config := p.Config.Inherit()

	parser := &Parser{
		identifier:        identifier,
		stream:            tokens.Lex(source.String(), config),
		controlStructures: p.controlStructures,
		Config:            config,
		Loader:            loader,
	}
	return parser.Parse()
}
