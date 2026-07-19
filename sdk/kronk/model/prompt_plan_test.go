package model

import (
	"context"
	"crypto/sha256"
	"reflect"
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func mediaPlan(units ...promptUnit) promptPlan {
	plan := promptPlan{units: units}
	for _, unit := range units {
		if unit.isMedia {
			plan.mediaCount++
		} else {
			plan.textTokens++
		}
	}
	return plan
}

func TestProcessIMCMediaPlansMatches(t *testing.T) {
	imageA := sha256.Sum256([]byte("image-a"))
	imageB := sha256.Sum256([]byte("image-b"))
	text := func(token llama.Token) promptUnit { return promptUnit{token: token} }
	media := func(digest [sha256.Size]byte) promptUnit { return promptUnit{media: digest, isMedia: true} }
	base := mediaPlan(text(1), media(imageA), text(2))

	tests := []struct {
		name      string
		stable    promptPlan
		actual    promptPlan
		kvBytes   bool
		wantMatch string
	}{
		{name: "exact", stable: base, actual: mediaPlan(text(1), media(imageA), text(2), text(9)), kvBytes: true, wantMatch: "exact"},
		{name: "text append", stable: mediaPlan(text(1), media(imageA), text(2), text(3)), actual: mediaPlan(text(1), media(imageA), text(2), text(3), text(9)), kvBytes: true, wantMatch: "anchor"},
		{name: "changed media", stable: mediaPlan(text(1), media(imageB), text(2)), actual: mediaPlan(text(1), media(imageB), text(2), text(9)), kvBytes: true, wantMatch: "rebuild"},
		{name: "added media", stable: mediaPlan(text(1), media(imageA), text(2), media(imageB)), actual: mediaPlan(text(1), media(imageA), text(2), media(imageB), text(9)), kvBytes: true, wantMatch: "rebuild"},
		{name: "removed media", stable: mediaPlan(text(1), text(2)), actual: mediaPlan(text(1), text(2), text(9)), kvBytes: true, wantMatch: "rebuild"},
		{name: "reordered media", stable: mediaPlan(text(1), media(imageB), media(imageA), text(2)), actual: mediaPlan(text(1), media(imageB), media(imageA), text(2), text(9)), kvBytes: true, wantMatch: "rebuild"},
		{name: "text divergence before media", stable: mediaPlan(text(8), media(imageA), text(2)), actual: mediaPlan(text(8), media(imageA), text(2), text(9)), kvBytes: true, wantMatch: "rebuild"},
		{name: "suffix contains media", stable: mediaPlan(text(1), media(imageA), text(2), media(imageB)), actual: mediaPlan(text(1), media(imageA), text(2), media(imageB), text(9)), kvBytes: true, wantMatch: "rebuild"},
		{name: "empty snapshot", stable: base, actual: mediaPlan(text(1), media(imageA), text(2), text(9)), wantMatch: "rebuild"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := ramSessionStore()
			if tt.kvBytes {
				buf := store.Prepare(3)
				copy(buf, []byte{1, 2, 3})
				store.Commit(len(buf))
			}
			session := &imcSession{
				id:                0,
				seqID:             imcSeqIDUnbound,
				totalTokensCached: 3,
				nextLogicalPos:    3,
				cachedMsgCount:    1,
				cachedMsgsHash:    "hash",
				hasMedia:          true,
				promptPlan:        base,
				mediaKVCounts:     []int{1},
				kvState:           store,
			}
			m := &Model{
				imcSessions: []*imcSession{session},
				log:         func(context.Context, string, ...any) {},
			}
			d := D{"messages": []D{{"role": "user", "content": "test"}}}
			result := m.processIMCMediaPlans(context.Background(), d, d, tt.actual, tt.stable, []llama.Token{9}, time.Now())
			if result.imcMatchKind != tt.wantMatch {
				t.Fatalf("imcMatchKind = %q, want %q", result.imcMatchKind, tt.wantMatch)
			}
			if (tt.wantMatch == "exact" || tt.wantMatch == "anchor") && !session.pending {
				t.Fatal("media match did not reserve the session")
			}
			if tt.wantMatch == "anchor" && (!result.imcMediaAnchorAdvance || result.imcReadOnlyReservation) {
				t.Fatal("anchor did not select advancing-snapshot mode")
			}
		})
	}
}

func TestProcessIMCMediaAnchorPlanningIsImmutableAndUsesLogicalPosition(t *testing.T) {
	digest := sha256.Sum256([]byte("image"))
	base := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2})
	stable := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2}, promptUnit{token: 3})
	actual := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2}, promptUnit{token: 3}, promptUnit{token: 4})
	store := populatedTestSessionStore()
	session := &imcSession{
		id:                0,
		totalTokensCached: 12,
		nextLogicalPos:    5,
		cachedMsgCount:    2,
		cachedMsgsHash:    "hash",
		hasMedia:          true,
		useMRoPE:          true,
		promptPlan:        base,
		mediaKVCounts:     []int{8},
		kvState:           store,
	}
	originalBytes := append([]byte(nil), store.Bytes()...)
	originalPlan := append([]promptUnit(nil), session.promptPlan.units...)
	originalLastUsed := session.lastUsed
	m := &Model{imcSessions: []*imcSession{session}, log: func(context.Context, string, ...any) {}}
	d := D{"messages": []D{{"role": "user", "content": "test"}}}

	result := m.processIMCMediaPlans(context.Background(), d, d, actual, stable, []llama.Token{4}, time.Now())

	if result.imcMatchKind != "anchor" || result.cacheIdx != 5 || result.imcExpectedTokens != 12 {
		t.Fatalf("anchor result = kind %q cacheIdx %d physical %d", result.imcMatchKind, result.cacheIdx, result.imcExpectedTokens)
	}
	if want := []llama.Token{4}; !reflect.DeepEqual(result.imcTailTokens, want) {
		t.Fatalf("tail = %v, want %v", result.imcTailTokens, want)
	}
	if want := []llama.Token{3}; !reflect.DeepEqual(result.imcNewCacheTokens, want) {
		t.Fatalf("advance tokens = %v, want %v", result.imcNewCacheTokens, want)
	}
	if !reflect.DeepEqual(session.promptPlan.units, originalPlan) || !reflect.DeepEqual(store.Bytes(), originalBytes) || session.lastUsed != originalLastUsed {
		t.Fatal("anchor planning mutated stored session metadata or snapshot")
	}
}

func TestIMCCommitMediaAdvanceAndReuse(t *testing.T) {
	digest := sha256.Sum256([]byte("image"))
	base := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2})
	advanced := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2}, promptUnit{token: 3})
	next := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2}, promptUnit{token: 3}, promptUnit{token: 4})
	oldStore := populatedTestSessionStore()
	staged := populatedTestSessionStore()
	session := &imcSession{
		id:                0,
		totalTokensCached: 12,
		nextLogicalPos:    5,
		cachedMsgCount:    2,
		cachedMsgsHash:    "old",
		hasMedia:          true,
		useMRoPE:          true,
		promptPlan:        base,
		mediaKVCounts:     []int{8},
		kvState:           oldStore,
		pending:           true,
	}
	m := &Model{imcSessions: []*imcSession{session}, log: func(context.Context, string, ...any) {}}
	d := D{"messages": []D{{"role": "user", "content": "test"}}}

	gotOld := m.imcCommitMediaAdvance(session, staged, "advanced", 13, 3, 6, advanced, "render")
	if gotOld != oldStore || session.kvState != staged || session.totalTokensCached != 13 || session.nextLogicalPos != 6 || !session.promptPlan.equal(advanced) {
		t.Fatal("media advance did not atomically publish the staged state")
	}
	m.imcPublishSession(session)

	exactActual := mediaPlan(append(append([]promptUnit{}, advanced.units...), promptUnit{token: 9})...)
	exact := m.processIMCMediaPlans(context.Background(), d, d, exactActual, advanced, []llama.Token{9}, time.Now())
	if exact.imcMatchKind != "exact" {
		t.Fatalf("advanced exact match = %q, want exact", exact.imcMatchKind)
	}
	m.imcClearPending(session.id)

	nextActual := mediaPlan(append(append([]promptUnit{}, next.units...), promptUnit{token: 9})...)
	appendResult := m.processIMCMediaPlans(context.Background(), d, d, nextActual, next, []llama.Token{9}, time.Now())
	if appendResult.imcMatchKind != "anchor" || !reflect.DeepEqual(appendResult.imcNewCacheTokens, []llama.Token{4}) {
		t.Fatalf("next append = kind %q tokens %v, want anchor [4]", appendResult.imcMatchKind, appendResult.imcNewCacheTokens)
	}
}

func TestIMCCommitMediaAdvanceRejectsMissingStage(t *testing.T) {
	oldStore := populatedTestSessionStore()
	session := &imcSession{
		cachedMsgsHash:    "old",
		totalTokensCached: 12,
		nextLogicalPos:    5,
		kvState:           oldStore,
	}
	m := &Model{}

	if old := m.imcCommitMediaAdvance(session, nil, "new", 13, 3, 6, promptPlan{}, "render"); old != nil {
		t.Fatalf("old store = %T, want nil for rejected commit", old)
	}
	if session.kvState != oldStore || session.cachedMsgsHash != "old" || session.totalTokensCached != 12 || session.nextLogicalPos != 5 {
		t.Fatal("rejected media advance mutated the prior valid session")
	}
}

func TestBuildPromptPlanTokens(t *testing.T) {
	marker := "<media>"
	mediaBytes := []byte("image")
	var calls []string

	plan, err := buildPromptPlanTokens("before"+marker+"after", marker, [][]byte{mediaBytes}, 1, true, func(text string) []llama.Token {
		calls = append(calls, text)
		switch text {
		case "before":
			return []llama.Token{2, 3}
		case "after":
			return []llama.Token{4}
		default:
			return nil
		}
	})
	if err != nil {
		t.Fatalf("buildPromptPlanTokens: %v", err)
	}
	if want := []string{"before", "after"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("tokenized segments = %q, want %q", calls, want)
	}
	wantUnits := []promptUnit{
		{token: 1},
		{token: 2},
		{token: 3},
		{media: sha256.Sum256(mediaBytes), isMedia: true},
		{token: 4},
	}
	if !reflect.DeepEqual(plan.units, wantUnits) {
		t.Fatalf("units = %#v, want %#v", plan.units, wantUnits)
	}
	if plan.textTokens != 4 || plan.mediaCount != 1 {
		t.Fatalf("counts = text %d media %d, want text 4 media 1", plan.textTokens, plan.mediaCount)
	}
}

func TestBuildPromptPlanTokensAddsBOSOnceAcrossMediaBoundaries(t *testing.T) {
	marker := "<media>"
	plan, err := buildPromptPlanTokens(marker+marker, marker, [][]byte{[]byte("a"), []byte("b")}, 7, true, func(string) []llama.Token {
		return nil
	})
	if err != nil {
		t.Fatalf("buildPromptPlanTokens: %v", err)
	}
	if plan.textTokens != 1 || len(plan.units) != 3 || plan.units[0].token != 7 || plan.units[0].isMedia {
		t.Fatalf("plan = %#v, want one global BOS followed by two media units", plan)
	}
}

func TestBuildPromptPlanTokensRejectsMarkerMediaMismatch(t *testing.T) {
	_, err := buildPromptPlanTokens("no marker", "<media>", [][]byte{[]byte("image")}, 1, true, func(string) []llama.Token { return nil })
	if err == nil {
		t.Fatal("buildPromptPlanTokens accepted a marker/media mismatch")
	}
}

func TestPromptPlanPrefix(t *testing.T) {
	imageA := sha256.Sum256([]byte("image-a"))
	imageB := sha256.Sum256([]byte("image-b"))
	text := func(token llama.Token) promptUnit { return promptUnit{token: token} }
	media := func(digest [sha256.Size]byte) promptUnit { return promptUnit{media: digest, isMedia: true} }

	base := promptPlan{units: []promptUnit{text(1), media(imageA), text(2)}}
	tests := []struct {
		name string
		plan promptPlan
		want bool
	}{
		{name: "exact", plan: base, want: true},
		{name: "text append", plan: promptPlan{units: []promptUnit{text(1), media(imageA), text(2), text(3)}}, want: true},
		{name: "changed media", plan: promptPlan{units: []promptUnit{text(1), media(imageB), text(2)}}, want: false},
		{name: "reordered media", plan: promptPlan{units: []promptUnit{text(1), media(imageB), media(imageA), text(2)}}, want: false},
		{name: "text divergence", plan: promptPlan{units: []promptUnit{text(1), media(imageA), text(9)}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plan.hasPrefix(base); got != tt.want {
				t.Errorf("hasPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPromptPlanTextTail(t *testing.T) {
	digest := sha256.Sum256([]byte("image"))
	base := promptPlan{units: []promptUnit{{token: 1}, {media: digest, isMedia: true}}}

	textPlan := promptPlan{units: append(append([]promptUnit{}, base.units...), promptUnit{token: 2}, promptUnit{token: 3})}
	if got, ok := textPlan.textTail(base); !ok || len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Errorf("textTail() = %v, %v, want [2 3], true", got, ok)
	}

	mediaPlan := promptPlan{units: append(append([]promptUnit{}, base.units...), promptUnit{media: digest, isMedia: true})}
	if _, ok := mediaPlan.textTail(base); ok {
		t.Fatal("textTail() accepted a media tail")
	}
}

func TestPromptPlanEqual(t *testing.T) {
	first := promptPlan{units: []promptUnit{{token: 1}, {token: 2}}}
	second := promptPlan{units: []promptUnit{{token: 1}, {token: 2}}}
	diverged := promptPlan{units: []promptUnit{{token: 1}, {token: 3}}}

	if !first.equal(second) {
		t.Error("equal() = false for identical plans")
	}
	if first.equal(diverged) {
		t.Error("equal() = true for divergent plans")
	}
}
