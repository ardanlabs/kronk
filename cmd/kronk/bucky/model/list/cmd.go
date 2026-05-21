// Package list lists installed whisper models.
package list

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/spf13/cobra"
)

// Cmd is the cobra command for "kronk bucky model list".
var Cmd = &cobra.Command{
	Use:   "list",
	Short: "List installed whisper models",
	Long: `List installed whisper models found under the bucky models root
(default: ~/.kronk/bucky-models/).

MODES

  Web Mode (default): Reserved for a future server-wiring step.
  Local Mode (--local): Lists model files on disk.

ENVIRONMENT VARIABLES

  KRONK_BASE_PATH  Base path for kronk data (models, libraries, catalog, model_config)`,
	Args: cobra.NoArgs,
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", false, "Run without the model server (currently required; web mode lands with the server-wiring step)")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command) error {
	local, _ := cmd.Flags().GetBool("local")
	if !local {
		return fmt.Errorf("bucky model list: web mode not yet implemented; pass --local to run against local files")
	}

	mdls, err := models.NewWithPaths(client.GetBasePath(cmd))
	if err != nil {
		return fmt.Errorf("bucky model list: new: %w", err)
	}

	if err := mdls.BuildIndex(bucky.DiscardLogger, false); err != nil {
		return fmt.Errorf("bucky model list: build index: %w", err)
	}

	entries, err := os.ReadDir(mdls.Path())
	if err != nil {
		return fmt.Errorf("bucky model list: read models directory: %w", err)
	}

	type row struct {
		name string
		file string
		size int64
	}

	rows := make([]row, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".bin") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		short := strings.TrimSuffix(strings.TrimPrefix(name, "ggml-"), ".bin")
		rows = append(rows, row{name: short, file: filepath.Join(mdls.Path(), name), size: info.Size()})
	}

	if len(rows) == 0 {
		fmt.Println("no models installed")
		return nil
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	fmt.Printf("%-25s %-10s %s\n", "NAME", "SIZE", "PATH")
	for _, r := range rows {
		fmt.Printf("%-25s %-10s %s\n", r.name, humanSize(r.size), r.file)
	}

	return nil
}

func humanSize(n int64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
	)
	switch {
	case n >= gib:
		return fmt.Sprintf("%.1f GiB", float64(n)/float64(gib))
	case n >= mib:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(mib))
	case n >= kib:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(kib))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
