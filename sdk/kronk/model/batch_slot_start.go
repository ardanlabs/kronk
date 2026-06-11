package model

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

// startSlot initializes a slot with a new request.
func (e *batchEngine) startSlot(s *slot, job *chatJob, buf []byte) {
	s.reset()
	s.active = true
	s.job = job

	// If the rendered prompt ends with "<think>" followed by any trailing
	// whitespace, the template has already opened a reasoning block. Prime
	// the parser and slot to start in reasoning mode so generated tokens
	// are correctly classified until </think>. Setting reasonFlag ensures
	// grammar sampling is skipped during the thinking phase.
	//
	// Templates differ in trailing whitespace after the <think> opener:
	// Qwen emits exactly "<think>\n", Nemotron emits "<think>\n\n", and
	// some custom templates may emit "<think>" with no newline. Accept
	// any of these by trimming trailing ASCII whitespace before checking.
	//
	// Skip reasoning mode when grammar is specified — grammar constrains
	// the output format, so free-form thinking is counterproductive and
	// would consume max_tokens before producing any constrained content.
	if strings.HasSuffix(strings.TrimRight(job.prompt, " \t\r\n"), "<think>") && job.params.Grammar == "" {
		// Drive the state machine into reasoning mode by feeding the same
		// marker the model would have emitted. Parsers that recognize
		// <think> (standard, qwen, mistral, glm) flip to ChannelReasoning;
		// parsers that don't (gemma, gpt) treat it as content — but
		// those parsers do not produce a "<think>\n" suffix in the
		// prompt, so this branch never runs for them.
		s.stateMachine.Classify("<think>")
		s.reasonFlag = 1
	}

	// End the queue-wait span now that the job has been picked up.
	if job.queueWaitSpan != nil {
		job.queueWaitSpan.End()
	}
	if !job.queuedAt.IsZero() {
		metrics.ObserveChatQueueWait(e.model.modelInfo.ID, time.Since(job.queuedAt))
	}

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

	// Create sampler for this request.
	s.sampler = e.model.toSampler(job.ctx, job.params)

	// Create grammar sampler if grammar is specified (kept separate from chain).
	if job.params.Grammar != "" {
		s.grammarSampler = NewGrammarSampler(e.model.vocab, job.params.Grammar)
	}

	// Create a fresh per-request mtmd context for any request that touches
	// the mtmd pipeline (media-bearing requests, or IMC media cache builds
	// for media-using sessions). Per-request lifetime keeps any internal
	// mtmd state (image_tokens, output buffer, bitmap registry, vision
	// support flags) bounded to a single request. The context is freed in
	// freeSlotResources, which finishSlot calls on every exit path.
	needsMTMD := e.model.projFile != "" && (job.imcMediaBuild ||
		(job.object == ObjectChatMedia && len(job.media) > 0))
	if needsMTMD {
		mtmdCtx, err := mtmd.InitFromFile(e.model.projFile, e.model.model, mtmdContextParams(e.model.cfg))
		if err != nil {
			// For IMC media builds processIMC already reserved the
			// session with pending=true. We failed before reaching the
			// commit/publish/reset block in the IMC switch below, so
			// the session would otherwise stay pending forever and
			// deadlock waiters. Drop the reservation here.
			if job.imcMediaBuild && job.imcSession != nil {
				e.model.imcClearPending(job.imcSessionID)
			}

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
	// session pool is sized NSeqMax * imcSessionMultiplier, so there is no
	// fixed slot-to-session correspondence: the session is bound to this
	// slot's KV sequence here and unbound in finishSlot.
	var cacheIdx llama.Pos

	// sessionWasCommitted records whether we called imcCommitSession in
	// the cache-build / cache-extend branches below. When true, the
	// session's metadata is up to date but its pending flag is still set;
	// the snapshot block must finalize publication (imcPublishSession on
	// snapshot success, or imcResetSession on snapshot failure) so a
	// concurrent processIMC scanner never sees the new metadata against
	// stale/empty kvState bytes.
	var sessionWasCommitted bool

	switch {
	case e.model.cfg.IncrementalCache() && job.imcCacheHit:
		// Snapshot session state under lock. With externalized KV, the
		// session's kvState slice header may be reset/regrown by another
		// goroutine's eviction, so we must copy the slice header atomically.
		//
		// For pure-hit snapshot-skip candidates we additionally validate
		// that the live session still matches the version processIMC
		// observed. A concurrent extend/rebuild between processIMC and
		// startSlot would change cachedMsgsHash / cachedMsgCount /
		// totalTokensCached / cachedRenderInputHash; restoring that newer
		// state against a suffix prompt rendered for the older boundary
		// would corrupt the request. On mismatch we fail with a retryable
		// error so the caller re-runs processIMC against the new boundary.
		var kvState []byte
		if job.imcSession != nil {
			session := job.imcSession

			e.model.cacheMu.Lock()
			cacheIdx = llama.Pos(session.totalTokensCached)
			kvState = session.kvState.Bytes()
			sessionVersionOK := !job.imcPureHitSkipSnapshot ||
				(session.cachedMsgsHash == job.imcExpectedHash &&
					session.cachedMsgCount == job.imcExpectedCachedMsgs &&
					session.totalTokensCached == job.imcExpectedTokens &&
					session.cachedRenderInputHash != "" &&
					session.cachedRenderInputHash == job.imcExpectedRenderHash &&
					!session.hasMedia &&
					!session.pending &&
					session.kvState.Len() > 0)

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
					"slot", s.id, "imc_slot", job.imcSessionID,
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
		if len(kvState) > 0 && !job.imcClearSeq {
			e.model.log(job.ctx, "start-slot", "status", "imc-restore-start",
				"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
				"ram_bytes", fmtBytes(uint64(len(kvState))))

			restoreStart := time.Now()

			e.model.decodeMu.Lock()
			nRead := llama.StateSeqSetData(e.model.lctx, kvState, s.seqID)
			e.model.decodeMu.Unlock()

			if nRead == 0 {
				e.model.imcClearPending(job.imcSessionID)
				e.finishSlot(s, fmt.Errorf("start-slot: imc restore failed for seq %d", s.seqID))
				return
			}

			e.model.log(job.ctx, "start-slot", "status", "imc-restore-done",
				"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
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
			if e.model.draft != nil && e.model.draft.mtp && job.imcSession != nil && job.imcSession.draftKVState != nil {
				draft := e.model.draft

				var draftBytes []byte
				var savedPendingH []float32
				e.model.cacheMu.RLock()
				draftBytes = job.imcSession.draftKVState.Bytes()
				if len(job.imcSession.pendingH) == draft.nEmbd {
					savedPendingH = append(savedPendingH, job.imcSession.pendingH...)
				}
				e.model.cacheMu.RUnlock()

				switch {
				case len(draftBytes) > 0:
					draftRestoreStart := time.Now()

					e.model.decodeMu.Lock()
					nDraftRead := llama.StateSeqSetData(draft.lctx, draftBytes, s.seqID)
					e.model.decodeMu.Unlock()

					switch {
					case nDraftRead > 0:
						// Mirror the slot's draft state to what the
						// snapshot covers so subsequent mirror /
						// generateDraftTokensMTP calls find a
						// consistent draftNPast and pendingH.
						s.draftNPast = cacheIdx
						if len(savedPendingH) == draft.nEmbd {
							if cap(s.pendingH) < draft.nEmbd {
								s.pendingH = make([]float32, draft.nEmbd)
							} else {
								s.pendingH = s.pendingH[:draft.nEmbd]
							}
							copy(s.pendingH, savedPendingH)
						}
						e.model.log(job.ctx, "start-slot", "status", "imc-draft-restore-done",
							"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
							"restored_bytes", fmtBytes(nDraftRead),
							"pending_h", len(s.pendingH) == draft.nEmbd,
							"elapsed", fmtDur(time.Since(draftRestoreStart)))
					default:
						// Restore failed — drop draft seq + pendingH so
						// startSlotText falls back to the mtp-disabled
						// path. We don't fail the whole request because
						// the target restored fine; only MTP is lost.
						e.model.decodeMu.Lock()
						llama.MemorySeqRm(draft.mem, s.seqID, -1, -1)
						e.model.decodeMu.Unlock()
						s.draftNPast = 0
						s.pendingH = s.pendingH[:0]
						e.model.log(job.ctx, "start-slot", "status", "imc-draft-restore-failed",
							"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx)
					}

				default:
					// No draft snapshot on the session (e.g., the
					// build-time draft snapshot failed). Leave draft
					// seq empty so startSlotText disables MTP for the
					// request via the existing mtp-disabled-imc-hit
					// path.
					e.model.log(job.ctx, "start-slot", "status", "imc-draft-restore-skip-empty",
						"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx)
				}
			}
		}

		// Decode new cache extension tokens into the slot's sequence if any.
		switch {
		case job.imcMediaBuild:
			skipTokens := job.imcMediaSkipTextTokens

			if skipTokens > 0 {
				// Partial media extend: keep existing text cache, only decode
				// new content (text suffix + media + post-media text).
				e.model.log(job.ctx, "start-slot", "status", "imc-media-extend", "slot", s.id, "seq", s.seqID,
					"skip_text_tokens", skipTokens)
			} else {
				// Full media rebuild: clear sequence and decode all cached
				// messages through the mtmd pipeline.
				e.model.log(job.ctx, "start-slot", "status", "imc-media-build", "slot", s.id, "seq", s.seqID)

				e.model.decodeMu.Lock()
				llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
				e.model.decodeMu.Unlock()
			}

			imcDecodeStart := time.Now()

			totalCached, mediaKVCounts, err := e.model.decodeMediaIntoCache(job.ctx, job.imcMediaCacheD, s.seqID, s.mtmdCtx, skipTokens)
			if err != nil {
				e.model.decodeMu.Lock()
				if skipTokens > 0 {
					// Partial extend failed: remove only the newly decoded
					// content, preserving the original text cache.
					llama.MemorySeqRm(e.model.mem, s.seqID, llama.Pos(skipTokens), -1)
				} else {
					llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
				}
				e.model.decodeMu.Unlock()

				e.model.imcClearPending(job.imcSessionID)

				e.finishSlot(s, fmt.Errorf("start-slot: imc media build: %w", err))
				return
			}

			metrics.AddPrefillTime(e.model.modelInfo.ID, "imc-decode", time.Since(imcDecodeStart))

			cacheIdx = llama.Pos(totalCached)

			e.model.imcCommitSession(job.imcSession, job.imcNewMsgsHash, totalCached, job.imcNewCachedMsgCount, nil, true, mediaKVCounts, job.imcSysPromptHash, job.imcSysPromptTokens, job.imcExpectedRenderHash)
			sessionWasCommitted = true

			// Store whether this media build used M-RoPE so follow-up
			// text-only requests on this session can use the correct position format.
			if s.mtmdCtx != 0 && job.imcSession != nil {
				e.model.cacheMu.Lock()
				job.imcSession.useMRoPE = mtmd.DecodeUseMRope(s.mtmdCtx)
				e.model.cacheMu.Unlock()
			}

			if skipTokens > 0 {
				e.model.log(job.ctx, "start-slot", "status", "imc-media-extended", "slot", s.id, "seq", s.seqID,
					"total_cached", totalCached, "skipped_text_tokens", skipTokens)
			} else {
				e.model.log(job.ctx, "start-slot", "status", "imc-media-built", "slot", s.id, "seq", s.seqID,
					"total_cached", totalCached)
			}

		case len(job.imcNewCacheTokens) > 0:
			// Detect stale extension: if another request extended this slot
			// between our scan and now, cacheIdx won't match the position
			// these tokens were sliced from. For extends (not rebuilds or
			// partial prefix trims), the expected start position is
			// imcNewTotalCached - len(imcNewCacheTokens).
			if !job.imcClearSeq && job.imcTrimPos == 0 {
				expectedStart := llama.Pos(job.imcNewTotalCached - len(job.imcNewCacheTokens))
				if cacheIdx != expectedStart {
					e.model.log(job.ctx, "start-slot", "status", "imc-extend-stale", "slot", s.id, "seq", s.seqID,
						"cache_idx", cacheIdx, "expected_start", expectedStart,
						"new_total_cached", job.imcNewTotalCached)

					e.model.imcClearPending(job.imcSessionID)

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
				llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
				// MTP: the prior imc-restore (or a previous decode in
				// this slot) populated the draft seq KV at positions
				// [0..N). The forthcoming decodeTokensIntoCacheMTP
				// rewrites draft positions starting at 0; without
				// clearing the draft seq first, the mirror decode
				// collides with the surviving positions and fails
				// with "the input could not be processed".
				if e.model.draft != nil && e.model.draft.mtp {
					llama.MemorySeqRm(e.model.draft.mem, s.seqID, -1, -1)
					s.draftNPast = 0
					s.pendingH = s.pendingH[:0]
				}
				e.model.decodeMu.Unlock()

				cacheIdx = 0

				e.model.log(job.ctx, "start-slot", "status", "imc-build", "slot", s.id, "seq", s.seqID,
					"tokens", len(job.imcNewCacheTokens))

			case job.imcTrimPos > 0:
				// Token prefix fallback or sys-prompt-preserve: trim the
				// divergent suffix and re-decode only the new tokens.
				//
				// Dense/MoE: partial MemorySeqRm is safe — delete from
				// trimPos onward and decode only the new conversation body.
				// This preserves the system prompt KV state.
				//
				// Hybrid: partial MemorySeqRm corrupts recurrent state
				// (DeltaNet/SSM), so clear the entire sequence and
				// re-decode everything from scratch.
				if e.model.modelInfo.Type == ModelTypeHybrid {
					e.model.log(job.ctx, "start-slot", "status", "imc-rebuild-full", "slot", s.id, "seq", s.seqID,
						"trim_pos", job.imcTrimPos, "total_tokens", len(job.imcNewCachedTokens))

					e.model.decodeMu.Lock()
					llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
					// MTP: clear the draft seq in lock-step (see the
					// imcClearSeq branch above for the rationale).
					if e.model.draft != nil && e.model.draft.mtp {
						llama.MemorySeqRm(e.model.draft.mem, s.seqID, -1, -1)
						s.draftNPast = 0
						s.pendingH = s.pendingH[:0]
					}
					e.model.decodeMu.Unlock()

					job.imcNewCacheTokens = job.imcNewCachedTokens
					cacheIdx = 0
				} else {
					e.model.log(job.ctx, "start-slot", "status", "imc-trim-prefix", "slot", s.id, "seq", s.seqID,
						"trim_pos", job.imcTrimPos, "kept_tokens", job.imcTrimPos,
						"new_tokens", len(job.imcNewCacheTokens))

					e.model.decodeMu.Lock()
					llama.MemorySeqRm(e.model.mem, s.seqID, llama.Pos(job.imcTrimPos), -1)
					// MTP: trim the draft seq KV symmetrically so the
					// upcoming mirror decode doesn't collide with stale
					// positions in [trimPos..oldEnd). Roll draftNPast
					// back to trimPos and drop pendingH (the last-row
					// h carried over from beyond trimPos is no longer
					// the previous-position h for the new tokens; the
					// mirror's first chunk will fall back to zeros at
					// slot 0, which matches what a fresh build does).
					if e.model.draft != nil && e.model.draft.mtp {
						llama.MemorySeqRm(e.model.draft.mem, s.seqID, llama.Pos(job.imcTrimPos), -1)
						if s.draftNPast > llama.Pos(job.imcTrimPos) {
							s.draftNPast = llama.Pos(job.imcTrimPos)
						}
						s.pendingH = s.pendingH[:0]
					}
					e.model.decodeMu.Unlock()

					cacheIdx = llama.Pos(job.imcTrimPos)
				}

			default:
				e.model.log(job.ctx, "start-slot", "status", "imc-extend", "slot", s.id, "seq", s.seqID,
					"cached_tokens", cacheIdx, "new_cache_tokens", len(job.imcNewCacheTokens))
			}

			// Decode extension tokens into the slot's sequence.
			// When MTP is enabled, use the mirror-aware variant so the
			// draft seq KV is populated in lock-step with the target
			// seq KV. The post-build snapshot below then captures both
			// seqs, so a cache hit on the next request can restore the
			// draft state too and MTP can keep running through the
			// cached prefix instead of being disabled.
			imcDecodeStart := time.Now()

			var decodeErr error
			switch {
			case e.model.draft != nil && e.model.draft.mtp:
				decodeErr = e.decodeTokensIntoCacheMTP(job.ctx, s, job.imcNewCacheTokens, int(cacheIdx))
			default:
				decodeErr = e.model.decodeTokensIntoCache(job.ctx, job.imcNewCacheTokens, s.seqID, int(cacheIdx))
			}
			if decodeErr != nil {
				e.model.decodeMu.Lock()
				llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
				if e.model.draft != nil && e.model.draft.mtp {
					llama.MemorySeqRm(e.model.draft.mem, s.seqID, -1, -1)
					s.draftNPast = 0
					s.pendingH = s.pendingH[:0]
				}
				e.model.decodeMu.Unlock()

				if job.imcSession != nil {
					e.model.cacheMu.Lock()
					imcResetSession(job.imcSession)
					e.model.cacheMu.Unlock()
					e.model.notifyIMCSlotAvailable()
				}

				e.finishSlot(s, fmt.Errorf("start-slot: imc decode: %w", decodeErr))
				return
			}

			metrics.AddPrefillTime(e.model.modelInfo.ID, "imc-decode", time.Since(imcDecodeStart))

			cacheIdx = llama.Pos(job.imcNewTotalCached)

			hasMedia := len(job.imcMediaKVCounts) > 0
			e.model.imcCommitSession(job.imcSession, job.imcNewMsgsHash, job.imcNewTotalCached, job.imcNewCachedMsgCount, job.imcNewCachedTokens, hasMedia, job.imcMediaKVCounts, job.imcSysPromptHash, job.imcSysPromptTokens, job.imcExpectedRenderHash)
			sessionWasCommitted = true

			e.model.log(job.ctx, "start-slot", "status", "imc-cache-ready", "slot", s.id, "seq", s.seqID,
				"total_cached", job.imcNewTotalCached)

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
	if e.model.cfg.IncrementalCache() && job.imcCacheHit && cacheIdx > 0 && job.imcSession != nil {
		// Pure-hit snapshot skip: when processIMC marked this job as a
		// text-only exact pure hit AND no cached-prefix mutation happened
		// in this startSlot (no extension tokens, no media build, no trim,
		// no clear), the session's externalized kvState already contains
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
		if job.imcPureHitSkipSnapshot &&
			len(job.imcNewCacheTokens) == 0 &&
			!job.imcMediaBuild &&
			!job.imcSessionMedia &&
			len(job.imcMediaKVCounts) == 0 &&
			!job.imcClearSeq &&
			job.imcTrimPos == 0 &&
			cacheIdx == llama.Pos(job.imcExpectedTokens) {

			e.model.cacheMu.RLock()
			session := job.imcSession
			versionOK := session != nil &&
				session.cachedMsgsHash == job.imcExpectedHash &&
				session.cachedMsgCount == job.imcExpectedCachedMsgs &&
				session.totalTokensCached == job.imcExpectedTokens &&
				session.cachedRenderInputHash != "" &&
				session.cachedRenderInputHash == job.imcExpectedRenderHash &&
				session.kvState.Len() > 0 &&
				!session.hasMedia &&
				!session.pending
			e.model.cacheMu.RUnlock()

			// MTP draft guard: if a draft is configured, the draft seq
			// state must have been restored alongside the target (i.e.
			// the prior request's draft snapshot succeeded). If draft
			// restore failed earlier, fall through to the normal
			// snapshot block so the draft snapshot path runs / is
			// logged as today.
			draftOK := true
			if e.model.draft != nil && e.model.draft.mtp {
				draft := e.model.draft
				draftOK = s.draftNPast == cacheIdx
				if draftOK && draft.nEmbd > 0 {
					draftOK = len(s.pendingH) == draft.nEmbd
				}
			}

			skipSnapshot = versionOK && draftOK
		}

		if skipSnapshot {
			metrics.AddIMCSnapshotSkipped(e.model.modelInfo.ID)
			e.model.log(job.ctx, "start-slot", "status", "imc-snapshot-skip-pure-hit",
				"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
				"imc_slot", job.imcSessionID)

			// Fall through to suffix decode; session.kvState,
			// draftKVState, and pendingH all remain valid because the
			// slot was restored from them and no cached-prefix decode
			// happened afterward. No Synchronize is needed: the
			// restore (StateSeqSetData) completed under decodeMu above
			// and we did not mutate KV state since.

		} else {
			e.model.log(job.ctx, "start-slot", "status", "imc-snapshot-start",
				"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx)

			snapshotStart := time.Now()

			// Reuse the session's SessionStore in place. Prepare returns a slice
			// of length kvSize, reusing the existing backing array when its
			// capacity is sufficient (the common case after the first turn)
			// and allocating only when the conversation has grown beyond any
			// previous peak. Per-session serialization (the pending flag and
			// the imcSessions ownership model) guarantees no concurrent reader
			// holds a reference to this buffer while we fill it.
			//
			// capBefore lets us log whether this snapshot grew the backing
			// array (allocation) or reused it (zero allocation) — the central
			// invariant we want to observe in production.
			capBefore := job.imcSession.kvState.Cap()

			e.model.decodeMu.Lock()
			llama.Synchronize(e.model.lctx)
			kvSize := llama.StateSeqGetSize(e.model.lctx, s.seqID)
			kvBuf := job.imcSession.kvState.Prepare(int(kvSize))
			nExtracted := llama.StateSeqGetData(e.model.lctx, kvBuf, s.seqID)
			e.model.decodeMu.Unlock()

			capAfter := job.imcSession.kvState.Cap()
			bufAction := "reuse"
			if capAfter > capBefore {
				bufAction = "grow"
			}

			// Commit (or zero) the buffer length under cacheMu so concurrent
			// readers (LRU snapshot scans, future requests matching this
			// session) see a consistent length.
			e.model.cacheMu.Lock()
			job.imcSession.kvState.Commit(int(nExtracted))
			e.model.cacheMu.Unlock()

			if nExtracted > 0 {
				e.model.log(job.ctx, "start-slot", "status", "imc-snapshot-done",
					"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
					"snapshot_bytes", fmtBytes(nExtracted), "kv_alloc", fmtBytes(kvSize),
					"buf_action", bufAction,
					"buf_cap_before", fmtBytes(uint64(capBefore)),
					"buf_cap_after", fmtBytes(uint64(capAfter)),
					"elapsed", fmtDur(time.Since(snapshotStart)))
			} else {
				e.model.log(job.ctx, "start-slot", "status", "imc-snapshot-failed",
					"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
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
			if nExtracted > 0 && e.model.draft != nil && e.model.draft.mtp && job.imcSession.draftKVState != nil {
				draft := e.model.draft

				draftCapBefore := job.imcSession.draftKVState.Cap()

				e.model.decodeMu.Lock()
				llama.Synchronize(draft.lctx)
				draftKVSize := llama.StateSeqGetSize(draft.lctx, s.seqID)
				draftBuf := job.imcSession.draftKVState.Prepare(int(draftKVSize))
				nDraftExtracted := llama.StateSeqGetData(draft.lctx, draftBuf, s.seqID)
				e.model.decodeMu.Unlock()

				draftCapAfter := job.imcSession.draftKVState.Cap()
				draftBufAction := "reuse"
				if draftCapAfter > draftCapBefore {
					draftBufAction = "grow"
				}

				e.model.cacheMu.Lock()
				job.imcSession.draftKVState.Commit(int(nDraftExtracted))
				// pendingH snapshot: copy the slot's pendingH into the
				// session so a later cache hit can restore it. Lazy-grow
				// the session's pendingH backing slice.
				if len(s.pendingH) == draft.nEmbd {
					if cap(job.imcSession.pendingH) < draft.nEmbd {
						job.imcSession.pendingH = make([]float32, draft.nEmbd)
					} else {
						job.imcSession.pendingH = job.imcSession.pendingH[:draft.nEmbd]
					}
					copy(job.imcSession.pendingH, s.pendingH)
				} else {
					job.imcSession.pendingH = job.imcSession.pendingH[:0]
				}
				e.model.cacheMu.Unlock()

				switch {
				case nDraftExtracted > 0:
					e.model.log(job.ctx, "start-slot", "status", "imc-draft-snapshot-done",
						"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
						"snapshot_bytes", fmtBytes(nDraftExtracted),
						"kv_alloc", fmtBytes(draftKVSize),
						"buf_action", draftBufAction,
						"buf_cap_before", fmtBytes(uint64(draftCapBefore)),
						"buf_cap_after", fmtBytes(uint64(draftCapAfter)),
						"pending_h", len(s.pendingH) == draft.nEmbd)
				default:
					e.model.log(job.ctx, "start-slot", "status", "imc-draft-snapshot-failed",
						"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
						"kv_alloc", fmtBytes(draftKVSize),
						"buf_action", draftBufAction,
						"buf_cap_before", fmtBytes(uint64(draftCapBefore)),
						"buf_cap_after", fmtBytes(uint64(draftCapAfter)))
				}
			}

			// Finalize publication for sessions whose metadata was
			// committed above. Sessions still carry pending=true at
			// this point; publishing makes the new metadata + kvState
			// pair visible to processIMC atomically with respect to
			// concurrent scanners. If the target snapshot failed
			// (nExtracted == 0) the externalized kvState is empty —
			// reset the session so its metadata doesn't advertise N
			// cached tokens backed by zero bytes.
			if sessionWasCommitted {
				switch {
				case nExtracted > 0:
					e.model.imcPublishSession(job.imcSession)
				default:
					e.model.cacheMu.Lock()
					imcResetSession(job.imcSession)
					e.model.cacheMu.Unlock()
					e.model.notifyIMCSlotAvailable()
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
		"total_prompt", s.nPrompt, "imc_cache_hit", job.imcCacheHit, "imc_slot", job.imcSessionID, "kv_used", kvUsed)
}

// startSlotText initializes a text-only slot. Returns true on success.
func (e *batchEngine) startSlotText(s *slot, job *chatJob, cacheIdx llama.Pos) bool {
	// Tokenize the prompt (cached messages already removed).
	// Only add BOS if no cached tokens AND model metadata says to add BOS.
	addBOS := cacheIdx == 0 && e.model.addBOSToken

	// Guard against passing a prompt that still carries an unresolved media
	// marker into libllama's tokenizer. That happens when a media-bearing
	// request is mis-routed to the text path (e.g. media bytes failed to
	// extract, template rendered the marker but the bitmap list is empty).
	// Tokenizing a marker with parseSpecial=true can NULL-deref deep inside
	// libllama, which is an uncatchable cgo SIGSEGV. Fail the slot cleanly
	// instead so the caller gets an error and the process stays up.
	if marker := mtmd.DefaultMarker(); marker != "" && strings.Contains(job.prompt, marker) {
		err := fmt.Errorf("start-slot: prompt routed to text path still contains media marker %q (object=%s, media_count=%d) — refusing to tokenize to avoid libllama SIGSEGV", marker, job.object, len(job.media))
		e.finishSlot(s, err)
		return false
	}

	tokens := llama.Tokenize(e.model.vocab, job.prompt, addBOS, true)

	// suffixTokens is the number of new tokens to process (not cached).
	// totalPrompt is the full context size including cached tokens.
	suffixTokens := len(tokens)
	totalPrompt := suffixTokens + int(cacheIdx)
	s.nPrompt = totalPrompt

	// Log token counts for debugging batch overflow.
	e.model.log(job.ctx, "start-slot", "status", "tokenized",
		"slot", s.id,
		"suffix_tokens", suffixTokens,
		"cached_tokens", cacheIdx,
		"total_prompt", totalPrompt,
		"nbatch", e.model.cfg.NBatch(),
		"batch_current", e.batch.NTokens)

	// Check context window.
	if s.nPrompt > e.model.cfg.ContextWindow() {
		err := fmt.Errorf("start-slot: input tokens [%d] exceed context window [%d]", s.nPrompt, e.model.cfg.ContextWindow())
		e.finishSlot(s, err)
		return false
	}

	// Store full prompt tokens for draft model prefill if speculative decoding
	// is enabled. The draft model needs all tokens (cached + new suffix) to
	// build its KV cache after the target's prefill completes. Reuses the
	// pre-allocated promptBuf to avoid per-request allocations.
	// Skip when the slot has media cached — cachedTokens can't represent
	// image/audio embeddings, so the draft model can't reconstruct the prompt.
	draftSlotHasMedia := false
	if job.imcCacheHit && job.imcSession != nil {
		e.model.cacheMu.RLock()
		draftSlotHasMedia = job.imcSession.hasMedia
		e.model.cacheMu.RUnlock()
	}
	// MTP draft: skip the separate draft-prefill path. MTP populates the
	// draft KV by mirroring the TARGET's prefill chunks (each target
	// decode emits a pre-norm hidden buffer that we replay into the
	// draft with batch.embd populated — see batch_mtp.go). The mirror
	// step also advances draft.draftNPast in lock-step with the target,
	// so the draftPrefillNeeded / draftPromptTokens scaffolding used by
	// the separate-GGUF path would only cause a redundant (and broken,
	// because it can't supply embd) re-prefill.
	if e.model.draft != nil && e.model.draft.mtp {
		// Clear any stale draftPromptTokens from a previous non-MTP slot
		// reuse; mtpHasBatch / pendingH are reset in slot.reset().
		s.draftPromptTokens = nil
		s.draftPrefillNeeded = false

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
		// empty draft KV produces near-zero acceptance and poisons
		// specAccEMA (which persists across requests on the slot).
		if job.imcCacheHit && s.draftNPast < cacheIdx {
			s.mtpDisabledForRequest = true
			s.mtpDisableReason = "imc-hit"
			e.model.log(job.ctx, "speculative", "status", "mtp-disabled-imc-hit",
				"slot", s.id, "id", job.id, "cached_tokens", cacheIdx,
				"draft_n_past", s.draftNPast)
		}
	}
	if e.model.draft != nil && !e.model.draft.mtp && !draftSlotHasMedia {
		draft := e.model.draft
		var needed int
		var cachedLen int

		switch {
		case job.imcCacheHit && job.imcSession != nil:
			e.model.cacheMu.RLock()
			cached := job.imcSession.cachedTokens
			e.model.cacheMu.RUnlock()

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

	// Add first chunk of prompt tokens to batch. Use NBatch as the limit
	// since this is the initial fill for a newly assigned slot.
	if !e.addPrefillChunk(s, e.model.cfg.NBatch()) {
		e.finishSlot(s, e.slotCancelError(s))
		return false
	}

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

	// For follow-up requests, check the stored flag on the matched session.
	if job.imcSession != nil {
		e.model.cacheMu.RLock()
		needsMRoPE := job.imcSession.useMRoPE
		e.model.cacheMu.RUnlock()
		return needsMRoPE
	}

	return false
}

// startSlotTextMRoPE initializes a text-only slot that must use M-RoPE 4D
// positioning. This is used when the IMC media cache was built with M-RoPE
// positions (e.g., Qwen vision models) and the suffix text must use the same
// positional encoding scheme. Decodes the suffix via decodeTextMRoPE instead
// of the shared batch, then samples the first token. Returns true on success.
func (e *batchEngine) startSlotTextMRoPE(s *slot, job *chatJob, cacheIdx llama.Pos, buf []byte) bool {
	addBOS := cacheIdx == 0 && e.model.addBOSToken
	tokens := llama.Tokenize(e.model.vocab, job.prompt, addBOS, true)

	suffixTokens := len(tokens)
	totalPrompt := suffixTokens + int(cacheIdx)
	s.nPrompt = totalPrompt

	e.model.log(job.ctx, "start-slot", "status", "tokenized-mrope-suffix",
		"slot", s.id,
		"suffix_tokens", suffixTokens,
		"cached_tokens", cacheIdx,
		"total_prompt", totalPrompt)

	if s.nPrompt > e.model.cfg.ContextWindow() {
		err := fmt.Errorf("start-slot: input tokens [%d] exceed context window [%d]", s.nPrompt, e.model.cfg.ContextWindow())
		e.finishSlot(s, err)
		return false
	}

	s.useMRoPE = true

	nBatch := e.model.cfg.NBatch()
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
			bmp, err := newMediaBitmap(med)
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

	// Check context window.
	if s.nPrompt > e.model.cfg.ContextWindow() {
		err := fmt.Errorf("start-slot-media: input tokens [%d] exceed context window [%d]", s.nPrompt, e.model.cfg.ContextWindow())
		e.finishSlot(s, err)
		return false
	}

	// Process first chunk. Media prefill is handled chunk-by-chunk in processBatch.
	//
	// addPrefillMediaChunk has two failure modes:
	//   1. Cancellation/shutdown: returns false WITHOUT calling finishSlot
	//      (s.job is still attached). Caller must finishSlot with cancel err.
	//   2. Internal error (e.g., decode failure): calls finishSlot before
	//      returning false, which resets the slot and nils s.job.
	// We must distinguish so we don't dereference a nil s.job in case 2.
	if !e.addPrefillMediaChunk(s, buf) {
		if s.job != nil {
			e.finishSlot(s, e.slotCancelError(s))
		}
		return false
	}

	return true
}
