import React, { useState, useCallback, useEffect, useRef } from 'react';
import type {
  PlaygroundSessionResponse,
  AutoTestTrialResult,
  SamplingCandidate,
  AutoTestSweepMode,
  ConfigSweepDefinition,
  AutoTestSessionSeed,
  BestConfigWeights,
} from '../types';
import { defaultConfigSweepDef, defaultBestConfigWeights, chatScenario, toolCallScenario, generateConfigCandidates } from '../services/autoTestRunner';
import type { AutoTestScenario } from '../types';
import { useAutoTestRunner } from '../contexts/AutoTestRunnerContext';
import type { ConfigTrialResult } from '../contexts/AutoTestRunnerContext';

interface AutomatedTestingPanelProps {
  session: PlaygroundSessionResponse | null;
  sessionSeed: AutoTestSessionSeed | null;
}

const defaultBaseline: SamplingCandidate = {
  temperature: 0.8,
  top_p: 0.9,
  top_k: 40,
  min_p: 0,
  repeat_penalty: 1.0,
  repeat_last_n: 64,
  frequency_penalty: 0.0,
  presence_penalty: 0.0,
  dry_multiplier: 1.05,
  dry_base: 1.75,
  dry_allowed_length: 2,
  dry_penalty_last_n: 0,
  xtc_probability: 0.0,
  xtc_threshold: 0.1,
  xtc_min_keep: 1,
  enable_thinking: 'true',
  reasoning_effort: 'medium',
  max_tokens: 4096,
};

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

interface RunTimingProps {
  trials: AutoTestTrialResult[];
  totalCount: number;
}

function RunTiming({ trials, totalCount }: RunTimingProps) {
  const [, setTick] = useState(0);

  const completed = trials.filter((t) =>
    t?.startedAt && t?.finishedAt,
  ).length;
  const isActive = completed < totalCount;

  useEffect(() => {
    if (!isActive) return;
    const id = setInterval(() => setTick((t) => t + 1), 1000);
    return () => clearInterval(id);
  }, [isActive]);

  const firstStartedTrial = trials.find((t) => t?.startedAt);
  const firstStartMs = firstStartedTrial?.startedAt
    ? Date.parse(firstStartedTrial.startedAt)
    : NaN;
  const elapsedMs = Number.isFinite(firstStartMs) ? Date.now() - firstStartMs : 0;
  const elapsed = elapsedMs > 0 ? formatDuration(elapsedMs) : null;

  let estimate: string | null = null;
  if (completed > 0 && completed < totalCount) {
    const avgMs = elapsedMs / completed;
    const remaining = Math.max(0, totalCount - completed);
    const estimatedRemainingMs = avgMs * remaining;
    estimate = formatDuration(estimatedRemainingMs);
  }

  if (!elapsed && !estimate) return null;

  return (
    <span style={{ marginLeft: 12, opacity: 0.7 }}>
      {elapsed && <>Elapsed: {elapsed}</>}
      {estimate && <>{elapsed && ' · '}~{estimate} remaining</>}
    </span>
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

export default function AutomatedTestingPanel({ session, sessionSeed }: AutomatedTestingPanelProps) {
  const { run, isRunning, startSamplingRun, startConfigRun, stopRun, clearRun, reevaluateBestTrial } = useAutoTestRunner();

  const [sweepMode, setSweepMode] = useState<AutoTestSweepMode>('sampling');
  const [enabledScenarios, setEnabledScenarios] = useState({ chat: true, tool_call: true });
  const [useCustomBaseline, setUseCustomBaseline] = useState(false);
  const [baseline, setBaseline] = useState<SamplingCandidate>({ ...defaultBaseline });
  const maxTrials = Infinity;
  const [configSweepDef, setConfigSweepDef] = useState<ConfigSweepDefinition>(structuredClone(defaultConfigSweepDef));
  const [weights, setWeights] = useState<BestConfigWeights>({ ...defaultBestConfigWeights });
  const [weightsChanged, setWeightsChanged] = useState(false);
  const appliedWeightsRef = useRef<BestConfigWeights>({ ...defaultBestConfigWeights });
  const [resultsExpanded, setResultsExpanded] = useState(false);
  const [expandedTrials, setExpandedTrials] = useState<Set<number>>(new Set());
  const [repeats, setRepeats] = useState(3);

  const scenarioLookup: Record<string, AutoTestScenario> = {
    chat: chatScenario,
    tool_call: toolCallScenario,
  };

  const toggleTrialExpanded = useCallback((index: number) => {
    setExpandedTrials(prev => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index);
      else next.add(index);
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

  const hasEnabledScenario = enabledScenarios.chat || enabledScenarios.tool_call;

  const handleRun = useCallback(() => {
    if (sweepMode === 'sampling') {
      if (!session) return;
      startSamplingRun({
        sessionId: session.session_id,
        enabledScenarios,
        useCustomBaseline,
        baseline: useCustomBaseline ? baseline : defaultBaseline,
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
  }, [sweepMode, session, sessionSeed, enabledScenarios, useCustomBaseline, baseline, maxTrials, configSweepDef, weights, repeats, startSamplingRun, startConfigRun]);

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
      {/* Sweep Mode */}
      <div className="playground-autotest-section">
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

      {/* Repeats Per Test Case */}
      <div className="playground-autotest-section">
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

      {/* Config Parameters (config mode only) */}
      {sweepMode === 'config' && (
        <div className="playground-autotest-section">
          <h4>Config Parameters</h4>
          <p style={{ fontSize: 12, color: '#6d4c00', marginBottom: 8 }}>
            ⚠ Each candidate reloads the model. This is slower than sampling sweeps.
          </p>
          <div className="playground-sweep-params">
            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle">NBatch</label>
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
              <label className="playground-sweep-param-toggle">NUBatch</label>
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
              <label className="playground-sweep-param-toggle">Context Window</label>
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
              <label className="playground-sweep-param-toggle">NSeqMax</label>
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
              <label className="playground-sweep-param-toggle">Flash Attention</label>
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
              <label className="playground-sweep-param-toggle">Cache Type</label>
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
              <label className="playground-sweep-param-toggle">Cache Mode</label>
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
          <p style={{ fontSize: 12, color: 'var(--color-gray-600)', marginTop: 8 }}>
            Trials: {sessionSeed ? generateConfigCandidates(sessionSeed.base_config, configSweepDef).length : 1}
          </p>
        </div>
      )}

      {/* Baseline Parameters (sampling mode only) */}
      {sweepMode === 'sampling' && (
        <div className="playground-autotest-section">
          <h4>Baseline Parameters</h4>
          <div className="form-group checkbox-group">
            <label>
              <input
                type="checkbox"
                checked={useCustomBaseline}
                onChange={(e) => setUseCustomBaseline(e.target.checked)}
                disabled={isRunning}
              />
              Override baseline parameters
            </label>
          </div>

          {useCustomBaseline && (
            <div className="playground-autotest-baseline-inputs">
              <h5 className="playground-param-group-title">Generation</h5>
              <div className="playground-config-grid-fluid">
                <div className="form-group">
                  <label>Temperature</label>
                  <input
                    type="number"
                    value={baseline.temperature}
                    onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, temperature: n })); }}
                    step={0.1}
                    disabled={isRunning}
                  />
                </div>
                <div className="form-group">
                  <label>Top P</label>
                  <input
                    type="number"
                    value={baseline.top_p}
                    onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, top_p: n })); }}
                    step={0.05}
                    disabled={isRunning}
                  />
                </div>
                <div className="form-group">
                  <label>Top K</label>
                  <input
                    type="number"
                    value={baseline.top_k}
                    onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, top_k: Math.floor(n) })); }}
                    step={1}
                    disabled={isRunning}
                  />
                </div>
                <div className="form-group">
                  <label>Min P</label>
                  <input
                    type="number"
                    value={baseline.min_p}
                    onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, min_p: n })); }}
                    step={0.01}
                    disabled={isRunning}
                  />
                </div>
              </div>

              <h5 className="playground-param-group-title">Repetition Control</h5>
              <div className="playground-config-grid-fluid">
                <div className="form-group">
                  <label>Repeat Penalty</label>
                  <input type="number" value={baseline.repeat_penalty ?? 1.0} onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, repeat_penalty: n })); }} step={0.05} disabled={isRunning} />
                </div>
                <div className="form-group">
                  <label>Repeat Last N</label>
                  <input type="number" value={baseline.repeat_last_n ?? 64} onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, repeat_last_n: Math.floor(n) })); }} step={1} disabled={isRunning} />
                </div>
                <div className="form-group">
                  <label>Frequency Penalty</label>
                  <input type="number" value={baseline.frequency_penalty ?? 0.0} onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, frequency_penalty: n })); }} step={0.1} disabled={isRunning} />
                </div>
                <div className="form-group">
                  <label>Presence Penalty</label>
                  <input type="number" value={baseline.presence_penalty ?? 0.0} onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, presence_penalty: n })); }} step={0.1} disabled={isRunning} />
                </div>
              </div>

              <h5 className="playground-param-group-title">DRY Sampler</h5>
              <div className="playground-config-grid-fluid">
                <div className="form-group">
                  <label>DRY Multiplier</label>
                  <input type="number" value={baseline.dry_multiplier ?? 1.05} onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, dry_multiplier: n })); }} step={0.05} disabled={isRunning} />
                </div>
                <div className="form-group">
                  <label>DRY Base</label>
                  <input type="number" value={baseline.dry_base ?? 1.75} onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, dry_base: n })); }} step={0.05} disabled={isRunning} />
                </div>
                <div className="form-group">
                  <label>DRY Allowed Length</label>
                  <input type="number" value={baseline.dry_allowed_length ?? 2} onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, dry_allowed_length: Math.floor(n) })); }} step={1} disabled={isRunning} />
                </div>
                <div className="form-group">
                  <label>DRY Penalty Last N</label>
                  <input type="number" value={baseline.dry_penalty_last_n ?? 0} onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, dry_penalty_last_n: Math.floor(n) })); }} step={1} disabled={isRunning} />
                </div>
              </div>

              <h5 className="playground-param-group-title">XTC Sampler</h5>
              <div className="playground-config-grid-fluid">
                <div className="form-group">
                  <label>XTC Probability</label>
                  <input type="number" value={baseline.xtc_probability ?? 0.0} onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, xtc_probability: n })); }} step={0.01} disabled={isRunning} />
                </div>
                <div className="form-group">
                  <label>XTC Threshold</label>
                  <input type="number" value={baseline.xtc_threshold ?? 0.1} onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, xtc_threshold: n })); }} step={0.01} disabled={isRunning} />
                </div>
                <div className="form-group">
                  <label>XTC Min Keep</label>
                  <input type="number" value={baseline.xtc_min_keep ?? 1} onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) setBaseline((b) => ({ ...b, xtc_min_keep: Math.floor(n) })); }} step={1} disabled={isRunning} />
                </div>
              </div>

              <h5 className="playground-param-group-title">Reasoning</h5>
              <div className="playground-config-grid-fluid">
                <div className="form-group">
                  <label>Enable Thinking</label>
                  <select value={baseline.enable_thinking ?? 'true'} onChange={(e) => setBaseline((b) => ({ ...b, enable_thinking: e.target.value as 'true' | 'false' }))} disabled={isRunning}>
                    <option value="true">Enabled</option>
                    <option value="false">Disabled</option>
                  </select>
                </div>
                <div className="form-group">
                  <label>Reasoning Effort</label>
                  <select value={baseline.reasoning_effort ?? 'medium'} onChange={(e) => setBaseline((b) => ({ ...b, reasoning_effort: e.target.value as SamplingCandidate['reasoning_effort'] }))} disabled={isRunning}>
                    <option value="none">None</option>
                    <option value="minimal">Minimal</option>
                    <option value="low">Low</option>
                    <option value="medium">Medium</option>
                    <option value="high">High</option>
                  </select>
                </div>
              </div>
            </div>
          )}
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
        <div className="playground-autotest-progress">
          Trial {currentTrialIndex} / {totalTrials}
          <RunTiming
            trials={displayMode === 'config' ? configTrials : trials}
            totalCount={totalTrials}
          />
        </div>
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
                    <th>Context Window</th>
                    <th>NBatch</th>
                    <th>NUBatch</th>
                    <th>NSeqMax</th>
                    <th>Flash Attn</th>
                    <th>KV Cache</th>
                    <th>Cache</th>
                    <th>Status</th>
                  </>
                ) : (
                  <>
                    <th>Temperature</th>
                    <th>Top P</th>
                    <th>Top K</th>
                    <th>Min P</th>
                  </>
                )}
                <th>Chat Score</th>
                <th>Tool Score</th>
                <th>Total Score</th>
                <th>Avg TPS</th>
                <th>Avg TTFT</th>
                <th>TPS @20%</th>
                <th>TPS @50%</th>
                <th>TPS @80%</th>
              </tr>
            </thead>
            <tbody>
              {displayMode === 'config'
                ? configTrials.map((trial, i) => {
                    const isBest = bestConfigTrial && trial === bestConfigTrial && runnerState === 'completed';
                    const isPending = trial.totalScore === undefined || trial.totalScore === null;
                    const isExpanded = expandedTrials.has(i);
                    return (
                      <React.Fragment key={i}>
                        <tr
                          className={`autotest-trial-row${isBest ? ' autotest-best-row' : ''}`}
                          style={{ cursor: 'pointer' }}
                          onClick={() => toggleTrialExpanded(i)}
                        >
                          <td>{isExpanded ? '▾' : '▸'} {i + 1}</td>
                          <td>{trial.config?.['context_window'] ?? '—'}</td>
                          <td>{trial.config?.nbatch ?? '—'}</td>
                          <td>{trial.config?.nubatch ?? '—'}</td>
                          <td>{trial.config?.['nseq_max'] ?? '—'}</td>
                          <td>{trial.config?.['flash_attention'] ?? '—'}</td>
                          <td>{trial.config?.['cache_type'] ?? '—'}</td>
                          <td>{trial.config?.['cache_mode'] ? (trial.config['cache_mode'] === 'none' ? 'None' : trial.config['cache_mode'].toUpperCase()) : '—'}</td>
                          <td style={trial.error ? { color: '#c62828', fontSize: '0.85em' } : isPending ? { color: '#666' } : { color: '#2e7d32' }}>
                            {trial.error ? `Error: ${trial.error}` : isPending ? '...' : 'OK'}
                          </td>
                          <td style={!isPending ? { color: scoreColor(getScenarioScore(trial, 'chat') ?? 0) } : undefined}>
                            {isPending ? '...' : getScenarioScore(trial, 'chat') ?? '—'}
                          </td>
                          <td style={!isPending ? { color: scoreColor(getScenarioScore(trial, 'tool_call') ?? 0) } : undefined}>
                            {isPending ? '...' : getScenarioScore(trial, 'tool_call') ?? '—'}
                          </td>
                          <td style={!isPending ? { color: scoreColor(trial.totalScore ?? 0) } : undefined}>
                            {isPending ? '...' : trial.totalScore}
                          </td>
                          <td>{isPending ? '...' : trial.avgTPS?.toFixed(1)}</td>
                          <td>{isPending ? '...' : trial.avgTTFT !== undefined ? `${trial.avgTTFT.toFixed(0)}ms` : '—'}</td>
                          <td>{isPending ? '...' : trial.avgTPSByFill?.['20%']?.toFixed(1) ?? '—'}</td>
                          <td>{isPending ? '...' : trial.avgTPSByFill?.['50%']?.toFixed(1) ?? '—'}</td>
                          <td>{isPending ? '...' : trial.avgTPSByFill?.['80%']?.toFixed(1) ?? '—'}</td>
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
                : trials.map((trial, i) => {
                    const isBest = bestTrial && trial === bestTrial && runnerState === 'completed';
                    const isPending = trial.totalScore === undefined || trial.totalScore === null;
                    const isExpanded = expandedTrials.has(i);
                    return (
                      <React.Fragment key={i}>
                        <tr
                          className={`autotest-trial-row${isBest ? ' autotest-best-row' : ''}`}
                          style={{ cursor: 'pointer' }}
                          onClick={() => toggleTrialExpanded(i)}
                        >
                          <td>{isExpanded ? '▾' : '▸'} {i + 1}</td>
                          <td>{trial.candidate.temperature}</td>
                          <td>{trial.candidate.top_p}</td>
                          <td>{trial.candidate.top_k}</td>
                          <td>{trial.candidate.min_p}</td>
                          <td style={!isPending ? { color: scoreColor(getScenarioScore(trial, 'chat') ?? 0) } : undefined}>
                            {isPending ? '...' : getScenarioScore(trial, 'chat') ?? '—'}
                          </td>
                          <td style={!isPending ? { color: scoreColor(getScenarioScore(trial, 'tool_call') ?? 0) } : undefined}>
                            {isPending ? '...' : getScenarioScore(trial, 'tool_call') ?? '—'}
                          </td>
                          <td style={!isPending ? { color: scoreColor(trial.totalScore ?? 0) } : undefined}>
                            {isPending ? '...' : trial.totalScore}
                          </td>
                          <td>{isPending ? '...' : trial.avgTPS?.toFixed(1)}</td>
                          <td>{isPending ? '...' : trial.avgTTFT !== undefined ? `${trial.avgTTFT.toFixed(0)}ms` : '—'}</td>
                          <td>{isPending ? '...' : trial.avgTPSByFill?.['20%']?.toFixed(1) ?? '—'}</td>
                          <td>{isPending ? '...' : trial.avgTPSByFill?.['50%']?.toFixed(1) ?? '—'}</td>
                          <td>{isPending ? '...' : trial.avgTPSByFill?.['80%']?.toFixed(1) ?? '—'}</td>
                        </tr>
                        {isExpanded && (
                          <tr className="autotest-detail-row">
                            <td colSpan={13}>
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
