package decisionapp

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

type decisionRequest struct {
	Model     string            `json:"model"`
	State     any               `json:"state"`
	Questions decisionQuestions `json:"questions"`
}

type decisionQuestions []decisionQuestion

type decisionQuestion struct {
	ID           string `json:"-"`
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type orderedField struct {
	Name  string
	Value any
}

type orderedObject []orderedField

func decodeDecisionRequest(r io.Reader) (decisionRequest, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()

	value, err := decodeOrderedValue(dec)
	if err != nil {
		return decisionRequest{}, fmt.Errorf("decode decision request: %w", err)
	}

	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return decisionRequest{}, fmt.Errorf("decode decision request: multiple JSON values")
		}
		return decisionRequest{}, fmt.Errorf("decode decision request: %w", err)
	}

	object, ok := value.(orderedObject)
	if !ok {
		return decisionRequest{}, fmt.Errorf("decision request must be a JSON object")
	}

	modelValue, exists := objectField(object, "model")
	if !exists {
		return decisionRequest{}, fmt.Errorf("missing model field")
	}

	modelID, ok := modelValue.(string)
	if !ok || strings.TrimSpace(modelID) == "" {
		return decisionRequest{}, fmt.Errorf("model must be a non-empty string")
	}

	state, exists := objectField(object, "state")
	if !exists {
		return decisionRequest{}, fmt.Errorf("missing state field")
	}

	questionsValue, exists := objectField(object, "questions")
	if !exists {
		return decisionRequest{}, fmt.Errorf("missing questions field")
	}

	questionsObject, ok := questionsValue.(orderedObject)
	if !ok {
		return decisionRequest{}, fmt.Errorf("questions must be a JSON object")
	}

	if len(questionsObject) == 0 {
		return decisionRequest{}, fmt.Errorf("questions must not be empty")
	}

	questions := make(decisionQuestions, len(questionsObject))
	for i, field := range questionsObject {
		question, err := decodeQuestion(field.Name, field.Value)
		if err != nil {
			return decisionRequest{}, fmt.Errorf("question %q: %w", field.Name, err)
		}
		questions[i] = question
	}

	return decisionRequest{
		Model:     modelID,
		State:     state,
		Questions: questions,
	}, nil
}

func decodeQuestion(id string, value any) (decisionQuestion, error) {
	object, ok := value.(orderedObject)
	if !ok {
		return decisionQuestion{}, fmt.Errorf("must be a JSON object")
	}

	typeValue, exists := objectField(object, "type")
	if !exists {
		return decisionQuestion{}, fmt.Errorf("missing type field")
	}

	questionType, ok := typeValue.(string)
	if !ok {
		return decisionQuestion{}, fmt.Errorf("type must be a string")
	}

	instructions, exists := objectField(object, "instructions")
	if !exists {
		return decisionQuestion{}, fmt.Errorf("missing instructions field")
	}

	criteria, hasCriteria := objectField(object, "criteria")

	switch model.DecisionQuestionType(questionType) {
	case model.DecisionQuestionTypeChoice:
		criteriaObject, ok := criteria.(orderedObject)
		if !hasCriteria || !ok {
			return decisionQuestion{}, fmt.Errorf("choice criteria must be a JSON object")
		}

		return decisionQuestion{ID: id, Type: questionType, Instructions: instructions, Criteria: criteriaObject}, nil

	case model.DecisionQuestionTypeScore:
		levels, ok := criteria.([]any)
		if !hasCriteria || !ok {
			return decisionQuestion{}, fmt.Errorf("score criteria must be a JSON array")
		}
		return decisionQuestion{ID: id, Type: questionType, Instructions: instructions, Criteria: levels}, nil

	case model.DecisionQuestionTypeNoul:
		if !hasCriteria || criteria == nil {
			return decisionQuestion{ID: id, Type: questionType, Instructions: instructions}, nil
		}

		criteriaObject, ok := criteria.(orderedObject)
		if !ok {
			return decisionQuestion{}, fmt.Errorf("noul criteria must be a JSON object")
		}

		falseValue, _ := objectField(criteriaObject, "false")
		trueValue, _ := objectField(criteriaObject, "true")

		return decisionQuestion{ID: id, Type: questionType, Instructions: instructions, Criteria: orderedObject{
			{Name: "false", Value: falseValue},
			{Name: "true", Value: trueValue},
		}}, nil

	default:
		return decisionQuestion{}, fmt.Errorf("unknown type %q", questionType)
	}
}

func decodeOrderedValue(dec *json.Decoder) (any, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}

	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}

	switch delim {
	case '{':
		var object orderedObject
		for dec.More() {
			name, err := dec.Token()
			if err != nil {
				return nil, err
			}
			value, err := decodeOrderedValue(dec)
			if err != nil {
				return nil, err
			}
			object = append(object, orderedField{Name: name.(string), Value: value})
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return object, nil

	case '[':
		var array []any
		for dec.More() {
			value, err := decodeOrderedValue(dec)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return array, nil
	}

	return nil, fmt.Errorf("unexpected JSON delimiter %q", delim)
}

func objectField(object orderedObject, name string) (any, bool) {
	for _, field := range object {
		if field.Name == name {
			return field.Value, true
		}
	}

	return nil, false
}

func (r decisionRequest) toSDK() model.DecisionRequest {
	questions := make([]model.DecisionQuestion, len(r.Questions))
	for i, question := range r.Questions {
		instructions := toDecisionValue(question.Instructions)

		switch model.DecisionQuestionType(question.Type) {
		case model.DecisionQuestionTypeChoice:
			criteria := question.Criteria.(orderedObject)
			options := make([]model.DecisionOption, len(criteria))
			for j, option := range criteria {
				options[j] = model.DecisionQuestionOpt(option.Name, toDecisionValue(option.Value))
			}
			questions[i] = model.DecisionQuestionChoice(question.ID, instructions, options...)

		case model.DecisionQuestionTypeScore:
			criteria := question.Criteria.([]any)
			levels := make([]any, len(criteria))
			for j, level := range criteria {
				levels[j] = toDecisionValue(level)
			}
			questions[i] = model.DecisionQuestionScore(question.ID, instructions, levels...)

		case model.DecisionQuestionTypeNoul:
			var criteria *model.DecisionNoulCriteria
			if question.Criteria != nil {
				values := question.Criteria.(orderedObject)
				falseValue, _ := objectField(values, "false")
				trueValue, _ := objectField(values, "true")
				criteria = &model.DecisionNoulCriteria{
					False: toDecisionValue(falseValue),
					True:  toDecisionValue(trueValue),
				}
			}
			questions[i] = model.DecisionQuestionNoul(question.ID, instructions, criteria)
		}
	}

	return model.DecisionRequest{
		State:     toDecisionValue(r.State),
		Questions: questions,
	}
}

func toDecisionValue(value any) any {
	switch value := value.(type) {
	case orderedObject:
		object := make(model.DecisionStateValue, len(value))
		for i, field := range value {
			object[i] = model.DecisionStateData(field.Name, toDecisionValue(field.Value))
		}
		return object

	case []any:
		array := make([]any, len(value))
		for i, item := range value {
			array[i] = toDecisionValue(item)
		}
		return array
	}

	return value
}
