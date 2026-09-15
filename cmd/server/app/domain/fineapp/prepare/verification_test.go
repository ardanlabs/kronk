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
	system := verificationSystemPrompt(GoalQuestionAnswer)
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
	}})
	for _, value := range []string{"PROMPT: How many pages", "EVIDENCE: 424 pages", "[TARGET 1] 424 pages"} {
		if !strings.Contains(user, value) {
			t.Fatalf("verification user prompt is missing %q: %s", value, user)
		}
	}
}

func TestVerificationOnlyOffersTheSelectedTarget(t *testing.T) {
	model := &targetChoiceCompleter{content: `{"results":[{"record_number":1,"expected_target_id":1}]}`}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	record := Record{Prompt: "What is Go's error interface named?", Target: "error", Evidence: "type error interface { Error() string }"}
	if _, _, err := previewer.verifyRecords(t.Context(), "model", GoalQuestionAnswer, []Record{record}); err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(model.request.Messages[1].Content, "[TARGET "); count != 1 {
		t.Fatalf("normal verification received %d targets, want only the selected target: %s", count, model.request.Messages[1].Content)
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
