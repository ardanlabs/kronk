package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type config struct {
	Host      string
	Token     string
	Out       string
	ServerLog string
	Scenarios []string
	Timeout   time.Duration
	Seed      int64

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

	MediaModel               string
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
	var list bool

	fs.StringVar(&scenarios, "scenario", "all", "comma-separated scenarios: mtp, hybrid-state, long-context, batch, media, or all")
	fs.BoolVar(&list, "list", false, "list scenario names and exit")
	fs.StringVar(&cfg.Host, "host", envOr("KRONK_WEB_API_HOST", "http://localhost:11435"), "Kronk server base URL")
	fs.StringVar(&cfg.Out, "out", ".tools/reliability/output", "replaceable artifact directory")
	fs.StringVar(&cfg.ServerLog, "server-log", filepath.Join(home, ".kronk", "kronk.log"), "detached Kronk JSON log")
	fs.DurationVar(&cfg.Timeout, "timeout", 30*time.Minute, "per-request timeout")
	fs.Int64Var(&cfg.Seed, "seed", 42, "deterministic sampling seed")

	fs.StringVar(&cfg.MTPEmbeddedModel, "mtp-embedded-model", "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT", "embedded-MTP model")
	fs.StringVar(&cfg.MTPCompanionModel, "mtp-companion-model", "unsloth/Qwen3.8-27B-UD-Q4_K_XL/AGENT", "companion own-KV MTP model")
	fs.StringVar(&cfg.MTPProfile, "mtp-profile", "all", "MTP profile: embedded, companion, or all")
	fs.IntVar(&cfg.MTPRequests, "mtp-requests", 0, "concurrent MTP requests (0 uses every configured slot)")
	fs.IntVar(&cfg.MTPPromptTokens, "mtp-prompt-tokens", 2200, "calibrated MTP prompt tokens")
	fs.IntVar(&cfg.MTPMaxTokens, "mtp-max-tokens", 256, "maximum MTP completion tokens")
	fs.IntVar(&cfg.MTPEmbeddedSlots, "mtp-embedded-slots", 4, "expected embedded-MTP slots")
	fs.IntVar(&cfg.MTPCompanionSlots, "mtp-companion-slots", 2, "expected companion-MTP slots")

	fs.StringVar(&cfg.HybridModel, "hybrid-model", "unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL/AGENT", "single-slot hybrid model")

	fs.StringVar(&cfg.LongContextModel, "long-context-model", "unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL/AGENT", "long-context model")
	fs.StringVar(&stages, "long-context-stages", "4096,8192,16384,32768,65536,131072", "comma-separated long-context token targets")
	fs.IntVar(&cfg.LongContextMaxTokens, "long-context-max-tokens", 96, "maximum long-context completion tokens")

	fs.StringVar(&cfg.BatchModel, "batch-model", "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT", "multi-slot batch model")
	fs.IntVar(&cfg.BatchTurns, "batch-turns", 21, "synchronized turns per conversation")
	fs.IntVar(&cfg.BatchTargetTokens, "batch-target-tokens", 30000, "minimum final prompt tokens")
	fs.IntVar(&cfg.BatchTokensPerTurn, "batch-tokens-per-turn", 1400, "target tokens added per turn")
	fs.IntVar(&cfg.BatchMaxTokens, "batch-max-tokens", 128, "maximum batch completion tokens")
	fs.IntVar(&cfg.BatchSlots, "batch-slots", 4, "expected generation slots")
	fs.IntVar(&cfg.BatchConversations, "batch-conversations", 5, "conversations; values above slots exercise queue pressure")

	fs.StringVar(&cfg.MediaModel, "media-model", "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT", "multimodal model")
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

	if list {
		return cfg, true, nil
	}
	if err := cfg.validate(); err != nil {
		return config{}, false, err
	}
	return cfg, false, nil
}

func (cfg config) validate() error {
	if cfg.Host == "" || cfg.Out == "" || cfg.ServerLog == "" {
		return errors.New("-host, -out, and -server-log must not be empty")
	}
	if cfg.Timeout <= 0 || cfg.MTPPromptTokens <= 0 || cfg.MTPMaxTokens <= 0 || cfg.MTPRequests < 0 {
		return errors.New("timeouts and MTP token counts must be positive; -mtp-requests must not be negative")
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
	if cfg.MediaMaxTokens <= 0 || cfg.MediaGenerationMaxTokens <= 0 || cfg.MediaImageMaxTokens <= 0 || cfg.MediaMaxGenerationGap <= 0 {
		return errors.New("media token counts and generation gap must be positive")
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
