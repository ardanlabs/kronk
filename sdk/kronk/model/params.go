package model

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	// DefAdaptivePDecay controls how quickly the Adaptive-P sampler adjusts.
	DefAdaptivePDecay = 0.0

	// DefAdaptivePTarget is the target probability threshold for Adaptive-P
	// sampling. When > 0, enables adaptive sampling that dynamically adjusts
	// based on the probability distribution to prevent predictable patterns.
	DefAdaptivePTarget = 0.0

	// DefDryAllowedLen is the minimum n-gram length before DRY applies.
	DefDryAllowedLen = 2

	// DefDryBase is the base for exponential penalty growth in DRY.
	DefDryBase = 1.75

	// DefDryMultiplier controls the DRY (Don't Repeat Yourself) sampler which penalizes
	// n-gram pattern repetition. 0.8 - Light repetition penalty,
	// 1.0–1.5 - Moderate (typical starting point), 2.0–3.0 - Aggressive.
	// Default is 0.0 (disabled) to match Ollama and maximize tool calling stability.
	DefDryMultiplier = 0.0

	// DefDryPenaltyLast limits how many recent tokens DRY considers. A value
	// of -1 uses the full context.
	DefDryPenaltyLast = -1

	// DefEnableThinking determines if the model should think or not. It is used for
	// most non-GPT models. It accepts 1, t, T, TRUE, true, True, 0, f, F, FALSE,
	// false, False.
	DefEnableThinking = ThinkingEnabled

	// DefFrequencyPenalty penalizes tokens proportionally to how often they have
	// appeared in the output. Higher values more strongly discourage frequent
	// repetition. Default is 0.0 (disabled).
	DefFrequencyPenalty float32 = 0.0

	// DefIncludeUsage determines whether to include token usage information in
	// streaming responses.
	DefIncludeUsage = true

	// DefLogprobs determines whether to return log probabilities of output tokens.
	// When enabled, the response includes probability data for each generated token.
	DefLogprobs = false

	// DefMaxTokens exists for backward compatibility. When max_tokens is
	// not specified in a request, adjustParams defaults to the model's
	// context window size.
	DefMaxTokens = 4096

	// DefMaxTopLogprobs defines the number of maximum logprobs to use.
	DefMaxTopLogprobs = 5

	// DefMinP is a dynamic sampling threshold that helps balance the coherence
	// (quality) and diversity (creativity) of the generated text.
	DefMinP = 0.0

	// DefPresencePenalty applies a flat penalty to any token that has already
	// appeared in the output, regardless of frequency. Higher values encourage
	// the model to introduce new topics. Default is 0.0 (disabled).
	DefPresencePenalty float32 = 0.0

	// DefReasoningEffort is a string that specifies the level of reasoning effort to
	// use for GPT models.
	DefReasoningEffort = ReasoningEffortMedium

	// DefRepeatLastN specifies how many recent tokens to consider when applying the
	// repetition penalty. A larger value considers more context but may be slower.
	DefRepeatLastN = 64

	// DefRepeatPenalty applies a penalty to tokens that have already appeared in the
	// output, reducing repetitive text. A value of 1.0 means no penalty. Values
	// above 1.0 reduce repetition (e.g., 1.1 is a mild penalty, 1.5 is strong).
	// Default is 1.0 (disabled) because even mild penalties suppress structural
	// JSON tokens like { in tool call formats (e.g., Gemma's call:func{{...}}),
	// causing the model to substitute [ for { and producing invalid arguments.
	DefRepeatPenalty = 1.0

	// DefReturnPrompt determines whether to include the prompt in the final response.
	// When set to true, the prompt will be included.
	DefReturnPrompt = false

	// DefTemp controls the randomness of the output. It rescales the probability
	// distribution of possible next tokens.
	DefTemp = 0.8

	// DefTopK limits the pool of possible next tokens to the K number of most
	// probable tokens. Default is 40 to match Ollama and cut off low-probability
	// tokens that can break structured output like tool calls.
	DefTopK int32 = 40

	// DefTopLogprobs specifies how many of the most likely tokens to return at each
	// position, along with their log probabilities. Must be between 0 and 5.
	// Setting this to a value > 0 implicitly enables logprobs.
	DefTopLogprobs = 0

	// DefTopP, also known as nucleus sampling, works differently than top_k by
	// selecting a dynamic pool of tokens whose cumulative probability exceeds a
	// threshold P. Instead of a fixed number of tokens (K), it selects the minimum
	// number of most probable tokens required to reach the cumulative probability P.
	DefTopP = 0.9

	// DefXtcMinKeep is the minimum tokens to keep after XTC culling.
	DefXtcMinKeep = 1

	// DefXtcProbability controls XTC (eXtreme Token Culling) which randomly removes
	// tokens close to top probability. Must be > 0 to activate.
	DefXtcProbability = 0.0

	// DefXtcThreshold is the probability threshold for XTC culling.
	DefXtcThreshold = 0.1
)

const (
	// The model will perform thinking. This is the default setting.
	ThinkingEnabled = "true"

	// The model will not perform thinking.
	ThinkingDisabled = "false"
)

const (
	// The model does not perform reasoning This setting is fastest and lowest
	// cost, ideal for latency-sensitive tasks that do not require complex logic,
	// such as simple translation or data reformatting.
	ReasoningEffortNone = "none"

	// GPT: A very low amount of internal reasoning, optimized for throughput
	// and speed.
	ReasoningEffortMinimal = "minimal"

	// GPT: Light reasoning that favors speed and lower token usage, suitable
	// for triage or short answers.
	ReasoningEffortLow = "low"

	// GPT: The default setting, providing a balance between speed and reasoning
	// accuracy. This is a good general-purpose choice for most tasks like
	// content drafting or standard Q&A.
	ReasoningEffortMedium = "medium"

	// GPT: Extensive reasoning for complex, multi-step problems. This setting
	// leads to the most thorough and accurate analysis but increases latency
	// and cost due to a larger number of internal reasoning tokens used.
	ReasoningEffortHigh = "high"
)

type Params struct {
	// AdaptivePDecay controls how quickly the Adaptive-P sampler adjusts.
	// Default is 0.0.
	AdaptivePDecay float32 `json:"adaptive_p_decay"`

	// AdaptivePTarget is the target probability threshold for Adaptive-P
	// sampling. When > 0, enables adaptive sampling that dynamically adjusts
	// based on the probability distribution to prevent predictable patterns.
	// Default is 0.0 (disabled).
	AdaptivePTarget float32 `json:"adaptive_p_target"`

	// DryAllowedLen is the minimum n-gram length before DRY applies. Default is 2.
	DryAllowedLen int32 `json:"dry_allowed_length"`

	// DryBase is the base for exponential penalty growth in DRY. Default is 1.75.
	DryBase float32 `json:"dry_base"`

	// DryMultiplier controls the DRY (Don't Repeat Yourself) sampler which
	// penalizes n-gram pattern repetition. 0.8 - Light repetition penalty,
	// 1.0–1.5 - Moderate (typical starting point), 2.0–3.0 - Aggressive.
	// Default is 1.05.
	DryMultiplier float32 `json:"dry_multiplier"`

	// DryPenaltyLast limits how many recent tokens DRY considers. A value of 0
	// disables DRY and -1 uses the full context.
	DryPenaltyLast int32 `json:"dry_penalty_last_n"`

	// FrequencyPenalty penalizes tokens proportionally to how often they have
	// appeared in the output. Higher values more strongly discourage frequent
	// repetition. Default is 0.0 (disabled).
	FrequencyPenalty float32 `json:"frequency_penalty"`

	// Grammar constrains output to match a GBNF grammar specification.
	// When set, the model output will be forced to conform to this grammar.
	// Use preset grammars like GrammarJSON or generate from JSON Schema.
	Grammar string `json:"grammar"`

	// IncludeUsage determines whether to include token usage information in
	// streaming responses. Default is true.
	IncludeUsage bool `json:"include_usage"`

	// Logprobs determines whether to return log probabilities of output tokens.
	// When enabled, the response includes probability data for each generated
	// token. Default is false.
	Logprobs bool `json:"logprobs"`

	// MaxTokens is the maximum tokens for generation when not derived from the
	// model's context window the default is 4096.
	MaxTokens int `json:"max_tokens"`

	// MinP is a dynamic sampling threshold that helps balance the coherence
	// (quality) and diversity (creativity) of the generated text. Default is 0.0.
	MinP float32 `json:"min_p"`

	// PresencePenalty applies a flat penalty to any token that has already
	// appeared in the output, regardless of frequency. Higher values encourage
	// the model to introduce new topics. Default is 0.0 (disabled).
	PresencePenalty float32 `json:"presence_penalty"`

	// ReasoningEffort is a string that specifies the level of reasoning effort
	// to use for GPT models. Default is ReasoningEffortMedium.
	ReasoningEffort string `json:"reasoning_effort"`

	// RepeatLastN specifies how many recent tokens to consider when applying
	// the repetition penalty. A larger value considers more context but may be
	// slower. Default is 64.
	RepeatLastN int32 `json:"repeat_last_n"`

	// RepeatPenalty applies a penalty to tokens that have already appeared in
	// the output, reducing repetitive text. A value of 1.0 means no penalty.
	// Values above 1.0 reduce repetition (e.g., 1.1 is a mild penalty, 1.5 is
	// strong). Default is 1.0 which turns it off.
	RepeatPenalty float32 `json:"repeat_penalty"`

	// ReturnPrompt determines whether to include the prompt in the final
	// response. When set to true, the prompt will be included. Default is false.
	ReturnPrompt bool `json:"return_prompt"`

	// Seed initializes request sampling randomness. Nil selects a random seed;
	// any non-nil value, including 0, requests repeatable sampling.
	Seed *uint32 `json:"seed,omitempty"`

	// Stream determines whether to stream the response.
	Stream bool `json:"stream"`

	// Temperature controls the randomness of the output. It rescales the
	// probability distribution of possible next tokens. Default is 0.8.
	Temperature float32 `json:"temperature"`

	// Thinking determines if the model should think or not. It is used for most
	// non-GPT models. It accepts 1, t, T, TRUE, true, True, 0, f, F, FALSE,
	// false, False. Default is "true".
	Thinking string `json:"enable_thinking"`

	// TopK limits the pool of possible next tokens to the K number of most
	// probable tokens. If a model predicts 10,000 possible next tokens, setting
	// top_k to 50 means only the 50 tokens with the highest probabilities are
	// considered for selection (after temperature scaling). Default is 40.
	TopK int32 `json:"top_k"`

	// TopLogprobs specifies how many of the most likely tokens to return at
	// each position, along with their log probabilities. Must be between 0 and
	// 5. Setting this to a value > 0 implicitly enables logprobs. Default is 0.
	TopLogprobs int `json:"top_logprobs"`

	// TopP, also known as nucleus sampling, works differently than top_k by
	// selecting a dynamic pool of tokens whose cumulative probability exceeds a
	// threshold P. Instead of a fixed number of tokens (K), it selects the
	// minimum number of most probable tokens required to reach the cumulative
	// probability P. Default is 0.9.
	TopP float32 `json:"top_p"`

	// XtcMinKeep is the minimum tokens to keep after XTC culling. Default is 1.
	XtcMinKeep uint32 `json:"xtc_min_keep"`

	// XtcProbability controls XTC (eXtreme Token Culling) which randomly removes
	// tokens close to top probability. Must be > 0 to activate. Default is 0.0
	// (disabled).
	XtcProbability float32 `json:"xtc_probability"`

	// XtcThreshold is the probability threshold for XTC culling. Default is 0.1.
	XtcThreshold float32 `json:"xtc_threshold"`
}

// String returns a string representation of all resolved Params values in the
// format key[value]\nkey[value]\n ... Grammar contents are intentionally
// redacted; only whether a grammar is active is reported.
func (p Params) String() string {
	var b strings.Builder

	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "adaptive_p_decay[%v]\n", p.AdaptivePDecay)
	fmt.Fprintf(&b, "adaptive_p_target[%v]\n", p.AdaptivePTarget)
	fmt.Fprintf(&b, "dry_allowed_length[%v]\n", p.DryAllowedLen)
	fmt.Fprintf(&b, "dry_base[%v]\n", p.DryBase)
	fmt.Fprintf(&b, "dry_multiplier[%v]\n", p.DryMultiplier)
	fmt.Fprintf(&b, "dry_penalty_last_n[%v]\n", p.DryPenaltyLast)
	fmt.Fprintf(&b, "frequency_penalty[%v]\n", p.FrequencyPenalty)
	fmt.Fprintf(&b, "grammar[%v]\n", p.Grammar != "")
	fmt.Fprintf(&b, "include_usage[%v]\n", p.IncludeUsage)
	fmt.Fprintf(&b, "logprobs[%v]\n", p.Logprobs)
	fmt.Fprintf(&b, "max_tokens[%v]\n", p.MaxTokens)
	fmt.Fprintf(&b, "min_p[%v]\n", p.MinP)
	fmt.Fprintf(&b, "presence_penalty[%v]\n", p.PresencePenalty)
	fmt.Fprintf(&b, "reasoning_effort[%v]\n", p.ReasoningEffort)
	fmt.Fprintf(&b, "repeat_last_n[%v]\n", p.RepeatLastN)
	fmt.Fprintf(&b, "repeat_penalty[%v]\n", p.RepeatPenalty)
	fmt.Fprintf(&b, "return_prompt[%v]\n", p.ReturnPrompt)
	if p.Seed == nil {
		fmt.Fprintln(&b, "seed[random]")
	} else {
		fmt.Fprintf(&b, "seed[%d]\n", *p.Seed)
	}
	fmt.Fprintf(&b, "stream[%v]\n", p.Stream)
	fmt.Fprintf(&b, "temperature[%v]\n", p.Temperature)
	fmt.Fprintf(&b, "enable_thinking[%v]\n", p.Thinking)
	fmt.Fprintf(&b, "top_k[%v]\n", p.TopK)
	fmt.Fprintf(&b, "top_logprobs[%v]\n", p.TopLogprobs)
	fmt.Fprintf(&b, "top_p[%v]\n", p.TopP)
	fmt.Fprintf(&b, "xtc_min_keep[%v]\n", p.XtcMinKeep)
	fmt.Fprintf(&b, "xtc_probability[%v]\n", p.XtcProbability)
	fmt.Fprintf(&b, "xtc_threshold[%v]\n", p.XtcThreshold)

	return strings.TrimSuffix(b.String(), " ")
}

// AddParams adds the values from the Params struct into the provided D map.
// Only non-zero values are added.
func AddParams(params Params, d D) {
	if params.AdaptivePDecay != 0 {
		d["adaptive_p_decay"] = params.AdaptivePDecay
	}
	if params.AdaptivePTarget != 0 {
		d["adaptive_p_target"] = params.AdaptivePTarget
	}
	if params.DryAllowedLen != 0 {
		d["dry_allowed_length"] = params.DryAllowedLen
	}
	if params.DryBase != 0 {
		d["dry_base"] = params.DryBase
	}
	if params.DryMultiplier != 0 {
		d["dry_multiplier"] = params.DryMultiplier
	}
	if params.DryPenaltyLast != 0 {
		d["dry_penalty_last_n"] = params.DryPenaltyLast
	}
	if params.FrequencyPenalty != 0 {
		d["frequency_penalty"] = params.FrequencyPenalty
	}
	if params.Grammar != "" {
		d["grammar"] = params.Grammar
	}
	if params.IncludeUsage {
		d["include_usage"] = params.IncludeUsage
	}
	if params.Logprobs {
		d["logprobs"] = params.Logprobs
	}
	if params.MaxTokens != 0 {
		d["max_tokens"] = params.MaxTokens
	}
	if params.MinP != 0 {
		d["min_p"] = params.MinP
	}
	if params.PresencePenalty != 0 {
		d["presence_penalty"] = params.PresencePenalty
	}
	if params.ReasoningEffort != "" {
		d["reasoning_effort"] = params.ReasoningEffort
	}
	if params.RepeatLastN != 0 {
		d["repeat_last_n"] = params.RepeatLastN
	}
	if params.RepeatPenalty != 0 {
		d["repeat_penalty"] = params.RepeatPenalty
	}
	if params.ReturnPrompt {
		d["return_prompt"] = params.ReturnPrompt
	}
	if params.Seed != nil {
		d["seed"] = *params.Seed
	}
	if params.Stream {
		d["stream"] = params.Stream
	}
	if params.Temperature != 0 {
		d["temperature"] = params.Temperature
	}
	if params.Thinking != "" {
		d["enable_thinking"] = params.Thinking == "true"
	}
	if params.TopK != 0 {
		d["top_k"] = params.TopK
	}
	if params.TopLogprobs != 0 {
		d["top_logprobs"] = params.TopLogprobs
	}
	if params.TopP != 0 {
		d["top_p"] = params.TopP
	}
	if params.XtcMinKeep != 0 {
		d["xtc_min_keep"] = params.XtcMinKeep
	}
	if params.XtcProbability != 0 {
		d["xtc_probability"] = params.XtcProbability
	}
	if params.XtcThreshold != 0 {
		d["xtc_threshold"] = params.XtcThreshold
	}
}

func (m *Model) parseParams(ctx context.Context, d D) (Params, error) {
	m.log(ctx, "parse-params", "request", d.String())

	p := m.cfg.DefaultParams
	p.Seed = nil

	if val, exists := d["adaptive_p_decay"]; exists {
		adaptivePDecay, err := parseFloat32("adaptive_p_decay", val)
		if err != nil {
			return Params{}, err
		}
		p.AdaptivePDecay = adaptivePDecay
	}

	if val, exists := d["adaptive_p_target"]; exists {
		adaptivePTarget, err := parseFloat32("adaptive_p_target", val)
		if err != nil {
			return Params{}, err
		}
		p.AdaptivePTarget = adaptivePTarget
	}

	if val, exists := d["dry_allowed_length"]; exists {
		dryAllowedLen, err := parseInt("dry_allowed_length", val)
		if err != nil {
			return Params{}, err
		}
		p.DryAllowedLen = int32(dryAllowedLen)
	}

	if val, exists := d["dry_base"]; exists {
		dryBase, err := parseFloat32("dry_base", val)
		if err != nil {
			return Params{}, err
		}
		p.DryBase = dryBase
	}

	if val, exists := d["dry_multiplier"]; exists {
		dryMultiplier, err := parseFloat32("dry_multiplier", val)
		if err != nil {
			return Params{}, err
		}
		p.DryMultiplier = dryMultiplier
	}

	if val, exists := d["dry_penalty_last_n"]; exists {
		dryPenaltyLast, err := parseInt("dry_penalty_last_n", val)
		if err != nil {
			return Params{}, err
		}
		p.DryPenaltyLast = int32(dryPenaltyLast)
	}

	if val, exists := d["frequency_penalty"]; exists {
		fp, err := parseFloat32("frequency_penalty", val)
		if err != nil {
			return Params{}, err
		}
		p.FrequencyPenalty = fp
	}

	if val, exists := d["enable_thinking"]; exists {
		enableThinking, err := parseBool("enable_thinking", val)
		if err != nil {
			return Params{}, err
		}
		p.Thinking = strconv.FormatBool(enableThinking)
	}

	if val, exists := d["grammar"]; exists {
		if grammar, ok := val.(string); ok {
			p.Grammar = grammar
		}
	}

	if val, exists := d["json_schema"]; exists {
		grammar, err := fromJSONSchema(val)
		if err != nil {
			return Params{}, fmt.Errorf("to-params: %w", err)
		}
		p.Grammar = grammar
	}

	// OpenAI-compatible response_format. Only applied when neither an explicit
	// grammar nor a json_schema field has already been provided so the
	// Kronk-native fields take precedence.
	if val, exists := d["response_format"]; exists && p.Grammar == "" {
		grammar, err := fromResponseFormat(val)
		if err != nil {
			return Params{}, fmt.Errorf("to-params: %w", err)
		}
		if grammar != "" {
			p.Grammar = grammar
		}
	}

	if val, exists := d["logprobs"]; exists {
		logprobs, err := parseBool("logprobs", val)
		if err != nil {
			return Params{}, err
		}
		p.Logprobs = logprobs
	}

	if val, exists := d["max_tokens"]; exists {
		maxTokens, err := parseInt("max_tokens", val)
		if err != nil {
			return Params{}, err
		}
		p.MaxTokens = maxTokens
	}

	if val, exists := d["min_p"]; exists {
		minP, err := parseFloat32("min_p", val)
		if err != nil {
			return Params{}, err
		}
		p.MinP = minP
	}

	if val, exists := d["presence_penalty"]; exists {
		pp, err := parseFloat32("presence_penalty", val)
		if err != nil {
			return Params{}, err
		}
		p.PresencePenalty = pp
	}

	if val, exists := d["reasoning_effort"]; exists {
		reasoningEffort, err := parseReasoningString("reasoning_effort", val)
		if err != nil {
			return Params{}, err
		}
		p.ReasoningEffort = reasoningEffort
	}

	if val, exists := d["repeat_last_n"]; exists {
		repeatLastN, err := parseInt("repeat_last_n", val)
		if err != nil {
			return Params{}, err
		}
		p.RepeatLastN = int32(repeatLastN)
	}

	if val, exists := d["repeat_penalty"]; exists {
		repeatPenalty, err := parseFloat32("repeat_penalty", val)
		if err != nil {
			return Params{}, err
		}
		p.RepeatPenalty = repeatPenalty
	}

	if val, exists := d["return_prompt"]; exists {
		returnPrompt, err := parseBool("return_prompt", val)
		if err != nil {
			return Params{}, err
		}
		p.ReturnPrompt = returnPrompt
	}

	if val, exists := d["seed"]; exists {
		seed, err := parseSeed(val)
		if err != nil {
			return Params{}, err
		}
		p.Seed = new(seed)
	}

	if val, exists := d["stream"]; exists {
		stream, err := parseBool("stream", val)
		if err != nil {
			return Params{}, err
		}
		p.Stream = stream
	}

	if streamOpts, exists := d["stream_options"]; exists {
		if optsMap, ok := streamOpts.(map[string]any); ok {
			if val, exists := optsMap["include_usage"]; exists {
				includeUsage, err := parseBool("stream_options.include_usage", val)
				if err != nil {
					return Params{}, err
				}
				p.IncludeUsage = includeUsage
			}
		}
	}

	if val, exists := d["temperature"]; exists {
		temp, err := parseFloat32("temperature", val)
		if err != nil {
			return Params{}, err
		}
		p.Temperature = temp
	}

	if val, exists := d["top_k"]; exists {
		topK, err := parseInt("top_k", val)
		if err != nil {
			return Params{}, err
		}
		p.TopK = int32(topK)
	}

	if val, exists := d["top_logprobs"]; exists {
		topLogprobs, err := parseInt("top_logprobs", val)
		if err != nil {
			return Params{}, err
		}

		// Clamp to valid range (0-20 per OpenAI spec)
		if topLogprobs < 0 {
			topLogprobs = DefTopLogprobs
		}

		if topLogprobs > DefMaxTopLogprobs {
			topLogprobs = DefMaxTopLogprobs
		}

		p.TopLogprobs = topLogprobs

		// If top_logprobs is set, implicitly enable logprobs
		if topLogprobs > 0 {
			p.Logprobs = true
		}
	}

	if val, exists := d["top_p"]; exists {
		topP, err := parseFloat32("top_p", val)
		if err != nil {
			return Params{}, err
		}
		p.TopP = topP
	}

	if val, exists := d["xtc_min_keep"]; exists {
		xtcMinKeep, err := parseInt("xtc_min_keep", val)
		if err != nil {
			return Params{}, err
		}
		p.XtcMinKeep = uint32(xtcMinKeep)
	}

	if val, exists := d["xtc_probability"]; exists {
		xtcProbability, err := parseFloat32("xtc_probability", val)
		if err != nil {
			return Params{}, err
		}
		p.XtcProbability = xtcProbability
	}

	if val, exists := d["xtc_threshold"]; exists {
		xtcThreshold, err := parseFloat32("xtc_threshold", val)
		if err != nil {
			return Params{}, err
		}
		p.XtcThreshold = xtcThreshold
	}

	// Grammar/JSON-schema constrained output is incompatible with a thinking
	// prelude because the first emitted token must satisfy the grammar — a
	// <think> token would violate it. When grammar is set and the user did
	// not explicitly provide enable_thinking, force it off.
	if p.Grammar != "" {
		if _, set := d["enable_thinking"]; !set {
			p.Thinking = ThinkingDisabled
		}
	}

	p = m.adjustParams(p, d)

	// Mirror the resolved enable_thinking into d as a normalized bool. The
	// Jinja chat template reads d["enable_thinking"] directly to decide
	// whether to inject <think> tokens into the prompt, and Jinja's
	// "is true"/"is false" tests require a real bool, not a string.
	d["enable_thinking"] = p.Thinking == ThinkingEnabled

	// Mirror the resolved reasoning_effort back into d so any parser-level
	// coercion (e.g. mistral coercing "medium" → "high" for templates that
	// only accept {none, high}) is visible to the Jinja template, which
	// reads d["reasoning_effort"] directly.
	if p.ReasoningEffort != "" {
		d["reasoning_effort"] = p.ReasoningEffort
	}

	return p, nil
}

func (m *Model) adjustParams(p Params, request D) Params {
	requested := func(key string) bool {
		_, exists := request[key]
		return exists
	}

	if p.DryAllowedLen <= 0 {
		p.DryAllowedLen = DefDryAllowedLen
		if m.paramsResolved {
			p.DryAllowedLen = m.cfg.DefaultParams.DryAllowedLen
		}
	}
	if p.DryBase <= 0 {
		p.DryBase = DefDryBase
		if m.paramsResolved {
			p.DryBase = m.cfg.DefaultParams.DryBase
		}
	}
	if p.DryMultiplier <= 0 {
		p.DryMultiplier = DefDryMultiplier
		if m.paramsResolved {
			p.DryMultiplier = m.cfg.DefaultParams.DryMultiplier
		}
	}
	if !requested("dry_penalty_last_n") && !m.paramsResolved && p.DryPenaltyLast == 0 {
		p.DryPenaltyLast = DefDryPenaltyLast
	}
	if p.MaxTokens <= 0 {
		p.MaxTokens = m.cfg.DefaultParams.MaxTokens
		if !m.paramsResolved {
			p.MaxTokens = m.cfg.ContextWindow()
		}
	}
	if p.MinP < 0 || (p.MinP == 0 && !requested("min_p")) {
		p.MinP = DefMinP
		if m.paramsResolved {
			p.MinP = m.cfg.DefaultParams.MinP
		}
	}
	if p.ReasoningEffort == "" {
		p.ReasoningEffort = DefReasoningEffort
		if m.paramsResolved {
			p.ReasoningEffort = m.cfg.DefaultParams.ReasoningEffort
		}
	}
	if !requested("repeat_last_n") && !m.paramsResolved && p.RepeatLastN == 0 {
		p.RepeatLastN = DefRepeatLastN
	}
	if p.RepeatPenalty <= 0 {
		p.RepeatPenalty = DefRepeatPenalty
	}
	if p.Temperature < 0 || (p.Temperature == 0 && !requested("temperature")) {
		p.Temperature = DefTemp
		if m.paramsResolved {
			p.Temperature = m.cfg.DefaultParams.Temperature
		}
	}
	if p.Thinking == "" {
		p.Thinking = DefEnableThinking
		if m.paramsResolved {
			p.Thinking = m.cfg.DefaultParams.Thinking
		}
	}
	if !requested("top_k") && !m.paramsResolved && p.TopK == 0 {
		p.TopK = DefTopK
	}
	if p.TopP < 0 || (p.TopP == 0 && !requested("top_p")) {
		p.TopP = DefTopP
		if m.paramsResolved {
			p.TopP = m.cfg.DefaultParams.TopP
		}
	}
	if p.XtcMinKeep <= 0 {
		p.XtcMinKeep = DefXtcMinKeep
		if m.paramsResolved {
			p.XtcMinKeep = m.cfg.DefaultParams.XtcMinKeep
		}
	}
	if p.XtcProbability <= 0 {
		p.XtcProbability = DefXtcProbability
		if m.paramsResolved {
			p.XtcProbability = m.cfg.DefaultParams.XtcProbability
		}
	}
	if p.XtcThreshold <= 0 {
		p.XtcThreshold = DefXtcThreshold
		if m.paramsResolved {
			p.XtcThreshold = m.cfg.DefaultParams.XtcThreshold
		}
	}

	// Give the selected parser a chance to coerce params into values its
	// chat template will accept (e.g. Mistral Medium 3.5 only allows
	// reasoning_effort in {none, high}).
	if adj, ok := m.parser.(ParamsAdjuster); ok {
		p = adj.AdjustParams(p)
	}

	return p
}

func resolveSamplingDefaults(p Params, metadata map[string]string, contextWindow int) Params {
	if p.DryAllowedLen <= 0 {
		p.DryAllowedLen = DefDryAllowedLen
	}
	if p.DryBase <= 0 {
		p.DryBase = DefDryBase
	}
	if p.DryPenaltyLast == 0 {
		p.DryPenaltyLast = DefDryPenaltyLast
	}
	if p.MaxTokens <= 0 {
		p.MaxTokens = contextWindow
	}
	if p.MinP == 0 {
		p.MinP = samplingMetadataFloat32(metadata, "general.sampling.min_p", DefMinP)
	}
	if p.ReasoningEffort == "" {
		p.ReasoningEffort = DefReasoningEffort
	}
	if p.RepeatLastN == 0 {
		p.RepeatLastN = samplingMetadataInt32(metadata, "general.sampling.penalty_last_n", DefRepeatLastN)
	}
	if p.RepeatPenalty == 0 {
		p.RepeatPenalty = DefRepeatPenalty
	}
	if p.Temperature == 0 {
		p.Temperature = samplingMetadataFloat32(metadata, "general.sampling.temp", DefTemp)
	}
	if p.Thinking == "" {
		p.Thinking = DefEnableThinking
	}
	if p.TopK == 0 {
		p.TopK = samplingMetadataInt32(metadata, "general.sampling.top_k", DefTopK)
	}
	if p.TopP == 0 {
		p.TopP = samplingMetadataFloat32(metadata, "general.sampling.top_p", DefTopP)
	}
	if p.XtcMinKeep == 0 {
		p.XtcMinKeep = DefXtcMinKeep
	}
	if p.XtcProbability == 0 {
		p.XtcProbability = samplingMetadataFloat32(metadata, "general.sampling.xtc.probability", DefXtcProbability)
	}
	if p.XtcThreshold == 0 {
		p.XtcThreshold = samplingMetadataFloat32(metadata, "general.sampling.xtc.threshold", DefXtcThreshold)
	}

	return p
}

func samplingMetadataFloat32(metadata map[string]string, key string, fallback float32) float32 {
	value, exists := metadata[key]
	if !exists {
		return fallback
	}

	parsed, err := strconv.ParseFloat(value, 32)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return fallback
	}

	return float32(parsed)
}

func samplingMetadataInt32(metadata map[string]string, key string, fallback int32) int32 {
	value, exists := metadata[key]
	if !exists {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return fallback
	}

	return int32(parsed)
}

func (m *Model) toSampler(ctx context.Context, p Params, seeds samplingSeeds) llama.Sampler {
	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())

	var order int
	if len(m.suppressTokens) > 0 {
		order++
		addSuppressTokenSampler(sampler, m.vocab, m.suppressTokens)
		m.log(ctx, "sampler-chain", "order", order, "sampler", "suppress-token-bias", "tokens", len(m.suppressTokens))
	}

	// NOTE: Grammar is NOT added to the sampler chain. The grammar sampler's
	// accept() function crashes in llama.cpp when llama_sampler_chain_accept
	// iterates through all samplers. Grammar is handled separately in the
	// batch engine via GrammarSampler.SampleWithGrammar().

	// The sampler order below matches llama.cpp's common_sampler_init default
	// pipeline: model suppress-token bias → Penalties → DRY → top-k → top-p →
	// min-p → XTC → Temperature → dist. This ordering is critical for tool
	// calling stability.

	if p.RepeatPenalty != 1.0 || p.FrequencyPenalty != 0 || p.PresencePenalty != 0 {
		order++
		llama.SamplerChainAdd(sampler, llama.SamplerInitPenalties(p.RepeatLastN, p.RepeatPenalty, p.FrequencyPenalty, p.PresencePenalty))
		m.log(ctx, "sampler-chain", "order", order, "sampler", "penalties", "repeat_last_n", p.RepeatLastN, "repeat_penalty", fmt.Sprintf("%.2f", p.RepeatPenalty), "frequency_penalty", fmt.Sprintf("%.2f", p.FrequencyPenalty), "presence_penalty", fmt.Sprintf("%.2f", p.PresencePenalty))
	}

	if p.DryMultiplier > 0 {
		order++
		llama.SamplerChainAdd(sampler, llama.SamplerInitDry(m.vocab, int32(m.cfg.ContextWindow()), p.DryMultiplier, p.DryBase, p.DryAllowedLen, p.DryPenaltyLast, []string{"\n", ":", "\"", "*"}))
		m.log(ctx, "sampler-chain", "order", order, "sampler", "dry", "multiplier", fmt.Sprintf("%.2f", p.DryMultiplier), "base", fmt.Sprintf("%.2f", p.DryBase), "allowed_len", p.DryAllowedLen, "penalty_last_n", p.DryPenaltyLast)
	}

	order++
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopK(p.TopK))
	m.log(ctx, "sampler-chain", "order", order, "sampler", "top-k", "k", p.TopK)

	order++
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopP(p.TopP, 0))
	m.log(ctx, "sampler-chain", "order", order, "sampler", "top-p", "p", fmt.Sprintf("%.2f", p.TopP))

	order++
	llama.SamplerChainAdd(sampler, llama.SamplerInitMinP(p.MinP, 0))
	m.log(ctx, "sampler-chain", "order", order, "sampler", "min-p", "p", fmt.Sprintf("%.2f", p.MinP))

	if p.XtcProbability > 0 {
		order++
		llama.SamplerChainAdd(sampler, llama.SamplerInitXTC(p.XtcProbability, p.XtcThreshold, p.XtcMinKeep, seeds.targetXTC))
		m.log(ctx, "sampler-chain", "order", order, "sampler", "xtc", "probability", fmt.Sprintf("%.2f", p.XtcProbability), "threshold", fmt.Sprintf("%.2f", p.XtcThreshold), "min_keep", p.XtcMinKeep)
	}

	if p.AdaptivePTarget > 0 {
		order++
		llama.SamplerChainAdd(sampler, llama.SamplerInitAdaptiveP(p.AdaptivePTarget, p.AdaptivePDecay, seeds.targetAdaptiveP))
		m.log(ctx, "sampler-chain", "order", order, "sampler", "adaptive-p", "target", fmt.Sprintf("%.2f", p.AdaptivePTarget), "decay", fmt.Sprintf("%.2f", p.AdaptivePDecay))
	}

	order++
	llama.SamplerChainAdd(sampler, llama.SamplerInitTempExt(p.Temperature, 0, 1.0))
	m.log(ctx, "sampler-chain", "order", order, "sampler", "temperature", "temp", fmt.Sprintf("%.2f", p.Temperature))

	order++
	llama.SamplerChainAdd(sampler, llama.SamplerInitDist(seeds.targetDist))
	m.log(ctx, "sampler-chain", "order", order, "sampler", "dist")

	return sampler
}

func copySuppressTokens(vocab llama.Vocab) []llama.Token {
	return slices.Clone(llama.VocabGetSuppressTokens(vocab))
}

func suppressTokenLogitBiases(tokens []llama.Token) []llama.LogitBias {
	biases := make([]llama.LogitBias, len(tokens))
	for i, token := range tokens {
		biases[i] = llama.LogitBias{Token: token, Bias: float32(math.Inf(-1))}
	}

	return biases
}

func addSuppressTokenSampler(chain llama.Sampler, vocab llama.Vocab, tokens []llama.Token) {
	if len(tokens) == 0 {
		return
	}

	biases := suppressTokenLogitBiases(tokens)
	biasSampler := llama.SamplerInitLogitBias(llama.VocabNTokens(vocab), int32(len(biases)), &biases[0])
	runtime.KeepAlive(biases)
	llama.SamplerChainAdd(chain, biasSampler)
}

func maskSuppressTokenLogits(logits []float32, tokens []llama.Token) {
	for _, token := range tokens {
		if token >= 0 && int(token) < len(logits) {
			logits[token] = float32(math.Inf(-1))
		}
	}
}

func isSuppressedToken(tokens []llama.Token, token llama.Token) bool {
	return slices.Contains(tokens, token)
}

func parseFloat32(fieldName string, val any) (float32, error) {
	var result float32

	switch v := val.(type) {
	case string:
		temp32, err := strconv.ParseFloat(v, 32)
		if err != nil {
			return 0, fmt.Errorf("parse-float32: field-name[%s] is not valid: %w", fieldName, err)
		}
		result = float32(temp32)

	case float32:
		result = v

	case float64:
		result = float32(v)

	case int:
		result = float32(v)

	case int32:
		result = float32(v)

	case int64:
		result = float32(v)

	default:
		return 0, fmt.Errorf("parse-float32: field-name[%s] is not a valid type", fieldName)
	}

	return result, nil
}

func parseInt(fieldName string, val any) (int, error) {
	var result int

	switch v := val.(type) {
	case string:
		temp32, err := strconv.ParseFloat(v, 32)
		if err != nil {
			return 0, fmt.Errorf("parse-int: field-name[%s] is not valid: %w", fieldName, err)
		}
		result = int(temp32)

	case float32:
		result = int(v)

	case float64:
		result = int(v)

	case int:
		result = v

	case int32:
		result = int(v)

	case int64:
		result = int(v)

	default:
		return 0, fmt.Errorf("parse-int: field-name[%s] is not a valid type", fieldName)
	}

	return result, nil
}

func parseSeed(val any) (uint32, error) {
	invalid := func() (uint32, error) {
		return 0, fmt.Errorf("parse-seed: field-name[seed] must be an integer between 0 and %d", uint64(math.MaxUint32))
	}
	fromUint64 := func(v uint64) (uint32, error) {
		if v > math.MaxUint32 {
			return invalid()
		}
		return uint32(v), nil
	}
	fromInt64 := func(v int64) (uint32, error) {
		if v < 0 {
			return invalid()
		}
		return fromUint64(uint64(v))
	}
	fromFloat64 := func(v float64) (uint32, error) {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > math.MaxUint32 || math.Trunc(v) != v {
			return invalid()
		}
		return uint32(v), nil
	}

	switch v := val.(type) {
	case string:
		parsed, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return invalid()
		}
		return uint32(parsed), nil
	case json.Number:
		parsed, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			return invalid()
		}
		return fromFloat64(parsed)
	case float32:
		return fromFloat64(float64(v))
	case float64:
		return fromFloat64(v)
	case int:
		return fromInt64(int64(v))
	case int8:
		return fromInt64(int64(v))
	case int16:
		return fromInt64(int64(v))
	case int32:
		return fromInt64(int64(v))
	case int64:
		return fromInt64(v)
	case uint:
		return fromUint64(uint64(v))
	case uint8:
		return uint32(v), nil
	case uint16:
		return uint32(v), nil
	case uint32:
		return v, nil
	case uint64:
		return fromUint64(v)
	default:
		return invalid()
	}
}

func parseBool(fieldName string, val any) (bool, error) {
	switch v := val.(type) {
	case bool:
		return v, nil

	case string:
		if v == "" {
			return true, nil
		}

		b, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("parse-bool: field-name[%s] is not valid: %w", fieldName, err)
		}

		return b, nil
	}

	return true, nil
}

func parseReasoningString(fieldName string, val any) (string, error) {
	result := ReasoningEffortMedium

	switch v := val.(type) {
	case string:
		if v != ReasoningEffortNone &&
			v != ReasoningEffortMinimal &&
			v != ReasoningEffortLow &&
			v != ReasoningEffortMedium &&
			v != ReasoningEffortHigh {
			return "", fmt.Errorf("parse-reasoning-string: field-name[%s] is not valid option[%s]", fieldName, v)
		}

		result = v
	}

	return result, nil
}
