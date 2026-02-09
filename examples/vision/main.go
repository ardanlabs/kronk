// This example shows you how to execute a simple prompt against a vision model.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-vision

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

const (
	modelURL  = "ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	projURL   = "ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	imageFile = "examples/samples/giraffe.jpg"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	info, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	krn, err := newKronk(info)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	if err := vision(krn); err != nil {
		return err
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// -------------------------------------------------------------------------

	templates, err := templates.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := templates.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelURL, projURL)
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	// -------------------------------------------------------------------------
	// You could also download this model using the catalog system.
	// templates.Catalog().DownloadModel("Qwen2.5-VL-3B-Instruct-Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	fmt.Println("loading model...")

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	cfg := model.Config{
		ModelFiles: mp.ModelFiles,
		ProjFile:   mp.ProjFile,
		CacheTypeK: model.GGMLTypeQ8_0,
		CacheTypeV: model.GGMLTypeQ8_0,
	}

	krn, err := kronk.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\n\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Printf("- k/v          : %s/%s\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- nBatch       :", krn.ModelConfig().NBatch)
	fmt.Println("- nuBatch      :", krn.ModelConfig().NUBatch)
	fmt.Println("- embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)

	return krn, nil
}

func vision(krn *kronk.Kronk) error {
	question := "What is in this picture?"

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	ch, err := performChat(ctx, krn, question, imageFile)
	if err != nil {
		return fmt.Errorf("perform chat: %w", err)
	}

	if err := modelResponse(krn, ch); err != nil {
		return fmt.Errorf("model response: %w", err)
	}

	return nil
}

func performChat(ctx context.Context, krn *kronk.Kronk, question string, imageFile string) (<-chan model.ChatResponse, error) {
	image, err := readImage(imageFile)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}

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
		return nil, fmt.Errorf("vision streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, ch <-chan model.ChatResponse) error {
	fmt.Print("\nMODEL> ")

	var reasoning bool
	var lr model.ChatResponse

loop:
	for resp := range ch {
		lr = resp

		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonStop:
			break loop

		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)
		}

		if resp.Choice[0].Delta.Reasoning != "" {
			fmt.Printf("\u001b[91m%s\u001b[0m", resp.Choice[0].Delta.Reasoning)
			reasoning = true
			continue
		}

		if reasoning {
			reasoning = false
			fmt.Print("\n\n")
		}

		fmt.Printf("%s", resp.Choice[0].Delta.Content)
	}

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\n\n\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\u001b[0m\n",
		lr.Usage.PromptTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return nil
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
