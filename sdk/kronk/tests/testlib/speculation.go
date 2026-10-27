package testlib

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// TestSpeculation runs serial groups with evidence scoped to one model load.
// Recurrent snapshot-replay is not qualified here: the normal loader provisions
// enough recurrent snapshots for native rollback.
func TestSpeculation(t *testing.T, cfg model.Config, backend, loadEvent, strategy string) {
	t.Helper()
	// ROCm does not populate the sampled candidates needed by classic drafting.
	requireDraftEvidence := backend != "classic-own-kv" || Processor() != "rocm"
	for _, disabled := range []bool{false, true} {
		name := "Active"
		if disabled {
			name = "Disabled"
		}
		t.Run(name, func(t *testing.T) {
			cfg := cfg
			cfg.PtrIncrementalCache = new(true)
			cfg.PtrCacheMinTokens = new(1)
			if disabled {
				cfg.Speculation = model.SpeculationDisabled
			}
			var mu sync.Mutex
			var loaded, rejected bool
			rollbacks := make(map[string]int)
			var disableReason string
			log := cfg.Log
			cfg.Log = func(ctx context.Context, msg string, args ...any) {
				if log != nil {
					log(ctx, msg, args...)
				}
				fields := make(map[string]string)
				for i := 0; i+1 < len(args); i += 2 {
					fields[fmt.Sprint(args[i])] = fmt.Sprint(args[i+1])
				}
				mu.Lock()
				defer mu.Unlock()
				if msg == "batch-engine" && fields["status"] == "slot-finished" && fields["draft_disable_reason"] != "" {
					disableReason = fields["draft_disable_reason"]
				}
				if msg == loadEvent && fields["status"] == "loaded" {
					loaded = backend == "classic-own-kv" || fields["backend"] == backend &&
						((backend == "qwen35-own-kv" && fields["source"] == "auto-detected") ||
							(backend == "gemma-shared-kv" && fields["source"] == "mtp-drafter-file" && fields["file"] == cfg.MTPDrafterFile))
				}
				if msg == "speculative" && fields["status"] == "target-rollback" && fields["backend"] == backend && fields["strategy"] == strategy {
					accepted, aerr := strconv.Atoi(fields["accepted"])
					drafted, derr := strconv.Atoi(fields["drafted"])
					_, serr := strconv.Atoi(fields["slot"])
					if aerr == nil && derr == nil && serr == nil && fields["id"] != "" && accepted >= 0 && drafted > accepted {
						rejected = true
						rollbacks[fields["id"]]++
					}
				}
			}
			WithModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
				mu.Lock()
				gotLoad := loaded
				mu.Unlock()
				if !disabled && !gotLoad {
					t.Fatal("missing qualified drafter load provenance")
				}
				if disabled && gotLoad {
					t.Fatal("disabled control loaded a drafter")
				}
				var accepted int
				check := func(t *testing.T, u *model.Usage) {
					t.Helper()
					if u == nil {
						t.Fatal("missing speculation usage")
					}
					t.Logf("draft=%d accepted=%d rate=%g coverage=%g reason=%s", u.DraftTokens, u.DraftAcceptedTokens, u.DraftAcceptanceRate, u.DraftCoverage, u.DraftDisableReason)
					if disabled {
						if u.DraftTokens != 0 || u.DraftAcceptedTokens != 0 || u.DraftAcceptanceRate != 0 || u.DraftCoverage != 0 {
							t.Errorf("disabled speculation: got %+v, want zero draft/accepted/rate/coverage", u)
						}
					} else if backend != "classic-own-kv" && u.DraftDisableReason != "" {
						t.Errorf("qualified MTP unexpectedly disabled: %s", u.DraftDisableReason)
					}
					accepted += u.DraftAcceptedTokens
				}
				if !disabled {
					t.Run("IMC", func(t *testing.T) { TestIMCState(t, krn, true) })
					t.Run("Isolation", func(t *testing.T) { TestChatIsolation(t, krn, []string{"BRAVO", "COPPER", "DELTA"}, false) })
				}
				// Short replies are controls, not per-response acceptance gates.
				t.Run("ShortChat", func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), TestDuration)
					defer cancel()
					resp, err := krn.Chat(ctx, StateRequest("ALPHA", "DELTA", 101))
					if err != nil {
						t.Fatal(err)
					}
					result := TestChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatTextFinal, "ALPHA", "", "", false, false)
					if result.Err != nil {
						t.Fatal(result.Err)
					}
					check(t, resp.Usage)
				})
				t.Run("LongContinuation", func(t *testing.T) {
					mu.Lock()
					rejected = false
					mu.Unlock()
					ctx, cancel := context.WithTimeout(context.Background(), TestDuration)
					defer cancel()
					d := model.D{"messages": []model.D{{"role": "user", "content": "Write a detailed numbered list of 40 distinct practical tips for maintaining a vegetable garden. Explain each tip in two sentences."}}, "max_tokens": 768, "enable_thinking": false, "temperature": 0.7, "stream_options": model.D{"include_usage": true}}
					for range 3 {
						ch, err := krn.ChatStreaming(ctx, d)
						if err != nil {
							t.Fatal(err)
						}
						resp, text, err := DrainStateChat(ctx, ch)
						if err != nil {
							t.Fatal(err)
						}
						if strings.TrimSpace(text) == "" || len(resp.Choices) != 1 || resp.Choices[0].FinishReason() == "" {
							t.Fatal("long request did not complete with text")
						}
						check(t, resp.Usage)
						// Continue using actual output, never compare stochastic text.
						d["messages"] = append(d["messages"].([]model.D), model.D{"role": "assistant", "content": text}, model.D{"role": "user", "content": "Continue with more detailed gardening advice."})
					}
					mu.Lock()
					gotRejected := rejected
					mu.Unlock()
					if !disabled && requireDraftEvidence && !gotRejected {
						t.Errorf("long workload produced no successful rejected-round rollback: backend=%s strategy=%s", backend, strategy)
					}
					// A subsequent independent request verifies slot recovery after rollback.
					ch, err := krn.ChatStreaming(ctx, StateRequest("COPPER", "ECHO", 303))
					if err != nil {
						t.Fatal(err)
					}
					resp, text, err := DrainStateChat(ctx, ch)
					if err != nil {
						t.Fatal(err)
					}
					if err := CheckStateReply(resp, text, "COPPER"); err != nil {
						t.Fatal(err)
					}
					check(t, resp.Usage)
				})
				t.Run("CancellationRecovery", func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), TestDuration)
					defer cancel()
					victim, stop := context.WithCancel(ctx)
					defer stop()
					ch, err := krn.ChatStreaming(victim, model.D{"messages": []model.D{{"role": "user", "content": "Write a very long essay about gardening."}}, "max_tokens": 768, "enable_thinking": false})
					if err != nil {
						t.Fatal(err)
					}
					// Wait for generated text, not a timing guess, before cancellation.
					started := false
					var victimID string
					for !started {
						select {
						case <-ctx.Done():
							t.Fatal(ctx.Err())
						case resp, ok := <-ch:
							if !ok {
								t.Fatal("victim ended before cancellation")
							}
							for _, choice := range resp.Choices {
								started = started || choice.Delta != nil && choice.Delta.Content != ""
							}
							victimID = resp.ID
						}
					}
					var peer <-chan model.ChatResponse
					if cfg.NSeqMax() > 1 {
						mu.Lock()
						before := rollbacks[victimID]
						mu.Unlock()
						request := StateRequest("SILVER", "ECHO", 404)
						request["messages"].([]model.D)[1]["content"] = "Ignore this padding: " +
							strings.Repeat("red green blue orange violet yellow ", 900) + " What is your first code word?"
						peer, err = krn.ChatStreaming(ctx, request)
						if err != nil {
							t.Fatal(err)
						}
						ticker := time.NewTicker(5 * time.Millisecond)
						defer ticker.Stop()
						for {
							snapshot, ok := krn.BatchEngineSnapshot()
							occupied := 0
							for _, slot := range snapshot.Slots {
								if slot.RequestID != "" {
									occupied++
								}
							}
							mu.Lock()
							rolledBack := rollbacks[victimID] > before
							mu.Unlock()
							if ok && occupied == 2 && (disabled || !requireDraftEvidence || rolledBack) {
								t.Logf("observed native peer overlap; victim rollback=%t slots=%+v", rolledBack, snapshot.Slots)
								break
							}
							select {
							case <-ctx.Done():
								t.Fatal("native peer overlap/rejected-round evidence not observed: ", ctx.Err())
							case _, ok := <-ch:
								if !ok {
									t.Fatal("victim completed before native peer overlap/recovery")
								}
							case <-ticker.C:
							}
						}
					}
					stop()
					// Drain with the live parent context so native cleanup must finish.
					for {
						select {
						case <-ctx.Done():
							t.Fatal(ctx.Err())
						case _, ok := <-ch:
							if !ok {
								goto drained
							}
						}
					}
				drained:
					if peer == nil {
						peer, err = krn.ChatStreaming(ctx, StateRequest("SILVER", "ECHO", 404))
						if err != nil {
							t.Fatal(err)
						}
					}
					resp, text, err := DrainStateChat(ctx, peer)
					if err != nil {
						t.Fatal(err)
					}
					if err := CheckStateReply(resp, text, "SILVER"); err != nil {
						t.Fatal(err)
					}
					check(t, resp.Usage)
				})
				if !disabled {
					t.Run("Evidence", func(t *testing.T) {
						if backend == "classic-own-kv" {
							SkipOnBackends(t, "llama_decode does not populate sampled candidates", "rocm")
						}
						if accepted == 0 {
							t.Error("loaded workload accepted no draft tokens")
						}
						mu.Lock()
						defer mu.Unlock()
						if backend != "classic-own-kv" && disableReason != "" {
							t.Errorf("MTP disabled during IMC/isolation/workload: %s", disableReason)
						}
						if !rejected {
							t.Errorf("missing successful rejected-round rollback: backend=%s strategy=%s", backend, strategy)
						}
					})
				}
			})
		})
	}
}
