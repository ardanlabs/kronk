package evaluate

// EvaluationReport records normalized exact-answer scores and acceptance.
type EvaluationReport struct {
	BaseCorrect    int                `json:"base_correct"`
	AdapterCorrect int                `json:"adapter_correct"`
	Total          int                `json:"total"`
	Accepted       bool               `json:"accepted"`
	Checks         []AcceptanceCheck  `json:"checks,omitempty"`
	Answers        []EvaluationAnswer `json:"answers,omitzero"`
}

// AcceptanceCheck preserves the policy and outcome used for this evaluation.
// Reports written before checks were recorded leave this list absent.
type AcceptanceCheck struct {
	Code     string `json:"code"`
	Actual   int    `json:"actual"`
	Required int    `json:"required"`
	Passed   bool   `json:"passed"`
}

// EvaluationAnswer preserves actual outputs, not just normalized scores.
type EvaluationAnswer struct {
	Prompt        string `json:"prompt"`
	Expected      string `json:"expected"`
	BaseAnswer    string `json:"base_answer"`
	AdapterAnswer string `json:"adapter_answer"`
	AdapterPassed *bool  `json:"adapter_passed,omitempty"`
}
