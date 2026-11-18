package normalization

import (
	"context"
	"slices"
)

type span struct {
	start, end int
}

type matcher struct {
	ctx   context.Context
	text  []rune
	icase bool
	caps  []span
}

func (m *matcher) setMatch(n *node, c rune) bool {
	test := func(c rune) bool {
		if inRanges(c, n.ranges...) {
			return true
		}
		for _, cl := range n.classes {
			if classMatch(cl.kind, c) != cl.neg {
				return true
			}
		}
		return false
	}
	matched := test(c)
	if !matched && m.icase {
		matched = test(swapCase(c))
	}
	return matched != n.neg
}

func (m *matcher) alternatives(alts [][]item, pos int) int {
	for _, seq := range alts {
		if m.ctx.Err() != nil {
			return -1
		}
		saved := slices.Clone(m.caps)
		if end := m.sequence(seq, pos); end >= 0 {
			return end
		}
		m.caps = saved
	}
	return -1
}

func (m *matcher) sequence(seq []item, pos int) int {
	for _, it := range seq {
		count := 0
		for count < it.max {
			if m.ctx.Err() != nil {
				return -1
			}
			var saved []span
			if it.atom.op == group {
				saved = slices.Clone(m.caps)
			}
			e := m.once(it.atom, pos)
			if e < 0 {
				if it.atom.op == group {
					m.caps = saved
				}
				break
			}
			if e == pos {
				count = max(count+1, it.min)
				break
			}
			pos = e
			count++
		}
		if count < it.min {
			return -1
		}
	}
	return pos
}

func (m *matcher) word(pos int) bool {
	return pos >= 0 && pos < len(m.text) && classMatch('w', m.text[pos])
}

func (m *matcher) once(n *node, pos int) int {
	if m.ctx.Err() != nil {
		return -1
	}
	switch n.op {
	case literal:
		if pos < len(m.text) && (m.text[pos] == n.cp || m.icase && lower(m.text[pos]) == lower(n.cp)) {
			return pos + 1
		}
	case set:
		if pos < len(m.text) && m.setMatch(n, m.text[pos]) {
			return pos + 1
		}
	case anyRune:
		if pos < len(m.text) {
			return pos + 1
		}
	case start:
		if pos == 0 {
			return pos
		}
	case end:
		if pos == len(m.text) {
			return pos
		}
	case boundary:
		if m.word(pos-1) != m.word(pos) {
			return pos
		}
	case group:
		e := m.alternatives(n.alts, pos)
		if e >= 0 {
			if n.capture >= 0 {
				m.caps[n.capture] = span{pos, e}
			}
			return e
		}
	case lookahead:
		saved := slices.Clone(m.caps)
		ok := m.alternatives(n.alts, pos) >= 0
		m.caps = saved
		if ok != n.neg {
			return pos
		}
	case lookbehind:
		ok := pos > 0 && m.once(n.alts[0][0].atom, pos-1) == pos
		if ok != n.neg {
			return pos
		}
	}
	return -1
}

type segment struct {
	rule       int
	start, end int
	groups     []span
}

// scan uses rune positions for matching but byte offsets for returned spans, so
// even invalid UTF-8 bytes are preserved exactly in substitutions and captures.
func scan(ctx context.Context, text string, patterns []*pattern) ([]segment, error) {
	t := make([]rune, 0, len(text))
	offsets := make([]int, 0, len(text)+1)
	for offset, c := range text {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		t = append(t, c)
		offsets = append(offsets, offset)
	}
	offsets = append(offsets, len(text))
	var out []segment
	pos, pending := 0, 0
	for pos < len(t) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		matched := false
		for r, pat := range patterns {
			m := matcher{ctx: ctx, text: t, icase: pat.icase, caps: make([]span, pat.caps)}
			for i := range m.caps {
				m.caps[i] = span{-1, -1}
			}
			e := m.alternatives(pat.alts, pos)
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if e <= pos {
				continue
			}
			if pending < pos {
				out = append(out, segment{rule: -1, start: offsets[pending], end: offsets[pos]})
			}
			for i, g := range m.caps {
				if g.start >= 0 {
					m.caps[i] = span{offsets[g.start], offsets[g.end]}
				}
			}
			out = append(out, segment{r, offsets[pos], offsets[e], m.caps})
			pos, pending, matched = e, e, true
			break
		}
		if !matched {
			pos++
		}
	}
	if pending < len(t) {
		out = append(out, segment{rule: -1, start: offsets[pending], end: len(text)})
	}
	return out, ctx.Err()
}
