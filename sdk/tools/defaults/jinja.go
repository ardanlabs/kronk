package defaults

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

//go:embed jinja/*.jinja
var jinjaFS embed.FS

const (
	// jinjaDirName is the per-user jinja directory under BaseDir. It owns
	// the chat template files shipped with kronk.
	jinjaDirName = "jinja"

	// embeddedJinjaDir is the directory inside jinjaFS, which mirrors the
	// //go:embed directive above.
	embeddedJinjaDir = "jinja"
)

// JinjaDir returns the absolute path to the per-user jinja directory under
// BaseDir. The override is forwarded to BaseDir, so callers that already
// resolved the base path can pass it through.
func JinjaDir(override string) string {
	return filepath.Join(BaseDir(override), jinjaDirName)
}

// WriteJinjaFiles seeds the embedded jinja chat templates to disk. If
// override is provided, files are written under that directory. Otherwise
// the templates are written to <basePath>/jinja/. Existing files are
// overwritten so fixes shipped in the embedded templates always reach
// disk.
func WriteJinjaFiles(override string, basePath string) error {
	targetDir := override
	if targetDir == "" {
		targetDir = JinjaDir(basePath)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("write-jinja-files: creating jinja directory: %w", err)
	}

	entries, err := fs.ReadDir(jinjaFS, embeddedJinjaDir)
	if err != nil {
		return fmt.Errorf("write-jinja-files: reading embedded jinja directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		data, err := fs.ReadFile(jinjaFS, path.Join(embeddedJinjaDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("write-jinja-files: reading embedded %q: %w", entry.Name(), err)
		}

		target := filepath.Join(targetDir, entry.Name())
		if err := os.WriteFile(target, data, 0644); err != nil {
			return fmt.Errorf("write-jinja-files: writing %q: %w", target, err)
		}
	}

	return nil
}
