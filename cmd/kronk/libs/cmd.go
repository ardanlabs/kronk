package libs

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "libs",
	Short: "Install or upgrade llama.cpp libraries",
	Long: `Install or upgrade llama.cpp libraries

Environment Variables (web mode - default):
      KRONK_TOKEN         (required when auth enabled)  Authentication token for the kronk server.
      KRONK_WEB_API_HOST  (default localhost:8080)  IP Address for the kronk server.

Environment Variables (--local mode):
      KRONK_ARCH       (default: runtime.GOARCH)          The architecture to install.
      KRONK_LIB_PATH   (default: $HOME/.kronk/libraries)  The path to the libraries directory,
      KRONK_OS         (default: runtime.GOOS)            The operating system to install.
      KRONK_PROCESSOR  (default: cpu)                     Options: cpu, cuda, metal, vulkan

Flags:
      --local        Run without the model server
      --no-upgrade   Don't upgrade if libraries are already installed
      --version      Download a specific llama.cpp version instead of latest`,
	Args: cobra.NoArgs,
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", false, "Run without the model server")
	Cmd.Flags().Bool("no-upgrade", false, "Don't upgrade if libraries are already installed")
	Cmd.Flags().String("version", "", "Download a specific llama.cpp version instead of latest")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command) error {
	local, _ := cmd.Flags().GetBool("local")
	noUpgrade, _ := cmd.Flags().GetBool("no-upgrade")
	version, _ := cmd.Flags().GetString("version")

	var err error

	switch local {
	case true:
		err = runLocal(noUpgrade, version)
	default:
		err = runWeb(noUpgrade, version)
	}

	if err != nil {
		return err
	}

	return nil
}
