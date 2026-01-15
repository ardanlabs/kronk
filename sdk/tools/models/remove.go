package models

import (
	"fmt"
	"os"
	"path/filepath"
)

// Remove remove the specified file from the models directory.
func (m *Models) Remove(mp Path, log Logger) (err error) {
	defer func() {
		if errDfr := m.BuildIndex(log); err != nil {
			err = errDfr
		}
	}()

	for _, modelFile := range mp.ModelFiles {
		if err := os.Remove(modelFile); err != nil {
			return fmt.Errorf("remove: unable to remove model: %q", modelFile)
		}

		dir := filepath.Dir(modelFile)
		base := filepath.Base(modelFile)
		shaFile := filepath.Join(dir, "sha", base)

		if err := os.Remove(shaFile); err != nil {
			return fmt.Errorf("remove: unable to remove model: %q", shaFile)
		}
	}

	if mp.ProjFile != "" {
		if err := os.Remove(mp.ProjFile); err != nil {
			return fmt.Errorf("remove: unable to remove mmproj: %q", mp.ProjFile)
		}

		dir := filepath.Dir(mp.ProjFile)
		base := filepath.Base(mp.ProjFile)
		shaFile := filepath.Join(dir, "sha", base)

		if err := os.Remove(shaFile); err != nil {
			return fmt.Errorf("remove: unable to remove model: %q", shaFile)
		}
	}

	return nil
}
