package tuning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.temporal.io/sdk/mocks"
)

func TestDatasetImportIntegrityAndRollback(t *testing.T) {
	root := t.TempDir()
	b, err := New(&mocks.Client{}, Config{DatasetRoot: root, MaximumImportBytes: 12})
	if err != nil {
		t.Fatal(err)
	}
	text := "Hello source"
	dataset, err := b.ImportDataset(t.Context(), ImportInput{Name: " source.MD "}, strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(text))
	if dataset.SizeBytes != 12 || dataset.Name != "source.MD" || dataset.Format != ".md" || dataset.SHA256 != hex.EncodeToString(hash[:]) {
		t.Fatalf("metadata: %+v", dataset)
	}
	stored, err := b.GetDataset(t.Context(), dataset.ID)
	if err != nil || !reflect.DeepEqual(stored, dataset) {
		t.Fatalf("stored dataset: %+v %v", stored, err)
	}
	for _, input := range []struct{ name, text string }{
		{"../source.md", text}, {"source.exe", text}, {"empty.txt", ""}, {"large.md", text + "!"}, {"bad.jsonl", "not JSON"},
	} {
		if _, err := b.ImportDataset(t.Context(), ImportInput{Name: input.name}, strings.NewReader(input.text)); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("accepted %q: %v", input.name, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := b.ImportDataset(ctx, ImportInput{Name: "cancel.txt"}, strings.NewReader(text)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "imports"))
	if err != nil || len(entries) != 1 || entries[0].Name() != dataset.ID {
		t.Fatalf("failed imports were published: %v %v", entries, err)
	}
	if _, err := b.GetDataset(t.Context(), "../source"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("accepted path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "imports", dataset.ID, "source.md"), []byte("Other source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := b.GetDataset(t.Context(), dataset.ID); err == nil {
		t.Fatal("tampered source accepted")
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "imports", dataset.ID, "source.md")
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, source); err != nil {
		t.Fatal(err)
	}
	if _, err := b.GetDataset(t.Context(), dataset.ID); err == nil {
		t.Fatal("accepted an escaping symlink with a matching content hash")
	}
}

func TestImportRejectsEscapingDirectorySymlink(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "imports")); err != nil {
		t.Fatal(err)
	}
	b, err := New(&mocks.Client{}, Config{DatasetRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.ImportDataset(t.Context(), ImportInput{Name: "source.txt"}, strings.NewReader("Must stay inside the root.")); err == nil {
		t.Fatal("import accepted escaping storage")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("import wrote outside its root: %v %v", entries, err)
	}
}

func TestPreparedJSONLImport(t *testing.T) {
	b, err := New(&mocks.Client{}, Config{DatasetRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var source strings.Builder
	for i := range 60 {
		row := Record{Prompt: fmt.Sprintf("Question %03d?", i), Target: fmt.Sprintf("Answer %03d.", i), Evidence: fmt.Sprintf("Source passage %03d supports this answer and preserves its provenance.", i)}
		if err := json.NewEncoder(&source).Encode(row); err != nil {
			t.Fatal(err)
		}
	}
	dataset, err := b.ImportDataset(t.Context(), ImportInput{Name: "prepared.jsonl"}, strings.NewReader(source.String()))
	if err != nil || dataset.Records != 60 {
		t.Fatalf("JSONL import: %+v %v", dataset, err)
	}
	if _, err := b.ImportDataset(t.Context(), ImportInput{Name: "missing.jsonl"}, strings.NewReader(`{"prompt":"Question?","target":"Answer."}`+"\n")); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("accepted missing evidence: %v", err)
	}
}

func TestImportedDatasetReadinessBoundaries(t *testing.T) {
	for _, test := range []struct {
		name                                       string
		facts, variants                            int
		approvedVariants                           int
		sharedEvidence, approved, legacy, accepted bool
	}{
		{name: "distinct-evidence", facts: 50, variants: 1, accepted: true},
		{name: "normalized-shared-evidence", facts: 50, variants: 1, sharedEvidence: true},
		{name: "all-approved-training-only", facts: 50, variants: 1, approved: true},
		{name: "legacy-boundary", facts: 30, variants: 5, legacy: true, accepted: true},
		{name: "legacy-too-few-facts", facts: 29, variants: 5, legacy: true},
		{name: "legacy-too-few-variants", facts: 30, variants: 4, legacy: true},
		{name: "legacy-all-approved", facts: 30, variants: 5, legacy: true, approved: true},
		{name: "legacy-mixed-approved", facts: 30, variants: 5, legacy: true, approvedVariants: 3, accepted: true},
		{name: "legacy-insufficient-unapproved", facts: 30, variants: 5, legacy: true, approvedVariants: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, err := New(&mocks.Client{}, Config{DatasetRoot: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			var source strings.Builder
			for fact := range test.facts {
				for variant := range test.variants {
					r := Record{Prompt: fmt.Sprintf("Question %d variant %d?", fact, variant), Target: "yes", Evidence: fmt.Sprintf("Evidence %d says yes.", fact), UserApproved: test.approved || variant < test.approvedVariants}
					if test.sharedEvidence {
						r.Evidence = "Shared evidence says yes."
						if fact%2 == 0 {
							r.Evidence = " SHARED  evidence\n says yes. "
						}
					}
					if test.legacy {
						r.FactID = fmt.Sprintf("fact-%d", fact)
					}
					if err := json.NewEncoder(&source).Encode(r); err != nil {
						t.Fatal(err)
					}
				}
			}
			dataset, err := b.ImportDataset(t.Context(), ImportInput{Name: "records.jsonl"}, strings.NewReader(source.String()))
			if test.accepted {
				if err != nil || dataset.Records != test.facts*test.variants {
					t.Fatalf("eligible import: %+v %v", dataset, err)
				}
			} else if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("ineligible dataset accepted: %+v %v", dataset, err)
			}
		})
	}
}

func TestDatasetFactContractValidation(t *testing.T) {
	var records []Record
	for i := range 20 {
		r := Record{Prompt: fmt.Sprintf("Training question %02d?", i), Target: fmt.Sprintf("Answer %02d.", i), Evidence: fmt.Sprintf("Evidence for fact %02d.", i), FactID: fmt.Sprintf("fact-%02d", i)}
		records = append(records, r)
	}
	for i := range 10 {
		r := records[i]
		r.Prompt = fmt.Sprintf("Validation question %02d?", i)
		r.ValidationPrompt = true
		records = append(records, r)
	}
	for i := range 20 {
		r := records[i]
		r.Prompt = fmt.Sprintf("Independent question %02d?", i)
		r.IndependentPrompt = true
		records = append(records, r)
	}
	encode := func(rows []Record) string {
		var data strings.Builder
		for _, r := range rows {
			if err := json.NewEncoder(&data).Encode(r); err != nil {
				t.Fatal(err)
			}
		}
		return data.String()
	}
	b, err := New(&mocks.Client{}, Config{DatasetRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	valid := encode(records)
	if dataset, err := b.ImportDataset(t.Context(), ImportInput{Name: "facts.jsonl"}, strings.NewReader(valid)); err != nil || dataset.Records != 50 {
		t.Fatalf("valid fact contract: %+v %v", dataset, err)
	}
	allowed := slices.Clone(records)
	allowed[20].Evidence = "  " + strings.ToUpper(allowed[0].Evidence) + "\n"
	allowed[20].Target = "  " + strings.ToUpper(allowed[0].Target) + "\n"
	for i := range allowed {
		if allowed[i].FactID == "fact-01" {
			allowed[i].Evidence = allowed[0].Evidence // Same evidence, different target is different knowledge.
		}
	}
	if _, err := b.ImportDataset(t.Context(), ImportInput{Name: "normalized-facts.jsonl"}, strings.NewReader(encode(allowed))); err != nil {
		t.Fatalf("valid normalized facts rejected: %v", err)
	}
	for name, change := range map[string]func([]Record) []Record{
		"duplicate-prompt": func(rows []Record) []Record { rows[20].Prompt = rows[0].Prompt; return rows },
		"conflicting-fact": func(rows []Record) []Record { rows[20].Target = "Different answer."; return rows },
		"duplicate-knowledge": func(rows []Record) []Record {
			for i := range rows {
				if rows[i].FactID == "fact-01" {
					rows[i].Evidence = "  " + strings.ToUpper(rows[0].Evidence) + "\n"
					rows[i].Target = "  " + strings.ToUpper(rows[0].Target) + "\n"
				}
			}
			return rows
		},
		"approved-evaluation": func(rows []Record) []Record { rows[20].UserApproved = true; return rows },
		"mixed-fact-mode":     func(rows []Record) []Record { rows[0].FactID = ""; return rows },
		"missing-validation":  func(rows []Record) []Record { return append(rows[:20], rows[21:]...) },
		"missing-test":        func(rows []Record) []Record { return rows[:49] },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := b.ImportDataset(t.Context(), ImportInput{Name: name + ".jsonl"}, strings.NewReader(encode(change(slices.Clone(records))))); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("invalid fact contract accepted: %v", err)
			}
		})
	}
	first, rest, _ := strings.Cut(valid, "\n")
	for _, invalid := range []string{valid + "\n", first + " {}\n" + rest, strings.Replace(valid, `"prompt":`, `"unknown":true,"prompt":`, 1)} {
		if _, err := b.ImportDataset(t.Context(), ImportInput{Name: "invalid.jsonl"}, strings.NewReader(invalid)); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("invalid JSONL accepted: %v", err)
		}
	}
}
