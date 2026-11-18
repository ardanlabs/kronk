package normalization

import (
	"errors"
	"fmt"
	"strings"
)

const (
	literal = iota
	set
	anyRune
	start
	end
	boundary
	group
	lookahead
	lookbehind
)

const infinity = 1 << 31

type characterClass struct {
	kind rune
	neg  bool
}

type node struct {
	op      int
	cp      rune
	neg     bool
	ranges  [][2]rune
	classes []characterClass
	alts    [][]item
	capture int
}

type item struct {
	atom     *node
	min, max int
}

type pattern struct {
	alts  [][]item
	icase bool
	caps  int
}

type parser struct {
	s    []rune
	p    int
	caps int
	err  error
}

func compile(src string) (*pattern, error) {
	if len(src) > 4096 {
		return nil, errors.New("invalid TTS norm pattern: pattern is longer than 4096 bytes")
	}
	p := parser{s: []rune(src)}
	pat := pattern{icase: strings.HasPrefix(src, "(?i)")}
	if pat.icase {
		p.p = 4
	}
	pat.alts = p.alternatives()
	pat.caps = p.caps
	if p.p < len(p.s) && p.err == nil {
		p.fail("unbalanced ')'")
	}
	if p.err != nil {
		return nil, p.err
	}
	return &pat, nil
}

func (p *parser) fail(msg string) {
	if p.err == nil {
		p.err = fmt.Errorf("invalid TTS norm pattern: %s at position %d", msg, p.p)
	}
}

func (p *parser) peek(c rune) bool {
	return p.p < len(p.s) && p.s[p.p] == c
}

func (p *parser) expect(c rune, what string) {
	if !p.peek(c) {
		p.fail("expected " + what)
		return
	}
	p.p++
}

func (p *parser) alternatives() [][]item {
	alts := [][]item{p.sequence()}
	for p.peek('|') && p.err == nil {
		p.p++
		alts = append(alts, p.sequence())
	}
	return alts
}

func (p *parser) sequence() []item {
	var seq []item
	for p.p < len(p.s) && !p.peek('|') && !p.peek(')') && p.err == nil {
		it := item{atom: p.atom(), min: 1, max: 1}
		p.quantifier(&it)
		seq = append(seq, it)
	}
	return seq
}

func (p *parser) number() int {
	if p.p >= len(p.s) || p.s[p.p] < '0' || p.s[p.p] > '9' {
		p.fail("expected number in quantifier")
		return 0
	}
	v := 0
	for p.p < len(p.s) && p.s[p.p] >= '0' && p.s[p.p] <= '9' {
		v = v*10 + int(p.s[p.p]-'0')
		if v > 1000 {
			p.fail("repetition count exceeds 1000")
			return 0
		}
		p.p++
	}
	return v
}

func (p *parser) quantifier(it *item) {
	if p.p >= len(p.s) || !strings.ContainsRune("?*+{", p.s[p.p]) || p.err != nil {
		return
	}
	switch it.atom.op {
	case start, end, boundary, lookahead, lookbehind:
		p.fail("zero-width assertion cannot be repeated")
		return
	}
	c := p.s[p.p]
	p.p++
	switch c {
	case '?':
		it.min, it.max = 0, 1
	case '*':
		it.min, it.max = 0, infinity
	case '+':
		it.min, it.max = 1, infinity
	case '{':
		it.min = p.number()
		it.max = it.min
		if p.peek(',') {
			p.p++
			it.max = infinity
			if !p.peek('}') {
				it.max = p.number()
			}
		}
		p.expect('}', "'}' to close quantifier")
		if it.max < it.min {
			p.fail("quantifier max is smaller than min")
		}
	}
	if p.p < len(p.s) && strings.ContainsRune("?*+{", p.s[p.p]) {
		p.fail("multiple quantifiers")
	}
}

func (p *parser) parseGroup() *node {
	n := &node{op: group, capture: -1}
	if !p.peek('?') {
		n.capture = p.caps
		p.caps++
	} else {
		p.p++
		if p.p >= len(p.s) {
			p.fail("unterminated group")
			return n
		}
		c := p.s[p.p]
		switch {
		case c == ':':
			p.p++
		case c == '=' || c == '!':
			p.p++
			n.op, n.neg = lookahead, c == '!'
		case c == '<' && p.p+1 < len(p.s) && (p.s[p.p+1] == '=' || p.s[p.p+1] == '!'):
			n.op, n.neg = lookbehind, p.s[p.p+1] == '!'
			p.p += 2
			if p.p >= len(p.s) || p.peek(')') {
				p.fail("lookbehind must contain exactly one codepoint matcher")
				return n
			}
			a := p.atom()
			if (a.op != literal && a.op != set && a.op != anyRune) || !p.peek(')') {
				p.fail("lookbehind must contain exactly one codepoint matcher")
				return n
			}
			p.p++
			n.alts = [][]item{{{atom: a, min: 1, max: 1}}}
			return n
		case c == 'i':
			p.fail("(?i) is only allowed at the start of the pattern")
			return n
		default:
			p.fail("unknown group type")
			return n
		}
	}
	n.alts = p.alternatives()
	p.expect(')', "')' to close group")
	return n
}

func (p *parser) escape(inSet bool) *node {
	n := &node{op: literal}
	if p.p >= len(p.s) {
		p.fail("dangling '%'")
		return n
	}
	c := p.s[p.p]
	p.p++
	switch {
	case c < 128 && strings.ContainsRune("dspawulhc", lower(c)):
		n.op = set
		n.classes = []characterClass{{kind: lower(c), neg: c >= 'A' && c <= 'Z'}}
	case c == 'b' && !inSet:
		n.op = boundary
	case c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
		p.p--
		p.fail(fmt.Sprintf("unknown escape '%%%c'", c))
	default:
		n.cp = c
	}
	return n
}

func (p *parser) setPoint() *node {
	if p.peek('%') {
		p.p++
		return p.escape(true)
	}
	n := &node{op: literal, cp: p.s[p.p]}
	p.p++
	return n
}

func (p *parser) parseSet() *node {
	n := &node{op: set}
	if p.peek('^') {
		n.neg = true
		p.p++
	}
	empty := true
	for p.err == nil {
		if p.p >= len(p.s) {
			p.fail("unterminated set")
			return n
		}
		if p.peek(']') {
			p.p++
			break
		}
		empty = false
		a := p.setPoint()
		if a.op == set {
			n.classes = append(n.classes, a.classes...)
			continue
		}
		lo, hi := a.cp, a.cp
		if p.peek('-') && p.p+1 < len(p.s) && p.s[p.p+1] != ']' {
			p.p++
			b := p.setPoint()
			if b.op == set {
				p.fail("character class cannot be used in a range")
			}
			hi = b.cp
			if hi < lo {
				p.fail("invalid range in set")
			}
		}
		n.ranges = append(n.ranges, [2]rune{lo, hi})
	}
	if empty {
		p.fail("empty set")
	}
	return n
}

func (p *parser) atom() *node {
	c := p.s[p.p]
	p.p++
	n := &node{op: literal, cp: c}
	switch c {
	case '(':
		return p.parseGroup()
	case '[':
		return p.parseSet()
	case '.':
		n.op = anyRune
	case '^':
		n.op = start
	case '$':
		n.op = end
	case '%':
		return p.escape(false)
	case '?', '*', '+', '{':
		p.p--
		p.fail("nothing to repeat")
	case ']', '}':
		p.p--
		p.fail(fmt.Sprintf("unescaped '%c', use '%%%c'", c, c))
	}
	return n
}
