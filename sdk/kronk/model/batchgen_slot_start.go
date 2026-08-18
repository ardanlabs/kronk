package model

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

// startSlot initializes a generation slot with a new request.
func (e *batchEngine) startSlot(s *slot, job *chatJob, buf []byte) {
	if job.imcSystemCache != nil {
		defer job.releaseIMCSystemCache(e.model)
	}

	s.reset()
	s.active = true
	s.job = job
	s.stopGate = newStopGate(job.params.Stop)
	job.imcUsageVersion = e.model.imcBeginRequestUsage(job.imcSession)
	s.reusedPromptTokens = job.reusedPromptTokens
	s.suppressTools = job.d["tool_choice"] == "none"
	if stateMachine, ok := s.stateMachine.(ToolAwareStateMachine); ok {
		tools, _ := job.d["tools"].([]D)
		stateMachine.SetTools(tools)
	}

	// If the rendered prompt ends with a reasoning opener followed by any
	// trailing whitespace, the template has already opened a reasoning block.
	// Prime the parser and slot so generated tokens are correctly classified
	// until the corresponding closer. Setting reasonFlag ensures grammar
	// sampling is skipped during the thinking phase.
	//
	// Templates differ in their opener and trailing whitespace: Kimi uses
	// <|open|>think<|sep|>, while templates using <think> may add whitespace.
	// Qwen emits exactly "<think>\n", Nemotron emits "<think>\n\n", and
	// some custom templates may emit "<think>" with no newline. Accept
	// any of these by trimming trailing ASCII whitespace before checking.
	//
	// Skip reasoning mode when grammar is specified — grammar constrains
	// the output format, so free-form thinking is counterproductive and
	// would consume max_tokens before producing any constrained content.
	trimmedPrompt := strings.TrimRight(job.prompt, " \t\r\n")
	if (strings.HasSuffix(trimmedPrompt, "<think>") ||
		strings.HasSuffix(trimmedPrompt, "<|open|>think<|sep|>")) && job.params.Grammar == "" {
		// Drive the state machine into reasoning mode by feeding the same
		// marker the model would have emitted. Parsers that recognize
		// <think> (fallback, qwen, mistral, glm) flip to ChannelReasoning;
		// Kimi recognizes its <|open|>think<|sep|> marker. Parsers that don't
		// (gemma, gpt) treat the marker as content — but
		// those parsers do not produce a "<think>\n" suffix in the
		// prompt, so this branch never runs for them.
		marker := "<think>"
		if strings.HasSuffix(trimmedPrompt, "<|open|>think<|sep|>") {
			marker = "<|open|>think<|sep|>"
		}
		s.stateMachine.Classify(marker)
		s.reasonFlag = 1
	}

	// End the queue-wait span now that the job has been picked up.
	var queueWait time.Duration
	if job.queueWaitSpan != nil {
		job.queueWaitSpan.End()
	}
	if !job.queuedAt.IsZero() {
		queueWait = time.Since(job.queuedAt)
		metrics.ObserveChatQueueWait(e.model.modelInfo.ID, queueWait)
	}
	e.model.log(job.ctx, "request-lifecycle",
		"stage", 3,
		"stage_name", "schedule-job",
		"status", "complete",
		"id", job.id,
		"slot", s.id,
		"queue_wait", queueWait.String(),
	)

	// Start span for this chat request. Store the span context so child
	// spans (prefill, token-generation) are nested under process-request.
	var processCtx context.Context
	processCtx, s.span = otel.AddSpan(job.ctx, "process-request",
		attribute.String("id", job.id),
		attribute.Int("slot", s.id),
	)
	job.ctx = processCtx

	// Start prefill span and record start time for TTFT.
	_, s.prefillSpan = otel.AddSpan(processCtx, "prefill",
		attribute.Int("slot", s.id),
	)
	s.prefillStart = time.Now()

	// Token-v2 planning already computed the complete prompt size. Reject an
	// oversized prompt before restoring or extending its cached KV state.
	if job.imcTokenPlan {
		s.nPrompt = job.imcNewTotalCached + len(job.tailTokens)
		if !e.applyContextTokenBudget(s, "start-slot") {
			return
		}
	}

	// Resolve one concrete master seed for every request. A matched IMC
	// session retains its seed across conversation turns; a new or reused
	// session gets a fresh seed when the caller does not provide one.
	_, seedProvided := job.d["seed"]
	seeds, specRNG, seedSource, err := e.model.resolveRequestSamplingSeeds(job.params.Seed, seedProvided, job.imcSession)
	if err != nil {
		e.finishSlot(s, fmt.Errorf("start-slot: %w", err))
		return
	}
	e.model.log(job.ctx, "request-lifecycle",
		"stage", 4,
		"stage_name", "execute-in-slot",
		"status", "started",
		"id", job.id,
		"slot", s.id,
		"seq", s.seqID,
		"seed", seeds.master,
		"seed_source", seedSource,
	)

	// Create sampler and speculative RNG state for this request.
	s.samplingSeeds = seeds
	s.specRNG = specRNG
	s.sampler = e.model.toSampler(job.ctx, job.params, seeds)

	// Create grammar sampler if grammar is specified (kept separate from chain).
	if job.params.Grammar != "" {
		s.grammarSampler, err = newGrammarSampler(e.model.vocab, job.params.Grammar)
		if err != nil {
			e.finishSlot(s, fmt.Errorf("start-slot: %w", err))
			return
		}
	}

	// Create a fresh per-request mtmd context for any request that touches
	// the mtmd pipeline (media-bearing requests, or IMC media cache builds
	// for media-using sessions). Per-request lifetime keeps any internal
	// mtmd state (image_tokens, output buffer, bitmap registry, vision
	// support flags) bounded to a single request. The context is freed in
	// freeSlotResources, which finishSlot calls on every exit path.
	needsMTMD := e.model.projFile != "" && (job.imcMediaBuild || job.imcMediaAppend ||
		(job.object == ObjectChatMedia && len(job.media) > 0))
	if needsMTMD {
		mtmdCtx, err := mtmd.InitFromFile(e.model.projFile, e.model.model, mtmdContextParams(e.model.cfg))
		if err != nil {
			// For IMC media builds token-v2 planner already reserved the
			// session with reserved=true. We failed before reaching the
			// commit/publish/reset block in the IMC switch below, so
			// the session would otherwise stay reserved forever and become
			// unavailable. finishSlot releases the held reservation.
			e.finishSlot(s, fmt.Errorf("start-slot: init per-request mtmd context: %w", err))
			return
		}
		s.mtmdCtx = mtmdCtx
	}

	// IMC: restore externalized KV state from session.kvState, then decode
	// any extension tokens. The slot's sequence was cleared in the previous
	// finishSlot, so we always restore from RAM.
	//
	// Read cache state from the MATCHED session (job.imcSession). The IMC
	// session pool is at least as large as generation admission capacity, so
	// there is no
	// fixed slot-to-session correspondence: the session is bound to this
	// slot's KV sequence here and unbound in finishSlot.
	var cacheIdx llama.Pos
	var sessionUpdateDisabled bool

	// sessionWasCommitted records whether we called imcCommitSession in
	// the cache-build / cache-extend branches below. When true, the
	// session's metadata is up to date but its reserved flag is still set;
	// the snapshot block must finalize publication (imcPublishSession on
	// snapshot success, or imcResetSession on snapshot failure) so a
	// concurrent token-v2 planner scanner never sees the new metadata against
	// stale/empty kvState bytes.
	var sessionWasCommitted bool
	var restoredPhysicalKVCells int

	switch {
	case e.model.cfg.IncrementalCache() && job.imcCacheHit:
		// Snapshot session state under lock. With externalized KV, the
		// session's kvState slice header may be reset/regrown by another
		// goroutine's eviction, so we must copy the slice header atomically.
		//
		// For pure-hit snapshot-skip candidates we additionally validate
		// that the live session still matches the version token-v2 planner
		// observed. A concurrent extend/rebuild between token-v2 planner and
		// startSlot would change cachedMsgsHash / cachedMsgCount /
		// totalTokensCached / cachedRenderInputHash; restoring that newer
		// state against a suffix prompt rendered for the older boundary
		// would corrupt the request. On mismatch we fail with a retryable
		// error so the caller re-runs token-v2 planner against the new boundary.
		var kvState []byte
		if job.imcSession != nil {
			session := job.imcSession

			e.model.cacheMu.Lock()
			var sessionVersionOK bool
			switch {
			case job.imcSystemCache != nil:
				cacheIdx = llama.Pos(len(job.imcSystemCache.cachedTokens))
				restoredPhysicalKVCells = len(job.imcSystemCache.cachedTokens)
				kvState = job.imcSystemCache.kvState.Bytes()
				sessionVersionOK = session.reserved && len(kvState) > 0
			default:
				cacheIdx = llama.Pos(session.logicalPosition())
				restoredPhysicalKVCells = session.totalTokensCached
				kvState = session.kvState.Bytes()
				sessionVersionOK = !(job.imcReadOnlyReservation || job.imcMediaAnchorAdvance) ||
					(session.cachedMsgsHash == job.imcExpectedHash &&
						session.cachedMsgCount == job.imcExpectedCachedMsgs &&
						session.totalTokensCached == job.imcExpectedTokens &&
						session.logicalPosition() == job.imcExpectedPosition &&
						(!job.imcReadOnlyReservation ||
							(job.imcExpectedRenderHash != "" && session.cachedRenderInputHash == job.imcExpectedRenderHash)) &&
						(!session.hasMedia || session.promptPlan.equal(job.imcExpectedPromptPlan)) &&
						session.reserved &&
						len(kvState) > 0)
			}

			// Bind the session to this slot's KV sequence id. Sessions
			// have no static "home" seq with the multi-session pool:
			// the seqID is set here and reset to imcSeqIDUnbound in
			// finishSlot after the slot's seq is cleared. This keeps
			// the defensive KV-pressure eviction path (which calls
			// MemorySeqRm with the session's seqID) consistent with
			// the slot the session is currently resident on.
			if sessionVersionOK {
				session.seqID = s.seqID
			}
			e.model.cacheMu.Unlock()

			if !sessionVersionOK {
				metrics.AddIMCPureHitStaleSession(e.model.modelInfo.ID)
				e.model.log(job.ctx, "start-slot", "status", "imc-pure-hit-stale",
					"slot", s.id, "imc_cache_entry", job.imcSessionID,
					"expected_msgs", job.imcExpectedCachedMsgs,
					"expected_tokens", job.imcExpectedTokens)

				e.finishSlot(s, fmt.Errorf("start-slot: imc pure hit stale, server busy processing other requests, try again shortly"))
				return
			}
		}

		// Restore externalized KV state from session.kvState into this
		// slot's sequence via StateSeqSetData. The slot's
		// sequence was cleared in finishSlot, so we must restore before
		// decoding extension tokens or processing the suffix.
		if cacheIdx > 0 && len(kvState) == 0 && !job.imcClearSeq {
			e.model.imcInvalidateReservedSession(job.imcSession)
			e.finishSlot(s, fmt.Errorf("start-slot: imc externalized state is empty for seq %d", s.seqID))
			return
		}

		if len(kvState) > 0 && !job.imcClearSeq {
			e.model.log(job.ctx, "start-slot", "status", "imc-restore-start",
				"slot", s.id, "seq", s.seqID, "next_logical_position", cacheIdx,
				"physical_kv_cells", restoredPhysicalKVCells,
				"ram_bytes", fmtBytes(uint64(len(kvState))))

			s.imcRestoring = true
			e.publishDiagnostics(true)
			restoreStart := time.Now()

			e.model.decodeMu.Lock()
			nRead := llama.StateSeqSetData(e.model.lctx, kvState, s.seqID)
			e.model.decodeMu.Unlock()

			expectedBytes := uint64(len(kvState))
			if nRead != expectedBytes {
				e.model.decodeMu.Lock()
				llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
				e.model.decodeMu.Unlock()
				e.model.imcInvalidateReservedSession(job.imcSession)
				e.finishSlot(s, fmt.Errorf("start-slot: imc restore for seq %d read %d bytes, expected %d", s.seqID, nRead, expectedBytes))
				return
			}
			job.imcSnapshotReused = true

			e.model.log(job.ctx, "start-slot", "status", "imc-restore-done",
				"slot", s.id, "seq", s.seqID, "next_logical_position", cacheIdx,
				"physical_kv_cells", restoredPhysicalKVCells,
				"restored_bytes", fmtBytes(nRead), "elapsed", fmtDur(time.Since(restoreStart)))

			// MTP: restore the draft seq state and pendingH alongside
			// the target. The previous request that built the cache
			// also snapshotted the draft seq KV (see imc-draft-
			// snapshot-done above), so we restore both seqs in
			// lock-step here. With the draft seq populated and
			// pendingH carrying the last cached position's pre-norm
			// row, MTP can keep drafting from the very first round
			// instead of being disabled for the whole request.
			//
			// Snapshot the session state under cacheMu (parallel to
			// the target's kvState read above) so we don't race with
			// a concurrent evictor / writer mutating draftKVState's
			// slice header.
			if ext, ok := e.model.draft.(draftKVExternalizer); ok && job.imcSession != nil {
				draft := ext.core()

				var draftBytes []byte
				var savedPendingH []float32
				e.model.cacheMu.RLock()
				nEmbd := draft.mtp.EmbeddingSize()
				switch {
				case job.imcSystemCache != nil:
					if job.imcSystemCache.draftKVState != nil {
						draftBytes = job.imcSystemCache.draftKVState.Bytes()
					}
					if len(job.imcSystemCache.pendingH) == nEmbd {
						savedPendingH = append(savedPendingH, job.imcSystemCache.pendingH...)
					}
				default:
					if job.imcSession.draftKVState != nil {
						draftBytes = job.imcSession.draftKVState.Bytes()
					}
					if len(job.imcSession.pendingH) == nEmbd {
						savedPendingH = append(savedPendingH, job.imcSession.pendingH...)
					}
				}
				e.model.cacheMu.RUnlock()

				switch {
				case len(draftBytes) > 0:
					draftRestoreStart := time.Now()

					e.model.decodeMu.Lock()
					nDraftRead := llama.StateSeqSetData(ext.draftKVCtx(), draftBytes, s.seqID)
					e.model.decodeMu.Unlock()

					switch {
					case validMTPDraftState(nDraftRead, uint64(len(draftBytes)), savedPendingH, nEmbd):
						// Mirror the slot's draft state to what the
						// snapshot covers so subsequent mirror /
						// generateDraftTokensMTP calls find a
						// consistent draftNPast and pendingH.
						s.draftNPast = cacheIdx
						s.mtp.ResumeSource = "restored-draft-kv"
						if len(savedPendingH) == nEmbd {
							if cap(s.mtp.PendingHidden) < nEmbd {
								s.mtp.PendingHidden = make([]float32, nEmbd)
							} else {
								s.mtp.PendingHidden = s.mtp.PendingHidden[:nEmbd]
							}
							copy(s.mtp.PendingHidden, savedPendingH)
						}
						e.model.log(job.ctx, "start-slot", "status", "imc-draft-restore-done",
							"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
							"restored_bytes", fmtBytes(nDraftRead),
							"pending_h", len(s.mtp.PendingHidden) == nEmbd,
							"elapsed", fmtDur(time.Since(draftRestoreStart)))
					default:
						// Restore failed or did not include the paired
						// target hidden state — drop draft seq + pendingH so
						// startSlotText falls back to the mtp-disabled
						// path. We don't fail the whole request because
						// the target restored fine; only MTP is lost.
						e.model.decodeMu.Lock()
						llama.MemorySeqRm(draft.mem, s.seqID, -1, -1)
						e.model.decodeMu.Unlock()
						s.draftNPast = 0
						s.mtp.Disable("imc-hit")
						e.model.log(job.ctx, "start-slot", "status", "imc-draft-restore-failed",
							"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
							"restored_bytes", fmtBytes(nDraftRead),
							"expected_bytes", fmtBytes(uint64(len(draftBytes))),
							"pending_h", len(savedPendingH) == nEmbd)
					}

				default:
					// No draft snapshot on the session (e.g., the
					// build-time draft snapshot failed). Leave draft
					// seq empty so startSlotText disables MTP for the
					// request via the existing mtp-disabled-imc-hit
					// path.
					s.mtp.Disable("imc-hit")
					e.model.log(job.ctx, "start-slot", "status", "imc-draft-restore-skip-empty",
						"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx)
				}
			}
			s.imcRestoring = false
		}

		// Decode new cache extension tokens into the slot's sequence if any.
		switch {
		case job.imcMediaBuild:
			e.model.log(job.ctx, "start-slot", "status", "imc-media-build", "slot", s.id, "seq", s.seqID)

			e.model.decodeMu.Lock()
			llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
			e.model.decodeMu.Unlock()

			imcDecodeStart := time.Now()

			nextLogicalPos, physicalKVCells, mediaKVCounts, samplerCacheTokens, nativeChunks, err := e.model.decodeMediaIntoCache(job.ctx, job.imcMediaCacheD, s.seqID, s.mtmdCtx)
			if err != nil {
				e.model.decodeMu.Lock()
				llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
				e.model.decodeMu.Unlock()

				e.finishSlot(s, fmt.Errorf("start-slot: imc media build: %w", err))
				return
			}

			metrics.AddPrefillTime(e.model.modelInfo.ID, "imc-decode", time.Since(imcDecodeStart))
			cacheIdx = llama.Pos(nextLogicalPos)
			job.imcPhysicalCached = physicalKVCells

			e.model.imcCommitSession(job.imcSession, job.imcNewMsgsHash, physicalKVCells, job.imcNewCachedMsgCount, nil, true, mediaKVCounts, job.imcExpectedRenderHash)
			e.model.cacheMu.Lock()
			job.imcSession.promptPlan = job.imcPromptPlan
			job.imcSession.samplerPromptTokens = slices.Clone(samplerCacheTokens)
			job.imcSession.mediaNativeChunks = nativeChunks
			job.imcSession.nextLogicalPos = nextLogicalPos
			e.model.cacheMu.Unlock()
			job.imcMediaSamplerTokens = samplerCacheTokens
			job.samplerPromptTokens = slices.Clone(samplerCacheTokens)
			job.samplerPromptTokens = append(job.samplerPromptTokens, job.tailTokens...)
			sessionWasCommitted = true

			if s.mtmdCtx != 0 && job.imcSession != nil {
				job.imcSessionUseMRoPE = mtmd.DecodeUseMRope(s.mtmdCtx)
				e.model.cacheMu.Lock()
				job.imcSession.useMRoPE = job.imcSessionUseMRoPE
				job.imcSession.useNonCausal = mtmd.DecodeUseNonCausal(s.mtmdCtx, 0)
				e.model.cacheMu.Unlock()
			}

			e.model.log(job.ctx, "start-slot", "status", "imc-media-built", "slot", s.id, "seq", s.seqID,
				"physical_kv_cells", physicalKVCells, "next_logical_position", nextLogicalPos)

		case job.imcMediaAnchorAdvance:
			imcDecodeStart := time.Now()
			oldLogicalPosition := int(cacheIdx)
			var decodeErr error
			switch job.imcMediaAppend {
			case true:
				var addedPhysical int
				var addedMediaKV []int
				var samplerCacheTokens []llama.Token
				prefix := job.imcSession.mediaNativeChunks
				textToMedia := !job.imcSession.hasMedia
				if textToMedia {
					prefix = []imcMediaChunk{{kind: imcMediaChunkText, tokens: slices.Clone(job.imcSession.cachedTokens)}}
				}
				useNonCausal := mtmd.DecodeUseNonCausal(s.mtmdCtx, 0)
				switch {
				case textToMedia && useNonCausal:
					e.model.log(job.ctx, "start-slot", "status", "imc-text-prefix-noncausal-rebuild", "slot", s.id, "seq", s.seqID)
					e.model.decodeMu.Lock()
					llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
					e.model.decodeMu.Unlock()
					job.imcNewLogicalPosition, addedPhysical, addedMediaKV, samplerCacheTokens, job.imcMediaNativeChunks, decodeErr = e.model.decodeMediaIntoCache(
						job.ctx, job.imcMediaCacheD, s.seqID, s.mtmdCtx)
					oldLogicalPosition = 0
				default:
					job.imcNewLogicalPosition, addedPhysical, addedMediaKV, samplerCacheTokens, job.imcMediaNativeChunks, decodeErr = e.model.decodeMediaIntoCacheFromPlan(
						job.ctx, job.imcMediaCacheD, prefix, s.seqID, s.mtmdCtx, oldLogicalPosition)
				}
				if errors.Is(decodeErr, errIMCMediaNativePrefix) {
					e.model.log(job.ctx, "start-slot", "status", "imc-media-native-prefix-rebuild", "slot", s.id, "seq", s.seqID, "err", decodeErr)
					e.model.decodeMu.Lock()
					llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
					e.model.decodeMu.Unlock()
					job.imcNewLogicalPosition, addedPhysical, addedMediaKV, samplerCacheTokens, job.imcMediaNativeChunks, decodeErr = e.model.decodeMediaIntoCache(
						job.ctx, job.imcMediaCacheD, s.seqID, s.mtmdCtx)
					oldLogicalPosition = 0
				}
				if decodeErr == nil {
					job.imcSessionUseMRoPE = mtmd.DecodeUseMRope(s.mtmdCtx)
					job.imcNewTotalCached = addedPhysical
					if oldLogicalPosition > 0 {
						job.imcNewTotalCached += job.imcExpectedTokens
						job.imcMediaKVCounts = append(job.imcMediaKVCounts, addedMediaKV...)
					} else {
						job.imcMediaKVCounts = addedMediaKV
					}
					job.imcMediaSamplerTokens = samplerCacheTokens
					job.samplerPromptTokens = append(slices.Clone(job.imcMediaSamplerTokens), job.tailTokens...)
					if e.model.draft != nil && e.model.draft.mtp() {
						s.mtp.Disable("media-append")
					}
				}
			case false:
				job.imcMediaNativeChunks = job.imcSession.mediaNativeChunks
				advanceTokens := job.imcNewCacheTokens
				switch {
				case job.imcSessionUseMRoPE:
					_, decodeErr = e.model.decodeTextMRoPEIntoCache(advanceTokens, s.seqID, oldLogicalPosition)
				case e.model.draft != nil:
					if _, shared := e.model.draft.(*sharedMTPDrafter); shared {
						decodeErr = e.decodeTokensIntoCacheMTP(job.ctx, s, advanceTokens, oldLogicalPosition)
					} else {
						decodeErr = e.model.decodeTokensIntoCache(job.ctx, advanceTokens, s.seqID, oldLogicalPosition)
					}
				default:
					decodeErr = e.model.decodeTokensIntoCache(job.ctx, advanceTokens, s.seqID, oldLogicalPosition)
				}
			}
			if decodeErr != nil {
				e.finishSlot(s, fmt.Errorf("start-slot: imc media anchor advance: %w", decodeErr))
				return
			}

			metrics.AddPrefillTime(e.model.modelInfo.ID, "imc-decode", time.Since(imcDecodeStart))
			cacheIdx = llama.Pos(job.imcNewLogicalPosition)
			job.imcPhysicalCached = job.imcNewTotalCached

			e.model.log(job.ctx, "start-slot", "status", "imc-media-anchor-advanced-in-slot",
				"slot", s.id, "seq", s.seqID, "replay_text_tokens", len(job.imcNewCacheTokens),
				"media_append", job.imcMediaAppend,
				"physical_kv_cells", job.imcNewTotalCached, "next_logical_position", job.imcNewLogicalPosition)

		case len(job.imcNewCacheTokens) > 0:
			// Detect stale extension: if another request extended this slot
			// between our scan and now, cacheIdx won't match the position
			// these tokens were sliced from. For appends (not rebuilds), the
			// expected start position is
			// imcNewTotalCached - len(imcNewCacheTokens).
			if !job.imcClearSeq {
				expectedStart := llama.Pos(job.imcNewTotalCached - len(job.imcNewCacheTokens))
				if cacheIdx != expectedStart {
					e.model.log(job.ctx, "start-slot", "status", "imc-extend-stale", "slot", s.id, "seq", s.seqID,
						"cache_idx", cacheIdx, "expected_start", expectedStart,
						"new_total_cached", job.imcNewTotalCached)

					e.finishSlot(s, fmt.Errorf("start-slot: imc extend stale (cache moved from %d to %d), retry request", expectedStart, cacheIdx))
					return
				}
			}

			switch {
			case job.imcClearSeq:
				// Rebuilding from scratch (prefix mismatch). Clear the old
				// sequence first so we don't append on top of stale tokens.
				e.model.log(job.ctx, "start-slot", "status", "imc-clear-seq", "slot", s.id, "seq", s.seqID,
					"old_cached_tokens", cacheIdx)

				e.model.decodeMu.Lock()
				_, targetClearErr := llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
				if targetClearErr == nil {
					var maxPos llama.Pos
					maxPos, targetClearErr = llama.MemorySeqPosMax(e.model.mem, s.seqID)
					if targetClearErr == nil && maxPos >= 0 {
						targetClearErr = fmt.Errorf("sequence still contains position %d after clear", maxPos)
					}
				}
				// MTP: the prior imc-restore (or a previous decode in
				// this slot) populated the draft seq KV at positions
				// [0..N). The forthcoming decodeTokensIntoCacheMTP
				// rewrites draft positions starting at 0; without
				// clearing the draft seq first, the mirror decode
				// collides with the surviving positions and fails
				// with "the input could not be processed".
				var draftClearErr error
				clear(s.mtp.PendingHidden[:cap(s.mtp.PendingHidden)])
				s.mtp.PendingHidden = s.mtp.PendingHidden[:0]
				if e.model.draft != nil && e.model.draft.mtp() {
					_, draftClearErr = llama.MemorySeqRm(e.model.draft.core().mem, s.seqID, -1, -1)
					if draftClearErr == nil {
						var maxPos llama.Pos
						maxPos, draftClearErr = llama.MemorySeqPosMax(e.model.draft.core().mem, s.seqID)
						if draftClearErr == nil && maxPos >= 0 {
							draftClearErr = fmt.Errorf("sequence still contains position %d after clear", maxPos)
						}
					}
					s.draftNPast = 0
				}
				e.model.decodeMu.Unlock()
				if targetClearErr != nil {
					e.finishSlot(s, fmt.Errorf("start-slot: clear reused target sequence: %w", targetClearErr))
					return
				}
				if draftClearErr != nil {
					e.finishSlot(s, fmt.Errorf("start-slot: clear reused draft sequence: %w", draftClearErr))
					return
				}

				cacheIdx = 0

				e.model.log(job.ctx, "start-slot", "status", "imc-build", "slot", s.id, "seq", s.seqID,
					"tokens", len(job.imcNewCacheTokens))

			default:
				e.model.log(job.ctx, "start-slot", "status", "imc-extend", "slot", s.id, "seq", s.seqID,
					"cached_tokens", cacheIdx, "new_cache_tokens", len(job.imcNewCacheTokens))
			}

			// Large text IMC builds and extensions are prepared incrementally by
			// processBatch. Returning here lets fillSlots bind the remaining jobs
			// before any one request monopolizes the model context.
			s.imcPrep = &imcPreparation{
				cacheIdx:              cacheIdx,
				position:              int(cacheIdx),
				sessionUpdateDisabled: sessionUpdateDisabled,
			}
			e.model.log(job.ctx, "start-slot", "status", "imc-preparation-queued",
				"slot", s.id, "seq", s.seqID, "tokens", len(job.imcNewCacheTokens),
				"start_position", cacheIdx, "chunk_tokens", e.imcPreparationChunkSize())
			return

		case cacheIdx > 0:
			e.model.log(job.ctx, "start-slot", "status", "imc-reuse", "slot", s.id, "seq", s.seqID,
				"cached_tokens", cacheIdx)
		}

	default:
		// Non-IMC mode: clear the slot's sequence. Held under decodeMu to
		// serialize with the batch engine's llama.Decode and IMC decode
		// paths, matching the lock discipline used elsewhere in this file
		// for target-context KV mutations.
		e.model.decodeMu.Lock()
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
		e.model.decodeMu.Unlock()
	}

	e.finishStartSlot(s, job, cacheIdx, sessionUpdateDisabled, sessionWasCommitted, buf)
}

// finishStartSlot snapshots and publishes any prepared IMC prefix, then stages
// the request's ordinary suffix prefill. Text IMC preparation calls this only
// after its final resumable chunk has completed.
func (e *batchEngine) finishStartSlot(s *slot, job *chatJob, cacheIdx llama.Pos, sessionUpdateDisabled, sessionWasCommitted bool, buf []byte) {

	s.nPast = cacheIdx

	// Snapshot the cached prefix KV state into session.kvState. This
	// externalized state is used to restore the cache into any available slot
	// on the next request. The snapshot is taken AFTER cache build/extend but
	// BEFORE suffix tokens are decoded, capturing exactly the cached
	// conversation prefix.
	//
	// StateSeqGetData captures raw KV bytes regardless of whether they were
	// produced by text tokens or media embeddings (image/audio). For Hybrid
	// models it also captures recurrent state (DeltaNet/SSM).
	if e.model.cfg.IncrementalCache() && job.imcCacheHit && !sessionUpdateDisabled && cacheIdx > 0 && job.imcSession != nil {
		var snapshotPhysicalKVCells int
		switch {
		case job.imcMediaAnchorAdvance:
			snapshotPhysicalKVCells = job.imcNewTotalCached
		default:
			e.model.cacheMu.RLock()
			snapshotPhysicalKVCells = job.imcSession.totalTokensCached
			e.model.cacheMu.RUnlock()
		}

		// Pure-hit snapshot skip: when token-v2 planner marked this job as an
		// exact pure hit AND no cached-prefix mutation happened in this
		// startSlot (no extension tokens, media build, or clear), the
		// session's externalized kvState already contains
		// the bytes we just restored from. Re-snapshotting would be a
		// byte-for-byte round trip. We re-validate the live session
		// version under cacheMu (a concurrent extend could have moved it
		// forward between the restore above and here) and confirm MTP
		// draft state was restored successfully before skipping.
		//
		// llama_state_seq_get_data is a host-side serializer (writes only
		// to its dst buffer); skipping it cannot leave KV state in a bad
		// shape. See yzma pkg/llama/state.go for the FFI contract.
		skipSnapshot := false
		if job.imcReadOnlyReservation &&
			len(job.imcNewCacheTokens) == 0 &&
			!job.imcMediaBuild &&
			len(job.imcMediaKVCounts) == 0 &&
			!job.imcClearSeq &&
			cacheIdx == llama.Pos(job.imcExpectedPosition) {

			e.model.cacheMu.RLock()
			session := job.imcSession
			versionOK := session != nil &&
				session.cachedMsgsHash == job.imcExpectedHash &&
				session.cachedMsgCount == job.imcExpectedCachedMsgs &&
				session.totalTokensCached == job.imcExpectedTokens &&
				session.logicalPosition() == job.imcExpectedPosition &&
				job.imcExpectedRenderHash != "" &&
				session.cachedRenderInputHash == job.imcExpectedRenderHash &&
				(!session.hasMedia || session.promptPlan.equal(job.imcExpectedPromptPlan)) &&
				session.kvState.Len() > 0 &&
				session.reserved
			e.model.cacheMu.RUnlock()

			skipSnapshot = versionOK
		}

		if skipSnapshot {
			metrics.AddIMCSnapshotSkipped(e.model.modelInfo.ID)
			e.model.log(job.ctx, "start-slot", "status", "imc-snapshot-skip-read-only", "snapshot_action", "skip-read-only",
				"slot", s.id, "seq", s.seqID, "next_logical_position", cacheIdx,
				"physical_kv_cells", snapshotPhysicalKVCells,
				"imc_cache_entry", job.imcSessionID)

			// Fall through to suffix decode; session.kvState,
			// draftKVState, and pendingH all remain valid because the
			// slot was restored from them and no cached-prefix decode
			// happened afterward. No Synchronize is needed: the
			// restore (StateSeqSetData) completed under decodeMu above
			// and we did not mutate KV state since.

		} else {
			e.model.log(job.ctx, "start-slot", "status", "imc-snapshot-start",
				"slot", s.id, "seq", s.seqID, "next_logical_position", cacheIdx,
				"physical_kv_cells", snapshotPhysicalKVCells)

			snapshotStart := time.Now()

			// Reuse the session's SessionStore in place. Prepare returns a slice
			// of length kvSize, reusing the existing backing array when its
			// capacity is sufficient (the common case after the first turn)
			// and allocating only when the conversation has grown beyond any
			// previous peak. Per-session serialization (the reserved flag and
			// the imcSessions ownership model) guarantees no concurrent reader
			// holds a reference to this buffer while we fill it.
			//
			// capBefore lets us log whether this snapshot grew the backing
			// array (allocation) or reused it (zero allocation) — the central
			// invariant we want to observe in production.
			snapshotStore := job.imcSession.kvState
			if job.imcMediaAnchorAdvance {
				var err error
				snapshotStore, err = newSessionStore(e.model.cfg)
				if err != nil {
					e.finishSlot(s, fmt.Errorf("start-slot: create staged media snapshot: %w", err))
					return
				}
			}
			capBefore := snapshotStore.Cap()

			e.model.decodeMu.Lock()
			llama.Synchronize(e.model.lctx)
			kvSize := llama.StateSeqGetSize(e.model.lctx, s.seqID)
			kvBuf := snapshotStore.Prepare(int(kvSize))
			nExtracted := llama.StateSeqGetData(e.model.lctx, kvBuf, s.seqID)
			e.model.decodeMu.Unlock()

			capAfter := snapshotStore.Cap()
			bufAction := "reuse"
			if capAfter > capBefore {
				bufAction = "grow"
			}

			snapshotOK := kvSize > 0 && nExtracted == kvSize

			// Commit only a complete state transfer. A partial serialized state
			// cannot safely represent the cached logical position, especially for
			// hybrid models where it includes both attention KV and recurrent
			// state. Reset it instead so no future request can restore it.
			e.model.cacheMu.Lock()
			if snapshotOK {
				snapshotStore.Commit(int(nExtracted))
			} else {
				snapshotStore.Reset()
			}
			storedSnapshotBytes := snapshotStore.Len()
			e.model.cacheMu.Unlock()
			snapshotOK = snapshotOK && storedSnapshotBytes == int(kvSize)

			if snapshotOK {
				e.model.log(job.ctx, "start-slot", "status", "imc-snapshot-done",
					"slot", s.id, "seq", s.seqID, "next_logical_position", cacheIdx,
					"physical_kv_cells", snapshotPhysicalKVCells,
					"snapshot_bytes", fmtBytes(nExtracted), "kv_alloc", fmtBytes(kvSize),
					"buf_action", bufAction,
					"buf_cap_before", fmtBytes(uint64(capBefore)),
					"buf_cap_after", fmtBytes(uint64(capAfter)),
					"elapsed", fmtDur(time.Since(snapshotStart)))
			} else {
				e.model.log(job.ctx, "start-slot", "status", "imc-snapshot-failed",
					"slot", s.id, "seq", s.seqID, "next_logical_position", cacheIdx,
					"physical_kv_cells", snapshotPhysicalKVCells,
					"extracted_bytes", fmtBytes(nExtracted), "stored_bytes", fmtBytes(uint64(storedSnapshotBytes)),
					"kv_alloc", fmtBytes(kvSize),
					"buf_action", bufAction,
					"buf_cap_before", fmtBytes(uint64(capBefore)),
					"buf_cap_after", fmtBytes(uint64(capAfter)),
					"elapsed", fmtDur(time.Since(snapshotStart)))
			}

			// MTP: snapshot the draft seq's per-sequence state alongside
			// the target's, so cache hits on later requests can restore
			// both seqs and MTP can keep running through the cached prefix
			// (instead of being disabled via mtp-disabled-imc-hit). Also
			// snapshot the slot's pendingH so the first MTP draft round on
			// the next request can condition on the correct previous-
			// position hidden state. Gated on a successful target snapshot
			// (nExtracted > 0) — without that the cache hit is going to
			// fail anyway.
			if ext, ok := e.model.draft.(draftKVExternalizer); ok && snapshotOK && !job.imcSession.hasMedia && job.imcSession.draftKVState != nil {
				draft := ext.core()
				dctx := ext.draftKVCtx()

				draftCapBefore := job.imcSession.draftKVState.Cap()

				e.model.decodeMu.Lock()
				llama.Synchronize(dctx)
				draftKVSize := llama.StateSeqGetSize(dctx, s.seqID)
				draftBuf := job.imcSession.draftKVState.Prepare(int(draftKVSize))
				nDraftExtracted := llama.StateSeqGetData(dctx, draftBuf, s.seqID)
				e.model.decodeMu.Unlock()

				draftCapAfter := job.imcSession.draftKVState.Cap()
				draftBufAction := "reuse"
				if draftCapAfter > draftCapBefore {
					draftBufAction = "grow"
				}

				nEmbd := draft.mtp.EmbeddingSize()
				draftSnapshotOK := validMTPDraftState(nDraftExtracted, draftKVSize, s.mtp.PendingHidden, nEmbd)

				e.model.cacheMu.Lock()
				// pendingH snapshot: copy the slot's pendingH into the
				// session so a later cache hit can restore it. Lazy-grow
				// the session's pendingH backing slice.
				if draftSnapshotOK {
					job.imcSession.draftKVState.Commit(int(nDraftExtracted))
					if cap(job.imcSession.pendingH) < nEmbd {
						job.imcSession.pendingH = make([]float32, nEmbd)
					} else {
						job.imcSession.pendingH = job.imcSession.pendingH[:nEmbd]
					}
					copy(job.imcSession.pendingH, s.mtp.PendingHidden)
				} else {
					job.imcSession.draftKVState.Reset()
					job.imcSession.pendingH = job.imcSession.pendingH[:0]
				}
				e.model.cacheMu.Unlock()

				switch {
				case draftSnapshotOK:
					e.model.log(job.ctx, "start-slot", "status", "imc-draft-snapshot-done",
						"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
						"snapshot_bytes", fmtBytes(nDraftExtracted),
						"kv_alloc", fmtBytes(draftKVSize),
						"buf_action", draftBufAction,
						"buf_cap_before", fmtBytes(uint64(draftCapBefore)),
						"buf_cap_after", fmtBytes(uint64(draftCapAfter)),
						"pending_h", len(s.mtp.PendingHidden) == nEmbd)
				default:
					e.model.log(job.ctx, "start-slot", "status", "imc-draft-snapshot-failed",
						"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
						"extracted_bytes", fmtBytes(nDraftExtracted),
						"kv_alloc", fmtBytes(draftKVSize),
						"buf_action", draftBufAction,
						"buf_cap_before", fmtBytes(uint64(draftCapBefore)),
						"buf_cap_after", fmtBytes(uint64(draftCapAfter)),
						"pending_h", len(s.mtp.PendingHidden) == nEmbd)
				}
			}

			// Finalize publication for sessions whose metadata was
			// committed above. Sessions still carry reserved=true at
			// this point; publishing makes the new metadata + kvState
			// pair visible to token-v2 planner atomically with respect to
			// concurrent scanners. If the target snapshot failed
			// (nExtracted == 0) the externalized kvState is empty —
			// reset the session so its metadata doesn't advertise N
			// cached tokens backed by zero bytes.
			if job.imcMediaAnchorAdvance {
				if !snapshotOK {
					_ = snapshotStore.Close()
					e.finishSlot(s, fmt.Errorf("start-slot: staged media snapshot failed"))
					return
				}

				useNonCausal := job.imcSession.useNonCausal
				if job.imcMediaAppend && s.mtmdCtx != 0 {
					useNonCausal = mtmd.DecodeUseNonCausal(s.mtmdCtx, 0)
				}
				oldStore := e.model.imcCommitMediaAdvance(job.imcSession, snapshotStore,
					job.imcNewMsgsHash, job.imcNewTotalCached, job.imcNewCachedMsgCount,
					job.imcNewLogicalPosition, job.imcPromptPlan, job.imcMediaSamplerTokens, job.imcMediaKVCounts, job.imcMediaNativeChunks,
					job.imcSessionUseMRoPE, useNonCausal, job.imcExpectedRenderHash)
				if oldStore != nil {
					if err := oldStore.Close(); err != nil {
						e.model.log(job.ctx, "start-slot", "status", "imc-media-anchor-old-store-close-failed", "err", err)
					}
				}
				// Keep the reservation through suffix setup and generation. Those
				// paths still read media/M-RoPE accounting from the session, so
				// publishing here would allow an LRU reset to race those reads.
				// finishSlot is the single release/publication point.
				e.model.log(job.ctx, "start-slot", "status", "imc-media-anchor-committed",
					"slot", s.id, "seq", s.seqID, "physical_kv_cells", job.imcNewTotalCached,
					"next_logical_position", job.imcNewLogicalPosition, "replay_text_tokens", len(job.imcNewCacheTokens))
			} else if sessionWasCommitted {
				switch {
				case snapshotOK:
					e.model.imcPublishSession(job.imcSession)
					job.imcReservationHeld = false
				default:
					e.model.imcInvalidateReservedSession(job.imcSession)
				}
			}
		}
	}

	// Branch based on request type: media vs text-only.
	// Use len(job.media) to distinguish: after an IMC media cache build the
	// suffix is text-only (images are already in KV cache), so route to
	// startSlotText even though job.object may be ObjectChatMedia.
	//
	// Special case: if the IMC media cache was built using M-RoPE positions,
	// the suffix text must also use M-RoPE 4D positions to maintain consistent
	// positional encoding. Route through startSlotTextMRoPE which decodes via
	// the M-RoPE text helper instead of the shared batch.
	switch {
	case job.object == ObjectChatMedia && len(job.media) > 0:
		if !e.startSlotMedia(s, job, cacheIdx, buf) {
			return
		}

	case e.slotNeedsMRoPE(s, job):
		if !e.startSlotTextMRoPE(s, job, cacheIdx, buf) {
			return
		}

	default:
		if !e.startSlotText(s, job, cacheIdx) {
			return
		}
	}

	// Calculate current KV usage for diagnostics.
	var kvUsed llama.Pos
	for _, slot := range e.slots {
		if slot.active {
			if posMax, err := llama.MemorySeqPosMax(e.model.mem, slot.seqID); err == nil && posMax >= 0 {
				kvUsed += posMax + 1
			}
		}
	}

	e.model.log(job.ctx, "batch-engine", "status", "slot-started", "slot", s.id, "seq", s.seqID, "id", job.id,
		"total_prompt", s.nPrompt, "imc_active", job.imcCacheHit, "imc_cache_hit", job.imcSnapshotReused,
		"imc_cache_entry", job.imcSessionID, "kv_logical_positions", kvUsed)
}

// startSlotText initializes a text-only slot. Returns true on success.
func (e *batchEngine) startSlotText(s *slot, job *chatJob, cacheIdx llama.Pos) bool {
	// Token-v2 plans supply the exact complete-render tail. Other text requests
	// normally carry tokens from synchronous preparation; retain tokenization as
	// a fallback for internal jobs constructed without that preparation.
	addBOS := cacheIdx == 0 && e.model.addBOSToken

	// Guard against passing a prompt that still carries an unresolved media
	// marker into libllama's tokenizer. That happens when a media-bearing
	// request is mis-routed to the text path (e.g. media bytes failed to
	// extract, template rendered the marker but the bitmap list is empty).
	// Tokenizing a marker with parseSpecial=true can NULL-deref deep inside
	// libllama, which is an uncatchable cgo SIGSEGV. Fail the slot cleanly
	// instead so the caller gets an error and the process stays up.
	if marker := mtmd.DefaultMarker(); !job.imcTokenPlan && marker != "" && strings.Contains(job.prompt, marker) {
		err := fmt.Errorf("start-slot: prompt routed to text path still contains media marker %q (object=%s, media_count=%d) — refusing to tokenize to avoid libllama SIGSEGV", marker, job.object, len(job.media))
		e.finishSlot(s, err)
		return false
	}

	var tokens []llama.Token
	if job.imcTokenPlan {
		tokens = job.tailTokens
	} else if job.textTokens != nil {
		tokens = job.textTokens
	} else {
		tokens = llama.Tokenize(e.model.vocab, job.prompt, addBOS, true)
	}

	// suffixTokens is the number of new tokens to process (not cached).
	// totalPrompt is the full context size including cached tokens.
	suffixTokens := len(tokens)
	totalPrompt := suffixTokens + int(cacheIdx)
	s.nPrompt = totalPrompt

	// Log token counts for debugging batch overflow.
	e.model.log(job.ctx, "start-slot", "status", "tokenized",
		"slot", s.id,
		"cache_mode", func() string {
			if job.imcTokenPlan {
				return "token-v2"
			}
			return "none"
		}(),
		"match_kind", job.imcMatchKind,
		"suffix_tokens", suffixTokens,
		"cached_tokens", cacheIdx,
		"total_prompt", totalPrompt,
		"nbatch", e.model.cfg.EffectiveNBatch(),
		"batch_current", e.batch.NTokens)

	if !e.applyContextTokenBudget(s, "start-slot") {
		return false
	}

	// Prime penalties and DRY with the complete logical prompt, including any
	// prefix restored from IMC. The suffix alone is not sufficient because the
	// sampler has no state corresponding to restored KV cells.
	samplerTokens := tokens
	if job.imcTokenPlan {
		samplerTokens = job.samplerPromptTokens
	}
	primeSampler(s.sampler, samplerTokens, job.params)

	// Store full prompt tokens for draft model prefill if speculative decoding
	// is enabled. The draft model needs all tokens (cached + new suffix) to
	// build its KV cache after the target's prefill completes. Reuses the
	// pre-allocated promptBuf to avoid per-request allocations.
	// Skip when the slot has media cached — cachedTokens can't represent
	// image/audio embeddings, so the draft model can't reconstruct the prompt.
	draftSlotHasMedia := job.imcCacheHit && job.imcSessionMedia
	// MTP draft: skip the separate draft-prefill path. MTP populates the
	// draft KV by mirroring the TARGET's prefill chunks (each target
	// decode emits a pre-norm hidden buffer that we replay into the
	// draft with batch.embd populated — see batchgen_mtp.go). The mirror
	// step also advances draft.draftNPast in lock-step with the target,
	// so the draftPrefillNeeded / draftPromptTokens scaffolding used by
	// the separate-GGUF path would only cause a redundant (and broken,
	// because it can't supply embd) re-prefill.
	if e.model.draft != nil && e.model.draft.mtp() {
		// Clear any stale draftPromptTokens from a previous non-MTP slot
		// reuse; mtpHasBatch / pendingH are reset in slot.reset().
		s.draftPromptTokens = nil
		s.draftPrefillNeeded = false

		// Gemma4's assistant shares the target KV. Restoring the target
		// therefore restores the assistant's resume point as well; the
		// guaranteed token-v2 tail captures pendingH before drafting.
		if _, shared := e.model.draft.(*sharedMTPDrafter); shared && job.imcCacheHit {
			s.draftNPast = cacheIdx
			s.mtp.ResumeSource = "shared-target-kv"
			e.model.log(job.ctx, "speculative", "status", "mtp-resume", "slot", s.id,
				"resume_source", "shared-target-kv", "cached_tokens", cacheIdx,
				"tail_tokens", len(tokens))
		}

		// Disable MTP for this request only when IMC restored the
		// target prefix but the draft seq state did NOT come along —
		// the IMC restore block above attempts to restore the draft
		// seq KV + pendingH alongside the target. If that succeeded,
		// s.draftNPast is advanced to cacheIdx and pendingH carries
		// the cached prefix's last pre-norm row, so MTP can keep
		// running. If it failed (build-time draft snapshot was
		// missing, restore returned 0 bytes, etc.), s.draftNPast
		// stays at 0 and we fall back to target-only decoding for
		// the remainder of the request. Running MTP with stale /
		// empty draft KV produces near-zero acceptance for the remainder
		// of the request.
		if job.imcCacheHit && s.draftNPast < cacheIdx {
			s.mtp.Disable("imc-hit")
			e.model.log(job.ctx, "speculative", "status", "mtp-disabled-imc-hit",
				"slot", s.id, "id", job.id, "cached_tokens", cacheIdx,
				"draft_n_past", s.draftNPast)
		}
	}
	if e.model.draft != nil && !e.model.draft.mtp() && !draftSlotHasMedia {
		draft := e.model.draft.core()
		var needed int
		var cachedLen int

		switch {
		case job.imcCacheHit && len(job.imcNewCachedTokens) > 0:
			cached := job.imcNewCachedTokens
			cachedLen = len(cached)
			needed = cachedLen + len(tokens)

			if cap(draft.promptBuf) >= needed {
				draft.promptBuf = draft.promptBuf[:needed]
			} else {
				draft.promptBuf = make([]llama.Token, needed)
			}
			copy(draft.promptBuf, cached)
			copy(draft.promptBuf[cachedLen:], tokens)

		default:
			needed = len(tokens)

			if cap(draft.promptBuf) >= needed {
				draft.promptBuf = draft.promptBuf[:needed]
			} else {
				draft.promptBuf = make([]llama.Token, needed)
			}
			copy(draft.promptBuf, tokens)
		}

		s.draftPromptTokens = draft.promptBuf

		e.model.log(job.ctx, "speculative", "status", "draft-prompt-assembled",
			"slot", s.id, "imc_cached", cachedLen, "new_suffix", len(tokens),
			"total_draft_tokens", len(s.draftPromptTokens))

		s.draftPrefillNeeded = true
	}

	// Store tokens for chunked prefill.
	s.prefillTokens = tokens
	s.nPrefilled = 0

	// processBatch selects the current prefill owner after all generation rows
	// have been staged. Do not let admission bypass that ordering or cursor.
	return true
}

// slotNeedsMRoPE returns true if the slot has cached media that was built with
// M-RoPE 4D positions, meaning the suffix text must also use M-RoPE decoding.
func (e *batchEngine) slotNeedsMRoPE(s *slot, job *chatJob) bool {
	if !job.imcCacheHit {
		return false
	}

	// For the initial media build, check the mtmdCtx directly.
	if job.imcMediaBuild && s.mtmdCtx != 0 {
		return mtmd.DecodeUseMRope(s.mtmdCtx)
	}

	return job.imcSessionUseMRoPE
}

// startSlotTextMRoPE initializes a text-only slot that must use M-RoPE 4D
// positioning. This is used when the IMC media cache was built with M-RoPE
// positions (e.g., Qwen vision models) and the suffix text must use the same
// positional encoding scheme. Decodes the suffix via decodeTextMRoPE instead
// of the shared batch, then samples the first token. Returns true on success.
func (e *batchEngine) startSlotTextMRoPE(s *slot, job *chatJob, cacheIdx llama.Pos, buf []byte) bool {
	addBOS := cacheIdx == 0 && e.model.addBOSToken
	var tokens []llama.Token
	if job.imcTokenPlan {
		tokens = job.tailTokens
	} else {
		tokens = llama.Tokenize(e.model.vocab, job.prompt, addBOS, true)
	}

	suffixTokens := len(tokens)
	cachedPromptTokens := job.imcPhysicalCached
	if cachedPromptTokens == 0 {
		cachedPromptTokens = int(cacheIdx)
	}
	totalPrompt := suffixTokens + cachedPromptTokens
	s.nPrompt = totalPrompt

	e.model.log(job.ctx, "start-slot", "status", "tokenized-mrope-suffix",
		"slot", s.id,
		"suffix_tokens", suffixTokens,
		"cached_kv_cells", cachedPromptTokens,
		"next_logical_position", cacheIdx,
		"total_prompt", totalPrompt)

	if !e.applyContextTokenBudget(s, "start-slot") {
		return false
	}

	primeSampler(s.sampler, job.samplerPromptTokens, job.params)

	s.useMRoPE = true
	if e.model.draft != nil && e.model.draft.mtp() {
		// M-RoPE media snapshots currently externalize target KV only. Until
		// draft position compatibility is proven, keep target reuse enabled
		// but disable speculative decoding for this request.
		s.mtp.Disable("media-mrope")
		e.model.log(job.ctx, "speculative", "status", "mtp-disabled-media-mrope",
			"slot", s.id, "id", job.id, "reason", s.mtp.DisableReason)
	}

	nBatch := e.model.cfg.EffectiveNBatch()
	for start := 0; start < len(tokens); start += nBatch {
		end := min(start+nBatch, len(tokens))
		if err := e.decodeTextMRoPE(s, tokens[start:end]); err != nil {
			e.finishSlot(s, fmt.Errorf("decode cached-media suffix (M-RoPE) failed: %w", err))
			return false
		}
	}

	return e.sampleFirstToken(s, buf)
}

// startSlotMedia initializes a media (vision/audio) slot. Returns true on success.
func (e *batchEngine) startSlotMedia(s *slot, job *chatJob, cacheIdx llama.Pos, buf []byte) bool {
	// Convert raw media bytes into bitmap structures for the vision/audio
	// encoder. Images are decoded in Go (newMediaBitmap) and built via the
	// stable mtmd_bitmap_init core API; audio still goes through the
	// mtmd-helper. Reject empty payloads or any bytes that fail to decode so
	// we surface a precise error instead of the generic "tokenization failed
	// with code 1" from mtmd.Tokenize.
	if len(job.media) > 0 {
		s.bitmaps = make([]mtmd.Bitmap, len(job.media))
		for i, med := range job.media {
			if len(med) == 0 {
				e.finishSlot(s, fmt.Errorf("start-slot-media: media[%d] is empty", i))
				return false
			}
			bmp, err := newMediaBitmap(s.mtmdCtx, med)
			if err != nil {
				e.finishSlot(s, fmt.Errorf("start-slot-media: media[%d]: %w", i, err))
				return false
			}
			s.bitmaps[i] = bmp
		}
	}

	// Verify the marker count in the rendered prompt matches the number of
	// bitmaps before calling mtmd.Tokenize. mtmd returns an opaque code 1
	// when these don't match; pre-checking here gives a precise error and
	// catches double-render or template bugs early.
	markerCount := strings.Count(job.prompt, mtmd.DefaultMarker())
	if markerCount != len(s.bitmaps) {
		e.finishSlot(s, fmt.Errorf("start-slot-media: marker/bitmap count mismatch: prompt has %d %q markers but %d bitmaps were prepared", markerCount, mtmd.DefaultMarker(), len(s.bitmaps)))
		return false
	}

	// Create input chunks that interleave text tokens with image embeddings.
	s.inputChunks = mtmd.InputChunksInit()

	// Tokenize produces a sequence of chunks: text tokens and image patches.
	input := mtmd.NewInputText(job.prompt, true, true)

	result := mtmd.Tokenize(s.mtmdCtx, s.inputChunks, input, s.bitmaps)
	if result != 0 {
		err := fmt.Errorf("start-slot-media: tokenization failed with code %d", result)
		e.finishSlot(s, err)
		return false
	}

	// Set model-specific flags for positioning and attention.
	s.useMRoPE = mtmd.DecodeUseMRope(s.mtmdCtx)
	s.useNonCausal = mtmd.DecodeUseNonCausal(s.mtmdCtx, 0)

	// Count total tokens across all chunks.
	numChunks := mtmd.InputChunksSize(s.inputChunks)
	var totalTokens uint64
	for i := range numChunks {
		chunk := mtmd.InputChunksGet(s.inputChunks, i)
		totalTokens += mtmd.InputChunkGetNTokens(chunk)
	}

	s.nPrompt = int(totalTokens) + int(cacheIdx)
	s.chunkIdx = 0

	e.model.log(job.ctx, "start-slot-media", "status", "tokenized",
		"slot", s.id,
		"num_chunks", numChunks,
		"total_tokens", totalTokens,
		"cached_tokens", cacheIdx,
		"use_mrope", s.useMRoPE,
		"use_noncausal", s.useNonCausal)

	if !e.applyContextTokenBudget(s, "start-slot-media") {
		return false
	}

	// mtmd text chunks contain vocabulary tokens; image and audio chunks
	// represent embeddings and must never be accepted into the sampler.
	for i := range numChunks {
		chunk := mtmd.InputChunksGet(s.inputChunks, i)
		if mtmd.InputChunkGetType(chunk) == mtmd.InputChunkTypeText {
			primeSampler(s.sampler, mtmd.InputChunkGetTokensText(chunk), job.params)
		}
	}

	// Media prefill starts in processBatch after ready generation work has
	// received priority. Admission only initializes and tokenizes the request.
	return true
}

func (e *batchEngine) applyContextTokenBudget(s *slot, operation string) bool {
	contextWindow := e.model.cfg.ContextWindow()
	effectiveMaxTokens, ok := contextOutputBudget(s.nPrompt, s.job.params.MaxTokens, contextWindow)
	if !ok {
		err := fmt.Errorf("%s: %w: input tokens [%d] exceed context window [%d]", operation, ErrInvalidRequest, s.nPrompt, contextWindow)
		e.finishSlot(s, err)
		return false
	}

	requestedMaxTokens := s.job.params.MaxTokens
	s.job.params.MaxTokens = effectiveMaxTokens
	if effectiveMaxTokens < requestedMaxTokens {
		e.model.log(s.job.ctx, operation,
			"status", "max-tokens-clamped",
			"prompt_tokens", s.nPrompt,
			"context_window", contextWindow,
			"requested_max_tokens", requestedMaxTokens,
			"effective_max_tokens", effectiveMaxTokens)
	}

	return true
}

// snapshotSystemCache serializes the live target and own-KV MTP state at
// the verified system-only token boundary. The model publishes it into the
// immutable System preload pool. A missing draft snapshot degrades safely to
// target-only reuse.
func (e *batchEngine) snapshotSystemCache(ctx context.Context, s *slot, job *chatJob, boundary int) bool {
	targetStore, err := newSystemCacheStore()
	if err != nil {
		e.model.log(ctx, "start-slot", "status", "imc-system-target-create-failed", "err", err)
		return false
	}

	closeTarget := func() {
		if err := targetStore.Close(); err != nil {
			e.model.log(ctx, "start-slot", "status", "imc-system-target-close-failed", "err", err)
		}
	}

	e.model.decodeMu.Lock()
	llama.Synchronize(e.model.lctx)
	targetSize := llama.StateSeqGetSize(e.model.lctx, s.seqID)
	targetExtracted := llama.StateSeqGetData(e.model.lctx, targetStore.Prepare(int(targetSize)), s.seqID)
	e.model.decodeMu.Unlock()
	if targetSize == 0 || targetExtracted != targetSize {
		closeTarget()
		e.model.log(ctx, "start-slot", "status", "imc-system-target-snapshot-failed",
			"extracted_bytes", targetExtracted, "expected_bytes", targetSize)
		return false
	}
	targetStore.Commit(int(targetExtracted))

	var draftStore SessionStore
	var pendingH []float32
	if ext, ok := e.model.draft.(draftKVExternalizer); ok {
		draftStore, err = newSystemCacheStore()
		if err != nil {
			e.model.log(ctx, "start-slot", "status", "imc-system-draft-create-failed", "err", err)
		} else {
			draft := ext.core()
			dctx := ext.draftKVCtx()
			e.model.decodeMu.Lock()
			llama.Synchronize(dctx)
			draftSize := llama.StateSeqGetSize(dctx, s.seqID)
			draftExtracted := llama.StateSeqGetData(dctx, draftStore.Prepare(int(draftSize)), s.seqID)
			e.model.decodeMu.Unlock()
			if validMTPDraftState(draftExtracted, draftSize, s.mtp.PendingHidden, draft.mtp.EmbeddingSize()) {
				draftStore.Commit(int(draftExtracted))
				pendingH = slices.Clone(s.mtp.PendingHidden)
			} else {
				if closeErr := draftStore.Close(); closeErr != nil {
					e.model.log(ctx, "start-slot", "status", "imc-system-draft-close-failed", "err", closeErr)
				}
				draftStore = nil
				e.model.log(ctx, "start-slot", "status", "imc-system-draft-snapshot-failed",
					"extracted_bytes", draftExtracted, "expected_bytes", draftSize)
			}
		}
	}

	return e.model.imcPublishSystemCache(ctx, job.imcNewCachedTokens[:boundary], targetStore, draftStore, pendingH)
}

func contextOutputBudget(promptTokens, requestedMaxTokens, contextWindow int) (int, bool) {
	if promptTokens >= contextWindow {
		return 0, false
	}

	return min(requestedMaxTokens, contextWindow-promptTokens), true
}

func validMTPDraftState(actualBytes, expectedBytes uint64, pendingH []float32, nEmbd int) bool {
	return expectedBytes > 0 && actualBytes == expectedBytes && nEmbd > 0 && len(pendingH) == nEmbd
}

func primeSampler(sampler llama.Sampler, tokens []llama.Token, params Params) {
	primeSamplerWith(sampler, tokens, samplerPromptRequired(params), llama.SamplerAccept)
}

func primeSamplerWith(sampler llama.Sampler, tokens []llama.Token, required bool, accept func(llama.Sampler, llama.Token)) {
	if !required {
		return
	}

	for _, token := range tokens {
		if token != llama.TokenNull {
			accept(sampler, token)
		}
	}
}
