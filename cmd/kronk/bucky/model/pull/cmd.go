// Package pull downloads a whisper model by short name or URL.
package pull

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/spf13/cobra"
)

// Cmd is the cobra command for "kronk bucky model pull".
var Cmd = &cobra.Command{
	Use:   "pull <MODEL>",
	Short: "Download a whisper model by short name or URL",
	Long: `Download a whisper model from the bundled catalog or from a URL.

The argument may be:

  - A short catalog name        ("tiny", "base.en", "large-v3")
  - A full ggml filename        ("ggml-tiny.bin")
  - A fully qualified URL       (any go-getter-compatible URL)

Use "kronk bucky model catalog" to list the bundled short names.

MODES

  Web Mode (default): Downloads through the model server at
    /v1/bucky/models/pull.
  Local Mode (--local): Downloads in-process directly to the bucky
    models root.

EXAMPLES

  # Download the tiny English model via a running server.
  kronk bucky model pull tiny.en

  # Download in-process by full filename.
  kronk bucky model pull --local ggml-base.bin

  # Download from a custom URL.
  kronk bucky model pull --local https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin

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

func run(cmd *cobra.Command, source string) error {
	local, _ := cmd.Flags().GetBool("local")
	if local {
		return runLocal(cmd, source)
	}
	return runWeb(source)
}

func runWeb(source string) error {
	url, err := client.DefaultURL("/v1/bucky/models/pull")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	body := client.D{
		"source": source,
	}

	cln := client.NewSSE[toolapp.PullResponse](
		client.FmtLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	ch := make(chan toolapp.PullResponse)
	if err := cln.Do(ctx, http.MethodPost, url, body, ch); err != nil {
		return fmt.Errorf("do: unable to download whisper model: %w", err)
	}

	for ver := range ch {
		fmt.Print(ver.Status)
	}

	fmt.Println()

	return nil
}

func runLocal(cmd *cobra.Command, source string) error {
	mdls, err := models.NewWithPaths(client.GetBasePath(cmd))
	if err != nil {
		return fmt.Errorf("bucky model pull: new: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	path, err := mdls.Download(ctx, bucky.FmtLogger, source)
	if err != nil {
		return fmt.Errorf("bucky model pull: %w", err)
	}

	fmt.Println()
	for _, f := range path.ModelFiles {
		fmt.Println("installed:", f)
	}

	return nil
}
