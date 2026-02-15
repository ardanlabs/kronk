import { useState, useRef, useCallback } from 'react';
import type {
  PlaygroundSessionResponse,
  AutoTestRunnerState,
  AutoTestTrialResult,
  SamplingCandidate,
  AutoTestScenario,
} from '../types';
import {
  chatScenario,
  toolCallScenario,
  generateTrialCandidates,
  probeTemplate,
  runTrial,
} from '../services/autoTestRunner';

interface AutomatedTestingPanelProps {
  session: PlaygroundSessionResponse | null;
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

export default function AutomatedTestingPanel({ session }: AutomatedTestingPanelProps) {
  const [runnerState, setRunnerState] = useState<AutoTestRunnerState>('idle');
  const [enabledScenarios, setEnabledScenarios] = useState({ chat: true, tool_call: true });
  const [useCustomBaseline, setUseCustomBaseline] = useState(false);
  const [baseline, setBaseline] = useState<SamplingCandidate>({ ...defaultBaseline });
  const [trials, setTrials] = useState<AutoTestTrialResult[]>([]);
  const [currentTrialIndex, setCurrentTrialIndex] = useState(0);
  const [totalTrials, setTotalTrials] = useState(0);
  const [bestTrial, setBestTrial] = useState<AutoTestTrialResult | null>(null);
  const [templateRepairStatus, setTemplateRepairStatus] = useState('');
  const [errorMessage, setErrorMessage] = useState('');
  const abortControllerRef = useRef<AbortController | null>(null);

  const isRunning = runnerState === 'repairing_template' || runnerState === 'running_trials';
  const hasEnabledScenario = enabledScenarios.chat || enabledScenarios.tool_call;

  const handleRun = useCallback(async () => {
    if (!session) return;

    setRunnerState('repairing_template');
    setErrorMessage('');
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
      const candidates = generateTrialCandidates(activeBaseline);
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
  }, [session, enabledScenarios, useCustomBaseline, baseline]);

  const handleStop = useCallback(() => {
    abortControllerRef.current?.abort();
    setRunnerState('cancelled');
  }, []);

  const handleClear = useCallback(() => {
    setTrials([]);
    setBestTrial(null);
    setCurrentTrialIndex(0);
    setTotalTrials(0);
    setRunnerState('idle');
    setErrorMessage('');
  }, []);

  return (
    <div className="playground-autotest-container">
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

      {/* Baseline Parameters */}
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
                onChange={(e) => setBaseline((b) => ({ ...b, temperature: Number(e.target.value) }))}
                step={0.1}
                disabled={isRunning}
              />
            </div>
            <div className="form-group">
              <label>Top P</label>
              <input
                type="number"
                value={baseline.top_p}
                onChange={(e) => setBaseline((b) => ({ ...b, top_p: Number(e.target.value) }))}
                step={0.1}
                disabled={isRunning}
              />
            </div>
            <div className="form-group">
              <label>Top K</label>
              <input
                type="number"
                value={baseline.top_k}
                onChange={(e) => setBaseline((b) => ({ ...b, top_k: Number(e.target.value) }))}
                disabled={isRunning}
              />
            </div>
            <div className="form-group">
              <label>Min P</label>
              <input
                type="number"
                value={baseline.min_p}
                onChange={(e) => setBaseline((b) => ({ ...b, min_p: Number(e.target.value) }))}
                step={0.01}
                disabled={isRunning}
              />
            </div>
          </div>
        )}
      </div>

      {/* Action Buttons */}
      <div className="playground-autotest-actions">
        <button
          className="btn btn-primary"
          onClick={handleRun}
          disabled={!session || isRunning || !hasEnabledScenario}
        >
          Run Automated Testing
        </button>
        {isRunning && (
          <button className="btn btn-danger" onClick={handleStop}>
            Stop
          </button>
        )}
        {trials.length > 0 && !isRunning && (
          <button className="btn btn-secondary btn-small" onClick={handleClear}>
            Clear Results
          </button>
        )}
      </div>

      {/* Template Repair Status */}
      {runnerState === 'repairing_template' && (
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
        </div>
      )}

      {/* Results Table */}
      {trials.length > 0 && (
        <div className="playground-autotest-results">
          <h4>Results</h4>
          <table className="playground-autotest-table">
            <thead>
              <tr>
                <th>#</th>
                <th>Temperature</th>
                <th>Top P</th>
                <th>Top K</th>
                <th>Min P</th>
                <th>Chat Score</th>
                <th>Tool Score</th>
                <th>Total Score</th>
                <th>Avg TPS</th>
              </tr>
            </thead>
            <tbody>
              {trials.map((trial, i) => {
                const isBest = bestTrial && trial === bestTrial && runnerState === 'completed';
                const isPending = trial.totalScore === undefined || trial.totalScore === null;

                return (
                  <tr
                    key={i}
                    style={isBest ? { backgroundColor: '#e8f5e9' } : undefined}
                  >
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
      {bestTrial && runnerState === 'completed' && (
        <div className="playground-autotest-best">
          <h4>Best Configuration Found</h4>
          <div className="playground-autotest-best-details">
            <div><strong>Temperature:</strong> {bestTrial.candidate.temperature}</div>
            <div><strong>Top P:</strong> {bestTrial.candidate.top_p}</div>
            <div><strong>Top K:</strong> {bestTrial.candidate.top_k}</div>
            <div><strong>Min P:</strong> {bestTrial.candidate.min_p}</div>
            <div><strong>Chat Score:</strong> {getScenarioScore(bestTrial, 'chat') ?? '—'}</div>
            <div><strong>Tool Score:</strong> {getScenarioScore(bestTrial, 'tool_call') ?? '—'}</div>
            <div><strong>Total Score:</strong> {bestTrial.totalScore}</div>
            <div><strong>Avg TPS:</strong> {bestTrial.avgTPS?.toFixed(1)}</div>
          </div>
        </div>
      )}
    </div>
  );
}
