package extract_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/prepare/extract"
)

type extractor struct {
	profile string
	convert func(context.Context, string) (model.Document, error)
}

func (e extractor) ExtractionProfile() string { return e.profile }
func (e extractor) Convert(ctx context.Context, path string) (model.Document, error) {
	return e.convert(ctx, path)
}

func roots(t *testing.T, format, content string) (*os.Root, *os.Root, model.PreparationSource) {
	t.Helper()
	source, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { source.Close() })
	workspace, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { workspace.Close() })
	if err := source.MkdirAll("imports/doc", 0700); err != nil {
		t.Fatal(err)
	}
	if err := source.WriteFile("imports/doc/source"+format, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return source, workspace, model.PreparationSource{ID: "doc", Format: format, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(content)))}
}

func TestRunText(t *testing.T) {
	for _, format := range []string{".txt", ".md"} {
		t.Run(format, func(t *testing.T) {
			text := "# Harbor\n\nThe harbor opens at dawn."
			source, workspace, info := roots(t, format, text)
			doc, err := extract.Run(t.Context(), nil, info, source, workspace)
			if err != nil {
				t.Fatal(err)
			}
			var structured struct {
				Texts []struct {
					Text string `json:"text"`
				} `json:"texts"`
			}
			if err := json.Unmarshal(doc.StructuredJSON, &structured); err != nil {
				t.Fatal(err)
			}
			if doc.Markdown != text || len(structured.Texts) != 1 || structured.Texts[0].Text != text {
				t.Fatalf("document = %+v", doc)
			}
			if err := source.Remove("imports/doc/source" + format); err != nil {
				t.Fatal(err)
			}
			cached, err := extract.Run(t.Context(), nil, info, source, workspace)
			if err != nil || !reflect.DeepEqual(cached, doc) {
				t.Fatalf("cached = %+v, %v", cached, err)
			}
		})
	}
}

func TestRunConversion(t *testing.T) {
	source, workspace, info := roots(t, ".pdf", "private source bytes")
	want := model.Document{Markdown: "Converted evidence.", StructuredJSON: json.RawMessage(`{"texts":[]}`)}
	var snapshot string
	e := extractor{profile: "v1", convert: func(ctx context.Context, path string) (model.Document, error) {
		snapshot = path
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "private source bytes" {
			t.Fatalf("snapshot = %q", data)
		}
		return want, nil
	}}
	got, err := extract.Run(t.Context(), e, info, source, workspace)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("document = %+v, %v", got, err)
	}
	if _, err := os.Stat(snapshot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot not cleaned up: %v", err)
	}
	failure := errors.New("conversion failed")
	e.convert = func(context.Context, string) (model.Document, error) { return model.Document{}, failure }
	if _, err := extract.Run(t.Context(), e, info, source, workspace); err != nil {
		t.Fatalf("cached conversion retried: %v", err)
	}
	e.profile = "v2"
	if _, err := extract.Run(t.Context(), e, info, source, workspace); !errors.Is(err, failure) {
		t.Fatalf("profile change error = %v", err)
	}
}

func TestRunRejectsInvalidSources(t *testing.T) {
	for _, tc := range []struct {
		name, format, content string
		corrupt, cancel       bool
	}{
		{name: "hash mismatch", format: ".txt", content: "Evidence", corrupt: true},
		{name: "empty", format: ".txt", content: " \n"},
		{name: "invalid utf8", format: ".txt", content: "\xff"},
		{name: "missing extractor", format: ".pdf", content: "PDF"},
		{name: "canceled", format: ".txt", content: "Evidence", cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, workspace, info := roots(t, tc.format, tc.content)
			if tc.corrupt {
				info.SHA256 = "wrong"
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			if _, err := extract.Run(ctx, nil, info, source, workspace); err == nil {
				t.Fatal("accepted invalid source")
			} else if tc.cancel && !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v", err)
			}
			if _, err := workspace.Stat("extraction.json"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid result cached: %v", err)
			}
		})
	}
}
