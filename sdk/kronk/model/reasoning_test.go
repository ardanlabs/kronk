package model

import (
	"context"
	"strings"
	"testing"
)

func TestApplyJinjaTemplatePreservesRenderedReasoningMarkup(t *testing.T) {
	const script = `{%- for message in messages %}
{%- if message.role == "user" %}
{{- '<|im_start|>user\n' + message.content + '<|im_end|>\n' }}
{%- elif message.role == "assistant" %}
{{- '<|im_start|>assistant\n<think>\n' + message.reasoning_content + '\n</think>\n\n' + message.content + '<|im_end|>\n' }}
{%- endif %}
{%- endfor %}
{%- if add_generation_prompt %}
{{- '<|im_start|>assistant\n<think>\n' }}
{%- endif %}`

	m := Model{log: noopLog,
		template: Template{FileName: "reasoning-test", Script: script}}

	messages := []D{
		{"role": "user", "content": "explain <think>\n</think> markers"},
		{"role": "assistant", "content": "answer", "reasoning_content": "thought"},
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
		"chat_template_kwargs":  D{"preserve_thinking": true},
	}

	stable, err := m.applyJinjaTemplate(context.Background(), stableInput)
	if err != nil {
		t.Fatalf("applyJinjaTemplate stable render: %v", err)
	}
	actual, err := m.applyJinjaTemplate(context.Background(), actualInput)
	if err != nil {
		t.Fatalf("applyJinjaTemplate actual render: %v", err)
	}
	kwargs := actualInput["chat_template_kwargs"].(D)
	if preserve, ok := kwargs["preserve_thinking"].(bool); !ok || !preserve {
		t.Error("chat_template_kwargs preserve_thinking was not passed through unchanged")
	}

	wantStable := "<|im_start|>user\nexplain <think>\n</think> markers<|im_end|>\n" +
		"<|im_start|>assistant\n<think>\nthought\n</think>\n\nanswer<|im_end|>\n"
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
