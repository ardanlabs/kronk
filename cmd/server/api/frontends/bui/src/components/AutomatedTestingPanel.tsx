import { useState, useRef, useCallback, useEffect } from 'react';
import type {
  PlaygroundSessionResponse,
  AutoTestRunnerState,
  AutoTestTrialResult,
  SamplingCandidate,
  AutoTestScenario,
  AutoTestSweepMode,
  ConfigSweepDefinition,
  ConfigCandidate,
  AutoTestSessionSeed,
} from '../types';
import {
  chatScenario,
  toolCallScenario,
  generateTrialCandidates,
  generateConfigCandidates,
  defaultConfigSweepDef,
  probeTemplate,
  runTrial,
} from '../services/autoTestRunner';
import { api } from '../services/api';

interface AutomatedTestingPanelProps {
  session: PlaygroundSessionResponse | null;
  sessionSeed: AutoTestSessionSeed | null;
}

const defaultBaseline: SamplingCandidate = {
  temperature: 0.8,
  top_p: 0.9,
  top_k: 40,
  min_p: 0,
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

  const firstStartMs = trials.find((t) => t?.startedAt)?.startedAt
    ? Date.parse(trials.find((t) => t?.startedAt)!.startedAt!)
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

export default function AutomatedTestingPanel({ session, sessionSeed }: AutomatedTestingPanelProps) {
  const [runnerState, setRunnerState] = useState<AutoTestRunnerState>('idle');
  const [sweepMode, setSweepMode] = useState<AutoTestSweepMode>('sampling');
  const [enabledScenarios, setEnabledScenarios] = useState({ chat: true, tool_call: true });
  const [useCustomBaseline, setUseCustomBaseline] = useState(false);
  const [baseline, setBaseline] = useState<SamplingCandidate>({ ...defaultBaseline });
  const [maxTrials, setMaxTrials] = useState(25);
  const [trials, setTrials] = useState<AutoTestTrialResult[]>([]);
  const [currentTrialIndex, setCurrentTrialIndex] = useState(0);
  const [totalTrials, setTotalTrials] = useState(0);
  const [bestTrial, setBestTrial] = useState<AutoTestTrialResult | null>(null);
  const [templateRepairStatus, setTemplateRepairStatus] = useState('');
  const [errorMessage, setErrorMessage] = useState('');
  const [configSweepDef, setConfigSweepDef] = useState<ConfigSweepDefinition>(structuredClone(defaultConfigSweepDef));

  const [configTrials, setConfigTrials] = useState<Array<AutoTestTrialResult & { config?: ConfigCandidate }>>([]);
  const [bestConfigTrial, setBestConfigTrial] = useState<(AutoTestTrialResult & { config?: ConfigCandidate }) | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const currentConfigSessionRef = useRef<string | null>(null);

  useEffect(() => {
    return () => {
      abortControllerRef.current?.abort();
      const sid = currentConfigSessionRef.current;
      if (sid) api.deletePlaygroundSession(sid).catch(() => {});
      currentConfigSessionRef.current = null;
    };
  }, []);

  const isRunning = runnerState === 'repairing_template' || runnerState === 'running_trials';
  const hasEnabledScenario = enabledScenarios.chat || enabledScenarios.tool_call;
  const hasEnabledConfigParam = configSweepDef.nbatch.enabled || configSweepDef.nubatch.enabled || configSweepDef.contextWindow.enabled || configSweepDef.nSeqMax.enabled;

  const handleRun = useCallback(async () => {
    if (sweepMode === 'sampling') {
      if (!session) return;

      setRunnerState('repairing_template');
      setErrorMessage('');
      setTemplateRepairStatus('');
      setTrials([]);
      setBestTrial(null);
      setCurrentTrialIndex(0);

      const controller = new AbortController();
      abortControllerRef.current = controller;

      try {
        const scenarios: AutoTestScenario[] = [];
        if (enabledScenarios.chat) scenarios.push(chatScenario);
        if (enabledScenarios.tool_call) scenarios.push(toolCallScenario);

        if (enabledScenarios.tool_call) {
          setTemplateRepairStatus('Probing template for tool calling compatibility...');
          const probeResult = await probeTemplate(session.session_id);
          if (probeResult) {
            setTemplateRepairStatus('Template OK ✓');
          } else {
            setTemplateRepairStatus('Template probe failed — running chat-only tests');
            const idx = scenarios.indexOf(toolCallScenario);
            if (idx !== -1) scenarios.splice(idx, 1);
          }
        }

        const activeBaseline = useCustomBaseline ? baseline : defaultBaseline;
        const candidates = generateTrialCandidates(activeBaseline, maxTrials);
        setTotalTrials(candidates.length);
        setCurrentTrialIndex(0);
        setRunnerState('running_trials');

        let currentBest: AutoTestTrialResult | null = null;

        for (let i = 0; i < candidates.length; i++) {
          if (controller.signal.aborted) break;

          const candidate = candidates[i];

          const onUpdate = (partial: AutoTestTrialResult) => {
            setTrials((prev) => {
              const updated = [...prev];
              updated[i] = partial;
              return updated;
            });
          };

          const result = await runTrial(session.session_id, candidate, scenarios, onUpdate, controller.signal);

          setTrials((prev) => {
            const updated = [...prev];
            updated[i] = result;
            return updated;
          });

          if (!currentBest || (result.totalScore ?? 0) > (currentBest.totalScore ?? 0)) {
            currentBest = result;
            setBestTrial(result);
          }

          setCurrentTrialIndex(i + 1);
        }

        setRunnerState(controller.signal.aborted ? 'cancelled' : 'completed');
      } catch (err: any) {
        setErrorMessage(err.message || 'Automated testing failed');
        setRunnerState('error');
      }
    } else {
      // Config sweep mode
      if (!sessionSeed?.model_id || session) return;

      setRunnerState('running_trials');
      setErrorMessage('');
      setTemplateRepairStatus('');
      setConfigTrials([]);
      setBestConfigTrial(null);
      setCurrentTrialIndex(0);

      const controller = new AbortController();
      abortControllerRef.current = controller;

      try {
        const configCandidates = generateConfigCandidates(sessionSeed.base_config, configSweepDef);
        setTotalTrials(configCandidates.length);
        setCurrentTrialIndex(0);
        setRunnerState('running_trials');

        const scenarios: AutoTestScenario[] = [];
        if (enabledScenarios.chat) scenarios.push(chatScenario);
        if (enabledScenarios.tool_call) scenarios.push(toolCallScenario);

        let currentBest: (AutoTestTrialResult & { config?: ConfigCandidate }) | null = null;

        for (let i = 0; i < configCandidates.length; i++) {
          if (controller.signal.aborted) break;

          const cfg = configCandidates[i];
          let configSessionId: string | null = null;

          try {
            const resp = await api.createPlaygroundSession({
              model_id: sessionSeed.model_id,
              template_mode: sessionSeed.template_mode,
              template_name: sessionSeed.template_name,
              template_script: sessionSeed.template_script,
              config: { ...sessionSeed.base_config, ...cfg },
            });
            configSessionId = resp.session_id;
            currentConfigSessionRef.current = configSessionId;

            const activeScenarios = [...scenarios];

            if (enabledScenarios.tool_call) {
              setTemplateRepairStatus('Probing template for tool calling compatibility...');
              const probeResult = await probeTemplate(configSessionId);
              if (probeResult) {
                setTemplateRepairStatus('Template OK ✓');
              } else {
                setTemplateRepairStatus('Template probe failed — running chat-only tests');
                const idx = activeScenarios.indexOf(toolCallScenario);
                if (idx !== -1) activeScenarios.splice(idx, 1);
              }
            }

            const activeBaseline = useCustomBaseline ? baseline : defaultBaseline;

            const onUpdate = (partial: AutoTestTrialResult) => {
              setConfigTrials((prev) => {
                const updated = [...prev];
                updated[i] = { ...partial, config: cfg };
                return updated;
              });
            };

            const result = await runTrial(configSessionId, activeBaseline, activeScenarios, onUpdate, controller.signal);

            const configResult = { ...result, config: cfg };
            setConfigTrials((prev) => {
              const updated = [...prev];
              updated[i] = configResult;
              return updated;
            });

            if (!currentBest || (result.totalScore ?? 0) > (currentBest.totalScore ?? 0)) {
              currentBest = configResult;
              setBestConfigTrial(configResult);
            }
          } finally {
            if (configSessionId) {
              await api.deletePlaygroundSession(configSessionId).catch(() => {});
              if (currentConfigSessionRef.current === configSessionId) {
                currentConfigSessionRef.current = null;
              }
            }
          }

          setCurrentTrialIndex(i + 1);
        }

        setRunnerState(controller.signal.aborted ? 'cancelled' : 'completed');
      } catch (err: any) {
        setErrorMessage(err.message || 'Automated testing failed');
        setRunnerState('error');
      }
    }
  }, [session, sessionSeed, sweepMode, enabledScenarios, useCustomBaseline, baseline, maxTrials, configSweepDef]);

  const handleStop = useCallback(() => {
    abortControllerRef.current?.abort();
    if (currentConfigSessionRef.current) {
      api.deletePlaygroundSession(currentConfigSessionRef.current).catch(() => {});
      currentConfigSessionRef.current = null;
    }
    setRunnerState('cancelled');
  }, []);

  const handleClear = useCallback(() => {
    setTrials([]);
    setBestTrial(null);
    setConfigTrials([]);
    setBestConfigTrial(null);
    setCurrentTrialIndex(0);
    setTotalTrials(0);
    setRunnerState('idle');
    setErrorMessage('');
    setTemplateRepairStatus('');
  }, []);

  const canRun = sweepMode === 'sampling'
    ? !!(session && !isRunning && hasEnabledScenario)
    : !!(sessionSeed?.model_id && !session && !isRunning && hasEnabledScenario && hasEnabledConfigParam);

  return (
    <div className="playground-autotest-container">
      {/* Sweep Mode */}
      <div className="playground-autotest-section">
        <h4>Sweep Mode</h4>
        <div className="form-group checkbox-group">
          <label>
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
        </div>
        <div className="form-group checkbox-group">
          <label>
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
        <div className="form-group checkbox-group">
          <label>
            <input
              type="checkbox"
              checked={enabledScenarios.chat}
              onChange={(e) => setEnabledScenarios((s) => ({ ...s, chat: e.target.checked }))}
              disabled={isRunning}
            />
            Chat Quality
          </label>
        </div>
        <div className="form-group checkbox-group">
          <label>
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

      {/* Config Parameters (config mode only) */}
      {sweepMode === 'config' && (
        <div className="playground-autotest-section">
          <h4>Config Parameters</h4>
          <p style={{ fontSize: 12, color: '#6d4c00', marginBottom: 8 }}>
            ⚠ Each candidate reloads the model. This is slower than sampling sweeps.
          </p>
          <div className="form-group checkbox-group">
            <label>
              <input
                type="checkbox"
                checked={configSweepDef.nbatch.enabled}
                onChange={(e) => setConfigSweepDef((d) => ({ ...d, nbatch: { ...d.nbatch, enabled: e.target.checked } }))}
                disabled={isRunning}
              />
              NBatch
            </label>
            {configSweepDef.nbatch.enabled && (
              <input
                type="text"
                value={configSweepDef.nbatch.values.join(', ')}
                onChange={(e) => {
                  const values = e.target.value.split(',').map(s => Number(s.trim())).filter(n => Number.isFinite(n) && n > 0);
                  setConfigSweepDef(d => ({ ...d, nbatch: { ...d.nbatch, values } }));
                }}
                placeholder="512, 1024, 2048, 4096"
                disabled={isRunning}
                style={{ marginLeft: 8, flex: 1 }}
              />
            )}
          </div>
          <div className="form-group checkbox-group">
            <label>
              <input
                type="checkbox"
                checked={configSweepDef.nubatch.enabled}
                onChange={(e) => setConfigSweepDef((d) => ({ ...d, nubatch: { ...d.nubatch, enabled: e.target.checked } }))}
                disabled={isRunning}
              />
              NUBatch
            </label>
            {configSweepDef.nubatch.enabled && (
              <input
                type="text"
                value={configSweepDef.nubatch.values.join(', ')}
                onChange={(e) => {
                  const values = e.target.value.split(',').map(s => Number(s.trim())).filter(n => Number.isFinite(n) && n > 0);
                  setConfigSweepDef(d => ({ ...d, nubatch: { ...d.nubatch, values } }));
                }}
                placeholder="128, 256, 512, 1024, 2048"
                disabled={isRunning}
                style={{ marginLeft: 8, flex: 1 }}
              />
            )}
          </div>
          <div className="form-group checkbox-group">
            <label>
              <input
                type="checkbox"
                checked={configSweepDef.contextWindow.enabled}
                onChange={(e) => setConfigSweepDef((d) => ({ ...d, contextWindow: { ...d.contextWindow, enabled: e.target.checked } }))}
                disabled={isRunning}
              />
              Context Window
            </label>
            {configSweepDef.contextWindow.enabled && (
              <input
                type="text"
                value={configSweepDef.contextWindow.values.join(', ')}
                onChange={(e) => {
                  const values = e.target.value.split(',').map(s => Number(s.trim())).filter(n => Number.isFinite(n) && n > 0);
                  setConfigSweepDef(d => ({ ...d, contextWindow: { ...d.contextWindow, values } }));
                }}
                placeholder="2048, 4096, 8192, 16384, 32768"
                disabled={isRunning}
                style={{ marginLeft: 8, flex: 1 }}
              />
            )}
          </div>
          <div className="form-group checkbox-group">
            <label>
              <input
                type="checkbox"
                checked={configSweepDef.nSeqMax.enabled}
                onChange={(e) => setConfigSweepDef((d) => ({ ...d, nSeqMax: { ...d.nSeqMax, enabled: e.target.checked } }))}
                disabled={isRunning}
              />
              NSeqMax
            </label>
            {configSweepDef.nSeqMax.enabled && (
              <input
                type="text"
                value={configSweepDef.nSeqMax.values.join(', ')}
                onChange={(e) => {
                  const values = e.target.value.split(',').map(s => Number(s.trim())).filter(n => Number.isFinite(n) && n > 0);
                  setConfigSweepDef(d => ({ ...d, nSeqMax: { ...d.nSeqMax, values } }));
                }}
                placeholder="1, 2, 4, 8"
                disabled={isRunning}
                style={{ marginLeft: 8, flex: 1 }}
              />
            )}
          </div>
          {hasEnabledConfigParam && (
            <p style={{ fontSize: 12, color: 'var(--color-gray-600)', marginTop: 8 }}>
              Estimated trials: {(() => {
                const axes: number[] = [];
                if (configSweepDef.nbatch.enabled && configSweepDef.nbatch.values.length > 0) axes.push(configSweepDef.nbatch.values.length);
                if (configSweepDef.nubatch.enabled && configSweepDef.nubatch.values.length > 0) axes.push(configSweepDef.nubatch.values.length);
                if (configSweepDef.contextWindow.enabled && configSweepDef.contextWindow.values.length > 0) axes.push(configSweepDef.contextWindow.values.length);
                if (configSweepDef.nSeqMax.enabled && configSweepDef.nSeqMax.values.length > 0) axes.push(configSweepDef.nSeqMax.values.length);
                return axes.length > 0 ? axes.reduce((a, b) => a * b, 1) : 1;
              })()}
            </p>
          )}
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
          )}
        </div>
      )}

      {/* Trial Settings (sampling mode only) */}
      {sweepMode === 'sampling' && (
        <div className="playground-autotest-section">
          <h4>Trial Settings</h4>
          <div className="form-group">
            <label>Max Trials</label>
            <input
              type="number"
              value={maxTrials}
              min={5}
              onChange={(e) => {
                const n = Number(e.target.value);
                setMaxTrials(Number.isFinite(n) ? Math.max(5, n) : 25);
              }}
              disabled={isRunning}
            />
          </div>
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
        {(sweepMode === 'config' ? configTrials : trials).length > 0 && !isRunning && (
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

      {/* Error Display */}
      {errorMessage && <div className="playground-error">{errorMessage}</div>}

      {/* Progress */}
      {runnerState === 'running_trials' && (
        <div className="playground-autotest-progress">
          Trial {currentTrialIndex} / {totalTrials}
          <RunTiming
            trials={sweepMode === 'config' ? configTrials : trials}
            totalCount={totalTrials}
          />
        </div>
      )}

      {/* Results Table */}
      {(sweepMode === 'config' ? configTrials : trials).length > 0 && (
        <div className="playground-autotest-results">
          <h4>Results</h4>
          <table className="playground-autotest-table">
            <thead>
              <tr>
                <th>#</th>
                {sweepMode === 'config' ? (
                  <>
                    <th>Context Window</th>
                    <th>NBatch</th>
                    <th>NUBatch</th>
                    <th>NSeqMax</th>
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
              </tr>
            </thead>
            <tbody>
              {sweepMode === 'config'
                ? configTrials.map((trial, i) => {
                    const isBest = bestConfigTrial && trial === bestConfigTrial && runnerState === 'completed';
                    const isPending = trial.totalScore === undefined || trial.totalScore === null;
                    return (
                      <tr key={i} style={isBest ? { backgroundColor: '#e8f5e9' } : undefined}>
                        <td>{i + 1}</td>
                        <td>{trial.config?.['context-window'] ?? '—'}</td>
                        <td>{trial.config?.nbatch ?? '—'}</td>
                        <td>{trial.config?.nubatch ?? '—'}</td>
                        <td>{trial.config?.['nseq-max'] ?? '—'}</td>
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
                      </tr>
                    );
                  })
                : trials.map((trial, i) => {
                    const isBest = bestTrial && trial === bestTrial && runnerState === 'completed';
                    const isPending = trial.totalScore === undefined || trial.totalScore === null;
                    return (
                      <tr key={i} style={isBest ? { backgroundColor: '#e8f5e9' } : undefined}>
                        <td>{i + 1}</td>
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
                      </tr>
                    );
                  })}
            </tbody>
          </table>
        </div>
      )}

      {/* Best Result Summary */}
      {runnerState === 'completed' && (sweepMode === 'config' ? bestConfigTrial : bestTrial) && (
        <div className="playground-autotest-best">
          <h4>Best Configuration Found</h4>
          <div className="playground-autotest-best-details">
            {sweepMode === 'config' && bestConfigTrial ? (
              <>
                <div><strong>Context Window:</strong> {bestConfigTrial.config?.['context-window'] ?? '—'}</div>
                <div><strong>NBatch:</strong> {bestConfigTrial.config?.nbatch ?? '—'}</div>
                <div><strong>NUBatch:</strong> {bestConfigTrial.config?.nubatch ?? '—'}</div>
                <div><strong>NSeqMax:</strong> {bestConfigTrial.config?.['nseq-max'] ?? '—'}</div>
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
              const best = sweepMode === 'config' ? bestConfigTrial : bestTrial;
              if (!best) return null;
              return (
                <>
                  <div><strong>Chat Score:</strong> {getScenarioScore(best, 'chat') ?? '—'}</div>
                  <div><strong>Tool Score:</strong> {getScenarioScore(best, 'tool_call') ?? '—'}</div>
                  <div><strong>Total Score:</strong> {best.totalScore}</div>
                  <div><strong>Avg TPS:</strong> {best.avgTPS?.toFixed(1)}</div>
                </>
              );
            })()}
          </div>
        </div>
      )}
    </div>
  );
}
