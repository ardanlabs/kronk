package model

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/decision"
)

// ErrDecisionRequest indicates that a decision request is invalid.
var ErrDecisionRequest = decision.ErrDecisionRequest

// ErrDecisionBudget indicates that a rendered decision input exceeds the
// protocol's token budget. Inputs are never truncated.
var ErrDecisionBudget = decision.ErrDecisionBudget

// DecisionStateField is one ordered field in a DecisionStateValue.
type DecisionStateField = decision.DecisionStateField

// DecisionStateValue represents a JSON object whose field order must be retained.
// Use this for structured state, instructions, or criteria when prompt byte
// compatibility matters. Plain Go maps are accepted but have sorted keys.
type DecisionStateValue = decision.DecisionStateValue

// DecisionStateData constructs one ordered field for decision state.
func DecisionStateData(name string, value any) DecisionStateField {
	return decision.DecisionStateData(name, value)
}

// DecisionState constructs an ordered decision state value.
func DecisionState(fields ...DecisionStateField) DecisionStateValue {
	return decision.DecisionState(fields...)
}

// DecisionOption is one ordered choice option.
type DecisionOption = decision.DecisionOption

// DecisionQuestionOpt constructs one choice option.
func DecisionQuestionOpt(name string, description any) DecisionOption {
	return decision.DecisionQuestionOpt(name, description)
}

// DecisionNoulCriteria optionally describes the false and true boundaries of
// a Noul question.
type DecisionNoulCriteria = decision.DecisionNoulCriteria

// DecisionQuestion is one named, typed question. Construct values with
// DecisionQuestionChoice, DecisionQuestionScore, and DecisionQuestionNoul so
// criteria remain ordered.
type DecisionQuestion = decision.DecisionQuestion

// DecisionQuestionChoice constructs a question that selects one ordered option.
func DecisionQuestionChoice(id string, instructions any, options ...DecisionOption) DecisionQuestion {
	return decision.DecisionQuestionChoice(id, instructions, options...)
}

// DecisionQuestionScore constructs a question that rates state against ordered levels.
func DecisionQuestionScore(id string, instructions any, levels ...any) DecisionQuestion {
	return decision.DecisionQuestionScore(id, instructions, levels...)
}

// DecisionQuestionNoul constructs a calibrated yes/no question. Criteria may be nil
// when the default true and false descriptions are sufficient.
func DecisionQuestionNoul(id string, instructions any, criteria *DecisionNoulCriteria) DecisionQuestion {
	return decision.DecisionQuestionNoul(id, instructions, criteria)
}

// DecisionRequest evaluates ordered questions independently against one shared state.
type DecisionRequest = decision.DecisionRequest

// DecisionAudioInstructions supplies encoded audio in place of a question's
// text instructions. It requires an audio-capable decision protocol and projector.
type DecisionAudioInstructions = decision.DecisionAudioInstructions

// DecisionAnswer contains the typed answer to one decision question.
// Choice is populated for Choice, Score and Legend for Score, Noul for Noul,
// and Probabilities and Confidence for Choice and Score.
type DecisionAnswer = decision.DecisionAnswer

// DecisionUsage contains model token usage. Decision readouts do not generate
// output tokens, so OutputTokens is zero.
type DecisionUsage = decision.DecisionUsage

// DecisionResponse contains answers keyed by the caller's question IDs.
type DecisionResponse = decision.DecisionResponse

func validateDecisionRequest(req DecisionRequest, protocol DecisionProtocol) error {
	for i, image := range req.Images {
		if mediaTypeFromMagicBytes(image) != MediaTypeVision {
			return fmt.Errorf("%w: images[%d] must be a JPEG, PNG, GIF, or WebP image", ErrDecisionRequest, i)
		}
	}
	for i, file := range req.Files {
		switch mediaTypeFromMagicBytes(file) {
		case MediaTypeVision, MediaTypeAudio:
		default:
			return fmt.Errorf("%w: files[%d] must be an encoded image or audio clip", ErrDecisionRequest, i)
		}
	}
	for _, question := range req.Questions {
		if audio, ok := question.Instructions.(DecisionAudioInstructions); ok && mediaTypeFromMagicBytes(audio.Data) != MediaTypeAudio {
			return fmt.Errorf("%w: question %q instructions must contain an encoded audio clip", ErrDecisionRequest, question.ID)
		}
	}
	return decision.ValidateRequest(req, protocol.String())
}
