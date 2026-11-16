// These fixtures and native diagnostic assertions are test-only. The runnable
// example uses only the public storage contract and response usage.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"sync"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/imc"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const visionQuestion = "What color fills this image? Reply with only the color name."
const repeatedCode = "713 713 713 713 713 713 713 713"

func initialRequest(scenario string) (model.D, error) {
	messages := []model.D{
		{"role": "system", "content": "You are a concise assistant. Follow the user's instructions exactly."},
		{"role": "user", "content": "Remember this project code: 713. We are building a disk backed conversation cache."},
		{"role": "assistant", "content": "I will remember project code 713."},
		{"role": "user", "content": followUp(scenario)},
	}

	switch scenario {
	case "text", "mtp-own", "mtp-shared":
	case "vision":
		data, err := colorImage(false)
		if err != nil {
			return nil, err
		}
		messages = model.ImageMessage(visionQuestion, data, "png")
	default:
		return nil, fmt.Errorf("unknown scenario %q", scenario)
	}

	return model.D{
		"messages": messages, "max_tokens": 128,
		"temperature": 0.0, "enable_thinking": false,
	}, nil
}

func followUp(scenario string) string {
	switch scenario {
	case "vision":
		return "What color fills the image shown earlier? Reply with only the color name."
	case "mtp-own", "mtp-shared":
		return "Repeat our project code exactly eight times, separated by spaces. Reply with only the eight numbers."
	default:
		return "What is our project code? Reply with only the number."
	}
}

func colorImage(blue bool) ([]byte, error) {
	c := color.NRGBA{R: 255, A: 255}
	if blue {
		c = color.NRGBA{B: 255, A: 255}
	}

	img := image.NewNRGBA(image.Rect(0, 0, 224, 224))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// inspectSnapshot uses only the public persistence contract. Vision additionally
// reads the payloads to check the persisted media, not private restore metadata.
func inspectSnapshot(ctx context.Context, store imc.Store, ref imc.Ref, scenario string, blue bool) (record imc.Record, retErr error) {
	handle, err := store.Open(ctx, ref)
	if err != nil {
		return record, err
	}

	defer func() { retErr = errors.Join(retErr, handle.Close()) }()

	record = handle.Record()
	target, draft, media := handle.Sizes()

	fmt.Printf("Uncompressed target/draft: %d/%d bytes; media items: %d\n", target, draft, len(media))

	if target <= 0 || (scenario == "mtp-own") != (draft > 0) {
		return record, errors.New("snapshot does not contain the expected target/draft state")
	}

	if scenario != "vision" {
		if len(media) != 0 {
			return record, errors.New("text snapshot unexpectedly contains media")
		}
		return record, nil
	}

	expected, err := colorImage(blue)
	if err != nil {
		return record, err
	}

	if len(media) != 1 || media[0] != int64(len(expected)) {
		return record, errors.New("vision snapshot has incorrect media sizes")
	}

	// This verifier deliberately allocates buffers to inspect a small vision
	// fixture. Bound that allocation independently of the engine's MaxBytes.
	if target > 512<<20 || draft != 0 {
		return record, errors.New("vision fixture exceeds the verifier's 512 MiB read limit")
	}

	payload := [][]byte{make([]byte, media[0])}
	if err := handle.Read(ctx, make([]byte, target), nil, payload); err != nil {
		return record, err
	}

	if !bytes.Equal(payload[0], expected) {
		return record, errors.New("persisted image differs from the requested image")
	}

	return record, nil
}

// Native restore evidence prevents a correct cold answer or target-only MTP
// fallback from passing. These diagnostic event names are engine-specific, not
// part of the Store contract. Keep them in step with the engine when it changes.
type evidence struct {
	mu     sync.Mutex
	target bool
	draft  bool
	resume string
}

func (e *evidence) log(ctx context.Context, message string, args ...any) {
	kronk.FmtLogger(ctx, message, args...)
	if message != "start-slot" && message != "batch-engine" {
		return
	}

	fields := make(map[string]any)

	for i := 0; i+1 < len(args); i += 2 {
		fields[fmt.Sprint(args[i])] = args[i+1]
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	switch fields["status"] {
	case "imc-restore-done":
		e.target = true
	case "imc-draft-restore-done":
		e.draft = fields["pending_h"] == true
	case "slot-finished":
		if source, ok := fields["draft_resume_source"].(string); ok {
			e.resume = source
		}
	}
}

func (e *evidence) check(scenario, phase string, response model.ChatResponse) error {
	expected := "713"

	switch scenario {
	case "vision":
		expected = "red"
		if phase == "changed-image" {
			expected = "blue"
		}
	case "mtp-own", "mtp-shared":
		expected = repeatedCode
	}

	answer := strings.Join(strings.Fields(response.Choices[0].Message.Content), " ")
	if answer != expected {
		return fmt.Errorf("answer: got %q, want %q", answer, expected)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	restoring := phase == "restore"
	cached := response.Usage.PromptTokensDetails.CachedTokens

	if e.target != restoring || (cached > 0) != restoring {
		return fmt.Errorf("restore evidence mismatch: native=%t cached=%d phase=%s", e.target, cached, phase)
	}

	if !strings.HasPrefix(scenario, "mtp-") {
		return nil
	}

	usage := response.Usage
	fmt.Printf("Drafted/accepted: %d/%d; resume source: %s; disable reason: %q\n",
		usage.DraftTokens, usage.DraftAcceptedTokens, e.resume, usage.DraftDisableReason)

	if usage.DraftTokens <= 0 || usage.DraftDisableReason != "" {
		return errors.New("MTP did not remain active")
	}

	if !restoring {
		return nil
	}

	expectedSource := "shared-target-kv"
	if scenario == "mtp-own" {
		expectedSource = "restored-draft-kv"
	}

	if e.resume != expectedSource || e.draft != (scenario == "mtp-own") {
		return fmt.Errorf("MTP restore mismatch: draft=%t resume=%q, want %q", e.draft, e.resume, expectedSource)
	}

	return nil
}
