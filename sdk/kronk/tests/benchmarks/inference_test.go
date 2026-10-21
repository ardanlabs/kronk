package benchmarks_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// BenchmarkInferenceMatrix measures one resident model and one request at a time.
// Native execution is opt-in; missing fixtures fail rather than silently shrinking
// the matrix. See README.md for baseline collection and comparison.
func BenchmarkInferenceMatrix(b *testing.B) {
	if os.Getenv("KRONK_BENCHMARK") != "true" {
		b.Skip("native inference requires KRONK_BENCHMARK=true; see README.md")
	}
	if err := kronk.Init(); err != nil {
		b.Fatal(err)
	}
	mdls, err := models.New()
	if err != nil {
		b.Fatal(err)
	}
	for _, fixture := range []struct {
		name, id, modelType, attention string
		mtp                            bool
	}{
		{"Qwen3", "unsloth/Qwen3-1.7B-Q4_K_M", "Dense", "Attention-only", false},
		{"Qwopus", "mradermacher/Qwopus3.5-4B-Coder.Q4_K_M", "Dense", "Hybrid", false},
		{"Gemma4", "unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M", "MoE", "Attention-only", true},
		{"Qwen36", "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL", "MoE", "Hybrid", true},
	} {
		b.Run(fixture.modelType+"/"+fixture.attention+"/"+fixture.name, func(b *testing.B) {
			path, err := mdls.FullPath(fixture.id)
			if err != nil {
				b.Fatal(err)
			}
			for _, imc := range []bool{false, true} {
				for _, mtp := range []bool{false, true} {
					if mtp && !fixture.mtp {
						continue // Unsupported, not a benchmark result.
					}
					b.Run(fmt.Sprintf("IMC=%t/MTP=%t", imc, mtp), func(b *testing.B) {
						cfg := model.Config{
							ModelFiles: path.ModelFiles, PtrContextWindow: new(8192),
							PtrNSeqMax: new(1), PtrPrefillBatchSize: new(512), PtrPhysicalBatchSize: new(512),
							CacheTypeK: model.GGMLTypeF16, CacheTypeV: model.GGMLTypeF16,
							PtrIncrementalCache: new(imc), PtrCacheMinTokens: new(1),
							Speculation: model.SpeculationDisabled,
							Log:         func(context.Context, string, ...any) {},
						}
						if mtp {
							cfg.Speculation = model.SpeculationMTP
							cfg.PtrDraftModel = &model.DraftModelConfig{NDraft: 3}
							if fixture.name == "Gemma4" {
								if path.MTPFile == "" {
									b.Fatal("Gemma MTP assistant is required")
								}
								cfg.MTPDrafterFile = path.MTPFile
							}
						}
						krn, err := kronk.New(model.WithConfig(cfg))
						if err != nil {
							b.Fatal(err)
						}
						b.Cleanup(func() {
							if err := krn.Unload(context.Background()); err != nil {
								b.Error(err)
							}
						})
						info, effective := krn.ModelInfo(), krn.ModelConfig()
						if info.Type != fixture.modelType || info.SequenceProcessing != fixture.attention ||
							effective.IncrementalCache() != imc || effective.SpeculationMode() != cfg.Speculation {
							b.Fatalf("fixture/mode mismatch: info=%s config=%s", info.String(), effective.String())
						}
						b.Logf("fixture=%s architecture=%s config=%s", fixture.id, info.Architecture, effective.String())
						for _, workload := range []string{"Miss", "Replay"} {
							b.Run(workload, func(b *testing.B) {
								benchmarkRequests(b, krn, imc, mtp, workload == "Replay")
							})
						}
					})
				}
			}
		})
	}
}

func benchmarkRequests(b *testing.B, krn *kronk.Kronk, imc, mtp, replay bool) {
	b.Helper()
	b.StopTimer()
	request := func(index int) model.D {
		// Changing the system prefix prevents complete-prefix and immutable
		// System-cache reuse; the usage check below verifies the miss.
		return model.D{
			"messages": []model.D{
				{"role": "system", "content": fmt.Sprintf("Benchmark record %08d.\n", index) +
					strings.Repeat("Amber cedar quartz harbor ledger records are independent.\n", 128)},
				{"role": "user", "content": "Write 100 numbered practical tips for maintaining a vegetable garden. Explain each tip in two sentences. Do not summarize or stop early."},
			},
			"enable_thinking": false, "temperature": 0.0, "seed": uint32(42),
			"max_tokens": 256, "stream_options": model.D{"include_usage": true},
		}
	}
	// Warm native kernels before either workload. Replay is then primed with
	// the exact request; Miss uses a new system prefix on every iteration.
	query := request(-1)
	run := func(query model.D) *model.Usage {
		ctx, cancel := context.WithTimeout(b.Context(), 5*time.Minute)
		defer cancel()
		ch, err := krn.ChatStreaming(ctx, query)
		if err != nil {
			b.Fatal(err)
		}
		resp, text, err := testlib.DrainStateChat(ctx, ch)
		if err != nil {
			b.Fatal(err)
		}
		if resp.ID == "" || resp.Model != krn.ModelID() || strings.TrimSpace(text) == "" || len(resp.Choices) != 1 ||
			resp.Choices[0].FinishReason() != model.FinishReasonLength || resp.Usage == nil {
			b.Fatal("incomplete benchmark response (expected 256-token length termination)")
		}
		u := resp.Usage
		if u.PromptTokens <= 0 || u.CompletionTokens != 256 || u.TotalTokens != u.PromptTokens+u.CompletionTokens ||
			u.PromptTokensDetails.CachedTokens < 0 || u.PromptTokensDetails.CachedTokens > u.PromptTokens ||
			u.TimeToFirstTokenMS <= 0 || u.TokensPerSecond <= 0 || math.IsNaN(u.TokensPerSecond) || math.IsInf(u.TokensPerSecond, 0) {
			b.Fatalf("invalid benchmark usage: %+v", u)
		}
		if !imc && u.PromptTokensDetails.CachedTokens != 0 {
			b.Fatal("IMC-off request reused cached tokens")
		}
		if !mtp && (u.DraftTokens != 0 || u.DraftAcceptedTokens != 0 || u.DraftAcceptanceRate != 0 || u.DraftCoverage != 0) {
			b.Fatal("MTP-off request reported speculation")
		}
		if u.DraftDisableReason != "" || u.DraftAcceptedTokens < 0 || u.DraftAcceptedTokens > u.DraftTokens {
			b.Fatalf("invalid/disabled speculation: %+v", u)
		}
		return u
	}
	run(query)
	var ttft, decodeSeconds float64
	var prompt, cached, drafted, accepted int
	i := 0
	for b.Loop() {
		b.StopTimer()
		if !replay {
			query = request(i)
		}
		i++
		b.StartTimer()
		u := run(query)
		b.StopTimer()
		if imc && replay && u.PromptTokensDetails.CachedTokens <= 0 {
			b.Fatal("designated replay did not reuse IMC")
		}
		if !replay && u.PromptTokensDetails.CachedTokens != 0 {
			b.Fatal("designated miss reused IMC; invalid cold-prefix measurement")
		}
		ttft += u.TimeToFirstTokenMS
		decodeSeconds += float64(u.CompletionTokens-1) / u.TokensPerSecond
		prompt += u.PromptTokens
		cached += u.PromptTokensDetails.CachedTokens
		drafted += u.DraftTokens
		accepted += u.DraftAcceptedTokens
	}
	if mtp && (drafted <= 0 || accepted <= 0) {
		b.Fatal("MTP benchmark did not draft and accept tokens")
	}
	b.ReportMetric(ttft/float64(b.N), "ttft-ms/op")
	b.ReportMetric(float64(255*b.N)/decodeSeconds, "decode-tok/s")
	b.ReportMetric(float64(prompt)/float64(b.N), "prompt-tok/op")
	b.ReportMetric(float64(cached)/float64(b.N), "cached-tok/op")
	b.ReportMetric(256, "output-tok/op")
	if mtp {
		b.ReportMetric(float64(accepted)/float64(drafted), "draft-acceptance/op")
	}
}
