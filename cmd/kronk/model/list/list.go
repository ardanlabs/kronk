// Package list provides the pull command code.
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
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func runWeb() error {
	url, err := client.DefaultURL("/v1/models")
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

	var info toolapp.ListModelInfoResponse
	if err := cln.Do(ctx, http.MethodGet, url, nil, &info); err != nil {
		return fmt.Errorf("do: unable to get model list: %w", err)
	}

	printWeb(info.Data)

	return nil
}

func runLocal(models *models.Models) error {
	files, err := models.Files()
	if err != nil {
		return err
	}

	printLocal(files)

	return nil
}

// =============================================================================

func printWeb(models []toolapp.ListModelDetail) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "VAL\tMODEL ID\tPROVIDER\tFAMILY\tMTMD\tSIZE\tMODIFIED")

	for _, model := range models {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			validatedMark(model.Validated), model.ID,
			dash(model.OwnedBy), dash(model.ModelFamily),
			projectionMark(model.HasProjection),
			formatSize(model.Size), formatTime(model.Modified))
	}

	w.Flush()
}

func printLocal(files []models.File) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "VAL\tMODEL ID\tPROVIDER\tFAMILY\tMTMD\tSIZE\tMODIFIED")

	for _, model := range files {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			validatedMark(model.Validated), model.ID,
			dash(model.OwnedBy), dash(model.ModelFamily),
			projectionMark(model.HasProjection),
			formatSize(model.Size), formatTime(model.Modified))
	}

	w.Flush()
}

func validatedMark(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

func projectionMark(b bool) string {
	if b {
		return "✓"
	}
	return ""
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func formatSize(bytes int64) string {
	const (
		KB = 1000
		MB = KB * 1000
		GB = MB * 1000
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
