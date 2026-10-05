package prepare

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type planCompleter struct {
	calls int
}

func (completer *planCompleter) Complete(_ context.Context, _ CompletionRequest) (Completion, error) {
	completer.calls++
	return Completion{Content: `{"target_ids":[[1,2]]}`, PromptTokens: 7, CompletionTokens: 3}, nil
}

type selectionCompleter struct {
	completions []Completion
	errs        []error
	requests    []CompletionRequest
}

type budgetCompleter struct {
	requests []CompletionRequest
}

func (completer *budgetCompleter) Complete(_ context.Context, request CompletionRequest) (Completion, error) {
	completer.requests = append(completer.requests, request)
	targetIDs := make([][]int, strings.Count(request.Messages[1].Content, "[PASSAGE "))
	for index := range targetIDs {
		targetIDs[index] = []int{1}
	}
	content, _ := json.Marshal(map[string]any{"target_ids": targetIDs})
	return Completion{Content: string(content)}, nil
}

func (completer *selectionCompleter) Complete(_ context.Context, request CompletionRequest) (Completion, error) {
	completer.requests = append(completer.requests, request)
	if index := len(completer.requests) - 1; index < len(completer.errs) && completer.errs[index] != nil {
		return Completion{}, completer.errs[index]
	}
	return completer.completions[len(completer.requests)-1], nil
}

func TestQuestionAnswerPlanContentScalesSelectedFactsAndCachesLineage(t *testing.T) {
	directory := t.TempDir()
	markdown := []byte("Alpha is the first supported fact. Beta is the second supported fact.")
	structured := `{"texts":[{"content_layer":"body","label":"text","text":"Alpha is the first supported fact.","prov":[{"page_no":2}]},{"content_layer":"body","label":"text","text":"Beta is the second supported fact.","prov":[{"page_no":3}]}]}`
	if err := os.WriteFile(filepath.Join(directory, "extracted.docling.json"), []byte(structured), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &planCompleter{}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	request := FullBuildRequest{DatasetDirectory: directory, Goal: GoalQuestionAnswer, Model: "model"}
	plan, metrics, err := previewer.buildQuestionAnswerPlan(t.Context(), request, markdown, "fingerprint", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Chunks) != 1 || len(plan.Facts) != 2 || plan.PlannedRecords != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Facts[0].SourceChunkID != plan.Chunks[0].ID {
		t.Fatalf("facts lost source lineage: %+v", plan.Facts)
	}
	if len(plan.Chunks[0].Pages) != 2 || plan.Chunks[0].Pages[0] != 2 || plan.Chunks[0].Pages[1] != 3 {
		t.Fatalf("chunk pages = %v, want [2 3]", plan.Chunks[0].Pages)
	}
	if metrics.generationAttempts != 1 || metrics.promptTokens != 7 || metrics.completionTokens != 3 {
		t.Fatalf("planning metrics = %+v", metrics)
	}

	cached, cachedMetrics, err := previewer.buildQuestionAnswerPlan(t.Context(), request, markdown, "fingerprint", nil)
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 1 || cached.PlannedRecords != plan.PlannedRecords || cachedMetrics != metrics {
		t.Fatalf("cached plan = %+v, metrics = %+v, calls = %d", cached, cachedMetrics, model.calls)
	}
}

func TestPlannedQuestionAnswerRecordsAddOnlyFixedEvaluationPrompts(t *testing.T) {
	if got := plannedQuestionAnswerRecords(questionAnswerTestRecords - 1); got != questionAnswerTestRecords-1 {
		t.Fatalf("records below evaluation threshold = %d, want %d", got, questionAnswerTestRecords-1)
	}
	want := questionAnswerTestRecords + questionAnswerValidationRecords + questionAnswerTestRecords
	if got := plannedQuestionAnswerRecords(questionAnswerTestRecords); got != want {
		t.Fatalf("records at evaluation threshold = %d, want %d", got, want)
	}
	if got := plannedQuestionAnswerRecords(2_000); got != 2_030 {
		t.Fatalf("records for 2,000 facts = %d, want 2,030", got)
	}
}

func TestSourceChunksHaveStableContentIdentity(t *testing.T) {
	markdown := "Alpha is stable.\n\nBeta is stable."
	first, err := sourceChunks(markdown, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := sourceChunks(markdown, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || len(first) != len(second) {
		t.Fatalf("chunk counts = %d and %d", len(first), len(second))
	}
	for index := range first {
		if first[index].ID != second[index].ID || first[index].SHA256 != second[index].SHA256 {
			t.Fatalf("chunk %d is unstable: %+v / %+v", index, first[index], second[index])
		}
	}
}

func TestSourceChunksUseDoclingStructureAndExcludeNavigation(t *testing.T) {
	directory := t.TempDir()
	document := map[string]any{
		"texts": []map[string]any{
			{"content_layer": "body", "label": "text", "text": strings.Repeat("Alpha content explains supported behavior. ", 45), "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "section_header", "text": "Contents", "prov": []map[string]any{{"page_no": 2}}},
			{"content_layer": "body", "label": "list_item", "text": "Concurrency 128 Channels 147 Testing 211", "prov": []map[string]any{{"page_no": 2}}},
			{"content_layer": "body", "label": "text", "text": strings.Repeat("Beta content describes another grounded behavior. ", 45), "prov": []map[string]any{{"page_no": 3}}},
			{"content_layer": "body", "label": "text", "text": "A search index maps terms to stored records without beginning the book's back matter.", "prov": []map[string]any{{"page_no": 3}}},
			{"content_layer": "furniture", "label": "page_footer", "text": "Licensed footer", "prov": []map[string]any{{"page_no": 3}}},
			{"content_layer": "body", "label": "section_header", "text": "Index", "prov": []map[string]any{{"page_no": 4}}},
			{"content_layer": "body", "label": "text", "text": "channels 147-153", "prov": []map[string]any{{"page_no": 5}}},
		},
		"tables": []map[string]any{{
			"content_layer": "body", "label": "document_index", "prov": []map[string]any{{"page_no": 2}},
			"data": map[string]any{"table_cells": []map[string]any{{"start_row_offset_idx": 0, "start_col_offset_idx": 0, "text": "Go type system 88"}}},
		}},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.docling.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	chunks, err := sourceChunks("flattened Markdown must not be used", directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks = %+v, want two body chunks", chunks)
	}
	if !slices.Equal(chunks[0].Pages, []int{1}) || !slices.Equal(chunks[1].Pages, []int{3}) {
		t.Fatalf("chunk pages = %v and %v", chunks[0].Pages, chunks[1].Pages)
	}
	for _, chunk := range chunks {
		if strings.Contains(chunk.Text, "Concurrency 128") || strings.Contains(chunk.Text, "Licensed footer") || strings.Contains(chunk.Text, "channels 147-153") {
			t.Fatalf("chunk retained navigation or furniture: %+v", chunk)
		}
	}
	if !strings.Contains(chunks[1].Text, "search index maps terms") {
		t.Fatalf("ordinary prose containing index was incorrectly excluded: %+v", chunks[1])
	}
}

func TestSourceChunksRejectInvalidDoclingInsteadOfFallingBackToMarkdown(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "extracted.docling.json"), []byte("not JSON"), 0o600); err != nil {
		t.Fatal(err)
	}

	chunks, err := sourceChunks("Fallback content must not hide extraction corruption.", directory)
	if err == nil || !strings.Contains(err.Error(), "decode Docling extraction") || len(chunks) != 0 {
		t.Fatalf("chunks = %+v, error = %v; want explicit Docling decode failure", chunks, err)
	}
}

func TestDoclingSourceChunksExcludeTableOfContentsPages(t *testing.T) {
	directory := t.TempDir()
	document := map[string]any{
		"texts": []map[string]any{
			{"content_layer": "body", "label": "section_header", "text": "Table of Contents", "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "text", "text": "Chapter 3 Data Structures 41", "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "section_header", "text": "Data Structures", "prov": []map[string]any{{"page_no": 2}}},
			{"content_layer": "body", "label": "text", "text": "Contiguous memory improves cache locality.", "prov": []map[string]any{{"page_no": 2}}},
		},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.docling.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	chunks := doclingSourceChunks(directory)
	if len(chunks) != 1 || slices.Contains(chunks[0].Pages, 1) || strings.Contains(chunks[0].Text, "Chapter 3 Data Structures 41") {
		t.Fatalf("table of contents remained in coverage chunks: %+v", chunks)
	}
}

func TestDoclingSourceChunksExcludeAcknowledgementsPages(t *testing.T) {
	directory := t.TempDir()
	document := map[string]any{
		"texts": []map[string]any{
			{"content_layer": "body", "label": "section_header", "text": "Acknowledgements", "prov": []map[string]any{{"page_no": 6}}},
			{"content_layer": "body", "label": "text", "text": "You are the most beautiful, kind, and smartest person I've known and loved.", "prov": []map[string]any{{"page_no": 6}}},
			{"content_layer": "body", "label": "section_header", "text": "Chapter 1: Runtime Behavior", "prov": []map[string]any{{"page_no": 7}}},
			{"content_layer": "body", "label": "text", "text": "Channels communicate typed values between goroutines.", "prov": []map[string]any{{"page_no": 7}}},
		},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.docling.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	chunks := doclingSourceChunks(directory)
	if len(chunks) != 1 || slices.Contains(chunks[0].Pages, 6) || strings.Contains(chunks[0].Text, "most beautiful") || !strings.Contains(chunks[0].Text, "Channels communicate") {
		t.Fatalf("acknowledgements remained in coverage chunks: %+v", chunks)
	}
}

func TestDoclingSourceChunksReconstructWrappedProseAndRespectSections(t *testing.T) {
	directory := t.TempDir()
	document := map[string]any{
		"texts": []map[string]any{
			{"self_ref": "#/texts/0", "content_layer": "body", "label": "section_header", "text": "1.1 Publishing", "prov": []map[string]any{{"page_no": 1}}},
			{"self_ref": "#/texts/1", "content_layer": "body", "label": "text", "text": "The publisher offers discounts when books are ordered in", "prov": []map[string]any{{"page_no": 1}}},
			{"self_ref": "#/texts/2", "content_layer": "furniture", "label": "page_footer", "text": "1", "prov": []map[string]any{{"page_no": 1}}},
			{"self_ref": "#/texts/3", "content_layer": "body", "label": "text", "text": "quantity for engineering teams.", "prov": []map[string]any{{"page_no": 2}}},
			{"self_ref": "#/texts/4", "content_layer": "body", "label": "section_header", "text": "1.2 Concurrency", "prov": []map[string]any{{"page_no": 3}}},
			{"self_ref": "#/texts/5", "content_layer": "body", "label": "text", "text": "Channels communicate typed values between goroutines.", "prov": []map[string]any{{"page_no": 3}}},
		},
		"body": map[string]any{"children": []map[string]any{
			{"$ref": "#/texts/0"}, {"$ref": "#/texts/1"}, {"$ref": "#/texts/2"}, {"$ref": "#/texts/3"}, {"$ref": "#/texts/4"}, {"$ref": "#/texts/5"},
		}},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.docling.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	chunks := doclingSourceChunks(directory)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %+v, want one coverage unit per substantive section", chunks)
	}
	if !strings.Contains(chunks[0].Text, "ordered in quantity") || strings.Contains(chunks[0].Text, "\n\n\n\n") {
		t.Fatalf("wrapped prose was not reconstructed cleanly: %q", chunks[0].Text)
	}
	if !slices.Equal(chunks[0].Pages, []int{1, 2}) {
		t.Fatalf("cross-page sentence lineage = %v, want pages 1 and 2", chunks[0].Pages)
	}
	if strings.Contains(chunks[0].Text, "Channels communicate") || !strings.Contains(chunks[1].Text, "Channels communicate") {
		t.Fatalf("section boundary mixed unrelated content: %+v", chunks)
	}
}

func TestDoclingSourceChunksPreserveBodyReadingOrderAroundTables(t *testing.T) {
	directory := t.TempDir()
	document := map[string]any{
		"texts": []map[string]any{
			{"self_ref": "#/texts/0", "content_layer": "body", "label": "section_header", "text": "2.1 Scheduling", "prov": []map[string]any{{"page_no": 3}}},
			{"self_ref": "#/texts/1", "content_layer": "body", "label": "text", "text": "Before the table.", "prov": []map[string]any{{"page_no": 3}}},
			{"self_ref": "#/texts/2", "content_layer": "body", "label": "text", "text": "Text extracted from the diagram.", "prov": []map[string]any{{"page_no": 3}}},
			{"self_ref": "#/texts/3", "content_layer": "body", "label": "text", "text": "After the diagram.", "prov": []map[string]any{{"page_no": 3}}},
		},
		"tables": []map[string]any{{
			"self_ref": "#/tables/0", "content_layer": "body", "label": "table", "prov": []map[string]any{{"page_no": 3}},
			"data": map[string]any{"table_cells": []map[string]any{{"start_row_offset_idx": 0, "start_col_offset_idx": 0, "text": "Worker"}, {"start_row_offset_idx": 0, "start_col_offset_idx": 1, "text": "Ready"}}},
		}},
		"pictures": []map[string]any{{
			"self_ref": "#/pictures/0", "content_layer": "body", "prov": []map[string]any{{"page_no": 3}},
			"children": []map[string]any{{"$ref": "#/texts/2"}},
		}},
		"body": map[string]any{"children": []map[string]any{
			{"$ref": "#/texts/0"}, {"$ref": "#/texts/1"}, {"$ref": "#/tables/0"}, {"$ref": "#/pictures/0"}, {"$ref": "#/texts/3"},
		}},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.docling.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	chunks, ledger := doclingSourceChunksWithCoverage(directory)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %+v, want one scheduling chunk", chunks)
	}
	if ledger.Total != 6 || ledger.Included != 4 || ledger.Context != 1 || ledger.Excluded != 1 ||
		ledger.Total != ledger.Included+ledger.Context+ledger.Excluded || ledger.ExclusionReasons["picture_container"] != 1 {
		t.Fatalf("coverage ledger did not conserve every source block: %+v", ledger)
	}
	before := strings.Index(chunks[0].Text, "Before the table.")
	table := strings.Index(chunks[0].Text, "Worker | Ready")
	pictureText := strings.Index(chunks[0].Text, "Text extracted from the diagram.")
	after := strings.Index(chunks[0].Text, "After the diagram.")
	if before < 0 || table < before || pictureText < table || after < pictureText {
		t.Fatalf("body reading order was not preserved: %q", chunks[0].Text)
	}
}

func TestDoclingCodeCandidatesExcludeWholeListings(t *testing.T) {
	code := `func main() { fmt.Println("hello") }`
	candidates, rejections := cleanDoclingCandidates("code", "Functions", code)
	if slices.Contains(candidates, code) {
		t.Fatalf("whole code listing remained selectable: %v", candidates)
	}
	if !slices.Contains(candidates, "main") {
		t.Fatalf("focused code symbol was not preserved: %v; rejections %v", candidates, rejections)
	}
}

func TestDoclingCandidatesPreserveListContextAndRejectConfirmedMetadata(t *testing.T) {
	directory := t.TempDir()
	document := map[string]any{
		"texts": []map[string]any{
			{"content_layer": "body", "label": "section_header", "text": "Interfaces", "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "text", "text": "Don't use an interface:", "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "list_item", "text": "Use a concrete type when callers do not need polymorphism.", "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "list_item", "text": "Use a concrete type when callers do not need polymorphism.", "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "section_header", "text": "Reading", "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "list_item", "text": "The Go Programming Language - Donovan and Kernighan", "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "section_header", "text": "Go in Action - Kennedy", "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "section_header", "text": "Chapter 2 Runtime Behavior", "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "text", "text": "Listing 4.1 Runtime sample", "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "text", "text": "2.3 Runtime details", "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "caption", "text": "Figure 2.3 Runtime diagram", "prov": []map[string]any{{"page_no": 1}}},
			{"content_layer": "body", "label": "text", "text": "SIGPROF", "prov": []map[string]any{{"page_no": 1}}},
		},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "extracted.docling.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	chunks := doclingSourceChunks(directory)
	if len(chunks) < 2 {
		t.Fatalf("chunks = %+v, want semantic section boundaries", chunks)
	}
	var candidates, labels, contexts []string
	rejections := make(map[string]int)
	for _, chunk := range chunks {
		candidates = append(candidates, chunk.Candidates...)
		labels = append(labels, chunk.CandidateLabels...)
		contexts = append(contexts, chunk.CandidateContexts...)
		for reason, count := range chunk.CandidateRejections {
			rejections[reason] += count
		}
	}
	if !slices.Contains(candidates, "SIGPROF") {
		t.Fatalf("short grounded target was removed: %+v", candidates)
	}
	target := "Use a concrete type when callers do not need polymorphism."
	index := slices.Index(candidates, target)
	if index < 0 || labels[index] != "list_item" || contexts[index] != "Don't use an interface:" {
		t.Fatalf("list candidate lineage = candidates %v, labels %v, contexts %v", candidates, labels, contexts)
	}
	for _, rejected := range []string{"Reading", "Go in Action - Kennedy", "Listing 4.1 Runtime sample", "2.3 Runtime details", "Figure 2.3 Runtime diagram", "Don't use an interface:"} {
		if slices.Contains(candidates, rejected) {
			t.Fatalf("metadata %q remained selectable: %v", rejected, candidates)
		}
	}
	for reason, minimum := range map[string]int{"heading": 3, "list_context": 1, "bibliography": 2, "presentation": 2, "caption": 1, "duplicate": 1} {
		if rejections[reason] < minimum {
			t.Fatalf("rejections[%q] = %d, want at least %d: %+v", reason, rejections[reason], minimum, rejections)
		}
	}
}

func TestQuestionAnswerTargetPromptRendersNonSelectableCandidateContext(t *testing.T) {
	prompt := questionAnswerTargetPrompt(
		[][]string{{"Use a concrete type when callers do not need polymorphism."}},
		nil,
		nil,
		"",
		[][]string{{"Don't use an interface:"}},
	)
	if !strings.Contains(prompt, "[CONTEXT FOR TARGET 1 — NOT SELECTABLE] Don't use an interface:") ||
		!strings.Contains(prompt, "[TARGET 1] Use a concrete type when callers do not need polymorphism.") {
		t.Fatalf("prompt did not distinguish context from target:\n%s", prompt)
	}
}

func TestAnalyzedDoclingChunkWithNoCandidatesDoesNotFallBackToFlattenedText(t *testing.T) {
	chunk := newSourceChunk(0, []int{1}, "Reading")
	chunk.CandidatesAnalyzed = true
	chunk.CandidateRejections = map[string]int{"heading": 1}
	candidates, contexts := sourceChunkCandidates([]SourceChunk{chunk})
	if len(candidates) != 1 || len(candidates[0]) != 0 || len(contexts[0]) != 0 {
		t.Fatalf("analyzed candidate-free chunk was rescanned: candidates %v, contexts %v", candidates, contexts)
	}
}

func TestChunkAnalysisPersistsCandidateSelectionDiagnostics(t *testing.T) {
	directory := t.TempDir()
	chunk := newSourceChunk(0, []int{7}, "SIGPROF interrupts the program. Runtime profiling uses sampled stacks.")
	chunk.CandidatesAnalyzed = true
	chunk.Candidates = []string{"SIGPROF", "Runtime profiling uses sampled stacks."}
	chunk.CandidateLabels = []string{"text", "text"}
	chunk.CandidateContexts = []string{"Runtime profiling", "Runtime profiling"}
	chunk.CandidateRejections = map[string]int{"heading": 1}
	selected := []Record{{Target: "SIGPROF", Evidence: chunk.Text}}
	analysis := appendChunkAnalysis(nil, []SourceChunk{chunk}, [][]string{chunk.Candidates}, selected, 3, nil)
	plan := DatasetPlan{
		Version: datasetPlanVersion, Fingerprint: "fingerprint", Chunks: []SourceChunk{chunk}, ChunkAnalysis: analysis,
		CompletedChunks: 1, Complete: true, RecordsPerFact: questionAnswerRecordsPerFact,
	}
	if err := writeDatasetPlan(directory, plan); err != nil {
		t.Fatal(err)
	}
	loaded, found := loadDatasetPlan(directory, "fingerprint")
	if !found || len(loaded.ChunkAnalysis) != 1 {
		t.Fatalf("loaded diagnostics = %+v, found %t", loaded.ChunkAnalysis, found)
	}
	got := loaded.ChunkAnalysis[0]
	if got.ExtractedCandidates != 3 || got.EligibleCandidates != 2 || got.SelectedCandidates != 1 || got.ModelOmittedCandidates != 1 || got.BatchGenerationCalls != 3 || got.CandidateRejections["heading"] != 1 {
		t.Fatalf("chunk diagnostics = %+v", got)
	}
}

func TestDatasetPlanRejectsPreviousPlannerVersion(t *testing.T) {
	directory := t.TempDir()
	stale := DatasetPlan{
		Version: datasetPlanVersion - 2, Fingerprint: "fingerprint",
		RecordsPerFact: questionAnswerRecordsPerFact, PlannedRecords: 23_445, Complete: true,
	}
	if err := writeDatasetPlan(directory, stale); err != nil {
		t.Fatal(err)
	}
	if plan, found := loadDatasetPlan(directory, "fingerprint"); found {
		t.Fatalf("loaded stale oversized plan: %+v", plan)
	}
}

func TestDatasetPlanRejectsCompletePlanWithSanitizedFacts(t *testing.T) {
	directory := t.TempDir()
	plan := DatasetPlan{
		Version: datasetPlanVersion, Fingerprint: "fingerprint", Complete: true, CompletedChunks: 1,
		Chunks: []SourceChunk{{ID: "content"}},
		Facts: []PlannedFact{
			{SourceChunkID: "content", Target: "Channels communicate typed values between goroutines.", FactID: "good"},
			{SourceChunkID: "content", Target: "In this chapter, channels are introduced.", FactID: "invalid"},
		},
		RecordsPerFact: questionAnswerRecordsPerFact, PlannedRecords: 2,
	}
	if err := writeDatasetPlan(directory, plan); err != nil {
		t.Fatal(err)
	}

	loaded, found := loadDatasetPlan(directory, "fingerprint")
	if found {
		t.Fatalf("loaded complete plan after sanitizing facts: %+v", loaded)
	}
}

func TestDatasetPlanRejectsPreviousChunkingVersion(t *testing.T) {
	directory := t.TempDir()
	plan := DatasetPlan{
		Version: datasetPlanVersion - 1, Fingerprint: "fingerprint", Complete: true, CompletedChunks: 2,
		Chunks: []SourceChunk{{ID: "navigation"}, {ID: "content"}},
		Facts: []PlannedFact{
			{SourceChunkID: "navigation", Target: "6.3 Race conditions 139", FactID: "bad"},
			{SourceChunkID: "content", Target: "Channels communicate typed values between goroutines.", FactID: "good"},
		},
		RecordsPerFact: questionAnswerRecordsPerFact, PlannedRecords: 15,
	}
	if err := writeDatasetPlan(directory, plan); err != nil {
		t.Fatal(err)
	}

	loaded, found := loadDatasetPlan(directory, "fingerprint")
	if found {
		t.Fatalf("loaded plan from previous chunking contract: %+v", loaded)
	}
}

func TestPlannedQuestionAnswerFactsRetainNonOverlappingFactsPerChunk(t *testing.T) {
	passage := strings.Repeat("source ", 300)
	selected := []Record{
		{Target: "The scheduler runs goroutines on operating system threads.", Evidence: passage},
		{Target: "operating system threads", Evidence: passage},
		{Target: "In this chapter, readers learn scheduling behavior.", Evidence: passage},
		{Target: "Channels communicate typed values between concurrent goroutines.", Evidence: passage},
		{Target: "A mutex protects shared state from concurrent access.", Evidence: passage},
		{Target: "The race detector reports unsynchronized memory access.", Evidence: passage},
		{Target: "Context propagates cancellation across API boundaries.", Evidence: passage},
	}

	planned := selectPlannedQuestionAnswerFacts(selected, []string{passage})
	if len(planned) != 5 {
		t.Fatalf("planned facts = %+v, want five distinct standalone facts from the chunk", planned)
	}
	for _, record := range planned {
		if record.Target == "operating system threads" || strings.HasPrefix(record.Target, "In this chapter") {
			t.Fatalf("planned facts retained fragment or presentation text: %+v", planned)
		}
	}
}

func TestPlannedQuestionAnswerFactsCollapseContainedVariants(t *testing.T) {
	passage := strings.Repeat("source ", doclingChunkTargetWords)
	selected := []Record{
		{Target: "SIGPROF interrupts the running program for CPU profiling.", Evidence: passage},
		{Target: "the running program for CPU profiling", Evidence: passage},
	}

	planned := selectPlannedQuestionAnswerFacts(selected, []string{passage})
	if len(planned) != 1 || planned[0].Target != selected[0].Target {
		t.Fatalf("collapsed facts = %+v", planned)
	}
}

func TestUsefulPlannedQuestionAnswerTargetRejectsFailedExtractionArtifacts(t *testing.T) {
	rejected := []string{
		"}",
		"my",
		"running.",
		"purposes.",
		"post was written by Yang Zhou; Sottware",
		"How much code can an engineer maintain in their head?",
		"The framework implements a default matcher that has no intelligence, and in the matchers",
		`01 package main 02 03 import ( 04 "log" 05 "os" 06 )`,
		"39 func incCounter(id int) { 40 // Schedule the call to Done to tell main we are done.",
		"func Retrieve() (config, error) { // ...",
		"Here you see the use of the keyword defer.",
		"The next listing will have our goroutines running in parallel.",
	}
	for _, target := range rejected {
		if usefulPlannedQuestionAnswerTarget(target) {
			t.Errorf("artifact %q remained eligible", target)
		}
	}

	accepted := []string{
		"SIGPROF",
		"25%",
		"--race",
		"GOMAXPROCS=2",
		"Channels carry typed values between goroutines.",
		"A for range loop terminates after the channel is closed and buffered values are received.",
	}
	for _, target := range accepted {
		if !usefulPlannedQuestionAnswerTarget(target) {
			t.Errorf("grounded target %q was rejected as %q", target, plannedTargetRejectionReason(target))
		}
	}
}

func TestDoclingCandidateCleanupReportsCodeListingsAndFragments(t *testing.T) {
	text := `01 package main 02 03 import ( 04 "fmt" 05 )

running.

Channels carry typed values between goroutines.`
	candidates, rejections := cleanDoclingCandidates("text", "Channels", text)
	if slices.Contains(candidates, `01 package main 02 03 import ( 04 "fmt" 05 )`) || slices.Contains(candidates, "running.") {
		t.Fatalf("cleanup retained extraction artifacts: %v", candidates)
	}
	if !slices.Contains(candidates, "Channels carry typed values between goroutines.") {
		t.Fatalf("cleanup removed instructional fact: %v", candidates)
	}
	if rejections["code_listing"] == 0 || rejections["fragment"] == 0 {
		t.Fatalf("cleanup diagnostics = %v, want code_listing and fragment", rejections)
	}
}
func TestPlannedQuestionAnswerFactsRejectMalformedTableOfContentsTargets(t *testing.T) {
	passage := "| 3.1 | Packaging and tooling 39 | Packages 40 | Go tooling 47 |"
	selected := []Record{
		{Target: "Atomic functions 142 ■ Mutexes 145", Evidence: passage},
		{Target: "between: Built-in types 96 Reference types 97 Struct types 98", Evidence: passage},
	}

	if planned := selectPlannedQuestionAnswerFacts(selected, []string{passage}); len(planned) != 0 {
		t.Fatalf("planned malformed navigation fact: %+v", planned)
	}
}

func TestQuestionAnswerPlanRetriesMalformedPassageSelections(t *testing.T) {
	model := &selectionCompleter{completions: []Completion{
		{Content: `{"target_ids":[]}`, PromptTokens: 5, CompletionTokens: 1},
		{Content: `{"target_ids":[[],[1]]}`, PromptTokens: 7, CompletionTokens: 2},
	}}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	passages := []string{
		"In this chapter, we will introduce the subject.",
		"The profiler uses SIGPROF to interrupt the program.",
	}
	records, metrics, err := previewer.selectQuestionAnswerFacts(t.Context(), "model", "", passages, generationExclusions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Target != "The profiler uses SIGPROF to interrupt the program." {
		t.Fatalf("records = %+v", records)
	}
	if metrics.generationAttempts != 2 || metrics.promptTokens != 12 || metrics.completionTokens != 3 {
		t.Fatalf("metrics = %+v", metrics)
	}
	if len(model.requests) != 2 || !strings.Contains(model.requests[1].Messages[1].Content, "exactly 2 arrays in passage order") {
		t.Fatalf("retry did not explain the passage-array contract: %+v", model.requests)
	}
}

func TestQuestionAnswerPlanRetriesTruncatedPassageSelection(t *testing.T) {
	model := &selectionCompleter{
		completions: []Completion{
			{},
			{Content: `{"target_ids":[[1]]}`},
		},
		errs: []error{ErrCompletionTruncated},
	}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	passage := "The profiler uses SIGPROF to interrupt the program."

	records, metrics, err := previewer.selectQuestionAnswerFacts(t.Context(), "model", "", []string{passage}, generationExclusions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || metrics.generationAttempts != 2 || len(model.requests) != 2 {
		t.Fatalf("records = %+v, metrics = %+v, requests = %d; want one record after one bounded retry", records, metrics, len(model.requests))
	}
}

func TestQuestionAnswerPlanRetriesOutOfRangeTargetWithExactRanges(t *testing.T) {
	model := &selectionCompleter{completions: []Completion{
		{Content: `{"target_ids":[[],[31]]}`},
		{Content: `{"target_ids":[[],[1]]}`},
	}}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	passages := []string{
		"In this chapter, we will introduce the subject.",
		"The profiler uses SIGPROF to interrupt the program.",
	}
	records, metrics, err := previewer.selectQuestionAnswerFacts(t.Context(), "model", "", passages, generationExclusions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || metrics.generationAttempts != 2 {
		t.Fatalf("selection = records %+v, metrics %+v", records, metrics)
	}
	correction := model.requests[1].Messages[1].Content
	if !strings.Contains(correction, "passage 1 = none, passage 2 = 1-2") || !strings.Contains(correction, "passage 2 has invalid target_id 31") {
		t.Fatalf("retry omitted exact target ranges: %s", correction)
	}
}

func TestQuestionAnswerPlanDoesNotSilentlyOmitEligiblePassage(t *testing.T) {
	passage := "Channels communicate typed values between goroutines."
	candidates := [][]string{{passage}}
	_, err := validateQuestionAnswerTargets([][]int{{}}, []string{passage}, candidates)
	if err == nil || !strings.Contains(err.Error(), "omitted all 1 eligible targets") {
		t.Fatalf("error = %v, want explicit eligible-target omission", err)
	}
}

func TestQuestionAnswerPlanRecoversRepeatedInvalidSingletonSelection(t *testing.T) {
	model := &selectionCompleter{}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	passage := "The profiler uses SIGPROF to interrupt the program."
	records, metrics, err := previewer.recoverQuestionAnswerTargets(t.Context(), "model", "", []string{passage}, [][]string{{passage}}, generationExclusions{}, [][]int{{31}})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Target != passage {
		t.Fatalf("recovered records = %+v", records)
	}
	if metrics.generationAttempts != 0 || len(model.requests) != 0 {
		t.Fatalf("bounded selection recovery: metrics %+v, requests %d", metrics, len(model.requests))
	}
}

func TestQuestionAnswerPlanIsolatesInvalidPassageSelection(t *testing.T) {
	model := &selectionCompleter{completions: []Completion{
		{Content: `{"target_ids":[[1],[34]]}`},
		{Content: `{"target_ids":[[1],[34]]}`},
		{Content: `{"target_ids":[[1],[34]]}`},
		{Content: `{"target_ids":[[1]]}`},
	}}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	passages := []string{
		"Go schedules goroutines onto operating system threads.",
		"Concurrency is not parallelism.",
	}
	records, metrics, err := previewer.selectQuestionAnswerFacts(t.Context(), "model", "", passages, generationExclusions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Evidence != passages[0] || records[1].Evidence != passages[1] {
		t.Fatalf("recovered records = %+v", records)
	}
	if metrics.generationAttempts != 4 || len(model.requests) != 4 {
		t.Fatalf("recovery metrics = %+v, requests = %d", metrics, len(model.requests))
	}
	if count := strings.Count(model.requests[3].Messages[1].Content, "[PASSAGE "); count != 1 {
		t.Fatalf("invalid passage was not isolated: %s", model.requests[3].Messages[1].Content)
	}
}

func TestQuestionAnswerPlanSkipsModelWhenNoCandidateExists(t *testing.T) {
	model := &selectionCompleter{}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	records, metrics, err := previewer.selectQuestionAnswerFacts(t.Context(), "model", "", []string{"In this chapter, we will introduce the subject."}, generationExclusions{})
	if err != nil || len(records) != 0 || metrics.generationAttempts != 0 || len(model.requests) != 0 {
		t.Fatalf("candidate-free selection = records %+v, metrics %+v, requests %d, err %v", records, metrics, len(model.requests), err)
	}
}

func TestQuestionAnswerPlanResumesAfterLastPersistedBatch(t *testing.T) {
	directory := t.TempDir()
	chunks := make([]SourceChunk, 6)
	for index := range chunks {
		chunks[index] = SourceChunk{ID: fmt.Sprintf("chunk-%04d", index+1), Ordinal: index + 1, Text: fmt.Sprintf("Supported fact number %d is documented.", index+1)}
	}
	partial := DatasetPlan{
		Version: datasetPlanVersion, Fingerprint: "fingerprint", Chunks: chunks,
		Facts:           []PlannedFact{{SourceChunkID: chunks[0].ID, Target: chunks[0].Text, Evidence: chunks[0].Text, FactID: "fact-1"}},
		CompletedChunks: 5, RecordsPerFact: questionAnswerRecordsPerFact, PlannedRecords: questionAnswerRecordsPerFact,
		GenerationAttempts: 1, PromptTokens: 10, CompletionTokens: 2,
	}
	if err := writeDatasetPlan(directory, partial); err != nil {
		t.Fatal(err)
	}
	model := &selectionCompleter{completions: []Completion{{Content: `{"target_ids":[[1]]}`, PromptTokens: 7, CompletionTokens: 3}}}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	request := FullBuildRequest{DatasetDirectory: directory, Goal: GoalQuestionAnswer, Model: "model"}
	plan, metrics, err := previewer.buildQuestionAnswerPlan(t.Context(), request, []byte("unused because chunks were persisted"), "fingerprint", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Complete || plan.CompletedChunks != 6 || len(plan.Facts) != 2 || len(model.requests) != 1 {
		t.Fatalf("resumed plan = %+v, requests = %d", plan, len(model.requests))
	}
	if metrics.generationAttempts != 2 || metrics.promptTokens != 17 || metrics.completionTokens != 5 {
		t.Fatalf("resumed metrics = %+v", metrics)
	}
}

func TestQuestionAnswerPlanSplitsOversizedPromptsWithoutDroppingPassages(t *testing.T) {
	passages := make([]string, 5)
	for passage := range passages {
		var text strings.Builder
		for fact := range 50 {
			fmt.Fprintf(&text, "Document %d fact %d describes supported behavior in complete detail. ", passage+1, fact+1)
		}
		passages[passage] = text.String()
	}
	model := &budgetCompleter{}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	records, _, err := previewer.selectQuestionAnswerFacts(t.Context(), "model", "", passages, generationExclusions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(model.requests) < 2 {
		t.Fatalf("oversized prompt was not split: %d request", len(model.requests))
	}
	for index, request := range model.requests {
		if size := len(request.Messages[1].Content); size > maxQuestionAnswerPromptBytes {
			t.Fatalf("request %d size = %d, budget = %d", index+1, size, maxQuestionAnswerPromptBytes)
		}
	}
	for index, passage := range passages {
		if !slices.ContainsFunc(records, func(record Record) bool { return record.Evidence == passage }) {
			t.Fatalf("passage %d was dropped during prompt splitting", index+1)
		}
	}
}
