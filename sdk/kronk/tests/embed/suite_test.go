package embed_test

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
	if len(testlib.MPEmbedBatchSeq.ModelFiles) == 0 {
		t.Skip("model not downloaded")
	}

	testlib.WithModel(t, testlib.CfgEmbedBatchSeq(), func(t *testing.T, krn *kronk.Kronk) {
		t.Run("Embedding", func(t *testing.T) { testEmbedding(t, krn) })
		t.Run("ConcurrentEmbeddings", func(t *testing.T) { testlib.TestEmbeddingIntegrity(t, krn.ModelInfo().ID, krn.Embeddings) })
	})
}

func testEmbedding(t *testing.T, krn *kronk.Kronk) {
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

		return testlib.CheckEmbeddings(embed, krn.ModelInfo().ID, len(inputs), 768)
	}

	var g errgroup.Group
	for range testlib.Goroutines {
		g.Go(testlib.WithRetry(t, f))
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}
