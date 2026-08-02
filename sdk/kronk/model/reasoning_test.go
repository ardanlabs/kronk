package model

import (
	"context"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

// normalizingParser is a fake Parser that also implements ReasoningNormalizer
// by stripping a <r>...</r> span, used to verify the engine invokes the
// parser hook on assistant content.
type normalizingParser struct{}

func (normalizingParser) Name() string                  { return "normalizing" }
func (normalizingParser) NewStateMachine() StateMachine { return nil }
func (normalizingParser) ToolCall(_ context.Context, _ applog.Logger, _ string) []ResponseToolCall {
	return nil
}

func (normalizingParser) StripReasoningContent(content string) string {
	for {
		i := strings.Index(content, "<r>")
		j := strings.Index(content, "</r>")
		if i < 0 || j < 0 || j < i {
			return content
		}
		content = content[:i] + content[j+len("</r>"):]
	}
}

func (normalizingParser) StripEmptyReasoning(rendered string) string {
	panic("completed template output must not be rewritten")
}

func newReasoningTestModel(imc bool, parser Parser) *Model {
	m := &Model{
		cfg: Config{
			PtrIncrementalCache: new(imc),
		},
		parser: parser,
		log:    noopLog,
	}
	return m
}

func TestNormalizeHistoryReasoning(t *testing.T) {
	makeMsgs := func() []D {
		return []D{
			{"role": "system", "content": "sys"},
			{"role": "user", "content": "u1"},
			{"role": "assistant", "content": "a1<r>secret</r>", "reasoning_content": "deep thought"},
			{"role": "tool", "content": "result", "reasoning": "should-not-be-touched-but-not-assistant"},
			{"role": "assistant", "content": "a2", "reasoning": "more"},
		}
	}

	tests := []struct {
		name   string
		imc    bool
		d      D
		parser Parser
		verify func(t *testing.T, msgs []D)
	}{
		{
			name:   "strips fields and content when IMC on and preserve off",
			imc:    true,
			d:      D{},
			parser: normalizingParser{},
			verify: func(t *testing.T, msgs []D) {
				if _, ok := msgs[2]["reasoning_content"]; ok {
					t.Error("assistant[2] reasoning_content not dropped")
				}
				if got := msgs[2]["content"]; got != "a1" {
					t.Errorf("assistant[2] content: got %q, want %q", got, "a1")
				}
				if _, ok := msgs[4]["reasoning"]; ok {
					t.Error("assistant[4] reasoning not dropped")
				}
				if got := msgs[3]["reasoning"]; got != "should-not-be-touched-but-not-assistant" {
					t.Error("non-assistant message was modified")
				}
			},
		},
		{
			name:   "no-op when preserve_thinking true",
			imc:    true,
			d:      D{"preserve_thinking": true},
			parser: normalizingParser{},
			verify: func(t *testing.T, msgs []D) {
				if _, ok := msgs[2]["reasoning_content"]; !ok {
					t.Error("reasoning_content dropped despite preserve_thinking=true")
				}
			},
		},
		{
			name:   "no-op when IMC disabled",
			imc:    false,
			d:      D{},
			parser: normalizingParser{},
			verify: func(t *testing.T, msgs []D) {
				if _, ok := msgs[4]["reasoning"]; !ok {
					t.Error("reasoning dropped despite IMC disabled")
				}
			},
		},
		{
			name:   "fields dropped even without a normalizer parser",
			imc:    true,
			d:      D{},
			parser: fakeParser{name: "no-normalizer"},
			verify: func(t *testing.T, msgs []D) {
				if _, ok := msgs[2]["reasoning_content"]; ok {
					t.Error("reasoning_content not dropped")
				}
				// Content untouched (parser cannot strip spans).
				if got := msgs[2]["content"]; got != "a1<r>secret</r>" {
					t.Errorf("content modified without normalizer: got %q", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newReasoningTestModel(tt.imc, tt.parser)

			orig := makeMsgs()
			d := tt.d
			d["messages"] = orig

			out := m.normalizeHistoryReasoning(d)
			msgs, _ := out["messages"].([]D)
			tt.verify(t, msgs)

			// Copy-on-write: the original message maps must never be mutated.
			if _, ok := orig[2]["reasoning_content"]; !ok {
				t.Error("original assistant message was mutated (reasoning_content removed)")
			}
		})
	}
}

func TestApplyJinjaTemplatePreservesRenderedReasoningMarkup(t *testing.T) {
	const script = `{%- for message in messages %}
{%- if message.role == "user" %}
{{- '<|im_start|>user\n' + message.content + '<|im_end|>\n' }}
{%- elif message.role == "assistant" %}
{{- '<|im_start|>assistant\n<think>\n\n</think>\n\n' + message.content + '<|im_end|>\n' }}
{%- endif %}
{%- endfor %}
{%- if add_generation_prompt %}
{{- '<|im_start|>assistant\n<think>\n' }}
{%- endif %}`

	m := newReasoningTestModel(true, normalizingParser{})
	m.template = Template{FileName: "reasoning-test", Script: script}

	messages := []D{
		{"role": "user", "content": "explain <think>\n</think> markers"},
		{"role": "assistant", "content": "answer"},
	}
	stableInput := D{
		"messages":              messages,
		"add_generation_prompt": false,
		"bos_token":             "",
		"eos_token":             "<|im_end|>",
	}
	actualInput := D{
		"messages":              messages,
		"add_generation_prompt": true,
		"bos_token":             "",
		"eos_token":             "<|im_end|>",
	}

	stable, err := m.applyJinjaTemplate(context.Background(), stableInput)
	if err != nil {
		t.Fatalf("applyJinjaTemplate stable render: %v", err)
	}
	actual, err := m.applyJinjaTemplate(context.Background(), actualInput)
	if err != nil {
		t.Fatalf("applyJinjaTemplate actual render: %v", err)
	}

	wantStable := "<|im_start|>user\nexplain <think>\n</think> markers<|im_end|>\n" +
		"<|im_start|>assistant\n<think>\n\n</think>\n\nanswer<|im_end|>\n"
	wantActual := wantStable + "<|im_start|>assistant\n<think>\n"
	if stable != wantStable {
		t.Errorf("stable render: got %q, want %q", stable, wantStable)
	}
	if actual != wantActual {
		t.Errorf("actual render: got %q, want %q", actual, wantActual)
	}
	if !strings.HasPrefix(actual, stable) {
		t.Errorf("stable render is not an actual-render prefix: stable %q, actual %q", stable, actual)
	}
}
