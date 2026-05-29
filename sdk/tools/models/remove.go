package models

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Remove remove the specified file from the models directory.
func (m *Models) Remove(mp Path, log applog.Logger) (err error) {
	defer func() {
		if errDfr := m.BuildIndex(log, false); errDfr != nil && err == nil {
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

		// Best-effort cleanup of the verified-sentinel sibling. A missing
		// sentinel is normal (older downloads, never-loaded models, or
		// read-only mounts) and must not error the delete.
		if err := model.RemoveVerifiedSentinel(modelFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove: unable to remove verified sentinel for %q: %w", modelFile, err)
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

		if err := model.RemoveVerifiedSentinel(mp.ProjFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove: unable to remove verified sentinel for %q: %w", mp.ProjFile, err)
		}
	}

	return nil
}
