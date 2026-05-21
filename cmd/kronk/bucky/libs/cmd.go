// Package libs provides the "bucky libs" sub-command code.
package libs

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Cmd is the cobra command for "kronk bucky libs". It manages the
// whisper.cpp shared libraries that the whisper runtime loads at
// startup.
var Cmd = &cobra.Command{
	Use:   "libs",
	Short: "Install or upgrade whisper.cpp libraries",
	Long: `Install or upgrade whisper.cpp libraries.

Kronk's whisper backend (bucky) requires whisper.cpp shared libraries
for runtime inference. This command downloads and installs the
appropriate library bundle for your hardware platform under the bucky
libraries root (default: ~/.kronk/bucky-libraries/).

MODES

  Web Mode (default): Reserved for a future server-wiring step. The
    model server does not yet expose whisper endpoints; running this
    command without --local returns an error.
  Local Mode (--local): Direct download without requiring a server.

The command auto-detects your system architecture (amd64/arm64),
operating system (linux/darwin/windows), and processor type
(cpu/metal/cuda/vulkan).

HARDWARE BACKENDS

  cpu    - CPU-only inference (works on all systems)
  metal  - Apple Silicon GPU acceleration (macOS, universal slice)
  cuda   - NVIDIA GPU acceleration (Linux, Windows)
  vulkan - Cross-platform GPU acceleration (Linux)

EXAMPLES

  # Install the default whisper.cpp libraries for the current host.
  kronk bucky libs --local

  # Install a specific whisper.cpp version.
  kronk bucky libs --local --version=v1.7.0

  # List supported (arch, os, processor) combinations.
  kronk bucky libs --local --list-combinations

  # Install a Linux/CUDA bundle alongside the active install.
  kronk bucky libs --local --install --arch=amd64 --os=linux --processor=cuda

  # List installed library bundles.
  kronk bucky libs --local --list-installs

  # Remove an install.
  kronk bucky libs --local --remove-install --arch=amd64 --os=linux --processor=cuda

  # Switch to a previously installed bundle by setting KRONK_BUCKY_LIB_PATH.
  export KRONK_BUCKY_LIB_PATH=~/.kronk/bucky-libraries/linux/amd64/cuda

ENVIRONMENT VARIABLES

  KRONK_ARCH             - Architecture: amd64, arm64
  KRONK_BUCKY_LIB_PATH   - Whisper library directory path
  KRONK_OS               - Operating system: linux, darwin, windows
  KRONK_PROCESSOR        - Hardware backend: cpu, cuda, metal, vulkan`,
	Args: cobra.NoArgs,
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", false, "Run without the model server (currently required; web mode lands with the server-wiring step)")
	Cmd.Flags().String("version", "", "Download a specific whisper.cpp version instead of the default")

	Cmd.Flags().Bool("install", false, "Install for the supplied --arch/--os/--processor triple (lands in its own folder under the libraries root)")
	Cmd.Flags().String("arch", "", "Architecture for triple-aware install operations (amd64, arm64)")
	Cmd.Flags().String("os", "", "Operating system for triple-aware install operations (linux, darwin, windows)")
	Cmd.Flags().String("processor", "", "Processor for triple-aware install operations (cpu, cuda, metal, vulkan)")
	Cmd.Flags().Bool("list-combinations", false, "List supported (arch, os, processor) combinations and exit")
	Cmd.Flags().Bool("list-installs", false, "List installed library bundles under the libraries root and exit")
	Cmd.Flags().Bool("remove-install", false, "Remove the install matching --arch/--os/--processor")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command) error {
	local, _ := cmd.Flags().GetBool("local")
	if !local {
		return fmt.Errorf("bucky libs: web mode not yet implemented; pass --local to run against local files")
	}

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
		return runInstall(opts)
	}

	return runDefault(version)
}
