package session

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// Config provides the settings required to construct a Tracker.
type Config struct {
	StorePath    string
	MaxCompleted int
}

type liveSession struct {
	currentRequestID string
	summary          Summary
}

// Tracker manages live session summaries and persisted completed summaries.
type Tracker struct {
	mu       sync.RWMutex
	live     map[Key]*liveSession
	store    *store
	closed   bool
	closeErr error
}

// New constructs a session tracker and opens its completed-summary store.
func New(cfg Config) (*Tracker, error) {
	if cfg.StorePath == "" {
		return nil, errors.New("new session tracker: store path is required")
	}
	if cfg.MaxCompleted == 0 {
		cfg.MaxCompleted = DefaultMaxCompleted
	}
	if cfg.MaxCompleted < 1 {
		return nil, errors.New("new session tracker: max completed must be greater than zero")
	}

	store, err := newStore(cfg.StorePath, cfg.MaxCompleted)
	if err != nil {
		return nil, fmt.Errorf("new session tracker: %w", err)
	}

	return &Tracker{
		live:  make(map[Key]*liveSession),
		store: store,
	}, nil
}

// RequestStarted marks a session Active and records the request's initial
// prompt occupancy.
func (t *Tracker) RequestStarted(event RequestStart) error {
	if err := validateKey(event.Key); err != nil {
		return fmt.Errorf("request started: %w", err)
	}
	if event.RequestID == "" {
		return errors.New("request started: request ID is required")
	}

	now := eventTime(event.StartedAt)
	promptTokens := max(event.PromptTokens, 0)

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrClosed
	}

	live, exists := t.live[event.Key]
	if !exists {
		live = &liveSession{
			summary: Summary{
				ModelID:       event.Key.ModelID,
				SessionID:     event.Key.SessionID,
				StartedAt:     now,
				LastActiveAt:  now,
				ContextWindow: event.ContextWindow,
			},
		}
		t.live[event.Key] = live
	}

	if live.summary.State == StateActive {
		if live.currentRequestID == event.RequestID {
			return nil
		}
		return ErrRequestActive
	}

	live.currentRequestID = event.RequestID
	live.summary.State = StateActive
	live.summary.LastActiveAt = now
	live.summary.RequestCount++
	live.summary.CurrentContext = promptTokens
	live.summary.ContextWindow = event.ContextWindow
	live.summary.PeakPrompt = max(live.summary.PeakPrompt, promptTokens)
	live.summary.PeakContext = max(live.summary.PeakContext, promptTokens)

	return nil
}

// RequestCompleted applies final token accounting and moves a reusable session
// to Idle. A non-reusable request is finalized and persisted immediately.
func (t *Tracker) RequestCompleted(ctx context.Context, event RequestCompletion) error {
	if err := validateKey(event.Key); err != nil {
		return fmt.Errorf("request completed: %w", err)
	}
	if event.RequestID == "" {
		return errors.New("request completed: request ID is required")
	}

	now := eventTime(event.CompletedAt)

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrClosed
	}

	live, exists := t.live[event.Key]
	if !exists {
		return ErrNotFound
	}
	if live.summary.State != StateActive || live.currentRequestID != event.RequestID {
		return ErrRequestMismatch
	}
	previous := *live

	promptTokens := max(event.PromptTokens, 0)
	cachedTokens := min(max(event.CachedTokens, 0), promptTokens)
	outputTokens := max(event.OutputTokens, 0)
	requestContext := promptTokens + outputTokens

	live.currentRequestID = ""
	live.summary.LastActiveAt = now
	live.summary.CurrentContext = 0
	live.summary.PeakPrompt = max(live.summary.PeakPrompt, promptTokens)
	live.summary.PeakOutput = max(live.summary.PeakOutput, outputTokens)
	live.summary.PeakContext = max(live.summary.PeakContext, requestContext)
	live.summary.CachedTokens = cachedTokens
	live.summary.TotalCachedTokens += int64(cachedTokens)
	live.summary.TotalProcessedTokens += int64(promptTokens - cachedTokens)
	live.summary.ContextFull = live.summary.ContextFull || event.ContextFull

	if event.Reusable {
		live.summary.State = StateIdle
		return nil
	}

	summary := prepareCompletion(live, now)
	if err := t.store.save(ctx, summary); err != nil {
		*live = previous
		return err
	}

	delete(t.live, event.Key)
	return nil
}

// SessionCompleted permanently completes and persists a live session.
func (t *Tracker) SessionCompleted(ctx context.Context, key Key) error {
	if err := validateKey(key); err != nil {
		return fmt.Errorf("session completed: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrClosed
	}

	live, exists := t.live[key]
	if !exists {
		return ErrNotFound
	}

	summary := prepareCompletion(live, time.Now().UTC())
	if err := t.store.save(ctx, summary); err != nil {
		return err
	}

	delete(t.live, key)
	return nil
}

func prepareCompletion(live *liveSession, endedAt time.Time) Summary {
	summary := live.summary
	summary.State = StateCompleted
	summary.EndedAt = &endedAt
	summary.Incomplete = live.summary.State == StateActive
	summary.CurrentContext = 0
	return summary
}

// List returns an offset-paginated page for one lifecycle group.
func (t *Tracker) List(ctx context.Context, state State, query Query) (Page, error) {
	switch state {
	case StateCompleted:
		t.mu.RLock()
		defer t.mu.RUnlock()
		if t.closed {
			return Page{}, ErrClosed
		}
		return t.store.list(ctx, query)
	case StateActive, StateIdle:
		return t.listLive(ctx, state, query)
	default:
		return Page{}, fmt.Errorf("list sessions: invalid state %q", state)
	}
}

func (t *Tracker) listLive(ctx context.Context, state State, query Query) (Page, error) {
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}

	limit := normalizeLimit(query.Limit)
	offset := max(query.Offset, 0)

	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		return Page{}, ErrClosed
	}

	summaries := make([]Summary, 0, len(t.live))
	for _, live := range t.live {
		if live.summary.State == state && matches(live.summary, query) {
			summaries = append(summaries, live.summary)
		}
	}
	t.mu.RUnlock()

	slices.SortFunc(summaries, func(a, b Summary) int {
		if cmp := b.LastActiveAt.Compare(a.LastActiveAt); cmp != 0 {
			return cmp
		}
		if a.ModelID < b.ModelID {
			return 1
		}
		if a.ModelID > b.ModelID {
			return -1
		}
		if a.SessionID < b.SessionID {
			return 1
		}
		if a.SessionID > b.SessionID {
			return -1
		}
		return 0
	})

	start := min(offset, len(summaries))
	end := min(start+limit, len(summaries))
	page := Page{
		Sessions:   append([]Summary(nil), summaries[start:end]...),
		NextOffset: end,
		HasMore:    end < len(summaries),
	}
	if !page.HasMore {
		page.NextOffset = 0
	}

	return page, nil
}

// Counts reports the current number of Active, Idle, and Completed summaries.
func (t *Tracker) Counts() Counts {
	t.mu.RLock()
	defer t.mu.RUnlock()

	counts := Counts{Completed: t.store.countValue()}
	for _, live := range t.live {
		switch live.summary.State {
		case StateActive:
			counts.Active++
		case StateIdle:
			counts.Idle++
		}
	}

	return counts
}

// Summary returns context distributions across all Active, Idle, and retained
// Completed sessions.
func (t *Tracker) Summary(ctx context.Context) (Overview, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.closed {
		return Overview{}, ErrClosed
	}

	completed, err := t.store.summaries(ctx)
	if err != nil {
		return Overview{}, err
	}

	contexts := make([]int, 0, len(t.live)+len(completed))
	utilizations := make([]float64, 0, len(t.live)+len(completed))
	summary := Overview{Completed: len(completed)}

	for _, live := range t.live {
		switch live.summary.State {
		case StateActive:
			summary.Active++
		case StateIdle:
			summary.Idle++
		default:
			continue
		}

		contexts = append(contexts, live.summary.PeakContext)
		utilizations = append(utilizations, live.summary.Utilization())
	}
	for _, completedSummary := range completed {
		contexts = append(contexts, completedSummary.PeakContext)
		utilizations = append(utilizations, completedSummary.Utilization())
	}

	summary.Total = len(contexts)
	if summary.Total == 0 {
		return summary, nil
	}

	slices.Sort(contexts)
	slices.Sort(utilizations)
	summary.Context = TokenPercentiles{
		P50: percentile(contexts, 50),
		P90: percentile(contexts, 90),
		P95: percentile(contexts, 95),
		P99: percentile(contexts, 99),
		Max: contexts[len(contexts)-1],
	}
	summary.Utilization = UtilizationPercentiles{
		P50: percentile(utilizations, 50),
		P90: percentile(utilizations, 90),
		P95: percentile(utilizations, 95),
		P99: percentile(utilizations, 99),
		Max: utilizations[len(utilizations)-1],
	}

	return summary, nil
}

func percentile[T int | float64](values []T, percent int) T {
	index := (percent*len(values) + 99) / 100
	return values[max(index-1, 0)]
}

// Shutdown freezes the tracker, persists every Active and Idle session as
// Completed, and closes the completed-summary store.
func (t *Tracker) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return t.closeErr
	}

	now := time.Now().UTC()
	flushCtx := context.WithoutCancel(ctx)
	var errs []error
	for key, live := range t.live {
		summary := prepareCompletion(live, now)
		if err := t.store.save(flushCtx, summary); err != nil {
			errs = append(errs, err)
			continue
		}
		delete(t.live, key)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	t.closeErr = t.store.close()
	t.closed = true
	return t.closeErr
}

func validateKey(key Key) error {
	if key.ModelID == "" {
		return errors.New("model ID is required")
	}
	if key.SessionID == "" {
		return errors.New("session ID is required")
	}

	return nil
}
