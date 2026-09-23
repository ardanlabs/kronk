package prepare

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	fullBatchPassages         = 5
	fullEvidenceRunes         = 3000
	buildCheckpointFile       = ".dataset-build.json"
	datasetSpareFile          = "dataset-spares.jsonl"
	datasetContract           = 45
	questionCandidatePoolSize = 10
	maximumTargetSimilarity   = 0.20
)

type buildCheckpoint struct {
	Version              int      `json:"version"`
	Fingerprint          string   `json:"fingerprint"`
	CompletedBatches     int      `json:"completed_batches"`
	SpareBatchCompleted  bool     `json:"spare_batch_completed"`
	Records              []Record `json:"records"`
	ExcludedEvidence     int      `json:"excluded_evidence"`
	GenerationAttempts   int      `json:"generation_attempts"`
	VerificationAttempts int      `json:"verification_attempts"`
	PromptTokens         int      `json:"prompt_tokens"`
	CompletionTokens     int      `json:"completion_tokens"`
}

// FullBuildRequest selects an extraction and reviewed records for a complete
// canonical dataset build.
type FullBuildRequest struct {
	DatasetDirectory           string
	Goal                       Goal
	Model                      string
	DatasetInstructions        string
	ReviewedRecords            []Record
	VerifiedPreviewRecords     []Record
	MinimumQuestionAnswerFacts int
	Progress                   func(FullBuildProgress)
}

// FullBuildProgress reports durable units of work completed by a full build.
type FullBuildProgress struct {
	CompletedBatches int
	TotalBatches     int
	AcceptedRecords  int
	AcceptedFacts    int
	CompletedUnits   int
	TotalUnits       int
	Stage            string
}

// FullBuildArtifact describes a persisted complete canonical dataset.
type FullBuildArtifact struct {
	Records              []Record
	SpareRecords         []Record
	RecordsPath          string
	SpareRecordsPath     string
	ReportPath           string
	ReviewedRecordCount  int
	GeneratedRecordCount int
	GenerationAttempts   int
	VerificationAttempts int
	PromptTokens         int
	CompletionTokens     int
	ExcludedEvidence     int
}

// FullBuildDocument is one independently prepared source included in a
// combined training dataset.
type FullBuildDocument struct {
	ID       string
	Name     string
	Artifact FullBuildArtifact
}

// FullBuildMergeRequest describes the independently prepared sources that
// should become one training dataset.
type FullBuildMergeRequest struct {
	DatasetDirectory    string
	Goal                Goal
	Model               string
	DatasetInstructions string
	Documents           []FullBuildDocument
}

// FullBuildDocumentReport records one source's contribution to a combined build.
type FullBuildDocumentReport struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Facts   int    `json:"facts"`
	Records int    `json:"records"`
}

// FullBuildReport records the provenance and model cost of a complete build.
type FullBuildReport struct {
	Version              int                       `json:"version"`
	CompletedAt          time.Time                 `json:"completed_at"`
	Goal                 Goal                      `json:"goal"`
	Model                string                    `json:"model"`
	DatasetInstructions  string                    `json:"dataset_instructions,omitempty"`
	TotalRecords         int                       `json:"total_records"`
	ReviewedSeedInput    int                       `json:"reviewed_seed_input_count"`
	ReviewedSeedCount    int                       `json:"reviewed_seed_count"`
	GeneratedCount       int                       `json:"generated_count"`
	GenerationAttempts   int                       `json:"generation_attempts"`
	VerificationAttempts int                       `json:"verification_attempts"`
	PromptTokens         int                       `json:"prompt_tokens"`
	CompletionTokens     int                       `json:"completion_tokens"`
	ExcludedEvidence     int                       `json:"excluded_evidence"`
	SpareFacts           int                       `json:"spare_facts"`
	Documents            []FullBuildDocumentReport `json:"documents,omitempty"`
}

// BuildFull creates, independently verifies, and atomically persists a
// canonical dataset from the complete extracted Markdown document.
func (p *Previewer) BuildFull(ctx context.Context, request FullBuildRequest) (FullBuildArtifact, error) {
	if request.DatasetDirectory == "" {
		return FullBuildArtifact{}, fmt.Errorf("build dataset: dataset directory is required")
	}
	if request.Model == "" {
		return FullBuildArtifact{}, fmt.Errorf("build dataset: preparation model is required")
	}
	if !request.Goal.valid() {
		return FullBuildArtifact{}, fmt.Errorf("build dataset: unsupported goal %q", request.Goal)
	}
	instructions, err := normalizeDatasetInstructions(request.DatasetInstructions)
	if err != nil {
		return FullBuildArtifact{}, fmt.Errorf("build dataset: %w", err)
	}
	request.DatasetInstructions = instructions
	if err := ctx.Err(); err != nil {
		return FullBuildArtifact{}, fmt.Errorf("build dataset: %w", err)
	}

	seeds, err := ValidateReviewedRecords(request.ReviewedRecords)
	if err != nil {
		return FullBuildArtifact{}, fmt.Errorf("build dataset: invalid reviewed records: %w", err)
	}
	verifiedPreview, err := ValidateReviewedRecords(request.VerifiedPreviewRecords)
	if err != nil {
		return FullBuildArtifact{}, fmt.Errorf("build dataset: invalid verified preview records: %w", err)
	}
	reviewedSeedInput := len(seeds)
	previewAligned := len(seeds) == len(verifiedPreview)
	usableSeeds := make([]Record, 0, len(seeds))
	for index, record := range seeds {
		qualityErr := validateRecordShape(request.Goal, record.Prompt, record.Target)
		if qualityErr == nil && request.Goal == GoalQuestionAnswer {
			qualityErr = validateNaturalQuestion(record.Prompt)
		}
		if qualityErr != nil {
			unchanged := previewAligned && sameReviewedRecord(record, verifiedPreview[index])
			if unchanged {
				continue
			}
			return FullBuildArtifact{}, fmt.Errorf("build dataset: invalid reviewed record %d: %w", index+1, qualityErr)
		}
		if request.Goal == GoalQuestionAnswer {
			record.FactID = recordFactID(record.Evidence, record.Target)
		}
		usableSeeds = append(usableSeeds, record)
	}
	seeds = usableSeeds
	reviewedToVerify := seeds
	reviewedIndexes := make([]int, len(seeds))
	for index := range reviewedIndexes {
		reviewedIndexes[index] = index
	}
	markdown, err := os.ReadFile(filepath.Join(request.DatasetDirectory, "extracted.md"))
	if err != nil {
		return FullBuildArtifact{}, fmt.Errorf("read extracted Markdown: %w", err)
	}

	passages := fullPassages(string(markdown), seeds)
	if request.Goal == GoalQuestionAnswer {
		passages = prioritizeFullPassages(passages, request.MinimumQuestionAnswerFacts)
	}
	batches := fullPassageBatches(passages)
	records := append([]Record(nil), seeds...)
	if len(records) == 0 && len(batches) == 0 {
		return FullBuildArtifact{}, fmt.Errorf("build dataset: extracted document has no substantive passages")
	}
	var metrics previewMetrics
	var excludedEvidence, completedBatches int
	var spareBatchCompleted bool
	fingerprint, err := buildFingerprint(markdown, request, seeds)
	if err != nil {
		return FullBuildArtifact{}, err
	}
	checkpoint, checkpointFound := loadBuildCheckpoint(request.DatasetDirectory, fingerprint, len(batches))
	if checkpointFound {
		records = checkpoint.Records
		completedBatches = checkpoint.CompletedBatches
		spareBatchCompleted = checkpoint.SpareBatchCompleted
		excludedEvidence = checkpoint.ExcludedEvidence
		metrics = previewMetrics{
			generationAttempts: checkpoint.GenerationAttempts, verificationAttempts: checkpoint.VerificationAttempts,
			promptTokens: checkpoint.PromptTokens, completionTokens: checkpoint.CompletionTokens,
		}
	}
	// Keep one finalization unit outstanding so 100% means the dataset artifacts
	// have actually been published, not merely that the last batch completed.
	questionAnswerProgress := request.Goal == GoalQuestionAnswer && request.MinimumQuestionAnswerFacts > 0
	totalUnits := len(batches)*2 + 1
	completedUnits := completedBatches * 2
	if questionAnswerProgress {
		totalUnits = request.MinimumQuestionAnswerFacts + 1
		completedUnits = min(questionAnswerFactCount(records), request.MinimumQuestionAnswerFacts)
	}
	reportProgress := func(stage string, units int) {
		if request.Progress == nil {
			return
		}
		request.Progress(FullBuildProgress{
			CompletedBatches: completedBatches,
			TotalBatches:     len(batches),
			AcceptedRecords:  len(records),
			AcceptedFacts:    questionAnswerFactCount(records),
			CompletedUnits:   units,
			TotalUnits:       totalUnits,
			Stage:            stage,
		})
	}
	if len(seeds) > 0 && !checkpointFound {
		if !questionAnswerProgress && len(reviewedToVerify) > 0 {
			totalUnits++
		}
		previewStage := "Checking reviewed preview"
		reportProgress(previewStage, completedUnits)
		if len(reviewedToVerify) > 0 {
			verification, verificationMetrics, err := p.verifyRecords(ctx, request.Model, request.Goal, request.DatasetInstructions, reviewedToVerify)
			metrics.add(verificationMetrics)
			if err != nil {
				return FullBuildArtifact{}, fmt.Errorf("build dataset: verify reviewed preview: %w", err)
			}
			if failed := failedVerification(verification); len(failed) > 0 {
				recordNumber := reviewedIndexes[failed[0].RecordNumber-1] + 1
				return FullBuildArtifact{}, fmt.Errorf("build dataset: reviewed preview record %d is not supported by its prompt, evidence, and dataset requirements: %s", recordNumber, failed[0].Reason)
			}
		}
		if request.Goal == GoalQuestionAnswer {
			reportProgress("Creating question pools for reviewed facts", completedUnits)
			exclusions := generationExclusions{prompts: recordPrompts(records)}
			expanded, expansionMetrics, expansionErr := p.expandQuestionAnswerRecords(ctx, request.Model, request.DatasetInstructions, records, exclusions)
			metrics.add(expansionMetrics)
			if expansionErr != nil {
				if !isRecordQualityError(expansionErr) {
					return FullBuildArtifact{}, fmt.Errorf("build dataset: create question pools for reviewed preview: %w", expansionErr)
				}
				records = nil
			} else {
				reportProgress("Verifying reviewed question pools", completedUnits)
				var repairMetrics previewMetrics
				var repairErr error
				records, repairMetrics, repairErr = p.verifyQuestionAnswerRecords(ctx, request.Model, request.DatasetInstructions, seeds, expanded)
				metrics.add(repairMetrics)
				if repairErr != nil {
					return FullBuildArtifact{}, repairErr
				}
			}
		}
		if questionAnswerProgress {
			completedUnits = min(questionAnswerFactCount(records), request.MinimumQuestionAnswerFacts)
		} else if len(reviewedToVerify) > 0 {
			completedUnits++
		}
		if err := saveBuildCheckpoint(request.DatasetDirectory, buildCheckpointFrom(fingerprint, 0, spareBatchCompleted, records, excludedEvidence, metrics)); err != nil {
			return FullBuildArtifact{}, err
		}
	} else if len(seeds) > 0 {
		if questionAnswerProgress {
			completedUnits = min(questionAnswerFactCount(records), request.MinimumQuestionAnswerFacts)
		} else {
			totalUnits++
			completedUnits++
		}
	}
	exclusions := generationExclusions{prompts: recordPrompts(records), examples: seeds, targets: recordTargets(records)}

	for index := completedBatches; index < len(batches); index++ {
		hasRequiredFacts := questionAnswerProgress && questionAnswerFactCount(records) >= request.MinimumQuestionAnswerFacts
		if hasRequiredFacts && spareBatchCompleted {
			break
		}
		if err := ctx.Err(); err != nil {
			return FullBuildArtifact{}, fmt.Errorf("build dataset: %w", err)
		}
		reportProgress(fmt.Sprintf("Generating batch %d of %d", index+1, len(batches)), completedUnits)
		generated, skipped, batchMetrics, err := p.prepareFullBatch(ctx, request, batches[index], exclusions, func(stage string) {
			activeUnits := completedUnits
			if !questionAnswerProgress {
				activeUnits++
			}
			reportProgress(stage, activeUnits)
		})
		metrics.add(batchMetrics)
		if err != nil {
			if hasRequiredFacts && ctx.Err() == nil {
				reportProgress("Using verified facts; optional spare batch was unavailable", completedUnits)
				break
			}
			return FullBuildArtifact{}, fmt.Errorf("build dataset batch %d of %d: %w", index+1, len(batches), err)
		}
		if !questionAnswerProgress {
			completedUnits += 2
		}
		if err := validateNewRecords(records, generated); err != nil {
			if hasRequiredFacts {
				reportProgress("Using verified facts; optional spare batch conflicted with existing facts", completedUnits)
				break
			}
			return FullBuildArtifact{}, fmt.Errorf("build dataset batch %d of %d: %w", index+1, len(batches), err)
		}
		records = append(records, generated...)
		if hasRequiredFacts {
			spareBatchCompleted = true
		}
		if questionAnswerProgress {
			completedUnits = min(questionAnswerFactCount(records), request.MinimumQuestionAnswerFacts)
		}
		completedBatches = index + 1
		excludedEvidence += skipped
		exclusions.prompts = recordPrompts(records)
		exclusions.targets = recordTargets(records)
		if err := saveBuildCheckpoint(request.DatasetDirectory, buildCheckpointFrom(fingerprint, index+1, spareBatchCompleted, records, excludedEvidence, metrics)); err != nil {
			return FullBuildArtifact{}, err
		}
		reportProgress(fmt.Sprintf("Completed batch %d of %d", index+1, len(batches)), completedUnits)
	}
	if err := ctx.Err(); err != nil {
		return FullBuildArtifact{}, fmt.Errorf("build dataset: %w", err)
	}
	if len(records) == 0 {
		return FullBuildArtifact{}, fmt.Errorf("build dataset: no source passage produced a verified training record")
	}
	reviewedRecordCount := len(seeds)
	var spareRecords []Record
	if request.Goal == GoalQuestionAnswer {
		records = completeQuestionAnswerVariants(records)
		if request.MinimumQuestionAnswerFacts > 0 {
			records, spareRecords = partitionQuestionAnswerFacts(records, request.MinimumQuestionAnswerFacts)
		}
		reviewedRecordCount = 0
		for _, seed := range seeds {
			if slices.ContainsFunc(records, func(record Record) bool {
				return record.Prompt == seed.Prompt && record.FactID == seed.FactID
			}) {
				reviewedRecordCount++
			}
		}
		if len(records) == 0 {
			return FullBuildArtifact{}, fmt.Errorf("build dataset: no source fact produced %d verified training questions", questionAnswerRequiredVariants)
		}
	}
	artifact := FullBuildArtifact{
		Records: records, SpareRecords: spareRecords, ReviewedRecordCount: reviewedRecordCount, GeneratedRecordCount: len(records) - reviewedRecordCount,
		GenerationAttempts: metrics.generationAttempts, VerificationAttempts: metrics.verificationAttempts,
		PromptTokens: metrics.promptTokens, CompletionTokens: metrics.completionTokens, ExcludedEvidence: excludedEvidence,
	}
	report := FullBuildReport{
		Version: datasetContract, CompletedAt: p.now().UTC(), Goal: request.Goal, Model: request.Model, DatasetInstructions: request.DatasetInstructions,
		TotalRecords: len(artifact.Records), ReviewedSeedInput: reviewedSeedInput, ReviewedSeedCount: artifact.ReviewedRecordCount, GeneratedCount: artifact.GeneratedRecordCount,
		GenerationAttempts: artifact.GenerationAttempts, VerificationAttempts: artifact.VerificationAttempts,
		PromptTokens: artifact.PromptTokens, CompletionTokens: artifact.CompletionTokens, ExcludedEvidence: artifact.ExcludedEvidence,
		SpareFacts: questionAnswerFactCount(spareRecords),
	}
	artifact, err = writeFullBuildArtifacts(request.DatasetDirectory, artifact, spareRecords, report)
	if err != nil {
		return FullBuildArtifact{}, err
	}
	reportProgress("Dataset preparation complete", totalUnits)

	return artifact, nil
}

// MergeFullBuildArtifacts namespaces independently prepared source facts and
// atomically publishes one combined dataset for a single training run.
func MergeFullBuildArtifacts(request FullBuildMergeRequest) (FullBuildArtifact, error) {
	if request.DatasetDirectory == "" {
		return FullBuildArtifact{}, fmt.Errorf("merge dataset: dataset directory is required")
	}
	if len(request.Documents) == 0 {
		return FullBuildArtifact{}, fmt.Errorf("merge dataset: at least one document is required")
	}

	artifact := FullBuildArtifact{}
	report := FullBuildReport{
		Version: datasetContract, CompletedAt: time.Now().UTC(), Goal: request.Goal,
		Model: request.Model, DatasetInstructions: request.DatasetInstructions,
		Documents: make([]FullBuildDocumentReport, 0, len(request.Documents)),
	}
	seenPrompts := make(map[string]string)
	for _, document := range request.Documents {
		if document.ID == "" || document.Name == "" {
			return FullBuildArtifact{}, fmt.Errorf("merge dataset: document id and name are required")
		}
		records := namespaceDocumentRecords(document, document.Artifact.Records)
		for _, record := range records {
			key := strings.ToLower(strings.TrimSpace(record.Prompt))
			if source, duplicate := seenPrompts[key]; duplicate {
				return FullBuildArtifact{}, fmt.Errorf("merge dataset: duplicate prompt %q from %q and %q", record.Prompt, source, document.Name)
			}
			seenPrompts[key] = document.Name
		}
		artifact.Records = append(artifact.Records, records...)
		artifact.SpareRecords = append(artifact.SpareRecords, namespaceDocumentRecords(document, document.Artifact.SpareRecords)...)
		artifact.ReviewedRecordCount += document.Artifact.ReviewedRecordCount
		artifact.GeneratedRecordCount += document.Artifact.GeneratedRecordCount
		artifact.GenerationAttempts += document.Artifact.GenerationAttempts
		artifact.VerificationAttempts += document.Artifact.VerificationAttempts
		artifact.PromptTokens += document.Artifact.PromptTokens
		artifact.CompletionTokens += document.Artifact.CompletionTokens
		artifact.ExcludedEvidence += document.Artifact.ExcludedEvidence
		report.Documents = append(report.Documents, FullBuildDocumentReport{
			ID: document.ID, Name: document.Name,
			Facts: questionAnswerFactCount(document.Artifact.Records), Records: len(document.Artifact.Records),
		})
	}
	if len(artifact.Records) == 0 {
		return FullBuildArtifact{}, fmt.Errorf("merge dataset: documents produced no training records")
	}
	report.TotalRecords = len(artifact.Records)
	report.ReviewedSeedInput = artifact.ReviewedRecordCount
	report.ReviewedSeedCount = artifact.ReviewedRecordCount
	report.GeneratedCount = artifact.GeneratedRecordCount
	report.GenerationAttempts = artifact.GenerationAttempts
	report.VerificationAttempts = artifact.VerificationAttempts
	report.PromptTokens = artifact.PromptTokens
	report.CompletionTokens = artifact.CompletionTokens
	report.ExcludedEvidence = artifact.ExcludedEvidence
	report.SpareFacts = questionAnswerFactCount(artifact.SpareRecords)

	return writeFullBuildArtifacts(request.DatasetDirectory, artifact, artifact.SpareRecords, report)
}

func namespaceDocumentRecords(document FullBuildDocument, records []Record) []Record {
	converted := make([]Record, len(records))
	for index, record := range records {
		record.SourceDocument = document.Name
		if record.FactID != "" {
			record.FactID = document.ID + "-" + record.FactID
		}
		converted[index] = record
	}
	return converted
}

func writeFullBuildArtifacts(directory string, artifact FullBuildArtifact, spareRecords []Record, report FullBuildReport) (FullBuildArtifact, error) {
	reportData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return FullBuildArtifact{}, fmt.Errorf("encode dataset report: %w", err)
	}
	artifact.RecordsPath = filepath.Join(directory, "dataset.jsonl")
	if err := writeRecords(artifact.RecordsPath, artifact.Records); err != nil {
		return FullBuildArtifact{}, fmt.Errorf("write dataset records: %w", err)
	}
	artifact.SpareRecordsPath = filepath.Join(directory, datasetSpareFile)
	if len(spareRecords) > 0 {
		if err := writeRecords(artifact.SpareRecordsPath, spareRecords); err != nil {
			return FullBuildArtifact{}, fmt.Errorf("write dataset spare records: %w", err)
		}
	} else {
		if err := os.Remove(artifact.SpareRecordsPath); err != nil && !os.IsNotExist(err) {
			return FullBuildArtifact{}, fmt.Errorf("remove stale dataset spare records: %w", err)
		}
		artifact.SpareRecordsPath = ""
	}
	artifact.ReportPath = filepath.Join(directory, "dataset-report.json")
	if err := writePreviewReport(artifact.ReportPath, append(reportData, '\n')); err != nil {
		return FullBuildArtifact{}, fmt.Errorf("write dataset report: %w", err)
	}
	if err := os.Remove(filepath.Join(directory, buildCheckpointFile)); err != nil && !os.IsNotExist(err) {
		return FullBuildArtifact{}, fmt.Errorf("remove completed dataset checkpoint: %w", err)
	}
	return artifact, nil
}

func (p *Previewer) prepareFullBatch(ctx context.Context, request FullBuildRequest, passages []string, exclusions generationExclusions, progress func(string)) ([]Record, int, previewMetrics, error) {
	previewRequest := PreviewRequest{Goal: request.Goal, Model: request.Model, DatasetInstructions: request.DatasetInstructions}
	recordCount := len(passages)
	var generated []Record
	var metrics previewMetrics
	var err error
	if request.Goal == GoalQuestionAnswer {
		progress("Selecting grounded answers")
		generated, metrics, err = p.selectQuestionAnswerFacts(ctx, request.Model, request.DatasetInstructions, passages, exclusions)
	} else {
		generated, metrics, err = p.generateRecords(ctx, previewRequest, formatPassages(passages), passages, recordCount, exclusions, true, maxGenerationAttempts)
	}
	if err == nil && request.Goal == GoalQuestionAnswer && len(generated) == 0 {
		return nil, len(passages), metrics, nil
	}
	if err == nil && request.Goal == GoalQuestionAnswer {
		progress("Creating questions for grounded answers")
		expanded, expansionMetrics, expansionErr := p.expandQuestionAnswerRecords(ctx, request.Model, request.DatasetInstructions, generated, exclusions)
		metrics.add(expansionMetrics)
		if expansionErr == nil {
			progress("Independently verifying generated records")
			generated, expansionMetrics, expansionErr = p.verifyQuestionAnswerRecords(ctx, request.Model, request.DatasetInstructions, generated, expanded)
			metrics.add(expansionMetrics)
		}
		err = expansionErr
	}
	if err == nil {
		verified := generated
		if request.Goal != GoalQuestionAnswer {
			progress("Independently verifying generated records")
			verification, verificationMetrics, verifyErr := p.verifyRecords(ctx, request.Model, request.Goal, request.DatasetInstructions, generated)
			metrics.add(verificationMetrics)
			if verifyErr != nil {
				err = verifyErr
			} else {
				verified, _ = passingRecords(generated, verification)
			}
		}
		if err == nil {
			acceptedEvidence := make(map[string]struct{}, len(verified)/questionAnswerRequiredVariants)
			for _, record := range verified {
				acceptedEvidence[record.Evidence] = struct{}{}
			}
			incomplete := 0
			for _, passage := range passages {
				if _, accepted := acceptedEvidence[passage]; !accepted {
					incomplete++
				}
			}
			return verified, incomplete, metrics, nil
		}
	}
	if !isRecordQualityError(err) {
		return nil, 0, metrics, err
	}
	progress("Skipping source passages that did not produce complete records")
	return nil, len(passages), metrics, nil
}

func (p *Previewer) selectQuestionAnswerFacts(ctx context.Context, model, datasetInstructions string, passages []string, exclusions generationExclusions) ([]Record, previewMetrics, error) {
	var metrics previewMetrics
	targetCandidates := questionAnswerTargetCandidates(passages)
	prioritizeQuestionAnswerTargets(targetCandidates, passages, exclusions.targets)
	userPrompt := questionAnswerTargetPrompt(targetCandidates, exclusions.examples, exclusions.targets, datasetInstructions)
	metrics.generationAttempts++
	completion, err := p.completer.Complete(ctx, CompletionRequest{
		Model:          model,
		Messages:       generationMessages(userPrompt, ""),
		ResponseSchema: questionAnswerTargetSchema(len(passages)),
		MaxTokens:      max(generationMaxTokens, len(passages)*32),
		Seed:           preparationGenerationSeed,
	})
	if err != nil {
		return nil, metrics, fmt.Errorf("select grounded answers: %w", err)
	}
	metrics.promptTokens += completion.PromptTokens
	metrics.completionTokens += completion.CompletionTokens

	var response struct {
		TargetIDs []int `json:"target_ids"`
	}
	if err := json.Unmarshal([]byte(completion.Content), &response); err != nil {
		return nil, metrics, recordQualityError{err: fmt.Errorf("decode selected answers: %w", err)}
	}
	records, err := validateQuestionAnswerTargets(response.TargetIDs, passages, targetCandidates)
	if err != nil {
		return nil, metrics, recordQualityError{err: err}
	}
	return records, metrics, nil
}

func questionAnswerTargetPrompt(targetCandidates [][]string, examples []Record, previousTargets []string, datasetInstructions string) string {
	var prompt strings.Builder
	targetDetailRequirement := "Choose the shortest target that completely expresses that fact."
	targetScopeRequirement := "When the central fact naturally asks for a name, function, method, value, signal, duration, or identifier, prefer that atomic answer over a sentence containing it. Choose a sentence when the useful fact is a behavior, comparison, condition, relationship, or effect, and retain every clause needed to express that proposition."
	if datasetInstructions != "" {
		prompt.WriteString(datasetInstructionsPrompt(datasetInstructions))
		prompt.WriteString("\n\n")
		targetDetailRequirement = "Choose a grounded target that completely expresses one useful fact."
		targetScopeRequirement = "Choose the target's scope and level of detail by following the user instruction."
	}
	fmt.Fprintf(&prompt, "Review %d source passages and select at most one useful training answer from each. Do not write questions yet.\n", len(targetCandidates))
	fmt.Fprintf(&prompt, "Choose the most useful central instructional fact in each passage. %s The target is the complete answer users will be trained to return. Do not choose an incidental package name, declaration, heading, or example detail merely because it is shorter than the passage's useful behavior or principle. %s Prefer durable facts over one-run benchmark or profile measurements, and prefer a new target over one already selected; these are preferences, so reuse the best available target rather than skipping an otherwise useful passage. Avoid targets containing unrelated surrounding text, unresolved words such as this or it, incomplete code declarations, or additional clauses. Preserve the exact role and capitalization of code symbols: a method name must not become a type, interface, package, or unrelated API. Do not merge facts from alternative packages or APIs. Return one target_id per passage in passage order. Return target_id 0 for navigation, tables of contents, malformed tables, presentation-only text, quotations without useful context, or any passage that cannot support a natural standalone question without revealing its answer.\n", targetDetailRequirement, targetScopeRequirement)
	for index, example := range examples {
		fmt.Fprintf(&prompt, "\n[APPROVED EXAMPLE %d]\nTARGET: %s\n", index+1, example.Target)
	}
	if len(previousTargets) > 0 {
		prompt.WriteString("\n[PREVIOUSLY SELECTED TARGETS — PREFER A DIFFERENT FACT WHEN AVAILABLE]\n")
		for _, target := range previousTargets {
			fmt.Fprintf(&prompt, "- %s\n", target)
		}
	}
	prompt.WriteString("\nSOURCE:\n")
	prompt.WriteString(formatQuestionAnswerTargetPassages(targetCandidates))
	return prompt.String()
}

func questionAnswerTargetCandidates(passages []string) [][]string {
	candidates := make([][]string, len(passages))
	for passageIndex, passage := range passages {
		for _, target := range quoteCandidates(passage) {
			if usableQuestionAnswerTarget(target) {
				candidates[passageIndex] = append(candidates[passageIndex], target)
			}
		}
	}
	return candidates
}

func prioritizeQuestionAnswerTargets(candidates [][]string, passages, previousTargets []string) {
	previous := make(map[string]struct{}, len(previousTargets))
	for _, target := range previousTargets {
		previous[normalizedTarget(target)] = struct{}{}
	}
	for index := range candidates {
		slices.SortStableFunc(candidates[index], func(left, right string) int {
			return cmp.Compare(questionAnswerTargetPreference(left, passages[index], previous), questionAnswerTargetPreference(right, passages[index], previous))
		})
	}
}

func questionAnswerTargetPreference(target, evidence string, previous map[string]struct{}) int {
	preference := 0
	if transientBenchmarkTarget(target, evidence) {
		preference += 2
	}
	if _, duplicate := previous[normalizedTarget(target)]; duplicate {
		preference++
	}
	return preference
}

func transientBenchmarkTarget(target, evidence string) bool {
	target = strings.ToLower(target)
	if strings.Contains(target, "default") {
		return false
	}
	evidence = strings.ToLower(evidence)
	if !strings.Contains(evidence, "benchmark") && !strings.Contains(evidence, "performance") {
		return false
	}
	return percentagePattern.MatchString(target) ||
		strings.Contains(target, "requests a second") ||
		strings.Contains(target, "requests per second") ||
		strings.Contains(target, "slowest request") ||
		strings.Contains(target, "ns/op") ||
		(strings.Contains(target, "files") && strings.Contains(target, "cores"))
}

func normalizedTarget(target string) string {
	return strings.ToLower(strings.Join(strings.Fields(normalizeSourceText(target)), " "))
}

func usableQuestionAnswerTarget(target string) bool {
	if utf8.RuneCountInString(target) > maxQuestionAnswerRunes ||
		strings.Contains(target, "<--") ||
		isDocumentNavigationTarget(target) ||
		hasLeadingContextReference(target) ||
		hasUnresolvedStandaloneReference(target) {
		return false
	}

	target = strings.ToLower(strings.TrimSpace(target))
	for _, prefix := range []string{
		"i ", "i'm ", "i’m ", "i've ", "i’ve ", "i'd ", "i’d ", "i'll ", "i’ll ",
		"we ", "we're ", "we’re ", "we've ", "we’ve ", "we'd ", "we’d ", "we'll ", "we’ll ",
		"what i need ", "what we need ",
		"in this chapter", "in this section", "in this lesson", "in this video",
		"now let's ", "now let’s ",
	} {
		if strings.HasPrefix(target, prefix) {
			return false
		}
	}
	return true
}

func formatQuestionAnswerTargetPassages(candidates [][]string) string {
	var builder strings.Builder
	for passageIndex, targets := range candidates {
		fmt.Fprintf(&builder, "[PASSAGE %d]\n", passageIndex+1)
		if len(targets) == 0 {
			builder.WriteString("[NO USABLE TARGETS: RETURN 0]\n")
		}
		for targetIndex, target := range targets {
			fmt.Fprintf(&builder, "[TARGET %d] %s\n", targetIndex+1, target)
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func questionAnswerTargetSchema(count int) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target_ids": map[string]any{
				"type": "array", "minItems": count, "maxItems": count,
				"items": map[string]any{"type": "integer", "minimum": 0},
			},
		},
		"required": []string{"target_ids"},
	}
}

func validateQuestionAnswerTargets(targetIDs []int, passages []string, targetCandidates [][]string) ([]Record, error) {
	if len(targetIDs) != len(passages) || len(targetCandidates) != len(passages) {
		return nil, fmt.Errorf("model selected %d answers for %d passages", len(targetIDs), len(passages))
	}

	records := make([]Record, 0, len(targetIDs))
	for index, targetID := range targetIDs {
		if targetID == 0 {
			continue
		}
		if targetID < 1 || targetID > len(targetCandidates[index]) {
			return nil, fmt.Errorf("answer %d has invalid target_id %d", index+1, targetID)
		}
		target := targetCandidates[index][targetID-1]
		evidence := passages[index]
		records = append(records, Record{Target: target, Evidence: evidence, FactID: recordFactID(evidence, target)})
	}
	return records, nil
}

type generatedQuestionPool struct {
	RecordNumber int      `json:"record_number"`
	Questions    []string `json:"questions"`
}

func (p *Previewer) verifyQuestionAnswerRecords(ctx context.Context, model, datasetInstructions string, seeds, candidates []Record) ([]Record, previewMetrics, error) {
	verification, metrics, err := p.verifyRecords(ctx, model, GoalQuestionAnswer, datasetInstructions, candidates)
	if err != nil {
		return nil, metrics, fmt.Errorf("build dataset: verify question pools: %w", err)
	}
	passed, _ := passingRecords(candidates, verification)
	return selectReviewedQuestionAnswerGroups(seeds, passed), metrics, nil
}

func selectReviewedQuestionAnswerGroups(seeds, records []Record) []Record {
	completeFacts := completeQuestionAnswerFacts(records)
	selected := make([]Record, 0, len(completeFacts)*(questionAnswerRequiredVariants+1))
	for _, seed := range seeds {
		if _, complete := completeFacts[seed.FactID]; !complete {
			continue
		}
		regular := 0
		for _, record := range records {
			if record.FactID != seed.FactID || record.IndependentPrompt || regular == questionAnswerRequiredVariants {
				continue
			}
			selected = append(selected, record)
			regular++
		}
		for _, record := range records {
			if record.FactID == seed.FactID && record.IndependentPrompt {
				selected = append(selected, record)
				break
			}
		}
	}
	return selected
}

func (p *Previewer) expandQuestionAnswerRecords(ctx context.Context, model, datasetInstructions string, facts []Record, exclusions generationExclusions) ([]Record, previewMetrics, error) {
	var metrics previewMetrics
	metrics.generationAttempts++
	completion, err := p.completer.Complete(ctx, CompletionRequest{
		Model:          model,
		Messages:       questionPoolMessages(facts, datasetInstructions),
		ResponseSchema: questionPoolSchema(len(facts)),
		MaxTokens:      max(generationMaxTokens, len(facts)*questionCandidatePoolSize*96),
		Seed:           preparationGenerationSeed,
	})
	if err != nil {
		return nil, metrics, fmt.Errorf("build question pool: %w", err)
	}
	metrics.promptTokens += completion.PromptTokens
	metrics.completionTokens += completion.CompletionTokens

	var response struct {
		Pools []generatedQuestionPool `json:"pools"`
	}
	if err := json.Unmarshal([]byte(completion.Content), &response); err != nil {
		return nil, metrics, recordQualityError{err: fmt.Errorf("decode question pool: %w", err)}
	}
	expanded := selectQuestionPools(facts, response.Pools, exclusions.prompts)
	if len(expanded) == 0 {
		return nil, metrics, recordQualityError{err: fmt.Errorf("no source fact produced usable alternate questions")}
	}
	return expanded, metrics, nil
}

func selectQuestionPools(facts []Record, pools []generatedQuestionPool, excludedPrompts []string) []Record {
	byRecord := make(map[int][]string, len(pools))
	for _, pool := range pools {
		if pool.RecordNumber >= 1 && pool.RecordNumber <= len(facts) {
			byRecord[pool.RecordNumber] = pool.Questions
		}
	}

	excluded := make(map[string]struct{}, len(excludedPrompts))
	for _, prompt := range excludedPrompts {
		excluded[strings.ToLower(strings.TrimSpace(prompt))] = struct{}{}
	}
	selected := make([]Record, 0, len(facts)*(questionAnswerRequiredVariants+1))
	for index, fact := range facts {
		prompts := make([]string, 0, questionCandidatePoolSize)
		if fact.Prompt != "" {
			prompts = append(prompts, fact.Prompt)
		}
		candidates := validQuestionCandidates(fact, byRecord[index+1], excluded)
		preferred := slices.DeleteFunc(slices.Clone(candidates), func(candidate string) bool {
			return questionRepeatsAtomicTarget(candidate, fact.Target)
		})
		if len(preferred) >= questionAnswerRequiredVariants+1-len(prompts) {
			candidates = preferred
		}
		prompts = append(prompts, selectDistinctQuestions(prompts, candidates, questionAnswerRequiredVariants+1-len(prompts), fact.Target)...)
		if len(prompts) != questionAnswerRequiredVariants+1 {
			continue
		}
		for promptIndex, prompt := range prompts {
			selected = append(selected, Record{
				Prompt: prompt, Target: fact.Target, Evidence: fact.Evidence, FactID: fact.FactID,
				IndependentPrompt: promptIndex == questionAnswerRequiredVariants,
			})
			excluded[strings.ToLower(strings.TrimSpace(prompt))] = struct{}{}
		}
	}
	return selected
}

func validQuestionCandidates(fact Record, candidates []string, excluded map[string]struct{}) []string {
	valid := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		key := strings.ToLower(candidate)
		if _, found := excluded[key]; found {
			continue
		}
		if _, found := seen[key]; found {
			continue
		}
		if validateNaturalQuestion(candidate) != nil || validateRecordShape(GoalQuestionAnswer, candidate, fact.Target) != nil {
			continue
		}
		seen[key] = struct{}{}
		valid = append(valid, candidate)
	}
	return valid
}

func selectDistinctQuestions(existing, candidates []string, count int, target string) []string {
	selected := append([]string(nil), existing...)
	remaining := append([]string(nil), candidates...)
	for len(selected)-len(existing) < count && len(remaining) > 0 {
		best := 0
		bestRevealsTarget := true
		bestSimilarity := 2.0
		for index, candidate := range remaining {
			revealsTarget := questionRepeatsAtomicTarget(candidate, target)
			similarity := maximumQuestionSimilarity(candidate, selected)
			if similarity < bestSimilarity || (similarity == bestSimilarity && !revealsTarget && bestRevealsTarget) {
				best = index
				bestRevealsTarget = revealsTarget
				bestSimilarity = similarity
			}
		}
		selected = append(selected, remaining[best])
		remaining = slices.Delete(remaining, best, best+1)
	}
	return selected[len(existing):]
}

func questionRepeatsAtomicTarget(question, target string) bool {
	question = strings.ToLower(strings.Join(strings.FieldsFunc(question, func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	}), " "))
	target = strings.ToLower(strings.Join(strings.FieldsFunc(target, func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	}), " "))
	if target == "" || len(strings.Fields(target)) > 3 {
		return false
	}
	return strings.Contains(" "+question+" ", " "+target+" ")
}

func maximumQuestionSimilarity(question string, previous []string) float64 {
	return maximumWordSimilarity(question, previous)
}

func maximumWordSimilarity(text string, previous []string) float64 {
	maximum := 0.0
	words := significantWords(text)
	for _, candidate := range previous {
		candidateWords := significantWords(candidate)
		intersection := 0
		for word := range words {
			if _, found := candidateWords[word]; found {
				intersection++
			}
		}
		union := len(words) + len(candidateWords) - intersection
		if union > 0 {
			maximum = max(maximum, float64(intersection)/float64(union))
		}
	}
	return maximum
}

func questionPoolMessages(facts []Record, datasetInstructions string) []Message {
	var prompt strings.Builder
	if datasetInstructions != "" {
		prompt.WriteString(datasetInstructionsPrompt(datasetInstructions))
		prompt.WriteString("\n\n")
	}
	styleRequirements := "Every question must differ in framing or information need, not only synonyms or what/which substitutions."
	if datasetInstructions == "" {
		styleRequirements = "Use these forms in order: (1) concise and direct, (2) contextual with the subject named, (3) a realistic scenario, (4) conversational, (5) a different concise framing, (6) a different contextual framing, (7) a different scenario, (8) a different conversational framing, and (9-10) two additional natural alternatives. Each conversational form must directly ask a question and end with a question mark. Every question must differ in framing or information need, not only synonyms or what/which substitutions."
	}
	fmt.Fprintf(&prompt, "Write exactly %d natural candidate questions for each locked fact and fill all %d array positions; do not stop after the first four. Every position is required. %s For an identifier answer, describe what the code does without repeating words contained in that identifier.\n", questionCandidatePoolSize, questionCandidatePoolSize, styleRequirements)
	prompt.WriteString("Every question must stand alone, end with a question mark, and be answered exactly and completely by the locked target and evidence. Match the question's scope to the complete target: if the target is a sentence, ask for its entire proposition rather than one name or detail embedded in it; if the question asks only for a name or value, the locked target must be only that name or value. Name the subject needed to understand the question. Never use context-dependent wording such as this program, this function, this type, this example, it, or they unless the question itself uniquely identifies what those words mean. Preserve the target's answer type, direction, scope, code-symbol role, package, and capitalization. Never reveal the answer, reverse the relationship, combine alternate APIs, invent context, or infer a stronger effect than the evidence states; registering handlers, for example, does not start a server. Do not refer to provided evidence, code, examples, output, text, documents, listings, sections, or tables. Return only the record number and questions.\n")
	for index, fact := range facts {
		fmt.Fprintf(&prompt, "\n[LOCKED FACT %d]\nEVIDENCE: %s\nLOCKED TARGET: %s\n", index+1, verificationEvidence(fact), fact.Target)
	}
	return []Message{
		{Role: "system", Content: "You create natural, meaningfully distinct questions for locked source-grounded facts. Never change or add facts. The user's instruction controls how every question is written and must be followed. Only source grounding, factual correctness, and the required output structure take priority over the user's instruction."},
		{Role: "user", Content: strings.TrimSpace(prompt.String())},
	}
}

func questionPoolSchema(recordCount int) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pools": map[string]any{
				"type": "array", "minItems": recordCount, "maxItems": recordCount,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"record_number": map[string]any{"type": "integer", "minimum": 1, "maximum": recordCount},
						"questions": map[string]any{
							"type": "array", "minItems": questionCandidatePoolSize, "maxItems": questionCandidatePoolSize,
							"items": map[string]any{"type": "string"},
						},
					},
					"required": []string{"record_number", "questions"},
				},
			},
		},
		"required": []string{"pools"},
	}
}

func validateNaturalQuestion(prompt string) error {
	normalized := strings.ToLower(strings.Join(strings.Fields(prompt), " "))
	for _, phrase := range []string{
		"rewrite the question",
		"change the grammar",
		"grammar and information order",
		"in the question:",
		"according to the text",
		"according to the source",
		"according to the evidence",
		"source passage",
		"source material",
		"source code",
		"provided text",
		"provided code",
		"provided documentation",
		"provided go code",
		"provided example",
		"provided orchestration",
		"provided string",
		"provided test output",
		"in the example program",
		"in the text",
		"does the text",
		"shown in the output",
		"shown in the second",
		"second time command output",
		"this batch",
		"according to table",
		"shown in table",
		"in the code",
		"after the code",
		"code executes",
		"code runs",
		"code snippet",
		"code example",
		"code block",
		"first target",
	} {
		if strings.Contains(normalized, phrase) {
			return fmt.Errorf("question depends on preparation instructions or unavailable source context")
		}
	}
	for word := range strings.FieldsFuncSeq(normalized, func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	}) {
		if word == "passage" || word == "passages" {
			return fmt.Errorf("question depends on an unavailable source passage")
		}
	}
	if strings.Contains(normalized, "source block") || hasOrdinalTargetReference(normalized) {
		return fmt.Errorf("question depends on internal preparation labels")
	}
	if strings.Contains(normalized, "listing ") {
		return fmt.Errorf("question depends on an unavailable document listing")
	}
	return nil
}

func hasOrdinalTargetReference(prompt string) bool {
	for _, ordinal := range []string{"first", "second", "third", "fourth", "fifth", "sixth", "last"} {
		if strings.Contains(prompt, ordinal+" target") || strings.Contains(prompt, "target "+ordinal) {
			return true
		}
	}
	return false
}

func significantWords(text string) map[string]struct{} {
	// Compare long words by a stable prefix so inflections such as terminate,
	// termination, guarantee, and guaranteed count as the same question cue.
	const comparisonPrefix = 8

	ignored := map[string]struct{}{
		"a": {}, "an": {}, "and": {}, "are": {}, "do": {}, "does": {}, "for": {}, "how": {}, "in": {}, "is": {},
		"of": {}, "or": {}, "the": {}, "to": {}, "was": {}, "were": {}, "what": {}, "when": {}, "where": {},
		"which": {}, "who": {}, "why": {}, "with": {},
	}
	words := make(map[string]struct{})
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	}) {
		if _, skip := ignored[word]; !skip {
			characters := []rune(word)
			if len(characters) > comparisonPrefix {
				word = string(characters[:comparisonPrefix])
			}
			words[word] = struct{}{}
		}
	}
	return words
}

func completeQuestionAnswerVariants(records []Record) []Record {
	completeFacts := completeQuestionAnswerFacts(records)
	complete := make([]Record, 0, len(records))
	for _, record := range records {
		if _, found := completeFacts[record.FactID]; found {
			complete = append(complete, record)
		}
	}
	return complete
}

func partitionQuestionAnswerFacts(records []Record, limit int) ([]Record, []Record) {
	factCapacity := len(records) / (questionAnswerRequiredVariants + 1)
	facts := make(map[string][]Record, factCapacity)
	order := make([]string, 0, factCapacity)
	for _, record := range records {
		if _, found := facts[record.FactID]; !found {
			order = append(order, record.FactID)
		}
		facts[record.FactID] = append(facts[record.FactID], record)
	}
	slices.SortStableFunc(order, func(left, right string) int {
		leftTarget := facts[left][0].Target
		rightTarget := facts[right][0].Target
		return cmp.Or(
			cmp.Compare(questionAnswerTargetInterference(leftTarget), questionAnswerTargetInterference(rightTarget)),
			cmp.Compare(len(significantWords(leftTarget)), len(significantWords(rightTarget))),
		)
	})

	selectionCapacity := min(limit, len(order))
	selected := make(map[string]struct{}, selectionCapacity)
	selectedTargets := make([]string, 0, selectionCapacity)
	deferred := make([]string, 0, len(order))
	currentRisk := -1
	for _, factID := range order {
		target := facts[factID][0].Target
		risk := questionAnswerTargetInterference(target)
		if risk != currentRisk {
			if len(selected)+len(deferred) >= limit {
				break
			}
			currentRisk = risk
		}
		if maximumWordSimilarity(target, selectedTargets) >= maximumTargetSimilarity {
			deferred = append(deferred, factID)
			continue
		}
		selected[factID] = struct{}{}
		selectedTargets = append(selectedTargets, target)
		if len(selected) == limit {
			break
		}
	}
	for _, factID := range deferred {
		if len(selected) == limit {
			break
		}
		selected[factID] = struct{}{}
	}

	limited := make([]Record, 0, min(len(records), limit*(questionAnswerRequiredVariants+1)))
	for _, record := range records {
		if _, found := selected[record.FactID]; found {
			limited = append(limited, record)
		}
	}
	spares := make([]Record, 0, len(records)-len(limited))
	for _, factID := range order {
		if _, found := selected[factID]; !found {
			spares = append(spares, facts[factID]...)
		}
	}
	return limited, spares
}

func questionAnswerTargetInterference(target string) int {
	normalizedTarget := strings.Join(strings.Fields(normalizeSourceText(target)), " ")
	target = strings.ToLower(normalizedTarget)
	risk := 0
	if fields := strings.Fields(normalizedTarget); len(fields) > 4 {
		for _, character := range normalizedTarget {
			if unicode.IsLetter(character) {
				if unicode.IsLower(character) {
					risk += 2
				}
				break
			}
		}
	}
	if hasNumberedDocumentReference(target) {
		risk += 2
	}
	for _, prefix := range []string{
		"in this ", "the key here", "the beauty of", "everything i ", "finally ", "these ", "those ",
	} {
		if strings.HasPrefix(target, prefix) {
			risk += 2
			break
		}
	}

	for word := range strings.FieldsFuncSeq(target, func(character rune) bool {
		return !unicode.IsLetter(character)
	}) {
		switch word {
		case "i", "this", "that":
			risk++
		}
	}
	if (strings.HasPrefix(target, "\"") || strings.HasPrefix(target, "“")) &&
		(strings.Contains(target, " - ") || strings.Contains(target, " — ")) {
		risk++
	}
	return risk
}

func hasNumberedDocumentReference(target string) bool {
	fields := strings.Fields(target)
	for index := 0; index+1 < len(fields); index++ {
		label := strings.Trim(fields[index], "`'\".,;:()[]{}")
		number := strings.Trim(fields[index+1], "`'\".,;:()[]{}")
		switch label {
		case "line", "listing", "figure", "page":
			if number != "" && unicode.IsDigit(rune(number[0])) {
				return true
			}
		}
	}
	return false
}

func questionAnswerFactCount(records []Record) int {
	return len(completeQuestionAnswerFacts(records))
}

func completeQuestionAnswerFacts(records []Record) map[string]struct{} {
	counts := make(map[string]int, len(records)/questionAnswerRequiredVariants)
	holdouts := make(map[string]struct{}, len(counts))
	for _, record := range records {
		if record.FactID == "" {
			continue
		}
		if record.IndependentPrompt {
			holdouts[record.FactID] = struct{}{}
		} else {
			counts[record.FactID]++
		}
	}
	complete := make(map[string]struct{}, len(counts))
	for factID, count := range counts {
		if _, hasHoldout := holdouts[factID]; count >= questionAnswerRequiredVariants && hasHoldout {
			complete[factID] = struct{}{}
		}
	}
	return complete
}

func buildFingerprint(markdown []byte, request FullBuildRequest, seeds []Record) (string, error) {
	contract, err := json.Marshal(struct {
		Version             int      `json:"version"`
		Goal                Goal     `json:"goal"`
		Model               string   `json:"model"`
		DatasetInstructions string   `json:"dataset_instructions,omitempty"`
		Seeds               []Record `json:"seeds"`
	}{Version: datasetContract, Goal: request.Goal, Model: request.Model, DatasetInstructions: request.DatasetInstructions, Seeds: seeds})
	if err != nil {
		return "", fmt.Errorf("encode dataset checkpoint identity: %w", err)
	}
	digest := sha256.New()
	_, _ = digest.Write(markdown)
	_, _ = digest.Write(contract)
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func loadBuildCheckpoint(directory, fingerprint string, totalBatches int) (buildCheckpoint, bool) {
	data, err := os.ReadFile(filepath.Join(directory, buildCheckpointFile))
	if err != nil {
		return buildCheckpoint{}, false
	}
	var checkpoint buildCheckpoint
	if json.Unmarshal(data, &checkpoint) != nil || checkpoint.Version != datasetContract || checkpoint.Fingerprint != fingerprint || checkpoint.CompletedBatches < 0 || checkpoint.CompletedBatches > totalBatches {
		return buildCheckpoint{}, false
	}
	if _, err := ValidateReviewedRecords(checkpoint.Records); err != nil {
		return buildCheckpoint{}, false
	}
	return checkpoint, true
}

func buildCheckpointFrom(fingerprint string, completed int, spareBatchCompleted bool, records []Record, excluded int, metrics previewMetrics) buildCheckpoint {
	return buildCheckpoint{
		Version: datasetContract, Fingerprint: fingerprint, CompletedBatches: completed, SpareBatchCompleted: spareBatchCompleted,
		Records: records, ExcludedEvidence: excluded,
		GenerationAttempts: metrics.generationAttempts, VerificationAttempts: metrics.verificationAttempts,
		PromptTokens: metrics.promptTokens, CompletionTokens: metrics.completionTokens,
	}
}

func saveBuildCheckpoint(directory string, checkpoint buildCheckpoint) error {
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dataset checkpoint: %w", err)
	}
	if err := writePreviewReport(filepath.Join(directory, buildCheckpointFile), append(data, '\n')); err != nil {
		return fmt.Errorf("save dataset checkpoint: %w", err)
	}
	return nil
}

// ValidateReviewedRecords checks user-reviewed preview rows before a full build.
func ValidateReviewedRecords(records []Record) ([]Record, error) {
	validated := make([]Record, len(records))
	for index, record := range records {
		record.Prompt = strings.TrimSpace(record.Prompt)
		record.Target = strings.TrimSpace(normalizeSourceText(record.Target))
		record.Evidence = strings.TrimSpace(normalizeSourceText(record.Evidence))
		if utf8.RuneCountInString(record.Prompt) < 3 || record.Target == "" || record.Evidence == "" {
			return nil, fmt.Errorf("record %d has an empty or unusable prompt, target, or evidence", index+1)
		}
		candidates := quoteCandidates(record.Evidence)
		if canonical := cleanProse(record.Target); slices.Contains(candidates, canonical) {
			record.Target = canonical
		}
		grounded := slices.Contains(candidates, record.Target)
		if !grounded {
			return nil, fmt.Errorf("record %d target is not an exact quote candidate from its evidence", index+1)
		}
		record.FactID = recordFactID(record.Evidence, record.Target)
		if err := validateNewRecords(validated[:index], []Record{record}); err != nil {
			return nil, fmt.Errorf("record %d: %w", index+1, err)
		}
		validated[index] = record
	}
	return validated, nil
}

func sameReviewedRecord(left, right Record) bool {
	return left.Prompt == right.Prompt && left.Target == right.Target && left.Evidence == right.Evidence
}

func validateNewRecords(existing, added []Record) error {
	prompts := make(map[string]struct{}, len(existing)+len(added))
	evidenceFacts := make(map[string]string, len(existing)+len(added))
	for _, record := range append(append([]Record(nil), existing...), added...) {
		prompt := strings.ToLower(strings.TrimSpace(record.Prompt))
		if _, duplicate := prompts[prompt]; duplicate {
			return fmt.Errorf("duplicate prompt %q", record.Prompt)
		}
		fact := record.FactID
		if fact == "" {
			fact = recordFactID(record.Evidence, record.Target)
		}
		if previous, duplicate := evidenceFacts[record.Evidence]; duplicate && previous != fact {
			return fmt.Errorf("same evidence selects conflicting training facts")
		}
		prompts[prompt] = struct{}{}
		evidenceFacts[record.Evidence] = fact
	}
	return nil
}

func recordPrompts(records []Record) []string {
	prompts := make([]string, len(records))
	for index, record := range records {
		prompts[index] = record.Prompt
	}
	return prompts
}

func recordTargets(records []Record) []string {
	targets := make([]string, 0, len(records)/(questionAnswerRequiredVariants+1))
	seen := make(map[string]struct{}, cap(targets))
	for _, record := range records {
		key := normalizedTarget(record.Target)
		if _, found := seen[key]; found {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, record.Target)
	}
	return targets
}

func fullPassages(markdown string, seeds []Record) []string {
	markdown = normalizeSourceText(markdown)
	markdown = strings.ReplaceAll(markdown, "<!-- image -->", "")
	seedEvidence := make(map[string]struct{}, len(seeds))
	for _, seed := range seeds {
		seedEvidence[seed.Evidence] = struct{}{}
	}
	parts := strings.Split(markdown, "\n\n")
	passages := make([]string, 0, len(parts))
	seenParts := make(map[string]struct{}, len(parts))
	seenPassages := make(map[string]struct{}, len(parts))
	current := ""
	flush := func() {
		if current != "" {
			passages = append(passages, current)
			current = ""
		}
	}
	for _, part := range parts {
		if strings.HasPrefix(strings.TrimSpace(part), "# Source Document:") {
			flush()
			continue
		}
		part = stripMarkdownHeadings(part)
		if part == "" || len(quoteCandidates(part)) == 0 {
			continue
		}
		if _, represented := seedEvidence[part]; represented {
			flush()
			seenPassages[part] = struct{}{}
			continue
		}
		if _, duplicate := seenParts[part]; duplicate {
			continue
		}
		seenParts[part] = struct{}{}
		for _, bounded := range boundedPassages(part) {
			if _, duplicate := seenPassages[bounded]; duplicate {
				continue
			}
			seenPassages[bounded] = struct{}{}
			if current == "" {
				current = bounded
				continue
			}
			if utf8.RuneCountInString(current)+2+utf8.RuneCountInString(bounded) <= fullEvidenceRunes {
				current += "\n\n" + bounded
				continue
			}
			flush()
			current = bounded
		}
	}
	flush()
	return passages
}

func stripMarkdownHeadings(value string) string {
	lines := strings.Split(value, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func boundedPassages(passage string) []string {
	if utf8.RuneCountInString(passage) <= fullEvidenceRunes {
		return []string{passage}
	}

	runes := []rune(passage)
	parts := make([]string, 0, len(runes)/fullEvidenceRunes+1)
	for len(runes) > 0 {
		end := min(len(runes), fullEvidenceRunes)
		if end < len(runes) {
			for end > fullEvidenceRunes/2 && !strings.ContainsRune(".!?\n", runes[end-1]) {
				end--
			}
			if end == fullEvidenceRunes/2 {
				end = fullEvidenceRunes
			}
		}
		part := strings.TrimSpace(string(runes[:end]))
		if len(quoteCandidates(part)) > 0 {
			parts = append(parts, part)
		}
		runes = runes[end:]
	}
	return parts
}

func fullPassageBatches(passages []string) [][]string {
	var batches [][]string
	for len(passages) > 0 {
		count := min(len(passages), fullBatchPassages)
		batches = append(batches, passages[:count])
		passages = passages[count:]
	}
	return batches
}

func prioritizeFullPassages(passages []string, targetFacts int) []string {
	selectedCount := min(len(passages), targetFacts*2)
	if targetFacts <= 0 || selectedCount == len(passages) {
		return passages
	}

	prioritized := make([]string, 0, len(passages))
	selected := make(map[int]struct{}, selectedCount)
	for index := 1; index <= selectedCount; index++ {
		passageIndex := index * len(passages) / (selectedCount + 1)
		selected[passageIndex] = struct{}{}
		prioritized = append(prioritized, passages[passageIndex])
	}
	for index, passage := range passages {
		if _, found := selected[index]; !found {
			prioritized = append(prioritized, passage)
		}
	}
	return prioritized
}
