package model

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestTokensHavePrefix(t *testing.T) {
	tests := []struct {
		name   string
		tokens []llama.Token
		prefix []llama.Token
		want   bool
	}{
		{name: "exact", tokens: []llama.Token{1, 2}, prefix: []llama.Token{1, 2}, want: true},
		{name: "append", tokens: []llama.Token{1, 2, 3}, prefix: []llama.Token{1, 2}, want: true},
		{name: "divergence", tokens: []llama.Token{1, 9, 3}, prefix: []llama.Token{1, 2}, want: false},
		{name: "longer prefix", tokens: []llama.Token{1}, prefix: []llama.Token{1, 2}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tokensHavePrefix(tt.tokens, tt.prefix); got != tt.want {
				t.Errorf("tokensHavePrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCommonTokenPrefixLen(t *testing.T) {
	tests := []struct {
		name string
		a    []llama.Token
		b    []llama.Token
		want int
	}{
		{name: "empty", a: nil, b: []llama.Token{1}, want: 0},
		{name: "diverges immediately", a: []llama.Token{1}, b: []llama.Token{2}, want: 0},
		{name: "divergent prefix", a: []llama.Token{1, 2, 3, 9}, b: []llama.Token{1, 2, 3, 4}, want: 3},
		{name: "shorter complete", a: []llama.Token{1, 2}, b: []llama.Token{1, 2, 3}, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commonTokenPrefixLen(tt.a, tt.b); got != tt.want {
				t.Errorf("commonTokenPrefixLen() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestProcessIMCTokenPlanProgressiveReusablePrefix(t *testing.T) {
	fallback := &imcSession{
		id:                0,
		cachedTokens:      []llama.Token{1, 2},
		totalTokensCached: 2,
		kvState:           populatedTestSessionStore(),
	}
	observer := &imcSession{
		id:                1,
		cachedTokens:      []llama.Token{1, 2, 3, 4, 8},
		totalTokensCached: 5,
		kvState:           populatedTestSessionStore(),
	}
	m := Model{
		cfg:         Config{PtrCacheMinTokens: new(2)},
		log:         applog.DiscardLogger,
		imcSessions: []*imcSession{fallback, observer},
	}
	target := []llama.Token{1, 2, 3, 4, 5, 6}
	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "user", "content": "r4"}}}, append(slices.Clone(target), 9), target, time.Now())

	if result.cacheIdx != 2 || result.imcCheckpointTokens != 4 {
		t.Errorf("plan restored/checkpoint tokens = %d/%d, want 2/4", result.cacheIdx, result.imcCheckpointTokens)
	}
	if observer.reserved {
		t.Error("LCP observer was reserved")
	}

	// Represent publication of the freshly serialized ABCD checkpoint and
	// prove the next request can select it as the complete reusable prefix.
	fallback.reserved = false
	fallback.turnCheckpoint = &imcSnapshot{
		cachedTokens:      slices.Clone(target[:4]),
		totalTokensCached: 4,
		kvState:           populatedTestSessionStore(),
	}
	nextTarget := []llama.Token{1, 2, 3, 4, 7}
	next := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "user", "content": "r5"}}}, append(slices.Clone(nextTarget), 9), nextTarget, time.Now())
	if next.cacheIdx != 4 {
		t.Errorf("subsequent cacheIdx = %d, want published checkpoint length 4", next.cacheIdx)
	}
}

func TestProcessIMCTokenPlanSelectsLongestCompletePrefix(t *testing.T) {
	m := Model{
		cfg: Config{PtrCacheMinTokens: new(1)},
		log: applog.DiscardLogger,
		imcSessions: []*imcSession{
			{id: 0, cachedTokens: []llama.Token{1}, totalTokensCached: 1, kvState: populatedTestSessionStore()},
			{id: 1, cachedTokens: []llama.Token{1, 2}, totalTokensCached: 2, kvState: populatedTestSessionStore()},
			{id: 2, cachedTokens: []llama.Token{1, 9}, totalTokensCached: 2, kvState: populatedTestSessionStore()},
		},
	}

	actual := []llama.Token{1, 2, 3, 4}
	stable := []llama.Token{1, 2, 3}
	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "user", "content": "x"}}}, actual, stable, time.Now())

	if result.imcSessionID != 1 {
		t.Errorf("imcSessionID = %d, want 1", result.imcSessionID)
	}
	if result.imcMatchKind != "append" {
		t.Errorf("imcMatchKind = %q, want %q", result.imcMatchKind, "append")
	}
	if len(result.imcNewCacheTokens) != 1 || result.imcNewCacheTokens[0] != 3 {
		t.Errorf("imcNewCacheTokens = %v, want [3]", result.imcNewCacheTokens)
	}
	if len(result.imcTailTokens) != 1 || result.imcTailTokens[0] != 4 {
		t.Errorf("imcTailTokens = %v, want [4]", result.imcTailTokens)
	}
}

func TestProcessIMCTokenPlanRejectsNonPrefixRender(t *testing.T) {
	m := Model{cfg: Config{PtrCacheMinTokens: new(1)}}
	result := m.processIMCTokenPlan(context.Background(), nil, []llama.Token{1, 2}, []llama.Token{1, 9}, time.Now())
	if result.imcTokenPlan {
		t.Fatal("imcTokenPlan = true, want false")
	}
}

func TestProcessIMCTokenPlanUsesTurnCheckpointWhenTemplateMovesLastAssistantReasoning(t *testing.T) {
	const script = `{%- for message in messages -%}
{{- message.role + ':' -}}
{%- if message.role == "assistant" and loop.last -%}
{{- message.reasoning_content + ':' -}}
{%- endif -%}
{{- message.content + ';' -}}
{%- endfor -%}
{%- if add_generation_prompt -%}
{{- 'assistant:' -}}
{%- endif -%}`

	m := Model{
		cfg: Config{PtrCacheMinTokens: new(1)},
		log: applog.DiscardLogger,
		template: Template{
			FileName: "last-assistant-reasoning",
			Script:   script,
		},
	}

	render := func(t *testing.T, messages []D, addGenerationPrompt bool) []llama.Token {
		t.Helper()

		prompt, err := m.applyJinjaTemplate(context.Background(), D{
			"messages":              messages,
			"add_generation_prompt": addGenerationPrompt,
			"bos_token":             "",
			"eos_token":             "",
		})
		if err != nil {
			t.Fatalf("applyJinjaTemplate: %v", err)
		}

		tokens := make([]llama.Token, len(prompt))
		for i, b := range []byte(prompt) {
			tokens[i] = llama.Token(b)
		}
		return tokens
	}

	firstMessages := []D{
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "answer", "reasoning_content": "thought"},
	}
	checkpointTokens := render(t, firstMessages[:1], false)
	firstStable := render(t, firstMessages, false)
	m.imcSessions = []*imcSession{
		{
			id:                0,
			cachedTokens:      firstStable,
			totalTokensCached: len(firstStable),
			cachedMsgCount:    len(firstMessages),
			kvState:           populatedTestSessionStore(),
			turnCheckpoint: &imcSnapshot{
				cachedTokens:      checkpointTokens,
				totalTokensCached: len(checkpointTokens),
				cachedMsgCount:    1,
				kvState:           populatedTestSessionStore(),
				endsAtUser:        true,
			},
		},
		{id: 1, kvState: ramSessionStore()},
	}

	secondMessages := append(slices.Clone(firstMessages), D{"role": "user", "content": "next"})
	secondStable := render(t, secondMessages, false)
	secondActual := render(t, secondMessages, true)
	result := m.processIMCTokenPlan(context.Background(), D{"messages": secondMessages}, secondActual, secondStable, time.Now())

	if result.imcMatchKind != "append" {
		t.Errorf("imcMatchKind = %q, want append", result.imcMatchKind)
	}
	if result.imcSessionID != 0 {
		t.Errorf("imcSessionID = %d, want checkpoint session 0", result.imcSessionID)
	}
	if result.cacheIdx != llama.Pos(len(checkpointTokens)) {
		t.Errorf("cacheIdx = %d, want checkpoint length %d", result.cacheIdx, len(checkpointTokens))
	}
	wantCheckpoint := commonTokenPrefixLen(firstStable, secondStable)
	if result.imcCheckpointTokens != wantCheckpoint {
		t.Errorf("imcCheckpointTokens = %d, want divergent rolling LCP %d", result.imcCheckpointTokens, wantCheckpoint)
	}
	if !m.imcSessions[0].reserved {
		t.Error("checkpoint session was not reserved")
	}
	if !slices.Equal(m.imcSessions[0].cachedTokens, checkpointTokens) {
		t.Errorf("rolling tokens after selection = %v, want checkpoint %v", m.imcSessions[0].cachedTokens, checkpointTokens)
	}
	if m.imcSessions[0].turnCheckpoint == nil || !slices.Equal(m.imcSessions[0].turnCheckpoint.cachedTokens, firstStable) {
		t.Error("prior rolling snapshot was not retained after checkpoint selection swap")
	}
}

func TestProcessIMCTokenPlanPrefersLongerRollingSnapshotOverCheckpoint(t *testing.T) {
	session := &imcSession{
		id:                0,
		cachedTokens:      []llama.Token{1, 2, 3},
		totalTokensCached: 3,
		kvState:           populatedTestSessionStore(),
		turnCheckpoint: &imcSnapshot{
			cachedTokens:      []llama.Token{1, 2},
			totalTokensCached: 2,
			kvState:           populatedTestSessionStore(),
			endsAtUser:        true,
		},
	}
	m := Model{
		cfg:         Config{PtrCacheMinTokens: new(1)},
		log:         applog.DiscardLogger,
		imcSessions: []*imcSession{session},
	}

	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "user", "content": "x"}}}, []llama.Token{1, 2, 3, 4, 5}, []llama.Token{1, 2, 3, 4}, time.Now())

	if result.cacheIdx != 3 {
		t.Errorf("cacheIdx = %d, want longer rolling prefix 3", result.cacheIdx)
	}
	if !slices.Equal(session.cachedTokens, []llama.Token{1, 2, 3}) {
		t.Error("planner swapped in shorter checkpoint instead of retaining rolling state")
	}
}

func TestMessagesEndAtRealUser(t *testing.T) {
	tests := []struct {
		name     string
		messages []D
		want     bool
	}{
		{name: "real user", messages: []D{{"role": "user", "content": "next"}}, want: true},
		{name: "tool role", messages: []D{{"role": "tool", "content": "result"}}},
		{name: "user tool id", messages: []D{{"role": "user", "tool_call_id": "call-1", "content": "result"}}},
		{name: "wrapped tool response", messages: []D{{"role": "user", "content": "<tool_response>result</tool_response>"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := messagesEndAtRealUser(tt.messages); got != tt.want {
				t.Errorf("messagesEndAtRealUser() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProcessIMCTokenPlanReservesExactMatch(t *testing.T) {
	d := D{"messages": []D{{"role": "user", "content": "x"}}}
	session := &imcSession{
		id:                0,
		cachedTokens:      []llama.Token{1, 2},
		totalTokensCached: 2,
		cachedMsgCount:    1,
		kvState:           populatedTestSessionStore(),
	}
	m := Model{
		cfg:         Config{PtrCacheMinTokens: new(1)},
		log:         applog.DiscardLogger,
		imcSessions: []*imcSession{session},
	}
	session.cachedRenderInputHash, _ = m.imcRenderFingerprint(d, dMessages(d))

	result := m.processIMCTokenPlan(context.Background(), d, []llama.Token{1, 2, 3}, []llama.Token{1, 2}, time.Now())

	if result.imcMatchKind != "exact" {
		t.Errorf("imcMatchKind = %q, want exact", result.imcMatchKind)
	}
	if !result.imcPureHitSkipSnapshot {
		t.Error("imcPureHitSkipSnapshot = false, want true")
	}
	if !session.reserved {
		t.Error("session.reserved = false, want true")
	}
}

func TestProcessIMCTokenPlanPreservesCompletePrompt(t *testing.T) {
	tests := []struct {
		name      string
		cacheMin  int
		cached    []llama.Token
		stable    []llama.Token
		actual    []llama.Token
		wantMatch string
	}{
		{name: "exact", cacheMin: 1, cached: []llama.Token{1, 2}, stable: []llama.Token{1, 2}, actual: []llama.Token{1, 2, 9}, wantMatch: "exact"},
		{name: "append", cacheMin: 1, cached: []llama.Token{1}, stable: []llama.Token{1, 2}, actual: []llama.Token{1, 2, 9}, wantMatch: "append"},
		{name: "rebuild after divergence", cacheMin: 1, cached: []llama.Token{7}, stable: []llama.Token{1, 2}, actual: []llama.Token{1, 2, 9}, wantMatch: "rebuild"},
		{name: "below minimum", cacheMin: 10, cached: nil, stable: []llama.Token{1, 2}, actual: []llama.Token{1, 2, 9}, wantMatch: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := D{"messages": []D{{"role": "user", "content": "x"}}}
			sessions := []*imcSession{
				{id: 0, cachedTokens: slices.Clone(tt.cached), totalTokensCached: len(tt.cached), kvState: populatedTestSessionStore()},
				{id: 1, kvState: ramSessionStore()},
			}
			m := Model{
				cfg:         Config{PtrCacheMinTokens: &tt.cacheMin},
				log:         applog.DiscardLogger,
				imcSessions: sessions,
			}
			sessions[0].cachedRenderInputHash, _ = m.imcRenderFingerprint(d, dMessages(d))

			result := m.processIMCTokenPlan(context.Background(), d, tt.actual, tt.stable, time.Now())
			if result.imcMatchKind != tt.wantMatch {
				t.Errorf("imcMatchKind = %q, want %q", result.imcMatchKind, tt.wantMatch)
			}
			if !slices.Equal(result.imcSamplerPromptTokens, tt.actual) {
				t.Errorf("imcSamplerPromptTokens = %v, want complete prompt %v", result.imcSamplerPromptTokens, tt.actual)
			}

			got := slices.Clone(tt.actual[:result.cacheIdx])
			got = append(got, result.imcNewCacheTokens...)
			got = append(got, result.imcTailTokens...)
			if !slices.Equal(got, tt.actual) {
				t.Errorf("restored prefix + extension + tail = %v, want %v", got, tt.actual)
			}
		})
	}
}

func TestProcessIMCTokenPlanRebuildsExactTokensWhenRenderFingerprintChanges(t *testing.T) {
	priorD := D{
		"messages":             []D{{"role": "user", "content": "x"}},
		"chat_template_kwargs": D{"custom_mode": "a"},
	}
	currentD := priorD.Clone()
	currentD["chat_template_kwargs"] = D{"custom_mode": "b"}

	m := Model{
		cfg: Config{PtrCacheMinTokens: new(1)},
		log: applog.DiscardLogger,
		imcSessions: []*imcSession{
			{id: 0, cachedTokens: []llama.Token{1, 2}, totalTokensCached: 2, kvState: populatedTestSessionStore()},
		},
	}
	m.imcSessions[0].cachedRenderInputHash, _ = m.imcRenderFingerprint(priorD, dMessages(priorD))

	result := m.processIMCTokenPlan(context.Background(), currentD, []llama.Token{1, 2, 3}, []llama.Token{1, 2}, time.Now())
	if result.imcMatchKind != "rebuild" {
		t.Errorf("imcMatchKind: got %q, want %q", result.imcMatchKind, "rebuild")
	}
	if result.err != nil {
		t.Errorf("err: got %v, want nil", result.err)
	}
}

func populatedTestSessionStore() SessionStore {
	store := ramSessionStore()
	buf := store.Prepare(1)
	buf[0] = 1
	store.Commit(1)
	return store
}
