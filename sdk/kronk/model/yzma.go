package model

import (
	"math"
	"runtime"
	"strings"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/loader"
	"github.com/jupiterrider/ffi"
)

var tokenizeNULFunc ffi.Fun

// InitYzmaWorkarounds initializes Kronk-specific Yzma compatibility code.
func InitYzmaWorkarounds(libPath string) error {
	lib, err := loader.LoadLibrary(libPath, "llama")
	if err != nil {
		return err
	}

	tokenizeNULFunc, err = lib.Prep("llama_tokenize", &ffi.TypeSint32,
		&ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypePointer,
		&ffi.TypeSint32, &ffi.TypeUint8, &ffi.TypeUint8)
	return err
}

func tokenize(vocab llama.Vocab, text string, addSpecial bool, parseSpecial bool) []llama.Token {
	if !strings.ContainsRune(text, '\x00') {
		return llama.Tokenize(vocab, text, addSpecial, parseSpecial)
	}
	if vocab == 0 || tokenizeNULFunc.Addr == 0 || len(text) > math.MaxInt32 {
		return nil
	}

	textBuf, textPtr, textLen := tokenizeText(text)

	var (
		result ffi.Arg
		toks   *llama.Token
		max    int32
	)
	tokenizeNULFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&vocab), unsafe.Pointer(&textPtr), &textLen,
		unsafe.Pointer(&toks), &max, &addSpecial, &parseSpecial)

	nTokens := int32(result)
	if nTokens == math.MinInt32 {
		runtime.KeepAlive(textBuf)
		return nil
	}
	if nTokens >= 0 {
		runtime.KeepAlive(textBuf)
		return []llama.Token{}
	}

	tokens := make([]llama.Token, -nTokens)
	toks = unsafe.SliceData(tokens)
	max = int32(len(tokens))
	tokenizeNULFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&vocab), unsafe.Pointer(&textPtr), &textLen,
		unsafe.Pointer(&toks), &max, &addSpecial, &parseSpecial)
	runtime.KeepAlive(textBuf)

	nTokens = int32(result)
	if nTokens < 0 || nTokens > int32(len(tokens)) {
		return nil
	}
	return tokens[:nTokens]
}

func tokenizeText(text string) ([]byte, *byte, int32) {
	buf := append([]byte(text), 0)
	return buf, unsafe.SliceData(buf), int32(len(text))
}
