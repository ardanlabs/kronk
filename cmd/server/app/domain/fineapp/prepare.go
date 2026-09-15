package fineapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

const (
	maxDocumentUploadBytes = 100 << 20
	maxMultipartOverhead   = 1 << 20
)

func (a *app) uploadDataset(ctx context.Context, r *http.Request) web.Encoder {
	r.Body = http.MaxBytesReader(nil, r.Body, maxDocumentUploadBytes+maxMultipartOverhead)
	if err := r.ParseMultipartForm(maxDocumentUploadBytes); err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("parse multipart form: %w", err))
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("file form field: %w", err))
	}
	defer file.Close()
	if header.Size > maxDocumentUploadBytes {
		return errs.Errorf(errs.InvalidArgument, "file exceeds 100 MiB limit")
	}

	sourceName := filepath.Base(strings.ReplaceAll(header.Filename, `\`, "/"))
	extension := strings.ToLower(filepath.Ext(sourceName))
	if extension != ".pdf" && extension != ".docx" && extension != ".jsonl" {
		return errs.Errorf(errs.InvalidArgument, "file must be a PDF, DOCX, or prepared JSONL dataset")
	}

	temporaryDirectory, err := os.MkdirTemp("", "moxie-document-")
	if err != nil {
		return errs.New(errs.Internal, fmt.Errorf("create temporary upload directory: %w", err))
	}
	defer os.RemoveAll(temporaryDirectory)
	temporaryPath := filepath.Join(temporaryDirectory, sourceName)
	temporary, err := os.OpenFile(temporaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errs.New(errs.Internal, fmt.Errorf("create temporary upload: %w", err))
	}

	written, copyErr := io.Copy(temporary, io.LimitReader(file, maxDocumentUploadBytes+1))
	closeErr := temporary.Close()
	if copyErr != nil {
		return errs.New(errs.Internal, fmt.Errorf("store temporary upload: %w", copyErr))
	}
	if closeErr != nil {
		return errs.New(errs.Internal, fmt.Errorf("close temporary upload: %w", closeErr))
	}
	if written > maxDocumentUploadBytes {
		return errs.Errorf(errs.InvalidArgument, "file exceeds 100 MiB limit")
	}
	if extension == ".jsonl" {
		operationContext, operationID, err := a.preparations.start(ctx, "prepared-upload", "Validating prepared dataset", 2)
		if err != nil {
			return errs.New(errs.FailedPrecondition, err)
		}
		if _, err := finetune.ValidateDatasetContract(temporaryPath); err != nil {
			a.preparations.finish(operationID, err)
			return errs.New(errs.InvalidArgument, fmt.Errorf("validate prepared dataset: %w", err))
		}
		if err := context.Cause(operationContext); err != nil {
			a.preparations.finish(operationID, err)
			return errs.FromSDK(fmt.Errorf("validate prepared dataset: %w", err))
		}
		a.preparations.update(operationID, "Storing prepared dataset", 1, 2, 0)
		datasetID, err := a.importPreparedDataset(temporaryPath)
		if err != nil {
			a.preparations.finish(operationID, err)
			return errs.New(errs.Internal, fmt.Errorf("import prepared dataset: %w", err))
		}
		a.preparations.update(operationID, "Prepared dataset ready", 2, 2, 0)
		a.preparations.finish(operationID, nil)
		return jsonResponse{value: DatasetResponse{DatasetID: datasetID, SourceName: sourceName}}
	}
	if a.preparer == nil {
		return errs.Errorf(errs.FailedPrecondition, "document preparation is not configured")
	}

	operationContext, operationID, err := a.preparations.start(ctx, "extraction", "Extracting source with Docling", 1)
	if err != nil {
		return errs.New(errs.FailedPrecondition, err)
	}
	artifact, err := a.preparer.Extract(operationContext, temporaryPath)
	if err != nil {
		a.preparations.finish(operationID, err)
		return errs.FromSDK(fmt.Errorf("prepare uploaded document: %w", err))
	}
	a.preparations.update(operationID, "Source extraction complete", 1, 1, 0)
	a.preparations.finish(operationID, nil)

	return jsonResponse{value: DatasetResponse{
		DatasetID:  artifact.ID,
		SourceName: filepath.Base(artifact.SourcePath),
	}}
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
	return jsonResponse{value: PreparationModelsResponse{Models: a.configuredModelIDs}}
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
	goal, _ := goalToBusiness(req.Goal)
	operationContext, operationID, err := a.preparations.start(ctx, "preview", "Generating preview records", 2)
	if err != nil {
		return errs.New(errs.FailedPrecondition, err)
	}

	artifact, err := a.builder.Build(operationContext, prepare.PreviewRequest{
		DatasetDirectory: filepath.Dir(datasetMarkdown),
		Goal:             goal,
		Model:            req.ModelID,
		RecordCount:      req.RecordCount,
		Progress: func(progress prepare.PreviewProgress) {
			a.preparations.update(operationID, progress.Stage, progress.CompletedStages, progress.TotalStages, 0)
		},
	})
	if err != nil {
		a.preparations.finish(operationID, err)
		return errs.FromSDK(fmt.Errorf("build dataset preview: %w", err))
	}
	a.preparations.finish(operationID, nil)

	return jsonResponse{value: PreviewResponse{
		DatasetID:            req.DatasetID,
		Records:              recordsFromBusiness(artifact.Records),
		Verification:         verificationFromBusiness(artifact.Verification),
		GenerationAttempts:   artifact.GenerationAttempts,
		VerificationAttempts: artifact.VerificationAttempts,
		PromptTokens:         artifact.PromptTokens,
		CompletionTokens:     artifact.CompletionTokens,
	}}
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
	goal, _ := goalToBusiness(req.Goal)
	buildTotal := 0
	if goal == prepare.GoalQuestionAnswer {
		buildTotal = finetune.MinimumKnowledgeFacts + 2
	}
	operationContext, operationID, err := a.preparations.start(ctx, "build", "Preparing dataset build", buildTotal)
	if err != nil {
		return errs.New(errs.FailedPrecondition, err)
	}

	artifact, err := a.builder.BuildFull(operationContext, prepare.FullBuildRequest{
		DatasetDirectory:           filepath.Dir(datasetMarkdown),
		Goal:                       goal,
		Model:                      req.ModelID,
		ReviewedRecords:            recordsToBusiness(req.ReviewedRecords),
		VerifiedPreviewRecords:     recordsToBusiness(req.VerifiedPreviewRecords),
		MinimumQuestionAnswerFacts: finetune.MinimumKnowledgeFacts,
		Progress: func(progress prepare.FullBuildProgress) {
			buildTotal = progress.TotalUnits + 1
			a.preparations.update(operationID, progress.Stage, progress.CompletedUnits, buildTotal, progress.AcceptedRecords)
		},
	})
	if err != nil {
		a.preparations.finish(operationID, err)
		return errs.FromSDK(fmt.Errorf("build full dataset: %w", err))
	}
	a.preparations.update(operationID, "Validating training dataset", buildTotal-1, buildTotal, len(artifact.Records))
	contractDirectory := filepath.Join(filepath.Dir(datasetMarkdown), "training")
	if _, err := finetune.ReplaceDatasetContract(artifact.RecordsPath, contractDirectory); err != nil {
		a.preparations.finish(operationID, err)
		return errs.New(errs.FailedPrecondition, fmt.Errorf("validate training dataset: %w", err))
	}
	a.preparations.update(operationID, "Training dataset ready", buildTotal, buildTotal, len(artifact.Records))
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
	}}
}
