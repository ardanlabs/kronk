// This example shows you how to use an embedding model.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-embedding

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSpec defines how to obtain the model to download.
// - SourceURL: Download the model file directly from a HuggingFace URL
// - ModelID  : Download the model from the catalog by model ID
//
// To use a catalog model, comment out SourceURL and set ModelID.
// To use a direct URL, comment out ModelID and set SourceURL.
type modelSpec struct {
	SourceURL string
	ModelID   string
}

// Configure this to switch between URL and catalog downloads.
// Set either SourceURL or ModelID, not both.
var modelSpecConfig = modelSpec{
	SourceURL: "https://huggingface.co/ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/resolve/main/embeddinggemma-300m-qat-Q8_0.gguf",
	// ModelID: "embeddinggemma-300m-qat-Q8_0",
}

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to installation system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	if err := embedding(krn); err != nil {
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

	ctlg, err := catalog.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create catalog system: %w", err)
	}

	if err := ctlg.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// Download model based on spec config using switch/case
	var mp models.Path

	switch {
	case modelSpecConfig.SourceURL != "":
		fmt.Println("Downloading model from URL:", modelSpecConfig.SourceURL)
		mp, err = mdls.Download(ctx, kronk.FmtLogger, modelSpecConfig.SourceURL, "")

	case modelSpecConfig.ModelID != "":
		fmt.Println("Downloading model from catalog:", modelSpecConfig.ModelID)
		mp, err = ctlg.DownloadModel(ctx, kronk.FmtLogger, modelSpecConfig.ModelID)

	default:
		return models.Path{}, fmt.Errorf("modelSpecConfig requires either SourceURL or ModelID to be set")
	}

	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	fmt.Println("loading model...")

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	cfg := model.Config{
		ModelFiles: mp.ModelFiles,
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
	fmt.Println("- modelType    :", krn.ModelInfo().Type)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)

	return krn, nil
}

func embedding(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	d := model.D{
		"input":              "Why is the sky blue?",
		"truncate":           true,
		"truncate_direction": "right",
	}

	resp, err := krn.Embeddings(ctx, d)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Model  :", resp.Model)
	fmt.Println("Object :", resp.Object)
	fmt.Println("Created:", time.UnixMilli(resp.Created))
	fmt.Println("  Index    :", resp.Data[0].Index)
	fmt.Println("  Object   :", resp.Data[0].Object)
	fmt.Printf("  Embedding: [%v...%v]\n", resp.Data[0].Embedding[:3], resp.Data[0].Embedding[len(resp.Data[0].Embedding)-3:])

	return nil
}
