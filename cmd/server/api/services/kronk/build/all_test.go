package build_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/api/services/kronk/build"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/trainapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mux"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"github.com/stretchr/testify/mock"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/mocks"
)

func TestTrainingRouteWiring(t *testing.T) {
	c := new(mocks.Client)
	store, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	job, err := training.New(c, training.Config{
		Uploads: store, StorageRoot: t.TempDir(), Models: map[string]string{"model-a": "/model.gguf"},
		Trainer: struct{ optimization.Trainer }{}, Log: func(context.Context, string, ...any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := converter.GetDefaultDataConverter().ToPayload(tmprl.Run[training.Request, training.Artifacts]{Status: tmprl.StatusQueued})
	if err != nil {
		t.Fatal(err)
	}
	c.On("DescribeWorkflowExecution", mock.Anything, "training-example", "run-2").Return(&workflowservice.DescribeWorkflowExecutionResponse{
		WorkflowExecutionInfo: &workflowpb.WorkflowExecutionInfo{
			Execution: &commonpb.WorkflowExecution{WorkflowId: "training-example", RunId: "run-2"},
			Type:      &commonpb.WorkflowType{Name: "moxie.training.Job"}, Status: enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING,
			Memo: &commonpb.Memo{Fields: map[string]*commonpb.Payload{"moxie.training.job": payload}},
		},
	}, nil)
	log := logger.New(io.Discard, logger.LevelInfo, "route-test", nil)
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		var cfg build.Config
		if enabled {
			name = "enabled"
			cfg.Training = trainapp.Config{Job: job, Uploads: store}
		}
		t.Run(name, func(t *testing.T) {
			api := mux.WebAPI(mux.Config{Log: log, AuthorizationMode: auth.Open}, build.Routes(cfg))
			for _, legacy := range []struct{ method, path string }{
				{http.MethodPost, "/fine-tuning/datasets"},
				{http.MethodGet, "/fine-tuning/prepare/models"},
				{http.MethodGet, "/fine-tuning/prepare/current"},
				{http.MethodPost, "/fine-tuning/prepare/current/stop"},
				{http.MethodPost, "/fine-tuning/prepare/preview"},
				{http.MethodPost, "/fine-tuning/prepare/build"},
				{http.MethodGet, "/fine-tuning/recipe"},
				{http.MethodPost, "/fine-tuning/runs"},
				{http.MethodGet, "/fine-tuning/runs"},
				{http.MethodGet, "/fine-tuning/runs/current"},
				{http.MethodPost, "/fine-tuning/runs/current/stop"},
				{http.MethodGet, "/fine-tuning/runs/example"},
				{http.MethodPost, "/fine-tuning/runs/example/resume"},
			} {
				w := httptest.NewRecorder()
				api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), legacy.method, "/v1"+legacy.path, nil))
				if w.Code != http.StatusNotFound {
					t.Errorf("removed route %s %s: got %d, want 404", legacy.method, legacy.path, w.Code)
				}
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/training/uploads", nil))
			wantUploadStatus := http.StatusNotFound
			if enabled {
				wantUploadStatus = http.StatusBadRequest
			}
			if w.Code != wantUploadStatus {
				t.Fatalf("upload route: %d %s", w.Code, w.Body)
			}
			w = httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/training/runs/training-example/run-2", nil))
			if !enabled {
				if w.Code != http.StatusNotFound {
					t.Fatalf("disabled route: %d %s", w.Code, w.Body)
				}
				return
			}
			if w.Code != http.StatusOK {
				t.Fatalf("enabled route: %d %s", w.Code, w.Body)
			}
			var got trainapp.Run
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.WorkflowID != "training-example" || got.RunID != "run-2" || got.Status != "queued" || got.StateStale || len(got.Stages) != 0 {
				t.Fatalf("state: %+v", got)
			}
		})
	}
}
