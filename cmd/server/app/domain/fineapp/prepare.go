package fineapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

const (
	maxDocumentUploadBytes   = 100 << 20
	maxDocumentUploads       = 10
	maxCollectionUploadBytes = 500 << 20
	maxMultipartOverhead     = maxDocumentUploads << 20
)

func (a *app) uploadDataset(ctx context.Context, r *http.Request) web.Encoder {
	r.Body = http.MaxBytesReader(nil, r.Body, maxCollectionUploadBytes+maxMultipartOverhead)
	if err := r.ParseMultipartForm(maxDocumentUploadBytes); err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("parse multipart form: %w", err))
	}
	defer r.MultipartForm.RemoveAll()

	headers := append([]*multipart.FileHeader(nil), r.MultipartForm.File["files"]...)
	headers = append(headers, r.MultipartForm.File["file"]...)
	if len(headers) == 0 {
		return errs.Errorf(errs.InvalidArgument, "at least one file form field is required")
	}
	if len(headers) > maxDocumentUploads {
		return errs.Errorf(errs.InvalidArgument, "no more than %d documents may be prepared together", maxDocumentUploads)
	}

	sourceNames := make([]string, len(headers))
	extensions := make([]string, len(headers))
	for index, header := range headers {
		if header.Size > maxDocumentUploadBytes {
			return errs.Errorf(errs.InvalidArgument, "%q exceeds the 100 MiB limit", header.Filename)
		}
		sourceNames[index] = filepath.Base(strings.ReplaceAll(header.Filename, `\`, "/"))
		extensions[index] = strings.ToLower(filepath.Ext(sourceNames[index]))
		if extensions[index] != ".pdf" && extensions[index] != ".docx" && extensions[index] != ".jsonl" {
			return errs.Errorf(errs.InvalidArgument, "%q must be a PDF, DOCX, or prepared JSONL dataset", sourceNames[index])
		}
	}
	if slices.Contains(extensions, ".jsonl") && len(headers) != 1 {
		return errs.Errorf(errs.InvalidArgument, "a prepared JSONL dataset cannot be combined with other files")
	}

	temporaryDirectory, err := os.MkdirTemp("", "moxie-documents-")
	if err != nil {
		return errs.New(errs.Internal, fmt.Errorf("create temporary upload directory: %w", err))
	}
	defer os.RemoveAll(temporaryDirectory)
	temporaryPaths := make([]string, len(headers))
	var totalWritten int64
	for index, header := range headers {
		documentDirectory := filepath.Join(temporaryDirectory, fmt.Sprintf("%03d", index+1))
		if err := os.Mkdir(documentDirectory, 0o700); err != nil {
			return errs.New(errs.Internal, fmt.Errorf("create temporary document directory: %w", err))
		}
		temporaryPaths[index] = filepath.Join(documentDirectory, sourceNames[index])
		written, err := storeMultipartUpload(header, temporaryPaths[index])
		if err != nil {
			return err
		}
		totalWritten += written
		if totalWritten > maxCollectionUploadBytes {
			return errs.Errorf(errs.InvalidArgument, "selected documents exceed the 500 MiB combined limit")
		}
	}
	if extensions[0] == ".jsonl" {
		operationContext, operationID, err := a.preparations.start(ctx, "prepared-upload", "Validating prepared dataset", 2)
		if err != nil {
			return errs.New(errs.FailedPrecondition, err)
		}
		if _, err := finetune.ValidateDatasetContract(temporaryPaths[0]); err != nil {
			a.preparations.finish(operationID, err)
			return errs.New(errs.InvalidArgument, fmt.Errorf("validate prepared dataset: %w", err))
		}
		if err := context.Cause(operationContext); err != nil {
			a.preparations.finish(operationID, err)
			return errs.FromSDK(fmt.Errorf("validate prepared dataset: %w", err))
		}
		a.preparations.update(operationID, "Storing prepared dataset", 1, 2, 0)
		datasetID, err := a.importPreparedDataset(temporaryPaths[0])
		if err != nil {
			a.preparations.finish(operationID, err)
			return errs.New(errs.Internal, fmt.Errorf("import prepared dataset: %w", err))
		}
		a.preparations.update(operationID, "Prepared dataset ready", 2, 2, 0)
		a.preparations.finish(operationID, nil)
		return jsonResponse{value: DatasetResponse{DatasetID: datasetID, SourceName: sourceNames[0], SourceNames: sourceNames}}
	}
	if a.preparer == nil {
		return errs.Errorf(errs.FailedPrecondition, "document preparation is not configured")
	}

	operationContext, operationID, err := a.preparations.start(ctx, "extraction", "Preparing source documents", len(temporaryPaths))
	if err != nil {
		return errs.New(errs.FailedPrecondition, err)
	}
	a.preparations.initializeDocuments(operationID, sourceNames)
	artifact, err := a.preparer.ExtractDocuments(operationContext, temporaryPaths, func(progress prepare.ExtractionProgress) {
		a.preparations.updateDocument(operationID, progress)
	})
	if err != nil {
		a.preparations.finish(operationID, err)
		return errs.FromSDK(fmt.Errorf("prepare uploaded documents: %w", err))
	}
	a.preparations.update(operationID, "Source extraction complete", len(temporaryPaths), len(temporaryPaths), 0)
	a.preparations.finish(operationID, nil)

	return jsonResponse{value: DatasetResponse{
		DatasetID:   artifact.ID,
		SourceName:  sourceNames[0],
		SourceNames: sourceNames,
		Documents:   documentResponsesFromNames(sourceNames),
	}}
}

func documentResponsesFromNames(names []string) []DatasetDocumentResponse {
	documents := make([]DatasetDocumentResponse, len(names))
	for index, name := range names {
		documents[index] = DatasetDocumentResponse{ID: fmt.Sprintf("%03d", index+1), Name: name}
	}
	return documents
}

func storeMultipartUpload(header *multipart.FileHeader, destination string) (int64, *errs.Error) {
	file, err := header.Open()
	if err != nil {
		return 0, errs.New(errs.InvalidArgument, fmt.Errorf("open uploaded file %q: %w", header.Filename, err))
	}
	defer file.Close()

	temporary, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, errs.New(errs.Internal, fmt.Errorf("create temporary upload: %w", err))
	}
	written, copyErr := io.Copy(temporary, io.LimitReader(file, maxDocumentUploadBytes+1))
	closeErr := temporary.Close()
	if copyErr != nil {
		return 0, errs.New(errs.Internal, fmt.Errorf("store temporary upload: %w", copyErr))
	}
	if closeErr != nil {
		return 0, errs.New(errs.Internal, fmt.Errorf("close temporary upload: %w", closeErr))
	}
	if written > maxDocumentUploadBytes {
		return 0, errs.Errorf(errs.InvalidArgument, "%q exceeds the 100 MiB limit", header.Filename)
	}
	return written, nil
}

func (a *app) importPreparedDataset(sourcePath string) (string, error) {
	if a.datasetsPath == "" {
		return "", fmt.Errorf("dataset storage is not configured")
	}
	if err := os.MkdirAll(a.datasetsPath, 0o700); err != nil {
		return "", fmt.Errorf("create dataset storage: %w", err)
	}
	if err := os.Chmod(a.datasetsPath, 0o700); err != nil {
		return "", fmt.Errorf("secure dataset storage: %w", err)
	}

	staging, err := os.MkdirTemp(a.datasetsPath, ".importing-")
	if err != nil {
		return "", fmt.Errorf("create dataset staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(staging)
		}
	}()

	destination := filepath.Join(staging, "dataset.jsonl")
	if err := copyPreparedDataset(sourcePath, destination); err != nil {
		return "", err
	}
	id, err := newPreparedDatasetID()
	if err != nil {
		return "", err
	}
	if err := os.Rename(staging, filepath.Join(a.datasetsPath, id)); err != nil {
		return "", fmt.Errorf("publish prepared dataset: %w", err)
	}
	published = true
	return id, nil
}

func copyPreparedDataset(sourcePath, destination string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open prepared dataset: %w", err)
	}
	defer source.Close()

	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create managed dataset: %w", err)
	}
	if _, err := io.Copy(file, source); err != nil {
		_ = file.Close()
		return fmt.Errorf("copy prepared dataset: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close managed dataset: %w", err)
	}
	return nil
}

func newPreparedDatasetID() (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("create dataset id: %w", err)
	}
	return "prepared-" + hex.EncodeToString(random), nil
}

func (a *app) preparationModels(_ context.Context, _ *http.Request) web.Encoder {
	modelIDs, err := a.preparationModelIDs()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return jsonResponse{value: PreparationModelsResponse{Models: modelIDs}}
}

func (a *app) currentPreparation(_ context.Context, _ *http.Request) web.Encoder {
	progress, exists := a.preparations.currentStatus()
	if !exists {
		return errs.Errorf(errs.NotFound, "no dataset preparation operation exists")
	}
	return jsonResponse{value: progress}
}

func (a *app) stopPreparation(_ context.Context, _ *http.Request) web.Encoder {
	if err := a.preparations.stop(); err != nil {
		return errs.New(errs.FailedPrecondition, err)
	}
	return jsonResponse{value: ActionResponse{Status: preparationStopping}}
}

func (a *app) previewDataset(ctx context.Context, r *http.Request) web.Encoder {
	if a.builder == nil {
		return errs.Errorf(errs.FailedPrecondition, "dataset preparation is not configured")
	}

	var req PreviewRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	if err := a.validatePreparationModel(req.ModelID); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	datasetMarkdown, err := a.datasetFile(req.DatasetID, "extracted.md")
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	documents, err := selectDatasetDocuments(filepath.Dir(datasetMarkdown), req.DocumentIDs)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	goal, _ := goalToBusiness(req.Goal)
	operationContext, operationID, err := a.preparations.start(ctx, "preview", "Preparing document previews", len(documents))
	if err != nil {
		return errs.New(errs.FailedPrecondition, err)
	}
	a.preparations.initializeDocumentDetails(operationID, datasetDocumentIDs(documents), datasetDocumentNames(documents))

	response := PreviewResponse{DatasetID: req.DatasetID, Documents: make([]DocumentPreviewResponse, 0, len(documents))}
	for index, document := range documents {
		a.preparations.updateDocumentWork(operationID, index, documentProcessing, "Generating preview", 0, 0, 0, 0, 0, nil)
		artifact, buildErr := a.builder.Build(operationContext, prepare.PreviewRequest{
			DatasetDirectory:    document.Directory,
			Goal:                goal,
			Model:               req.ModelID,
			RecordCount:         req.RecordCount,
			DatasetInstructions: strings.TrimSpace(req.DatasetInstructions),
			Progress: func(progress prepare.PreviewProgress) {
				a.preparations.updateDocumentWork(operationID, index, documentProcessing, progress.Stage, 0, 0, 0, progress.CompletedStages, progress.TotalStages, nil)
			},
		})
		if buildErr != nil {
			a.preparations.updateDocumentWork(operationID, index, documentFailed, "Preview failed", 0, 0, 0, 0, 0, buildErr)
			a.preparations.finish(operationID, buildErr)
			return errs.FromSDK(fmt.Errorf("build preview for %q: %w", document.Name, buildErr))
		}
		documentResponse := DocumentPreviewResponse{
			ID: document.ID, Name: document.Name,
			Records: recordsFromBusiness(artifact.Records), Verification: verificationFromBusiness(artifact.Verification),
			GenerationAttempts: artifact.GenerationAttempts, VerificationAttempts: artifact.VerificationAttempts,
			PromptTokens: artifact.PromptTokens, CompletionTokens: artifact.CompletionTokens,
		}
		response.Documents = append(response.Documents, documentResponse)
		response.GenerationAttempts += artifact.GenerationAttempts
		response.VerificationAttempts += artifact.VerificationAttempts
		response.PromptTokens += artifact.PromptTokens
		response.CompletionTokens += artifact.CompletionTokens
		a.preparations.updateDocumentWork(operationID, index, documentCompleted, "Preview ready", len(artifact.Records), 0, 0, 2, 2, nil)
	}
	a.preparations.finish(operationID, nil)
	response.Records = response.Documents[0].Records
	response.Verification = response.Documents[0].Verification
	return jsonResponse{value: response}
}

func (a *app) buildDataset(ctx context.Context, r *http.Request) web.Encoder {
	if a.builder == nil {
		return errs.Errorf(errs.FailedPrecondition, "dataset preparation is not configured")
	}

	var req BuildRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	if err := a.validatePreparationModel(req.ModelID); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	datasetMarkdown, err := a.datasetFile(req.DatasetID, "extracted.md")
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	datasetDirectory := filepath.Dir(datasetMarkdown)
	documents, err := prepare.DatasetDocuments(datasetDirectory)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	jobs, err := resolveDocumentBuilds(documents, req)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	goal, _ := goalToBusiness(req.Goal)
	targetFacts := 0
	if goal == prepare.GoalQuestionAnswer {
		targetFacts = finetune.MinimumKnowledgeFacts
	}
	operationContext, operationID, err := a.preparations.start(ctx, "build", "Preparing document builds", len(jobs))
	if err != nil {
		return errs.New(errs.FailedPrecondition, err)
	}
	a.preparations.initializeDocumentDetails(operationID, documentBuildIDs(jobs), documentBuildNames(jobs))

	built := make([]prepare.FullBuildDocument, 0, len(jobs))
	documentResponses := make([]DocumentBuildResponse, 0, len(jobs))
	for index, job := range jobs {
		a.preparations.updateDocumentWork(operationID, index, documentProcessing, "Preparing document build", 0, 0, targetFacts, 0, 0, nil)
		artifact, buildErr := a.builder.BuildFull(operationContext, prepare.FullBuildRequest{
			DatasetDirectory:           job.document.Directory,
			Goal:                       goal,
			Model:                      req.ModelID,
			DatasetInstructions:        strings.TrimSpace(req.DatasetInstructions),
			ReviewedRecords:            recordsToBusiness(job.request.ReviewedRecords),
			VerifiedPreviewRecords:     recordsToBusiness(job.request.VerifiedPreviewRecords),
			MinimumQuestionAnswerFacts: targetFacts,
			Progress: func(progress prepare.FullBuildProgress) {
				a.preparations.updateDocumentWork(
					operationID, index, documentProcessing, progress.Stage,
					progress.AcceptedRecords, progress.AcceptedFacts, targetFacts,
					progress.CompletedBatches, progress.TotalBatches, nil,
				)
			},
		})
		if buildErr != nil {
			a.preparations.updateDocumentWork(operationID, index, documentFailed, "Build failed", 0, 0, targetFacts, 0, 0, buildErr)
			a.preparations.finish(operationID, buildErr)
			return errs.FromSDK(fmt.Errorf("build full dataset for %q: %w", job.document.Name, buildErr))
		}
		facts := preparedFactCount(artifact.Records)
		stage := fmt.Sprintf("Complete · %d facts", facts)
		if goal != prepare.GoalQuestionAnswer {
			stage = fmt.Sprintf("Complete · %d records", len(artifact.Records))
		}
		a.preparations.updateDocumentWork(operationID, index, documentCompleted, stage, len(artifact.Records), facts, targetFacts, 0, 0, nil)
		built = append(built, prepare.FullBuildDocument{ID: job.document.ID, Name: job.document.Name, Artifact: artifact})
		documentResponses = append(documentResponses, DocumentBuildResponse{ID: job.document.ID, Name: job.document.Name, FactCount: facts, RecordCount: len(artifact.Records)})
	}
	a.preparations.update(operationID, "Merging document datasets", len(jobs), len(jobs), 0)
	artifact, err := prepare.MergeFullBuildArtifacts(prepare.FullBuildMergeRequest{
		DatasetDirectory: datasetDirectory, Goal: goal, Model: req.ModelID,
		DatasetInstructions: strings.TrimSpace(req.DatasetInstructions), Documents: built,
	})
	if err != nil {
		a.preparations.finish(operationID, err)
		return errs.New(errs.FailedPrecondition, fmt.Errorf("merge document datasets: %w", err))
	}
	a.preparations.update(operationID, "Validating training dataset", len(jobs), len(jobs), len(artifact.Records))
	contractDirectory := filepath.Join(datasetDirectory, "training")
	if _, err := finetune.ReplaceDatasetContract(artifact.RecordsPath, contractDirectory); err != nil {
		a.preparations.finish(operationID, err)
		return errs.New(errs.FailedPrecondition, fmt.Errorf("validate training dataset: %w", err))
	}
	a.preparations.update(operationID, "Training dataset ready", len(jobs), len(jobs), len(artifact.Records))
	a.preparations.finish(operationID, nil)

	return jsonResponse{value: BuildResponse{
		DatasetID:            req.DatasetID,
		RecordCount:          len(artifact.Records),
		ReviewedRecordCount:  artifact.ReviewedRecordCount,
		GeneratedRecordCount: artifact.GeneratedRecordCount,
		GenerationAttempts:   artifact.GenerationAttempts,
		VerificationAttempts: artifact.VerificationAttempts,
		PromptTokens:         artifact.PromptTokens,
		CompletionTokens:     artifact.CompletionTokens,
		ExcludedEvidence:     artifact.ExcludedEvidence,
		Documents:            documentResponses,
	}}
}

type documentBuild struct {
	document prepare.DatasetDocument
	request  DocumentBuildRequest
}

func resolveDocumentBuilds(documents []prepare.DatasetDocument, request BuildRequest) ([]documentBuild, error) {
	if len(request.Documents) == 0 {
		if len(documents) != 1 {
			return nil, fmt.Errorf("reviewed records are required for each included document")
		}
		return []documentBuild{{
			document: documents[0],
			request: DocumentBuildRequest{
				ID: documents[0].ID, ReviewedRecords: request.ReviewedRecords,
				VerifiedPreviewRecords: request.VerifiedPreviewRecords,
			},
		}}, nil
	}

	requests := make(map[string]DocumentBuildRequest, len(request.Documents))
	for _, document := range request.Documents {
		requests[document.ID] = document
	}
	jobs := make([]documentBuild, 0, len(request.Documents))
	for _, document := range documents {
		request, included := requests[document.ID]
		if !included {
			continue
		}
		jobs = append(jobs, documentBuild{document: document, request: request})
		delete(requests, document.ID)
	}
	if len(requests) > 0 {
		return nil, fmt.Errorf("one or more requested documents do not belong to this dataset")
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("at least one document must be included")
	}
	return jobs, nil
}

func selectDatasetDocuments(directory string, requestedIDs []string) ([]prepare.DatasetDocument, error) {
	documents, err := prepare.DatasetDocuments(directory)
	if err != nil {
		return nil, err
	}
	if len(requestedIDs) == 0 {
		return documents, nil
	}
	available := make(map[string]prepare.DatasetDocument, len(documents))
	for _, document := range documents {
		available[document.ID] = document
	}
	selected := make([]prepare.DatasetDocument, len(requestedIDs))
	for index, id := range requestedIDs {
		document, exists := available[id]
		if !exists {
			return nil, fmt.Errorf("document %q does not belong to this dataset", id)
		}
		selected[index] = document
	}
	return selected, nil
}

func datasetDocumentIDs(documents []prepare.DatasetDocument) []string {
	ids := make([]string, len(documents))
	for index, document := range documents {
		ids[index] = document.ID
	}
	return ids
}

func datasetDocumentNames(documents []prepare.DatasetDocument) []string {
	names := make([]string, len(documents))
	for index, document := range documents {
		names[index] = document.Name
	}
	return names
}

func documentBuildIDs(builds []documentBuild) []string {
	ids := make([]string, len(builds))
	for index, build := range builds {
		ids[index] = build.document.ID
	}
	return ids
}

func documentBuildNames(builds []documentBuild) []string {
	names := make([]string, len(builds))
	for index, build := range builds {
		names[index] = build.document.Name
	}
	return names
}

func preparedFactCount(records []prepare.Record) int {
	facts := make(map[string]bool)
	for _, record := range records {
		if record.FactID != "" {
			facts[record.FactID] = true
		}
	}
	return len(facts)
}
