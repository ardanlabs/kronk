package extract_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/extract"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

type extractor struct {
	profile string
	convert func(context.Context, string) (providers.Document, error)
}

func (e extractor) ExtractionProfile() string { return e.profile }
func (e extractor) Convert(ctx context.Context, path string) (providers.Document, error) {
	return e.convert(ctx, path)
}

func roots(t *testing.T, format, content string) (*os.Root, *os.Root, uploads.File) {
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
	return source, workspace, uploads.File{ID: "doc", Format: format, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(content)))}
}

func openSource(root *os.Root, info uploads.File) func(context.Context) (io.ReadCloser, error) {
	return func(context.Context) (io.ReadCloser, error) {
		return root.Open("imports/doc/source" + info.Format)
	}
}

func TestRunText(t *testing.T) {
	for _, format := range []string{".txt", ".md"} {
		t.Run(format, func(t *testing.T) {
			text := "# Harbor\n\nThe harbor opens at dawn."
			source, workspace, info := roots(t, format, text)
			doc, err := extract.Run(t.Context(), nil, info, openSource(source, info), workspace)
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
			cached, err := extract.Run(t.Context(), nil, info, openSource(source, info), workspace)
			if err != nil || !reflect.DeepEqual(cached, doc) {
				t.Fatalf("cached = %+v, %v", cached, err)
			}
		})
	}
}

func TestRunConversion(t *testing.T) {
	source, workspace, info := roots(t, ".pdf", "private source bytes")
	want := providers.Document{Markdown: "Converted evidence.", StructuredJSON: json.RawMessage(`{"texts":[]}`)}
	var snapshot string
	e := extractor{profile: "v1", convert: func(ctx context.Context, path string) (providers.Document, error) {
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
	got, err := extract.Run(t.Context(), e, info, openSource(source, info), workspace)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("document = %+v, %v", got, err)
	}
	if _, err := os.Stat(snapshot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot not cleaned up: %v", err)
	}
	failure := errors.New("conversion failed")
	e.convert = func(context.Context, string) (providers.Document, error) { return providers.Document{}, failure }
	if _, err := extract.Run(t.Context(), e, info, openSource(source, info), workspace); err != nil {
		t.Fatalf("cached conversion retried: %v", err)
	}
	e.profile = "v2"
	if _, err := extract.Run(t.Context(), e, info, openSource(source, info), workspace); !errors.Is(err, failure) {
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
			if _, err := extract.Run(ctx, nil, info, openSource(source, info), workspace); err == nil {
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

func TestRead(t *testing.T) {
	for _, format := range []string{".txt", ".md", ".pdf", ".docx"} {
		t.Run(format, func(t *testing.T) {
			text := "The ferry departs at noon. The harbor opens at dawn."
			source, workspace, info := roots(t, format, text)
			want := providers.Document{Markdown: "Converted ferry evidence.", StructuredJSON: json.RawMessage(`{"texts":[{"text":"Ferry"}]}`)}
			e := extractor{profile: "v1", convert: func(context.Context, string) (providers.Document, error) {
				return want, nil
			}}
			if format == ".txt" || format == ".md" {
				structured, err := json.Marshal(map[string]any{"texts": []map[string]string{{"text": text}}})
				if err != nil {
					t.Fatal(err)
				}
				want = providers.Document{Markdown: text, StructuredJSON: structured}
			}
			if _, err := extract.Run(t.Context(), e, info, openSource(source, info), workspace); err != nil {
				t.Fatal(err)
			}
			if err := source.Remove("imports/doc/source" + format); err != nil {
				t.Fatal(err)
			}
			e.convert = func(context.Context, string) (providers.Document, error) {
				return providers.Document{}, errors.New("conversion unavailable")
			}
			got, err := extract.Read(t.Context(), e, info, workspace)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("Read = %+v, %v; want %+v", got, err, want)
			}
		})
	}
}

func TestReadRejectsUnavailableExtraction(t *testing.T) {
	for _, tc := range []struct {
		name string
		want error
	}{
		{name: "missing", want: os.ErrNotExist},
		{name: "source mismatch", want: preparation.ErrInvalidArgument},
		{name: "profile mismatch", want: preparation.ErrInvalidArgument},
		{name: "missing extractor", want: preparation.ErrInvalidArgument},
		{name: "malformed"},
		{name: "canceled", want: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, workspace, info := roots(t, ".pdf", "private PDF source")
			e := extractor{profile: "v1", convert: func(context.Context, string) (providers.Document, error) {
				return providers.Document{Markdown: "Extracted evidence.", StructuredJSON: json.RawMessage(`{"texts":[]}`)}, nil
			}}
			if _, err := extract.Run(t.Context(), e, info, openSource(source, info), workspace); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var provider preparation.DocumentExtractor = e
			switch tc.name {
			case "missing":
				if err := workspace.Remove("extraction.json"); err != nil {
					t.Fatal(err)
				}
			case "source mismatch":
				info.SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("another PDF source")))
			case "profile mismatch":
				e.profile = "v2"
				provider = e
			case "missing extractor":
				provider = nil
			case "malformed":
				if err := workspace.WriteFile("extraction.json", []byte(`{"source_hash":`), 0600); err != nil {
					t.Fatal(err)
				}
			case "canceled":
				cancel()
			}
			before, beforeErr := workspace.ReadFile("extraction.json")
			got, err := extract.Read(ctx, provider, info, workspace)
			if err == nil || !reflect.DeepEqual(got, providers.Document{}) {
				t.Fatalf("Read = %+v, %v; want an error and no document", got, err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("error = %v; want %v", err, tc.want)
			}
			if tc.name == "malformed" {
				if _, ok := errors.AsType[*json.SyntaxError](err); !ok {
					t.Fatalf("error = %v; want a JSON syntax error", err)
				}
			}
			after, afterErr := workspace.ReadFile("extraction.json")
			if string(after) != string(before) || errors.Is(afterErr, os.ErrNotExist) != errors.Is(beforeErr, os.ErrNotExist) || afterErr != nil && !errors.Is(afterErr, os.ErrNotExist) {
				t.Fatalf("Read modified extraction: before %q, %v; after %q, %v", before, beforeErr, after, afterErr)
			}
		})
	}
}

func TestRunRefreshesExtraction(t *testing.T) {
	for _, name := range []string{"missing", "source mismatch", "profile mismatch"} {
		t.Run(name, func(t *testing.T) {
			source, workspace, info := roots(t, ".pdf", "original PDF source")
			e := extractor{profile: "v1", convert: func(context.Context, string) (providers.Document, error) {
				return providers.Document{Markdown: "Original evidence.", StructuredJSON: json.RawMessage(`{"texts":[]}`)}, nil
			}}
			if _, err := extract.Run(t.Context(), e, info, openSource(source, info), workspace); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "missing":
				if err := workspace.Remove("extraction.json"); err != nil {
					t.Fatal(err)
				}
			case "source mismatch":
				content := "updated PDF source"
				if err := source.WriteFile("imports/doc/source.pdf", []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
				info.SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
			case "profile mismatch":
				e.profile = "v2"
			}
			want := providers.Document{Markdown: "Refreshed evidence.", StructuredJSON: json.RawMessage(`{"texts":[{"text":"Refreshed"}]}`)}
			e.convert = func(context.Context, string) (providers.Document, error) { return want, nil }
			got, err := extract.Run(t.Context(), e, info, openSource(source, info), workspace)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("Run = %+v, %v; want %+v", got, err, want)
			}
			cached, err := extract.Read(t.Context(), e, info, workspace)
			if err != nil || !reflect.DeepEqual(cached, want) {
				t.Fatalf("Read refreshed extraction = %+v, %v; want %+v", cached, err, want)
			}
		})
	}
}
