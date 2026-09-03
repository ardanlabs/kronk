package model

import (
	"context"
	"crypto/sha256"
	"errors"
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
	baseSamplerTokens := []llama.Token{100, 1, 2, 101}

	tests := []struct {
		name      string
		stable    promptPlan
		actual    promptPlan
		kvBytes   bool
		nonCausal bool
		noNative  bool
		wantMatch string
	}{
		{name: "exact", stable: base, actual: mediaPlan(text(1), media(imageA), text(2), text(9)), kvBytes: true, wantMatch: "exact"},
		{name: "text append", stable: mediaPlan(text(1), media(imageA), text(2), text(3)), actual: mediaPlan(text(1), media(imageA), text(2), text(3), text(9)), kvBytes: true, wantMatch: "anchor"},
		{name: "changed media", stable: mediaPlan(text(1), media(imageB), text(2)), actual: mediaPlan(text(1), media(imageB), text(2), text(9)), kvBytes: true, wantMatch: "rebuild"},
		{name: "added media", stable: mediaPlan(text(1), media(imageA), text(2), media(imageB)), actual: mediaPlan(text(1), media(imageA), text(2), media(imageB), text(9)), kvBytes: true, wantMatch: "media-append"},
		{name: "added media non-causal", stable: mediaPlan(text(1), media(imageA), text(2), media(imageB)), actual: mediaPlan(text(1), media(imageA), text(2), media(imageB), text(9)), kvBytes: true, nonCausal: true, wantMatch: "rebuild"},
		{name: "added media without native metadata", stable: mediaPlan(text(1), media(imageA), text(2), media(imageB)), actual: mediaPlan(text(1), media(imageA), text(2), media(imageB), text(9)), kvBytes: true, noNative: true, wantMatch: "rebuild"},
		{name: "removed media", stable: mediaPlan(text(1), text(2)), actual: mediaPlan(text(1), text(2), text(9)), kvBytes: true, wantMatch: "rebuild"},
		{name: "reordered media", stable: mediaPlan(text(1), media(imageB), media(imageA), text(2)), actual: mediaPlan(text(1), media(imageB), media(imageA), text(2), text(9)), kvBytes: true, wantMatch: "rebuild"},
		{name: "text divergence before media", stable: mediaPlan(text(8), media(imageA), text(2)), actual: mediaPlan(text(8), media(imageA), text(2), text(9)), kvBytes: true, wantMatch: "rebuild"},
		{name: "suffix contains media", stable: mediaPlan(text(1), media(imageA), text(2), media(imageB)), actual: mediaPlan(text(1), media(imageA), text(2), media(imageB), text(9)), kvBytes: true, wantMatch: "media-append"},
		{name: "empty snapshot", stable: base, actual: mediaPlan(text(1), media(imageA), text(2), text(9)), wantMatch: "rebuild"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := ramSessionStore()
			if tt.kvBytes {
				buf := prepareTestStore(t, store, 3)
				copy(buf, []byte{1, 2, 3})
				commitTestStore(t, store, len(buf))
			}
			session := &imcSession{
				id:                  0,
				seqID:               imcSeqIDUnbound,
				totalTokensCached:   3,
				nextLogicalPos:      3,
				cachedMsgCount:      1,
				cachedMsgsHash:      "hash",
				hasMedia:            true,
				promptPlan:          base,
				samplerPromptTokens: baseSamplerTokens,
				mediaKVCounts:       []int{1},
				mediaNativeChunks:   []imcMediaChunk{{kind: imcMediaChunkImage, nTokens: 1, nPos: 1}},
				kvState:             store,
			}
			session.useNonCausal = tt.nonCausal
			if tt.noNative {
				session.mediaNativeChunks = nil
			}
			m := &Model{
				imcSessions: []*imcSession{session},
				log:         func(context.Context, string, ...any) {},
			}
			d := D{"messages": []D{{"role": "user", "content": "test"}}}
			session.cachedRenderInputHash, _ = m.imcRenderFingerprint(d, dMessages(d))
			result := m.processIMCMediaPlans(context.Background(), d, d, tt.actual, tt.stable, []llama.Token{9}, time.Now())
			if result.imcMatchKind != tt.wantMatch {
				t.Fatalf("imcMatchKind = %q, want %q", result.imcMatchKind, tt.wantMatch)
			}
			var wantSamplerTokens []llama.Token
			switch tt.wantMatch {
			case "exact":
				wantSamplerTokens = []llama.Token{100, 1, 2, 101, 9}
			case "anchor":
				wantSamplerTokens = []llama.Token{100, 1, 2, 101, 3, 9}
			}
			if got := result.imcSamplerPromptTokens; !reflect.DeepEqual(got, wantSamplerTokens) {
				t.Fatalf("imcSamplerPromptTokens = %v, want authoritative mtmd text prompt %v", got, wantSamplerTokens)
			}
			if (tt.wantMatch == "exact" || tt.wantMatch == "anchor" || tt.wantMatch == "media-append") && !session.reserved {
				t.Fatal("media match did not reserve the session")
			}
			if tt.wantMatch == "anchor" && (!result.imcMediaAnchorAdvance || result.imcReadOnlyReservation) {
				t.Fatal("anchor did not select advancing-snapshot mode")
			}
			if tt.wantMatch == "media-append" && (!result.imcMediaAnchorAdvance || !result.imcMediaAppend || result.imcReadOnlyReservation) {
				t.Fatal("media append did not select media advancing-snapshot mode")
			}
		})
	}
}

func TestProcessIMCMediaAnchorPlanningIsImmutableAndUsesLogicalPosition(t *testing.T) {
	digest := sha256.Sum256([]byte("image"))
	base := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2})
	stable := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2}, promptUnit{token: 3})
	actual := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2}, promptUnit{token: 3}, promptUnit{token: 4})
	store := populatedTestSessionStore(t)
	session := &imcSession{
		id:                  0,
		totalTokensCached:   12,
		nextLogicalPos:      5,
		cachedMsgCount:      2,
		cachedMsgsHash:      "hash",
		hasMedia:            true,
		useMRoPE:            true,
		promptPlan:          base,
		samplerPromptTokens: []llama.Token{100, 1, 2, 101},
		mediaKVCounts:       []int{8},
		kvState:             store,
	}
	originalBytes := append([]byte(nil), bytesTestStore(t, store)...)
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
	if want := []llama.Token{100, 1, 2, 101, 3, 4}; !reflect.DeepEqual(result.imcSamplerPromptTokens, want) {
		t.Fatalf("sampler prompt = %v, want authoritative mtmd prefix plus extension and tail %v", result.imcSamplerPromptTokens, want)
	}
	if want := []llama.Token{100, 1, 2, 101, 3}; !reflect.DeepEqual(result.imcMediaSamplerTokens, want) {
		t.Fatalf("cached sampler prompt = %v, want %v", result.imcMediaSamplerTokens, want)
	}
	if !reflect.DeepEqual(session.promptPlan.units, originalPlan) || !reflect.DeepEqual(bytesTestStore(t, store), originalBytes) || session.lastUsed != originalLastUsed {
		t.Fatal("anchor planning mutated stored session metadata or snapshot")
	}
}

func TestIMCCommitMediaAdvanceAndReuse(t *testing.T) {
	digest := sha256.Sum256([]byte("image"))
	base := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2})
	advanced := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2}, promptUnit{token: 3})
	next := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2}, promptUnit{token: 3}, promptUnit{token: 4})
	oldStore := populatedTestSessionStore(t)
	staged := populatedTestSessionStore(t)
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
		reserved:          true,
	}
	m := &Model{imcSessions: []*imcSession{session}, log: func(context.Context, string, ...any) {}}
	d := D{"messages": []D{{"role": "user", "content": "test"}}}
	renderHash, _ := m.imcRenderFingerprint(d, dMessages(d))

	gotOld := m.imcCommitMediaAdvance(context.Background(), session, staged, "advanced", 13, 3, 6, advanced, []llama.Token{1, 2, 3}, []int{8}, nil, true, false, renderHash)
	if gotOld != oldStore || session.kvState != staged || session.totalTokensCached != 13 || session.nextLogicalPos != 6 || !session.promptPlan.equal(advanced) || !reflect.DeepEqual(session.samplerPromptTokens, []llama.Token{1, 2, 3}) {
		t.Fatal("media advance did not atomically publish the staged state")
	}
	m.imcPublishSession(session)

	exactActual := mediaPlan(append(append([]promptUnit{}, advanced.units...), promptUnit{token: 9})...)
	exact := m.processIMCMediaPlans(context.Background(), d, d, exactActual, advanced, []llama.Token{9}, time.Now())
	if exact.imcMatchKind != "exact" {
		t.Fatalf("advanced exact match = %q, want exact", exact.imcMatchKind)
	}
	m.imcReleaseReservation(session.id)

	nextActual := mediaPlan(append(append([]promptUnit{}, next.units...), promptUnit{token: 9})...)
	appendResult := m.processIMCMediaPlans(context.Background(), d, d, nextActual, next, []llama.Token{9}, time.Now())
	if appendResult.imcMatchKind != "anchor" || !reflect.DeepEqual(appendResult.imcNewCacheTokens, []llama.Token{4}) {
		t.Fatalf("next append = kind %q tokens %v, want anchor [4]", appendResult.imcMatchKind, appendResult.imcNewCacheTokens)
	}
}

func TestProcessIMCMediaPlansRebuildsExactPlanWhenRenderFingerprintChanges(t *testing.T) {
	digest := sha256.Sum256([]byte("image"))
	stable := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2})
	actual := mediaPlan(promptUnit{token: 1}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 2}, promptUnit{token: 9})
	priorD := D{
		"messages":             []D{{"role": "user", "content": "test"}},
		"chat_template_kwargs": D{"custom_mode": "a"},
	}
	currentD := priorD.Clone()
	currentD["chat_template_kwargs"] = D{"custom_mode": "b"}
	session := &imcSession{
		id:                  0,
		totalTokensCached:   3,
		nextLogicalPos:      3,
		hasMedia:            true,
		promptPlan:          stable,
		samplerPromptTokens: []llama.Token{1, 2},
		mediaKVCounts:       []int{1},
		kvState:             populatedTestSessionStore(t),
	}
	m := &Model{imcSessions: []*imcSession{session}, log: func(context.Context, string, ...any) {}}
	session.cachedRenderInputHash, _ = m.imcRenderFingerprint(priorD, dMessages(priorD))

	result := m.processIMCMediaPlans(context.Background(), currentD, currentD, actual, stable, []llama.Token{9}, time.Now())
	if result.imcMatchKind != "rebuild" {
		t.Errorf("imcMatchKind: got %q, want %q", result.imcMatchKind, "rebuild")
	}
	if result.err != nil {
		t.Errorf("err: got %v, want nil", result.err)
	}
}

func TestIMCCommitMediaAdvanceRejectsMissingStage(t *testing.T) {
	oldStore := populatedTestSessionStore(t)
	session := &imcSession{
		cachedMsgsHash:    "old",
		totalTokensCached: 12,
		nextLogicalPos:    5,
		kvState:           oldStore,
	}
	m := &Model{}

	old := m.imcCommitMediaAdvance(context.Background(), session, nil, "new", 13, 3, 6, promptPlan{}, nil, nil, nil, false, false, "render")
	if old != nil {
		t.Fatalf("old store = %T, want nil for rejected commit", old)
	}
	if session.kvState != oldStore || session.cachedMsgsHash != "old" || session.totalTokensCached != 12 || session.nextLogicalPos != 5 {
		t.Fatal("rejected media advance mutated the prior valid session")
	}
}

func TestIMCCommitMediaAdvanceInvalidatesDraftMetadataWhenResetFails(t *testing.T) {
	wantErr := errors.New("reset failed")
	oldStore := populatedTestSessionStore(t)
	staged := populatedTestSessionStore(t)
	session := &imcSession{
		kvState:      oldStore,
		draftKVState: &resetErrorStore{err: wantErr},
		pendingH:     []float32{1, 2, 3},
		reserved:     true,
	}
	var loggedErr error
	m := Model{log: func(_ context.Context, _ string, args ...any) {
		for i := 0; i+1 < len(args); i += 2 {
			if args[i] == "err" {
				loggedErr, _ = args[i+1].(error)
			}
		}
	}}

	gotOld := m.imcCommitMediaAdvance(context.Background(), session, staged, "advanced", 13, 3, 6, promptPlan{}, nil, nil, nil, false, false, "render")

	if !errors.Is(loggedErr, wantErr) {
		t.Errorf("logged error = %v, want %v", loggedErr, wantErr)
	}
	if gotOld != oldStore || session.kvState != staged {
		t.Fatal("target snapshot was not advanced")
	}
	if len(session.pendingH) != 0 {
		t.Errorf("len(pendingH) = %d, want 0", len(session.pendingH))
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

func TestProcessIMCMediaPlansReusesTextSessionForFirstMedia(t *testing.T) {
	digest := sha256.Sum256([]byte("image"))
	stable := mediaPlan(promptUnit{token: 1}, promptUnit{token: 2}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 3})
	actual := mediaPlan(promptUnit{token: 1}, promptUnit{token: 2}, promptUnit{media: digest, isMedia: true}, promptUnit{token: 3}, promptUnit{token: 9})
	session := &imcSession{
		id:                0,
		totalTokensCached: 2,
		cachedTokens:      []llama.Token{1, 2},
		cachedMsgCount:    2,
		cachedMsgsHash:    "text",
		kvState:           populatedTestSessionStore(t),
	}
	m := &Model{imcSessions: []*imcSession{session}, log: func(context.Context, string, ...any) {}}
	d := D{"messages": []D{{"role": "user", "content": "test"}}}

	result := m.processIMCMediaPlans(context.Background(), d, d, actual, stable, []llama.Token{9}, time.Now())

	if result.imcMatchKind != "media-append" || result.imcSession != session || result.cacheIdx != 2 {
		t.Fatalf("text-to-media result = kind %q, session %p, cache index %d; want media-append, %p, 2", result.imcMatchKind, result.imcSession, result.cacheIdx, session)
	}
	if !result.imcMediaAppend || !result.imcMediaAnchorAdvance || !session.reserved {
		t.Fatal("text-to-media append did not reserve the existing session for media advancement")
	}
}

func TestIMCMediaPrefixCursor(t *testing.T) {
	prefix := []imcMediaChunk{
		{kind: imcMediaChunkText, tokens: []llama.Token{1, 2}},
		{kind: imcMediaChunkImage, nTokens: 4, nPos: 3},
		{kind: imcMediaChunkImage, nTokens: 5, nPos: 4},
		{kind: imcMediaChunkText, tokens: []llama.Token{3, 4}},
	}
	cursor := imcMediaPrefixCursor{chunks: prefix}

	if tokens, skipped, err := cursor.consumeText([]llama.Token{1}); err != nil || len(tokens) != 0 || skipped != 1 {
		t.Fatalf("first split text = %v, %d, %v; want empty, 1, nil", tokens, skipped, err)
	}
	if tokens, skipped, err := cursor.consumeText([]llama.Token{2}); err != nil || len(tokens) != 0 || skipped != 1 {
		t.Fatalf("second split text = %v, %d, %v; want empty, 1, nil", tokens, skipped, err)
	}
	for _, chunk := range prefix[1:3] {
		if cached, err := cursor.consumeMedia(chunk); err != nil || !cached {
			t.Fatalf("tiled media = %v, %v; want true, nil", cached, err)
		}
	}
	tokens, skipped, err := cursor.consumeText([]llama.Token{3, 4, 5})
	if err != nil || !reflect.DeepEqual(tokens, []llama.Token{5}) || skipped != 2 || !cursor.done() {
		t.Fatalf("coalesced appended text = %v, %d, %v, done %v; want [5], 2, nil, true", tokens, skipped, err, cursor.done())
	}
}

func TestIMCMediaPrefixCursorRejectsDivergence(t *testing.T) {
	cursor := imcMediaPrefixCursor{chunks: []imcMediaChunk{{kind: imcMediaChunkText, tokens: []llama.Token{1, 2}}}}
	if _, _, err := cursor.consumeText([]llama.Token{1, 9}); err == nil {
		t.Fatal("consumeText accepted divergent native tokens")
	}

	cursor = imcMediaPrefixCursor{chunks: []imcMediaChunk{{kind: imcMediaChunkImage, nTokens: 4, nPos: 3}}}
	if _, err := cursor.consumeMedia(imcMediaChunk{kind: imcMediaChunkImage, nTokens: 5, nPos: 3}); err == nil {
		t.Fatal("consumeMedia accepted divergent native media dimensions")
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
