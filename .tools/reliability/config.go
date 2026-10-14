package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type config struct {
	Host        string
	Token       string `json:"-"`
	Out         string
	ServerLog   string
	Scenarios   []string
	Repetitions int
	RunLabel    string
	Timeout     time.Duration
	Seed        int64
	MTP         string
	IMC         string
	Matrix      []modeCell

	MTPEmbeddedModel  string
	MTPCompanionModel string
	MTPProfile        string
	MTPRequests       int
	MTPPromptTokens   int
	MTPMaxTokens      int
	MTPEmbeddedSlots  int
	MTPCompanionSlots int

	HybridModel string

	LongContextModel     string
	LongContextStages    []int
	LongContextMaxTokens int

	BatchModel         string
	BatchTurns         int
	BatchTargetTokens  int
	BatchTokensPerTurn int
	BatchMaxTokens     int
	BatchSlots         int
	BatchConversations int
	BatchProfile       string
	BatchImages        []string
	BatchImageSubjects []string

	MediaModel               string
	MediaProfile             string
	MediaImage               string
	MediaExpectedTerms       []string
	MediaMaxTokens           int
	MediaGenerationMaxTokens int
	MediaImageMaxTokens      int
	MediaMaxGenerationGap    time.Duration
}

func parseConfig(args []string) (config, bool, error) {
	fs := newFlagSet()
	home, _ := os.UserHomeDir()
	cfg := config{}
	var scenarios string
	var stages string
	var expectedTerms string
	var matrix string
	var batchImages string
	var batchSubjects string
	var list bool

	fs.StringVar(&scenarios, "scenario", "all", "comma-separated scenarios: mtp, hybrid-state, long-context, batch, media, or all")
	fs.BoolVar(&list, "list", false, "list scenario names and exit")
	fs.IntVar(&cfg.Repetitions, "repetitions", 1, "repeat the selected matrix; each repetition reloads models")
	fs.StringVar(&cfg.RunLabel, "run-label", "", "operator-supplied version/environment label (not verified)")
	fs.StringVar(&cfg.Host, "host", envOr("KRONK_WEB_API_HOST", "http://localhost:11435"), "Kronk server base URL")
	fs.StringVar(&cfg.Out, "out", ".tools/reliability/output", "replaceable artifact directory")
	fs.StringVar(&cfg.ServerLog, "server-log", filepath.Join(home, ".kronk", "kronk.log"), "detached Kronk JSON log")
	fs.DurationVar(&cfg.Timeout, "timeout", 30*time.Minute, "per-request timeout")
	fs.Int64Var(&cfg.Seed, "seed", 42, "deterministic sampling seed")
	fs.StringVar(&cfg.MTP, "mtp", "on", "expected MTP mode: on or off (does not change the server)")
	fs.StringVar(&cfg.IMC, "imc", "on", "expected IMC mode: on or off (does not change the server)")
	fs.StringVar(&matrix, "matrix", "", "matrix cells: all or comma-separated labeled pairs (MTPon/IMCon,MTPon/IMCoff,MTPoff/IMCon,MTPoff/IMCoff); empty uses -mtp/-imc")

	fs.StringVar(&cfg.MTPEmbeddedModel, "mtp-embedded-model", "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL", "embedded-MTP base model or TOOLS-MTP-* alias; custom alias allowed for one cell")
	fs.StringVar(&cfg.MTPCompanionModel, "mtp-companion-model", "unsloth/Qwen3.8-27B-UD-Q4_K_XL", "companion own-KV MTP base model or TOOLS-MTP-* alias; custom alias allowed for one cell")
	fs.StringVar(&cfg.MTPProfile, "mtp-profile", "all", "MTP profile: embedded, companion, or all")
	fs.IntVar(&cfg.MTPRequests, "mtp-requests", 0, "concurrent MTP requests (0 uses every configured slot)")
	fs.IntVar(&cfg.MTPPromptTokens, "mtp-prompt-tokens", 2200, "calibrated MTP prompt tokens")
	fs.IntVar(&cfg.MTPMaxTokens, "mtp-max-tokens", 256, "maximum MTP completion tokens")
	fs.IntVar(&cfg.MTPEmbeddedSlots, "mtp-embedded-slots", 4, "expected embedded-MTP slots")
	fs.IntVar(&cfg.MTPCompanionSlots, "mtp-companion-slots", 2, "expected companion-MTP slots")

	fs.StringVar(&cfg.HybridModel, "hybrid-model", "unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL", "single-slot hybrid base model or TOOLS-MTP-* alias; custom alias allowed for one cell")

	fs.StringVar(&cfg.LongContextModel, "long-context-model", "unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL", "long-context base model or TOOLS-MTP-* alias; custom alias allowed for one cell")
	fs.StringVar(&stages, "long-context-stages", "4096,8192,16384,32768,65536,131072", "comma-separated long-context token targets")
	fs.IntVar(&cfg.LongContextMaxTokens, "long-context-max-tokens", 96, "maximum long-context completion tokens")

	fs.StringVar(&cfg.BatchModel, "batch-model", "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL", "multi-slot batch base model or TOOLS-MTP-* alias; custom alias allowed for one cell")
	fs.IntVar(&cfg.BatchTurns, "batch-turns", 21, "synchronized turns per conversation")
	fs.IntVar(&cfg.BatchTargetTokens, "batch-target-tokens", 30000, "minimum final prompt tokens")
	fs.IntVar(&cfg.BatchTokensPerTurn, "batch-tokens-per-turn", 1400, "target tokens added per turn")
	fs.IntVar(&cfg.BatchMaxTokens, "batch-max-tokens", 128, "maximum batch completion tokens")
	fs.IntVar(&cfg.BatchSlots, "batch-slots", 4, "expected generation slots")
	fs.IntVar(&cfg.BatchConversations, "batch-conversations", 5, "conversations; values above slots exercise queue pressure")
	fs.StringVar(&cfg.BatchProfile, "batch-profile", "all", "batch profile: text, images, or all")
	fs.StringVar(&batchImages, "batch-images", "examples/samples/giraffe.jpg,examples/samples/adetailer-face.png", "comma-separated distinct batch image fixtures")
	fs.StringVar(&batchSubjects, "batch-image-subjects", "giraffe,astronaut", "comma-separated expected subject per batch image, in matching order")

	fs.StringVar(&cfg.MediaModel, "media-model", "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL", "multimodal base model or TOOLS-MTP-* alias; custom alias allowed for one cell")
	fs.StringVar(&cfg.MediaProfile, "media-profile", "all", "media profile: correctness, prefill, or all")
	fs.StringVar(&cfg.MediaImage, "media-image", "examples/samples/giraffe.jpg", "probe image")
	fs.StringVar(&expectedTerms, "media-expect", "giraffe", "comma-separated expected subject terms")
	fs.IntVar(&cfg.MediaMaxTokens, "media-max-tokens", 128, "maximum media correctness completion tokens")
	fs.IntVar(&cfg.MediaGenerationMaxTokens, "media-generation-max-tokens", 512, "maximum concurrent text completion tokens")
	fs.IntVar(&cfg.MediaImageMaxTokens, "media-image-max-tokens", 64, "maximum concurrent image completion tokens")
	fs.DurationVar(&cfg.MediaMaxGenerationGap, "media-max-generation-gap", time.Second, "largest allowed text event gap during media prefill")

	if err := fs.Parse(args); err != nil {
		return config{}, false, err
	}
	if fs.NArg() != 0 {
		return config{}, false, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	selected, err := parseScenarios(scenarios)
	if err != nil {
		return config{}, false, err
	}
	cfg.Scenarios = selected
	cfg.Token = strings.TrimSpace(os.Getenv("KRONK_TOKEN"))
	cfg.Host = strings.TrimRight(strings.TrimSpace(cfg.Host), "/")
	if !strings.Contains(cfg.Host, "://") {
		cfg.Host = "http://" + cfg.Host
	}
	cfg.LongContextStages, err = parsePositiveInts(stages)
	if err != nil {
		return config{}, false, fmt.Errorf("-long-context-stages: %w", err)
	}
	cfg.MediaExpectedTerms = splitNonempty(expectedTerms)
	cfg.BatchImages = splitNonempty(batchImages)
	cfg.BatchImageSubjects = splitNonempty(batchSubjects)
	conflictingMode := false
	fs.Visit(func(f *flag.Flag) {
		conflictingMode = conflictingMode || f.Name == "mtp" || f.Name == "imc"
	})
	if matrix != "" && conflictingMode {
		return config{}, false, errors.New("use -matrix or -mtp/-imc, not both")
	}
	if matrix == "" {
		matrix = "MTP" + cfg.MTP + "/IMC" + cfg.IMC
	}
	cfg.Matrix, err = parseMatrix(matrix)
	if err != nil {
		return config{}, false, err
	}
	cfg, err = cfg.forCell(cfg.Matrix[0])
	if err != nil {
		return config{}, false, err
	}

	if list {
		return cfg, true, nil
	}
	if err := cfg.validate(); err != nil {
		return config{}, false, err
	}
	return cfg, false, nil
}

type modeCell struct {
	MTP string `json:"mtp"`
	IMC string `json:"imc"`
}

func (cell modeCell) id() string { return "mtp-" + cell.MTP + "-imc-" + cell.IMC }

func parseMatrix(value string) ([]modeCell, error) {
	if value == "all" {
		value = "MTPon/IMCon,MTPon/IMCoff,MTPoff/IMCon,MTPoff/IMCoff"
	}
	var cells []modeCell
	seen := make(map[string]bool)
	for item := range strings.SplitSeq(value, ",") {
		parts := strings.Split(strings.TrimSpace(item), "/")
		if len(parts) != 2 || (parts[0] != "MTPon" && parts[0] != "MTPoff") || (parts[1] != "IMCon" && parts[1] != "IMCoff") {
			return nil, fmt.Errorf("invalid matrix cell %q: use MTPon/IMCon, MTPon/IMCoff, MTPoff/IMCon, or MTPoff/IMCoff", item)
		}
		cell := modeCell{MTP: strings.TrimPrefix(parts[0], "MTP"), IMC: strings.TrimPrefix(parts[1], "IMC")}
		if !seen[cell.id()] {
			cells = append(cells, cell)
			seen[cell.id()] = true
		}
	}
	return cells, nil
}

func (cfg config) forCell(cell modeCell) (config, error) {
	cfg.MTP, cfg.IMC = cell.MTP, cell.IMC
	selected := cfg.selectedModels()
	for _, id := range []*string{&cfg.MTPEmbeddedModel, &cfg.MTPCompanionModel, &cfg.HybridModel, &cfg.LongContextModel, &cfg.BatchModel, &cfg.MediaModel} {
		if !slices.Contains(selected, *id) {
			continue
		}
		parts := strings.Split(*id, "/")
		if len(parts) == 2 || len(parts) == 3 && strings.HasPrefix(parts[2], "TOOLS-MTP-") {
			*id = strings.Join(parts[:2], "/") + "/TOOLS-MTP-" + strings.ToUpper(cell.MTP) + "-IMC-" + strings.ToUpper(cell.IMC)
		} else if len(cfg.Matrix) > 1 {
			return config{}, fmt.Errorf("matrix model %q must use provider/model or a TOOLS-MTP-* profile", *id)
		}
	}
	return cfg, nil
}

func (cfg config) selectedModels() []string {
	var ids []string
	for _, scenario := range cfg.Scenarios {
		switch scenario {
		case "mtp":
			if cfg.MTPProfile != "companion" {
				ids = append(ids, cfg.MTPEmbeddedModel)
			}
			if cfg.MTPProfile != "embedded" {
				ids = append(ids, cfg.MTPCompanionModel)
			}
		case "hybrid-state":
			ids = append(ids, cfg.HybridModel)
		case "long-context":
			ids = append(ids, cfg.LongContextModel)
		case "batch":
			ids = append(ids, cfg.BatchModel)
		case "media":
			ids = append(ids, cfg.MediaModel)
		}
	}
	return ids
}

func (cfg config) validate() error {
	if cfg.Repetitions < 1 {
		return errors.New("-repetitions must be positive")
	}
	if (cfg.MTP != "on" && cfg.MTP != "off") || (cfg.IMC != "on" && cfg.IMC != "off") {
		return errors.New("-mtp and -imc must be on or off")
	}
	if cfg.Host == "" || cfg.Out == "" || cfg.ServerLog == "" {
		return errors.New("-host, -out, and -server-log must not be empty")
	}
	if cfg.Timeout <= 0 || cfg.MTPPromptTokens <= 0 || cfg.MTPMaxTokens <= 0 || cfg.MTPRequests < 0 {
		return errors.New("timeouts and MTP token counts must be positive; -mtp-requests must not be negative")
	}
	if cfg.Seed < 0 {
		return errors.New("-seed must not be negative")
	}
	if cfg.MTPProfile != "all" && cfg.MTPProfile != "embedded" && cfg.MTPProfile != "companion" {
		return errors.New("-mtp-profile must be embedded, companion, or all")
	}
	if cfg.MTPEmbeddedSlots <= 0 || cfg.MTPCompanionSlots <= 0 {
		return errors.New("MTP slot counts must be positive")
	}
	if cfg.LongContextMaxTokens <= 0 {
		return errors.New("-long-context-max-tokens must be positive")
	}
	if cfg.BatchTurns < 21 || cfg.BatchTargetTokens < 30000 || cfg.BatchTokensPerTurn <= 0 || cfg.BatchMaxTokens <= 0 {
		return errors.New("batch requires at least 21 turns and 30000 target tokens; per-turn and completion tokens must be positive")
	}
	if cfg.BatchSlots < 2 || cfg.BatchConversations <= cfg.BatchSlots {
		return errors.New("batch requires at least 2 slots and conversations greater than slots to exercise queue pressure")
	}
	if cfg.BatchProfile != "text" && cfg.BatchProfile != "images" && cfg.BatchProfile != "all" {
		return errors.New("-batch-profile must be text, images, or all")
	}
	if cfg.BatchProfile != "text" {
		if len(cfg.BatchImages) < 2 || len(cfg.BatchImages) != len(cfg.BatchImageSubjects) {
			return errors.New("image batch requires at least two images and one matching subject per image")
		}
		for index, subject := range cfg.BatchImageSubjects {
			for previous := range index {
				other := cfg.BatchImageSubjects[previous]
				if cfg.BatchImages[index] == cfg.BatchImages[previous] || strings.Contains(strings.ToLower(subject), strings.ToLower(other)) || strings.Contains(strings.ToLower(other), strings.ToLower(subject)) {
					return errors.New("batch images and their subject terms must be distinct and non-overlapping")
				}
			}
		}
	}
	if cfg.MediaMaxTokens <= 0 || cfg.MediaGenerationMaxTokens <= 0 || cfg.MediaImageMaxTokens <= 0 || cfg.MediaMaxGenerationGap <= 0 {
		return errors.New("media token counts and generation gap must be positive")
	}
	if cfg.MediaProfile != "all" && cfg.MediaProfile != "correctness" && cfg.MediaProfile != "prefill" {
		return errors.New("-media-profile must be correctness, prefill, or all")
	}
	if len(cfg.MediaExpectedTerms) == 0 {
		return errors.New("-media-expect must contain at least one term")
	}
	return nil
}

func parsePositiveInts(value string) ([]int, error) {
	var result []int
	seen := map[int]bool{}
	for _, item := range splitNonempty(value) {
		number, err := strconv.Atoi(item)
		if err != nil || number <= 0 {
			return nil, fmt.Errorf("%q is not a positive integer", item)
		}
		if !seen[number] {
			result = append(result, number)
			seen[number] = true
		}
	}
	if len(result) == 0 {
		return nil, errors.New("at least one value is required")
	}
	return result, nil
}

func splitNonempty(value string) []string {
	var result []string
	for item := range strings.SplitSeq(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
