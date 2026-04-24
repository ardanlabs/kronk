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
// All IMC jobs (text and media) use first-available slot — KV state is
// restored from RAM via StateSeqSetData. Non-IMC jobs also use
// first-available.
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

		for _, s := range e.slots {
			if s.active {
				continue
			}
			e.startSlot(s, job, buf)
			assigned = true
			break
		}

		if !assigned {
			remaining = append(remaining, job)
		}
	}
	e.pendingJobs = remaining
}
