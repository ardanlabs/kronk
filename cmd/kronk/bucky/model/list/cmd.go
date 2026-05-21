// Package list lists installed whisper models.
package list

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
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

  Web Mode (default): Lists installed models reported by the model
    server at /v1/bucky/models.
  Local Mode (--local): Lists model files on disk in-process without
    contacting a server.

ENVIRONMENT VARIABLES

  KRONK_BASE_PATH  Base path for kronk data (models, libraries, catalog, model_config)`,
	Args: cobra.NoArgs,
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", false, "Run without the model server")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command) error {
	local, _ := cmd.Flags().GetBool("local")
	if local {
		return runLocal(cmd)
	}
	return runWeb()
}

func runWeb() error {
	url, err := client.DefaultURL("/v1/bucky/models")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	cln := client.New(
		client.FmtLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var resp toolapp.BuckyModelsResponse
	if err := cln.Do(ctx, http.MethodGet, url, nil, &resp); err != nil {
		return fmt.Errorf("do: unable to get bucky model list: %w", err)
	}

	if len(resp.Models) == 0 {
		fmt.Println("no models installed")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSIZE\tPATH")
	for _, m := range resp.Models {
		fmt.Fprintf(w, "%s\t%s\t%s\n", m.ID, humanSize(m.Size), m.Path)
	}
	w.Flush()

	return nil
}

func runLocal(cmd *cobra.Command) error {
	mdls, err := models.NewWithPaths(client.GetBasePath(cmd))
	if err != nil {
		return fmt.Errorf("bucky model list: new: %w", err)
	}

	if err := mdls.BuildIndex(bucky.DiscardLogger, false); err != nil {
		return fmt.Errorf("bucky model list: build index: %w", err)
	}

	files, err := mdls.Files()
	if err != nil {
		return fmt.Errorf("bucky model list: files: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("no models installed")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSIZE\tPATH")
	for _, f := range files {
		fmt.Fprintf(w, "%s\t%s\t%s\n", f.ID, humanSize(f.Size), f.Path)
	}
	w.Flush()

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
