// This example qualifies concurrent decision requests against one loaded
// Jev-Style model and one llama.cpp context. Request goroutines submit work to
// a scheduler goroutine, which is the sole owner of context, memory, process,
// and logits operations. The scheduler combines requests under distinct
// sequence IDs in one process call.
//
// The example first evaluates each request serially, then submits all requests
// concurrently and compares their answers and probabilities. Concurrency here
// means batched request service, not concurrent calls into one llama context.
//
// Run the example like this from the root of the project:
// $ make example-yzma-step9

package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/examples/yzma/internal/yzmainit"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	modelSource = "https://huggingface.co/chaoliangUNSW/Jev-Style-0.8B-Decision-v3-GGUF/resolve/main/Jev-Style-0.8B-Decision-v3-Q8_0.gguf"

	contextSize          = 8 * 1024
	physicalBatchSize    = 1024
	maxConcurrent        = 4
	maxOutputs           = 64
	batchWindow          = 100 * time.Millisecond
	probabilityTolerance = 0.01

	yesToken    llama.Token = 9542
	noToken     llama.Token = 874
	slotToken   llama.Token = 1411
	temperature             = 0.8800546821789332
)

type decisionInput struct {
	state    string
	question string
	options  []string
}

type decisionWork struct {
	names  []string
	tokens []llama.Token
	slots  []int
}

type decisionResult struct {
	answer        string
	probabilities []float64
}

type scheduledRequest struct {
	ctx      context.Context
	work     decisionWork
	response chan scheduledResponse
}

type scheduledResponse struct {
	result decisionResult
	err    error
}

type decisionScheduler struct {
	lctx         llama.Context
	mem          llama.Memory
	batch        llama.BatchExt
	nVocab       int
	queue        chan scheduledRequest
	done         chan struct{}
	processCalls atomic.Int64
}

func main() {
	if err := run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func run() error {
	modelFile, err := installModel()
	if err != nil {
		return err
	}

	if err := initYzma(); err != nil {
		return err
	}

	fmt.Println("Loading model once:", modelFile)

	mdl, err := llama.ModelLoadFromFile(modelFile, llama.ModelDefaultParams())
	if err != nil {
		return fmt.Errorf("load model: %w", err)
	}
	defer llama.ModelFree(mdl)

	vocab := llama.ModelGetVocab(mdl)
	if err := validateProtocolTokens(vocab); err != nil {
		return err
	}

	scheduler, err := newDecisionScheduler(mdl)
	if err != nil {
		return err
	}
	defer scheduler.close()

	inputs := []decisionInput{
		{
			state:    "A customer says they were charged twice for one subscription renewal.",
			question: "Which team should handle this request?",
			options:  []string{"billing: payments and duplicate charges", "support: product bugs", "sales: new purchases"},
		},
		{
			state:    "A customer cannot sign in after resetting their password.",
			question: "Which team should handle this request?",
			options:  []string{"accounts: login and access", "billing: payments", "sales: new purchases"},
		},
		{
			state:    "A prospect asks for pricing and wants to upgrade to the enterprise plan.",
			question: "Which team should handle this request?",
			options:  []string{"support: product bugs", "sales: plans and pricing", "billing: existing invoices"},
		},
		{
			state:    "A customer says they were charged twice for one subscription renewal.",
			question: "Which team should handle this request?",
			options:  []string{"billing: payments and duplicate charges", "support: product bugs", "sales: new purchases"},
		},
	}

	work := make([]decisionWork, len(inputs))
	for i, input := range inputs {
		work[i], err = render(vocab, input)
		if err != nil {
			return fmt.Errorf("render request %d: %w", i, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	serial := make([]decisionResult, len(work))
	for i, item := range work {
		serial[i], err = scheduler.submit(ctx, item)
		if err != nil {
			return fmt.Errorf("serial request %d: %w", i, err)
		}
	}
	serialCalls := scheduler.processCalls.Load()

	concurrent := make([]decisionResult, len(work))
	errs := make([]error, len(work))
	start := make(chan struct{})

	var wg sync.WaitGroup
	for i, item := range work {
		wg.Go(func() {
			<-start
			concurrent[i], errs[i] = scheduler.submit(ctx, item)
		})
	}

	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("concurrent request %d: %w", i, err)
		}
	}

	concurrentCalls := scheduler.processCalls.Load() - serialCalls
	if concurrentCalls != 1 {
		return fmt.Errorf("concurrent requests used %d process calls, want 1", concurrentCalls)
	}

	fmt.Println("ModelLoads           : 1")
	fmt.Println("ContextLoads         : 1")
	fmt.Println("ConcurrentRequests   :", len(work))
	fmt.Println("SerialProcessCalls    :", serialCalls)
	fmt.Println("ConcurrentProcessCalls:", concurrentCalls)

	for i := range work {
		delta, err := compare(serial[i], concurrent[i])
		if err != nil {
			return fmt.Errorf("request %d: %w", i, err)
		}

		fmt.Printf("Request[%d] answer=%q max-probability-delta=%.8g\n", i, concurrent[i].answer, delta)
	}

	duplicateDelta, err := compare(concurrent[0], concurrent[len(concurrent)-1])
	if err != nil {
		return fmt.Errorf("duplicate request: %w", err)
	}
	fmt.Printf("DuplicateProbabilityDelta: %.8g\n", duplicateDelta)
	fmt.Println("RESULT: PASS")

	return nil
}

func installModel() (string, error) {
	mdls, err := models.New()
	if err != nil {
		return "", fmt.Errorf("initialize models: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelSource)
	if err != nil {
		return "", fmt.Errorf("download model: %w", err)
	}
	if len(mp.ModelFiles) == 0 {
		return "", fmt.Errorf("download returned no model files")
	}

	return mp.ModelFiles[0], nil
}

func initYzma() error {
	libPath, err := yzmainit.LibraryPath()
	if err != nil {
		return fmt.Errorf("prepare library path: %w", err)
	}

	if err := llama.Load(libPath); err != nil {
		return fmt.Errorf("load library: %w", err)
	}

	if err := llama.Init(); err != nil {
		return fmt.Errorf("initialize llama: %w", err)
	}
	llama.LogSet(llama.LogSilent())

	return nil
}

func validateProtocolTokens(vocab llama.Vocab) error {
	expected := []struct {
		text  string
		token llama.Token
	}{
		{text: " yes", token: yesToken},
		{text: " no", token: noToken},
		{text: " ->", token: slotToken},
	}

	for _, item := range expected {
		tokens := llama.Tokenize(vocab, item.text, false, false)
		if len(tokens) != 1 || tokens[0] != item.token {
			return fmt.Errorf("protocol token %q: got %v, want [%d]", item.text, tokens, item.token)
		}
	}

	return nil
}

func render(vocab llama.Vocab, input decisionInput) (decisionWork, error) {
	if len(input.options) < 2 {
		return decisionWork{}, fmt.Errorf("decision needs at least two options")
	}

	encode := func(text string) []llama.Token {
		return llama.Tokenize(vocab, text, false, false)
	}

	tokens := encode(fmt.Sprintf("State:\n%s\n\nQuestion [choice]: %s\nOptions:\n", input.state, input.question))
	for _, option := range input.options {
		tokens = append(tokens, encode("- "+option+"\n")...)
	}
	tokens = append(tokens, encode("Judge each option:\n")...)

	slots := make([]int, len(input.options))
	for i, option := range input.options {
		tokens = append(tokens, encode(option)...)
		slots[i] = len(tokens)
		tokens = append(tokens, slotToken)
		tokens = append(tokens, encode("\n")...)
	}

	return decisionWork{
		names:  append([]string(nil), input.options...),
		tokens: tokens,
		slots:  slots,
	}, nil
}

func newDecisionScheduler(mdl llama.Model) (*decisionScheduler, error) {
	params := llama.ContextDefaultParams()
	params.NCtx = contextSize
	params.NBatch = contextSize
	params.NUbatch = physicalBatchSize
	params.NSeqMax = maxConcurrent
	params.NOutputsMax = maxOutputs
	params.NOutputsMaxPerSeq = maxOutputs
	params.KVUnified = 1
	params.NoPerf = 1

	lctx, err := llama.InitFromModel(mdl, params)
	if err != nil {
		return nil, fmt.Errorf("initialize context: %w", err)
	}

	mem, err := llama.GetMemory(lctx)
	if err != nil {
		llama.Free(lctx)
		return nil, fmt.Errorf("get context memory: %w", err)
	}
	if err := llama.MemoryClear(mem, true); err != nil {
		llama.Free(lctx)
		return nil, fmt.Errorf("clear context memory: %w", err)
	}

	batch, err := llama.BatchExtInit(lctx)
	if err != nil {
		llama.Free(lctx)
		return nil, fmt.Errorf("initialize batch: %w", err)
	}

	scheduler := decisionScheduler{
		lctx:   lctx,
		mem:    mem,
		batch:  batch,
		nVocab: int(llama.VocabNTokens(llama.ModelGetVocab(mdl))),
		queue:  make(chan scheduledRequest, maxConcurrent),
		done:   make(chan struct{}),
	}
	go scheduler.run()

	return &scheduler, nil
}

func (s *decisionScheduler) submit(ctx context.Context, work decisionWork) (decisionResult, error) {
	response := make(chan scheduledResponse, 1)
	request := scheduledRequest{ctx: ctx, work: work, response: response}

	select {
	case s.queue <- request:
	case <-ctx.Done():
		return decisionResult{}, ctx.Err()
	}

	select {
	case resp := <-response:
		return resp.result, resp.err
	case <-ctx.Done():
		return decisionResult{}, ctx.Err()
	}
}

func (s *decisionScheduler) run() {
	defer close(s.done)

	for first := range s.queue {
		requests := []scheduledRequest{first}
		timer := time.NewTimer(batchWindow)

	collect:
		for len(requests) < maxConcurrent {
			select {
			case request := <-s.queue:
				requests = append(requests, request)
			case <-timer.C:
				break collect
			}
		}

		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		s.process(requests)
	}
}

func (s *decisionScheduler) process(requests []scheduledRequest) {
	active := requests[:0]
	for _, request := range requests {
		if err := request.ctx.Err(); err != nil {
			request.response <- scheduledResponse{err: err}
			continue
		}
		active = append(active, request)
	}
	if len(active) == 0 {
		return
	}

	results, err := s.decode(active)
	if err != nil {
		clearErr := llama.MemoryClear(s.mem, true)
		if clearErr != nil {
			err = errors.Join(err, fmt.Errorf("clear context memory: %w", clearErr))
		}
		for _, request := range active {
			request.response <- scheduledResponse{err: err}
		}
		return
	}

	for i, request := range active {
		request.response <- scheduledResponse{result: results[i]}
	}
}

func (s *decisionScheduler) decode(requests []scheduledRequest) ([]decisionResult, error) {
	var totalTokens int
	var totalOutputs int
	for _, request := range requests {
		totalTokens += len(request.work.tokens)
		totalOutputs += len(request.work.slots)
	}
	if totalTokens > contextSize {
		return nil, fmt.Errorf("batch has %d tokens, context limit is %d", totalTokens, contextSize)
	}
	if totalOutputs > maxOutputs {
		return nil, fmt.Errorf("batch has %d outputs, limit is %d", totalOutputs, maxOutputs)
	}

	if err := llama.BatchExtClear(s.batch); err != nil {
		return nil, fmt.Errorf("clear batch: %w", err)
	}

	indices := make([][]int32, len(requests))
	for requestIndex, request := range requests {
		seqID := llama.SeqId(requestIndex)
		slotAt := make(map[int]bool, len(request.work.slots))
		for _, slot := range request.work.slots {
			slotAt[slot] = true
		}

		for position, token := range request.work.tokens {
			idx, err := llama.BatchExtAddToken(s.batch, seqID, token)
			if err != nil {
				return nil, fmt.Errorf("add request %d token at position %d: %w", requestIndex, position, err)
			}
			if err := llama.BatchExtSetPos(s.batch, idx, llama.Pos(position)); err != nil {
				return nil, fmt.Errorf("set request %d token position %d: %w", requestIndex, position, err)
			}
			if slotAt[position] {
				if err := llama.BatchExtSetOutputLogits(s.batch, idx, true); err != nil {
					return nil, fmt.Errorf("request %d logits at position %d: %w", requestIndex, position, err)
				}
				indices[requestIndex] = append(indices[requestIndex], idx)
			}
		}
	}

	code, err := llama.Process(s.lctx, llama.ProcessTypeDecode, s.batch)
	if err != nil {
		return nil, fmt.Errorf("process: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("process returned %d", code)
	}
	s.processCalls.Add(1)

	results := make([]decisionResult, len(requests))
	for requestIndex, request := range requests {
		scores := make([]float64, len(indices[requestIndex]))
		for slotIndex, batchIndex := range indices[requestIndex] {
			logits, err := llama.GetLogitsIth(s.lctx, batchIndex, s.nVocab)
			if err != nil {
				return nil, fmt.Errorf("request %d logits at batch index %d: %w", requestIndex, batchIndex, err)
			}
			if logits == nil {
				return nil, fmt.Errorf("request %d has no logits at batch index %d", requestIndex, batchIndex)
			}
			scores[slotIndex] = float64(logits[yesToken] - logits[noToken])
		}

		probabilities, err := softmax(scores)
		if err != nil {
			return nil, fmt.Errorf("request %d: %w", requestIndex, err)
		}

		top := 0
		for i := range probabilities {
			if probabilities[i] > probabilities[top] {
				top = i
			}
		}
		results[requestIndex] = decisionResult{
			answer:        request.work.names[top],
			probabilities: probabilities,
		}
	}

	for i := range requests {
		if _, err := llama.MemorySeqRm(s.mem, llama.SeqId(i), -1, -1); err != nil {
			return nil, fmt.Errorf("remove sequence %d: %w", i, err)
		}
	}

	return results, nil
}

func (s *decisionScheduler) close() {
	close(s.queue)
	<-s.done
	llama.Synchronize(s.lctx)
	llama.BatchExtFree(s.batch)
	llama.Free(s.lctx)
}

func softmax(scores []float64) ([]float64, error) {
	maximum := math.Inf(-1)
	for _, score := range scores {
		scaled := score / temperature
		if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
			return nil, fmt.Errorf("score is not finite")
		}
		maximum = max(maximum, scaled)
	}

	probabilities := make([]float64, len(scores))
	var sum float64
	for i, score := range scores {
		probabilities[i] = math.Exp(score/temperature - maximum)
		sum += probabilities[i]
	}
	for i := range probabilities {
		probabilities[i] /= sum
	}

	return probabilities, nil
}

func compare(want, got decisionResult) (float64, error) {
	if got.answer != want.answer {
		return 0, fmt.Errorf("answer got %q, want %q", got.answer, want.answer)
	}
	if len(got.probabilities) != len(want.probabilities) {
		return 0, fmt.Errorf("got %d probabilities, want %d", len(got.probabilities), len(want.probabilities))
	}

	var maximumDelta float64
	for i := range got.probabilities {
		maximumDelta = max(maximumDelta, math.Abs(got.probabilities[i]-want.probabilities[i]))
	}
	if maximumDelta > probabilityTolerance {
		return maximumDelta, fmt.Errorf("maximum probability delta %.8g exceeds %.4g", maximumDelta, probabilityTolerance)
	}

	return maximumDelta, nil
}
