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
func (dqt DecisionQuestionType) String() string { return dqt.value }

// Equal reports whether the values match.
func (dqt DecisionQuestionType) Equal(other DecisionQuestionType) bool { return dqt == other }

// IsZero reports whether the answer shape is unset.
func (dqt DecisionQuestionType) IsZero() bool { return dqt.value == "" }

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
func (dqt DecisionQuestionType) MarshalText() ([]byte, error) { return []byte(dqt.value), nil }

// UnmarshalText parses a known answer shape or an unset value.
func (dqt *DecisionQuestionType) UnmarshalText(data []byte) error {
	v, err := ParseDecisionQuestionType(string(data))
	if err != nil {
		return err
	}
	*dqt = v
	return nil
}
