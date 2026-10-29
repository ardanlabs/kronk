package source

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

// ConfigFiles lists the config files to offer: the one the server reads,
// then model_config*.yaml files in the current directory. Other YAML files
// can load as a config too (modelconfig.Load ignores unknown keys), so the
// name is what selects them.
func ConfigFiles(serverFile string) []string {
	files := []string{serverFile}

	cwd, err := os.Getwd()
	if err != nil {
		return files
	}

	var found []string
	for _, pattern := range []string{"model_config*.yaml", "model_config*.yml"} {
		m, _ := filepath.Glob(filepath.Join(cwd, pattern))
		found = append(found, m...)
	}
	slices.Sort(found)

	for _, f := range found {
		if f == serverFile || slices.Contains(files, f) {
			continue
		}
		if _, err := modelconfig.Load(f); err != nil {
			continue
		}
		files = append(files, f)
	}

	return files
}
