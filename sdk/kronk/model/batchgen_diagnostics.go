package model

import "time"

const diagnosticsPublishInterval = 100 * time.Millisecond

// BatchGenerationContribution describes one slot's generation rows in the
// latest logical batch.
type BatchGenerationContribution struct {
	SlotID int
	Rows   int
	Mode   string
}

// BatchSlotSnapshot describes one generation slot at a batch-loop boundary.
type BatchSlotSnapshot struct {
	ID                      int
	Phase                   string
	RequestID               string
	RequestAge              time.Duration
	PrefillOwner            bool
	PromptTokens            int
	PrefilledTokens         int
	PrefillRemaining        int
	GeneratedTokens         int
	PastTokens              int
	GenerationMode          string
	GenerationRows          int
	IMCPreparedTokens       int
	IMCTotalTokens          int
	IMCPreparationRemaining int
}

// BatchEngineSnapshot describes the latest immutable scheduler state published
// by a model's generation batch engine.
type BatchEngineSnapshot struct {
	Iteration               uint64
	PrefillBatchSize        int
	NBatch                  int
	NUBatch                 int
	MTP                     bool
	NDraft                  int
	QueuedRequests          int
	PendingRequests         int
	PrefillSelectorStart    int
	PrefillSelectorSelected int
	PrefillSelectorNext     int
	EligiblePrefillSlots    []int
	IMCSelectorStart        int
	IMCSelectorSelected     int
	IMCSelectorNext         int
	EligibleIMCSlots        []int
	GenerationRows          int
	PrefillRows             int
	TotalRows               int
	GenerationContributions []BatchGenerationContribution
	Slots                   []BatchSlotSnapshot
}

func (e *batchEngine) publishDiagnostics(force bool) {
	now := time.Now()
	if !force && now.Sub(e.diagnosticLastPublished) < diagnosticsPublishInterval {
		return
	}

	snapshot := BatchEngineSnapshot{
		Iteration:               e.batchIteration,
		PrefillBatchSize:        e.model.cfg.PrefillBatchSize(),
		NBatch:                  e.model.cfg.EffectiveNBatch(),
		NUBatch:                 e.model.cfg.EffectiveNUBatch(),
		QueuedRequests:          len(e.requestQ),
		PendingRequests:         len(e.pendingJobs),
		PrefillSelectorStart:    e.diagnosticPrefillStart,
		PrefillSelectorSelected: e.diagnosticPrefillSelected,
		PrefillSelectorNext:     e.prefillNext,
		EligiblePrefillSlots:    e.prefillSlotIDs(),
		IMCSelectorStart:        e.diagnosticIMCStart,
		IMCSelectorSelected:     e.diagnosticIMCSelected,
		IMCSelectorNext:         e.imcPrepNext,
		EligibleIMCSlots:        e.imcPreparationSlotIDs(),
		GenerationRows:          e.diagnosticGenerationRows,
		TotalRows:               int(e.batch.NTokens),
		GenerationContributions: append([]BatchGenerationContribution(nil), e.diagnosticGeneration...),
		Slots:                   make([]BatchSlotSnapshot, len(e.slots)),
	}
	snapshot.PrefillRows = max(snapshot.TotalRows-snapshot.GenerationRows, 0)

	if e.model.draft != nil {
		snapshot.MTP = e.model.draft.mtp()
		snapshot.NDraft = e.model.draft.core().nDraft
	}

	for i, s := range e.slots {
		snapshot.Slots[i] = e.slotSnapshot(now, s)
	}

	e.diagnosticLastPublished = now
	e.diagnostics.Store(&snapshot)
}

func (e *batchEngine) slotSnapshot(now time.Time, s *slot) BatchSlotSnapshot {
	snapshot := BatchSlotSnapshot{
		ID:              s.id,
		Phase:           "idle",
		PrefillOwner:    s.id == e.diagnosticPrefillSelected,
		PromptTokens:    s.nPrompt,
		PrefilledTokens: s.nPrefilled,
		GeneratedTokens: s.reasonTokens + s.completionTokens,
		PastTokens:      int(s.nPast),
		GenerationMode:  generationMode(e, s),
	}

	for _, contribution := range e.diagnosticGeneration {
		if contribution.SlotID == s.id {
			snapshot.GenerationRows = contribution.Rows
			break
		}
	}

	if !s.active {
		return snapshot
	}

	snapshot.RequestID = s.job.id
	if !s.job.requestStart.IsZero() {
		snapshot.RequestAge = now.Sub(s.job.requestStart)
	}

	switch {
	case s.imcRestoring:
		snapshot.Phase = "imc-restore"

	case s.imcPrep != nil:
		snapshot.Phase = "prefill-imc"
		snapshot.IMCPreparedTokens = s.imcPrep.nextToken
		snapshot.IMCTotalTokens = len(s.job.imcNewCacheTokens)
		snapshot.IMCPreparationRemaining = max(snapshot.IMCTotalTokens-snapshot.IMCPreparedTokens, 0)

	case s.prefillTokens != nil:
		snapshot.Phase = "prefill"
		snapshot.PrefillRemaining = max(len(s.prefillTokens)-s.nPrefilled, 0)

	case s.inputChunks != 0:
		snapshot.Phase = "media-prefill"

	case s.prefillDone:
		snapshot.Phase = "generation"

	default:
		snapshot.Phase = "starting"
	}

	return snapshot
}

func generationMode(e *batchEngine, s *slot) string {
	switch {
	case !s.active || !s.prefillDone:
		return ""
	case s.useMRoPE:
		return "mrope"
	case e.model.draft == nil:
		return "ordinary"
	case e.model.draft.mtp() && s.mtpDisabledForRequest:
		return "ordinary-mtp-disabled"
	case e.model.draft.mtp():
		return "mtp"
	default:
		return "speculative"
	}
}

// BatchEngineSnapshot returns the latest generation scheduler snapshot. Models
// used only for embeddings or reranking do not have a generation batch engine.
func (m *Model) BatchEngineSnapshot() (BatchEngineSnapshot, bool) {
	if m.batch == nil {
		return BatchEngineSnapshot{}, false
	}

	snapshot := m.batch.diagnostics.Load()
	if snapshot == nil {
		return BatchEngineSnapshot{}, false
	}

	result := *snapshot
	result.EligiblePrefillSlots = append([]int(nil), snapshot.EligiblePrefillSlots...)
	result.EligibleIMCSlots = append([]int(nil), snapshot.EligibleIMCSlots...)
	result.GenerationContributions = append([]BatchGenerationContribution(nil), snapshot.GenerationContributions...)
	result.Slots = append([]BatchSlotSnapshot(nil), snapshot.Slots...)
	return result, true
}
