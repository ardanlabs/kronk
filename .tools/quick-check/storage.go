package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (r report) serverVersion() string {
	if len(r.ServerRevision) != 40 && len(r.ServerRevision) != 64 {
		return "unknown"
	}
	if _, err := hex.DecodeString(r.ServerRevision); err != nil {
		return "unknown"
	}
	return r.ServerRevision
}

// Replace same-commit candidates while preserving their baseline; otherwise
// promote the previous candidate. Verdicts do not affect retention.
func retainRuns(parent, current string) error {
	data, err := os.ReadFile(filepath.Join(current, "summary.json"))
	if err != nil {
		return err
	}
	var candidate report
	if err := json.Unmarshal(data, &candidate); err != nil {
		return err
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	type run struct {
		path     string
		finished time.Time
		version  string
	}
	var runs []run
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		stamp, suffix, ok := strings.Cut(entry.Name(), "-")
		started, err := time.Parse("20060102T150405Z", stamp)
		if err != nil || !ok {
			continue
		}
		if _, err := strconv.ParseUint(suffix, 10, 64); err != nil {
			continue
		}
		path := filepath.Join(parent, entry.Name())
		if entry.Name() == filepath.Base(current) {
			continue
		}
		// Do not remove unrelated directories, symlinks, or incomplete artifacts.
		complete := true
		for _, name := range []string{"summary.json", "events.ndjson", "tool.log"} {
			info, err := os.Lstat(filepath.Join(path, name))
			if err != nil || !info.Mode().IsRegular() {
				complete = false
				break
			}
		}
		if !complete {
			continue
		}
		data, err := os.ReadFile(filepath.Join(path, "summary.json"))
		if err != nil {
			return err
		}
		var r report
		if json.Unmarshal(data, &r) != nil || r.Workload == "" || r.Model == "" || (r.Status != "PASS" && r.Status != "FAIL") {
			continue
		}
		if precise, err := time.Parse(time.RFC3339Nano, r.TestStartedAt); err == nil {
			started = precise
		}
		runs = append(runs, run{path, started, r.serverVersion()})
	}
	slices.SortFunc(runs, func(a, b run) int {
		if order := b.finished.Compare(a.finished); order != 0 {
			return order
		}
		return strings.Compare(b.path, a.path)
	})
	keep := ""
	if len(runs) > 0 {
		if candidate.serverVersion() != "unknown" && candidate.serverVersion() == runs[0].version {
			if len(runs) > 1 {
				keep = runs[1].path
			}
		} else {
			keep = runs[0].path
		}
	}
	for _, run := range runs {
		if run.path != keep {
			if err := os.RemoveAll(run.path); err != nil {
				return fmt.Errorf("prune old quick-check run %s: %w", run.path, err)
			}
		}
	}
	return nil
}
