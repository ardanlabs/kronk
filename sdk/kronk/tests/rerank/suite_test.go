package rerank_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func TestSuite(t *testing.T) {
	testlib.WithModel(t, testlib.CfgRerank(), func(t *testing.T, krn *kronk.Kronk) {
		t.Run("Rerank", func(t *testing.T) { testRerank(t, krn) })
	})
}

func testRerank(t *testing.T, krn *kronk.Kronk) {
	if testlib.RunInParallel {
		t.Parallel()
	}

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

		if rerank.Object != "list" {
			return fmt.Errorf("unexpected object: got %s, exp %s", rerank.Object, "list")
		}

		if rerank.Model != krn.ModelInfo().ID {
			return fmt.Errorf("unexpected model: got %s, exp %s", rerank.Model, krn.ModelInfo().ID)
		}

		if rerank.Created == 0 {
			return fmt.Errorf("unexpected created: got %d", rerank.Created)
		}

		if len(rerank.Data) == 0 {
			return fmt.Errorf("unexpected data length: got %d", len(rerank.Data))
		}

		if len(rerank.Data) > 3 {
			return fmt.Errorf("expected top_n=3 to limit results: got %d", len(rerank.Data))
		}

		for i := 1; i < len(rerank.Data); i++ {
			if rerank.Data[i].RelevanceScore > rerank.Data[i-1].RelevanceScore {
				return fmt.Errorf("results not sorted by relevance: index %d (%.4f) > index %d (%.4f)",
					i, rerank.Data[i].RelevanceScore, i-1, rerank.Data[i-1].RelevanceScore)
			}
		}

		for i, result := range rerank.Data {
			if result.RelevanceScore < 0 || result.RelevanceScore > 1 {
				return fmt.Errorf("score out of range [0,1]: index %d, score %.4f", i, result.RelevanceScore)
			}
		}

		for i, result := range rerank.Data {
			if result.Document == "" {
				return fmt.Errorf("expected document to be returned: index %d", i)
			}
		}

		topResult := rerank.Data[0]
		if !strings.Contains(strings.ToLower(topResult.Document), "paris") &&
			!strings.Contains(strings.ToLower(topResult.Document), "france") {
			return fmt.Errorf("expected top result to be about Paris/France, got: %s", topResult.Document)
		}

		if rerank.Usage.PromptTokens == 0 {
			return fmt.Errorf("expected prompt tokens to be non-zero")
		}

		if rerank.Usage.TotalTokens == 0 {
			return fmt.Errorf("expected total tokens to be non-zero")
		}

		return nil
	}

	var g errgroup.Group
	for range testlib.Goroutines {
		g.Go(f)
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}
