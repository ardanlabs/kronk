// Package remove provides the remove command code.
package remove

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/install"
)

// Run executes the pull command.
func Run(args []string) error {
	modelPath := defaults.ModelsDir()
	modelName := args[0]

	fmt.Println("Model Path: ", modelPath)
	fmt.Println("Model Name: ", modelName)

	modelFile, err := install.FindModel(modelPath, modelName)
	if err != nil {
		return err
	}

	if err := os.Remove(modelFile); err != nil {
		return fmt.Errorf("unable to remove %q", modelFile)
	}

	// This file may not exist, so deleting it blindly.
	modelFileName := filepath.Base(modelFile)
	projFileName := fmt.Sprintf("mmproj-%s", modelFileName)
	projFile := strings.Replace(modelFile, modelFileName, projFileName, 1)
	os.Remove(projFile)

	fmt.Println("Remove complete")
	return nil
}
