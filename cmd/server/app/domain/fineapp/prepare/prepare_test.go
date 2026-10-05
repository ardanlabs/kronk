package prepare_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
)

const extractionProfile = "docling-v1.12.0-dlparse_v4-accurate-v1"

type extractor struct {
	document prepare.Document
}

func (e extractor) Convert(context.Context, string) (prepare.Document, error) {
	return e.document, nil
}

func (extractor) ExtractionProfile() string {
	return extractionProfile
}

type countingExtractor struct {
	document prepare.Document
	calls    int
}

func (e *countingExtractor) Convert(context.Context, string) (prepare.Document, error) {
	e.calls++
	return e.document, nil
}

func (*countingExtractor) ExtractionProfile() string {
	return extractionProfile
}

func TestExtractStoresMoxieDataset(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".moxie")
	manager, err := prepare.NewAt(extractor{document: prepare.Document{
		Markdown:       "# Source\n\nPrepared content.",
		StructuredJSON: json.RawMessage(`{"texts":[{"text":"Prepared content."}]}`),
		ProcessingTime: 2.5,
	}}, root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	sourcePath := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(sourcePath, []byte("original"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	artifact, err := manager.Extract(context.Background(), sourcePath)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	if got, want := filepath.Dir(filepath.Dir(artifact.Directory)), root; got != want {
		t.Fatalf("artifact root: got %q want %q", got, want)
	}
	assertFileContents(t, artifact.SourcePath, "original")
	assertFileContents(t, artifact.MarkdownPath, "# Source\n\nPrepared content.")
	assertFileContents(t, artifact.StructuredPath, "{\"texts\":[{\"text\":\"Prepared content.\"}]}\n")

	manifestData, err := os.ReadFile(artifact.ManifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if got, want := manifest["extractor"], "docling"; got != want {
		t.Fatalf("manifest extractor: got %v want %v", got, want)
	}
	if got, want := manifest["source_name"], "source.pdf"; got != want {
		t.Fatalf("manifest source: got %v want %v", got, want)
	}
	if got, want := manifest["extraction_profile"], "docling-v1.12.0-dlparse_v4-accurate-v1"; got != want {
		t.Fatalf("manifest extraction profile: got %v want %v", got, want)
	}
}

func TestExtractDocumentsPublishesOneDatasetCollection(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".moxie")
	extractor := &pathExtractor{documents: map[string]prepare.Document{
		"guide.pdf":   {Markdown: "# Guide\n\nGuide content.", StructuredJSON: json.RawMessage(`{"texts":[{"text":"Guide content."}]}`)},
		"manual.docx": {Markdown: "# Manual\n\nManual content.", StructuredJSON: json.RawMessage(`{"texts":[{"text":"Manual content."}]}`)},
	}}
	manager, err := prepare.NewAt(extractor, root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	sourceDirectory := t.TempDir()
	paths := []string{filepath.Join(sourceDirectory, "guide.pdf"), filepath.Join(sourceDirectory, "manual.docx")}
	for index, path := range paths {
		if err := os.WriteFile(path, []byte(fmt.Sprintf("source-%d", index+1)), 0o600); err != nil {
			t.Fatalf("write source: %v", err)
		}
	}

	var updates []prepare.ExtractionProgress
	artifact, err := manager.ExtractDocuments(t.Context(), paths, func(progress prepare.ExtractionProgress) {
		updates = append(updates, progress)
	})
	if err != nil {
		t.Fatalf("extract documents: %v", err)
	}

	if artifact.ID == "" || filepath.Base(artifact.Directory) != artifact.ID {
		t.Fatalf("collection artifact: %+v", artifact)
	}
	assertFileContents(t, artifact.MarkdownPath, "# Source Document: guide.pdf\n\n# Guide\n\nGuide content.\n\n# Source Document: manual.docx\n\n# Manual\n\nManual content.")
	assertFileContents(t, filepath.Join(artifact.Directory, "source", "001-guide.pdf"), "source-1")
	assertFileContents(t, filepath.Join(artifact.Directory, "source", "002-manual.docx"), "source-2")
	assertFileContents(t, filepath.Join(artifact.Directory, "documents", "001", "extracted.md"), "# Guide\n\nGuide content.")
	assertFileContents(t, filepath.Join(artifact.Directory, "documents", "002", "extracted.md"), "# Manual\n\nManual content.")
	if got, want := len(updates), 4; got != want {
		t.Fatalf("progress updates = %d, want %d: %+v", got, want, updates)
	}
	for index := range 2 {
		started := updates[index*2]
		completed := updates[index*2+1]
		if started.Index != index || started.Completed || completed.Index != index || !completed.Completed {
			t.Fatalf("document %d progress = (%+v, %+v)", index, started, completed)
		}
	}

	manifestData, err := os.ReadFile(artifact.ManifestPath)
	if err != nil {
		t.Fatalf("read collection manifest: %v", err)
	}
	var manifest struct {
		Version int `json:"version"`
		Sources []struct {
			Name string `json:"name"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode collection manifest: %v", err)
	}
	if manifest.Version != 3 || len(manifest.Sources) != 2 || manifest.Sources[0].Name != "guide.pdf" || manifest.Sources[1].Name != "manual.docx" {
		t.Fatalf("collection manifest: %+v", manifest)
	}

	var structured struct {
		Documents []json.RawMessage `json:"documents"`
	}
	structuredData, err := os.ReadFile(artifact.StructuredPath)
	if err != nil {
		t.Fatalf("read collection JSON: %v", err)
	}
	if err := json.Unmarshal(structuredData, &structured); err != nil || len(structured.Documents) != 2 {
		t.Fatalf("collection JSON: documents=%d err=%v", len(structured.Documents), err)
	}

	documents, err := prepare.DatasetDocuments(artifact.Directory)
	if err != nil {
		t.Fatalf("enumerate dataset documents: %v", err)
	}
	wantDocuments := []prepare.DatasetDocument{
		{ID: "001", Name: "guide.pdf", Directory: filepath.Join(artifact.Directory, "documents", "001")},
		{ID: "002", Name: "manual.docx", Directory: filepath.Join(artifact.Directory, "documents", "002")},
	}
	if !reflect.DeepEqual(documents, wantDocuments) {
		t.Fatalf("dataset documents = %#v, want %#v", documents, wantDocuments)
	}
}

func TestDatasetDocumentsSupportsSingleDocumentArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".moxie")
	manager, err := prepare.NewAt(&pathExtractor{documents: map[string]prepare.Document{
		"guide.pdf": {Markdown: "Guide content.", StructuredJSON: json.RawMessage(`{"texts":[{"text":"Guide content."}]}`)},
	}}, root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	source := filepath.Join(t.TempDir(), "guide.pdf")
	if err := os.WriteFile(source, []byte("guide"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	artifact, err := manager.Extract(t.Context(), source)
	if err != nil {
		t.Fatalf("extract document: %v", err)
	}

	documents, err := prepare.DatasetDocuments(artifact.Directory)
	if err != nil {
		t.Fatalf("enumerate dataset documents: %v", err)
	}
	want := []prepare.DatasetDocument{{ID: "001", Name: "guide.pdf", Directory: artifact.Directory}}
	if !reflect.DeepEqual(documents, want) {
		t.Fatalf("dataset documents = %#v, want %#v", documents, want)
	}
}

func TestExtractRejectsInvalidStructuredJSON(t *testing.T) {
	for _, structured := range []json.RawMessage{nil, json.RawMessage(`not JSON`), json.RawMessage(`{}`)} {
		manager, err := prepare.NewAt(extractor{document: prepare.Document{
			Markdown:       "Prepared content.",
			StructuredJSON: structured,
		}}, filepath.Join(t.TempDir(), ".moxie"))
		if err != nil {
			t.Fatal(err)
		}
		source := filepath.Join(t.TempDir(), "guide.pdf")
		if err := os.WriteFile(source, []byte("guide"), 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := manager.Extract(t.Context(), source); err == nil || !strings.Contains(err.Error(), "invalid structured JSON") {
			t.Fatalf("structured JSON %q: error = %v", structured, err)
		}
	}
}

type pathExtractor struct {
	documents map[string]prepare.Document
	calls     int
}

func (e *pathExtractor) Convert(_ context.Context, sourcePath string) (prepare.Document, error) {
	e.calls++
	document, exists := e.documents[filepath.Base(sourcePath)]
	if !exists {
		return prepare.Document{}, fmt.Errorf("unexpected source %q", sourcePath)
	}
	return document, nil
}

func (*pathExtractor) ExtractionProfile() string {
	return extractionProfile
}

func TestExtractReusesExactContentsAcrossPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".moxie")
	extractor := &countingExtractor{document: prepare.Document{
		Markdown:       "Prepared content.",
		StructuredJSON: json.RawMessage(`{"texts":[{"text":"Prepared content."}]}`),
		ProcessingTime: 12,
	}}
	manager, err := prepare.NewAt(extractor, root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	firstPath := filepath.Join(t.TempDir(), "first.pdf")
	secondPath := filepath.Join(t.TempDir(), "renamed.pdf")
	for _, path := range []string{firstPath, secondPath} {
		if err := os.WriteFile(path, []byte("identical document contents"), 0o600); err != nil {
			t.Fatalf("write source: %v", err)
		}
	}

	first, err := manager.Extract(context.Background(), firstPath)
	if err != nil {
		t.Fatalf("first extraction: %v", err)
	}
	cached, found, err := manager.FindCachedExtraction(context.Background(), secondPath)
	if err != nil {
		t.Fatalf("find cached extraction: %v", err)
	}
	if !found || cached.Directory != first.Directory {
		t.Fatalf("cached artifact = (%v, %q), want (true, %q)", found, cached.Directory, first.Directory)
	}

	second, err := manager.Extract(context.Background(), secondPath)
	if err != nil {
		t.Fatalf("second extraction: %v", err)
	}
	if second.Directory != first.Directory {
		t.Fatalf("reused directory = %q, want %q", second.Directory, first.Directory)
	}
	if extractor.calls != 1 {
		t.Fatalf("Docling calls = %d, want 1", extractor.calls)
	}
	differentFormat := filepath.Join(t.TempDir(), "renamed.docx")
	if err := os.WriteFile(differentFormat, []byte("identical document contents"), 0o600); err != nil {
		t.Fatalf("write different format: %v", err)
	}
	if _, found, err := manager.FindCachedExtraction(context.Background(), differentFormat); err != nil {
		t.Fatalf("find different format: %v", err)
	} else if found {
		t.Fatal("cached PDF extraction was reused as DOCX")
	}

	if err := os.WriteFile(secondPath, []byte("changed document contents"), 0o600); err != nil {
		t.Fatalf("change source: %v", err)
	}
	changed, err := manager.Extract(context.Background(), secondPath)
	if err != nil {
		t.Fatalf("changed extraction: %v", err)
	}
	if changed.Directory == first.Directory {
		t.Fatalf("changed source reused %q", changed.Directory)
	}
	if extractor.calls != 2 {
		t.Fatalf("Docling calls after change = %d, want 2", extractor.calls)
	}
}

func TestExtractRejectsTamperedOrSymlinkedCache(t *testing.T) {
	for _, test := range []struct {
		name   string
		tamper func(t *testing.T, artifact prepare.Artifact)
	}{
		{name: "modified extraction", tamper: func(t *testing.T, artifact prepare.Artifact) {
			if err := os.WriteFile(artifact.MarkdownPath, []byte("tampered"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlinked extraction", tamper: func(t *testing.T, artifact prepare.Artifact) {
			outside := filepath.Join(t.TempDir(), "outside.md")
			if err := os.WriteFile(outside, []byte("Prepared content."), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(artifact.MarkdownPath); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, artifact.MarkdownPath); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			extractor := &countingExtractor{document: prepare.Document{Markdown: "Prepared content.", StructuredJSON: json.RawMessage(`{"texts":[{"text":"Prepared content."}]}`)}}
			manager, err := prepare.NewAt(extractor, filepath.Join(t.TempDir(), ".moxie"))
			if err != nil {
				t.Fatal(err)
			}
			sourcePath := filepath.Join(t.TempDir(), "source.pdf")
			if err := os.WriteFile(sourcePath, []byte("document contents"), 0o600); err != nil {
				t.Fatal(err)
			}
			first, err := manager.Extract(context.Background(), sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			test.tamper(t, first)
			second, err := manager.Extract(context.Background(), sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			if second.Directory == first.Directory || extractor.calls != 2 {
				t.Fatalf("corrupted cache reused: first=%q second=%q calls=%d", first.Directory, second.Directory, extractor.calls)
			}
		})
	}
}

func TestExtractIgnoresIncompleteCachedArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".moxie")
	extractor := &countingExtractor{document: prepare.Document{
		Markdown:       "Prepared content.",
		StructuredJSON: json.RawMessage(`{"texts":[{"text":"Prepared content."}]}`),
	}}
	manager, err := prepare.NewAt(extractor, root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	sourcePath := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(sourcePath, []byte("document contents"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	first, err := manager.Extract(context.Background(), sourcePath)
	if err != nil {
		t.Fatalf("first extraction: %v", err)
	}
	if err := os.Remove(first.MarkdownPath); err != nil {
		t.Fatalf("remove cached Markdown: %v", err)
	}

	second, err := manager.Extract(context.Background(), sourcePath)
	if err != nil {
		t.Fatalf("replacement extraction: %v", err)
	}
	if second.Directory == first.Directory {
		t.Fatalf("incomplete artifact was reused: %q", second.Directory)
	}
	if extractor.calls != 2 {
		t.Fatalf("Docling calls = %d, want 2", extractor.calls)
	}
}

func assertFileContents(t *testing.T, path string, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if got := string(data); got != want {
		t.Fatalf("contents of %s: got %q want %q", path, got, want)
	}
}
