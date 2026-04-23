package model

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
//
// Text-only IMC jobs use first-available slot (KV restored from RAM).
// Media IMC jobs route to their target slot (KV stays in VRAM, slot-dedicated).
// Non-IMC jobs use first-available slot.
//
// Jobs that can't be assigned yet are held in pendingJobs (engine-local
// slice) rather than re-queued into requestQ, which would risk deadlocking
// the batch engine goroutine.
func (e *batchEngine) fillSlots(buf []byte) {
	// Drain new jobs from requestQ into pendingJobs.
	for {
		select {
		case job := <-e.requestQ:
			e.pendingJobs = append(e.pendingJobs, job)
		default:
			goto assign
		}
	}

assign:
	// Try to assign pending jobs to available slots.
	remaining := e.pendingJobs[:0]
	for _, job := range e.pendingJobs {
		if job.ctx.Err() != nil {
			e.failJob(job, job.ctx.Err())
			continue
		}

		assigned := false

		// Media IMC cache hits must go to their specific slot because
		// the media KV state (image/audio embeddings) stays in VRAM.
		if job.imcCacheHit && job.imcSessionMedia {
			targetSlotID := job.imcSlotID
			if targetSlotID < len(e.slots) {
				s := e.slots[targetSlotID]
				if !s.active {
					e.startSlot(s, job, buf)
					assigned = true
				}
			}
		} else {
			// Text-only and non-IMC: assign to any available slot.
			for _, s := range e.slots {
				if s.active {
					continue
				}
				e.startSlot(s, job, buf)
				assigned = true
				break
			}
		}

		if !assigned {
			remaining = append(remaining, job)
		}
	}
	e.pendingJobs = remaining
}
