package chatapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/malinaprogress"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mux"
)

func TestImageProgressEvents(t *testing.T) {
	progress := malinaprogress.New()
	server := httptest.NewTestServer(t, newTransportAPI(t, mux.Config{MalinaProgress: progress}))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://kronk/v1/images/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("SSE: got status %d headers %v", response.StatusCode, response.Header)
	}
	reader := bufio.NewReader(response.Body)
	connected := readImageProgressEvent(t, reader)
	if connected.Scope != "global" || connected.Status != "connected" {
		t.Errorf("connected event: got %+v, want global connected", connected)
	}
	progress.Publish(2, 8, 0.25)
	update := readImageProgressEvent(t, reader)
	if update.Scope != "global" || update.Status != "progress" || update.Step != 2 || update.Steps != 8 || update.Percent != 25 || update.SecondsPerStep != 0.25 {
		t.Errorf("progress event: got %+v, want global step 2 of 8 at 25 percent", update)
	}
}

type imageProgressEvent struct {
	Scope          string  `json:"scope"`
	Status         string  `json:"status"`
	Step           int     `json:"step"`
	Steps          int     `json:"steps"`
	Percent        float64 `json:"percent"`
	SecondsPerStep float32 `json:"seconds_per_step"`
}

func readImageProgressEvent(t *testing.T, reader *bufio.Reader) imageProgressEvent {
	t.Helper()
	var data string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if value, ok := strings.CutPrefix(line, "data: "); ok {
			data = strings.TrimSpace(value)
		}
		if line == "\n" && data != "" {
			var event imageProgressEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				t.Fatal(err)
			}
			return event
		}
	}
}

func TestImageEditMultipartValidation(t *testing.T) {
	api := newTransportAPI(t, mux.Config{})
	var table []apitest.Table
	for _, tt := range []struct{ field, value, want string }{
		{"steps", "bad", "field[steps] must be an integer"},
		{"seed", "bad", "field[seed] must be an integer"},
		{"cfg_scale", "bad", "field[cfg_scale] must be a number"},
		{"strength", "bad", "field[strength] must be a number"},
		{"n", "bad", "field[n] must be an integer"},
		{"model", "", "model is required"},
		{"prompt", "", "prompt"},
		{"size", "bad", "size must use WIDTHxHEIGHT"},
		{"response_format", "url", "response_format must be b64_json"},
		{"n", "2", "n must be 1"},
		{"strength", "0", "strength"},
		{"steps", "0", "steps"},
		{"cfg_scale", "-1", "CFG scale"},
	} {
		fields := map[string]string{"model": "sd-1.5", "prompt": "watercolor", "negative_prompt": "blurry", "size": "128x128", "steps": "2", "cfg_scale": "4.5", "seed": "42", "strength": "0.6"}
		fields[tt.field] = tt.value
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		for field, value := range fields {
			if err := form.WriteField(field, value); err != nil {
				t.Fatal(err)
			}
		}
		file, err := form.CreateFormFile("image", "source.png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 128, 128))); err != nil {
			t.Fatal(err)
		}
		if err := form.Close(); err != nil {
			t.Fatal(err)
		}
		table = append(table, apitest.Table{
			Name:   tt.field + "=" + tt.value,
			Method: http.MethodPost, URL: "/v1/images/edits",
			RawBody: body.Bytes(), Headers: map[string]string{"Content-Type": form.FormDataContentType()},
			StatusCode: http.StatusBadRequest,
			ResponseCmpFunc: func(header http.Header, body []byte) string {
				if diff := jsonContentType(header, body); diff != "" {
					return diff
				}
				if !strings.Contains(string(body), tt.want) {
					return fmt.Sprintf("validation: got body %s, want containing %q", body, tt.want)
				}
				return ""
			},
		})
	}
	api.Run(t, table, "image edit validation")
}
