package tokens

import "fmt"

type Stream struct {
	it       TokenIterator
	previous *Token
	current  *Token
	next     *Token
	backup   *Token
	buffer   []*Token
	tokens   []*Token
}

type TokenIterator interface {
	Next() *Token
}

type chanIterator struct {
	input chan *Token
}

func ChanIterator(input chan *Token) TokenIterator {
	return &chanIterator{input}
}

func (ci *chanIterator) Next() *Token {
	return <-ci.input
}

type sliceIterator struct {
	input []*Token
	idx   int
}

func SliceIterator(input []*Token) TokenIterator {
	length := len(input)
	var last *Token
	if length > 0 {
		last = input[length-1]
	}
	if last == nil || last.Type != EOF {
		input = append(input, &Token{Type: EOF})
	}
	return &sliceIterator{input, 0}
}

func (si *sliceIterator) Next() *Token {
	if si.idx < len(si.input) {
		tok := si.input[si.idx]
		si.idx++
		return tok
	} else {
		return nil
	}
}

func NewStream(input any) *Stream {
	var it TokenIterator

	switch t := input.(type) {
	case chan *Token:
		it = ChanIterator(t)
	case []*Token:
		it = SliceIterator(t)
	default:
		panic(fmt.Sprintf(`Unsupported stream input type "%T"`, t))
	}

	s := &Stream{
		it:     it,
		buffer: []*Token{},
		tokens: []*Token{},
	}
	s.init()
	return s
}

func (s *Stream) init() {
	s.current = s.nonIgnored()
	if !s.End() {
		s.next = s.nonIgnored()
	}
}

func (s *Stream) nonIgnored() *Token {
	var tok *Token
	for tok = s.it.Next(); tok.Type == Whitespace; tok = s.it.Next() {
	}
	return tok
}

func (s *Stream) consume() *Token {
	s.previous = s.current
	s.current = s.next
	if s.backup != nil {
		s.next = s.backup
		s.backup = nil
	} else if s.End() {
		s.next = nil
	} else {
		s.next = s.nonIgnored()
	}
	return s.previous
}

func (s *Stream) Current() *Token {
	return s.current
}

func (s *Stream) Next() *Token {
	return s.consume()
}

func (s *Stream) EOF() bool {
	return s.current.Type == EOF
}

func (s *Stream) IsError() bool {
	return s.current.Type == Error
}

func (s *Stream) End() bool {
	return s.EOF() || s.IsError()
}

func (s *Stream) Peek() *Token {
	return s.next
}

func (s *Stream) Backup() {
	if s.previous == nil {
		panic("Can't backup")
	}
	s.backup = s.next
	s.next = s.current
	s.current = s.previous
	s.previous = nil
}
