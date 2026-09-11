package imageapp

import (
	"bufio"
	"context"
	"encoding/json"
	"image"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/malinaprogress"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

func TestProgressEvents(t *testing.T) {
	progress := malinaprogress.New()
	app := web.NewApp(func(context.Context, string, ...any) {})
	Routes(app, Config{
		MalinaProgress:    progress,
		AuthorizationMode: auth.Open,
	})
	server := httptest.NewServer(app)
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/images/events", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer response.Body.Close()
	defer cancel()

	if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type: got %q, want %q", got, "text/event-stream")
	}
	reader := bufio.NewReader(response.Body)
	connected := readProgressEvent(t, reader)
	if connected.Scope != "global" || connected.Status != "connected" {
		t.Errorf("connected event: got %+v, want global connected event", connected)
	}

	progress.Publish(2, 8, 0.25)
	update := readProgressEvent(t, reader)
	if update.Scope != "global" || update.Status != "progress" || update.Step != 2 || update.Steps != 8 || update.Percent != 25 || update.SecondsPerStep != 0.25 {
		t.Errorf("progress event: got %+v, want global step 2 of 8 at 25 percent", update)
	}
}

func readProgressEvent(t *testing.T, reader *bufio.Reader) progressEvent {
	t.Helper()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("ReadString() error = %v", err)
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		var event progressEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data: "))), &event); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		return event
	}
}

func TestGenerationRequestDefaults(t *testing.T) {
	req := generationRequest{Model: "sd-1.5", Prompt: "lighthouse"}

	params, err := req.params()
	if err != nil {
		t.Fatalf("params() error = %v", err)
	}
	if params.Width != 512 || params.Height != 512 {
		t.Errorf("dimensions: got %dx%d, want 512x512", params.Width, params.Height)
	}
	if params.Steps != 20 || params.CFGScale != 7 || params.Seed != -1 {
		t.Errorf("defaults: got steps=%d cfg-scale=%v seed=%d", params.Steps, params.CFGScale, params.Seed)
	}
}

func TestGenerationRequestOptions(t *testing.T) {
	steps := 30
	cfgScale := float32(4.5)
	seed := int64(42)
	req := generationRequest{
		Model:          "sd-1.5",
		Prompt:         "lighthouse",
		NegativePrompt: "fog",
		Size:           "768x512",
		N:              1,
		ResponseFormat: "b64_json",
		Steps:          &steps,
		CFGScale:       &cfgScale,
		Seed:           &seed,
	}

	params, err := req.params()
	if err != nil {
		t.Fatalf("params() error = %v", err)
	}
	if params.Width != 768 || params.Height != 512 || params.Steps != steps || params.CFGScale != cfgScale || params.Seed != seed {
		t.Errorf("params: got %+v", params)
	}
	if params.NegativePrompt != req.NegativePrompt {
		t.Errorf("NegativePrompt: got %q, want %q", params.NegativePrompt, req.NegativePrompt)
	}
}

func TestGenerationRequestRejectsUnsupportedValues(t *testing.T) {
	tests := []struct {
		name string
		req  generationRequest
	}{
		{name: "missing model", req: generationRequest{Prompt: "image"}},
		{name: "missing prompt", req: generationRequest{Model: "sd-1.5"}},
		{name: "multiple images", req: generationRequest{Model: "sd-1.5", Prompt: "image", N: 2}},
		{name: "url response", req: generationRequest{Model: "sd-1.5", Prompt: "image", ResponseFormat: "url"}},
		{name: "malformed size", req: generationRequest{Model: "sd-1.5", Prompt: "image", Size: "512"}},
		{name: "invalid dimensions", req: generationRequest{Model: "sd-1.5", Prompt: "image", Size: "100x100"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.req.params(); err == nil {
				t.Fatal("params() error = nil, want error")
			}
		})
	}
}

func TestEditParams(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 130, 99))
	req := generationRequest{Model: "sd-1.5", Prompt: "watercolor"}

	params, err := editParams(req, source, 0.6)
	if err != nil {
		t.Fatalf("editParams() error = %v", err)
	}
	if params.Width != 128 || params.Height != 96 {
		t.Errorf("dimensions: got %dx%d, want 128x96", params.Width, params.Height)
	}
	if params.InitImage.Bounds().Dx() != 128 || params.InitImage.Bounds().Dy() != 96 {
		t.Errorf("init image dimensions: got %dx%d, want 128x96", params.InitImage.Bounds().Dx(), params.InitImage.Bounds().Dy())
	}
	if params.Strength != 0.6 {
		t.Errorf("Strength: got %v, want 0.6", params.Strength)
	}
}

func TestParseEditRequestOptions(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/images/edits", nil)
	r.MultipartForm = &multipart.Form{Value: map[string][]string{
		"model":           {"sd-1.5"},
		"prompt":          {"watercolor"},
		"negative_prompt": {"blurry"},
		"size":            {"768x512"},
		"steps":           {"30"},
		"cfg_scale":       {"4.5"},
		"seed":            {"42"},
		"strength":        {"0.6"},
	}}

	req, strength, err := parseEditRequest(r)
	if err != nil {
		t.Fatalf("parseEditRequest() error = %v", err)
	}
	if req.Model != "sd-1.5" || req.Prompt != "watercolor" || req.NegativePrompt != "blurry" || req.Size != "768x512" {
		t.Errorf("request strings: got %+v", req)
	}
	if req.Steps == nil || *req.Steps != 30 || req.CFGScale == nil || *req.CFGScale != 4.5 || req.Seed == nil || *req.Seed != 42 {
		t.Errorf("request controls: got %+v", req)
	}
	if strength != 0.6 {
		t.Errorf("strength: got %v, want 0.6", strength)
	}
}

func TestEditParamsRejectsStrength(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 128, 128))
	req := generationRequest{Model: "sd-1.5", Prompt: "watercolor"}

	if _, err := editParams(req, source, 0); err == nil {
		t.Fatal("editParams() error = nil, want error")
	}
}

func TestGenerationSizeScalesAndAligns(t *testing.T) {
	width, height, err := generationSize(image.Rect(0, 0, 1600, 901))
	if err != nil {
		t.Fatalf("generationSize() error = %v", err)
	}
	if width != 1024 || height != 576 {
		t.Errorf("generationSize(): got %dx%d, want 1024x576", width, height)
	}
}
