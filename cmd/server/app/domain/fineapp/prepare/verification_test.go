package prepare

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

type targetChoiceCompleter struct {
	content string
	request CompletionRequest
}

func (c *targetChoiceCompleter) Complete(_ context.Context, request CompletionRequest) (Completion, error) {
	c.request = request
	return Completion{Content: c.content}, nil
}

func TestVerificationTokenLimitFitsAFullGeneratedBatch(t *testing.T) {
	if got, want := verificationTokenLimit(30), 1264; got != want {
		t.Fatalf("verification token limit = %d, want %d", got, want)
	}
}

func TestVerificationEvidenceCentersBoundedContextOnTarget(t *testing.T) {
	record := Record{
		Target:   "The supported answer is 42.",
		Evidence: strings.Repeat("before ", 300) + "The supported answer is 42." + strings.Repeat(" after", 300),
	}

	evidence := verificationEvidence(record)
	if !strings.Contains(evidence, record.Target) {
		t.Fatalf("verification evidence omitted target: %q", evidence)
	}
	if length := utf8.RuneCountInString(evidence); length > maxVerificationEvidenceRunes {
		t.Fatalf("verification evidence has %d runes, maximum %d", length, maxVerificationEvidenceRunes)
	}
}

func TestVerificationPromptRequiresEvidenceToConnectPromptAndTarget(t *testing.T) {
	system := verificationSystemPrompt(GoalQuestionAnswer, false)
	for _, requirement := range []string{
		"source evidence",
		"Accept faithful paraphrases",
		"Shared topic words are not enough",
		"reject an isolated value",
		"subject and setup",
		"no additional fact or clause",
		"this manner",
	} {
		if !strings.Contains(system, requirement) {
			t.Fatalf("verification system prompt is missing %q: %s", requirement, system)
		}
	}

	user := verificationUserPrompt([]Record{{
		Prompt:   "How many pages does the handbook contain?",
		Target:   "424 pages",
		Evidence: "424 pages",
	}}, "Write longer questions for experienced developers.")
	for _, value := range []string{"PROMPT: How many pages", "EVIDENCE: 424 pages", "[TARGET 1] 424 pages"} {
		if !strings.Contains(user, value) {
			t.Fatalf("verification user prompt is missing %q: %s", value, user)
		}
	}
	if !strings.HasPrefix(user, "USER INSTRUCTION — FOLLOW WHEN WRITING EVERY RECORD:\nWrite longer questions for experienced developers.") || strings.Contains(user, `"Write longer questions for experienced developers."`) {
		t.Fatalf("verification user prompt omitted dataset requirements: %s", user)
	}
}

func TestVerificationPromptEnforcesCustomStyleRequirements(t *testing.T) {
	custom := verificationSystemPrompt(GoalQuestionAnswer, true)
	for _, requirement := range []string{
		"user's instruction controls how every record is written",
		"is an acceptance requirement",
		"Reject a record that does not follow it",
		"Only source grounding, factual correctness, and the required output structure take priority",
	} {
		if !strings.Contains(custom, requirement) {
			t.Fatalf("custom verification prompt is missing %q: %s", requirement, custom)
		}
	}
	if strings.Contains(custom, "Do not reject merely for stylistic preference") {
		t.Fatalf("custom verification prompt retained the style exemption: %s", custom)
	}

	defaults := verificationSystemPrompt(GoalQuestionAnswer, false)
	if !strings.Contains(defaults, "Do not reject merely for stylistic preference") {
		t.Fatalf("default verification prompt lost its style tolerance: %s", defaults)
	}
	if strings.Contains(defaults, verificationUserRequirements) {
		t.Fatalf("default verification prompt offers a user-requirements outcome without user requirements: %s", defaults)
	}
}

func TestVerificationOnlyOffersTheSelectedTarget(t *testing.T) {
	model := &targetChoiceCompleter{content: `{"record_1":"pass"}`}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	record := Record{Prompt: "What is Go's error interface named?", Target: "error", Evidence: "type error interface { Error() string }"}
	if _, _, err := previewer.verifyRecords(t.Context(), "model", GoalQuestionAnswer, "", []Record{record}); err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(model.request.Messages[1].Content, "[TARGET "); count != 1 {
		t.Fatalf("normal verification received %d targets, want only the selected target: %s", count, model.request.Messages[1].Content)
	}
}

func TestVerificationRequiresOneNamedVerdictPerRecord(t *testing.T) {
	model := &targetChoiceCompleter{content: `{"record_1":"pass","record_2":"grounding"}`}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	records := []Record{
		{Prompt: "What language is used?", Target: "Go", Evidence: "The example uses Go."},
		{Prompt: "What language is used?", Target: "Rust", Evidence: "The example uses Go."},
	}
	results, _, err := previewer.verifyRecords(t.Context(), "model", GoalQuestionAnswer, "", records)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || !results[0].Passed || results[1].Passed {
		t.Fatalf("verification results = %+v, want one pass followed by one rejection", results)
	}
	if results[1].Reason != "The selected target is not grounded in the source evidence." {
		t.Fatalf("rejection reason = %q, want grounding diagnostic", results[1].Reason)
	}

	properties, ok := model.request.ResponseSchema["properties"].(map[string]any)
	if !ok || len(properties) != 2 || properties["record_1"] == nil || properties["record_2"] == nil || properties["results"] != nil {
		t.Fatalf("verification schema properties = %#v, want two named verdicts", properties)
	}
	recordSchema, ok := properties["record_1"].(map[string]any)
	if !ok || recordSchema["type"] != "string" || recordSchema["enum"] != nil {
		t.Fatalf("verification outcome schema = %#v, want a string validated by Go without the broken enum grammar", recordSchema)
	}
}

func TestVerificationRejectsMissingNamedVerdict(t *testing.T) {
	model := &targetChoiceCompleter{content: `{"record_1":"pass"}`}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	records := []Record{
		{Prompt: "What language is used?", Target: "Go", Evidence: "The example uses Go."},
		{Prompt: "What function runs first?", Target: "main", Evidence: "The main function runs first."},
	}
	_, _, err = previewer.verifyRecords(t.Context(), "model", GoalQuestionAnswer, "", records)
	if err == nil || !strings.Contains(err.Error(), "model returned 1 verification results; expected 2") {
		t.Fatalf("verifyRecords error = %v, want missing-verdict validation error", err)
	}
}

func TestVerificationDiagnosesCustomRequirementFailure(t *testing.T) {
	model := &targetChoiceCompleter{content: `{"record_1":"user_requirements"}`}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	record := Record{Prompt: "What language is used?", Target: "Go", Evidence: "The example uses Go."}
	results, _, err := previewer.verifyRecords(t.Context(), "model", GoalQuestionAnswer, "Write detailed questions.", []Record{record})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Passed || results[0].Reason != "The record does not satisfy the user-provided dataset requirements." {
		t.Fatalf("verification results = %+v, want user-requirements diagnostic", results)
	}
}

func TestVerificationRejectsUnknownOutcome(t *testing.T) {
	model := &targetChoiceCompleter{content: `{"record_1":"maybe"}`}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	record := Record{Prompt: "What language is used?", Target: "Go", Evidence: "The example uses Go."}
	_, _, err = previewer.verifyRecords(t.Context(), "model", GoalQuestionAnswer, "", []Record{record})
	if err == nil || !strings.Contains(err.Error(), `invalid verification outcome "maybe"`) {
		t.Fatalf("verifyRecords error = %v, want invalid-outcome validation error", err)
	}
}

func TestUnsupportedIsolatedValueIsNarrow(t *testing.T) {
	tests := []struct {
		name        string
		record      Record
		unsupported bool
	}{
		{name: "isolated number", record: Record{Target: "424 pages", Evidence: "424 pages"}, unsupported: true},
		{name: "isolated name", record: Record{Target: "Mira Chen", Evidence: "Mira Chen"}, unsupported: true},
		{name: "supported number", record: Record{Target: "424 pages", Evidence: "The Northwind Handbook contains 424 pages."}},
		{name: "complete sentence", record: Record{Target: "Channels carry values.", Evidence: "Channels carry values."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUnsupportedIsolatedValue(tt.record); got != tt.unsupported {
				t.Fatalf("isUnsupportedIsolatedValue() = %t, want %t", got, tt.unsupported)
			}
		})
	}
}

func TestValidateVerificationRejectsIsolatedValueDespiteModelApproval(t *testing.T) {
	records := []Record{{
		Prompt:   "How many pages does the Northwind Handbook contain?",
		Target:   "424 pages",
		Evidence: "424 pages",
	}}
	results, err := validateVerification(GoalQuestionAnswer, []generatedVerification{{RecordNumber: 1, ExpectedTargetID: 1}}, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Passed {
		t.Fatalf("verification results = %+v, want one rejection", results)
	}
}
