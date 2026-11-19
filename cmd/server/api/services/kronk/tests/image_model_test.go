//go:build malina_integration

package chatapi_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

func TestImageGenerationModel(t *testing.T) {
	models, err := malinamodels.New()
	if err != nil {
		t.Fatalf("malinamodels.New() error = %v", err)
	}
	if _, err := models.FullPath(malinamodels.BundleSD15.String()); err != nil {
		t.Fatalf("%s bundle is not installed: %v", malinamodels.BundleSD15, err)
	}
	api := apitest.NewWithModelConfig(t, t.Name(), "testdata/model_config.yaml")
	token, err := api.Sec.GenerateToken(false, map[string]auth.RateLimit{
		"image-generations": {Window: auth.RateUnlimited},
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	var generated imageGenerationResponse
	api.Run(t, []apitest.Table{{
		Name: "generation", Method: http.MethodPost, URL: "/v1/images/generations",
		Token: token, Headers: map[string]string{"Content-Type": "application/json"},
		RawBody: []byte(`{
		"model":"sd-1.5",
		"prompt":"a red square",
		"size":"128x128",
		"steps":1,
		"seed":42
	}`),
		StatusCode: http.StatusOK, ResponseCmpFunc: jsonContentType,
		GotResp: &generated, ExpResp: int64(42), CmpFunc: compareImageGenerationResponse,
	}}, "image")
	if t.Failed() {
		return
	}

	// Reuse the generated PNG as the edit fixture, preserving the real
	// generation-to-edit round trip.
	data, err := base64.StdEncoding.DecodeString(generated.Data[0].B64JSON)
	if err != nil {
		t.Fatalf("decode b64_json: %v", err)
	}
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("model", "sd-1.5"); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("prompt", "a blue square"); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("steps", "2"); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("seed", "43"); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("strength", "0.6"); err != nil {
		t.Fatal(err)
	}
	for field, value := range map[string]string{"negative_prompt": "blurry", "cfg_scale": "4.5", "size": "128x128", "n": "1", "response_format": "b64_json"} {
		if err := form.WriteField(field, value); err != nil {
			t.Fatal(err)
		}
	}
	file, err := form.CreateFormFile("image", "source.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	api.Run(t, []apitest.Table{{
		Name: "edit", Method: http.MethodPost, URL: "/v1/images/edits",
		Token: token, RawBody: body.Bytes(),
		Headers:    map[string]string{"Content-Type": form.FormDataContentType()},
		StatusCode: http.StatusOK, ResponseCmpFunc: jsonContentType,
		GotResp: &imageGenerationResponse{}, ExpResp: int64(43), CmpFunc: compareImageGenerationResponse,
	}}, "image")
}

type imageGenerationResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		B64JSON string `json:"b64_json"`
		Seed    int64  `json:"seed"`
		Width   int    `json:"width"`
		Height  int    `json:"height"`
	} `json:"data"`
}

func compareImageGenerationResponse(got, exp any) string {
	generated := got.(*imageGenerationResponse)
	if generated.Created <= 0 {
		return fmt.Sprintf("created: got %d, want positive timestamp", generated.Created)
	}
	if len(generated.Data) != 1 {
		return fmt.Sprintf("images: got %d, want 1", len(generated.Data))
	}
	if generated.Data[0].Width != 128 || generated.Data[0].Height != 128 {
		return fmt.Sprintf("image dimensions: got %dx%d, want 128x128", generated.Data[0].Width, generated.Data[0].Height)
	}
	if generated.Data[0].Seed != exp.(int64) {
		return fmt.Sprintf("seed: got %d, want %d", generated.Data[0].Seed, exp.(int64))
	}
	data, err := base64.StdEncoding.DecodeString(generated.Data[0].B64JSON)
	if err != nil {
		return fmt.Sprintf("decode b64_json: %v", err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Sprintf("decode PNG: %v", err)
	}
	if decoded.Bounds().Dx() != 128 || decoded.Bounds().Dy() != 128 {
		return fmt.Sprintf("PNG dimensions: got %v, want 128x128", decoded.Bounds())
	}
	return ""
}
