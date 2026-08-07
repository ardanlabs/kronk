package model

import "strings"

type stopPiece struct {
	content           string
	logprob           *ContentLogprob
	utf8Logprobs      []*ContentLogprob
	provisionalReason bool
	speculative       bool
}

type stopGate struct {
	stops     []string
	pending   []stopPiece
	discarded []stopPiece
}

func newStopGate(stops []string) *stopGate {
	if len(stops) == 0 {
		return nil
	}

	return &stopGate{stops: stops}
}

func (g *stopGate) feed(piece stopPiece) ([]stopPiece, bool) {
	g.pending = append(g.pending, piece)

	var text strings.Builder
	for _, pending := range g.pending {
		text.WriteString(pending.content)
	}
	all := text.String()

	matchStart := -1
	matchEnd := -1
	matchOrder := len(g.stops)
	for order, stop := range g.stops {
		start := strings.Index(all, stop)
		if start < 0 {
			continue
		}
		end := start + len(stop)
		if matchEnd < 0 || end < matchEnd || end == matchEnd && order < matchOrder {
			matchStart = start
			matchEnd = end
			matchOrder = order
		}
	}
	if matchEnd >= 0 {
		emitted := g.takeBefore(matchStart, true)
		g.discarded = append(g.discarded[:0], g.pending...)
		g.pending = nil
		return emitted, true
	}

	ambiguous := 0
	for _, stop := range g.stops {
		limit := min(len(stop)-1, len(all))
		for n := limit; n > ambiguous; n-- {
			if strings.HasSuffix(all, stop[:n]) {
				ambiguous = n
				break
			}
		}
	}
	return g.takeBefore(len(all)-ambiguous, false), false
}

func (g *stopGate) flush() []stopPiece {
	pending := g.pending
	g.pending = nil
	return pending
}

func (g *stopGate) takeDiscarded() []stopPiece {
	discarded := g.discarded
	g.discarded = nil
	return discarded
}

func (g *stopGate) takeBefore(offset int, truncate bool) []stopPiece {
	var emitted []stopPiece
	consumed := 0
	keepAt := 0
	for i, piece := range g.pending {
		end := consumed + len(piece.content)
		switch {
		case end <= offset:
			emitted = append(emitted, piece)
			keepAt = i + 1
		case consumed < offset && truncate:
			piece.content = piece.content[:offset-consumed]
			piece.logprob = nil
			emitted = append(emitted, piece)
			keepAt = i + 1
		}
		if end > offset {
			break
		}
		consumed = end
	}
	g.pending = g.pending[keepAt:]
	return emitted
}
