package mtmd

import (
	"fmt"
	"sync"

	"github.com/hybridgroup/yzma/pkg/loader"
)

var muHelperEvalChunks sync.Mutex

// Load loads the shared mtmd library from the specified path.
func Load(path string) error {
	lib, err := loader.LoadLibrary(path, "mtmd")
	if err != nil {
		return err
	}

	if err := loadFuncs(lib); err != nil {
		return err
	}

	if err := loadBitmapFuncs(lib); err != nil {
		return err
	}

	if err := loadChunkFuncs(lib); err != nil {
		return err
	}

	return nil
}

func loadError(name string, err error) error {
	return fmt.Errorf("could not load '': %w", err)
}
