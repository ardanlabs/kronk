package model

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestApplySamplerFiltersMatchesNative(t *testing.T) {
	if testing.Short() {
		t.Skip("native llama.cpp comparison skipped in short mode")
	}
	loadNativeSamplerLibrary(t)

	tests := []struct {
		name        string
		temperature float32
		topP        float32
		minP        float32
		topK        int32
		suppress    []llama.Token
	}{
		{name: "explicit top-p zero", temperature: 0.9, topP: 0, topK: 0},
		{name: "disabled top-k", temperature: 0.7, topP: 0.7, topK: -1},
		{name: "disabled truncation", temperature: 1.2, topP: 1, topK: 0},
		{name: "top-k and top-p", temperature: 0.8, topP: 0.8, topK: 3},
		{name: "min-p", temperature: 1, topP: 1, minP: 0.2, topK: 0},
		{name: "suppressed maximum", temperature: 1, topP: 0.8, topK: 0, suppress: []llama.Token{1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logits := []float32{-2.3, 3.7, 0.2, 2.1, -0.8, 1.2}
			goLogits := slices.Clone(logits)
			goProbs := make([]float32, len(logits))
			var fs filterState

			applySamplerFilters(goLogits, goProbs, tt.suppress, tt.temperature, tt.topP, tt.minP, tt.topK, nil, &fs)
			nativeProbs := nativeSamplerProbabilities(t, logits, tt.suppress, tt.temperature, tt.topP, tt.minP, tt.topK)

			for i := range goProbs {
				if math.Abs(float64(goProbs[i]-nativeProbs[i])) > 1e-5 {
					t.Errorf("token %d probability: got %v, native %v", i, goProbs[i], nativeProbs[i])
				}
			}
		})
	}
}

func TestApplySamplerFiltersMatchesNativeAdaptiveTopP(t *testing.T) {
	if testing.Short() {
		t.Skip("native llama.cpp comparison skipped in short mode")
	}
	loadNativeSamplerLibrary(t)

	const size = 1025
	logits := make([]float32, size)
	for i := range logits {
		logits[i] = float32(i)/1000 + float32(i%7)/10000
	}
	goLogits := slices.Clone(logits)
	goProbs := make([]float32, len(logits))
	var fs filterState

	applySamplerFilters(goLogits, goProbs, nil, 0.9, 0.8, 0, 0, nil, &fs)
	nativeProbs := nativeSamplerProbabilities(t, logits, nil, 0.9, 0.8, 0, 0)

	for i := range goProbs {
		if math.Abs(float64(goProbs[i]-nativeProbs[i])) > 1e-5 {
			t.Errorf("token %d probability: got %v, native %v", i, goProbs[i], nativeProbs[i])
		}
	}
}

func loadNativeSamplerLibrary(t testing.TB) {
	t.Helper()

	libPath := libs.Path("")
	entries, err := os.ReadDir(libPath)
	if err != nil {
		t.Skipf("native llama.cpp libraries unavailable at %s", libPath)
	}
	found := false
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "llama") {
			found = true
			break
		}
	}
	if !found {
		t.Skipf("native llama.cpp libraries unavailable at %s", libPath)
	}
	if err := llama.Load(libPath); err != nil {
		t.Fatalf("load native llama.cpp libraries: %v", err)
	}
}

func nativeSamplerProbabilities(t testing.TB, logits []float32, suppress []llama.Token, temperature, topP, minP float32, topK int32) []float32 {
	t.Helper()

	nativeLogits := slices.Clone(logits)
	maskSuppressTokenLogits(nativeLogits, suppress)
	tokenData := make([]llama.TokenData, len(nativeLogits))
	for i, logit := range nativeLogits {
		tokenData[i] = llama.TokenData{Id: llama.Token(i), Logit: logit}
	}
	curP := llama.TokenDataArray{
		Data:     &tokenData[0],
		Size:     uint64(len(tokenData)),
		Selected: -1,
	}

	chain := nativeFilterChain(temperature, topP, minP, topK)
	t.Cleanup(func() {
		llama.SamplerFree(chain)
	})
	llama.SamplerApply(chain, &curP)

	survivors := tokenData[:int(curP.Size)]
	maxLogit := survivors[0].Logit
	for _, candidate := range survivors[1:] {
		maxLogit = max(maxLogit, candidate.Logit)
	}
	var sum float64
	for _, candidate := range survivors {
		sum += math.Exp(float64(candidate.Logit - maxLogit))
	}

	probs := make([]float32, len(logits))
	for _, candidate := range survivors {
		probs[candidate.Id] = float32(math.Exp(float64(candidate.Logit-maxLogit)) / sum)
	}
	return probs
}

func nativeFilterChain(temperature, topP, minP float32, topK int32) llama.Sampler {
	chain := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	llama.SamplerChainAdd(chain, llama.SamplerInitTopK(topK))
	llama.SamplerChainAdd(chain, llama.SamplerInitTopP(topP, 0))
	llama.SamplerChainAdd(chain, llama.SamplerInitMinP(minP, 0))
	llama.SamplerChainAdd(chain, llama.SamplerInitTempExt(temperature, 0, 1))
	return chain
}

func BenchmarkNativeSamplerFilters(b *testing.B) {
	loadNativeSamplerLibrary(b)

	const qwenVocab = 151936
	logits := make([]float32, qwenVocab)
	for i := range logits {
		rank := (i * 7919) % qwenVocab
		logits[i] = -float32(rank) / 50
	}

	tests := []struct {
		topP float32
		minP float32
	}{
		{topP: 0},
		{topP: 0.95},
		{topP: 1},
		{topP: 1, minP: 0.05},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("top_k=0/top_p=%.2f/min_p=%.2f", tt.topP, tt.minP)
		b.Run(name, func(b *testing.B) {
			chain := nativeFilterChain(0.9, tt.topP, tt.minP, 0)
			b.Cleanup(func() {
				llama.SamplerFree(chain)
			})
			tokenData := make([]llama.TokenData, len(logits))
			probs := make([]float32, len(logits))
			previous := make([]llama.Token, 0, len(logits))

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				for _, token := range previous {
					probs[token] = 0
				}
				for i, logit := range logits {
					tokenData[i] = llama.TokenData{Id: llama.Token(i), Logit: logit}
				}
				curP := llama.TokenDataArray{Data: &tokenData[0], Size: uint64(len(tokenData)), Selected: -1}
				llama.SamplerApply(chain, &curP)

				survivors := tokenData[:int(curP.Size)]
				maxLogit := survivors[0].Logit
				for _, candidate := range survivors[1:] {
					maxLogit = max(maxLogit, candidate.Logit)
				}
				var sum float64
				for _, candidate := range survivors {
					sum += math.Exp(float64(candidate.Logit - maxLogit))
				}
				previous = previous[:len(survivors)]
				for i, candidate := range survivors {
					previous[i] = candidate.Id
					probs[candidate.Id] = float32(math.Exp(float64(candidate.Logit-maxLogit)) / sum)
				}
			}
		})
	}
}
