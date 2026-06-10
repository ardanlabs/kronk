import { useMemo } from 'react';
import { useModelList } from '../contexts/ModelListContext';
import {
  useAccuracyRunner,
  isChatModel,
  MAX_COMPARE,
  MAX_RANDOM,
  type BatchRow,
  type SortBy,
} from '../contexts/AccuracyRunnerContext';
import type { AccuracyFunction, AccuracyResponse } from '../types';

// sortFunctions returns a copy of the list ordered by the chosen field.
function sortFunctions(list: AccuracyFunction[], by: SortBy): AccuracyFunction[] {
  const out = [...list];
  out.sort((a, b) => {
    switch (by) {
      case 'loc':
        return a.loc - b.loc || a.line - b.line;
      case 'name':
        return a.identifier.localeCompare(b.identifier);
      default:
        return a.line - b.line;
    }
  });
  return out;
}

// matchClass colors a percentage: green (strong), amber (partial), red (weak).
function matchClass(pct: number): string {
  if (pct >= 90) return 'accuracy-match-good';
  if (pct >= 50) return 'accuracy-match-mid';
  return 'accuracy-match-bad';
}

// rankResult orders two compare results best-first: higher match %, then
// faster (higher tps), then fewer completion tokens. Returns 0 only when all
// three are identical (a true tie).
function rankResult(a: AccuracyResponse, b: AccuracyResponse): number {
  if (a.match_percent !== b.match_percent) return b.match_percent - a.match_percent;
  if (a.usage.tokens_per_second !== b.usage.tokens_per_second) {
    return b.usage.tokens_per_second - a.usage.tokens_per_second;
  }
  return a.usage.completion_tokens - b.usage.completion_tokens;
}

// DiffView renders the structured (-got +want) diff returned by the server,
// with a title and a key explaining what the + and - lines mean.
function DiffView({ result, title = 'Code Difference' }: { result: AccuracyResponse; title?: string }) {
  return (
    <div className="accuracy-diff-wrap">
      <div className="accuracy-diff-header">
        <span className="accuracy-diff-title">{title}</span>
        <span className="accuracy-diff-key">
          <span className="accuracy-diff-key-item accuracy-diff-del">- got (model output)</span>
          <span className="accuracy-diff-key-item accuracy-diff-add">+ want (actual source)</span>
        </span>
      </div>
      <div className="accuracy-diff">
        <pre>
          {result.diff.map((d, i) => (
            <div key={i} className={`accuracy-diff-line accuracy-diff-${d.op}`}>
              <span className="accuracy-diff-gutter">
                {d.op === 'add' ? '+' : d.op === 'del' ? '-' : ' '}
              </span>
              <span className="accuracy-diff-text">{d.text || ' '}</span>
            </div>
          ))}
        </pre>
      </div>
    </div>
  );
}

export default function Accuracy() {
  const { models, loading: modelsLoading } = useModelList();

  // All run state and logic lives in AccuracyRunnerContext so an in-progress
  // run (and its results) survives navigating to other pages.
  const {
    mode,
    setMode,
    selectedModel,
    setSelectedModel,
    functions,
    sortBy,
    setSortBy,
    error,
    selectedFn,
    setSelectedFn,
    running,
    result,
    runManual,
    stopManual,
    clearManual,
    checked,
    setChecked,
    randomCount,
    setRandomCount,
    rows,
    batchRunning,
    expanded,
    toggle,
    clearAll,
    pickRandom,
    runBatch,
    stopBatch,
    toggleExpand,
    compareModels,
    compareFn,
    setCompareFn,
    compareCells,
    comparing,
    compareLayout,
    setCompareLayout,
    toggleCompareModel,
    clearCompare,
    runCompare,
    stopCompare,
    isRunning,
  } = useAccuracyRunner();

  // ── Derived ──

  const sortedFunctions = useMemo(
    () => (functions ? sortFunctions(functions, sortBy) : []),
    [functions, sortBy],
  );

  const done = rows.filter((r) => r.status === 'done' || r.status === 'error');
  const completed = rows.filter((r) => r.status === 'done' && r.result);
  const avgMatch = useMemo(() => {
    if (completed.length === 0) return null;
    const sum = completed.reduce((acc, r) => acc + (r.result?.match_percent ?? 0), 0);
    return sum / completed.length;
  }, [completed]);

  const compareDoneCount = compareCells.filter(
    (c) => c.status === 'done' || c.status === 'error',
  ).length;
  const compareFnInfo = sortedFunctions.find((f) => f.identifier === compareFn);

  // compareOutcome decides the winner(s) by match %, then speed (tps), then
  // conciseness (completion tokens). When models are identical on all three
  // it reports a tie (co-winners) instead of arbitrarily picking one, and it
  // explains which criterion separated a clear winner from the runner-up.
  const compareOutcome = useMemo(() => {
    const finished = compareCells.filter((c) => c.status === 'done' && c.result);
    if (finished.length < 2) return null;

    const sorted = [...finished].sort((x, y) => rankResult(x.result!, y.result!));
    const top = sorted[0].result!;

    const winners = sorted
      .filter((c) => rankResult(c.result!, top) === 0)
      .map((c) => c.model);

    if (winners.length > 1) {
      return { winners, tied: true, reason: 'identical match, speed & length' };
    }

    const runnerUp = sorted[1].result!;
    let reason: string;
    if (top.match_percent > runnerUp.match_percent) {
      reason = 'highest match';
    } else if (top.usage.tokens_per_second > runnerUp.usage.tokens_per_second) {
      reason = 'fastest';
    } else {
      reason = 'fewest tokens';
    }
    return { winners, tied: false, reason };
  }, [compareCells]);

  // functionsMeta is the "N functions · Sort by" control shown inline next to
  // each mode's selection label. Only one mode renders at a time, so the
  // select id stays unique.
  const functionsMeta = (
    <span className="accuracy-functions-meta">
      <span className="accuracy-count">{functions?.length ?? 0} functions</span>
      <span className="accuracy-sort">
        <label htmlFor="accuracy-sort">Sort by:</label>
        <select
          id="accuracy-sort"
          value={sortBy}
          onChange={(e) => setSortBy(e.target.value as SortBy)}
        >
          <option value="line">Line number</option>
          <option value="loc">Lines of Code</option>
          <option value="name">Name</option>
        </select>
      </span>
    </span>
  );

  return (
    <div className="accuracy-page">
      <div className="page-header">
        <h2>Accuracy</h2>
        <p>
          Test how accurately a model recalls source code. Pick a model and a function,
          then compare the model's output to the real source — in single, batch, or
          compare mode.
        </p>
      </div>

      {/* Mode tabs */}
      <div className="tabs">
        <button
          className={`tab ${mode === 'manual' ? 'active' : ''}`}
          onClick={() => setMode('manual')}
        >
          Manual
        </button>
        <button
          className={`tab ${mode === 'batch' ? 'active' : ''}`}
          onClick={() => setMode('batch')}
        >
          Batch
        </button>
        <button
          className={`tab ${mode === 'compare' ? 'active' : ''}`}
          onClick={() => setMode('compare')}
        >
          Compare
        </button>
      </div>

      {/* Setup: model dropdown (Chat-style). Compare mode picks its own
          set of models, so the single dropdown is hidden there. */}
      {mode !== 'compare' && (
        <div className="card accuracy-setup">
          <div className="form-group">
            <label>Model</label>
            <select
              value={selectedModel}
              onChange={(e) => setSelectedModel(e.target.value)}
              disabled={modelsLoading || isRunning}
            >
              {modelsLoading && <option>Loading models...</option>}
              {!modelsLoading && models?.data?.filter((m) => isChatModel(m.id)).length === 0 && (
                <option>No models available</option>
              )}
              {models?.data
                ?.filter((m) => isChatModel(m.id))
                .map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.id}
                  </option>
                ))}
            </select>
          </div>
        </div>
      )}

      {error && <div className="accuracy-error">{error}</div>}

      {functions && functions.length > 0 && (
        <>
          {/* Manual mode */}
          {mode === 'manual' && (
            <div className="card">
              <div className="form-row">
                <div className="form-group" style={{ flex: 1 }}>
                  <div className="accuracy-label-row">
                    <label>Select a Function:</label>
                    {functionsMeta}
                  </div>
                  <select
                    value={selectedFn}
                    onChange={(e) => setSelectedFn(e.target.value)}
                    disabled={running}
                  >
                    {sortedFunctions.map((f) => (
                      <option key={f.identifier} value={f.identifier}>
                        {f.identifier} ({f.loc} loc) — line {f.line}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="form-group" style={{ alignSelf: 'flex-end', display: 'flex', flexDirection: 'row', gap: '0.5rem' }}>
                  {!running ? (
                    <button
                      className="btn btn-primary"
                      onClick={runManual}
                      disabled={!selectedModel || !selectedFn || isRunning}
                    >
                      Run test
                    </button>
                  ) : (
                    <button className="btn btn-danger" onClick={stopManual}>
                      Stop
                    </button>
                  )}
                  <button
                    className="btn btn-secondary"
                    onClick={clearManual}
                    disabled={running || (!result && !error)}
                  >
                    Clear
                  </button>
                </div>
              </div>

              {result && (
                <div className="accuracy-result">
                  <div className="accuracy-result-top">
                    <span className={`accuracy-match ${matchClass(result.match_percent)}`}>
                      {result.match_percent.toFixed(2)}% match
                    </span>
                    <span className="accuracy-meta">
                      {result.model} · {result.usage.prompt_tokens} in /{' '}
                      {result.usage.completion_tokens} out · {result.usage.tokens_per_second.toFixed(1)} tps
                    </span>
                  </div>
                  <div className="accuracy-fields">
                    <span>Function: <strong>{result.function} ({result.want.split('\n').length} loc)</strong></span>
                    <span>line: {result.line}</span>
                  </div>
                  <DiffView result={result} />
                </div>
              )}
            </div>
          )}

          {/* Batch mode */}
          {mode === 'batch' && (
            <div className="card">
              <div className="accuracy-batch-controls">
                <span className="accuracy-random">
                  <select
                    value={randomCount}
                    onChange={(e) => {
                      setRandomCount(Number(e.target.value));
                      // Changing the count starts a fresh selection.
                      setChecked(new Set());
                    }}
                    disabled={batchRunning}
                    style={{ width: '4rem' }}
                  >
                    {Array.from({ length: MAX_RANDOM }, (_, i) => i + 1).map((n) => (
                      <option key={n} value={n}>
                        {n}
                      </option>
                    ))}
                  </select>
                  <button className="btn btn-secondary btn-sm" onClick={pickRandom} disabled={batchRunning}>
                    Pick random
                  </button>
                </span>
                <button className="btn btn-secondary btn-sm" onClick={clearAll} disabled={batchRunning}>
                  Clear
                </button>
                <span className="accuracy-count">{checked.size} selected</span>
                <span className="accuracy-batch-run">
                  {!batchRunning ? (
                    <button
                      className="btn btn-primary"
                      onClick={runBatch}
                      disabled={!selectedModel || checked.size === 0 || isRunning}
                    >
                      Run {checked.size || ''} tests
                    </button>
                  ) : (
                    <button className="btn btn-danger" onClick={stopBatch}>
                      Stop ({done.length}/{rows.length})
                    </button>
                  )}
                </span>
              </div>

              {/* Function picker */}
              <div className="accuracy-label-row">
                <label>Select Functions:</label>
                {functionsMeta}
              </div>
              <div className="accuracy-func-grid">
                {sortedFunctions.map((f) => (
                  <label key={f.identifier} className="accuracy-func-item">
                    <input
                      type="checkbox"
                      checked={checked.has(f.identifier)}
                      onChange={() => toggle(f.identifier)}
                      disabled={
                        batchRunning ||
                        (!checked.has(f.identifier) && checked.size >= MAX_RANDOM)
                      }
                    />
                    <span className="accuracy-func-name" title={f.identifier}>{f.identifier}</span>
                    <span className="accuracy-func-loc">({f.loc} loc)</span>
                    <span className="accuracy-func-line">line {f.line}</span>
                  </label>
                ))}
              </div>

              {/* Results */}
              {rows.length > 0 && (
                <div className="accuracy-batch-results">
                  {avgMatch !== null && (
                    <div className="accuracy-batch-summary">
                      <span className={`accuracy-match ${matchClass(avgMatch)}`}>
                        {avgMatch.toFixed(2)}% average
                      </span>
                      <span className="accuracy-meta">
                        {completed.length}/{rows.length} completed · {selectedModel}
                      </span>
                    </div>
                  )}
                  <div className="table-container">
                    <table>
                      <thead>
                        <tr>
                          <th>Function</th>
                          <th>Line</th>
                          <th>Match</th>
                          <th>Tokens</th>
                          <th title="Click a row to expand its code difference">Code Diff ▸</th>
                        </tr>
                      </thead>
                      <tbody>
                        {rows.map((r) => (
                          <BatchResultRow
                            key={r.identifier}
                            row={r}
                            expanded={expanded.has(r.identifier)}
                            onToggle={() => toggleExpand(r.identifier)}
                          />
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Compare mode */}
          {mode === 'compare' && (
            <div className="card">
              <div className="form-row">
                <div className="form-group" style={{ flex: 1 }}>
                  <div className="accuracy-label-row">
                    <label>Select a Function:</label>
                    {functionsMeta}
                  </div>
                  <select
                    value={compareFn}
                    onChange={(e) => setCompareFn(e.target.value)}
                    disabled={comparing}
                  >
                    {sortedFunctions.map((f) => (
                      <option key={f.identifier} value={f.identifier}>
                        {f.identifier} ({f.loc} loc) — line {f.line}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="form-group" style={{ alignSelf: 'flex-end', display: 'flex', flexDirection: 'row', gap: '0.5rem' }}>
                  {!comparing ? (
                    <button
                      className="btn btn-primary"
                      onClick={runCompare}
                      disabled={compareModels.size < 2 || !compareFn || isRunning}
                    >
                      Compare {compareModels.size || ''} models
                    </button>
                  ) : (
                    <button className="btn btn-danger" onClick={stopCompare}>
                      Stop ({compareDoneCount}/{compareCells.length})
                    </button>
                  )}
                  <button
                    className="btn btn-secondary"
                    onClick={clearCompare}
                    disabled={comparing}
                  >
                    Clear
                  </button>
                </div>
              </div>

              {/* Model picker (exactly MAX_COMPARE) */}
              <label className="accuracy-compare-label">
                Models — pick {MAX_COMPARE} ({compareModels.size} selected)
              </label>
              <div className="accuracy-func-grid">
                {models?.data
                  ?.filter((m) => isChatModel(m.id))
                  .map((m) => (
                    <label key={m.id} className="accuracy-func-item">
                      <input
                        type="checkbox"
                        checked={compareModels.has(m.id)}
                        onChange={() => toggleCompareModel(m.id)}
                        disabled={comparing}
                      />
                      <span className="accuracy-func-name" title={m.id}>{m.id}</span>
                    </label>
                  ))}
              </div>

              {/* Results: shared function header + one card per model */}
              {compareCells.length > 0 && (
                <div className="accuracy-compare-results">
                  <div className="accuracy-compare-toolbar">
                    <div className="accuracy-fields">
                      <span>
                        Function:{' '}
                        <strong>
                          {compareFn}
                          {compareFnInfo ? ` (${compareFnInfo.loc} loc)` : ''}
                        </strong>
                      </span>
                      {compareFnInfo && <span>line: {compareFnInfo.line}</span>}
                    </div>
                    <div className="accuracy-compare-layout">
                      <button
                        className={`btn btn-sm ${compareLayout === 'columns' ? 'btn-primary' : 'btn-secondary'}`}
                        onClick={() => setCompareLayout('columns')}
                      >
                        Side by side
                      </button>
                      <button
                        className={`btn btn-sm ${compareLayout === 'stacked' ? 'btn-primary' : 'btn-secondary'}`}
                        onClick={() => setCompareLayout('stacked')}
                      >
                        Stacked
                      </button>
                    </div>
                  </div>
                  <div className={`accuracy-compare-grid${compareLayout === 'stacked' ? ' stacked' : ''}`}>
                    {compareCells.map((c) => {
                      const isWinner = compareOutcome?.winners.includes(c.model) ?? false;
                      const tied = compareOutcome?.tied ?? false;
                      return (
                      <div
                        key={c.model}
                        className={`accuracy-compare-card${
                          isWinner
                            ? tied
                              ? ' accuracy-compare-tied'
                              : ' accuracy-compare-winner'
                            : ''
                        }`}
                      >
                        <div className="accuracy-compare-card-head">
                          <span className="accuracy-compare-model" title={c.model}>
                            {c.model}
                          </span>
                          {isWinner && (
                            <span
                              className={`accuracy-compare-badge${
                                tied ? ' accuracy-compare-badge-tied' : ''
                              }`}
                              title={
                                tied
                                  ? 'Tied — identical match %, tokens/sec, and completion tokens'
                                  : `Best — won on ${compareOutcome!.reason}`
                              }
                            >
                              {tied ? 'Tied' : `Best · ${compareOutcome!.reason}`}
                            </span>
                          )}
                        </div>
                        {c.status === 'done' && c.result ? (
                          <>
                            <div className="accuracy-result-top">
                              <span className={`accuracy-match ${matchClass(c.result.match_percent)}`}>
                                {c.result.match_percent.toFixed(2)}% match
                              </span>
                              <span className="accuracy-meta">
                                {c.result.usage.prompt_tokens} in /{' '}
                                {c.result.usage.completion_tokens} out ·{' '}
                                {c.result.usage.tokens_per_second.toFixed(1)} tps
                              </span>
                            </div>
                            <DiffView result={c.result} />
                          </>
                        ) : c.status === 'running' ? (
                          <div className="accuracy-meta">running…</div>
                        ) : c.status === 'error' ? (
                          <div className="accuracy-error-cell">{c.error}</div>
                        ) : (
                          <div className="accuracy-meta">pending</div>
                        )}
                      </div>
                      );
                    })}
                  </div>
                </div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function BatchResultRow({
  row,
  expanded,
  onToggle,
}: {
  row: BatchRow;
  expanded: boolean;
  onToggle: () => void;
}) {
  const canExpand = row.status === 'done' && !!row.result;

  return (
    <>
      <tr className={canExpand ? 'accuracy-row-clickable' : ''} onClick={canExpand ? onToggle : undefined}>
        <td>{row.identifier} ({row.loc} loc)</td>
        <td>{row.line}</td>
        <td>
          {row.status === 'done' && row.result ? (
            <span className={`accuracy-match ${matchClass(row.result.match_percent)}`}>
              {row.result.match_percent.toFixed(2)}%
            </span>
          ) : row.status === 'running' ? (
            <span className="accuracy-meta">running…</span>
          ) : row.status === 'error' ? (
            <span className="accuracy-match accuracy-match-bad">error</span>
          ) : (
            <span className="accuracy-meta">pending</span>
          )}
        </td>
        <td>
          {row.result
            ? `${row.result.usage.prompt_tokens}/${row.result.usage.completion_tokens}`
            : '—'}
        </td>
        <td>
          {canExpand && (
            <button
              className="btn btn-secondary btn-sm accuracy-diff-toggle"
              onClick={(e) => {
                e.stopPropagation();
                onToggle();
              }}
            >
              {expanded ? '▾ Hide' : '▸ Show'}
            </button>
          )}
        </td>
      </tr>
      {expanded && row.result && (
        <tr>
          <td colSpan={5}>
            <DiffView result={row.result} title={`Code Difference — ${row.identifier}`} />
          </td>
        </tr>
      )}
      {row.status === 'error' && (
        <tr>
          <td colSpan={5} className="accuracy-error-cell">{row.error}</td>
        </tr>
      )}
    </>
  );
}
