package prepare_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
)

type fullBuildCompleter struct {
	requests []prepare.CompletionRequest
}

type qualityRejectingCompleter struct{}

type semanticRejectingCompleter struct {
	requests []prepare.CompletionRequest
}

type incompleteVariantCompleter struct{}

type skippingPassageCompleter struct {
	requests int
}

type partialReviewedVariantCompleter struct {
	allowGenerated bool
}

type spareBatchCompleter struct {
	full       fullBuildCompleter
	selections int
	failSpare  bool
}

func (qualityRejectingCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	content := request.Messages[1].Content
	if strings.Contains(content, "[RECORD 1]") {
		count := strings.Count(content, "[RECORD ")
		results := make([]map[string]any, count)
		for index := range results {
			results[index] = map[string]any{"record_number": index + 1, "expected_target_id": 1, "reason": "The reviewed target is supported."}
		}
		data, _ := json.Marshal(map[string]any{"results": results})
		return prepare.Completion{Content: string(data)}, nil
	}
	return prepare.Completion{Content: `{"records":[]}`}, nil
}

func (c *semanticRejectingCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	c.requests = append(c.requests, request)
	content := request.Messages[1].Content
	switch {
	case strings.Contains(content, "[RECORD 1]"):
		return verificationCompletion(strings.Count(content, "[RECORD "), func(int) bool { return false }), nil
	case strings.Contains(content, "[LOCKED FACT 1]"):
		return variantCompletion(strings.Count(content, "[LOCKED FACT "), "Rejected"), nil
	default:
		return selectedTargetsCompletion(strings.Count(content, "[PASSAGE ")), nil
	}
}

func (c partialReviewedVariantCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	content := request.Messages[1].Content
	switch {
	case strings.Contains(content, "[RECORD 1]"):
		return verificationCompletion(strings.Count(content, "[RECORD "), func(int) bool { return true }), nil
	case strings.Contains(content, "[LOCKED FACT 2]"):
		return variantCompletion(1, "Reviewed"), nil
	case strings.Contains(content, "[LOCKED FACT 1]"):
		if c.allowGenerated && strings.Contains(content, "Beta is the second supported fact.") {
			return variantCompletion(1, "Generated"), nil
		}
		return prepare.Completion{Content: `{"pools":[{"record_number":1,"questions":["not a question"]}]}`}, nil
	default:
		return selectedTargetsCompletion(strings.Count(content, "[PASSAGE ")), nil
	}
}

func (c *spareBatchCompleter) Complete(ctx context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	content := request.Messages[1].Content
	if !strings.Contains(content, "[RECORD 1]") && !strings.Contains(content, "[LOCKED FACT 1]") {
		c.selections++
		if c.failSpare && c.selections == 2 {
			return prepare.Completion{}, fmt.Errorf("optional spare batch failed")
		}
	}
	return c.full.Complete(ctx, request)
}

func selectedTargetsCompletion(count int) prepare.Completion {
	targetIDs := make([]int, count)
	for index := range targetIDs {
		targetIDs[index] = 1
	}
	data, _ := json.Marshal(map[string]any{"target_ids": targetIDs})
	return prepare.Completion{Content: string(data)}
}

func variantCompletion(count int, prefix string) prepare.Completion {
	pools := make([]map[string]any, count)
	for index := range pools {
		pools[index] = map[string]any{
			"record_number": index + 1,
			"questions": []string{
				fmt.Sprintf("%s: Which term names fact %d?", prefix, index+1),
				fmt.Sprintf("%s: How would a developer identify concept %d?", prefix, index+1),
				fmt.Sprintf("%s: During review, what label applies to item %d?", prefix, index+1),
				fmt.Sprintf("%s: Item %d belongs to which technical category?", prefix, index+1),
				fmt.Sprintf("%s: What answer applies to grounded fact %d?", prefix, index+1),
				fmt.Sprintf("%s: In a practical scenario, how is fact %d identified?", prefix, index+1),
			},
		}
	}
	data, _ := json.Marshal(map[string]any{"pools": pools})
	return prepare.Completion{Content: string(data)}
}

func verificationCompletion(count int, passes func(int) bool) prepare.Completion {
	results := make([]map[string]any, count)
	for index := range results {
		expectedTargetID := 0
		if passes(index) {
			expectedTargetID = 1
		}
		results[index] = map[string]any{
			"record_number":      index + 1,
			"expected_target_id": expectedTargetID,
			"reason":             "Deterministic test verification result.",
		}
	}
	data, _ := json.Marshal(map[string]any{"results": results})
	return prepare.Completion{Content: string(data)}
}

func (incompleteVariantCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	content := request.Messages[1].Content
	switch {
	case strings.Contains(content, "[RECORD 1]"):
		return verificationCompletion(strings.Count(content, "[RECORD "), func(int) bool { return true }), nil
	case strings.Contains(content, "[LOCKED FACT 1]"):
		return prepare.Completion{Content: `{"pools":[{"record_number":1,"questions":["not a question","What is the first alternate phrasing?","How is the fact described another way?","Which answer does this grounded question request?"]}]}`}, nil
	default:
		return selectedTargetsCompletion(1), nil
	}
}

func (c *skippingPassageCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	c.requests++
	content := request.Messages[1].Content
	switch {
	case strings.Contains(content, "[RECORD 1]"):
		return verificationCompletion(strings.Count(content, "[RECORD "), func(int) bool { return true }), nil
	case strings.Contains(content, "[LOCKED FACT 1]"):
		return variantCompletion(1, "Selected"), nil
	default:
		return prepare.Completion{Content: `{"target_ids":[0,1]}`}, nil
	}
}

func (c *fullBuildCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	c.requests = append(c.requests, request)
	content := request.Messages[1].Content
	if strings.Contains(content, "[RECORD 1]") {
		count := strings.Count(content, "[RECORD ")
		results := make([]map[string]any, count)
		for index := range results {
			results[index] = map[string]any{"record_number": index + 1, "expected_target_id": 1, "reason": "The target directly answers the prompt."}
		}
		data, _ := json.Marshal(map[string]any{"results": results})
		return prepare.Completion{Content: string(data), PromptTokens: 2, CompletionTokens: 1}, nil
	}
	if strings.Contains(content, "[LOCKED FACT 1]") {
		count := strings.Count(content, "[LOCKED FACT ")
		completion := variantCompletion(count, fmt.Sprintf("Request %d", len(c.requests)))
		completion.PromptTokens = 3
		completion.CompletionTokens = 2
		return completion, nil
	}

	count := requestedRecordCount(content)
	targetIDs := make([]int, count)
	for index := range targetIDs {
		targetIDs[index] = 1
	}
	data, _ := json.Marshal(map[string]any{"target_ids": targetIDs})
	return prepare.Completion{Content: string(data), PromptTokens: 3, CompletionTokens: 2}, nil
}

func requestedRecordCount(prompt string) int {
	var count int
	_, _ = fmt.Sscanf(prompt, "Review %d source passages", &count)
	return count
}

func TestBuildFullProcessesTheCompleteSourceAndPersists(t *testing.T) {
	directory := t.TempDir()
	recordsPath := filepath.Join(directory, "dataset.jsonl")
	reportPath := filepath.Join(directory, "dataset-report.json")
	for _, path := range []string{recordsPath, reportPath} {
		if err := os.WriteFile(path, []byte("previous dataset artifact\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	seedEvidence := "Passage 1 contains unique fact number 1."
	var source strings.Builder
	for index := 1; index <= 10; index++ {
		if index == 10 {
			source.WriteString("# Closing section\n")
		}
		fmt.Fprintf(&source, "Passage %d contains unique fact number %d.\n\n", index, index)
	}
	source.WriteString("Passage 2 contains unique fact number 2.\n\n")
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &fullBuildCompleter{}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	var progress []prepare.FullBuildProgress
	publishedAtCompletion := false
	verifiedSeed := prepare.Record{Prompt: "What is fact one?", Target: seedEvidence, Evidence: seedEvidence}
	artifact, err := previewer.BuildFull(context.Background(), prepare.FullBuildRequest{
		DatasetDirectory:       directory,
		Goal:                   prepare.GoalQuestionAnswer,
		Model:                  "model",
		ReviewedRecords:        []prepare.Record{verifiedSeed},
		VerifiedPreviewRecords: []prepare.Record{verifiedSeed},
		Progress: func(update prepare.FullBuildProgress) {
			progress = append(progress, update)
			if update.Stage == "Dataset preparation complete" {
				records, recordsErr := os.ReadFile(recordsPath)
				report, reportErr := os.ReadFile(reportPath)
				publishedAtCompletion = recordsErr == nil && reportErr == nil &&
					!strings.Contains(string(records), "previous dataset artifact") &&
					!strings.Contains(string(report), "previous dataset artifact")
			}
		},
	})
	if err != nil {
		t.Fatalf("build full: %v", err)
	}
	if len(artifact.Records) != 12 || artifact.ReviewedRecordCount != 1 || artifact.GeneratedRecordCount != 11 {
		t.Fatalf("artifact: %+v", artifact)
	}
	independent := 0
	for _, record := range artifact.Records {
		if record.IndependentPrompt {
			independent++
		}
	}
	if independent != 2 {
		t.Fatalf("independent prompts = %d, want one for each fact", independent)
	}
	if len(progress) != 9 {
		t.Fatalf("progress: %+v", progress)
	}
	assertBuildProgressContract(t, progress)
	lastProgress := progress[len(progress)-1]
	if progress[0].Stage != "Using verified preview" || !slices.ContainsFunc(progress, func(update prepare.FullBuildProgress) bool {
		return update.Stage == "Independently verifying generated records"
	}) || lastProgress.Stage != "Dataset preparation complete" || lastProgress.CompletedBatches != 1 || lastProgress.CompletedUnits != lastProgress.TotalUnits || lastProgress.AcceptedRecords != 12 {
		t.Fatalf("progress: %+v", progress)
	}
	if len(model.requests) == 0 || !strings.Contains(model.requests[0].Messages[1].Content, "[LOCKED FACT 1]") {
		t.Fatal("full build did not proceed directly from the unchanged verified preview to question expansion")
	}
	wantCalls := []string{"create questions", "verify", "select answers", "create questions", "verify"}
	if got := completionPurposes(model.requests); !slices.Equal(got, wantCalls) {
		t.Fatalf("model calls = %v, want %v", got, wantCalls)
	}
	if artifact.GenerationAttempts != 3 || artifact.VerificationAttempts != 2 || artifact.PromptTokens != 13 || artifact.CompletionTokens != 8 {
		t.Fatalf("model cost = generations %d, verifications %d, prompt tokens %d, completion tokens %d",
			artifact.GenerationAttempts, artifact.VerificationAttempts, artifact.PromptTokens, artifact.CompletionTokens)
	}
	for _, request := range model.requests {
		if request.Seed != 3407 {
			t.Fatalf("preparation request seed = %d, want 3407", request.Seed)
		}
	}
	selectionRequestIndex := slices.IndexFunc(model.requests, func(request prepare.CompletionRequest) bool {
		return strings.Contains(request.Messages[1].Content, "SOURCE:\n")
	})
	if selectionRequestIndex < 0 || !strings.Contains(model.requests[selectionRequestIndex].Messages[1].Content, "Passage 10 contains unique fact number 10") {
		t.Fatalf("generation omitted the end of the document")
	}
	generationPrompt := model.requests[selectionRequestIndex].Messages[1].Content
	if !strings.Contains(generationPrompt, "[APPROVED EXAMPLE 1]") || !strings.Contains(generationPrompt, seedEvidence) || strings.Contains(generationPrompt, "What is fact one?") {
		t.Fatalf("answer selection did not use only the reviewed target example")
	}
	selectionSchema, err := json.Marshal(model.requests[selectionRequestIndex].ResponseSchema)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(selectionSchema), `"prompt"`) || !strings.Contains(string(selectionSchema), `"target_ids"`) {
		t.Fatalf("answer selection schema also requested questions: %s", selectionSchema)
	}
	_, sourceText, _ := strings.Cut(generationPrompt, "SOURCE:\n")
	if strings.Contains(sourceText, seedEvidence) {
		t.Fatalf("reviewed evidence was reused for answer selection")
	}
	if !slices.ContainsFunc(model.requests, func(request prepare.CompletionRequest) bool {
		content := request.Messages[1].Content
		return strings.Contains(content, "[LOCKED FACT 1]") && strings.Contains(content, "LOCKED TARGET: "+seedEvidence)
	}) {
		t.Fatalf("reviewed fact was not expanded with its target locked")
	}
	variantRequestIndex := slices.IndexFunc(model.requests, func(request prepare.CompletionRequest) bool {
		return strings.Contains(request.Messages[1].Content, "[LOCKED FACT 1]")
	})
	variantRequest := model.requests[variantRequestIndex]
	variantSchema, err := json.Marshal(variantRequest.ResponseSchema)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(variantRequest.Messages[1].Content, "question_1") || strings.Contains(string(variantSchema), "style") {
		t.Fatalf("question pool still uses forced styles: %s", variantSchema)
	}
	if !strings.Contains(variantRequest.Messages[1].Content, "exactly 10 natural candidate questions") || !strings.Contains(string(variantSchema), `"minItems":10`) {
		t.Fatalf("question pool does not request ten candidates: %s", variantSchema)
	}
	generatedVariant := slices.IndexFunc(model.requests, func(request prepare.CompletionRequest) bool {
		content := request.Messages[1].Content
		return strings.Contains(content, "[LOCKED FACT 1]") && strings.Contains(content, "Passage 10 contains unique fact number 10")
	})
	if generatedVariant < 0 || strings.Contains(model.requests[generatedVariant].Messages[1].Content, "ORIGINAL QUESTION:") {
		t.Fatal("generated questions were not created after locking an answer")
	}
	for _, path := range []string{artifact.RecordsPath, artifact.ReportPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("persisted artifact %s: %v", path, err)
		}
		if strings.Contains(string(data), "previous dataset artifact") {
			t.Fatalf("persisted artifact %s was not replaced", path)
		}
	}
	if !publishedAtCompletion {
		t.Fatal("completion progress was reported before replacing the dataset artifacts")
	}
	var report prepare.FullBuildReport
	reportData, err := os.ReadFile(artifact.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatal(err)
	}
	if report.TotalRecords != len(artifact.Records) || report.GenerationAttempts != artifact.GenerationAttempts ||
		report.VerificationAttempts != artifact.VerificationAttempts || report.PromptTokens != artifact.PromptTokens ||
		report.CompletionTokens != artifact.CompletionTokens {
		t.Fatalf("persisted report disagrees with artifact: report %+v, artifact %+v", report, artifact)
	}
}

func completionPurposes(requests []prepare.CompletionRequest) []string {
	purposes := make([]string, len(requests))
	for index, request := range requests {
		content := request.Messages[1].Content
		switch {
		case strings.Contains(content, "[RECORD 1]"):
			purposes[index] = "verify"
		case strings.Contains(content, "[LOCKED FACT 1]"):
			purposes[index] = "create questions"
		default:
			purposes[index] = "select answers"
		}
	}
	return purposes
}

func assertBuildProgressContract(t *testing.T, progress []prepare.FullBuildProgress) {
	t.Helper()
	for index, update := range progress {
		if update.CompletedUnits < 0 || update.CompletedUnits > update.TotalUnits {
			t.Fatalf("progress update %d has invalid units: %+v", index, update)
		}
		if index > 0 && update.CompletedUnits < progress[index-1].CompletedUnits {
			t.Fatalf("progress moved backward at update %d: %+v", index, progress)
		}
		if index < len(progress)-1 && update.CompletedUnits == update.TotalUnits {
			t.Fatalf("progress reached 100%% before completion at update %d: %+v", index, progress)
		}
	}
}

func TestBuildFullExpandsReviewedQuestionWithVerifiedVariants(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte("First supported fact."), 0o600); err != nil {
		t.Fatal(err)
	}
	reviewedEvidence := "Second reviewed fact."
	previewer, err := prepare.NewPreviewer(&fullBuildCompleter{})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		ReviewedRecords: []prepare.Record{{
			Prompt: "What is the reviewed fact?", Target: reviewedEvidence, Evidence: reviewedEvidence,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 12 || artifact.ReviewedRecordCount != 1 || artifact.GeneratedRecordCount != 11 {
		t.Fatalf("artifact = %+v", artifact)
	}
	if !slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
		return record.Prompt == "What is the reviewed fact?" && record.Target == reviewedEvidence
	}) {
		t.Fatalf("reviewed fact was not retained: %+v", artifact.Records)
	}
}

func TestBuildFullReplacesUnchangedPreviewRowThatFailsFullQualityChecks(t *testing.T) {
	directory := t.TempDir()
	evidence := "A package is one unit of Go code."
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(evidence), 0o600); err != nil {
		t.Fatal(err)
	}
	generatedPreview := prepare.Record{
		Prompt: "According to the text, what is one unit of Go code?", Target: evidence, Evidence: evidence,
	}
	previewer, err := prepare.NewPreviewer(&fullBuildCompleter{})
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory:           directory,
		Goal:                       prepare.GoalQuestionAnswer,
		Model:                      "model",
		ReviewedRecords:            []prepare.Record{generatedPreview},
		VerifiedPreviewRecords:     []prepare.Record{generatedPreview},
		MinimumQuestionAnswerFacts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ReviewedRecordCount != 0 || len(artifact.Records) != 6 {
		t.Fatalf("invalid generated preview was not replaced: %+v", artifact)
	}
	if slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
		return record.Prompt == generatedPreview.Prompt
	}) {
		t.Fatalf("invalid generated preview was retained: %+v", artifact.Records)
	}
	reportData, err := os.ReadFile(artifact.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reportData), `"reviewed_seed_input_count": 1`) || !strings.Contains(string(reportData), `"reviewed_seed_count": 0`) {
		t.Fatalf("report does not distinguish the replaced preview row: %s", reportData)
	}
}

func TestBuildFullContinuesWhenOneReviewedFactCannotProduceCompleteVariants(t *testing.T) {
	directory := t.TempDir()
	firstEvidence := "Alpha is the first supported fact."
	secondEvidence := "Beta is the second supported fact."
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(firstEvidence+"\n\n"+secondEvidence), 0o600); err != nil {
		t.Fatal(err)
	}
	reviewed := []prepare.Record{
		{Prompt: "What is the first supported fact?", Target: firstEvidence, Evidence: firstEvidence},
		{Prompt: "What is the second supported fact?", Target: secondEvidence, Evidence: secondEvidence},
	}
	previewer, err := prepare.NewPreviewer(partialReviewedVariantCompleter{})
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory:           directory,
		Goal:                       prepare.GoalQuestionAnswer,
		Model:                      "model",
		ReviewedRecords:            reviewed,
		VerifiedPreviewRecords:     reviewed,
		MinimumQuestionAnswerFacts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 6 || artifact.ReviewedRecordCount != 1 {
		t.Fatalf("artifact = %+v, want one complete retained fact", artifact)
	}
	if !slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
		return record.Prompt == reviewed[0].Prompt
	}) {
		t.Fatalf("complete reviewed fact was not retained: %+v", artifact.Records)
	}
}

func TestBuildFullLimitsOversizedReviewedBatchToRequestedFacts(t *testing.T) {
	directory := t.TempDir()
	firstEvidence := "Alpha is the first supported fact."
	secondEvidence := "Beta is the second supported fact."
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(firstEvidence+"\n\n"+secondEvidence), 0o600); err != nil {
		t.Fatal(err)
	}
	reviewed := []prepare.Record{
		{Prompt: "What is the first supported fact?", Target: firstEvidence, Evidence: firstEvidence},
		{Prompt: "What is the second supported fact?", Target: secondEvidence, Evidence: secondEvidence},
	}
	previewer, err := prepare.NewPreviewer(&fullBuildCompleter{})
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory:           directory,
		Goal:                       prepare.GoalQuestionAnswer,
		Model:                      "model",
		ReviewedRecords:            reviewed,
		VerifiedPreviewRecords:     reviewed,
		MinimumQuestionAnswerFacts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 6 || artifact.ReviewedRecordCount != 1 {
		t.Fatalf("artifact = %+v, want exactly one complete fact", artifact)
	}
	if slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
		return record.Target == secondEvidence
	}) {
		t.Fatalf("artifact retained the oversized batch's second fact: %+v", artifact.Records)
	}
}

func TestBuildFullPrefersDistinctFactsAndStillFillsRequestedCount(t *testing.T) {
	directory := t.TempDir()
	targets := []string{
		"In Go, identifiers are either exported or unexported from a package.",
		"Interfaces allow you to express the behavior of a type.",
		"Go supports the exporting and unexporting of identifiers from a package.",
		"Interfaces are types that just declare behavior.",
		"Examples are based on existing functions or methods.",
		"Benchmark functions repeat for at least one second.",
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(strings.Join(targets, "\n\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	reviewed := make([]prepare.Record, len(targets))
	for index, target := range targets {
		reviewed[index] = prepare.Record{
			Prompt:   fmt.Sprintf("What is supported fact %d?", index+1),
			Target:   target,
			Evidence: target,
		}
	}
	previewer, err := prepare.NewPreviewer(&fullBuildCompleter{})
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory:           directory,
		Goal:                       prepare.GoalQuestionAnswer,
		Model:                      "model",
		ReviewedRecords:            reviewed,
		VerifiedPreviewRecords:     reviewed,
		MinimumQuestionAnswerFacts: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 24 {
		t.Fatalf("records = %d, want four complete six-record facts", len(artifact.Records))
	}
	for _, index := range []int{2, 3} {
		if slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
			return record.Target == targets[index]
		}) {
			t.Fatalf("retained overlapping fact %q", targets[index])
		}
	}
	for _, index := range []int{4, 5} {
		if !slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
			return record.Target == targets[index]
		}) {
			t.Fatalf("did not backfill distinct fact %q", targets[index])
		}
	}
}

func TestBuildFullPrefersSelfContainedConciseFacts(t *testing.T) {
	directory := t.TempDir()
	weakTargets := []string{
		"These functions assemble the input and output into a single stream for processing.",
		"Everything I know suggests that numCPU should be the fastest since it is efficiently using the cpu capacity of the machine.",
		"the build tools to produce an executable, the function main must be declared, and it becomes the entry point for the program.",
		"The for range loop on line 31 in listing 9.10 allows the test to iterate over the table and run the test code for each different URL.",
	}
	strongTargets := []string{
		"SIGPROF interrupts a running program for CPU profiling while preserving a self-contained explanation of the operating-system sampling mechanism.",
		"A mutex permits only one Goroutine to execute protected code at a time while maintaining exclusive access to shared application state.",
		"Escape analysis chooses whether a value is constructed on the stack or heap based on whether the value outlives its declaring function.",
		"The -benchtime setting controls how long a benchmark loop runs before the testing framework reports its stable measurement results.",
	}
	for index := len(strongTargets); index < 30; index++ {
		strongTargets = append(strongTargets, fmt.Sprintf("Independent concept %d provides a complete and durable explanation whose meaning does not depend on document coordinates or surrounding prose.", index+1))
	}
	targets := append(weakTargets, strongTargets...)
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(strings.Join(targets, "\n\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	reviewed := make([]prepare.Record, len(targets))
	for index, target := range targets {
		reviewed[index] = prepare.Record{
			Prompt:   fmt.Sprintf("What is supported fact %d?", index+1),
			Target:   target,
			Evidence: target,
		}
	}
	previewer, err := prepare.NewPreviewer(&fullBuildCompleter{})
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory:           directory,
		Goal:                       prepare.GoalQuestionAnswer,
		Model:                      "model",
		ReviewedRecords:            reviewed,
		VerifiedPreviewRecords:     reviewed,
		MinimumQuestionAnswerFacts: len(strongTargets),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 180 {
		t.Fatalf("records = %d, want exactly 30 complete facts (180 records)", len(artifact.Records))
	}
	for _, target := range weakTargets {
		if slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
			return record.Target == target
		}) {
			t.Fatalf("retained context-dependent target %q", target)
		}
	}
	for _, target := range strongTargets {
		if !slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
			return record.Target == target
		}) {
			t.Fatalf("did not retain self-contained target %q", target)
		}
	}
}

func TestBuildFullBackfillsWhenEveryAvailableFactOverlaps(t *testing.T) {
	directory := t.TempDir()
	targets := []string{
		"Go identifiers can be exported or unexported from packages.",
		"Go packages support exported and unexported identifiers.",
		"Packages contain exported and unexported Go identifiers.",
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(strings.Join(targets, "\n\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	reviewed := make([]prepare.Record, len(targets))
	for index, target := range targets {
		reviewed[index] = prepare.Record{
			Prompt:   fmt.Sprintf("What is overlapping fact %d?", index+1),
			Target:   target,
			Evidence: target,
		}
	}
	previewer, err := prepare.NewPreviewer(&fullBuildCompleter{})
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory:           directory,
		Goal:                       prepare.GoalQuestionAnswer,
		Model:                      "model",
		ReviewedRecords:            reviewed,
		VerifiedPreviewRecords:     reviewed,
		MinimumQuestionAnswerFacts: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 18 {
		t.Fatalf("records = %d, want all three complete facts backfilled", len(artifact.Records))
	}
}

func TestBuildFullProcessesAtMostOneOptionalSpareBatch(t *testing.T) {
	for _, test := range []struct {
		name      string
		failSpare bool
	}{
		{name: "available"},
		{name: "unavailable", failSpare: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			var source strings.Builder
			for index := 1; index <= 16; index++ {
				fmt.Fprintf(&source, "Passage %d contains a distinct fact for spare selection. %s\n\n", index, strings.Repeat("detail ", 450))
			}
			if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(source.String()), 0o600); err != nil {
				t.Fatal(err)
			}
			model := &spareBatchCompleter{failSpare: test.failSpare}
			previewer, err := prepare.NewPreviewer(model)
			if err != nil {
				t.Fatal(err)
			}

			artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
				DatasetDirectory:           directory,
				Goal:                       prepare.GoalQuestionAnswer,
				Model:                      "model",
				MinimumQuestionAnswerFacts: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if model.selections != 2 {
				t.Fatalf("selection batches = %d, want one required and one optional", model.selections)
			}
			if len(artifact.Records) != 6 {
				t.Fatalf("records = %d, want exactly one complete selected fact", len(artifact.Records))
			}
			spares, err := os.ReadFile(artifact.SpareRecordsPath)
			if err != nil {
				t.Fatal(err)
			}
			if lines := strings.Count(string(spares), "\n"); lines == 0 || lines%6 != 0 {
				t.Fatalf("spare records = %d, want complete six-record facts", lines)
			}
		})
	}
}

func TestBuildFullDoesNotRepeatCompletedSpareBatchAfterResume(t *testing.T) {
	directory := t.TempDir()
	var source strings.Builder
	for index := 1; index <= 16; index++ {
		fmt.Fprintf(&source, "Passage %d contains a distinct resumable spare fact. %s\n\n", index, strings.Repeat("detail ", 450))
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	model := &spareBatchCompleter{}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	_, err = previewer.BuildFull(ctx, prepare.FullBuildRequest{
		DatasetDirectory:           directory,
		Goal:                       prepare.GoalQuestionAnswer,
		Model:                      "model",
		MinimumQuestionAnswerFacts: 1,
		Progress: func(progress prepare.FullBuildProgress) {
			if progress.CompletedBatches == 2 {
				cancel()
			}
		},
	})
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("first build error = %v, want cancellation after spare batch", err)
	}
	if model.selections != 2 {
		t.Fatalf("selection batches before resume = %d, want two", model.selections)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory:           directory,
		Goal:                       prepare.GoalQuestionAnswer,
		Model:                      "model",
		MinimumQuestionAnswerFacts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if model.selections != 2 {
		t.Fatalf("resume repeated completed spare batch; selection batches = %d", model.selections)
	}
	if len(artifact.Records) != 6 {
		t.Fatalf("records = %d, want exactly one complete selected fact", len(artifact.Records))
	}
}

func TestBuildFullContinuesFromUnusedEvidenceWhenNoReviewedFactCanExpand(t *testing.T) {
	directory := t.TempDir()
	firstEvidence := "Alpha is the first supported fact."
	secondEvidence := "Beta is the second supported fact."
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(firstEvidence+"\n\n"+secondEvidence), 0o600); err != nil {
		t.Fatal(err)
	}
	reviewed := prepare.Record{Prompt: "What is the first supported fact?", Target: firstEvidence, Evidence: firstEvidence}
	previewer, err := prepare.NewPreviewer(partialReviewedVariantCompleter{allowGenerated: true})
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory:           directory,
		Goal:                       prepare.GoalQuestionAnswer,
		Model:                      "model",
		ReviewedRecords:            []prepare.Record{reviewed},
		VerifiedPreviewRecords:     []prepare.Record{reviewed},
		MinimumQuestionAnswerFacts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 6 || artifact.ReviewedRecordCount != 0 {
		t.Fatalf("artifact = %+v, want one generated replacement fact", artifact)
	}
}

func TestBuildFullRejectsFactWhenAnyRequiredAlternateQuestionIsUnusable(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte("One grounded fact supports this training record."), 0o600); err != nil {
		t.Fatal(err)
	}
	previewer, err := prepare.NewPreviewer(incompleteVariantCompleter{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory:           directory,
		Goal:                       prepare.GoalQuestionAnswer,
		Model:                      "model",
		MinimumQuestionAnswerFacts: 1,
	})
	if err == nil {
		t.Fatal("build succeeded without every required question variant")
	}
}

func TestBuildFullSkipsUnusablePassageWithoutRecursiveRepair(t *testing.T) {
	directory := t.TempDir()
	source := strings.Repeat("Table of contents navigation entry. ", 70) + "\n\n" +
		"The profiler uses SIGPROF to interrupt the program. " + strings.Repeat("Profiling context. ", 70)
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	model := &skippingPassageCompleter{}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory:           directory,
		Goal:                       prepare.GoalQuestionAnswer,
		Model:                      "model",
		MinimumQuestionAnswerFacts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 6 || artifact.ExcludedEvidence != 1 {
		t.Fatalf("artifact = %+v", artifact)
	}
	if model.requests != 3 {
		t.Fatalf("model requests = %d, want selection, pool, and verification without recursive repair", model.requests)
	}
}

func TestBuildFullExcludesOneUnusableEvidenceUnitWithoutDiscardingValidRecords(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte("Channels communicate values between goroutines."), 0o600); err != nil {
		t.Fatal(err)
	}
	previewer, err := prepare.NewPreviewer(qualityRejectingCompleter{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = previewer.BuildFull(context.Background(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
	})
	if err == nil || !strings.Contains(err.Error(), "no source passage produced a verified training record") {
		t.Fatalf("error = %v, want no verified training record", err)
	}
}

func TestBuildFullRejectsSemanticallyInvalidReviewedRecord(t *testing.T) {
	directory := t.TempDir()
	evidence := "Channels communicate values between goroutines."
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(evidence), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"results":[{"record_number":1,"expected_target_id":0,"reason":"The prompt asks about protection but the evidence only describes communication."}]}`},
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}

	_, err = previewer.BuildFull(context.Background(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		ReviewedRecords: []prepare.Record{{
			Prompt:   "What do channels protect?",
			Target:   evidence,
			Evidence: evidence,
		}},
		VerifiedPreviewRecords: []prepare.Record{{
			Prompt:   "What do channels communicate?",
			Target:   evidence,
			Evidence: evidence,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "edited preview record 1 is not supported") {
		t.Fatalf("error: %v", err)
	}
}

func TestBuildFullRejectsInvalidReviewedRecord(t *testing.T) {
	previewer, err := prepare.NewPreviewer(&fullBuildCompleter{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = previewer.BuildFull(context.Background(), prepare.FullBuildRequest{
		DatasetDirectory: t.TempDir(), Goal: prepare.GoalQuestionAnswer, Model: "model",
		ReviewedRecords: []prepare.Record{{Prompt: "Question?", Target: "invented", Evidence: "The actual grounded evidence is here."}},
	})
	if err == nil || !strings.Contains(err.Error(), "not an exact quote") {
		t.Fatalf("error: %v", err)
	}
}

func TestBuildFullDoesNotRegenerateSemanticFailures(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte("Channels communicate values between goroutines."), 0o600); err != nil {
		t.Fatal(err)
	}
	model := &semanticRejectingCompleter{}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}

	_, err = previewer.BuildFull(context.Background(), prepare.FullBuildRequest{
		DatasetDirectory:           directory,
		Goal:                       prepare.GoalQuestionAnswer,
		Model:                      "model",
		MinimumQuestionAnswerFacts: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "no source passage produced a verified training record") {
		t.Fatalf("error = %v, want no verified training record", err)
	}
	if len(model.requests) != 3 {
		t.Fatalf("requests: got %d want one selection, pool, and verification", len(model.requests))
	}
}

func TestBuildFullKeepsAnswerSelectionIndependentOfPreviousQuestions(t *testing.T) {
	directory := t.TempDir()
	var source strings.Builder
	for index := 1; index <= 41; index++ {
		fmt.Fprintf(&source, "Passage %d contains a distinct fact for bounded context. %s\n\n", index, strings.Repeat("detail ", 450))
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &fullBuildCompleter{}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := previewer.BuildFull(context.Background(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
	}); err != nil {
		t.Fatalf("build full: %v", err)
	}

	for _, request := range model.requests {
		content := request.Messages[1].Content
		if strings.Contains(content, "[RECORD 1]") {
			continue
		}
		if count := strings.Count(content, "[PASSAGE "); count > 5 {
			t.Fatalf("generation request contains %d passages; want at most 5", count)
		}
		if strings.Contains(content, "SOURCE:\n") && strings.Contains(content, "Do not repeat these previous prompts") {
			t.Fatal("answer selection was coupled to previously generated questions")
		}
	}
}

func TestValidateReviewedRecordsAllowsRepeatedClassificationTargets(t *testing.T) {
	records := []prepare.Record{
		{Prompt: "Classify the first record.", Target: "positive.", Evidence: "positive."},
		{Prompt: "Classify the second record.", Target: "positive.", Evidence: "positive. This is second evidence."},
	}
	if _, err := prepare.ValidateReviewedRecords(records); err != nil {
		t.Fatalf("repeated classification target should be valid: %v", err)
	}
}

func TestBuildFullHonorsCancellationBetweenBatches(t *testing.T) {
	directory := t.TempDir()
	recordsPath := filepath.Join(directory, "dataset.jsonl")
	reportPath := filepath.Join(directory, "dataset-report.json")
	for _, path := range []string{recordsPath, reportPath} {
		if err := os.WriteFile(path, []byte("previous dataset artifact\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var source strings.Builder
	for index := 1; index <= 9; index++ {
		fmt.Fprintf(&source, "Passage %d has a distinct grounded statement.\n\n", index)
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	previewer, _ := prepare.NewPreviewer(&fullBuildCompleter{})
	ctx, cancel := context.WithCancel(context.Background())
	_, err := previewer.BuildFull(ctx, prepare.FullBuildRequest{
		DatasetDirectory: directory, Goal: prepare.GoalQuestionAnswer, Model: "model",
		Progress: func(prepare.FullBuildProgress) { cancel() },
	})
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("error: %v", err)
	}
	for _, path := range []string{recordsPath, reportPath} {
		data, readErr := os.ReadFile(path)
		if readErr != nil || string(data) != "previous dataset artifact\n" {
			t.Fatalf("canceled build changed prior artifact %s: data %q, error %v", path, data, readErr)
		}
	}
}

func TestBuildFullResumesFromOneAtomicCheckpoint(t *testing.T) {
	directory := t.TempDir()
	var source strings.Builder
	for index := 1; index <= 12; index++ {
		fmt.Fprintf(&source, "Passage %d contains a distinct resumable fact. %s\n\n", index, strings.Repeat("detail ", 450))
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &fullBuildCompleter{}
	previewer, _ := prepare.NewPreviewer(model)
	ctx, cancel := context.WithCancel(context.Background())
	totalBatches := 0
	_, err := previewer.BuildFull(ctx, prepare.FullBuildRequest{
		DatasetDirectory: directory, Goal: prepare.GoalQuestionAnswer, Model: "model",
		Progress: func(progress prepare.FullBuildProgress) {
			totalBatches = progress.TotalBatches
			if progress.CompletedBatches == 1 {
				cancel()
			}
		},
	})
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("first build error = %v, want cancellation", err)
	}
	checkpointPath := filepath.Join(directory, ".dataset-build.json")
	if _, err := os.Stat(checkpointPath); err != nil {
		t.Fatalf("checkpoint was not persisted: %v", err)
	}
	callsBeforeResume := len(model.requests)

	var resumed []prepare.FullBuildProgress
	artifact, err := previewer.BuildFull(context.Background(), prepare.FullBuildRequest{
		DatasetDirectory: directory, Goal: prepare.GoalQuestionAnswer, Model: "model",
		Progress: func(progress prepare.FullBuildProgress) { resumed = append(resumed, progress) },
	})
	if err != nil {
		t.Fatalf("resume build: %v", err)
	}
	if len(resumed) == 0 || resumed[0].CompletedBatches != 1 || !strings.Contains(resumed[0].Stage, "batch 2") {
		t.Fatalf("resume started from the wrong batch: %+v", resumed)
	}
	assertBuildProgressContract(t, resumed)
	if got, want := len(model.requests)-callsBeforeResume, (totalBatches-1)*3; got != want {
		t.Fatalf("resume model calls = %d, want %d for %d remaining batches", got, want, totalBatches-1)
	}
	if len(artifact.Records) < 2 {
		t.Fatalf("resumed artifact records = %d", len(artifact.Records))
	}
	if _, err := os.Stat(checkpointPath); !os.IsNotExist(err) {
		t.Fatalf("completed checkpoint still exists: %v", err)
	}
}
