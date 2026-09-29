package decisionapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
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
	if got.Questions[0].Type != "choice" || choiceCriteria[0].Name != "billing" {
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
		{name: "missing instructions", input: `{"model":"m","state":{},"questions":{"q":{"type":"noul"}}}`},
		{name: "choice criteria array", input: `{"model":"m","state":{},"questions":{"q":{"type":"choice","instructions":"Q?","criteria":[]}}}`},
		{name: "score criteria object", input: `{"model":"m","state":{},"questions":{"q":{"type":"score","instructions":"Q?","criteria":{}}}}`},
		{name: "noul criteria array", input: `{"model":"m","state":{},"questions":{"q":{"type":"noul","instructions":"Q?","criteria":[]}}}`},
		{name: "unknown type", input: `{"model":"m","state":{},"questions":{"q":{"type":"other","instructions":"Q?"}}}`},
		{name: "multiple values", input: `{"model":"m","state":{},"questions":{"q":{"type":"noul","instructions":"Q?"}}} {}`},
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
