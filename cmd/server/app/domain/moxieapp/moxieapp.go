// Package moxieapp provides the HTTP API for training jobs.
package moxieapp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets"
	datasetstore "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxierun"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

type app struct {
	training *training.Job
	uploads  *uploads.Store
	datasets *datasets.Service
}

func newApp(cfg moxierun.Services) *app {
	return &app{training: cfg.Job, uploads: cfg.Uploads, datasets: cfg.Datasets}
}

// readiness resolves one immutable service snapshot after authorization. A failed
// initialization never reaches the operation, especially a training submission.
func (cfg Config) readiness(operation func(*app, context.Context, *http.Request) web.Encoder, require func(*app, web.HandlerFunc) web.HandlerFunc) web.HandlerFunc {
	return func(ctx context.Context, r *http.Request) web.Encoder {
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		services := moxierun.Services{Job: cfg.Job, Uploads: cfg.Uploads, Datasets: cfg.Datasets}
		if cfg.Resolve != nil {
			var err error
			services, err = cfg.Resolve(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return toAppError(ctx.Err())
				}
				return errs.Errorf(errs.Unavailable, "Moxie is unavailable; check PostgreSQL and Temporal, then try again")
			}
		}

		api := newApp(services)

		next := func(ctx context.Context, r *http.Request) web.Encoder { return operation(api, ctx, r) }
		if require != nil {
			next = require(api, next)
		}

		return next(ctx, r.WithContext(ctx))
	}
}

func (a *app) requireDatasets(next web.HandlerFunc) web.HandlerFunc {
	return func(ctx context.Context, r *http.Request) web.Encoder {
		if a.datasets == nil {
			return errs.Errorf(errs.Unavailable, "preparation dataset storage is unavailable")
		}
		return next(ctx, r)
	}
}

func (a *app) listDatasets(ctx context.Context, r *http.Request) web.Encoder {
	web.GetWriter(ctx).Header().Set("Cache-Control", "no-store")
	datasets, err := a.datasets.List(ctx)
	if err != nil {
		return toAppError(err)
	}
	result := Datasets{Datasets: make([]Dataset, len(datasets))}
	for i, dataset := range datasets {
		result.Datasets[i] = toAppDataset(dataset)
	}
	return result
}

func (a *app) getPreparationInstructions(ctx context.Context, r *http.Request) web.Encoder {
	web.GetWriter(ctx).Header().Set("Cache-Control", "no-store")
	instructions, err := a.datasets.PreparationInstructions(ctx)
	if err != nil {
		return toAppError(err)
	}
	return PreparationInstructions{Instructions: instructions}
}

func (a *app) savePreparationInstructions(ctx context.Context, r *http.Request) web.Encoder {
	r.Body = http.MaxBytesReader(web.GetWriter(ctx), r.Body, 16<<10)
	var input PreparationInstructions
	if err := web.Decode(r, &input); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	instructions, err := a.datasets.SavePreparationInstructions(ctx, input.Instructions)
	if err != nil {
		return toAppError(err)
	}
	return PreparationInstructions{Instructions: instructions}
}

func (a *app) createDataset(ctx context.Context, r *http.Request) web.Encoder {
	r.Body = http.MaxBytesReader(web.GetWriter(ctx), r.Body, 4096)
	var input NewDataset
	if err := web.Decode(r, &input); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	dataset, err := a.datasets.Create(ctx, input.Name)
	if err != nil {
		return toAppError(err)
	}
	return toAppDataset(dataset)
}

func (a *app) getDataset(ctx context.Context, r *http.Request) web.Encoder {
	web.GetWriter(ctx).Header().Set("Cache-Control", "no-store")
	version := 0
	if r.URL.Query().Has("version") {
		var err error
		version, err = strconv.Atoi(r.URL.Query().Get("version"))
		if err != nil || version < 1 {
			return errs.Errorf(errs.InvalidArgument, "version must be a positive integer")
		}
	}
	dataset, err := a.datasets.Get(ctx, r.PathValue("dataset_id"), version)
	if err != nil {
		return toAppError(err)
	}
	return toAppDataset(dataset)
}

func (a *app) removeDocument(ctx context.Context, r *http.Request) web.Encoder {
	version, err := strconv.Atoi(r.URL.Query().Get("base_version"))
	if err != nil || version < 1 {
		return errs.Errorf(errs.InvalidArgument, "base_version must be a positive integer")
	}
	dataset, err := a.datasets.Remove(ctx, r.PathValue("dataset_id"), version, r.PathValue("document_id"))
	if err != nil {
		return toAppError(err)
	}
	return toAppDataset(dataset)
}

func (a *app) previewDeletion(ctx context.Context, r *http.Request) web.Encoder {
	web.GetWriter(ctx).Header().Set("Cache-Control", "no-store")
	preview, err := a.training.PreviewDeletion(ctx, tmprl.Execution{WorkflowID: r.PathValue("workflow_id"), RunID: r.PathValue("run_id")})
	if err != nil {
		return toAppError(err)
	}
	return RunDeletion{DeletionPreview: preview}
}

func (a *app) deleteRun(ctx context.Context, r *http.Request) web.Encoder {
	web.GetWriter(ctx).Header().Set("Cache-Control", "no-store")
	revision := r.URL.Query().Get("revision")
	if revision == "" {
		return errs.Errorf(errs.InvalidArgument, "preview and confirm the deletion scope first")
	}
	preview, err := a.training.DeleteRun(ctx, tmprl.Execution{WorkflowID: r.PathValue("workflow_id"), RunID: r.PathValue("run_id")}, revision)
	if errors.Is(err, tmprl.ErrControlConflict) {
		return errs.New(errs.Aborted, err)
	}
	if errors.Is(err, moxiedb.ErrConflict) {
		return errs.Errorf(errs.Aborted, "deletion scope changed or a dependent action is pending; refresh the deletion preview")
	}
	if err != nil {
		return toAppError(err)
	}
	return RunDeletion{DeletionPreview: preview}
}

func (a *app) downloadDatasetVersion(ctx context.Context, r *http.Request) web.Encoder {
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || version < 1 {
		return errs.Errorf(errs.InvalidArgument, "version must be a positive integer")
	}
	records, err := a.datasets.Records(ctx, r.PathValue("dataset_id"), version)
	if err != nil {
		return toAppError(err)
	}
	w := web.GetWriter(ctx)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="dataset-v%d.jsonl"`, version))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return web.NewNoResponseError(errs.New(errs.Internal, err))
		}
	}
	return web.NewNoResponse()
}

func (a *app) requireJob(next web.HandlerFunc) web.HandlerFunc {
	return func(ctx context.Context, r *http.Request) web.Encoder {
		if a.training == nil {
			return errs.Errorf(errs.Unavailable, "training service is unavailable; check server startup logs")
		}
		return next(ctx, r)
	}
}

func (a *app) requireRuns(next web.HandlerFunc) web.HandlerFunc {
	return func(ctx context.Context, r *http.Request) web.Encoder {
		if a.datasets == nil || !a.datasets.RunsAvailable() {
			return errs.Errorf(errs.Unavailable, "training service is unavailable; check server startup logs")
		}
		return next(ctx, r)
	}
}

func (a *app) requireCatalog(next web.HandlerFunc) web.HandlerFunc {
	return func(ctx context.Context, r *http.Request) web.Encoder {
		if a.datasets == nil || !a.datasets.CatalogAvailable() {
			return errs.Errorf(errs.Unavailable, "preparation run catalog is unavailable; check server startup logs")
		}
		return next(ctx, r)
	}
}

func (a *app) upload(ctx context.Context, r *http.Request) web.Encoder {
	// Bound the entire request, including multipart framing, before parsing it.
	r.Body = http.MaxBytesReader(web.GetWriter(ctx), r.Body, a.uploads.MaximumBytes()+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return errs.Errorf(errs.InvalidArgument, "a multipart file upload within the size limit is required")
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 || len(r.MultipartForm.Value) != 0 {
		return errs.Errorf(errs.InvalidArgument, "exactly one file form field is required")
	}
	header := r.MultipartForm.File["file"][0]
	if header.Size < 1 || header.Size > a.uploads.MaximumBytes() {
		return errs.Errorf(errs.InvalidArgument, "file must contain 1–%d bytes", a.uploads.MaximumBytes())
	}
	source, err := header.Open()
	if err != nil {
		return toAppError(err)
	}
	defer source.Close()
	file, err := a.uploads.Put(ctx, uploads.Input{Name: header.Filename}, source)
	if err != nil {
		return toAppError(err)
	}
	return Upload{
		ID: file.ID, Name: file.Name, Format: file.Format, SizeBytes: file.SizeBytes,
		SHA256: file.SHA256, CreatedAt: file.CreatedAt,
	}
}

func (a *app) create(ctx context.Context, r *http.Request) web.Encoder {
	r.Body = http.MaxBytesReader(web.GetWriter(ctx), r.Body, 1<<20)
	var app NewTraining
	if err := web.Decode(r, &app); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	request, err := toJobRequest(app)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	run, err := a.datasets.StartRun(ctx, request)
	if err != nil {
		return toAppError(err)
	}

	return toAppRun(run)
}

func (a *app) continuePreview(ctx context.Context, r *http.Request) web.Encoder {
	execution := tmprl.Execution{WorkflowID: r.PathValue("workflow_id"), RunID: r.PathValue("run_id")}
	if err := execution.Validate(); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	r.Body = http.MaxBytesReader(web.GetWriter(ctx), r.Body, 64<<10)
	var app ContinuePreview
	if err := web.Decode(r, &app); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	decision, err := toPreviewDecision(app)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	receipt, err := a.datasets.ContinuePreview(ctx, execution, decision)
	if err != nil {
		return toAppError(err)
	}
	return toAppDecisionReceipt(receipt)
}

func (a *app) pausePreparation(ctx context.Context, r *http.Request) web.Encoder {
	return a.runControl(ctx, r, "preparation", "pause")
}

func (a *app) continuePreparation(ctx context.Context, r *http.Request) web.Encoder {
	return a.runControl(ctx, r, "preparation", "resume")
}

func (a *app) pauseTraining(ctx context.Context, r *http.Request) web.Encoder {
	return a.runControl(ctx, r, "training", "pause")
}

func (a *app) resumeTraining(ctx context.Context, r *http.Request) web.Encoder {
	return a.runControl(ctx, r, "training", "resume")
}

func (a *app) runControl(ctx context.Context, r *http.Request, stage, action string) web.Encoder {
	execution := tmprl.Execution{WorkflowID: r.PathValue("workflow_id"), RunID: r.PathValue("run_id")}
	if err := execution.Validate(); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	r.Body = http.MaxBytesReader(web.GetWriter(ctx), r.Body, 4096)
	var app PreparationControl
	if err := web.Decode(r, &app); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	request, err := toControlRequest(app, action)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	var receipt tmprl.ControlReceipt
	if stage == "training" {
		receipt, err = a.training.TrainingControl(ctx, execution, request)
	} else {
		receipt, err = a.datasets.PreparationControl(ctx, execution, request)
	}
	if err != nil {
		return toAppError(err)
	}
	return toAppControlReceipt(receipt)
}

func (a *app) startTraining(ctx context.Context, r *http.Request) web.Encoder {
	execution := tmprl.Execution{WorkflowID: r.PathValue("workflow_id"), RunID: r.PathValue("run_id")}
	if err := execution.Validate(); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	r.Body = http.MaxBytesReader(web.GetWriter(ctx), r.Body, 64<<10)
	var app StartTraining
	if err := web.Decode(r, &app); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	receipt, err := a.training.StartTraining(ctx, execution, tmprl.Decision{
		CheckpointID: app.CheckpointID, SubmissionID: app.SubmissionID,
	})
	if err != nil {
		return toAppError(err)
	}
	return toAppDecisionReceipt(receipt)
}

// stop acknowledges cooperative cancellation; polling establishes terminal state.
func (a *app) stop(ctx context.Context, r *http.Request) web.Encoder {
	execution := tmprl.Execution{WorkflowID: r.PathValue("workflow_id"), RunID: r.PathValue("run_id")}
	if err := execution.Validate(); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	if err := a.datasets.StopRun(ctx, execution); err != nil {
		return toAppError(err)
	}
	return nil
}

func (a *app) query(ctx context.Context, r *http.Request) web.Encoder {
	pageSize := 25
	if value := r.URL.Query().Get("page_size"); value != "" {
		var err error
		pageSize, err = strconv.Atoi(value)
		if err != nil || pageSize < 1 || pageSize > 100 {
			return errs.Errorf(errs.InvalidArgument, "page_size must be between 1 and 100")
		}
	}
	token, err := base64.RawURLEncoding.DecodeString(r.URL.Query().Get("next_page_token"))
	if err != nil {
		return errs.Errorf(errs.InvalidArgument, "invalid next_page_token")
	}
	since := r.URL.Query().Get("since")
	if since != "" {
		if decoded, err := hex.DecodeString(since); err != nil || len(decoded) != sha256.Size {
			return errs.Errorf(errs.InvalidArgument, "invalid since revision")
		}
	}
	web.GetWriter(ctx).Header().Set("Cache-Control", "no-store")
	waitCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	options := datasets.RunFilter{PageSize: pageSize, NextPageToken: token, Mode: r.URL.Query().Get("mode"), TargetDatasetID: r.URL.Query().Get("target_dataset_id")}
	switch r.URL.Query().Get("order") {
	case "", "newest":
	case "oldest":
		options.Oldest = true
	default:
		return errs.Errorf(errs.InvalidArgument, "order must be newest or oldest")
	}
	for _, value := range r.URL.Query()["status"] {
		options.Statuses = append(options.Statuses, tmprl.Status(value))
	}
	var result Runs
	for {
		runs, next, err := a.datasets.ListRuns(waitCtx, options)
		if err != nil {
			if ctx.Err() == nil && waitCtx.Err() == context.DeadlineExceeded && result.Revision != "" {
				return result
			}
			return toAppError(err)
		}
		result = Runs{Runs: make([]Run, len(runs)), NextPageToken: base64.RawURLEncoding.EncodeToString(next)}
		for i, run := range runs {
			result.Runs[i] = toAppRun(run)
		}
		// Revisions describe public state, not opaque pagination-token changes.
		data, err := json.Marshal(result.Runs)
		if err != nil {
			return toAppError(err)
		}
		digest := sha256.Sum256(data)
		result.Revision = hex.EncodeToString(digest[:])
		if since != result.Revision {
			return result
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return toAppError(ctx.Err())
		case <-waitCtx.Done():
			timer.Stop()
			if ctx.Err() != nil {
				return toAppError(ctx.Err())
			}
			return result
		case <-timer.C:
		}
	}
}

func (a *app) queryByID(ctx context.Context, r *http.Request) web.Encoder {
	execution := tmprl.Execution{WorkflowID: r.PathValue("workflow_id"), RunID: r.PathValue("run_id")}
	if err := execution.Validate(); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	run, err := a.datasets.GetRun(ctx, execution)
	if err != nil {
		return toAppError(err)
	}

	return toAppRun(run)
}

func (a *app) evaluation(ctx context.Context, r *http.Request) web.Encoder {
	web.GetWriter(ctx).Header().Set("Cache-Control", "no-store")
	execution := tmprl.Execution{WorkflowID: r.PathValue("workflow_id"), RunID: r.PathValue("run_id")}
	if err := execution.Validate(); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	report, err := a.training.Evaluation(ctx, execution)
	if err != nil {
		if errors.Is(err, tmprl.ErrInvalidArgument) {
			return errs.Errorf(errs.NotFound, "evaluation report is not available")
		}
		appError := toAppError(err)
		if appError.Code == errs.Internal {
			return errs.Errorf(errs.Internal, "evaluation report could not be read")
		}
		return appError
	}
	return toAppEvaluation(report)
}

func (a *app) adapter(ctx context.Context, r *http.Request) web.Encoder {
	return a.downloadArtifact(ctx, r, "adapter")
}

func (a *app) dataset(ctx context.Context, r *http.Request) web.Encoder {
	return a.downloadArtifact(ctx, r, "dataset")
}

func (a *app) downloadArtifact(ctx context.Context, r *http.Request, kind string) web.Encoder {
	w := web.GetWriter(ctx)
	w.Header().Set("Cache-Control", "no-store")
	execution := tmprl.Execution{WorkflowID: r.PathValue("workflow_id"), RunID: r.PathValue("run_id")}
	if err := execution.Validate(); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	open, filename := a.training.OpenAdapter, "adapter.gguf"
	if kind == "dataset" {
		open, filename = a.datasets.OpenPreparedDataset, "dataset.jsonl"
	}
	file, err := open(ctx, execution)
	if err != nil {
		if errors.Is(err, tmprl.ErrInvalidArgument) {
			return errs.Errorf(errs.NotFound, "%s is not available", kind)
		}
		appError := toAppError(err)
		if appError.Code == errs.Internal {
			return errs.Errorf(errs.Internal, "%s could not be read", kind)
		}
		return appError
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, tmprl.ContextReader{Context: ctx, Reader: file}); err != nil {
		return web.NewNoResponseError(errs.New(errs.Internal, err))
	}
	return web.NewNoResponse()
}

func toAppError(err error) *errs.Error {
	switch {
	case errors.Is(err, tmprl.ErrControlConflict):
		return errs.Errorf(errs.Aborted, "preparation control conflicts with the current run or request; refresh its state")
	case errors.Is(err, datasetstore.ErrDocumentExists):
		return errs.New(errs.FailedPrecondition, err)
	case errors.Is(err, datasetstore.ErrDatasetConflict):
		return errs.New(errs.Aborted, err)
	case errors.Is(err, datasetstore.ErrDatasetNotFound):
		return errs.Errorf(errs.NotFound, "preparation dataset or document not found")
	case errors.Is(err, tmprl.ErrInvalidArgument), errors.Is(err, uploads.ErrInvalidArgument):
		return errs.New(errs.InvalidArgument, err)
	case errors.Is(err, tmprl.ErrUnsupportedWorkflow), errors.Is(err, uploads.ErrNotFound):
		return errs.Errorf(errs.NotFound, "training resource not found")
	case errors.Is(err, context.Canceled):
		return errs.New(errs.Canceled, err)
	case errors.Is(err, context.DeadlineExceeded):
		return errs.Errorf(errs.DeadlineExceeded, "training request timed out")
	case errors.Is(err, training.ErrNotFound):
		return errs.Errorf(errs.NotFound, "training resource not found")
	case errors.Is(err, training.ErrAlreadyExists):
		return errs.Errorf(errs.AlreadyExists, "training job already exists")
	case errors.Is(err, training.ErrUnavailable), moxiedb.IsUnavailable(err):
		return errs.Errorf(errs.Unavailable, "training service is unavailable")
	}
	return errs.Errorf(errs.Internal, "training request: %s", err)
}
