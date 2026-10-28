package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	report  matrixReport
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
		var r matrixReport
		if json.Unmarshal(data, &r) != nil || (r.Status != "PASS" && r.Status != "FAIL" && r.Status != "INTERRUPTED") {
			continue
		}
		if len(r.Cells) == 0 {
			return nil, fmt.Errorf("%s: missing matrix cells; archive older artifacts and rerun quick-check", path)
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

func recordReleaseComparison(parent, baselineTag, candidateTag string, a, b matrixInput, c matrixComparison) error {
	// The compact history has no diagnostics or room for two machine descriptions.
	// Keep failed comparisons and mismatched/unknown machines in comparison.log only.
	host, _ := b.summary.Machine["host"].(string)
	ram, vram := eventInt(b.summary.Machine, "system_ram_bytes"), eventInt(b.summary.Machine, "gpu_total_bytes")
	unified, knownUnified := b.summary.Machine["unified_memory"].(bool)
	if c.Status != "PASS" || !reflect.DeepEqual(a.summary.Machine, b.summary.Machine) || host == "" || ram <= 0 || vram <= 0 || !knownUnified {
		return nil
	}
	path := filepath.Join(filepath.Dir(parent), "release-history.log")
	const title = "Release comparison: "
	const referenceIDLabel = "; reference ID: "
	var sections []string
	data, err := os.ReadFile(path)
	if err == nil {
		if !strings.HasPrefix(string(data), title) {
			return fmt.Errorf("release history: missing release comparison title")
		}
		sections = strings.Split(strings.TrimSpace(strings.TrimPrefix(string(data), title)), "\n\n"+title)
		for _, section := range sections {
			lines := strings.Split(section, "\n")
			if len(lines) < 6 || !strings.HasPrefix(lines[1], "machine: ") || !strings.HasPrefix(lines[2], "References updated: ") || !strings.Contains(lines[2], referenceIDLabel) || !strings.Contains(lines[4], "Baseline median [SD]") {
				return fmt.Errorf("release history: invalid comparison entry")
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	text := strings.NewReplacer("\r", " ", "\n", " ").Replace
	gib := func(bytes int) string {
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", float64(bytes)/(1<<30)), "0"), ".") + "GiB"
	}
	machine := fmt.Sprintf("machine: %s; RAM: %s; VRAM: %s; unified memory: %t", text(host), gib(ram), gib(vram), unified)
	references := make(map[string]normalizationReference)
	for _, cell := range c.Cells {
		if cell.Status != "NOT_RUN" {
			references[cell.Cell] = b.cells[cell.Cell].summary.ReferenceTokens
		}
	}
	// JSON sorts map keys; arrival order and repetitions cannot change this ID.
	data, err = json.Marshal(references)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	referenceID := hex.EncodeToString(digest[:])[:16]
	previousID := "unknown"
	for _, section := range sections {
		lines := strings.Split(section, "\n")
		if lines[1] == machine {
			_, previousID, _ = strings.Cut(lines[2], referenceIDLabel)
		}
	}
	updated := "unknown"
	if previousID != "unknown" && previousID != referenceID {
		updated = "yes"
	} else if previousID == referenceID {
		updated = "no"
	}
	key := fmt.Sprintf("%s → %s\n%s\n", text(baselineTag), text(candidateTag), machine)
	var entry strings.Builder
	entry.WriteString(key)
	fmt.Fprintf(&entry, "References updated: %s%s%s\n", updated, referenceIDLabel, referenceID)
	if err := c.writeTable(&entry); err != nil {
		return err
	}
	// One complete table per ordered release pair and machine. Move updated
	// entries to the end so the next comparison sees the latest reference ID.
	sections = slices.DeleteFunc(sections, func(section string) bool { return strings.HasPrefix(section, key) })
	sections = append(sections, strings.TrimSpace(entry.String()))
	data = []byte(title + strings.Join(sections, "\n\n"+title))
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

// Keep the fresh candidate, the most recent previous PASS release, and the newest
// remaining PASS commit. Only a PASS candidate replaces baselines of its commit;
// failed/interrupted candidates preserve them. Baseline commits stay distinct.
// A fresh release only becomes the release baseline on a later run.
func retainRuns(ctx context.Context, parent, current string, tags map[string]string) (string, error) {
	runs, err := savedRuns(parent)
	if err != nil {
		return current, err
	}
	var candidate storedRun
	var previous []storedRun
	for _, run := range runs {
		if run.path == current {
			candidate = run
		} else if run.report.Status == "PASS" {
			previous = append(previous, run)
		}
	}
	if candidate.path == "" {
		return current, fmt.Errorf("current run is not finalized: %s", current)
	}
	var release, older storedRun
	for _, run := range previous {
		if candidate.report.Status == "PASS" && candidate.report.serverVersion() != "unknown" && run.report.serverVersion() == candidate.report.serverVersion() {
			continue
		}
		if tags[run.report.ServerRevision] != "" || strings.HasPrefix(filepath.Base(run.path), "release-") {
			release = run
			break
		}
	}
	for _, run := range previous {
		version := run.report.serverVersion()
		if (candidate.report.Status == "PASS" && version != "unknown" && version == candidate.report.serverVersion()) || run.path == release.path || (version != "unknown" && version == release.report.serverVersion()) {
			continue
		}
		older = run
		break
	}
	label := func(run storedRun) string {
		if version := run.report.serverVersion(); version != "unknown" {
			if tag := tags[run.report.ServerRevision]; tag != "" {
				return tag + "-" + version[:7]
			}
			return version[:7]
		}
		return "unknown"
	}
	selected := []storedRun{candidate}
	names := []string{"new-" + label(candidate)}
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
		names = append(names, "old-"+label(older))
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
