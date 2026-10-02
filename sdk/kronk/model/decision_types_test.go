package model

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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

func TestDecisionResponseMarshalJSON(t *testing.T) {
	resp := DecisionResponse{
		Model: "decision-model",
		Answers: map[string]DecisionAnswer{
			"approve": {Type: DecisionQuestionTypeNoul},
			"route":   {Type: DecisionQuestionTypeChoice, Probabilities: map[string]float64{}},
			"urgency": {Type: DecisionQuestionTypeScore, Legend: map[string]any{}, Probabilities: map[string]float64{}},
		},
		Usage: DecisionUsage{InputTokens: 42},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	answers := got["answers"].(map[string]any)
	if want := map[string]any{"type": "choice", "choice": "", "probabilities": map[string]any{}, "confidence": float64(0)}; !reflect.DeepEqual(answers["route"], want) {
		t.Errorf("route answer: got %#v, want %#v", answers["route"], want)
	}
	if want := map[string]any{"type": "score", "score": float64(0), "legend": map[string]any{}, "probabilities": map[string]any{}, "confidence": float64(0)}; !reflect.DeepEqual(answers["urgency"], want) {
		t.Errorf("urgency answer: got %#v, want %#v", answers["urgency"], want)
	}
	if want := map[string]any{"type": "noul", "noul": float64(0)}; !reflect.DeepEqual(answers["approve"], want) {
		t.Errorf("approve answer: got %#v, want %#v", answers["approve"], want)
	}
	if want := map[string]any{"input_tokens": float64(42), "output_tokens": float64(0)}; !reflect.DeepEqual(got["usage"], want) {
		t.Errorf("usage: got %#v, want %#v", got["usage"], want)
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
			name:    "OpenJEV decision metadata",
			modelID: "renamed-model",
			metadata: map[string]string{
				"general.architecture": "qwen35",
				"qwen35.decision.type": "openjev",
			},
			want: DecisionProtocolOpenJEV,
		},
		{
			name:    "Laya decision metadata",
			modelID: "renamed-model",
			metadata: map[string]string{
				"general.architecture":      "modern-bert",
				"modern-bert.decision.type": "laya",
			},
			want: DecisionProtocolLaya,
		},
		{
			name:    "Lev decision metadata",
			modelID: "renamed-model",
			metadata: map[string]string{
				"general.architecture": "qwen35",
				"qwen35.decision.type": "lev",
			},
			want: DecisionProtocolLev,
		},
		{
			name:    "Kev decision metadata",
			modelID: "renamed-model",
			metadata: map[string]string{
				"general.architecture": "qwen35",
				"qwen35.decision.type": "kev",
			},
			want: DecisionProtocolKev,
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
