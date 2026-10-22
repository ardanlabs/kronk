package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

type storedRun struct {
	path    string
	report  report
	started time.Time
}

// Recognize finalized managed runs only; archives, symlinks and partial outputs
// are never eviction candidates. Timestamp names identify runs awaiting retention.
func savedRuns(parent string) ([]storedRun, error) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil, err
	}
	var runs []storedRun
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		stamp, suffix, staged := strings.Cut(name, "-")
		_, stampErr := time.Parse("20060102T150405Z", stamp)
		_, suffixErr := strconv.ParseUint(suffix, 10, 64)
		if !(staged && stampErr == nil && suffixErr == nil) && !strings.HasPrefix(name, "old-") && !strings.HasPrefix(name, "new-") && !strings.HasPrefix(name, "release-") {
			continue
		}
		path := filepath.Join(parent, name)
		complete := true
		for _, artifact := range []string{"summary.json", "events.ndjson", "tool.log"} {
			info, err := os.Lstat(filepath.Join(path, artifact))
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
			return nil, err
		}
		var r report
		if json.Unmarshal(data, &r) != nil || r.Workload == "" || r.Model == "" || (r.Status != "PASS" && r.Status != "FAIL") {
			continue
		}
		started, err := time.Parse(time.RFC3339Nano, r.TestStartedAt)
		if err != nil {
			return nil, fmt.Errorf("%s: missing or invalid test timestamp: %w", path, err)
		}
		runs = append(runs, storedRun{path, r, started})
	}
	slices.SortFunc(runs, func(a, b storedRun) int {
		if order := b.started.Compare(a.started); order != 0 {
			return order
		}
		return strings.Compare(b.path, a.path)
	})
	return runs, nil
}

// Resolve remote tags once per operation, including annotated tags' peeled commits.
// A failed lookup must never be mistaken for an ordinary, untagged commit.
func releaseTags(parent context.Context) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "origin", "refs/tags/v*")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("release recognition unavailable; outputs preserved: %w", err)
	}
	refs := map[string]string{}
	peeled := map[string]string{}
	pattern := regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$`)
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		tag := strings.TrimPrefix(fields[1], "refs/tags/")
		name := strings.TrimSuffix(tag, "^{}")
		if !pattern.MatchString(name) {
			continue
		}
		if name != tag {
			peeled[name] = fields[0]
		} else {
			refs[name] = fields[0]
		}
	}
	maps.Copy(refs, peeled)
	tags := map[string]string{}
	for tag, commit := range refs {
		// A deterministic name when several version tags identify the same commit.
		if tag > tags[commit] {
			tags[commit] = tag
		}
	}
	return tags, nil
}

func recordReleaseComparison(parent, baselineTag, candidateTag string, a, b comparisonInput, c comparison) error {
	path := filepath.Join(filepath.Dir(parent), "release-history.md")
	const header = "# Quick-check release history\n"
	history := header
	data, err := os.ReadFile(path)
	if err == nil {
		if !strings.HasPrefix(string(data), header) {
			return fmt.Errorf("release history: missing Markdown title")
		}
		history = string(data)
	} else if !os.IsNotExist(err) {
		return err
	}
	text := strings.NewReplacer("\r", " ", "\n", " ", "`", "'", "|", "\\|").Replace
	key := fmt.Sprintf("- Commits: `%s` → `%s`\n- Cell: %s\n", a.summary.ServerRevision, b.summary.ServerRevision, text(c.Cell))
	var entry strings.Builder
	fmt.Fprintf(&entry, "%s → %s\n\n%s", text(baselineTag), text(candidateTag), key)
	fmt.Fprintf(&entry, "- Comparison: **%s**; saved runs: baseline **%s** / candidate **%s**\n", c.Status, a.summary.Status, b.summary.Status)
	for i, run := range []runSummary{a.summary, b.summary} {
		label := []string{"Baseline", "Candidate"}[i]
		fmt.Fprintf(&entry, "- %s: %s; model: `%s`; workload: `%s`; library: `%s`\n", label, text(run.TestStartedAt), text(run.Model), text(run.Workload), text(fmt.Sprint(run.InstalledLibrary)))
		fmt.Fprintf(&entry, "- %s machine: `%s`; RAM: %v bytes; VRAM: %v bytes; unified memory: %v\n", label,
			text(fmt.Sprint(run.Machine["host"])), run.Machine["system_ram_bytes"], run.Machine["gpu_total_bytes"], run.Machine["unified_memory"])
	}
	for _, warning := range c.Warnings {
		if strings.Contains(warning, "machine") || strings.Contains(warning, "missing metadata") {
			entry.WriteString("\n**WARNING: MACHINE METADATA DIFFERS OR IS MISSING!**\n")
			break
		}
	}
	format := func(value *measurement) string {
		if value == nil {
			return "unavailable"
		}
		sd := "unavailable"
		if value.SampleStdDev != nil {
			sd = fmt.Sprintf("%.6f", *value.SampleStdDev)
		}
		return fmt.Sprintf("%.6f [SD %s; n %d]", value.Median, sd, value.Samples)
	}
	entry.WriteString("\n| Seconds | Baseline median [SD; n] | Candidate median [SD; n] | Change |\n| --- | ---: | ---: | ---: |\n")
	for _, metric := range c.Metrics {
		label := map[string]string{"workload.elapsed_seconds": "Warmed workload", "warmup.elapsed_seconds": "Warm-up request"}[metric.Metric]
		if label != "" {
			change := "unavailable"
			if metric.Percent != nil {
				change = fmt.Sprintf("%+.2f%%", *metric.Percent)
			}
			fmt.Fprintf(&entry, "| %s | %s | %s | %s |\n", label, format(metric.Baseline), format(metric.Candidate), change)
		}
	}
	// Keep the latest comparison for each ordered release pair and cell.
	sections := strings.Split(strings.TrimSpace(history), "\n## ")
	sections = slices.DeleteFunc(sections, func(section string) bool { return strings.Contains(section, "\n"+key) })
	sections = append(sections, entry.String())
	data = []byte(strings.Join(sections, "\n## "))
	f, err := os.CreateTemp(filepath.Dir(path), ".release-history-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(append(data, '\n'))
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

// Keep three distinct runs: fresh, most recent older release, and newest remaining
// older commit. A fresh release only becomes the release baseline on a later run.
func retainRuns(ctx context.Context, parent, current string) (string, error) {
	runs, err := savedRuns(parent)
	if err != nil {
		return current, err
	}
	tags, err := releaseTags(ctx)
	if err != nil {
		return current, err
	}
	var candidate storedRun
	var previous []storedRun
	for _, run := range runs {
		if run.path == current {
			candidate = run
		} else {
			previous = append(previous, run)
		}
	}
	if candidate.path == "" {
		return current, fmt.Errorf("current run is not finalized: %s", current)
	}
	var release, older storedRun
	for _, run := range previous {
		if candidate.report.serverVersion() != "unknown" && run.report.serverVersion() == candidate.report.serverVersion() {
			continue
		}
		if tags[run.report.ServerRevision] != "" || strings.HasPrefix(filepath.Base(run.path), "release-") {
			release = run
			break
		}
	}
	for _, run := range previous {
		version := run.report.serverVersion()
		if (version != "unknown" && version == candidate.report.serverVersion()) || run.path == release.path || (version != "unknown" && version == release.report.serverVersion()) {
			continue
		}
		older = run
		break
	}
	short := func(run storedRun) string {
		if version := run.report.serverVersion(); version != "unknown" {
			return version[:7]
		}
		return "unknown"
	}
	selected := []storedRun{candidate}
	names := []string{"new-" + short(candidate)}
	if release.path != "" {
		tag := tags[release.report.ServerRevision]
		if tag == "" {
			tag = strings.TrimPrefix(filepath.Base(release.path), "release-")
		}
		selected = append(selected, release)
		names = append(names, "release-"+tag)
	}
	if older.path != "" {
		selected = append(selected, older)
		names = append(names, "old-"+short(older))
	}
	for _, name := range names {
		path := filepath.Join(parent, name)
		if _, err := os.Lstat(path); err == nil && !slices.ContainsFunc(runs, func(run storedRun) bool { return run.path == path }) {
			return current, fmt.Errorf("refusing to replace unmanaged output %s", path)
		} else if err != nil && !os.IsNotExist(err) {
			return current, err
		}
	}
	// Stage retained directories before changing roles, so role swaps cannot
	// overwrite another retained run. On I/O errors leave staged evidence intact.
	// Once this short filesystem commit starts, finish it even on interruption.
	if err := ctx.Err(); err != nil {
		return current, err
	}
	staged, err := os.MkdirTemp(parent, ".retain-*")
	if err != nil {
		return current, err
	}
	for i, run := range selected {
		if err := os.Rename(run.path, filepath.Join(staged, names[i])); err != nil {
			return current, fmt.Errorf("retention failed; retained evidence under %s: %w", staged, err)
		}
	}
	for _, run := range runs {
		if !slices.ContainsFunc(selected, func(kept storedRun) bool { return kept.path == run.path }) {
			if err := os.RemoveAll(run.path); err != nil {
				return current, fmt.Errorf("retention failed; retained evidence under %s: %w", staged, err)
			}
		}
	}
	for _, name := range names {
		if err := os.Rename(filepath.Join(staged, name), filepath.Join(parent, name)); err != nil {
			return current, fmt.Errorf("retention failed; retained evidence under %s: %w", staged, err)
		}
	}
	return filepath.Join(parent, names[0]), os.Remove(staged)
}
