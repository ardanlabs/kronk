package build_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/api/services/kronk/build"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/moxieapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mux"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"github.com/ardanlabs/kronk/sdk/tools/models"
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
	catalog, err := models.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	job, err := training.New(c, training.Config{
		Uploads: store, StorageRoot: t.TempDir(),
		Models: map[string]string{"ggml-org/gemma-3-1b-it-Q8_0": ""}, ModelCatalog: catalog,
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
	c.On("ListWorkflow", mock.Anything, mock.Anything).Return(&workflowservice.ListWorkflowExecutionsResponse{}, nil)
	log := logger.New(io.Discard, logger.LevelInfo, "route-test", nil)
	for _, test := range []struct {
		name       string
		training   moxieapp.Config
		wantUpload int
		wantRun    int
	}{
		{"disabled", moxieapp.Config{}, http.StatusNotFound, http.StatusNotFound},
		// Startup always pairs uploads with the datasets service, which reads runs through the job.
		{"uploads-only", moxieapp.Config{Uploads: store, Datasets: datasets.New(datasets.Config{Uploads: store})}, http.StatusBadRequest, http.StatusServiceUnavailable},
		{"enabled", moxieapp.Config{Job: job, Uploads: store, Datasets: datasets.New(datasets.Config{Job: job, Uploads: store})}, http.StatusBadRequest, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := mux.WebAPI(mux.Config{Log: log, AuthorizationMode: auth.Open}, build.Routes(build.Config{Moxie: test.training}))
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
			if w.Code != test.wantUpload {
				t.Fatalf("upload route: %d %s", w.Code, w.Body)
			}
			w = httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/training/runs/training-example/run-2", nil))
			if w.Code != test.wantRun {
				t.Fatalf("run route: %d %s", w.Code, w.Body)
			}
			if test.wantRun != http.StatusOK {
				return
			}
			for _, mode := range []string{"preparation", "training"} {
				list := httptest.NewRecorder()
				api.ServeHTTP(list, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/training/runs?mode="+mode, nil))
				var result moxieapp.Runs
				if err := json.Unmarshal(list.Body.Bytes(), &result); err != nil || list.Code != http.StatusOK || result.Runs == nil || len(result.Runs) != 0 {
					t.Fatalf("%s history without downloaded models: got %d %s, want 200 with an empty runs array: %v", mode, list.Code, list.Body, err)
				}
			}
			var got moxieapp.Run
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.WorkflowID != "training-example" || got.RunID != "run-2" || got.Status != "queued" || got.StateStale || len(got.Stages) != 0 {
				t.Fatalf("state: %+v", got)
			}
			missing := httptest.NewRecorder()
			api.ServeHTTP(missing, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/training/runs", strings.NewReader(`{"model":"ggml-org/gemma-3-1b-it-Q8_0","dataset_id":"dataset"}`)))
			if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "not downloaded") {
				t.Fatalf("training without a model: got %d %s, want 400 explaining the missing download", missing.Code, missing.Body)
			}
		})
	}
}
