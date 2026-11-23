package model

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/beam"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/deepseek"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/fallback"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/gemma"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/glm"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/gpt"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/inkling"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/kimi"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/lfm"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/llama"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/mistral"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/nemotron"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/qwen"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/toolcall"
	yzllama "github.com/hybridgroup/yzma/pkg/llama"
)

// selectDefaultParser keeps explicit protocols ahead of architecture lineages:
// Inkling message blocks and Beam XML-Harmony precede GPT JSON-Harmony, and
// marked tool-call protocols precede inherited architecture metadata.
// The catch-all remains last.
func selectDefaultParser(fp Fingerprint) Parser {
	factories := [...]func(parsers.Fingerprint) (parsers.Parser, bool){
		inkling.New,
		beam.New,
		gpt.New,
		lfm.New,
		kimi.New,
		deepseek.New,
		llama.New,
		nemotron.New,
		qwen.New,
		toolcall.New,
		gemma.New,
		glm.New,
		mistral.New,
		fallback.New,
	}

	for _, factory := range factories {
		p, ok := factory(parsers.Fingerprint(fp))
		if ok {
			return &builtinParser{parser: p}
		}
	}
	return nil
}

// builtinParser bridges internal parsing data to the public custom-parser
// contract. Family implementations never import their parent model package.
type builtinParser struct {
	parser parsers.Parser
	vocab  yzllama.Vocab
}

func (p *builtinParser) Name() string { return p.parser.Name() }

func (p *builtinParser) NewStateMachine() StateMachine {
	sm := &builtinStateMachine{state: p.parser.NewStateMachine()}
	if state, ok := sm.state.(parsers.TokenVocabularyStateMachine); ok && p.vocab != 0 {
		state.SetTokenVocabulary(func(marker string) bool {
			tokens := yzllama.Tokenize(p.vocab, marker, false, true)
			if len(tokens) != 1 || yzllama.VocabGetText(p.vocab, tokens[0]) != marker {
				return false
			}
			return yzllama.VocabGetAttr(p.vocab, tokens[0])&(yzllama.TokenAttrControl|yzllama.TokenAttrUserDef) != 0
		})
	}
	if state, ok := sm.state.(parsers.TokenStateMachine); ok {
		return &builtinTokenStateMachine{builtinStateMachine: sm, tokenState: state}
	}
	return sm
}

func (p *builtinParser) ToolCall(ctx context.Context, log applog.Logger, buf string) []ResponseToolCall {
	return publicToolCalls(p.parser.ToolCall(ctx, log, buf))
}

func (p *builtinParser) ToolCallWithSchema(ctx context.Context, log applog.Logger, buf string, tools []D) []ResponseToolCall {
	if parser, ok := p.parser.(parsers.ToolCallSchemaParser); ok {
		return publicToolCalls(parser.ToolCallWithSchema(ctx, log, buf, parserTools(tools)))
	}
	return p.ToolCall(ctx, log, buf)
}

func (p *builtinParser) AdjustParams(params Params) Params {
	if parser, ok := p.parser.(parsers.ParamsAdjuster); ok {
		adjusted := parser.AdjustParams(parsers.Params{ReasoningEffort: params.ReasoningEffort})
		params.ReasoningEffort = adjusted.ReasoningEffort
	}
	return params
}

type builtinStateMachine struct {
	state parsers.StateMachine
}

// builtinTokenStateMachine exposes token provenance only for families that
// consume it, leaving other built-ins on the text-only generation path.
type builtinTokenStateMachine struct {
	*builtinStateMachine
	tokenState parsers.TokenStateMachine
}

func (sm *builtinTokenStateMachine) ClassifyToken(content string, special bool) (Result, bool) {
	result, eog := sm.tokenState.ClassifyToken(content, special)
	return publicParserResult(result), eog
}

func (sm *builtinStateMachine) Classify(content string) (Result, bool) {
	result, eog := sm.state.Classify(content)
	return publicParserResult(result), eog
}

func (sm *builtinStateMachine) Reset() { sm.state.Reset() }

func (sm *builtinStateMachine) ConsumeVocabEOG(content string) {
	if consumer, ok := sm.state.(parsers.VocabEOGConsumer); ok {
		consumer.ConsumeVocabEOG(content)
	}
}

func (sm *builtinStateMachine) SetTools(tools []D) {
	if consumer, ok := sm.state.(parsers.ToolAwareStateMachine); ok {
		consumer.SetTools(parserTools(tools))
	}
}

func (sm *builtinStateMachine) Flush() Result {
	if flusher, ok := sm.state.(parsers.StateMachineFlusher); ok {
		return publicParserResult(flusher.Flush())
	}
	return Result{}
}

func (sm *builtinStateMachine) ToolCallDeltas() []ResponseToolCallDelta {
	if streamer, ok := sm.state.(parsers.ToolCallDeltaStreamer); ok {
		return publicToolCallDeltas(streamer.ToolCallDeltas())
	}
	return nil
}

func (sm *builtinStateMachine) StartedToolCalls() []ResponseToolCallDelta {
	if streamer, ok := sm.state.(parsers.ToolCallDeltaStreamer); ok {
		return publicToolCallDeltas(streamer.StartedToolCalls())
	}
	return nil
}

func publicParserResult(result parsers.Result) Result {
	channel := ChannelNone
	switch result.Channel {
	case parsers.ChannelReasoning:
		channel = ChannelReasoning
	case parsers.ChannelAnswer:
		channel = ChannelAnswer
	case parsers.ChannelTool:
		channel = ChannelTool
	}
	return Result{Channel: channel, Content: result.Content}
}

func publicToolCalls(calls []parsers.ResponseToolCall) []ResponseToolCall {
	if calls == nil {
		return nil
	}
	result := make([]ResponseToolCall, len(calls))
	for i, call := range calls {
		result[i] = ResponseToolCall{
			ID: call.ID, Index: call.Index, Type: call.Type,
			Function: ResponseToolCallFunction{
				Name: call.Function.Name, Arguments: ToolCallArguments(call.Function.Arguments),
			},
			Status: call.Status, Raw: call.Raw, Error: call.Error,
		}
	}
	return result
}

func publicToolCallDeltas(deltas []parsers.ResponseToolCallDelta) []ResponseToolCallDelta {
	if deltas == nil {
		return nil
	}
	result := make([]ResponseToolCallDelta, len(deltas))
	for i, delta := range deltas {
		result[i] = ResponseToolCallDelta{
			ID: delta.ID, Index: delta.Index, Type: delta.Type,
			Function: ResponseToolCallDeltaFunction(delta.Function),
		}
	}
	return result
}

func parserTools(tools []D) []parsers.D {
	if tools == nil {
		return nil
	}
	result := make([]parsers.D, len(tools))
	for i, tool := range tools {
		result[i] = parserValue(tool).(parsers.D)
	}
	return result
}

func parserValue(value any) any {
	switch value := value.(type) {
	case D:
		if value == nil {
			return parsers.D(nil)
		}
		result := make(parsers.D, len(value))
		for key, item := range value {
			result[key] = parserValue(item)
		}
		return result
	case []D:
		return parserTools(value)
	case []any:
		if value == nil {
			return []any(nil)
		}
		result := make([]any, len(value))
		for i, item := range value {
			result[i] = parserValue(item)
		}
		return result
	default:
		return value
	}
}
