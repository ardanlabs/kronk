package caching

import (
	"context"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// fakeDeps implements Deps with configurable function fields.
// All fields default to no-op implementations.
type fakeDeps struct {
	createPromptFn       func(ctx context.Context, d D) (string, [][]byte, error)
	tokenizeStringFn     func(prompt string) []llama.Token
	decodeTokensFn       func(ctx context.Context, tokens []llama.Token, seqID llama.SeqId, startPos int) error
	clearSequenceFn      func(seqID llama.SeqId)
	extractKVStateFn     func(seqID llama.SeqId) ([]byte, int, error)
	restoreKVStateFn     func(data []byte, dstSeqID llama.SeqId) (int, error)
	mediaMarkerTokensFn  func(ctx context.Context) int
	logFn func(ctx context.Context, msg string, args ...any)
}

func (f *fakeDeps) CreatePrompt(ctx context.Context, d D) (string, [][]byte, error) {
	if f.createPromptFn != nil {
		return f.createPromptFn(ctx, d)
	}
	return "", nil, nil
}

func (f *fakeDeps) TokenizeString(prompt string) []llama.Token {
	if f.tokenizeStringFn != nil {
		return f.tokenizeStringFn(prompt)
	}
	return nil
}

func (f *fakeDeps) DecodeTokensIntoCache(ctx context.Context, tokens []llama.Token, seqID llama.SeqId, startPos int) error {
	if f.decodeTokensFn != nil {
		return f.decodeTokensFn(ctx, tokens, seqID, startPos)
	}
	return nil
}

func (f *fakeDeps) ClearSequence(seqID llama.SeqId) {
	if f.clearSequenceFn != nil {
		f.clearSequenceFn(seqID)
	}
}

func (f *fakeDeps) ExtractKVState(seqID llama.SeqId) ([]byte, int, error) {
	if f.extractKVStateFn != nil {
		return f.extractKVStateFn(seqID)
	}
	return nil, 0, nil
}

func (f *fakeDeps) RestoreKVState(data []byte, dstSeqID llama.SeqId) (int, error) {
	if f.restoreKVStateFn != nil {
		return f.restoreKVStateFn(data, dstSeqID)
	}
	return 0, nil
}

func (f *fakeDeps) MediaMarkerTokens(ctx context.Context) int {
	if f.mediaMarkerTokensFn != nil {
		return f.mediaMarkerTokensFn(ctx)
	}
	return 0
}

func (f *fakeDeps) Log(ctx context.Context, msg string, args ...any) {
	if f.logFn != nil {
		f.logFn(ctx, msg, args...)
	}
}
