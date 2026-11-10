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
	"slices"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/extract"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
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

func TestSourceUnitsSavedQuoteBoundaries(t *testing.T) {
	// Saved page-split extraction, not a repaired or reattributed quotation.
	blocks := []string{
		`- "Programming is, among other things, a kind of writing. One way to learn writing is to write, but in all other forms of writing, one also reads. We read`,
		`- examples both good and bad to facilitate learning. But how many programmers learn to write programs by reading programs?" - Gerald M. Weinberg`,
		`- "Skill develops when we produce, not consume." - Katrina Owen`,
	}
	doc := providers.Document{Markdown: "## Quotes\n\n" + strings.Join(blocks, "\n\n")}
	for _, goal := range []preparation.Goal{preparation.GoalQuestionAnswer, preparation.GoalTextGeneration} {
		t.Run(string(goal), func(t *testing.T) {
			units, exclusions, err := extract.SourceUnits(doc, goal)
			if err != nil || len(exclusions) != 0 || len(units) != len(blocks) {
				t.Fatalf("SourceUnits = %+v, %+v, %v", units, exclusions, err)
			}
			for i, unit := range units {
				if !slices.Equal(unit.Targets, []string{blocks[i]}) {
					t.Errorf("unit %d targets = %q; want only original block %q", i, unit.Targets, blocks[i])
				}
				for _, target := range unit.Targets {
					if target == "We read" || strings.Contains(target, "We read examples") || strings.Contains(target, "Weinberg") && strings.Contains(target, "Katrina") {
						t.Errorf("manufactured or mixed-author target: %q", target)
					}
				}
				if !strings.Contains(unit.Evidence, "[FOCAL BEGIN]\n"+blocks[i]+"\n[FOCAL END]") {
					t.Errorf("focal block changed: %q", unit.Evidence)
				}
			}
			if !strings.Contains(units[0].Evidence, blocks[1]) || strings.Contains(units[0].Evidence, blocks[2]) || !strings.Contains(units[1].Evidence, blocks[0]) || !strings.Contains(units[1].Evidence, blocks[2]) {
				t.Fatalf("immediate local context lost or distant block included: %+v", units)
			}
		})
	}
}

const sourceDocling = `{
 "schema_name":"DoclingDocument",
 "body":{"self_ref":"#/body","children":[{"$ref":"#/groups/0"},{"$ref":"#/texts/0"},{"$ref":"#/texts/2"}]},
 "groups":[{"self_ref":"#/groups/0","label":"list","children":[{"$ref":"#/texts/1"}]}],
 "texts":[
  {"self_ref":"#/texts/0","label":"text","text":"Second ferry carries 7 passengers."},
  {"self_ref":"#/texts/1","label":"list_item","text":"First ferry carries 3 passengers."},
  {"self_ref":"#/texts/2","label":"text","content_layer":"furniture","text":"10"}
 ]
}`

func TestSourceUnitsDoclingReadingOrder(t *testing.T) {
	doc := providers.Document{Markdown: "Wrong Markdown carries 99 passengers.", StructuredJSON: json.RawMessage(sourceDocling)}
	for _, goal := range []preparation.Goal{preparation.GoalQuestionAnswer, preparation.GoalNumericPrediction} {
		t.Run(string(goal), func(t *testing.T) {
			units, exclusions, err := extract.SourceUnits(doc, goal)
			if err != nil || len(units) != 2 || len(exclusions) != 1 {
				t.Fatalf("SourceUnits = %+v, %+v, %v", units, exclusions, err)
			}
			for i, reference := range []string{"#/texts/1", "#/texts/0"} {
				if units[i].Reference != reference || strings.Contains(units[i].Evidence, "10") || strings.Contains(units[i].Evidence, "Wrong Markdown") {
					t.Errorf("unit %d = %+v", i, units[i])
				}
			}
			if strings.Index(units[0].Evidence, "First ferry") >= strings.Index(units[0].Evidence, "Second ferry") {
				t.Errorf("evidence not in body/group reading order: %q", units[0].Evidence)
			}
			if goal == preparation.GoalNumericPrediction && (!slices.Equal(units[0].Targets, []string{"3"}) || !slices.Equal(units[1].Targets, []string{"7"})) {
				t.Errorf("numeric targets include furniture or wrong focal block: %+v", units)
			}
			if exclusions[0].Reference != "#/texts/2" || !strings.Contains(exclusions[0].Reason, "furniture") {
				t.Errorf("footer exclusion = %+v", exclusions)
			}
		})
	}
}

func TestSourceUnitsDoclingFailsClosed(t *testing.T) {
	for _, tc := range []struct{ name, old, replacement, reason string }{
		{"malformed body", `"self_ref":"#/body"`, `"self_ref":"#/wrong"`, "malformed"},
		{"malformed node", `"self_ref":"#/texts/1"`, `"self_ref":"#/texts/8"`, "malformed"},
		{"malformed reference", `{"$ref":"#/texts/1"}`, `{"ref":"#/texts/1"}`, "reference"},
		{"unresolved reference", `{"$ref":"#/texts/1"}`, `{"$ref":"#/texts/99"}`, "reference"},
		{"cycle", `{"$ref":"#/texts/1"}`, `{"$ref":"#/body"}`, "cyclic"},
		{"repeated reference", `{"$ref":"#/texts/1"}`, `{"$ref":"#/texts/1"},{"$ref":"#/texts/1"}`, "repeated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := providers.Document{Markdown: "This fallback must not be selected.", StructuredJSON: json.RawMessage(strings.Replace(sourceDocling, tc.old, tc.replacement, 1))}
			units, _, err := extract.SourceUnits(doc, preparation.GoalQuestionAnswer)
			if err == nil || !strings.Contains(err.Error(), tc.reason) || len(units) != 0 {
				t.Fatalf("SourceUnits = %+v, %v; want no units and %q error", units, err, tc.reason)
			}
		})
	}
}

func TestSourceUnitsMarkdownStructure(t *testing.T) {
	code := "```go\n// # Not a heading\nvalue := 73\n\nvalue++\n```"
	blocks := []string{"Before the list.", "- First item.", "- Second item.", code, "After the code.", "Other section only."}
	markdown := "# Manual\n\n## Local\n\n" + strings.Join(blocks[:5], "\n\n") + "\n\n## Remote\n\n" + blocks[5]
	// Text Run saves this synthetic shape; it must not flatten Markdown structure.
	structured, err := json.Marshal(map[string]any{"texts": []map[string]string{{"text": "Synthetic decoy must not become a target."}}})
	if err != nil {
		t.Fatal(err)
	}
	units, exclusions, err := extract.SourceUnits(providers.Document{Markdown: markdown, StructuredJSON: structured}, preparation.GoalQuestionAnswer)
	if err != nil || len(exclusions) != 0 || len(units) != len(blocks) {
		t.Fatalf("SourceUnits = %+v, %+v, %v", units, exclusions, err)
	}
	for i, unit := range units {
		if !slices.Equal(unit.Targets, []string{blocks[i]}) || !strings.Contains(unit.Evidence, "# Manual") {
			t.Errorf("unit %d = %+v; want intact block %q and ancestor heading", i, unit, blocks[i])
		}
		own, other := "## Local", "## Remote"
		if i == len(blocks)-1 {
			own, other = other, own
		}
		if !strings.Contains(unit.Evidence, own) || strings.Contains(unit.Evidence, other) || strings.Contains(unit.Evidence, "Synthetic decoy") {
			t.Errorf("heading ancestry/section isolation failed: %+v", unit)
		}
	}
	if strings.Contains(units[4].Evidence, blocks[5]) || strings.Contains(units[5].Evidence, blocks[4]) || !strings.Contains(units[2].Evidence, code) {
		t.Fatalf("adjacency crossed sections or lost intact code: %+v", units)
	}
}

func TestSourceUnitsWholeBlockBudgets(t *testing.T) {
	for _, size := range []int{400, 401, 800, 801, 1201, 2900, 3000} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			text := strings.Repeat("界", size-1) + "。"
			units, exclusions, err := extract.SourceUnits(providers.Document{Markdown: text}, preparation.GoalTextGeneration)
			if err != nil {
				t.Fatal(err)
			}
			if size <= 2900 {
				if len(units) != 1 || len(exclusions) != 0 || !slices.Equal(units[0].Targets, []string{text}) || !strings.Contains(units[0].Evidence, "[FOCAL BEGIN]\n"+text+"\n[FOCAL END]") {
					t.Fatalf("whole %d-rune block changed or excluded: %+v, %+v", size, units, exclusions)
				}
			} else if len(units) != 0 || len(exclusions) != 1 || exclusions[0].Reference != "markdown:line:1" || !strings.Contains(exclusions[0].Reason, "3000") {
				t.Fatalf("oversized evidence truncated or unreported: %+v, %+v", units, exclusions)
			}
		})
	}
	t.Run("mandatory heading evidence", func(t *testing.T) {
		markdown := "# " + strings.Repeat("界", 3000) + "\n\nSmall valid focal block."
		units, exclusions, err := extract.SourceUnits(providers.Document{Markdown: markdown}, preparation.GoalQuestionAnswer)
		if err != nil || len(units) != 0 || len(exclusions) != 1 || exclusions[0].Reference != "markdown:line:3" || !strings.Contains(exclusions[0].Reason, "3000") || !strings.Contains(exclusions[0].Reason, "heading") {
			t.Fatalf("mandatory evidence truncated or unreported: %+v, %+v, %v", units, exclusions, err)
		}
	})
}

func TestSourceUnitsProsePassagePacking(t *testing.T) {
	for _, tc := range []struct {
		name, markdown string
		want           []string
	}{
		{"adjacent prose", "# Harbor\n\nOpens at dawn.\n\nCloses at dusk.", []string{"Opens at dawn.\n\nCloses at dusk."}},
		{"section boundary", "# Harbor\n\nOpens at dawn.\n\n# Ferry\n\nLeaves at noon.", []string{"Opens at dawn.", "Leaves at noon."}},
		{"list control", "Before.\n\n- First.\n\n- Second.\n\nAfter.", []string{"Before.", "- First.", "- Second.", "After."}},
		{"code control", "Before.\n\n```\ncode\n```\n\nAfter.", []string{"Before.", "```\ncode\n```", "After."}},
		{"exact soft bound", strings.Repeat("界", 599) + "\n\n" + strings.Repeat("語", 599), []string{strings.Repeat("界", 599) + "\n\n" + strings.Repeat("語", 599)}},
		{"over soft bound", strings.Repeat("界", 600) + "\n\n" + strings.Repeat("語", 599), []string{strings.Repeat("界", 600), strings.Repeat("語", 599)}},
		{"single block above soft bound", strings.Repeat("界", 1201), []string{strings.Repeat("界", 1201)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			units, exclusions, err := extract.SourceUnits(providers.Document{Markdown: tc.markdown}, preparation.GoalQuestionAnswer)
			if err != nil || len(exclusions) != 0 || len(units) != len(tc.want) {
				t.Fatalf("passage plan = %+v, %+v, %v", units, exclusions, err)
			}
			for i, want := range tc.want {
				if !slices.Equal(units[i].Targets, []string{want}) || !strings.Contains(units[i].Evidence, "[FOCAL BEGIN]\n"+want+"\n[FOCAL END]") {
					t.Fatalf("unit %d lost whole passage: %+v", i, units[i])
				}
			}
		})
	}
}

func TestSourceUnitsPictureOCRPassage(t *testing.T) {
	document := providers.Document{Markdown: "Fallback must not be selected.", StructuredJSON: json.RawMessage(`{
 "schema_name":"DoclingDocument",
 "body":{"self_ref":"#/body","children":[{"$ref":"#/texts/0"},{"$ref":"#/texts/1"},{"$ref":"#/pictures/0"},{"$ref":"#/texts/5"}]},
 "pictures":[{"self_ref":"#/pictures/0","label":"picture","children":[{"$ref":"#/texts/2"},{"$ref":"#/texts/3"},{"$ref":"#/texts/4"}]}],
 "texts":[
 {"self_ref":"#/texts/0","label":"title","level":1,"text":"Manual"},
 {"self_ref":"#/texts/1","label":"section_header","level":2,"text":"Harbor"},
 {"self_ref":"#/texts/2","label":"section_header","level":1,"text":"Opening labels"},
 {"self_ref":"#/texts/3","label":"text","text":"Amber"},
 {"self_ref":"#/texts/4","label":"text","text":"Dawn"},
 {"self_ref":"#/texts/5","label":"text","text":"Outside picture prose."}
 ]}`)}
	units, exclusions, err := extract.SourceUnits(document, preparation.GoalQuestionAnswer)
	if err != nil || len(units) != 2 || len(exclusions) != 1 {
		t.Fatalf("picture plan = %+v, %+v, %v", units, exclusions, err)
	}
	if units[0].Reference != "#/pictures/0" || !slices.Equal(units[0].Targets, []string{"Opening labels\n\nAmber\n\nDawn"}) || !slices.Equal(units[1].Targets, []string{"Outside picture prose."}) {
		t.Fatalf("OCR was fragmented, merged with prose or lost its heading: %+v", units)
	}
	for _, unit := range units {
		if !strings.Contains(unit.Evidence, "Manual\n\nHarbor\n\n") {
			t.Fatalf("picture heading changed enclosing ancestry: %+v", unit)
		}
	}
	if exclusions[0].Reference != "#/pictures/0" || !strings.Contains(exclusions[0].Reason, "visual relationships unsupported") {
		t.Fatalf("visual limitation not reported: %+v", exclusions)
	}
}

func TestSourceUnitsOmittedStructureBarriers(t *testing.T) {
	for _, middle := range []string{"![diagram](missing.png)", "| Name | Value |\n| --- | --- |\n| Missing | table fact |", "| Name |\n| --- |\n| Missing table fact |"} {
		t.Run(middle, func(t *testing.T) {
			units, exclusions, err := extract.SourceUnits(providers.Document{Markdown: "# Manual\n\nBefore.\n\n" + middle + "\n\nAfter."}, preparation.GoalQuestionAnswer)
			if err != nil || len(units) != 2 || len(exclusions) != 1 {
				t.Fatalf("omitted structure barrier = %+v, %+v, %v", units, exclusions, err)
			}
			for i, text := range []string{"Before.", "After."} {
				if !slices.Equal(units[i].Targets, []string{text}) || !strings.Contains(units[i].Evidence, "# Manual") || strings.Contains(units[i].Evidence, []string{"After.", "Before."}[i]) {
					t.Fatalf("prose crossed omitted structure or lost headings: %+v", units[i])
				}
			}
		})
	}
	for _, tc := range []struct{ label, reference, extraText, table string }{
		{"table", "#/tables/0", "", `{"self_ref":"#/tables/0","label":"table"}`},
		{"formula", "#/texts/3", `,{"self_ref":"#/texts/3","label":"formula","text":"omitted"}`, ""},
	} {
		t.Run("Docling "+tc.label, func(t *testing.T) {
			data := fmt.Sprintf(`{"schema_name":"DoclingDocument","body":{"self_ref":"#/body","children":[{"$ref":"#/texts/0"},{"$ref":"#/texts/1"},{"$ref":%q},{"$ref":"#/texts/2"}]},"texts":[{"self_ref":"#/texts/0","label":"title","level":1,"text":"Manual"},{"self_ref":"#/texts/1","label":"text","text":"Before."},{"self_ref":"#/texts/2","label":"text","text":"After."}%s],"tables":[%s]}`, tc.reference, tc.extraText, tc.table)
			units, exclusions, err := extract.SourceUnits(providers.Document{StructuredJSON: json.RawMessage(data)}, preparation.GoalQuestionAnswer)
			if err != nil || len(units) != 2 || len(exclusions) != 1 {
				t.Fatalf("Docling barrier = %+v, %+v, %v", units, exclusions, err)
			}
			for i, text := range []string{"Before.", "After."} {
				if !slices.Equal(units[i].Targets, []string{text}) || !strings.Contains(units[i].Evidence, "Manual") || strings.Contains(units[i].Evidence, []string{"After.", "Before."}[i]) {
					t.Fatalf("Docling prose crossed omitted structure: %+v", units[i])
				}
			}
		})
	}
}

func TestStepReviewedRecordRetainsPassage(t *testing.T) {
	for _, name := range []string{"approved", "wrong source", "changed evidence", "not approved", "invalid prompt", "source integrity"} {
		t.Run(name, func(t *testing.T) {
			text := "# Harbor\n\nThe harbor opens at dawn.\n\nThe harbor closes at dusk."
			source, workspace, info := roots(t, ".md", text)
			units, exclusions, err := extract.SourceUnits(providers.Document{Markdown: text}, preparation.GoalQuestionAnswer)
			if err != nil || len(units) != 1 || len(exclusions) != 0 {
				t.Fatalf("source plan = %+v, %+v, %v", units, exclusions, err)
			}
			reviewed := training.Record{
				Prompt: "  When does the harbor open?  ", Target: "  At\t dawn.  ",
				Evidence: units[0].Evidence, SourceDocument: info.ID, UserApproved: true, FactID: "stale-id",
			}
			switch name {
			case "wrong source":
				reviewed.SourceDocument = "another-source"
			case "changed evidence":
				reviewed.Evidence += " "
			case "not approved":
				reviewed.UserApproved = false
			case "invalid prompt":
				reviewed.Prompt = "No question mark"
			case "source integrity":
				info.SHA256 = "incorrect hash"
			}
			input := preparation.StepContext{
				Workspace: workspace,
				Preparation: &preparation.PreparationContext{
					Input: preparation.PreparationInput{Kind: preparation.PreparationFull, Request: preparation.PreparationRequest{
						DatasetID: info.ID, Model: "catalog/qwen", Goal: preparation.GoalQuestionAnswer,
						ReviewedRecords: []training.Record{reviewed},
					}},
					Source: info, OpenSource: openSource(source, info),
				},
			}
			_, err = (extract.Step{}).Execute(t.Context(), input)
			if name != "approved" {
				if err == nil {
					t.Fatal("extraction accepted invalid reviewed ownership, evidence, prompt or source")
				}
				if _, err := workspace.Stat(checkpoint.File); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("invalid review/source committed a checkpoint: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			state, exists, err := checkpoint.Load(t.Context(), input, false)
			if err != nil || !exists {
				t.Fatalf("load initialized extraction = %+v, %t, %v", state, exists, err)
			}
			want := reviewed
			want.Prompt = strings.TrimSpace(want.Prompt)
			want.Target = training.Normalized(want.Target)
			want.FactID = training.FactID(want.Evidence, want.Target)
			if !slices.Equal(state.Records, []training.Record{want}) || state.ReviewedCount != 1 || !reflect.DeepEqual(state.Units, units) || state.Next != 0 || state.Generated || state.Verified != 0 {
				t.Fatalf("approved example or pending source passage lost: %+v", state)
			}
			if !strings.Contains(state.Units[0].Targets[0], "The harbor closes at dusk.") {
				t.Fatal("unreviewed fact was removed with the approved example")
			}
			if input.Preparation.Input.Request.ReviewedRecords[0] != reviewed {
				t.Fatal("normalizing saved review mutated the submitted record")
			}
			before, err := workspace.ReadFile(checkpoint.File)
			if err != nil {
				t.Fatal(err)
			}
			input.Preparation.OpenSource = func(context.Context) (io.ReadCloser, error) {
				t.Fatal("extraction replay reopened the source")
				return nil, errors.New("unexpected source open")
			}
			if _, err := (extract.Step{}).Execute(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			if after, err := workspace.ReadFile(checkpoint.File); err != nil || string(after) != string(before) {
				t.Fatalf("extraction replay changed approved example or passage: %v", err)
			}
		})
	}
}
