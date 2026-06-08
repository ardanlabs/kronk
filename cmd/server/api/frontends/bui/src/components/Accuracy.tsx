import { useEffect, useMemo, useRef, useState } from 'react';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import type { AccuracyFunction, AccuracyResponse } from '../types';

type Mode = 'manual' | 'batch';
type SortBy = 'line' | 'loc' | 'name';

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

// A batch row tracks a single function's run lifecycle.
interface BatchRow {
  identifier: string;
  line: number;
  loc: number;
  status: 'pending' | 'running' | 'done' | 'error';
  result?: AccuracyResponse;
  error?: string;
}

const DEFAULT_RANDOM = 5;
const MAX_RANDOM = 5;

// isChatModel filters out embedding/rerank models, matching the Chat app.
function isChatModel(id: string): boolean {
  const lid = id.toLowerCase();
  return !lid.includes('embed') && !lid.includes('rerank');
}

// matchClass colors a percentage: green (strong), amber (partial), red (weak).
function matchClass(pct: number): string {
  if (pct >= 90) return 'accuracy-match-good';
  if (pct >= 50) return 'accuracy-match-mid';
  return 'accuracy-match-bad';
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
  const { models, loading: modelsLoading, loadModels } = useModelList();

  const [mode, setMode] = useState<Mode>('manual');
  const [selectedModel, setSelectedModel] = useState('');

  const [functions, setFunctions] = useState<AccuracyFunction[] | null>(null);
  const [sortBy, setSortBy] = useState<SortBy>('line');
  const [error, setError] = useState<string | null>(null);

  // Manual mode state.
  const [selectedFn, setSelectedFn] = useState('');
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<AccuracyResponse | null>(null);

  // Batch mode state.
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [randomCount, setRandomCount] = useState(DEFAULT_RANDOM);
  const [rows, setRows] = useState<BatchRow[]>([]);
  const [batchRunning, setBatchRunning] = useState(false);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const stopRef = useRef(false);

  // Load models on mount.
  useEffect(() => {
    loadModels();
  }, [loadModels]);

  // Default to the first chat-capable model (same rule as the Chat app).
  useEffect(() => {
    if (models?.data && models.data.length > 0) {
      const chatModels = models.data.filter((m) => isChatModel(m.id));
      const valid = chatModels.some((m) => m.id === selectedModel);
      if (!valid && chatModels.length > 0) {
        setSelectedModel(chatModels[0].id);
      }
    }
  }, [models, selectedModel]);

  // Switching models resets the run state in both modes.
  useEffect(() => {
    setResult(null);
    setRows([]);
    setExpanded(new Set());
    setChecked(new Set());
    stopRef.current = true;
  }, [selectedModel]);

  // Load the fixed function list on mount.
  useEffect(() => {
    let cancelled = false;
    api
      .listAccuracyFunctions()
      .then((resp) => {
        if (cancelled) return;
        setFunctions(resp.data);
        if (resp.data.length > 0) setSelectedFn(resp.data[0].identifier);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : 'Failed to read functions');
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // ── Manual run ──

  async function runManual() {
    if (!selectedModel || !selectedFn) return;
    setRunning(true);
    setError(null);
    setResult(null);
    try {
      const resp = await api.runAccuracy(selectedModel, selectedFn);
      setResult(resp);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Test failed');
    } finally {
      setRunning(false);
    }
  }

  // clearManual clears the current manual result.
  function clearManual() {
    setResult(null);
    setError(null);
  }

  // ── Batch selection ──

  function toggle(identifier: string) {
    setChecked((prev) => {
      const next = new Set(prev);
      if (next.has(identifier)) next.delete(identifier);
      else next.add(identifier);
      return next;
    });
  }

  // clearAll clears the selection and any results (like Manual's Clear).
  function clearAll() {
    stopRef.current = true;
    setChecked(new Set());
    setRows([]);
    setExpanded(new Set());
  }

  function pickRandom() {
    const all = (functions ?? []).map((f) => f.identifier);
    const n = Math.max(1, Math.min(randomCount, MAX_RANDOM, all.length));
    // Fisher–Yates shuffle, then take the first n.
    const shuffled = [...all];
    for (let i = shuffled.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [shuffled[i], shuffled[j]] = [shuffled[j], shuffled[i]];
    }
    setChecked(new Set(shuffled.slice(0, n)));
  }

  // ── Batch run (sequential, client-side loop) ──

  async function runBatch() {
    if (!selectedModel || checked.size === 0) return;

    const targets = (functions ?? []).filter((f) => checked.has(f.identifier));

    setBatchRunning(true);
    setError(null);
    setExpanded(new Set());
    stopRef.current = false;

    const initial: BatchRow[] = targets.map((f) => ({
      identifier: f.identifier,
      line: f.line,
      loc: f.loc,
      status: 'pending',
    }));
    setRows(initial);

    for (let i = 0; i < targets.length; i++) {
      if (stopRef.current) break;
      const fn = targets[i].identifier;

      setRows((prev) =>
        prev.map((r) => (r.identifier === fn ? { ...r, status: 'running' } : r)),
      );

      try {
        const resp = await api.runAccuracy(selectedModel, fn);
        setRows((prev) =>
          prev.map((r) =>
            r.identifier === fn ? { ...r, status: 'done', result: resp } : r,
          ),
        );
      } catch (err) {
        setRows((prev) =>
          prev.map((r) =>
            r.identifier === fn
              ? { ...r, status: 'error', error: err instanceof Error ? err.message : 'failed' }
              : r,
          ),
        );
      }
    }

    setBatchRunning(false);
  }

  function stopBatch() {
    stopRef.current = true;
  }

  function toggleExpand(identifier: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(identifier)) next.delete(identifier);
      else next.add(identifier);
      return next;
    });
  }

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

  return (
    <div className="accuracy-page">
      <div className="page-header">
        <h2>Accuracy</h2>
        <p>
          Test how accurately a model recalls source code. Pick a model and a function,
          then compare the model's output to the real source — in single or batch mode.
        </p>
      </div>

      {/* Setup: model dropdown (Chat-style) */}
      <div className="card accuracy-setup">
        <div className="form-group">
          <label>Model</label>
          <select
            value={selectedModel}
            onChange={(e) => setSelectedModel(e.target.value)}
            disabled={modelsLoading}
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

      {error && <div className="accuracy-error">{error}</div>}

      {functions && functions.length > 0 && (
        <>
          {/* Mode toggle */}
          <div className="accuracy-mode-toggle">
            <button
              className={`btn ${mode === 'manual' ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => setMode('manual')}
            >
              Manual
            </button>
            <button
              className={`btn ${mode === 'batch' ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => setMode('batch')}
            >
              Batch
            </button>
            <span className="accuracy-count">{functions.length} functions</span>
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
          </div>

          {/* Manual mode */}
          {mode === 'manual' && (
            <div className="card">
              <div className="form-row">
                <div className="form-group" style={{ flex: 1 }}>
                  <label>Function</label>
                  <select value={selectedFn} onChange={(e) => setSelectedFn(e.target.value)}>
                    {sortedFunctions.map((f) => (
                      <option key={f.identifier} value={f.identifier}>
                        {f.identifier} ({f.loc} loc) — line {f.line}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="form-group" style={{ alignSelf: 'flex-end' }}>
                  <button
                    className="btn btn-primary"
                    onClick={runManual}
                    disabled={running || !selectedModel || !selectedFn}
                  >
                    {running ? 'Running…' : 'Run test'}
                  </button>
                </div>
                <div className="form-group" style={{ alignSelf: 'flex-end' }}>
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
                    onChange={(e) => setRandomCount(Number(e.target.value))}
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
                      disabled={!selectedModel || checked.size === 0}
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
              <div className="accuracy-func-grid">
                {sortedFunctions.map((f) => (
                  <label key={f.identifier} className="accuracy-func-item">
                    <input
                      type="checkbox"
                      checked={checked.has(f.identifier)}
                      onChange={() => toggle(f.identifier)}
                      disabled={batchRunning}
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
