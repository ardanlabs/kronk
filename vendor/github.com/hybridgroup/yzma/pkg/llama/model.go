package llama

import (
	"errors"
	"os"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/utils"
	"github.com/jupiterrider/ffi"
)

var (
	ffiTypeSize = ffi.TypeUint64

	// ffiTypeModelParams represents the C struct llama_model_params
	ffiTypeModelParams = ffi.NewType(&ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32,
		&ffi.TypeSint32, &ffi.TypeSint32,
		&ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer,
		&ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8,
		&ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8)

	// ffiTypeModelQuantizeParams represents the C struct llama_model_quantize_params
	ffiTypeModelQuantizeParams = ffi.NewType(&ffi.TypeSint32, &ffi.TypeSint32,
		&ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8,
		&ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer)
)

var (
	// LLAMA_API struct llama_model_params          llama_model_default_params(void);
	modelDefaultParamsFunc ffi.Fun

	// LLAMA_API struct llama_model * llama_model_load_from_file(
	//                          const char * path_model,
	//           				struct llama_model_params   params);
	modelLoadFromFileFunc ffi.Fun

	// Load the model from multiple splits (support custom naming scheme)
	// The paths must be in the correct order
	// LLAMA_API struct llama_model * llama_model_load_from_splits(
	//                          const char ** paths,
	//                          size_t    n_paths,
	//                          struct llama_model_params    params);
	modelLoadFromSplitsFunc ffi.Fun

	// LLAMA_API struct llama_model_params          llama_model_default_params(void);
	modelFreeFunc ffi.Fun

	// LLAMA_API struct llama_context * llama_init_from_model(
	//                  struct llama_model * model,
	//         			struct llama_context_params   params);
	initFromModelFunc ffi.Fun

	// LLAMA_API const char * llama_model_chat_template(const struct llama_model * model, const char * name);
	modelChatTemplateFunc ffi.Fun

	// LLAMA_API bool llama_model_has_encoder(const struct llama_model * model);
	modelHasEncoderFunc ffi.Fun

	// LLAMA_API bool llama_model_has_decoder(const struct llama_model * model);
	modelHasDecoderFunc ffi.Fun

	// LLAMA_API llama_token llama_model_decoder_start_token(const struct llama_model * model);
	modelDecoderStartTokenFunc ffi.Fun

	// LLAMA_API int32_t llama_model_n_ctx_train(const struct llama_model * model);
	modelNCtxTrainFunc ffi.Fun

	// LLAMA_API int32_t llama_model_n_embd     (const struct llama_model * model);
	modelNEmbdFunc ffi.Fun

	// LLAMA_API int32_t llama_model_n_embd_inp (const struct llama_model * model);
	modelNEmbdInpFunc ffi.Fun

	// LLAMA_API int32_t llama_model_n_embd_out(const struct llama_model * model);
	modelNEmbdOutFunc ffi.Fun

	// LLAMA_API int32_t llama_model_n_layer    (const struct llama_model * model);
	modelNLayerFunc ffi.Fun

	// LLAMA_API int32_t llama_model_n_head     (const struct llama_model * model);
	modelNHeadFunc ffi.Fun

	// LLAMA_API int32_t llama_model_n_head_kv  (const struct llama_model * model);
	modelNHeadKVFunc ffi.Fun

	// LLAMA_API int32_t llama_model_n_swa      (const struct llama_model * model);
	modelNSWAFunc ffi.Fun

	// LLAMA_API uint32_t llama_model_n_cls_out(const struct llama_model * model);
	modelNClsOutFunc ffi.Fun

	// LLAMA_API const char * llama_model_cls_label(const struct llama_model * model, uint32_t i);
	modelClsLabelFunc ffi.Fun

	// LLAMA_API int32_t llama_model_desc(const struct llama_model * model, char * buf, size_t buf_size);
	modelDescFunc ffi.Fun

	// LLAMA_API uint64_t llama_model_size(const struct llama_model * model);
	modelSizeFunc ffi.Fun

	// LLAMA_API bool llama_model_is_recurrent(const struct llama_model * model);
	modelIsRecurrentFunc ffi.Fun

	// LLAMA_API bool llama_model_is_hybrid(const struct llama_model * model);
	modelIsHybridFunc ffi.Fun

	// LLAMA_API bool llama_model_is_diffusion(const struct llama_model * model);
	modelIsDiffusionFunc ffi.Fun

	// LLAMA_API float llama_model_rope_freq_scale_train(const struct llama_model * model);
	modelRopeFreqScaleTrainFunc ffi.Fun

	// LLAMA_API enum llama_rope_type llama_model_rope_type(const struct llama_model * model);
	modelRopeTypeFunc ffi.Fun

	// Get metadata value as a string by key name
	// LLAMA_API int32_t llama_model_meta_val_str(const struct llama_model * model, const char * key, char * buf, size_t buf_size);
	modelMetaValStrFunc ffi.Fun

	// Get the number of metadata key/value pairs
	// LLAMA_API int32_t llama_model_meta_count(const struct llama_model * model);
	modelMetaCountFunc ffi.Fun

	// Get metadata key name by index
	// LLAMA_API int32_t llama_model_meta_key_by_index(const struct llama_model * model, int32_t i, char * buf, size_t buf_size);
	modelMetaKeyByIndexFunc ffi.Fun

	// Get metadata value as a string by index
	// LLAMA_API int32_t llama_model_meta_val_str_by_index(const struct llama_model * model, int32_t i, char * buf, size_t buf_size);
	modelMetaValStrByIndexFunc ffi.Fun

	// Get sampling metadata key name. Returns nullptr if the key is invalid
	// LLAMA_API const char * llama_model_meta_key_str(enum llama_model_meta_key key);
	modelMetaKeyStrFunc ffi.Fun

	// LLAMA_API struct llama_model_quantize_params llama_model_quantize_default_params(void);
	modelQuantizeDefaultParamsFunc ffi.Fun

	//     LLAMA_API uint32_t llama_model_quantize(
	//        const char * fname_inp,
	//        const char * fname_out,
	//        const llama_model_quantize_params * params);
	modelQuantizeFunc ffi.Fun

	// LLAMA_API enum llama_params_fit_status llama_params_fit(
	//                            const char   * path_model,
	//             struct llama_model_params   * mparams,
	//             struct llama_context_params * cparams,
	//                                   float * tensor_split,
	// struct llama_model_tensor_buft_override * tensor_buft_overrides,
	//                                  size_t * margins,
	//                                uint32_t   n_ctx_min,
	//                     enum ggml_log_level   log_level);
	modelParamsFitFunc ffi.Fun

	// LLAMA_API void llama_model_save_to_file(
	//         const struct llama_model * model,
	//                     const char * path_model);
	modelSaveToFileFunc ffi.Fun

	// LLAMA_API uint64_t llama_model_n_params(const struct llama_model * model);
	modelNParamsFunc ffi.Fun

	// LLAMA_API int32_t llama_split_path(char * split_path, size_t maxlen, const char * path_prefix, int32_t split_no, int32_t split_count);
	splitPathFunc ffi.Fun

	// LLAMA_API int32_t llama_split_prefix(char * split_prefix, size_t maxlen, const char * split_path, int32_t split_no, int32_t split_count);
	splitPrefixFunc ffi.Fun
)

func loadModelFuncs(lib ffi.Lib) error {
	var err error

	if modelDefaultParamsFunc, err = lib.Prep("llama_model_default_params", &ffiTypeModelParams); err != nil {
		return loadError("llama_model_default_params", err)
	}

	if modelLoadFromFileFunc, err = lib.Prep("llama_model_load_from_file", &ffi.TypePointer, &ffi.TypePointer, &ffiTypeModelParams); err != nil {
		return loadError("llama_model_load_from_file", err)
	}

	if modelLoadFromSplitsFunc, err = lib.Prep("llama_model_load_from_splits", &ffi.TypePointer, &ffi.TypePointer, &ffiTypeSize, &ffiTypeModelParams); err != nil {
		return loadError("llama_model_load_from_splits", err)
	}

	if modelFreeFunc, err = lib.Prep("llama_model_free", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("llama_model_free", err)
	}

	if initFromModelFunc, err = lib.Prep("llama_init_from_model", &ffi.TypePointer, &ffi.TypePointer, &ffiTypeContextParams); err != nil {
		return loadError("llama_init_from_model", err)
	}

	if modelChatTemplateFunc, err = lib.Prep("llama_model_chat_template", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_model_chat_template", err)
	}

	if modelHasEncoderFunc, err = lib.Prep("llama_model_has_encoder", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("llama_model_has_encoder", err)
	}

	if modelHasDecoderFunc, err = lib.Prep("llama_model_has_decoder", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("llama_model_has_decoder", err)
	}

	if modelDecoderStartTokenFunc, err = lib.Prep("llama_model_decoder_start_token", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_model_decoder_start_token", err)
	}

	if modelNCtxTrainFunc, err = lib.Prep("llama_model_n_ctx_train", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_model_n_ctx_train", err)
	}

	if modelNEmbdFunc, err = lib.Prep("llama_model_n_embd", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_model_n_embd", err)
	}

	if modelNEmbdInpFunc, err = lib.Prep("llama_model_n_embd_inp", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_model_n_embd_inp", err)
	}

	if modelNEmbdOutFunc, err = lib.Prep("llama_model_n_embd_out", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_model_n_embd_out", err)
	}

	if modelNLayerFunc, err = lib.Prep("llama_model_n_layer", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_model_n_layer", err)
	}

	if modelNHeadFunc, err = lib.Prep("llama_model_n_head", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_model_n_head", err)
	}

	if modelNHeadKVFunc, err = lib.Prep("llama_model_n_head_kv", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_model_n_head_kv", err)
	}

	if modelNSWAFunc, err = lib.Prep("llama_model_n_swa", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_model_n_swa", err)
	}

	if modelNClsOutFunc, err = lib.Prep("llama_model_n_cls_out", &ffi.TypeUint32, &ffi.TypePointer); err != nil {
		return loadError("llama_model_n_cls_out", err)
	}

	if modelClsLabelFunc, err = lib.Prep("llama_model_cls_label", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint32); err != nil {
		return loadError("llama_model_cls_label", err)
	}

	if modelDescFunc, err = lib.Prep("llama_model_desc", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint64); err != nil {
		return loadError("llama_model_desc", err)
	}

	if modelSizeFunc, err = lib.Prep("llama_model_size", &ffi.TypeUint64, &ffi.TypePointer); err != nil {
		return loadError("llama_model_size", err)
	}

	if modelIsRecurrentFunc, err = lib.Prep("llama_model_is_recurrent", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("llama_model_is_recurrent", err)
	}

	if modelIsHybridFunc, err = lib.Prep("llama_model_is_hybrid", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("llama_model_is_hybrid", err)
	}

	if modelIsDiffusionFunc, err = lib.Prep("llama_model_is_diffusion", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("llama_model_is_diffusion", err)
	}

	if modelRopeFreqScaleTrainFunc, err = lib.Prep("llama_model_rope_freq_scale_train", &ffi.TypeFloat, &ffi.TypePointer); err != nil {
		return loadError("llama_model_rope_freq_scale_train", err)
	}

	if modelRopeTypeFunc, err = lib.Prep("llama_model_rope_type", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_model_rope_type", err)
	}

	if modelMetaValStrFunc, err = lib.Prep("llama_model_meta_val_str", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffiTypeSize); err != nil {
		return loadError("llama_model_meta_val_str", err)
	}

	if modelMetaCountFunc, err = lib.Prep("llama_model_meta_count", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_model_meta_count", err)
	}

	if modelMetaKeyByIndexFunc, err = lib.Prep("llama_model_meta_key_by_index", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypePointer, &ffiTypeSize); err != nil {
		return loadError("llama_model_meta_key_by_index", err)
	}

	if modelMetaValStrByIndexFunc, err = lib.Prep("llama_model_meta_val_str_by_index", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypePointer, &ffiTypeSize); err != nil {
		return loadError("llama_model_meta_val_str_by_index", err)
	}

	if modelMetaKeyStrFunc, err = lib.Prep("llama_model_meta_key_str", &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_model_meta_key_str", err)
	}

	if modelQuantizeDefaultParamsFunc, err = lib.Prep("llama_model_quantize_default_params", &ffiTypeModelQuantizeParams); err != nil {
		return loadError("llama_model_quantize_default_params", err)
	}

	if modelQuantizeFunc, err = lib.Prep("llama_model_quantize", &ffi.TypeUint32, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_model_quantize", err)
	}

	if modelParamsFitFunc, err = lib.Prep("llama_params_fit", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint32, &ffi.TypeSint32); err != nil {
		return loadError("llama_params_fit", err)
	}

	if modelSaveToFileFunc, err = lib.Prep("llama_model_save_to_file", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_model_save_to_file", err)
	}

	if modelNParamsFunc, err = lib.Prep("llama_model_n_params", &ffi.TypeUint64, &ffi.TypePointer); err != nil {
		return loadError("llama_model_n_params", err)
	}

	if splitPathFunc, err = lib.Prep("llama_split_path", &ffi.TypeSint32, &ffi.TypePointer, &ffiTypeSize, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32); err != nil {
		return loadError("llama_split_path", err)
	}

	if splitPrefixFunc, err = lib.Prep("llama_split_prefix", &ffi.TypeSint32, &ffi.TypePointer, &ffiTypeSize, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32); err != nil {
		return loadError("llama_split_prefix", err)
	}

	return nil
}

// ModelDefaultParams returns default parameters for loading a Model.
func ModelDefaultParams() ModelParams {
	var p ModelParams
	modelDefaultParamsFunc.Call(unsafe.Pointer(&p))
	return p
}

// ModelLoadFromFile loads a Model from a GGUF file.
func ModelLoadFromFile(pathModel string, params ModelParams) (Model, error) {
	var model Model
	if _, err := os.Stat(pathModel); os.IsNotExist(err) {
		// no such file
		return model, err
	}

	file := &[]byte(pathModel + "\x00")[0]
	modelLoadFromFileFunc.Call(unsafe.Pointer(&model), unsafe.Pointer(&file), unsafe.Pointer(&params))
	if model == 0 {
		return model, errors.New("failed to load model")
	}

	return model, nil
}

// ModelLoadFromSplits loads a Model from multiple split files.
// The paths slice must be in the correct order.
func ModelLoadFromSplits(paths []string, params ModelParams) (Model, error) {
	var model Model
	if len(paths) == 0 {
		return model, errors.New("no paths provided")
	}

	// Allocate C array of pointers to null-terminated strings
	cStrs := make([]*byte, len(paths))
	for i := range paths {
		cStrs[i] = &[]byte(paths[i] + "\x00")[0]
	}
	cPaths := unsafe.Pointer(&cStrs[0])
	nPaths := uint64(len(paths))

	modelLoadFromSplitsFunc.Call(unsafe.Pointer(&model), &cPaths, &nPaths, unsafe.Pointer(&params))
	if model == 0 {
		return model, errors.New("failed to load model from splits")
	}

	return model, nil
}

// ModelFree frees a previously opened model.
func ModelFree(model Model) error {
	if model == 0 {
		return errors.New("invalid model")
	}
	modelFreeFunc.Call(nil, unsafe.Pointer(&model))
	return nil
}

// InitFromModel initializes a previously loaded Model, and then returns a new Context.
func InitFromModel(model Model, params ContextParams) (Context, error) {
	var ctx Context
	if model == 0 {
		return ctx, errors.New("invalid model")
	}
	initFromModelFunc.Call(unsafe.Pointer(&ctx), unsafe.Pointer(&model), unsafe.Pointer(&params))

	if ctx == 0 {
		return ctx, errors.New("failed to initialize model")
	}
	return ctx, nil
}

// ModelChatTemplate returns a named chat template for the Model.
func ModelChatTemplate(model Model, name string) string {
	if model == 0 {
		return ""
	}
	var template *byte
	var n *byte
	if len(name) > 0 {
		n = &[]byte(name + "\x00")[0]
	}
	modelChatTemplateFunc.Call(unsafe.Pointer(&template), unsafe.Pointer(&model), unsafe.Pointer(&n))

	return utils.BytePtrToString(template)
}

// ModelHasEncoder returns if the Model has an encoder.
func ModelHasEncoder(model Model) bool {
	if model == 0 {
		return false
	}
	var result ffi.Arg
	modelHasEncoderFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))

	return result.Bool()
}

// ModelHasDecoder returns if the Model has an decoder.
func ModelHasDecoder(model Model) bool {
	if model == 0 {
		return false
	}
	var result ffi.Arg
	modelHasDecoderFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))

	return result.Bool()
}

// ModelDecoderStartToken returns the start Token for the Model's decoder.
func ModelDecoderStartToken(model Model) Token {
	if model == 0 {
		return TokenNull
	}
	var result ffi.Arg
	modelDecoderStartTokenFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))

	return Token(result)
}

// ModelNCtxTrain returns the number of context tokens used during training.
func ModelNCtxTrain(model Model) int32 {
	if model == 0 {
		return 0
	}
	var result ffi.Arg
	modelNCtxTrainFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))

	return int32(result)
}

// ModelNEmbd returns the embedding size of the Model.
func ModelNEmbd(model Model) int32 {
	if model == 0 {
		return 0
	}
	var result ffi.Arg
	modelNEmbdFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))

	return int32(result)
}

// ModelNEmbdInp returns the input embedding size of the Model.
func ModelNEmbdInp(model Model) int32 {
	if model == 0 {
		return 0
	}
	var result ffi.Arg
	modelNEmbdInpFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))

	return int32(result)
}

// ModelNEmbdOut returns the output embedding size of the Model.
func ModelNEmbdOut(model Model) int32 {
	if model == 0 {
		return 0
	}
	var result ffi.Arg
	modelNEmbdOutFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))
	return int32(result)
}

// ModelNLayer returns the number of layers in the Model.
func ModelNLayer(model Model) int32 {
	if model == 0 {
		return 0
	}
	var result ffi.Arg
	modelNLayerFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))

	return int32(result)
}

// ModelNHead returns the number of attention heads in the Model.
func ModelNHead(model Model) int32 {
	if model == 0 {
		return 0
	}
	var result ffi.Arg
	modelNHeadFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))

	return int32(result)
}

// ModelNHeadKV returns the number of key/value attention heads in the Model.
func ModelNHeadKV(model Model) int32 {
	if model == 0 {
		return 0
	}
	var result ffi.Arg
	modelNHeadKVFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))

	return int32(result)
}

// ModelNSWA returns the number of SWA layers in the Model.
func ModelNSWA(model Model) int32 {
	if model == 0 {
		return 0
	}
	var result ffi.Arg
	modelNSWAFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))

	return int32(result)
}

// ModelNClsOut returns the number of classifier outputs (only valid for classifier models).
func ModelNClsOut(model Model) uint32 {
	if model == 0 {
		return 0
	}
	var nClsOut ffi.Arg
	modelNClsOutFunc.Call(unsafe.Pointer(&nClsOut), unsafe.Pointer(&model))
	return uint32(nClsOut)
}

// ModelClsLabel returns the label of a classifier output by index.
func ModelClsLabel(model Model, index uint32) string {
	if model == 0 {
		return ""
	}
	var labelPtr *byte
	modelClsLabelFunc.Call(unsafe.Pointer(&labelPtr), unsafe.Pointer(&model), &index)

	if labelPtr == nil {
		return ""
	}

	return utils.BytePtrToString(labelPtr)
}

// ModelDesc retrieves a string describing the model type.
func ModelDesc(model Model) string {
	if model == 0 {
		return ""
	}
	buf := make([]byte, 128)
	b := unsafe.SliceData(buf)
	bLen := int32(len(buf))

	var result ffi.Arg
	modelDescFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model), unsafe.Pointer(&b), &bLen)

	if int32(result) < 0 {
		return ""
	}

	return string(buf[:int32(result)])
}

// ModelSize returns the total size of all tensors in the model in bytes.
func ModelSize(model Model) uint64 {
	if model == 0 {
		return 0
	}
	var size ffi.Arg
	modelSizeFunc.Call(unsafe.Pointer(&size), unsafe.Pointer(&model))
	return uint64(size)
}

// ModelIsRecurrent returns true if the model is recurrent.
func ModelIsRecurrent(model Model) bool {
	if model == 0 {
		return false
	}
	var result ffi.Arg
	modelIsRecurrentFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))
	return result.Bool()
}

// ModelIsHybrid returns true if the model is hybrid.
func ModelIsHybrid(model Model) bool {
	if model == 0 {
		return false
	}
	var result ffi.Arg
	modelIsHybridFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))
	return result.Bool()
}

// ModelIsDiffusion returns true if the model is diffusion-based.
func ModelIsDiffusion(model Model) bool {
	if model == 0 {
		return false
	}
	var result ffi.Arg
	modelIsDiffusionFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))
	return result.Bool()
}

// ModelRopeFreqScaleTrain retrieves the model's RoPE frequency scaling factor.
func ModelRopeFreqScaleTrain(model Model) float32 {
	if model == 0 {
		return 0.0
	}

	var freqScale ffi.Arg
	modelRopeFreqScaleTrainFunc.Call(unsafe.Pointer(&freqScale), unsafe.Pointer(&model))
	return float32(freqScale)
}

// ModelRopeType retrieves the RoPE type of the model.
func ModelRopeType(model Model) RopeScalingType {
	if model == 0 {
		return RopeScalingTypeNone
	}
	var ropeType ffi.Arg
	modelRopeTypeFunc.Call(unsafe.Pointer(&ropeType), unsafe.Pointer(&model))
	return RopeScalingType(int32(ropeType))
}

// Warmup is to warm-up a model.
// It processes a representative batch of tokens (32) to trigger GPU kernel JIT
// compilation for common tensor shapes, reducing latency on first real request.
func Warmup(lctx Context, model Model) error {
	if lctx == 0 || model == 0 {
		return errors.New("invalid context or model")
	}

	vocab := ModelGetVocab(model)

	SetWarmup(lctx, true)

	bos := VocabBOS(vocab)
	eos := VocabEOS(vocab)

	// Build a representative batch of 32 tokens for proper kernel warmup.
	// This triggers CUDA/Metal JIT compilation for common batch sizes.
	const warmupBatchSize = 32
	tokens := make([]Token, 0, warmupBatchSize)

	// Start with BOS token if available.
	if bos != TokenNull {
		tokens = append(tokens, bos)
	}

	// Fill remaining slots with valid tokens (cycling through a small vocab range).
	vocabSize := VocabNTokens(vocab)
	if vocabSize <= 0 {
		vocabSize = 32000 // Reasonable default
	}

	for len(tokens) < warmupBatchSize-1 {
		// Use modulo to stay within vocab bounds, avoiding special tokens.
		tokenID := Token((len(tokens) + 100) % int(vocabSize))
		tokens = append(tokens, tokenID)
	}

	// End with EOS token if available.
	if eos != TokenNull {
		tokens = append(tokens, eos)
	} else if len(tokens) < warmupBatchSize {
		tokens = append(tokens, 0)
	}

	if ModelHasEncoder(model) {
		batch := BatchGetOne(tokens)
		Encode(lctx, batch)

		start := ModelDecoderStartToken(model)
		if start == TokenNull {
			start = bos
		}
		tokens = []Token{start}
	}

	if ModelHasDecoder(model) {
		batch := BatchGetOne(tokens)
		Decode(lctx, batch)
	}

	mem, err := GetMemory(lctx)
	if err != nil {
		return err
	}
	if err := MemoryClear(mem, true); err != nil {
		return err
	}

	Synchronize(lctx)
	SetWarmup(lctx, false)

	return nil
}

// ModelMetaValStr gets metadata value as a string by key name.
// Returns the string and true on success, or "" and false on failure.
func ModelMetaValStr(model Model, key string) (string, bool) {
	if model == 0 {
		return "", false
	}
	buf := make([]byte, 32768)
	b := unsafe.SliceData(buf)
	bLen := int32(len(buf))

	keyPtr, _ := utils.BytePtrFromString(key)
	var result ffi.Arg
	modelMetaValStrFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&model),
		unsafe.Pointer(&keyPtr),
		unsafe.Pointer(&b),
		&bLen,
	)
	if int32(result) < 0 {
		return "", false
	}

	// copy to a new slice to avoid retaining the entire buffer
	value := make([]byte, int32(result))
	copy(value, buf[:int32(result)])

	return string(value), true
}

// ModelMetaCount gets the number of metadata key/value pairs.
func ModelMetaCount(model Model) int32 {
	if model == 0 {
		return 0
	}
	var result ffi.Arg
	modelMetaCountFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))
	return int32(result)
}

// ModelMetaKeyByIndex gets metadata key name by index.
// Returns the string and true on success, or "" and false on failure.
func ModelMetaKeyByIndex(model Model, i int32) (string, bool) {
	if model == 0 {
		return "", false
	}
	buf := make([]byte, 128)
	b := unsafe.SliceData(buf)
	bLen := int32(len(buf))

	var result ffi.Arg
	modelMetaKeyByIndexFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&model),
		&i,
		unsafe.Pointer(&b),
		&bLen)
	if int32(result) < 0 {
		return "", false
	}

	// copy to a new slice to avoid retaining the entire buffer
	value := make([]byte, int32(result))
	copy(value, buf[:int32(result)])

	return string(value), true
}

// ModelMetaValStrByIndex gets metadata value as a string by index.
// Returns the string and true on success, or "" and false on failure.
func ModelMetaValStrByIndex(model Model, i int32) (string, bool) {
	if model == 0 {
		return "", false
	}
	buf := make([]byte, 32768)
	b := unsafe.SliceData(buf)
	bLen := int32(len(buf))

	var result ffi.Arg
	modelMetaValStrByIndexFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&model),
		&i,
		unsafe.Pointer(&b),
		&bLen)
	if int32(result) < 0 {
		return "", false
	}

	// copy to a new slice to avoid retaining the entire buffer
	value := make([]byte, int32(result))
	copy(value, buf[:int32(result)])

	return string(value), true
}

// ModelMetaKeyStr returns the metadata key name for a given enum key.
// Returns an empty string if the key is invalid.
func ModelMetaKeyStr(key ModelMetaKey) string {
	var ptr *byte
	modelMetaKeyStrFunc.Call(unsafe.Pointer(&ptr), &key)
	if ptr == nil {
		return ""
	}
	return utils.BytePtrToString(ptr)
}

// SetTensorBufOverrides sets tensor buffer overrides for Mixture of Experts (MoE) execution.
// The slice must be sentinel-terminated: the last element must have Pattern == nil.
// The caller must keep the slice alive (e.g., via runtime.KeepAlive) until the
// model load call using these params completes.
func (p *ModelParams) SetTensorBufOverrides(overrides []TensorBuftOverride) error {
	if len(overrides) == 0 {
		p.TensorBuftOverrides = uintptr(0)
		return nil
	}

	if overrides[len(overrides)-1].Pattern != nil {
		return errors.New("SetTensorBufOverrides: slice must be sentinel-terminated (last element Pattern must be nil)")
	}

	p.TensorBuftOverrides = uintptr(unsafe.Pointer(&overrides[0]))

	return nil
}

var progressCallbackCode unsafe.Pointer
var progressCallbackCif *ffi.Cif
var sizeOfClosure = unsafe.Sizeof(ffi.Closure{})

// SetProgressCallback sets a progress callback for model loading.
func (p *ModelParams) SetProgressCallback(cb ProgressCallback) {
	if cb == nil {
		p.ProgressCallback = uintptr(0)
		return
	}

	closure := ffi.ClosureAlloc(sizeOfClosure, &progressCallbackCode)

	fn := ffi.NewCallback(func(cif *ffi.Cif, ret unsafe.Pointer, args *unsafe.Pointer, userData unsafe.Pointer) uintptr {
		if args == nil || ret == nil {
			return 1 // error
		}

		arg := unsafe.Slice(args, cif.NArgs)
		progress := *(*float32)(arg[0])
		userDataPtr := *(*uintptr)(arg[1])
		result := cb(progress, userDataPtr)
		*(*uint8)(ret) = result
		return 0
	})

	progressCallbackCif = new(ffi.Cif)
	if status := ffi.PrepCif(progressCallbackCif, ffi.DefaultAbi, 2, &ffi.TypeUint8, &ffi.TypeFloat, &ffi.TypePointer); status != ffi.OK {
		panic(status)
	}

	if closure != nil {
		if status := ffi.PrepClosureLoc(closure, progressCallbackCif, fn, nil, progressCallbackCode); status != ffi.OK {
			panic(status)
		}
	}

	p.ProgressCallback = uintptr(progressCallbackCode)
}

// SetDevices sets the devices to be used for model execution.
// The slice must be NULL-terminated: the last element must be 0.
// The caller must keep the slice alive (e.g., via runtime.KeepAlive) until
// the model load call using these params completes.
func (p *ModelParams) SetDevices(devices []GGMLBackendDevice) error {
	if len(devices) == 0 {
		p.Devices = uintptr(0)
		return nil
	}

	if devices[len(devices)-1] != 0 {
		return errors.New("SetDevices: slice must be NULL-terminated (last element must be 0)")
	}

	p.Devices = uintptr(unsafe.Pointer(&devices[0]))

	return nil
}

// ModelQuantizeDefaultParams returns default parameters for model quantization.
func ModelQuantizeDefaultParams() ModelQuantizeParams {
	var p ModelQuantizeParams
	modelQuantizeDefaultParamsFunc.Call(unsafe.Pointer(&p))
	return p
}

// ModelQuantize quantizes a model from an input file to an output file using the specified parameters.
func ModelQuantize(fnameInp, fnameOut string, params *ModelQuantizeParams) uint32 {
	fileInp, err := utils.BytePtrFromString(fnameInp)
	if err != nil {
		return 0
	}

	fileOut, err := utils.BytePtrFromString(fnameOut)
	if err != nil {
		return 0
	}

	var result ffi.Arg
	modelQuantizeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&fileInp), unsafe.Pointer(&fileOut), unsafe.Pointer(&params))
	return uint32(result)
}

type ModelParamsFitStatus int32

const (
	ModelParamsFitStatusSuccess ModelParamsFitStatus = 0 // found allocations that are projected to fit
	ModelParamsFitStatusFailure ModelParamsFitStatus = 1 // could not find allocations that are projected to fit
	ModelParamsFitStatusError   ModelParamsFitStatus = 2 // a hard error occurred
)

// ModelParamsFit attempts to fit model and context parameters to available device memory.
// It assumes system memory is unlimited and only modifies parameters that have the same
// value as in the default model params, with the exception of context size which is
// modified if and only if equal to 0.
//
// Parameters:
//   - pathModel: path to the model file
//   - mparams: pointer to model parameters (will be modified to fit)
//   - cparams: pointer to context parameters (will be modified to fit)
//   - tensorSplit: writable buffer for tensor split, needs at least MaxDevices() elements
//   - tensorBuftOverrides: writable buffer for overrides, needs at least MaxTensorBuftOverrides() elements
//   - margins: margins of memory to leave per device in bytes
//   - nCtxMin: minimum context size to set when trying to reduce memory use
//   - logLevel: minimum log level to print during fitting, lower levels go to debug log
//
// Note: This function is NOT thread safe because it modifies the global llama logger state.
func ModelParamsFit(pathModel string, mparams *ModelParams, cparams *ContextParams, tensorSplit []float32, tensorBuftOverrides []TensorBuftOverride, margins []uint64, nCtxMin uint32, logLevel LogLevel) ModelParamsFitStatus {
	pathPtr, err := utils.BytePtrFromString(pathModel)
	if err != nil {
		return ModelParamsFitStatusError
	}

	var tensorSplitPtr *float32
	if len(tensorSplit) > 0 {
		tensorSplitPtr = &tensorSplit[0]
	}

	var tensorBuftOverridesPtr *TensorBuftOverride
	if len(tensorBuftOverrides) > 0 {
		tensorBuftOverridesPtr = &tensorBuftOverrides[0]
	}

	var marginsPtr *uint64
	if len(margins) > 0 {
		marginsPtr = &margins[0]
	}

	var result ffi.Arg
	modelParamsFitFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&pathPtr),
		unsafe.Pointer(&mparams),
		unsafe.Pointer(&cparams),
		unsafe.Pointer(&tensorSplitPtr),
		unsafe.Pointer(&tensorBuftOverridesPtr),
		unsafe.Pointer(&marginsPtr),
		&nCtxMin,
		&logLevel,
	)

	return ModelParamsFitStatus(int32(result))
}

// ModelSaveToFile saves the model to a file.
func ModelSaveToFile(model Model, pathModel string) {
	if model == 0 {
		return
	}
	path, err := utils.BytePtrFromString(pathModel)
	if err != nil {
		return
	}
	modelSaveToFileFunc.Call(nil, unsafe.Pointer(&model), unsafe.Pointer(&path))
}

// ModelNParams returns the total number of parameters in the model.
func ModelNParams(model Model) uint64 {
	if model == 0 {
		return 0
	}
	var result ffi.Arg
	modelNParamsFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model))
	return uint64(result)
}

// SplitPath builds a split GGUF final path for the given chunk.
// For example: SplitPath("/models/ggml-model-q4_0", 2, 4) returns "/models/ggml-model-q4_0-00002-of-00004.gguf"
// Returns the path string and the length, or empty string on failure.
func SplitPath(pathPrefix string, splitNo, splitCount int32) (string, int32) {
	buf := make([]byte, 1024)
	b := unsafe.SliceData(buf)
	bLen := uint64(len(buf))

	prefix, err := utils.BytePtrFromString(pathPrefix)
	if err != nil {
		return "", -1
	}

	var result ffi.Arg
	splitPathFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&b),
		&bLen,
		unsafe.Pointer(&prefix),
		&splitNo,
		&splitCount,
	)

	length := int32(result)
	if length < 0 || int(length) >= len(buf) {
		return "", length
	}

	value := make([]byte, length)
	copy(value, buf[:length])

	return string(value), length
}

// SplitPrefix extracts the path prefix from the split_path if the split_no and split_count match.
// For example: SplitPrefix("/models/ggml-model-q4_0-00002-of-00004.gguf", 2, 4) returns "/models/ggml-model-q4_0"
// Returns the prefix string and the length, or empty string on failure.
func SplitPrefix(splitPath string, splitNo, splitCount int32) (string, int32) {
	buf := make([]byte, 1024)
	b := unsafe.SliceData(buf)
	bLen := uint64(len(buf))

	path, err := utils.BytePtrFromString(splitPath)
	if err != nil {
		return "", -1
	}

	var result ffi.Arg
	splitPrefixFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&b),
		&bLen,
		unsafe.Pointer(&path),
		&splitNo,
		&splitCount,
	)

	length := int32(result)
	if length < 0 || int(length) >= len(buf) {
		return "", length
	}

	value := make([]byte, length)
	copy(value, buf[:length])

	return string(value), length
}
