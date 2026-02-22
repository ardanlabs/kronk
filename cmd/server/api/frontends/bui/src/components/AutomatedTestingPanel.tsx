import React, { useState, useCallback, useEffect, useRef, useMemo } from 'react';
import type {
  PlaygroundSessionResponse,
  AutoTestTrialResult,
  SamplingSweepDefinition,
  AutoTestSweepMode,
  ConfigSweepDefinition,
  AutoTestSessionSeed,
  BestConfigWeights,
  SamplingConfig,
} from '../types';
import { defaultSamplingSweepDef, defaultConfigSweepDef, defaultBestConfigWeights, configChatScenario, configToolCallScenario, generateConfigCandidates, generateTrialCandidates } from '../services/autoTestRunner';
import type { AutoTestScenario } from '../types';
import { useAutoTestRunner } from '../contexts/AutoTestRunnerContext';
import type { ConfigTrialResult } from '../contexts/AutoTestRunnerContext';
import { PARAM_TOOLTIPS, ParamTooltip } from './ParamTooltips';

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

type SamplingNumericKey = 'temperature' | 'top_p' | 'top_k' | 'min_p' | 'repeat_penalty' | 'repeat_last_n' |
  'frequency_penalty' | 'presence_penalty' | 'dry_multiplier' | 'dry_base' | 'dry_allowed_length' |
  'dry_penalty_last_n' | 'xtc_probability' | 'xtc_threshold' | 'xtc_min_keep' | 'max_tokens';

interface SweepParamRange {
  validMin: number;
  validMax: number;
  defaultStep: number;
}

interface SweepInputTriple {
  min: string;
  max: string;
  step: string;
}

const SWEEP_PARAM_RANGES: Record<SamplingNumericKey, SweepParamRange> = {
  temperature: { validMin: 0, validMax: 2, defaultStep: 0.1 },
  top_p: { validMin: 0, validMax: 1, defaultStep: 0.1 },
  top_k: { validMin: 0, validMax: 200, defaultStep: 10 },
  min_p: { validMin: 0, validMax: 1, defaultStep: 0.05 },
  repeat_penalty: { validMin: 0, validMax: 3, defaultStep: 0.1 },
  repeat_last_n: { validMin: 0, validMax: 2048, defaultStep: 64 },
  frequency_penalty: { validMin: -2, validMax: 2, defaultStep: 0.1 },
  presence_penalty: { validMin: -2, validMax: 2, defaultStep: 0.1 },
  dry_multiplier: { validMin: 0, validMax: 5, defaultStep: 0.1 },
  dry_base: { validMin: 1, validMax: 3, defaultStep: 0.25 },
  dry_allowed_length: { validMin: 0, validMax: 100, defaultStep: 1 },
  dry_penalty_last_n: { validMin: 0, validMax: 2048, defaultStep: 64 },
  xtc_probability: { validMin: 0, validMax: 1, defaultStep: 0.1 },
  xtc_threshold: { validMin: 0, validMax: 1, defaultStep: 0.1 },
  xtc_min_keep: { validMin: 1, validMax: 100, defaultStep: 1 },
  max_tokens: { validMin: 1, validMax: 131072, defaultStep: 512 },
};

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

function scoreColor(score: number): string {
  if (score >= 80) return '#2e7d32';
  if (score >= 50) return '#f9a825';
  return '#c62828';
}

function getScenarioScore(trial: AutoTestTrialResult, id: 'chat' | 'tool_call'): number | undefined {
  const s = trial.scenarioResults.find((r) => r.scenarioId === id);
  return s?.score;
}

function formatDuration(ms: number): string {
  const totalSec = Math.max(0, Math.ceil(ms / 1000));
  const hrs = Math.floor(totalSec / 3600);
  const mins = Math.floor((totalSec % 3600) / 60);
  const secs = totalSec % 60;
  if (hrs > 0) return `${hrs}h ${mins}m ${secs}s`;
  if (mins > 0) return `${mins}m ${secs}s`;
  return `${secs}s`;
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

function useRunTiming(trials: AutoTestTrialResult[], totalCount: number, running: boolean) {
  const [, setTick] = useState(0);

  const completed = trials.filter((t) =>
    t?.startedAt && t?.finishedAt,
  ).length;
  const isActive = running && completed < totalCount;

  useEffect(() => {
    if (!isActive) return;
    const id = setInterval(() => setTick((t) => t + 1), 1000);
    return () => clearInterval(id);
  }, [isActive]);

  const startTimes = trials
    .map((t) => t?.startedAt ? Date.parse(t.startedAt) : NaN)
    .filter(Number.isFinite) as number[];
  const firstStartMs = startTimes.length ? Math.min(...startTimes) : NaN;
  const elapsedMs = Number.isFinite(firstStartMs) ? Math.max(0, Date.now() - firstStartMs) : 0;
  const elapsed = elapsedMs > 0 ? formatDuration(elapsedMs) : null;

  let estimate: string | null = null;
  let estimatedCompletion: string | null = null;
  if (completed > 0 && completed < totalCount) {
    const avgMs = elapsedMs / completed;
    const remaining = Math.max(0, totalCount - completed);
    const estimatedRemainingMs = avgMs * remaining;
    estimate = formatDuration(estimatedRemainingMs);
    if (completed >= 3) {
      estimatedCompletion = formatCompletionTime(new Date(Date.now() + estimatedRemainingMs));
    }
  }

  return { elapsed, estimate, estimatedCompletion };
}

interface TrialProgressBarProps {
  currentTrialIndex: number;
  totalTrials: number;
  trials: AutoTestTrialResult[];
  running: boolean;
}

function TrialProgressBar({ currentTrialIndex, totalTrials, trials, running }: TrialProgressBarProps) {
  const { elapsed, estimate, estimatedCompletion } = useRunTiming(trials, totalTrials, running);
  const pct = Math.min(100, totalTrials > 0 ? ((currentTrialIndex + (running && currentTrialIndex < totalTrials ? 0.5 : 0)) / totalTrials) * 100 : 0);

  const currentTrial = trials[currentTrialIndex];
  let promptStatus: string | null = null;
  if (currentTrial && currentTrial.status === 'running') {
    const completedPrompts = currentTrial.scenarioResults.reduce((sum, sr) => sum + sr.promptResults.length, 0);
    promptStatus = completedPrompts > 0 ? `Prompt ${completedPrompts} completed` : 'Starting…';
  }

  const label = `${elapsed ?? '0s'}${estimate ? ` · ~${estimate} left` : ''}${estimatedCompletion ? ` · ETA ${estimatedCompletion}` : ''}`;
  const showInside = pct >= 50;

  return (
    <div className="playground-autotest-progress">
      <div className="playground-autotest-progress-text">
        <span>Trial {Math.min(currentTrialIndex + (running ? 1 : 0), totalTrials)} / {totalTrials}</span>
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

function TrialCountInfo({ count }: { count: number }) {
  return (
    <p style={{ fontSize: 12, color: 'var(--color-gray-600)', marginTop: 8 }}>
      Trials: {count}
    </p>
  );
}

interface TrialDetailsProps {
  trial: AutoTestTrialResult;
  scenarioLookup: Record<string, AutoTestScenario>;
}

function TrialDetails({ trial, scenarioLookup }: TrialDetailsProps) {
  if (trial.scenarioResults.length === 0) {
    return <div className="autotest-detail-empty">No scenario results yet.</div>;
  }

  return (
    <div className="autotest-detail-content">
      {trial.scenarioResults.map((sr) => {
        const scenario = scenarioLookup[sr.scenarioId];
        return (
          <div key={sr.scenarioId} className="autotest-detail-scenario">
            <div className="autotest-detail-scenario-header">
              <span className="autotest-detail-scenario-name">{scenario?.name ?? sr.scenarioId}</span>
              <span className="autotest-detail-scenario-score" style={{ color: scoreColor(sr.score) }}>
                Score: {sr.score.toFixed(1)}
              </span>
              {sr.avgTPS !== undefined && <span>TPS: {sr.avgTPS.toFixed(1)}</span>}
              {sr.avgTTFT !== undefined && <span>TTFT: {sr.avgTTFT.toFixed(0)}ms</span>}
              {sr.avgTPSByFill && Object.keys(sr.avgTPSByFill).length > 0 && (
                <span style={{ marginLeft: 8, opacity: 0.85 }}>
                  Context Fill TPS:
                  {sr.avgTPSByFill['20%'] !== undefined && ` @20%: ${sr.avgTPSByFill['20%'].toFixed(1)}`}
                  {sr.avgTPSByFill['50%'] !== undefined && ` @50%: ${sr.avgTPSByFill['50%'].toFixed(1)}`}
                  {sr.avgTPSByFill['80%'] !== undefined && ` @80%: ${sr.avgTPSByFill['80%'].toFixed(1)}`}
                </span>
              )}
            </div>
            <div className="autotest-detail-prompts">
              {sr.promptResults.map((pr) => {
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

                return (
                  <div key={pr.promptId} className="autotest-detail-prompt">
                    <div className="autotest-detail-prompt-header">
                      <span className="autotest-detail-prompt-id">{isCtxFill ? `Context Fill @${pr.promptId.replace('ctxfill-', '')}%` : pr.promptId}</span>
                      <span className="autotest-detail-prompt-score" style={{ color: scoreColor(pr.score) }}>
                        {pr.score.toFixed(1)}
                      </span>
                    </div>
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
                              <span>TTFT: {pr.usage.time_to_first_token_ms.toFixed(0)}ms</span>
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
                  </div>
                );
              })}
            </div>
          </div>
        );
      })}
    </div>
  );
}

export default function AutomatedTestingPanel({ session, sessionSeed, catalogSampling }: AutomatedTestingPanelProps) {
  const { run, isRunning, startSamplingRun, startConfigRun, stopRun, clearRun, reevaluateBestTrial } = useAutoTestRunner();

  const [sweepMode, setSweepMode] = useState<AutoTestSweepMode>('sampling');
  const [enabledScenarios, setEnabledScenarios] = useState({ chat: true, tool_call: true });
  const [sweepDef, setSweepDef] = useState<SamplingSweepDefinition>(structuredClone(defaultSamplingSweepDef));
  const [sweepInputs, setSweepInputs] = useState(() => deriveSweepInputs(defaultSamplingSweepDef));
  const sweepInputsRef = useRef(sweepInputs);
  useEffect(() => { sweepInputsRef.current = sweepInputs; }, [sweepInputs]);
  const [sweepDirty, setSweepDirty] = useState(false);
  const [lastCatalogRef, setLastCatalogRef] = useState<SamplingConfig | null>(null);
  const maxTrials = Infinity;
  const [configSweepDef, setConfigSweepDef] = useState<ConfigSweepDefinition>(structuredClone(defaultConfigSweepDef));
  const [weights, setWeights] = useState<BestConfigWeights>({ ...defaultBestConfigWeights });
  const [weightsChanged, setWeightsChanged] = useState(false);
  const appliedWeightsRef = useRef<BestConfigWeights>({ ...defaultBestConfigWeights });
  const [resultsExpanded, setResultsExpanded] = useState(false);
  const [expandedTrials, setExpandedTrials] = useState<Set<string>>(new Set());
  const [repeats, setRepeats] = useState(3);
  const [sort, setSort] = useState<SortState>({ column: null, direction: null });

  const scenarioLookup: Record<string, AutoTestScenario> = {
    chat: configChatScenario,
    tool_call: configToolCallScenario,
  };

  const handleSort = useCallback((column: string) => {
    setSort(prev => ({
      column: prev.column === column && nextSortDirection(prev.direction) === null ? null : column,
      direction: prev.column === column ? nextSortDirection(prev.direction) : 'asc',
    }));
  }, []);

  const getSamplingValue = useCallback((row: AutoTestTrialResult, col: string): number | string | undefined => {
    switch (col) {
      case 'temperature': return row.candidate.temperature;
      case 'top_p': return row.candidate.top_p;
      case 'top_k': return row.candidate.top_k;
      case 'min_p': return row.candidate.min_p;
      case 'chat_score': return getScenarioScore(row, 'chat');
      case 'tool_score': return getScenarioScore(row, 'tool_call');
      case 'total_score': return row.totalScore;
      case 'status': return row.status === 'failed' ? 'Failed' : (row.totalScore !== undefined && row.totalScore !== null) ? 'OK' : '...';
      case 'avg_tps': return row.avgTPS;
      case 'avg_ttft': return row.avgTTFT;
      case 'tps_20': return row.avgTPSByFill?.['20%'];
      case 'tps_50': return row.avgTPSByFill?.['50%'];
      case 'tps_80': return row.avgTPSByFill?.['80%'];
      default: return undefined;
    }
  }, []);

  const getConfigValue = useCallback((row: ConfigTrialResult, col: string): number | string | undefined => {
    switch (col) {
      case 'context_window': return row.config?.['context_window'];
      case 'nbatch': return row.config?.nbatch;
      case 'nubatch': return row.config?.nubatch;
      case 'nseq_max': return row.config?.['nseq_max'];
      case 'flash_attention': return row.config?.['flash_attention'];
      case 'cache_type': return row.config?.['cache_type'];
      case 'cache_mode': return row.config?.['cache_mode'];
      case 'status': return row.error ? `Error: ${row.error}` : (row.totalScore !== undefined && row.totalScore !== null) ? 'OK' : '...';
      case 'chat_score': return getScenarioScore(row, 'chat');
      case 'tool_score': return getScenarioScore(row, 'tool_call');
      case 'total_score': return row.totalScore;
      case 'avg_tps': return row.avgTPS;
      case 'avg_ttft': return row.avgTTFT;
      case 'tps_20': return row.avgTPSByFill?.['20%'];
      case 'tps_50': return row.avgTPSByFill?.['50%'];
      case 'tps_80': return row.avgTPSByFill?.['80%'];
      default: return undefined;
    }
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
  const [rawNBatch, setRawNBatch] = useState(defaultConfigSweepDef.nbatch.values.join(', '));
  const [rawNUBatch, setRawNUBatch] = useState(defaultConfigSweepDef.nubatch.values.join(', '));
  const [rawContextWindow, setRawContextWindow] = useState(defaultConfigSweepDef.contextWindow.values.join(', '));
  const [rawNSeqMax, setRawNSeqMax] = useState(defaultConfigSweepDef.nSeqMax.values.join(', '));

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
  useEffect(() => {
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
  }, [catalogSampling, lastCatalogRef, sweepDirty]);

  const runnerState = run?.status ?? 'idle';
  const errorMessage = run?.errorMessage ?? '';
  const templateRepairStatus = run?.templateRepairStatus ?? '';
  const calibrationStatus = run?.calibrationStatus ?? '';
  const currentTrialIndex = run?.currentTrialIndex ?? 0;
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

  const sortedTrials = useMemo(() => sortRows(trials, sort, getSamplingValue), [trials, sort, getSamplingValue]);
  const sortedConfigTrials = useMemo(() => sortRows(configTrials, sort, getConfigValue), [configTrials, sort, getConfigValue]);

  const hasEnabledScenario = enabledScenarios.chat || enabledScenarios.tool_call;

  const samplingTrialCount = useMemo(() => generateTrialCandidates(sweepDef, maxTrials).length, [sweepDef, maxTrials]);
  const configTrialCount = useMemo(
    () => sessionSeed ? generateConfigCandidates(sessionSeed.base_config, configSweepDef).length : 1,
    [sessionSeed, configSweepDef],
  );

  // Auto-expand results when first trial data arrives
  const activeTrials = displayMode === 'config' ? configTrials : trials;
  const prevTrialCountRef = useRef(0);
  useEffect(() => {
    if (activeTrials.length > 0 && prevTrialCountRef.current === 0 && !resultsExpanded) {
      setResultsExpanded(true);
    }
    prevTrialCountRef.current = activeTrials.length;
  }, [activeTrials.length, resultsExpanded]);

  const handleRun = useCallback(() => {
    if (sweepMode === 'sampling') {
      if (!session) return;
      startSamplingRun({
        sessionId: session.session_id,
        enabledScenarios,
        sweepDef,
        maxTrials,
        weights,
        repeats,
        effectiveConfig: session.effective_config,
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
  }, [sweepMode, session, sessionSeed, enabledScenarios, sweepDef, maxTrials, configSweepDef, weights, repeats, startSamplingRun, startConfigRun]);

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
    ? !!(session && !isRunning && hasEnabledScenario)
    : !!(sessionSeed?.model_id && !session && !isRunning && hasEnabledScenario);

  return (
    <div className="playground-autotest-container">
      {/* Sweep Mode + Repeats */}
      <div className="playground-autotest-section" style={{ display: 'flex', gap: 32, alignItems: 'flex-start' }}>
        <div>
          <h4>Sweep Mode</h4>
          <div className="playground-inline-options">
            <label className="playground-inline-option">
              <input
                type="radio"
                name="sweepMode"
                value="sampling"
                checked={sweepMode === 'sampling'}
                onChange={() => setSweepMode('sampling')}
                disabled={isRunning}
              />
              Sampling Sweep
            </label>
            <label className="playground-inline-option">
              <input
                type="radio"
                name="sweepMode"
                value="config"
                checked={sweepMode === 'config'}
                onChange={() => setSweepMode('config')}
                disabled={isRunning}
              />
              Config Sweep
            </label>
          </div>

          {/* Scenario Selection */}
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
        <div className="playground-autotest-section">
          <h4>Config Parameters</h4>
          <p style={{ fontSize: 12, color: '#6d4c00', marginBottom: 8 }}>
            ⚠ Each candidate reloads the model. This is slower than sampling sweeps.
          </p>
          <div className="playground-sweep-params">
            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle">NBatch{PARAM_TOOLTIPS.nbatch && <ParamTooltip text={PARAM_TOOLTIPS.nbatch} />}</label>
              <input
                type="text"
                className="playground-sweep-param-values"
                value={rawNBatch}
                onChange={(e) => setRawNBatch(e.target.value)}
                onBlur={() => commitNumericSweep(rawNBatch, 'nbatch', setRawNBatch)}
                onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
                placeholder="512, 1024, 2048, 4096"
                disabled={isRunning}
              />
            </div>

            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle">NUBatch{PARAM_TOOLTIPS.nubatch && <ParamTooltip text={PARAM_TOOLTIPS.nubatch} />}</label>
              <input
                type="text"
                className="playground-sweep-param-values"
                value={rawNUBatch}
                onChange={(e) => setRawNUBatch(e.target.value)}
                onBlur={() => commitNumericSweep(rawNUBatch, 'nubatch', setRawNUBatch)}
                onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
                placeholder="128, 256, 512, 1024, 2048"
                disabled={isRunning}
              />
            </div>

            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle">Context Window{PARAM_TOOLTIPS.contextWindow && <ParamTooltip text={PARAM_TOOLTIPS.contextWindow} />}</label>
              <input
                type="text"
                className="playground-sweep-param-values"
                value={rawContextWindow}
                onChange={(e) => setRawContextWindow(e.target.value)}
                onBlur={() => commitNumericSweep(rawContextWindow, 'contextWindow', setRawContextWindow)}
                onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
                placeholder="2048, 4096, 8192, 16384, 32768"
                disabled={isRunning}
              />
            </div>

            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle">NSeqMax{PARAM_TOOLTIPS.nSeqMax && <ParamTooltip text={PARAM_TOOLTIPS.nSeqMax} />}</label>
              <input
                type="text"
                className="playground-sweep-param-values"
                value={rawNSeqMax}
                onChange={(e) => setRawNSeqMax(e.target.value)}
                onBlur={() => commitNumericSweep(rawNSeqMax, 'nSeqMax', setRawNSeqMax)}
                onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
                placeholder="1, 2, 4, 8"
                disabled={isRunning}
              />
            </div>

            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle">Flash Attention{PARAM_TOOLTIPS.flashAttention && <ParamTooltip text={PARAM_TOOLTIPS.flashAttention} />}</label>
              <div className="playground-sweep-option-checks">
                {['auto', 'enabled', 'disabled'].map((val) => (
                  <label key={val} className="playground-sweep-option-label">
                    <input
                      type="checkbox"
                      checked={configSweepDef.flashAttention.values.includes(val)}
                      onChange={(e) => {
                        setConfigSweepDef(d => {
                          const prev = d.flashAttention.values;
                          const next = e.target.checked ? [...prev, val] : prev.filter(v => v !== val);
                          return { ...d, flashAttention: { ...d.flashAttention, values: next } };
                        });
                      }}
                      disabled={isRunning}
                    />
                    {val}
                  </label>
                ))}
              </div>
            </div>

            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle">Cache Type{PARAM_TOOLTIPS.cacheType && <ParamTooltip text={PARAM_TOOLTIPS.cacheType} />}</label>
              <div className="playground-sweep-option-checks">
                {['f16', 'q8_0', 'q4_0'].map((val) => (
                  <label key={val} className="playground-sweep-option-label">
                    <input
                      type="checkbox"
                      checked={configSweepDef.cacheType.values.includes(val)}
                      onChange={(e) => {
                        setConfigSweepDef(d => {
                          const prev = d.cacheType.values;
                          const next = e.target.checked ? [...prev, val] : prev.filter(v => v !== val);
                          return { ...d, cacheType: { ...d.cacheType, values: next } };
                        });
                      }}
                      disabled={isRunning}
                    />
                    {val}
                  </label>
                ))}
              </div>
            </div>

            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle">Cache Mode{PARAM_TOOLTIPS.cacheMode && <ParamTooltip text={PARAM_TOOLTIPS.cacheMode} />}</label>
              <div className="playground-sweep-option-checks">
                {['none', 'spc', 'imc'].map((val) => (
                  <label key={val} className="playground-sweep-option-label">
                    <input
                      type="checkbox"
                      checked={configSweepDef.cacheMode.values.includes(val)}
                      onChange={(e) => {
                        setConfigSweepDef(d => {
                          const prev = d.cacheMode.values;
                          const next = e.target.checked ? [...prev, val] : prev.filter(v => v !== val);
                          return { ...d, cacheMode: { ...d.cacheMode, values: next } };
                        });
                      }}
                      disabled={isRunning}
                    />
                    {val === 'none' ? 'None' : val.toUpperCase()}
                  </label>
                ))}
              </div>
            </div>
          </div>
          <TrialCountInfo count={configTrialCount} />
        </div>
      )}

      {/* Sampling Sweep Parameters (sampling mode only) */}
      {sweepMode === 'sampling' && (
        <div className="playground-autotest-section">
          <h4>Sampling Sweep Parameters</h4>
          <p style={{ fontSize: 12, color: 'var(--color-gray-600)', marginBottom: 8 }}>
            Set min, max, and step values to define the sweep range. Set min = max for no sweep.
          </p>
          <div className="playground-sweep-params">
            <div className="playground-sweep-group-title">Generation</div>
            {([
              ['temperature', 'Temperature'],
              ['top_p', 'Top P'],
              ['top_k', 'Top K'],
              ['min_p', 'Min P'],
            ] as [SamplingNumericKey & keyof SamplingConfig, string][]).map(([key, label]) => {
              const r = SWEEP_PARAM_RANGES[key];
              const triple = sweepInputs[key];
              return (
                <div className="playground-sweep-param" key={key}>
                  <label className="playground-sweep-param-toggle">
                    {label}
                    {PARAM_TOOLTIPS[key] && <ParamTooltip text={PARAM_TOOLTIPS[key]} />}
                    {catalogSampling && catalogSampling[key] !== undefined && (
                      <span className="sweep-catalog-hint" title="Default catalog value">(default: {catalogSampling[key]})</span>
                    )}
                  </label>
                  <div className="playground-sweep-param-range">
                    <div className="playground-sweep-param-range-field">
                      <span className="playground-sweep-param-range-label">Min ({r.validMin})</span>
                      <input type="number" value={triple.min} min={r.validMin} max={r.validMax} step={r.defaultStep}
                        onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], min: e.target.value } }))}
                        onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
                    </div>
                    <div className="playground-sweep-param-range-field">
                      <span className="playground-sweep-param-range-label">Max ({r.validMax})</span>
                      <input type="number" value={triple.max} min={r.validMin} max={r.validMax} step={r.defaultStep}
                        onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], max: e.target.value } }))}
                        onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
                    </div>
                    <div className="playground-sweep-param-range-field">
                      <span className="playground-sweep-param-range-label">Step ({r.defaultStep})</span>
                      <input type="number" value={triple.step} min={0} step={r.defaultStep}
                        onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], step: e.target.value } }))}
                        onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
                    </div>
                  </div>
                </div>
              );
            })}

            <div className="playground-sweep-group-title">Repetition Control</div>
            {([
              ['repeat_penalty', 'Repeat Penalty'],
              ['repeat_last_n', 'Repeat Last N'],
              ['frequency_penalty', 'Frequency Penalty'],
              ['presence_penalty', 'Presence Penalty'],
            ] as [SamplingNumericKey & keyof SamplingConfig, string][]).map(([key, label]) => {
              const r = SWEEP_PARAM_RANGES[key];
              const triple = sweepInputs[key];
              return (
                <div className="playground-sweep-param" key={key}>
                  <label className="playground-sweep-param-toggle">
                    {label}
                    {PARAM_TOOLTIPS[key] && <ParamTooltip text={PARAM_TOOLTIPS[key]} />}
                    {catalogSampling && catalogSampling[key] !== undefined && (
                      <span className="sweep-catalog-hint" title="Default catalog value">(default: {catalogSampling[key]})</span>
                    )}
                  </label>
                  <div className="playground-sweep-param-range">
                    <div className="playground-sweep-param-range-field">
                      <span className="playground-sweep-param-range-label">Min ({r.validMin})</span>
                      <input type="number" value={triple.min} min={r.validMin} max={r.validMax} step={r.defaultStep}
                        onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], min: e.target.value } }))}
                        onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
                    </div>
                    <div className="playground-sweep-param-range-field">
                      <span className="playground-sweep-param-range-label">Max ({r.validMax})</span>
                      <input type="number" value={triple.max} min={r.validMin} max={r.validMax} step={r.defaultStep}
                        onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], max: e.target.value } }))}
                        onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
                    </div>
                    <div className="playground-sweep-param-range-field">
                      <span className="playground-sweep-param-range-label">Step ({r.defaultStep})</span>
                      <input type="number" value={triple.step} min={0} step={r.defaultStep}
                        onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], step: e.target.value } }))}
                        onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
                    </div>
                  </div>
                </div>
              );
            })}

            <div className="playground-sweep-group-title">DRY Sampler</div>
            {([
              ['dry_multiplier', 'DRY Multiplier'],
              ['dry_base', 'DRY Base'],
              ['dry_allowed_length', 'DRY Allowed Length'],
              ['dry_penalty_last_n', 'DRY Penalty Last N'],
            ] as [SamplingNumericKey & keyof SamplingConfig, string][]).map(([key, label]) => {
              const r = SWEEP_PARAM_RANGES[key];
              const triple = sweepInputs[key];
              return (
                <div className="playground-sweep-param" key={key}>
                  <label className="playground-sweep-param-toggle">
                    {label}
                    {PARAM_TOOLTIPS[key] && <ParamTooltip text={PARAM_TOOLTIPS[key]} />}
                    {catalogSampling && catalogSampling[key] !== undefined && (
                      <span className="sweep-catalog-hint" title="Default catalog value">(default: {catalogSampling[key]})</span>
                    )}
                  </label>
                  <div className="playground-sweep-param-range">
                    <div className="playground-sweep-param-range-field">
                      <span className="playground-sweep-param-range-label">Min ({r.validMin})</span>
                      <input type="number" value={triple.min} min={r.validMin} max={r.validMax} step={r.defaultStep}
                        onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], min: e.target.value } }))}
                        onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
                    </div>
                    <div className="playground-sweep-param-range-field">
                      <span className="playground-sweep-param-range-label">Max ({r.validMax})</span>
                      <input type="number" value={triple.max} min={r.validMin} max={r.validMax} step={r.defaultStep}
                        onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], max: e.target.value } }))}
                        onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
                    </div>
                    <div className="playground-sweep-param-range-field">
                      <span className="playground-sweep-param-range-label">Step ({r.defaultStep})</span>
                      <input type="number" value={triple.step} min={0} step={r.defaultStep}
                        onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], step: e.target.value } }))}
                        onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
                    </div>
                  </div>
                </div>
              );
            })}

            <div className="playground-sweep-group-title">XTC Sampler</div>
            {([
              ['xtc_probability', 'XTC Probability'],
              ['xtc_threshold', 'XTC Threshold'],
              ['xtc_min_keep', 'XTC Min Keep'],
            ] as [SamplingNumericKey & keyof SamplingConfig, string][]).map(([key, label]) => {
              const r = SWEEP_PARAM_RANGES[key];
              const triple = sweepInputs[key];
              return (
                <div className="playground-sweep-param" key={key}>
                  <label className="playground-sweep-param-toggle">
                    {label}
                    {PARAM_TOOLTIPS[key] && <ParamTooltip text={PARAM_TOOLTIPS[key]} />}
                    {catalogSampling && catalogSampling[key] !== undefined && (
                      <span className="sweep-catalog-hint" title="Default catalog value">(default: {catalogSampling[key]})</span>
                    )}
                  </label>
                  <div className="playground-sweep-param-range">
                    <div className="playground-sweep-param-range-field">
                      <span className="playground-sweep-param-range-label">Min ({r.validMin})</span>
                      <input type="number" value={triple.min} min={r.validMin} max={r.validMax} step={r.defaultStep}
                        onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], min: e.target.value } }))}
                        onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
                    </div>
                    <div className="playground-sweep-param-range-field">
                      <span className="playground-sweep-param-range-label">Max ({r.validMax})</span>
                      <input type="number" value={triple.max} min={r.validMin} max={r.validMax} step={r.defaultStep}
                        onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], max: e.target.value } }))}
                        onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
                    </div>
                    <div className="playground-sweep-param-range-field">
                      <span className="playground-sweep-param-range-label">Step ({r.defaultStep})</span>
                      <input type="number" value={triple.step} min={0} step={r.defaultStep}
                        onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], step: e.target.value } }))}
                        onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
                    </div>
                  </div>
                </div>
              );
            })}

            <div className="playground-sweep-group-title">Reasoning</div>
            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle">
                Enable Thinking
                {PARAM_TOOLTIPS.enable_thinking && <ParamTooltip text={PARAM_TOOLTIPS.enable_thinking} />}
                {catalogSampling?.enable_thinking && (
                  <span className="sweep-catalog-hint" title="Default catalog value">— {catalogSampling.enable_thinking === 'true' ? 'Enabled' : 'Disabled'}</span>
                )}
              </label>
              <div className="playground-sweep-option-checks">
                {(['true', 'false'] as const).map((val) => (
                  <label key={val} className="playground-sweep-option-label">
                    <input
                      type="checkbox"
                      checked={sweepDef.enable_thinking.includes(val)}
                      onChange={(e) => {
                        setSweepDef(d => {
                          const prev = d.enable_thinking;
                          const next = e.target.checked ? [...prev, val] : prev.filter(v => v !== val);
                          return { ...d, enable_thinking: next.length > 0 ? next : prev };
                        });
                        setSweepDirty(true);
                      }}
                      disabled={isRunning}
                    />
                    {val === 'true' ? 'Enabled' : 'Disabled'}
                  </label>
                ))}
              </div>
            </div>
            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle">
                Reasoning Effort
                {PARAM_TOOLTIPS.reasoning_effort && <ParamTooltip text={PARAM_TOOLTIPS.reasoning_effort} />}
                {catalogSampling?.reasoning_effort && (
                  <span className="sweep-catalog-hint" title="Default catalog value">— {catalogSampling.reasoning_effort}</span>
                )}
              </label>
              <div className="playground-sweep-option-checks">
                {(['none', 'minimal', 'low', 'medium', 'high'] as const).map((val) => (
                  <label key={val} className="playground-sweep-option-label">
                    <input
                      type="checkbox"
                      checked={sweepDef.reasoning_effort.includes(val)}
                      onChange={(e) => {
                        setSweepDef(d => {
                          const prev = d.reasoning_effort;
                          const next = e.target.checked ? [...prev, val] : prev.filter(v => v !== val);
                          return { ...d, reasoning_effort: next.length > 0 ? next : prev };
                        });
                        setSweepDirty(true);
                      }}
                      disabled={isRunning}
                    />
                    {val.charAt(0).toUpperCase() + val.slice(1)}
                  </label>
                ))}
              </div>
            </div>
          </div>
          <TrialCountInfo count={samplingTrialCount} />
        </div>
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
        {(displayMode === 'config' ? configTrials : trials).length > 0 && !isRunning && (
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
      {templateRepairStatus && isRunning && (
        <div className="playground-autotest-status">
          <span className="playground-autotest-spinner" /> {templateRepairStatus}
        </div>
      )}

      {/* Calibration Status */}
      {calibrationStatus && isRunning && (
        <div className="playground-autotest-status">
          <span className="playground-autotest-spinner" /> {calibrationStatus}
        </div>
      )}

      {/* Error Display */}
      {errorMessage && <div className="playground-error">{errorMessage}</div>}

      {/* Progress */}
      {runnerState === 'running_trials' && (
        <TrialProgressBar
          currentTrialIndex={currentTrialIndex}
          totalTrials={totalTrials}
          trials={displayMode === 'config' ? configTrials : trials}
          running={isRunning}
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
              return (
                <>
                  <div><strong>Chat Score:</strong> {getScenarioScore(best, 'chat') ?? '—'}</div>
                  <div><strong>Tool Score:</strong> {getScenarioScore(best, 'tool_call') ?? '—'}</div>
                  <div><strong>Total Score:</strong> {best.totalScore}</div>
                  <div><strong>Avg TPS:</strong> {best.avgTPS?.toFixed(1)}</div>
                  <div><strong>Avg TTFT:</strong> {best.avgTTFT !== undefined ? `${best.avgTTFT.toFixed(0)}ms` : '—'}</div>
                  {best.avgTPSByFill && (
                    <>
                      <div><strong>TPS @20%:</strong> {best.avgTPSByFill['20%']?.toFixed(1) ?? '—'}</div>
                      <div><strong>TPS @50%:</strong> {best.avgTPSByFill['50%']?.toFixed(1) ?? '—'}</div>
                      <div><strong>TPS @80%:</strong> {best.avgTPSByFill['80%']?.toFixed(1) ?? '—'}</div>
                    </>
                  )}
                </>
              );
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
                ['chatScore', 'Chat Score'],
                ['toolScore', 'Tool Score'],
                ['totalScore', 'Total Score'],
                ['avgTPS', 'Avg TPS'],
                ['avgTTFT', 'Avg TTFT (lower is better)'],
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
      {(displayMode === 'config' ? configTrials : trials).length > 0 && (
        <details className="playground-autotest-results" open={resultsExpanded} onToggle={(e) => setResultsExpanded((e.currentTarget as HTMLDetailsElement).open)}>
          <summary style={{ cursor: 'pointer', fontWeight: 600, fontSize: 13, color: 'var(--color-gray-700)', marginBottom: 8 }}>
            Results ({(displayMode === 'config' ? configTrials : trials).length} trials)
          </summary>
          <table className="playground-autotest-table">
            <thead>
              <tr>
                <th>#</th>
                {displayMode === 'config' ? (
                  <>
                    <th className="sortable-th" onClick={() => handleSort('context_window')}>Context Window{sortIndicator('context_window', sort)}</th>
                    <th className="sortable-th" onClick={() => handleSort('nbatch')}>NBatch{sortIndicator('nbatch', sort)}</th>
                    <th className="sortable-th" onClick={() => handleSort('nubatch')}>NUBatch{sortIndicator('nubatch', sort)}</th>
                    <th className="sortable-th" onClick={() => handleSort('nseq_max')}>NSeqMax{sortIndicator('nseq_max', sort)}</th>
                    <th className="sortable-th" onClick={() => handleSort('flash_attention')}>Flash Attn{sortIndicator('flash_attention', sort)}</th>
                    <th className="sortable-th" onClick={() => handleSort('cache_type')}>KV Cache{sortIndicator('cache_type', sort)}</th>
                    <th className="sortable-th" onClick={() => handleSort('cache_mode')}>Cache{sortIndicator('cache_mode', sort)}</th>
                    <th className="sortable-th" onClick={() => handleSort('status')}>Status{sortIndicator('status', sort)}</th>
                  </>
                ) : (
                  <>
                    <th className="sortable-th" onClick={() => handleSort('temperature')}>Temperature{sortIndicator('temperature', sort)}</th>
                    <th className="sortable-th" onClick={() => handleSort('top_p')}>Top P{sortIndicator('top_p', sort)}</th>
                    <th className="sortable-th" onClick={() => handleSort('top_k')}>Top K{sortIndicator('top_k', sort)}</th>
                    <th className="sortable-th" onClick={() => handleSort('min_p')}>Min P{sortIndicator('min_p', sort)}</th>
                    <th className="sortable-th" onClick={() => handleSort('status')}>Status{sortIndicator('status', sort)}</th>
                  </>
                )}
                <th className="sortable-th" onClick={() => handleSort('chat_score')}>Chat Score{sortIndicator('chat_score', sort)}</th>
                <th className="sortable-th" onClick={() => handleSort('tool_score')}>Tool Score{sortIndicator('tool_score', sort)}</th>
                <th className="sortable-th" onClick={() => handleSort('total_score')}>Total Score{sortIndicator('total_score', sort)}</th>
                <th className="sortable-th" onClick={() => handleSort('avg_tps')}>Avg TPS{sortIndicator('avg_tps', sort)}</th>
                <th className="sortable-th" onClick={() => handleSort('avg_ttft')}>Avg TTFT{sortIndicator('avg_ttft', sort)}</th>
                <th className="sortable-th" onClick={() => handleSort('tps_20')}>TPS @20%{sortIndicator('tps_20', sort)}</th>
                <th className="sortable-th" onClick={() => handleSort('tps_50')}>TPS @50%{sortIndicator('tps_50', sort)}</th>
                <th className="sortable-th" onClick={() => handleSort('tps_80')}>TPS @80%{sortIndicator('tps_80', sort)}</th>
              </tr>
            </thead>
            <tbody>
              {displayMode === 'config'
                ? sortedConfigTrials.map((trial, i) => {
                    const isBest = bestConfigTrial && trial === bestConfigTrial && runnerState === 'completed';
                    const isPending = trial.totalScore === undefined || trial.totalScore === null;
                    const isInProgress = isPending && trial.status === 'running';
                    const partialChat = isInProgress ? getScenarioScore(trial, 'chat') : undefined;
                    const partialTool = isInProgress ? getScenarioScore(trial, 'tool_call') : undefined;
                    const partialTPS = isInProgress ? trial.scenarioResults.find(r => r.avgTPS !== undefined)?.avgTPS : undefined;
                    const isExpanded = expandedTrials.has(trial.id);
                    return (
                      <React.Fragment key={trial.id}>
                        <tr
                          className={`autotest-trial-row${isBest ? ' autotest-best-row' : ''}${isInProgress ? ' autotest-running-row' : ''}`}
                          style={{ cursor: 'pointer' }}
                          onClick={() => toggleTrialExpanded(trial.id)}
                        >
                          <td>{isExpanded ? '▾' : '▸'} {isInProgress && <span className="playground-autotest-spinner-inline" />}{i + 1}</td>
                          <td>{trial.config?.['context_window'] ?? '—'}</td>
                          <td>{trial.config?.nbatch ?? '—'}</td>
                          <td>{trial.config?.nubatch ?? '—'}</td>
                          <td>{trial.config?.['nseq_max'] ?? '—'}</td>
                          <td>{trial.config?.['flash_attention'] ?? '—'}</td>
                          <td>{trial.config?.['cache_type'] ?? '—'}</td>
                          <td>{trial.config?.['cache_mode'] ? (trial.config['cache_mode'] === 'none' ? 'None' : trial.config['cache_mode'].toUpperCase()) : '—'}</td>
                          <td style={trial.error ? { color: '#c62828', fontSize: '0.85em' } : isInProgress ? { color: '#1565c0' } : isPending ? { color: '#666' } : { color: '#2e7d32' }}>
                            {trial.error ? `Error: ${trial.error}` : isInProgress ? 'Running…' : isPending ? '…' : 'OK'}
                          </td>
                          <td style={partialChat !== undefined ? { color: scoreColor(partialChat), opacity: 0.7 } : !isPending ? { color: scoreColor(getScenarioScore(trial, 'chat') ?? 0) } : undefined}>
                            {isPending ? (partialChat !== undefined ? `~${partialChat}` : '…') : getScenarioScore(trial, 'chat') ?? '—'}
                          </td>
                          <td style={partialTool !== undefined ? { color: scoreColor(partialTool), opacity: 0.7 } : !isPending ? { color: scoreColor(getScenarioScore(trial, 'tool_call') ?? 0) } : undefined}>
                            {isPending ? (partialTool !== undefined ? `~${partialTool}` : '…') : getScenarioScore(trial, 'tool_call') ?? '—'}
                          </td>
                          <td style={!isPending ? { color: scoreColor(trial.totalScore ?? 0) } : undefined}>
                            {isPending ? '…' : trial.totalScore}
                          </td>
                          <td>{isPending ? (partialTPS !== undefined ? `~${partialTPS.toFixed(1)}` : '…') : trial.avgTPS?.toFixed(1)}</td>
                          <td>{isPending ? '…' : trial.avgTTFT !== undefined ? `${trial.avgTTFT.toFixed(0)}ms` : '—'}</td>
                          <td>{isPending ? '…' : trial.avgTPSByFill?.['20%']?.toFixed(1) ?? '—'}</td>
                          <td>{isPending ? '…' : trial.avgTPSByFill?.['50%']?.toFixed(1) ?? '—'}</td>
                          <td>{isPending ? '…' : trial.avgTPSByFill?.['80%']?.toFixed(1) ?? '—'}</td>
                        </tr>
                        {isExpanded && (
                          <tr className="autotest-detail-row">
                            <td colSpan={17}>
                              <TrialDetails trial={trial} scenarioLookup={scenarioLookup} />
                            </td>
                          </tr>
                        )}
                      </React.Fragment>
                    );
                  })
                : sortedTrials.map((trial, i) => {
                    const isBest = bestTrial && trial === bestTrial && runnerState === 'completed';
                    const isPending = trial.totalScore === undefined || trial.totalScore === null;
                    const isInProgress = isPending && trial.status === 'running';
                    const partialChat = isInProgress ? getScenarioScore(trial, 'chat') : undefined;
                    const partialTool = isInProgress ? getScenarioScore(trial, 'tool_call') : undefined;
                    const partialTPS = isInProgress ? trial.scenarioResults.find(r => r.avgTPS !== undefined)?.avgTPS : undefined;
                    const isExpanded = expandedTrials.has(trial.id);
                    return (
                      <React.Fragment key={trial.id}>
                        <tr
                          className={`autotest-trial-row${isBest ? ' autotest-best-row' : ''}${isInProgress ? ' autotest-running-row' : ''}`}
                          style={{ cursor: 'pointer' }}
                          onClick={() => toggleTrialExpanded(trial.id)}
                        >
                          <td>{isExpanded ? '▾' : '▸'} {isInProgress && <span className="playground-autotest-spinner-inline" />}{i + 1}</td>
                          <td>{trial.candidate.temperature}</td>
                          <td>{trial.candidate.top_p}</td>
                          <td>{trial.candidate.top_k}</td>
                          <td>{trial.candidate.min_p}</td>
                          <td style={trial.status === 'failed' ? { color: '#c62828', fontSize: '0.85em' } : isInProgress ? { color: '#1565c0' } : isPending ? { color: '#666' } : { color: '#2e7d32' }}>
                            {trial.status === 'failed' ? 'Failed' : isInProgress ? 'Running…' : isPending ? '…' : 'OK'}
                          </td>
                          <td style={partialChat !== undefined ? { color: scoreColor(partialChat), opacity: 0.7 } : !isPending ? { color: scoreColor(getScenarioScore(trial, 'chat') ?? 0) } : undefined}>
                            {isPending ? (partialChat !== undefined ? `~${partialChat}` : '…') : getScenarioScore(trial, 'chat') ?? '—'}
                          </td>
                          <td style={partialTool !== undefined ? { color: scoreColor(partialTool), opacity: 0.7 } : !isPending ? { color: scoreColor(getScenarioScore(trial, 'tool_call') ?? 0) } : undefined}>
                            {isPending ? (partialTool !== undefined ? `~${partialTool}` : '…') : getScenarioScore(trial, 'tool_call') ?? '—'}
                          </td>
                          <td style={!isPending ? { color: scoreColor(trial.totalScore ?? 0) } : undefined}>
                            {isPending ? '…' : trial.totalScore}
                          </td>
                          <td>{isPending ? (partialTPS !== undefined ? `~${partialTPS.toFixed(1)}` : '…') : trial.avgTPS?.toFixed(1)}</td>
                          <td>{isPending ? '…' : trial.avgTTFT !== undefined ? `${trial.avgTTFT.toFixed(0)}ms` : '—'}</td>
                          <td>{isPending ? '…' : trial.avgTPSByFill?.['20%']?.toFixed(1) ?? '—'}</td>
                          <td>{isPending ? '…' : trial.avgTPSByFill?.['50%']?.toFixed(1) ?? '—'}</td>
                          <td>{isPending ? '…' : trial.avgTPSByFill?.['80%']?.toFixed(1) ?? '—'}</td>
                        </tr>
                        {isExpanded && (
                          <tr className="autotest-detail-row">
                            <td colSpan={14}>
                              <TrialDetails trial={trial} scenarioLookup={scenarioLookup} />
                            </td>
                          </tr>
                        )}
                      </React.Fragment>
                    );
                  })}
            </tbody>
          </table>
        </details>
      )}
    </div>
  );
}
