package kronk

import (
	"context"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	modeChat   = "chat"
	modeVision = "vision"
)

type model struct {
	cfg       ModelConfig
	model     llama.Model
	vocab     llama.Vocab
	ctxParams llama.ContextParams
	template  string
	projFile  string
}

func newModel(modelFile string, projFile string, cfg ModelConfig) (*model, error) {
	mparams := llama.ModelDefaultParams()
	if cfg.Device != "" {
		dev := llama.GGMLBackendDeviceByName(cfg.Device)
		if dev == 0 {
			return nil, fmt.Errorf("unknown device: %s", cfg.Device)
		}
		mparams.SetDevices([]llama.GGMLBackendDevice{dev})
	}

	mdl, err := llama.ModelLoadFromFile(modelFile, mparams)
	if err != nil {
		return nil, fmt.Errorf("ModelLoadFromFile: %w", err)
	}

	cfg = adjustConfig(cfg, mdl)

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
		cfg:       cfg,
		model:     mdl,
		vocab:     vocab,
		ctxParams: modelCtxParams(cfg),
		template:  template,
		projFile:  projFile,
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

func (m *model) processTokens(ctx context.Context, lctx llama.Context, mode string, prompt string, params Params, ch chan<- ChatResponse) {
	var inputTokens int
	var outputTokens int
	var contextTokens int
	var totalOutTokens int

	var tokens []llama.Token
	var batch llama.Batch

	params = adjustParams(params)
	sampler := toSampler(params)

	tokens = llama.Tokenize(m.vocab, prompt, true, true)
	batch = llama.BatchGetOne(tokens)

	inputTokens = int(batch.NTokens)
	inputTokens += inputTokens
	contextTokens += inputTokens

	switch mode {
	case modeVision:
		tokens = []llama.Token{llama.SamplerSample(sampler, lctx, -1)}
		batch = llama.BatchGetOne(tokens)

		outputTokens = int(batch.NTokens)
		totalOutTokens += outputTokens
		contextTokens += outputTokens
	}

	const bufferSize = 32 * 1024
	buf := make([]byte, bufferSize)

	for totalOutTokens <= params.MaxTokens {
		select {
		case <-ctx.Done():
			ch <- ChatResponse{
				Err: ctx.Err(),
				Tokens: Tokens{
					Input:   inputTokens,
					Output:  outputTokens,
					Context: contextTokens,
				},
			}
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
			ch <- ChatResponse{
				Err: ctx.Err(),
				Tokens: Tokens{
					Input:   inputTokens,
					Output:  outputTokens,
					Context: contextTokens,
				},
			}
			return

		case ch <- ChatResponse{
			Response: resp,
			Tokens: Tokens{
				Input:   inputTokens,
				Output:  outputTokens,
				Context: contextTokens,
			}}:
		}

		tokens = []llama.Token{token}
		batch = llama.BatchGetOne(tokens)

		outputTokens = int(batch.NTokens)
		totalOutTokens += outputTokens
		contextTokens += outputTokens
	}
}
