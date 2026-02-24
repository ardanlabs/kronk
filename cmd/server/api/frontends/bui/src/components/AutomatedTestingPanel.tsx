import React, { useState, useCallback, useEffect, useRef, useMemo } from 'react';
import type {
  PlaygroundSessionResponse,
  AutoTestTrialResult,
  AutoTestPromptResult,
  AutoTestScenarioResult,
  SamplingSweepDefinition,
  AutoTestSweepMode,
  ConfigSweepDefinition,
  AutoTestSessionSeed,
  BestConfigWeights,
  SamplingConfig,
} from '../types';
import { defaultSamplingSweepDef, defaultConfigSweepDef, defaultBestConfigWeights, defaultConfigBestWeights, chatScenario, toolCallScenario, configPerfScenario, generateConfigCandidates, generateTrialCandidates, TRIAL_PAUSE_MS } from '../services/autoTestRunner';
import type { AutoTestScenario } from '../types';
import { useAutoTestRunner } from '../contexts/AutoTestRunnerContext';
import type { ConfigTrialResult } from '../contexts/AutoTestRunnerContext';
import { formatMs, scoreColorSafe, formatScore, getScenarioScore, buildSamplingColumns, buildConfigColumns, SWEEP_MODES, FILL_LEVELS } from '../services/sweepModeColumns';
import type { ColumnDef, CellMeta } from '../services/sweepModeColumns';
import SamplingSweepParams from './SamplingSweepParams';
import { SWEEP_PARAM_RANGES } from './SamplingSweepParams';
import type { SamplingNumericKey, SweepInputTriple } from './SamplingSweepParams';
import ConfigSweepParams from './ConfigSweepParams';

type SortDirection = 'asc' | 'desc' | null;

interface SortState {
  column: string | null;
  direction: SortDirection;
}

function nextSortDirection(current: SortDirection): SortDirection {
  if (current === null) return 'asc';
  if (current === 'asc') return 'desc';
  return null;
}

function sortIndicator(column: string, sort: SortState): string {
  if (sort.column !== column || sort.direction === null) return '';
  return sort.direction === 'asc' ? ' ▲' : ' ▼';
}

function sortRows<T extends AutoTestTrialResult>(
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

interface AutomatedTestingPanelProps {
  session: PlaygroundSessionResponse | null;
  sessionSeed: AutoTestSessionSeed | null;
  catalogSampling?: SamplingConfig | null;
}

function clampNum(n: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, n));
}

function normalizeTriple(key: SamplingNumericKey, raw: SweepInputTriple): { min: number; max: number; step: number } {
  const r = SWEEP_PARAM_RANGES[key];
  const minP = Number(raw.min);
  const maxP = Number(raw.max);
  const stepP = Number(raw.step);
  let min = Number.isFinite(minP) ? clampNum(minP, r.validMin, r.validMax) : r.validMin;
  let max = Number.isFinite(maxP) ? clampNum(maxP, r.validMin, r.validMax) : min;
  if (min > max) max = min;
  const step = Number.isFinite(stepP) && stepP > 0 ? Math.max(stepP, 1e-6) : r.defaultStep;
  return { min, max, step };
}

function expandSweep(min: number, max: number, step: number): number[] {
  if (step <= 0 || min === max) return [min];
  const values: number[] = [];
  const eps = step * 0.001;
  for (let v = min; v <= max + eps; ) {
    const rounded = Math.round(v * 1e6) / 1e6;
    if (rounded <= max + 1e-9) values.push(rounded);
    if (values.length >= 200) break;
    const next = v + step;
    if (next === v) break;
    v = next;
  }
  return values.length > 0 ? values : [min];
}

function deriveTripleFromValues(key: SamplingNumericKey, values: number[]): SweepInputTriple {
  const r = SWEEP_PARAM_RANGES[key];
  if (!values || values.length === 0) {
    return { min: String(r.validMin), max: String(r.validMin), step: String(r.defaultStep) };
  }
  if (values.length === 1) {
    return { min: String(values[0]), max: String(values[0]), step: String(r.defaultStep) };
  }
  const min = values[0];
  const max = values[values.length - 1];
  const inferredStep = values[1] - values[0];
  let uniform = inferredStep > 0;
  for (let i = 2; i < values.length && uniform; i++) {
    if (Math.abs((values[i] - values[i - 1]) - inferredStep) > 1e-6) uniform = false;
  }
  const step = uniform && inferredStep > 0 ? Math.round(inferredStep * 1e6) / 1e6 : r.defaultStep;
  return { min: String(min), max: String(max), step: String(step) };
}

function deriveSweepInputs(def: SamplingSweepDefinition): Record<SamplingNumericKey, SweepInputTriple> {
  const out = {} as Record<SamplingNumericKey, SweepInputTriple>;
  for (const key of Object.keys(SWEEP_PARAM_RANGES) as SamplingNumericKey[]) {
    out[key] = deriveTripleFromValues(key, def[key] as number[]);
  }
  return out;
}

function estimatePromptDurationMs(usage: AutoTestPromptResult['usage']): number | undefined {
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

function formatCompletionTime(date: Date): string {
  const now = new Date();
  const sameDay = date.getDate() === now.getDate() &&
    date.getMonth() === now.getMonth() &&
    date.getFullYear() === now.getFullYear();
  const time = date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
  if (sameDay) return time;
  return date.toLocaleDateString([], { month: 'short', day: 'numeric' }) + ', ' + time;
}

function useRunTiming(runStartedAt: string | undefined, trials: AutoTestTrialResult[], totalCount: number, running: boolean) {
  const [, setTick] = useState(0);

  const completed = trials.filter((t) => t?.finishedAt).length;
  const isActive = running && completed < totalCount;

  useEffect(() => {
    if (!isActive) return;
    const id = setInterval(() => setTick((t) => t + 1), 1000);
    return () => clearInterval(id);
  }, [isActive]);

  const runStartMs = runStartedAt ? Date.parse(runStartedAt) : NaN;
  const trialStartTimes = trials
    .map((t) => t?.startedAt ? Date.parse(t.startedAt) : NaN)
    .filter(Number.isFinite) as number[];
  const startMs = Number.isFinite(runStartMs) ? runStartMs : (trialStartTimes.length ? Math.min(...trialStartTimes) : NaN);
  const elapsedMs = Number.isFinite(startMs) ? Math.max(0, Date.now() - startMs) : 0;
  const elapsed = Number.isFinite(startMs) ? formatMs(elapsedMs) : null;

  let estimate: string | null = null;
  let estimatedCompletion: string | null = null;
  if (completed > 0 && completed < totalCount) {
    const durations = trials
      .filter(t => t?.startedAt && t?.finishedAt)
      .map(t => Date.parse(t.finishedAt!) - Date.parse(t.startedAt!))
      .filter(ms => Number.isFinite(ms) && ms > 0);
    if (durations.length > 0) {
      const avgMs = durations.reduce((a, b) => a + b, 0) / durations.length;
      const remaining = Math.max(0, totalCount - completed);
      const remainingPauses = completed > 0 ? remaining : Math.max(0, remaining - 1);
      const estimatedRemainingMs = avgMs * remaining + TRIAL_PAUSE_MS * remainingPauses;
      estimate = formatMs(estimatedRemainingMs);
      if (completed >= 3) {
        estimatedCompletion = formatCompletionTime(new Date(Date.now() + estimatedRemainingMs));
      }
    }
  }

  return { elapsed, estimate, estimatedCompletion };
}

interface TrialProgressBarProps {
  totalTrials: number;
  trials: AutoTestTrialResult[];
  running: boolean;
  runStartedAt?: string;
}

function TrialProgressBar({ totalTrials, trials, running, runStartedAt }: TrialProgressBarProps) {
  const { elapsed, estimate, estimatedCompletion } = useRunTiming(runStartedAt, trials, totalTrials, running);

  const completedCount = trials.filter(t => t?.finishedAt).length;
  const hasActive = running && completedCount < totalTrials;
  const pct = Math.min(100, totalTrials > 0 ? ((completedCount + (hasActive ? 0.5 : 0)) / totalTrials) * 100 : 0);

  const runningTrial = trials.find(t => t?.status === 'running');
  let promptStatus: string | null = null;
  if (runningTrial) {
    const completedPrompts = runningTrial.scenarioResults.reduce((sum, sr) => sum + sr.promptResults.length, 0);
    promptStatus = completedPrompts > 0 ? `Prompt ${completedPrompts} completed` : 'Starting…';
  }

  const displayIndex = Math.min(completedCount + (hasActive ? 1 : 0), totalTrials);

  const label = `${elapsed ?? '0s'}${estimate ? ` · ~${estimate} left` : ''}${estimatedCompletion ? ` · ETA ${estimatedCompletion}` : ''}`;
  const showInside = pct >= 50;

  return (
    <div className="playground-autotest-progress">
      <div className="playground-autotest-progress-text">
        <span>Trial {displayIndex} / {totalTrials}</span>
        {promptStatus && <span className="playground-autotest-prompt-progress"> · {promptStatus}</span>}
      </div>
      <div className="playground-autotest-progress-bar">
        <div
          className="playground-autotest-progress-fill"
          style={{ width: `${pct}%` }}
        >
          {showInside && <span className="playground-autotest-progress-label">{label}</span>}
        </div>
        {!showInside && <span className="playground-autotest-progress-label-outside">{label}</span>}
      </div>
    </div>
  );
}

function BestTrialMetrics({ trial, showScores }: { trial: AutoTestTrialResult; showScores: boolean }) {
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

interface TrialDetailsProps {
  trial: AutoTestTrialResult;
  scenarioLookup: Record<string, AutoTestScenario>;
  hideScores?: boolean;
}

interface ScenarioHeaderProps {
  sr: AutoTestScenarioResult;
  scenarioName: string;
  hideScores?: boolean;
}

function ScenarioHeader({ sr, scenarioName, hideScores }: ScenarioHeaderProps) {
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

function formatLogTime(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function TrialDetails({ trial, scenarioLookup, hideScores }: TrialDetailsProps) {
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
      {trial.scenarioResults.length > 0 && (
        <div className="autotest-detail-scenarios-section">
          <button type="button" className="autotest-detail-log-header" aria-expanded={scenariosExpanded} onClick={e => { e.stopPropagation(); setScenariosExpanded(v => !v); }}>
            <span>{scenariosExpanded ? '▾' : '▸'} Performance</span>
          </button>
          {!scenariosExpanded ? (
            <div className="autotest-detail-scenarios-summary">
              {trial.scenarioResults.map((sr) => (
                <ScenarioHeader key={sr.scenarioId} sr={sr} scenarioName={scenarioLookup[sr.scenarioId]?.name ?? sr.scenarioId} hideScores={hideScores} />
              ))}
            </div>
          ) : (
            trial.scenarioResults.map((sr) => {
              const scenario = scenarioLookup[sr.scenarioId];
              return (
                <div key={sr.scenarioId} className="autotest-detail-scenario">
                  <ScenarioHeader sr={sr} scenarioName={scenario?.name ?? sr.scenarioId} hideScores={hideScores} />
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
                </div>
              );
            })
          )}
        </div>
      )}
    </div>
  );
}

export default function AutomatedTestingPanel({ session, sessionSeed, catalogSampling }: AutomatedTestingPanelProps) {
  const { run, isRunning, startSamplingRun, startConfigRun, stopRun, clearRun, reevaluateBestTrial, reorderQueuedTrial, skipTrial, unskipTrial } = useAutoTestRunner();

  // Compute initial values from the run (if any) so that remounting
  // after navigation restores the sweep parameters instead of resetting.
  const initSweepDef = run?.kind === 'sampling' ? run.sweepDef : defaultSamplingSweepDef;
  const initConfigSweepDef = run?.kind === 'config' ? run.configSweepDef : defaultConfigSweepDef;
  const initWeights = run?.weights ?? (run?.kind === 'config' ? defaultConfigBestWeights : defaultBestConfigWeights);

  const [sweepMode, setSweepMode] = useState<AutoTestSweepMode>(() => run?.kind ?? 'sampling');
  const [enabledScenarios, setEnabledScenarios] = useState(() => run?.enabledScenarios ?? { chat: true, tool_call: true });
  const [sweepDef, setSweepDef] = useState<SamplingSweepDefinition>(() => structuredClone(initSweepDef));
  const [sweepInputs, setSweepInputs] = useState(() => deriveSweepInputs(initSweepDef));
  const sweepInputsRef = useRef(sweepInputs);
  useEffect(() => { sweepInputsRef.current = sweepInputs; }, [sweepInputs]);
  const [sweepDirty, setSweepDirty] = useState(!!run);
  const [lastCatalogRef, setLastCatalogRef] = useState<SamplingConfig | null>(null);
  const maxTrials = Infinity;
  const [configSweepDef, setConfigSweepDef] = useState<ConfigSweepDefinition>(() => structuredClone(initConfigSweepDef));
  const [weights, setWeights] = useState<BestConfigWeights>(() => ({ ...initWeights }));
  const [weightsChanged, setWeightsChanged] = useState(false);
  const appliedWeightsRef = useRef<BestConfigWeights>({ ...initWeights });
  useEffect(() => {
    if (!isRunning && !run) {
      const next = sweepMode === 'config' ? defaultConfigBestWeights : defaultBestConfigWeights;
      setWeights(next);
      appliedWeightsRef.current = { ...next };
      setWeightsChanged(false);
    }
  }, [sweepMode, isRunning, run]);
  const [resultsExpanded, setResultsExpanded] = useState(false);
  const [expandedTrials, setExpandedTrials] = useState<Set<string>>(new Set());
  const [repeats, setRepeats] = useState(() => run?.repeats ?? 1);
  const [sort, setSort] = useState<SortState>({ column: null, direction: null });
  const canReorder = isRunning && run?.status === 'running_trials' && sort.column === null;

  const [dragTrialId, setDragTrialId] = useState<string | null>(null);
  const [dragOverTrialId, setDragOverTrialId] = useState<string | null>(null);

  const handleDragStart = useCallback((e: React.DragEvent<HTMLTableRowElement>, trialId: string) => {
    setDragTrialId(trialId);
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', trialId);
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent<HTMLTableRowElement>, trialId: string) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    setDragOverTrialId(trialId);
  }, []);

  const handleDragLeave = useCallback(() => {
    setDragOverTrialId(null);
  }, []);

  const handleDrop = useCallback((e: React.DragEvent<HTMLTableRowElement>, targetId: string) => {
    e.preventDefault();
    const sourceId = e.dataTransfer.getData('text/plain');
    if (sourceId && sourceId !== targetId) {
      reorderQueuedTrial({ trialId: sourceId, targetId });
    }
    setDragTrialId(null);
    setDragOverTrialId(null);
  }, [reorderQueuedTrial]);

  const handleDragEnd = useCallback(() => {
    setDragTrialId(null);
    setDragOverTrialId(null);
  }, []);

  const [, setTick] = useState(0);
  useEffect(() => {
    if (!isRunning) return;
    const id = setInterval(() => setTick(t => t + 1), 1000);
    return () => clearInterval(id);
  }, [isRunning]);

  const scenarioLookup: Record<string, AutoTestScenario> = useMemo(() => {
    const lookup: Record<string, AutoTestScenario> = { chat: sweepMode === 'config' ? configPerfScenario : chatScenario };
    if (sweepMode !== 'config') lookup.tool_call = toolCallScenario;
    return lookup;
  }, [sweepMode]);

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

  // Raw text state for numeric sweep inputs so users can type freely (e.g. ", 1234").
  // We only parse into numbers on blur.
  const [rawNBatch, setRawNBatch] = useState(() => initConfigSweepDef.nbatch.values.join(', '));
  const [rawNUBatch, setRawNUBatch] = useState(() => initConfigSweepDef.nubatch.values.join(', '));
  const [rawContextWindow, setRawContextWindow] = useState(() => initConfigSweepDef.contextWindow.values.join(', '));
  const [rawNSeqMax, setRawNSeqMax] = useState(() => initConfigSweepDef.nSeqMax.values.join(', '));

  // Hydrate local state when a new run appears (e.g. after navigation).
  // Keyed on runId so it fires once per run, not on every trial update.
  const hydratedRunIdRef = useRef<string | undefined>(run?.runId);
  useEffect(() => {
    if (!run || run.runId === hydratedRunIdRef.current) return;
    hydratedRunIdRef.current = run.runId;
    setSweepMode(run.kind);
    setEnabledScenarios(run.enabledScenarios);
    setRepeats(run.repeats);
    setWeights({ ...run.weights });
    appliedWeightsRef.current = { ...run.weights };
    setWeightsChanged(false);
    if (run.kind === 'sampling') {
      setSweepDef(structuredClone(run.sweepDef));
      setSweepInputs(deriveSweepInputs(run.sweepDef));
      setSweepDirty(true);
    } else {
      setConfigSweepDef(structuredClone(run.configSweepDef));
      setRawNBatch(run.configSweepDef.nbatch.values.join(', '));
      setRawNUBatch(run.configSweepDef.nubatch.values.join(', '));
      setRawContextWindow(run.configSweepDef.contextWindow.values.join(', '));
      setRawNSeqMax(run.configSweepDef.nSeqMax.values.join(', '));
    }
  }, [run?.runId]);

  // When a run is cleared, allow catalog defaults to apply again.
  const prevRunRef = useRef<typeof run>(run);
  useEffect(() => {
    if (prevRunRef.current && !run) {
      setSweepDirty(false);
      hydratedRunIdRef.current = undefined;
    }
    prevRunRef.current = run;
  }, [run]);

  const commitNumericSweep = useCallback((
    raw: string,
    field: 'nbatch' | 'nubatch' | 'contextWindow' | 'nSeqMax',
    setRaw: (v: string) => void,
  ) => {
    const values = raw.split(',').map(s => Math.floor(Number(s.trim()))).filter(n => Number.isFinite(n) && n > 0);
    if (values.length === 0) {
      setConfigSweepDef(d => {
        setRaw(d[field].values.join(', '));
        return d;
      });
      return;
    }
    setConfigSweepDef(d => ({ ...d, [field]: { ...d[field], enabled: true, values } }));
    setRaw(values.join(', '));
  }, []);

  const commitTriple = useCallback((key: SamplingNumericKey) => {
    const { min, max, step } = normalizeTriple(key, sweepInputsRef.current[key]);
    const values = expandSweep(min, max, step);
    setSweepInputs(prev => ({ ...prev, [key]: { min: String(min), max: String(max), step: String(step) } }));
    setSweepDef(d => ({ ...d, [key]: values }));
    setSweepDirty(true);
  }, []);

  // Initialize sweep def from catalog sampling defaults.
  // Re-initializes when catalog changes (model switch) unless user has edited values.
  // Skip when a run exists — the UI should reflect the run's actual parameters.
  useEffect(() => {
    if (run) return;
    if (!catalogSampling || catalogSampling === lastCatalogRef) return;
    setLastCatalogRef(catalogSampling);
    if (sweepDirty) return;
    const cs = catalogSampling;
    const updated: SamplingSweepDefinition = {
      temperature: [cs.temperature ?? 0.8],
      top_p: [cs.top_p ?? 0.9],
      top_k: [cs.top_k ?? 40],
      min_p: [cs.min_p ?? 0],
      repeat_penalty: [cs.repeat_penalty ?? 1.0],
      repeat_last_n: [cs.repeat_last_n ?? 64],
      frequency_penalty: [cs.frequency_penalty ?? 0.0],
      presence_penalty: [cs.presence_penalty ?? 0.0],
      dry_multiplier: [cs.dry_multiplier ?? 1.05],
      dry_base: [cs.dry_base ?? 1.75],
      dry_allowed_length: [cs.dry_allowed_length ?? 2],
      dry_penalty_last_n: [cs.dry_penalty_last_n ?? 0],
      xtc_probability: [cs.xtc_probability ?? 0.0],
      xtc_threshold: [cs.xtc_threshold ?? 0.1],
      xtc_min_keep: [cs.xtc_min_keep ?? 1],
      max_tokens: [cs.max_tokens ?? 4096],
      enable_thinking: [cs.enable_thinking || 'true'],
      reasoning_effort: [cs.reasoning_effort || 'medium'],
    };
    setSweepDef(updated);
    setSweepInputs(deriveSweepInputs(updated));
  }, [catalogSampling, lastCatalogRef, sweepDirty, run]);

  const runnerState = run?.status ?? 'idle';
  const errorMessage = run?.errorMessage ?? '';
  const templateRepairStatus = run?.templateRepairStatus ?? '';
  const calibrationStatus = run?.calibrationStatus ?? '';
  const totalTrials = run?.totalTrials ?? 0;
  const trials = run?.kind === 'sampling' ? run.trials : [];
  const configTrials: ConfigTrialResult[] = run?.kind === 'config' ? run.trials : [];
  const bestTrial = run?.kind === 'sampling' && run.bestTrialId
    ? run.trials.find(t => t.id === run.bestTrialId) ?? null
    : null;
  const bestConfigTrial = run?.kind === 'config' && run.bestTrialId
    ? run.trials.find(t => t.id === run.bestTrialId) ?? null
    : null;

  const displayMode: AutoTestSweepMode = run ? run.kind : sweepMode;

  // Keep local sweepMode in sync with the active run so that radio buttons
  // and parameter sections reflect the correct mode after navigation.
  const runKind = run?.kind;
  useEffect(() => {
    if (runKind) setSweepMode(runKind);
  }, [runKind]);

  const activeTrials = displayMode === 'config' ? configTrials : trials;

  const columns = useMemo<ColumnDef<any>[]>(() =>
    displayMode === 'config' ? buildConfigColumns() : buildSamplingColumns(),
  [displayMode]);

  const columnById = useMemo(() =>
    Object.fromEntries(columns.map(c => [c.id, c])) as Record<string, ColumnDef<any>>,
  [columns]);

  const getColumnValue = useCallback((row: AutoTestTrialResult, col: string): number | string | undefined =>
    columnById[col]?.getValue(row),
  [columnById]);

  const sortedActiveTrials = useMemo(() => sortRows(activeTrials, sort, getColumnValue), [activeTrials, sort, getColumnValue]);

  const hasEnabledScenario = enabledScenarios.chat || enabledScenarios.tool_call;

  const effectiveSweepDef = useMemo<SamplingSweepDefinition>(() => {
    const numericKeys = Object.keys(SWEEP_PARAM_RANGES) as SamplingNumericKey[];
    const numericExpanded = Object.fromEntries(
      numericKeys.map((key) => {
        const { min, max, step } = normalizeTriple(key, sweepInputs[key]);
        return [key, expandSweep(min, max, step)];
      }),
    ) as Pick<SamplingSweepDefinition, SamplingNumericKey>;
    return { ...sweepDef, ...numericExpanded };
  }, [sweepInputs, sweepDef]);

  const samplingTrialCount = useMemo(() => generateTrialCandidates(effectiveSweepDef, maxTrials).length, [effectiveSweepDef, maxTrials]);
  const configTrialCount = useMemo(
    () => sessionSeed ? generateConfigCandidates(sessionSeed.base_config, configSweepDef).length : 1,
    [sessionSeed, configSweepDef],
  );

  // Auto-expand results when first trial data arrives
  const prevTrialCountRef = useRef(0);
  useEffect(() => {
    if (activeTrials.length > 0 && prevTrialCountRef.current === 0 && !resultsExpanded) {
      setResultsExpanded(true);
    }
    prevTrialCountRef.current = activeTrials.length;
  }, [activeTrials.length, resultsExpanded]);

  const handleRun = useCallback(() => {
    if (sweepMode === 'sampling') {
      if (!session && !sessionSeed?.model_id) return;
      startSamplingRun({
        sessionId: session?.session_id,
        sessionSeed: session ? undefined : sessionSeed ?? undefined,
        enabledScenarios,
        sweepDef: effectiveSweepDef,
        maxTrials,
        weights,
        repeats,
        effectiveConfig: session?.effective_config,
      });
    } else {
      if (!sessionSeed?.model_id || session) return;
      startConfigRun({
        sessionSeed,
        enabledScenarios,
        configSweepDef,
        weights,
        repeats,
      });
    }
    appliedWeightsRef.current = { ...weights };
    setWeightsChanged(false);
  }, [sweepMode, session, sessionSeed, enabledScenarios, effectiveSweepDef, maxTrials, configSweepDef, weights, repeats, startSamplingRun, startConfigRun]);

  const handleWeightChange = useCallback((key: keyof BestConfigWeights, value: number) => {
    setWeights(w => {
      const next = { ...w, [key]: value };
      const applied = appliedWeightsRef.current;
      const changed = (Object.keys(next) as (keyof BestConfigWeights)[]).some(k => next[k] !== applied[k]);
      setWeightsChanged(changed);
      return next;
    });
  }, []);

  const handleReevaluate = useCallback(() => {
    reevaluateBestTrial(weights);
    appliedWeightsRef.current = { ...weights };
    setWeightsChanged(false);
  }, [weights, reevaluateBestTrial]);

  const handleStop = useCallback(() => {
    stopRun();
  }, [stopRun]);

  const handleClear = useCallback(() => {
    clearRun();
  }, [clearRun]);

  const canRun = sweepMode === 'sampling'
    ? !!((session || sessionSeed?.model_id) && !isRunning && hasEnabledScenario)
    : !!(sessionSeed?.model_id && !session && !isRunning && hasEnabledScenario);

  return (
    <div className="playground-autotest-container">
      {/* Sweep Mode + Repeats */}
      <div className="playground-autotest-section" style={{ display: 'flex', gap: 32, alignItems: 'flex-start' }}>
        <div style={{ width: '50%' }}>
          <h4>Sweep Mode</h4>
          <div className="playground-inline-options" style={{ paddingBottom: '20px' }}>
            {SWEEP_MODES.map(m => (
              <label key={m.kind} className="playground-inline-option">
                <input
                  type="radio"
                  name="sweepMode"
                  value={m.kind}
                  checked={sweepMode === m.kind}
                  onChange={() => setSweepMode(m.kind as AutoTestSweepMode)}
                  disabled={isRunning}
                />
                {m.label}
              </label>
            ))}
          </div>

          {/* Scenario Selection (sampling mode only) */}
          {sweepMode === 'sampling' && (
            <div className="playground-autotest-section">
              <h4>Scenario Selection</h4>
              <div className="playground-inline-options">
                <label className="playground-inline-option">
                  <input
                      type="checkbox"
                      checked={enabledScenarios.chat}
                      onChange={(e) => setEnabledScenarios((s) => ({ ...s, chat: e.target.checked }))}
                      disabled={isRunning}
                  />
                  Chat Quality
                </label>
                <label className="playground-inline-option">
                  <input
                      type="checkbox"
                      checked={enabledScenarios.tool_call}
                      onChange={(e) => setEnabledScenarios((s) => ({ ...s, tool_call: e.target.checked }))}
                      disabled={isRunning}
                  />
                  Tool Calling
                </label>
              </div>
            </div>
          )}
        </div>
        <div>
          <h4>Repeats Per Test Case</h4>
          <p style={{ fontSize: 12, color: 'var(--color-gray-600)', marginBottom: 8 }}>
            Each prompt is run this many times and scores are averaged for more stable results.
          </p>
          <input
            type="number"
            value={repeats}
            onChange={(e) => { const n = Math.floor(Number(e.target.value)); if (Number.isFinite(n) && n >= 1) setRepeats(n); }}
            min={1}
            max={20}
            style={{ width: 60 }}
            disabled={isRunning}
          />
        </div>
      </div>

      {/* Config Parameters (config mode only) */}
      {sweepMode === 'config' && (
        <ConfigSweepParams
          configSweepDef={configSweepDef}
          setConfigSweepDef={setConfigSweepDef}
          rawNBatch={rawNBatch}
          setRawNBatch={setRawNBatch}
          rawNUBatch={rawNUBatch}
          setRawNUBatch={setRawNUBatch}
          rawContextWindow={rawContextWindow}
          setRawContextWindow={setRawContextWindow}
          rawNSeqMax={rawNSeqMax}
          setRawNSeqMax={setRawNSeqMax}
          commitNumericSweep={commitNumericSweep}
          isRunning={isRunning}
          trialCount={configTrialCount}
        />
      )}

      {/* Sampling Sweep Parameters (sampling mode only) */}
      {sweepMode === 'sampling' && (
        <SamplingSweepParams
          sweepDef={sweepDef}
          setSweepDef={setSweepDef}
          sweepInputs={sweepInputs}
          setSweepInputs={setSweepInputs}
          commitTriple={commitTriple}
          setSweepDirty={setSweepDirty}
          catalogSampling={catalogSampling}
          isRunning={isRunning}
          trialCount={samplingTrialCount}
        />
      )}

      {/* Action Buttons */}
      <div className="playground-autotest-actions">
        <button
          className="btn btn-primary"
          onClick={handleRun}
          disabled={!canRun}
        >
          Run Automated Testing
        </button>
        {isRunning && (
          <button className="btn btn-danger" onClick={handleStop}>
            Stop
          </button>
        )}
        {activeTrials.length > 0 && !isRunning && (
          <button className="btn btn-secondary btn-small" onClick={handleClear}>
            Clear Results
          </button>
        )}
      </div>

      {/* Config mode session warning */}
      {sweepMode === 'config' && session && !isRunning && (
        <div className="playground-error">Unload the current session before running config sweeps</div>
      )}

      {/* Template Repair Status */}
      {templateRepairStatus && isRunning && runnerState !== 'running_trials' && (
        <div className="playground-autotest-status">
          <span className="playground-autotest-spinner" /> {templateRepairStatus}
        </div>
      )}

      {/* Calibration Status */}
      {calibrationStatus && isRunning && runnerState !== 'running_trials' && (
        <div className="playground-autotest-status">
          <span className="playground-autotest-spinner" /> {calibrationStatus}
        </div>
      )}

      {/* Error Display */}
      {errorMessage && <div className="playground-error">{errorMessage}</div>}

      {/* Progress */}
      {runnerState === 'running_trials' && (
        <TrialProgressBar
          totalTrials={totalTrials}
          trials={activeTrials}
          running={isRunning}
          runStartedAt={run?.runStartedAt}
        />
      )}

      {/* Best Configuration Found (shown after run completes, before results) */}
      {runnerState === 'completed' && (displayMode === 'config' ? bestConfigTrial : bestTrial) && (
        <div className="playground-autotest-best">
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

          {/* Best Configuration Criteria (collapsible inside best box) */}
          <details className="playground-sampling-params" style={{ marginTop: 12 }}>
            <summary>Best Configuration Criteria</summary>
            <p style={{ fontSize: 12, color: 'var(--color-gray-600)', marginBottom: 8 }}>
              Weights control how the best configuration is chosen. Higher weight = more influence.
            </p>
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
                  <label className="playground-sweep-param-toggle" htmlFor={`best-config-weight-${key}`}>{label}</label>
                  <input
                    id={`best-config-weight-${key}`}
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
              <p style={{ fontSize: 11, color: '#b45309', marginTop: 6 }}>
                ⚠ Total Score is derived from Chat/Tool weights. Weighting Total Score alongside Chat or Tool Score will double-count quality.
              </p>
            )}
            {weightsChanged && (
              <button
                className="btn btn-primary btn-small"
                style={{ marginTop: 8 }}
                onClick={handleReevaluate}
              >
                Reevaluate
              </button>
            )}
          </details>
        </div>
      )}

      {/* Results Table (collapsed by default) */}
      {activeTrials.length > 0 && (
        <details className="playground-autotest-results" open={resultsExpanded} onToggle={(e) => setResultsExpanded((e.currentTarget as HTMLDetailsElement).open)}>
          <summary style={{ cursor: 'pointer', fontWeight: 600, fontSize: 13, color: 'var(--color-gray-700)', marginBottom: 8 }}>
            Results ({activeTrials.length} trials)
          </summary>
          <div className="playground-autotest-table-scroll">
            <table className="playground-autotest-table">
              <thead>
                <tr>
                  <th>#</th>
                  {isRunning && <th>Actions</th>}
                  {columns.map(c => (
                    <th key={c.id} className={c.sortable !== false ? 'sortable-th' : undefined} onClick={c.sortable !== false ? () => handleSort(c.id) : undefined}>
                      {c.title}{sortIndicator(c.id, sort)}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {sortedActiveTrials.map((trial, i) => {
                  const isPending = (trial.totalScore === undefined || trial.totalScore === null) && trial.status !== 'skipped';
                  const isInProgress = isPending && trial.status === 'running';
                  const bestTrialForMode = displayMode === 'config' ? bestConfigTrial : bestTrial;
                  const isBest = bestTrialForMode && trial === bestTrialForMode && runnerState === 'completed';
                  const isExpanded = expandedTrials.has(trial.id);
                  const isDragging = dragTrialId === trial.id;
                  const isDragOver = dragOverTrialId === trial.id && dragTrialId !== trial.id;
                  const isDraggable = canReorder && trial.status === 'queued';
                  const meta: CellMeta = { isPending, isInProgress, isBest: !!isBest, index: i };
                  return (
                    <React.Fragment key={trial.id}>
                      <tr
                        className={`autotest-trial-row${isBest ? ' autotest-best-row' : ''}${isInProgress ? ' autotest-running-row' : ''}${trial.status === 'skipped' ? ' autotest-skipped-row' : ''}${isDragging ? ' autotest-dragging-row' : ''}${isDragOver ? ' autotest-dragover-row' : ''}`}
                        style={{ cursor: 'pointer' }}
                        onClick={() => toggleTrialExpanded(trial.id)}
                        draggable={isDraggable}
                        onDragStart={isDraggable ? (e) => handleDragStart(e, trial.id) : undefined}
                        onDragOver={isDraggable ? (e) => handleDragOver(e, trial.id) : undefined}
                        onDragLeave={isDraggable ? handleDragLeave : undefined}
                        onDrop={isDraggable ? (e) => handleDrop(e, trial.id) : undefined}
                        onDragEnd={isDraggable ? handleDragEnd : undefined}
                      >
                        <td>{isExpanded ? '▾' : '▸'} {isInProgress && <span className="playground-autotest-spinner-inline" />}{i + 1}</td>
                        {isRunning && (
                          <td className="autotest-actions-cell" onClick={(e) => e.stopPropagation()}>
                            {trial.status === 'queued' && (
                              <span className="autotest-queue-controls">
                                <span
                                  className={`autotest-drag-handle${!canReorder ? ' disabled' : ''}`}
                                  title={canReorder ? 'Drag to reorder' : 'Clear column sorting to reorder'}
                                >
                                  ☰
                                </span>
                                <button
                                  className="btn btn-small autotest-skip-btn"
                                  onClick={() => skipTrial({ trialId: trial.id })}
                                  title="Skip this trial"
                                >
                                  Skip
                                </button>
                              </span>
                            )}
                            {trial.status === 'skipped' && (
                              <button
                                className="btn btn-small autotest-unskip-btn"
                                onClick={() => unskipTrial({ trialId: trial.id })}
                                title="Restore this trial to the queue"
                              >
                                Unskip
                              </button>
                            )}
                          </td>
                        )}
                        {columns.map(c => (
                          <td key={c.id}>{c.renderCell(trial, meta)}</td>
                        ))}
                      </tr>
                      {isExpanded && (
                        <tr className="autotest-detail-row">
                          <td colSpan={columns.length + (isRunning ? 2 : 1)}>
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
