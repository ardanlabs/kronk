package model

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestValidateDecisionRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     DecisionRequest
		wantErr bool
	}{
		{
			name: "mixed questions",
			req: DecisionRequest{Questions: []DecisionQuestion{
				DecisionQuestionChoice("route", "Where?", DecisionQuestionOpt("billing", nil), DecisionQuestionOpt("support", "Technical help")),
				DecisionQuestionScore("urgency", "How urgent?", "low", "high"),
				DecisionQuestionNoul("escalate", "Escalate?", nil),
			}},
		},
		{
			name:    "no questions",
			req:     DecisionRequest{},
			wantErr: true,
		},
		{
			name: "duplicate ids",
			req: DecisionRequest{Questions: []DecisionQuestion{
				DecisionQuestionNoul("same", "First?", nil),
				DecisionQuestionNoul("same", "Second?", nil),
			}},
			wantErr: true,
		},
		{
			name: "duplicate options",
			req: DecisionRequest{Questions: []DecisionQuestion{
				DecisionQuestionChoice("route", "Where?", DecisionQuestionOpt("billing", nil), DecisionQuestionOpt("billing", nil)),
			}},
			wantErr: true,
		},
		{
			name: "eleven score levels",
			req: DecisionRequest{Questions: []DecisionQuestion{
				DecisionQuestionScore("score", "Rate it", 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10),
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDecisionRequest(tt.req)
			if tt.wantErr && !errors.Is(err, ErrDecisionRequest) {
				t.Fatalf("error: got %v, want ErrDecisionRequest", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("error: got %v, want nil", err)
			}
		})
	}
}

func TestDecisionAnswerMarshalJSONKeepsTypedZeros(t *testing.T) {
	tests := []struct {
		answer  DecisionAnswer
		include []string
		exclude []string
	}{
		{
			answer:  DecisionAnswer{Type: DecisionQuestionTypeChoice, Probabilities: map[string]float64{"only": 0}},
			include: []string{`"choice":""`, `"confidence":0`},
			exclude: []string{`"score"`, `"noul"`, `"legend"`},
		},
		{
			answer:  DecisionAnswer{Type: DecisionQuestionTypeScore, Legend: map[string]any{"0": "low"}, Probabilities: map[string]float64{"0": 1}},
			include: []string{`"score":0`, `"confidence":0`},
			exclude: []string{`"choice"`, `"noul"`},
		},
		{
			answer:  DecisionAnswer{Type: DecisionQuestionTypeNoul},
			include: []string{`"noul":0`},
			exclude: []string{`"choice"`, `"score"`, `"confidence"`, `"probabilities"`},
		},
	}

	for _, tt := range tests {
		data, err := json.Marshal(tt.answer)
		if err != nil {
			t.Fatal(err)
		}
		for _, fragment := range tt.include {
			if !strings.Contains(string(data), fragment) {
				t.Errorf("%s does not contain %s", data, fragment)
			}
		}
		for _, fragment := range tt.exclude {
			if strings.Contains(string(data), fragment) {
				t.Errorf("%s unexpectedly contains %s", data, fragment)
			}
		}
	}
}

func TestDetectDecisionProtocol(t *testing.T) {
	tests := []struct {
		name       string
		configured DecisionProtocol
		modelID    string
		metadata   map[string]string
		want       DecisionProtocol
	}{
		{
			name:     "Jev-Style metadata survives renamed file",
			modelID:  "renamed-model-Q8_0",
			metadata: map[string]string{"general.name": "Jev-Style-0.8B-Decision-v3"},
			want:     DecisionProtocolJevStyle,
		},
		{
			name:    "OpenJEV filename fallback",
			modelID: "OpenJev-Q4_K_M",
			metadata: map[string]string{
				"general.name": "Snapshot_Cfg",
			},
			want: DecisionProtocolOpenJEV,
		},
		{
			name:       "explicit override survives unknown identity",
			configured: DecisionProtocolOpenJEV,
			modelID:    "renamed-model",
			want:       DecisionProtocolOpenJEV,
		},
		{
			name:    "ordinary model",
			modelID: "Qwen3.5-0.8B-Q8_0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectDecisionProtocol(tt.configured, tt.modelID, tt.metadata)
			if got != tt.want {
				t.Fatalf("protocol: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestModelWithoutGenerationRuntimeRejectsChat(t *testing.T) {
	m := Model{modelInfo: ModelInfo{ID: "non-generation"}}
	_, err := m.ChatStreaming(context.Background(), D{})
	if err == nil {
		t.Fatal("ChatStreaming: got nil error, want unsupported operation")
	}
	for _, fragment := range []string{"non-generation", "doesn't support chat"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("ChatStreaming error %q does not contain %q", err, fragment)
		}
	}
}

func TestDecisionReportsUndetectedProtocol(t *testing.T) {
	m := Model{modelInfo: ModelInfo{ID: "renamed-model"}}

	_, err := m.Decision(context.Background(), DecisionRequest{})
	if err == nil {
		t.Fatal("Decision: got nil error, want undetected protocol error")
	}

	for _, fragment := range []string{"renamed-model", "model.WithDecisionProtocol(...)"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("Decision error %q does not contain %q", err, fragment)
		}
	}
}
