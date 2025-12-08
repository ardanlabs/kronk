package model

import (
	"fmt"
	"strconv"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	defTopK            = 40
	defTopP            = 0.9
	defMinP            = 0.0
	defTemp            = 0.8
	defMaxTokens       = 128
	defEnableThinking  = ThinkingEnabled
	defReasoningEffort = ReasoningEffortMedium
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

// Params represents the different options when using a model. The defaults are
// used when these values are set to 0.
//
// Temperature controls the randomness of the output. It rescales the probability
// distribution of possible next tokens.
// When set to 0, the default value is 0.7.
//
// TopK limits the pool of possible next tokens to the K number of most probable
// tokens. If a model predicts 10,000 possible next tokens, setting Top-K to 50
// means only the 50 tokens with the highest probabilities are considered for
// selection (after temperature scaling). The rest are ignored.
// When set to 0, the default value is 40.
//
// TopP, also known as nucleus sampling, works differently than Top-K by
// selecting a dynamic pool of tokens whose cumulative probability exceeds a
// threshold P. Instead of a fixed number of tokens (K), it selects the minimum
// number of most probable tokens required to reach the cumulative probability P.
// When set to 0, the default value is 0.9.
//
// MinP, is a dynamic sampling threshold that helps balance the coherence
// (quality) and diversity (creativity) of the generated text.
// When set to 0, the default value is 0.0.
//
// These parameters (TopK, TopP, Temperature) are typically used together. The
// sampling process usually applies temperature first, then filters the token
// list using Top-K, and finally filters it again using Top-P before selecting
// the next token randomly from the remaining pool based on their (now adjusted)
// probabilities.
//
// MaxTokens defines the maximum number of output tokens to generate for a
// single response.
// When set to 0, the default value is 512.
//
// EnableThinking determines if the model should think or not. It is used for
// most non-GPT models. It accepts 1, t, T, TRUE, true, True, 0, f, F, FALSE,
// false, False.
// When set to an empty string, the default value is "true".
//
// ReasoningEffort is a string that specifies the level of reasoning effort to
// use for GPT models.
type Params struct {
	Temperature     float32 `json:"temperature"`
	TopK            int32   `json:"top_k"`
	TopP            float32 `json:"top_p"`
	MinP            float32 `json:"min_p"`
	MaxTokens       int     `json:"max_tokens"`
	Thinking        string  `json:"enable_thinking"`
	ReasoningEffort string  `json:"reasoning_effort"`
}

// AddParams can be used to add the configured parameters to the
// specified document.
func AddParams(p Params, d D) {
	d["temperature"] = p.Temperature
	d["top_k"] = p.TopK
	d["top_p"] = p.TopP
	d["min_p"] = p.MinP
	d["max_tokens"] = p.MaxTokens

	if p.Thinking != "" {
		d["enable_thinking"] = (p.Thinking != "false")
	}

	if p.ReasoningEffort != "" {
		d["reasoning_effort"] = p.ReasoningEffort
	}
}

func parseParams(d D) (Params, error) {
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

	params := Params{
		Temperature:     temp,
		TopK:            int32(topK),
		TopP:            topP,
		MinP:            minP,
		MaxTokens:       maxTokens,
		Thinking:        strconv.FormatBool(enableThinking),
		ReasoningEffort: reasoningEffort,
	}

	return adjustParams(params), nil
}

func adjustParams(p Params) Params {
	if p.Temperature <= 0 {
		p.Temperature = defTemp
	}

	if p.TopK <= 0 {
		p.TopK = defTopK
	}

	if p.TopP <= 0 {
		p.TopP = defTopP
	}

	if p.MinP <= 0 {
		p.TopP = defMinP
	}

	if p.MaxTokens <= 0 {
		p.MaxTokens = defMaxTokens
	}

	if p.Thinking == "" {
		p.Thinking = defEnableThinking
	}

	if p.ReasoningEffort == "" {
		p.ReasoningEffort = defReasoningEffort
	}

	return p
}

func toSampler(p Params) llama.Sampler {
	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())

	llama.SamplerChainAdd(sampler, llama.SamplerInitTempExt(p.Temperature, 0, 1.0))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopK(p.TopK))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopP(p.TopP, 0))
	llama.SamplerChainAdd(sampler, llama.SamplerInitMinP(p.MinP, 0))
	llama.SamplerChainAdd(sampler, llama.SamplerInitDist(llama.DefaultSeed))

	return sampler
}

func parseFloat32(fieldName string, val any) (float32, error) {
	var result float32

	switch v := val.(type) {
	case string:
		temp32, err := strconv.ParseFloat(v, 32)
		if err != nil {
			return 0, fmt.Errorf("%s is not valid: %w", fieldName, err)
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
		return 0, fmt.Errorf("%s is not a valid type", fieldName)
	}

	return result, nil
}

func parseInt(fieldName string, val any) (int, error) {
	var result int

	switch v := val.(type) {
	case string:
		temp32, err := strconv.ParseFloat(v, 32)
		if err != nil {
			return 0, fmt.Errorf("%s is not valid: %w", fieldName, err)
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
		return 0, fmt.Errorf("%s is not a valid type", fieldName)
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
			return false, fmt.Errorf("%s is not valid: %w", fieldName, err)
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
			return "", fmt.Errorf("%s is not valid option: %s", fieldName, v)
		}

		result = v
	}

	return result, nil
}
