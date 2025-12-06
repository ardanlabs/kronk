package tools

import (
	"fmt"
	"os"
)

// RemoveModel remove the specified file from the models directory.
func RemoveModel(mp ModelPath) error {
	if err := os.Remove(mp.ModelFile); err != nil {
		return fmt.Errorf("unable to remove model: %q", mp.ModelFile)
	}

	if mp.ProjFile != "" {
		if err := os.Remove(mp.ProjFile); err != nil {
			return fmt.Errorf("unable to remove mmproj: %q", mp.ModelFile)
		}
	}

	return nil
}
