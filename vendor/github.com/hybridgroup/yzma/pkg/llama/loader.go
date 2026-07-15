package llama

import (
	"fmt"

	"github.com/hybridgroup/yzma/pkg/loader"
)

var libPath string

// LibPath returns the path to the loaded llama.cpp shared libraries.
func LibPath() string {
	return libPath
}

// Load loads the shared llama.cpp libraries from the specified path.
func Load(path string) error {
	libPath = path
	lib, err := loader.LoadLibrary(path, "ggml")
	if err != nil {
		return err
	}

	if err := loadGGML(lib); err != nil {
		return err
	}

	lib, err = loader.LoadLibrary(path, "ggml-base")
	if err != nil {
		return err
	}

	if err := loadGGMLBase(lib); err != nil {
		return err
	}

	lib, err = loader.LoadLibrary(path, "llama")
	if err != nil {
		return err
	}

	if err := loadBackendFuncs(lib); err != nil {
		return err
	}

	if err := loadModelFuncs(lib); err != nil {
		return err
	}

	if err := loadBatchFuncs(lib); err != nil {
		return err
	}

	if err := loadVocabFuncs(lib); err != nil {
		return err
	}

	if err := loadSamplingFuncs(lib); err != nil {
		return err
	}

	if err := loadChatFuncs(lib); err != nil {
		return err
	}

	if err := loadContextFuncs(lib); err != nil {
		return err
	}

	if err := loadMemoryFuncs(lib); err != nil {
		return err
	}

	if err := loadLogFuncs(lib); err != nil {
		return err
	}

	if err := loadStateFuncs(lib); err != nil {
		return err
	}

	if err := loadLoraFuncs(lib); err != nil {
		return err
	}

	if err := loadPerfFuncs(lib); err != nil {
		return err
	}

	return nil
}

// Init is a convenience function to handle initialization of llama.cpp.
func Init() {
	BackendInit()
	GGMLBackendLoadAllFromPath(libPath)
}

// Close frees resources used by llama.cpp and unloads any dynamically loaded backends.
func Close() {
	BackendFree()

	for i := uint64(0); i < GGMLBackendRegCount(); i++ {
		reg := GGMLBackendRegGet(i)
		if reg == 0 {
			continue
		}
		GGMLBackendUnload(reg)
	}
}

func loadError(name string, err error) error {
	return fmt.Errorf("could not load %q: %w", name, err)
}
