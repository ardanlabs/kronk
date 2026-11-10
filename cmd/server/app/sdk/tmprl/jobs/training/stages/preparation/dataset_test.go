package preparation_test

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

func documentAddition(id, hash string, prompts ...string) preparation.DatasetAddition {
	addition := preparation.DatasetAddition{PublicationID: "run-" + id, Source: uploads.File{ID: id, Name: id + ".txt", SHA256: strings.Repeat(hash, 64)}}
	for _, prompt := range prompts {
		addition.Records = append(addition.Records, training.Record{Prompt: prompt, Target: "dawn", Evidence: "The harbor opens at dawn.", SourceDocument: id})
	}
	return addition
}

func TestDatasetVersionsAndDocumentRemoval(t *testing.T) {
	ctx := t.Context()
	path := t.TempDir()
	store, err := preparation.NewDatasetStore(path)
	if err != nil {
		t.Fatal(err)
	}
	dataset, err := store.Create(ctx, "Harbor notes")
	if err != nil {
		t.Fatal(err)
	}
	first := documentAddition("source-a", "a", "When does the harbor open?", "What is the opening time?")
	second := documentAddition("source-b", "b", "At what time does the ferry leave?")
	added, err := store.Add(ctx, dataset.ID, 1, first)
	if err != nil || added.Version != 2 || added.RecordCount != 2 {
		t.Fatalf("first addition: %+v, %v", added, err)
	}
	added, err = store.Add(ctx, dataset.ID, 2, second)
	if err != nil || added.Version != 3 || added.RecordCount != 3 {
		t.Fatalf("second addition: %+v, %v", added, err)
	}
	if _, err := store.Remove(ctx, dataset.ID, 2, first.PublicationID); !errors.Is(err, preparation.ErrDatasetConflict) {
		t.Fatalf("stale removal: %v", err)
	}
	removed, err := store.Remove(ctx, dataset.ID, 3, first.PublicationID)
	if err != nil || removed.Version != 4 || removed.RecordCount != 1 || len(removed.Documents) != 1 || removed.Documents[0].Name != "source-b.txt" {
		t.Fatalf("remove: %+v, %v", removed, err)
	}
	// Reopening the store proves persistence rather than an in-memory cache.
	store, err = preparation.NewDatasetStore(path)
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
	if _, err := store.Add(ctx, dataset.ID, 1, first); !errors.Is(err, preparation.ErrDatasetConflict) {
		t.Fatalf("changed idempotency payload accepted: %v", err)
	}
	duplicate := documentAddition("duplicate", "b", "Another question about the ferry?")
	if _, err := store.Add(ctx, dataset.ID, 4, duplicate); !errors.Is(err, preparation.ErrDatasetConflict) {
		t.Fatalf("duplicate document accepted: %v", err)
	}
	duplicate.Source.SHA256 = strings.Repeat("c", 64)
	duplicate.Records[0].Prompt = second.Records[0].Prompt
	if _, err := store.Add(ctx, dataset.ID, 4, duplicate); !errors.Is(err, preparation.ErrDatasetConflict) {
		t.Fatalf("duplicate example accepted: %v", err)
	}
	listed, err := store.List(ctx)
	if err != nil || len(listed) != 1 || listed[0].RecordCount != 1 {
		t.Fatalf("list: %+v, %v", listed, err)
	}
}

func TestDatasetConcurrentPublishers(t *testing.T) {
	path := t.TempDir()
	first, err := preparation.NewDatasetStore(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := preparation.NewDatasetStore(path)
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
	for i, store := range []*preparation.DatasetStore{first, second} {
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
	if _, err := first.Get(t.Context(), "../outside", 0); !errors.Is(err, preparation.ErrInvalidArgument) {
		t.Fatalf("unconfined identity accepted: %v", err)
	}
	if _, err := first.Get(t.Context(), dataset.ID, 42); !errors.Is(err, preparation.ErrDatasetNotFound) {
		t.Fatalf("missing version: %v", err)
	}
}
