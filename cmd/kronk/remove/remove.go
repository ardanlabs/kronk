// Package remove provides the remove command code.
package remove

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/tools"
)

// Run executes the pull command.
func Run(args []string) error {
	modelPath := defaults.ModelsDir()
	modelName := args[0]

	fmt.Println("Model Path: ", modelPath)
	fmt.Println("Model Name: ", modelName)

	fi, err := tools.FindModel(modelPath, modelName)
	if err != nil {
		return err
	}

	modelFileName := filepath.Base(fi.ModelFile)

	fmt.Printf("\nAre you sure you want to remove %q? (y/n): ", modelFileName)
	var response string
	fmt.Scanln(&response)
	if response != "y" && response != "Y" {
		fmt.Println("Remove cancelled")
		return nil
	}

	if err := os.Remove(fi.ModelFile); err != nil {
		return fmt.Errorf("unable to remove %q", fi.ModelFile)
	}

	// This file may not exist, so deleting it blindly.
	projFileName := fmt.Sprintf("mmproj-%s", modelFileName)
	projFile := strings.Replace(fi.ModelFile, modelFileName, projFileName, 1)
	os.Remove(projFile)

	fmt.Println("Remove complete")
	return nil
}
