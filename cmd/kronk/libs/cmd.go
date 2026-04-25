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

  Web Mode (default): Installs via the running server at localhost:11435.
  Local Mode (--local): Direct download without requiring a server.

EXAMPLES

  # Install libraries for current platform
  kronk libs --local

  # Install CUDA libraries explicitly
  KRONK_PROCESSOR=cuda kronk libs --local

  # Download specific version
  kronk libs --local --version=b7406

  # List supported (arch, os, processor) combinations
  kronk libs --list-combinations

  # Install a Linux/CUDA bundle alongside the active install
  kronk libs --install --arch=amd64 --os=linux --processor=cuda

  # List installed library bundles
  kronk libs --list-installs

  # Remove an install
  kronk libs --remove-install --arch=amd64 --os=linux --processor=cuda

  # Switch to a previously installed bundle by setting KRONK_LIB_PATH
  export KRONK_LIB_PATH=~/.kronk/libraries/linux/amd64/cuda

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

	Cmd.Flags().Bool("install", false, "Install for the supplied --arch/--os/--processor triple (lands in its own folder under the libraries root)")
	Cmd.Flags().String("arch", "", "Architecture for triple-aware install operations (amd64, arm64)")
	Cmd.Flags().String("os", "", "Operating system for triple-aware install operations (linux, bookworm, trixie, darwin, windows)")
	Cmd.Flags().String("processor", "", "Processor for triple-aware install operations (cpu, cuda, metal, rocm, vulkan)")
	Cmd.Flags().Bool("list-combinations", false, "List supported (arch, os, processor) combinations and exit")
	Cmd.Flags().Bool("list-installs", false, "List installed library bundles under the libraries root and exit")
	Cmd.Flags().Bool("remove-install", false, "Remove the install matching --arch/--os/--processor")
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

	tripleInstall, _ := cmd.Flags().GetBool("install")
	arch, _ := cmd.Flags().GetString("arch")
	opSys, _ := cmd.Flags().GetString("os")
	processor, _ := cmd.Flags().GetString("processor")
	listCombinations, _ := cmd.Flags().GetBool("list-combinations")
	listInstalls, _ := cmd.Flags().GetBool("list-installs")
	removeInstall, _ := cmd.Flags().GetBool("remove-install")

	opts := installOpts{
		arch:      arch,
		os:        opSys,
		processor: processor,
		version:   version,
		install:   tripleInstall,
		list:      listInstalls,
		listCombo: listCombinations,
		remove:    removeInstall,
	}

	if opts.isInstallOp() {
		if local {
			return runInstallLocal(opts)
		}
		return runInstallWeb(opts)
	}

	if local {
		return runLocal(noUpgrade, version)
	}
	return runWeb(noUpgrade, version)
}
