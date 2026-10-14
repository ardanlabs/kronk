package model_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	kvTopologyModel         = "unsloth/Qwen3-0.6B-Q8_0"
	kvTopologyContextWindow = 512
	kvTopologyChunkSize     = 64
)

func TestNonUnifiedKVProvidesFullContextPerSequence(t *testing.T) {
	mdls, err := models.New()
	if err != nil {
		t.Fatalf("construct models system: %v", err)
	}

	modelPath, err := mdls.FullPath(kvTopologyModel)
	if err != nil || len(modelPath.ModelFiles) == 0 {
		t.Skipf("model %s not downloaded", kvTopologyModel)
	}

	if err := llama.Load(libs.Path("")); err != nil {
		t.Fatalf("load llama library: %v", err)
	}
	if err := llama.Init(); err != nil {
		t.Fatalf("initialize llama: %v", err)
	}
	llama.LogSet(llama.LogSilent())

	mdl, err := llama.ModelLoadFromFile(modelPath.ModelFiles[0], llama.ModelDefaultParams())
	if err != nil {
		t.Fatalf("load model: %v", err)
	}
	t.Cleanup(func() {
		if err := llama.ModelFree(mdl); err != nil {
			t.Errorf("free model: %v", err)
		}
	})

	tokens := llama.Tokenize(llama.ModelGetVocab(mdl), "test", true, true)
	if len(tokens) == 0 {
		t.Fatal("tokenize test input: got no tokens")
	}
	token := tokens[len(tokens)-1]

	orders := []struct {
		name string
		seqs []llama.SeqId
	}{
		{name: "sequence zero first", seqs: []llama.SeqId{0, 1}},
		{name: "sequence one first", seqs: []llama.SeqId{1, 0}},
	}

	for _, order := range orders {
		t.Run(order.name, func(t *testing.T) {
			lctx, mem := newKVTopologyContext(t, mdl)

			for _, seqID := range order.seqs {
				if err := decodeSequence(lctx, token, seqID, 0, kvTopologyContextWindow); err != nil {
					t.Fatalf("fill sequence %d: %v", seqID, err)
				}

				got, err := llama.MemorySeqPosMax(mem, seqID)
				if err != nil {
					t.Fatalf("sequence %d maximum position: %v", seqID, err)
				}
				if want := llama.Pos(kvTopologyContextWindow - 1); got != want {
					t.Errorf("sequence %d maximum position: got %d, want %d", seqID, got, want)
				}
			}
		})
	}

	t.Run("one sequence cannot claim another sequence partition", func(t *testing.T) {
		lctx, _ := newKVTopologyContext(t, mdl)
		if err := decodeSequence(lctx, token, 0, 0, kvTopologyContextWindow); err != nil {
			t.Fatalf("fill sequence 0: %v", err)
		}

		if err := decodeSequence(lctx, token, 0, kvTopologyContextWindow, 1); err == nil {
			t.Fatal("sequence 0 decoded beyond its context partition")
		}
	})
}

func TestKVCacheModesRespectRequestContext(t *testing.T) {
	mdls, err := models.New()
	if err != nil {
		t.Fatal(err)
	}
	path, err := mdls.FullPath(kvTopologyModel)
	if err != nil || len(path.ModelFiles) == 0 {
		t.Skipf("model %s not downloaded", kvTopologyModel)
	}
	if err := kronk.Init(); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []model.KVCacheMode{model.KVCachePartitioned, model.KVCacheShared} {
		t.Run(mode.String(), func(t *testing.T) {
			krn, err := kronk.New(model.WithModelFiles(path.ModelFiles), model.WithContextWindow(256),
				model.WithNSeqMax(3), model.WithKVCacheMode(mode), model.WithIncrementalCache(true), model.WithCacheMinTokens(1))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := krn.Unload(context.Background()); err != nil {
					t.Errorf("unload: %v", err)
				}
			})
			// Cover both concurrent request rounds on slower CI inference backends.
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
			defer cancel()
			results := make(chan error, 3)
			for i := range 3 {
				go func() {
					for turn := range 2 {
						resp, err := krn.Chat(ctx, model.D{
							"messages": model.Messages(model.TextMessage(model.RoleSystem, "Follow the user's counting instruction without commentary."), model.TextMessage(model.RoleUser,
								fmt.Sprintf("Count from %d to 5000, writing every number in words, one per line. Do not stop early.", i+1))),
							"enable_thinking": false, "max_tokens": 1000, "temperature": 0.0,
						})
						if err != nil {
							results <- err
							return
						}
						if resp.Usage == nil || resp.Usage.CompletionTokens == 0 || resp.Usage.TotalTokens > 256 {
							results <- fmt.Errorf("request %d: invalid context usage %+v", i, resp.Usage)
							return
						}
						if turn == 1 && resp.Usage.PromptTokensDetails.CachedTokens == 0 {
							results <- fmt.Errorf("request %d: repeated prompt did not restore cached tokens", i)
							return
						}
						t.Logf("request %d: prompt=%d completion=%d total=%d cached=%d", i,
							resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens, resp.Usage.PromptTokensDetails.CachedTokens)
					}
					results <- nil
				}()
			}
			for range 3 {
				if err := <-results; err != nil {
					t.Errorf("chat: %v", err)
				}
			}
			_, err = krn.Chat(ctx, model.D{"messages": model.Messages(model.TextMessage(model.RoleUser, strings.Repeat("hello ", 300)))})
			if !errors.Is(err, model.ErrInvalidRequest) {
				t.Errorf("oversized prompt: got %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func newKVTopologyContext(t *testing.T, mdl llama.Model) (llama.Context, llama.Memory) {
	t.Helper()

	params := llama.ContextDefaultParams()
	params.NCtx = 2 * kvTopologyContextWindow
	params.NBatch = kvTopologyChunkSize
	params.NUbatch = kvTopologyChunkSize
	params.NSeqMax = 2
	params.KVUnified = 0

	lctx, err := llama.InitFromModel(mdl, params)
	if err != nil {
		t.Fatalf("initialize context: %v", err)
	}
	t.Cleanup(func() {
		if err := llama.Synchronize(lctx); err != nil {
			t.Errorf("synchronize context: %v", err)
		}
		if err := llama.Free(lctx); err != nil {
			t.Errorf("free context: %v", err)
		}
	})

	if got, want := llama.NCtxSeq(lctx), uint32(kvTopologyContextWindow); got != want {
		t.Fatalf("context per sequence: got %d, want %d", got, want)
	}

	mem, err := llama.GetMemory(lctx)
	if err != nil {
		t.Fatalf("get context memory: %v", err)
	}
	if err := llama.MemoryClear(mem, true); err != nil {
		t.Fatalf("clear context memory: %v", err)
	}

	return lctx, mem
}

func decodeSequence(lctx llama.Context, token llama.Token, seqID llama.SeqId, start, count int) (retErr error) {
	batch, err := llama.BatchExtInit(lctx)
	if err != nil {
		return fmt.Errorf("initialize extended batch: %w", err)
	}
	defer func() {
		retErr = errors.Join(retErr, llama.BatchExtFree(batch))
	}()

	for offset := 0; offset < count; offset += kvTopologyChunkSize {
		if err := llama.BatchExtClear(batch); err != nil {
			return fmt.Errorf("clear batch: %w", err)
		}

		chunkSize := min(kvTopologyChunkSize, count-offset)
		for i := range chunkSize {
			idx, err := llama.BatchExtAddToken(batch, seqID, token)
			if err != nil {
				return fmt.Errorf("add token at position %d: %w", start+offset+i, err)
			}
			if err := llama.BatchExtSetPos(batch, idx, llama.Pos(start+offset+i)); err != nil {
				return fmt.Errorf("set token position %d: %w", start+offset+i, err)
			}
		}

		ret, err := llama.Process(lctx, llama.ProcessTypeDecode, batch)
		if err != nil {
			return fmt.Errorf("decode positions [%d,%d): %w", start+offset, start+offset+chunkSize, err)
		}
		if ret != 0 {
			return fmt.Errorf("decode positions [%d,%d): return code %d", start+offset, start+offset+chunkSize, ret)
		}
	}

	if err := llama.Synchronize(lctx); err != nil {
		return fmt.Errorf("synchronize context: %w", err)
	}

	return nil
}
