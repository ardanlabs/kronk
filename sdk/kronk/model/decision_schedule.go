package model

import (
	"fmt"
	"slices"
)

type decisionScheduledEntry struct {
	job        *decisionJob
	workOffset int
	work       decisionWork
}

type decisionJobFailure struct {
	job *decisionJob
	err error
}

type decisionSchedule struct {
	entries  []decisionScheduledEntry
	done     []*decisionJob
	failed   []decisionJobFailure
	nTokens  int
	nOutputs int
}

func decisionWorkOutputs(work decisionWork) int {
	if work.jointScores > 0 {
		return len(work.tokens)
	}
	return len(work.readouts)
}

// scheduleDecision selects complete work items from active requests in
// round-robin order. Jobs that still have work return in the order they should
// be considered for the next native batch.
func scheduleDecision(jobs []*decisionJob, maxSequences, maxTokens, maxOutputs int) (decisionSchedule, []*decisionJob, error) {
	if maxSequences <= 0 {
		return decisionSchedule{}, nil, fmt.Errorf("schedule-decision: sequence limit must be positive")
	}
	if maxTokens <= 0 {
		return decisionSchedule{}, nil, fmt.Errorf("schedule-decision: token limit must be positive")
	}
	if maxOutputs <= 0 {
		return decisionSchedule{}, nil, fmt.Errorf("schedule-decision: output limit must be positive")
	}
	if len(jobs) == 0 {
		return decisionSchedule{}, nil, nil
	}

	schedule := decisionSchedule{
		entries: make([]decisionScheduledEntry, 0, maxSequences),
	}
	pending := slices.Clone(jobs)
	deferred := 0

	for len(pending) > 0 && len(schedule.entries) < maxSequences {
		job := pending[0]
		pending = pending[1:]

		if job.next >= len(job.work) {
			schedule.done = append(schedule.done, job)
			deferred = 0
			continue
		}

		item := job.work[job.next]
		outputs := decisionWorkOutputs(item)
		switch {
		case len(item.tokens) == 0:
			schedule.failed = append(schedule.failed, decisionJobFailure{
				job: job,
				err: fmt.Errorf("schedule-decision: work[%d] has no tokens", job.next),
			})
			deferred = 0
			continue

		case len(item.tokens) > maxTokens:
			schedule.failed = append(schedule.failed, decisionJobFailure{
				job: job,
				err: fmt.Errorf("schedule-decision: work[%d] has %d tokens but limit is %d", job.next, len(item.tokens), maxTokens),
			})
			deferred = 0
			continue

		case outputs > maxOutputs:
			schedule.failed = append(schedule.failed, decisionJobFailure{
				job: job,
				err: fmt.Errorf("schedule-decision: work[%d] has %d outputs but limit is %d", job.next, outputs, maxOutputs),
			})
			deferred = 0
			continue

		case schedule.nTokens+len(item.tokens) > maxTokens || schedule.nOutputs+outputs > maxOutputs || item.jointScores > 0 && len(schedule.entries) > 0:
			pending = append(pending, job)
			deferred++
			if deferred >= len(pending) {
				return schedule, pending, nil
			}
			continue
		}

		schedule.entries = append(schedule.entries, decisionScheduledEntry{
			job:        job,
			workOffset: job.next,
			work:       item,
		})
		schedule.nTokens += len(item.tokens)
		schedule.nOutputs += outputs
		job.next++
		deferred = 0

		if job.next == len(job.work) {
			schedule.done = append(schedule.done, job)
		} else {
			pending = append(pending, job)
		}
		if item.jointScores > 0 {
			break
		}
	}

	return schedule, pending, nil
}
