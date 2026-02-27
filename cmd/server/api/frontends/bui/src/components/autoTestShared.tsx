import { useState, useCallback, useEffect, useRef } from 'react';
import type {
  AutoTestTrialResult,
  AutoTestScenarioResult,
  AutoTestPromptResult,
  AutoTestActivePrompt,
  AutoTestScenario,
  AutoTestScenarioID,
} from '../types';
import { formatMs, scoreColorSafe, formatScore, getScenarioScore, FILL_LEVELS } from '../services/sweepModeColumns';

// ---------------------------------------------------------------------------
// Sort utilities
// ---------------------------------------------------------------------------

export type SortDirection = 'asc' | 'desc' | null;

export interface SortState {
  column: string | null;
  direction: SortDirection;
}

export function nextSortDirection(current: SortDirection): SortDirection {
  if (current === null) return 'asc';
  if (current === 'asc') return 'desc';
  return null;
}

export function sortIndicator(column: string, sort: SortState): string {
  if (sort.column !== column || sort.direction === null) return '';
  return sort.direction === 'asc' ? ' ▲' : ' ▼';
}

export function sortRows<T extends AutoTestTrialResult>(
  rows: T[],
  sort: SortState,
  getValue: (row: T, col: string) => number | string | undefined,
): T[] {
  if (!sort.column || !sort.direction) return rows;
  const dir = sort.direction === 'asc' ? 1 : -1;
  return [...rows].sort((a, b) => {
    const va = getValue(a, sort.column!);
    const vb = getValue(b, sort.column!);
    const na = va == null ? undefined : va;
    const nb = vb == null ? undefined : vb;
    if (na === undefined && nb === undefined) return 0;
    if (na === undefined) return 1;
    if (nb === undefined) return -1;
    if (typeof na === 'number' && typeof nb === 'number') return (na - nb) * dir;
    return String(na).localeCompare(String(nb)) * dir;
  });
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

export function estimatePromptDurationMs(usage: AutoTestPromptResult['usage']): number | undefined {
  if (!usage) return undefined;
  const out = usage.output_tokens;
  const tps = usage.tokens_per_second;
  if (!Number.isFinite(out) || !Number.isFinite(tps) || tps <= 0) return undefined;
  const ttftMs = usage.time_to_first_token_ms ?? 0;
  const genMs = (out / tps) * 1000;
  const total = genMs + ttftMs;
  if (!Number.isFinite(total) || total < 0) return undefined;
  return total;
}

export function formatLogTime(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

export function formatRunningLabel(ap: AutoTestActivePrompt, scenarioLookup: Record<string, AutoTestScenario>): string {
  const scenarioName = scenarioLookup[ap.scenarioId]?.name ?? ap.scenarioId;
  const totalPrompts = scenarioLookup[ap.scenarioId]?.prompts?.length;
  let label = `${scenarioName} · Prompt ${ap.promptIndex + 1}${totalPrompts ? `/${totalPrompts}` : ''}`;
  if (ap.repeats && ap.repeats > 1 && ap.repeatIndex != null) {
    label += ` · Repeat ${ap.repeatIndex}/${ap.repeats}`;
  }
  return label;
}

// ---------------------------------------------------------------------------
// BestTrialMetrics component
// ---------------------------------------------------------------------------

export function BestTrialMetrics({ trial, showScores }: { trial: AutoTestTrialResult; showScores: boolean }) {
  return (
    <>
      {showScores && (
        <>
          <div><strong>Chat Score:</strong> {formatScore(getScenarioScore(trial, 'chat'))}</div>
          <div><strong>Tool Score:</strong> {formatScore(getScenarioScore(trial, 'tool_call'))}</div>
          <div><strong>Total Score:</strong> {formatScore(trial.totalScore)}</div>
        </>
      )}
      <div><strong>Avg TPS:</strong> {trial.avgTPS?.toFixed(1) ?? '—'}</div>
      <div><strong>Avg TTFT:</strong> {trial.avgTTFT !== undefined ? formatMs(trial.avgTTFT) : '—'}</div>
      {trial.avgTPSByFill && FILL_LEVELS.map(level => (
        <div key={`tps-${level}`}><strong>TPS @{level}:</strong> {trial.avgTPSByFill![level]?.toFixed(1) ?? '—'}</div>
      ))}
      {trial.avgTTFTByFill && FILL_LEVELS.map(level => (
        <div key={`ttft-${level}`}><strong>TTFT @{level}:</strong> {trial.avgTTFTByFill![level] !== undefined ? formatMs(trial.avgTTFTByFill![level]) : '—'}</div>
      ))}
    </>
  );
}

// ---------------------------------------------------------------------------
// ScenarioHeader component
// ---------------------------------------------------------------------------

export interface ScenarioHeaderProps {
  sr: AutoTestScenarioResult;
  scenarioName: string;
  hideScores?: boolean;
}

export function ScenarioHeader({ sr, scenarioName, hideScores }: ScenarioHeaderProps) {
  return (
    <div className="autotest-detail-scenario-header">
      <span className="autotest-detail-scenario-name">{scenarioName}</span>
      {!hideScores && (
        <span className="autotest-detail-scenario-score" style={{ color: scoreColorSafe(sr.score) }}>
          Score: {formatScore(sr.score)}
        </span>
      )}
      {sr.avgTPS !== undefined && <span>TPS: {sr.avgTPS.toFixed(1)}</span>}
      {sr.avgTTFT !== undefined && <span>TTFT: {formatMs(sr.avgTTFT)}</span>}
      {sr.avgTPSByFill && Object.keys(sr.avgTPSByFill).length > 0 && (
        <span style={{ marginLeft: 8, opacity: 0.85 }}>
          Context Fill TPS:
          {FILL_LEVELS.map(level => sr.avgTPSByFill![level] !== undefined ? ` @${level}: ${sr.avgTPSByFill![level].toFixed(1)}` : null)}
        </span>
      )}
      {sr.avgTTFTByFill && Object.keys(sr.avgTTFTByFill).length > 0 && (
        <span style={{ marginLeft: 8, opacity: 0.85 }}>
          Context Fill TTFT:
          {FILL_LEVELS.map(level => sr.avgTTFTByFill![level] !== undefined ? ` @${level}: ${formatMs(sr.avgTTFTByFill![level])}` : null)}
        </span>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// TrialDetails component
// ---------------------------------------------------------------------------

export interface TrialDetailsProps {
  trial: AutoTestTrialResult;
  scenarioLookup: Record<string, AutoTestScenario>;
  hideScores?: boolean;
}

export function TrialDetails({ trial, scenarioLookup, hideScores }: TrialDetailsProps) {
  const [expandedPrompts, setExpandedPrompts] = useState<Set<string>>(() => new Set());
  const togglePromptExpanded = useCallback((key: string) => {
    setExpandedPrompts(prev => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);
  const logBodyRef = useRef<HTMLDivElement>(null);
  const [logExpanded, setLogExpanded] = useState(false);
  const [scenariosExpanded, setScenariosExpanded] = useState(false);
  const hasActive = trial.status === 'running' && trial.activePrompts && trial.activePrompts.length > 0;
  const hasLogs = trial.logEntries && trial.logEntries.length > 0;

  useEffect(() => {
    const el = logBodyRef.current;
    if (!el || !logExpanded || trial.status !== 'running') return;
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    if (nearBottom) el.scrollTop = el.scrollHeight;
  }, [trial.logEntries?.length, trial.status, logExpanded]);

  if (trial.scenarioResults.length === 0 && !hasActive && !hasLogs) {
    return <div className="autotest-detail-empty">No scenario results yet.</div>;
  }

  return (
    <div className="autotest-detail-content">
      {hasActive && (
          <div className="autotest-detail-active">
            <div className="autotest-detail-active-header">
              <span className="playground-autotest-spinner-inline" /> Currently Running
            </div>
            {trial.activePrompts!.map((ap) => (
                <div key={`${ap.scenarioId}-${ap.promptIndex}`} className="autotest-detail-active-prompt">
                  <span className="autotest-detail-active-scenario">{scenarioLookup[ap.scenarioId]?.name ?? ap.scenarioId}</span>
                  <span className="autotest-detail-active-id">{ap.promptId}</span>
                  {ap.repeats && ap.repeats > 1 && ap.repeatIndex && (
                      <span className="autotest-detail-active-repeat">Repeat {ap.repeatIndex}/{ap.repeats}</span>
                  )}
                  {ap.preview && <div className="autotest-detail-active-preview">{ap.preview}</div>}
                </div>
            ))}
          </div>
      )}
      {hasLogs && (
        <div className="autotest-detail-log">
          <button type="button" className="autotest-detail-log-header" aria-expanded={logExpanded} onClick={e => { e.stopPropagation(); setLogExpanded(v => !v); }}>
            {trial.status === 'running' && <span className="playground-autotest-spinner-inline" />}
            <span>{logExpanded ? '▾' : '▸'} Activity Log</span>
          </button>
          <div className="autotest-detail-log-body" ref={logBodyRef}>
            {logExpanded
              ? trial.logEntries!.map((entry, idx) => (
                  <div key={idx} className="autotest-detail-log-entry">
                    <span className="autotest-detail-log-time">{formatLogTime(entry.timestamp)}</span>
                    <span className="autotest-detail-log-msg">{entry.message}</span>
                  </div>
                ))
              : (() => { const last = trial.logEntries![trial.logEntries!.length - 1]; return (
                  <div className="autotest-detail-log-entry">
                    <span className="autotest-detail-log-time">{formatLogTime(last.timestamp)}</span>
                    <span className="autotest-detail-log-msg">{last.message}</span>
                  </div>
                ); })()
            }
          </div>
        </div>
      )}
      {(trial.scenarioResults.length > 0 || (trial.status === 'running' && trial.activePrompts && trial.activePrompts.length > 0)) && (
        <div className="autotest-detail-scenarios-section">
          <button type="button" className="autotest-detail-log-header" aria-expanded={scenariosExpanded} onClick={e => { e.stopPropagation(); setScenariosExpanded(v => !v); }}>
            <span>{scenariosExpanded ? '▾' : '▸'} Performance</span>
          </button>
          {!scenariosExpanded ? (
            <div className="autotest-detail-scenarios-summary">
              {trial.status === 'running' && trial.activePrompts && trial.activePrompts.length > 0 && (
                <div className="autotest-detail-running-indicator">
                  <span className="playground-autotest-spinner-inline" />{' '}
                  Running: {formatRunningLabel(trial.activePrompts[0], scenarioLookup)}
                </div>
              )}
              {trial.scenarioResults.map((sr) => (
                <ScenarioHeader key={sr.scenarioId} sr={sr} scenarioName={scenarioLookup[sr.scenarioId]?.name ?? sr.scenarioId} hideScores={hideScores} />
              ))}
            </div>
          ) : (
            (() => {
              const renderedIds = new Set(trial.scenarioResults.map(sr => sr.scenarioId));
              const activeByScenario: Record<string, AutoTestActivePrompt[]> = {};
              if (trial.status === 'running' && trial.activePrompts) {
                for (const ap of trial.activePrompts) {
                  if (!activeByScenario[ap.scenarioId]) activeByScenario[ap.scenarioId] = [];
                  activeByScenario[ap.scenarioId].push(ap);
                }
              }
              const scenarioIds = [...renderedIds];
              for (const sid of Object.keys(activeByScenario)) {
                const id = sid as AutoTestScenarioID;
                if (!renderedIds.has(id)) scenarioIds.push(id);
              }

              return scenarioIds.map((scenarioId) => {
                const sr = trial.scenarioResults.find(s => s.scenarioId === scenarioId);
                const scenario = scenarioLookup[scenarioId];
                const activeForScenario = activeByScenario[scenarioId] ?? [];

                if (!sr && activeForScenario.length === 0) return null;

                return (
                  <div key={scenarioId} className="autotest-detail-scenario">
                    {sr ? (
                      <ScenarioHeader sr={sr} scenarioName={scenario?.name ?? scenarioId} hideScores={hideScores} />
                    ) : (
                      <div className="autotest-detail-scenario-header">
                        <span className="autotest-detail-scenario-name">{scenario?.name ?? scenarioId}</span>
                      </div>
                    )}
                    {activeForScenario.length > 0 && (
                      <div className="autotest-detail-running-indicator autotest-detail-running-indicator--expanded">
                        <span className="playground-autotest-spinner-inline" />{' '}
                        Running: {formatRunningLabel(activeForScenario[0], scenarioLookup)}
                      </div>
                    )}
                    {sr && (
                      <div className="autotest-detail-prompts">
                        {sr.promptResults.map((pr) => {
                          const promptKey = `${sr.scenarioId}:${pr.promptId}`;
                          const isPromptExpanded = expandedPrompts.has(promptKey);
                          const isCtxFill = pr.promptId.startsWith('ctxfill-');
                          const promptDef = scenario?.prompts.find(p => p.id === pr.promptId);
                          const lastUserMsg = promptDef?.messages
                            .filter(m => m.role === 'user')
                            .pop();
                          const inputText = isCtxFill
                            ? `Context fill test (${pr.promptId.replace('ctxfill-', '')}% fill) — ${pr.usage?.prompt_tokens ?? '?'} prompt tokens`
                            : typeof lastUserMsg?.content === 'string'
                              ? lastUserMsg.content
                              : lastUserMsg?.content?.map(p => ('text' in p ? p.text : '')).join('') ?? '';
                          const expectedLabel = isCtxFill
                            ? 'Performance metric only'
                            : promptDef?.expected
                              ? promptDef.expected.type === 'tool_call'
                                ? 'Tool call'
                                : promptDef.expected.type === 'no_tool_call'
                                  ? 'No tool call' + (promptDef.expected.value ? ` (text: ${promptDef.expected.value})` : '')
                                  : promptDef.expected.type === 'exact'
                                    ? `Exact: "${promptDef.expected.value}"`
                                    : `Regex: ${promptDef.expected.value}`
                              : '—';
                          const durMs = estimatePromptDurationMs(pr.usage);
                          const durationLabel = durMs !== undefined ? formatMs(durMs) : '—';

                          return (
                            <div key={pr.promptId} className={`autotest-detail-prompt ${isPromptExpanded ? 'expanded' : 'collapsed'}`}>
                              <div
                                className="autotest-detail-prompt-header autotest-detail-prompt-header--clickable"
                                role="button"
                                tabIndex={0}
                                aria-expanded={isPromptExpanded}
                                onClick={() => togglePromptExpanded(promptKey)}
                                onKeyDown={(e) => {
                                  if (e.key === 'Enter' || e.key === ' ') {
                                    e.preventDefault();
                                    togglePromptExpanded(promptKey);
                                  }
                                }}
                              >
                                <div className="autotest-detail-prompt-summary">
                                  <span className="autotest-detail-prompt-toggle">{isPromptExpanded ? '▼' : '▶'}</span>
                                  <span className="autotest-detail-prompt-id">{isCtxFill ? `Context Fill @${pr.promptId.replace('ctxfill-', '')}%` : pr.promptId}</span>
                                  <span className="autotest-detail-prompt-duration">Duration: {durationLabel}</span>
                                  <span className="autotest-detail-prompt-usage-inline">
                                    <span>In: {pr.usage?.prompt_tokens ?? '—'}</span>
                                    <span>Out: {pr.usage?.output_tokens ?? '—'}</span>
                                    <span>TPS: {pr.usage?.tokens_per_second !== undefined ? pr.usage.tokens_per_second.toFixed(1) : '—'}</span>
                                  </span>
                                </div>
                                {!hideScores && (
                                  <span className="autotest-detail-prompt-score" style={{ color: scoreColorSafe(pr.score) }}>
                                    {formatScore(pr.score)}
                                  </span>
                                )}
                              </div>
                              {isPromptExpanded && (
                              <div className="autotest-detail-prompt-grid">
                                <div className="autotest-detail-field">
                                  <div className="autotest-detail-label">Input</div>
                                  <div className="autotest-detail-value">{inputText || '—'}</div>
                                </div>
                                <div className="autotest-detail-field">
                                  <div className="autotest-detail-label">Expected</div>
                                  <div className="autotest-detail-value">{expectedLabel}</div>
                                </div>
                                <div className="autotest-detail-field">
                                  <div className="autotest-detail-label">Output</div>
                                  <div className="autotest-detail-value">
                                    {pr.toolCalls.length > 0
                                      ? pr.toolCalls.map((tc, ti) => (
                                          <div key={ti} className="autotest-detail-toolcall">
                                            <code>{tc.function.name}({tc.function.arguments})</code>
                                          </div>
                                        ))
                                      : pr.assistantText || '(empty)'}
                                  </div>
                                </div>
                                {pr.usage && (
                                  <div className="autotest-detail-field">
                                    <div className="autotest-detail-label">Usage</div>
                                    <div className="autotest-detail-value autotest-detail-usage">
                                      <span>In: {pr.usage.prompt_tokens}</span>
                                      <span>Out: {pr.usage.output_tokens}</span>
                                      <span>TPS: {pr.usage.tokens_per_second.toFixed(1)}</span>
                                      {pr.usage.time_to_first_token_ms !== undefined && (
                                        <span>TTFT: {formatMs(pr.usage.time_to_first_token_ms)}</span>
                                      )}
                                    </div>
                                  </div>
                                )}
                                {pr.notes && pr.notes.length > 0 && (
                                  <div className="autotest-detail-field">
                                    <div className="autotest-detail-label">Notes</div>
                                    <div className="autotest-detail-value autotest-detail-notes">
                                      {pr.notes.map((n, ni) => <div key={ni}>{n}</div>)}
                                    </div>
                                  </div>
                                )}
                              </div>
                              )}
                            </div>
                          );
                        })}
                      </div>
                    )}
                  </div>
                );
              });
            })()
          )}
        </div>
      )}
    </div>
  );
}
