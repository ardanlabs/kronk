package model

import (
	"fmt"
	"strconv"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	// DefDryAllowedLen is the minimum n-gram length before DRY applies.
	DefDryAllowedLen = 2

	// DefDryBase is the base for exponential penalty growth in DRY.
	DefDryBase = 1.75

	// DefDryMultiplier controls the DRY (Don't Repeat Yourself) sampler which penalizes
	// n-gram pattern repetition. 0.8 - Light repetition penalty,
	// 1.0–1.5 - Moderate (typical starting point), 2.0–3.0 - Aggressive.
	DefDryMultiplier = 1.05

	// DefDryPenaltyLast limits how many recent tokens DRY considers.
	DefDryPenaltyLast = 0.0

	// DefEnableThinking determines if the model should think or not. It is used for
	// most non-GPT models. It accepts 1, t, T, TRUE, true, True, 0, f, F, FALSE,
	// false, False.
	DefEnableThinking = ThinkingEnabled

	// DefIncludeUsage determines whether to include token usage information in
	// streaming responses.
	DefIncludeUsage = true

	// DefLogprobs determines whether to return log probabilities of output tokens.
	// When enabled, the response includes probability data for each generated token.
	DefLogprobs = false

	// DefTopLogprobs specifies how many of the most likely tokens to return at each
	// position, along with their log probabilities. Must be between 0 and 5.
	// Setting this to a value > 0 implicitly enables logprobs.
	DefTopLogprobs = 0

	// DefMaxTopLogprobs defines the number of maximum logprobs to use.
	DefMaxTopLogprobs = 5

	// DefReasoningEffort is a string that specifies the level of reasoning effort to
	// use for GPT models.
	DefReasoningEffort = ReasoningEffortMedium

	// DefRepeatLastN specifies how many recent tokens to consider when applying the
	// repetition penalty. A larger value considers more context but may be slower.
	DefRepeatLastN = 64

	// DefRepeatPenalty applies a penalty to tokens that have already appeared in the
	// output, reducing repetitive text. A value of 1.0 means no penalty. Values
	// above 1.0 reduce repetition (e.g., 1.1 is a mild penalty, 1.5 is strong).
	DefRepeatPenalty = 1.0

	// DefReturnPrompt determines whether to include the prompt in the final response.
	// When set to true, the prompt will be included.
	DefReturnPrompt = false

	// DefTemp controls the randomness of the output. It rescales the probability
	// distribution of possible next tokens.
	DefTemp = 0.8

	// DefTopK limits the pool of possible next tokens to the K number of most probable
	// tokens. If a model predicts 10,000 possible next tokens, setting top_k to 50
	// means only the 50 tokens with the highest probabilities are considered for
	// selection (after temperature scaling). The rest are ignored.
	DefTopK = 40

	// DefMinP is a dynamic sampling threshold that helps balance the coherence
	// (quality) and diversity (creativity) of the generated text.
	DefMinP = 0.0

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

	// DefMaxTokens is the default maximum tokens for generation when not
	// derived from the model's context window.
	DefMaxTokens = 4096
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
	// Temperature controls the randomness of the output. It rescales the
	// probability distribution of possible next tokens. Default is 0.8.
	Temperature float32 `json:"temperature"`

	// TopK limits the pool of possible next tokens to the K number of most
	// probable tokens. If a model predicts 10,000 possible next tokens, setting
	// top_k to 50 means only the 50 tokens with the highest probabilities are
	// considered for selection (after temperature scaling). Default is 40.
	TopK int32 `json:"top_k"`

	// TopP, also known as nucleus sampling, works differently than top_k by
	// selecting a dynamic pool of tokens whose cumulative probability exceeds a
	// threshold P. Instead of a fixed number of tokens (K), it selects the
	// minimum number of most probable tokens required to reach the cumulative
	// probability P. Default is 0.9.
	TopP float32 `json:"top_p"`

	// MinP is a dynamic sampling threshold that helps balance the coherence
	// (quality) and diversity (creativity) of the generated text. Default is 0.0.
	MinP float32 `json:"min_p"`

	// MaxTokens is the maximum tokens for generation when not derived from the
	// model's context window. Default is 4096.
	MaxTokens int `json:"max_tokens"`

	// RepeatPenalty applies a penalty to tokens that have already appeared in
	// the output, reducing repetitive text. A value of 1.0 means no penalty.
	// Values above 1.0 reduce repetition (e.g., 1.1 is a mild penalty, 1.5 is
	// strong). Default is 1.0 which turns it off.
	RepeatPenalty float32 `json:"repeat_penalty"`

	// RepeatLastN specifies how many recent tokens to consider when applying
	// the repetition penalty. A larger value considers more context but may be
	// slower. Default is 64.
	RepeatLastN int32 `json:"repeat_last_n"`

	// DryMultiplier controls the DRY (Don't Repeat Yourself) sampler which
	// penalizes n-gram pattern repetition. 0.8 - Light repetition penalty,
	// 1.0–1.5 - Moderate (typical starting point), 2.0–3.0 - Aggressive.
	// Default is 1.05.
	DryMultiplier float32 `json:"dry_multiplier"`

	// DryBase is the base for exponential penalty growth in DRY. Default is 1.75.
	DryBase float32 `json:"dry_base"`

	// DryAllowedLen is the minimum n-gram length before DRY applies. Default is 2.
	DryAllowedLen int32 `json:"dry_allowed_length"`

	// DryPenaltyLast limits how many recent tokens DRY considers. Default of 0
	// means full context.
	DryPenaltyLast int32 `json:"dry_penalty_last_n"`

	// XtcProbability controls XTC (eXtreme Token Culling) which randomly removes
	// tokens close to top probability. Must be > 0 to activate. Default is 0.0
	// (disabled).
	XtcProbability float32 `json:"xtc_probability"`

	// XtcThreshold is the probability threshold for XTC culling. Default is 0.1.
	XtcThreshold float32 `json:"xtc_threshold"`

	// XtcMinKeep is the minimum tokens to keep after XTC culling. Default is 1.
	XtcMinKeep uint32 `json:"xtc_min_keep"`

	// Thinking determines if the model should think or not. It is used for most
	// non-GPT models. It accepts 1, t, T, TRUE, true, True, 0, f, F, FALSE,
	// false, False. Default is "true".
	Thinking string `json:"enable_thinking"`

	// ReasoningEffort is a string that specifies the level of reasoning effort
	// to use for GPT models. Default is ReasoningEffortMedium.
	ReasoningEffort string `json:"reasoning_effort"`

	// ReturnPrompt determines whether to include the prompt in the final
	// response. When set to true, the prompt will be included. Default is false.
	ReturnPrompt bool `json:"return_prompt"`

	// IncludeUsage determines whether to include token usage information in
	// streaming responses. Default is true.
	IncludeUsage bool `json:"include_usage"`

	// Logprobs determines whether to return log probabilities of output tokens.
	// When enabled, the response includes probability data for each generated
	// token. Default is false.
	Logprobs bool `json:"logprobs"`

	// TopLogprobs specifies how many of the most likely tokens to return at
	// each position, along with their log probabilities. Must be between 0 and
	// 5. Setting this to a value > 0 implicitly enables logprobs. Default is 0.
	TopLogprobs int `json:"top_logprobs"`

	// Stream determines whether to stream the response.
	Stream bool `json:"stream"`
}

func (m *Model) AddParams(params Params, d D) (Params, error) {
	if val, exists := d["temperature"]; exists {
		v, err := parseFloat32("temperature", val)
		if err != nil {
			return Params{}, err
		}
		params.Temperature = v
	}

	if val, exists := d["top_k"]; exists {
		v, err := parseInt("top_k", val)
		if err != nil {
			return Params{}, err
		}
		params.TopK = int32(v)
	}

	if val, exists := d["top_p"]; exists {
		v, err := parseFloat32("top_p", val)
		if err != nil {
			return Params{}, err
		}
		params.TopP = v
	}

	if val, exists := d["min_p"]; exists {
		v, err := parseFloat32("min_p", val)
		if err != nil {
			return Params{}, err
		}
		params.MinP = v
	}

	if val, exists := d["max_tokens"]; exists {
		v, err := parseInt("max_tokens", val)
		if err != nil {
			return Params{}, err
		}
		params.MaxTokens = v
	}

	if val, exists := d["enable_thinking"]; exists {
		v, err := parseBool("enable_thinking", val)
		if err != nil {
			return Params{}, err
		}
		params.Thinking = strconv.FormatBool(v)
	}

	if val, exists := d["reasoning_effort"]; exists {
		v, err := parseReasoningString("reasoning_effort", val)
		if err != nil {
			return Params{}, err
		}
		params.ReasoningEffort = v
	}

	if val, exists := d["return_prompt"]; exists {
		v, err := parseBool("return_prompt", val)
		if err != nil {
			return Params{}, err
		}
		params.ReturnPrompt = v
	}

	if val, exists := d["repeat_penalty"]; exists {
		v, err := parseFloat32("repeat_penalty", val)
		if err != nil {
			return Params{}, err
		}
		params.RepeatPenalty = v
	}

	if val, exists := d["repeat_last_n"]; exists {
		v, err := parseInt("repeat_last_n", val)
		if err != nil {
			return Params{}, err
		}
		params.RepeatLastN = int32(v)
	}

	if val, exists := d["dry_multiplier"]; exists {
		v, err := parseFloat32("dry_multiplier", val)
		if err != nil {
			return Params{}, err
		}
		params.DryMultiplier = v
	}

	if val, exists := d["dry_base"]; exists {
		v, err := parseFloat32("dry_base", val)
		if err != nil {
			return Params{}, err
		}
		params.DryBase = v
	}

	if val, exists := d["dry_allowed_length"]; exists {
		v, err := parseInt("dry_allowed_length", val)
		if err != nil {
			return Params{}, err
		}
		params.DryAllowedLen = int32(v)
	}

	if val, exists := d["dry_penalty_last_n"]; exists {
		v, err := parseInt("dry_penalty_last_n", val)
		if err != nil {
			return Params{}, err
		}
		params.DryPenaltyLast = int32(v)
	}

	if val, exists := d["xtc_probability"]; exists {
		v, err := parseFloat32("xtc_probability", val)
		if err != nil {
			return Params{}, err
		}
		params.XtcProbability = v
	}

	if val, exists := d["xtc_threshold"]; exists {
		v, err := parseFloat32("xtc_threshold", val)
		if err != nil {
			return Params{}, err
		}
		params.XtcThreshold = v
	}

	if val, exists := d["xtc_min_keep"]; exists {
		v, err := parseInt("xtc_min_keep", val)
		if err != nil {
			return Params{}, err
		}
		params.XtcMinKeep = uint32(v)
	}

	if val, exists := d["include_usage"]; exists {
		v, err := parseBool("include_usage", val)
		if err != nil {
			return Params{}, err
		}
		params.IncludeUsage = v
	}

	if val, exists := d["logprobs"]; exists {
		v, err := parseBool("logprobs", val)
		if err != nil {
			return Params{}, err
		}
		params.Logprobs = v
	}

	if val, exists := d["top_logprobs"]; exists {
		v, err := parseInt("top_logprobs", val)
		if err != nil {
			return Params{}, err
		}
		if v < 0 {
			v = DefTopLogprobs
		}
		if v > DefMaxTopLogprobs {
			v = DefMaxTopLogprobs
		}
		if v > 0 {
			params.Logprobs = true
		}
		params.TopLogprobs = v
	}

	if val, exists := d["stream"]; exists {
		v, err := parseBool("stream", val)
		if err != nil {
			return Params{}, err
		}
		params.Stream = v
	}

	return m.adjustParams(params), nil
}

func (m *Model) parseParams(d D) (Params, error) {
	var temp float32
	if tempVal, exists := d["temperature"]; exists {
		var err error
		temp, err = parseFloat32("temperature", tempVal)
		if err != nil {
			return Params{}, err
		}
	}

	var topK int
	if topKVal, exists := d["top_k"]; exists {
		var err error
		topK, err = parseInt("top_k", topKVal)
		if err != nil {
			return Params{}, err
		}
	}

	var topP float32
	if topPVal, exists := d["top_p"]; exists {
		var err error
		topP, err = parseFloat32("top_p", topPVal)
		if err != nil {
			return Params{}, err
		}
	}

	var minP float32
	if minPVal, exists := d["min_p"]; exists {
		var err error
		minP, err = parseFloat32("min_p", minPVal)
		if err != nil {
			return Params{}, err
		}
	}

	var maxTokens int
	if maxTokensVal, exists := d["max_tokens"]; exists {
		var err error
		maxTokens, err = parseInt("max_tokens", maxTokensVal)
		if err != nil {
			return Params{}, err
		}
	}

	enableThinking := true
	if enableThinkingVal, exists := d["enable_thinking"]; exists {
		var err error
		enableThinking, err = parseBool("enable_thinking", enableThinkingVal)
		if err != nil {
			return Params{}, err
		}
	}

	reasoningEffort := ReasoningEffortMedium
	if reasoningEffortVal, exists := d["reasoning_effort"]; exists {
		var err error
		reasoningEffort, err = parseReasoningString("reasoning_effort", reasoningEffortVal)
		if err != nil {
			return Params{}, err
		}
	}

	returnPrompt := DefReturnPrompt
	if returnPromptVal, exists := d["return_prompt"]; exists {
		var err error
		returnPrompt, err = parseBool("return_prompt", returnPromptVal)
		if err != nil {
			return Params{}, err
		}
	}

	var repeatPenalty float32
	if repeatPenaltyVal, exists := d["repeat_penalty"]; exists {
		var err error
		repeatPenalty, err = parseFloat32("repeat_penalty", repeatPenaltyVal)
		if err != nil {
			return Params{}, err
		}
	}

	var repeatLastN int
	if repeatLastNVal, exists := d["repeat_last_n"]; exists {
		var err error
		repeatLastN, err = parseInt("repeat_last_n", repeatLastNVal)
		if err != nil {
			return Params{}, err
		}
	}

	var dryMultiplier float32
	if val, exists := d["dry_multiplier"]; exists {
		var err error
		dryMultiplier, err = parseFloat32("dry_multiplier", val)
		if err != nil {
			return Params{}, err
		}
	}

	var dryBase float32
	if val, exists := d["dry_base"]; exists {
		var err error
		dryBase, err = parseFloat32("dry_base", val)
		if err != nil {
			return Params{}, err
		}
	}

	var dryAllowedLen int
	if val, exists := d["dry_allowed_length"]; exists {
		var err error
		dryAllowedLen, err = parseInt("dry_allowed_length", val)
		if err != nil {
			return Params{}, err
		}
	}

	var dryPenaltyLast int
	if val, exists := d["dry_penalty_last_n"]; exists {
		var err error
		dryPenaltyLast, err = parseInt("dry_penalty_last_n", val)
		if err != nil {
			return Params{}, err
		}
	}

	var xtcProbability float32
	if val, exists := d["xtc_probability"]; exists {
		var err error
		xtcProbability, err = parseFloat32("xtc_probability", val)
		if err != nil {
			return Params{}, err
		}
	}

	var xtcThreshold float32
	if val, exists := d["xtc_threshold"]; exists {
		var err error
		xtcThreshold, err = parseFloat32("xtc_threshold", val)
		if err != nil {
			return Params{}, err
		}
	}

	var xtcMinKeep int
	if val, exists := d["xtc_min_keep"]; exists {
		var err error
		xtcMinKeep, err = parseInt("xtc_min_keep", val)
		if err != nil {
			return Params{}, err
		}
	}

	includeUsage := DefIncludeUsage
	if streamOpts, exists := d["stream_options"]; exists {
		if optsMap, ok := streamOpts.(map[string]any); ok {
			if val, exists := optsMap["include_usage"]; exists {
				var err error
				includeUsage, err = parseBool("stream_options.include_usage", val)
				if err != nil {
					return Params{}, err
				}
			}
		}
	}

	logprobs := DefLogprobs
	if val, exists := d["logprobs"]; exists {
		var err error
		logprobs, err = parseBool("logprobs", val)
		if err != nil {
			return Params{}, err
		}
	}

	topLogprobs := DefTopLogprobs
	if val, exists := d["top_logprobs"]; exists {
		var err error
		topLogprobs, err = parseInt("top_logprobs", val)
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

		// If top_logprobs is set, implicitly enable logprobs
		if topLogprobs > 0 {
			logprobs = true
		}
	}

	var stream bool
	if val, exists := d["stream"]; exists {
		var err error
		stream, err = parseBool("stream", val)
		if err != nil {
			return Params{}, err
		}
	}

	p := Params{
		Temperature:     temp,
		TopK:            int32(topK),
		TopP:            topP,
		MinP:            minP,
		MaxTokens:       maxTokens,
		RepeatPenalty:   repeatPenalty,
		RepeatLastN:     int32(repeatLastN),
		DryMultiplier:   dryMultiplier,
		DryBase:         dryBase,
		DryAllowedLen:   int32(dryAllowedLen),
		DryPenaltyLast:  int32(dryPenaltyLast),
		XtcProbability:  xtcProbability,
		XtcThreshold:    xtcThreshold,
		XtcMinKeep:      uint32(xtcMinKeep),
		Thinking:        strconv.FormatBool(enableThinking),
		ReasoningEffort: reasoningEffort,
		ReturnPrompt:    returnPrompt,
		IncludeUsage:    includeUsage,
		Logprobs:        logprobs,
		TopLogprobs:     topLogprobs,
		Stream:          stream,
	}

	return m.adjustParams(p), nil
}

func (m *Model) adjustParams(p Params) Params {
	if p.Temperature <= 0 {
		p.Temperature = DefTemp
	}

	if p.TopK <= 0 {
		p.TopK = DefTopK
	}

	if p.TopP <= 0 {
		p.TopP = DefTopP
	}

	if p.MinP <= 0 {
		p.MinP = DefMinP
	}

	if p.MaxTokens <= 0 {
		p.MaxTokens = m.cfg.ContextWindow
	}

	if p.RepeatPenalty <= 0 {
		p.RepeatPenalty = DefRepeatPenalty
	}

	if p.RepeatLastN <= 0 {
		p.RepeatLastN = DefRepeatLastN
	}

	if p.DryMultiplier <= 0 {
		p.DryMultiplier = DefDryMultiplier
	}

	if p.DryBase <= 0 {
		p.DryBase = DefDryBase
	}

	if p.DryAllowedLen <= 0 {
		p.DryAllowedLen = DefDryAllowedLen
	}

	if p.DryPenaltyLast < 0 {
		p.DryPenaltyLast = DefDryPenaltyLast
	}

	if p.XtcProbability <= 0 {
		p.XtcProbability = DefXtcProbability
	}

	if p.XtcThreshold <= 0 {
		p.XtcThreshold = DefXtcThreshold
	}

	if p.XtcMinKeep <= 0 {
		p.XtcMinKeep = DefXtcMinKeep
	}

	if p.Thinking == "" {
		p.Thinking = DefEnableThinking
	}

	if p.ReasoningEffort == "" {
		p.ReasoningEffort = DefReasoningEffort
	}

	return p
}

func (m *Model) toSampler(p Params) llama.Sampler {
	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())

	if p.DryMultiplier > 0 {
		llama.SamplerChainAdd(sampler, llama.SamplerInitDry(m.vocab, int32(m.cfg.ContextWindow), p.DryMultiplier, p.DryBase, p.DryAllowedLen, p.DryPenaltyLast, nil))
	}
	if p.RepeatPenalty != DefRepeatPenalty {
		llama.SamplerChainAdd(sampler, llama.SamplerInitPenalties(p.RepeatLastN, p.RepeatPenalty, 0, 0))
	}
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopK(p.TopK))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopP(p.TopP, 0))
	llama.SamplerChainAdd(sampler, llama.SamplerInitMinP(p.MinP, 0))
	if p.XtcProbability > 0 {
		llama.SamplerChainAdd(sampler, llama.SamplerInitXTC(p.XtcProbability, p.XtcThreshold, p.XtcMinKeep, llama.DefaultSeed))
	}
	llama.SamplerChainAdd(sampler, llama.SamplerInitTempExt(p.Temperature, 0, 1.0))
	llama.SamplerChainAdd(sampler, llama.SamplerInitDist(llama.DefaultSeed))

	return sampler
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

func parseBool(fieldName string, val any) (bool, error) {
	result := true

	switch v := val.(type) {
	case string:
		if v == "" {
			break
		}

		b, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("parse-bool: field-name[%s] is not valid: %w", fieldName, err)
		}

		result = b
	}

	return result, nil
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
