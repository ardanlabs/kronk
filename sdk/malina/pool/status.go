package pool

import "fmt"

// ModelStatus returns loaded and in-flight Malina models.
func (p *Pool) ModelStatus() ([]ModelDetail, error) {
	usage := p.resman.Usage()
	reservedByKey := make(map[string]int64, len(usage.Reservations))
	for _, reservation := range usage.Reservations {
		reservedByKey[reservation.Key] = reservation.VRAMBytes + reservation.RAMBytes
	}

	details := make([]ModelDetail, 0)
	loaded := make(map[string]struct{})

	for entry := range p.engine.Coldest() {
		handle := entry.Value
		display := p.loader.Display(handle, entry.Key)
		size, err := p.modelSize(entry.Key)
		if err != nil {
			return nil, fmt.Errorf("model-status: %w", err)
		}

		details = append(details, ModelDetail{
			ID:                entry.Key,
			Backend:           "malina",
			ModelFamily:       entry.Key,
			Size:              size,
			VRAMTotal:         reservedByKey[entry.Key],
			Slots:             display.Slots,
			ExpiresAt:         p.engine.EntryExpiresAt(entry),
			ActiveGenerations: handle.ActiveGenerations(),
			Status:            ModelStatusLoaded,
		})
		loaded[entry.Key] = struct{}{}
	}

	for _, reservation := range usage.Reservations {
		if _, exists := loaded[reservation.Key]; exists || !p.engine.HasTicket(reservation.Key) {
			continue
		}

		size, err := p.modelSize(reservation.Key)
		if err != nil {
			return nil, fmt.Errorf("model-status: %w", err)
		}

		details = append(details, ModelDetail{
			ID:          reservation.Key,
			Backend:     "malina",
			ModelFamily: reservation.Key,
			Size:        size,
			VRAMTotal:   reservation.VRAMBytes + reservation.RAMBytes,
			Status:      ModelStatusLoading,
		})
	}

	return details, nil
}

func (p *Pool) modelSize(modelID string) (int64, error) {
	size, err := p.loader.modelSize(modelID)
	if err != nil {
		return 0, fmt.Errorf("model size: %w", err)
	}

	return size, nil
}
