package llama

import (
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/utils"
	"github.com/jupiterrider/ffi"
)

var (
	// LLAMA_API const struct llama_vocab * llama_model_get_vocab(const struct llama_model * model);
	modelGetVocabFunc ffi.Fun

	// LLAMA_API llama_token llama_vocab_bos(const struct llama_vocab * vocab); // beginning-of-sentence
	vocabBOSFunc ffi.Fun

	// LLAMA_API llama_token llama_vocab_eos(const struct llama_vocab * vocab); // end-of-sentence
	vocabEOSFunc ffi.Fun

	// LLAMA_API llama_token llama_vocab_eot(const struct llama_vocab * vocab); // end-of-turn
	vocabEOTFunc ffi.Fun

	// LLAMA_API llama_token llama_vocab_sep(const struct llama_vocab * vocab); // sentence separator
	vocabSEPFunc ffi.Fun

	// LLAMA_API llama_token llama_vocab_nl(const struct llama_vocab * vocab); // next-line
	vocabNLFunc ffi.Fun

	// LLAMA_API llama_token llama_vocab_pad(const struct llama_vocab * vocab); // padding
	vocabPADFunc ffi.Fun

	// LLAMA_API llama_token llama_vocab_mask(const struct llama_vocab * vocab); // mask
	vocabMASKFunc ffi.Fun

	// LLAMA_API bool llama_vocab_get_add_bos(const struct llama_vocab * vocab);
	vocabGetAddBOSFunc ffi.Fun

	// LLAMA_API bool llama_vocab_get_add_eos(const struct llama_vocab * vocab);
	vocabGetAddEOSFunc ffi.Fun

	// LLAMA_API bool llama_vocab_get_add_sep(const struct llama_vocab * vocab);
	vocabGetAddSEPFunc ffi.Fun

	// LLAMA_API llama_token llama_vocab_fim_pre(const struct llama_vocab * vocab);
	vocabFIMPreFunc ffi.Fun

	// LLAMA_API llama_token llama_vocab_fim_suf(const struct llama_vocab * vocab);
	vocabFIMSufFunc ffi.Fun

	// LLAMA_API llama_token llama_vocab_fim_mid(const struct llama_vocab * vocab);
	vocabFIMMidFunc ffi.Fun

	// LLAMA_API llama_token llama_vocab_fim_pad(const struct llama_vocab * vocab);
	vocabFIMPadFunc ffi.Fun

	// LLAMA_API llama_token llama_vocab_fim_rep(const struct llama_vocab * vocab);
	vocabFIMRepFunc ffi.Fun

	// LLAMA_API llama_token llama_vocab_fim_sep(const struct llama_vocab * vocab);
	vocabFIMSepFunc ffi.Fun

	// LLAMA_API bool llama_vocab_is_eog(const struct llama_vocab * vocab, llama_token token);
	vocabIsEOGFunc ffi.Fun

	// LLAMA_API bool llama_vocab_is_control(const struct llama_vocab * vocab, llama_token token);
	vocabIsControlFunc ffi.Fun

	// LLAMA_API int32_t llama_vocab_n_tokens(const struct llama_vocab * vocab);
	vocabNTokensFunc ffi.Fun

	// LLAMA_API int32_t llama_token_to_piece(
	//              const struct llama_vocab * vocab,
	//                           llama_token   token,
	//                                  char * buf,
	//                               int32_t   length,
	//                               int32_t   lstrip,
	//                               bool   special);
	tokenToPieceFunc ffi.Fun

	// LLAMA_API int32_t llama_tokenize(
	//     const struct llama_vocab * vocab,
	//                   const char * text,
	//                      int32_t   text_len,
	//                  llama_token * tokens,
	//                      int32_t   n_tokens_max,
	//                         bool   add_special,
	//                         bool   parse_special);
	tokenizeFunc ffi.Fun

	// LLAMA_API int32_t llama_detokenize(
	//     const struct llama_vocab * vocab,
	//            const llama_token * tokens,
	//                      int32_t   n_tokens,
	//                         char * text,
	//                      int32_t   text_len_max,
	//                         bool   remove_special,
	//                         bool   unparse_special);
	detokenizeFunc ffi.Fun

	// LLAMA_API enum llama_token_attr llama_vocab_get_attr(const struct llama_vocab * vocab, llama_token token);
	vocabGetAttrFunc ffi.Fun

	// LLAMA_API float llama_vocab_get_score(const struct llama_vocab * vocab, llama_token token);
	vocabGetScoreFunc ffi.Fun

	// LLAMA_API const char * llama_vocab_get_text(const struct llama_vocab * vocab, llama_token token);
	vocabGetTextFunc ffi.Fun

	// LLAMA_API enum llama_vocab_type llama_vocab_type(const struct llama_vocab * vocab);
	vocabTypeFunc ffi.Fun
)

func loadVocabFuncs(lib ffi.Lib) error {
	var err error

	if modelGetVocabFunc, err = lib.Prep("llama_model_get_vocab", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_model_get_vocab", err)
	}

	if vocabBOSFunc, err = lib.Prep("llama_vocab_bos", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_bos", err)
	}

	if vocabEOSFunc, err = lib.Prep("llama_vocab_eos", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_eos", err)
	}

	if vocabEOTFunc, err = lib.Prep("llama_vocab_eot", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_eot", err)
	}

	if vocabSEPFunc, err = lib.Prep("llama_vocab_sep", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_sep", err)
	}

	if vocabNLFunc, err = lib.Prep("llama_vocab_nl", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_nl", err)
	}

	if vocabPADFunc, err = lib.Prep("llama_vocab_pad", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_pad", err)
	}

	if vocabMASKFunc, err = lib.Prep("llama_vocab_mask", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_mask", err)
	}

	if vocabGetAddBOSFunc, err = lib.Prep("llama_vocab_get_add_bos", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_get_add_bos", err)
	}

	if vocabGetAddEOSFunc, err = lib.Prep("llama_vocab_get_add_eos", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_get_add_eos", err)
	}

	if vocabGetAddSEPFunc, err = lib.Prep("llama_vocab_get_add_sep", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_get_add_sep", err)
	}

	if vocabFIMPreFunc, err = lib.Prep("llama_vocab_fim_pre", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_fim_pre", err)
	}

	if vocabFIMSufFunc, err = lib.Prep("llama_vocab_fim_suf", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_fim_suf", err)
	}

	if vocabFIMMidFunc, err = lib.Prep("llama_vocab_fim_mid", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_fim_mid", err)
	}

	if vocabFIMPadFunc, err = lib.Prep("llama_vocab_fim_pad", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_fim_pad", err)
	}

	if vocabFIMRepFunc, err = lib.Prep("llama_vocab_fim_rep", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_fim_rep", err)
	}

	if vocabFIMSepFunc, err = lib.Prep("llama_vocab_fim_sep", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_fim_sep", err)
	}

	if vocabIsEOGFunc, err = lib.Prep("llama_vocab_is_eog", &ffi.TypeUint8, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_vocab_is_eog", err)
	}

	if vocabIsControlFunc, err = lib.Prep("llama_vocab_is_control", &ffi.TypeUint8, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_vocab_is_control", err)
	}

	if vocabNTokensFunc, err = lib.Prep("llama_vocab_n_tokens", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_n_tokens", err)
	}

	if tokenToPieceFunc, err = lib.Prep("llama_token_to_piece", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeSint32,
		&ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypeUint8); err != nil {
		return loadError("llama_token_to_piece", err)
	}

	if tokenizeFunc, err = lib.Prep("llama_tokenize", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32,
		&ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeUint8, &ffi.TypeUint8); err != nil {
		return loadError("llama_tokenize", err)
	}

	if detokenizeFunc, err = lib.Prep("llama_detokenize", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer,
		&ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeUint8, &ffi.TypeUint8); err != nil {
		return loadError("llama_detokenize", err)
	}

	if vocabGetAttrFunc, err = lib.Prep("llama_vocab_get_attr", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_vocab_get_attr", err)
	}

	if vocabGetScoreFunc, err = lib.Prep("llama_vocab_get_score", &ffi.TypeFloat, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_vocab_get_score", err)
	}

	if vocabGetTextFunc, err = lib.Prep("llama_vocab_get_text", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_vocab_get_text", err)
	}

	if vocabTypeFunc, err = lib.Prep("llama_vocab_type", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_vocab_type", err)
	}

	return nil
}

// ModelGetVocab retrieves the vocabulary associated with a given model.
func ModelGetVocab(model Model) Vocab {
	var vocab Vocab
	if model == 0 {
		return vocab
	}
	modelGetVocabFunc.Call(unsafe.Pointer(&vocab), unsafe.Pointer(&model))

	return vocab
}

// VocabBOS retrieves the beginning-of-sentence token from the vocabulary.
func VocabBOS(vocab Vocab) Token {
	var token ffi.Arg
	if vocab == 0 {
		return TokenNull
	}
	vocabBOSFunc.Call(unsafe.Pointer(&token), unsafe.Pointer(&vocab))

	return Token(token)
}

// VocabEOS retrieves the end-of-sentence token from the vocabulary.
func VocabEOS(vocab Vocab) Token {
	var token ffi.Arg
	if vocab == 0 {
		return TokenNull
	}
	vocabEOSFunc.Call(unsafe.Pointer(&token), unsafe.Pointer(&vocab))

	return Token(token)
}

// VocabEOT retrieves the end-of-turn token from the vocabulary.
func VocabEOT(vocab Vocab) Token {
	var token ffi.Arg
	if vocab == 0 {
		return TokenNull
	}
	vocabEOTFunc.Call(unsafe.Pointer(&token), unsafe.Pointer(&vocab))
	return Token(token)
}

// VocabSEP retrieves the sentence separator token from the vocabulary.
func VocabSEP(vocab Vocab) Token {
	var token ffi.Arg
	if vocab == 0 {
		return TokenNull
	}
	vocabSEPFunc.Call(unsafe.Pointer(&token), unsafe.Pointer(&vocab))
	return Token(token)
}

// VocabNL retrieves the next-line token from the vocabulary.
func VocabNL(vocab Vocab) Token {
	var token ffi.Arg
	if vocab == 0 {
		return TokenNull
	}
	vocabNLFunc.Call(unsafe.Pointer(&token), unsafe.Pointer(&vocab))
	return Token(token)
}

// VocabPAD retrieves the padding token from the vocabulary.
func VocabPAD(vocab Vocab) Token {
	var token ffi.Arg
	if vocab == 0 {
		return TokenNull
	}
	vocabPADFunc.Call(unsafe.Pointer(&token), unsafe.Pointer(&vocab))
	return Token(token)
}

// VocabMASK retrieves the mask token from the vocabulary.
func VocabMASK(vocab Vocab) Token {
	var token ffi.Arg
	if vocab == 0 {
		return TokenNull
	}
	vocabMASKFunc.Call(unsafe.Pointer(&token), unsafe.Pointer(&vocab))
	return Token(token)
}

// VocabGetAddBOS retrieves whether to add the beginning-of-sentence token.
func VocabGetAddBOS(vocab Vocab) bool {
	var result ffi.Arg
	if vocab == 0 {
		return false
	}
	vocabGetAddBOSFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&vocab))
	return result.Bool()
}

// VocabGetAddEOS retrieves whether to add the end-of-sentence token.
func VocabGetAddEOS(vocab Vocab) bool {
	var result ffi.Arg
	if vocab == 0 {
		return false
	}
	vocabGetAddEOSFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&vocab))
	return result.Bool()
}

// VocabGetAddSEP retrieves whether to add the sentence separator token.
func VocabGetAddSEP(vocab Vocab) bool {
	var result ffi.Arg
	if vocab == 0 {
		return false
	}
	vocabGetAddSEPFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&vocab))
	return result.Bool()
}

// VocabFIMPre retrieves the FIM pre-token from the vocabulary.
func VocabFIMPre(vocab Vocab) Token {
	var token ffi.Arg
	if vocab == 0 {
		return TokenNull
	}
	vocabFIMPreFunc.Call(unsafe.Pointer(&token), unsafe.Pointer(&vocab))
	return Token(token)
}

// VocabFIMSuf retrieves the FIM suffix token from the vocabulary.
func VocabFIMSuf(vocab Vocab) Token {
	var token ffi.Arg
	if vocab == 0 {
		return TokenNull
	}
	vocabFIMSufFunc.Call(unsafe.Pointer(&token), unsafe.Pointer(&vocab))
	return Token(token)
}

// VocabFIMMid retrieves the FIM middle token from the vocabulary.
func VocabFIMMid(vocab Vocab) Token {
	var token ffi.Arg
	if vocab == 0 {
		return TokenNull
	}
	vocabFIMMidFunc.Call(unsafe.Pointer(&token), unsafe.Pointer(&vocab))
	return Token(token)
}

// VocabFIMPad retrieves the FIM padding token from the vocabulary.
func VocabFIMPad(vocab Vocab) Token {
	var token ffi.Arg
	if vocab == 0 {
		return TokenNull
	}
	vocabFIMPadFunc.Call(unsafe.Pointer(&token), unsafe.Pointer(&vocab))
	return Token(token)
}

// VocabFIMRep retrieves the FIM repeat token from the vocabulary.
func VocabFIMRep(vocab Vocab) Token {
	var token ffi.Arg
	if vocab == 0 {
		return TokenNull
	}
	vocabFIMRepFunc.Call(unsafe.Pointer(&token), unsafe.Pointer(&vocab))
	return Token(token)
}

// VocabFIMSep retrieves the FIM separator token from the vocabulary.
func VocabFIMSep(vocab Vocab) Token {
	var token ffi.Arg
	if vocab == 0 {
		return TokenNull
	}
	vocabFIMSepFunc.Call(unsafe.Pointer(&token), unsafe.Pointer(&vocab))
	return Token(token)
}

// VocabIsEOG checks if a token is an end-of-generation token in the vocabulary.
func VocabIsEOG(vocab Vocab, token Token) bool {
	var result ffi.Arg
	if vocab == 0 {
		return false
	}
	vocabIsEOGFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&vocab), unsafe.Pointer(&token))

	return result.Bool()
}

// VocabIsControl checks if a token is a control token in the vocabulary.
func VocabIsControl(vocab Vocab, token Token) bool {
	var result ffi.Arg
	if vocab == 0 {
		return false
	}
	vocabIsControlFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&vocab), unsafe.Pointer(&token))

	return result.Bool()
}

// VocabNTokens retrieves the number of tokens in the vocabulary.
func VocabNTokens(vocab Vocab) int32 {
	var result ffi.Arg
	if vocab == 0 {
		return 0
	}
	vocabNTokensFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&vocab))

	return int32(result)
}

// TokenToPiece converts a token to its corresponding piece (string) representation.
// The result is written into the provided buffer `buf`, which has a length of `len(buf)`.
// The `lstrip` parameter specifies the number of leading characters to strip from the piece.
// The `special` parameter indicates whether to treat special tokens differently.
func TokenToPiece(vocab Vocab, token Token, buf []byte, lstrip int32, special bool) int32 {
	if vocab == 0 {
		return 0
	}
	b := unsafe.SliceData(buf)
	bLen := int32(len(buf))

	var result ffi.Arg
	tokenToPieceFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&vocab), &token, unsafe.Pointer(&b),
		&bLen, &lstrip, &special)

	return int32(result)
}

// Tokenize converts an input text into a sequence of tokens using the specified vocabulary.
// The `addSpecial` parameter indicates whether to add special tokens, and the `parseSpecial` parameter
// specifies whether to parse special tokens in the input text.
// The function returns a slice of tokens.
func Tokenize(vocab Vocab, text string, addSpecial bool, parseSpecial bool) []Token {
	if vocab == 0 {
		return nil
	}
	txt, _ := utils.BytePtrFromString(text)
	txtLen := int32(len(text))

	// get the needed size
	var (
		result ffi.Arg
		toks   *Token
		max    int32
	)
	tokenizeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&vocab), unsafe.Pointer(&txt), &txtLen,
		unsafe.Pointer(&toks), &max, &addSpecial, &parseSpecial)
	size := -int32(result)

	// now get the actual tokens
	tokens := make([]Token, size)
	toks = unsafe.SliceData(tokens)
	nTokensMax := int32(len(tokens))

	tokenizeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&vocab), unsafe.Pointer(&txt), &txtLen,
		unsafe.Pointer(&toks), &nTokensMax, &addSpecial, &parseSpecial)

	return tokens
}

// Detokenize converts a sequence of tokens into text using the specified vocabulary.
// The `removeSpecial` parameter indicates whether to remove special tokens, and the `unparseSpecial` parameter
// specifies whether to render special tokens in the output.
// The function returns the detokenized text as a string.
func Detokenize(vocab Vocab, tokens []Token, removeSpecial bool, unparseSpecial bool) string {
	if vocab == 0 || len(tokens) == 0 {
		return ""
	}

	// Get the needed size by calling with a NULL text buffer
	var (
		result     ffi.Arg
		toks       *Token
		nTokens    int32
		textPtr    *byte
		textLenMax int32
	)
	toks = unsafe.SliceData(tokens)
	nTokens = int32(len(tokens))

	detokenizeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&vocab), unsafe.Pointer(&toks), &nTokens,
		unsafe.Pointer(&textPtr), &textLenMax, &removeSpecial, &unparseSpecial)
	size := -int32(result)

	// If size is still negative or zero, return empty string
	if size <= 0 {
		return ""
	}

	// Now get the actual text
	buf := make([]byte, size)
	textPtr = unsafe.SliceData(buf)
	textLenMax = int32(len(buf))

	detokenizeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&vocab), unsafe.Pointer(&toks), &nTokens,
		unsafe.Pointer(&textPtr), &textLenMax, &removeSpecial, &unparseSpecial)

	// Convert the byte slice to a string, excluding any null terminator
	// The function returns the number of bytes actually written
	actualSize := int32(result)
	if actualSize <= 0 {
		return ""
	}

	// Handle possible null terminator
	if buf[actualSize-1] == 0 {
		return string(buf[:actualSize-1])
	}

	return string(buf[:actualSize])
}

// VocabGetAttr retrieves the attribute of a given token in the vocabulary.
func VocabGetAttr(vocab Vocab, token Token) TokenAttr {
	if vocab == 0 {
		return TokenAttrUnknown
	}
	var attr ffi.Arg
	vocabGetAttrFunc.Call(unsafe.Pointer(&attr), unsafe.Pointer(&vocab), unsafe.Pointer(&token))
	return TokenAttr(int32(attr))
}

// VocabGetScore retrieves the score of a given token in the vocabulary.
func VocabGetScore(vocab Vocab, token Token) float32 {
	if vocab == 0 {
		return 0.0
	}
	var score ffi.Arg
	vocabGetScoreFunc.Call(unsafe.Pointer(&score), unsafe.Pointer(&vocab), unsafe.Pointer(&token))
	return float32(score)
}

// VocabGetText retrieves the text representation of a given token in the vocabulary.
func VocabGetText(vocab Vocab, token Token) string {
	if vocab == 0 {
		return ""
	}
	var textPtr *byte
	vocabGetTextFunc.Call(unsafe.Pointer(&textPtr), unsafe.Pointer(&vocab), unsafe.Pointer(&token))

	if textPtr == nil {
		return ""
	}

	return utils.BytePtrToString(textPtr)
}

// GetVocabType retrieves the type of the vocabulary.
func GetVocabType(vocab Vocab) VocabType {
	if vocab == 0 {
		return VocabTypeNone
	}
	var vocabType ffi.Arg
	vocabTypeFunc.Call(unsafe.Pointer(&vocabType), unsafe.Pointer(&vocab))

	return VocabType(int32(vocabType))
}
