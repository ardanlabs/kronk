// Package remove provides the remove command code.
package remove

import (
	"fmt"

	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/tools"
)

// RunLocal executes the pull command.
func RunLocal(args []string) error {
	modelPath := defaults.ModelsDir("")
	modelName := args[0]

	fmt.Println("Model Path: ", modelPath)
	fmt.Println("Model Name: ", modelName)

	mp, err := tools.FindModel(modelPath, modelName)
	if err != nil {
		return err
	}

	fmt.Printf("\nAre you sure you want to remove %q? (y/n): ", modelName)

	var response string
	fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		fmt.Println("Remove cancelled")
		return nil
	}

	if err := tools.RemoveModel(mp); err != nil {
		return fmt.Errorf("failed to remove model: %w", err)
	}

	fmt.Println("Remove complete")

	return nil
}
