package model

import "fmt"

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

// fillSlotsIMC routes IMC jobs to their dedicated slots. Each cache_id is
// bound to a specific slot, so jobs must wait for their assigned slot.
func (e *batchEngine) fillSlotsIMC() {
	select {
	case job := <-e.requestQ:
		// Find the dedicated slot for this job's cache_id.
		if job.imcID != "" {
			e.model.cacheMu.RLock()
			session, exists := e.model.imcSessions[job.imcID]
			e.model.cacheMu.RUnlock()

			if exists && session.slotID < len(e.slots) {
				s := e.slots[session.slotID]
				if !s.active {
					e.startSlot(s, job)
					return
				}

				// Dedicated slot is busy — put job back for retry.
				select {
				case e.requestQ <- job:
				default:
					e.finishSlot(s, fmt.Errorf("fillSlots: IMC queue full, dropping request"))
				}
				return
			}
		}

		// No dedicated slot found (new session or no cache_id).
		// If all slots are bound to IMC sessions and this job needs a
		// new session, no slot is available. Reject with an error rather
		// than assigning to a bound slot (which would destroy that
		// user's cache) or re-queuing forever.
		if job.imcID != "" {
			e.model.cacheMu.RLock()
			boundSlots := len(e.model.imcSessions)
			e.model.cacheMu.RUnlock()

			if boundSlots >= e.nSlots {
				e.model.log(job.ctx, "batch-engine", "status", "imc-slots-full",
					"bound", boundSlots, "total", e.nSlots, "cache_id", job.imcID)

				e.model.sendErrorResponse(job.ctx, job.ch, job.id, job.object, 0, "",
					fmt.Errorf("all %d inference slots are bound to IMC sessions, no capacity for additional requests", e.nSlots),
					Usage{})

				if job.queueWaitSpan != nil {
					job.queueWaitSpan.End()
				}

				close(job.ch)
				e.model.activeStreams.Add(-1)
				return
			}
		}

		// Assign to any available slot.
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
		}

	default:
	}
}
