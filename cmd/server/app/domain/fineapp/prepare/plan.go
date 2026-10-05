package prepare

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

const (
	datasetPlanFile          = "dataset-plan.json"
	datasetPlanVersion       = 12
	analysisBatchChunks      = 12
	doclingChunkTargetWords  = 300
	doclingChunkMaximumWords = 450
	doclingChunkMinimumWords = 40
)

// SourceChunk is one stable, traceable unit of extracted document content.
type SourceChunk struct {
	ID                  string         `json:"id"`
	Ordinal             int            `json:"ordinal"`
	Pages               []int          `json:"pages,omitempty"`
	Text                string         `json:"text"`
	SHA256              string         `json:"sha256"`
	CandidatesAnalyzed  bool           `json:"candidates_analyzed,omitempty"`
	Candidates          []string       `json:"candidates,omitempty"`
	CandidateLabels     []string       `json:"candidate_labels,omitempty"`
	CandidateContexts   []string       `json:"candidate_contexts,omitempty"`
	CandidateRejections map[string]int `json:"candidate_rejections,omitempty"`
}

// ChunkAnalysis records how deterministic candidate cleanup and model
// selection treated one source chunk.
type ChunkAnalysis struct {
	SourceChunkID          string         `json:"source_chunk_id"`
	ExtractedCandidates    int            `json:"extracted_candidates"`
	EligibleCandidates     int            `json:"eligible_candidates"`
	SelectedCandidates     int            `json:"selected_candidates"`
	ModelOmittedCandidates int            `json:"model_omitted_candidates"`
	BatchGenerationCalls   int            `json:"batch_generation_calls"`
	CandidateRejections    map[string]int `json:"candidate_rejections,omitempty"`
	SelectionError         string         `json:"selection_error,omitempty"`
}

// PlannedFact is one grounded fact selected from a source chunk before any
// question variants are generated.
type PlannedFact struct {
	SourceChunkID string `json:"source_chunk_id"`
	Prompt        string `json:"prompt,omitempty"`
	Target        string `json:"target"`
	Evidence      string `json:"evidence"`
	FactID        string `json:"fact_id"`
}

// DatasetPlan fixes the complete set of source chunks and facts used by one
// resumable full-document dataset build.
type DatasetPlan struct {
	Version              int             `json:"version"`
	Fingerprint          string          `json:"fingerprint"`
	Chunks               []SourceChunk   `json:"chunks"`
	ChunkAnalysis        []ChunkAnalysis `json:"chunk_analysis,omitempty"`
	Facts                []PlannedFact   `json:"facts"`
	CompletedChunks      int             `json:"completed_chunks"`
	Complete             bool            `json:"complete"`
	ExcludedChunks       int             `json:"excluded_chunks"`
	SeedFacts            int             `json:"seed_facts"`
	RecordsPerFact       int             `json:"records_per_fact"`
	PlannedRecords       int             `json:"planned_records"`
	GenerationAttempts   int             `json:"generation_attempts"`
	VerificationAttempts int             `json:"verification_attempts"`
	PromptTokens         int             `json:"prompt_tokens"`
	CompletionTokens     int             `json:"completion_tokens"`
}

func (p *Previewer) buildQuestionAnswerPlan(ctx context.Context, request FullBuildRequest, markdown []byte, fingerprint string, seeds []Record) (DatasetPlan, previewMetrics, error) {
	plan, found := loadDatasetPlan(request.DatasetDirectory, fingerprint)
	if found && plan.Complete {
		if err := writeDatasetPlan(request.DatasetDirectory, plan); err != nil {
			return DatasetPlan{}, previewMetrics{}, err
		}
		if request.Progress != nil {
			request.Progress(FullBuildProgress{
				Phase: "analysis", Stage: "Source analysis complete",
				CompletedChunks: len(plan.Chunks), TotalChunks: len(plan.Chunks), AcceptedFacts: len(plan.Facts), PlannedRecords: plan.PlannedRecords,
			})
		}
		return plan, previewMetrics{
			generationAttempts: plan.GenerationAttempts, verificationAttempts: plan.VerificationAttempts,
			promptTokens: plan.PromptTokens, completionTokens: plan.CompletionTokens,
		}, nil
	}

	if !found {
		chunks, err := sourceChunks(string(markdown), request.DatasetDirectory)
		if err != nil {
			return DatasetPlan{}, previewMetrics{}, fmt.Errorf("plan dataset: %w", err)
		}
		plan = DatasetPlan{
			Version: datasetPlanVersion, Fingerprint: fingerprint,
			Chunks: chunks, SeedFacts: len(completeSeedFactIDs(seeds)), RecordsPerFact: questionAnswerRecordsPerFact,
		}
	}
	chunks := plan.Chunks
	exclusions := generationExclusions{examples: seeds, targets: recordTargets(seeds)}
	plannedTargets := make(map[string]bool, len(exclusions.targets))
	for _, target := range exclusions.targets {
		plannedTargets[normalizedTarget(target)] = true
	}
	for _, fact := range plan.Facts {
		plannedTargets[normalizedTarget(fact.Target)] = true
		exclusions.targets = append(exclusions.targets, fact.Target)
	}
	metrics := previewMetrics{
		generationAttempts: plan.GenerationAttempts, verificationAttempts: plan.VerificationAttempts,
		promptTokens: plan.PromptTokens, completionTokens: plan.CompletionTokens,
	}
	if found && request.Progress != nil {
		request.Progress(FullBuildProgress{
			Phase: "analysis", Stage: "Resuming source chunk analysis",
			CompletedChunks: plan.CompletedChunks, TotalChunks: len(chunks), AcceptedFacts: len(plan.Facts),
		})
	}
	for start := plan.CompletedChunks; start < len(chunks); start += analysisBatchChunks {
		if err := ctx.Err(); err != nil {
			return DatasetPlan{}, metrics, fmt.Errorf("plan dataset: %w", err)
		}
		end := min(start+analysisBatchChunks, len(chunks))
		batch := chunks[start:end]
		passages := make([]string, len(batch))
		for index, chunk := range batch {
			passages[index] = chunk.Text
		}
		targetCandidates, candidateContexts := sourceChunkCandidates(batch)
		prioritizeQuestionAnswerTargetsWithContexts(targetCandidates, candidateContexts, passages, exclusions.targets)
		attemptsBefore := metrics.generationAttempts
		selected, selectionMetrics, err := p.selectQuestionAnswerCandidatesWithContexts(ctx, request.Model, request.DatasetInstructions, passages, targetCandidates, candidateContexts, exclusions)
		metrics.add(selectionMetrics)
		if err != nil {
			plan.ChunkAnalysis = appendChunkAnalysis(plan.ChunkAnalysis, batch, targetCandidates, nil, metrics.generationAttempts-attemptsBefore, err)
			plan.GenerationAttempts = metrics.generationAttempts
			plan.PromptTokens = metrics.promptTokens
			plan.CompletionTokens = metrics.completionTokens
			if writeErr := writeDatasetPlan(request.DatasetDirectory, plan); writeErr != nil {
				return DatasetPlan{}, metrics, writeErr
			}
			return DatasetPlan{}, metrics, fmt.Errorf("plan dataset chunks %d-%d: %w", start+1, end, err)
		}
		selected = selectPlannedQuestionAnswerFacts(selected, passages)
		plan.ChunkAnalysis = appendChunkAnalysis(plan.ChunkAnalysis, batch, targetCandidates, selected, metrics.generationAttempts-attemptsBefore, nil)
		covered := make(map[string]bool, len(batch))
		for _, record := range selected {
			chunkIndex := slices.IndexFunc(batch, func(chunk SourceChunk) bool { return chunk.Text == record.Evidence })
			if chunkIndex < 0 {
				return DatasetPlan{}, metrics, fmt.Errorf("plan dataset: selected fact has no source chunk")
			}
			chunk := batch[chunkIndex]
			covered[chunk.ID] = true
			if plannedTargets[normalizedTarget(record.Target)] {
				continue
			}
			fact := PlannedFact{
				SourceChunkID: chunk.ID, Target: record.Target, Evidence: record.Evidence,
				FactID: record.FactID,
			}
			if slices.ContainsFunc(plan.Facts, func(existing PlannedFact) bool { return existing.FactID == fact.FactID }) {
				continue
			}
			plan.Facts = append(plan.Facts, fact)
			plannedTargets[normalizedTarget(fact.Target)] = true
			exclusions.targets = append(exclusions.targets, fact.Target)
		}
		plan.ExcludedChunks += len(batch) - len(covered)
		plan.CompletedChunks = end
		plan.PlannedRecords = plannedQuestionAnswerRecords(len(plan.Facts) + plan.SeedFacts)
		plan.GenerationAttempts = metrics.generationAttempts
		plan.VerificationAttempts = metrics.verificationAttempts
		plan.PromptTokens = metrics.promptTokens
		plan.CompletionTokens = metrics.completionTokens
		if err := writeDatasetPlan(request.DatasetDirectory, plan); err != nil {
			return DatasetPlan{}, metrics, err
		}
		if request.Progress != nil {
			request.Progress(FullBuildProgress{
				Phase: "analysis", Stage: "Analyzing source chunks",
				CompletedChunks: end, TotalChunks: len(chunks), AcceptedFacts: len(plan.Facts), PlannedRecords: plan.PlannedRecords,
			})
		}
	}
	if len(plan.Facts) == 0 && len(seeds) == 0 {
		return DatasetPlan{}, metrics, fmt.Errorf("plan dataset: no source chunk produced a grounded fact")
	}
	plan.PlannedRecords = plannedQuestionAnswerRecords(len(plan.Facts) + plan.SeedFacts)
	plan.GenerationAttempts = metrics.generationAttempts
	plan.VerificationAttempts = metrics.verificationAttempts
	plan.PromptTokens = metrics.promptTokens
	plan.CompletionTokens = metrics.completionTokens
	plan.Complete = true
	if err := writeDatasetPlan(request.DatasetDirectory, plan); err != nil {
		return DatasetPlan{}, metrics, err
	}
	return plan, metrics, nil
}

func selectPlannedQuestionAnswerFacts(selected []Record, passages []string) []Record {
	planned := make([]Record, 0, len(selected))
	for _, passage := range passages {
		var candidates []Record
		for _, record := range selected {
			if record.Evidence == passage && usefulPlannedQuestionAnswerTarget(record.Target) {
				candidates = append(candidates, record)
			}
		}
		planned = append(planned, collapseContainedPlannedTargets(candidates)...)
	}
	return planned
}

func plannedQuestionAnswerRecords(facts int) int {
	records := facts * questionAnswerRecordsPerFact
	if facts >= questionAnswerTestRecords {
		records += questionAnswerValidationRecords + questionAnswerTestRecords
	}
	return records
}

func usefulPlannedQuestionAnswerTarget(target string) bool {
	return plannedTargetRejectionReason(target) == ""
}

func plannedTargetRejectionReason(target string) string {
	target = strings.TrimSpace(target)
	if target == "" || onlyCodePunctuation(target) {
		return "fragment"
	}
	if strings.HasSuffix(target, "?") {
		return "source_question"
	}
	if broadCodeListing(target) {
		return "code_listing"
	}
	if presentationCandidate(target) || numberedHeading(target) {
		return "presentation"
	}
	if clippedProseFragment(target) {
		return "fragment"
	}
	if !usableQuestionAnswerTarget(target) {
		return "unresolved_context"
	}
	lower := strings.ToLower(strings.TrimSpace(target))
	for _, prefix := range []string{
		"in this chapter", "in this section", "in this lesson",
		"you will learn", "you'll learn", "you’ll learn",
		"you will see", "you'll see", "you’ll see",
		"as you will see", "as you'll see", "as you’ll see",
		"as you'll continue", "as you’ll continue",
		"here you see", "here are", "here is",
		"let's look", "let’s look", "let's take", "let’s take",
		"let's review", "let’s review", "now that you've", "now that you’ve",
		"the next listing", "the following listing",
		"in listing", "as you can see in listing", "when we run", "when you run", "if you run",
		"the rules for", "what's important", "what’s important", "in the source code",
		"all source code in", "with the runner type declared", "you'll understand", "you’ll understand",
		"table of contents", "copyright page",
	} {
		if strings.HasPrefix(lower, prefix) {
			return "presentation"
		}
	}
	if isDocumentNavigationTarget(target) {
		return "navigation"
	}
	return ""
}

func broadCodeListing(target string) bool {
	if len(codeLineNumberPattern.FindAllStringIndex(target, -1)) >= 2 {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(target))
	if strings.Contains(target, "//") || strings.Contains(target, "/*") || strings.Contains(target, ":=") ||
		strings.Contains(target, "{ ...") || strings.Contains(target, "}") ||
		strings.HasPrefix(lower, "func ") || strings.HasPrefix(lower, "package ") ||
		strings.HasPrefix(lower, "type ") {
		return true
	}
	if !leadingCodeLineNumberPattern.MatchString(target) {
		return false
	}
	return strings.ContainsAny(target, "{}();:=<>") ||
		strings.Contains(lower, "package ") ||
		strings.Contains(lower, "func ") ||
		strings.Contains(lower, "type ") ||
		strings.Contains(lower, "var ") ||
		strings.Contains(lower, "const ") ||
		strings.Contains(target, "//")
}

func clippedProseFragment(target string) bool {
	words := strings.Fields(target)
	if len(words) == 0 {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "my", "read more", "running.", "purposes.", "started":
		return true
	}
	if len(words) == 1 {
		word := strings.Trim(words[0], ".,;:!?()[]{}")
		return word == "" || (word == strings.ToLower(word) && strings.HasSuffix(target, "."))
	}
	if len(words) < 4 || strings.ContainsAny(target, "=<>/\\`\"") {
		return false
	}
	last := []rune(strings.TrimSpace(target))
	if len(last) > 0 && strings.ContainsRune(".!?", last[len(last)-1]) {
		return false
	}
	return true
}

func collapseContainedPlannedTargets(candidates []Record) []Record {
	collapsed := make([]Record, 0, len(candidates))
	for _, candidate := range candidates {
		candidateTarget := containedTarget(candidate.Target)
		contained := false
		for index, c := range slices.Backward(collapsed) {
			existingTarget := containedTarget(c.Target)
			switch {
			case strings.Contains(existingTarget, candidateTarget):
				contained = true
			case strings.Contains(candidateTarget, existingTarget):
				collapsed = slices.Delete(collapsed, index, index+1)
			}
		}
		if !contained {
			collapsed = append(collapsed, candidate)
		}
	}
	return collapsed
}

func containedTarget(target string) string {
	words := strings.FieldsFunc(normalizedTarget(target), func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r)
	})
	return " " + strings.Join(words, " ") + " "
}

func sourceChunkIDForEvidence(evidence string, chunks []SourceChunk) string {
	normalizedEvidence := normalizePlanText(evidence)
	for _, chunk := range chunks {
		normalizedChunk := normalizePlanText(chunk.Text)
		if normalizedEvidence == normalizedChunk || strings.Contains(normalizedChunk, normalizedEvidence) {
			return chunk.ID
		}
	}
	return ""
}

func completeSeedFactIDs(records []Record) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		if record.FactID != "" && !slices.Contains(ids, record.FactID) {
			ids = append(ids, record.FactID)
		}
	}
	return ids
}

func sourceChunks(markdown, directory string) ([]SourceChunk, error) {
	doclingPath := filepath.Join(directory, "extracted.docling.json")
	if _, err := os.Stat(doclingPath); err == nil {
		chunks, _, err := readDoclingSourceChunks(doclingPath)
		if err != nil {
			return nil, err
		}
		if len(chunks) == 0 {
			return nil, fmt.Errorf("docling extraction contains no substantive source content")
		}
		return chunks, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect Docling extraction: %w", err)
	}

	passages := fullPassages(markdown, nil)
	chunks := make([]SourceChunk, len(passages))
	for index, passage := range passages {
		chunks[index] = newSourceChunk(index, nil, passage)
	}
	return chunks, nil
}

type doclingProvenance struct {
	Page int `json:"page_no"`
}

type doclingTextBlock struct {
	SelfRef      string              `json:"self_ref"`
	LastSelfRef  string              `json:"-"`
	ContentLayer string              `json:"content_layer"`
	Label        string              `json:"label"`
	Text         string              `json:"text"`
	Provenance   []doclingProvenance `json:"prov"`
}

type doclingTableCell struct {
	Row  int    `json:"start_row_offset_idx"`
	Col  int    `json:"start_col_offset_idx"`
	Text string `json:"text"`
}

type doclingTable struct {
	SelfRef      string              `json:"self_ref"`
	ContentLayer string              `json:"content_layer"`
	Label        string              `json:"label"`
	Provenance   []doclingProvenance `json:"prov"`
	Data         struct {
		Cells []doclingTableCell `json:"table_cells"`
	} `json:"data"`
}

type doclingReference struct {
	Ref string `json:"$ref"`
}

type doclingGroup struct {
	SelfRef  string             `json:"self_ref"`
	Children []doclingReference `json:"children"`
}

type doclingPicture struct {
	SelfRef      string              `json:"self_ref"`
	ContentLayer string              `json:"content_layer"`
	Provenance   []doclingProvenance `json:"prov"`
	Children     []doclingReference  `json:"children"`
}

type doclingDocument struct {
	Texts    []doclingTextBlock `json:"texts"`
	Tables   []doclingTable     `json:"tables"`
	Pictures []doclingPicture   `json:"pictures"`
	Body     doclingGroup       `json:"body"`
	Groups   []doclingGroup     `json:"groups"`
}

type doclingCoverageLedger struct {
	Total            int
	Included         int
	Context          int
	Excluded         int
	ExclusionReasons map[string]int
}

type doclingDisposition uint8

const (
	doclingIncluded doclingDisposition = iota
	doclingContext
	doclingExcluded
)

type doclingChunkBlock struct {
	Page                int
	Pages               []int
	Order               int
	Label               string
	Text                string
	SectionBoundary     bool
	Candidates          []string
	CandidateContexts   []string
	CandidateRejections map[string]int
}

func doclingSourceChunks(directory string) []SourceChunk {
	chunks, _ := doclingSourceChunksWithCoverage(directory)
	return chunks
}

func doclingSourceChunksWithCoverage(directory string) ([]SourceChunk, doclingCoverageLedger) {
	chunks, ledger, _ := readDoclingSourceChunks(filepath.Join(directory, "extracted.docling.json"))
	return chunks, ledger
}

func readDoclingSourceChunks(path string) ([]SourceChunk, doclingCoverageLedger, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, doclingCoverageLedger{}, fmt.Errorf("read Docling extraction: %w", err)
	}
	var document doclingDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, doclingCoverageLedger{}, fmt.Errorf("decode Docling extraction: %w", err)
	}

	readingOrder := doclingReadingOrder(document.Body, document.Groups, document.Pictures, nonBodyDoclingReferences(document.Texts))
	excludedPages := doclingNavigationPages(document.Texts, document.Tables)
	ledger := newDoclingCoverageLedger(document, excludedPages)
	document.Texts = coalesceDoclingTextBlocks(document.Texts, readingOrder)
	blocks := make([]doclingChunkBlock, 0, len(document.Texts))
	section := ""
	listContext := ""
	for _, item := range document.Texts {
		page := firstDoclingPage(item.Provenance)
		if disposition, _ := doclingTextDisposition(item, excludedPages); disposition == doclingExcluded {
			continue
		}
		text := strings.TrimSpace(normalizeSourceText(item.Text))
		if text == "" {
			continue
		}
		candidateSection := section
		if item.Label == "section_header" {
			if !(strings.EqualFold(section, "Reading") && bibliographyEntry(text)) {
				section = text
				candidateSection = ""
			}
			listContext = ""
		}
		context := section
		if item.Label == "list_item" && listContext != "" {
			context = listContext
		}
		candidates, rejections := cleanDoclingCandidates(item.Label, candidateSection, text)
		contexts := make([]string, len(candidates))
		for index := range contexts {
			contexts[index] = context
		}
		blocks = append(blocks, doclingChunkBlock{
			Page: page, Pages: doclingPages(item.Provenance), Order: readingOrder[item.SelfRef], Label: item.Label, Text: text,
			SectionBoundary: item.Label == "section_header" && sectionBoundaryHeading(text), Candidates: candidates,
			CandidateContexts: contexts, CandidateRejections: rejections,
		})
		if item.Label == "text" {
			if strings.HasSuffix(text, ":") {
				listContext = text
			} else {
				listContext = ""
			}
		}
	}
	for _, table := range document.Tables {
		page := firstDoclingPage(table.Provenance)
		if disposition, _ := doclingTableDisposition(table, excludedPages); disposition == doclingExcluded {
			continue
		}
		if text := doclingTableText(table); text != "" {
			candidates := quoteCandidates(text)
			blocks = append(blocks, doclingChunkBlock{
				Page: page, Pages: doclingPages(table.Provenance), Order: readingOrder[table.SelfRef], Label: "table", Text: text,
				Candidates: candidates, CandidateContexts: make([]string, len(candidates)),
			})
		}
	}
	slices.SortStableFunc(blocks, func(left, right doclingChunkBlock) int {
		if left.Order > 0 && right.Order > 0 {
			return cmp.Compare(left.Order, right.Order)
		}
		return cmp.Compare(left.Page, right.Page)
	})
	return chunkDoclingBlocks(blocks), ledger, nil
}

func newDoclingCoverageLedger(document doclingDocument, excludedPages map[int]bool) doclingCoverageLedger {
	ledger := doclingCoverageLedger{
		Total:            len(document.Texts) + len(document.Tables) + len(document.Pictures),
		ExclusionReasons: make(map[string]int),
	}
	account := func(disposition doclingDisposition, reason string) {
		switch disposition {
		case doclingIncluded:
			ledger.Included++
		case doclingContext:
			ledger.Context++
		case doclingExcluded:
			ledger.Excluded++
			ledger.ExclusionReasons[reason]++
		}
	}
	for _, item := range document.Texts {
		account(doclingTextDisposition(item, excludedPages))
	}
	for _, table := range document.Tables {
		account(doclingTableDisposition(table, excludedPages))
	}
	for _, picture := range document.Pictures {
		reason := "picture_without_text"
		if len(picture.Children) > 0 {
			reason = "picture_container"
		}
		account(doclingExcluded, reason)
	}
	return ledger
}

func doclingTextDisposition(item doclingTextBlock, excludedPages map[int]bool) (doclingDisposition, string) {
	page := firstDoclingPage(item.Provenance)
	switch {
	case page == 0:
		return doclingExcluded, "missing_page"
	case excludedPages[page]:
		return doclingExcluded, "navigation"
	case item.ContentLayer != "body":
		return doclingExcluded, "non_body"
	case !usableDoclingLabel(item.Label):
		return doclingExcluded, "unsupported_label"
	case strings.TrimSpace(normalizeSourceText(item.Text)) == "":
		return doclingExcluded, "empty"
	case item.Label == "section_header" || item.Label == "caption" || item.Label == "text" && strings.HasSuffix(strings.TrimSpace(item.Text), ":"):
		return doclingContext, ""
	default:
		return doclingIncluded, ""
	}
}

func doclingTableDisposition(table doclingTable, excludedPages map[int]bool) (doclingDisposition, string) {
	page := firstDoclingPage(table.Provenance)
	switch {
	case page == 0:
		return doclingExcluded, "missing_page"
	case excludedPages[page] || table.Label == "document_index":
		return doclingExcluded, "navigation"
	case table.ContentLayer != "body":
		return doclingExcluded, "non_body"
	case doclingTableText(table) == "":
		return doclingExcluded, "empty"
	default:
		return doclingIncluded, ""
	}
}

func doclingTableText(table doclingTable) string {
	cells := slices.Clone(table.Data.Cells)
	slices.SortStableFunc(cells, func(left, right doclingTableCell) int {
		return cmp.Or(cmp.Compare(left.Row, right.Row), cmp.Compare(left.Col, right.Col))
	})
	var values []string
	for _, cell := range cells {
		if text := strings.TrimSpace(normalizeSourceText(cell.Text)); text != "" && !slices.Contains(values, text) {
			values = append(values, text)
		}
	}
	return strings.Join(values, " | ")
}

func nonBodyDoclingReferences(texts []doclingTextBlock) map[string]bool {
	references := make(map[string]bool)
	for _, item := range texts {
		if item.ContentLayer != "body" {
			references[item.SelfRef] = true
		}
	}
	return references
}

func doclingReadingOrder(body doclingGroup, groups []doclingGroup, pictures []doclingPicture, ignored map[string]bool) map[string]int {
	order := make(map[string]int)
	byReference := make(map[string]doclingGroup, len(groups))
	for _, group := range groups {
		byReference[group.SelfRef] = group
	}
	picturesByReference := make(map[string]doclingPicture, len(pictures))
	for _, picture := range pictures {
		picturesByReference[picture.SelfRef] = picture
	}
	var visit func([]doclingReference)
	visit = func(references []doclingReference) {
		for _, reference := range references {
			if ignored[reference.Ref] {
				continue
			}
			if group, exists := byReference[reference.Ref]; exists {
				visit(group.Children)
				continue
			}
			if picture, exists := picturesByReference[reference.Ref]; exists {
				order[reference.Ref] = len(order) + 1
				visit(picture.Children)
				continue
			}
			if _, exists := order[reference.Ref]; !exists {
				order[reference.Ref] = len(order) + 1
			}
		}
	}
	visit(body.Children)
	return order
}

func coalesceDoclingTextBlocks(items []doclingTextBlock, readingOrder map[string]int) []doclingTextBlock {
	joined := make([]doclingTextBlock, 0, len(items))
	for _, item := range items {
		item.LastSelfRef = item.SelfRef
		previousIndex := len(joined) - 1
		for previousIndex >= 0 && joined[previousIndex].ContentLayer != "body" {
			previousIndex--
		}
		if previousIndex >= 0 && joinsPreviousDoclingText(joined[previousIndex], item, readingOrder) {
			previous := &joined[previousIndex]
			previous.Text = strings.TrimSpace(previous.Text) + " " + strings.TrimSpace(item.Text)
			previous.LastSelfRef = item.SelfRef
			for _, provenance := range item.Provenance {
				if !slices.Contains(previous.Provenance, provenance) {
					previous.Provenance = append(previous.Provenance, provenance)
				}
			}
			continue
		}
		joined = append(joined, item)
	}
	return joined
}

func joinsPreviousDoclingText(previous, current doclingTextBlock, readingOrder map[string]int) bool {
	if previous.ContentLayer != "body" || current.ContentLayer != "body" ||
		previous.Label != "text" || current.Label != "text" {
		return false
	}
	previousOrder, previousOrdered := readingOrder[previous.LastSelfRef]
	currentOrder, currentOrdered := readingOrder[current.SelfRef]
	if previousOrdered != currentOrdered || previousOrdered && currentOrder != previousOrder+1 {
		return false
	}
	left := []rune(strings.TrimSpace(previous.Text))
	right := []rune(strings.TrimSpace(current.Text))
	if len(left) == 0 || len(right) == 0 || strings.ContainsRune(".!?:;)]}", left[len(left)-1]) {
		return false
	}
	return unicode.IsLower(right[0])
}

func sectionBoundaryHeading(heading string) bool {
	words := strings.Fields(strings.TrimSpace(heading))
	if len(words) == 0 {
		return false
	}
	if strings.EqualFold(words[0], "chapter") && len(words) > 1 {
		return true
	}
	first := strings.TrimRight(words[0], ".")
	if first == "" {
		return false
	}
	for _, char := range first {
		if !unicode.IsDigit(char) && char != '.' {
			return false
		}
	}
	return true
}

func doclingNavigationPages(texts []doclingTextBlock, tables []doclingTable) map[int]bool {
	excluded := make(map[int]bool)
	indexPage := 0
	for _, item := range texts {
		page := firstDoclingPage(item.Provenance)
		if page == 0 {
			continue
		}
		heading := strings.ToLower(strings.TrimSpace(normalizeSourceText(item.Text)))
		if (item.Label == "section_header" || item.Label == "page_header") && slices.Contains([]string{"brief contents", "contents", "table of contents"}, heading) {
			excluded[page] = true
		}
		if item.Label == "section_header" && slices.Contains([]string{"acknowledgements", "acknowledgments", "dedication", "dedications"}, heading) {
			excluded[page] = true
		}
		if (item.Label == "section_header" || item.Label == "page_header") && heading == "index" && (indexPage == 0 || page < indexPage) {
			indexPage = page
		}
		if item.Label == "section_header" && heading == "more titles from manning" {
			excluded[page] = true
		}
	}
	for _, table := range tables {
		if table.Label == "document_index" {
			excluded[firstDoclingPage(table.Provenance)] = true
		}
	}
	if indexPage > 0 {
		for _, item := range texts {
			if page := firstDoclingPage(item.Provenance); page >= indexPage {
				excluded[page] = true
			}
		}
	}
	return excluded
}

func firstDoclingPage(provenance []doclingProvenance) int {
	if len(provenance) == 0 {
		return 0
	}
	return provenance[0].Page
}

func doclingPages(provenance []doclingProvenance) []int {
	pages := make([]int, 0, len(provenance))
	for _, item := range provenance {
		if item.Page > 0 && !slices.Contains(pages, item.Page) {
			pages = append(pages, item.Page)
		}
	}
	return pages
}

func usableDoclingLabel(label string) bool {
	switch label {
	case "text", "list_item", "code", "caption", "section_header":
		return true
	default:
		return false
	}
}

func chunkDoclingBlocks(blocks []doclingChunkBlock) []SourceChunk {
	var chunks []SourceChunk
	var text strings.Builder
	var pages []int
	words := 0
	var candidates, labels, contexts []string
	rejections := make(map[string]int)
	substantive := false
	flush := func() {
		content := strings.TrimSpace(text.String())
		if content == "" {
			return
		}
		chunk := newSourceChunk(len(chunks), pages, content)
		chunk.CandidatesAnalyzed = true
		chunk.Candidates = slices.Clone(candidates)
		chunk.CandidateLabels = slices.Clone(labels)
		chunk.CandidateContexts = slices.Clone(contexts)
		chunk.CandidateRejections = cloneCounts(rejections)
		chunks = append(chunks, chunk)
		text.Reset()
		pages = nil
		words = 0
		candidates = nil
		labels = nil
		contexts = nil
		rejections = make(map[string]int)
		substantive = false
	}
	for _, block := range blocks {
		if block.SectionBoundary && substantive {
			flush()
		}
		parts := splitDoclingBlock(block.Text)
		for partIndex, part := range parts {
			partWords := len(strings.Fields(part))
			if words >= doclingChunkMinimumWords && words+partWords > doclingChunkTargetWords {
				flush()
			}
			if text.Len() > 0 {
				text.WriteString("\n\n")
			}
			text.WriteString(part)
			words += partWords
			for candidateIndex, candidate := range block.Candidates {
				if !strings.Contains(part, candidate) {
					continue
				}
				if slices.Contains(candidates, candidate) {
					rejections["duplicate"]++
					continue
				}
				candidates = append(candidates, candidate)
				labels = append(labels, block.Label)
				contexts = append(contexts, block.CandidateContexts[candidateIndex])
			}
			if partIndex == 0 {
				for reason, count := range block.CandidateRejections {
					rejections[reason] += count
				}
			}
			for _, page := range block.Pages {
				if !slices.Contains(pages, page) {
					pages = append(pages, page)
				}
			}
		}
		if block.Label != "section_header" && block.Label != "caption" {
			substantive = true
		}
	}
	flush()
	return chunks
}

func cleanDoclingCandidates(label, section, text string) ([]string, map[string]int) {
	raw := quoteCandidates(text)
	rejections := make(map[string]int)
	if label == "code" {
		raw = append(codeAtomicCandidates(text), textAtomicCandidates(text)...)
		rejections["code_listing"]++
	}
	if strings.EqualFold(strings.TrimSpace(section), "Reading") {
		rejections["bibliography"] = max(1, len(raw))
		return nil, rejections
	}
	if label == "section_header" {
		rejections["heading"] = max(1, len(raw))
		return nil, rejections
	}
	if label == "caption" {
		rejections["caption"] = max(1, len(raw))
		return nil, rejections
	}
	if label == "text" && strings.HasSuffix(strings.TrimSpace(text), ":") {
		rejections["list_context"] = max(1, len(raw))
		return nil, rejections
	}

	cleaned := make([]string, 0, len(raw))
	for _, candidate := range raw {
		if reason := plannedTargetRejectionReason(candidate); reason != "" {
			rejections[reason]++
			continue
		}
		if slices.Contains(cleaned, candidate) {
			rejections["duplicate"]++
			continue
		}
		cleaned = append(cleaned, candidate)
	}
	return cleaned, rejections
}

func presentationCandidate(candidate string) bool {
	words := strings.Fields(strings.TrimSpace(candidate))
	if len(words) == 0 {
		return true
	}
	first := strings.ToLower(strings.Trim(words[0], ".:()[]"))
	return first == "listing" || first == "figure"
}

func numberedHeading(candidate string) bool {
	words := strings.Fields(strings.TrimSpace(candidate))
	if len(words) < 2 || len(words) > 12 || strings.ContainsAny(candidate, "?!:") || strings.HasSuffix(candidate, ".") {
		return false
	}
	for _, r := range strings.Trim(words[0], ".") {
		if !unicode.IsDigit(r) && r != '.' {
			return false
		}
	}
	return true
}

func bibliographyEntry(text string) bool {
	return strings.Contains(text, " - ") || strings.Contains(text, " — ")
}

func cloneCounts(counts map[string]int) map[string]int {
	if len(counts) == 0 {
		return nil
	}
	cloned := make(map[string]int, len(counts))
	maps.Copy(cloned, counts)
	return cloned
}

func sourceChunkCandidates(chunks []SourceChunk) ([][]string, [][]string) {
	candidates := make([][]string, len(chunks))
	contexts := make([][]string, len(chunks))
	for index, chunk := range chunks {
		if chunk.CandidatesAnalyzed {
			candidates[index] = slices.Clone(chunk.Candidates)
			contexts[index] = slices.Clone(chunk.CandidateContexts)
			continue
		}
		candidates[index] = questionAnswerTargetCandidates([]string{chunk.Text})[0]
		contexts[index] = make([]string, len(candidates[index]))
	}
	return candidates, contexts
}

func appendChunkAnalysis(existing []ChunkAnalysis, chunks []SourceChunk, candidates [][]string, selected []Record, attempts int, selectionErr error) []ChunkAnalysis {
	chunkIDs := make(map[string]bool, len(chunks))
	for _, chunk := range chunks {
		chunkIDs[chunk.ID] = true
	}
	existing = slices.DeleteFunc(existing, func(analysis ChunkAnalysis) bool { return chunkIDs[analysis.SourceChunkID] })
	for index, chunk := range chunks {
		selectedCount := 0
		for _, record := range selected {
			if record.Evidence == chunk.Text {
				selectedCount++
			}
		}
		extracted := len(candidates[index])
		for _, count := range chunk.CandidateRejections {
			extracted += count
		}
		analysis := ChunkAnalysis{
			SourceChunkID: chunk.ID, ExtractedCandidates: extracted, EligibleCandidates: len(candidates[index]),
			SelectedCandidates: selectedCount, ModelOmittedCandidates: max(0, len(candidates[index])-selectedCount),
			BatchGenerationCalls: attempts, CandidateRejections: cloneCounts(chunk.CandidateRejections),
		}
		if selectionErr != nil {
			analysis.SelectionError = selectionErr.Error()
		}
		existing = append(existing, analysis)
	}
	return existing
}

func splitDoclingBlock(text string) []string {
	fields := strings.Fields(text)
	if len(fields) <= doclingChunkMaximumWords {
		return []string{text}
	}
	parts := make([]string, 0, (len(fields)+doclingChunkTargetWords-1)/doclingChunkTargetWords)
	for len(fields) > 0 {
		count := min(len(fields), doclingChunkTargetWords)
		parts = append(parts, strings.Join(fields[:count], " "))
		fields = fields[count:]
	}
	return parts
}

func newSourceChunk(index int, pages []int, text string) SourceChunk {
	digest := sha256.Sum256([]byte(text))
	return SourceChunk{
		ID: fmt.Sprintf("chunk-%04d-%x", index+1, digest[:4]), Ordinal: index + 1,
		Pages: slices.Clone(pages), Text: text, SHA256: fmt.Sprintf("%x", digest),
	}
}

func normalizePlanText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(normalizeSourceText(value)), " "))
}

func loadDatasetPlan(directory, fingerprint string) (DatasetPlan, bool) {
	data, err := os.ReadFile(filepath.Join(directory, datasetPlanFile))
	if err != nil {
		return DatasetPlan{}, false
	}
	var plan DatasetPlan
	if json.Unmarshal(data, &plan) != nil || plan.Fingerprint != fingerprint || plan.RecordsPerFact != questionAnswerRecordsPerFact || plan.CompletedChunks < 0 || plan.CompletedChunks > len(plan.Chunks) {
		return DatasetPlan{}, false
	}
	if plan.Version != datasetPlanVersion {
		return DatasetPlan{}, false
	}
	facts := len(plan.Facts)
	sanitizeDatasetPlan(&plan)
	if len(plan.Facts) != facts {
		return DatasetPlan{}, false
	}
	return plan, true
}

func sanitizeDatasetPlan(plan *DatasetPlan) {
	plan.Facts = slices.DeleteFunc(plan.Facts, func(fact PlannedFact) bool {
		reason := plannedTargetRejectionReason(fact.Target)
		if reason == "" {
			return false
		}
		for index := range plan.ChunkAnalysis {
			if plan.ChunkAnalysis[index].SourceChunkID != fact.SourceChunkID {
				continue
			}
			if plan.ChunkAnalysis[index].CandidateRejections == nil {
				plan.ChunkAnalysis[index].CandidateRejections = make(map[string]int)
			}
			plan.ChunkAnalysis[index].CandidateRejections[reason]++
			break
		}
		return true
	})
	covered := make(map[string]struct{}, len(plan.Facts))
	for _, fact := range plan.Facts {
		covered[fact.SourceChunkID] = struct{}{}
	}
	plan.ExcludedChunks = len(plan.Chunks) - len(covered)
	plan.PlannedRecords = plannedQuestionAnswerRecords(len(plan.Facts) + plan.SeedFacts)
}

func writeDatasetPlan(directory string, plan DatasetPlan) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dataset plan: %w", err)
	}
	if err := writePreviewReport(filepath.Join(directory, datasetPlanFile), append(data, '\n')); err != nil {
		return fmt.Errorf("write dataset plan: %w", err)
	}
	return nil
}
