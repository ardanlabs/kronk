package embed_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func TestSuite(t *testing.T) {
	testlib.WithModel(t, testlib.CfgEmbed(), func(t *testing.T, krn *kronk.Kronk) {
		t.Run("Embedding", func(t *testing.T) { testEmbedding(t, krn) })
	})
}

func testEmbedding(t *testing.T, krn *kronk.Kronk) {
	if testlib.RunInParallel {
		t.Parallel()
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

		inputs := []string{
			"The quick brown fox jumps over the lazy dog",
			"Machine learning is a subset of artificial intelligence",
			"Go is a statically typed programming language",
			"Embeddings convert text into numerical vectors",
		}

		embed, err := krn.Embeddings(ctx, model.D{"input": inputs})
		if err != nil {
			return fmt.Errorf("embed: %w", err)
		}

		if embed.Object != "list" {
			return fmt.Errorf("unexpected object: got %s, exp %s", embed.Object, "list")
		}

		if embed.Model != krn.ModelInfo().ID {
			return fmt.Errorf("unexpected model: got %s, exp %s", embed.Model, krn.ModelInfo().ID)
		}

		if embed.Created == 0 {
			return fmt.Errorf("unexpected created: got %d", embed.Created)
		}

		if len(embed.Data) != len(inputs) {
			return fmt.Errorf("unexpected data length: got %d, exp %d", len(embed.Data), len(inputs))
		}

		for i, data := range embed.Data {
			if data.Object != "embedding" {
				return fmt.Errorf("data[%d]: unexpected object: got %s, exp %s", i, data.Object, "embedding")
			}

			if data.Index != i {
				return fmt.Errorf("data[%d]: unexpected index: got %d", i, data.Index)
			}

			if len(data.Embedding) == 0 {
				return fmt.Errorf("data[%d]: empty embedding", i)
			}

			if data.Embedding[0] == 0 && data.Embedding[len(data.Embedding)-1] == 0 {
				return fmt.Errorf("data[%d]: expected to have values in the embedding", i)
			}
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
