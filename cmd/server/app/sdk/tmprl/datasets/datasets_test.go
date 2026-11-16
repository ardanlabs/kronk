package datasets_test

import (
	"errors"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
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

func TestDatasetManagementWithoutExecution(t *testing.T) {
	files, err := store.NewDatasetStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := datasets.New(files, nil)
	if err := service.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	dataset, err := service.Create(t.Context(), "Harbor")
	if err != nil || service.RunsAvailable() || dataset.Name != "Harbor" {
		t.Fatalf("dataset management without jobs: %+v, %v", dataset, err)
	}
	if _, err := service.StartRun(t.Context(), training.Request{}); !errors.Is(err, training.ErrUnavailable) {
		t.Fatalf("StartRun: %v", err)
	}
	if _, err := service.GetRun(t.Context(), tmprl.Execution{}); !errors.Is(err, training.ErrUnavailable) {
		t.Fatalf("GetRun: %v", err)
	}
	if _, _, err := service.ListRuns(t.Context(), tmprl.ListOptions{PageSize: 25}); !errors.Is(err, training.ErrUnavailable) {
		t.Fatalf("ListRuns: %v", err)
	}
	if _, err := service.ContinuePreview(t.Context(), tmprl.Execution{}, tmprl.Decision{}); !errors.Is(err, training.ErrUnavailable) {
		t.Fatalf("ContinuePreview: %v", err)
	}
	if err := service.StopRun(t.Context(), tmprl.Execution{}); !errors.Is(err, training.ErrUnavailable) {
		t.Fatalf("StopRun: %v", err)
	}
	if _, err := service.OpenPreparedDataset(t.Context(), tmprl.Execution{}); !errors.Is(err, training.ErrUnavailable) {
		t.Fatalf("OpenPreparedDataset: %v", err)
	}
}

func TestRunDatasetReadiness(t *testing.T) {
	for _, test := range []struct {
		name       string
		savedID    string
		savedCount int
		savedReady bool
		source     string
		path       string
		wantCount  int
		wantReady  bool
		available  bool
	}{
		{name: "contribution", source: "upload-source", path: "dataset.jsonl", wantCount: 5, available: true},
		{name: "ready combined version", savedID: "dataset-saved", savedCount: 73, savedReady: true, source: "upload-source", path: "dataset.jsonl", wantCount: 73, wantReady: true, available: true},
		{name: "small combined version", savedID: "dataset-saved", savedCount: 8, source: "upload-source", path: "dataset.jsonl", wantCount: 8, available: true},
		{name: "foreign source", savedID: "dataset-saved", savedCount: 73, savedReady: true, source: "other-upload", path: "dataset.jsonl", wantCount: 73},
		{name: "unpublished output", savedID: "dataset-saved", savedCount: 73, savedReady: true, source: "upload-source", wantCount: 73},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := mocks.NewClient(t)
			uploads, err := uploads.New(uploads.Config{Root: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			job, err := training.New(c, training.Config{Uploads: uploads, StorageRoot: t.TempDir(), Trainer: struct{ optimization.Trainer }{}})
			if err != nil {
				t.Fatal(err)
			}
			service := datasets.New(job.Datasets(), job)
			version := 0
			if test.savedID != "" {
				version = 4
			}
			snapshot := tmprl.Run[training.Request, training.Artifacts]{
				Status:  tmprl.StatusRunning,
				Request: training.Request{PreparationOnly: true, Prepare: &preparation.PreparationRequest{DatasetID: "upload-source"}},
				Artifacts: training.Artifacts{Preparation: &preparation.PreparationResult{
					DatasetID: test.source, RecordsPath: test.path, RecordCount: 5,
					SavedDatasetID: test.savedID, SavedVersion: version, SavedRecordCount: test.savedCount, SavedTrainingReady: test.savedReady,
				}},
			}
			payload, err := converter.GetDefaultDataConverter().ToPayload(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			e := tmprl.Execution{WorkflowID: "preparation-a", RunID: "run-b"}
			info := &workflowpb.WorkflowExecutionInfo{
				Execution: &commonpb.WorkflowExecution{WorkflowId: e.WorkflowID, RunId: e.RunID},
				Type:      &commonpb.WorkflowType{Name: "moxie.training.Job"}, Status: enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING,
				Memo: &commonpb.Memo{Fields: map[string]*commonpb.Payload{"moxie.training.job": payload}},
			}
			c.On("DescribeWorkflowExecution", mock.Anything, e.WorkflowID, e.RunID).Return(&workflowservice.DescribeWorkflowExecutionResponse{
				WorkflowExecutionInfo: info, PendingWorkflowTask: &workflowpb.PendingWorkflowTaskInfo{},
			}, nil)
			c.On("ListWorkflow", mock.Anything, mock.Anything).Return(&workflowservice.ListWorkflowExecutionsResponse{
				Executions: []*workflowpb.WorkflowExecutionInfo{info}, NextPageToken: []byte{0, 255, 7},
			}, nil)
			run, err := service.GetRun(t.Context(), e)
			if err != nil {
				t.Fatal(err)
			}
			page, token, err := service.ListRuns(t.Context(), tmprl.ListOptions{PageSize: 25})
			if err != nil || len(page) != 1 || string(token) != string([]byte{0, 255, 7}) {
				t.Fatalf("page: %+v, %v, %v", page, token, err)
			}
			for _, state := range []datasets.State{run, page[0]} {
				if state.Execution != e || state.DatasetAvailable != test.available || state.DatasetRecordCount != test.wantCount || state.TrainingReady != test.wantReady || state.PreparedDatasetID != test.savedID || state.PreparedDatasetVersion != version {
					t.Fatalf("dataset observation: %+v", state)
				}
			}
		})
	}
}
