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

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
)

type fullBuildCompleter struct {
	requests []prepare.CompletionRequest
}

type qualityRejectingCompleter struct{}

type semanticRejectingCompleter struct {
	requests []prepare.CompletionRequest
}

type recoveringPlannedFactCompleter struct {
	requests         []prepare.CompletionRequest
	verificationRuns int
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

type partialBatchCompleter struct {
	acceptFailedFact bool
}

type malformedReviewedPoolCompleter struct {
	irrecoverable    bool
	poolCalls        int
	retryInstruction bool
}

type accumulatingQuestionCompleter struct {
	poolCalls         int
	verificationCalls int
}

func (c *accumulatingQuestionCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	content := request.Messages[1].Content
	switch {
	case strings.Contains(content, "[RECORD 1]"):
		c.verificationCalls++
		passCount := 3
		if c.verificationCalls > 1 {
			passCount = 2
		}
		return verificationCompletion(strings.Count(content, "[RECORD "), func(index int) bool { return index < passCount }), nil
	case strings.Contains(content, "[LOCKED FACT 1]"):
		c.poolCalls++
		return variantCompletion(strings.Count(content, "[LOCKED FACT "), fmt.Sprintf("Attempt %d", c.poolCalls)), nil
	default:
		return selectedTargetsCompletion(strings.Count(content, "[PASSAGE ")), nil
	}
}

func (c *malformedReviewedPoolCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	content := request.Messages[1].Content
	switch {
	case strings.Contains(content, "[RECORD 1]"):
		return verificationCompletion(strings.Count(content, "[RECORD "), func(int) bool { return true }), nil
	case strings.Contains(content, "[LOCKED FACT 1]"):
		c.poolCalls++
		if c.poolCalls == 1 {
			if c.irrecoverable {
				return prepare.Completion{Content: "not-json"}, nil
			}
			completion := variantCompletion(strings.Count(content, "[LOCKED FACT "), "Reviewed")
			completion.Content = strings.Replace(completion.Content, "?", "?\n", 1)
			return completion, nil
		}
		c.retryInstruction = strings.Contains(content, "[RETRY OUTPUT REQUIREMENTS]") && strings.Contains(content, "Escape every newline")
		return variantCompletion(strings.Count(content, "[LOCKED FACT "), "Reviewed retry"), nil
	default:
		return selectedTargetsCompletion(strings.Count(content, "[PASSAGE ")), nil
	}
}

func (c *partialBatchCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	content := request.Messages[1].Content
	switch {
	case strings.Contains(content, "[RECORD 1]"):
		count := strings.Count(content, "[RECORD ")
		return verificationCompletion(count, func(index int) bool {
			return c.acceptFailedFact || count == 2 && index < 1
		}), nil
	case strings.Contains(content, "[LOCKED FACT 1]"):
		prefix := "Checkpoint"
		if c.acceptFailedFact {
			prefix = "Resume"
		}
		return variantCompletion(strings.Count(content, "[LOCKED FACT "), prefix), nil
	default:
		return selectedTargetsCompletion(strings.Count(content, "[PASSAGE ")), nil
	}
}

func (qualityRejectingCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	content := request.Messages[1].Content
	if strings.Contains(content, "[RECORD 1]") {
		count := strings.Count(content, "[RECORD ")
		return verificationCompletion(count, func(int) bool { return true }), nil
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

func (c *recoveringPlannedFactCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	c.requests = append(c.requests, request)
	content := request.Messages[1].Content
	switch {
	case strings.Contains(content, "[RECORD 1]"):
		c.verificationRuns++
		passed := c.verificationRuns > 1
		return verificationCompletion(strings.Count(content, "[RECORD "), func(int) bool { return passed }), nil
	case strings.Contains(content, "[LOCKED FACT 1]"):
		return variantCompletion(strings.Count(content, "[LOCKED FACT "), fmt.Sprintf("Attempt %d", c.verificationRuns+1)), nil
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
		if c.allowGenerated && strings.Contains(content, "Beta is the second supported fact.") {
			completion := variantCompletion(1, "Generated")
			completion.Content = strings.Replace(completion.Content, `"record_1"`, `"record_2"`, 1)
			return completion, nil
		}
		return variantCompletion(1, "Reviewed"), nil
	case strings.Contains(content, "[LOCKED FACT 1]"):
		if c.allowGenerated && strings.Contains(content, "Beta is the second supported fact.") {
			return variantCompletion(1, "Generated"), nil
		}
		return questionCompletion("not a question"), nil
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
	targetIDs := make([][]int, count)
	for index := range targetIDs {
		targetIDs[index] = []int{1}
	}
	data, _ := json.Marshal(map[string]any{"target_ids": targetIDs})
	return prepare.Completion{Content: string(data)}
}

func variantCompletion(count int, prefix string) prepare.Completion {
	questions := make(map[string]string, count)
	for index := range count {
		questions[fmt.Sprintf("record_%d", index+1)] = fmt.Sprintf("%s: Which term names fact %d?", prefix, index+1)
	}
	data, _ := json.Marshal(questions)
	return prepare.Completion{Content: string(data)}
}

func questionCompletion(questions ...string) prepare.Completion {
	response := make(map[string]string, len(questions))
	for index, question := range questions {
		response[fmt.Sprintf("record_%d", index+1)] = question
	}
	data, _ := json.Marshal(response)
	return prepare.Completion{Content: string(data)}
}

func verificationCompletion(count int, passes func(int) bool) prepare.Completion {
	verdicts := make(map[string]string, count)
	for index := range count {
		outcome := "task_quality"
		if passes(index) {
			outcome = "pass"
		}
		verdicts[fmt.Sprintf("record_%d", index+1)] = outcome
	}
	data, _ := json.Marshal(verdicts)
	return prepare.Completion{Content: string(data)}
}

func (incompleteVariantCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	content := request.Messages[1].Content
	switch {
	case strings.Contains(content, "[RECORD 1]"):
		return verificationCompletion(strings.Count(content, "[RECORD "), func(int) bool { return true }), nil
	case strings.Contains(content, "[LOCKED FACT 1]"):
		return questionCompletion("What is the first alternate phrasing?"), nil
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
		return prepare.Completion{Content: `{"target_ids":[[],[1]]}`}, nil
	}
}

func (c *fullBuildCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	c.requests = append(c.requests, request)
	content := request.Messages[1].Content
	if strings.Contains(content, "[RECORD 1]") {
		count := strings.Count(content, "[RECORD ")
		completion := verificationCompletion(count, func(int) bool { return true })
		completion.PromptTokens = 2
		completion.CompletionTokens = 1
		return completion, nil
	}
	if strings.Contains(content, "[LOCKED FACT 1]") {
		count := strings.Count(content, "[LOCKED FACT ")
		completion := variantCompletion(count, fmt.Sprintf("Request %d", len(c.requests)))
		completion.PromptTokens = 3
		completion.CompletionTokens = 2
		return completion, nil
	}

	count := requestedRecordCount(content)
	targetIDs := make([][]int, count)
	for index := range targetIDs {
		targetIDs[index] = []int{1}
	}
	data, _ := json.Marshal(map[string]any{"target_ids": targetIDs})
	return prepare.Completion{Content: string(data), PromptTokens: 3, CompletionTokens: 2}, nil
}

func requestedRecordCount(prompt string) int {
	var count int
	if start := strings.Index(prompt, "Review "); start >= 0 {
		prompt = prompt[start:]
	}
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
		DatasetInstructions:    "Write for experienced Go developers.",
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
	if len(artifact.Records) != 2 || artifact.ReviewedRecordCount != 1 || artifact.GeneratedRecordCount != 1 {
		t.Fatalf("artifact: %+v", artifact)
	}
	independent := 0
	for _, record := range artifact.Records {
		if record.IndependentPrompt {
			independent++
		}
	}
	if independent != 0 {
		t.Fatalf("independent prompts = %d, want none for a dataset below the evaluation threshold", independent)
	}
	assertBuildProgressContract(t, progress)
	lastProgress := progress[len(progress)-1]
	if !slices.ContainsFunc(progress, func(update prepare.FullBuildProgress) bool { return update.Phase == "analysis" }) ||
		!slices.ContainsFunc(progress, func(update prepare.FullBuildProgress) bool { return update.Stage == "Checking reviewed examples" }) ||
		!slices.ContainsFunc(progress, func(update prepare.FullBuildProgress) bool {
			return update.Stage == "Independently verifying planned records"
		}) || lastProgress.Stage != "Dataset preparation complete" || lastProgress.CompletedBatches != 1 || lastProgress.CompletedUnits != lastProgress.TotalUnits || lastProgress.AcceptedRecords != 2 {
		t.Fatalf("progress: %+v", progress)
	}
	if !slices.ContainsFunc(model.requests, func(request prepare.CompletionRequest) bool {
		return strings.Contains(request.Messages[1].Content, "[RECORD 1]")
	}) {
		t.Fatal("full build did not independently verify the reviewed preview")
	}
	wantCalls := []string{"verify", "select answers", "create questions", "verify"}
	if got := completionPurposes(model.requests); !slices.Equal(got, wantCalls) {
		t.Fatalf("model calls = %v, want %v", got, wantCalls)
	}
	if artifact.GenerationAttempts != 2 || artifact.VerificationAttempts != 2 || artifact.PromptTokens != 10 || artifact.CompletionTokens != 6 {
		t.Fatalf("model cost = generations %d, verifications %d, prompt tokens %d, completion tokens %d",
			artifact.GenerationAttempts, artifact.VerificationAttempts, artifact.PromptTokens, artifact.CompletionTokens)
	}
	for _, request := range model.requests {
		if request.Seed != 3407 {
			t.Fatalf("preparation request seed = %d, want 3407", request.Seed)
		}
		hasInstructions := strings.Contains(request.Messages[1].Content, "experienced Go developers")
		if !hasInstructions {
			t.Fatalf("dataset instructions missing from model request: %s", request.Messages[1].Content)
		}
		if !strings.HasPrefix(request.Messages[1].Content, "USER INSTRUCTION — FOLLOW WHEN WRITING EVERY RECORD:\n") {
			t.Fatalf("model request did not begin with the exact user instruction: %s", request.Messages[1].Content)
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
	if !strings.Contains(variantRequest.Messages[1].Content, "exactly one natural question") || !strings.Contains(string(variantSchema), `"record_1"`) {
		t.Fatalf("question pool does not request one candidate: %s", variantSchema)
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
	if report.DatasetInstructions != "Write for experienced Go developers." {
		t.Fatalf("persisted instructions = %q", report.DatasetInstructions)
	}
}

func TestQuestionAnswerBuildProducesTrainingContractEndToEnd(t *testing.T) {
	directory := t.TempDir()
	var source strings.Builder
	for index := 1; index <= finetune.MinimumKnowledgeFacts; index++ {
		fmt.Fprintf(&source, "Supported behavior number %d is documented for this feature. %s\n\n", index, strings.Repeat("Additional grounded context explains this behavior. ", 35))
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	previewer, err := prepare.NewPreviewer(&fullBuildCompleter{})
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
	})
	if err != nil {
		t.Fatalf("build complete dataset: %v", err)
	}
	trainingDirectory := filepath.Join(directory, "training")
	if err := os.Mkdir(trainingDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	contract, err := finetune.BuildDatasetContract(artifact.RecordsPath, trainingDirectory)
	if err != nil {
		t.Fatalf("build training contract: %v", err)
	}
	if len(contract.Train) != finetune.MinimumKnowledgeFacts || len(contract.Validation) != finetune.MinimumValidationRecords || len(contract.Test) != finetune.MinimumTestRecords {
		t.Fatalf("contract split sizes = %d/%d/%d", len(contract.Train), len(contract.Validation), len(contract.Test))
	}
	for _, name := range []string{"train.jsonl", "validation.jsonl", "test.jsonl", "contract.json"} {
		info, err := os.Stat(filepath.Join(trainingDirectory, name))
		if err != nil || info.Size() == 0 {
			t.Fatalf("training artifact %s was not produced: info=%v error=%v", name, info, err)
		}
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
		if update.Phase == "analysis" {
			continue
		}
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
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte("First independently supported source fact."), 0o600); err != nil {
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
	if len(artifact.Records) != 2 || artifact.ReviewedRecordCount != 1 || artifact.GeneratedRecordCount != 1 {
		t.Fatalf("artifact = %+v", artifact)
	}
	if !slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
		return record.Prompt == "What is the reviewed fact?" && record.Target == reviewedEvidence
	}) {
		t.Fatalf("reviewed fact was not retained: %+v", artifact.Records)
	}
}

func TestBuildFullIncludesUserApprovedQuestionAnswerExactlyAsWritten(t *testing.T) {
	directory := t.TempDir()
	evidence := "The source says Taylor leads the project."
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(evidence), 0o600); err != nil {
		t.Fatal(err)
	}
	previewer, err := prepare.NewPreviewer(&fullBuildCompleter{})
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		ReviewedRecords: []prepare.Record{{
			Prompt: "  Tell me who leads the project  ", Target: "  Taylor  ", Evidence: evidence, UserApproved: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ReviewedRecordCount != 1 {
		t.Fatalf("reviewed record count = %d, want 1", artifact.ReviewedRecordCount)
	}
	if !slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
		return record.Prompt == "Tell me who leads the project" && record.Target == "Taylor" && record.UserApproved
	}) {
		t.Fatalf("approved edit was not retained exactly in training records: %+v", artifact.Records)
	}
	if slices.ContainsFunc(artifact.SpareRecords, func(record prepare.Record) bool {
		return record.Prompt == "Tell me who leads the project"
	}) {
		t.Fatalf("approved edit was moved to spare records: %+v", artifact.SpareRecords)
	}
	if slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
		return record.IndependentPrompt && record.UserApproved
	}) {
		t.Fatalf("approved edit was marked as an evaluation record: %+v", artifact.Records)
	}
}

func TestBuildFullSemanticallyVerifiesUserApprovedQuestionAnswer(t *testing.T) {
	directory := t.TempDir()
	evidence := "The source says Jordan leads the project."
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(evidence), 0o600); err != nil {
		t.Fatal(err)
	}
	previewer, err := prepare.NewPreviewer(&semanticRejectingCompleter{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		ReviewedRecords: []prepare.Record{{
			Prompt: "Who leads the project?", Target: "Taylor", Evidence: evidence, UserApproved: true,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "reviewed preview record 1 is not supported") {
		t.Fatalf("error = %v, want unsupported approved edit to be rejected", err)
	}
}

func TestBuildFullRetriesMalformedReviewedQuestionPoolJSON(t *testing.T) {
	for _, test := range []struct {
		name          string
		irrecoverable bool
		wantCalls     int
	}{
		{name: "literal newline is retried", wantCalls: 2},
		{name: "irrecoverable response is retried", irrecoverable: true, wantCalls: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			evidence := "Go developers use the Playground to share code ideas, test theories, and debug their code."
			if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(evidence), 0o600); err != nil {
				t.Fatal(err)
			}
			model := &malformedReviewedPoolCompleter{irrecoverable: test.irrecoverable}
			previewer, err := prepare.NewPreviewer(model)
			if err != nil {
				t.Fatal(err)
			}

			artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
				DatasetDirectory: directory,
				Goal:             prepare.GoalQuestionAnswer,
				Model:            "model",
				ReviewedRecords: []prepare.Record{{
					Prompt: "Why do Go developers use the Playground?", Target: evidence, Evidence: evidence, UserApproved: true,
				}},
				VerifiedPreviewRecords: []prepare.Record{{
					Prompt: "Why do Go developers use the Playground?", Target: evidence, Evidence: evidence,
				}},
			})
			if err != nil {
				t.Fatalf("build with malformed model JSON: %v", err)
			}
			if len(artifact.Records) != 1 || model.poolCalls != test.wantCalls || !model.retryInstruction {
				t.Fatalf("records = %d, question-pool calls = %d, retry instruction = %t; want 1, %d, true", len(artifact.Records), model.poolCalls, model.retryInstruction, test.wantCalls)
			}
		})
	}
}

func TestBuildFullKeepsUserApprovedRecordWithoutRequiringAlternateQuestions(t *testing.T) {
	directory := t.TempDir()
	evidence := "The source says Jordan leads the project."
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(evidence), 0o600); err != nil {
		t.Fatal(err)
	}
	previewer, err := prepare.NewPreviewer(incompleteVariantCompleter{})
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		ReviewedRecords: []prepare.Record{{
			Prompt: "Who leads the project?", Target: "Taylor", Evidence: evidence, UserApproved: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
		return record.Prompt == "Who leads the project?" && record.Target == "Taylor" && record.UserApproved
	}) {
		t.Fatalf("artifact = %+v, want the approved training record retained", artifact)
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
		DatasetDirectory:       directory,
		Goal:                   prepare.GoalQuestionAnswer,
		Model:                  "model",
		ReviewedRecords:        []prepare.Record{{Prompt: generatedPreview.Prompt, Target: generatedPreview.Target, Evidence: generatedPreview.Evidence, UserApproved: true}},
		VerifiedPreviewRecords: []prepare.Record{generatedPreview},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ReviewedRecordCount != 0 || len(artifact.Records) != 1 {
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
		DatasetDirectory:       directory,
		Goal:                   prepare.GoalQuestionAnswer,
		Model:                  "model",
		ReviewedRecords:        reviewed,
		VerifiedPreviewRecords: reviewed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 2 || artifact.ReviewedRecordCount != 2 {
		t.Fatalf("artifact = %+v, want both reviewed training facts", artifact)
	}
	if !slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
		return record.Prompt == reviewed[0].Prompt
	}) {
		t.Fatalf("complete reviewed fact was not retained: %+v", artifact.Records)
	}
}

func TestBuildFullRetainsEveryCompleteReviewedFact(t *testing.T) {
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
		DatasetDirectory:       directory,
		Goal:                   prepare.GoalQuestionAnswer,
		Model:                  "model",
		ReviewedRecords:        reviewed,
		VerifiedPreviewRecords: reviewed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 2 || artifact.ReviewedRecordCount != 2 {
		t.Fatalf("artifact = %+v, want both complete facts", artifact)
	}
	if !slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
		return record.Target == secondEvidence
	}) {
		t.Fatalf("artifact omitted the second fact: %+v", artifact.Records)
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
		DatasetDirectory:       directory,
		Goal:                   prepare.GoalQuestionAnswer,
		Model:                  "model",
		ReviewedRecords:        reviewed,
		VerifiedPreviewRecords: reviewed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != len(targets) {
		t.Fatalf("records = %d, want one record for every fact", len(artifact.Records))
	}
	for index := range targets {
		if !slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
			return record.Target == targets[index]
		}) {
			t.Fatalf("did not retain fact %q", targets[index])
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
		DatasetDirectory:       directory,
		Goal:                   prepare.GoalQuestionAnswer,
		Model:                  "model",
		ReviewedRecords:        reviewed,
		VerifiedPreviewRecords: reviewed,
	})
	if err != nil {
		t.Fatal(err)
	}
	// SIGPROF is a legitimate grounded short answer distinct from the reviewed
	// explanatory sentence and must no longer be discarded by word count.
	if len(artifact.Records) != len(targets)+1+finetune.MinimumValidationRecords+finetune.MinimumTestRecords {
		t.Fatalf("records = %d, want one training record per fact plus fixed evaluation prompts", len(artifact.Records))
	}
	if !slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool { return record.Target == "SIGPROF" }) {
		t.Fatalf("records omitted grounded short target: %+v", artifact.Records)
	}
	for _, target := range weakTargets {
		if !slices.ContainsFunc(artifact.Records, func(record prepare.Record) bool {
			return record.Target == target
		}) {
			t.Fatalf("omitted reviewed target %q", target)
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
		DatasetDirectory:       directory,
		Goal:                   prepare.GoalQuestionAnswer,
		Model:                  "model",
		ReviewedRecords:        reviewed,
		VerifiedPreviewRecords: reviewed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 3 {
		t.Fatalf("records = %d, want all three complete facts backfilled", len(artifact.Records))
	}
}

func TestBuildFullProcessesEveryPlannedBatch(t *testing.T) {
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
				DatasetDirectory: directory,
				Goal:             prepare.GoalQuestionAnswer,
				Model:            "model",
			})
			if test.failSpare {
				if err == nil || !strings.Contains(err.Error(), "optional spare batch failed") {
					t.Fatalf("error = %v, want analysis failure", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if model.selections != 2 || len(artifact.Records) != 16 {
				t.Fatalf("selection batches = %d, records = %d; want both planning batches and one record per planned fact", model.selections, len(artifact.Records))
			}
			if artifact.SpareRecordsPath != "" || len(artifact.SpareRecords) != 0 {
				t.Fatalf("document-dependent build unexpectedly created spare records: %+v", artifact)
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
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
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
		t.Fatalf("selection batches before resume = %d, want both 12-chunk planning batches", model.selections)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if model.selections != 2 {
		t.Fatalf("resume repeated completed spare batch; selection batches = %d", model.selections)
	}
	if len(artifact.Records) != 16 {
		t.Fatalf("records = %d, want every planned fact", len(artifact.Records))
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
		DatasetDirectory:       directory,
		Goal:                   prepare.GoalQuestionAnswer,
		Model:                  "model",
		ReviewedRecords:        []prepare.Record{reviewed},
		VerifiedPreviewRecords: []prepare.Record{reviewed},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 2 || artifact.ReviewedRecordCount != 1 {
		t.Fatalf("artifact = %+v, want the reviewed and generated facts", artifact)
	}
}

func TestBuildFullAcceptsFactWhenOneGeneratedQuestionIsUsable(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte("One grounded fact supports this training record."), 0o600); err != nil {
		t.Fatal(err)
	}
	previewer, err := prepare.NewPreviewer(incompleteVariantCompleter{})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 1 {
		t.Fatalf("records = %d, want one verified training record", len(artifact.Records))
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
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 1 || artifact.ExcludedEvidence != 1 {
		t.Fatalf("artifact = %+v", artifact)
	}
	if model.requests != 3 {
		t.Fatalf("model requests = %d, want selection, pool, and verification without recursive repair", model.requests)
	}
}

func TestBuildFullRejectsRecoveredFactWithoutUsableQuestions(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte("Channels communicate values between goroutines."), 0o600); err != nil {
		t.Fatal(err)
	}
	previewer, err := prepare.NewPreviewer(qualityRejectingCompleter{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
	})
	if err == nil || !strings.Contains(err.Error(), "planned facts remain unresolved after 3 regeneration attempts") {
		t.Fatalf("error = %v, want question quality failure", err)
	}
}

func TestBuildFullRejectsSemanticallyInvalidReviewedRecord(t *testing.T) {
	directory := t.TempDir()
	evidence := "Channels communicate values between goroutines."
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(evidence), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		verificationCompletion(1, func(int) bool { return false }),
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
	if err == nil || !strings.Contains(err.Error(), "reviewed preview record 1 is not supported") {
		t.Fatalf("error: %v", err)
	}
	if len(model.requests) != 1 {
		t.Fatalf("model requests = %d, want reviewed-record verification before source analysis", len(model.requests))
	}
	if _, statErr := os.Stat(filepath.Join(directory, "dataset-plan.json")); !os.IsNotExist(statErr) {
		t.Fatalf("invalid reviewed record started source analysis: %v", statErr)
	}
}

func TestBuildFullReplacesUnchangedPreviewRecordRejectedBeforeAnalysis(t *testing.T) {
	directory := t.TempDir()
	evidence := "Channels communicate values between goroutines."
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(evidence), 0o600); err != nil {
		t.Fatal(err)
	}
	record := prepare.Record{Prompt: "What unrelated role do channels perform?", Target: evidence, Evidence: evidence}
	model := &sequenceCompleter{completions: []prepare.Completion{
		verificationCompletion(1, func(int) bool { return false }),
		selectedTargetsCompletion(1),
		variantCompletion(1, "Replacement"),
		verificationCompletion(1, func(int) bool { return true }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory:       directory,
		Goal:                   prepare.GoalQuestionAnswer,
		Model:                  "model",
		ReviewedRecords:        []prepare.Record{record},
		VerifiedPreviewRecords: []prepare.Record{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ReviewedRecordCount != 0 || len(artifact.Records) != 1 {
		t.Fatalf("artifact = %+v, want one generated replacement group", artifact)
	}
	if slices.ContainsFunc(artifact.Records, func(candidate prepare.Record) bool { return candidate.Prompt == record.Prompt }) {
		t.Fatalf("rejected preview prompt was retained: %+v", artifact.Records)
	}
}

func TestBuildFullReverifiesUnchangedReviewedRecord(t *testing.T) {
	directory := t.TempDir()
	evidence := "Write a concise explanation of the supported behavior."
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(evidence), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		verificationCompletion(1, func(int) bool { return false }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	record := prepare.Record{Prompt: "Write the supported explanation.", Target: evidence, Evidence: evidence}

	_, err = previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory:       directory,
		Goal:                   prepare.GoalTextGeneration,
		Model:                  "model",
		DatasetInstructions:    "Use detailed explanations for an expert audience.",
		ReviewedRecords:        []prepare.Record{record},
		VerifiedPreviewRecords: []prepare.Record{record},
	})
	if err == nil || !strings.Contains(err.Error(), "reviewed preview record 1 is not supported") {
		t.Fatalf("build error = %v, want unchanged reviewed row to be rejected", err)
	}
	if len(model.requests) != 1 || !strings.Contains(model.requests[0].Messages[1].Content, "Use detailed explanations for an expert audience.") {
		t.Fatalf("full build did not independently verify the unchanged row against active instructions: %+v", model.requests)
	}
	if _, statErr := os.Stat(filepath.Join(directory, "dataset.jsonl")); !os.IsNotExist(statErr) {
		t.Fatalf("failed build published dataset: %v", statErr)
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

func TestBuildFullBoundsRegenerationForPersistentSemanticFailures(t *testing.T) {
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
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
	})
	if err == nil || !strings.Contains(err.Error(), "planned facts remain unresolved after 3 regeneration attempts") {
		t.Fatalf("error = %v, want incomplete planned fact", err)
	}
	if len(model.requests) != 5 {
		t.Fatalf("requests: got %d, want selection, three bounded generation attempts, and one verification before duplicate retries are rejected locally", len(model.requests))
	}
}

func TestBuildFullRetainsVerifiedQuestionsAcrossRegenerationAttempts(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte("Channels communicate values between goroutines."), 0o600); err != nil {
		t.Fatal(err)
	}
	model := &accumulatingQuestionCompleter{}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 1 {
		t.Fatalf("records = %d, want one verified question", len(artifact.Records))
	}
	if model.poolCalls != 1 || model.verificationCalls != 1 {
		t.Fatalf("pool calls = %d, verification calls = %d; want one of each", model.poolCalls, model.verificationCalls)
	}
}

func TestBuildFullCheckpointsCompleteFactsBeforeReportingFailedFact(t *testing.T) {
	directory := t.TempDir()
	source := "Channels communicate typed values between concurrent goroutines. " + strings.Repeat("Additional channel context explains this supported communication behavior in detail. ", 20) + "\n\n" +
		"Mutexes protect shared state from concurrent access. " + strings.Repeat("Additional mutex context explains this supported synchronization behavior in detail. ", 20)
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	model := &partialBatchCompleter{}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}

	_, err = previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
	})
	if err == nil || !strings.Contains(err.Error(), "completed facts were checkpointed") {
		t.Fatalf("first build error = %v, want an explicit failed-fact error", err)
	}
	checkpointData, err := os.ReadFile(filepath.Join(directory, ".dataset-build.json"))
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint struct {
		Records       []prepare.Record       `json:"records"`
		RejectedFacts []prepare.RejectedFact `json:"rejected_facts"`
	}
	if err := json.Unmarshal(checkpointData, &checkpoint); err != nil {
		t.Fatal(err)
	}
	if len(checkpoint.Records) != 1 || len(checkpoint.RejectedFacts) != 1 {
		t.Fatalf("checkpoint = %+v, want one complete fact and one explicit failed fact", checkpoint)
	}

	model.acceptFailedFact = true
	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
	})
	if err != nil {
		t.Fatalf("resume build: %v", err)
	}
	if len(artifact.Records) != 2 || len(artifact.RejectedFacts) != 0 {
		t.Fatalf("resumed artifact = %+v, want both complete facts and no rejection", artifact)
	}
}

func TestBuildFullStopsAtFirstUnresolvedBatch(t *testing.T) {
	directory := t.TempDir()
	var source strings.Builder
	for index := 1; index <= 20; index++ {
		fmt.Fprintf(&source, "Passage %d contains an independently supported fact. %s\n\n", index, strings.Repeat("grounded detail ", 180))
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	model := &semanticRejectingCompleter{}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	_, err = previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
	})
	if err == nil || !strings.Contains(err.Error(), "build dataset batch 1 of") {
		t.Fatalf("build error = %v, want an immediate first-batch failure", err)
	}
	planData, err := os.ReadFile(filepath.Join(directory, "dataset-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Facts []json.RawMessage `json:"facts"`
	}
	if err := json.Unmarshal(planData, &plan); err != nil {
		t.Fatal(err)
	}
	checkpointData, err := os.ReadFile(filepath.Join(directory, ".dataset-build.json"))
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint struct {
		CompletedBatches int                    `json:"completed_batches"`
		RejectedFacts    []prepare.RejectedFact `json:"rejected_facts"`
	}
	if err := json.Unmarshal(checkpointData, &checkpoint); err != nil {
		t.Fatal(err)
	}
	if len(plan.Facts) <= 16 || checkpoint.CompletedBatches != 0 || len(checkpoint.RejectedFacts) == 0 || len(checkpoint.RejectedFacts) > 16 {
		t.Fatalf("checkpoint = completed batches %d, rejected facts %d, planned facts %d; build should stop within its first batch", checkpoint.CompletedBatches, len(checkpoint.RejectedFacts), len(plan.Facts))
	}
}

func TestBuildFullRegeneratesIncompletePlannedFactWithDifferentQuestions(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "extracted.md"), []byte("Channels communicate typed values between concurrent goroutines."), 0o600); err != nil {
		t.Fatal(err)
	}
	model := &recoveringPlannedFactCompleter{}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.BuildFull(t.Context(), prepare.FullBuildRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 1 || len(model.requests) != 5 {
		t.Fatalf("artifact records = %d, requests = %d", len(artifact.Records), len(model.requests))
	}
	firstPool := model.requests[1]
	secondPool := model.requests[3]
	if firstPool.Seed == secondPool.Seed || !strings.Contains(secondPool.Messages[1].Content, "[QUESTIONS TO AVOID — WRITE DIFFERENT QUESTIONS]") || !strings.Contains(secondPool.Messages[1].Content, "Attempt 1:") {
		t.Fatalf("regeneration repeated the same request: first=%+v second=%+v", firstPool, secondPool)
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
		if count := strings.Count(content, "[PASSAGE "); count > 12 {
			t.Fatalf("generation request contains %d passages; want at most 12", count)
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
	firstGeneration := slices.IndexFunc(resumed, func(progress prepare.FullBuildProgress) bool { return progress.Phase == "generation" })
	if firstGeneration < 0 || resumed[firstGeneration].CompletedBatches != 1 || !strings.Contains(resumed[firstGeneration].Stage, "batch 2") {
		t.Fatalf("resume started from the wrong batch: %+v", resumed)
	}
	assertBuildProgressContract(t, resumed)
	if got, want := len(model.requests)-callsBeforeResume, (totalBatches-1)*2; got != want {
		t.Fatalf("resume model calls = %d, want %d for %d remaining batches", got, want, totalBatches-1)
	}
	if len(artifact.Records) < 2 {
		t.Fatalf("resumed artifact records = %d", len(artifact.Records))
	}
	if _, err := os.Stat(checkpointPath); !os.IsNotExist(err) {
		t.Fatalf("completed checkpoint still exists: %v", err)
	}
}

func TestMergeFullBuildArtifactsPreservesDocumentProvenance(t *testing.T) {
	directory := t.TempDir()
	artifact, err := prepare.MergeFullBuildArtifacts(prepare.FullBuildMergeRequest{
		DatasetDirectory: directory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "preparation-model",
		Documents: []prepare.FullBuildDocument{
			{
				ID: "001", Name: "guide.pdf",
				Artifact: prepare.FullBuildArtifact{
					Records:             []prepare.Record{{Prompt: "What does the guide explain?", Target: "The first fact.", Evidence: "Guide evidence.", FactID: "fact-a"}},
					SpareRecords:        []prepare.Record{{Prompt: "What spare detail does the guide provide?", Target: "The spare fact.", Evidence: "Spare guide evidence.", FactID: "spare-a"}},
					ReviewedRecordCount: 1, GenerationAttempts: 2,
				},
			},
			{
				ID: "002", Name: "manual.docx",
				Artifact: prepare.FullBuildArtifact{
					Records:              []prepare.Record{{Prompt: "What does the manual explain?", Target: "The second fact.", Evidence: "Manual evidence.", FactID: "fact-b"}},
					GeneratedRecordCount: 1, VerificationAttempts: 3,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("merge full builds: %v", err)
	}
	if len(artifact.Records) != 2 || len(artifact.SpareRecords) != 1 {
		t.Fatalf("merged artifact sizes = records:%d spares:%d", len(artifact.Records), len(artifact.SpareRecords))
	}
	wantFacts := []string{"001-fact-a", "002-fact-b"}
	wantSources := []string{"guide.pdf", "manual.docx"}
	for index, record := range artifact.Records {
		if record.FactID != wantFacts[index] || record.SourceDocument != wantSources[index] {
			t.Fatalf("record %d provenance = %+v", index, record)
		}
	}
	if artifact.SpareRecords[0].FactID != "001-spare-a" || artifact.SpareRecords[0].SourceDocument != "guide.pdf" {
		t.Fatalf("spare provenance = %+v", artifact.SpareRecords[0])
	}
	if artifact.ReviewedRecordCount != 1 || artifact.GeneratedRecordCount != 1 || artifact.GenerationAttempts != 2 || artifact.VerificationAttempts != 3 {
		t.Fatalf("merged metrics = %+v", artifact)
	}

	data, err := os.ReadFile(artifact.RecordsPath)
	if err != nil {
		t.Fatalf("read merged records: %v", err)
	}
	if !strings.Contains(string(data), `"source_document":"guide.pdf"`) || !strings.Contains(string(data), `"source_document":"manual.docx"`) {
		t.Fatalf("merged JSONL lost document provenance: %s", data)
	}
	var report prepare.FullBuildReport
	reportData, err := os.ReadFile(artifact.ReportPath)
	if err != nil {
		t.Fatalf("read merged report: %v", err)
	}
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatalf("decode merged report: %v", err)
	}
	if len(report.Documents) != 2 || report.Documents[0].Name != "guide.pdf" || report.Documents[1].Name != "manual.docx" {
		t.Fatalf("merged document report = %+v", report.Documents)
	}
}
