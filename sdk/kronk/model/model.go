// Package model provides the low-level api for working with models.
package model

import (
	"context"
	"fmt"
	"path"
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
// Each unique cache_id gets its own session with an assigned cache sequence.
type imcSession struct {
	cachedMsgsHash    string      // Hash of all cached messages
	totalTokensCached int         // Total tokens in cache
	lastMsgIdxCached  int         // The index of the last message cached
	seqID             llama.SeqId // Assigned cache sequence ID
	lastUsed          time.Time   // Last access time (for eviction)
}

// spcSession holds the state for a single SPC (System Prompt Cache) session.
// Each unique cache_id gets its own session with an assigned cache sequence.
type spcSession struct {
	sysPromptHash   string      // Hash of the system prompt
	sysPromptTokens int         // Number of tokens in system prompt cache
	sysPromptLen    int         // Length of system prompt string
	seqID           llama.SeqId // Assigned cache sequence ID
	lastUsed        time.Time   // Last access time (for eviction)
}

// TemplateRetriever returns a configured template for a model.
type TemplateRetriever interface {
	Retrieve(modelID string) (Template, error)
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
	imcSessions   map[string]*imcSession // IMC sessions keyed by cache_id
	imcNextSeq    llama.SeqId            // Next available cache sequence
	imcMaxSeqs    int                    // Max IMC sessions from config
	spcSessions   map[string]*spcSession // SPC sessions keyed by cache_id
	spcNextSeq    llama.SeqId            // Next available cache sequence
	spcMaxSeqs    int                    // Max SPC sessions from config
	addBOSToken   bool                   // Whether to add BOS token (from model metadata)
}

func NewModel(ctx context.Context, tmplRetriever TemplateRetriever, cfg Config) (*Model, error) {
	l := cfg.Log
	if cfg.Log == nil {
		l = func(ctx context.Context, msg string, args ...any) {}
	}

	if tmplRetriever == nil {
		return nil, fmt.Errorf("templater required, use templater.New()")
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

	mdl, err := loadModelFromFiles(ctx, l, cfg.ModelFiles, mParams)
	if err != nil {
		return nil, fmt.Errorf("load-model-from-files: unable to load model: %w", err)
	}

	// -------------------------------------------------------------------------

	cfg = adjustConfig(cfg, mdl)
	modelInfo := toModelInfo(cfg, mdl)

	template, err := retrieveTemplate(tmplRetriever, cfg, mdl, modelInfo)
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

	l(ctx, "MODEL-INFO", "values", modelInfo.String(), "addBOSToken", addBOSToken)

	l(ctx, "MODEL-CONFIG", "values", cfg.String())

	l(ctx, "LLAMA-CONTEXT-PARAMS", "values", fmt.Sprintf("\nNCtx[%d]\nNBatch[%d]\nNUBatch[%d]\nNSeqMax[%d]\nTypeK[%d]\nTypeV[%d]\nNThreads[%d]\nNThreadsBatch[%d]\nEmbeddings[%d]\nPoolingType[%d]\nFlashAttentionType[%d]\nOffloadKQV[%d]\nOpOffload[%d]\nRopeScalingType[%d]\nRopeFreqBase[%g]\nRopeFreqScale[%g]\nYarnExtFactor[%g]\nYarnAttnFactor[%g]\nYarnBetaFast[%g]\nYarnBetaSlow[%g]\nYarnOrigCtx[%d]\n",
		ctxParams.NCtx, ctxParams.NBatch, ctxParams.NUbatch, ctxParams.NSeqMax,
		ctxParams.TypeK, ctxParams.TypeV, ctxParams.NThreads, ctxParams.NThreadsBatch,
		ctxParams.Embeddings, ctxParams.PoolingType, ctxParams.FlashAttentionType,
		ctxParams.Offload_kqv, ctxParams.OpOffload,
		ctxParams.RopeScalingType, ctxParams.RopeFreqBase, ctxParams.RopeFreqScale,
		ctxParams.YarnExtFactor, ctxParams.YarnAttnFactor, ctxParams.YarnBetaFast,
		ctxParams.YarnBetaSlow, ctxParams.YarnOrigCtx))

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

	// Clear KV cache to ensure clean state on first request.
	// Without this, uninitialized memory can cause SIGTRAP in llama.cpp decode.
	llama.MemoryClear(mem, true)

	// Initialize IMC session tracking when enabled.
	// Sessions start at seq 0 and increment up to MaxCacheSessions.
	var imcSessions map[string]*imcSession
	var imcMaxSeqs int
	if cfg.IncrementalCache {
		imcMaxSeqs = max(cfg.MaxCacheSessions, 1)
		imcSessions = make(map[string]*imcSession, imcMaxSeqs)
	}

	// Initialize SPC session tracking when enabled.
	// Sessions start at seq 0 and increment up to MaxCacheSessions.
	var spcSessions map[string]*spcSession
	var spcMaxSeqs int
	if cfg.SystemPromptCache {
		spcMaxSeqs = max(cfg.MaxCacheSessions, 1)
		spcSessions = make(map[string]*spcSession, spcMaxSeqs)
	}

	m := Model{
		cfg:         cfg,
		log:         l,
		model:       mdl,
		vocab:       llama.ModelGetVocab(mdl),
		ctxParams:   ctxParams,
		lctx:        lctx,
		mem:         mem,
		template:    template,
		projFile:    cfg.ProjFile,
		modelInfo:   modelInfo,
		imcSessions: imcSessions,
		imcNextSeq:  0,
		imcMaxSeqs:  imcMaxSeqs,
		spcSessions: spcSessions,
		spcNextSeq:  0,
		spcMaxSeqs:  spcMaxSeqs,
		addBOSToken: addBOSToken,
	}

	// Initialize batch engine for parallel inference.
	// Supports both text-only and multi-modal (mtmd) models.
	nSlots := max(cfg.NSeqMax, 1)
	m.batch = newBatchEngine(&m, nSlots)
	m.batch.start(ctx)

	return &m, nil
}

func loadModelFromFiles(ctx context.Context, log Logger, modelFiles []string, params llama.ModelParams) (llama.Model, error) {
	baseModelFile := path.Base(modelFiles[0])

	log(ctx, "loading model from file", "status", "started", "model", baseModelFile)
	defer log(ctx, "loading model from file", "status", "completed", "model", baseModelFile)

	_, span := otel.AddSpan(ctx, "proj-file-load-time",
		attribute.String("model-file", baseModelFile),
	)
	defer span.End()

	start := time.Now()
	defer func() {
		metrics.AddModelFileLoadTime(baseModelFile, time.Since(start))
	}()

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

func retrieveTemplate(tmlpRetriever TemplateRetriever, cfg Config, mdl llama.Model, modelInfo ModelInfo) (Template, error) {
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

	if tmlpRetriever != nil {
		template, err := tmlpRetriever.Retrieve(modelInfo.ID)
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

	// Free batch buffer before context (batch references context internals).
	if hasBatch {
		m.batch.freeBatch()
	}

	// Synchronize ensures all GPU operations complete before freeing.
	llama.Synchronize(m.lctx)
	llama.Free(m.lctx)
	llama.ModelFree(m.model)
	llama.BackendFree()

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

	default:
	}
}
