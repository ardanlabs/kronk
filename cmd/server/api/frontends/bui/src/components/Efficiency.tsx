import { useState, useEffect, useCallback } from 'react';
import { useModelList } from '../contexts/ModelListContext';
import {
  useEfficiencyRunner,
  isChatModel,
  MAX_MODELS,
  type RunState,
  type RunStatus,
} from '../contexts/EfficiencyRunnerContext';
import {
  loadHistory,
  saveRun,
  deleteEntries,
  subscribe,
  type EfficiencyHistoryEntry,
} from '../services/efficiencyHistory';
import type { EfficiencyResponse } from '../types';
import CodeBlock from './CodeBlock';

type Tab = 'manual' | 'history';
type SortKey = 'date' | 'model' | 'prompt' | 'out_tps' | 'ttft' | 'wall' | 'tokens';
type SortDir = 'asc' | 'desc';
type CompareLayout = 'columns' | 'stacked';

// Default sort direction per column: rates/dates start high→low, names and
// time-based metrics (where smaller is better) start low→high.
const SORT_DEFAULT_DIR: Record<SortKey, SortDir> = {
  date: 'desc',
  model: 'asc',
  prompt: 'asc',
  out_tps: 'desc',
  ttft: 'asc',
  wall: 'asc',
  tokens: 'desc',
};

// compareEntries orders two runs by the given column.
function compareEntries(
  a: EfficiencyHistoryEntry,
  b: EfficiencyHistoryEntry,
  key: SortKey,
): number {
  switch (key) {
    case 'model':
      return a.model.localeCompare(b.model);
    case 'prompt':
      return a.prompt.localeCompare(b.prompt);
    case 'out_tps':
      return a.result.usage.out_tps - b.result.usage.out_tps;
    case 'ttft':
      return a.result.usage.ttft_ms - b.result.usage.ttft_ms;
    case 'wall':
      return a.result.usage.wallclock_ms - b.result.usage.wallclock_ms;
    case 'tokens':
      return a.result.usage.completion_tokens - b.result.usage.completion_tokens;
    default:
      return a.savedAt - b.savedAt;
  }
}

// ManualRow is one completed Manual run shown in the summary table.
interface ManualRow {
  model: string;
  result: EfficiencyResponse;
}

// compareManual orders two completed Manual runs by the given column.
function compareManual(a: ManualRow, b: ManualRow, key: SortKey): number {
  switch (key) {
    case 'model':
      return a.model.localeCompare(b.model);
    case 'ttft':
      return a.result.usage.ttft_ms - b.result.usage.ttft_ms;
    case 'wall':
      return a.result.usage.wallclock_ms - b.result.usage.wallclock_ms;
    case 'tokens':
      return a.result.usage.completion_tokens - b.result.usage.completion_tokens;
    case 'out_tps':
    default:
      return a.result.usage.out_tps - b.result.usage.out_tps;
  }
}

// Max saved runs that can be compared at once. Comparison is only readable with
// a handful of cards; this caps it (read-only, so it's looser than MAX_MODELS).
const MAX_COMPARE = 4;

// RankBy selects which axis decides the "best" model in a comparison. Wall clock
// alone is misleading because it scales with how many tokens a model happened to
// generate; output tokens/sec measures raw decode speed independent of answer
// length, which is the fairest single efficiency measure.
type RankBy = 'out_tps' | 'ttft' | 'wall';

const RANK_LABELS: Record<RankBy, string> = {
  out_tps: 'output speed (tps)',
  ttft: 'first token (ttft)',
  wall: 'wall clock',
};

// bestIdFor returns the id of the winning entry for a given metric.
function bestIdFor(entries: EfficiencyHistoryEntry[], rankBy: RankBy): string | null {
  if (entries.length < 2) return null;
  const sorted = [...entries].sort((a, b) => {
    const ua = a.result.usage;
    const ub = b.result.usage;
    switch (rankBy) {
      case 'out_tps':
        return ub.out_tps - ua.out_tps;
      case 'ttft':
        return ua.ttft_ms - ub.ttft_ms;
      case 'wall':
        return ua.wallclock_ms - ub.wallclock_ms;
    }
  });
  return sorted[0].id;
}

// fmtSeconds renders milliseconds as seconds with one decimal.
function fmtSeconds(ms: number): string {
  return `${(ms / 1000).toFixed(1)}s`;
}

function fmtTPS(tps: number): string {
  return tps.toFixed(1);
}

// statusLabel maps a run status to its display text + dot class.
function statusLabel(status: RunStatus): { text: string; cls: string } {
  switch (status) {
    case 'loading':
      return { text: 'loading model…', cls: 'loading' };
    case 'running':
      return { text: 'running…', cls: 'running' };
    case 'done':
      return { text: 'done', cls: 'done' };
    case 'error':
      return { text: 'error', cls: 'error' };
    default:
      return { text: 'idle', cls: 'idle' };
  }
}

// UsageMetrics renders the throughput block shared by Manual cards and the
// History compare cards. When `bests` is provided (compare view) the metrics
// that this card wins are highlighted, so you can see at a glance which model
// leads on each axis rather than only the single overall winner.
function UsageMetrics({
  usage,
  bests,
}: {
  usage: EfficiencyResponse['usage'];
  bests?: Partial<Record<RankBy, boolean>>;
}) {
  const cls = (k: RankBy) =>
    bests?.[k] ? 'efficiency-metric efficiency-metric-best' : 'efficiency-metric';

  return (
    <div className="efficiency-metrics">
      <div>{usage.prompt_tokens} in / {usage.completion_tokens} out</div>
      <div className={cls('out_tps')}>output: {fmtTPS(usage.out_tps)} tps</div>
      <div className="efficiency-metric">prefill: {fmtTPS(usage.in_tps)} tps</div>
      <div className={cls('ttft')}>ttft: {usage.ttft_ms.toFixed(0)} ms</div>
      <div className={cls('wall')} title="Generation time only — excludes model load and prefill">
        wall clock: {fmtSeconds(usage.wallclock_ms)}
      </div>
    </div>
  );
}

// =============================================================================
// Manual tab

function ManualTab() {
  const { models } = useModelList();
  const {
    selectedModels,
    toggleModel,
    prompt,
    setPrompt,
    maxTokens,
    setMaxTokens,
    clear,
    runs,
    runModel,
    runAll,
    stopActive,
    isRunning,
  } = useEfficiencyRunner();

  const chatModels = (models?.data ?? []).filter((m) => isChatModel(m.id));
  const selected = Array.from(selectedModels);

  // Sortable summary of completed runs for quick scanning.
  const [sortKey, setSortKey] = useState<SortKey>('out_tps');
  const [sortDir, setSortDir] = useState<SortDir>('desc');

  const completed: ManualRow[] = selected
    .map((model) => ({ model, state: runs.get(model) }))
    .filter((r) => r.state?.status === 'done' && r.state.result)
    .map((r) => ({ model: r.model, result: r.state!.result! }));

  const sortedSummary = [...completed].sort((a, b) => {
    const cmp = compareManual(a, b, sortKey);
    return sortDir === 'asc' ? cmp : -cmp;
  });

  const handleSort = (key: SortKey) => {
    if (key === sortKey) setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    else {
      setSortKey(key);
      setSortDir(SORT_DEFAULT_DIR[key]);
    }
  };

  const sortIndicator = (key: SortKey) => {
    const active = key === sortKey;
    const symbol = active ? (sortDir === 'asc' ? '▲' : '▼') : '⇅';
    return <span className={`efficiency-sort-ind${active ? ' active' : ''}`}>{symbol}</span>;
  };

  return (
    <>
      <div className="accuracy-label-row">
        <label>Select Models:</label>
        <span className="efficiency-count">
          {selectedModels.size}/{MAX_MODELS} selected
        </span>
      </div>
      <div className="accuracy-func-grid" style={{ marginBottom: '1.25rem' }}>
        {chatModels.length === 0 && <div className="efficiency-muted">No models available</div>}
        {chatModels.map((m) => {
          const checked = selectedModels.has(m.id);
          const atCap = !checked && selectedModels.size >= MAX_MODELS;
          return (
            <label
              key={m.id}
              className="accuracy-func-item"
              style={atCap ? { opacity: 0.5 } : undefined}
            >
              <input
                type="checkbox"
                checked={checked}
                onChange={() => toggleModel(m.id)}
                disabled={isRunning || atCap}
              />
              <span className="accuracy-func-name" title={m.id}>{m.id}</span>
            </label>
          );
        })}
      </div>

      <div className="form-group">
        <div className="efficiency-section-head">
          <label>Choose Prompt:</label>
          <div className="efficiency-prompt-controls">
            <button className="btn btn-secondary btn-sm" onClick={clear} disabled={isRunning}>
              clear
            </button>
          </div>
        </div>
        <textarea
          className="efficiency-prompt"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder="Write a hello world program in Go."
          rows={3}
          disabled={isRunning}
        />
        <div className="efficiency-maxtokens">
          <label>max_tokens</label>
          <input
            type="number"
            min={1}
            value={maxTokens}
            onChange={(e) => setMaxTokens(Math.max(1, Number(e.target.value) || 1))}
            disabled={isRunning}
          />
          {!isRunning ? (
            <button
              className="btn btn-primary efficiency-run-all"
              onClick={runAll}
              disabled={selected.length === 0 || !prompt.trim()}
            >
              Run all
            </button>
          ) : (
            <button className="btn btn-danger efficiency-run-all" onClick={stopActive}>
              Stop
            </button>
          )}
        </div>
      </div>

      {sortedSummary.length > 0 && (
        <div className="form-group">
          <label>Summary</label>
          <div className="efficiency-history-list">
            <table className="efficiency-history-table">
              <thead>
                <tr>
                  <th onClick={() => handleSort('model')}>
                    Model{sortIndicator('model')}
                  </th>
                  <th
                    className="efficiency-th-num"
                    onClick={() => handleSort('out_tps')}
                  >
                    Output tps{sortIndicator('out_tps')}
                  </th>
                  <th
                    className="efficiency-th-num"
                    onClick={() => handleSort('ttft')}
                  >
                    TTFT{sortIndicator('ttft')}
                  </th>
                  <th
                    className="efficiency-th-num"
                    onClick={() => handleSort('wall')}
                  >
                    Wall{sortIndicator('wall')}
                  </th>
                  <th
                    className="efficiency-th-num"
                    onClick={() => handleSort('tokens')}
                  >
                    In/Out{sortIndicator('tokens')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {sortedSummary.map(({ model, result }) => {
                  const u = result.usage;
                  return (
                    <tr key={model}>
                      <td className="efficiency-cell-model" title={model}>{model}</td>
                      <td className="efficiency-th-num">{fmtTPS(u.out_tps)}</td>
                      <td className="efficiency-th-num">{u.ttft_ms.toFixed(0)} ms</td>
                      <td className="efficiency-th-num">{fmtSeconds(u.wallclock_ms)}</td>
                      <td className="efficiency-th-num">
                        {u.prompt_tokens}/{u.completion_tokens}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {selected.length > 0 && (
        <div className="form-group">
          <label>Results</label>
          <div className="accuracy-compare-grid">
            {selected.map((model) => (
              <ManualResultCard
                key={model}
                model={model}
                state={runs.get(model)}
                onRun={() => runModel(model)}
                onStop={stopActive}
                prompt={prompt}
                disabled={isRunning}
              />
            ))}
          </div>
        </div>
      )}
    </>
  );
}

function ManualResultCard({
  model,
  state,
  onRun,
  onStop,
  prompt,
  disabled,
}: {
  model: string;
  state: RunState | undefined;
  onRun: () => void;
  onStop: () => void;
  prompt: string;
  disabled: boolean;
}) {
  const status = state?.status ?? 'idle';
  const { text, cls } = statusLabel(status);
  const inFlight = status === 'loading' || status === 'running';
  const [saved, setSaved] = useState(false);

  // Reset the saved flag whenever a new result arrives so a fresh run can be
  // saved again; otherwise it stays "saved ✓" permanently.
  useEffect(() => {
    setSaved(false);
  }, [state?.result]);

  const handleSave = () => {
    if (!state?.result || saved) return;
    // Use the prompt the result was actually measured with, not the live
    // textarea, which may have been edited since the run.
    saveRun(model, state.result.prompt, state.result);
    setSaved(true);
  };

  return (
    <div className="accuracy-compare-card">
      <div className="accuracy-compare-card-head">
        <span className="accuracy-compare-model" title={model}>{model}</span>
        {state?.result && status === 'done' && (
          <button
            className="btn btn-secondary btn-sm efficiency-run-btn"
            onClick={handleSave}
            disabled={saved}
          >
            {saved ? 'saved ✓' : 'save to history'}
          </button>
        )}
      </div>
      <div className="efficiency-status-row">
        <span className={`efficiency-dot ${cls}`}>●</span>
        <span className="efficiency-status-text">{text}</span>
        {inFlight ? (
          <button className="btn btn-danger btn-sm efficiency-run-btn" onClick={onStop}>
            stop
          </button>
        ) : (
          <button
            className="btn btn-secondary btn-sm efficiency-run-btn"
            onClick={onRun}
            disabled={disabled || !prompt.trim()}
          >
            {state?.result ? 'run again' : 'run'}
          </button>
        )}
      </div>

      {status === 'error' && <div className="accuracy-error-cell">{state?.error}</div>}

      {state?.result && status === 'done' && (
        <>
          <UsageMetrics usage={state.result.usage} />
          <div className="efficiency-output-label">Output</div>
          <CodeBlock code={state.result.output || '(empty)'} language="go" collapsible />
        </>
      )}
    </div>
  );
}

// =============================================================================
// History tab

function HistoryTab() {
  const [entries, setEntries] = useState<EfficiencyHistoryEntry[]>(() => loadHistory());
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [sortKey, setSortKey] = useState<SortKey>('date');
  const [sortDir, setSortDir] = useState<SortDir>('desc');
  const [comparing, setComparing] = useState<EfficiencyHistoryEntry[] | null>(null);
  const [layout, setLayout] = useState<CompareLayout>('columns');
  const [rankBy, setRankBy] = useState<RankBy>('out_tps');

  // Keep in sync with storage (this tab and others).
  useEffect(() => subscribe(() => setEntries(loadHistory())), []);

  // Drop selections / comparisons for entries that no longer exist.
  useEffect(() => {
    const ids = new Set(entries.map((e) => e.id));
    setSelectedIds((prev) => {
      const next = new Set(Array.from(prev).filter((id) => ids.has(id)));
      return next.size === prev.size ? prev : next;
    });
    setComparing((prev) => (prev ? prev.filter((e) => ids.has(e.id)) : prev));
  }, [entries]);

  const sorted = [...entries].sort((a, b) => {
    const cmp = compareEntries(a, b, sortKey);
    return sortDir === 'asc' ? cmp : -cmp;
  });

  // handleSort toggles direction when re-clicking a column, else switches to the
  // new column with its sensible default direction.
  const handleSort = (key: SortKey) => {
    if (key === sortKey) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortKey(key);
      setSortDir(SORT_DEFAULT_DIR[key]);
    }
  };

  // A faint ⇅ on every sortable header signals it's clickable; the active column
  // shows a solid ▲/▼ for the current direction.
  const sortIndicator = (key: SortKey) => {
    const active = key === sortKey;
    const symbol = active ? (sortDir === 'asc' ? '▲' : '▼') : '⇅';
    return <span className={`efficiency-sort-ind${active ? ' active' : ''}`}>{symbol}</span>;
  };

  // Select-all toggles every visible run (handy for bulk delete; compare is
  // still gated to MAX_COMPARE on its own button).
  const allSelected = sorted.length > 0 && sorted.every((e) => selectedIds.has(e.id));
  const toggleSelectAll = () => {
    setSelectedIds(allSelected ? new Set() : new Set(sorted.map((e) => e.id)));
  };

  // Selection is uncapped so any number of runs can be deleted at once. The
  // MAX_COMPARE limit is enforced only on the Compare action below.
  const toggleSelect = useCallback((id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const handleCompare = () => {
    setComparing(entries.filter((e) => selectedIds.has(e.id)));
  };

  const handleDelete = () => {
    deleteEntries(selectedIds);
    setSelectedIds(new Set());
  };

  // The winner is decided by the user-selected metric, and only that metric is
  // highlighted — highlighting every axis would be noise when you're ranking by
  // one of them.
  const winnerId = comparing ? bestIdFor(comparing, rankBy) : null;

  return (
    <>
      <div className="efficiency-history-toolbar">
        <label>Saved Runs</label>
        <div className="efficiency-history-controls">
          <button
            className="btn btn-primary btn-sm"
            onClick={handleCompare}
            disabled={selectedIds.size < 2 || selectedIds.size > MAX_COMPARE}
            title={`Select 2–${MAX_COMPARE} runs to compare`}
          >
            Compare Selected ({selectedIds.size}/{MAX_COMPARE})
          </button>
          <button
            className="btn btn-danger btn-sm"
            onClick={handleDelete}
            disabled={selectedIds.size === 0}
          >
            Delete Selected ({selectedIds.size})
          </button>
        </div>
      </div>

      {sorted.length === 0 ? (
        <div className="efficiency-muted" style={{ padding: '1rem' }}>
          No saved runs yet. Run a model in the Manual tab and click "save to history".
        </div>
      ) : (
        <div className="efficiency-history-list">
          <table className="efficiency-history-table">
            <thead>
              <tr>
                <th className="efficiency-th-check">
                  <input
                    type="checkbox"
                    checked={allSelected}
                    onChange={toggleSelectAll}
                    title="Select all"
                  />
                </th>
                <th onClick={() => handleSort('model')}>
                  Model{sortIndicator('model')}
                </th>
                <th onClick={() => handleSort('prompt')}>
                  Prompt{sortIndicator('prompt')}
                </th>
                <th
                  className="efficiency-th-num"
                  onClick={() => handleSort('out_tps')}
                >
                  Output tps{sortIndicator('out_tps')}
                </th>
                <th
                  className="efficiency-th-num"
                  onClick={() => handleSort('ttft')}
                >
                  TTFT{sortIndicator('ttft')}
                </th>
                <th
                  className="efficiency-th-num"
                  onClick={() => handleSort('wall')}
                >
                  Wall{sortIndicator('wall')}
                </th>
                <th
                  className="efficiency-th-num"
                  onClick={() => handleSort('tokens')}
                >
                  In/Out{sortIndicator('tokens')}
                </th>
                <th
                  className="efficiency-th-num"
                  onClick={() => handleSort('date')}
                >
                  Saved{sortIndicator('date')}
                </th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((e) => {
                const u = e.result.usage;
                return (
                  <tr
                    key={e.id}
                    className={selectedIds.has(e.id) ? 'efficiency-row-selected' : undefined}
                    onClick={() => toggleSelect(e.id)}
                  >
                    <td className="efficiency-th-check">
                      <input
                        type="checkbox"
                        checked={selectedIds.has(e.id)}
                        onChange={() => toggleSelect(e.id)}
                        onClick={(ev) => ev.stopPropagation()}
                      />
                    </td>
                    <td className="efficiency-cell-model" title={e.model}>{e.model}</td>
                    <td className="efficiency-cell-prompt" title={e.prompt}>{e.prompt}</td>
                    <td className="efficiency-th-num">{fmtTPS(u.out_tps)}</td>
                    <td className="efficiency-th-num">{u.ttft_ms.toFixed(0)} ms</td>
                    <td className="efficiency-th-num">{fmtSeconds(u.wallclock_ms)}</td>
                    <td className="efficiency-th-num">
                      {u.prompt_tokens}/{u.completion_tokens}
                    </td>
                    <td className="efficiency-th-num efficiency-cell-date">
                      {new Date(e.savedAt).toLocaleString([], {
                        month: 'short',
                        day: 'numeric',
                        hour: 'numeric',
                        minute: '2-digit',
                      })}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {comparing && comparing.length >= 2 && (
        <div className="form-group" style={{ marginTop: '1.5rem' }}>
          <div className="accuracy-compare-toolbar">
            <label>Comparison</label>
            <div className="accuracy-compare-layout">
              <label className="efficiency-rankby">
                Best by:{' '}
                <select value={rankBy} onChange={(e) => setRankBy(e.target.value as RankBy)}>
                  <option value="out_tps">Output speed (tps)</option>
                  <option value="ttft">First token (ttft)</option>
                  <option value="wall">Wall clock</option>
                </select>
              </label>
              <button
                className={`btn btn-sm ${layout === 'columns' ? 'btn-primary' : 'btn-secondary'}`}
                onClick={() => setLayout('columns')}
              >
                side by side
              </button>
              <button
                className={`btn btn-sm ${layout === 'stacked' ? 'btn-primary' : 'btn-secondary'}`}
                onClick={() => setLayout('stacked')}
              >
                stacked
              </button>
              <button
                className="btn btn-secondary btn-sm"
                onClick={() => {
                  setComparing(null);
                  setSelectedIds(new Set());
                }}
              >
                Clear
              </button>
            </div>
          </div>
          <div className={`accuracy-compare-grid${layout === 'stacked' ? ' stacked' : ''}`}>
            {comparing.map((e) => (
              <div
                key={e.id}
                className={`accuracy-compare-card${e.id === winnerId ? ' accuracy-compare-winner' : ''}`}
              >
                <div className="accuracy-compare-card-head">
                  <span className="accuracy-compare-model" title={e.model}>{e.model}</span>
                  {e.id === winnerId && (
                    <span className="accuracy-compare-badge">best · {RANK_LABELS[rankBy]}</span>
                  )}
                </div>
                <div className="efficiency-history-prompt">Prompt: "{e.prompt}"</div>
                <UsageMetrics
                  usage={e.result.usage}
                  bests={e.id === winnerId ? { [rankBy]: true } : undefined}
                />
                <div className="efficiency-output-label">Output</div>
                <CodeBlock code={e.result.output || '(empty)'} language="go" collapsible />
              </div>
            ))}
          </div>
        </div>
      )}
    </>
  );
}

// =============================================================================
// Page

export default function Efficiency() {
  const [tab, setTab] = useState<Tab>('manual');

  return (
    <div className="accuracy-page">
      <div className="page-header">
        <h2>Efficiency</h2>
        <p>
          Measure how fast a model runs a prompt — tokens-per-second, time-to-first-token, and
          wall clock (model load and warm-up excluded). Save runs to History to compare across
          models or prompts.
        </p>
      </div>

      <div className="tabs">
        <button className={`tab ${tab === 'manual' ? 'active' : ''}`} onClick={() => setTab('manual')}>
          Manual
        </button>
        <button className={`tab ${tab === 'history' ? 'active' : ''}`} onClick={() => setTab('history')}>
          History
        </button>
      </div>

      <div className="card">{tab === 'manual' ? <ManualTab /> : <HistoryTab />}</div>
    </div>
  );
}
