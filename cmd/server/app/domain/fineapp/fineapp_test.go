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
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

type prepareStub struct {
	path      string
	data      []byte
	paths     []string
	documents [][]byte
}

func (stub *prepareStub) Extract(_ context.Context, sourcePath string) (prepare.Artifact, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return prepare.Artifact{}, err
	}
	stub.path = sourcePath
	stub.data = data
	stub.paths = append(stub.paths, sourcePath)
	stub.documents = append(stub.documents, data)
	return prepare.Artifact{ID: "dataset-1", SourcePath: sourcePath}, nil
}

func (stub *prepareStub) ExtractDocuments(ctx context.Context, sourcePaths []string, progress prepare.ExtractionProgressFunc) (prepare.Artifact, error) {
	var artifact prepare.Artifact
	for index, sourcePath := range sourcePaths {
		if progress != nil {
			progress(prepare.ExtractionProgress{Index: index, Stage: "Extracting with Docling"})
		}
		var err error
		artifact, err = stub.Extract(ctx, sourcePath)
		if err != nil {
			return prepare.Artifact{}, err
		}
		if progress != nil {
			progress(prepare.ExtractionProgress{Index: index, Stage: "Extraction complete", Completed: true})
		}
	}
	return artifact, nil
}

type datasetBuilderStub struct {
	request         prepare.FullBuildRequest
	requests        []prepare.FullBuildRequest
	previewRequests []prepare.PreviewRequest
	records         []prepare.Record
	recordsByPath   map[string][]prepare.Record
	previewsByPath  map[string]prepare.PreviewArtifact
}

type blockingDatasetBuilder struct {
	started chan struct{}
}

type preparationModelCatalogStub struct {
	catalog     models.Catalog
	downloaded  map[string]bool
	validated   map[string]bool
	information map[string]models.ModelInfo
}

func (stub *preparationModelCatalogStub) Catalog() (models.Catalog, error) {
	return stub.catalog, nil
}

func (stub *preparationModelCatalogStub) IndexState() (map[string]bool, map[string]bool) {
	return stub.downloaded, stub.validated
}

func (stub *preparationModelCatalogStub) ModelInformation(modelID string) (models.ModelInfo, error) {
	info, exists := stub.information[modelID]
	if !exists {
		return models.ModelInfo{}, fmt.Errorf("model information for %q is unavailable", modelID)
	}
	return info, nil
}

func preparationCatalog(modelIDs ...string) *preparationModelCatalogStub {
	catalog := &preparationModelCatalogStub{
		catalog:     models.Catalog{Models: make(map[string]models.CatalogEntry, len(modelIDs))},
		downloaded:  make(map[string]bool, len(modelIDs)),
		validated:   make(map[string]bool, len(modelIDs)),
		information: make(map[string]models.ModelInfo, len(modelIDs)),
	}
	for _, modelID := range modelIDs {
		metadata := map[string]string{
			"general.architecture":    "qwen3",
			"tokenizer.chat_template": "template",
		}
		catalog.catalog.Models[modelID] = models.CatalogEntry{
			Capabilities: models.CapabilitiesFor(metadata, false),
		}
		catalog.downloaded[modelID] = true
		catalog.validated[modelID] = true
		catalog.information[modelID] = models.ModelInfo{ID: modelID, Metadata: metadata}
	}
	return catalog
}

func writeDatasetCollection(t *testing.T, datasetsPath, id string, names ...string) string {
	t.Helper()
	datasetDirectory := filepath.Join(datasetsPath, id)
	if err := os.MkdirAll(filepath.Join(datasetDirectory, "documents"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(datasetDirectory, "extracted.md"), []byte("combined source"), 0o600); err != nil {
		t.Fatal(err)
	}
	sources := make([]map[string]string, 0, len(names))
	for index, name := range names {
		documentID := fmt.Sprintf("%03d", index+1)
		markdownFile := filepath.Join("documents", documentID, "extracted.md")
		documentDirectory := filepath.Join(datasetDirectory, "documents", documentID)
		if err := os.MkdirAll(documentDirectory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(documentDirectory, "extracted.md"), []byte("source for "+name), 0o600); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, map[string]string{"name": name, "markdown_file": markdownFile})
	}
	manifest, err := json.Marshal(map[string]any{
		"version":          3,
		"id":               id,
		"documents_folder": "documents",
		"sources":          sources,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(datasetDirectory, "manifest.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	return datasetDirectory
}

func knowledgeRecords(prefix string) []prepare.Record {
	records := make([]prepare.Record, 0, finetune.MinimumKnowledgeFacts*5)
	for fact := range finetune.MinimumKnowledgeFacts {
		for variant := range 5 {
			records = append(records, prepare.Record{
				Prompt:   fmt.Sprintf("%s question variant %d for fact %d?", prefix, variant, fact),
				Target:   fmt.Sprintf("%s detailed answer for fact %d with enough words to require generation.", prefix, fact),
				Evidence: fmt.Sprintf("%s source evidence for fact %d supports the detailed answer.", prefix, fact),
				FactID:   fmt.Sprintf("%s-fact-%d", prefix, fact),
			})
		}
	}
	return records
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

func (stub *datasetBuilderStub) Build(_ context.Context, request prepare.PreviewRequest) (prepare.PreviewArtifact, error) {
	stub.previewRequests = append(stub.previewRequests, request)
	return stub.previewsByPath[request.DatasetDirectory], nil
}

func (stub *datasetBuilderStub) BuildFull(_ context.Context, request prepare.FullBuildRequest) (prepare.FullBuildArtifact, error) {
	stub.request = request
	stub.requests = append(stub.requests, request)
	records := stub.records
	if selected, exists := stub.recordsByPath[request.DatasetDirectory]; exists {
		records = selected
	}
	path := filepath.Join(request.DatasetDirectory, "dataset.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return prepare.FullBuildArtifact{}, err
	}
	for _, record := range records {
		if err := json.NewEncoder(file).Encode(record); err != nil {
			_ = file.Close()
			return prepare.FullBuildArtifact{}, err
		}
	}
	if err := file.Close(); err != nil {
		return prepare.FullBuildArtifact{}, err
	}
	return prepare.FullBuildArtifact{Records: records, RecordsPath: path}, nil
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

func TestUploadDatasetPreparesMultipleDocumentsAsOneDataset(t *testing.T) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, document := range []struct {
		name string
		data string
	}{
		{name: "guide.pdf", data: "guide document"},
		{name: "manual.docx", data: "manual document"},
	} {
		file, err := form.CreateFormFile("files", document.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(document.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/datasets", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	preparer := &prepareStub{}
	api := newApp(Config{Preparer: preparer})
	response := api.uploadDataset(t.Context(), request)
	data, _, err := response.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var got DatasetResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.DatasetID != "dataset-1" || got.SourceName != "guide.pdf" || !slices.Equal(got.SourceNames, []string{"guide.pdf", "manual.docx"}) {
		t.Fatalf("response: %+v", got)
	}
	if len(got.Documents) != 2 || got.Documents[0] != (DatasetDocumentResponse{ID: "001", Name: "guide.pdf"}) || got.Documents[1] != (DatasetDocumentResponse{ID: "002", Name: "manual.docx"}) {
		t.Fatalf("response documents: %+v", got.Documents)
	}
	if len(preparer.documents) != 2 || string(preparer.documents[0]) != "guide document" || string(preparer.documents[1]) != "manual document" {
		t.Fatalf("uploaded documents: %q", preparer.documents)
	}
	for _, path := range preparer.paths {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("temporary upload still exists: %v", err)
		}
	}
	progress, exists := api.preparations.currentStatus()
	if !exists || progress.Status != preparationCompleted || progress.Completed != 2 || progress.Total != 2 || len(progress.Documents) != 2 {
		t.Fatalf("preparation progress: %+v", progress)
	}
	for _, document := range progress.Documents {
		if document.Status != documentCompleted || document.Stage != "Extraction complete" {
			t.Fatalf("document progress: %+v", document)
		}
	}
}

func TestPreviewDatasetBuildsEachDocumentIndependently(t *testing.T) {
	datasetsPath := t.TempDir()
	datasetDirectory := writeDatasetCollection(t, datasetsPath, "dataset-1", "guide.pdf", "manual.docx")
	guideDirectory := filepath.Join(datasetDirectory, "documents", "001")
	manualDirectory := filepath.Join(datasetDirectory, "documents", "002")
	builder := &datasetBuilderStub{previewsByPath: map[string]prepare.PreviewArtifact{
		guideDirectory: {
			Records:      []prepare.Record{{Prompt: "Guide question?", Target: "Guide answer", Evidence: "Guide evidence"}},
			Verification: []prepare.VerificationResult{{RecordNumber: 1, Passed: true}},
		},
		manualDirectory: {
			Records:      []prepare.Record{{Prompt: "Manual question?", Target: "Manual answer", Evidence: "Manual evidence"}},
			Verification: []prepare.VerificationResult{{RecordNumber: 1, Passed: true}},
		},
	}}
	api := newApp(Config{
		Builder: builder, PreparationModels: preparationCatalog("org/model/AGENT"), DatasetsPath: datasetsPath,
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/prepare/preview", bytes.NewBufferString(`{
		"dataset_id":"dataset-1","goal":"question_answer","model_id":"org/model/AGENT","record_count":5
	}`))
	response := api.previewDataset(t.Context(), request)
	if _, ok := response.(error); ok {
		t.Fatalf("preview response: %v", response)
	}
	data, _, err := response.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var got PreviewResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Documents) != 2 || got.Documents[0].Name != "guide.pdf" || got.Documents[0].Records[0].Prompt != "Guide question?" || got.Documents[1].Name != "manual.docx" || got.Documents[1].Records[0].Prompt != "Manual question?" {
		t.Fatalf("document previews: %+v", got.Documents)
	}
	if len(builder.previewRequests) != 2 || builder.previewRequests[0].DatasetDirectory != guideDirectory || builder.previewRequests[1].DatasetDirectory != manualDirectory {
		t.Fatalf("preview requests: %+v", builder.previewRequests)
	}
	progress, exists := api.preparations.currentStatus()
	if !exists || progress.Status != preparationCompleted || progress.Completed != 2 || len(progress.Documents) != 2 || progress.Documents[0].Status != documentCompleted || progress.Documents[1].Status != documentCompleted {
		t.Fatalf("preview progress: %+v", progress)
	}

	builder.previewRequests = nil
	regenerate := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/prepare/preview", bytes.NewBufferString(`{
		"dataset_id":"dataset-1","goal":"question_answer","model_id":"org/model/AGENT","record_count":5,"document_ids":["002"]
	}`))
	regenerated := api.previewDataset(t.Context(), regenerate)
	if _, ok := regenerated.(error); ok {
		t.Fatalf("regenerate response: %v", regenerated)
	}
	if len(builder.previewRequests) != 1 || builder.previewRequests[0].DatasetDirectory != manualDirectory {
		t.Fatalf("regenerate requests: %+v", builder.previewRequests)
	}
}

func TestBuildDatasetBuildsDocumentsSequentiallyAndMergesOneContract(t *testing.T) {
	datasetsPath := t.TempDir()
	datasetDirectory := writeDatasetCollection(t, datasetsPath, "dataset-1", "guide.pdf", "manual.docx")
	guideDirectory := filepath.Join(datasetDirectory, "documents", "001")
	manualDirectory := filepath.Join(datasetDirectory, "documents", "002")
	builder := &datasetBuilderStub{recordsByPath: map[string][]prepare.Record{
		guideDirectory:  knowledgeRecords("guide"),
		manualDirectory: knowledgeRecords("manual"),
	}}
	api := newApp(Config{
		Builder: builder, PreparationModels: preparationCatalog("org/model/AGENT"), DatasetsPath: datasetsPath,
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/prepare/build", bytes.NewBufferString(`{
		"dataset_id":"dataset-1","goal":"question_answer","model_id":"org/model/AGENT",
		"documents":[
			{"id":"001","reviewed_records":[{"prompt":"Guide seed?","target":"Guide seed answer","evidence":"Guide seed evidence"}],"verified_preview_records":[{"prompt":"Guide seed?","target":"Guide seed answer","evidence":"Guide seed evidence"}]},
			{"id":"002","reviewed_records":[{"prompt":"Manual seed?","target":"Manual seed answer","evidence":"Manual seed evidence"}],"verified_preview_records":[{"prompt":"Manual seed?","target":"Manual seed answer","evidence":"Manual seed evidence"}]}
		]
	}`))
	response := api.buildDataset(t.Context(), request)
	if _, ok := response.(error); ok {
		t.Fatalf("build response: %v", response)
	}
	if len(builder.requests) != 2 || builder.requests[0].DatasetDirectory != guideDirectory || builder.requests[1].DatasetDirectory != manualDirectory {
		t.Fatalf("build requests: %+v", builder.requests)
	}
	for _, buildRequest := range builder.requests {
		if buildRequest.MinimumQuestionAnswerFacts != finetune.MinimumKnowledgeFacts {
			t.Fatalf("minimum facts = %d, want %d", buildRequest.MinimumQuestionAnswerFacts, finetune.MinimumKnowledgeFacts)
		}
	}
	data, _, err := response.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var got BuildResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.RecordCount != 300 || len(got.Documents) != 2 || got.Documents[0].FactCount != 30 || got.Documents[1].FactCount != 30 {
		t.Fatalf("build response: %+v", got)
	}
	contract, err := finetune.ValidateDatasetContract(filepath.Join(datasetDirectory, "dataset.jsonl"))
	if err != nil {
		t.Fatalf("validate merged contract: %v", err)
	}
	if contract.Report.SourceRecords != 300 {
		t.Fatalf("merged source records = %d, want 300", contract.Report.SourceRecords)
	}
	if contract.Train[0].SourceDocument == "" {
		t.Fatalf("merged contract lost source provenance: %+v", contract.Train[0])
	}
	progress, exists := api.preparations.currentStatus()
	if !exists || progress.Status != preparationCompleted || progress.Completed != 2 || len(progress.Documents) != 2 || progress.Documents[0].AcceptedFacts != 30 || progress.Documents[1].AcceptedFacts != 30 {
		t.Fatalf("build progress: %+v", progress)
	}
}

func TestUploadDatasetRejectsPreparedDatasetMixedWithDocuments(t *testing.T) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, name := range []string{"prepared.jsonl", "guide.pdf"} {
		file, err := form.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte("content")); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/datasets", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response := newApp(Config{Preparer: &prepareStub{}}).uploadDataset(t.Context(), request)
	var appErr *errs.Error
	if !errors.As(response.(error), &appErr) || appErr.Code != errs.InvalidArgument || !strings.Contains(appErr.Message, "cannot be combined") {
		t.Fatalf("error: got %T %v", response, response)
	}
}

func TestStopDocumentPreparationPreservesCompletedProgress(t *testing.T) {
	operations := newPreparationOperations()
	ctx, operationID, err := operations.start(t.Context(), "extraction", "Preparing source documents", 2)
	if err != nil {
		t.Fatal(err)
	}
	operations.initializeDocuments(operationID, []string{"guide.pdf", "manual.docx"})
	operations.updateDocument(operationID, prepare.ExtractionProgress{Index: 0, Stage: "Extraction complete", Completed: true})
	operations.updateDocument(operationID, prepare.ExtractionProgress{Index: 1, Stage: "Extracting with Docling"})
	if err := operations.stop(); err != nil {
		t.Fatal(err)
	}
	<-ctx.Done()
	operations.finish(operationID, ctx.Err())

	progress, exists := operations.currentStatus()
	if !exists || progress.Status != preparationStopped || len(progress.Documents) != 2 {
		t.Fatalf("stopped progress: %+v", progress)
	}
	if progress.Documents[0].Status != documentCompleted || progress.Documents[1].Status != documentStopped {
		t.Fatalf("document statuses: %+v", progress.Documents)
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

type trainingRecipeProviderStub struct {
	modelPath string
	recipe    finetune.Recipe
}

func (stub *trainingRecipeProviderStub) Recipe(modelPath string) (finetune.Recipe, error) {
	stub.modelPath = modelPath
	return stub.recipe, nil
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

func TestTrainingRecipeReportsBackendSettings(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "gemma.gguf")
	provider := &trainingRecipeProviderStub{recipe: finetune.Recipe{
		Method: "lora", BaseWeights: "frozen", Optimizer: "adam",
		LearningRate: 0.0002, WeightDecay: 0.01, BatchSize: 2, MaximumEpochs: 3, Seed: 3407,
		LoRA: finetune.LoRARecipe{
			Rank: 8, Alpha: 8, Scale: 1,
			TargetModules: []string{"query", "key", "value", "output", "gate", "up", "down"},
		},
		Sequence: finetune.SequenceRecipe{
			MaximumTokens: 32768, Source: "model_gguf", Overflow: "reject", Padding: "power_of_two",
		},
		Loss: "answer_only_cross_entropy", CheckpointSelection: "lowest_validation_loss",
		Output: finetune.OutputRecipe{Format: "gguf", Precision: "f32", Filename: "adapter-f32.gguf"},
	}}
	api := newApp(Config{
		Models:  modelResolverStub{path: backend.ModelPath{ModelFiles: []string{modelPath}}},
		Recipes: provider,
	})

	request := httptest.NewRequest(http.MethodGet, "/v1/fine-tuning/recipe?model_id=ggml-org/gemma-3-1b-it-Q8_0", nil)
	response := api.trainingRecipe(t.Context(), request)
	data, _, err := response.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var got RecipeResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if provider.modelPath != modelPath || got.ModelID != "ggml-org/gemma-3-1b-it-Q8_0" || got.Method != "lora" || got.BaseWeights != "frozen" || got.Optimizer != "adam" || got.LearningRate != 0.0002 || got.WeightDecay != 0.01 || got.BatchSize != 2 || got.MaximumEpochs != 3 || got.Seed != 3407 || got.LoRA.Rank != 8 || got.LoRA.Alpha != 8 || got.LoRA.Scale != 1 || got.Sequence.MaximumTokens != 32768 || got.Sequence.Source != "model_gguf" || got.Sequence.Overflow != "reject" || got.Sequence.Padding != "power_of_two" || got.Loss != "answer_only_cross_entropy" || got.CheckpointSelection != "lowest_validation_loss" || got.Output.Format != "gguf" || got.Output.Precision != "f32" || got.Output.Filename != "adapter-f32.gguf" {
		t.Fatalf("recipe response = %+v", got)
	}
	wantModules := []string{"query", "key", "value", "output", "gate", "up", "down"}
	if !slices.Equal(got.LoRA.TargetModules, wantModules) {
		t.Fatalf("target modules: got %v, want %v", got.LoRA.TargetModules, wantModules)
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
	catalog := preparationCatalog("org/zeta/AGENT", "org/alpha/AGENT", "org/not-downloaded", "org/not-validated", "org/text-embed", "org/reranker", "org/vision")
	catalog.downloaded["org/not-downloaded"] = false
	catalog.validated["org/not-validated"] = false
	catalog.information["org/text-embed"] = models.ModelInfo{
		ID:           "org/text-embed",
		IsEmbedModel: true,
		Metadata:     catalog.information["org/text-embed"].Metadata,
	}
	catalog.information["org/reranker"] = models.ModelInfo{
		ID:            "org/reranker",
		IsRerankModel: true,
		Metadata:      catalog.information["org/reranker"].Metadata,
	}
	catalog.information["org/vision"] = models.ModelInfo{
		ID:            "org/vision",
		HasProjection: true,
		Metadata:      catalog.information["org/vision"].Metadata,
	}
	api := newApp(Config{PreparationModels: catalog})

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
	api := newApp(Config{PreparationModels: preparationCatalog("org/model/AGENT")})
	if err := api.validatePreparationModel("org/model/AGENT"); err != nil {
		t.Fatalf("available model: %v", err)
	}
	if err := api.validatePreparationModel("org/other"); err == nil {
		t.Fatal("unavailable model: got nil error")
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

func TestPreparationRequestsRejectOversizedDatasetInstructions(t *testing.T) {
	instructions := strings.Repeat("界", prepare.MaxDatasetInstructionsRunes+1)
	preview := PreviewRequest{
		DatasetID:           "dataset-1",
		Goal:                "question_answer",
		ModelID:             "org/model/AGENT",
		RecordCount:         1,
		DatasetInstructions: instructions,
	}
	if err := preview.Validate(); err == nil {
		t.Fatal("oversized preview instructions: got nil error")
	}

	build := BuildRequest{
		DatasetID:           "dataset-1",
		Goal:                "question_answer",
		ModelID:             "org/model/AGENT",
		DatasetInstructions: instructions,
		ReviewedRecords:     []Record{{Prompt: "Question?", Target: "Answer", Evidence: "Answer"}},
	}
	if err := build.Validate(); err == nil {
		t.Fatal("oversized build instructions: got nil error")
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
		Builder:           builder,
		PreparationModels: preparationCatalog("org/model/AGENT"),
		DatasetsPath:      datasetsPath,
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/fine-tuning/prepare/build", bytes.NewBufferString(`{
		"dataset_id":"dataset-1",
		"goal":"question_answer",
		"model_id":"org/model/AGENT",
		"dataset_instructions":"  Focus on concurrency concepts.  ",
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
	if builder.request.DatasetInstructions != "Focus on concurrency concepts." {
		t.Fatalf("dataset instructions: got %q", builder.request.DatasetInstructions)
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
		Builder:           builder,
		PreparationModels: preparationCatalog("org/model/AGENT"),
		DatasetsPath:      datasetsPath,
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
		Builder:           builder,
		PreparationModels: preparationCatalog("org/model/AGENT"),
		DatasetsPath:      datasetsPath,
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
	if !exists || progress.Status != preparationRunning || progress.Completed != 0 || progress.Total != 1 || progress.Stage != "Verifying generated records" {
		t.Fatalf("active preview progress: got %+v, exists %t", progress, exists)
	}
	if len(progress.Documents) != 1 || progress.Documents[0].Status != documentProcessing || progress.Documents[0].CompletedBatches != 1 || progress.Documents[0].TotalBatches != 2 {
		t.Fatalf("active document preview progress: %+v", progress.Documents)
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
