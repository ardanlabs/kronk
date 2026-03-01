import React, { useState, useCallback, useMemo, useEffect, useRef } from 'react';
import type { AutoTestTrialResult, BestConfigWeights, AutoTestSweepMode } from '../types';
import type { ConfigTrialResult } from '../contexts/AutoTestRunnerContext';
import { loadAutoTestHistory, deleteHistoryEntry, updateHistoryEntry, deleteHistoryEntries, exportAutoTestHistoryJSON, importAutoTestHistoryJSON } from '../services/autoTestHistory';
import type { AutoTestHistoryEntry } from '../services/autoTestHistory';
import { computeCompositeScore, defaultBestConfigWeights, defaultConfigBestWeights, chatScenario, toolCallScenario, configPerfScenario, presetWeights } from '../services/autoTestRunner';
import type { PresetName } from '../services/autoTestRunner';
import type { AutoTestScenario } from '../types';
import { formatMs, buildSamplingColumns, buildConfigColumns } from '../services/sweepModeColumns';
import type { ColumnDef, CellMeta } from '../services/sweepModeColumns';
import { sortRows, sortIndicator, nextSortDirection, BestTrialMetrics, TrialDetails } from './autoTestShared';
import type { SortState } from './autoTestShared';

export default function PlaygroundHistory() {
  const [entries, setEntries] = useState<AutoTestHistoryEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [sort, setSort] = useState<SortState>({ column: null, direction: null });
  const [expandedTrials, setExpandedTrials] = useState<Set<string>>(new Set());
  const [weights, setWeights] = useState<BestConfigWeights>(() => ({ ...defaultBestConfigWeights }));
  const [weightsChanged, setWeightsChanged] = useState(false);
  const [importMessage, setImportMessage] = useState<string | null>(null);
  const appliedWeightsRef = useRef<BestConfigWeights>({ ...defaultBestConfigWeights });
  const fileInputRef = useRef<HTMLInputElement>(null);
  const selectAllRef = useRef<HTMLInputElement>(null);

  // Initial async load
  useEffect(() => {
    let alive = true;
    void loadAutoTestHistory().then(data => {
      if (alive) { setEntries(data); setLoading(false); }
    });
    return () => { alive = false };
  }, []);

  // Listen for history updates (from auto-save in AutoTestRunnerContext)
  useEffect(() => {
    const reload = () => { void loadAutoTestHistory().then(setEntries) };
    const onStorage = (e: Event) => {
      if (e instanceof StorageEvent && e.key !== 'playground:autoTestHistory:updatedAt') return;
      reload();
    };
    window.addEventListener('autotest-history-updated', reload);
    window.addEventListener('storage', onStorage);
    return () => {
      window.removeEventListener('autotest-history-updated', reload);
      window.removeEventListener('storage', onStorage);
    };
  }, []);

  // Auto-clear import message
  useEffect(() => {
    if (!importMessage) return;
    const timer = setTimeout(() => setImportMessage(null), 4000);
    return () => clearTimeout(timer);
  }, [importMessage]);

  // Reconcile selectedIds when entries change (e.g. after external delete).
  useEffect(() => {
    const entryIds = new Set(entries.map(e => e.id));
    setSelectedIds(prev => {
      let changed = false;
      for (const id of prev) {
        if (!entryIds.has(id)) { changed = true; break; }
      }
      if (!changed) return prev;
      const next = new Set<string>();
      for (const id of prev) {
        if (entryIds.has(id)) next.add(id);
      }
      return next;
    });
  }, [entries]);

  // Set indeterminate state on select-all checkbox
  useEffect(() => {
    if (!selectAllRef.current) return;
    const count = selectedIds.size;
    const total = entries.length;
    selectAllRef.current.indeterminate = count > 0 && count < total;
  }, [selectedIds, entries.length]);

  const selected = useMemo(() => entries.find(e => e.id === selectedId) ?? null, [entries, selectedId]);

  const handleSelectEntry = useCallback((id: string) => {
    const entry = entries.find(e => e.id === id);
    setSelectedId(id);
    setSelectedIds(new Set());
    setSort({ column: null, direction: null });
    setExpandedTrials(new Set());
    const fallback = entry?.run?.kind === 'config' ? defaultConfigBestWeights : defaultBestConfigWeights;
    const w = { ...fallback, ...(entry?.run?.weights ?? {}) };
    setWeights({ ...w });
    appliedWeightsRef.current = { ...w };
    setWeightsChanged(false);
  }, [entries]);

  const handleDelete = useCallback((id: string) => {
    void deleteHistoryEntry(id);
    if (selectedId === id) {
      setSelectedId(null);
    }
  }, [selectedId]);

  const handleDeleteSelected = useCallback(() => {
    if (selectedIds.size === 0) return;
    const ids = Array.from(selectedIds);
    void deleteHistoryEntries(ids);
    if (selectedId && selectedIds.has(selectedId)) {
      setSelectedId(null);
    }
    setSelectedIds(new Set());
  }, [selectedIds, selectedId]);

  const handleExport = useCallback(async () => {
    const blob = await exportAutoTestHistoryJSON();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    const date = new Date().toISOString().slice(0, 10);
    a.href = url;
    a.download = `autotest-history-${date}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }, []);

  const handleImport = useCallback(async (file: File) => {
    try {
      const text = await file.text();
      const result = await importAutoTestHistoryJSON(text);
      setImportMessage(`Imported ${result.imported} entr${result.imported !== 1 ? 'ies' : 'y'}${result.skipped > 0 ? ` (${result.skipped} skipped)` : ''}`);
    } catch (err) {
      setImportMessage(`Import failed: ${err instanceof Error ? err.message : 'unknown error'}`);
    }
  }, []);

  const handleWeightChange = useCallback((key: keyof BestConfigWeights, value: number) => {
    setWeights(w => {
      const next = { ...w, [key]: value };
      const applied = appliedWeightsRef.current;
      const changed = (Object.keys(next) as (keyof BestConfigWeights)[]).some(k => next[k] !== applied[k]);
      setWeightsChanged(changed);
      return next;
    });
  }, []);

  const reevaluateWithWeights = useCallback((nextWeights: BestConfigWeights) => {
    if (!selected) return;
    const trialsRaw = selected.run?.trials;
    if (!Array.isArray(trialsRaw)) return;
    const trials = trialsRaw as AutoTestTrialResult[];
    let bestId: string | null = null;
    let bestScore = -Infinity;
    let bestTPS = -Infinity;
    for (const trial of trials) {
      if (trial.status !== 'completed') continue;
      const score = computeCompositeScore(trial, nextWeights);
      if (score > bestScore || (score === bestScore && (trial.avgTPS ?? 0) > bestTPS)) {
        bestScore = score;
        bestTPS = trial.avgTPS ?? 0;
        bestId = trial.id;
      }
    }
    void updateHistoryEntry(selected.id, entry => ({
      ...entry,
      run: { ...entry.run, bestTrialId: bestId ?? undefined, weights: { ...nextWeights } },
    }));
    appliedWeightsRef.current = { ...nextWeights };
    setWeightsChanged(false);
  }, [selected]);

  const handleReevaluate = useCallback(() => {
    reevaluateWithWeights(weights);
  }, [weights, reevaluateWithWeights]);

  const applyPreset = useCallback((preset: PresetName) => {
    if (!selected) return;
    const mode = selected.run.kind;
    const next = presetWeights(preset, mode);
    setWeights(next);
    reevaluateWithWeights(next);
  }, [selected, reevaluateWithWeights]);

  const handleSort = useCallback((column: string) => {
    setSort(prev => ({
      column: prev.column === column && nextSortDirection(prev.direction) === null ? null : column,
      direction: prev.column === column ? nextSortDirection(prev.direction) : 'asc',
    }));
  }, []);

  const toggleTrialExpanded = useCallback((id: string) => {
    setExpandedTrials(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  // ---------------------------------------------------------------------------
  // Derived data for the selected entry
  // ---------------------------------------------------------------------------

  const displayMode: AutoTestSweepMode | null = selected ? selected.run.kind : null;

  const columns = useMemo<ColumnDef<AutoTestTrialResult>[]>(() =>
    displayMode === 'config' ? buildConfigColumns() : buildSamplingColumns(),
  [displayMode]);

  const columnById = useMemo(() =>
    Object.fromEntries(columns.map(c => [c.id, c])) as Record<string, ColumnDef<AutoTestTrialResult>>,
  [columns]);

  const getColumnValue = useCallback((row: AutoTestTrialResult, col: string): number | string | undefined =>
    columnById[col]?.getValue(row),
  [columnById]);

  const activeTrials: AutoTestTrialResult[] = selected
    ? (displayMode === 'config' ? selected.run.trials as ConfigTrialResult[] : selected.run.trials as AutoTestTrialResult[])
    : [];

  const sortedActiveTrials = useMemo(() => sortRows(activeTrials, sort, getColumnValue), [activeTrials, sort, getColumnValue]);

  const bestTrialId: string | null = selected?.run.bestTrialId ?? null;
  const bestTrial = displayMode === 'sampling' && bestTrialId
    ? activeTrials.find(t => t.id === bestTrialId) ?? null
    : null;
  const bestConfigTrial = displayMode === 'config' && bestTrialId
    ? (activeTrials as ConfigTrialResult[]).find(t => t.id === bestTrialId) ?? null
    : null;

  const scenarioLookup: Record<string, AutoTestScenario> = useMemo(() => {
    const lookup: Record<string, AutoTestScenario> = {};
    const primaryScenario = displayMode === 'config' ? configPerfScenario : chatScenario;
    lookup[primaryScenario.id] = primaryScenario;
    if (displayMode !== 'config') lookup[toolCallScenario.id] = toolCallScenario;
    return lookup;
  }, [displayMode]);

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  if (selected) {
    const hasBest = !!(displayMode === 'config' ? bestConfigTrial : bestTrial);
    return (
      <div className="playground-history-container">
        <button
          className="btn btn-small btn-secondary"
          style={{ marginBottom: 12 }}
          onClick={() => setSelectedId(null)}
        >
          ← Back to list
        </button>

        <div style={{ fontSize: 13, color: 'var(--color-gray-700)', marginBottom: 12 }}>
          <strong>{selected.modelId || 'Unknown model'}</strong>
          {' · '}
          {selected.sweepMode === 'config' ? 'Config Sweep' : 'Sampling Sweep'}
          {' · '}
          {selected.completedAt
            ? new Date(selected.completedAt).toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' }) +
              ', ' + new Date(selected.completedAt).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
            : '—'}
        </div>

        {/* Best Configuration Found */}
        <div className="playground-autotest-best">
          {hasBest ? (
            <>
              <h4>Best Configuration Found</h4>
              <div className="playground-autotest-best-details">
                {displayMode === 'config' && bestConfigTrial ? (
                  <>
                    <div><strong>Context Window:</strong> {bestConfigTrial.config?.['context_window'] ?? '—'}</div>
                    <div><strong>NBatch:</strong> {bestConfigTrial.config?.nbatch ?? '—'}</div>
                    <div><strong>NUBatch:</strong> {bestConfigTrial.config?.nubatch ?? '—'}</div>
                    <div><strong>NSeqMax:</strong> {bestConfigTrial.config?.['nseq_max'] ?? '—'}</div>
                    <div><strong>Flash Attention:</strong> {bestConfigTrial.config?.['flash_attention'] ?? '—'}</div>
                    <div><strong>KV Cache Type:</strong> {bestConfigTrial.config?.['cache_type'] ?? '—'}</div>
                    <div><strong>Cache Mode:</strong> {bestConfigTrial.config?.['cache_mode'] ? (bestConfigTrial.config['cache_mode'] === 'none' ? 'None' : bestConfigTrial.config['cache_mode'].toUpperCase()) : '—'}</div>
                  </>
                ) : bestTrial ? (
                  <>
                    <div><strong>Temperature:</strong> {bestTrial.candidate.temperature}</div>
                    <div><strong>Top P:</strong> {bestTrial.candidate.top_p}</div>
                    <div><strong>Top K:</strong> {bestTrial.candidate.top_k}</div>
                    <div><strong>Min P:</strong> {bestTrial.candidate.min_p}</div>
                  </>
                ) : null}
                {(() => {
                  const best = displayMode === 'config' ? bestConfigTrial : bestTrial;
                  if (!best) return null;
                  return <BestTrialMetrics trial={best} showScores={displayMode !== 'config'} />;
                })()}
              </div>
            </>
          ) : (
            <div style={{ fontSize: 12, color: 'var(--color-gray-700)' }}>
              No best configuration could be selected. Adjust weights and reevaluate.
            </div>
          )}

          {/* Best Configuration Criteria */}
          <details className="playground-sampling-params" style={{ marginTop: hasBest ? 12 : 0 }}>
            <summary>Best Configuration Criteria</summary>
            <p style={{ fontSize: 12, color: 'var(--color-gray-600)', marginBottom: 8 }}>
              Weights control how the best configuration is chosen. Higher weight = more influence.
            </p>
            <div style={{ display: 'flex', gap: 6, marginBottom: 10 }}>
              <button type="button" className="btn btn-small" onClick={() => applyPreset('overall')}>Best overall</button>
              <button type="button" className="btn btn-small" onClick={() => applyPreset('0pct')}>Best at 0% context</button>
              <button type="button" className="btn btn-small" onClick={() => applyPreset('80pct')}>Best at 80% context</button>
            </div>
            <div className="playground-sweep-params">
              {([
                ...(displayMode !== 'config' ? [
                  ['chatScore', 'Chat Score'],
                  ['toolScore', 'Tool Score'],
                  ['totalScore', 'Total Score'],
                ] : []),
                ['avgTPS', 'Avg TPS'],
                ['avgTTFT', 'Avg TTFT (lower is better)'],
                ['tps0', 'TPS @0% Fill'],
                ['tps20', 'TPS @20% Fill'],
                ['tps50', 'TPS @50% Fill'],
                ['tps80', 'TPS @80% Fill'],
                ['ttft0', 'TTFT @0% Fill (lower is better)'],
                ['ttft20', 'TTFT @20% Fill (lower is better)'],
                ['ttft50', 'TTFT @50% Fill (lower is better)'],
                ['ttft80', 'TTFT @80% Fill (lower is better)'],
              ] as [keyof BestConfigWeights, string][]).map(([key, label]) => (
                <div className="playground-sweep-param" key={key}>
                  <label className="playground-sweep-param-toggle" htmlFor={`history-weight-${key}`}>{label}</label>
                  <input
                    id={`history-weight-${key}`}
                    type="number"
                    className="playground-sweep-param-values"
                    value={weights[key]}
                    onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n) && n >= 0) handleWeightChange(key, n); }}
                    step={0.1}
                    min={0}
                  />
                </div>
              ))}
            </div>
            {weights.totalScore > 0 && (weights.chatScore > 0 || weights.toolScore > 0) && (
              <p style={{ fontSize: 11, color: 'var(--color-warning-text)', marginTop: 6 }}>
                ⚠ Total Score is derived from Chat/Tool weights. Weighting Total Score alongside Chat or Tool Score will double-count quality.
              </p>
            )}
            <button
              className="btn btn-primary btn-small"
              style={{ marginTop: 8 }}
              onClick={handleReevaluate}
              disabled={!weightsChanged}
            >
              Reevaluate
            </button>
          </details>
        </div>

        {/* Results Table */}
        {activeTrials.length > 0 && (
          <details className="playground-autotest-results" open>
            <summary style={{ cursor: 'pointer', fontWeight: 600, fontSize: 13, color: 'var(--color-gray-700)', marginBottom: 8 }}>
              Results ({activeTrials.length} trials)
              {(() => {
                const startMs = selected.run.runStartedAt ? Date.parse(selected.run.runStartedAt) : NaN;
                const finishTimes = activeTrials
                  .map((t: AutoTestTrialResult) => t?.finishedAt ? Date.parse(t.finishedAt) : NaN)
                  .filter(Number.isFinite) as number[];
                const endMs = finishTimes.length > 0 ? Math.max(...finishTimes) : NaN;
                if (!Number.isFinite(startMs) || !Number.isFinite(endMs)) return null;
                const totalRuntime = formatMs(Math.max(0, endMs - startMs));
                const finishedDate = new Date(endMs);
                const finishedStr = finishedDate.toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' }) +
                  ', ' + finishedDate.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit', second: '2-digit' });
                return (
                  <span style={{ fontWeight: 400, fontSize: 12, color: 'var(--color-gray-500)', marginLeft: 8 }}>
                    — {totalRuntime} total · finished {finishedStr}
                  </span>
                );
              })()}
            </summary>
            <div className="playground-autotest-table-scroll">
              <table className="playground-autotest-table">
                <thead>
                  <tr>
                    <th>#</th>
                    {columns.map(c => (
                      <th key={c.id} className={c.sortable !== false ? 'sortable-th' : undefined} onClick={c.sortable !== false ? () => handleSort(c.id) : undefined}>
                        {c.title}{sortIndicator(c.id, sort)}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {sortedActiveTrials.map((trial, i) => {
                    const isPending = trial.status === 'queued' || trial.status === 'running';
                    const isInProgress = false;
                    const bestTrialForMode = displayMode === 'config' ? bestConfigTrial : bestTrial;
                    const isBest = bestTrialForMode && trial === bestTrialForMode;
                    const isExpanded = expandedTrials.has(trial.id);
                    const meta: CellMeta = { isPending, isInProgress, isBest: !!isBest, index: i };
                    return (
                      <React.Fragment key={trial.id}>
                        <tr
                          className={`autotest-trial-row${isBest ? ' autotest-best-row' : ''}${trial.status === 'skipped' ? ' autotest-skipped-row' : ''}`}
                          style={{ cursor: 'pointer' }}
                          onClick={() => toggleTrialExpanded(trial.id)}
                        >
                          <td>{isExpanded ? '▾' : '▸'} {i + 1}</td>
                          {columns.map(c => (
                            <td key={c.id}>{c.renderCell(trial, meta)}</td>
                          ))}
                        </tr>
                        {isExpanded && (
                          <tr className="autotest-detail-row">
                            <td colSpan={columns.length + 1}>
                              <TrialDetails trial={trial} scenarioLookup={scenarioLookup} hideScores={displayMode === 'config'} />
                            </td>
                          </tr>
                        )}
                      </React.Fragment>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </details>
        )}
      </div>
    );
  }

  // ---------------------------------------------------------------------------
  // History list (no entry selected)
  // ---------------------------------------------------------------------------

  if (loading) {
    return (
      <div className="playground-history-container">
        <div style={{ fontSize: 13, color: 'var(--color-gray-500)', padding: 16 }}>Loading history…</div>
      </div>
    );
  }

  return (
    <div className="playground-history-container">
      <div className="playground-history-list">
        <div className="playground-history-header">
          <h3>Test Run History</h3>
          {entries.length > 0 && (
            <>
              <input
                ref={selectAllRef}
                type="checkbox"
                checked={selectedIds.size === entries.length && entries.length > 0}
                onChange={() => {
                  if (selectedIds.size === entries.length) {
                    setSelectedIds(new Set());
                  } else {
                    setSelectedIds(new Set(entries.map(e => e.id)));
                  }
                }}
                style={{ marginLeft: 8 }}
              />
              <span style={{ fontSize: 12, color: 'var(--color-gray-500)' }}>
                {entries.length} run{entries.length !== 1 ? 's' : ''}
              </span>
            </>
          )}
          {selectedIds.size > 0 && (
            <button
              className="btn btn-small btn-danger"
              style={{ marginLeft: 8 }}
              onClick={handleDeleteSelected}
            >
              Delete Selected ({selectedIds.size})
            </button>
          )}
          <button
            className="btn btn-small btn-secondary"
            style={{ marginLeft: 8 }}
            onClick={() => { void handleExport() }}
          >
            Export
          </button>
          <button
            className="btn btn-small btn-secondary"
            style={{ marginLeft: 4 }}
            onClick={() => fileInputRef.current?.click()}
          >
            Import
          </button>
          <input
            ref={fileInputRef}
            type="file"
            accept=".json"
            style={{ display: 'none' }}
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) void handleImport(file);
              e.target.value = '';
            }}
          />
          {importMessage && (
            <span style={{ fontSize: 12, color: 'var(--color-gray-600)', marginLeft: 8 }}>
              {importMessage}
            </span>
          )}
        </div>
        {entries.length === 0 ? (
          <div className="playground-history-empty">
            No test runs saved yet. Complete an automated test run and it will appear here.
          </div>
        ) : (
          entries.map(entry => (
            <div
              key={entry.id}
              className={`playground-history-item ${selectedId === entry.id ? 'active' : ''}`}
              onClick={() => handleSelectEntry(entry.id)}
            >
              <input
                type="checkbox"
                checked={selectedIds.has(entry.id)}
                onClick={(e) => e.stopPropagation()}
                onChange={() => {
                  setSelectedIds(prev => {
                    const next = new Set(prev);
                    if (next.has(entry.id)) next.delete(entry.id);
                    else next.add(entry.id);
                    return next;
                  });
                }}
              />
              <div className="playground-history-item-content">
                <div className="playground-history-item-model">{entry.modelId || 'Unknown model'}</div>
                <div className="playground-history-item-meta">
                  <span className="playground-history-item-mode">
                    {entry.sweepMode === 'config' ? 'Config Sweep' : 'Sampling Sweep'}
                  </span>
                  <span className="playground-history-item-date">
                    {entry.completedAt ? new Date(entry.completedAt).toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' }) + ', ' + new Date(entry.completedAt).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' }) : '—'}
                  </span>
                </div>
                <div className="playground-history-item-stats">
                  {entry.run.trials?.length ?? 0} trials
                </div>
              </div>
              <button
                className="btn btn-small btn-danger playground-history-delete"
                onClick={(e) => { e.stopPropagation(); handleDelete(entry.id); }}
              >
                Delete
              </button>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
