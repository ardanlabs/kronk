// Package libs provides the "malina libs" sub-command.
package libs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/malina"
	malinalibs "github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	"github.com/spf13/cobra"
)

// Cmd is the cobra command for "kronk malina libs".
var Cmd = newCmd()

func newCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "libs",
		Short: "Install or manage stable-diffusion.cpp libraries",
		Long: `Install or manage stable-diffusion.cpp libraries on the local machine.

Malina library routes are not yet available in the model server, so --local is
currently required. With no operation flag, the command detects the current
host and installs Kronk's pinned stable-diffusion.cpp version. Use --upgrade to
track the latest published release or --version to select a specific release.

EXAMPLES

  kronk malina libs --local
  kronk malina libs --local --upgrade
  kronk malina libs --local --list-combinations
  kronk malina libs --local --list-installs
  kronk malina libs --local --install --arch=arm64 --os=darwin --processor=metal
  kronk malina libs --local --remove-install --arch=arm64 --os=darwin --processor=metal`,
		Args: cobra.NoArgs,
		RunE: run,
	}

	cmd.Flags().Bool("local", false, "Run without the model server")
	cmd.Flags().Bool("upgrade", false, "Track the latest stable-diffusion.cpp release")
	cmd.Flags().String("version", "", "Install a specific stable-diffusion.cpp version")
	cmd.Flags().Bool("install", false, "Install for the supplied --arch/--os/--processor triple")
	cmd.Flags().String("arch", "", "Architecture for an explicit install operation")
	cmd.Flags().String("os", "", "Operating system for an explicit install operation")
	cmd.Flags().String("processor", "", "Processor for an explicit install operation")
	cmd.Flags().Bool("list-combinations", false, "List supported platform combinations")
	cmd.Flags().Bool("list-installs", false, "List installed library bundles")
	cmd.Flags().Bool("remove-install", false, "Remove the install matching --arch/--os/--processor")

	return cmd
}

type options struct {
	arch             string
	opSys            string
	processor        string
	version          string
	upgrade          bool
	install          bool
	listCombinations bool
	listInstalls     bool
	removeInstall    bool
}

func run(cmd *cobra.Command, args []string) error {
	local, _ := cmd.Flags().GetBool("local")
	if !local {
		return errors.New("malina libs: model-server mode is not available; use --local")
	}

	opts := options{}
	opts.upgrade, _ = cmd.Flags().GetBool("upgrade")
	opts.version, _ = cmd.Flags().GetString("version")
	opts.install, _ = cmd.Flags().GetBool("install")
	opts.arch, _ = cmd.Flags().GetString("arch")
	opts.opSys, _ = cmd.Flags().GetString("os")
	opts.processor, _ = cmd.Flags().GetString("processor")
	opts.listCombinations, _ = cmd.Flags().GetBool("list-combinations")
	opts.listInstalls, _ = cmd.Flags().GetBool("list-installs")
	opts.removeInstall, _ = cmd.Flags().GetBool("remove-install")

	if err := opts.validate(); err != nil {
		return err
	}

	switch {
	case opts.listCombinations:
		return listCombinations(cmd)
	case opts.listInstalls:
		return listInstalls(cmd)
	case opts.removeInstall:
		return removeInstall(cmd, opts)
	case opts.install:
		return installFor(cmd, opts)
	default:
		return installDefault(cmd, opts)
	}
}

func (o options) validate() error {
	operations := 0
	for _, set := range []bool{o.install, o.listCombinations, o.listInstalls, o.removeInstall} {
		if set {
			operations++
		}
	}
	if operations > 1 {
		return fmt.Errorf("malina libs: choose only one install, list, or remove operation")
	}
	if (o.install || o.removeInstall) && (o.arch == "" || o.opSys == "" || o.processor == "") {
		return fmt.Errorf("malina libs: --arch, --os, and --processor are required for this operation")
	}
	if operations > 0 && o.upgrade {
		return fmt.Errorf("malina libs: --upgrade applies only to the detected host install")
	}

	return nil
}

func installDefault(cmd *cobra.Command, opts options) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Minute)
	defer cancel()

	lib, err := malinalibs.New(
		malinalibs.WithBasePath(client.GetBasePath(cmd)),
		malinalibs.WithVersion(opts.version),
		malinalibs.WithAllowUpgrade(opts.upgrade),
		malinalibs.WithDetect(ctx, malina.FmtLogger),
	)
	if err != nil {
		return fmt.Errorf("malina libs: new: %w", err)
	}
	tag, err := lib.Download(ctx, malina.FmtLogger)
	if err != nil {
		return fmt.Errorf("malina libs: install stable-diffusion.cpp: %w", err)
	}
	if err := malina.Init(
		malina.WithLibPath(lib.LibsPath()),
		malina.WithProgress(malina.DiscardProgress),
	); err != nil {
		return fmt.Errorf("malina libs: validate installation: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "installed stable-diffusion.cpp %s at %s\n", tag.Version, lib.LibsPath())
	return nil
}

func installFor(cmd *cobra.Command, opts options) error {
	if !malinalibs.IsSupported(opts.arch, opts.opSys, opts.processor) {
		return fmt.Errorf("malina libs: unsupported combination arch=%s os=%s processor=%s", opts.arch, opts.opSys, opts.processor)
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Minute)
	defer cancel()
	lib, err := malinalibs.New(malinalibs.WithBasePath(client.GetBasePath(cmd)))
	if err != nil {
		return fmt.Errorf("malina libs: new: %w", err)
	}
	tag, err := lib.DownloadFor(ctx, malina.FmtLogger, opts.arch, opts.opSys, opts.processor, opts.version)
	if err != nil {
		return fmt.Errorf("malina libs: install: %w", err)
	}

	path := filepath.Join(lib.Root(), opts.opSys, opts.arch, opts.processor)
	fmt.Fprintf(cmd.OutOrStdout(), "installed stable-diffusion.cpp %s at %s\n", tag.Version, path)
	fmt.Fprintf(cmd.OutOrStdout(), "to use this install, set KRONK_MALINA_LIB_PATH=%s\n", path)
	return nil
}

func listCombinations(cmd *cobra.Command) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "OS\tARCH\tPROCESSOR")
	for _, combo := range malinalibs.SupportedCombinations() {
		fmt.Fprintf(w, "%s\t%s\t%s\n", combo.OS, combo.Arch, combo.Processor)
	}
	return w.Flush()
}

func listInstalls(cmd *cobra.Command) error {
	lib, err := malinalibs.New(malinalibs.WithBasePath(client.GetBasePath(cmd)))
	if err != nil {
		return fmt.Errorf("malina libs: new: %w", err)
	}
	tags, err := lib.List()
	if err != nil {
		return fmt.Errorf("malina libs: list installs: %w", err)
	}
	if len(tags) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no installs found")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "OS\tARCH\tPROCESSOR\tVERSION\tACTIVE")
	for _, tag := range tags {
		active := ""
		if tag.OS == lib.OS() && tag.Arch == lib.Arch() && tag.Processor == lib.Processor() {
			active = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", tag.OS, tag.Arch, tag.Processor, tag.Version, active)
	}
	return w.Flush()
}

func removeInstall(cmd *cobra.Command, opts options) error {
	lib, err := malinalibs.New(malinalibs.WithBasePath(client.GetBasePath(cmd)))
	if err != nil {
		return fmt.Errorf("malina libs: new: %w", err)
	}
	if err := lib.Remove(opts.arch, opts.opSys, opts.processor); err != nil {
		return fmt.Errorf("malina libs: remove install: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "removed install arch=%s os=%s processor=%s\n", opts.arch, opts.opSys, opts.processor)
	return nil
}
