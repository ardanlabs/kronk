package model

import (
	"strings"
	"testing"
)

func TestStopGate(t *testing.T) {
	tests := []struct {
		name        string
		stops       []string
		pieces      []string
		want        []string
		wantMatch   bool
		wantPending string
	}{
		{name: "cross-piece match", stops: []string{"STOP"}, pieces: []string{"hello ST", "OP later"}, want: []string{"hello "}, wantMatch: true},
		{name: "false prefix preserves pieces", stops: []string{"STOP"}, pieces: []string{"hello ST", "X"}, want: []string{"hello ST", "X"}},
		{name: "match truncates piece", stops: []string{"STOP"}, pieces: []string{"hello STOP later"}, want: []string{"hello "}, wantMatch: true},
		{name: "causal completion beats earlier start", stops: []string{"abcde", "bc"}, pieces: []string{"abc"}, want: []string{"a"}, wantMatch: true},
		{name: "request order tie", stops: []string{"abc", "bc"}, pieces: []string{"abc"}, wantMatch: true},
		{name: "ambiguous suffix", stops: []string{"STOP"}, pieces: []string{"hello", " ST"}, want: []string{"hello"}, wantPending: " ST"},
		{name: "single byte stop", stops: []string{"X"}, pieces: []string{"beforeXafter"}, want: []string{"before"}, wantMatch: true},
		{name: "multibyte stop", stops: []string{"🌍END"}, pieces: []string{"hello 🌍", "END later"}, want: []string{"hello "}, wantMatch: true},
		{name: "multicodepoint stop", stops: []string{"終わり"}, pieces: []string{"text 終", "わ", "り later"}, want: []string{"text "}, wantMatch: true},
		{name: "exact codepoints do not normalize", stops: []string{"é"}, pieces: []string{"cafe", "\u0301"}, want: []string{"cafe", "\u0301"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := newStopGate(tt.stops)
			var got []string
			matched := false
			for _, content := range tt.pieces {
				emitted, match := gate.feed(stopPiece{content: content})
				for _, piece := range emitted {
					got = append(got, piece.content)
				}
				if match {
					matched = true
					break
				}
			}
			if !matched && tt.wantPending == "" {
				for _, piece := range gate.flush() {
					got = append(got, piece.content)
				}
			}
			if strings.Join(got, "") != strings.Join(tt.want, "") {
				t.Errorf("content: got %q, want %q", got, tt.want)
			}
			if matched != tt.wantMatch {
				t.Errorf("matched: got %t, want %t", matched, tt.wantMatch)
			}
			if tt.wantPending != "" {
				var pending strings.Builder
				for _, piece := range gate.flush() {
					pending.WriteString(piece.content)
				}
				if pending.String() != tt.wantPending {
					t.Errorf("pending: got %q, want %q", pending.String(), tt.wantPending)
				}
			}
		})
	}
}

func TestStopGateMatchesCodepointSplitAcrossUTF8Bytes(t *testing.T) {
	world := []byte("🌍")
	chunks := [][]byte{
		append([]byte("hello "), world[:2]...),
		append(append([]byte{}, world[2:]...), 'E'),
		[]byte("ND later"),
	}

	gate := newStopGate([]string{"🌍END"})
	var utf8Buf []byte
	var output strings.Builder
	matched := false
	for _, chunk := range chunks {
		utf8Buf = append(utf8Buf, chunk...)
		complete, remainder := extractCompleteUTF8(utf8Buf)
		content := string(complete)
		utf8Buf = append(utf8Buf[:0], remainder...)
		if content == "" {
			continue
		}

		emitted, match := gate.feed(stopPiece{content: content})
		for _, piece := range emitted {
			output.WriteString(piece.content)
		}
		if match {
			matched = true
			break
		}
	}

	if !matched {
		t.Fatal("matched: got false, want true")
	}
	if got, want := output.String(), "hello "; got != want {
		t.Errorf("content: got %q, want %q", got, want)
	}
	if len(utf8Buf) != 0 {
		t.Errorf("UTF-8 remainder: got %v, want empty", utf8Buf)
	}
}

func TestStopGateOmitsLogprobForTruncatedPiece(t *testing.T) {
	logprob := &ContentLogprob{}
	gate := newStopGate([]string{"STOP"})
	emitted, matched := gate.feed(stopPiece{content: "visibleSTOP", logprob: logprob})
	if !matched {
		t.Fatal("matched: got false, want true")
	}
	if len(emitted) != 1 || emitted[0].content != "visible" {
		t.Fatalf("emitted: got %+v, want visible", emitted)
	}
	if emitted[0].logprob != nil {
		t.Error("logprob: got entry, want nil for truncated piece")
	}
}

func TestStopGateUnicodeOverlapFlushAndBounds(t *testing.T) {
	gate := newStopGate([]string{"🌍END", "END"})
	inputs := []string{"hello 🌍", "E", "X", " and ", "END"}
	var output strings.Builder
	for _, input := range inputs {
		emitted, matched := gate.feed(stopPiece{content: input})
		for _, piece := range emitted {
			output.WriteString(piece.content)
		}
		if len(gate.pending) > len("🌍END") {
			t.Fatalf("pending pieces: got %d, want at most %d", len(gate.pending), len("🌍END"))
		}
		if matched {
			break
		}
	}
	if got, want := output.String(), "hello 🌍EX and "; got != want {
		t.Errorf("content: got %q, want %q", got, want)
	}

	gate = newStopGate([]string{"STOP"})
	var natural strings.Builder
	for _, input := range []string{"natural ", "ST", "ream"} {
		emitted, matched := gate.feed(stopPiece{content: input})
		if matched {
			t.Fatal("matched: got true, want false")
		}
		for _, piece := range emitted {
			natural.WriteString(piece.content)
		}
	}
	for _, piece := range gate.flush() {
		natural.WriteString(piece.content)
	}
	if got, want := natural.String(), "natural STream"; got != want {
		t.Errorf("natural flush: got %q, want %q", got, want)
	}
}

func TestStopGateRemovedPieceLogprobs(t *testing.T) {
	first := &ContentLogprob{Token: "first"}
	second := &ContentLogprob{Token: "second"}
	gate := newStopGate([]string{"STOP"})

	if emitted, matched := gate.feed(stopPiece{content: "ST", logprob: first}); matched || len(emitted) != 0 {
		t.Fatalf("first feed: got emitted=%v matched=%t, want pending", emitted, matched)
	}
	emitted, matched := gate.feed(stopPiece{content: "OP", logprob: second})
	if !matched || len(emitted) != 0 {
		t.Fatalf("fully removed: got emitted=%v matched=%t, want no emitted pieces and match", emitted, matched)
	}

	gate = newStopGate([]string{"STOP"})
	emitted, matched = gate.feed(stopPiece{content: "visibleSTOP", logprob: first})
	if !matched || len(emitted) != 1 || emitted[0].content != "visible" || emitted[0].logprob != nil {
		t.Fatalf("partially removed: got emitted=%+v matched=%t, want visible without logprob", emitted, matched)
	}
}

func TestAppendStopPieceLogprobsPreservesUTF8Tokens(t *testing.T) {
	first := &ContentLogprob{Token: "first"}
	second := &ContentLogprob{Token: "second"}
	s := slot{}

	index := appendStopPieceLogprobs(&s, stopPiece{
		logprob:      second,
		utf8Logprobs: []*ContentLogprob{first},
	})
	if index != 1 {
		t.Fatalf("index: got %d, want 1", index)
	}
	if len(s.logprobsData) != 2 {
		t.Fatalf("logprobs: got %d, want 2", len(s.logprobsData))
	}
	if s.logprobsData[0].Token != "first" || s.logprobsData[1].Token != "second" {
		t.Errorf("tokens: got %q, %q, want first, second", s.logprobsData[0].Token, s.logprobsData[1].Token)
	}
}

func TestStopPieceAccounting(t *testing.T) {
	s := slot{reasonTokens: 2, completionTokens: 3, specCoveredTotal: 1}
	piece := stopPiece{provisionalReason: true, speculative: true}

	accountStopPiece(&s, piece)
	if s.reasonTokens != 3 || s.completionTokens != 3 || s.specCoveredTotal != 2 {
		t.Fatalf("provisional: got reason=%d completion=%d covered=%d, want 3, 3, 2", s.reasonTokens, s.completionTokens, s.specCoveredTotal)
	}
	s.reasonFlag = 0
	reconcileStopPiece(&s, piece)
	if s.reasonTokens != 2 || s.completionTokens != 4 || s.specCoveredTotal != 2 {
		t.Fatalf("reconciled: got reason=%d completion=%d covered=%d, want 2, 4, 2", s.reasonTokens, s.completionTokens, s.specCoveredTotal)
	}
	unaccountStopPiece(&s, stopPiece{speculative: true})
	if s.reasonTokens != 2 || s.completionTokens != 3 || s.specCoveredTotal != 1 {
		t.Fatalf("parser EOG undo: got reason=%d completion=%d covered=%d, want 2, 3, 1", s.reasonTokens, s.completionTokens, s.specCoveredTotal)
	}
}

func TestStopGateReconcilesDiscardedPiecesAfterChannelTransition(t *testing.T) {
	gate := newStopGate([]string{"STOP"})
	s := slot{}
	first := stopPiece{content: "<think>S"}
	second := stopPiece{content: "TOP"}

	accountStopPiece(&s, first)
	if emitted, matched := gate.feed(first); matched || len(emitted) != 0 {
		t.Fatalf("first feed: got emitted=%v matched=%t, want pending", emitted, matched)
	}
	accountStopPiece(&s, second)
	emitted, matched := gate.feed(second)
	if !matched || len(emitted) != 1 || emitted[0].content != "<think>" {
		t.Fatalf("second feed: got emitted=%v matched=%t, want <think> and match", emitted, matched)
	}

	s.reasonFlag = 1
	reconcileStopPiece(&s, emitted[0])
	for _, piece := range gate.takeDiscarded() {
		reconcileStopPiece(&s, piece)
	}
	if s.reasonTokens != 2 || s.completionTokens != 0 {
		t.Errorf("tokens: got reasoning=%d completion=%d, want 2, 0", s.reasonTokens, s.completionTokens)
	}
}
