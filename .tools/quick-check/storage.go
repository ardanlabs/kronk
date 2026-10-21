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
	if _, err := hex.DecodeString(r.ServerRevision); err != nil || r.ServerModified == nil {
		return "unknown"
	}
	if *r.ServerModified {
		return r.ServerRevision + "+develop"
	}
	return r.ServerRevision
}

// Retain the new candidate and the newest prior committed run. Without a
// committed baseline, retain the newest prior candidate instead. Verdicts do not
// affect retention: a failed check still needs its diagnostic evidence.
func retainRuns(parent, current string) error {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	type run struct {
		path      string
		finished  time.Time
		committed bool
	}
	var runs []run
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		stamp, suffix, ok := strings.Cut(entry.Name(), "-")
		if _, err := time.Parse("20060102T150405Z", stamp); err != nil || !ok {
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
		var finished time.Time
		for _, name := range []string{"summary.json", "events.ndjson", "tool.log"} {
			info, err := os.Lstat(filepath.Join(path, name))
			if err != nil || !info.Mode().IsRegular() {
				complete = false
				break
			}
			if name == "summary.json" {
				finished = info.ModTime()
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
		runs = append(runs, run{path, finished, r.serverVersion() != "unknown" && !*r.ServerModified})
	}
	slices.SortFunc(runs, func(a, b run) int {
		if order := b.finished.Compare(a.finished); order != 0 {
			return order
		}
		return strings.Compare(b.path, a.path)
	})
	keep := ""
	for _, run := range runs {
		if keep == "" || run.committed {
			keep = run.path
		}
		if run.committed {
			break
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
