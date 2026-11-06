package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

// Keep historical fixture cases tied to CSV rows without using the selector's
// own identity formatter to build its input or expected output.
func testCellIDs(t *testing.T, numbers string) string {
	t.Helper()
	f, err := os.Open("coverage.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for number := range strings.SplitSeq(numbers, ",") {
		n, err := strconv.Atoi(number)
		if err != nil || n < 2 || n > 65 {
			t.Fatalf("invalid single-GPU fixture row %q", number)
		}
		r := rows[n-1]
		state := map[string]string{"Pure": "Att", "Hybrid": "Hyb"}[r[6]]
		slots := map[string]string{"1": "1slot", "n": "Nslot"}[r[1]]
		imc := map[string]string{"Off": "IMC-", "On": "IMC+"}[r[2]]
		mtp := map[string]string{"Off": "MTP-", "On": "MTP+"}[r[3]]
		img := map[string]string{"No": "img-", "Yes": "img+"}[r[4]]
		ids = append(ids, strings.Join([]string{r[5], state, slots, imc, mtp, img}, "/"))
	}
	return strings.Join(ids, "\n")
}

func TestExplicitCellSyntax(t *testing.T) {
	const first = "MoE/Hyb/1slot/IMC-/MTP+/img-"
	const second = "Dense/Att/Nslot/IMC+/MTP-/img+"
	rows, err := selectCells("\n  " + first + "\n\t" + second + "\n" + first + "\n")
	if err != nil || !reflect.DeepEqual(rows, []int{13, 54}) {
		t.Fatalf("multiline selection: rows=%v err=%v", rows, err)
	}
	for _, value := range []string{"", "\n\t", "13", first + "," + second, "all\n" + first,
		"MoE/Hybrid/1slot/IMC-/MTP+/img-", "MoE/Hyb/1s/IMC-/MTP+/img-",
		"MoE/Hyb/1slot/IMC-/MTP+/img-/1gpu", "MoE/Hyb/1slot/IMC?/MTP+/img-"} {
		if _, err := selectCells(value); err == nil {
			t.Errorf("accepted invalid cells %q", value)
		}
	}
	for _, name := range []string{"hybrid", "media-correctness", "media-prefill"} {
		if _, err := selectProfiles(name); err == nil {
			t.Errorf("accepted retired profile %q", name)
		}
	}
}

func TestExplicitProfileCellSelection(t *testing.T) {
	const cell = "Dense/Att/Nslot/IMC-/MTP-/img-"
	for _, args := range [][]string{
		nil, {"-profiles", "all"}, {"-cells", "all"},
		{"-scenario", "batch", "-matrix", ""},
		{"-profiles", "", "-cells", cell},
		{"-profiles", "batch-text", "-cells", "1"},
		{"-profiles", "batch-text", "-cells", "130"},
		{"-profiles", "batch-text", "-cells", "34,"},
		{"-profiles", "all,batch-text", "-cells", cell},
		{"-profiles", "batch-text", "-cells", "all,34"},
		{"-profiles", "batch-text", "-cells", cell, "-matrix", "all"},
		{"-profiles", "batch-text", "-cells", cell, "-batch-slots", "2"},
	} {
		if _, _, err := parseConfig(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	for _, tc := range []struct {
		profiles, cells string
		count           int
	}{
		{"batch-text,batch-images", testCellIDs(t, "34,50,34"), 4},
		{"all", testCellIDs(t, "34"), 8}, {"batch-text", "all", 64}, {"all", "all", 512},
	} {
		cfg, _, err := parseConfig([]string{"-profiles", tc.profiles, "-cells", tc.cells})
		if err != nil {
			t.Fatal(err)
		}
		if got := len(executionPlan(cfg)); got != tc.count {
			t.Fatalf("%+v: got %d combinations", tc, got)
		}
	}
	for _, tc := range []struct{ profile, cell, scenario, model string }{
		{"long-context", "Dense/Att/1slot/IMC-/MTP-/img-", "long-context", "unsloth/Qwen3-0.6B-Q8_0/TOOLS-MTP-OFF-IMC-OFF"},
		{"batch-images", "MoE/Hyb/Nslot/IMC-/MTP-/img+", "batch", "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/TOOLS-MTP-OFF-IMC-OFF"},
	} {
		cfg, _, err := parseConfig([]string{"-profiles", tc.profile, "-cells", tc.cell})
		if err != nil {
			t.Fatal(err)
		}
		probe := executionPlan(cfg)[0]
		if probe.Status != "runnable" || probe.Scenario != tc.scenario || !reflect.DeepEqual(probe.Config.selectedModels(), []string{tc.model}) {
			t.Fatalf("lost required selection %s × %s: %+v", tc.profile, tc.cell, probe)
		}
		if tc.profile == "long-context" && !reflect.DeepEqual(probe.Config.LongContextStages, []int{4096, 8192, 16384}) || tc.profile == "batch-images" && probe.Config.BatchProfile != "images" {
			t.Fatalf("wrong workload for %s: %+v", tc.profile, probe.Config)
		}
	}
	cfg, _, err := parseConfig([]string{"-profiles", "batch-text", "-cells", testCellIDs(t, "34,50")})
	if err != nil {
		t.Fatal(err)
	}
	for i, probe := range executionPlan(cfg) {
		mode := []string{"OFF", "ON"}[i]
		want := "ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0/TOOLS-BATCH-MTP-OFF-IMC-" + mode
		if probe.Status != "runnable" || probe.Config.BatchModel != want || probe.Config.BatchTokensPerTurn != 1300 || probe.Config.BatchSlots != 4 || probe.Config.BatchConversations != 8 || probe.Config.ExpectedState != "Attention-only" {
			t.Fatalf("incorrect qualified batch fixture: %+v", probe)
		}
	}
	cfg, _, err = parseConfig([]string{"-profiles", "image-state,image-prefill", "-cells", testCellIDs(t, "6,22,38,54")})
	if err != nil {
		t.Fatal(err)
	}
	for _, probe := range executionPlan(cfg) {
		if probe.Profile == "image-prefill" && (probe.Cell == testCellIDs(t, "6") || probe.Cell == testCellIDs(t, "22")) {
			if probe.Status != "not apply" {
				t.Fatalf("one-slot prefill became runnable: %+v", probe)
			}
			continue
		}
		base := "unsloth/gemma-4-E4B-it-qat-UD-Q4_K_XL"
		if probe.Profile == "image-prefill" {
			base = "ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0"
		}
		suffix, slots := "/TOOLS", 2
		if probe.Cell == testCellIDs(t, "6") || probe.Cell == testCellIDs(t, "22") {
			suffix, slots = "/TOOLS-1S", 1
		}
		imc := "OFF"
		if probe.Cell == testCellIDs(t, "22") || probe.Cell == testCellIDs(t, "54") {
			imc = "ON"
		}
		if probe.Status != "runnable" || probe.Config.MediaModel != base+suffix+"-MTP-OFF-IMC-"+imc || probe.Config.MediaSlots != slots || probe.Config.ExpectedState != "Attention-only" {
			t.Fatalf("incorrect Attention-only image fixture: %+v", probe)
		}
	}
}

func TestCellIdentitiesAndRunnableFixtures(t *testing.T) {
	f, err := os.Open("coverage.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := parseConfig([]string{"-profiles", "all", "-cells", "all"})
	if err != nil {
		t.Fatal(err)
	}
	for i, profile := range cfg.Profiles {
		if want := fmt.Sprintf("%d) %s", i+1, profile); rows[0][9+i] != want {
			t.Fatalf("profile order differs from CSV: column %d = %q, want %q", i+1, rows[0][9+i], want)
		}
	}
	cells := coverageCells()
	if len(rows) != len(cells)+1 {
		t.Fatal("cell catalogue length differs from CSV")
	}
	for _, cell := range cells {
		state := cell.State
		if state == "Attention-only" {
			state = "Pure"
		}
		want := []string{cell.GPU, cell.Slots, cell.IMC, cell.MTP, cell.Media, cell.Type, state}
		got := append([]string(nil), rows[cell.Row-1][:7]...)
		got[2], got[3] = strings.ToLower(got[2]), strings.ToLower(got[3])
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("cell %d: CSV=%v catalogue=%v", cell.Row, got, want)
		}
		if cell.GPU == "1" {
			id := testCellIDs(t, strconv.Itoa(cell.Row))
			selected, err := selectCells(id)
			if err != nil || !reflect.DeepEqual(selected, []int{cell.Row}) || cell.id() != id {
				t.Fatalf("cell %d lost its explicit identity %q: selected=%v err=%v", cell.Row, id, selected, err)
			}
		}
	}
	doc, err := modelconfig.Load("../../zarf/kms/tools_model_config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, probe := range executionPlan(cfg) {
		if probe.Status != "runnable" {
			continue
		}
		for _, id := range probe.Config.selectedModels() {
			entry, ok := doc.Models[id]
			if !ok {
				t.Fatalf("%s × %s references missing alias %s", probe.Profile, probe.Cell, id)
			}
			if entry.PtrIncrementalCache == nil || *entry.PtrIncrementalCache != (probe.Config.IMC == "on") || (entry.Speculation.String() == "mtp") != (probe.Config.MTP == "on") {
				t.Fatalf("%s × %s has wrong modes: %s", probe.Profile, probe.Cell, id)
			}
			wantSlots := 1
			switch probe.Profile {
			case "mtp-embedded":
				wantSlots = probe.Config.MTPEmbeddedSlots
			case "mtp-companion":
				wantSlots = probe.Config.MTPCompanionSlots
			case "batch-text", "batch-images":
				wantSlots = probe.Config.BatchSlots
			case "image-state", "image-prefill":
				wantSlots = probe.Config.MediaSlots
			}
			if entry.PtrNSeqMax == nil || *entry.PtrNSeqMax != wantSlots {
				t.Fatalf("%s × %s: slots mismatch for %s", probe.Profile, probe.Cell, id)
			}
		}
	}
}

func TestAutomaticPressureMapping(t *testing.T) {
	for _, tc := range []struct {
		profile, cells string
		cycles, waves  int
	}{
		{"text-state", "2,5,18,21", 3, 1},
		{"image-state", "6,9,17,22,25,33,38,39,41,47,49,54,55,57,63,65", 3, 1},
		{"mtp-embedded", "5,13,21,29,37,45,53,61", 1, 3},
		{"mtp-companion", "3,11,19,27,35,43,51,59", 1, 3},
		{"batch-text", "34,37,50,53", 1, 1},
	} {
		cfg, _, err := parseConfig([]string{"-profiles", tc.profile, "-cells", testCellIDs(t, tc.cells)})
		if err != nil {
			t.Fatal(err)
		}
		for _, probe := range executionPlan(cfg) {
			if probe.Status != "runnable" || probe.Config.RecoveryCycles != tc.cycles || probe.Config.MTPWaves != tc.waves || probe.Config.MTPMinRollbackRounds != 0 {
				t.Fatalf("wrong pressure for %s × %s: %+v", tc.profile, probe.Cell, probe.Config)
			}
		}
	}
	cfg, _, err := parseConfig([]string{"-profiles", "image-prefill", "-cells", testCellIDs(t, "38,39,41,47,49,54,55,57,63,65")})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"isolated", "shared", "shared", "shared", "shared", "imc", "imc", "oversized-imc", "imc", "imc"}
	doc, err := modelconfig.Load("../../zarf/kms/tools_model_config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for i, probe := range executionPlan(cfg) {
		if probe.Status != "runnable" || probe.Config.MediaPrefillExecution != want[i] {
			t.Fatalf("wrong prefill mapping: %+v", probe)
		}
		if (len(probe.AdditionalPhases) == 1) != (probe.Cell == testCellIDs(t, "41")) {
			t.Fatalf("wrong phase expansion: %+v", probe)
		}
		for _, phase := range append([]config{probe.Config}, probe.AdditionalPhases...) {
			fixture, ok := doc.Models[phase.MediaModel]
			if !ok || fixture.PtrNSeqMax == nil || *fixture.PtrNSeqMax != phase.MediaSlots || fixture.PtrIncrementalCache == nil || *fixture.PtrIncrementalCache != (phase.IMC == "on") || (fixture.Speculation.String() == "mtp") != (phase.MTP == "on") {
				t.Fatalf("phase changed selected cell modes/slots: %+v", phase)
			}
			if phase.MediaRequireIsolatedPrefill && (fixture.PtrPrefillBatchSize == nil || fixture.PtrImageBatchTokenCapacity == nil || *fixture.PtrPrefillBatchSize <= 0 || *fixture.PtrImageBatchTokenCapacity <= *fixture.PtrPrefillBatchSize) {
				t.Fatalf("forced-isolation fixture needs decode capacity above its contribution limit: %+v", fixture)
			}
		}
	}
	// Explicit pressure overrides remain possible, including when profiles are mixed.
	cfg, _, err = parseConfig([]string{"-profiles", "mtp-embedded,text-state", "-cells", testCellIDs(t, "5"), "-recovery-cycles", "2", "-mtp-waves", "5"})
	if err != nil {
		t.Fatal(err)
	}
	plan := executionPlan(cfg)
	if plan[0].Config.MTPWaves != 5 || plan[0].Config.RecoveryCycles != 1 || plan[1].Config.RecoveryCycles != 2 || plan[1].Config.MTPWaves != 1 {
		t.Fatal("explicit pressure overrides lost or applied to unrelated profiles")
	}
}

func TestSelectedCommandReportsEveryCombination(t *testing.T) {
	for _, cells := range []string{"2,35", "34,2,35"} {
		t.Run(cells, func(t *testing.T) {
			requests := 0
			var loaded []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				switch r.URL.Path {
				case "/v1/kronk/models/ps":
					fmt.Fprint(w, "[]")
				case "/v1/chat/completions":
					var body struct {
						Model string `json:"model"`
					}
					json.NewDecoder(r.Body).Decode(&body)
					loaded = append(loaded, body.Model)
					http.Error(w, "deliberate fixture failure", http.StatusBadRequest)
				default:
					fmt.Fprint(w, "{}")
				}
			}))
			defer server.Close()
			dir := t.TempDir()
			old := os.Args
			defer func() { os.Args = old }()
			os.Args = []string{"reliability", "-profiles", "batch-text", "-cells", testCellIDs(t, cells), "-host", server.URL, "-out", filepath.Join(dir, "out"), "-server-log", filepath.Join(dir, "absent.log")}
			if err := run(); err == nil {
				t.Fatal("gap/failed selection returned success")
			}
			data, err := os.ReadFile(filepath.Join(dir, "out", "summary.json"))
			if err != nil {
				t.Fatal(err)
			}
			var summary runSummary
			if err := json.Unmarshal(data, &summary); err != nil {
				t.Fatal(err)
			}
			if summary.CoverageStatus != "INCOMPLETE" {
				t.Fatalf("gap not reported: %+v", summary)
			}
			want := map[string]string{"2": "NOT APPLY", "35": "GAP"}
			if strings.HasPrefix(cells, "34") {
				want["34"] = "FAIL"
			} else if requests != 0 || summary.Status != "NOT RUN" {
				t.Fatalf("nonrunnable selection contacted server or passed: requests=%d status=%s", requests, summary.Status)
			}
			if len(summary.Selection) != len(want) || len(summary.Scenarios) != len(want) {
				t.Fatal("omitted selected combinations")
			}
			statuses := make(map[string]string)
			for row, status := range want {
				statuses[testCellIDs(t, row)] = status
			}
			for _, result := range summary.Scenarios {
				if result.Status != statuses[result.Cell] {
					t.Fatalf("cell %s: status %s, want %s", result.Cell, result.Status, statuses[result.Cell])
				}
			}
			if _, ok := want["34"]; ok && !reflect.DeepEqual(loaded, []string{"ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0/TOOLS-BATCH-MTP-OFF-IMC-OFF"}) {
				t.Fatalf("wrong selected model requests: %v", loaded)
			}
			for _, selected := range summary.Selection {
				if _, ok := statuses[selected.Cell]; !ok {
					t.Fatalf("selection lost explicit identity: %s", selected.Cell)
				}
			}
		})
	}
}

func TestMakeMultilineCellSelection(t *testing.T) {
	const cells = "\n  MoE/Hyb/1slot/IMC-/MTP+/img-\n  Dense/Att/Nslot/IMC+/MTP-/img+\n"
	for _, tc := range []struct{ target, profiles string }{
		{"text-state", "text-state"}, {"image-state", "image-state"},
		{"long-context", "long-context"}, {"image-prefill", "image-prefill"},
		{"batch-text", "batch-text"}, {"batch-images", "batch-images"},
		{"mtp-embedded", "mtp-embedded"}, {"mtp-companion", "mtp-companion"},
		{"images", "image-state,image-prefill"},
	} {
		cmd := exec.Command("make", "-n", "-f", ".make/tools.mk", "test-load-"+tc.target, "RELIABILITY_CELLS="+cells)
		cmd.Dir = "../.."
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("make %s: %v\n%s", tc.target, err, output)
		}
		if !strings.Contains(string(output), `-profiles "`+tc.profiles+`"`) ||
			!strings.Contains(string(output), `-cells "MoE/Hyb/1slot/IMC-/MTP+/img- Dense/Att/Nslot/IMC+/MTP-/img+"`) {
			t.Fatalf("make lost the selected profile or multiline cells: %s", output)
		}
	}
	cmd := exec.Command("make", "-n", "-f", ".make/tools.mk", "test-load-text-state",
		"RELIABILITY_MATRIX=MTPoff/IMCon", "RELIABILITY_TEXT_STATE_MODEL=custom-text",
		"RELIABILITY_IMAGE_MODEL=custom-image", "RELIABILITY_IMAGE_STATE_MAX_TOKENS=137")
	cmd.Dir = "../.."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("custom Make fixtures: %v\n%s", err, output)
	}
	for _, want := range []string{`-scenario "text-state"`, `-text-state-model "custom-text"`, `-image-model "custom-image"`, `-image-state-max-tokens "137"`} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("renamed Make override lost %s: %s", want, output)
		}
	}
}
