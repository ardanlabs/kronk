package model

import (
	"fmt"
	"slices"

	"github.com/hybridgroup/yzma/pkg/llama"
)

type batchSeqItem struct {
	index  int
	tokens []llama.Token
}

type batchSeqEntry struct {
	itemOffset int
	itemIndex  int
	seqID      llama.SeqId
	tokens     []llama.Token
}

type batchSeqPlan struct {
	entries []batchSeqEntry
	nTokens int
	next    int
}

func planBatchSeqItems(items []batchSeqItem, start, maxSequences, maxTokens int) (batchSeqPlan, error) {
	if start < 0 || start > len(items) {
		return batchSeqPlan{}, fmt.Errorf("plan-batchseq: start index %d outside item range %d", start, len(items))
	}
	if maxSequences <= 0 {
		return batchSeqPlan{}, fmt.Errorf("plan-batchseq: sequence limit must be positive")
	}
	if maxTokens <= 0 {
		return batchSeqPlan{}, fmt.Errorf("plan-batchseq: token limit must be positive")
	}

	plan := batchSeqPlan{
		entries: make([]batchSeqEntry, 0, min(maxSequences, len(items)-start)),
		next:    start,
	}

	for itemOffset := start; itemOffset < len(items) && len(plan.entries) < maxSequences; itemOffset++ {
		item := items[itemOffset]
		if len(item.tokens) == 0 {
			return batchSeqPlan{}, fmt.Errorf("plan-batchseq: item[%d] has no tokens", item.index)
		}
		if len(item.tokens) > maxTokens {
			return batchSeqPlan{}, fmt.Errorf("plan-batchseq: item[%d] has %d tokens but limit is %d", item.index, len(item.tokens), maxTokens)
		}
		if plan.nTokens+len(item.tokens) > maxTokens {
			break
		}

		plan.entries = append(plan.entries, batchSeqEntry{
			itemOffset: itemOffset,
			itemIndex:  item.index,
			seqID:      llama.SeqId(len(plan.entries)),
			tokens:     item.tokens,
		})
		plan.nTokens += len(item.tokens)
		plan.next = itemOffset + 1
	}

	return plan, nil
}

type batchSeqScheduledEntry struct {
	job        *batchSeqJob
	itemOffset int
	item       batchSeqItem
}

type batchSeqJobFailure struct {
	job *batchSeqJob
	err error
}

type batchSeqSchedule struct {
	entries     []batchSeqScheduledEntry
	done        []*batchSeqJob
	failed      []batchSeqJobFailure
	nTokens     int
	outputWidth int
}

// scheduleBatchSeq selects complete items from active requests in round-robin
// order. Jobs that still have work return in the order they should be
// considered for the next native batch.
func scheduleBatchSeq(jobs []*batchSeqJob, maxSequences, maxTokens int) (batchSeqSchedule, []*batchSeqJob, error) {
	if maxSequences <= 0 {
		return batchSeqSchedule{}, nil, fmt.Errorf("schedule-batchseq: sequence limit must be positive")
	}
	if maxTokens <= 0 {
		return batchSeqSchedule{}, nil, fmt.Errorf("schedule-batchseq: token limit must be positive")
	}
	if len(jobs) == 0 {
		return batchSeqSchedule{}, nil, nil
	}

	schedule := batchSeqSchedule{
		entries:     make([]batchSeqScheduledEntry, 0, maxSequences),
		outputWidth: jobs[0].outputWidth,
	}
	pending := slices.Clone(jobs)
	deferred := 0

	for len(pending) > 0 && len(schedule.entries) < maxSequences {
		job := pending[0]
		pending = pending[1:]

		if job.next >= len(job.items) {
			schedule.done = append(schedule.done, job)
			deferred = 0
			continue
		}

		if job.outputWidth != schedule.outputWidth {
			pending = append(pending, job)
			deferred++
			if deferred >= len(pending) {
				break
			}
			continue
		}

		item := job.items[job.next]
		switch {
		case len(item.tokens) == 0:
			schedule.failed = append(schedule.failed, batchSeqJobFailure{
				job: job,
				err: fmt.Errorf("schedule-batchseq: item[%d] has no tokens", item.index),
			})
			deferred = 0
			continue

		case len(item.tokens) > maxTokens:
			schedule.failed = append(schedule.failed, batchSeqJobFailure{
				job: job,
				err: fmt.Errorf("schedule-batchseq: item[%d] has %d tokens but limit is %d", item.index, len(item.tokens), maxTokens),
			})
			deferred = 0
			continue

		case schedule.nTokens+len(item.tokens) > maxTokens:
			pending = append(pending, job)
			deferred++
			if deferred >= len(pending) {
				return schedule, pending, nil
			}
			continue
		}

		schedule.entries = append(schedule.entries, batchSeqScheduledEntry{
			job:        job,
			itemOffset: job.next,
			item:       item,
		})
		schedule.nTokens += len(item.tokens)
		job.next++
		deferred = 0

		if job.next == len(job.items) {
			schedule.done = append(schedule.done, job)
		} else {
			pending = append(pending, job)
		}
	}

	return schedule, pending, nil
}
