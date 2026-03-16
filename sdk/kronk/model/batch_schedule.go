package model

import (
	"fmt"
	"time"
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

// cacheSlotTimeout returns the configured cache slot timeout duration.
func (e *batchEngine) cacheSlotTimeout() time.Duration {
	return time.Duration(e.model.cfg.CacheSlotTimeout) * time.Second
}

// jobCancelled checks if the job's context has been cancelled and fails it
// if so. Returns true if the job was cancelled and cleaned up.
func (e *batchEngine) jobCancelled(job *chatJob) bool {
	if job.ctx.Err() != nil {
		e.failJob(job, job.ctx.Err())
		return true
	}
	return false
}

// fillSlots assigns pending requests to available slots.
func (e *batchEngine) fillSlots(buf []byte) {
	if e.model.cfg.IncrementalCache {
		e.fillSlotsIMC(buf)
		return
	}

	for _, s := range e.slots {
		if s.active {
			continue
		}

		// Try to get a request from the queue.
		select {
		case job := <-e.requestQ:
			e.startSlot(s, job, buf)
			return // Only prefill one slot per iteration to avoid exceeding NBatch

		default:
			return
		}
	}
}

// nextIMCJob returns the next job to process, checking the deferred job first
// before reading from the request queue.
func (e *batchEngine) nextIMCJob() *chatJob {
	if e.deferredJob != nil {
		job := e.deferredJob
		e.deferredJob = nil
		return job
	}

	select {
	case job := <-e.requestQ:
		return job
	default:
		return nil
	}
}

// fillSlotsIMC routes IMC jobs to their target slot. processIMC determines
// which slot to use via hash matching; the target slot index is carried
// in the job's imc.slotID field.
func (e *batchEngine) fillSlotsIMC(buf []byte) {

	// Don't schedule new work while a preemption is pending. The slot will
	// be freed at the top of the next processBatch iteration.
	if e.pendingPreempt != nil {
		return
	}

	job := e.nextIMCJob()
	if job == nil {
		return
	}

	// Drop cancelled jobs immediately to avoid preempting a live slot
	// for a request nobody is waiting for.
	if e.jobCancelled(job) {
		return
	}

	// Route to the specific slot determined by processIMC.
	if job.imc != nil && job.imc.slotID < len(e.slots) {
		s := e.slots[job.imc.slotID]
		if !s.active {
			e.startSlot(s, job, buf)
			return
		}

		// Target slot is busy. Check if the job has waited longer than
		// CacheSlotTimeout. If so, schedule preemption of the target slot.
		timeout := e.cacheSlotTimeout()
		if time.Since(job.queuedAt) >= timeout {
			e.schedulePreempt(s, job)
			return
		}

		// Under timeout — defer the job for the next iteration.
		e.deferredJob = job
		return
	}

	// No IMC routing (no cache hit or invalid slot) — assign to any
	// available slot.
	for _, s := range e.slots {
		if !s.active {
			e.startSlot(s, job, buf)
			return
		}
	}

	// All slots busy. Check if the job has waited longer than
	// CacheSlotTimeout. If so, preempt the longest-running slot.
	timeout := e.cacheSlotTimeout()
	if time.Since(job.queuedAt) >= timeout {
		victim := e.longestRunningSlot()
		if victim != nil {
			e.schedulePreempt(victim, job)
			return
		}
	}

	// Under timeout — defer the job for the next iteration.
	e.deferredJob = job
}

// schedulePreempt marks a slot for preemption at the start of the next
// processBatch iteration and defers the waiting job.
func (e *batchEngine) schedulePreempt(victim *slot, job *chatJob) {
	waited := time.Since(job.queuedAt)

	e.model.log(job.ctx, "batch-engine",
		"status", "preempting-slot",
		"slot", victim.id,
		"victim_id", victim.job.id,
		"victim_output_tokens", victim.reasonTokens+victim.completionTokens,
		"victim_running", time.Since(victim.prefillStart).String(),
		"queued_job_id", job.id,
		"queued_wait", waited.String(),
	)

	e.pendingPreempt = victim
	e.pendingPreemptErr = fmt.Errorf("preempted by queued request %s after %s wait", job.id, waited)
	e.deferredJob = job
}

// longestRunningSlot returns the active slot that has been running the longest,
// or nil if no slots are active.
func (e *batchEngine) longestRunningSlot() *slot {
	var victim *slot

	for _, s := range e.slots {
		if !s.active {
			continue
		}

		if victim == nil || s.prefillStart.Before(victim.prefillStart) {
			victim = s
		}
	}

	return victim
}
