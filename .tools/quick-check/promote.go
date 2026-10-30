package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Promotion labels saved measurements; it never rewrites the tested provenance.
type releasePromotion struct {
	Release        string `json:"release"`
	ServerRevision string `json:"server_revision"`
}

var releasePattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$`)

func promotedRelease(dir, revision string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "promotion.json"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var p releasePromotion
	if err := json.Unmarshal(data, &p); err != nil {
		return "", fmt.Errorf("%s: invalid promotion: %w", dir, err)
	}
	if !releasePattern.MatchString(p.Release) || p.ServerRevision != revision {
		return "", fmt.Errorf("%s: promotion release or tested revision is invalid", dir)
	}
	return p.Release, nil
}

func promoteCommand(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("quick-check promote", flag.ContinueOnError)
	flags.SetOutput(out)
	parent := flags.String("out", ".tools/quick-check/output", "retained run queue")
	release := flags.String("release", "", "release baseline label (vX.Y.Z); no tag is created or required")
	commit := flags.String("commit", "", "full tested commit SHA to promote")
	if err := flags.Parse(args); err != nil {
		return err
	}
	_, shaErr := hex.DecodeString(*commit)
	if flags.NArg() != 0 || !releasePattern.MatchString(*release) || shaErr != nil || (len(*commit) != 40 && len(*commit) != 64) {
		return fmt.Errorf("usage: quick-check promote -release vX.Y.Z -commit FULL_SHA [-out DIR]")
	}
	lock := filepath.Join(*parent, ".quick-check.lock")
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("acquire output lock: %w", err)
	}
	f.Close()
	defer os.Remove(lock)
	runs, err := savedRuns(*parent)
	if err != nil {
		return err
	}
	var source, previous string
	for _, run := range runs {
		if strings.HasPrefix(filepath.Base(run.path), "release-") {
			if previous != "" {
				return fmt.Errorf("multiple release baselines in queue")
			}
			previous = run.path
			if filepath.Base(run.path) == "release-"+*release && run.report.ServerRevision != *commit {
				return fmt.Errorf("release %s already represents another tested commit", *release)
			}
		}
		if run.report.ServerRevision == *commit {
			// Prefer the saved candidate over its earlier promoted copy when
			// the same commit has been tested again with a different cell set.
			if source == "" || (strings.HasPrefix(filepath.Base(source), "release-") && !strings.HasPrefix(filepath.Base(run.path), "release-")) {
				source = run.path
			}
		}
	}
	if source == "" {
		return fmt.Errorf("no retained run for commit %s", *commit)
	}
	destination := filepath.Join(*parent, "release-"+*release)
	if previous == destination {
		fmt.Fprintf(out, "%s already represents tested commit %s\n", *release, *commit)
		return nil
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		return fmt.Errorf("refusing to replace existing destination %s", destination)
	}
	staged, err := os.MkdirTemp(*parent, ".promote-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged)
	for _, name := range []string{"summary.json", "events.ndjson", "tool.log"} {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(staged, name), data, 0600); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(releasePromotion{*release, *commit}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(staged, "promotion.json"), append(data, '\n'), 0600); err != nil {
		return err
	}
	if err := os.Rename(staged, destination); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			os.RemoveAll(destination)
		}
	}()
	if previous != "" {
		if err := compareCommand([]string{"-against", "release", "-baseline", previous, "-candidate", destination, "-out", *parent}, out); err != nil {
			// Comparison reports what is available; it does not authorize
			// promotion. The operator may choose a failed or interrupted run.
			fmt.Fprintf(out, "Release comparison: %v\nPromotion remains the operator's decision.\n", err)
		}
	}
	if previous != "" {
		if err := os.RemoveAll(previous); err != nil {
			return err
		}
	}
	committed = true
	fmt.Fprintf(out, "Promoted tested commit %s to %s\nMeasurements and token references unchanged; no release was created.\n", *commit, *release)
	return nil
}
