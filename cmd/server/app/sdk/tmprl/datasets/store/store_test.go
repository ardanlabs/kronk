package store_test

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	datasets "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

func documentAddition(id, hash string, prompts ...string) datasets.Addition {
	addition := datasets.Addition{PublicationID: "run-" + id, Source: uploads.File{ID: id, Name: id + ".txt", SHA256: strings.Repeat(hash, 64)}}
	for _, prompt := range prompts {
		addition.Records = append(addition.Records, training.Record{Prompt: prompt, Target: "dawn", Evidence: "The harbor opens at dawn.", SourceDocument: id})
	}
	return addition
}

func TestDatasetVersionsAndDocumentRemoval(t *testing.T) {
	ctx := t.Context()
	path := t.TempDir()
	store, err := datasets.New(datasets.Config{Root: path})
	if err != nil {
		t.Fatal(err)
	}
	dataset, err := store.Create(ctx, "Harbor notes")
	if err != nil {
		t.Fatal(err)
	}
	first := documentAddition("source-a", "a", "When does the harbor open?", "What is the opening time?")
	second := documentAddition("source-b", "b", "At what time does the ferry leave?")
	invalid := first
	invalid.Source.SHA256 = "not-a-source-hash"
	if _, err := store.Add(ctx, dataset.ID, 1, invalid); !errors.Is(err, tmprl.ErrInvalidArgument) {
		t.Fatalf("invalid source hash: %v", err)
	}
	invalid = first
	invalid.PageEnd = 2
	if _, err := store.Add(ctx, dataset.ID, 1, invalid); !errors.Is(err, tmprl.ErrInvalidArgument) {
		t.Fatalf("unpaired page range: %v", err)
	}
	added, err := store.Add(ctx, dataset.ID, 1, first)
	if err != nil || added.Version != 2 || added.RecordCount != 2 {
		t.Fatalf("first addition: %+v, %v", added, err)
	}
	added, err = store.Add(ctx, dataset.ID, 2, second)
	if err != nil || added.Version != 3 || added.RecordCount != 3 {
		t.Fatalf("second addition: %+v, %v", added, err)
	}
	if _, err := store.Remove(ctx, dataset.ID, 2, first.PublicationID); !errors.Is(err, datasets.ErrDatasetConflict) {
		t.Fatalf("stale removal: %v", err)
	}
	removed, err := store.Remove(ctx, dataset.ID, 3, first.PublicationID)
	if err != nil || removed.Version != 4 || removed.RecordCount != 1 || len(removed.Documents) != 1 || removed.Documents[0].Name != "source-b.txt" {
		t.Fatalf("remove: %+v, %v", removed, err)
	}
	// Reopening the store proves persistence rather than an in-memory cache.
	store, err = datasets.New(datasets.Config{Root: path})
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.Records(ctx, dataset.ID, 0)
	if err != nil || !reflect.DeepEqual(current, second.Records) {
		t.Fatalf("remaining records: %+v, %v", current, err)
	}
	previous, err := store.Records(ctx, dataset.ID, 3)
	if err != nil || !reflect.DeepEqual(previous, append(first.Records, second.Records...)) {
		t.Fatalf("historical records changed: %+v, %v", previous, err)
	}
	// Repeated publication after removal must acknowledge the old version, not resurrect the document.
	replayed, err := store.Add(ctx, dataset.ID, 1, first)
	if err != nil || replayed.Version != 2 {
		t.Fatalf("publication replay: %+v, %v", replayed, err)
	}
	latest, err := store.Get(ctx, dataset.ID, 0)
	if err != nil || latest.Version != 4 || latest.RecordCount != 1 || !reflect.DeepEqual(latest.Versions, []int{1, 2, 3, 4}) {
		t.Fatalf("latest: %+v, %v", latest, err)
	}
	first.Records[0].Prompt = "A conflicting change to accepted input?"
	if _, err := store.Add(ctx, dataset.ID, 1, first); !errors.Is(err, datasets.ErrDatasetConflict) {
		t.Fatalf("changed idempotency payload accepted: %v", err)
	}
	info, err := store.Inspect(ctx, dataset.ID, 4)
	if err != nil || !info.HasDocument(second.Source.SHA256, 0, 0) || info.HasDocument(second.Source.SHA256, 1, 2) || info.HasDocument(first.Source.SHA256, 0, 0) {
		t.Fatalf("HasDocument must match only a present source and exact page range: %+v, %v", info.Documents, err)
	}
	duplicate := documentAddition("duplicate", "b", "Another question about the ferry?")
	if _, err := store.Add(ctx, dataset.ID, 4, duplicate); !errors.Is(err, datasets.ErrDocumentExists) || !errors.Is(err, datasets.ErrDatasetConflict) {
		t.Fatalf("duplicate document accepted: %v", err)
	}
	duplicate.Source.SHA256 = strings.Repeat("c", 64)
	duplicate.Records[0].Prompt = second.Records[0].Prompt
	if _, err := store.Add(ctx, dataset.ID, 4, duplicate); !errors.Is(err, datasets.ErrQuestionsExist) || !errors.Is(err, datasets.ErrDatasetConflict) {
		t.Fatalf("all-repeated document accepted: %v", err)
	}
	// Repeats are compared case- and space-insensitively and skipped; new questions are saved.
	partial := documentAddition("partial", "d", "  at WHAT time does the ferry leave? ", "Where is the fuel dock?")
	saved, err := store.Add(ctx, dataset.ID, 4, partial)
	if err != nil || saved.Version != 5 || saved.RecordCount != 2 || saved.Documents[1].ID != partial.PublicationID || saved.Documents[1].RecordCount != 1 {
		t.Fatalf("partial repeat: %+v, %v", saved, err)
	}
	current, err = store.Records(ctx, dataset.ID, 5)
	if err != nil || len(current) != 2 || current[1].Prompt != "Where is the fuel dock?" {
		t.Fatalf("saved records after skipping a repeat: %+v, %v", current, err)
	}
	if replayed, err := store.Add(ctx, dataset.ID, 4, partial); err != nil || replayed.Version != 5 {
		t.Fatalf("replay of a partially skipped publication: %+v, %v", replayed, err)
	}
	listed, err := store.List(ctx)
	if err != nil || len(listed) != 1 || listed[0].RecordCount != 2 {
		t.Fatalf("list: %+v, %v", listed, err)
	}
}

// Evaluation questions can reword another document's records; each record keeps
// one question per kind, and questions leave with the record they reword.
func TestDatasetEvaluationQuestionsFollowTheirRecords(t *testing.T) {
	ctx := t.Context()
	store, err := datasets.New(datasets.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	dataset, err := store.Create(ctx, "Harbor notes")
	if err != nil {
		t.Fatal(err)
	}
	fact := func(source, prompt, evidence string) training.Record {
		return training.Record{Prompt: prompt, Target: evidence, Evidence: evidence, FactID: training.FactID(evidence, evidence), SourceDocument: source}
	}
	first := documentAddition("source-a", "a")
	first.Records = []training.Record{fact("source-a", "When does the harbor open?", "The harbor opens at dawn.")}
	second := documentAddition("source-b", "b")
	question := fact("source-b", "At what hour does the harbor open?", "The harbor opens at dawn.")
	question.ValidationPrompt = true
	repeat := question
	repeat.Prompt = "What time is the harbor opened?"
	second.Records = []training.Record{fact("source-b", "When does the ferry leave?", "The ferry leaves at noon."), question, repeat}
	if _, err := store.Add(ctx, dataset.ID, 1, first); err != nil {
		t.Fatal(err)
	}
	added, err := store.Add(ctx, dataset.ID, 2, second)
	if err != nil || added.RecordCount != 3 {
		t.Fatalf("second document: %+v, %v", added, err)
	}
	if records, err := store.Records(ctx, dataset.ID, 3); err != nil || len(records) != 3 || records[2] != question {
		t.Fatalf("one validation question per record: %+v, %v", records, err)
	}
	removed, err := store.Remove(ctx, dataset.ID, 3, first.PublicationID)
	if err != nil || removed.RecordCount != 1 {
		t.Fatalf("removing the reworded record: %+v, %v", removed, err)
	}
	if records, err := store.Records(ctx, dataset.ID, 4); err != nil || len(records) != 1 || records[0].ValidationPrompt {
		t.Fatalf("question outlived its record: %+v, %v", records, err)
	}
	if _, err := store.Inspect(ctx, dataset.ID, 4); err != nil {
		t.Fatalf("filtered version failed its index check: %v", err)
	}
}

func TestDatasetConcurrentPublishers(t *testing.T) {
	path := t.TempDir()
	first, err := datasets.New(datasets.Config{Root: path})
	if err != nil {
		t.Fatal(err)
	}
	second, err := datasets.New(datasets.Config{Root: path})
	if err != nil {
		t.Fatal(err)
	}
	dataset, err := first.Create(t.Context(), "Concurrent notes")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	failures := make(chan error, 2)
	var group sync.WaitGroup
	for i, store := range []*datasets.Store{first, second} {
		group.Go(func() {
			<-start
			addition := documentAddition("source-a", "a", "When does the harbor open?")
			if i == 1 {
				addition = documentAddition("source-b", "b", "When does the ferry depart?")
			}
			_, err := store.Add(t.Context(), dataset.ID, 1, addition)
			failures <- err
		})
	}
	close(start)
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	latest, err := first.Get(t.Context(), dataset.ID, 0)
	if err != nil || latest.Version != 3 || latest.RecordCount != 2 || len(latest.Documents) != 2 {
		t.Fatalf("lost update: %+v, %v", latest, err)
	}
	if _, err := first.Get(t.Context(), "../outside", 0); !errors.Is(err, tmprl.ErrInvalidArgument) {
		t.Fatalf("unconfined identity accepted: %v", err)
	}
	if _, err := first.Get(t.Context(), dataset.ID, 42); !errors.Is(err, datasets.ErrDatasetNotFound) {
		t.Fatalf("missing version: %v", err)
	}
}
