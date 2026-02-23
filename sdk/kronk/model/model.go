// Package model provides the low-level api for working with models.
package model

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/nikolalohinski/gonja/v2/exec"
	"go.opentelemetry.io/otel/attribute"
)

// compiledTemplate holds a pre-compiled Jinja template for reuse across requests.
type compiledTemplate struct {
	tmpl *exec.Template
	err  error
}

// imcSession holds the state for a single IMC (Incremental Message Cache) session.
// Each slot gets its own session with an assigned cache sequence.
type imcSession struct {
	cachedMsgsHash    string        // Hash of all cached messages
	cachedTokens      []llama.Token // Full token sequence in KV cache (immutable; replaced, never mutated)
	totalTokensCached int           // Total tokens in cache
	cachedMsgCount    int           // Number of messages cached
	seqID             llama.SeqId   // Assigned cache sequence ID
	slotID            int           // Dedicated slot ID bound to this session
	lastUsed          time.Time     // Last access time (for eviction)
	pending           bool          // True while a build/rebuild is in-flight (deferred decode)
}

// spcSession holds the state for a single SPC (System Prompt Cache) session.
// The system prompt is decoded once into a temporary cache sequence, the KV
// state is extracted into an external byte buffer, and the sequence is freed.
// On each request, the KV state is restored into the slot's working sequence
// via StateSeqSetData, avoiding a permanently dedicated cache sequence.
type spcSession struct {
	sysPromptHash   string      // Hash of the system prompt
	sysPromptTokens int         // Number of tokens in system prompt cache
	sysPromptLen    int         // Length of system prompt string
	seqID           llama.SeqId // Sequence ID used for initial decode
	lastUsed        time.Time   // Last access time
	kvState         []byte      // Externalized KV cache state (post-decode tensors)
}

// draftModel holds resources for the draft model used in speculative decoding.
// The draft model is a smaller, faster model that generates candidate tokens
// for the target model to verify in a single forward pass.
type draftModel struct {
	model        llama.Model
	vocab        llama.Vocab
	lctx         llama.Context
	mem          llama.Memory
	sampler      llama.Sampler
	batch        llama.Batch
	nDraft       int
	cachedTokens []llama.Token // Prompt tokens currently in draft KV cache (for incremental prefill)

	// Pre-allocated buffers for speculative sampling to avoid per-round
	// allocations of vocab-sized slices (~600KB each for 152k vocab).
	draftProbs  [][]float32 // nDraft reusable buffers for draft probability distributions
	targetProbs []float32   // Reusable buffer for target probability distribution
	adjusted    []float32   // Reusable buffer for sampleAdjusted computation
}

// Cataloger provides support to retrieve catalog config and template
// information.
type Cataloger interface {
	RetrieveTemplate(modelID string) (Template, error)
	RetrieveConfig(modelID string) (Config, error)
}

// Model represents a model and provides a low-level API for working with it.
type Model struct {
	cfg           Config
	log           Logger
	model         llama.Model
	vocab         llama.Vocab
	ctxParams     llama.ContextParams
	lctx          llama.Context
	mem           llama.Memory
	batch         *batchEngine
	template      Template
	compiledTmpl  *compiledTemplate
	templateOnce  sync.Once
	projFile      string
	modelInfo     ModelInfo
	activeStreams atomic.Int32
	unloaded      atomic.Bool
	decodeMu      sync.Mutex
	cacheMu       sync.RWMutex
	cacheCond     *sync.Cond    // Broadcast when any IMC slot's pending flag is cleared
	imcSlots      []*imcSession // Per-slot branch state, len = NSeqMax
	spcSession    *spcSession   // SPC session (single dedicated cache sequence)
	spcCacheSeqID llama.SeqId   // Dedicated SPC cache sequence ID
	addBOSToken   bool          // Whether to add BOS token (from model metadata)
	pool          *contextPool  // Context pool for parallel embed/rerank
	draft         *draftModel   // Draft model for speculative decoding
}

func NewModel(ctx context.Context, cataloger Cataloger, cfg Config) (*Model, error) {
	l := cfg.Log
	if cfg.Log == nil {
		l = func(ctx context.Context, msg string, args ...any) {}
	}

	if cataloger == nil {
		return nil, fmt.Errorf("catalog required, use catalog.New()")
	}

	if len(cfg.ModelFiles) == 0 {
		return nil, fmt.Errorf("model required")
	}

	// -------------------------------------------------------------------------

	modelID := modelIDFromFiles(cfg.ModelFiles)

	catCfg, err := cataloger.RetrieveConfig(modelID)

	switch err {
	case nil:
		cfg = applyCatalogConfig(cfg, catCfg)

	default:
		l(ctx, "CATALOG-CONFIG", "status", "not found", "modelID", modelID, "err", err)
	}

	if err := validateConfig(ctx, cfg, l); err != nil {
		return nil, fmt.Errorf("validate-config: unable to validate config: %w", err)
	}

	mParams := llama.ModelDefaultParams()

	if cfg.Device != "" {
		dev := llama.GGMLBackendDeviceByName(cfg.Device)
		if dev == 0 {
			return nil, fmt.Errorf("ggml-backend-device-by-name: unknown device: %s", cfg.Device)
		}
		mParams.SetDevices([]llama.GGMLBackendDevice{dev})
	}

	// llama.cpp has a -1 default for loading all layers into the GPU
	// However, we want to make it convenient to write the configuration.
	// So, we default to invert these two values after loading them.
	switch {
	case cfg.NGpuLayers == nil:
		mParams.NGpuLayers = -1
	case *cfg.NGpuLayers == 0:
		mParams.NGpuLayers = -1
	case *cfg.NGpuLayers == -1:
		mParams.NGpuLayers = 0
	default:
		mParams.NGpuLayers = int32(*cfg.NGpuLayers)
	}

	// Set split mode for multi-GPU and tensor parallelism (expert-parallel for MoE).
	// Default to SplitModeRow (tensor parallelism) when not explicitly configured,
	// as it provides the best performance for MoE models and works well for dense models.
	switch cfg.SplitMode == SplitModeNone {
	case true:
		mParams.SplitMode = SplitModeRow.ToYZMAType()
	case false:
		mParams.SplitMode = cfg.SplitMode.ToYZMAType()
	}

	// -------------------------------------------------------------------------

	loadStart := time.Now()

	mdl, err := loadModelFromFiles(ctx, l, cfg.ModelFiles, mParams)
	if err != nil {
		return nil, fmt.Errorf("load-model-from-files: unable to load model: %w", err)
	}

	loadDuration := time.Since(loadStart)

	cfg = adjustConfig(cfg, mdl)
	modelInfo := toModelInfo(cfg, mdl)

	metrics.AddModelFileLoadTime(modelInfo.ID, loadDuration)

	// -------------------------------------------------------------------------

	modelInfo.VRAMTotal, modelInfo.SlotMemory = calculateVRAM(cfg, modelInfo)

	metrics.SetVRAM(modelInfo.ID, modelInfo.VRAMTotal, modelInfo.SlotMemory)

	template, err := retrieveTemplate(cataloger, cfg, mdl, modelInfo)
	if err != nil {
		return nil, fmt.Errorf("retrieve-template: failed to retrieve model template: %w", err)
	}

	modelInfo.Template = template

	// Check if model metadata specifies to add BOS token.
	// Default to true for backward compatibility with models that don't specify.
	addBOSToken := true
	if v, ok := modelInfo.Metadata["tokenizer.ggml.add_bos_token"]; ok && v == "false" {
		addBOSToken = false
	}

	// -------------------------------------------------------------------------

	ctxParams := modelCtxParams(cfg, modelInfo)

	// Check if the current parameters will fit in available VRAM.
	// Uses copies so the actual parameters are never modified.
	fitsInVRAM, fitContextWin := checkParamsFit(cfg.ModelFiles[0], mParams, ctxParams)

	l(ctx, "PARAMS-FIT", "fitsInVRAM", fitsInVRAM, "fitContextWin", fitContextWin)

	l(ctx, "MODEL-INFO", "values", modelInfo.String(), "addBOSToken", addBOSToken)

	l(ctx, "MODEL-CONFIG", "values", cfg.String())

	l(ctx, "LLAMA-CONTEXT-PARAMS", "values", fmt.Sprintf("\nEmbeddings[%d]\nFlashAttentionType[%d]\nNBatch[%d]\nNCtx[%d]\nNSeqMax[%d]\nNThreads[%d]\nNThreadsBatch[%d]\nNUBatch[%d]\nOffloadKQV[%d]\nOpOffload[%d]\nPoolingType[%d]\nRopeFreqBase[%g]\nRopeFreqScale[%g]\nRopeScalingType[%d]\nTypeK[%d]\nTypeV[%d]\nYarnAttnFactor[%g]\nYarnBetaFast[%g]\nYarnBetaSlow[%g]\nYarnExtFactor[%g]\nYarnOrigCtx[%d]\n",
		ctxParams.Embeddings, ctxParams.FlashAttentionType, ctxParams.NBatch, ctxParams.NCtx,
		ctxParams.NSeqMax, ctxParams.NThreads, ctxParams.NThreadsBatch, ctxParams.NUbatch,
		ctxParams.Offload_kqv, ctxParams.OpOffload, ctxParams.PoolingType,
		ctxParams.RopeFreqBase, ctxParams.RopeFreqScale, ctxParams.RopeScalingType,
		ctxParams.TypeK, ctxParams.TypeV, ctxParams.YarnAttnFactor, ctxParams.YarnBetaFast,
		ctxParams.YarnBetaSlow, ctxParams.YarnExtFactor, ctxParams.YarnOrigCtx))

	// -------------------------------------------------------------------------

	m := Model{
		cfg:         cfg,
		log:         l,
		model:       mdl,
		vocab:       llama.ModelGetVocab(mdl),
		ctxParams:   ctxParams,
		template:    template,
		projFile:    cfg.ProjFile,
		modelInfo:   modelInfo,
		addBOSToken: addBOSToken,
	}

	// Initialize either context pool (for embed/rerank) or batch engine (for generation).
	// Embed/rerank models use a pool of contexts for parallel processing.
	// Generation models use the batch engine with a primary context.
	nSlots := max(cfg.NSeqMax, 1)

	switch {
	case modelInfo.IsEmbedModel || modelInfo.IsRerankModel:
		pool, err := newContextPool(ctx, mdl, ctxParams, l, nSlots)
		if err != nil {
			llama.ModelFree(mdl)
			return nil, fmt.Errorf("new-context-pool: unable to create context pool: %w", err)
		}
		m.pool = pool

	default:
		// Generation models need a primary context for the batch engine.
		lctx, err := llama.InitFromModel(mdl, ctxParams)
		if err != nil {
			llama.ModelFree(mdl)
			return nil, fmt.Errorf("init-from-model: unable to init context: %w", err)
		}

		mem, err := llama.GetMemory(lctx)
		if err != nil {
			llama.Free(lctx)
			llama.ModelFree(mdl)
			return nil, fmt.Errorf("get-memory: unable to get memory: %w", err)
		}

		llama.MemoryClear(mem, true)

		m.lctx = lctx
		m.mem = mem

		// Initialize IMC per-slot branch tracking when enabled.
		if cfg.IncrementalCache {
			m.cacheCond = sync.NewCond(&m.cacheMu)
			m.imcSlots = make([]*imcSession, nSlots)
			for i := range nSlots {
				m.imcSlots[i] = &imcSession{
					seqID:  llama.SeqId(i),
					slotID: i,
				}
			}
		}

		// Initialize SPC. The last slot's sequence is borrowed temporarily
		// for the initial decode. The KV state is externalized to a byte
		// buffer immediately after, so the sequence is freed for normal use.
		if cfg.SystemPromptCache {
			m.spcCacheSeqID = llama.SeqId(nSlots - 1)
		}

		m.batch = newBatchEngine(&m, nSlots)
		m.batch.start(ctx)

		// Initialize draft model for speculative decoding if configured.
		if cfg.DraftModel != nil {
			draft, err := loadDraftModel(ctx, l, cfg, mdl, ctxParams)
			if err != nil {
				m.batch.stop(ctx)
				m.batch.freeBatch()
				llama.Free(lctx)
				llama.ModelFree(mdl)
				return nil, fmt.Errorf("load-draft-model: %w", err)
			}
			m.draft = draft
			l(ctx, "draft-model", "status", "loaded",
				"nDraft", draft.nDraft, "device", cfg.DraftModel.Device,
				"nCtx", llama.NCtx(draft.lctx))
		}
	}

	return &m, nil
}

// loadDraftModel loads the draft model for speculative decoding. It creates
// a separate model, context, and greedy sampler. The draft model uses the
// same context window as the target to support long prompts.
func loadDraftModel(ctx context.Context, log Logger, cfg Config, targetModel llama.Model, targetCtxParams llama.ContextParams) (*draftModel, error) {
	dCfg := cfg.DraftModel

	// Load draft model.
	mParams := llama.ModelDefaultParams()
	switch {
	case dCfg.NGpuLayers == nil:
		mParams.NGpuLayers = -1
	case *dCfg.NGpuLayers == 0:
		mParams.NGpuLayers = -1
	case *dCfg.NGpuLayers == -1:
		mParams.NGpuLayers = 0
	default:
		mParams.NGpuLayers = int32(*dCfg.NGpuLayers)
	}

	if dCfg.Device != "" {
		dev := llama.GGMLBackendDeviceByName(dCfg.Device)
		if dev == 0 {
			return nil, fmt.Errorf("ggml-backend-device-by-name: unknown device: %s", dCfg.Device)
		}
		mParams.SetDevices([]llama.GGMLBackendDevice{dev})
	}

	log(ctx, "draft-model", "status", "loading",
		"files", fmt.Sprintf("%v", dCfg.ModelFiles),
		"device", dCfg.Device,
		"nDraft", dCfg.NDraft,
		"gpu_layers", mParams.NGpuLayers)

	dModel, err := loadModelFromFiles(ctx, log, dCfg.ModelFiles, mParams)
	if err != nil {
		return nil, fmt.Errorf("unable to load draft model: %w", err)
	}

	// Validate vocabulary compatibility.
	dVocab := llama.ModelGetVocab(dModel)
	targetVocab := llama.ModelGetVocab(targetModel)
	targetVocabSize := llama.VocabNTokens(targetVocab)
	draftVocabSize := llama.VocabNTokens(dVocab)

	log(ctx, "draft-model", "status", "vocab-check",
		"target_vocab", targetVocabSize, "draft_vocab", draftVocabSize)

	if draftVocabSize != targetVocabSize {
		llama.ModelFree(dModel)
		return nil, fmt.Errorf("vocabulary mismatch: target has %d tokens, draft has %d tokens",
			targetVocabSize, draftVocabSize)
	}

	// Create draft context with same context window as target.
	dCtxParams := llama.ContextDefaultParams()
	dCtxParams.NCtx = targetCtxParams.NCtx
	dCtxParams.NBatch = targetCtxParams.NBatch
	dCtxParams.NUbatch = targetCtxParams.NUbatch
	dCtxParams.NSeqMax = 1
	dCtxParams.FlashAttentionType = targetCtxParams.FlashAttentionType
	dCtxParams.NThreads = targetCtxParams.NThreads
	dCtxParams.NThreadsBatch = targetCtxParams.NThreadsBatch

	dLctx, err := llama.InitFromModel(dModel, dCtxParams)
	if err != nil {
		llama.ModelFree(dModel)
		return nil, fmt.Errorf("unable to init draft context: %w", err)
	}

	dMem, err := llama.GetMemory(dLctx)
	if err != nil {
		llama.Free(dLctx)
		llama.ModelFree(dModel)
		return nil, fmt.Errorf("unable to get draft memory: %w", err)
	}

	llama.MemoryClear(dMem, true)

	// Create greedy sampler for draft model (temperature=0 for speed).
	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	llama.SamplerChainAdd(sampler, llama.SamplerInitGreedy())

	// Create reusable batch for drafting (1 token at a time).
	batch := llama.BatchInit(1, 0, 1)

	// Pre-allocate reusable buffers for speculative sampling.
	nVocab := int(llama.VocabNTokens(dVocab))
	draftProbs := make([][]float32, dCfg.NDraft)
	for i := range draftProbs {
		draftProbs[i] = make([]float32, nVocab)
	}

	return &draftModel{
		model:       dModel,
		vocab:       dVocab,
		lctx:        dLctx,
		mem:         dMem,
		sampler:     sampler,
		batch:       batch,
		nDraft:      dCfg.NDraft,
		draftProbs:  draftProbs,
		targetProbs: make([]float32, nVocab),
		adjusted:    make([]float32, nVocab),
	}, nil
}

// paramsFitMu serializes calls to checkParamsFit because the underlying
// llama.ModelParamsFit function modifies global logger state and is not
// thread safe.
var paramsFitMu sync.Mutex

func checkParamsFit(modelFile string, mParams llama.ModelParams, ctxParams llama.ContextParams) (bool, uint32) {
	paramsFitMu.Lock()
	defer paramsFitMu.Unlock()

	mTest := mParams
	cTest := ctxParams

	nDevices := int(llama.MaxDevices())
	tensorSplit := make([]float32, nDevices)
	tensorBuftOverrides := make([]llama.TensorBuftOverride, llama.MaxTensorBuftOverrides())
	margins := make([]uint64, nDevices)

	status := llama.ModelParamsFit(
		modelFile,
		&mTest,
		&cTest,
		tensorSplit,
		tensorBuftOverrides,
		margins,
		512,
		llama.LogLevelWarn,
	)

	return status == llama.ModelParamsFitStatusSuccess, cTest.NCtx
}

func loadModelFromFiles(ctx context.Context, log Logger, modelFiles []string, params llama.ModelParams) (llama.Model, error) {
	baseModelFile := path.Base(modelFiles[0])

	log(ctx, "loading model from file", "status", "started", "model", baseModelFile)
	defer log(ctx, "loading model from file", "status", "completed", "model", baseModelFile)

	_, span := otel.AddSpan(ctx, "model-file-load-time",
		attribute.String("model-file", baseModelFile),
	)
	defer span.End()

	var err error
	var mdl llama.Model

	switch len(modelFiles) {
	case 1:
		mdl, err = llama.ModelLoadFromFile(modelFiles[0], params)
		if err != nil {
			return 0, fmt.Errorf("model-load-from-file: unable to load model: %w", err)
		}

	default:
		mdl, err = llama.ModelLoadFromSplits(modelFiles, params)
		if err != nil {
			return 0, fmt.Errorf("model-load-from-splits: unable to load model from split: %w", err)
		}
	}

	return mdl, nil
}

func retrieveTemplate(cataloger Cataloger, cfg Config, mdl llama.Model, modelInfo ModelInfo) (Template, error) {
	if cfg.JinjaFile != "" {
		data, err := readJinjaTemplate(cfg.JinjaFile)
		if err != nil {
			return Template{}, fmt.Errorf("read-jinja-template: failed to read jinja template: %w", err)
		}

		if data == "" {
			return Template{}, fmt.Errorf("read-jinja-template: jinja template is empty")
		}

		return Template{
			FileName: cfg.JinjaFile,
			Script:   data,
		}, nil
	}

	if cataloger != nil {
		template, err := cataloger.RetrieveTemplate(modelInfo.ID)
		if err == nil {
			return template, nil
		}
	}

	data := llama.ModelChatTemplate(mdl, "")
	if data == "" {
		data, _ = llama.ModelMetaValStr(mdl, "tokenizer.chat_template")
	}

	return Template{
		FileName: "tokenizer.chat_template",
		Script:   data,
	}, nil
}

func (m *Model) Unload(ctx context.Context) error {
	if !m.unloaded.CompareAndSwap(false, true) {
		return nil // Already unloaded
	}

	if _, exists := ctx.Deadline(); !exists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	// Stop the batch engine if running.
	hasBatch := m.batch != nil
	if hasBatch {
		m.batch.stop(ctx)
	}

	m.log(ctx, "unload", "status", "waiting-for-streams", "active", m.activeStreams.Load())

	for m.activeStreams.Load() > 0 {
		select {
		case <-ctx.Done():
			return fmt.Errorf("unload: cannot unload %d active streams: %w", m.activeStreams.Load(), ctx.Err())

		case <-time.After(100 * time.Millisecond):
		}
	}

	m.log(ctx, "unload", "status", "streams-drained")

	// Free draft model resources if loaded.
	if m.draft != nil {
		llama.SamplerFree(m.draft.sampler)
		llama.BatchFree(m.draft.batch)
		llama.Free(m.draft.lctx)
		llama.ModelFree(m.draft.model)
		m.draft = nil
		m.log(ctx, "unload", "status", "draft-model-freed")
	}

	// Free batch buffer before context (batch references context internals).
	if hasBatch {
		m.batch.freeBatch()
	}

	// Close the context pool if running (embed/rerank models).
	if m.pool != nil {
		m.pool.close()
	}

	// Free primary context if it exists (generation models only).
	if m.lctx != 0 {
		llama.Synchronize(m.lctx)
		llama.Free(m.lctx)
	}

	llama.ModelFree(m.model)

	return nil
}

func (m *Model) Config() Config {
	return m.cfg
}

func (m *Model) ModelInfo() ModelInfo {
	return m.modelInfo
}

func (m *Model) resetContext() {
	llama.Synchronize(m.lctx)

	mem, err := llama.GetMemory(m.lctx)
	if err == nil {
		llama.MemoryClear(mem, true)
	}

	m.clearCaches()
}

func (m *Model) isUnncessaryCRLF(reasonFlag int, completionFlag int, content string) bool {
	// We just started reasoning or tool calling so remove leading CR.
	if reasonFlag == 1 && content == "\x0A" {
		return true
	}

	// We just started completion so remove leading CR.
	if completionFlag == 1 && (content == "\x0A\x0A" || content == "\x0A") {
		return true
	}

	return false
}

func (m *Model) sendDeltaResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, choiceIndex int, prompt string, content string, reasonFlag int, outputTokens int, logprob *ContentLogprob) error {
	if outputTokens%500 == 0 {
		m.log(ctx, "chat-completion", "status", "delta", "id", id, "tokens", outputTokens, "object", object, "reasoning", reasonFlag, "content", len(content))
	}

	select {
	case <-ctx.Done():
		select {
		case ch <- ChatResponseErr(id, object, m.modelInfo.ID, choiceIndex, prompt, ctx.Err(), Usage{}):
		default:
		}

		return ctx.Err()

	case ch <- chatResponseDelta(id, object, m.modelInfo.ID, choiceIndex, content, reasonFlag > 0, logprob):
	}

	return nil
}

func (m *Model) sendFinalResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, choiceIndex int, prompt string, finalContent *strings.Builder, finalReasoning *strings.Builder, respToolCalls []ResponseToolCall, logprobsData []ContentLogprob, streaming bool, usage Usage) {
	m.log(ctx, "chat-completion", "status", "final", "id", id, "tokens", usage.OutputTokens, "object", object, "tooling", len(respToolCalls) > 0, "reasoning", finalReasoning.Len(), "content", finalContent.Len())

	// For streaming responses, logprobs were already sent per-delta chunk.
	// Only include accumulated logprobs for non-streaming requests.
	finalLogprobs := logprobsData
	if streaming {
		finalLogprobs = nil
	}

	select {
	case <-ctx.Done():
		select {
		case ch <- ChatResponseErr(id, object, m.modelInfo.ID, choiceIndex, prompt, ctx.Err(), usage):
		default:
		}

	case ch <- chatResponseFinal(id, object, m.modelInfo.ID, choiceIndex, prompt,
		finalContent.String(),
		finalReasoning.String(),
		respToolCalls,
		finalLogprobs,
		usage):
	}

	contextTokens := usage.PromptTokens + usage.CompletionTokens
	contextWindow := m.cfg.ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	m.log(ctx, "chat-completion (send final response)", "prompt", usage.PromptTokens, "output", usage.OutputTokens,
		"context", contextTokens, "down", fmt.Sprintf("(%.0f%% of %.0fK) TPS: %.2f", percentage, of, usage.TokensPerSecond))
}

func (m *Model) sendErrorResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, choiceIndex int, prompt string, err error, usage Usage) {
	m.log(ctx, "chat-completion", "status", "ERROR", "msg", err, "id", id, "object", object)

	select {
	case <-ctx.Done():

	case ch <- ChatResponseErr(id, object, m.modelInfo.ID, choiceIndex, prompt,
		err,
		usage):
	}
}

func calculateVRAM(cfg Config, mi ModelInfo) (vramTotal int64, slotMemory int64) {
	arch := mi.Metadata["general.architecture"]
	if arch == "" {
		return int64(mi.Size), 0
	}

	blockCount, err := strconv.ParseInt(mi.Metadata[arch+".block_count"], 10, 64)
	if err != nil {
		return int64(mi.Size), 0
	}

	headCountKV, err := strconv.ParseInt(mi.Metadata[arch+".attention.head_count_kv"], 10, 64)
	if err != nil {
		return int64(mi.Size), 0
	}

	keyLength, err := strconv.ParseInt(mi.Metadata[arch+".attention.key_length"], 10, 64)
	if err != nil {
		return int64(mi.Size), 0
	}

	valueLength, err := strconv.ParseInt(mi.Metadata[arch+".attention.value_length"], 10, 64)
	if err != nil {
		return int64(mi.Size), 0
	}

	bytesPerElement := ggmlTypeToBytes(cfg.CacheTypeK, cfg.CacheTypeV)

	nSeqMax := int64(max(cfg.NSeqMax, 1))

	contextWindow := int64(cfg.ContextWindow)

	kvPerTokenPerLayer := headCountKV * (keyLength + valueLength) * bytesPerElement
	kvPerSlot := contextWindow * blockCount * kvPerTokenPerLayer
	slotMemory = nSeqMax * kvPerSlot
	vramTotal = int64(mi.Size) + slotMemory

	return vramTotal, slotMemory
}

// restoreSPCToSeq restores the externalized SPC KV state into the destination
// sequence via StateSeqSetData. This avoids needing a permanently dedicated
// cache sequence by restoring from a byte buffer in RAM.
func (m *Model) restoreSPCToSeq(dstSeqID llama.SeqId) error {
	m.cacheMu.RLock()
	session := m.spcSession
	m.cacheMu.RUnlock()

	if session == nil || len(session.kvState) == 0 {
		return fmt.Errorf("restore-spc: no cached KV state available")
	}

	m.decodeMu.Lock()
	nRead := llama.StateSeqSetData(m.lctx, session.kvState, dstSeqID)
	m.decodeMu.Unlock()

	if nRead == 0 {
		return fmt.Errorf("restore-spc: StateSeqSetData failed for seq %d", dstSeqID)
	}

	return nil
}

func ggmlTypeToBytes(typeK, typeV GGMLType) int64 {
	bytesK := ggmlBytes(typeK)
	bytesV := ggmlBytes(typeV)

	if bytesK > bytesV {
		return bytesK
	}
	return bytesV
}

func ggmlBytes(t GGMLType) int64 {
	switch t {
	case GGMLTypeF32:
		return 4
	case GGMLTypeF16, GGMLTypeBF16:
		return 2
	case GGMLTypeQ8_0:
		return 1
	case GGMLTypeQ4_0, GGMLTypeQ4_1, GGMLTypeQ5_0, GGMLTypeQ5_1:
		return 1
	default:
		return 2
	}
}
