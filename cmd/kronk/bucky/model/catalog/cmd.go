// Package catalog prints the bundled whisper model catalog.
package catalog

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/spf13/cobra"
)

// Cmd is the cobra command for "kronk bucky model catalog".
var Cmd = &cobra.Command{
	Use:   "catalog",
	Short: "List the bundled catalog of well-known whisper models",
	Long: `List the bundled catalog of well-known whisper models that the bucky
backend knows how to download by short name (tiny, base.en, large-v3,
...). Each row reports the short name, the published download size, and
the resolved download URL.

MODES

  Web Mode (default): Reserved for a future server-wiring step.
  Local Mode (--local): Reads the bundled catalog in-process.

EXAMPLES

  # List every bundled catalog entry.
  kronk bucky model catalog --local`,
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
		return fmt.Errorf("bucky model catalog: web mode not yet implemented; pass --local to run against local files")
	}

	entries := models.Catalog()
	if len(entries) == 0 {
		fmt.Println("no catalog entries")
		return nil
	}

	names := models.SupportedModels()

	fmt.Printf("%-20s %-10s %s\n", "NAME", "SIZE", "URL")
	for _, name := range names {
		e := entries[name]
		fmt.Printf("%-20s %-10s %s\n", name, e.Size, e.URL)
	}

	return nil
}
