package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoclingConversion(t *testing.T) {
	for _, test := range []struct {
		ext     string
		options DocumentOptions
	}{{".pdf", DocumentOptions{}}, {".docx", DocumentOptions{}}, {".pdf", DocumentOptions{PageStart: 11, PageEnd: 20}}} {
		t.Run(fmt.Sprint(test), func(t *testing.T) {
			ext := test.ext
			var temporary atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/convert/file/async":
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					defer r.MultipartForm.RemoveAll()
					if r.Method != "POST" || !slices.Equal(r.MultipartForm.Value["to_formats"], []string{"md", "json"}) || r.FormValue("table_mode") != "accurate" {
						t.Error("incorrect conversion settings")
					}
					if (r.FormValue("pdf_backend") == "dlparse_v4") != (ext == ".pdf") {
						t.Error("wrong PDF backend option")
					}
					var pages []string
					switch {
					case test.options.PageStart > 0:
						pages = []string{"11", "20"}
					case ext == ".pdf":
						pages = []string{"1", "10"}
					}
					// A whole PDF asks for its next batch, which starts past the end.
					if got := r.MultipartForm.Value["page_range"]; ext == ".pdf" && test.options.PageStart == 0 && slices.Equal(got, []string{"11", "20"}) {
						fmt.Fprint(w, `{"task_id":"task-past","task_status":"pending"}`)
						return
					}
					if !slices.Equal(r.MultipartForm.Value["page_range"], pages) {
						t.Errorf("page range = %v, want %v", r.MultipartForm.Value["page_range"], pages)
					}
					file, header, err := r.FormFile("files")
					if err != nil {
						t.Error(err)
						return
					}
					defer file.Close()
					data, err := io.ReadAll(file)
					if err != nil || string(data) != "document fixture" || header.Filename != "source"+ext {
						t.Errorf("uploaded document: %s %+v %v", data, header, err)
					}
					fmt.Fprint(w, `{"task_id":"task-1","task_status":"pending"}`)
				case "/v1/status/poll/task-1", "/v1/status/poll/task-past":
					fmt.Fprint(w, `{"task_id":"task","task_status":"success"}`)
				case "/v1/result/task-past":
					fmt.Fprint(w, pastEnd)
				case "/v1/result/task-1":
					if !temporary.Swap(true) {
						w.WriteHeader(503)
						return
					}
					fmt.Fprint(w, `{"status":"success","processing_time":1.25,"document":{"md_content":"Extracted facts.","json_content":{"texts":[{"text":"Extracted facts."}]}}}`)
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			extractor, err := NewDoclingExtractor(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "source"+ext)
			if err := os.WriteFile(path, []byte("document fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
			document, err := extractor.Convert(t.Context(), path, test.options, nil)
			if err != nil || document.Markdown != "Extracted facts." || document.ProcessingTime != 1.25 || string(document.StructuredJSON) != `{"texts":[{"text":"Extracted facts."}]}` {
				t.Fatalf("conversion result: %+v %v", document, err)
			}
		})
	}
}

const pastEnd = `{"status":"failure","errors":[{"component_type":"user_input","error_message":"Document has 10 pages, fewer than the requested page_range start 11.","category":"policy"}]}`

// batch is a DoclingDocument for one batch whose items reference each other by
// batch-local index, as Docling returns them.
func batch(page int, heading, text string) string {
	return fmt.Sprintf(`{"schema_name":"DoclingDocument","body":{"self_ref":"#/body","children":[{"$ref":"#/texts/0"},{"$ref":"#/groups/0"}]},"furniture":{"self_ref":"#/furniture","children":[]},`+
		`"groups":[{"self_ref":"#/groups/0","parent":{"$ref":"#/body"},"children":[{"$ref":"#/texts/1"}],"label":"list"}],`+
		`"texts":[{"self_ref":"#/texts/0","parent":{"$ref":"#/body"},"children":[],"label":"section_header","text":%q,"prov":[{"page_no":%d,"bbox":{"l":72.25}}]},`+
		`{"self_ref":"#/texts/1","parent":{"$ref":"#/groups/0"},"children":[],"label":"list_item","text":%q}],`+
		`"pictures":[],"tables":[],"pages":{"%d":{"page_no":%d}}}`, heading, page, text, page, page)
}

func TestDoclingPageBatches(t *testing.T) {
	for _, test := range []struct {
		name    string
		options DocumentOptions
		// pages is the document length; Docling trims a batch at the last page.
		pages   int
		ranges  [][2]int
		failing string
		want    error
	}{
		{name: "whole document", pages: 25, ranges: [][2]int{{1, 10}, {11, 20}, {21, 30}, {31, 40}}},
		{name: "last batch full", pages: 20, ranges: [][2]int{{1, 10}, {11, 20}, {21, 30}}},
		{name: "page range", options: DocumentOptions{PageStart: 20, PageEnd: 31}, pages: 60, ranges: [][2]int{{20, 29}, {30, 31}}},
		{name: "range past the end", options: DocumentOptions{PageStart: 70, PageEnd: 80}, pages: 60, ranges: [][2]int{{70, 79}}, want: ErrPageRangePastEnd},
		{name: "conversion failure", pages: 60, ranges: [][2]int{{1, 10}}, failing: "1", want: ErrExtractionFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requested [][2]int
			var restarted atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/convert/file/async":
					// The first submission meets a restarting service.
					if !restarted.Swap(true) {
						io.Copy(io.Discard, r.Body)
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
						return
					}
					defer r.MultipartForm.RemoveAll()
					var pages [2]int
					fmt.Sscan(r.MultipartForm.Value["page_range"][0], &pages[0])
					fmt.Sscan(r.MultipartForm.Value["page_range"][1], &pages[1])
					requested = append(requested, pages)
					fmt.Fprintf(w, `{"task_id":"%d","task_status":"pending"}`, pages[0])
				case strings.HasPrefix(r.URL.Path, "/v1/status/poll/"):
					fmt.Fprint(w, `{"task_status":"success"}`)
				case strings.HasPrefix(r.URL.Path, "/v1/result/"):
					var start int
					fmt.Sscan(strings.TrimPrefix(r.URL.Path, "/v1/result/"), &start)
					switch {
					case strconv.Itoa(start) == test.failing:
						fmt.Fprint(w, `{"status":"failure","errors":[{"error_message":"conversion crashed"}]}`)
					case start > test.pages:
						fmt.Fprint(w, pastEnd)
					default:
						content, err := json.Marshal(json.RawMessage(batch(start, fmt.Sprintf("Section %d", start), fmt.Sprintf("Fact %d", start))))
						if err != nil {
							t.Error(err)
						}
						fmt.Fprintf(w, `{"status":"success","processing_time":1,"document":{"md_content":"Page %d","json_content":%s}}`, start, content)
					}
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			extractor, err := NewDoclingExtractor(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "source.pdf")
			if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
			var reported []int
			saved := make(map[[2]int]Document)
			document, err := extractor.Convert(t.Context(), path, test.options, func(pages int) { reported = append(reported, pages) }, DocumentCheckpoint{
				Save: func(start, end int, document Document) error {
					saved[[2]int{start, end}] = document
					return nil
				},
			})
			if !slices.Equal(requested, test.ranges) {
				t.Fatalf("requested pages %v, want %v", requested, test.ranges)
			}
			if test.want != nil {
				if !errors.Is(err, test.want) {
					t.Fatalf("error = %v, want %v", err, test.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var starts []int
			for _, pages := range test.ranges {
				if pages[0] <= test.pages {
					starts = append(starts, pages[0])
				}
			}
			// Each batch reports the pages read so far, never a batch past the end.
			var want []int
			for i := range starts {
				want = append(want, i+1)
			}
			if !slices.Equal(reported, want) {
				t.Fatalf("pages read reports %v, want %v", reported, want)
			}
			var markdown []string
			for _, start := range starts {
				markdown = append(markdown, fmt.Sprintf("Page %d", start))
			}
			if document.Markdown != strings.Join(markdown, "\n\n") || document.ProcessingTime != float64(len(starts)) {
				t.Fatalf("joined markdown %q, processing time %v", document.Markdown, document.ProcessingTime)
			}
			// A fresh converter must produce exactly the same joined references,
			// pages and text from saved batches, including the end-of-range probe.
			extractor, err = NewDoclingExtractor(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			requested = nil
			restored, err := extractor.Convert(t.Context(), path, test.options, nil, DocumentCheckpoint{
				Load: func(start, end int) (Document, error) {
					document, ok := saved[[2]int{start, end}]
					if !ok {
						return Document{}, os.ErrNotExist
					}
					return document, nil
				},
				Save: func(int, int, Document) error {
					t.Error("reconverted a committed batch")
					return errors.New("unexpected new batch")
				},
			})
			if err != nil || restored.Markdown != document.Markdown || string(restored.StructuredJSON) != string(document.StructuredJSON) || restored.ProcessingTime != document.ProcessingTime {
				t.Fatalf("restored document differs: %+v, %v", restored, err)
			}
			var probes [][2]int
			if test.options.PageEnd == 0 {
				probes = test.ranges[len(test.ranges)-1:]
			}
			if !slices.Equal(requested, probes) {
				t.Fatalf("restore requested %v; want only end probe %v", requested, probes)
			}
			var joined struct {
				Body struct {
					Children []struct {
						Ref string `json:"$ref"`
					}
				} `json:"body"`
				Groups []struct {
					SelfRef  string `json:"self_ref"`
					Children []struct {
						Ref string `json:"$ref"`
					} `json:"children"`
				} `json:"groups"`
				Texts []struct {
					SelfRef string `json:"self_ref"`
					Parent  struct {
						Ref string `json:"$ref"`
					} `json:"parent"`
					Text string                                   `json:"text"`
					Prov []struct{ Bbox struct{ L json.Number } } `json:"prov"`
				} `json:"texts"`
				Pages map[string]any `json:"pages"`
			}
			if err := json.Unmarshal(document.StructuredJSON, &joined); err != nil {
				t.Fatal(err)
			}
			if len(joined.Texts) != 2*len(starts) || len(joined.Groups) != len(starts) || len(joined.Body.Children) != 2*len(starts) || len(joined.Pages) != len(starts) {
				t.Fatalf("joined document: %s", document.StructuredJSON)
			}
			for i, start := range starts {
				// Batch i's items follow every item of the batches before it.
				heading, item, group := fmt.Sprintf("#/texts/%d", 2*i), fmt.Sprintf("#/texts/%d", 2*i+1), fmt.Sprintf("#/groups/%d", i)
				if joined.Body.Children[2*i].Ref != heading || joined.Body.Children[2*i+1].Ref != group ||
					joined.Texts[2*i].SelfRef != heading || joined.Texts[2*i].Text != fmt.Sprintf("Section %d", start) || joined.Texts[2*i].Parent.Ref != "#/body" ||
					joined.Texts[2*i+1].SelfRef != item || joined.Texts[2*i+1].Parent.Ref != group || joined.Texts[2*i+1].Text != fmt.Sprintf("Fact %d", start) ||
					joined.Groups[i].SelfRef != group || joined.Groups[i].Children[0].Ref != item || joined.Pages[strconv.Itoa(start)] == nil {
					t.Fatalf("batch %d references were not shifted: %s", i, document.StructuredJSON)
				}
				if joined.Texts[2*i].Prov[0].Bbox.L != "72.25" {
					t.Fatalf("joining changed a number: %s", joined.Texts[2*i].Prov[0].Bbox.L)
				}
			}
		})
	}
}

func TestDoclingCancellationAndInvalidResult(t *testing.T) {
	for _, mode := range []string{"pending", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					io.Copy(io.Discard, r.Body)
					if mode == "pending" {
						fmt.Fprint(w, `{"task_id":"task-1","task_status":"pending"}`)
					} else {
						fmt.Fprint(w, `{"task_id":"task-1","task_status":"success"}`)
					}
					return
				}
				fmt.Fprint(w, `{"status":"success","document":{"md_content":"Missing structured content","json_content":{}}}`)
			}))
			defer server.Close()
			extractor, err := NewDoclingExtractor(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "source.pdf")
			if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
			timeout := 5 * time.Second
			if mode == "pending" {
				timeout = 100 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(t.Context(), timeout)
			defer cancel()
			_, err = extractor.Convert(ctx, path, DocumentOptions{}, nil)
			if err == nil || mode == "pending" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("%s result: %v", mode, err)
			}
			if mode == "invalid" && (errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "invalid or empty document content")) {
				t.Fatalf("invalid content was not rejected: %v", err)
			}
		})
	}
}
