package prepare

import (
	"context"
	"encoding/json"
	"errors"
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

type schemaVerificationCompleter struct {
	requests []CompletionRequest
}

func (c *schemaVerificationCompleter) Complete(_ context.Context, request CompletionRequest) (Completion, error) {
	c.requests = append(c.requests, request)
	required, _ := request.ResponseSchema["required"].([]string)
	verdicts := make(map[string]string, len(required))
	for _, name := range required {
		verdicts[name] = verificationPass
	}
	content, err := json.Marshal(verdicts)
	return Completion{Content: string(content)}, err
}

type boundedVerificationCompleter struct {
	requests          []CompletionRequest
	truncateAbove     int
	malformedAbove    int
	persistentFailure bool
}

func (c *boundedVerificationCompleter) Complete(_ context.Context, request CompletionRequest) (Completion, error) {
	c.requests = append(c.requests, request)
	required, _ := request.ResponseSchema["required"].([]string)
	if c.persistentFailure || len(required) > c.truncateAbove && c.truncateAbove > 0 {
		return Completion{}, ErrCompletionTruncated
	}
	if len(required) > c.malformedAbove && c.malformedAbove > 0 {
		return Completion{Content: `{"record_1":"pass"`}, nil
	}

	verdicts := make(map[string]string, len(required))
	for _, name := range required {
		verdicts[name] = verificationPass
	}
	content, err := json.Marshal(verdicts)
	return Completion{Content: string(content)}, err
}

type cancelingVerificationCompleter struct {
	calls  int
	cancel context.CancelFunc
}

func (c *cancelingVerificationCompleter) Complete(ctx context.Context, _ CompletionRequest) (Completion, error) {
	c.calls++
	if c.calls == 1 {
		return Completion{}, ErrCompletionTruncated
	}
	c.cancel()
	return Completion{}, ctx.Err()
}

func TestVerificationTokenLimitFitsAFullGeneratedBatch(t *testing.T) {
	if got, want := verificationTokenLimit(30), 1264; got != want {
		t.Fatalf("verification token limit = %d, want %d", got, want)
	}
}

func TestVerifyRecordsBoundsEachModelRequest(t *testing.T) {
	model := &schemaVerificationCompleter{}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	records := make([]Record, 80)
	for index := range records {
		records[index] = Record{
			Prompt:   "What programming language is used?",
			Target:   "Go",
			Evidence: "The programming language used is Go.",
		}
	}

	results, metrics, err := previewer.verifyRecords(t.Context(), "model", GoalQuestionAnswer, "", records)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(records) || results[len(results)-1].RecordNumber != len(records) {
		t.Fatalf("verification results = %d with last record %d, want %d ordered results", len(results), results[len(results)-1].RecordNumber, len(records))
	}
	if len(model.requests) != 5 || metrics.verificationAttempts != 5 {
		t.Fatalf("verification requests = %d, attempts = %d; want 5 bounded requests", len(model.requests), metrics.verificationAttempts)
	}
	for index, request := range model.requests {
		required, _ := request.ResponseSchema["required"].([]string)
		if len(required) > maxVerificationRecordsPerRequest {
			t.Fatalf("verification request %d contains %d records; maximum %d", index+1, len(required), maxVerificationRecordsPerRequest)
		}
	}
}

func TestVerifyRecordsSplitsLengthTerminatedBatches(t *testing.T) {
	model := &boundedVerificationCompleter{truncateAbove: 2}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	records := make([]Record, 10)
	for index := range records {
		records[index] = Record{Prompt: "What language is used?", Target: "Go", Evidence: "The example uses Go."}
	}

	results, metrics, err := previewer.verifyRecords(t.Context(), "model", GoalQuestionAnswer, "", records)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(records) || !results[len(results)-1].Passed || results[len(results)-1].RecordNumber != len(records) {
		t.Fatalf("verification results = %+v, want ten ordered passes", results)
	}
	if len(model.requests) != 11 || metrics.verificationAttempts != 11 {
		t.Fatalf("requests = %d, attempts = %d; want 11 bounded subdivision requests", len(model.requests), metrics.verificationAttempts)
	}
}

func TestVerifyRecordsSplitsRepeatedMalformedJSON(t *testing.T) {
	model := &boundedVerificationCompleter{malformedAbove: 1}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	records := make([]Record, 4)
	for index := range records {
		records[index] = Record{Prompt: "What language is used?", Target: "Go", Evidence: "The example uses Go."}
	}

	results, metrics, err := previewer.verifyRecords(t.Context(), "model", GoalQuestionAnswer, "", records)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(records) || !results[len(results)-1].Passed {
		t.Fatalf("verification results = %+v, want four passes after subdivision", results)
	}
	if len(model.requests) != 10 || metrics.verificationAttempts != 10 {
		t.Fatalf("requests = %d, attempts = %d; want two attempts for each malformed parent and one for each singleton", len(model.requests), metrics.verificationAttempts)
	}
}

func TestVerifyRecordsRejectsOnlyPersistentlyTruncatedRecord(t *testing.T) {
	model := &boundedVerificationCompleter{persistentFailure: true}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	record := Record{Prompt: "What language is used?", Target: "Go", Evidence: "The example uses Go."}

	results, metrics, err := previewer.verifyRecords(t.Context(), "model", GoalQuestionAnswer, "", []Record{record})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Passed || results[0].ExpectedTarget != record.Target {
		t.Fatalf("verification results = %+v, want one isolated rejection", results)
	}
	if metrics.verificationAttempts != 1 {
		t.Fatalf("verification attempts = %d, want one bounded attempt", metrics.verificationAttempts)
	}
}

func TestVerifyRecordsStopsSubdivisionWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	model := &cancelingVerificationCompleter{cancel: cancel}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	records := make([]Record, 4)
	for index := range records {
		records[index] = Record{Prompt: "What language is used?", Target: "Go", Evidence: "The example uses Go."}
	}

	_, _, err = previewer.verifyRecords(ctx, "model", GoalQuestionAnswer, "", records)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if model.calls != 2 {
		t.Fatalf("calls = %d, want initial truncation followed by one canceled child request", model.calls)
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
		"function named main does not by itself prove that it is the program entry point",
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
	for _, value := range []string{"PROMPT: How many pages", "[SOURCE EVIDENCE 1]\n424 pages", "EVIDENCE BLOCK: 1", "[TARGET 1] 424 pages"} {
		if !strings.Contains(user, value) {
			t.Fatalf("verification user prompt is missing %q: %s", value, user)
		}
	}
	if !strings.HasPrefix(user, "USER INSTRUCTION — FOLLOW WHEN WRITING EVERY RECORD:\nWrite longer questions for experienced developers.") || strings.Contains(user, `"Write longer questions for experienced developers."`) {
		t.Fatalf("verification user prompt omitted dataset requirements: %s", user)
	}
}

func TestVerificationPromptSharesEvidenceAcrossRecords(t *testing.T) {
	evidence := "The profiler uses SIGPROF to interrupt the program and collect CPU samples."
	records := []Record{
		{Prompt: "Which signal interrupts the program?", Target: "SIGPROF", Evidence: evidence},
		{Prompt: "Why does the profiler interrupt the program?", Target: "to collect CPU samples", Evidence: evidence},
	}
	prompt := verificationUserPrompt(records, "")
	if strings.Count(prompt, evidence) != 1 || strings.Count(prompt, "[SOURCE EVIDENCE ") != 1 {
		t.Fatalf("verification prompt repeated shared evidence: %s", prompt)
	}
	if strings.Count(prompt, "EVIDENCE BLOCK: 1") != 2 {
		t.Fatalf("verification records lost shared evidence references: %s", prompt)
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
	if results[0].ExpectedTarget != records[0].Target {
		t.Fatalf("rejected target = %q, want %q retained for regeneration feedback", results[0].ExpectedTarget, records[0].Target)
	}
}

func TestValidateVerificationRetainsTargetRejectedByModel(t *testing.T) {
	records := []Record{{
		Prompt:   "Which signal interrupts a running program during CPU profiling?",
		Target:   "SIGPROF",
		Evidence: "The profiler uses SIGPROF to interrupt the running program.",
	}}
	results, err := validateVerification(GoalQuestionAnswer, []generatedVerification{{RecordNumber: 1, FailureCategory: verificationTaskQuality}}, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Passed || results[0].ExpectedTarget != "SIGPROF" {
		t.Fatalf("verification results = %+v, want rejected result with its target retained", results)
	}
}
