// Package diagnose provides the diagnose command for inspecting the host
// environment (versions, system/hardware, llama.cpp devices, and a benchmark)
// to help debug problems on a user's machine.
package diagnose

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Cmd is the "kronk diagnose" command.
var Cmd = &cobra.Command{
	Use:   "diagnose",
	Short: "Inspect the host environment for debugging",
	Long: `Inspect the host environment and report information useful for debugging
"Kronk doesn't work" or "the model is slow" problems.

The report includes component versions (Kronk, yzma), host/hardware details,
the installed llama.cpp build and the compute devices it sees, and a small
llama-bench run.

By default this command is inspect-only and never downloads: it reports on the
llama.cpp libraries and model already installed. Use --install to download
anything missing.

EXAMPLES

  # Human-readable report (default, inspect-only)
  kronk diagnose

  # Download missing llama.cpp libraries / benchmark model, then report
  kronk diagnose --install

  # Machine-readable report (paste into a bug report)
  kronk diagnose --format json
  kronk diagnose --format yaml

  # Skip the benchmark (faster)
  kronk diagnose --no-bench

  # Benchmark a specific model (source or local .gguf path)
  kronk diagnose --model unsloth/Qwen3-8B-Q8_0`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		code, err := run(cmd)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(code)
	},
}

func init() {
	Cmd.Flags().String("format", "text", "Output format: text, json, or yaml")
	Cmd.Flags().Bool("install", false, "Download missing llama.cpp libraries and benchmark model")
	Cmd.Flags().Bool("no-bench", false, "Skip the llama-bench step")
	Cmd.Flags().String("model", "", "Model source or local .gguf path to benchmark")
}
