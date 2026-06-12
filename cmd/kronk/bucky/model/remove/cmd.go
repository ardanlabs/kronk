// Package remove deletes an installed whisper model from disk.
package remove

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/spf13/cobra"
)

// Cmd is the cobra command for "kronk bucky model remove".
var Cmd = &cobra.Command{
	Use:   "remove <MODEL>",
	Short: "Remove an installed whisper model from disk",
	Long: `Remove an installed whisper model from disk. The argument is the
short name ("tiny", "base.en"), the full ggml filename, or the bare
basename without extension.

MODES

  Web Mode (default): Removes the model through the model server at
    /v1/bucky/models/{model}.
  Local Mode (--local): Removes the model file from disk directly
    without contacting a server.

EXAMPLES

  # Remove the tiny English model from a running server.
  kronk bucky model remove ggml-tiny.bin

  # Remove the tiny English model in-process.
  kronk bucky model remove --local ggml-tiny.bin

ENVIRONMENT VARIABLES

  KRONK_BASE_PATH  Base path for kronk data (models, libraries, catalog, model_config)`,
	Args: cobra.ExactArgs(1),
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", false, "Run without the model server")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd, args[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, modelID string) error {
	local, _ := cmd.Flags().GetBool("local")
	if local {
		return runLocal(cmd, modelID)
	}
	return runWeb(modelID)
}

func runWeb(modelID string) error {
	base, err := client.DefaultURL("/v1/bucky/models")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	full := fmt.Sprintf("%s/%s", base, url.PathEscape(modelID))

	fmt.Println("URL:", full)

	fmt.Printf("\nAre you sure you want to remove %q? (y/n): ", modelID)

	var response string
	fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		fmt.Println("Remove cancelled")
		return nil
	}

	cln := client.New(
		client.FmtLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := cln.Do(ctx, http.MethodDelete, full, nil, nil); err != nil {
		return fmt.Errorf("remove-bucky-model: %w", err)
	}

	fmt.Println("Remove complete")

	return nil
}

func runLocal(cmd *cobra.Command, modelID string) error {
	mdls, err := models.NewWithPaths(client.GetBasePath(cmd))
	if err != nil {
		return fmt.Errorf("bucky model remove: new: %w", err)
	}

	if err := mdls.BuildIndex(bucky.DiscardLogger, false); err != nil {
		return fmt.Errorf("bucky model remove: build index: %w", err)
	}

	mp, err := mdls.FullPath(modelID)
	if err != nil {
		return fmt.Errorf("bucky model remove: %w", err)
	}

	if err := mdls.Remove(mp, bucky.FmtLogger); err != nil {
		return fmt.Errorf("bucky model remove: %w", err)
	}

	for _, f := range mp.ModelFiles {
		fmt.Println("removed:", f)
	}

	return nil
}
