package main

import (
	"fmt"
	"strings"
)

// Only implemented cells belong here. Adding a workload also adds its explicit
// NOT_RUN entry to reports made with a subset of the matrix selected.
var matrixCells = []struct {
	id         string
	images     bool
	targetOnly bool
	noCache    bool
	twoSlots   bool
}{
	{"MoE/1s/IMC+/MTP+/img-", false, false, false, false},
	{"MoE/1s/IMC+/MTP+/img+", true, false, false, false},
	{"MoE/1s/IMC+/MTP-/img-", false, true, false, false},
	{"MoE/1s/IMC+/MTP-/img+", true, true, false, false},
	{"MoE/1s/IMC-/MTP+/img-", false, false, true, false},
	{"MoE/1s/IMC-/MTP+/img+", true, false, true, false},
	{"MoE/1s/IMC-/MTP-/img-", false, true, true, false},
	{"MoE/1s/IMC-/MTP-/img+", true, true, true, false},
	{"MoE/2s/IMC+/MTP+/img-", false, false, false, true},
	{"MoE/2s/IMC+/MTP+/img+", true, false, false, true},
	{"MoE/2s/IMC+/MTP-/img-", false, true, false, true},
	{"MoE/2s/IMC+/MTP-/img+", true, true, false, true},
	{"MoE/2s/IMC-/MTP+/img-", false, false, true, true},
	{"MoE/2s/IMC-/MTP+/img+", true, false, true, true},
	{"MoE/2s/IMC-/MTP-/img-", false, true, true, true},
	{"MoE/2s/IMC-/MTP-/img+", true, true, true, true},
}

func selectedCells(selection string) (map[string]bool, error) {
	selected := make(map[string]bool)
	for name := range strings.SplitSeq(selection, ",") {
		name = strings.TrimSpace(name)
		if name == "one-slot" || name == "two-slots" {
			for _, cell := range matrixCells {
				if cell.twoSlots == (name == "two-slots") {
					selected[cell.id] = true
				}
			}
			continue
		}
		if base, ok := strings.CutSuffix(name, "-2s"); ok {
			cells, err := selectedCells(base)
			if err != nil {
				return nil, err
			}
			for id := range cells {
				selected[strings.Replace(id, "/1s/", "/2s/", 1)] = true
			}
			continue
		}
		switch name {
		case "all", "":
			for _, cell := range matrixCells {
				selected[cell.id] = true
			}
		case "text":
			selected[matrixCells[0].id] = true
		case "images":
			selected[matrixCells[1].id] = true
		case "text-no-mtp":
			selected[matrixCells[2].id] = true
		case "images-no-mtp":
			selected[matrixCells[3].id] = true
		case "no-mtp":
			selected[matrixCells[2].id] = true
			selected[matrixCells[3].id] = true
		case "text-no-imc":
			selected[matrixCells[4].id] = true
		case "images-no-imc":
			selected[matrixCells[5].id] = true
		case "text-no-imc-no-mtp":
			selected[matrixCells[6].id] = true
		case "images-no-imc-no-mtp":
			selected[matrixCells[7].id] = true
		case "no-imc":
			for _, cell := range matrixCells {
				if cell.noCache && !cell.twoSlots {
					selected[cell.id] = true
				}
			}
		default:
			return nil, fmt.Errorf("unknown cell selector %q; use all, one-slot, two-slots, text, images, text-no-mtp, images-no-mtp, no-mtp, text-no-imc, images-no-imc, text-no-imc-no-mtp, images-no-imc-no-mtp, no-imc; append -2s for two-slot variants or combine with commas", name)
		}
	}
	return selected, nil
}

func (r report) phases() []string {
	if r.TwoSlots {
		return []string{"cold", "replay", "fresh-session", "append", "peer-append", "survivor", "cancel", "recovery"}
	}
	return []string{"cold", "replay", "append", "fresh-session", "cancel", "recovery"}
}

type matrixReport struct {
	Status           string   `json:"status"`
	ServerRevision   string   `json:"server_revision"`
	ServerCommitTime string   `json:"server_commit_time"`
	ReleaseTag       string   `json:"release_tag,omitempty"`
	TestStartedAt    string   `json:"test_started_at"`
	Findings         []string `json:"findings"`
	Cells            []report `json:"cells"`
}

type matrixSummary struct {
	Machine          map[string]any `json:"machine"`
	ServerVersion    string         `json:"server_version"`
	ServerRevision   string         `json:"server_revision"`
	ServerCommitTime string         `json:"server_commit_time"`
	ReleaseTag       string         `json:"release_tag,omitempty"`
	TestStartedAt    string         `json:"test_started_at"`
	InstalledLibrary any            `json:"installed_library"`
	Status           string         `json:"status"`
	Findings         []string       `json:"findings"`
	Cells            []runSummary   `json:"cells"`
}

func (r matrixReport) serverVersion() string {
	return (report{ServerRevision: r.ServerRevision}).serverVersion()
}

func (r matrixReport) summary() matrixSummary {
	s := matrixSummary{
		ServerVersion: r.serverVersion(), ServerRevision: r.ServerRevision,
		ServerCommitTime: r.ServerCommitTime, ReleaseTag: r.ReleaseTag,
		TestStartedAt: r.TestStartedAt, Status: r.Status,
		Findings: append([]string{}, r.Findings...), Cells: []runSummary{},
	}
	for _, cell := range r.Cells {
		cs := cell.summary()
		if s.Machine == nil && cell.Status != "NOT_RUN" {
			s.Machine, s.InstalledLibrary = cs.Machine, cs.InstalledLibrary
		}
		s.Cells = append(s.Cells, cs)
	}
	return s
}

func (r matrixReport) logSummary() string {
	version := r.serverVersion()
	if version != "unknown" {
		version = version[:7]
	}
	if r.ReleaseTag != "" {
		version += " (release " + r.ReleaseTag + ")"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Quick-check: %s\nTested commit: %s from %s\nTest started: %s\n", r.Status, version, r.ServerCommitTime, r.TestStartedAt)
	for _, cell := range r.Cells {
		if cell.Status == "NOT_RUN" {
			fmt.Fprintf(&b, "Cell: %s | NOT_RUN\n", cell.ID)
			continue
		}
		b.WriteString(cell.logSummary())
	}
	if len(r.Findings) > 0 {
		fmt.Fprintln(&b, "Details: summary.json (run findings)")
	}
	return b.String()
}
