package diagnose

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/diagnose"
	"github.com/spf13/cobra"
	yaml "go.yaml.in/yaml/v2"
)

// Exit codes returned by the command.
const (
	exitOK    = 0
	exitError = 1
)

// run collects the diagnostic report and renders it. It returns the process
// exit code along with any error.
func run(cmd *cobra.Command) (int, error) {
	format, _ := cmd.Flags().GetString("format")
	install, _ := cmd.Flags().GetBool("install")
	noBench, _ := cmd.Flags().GetBool("no-bench")
	model, _ := cmd.Flags().GetString("model")

	switch format {
	case "text", "json", "yaml":
	default:
		return exitError, fmt.Errorf("invalid --format %q: must be text, json, or yaml", format)
	}

	// Progress goes to stderr so stdout stays clean (important for json/yaml).
	log := func(_ context.Context, msg string, args ...any) {
		fmt.Fprintf(os.Stderr, "%s", msg)
		for i := 0; i+1 < len(args); i += 2 {
			fmt.Fprintf(os.Stderr, " %v[%v]", args[i], args[i+1])
		}
		fmt.Fprintln(os.Stderr)
	}

	report, err := diagnose.Collect(context.Background(), log,
		diagnose.WithKronkVersion(kronk.Version),
		diagnose.WithModelSource(model),
		diagnose.WithSkipBench(noBench),
		diagnose.WithInstall(install),
	)
	if err != nil {
		return exitError, fmt.Errorf("unable to collect diagnostics: %w", err)
	}

	switch format {
	case "json":
		if err := printJSON(report); err != nil {
			return exitError, err
		}
	case "yaml":
		if err := printYAML(report); err != nil {
			return exitError, err
		}
	default:
		printText(report, noBench)
	}

	return exitOK, nil
}

func printJSON(report diagnose.Report) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("unable to encode report: %w", err)
	}
	return nil
}

func printYAML(report diagnose.Report) error {
	out, err := yaml.Marshal(report)
	if err != nil {
		return fmt.Errorf("unable to encode report: %w", err)
	}
	fmt.Print(string(out))
	return nil
}

func printText(report diagnose.Report, noBench bool) {
	fmt.Println("========== VERSIONS ==========")
	fmt.Println("- kronk   :", report.Versions.Kronk)
	fmt.Println("- yzma    :", report.Versions.Yzma)

	fmt.Println("\n========== SYSTEM INFO ==========")
	fmt.Println("- goOS    :", report.System.OS)
	fmt.Println("- goArch  :", report.System.Arch)
	fmt.Println("- numCPU  :", report.System.NumCPU)
	fmt.Println("- cpu     :", valueOrUnknown(report.System.CPUModel))
	fmt.Println("- ram     :", humanBytes(report.System.RAMBytes))
	printCommands(report.System.Commands)

	fmt.Println("\n========== LLAMA.CPP INFO ==========")
	if !report.Llama.Installed {
		fmt.Println("llama.cpp is not installed. Run: kronk diagnose --install")
		return
	}
	fmt.Println("- binDir  :", report.Llama.BinDir)
	fmt.Println("- build   :", valueOrUnknown(report.Llama.Build))
	if len(report.Llama.Devices) == 0 {
		fmt.Println("- device  : none detected (running CPU-only)")
	}
	for _, d := range report.Llama.Devices {
		fmt.Printf("- device  : %s %s (%d MiB total, %d MiB free)\n", d.ID, d.Name, d.VRAMTotalMiB, d.VRAMFreeMiB)
	}
	printCommands(report.Llama.Commands)

	if noBench {
		return
	}

	fmt.Println("\n========== LLAMA-BENCH ==========")
	if report.Bench.Model == "" {
		fmt.Println("benchmark skipped: model not installed (use --install or --model <path>)")
		return
	}
	fmt.Println("- model   :", report.Bench.Model)
	printCommands(report.Bench.Commands)
}

// valueOrUnknown returns v, or "unknown" when v is empty.
func valueOrUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

// humanBytes renders a byte count as a GiB string, e.g. "128.0 GiB". It returns
// "unknown" when the value is zero.
func humanBytes(b uint64) string {
	if b == 0 {
		return "unknown"
	}
	return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
}

func printCommands(cmds []diagnose.Command) {
	for _, c := range cmds {
		fmt.Printf("\n$ %s\n", c.Cmd)
		if c.Output != "" {
			fmt.Print(c.Output)
			if c.Output[len(c.Output)-1] != '\n' {
				fmt.Println()
			}
		}
		if c.Err != "" {
			fmt.Printf("(error: %s)\n", c.Err)
		}
	}
}
