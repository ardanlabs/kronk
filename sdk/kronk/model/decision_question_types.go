package model

import "github.com/ardanlabs/kronk/sdk/kronk/model/internal/decision"

// DecisionQuestionType identifies the answer shape. Its zero value is unset.
type DecisionQuestionType = decision.DecisionQuestionType

var (
	// DecisionQuestionTypeChoice selects one named option.
	DecisionQuestionTypeChoice = decision.DecisionQuestionTypeChoice
	// DecisionQuestionTypeScore rates state against ordered levels.
	DecisionQuestionTypeScore = decision.DecisionQuestionTypeScore
	// DecisionQuestionTypeNoul returns a calibrated yes probability.
	DecisionQuestionTypeNoul = decision.DecisionQuestionTypeNoul
)

// ParseDecisionQuestionType parses a known answer shape or an unset value.
func ParseDecisionQuestionType(value string) (DecisionQuestionType, error) {
	return decision.ParseDecisionQuestionType(value)
}

// MustParseDecisionQuestionType parses a name and panics if it is invalid.
func MustParseDecisionQuestionType(value string) DecisionQuestionType {
	return decision.MustParseDecisionQuestionType(value)
}
