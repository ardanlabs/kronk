// Package main generates a durable summary for a code-generation benchmark run.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type options struct {
	runDir string
}

type result struct {
	Model             modelReport `json:"model"`
	Iteration         int         `json:"iteration"`
	SDKHeapDeltaBytes int64       `json:"sdk_go_heap_delta_bytes"`
	Generation        turnResult  `json:"generation"`
	Grade             gradeResult `json:"grade"`
	LegacyGeneration  turnResult  `json:"base_turn"`
	LegacyGrade       gradeResult `json:"base_grade"`
}

type modelReport struct {
	ModelID            string  `json:"model_id"`
	LlamaCppVersion    string  `json:"llama_cpp_version"`
	ModelType          string  `json:"model_type"`
	Quantization       string  `json:"quantization"`
	ModelBytes         uint64  `json:"model_bytes"`
	EstimatedVRAMBytes int64   `json:"estimated_vram_bytes"`
	EstimatedSlotBytes int64   `json:"estimated_slot_bytes"`
	ContextWindow      int     `json:"context_window"`
	NSeqMax            int     `json:"nseq_max"`
	Speculation        string  `json:"speculation"`
	Temperature        float32 `json:"temperature"`
	TopK               int32   `json:"top_k"`
	TopP               float32 `json:"top_p"`
	MaxTokens          int     `json:"max_tokens"`
	Thinking           string  `json:"thinking"`
}

type turnResult struct {
	FinishReason string        `json:"finish_reason"`
	Usage        usage         `json:"usage"`
	WallTime     time.Duration `json:"wall_time"`
}

type usage struct {
	CompletionTokens       int               `json:"completion_tokens"`
	CompletionTokenDetails completionDetails `json:"completion_tokens_details"`
	PromptTokens           int               `json:"prompt_tokens"`
	TokensPerSecond        float64           `json:"tokens_per_second"`
	TimeToFirstTokenMS     float64           `json:"time_to_first_token_ms"`
	DraftTokens            int               `json:"draft_tokens"`
	DraftAcceptanceRate    float64           `json:"draft_acceptance_rate"`
	DraftCoverage          float64           `json:"draft_coverage"`
}

type completionDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type gradeResult struct {
	Passed int          `json:"passed"`
	Total  int          `json:"total"`
	Checks []gradeCheck `json:"checks"`
}

type gradeCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type modelResults struct {
	Name       string
	Iterations []result
}

func main() {
	var opts options
	flag.StringVar(&opts.runDir, "run", "", "benchmark run directory")
	flag.Parse()

	if err := generate(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(opts options) error {
	if opts.runDir == "" {
		return errors.New("benchmark report: -run is required")
	}

	models, err := loadResults(opts.runDir)
	if err != nil {
		return err
	}

	report := renderReport(opts.runDir, models)
	path := filepath.Join(opts.runDir, "report.md")
	if err := os.WriteFile(path, []byte(report), 0644); err != nil {
		return fmt.Errorf("benchmark report: writing %s: %w", path, err)
	}

	fmt.Printf("Report written to %s\n", path)
	return nil
}

func loadResults(runDir string) ([]modelResults, error) {
	paths, err := filepath.Glob(filepath.Join(runDir, "*", "iteration-*", "result.json"))
	if err != nil {
		return nil, fmt.Errorf("benchmark report: finding results: %w", err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("benchmark report: no iteration results found in %s", runDir)
	}

	byModel := make(map[string][]result)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("benchmark report: reading %s: %w", path, err)
		}

		var item result
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, fmt.Errorf("benchmark report: decoding %s: %w", path, err)
		}
		if item.Grade.Total == 0 {
			item.Generation = item.LegacyGeneration
			item.Grade = item.LegacyGrade
		}
		modelName := filepath.Base(filepath.Dir(filepath.Dir(path)))
		byModel[modelName] = append(byModel[modelName], item)
	}

	models := make([]modelResults, 0, len(byModel))
	for name, iterations := range byModel {
		slices.SortFunc(iterations, func(a, b result) int {
			return a.Iteration - b.Iteration
		})
		models = append(models, modelResults{Name: name, Iterations: iterations})
	}
	slices.SortFunc(models, func(a, b modelResults) int {
		return strings.Compare(a.Name, b.Name)
	})

	return models, nil
}

func renderReport(runDir string, models []modelResults) string {
	var report strings.Builder
	fmt.Fprintf(&report, "# Code-generation benchmark report\n\n")
	fmt.Fprintf(&report, "Run: `%s`  \n", filepath.Base(filepath.Clean(runDir)))
	fmt.Fprintf(&report, "Generated: %s  \n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&report, "Models: %d\n\n", len(models))

	report.WriteString("## Summary\n\n")
	report.WriteString("| Model | Attempts | Score | Buildable | Fully passed |\n")
	report.WriteString("| --- | ---: | ---: | ---: | ---: |\n")
	for _, model := range models {
		var passed, total, buildable, fullyPassed int

		for _, iteration := range model.Iterations {
			passed += iteration.Grade.Passed
			total += iteration.Grade.Total
			if iteration.Grade.PassedCheck("go-build") {
				buildable++
			}
			if iteration.Grade.Passed == iteration.Grade.Total {
				fullyPassed++
			}
		}

		fmt.Fprintf(&report, "| %s | %d | %.2f%% | %.2f%% | %.2f%% |\n",
			model.Name,
			len(model.Iterations),
			percentage(passed, total),
			percentage(buildable, len(model.Iterations)),
			percentage(fullyPassed, len(model.Iterations)),
		)
	}

	report.WriteString("\nBuildable is the percentage of attempts that passed `go build`. Fully passed is the percentage that passed every structural, build, vet, unit, and end-to-end scenario check.\n\n")
	report.WriteString("## Performance\n\n")
	report.WriteString("| Model | tok/s | TTFT | Wall/op | Prompt tok/op | Completion tok/op | Reasoning tok/op | Draft accept | Draft coverage |\n")
	report.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, model := range models {
		var tokensPerSecond, ttftMS, draftAcceptance, draftCoverage float64
		var wallTime time.Duration
		var promptTokens, completionTokens, reasoningTokens, draftMeasurements int

		for _, iteration := range model.Iterations {
			generation := iteration.Generation
			tokensPerSecond += generation.Usage.TokensPerSecond
			ttftMS += generation.Usage.TimeToFirstTokenMS
			wallTime += generation.WallTime
			promptTokens += generation.Usage.PromptTokens
			completionTokens += generation.Usage.CompletionTokens
			reasoningTokens += generation.Usage.CompletionTokenDetails.ReasoningTokens
			if generation.Usage.DraftTokens > 0 {
				draftAcceptance += generation.Usage.DraftAcceptanceRate
				draftCoverage += generation.Usage.DraftCoverage
				draftMeasurements++
			}
		}

		iterations := float64(len(model.Iterations))
		fmt.Fprintf(&report, "| %s | %.2f | %.0f ms | %.2f s | %.0f | %.0f | %.0f | %.2f%% | %.2f%% |\n",
			model.Name,
			tokensPerSecond/iterations,
			ttftMS/iterations,
			wallTime.Seconds()/iterations,
			float64(promptTokens)/iterations,
			float64(completionTokens)/iterations,
			float64(reasoningTokens)/iterations,
			percentageFloat(draftAcceptance, float64(draftMeasurements)),
			percentageFloat(draftCoverage, float64(draftMeasurements)),
		)
	}

	report.WriteString("\nPerformance metrics are averages across attempts.\n\n")
	report.WriteString("## Resources\n\n")
	report.WriteString("| Model | Model | Slot | Total memory | SDK heap/op |\n")
	report.WriteString("| --- | ---: | ---: | ---: | ---: |\n")
	for _, model := range models {
		config := model.Iterations[0].Model
		var sdkHeapBytes int64
		for _, iteration := range model.Iterations {
			sdkHeapBytes += iteration.SDKHeapDeltaBytes
		}
		fmt.Fprintf(&report, "| %s | %.2f GiB | %.0f MiB | %.2f GiB | %.2f MiB |\n",
			model.Name,
			float64(config.ModelBytes)/(1<<30),
			float64(config.EstimatedSlotBytes)/(1<<20),
			float64(config.EstimatedVRAMBytes)/(1<<30),
			float64(sdkHeapBytes)/float64(len(model.Iterations))/(1<<20),
		)
	}

	report.WriteString("\nSDK heap is Go-managed memory only. Model, slot, and total-memory estimates include native llama.cpp allocations.\n\n")
	report.WriteString("## Configuration\n\n")
	report.WriteString("| Model | Model ID | Type | Quant | Context | Sampling | Max tokens | Thinking | Speculation | llama.cpp |\n")
	report.WriteString("| --- | --- | --- | --- | ---: | --- | ---: | --- | --- | --- |\n")
	for _, model := range models {
		config := model.Iterations[0].Model
		fmt.Fprintf(&report, "| %s | `%s` | %s | %s | %d | temp=%g, top-k=%d, top-p=%g | %d | %s | %s | %s |\n",
			model.Name,
			config.ModelID,
			config.ModelType,
			config.Quantization,
			config.ContextWindow,
			config.Temperature,
			config.TopK,
			config.TopP,
			config.MaxTokens,
			config.Thinking,
			config.Speculation,
			config.LlamaCppVersion,
		)
	}

	report.WriteString("\n## Iterations\n")
	for _, model := range models {
		fmt.Fprintf(&report, "\n### %s\n", model.Name)
		for _, iteration := range model.Iterations {
			fmt.Fprintf(&report, "\n#### Iteration %d\n\n", iteration.Iteration)
			fmt.Fprintf(&report, "- Score: %d/%d (%.2f%%), finish `%s`\n",
				iteration.Grade.Passed, iteration.Grade.Total,
				percentage(iteration.Grade.Passed, iteration.Grade.Total), iteration.Generation.FinishReason)
			writeFailures(&report, iteration.Grade)
		}
	}

	return report.String()
}

func writeFailures(report *strings.Builder, grade gradeResult) {
	for _, check := range grade.Checks {
		if check.Passed {
			continue
		}
		fmt.Fprintf(report, "\n**Failure — `%s`**\n\n", check.Name)
		fmt.Fprintf(report, "```text\n%s\n```\n", check.Detail)
	}
}

func (gr gradeResult) PassedCheck(name string) bool {
	for _, check := range gr.Checks {
		if check.Name == name {
			return check.Passed
		}
	}
	return false
}

func percentage(passed, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(passed) / float64(total)
}

func percentageFloat(value, count float64) float64 {
	if count == 0 {
		return 0
	}
	return 100 * value / count
}
