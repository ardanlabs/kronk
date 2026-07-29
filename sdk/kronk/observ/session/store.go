package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/dgraph-io/badger/v4"
)

var completedPrefix = []byte("completed/")

type storedSummary struct {
	key     []byte
	summary Summary
}

type store struct {
	mu           sync.Mutex
	db           *badger.DB
	maxCompleted int
	count        int
	closed       bool
}

func newStore(path string, maxCompleted int) (*store, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("new store: open database: %w", err)
	}

	s := store{
		db:           db,
		maxCompleted: maxCompleted,
	}

	records, err := s.load(context.Background())
	if err != nil {
		db.Close()
		return nil, err
	}
	s.count = len(records)

	if s.count > s.maxCompleted {
		if err := s.deleteOldest(s.count - s.maxCompleted); err != nil {
			db.Close()
			return nil, err
		}
	}

	return &s, nil
}

func (s *store) save(ctx context.Context, summary Summary) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("save completed summary: encode: %w", err)
	}

	var oldest [][]byte
	if s.count+1 > s.maxCompleted {
		pruneCount := max(s.maxCompleted/10, 1)
		records, err := s.load(ctx)
		if err != nil {
			return err
		}
		sortOldest(records)
		pruneCount = min(pruneCount, len(records))
		oldest = make([][]byte, pruneCount)
		for i := range pruneCount {
			oldest[i] = records[i].key
		}
	}

	key := completedKey(summary.ModelID, summary.SessionID)
	if err := s.db.Update(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if err == nil {
			return ErrAlreadyCompleted
		}
		if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		for _, oldKey := range oldest {
			if err := txn.Delete(oldKey); err != nil {
				return err
			}
		}
		return txn.Set(key, data)
	}); err != nil {
		return fmt.Errorf("save completed summary: write: %w", err)
	}

	s.count += 1 - len(oldest)
	return nil
}

func (s *store) list(ctx context.Context, query Query) (Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return Page{}, ErrClosed
	}

	records, err := s.load(ctx)
	if err != nil {
		return Page{}, err
	}

	summaries := make([]Summary, 0, len(records))
	for _, record := range records {
		if matches(record.summary, query) {
			summaries = append(summaries, record.summary)
		}
	}
	sortNewest(summaries)

	limit := normalizeLimit(query.Limit)
	start := min(max(query.Offset, 0), len(summaries))
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

func (s *store) summaries(ctx context.Context) ([]Summary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrClosed
	}

	records, err := s.load(ctx)
	if err != nil {
		return nil, err
	}

	summaries := make([]Summary, len(records))
	for i, record := range records {
		summaries[i] = record.summary
	}

	return summaries, nil
}

func (s *store) load(ctx context.Context) ([]storedSummary, error) {
	records := make([]storedSummary, 0, s.count)
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = completedPrefix
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}

			item := it.Item()
			var summary Summary
			if err := item.Value(func(value []byte) error {
				return json.Unmarshal(value, &summary)
			}); err != nil {
				return fmt.Errorf("load completed summaries: decode: %w", err)
			}

			records = append(records, storedSummary{
				key:     item.KeyCopy(nil),
				summary: summary,
			})
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load completed summaries: %w", err)
	}

	return records, nil
}

func (s *store) deleteOldest(count int) error {
	records, err := s.load(context.Background())
	if err != nil {
		return err
	}
	sortOldest(records)
	count = min(count, len(records))

	batch := s.db.NewWriteBatch()
	defer batch.Cancel()
	for i := range count {
		if err := batch.Delete(records[i].key); err != nil {
			return fmt.Errorf("delete oldest completed summaries: %w", err)
		}
	}
	if err := batch.Flush(); err != nil {
		return fmt.Errorf("delete oldest completed summaries: %w", err)
	}

	s.count -= count
	return nil
}

func (s *store) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close session store: %w", err)
	}

	return nil
}

func (s *store) countValue() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

func completedKey(modelID string, sessionID string) []byte {
	key, _ := json.Marshal(Key{ModelID: modelID, SessionID: sessionID})
	return append(append([]byte(nil), completedPrefix...), key...)
}

func sortOldest(records []storedSummary) {
	slices.SortFunc(records, func(a, b storedSummary) int {
		return compareSummaries(a.summary, b.summary)
	})
}

func sortNewest(summaries []Summary) {
	slices.SortFunc(summaries, func(a, b Summary) int {
		return -compareSummaries(a, b)
	})
}

func compareSummaries(a Summary, b Summary) int {
	if cmp := a.LastActiveAt.Compare(b.LastActiveAt); cmp != 0 {
		return cmp
	}
	if a.ModelID < b.ModelID {
		return -1
	}
	if a.ModelID > b.ModelID {
		return 1
	}
	if a.SessionID < b.SessionID {
		return -1
	}
	if a.SessionID > b.SessionID {
		return 1
	}
	return 0
}

func matches(summary Summary, query Query) bool {
	if query.ModelID != "" && summary.ModelID != query.ModelID {
		return false
	}
	if query.MinUtilization > 0 && summary.Utilization() < query.MinUtilization {
		return false
	}

	return true
}
