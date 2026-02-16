package model

import (
	"fmt"
)

// hasActiveSlots returns true if any slot is currently processing.
func (e *batchEngine) hasActiveSlots() bool {
	for _, s := range e.slots {
		if s.active {
			return true
		}
	}
	return false
}

// fillSlots assigns pending requests to available slots.
func (e *batchEngine) fillSlots() {
	if e.model.cfg.IncrementalCache {
		e.fillSlotsIMC()
		return
	}

	for _, s := range e.slots {
		if s.active {
			continue
		}

		// Try to get a request from the queue.
		select {
		case job := <-e.requestQ:
			e.startSlot(s, job)
			return // Only prefill one slot per iteration to avoid exceeding NBatch

		default:
			return
		}
	}
}

// fillSlotsIMC routes IMC jobs to their target slot. processIMC determines
// which slot to use via hash matching; the target slot index is carried
// in the job's imcSlotID field.
func (e *batchEngine) fillSlotsIMC() {
	select {
	case job := <-e.requestQ:
		// Route to the specific slot determined by processIMC.
		targetSlotID := job.imcSlotID
		if job.imcCacheHit && targetSlotID < len(e.slots) {
			s := e.slots[targetSlotID]
			if !s.active {
				e.startSlot(s, job)
				return
			}

			// Target slot is busy — put job back for retry.
			select {
			case e.requestQ <- job:
			default:
				e.failJob(job, fmt.Errorf("fillSlots: IMC queue full, dropping request"))
			}
			return
		}

		// No IMC routing (no cache hit or invalid slot) — assign to any
		// available slot.
		for _, s := range e.slots {
			if !s.active {
				e.startSlot(s, job)
				return
			}
		}

		// All slots busy — put job back.
		select {
		case e.requestQ <- job:
		default:
			e.failJob(job, fmt.Errorf("fillSlots: all slots busy, dropping request"))
		}

	default:
	}
}
