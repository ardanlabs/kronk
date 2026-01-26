package kronk_test

import (
	"context"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// TestSuite - runs all tests grouped by model to minimize load/unload cycles.
// Set RUN_IN_PARALLEL=yes to run model groups concurrently (requires more resources).
func TestSuite(t *testing.T) {
	t.Run("Chat/Qwen3", func(t *testing.T) {
		if runInParallel {
			t.Parallel()
		}

		withModel(t, cfgThinkToolChat(), func(t *testing.T, krn *kronk.Kronk) {
			t.Run("ThinkChat", func(t *testing.T) { testChat(t, krn, dChatNoTool, false) })
			t.Run("ThinkStreamingChat", func(t *testing.T) { testChatStreaming(t, krn, dChatNoTool, false) })
			t.Run("ToolChat", func(t *testing.T) { testChat(t, krn, dChatTool, true) })
			t.Run("ToolStreamingChat", func(t *testing.T) { testChatStreaming(t, krn, dChatTool, true) })

			if os.Getenv("GITHUB_ACTIONS") != "true" {
				t.Run("ArrayFormatChat", func(t *testing.T) { testChat(t, krn, dChatNoToolArray, false) })
				t.Run("ArrayFormatStreamingChat", func(t *testing.T) { testChatStreaming(t, krn, dChatNoToolArray, false) })
				t.Run("ThinkResponse", func(t *testing.T) { testResponse(t, krn, dResponseNoTool, false) })
				t.Run("ThinkStreamingResponse", func(t *testing.T) { testResponseStreaming(t, krn, dResponseNoTool, false) })
				t.Run("ToolResponse", func(t *testing.T) { testResponse(t, krn, dResponseTool, true) })
				t.Run("ToolStreamingResponse", func(t *testing.T) { testResponseStreaming(t, krn, dResponseTool, true) })
			}
		})
	})

	t.Run("Media/Qwen2.5-VL", func(t *testing.T) {
		if runInParallel {
			t.Parallel()
		}

		withModel(t, cfgSimpleVision(), func(t *testing.T, krn *kronk.Kronk) {
			t.Run("SimpleMedia", func(t *testing.T) { testMedia(t, krn) })
			t.Run("SimpleMediaStreaming", func(t *testing.T) { testMediaStreaming(t, krn) })
			t.Run("SimpleMediaResponse", func(t *testing.T) { testMediaResponse(t, krn) })
			t.Run("SimpleMediaResponseStreaming", func(t *testing.T) { testMediaResponseStreaming(t, krn) })
			t.Run("ArrayFormatMedia", func(t *testing.T) { testMediaArray(t, krn) })
			t.Run("ArrayFormatMediaStreaming", func(t *testing.T) { testMediaArrayStreaming(t, krn) })
		})
	})

	t.Run("Embed/embeddinggemma", func(t *testing.T) {
		if runInParallel {
			t.Parallel()
		}

		withModel(t, cfgEmbed(), func(t *testing.T, krn *kronk.Kronk) {
			t.Run("Embedding", func(t *testing.T) { testEmbedding(t, krn) })
		})
	})

	t.Run("Rerank/bge-reranker", func(t *testing.T) {
		if runInParallel {
			t.Parallel()
		}

		withModel(t, cfgRerank(), func(t *testing.T, krn *kronk.Kronk) {
			t.Run("Rerank", func(t *testing.T) { testRerank(t, krn) })
		})
	})

	t.Run("Chat/GPT", func(t *testing.T) {
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			t.Skip("Skipping GPT tests in GitHub Actions (requires more resources)")
		}

		if runInParallel {
			t.Parallel()
		}

		withModel(t, cfgGPTChat(), func(t *testing.T, krn *kronk.Kronk) {
			t.Run("GPTChat", func(t *testing.T) { testChat(t, krn, dChatNoTool, false) })
			t.Run("GPTStreamingChat", func(t *testing.T) { testChatStreaming(t, krn, dChatNoTool, false) })
			t.Run("ToolGPTChat", func(t *testing.T) { testChat(t, krn, dChatToolGPT, true) })
			t.Run("ToolGPTStreamingChat", func(t *testing.T) { testChatStreaming(t, krn, dChatToolGPT, true) })
		})
	})

	t.Run("Audio/Qwen2-Audio", func(t *testing.T) {
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			t.Skip("Skipping Audio tests in GitHub Actions (requires more resources)")
		}

		if runInParallel {
			t.Parallel()
		}

		withModel(t, cfgAudio(), func(t *testing.T, krn *kronk.Kronk) {
			t.Run("AudioChat", func(t *testing.T) { testAudio(t, krn) })
			t.Run("AudioStreamingChat", func(t *testing.T) { testAudioStreaming(t, krn) })
		})
	})
}

// =============================================================================

// withModel creates a Kronk instance for the duration of fn, handling cleanup.
func withModel(t *testing.T, cfg model.Config, fn func(t *testing.T, krn *kronk.Kronk)) {
	t.Helper()

	krn, err := kronk.New(cfg)
	if err != nil {
		t.Fatalf("unable to load model %v: %v", cfg.ModelFiles, err)
	}

	t.Cleanup(func() {
		t.Logf("active streams: %d", krn.ActiveStreams())
		t.Log("unloading model")
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("failed to unload model: %v", err)
		}
	})

	fn(t, krn)
}

// =============================================================================
// Config builders for each model type

func cfgThinkToolChat() model.Config {
	return model.Config{
		ModelFiles:    mpThinkToolChat.ModelFiles,
		ContextWindow: 32768,
		NBatch:        1024,
		NUBatch:       256,
		CacheTypeK:    model.GGMLTypeF16,
		CacheTypeV:    model.GGMLTypeF16,
		NSeqMax:       2,
	}
}

func cfgGPTChat() model.Config {
	return model.Config{
		ModelFiles:    mpGPTChat.ModelFiles,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       512,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
		NSeqMax:       2,
	}
}

func cfgSimpleVision() model.Config {
	return model.Config{
		ModelFiles:    mpSimpleVision.ModelFiles,
		ProjFile:      mpSimpleVision.ProjFile,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       2048,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
	}
}

func cfgEmbed() model.Config {
	return model.Config{
		ModelFiles:     mpEmbed.ModelFiles,
		ContextWindow:  2048,
		NBatch:         2048,
		NUBatch:        512,
		CacheTypeK:     model.GGMLTypeQ8_0,
		CacheTypeV:     model.GGMLTypeQ8_0,
		FlashAttention: model.FlashAttentionEnabled,
	}
}

func cfgRerank() model.Config {
	return model.Config{
		ModelFiles:     mpRerank.ModelFiles,
		ContextWindow:  2048,
		NBatch:         2048,
		NUBatch:        512,
		CacheTypeK:     model.GGMLTypeQ8_0,
		CacheTypeV:     model.GGMLTypeQ8_0,
		FlashAttention: model.FlashAttentionEnabled,
	}
}

func cfgAudio() model.Config {
	return model.Config{
		ModelFiles:    mpAudio.ModelFiles,
		ProjFile:      mpAudio.ProjFile,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       2048,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
	}
}
