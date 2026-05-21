package libs

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
)

// installOpts captures the parsed install-related CLI flags. Exactly
// one of install/list/listCombo/remove should be true when isInstallOp
// returns true; runInstall returns an error otherwise.
type installOpts struct {
	arch      string
	os        string
	processor string
	version   string
	install   bool
	list      bool
	listCombo bool
	remove    bool
}

// isInstallOp reports whether any install-related flag was supplied.
func (o installOpts) isInstallOp() bool {
	return o.install || o.list || o.listCombo || o.remove
}

// requireTriple validates that arch/os/processor were all supplied for
// operations that need a specific install identity.
func (o installOpts) requireTriple() error {
	if o.arch == "" || o.os == "" || o.processor == "" {
		return fmt.Errorf("--arch, --os, and --processor are all required for this operation")
	}
	return nil
}

// runInstall handles every install-related subcommand against an
// in-process libs.Libs. The bucky CLI is local-only; there is no
// server-mode counterpart.
func runInstall(opts installOpts) error {
	if opts.listCombo {
		printCombinations(libs.SupportedCombinations())
		return nil
	}

	lib, err := libs.New()
	if err != nil {
		return fmt.Errorf("bucky libs: install: %w", err)
	}

	switch {
	case opts.list:
		tags, err := lib.List()
		if err != nil {
			return fmt.Errorf("bucky libs: list installs: %w", err)
		}
		printInstallTags(lib, tags)
		return nil

	case opts.remove:
		if err := opts.requireTriple(); err != nil {
			return err
		}
		if err := lib.Remove(opts.arch, opts.os, opts.processor); err != nil {
			return fmt.Errorf("bucky libs: remove install: %w", err)
		}
		fmt.Printf("removed install arch=%s os=%s processor=%s\n", opts.arch, opts.os, opts.processor)
		return nil

	case opts.install:
		if err := opts.requireTriple(); err != nil {
			return err
		}
		if !libs.IsSupported(opts.arch, opts.os, opts.processor) {
			return fmt.Errorf("bucky libs: unsupported combination arch=%s os=%s processor=%s", opts.arch, opts.os, opts.processor)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		if _, err := lib.DownloadFor(ctx, bucky.FmtLogger, opts.arch, opts.os, opts.processor, opts.version); err != nil {
			return fmt.Errorf("bucky libs: install: %w", err)
		}
		fmt.Println()

		printUseHint(lib, opts.arch, opts.os, opts.processor)
		return nil
	}

	return fmt.Errorf("bucky libs: no install operation requested")
}

func printCombinations(combos []libs.Combination) {
	if len(combos) == 0 {
		fmt.Println("no combinations available")
		return
	}
	fmt.Printf("%-10s %-10s %-10s\n", "OS", "ARCH", "PROCESSOR")
	for _, c := range combos {
		fmt.Printf("%-10s %-10s %-10s\n", c.OS, c.Arch, c.Processor)
	}
}

// printInstallTags prints the installed bundles. The active install
// (matching lib.LibsPath) is annotated with "(active)".
func printInstallTags(lib *libs.Libs, tags []libs.VersionTag) {
	if len(tags) == 0 {
		fmt.Println("no installs found")
		return
	}

	activeTriple := [3]string{lib.OS(), lib.Arch(), lib.Processor()}

	fmt.Printf("%-10s %-10s %-10s %-10s %s\n", "OS", "ARCH", "PROCESSOR", "VERSION", "")
	for _, t := range tags {
		flag := ""
		if [3]string{t.OS, t.Arch, t.Processor} == activeTriple {
			flag = "(active)"
		}
		fmt.Printf("%-10s %-10s %-10s %-10s %s\n", t.OS, t.Arch, t.Processor, t.Version, flag)
	}
}

// printUseHint prints the environment variable a user must export to
// load the install for the supplied triple as the active runtime
// libraries.
func printUseHint(lib *libs.Libs, arch string, opSys string, processor string) {
	path := filepath.Join(lib.Root(), opSys, arch, processor)
	fmt.Println("To use this install at runtime, set:")
	fmt.Printf("  export KRONK_BUCKY_LIB_PATH=%s\n", path)
}
