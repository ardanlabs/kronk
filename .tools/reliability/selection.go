package main

import (
	"fmt"
	"slices"
	"strings"
)

var profileOrder = []string{"text-state", "image-state", "long-context", "image-prefill", "batch-text", "batch-images", "mtp-embedded", "mtp-companion"}

// Row is the coverage.csv line number (including its header). The axis order
// matches that table; fixture selection does not depend on its prose/statuses.
type coverageCell struct {
	Row                                      int
	GPU, Slots, IMC, MTP, Media, Type, State string
}

func (c coverageCell) id() string {
	state, slots, imc, mtp, image := "Att", "Nslot", "IMC-", "MTP-", "img-"
	if c.State == "Hybrid" {
		state = "Hyb"
	}
	if c.Slots == "1" {
		slots = "1slot"
	}
	if c.IMC == "on" {
		imc = "IMC+"
	}
	if c.MTP == "on" {
		mtp = "MTP+"
	}
	if c.Media == "Yes" {
		image = "img+"
	}
	return strings.Join([]string{c.Type, state, slots, imc, mtp, image}, "/")
}

func coverageCells() []coverageCell {
	var cells []coverageCell
	for _, gpu := range []string{"1", "n"} {
		for _, slots := range []string{"1", "n"} {
			for _, imc := range []string{"off", "on"} {
				for _, mtp := range []string{"off", "on"} {
					for _, media := range []string{"No", "Yes"} {
						for _, kind := range []string{"Dense", "MoE"} {
							for _, state := range []string{"Attention-only", "Hybrid"} {
								cells = append(cells, coverageCell{len(cells) + 2, gpu, slots, imc, mtp, media, kind, state})
							}
						}
					}
				}
			}
		}
	}
	return cells
}

type probeSelection struct {
	Profile          string   `json:"profile"`
	Cell             string   `json:"cell"`
	Status           string   `json:"status"`
	Reason           string   `json:"reason,omitempty"`
	Scenario         string   `json:"-"`
	Config           config   `json:"-"`
	AdditionalPhases []config `json:"-"`
}

func selectProfiles(value string) ([]string, error) {
	if value == "all" {
		return slices.Clone(profileOrder), nil
	}
	var selected []string
	for name := range strings.SplitSeq(value, ",") {
		name = strings.TrimSpace(name)
		if !slices.Contains(profileOrder, name) {
			return nil, fmt.Errorf("invalid profile %q: select %s, or all", name, strings.Join(profileOrder, ","))
		}
		if !slices.Contains(selected, name) {
			selected = append(selected, name)
		}
	}
	return selected, nil
}

func selectCells(value string) ([]int, error) {
	value = strings.TrimSpace(value)
	var rows []int
	if value == "all" {
		for _, cell := range coverageCells() {
			if cell.GPU == "1" {
				rows = append(rows, cell.Row)
			}
		}
		return rows, nil
	}
	for raw := range strings.FieldsSeq(value) {
		row := 0
		for _, cell := range coverageCells() {
			if cell.GPU == "1" && raw == cell.id() {
				row = cell.Row
				break
			}
		}
		if row == 0 {
			return nil, fmt.Errorf("invalid cell %q: use Routing/State/Slots/IMC/MTP/Images (e.g. MoE/Hyb/1slot/IMC-/MTP+/img-), one per line, or all; row numbers and GPU selectors are not supported", raw)
		}
		if !slices.Contains(rows, row) {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("select at least one explicit cell or all")
	}
	return rows, nil
}

func executionPlan(cfg config) []probeSelection {
	if len(cfg.Profiles) > 0 {
		var plan []probeSelection
		cells := coverageCells()
		for _, row := range cfg.Cells {
			for _, profile := range cfg.Profiles {
				plan = append(plan, selectProbe(cfg, cells[row-2], profile))
			}
		}
		return plan
	}
	var plan []probeSelection
	for _, cell := range cfg.Matrix {
		current, _ := cfg.forCell(cell)
		for _, name := range cfg.Scenarios {
			if name == "batch" && cfg.BatchProfile == "all" {
				for _, workload := range []string{"text", "images"} {
					batch := current
					batch.BatchProfile = workload
					plan = append(plan, probeSelection{Profile: "batch-" + workload, Cell: cell.id(), Status: "runnable", Scenario: "batch/" + workload, Config: batch})
				}
			} else {
				plan = append(plan, probeSelection{Profile: name, Cell: cell.id(), Status: "runnable", Scenario: name, Config: current})
			}
		}
	}
	return plan
}

func selectProbe(cfg config, cell coverageCell, profile string) probeSelection {
	p := probeSelection{Profile: profile, Cell: cell.id(), Status: "coverage gap", Reason: "no matching fixture/workload"}
	text := profile == "mtp-embedded" || profile == "mtp-companion" || profile == "text-state" || profile == "long-context" || profile == "batch-text"
	if text != (cell.Media == "No") || (strings.HasPrefix(profile, "batch-") || profile == "image-prefill") && cell.Slots == "1" || (profile == "text-state" || profile == "long-context") && cell.Slots != "1" {
		p.Status, p.Reason = "not apply", "workload does not exercise this input/slot path"
		return p
	}
	if cell.GPU != "1" {
		p.Reason = "multi-GPU fixtures are deferred"
		return p
	}
	cfg.MTP, cfg.IMC, cfg.ExpectedState = cell.MTP, cell.IMC, cell.State
	cfg.Matrix = []modeCell{{cell.MTP, cell.IMC}}
	if profile == "text-state" || profile == "image-state" {
		if !cfg.explicitFlags["recovery-cycles"] {
			cfg.RecoveryCycles = 3
		}
	} else {
		cfg.RecoveryCycles = 1
	}
	if profile == "mtp-embedded" || profile == "mtp-companion" {
		if !cfg.explicitFlags["mtp-waves"] {
			cfg.MTPWaves = 3
		}
	} else {
		cfg.MTPWaves, cfg.MTPMinRollbackRounds = 1, 0
	}
	mode := "-MTP-" + strings.ToUpper(cell.MTP) + "-IMC-" + strings.ToUpper(cell.IMC)
	embedded := "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL"
	companion := "unsloth/Qwen3.8-27B-UD-Q4_K_XL"
	vision := "ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0"
	base := ""
	suffix := "/TOOLS" + mode
	slots := 4
	if cell.Slots == "1" {
		slots = 1
	}
	switch profile {
	case "mtp-embedded", "mtp-companion":
		if cell.State != "Hybrid" {
			return p
		}
		if profile == "mtp-embedded" && cell.Type == "MoE" {
			base = embedded
			cfg.MTPProfile = "embedded"
		}
		if profile == "mtp-companion" && cell.Type == "Dense" {
			base = companion
			cfg.MTPProfile = "companion"
			if slots > 1 {
				slots = 2
			}
		}
		if slots == 1 {
			suffix = "/TOOLS-1S" + mode
		}
		cfg.MTPEmbeddedSlots, cfg.MTPCompanionSlots = slots, slots
		cfg.MTPEmbeddedModel, cfg.MTPCompanionModel = base+suffix, base+suffix
		p.Scenario = "mtp"
	case "text-state", "long-context":
		if cell.State == "Hybrid" && cell.Type == "MoE" {
			base = "unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL"
		}
		if cell.State == "Attention-only" && cell.Type == "Dense" && cell.MTP == "off" {
			base = "unsloth/Qwen3-0.6B-Q8_0"
			cfg.LongContextStages = []int{4096, 8192, 16384}
		}
		cfg.HybridModel, cfg.LongContextModel = base+suffix, base+suffix
		p.Scenario = "text-state"
		if profile == "long-context" {
			p.Scenario = "long-context"
		}
	case "batch-text", "batch-images":
		if cell.State == "Hybrid" && cell.Type == "MoE" {
			base = embedded
		}
		if profile == "batch-text" && cell.State == "Attention-only" && cell.Type == "Dense" && cell.MTP == "off" {
			base, suffix = vision, "/TOOLS-BATCH"+mode
			cfg.BatchTokensPerTurn = 1300
		}
		cfg.BatchModel = base + suffix
		cfg.BatchProfile = "text"
		if profile == "batch-images" {
			cfg.BatchProfile = "images"
		}
		p.Scenario = "batch"
	case "image-state", "image-prefill":
		if cell.State == "Hybrid" {
			if cell.Type == "MoE" {
				base = embedded
			} else {
				base = companion
				if slots > 1 {
					slots = 2
				}
			}
			if slots == 1 {
				suffix = "/TOOLS-1S" + mode
			}
		} else if cell.Type == "Dense" && cell.MTP == "off" {
			base = vision
			if profile == "image-state" {
				base = "unsloth/gemma-4-E4B-it-qat-UD-Q4_K_XL"
			}
			if slots == 1 {
				suffix = "/TOOLS-1S" + mode
			} else {
				slots = 2
			}
		}
		if cfg.MediaRequireIsolatedPrefill && profile == "image-prefill" && (base != embedded || cell.MTP != "off") {
			p.Reason = "no forced-isolation fixture for this path"
			return p
		}
		cfg.MediaModel, cfg.MediaSlots = base+suffix, slots
		cfg.MediaProfile = "state"
		if profile == "image-prefill" {
			cfg.MediaProfile = "prefill"
			cfg.MediaPrefillExecution = "shared"
			if base == vision {
				cfg.MediaPrefillExecution = "isolated"
			}
			if cell.IMC == "on" {
				cfg.MediaPrefillExecution = "imc"
			}
			if cell.Row == 57 || cfg.MediaRequireIsolatedPrefill {
				cfg.MediaModel, cfg.MediaSlots = embedded+"/TOOLS-ISOLATED"+mode, 2
				cfg.MediaRequireIsolatedPrefill = true
				cfg.MediaPrefillExecution = "oversized-isolated"
				if cell.IMC == "on" {
					cfg.MediaPrefillExecution = "oversized-imc"
				}
			}
			if cell.Row == 41 && !cfg.explicitFlags["image-require-isolated-prefill"] {
				isolated := cfg
				isolated.MediaModel, isolated.MediaSlots = embedded+"/TOOLS-ISOLATED"+mode, 2
				isolated.MediaRequireIsolatedPrefill = true
				isolated.MediaPrefillExecution = "oversized-isolated"
				isolated.Scenarios = []string{"images"}
				p.AdditionalPhases = []config{isolated}
			}
		}
		p.Scenario = "images"
	}
	if base == "" {
		return p
	}
	cfg.Scenarios = []string{p.Scenario}
	p.Status, p.Reason, p.Config = "runnable", "", cfg
	return p
}
