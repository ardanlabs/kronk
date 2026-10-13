package model

import "fmt"

// DecisionQuestionType identifies the answer shape. Its zero value is unset.
type DecisionQuestionType struct{ value string }

var decisionQuestionTypes = make(map[string]DecisionQuestionType)
var (
	// DecisionQuestionTypeChoice selects one named option.
	DecisionQuestionTypeChoice = newDecisionQuestionType("choice")
	// DecisionQuestionTypeScore rates state against ordered levels.
	DecisionQuestionTypeScore = newDecisionQuestionType("score")
	// DecisionQuestionTypeNoul returns a calibrated yes probability.
	DecisionQuestionTypeNoul = newDecisionQuestionType("noul")
)

func newDecisionQuestionType(value string) DecisionQuestionType {
	t := DecisionQuestionType{value: value}
	decisionQuestionTypes[value] = t
	return t
}

// String returns the answer shape name.
func (t DecisionQuestionType) String() string { return t.value }

// Equal reports whether the values match.
func (t DecisionQuestionType) Equal(other DecisionQuestionType) bool { return t == other }

// IsZero reports whether the answer shape is unset.
func (t DecisionQuestionType) IsZero() bool { return t.value == "" }

// ParseDecisionQuestionType parses a known answer shape or an unset value.
func ParseDecisionQuestionType(value string) (DecisionQuestionType, error) {
	if value == "" {
		return DecisionQuestionType{}, nil
	}
	t, ok := decisionQuestionTypes[value]
	if !ok {
		return DecisionQuestionType{}, fmt.Errorf("invalid decision question type %q", value)
	}
	return t, nil
}

// MustParseDecisionQuestionType parses a name and panics if it is invalid.
func MustParseDecisionQuestionType(value string) DecisionQuestionType {
	t, err := ParseDecisionQuestionType(value)
	if err != nil {
		panic(err)
	}
	return t
}

// MarshalText serializes the answer shape name.
func (t DecisionQuestionType) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText parses a known answer shape or an unset value.
func (t *DecisionQuestionType) UnmarshalText(data []byte) error {
	v, err := ParseDecisionQuestionType(string(data))
	if err != nil {
		return err
	}
	*t = v
	return nil
}
