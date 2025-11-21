package kronk

import (
	"context"
	"fmt"
	"math"
	"sync"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

type model struct {
	model     llama.Model
	vocab     llama.Vocab
	ctxParams llama.ContextParams
	template  string
	projFile  string
	muHEC     sync.Mutex
}

func newModel(modelFile string, cfg Config, options ...func(m *model) error) (*model, error) {
	mdl, err := llama.ModelLoadFromFile(modelFile, llama.ModelDefaultParams())
	if err != nil {
		return nil, fmt.Errorf("ModelLoadFromFile: %w", err)
	}

	vocab := llama.ModelGetVocab(mdl)

	// -------------------------------------------------------------------------

	template := llama.ModelChatTemplate(mdl, "")
	if template == "" {
		template, _ = llama.ModelMetaValStr(mdl, "tokenizer.chat_template")
	}

	if template == "" {
		template = "chatml"
	}

	// -------------------------------------------------------------------------

	m := model{
		model:     mdl,
		vocab:     vocab,
		ctxParams: cfg.ctxParams(),
		template:  template,
	}

	for _, option := range options {
		if err := option(&m); err != nil {
			llama.ModelFree(mdl)
			llama.BackendFree()
			return nil, err
		}
	}

	return &m, nil
}

func (m *model) unload() {
	llama.ModelFree(m.model)
	llama.BackendFree()
}

func (m *model) modelInfo() ModelInfo {
	desc := llama.ModelDesc(m.model)
	size := llama.ModelSize(m.model)
	encoder := llama.ModelHasEncoder(m.model)
	decoder := llama.ModelHasDecoder(m.model)
	recurrent := llama.ModelIsRecurrent(m.model)
	hybrid := llama.ModelIsHybrid(m.model)
	count := llama.ModelMetaCount(m.model)
	metadata := make(map[string]string)

	for i := range count {
		key, ok := llama.ModelMetaKeyByIndex(m.model, i)
		if !ok {
			continue
		}

		value, ok := llama.ModelMetaValStrByIndex(m.model, i)
		if !ok {
			continue
		}

		metadata[key] = value
	}

	return ModelInfo{
		Desc:        desc,
		Size:        size,
		HasEncoder:  encoder,
		HasDecoder:  decoder,
		IsRecurrent: recurrent,
		IsHybrid:    hybrid,
		Metadata:    metadata,
	}
}

func (m *model) chatCompletions(ctx context.Context, messages []ChatMessage, params Params) <-chan ChatResponse {
	ch := make(chan ChatResponse)

	go func() {
		defer close(ch)

		lctx, err := llama.InitFromModel(m.model, m.ctxParams)
		if err != nil {
			ch <- ChatResponse{Err: fmt.Errorf("unable to init from model: %w", err)}
			return
		}

		defer func() {
			llama.Synchronize(lctx)
			llama.Free(lctx)
		}()

		prompt := m.applyChatCompletionsTemplate(messages)
		m.processChatCompletions(ctx, lctx, prompt, toSampler(params), ch)
	}()

	return ch
}

func (m *model) applyChatCompletionsTemplate(messages []ChatMessage) string {
	msgs := make([]llama.ChatMessage, len(messages))
	for i, msg := range messages {
		msgs[i] = llama.NewChatMessage(msg.Role, msg.Content)
	}

	buf := make([]byte, 1024*32)
	l := llama.ChatApplyTemplate(m.template, msgs, true, buf)

	return string(buf[:l])
}

func (m *model) processChatCompletions(ctx context.Context, lctx llama.Context, prompt string, sampler llama.Sampler, ch chan<- ChatResponse) {
	tokens := llama.Tokenize(m.vocab, prompt, true, true)
	buf := make([]byte, 1024*32)

	for range llama.MaxToken {
		select {
		case <-ctx.Done():
			ch <- ChatResponse{Err: ctx.Err()}
			return
		default:
		}

		batch := llama.BatchGetOne(tokens)
		llama.Decode(lctx, batch)

		token := llama.SamplerSample(sampler, lctx, -1)

		if llama.VocabIsEOG(m.vocab, token) {
			break
		}

		l := llama.TokenToPiece(m.vocab, token, buf, 0, false)

		resp := string(buf[:l])
		if resp == "" {
			break
		}

		select {
		case <-ctx.Done():
			ch <- ChatResponse{Err: ctx.Err()}
			return
		case ch <- ChatResponse{
			Response: resp,
		}:
		}

		tokens = []llama.Token{token}
	}
}

func (m *model) chatVision(ctx context.Context, message ChatMessage, imageFile string, params Params) <-chan ChatResponse {
	ch := make(chan ChatResponse)

	go func() {
		defer close(ch)

		if m.projFile == "" {
			ch <- ChatResponse{Err: fmt.Errorf("projection file not set")}
			return
		}

		lctx, err := llama.InitFromModel(m.model, m.ctxParams)
		if err != nil {
			ch <- ChatResponse{Err: fmt.Errorf("unable to init from model: %w", err)}
			return
		}

		defer func() {
			llama.Synchronize(lctx)
			llama.Free(lctx)
		}()

		mctxParams := mtmd.ContextParamsDefault()
		mctxParams.UseGPU = true
		mctxParams.FlashAttentionType = llama.FlashAttentionTypeAuto

		mtmdCtx, err := mtmd.InitFromFile(m.projFile, m.model, mctxParams)
		if err != nil {
			ch <- ChatResponse{Err: fmt.Errorf("unable to init from model: %w", err)}
			return
		}
		defer mtmd.Free(mtmdCtx)

		prompt := m.applyChatVisionTemplate(message)

		bitmap, err := m.processBitmap(lctx, mtmdCtx, imageFile, prompt)
		if err != nil {
			ch <- ChatResponse{Err: err}
			return
		}
		defer mtmd.BitmapFree(bitmap)

		m.processChatVision(ctx, lctx, toSampler(params), ch)
	}()

	return ch
}

func (m *model) applyChatVisionTemplate(message ChatMessage) string {
	msgs := []llama.ChatMessage{
		llama.NewChatMessage(message.Role, message.Content),
		llama.NewChatMessage("user", mtmd.DefaultMarker()),
	}

	buf := make([]byte, 1024*32)
	l := llama.ChatApplyTemplate(m.template, msgs, true, buf)

	return string(buf[:l])
}

func (m *model) processBitmap(lctx llama.Context, mtmdCtx mtmd.Context, imageFile string, prompt string) (mtmd.Bitmap, error) {
	bitmap := mtmd.BitmapInitFromFile(mtmdCtx, imageFile)
	output := mtmd.InputChunksInit()
	input := mtmd.NewInputText(prompt, true, true)

	mtmd.Tokenize(mtmdCtx, output, input, []mtmd.Bitmap{bitmap})

	// Docs indicate this function is NOT thread-safe.
	func() {
		m.muHEC.Lock()
		defer m.muHEC.Unlock()
		var n llama.Pos
		mtmd.HelperEvalChunks(mtmdCtx, lctx, output, 0, 0, int32(m.ctxParams.NBatch), true, &n)
	}()

	return bitmap, nil
}

func (m *model) processChatVision(ctx context.Context, lctx llama.Context, sampler llama.Sampler, ch chan<- ChatResponse) {
	batch := llama.BatchInit(1, 0, 1)
	defer llama.BatchFree(batch)

	var sz int32 = 1
	batch.NSeqId = &sz
	batch.NTokens = 1
	seqs := unsafe.SliceData([]llama.SeqId{0})
	batch.SeqId = &seqs

	buf := make([]byte, 1024*32)

	for range llama.MaxToken {
		select {
		case <-ctx.Done():
			ch <- ChatResponse{Err: ctx.Err()}
			return
		default:
		}

		llama.Decode(lctx, batch)

		token := llama.SamplerSample(sampler, lctx, -1)

		if llama.VocabIsEOG(m.vocab, token) {
			break
		}

		l := llama.TokenToPiece(m.vocab, token, buf, 0, false)

		resp := string(buf[:l])
		if resp == "" {
			break
		}

		select {
		case <-ctx.Done():
			ch <- ChatResponse{Err: ctx.Err()}
			return
		case ch <- ChatResponse{
			Response: resp,
		}:
		}

		batch = llama.BatchGetOne([]llama.Token{token})
	}
}

func (m *model) embed(ctx context.Context, text string) ([]float32, error) {
	lctx, err := llama.InitFromModel(m.model, m.ctxParams)
	if err != nil {
		return nil, fmt.Errorf("unable to init from model: %w", err)
	}

	defer func() {
		llama.Synchronize(lctx)
		llama.Free(lctx)
	}()

	// Check context before potentially expensive operations
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	tokens := llama.Tokenize(m.vocab, text, true, true)
	batch := llama.BatchGetOne(tokens)
	llama.Decode(lctx, batch)

	dimensions := llama.ModelNEmbd(m.model)
	vec, err := llama.GetEmbeddingsSeq(lctx, 0, dimensions)
	if err != nil {
		return nil, fmt.Errorf("unable to get embeddings: %w", err)
	}

	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}

	if sum == 0 {
		return vec, nil
	}

	sum = math.Sqrt(sum)
	norm := float32(1.0 / sum)

	for i, v := range vec {
		vec[i] = v * norm
	}

	return vec, nil
}
