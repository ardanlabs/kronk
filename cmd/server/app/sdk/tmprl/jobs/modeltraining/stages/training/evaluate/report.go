package evaluate

// EvaluationReport records normalized exact-answer scores and acceptance.
type EvaluationReport struct {
	BaseCorrect    int                `json:"base_correct"`
	AdapterCorrect int                `json:"adapter_correct"`
	Total          int                `json:"total"`
	Accepted       bool               `json:"accepted"`
	Answers        []EvaluationAnswer `json:"answers,omitzero"`
}

// EvaluationAnswer preserves actual outputs, not just normalized scores.
type EvaluationAnswer struct {
	Prompt        string `json:"prompt"`
	Expected      string `json:"expected"`
	BaseAnswer    string `json:"base_answer"`
	AdapterAnswer string `json:"adapter_answer"`
}
