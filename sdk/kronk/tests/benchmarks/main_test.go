/*
Package benchmarks_test measures code-generation performance and correctness
for the configured agent models.

The benchmark is a two-turn coding session sourced from
examples/talks/tic-tac-toe.md. It captures the complete model response and
reasoning separately, then grades the emitted program without using an LLM
judge.
*/
package benchmarks_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
	promptSeparator = "\n================================================================================\n"
	turnTimeout     = 20 * time.Minute
	systemProtocol  = `You are participating in a code-generation benchmark. You have no filesystem, shell, skills, or other tools. Follow the user's specification directly. Return the complete tictactoe/main.go source in exactly one fenced Go code block. On revision turns, return the complete updated file rather than a diff. Do not include a plan or ask questions.`
)

type benchmarkModel struct {
	name string
	id   string
}

func BenchmarkCodeGen_Ornith15_35B_Q8(b *testing.B) {
	benchmarkCodeGeneration(b, benchmarkModel{
		name: "Ornith15-35B-Q8",
		id:   "ornith-ai/Ornith-1.5-35B-Q8_0/AGENT",
	})
}

func BenchmarkCodeGen_MTP_Qwen36_35B_A3B_Q8(b *testing.B) {
	benchmarkCodeGeneration(b, benchmarkModel{
		name: "MTP-Qwen36-35B-A3B-Q8",
		id:   "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT",
	})
}

func BenchmarkCodeGen_Qwen38_27B_Q4(b *testing.B) {
	benchmarkCodeGeneration(b, benchmarkModel{
		name: "Qwen38-27B-Q4",
		id:   "unsloth/Qwen3.8-27B-UD-Q4_K_XL/AGENT",
	})
}

func BenchmarkCodeGen_Gemma4_26B_A4B_Q8(b *testing.B) {
	benchmarkCodeGeneration(b, benchmarkModel{
		name: "Gemma4-26B-A4B-Q8",
		id:   "unsloth/gemma-4-26B-A4B-it-UD-Q8_K_XL/AGENT",
	})
}

type promptTurns struct {
	base string
	undo string
}

type turnResult struct {
	Content      string        `json:"content"`
	Reasoning    string        `json:"reasoning"`
	FinishReason string        `json:"finish_reason"`
	Usage        model.Usage   `json:"usage"`
	WallTime     time.Duration `json:"wall_time"`
	MeasuredTTFT time.Duration `json:"measured_ttft"`
}

type configReport struct {
	ModelID             string            `json:"model_id"`
	ModelConfigFile     string            `json:"model_config_file"`
	LlamaCppVersion     string            `json:"llama_cpp_version,omitempty"`
	ModelType           string            `json:"model_type"`
	Quantization        string            `json:"quantization"`
	ModelBytes          uint64            `json:"model_bytes"`
	EstimatedVRAMBytes  int64             `json:"estimated_vram_bytes"`
	EstimatedSlotBytes  int64             `json:"estimated_slot_bytes"`
	NativeContextWindow int               `json:"native_context_window"`
	ContextWindow       int               `json:"context_window"`
	NSeqMax             int               `json:"nseq_max"`
	PrefillBatchSize    int               `json:"prefill_batch_size"`
	CacheTypeK          string            `json:"cache_type_k"`
	CacheTypeV          string            `json:"cache_type_v"`
	FlashAttention      string            `json:"flash_attention"`
	Speculation         string            `json:"speculation"`
	Temperature         float32           `json:"temperature"`
	TopK                int32             `json:"top_k"`
	TopP                float32           `json:"top_p"`
	MaxTokens           int               `json:"max_tokens"`
	Thinking            string            `json:"thinking"`
	SystemInfo          map[string]string `json:"system_info"`
}

type iterationReport struct {
	Model             configReport `json:"model"`
	Iteration         int          `json:"iteration"`
	SDKHeapDeltaBytes int64        `json:"sdk_go_heap_delta_bytes"`
	Base              turnResult   `json:"base_turn"`
	Undo              turnResult   `json:"undo_turn"`
	BaseGrade         gradeResult  `json:"base_grade"`
	UndoGrade         gradeResult  `json:"undo_grade"`
}

type benchmarkTotals struct {
	runs              int
	promptTokens      int
	completionTokens  int
	reasoningTokens   int
	wallTime          time.Duration
	ttftMS            float64
	tokensPerSecond   float64
	baseScore         float64
	undoScore         float64
	draftRate         float64
	draftCoverage     float64
	draftMeasurements int
	sdkHeapDeltaBytes int64
}

func benchmarkCodeGeneration(b *testing.B, bm benchmarkModel) {
	b.Helper()

	root, err := repositoryRoot()
	if err != nil {
		b.Fatal(err)
	}

	prompts, err := loadPromptTurns(root)
	if err != nil {
		b.Fatal(err)
	}

	krn, report := loadBenchmarkModel(b, root, bm)
	resultsDir, err := benchmarkResultsDir(root, bm.name)
	if err != nil {
		b.Fatalf("creating results directory: %v", err)
	}
	if err := writeBenchmarkInputs(resultsDir, prompts); err != nil {
		b.Fatalf("writing benchmark inputs: %v", err)
	}

	b.Logf("model=%s type=%s quant=%s", report.ModelID, report.ModelType, report.Quantization)
	b.Logf("config: context=%d native-context=%d nseq=%d temp=%g top-k=%d top-p=%g max-tokens=%d thinking=%s",
		report.ContextWindow, report.NativeContextWindow, report.NSeqMax, report.Temperature,
		report.TopK, report.TopP, report.MaxTokens, report.Thinking)
	b.Logf("memory estimates: model=%.2f GiB slot=%.2f GiB total=%.2f GiB",
		gib(report.ModelBytes), gib(uint64(max(report.EstimatedSlotBytes, 0))), gib(uint64(max(report.EstimatedVRAMBytes, 0))))
	b.Logf("artifacts: %s", resultsDir)

	var totals benchmarkTotals
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		iteration := totals.runs + 1
		var heapBefore runtime.MemStats
		runtime.ReadMemStats(&heapBefore)

		base, baseErr := generateTurn(b.Context(), krn, []model.D{
			{"role": "system", "content": systemProtocol},
			{"role": "user", "content": prompts.base},
		})

		assistant := model.D{"role": "assistant", "content": base.Content}
		if base.Reasoning != "" {
			assistant["reasoning_content"] = base.Reasoning
		}
		undo, undoErr := generateTurn(b.Context(), krn, []model.D{
			{"role": "system", "content": systemProtocol},
			{"role": "user", "content": prompts.base},
			assistant,
			{"role": "user", "content": prompts.undo},
		})
		var heapAfter runtime.MemStats
		runtime.ReadMemStats(&heapAfter)
		heapDelta := int64(heapAfter.Alloc) - int64(heapBefore.Alloc)

		b.StopTimer()

		baseSource := extractGoSource(base.Content)
		undoSource := extractGoSource(undo.Content)
		baseGrade := gradeProgram(b.Context(), baseSource, gradeBase)
		undoGrade := gradeProgram(b.Context(), undoSource, gradeUndo)

		result := iterationReport{
			Model:             report,
			Iteration:         iteration,
			SDKHeapDeltaBytes: heapDelta,
			Base:              base,
			Undo:              undo,
			BaseGrade:         baseGrade,
			UndoGrade:         undoGrade,
		}
		if err := writeIterationArtifacts(resultsDir, result, baseSource, undoSource); err != nil {
			b.Fatalf("writing iteration artifacts: %v", err)
		}

		if baseErr != nil {
			b.Errorf("base generation: %v", baseErr)
		}
		if undoErr != nil {
			b.Errorf("undo generation: %v", undoErr)
		}

		b.Logf("iteration %d: base=%d/%d undo=%d/%d base-finish=%s undo-finish=%s",
			iteration, baseGrade.Passed, baseGrade.Total, undoGrade.Passed, undoGrade.Total,
			base.FinishReason, undo.FinishReason)
		for _, failure := range append(baseGrade.Failures("base"), undoGrade.Failures("undo")...) {
			b.Log(failure)
		}

		accumulateTotals(&totals, base, undo, baseGrade, undoGrade, heapDelta)
		b.StartTimer()
	}

	b.StopTimer()
	reportMetrics(b, totals, report)
}

func loadBenchmarkModel(b *testing.B, root string, bm benchmarkModel) (*kronk.Kronk, configReport) {
	b.Helper()

	if err := kronk.Init(); err != nil {
		b.Fatalf("initializing kronk: %v", err)
	}

	mdls, err := models.New()
	if err != nil {
		b.Fatalf("creating models system: %v", err)
	}
	if _, err := mdls.FullPath(bm.id); err != nil {
		b.Skipf("model %s is not downloaded: %v", bm.id, err)
	}

	configPath := os.Getenv("KRONK_POOL_MODEL_CONFIG_FILE")
	if configPath == "" {
		configPath = filepath.Join(root, "zarf", "kms", "model_config.yaml")
	}
	configs, err := models.LoadModelConfig(configPath)
	if err != nil {
		b.Fatalf("loading model config %s: %v", configPath, err)
	}
	if _, exists := configs[bm.id]; !exists {
		b.Fatalf("model config %s has no entry for %s", configPath, bm.id)
	}

	cfg, err := mdls.KronkResolvedConfig(bm.id, configs)
	if err != nil {
		b.Fatalf("resolving model config: %v", err)
	}
	cfg.PtrNSeqMax = new(1)

	krn, err := kronk.New(model.WithConfig(cfg))
	if err != nil {
		b.Fatalf("loading model: %v", err)
	}
	b.Cleanup(func() {
		if err := krn.Unload(context.Background()); err != nil {
			b.Errorf("unloading model: %v", err)
		}
	})

	var llamaCppVersion string
	if l, err := libs.New(); err == nil {
		if version, err := l.InstalledVersion(); err == nil {
			llamaCppVersion = version.Version
			b.Logf("llama.cpp %s (%s/%s/%s)", version.Version, version.OS, version.Arch, version.Processor)
		}
	}

	effective := krn.ModelConfig()
	info := krn.ModelInfo()

	return krn, configReport{
		ModelID:             bm.id,
		ModelConfigFile:     configPath,
		LlamaCppVersion:     llamaCppVersion,
		ModelType:           info.Type.String(),
		Quantization:        info.Quantization,
		ModelBytes:          info.Size,
		EstimatedVRAMBytes:  info.VRAMTotal,
		EstimatedSlotBytes:  info.SlotMemory,
		NativeContextWindow: nativeContextWindow(info),
		ContextWindow:       effective.ContextWindow(),
		NSeqMax:             effective.NSeqMax(),
		PrefillBatchSize:    effective.PrefillBatchSize(),
		CacheTypeK:          effective.CacheTypeK.String(),
		CacheTypeV:          effective.CacheTypeV.String(),
		FlashAttention:      effective.FlashAttention().String(),
		Speculation:         string(effective.SpeculationMode()),
		Temperature:         effective.DefaultParams.Temperature,
		TopK:                effective.DefaultParams.TopK,
		TopP:                effective.DefaultParams.TopP,
		MaxTokens:           effective.DefaultParams.MaxTokens,
		Thinking:            effective.DefaultParams.Thinking,
		SystemInfo:          krn.SystemInfo(),
	}
}

func generateTurn(parent context.Context, krn *kronk.Kronk, messages []model.D) (turnResult, error) {
	ctx, cancel := context.WithTimeout(parent, turnTimeout)
	defer cancel()

	start := time.Now()
	ch, err := krn.ChatStreaming(ctx, model.D{
		"messages":       messages,
		"stream_options": model.D{"include_usage": true},
	})
	if err != nil {
		return turnResult{}, fmt.Errorf("opening stream: %w", err)
	}

	var content strings.Builder
	var reasoning strings.Builder
	var result turnResult
	var responseErr error
	var usageSeen bool
	var ttftSeen bool

	for resp := range ch {
		if resp.Usage != nil {
			result.Usage = *resp.Usage
			usageSeen = true
		}
		if len(resp.Choices) == 0 {
			continue
		}

		choice := resp.Choices[0]
		if finish := choice.FinishReason(); finish != "" {
			result.FinishReason = finish
		}
		if choice.Delta != nil {
			if !ttftSeen && (choice.Delta.Content != "" || choice.Delta.Reasoning != "") {
				result.MeasuredTTFT = time.Since(start)
				ttftSeen = true
			}
			content.WriteString(choice.Delta.Content)
			reasoning.WriteString(choice.Delta.Reasoning)
		}
		if choice.Message != nil {
			if content.Len() == 0 {
				result.Content = choice.Message.Content
			}
			if reasoning.Len() == 0 {
				result.Reasoning = choice.Message.Reasoning
			}
		}
		if choice.FinishReason() == model.FinishReasonError {
			responseErr = errors.New("model returned an error response")
			if choice.Message != nil && choice.Message.Content != "" {
				responseErr = errors.New(choice.Message.Content)
			} else if choice.Delta != nil && choice.Delta.Content != "" {
				responseErr = errors.New(choice.Delta.Content)
			}
		}
	}

	result.WallTime = time.Since(start)
	if content.Len() > 0 {
		result.Content = content.String()
	}
	if reasoning.Len() > 0 {
		result.Reasoning = reasoning.String()
	}
	if responseErr != nil {
		return result, responseErr
	}
	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("generation: %w", err)
	}
	if !usageSeen {
		return result, errors.New("generation: terminal usage response is missing")
	}
	if result.FinishReason == "" {
		return result, errors.New("generation: terminal finish reason is missing")
	}

	return result, nil
}

func accumulateTotals(totals *benchmarkTotals, base, undo turnResult, baseGrade, undoGrade gradeResult, heapDelta int64) {
	totals.runs++
	for _, turn := range []turnResult{base, undo} {
		totals.promptTokens += turn.Usage.PromptTokens
		totals.completionTokens += turn.Usage.CompletionTokens
		totals.reasoningTokens += turn.Usage.CompletionTokensDetails.ReasoningTokens
		totals.wallTime += turn.WallTime
		totals.ttftMS += turn.Usage.TimeToFirstTokenMS
		totals.tokensPerSecond += turn.Usage.TokensPerSecond
		if turn.Usage.DraftTokens > 0 {
			totals.draftRate += turn.Usage.DraftAcceptanceRate
			totals.draftCoverage += turn.Usage.DraftCoverage
			totals.draftMeasurements++
		}
	}
	totals.baseScore += baseGrade.Percentage()
	totals.undoScore += undoGrade.Percentage()
	totals.sdkHeapDeltaBytes += heapDelta
}

func reportMetrics(b *testing.B, totals benchmarkTotals, report configReport) {
	b.Helper()
	if totals.runs == 0 {
		return
	}

	turns := float64(totals.runs * 2)
	b.ReportMetric(float64(totals.promptTokens)/float64(totals.runs), "prompt-tok/op")
	b.ReportMetric(float64(totals.completionTokens)/float64(totals.runs), "completion-tok/op")
	b.ReportMetric(float64(totals.reasoningTokens)/float64(totals.runs), "reasoning-tok/op")
	b.ReportMetric(totals.tokensPerSecond/turns, "tok/s")
	b.ReportMetric(totals.ttftMS/turns, "ttft-ms")
	b.ReportMetric(float64(totals.wallTime.Milliseconds())/float64(totals.runs), "wall-ms/op")
	b.ReportMetric(totals.baseScore/float64(totals.runs), "base-score-%")
	b.ReportMetric(totals.undoScore/float64(totals.runs), "undo-score-%")
	b.ReportMetric(float64(report.ModelBytes)/(1<<30), "model-GiB")
	b.ReportMetric(float64(report.EstimatedSlotBytes)/(1<<20), "slot-MiB")
	b.ReportMetric(float64(report.EstimatedVRAMBytes)/(1<<30), "memory-GiB")
	b.ReportMetric(float64(totals.sdkHeapDeltaBytes)/float64(totals.runs)/(1<<20), "sdk-heap-MiB/op")
	if totals.draftMeasurements > 0 {
		measurements := float64(totals.draftMeasurements)
		b.ReportMetric(100*totals.draftRate/measurements, "draft-accept-%")
		b.ReportMetric(100*totals.draftCoverage/measurements, "draft-coverage-%")
	}
}

func loadPromptTurns(root string) (promptTurns, error) {
	path := filepath.Join(root, "examples", "talks", "tic-tac-toe.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return promptTurns{}, fmt.Errorf("reading prompt %s: %w", path, err)
	}

	base, undo, ok := strings.Cut(string(data), promptSeparator)
	if !ok {
		return promptTurns{}, errors.New("tic-tac-toe prompt: turn separator is missing")
	}

	return promptTurns{base: strings.TrimSpace(base), undo: strings.TrimSpace(undo)}, nil
}

func repositoryRoot() (string, error) {
	if root := os.Getenv("GITHUB_WORKSPACE"); root != "" {
		return filepath.Abs(root)
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("locating benchmark source")
	}
	return filepath.Abs(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

func benchmarkResultsDir(root, modelName string) (string, error) {
	base := os.Getenv("CODEGEN_RESULTS_DIR")
	if base == "" {
		base = filepath.Join(root, "sdk", "kronk", "tests", "benchmarks", "runs", time.Now().Format("20060102-150405"))
	} else if !filepath.IsAbs(base) {
		base = filepath.Join(root, base)
	}
	dir := filepath.Join(base, modelName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func writeIterationArtifacts(resultsDir string, result iterationReport, baseSource, undoSource string) error {
	dir := filepath.Join(resultsDir, fmt.Sprintf("iteration-%02d", result.Iteration))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	files := map[string]string{
		"base-response.md":  result.Base.Content,
		"base-reasoning.md": result.Base.Reasoning,
		"base-main.go":      baseSource,
		"undo-response.md":  result.Undo.Content,
		"undo-reasoning.md": result.Undo.Reasoning,
		"undo-main.go":      undoSource,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "result.json"), append(data, '\n'), 0644)
}

func writeBenchmarkInputs(resultsDir string, prompts promptTurns) error {
	files := map[string]string{
		"base-prompt.md": prompts.base + "\n",
		"undo-prompt.md": prompts.undo + "\n",
		"protocol.txt":   systemProtocol + "\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(resultsDir, name), []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}
	return nil
}

func nativeContextWindow(info model.ModelInfo) int {
	for key, value := range info.Metadata {
		if !strings.HasSuffix(key, ".context_length") {
			continue
		}
		var contextWindow int
		if _, err := fmt.Sscan(value, &contextWindow); err == nil {
			return contextWindow
		}
	}
	return 0
}

func gib(bytes uint64) float64 {
	return float64(bytes) / (1 << 30)
}
