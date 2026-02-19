import { createContext, useContext, useState, useCallback, useRef, useEffect, type ReactNode } from 'react';
import type {
  AutoTestRunnerState,
  AutoTestTrialResult,
  SamplingCandidate,
  AutoTestScenario,
  ConfigSweepDefinition,
  ConfigCandidate,
  AutoTestSessionSeed,
  BestConfigWeights,
} from '../types';
import {
  chatScenario,
  toolCallScenario,
  generateTrialCandidates,
  generateConfigCandidates,
  probeTemplate,
  runTrial,
  computeCompositeScore,
} from '../services/autoTestRunner';
import { api } from '../services/api';

export interface EnabledScenarios {
  chat: boolean;
  tool_call: boolean;
}

interface AutoTestRunBase {
  runId: string;
  kind: 'sampling' | 'config';
  status: AutoTestRunnerState;
  errorMessage?: string;
  templateRepairStatus?: string;
  enabledScenarios: EnabledScenarios;
  currentTrialIndex: number;
  totalTrials: number;
}

export interface SamplingRun extends AutoTestRunBase {
  kind: 'sampling';
  sessionId: string;
  useCustomBaseline: boolean;
  baseline: SamplingCandidate;
  maxTrials: number;
  trials: AutoTestTrialResult[];
  bestTrialId?: string;
}

export interface ConfigTrialResult extends AutoTestTrialResult {
  config?: ConfigCandidate;
  error?: string;
}

export interface ConfigRun extends AutoTestRunBase {
  kind: 'config';
  sessionSeed: AutoTestSessionSeed;
  configSweepDef: ConfigSweepDefinition;
  trials: ConfigTrialResult[];
  bestTrialId?: string;
}

export type AutoTestRun = SamplingRun | ConfigRun;

interface AutoTestRunnerContextType {
  run: AutoTestRun | null;
  isRunning: boolean;

  startSamplingRun(args: {
    sessionId: string;
    enabledScenarios: EnabledScenarios;
    useCustomBaseline: boolean;
    baseline: SamplingCandidate;
    maxTrials: number;
    weights: BestConfigWeights;
    repeats: number;
  }): void;

  startConfigRun(args: {
    sessionSeed: AutoTestSessionSeed;
    enabledScenarios: EnabledScenarios;
    configSweepDef: ConfigSweepDefinition;
    weights: BestConfigWeights;
    repeats: number;
  }): void;

  stopRun(): void;
  clearRun(): void;
  reevaluateBestTrial(weights: BestConfigWeights): void;
}

const AutoTestRunnerContext = createContext<AutoTestRunnerContextType | null>(null);

export function AutoTestRunnerProvider({ children }: { children: ReactNode }) {
  const [run, setRun] = useState<AutoTestRun | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const currentConfigSessionRef = useRef<string | null>(null);

  const isRunning = run?.status === 'repairing_template' || run?.status === 'running_trials';

  useEffect(() => {
    return () => {
      abortControllerRef.current?.abort();
      const sid = currentConfigSessionRef.current;
      if (sid) api.deletePlaygroundSession(sid).catch(() => {});
      currentConfigSessionRef.current = null;
    };
  }, []);

  const startSamplingRun = useCallback(({ sessionId, enabledScenarios, useCustomBaseline, baseline, maxTrials, weights, repeats }: {
    sessionId: string;
    enabledScenarios: EnabledScenarios;
    useCustomBaseline: boolean;
    baseline: SamplingCandidate;
    maxTrials: number;
    weights: BestConfigWeights;
    repeats: number;
  }) => {
    if (abortControllerRef.current) return;

    const runId = `run-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const controller = new AbortController();
    abortControllerRef.current = controller;

    setRun({
      runId,
      kind: 'sampling',
      status: 'repairing_template',
      enabledScenarios,
      currentTrialIndex: 0,
      totalTrials: 0,
      sessionId,
      useCustomBaseline,
      baseline,
      maxTrials,
      trials: [],
    });

    (async () => {
      try {
        const scenarios: AutoTestScenario[] = [];
        if (enabledScenarios.chat) scenarios.push(chatScenario);
        if (enabledScenarios.tool_call) scenarios.push(toolCallScenario);

        if (enabledScenarios.tool_call) {
          setRun(prev => prev ? { ...prev, templateRepairStatus: 'Probing template for tool calling compatibility...' } : prev);
          const probeResult = await probeTemplate(sessionId, controller.signal);
          if (probeResult) {
            setRun(prev => prev ? { ...prev, templateRepairStatus: 'Template OK ✓' } : prev);
          } else {
            setRun(prev => prev ? { ...prev, templateRepairStatus: 'Template probe failed — running chat-only tests' } : prev);
            const idx = scenarios.indexOf(toolCallScenario);
            if (idx !== -1) scenarios.splice(idx, 1);
          }
        }

        const candidates = generateTrialCandidates(baseline, maxTrials);
        setRun(prev => prev ? { ...prev, status: 'running_trials', totalTrials: candidates.length, currentTrialIndex: 0 } : prev);

        let bestComposite = -Infinity;
        let bestTPS = -Infinity;
        let bestId: string | undefined;

        for (let i = 0; i < candidates.length; i++) {
          if (controller.signal.aborted) break;

          const candidate = candidates[i];
          const onUpdate = (partial: AutoTestTrialResult) => {
            setRun(prev => {
              if (!prev || prev.kind !== 'sampling') return prev;
              const trials = [...prev.trials];
              trials[i] = partial;
              return { ...prev, trials };
            });
          };

          const result = await runTrial(sessionId, candidate, scenarios, onUpdate, controller.signal, weights, repeats);

          const composite = computeCompositeScore(result, weights);
          const resultTPS = result.avgTPS ?? -Infinity;
          const isBetter = composite > bestComposite + 1e-6
            || (Math.abs(composite - bestComposite) <= 1e-6 && resultTPS > bestTPS);
          const newBestId = isBetter ? result.id : bestId;
          if (isBetter) {
            bestComposite = composite;
            bestTPS = resultTPS;
            bestId = result.id;
          }

          setRun(prev => {
            if (!prev || prev.kind !== 'sampling') return prev;
            const trials = [...prev.trials];
            trials[i] = result;
            return { ...prev, trials, currentTrialIndex: i + 1, bestTrialId: newBestId };
          });
        }

        setRun(prev => prev ? { ...prev, status: controller.signal.aborted ? 'cancelled' : 'completed' } : prev);
        abortControllerRef.current = null;
      } catch (err: any) {
        if (err instanceof DOMException && err.name === 'AbortError') {
          setRun(prev => prev ? { ...prev, status: 'cancelled' } : prev);
          abortControllerRef.current = null;
        } else {
          setRun(prev => prev ? { ...prev, errorMessage: err.message || 'Automated testing failed', status: 'error' } : prev);
          abortControllerRef.current = null;
        }
      }
    })();
  }, []);

  const startConfigRun = useCallback(({ sessionSeed, enabledScenarios, configSweepDef, weights, repeats }: {
    sessionSeed: AutoTestSessionSeed;
    enabledScenarios: EnabledScenarios;
    configSweepDef: ConfigSweepDefinition;
    weights: BestConfigWeights;
    repeats: number;
  }) => {
    if (abortControllerRef.current) return;

    const runId = `run-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const controller = new AbortController();
    abortControllerRef.current = controller;

    setRun({
      runId,
      kind: 'config',
      status: 'running_trials',
      enabledScenarios,
      currentTrialIndex: 0,
      totalTrials: 0,
      sessionSeed,
      configSweepDef,
      trials: [],
    });

    (async () => {
      try {
        const configCandidates = generateConfigCandidates(sessionSeed.base_config, configSweepDef);

        if (configCandidates.length === 0) {
          setRun(prev => prev ? { ...prev, errorMessage: 'No valid config candidates generated (check that nubatch does not exceed nbatch)', status: 'error' } : prev);
          abortControllerRef.current = null;
          return;
        }

        setRun(prev => prev ? { ...prev, totalTrials: configCandidates.length } : prev);

        const scenarios: AutoTestScenario[] = [];
        if (enabledScenarios.chat) scenarios.push(chatScenario);
        if (enabledScenarios.tool_call) scenarios.push(toolCallScenario);

        let bestComposite = -Infinity;
        let bestTPS = -Infinity;
        let bestId: string | undefined;

        const activeBaseline: SamplingCandidate = {};

        for (let i = 0; i < configCandidates.length; i++) {
          if (controller.signal.aborted) break;

          const candidate = configCandidates[i];
          const { 'cache-type': cacheType, ...cfgRest } = candidate;
          const apiCfg = {
            ...cfgRest,
            ...(cacheType !== undefined && { 'cache-type-k': cacheType, 'cache-type-v': cacheType }),
          };
          let configSessionId: string | null = null;

          try {
            const resp = await api.createPlaygroundSession({
              model_id: sessionSeed.model_id,
              template_mode: sessionSeed.template_mode,
              template_name: sessionSeed.template_name,
              template_script: sessionSeed.template_script,
              config: { ...sessionSeed.base_config, ...apiCfg },
            });
            configSessionId = resp.session_id;
            currentConfigSessionRef.current = configSessionId;

            const activeScenarios = [...scenarios];

            const onUpdate = (partial: AutoTestTrialResult) => {
              setRun(prev => {
                if (!prev || prev.kind !== 'config') return prev;
                const trials = [...prev.trials];
                trials[i] = { ...partial, config: candidate };
                return { ...prev, trials };
              });
            };

            const result = await runTrial(configSessionId, activeBaseline, activeScenarios, onUpdate, controller.signal, weights, repeats);

            const configResult: ConfigTrialResult = { ...result, config: candidate };
            const composite = computeCompositeScore(result, weights);
            const resultTPS = result.avgTPS ?? -Infinity;
            const isBetter = composite > bestComposite + 1e-6
              || (Math.abs(composite - bestComposite) <= 1e-6 && resultTPS > bestTPS);
            const newBestId = isBetter ? result.id : bestId;
            if (isBetter) {
              bestComposite = composite;
              bestTPS = resultTPS;
              bestId = result.id;
            }

            setRun(prev => {
              if (!prev || prev.kind !== 'config') return prev;
              const trials = [...prev.trials];
              trials[i] = configResult;
              return { ...prev, trials, currentTrialIndex: i + 1, bestTrialId: newBestId };
            });
          } catch (innerErr: any) {
            if (innerErr instanceof DOMException && innerErr.name === 'AbortError') {
              throw innerErr;
            }

            const errorMessage = innerErr instanceof Error ? innerErr.message : String(innerErr);
            const failedResult: ConfigTrialResult = {
              id: `trial-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
              status: 'completed',
              candidate: activeBaseline,
              startedAt: new Date().toISOString(),
              finishedAt: new Date().toISOString(),
              scenarioResults: [],
              totalScore: 0,
              avgTPS: undefined,
              config: candidate,
              error: errorMessage,
            };

            setRun(prev => {
              if (!prev || prev.kind !== 'config') return prev;
              const trials = [...prev.trials];
              trials[i] = failedResult;
              return { ...prev, trials, currentTrialIndex: i + 1 };
            });
          } finally {
            if (configSessionId) {
              await api.deletePlaygroundSession(configSessionId).catch(() => {});
              if (currentConfigSessionRef.current === configSessionId) {
                currentConfigSessionRef.current = null;
              }
            }
          }
        }

        setRun(prev => prev ? { ...prev, status: controller.signal.aborted ? 'cancelled' : 'completed' } : prev);
        abortControllerRef.current = null;
      } catch (err: any) {
        if (err instanceof DOMException && err.name === 'AbortError') {
          setRun(prev => prev ? { ...prev, status: 'cancelled' } : prev);
          abortControllerRef.current = null;
        } else {
          setRun(prev => prev ? { ...prev, errorMessage: err.message || 'Automated testing failed', status: 'error' } : prev);
          abortControllerRef.current = null;
        }
      }
    })();
  }, []);

  const stopRun = useCallback(() => {
    abortControllerRef.current?.abort();
    const sid = currentConfigSessionRef.current;
    if (sid) {
      api.deletePlaygroundSession(sid).catch(() => {});
      currentConfigSessionRef.current = null;
    }
    setRun(prev => prev ? { ...prev, status: 'cancelled' } : prev);
  }, []);

  const clearRun = useCallback(() => {
    if (abortControllerRef.current) return;
    setRun(null);
  }, []);

  const reevaluateBestTrial = useCallback((weights: BestConfigWeights) => {
    setRun(prev => {
      if (!prev || prev.status !== 'completed') return prev;
      let bestComposite = -Infinity;
      let bestTPS = -Infinity;
      let bestId: string | undefined;
      for (const trial of prev.trials) {
        if (!trial) continue;
        if (trial.totalScore === undefined || trial.totalScore === null) continue;
        if (prev.kind === 'config' && (trial as ConfigTrialResult).error) continue;
        const composite = computeCompositeScore(trial, weights);
        const tps = trial.avgTPS ?? -Infinity;
        const isBetter = composite > bestComposite + 1e-6
          || (Math.abs(composite - bestComposite) <= 1e-6 && tps > bestTPS);
        if (isBetter) {
          bestComposite = composite;
          bestTPS = tps;
          bestId = trial.id;
        }
      }
      return { ...prev, bestTrialId: bestId };
    });
  }, []);

  return (
    <AutoTestRunnerContext.Provider value={{ run, isRunning, startSamplingRun, startConfigRun, stopRun, clearRun, reevaluateBestTrial }}>
      {children}
    </AutoTestRunnerContext.Provider>
  );
}

export function useAutoTestRunner() {
  const context = useContext(AutoTestRunnerContext);
  if (!context) {
    throw new Error('useAutoTestRunner must be used within an AutoTestRunnerProvider');
  }
  return context;
}
