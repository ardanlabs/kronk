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
	"github.com/ardanlabs/kronk/tools"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func Test_Embedding(t *testing.T) {
	testEmbedding(t, mpEmbed)
}

// =============================================================================

func testEmbedding(t *testing.T, mp tools.ModelPath) {
	if runInParallel {
		t.Parallel()
	}

	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile: mp.ModelFile,
	})

	if err != nil {
		t.Fatalf("unable to create inference model: %v", err)
	}
	defer func() {
		t.Logf("active streams: %d", krn.ActiveStreams())
		t.Log("unload Kronk")
		if err := krn.Unload(context.Background()); err != nil {
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
			name := strings.TrimSuffix(mp.ModelFile, path.Ext(mp.ModelFile))
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		embed, err := krn.Embeddings(ctx, text)
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

		if len(embed.Data) == 0 {
			return fmt.Errorf("unexpected data length: got %d", len(embed.Data))
		}

		if embed.Data[0].Object != "embedding" {
			return fmt.Errorf("unexpected data object: got %s, exp %s", embed.Data[0].Object, "embedding")
		}

		if embed.Data[0].Index != 0 {
			return fmt.Errorf("unexpected index: got %d", embed.Data[0].Index)
		}

		if embed.Data[0].Embedding[0] == 0 || embed.Data[0].Embedding[len(embed.Data[0].Embedding)-1] == 0 {
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
