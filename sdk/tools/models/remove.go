package models

import (
	"fmt"
	"os"
	"path/filepath"
)

// Remove remove the specified file from the models directory.
func Remove(modelBasePath string, mp Path) (err error) {
	defer func() {
		if errDfr := BuildIndex(modelBasePath); err != nil {
			err = errDfr
		}
	}()

	if err := os.Remove(mp.ModelFile); err != nil {
		return fmt.Errorf("remove-model: unable to remove model: %q", mp.ModelFile)
	}

	if mp.ProjFile != "" {
		if err := os.Remove(mp.ProjFile); err != nil {
			return fmt.Errorf("remove-model: unable to remove mmproj: %q", mp.ModelFile)
		}
	}

	// Remove empty parent directories up to modelBasePath
	dir := filepath.Dir(mp.ModelFile)
	for dir != modelBasePath && dir != "." && dir != "/" {
		if err := os.Remove(dir); err != nil {
			break // Directory not empty or other error, stop
		}
		dir = filepath.Dir(dir)
	}

	return nil
}
