package prepare_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
)

type completer struct {
	completion prepare.Completion
	request    prepare.CompletionRequest
}

func (c *completer) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	c.request = request
	return c.completion, nil
}

type sequenceCompleter struct {
	completions []prepare.Completion
	requests    []prepare.CompletionRequest
}

func (c *sequenceCompleter) Complete(_ context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	c.requests = append(c.requests, request)
	if len(c.requests) > len(c.completions) {
		return prepare.Completion{}, fmt.Errorf("unexpected completion request %d", len(c.requests))
	}
	return c.completions[len(c.requests)-1], nil
}

func TestBuildPreview(t *testing.T) {
	datasetDirectory := t.TempDir()
	markdown := "# Concurrency\n\nA goroutine is an independently executing function in Go. It is lightweight.\n\nChannels communicate values between goroutines."
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte(markdown), 0o600); err != nil {
		t.Fatalf("write Markdown: %v", err)
	}

	generated := map[string]any{"records": []map[string]any{
		{
			"prompt":      "What is a goroutine?",
			"evidence_id": 1,
			"target_id":   1,
		},
		{
			"prompt":      "What do channels communicate?",
			"evidence_id": 2,
			"target_id":   1,
		},
	}}
	content, err := json.Marshal(generated)
	if err != nil {
		t.Fatalf("encode completion: %v", err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{
			Content:          string(content),
			PromptTokens:     120,
			CompletionTokens: 40,
		},
		func() prepare.Completion {
			completion := verificationCompletion(2, func(int) bool { return true })
			completion.PromptTokens = 60
			completion.CompletionTokens = 20
			return completion
		}(),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatalf("new previewer: %v", err)
	}

	var progress []prepare.PreviewProgress
	artifact, err := previewer.Build(context.Background(), prepare.PreviewRequest{
		DatasetDirectory:    datasetDirectory,
		Goal:                prepare.GoalQuestionAnswer,
		Model:               "Qwen3-8B-Q8_0",
		RecordCount:         2,
		DatasetInstructions: "  Write for beginning Go developers.  ",
		Progress: func(update prepare.PreviewProgress) {
			progress = append(progress, update)
		},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if len(artifact.Records) != 2 || artifact.PromptTokens != 180 || artifact.CompletionTokens != 60 || artifact.VerificationAttempts != 1 {
		t.Fatalf("artifact: got %+v", artifact)
	}
	if len(progress) != 2 || progress[0].CompletedStages != 1 || progress[1].CompletedStages != 2 {
		t.Fatalf("progress: got %+v", progress)
	}
	requestContent := model.requests[0].Messages[1].Content
	if model.requests[0].Model != "Qwen3-8B-Q8_0" ||
		model.requests[0].MaxTokens != 1024 ||
		!strings.Contains(requestContent, "[PASSAGE 1]") ||
		!strings.Contains(requestContent, "A goroutine is an independently executing function in Go.") ||
		!strings.Contains(requestContent, "USER INSTRUCTION — FOLLOW WHEN WRITING EVERY RECORD:\nWrite for beginning Go developers.") ||
		strings.Contains(requestContent, `"Write for beginning Go developers."`) ||
		strings.Index(requestContent, "USER INSTRUCTION") > strings.Index(requestContent, "Requirements:") {
		t.Fatalf("completion request did not contain the selected model, source, and required user instructions")
	}
	if !strings.Contains(model.requests[0].Messages[0].Content, "user's instruction controls how every record is written") ||
		!strings.Contains(model.requests[0].Messages[0].Content, "must be followed") {
		t.Fatalf("generation system prompt did not give user instructions priority over defaults: %s", model.requests[0].Messages[0].Content)
	}
	if !strings.HasPrefix(model.requests[1].Messages[1].Content, "USER INSTRUCTION — FOLLOW WHEN WRITING EVERY RECORD:\nWrite for beginning Go developers.") ||
		!strings.Contains(model.requests[1].Messages[1].Content, "beginning Go developers") {
		t.Fatal("independent verification did not evaluate user dataset requirements")
	}
	verificationSchema, err := json.Marshal(model.requests[1].ResponseSchema)
	if err != nil {
		t.Fatalf("encode verification schema: %v", err)
	}
	if strings.Contains(string(verificationSchema), `"reason"`) || model.requests[1].MaxTokens != 144 {
		t.Fatalf("verification request includes unnecessary model output: schema=%s max_tokens=%d", verificationSchema, model.requests[1].MaxTokens)
	}
	report, err := os.ReadFile(artifact.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(report), `"dataset_instructions": "Write for beginning Go developers."`) {
		t.Fatalf("preview report omitted normalized dataset instructions: %s", report)
	}
	verificationRules := model.requests[1].Messages[0].Content
	if !strings.Contains(verificationRules, "natural question ending with a question mark") || !strings.Contains(verificationRules, "entire function, transcript, trace, benchmark report") {
		t.Fatalf("verification request is missing task-aware quality rules: %s", verificationRules)
	}
	verificationContent := model.requests[1].Messages[1].Content
	if !strings.Contains(verificationContent, "EVIDENCE: A goroutine is an independently executing function in Go. It is lightweight.") ||
		!strings.Contains(verificationContent, "[TARGET 1] A goroutine is an independently executing function in Go.") {
		t.Fatalf("verification request must contain the selected target and its evidence: %s", verificationContent)
	}
	if _, err := os.Stat(artifact.RecordsPath); err != nil {
		t.Fatalf("preview records: %v", err)
	}
	if _, err := os.Stat(artifact.ReportPath); err != nil {
		t.Fatalf("preview report: %v", err)
	}
}

func TestBuildPreviewUsesAvailablePassagesForShortDocument(t *testing.T) {
	datasetDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte("Channels communicate values between goroutines."), 0o600); err != nil {
		t.Fatalf("write Markdown: %v", err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"records":[{"prompt":"What do channels communicate?","evidence_id":1,"target_id":1}]}`},
		verificationCompletion(1, func(int) bool { return true }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatalf("new previewer: %v", err)
	}

	artifact, err := previewer.Build(context.Background(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		RecordCount:      5,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(artifact.Records) != 1 {
		t.Fatalf("records: got %d want 1", len(artifact.Records))
	}
	if len(model.requests) != 2 {
		t.Fatalf("requests: got %d want 2", len(model.requests))
	}
}

func TestBuildPreviewIgnoresHeadingsAndDuplicatePassages(t *testing.T) {
	datasetDirectory := t.TempDir()
	markdown := "# Concurrency\n\nChannels communicate values between goroutines.\n\nChannels communicate values between goroutines."
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte(markdown), 0o600); err != nil {
		t.Fatalf("write Markdown: %v", err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"records":[{"prompt":"What do channels communicate?","evidence_id":1,"target_id":1}]}`},
		verificationCompletion(1, func(int) bool { return true }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatalf("new previewer: %v", err)
	}

	artifact, err := previewer.Build(context.Background(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		RecordCount:      5,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(artifact.Records) != 1 || artifact.Records[0].Evidence != "Channels communicate values between goroutines." {
		t.Fatalf("records: %+v", artifact.Records)
	}
}

func TestBuildPreviewRejectsInvalidEvidenceID(t *testing.T) {
	datasetDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte("The source says one thing."), 0o600); err != nil {
		t.Fatalf("write Markdown: %v", err)
	}

	model := &completer{completion: prepare.Completion{
		Content: `{"records":[{"prompt":"What does it say?","evidence_id":99,"target_id":1}]}`,
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatalf("new previewer: %v", err)
	}

	_, err = previewer.Build(context.Background(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "Qwen3-8B-Q8_0",
		RecordCount:      1,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid evidence_id") {
		t.Fatalf("build: got %v", err)
	}
}

func TestBuildPreviewRetriesInvalidGeneration(t *testing.T) {
	datasetDirectory := t.TempDir()
	markdown := "First source passage.\n\nSecond source passage."
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte(markdown), 0o600); err != nil {
		t.Fatalf("write Markdown: %v", err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"records":[{"prompt":"First question?","evidence_id":1,"target_id":1},{"prompt":"Second question?","evidence_id":1,"target_id":1}]}`},
		{Content: `{"records":[{"prompt":"Second question?","evidence_id":1,"target_id":1}]}`},
		verificationCompletion(2, func(int) bool { return true }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatalf("new previewer: %v", err)
	}

	artifact, err := previewer.Build(context.Background(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "Qwen/Qwen3-8B-Q8_0",
		RecordCount:      2,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if artifact.GenerationAttempts != 2 || artifact.VerificationAttempts != 1 || len(model.requests) != 3 {
		t.Fatalf("attempts: artifact=%d requests=%d", artifact.GenerationAttempts, len(model.requests))
	}
	if len(model.requests[1].Messages) != 2 ||
		!strings.Contains(model.requests[1].Messages[1].Content, "reuses evidence_id") ||
		!strings.Contains(model.requests[1].Messages[1].Content, "Create exactly 1 training records") ||
		strings.Contains(model.requests[1].Messages[1].Content, "First source passage") {
		t.Fatalf("retry did not include validation feedback: %+v", model.requests[1].Messages)
	}
}

func TestBuildPreviewReplacesInvalidPercentageRecord(t *testing.T) {
	datasetDirectory := t.TempDir()
	markdown := "Go was created at Google.\n\nThe paper is at least 15 percent recycled."
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte(markdown), 0o600); err != nil {
		t.Fatalf("write Markdown: %v", err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"records":[{"prompt":"Where was Go created?","evidence_id":1,"target_id":1},{"prompt":"What percent of the paper is recycled?","evidence_id":2,"target_id":1}]}`},
		{Content: `{"records":[{"prompt":"What percent of the paper is recycled?","evidence_id":1,"target_id":2}]}`},
		verificationCompletion(2, func(int) bool { return true }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatalf("new previewer: %v", err)
	}

	artifact, err := previewer.Build(context.Background(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		RecordCount:      2,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(artifact.Records) != 2 || artifact.Records[1].Target != "15 percent" {
		t.Fatalf("records: %+v", artifact.Records)
	}
	if strings.Contains(model.requests[1].Messages[1].Content, "Go was created at Google") {
		t.Fatalf("replacement request reused accepted evidence: %s", model.requests[1].Messages[1].Content)
	}
}

func TestBuildPreviewRetriesStatementShapedQuestion(t *testing.T) {
	datasetDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte("Go is a programming language."), 0o600); err != nil {
		t.Fatalf("write Markdown: %v", err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"records":[{"prompt":"What is Go.","evidence_id":1,"target_id":1}]}`},
		{Content: `{"records":[{"prompt":"What is Go.","evidence_id":1,"target_id":1}]}`},
		{Content: `{"records":[{"prompt":"What is Go?","evidence_id":1,"target_id":1}]}`},
		verificationCompletion(1, func(int) bool { return true }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatalf("new previewer: %v", err)
	}

	artifact, err := previewer.Build(context.Background(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		RecordCount:      1,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if artifact.GenerationAttempts != 3 || artifact.Records[0].Prompt != "What is Go?" {
		t.Fatalf("artifact: %+v", artifact)
	}
	if !strings.Contains(model.requests[1].Messages[1].Content, "must be a question ending with a question mark") {
		t.Fatalf("retry did not explain the malformed question: %+v", model.requests[1].Messages)
	}
}

func TestBuildPreviewReturnsLocallyRejectedRowsForReview(t *testing.T) {
	datasetDirectory := t.TempDir()
	markdown := "Go is a programming language."
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte(markdown), 0o600); err != nil {
		t.Fatalf("write Markdown: %v", err)
	}

	completion := prepare.Completion{Content: `{"records":[{"prompt":"According to the source, what is Go?","evidence_id":1,"target_id":1}]}`}
	model := &sequenceCompleter{completions: []prepare.Completion{
		completion,
		completion,
		completion,
		verificationCompletion(1, func(int) bool { return true }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatalf("new previewer: %v", err)
	}

	artifact, err := previewer.Build(context.Background(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		RecordCount:      1,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(artifact.Records) != 1 || artifact.Records[0].Prompt != "According to the source, what is Go?" {
		t.Fatalf("reviewable records: %+v", artifact.Records)
	}
	if artifact.GenerationAttempts != 3 || len(model.requests) != 4 {
		t.Fatalf("strict retries were not bounded before review: artifact=%+v requests=%d", artifact, len(model.requests))
	}
}

func TestBuildPreviewReplacesSemanticFailure(t *testing.T) {
	datasetDirectory := t.TempDir()
	markdown := "Go was created at Google.\n\nChannels communicate values.\n\nA mutex protects shared state."
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte(markdown), 0o600); err != nil {
		t.Fatalf("write Markdown: %v", err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"records":[{"prompt":"Where was Go created?","evidence_id":1,"target_id":1},{"prompt":"What do channels protect?","evidence_id":2,"target_id":1}]}`},
		verificationCompletion(2, func(index int) bool { return index == 0 }),
		{Content: `{"records":[{"prompt":"What protects shared state?","evidence_id":1,"target_id":1}]}`},
		verificationCompletion(2, func(int) bool { return true }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatalf("new previewer: %v", err)
	}

	artifact, err := previewer.Build(context.Background(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "Qwen/Qwen3-8B-Q8_0",
		RecordCount:      2,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if artifact.GenerationAttempts != 2 || artifact.VerificationAttempts != 2 || len(model.requests) != 4 {
		t.Fatalf("attempts: artifact=%+v requests=%d", artifact, len(model.requests))
	}
	if artifact.Records[0].Prompt != "Where was Go created?" || artifact.Records[1].Prompt != "What protects shared state?" {
		t.Fatalf("records: got %+v", artifact.Records)
	}
	if strings.Contains(model.requests[2].Messages[1].Content, "Go was created at Google") ||
		strings.Contains(model.requests[2].Messages[1].Content, "Channels communicate values") ||
		!strings.Contains(model.requests[2].Messages[1].Content, "A mutex protects shared state") ||
		!strings.Contains(model.requests[2].Messages[1].Content, "do not form a useful standalone example") {
		t.Fatalf("replacement request did not contain only unused evidence and failure feedback")
	}
	for _, result := range artifact.Verification {
		if !result.Passed {
			t.Fatalf("verification: got %+v", artifact.Verification)
		}
	}
}

func TestBuildPreviewRegeneratesRecordRejectedForDatasetRequirements(t *testing.T) {
	datasetDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte("Go is a programming language."), 0o600); err != nil {
		t.Fatal(err)
	}

	const instructions = "Write longer questions that include relevant context."
	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"records":[{"prompt":"What is Go?","evidence_id":1,"target_id":1}]}`},
		verificationCompletion(1, func(int) bool { return false }),
		{Content: `{"records":[{"prompt":"In software development, how would you describe the role and purpose of Go?","evidence_id":1,"target_id":1}]}`},
		verificationCompletion(1, func(int) bool { return true }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.Build(t.Context(), prepare.PreviewRequest{
		DatasetDirectory:    datasetDirectory,
		Goal:                prepare.GoalQuestionAnswer,
		Model:               "model",
		RecordCount:         1,
		DatasetInstructions: instructions,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 1 || artifact.Records[0].Prompt != "In software development, how would you describe the role and purpose of Go?" {
		t.Fatalf("preview did not replace the rejected question: %+v", artifact.Records)
	}
	if len(model.requests) != 4 {
		t.Fatalf("model requests = %d, want generation, verification, replacement, and verification", len(model.requests))
	}
	for _, index := range []int{1, 3} {
		if !strings.HasPrefix(model.requests[index].Messages[1].Content, "USER INSTRUCTION — FOLLOW WHEN WRITING EVERY RECORD:") ||
			!strings.Contains(model.requests[index].Messages[1].Content, instructions) {
			t.Fatalf("verification request %d omitted user requirements: %s", index, model.requests[index].Messages[1].Content)
		}
	}
	if !strings.Contains(model.requests[2].Messages[1].Content, instructions) {
		t.Fatalf("replacement generation omitted user requirements: %s", model.requests[2].Messages[1].Content)
	}
}

func TestBuildPreviewBackfillsFourRejectedRowsWithoutReplacingSurvivor(t *testing.T) {
	datasetDirectory := t.TempDir()
	var source strings.Builder
	for fact := 1; fact <= 9; fact++ {
		fmt.Fprintf(&source, "Fact %d is supported by source evidence.\n\n", fact)
	}
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"records":[{"prompt":"What is fact one?","evidence_id":1,"target_id":1},{"prompt":"What is fact two?","evidence_id":2,"target_id":1},{"prompt":"What is fact three?","evidence_id":3,"target_id":1},{"prompt":"What is fact four?","evidence_id":4,"target_id":1},{"prompt":"What is fact five?","evidence_id":5,"target_id":1}]}`},
		verificationCompletion(5, func(index int) bool { return index == 0 }),
		{Content: `{"records":[{"prompt":"What is replacement fact six?","evidence_id":1,"target_id":1},{"prompt":"What is replacement fact seven?","evidence_id":2,"target_id":1},{"prompt":"What is replacement fact eight?","evidence_id":3,"target_id":1},{"prompt":"What is replacement fact nine?","evidence_id":4,"target_id":1}]}`},
		verificationCompletion(5, func(index int) bool { return index == 0 }),
		{Content: `{"records":[{"prompt":"How is replacement fact six stated?","evidence_id":1,"target_id":1},{"prompt":"How is replacement fact seven stated?","evidence_id":2,"target_id":1},{"prompt":"How is replacement fact eight stated?","evidence_id":3,"target_id":1},{"prompt":"How is replacement fact nine stated?","evidence_id":4,"target_id":1}]}`},
		verificationCompletion(5, func(int) bool { return true }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.Build(t.Context(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		RecordCount:      5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 5 || artifact.Records[0].Prompt != "What is fact one?" {
		t.Fatalf("preview did not preserve the survivor and backfill five rows: %+v", artifact.Records)
	}
	if artifact.GenerationAttempts != 3 || artifact.VerificationAttempts != 3 || len(model.requests) != 6 {
		t.Fatalf("unbounded preview repair: artifact=%+v requests=%d", artifact, len(model.requests))
	}
}

func TestBuildPreviewBackfillsRowsMissingFromInitialGeneration(t *testing.T) {
	datasetDirectory := t.TempDir()
	markdown := "Fact one is supported.\n\nFact two is supported.\n\nFact three is supported.\n\nFact four is supported.\n\nFact five is supported."
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte(markdown), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"records":[{"prompt":"What is fact one?","evidence_id":1,"target_id":1}]}`},
		{Content: `{"records":[{"prompt":"What is fact one?","evidence_id":1,"target_id":1}]}`},
		{Content: `{"records":[{"prompt":"What is fact one?","evidence_id":1,"target_id":1}]}`},
		verificationCompletion(1, func(int) bool { return true }),
		{Content: `{"records":[{"prompt":"What is fact two?","evidence_id":1,"target_id":1},{"prompt":"What is fact three?","evidence_id":2,"target_id":1},{"prompt":"What is fact four?","evidence_id":3,"target_id":1},{"prompt":"What is fact five?","evidence_id":4,"target_id":1}]}`},
		verificationCompletion(5, func(int) bool { return true }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.Build(t.Context(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		RecordCount:      5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 5 || artifact.Records[0].Prompt != "What is fact one?" {
		t.Fatalf("preview did not preserve and complete the partial generation: %+v", artifact.Records)
	}
	if artifact.GenerationAttempts != 4 || artifact.VerificationAttempts != 2 || len(model.requests) != 6 {
		t.Fatalf("unbounded missing-row repair: artifact=%+v requests=%d", artifact, len(model.requests))
	}
}

func TestBuildPreviewSelectsFiveFromOversampledRepairCandidates(t *testing.T) {
	datasetDirectory := t.TempDir()
	var source strings.Builder
	for fact := 1; fact <= 9; fact++ {
		fmt.Fprintf(&source, "Fact %d is supported by source evidence.\n\n", fact)
	}
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"records":[{"prompt":"What is fact one?","evidence_id":1,"target_id":1},{"prompt":"What is fact two?","evidence_id":2,"target_id":1},{"prompt":"What is fact three?","evidence_id":3,"target_id":1},{"prompt":"What is fact four?","evidence_id":4,"target_id":1},{"prompt":"What is fact five?","evidence_id":5,"target_id":1}]}`},
		verificationCompletion(5, func(index int) bool { return index < 3 }),
		{Content: `{"records":[{"prompt":"What is fact six?","evidence_id":1,"target_id":1},{"prompt":"What is fact seven?","evidence_id":2,"target_id":1},{"prompt":"What is fact eight?","evidence_id":3,"target_id":1},{"prompt":"What is fact nine?","evidence_id":4,"target_id":1}]}`},
		verificationCompletion(7, func(index int) bool { return index < 5 }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.Build(t.Context(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		RecordCount:      5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 5 {
		t.Fatalf("records = %d, want five selected from verified candidates", len(artifact.Records))
	}
	if artifact.GenerationAttempts != 2 || artifact.VerificationAttempts != 2 || len(model.requests) != 4 {
		t.Fatalf("oversampled repair was not bounded: artifact=%+v requests=%d", artifact, len(model.requests))
	}
	if !strings.Contains(model.requests[2].Messages[1].Content, "Create exactly 4 training records") {
		t.Fatalf("repair did not oversample two missing records")
	}
}

func TestBuildPreviewReturnsReviewableRowsAfterFinalSemanticRejection(t *testing.T) {
	datasetDirectory := t.TempDir()
	markdown := "Go was created at Google.\n\nChannels communicate values.\n\nA mutex protects shared state."
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte(markdown), 0o600); err != nil {
		t.Fatalf("write Markdown: %v", err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"records":[{"prompt":"Where was Go created?","evidence_id":1,"target_id":1},{"prompt":"What do channels protect?","evidence_id":2,"target_id":1}]}`},
		verificationCompletion(2, func(index int) bool { return index == 0 }),
		{Content: `{"records":[{"prompt":"What color is a mutex?","evidence_id":1,"target_id":1}]}`},
		verificationCompletion(2, func(index int) bool { return index == 0 }),
		{Content: `{"records":[{"prompt":"What shape is a mutex?","evidence_id":1,"target_id":1}]}`},
		verificationCompletion(2, func(index int) bool { return index == 0 }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatalf("new previewer: %v", err)
	}

	artifact, err := previewer.Build(context.Background(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		RecordCount:      2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 2 || artifact.Records[0].Prompt != "Where was Go created?" || artifact.Records[1].Prompt != "What shape is a mutex?" {
		t.Fatalf("preview rows = %+v", artifact.Records)
	}
	if len(artifact.Verification) != 2 || !artifact.Verification[0].Passed || artifact.Verification[1].Passed {
		t.Fatalf("verification = %+v", artifact.Verification)
	}
}

func TestBuildPreviewReusesRejectedEvidenceOnFinalRepair(t *testing.T) {
	datasetDirectory := t.TempDir()
	markdown := "Fact one is supported.\n\nFact two is supported.\n\nFact three is supported.\n\nFact four is supported.\n\nFact five is supported."
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte(markdown), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"records":[{"prompt":"What is fact one?","evidence_id":1,"target_id":1},{"prompt":"What is unsupported fact two?","evidence_id":2,"target_id":1}]}`},
		verificationCompletion(2, func(index int) bool { return index == 0 }),
		{Content: `{"records":[{"prompt":"What is unsupported fact three?","evidence_id":1,"target_id":1},{"prompt":"What is unsupported fact four?","evidence_id":2,"target_id":1}]}`},
		verificationCompletion(3, func(index int) bool { return index == 0 }),
		{Content: `{"records":[{"prompt":"What is fact three?","evidence_id":1,"target_id":1},{"prompt":"What is fact four?","evidence_id":2,"target_id":1}]}`},
		verificationCompletion(3, func(int) bool { return true }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := previewer.Build(t.Context(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		RecordCount:      2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Records) != 2 || artifact.Records[1].Prompt != "What is fact three?" {
		t.Fatalf("records: %+v", artifact.Records)
	}
	finalRepairPrompt := model.requests[4].Messages[1].Content
	if !strings.Contains(finalRepairPrompt, "Fact three is supported") || !strings.Contains(finalRepairPrompt, "Fact four is supported") {
		t.Fatalf("final repair did not reuse rejected evidence: %s", finalRepairPrompt)
	}
}

func TestBuildPreviewAllowsRejectedPromptWithCorrectedEvidence(t *testing.T) {
	datasetDirectory := t.TempDir()
	markdown := "Channels communicate values.\n\nGo is a programming language.\n\nGo was created at Google."
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte(markdown), 0o600); err != nil {
		t.Fatalf("write Markdown: %v", err)
	}

	model := &sequenceCompleter{completions: []prepare.Completion{
		{Content: `{"records":[{"prompt":"What do channels communicate?","evidence_id":1,"target_id":1},{"prompt":"Where was Go created?","evidence_id":2,"target_id":1}]}`},
		verificationCompletion(2, func(index int) bool { return index == 0 }),
		{Content: `{"records":[{"prompt":"Where was Go created?","evidence_id":1,"target_id":1}]}`},
		verificationCompletion(2, func(int) bool { return true }),
	}}
	previewer, err := prepare.NewPreviewer(model)
	if err != nil {
		t.Fatalf("new previewer: %v", err)
	}

	artifact, err := previewer.Build(context.Background(), prepare.PreviewRequest{
		DatasetDirectory: datasetDirectory,
		Goal:             prepare.GoalQuestionAnswer,
		Model:            "model",
		RecordCount:      2,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(artifact.Records) != 2 || artifact.Records[1].Prompt != "Where was Go created?" || artifact.Records[1].Target != "Go was created at Google." {
		t.Fatalf("corrected records: %+v", artifact.Records)
	}
	if len(model.requests) != 4 {
		t.Fatalf("requests = %d, want one generation, verification, replacement, and verification call", len(model.requests))
	}
	replacementPrompt := model.requests[2].Messages[1].Content
	if strings.Contains(replacementPrompt, "- Where was Go created?") || !strings.Contains(replacementPrompt, "- What do channels communicate?") {
		t.Fatalf("replacement prompt exclusions are incorrect: %s", replacementPrompt)
	}
}
