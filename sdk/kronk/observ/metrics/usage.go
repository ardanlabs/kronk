package metrics

type usageData struct {
	PromptTokens     int
	ReasoningTokens  int
	CompletionTokens int
	OutputTokens     int
	TotalTokens      int
	TokensPerSecond  float64
}

type usage struct {
	promptTokens     *avgMetric
	reasoningTokens  *avgMetric
	completionTokens *avgMetric
	outputTokens     *avgMetric
	totalTokens      *avgMetric
	tokensPerSecond  *avgMetric
}

func newUsage(name string) *usage {
	return &usage{
		promptTokens:     newAvgMetric(name + "_tkns_prompt"),
		reasoningTokens:  newAvgMetric(name + "_tkns_reasoning"),
		completionTokens: newAvgMetric(name + "_tkns_completion"),
		outputTokens:     newAvgMetric(name + "_tkns_output"),
		totalTokens:      newAvgMetric(name + "_tkns_total"),
		tokensPerSecond:  newAvgMetric(name + "_tkns_persecond"),
	}
}

func (u *usage) add(data usageData) {
	u.promptTokens.add(float64(data.PromptTokens))
	u.reasoningTokens.add(float64(data.ReasoningTokens))
	u.completionTokens.add(float64(data.CompletionTokens))
	u.outputTokens.add(float64(data.OutputTokens))
	u.totalTokens.add(float64(data.TotalTokens))
	u.tokensPerSecond.add(float64(data.TokensPerSecond))
}
