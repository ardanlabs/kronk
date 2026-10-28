package decisionapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestDecodeDecisionRequestPreservesOrder(t *testing.T) {
	input := `{
		"model":"decision-model",
		"state":{"z":1,"a":{"second":2,"first":1}},
		"questions":{
			"route":{
				"type":"choice",
				"instructions":{"goal":"route","priority":2},
				"criteria":{"billing":{"team":"payments"},"support":"technical"}
			},
			"urgency":{
				"type":"score",
				"instructions":"Rate urgency",
				"criteria":["low",{"label":"high"}]
			},
			"escalate":{
				"type":"noul",
				"instructions":"Escalate?",
				"criteria":{"false":"automate","true":{"action":"review"}}
			}
		}
	}`

	got, err := decodeDecisionRequest(strings.NewReader(input))
	if err != nil {
		t.Fatalf("decodeDecisionRequest: %v", err)
	}

	if got.Model != "decision-model" {
		t.Errorf("Model: got %q, want %q", got.Model, "decision-model")
	}
	if _, ok := got.State.(orderedObject); !ok {
		t.Fatalf("State: got %T, want orderedObject", got.State)
	}
	choiceCriteria := got.Questions[0].Criteria.(orderedObject)
	if got.Questions[0].Type.String() != "choice" || choiceCriteria[0].Name != "billing" {
		t.Errorf("Questions[0]: got %+v, want app-owned choice question", got.Questions[0])
	}

	sdkRequest := got.toSDK()
	state, err := json.Marshal(sdkRequest.State)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if want := `{"z":1,"a":{"second":2,"first":1}}`; string(state) != want {
		t.Errorf("state: got %s, want %s", state, want)
	}

	questions := sdkRequest.Questions
	if len(questions) != 3 {
		t.Fatalf("questions: got %d, want 3", len(questions))
	}
	if questions[0].ID != "route" || questions[1].ID != "urgency" || questions[2].ID != "escalate" {
		t.Errorf("question order: got %q, %q, %q", questions[0].ID, questions[1].ID, questions[2].ID)
	}
	if questions[0].Options[0].Name != "billing" || questions[0].Options[1].Name != "support" {
		t.Errorf("option order: got %q, %q", questions[0].Options[0].Name, questions[0].Options[1].Name)
	}

	instructions, err := json.Marshal(questions[0].Instructions)
	if err != nil {
		t.Fatalf("marshal instructions: %v", err)
	}
	if want := `{"goal":"route","priority":2}`; string(instructions) != want {
		t.Errorf("instructions: got %s, want %s", instructions, want)
	}

	criteria := questions[2].NoulCriteria
	if criteria == nil {
		t.Fatal("noul criteria: got nil, want values")
	}
	trueValue, err := json.Marshal(criteria.True)
	if err != nil {
		t.Fatalf("marshal true criteria: %v", err)
	}
	if want := `{"action":"review"}`; string(trueValue) != want {
		t.Errorf("true criteria: got %s, want %s", trueValue, want)
	}
}

func TestDecodeDecisionRequestAllowsProtocolDefaultInstructions(t *testing.T) {
	got, err := decodeDecisionRequest(strings.NewReader(`{"model":"d3","state":null,"questions":{"q":{"type":"noul"}}}`))
	if err != nil {
		t.Fatalf("decode default instructions: %v", err)
	}
	req := got.toSDK()
	if len(req.Questions) != 1 || req.Questions[0].Instructions != nil {
		t.Errorf("default instructions: got %+v", req)
	}
}

func TestDecisionImagesPreserveOrderAndState(t *testing.T) {
	got, err := decodeDecisionRequest(strings.NewReader(`{"model":"clef","state":{"image_url":"ordinary state"},"images":["data:image/png;base64,iVBORw0KGgo=","/9j/4A=="],"questions":{"q":{"type":"noul","instructions":"Q?"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	req := got.toSDK()
	if len(req.Images) != 2 || !bytes.Equal(req.Images[0], []byte("\x89PNG\r\n\x1a\n")) || !bytes.Equal(req.Images[1], []byte{0xff, 0xd8, 0xff, 0xe0}) {
		t.Fatalf("images: got %v, want PNG then JPEG bytes", req.Images)
	}
	state, err := json.Marshal(req.State)
	if err != nil {
		t.Fatal(err)
	}
	if string(state) != `{"image_url":"ordinary state"}` {
		t.Errorf("state: got %s, want unchanged structured state", state)
	}
}

func TestDecisionFilesPreserveOrderAndState(t *testing.T) {
	// JSON field order does not change the SDK's images-before-files contract.
	input := `{"model":"lfm3-d1-omni","files":["data:audio/wav;base64,UklGRg==","data:image/png;base64,iVBORw0KGgo=","/9j/4A==","UklGRg=="],"images":["/9j/4A=="],"state":{"files":["ordinary state"],"type":"input_audio"},"questions":{"q":{"type":"noul"}}}`
	got, err := decodeDecisionRequest(strings.NewReader(input))
	if err != nil {
		t.Fatalf("decodeDecisionRequest: %v", err)
	}
	req := got.toSDK()
	if len(req.Images) != 1 || !bytes.Equal(req.Images[0], []byte{0xff, 0xd8, 0xff, 0xe0}) {
		t.Fatalf("images: got %v, want JPEG bytes", req.Images)
	}
	want := [][]byte{[]byte("RIFF"), []byte("\x89PNG\r\n\x1a\n"), {0xff, 0xd8, 0xff, 0xe0}, []byte("RIFF")}
	if len(req.Files) != len(want) {
		t.Fatalf("files: got %d, want %d", len(req.Files), len(want))
	}
	for i, data := range want {
		if !bytes.Equal(req.Files[i], data) {
			t.Errorf("files[%d]: got %v, want %v", i, req.Files[i], data)
		}
	}
	state, err := json.Marshal(req.State)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if want := `{"files":["ordinary state"],"type":"input_audio"}`; string(state) != want {
		t.Errorf("state: got %s, want %s", state, want)
	}
}

func TestDecisionAudioInstructions(t *testing.T) {
	for _, encoded := range []string{"UklGRg==", "data:audio/wav;base64,UklGRg=="} {
		t.Run(encoded, func(t *testing.T) {
			input := `{"model":"lfm3-d1-omni","state":null,"questions":{"choice":{"type":"choice","instructions":{"type":"input_audio","input_audio":{"data":"` + encoded + `","format":"wav"}},"criteria":{"a":"A","b":"B"}},"score":{"type":"score","instructions":{"type":"input_audio","input_audio":{"data":"` + encoded + `","format":"wav"}},"criteria":["low","high"]},"noul":{"type":"noul","instructions":{"type":"input_audio","input_audio":{"data":"` + encoded + `","format":"wav"}}}}}`
			got, err := decodeDecisionRequest(strings.NewReader(input))
			if err != nil {
				t.Fatalf("decodeDecisionRequest: %v", err)
			}
			for _, question := range got.toSDK().Questions {
				audio, ok := question.Instructions.(model.DecisionAudioInstructions)
				if !ok {
					t.Fatalf("%s instructions: got %T, want DecisionAudioInstructions", question.ID, question.Instructions)
				}
				if !bytes.Equal(audio.Data, []byte("RIFF")) {
					t.Errorf("%s audio: got %v, want RIFF bytes", question.ID, audio.Data)
				}
			}
		})
	}
}

func TestDecisionOrdinaryInstructionsUnchanged(t *testing.T) {
	for _, instructions := range []string{
		`{"type":"INPUT_AUDIO","input_audio":{"data":"not base64"}}`,
		`{"type":"other","input_audio":{"data":"https://example.com/audio.wav"}}`,
		`{"type":{"nested":"input_audio"},"goal":"ordinary"}`,
		`{"input_audio":{"data":"not base64"},"nested":{"type":"input_audio"}}`,
		`[{"type":"input_audio","input_audio":{"data":"not base64"}}]`,
	} {
		t.Run(instructions, func(t *testing.T) {
			input := `{"model":"m","state":{},"questions":{"q":{"type":"noul","instructions":` + instructions + `}}}`
			got, err := decodeDecisionRequest(strings.NewReader(input))
			if err != nil {
				t.Fatalf("decodeDecisionRequest: %v", err)
			}
			data, err := json.Marshal(got.toSDK().Questions[0].Instructions)
			if err != nil {
				t.Fatalf("marshal instructions: %v", err)
			}
			if string(data) != instructions {
				t.Errorf("instructions: got %s, want %s", data, instructions)
			}
		})
	}
}

func TestDecisionRejectsInvalidFilesAndAudioInstructions(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "files object", input: `"files":{}`},
		{name: "file number", input: `"files":[1]`},
		{name: "file null", input: `"files":[null]`},
		{name: "empty file", input: `"files":[""]`},
		{name: "malformed file", input: `"files":["%%%"]`},
		{name: "remote file", input: `"files":["https://example.com/audio.wav"]`},
		{name: "video data URL", input: `"files":["data:video/mp4;base64,UklGRg=="]`},
		{name: "text data URL", input: `"files":["data:text/plain;base64,UklGRg=="]`},
		{name: "non-base64 data URL", input: `"files":["data:audio/wav,UklGRg=="]`},
		{name: "empty data URL", input: `"files":["data:audio/wav;base64,"]`},
		{name: "empty media subtype", input: `"files":["data:audio/;base64,UklGRg=="]`},
		{name: "audio object missing", input: `"instructions":{"type":"input_audio"}`},
		{name: "audio object null", input: `"instructions":{"type":"input_audio","input_audio":null}`},
		{name: "audio object array", input: `"instructions":{"type":"input_audio","input_audio":[]}`},
		{name: "audio object string", input: `"instructions":{"type":"input_audio","input_audio":"UklGRg=="}`},
		{name: "audio data missing", input: `"instructions":{"type":"input_audio","input_audio":{}}`},
		{name: "audio data number", input: `"instructions":{"type":"input_audio","input_audio":{"data":42}}`},
		{name: "audio data empty", input: `"instructions":{"type":"input_audio","input_audio":{"data":""}}`},
		{name: "audio data malformed", input: `"instructions":{"type":"input_audio","input_audio":{"data":"%%%"}}`},
		{name: "remote audio", input: `"instructions":{"type":"input_audio","input_audio":{"data":"https://example.com/audio.wav"}}`},
		{name: "image instruction URL", input: `"instructions":{"type":"input_audio","input_audio":{"data":"data:image/png;base64,iVBORw0KGgo="}}`},
		{name: "video instruction URL", input: `"instructions":{"type":"input_audio","input_audio":{"data":"data:video/mp4;base64,UklGRg=="}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{"model":"m","state":{},` + tt.input + `,"questions":{"q":{"type":"noul"}}}`
			if strings.HasPrefix(tt.input, `"instructions"`) {
				input = `{"model":"m","state":{},"questions":{"q":{"type":"noul",` + tt.input + `}}}`
			}
			if _, err := decodeDecisionRequest(strings.NewReader(input)); err == nil {
				t.Fatal("decodeDecisionRequest: got nil error, want error")
			}
		})
	}
}

func TestDecodeDecisionRequestRejectsInvalidWireShapes(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty body", input: ``},
		{name: "array request", input: `[]`},
		{name: "missing model", input: `{"state":{},"questions":{"q":{"type":"noul","instructions":"Q?"}}}`},
		{name: "empty model", input: `{"model":" ","state":{},"questions":{"q":{"type":"noul","instructions":"Q?"}}}`},
		{name: "missing state", input: `{"model":"m","questions":{"q":{"type":"noul","instructions":"Q?"}}}`},
		{name: "missing questions", input: `{"model":"m","state":{}}`},
		{name: "questions array", input: `{"model":"m","state":{},"questions":[]}`},
		{name: "empty questions", input: `{"model":"m","state":{},"questions":{}}`},
		{name: "question scalar", input: `{"model":"m","state":{},"questions":{"q":true}}`},
		{name: "missing type", input: `{"model":"m","state":{},"questions":{"q":{"instructions":"Q?"}}}`},
		{name: "choice criteria array", input: `{"model":"m","state":{},"questions":{"q":{"type":"choice","instructions":"Q?","criteria":[]}}}`},
		{name: "score criteria object", input: `{"model":"m","state":{},"questions":{"q":{"type":"score","instructions":"Q?","criteria":{}}}}`},
		{name: "noul criteria array", input: `{"model":"m","state":{},"questions":{"q":{"type":"noul","instructions":"Q?","criteria":[]}}}`},
		{name: "unknown type", input: `{"model":"m","state":{},"questions":{"q":{"type":"other","instructions":"Q?"}}}`},
		{name: "multiple values", input: `{"model":"m","state":{},"questions":{"q":{"type":"noul","instructions":"Q?"}}} {}`},
		{name: "images object", input: `{"model":"m","state":{},"images":{},"questions":{"q":{"type":"noul"}}}`},
		{name: "image number", input: `{"model":"m","state":{},"images":[1],"questions":{"q":{"type":"noul"}}}`},
		{name: "image URL", input: `{"model":"m","state":{},"images":["https://example.com/image.png"],"questions":{"q":{"type":"noul"}}}`},
		{name: "malformed base64", input: `{"model":"m","state":{},"images":["%%%"],"questions":{"q":{"type":"noul"}}}`},
		{name: "empty image", input: `{"model":"m","state":{},"images":[""],"questions":{"q":{"type":"noul"}}}`},
		{name: "audio data URL", input: `{"model":"m","state":{},"images":["data:audio/wav;base64,UklGRg=="],"questions":{"q":{"type":"noul"}}}`},
		{name: "video", input: `{"model":"m","state":{},"videos":["video"],"questions":{"q":{"type":"noul"}}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeDecisionRequest(strings.NewReader(tt.input)); err == nil {
				t.Fatal("decodeDecisionRequest: got nil error, want error")
			}
		})
	}
}

func TestDecisionRoutes(t *testing.T) {
	app := web.NewApp(func(context.Context, string, ...any) {})
	Routes(app, Config{
		AuthorizationMode: auth.Open,
		InferenceTimeout:  time.Minute,
	})

	for _, path := range []string{"/v1/systemone", "/v1/decide"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			resp := httptest.NewRecorder()
			app.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Errorf("status: got %d, want %d", resp.Code, http.StatusBadRequest)
			}
		})
	}
}
