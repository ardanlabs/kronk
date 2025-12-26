package create

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "create",
	Short: "Create a security token",
	Long: `Create a security token

Flags:
      --duration     Token duration (e.g., 1h, 24h, 720h)
      --endpoints    Comma-separated list of endpoints with optional rate limits

Endpoint format:
      endpoint                  Unlimited access (default)
      endpoint:unlimited        Unlimited access (explicit)
      endpoint:limit/window     Rate limited (window: day, month, year)

Examples:
      --endpoints chat-completions,embeddings
      --endpoints "chat-completions:1000/day,embeddings:unlimited"
      --endpoints "chat-completions:100/month,embeddings:500/year"

Environment Variables (web mode - default):
      KRONK_TOKEN         (required when auth enabled)  Authentication token for the kronk server.
      KRONK_WEB_API_HOST  (default localhost:8080)  IP Address for the kronk server.`,
	Args: cobra.NoArgs,
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", false, "Run without the model server")
	Cmd.Flags().String("duration", "", "Token duration (e.g., 1h, 24h, 720h)")
	Cmd.Flags().StringSlice("endpoints", []string{}, "Endpoints with optional rate limits (e.g., chat-completions:1000/day)")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command) error {
	local, _ := cmd.Flags().GetBool("local")
	adminToken := os.Getenv("KRONK_TOKEN")
	flagDuration, _ := cmd.Flags().GetString("duration")
	flagEndpoints, _ := cmd.Flags().GetStringSlice("endpoints")

	duration, err := time.ParseDuration(flagDuration)
	if err != nil {
		return fmt.Errorf("parse-duration: %w", err)
	}

	endpoints, err := parseEndpoints(flagEndpoints)
	if err != nil {
		return fmt.Errorf("parse-endpoints: %w", err)
	}

	cfg := config{
		AdminToken: adminToken,
		Endpoints:  endpoints,
		Duration:   duration,
	}

	switch local {
	case true:
		err = runLocal(cfg)
	default:
		err = runWeb(cfg)
	}

	if err != nil {
		return err
	}

	return nil
}
