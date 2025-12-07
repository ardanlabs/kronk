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
	"runtime"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/model"
	"github.com/ardanlabs/kronk/tools"
	"github.com/hybridgroup/yzma/pkg/download"
)

const (
	modelURL       = "https://huggingface.co/ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/resolve/main/embeddinggemma-300m-qat-Q8_0.gguf"
	modelInstances = 1
)

var (
	libPath   = defaults.LibsDir("")
	modelPath = defaults.ModelsDir("")
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
		return fmt.Errorf("unable to installation system: %w", err)
	}

	krn, err := newKronk(libPath, info)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}
	defer func() {
		fmt.Println("\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	// -------------------------------------------------------------------------

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	question := "Why is the sky blue?"

	queryVector, err := krn.Embed(ctx, question)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}

	fmt.Println()
	fmt.Printf("Query Vector: [%v...%v]\n", queryVector[:3], queryVector[len(queryVector)-3:])

	return nil
}

func installSystem() (tools.ModelPath, error) {
	libCfg, err := tools.NewLibConfig(
		libPath,
		runtime.GOARCH,
		runtime.GOOS,
		download.CPU.String(),
		kronk.LogSilent.Int(),
		true,
	)
	if err != nil {
		return tools.ModelPath{}, err
	}

	_, err = tools.DownloadLibraries(context.Background(), tools.FmtLogger, libCfg)
	if err != nil {
		return tools.ModelPath{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	info, err := tools.DownloadModel(context.Background(), tools.FmtLogger, modelURL, "", modelPath)
	if err != nil {
		return tools.ModelPath{}, fmt.Errorf("unable to install model: %w", err)
	}

	return info, nil
}

func newKronk(libPath string, mp tools.ModelPath) (*kronk.Kronk, error) {
	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile:  mp.ModelFile,
		Embeddings: true,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\n\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("  - contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("  - embeddings   :", krn.ModelConfig().Embeddings)
	fmt.Println("  - isGPT        :", krn.ModelInfo().IsGPT)

	return krn, nil
}
