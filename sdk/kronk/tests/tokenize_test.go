package kronk_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func testTokenize(t *testing.T, krn *kronk.Kronk) {
	if runInParallel {
		t.Parallel()
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		input := "The quick brown fox jumps over the lazy dog"

		tok, err := krn.Tokenize(ctx, model.D{"input": input})
		if err != nil {
			return fmt.Errorf("tokenize: %w", err)
		}

		if tok.Object != "tokenize" {
			return fmt.Errorf("unexpected object: got %s, exp %s", tok.Object, "tokenize")
		}

		if tok.Model != krn.ModelInfo().ID {
			return fmt.Errorf("unexpected model: got %s, exp %s", tok.Model, krn.ModelInfo().ID)
		}

		if tok.Created == 0 {
			return fmt.Errorf("unexpected created: got %d", tok.Created)
		}

		if tok.Tokens == 0 {
			return fmt.Errorf("expected non-zero token count for input %q", input)
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

func testTokenizeWithTemplate(t *testing.T, krn *kronk.Kronk) {
	if runInParallel {
		t.Parallel()
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		input := "The quick brown fox jumps over the lazy dog"

		tokRaw, err := krn.Tokenize(ctx, model.D{"input": input})
		if err != nil {
			return fmt.Errorf("tokenize raw: %w", err)
		}

		tokTmpl, err := krn.Tokenize(ctx, model.D{
			"input":          input,
			"apply_template": true,
		})
		if err != nil {
			return fmt.Errorf("tokenize with template: %w", err)
		}

		if tokTmpl.Object != "tokenize" {
			return fmt.Errorf("unexpected object: got %s, exp %s", tokTmpl.Object, "tokenize")
		}

		if tokTmpl.Model != krn.ModelInfo().ID {
			return fmt.Errorf("unexpected model: got %s, exp %s", tokTmpl.Model, krn.ModelInfo().ID)
		}

		if tokTmpl.Created == 0 {
			return fmt.Errorf("unexpected created: got %d", tokTmpl.Created)
		}

		if tokTmpl.Tokens == 0 {
			return fmt.Errorf("expected non-zero token count with template for input %q", input)
		}

		if tokTmpl.Tokens <= tokRaw.Tokens {
			return fmt.Errorf("expected template token count (%d) to be greater than raw token count (%d) due to template overhead", tokTmpl.Tokens, tokRaw.Tokens)
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
