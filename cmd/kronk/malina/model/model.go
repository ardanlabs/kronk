// Package model provides the "malina model" sub-command tree.
package model

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/tools/malina/models"
	"github.com/spf13/cobra"
)

// Cmd is the cobra command for "kronk malina model".
var Cmd = newCmd()

func newCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "model",
		Short: "Manage curated image-model bundles",
		Long: `Manage curated image-model bundles stored under the local Kronk base path.

Malina model routes are not yet available in the model server, so --local is
currently required on each management command.

COMMANDS

  catalog  List supported bundles
  list     List installed bundles
  pull     Download a curated bundle
  remove   Remove an installed bundle`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	catalogCmd := &cobra.Command{
		Use:   "catalog",
		Short: "List supported image-model bundles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocal(cmd); err != nil {
				return err
			}
			return runCatalog(cmd, args)
		},
	}
	catalogCmd.Flags().Bool("local", false, "Run without the model server")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List installed image-model bundles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocal(cmd); err != nil {
				return err
			}
			return runList(cmd, args)
		},
	}
	listCmd.Flags().Bool("local", false, "Run without the model server")

	pullCmd := &cobra.Command{
		Use:   "pull <BUNDLE>",
		Short: "Download a curated image-model bundle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocal(cmd); err != nil {
				return err
			}
			return runPull(cmd, args[0])
		},
	}
	pullCmd.Flags().Bool("local", false, "Run without the model server")

	removeCmd := &cobra.Command{
		Use:   "remove <BUNDLE>",
		Short: "Remove an installed image-model bundle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocal(cmd); err != nil {
				return err
			}
			return runRemove(cmd, args[0])
		},
	}
	removeCmd.Flags().Bool("local", false, "Run without the model server")

	cmd.AddCommand(catalogCmd)
	cmd.AddCommand(listCmd)
	cmd.AddCommand(pullCmd)
	cmd.AddCommand(removeCmd)

	return cmd
}

func requireLocal(cmd *cobra.Command) error {
	local, _ := cmd.Flags().GetBool("local")
	if !local {
		return errors.New("malina model: model-server mode is not available; use --local")
	}
	return nil
}

func runCatalog(cmd *cobra.Command, args []string) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tFILES\tLICENSE\tGATED\tDESCRIPTION")
	for _, bundle := range models.Catalog() {
		fmt.Fprintf(w, "%s\t%d\t%s\t%t\t%s\n", bundle.Name, len(bundle.Files), bundle.License, bundle.Gated, bundle.Description)
	}
	return w.Flush()
}

func runList(cmd *cobra.Command, args []string) error {
	mdls, err := models.NewWithPaths(client.GetBasePath(cmd))
	if err != nil {
		return fmt.Errorf("malina model list: new: %w", err)
	}
	if err := mdls.BuildIndex(malina.DiscardLogger, false); err != nil {
		return fmt.Errorf("malina model list: build index: %w", err)
	}

	type installed struct {
		name string
		path models.Path
	}
	var found []installed
	for _, name := range models.SupportedBundles() {
		mp, err := mdls.FullPath(name.String())
		if errors.Is(err, models.ErrModelNotFound) {
			continue
		}
		if err != nil {
			return fmt.Errorf("malina model list: %w", err)
		}
		found = append(found, installed{name: name.String(), path: mp})
	}
	if len(found) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no models installed")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSIZE\tFILES\tPATH")
	for _, item := range found {
		var size int64
		for _, fileSize := range item.path.FileSizes {
			size += fileSize
		}
		path := ""
		if len(item.path.ModelFiles) > 0 {
			path = filepath.Dir(item.path.ModelFiles[0])
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", item.name, humanSize(size), len(item.path.ModelFiles), path)
	}
	return w.Flush()
}

func runPull(cmd *cobra.Command, source string) error {
	mdls, err := models.NewWithPaths(client.GetBasePath(cmd))
	if err != nil {
		return fmt.Errorf("malina model pull: new: %w", err)
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Minute)
	defer cancel()
	mp, err := mdls.Download(ctx, malina.FmtLogger, source)
	if err != nil {
		return fmt.Errorf("malina model pull: %w", err)
	}

	for _, path := range mp.ModelFiles {
		fmt.Fprintln(cmd.OutOrStdout(), "installed:", path)
	}
	return nil
}

func runRemove(cmd *cobra.Command, modelID string) error {
	mdls, err := models.NewWithPaths(client.GetBasePath(cmd))
	if err != nil {
		return fmt.Errorf("malina model remove: new: %w", err)
	}
	mp, err := mdls.FullPath(modelID)
	if err != nil {
		return fmt.Errorf("malina model remove: %w", err)
	}
	if err := mdls.Remove(mp, malina.FmtLogger); err != nil {
		return fmt.Errorf("malina model remove: %w", err)
	}

	for _, path := range mp.ModelFiles {
		fmt.Fprintln(cmd.OutOrStdout(), "removed:", path)
	}
	return nil
}

func humanSize(n int64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
	)
	switch {
	case n >= gib:
		return fmt.Sprintf("%.1f GiB", float64(n)/float64(gib))
	case n >= mib:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(mib))
	case n >= kib:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(kib))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
