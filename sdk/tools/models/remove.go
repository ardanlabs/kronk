package models

import (
	"fmt"
	"os"
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

	return nil
}
