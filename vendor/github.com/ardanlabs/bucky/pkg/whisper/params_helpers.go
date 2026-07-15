package whisper

import (
	"runtime"
	"unsafe"

	"github.com/ardanlabs/bucky/pkg/utils"
)

// StringRefs collects the Go-managed buffers that back pointer fields in a
// WhisperFullParams (C strings and the prompt-token array). The caller must
// keep the StringRefs value alive (e.g. via runtime.KeepAlive) for as long
// as the params struct is in use, so the underlying buffers are not
// garbage-collected while whisper.cpp reads through the pointers.
type StringRefs struct {
	keep       []*byte
	keepTokens [][]Token
}

// SetLanguage sets params.Language from a Go string. Pass "" or "auto" to
// have whisper auto-detect.
func (s *StringRefs) SetLanguage(p *WhisperFullParams, lang string) error {
	if lang == "" {
		p.Language = 0
		return nil
	}
	b, err := utils.BytePtrFromString(lang)
	if err != nil {
		return err
	}
	s.keep = append(s.keep, b)
	p.Language = uintptr(unsafe.Pointer(b))
	return nil
}

// SetInitialPrompt sets params.InitialPrompt from a Go string.
func (s *StringRefs) SetInitialPrompt(p *WhisperFullParams, prompt string) error {
	if prompt == "" {
		p.InitialPrompt = 0
		return nil
	}
	b, err := utils.BytePtrFromString(prompt)
	if err != nil {
		return err
	}
	s.keep = append(s.keep, b)
	p.InitialPrompt = uintptr(unsafe.Pointer(b))
	return nil
}

// SetSuppressRegex sets params.SuppressRegex from a Go string.
func (s *StringRefs) SetSuppressRegex(p *WhisperFullParams, re string) error {
	if re == "" {
		p.SuppressRegex = 0
		return nil
	}
	b, err := utils.BytePtrFromString(re)
	if err != nil {
		return err
	}
	s.keep = append(s.keep, b)
	p.SuppressRegex = uintptr(unsafe.Pointer(b))
	return nil
}

// SetPromptTokens sets params.PromptTokens and params.PromptNTokens from a
// slice of token ids harvested from a prior decode (e.g. the tail tokens
// read via FullGetTokenIDFromState). This seeds the decoder with prior
// linguistic context so a windowed/streaming caller keeps continuity across
// window boundaries. The tokens are copied into a buffer owned by s; keep s
// alive (StringRefs.KeepAlive) for as long as params is in use so the
// buffer is not garbage-collected. Passing an empty slice clears both
// fields (NULL pointer, zero count), which whisper interprets as "no
// prompt".
func (s *StringRefs) SetPromptTokens(p *WhisperFullParams, tokens []Token) {
	if len(tokens) == 0 {
		p.PromptTokens = 0
		p.PromptNTokens = 0
		return
	}

	cp := make([]Token, len(tokens))
	copy(cp, tokens)
	s.keepTokens = append(s.keepTokens, cp)

	p.PromptTokens = uintptr(unsafe.Pointer(&cp[0]))
	p.PromptNTokens = int32(len(cp))
}

// KeepAlive marks all backing buffers reachable. Defer this call right after
// constructing your StringRefs so the strings and token buffers outlive the
// FFI call.
func (s *StringRefs) KeepAlive() {
	runtime.KeepAlive(s.keep)
	runtime.KeepAlive(s.keepTokens)
}
