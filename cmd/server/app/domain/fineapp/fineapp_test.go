package fineapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
)

type prepareStub struct {
	path string
	data []byte
}

func (stub *prepareStub) Extract(_ context.Context, sourcePath string) (prepare.Artifact, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return prepare.Artifact{}, err
	}
	stub.path = sourcePath
	stub.data = data
	return prepare.Artifact{ID: "dataset-1", SourcePath: sourcePath}, nil
}

type datasetBuilderStub struct {
	request prepare.FullBuildRequest
	records []prepare.Record
}

type blockingDatasetBuilder struct {
	started chan struct{}
}

func (stub *blockingDatasetBuilder) Build(ctx context.Context, request prepare.PreviewRequest) (prepare.PreviewArtifact, error) {
	request.Progress(prepare.PreviewProgress{CompletedStages: 1, TotalStages: 2, Stage: "Verifying generated records"})
	close(stub.started)
	<-ctx.Done()
	return prepare.PreviewArtifact{}, fmt.Errorf("preview stopped: %w", ctx.Err())
}

func (*blockingDatasetBuilder) BuildFull(context.Context, prepare.FullBuildRequest) (prepare.FullBuildArtifact, error) {
	return prepare.FullBuildArtifact{}, nil
}

func (*datasetBuilderStub) Build(context.Context, prepare.PreviewRequest) (prepare.PreviewArtifact, error) {
	return prepare.PreviewArtifact{}, nil
}

func (stub *datasetBuilderStub) BuildFull(_ context.Context, request prepare.FullBuildRequest) (prepare.FullBuildArtifact, error) {
	stub.request = request
	path := filepath.Join(request.DatasetDirectory, "dataset.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return prepare.FullBuildArtifact{}, err
	}
	for _, record := range stub.records {
		if err := json.NewEncoder(file).Encode(record); err != nil {
			_ = file.Close()
			return prepare.FullBuildArtifact{}, err
		}
	}
	if err := file.Close(); err != nil {
		return prepare.FullBuildArtifact{}, err
	}
	return prepare.FullBuildArtifact{Records: stub.records, RecordsPath: path}, nil
}

func TestUploadDataset(t *testing.T) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", `C:\documents\guide.PDF`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("document")); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/datasets", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	preparer := &prepareStub{}
	response := newApp(Config{Preparer: preparer}).uploadDataset(context.Background(), request)

	data, _, err := response.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var got DatasetResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.DatasetID != "dataset-1" || got.SourceName != "guide.PDF" {
		t.Fatalf("response: got %+v", got)
	}
	if string(preparer.data) != "document" {
		t.Fatalf("uploaded data: got %q", preparer.data)
	}
	if _, err := os.Stat(preparer.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary upload still exists: %v", err)
	}
}

func TestUploadDatasetRejectsUnsupportedFile(t *testing.T) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if _, err := form.CreateFormFile("file", "notes.txt"); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/datasets", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response := newApp(Config{Preparer: &prepareStub{}}).uploadDataset(context.Background(), request)

	var appErr *errs.Error
	if !errors.As(response.(error), &appErr) || appErr.Code != errs.InvalidArgument {
		t.Fatalf("error: got %T %v", response, response)
	}
}

func TestUploadDatasetImportsPreparedJSONLWithoutDocumentPreparer(t *testing.T) {
	var dataset bytes.Buffer
	for index := range 50 {
		record := finetune.DatasetRecord{
			Prompt:   "Classify record " + strconv.Itoa(index),
			Target:   []string{"allowed", "blocked"}[index%2],
			Evidence: "Reviewed classification source " + strconv.Itoa(index),
		}
		if err := json.NewEncoder(&dataset).Encode(record); err != nil {
			t.Fatal(err)
		}
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", "reviewed.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(dataset.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	datasetsPath := filepath.Join(t.TempDir(), "datasets")
	request := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/datasets", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response := newApp(Config{DatasetsPath: datasetsPath}).uploadDataset(t.Context(), request)

	data, _, err := response.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var got DatasetResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.DatasetID == "" || got.SourceName != "reviewed.jsonl" {
		t.Fatalf("response: got %+v", got)
	}
	stored, err := os.ReadFile(filepath.Join(datasetsPath, got.DatasetID, "dataset.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, dataset.Bytes()) {
		t.Fatal("managed dataset differs from uploaded JSONL")
	}
}

func TestUploadDatasetRejectsUnusablePreparedJSONL(t *testing.T) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", "too-small.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(`{"prompt":"Classify this","target":"allowed","evidence":"Reviewed source"}`)); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	datasetsPath := filepath.Join(t.TempDir(), "datasets")
	request := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/datasets", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response := newApp(Config{DatasetsPath: datasetsPath}).uploadDataset(t.Context(), request)

	var appErr *errs.Error
	if !errors.As(response.(error), &appErr) || appErr.Code != errs.InvalidArgument {
		t.Fatalf("error: got %T %v", response, response)
	}
	if entries, err := os.ReadDir(datasetsPath); err == nil && len(entries) != 0 {
		t.Fatalf("invalid dataset published %d entries", len(entries))
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

type modelResolverStub struct {
	path backend.ModelPath
}

func (stub modelResolverStub) FullPath(string) (backend.ModelPath, error) {
	return stub.path, nil
}

type fineTunerStub struct {
	request       finetune.Request
	contextCanEnd bool
}

func (stub *fineTunerStub) Start(ctx context.Context, request finetune.Request, _ finetune.UpdateFunc) (finetune.RunInfo, error) {
	stub.request = request
	stub.contextCanEnd = ctx.Done() != nil
	return finetune.RunInfo{
		ID:      "run-1",
		Dataset: request.Dataset,
		Model:   request.Model,
		Status:  finetune.StatusPreparing,
	}, nil
}

func (*fineTunerStub) Resume(context.Context, string, finetune.UpdateFunc) (finetune.RunInfo, error) {
	return finetune.RunInfo{}, nil
}

func (*fineTunerStub) Stop() error {
	return nil
}

func (*fineTunerStub) Current() (finetune.RunInfo, bool) {
	return finetune.RunInfo{}, false
}

func (*fineTunerStub) History() ([]finetune.RunInfo, error) {
	return nil, nil
}

func TestStartRunResolvesManagedResources(t *testing.T) {
	datasetsPath := t.TempDir()
	datasetDirectory := filepath.Join(datasetsPath, "dataset-1")
	if err := os.Mkdir(datasetDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	datasetPath := filepath.Join(datasetDirectory, "dataset.jsonl")
	if err := os.WriteFile(datasetPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(t.TempDir(), "gemma.gguf")
	finetuner := &fineTunerStub{}
	api := &app{
		finetuner:    finetuner,
		models:       modelResolverStub{path: backend.ModelPath{ModelFiles: []string{modelPath}}},
		datasetsPath: datasetsPath,
		runtime:      finetune.Runtime{Port: 8181},
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/runs", bytes.NewBufferString(`{"dataset_id":"dataset-1","model_id":"google/gemma"}`))
	requestContext, cancel := context.WithCancel(context.Background())
	request = request.WithContext(requestContext)
	response := api.startRun(request.Context(), request)
	cancel()

	if finetuner.request.Dataset != datasetPath || finetuner.request.Model != modelPath || finetuner.request.Runtime.Port != 8181 {
		t.Fatalf("fine-tuning request: got %+v", finetuner.request)
	}
	if finetuner.contextCanEnd {
		t.Fatal("training context is tied to the HTTP request")
	}
	data, _, err := response.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(datasetsPath)) || bytes.Contains(data, []byte(filepath.Dir(modelPath))) {
		t.Fatalf("response exposed a server path: %s", data)
	}
}

func TestValidateResourceID(t *testing.T) {
	for _, id := range []string{"", ".", "..", "../dataset", `..\dataset`, " dataset"} {
		if err := validateResourceID(id); err == nil {
			t.Errorf("validateResourceID(%q): expected an error", id)
		}
	}
	if err := validateResourceID("20260911T143522Z-a1b2c3d4"); err != nil {
		t.Fatalf("valid id: %v", err)
	}
}

func TestPreparationModels(t *testing.T) {
	configured := []string{"org/zeta/AGENT", "org/alpha/AGENT"}
	api := newApp(Config{ConfiguredModelIDs: configured})
	configured[0] = "changed-after-construction"

	response := api.preparationModels(t.Context(), httptest.NewRequest(http.MethodGet, "/v1/fine-tuning/prepare/models", nil))
	data, _, err := response.Encode()
	if err != nil {
		t.Fatal(err)
	}

	var got PreparationModelsResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := []string{"org/alpha/AGENT", "org/zeta/AGENT"}
	if !slices.Equal(got.Models, want) {
		t.Fatalf("models: got %v, want %v", got.Models, want)
	}
}

func TestValidatePreparationModel(t *testing.T) {
	api := newApp(Config{ConfiguredModelIDs: []string{"org/model/AGENT"}})
	if err := api.validatePreparationModel("org/model/AGENT"); err != nil {
		t.Fatalf("configured model: %v", err)
	}
	if err := api.validatePreparationModel("org/other"); err == nil {
		t.Fatal("unconfigured model: got nil error")
	}
}

func TestPreviewRequestRejectsMoreThanFiveRecords(t *testing.T) {
	req := PreviewRequest{
		DatasetID:   "dataset-1",
		Goal:        "question_answer",
		ModelID:     "org/model/AGENT",
		RecordCount: prepare.MaxPreviewRecords,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("five preview records: %v", err)
	}

	req.RecordCount++
	if err := req.Validate(); err == nil {
		t.Fatal("six preview records: got nil error")
	}
}

func TestBuildDatasetEnforcesFactTargetAndValidatesTrainingContract(t *testing.T) {
	datasetsPath := t.TempDir()
	datasetDirectory := filepath.Join(datasetsPath, "dataset-1")
	if err := os.Mkdir(datasetDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}

	records := make([]prepare.Record, 0, finetune.MinimumKnowledgeFacts*5)
	for fact := range finetune.MinimumKnowledgeFacts {
		for variant := range 5 {
			records = append(records, prepare.Record{
				Prompt:   "Question variant " + strconv.Itoa(variant) + " for fact " + strconv.Itoa(fact) + "?",
				Target:   "Detailed answer for fact " + strconv.Itoa(fact) + " with enough words to require generation.",
				Evidence: "Source evidence for fact " + strconv.Itoa(fact) + " supports the detailed answer.",
				FactID:   "fact-" + strconv.Itoa(fact),
			})
		}
	}
	builder := &datasetBuilderStub{records: records}
	api := newApp(Config{
		Builder:            builder,
		ConfiguredModelIDs: []string{"org/model/AGENT"},
		DatasetsPath:       datasetsPath,
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/prepare/build", bytes.NewBufferString(`{
		"dataset_id":"dataset-1",
		"goal":"question_answer",
		"model_id":"org/model/AGENT",
		"reviewed_records":[{"prompt":"What is supported?","target":"This answer is supported by the source.","evidence":"The source supports this answer."}],
		"verified_preview_records":[{"prompt":"What is supported?","target":"This answer is supported by the source.","evidence":"The source supports this answer."}]
	}`))
	response := api.buildDataset(t.Context(), request)
	if _, ok := response.(error); ok {
		t.Fatalf("build response: %v", response)
	}
	if builder.request.MinimumQuestionAnswerFacts != finetune.MinimumKnowledgeFacts {
		t.Fatalf("minimum facts: got %d, want %d", builder.request.MinimumQuestionAnswerFacts, finetune.MinimumKnowledgeFacts)
	}
	for _, name := range []string{"contract.json", "train.jsonl", "validation.jsonl", "test.jsonl"} {
		if _, err := os.Stat(filepath.Join(datasetDirectory, "training", name)); err != nil {
			t.Fatalf("validated contract %s: %v", name, err)
		}
	}
}

func TestBuildDatasetDoesNotReportAnInvalidDatasetReady(t *testing.T) {
	datasetsPath := t.TempDir()
	datasetDirectory := filepath.Join(datasetsPath, "dataset-1")
	if err := os.Mkdir(datasetDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	builder := &datasetBuilderStub{records: []prepare.Record{{
		Prompt: "Only question?", Target: "Only answer", Evidence: "Only evidence",
	}}}
	api := newApp(Config{
		Builder:            builder,
		ConfiguredModelIDs: []string{"org/model/AGENT"},
		DatasetsPath:       datasetsPath,
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/prepare/build", bytes.NewBufferString(`{
		"dataset_id":"dataset-1",
		"goal":"question_answer",
		"model_id":"org/model/AGENT",
		"reviewed_records":[{"prompt":"Only question?","target":"Only answer","evidence":"Only evidence"}],
		"verified_preview_records":[{"prompt":"Only question?","target":"Only answer","evidence":"Only evidence"}]
	}`))
	response := api.buildDataset(t.Context(), request)
	appErr, ok := response.(*errs.Error)
	if !ok || appErr.Code != errs.FailedPrecondition {
		t.Fatalf("error: got %T %v", response, response)
	}
	if _, err := os.Stat(filepath.Join(datasetDirectory, "training")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid training contract was published: %v", err)
	}
}

func TestStopPreparationCancelsActivePreview(t *testing.T) {
	datasetsPath := t.TempDir()
	datasetDirectory := filepath.Join(datasetsPath, "dataset-1")
	if err := os.Mkdir(datasetDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	builder := &blockingDatasetBuilder{started: make(chan struct{})}
	api := newApp(Config{
		Builder:            builder,
		ConfiguredModelIDs: []string{"org/model/AGENT"},
		DatasetsPath:       datasetsPath,
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/prepare/preview", bytes.NewBufferString(`{
		"dataset_id":"dataset-1",
		"goal":"question_answer",
		"model_id":"org/model/AGENT",
		"record_count":5
	}`))
	response := make(chan web.Encoder, 1)
	go func() {
		response <- api.previewDataset(request.Context(), request)
	}()

	select {
	case <-builder.started:
	case <-time.After(time.Second):
		t.Fatal("preview did not start")
	}
	progress, exists := api.preparations.currentStatus()
	if !exists || progress.Status != preparationRunning || progress.Completed != 1 || progress.Total != 2 || progress.Stage != "Verifying generated records" {
		t.Fatalf("active preview progress: got %+v, exists %t", progress, exists)
	}
	stopResponse := api.stopPreparation(t.Context(), httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/prepare/current/stop", nil))
	if _, ok := stopResponse.(error); ok {
		t.Fatalf("stop response: %v", stopResponse)
	}

	select {
	case result := <-response:
		appErr, ok := result.(*errs.Error)
		if !ok || appErr.Code != errs.Canceled {
			t.Fatalf("preview response: got %T %v", result, result)
		}
	case <-time.After(time.Second):
		t.Fatal("preview did not stop")
	}
	progress, _ = api.preparations.currentStatus()
	if progress.Status != preparationStopped {
		t.Fatalf("stopped preview progress: got %+v", progress)
	}
}
