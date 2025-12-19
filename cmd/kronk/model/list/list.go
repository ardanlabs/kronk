// Package list provides the pull command code.
package list

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/client"
	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func runWeb() error {
	url, err := client.DefaultURL("/v1/models")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	client := client.New(client.FmtLogger)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var info toolapp.ListModelInfoResponse
	if err := client.Do(ctx, http.MethodGet, url, nil, &info); err != nil {
		return fmt.Errorf("do: unable to get model list: %w", err)
	}

	printWeb(info.Data)

	return nil
}

func runLocal() error {
	modelPath := defaults.ModelsDir("")

	models, err := models.RetrieveFiles(modelPath)
	if err != nil {
		return err
	}

	printLocal(models)

	return nil
}

// =============================================================================

func printWeb(models []toolapp.ListModelDetail) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tOWNED BY\tMODEL FAMILY\tSIZE\tMODIFIED")

	for _, model := range models {
		size := formatSize(model.Size)
		modified := formatTime(model.Modified)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", model.ID, model.OwnedBy, model.ModelFamily, size, modified)
	}

	w.Flush()
}

func printLocal(models []models.File) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tOWNED BY\tMODEL FAMILY\tSIZE\tMODIFIED")

	for _, model := range models {
		size := formatSize(model.Size)
		modified := formatTime(model.Modified)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", model.ID, model.OwnedBy, model.ModelFamily, size, modified)
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

func formatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case diff < 30*24*time.Hour:
		weeks := int(diff.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	default:
		months := int(diff.Hours() / 24 / 30)
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	}
}
