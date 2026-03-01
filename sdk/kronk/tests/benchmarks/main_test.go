/*
Benchmarks for inference caching across model types and IMC strategies.

Model Types (architecture — affects batch slot lifecycle and state management):
  - Dense:  Standard transformer. State cleanup via partial range delete.
  - MoE:    Mixture of Experts. Same cleanup as Dense, different perf profile.
  - Hybrid: Attention + Recurrent layers (DeltaNet/SSM). Snapshot/Restore.

IMC Strategies (template — affects slot matching):
  - Deterministic:     Hash-based matching. Consistent templates.
  - Non-Deterministic: Token prefix fallback. Variable templates.

Cache Modes Tested:
  - NonCaching: Baseline with no caching. Full prefill on every request.
  - SPC (System Prompt Cache): Caches the system prompt KV state. The system
    prompt is decoded once and its KV state is externalized to a byte buffer
    in RAM. Subsequent requests restore from the buffer, skipping redundant
    prefill of the system prompt.
  - IMC (Incremental Message Cache): Caches all messages except the last.
    The cache extends incrementally on each turn. Ideal for agentic workflows
    where conversations grow monotonically.

Benchmark Matrix:

	Model Type | Cache Mode          | IMC Strategy      | Speculative | Benchmark Name
	-----------|---------------------|-------------------|-------------|----------------------------------------------
	Dense      | NonCaching          | —                 | No          | BenchmarkDense_NonCaching
	Dense      | SPC                 | —                 | No          | BenchmarkDense_SPC
	Dense      | IMC                 | Deterministic     | No          | BenchmarkDense_IMCDeterministic
	Dense      | IMC                 | Deterministic     | Yes         | BenchmarkDense_IMCDeterministic_Speculative
	Dense      | IMC                 | Non-Deterministic | No          | BenchmarkDense_IMCNonDeterministic
	MoE        | IMC                 | Deterministic     | No          | BenchmarkMoE_IMCDeterministic
	Hybrid     | IMC                 | Deterministic     | No          | BenchmarkHybrid_IMCDeterministic
	MoE        | IMC                 | Deterministic     | No          | BenchmarkMoE_Speculative_Baseline
	MoE        | IMC                 | Deterministic     | Yes         | BenchmarkMoE_Speculative_WithDraft

Conversation Structure (~30k of 32k tokens):
  - System prompt (~10k tokens): Large system prompt simulating a real-world
    agentic workflow (similar to Cline, Cursor, etc.) with detailed technical
    competencies, code examples, API references, and project context. Must be
    large enough that SPC's KV state restore is faster than re-prefilling.
    At ~800 tokens the save/restore overhead exceeds re-prefill cost, so we
    target ~10k tokens where SPC shows clear benefit.
  - ~15+ conversation turns (~20k tokens): 6 unique technical Q&A pairs
    (GC tuning, PostgreSQL query optimization, Kafka partitioning, Redis
    caching, observability) cycled ~3 times with turn-number suffixes to
    avoid degenerate tokenization. Exercises IMC caching.
  - max_tokens=128: Small output keeps the benchmark focused on prefill and
    caching performance rather than generation.
  - temperature=0.0: Deterministic output for consistency across runs.

Metrics Reported Per Iteration:
  - ttft-ms     Time To First Token in milliseconds.
  - tok/s       Tokens per second (decode throughput).
  - total-ms    Wall-clock end-to-end request time in milliseconds.
  - prompt-tok  Prompt token count (consistency check across runs).
  - output-tok  Output token count (consistency check across runs).
  - B/op        Bytes allocated per operation (via ReportAllocs).
  - allocs/op   Number of allocations per operation (via ReportAllocs).

Running:

	go test -bench=. -benchtime=3x -timeout=60m ./sdk/kronk/tests/benchmarks/
	go test -bench=BenchmarkDense_SPC -benchtime=5x -timeout=60m ./sdk/kronk/tests/benchmarks/
*/
package benchmarks_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// =============================================================================
// Test setup - model path resolved once

var benchModelPath models.Path
var benchDraftModelPath models.Path
var benchMoEModelPath models.Path
var benchSpecModelPath models.Path
var benchHybridModelPath models.Path
var benchNonDetModelPath models.Path
var benchLog model.Logger
var benchLogFile *os.File

func TestMain(m *testing.M) {
	// When BENCH_LOG is set, write model logs to that file.
	// Usage: BENCH_LOG=bench.log go test -bench=BenchmarkDense_IMCDeterministic_Speculative ...
	if logPath := os.Getenv("BENCH_LOG"); logPath != "" {
		f, err := os.Create(logPath)
		if err != nil {
			fmt.Printf("bench: unable to create log file %s: %v\n", logPath, err)
			os.Exit(1)
		}
		benchLogFile = f
		logger := slog.New(slog.NewTextHandler(f, nil))
		benchLog = func(ctx context.Context, msg string, args ...any) {
			logger.Info(msg, args...)
		}
		fmt.Printf("bench: logging to %s\n", logPath)
	}

	mdls, err := models.New()
	if err != nil {
		fmt.Printf("bench: unable to create models system: %v\n", err)
		os.Exit(1)
	}

	benchModelPath = mdls.MustFullPath("Qwen3-8B-Q8_0")

	// Draft model is optional — only needed for BenchmarkDense_IMCDeterministic_Speculative.
	if dp, err := mdls.FullPath("Qwen3-0.6B-Q8_0"); err == nil {
		benchDraftModelPath = dp
	}

	// MoE target — only needed for BenchmarkMoE_IMCDeterministic.
	if dp, err := mdls.FullPath("Qwen_Qwen3.5-35B-A3B-Q8_0"); err == nil {
		benchMoEModelPath = dp
	}

	// Speculative target — only needed for BenchmarkMoE_Speculative_*.
	if dp, err := mdls.FullPath("cerebras_Qwen3-Coder-REAP-25B-A3B-Q8_0"); err == nil {
		benchSpecModelPath = dp
	}

	// Hybrid target — only needed for BenchmarkHybrid_* benchmarks.
	if dp, err := mdls.FullPath("Qwen3-Coder-Next-Q4_0"); err == nil {
		benchHybridModelPath = dp
	}

	// NonDeterministic target — only needed for BenchmarkDense_IMCNonDeterministic.
	if dp, err := mdls.FullPath("gpt-oss-20b-Q8_0"); err == nil {
		benchNonDetModelPath = dp
	}

	ctlg, err := catalog.New()
	if err != nil {
		fmt.Printf("bench: unable to create catalog: %v\n", err)
		os.Exit(1)
	}

	if err := ctlg.Download(context.Background()); err != nil {
		fmt.Printf("bench: unable to download catalog: %v\n", err)
		os.Exit(1)
	}

	if err := kronk.Init(); err != nil {
		fmt.Printf("bench: unable to init kronk: %v\n", err)
		os.Exit(1)
	}

	if l, err := libs.New(); err == nil {
		if vt, err := l.InstalledVersion(); err == nil {
			fmt.Printf("bench: llama.cpp %s (%s/%s/%s)\n", vt.Version, vt.OS, vt.Arch, vt.Processor)
		}
	}

	code := m.Run()

	if benchLogFile != nil {
		benchLogFile.Close()
	}

	os.Exit(code)
}
