// Package trainapp provides the HTTP API for training jobs.
package trainapp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"go.temporal.io/api/serviceerror"
)

type app struct {
	training *training.Job
	uploads  *uploads.Store
}

func newApp(cfg Config) *app {
	return &app{training: cfg.Job, uploads: cfg.Uploads}
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

	run, err := a.training.Start(ctx, request)
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
	receipt, err := a.training.ContinuePreview(ctx, execution, decision)
	if err != nil {
		return toAppError(err)
	}
	return toAppDecisionReceipt(receipt)
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
	var result Runs
	for {
		runs, next, err := a.training.Jobs().List(waitCtx, tmprl.ListOptions{PageSize: pageSize, NextPageToken: token})
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

	run, err := a.training.Jobs().Get(ctx, execution)
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
	w := web.GetWriter(ctx)
	w.Header().Set("Cache-Control", "no-store")
	execution := tmprl.Execution{WorkflowID: r.PathValue("workflow_id"), RunID: r.PathValue("run_id")}
	if err := execution.Validate(); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	file, err := a.training.OpenAdapter(ctx, execution)
	if err != nil {
		if errors.Is(err, tmprl.ErrInvalidArgument) {
			return errs.Errorf(errs.NotFound, "adapter is not available")
		}
		appError := toAppError(err)
		if appError.Code == errs.Internal {
			return errs.Errorf(errs.Internal, "adapter could not be read")
		}
		return appError
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="adapter.gguf"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, tmprl.ContextReader{Context: ctx, Reader: file}); err != nil {
		return web.NewNoResponseError(errs.New(errs.Internal, err))
	}
	return web.NewNoResponse()
}

func toAppError(err error) *errs.Error {
	switch {
	case errors.Is(err, tmprl.ErrInvalidArgument), errors.Is(err, uploads.ErrInvalidArgument):
		return errs.New(errs.InvalidArgument, err)
	case errors.Is(err, tmprl.ErrUnsupportedWorkflow), errors.Is(err, uploads.ErrNotFound):
		return errs.Errorf(errs.NotFound, "training resource not found")
	case errors.Is(err, context.Canceled):
		return errs.New(errs.Canceled, err)
	case errors.Is(err, context.DeadlineExceeded):
		return errs.Errorf(errs.DeadlineExceeded, "training request timed out")
	}
	if _, ok := errors.AsType[*serviceerror.NotFound](err); ok {
		return errs.Errorf(errs.NotFound, "training resource not found")
	}
	if _, ok := errors.AsType[*serviceerror.WorkflowExecutionAlreadyStarted](err); ok {
		return errs.Errorf(errs.AlreadyExists, "training job already exists")
	}
	if _, ok := errors.AsType[*serviceerror.Unavailable](err); ok {
		return errs.Errorf(errs.Unavailable, "training service is unavailable")
	}
	if _, ok := errors.AsType[*serviceerror.DeadlineExceeded](err); ok {
		return errs.Errorf(errs.DeadlineExceeded, "training request timed out")
	}
	return errs.Errorf(errs.Internal, "training request: %s", err)
}
