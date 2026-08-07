package model

import (
	"reflect"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestPrimeSamplerAcceptsPromptTokensOnce(t *testing.T) {
	const sampler llama.Sampler = 7
	tokens := []llama.Token{10, llama.TokenNull, 20, 30}

	var accepted []llama.Token
	primeSamplerWith(sampler, tokens, true, func(gotSampler llama.Sampler, token llama.Token) {
		if gotSampler != sampler {
			t.Fatalf("sampler = %d, want %d", gotSampler, sampler)
		}
		accepted = append(accepted, token)
	})

	if want := []llama.Token{10, 20, 30}; !reflect.DeepEqual(accepted, want) {
		t.Fatalf("accepted tokens = %v, want %v", accepted, want)
	}
}

func TestPrimeSamplerEmptyPrompt(t *testing.T) {
	primeSamplerWith(1, nil, true, func(llama.Sampler, llama.Token) {
		t.Fatal("accepted a token from an empty prompt")
	})
}

func TestPrimeSamplerSkipsUnusedHistory(t *testing.T) {
	primeSamplerWith(1, []llama.Token{10, 20, 30}, false, func(llama.Sampler, llama.Token) {
		t.Fatal("accepted a token without a history-sensitive sampler")
	})
}

func TestSamplerPromptRequired(t *testing.T) {
	tests := []struct {
		name   string
		params Params
		want   bool
	}{
		{name: "disabled", params: Params{RepeatPenalty: 1.0}},
		{name: "repeat penalty", params: Params{RepeatPenalty: 1.1}, want: true},
		{name: "frequency penalty", params: Params{RepeatPenalty: 1.0, FrequencyPenalty: 0.1}, want: true},
		{name: "presence penalty", params: Params{RepeatPenalty: 1.0, PresencePenalty: 0.1}, want: true},
		{name: "dry", params: Params{RepeatPenalty: 1.0, DryMultiplier: 0.8}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := samplerPromptRequired(tt.params); got != tt.want {
				t.Errorf("samplerPromptRequired: got %t, want %t", got, tt.want)
			}
		})
	}
}
