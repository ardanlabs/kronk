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
	datasetContract           = 53
	datasetFingerprintVersion = 51
	questionCandidatePoolSize = questionAnswerRecordsPerFact
	plannedFactAttempts       = 3
	maximumFactsPerBatch      = 16
	questionPoolBatchBytes    = 14_000
)

type buildCheckpoint struct {
	Version              int            `json:"version"`
	PlanVersion          int            `json:"plan_version"`
	Fingerprint          string         `json:"fingerprint"`
	CompletedBatches     int            `json:"completed_batches"`
	Records              []Record       `json:"records"`
	RejectedFacts        []RejectedFact `json:"rejected_facts,omitempty"`
	ExcludedEvidence     int            `json:"excluded_evidence"`
	GenerationAttempts   int            `json:"generation_attempts"`
	VerificationAttempts int            `json:"verification_attempts"`
	PromptTokens         int            `json:"prompt_tokens"`
	CompletionTokens     int            `json:"completion_tokens"`
}

// RejectedFact records a planned target that could not produce a complete,
// verified question group after bounded regeneration.
type RejectedFact struct {
	SourceDocument string `json:"source_document,omitempty"`
	SourceChunkID  string `json:"source_chunk_id"`
	FactID         string `json:"fact_id"`
	Prompt         string `json:"prompt,omitempty"`
	Target         string `json:"target"`
	Evidence       string `json:"evidence,omitempty"`
	UserApproved   bool   `json:"user_approved,omitempty"`
	UserEdited     bool   `json:"user_edited,omitempty"`
	Reason         string `json:"reason"`
}

// FullBuildRequest selects an extraction and reviewed records for a complete
// canonical dataset build.
type FullBuildRequest struct {
	DatasetDirectory       string
	Goal                   Goal
	Model                  string
	DatasetInstructions    string
	ReviewedRecords        []Record
	VerifiedPreviewRecords []Record
	Progress               func(FullBuildProgress)
}

// FullBuildProgress reports durable units of work completed by a full build.
type FullBuildProgress struct {
	Phase            string
	CompletedChunks  int
	TotalChunks      int
	CompletedBatches int
	TotalBatches     int
	AcceptedRecords  int
	AcceptedFacts    int
	PlannedRecords   int
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
	RejectedFacts        []RejectedFact
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
	RejectedFacts        []RejectedFact            `json:"rejected_facts,omitempty"`
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
	usableSeedIndexes := make([]int, 0, len(seeds))
	for index, record := range seeds {
		unchanged := previewAligned && sameReviewedRecord(record, verifiedPreview[index])
		record.UserEdited = record.UserApproved && !unchanged
		var qualityErr error
		if !record.UserEdited {
			qualityErr = validateRecordShape(request.Goal, record.Prompt, record.Target)
			if qualityErr == nil && request.Goal == GoalQuestionAnswer {
				qualityErr = validateNaturalQuestion(record.Prompt)
			}
		}
		if qualityErr != nil {
			if unchanged {
				continue
			}
			return FullBuildArtifact{}, fmt.Errorf("build dataset: invalid reviewed record %d: %w", index+1, qualityErr)
		}
		if request.Goal == GoalQuestionAnswer {
			record.FactID = recordFactID(record.Evidence, record.Target)
		}
		usableSeeds = append(usableSeeds, record)
		usableSeedIndexes = append(usableSeedIndexes, index)
	}
	seeds = usableSeeds
	reviewedToVerify := append([]Record(nil), seeds...)
	reviewedIndexes := slices.Clone(usableSeedIndexes)
	markdown, err := os.ReadFile(filepath.Join(request.DatasetDirectory, "extracted.md"))
	if err != nil {
		return FullBuildArtifact{}, fmt.Errorf("read extracted Markdown: %w", err)
	}
	var metrics previewMetrics
	if len(reviewedToVerify) > 0 {
		if request.Progress != nil {
			request.Progress(FullBuildProgress{Phase: "analysis", Stage: "Checking reviewed examples"})
		}
		verification, verificationMetrics, err := p.verifyRecords(ctx, request.Model, request.Goal, request.DatasetInstructions, reviewedToVerify)
		metrics.add(verificationMetrics)
		if err != nil {
			return FullBuildArtifact{}, fmt.Errorf("build dataset: verify reviewed preview: %w", err)
		}
		failed := failedVerification(verification)
		failedSeeds := make(map[int]struct{}, len(failed))
		for _, result := range failed {
			seedIndex := result.RecordNumber - 1
			originalIndex := reviewedIndexes[seedIndex]
			unchangedGeneratedPreview := request.Goal == GoalQuestionAnswer && previewAligned && sameReviewedRecord(seeds[seedIndex], verifiedPreview[originalIndex])
			if !unchangedGeneratedPreview {
				return FullBuildArtifact{}, fmt.Errorf("build dataset: reviewed preview record %d is not supported by its prompt, evidence, and dataset requirements: %s", originalIndex+1, result.Reason)
			}
			failedSeeds[seedIndex] = struct{}{}
		}
		if len(failedSeeds) > 0 {
			retained := make([]Record, 0, len(seeds)-len(failedSeeds))
			for index, seed := range seeds {
				if _, failed := failedSeeds[index]; !failed {
					retained = append(retained, seed)
				}
			}
			seeds = retained
		}
	}

	passages := fullPassages(string(markdown), seeds)
	batches := fullPassageBatches(passages)
	records := append([]Record(nil), seeds...)
	if len(records) == 0 && len(batches) == 0 {
		return FullBuildArtifact{}, fmt.Errorf("build dataset: extracted document has no substantive passages")
	}
	var plan DatasetPlan
	var plannedBatches [][]Record
	if request.Goal == GoalQuestionAnswer {
		fingerprint, fingerprintErr := buildFingerprint(markdown, request, seeds)
		if fingerprintErr != nil {
			return FullBuildArtifact{}, fingerprintErr
		}
		planned, planMetrics, planErr := p.buildQuestionAnswerPlan(ctx, request, markdown, fingerprint, seeds)
		metrics.add(planMetrics)
		if planErr != nil {
			return FullBuildArtifact{}, planErr
		}
		plan = planned
		for index := range seeds {
			seeds[index].SourceChunkID = sourceChunkIDForEvidence(seeds[index].Evidence, plan.Chunks)
		}
		records = nil
		plannedFacts := make([]Record, 0, len(seeds)+len(plan.Facts))
		plannedFacts = append(plannedFacts, seeds...)
		for _, fact := range plan.Facts {
			plannedFacts = append(plannedFacts, Record{
				Prompt: fact.Prompt, Target: fact.Target, Evidence: fact.Evidence, FactID: fact.FactID, SourceChunkID: fact.SourceChunkID,
			})
		}
		plannedBatches = questionAnswerFactBatches(plannedFacts, request.DatasetInstructions)
		batches = nil
	}
	totalBatches := len(batches)
	if request.Goal == GoalQuestionAnswer {
		totalBatches = len(plannedBatches)
	}
	var excludedEvidence, completedBatches int
	var rejectedFacts []RejectedFact
	if request.Goal == GoalQuestionAnswer {
		excludedEvidence = plan.ExcludedChunks
	}
	fingerprint, err := buildFingerprint(markdown, request, seeds)
	if err != nil {
		return FullBuildArtifact{}, err
	}
	checkpoint, checkpointFound := loadBuildCheckpoint(request.DatasetDirectory, fingerprint, totalBatches)
	if checkpointFound {
		records = checkpoint.Records
		rejectedFacts = checkpoint.RejectedFacts
		completedBatches = checkpoint.CompletedBatches
		excludedEvidence = checkpoint.ExcludedEvidence
		metrics = previewMetrics{
			generationAttempts: checkpoint.GenerationAttempts, verificationAttempts: checkpoint.VerificationAttempts,
			promptTokens: checkpoint.PromptTokens, completionTokens: checkpoint.CompletionTokens,
		}
	}
	// Keep one finalization unit outstanding so 100% means the dataset artifacts
	// have actually been published, not merely that the last batch completed.
	totalUnits := totalBatches*2 + 1
	completedUnits := completedBatches * 2
	reportProgress := func(stage string, units int) {
		if request.Progress == nil {
			return
		}
		phase := "generation"
		if strings.HasPrefix(stage, "Finalizing") || stage == "Dataset preparation complete" {
			phase = "finalization"
		}
		request.Progress(FullBuildProgress{
			Phase:            phase,
			CompletedBatches: completedBatches,
			TotalBatches:     totalBatches,
			AcceptedRecords:  len(records),
			AcceptedFacts:    questionAnswerFactCount(records),
			PlannedRecords:   plan.PlannedRecords,
			CompletedUnits:   units,
			TotalUnits:       totalUnits,
			Stage:            stage,
		})
	}
	exclusions := generationExclusions{prompts: recordPrompts(records), examples: seeds, targets: recordTargets(records)}

	for index := completedBatches; index < totalBatches; index++ {
		if err := ctx.Err(); err != nil {
			return FullBuildArtifact{}, fmt.Errorf("build dataset: %w", err)
		}
		reportProgress(fmt.Sprintf("Generating batch %d of %d", index+1, totalBatches), completedUnits)
		var generated []Record
		var skipped int
		var batchMetrics previewMetrics
		var batchRejected []RejectedFact
		var err error
		progress := func(stage string) {
			reportProgress(stage, completedUnits+1)
		}
		if request.Goal == GoalQuestionAnswer {
			completeFacts := completeQuestionAnswerFacts(records)
			plannedBatches[index] = slices.DeleteFunc(slices.Clone(plannedBatches[index]), func(fact Record) bool {
				_, complete := completeFacts[fact.FactID]
				return complete
			})
			batchFactIDs := make(map[string]struct{}, len(plannedBatches[index]))
			for _, fact := range plannedBatches[index] {
				batchFactIDs[fact.FactID] = struct{}{}
			}
			rejectedFacts = slices.DeleteFunc(rejectedFacts, func(rejected RejectedFact) bool {
				_, retrying := batchFactIDs[rejected.FactID]
				return retrying
			})
			if len(plannedBatches[index]) > 0 {
				generated, batchRejected, batchMetrics, err = p.preparePlannedQuestionAnswerBatch(ctx, request, plannedBatches[index], exclusions, progress)
			}
			rejectedFacts = append(rejectedFacts, batchRejected...)
		} else {
			generated, skipped, batchMetrics, err = p.prepareFullBatch(ctx, request, batches[index], exclusions, progress)
		}
		metrics.add(batchMetrics)
		if err != nil {
			return FullBuildArtifact{}, fmt.Errorf("build dataset batch %d of %d: %w", index+1, totalBatches, err)
		}
		if err := validateNewRecords(records, generated); err != nil {
			return FullBuildArtifact{}, fmt.Errorf("build dataset batch %d of %d: %w", index+1, totalBatches, err)
		}
		records = append(records, generated...)
		if len(batchRejected) > 0 {
			exclusions.prompts = recordPrompts(records)
			exclusions.targets = recordTargets(records)
			if err := saveBuildCheckpoint(request.DatasetDirectory, buildCheckpointFrom(fingerprint, index, records, rejectedFacts, excludedEvidence, metrics)); err != nil {
				return FullBuildArtifact{}, err
			}
			if rejected := slices.IndexFunc(batchRejected, func(fact RejectedFact) bool { return fact.UserApproved }); rejected >= 0 {
				record := slices.IndexFunc(seeds, func(seed Record) bool { return seed.FactID == batchRejected[rejected].FactID })
				return FullBuildArtifact{}, fmt.Errorf("build dataset batch %d of %d: user-approved record %d remains unresolved after %d regeneration attempts; completed facts were checkpointed", index+1, totalBatches, record+1, plannedFactAttempts)
			}
			return FullBuildArtifact{}, fmt.Errorf("build dataset batch %d of %d: %d planned facts remain unresolved after %d regeneration attempts; completed facts were checkpointed", index+1, totalBatches, len(batchRejected), plannedFactAttempts)
		}
		completedUnits += 2
		completedBatches = index + 1
		excludedEvidence += skipped
		exclusions.prompts = recordPrompts(records)
		exclusions.targets = recordTargets(records)
		if err := saveBuildCheckpoint(request.DatasetDirectory, buildCheckpointFrom(fingerprint, index+1, records, rejectedFacts, excludedEvidence, metrics)); err != nil {
			return FullBuildArtifact{}, err
		}
		reportProgress(fmt.Sprintf("Completed batch %d of %d", index+1, totalBatches), completedUnits)
	}
	if request.Goal == GoalQuestionAnswer && len(rejectedFacts) > 0 {
		return FullBuildArtifact{}, fmt.Errorf("build dataset: checkpoint contains %d unresolved planned facts", len(rejectedFacts))
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
		reviewedRecordCount = 0
		for _, seed := range seeds {
			if slices.ContainsFunc(records, func(record Record) bool {
				return record.Prompt == seed.Prompt && record.FactID == seed.FactID
			}) {
				reviewedRecordCount++
			}
		}
		if len(records) == 0 {
			return FullBuildArtifact{}, fmt.Errorf("build dataset: no source fact produced %d verified records", questionAnswerRecordsPerFact)
		}
		reportProgress("Creating evaluation prompts", completedUnits)
		evaluation, evaluationMetrics, err := p.prepareQuestionAnswerEvaluation(ctx, request, records, generationExclusions{
			prompts: recordPrompts(records), examples: seeds, targets: recordTargets(records),
		})
		metrics.add(evaluationMetrics)
		if err != nil {
			return FullBuildArtifact{}, err
		}
		if err := validateNewRecords(records, evaluation); err != nil {
			return FullBuildArtifact{}, fmt.Errorf("validate evaluation records: %w", err)
		}
		records = append(records, evaluation...)
	}
	artifact := FullBuildArtifact{
		Records: records, SpareRecords: spareRecords, ReviewedRecordCount: reviewedRecordCount, GeneratedRecordCount: len(records) - reviewedRecordCount,
		GenerationAttempts: metrics.generationAttempts, VerificationAttempts: metrics.verificationAttempts,
		PromptTokens: metrics.promptTokens, CompletionTokens: metrics.completionTokens, ExcludedEvidence: excludedEvidence, RejectedFacts: rejectedFacts,
	}
	report := FullBuildReport{
		Version: datasetContract, CompletedAt: p.now().UTC(), Goal: request.Goal, Model: request.Model, DatasetInstructions: request.DatasetInstructions,
		TotalRecords: len(artifact.Records), ReviewedSeedInput: reviewedSeedInput, ReviewedSeedCount: artifact.ReviewedRecordCount, GeneratedCount: artifact.GeneratedRecordCount,
		GenerationAttempts: artifact.GenerationAttempts, VerificationAttempts: artifact.VerificationAttempts,
		PromptTokens: artifact.PromptTokens, CompletionTokens: artifact.CompletionTokens, ExcludedEvidence: artifact.ExcludedEvidence, RejectedFacts: artifact.RejectedFacts,
		SpareFacts: questionAnswerFactCount(spareRecords),
	}
	reportProgress("Finalizing dataset artifacts", totalUnits-1)
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
		for _, rejected := range document.Artifact.RejectedFacts {
			rejected.SourceDocument = document.Name
			artifact.RejectedFacts = append(artifact.RejectedFacts, rejected)
		}
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
	report.RejectedFacts = artifact.RejectedFacts
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
		if record.SourceChunkID != "" {
			record.SourceChunkID = document.ID + "-" + record.SourceChunkID
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
		generated, metrics, err = p.generateRecords(ctx, previewRequest, formatPassages(passages), passages, recordCount, exclusions, true, maxGenerationAttempts, 0, 0)
	}
	if err == nil && request.Goal == GoalQuestionAnswer && len(generated) == 0 {
		return nil, len(passages), metrics, nil
	}
	if err == nil && request.Goal == GoalQuestionAnswer {
		progress("Creating questions for grounded answers")
		expanded, expansionMetrics, expansionErr := p.expandQuestionAnswerRecords(ctx, request.Model, request.DatasetInstructions, generated, exclusions, 0)
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
			acceptedEvidence := make(map[string]struct{}, len(verified)/questionAnswerRecordsPerFact)
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

func (p *Previewer) preparePlannedQuestionAnswerBatch(ctx context.Context, request FullBuildRequest, facts []Record, exclusions generationExclusions, progress func(string)) ([]Record, []RejectedFact, previewMetrics, error) {
	pending := slices.Clone(facts)
	verified := make([]Record, 0, len(facts)*questionAnswerRecordsPerFact)
	acceptedByFact := make(map[string][]Record, len(facts))
	retryExclusions := exclusions
	var metrics previewMetrics
	for attempt := range plannedFactAttempts {
		if err := ctx.Err(); err != nil {
			return nil, nil, metrics, err
		}
		if attempt == 0 {
			progress("Creating questions for planned facts")
		} else {
			progress(fmt.Sprintf("Regenerating incomplete planned facts · attempt %d of %d", attempt+1, plannedFactAttempts))
		}
		expanded, expansionMetrics, err := p.expandQuestionAnswerRecords(ctx, request.Model, request.DatasetInstructions, pending, retryExclusions, attempt)
		metrics.add(expansionMetrics)
		if err != nil {
			if !isRecordQualityError(err) {
				return nil, nil, metrics, err
			}
			for _, fact := range pending {
				retryExclusions.feedback = append(retryExclusions.feedback, VerificationResult{
					ExpectedTarget: fact.Target,
					Reason:         err.Error(),
				})
			}
			if attempt == plannedFactAttempts-1 {
				break
			}
			continue
		}
		progress("Independently verifying planned records")
		accepted, rejected, verificationMetrics, err := p.verifyQuestionAnswerCandidates(ctx, request.Model, request.DatasetInstructions, expanded)
		metrics.add(verificationMetrics)
		if err != nil {
			return nil, nil, metrics, err
		}
		for _, record := range accepted {
			group := acceptedByFact[record.FactID]
			if slices.ContainsFunc(group, func(existing Record) bool {
				return strings.EqualFold(strings.TrimSpace(existing.Prompt), strings.TrimSpace(record.Prompt))
			}) {
				continue
			}
			acceptedByFact[record.FactID] = append(group, record)
		}
		next := make([]Record, 0, len(pending))
		for _, fact := range pending {
			group := acceptedByFact[fact.FactID]
			if len(group) < questionAnswerRecordsPerFact {
				next = append(next, fact)
				continue
			}
			verified = append(verified, finalizeQuestionAnswerGroup(fact, group)...)
			delete(acceptedByFact, fact.FactID)
		}
		if len(next) == 0 {
			return verified, nil, metrics, nil
		}
		retryExclusions.prompts = append(retryExclusions.prompts, recordPrompts(expanded)...)
		retryExclusions.feedback = append(retryExclusions.feedback, rejected...)
		pending = next
	}
	rejected := make([]RejectedFact, len(pending))
	for index, fact := range pending {
		rejected[index] = RejectedFact{
			SourceChunkID: fact.SourceChunkID,
			FactID:        fact.FactID,
			Prompt:        fact.Prompt,
			Target:        fact.Target,
			Evidence:      fact.Evidence,
			UserApproved:  fact.UserApproved,
			UserEdited:    fact.UserEdited,
			Reason:        fmt.Sprintf("did not produce a complete %d-record question group after %d attempts", questionAnswerRecordsPerFact, plannedFactAttempts),
		}
	}
	return verified, rejected, metrics, nil
}

func (p *Previewer) prepareQuestionAnswerEvaluation(ctx context.Context, request FullBuildRequest, training []Record, exclusions generationExclusions) ([]Record, previewMetrics, error) {
	facts := evaluationQuestionAnswerFacts(training)
	if len(facts) < questionAnswerTestRecords {
		return nil, previewMetrics{}, nil
	}

	prepare := func(selected []Record, validation bool, independent bool) ([]Record, previewMetrics, error) {
		generated, rejected, metrics, err := p.preparePlannedQuestionAnswerBatch(ctx, request, selected, exclusions, func(string) {})
		if err != nil {
			return nil, metrics, err
		}
		if len(rejected) > 0 {
			return nil, metrics, fmt.Errorf("build dataset: %d evaluation facts did not produce verified alternate questions", len(rejected))
		}
		for index := range generated {
			generated[index].ValidationPrompt = validation
			generated[index].IndependentPrompt = independent
			generated[index].UserApproved = false
			generated[index].UserEdited = false
		}
		return generated, metrics, nil
	}

	validation, metrics, err := prepare(facts[:questionAnswerValidationRecords], true, false)
	if err != nil {
		return nil, metrics, fmt.Errorf("prepare validation prompts: %w", err)
	}
	exclusions.prompts = append(exclusions.prompts, recordPrompts(validation)...)
	test, testMetrics, err := prepare(facts[:questionAnswerTestRecords], false, true)
	metrics.add(testMetrics)
	if err != nil {
		return nil, metrics, fmt.Errorf("prepare protected test prompts: %w", err)
	}
	return append(validation, test...), metrics, nil
}

func evaluationQuestionAnswerFacts(records []Record) []Record {
	byFact := make(map[string]Record, len(records))
	for _, record := range records {
		if record.FactID == "" || record.ValidationPrompt || record.IndependentPrompt {
			continue
		}
		if _, exists := byFact[record.FactID]; exists {
			continue
		}
		record.Prompt = ""
		record.UserApproved = false
		record.UserEdited = false
		byFact[record.FactID] = record
	}
	facts := make([]Record, 0, len(byFact))
	for _, record := range byFact {
		facts = append(facts, record)
	}
	slices.SortFunc(facts, func(left, right Record) int {
		return cmp.Compare(left.FactID, right.FactID)
	})
	return facts
}

func finalizeQuestionAnswerGroup(fact Record, candidates []Record) []Record {
	group := slices.Clone(candidates[:questionAnswerRecordsPerFact])
	for index := range group {
		group[index].ValidationPrompt = false
		group[index].IndependentPrompt = false
		group[index].UserApproved = fact.UserApproved && group[index].Prompt == fact.Prompt
		group[index].UserEdited = fact.UserEdited && group[index].Prompt == fact.Prompt
	}
	return group
}

func (p *Previewer) selectQuestionAnswerFacts(ctx context.Context, model, datasetInstructions string, passages []string, exclusions generationExclusions) ([]Record, previewMetrics, error) {
	targetCandidates := questionAnswerTargetCandidates(passages)
	if !slices.ContainsFunc(targetCandidates, func(candidates []string) bool { return len(candidates) > 0 }) {
		return nil, previewMetrics{}, nil
	}
	prioritizeQuestionAnswerTargets(targetCandidates, passages, exclusions.targets)
	return p.selectQuestionAnswerCandidates(ctx, model, datasetInstructions, passages, targetCandidates, exclusions)
}

func (p *Previewer) selectQuestionAnswerCandidates(ctx context.Context, model, datasetInstructions string, passages []string, targetCandidates [][]string, exclusions generationExclusions) ([]Record, previewMetrics, error) {
	return p.selectQuestionAnswerCandidatesWithContexts(ctx, model, datasetInstructions, passages, targetCandidates, nil, exclusions)
}

func (p *Previewer) selectQuestionAnswerCandidatesWithContexts(ctx context.Context, model, datasetInstructions string, passages []string, targetCandidates, candidateContexts [][]string, exclusions generationExclusions) ([]Record, previewMetrics, error) {
	if !slices.ContainsFunc(targetCandidates, func(candidates []string) bool { return len(candidates) > 0 }) {
		return nil, previewMetrics{}, nil
	}
	userPrompt := questionAnswerTargetPrompt(targetCandidates, exclusions.examples, exclusions.targets, datasetInstructions, candidateContexts)
	if len(userPrompt) > maxQuestionAnswerPromptBytes && len(exclusions.targets) > 0 {
		exclusions.targets = nil
		userPrompt = questionAnswerTargetPrompt(targetCandidates, exclusions.examples, nil, datasetInstructions, candidateContexts)
	}
	if len(userPrompt) > maxQuestionAnswerPromptBytes {
		if len(passages) > 1 {
			middle := len(passages) / 2
			left, leftMetrics, err := p.selectQuestionAnswerCandidatesWithContexts(ctx, model, datasetInstructions, passages[:middle], targetCandidates[:middle], sliceCandidateContexts(candidateContexts, 0, middle), exclusions)
			if err != nil {
				return nil, leftMetrics, err
			}
			right, rightMetrics, err := p.selectQuestionAnswerCandidatesWithContexts(ctx, model, datasetInstructions, passages[middle:], targetCandidates[middle:], sliceCandidateContexts(candidateContexts, middle, len(passages)), exclusions)
			leftMetrics.add(rightMetrics)
			return append(left, right...), leftMetrics, err
		}
		if len(targetCandidates[0]) > 1 {
			middle := len(targetCandidates[0]) / 2
			leftCandidates := [][]string{targetCandidates[0][:middle]}
			rightCandidates := [][]string{targetCandidates[0][middle:]}
			leftContexts, rightContexts := splitSingleCandidateContexts(candidateContexts, middle)
			left, leftMetrics, err := p.selectQuestionAnswerCandidatesWithContexts(ctx, model, datasetInstructions, passages, leftCandidates, leftContexts, exclusions)
			if err != nil {
				return nil, leftMetrics, err
			}
			right, rightMetrics, err := p.selectQuestionAnswerCandidatesWithContexts(ctx, model, datasetInstructions, passages, rightCandidates, rightContexts, exclusions)
			leftMetrics.add(rightMetrics)
			return append(left, right...), leftMetrics, err
		}
		return nil, previewMetrics{}, fmt.Errorf("select grounded answers: one candidate exceeds the %d-byte prompt budget", maxQuestionAnswerPromptBytes)
	}

	var metrics previewMetrics
	correction := ""
	var validationErr error
	var lastTargetIDs [][]int
	for attempt := 1; attempt <= maxTargetSelectionAttempts; attempt++ {
		metrics.generationAttempts++
		completion, err := p.completer.Complete(ctx, CompletionRequest{
			Model:          model,
			Messages:       generationMessages(userPrompt, correction),
			ResponseSchema: questionAnswerTargetSchema(targetCandidates),
			MaxTokens:      max(generationMaxTokens, len(passages)*32),
			Seed:           preparationGenerationSeed,
		})
		if err != nil {
			return nil, metrics, fmt.Errorf("select grounded answers: %w", err)
		}
		metrics.promptTokens += completion.PromptTokens
		metrics.completionTokens += completion.CompletionTokens

		var response struct {
			TargetIDs [][]int `json:"target_ids"`
		}
		if err := json.Unmarshal([]byte(completion.Content), &response); err != nil {
			validationErr = fmt.Errorf("decode selected answers: %w", err)
		} else {
			lastTargetIDs = response.TargetIDs
			records, err := validateQuestionAnswerTargets(response.TargetIDs, passages, targetCandidates)
			validationErr = err
			if validationErr == nil {
				return records, metrics, nil
			}
		}
		correction = fmt.Sprintf("Retry %d: the previous response failed validation: %s. Return target_ids with exactly %d arrays in passage order. Use only these target ID ranges: %s. Use an empty array for an unusable passage; do not omit its array.", attempt+1, validationErr, len(passages), questionAnswerTargetRanges(targetCandidates))
	}
	recovered, recoveryMetrics, err := p.recoverQuestionAnswerTargetsWithContexts(ctx, model, datasetInstructions, passages, targetCandidates, candidateContexts, exclusions, lastTargetIDs)
	metrics.add(recoveryMetrics)
	if err != nil {
		return nil, metrics, recordQualityError{err: fmt.Errorf("selected answers failed validation after %d attempts (%v) and passage recovery failed: %w", maxTargetSelectionAttempts, validationErr, err)}
	}
	return recovered, metrics, nil
}

func (p *Previewer) recoverQuestionAnswerTargets(ctx context.Context, model, datasetInstructions string, passages []string, targetCandidates [][]string, exclusions generationExclusions, lastTargetIDs [][]int) ([]Record, previewMetrics, error) {
	return p.recoverQuestionAnswerTargetsWithContexts(ctx, model, datasetInstructions, passages, targetCandidates, nil, exclusions, lastTargetIDs)
}

func (p *Previewer) recoverQuestionAnswerTargetsWithContexts(ctx context.Context, model, datasetInstructions string, passages []string, targetCandidates, candidateContexts [][]string, exclusions generationExclusions, lastTargetIDs [][]int) ([]Record, previewMetrics, error) {
	if len(passages) == 1 {
		if len(targetCandidates[0]) == 0 {
			return nil, previewMetrics{}, nil
		}
		if len(targetCandidates[0]) == 1 {
			// This text was extracted directly from the source and passed the
			// deterministic candidate filters. Retain it rather than lose source
			// coverage because the model returned an impossible numeric ID.
			target := targetCandidates[0][0]
			return []Record{{Target: target, Evidence: passages[0], FactID: recordFactID(passages[0], target)}}, previewMetrics{}, nil
		}

		middle := len(targetCandidates[0]) / 2
		leftContexts, rightContexts := splitSingleCandidateContexts(candidateContexts, middle)
		left, leftMetrics, err := p.selectQuestionAnswerCandidatesWithContexts(ctx, model, datasetInstructions, passages, [][]string{targetCandidates[0][:middle]}, leftContexts, exclusions)
		if err != nil {
			return nil, leftMetrics, err
		}
		right, rightMetrics, err := p.selectQuestionAnswerCandidatesWithContexts(ctx, model, datasetInstructions, passages, [][]string{targetCandidates[0][middle:]}, rightContexts, exclusions)
		leftMetrics.add(rightMetrics)
		return append(left, right...), leftMetrics, err
	}

	var records []Record
	var metrics previewMetrics
	for index := range passages {
		if len(lastTargetIDs) == len(passages) {
			selected, err := validateQuestionAnswerTargets([][]int{lastTargetIDs[index]}, passages[index:index+1], targetCandidates[index:index+1])
			if err == nil {
				records = append(records, selected...)
				continue
			}
		}
		selected, selectionMetrics, err := p.selectQuestionAnswerCandidatesWithContexts(ctx, model, datasetInstructions, passages[index:index+1], targetCandidates[index:index+1], sliceCandidateContexts(candidateContexts, index, index+1), exclusions)
		metrics.add(selectionMetrics)
		if err != nil {
			return nil, metrics, err
		}
		records = append(records, selected...)
	}
	return records, metrics, nil
}

func questionAnswerTargetPrompt(targetCandidates [][]string, examples []Record, previousTargets []string, datasetInstructions string, candidateContexts ...[][]string) string {
	var prompt strings.Builder
	targetDetailRequirement := "Choose the shortest target that completely expresses that fact."
	targetScopeRequirement := "When the central fact naturally asks for a name, function, method, value, signal, duration, or identifier, prefer that atomic answer over a sentence containing it. Choose a sentence when the useful fact is a behavior, comparison, condition, relationship, or effect, and retain every clause needed to express that proposition."
	if datasetInstructions != "" {
		prompt.WriteString(datasetInstructionsPrompt(datasetInstructions))
		prompt.WriteString("\n\n")
		targetDetailRequirement = "Choose a grounded target that completely expresses one useful fact."
		targetScopeRequirement = "Choose the target's scope and level of detail by following the user instruction."
	}
	fmt.Fprintf(&prompt, "Review %d source passages and select a focused set of distinct useful training answers supported by each passage. Do not write questions yet.\n", len(targetCandidates))
	fmt.Fprintf(&prompt, "Choose a minimal, non-overlapping set of distinct useful instructional facts that collectively covers each passage's central teachable content. Do not select every sentence or every candidate, and never select both a complete proposition and its shorter fragment as separate facts. %s Each target is a complete answer users will be trained to return. Do not choose an incidental package name, declaration, heading, biography, forward reference, page number, or example detail merely because it is shorter than the passage's useful behavior or principle. %s Prefer durable facts over one-run benchmark or profile measurements, and prefer new targets over ones already selected; these are preferences, so reuse the best available target rather than skipping an otherwise useful passage. Avoid targets containing unrelated surrounding text, unresolved words such as this or it, incomplete code declarations, or additional clauses. Preserve the exact role and capitalization of code symbols: a method name must not become a type, interface, package, or unrelated API. Do not merge facts from alternative packages or APIs. Return one target_id array per passage in passage order. Use an empty array for navigation, tables of contents, malformed tables, or presentation-only text only when that passage says VALID TARGET IDS: NONE; every passage with eligible targets must select at least one so source coverage is not silently lost.\n", targetDetailRequirement, targetScopeRequirement)
	for index, example := range examples {
		fmt.Fprintf(&prompt, "\n[APPROVED EXAMPLE %d]\nTARGET: %s\n", index+1, example.Target)
	}
	if len(previousTargets) > 0 {
		prompt.WriteString("\n[PREVIOUSLY SELECTED TARGETS — PREFER A DIFFERENT FACT WHEN AVAILABLE]\n")
		start := max(len(previousTargets)-maxRenderedPreviousTargets, 0)
		for _, target := range previousTargets[start:] {
			fmt.Fprintf(&prompt, "- %s\n", target)
		}
	}
	prompt.WriteString("\nSOURCE:\n")
	prompt.WriteString(formatQuestionAnswerTargetPassages(targetCandidates, candidateContexts...))
	return prompt.String()
}

func questionAnswerTargetCandidates(passages []string) [][]string {
	candidates := make([][]string, len(passages))
	for passageIndex, passage := range passages {
		for _, target := range quoteCandidates(passage) {
			reason := plannedTargetRejectionReason(target)
			if usableQuestionAnswerTarget(target) && reason != "presentation" && reason != "navigation" {
				candidates[passageIndex] = append(candidates[passageIndex], target)
			}
		}
	}
	return candidates
}

func prioritizeQuestionAnswerTargets(candidates [][]string, passages, previousTargets []string) {
	prioritizeQuestionAnswerTargetsWithContexts(candidates, nil, passages, previousTargets)
}

func prioritizeQuestionAnswerTargetsWithContexts(candidates, contexts [][]string, passages, previousTargets []string) {
	previous := make(map[string]struct{}, len(previousTargets))
	for _, target := range previousTargets {
		previous[normalizedTarget(target)] = struct{}{}
	}
	for index := range candidates {
		type contextualCandidate struct {
			target  string
			context string
		}
		ordered := make([]contextualCandidate, len(candidates[index]))
		for candidateIndex, target := range candidates[index] {
			ordered[candidateIndex].target = target
			if index < len(contexts) && candidateIndex < len(contexts[index]) {
				ordered[candidateIndex].context = contexts[index][candidateIndex]
			}
		}
		slices.SortStableFunc(ordered, func(left, right contextualCandidate) int {
			return cmp.Compare(questionAnswerTargetPreference(left.target, passages[index], previous), questionAnswerTargetPreference(right.target, passages[index], previous))
		})
		for candidateIndex, candidate := range ordered {
			candidates[index][candidateIndex] = candidate.target
			if index < len(contexts) && candidateIndex < len(contexts[index]) {
				contexts[index][candidateIndex] = candidate.context
			}
		}
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

func formatQuestionAnswerTargetPassages(candidates [][]string, candidateContexts ...[][]string) string {
	var builder strings.Builder
	for passageIndex, targets := range candidates {
		if len(targets) == 0 {
			fmt.Fprintf(&builder, "[PASSAGE %d — VALID TARGET IDS: NONE]\n", passageIndex+1)
			builder.WriteString("[NO USABLE TARGETS: RETURN AN EMPTY ARRAY FOR THIS PASSAGE]\n")
		} else {
			fmt.Fprintf(&builder, "[PASSAGE %d — VALID TARGET IDS: 1-%d]\n", passageIndex+1, len(targets))
		}
		for targetIndex, target := range targets {
			if context := candidateContext(candidateContexts, passageIndex, targetIndex); context != "" && context != target {
				fmt.Fprintf(&builder, "[CONTEXT FOR TARGET %d — NOT SELECTABLE] %s\n", targetIndex+1, context)
			}
			fmt.Fprintf(&builder, "[TARGET %d] %s\n", targetIndex+1, target)
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func candidateContext(contexts [][][]string, passageIndex, targetIndex int) string {
	if len(contexts) == 0 || passageIndex >= len(contexts[0]) || targetIndex >= len(contexts[0][passageIndex]) {
		return ""
	}
	return strings.TrimSpace(contexts[0][passageIndex][targetIndex])
}

func sliceCandidateContexts(contexts [][]string, start, end int) [][]string {
	if len(contexts) == 0 {
		return nil
	}
	return contexts[start:end]
}

func splitSingleCandidateContexts(contexts [][]string, middle int) ([][]string, [][]string) {
	if len(contexts) == 0 {
		return nil, nil
	}
	return [][]string{contexts[0][:middle]}, [][]string{contexts[0][middle:]}
}

func questionAnswerTargetRanges(candidates [][]string) string {
	ranges := make([]string, len(candidates))
	for index, targets := range candidates {
		if len(targets) == 0 {
			ranges[index] = fmt.Sprintf("passage %d = none", index+1)
			continue
		}
		ranges[index] = fmt.Sprintf("passage %d = 1-%d", index+1, len(targets))
	}
	return strings.Join(ranges, ", ")
}

func questionAnswerTargetSchema(candidates [][]string) map[string]any {
	maximumID := 1
	for _, targets := range candidates {
		maximumID = max(maximumID, len(targets))
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target_ids": map[string]any{
				"type": "array", "minItems": len(candidates), "maxItems": len(candidates),
				"items": map[string]any{
					"type": "array", "uniqueItems": true,
					"items": map[string]any{"type": "integer", "minimum": 1, "maximum": maximumID},
				},
			},
		},
		"required": []string{"target_ids"},
	}
}

func validateQuestionAnswerTargets(targetIDs [][]int, passages []string, targetCandidates [][]string) ([]Record, error) {
	if len(targetIDs) != len(passages) || len(targetCandidates) != len(passages) {
		return nil, fmt.Errorf("model returned selections for %d of %d passages", len(targetIDs), len(passages))
	}

	var records []Record
	for index, selectedIDs := range targetIDs {
		if len(targetCandidates[index]) > 0 && len(selectedIDs) == 0 {
			return nil, fmt.Errorf("passage %d omitted all %d eligible targets", index+1, len(targetCandidates[index]))
		}
		seen := make(map[int]bool, len(selectedIDs))
		for _, targetID := range selectedIDs {
			if targetID < 1 || targetID > len(targetCandidates[index]) {
				return nil, fmt.Errorf("passage %d has invalid target_id %d", index+1, targetID)
			}
			if seen[targetID] {
				return nil, fmt.Errorf("passage %d repeats target_id %d", index+1, targetID)
			}
			seen[targetID] = true
			target := targetCandidates[index][targetID-1]
			evidence := passages[index]
			records = append(records, Record{Target: target, Evidence: evidence, FactID: recordFactID(evidence, target)})
		}
	}
	return records, nil
}

type generatedQuestionPool struct {
	RecordNumber int      `json:"record_number"`
	Questions    []string `json:"questions"`
}

func (p *Previewer) verifyQuestionAnswerRecords(ctx context.Context, model, datasetInstructions string, seeds, candidates []Record) ([]Record, previewMetrics, error) {
	passed, _, metrics, err := p.verifyQuestionAnswerCandidates(ctx, model, datasetInstructions, candidates)
	if err != nil {
		return nil, metrics, err
	}
	return selectReviewedQuestionAnswerGroups(seeds, passed), metrics, nil
}

func (p *Previewer) verifyQuestionAnswerCandidates(ctx context.Context, model, datasetInstructions string, candidates []Record) ([]Record, []VerificationResult, previewMetrics, error) {
	var metrics previewMetrics
	passed := make([]Record, 0, len(candidates))
	var rejected []VerificationResult
	if len(candidates) > 0 {
		verification, verificationMetrics, err := p.verifyRecords(ctx, model, GoalQuestionAnswer, datasetInstructions, candidates)
		metrics.add(verificationMetrics)
		if err != nil {
			return nil, nil, metrics, fmt.Errorf("build dataset: verify question pools: %w", err)
		}
		verified, _ := passingRecords(candidates, verification)
		passed = append(passed, verified...)
		rejected = failedVerification(verification)
	}
	return passed, rejected, metrics, nil
}

func selectReviewedQuestionAnswerGroups(seeds, records []Record) []Record {
	completeFacts := completeQuestionAnswerFacts(records)
	selected := make([]Record, 0, len(completeFacts)*questionAnswerRecordsPerFact)
	for _, seed := range seeds {
		if _, complete := completeFacts[seed.FactID]; !complete {
			continue
		}
		regular := 0
		for _, record := range records {
			if record.FactID != seed.FactID || record.ValidationPrompt || record.IndependentPrompt || regular == questionAnswerRecordsPerFact {
				continue
			}
			selected = append(selected, record)
			regular++
		}
		for _, record := range records {
			if record.FactID == seed.FactID && record.ValidationPrompt {
				selected = append(selected, record)
				break
			}
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

func (p *Previewer) expandQuestionAnswerRecords(ctx context.Context, model, datasetInstructions string, facts []Record, exclusions generationExclusions, attempt int) ([]Record, previewMetrics, error) {
	var metrics previewMetrics
	metrics.generationAttempts++
	messages := questionPoolMessagesExcluding(facts, datasetInstructions, exclusions.prompts, exclusions.feedback)
	if attempt > 0 {
		messages[1].Content += "\n\n[RETRY OUTPUT REQUIREMENTS]\nGenerate a fresh response that exactly matches the required JSON schema. Return JSON only. Escape every newline and other control character inside JSON strings; do not place literal line breaks inside a quoted string."
	}
	completion, err := p.completer.Complete(ctx, CompletionRequest{
		Model:          model,
		Messages:       messages,
		ResponseSchema: questionPoolSchema(len(facts)),
		MaxTokens:      max(generationMaxTokens, len(facts)*questionCandidatePoolSize*96),
		Seed:           preparationGenerationSeed + uint32(attempt),
	})
	if err != nil {
		return nil, metrics, fmt.Errorf("build question pool: %w", err)
	}
	metrics.promptTokens += completion.PromptTokens
	metrics.completionTokens += completion.CompletionTokens

	pools, err := decodeQuestionPools(completion.Content, len(facts))
	if err != nil {
		return nil, metrics, recordQualityError{err: fmt.Errorf("decode question pool: %w", err)}
	}
	expanded := selectQuestionPools(facts, pools, exclusions.prompts)
	if len(expanded) == 0 {
		return nil, metrics, recordQualityError{err: fmt.Errorf("no source fact produced usable alternate questions: %s", questionPoolFailureReason(facts, pools, exclusions.prompts))}
	}
	return expanded, metrics, nil
}

func questionPoolFailureReason(facts []Record, pools []generatedQuestionPool, excludedPrompts []string) string {
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
	for index, fact := range facts {
		questions := byRecord[index+1]
		if len(questions) == 0 {
			return fmt.Sprintf("locked fact %d received no question", index+1)
		}
		for _, question := range questions {
			question = strings.TrimSpace(question)
			if _, found := excluded[strings.ToLower(question)]; found {
				return fmt.Sprintf("locked fact %d repeated a previously rejected question", index+1)
			}
			if err := validateNaturalQuestion(question); err != nil {
				return fmt.Sprintf("locked fact %d question %q is invalid: %v", index+1, question, err)
			}
			if err := validateRecordShape(GoalQuestionAnswer, question, fact.Target); err != nil {
				return fmt.Sprintf("locked fact %d question %q has invalid shape: %v", index+1, question, err)
			}
		}
	}
	return "generated questions were not sufficiently distinct"
}

func decodeQuestionPools(content string, recordCount int) ([]generatedQuestionPool, error) {
	var response map[string]string
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, err
	}
	pools := make([]generatedQuestionPool, 0, recordCount)
	for index := range recordCount {
		name := fmt.Sprintf("record_%d", index+1)
		question, exists := response[name]
		if !exists {
			continue
		}
		pools = append(pools, generatedQuestionPool{RecordNumber: index + 1, Questions: []string{question}})
	}
	if len(pools) == 0 {
		return nil, fmt.Errorf("model generated no recognized question fields")
	}
	return pools, nil
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
	selected := make([]Record, 0, len(facts)*questionAnswerRecordsPerFact)
	for index, fact := range facts {
		prompts := make([]string, 0, questionCandidatePoolSize)
		if fact.Prompt != "" {
			prompts = append(prompts, fact.Prompt)
		}
		candidates := validQuestionCandidates(fact, byRecord[index+1], excluded)
		preferred := slices.DeleteFunc(slices.Clone(candidates), func(candidate string) bool {
			return questionRepeatsAtomicTarget(candidate, fact.Target)
		})
		if len(preferred) >= questionAnswerRecordsPerFact-len(prompts) {
			candidates = preferred
		}
		prompts = append(prompts, selectDistinctQuestions(prompts, candidates, questionAnswerRecordsPerFact-len(prompts), fact.Target)...)
		if len(prompts) != questionAnswerRecordsPerFact {
			continue
		}
		for _, prompt := range prompts {
			selected = append(selected, Record{
				Prompt: prompt, Target: fact.Target, Evidence: fact.Evidence, FactID: fact.FactID, SourceChunkID: fact.SourceChunkID,
				UserApproved: fact.UserApproved && prompt == fact.Prompt,
				UserEdited:   fact.UserEdited && prompt == fact.Prompt,
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
	return questionPoolMessagesExcluding(facts, datasetInstructions, nil)
}

func questionPoolMessagesExcluding(facts []Record, datasetInstructions string, excludedPrompts []string, feedback ...[]VerificationResult) []Message {
	var prompt strings.Builder
	if datasetInstructions != "" {
		prompt.WriteString(datasetInstructionsPrompt(datasetInstructions))
		prompt.WriteString("\n\n")
	}
	styleRequirements := "Write one direct, natural question for the locked fact."
	if datasetInstructions == "" {
		styleRequirements = "Write one concise, direct question with the subject and necessary context named."
	}
	prompt.WriteString("Write exactly one natural question for each locked fact. Return it in the matching required field: LOCKED FACT 1 uses record_1, LOCKED FACT 2 uses record_2, and so on. ")
	prompt.WriteString(styleRequirements)
	prompt.WriteString(" For an identifier answer, describe what the code does without repeating words contained in that identifier.\n")
	prompt.WriteString("Every question must stand alone, end with a question mark, and be answered exactly and completely by the locked target and evidence. Match the question's scope to the complete target: if the target is a sentence, ask for its entire proposition rather than one name or detail embedded in it; if the question asks only for a name or value, the locked target must be only that name or value. Name the subject needed to understand the question. Never use context-dependent wording such as this program, this function, this type, this example, it, or they unless the question itself uniquely identifies what those words mean. Preserve the target's answer type, direction, scope, code-symbol role, package, and capitalization. Never reveal the answer, reverse the relationship, combine alternate APIs, invent context, or infer a stronger effect than the evidence states; registering handlers, for example, does not start a server. Do not refer to provided evidence, code, examples, output, text, documents, listings, sections, or tables. Return only the required record_N fields.\n")
	if len(excludedPrompts) > 0 {
		prompt.WriteString("\n[QUESTIONS TO AVOID — WRITE DIFFERENT QUESTIONS]\n")
		start := max(0, len(excludedPrompts)-maxRenderedPreviousPrompts)
		for _, previous := range excludedPrompts[start:] {
			fmt.Fprintf(&prompt, "- %s\n", previous)
		}
	}
	if len(feedback) > 0 && len(feedback[0]) > 0 {
		prompt.WriteString("\n[VERIFICATION FEEDBACK — CORRECT THESE FAILURES]\n")
		start := max(0, len(feedback[0])-maximumFactsPerBatch)
		for _, result := range feedback[0][start:] {
			fmt.Fprintf(&prompt, "- Locked target %q was rejected: %s\n", result.ExpectedTarget, result.Reason)
		}
	}
	evidence, evidenceIDs := sharedEvidenceReferences(facts)
	prompt.WriteString("\nEach locked fact references one numbered evidence block. Use only that block to write its question.\n")
	for index, source := range evidence {
		fmt.Fprintf(&prompt, "\n[SOURCE EVIDENCE %d]\n%s\n", index+1, source)
	}
	for index, fact := range facts {
		fmt.Fprintf(&prompt, "\n[LOCKED FACT %d]\nEVIDENCE BLOCK: %d\nLOCKED TARGET: %s\n", index+1, evidenceIDs[index], fact.Target)
	}
	prompt.WriteString("\n[FINAL QUESTION CHECK — REQUIRED]\nWrite each question as if the reader cannot see this preparation prompt. Never mention or allude to a source, evidence, evidence block, passage, document, text, section, listing, table, code block, or provided material. A user style request for a large, big, detailed, or contextual question never overrides this standalone requirement and never permits asking for information beyond the complete locked target. Before returning JSON, replace any wording such as 'according to the evidence' with a direct subject-specific question.\n")
	return []Message{
		{Role: "system", Content: "You create natural, meaningfully distinct questions for locked source-grounded facts. Never change or add facts. The user's instruction controls how every question is written and must be followed. Only source grounding, factual correctness, and the required output structure take priority over the user's instruction."},
		{Role: "user", Content: strings.TrimSpace(prompt.String())},
	}
}

func questionPoolSchema(recordCount int) map[string]any {
	properties := make(map[string]any, recordCount)
	required := make([]string, recordCount)
	for index := range recordCount {
		name := fmt.Sprintf("record_%d", index+1)
		properties[name] = map[string]any{"type": "string"}
		required[index] = name
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
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

func questionAnswerFactCount(records []Record) int {
	return len(completeQuestionAnswerFacts(records))
}

func completeQuestionAnswerFacts(records []Record) map[string]struct{} {
	counts := make(map[string]int, len(records))
	for _, record := range records {
		if record.FactID == "" || record.ValidationPrompt || record.IndependentPrompt {
			continue
		}
		counts[record.FactID]++
	}
	complete := make(map[string]struct{}, len(counts))
	for factID, count := range counts {
		if count >= questionAnswerRecordsPerFact {
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
	}{Version: datasetFingerprintVersion, Goal: request.Goal, Model: request.Model, DatasetInstructions: request.DatasetInstructions, Seeds: seeds})
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
	if json.Unmarshal(data, &checkpoint) != nil || checkpoint.Version != datasetContract || checkpoint.PlanVersion != datasetPlanVersion || checkpoint.Fingerprint != fingerprint || checkpoint.CompletedBatches < 0 || checkpoint.CompletedBatches > totalBatches {
		return buildCheckpoint{}, false
	}
	if _, err := ValidateReviewedRecords(checkpoint.Records); err != nil {
		return buildCheckpoint{}, false
	}
	return checkpoint, true
}

func buildCheckpointFrom(fingerprint string, completed int, records []Record, rejected []RejectedFact, excluded int, metrics previewMetrics) buildCheckpoint {
	return buildCheckpoint{
		Version: datasetContract, PlanVersion: datasetPlanVersion, Fingerprint: fingerprint, CompletedBatches: completed,
		Records: records, RejectedFacts: rejected, ExcludedEvidence: excluded,
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
		if record.UserApproved {
			record.Target = strings.TrimSpace(record.Target)
		} else {
			record.Target = strings.TrimSpace(normalizeSourceText(record.Target))
		}
		record.Evidence = strings.TrimSpace(normalizeSourceText(record.Evidence))
		if utf8.RuneCountInString(record.Prompt) < 3 || record.Target == "" || record.Evidence == "" {
			return nil, fmt.Errorf("record %d has an empty or unusable prompt, target, or evidence", index+1)
		}
		if !record.UserApproved {
			candidates := quoteCandidates(record.Evidence)
			if canonical := cleanProse(record.Target); slices.Contains(candidates, canonical) {
				record.Target = canonical
			}
			grounded := slices.Contains(candidates, record.Target)
			if !grounded {
				return nil, fmt.Errorf("record %d target is not an exact quote candidate from its evidence", index+1)
			}
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
	for _, record := range append(append([]Record(nil), existing...), added...) {
		prompt := strings.ToLower(strings.TrimSpace(record.Prompt))
		if _, duplicate := prompts[prompt]; duplicate {
			return fmt.Errorf("duplicate prompt %q", record.Prompt)
		}
		prompts[prompt] = struct{}{}
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
	targets := make([]string, 0, len(records)/questionAnswerRecordsPerFact)
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

func questionAnswerFactBatches(facts []Record, datasetInstructions string) [][]Record {
	var batches [][]Record
	for len(facts) > 0 {
		count := 1
		for count < len(facts) && count < maximumFactsPerBatch {
			candidate := facts[:count+1]
			if len(questionPoolMessages(candidate, datasetInstructions)[1].Content) > questionPoolBatchBytes {
				break
			}
			count++
		}
		batches = append(batches, facts[:count])
		facts = facts[count:]
	}
	return batches
}
