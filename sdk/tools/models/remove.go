package models

import (
	"fmt"
	"os"
)

// Remove remove the specified file from the models directory.
func (m *Models) Remove(mp Path) (err error) {
	defer func() {
		if errDfr := m.BuildIndex(); err != nil {
			err = errDfr
		}
	}()

	for _, f := range mp.ModelFiles {
		if err := os.Remove(f); err != nil {
			return fmt.Errorf("remove-model: unable to remove model: %q", f)
		}
	}

	if mp.ProjFile != "" {
		if err := os.Remove(mp.ProjFile); err != nil {
			return fmt.Errorf("remove-model: unable to remove mmproj: %q", mp.ProjFile)
		}
	}

	return nil
}
