package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// DecisionQuestionType identifies the answer shape expected for a question.
type DecisionQuestionType string

const (
	// DecisionQuestionTypeChoice selects one named option.
	DecisionQuestionTypeChoice DecisionQuestionType = "choice"
	// DecisionQuestionTypeScore rates state against ordered levels.
	DecisionQuestionTypeScore DecisionQuestionType = "score"
	// DecisionQuestionTypeNoul returns a calibrated yes probability.
	DecisionQuestionTypeNoul DecisionQuestionType = "noul"
)

// ErrDecisionRequest indicates that a decision request is invalid.
var ErrDecisionRequest = errors.New("invalid decision request")

// ErrDecisionBudget indicates that a rendered decision input exceeds the
// protocol's token budget. Inputs are never truncated.
var ErrDecisionBudget = errors.New("decision input exceeds token budget")

// DecisionStateField is one ordered field in a DecisionStateValue.
type DecisionStateField struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// DecisionStateValue represents a JSON object whose field order must be retained.
// Use this for structured state, instructions, or criteria when prompt byte
// compatibility matters. Plain Go maps are accepted but have sorted keys.
type DecisionStateValue []DecisionStateField

// DecisionStateData constructs one ordered field for decision state.
func DecisionStateData(name string, value any) DecisionStateField {
	return DecisionStateField{Name: name, Value: value}
}

// DecisionState constructs an ordered decision state value.
func DecisionState(fields ...DecisionStateField) DecisionStateValue {
	return DecisionStateValue(fields)
}

// DecisionOption is one ordered choice option.
type DecisionOption struct {
	Name        string `json:"name"`
	Description any    `json:"description"`
}

// DecisionQuestionOpt constructs one choice option.
func DecisionQuestionOpt(name string, description any) DecisionOption {
	return DecisionOption{Name: name, Description: description}
}

// DecisionNoulCriteria optionally describes the false and true boundaries of
// a Noul question.
type DecisionNoulCriteria struct {
	False any `json:"false"`
	True  any `json:"true"`
}

// DecisionQuestion is one named, typed question. Construct values with
// DecisionQuestionChoice, DecisionQuestionScore, and DecisionQuestionNoul so
// criteria remain ordered.
type DecisionQuestion struct {
	ID           string                `json:"id"`
	Type         DecisionQuestionType  `json:"type"`
	Instructions any                   `json:"instructions"`
	Options      []DecisionOption      `json:"options,omitempty"`
	Levels       []any                 `json:"levels,omitempty"`
	NoulCriteria *DecisionNoulCriteria `json:"criteria,omitempty"`
}

// DecisionQuestionChoice constructs a question that selects one ordered option.
func DecisionQuestionChoice(id string, instructions any, options ...DecisionOption) DecisionQuestion {
	return DecisionQuestion{
		ID:           id,
		Type:         DecisionQuestionTypeChoice,
		Instructions: instructions,
		Options:      options,
	}
}

// DecisionQuestionScore constructs a question that rates state against ordered levels.
func DecisionQuestionScore(id string, instructions any, levels ...any) DecisionQuestion {
	return DecisionQuestion{
		ID:           id,
		Type:         DecisionQuestionTypeScore,
		Instructions: instructions,
		Levels:       levels,
	}
}

// DecisionQuestionNoul constructs a calibrated yes/no question. Criteria may be nil
// when the default true and false descriptions are sufficient.
func DecisionQuestionNoul(id string, instructions any, criteria *DecisionNoulCriteria) DecisionQuestion {
	return DecisionQuestion{
		ID:           id,
		Type:         DecisionQuestionTypeNoul,
		Instructions: instructions,
		NoulCriteria: criteria,
	}
}

// DecisionRequest evaluates ordered questions independently against one
// shared state.
type DecisionRequest struct {
	State     any                `json:"state"`
	Questions []DecisionQuestion `json:"questions"`
}

// DecisionAnswer contains the typed answer to one decision question.
// Choice is populated for Choice, Score and Legend for Score, Noul for Noul,
// and Probabilities and Confidence for Choice and Score.
type DecisionAnswer struct {
	Type          DecisionQuestionType `json:"type"`
	Choice        string               `json:"choice,omitempty"`
	Score         float64              `json:"score,omitempty"`
	Noul          float64              `json:"noul,omitempty"`
	Probabilities map[string]float64   `json:"probabilities,omitempty"`
	Legend        map[string]any       `json:"legend,omitempty"`
	Confidence    float64              `json:"confidence,omitempty"`
}

// MarshalJSON emits the standard type-specific answer shape, including
// meaningful zero values and excluding fields owned by other question types.
func (a DecisionAnswer) MarshalJSON() ([]byte, error) {
	switch a.Type {
	case DecisionQuestionTypeChoice:
		return json.Marshal(struct {
			Type          DecisionQuestionType `json:"type"`
			Choice        string               `json:"choice"`
			Probabilities map[string]float64   `json:"probabilities"`
			Confidence    float64              `json:"confidence"`
		}{a.Type, a.Choice, a.Probabilities, a.Confidence})

	case DecisionQuestionTypeScore:
		return json.Marshal(struct {
			Type          DecisionQuestionType `json:"type"`
			Score         float64              `json:"score"`
			Legend        map[string]any       `json:"legend"`
			Probabilities map[string]float64   `json:"probabilities"`
			Confidence    float64              `json:"confidence"`
		}{a.Type, a.Score, a.Legend, a.Probabilities, a.Confidence})

	case DecisionQuestionTypeNoul:
		return json.Marshal(struct {
			Type DecisionQuestionType `json:"type"`
			Noul float64              `json:"noul"`
		}{a.Type, a.Noul})

	default:
		return nil, fmt.Errorf("marshal decision answer: unknown type %q", a.Type)
	}
}

// DecisionUsage contains model token usage. Decision readouts do not generate
// output tokens, so OutputTokens is zero.
type DecisionUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// DecisionResponse contains answers keyed by the caller's question IDs.
type DecisionResponse struct {
	Model   string                    `json:"model"`
	Answers map[string]DecisionAnswer `json:"answers"`
	Usage   DecisionUsage             `json:"usage"`
}

func validateDecisionRequest(req DecisionRequest) error {
	if len(req.Questions) == 0 {
		return fmt.Errorf("%w: questions must not be empty", ErrDecisionRequest)
	}

	seen := make(map[string]bool, len(req.Questions))
	for i, question := range req.Questions {
		if strings.TrimSpace(question.ID) == "" {
			return fmt.Errorf("%w: question[%d] has no id", ErrDecisionRequest, i)
		}
		if seen[question.ID] {
			return fmt.Errorf("%w: duplicate question id %q", ErrDecisionRequest, question.ID)
		}
		seen[question.ID] = true

		if question.Instructions == nil {
			return fmt.Errorf("%w: question %q has no instructions", ErrDecisionRequest, question.ID)
		}
		if text, ok := question.Instructions.(string); ok && strings.TrimSpace(text) == "" {
			return fmt.Errorf("%w: question %q has empty instructions", ErrDecisionRequest, question.ID)
		}

		switch question.Type {
		case DecisionQuestionTypeChoice:
			if err := validateDecisionChoice(question); err != nil {
				return err
			}

		case DecisionQuestionTypeScore:
			if len(question.Levels) < 2 || len(question.Levels) > 10 {
				return fmt.Errorf("%w: score question %q needs 2 to 10 levels, got %d", ErrDecisionRequest, question.ID, len(question.Levels))
			}

		case DecisionQuestionTypeNoul:

		default:
			return fmt.Errorf("%w: question %q has unknown type %q", ErrDecisionRequest, question.ID, question.Type)
		}
	}

	return nil
}

func validateDecisionChoice(question DecisionQuestion) error {
	if len(question.Options) == 0 || len(question.Options) > 255 {
		return fmt.Errorf("%w: choice question %q needs 1 to 255 options, got %d", ErrDecisionRequest, question.ID, len(question.Options))
	}

	seen := make(map[string]bool, len(question.Options))
	for i, option := range question.Options {
		if option.Name == "" {
			return fmt.Errorf("%w: choice question %q option[%d] has no name", ErrDecisionRequest, question.ID, i)
		}
		if seen[option.Name] {
			return fmt.Errorf("%w: choice question %q has duplicate option %q", ErrDecisionRequest, question.ID, option.Name)
		}
		seen[option.Name] = true
	}

	return nil
}
