package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// slotCancelError returns an appropriate error for a cancelled slot.
// Uses context error if available, otherwise returns a shutdown error.
func (e *batchEngine) slotCancelError(s *slot) error {
	if err := s.job.ctx.Err(); err != nil {
		return err
	}
	return errors.New("engine shutting down")
}

// logDecodeError logs detailed KV cache diagnostics when decode fails.
func (e *batchEngine) logDecodeError(ctx context.Context, ret int32, err error) {
	nCtx := llama.NCtx(e.model.lctx)

	// Collect per-slot diagnostics (active slots only).
	var activeTokens llama.Pos
	slotInfo := make([]string, 0, e.nSlots)

	for _, s := range e.slots {
		if !s.active {
			continue
		}
		posMax, posErr := llama.MemorySeqPosMax(e.model.mem, s.seqID)
		if posErr == nil && posMax >= 0 {
			tokens := posMax + 1
			slotInfo = append(slotInfo, fmt.Sprintf("slot[%d,seq=%d]=%d", s.id, s.seqID, tokens))
			activeTokens += tokens
		}
	}

	// Collect total KV usage across all sequences (including idle IMC slots)
	// to provide accurate diagnostics when the unified KV cache is full.
	var totalKV int
	imcInfo := make([]string, 0, len(e.model.imcSlots))

	e.model.cacheMu.RLock()
	for _, slot := range e.model.imcSlots {
		if slot.totalTokensCached > 0 {
			imcInfo = append(imcInfo, fmt.Sprintf("imc[%d,seq=%d]=%d", slot.slotID, slot.seqID, slot.totalTokensCached))
			totalKV += slot.totalTokensCached
		}
	}
	e.model.cacheMu.RUnlock()

	e.model.log(ctx, "batch-engine",
		"status", "decode-error",
		"ret", ret,
		"err", err,
		"n_ctx", nCtx,
		"kv_used_active", activeTokens,
		"kv_used_total", totalKV,
		"batch_tokens", e.batch.NTokens,
		"active_slots", len(slotInfo),
		"slot_usage", strings.Join(slotInfo, ","),
		"imc_usage", strings.Join(imcInfo, ","),
	)
}

// decodeError returns a human-readable error message for llama_decode return codes.
// Return codes from llama.cpp:
//
//	0  - success
//	1  - could not find a KV slot for the batch (try reducing batch size or increase context)
//	2  - aborted
//	-1 - invalid input batch
//	<-1 - fatal error
func decodeError(ret int32, err error) error {
	var msg string
	switch ret {
	case 1:
		msg = "unable to process request: the context window is full. Please reduce the input size or increase the context window"
	case 2:
		msg = "request was cancelled"
	case -1:
		msg = "unable to process request: the input could not be processed. Please try reducing the input size or context length"
	default:
		switch {
		case ret < -1:
			msg = "an internal error occurred while processing your request"
		default:
			msg = "an unexpected error occurred while processing your request"
		}
	}

	if err != nil {
		return fmt.Errorf("%s: %w", msg, err)
	}
	return errors.New(msg)
}
