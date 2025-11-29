package kronk_test

import (
	"context"
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/model"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func Test_Embedding(t *testing.T) {
	testEmbedding(t, modelEmbedFile)
}

// =============================================================================

func testEmbedding(t *testing.T, modelFile string) {
	if runInParallel {
		t.Parallel()
	}

	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile:  modelFile,
		Embeddings: true,
	})

	if err != nil {
		t.Fatalf("unable to create inference model: %v", err)
	}
	defer func() {
		t.Logf("active streams: %d", krn.ActiveStreams())
		t.Log("unload Kronk")
		if err := krn.Unload(); err != nil {
			t.Errorf("failed to unload model: %v", err)
		}
	}()

	// -------------------------------------------------------------------------

	text := "Embed this sentence"

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			name := strings.TrimSuffix(modelFile, path.Ext(modelFile))
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		embed, err := krn.Embed(ctx, text)
		if err != nil {
			return fmt.Errorf("embed: %w", err)
		}

		if embed[0] == 0 || embed[len(embed)-1] == 0 {
			return fmt.Errorf("expected to have values in the embedding")
		}

		return nil
	}

	var g errgroup.Group
	for range goroutines {
		g.Go(f)
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}
