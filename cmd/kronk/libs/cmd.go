package libs

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "libs",
	Short: "Install or upgrade llama.cpp libraries",
	Long: `Install or upgrade llama.cpp libraries.

Kronk requires llama.cpp shared libraries for runtime inference. This command
downloads and installs the appropriate library version for your hardware platform.

The command auto-detects your system architecture (amd64/arm64), operating system
(linux/darwin/windows), and processor type (cpu/metal/cuda/rocm/vulkan).

HARDWARE BACKENDS

  cpu   - CPU-only inference (works on all systems)
  metal - Apple Silicon GPU acceleration (macOS)
  cuda  - NVIDIA GPU acceleration
  rocm  - AMD GPU acceleration
  vulkan- Cross-platform GPU acceleration

MODES

  Web Mode (default): Installs via the running server at localhost:8080.
  Local Mode (--local): Direct download without requiring a server.

EXAMPLES

  # Install libraries for current platform
  kronk libs --local

  # Install CUDA libraries explicitly
  KRONK_PROCESSOR=cuda kronk libs --local

  # Download specific version
  kronk libs --local --version=b7406

ENVIRONMENT VARIABLES (Local Mode)

  KRONK_ARCH       - Architecture: amd64, arm64
  KRONK_LIB_PATH   - Library directory path
  KRONK_OS         - Operating system: linux, darwin, windows
  KRONK_PROCESSOR  - Hardware backend: cpu, cuda, metal, rocm, vulkan`,
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
