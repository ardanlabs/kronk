package chatapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

// Test_API_Media keeps additional fixtures off the ordinary hosted API tier.
func Test_API_Media(t *testing.T) {
	if os.Getenv("KRONK_TEST_MEDIA") != "yes" {
		t.Skip("set KRONK_TEST_MEDIA=yes to qualify installed GPU media fixtures")
	}
	test := apitest.NewWithModelConfig(t, "Test_API_Media", "testdata/model_config_media.yaml")
	token := createTokens(t, test.Sec)["chat-completions"]
	t.Run("omni", func(t *testing.T) {
		const id = "ggml-org/Qwen2.5-Omni-3B-Q4_K_M"
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := test.Pool.Kronk.InvalidateSync(ctx, id); err != nil {
				t.Error(err)
			}
		}()
		krn, err := test.Pool.Kronk.AquireModel(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		testlib.TestModelProfile(t, krn, "Dense", "Attention-only", "qwen2vl")
		chatMediaState(t, test, token, id)
	})
	for _, fixture := range []struct {
		name, id, mode, mixing string
		batch                  int
		nonCausal              bool
	}{
		{"qwen-shared", visionModelID + "/MediaShared", "shared", "supported", 2048, false},
		{"qwen-size-isolated", visionModelID + "/MediaIsolated", "isolated", "supported", 32, false},
		{"gemma-noncausal-isolated", "unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M/MediaIsolated", "isolated", "", 2048, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := test.Pool.Kronk.InvalidateSync(ctx, fixture.id); err != nil {
					t.Error(err)
				}
			}()
			chatMediaScheduling(t, test, token, fixture.id, fixture.mode, fixture.mixing, fixture.batch, fixture.nonCausal)
		})
	}
}

func imageChatRequest(t *testing.T, file, prompt string) model.D {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(gw, "examples", "samples", file))
	if err != nil {
		t.Fatal(err)
	}
	return model.D{
		"messages": model.ImageMessage(prompt, data, "jpeg"), "temperature": 0.0,
		"max_tokens": 64, "enable_thinking": false, "stream_options": model.D{"include_usage": true},
	}
}

func mediaCompletion(t *testing.T, result liveChatResult, modelID string) (model.ChatResponse, string) {
	t.Helper()
	if result.err != nil || !result.done {
		t.Fatalf("incomplete media stream: done=%t error=%v", result.done, result.err)
	}
	if diff := validateChatUsageStreamReasoning(result.events, false); diff != "" {
		t.Fatal(diff)
	}
	var terminal model.ChatResponse
	var text strings.Builder
	for _, event := range result.events {
		var chunk model.ChatResponse
		if err := json.Unmarshal(event, &chunk); err != nil {
			t.Fatal(err)
		}
		if chunk.Model != modelID {
			t.Fatalf("response model: got %q, want %q", chunk.Model, modelID)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta != nil {
				text.WriteString(choice.Delta.Content)
			}
			if choice.FinishReason() != "" {
				terminal = chunk
			}
		}
		if chunk.Usage != nil {
			terminal.Usage = chunk.Usage
		}
	}
	return terminal, text.String()
}

func chatMediaState(t *testing.T, test *apitest.Test, token, id string) {
	t.Helper()
	krn, err := test.Pool.Kronk.AquireModel(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	cfg := krn.ModelConfig()
	if want := "mmproj-" + strings.Split(id, "/")[1] + ".gguf"; filepath.Base(cfg.ProjFile) != want || !cfg.IncrementalCache() {
		t.Fatalf("unexpected projector/cache configuration: %s", cfg.String())
	}
	t.Logf("media configuration: %s", cfg.String())
	server := httptest.NewServer(test)
	t.Cleanup(server.Close)
	defer func() {
		waitMediaState(t, krn, "native media cleanup before unload", func(model.BatchEngineSnapshot) bool {
			return krn.ActiveStreams() == 0
		})
	}()
	accepted := 0
	chat := func(t *testing.T, ctx context.Context, request model.D) (model.ChatResponse, string, error) {
		t.Helper()
		request["model"], request["stream"] = id, true
		stream := startLiveChat(ctx, t, server.Client(), server.URL, token, request)
		resp, text := mediaCompletion(t, stream.result(t), id)
		if err := validateSpeculationUsage(resp.Usage, cfg.Speculation == model.SpeculationMTP); err != nil {
			return resp, text, err
		}
		accepted += resp.Usage.DraftAcceptedTokens
		return resp, text, nil
	}
	testlib.TestMediaHistory(t, func(ctx context.Context, request model.D) (model.ChatResponse, string, error) {
		return chat(t, ctx, request)
	})
	if cfg.Speculation == model.SpeculationMTP {
		request := imageChatRequest(t, "giraffe.jpg", "Describe the animals and their appearance in four detailed sentences.")
		request["max_tokens"] = 192
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		_, text, err := chat(t, ctx, request)
		if err != nil || !strings.Contains(strings.ToLower(text), "giraffe") || strings.Contains(strings.ToLower(text), "deer") {
			t.Fatalf("MTP image description: text=%q error=%v", text, err)
		}
		if accepted == 0 {
			t.Fatal("image/IMC workload accepted no draft tokens")
		}
		t.Logf("image/IMC workload accepted %d draft tokens", accepted)
	}
	t.Run("cancel-image-and-recover", func(t *testing.T) {
		request := imageChatRequest(t, "giraffe.jpg", "Describe this picture in great detail, then write a long essay about the animals and their habitat.")
		request["model"], request["stream"], request["max_tokens"] = id, true, 2048
		stream := startLiveChat(t.Context(), t, server.Client(), server.URL, token, request)
		first := stream.content(t)
		waitMediaState(t, krn, "image generation", func(snapshot model.BatchEngineSnapshot) bool {
			for _, slot := range snapshot.Slots {
				if slot.RequestID == first.ID && slot.Phase == "generation" {
					return true
				}
			}
			return false
		})
		stream.cancel()
		stream.interrupted(t)
		waitMediaState(t, krn, "server-side image cancellation", func(model.BatchEngineSnapshot) bool {
			if krn.ActiveStreams() != 0 {
				return false
			}
			for _, record := range test.ModelEvents() {
				if record.Attributes["status"] == "cancel" && record.Attributes["id"] == first.ID {
					return true
				}
			}
			return false
		})
		for _, fixture := range []struct{ file, want, foreign string }{{"deer/image10.jpg", "deer", "giraffe"}, {"giraffe.jpg", "giraffe", "deer"}} {
			_, text, err := chat(t, t.Context(), imageChatRequest(t, fixture.file, "Name the animal in the picture. Reply with only the animal name."))
			if err != nil || !strings.Contains(strings.ToLower(text), fixture.want) || strings.Contains(strings.ToLower(text), fixture.foreign) {
				t.Fatalf("image recovery: got %q error=%v, want %s without %s", text, err, fixture.want, fixture.foreign)
			}
		}
	})
}

func waitMediaState(t *testing.T, krn *kronk.Kronk, label string, ready func(model.BatchEngineSnapshot) bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot, ok := krn.BatchEngineSnapshot()
		if ok && ready(snapshot) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s not observed: snapshot=%+v", label, snapshot)
		case <-ticker.C:
		}
	}
}

func chatMediaScheduling(t *testing.T, test *apitest.Test, token, id, mode, mixing string, batch int, nonCausal bool) {
	t.Helper()
	testlib.SkipOnBackends(t, "two sequences sharing one decode batch come back with corrupted logits", "rocm")
	krn, err := test.Pool.Kronk.AquireModel(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	cfg := krn.ModelConfig()
	if cfg.IncrementalCache() || cfg.Speculation != model.SpeculationDisabled || cfg.NSeqMax() != 2 || cfg.PrefillBatchSize() != batch {
		t.Fatalf("unexpected media scheduling configuration: %s", cfg.String())
	}
	server := httptest.NewServer(test)
	t.Cleanup(server.Close)
	request := testlib.StateRequest("ALPHA", "DELTA", 101)
	request["messages"] = []model.D{{"role": "user", "content": "Count from 1 to 2000, one number per line. Do not abbreviate or stop early."}}
	request["model"], request["stream"], request["max_tokens"] = id, true, 4096
	holder := startLiveChat(t.Context(), t, server.Client(), server.URL, token, request)
	defer func() {
		holder.cancel()
		waitMediaState(t, krn, "native stream cleanup before unload", func(model.BatchEngineSnapshot) bool {
			return krn.ActiveStreams() == 0
		})
	}()
	first := holder.content(t)
	generated := 0
	waitMediaState(t, krn, "text generation before media", func(snapshot model.BatchEngineSnapshot) bool {
		for _, slot := range snapshot.Slots {
			if slot.RequestID == first.ID && slot.Phase == "generation" {
				generated = slot.GeneratedTokens
				return true
			}
		}
		return false
	})
	marker := len(test.ModelEvents())
	started := time.Now()
	request = imageChatRequest(t, "giraffe.jpg", "Name the animal in the picture. Reply with only the animal name.")
	request["model"], request["stream"] = id, true
	stream := startLiveChat(t.Context(), t, server.Client(), server.URL, token, request)
	_, text := mediaCompletion(t, stream.result(t), id)
	if !strings.Contains(strings.ToLower(text), "giraffe") || strings.Contains(strings.ToLower(text), "deer") {
		t.Fatalf("scheduled media answer: got %q, want giraffe without deer", text)
	}
	waitMediaState(t, krn, "text progress during media request", func(snapshot model.BatchEngineSnapshot) bool {
		for _, slot := range snapshot.Slots {
			if slot.RequestID == first.ID && slot.Phase == "generation" && slot.GeneratedTokens > generated {
				return true
			}
		}
		return false
	})
	ended := time.Now()
	holder.cancel()
	result := holder.result(t)
	if !result.readErr || result.done {
		t.Fatal("text holder completed before media overlap was established")
	}
	last, gap := started, time.Duration(0)
	progress := 0
	for _, at := range result.contentTimes {
		if at.Before(started) || at.After(ended) {
			continue
		}
		progress++
		gap = max(gap, at.Sub(last))
		last = at
	}
	gap = max(gap, ended.Sub(last))
	if progress == 0 || gap > 15*time.Second {
		t.Fatalf("text stalled during media: content chunks=%d max gap=%s", progress, gap)
	}
	var plan map[string]any
	qualified := false
	for _, record := range test.ModelEvents()[marker:] {
		if record.Message == "batch-engine" && record.Attributes["status"] == "media-plan" {
			plan = record.Attributes
		}
		if record.Message != "prefill-media" || record.Attributes["status"] != "encoding-shared" && record.Attributes["status"] != "encoding-image" {
			continue
		}
		if plan == nil || plan["slot"] != record.Attributes["slot"] {
			t.Fatal("image encoding has no matching media plan")
		}
		rows, err := strconv.Atoi(fmt.Sprint(plan["rows"]))
		if err != nil || fmt.Sprint(plan["mode"]) != mode || fmt.Sprint(plan["input-mixing"]) != mixing || plan["non-causal"] != nonCausal {
			t.Fatalf("unexpected image scheduling plan: %+v", plan)
		}
		if !nonCausal && mode == "isolated" && rows <= batch || nonCausal && rows > batch {
			t.Fatalf("image isolation cause not qualified: rows=%d batch=%d plan=%+v", rows, batch, plan)
		}
		qualified = true
		t.Logf("native image scheduling=%v; concurrent content chunks=%d max gap=%s", plan, progress, gap)
	}
	if !qualified {
		t.Fatal("required native image scheduling path was not observed")
	}
}
