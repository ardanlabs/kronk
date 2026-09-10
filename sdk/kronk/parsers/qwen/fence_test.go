package qwen

import (
	"context"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

var fenceTools = []model.D{
	{"type": "function", "function": model.D{"name": "submit_findings"}},
}

func fencedMachine(t *testing.T) model.StateMachine {
	t.Helper()
	c := Parser{}.NewStateMachine()
	c.(model.ToolAwareStateMachine).SetTools(fenceTools)
	return c
}

// TestParser_FencedJSONToolCall verifies that a tool-call envelope emitted
// inside a markdown code fence as visible text — the shape Qwen2.5-Coder
// produces at larger prompt sizes — is delivered through the tool channel
// with the fences stripped, exactly like a marked <tool_call> envelope.
func TestParser_FencedJSONToolCall(t *testing.T) {
	c := fencedMachine(t)
	runSteps(t, "fenced-json-tool-call", c, []step{
		{token: "```json\n", channel: model.ChannelNone},
		{token: `{"name":"submit_findings","arguments":{"location":"Paris"}}`, channel: model.ChannelNone},
		{token: "\n```", channel: model.ChannelTool,
			content: `{"name":"submit_findings","arguments":{"location":"Paris"}}` + "\n"},
	})
	_, eog := c.Classify("done")
	if !eog {
		t.Errorf("expected EOG after tool call closed")
	}
}

func TestParser_FencedJSONToolCallSingleToken(t *testing.T) {
	c := fencedMachine(t)
	runSteps(t, "fenced-json-tool-call-single-token", c, []step{
		{token: "```json\n{\"name\":\"submit_findings\",\"arguments\":{}}\n```", channel: model.ChannelTool,
			content: `{"name":"submit_findings","arguments":{}}` + "\n"},
	})
}

func TestParser_FencedJSONToolCallSplitTokens(t *testing.T) {
	c := fencedMachine(t)
	runSteps(t, "fenced-json-tool-call-split-tokens", c, []step{
		{token: "`", channel: model.ChannelNone},
		{token: "`", channel: model.ChannelNone},
		{token: "`json\n", channel: model.ChannelNone},
		{token: `{"name":`, channel: model.ChannelNone},
		{token: `"submit_findings","arguments":{}}`, channel: model.ChannelNone},
		{token: "\n", channel: model.ChannelNone},
		{token: "```", channel: model.ChannelTool,
			content: `{"name":"submit_findings","arguments":{}}` + "\n"},
	})
}

func TestParser_FencedBareTagToolCall(t *testing.T) {
	c := fencedMachine(t)
	runSteps(t, "fenced-bare-tag-tool-call", c, []step{
		{token: "```\n", channel: model.ChannelNone},
		{token: `{"name":"submit_findings","arguments":{}}`, channel: model.ChannelNone},
		{token: "\n```", channel: model.ChannelTool,
			content: `{"name":"submit_findings","arguments":{}}` + "\n"},
	})
}

// TestParser_FencedUndeclaredNameStreamsAsAnswer verifies the declared-tool
// gate: fenced JSON naming a function the request did not declare is ordinary
// content and passes through verbatim, never delivered as a call.
func TestParser_FencedUndeclaredNameStreamsAsAnswer(t *testing.T) {
	c := fencedMachine(t)
	const undeclared = "```json\n" + `{"name":"other_tool","arguments":{}}` + "\n```"
	runSteps(t, "fenced-undeclared-name", c, []step{
		{token: "```json\n", channel: model.ChannelNone},
		// The body is envelope-shaped, so it is held until the close fence;
		// only the complete JSON reveals the undeclared name.
		{token: `{"name":"other_tool","arguments":{}}`, channel: model.ChannelNone},
		{token: "\n```", channel: model.ChannelAnswer, content: undeclared},
	})
}

// TestParser_FencedJSONWithoutDeclaredToolsStreamsAsAnswer verifies the hold
// never engages when the request declares no tools: fenced JSON answers
// stream untouched.
func TestParser_FencedJSONWithoutDeclaredToolsStreamsAsAnswer(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "fenced-json-no-tools", c, []step{
		{token: "```json\n", channel: model.ChannelAnswer, content: "```json\n"},
		{token: `{"name":"submit_findings"}`, channel: model.ChannelAnswer, content: `{"name":"submit_findings"}`},
		{token: "\n```", channel: model.ChannelAnswer, content: "\n```"},
	})
}

// TestParser_WhitespaceThenTaggedToolCallUnaffected verifies the fence hold
// does not disturb an ordinary marked call whose reply begins with
// whitespace: the whitespace streams and the marker still opens the call.
func TestParser_WhitespaceThenTaggedToolCallUnaffected(t *testing.T) {
	c := fencedMachine(t)
	runSteps(t, "whitespace-then-tagged-tool-call", c, []step{
		{token: "\n", channel: model.ChannelAnswer, content: "\n"},
		{token: "<tool_call>", channel: model.ChannelTool},
		{token: `{"name":"submit_findings","arguments":{}}`, channel: model.ChannelNone},
		{token: "</tool_call>", channel: model.ChannelTool,
			content: `{"name":"submit_findings","arguments":{}}` + "\n"},
	})
}

// TestParser_FencedCodeStreamsAsAnswer verifies that fenced content which is
// not a tool-call envelope passes through verbatim, fences included, and that
// it is released as soon as the body proves it is not an envelope rather than
// held to end of generation.
func TestParser_FencedCodeStreamsAsAnswer(t *testing.T) {
	c := fencedMachine(t)
	runSteps(t, "fenced-code-streams-as-answer", c, []step{
		{token: "```go\n", channel: model.ChannelNone},
		{token: "func", channel: model.ChannelAnswer, content: "```go\nfunc"},
		{token: " main() {}\n", channel: model.ChannelAnswer, content: " main() {}\n"},
		{token: "```", channel: model.ChannelAnswer, content: "```"},
	})
}

func TestParser_FencedProseInfoStringStreamsAsAnswer(t *testing.T) {
	c := fencedMachine(t)
	runSteps(t, "fenced-prose-info-string", c, []step{
		{token: "```here is a note\n", channel: model.ChannelAnswer, content: "```here is a note\n"},
		{token: "hello", channel: model.ChannelAnswer, content: "hello"},
	})
}

func TestParser_InlineBacktickProseStreamsAsAnswer(t *testing.T) {
	c := fencedMachine(t)
	runSteps(t, "inline-backtick-prose", c, []step{
		{token: "`", channel: model.ChannelNone},
		{token: "code` is inline", channel: model.ChannelAnswer, content: "`code` is inline"},
	})
}

// TestParser_FenceAfterProseIgnored verifies the hold engages only at the
// start of a reply: prose containing a fence later on streams untouched.
func TestParser_FenceAfterProseIgnored(t *testing.T) {
	c := fencedMachine(t)
	runSteps(t, "fence-after-prose", c, []step{
		{token: "Here is code:", channel: model.ChannelAnswer, content: "Here is code:"},
		{token: "```json\n{\"name\":\"submit_findings\",\"arguments\":{}}\n```",
			channel: model.ChannelAnswer, content: "```json\n{\"name\":\"submit_findings\",\"arguments\":{}}\n```"},
	})
}

func TestFlush_UnclosedFenceEnvelope(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.(model.ToolAwareStateMachine).SetTools(fenceTools)

	var tooling strings.Builder
	for _, token := range []string{"```json\n", `{"name":"submit_findings","arguments":{"location":"Paris"}}`} {
		result, eog := c.Classify(token)
		if eog {
			t.Fatalf("Classify(%q): got EOG before tool call completed", token)
		}
		if result.Channel == model.ChannelTool {
			tooling.WriteString(result.Content)
		}
	}
	flusher := c.(model.StateMachineFlusher)
	tooling.WriteString(flusher.Flush().Content)

	calls := Parser{}.ToolCall(context.Background(), noopLog, tooling.String())
	if len(calls) != 1 {
		t.Fatalf("ToolCall: got %d calls, want 1", len(calls))
	}
	if got, want := calls[0].Function.Name, "submit_findings"; got != want {
		t.Errorf("Function.Name: got %q, want %q", got, want)
	}
}

func TestParser_FencedNonEnvelopeUnderJsonTagReleased(t *testing.T) {
	c := fencedMachine(t)
	runSteps(t, "fenced-non-envelope-under-json-tag", c, []step{
		{token: "```json\n", channel: model.ChannelNone},
		{token: "not an envelope", channel: model.ChannelAnswer, content: "```json\nnot an envelope"},
	})
	flusher := c.(model.StateMachineFlusher)
	if result := flusher.Flush(); result.Channel != model.ChannelNone {
		t.Errorf("Flush after mid-stream release: got (%v, %q), want zero result",
			result.Channel, result.Content)
	}
}

func TestFenceBodyOf(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{"closed json", "```json\n{}\n```", "{}", true},
		{"closed bare", "```\n{}\n```", "{}", true},
		{"unclosed", "```json\n{}", "{}", true},
		{"leading whitespace", "\n```json\n{}\n```", "{}", true},
		{"prose tag", "```some prose\n{}\n```", "", false},
		{"no fence", "{}", "", false},
		{"opener only", "```json", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := fenceBodyOf(tt.in)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("fenceBodyOf(%q) = (%q, %v), want (%q, %v)",
					tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
