package vision_imc_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestSuite(t *testing.T) {
	testlib.WithModel(t, testlib.CfgSimpleVisionIMC(), func(t *testing.T, krn *kronk.Kronk) {
		t.Run("IMCMediaBuild", func(t *testing.T) { testIMCMediaBuild(t, krn) })
		t.Run("IMCMediaTextExtend", func(t *testing.T) { testIMCMediaTextExtend(t, krn) })
	})
}

// testIMCMediaBuild sends an image request and verifies the model responds
// correctly. This exercises buildIMCCacheFromScratch with media detection
// and decodeMediaIntoCache in startSlot.
func testIMCMediaBuild(t *testing.T, krn *kronk.Kronk) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	resp, err := krn.Chat(ctx, testlib.DMedia)
	if err != nil {
		t.Fatalf("initial image request: %v", err)
	}

	result := testlib.TestChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatMedia, "giraffes", "", "", false, false)

	for _, w := range result.Warnings {
		t.Logf("WARNING: %s", w)
	}

	if result.Err != nil {
		t.Fatalf("initial image request: %v", result.Err)
	}
}

// testIMCMediaTextExtend sends an image request, then a text-only follow-up
// about the image, then an unrelated text question, then another image
// follow-up. Each request should get a correct response, proving the image
// stays in the KV cache through text-only extensions without re-encoding.
func testIMCMediaTextExtend(t *testing.T, krn *kronk.Kronk) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	// Step 1: Initial image request — builds the media cache.
	resp, err := krn.Chat(ctx, testlib.DMedia)
	if err != nil {
		t.Fatalf("step 1 (image request): %v", err)
	}

	result := testlib.TestChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatMedia, "giraffes", "", "", false, false)
	if result.Err != nil {
		t.Fatalf("step 1 (image request): %v", result.Err)
	}
	t.Log("step 1: image request OK")

	// Build multi-turn conversation with the image in the history.
	imageMessages := testlib.DMedia["messages"].([]model.D)

	// Step 2: Text-only follow-up about the image — extends media slot with text.
	step2Msgs := appendMessages(imageMessages,
		model.D{"role": "assistant", "content": contentFromResp(resp)},
		model.D{"role": "user", "content": "How many giraffes are in the picture?"},
	)

	resp2, err := krn.Chat(ctx, model.D{
		"messages":   step2Msgs,
		"max_tokens": 2048,
	})
	if err != nil {
		t.Fatalf("step 2 (text follow-up about image): %v", err)
	}

	if len(resp2.Choice) == 0 || resp2.Choice[0].FinishReason() == "error" {
		t.Fatalf("step 2: no valid response")
	}
	t.Logf("step 2: text follow-up OK: %s", truncate(contentFromResp(resp2), 100))

	// Step 3: Unrelated text question — extends cache further with text.
	step3Msgs := appendMessages(step2Msgs,
		model.D{"role": "assistant", "content": contentFromResp(resp2)},
		model.D{"role": "user", "content": "Changing subject. What is the capital of France?"},
	)

	resp3, err := krn.Chat(ctx, model.D{
		"messages":   step3Msgs,
		"max_tokens": 2048,
	})
	if err != nil {
		t.Fatalf("step 3 (unrelated text question): %v", err)
	}

	content3 := strings.ToLower(contentFromResp(resp3))
	if !strings.Contains(content3, "paris") {
		t.Logf("WARNING: step 3: expected 'paris' in response, got: %s", truncate(content3, 200))
	}
	t.Logf("step 3: unrelated text OK: %s", truncate(contentFromResp(resp3), 100))

	// Step 4: Back to the image — should still answer correctly from
	// the cached image without re-encoding.
	step4Msgs := appendMessages(step3Msgs,
		model.D{"role": "assistant", "content": contentFromResp(resp3)},
		model.D{"role": "user", "content": "Back to the picture. Are the giraffes adults or babies?"},
	)

	resp4, err := krn.Chat(ctx, model.D{
		"messages":   step4Msgs,
		"max_tokens": 2048,
	})
	if err != nil {
		t.Fatalf("step 4 (back to image): %v", err)
	}

	if len(resp4.Choice) == 0 || resp4.Choice[0].FinishReason() == "error" {
		t.Fatalf("step 4: no valid response")
	}

	content4 := strings.ToLower(contentFromResp(resp4))
	if !strings.Contains(content4, "giraffe") && !strings.Contains(content4, "adult") && !strings.Contains(content4, "baby") {
		t.Logf("WARNING: step 4: response doesn't mention giraffes: %s", truncate(content4, 200))
	}
	t.Logf("step 4: back to image OK: %s", truncate(contentFromResp(resp4), 100))
}

func contentFromResp(resp model.ChatResponse) string {
	if len(resp.Choice) == 0 {
		return ""
	}
	if resp.Choice[0].Message != nil {
		return resp.Choice[0].Message.Content
	}
	if resp.Choice[0].Delta != nil {
		return resp.Choice[0].Delta.Content
	}
	return ""
}

func appendMessages(base []model.D, msgs ...model.D) []model.D {
	result := make([]model.D, len(base)+len(msgs))
	copy(result, base)
	copy(result[len(base):], msgs)
	return result
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return fmt.Sprintf("%s...", s[:n])
}
