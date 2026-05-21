package models

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

// File describes a single whisper model installed on disk.
type File struct {
	ID       string    // Short catalog name ("tiny", "base.en", "large-v3").
	Path     string    // Absolute path to the on-disk .bin file.
	Size     int64     // Size of the on-disk file in bytes.
	Modified time.Time // Last modification time of the on-disk file.
}

// Files returns every whisper model installed in the models directory
// sorted by short catalog id. The list is built from the on-disk index
// produced by BuildIndex; callers should invoke BuildIndex first when
// they need to reflect file-system changes made out-of-band.
func (m *Models) Files() ([]File, error) {
	index := m.loadIndex()

	list := make([]File, 0, len(index))

	for modelID, mp := range index {
		if len(mp.ModelFiles) == 0 {
			continue
		}

		path := mp.ModelFiles[0]

		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat %q: %w", path, err)
		}

		list = append(list, File{
			ID:       modelID,
			Path:     path,
			Size:     info.Size(),
			Modified: info.ModTime(),
		})
	}

	slices.SortFunc(list, func(a, b File) int {
		return strings.Compare(strings.ToLower(a.ID), strings.ToLower(b.ID))
	})

	return list, nil
}
