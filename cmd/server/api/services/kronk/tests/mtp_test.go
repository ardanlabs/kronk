package chatapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

// Test_API_MTP is deliberately separate from the base API suite: these large
// fixtures belong to GPU runners, not ordinary CPU or hosted API jobs.
func Test_API_MTP(t *testing.T) {
	if os.Getenv("KRONK_TEST_MTP") != "yes" {
		t.Skip("set KRONK_TEST_MTP=yes to qualify the installed GPU MTP fixtures")
	}
	test := apitest.NewWithModelConfig(t, "Test_API_MTP", "testdata/model_config_mtp.yaml")
	if backend := testlib.Processor(); backend == "cpu" || backend == "" {
		t.Fatalf("GPU MTP qualification requires an executing GPU backend, got %q", backend)
	}
	token := createTokens(t, test.Sec)["chat-completions"]
	for _, fixture := range []struct {
		name, modelID, architecture, sequence, backend, loadEvent, strategy string
	}{
		{"gemma", "unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M", "gemma4", "Attention-only", "gemma-shared-kv", "draft-model-mtp-shared", "attention-suffix"},
		{"qwen", "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL", "qwen35moe", "Hybrid", "qwen35-own-kv", "draft-model-mtp", "recurrent-native"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			for _, profile := range []struct {
				name    string
				slots   int
				enabled bool
			}{{"MTP1", 1, true}, {"MTP2", 2, true}, {"MTPOff", 1, false}} {
				t.Run(profile.name, func(t *testing.T) {
					if profile.slots == 2 {
						testlib.SkipOnBackends(t, "two sequences sharing one decode batch come back with corrupted logits", "rocm")
					}
					id := fixture.modelID + "/" + profile.name
					// Release each configuration before its control/next model is
					// loaded. These are not simultaneous resident copies.
					t.Cleanup(func() {
						ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancel()
						if err := test.Pool.Kronk.InvalidateSync(ctx, id); err != nil {
							t.Errorf("unloading %s: %v", id, err)
						}
					})
					marker := len(test.ModelEvents())
					krn, err := test.Pool.Kronk.AquireModel(t.Context(), id)
					if err != nil {
						t.Fatal(err)
					}
					testlib.TestModelProfile(t, krn, "MoE", fixture.sequence, fixture.architecture)
					cfg := krn.ModelConfig()
					if len(cfg.ModelFiles) != 1 || filepath.Base(cfg.ModelFiles[0]) != filepath.Base(fixture.modelID)+".gguf" {
						t.Fatalf("wrong target artifact: %v", cfg.ModelFiles)
					}
					mode := model.SpeculationDisabled
					if profile.enabled {
						mode = model.SpeculationMTP
					}
					if cfg.Speculation != mode || cfg.NSeqMax() != profile.slots || cfg.ContextWindow() != 8192 ||
						cfg.CacheTypeK != model.GGMLTypeF16 || cfg.CacheTypeV != model.GGMLTypeF16 || !cfg.IncrementalCache() {
						t.Fatalf("unexpected MTP API configuration: %s", cfg.String())
					}
					t.Logf("effective %s: %s", id, cfg.String())
					loaded := false
					for _, record := range test.ModelEvents()[marker:] {
						if record.Message == fixture.loadEvent && record.Attributes["status"] == "loaded" {
							if record.Attributes["backend"] != fixture.backend {
								t.Fatalf("wrong MTP backend: %+v", record.Attributes)
							}
							if fixture.backend == "gemma-shared-kv" {
								if record.Attributes["source"] != "mtp-drafter-file" || record.Attributes["file"] != cfg.MTPDrafterFile ||
									filepath.Base(cfg.MTPDrafterFile) != "mtp-gemma-4-26B-A4B-it-UD-Q4_K_M.gguf" {
									t.Fatalf("wrong assistant artifact: %+v", record.Attributes)
								}
							} else if cfg.MTPDrafterFile != "" || record.Attributes["source"] != "auto-detected-configured" && record.Attributes["source"] != "auto-detected" {
								t.Fatalf("expected embedded Qwen head: %+v", record.Attributes)
							}
							loaded = true
						}
						if !profile.enabled && strings.HasPrefix(record.Message, "draft-model") && record.Attributes["status"] == "loaded" {
							t.Fatal("disabled control loaded a drafter")
						}
					}
					if profile.enabled && !loaded {
						t.Fatal("missing required MTP backend/artifact load evidence")
					}
					accepted := 0
					requestIDs := make(map[string]bool)
					run := func(name string, request model.D, want string, reuse bool) string {
						request["model"], request["stream"] = id, true
						var text string
						test.RunStreaming(t, []apitest.Table{{
							Name: name, URL: "/v1/chat/completions", Method: http.MethodPost, Token: token,
							Input: request, StatusCode: http.StatusOK, GotResp: &model.ChatResponse{},
							RequireDone: true, StreamResponseOffset: 1, CmpFunc: func(any, any) string { return "" },
							StreamCmpFunc: func(events []json.RawMessage) string {
								if diff := validateChatUsageStreamReasoning(events, false); diff != "" {
									return diff
								}
								var terminal model.ChatResponse
								var content strings.Builder
								for _, data := range events {
									var chunk model.ChatResponse
									if err := json.Unmarshal(data, &chunk); err != nil {
										return err.Error()
									}
									if chunk.Model != id {
										return "stream changed response model"
									}
									for _, choice := range chunk.Choices {
										if choice.Delta != nil {
											content.WriteString(choice.Delta.Content)
										}
										if choice.FinishReason() != "" {
											terminal = chunk
										}
									}
									if chunk.Usage != nil {
										terminal.Usage = chunk.Usage
									}
								}
								text = content.String()
								if err := validateSpeculationUsage(terminal.Usage, profile.enabled); err != nil {
									return err.Error()
								}
								if want != "" {
									if err := testlib.CheckStateReply(terminal, text, want); err != nil {
										return err.Error()
									}
								} else if len(strings.Fields(text)) < 32 || terminal.Usage.CompletionTokens < 32 {
									return "long speculation workload did not produce enough text to qualify"
								}
								if reuse && terminal.Usage.PromptTokensDetails.CachedTokens == 0 {
									return "qualified IMC replay/append did not reuse cached tokens"
								}
								accepted += terminal.Usage.DraftAcceptedTokens
								requestIDs[terminal.ID] = true
								t.Logf("%s: draft=%d accepted=%d cached=%d", name, terminal.Usage.DraftTokens,
									terminal.Usage.DraftAcceptedTokens, terminal.Usage.PromptTokensDetails.CachedTokens)
								return ""
							},
						}}, "mtp")
						if t.Failed() {
							t.FailNow()
						}
						return text
					}
					initial := testlib.StateRequest("ALPHA", "DELTA", 101)
					first := run("initial", initial, "ALPHA", false)
					run("exact-replay", initial, "ALPHA", true)
					extended := initial.Clone()
					extended["messages"] = append(extended["messages"].([]model.D),
						model.D{"role": "assistant", "content": first}, model.D{"role": "user", "content": "What is your second code word?"})
					run("append", extended, "DELTA", true)
					long := testlib.StateRequest("BRAVO", "ECHO", 202)
					long["messages"] = []model.D{{"role": "user", "content": "Explain how TCP retransmission, database transactions and garden irrigation each recover from partial failure. Give a different concrete example for each, then explain where the analogy breaks down."}}
					long["max_tokens"] = 384
					long["temperature"] = 0.7
					run("long-stream", long, "", false)
					if profile.slots == 1 {
						t.Run("media-state", func(t *testing.T) { chatMediaState(t, test, token, id) })
					}
					if profile.enabled {
						if accepted == 0 {
							t.Fatal("MTP workload accepted no draft tokens")
						}
						rollback := false
						for _, record := range test.ModelEvents()[marker:] {
							if record.Message == "speculative" && record.Attributes["status"] == "target-rollback" &&
								record.Attributes["backend"] == fixture.backend && record.Attributes["strategy"] == fixture.strategy {
								if requestID, ok := record.Attributes["id"].(string); ok && requestIDs[requestID] {
									rollback = true
								}
							}
						}
						if !rollback {
							t.Fatalf("native rejected-round recovery not qualified: %s", fixture.strategy)
						}
						t.Run("transport", func(t *testing.T) { chatTransport(t, test, token, id, profile.slots) })
					}
				})
			}
		})
	}
}

func validateSpeculationUsage(u *model.Usage, enabled bool) error {
	if u == nil {
		return fmt.Errorf("missing speculation usage")
	}
	if u.DraftTokens < 0 || u.DraftAcceptedTokens < 0 || u.DraftAcceptedTokens > u.DraftTokens ||
		math.IsNaN(u.DraftAcceptanceRate) || math.IsInf(u.DraftAcceptanceRate, 0) ||
		math.IsNaN(u.DraftCoverage) || math.IsInf(u.DraftCoverage, 0) ||
		u.DraftAcceptanceRate < 0 || u.DraftAcceptanceRate > 1 || u.DraftCoverage < 0 || u.DraftCoverage > 1 {
		return fmt.Errorf("invalid speculation accounting: %+v", u)
	}
	if !enabled && (u.DraftTokens != 0 || u.DraftAcceptedTokens != 0 || u.DraftAcceptanceRate != 0 || u.DraftCoverage != 0) {
		return fmt.Errorf("disabled control reported speculative work: %+v", u)
	}
	wantRate := 0.0
	if u.DraftTokens > 0 {
		wantRate = float64(u.DraftAcceptedTokens) / float64(u.DraftTokens)
	}
	if math.Abs(u.DraftAcceptanceRate-wantRate) > 1e-9 {
		return fmt.Errorf("draft acceptance rate: got %g, want %g", u.DraftAcceptanceRate, wantRate)
	}
	if u.DraftDisableReason != "" {
		return fmt.Errorf("unexpected speculation disablement: %s", u.DraftDisableReason)
	}
	return nil
}

func TestSpeculationUsage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		usage   *model.Usage
		enabled bool
		wantErr bool
	}{
		{"short active response", &model.Usage{}, true, false},
		{"accepted", &model.Usage{DraftTokens: 7, DraftAcceptedTokens: 3, DraftAcceptanceRate: 3.0 / 7, DraftCoverage: .4}, true, false},
		{"disabled", &model.Usage{}, false, false},
		{"nil", nil, true, true},
		{"disabled drafting", &model.Usage{DraftTokens: 2}, false, true},
		{"disabled coverage", &model.Usage{DraftCoverage: .3}, false, true},
		{"unexpected fallback", &model.Usage{DraftDisableReason: "mirror-error"}, true, true},
		{"impossible accepted", &model.Usage{DraftTokens: 2, DraftAcceptedTokens: 3}, true, true},
		{"inconsistent rate", &model.Usage{DraftTokens: 7, DraftAcceptedTokens: 3, DraftAcceptanceRate: .9}, true, true},
		{"nonfinite rate", &model.Usage{DraftAcceptanceRate: math.NaN()}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateSpeculationUsage(tc.usage, tc.enabled); (err != nil) != tc.wantErr {
				t.Fatalf("usage validation: got %v, want error %t", err, tc.wantErr)
			}
		})
	}
}
