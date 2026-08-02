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
	primeSamplerWith(sampler, tokens, func(gotSampler llama.Sampler, token llama.Token) {
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
	primeSamplerWith(1, nil, func(llama.Sampler, llama.Token) {
		t.Fatal("accepted a token from an empty prompt")
	})
}
