// Package sessions provides the model sessions command.
package sessions

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/spf13/cobra"
)

// Cmd represents the model sessions command.
var Cmd = &cobra.Command{
	Use:   "sessions",
	Short: "List current IMC cache entries",
	Long: `List the current bounded IMC cache entries for loaded models.

Environment Variables:
      KRONK_TOKEN         (required when auth enabled)  Authentication token for the kronk server.
      KRONK_WEB_API_HOST  (default localhost:11435)  IP Address for the kronk server.`,
	Run: main,
}

func main(cmd *cobra.Command, args []string) {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	url, err := client.DefaultURL("/v1/kronk/models/imc-sessions")
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

	var sessions toolapp.IMCSessionsResponse
	if err := cln.Do(ctx, http.MethodGet, url, nil, &sessions); err != nil {
		return fmt.Errorf("do: unable to get IMC sessions: %w", err)
	}

	printSessions(os.Stdout, sessions)
	return nil
}

func printSessions(output io.Writer, sessions toolapp.IMCSessionsResponse) {
	w := tabwriter.NewWriter(output, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "MODEL\tENTRY\tSTATE\tMESSAGES\tCONTEXT\tALLOCATED\tCHECKPOINT\tCHECKPOINT ALLOCATED\tTOTAL ALLOCATED\tWINDOW\tUSED\tMEDIA\tLAST USED")

	for _, session := range sessions {
		used := "0%"
		if session.ContextWindow > 0 {
			used = fmt.Sprintf("%.1f%%", float64(session.Context)/float64(session.ContextWindow)*100)
		}

		lastUsed := "-"
		if !session.LastUsed.IsZero() {
			lastUsed = time.Since(session.LastUsed).Truncate(time.Second).String() + " ago"
		}

		media := "no"
		if session.HasMedia {
			media = "yes"
		}

		fmt.Fprintf(w, "%s\t%d\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%s\t%s\t%s\n",
			session.ModelID,
			session.ID,
			session.State,
			session.Messages,
			session.Context,
			session.Allocated,
			session.CheckpointContext,
			session.CheckpointAllocated,
			session.TotalAllocated,
			session.ContextWindow,
			used,
			media,
			lastUsed,
		)
	}

	w.Flush()
}
