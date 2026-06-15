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
type SortBy = 'date' | 'model' | 'prompt';
type CompareLayout = 'columns' | 'stacked';

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
            <select value="custom" disabled>
              <option value="custom">Custom</option>
            </select>
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

  const handleSave = () => {
    if (!state?.result) return;
    saveRun(model, prompt, state.result);
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  return (
    <div className="accuracy-compare-card">
      <div className="accuracy-compare-card-head">
        <span className="accuracy-compare-model" title={model}>{model}</span>
        {state?.result && status === 'done' && (
          <button className="btn btn-secondary btn-sm efficiency-run-btn" onClick={handleSave}>
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
            run again
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
  const [sortBy, setSortBy] = useState<SortBy>('date');
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
    switch (sortBy) {
      case 'model':
        return a.model.localeCompare(b.model);
      case 'prompt':
        return a.prompt.localeCompare(b.prompt);
      default:
        return b.savedAt - a.savedAt;
    }
  });

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
          <label>
            Sort by:{' '}
            <select value={sortBy} onChange={(e) => setSortBy(e.target.value as SortBy)}>
              <option value="date">Date</option>
              <option value="model">Model</option>
              <option value="prompt">Prompt</option>
            </select>
          </label>
          <button
            className="btn btn-primary btn-sm"
            onClick={handleCompare}
            disabled={selectedIds.size < 2}
          >
            Compare Selected ({selectedIds.size})
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
          {sorted.map((e) => (
            <label key={e.id} className="playground-history-item">
              <input
                type="checkbox"
                checked={selectedIds.has(e.id)}
                onChange={() => toggleSelect(e.id)}
              />
              <div className="playground-history-item-content">
                <div className="playground-history-item-model">{e.model}</div>
                <div className="efficiency-history-prompt">"{e.prompt}"</div>
                <div className="efficiency-history-stats">
                  {fmtSeconds(e.result.usage.wallclock_ms)} wall · {e.result.usage.prompt_tokens} in ·{' '}
                  {e.result.usage.completion_tokens} out · {fmtTPS(e.result.usage.out_tps)} tps
                  <span className="efficiency-history-date">
                    {new Date(e.savedAt).toLocaleString([], {
                      month: 'short',
                      day: 'numeric',
                      hour: 'numeric',
                      minute: '2-digit',
                    })}
                  </span>
                </div>
              </div>
            </label>
          ))}
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
          Compare how fast models run the same prompt — prefill/output tokens-per-second,
          time-to-first-token, and wall clock. Model load and a warm-up pass are excluded so
          the numbers reflect steady-state throughput.
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
