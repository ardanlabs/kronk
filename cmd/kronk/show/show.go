// Package show provides the show command code.
package show

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/model"
)

// Run executes the pull command.
func Run(args []string) error {
	modelInstances := 1
	modelPath := defaults.ModelsDir()
	modelName := args[0]
	modelFile := filepath.Join(modelPath, modelName)

	libPath := defaults.LibsDir()

	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile: modelFile,
	})
	if err != nil {
		return fmt.Errorf("unable to load kronk: %w", err)
	}

	defer krn.Unload(context.Background())

	mi := krn.ModelInfo()

	fmt.Println()
	fmt.Printf("Name:        %s\n", mi.Name)
	fmt.Printf("Desc:        %s\n", mi.Desc)
	fmt.Printf("Size:        %.2f MiB\n", float64(mi.Size)/(1024*1024))
	fmt.Printf("HasEncoder:  %t\n", mi.HasEncoder)
	fmt.Printf("HasDecoder:  %t\n", mi.HasDecoder)
	fmt.Printf("IsRecurrent: %t\n", mi.IsRecurrent)
	fmt.Printf("IsHybrid:    %t\n", mi.IsHybrid)
	fmt.Printf("IsGPT:       %t\n", mi.IsGPT)
	fmt.Println("Metadata:")
	for k, v := range mi.Metadata {
		fmt.Printf("  %s: %s\n", k, v)
	}

	return nil
}
