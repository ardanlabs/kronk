package speech

import (
	"context"
	"errors"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// NonAutoregressive owns a helper that encodes the whole phoneme segment and
// then vocodes it, without external sampling or a persistent backbone cache.
// Calls sharing the borrowed contexts must be serialized by the scheduler.
type NonAutoregressive struct {
	helper     mtmd.HelperGenAudio
	batchSize  int32
	prefilling bool
}

// NewNonAutoregressive creates a helper borrowing the backbone and decoder.
func NewNonAutoregressive(lctx llama.Context, decoder mtmd.Context) (*NonAutoregressive, error) {
	helper, err := mtmd.HelperGenAudioInit(lctx, decoder)
	if err != nil {
		return nil, err
	}
	n := NonAutoregressive{helper: helper, batchSize: int32(llama.NUBatch(lctx))}
	return &n, nil
}

// Start supplies a model-prepared phoneme prompt and preset voice.
func (n *NonAutoregressive) Start(input *mtmd.HelperGenAudioInput) error {
	if err := mtmd.HelperGenAudioSetInput(n.helper, input); err != nil {
		return err
	}
	n.prefilling = true
	return nil
}

// Step encodes the segment or returns its complete waveform. Native encoding
// and vocoding calls cannot be interrupted; cancellation is checked around them.
func (n *NonAutoregressive) Step(ctx context.Context, _ int) (Output, bool, bool, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, false, false, err
	}
	if n.prefilling {
		remaining, err := mtmd.HelperGenAudioStepPrompt(n.helper, n.batchSize)
		if err != nil {
			return Output{}, false, true, fmt.Errorf("speech phoneme encoding: %w", err)
		}
		n.prefilling = remaining > 0
		return Output{}, false, false, nil
	}
	_, stop, err := mtmd.HelperGenAudioStepGen(n.helper, llama.TokenNull, nil)
	if err != nil {
		return Output{}, false, true, err
	}
	if !stop {
		return Output{}, false, true, errors.New("non-autoregressive speech helper did not finish encoding")
	}
	output, err := readMonoOutput(n.helper)
	if cancelErr := ctx.Err(); cancelErr != nil {
		return Output{}, false, false, cancelErr
	}
	if err != nil {
		return Output{}, false, true, err
	}
	return output, true, false, nil
}

// Reset releases the helper's segment and waveform state. The noncausal
// backbone has no sequence cache to remove.
func (n *NonAutoregressive) Reset() error {
	mtmd.HelperGenAudioReset(n.helper)
	n.prefilling = false
	return nil
}

// Free releases the helper without freeing either borrowed context.
func (n *NonAutoregressive) Free() {
	mtmd.HelperGenAudioFree(n.helper)
	n.helper = 0
}
