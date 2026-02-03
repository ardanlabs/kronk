package model

import (
	"fmt"
	"strconv"
	"strings"

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

// String returns a string representation of the Params containing only
// non-zero values in the format key[value]\nkey[value]\n ...
func (p Params) String() string {
	var b strings.Builder

	if p.Temperature != 0 {
		fmt.Fprintf(&b, "\ntemperature[%v]\n", p.Temperature)
	}
	if p.TopK != 0 {
		fmt.Fprintf(&b, "top_k[%v]\n", p.TopK)
	}
	if p.TopP != 0 {
		fmt.Fprintf(&b, "top_p[%v]\n", p.TopP)
	}
	if p.MinP != 0 {
		fmt.Fprintf(&b, "min_p[%v]\n", p.MinP)
	}
	if p.MaxTokens != 0 {
		fmt.Fprintf(&b, "max_tokens[%v]\n", p.MaxTokens)
	}
	if p.RepeatPenalty != 0 {
		fmt.Fprintf(&b, "repeat_penalty[%v]\n", p.RepeatPenalty)
	}
	if p.RepeatLastN != 0 {
		fmt.Fprintf(&b, "repeat_last_n[%v]\n", p.RepeatLastN)
	}
	if p.DryMultiplier != 0 {
		fmt.Fprintf(&b, "dry_multiplier[%v]\n", p.DryMultiplier)
	}
	if p.DryBase != 0 {
		fmt.Fprintf(&b, "dry_base[%v]\n", p.DryBase)
	}
	if p.DryAllowedLen != 0 {
		fmt.Fprintf(&b, "dry_allowed_length[%v]\n", p.DryAllowedLen)
	}
	if p.DryPenaltyLast != 0 {
		fmt.Fprintf(&b, "dry_penalty_last_n[%v]\n", p.DryPenaltyLast)
	}
	if p.XtcProbability != 0 {
		fmt.Fprintf(&b, "xtc_probability[%v]\n", p.XtcProbability)
	}
	if p.XtcThreshold != 0 {
		fmt.Fprintf(&b, "xtc_threshold[%v]\n", p.XtcThreshold)
	}
	if p.XtcMinKeep != 0 {
		fmt.Fprintf(&b, "xtc_min_keep[%v]\n", p.XtcMinKeep)
	}
	if p.Thinking != "" {
		fmt.Fprintf(&b, "enable_thinking[%v]\n", p.Thinking)
	}
	if p.ReasoningEffort != "" {
		fmt.Fprintf(&b, "reasoning_effort[%v]\n", p.ReasoningEffort)
	}
	if p.ReturnPrompt {
		fmt.Fprintf(&b, "return_prompt[%v]\n", p.ReturnPrompt)
	}
	if p.IncludeUsage {
		fmt.Fprintf(&b, "include_usage[%v]\n", p.IncludeUsage)
	}
	if p.Logprobs {
		fmt.Fprintf(&b, "logprobs[%v]\n", p.Logprobs)
	}
	if p.TopLogprobs != 0 {
		fmt.Fprintf(&b, "top_logprobs[%v]\n", p.TopLogprobs)
	}
	if p.Stream {
		fmt.Fprintf(&b, "stream[%v]\n", p.Stream)
	}

	return strings.TrimSuffix(b.String(), " ")
}

// AddParams adds the values from the Params struct into the provided D map.
// Only non-zero values are added.
func AddParams(params Params, d D) {
	if params.Temperature != 0 {
		d["temperature"] = params.Temperature
	}
	if params.TopK != 0 {
		d["top_k"] = params.TopK
	}
	if params.TopP != 0 {
		d["top_p"] = params.TopP
	}
	if params.MinP != 0 {
		d["min_p"] = params.MinP
	}
	if params.MaxTokens != 0 {
		d["max_tokens"] = params.MaxTokens
	}
	if params.RepeatPenalty != 0 {
		d["repeat_penalty"] = params.RepeatPenalty
	}
	if params.RepeatLastN != 0 {
		d["repeat_last_n"] = params.RepeatLastN
	}
	if params.DryMultiplier != 0 {
		d["dry_multiplier"] = params.DryMultiplier
	}
	if params.DryBase != 0 {
		d["dry_base"] = params.DryBase
	}
	if params.DryAllowedLen != 0 {
		d["dry_allowed_length"] = params.DryAllowedLen
	}
	if params.DryPenaltyLast != 0 {
		d["dry_penalty_last_n"] = params.DryPenaltyLast
	}
	if params.XtcProbability != 0 {
		d["xtc_probability"] = params.XtcProbability
	}
	if params.XtcThreshold != 0 {
		d["xtc_threshold"] = params.XtcThreshold
	}
	if params.XtcMinKeep != 0 {
		d["xtc_min_keep"] = params.XtcMinKeep
	}
	if params.Thinking != "" {
		d["enable_thinking"] = params.Thinking
	}
	if params.ReasoningEffort != "" {
		d["reasoning_effort"] = params.ReasoningEffort
	}
	if params.ReturnPrompt {
		d["return_prompt"] = params.ReturnPrompt
	}
	if params.IncludeUsage {
		d["include_usage"] = params.IncludeUsage
	}
	if params.Logprobs {
		d["logprobs"] = params.Logprobs
	}
	if params.TopLogprobs != 0 {
		d["top_logprobs"] = params.TopLogprobs
	}
	if params.Stream {
		d["stream"] = params.Stream
	}
}

func (m *Model) parseParams(d D) (Params, error) {
	p := m.cfg.DefaultParams

	if tempVal, exists := d["temperature"]; exists {
		var err error
		temp, err := parseFloat32("temperature", tempVal)
		if err != nil {
			return Params{}, err
		}
		p.Temperature = temp
	}

	if topKVal, exists := d["top_k"]; exists {
		topK, err := parseInt("top_k", topKVal)
		if err != nil {
			return Params{}, err
		}
		p.TopK = int32(topK)
	}

	if topPVal, exists := d["top_p"]; exists {
		topP, err := parseFloat32("top_p", topPVal)
		if err != nil {
			return Params{}, err
		}
		p.TopP = topP
	}

	if minPVal, exists := d["min_p"]; exists {
		minP, err := parseFloat32("min_p", minPVal)
		if err != nil {
			return Params{}, err
		}
		p.MinP = minP
	}

	if maxTokensVal, exists := d["max_tokens"]; exists {
		maxTokens, err := parseInt("max_tokens", maxTokensVal)
		if err != nil {
			return Params{}, err
		}
		p.MaxTokens = maxTokens
	}

	if enableThinkingVal, exists := d["enable_thinking"]; exists {
		enableThinking, err := parseBool("enable_thinking", enableThinkingVal)
		if err != nil {
			return Params{}, err
		}
		p.Thinking = strconv.FormatBool(enableThinking)
	}

	if reasoningEffortVal, exists := d["reasoning_effort"]; exists {
		reasoningEffort, err := parseReasoningString("reasoning_effort", reasoningEffortVal)
		if err != nil {
			return Params{}, err
		}
		p.ReasoningEffort = reasoningEffort
	}

	if returnPromptVal, exists := d["return_prompt"]; exists {
		returnPrompt, err := parseBool("return_prompt", returnPromptVal)
		if err != nil {
			return Params{}, err
		}
		p.ReturnPrompt = returnPrompt
	}

	if repeatPenaltyVal, exists := d["repeat_penalty"]; exists {
		repeatPenalty, err := parseFloat32("repeat_penalty", repeatPenaltyVal)
		if err != nil {
			return Params{}, err
		}
		p.RepeatPenalty = repeatPenalty
	}

	if repeatLastNVal, exists := d["repeat_last_n"]; exists {
		repeatLastN, err := parseInt("repeat_last_n", repeatLastNVal)
		if err != nil {
			return Params{}, err
		}
		p.RepeatLastN = int32(repeatLastN)
	}

	if val, exists := d["dry_multiplier"]; exists {
		dryMultiplier, err := parseFloat32("dry_multiplier", val)
		if err != nil {
			return Params{}, err
		}
		p.DryMultiplier = dryMultiplier
	}

	if val, exists := d["dry_base"]; exists {
		dryBase, err := parseFloat32("dry_base", val)
		if err != nil {
			return Params{}, err
		}
		p.DryBase = dryBase
	}

	if val, exists := d["dry_allowed_length"]; exists {
		dryAllowedLen, err := parseInt("dry_allowed_length", val)
		if err != nil {
			return Params{}, err
		}
		p.DryAllowedLen = int32(dryAllowedLen)
	}

	if val, exists := d["dry_penalty_last_n"]; exists {
		dryPenaltyLast, err := parseInt("dry_penalty_last_n", val)
		if err != nil {
			return Params{}, err
		}
		p.DryPenaltyLast = int32(dryPenaltyLast)
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

	if val, exists := d["xtc_min_keep"]; exists {
		xtcMinKeep, err := parseInt("xtc_min_keep", val)
		if err != nil {
			return Params{}, err
		}
		p.XtcMinKeep = uint32(xtcMinKeep)
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

	if val, exists := d["logprobs"]; exists {
		logprobs, err := parseBool("logprobs", val)
		if err != nil {
			return Params{}, err
		}
		p.Logprobs = logprobs
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

	if val, exists := d["stream"]; exists {
		stream, err := parseBool("stream", val)
		if err != nil {
			return Params{}, err
		}
		p.Stream = stream
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
