/*
Benchmarks for inference caching across model types.

Model Types (architecture — affects batch slot lifecycle and state management):
  - Dense:  Standard transformer. State cleanup via partial range delete.
  - MoE:    Mixture of Experts. Same cleanup as Dense, different perf profile.
  - Hybrid: Attention + Recurrent/linear layers. Snapshot/Restore cleanup.

Cache Modes Tested:
  - NonCaching: Baseline with no caching. Full prefill on every request.
  - IMC (Incremental Message Cache): Caches all messages except the last.
    The cache extends incrementally on each turn. IMC externalizes KV state
    to RAM after cache build, freeing the VRAM slot for other requests.
    Ideal for agentic workflows where conversations grow monotonically.

Benchmark Matrix:

	Model Type | Cache Mode | Model                              | Benchmark Name
	-----------|------------|------------------------------------|----------------------------
	Dense      | NonCaching | Qwen3-0.6B-Q8_0                    | BenchmarkDense_NonCaching
	Dense      | IMC        | Qwen3-0.6B-Q8_0                    | BenchmarkDense_IMC
	MoE        | NonCaching | gemma-4-26B-A4B-it-UD-Q4_K_M       | BenchmarkMoE_NonCaching
	MoE        | IMC        | gemma-4-26B-A4B-it-UD-Q4_K_M       | BenchmarkMoE_IMC
	Hybrid     | NonCaching | Qwen3.6-35B-A3B-UD-Q4_K_M          | BenchmarkHybrid_NonCaching
	Hybrid     | IMC        | Qwen3.6-35B-A3B-UD-Q4_K_M          | BenchmarkHybrid_IMC

Conversation Structure (~30k of 32k tokens):
  - System prompt (~10k tokens): Large system prompt simulating a real-world
    agentic workflow (similar to Cline, Cursor, etc.) with detailed technical
    competencies, code examples, API references, and project context. Must be
    large enough that IMC's KV state restore is faster than re-prefilling.
    At ~800 tokens the save/restore overhead exceeds re-prefill cost, so we
    target ~10k tokens where IMC shows clear benefit.
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
*/
package benchmarks_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// defaultMemProfileRate is Go's runtime default sampling rate (one sample
// per ~512 KiB allocated). benchChat restores this value around the timed
// region so pprof memory profiles only reflect inference work and not the
// one-shot kronk.New / VRAM-diag GGUF parse cost.
const defaultMemProfileRate = 512 * 1024

// =============================================================================
// Test setup - model paths resolved once

var (
	benchDenseModelPath  models.Path
	benchMoEModelPath    models.Path
	benchHybridModelPath models.Path
	benchLog             kronk.Logger
	benchLogFile         *os.File
)

func TestMain(m *testing.M) {
	// Disable memory profile sampling during model load, warmup, and any
	// other setup work. benchChat re-enables sampling for the duration of
	// the timed loop so pprof's -memprofile reflects only inference cost,
	// not the one-shot kronk.New / VRAM-diag GGUF parse.
	runtime.MemProfileRate = 0

	// When BENCH_LOG is set, write model logs to that file.
	// Usage: BENCH_LOG=bench.log go test -bench=BenchmarkDense_IMC ...
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

	// Dense target — only needed for BenchmarkDense_* benchmarks.
	if dp, err := mdls.FullPath("Qwen3-0.6B-Q8_0"); err == nil {
		benchDenseModelPath = dp
	}

	// MoE target — only needed for BenchmarkMoE_* benchmarks.
	if dp, err := mdls.FullPath("gemma-4-26B-A4B-it-UD-Q4_K_M"); err == nil {
		benchMoEModelPath = dp
	}

	// Hybrid target — only needed for BenchmarkHybrid_* benchmarks.
	if dp, err := mdls.FullPath("Qwen3.6-35B-A3B-UD-Q4_K_M"); err == nil {
		benchHybridModelPath = dp
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
