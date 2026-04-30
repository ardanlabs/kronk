// This example shows you how to use the pool package to manage multiple
// models in memory at the same time. The pool will load models on demand,
// keep them resident up to a configured cap, and unload them after a TTL
// of inactivity.
//
// The first time you run this program the system will download and install
// the models and libraries.
//
// Run the example like this from the root of the project:
// $ make example-pool

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/pool"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
	questionModel = "unsloth/Qwen3-0.6B-Q8_0"
	visionModel   = "unsloth/Qwen3.5-0.8B-Q8_0"
	imageFile     = "samples/giraffe.jpg"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	if err := installSystem(); err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	if err := kronk.Init(); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	// -------------------------------------------------------------------------

	const cacheTTL = 15 * time.Second

	cfg := pool.Config{
		Log:           kronk.FmtLogger,
		BudgetPercent: 80,
		CacheTTL:      cacheTTL,
	}

	p, err := pool.New(cfg)
	if err != nil {
		return fmt.Errorf("unable to create pool: %w", err)
	}

	defer func() {
		fmt.Println("\nShutting down pool")
		if err := p.Shutdown(context.Background()); err != nil {
			fmt.Printf("failed to shutdown pool: %v\n", err)
		}
	}()

	// -------------------------------------------------------------------------

	if err := acquireAndAsk(p); err != nil {
		return fmt.Errorf("acquire and ask: %w", err)
	}

	printStatus(p, "after question model")

	if err := acquireAndSee(p); err != nil {
		return fmt.Errorf("acquire and see: %w", err)
	}

	printStatus(p, "after vision model")

	// -------------------------------------------------------------------------

	wait := cacheTTL + 5*time.Second
	fmt.Printf("\nWaiting %s for TTL to expire...\n", wait)
	time.Sleep(wait)

	printStatus(p, "after TTL expiry")

	return nil
}

func installSystem() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return fmt.Errorf("unable to create models system: %w", err)
	}

	for _, src := range []string{questionModel, visionModel} {
		fmt.Println("Downloading model:", src)
		if _, err := mdls.Download(ctx, kronk.FmtLogger, src); err != nil {
			return fmt.Errorf("unable to install model %q: %w", src, err)
		}
	}

	return nil
}

func acquireAndAsk(p *pool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	fmt.Println("\nAcquiring question model:", questionModel)

	krn, err := p.AquireModel(ctx, questionModel)
	if err != nil {
		return fmt.Errorf("acquire model: %w", err)
	}

	question := "Hello model"

	fmt.Println()
	fmt.Println("QUESTION:", question)
	fmt.Println()

	d := model.D{
		"messages": model.DocumentArray(
			model.TextMessage(model.RoleUser, question),
		),
		"temperature": 0.7,
		"top_p":       0.9,
		"top_k":       40,
		"max_tokens":  2048,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	return streamResponse(ch)
}

func acquireAndSee(p *pool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	fmt.Println("\nAcquiring vision model:", visionModel)

	krn, err := p.AquireModel(ctx, visionModel)
	if err != nil {
		return fmt.Errorf("acquire model: %w", err)
	}

	image, err := readImage(imageFile)
	if err != nil {
		return fmt.Errorf("read image: %w", err)
	}

	question := "What is in this picture?"

	fmt.Printf("\nQuestion: %s\n", question)

	d := model.D{
		"messages":    model.RawMediaMessage(question, image),
		"temperature": 0.7,
		"top_p":       0.9,
		"top_k":       40,
		"max_tokens":  2048,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("vision streaming: %w", err)
	}

	return streamResponse(ch)
}

func streamResponse(ch <-chan model.ChatResponse) error {
	fmt.Print("\nMODEL> ")

	var reasoning bool

	for resp := range ch {
		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)

		case model.FinishReasonStop:
			fmt.Println()
			return nil

		default:
			if resp.Choices[0].Delta.Reasoning != "" {
				reasoning = true
				fmt.Printf("\u001b[91m%s\u001b[0m", resp.Choices[0].Delta.Reasoning)
				continue
			}

			if reasoning {
				reasoning = false
				fmt.Println()
				continue
			}

			fmt.Printf("%s", resp.Choices[0].Delta.Content)
		}
	}

	return nil
}

func printStatus(p *pool.Pool, label string) {
	details, err := p.ModelStatus()
	if err != nil {
		fmt.Printf("\nModelStatus error: %v\n", err)
		return
	}

	fmt.Printf("\n--- pool status (%s) ---\n", label)
	fmt.Printf("models in cache: %d\n", len(details))
	for _, d := range details {
		fmt.Printf("  - id=%s family=%s vram=%dMiB slots=%d active=%d expires=%s\n",
			d.ID,
			d.ModelFamily,
			d.VRAMTotal/(1024*1024),
			d.Slots,
			d.ActiveStreams,
			d.ExpiresAt.Format(time.RFC3339),
		)
	}
	fmt.Println("------------------------")
}

func readImage(imageFile string) ([]byte, error) {
	if _, err := os.Stat(imageFile); err != nil {
		return nil, fmt.Errorf("error accessing file %q: %w", imageFile, err)
	}

	image, err := os.ReadFile(imageFile)
	if err != nil {
		return nil, fmt.Errorf("error reading file %q: %w", imageFile, err)
	}

	return image, nil
}
