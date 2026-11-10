package trainapp_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/trainapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/stretchr/testify/mock"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/mocks"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const statePath = "/v1/training/runs/training-example/run-2"

const continueBody = `{"checkpoint_id":"run-2/preparation/preview/preview-review","submission_id":"review-1","records":[{"prompt":"  At what time does the harbor open?  ","target":" At   dawn. ","evidence":"The harbor opens at dawn.","source_document":"upload-source"},{"prompt":"When does the ferry leave?","target":"At noon.","evidence":"The ferry leaves at noon.","source_document":"upload-source"}]}`

const startTrainingBody = `{"checkpoint_id":"run-2/preparation//start-training","submission_id":"training-1"}`

func testAPI(t *testing.T, c *mocks.Client) (*web.App, *uploads.Store) {
	t.Helper()
	store, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	job, err := training.New(c, training.Config{
		Uploads: store, StorageRoot: t.TempDir(), Models: map[string]string{"model-a": filepath.Join(t.TempDir(), "model.gguf")},
		// No computation is implemented: submissions must only queue work.
		Trainer: struct{ optimization.Trainer }{}, Completer: struct{ preparation.ModelCompleter }{},
		Log: func(context.Context, string, ...any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	log := logger.New(io.Discard, logger.LevelInfo, "trainapp-test", nil)
	api := web.NewApp(log.Info, mid.Errors(log))
	trainapp.Routes(api, trainapp.Config{Job: job, Uploads: store, AuthorizationMode: auth.Open})
	return api, store
}

func TestPreparationDatasetRoutes(t *testing.T) {
	store, err := preparation.NewDatasetStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	log := logger.New(io.Discard, logger.LevelInfo, "datasets-test", nil)
	api := web.NewApp(log.Info, mid.Errors(log))
	trainapp.Routes(api, trainapp.Config{Datasets: store, AuthorizationMode: auth.Open})
	request := func(method, path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		api.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, w.Code, status, w.Body)
		}
		return w
	}
	created := request(http.MethodPost, "/v1/training/datasets", `{"name":"Harbor"}`, http.StatusOK)
	var dataset trainapp.Dataset
	if err := json.Unmarshal(created.Body.Bytes(), &dataset); err != nil || dataset.Name != "Harbor" || dataset.Version != 1 {
		t.Fatalf("create: %+v, %v", dataset, err)
	}
	row := optimization.Record{Prompt: "When does the harbor open?", Target: "Dawn.", Evidence: "The harbor opens at dawn.", SourceDocument: "upload-a"}
	if _, err := store.Add(t.Context(), dataset.ID, 1, preparation.DatasetAddition{PublicationID: "run-a", Source: uploads.File{ID: "upload-a", Name: "harbor.txt", SHA256: "hash-a"}, Records: []optimization.Record{row}}); err != nil {
		t.Fatal(err)
	}
	path := "/v1/training/datasets/" + dataset.ID
	request(http.MethodGet, path, "", http.StatusOK)
	request(http.MethodGet, "/v1/training/datasets", "", http.StatusOK)
	before := request(http.MethodGet, path+"/versions/2/download", "", http.StatusOK)
	originalDownload := before.Body.String()
	rows, err := optimization.ReadRecords(t.Context(), before.Body)
	if err != nil || len(rows) != 1 || rows[0] != row || before.Header().Get("Content-Disposition") == "" {
		t.Fatalf("version download: %+v, %v", rows, err)
	}
	request(http.MethodDelete, path+"/documents/run-a?base_version=1", "", http.StatusConflict)
	removed := request(http.MethodDelete, path+"/documents/run-a?base_version=2", "", http.StatusOK)
	if err := json.Unmarshal(removed.Body.Bytes(), &dataset); err != nil || dataset.Version != 3 || dataset.RecordCount != 0 || len(dataset.Documents) != 0 {
		t.Fatalf("remove: %+v, %v", dataset, err)
	}
	after := request(http.MethodGet, path+"/versions/2/download", "", http.StatusOK)
	if after.Body.String() != originalDownload {
		t.Fatalf("historical download changed: %s", after.Body)
	}
	request(http.MethodGet, path+"/versions/99/download", "", http.StatusNotFound)
	request(http.MethodPost, "/v1/training/datasets", `{"name":""}`, http.StatusBadRequest)
}

func uploadRequest(t *testing.T, fields map[string][]string, data string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for field, names := range fields {
		for _, name := range names {
			part, err := form.CreateFormFile(field, name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(part, data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/training/uploads", &body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	return r
}

func TestUploadOnlyRoutes(t *testing.T) {
	if os.Getenv("MOXIE_UPLOAD_TESTS") != "1" {
		t.Skip("set MOXIE_UPLOAD_TESTS=1 to run upload regressions")
	}
	store, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		store  *uploads.Store
		status int
	}{
		{"no-dependencies", nil, http.StatusNotFound},
		{"store-only", store, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			log := logger.New(io.Discard, logger.LevelInfo, "upload-only-test", nil)
			api := web.NewApp(log.Info, mid.Errors(log))
			trainapp.Routes(api, trainapp.Config{Uploads: test.store, AuthorizationMode: auth.Open})
			w := httptest.NewRecorder()
			api.ServeHTTP(w, uploadRequest(t, map[string][]string{"file": {"harbor.txt"}}, "The harbor opens at dawn."))
			if w.Code != test.status {
				t.Fatalf("upload: %d %s", w.Code, w.Body)
			}
			for _, route := range []struct{ method, path string }{
				{http.MethodPost, "/v1/training/runs"},
				{http.MethodGet, "/v1/training/runs"},
				{http.MethodGet, statePath},
				{http.MethodPost, statePath + "/preview/continue"},
				{http.MethodPost, statePath + "/start-training"},
				{http.MethodGet, statePath + "/evaluation"},
				{http.MethodGet, statePath + "/adapter"},
			} {
				w := httptest.NewRecorder()
				api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), route.method, route.path, nil))
				if w.Code != http.StatusServiceUnavailable || !json.Valid(w.Body.Bytes()) || !strings.Contains(w.Body.String(), "check server startup logs") {
					t.Fatalf("%s %s: %d %s", route.method, route.path, w.Code, w.Body)
				}
			}
		})
	}
}

func TestUpload(t *testing.T) {
	api, store := testAPI(t, new(mocks.Client))
	// Storage must preserve bytes, not parse or process dataset contents.
	data := "not yet a valid dataset\n\x00unchanged"
	w := httptest.NewRecorder()
	api.ServeHTTP(w, uploadRequest(t, map[string][]string{"file": {"source.jsonl"}}, data))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	var got trainapp.Upload
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.ID, "upload-") || got.Name != "source.jsonl" || got.Format != ".jsonl" || got.SizeBytes != int64(len(data)) || got.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(data))) || got.CreatedAt.IsZero() {
		t.Fatalf("metadata: %+v", got)
	}
	source, err := store.Open(t.Context(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	stored, err := io.ReadAll(source)
	if err != nil || string(stored) != data {
		t.Fatalf("stored data: %q, error: %v", stored, err)
	}
}

func TestUploadValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		fields map[string][]string
		data   string
		status int
	}{
		{"at-limit", map[string][]string{"file": {"source.jsonl"}}, "12345678", http.StatusOK},
		{"over-limit", map[string][]string{"file": {"source.jsonl"}}, "123456789", http.StatusBadRequest},
		{"empty", map[string][]string{"file": {"source.jsonl"}}, "", http.StatusBadRequest},
		{"missing", nil, "", http.StatusBadRequest},
		{"wrong-field", map[string][]string{"files": {"source.jsonl"}}, "data", http.StatusBadRequest},
		{"duplicate", map[string][]string{"file": {"a.jsonl", "b.jsonl"}}, "data", http.StatusBadRequest},
		{"extra-file", map[string][]string{"file": {"a.jsonl"}, "other": {"b.jsonl"}}, "data", http.StatusBadRequest},
		{"unsafe-name", map[string][]string{"file": {`..\source.jsonl`}}, "data", http.StatusBadRequest},
		{"request-limit", map[string][]string{"file": {"source.jsonl"}}, strings.Repeat("x", (1<<20)+9), http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			store, err := uploads.New(uploads.Config{Root: root, MaximumBytes: 8})
			if err != nil {
				t.Fatal(err)
			}
			log := logger.New(io.Discard, logger.LevelInfo, "upload-test", nil)
			api := web.NewApp(log.Info, mid.Errors(log))
			trainapp.Routes(api, trainapp.Config{Uploads: store, AuthorizationMode: auth.Open})
			w := httptest.NewRecorder()
			api.ServeHTTP(w, uploadRequest(t, test.fields, test.data))
			if w.Code != test.status {
				t.Fatalf("upload: %d %s", w.Code, w.Body)
			}
			if test.status == http.StatusOK {
				return
			}
			entries, err := os.ReadDir(filepath.Join(root, "uploads"))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("rejected upload left stored files: %v", entries)
			}
		})
	}
	api, _ := testAPI(t, new(mocks.Client))
	for _, contentType := range []string{"application/json", "multipart/form-data; boundary=missing"} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/training/uploads", strings.NewReader("invalid body"))
		r.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("malformed request: %d %s", w.Code, w.Body)
		}
	}
}

func TestReadState(t *testing.T) {
	c := new(mocks.Client)
	api, _ := testAPI(t, c)
	stamp := time.Date(2026, 10, 27, 15, 4, 5, 0, time.UTC)
	state := tmprl.Run[training.Request, training.Artifacts]{
		Status: tmprl.StatusPaused, Stage: "preparation", Step: "generate", StartedAt: stamp, UpdatedAt: stamp,
		Request: training.Request{Model: "/private/model.gguf"}, WorkerHost: "private-worker",
		Stages: []tmprl.StageState{{Name: "preparation", Status: tmprl.StatusPaused, Steps: []tmprl.StepState{
			{Name: "extract", Status: tmprl.StatusCompleted, Attempt: 1, ClosedAt: stamp},
			{Name: "generate", Status: tmprl.StatusPaused, Attempt: 3},
		}}, {Name: "training", Status: tmprl.StatusQueued, Steps: []tmprl.StepState{
			{Name: "split", Status: tmprl.StatusQueued},
		}}},
		Progress: &tmprl.Progress{Stage: "preparation", Step: "generate", Completed: 7, Total: 23, Message: "private progress detail"},
	}
	payload, err := converter.GetDefaultDataConverter().ToPayload(state)
	if err != nil {
		t.Fatal(err)
	}
	c.On("DescribeWorkflowExecution", mock.Anything, "training-example", "run-2").Return(&workflowservice.DescribeWorkflowExecutionResponse{
		WorkflowExecutionInfo: &workflowpb.WorkflowExecutionInfo{
			Execution: &commonpb.WorkflowExecution{WorkflowId: "training-example", RunId: "run-2"},
			Type:      &commonpb.WorkflowType{Name: "moxie.training.Job"}, Status: enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING,
			Memo: &commonpb.Memo{Fields: map[string]*commonpb.Payload{"moxie.training.job": payload}},
		},
		PendingWorkflowTask: &workflowpb.PendingWorkflowTaskInfo{},
	}, nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath, nil))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response: %d %s", w.Code, w.Body)
	}
	var got trainapp.Run
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.WorkflowID != "training-example" || got.RunID != "run-2" || got.Status != "paused" || got.Stage != "preparation" || got.Step != "generate" || !got.StateStale || !got.StartedAt.Equal(stamp) || !got.UpdatedAt.Equal(stamp) {
		t.Fatalf("unexpected run: %+v", got)
	}
	if len(got.Stages) != 2 || len(got.Stages[0].Steps) != 2 || got.Stages[0].Name != "preparation" || got.Stages[0].Status != "paused" || got.Stages[0].Steps[0].Name != "extract" || got.Stages[0].Steps[0].Status != "completed" || !got.Stages[0].Steps[0].ClosedAt.Equal(stamp) || got.Stages[0].Steps[1].Name != "generate" || got.Stages[0].Steps[1].Status != "paused" || got.Stages[0].Steps[1].Attempt != 3 {
		t.Fatalf("unexpected stage state: %+v", got.Stages)
	}
	if got.Stages[1].Name != "training" || got.Stages[1].Status != "queued" || len(got.Stages[1].Steps) != 1 || got.Stages[1].Steps[0].Name != "split" || got.Stages[1].Steps[0].Status != "queued" {
		t.Fatalf("unexpected next stage: %+v", got.Stages[1])
	}
	if got.Progress.Stage != "preparation" || got.Progress.Step != "generate" || got.Progress.Completed != 7 || got.Progress.Total != 23 {
		t.Fatalf("unexpected progress: %+v", got.Progress)
	}
	for _, private := range []string{"private", "request", "artifacts", "definition", "worker_host"} {
		if strings.Contains(w.Body.String(), private) {
			t.Errorf("response exposes %q: %s", private, w.Body)
		}
	}
}

func TestReadProgressEstimate(t *testing.T) {
	for _, mode := range []string{"running", "paused", "stale", "wrong step", "old heartbeat"} {
		t.Run(mode, func(t *testing.T) {
			c := new(mocks.Client)
			api, _ := testAPI(t, c)
			start := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
			e := tmprl.Execution{WorkflowID: "training-example", RunID: "run-2"}
			state := tmprl.Run[training.Request, training.Artifacts]{Execution: e, Status: tmprl.StatusRunning, Stage: "preparation", Step: "generate"}
			progress := tmprl.Progress{Execution: e, Stage: "preparation", Step: "generate", InitialCompleted: 12, Completed: 16, Total: 20, StartedAt: start, UpdatedAt: start.Add(2 * time.Minute)}
			if mode == "paused" {
				state.Status = tmprl.StatusPaused
			}
			if mode == "wrong step" {
				progress.Step = "verify"
			}
			if mode == "old heartbeat" {
				progress.StartedAt, progress.UpdatedAt = time.Time{}, time.Time{}
			}
			payload, err := converter.GetDefaultDataConverter().ToPayload(state)
			if err != nil {
				t.Fatal(err)
			}
			heartbeat, err := converter.GetDefaultDataConverter().ToPayloads(progress)
			if err != nil {
				t.Fatal(err)
			}
			description := &workflowservice.DescribeWorkflowExecutionResponse{
				WorkflowExecutionInfo: &workflowpb.WorkflowExecutionInfo{
					Execution: &commonpb.WorkflowExecution{WorkflowId: e.WorkflowID, RunId: e.RunID},
					Type:      &commonpb.WorkflowType{Name: "moxie.training.Job"}, Status: enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING,
					Memo: &commonpb.Memo{Fields: map[string]*commonpb.Payload{"moxie.training.job": payload}},
				},
				PendingActivities: []*workflowpb.PendingActivityInfo{{ActivityType: &commonpb.ActivityType{Name: "moxie.training.Step"}, HeartbeatDetails: heartbeat}},
			}
			if mode == "stale" {
				description.PendingWorkflowTask = &workflowpb.PendingWorkflowTaskInfo{}
			}
			c.On("DescribeWorkflowExecution", mock.Anything, e.WorkflowID, e.RunID).Return(description, nil)
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath, nil))
			var got trainapp.Run
			if w.Code != http.StatusOK {
				t.Fatalf("response: %d %s", w.Code, w.Body)
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			want := int64(0)
			if mode == "running" {
				want = 120
			}
			if got.Progress.RemainingSeconds != want {
				t.Fatalf("remaining seconds = %d, want %d", got.Progress.RemainingSeconds, want)
			}
		})
	}
}

func TestDownloadDataset(t *testing.T) {
	for _, name := range []string{"ready", "small test dataset", "training failed", "not prepared", "wrong source", "missing artifact", "foreign reference", "missing file"} {
		t.Run(name, func(t *testing.T) {
			api, state, path := readyTrainingAPI(t, new(mocks.Client))
			state.Artifacts.Preparation.RecordsPath = "/private/worker/dataset.jsonl"
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			available, ready, status := true, true, http.StatusOK
			switch name {
			case "small test dataset":
				state.Artifacts.Preparation.TrainingReady = false
				state.Artifacts.Preparation.RecordCount = 5
				ready = false
			case "training failed":
				state.Status = tmprl.StatusFailed
			case "not prepared":
				state.Artifacts.Preparation = nil
				available, ready, status = false, false, http.StatusNotFound
			case "wrong source":
				state.Artifacts.Preparation.DatasetID = "another-source"
				available, ready, status = false, false, http.StatusNotFound
			case "missing artifact":
				state.Artifacts.Preparation.RecordsPath = ""
				available, ready, status = false, false, http.StatusNotFound
			case "foreign reference":
				foreign := filepath.Join(t.TempDir(), "other.jsonl")
				if err := os.WriteFile(foreign, []byte("foreign data"), 0600); err != nil {
					t.Fatal(err)
				}
				state.Artifacts.Preparation.RecordsPath = foreign
			case "missing file":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				status = http.StatusInternalServerError
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath+"/dataset", nil))
			if w.Code != status || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("dataset: %d %s", w.Code, w.Body)
			}
			if status == http.StatusOK {
				if !bytes.Equal(w.Body.Bytes(), want) || w.Header().Get("Content-Disposition") != `attachment; filename="dataset.jsonl"` || w.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatalf("incorrect dataset download: %v %s", w.Header(), w.Body)
				}
			} else if w.Header().Get("Content-Disposition") != "" || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("unsafe error response: %s", w.Body)
			}
			w = httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath, nil))
			var run trainapp.Run
			if err := json.Unmarshal(w.Body.Bytes(), &run); err != nil || w.Code != http.StatusOK || run.DatasetAvailable != available || run.TrainingReady != ready {
				t.Fatalf("dataset state: %+v, %v (%d)", run, err, w.Code)
			}
			if state.Artifacts.Preparation != nil && run.DatasetRecordCount != state.Artifacts.Preparation.RecordCount {
				t.Fatalf("incorrect prepared record count: %d", run.DatasetRecordCount)
			}
			if strings.Contains(w.Body.String(), "/private/") {
				t.Fatalf("state leaked artifact path: %s", w.Body)
			}
		})
	}
}

func TestDownloadAdapter(t *testing.T) {
	for _, name := range []string{"published", "rejected", "running", "waiting", "failed", "not accepted", "no report", "no adapter", "missing file", "foreign reference"} {
		t.Run(name, func(t *testing.T) {
			api, state, datasetPath := readyTrainingAPI(t, new(mocks.Client))
			state.Status = tmprl.StatusCompleted
			state.Artifacts.Training.Accepted = true
			state.Artifacts.Training.Report = "/private/worker/evaluation.json"
			state.Artifacts.Training.Adapter = "/private/worker/adapter.gguf"
			path := filepath.Join(filepath.Dir(filepath.Dir(datasetPath)), "training", "adapter.gguf")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			want := []byte{'G', 'G', 'U', 'F', 0, 255, 1, 128, '\n'}
			if name != "missing file" {
				if err := os.WriteFile(path, want, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			wantStatus := http.StatusNotFound
			switch name {
			case "published":
				wantStatus = http.StatusOK
			case "foreign reference":
				foreign := filepath.Join(t.TempDir(), "other.gguf")
				if err := os.WriteFile(foreign, []byte("wrong adapter"), 0o600); err != nil {
					t.Fatal(err)
				}
				state.Artifacts.Training.Adapter = foreign
				wantStatus = http.StatusOK
			case "rejected":
				state.Status = optimization.StatusNeedsImprovement
			case "running":
				state.Status = tmprl.StatusRunning
			case "waiting":
				state.Status = tmprl.StatusWaiting
			case "failed":
				state.Status = tmprl.StatusFailed
			case "not accepted":
				state.Artifacts.Training.Accepted = false
			case "no report":
				state.Artifacts.Training.Report = ""
			case "no adapter":
				state.Artifacts.Training.Adapter = ""
			case "missing file":
				wantStatus = http.StatusInternalServerError
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath+"/adapter", nil))
			if w.Code != wantStatus || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("adapter: %d %q", w.Code, w.Body.Bytes())
			}
			if wantStatus == http.StatusOK {
				if !bytes.Equal(w.Body.Bytes(), want) {
					t.Fatalf("adapter bytes: %v; want %v", w.Body.Bytes(), want)
				}
				for key, value := range map[string]string{
					"Content-Type": "application/octet-stream", "Content-Disposition": `attachment; filename="adapter.gguf"`, "X-Content-Type-Options": "nosniff",
				} {
					if w.Header().Get(key) != value {
						t.Errorf("%s: %q; want %q", key, w.Header().Get(key), value)
					}
				}
			} else if bytes.Contains(w.Body.Bytes(), want) || w.Header().Get("Content-Disposition") != "" {
				t.Fatalf("error response served an adapter: %s", w.Body)
			}
			for _, private := range []string{"private", path, state.Artifacts.Training.Adapter} {
				if private != "" && strings.Contains(w.Body.String(), private) {
					t.Errorf("adapter response leaked %q", private)
				}
			}
			w = httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath, nil))
			var run trainapp.Run
			wantAvailable := name == "published" || name == "foreign reference" || name == "missing file"
			if err := json.Unmarshal(w.Body.Bytes(), &run); err != nil || w.Code != http.StatusOK || run.AdapterAvailable != wantAvailable {
				t.Fatalf("adapter availability: %+v, %v (%d); want %t", run, err, w.Code, wantAvailable)
			}
			if !strings.Contains(w.Body.String(), `"adapter_available":`) {
				t.Errorf("availability field missing: %s", w.Body)
			}
			for _, private := range []string{"private", path, state.Artifacts.Training.Adapter} {
				if private != "" && strings.Contains(w.Body.String(), private) {
					t.Errorf("run response leaked %q", private)
				}
			}
		})
	}
}

func TestDownloadAdapterErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{"missing", serviceerror.NewNotFound("private detail"), http.StatusNotFound},
		{"wrong workflow", tmprl.ErrUnsupportedWorkflow, http.StatusNotFound},
		{"unavailable", serviceerror.NewUnavailable("private detail"), http.StatusServiceUnavailable},
		{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"cancelled", context.Canceled, http.StatusGatewayTimeout},
		{"unexpected", errors.New("private backend detail"), http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := new(mocks.Client)
			api, _ := testAPI(t, c)
			c.On("DescribeWorkflowExecution", mock.Anything, "training-example", "run-2").Return((*workflowservice.DescribeWorkflowExecutionResponse)(nil), test.err)
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath+"/adapter", nil))
			if w.Code != test.status || strings.Contains(w.Body.String(), "private") || w.Header().Get("Content-Disposition") != "" {
				t.Fatalf("adapter error: %d %s", w.Code, w.Body)
			}
		})
	}
	api, _ := testAPI(t, new(mocks.Client))
	for _, path := range []string{"/v1/training/runs/training-example//adapter", "/v1/training/runs//run-2/adapter"} {
		w := httptest.NewRecorder()
		api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		if w.Code == http.StatusOK {
			t.Fatalf("incomplete execution succeeded: %s", path)
		}
	}
}

func TestDownloadAdapterWriteFailure(t *testing.T) {
	api, state, datasetPath := readyTrainingAPI(t, new(mocks.Client))
	state.Status = tmprl.StatusCompleted
	state.Artifacts.Training.Accepted = true
	state.Artifacts.Training.Report = "evaluation.json"
	state.Artifacts.Training.Adapter = "adapter.gguf"
	path := filepath.Join(filepath.Dir(filepath.Dir(datasetPath)), "training", "adapter.gguf")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte{0, 255, 17}, 32768), 0o600); err != nil {
		t.Fatal(err)
	}
	w := &failedDownloadWriter{ResponseRecorder: httptest.NewRecorder()}
	api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath+"/adapter", nil))
	if w.Code != http.StatusOK || w.writes != 1 || !bytes.Equal(w.Body.Bytes(), []byte{0, 255, 17}) {
		t.Fatalf("failed download: status=%d writes=%d body=%q", w.Code, w.writes, w.Body.Bytes())
	}
}

type failedDownloadWriter struct {
	*httptest.ResponseRecorder
	writes int
}

func (w *failedDownloadWriter) Write(data []byte) (int, error) {
	w.writes++
	n, _ := w.ResponseRecorder.Write(data[:min(3, len(data))])
	return n, errors.New("client write failed")
}

func TestReadEvaluation(t *testing.T) {
	for _, name := range []string{"accepted", "rejected", "not ready", "missing file", "malformed"} {
		t.Run(name, func(t *testing.T) {
			api, state, datasetPath := readyTrainingAPI(t, new(mocks.Client))
			state.Artifacts.Training.Report = "/private/worker/evaluation.json"
			path := filepath.Join(filepath.Dir(filepath.Dir(datasetPath)), "training", "evaluation.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			want := trainapp.Evaluation{
				ScoreKind: "normalized_exact_match", BaseCorrect: 4, AdapterCorrect: 12, Total: 20,
				Answers: []trainapp.EvaluationAnswer{{Prompt: "When does the harbor open?", Expected: "The harbor opens at dawn.", BaseAnswer: "At dawn.", AdapterAnswer: "The harbor opens at sunset."}},
			}
			wantStatus := http.StatusOK
			switch name {
			case "accepted":
				state.Status = tmprl.StatusCompleted
				want.Accepted, want.AdapterCorrect = true, 19
			case "rejected":
				state.Status = optimization.StatusNeedsImprovement
			case "not ready":
				state.Artifacts.Training.Report = ""
				wantStatus = http.StatusNotFound
			case "missing file", "malformed":
				wantStatus = http.StatusInternalServerError
			}
			data, err := json.Marshal(want)
			if err != nil {
				t.Fatal(err)
			}
			if name == "malformed" {
				data = []byte(`{"private":`)
			}
			if name != "missing file" {
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath+"/evaluation", nil))
			if w.Code != wantStatus || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("evaluation: %d %s", w.Code, w.Body)
			}
			if wantStatus == http.StatusOK {
				var got trainapp.Evaluation
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("report: %+v, %v; want %+v", got, err, want)
				}
			}
			for _, private := range []string{"private", datasetPath, "worker_host", "artifacts"} {
				if strings.Contains(w.Body.String(), private) {
					t.Errorf("evaluation leaked %q: %s", private, w.Body)
				}
			}
			w = httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath, nil))
			var run trainapp.Run
			if err := json.Unmarshal(w.Body.Bytes(), &run); err != nil || w.Code != http.StatusOK || run.EvaluationAvailable != (name != "not ready") {
				t.Fatalf("availability: %+v, %v (%d)", run, err, w.Code)
			}
		})
	}
}

func TestReadEvaluationErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{"missing", serviceerror.NewNotFound("private detail"), http.StatusNotFound},
		{"wrong workflow", tmprl.ErrUnsupportedWorkflow, http.StatusNotFound},
		{"unavailable", serviceerror.NewUnavailable("private detail"), http.StatusServiceUnavailable},
		{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"cancelled", context.Canceled, http.StatusGatewayTimeout},
		{"unexpected", errors.New("private backend detail"), http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := new(mocks.Client)
			api, _ := testAPI(t, c)
			c.On("DescribeWorkflowExecution", mock.Anything, "training-example", "run-2").Return((*workflowservice.DescribeWorkflowExecutionResponse)(nil), test.err)
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath+"/evaluation", nil))
			if w.Code != test.status || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("evaluation error: %d %s", w.Code, w.Body)
			}
		})
	}
	api, _ := testAPI(t, new(mocks.Client))
	for _, path := range []string{"/v1/training/runs/training-example//evaluation", "/v1/training/runs//run-2/evaluation"} {
		w := httptest.NewRecorder()
		api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		if w.Code == http.StatusOK {
			t.Fatalf("incomplete execution succeeded: %s", path)
		}
	}
}

func TestReadPreview(t *testing.T) {
	review := &tmprl.Checkpoint{ID: "run-2/preparation/preview/preview-review", Name: "preview-review", Stage: "preparation", Step: "preview"}
	startTraining := &tmprl.Checkpoint{ID: "run-2/preparation//start-training", Name: "start-training", Stage: "preparation"}
	const previewJSON = `{"records":[{"prompt":"When does the harbor open?","target":"dawn","evidence":"The harbor opens at dawn.","source_document":"upload-source"},{"prompt":"When does the ferry leave?","target":"noon","evidence":"The ferry leaves at noon.","source_document":"upload-source"}],"warnings":["record 2: grounding"]}`
	for _, test := range []struct {
		name           string
		preview        bool
		status         tmprl.Status
		step           string
		checkpoint     *tmprl.Checkpoint
		wantCheckpoint string
	}{
		{name: "before preview", status: tmprl.StatusRunning, step: "extract"},
		{name: "review", preview: true, status: tmprl.StatusWaiting, step: "preview", checkpoint: review,
			wantCheckpoint: `{"id":"run-2/preparation/preview/preview-review","name":"preview-review","stage":"preparation","step":"preview"}`},
		{name: "continued", preview: true, status: tmprl.StatusRunning, step: "generate"},
		{name: "training choice", preview: true, status: tmprl.StatusWaiting, checkpoint: startTraining,
			wantCheckpoint: `{"id":"run-2/preparation//start-training","name":"start-training","stage":"preparation"}`},
		{name: "closed", preview: true, status: tmprl.StatusCompleted, checkpoint: review},
	} {
		for _, list := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/list=%t", test.name, list), func(t *testing.T) {
				c := new(mocks.Client)
				api, _ := testAPI(t, c)
				state := tmprl.Run[training.Request, training.Artifacts]{
					Status: test.status, Stage: "preparation", Step: test.step, Checkpoint: test.checkpoint,
					Request: training.Request{Model: "/private/model.gguf"}, WorkerHost: "private-worker", Error: "private backend failure",
					Decisions: []tmprl.Decision{{CheckpointID: review.ID, SubmissionID: "private-receipt", Payload: json.RawMessage(`{"private":"input"}`)}},
				}
				if test.preview {
					state.Artifacts.Preview = &preparation.PreparationResult{
						RecordsPath: "/private/records", ReportPath: "/private/report", Warnings: []string{"record 2: grounding"},
						Preview: []optimization.Record{
							{Prompt: "When does the harbor open?", Target: "dawn", Evidence: "The harbor opens at dawn.", SourceDocument: "upload-source", FactID: "private-fact", SourceChunkID: "private-chunk", ValidationPrompt: true, IndependentPrompt: true, UserApproved: true},
							{Prompt: "When does the ferry leave?", Target: "noon", Evidence: "The ferry leaves at noon.", SourceDocument: "upload-source"},
						},
					}
				}
				payload, err := converter.GetDefaultDataConverter().ToPayload(state)
				if err != nil {
					t.Fatal(err)
				}
				info := &workflowpb.WorkflowExecutionInfo{
					Execution: &commonpb.WorkflowExecution{WorkflowId: "training-example", RunId: "run-2"},
					Type:      &commonpb.WorkflowType{Name: "moxie.training.Job"}, Status: enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING,
					Memo: &commonpb.Memo{Fields: map[string]*commonpb.Payload{"moxie.training.job": payload}},
				}
				if test.status.Terminal() {
					info.Status = enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED
				}
				path := statePath
				if list {
					path = "/v1/training/runs"
					c.On("ListWorkflow", mock.Anything, mock.Anything).Return(&workflowservice.ListWorkflowExecutionsResponse{Executions: []*workflowpb.WorkflowExecutionInfo{info}}, nil)
				} else {
					c.On("DescribeWorkflowExecution", mock.Anything, "training-example", "run-2").Return(&workflowservice.DescribeWorkflowExecutionResponse{WorkflowExecutionInfo: info}, nil)
					if test.status.Terminal() {
						history := new(mocks.HistoryEventIterator)
						history.On("HasNext").Return(false)
						c.On("GetWorkflowHistory", mock.Anything, "training-example", "run-2", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT).Return(history)
					}
				}
				w := httptest.NewRecorder()
				api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
				if w.Code != http.StatusOK {
					t.Fatalf("response: %d %s", w.Code, w.Body)
				}
				var got map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if list {
					got = got["runs"].([]any)[0].(map[string]any)
				}
				wantPreview := ""
				if test.preview {
					wantPreview = previewJSON
				}
				for field, expected := range map[string]string{"preview": wantPreview, "checkpoint": test.wantCheckpoint} {
					value, present := got[field]
					if expected == "" {
						if present {
							t.Errorf("unexpected %s: %+v", field, value)
						}
						continue
					}
					var want map[string]any
					if err := json.Unmarshal([]byte(expected), &want); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(value, want) {
						t.Errorf("%s: got %+v, want %+v", field, value, want)
					}
				}
				for _, internal := range []string{"private", "records_path", "report_path", "fact_id", "source_chunk_id", "validation_prompt", "independent_prompt", "user_approved", "decisions"} {
					if strings.Contains(w.Body.String(), internal) {
						t.Errorf("response exposes %q: %s", internal, w.Body)
					}
				}
			})
		}
	}
}

func reviewAPI(t *testing.T, c *mocks.Client) (*web.App, *tmprl.Run[training.Request, training.Artifacts]) {
	t.Helper()
	api, _ := testAPI(t, c)
	state := &tmprl.Run[training.Request, training.Artifacts]{
		Status: tmprl.StatusWaiting, Stage: "preparation", Step: "preview",
		Checkpoint: &tmprl.Checkpoint{ID: "run-2/preparation/preview/preview-review", Name: "preview-review", Stage: "preparation", Step: "preview"},
		Request:    training.Request{Prepare: &preparation.PreparationRequest{DatasetID: "upload-source", Goal: preparation.GoalQuestionAnswer}},
		Artifacts: training.Artifacts{Preview: &preparation.PreparationResult{DatasetID: "upload-source", Preview: []optimization.Record{
			{Prompt: "When does the harbor open?", Target: "The harbor opens at dawn.", Evidence: "The harbor opens at dawn.", SourceDocument: "upload-source"},
			{Prompt: "When does the ferry leave?", Target: "The ferry leaves at noon.", Evidence: "The ferry leaves at noon.", SourceDocument: "upload-source"},
		}}},
	}
	c.On("DescribeWorkflowExecution", mock.Anything, "training-example", "run-2").Return(
		func(context.Context, string, string) *workflowservice.DescribeWorkflowExecutionResponse {
			payload, err := converter.GetDefaultDataConverter().ToPayload(state)
			if err != nil {
				t.Fatal(err)
			}
			status := enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING
			if state.Status.Terminal() {
				status = enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED
			}
			return &workflowservice.DescribeWorkflowExecutionResponse{WorkflowExecutionInfo: &workflowpb.WorkflowExecutionInfo{
				Execution: &commonpb.WorkflowExecution{WorkflowId: "training-example", RunId: "run-2"},
				Type:      &commonpb.WorkflowType{Name: "moxie.training.Job"}, Status: status,
				Memo: &commonpb.Memo{Fields: map[string]*commonpb.Payload{"moxie.training.job": payload}},
			}}
		}, nil)
	history := new(mocks.HistoryEventIterator)
	history.On("HasNext").Return(false)
	c.On("GetWorkflowHistory", mock.Anything, "training-example", "run-2", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT).Return(history)
	return api, state
}

func TestContinuePreview(t *testing.T) {
	c := new(mocks.Client)
	api, state := reviewAPI(t, c)
	handle := new(mocks.WorkflowUpdateHandle)
	var accepted tmprl.Decision
	c.On("UpdateWorkflow", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		options := args.Get(1).(client.UpdateWorkflowOptions)
		if options.WorkflowID != "training-example" || options.RunID != "run-2" {
			t.Errorf("wrong review execution: %+v", options)
		}
		accepted = options.Args[0].(tmprl.Decision)
		var review preparation.Review
		if err := json.Unmarshal(accepted.Payload, &review); err != nil {
			t.Fatal(err)
		}
		if len(review.Records) != 2 || review.Records[0].Prompt != "At what time does the harbor open?" || review.Records[0].Target != "At dawn." || review.Records[1].Target != "At noon." || review.Records[0].Evidence != "The harbor opens at dawn." || review.Records[1].Evidence != "The ferry leaves at noon." || review.Records[0].SourceDocument != "upload-source" || !review.Records[0].UserApproved || review.Records[0].FactID == "" {
			t.Errorf("review edits were not validated and retained: %+v", review)
		}
	}).Return(handle, nil).Once()
	handle.On("Get", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		*args.Get(1).(*tmprl.Decision) = accepted
		state.Decisions = []tmprl.Decision{accepted}
		state.Checkpoint = nil
		state.Status = tmprl.StatusRunning
	}).Return(nil)
	want := map[string]string{"checkpoint_id": "run-2/preparation/preview/preview-review", "submission_id": "review-1"}
	for _, phase := range []string{"accept", "retry", "closed retry", "conflict", "changed edits"} {
		if phase == "closed retry" {
			state.Status = tmprl.StatusCompleted
		}
		body := continueBody
		if phase == "accept" {
			body += strings.Repeat(" ", (64<<10)-len(body))
		}
		if phase == "conflict" {
			body = strings.Replace(body, "review-1", "review-2", 1)
		}
		if phase == "changed edits" {
			body = strings.Replace(body, "At what time does the harbor open?", "When is the harbor open?", 1)
		}
		w := httptest.NewRecorder()
		api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, statePath+"/preview/continue", strings.NewReader(body)))
		if phase == "conflict" || phase == "changed edits" {
			if w.Code != http.StatusBadRequest {
				t.Fatalf("conflicting retry: %d %s", w.Code, w.Body)
			}
			continue
		}
		var got map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s receipt: %d %s, %v", phase, w.Code, w.Body, err)
		}
	}
	c.AssertExpectations(t)
}

func readyTrainingAPI(t *testing.T, c *mocks.Client) (*web.App, *tmprl.Run[training.Request, training.Artifacts], string) {
	t.Helper()
	store, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	job, err := training.New(c, training.Config{
		Uploads: store, StorageRoot: root, Models: map[string]string{"model-a": filepath.Join(t.TempDir(), "model.gguf")},
		// Authorization must not perform computation or load a model.
		Trainer: struct{ optimization.Trainer }{}, Completer: struct{ preparation.ModelCompleter }{},
		Log: func(context.Context, string, ...any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	log := logger.New(io.Discard, logger.LevelInfo, "trainapp-test", nil)
	api := web.NewApp(log.Info, mid.Errors(log))
	trainapp.Routes(api, trainapp.Config{Job: job, Uploads: store, AuthorizationMode: auth.Open})
	state := &tmprl.Run[training.Request, training.Artifacts]{
		Status: tmprl.StatusWaiting, Stage: "preparation",
		Checkpoint: &tmprl.Checkpoint{ID: "run-2/preparation//start-training", Name: "start-training", Stage: "preparation"},
		Request:    training.Request{Prepare: &preparation.PreparationRequest{DatasetID: "upload-source"}},
		Stages:     []tmprl.StageState{{Name: "preparation", Status: tmprl.StatusCompleted}, {Name: "training", Status: tmprl.StatusQueued}},
		Artifacts:  training.Artifacts{Preparation: &preparation.PreparationResult{DatasetID: "upload-source", TrainingReady: true, RecordCount: 50}},
	}
	path := filepath.Join(root, "training-jobs", "run-2", "steps", "preparation", "dataset.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	var records strings.Builder
	for i := range 50 {
		fmt.Fprintf(&records, "{\"prompt\":\"What code belongs to device %d?\",\"target\":\"C%d\",\"evidence\":\"Device %d has code C%d\"}\n", i, i, i, i)
	}
	if err := os.WriteFile(path, []byte(records.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	c.On("DescribeWorkflowExecution", mock.Anything, "training-example", "run-2").Return(
		func(context.Context, string, string) *workflowservice.DescribeWorkflowExecutionResponse {
			payload, err := converter.GetDefaultDataConverter().ToPayload(state)
			if err != nil {
				t.Fatal(err)
			}
			status := enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING
			switch {
			case state.Status == tmprl.StatusFailed:
				status = enumspb.WORKFLOW_EXECUTION_STATUS_FAILED
			case state.Status.Terminal():
				status = enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED
			}
			return &workflowservice.DescribeWorkflowExecutionResponse{WorkflowExecutionInfo: &workflowpb.WorkflowExecutionInfo{
				Execution: &commonpb.WorkflowExecution{WorkflowId: "training-example", RunId: "run-2"},
				Type:      &commonpb.WorkflowType{Name: "moxie.training.Job"}, Status: status,
				Memo: &commonpb.Memo{Fields: map[string]*commonpb.Payload{"moxie.training.job": payload}},
			}}
		}, nil)
	history := new(mocks.HistoryEventIterator)
	history.On("HasNext").Return(false)
	c.On("GetWorkflowHistory", mock.Anything, "training-example", "run-2", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT).Return(history)
	return api, state, path
}

func TestStartTraining(t *testing.T) {
	c := new(mocks.Client)
	api, state, path := readyTrainingAPI(t, c)
	handle := new(mocks.WorkflowUpdateHandle)
	var accepted tmprl.Decision
	c.On("UpdateWorkflow", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		options := args.Get(1).(client.UpdateWorkflowOptions)
		if options.WorkflowID != "training-example" || options.RunID != "run-2" || options.UpdateName != "training-control.decide" {
			t.Errorf("wrong authorization execution: %+v", options)
		}
		accepted = options.Args[0].(tmprl.Decision)
		if accepted.CheckpointID != "run-2/preparation//start-training" || accepted.SubmissionID != "training-1" || len(accepted.Payload) != 0 {
			t.Errorf("unexpected authorization: %+v", accepted)
		}
	}).Return(handle, nil).Once()
	handle.On("Get", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		*args.Get(1).(*tmprl.Decision) = accepted
		state.Decisions = []tmprl.Decision{accepted}
		state.Checkpoint = nil
		state.Stage = "training"
		state.Status = tmprl.StatusRunning
	}).Return(nil).Once()
	for _, phase := range []string{"accept", "lost response retry", "closed retry", "conflict"} {
		t.Run(phase, func(t *testing.T) {
			body := startTrainingBody
			switch phase {
			case "accept":
				body += strings.Repeat(" ", (64<<10)-len(body))
			case "lost response retry":
				// The accepted receipt remains retrievable without rereading files.
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "closed retry":
				state.Status = tmprl.StatusCompleted
			case "conflict":
				body = strings.Replace(body, "training-1", "training-2", 1)
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, statePath+"/start-training", strings.NewReader(body)))
			if phase == "conflict" {
				if w.Code != http.StatusBadRequest {
					t.Fatalf("conflicting authorization: %d %s", w.Code, w.Body)
				}
				return
			}
			var got map[string]string
			want := map[string]string{"checkpoint_id": "run-2/preparation//start-training", "submission_id": "training-1"}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK || !reflect.DeepEqual(got, want) {
				t.Fatalf("receipt: %d %s, %v", w.Code, w.Body, err)
			}
		})
	}
	c.AssertExpectations(t)
}

func TestStartTrainingRejectsBody(t *testing.T) {
	for _, test := range []struct{ name, body string }{
		{"empty", ""}, {"null", "null"}, {"array", "[]"}, {"malformed", "{"},
		{"missing IDs", `{}`},
		{"missing checkpoint", `{"submission_id":"training-1"}`},
		{"missing submission", `{"checkpoint_id":"run-2/preparation//start-training"}`},
		{"blank checkpoint", strings.Replace(startTrainingBody, `"run-2/preparation//start-training"`, `"  "`, 1)},
		{"blank submission", strings.Replace(startTrainingBody, `"training-1"`, `"  "`, 1)},
		{"long checkpoint", strings.Replace(startTrainingBody, "run-2/preparation//start-training", strings.Repeat("c", 513), 1)},
		{"long submission", strings.Replace(startTrainingBody, "training-1", strings.Repeat("s", 201), 1)},
		{"trailing JSON", startTrainingBody + " {}"},
		{"wrong type", strings.Replace(startTrainingBody, `"training-1"`, `42`, 1)},
		{"records", strings.Replace(startTrainingBody, `"submission_id":`, `"records":[],"submission_id":`, 1)},
		{"model", strings.Replace(startTrainingBody, `"submission_id":`, `"model_id":"model-a","submission_id":`, 1)},
		{"payload", strings.Replace(startTrainingBody, `"submission_id":`, `"payload":{},"submission_id":`, 1)},
		{"settings", strings.Replace(startTrainingBody, `"submission_id":`, `"settings":{},"submission_id":`, 1)},
		{"too large", startTrainingBody + strings.Repeat(" ", (64<<10)+1-len(startTrainingBody))},
	} {
		t.Run(test.name, func(t *testing.T) {
			api, _ := testAPI(t, new(mocks.Client))
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, statePath+"/start-training", strings.NewReader(test.body)))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("response: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestStartTrainingRejectsState(t *testing.T) {
	for _, name := range []string{"preview", "unfinished", "no checkpoint", "wrong checkpoint", "wrong stage", "wrong step", "terminal", "no preparation", "missing artifact", "not ready", "wrong dataset", "wrong count", "missing file", "invalid records", "insufficient records", "stale checkpoint", "cross-run checkpoint"} {
		t.Run(name, func(t *testing.T) {
			api, state, path := readyTrainingAPI(t, new(mocks.Client))
			body := startTrainingBody
			switch name {
			case "preview":
				state.Step = "preview"
				state.Checkpoint = &tmprl.Checkpoint{ID: "run-2/preparation/preview/preview-review", Name: "preview-review", Stage: "preparation", Step: "preview"}
				state.Stages[0].Status = tmprl.StatusWaiting
			case "unfinished":
				state.Stages[0].Status = tmprl.StatusRunning
			case "no checkpoint":
				state.Checkpoint = nil
			case "wrong checkpoint":
				state.Checkpoint.ID = "another-checkpoint"
			case "wrong stage":
				state.Checkpoint.Stage = "training"
			case "wrong step":
				state.Checkpoint.Step = "verify"
			case "terminal":
				state.Status = tmprl.StatusCompleted
			case "no preparation":
				state.Request.Prepare = nil
			case "missing artifact":
				state.Artifacts.Preparation = nil
			case "not ready":
				state.Artifacts.Preparation.TrainingReady = false
			case "wrong dataset":
				state.Artifacts.Preparation.DatasetID = "another-upload"
			case "wrong count":
				state.Artifacts.Preparation.RecordCount = 49
			case "missing file":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "invalid records", "insufficient records":
				data := "not JSON\n"
				if name == "insufficient records" {
					data = "{\"prompt\":\"When does the ferry leave?\",\"target\":\"noon\",\"evidence\":\"The ferry leaves at noon.\"}\n"
					state.Artifacts.Preparation.RecordCount = 1
				}
				if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			case "stale checkpoint":
				body = strings.Replace(body, "start-training", "preview-review", 1)
			case "cross-run checkpoint":
				body = strings.Replace(body, "run-2", "run-1", 1)
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, statePath+"/start-training", strings.NewReader(body)))
			want := http.StatusBadRequest
			if name == "missing file" {
				want = http.StatusInternalServerError
			}
			if w.Code != want || strings.Contains(w.Body.String(), `"submission_id"`) {
				t.Fatalf("response: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestStartTrainingBackendErrors(t *testing.T) {
	for _, phase := range []string{"read", "update", "receipt"} {
		for _, test := range []struct {
			name   string
			err    error
			status int
		}{
			{"missing", serviceerror.NewNotFound("private detail"), http.StatusNotFound},
			{"wrong workflow", tmprl.ErrUnsupportedWorkflow, http.StatusNotFound},
			{"unavailable", serviceerror.NewUnavailable("private detail"), http.StatusServiceUnavailable},
			{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout},
			{"temporal deadline", serviceerror.NewDeadlineExceeded("private detail"), http.StatusGatewayTimeout},
			{"unexpected", errors.New("backend failure"), http.StatusInternalServerError},
		} {
			t.Run(phase+"/"+test.name, func(t *testing.T) {
				c := new(mocks.Client)
				var api *web.App
				switch phase {
				case "read":
					api, _ = testAPI(t, c)
					c.On("DescribeWorkflowExecution", mock.Anything, "training-example", "run-2").Return(nil, test.err)
				case "update":
					api, _, _ = readyTrainingAPI(t, c)
					c.On("UpdateWorkflow", mock.Anything, mock.Anything).Return(nil, test.err)
				case "receipt":
					api, _, _ = readyTrainingAPI(t, c)
					handle := new(mocks.WorkflowUpdateHandle)
					c.On("UpdateWorkflow", mock.Anything, mock.Anything).Return(handle, nil)
					handle.On("Get", mock.Anything, mock.Anything).Return(test.err)
				}
				w := httptest.NewRecorder()
				api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, statePath+"/start-training", strings.NewReader(startTrainingBody)))
				if w.Code != test.status || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), `"submission_id"`) {
					t.Fatalf("response: %d %s", w.Code, w.Body)
				}
			})
		}
	}
}

func TestContinuePreviewRejectsBody(t *testing.T) {
	for _, test := range []struct{ name, body string }{
		{"empty", ""},
		{"null", "null"},
		{"array", "[]"},
		{"malformed", "{"},
		{"trailing", continueBody + " {}"},
		{"unknown field", strings.Replace(continueBody, `"submission_id":`, `"extra":true,"submission_id":`, 1)},
		{"wrong type", strings.Replace(continueBody, `"review-1"`, `42`, 1)},
		{"approval flag", strings.Replace(continueBody, `"prompt":`, `"user_approved":true,"prompt":`, 1)},
		{"validation flag", strings.Replace(continueBody, `"prompt":`, `"validation_prompt":true,"prompt":`, 1)},
		{"independent flag", strings.Replace(continueBody, `"prompt":`, `"independent_prompt":true,"prompt":`, 1)},
		{"too large", continueBody + strings.Repeat(" ", (64<<10)+1-len(continueBody))},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := new(mocks.Client)
			api, _ := testAPI(t, c)
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, statePath+"/preview/continue", strings.NewReader(test.body)))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("response: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestContinuePreviewRejectsReview(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*trainapp.ContinuePreview)
	}{
		{"blank checkpoint", func(r *trainapp.ContinuePreview) { r.CheckpointID = " " }},
		{"long checkpoint", func(r *trainapp.ContinuePreview) { r.CheckpointID = strings.Repeat("c", 513) }},
		{"blank submission", func(r *trainapp.ContinuePreview) { r.SubmissionID = " " }},
		{"long submission", func(r *trainapp.ContinuePreview) { r.SubmissionID = strings.Repeat("s", 201) }},
		{"wrong checkpoint", func(r *trainapp.ContinuePreview) { r.CheckpointID = "another-run/preparation/preview/preview-review" }},
		{"changed evidence", func(r *trainapp.ContinuePreview) { r.Records[0].Evidence = "The harbor opens at noon." }},
		{"changed source", func(r *trainapp.ContinuePreview) { r.Records[0].SourceDocument = "another-upload" }},
		{"reordered", func(r *trainapp.ContinuePreview) { r.Records[0], r.Records[1] = r.Records[1], r.Records[0] }},
		{"missing record", func(r *trainapp.ContinuePreview) { r.Records = r.Records[:1] }},
		{"null records", func(r *trainapp.ContinuePreview) { r.Records = nil }},
		{"blank prompt", func(r *trainapp.ContinuePreview) { r.Records[0].Prompt = " " }},
		{"blank target", func(r *trainapp.ContinuePreview) { r.Records[0].Target = " " }},
		{"duplicate prompts", func(r *trainapp.ContinuePreview) { r.Records[0].Prompt = r.Records[1].Prompt }},
	} {
		t.Run(test.name, func(t *testing.T) {
			api, _ := reviewAPI(t, new(mocks.Client))
			var request trainapp.ContinuePreview
			if err := json.Unmarshal([]byte(continueBody), &request); err != nil {
				t.Fatal(err)
			}
			test.edit(&request)
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, statePath+"/preview/continue", bytes.NewReader(body)))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("response: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestContinuePreviewRejectsState(t *testing.T) {
	for _, name := range []string{"not waiting", "wrong step", "missing preview", "wrong dataset", "no preparation", "terminal"} {
		t.Run(name, func(t *testing.T) {
			api, state := reviewAPI(t, new(mocks.Client))
			switch name {
			case "not waiting":
				state.Checkpoint = nil
				state.Status = tmprl.StatusRunning
			case "wrong step":
				state.Checkpoint.Step = "generate"
			case "missing preview":
				state.Artifacts.Preview = nil
			case "wrong dataset":
				state.Artifacts.Preview.DatasetID = "another-upload"
			case "no preparation":
				state.Request.Prepare = nil
			case "terminal":
				state.Status = tmprl.StatusCompleted
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, statePath+"/preview/continue", strings.NewReader(continueBody)))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("response: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestContinuePreviewBackendErrors(t *testing.T) {
	for _, phase := range []string{"read", "update", "receipt"} {
		for _, test := range []struct {
			name   string
			err    error
			status int
		}{
			{"missing", serviceerror.NewNotFound("private detail"), http.StatusNotFound},
			{"wrong workflow", tmprl.ErrUnsupportedWorkflow, http.StatusNotFound},
			{"unavailable", serviceerror.NewUnavailable("private detail"), http.StatusServiceUnavailable},
			{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout},
			{"temporal deadline", serviceerror.NewDeadlineExceeded("private detail"), http.StatusGatewayTimeout},
			{"unexpected", errors.New("private detail"), http.StatusInternalServerError},
		} {
			t.Run(phase+"/"+test.name, func(t *testing.T) {
				c := new(mocks.Client)
				var api *web.App
				switch phase {
				case "read":
					api, _ = testAPI(t, c)
					c.On("DescribeWorkflowExecution", mock.Anything, "training-example", "run-2").Return(nil, test.err)
				case "update":
					api, _ = reviewAPI(t, c)
					c.On("UpdateWorkflow", mock.Anything, mock.Anything).Return(nil, test.err)
				case "receipt":
					api, _ = reviewAPI(t, c)
					handle := new(mocks.WorkflowUpdateHandle)
					c.On("UpdateWorkflow", mock.Anything, mock.Anything).Return(handle, nil)
					handle.On("Get", mock.Anything, mock.Anything).Return(test.err)
				}
				w := httptest.NewRecorder()
				api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, statePath+"/preview/continue", strings.NewReader(continueBody)))
				if w.Code != test.status || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), `"submission_id"`) {
					t.Fatalf("response: %d %s", w.Code, w.Body)
				}
			})
		}
	}
}

func TestReadStateErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{"missing", fmt.Errorf("lookup: %w", serviceerror.NewNotFound("private detail")), http.StatusNotFound},
		{"other-job", tmprl.ErrUnsupportedWorkflow, http.StatusNotFound},
		{"unavailable", serviceerror.NewUnavailable("private detail"), http.StatusServiceUnavailable},
		{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"temporal-deadline", serviceerror.NewDeadlineExceeded("private detail"), http.StatusGatewayTimeout},
		{"unexpected", errors.New("private detail"), http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := new(mocks.Client)
			api, _ := testAPI(t, c)
			c.On("DescribeWorkflowExecution", mock.Anything, "training-example", "run-2").Return(nil, test.err)
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath, nil))
			if w.Code != test.status || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("response: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestReadStateRequiresBothIDs(t *testing.T) {
	api, _ := testAPI(t, new(mocks.Client))
	for _, path := range []string{"/v1/training/runs/training-example", "/v1/training/runs//run-2"} {
		w := httptest.NewRecorder()
		api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		if w.Code == http.StatusOK {
			t.Errorf("incomplete identity %s succeeded", path)
		}
	}
}

func TestStartTrainingRequiresBothIDs(t *testing.T) {
	api, _ := testAPI(t, new(mocks.Client))
	for _, path := range []string{"/v1/training/runs/training-example/start-training", "/v1/training/runs//run-2/start-training", "/v1/training/runs/training-example//start-training"} {
		w := httptest.NewRecorder()
		api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(startTrainingBody)))
		if w.Code == http.StatusOK {
			t.Errorf("incomplete identity %s succeeded", path)
		}
	}
}

func TestReadStateCancellation(t *testing.T) {
	for _, test := range []struct{ method, path, body string }{
		{http.MethodGet, statePath, ""},
		{http.MethodPost, statePath + "/preview/continue", continueBody},
		{http.MethodPost, statePath + "/start-training", startTrainingBody},
	} {
		t.Run(test.path, func(t *testing.T) {
			c := new(mocks.Client)
			api, _ := testAPI(t, c)
			c.On("DescribeWorkflowExecution", mock.Anything, "training-example", "run-2").Return(
				func(ctx context.Context, _, _ string) *workflowservice.DescribeWorkflowExecutionResponse {
					if !errors.Is(ctx.Err(), context.Canceled) {
						t.Error("request cancellation did not reach Temporal")
					}
					return nil
				}, context.Canceled)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(ctx, test.method, test.path, strings.NewReader(test.body)))
			if w.Body.Len() != 0 {
				t.Fatalf("response written after cancellation: %s", w.Body)
			}
			c.AssertExpectations(t)
		})
	}
}

func TestReadStateRequiresAuthorization(t *testing.T) {
	for _, test := range []struct {
		name   string
		code   codes.Code
		status int
	}{
		{"unauthenticated", codes.Unauthenticated, http.StatusUnauthorized},
		{"forbidden", codes.PermissionDenied, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			listener := bufconn.Listen(1024 * 1024)
			server := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, _ grpc.ServerStream) error {
				return status.Error(test.code, "authentication required")
			}))
			serveErr := make(chan error, 1)
			go func() { serveErr <- server.Serve(listener) }()
			t.Cleanup(func() {
				server.Stop()
				if err := <-serveErr; err != nil {
					t.Errorf("auth server: %v", err)
				}
			})
			log := logger.New(io.Discard, logger.LevelInfo, "trainapp-test", nil)
			client, err := authclient.New(log, "passthrough:///auth", authclient.WithDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return listener.DialContext(ctx)
			}))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			store, err := uploads.New(uploads.Config{Root: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			job, err := training.New(new(mocks.Client), training.Config{
				Uploads: store, StorageRoot: t.TempDir(), Models: map[string]string{"model-a": "/model.gguf"},
				Trainer: struct{ optimization.Trainer }{}, Log: func(context.Context, string, ...any) {},
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, mode := range []auth.Mode{auth.Management, auth.Authenticated, auth.FullProtected} {
				t.Run(mode.String(), func(t *testing.T) {
					t.Run("upload-only", func(t *testing.T) {
						if os.Getenv("MOXIE_UPLOAD_TESTS") != "1" {
							t.Skip("set MOXIE_UPLOAD_TESTS=1 to run upload regressions")
						}
						api := web.NewApp(log.Info, mid.Errors(log))
						trainapp.Routes(api, trainapp.Config{Uploads: store, AuthClient: client, AuthorizationMode: mode})
						w := httptest.NewRecorder()
						api.ServeHTTP(w, uploadRequest(t, map[string][]string{"file": {"harbor.txt"}}, "The harbor opens at dawn."))
						if w.Code != test.status {
							t.Fatalf("upload authorization: %d %s", w.Code, w.Body)
						}
						for _, request := range []*http.Request{
							httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath, nil),
							httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/training/runs", nil),
						} {
							w := httptest.NewRecorder()
							api.ServeHTTP(w, request)
							if w.Code != test.status {
								t.Fatalf("unavailable job authorization: %d %s", w.Code, w.Body)
							}
						}
					})
					api := web.NewApp(log.Info, mid.Errors(log))
					trainapp.Routes(api, trainapp.Config{Job: job, Uploads: store, AuthClient: client, AuthorizationMode: mode})
					for _, method := range []string{http.MethodGet, http.MethodPost} {
						path := statePath
						if method == http.MethodPost {
							path = "/v1/training/runs"
						}
						w := httptest.NewRecorder()
						api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), method, path, nil))
						if w.Code != test.status {
							t.Fatalf("%s response: %d %s", method, w.Code, w.Body)
						}
					}
					w := httptest.NewRecorder()
					api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, statePath+"/preview/continue", strings.NewReader(continueBody)))
					if w.Code != test.status {
						t.Fatalf("continue response: %d %s", w.Code, w.Body)
					}
					w = httptest.NewRecorder()
					api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, statePath+"/start-training", strings.NewReader(startTrainingBody)))
					if w.Code != test.status {
						t.Fatalf("start training response: %d %s", w.Code, w.Body)
					}
					w = httptest.NewRecorder()
					api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/training/runs", nil))
					if w.Code != test.status {
						t.Fatalf("list response: %d %s", w.Code, w.Body)
					}
					w = httptest.NewRecorder()
					api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath+"/evaluation", nil))
					if w.Code != test.status {
						t.Fatalf("evaluation response: %d %s", w.Code, w.Body)
					}
					w = httptest.NewRecorder()
					api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath+"/adapter", nil))
					if w.Code != test.status {
						t.Fatalf("adapter response: %d %s", w.Code, w.Body)
					}
					w = httptest.NewRecorder()
					api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, statePath+"/dataset", nil))
					if w.Code != test.status {
						t.Fatalf("dataset response: %d %s", w.Code, w.Body)
					}
					w = httptest.NewRecorder()
					api.ServeHTTP(w, uploadRequest(t, map[string][]string{"file": {"source.jsonl"}}, "data"))
					if w.Code != test.status {
						t.Fatalf("upload response: %d %s", w.Code, w.Body)
					}
				})
			}
		})
	}
}

func preparedDataset(t *testing.T, api *web.App) string {
	t.Helper()
	var data strings.Builder
	for i := range 50 {
		fmt.Fprintf(&data, "{\"prompt\":\"What code belongs to device %d?\",\"target\":\"C%d\",\"evidence\":\"Device %d has code C%d\"}\n", i, i, i, i)
	}
	w := httptest.NewRecorder()
	api.ServeHTTP(w, uploadRequest(t, map[string][]string{"file": {"prepared.jsonl"}}, data.String()))
	if w.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	var file trainapp.Upload
	if err := json.Unmarshal(w.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	return file.ID
}

func TestCreateTraining(t *testing.T) {
	c := new(mocks.Client)
	api, _ := testAPI(t, c)
	datasetID := preparedDataset(t, api)
	workflow := new(mocks.WorkflowRun)
	workflow.On("GetID").Return("training-submitted")
	workflow.On("GetRunID").Return("run-submitted")
	c.On("ExecuteWorkflow", mock.Anything, mock.Anything, "moxie.training.Job", mock.Anything).
		Run(func(args mock.Arguments) {
			input := args.Get(3).(tmprl.JobInput[training.Request])
			want := optimization.Training{MaximumEpochs: 7, BatchSize: 3, LearningRate: 0.001, Seed: 19, LoRARank: 4, LoRAAlpha: 12}
			if input.Request.ID != "training-submitted" || input.Request.Model != "model-a" || input.Request.DatasetID != datasetID || input.Request.Training != want || input.Request.Prepare != nil {
				t.Errorf("submitted input: %+v", input.Request)
			}
		}).Return(workflow, nil)
	body := fmt.Sprintf(`{"id":"training-submitted","model":"model-a","dataset_id":%q,"training":{"maximum_epochs":7,"batch_size":3,"learning_rate":0.001,"seed":19,"lora_rank":4,"lora_alpha":12}}`, datasetID)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/training/runs", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("response: %d %s", w.Code, w.Body)
	}
	var got trainapp.Run
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.WorkflowID != "training-submitted" || got.RunID != "run-submitted" || got.Status != "queued" || len(got.Stages) != 1 || got.Stages[0].Name != "training" || len(got.Stages[0].Steps) != 4 {
		t.Fatalf("unexpected submitted state: %+v", got)
	}
	if got.Stages[0].Status != "queued" {
		t.Fatalf("stage status: %s", got.Stages[0].Status)
	}
	for i, name := range []string{"split", "train", "evaluate", "publish"} {
		if step := got.Stages[0].Steps[i]; step.Name != name || step.Status != "queued" {
			t.Errorf("step %d: %+v", i, step)
		}
	}
}

func TestCreatePreparation(t *testing.T) {
	c := new(mocks.Client)
	api, _ := testAPI(t, c)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, uploadRequest(t, map[string][]string{"file": {"source.pdf"}}, "The harbor opens at dawn."))
	if w.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	var file trainapp.Upload
	if err := json.Unmarshal(w.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	workflow := new(mocks.WorkflowRun)
	workflow.On("GetID").Return("preparation-submitted")
	workflow.On("GetRunID").Return("run-submitted")
	c.On("ExecuteWorkflow", mock.Anything, mock.Anything, "moxie.training.Job", mock.Anything).
		Run(func(args mock.Arguments) {
			input := args.Get(3).(tmprl.JobInput[training.Request])
			want := preparation.PreparationRequest{DatasetID: file.ID, Model: "catalog/qwen", Goal: preparation.GoalQuestionAnswer, Instructions: "Answer briefly.", PageStart: 11, PageEnd: 25, MaxQuestions: 50}
			if input.Request.ID != "preparation-submitted" || input.Request.Model != "model-a" || input.Request.DatasetID != "" || !reflect.DeepEqual(input.Request.Prepare, &want) {
				t.Errorf("submitted preparation: %+v", input.Request)
			}
			if input.Job.Stages[0].WaitAfter != "start-training" || input.Job.Stages[0].Steps[1].WaitAfter != "preview-review" {
				t.Errorf("submission lost review or Training authorization: %+v", input.Job)
			}
		}).Return(workflow, nil)
	body := fmt.Sprintf(`{"id":"preparation-submitted","model":"model-a","prepare":{"dataset_id":%q,"model":" catalog/qwen ","goal":"question_answer","instructions":"  Answer briefly.  ","page_start":11,"page_end":25,"max_questions":50}}`, file.ID)
	w = httptest.NewRecorder()
	api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/training/runs", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("response: %d %s", w.Code, w.Body)
	}
	var got trainapp.Run
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.WorkflowID != "preparation-submitted" || got.RunID != "run-submitted" || got.Status != "queued" || len(got.Stages) != 2 {
		t.Fatalf("unexpected submitted state: %+v", got)
	}
	for i, expected := range []struct {
		name  string
		steps []string
	}{
		{"preparation", []string{"extract", "preview", "generate", "verify"}},
		{"training", []string{"split", "train", "evaluate", "publish"}},
	} {
		stage := got.Stages[i]
		if stage.Name != expected.name || stage.Status != "queued" || !stage.StartedAt.IsZero() || len(stage.Steps) != len(expected.steps) {
			t.Fatalf("stage %d: %+v", i, stage)
		}
		for k, name := range expected.steps {
			if step := stage.Steps[k]; step.Name != name || step.Status != "queued" || !step.StartedAt.IsZero() {
				t.Errorf("%s step %d: %+v", stage.Name, k, step)
			}
		}
	}
}

func TestCreateTrainingRejectsInput(t *testing.T) {
	api, _ := testAPI(t, new(mocks.Client))
	datasetID := preparedDataset(t, api)
	for _, body := range []string{
		``, `null`, `[]`, `{`, `{} {}`, `{"model":"model-a","dataset_id":17}`,
		`{"model":"model-a"}`, `{"dataset_id":"some-upload"}`,
		fmt.Sprintf(`{"model":"model-a","dataset_id":%q,"unknown":true}`, datasetID),
		fmt.Sprintf(`{"model":"model-a","dataset_id":%q,"training":{"unknown":true}}`, datasetID),
		fmt.Sprintf(`{"model":"not-approved","dataset_id":%q}`, datasetID),
		fmt.Sprintf(`{"model":"model-a","dataset_id":%q,"training":{"maximum_epochs":-1}}`, datasetID),
		strings.Repeat(" ", 1<<20) + `{}`,
	} {
		w := httptest.NewRecorder()
		api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/training/runs", strings.NewReader(body)))
		if w.Code != http.StatusBadRequest {
			t.Errorf("input %.100q: %d %s", body, w.Code, w.Body)
		}
	}
}

func TestCreatePreparationRejectsInput(t *testing.T) {
	api, store := testAPI(t, new(mocks.Client))
	document, err := store.Put(t.Context(), uploads.Input{Name: "source.md"}, strings.NewReader("The harbor opens at dawn."))
	if err != nil {
		t.Fatal(err)
	}
	unsupported, err := store.Put(t.Context(), uploads.Input{Name: "source.csv"}, strings.NewReader("name,value"))
	if err != nil {
		t.Fatal(err)
	}
	datasetID := preparedDataset(t, api)
	for _, test := range []struct {
		name   string
		body   string
		status int
	}{
		{"both inputs", fmt.Sprintf(`{"model":"model-a","dataset_id":%q,"prepare":{"dataset_id":%q,"model":"catalog/qwen","goal":"question_answer"}}`, datasetID, document.ID), http.StatusBadRequest},
		{"null input", `{"model":"model-a","prepare":null}`, http.StatusBadRequest},
		{"empty input", `{"model":"model-a","prepare":{}}`, http.StatusBadRequest},
		{"wrong input type", `{"model":"model-a","prepare":[]}`, http.StatusBadRequest},
		{"missing preparation model", fmt.Sprintf(`{"model":"model-a","prepare":{"dataset_id":%q,"goal":"question_answer"}}`, document.ID), http.StatusBadRequest},
		{"invalid goal", fmt.Sprintf(`{"model":"model-a","prepare":{"dataset_id":%q,"model":"catalog/qwen","goal":"unsupported"}}`, document.ID), http.StatusBadRequest},
		{"long instructions", fmt.Sprintf(`{"model":"model-a","prepare":{"dataset_id":%q,"model":"catalog/qwen","goal":"question_answer","instructions":%q}}`, document.ID, strings.Repeat("é", 2001)), http.StatusBadRequest},
		{"missing upload ID", `{"model":"model-a","prepare":{"model":"catalog/qwen","goal":"question_answer"}}`, http.StatusBadRequest},
		{"invalid upload ID", `{"model":"model-a","prepare":{"dataset_id":"../source.md","model":"catalog/qwen","goal":"question_answer"}}`, http.StatusBadRequest},
		{"missing upload", fmt.Sprintf(`{"model":"model-a","prepare":{"dataset_id":%q,"model":"catalog/qwen","goal":"question_answer"}}`, "upload-"+strings.Repeat("A", 26)), http.StatusNotFound},
		{"unsupported format", fmt.Sprintf(`{"model":"model-a","prepare":{"dataset_id":%q,"model":"catalog/qwen","goal":"question_answer"}}`, unsupported.ID), http.StatusBadRequest},
		{"prepared JSONL", fmt.Sprintf(`{"model":"model-a","prepare":{"dataset_id":%q,"model":"catalog/qwen","goal":"question_answer"}}`, datasetID), http.StatusBadRequest},
		{"unknown nested field", fmt.Sprintf(`{"model":"model-a","prepare":{"dataset_id":%q,"model":"catalog/qwen","goal":"question_answer","unknown":true}}`, document.ID), http.StatusBadRequest},
		{"premature reviewed records", fmt.Sprintf(`{"model":"model-a","prepare":{"dataset_id":%q,"model":"catalog/qwen","goal":"question_answer","reviewed_records":[]}}`, document.ID), http.StatusBadRequest},
		{"invalid preview count", fmt.Sprintf(`{"model":"model-a","prepare":{"dataset_id":%q,"model":"catalog/qwen","goal":"question_answer","preview_records":6}}`, document.ID), http.StatusBadRequest},
		{"negative question cap", fmt.Sprintf(`{"model":"model-a","prepare":{"dataset_id":%q,"model":"catalog/qwen","goal":"question_answer","max_questions":-1}}`, document.ID), http.StatusBadRequest},
		{"fractional question cap", fmt.Sprintf(`{"model":"model-a","prepare":{"dataset_id":%q,"model":"catalog/qwen","goal":"question_answer","max_questions":2.5}}`, document.ID), http.StatusBadRequest},
		{"unapproved training model", fmt.Sprintf(`{"model":"catalog/qwen","prepare":{"dataset_id":%q,"model":"catalog/qwen","goal":"question_answer"}}`, document.ID), http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/training/runs", strings.NewReader(test.body)))
			if w.Code != test.status {
				t.Fatalf("response: %d %s, want %d", w.Code, w.Body, test.status)
			}
		})
	}
}

func TestCreateTrainingBackendFailure(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{"duplicate", serviceerror.NewWorkflowExecutionAlreadyStarted("private detail", "request-1", "run-1"), http.StatusConflict},
		{"unavailable", serviceerror.NewUnavailable("private detail"), http.StatusServiceUnavailable},
		{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"unexpected", errors.New("private detail"), http.StatusInternalServerError},
	} {
		for _, document := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/document=%t", test.name, document), func(t *testing.T) {
				c := new(mocks.Client)
				api, store := testAPI(t, c)
				datasetID := preparedDataset(t, api)
				body := fmt.Sprintf(`{"model":"model-a","dataset_id":%q}`, datasetID)
				if document {
					file, err := store.Put(t.Context(), uploads.Input{Name: "source.md"}, strings.NewReader("The harbor opens at dawn."))
					if err != nil {
						t.Fatal(err)
					}
					body = fmt.Sprintf(`{"model":"model-a","prepare":{"dataset_id":%q,"model":"catalog/qwen","goal":"question_answer"}}`, file.ID)
				}
				c.On("ExecuteWorkflow", mock.Anything, mock.Anything, "moxie.training.Job", mock.Anything).Return(nil, test.err)
				w := httptest.NewRecorder()
				api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/training/runs", strings.NewReader(body)))
				if w.Code != test.status || strings.Contains(w.Body.String(), "private") {
					t.Fatalf("response: %d %s", w.Code, w.Body)
				}
			})
		}
	}
}

func TestListTrainingRuns(t *testing.T) {
	c := new(mocks.Client)
	api, _ := testAPI(t, c)
	state := tmprl.Run[training.Request, training.Artifacts]{
		Status: tmprl.StatusPaused, Request: training.Request{Model: "/private/model"},
		Stages: []tmprl.StageState{{Name: "training", Status: tmprl.StatusPaused, Steps: []tmprl.StepState{{Name: "train", Status: tmprl.StatusPaused, Attempt: 2}}}},
	}
	payload, err := converter.GetDefaultDataConverter().ToPayload(state)
	if err != nil {
		t.Fatal(err)
	}
	token := []byte{0, 255, 128, 7}
	c.On("ListWorkflow", mock.Anything, mock.MatchedBy(func(r *workflowservice.ListWorkflowExecutionsRequest) bool {
		return r.PageSize == 25 && len(r.NextPageToken) == 0 && r.Query == "WorkflowType = 'moxie.training.Job'"
	})).Return(&workflowservice.ListWorkflowExecutionsResponse{
		Executions: []*workflowpb.WorkflowExecutionInfo{
			{Execution: &commonpb.WorkflowExecution{WorkflowId: "training-example", RunId: "run-2"},
				Type: &commonpb.WorkflowType{Name: "moxie.training.Job"}, Status: enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING,
				Memo: &commonpb.Memo{Fields: map[string]*commonpb.Payload{"moxie.training.job": payload}}},
			{Execution: &commonpb.WorkflowExecution{WorkflowId: "training-example", RunId: "run-1"},
				Type: &commonpb.WorkflowType{Name: "moxie.training.Job"}, Status: enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED},
		}, NextPageToken: token,
	}, nil).Once()
	w := httptest.NewRecorder()
	api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/training/runs", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("response: %d %s", w.Code, w.Body)
	}
	var got trainapp.Runs
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Runs) != 2 || got.Runs[0].RunID != "run-2" || got.Runs[0].Status != "paused" || got.Runs[1].RunID != "run-1" || got.Runs[1].Status != "completed" || got.NextPageToken != "AP-ABw" {
		t.Fatalf("page: %+v", got)
	}
	if len(got.Runs[0].Stages) != 1 || len(got.Runs[0].Stages[0].Steps) != 1 || got.Runs[0].Stages[0].Steps[0].Attempt != 2 || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "request") {
		t.Fatalf("public job state: %s", w.Body)
	}
	c.On("ListWorkflow", mock.Anything, &workflowservice.ListWorkflowExecutionsRequest{
		PageSize: 7, NextPageToken: token, Query: "WorkflowType = 'moxie.training.Job'",
	}).Return(&workflowservice.ListWorkflowExecutionsResponse{}, nil).Once()
	w = httptest.NewRecorder()
	api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/training/runs?page_size=7&next_page_token="+got.NextPageToken, nil))
	var last trainapp.Runs
	if err := json.Unmarshal(w.Body.Bytes(), &last); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || last.Runs == nil || len(last.Runs) != 0 || last.NextPageToken != "" {
		t.Fatalf("empty final page: %d %s", w.Code, w.Body)
	}
	c.AssertExpectations(t)
}

func TestListTrainingRunErrors(t *testing.T) {
	api, _ := testAPI(t, new(mocks.Client))
	for _, query := range []string{"page_size=0", "page_size=101", "page_size=no", "next_page_token=%25", "since=short", "since=" + strings.Repeat("z", 64)} {
		w := httptest.NewRecorder()
		api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/training/runs?"+query, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("query %s: %d %s", query, w.Code, w.Body)
		}
	}
	for _, test := range []struct {
		err    error
		status int
	}{
		{serviceerror.NewUnavailable("private detail"), http.StatusServiceUnavailable},
		{context.DeadlineExceeded, http.StatusGatewayTimeout},
		{errors.New("private detail"), http.StatusInternalServerError},
	} {
		c := new(mocks.Client)
		api, _ := testAPI(t, c)
		c.On("ListWorkflow", mock.Anything, mock.Anything).Return(nil, test.err).Once()
		w := httptest.NewRecorder()
		api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/training/runs", nil))
		if w.Code != test.status || strings.Contains(w.Body.String(), "private") {
			t.Errorf("failure response: %d %s", w.Code, w.Body)
		}
	}
}

func TestTrainingLongPoll(t *testing.T) {
	c := new(mocks.Client)
	api, _ := testAPI(t, c)
	c.On("ListWorkflow", mock.Anything, mock.Anything).Return(&workflowservice.ListWorkflowExecutionsResponse{}, nil).Once()
	w := httptest.NewRecorder()
	api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/training/runs", nil))
	var baseline trainapp.Runs
	if err := json.Unmarshal(w.Body.Bytes(), &baseline); err != nil {
		t.Fatal(err)
	}
	if baseline.Revision == "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing revision or cache policy: %s", w.Body)
	}
	// An opaque pagination-token change alone must not wake the waiting request.
	c.On("ListWorkflow", mock.Anything, mock.Anything).Return(&workflowservice.ListWorkflowExecutionsResponse{NextPageToken: []byte{1}}, nil).Once()
	c.On("ListWorkflow", mock.Anything, mock.Anything).Return(&workflowservice.ListWorkflowExecutionsResponse{
		Executions: []*workflowpb.WorkflowExecutionInfo{{
			Execution: &commonpb.WorkflowExecution{WorkflowId: "new-training", RunId: "run-new"},
			Type:      &commonpb.WorkflowType{Name: "moxie.training.Job"}, Status: enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED,
		}},
	}, nil).Once()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/training/runs?since="+baseline.Revision, nil)
	w = httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); api.ServeHTTP(w, r) }()
	select {
	case <-done:
		t.Fatalf("unchanged page returned instead of waiting: %s", w.Body)
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("job change did not release long poll")
	}
	var changed trainapp.Runs
	if err := json.Unmarshal(w.Body.Bytes(), &changed); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || changed.Revision == baseline.Revision || len(changed.Runs) != 1 || changed.Runs[0].WorkflowID != "new-training" || changed.Runs[0].Status != "completed" {
		t.Fatalf("changed state: %d %s", w.Code, w.Body)
	}
	c.AssertExpectations(t)
}

func TestPreviewLongPoll(t *testing.T) {
	for _, change := range []string{"preview", "checkpoint"} {
		t.Run(change, func(t *testing.T) {
			c := new(mocks.Client)
			api, _ := testAPI(t, c)
			state := tmprl.Run[training.Request, training.Artifacts]{
				Status: tmprl.StatusWaiting, Stage: "preparation", Step: "preview",
				Checkpoint: &tmprl.Checkpoint{ID: "review-original", Name: "preview-review", Stage: "preparation", Step: "preview"},
				Artifacts: training.Artifacts{Preview: &preparation.PreparationResult{Preview: []optimization.Record{
					{Prompt: "When does the harbor open?", Target: "dawn", Evidence: "The harbor opens at dawn.", SourceDocument: "upload-source"},
				}}},
			}
			page := func() *workflowservice.ListWorkflowExecutionsResponse {
				payload, err := converter.GetDefaultDataConverter().ToPayload(state)
				if err != nil {
					t.Fatal(err)
				}
				return &workflowservice.ListWorkflowExecutionsResponse{Executions: []*workflowpb.WorkflowExecutionInfo{{
					Execution: &commonpb.WorkflowExecution{WorkflowId: "training-example", RunId: "run-2"},
					Type:      &commonpb.WorkflowType{Name: "moxie.training.Job"}, Status: enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING,
					Memo: &commonpb.Memo{Fields: map[string]*commonpb.Payload{"moxie.training.job": payload}},
				}}}
			}
			c.On("ListWorkflow", mock.Anything, mock.Anything).Return(page(), nil).Once()
			w := httptest.NewRecorder()
			api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/training/runs", nil))
			var baseline trainapp.Runs
			if err := json.Unmarshal(w.Body.Bytes(), &baseline); err != nil || w.Code != http.StatusOK || baseline.Revision == "" {
				t.Fatalf("baseline: %d %s, %v", w.Code, w.Body, err)
			}
			c.On("ListWorkflow", mock.Anything, mock.Anything).Return(page(), nil).Once()
			if change == "preview" {
				state.Artifacts.Preview.Preview[0].Target = "sunrise"
			} else {
				state.Checkpoint.ID = "review-updated"
			}
			c.On("ListWorkflow", mock.Anything, mock.Anything).Return(page(), nil).Once()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			w = httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				defer close(done)
				api.ServeHTTP(w, httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/training/runs?since="+baseline.Revision, nil))
			}()
			select {
			case <-done:
				t.Fatal("unchanged public state did not wait")
			case <-time.After(100 * time.Millisecond):
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("Preview/checkpoint change did not release long poll")
			}
			var changed trainapp.Runs
			if err := json.Unmarshal(w.Body.Bytes(), &changed); err != nil || w.Code != http.StatusOK || changed.Revision == baseline.Revision || len(changed.Runs) != 1 {
				t.Fatalf("changed state: %d %s, %v", w.Code, w.Body, err)
			}
			got := changed.Runs[0]
			if got.Status != "waiting" || got.Stage != "preparation" || got.Step != "preview" {
				t.Fatalf("unrelated execution state changed: %+v", got)
			}
			if change == "preview" && (len(got.Preview.Records) != 1 || got.Preview.Records[0].Target != "sunrise") || change == "checkpoint" && got.Checkpoint.ID != "review-updated" {
				t.Fatalf("lost %s update: %+v", change, got)
			}
		})
	}
}

func TestTrainingLongPollCancellation(t *testing.T) {
	c := new(mocks.Client)
	api, _ := testAPI(t, c)
	c.On("ListWorkflow", mock.Anything, mock.Anything).Return(&workflowservice.ListWorkflowExecutionsResponse{}, nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/training/runs", nil))
	var baseline trainapp.Runs
	if err := json.Unmarshal(w.Body.Bytes(), &baseline); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/training/runs?since="+baseline.Revision, nil)
	done := make(chan struct{})
	go func() { defer close(done); api.ServeHTTP(httptest.NewRecorder(), r) }()
	select {
	case <-done:
		t.Fatal("long poll did not wait")
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled long poll did not release promptly")
	}
}
