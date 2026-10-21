package rerank_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"uuid"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
	"golang.org/x/sync/errgroup"
)

func TestSuite(t *testing.T) {
	if len(testlib.MPRerankBatchSeq.ModelFiles) == 0 {
		t.Skip("model not downloaded")
	}

	testlib.WithModel(t, testlib.CfgRerankBatchSeq(), func(t *testing.T, krn *kronk.Kronk) {
		t.Run("Rerank", func(t *testing.T) { testRerank(t, krn) })
		t.Run("ConcurrentRerank", func(t *testing.T) { testlib.TestRerankIntegrity(t, krn.ModelInfo().ID, krn.Rerank) })
	})
}

func testRerank(t *testing.T, krn *kronk.Kronk) {
	query := "What is the capital of France?"
	documents := []string{
		"Paris is the capital and largest city of France.",
		"Berlin is the capital of Germany.",
		"The Eiffel Tower is located in Paris.",
		"London is the capital of England.",
		"France is a country in Western Europe.",
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		rerank, err := krn.Rerank(ctx, model.D{
			"query":            query,
			"documents":        documents,
			"top_n":            3,
			"return_documents": true,
		})
		if err != nil {
			return fmt.Errorf("rerank: %w", err)
		}

		if err := testlib.CheckRerank(rerank, krn.ModelInfo().ID, documents, 3, true); err != nil {
			return err
		}
		if rerank.Data[0].Index != 0 {
			return fmt.Errorf("top result: got source index %d, want Paris/France at index 0", rerank.Data[0].Index)
		}

		return nil
	}

	var g errgroup.Group
	for range testlib.Goroutines {
		g.Go(testlib.WithRetry(t, f))
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}
