package whisper

import (
	"unsafe"

	"github.com/ardanlabs/bucky/pkg/utils"
	"github.com/jupiterrider/ffi"
)

// ffiTypeTokenData describes whisper_token_data to libffi. Used as the
// return type of whisper_full_get_token_data which returns by value.
//
// Layout (darwin/arm64, sizeof = 56):
//
//	whisper_token id      // int32  (4)
//	whisper_token tid     // int32  (4)
//	float p, plog, pt, ptsum  // 4*4
//	int64_t t0, t1, t_dtw // pad to 8 align, 3*8
//	float vlen            // 4 (+4 trailing padding)
var ffiTypeTokenData = ffi.NewType(
	&ffi.TypeSint32, // id
	&ffi.TypeSint32, // tid
	&ffi.TypeFloat,  // p
	&ffi.TypeFloat,  // plog
	&ffi.TypeFloat,  // pt
	&ffi.TypeFloat,  // ptsum
	&ffi.TypeSint64, // t0
	&ffi.TypeSint64, // t1
	&ffi.TypeSint64, // t_dtw
	&ffi.TypeFloat,  // vlen
)

var (
	// WHISPER_API int whisper_full_n_tokens(struct whisper_context * ctx, int i_segment);
	fullNTokensFunc ffi.Fun

	// WHISPER_API const char * whisper_full_get_token_text(struct whisper_context * ctx, int i_segment, int i_token);
	fullGetTokenTextFunc ffi.Fun

	// WHISPER_API whisper_token whisper_full_get_token_id(struct whisper_context * ctx, int i_segment, int i_token);
	fullGetTokenIDFunc ffi.Fun

	// WHISPER_API float whisper_full_get_token_p(struct whisper_context * ctx, int i_segment, int i_token);
	fullGetTokenPFunc ffi.Fun

	// WHISPER_API whisper_token_data whisper_full_get_token_data(struct whisper_context * ctx, int i_segment, int i_token);
	fullGetTokenDataFunc ffi.Fun

	// WHISPER_API const char * whisper_token_to_str(struct whisper_context * ctx, whisper_token token);
	tokenToStrFunc ffi.Fun
)

func loadTokensFuncs(lib ffi.Lib) error {
	var err error

	if fullNTokensFunc, err = lib.Prep("whisper_full_n_tokens", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("whisper_full_n_tokens", err)
	}

	if fullGetTokenTextFunc, err = lib.Prep("whisper_full_get_token_text",
		&ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32,
	); err != nil {
		return loadError("whisper_full_get_token_text", err)
	}

	if fullGetTokenIDFunc, err = lib.Prep("whisper_full_get_token_id",
		&ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32,
	); err != nil {
		return loadError("whisper_full_get_token_id", err)
	}

	if fullGetTokenPFunc, err = lib.Prep("whisper_full_get_token_p",
		&ffi.TypeFloat, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32,
	); err != nil {
		return loadError("whisper_full_get_token_p", err)
	}

	if fullGetTokenDataFunc, err = lib.Prep("whisper_full_get_token_data",
		&ffiTypeTokenData, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32,
	); err != nil {
		return loadError("whisper_full_get_token_data", err)
	}

	if tokenToStrFunc, err = lib.Prep("whisper_token_to_str", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("whisper_token_to_str", err)
	}

	return nil
}

// FullNTokens returns the number of tokens generated for the segment.
func FullNTokens(ctx Context, iSegment int32) int32 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	fullNTokensFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&iSegment))
	return int32(result)
}

// FullGetTokenText returns the textual form of the iToken'th token in the
// iSegment'th segment.
func FullGetTokenText(ctx Context, iSegment, iToken int32) string {
	if ctx == 0 {
		return ""
	}
	var ptr *byte
	fullGetTokenTextFunc.Call(unsafe.Pointer(&ptr), unsafe.Pointer(&ctx), unsafe.Pointer(&iSegment), unsafe.Pointer(&iToken))
	if ptr == nil {
		return ""
	}
	return utils.BytePtrToString(ptr)
}

// FullGetTokenID returns the vocabulary id of the iToken'th token in the
// iSegment'th segment.
func FullGetTokenID(ctx Context, iSegment, iToken int32) Token {
	if ctx == 0 {
		return TokenNull
	}
	var result ffi.Arg
	fullGetTokenIDFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&iSegment), unsafe.Pointer(&iToken))
	return Token(int32(result))
}

// FullGetTokenP returns the probability of the iToken'th token in the
// iSegment'th segment.
func FullGetTokenP(ctx Context, iSegment, iToken int32) float32 {
	if ctx == 0 {
		return 0
	}
	var result float32
	fullGetTokenPFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&iSegment), unsafe.Pointer(&iToken))
	return result
}

// FullGetTokenData returns the full TokenData (id, probability, timestamps,
// dtw timestamp, voice length) for the iToken'th token in the iSegment'th
// segment.
func FullGetTokenData(ctx Context, iSegment, iToken int32) TokenData {
	var td TokenData
	if ctx == 0 {
		return td
	}
	fullGetTokenDataFunc.Call(unsafe.Pointer(&td), unsafe.Pointer(&ctx), unsafe.Pointer(&iSegment), unsafe.Pointer(&iToken))
	return td
}

// TokenToStr returns the string for the given token id using the model's
// vocabulary.
func TokenToStr(ctx Context, token Token) string {
	if ctx == 0 {
		return ""
	}
	var ptr *byte
	tokenToStrFunc.Call(unsafe.Pointer(&ptr), unsafe.Pointer(&ctx), unsafe.Pointer(&token))
	if ptr == nil {
		return ""
	}
	return utils.BytePtrToString(ptr)
}
