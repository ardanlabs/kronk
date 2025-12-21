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
	  --username     The user to apply to the token
      --duration     Token duration (e.g., 1h, 1d, 1m, 1y)
      --endpoints    Comma-separated list of allowed endpoints`,
	Args: cobra.NoArgs,
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", false, "Run without the model server")
	Cmd.Flags().String("username", "", "The subject for the token")
	Cmd.Flags().String("duration", "", "Token duration (e.g., 1h, 1d, 1m, 1y)")
	Cmd.Flags().StringSlice("endpoints", []string{}, "Comma-separated list of allowed endpoints")
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
	username, _ := cmd.Flags().GetString("username")
	flagDuration, _ := cmd.Flags().GetString("duration")
	flagEndpoints, _ := cmd.Flags().GetStringSlice("endpoints")

	if username == "" {
		return fmt.Errorf("username required")
	}

	duration, err := time.ParseDuration(flagDuration)
	if err != nil {
		return fmt.Errorf("parse-duration: %w", err)
	}

	cfg := config{
		AdminToken: adminToken,
		UserName:   username,
		Endpoints:  flagEndpoints,
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
