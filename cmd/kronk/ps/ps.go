// Package ps provides the ps command code.
package ps

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/domain/toolapp"
)

// RunWeb executes the ps command against the model server.
func RunWeb(args []string) error {
	url, err := client.DefaultURL("/v1/models/status")
	if err != nil {
		return fmt.Errorf("list: default: %w", err)
	}

	fmt.Println("URL:", url)

	client := client.New(client.FmtLogger)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var info []toolapp.ModelDetail
	if err := client.Do(ctx, http.MethodGet, url, nil, &info); err != nil {
		return fmt.Errorf("libs: unable to get model list: %w", err)
	}

	printWeb(info)

	return nil
}

// =============================================================================

func printWeb(models []toolapp.ModelDetail) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tOWNED BY\tMODEL FAMILY\tSIZE\tEXPIRES\tSESSIONS")

	for _, model := range models {
		size := formatSize(model.Size)
		expiresIn := time.Until(model.ExpiresAt).Truncate(time.Second)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\n", model.ID, model.OwnedBy, model.ModelFamily, size, expiresIn, model.ActiveStreams)
	}

	w.Flush()
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
