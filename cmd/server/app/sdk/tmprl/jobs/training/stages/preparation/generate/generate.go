// Package generate creates source-grounded training records and held-out
// evaluation questions using structured model completions.
package generate

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/modeljson"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
)

// Match whole tokens first, so unsupported numbers and identifiers cannot
// contribute fragments. Commas are supported only as thousands separators.
var numberToken = regexp.MustCompile(`[\pL\pN_.,+-]+`)
var numberCandidate = regexp.MustCompile(`^[+-]?(?:(?:\d+|\d{1,3}(?:,\d{3})+)(?:\.\d+)?|\.\d+)(?:[eE][+-]?\d+)?$`)

// Targets selects source-supported targets for the requested goal.
func Targets(passage string, goal preparation.Goal) []string {
	switch goal {
	case preparation.GoalNumericPrediction:
		var numbers []string
		for _, token := range numberToken.FindAllString(passage, -1) {
			token = strings.TrimRight(token, ",.")
			if numberCandidate.MatchString(token) {
				numbers = append(numbers, strings.ReplaceAll(token, ",", ""))
			}
		}
		return slices.Compact(numbers)
	case preparation.GoalClassification:
		// Categories must be source words, not labels invented by the model.
		var words []string
		for word := range strings.FieldsSeq(passage) {
			words = append(words, strings.Trim(word, "\"'.,:;!?()[]"))
		}
		return words
	default:
		return []string{passage}
	}
}

// ValidateRecord checks record integrity and goal-specific prompt and target rules.
func ValidateRecord(goal preparation.Goal, r training.Record) error {
	if err := training.ValidateRecords([]training.Record{r}); err != nil {
		return err
	}
	if utf8.RuneCountInString(r.Prompt) > 2000 {
		return fmt.Errorf("generated prompts must not exceed 2000 characters")
	}
	switch goal {
	case preparation.GoalQuestionAnswer:
		if !strings.HasSuffix(strings.TrimSpace(r.Prompt), "?") {
			return fmt.Errorf("question-answer prompts must end with a question mark")
		}
	case preparation.GoalNumericPrediction:
		if _, err := strconv.ParseFloat(r.Target, 64); err != nil {
			return fmt.Errorf("numeric targets must be numbers")
		}
	case preparation.GoalClassification:
		if len(strings.Fields(r.Target)) > 8 {
			return fmt.Errorf("classification targets must be short source-supported categories")
		}
	}
	return nil
}

// rejectedCompletion is a repairable response, not an infrastructure failure.
type rejectedCompletion struct {
	reason string
}

func (e *rejectedCompletion) Error() string { return e.reason }

// Outcome preserves one provisional list of candidates per passage, in source order.
type Outcome struct {
	Decisions      []checkpoint.GenerationDecision
	RejectedPrompt string
}

// Run dispatches one completion. Its owning Step reserves and bounds attempts.
func Run(ctx context.Context, completer preparation.ModelCompleter, request preparation.PreparationRequest, units []preparation.SourceUnit, previous []training.Record, attempt uint32, feedback string) (Outcome, providers.Completion, error) {
	var source strings.Builder
	maximumTargetID := 0
	evidenceIDs := make([]any, len(units))
	for i, unit := range units {
		evidenceIDs[i] = i + 1
		fmt.Fprintf(&source, "[EVIDENCE %d]\n%s\n", i+1, unit.Evidence)
		for _, r := range previous {
			if r.Evidence == unit.Evidence {
				fmt.Fprintf(&source, "[SAVED EXAMPLE] prompt=%q answer=%q\n", r.Prompt, r.Target)
			}
		}
		source.WriteString("ANSWER SOURCES:\n")
		maximumTargetID = max(maximumTargetID, len(unit.Targets))
		for j, target := range unit.Targets {
			fmt.Fprintf(&source, "TARGET %d: %s\n", j+1, target)
		}
	}
	spanAnswers := request.Goal == preparation.GoalQuestionAnswer || request.Goal == preparation.GoalTextGeneration
	targetIDs := []any{0}
	if !spanAnswers {
		targetIDs = make([]any, maximumTargetID)
		for i := range targetIDs {
			targetIDs[i] = i + 1
		}
	}
	properties := map[string]any{
		"evidence_id": map[string]any{"type": "integer", "enum": evidenceIDs},
		"examples": map[string]any{"type": "array", "items": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"prompt", "answer_quote", "target_id"}, "properties": map[string]any{
				"prompt": map[string]any{"type": "string"}, "answer_quote": map[string]any{"type": "string"}, "target_id": map[string]any{"type": "integer", "enum": targetIDs},
			},
		}},
		"reason": map[string]any{"type": "string", "enum": []any{"none", "no_supported_example"}},
	}
	// KMS constrains the response shape. Array bounds and source coverage are
	// checked below because its grammar does not support minItems/maxItems.
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"decisions"}, "properties": map[string]any{
		"decisions": map[string]any{"type": "array", "items": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"evidence_id", "examples", "reason"}, "properties": properties,
		}},
	}}
	var excluded strings.Builder
	for _, r := range previous[max(0, len(previous)-20):] {
		fmt.Fprintf(&excluded, "\nAvoid repeating: %s", r.Prompt)
	}
	if feedback != "" {
		fmt.Fprintf(&excluded, "\nRun requirements and prior response feedback: %s. Use the evidence and target IDs supplied in this request; do not invent facts to make questions distinct.", feedback)
	}
	answerRule := "Set answer_quote to a nonempty exact contiguous quote from that passage's ANSWER SOURCE and target_id to 0."
	if !spanAnswers {
		answerRule = "Set target_id to a positive TARGET ID from that passage and answer_quote to an empty string."
	}
	selectionRule := "Select zero to three distinct facts per passage, not three paraphrases of one fact. Three is a maximum, never a quota. On repair, keep at most three total including saved examples from this passage. Do not repeat a saved prompt or answer from the same passage."
	if request.MaxQuestions > 0 {
		selectionRule = "Generate up to three NEW examples per passage on this request toward the requested record count. This is a per-response batch size, not a lifetime passage limit. Prefer unused facts. You may also ask a different standalone question about a supported fact with the same answer; preserve its meaning. Do not repeat a saved prompt."
	}
	prompt := fmt.Sprintf("Return exactly %d decisions, one for each evidence_id in %v, using each ID once. %s %s Every answer must answer its entire prompt using only local evidence. Picture OCR preserves words, not visual relationships; do not infer diagram meaning from label order. Use reason=none for examples. If a passage supports no useful example, return examples=[] and reason=no_supported_example; this is a reported model-selected omission, not proof of unusability. On repair, return only NEW candidates. If no new question can be made from a passage with saved examples, return examples=[] and reason=none; do not exclude that passage. Never invent facts or repair incomplete source quotations. Question-answer prompts end in ?. Goal: %s. Guidance: %s%s\nSOURCE:\n%s", len(units), evidenceIDs, selectionRule, answerRule, request.Goal, request.Instructions, excluded.String(), source.String())
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return Outcome{}, providers.Completion{}, err
	}
	completion, err := completer.Complete(ctx, providers.CompletionRequest{Model: request.Model, Messages: []providers.Message{{Role: "system", Content: preparation.DefaultInstructions + " Response JSON Schema: " + string(schemaJSON)}, {Role: "user", Content: prompt}}, ResponseSchema: schema, MaxTokens: max(2048, len(units)*768), Seed: 3407 + attempt})
	if err != nil {
		return Outcome{}, completion, err
	}
	var response struct {
		Decisions []struct {
			EvidenceID int `json:"evidence_id"`
			Examples   *[]struct {
				Prompt      *string `json:"prompt"`
				AnswerQuote *string `json:"answer_quote"`
				TargetID    *int    `json:"target_id"`
			} `json:"examples"`
			Reason string `json:"reason"`
		} `json:"decisions"`
	}
	reject := func(reason string) (Outcome, providers.Completion, error) {
		return Outcome{}, completion, &rejectedCompletion{reason: reason}
	}
	if err := modeljson.Decode(completion.Content, &response); err != nil {
		return reject("return only the required decisions object without unknown fields or trailing data")
	}
	if len(response.Decisions) != len(units) {
		return reject("return one decision for every source unit")
	}
	// Check the entire envelope before retaining any row. Never infer source IDs.
	used := make(map[int]bool, len(units))
	order := make([]int, len(units))
	for index, row := range response.Decisions {
		if row.Examples == nil || len(*row.Examples) > 3 {
			return reject("every decision requires an examples array with at most three items")
		}
		if row.EvidenceID < 1 || row.EvidenceID > len(units) || used[row.EvidenceID] {
			return reject("invalid or repeated evidence ID")
		}
		used[row.EvidenceID] = true
		order[row.EvidenceID-1] = index
		if row.Reason != "none" && row.Reason != "no_supported_example" {
			return reject("every decision requires reason=none or reason=no_supported_example")
		}
		for _, example := range *row.Examples {
			if example.Prompt == nil || example.AnswerQuote == nil || example.TargetID == nil {
				return reject("every example requires prompt, answer_quote and target_id")
			}
		}
	}
	prompts := make(map[string]bool, len(previous)+len(units))
	for _, r := range previous {
		prompts[training.Key(r.Prompt)] = true
	}
	result := Outcome{Decisions: make([]checkpoint.GenerationDecision, len(units))}
	records := slices.Clone(previous)
	for i, index := range order {
		row, unit := response.Decisions[index], units[i]
		decision := &result.Decisions[i]
		decision.Reference = unit.Reference
		decision.Complete = true
		prior := 0
		for _, r := range previous {
			if r.Evidence == unit.Evidence {
				prior++
			}
		}
		switch {
		case row.Reason == "no_supported_example" && prior+len(*row.Examples) == 0:
			decision.Reason = "model abstained: no_supported_example"
			continue
		case row.Reason == "no_supported_example":
			decision.Complete = false
			decision.Reason = "a passage with new or saved examples requires reason=none; "
		case prior+len(*row.Examples) == 0:
			decision.Complete = false
			decision.Reason = "an empty passage result requires reason=no_supported_example; "
		}
		for _, example := range *row.Examples {
			fail := func(reason string) {
				decision.Complete = false
				decision.Reason += reason + "; "
			}
			target := ""
			if spanAnswers {
				quote := strings.TrimSpace(*example.AnswerQuote)
				if *example.TargetID != 0 || quote == "" || utf8.RuneCountInString(quote) > 800 {
					fail("answer_quote must be an exact source span of 1–800 runes, with target_id=0")
					continue
				}
				words := strings.Fields(quote)
				for i, word := range words {
					words[i] = regexp.QuoteMeta(word)
				}
				// Ignore layout whitespace only when locating the quote; store
				// the original source span, never the model's reformatted text.
				pattern := regexp.MustCompile(strings.Join(words, `[\s\v\p{Z}\x{0085}]+`))
				for _, text := range unit.Targets {
					if target = pattern.FindString(text); target != "" {
						break
					}
				}
				if target == "" || utf8.RuneCountInString(target) > 800 {
					fail("answer_quote must match a source span of 1–800 runes; only whitespace may differ")
					continue
				}
			} else {
				if *example.AnswerQuote != "" || *example.TargetID < 1 || *example.TargetID > len(unit.Targets) {
					fail("an example requires a target ID belonging to its evidence block and an empty answer_quote")
					continue
				}
				target = unit.Targets[*example.TargetID-1]
			}
			r := training.Record{Prompt: strings.TrimSpace(*example.Prompt), Target: target, Evidence: unit.Evidence}
			r.FactID = training.FactID(r.Evidence, r.Target)
			if err := ValidateRecord(request.Goal, r); err != nil {
				fail(err.Error())
				continue
			}
			if slices.ContainsFunc(records, func(prior training.Record) bool {
				return prior.Evidence == r.Evidence && prior.Target == r.Target && training.Key(prior.Prompt) == training.Key(r.Prompt)
			}) {
				continue // The exact example is already retained; do not count it twice.
			}
			if prompts[training.Key(r.Prompt)] {
				fail("generated record repeats an existing prompt: " + strconv.Quote(r.Prompt))
				if result.RejectedPrompt == "" {
					result.RejectedPrompt = r.Prompt
				}
				continue
			}
			if request.MaxQuestions == 0 && slices.ContainsFunc(records, func(prior training.Record) bool { return prior.FactID == r.FactID }) {
				fail("generated example repeats a saved or selected answer for the same passage")
				continue
			}
			count := 0
			for _, prior := range records {
				if prior.Evidence == unit.Evidence {
					count++
				}
			}
			if request.MaxQuestions == 0 && count >= 3 {
				fail("a passage may have at most three examples including saved candidates")
				continue
			}
			if err := training.ValidateRecords(append(records, r)); err != nil {
				fail("generated example violates dataset integrity: " + err.Error())
				continue
			}
			prompts[training.Key(r.Prompt)] = true
			records = append(records, r)
			decision.Records = append(decision.Records, r)
		}
	}
	return result, completion, nil
}
