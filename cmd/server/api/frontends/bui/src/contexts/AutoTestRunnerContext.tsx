import { createContext, useContext, useState, useCallback, useRef, useEffect, type ReactNode } from 'react';
import type {
  AutoTestRunnerState,
  AutoTestTrialResult,
  SamplingCandidate,
  SamplingSweepDefinition,
  AutoTestScenario,
  ConfigSweepDefinition,
  ConfigCandidate,
  AutoTestSessionSeed,
  BestConfigWeights,
} from '../types';
import {
  chatScenario,
  toolCallScenario,
  configChatScenario,
  configToolCallScenario,
  generateTrialCandidates,
  generateConfigCandidates,
  probeTemplate,
  runTrial,
  computeCompositeScore,
  calibrateContextFillPrompts,
  extractContextWindow,
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
  calibrationStatus?: string;
  enabledScenarios: EnabledScenarios;
  currentTrialIndex: number;
  totalTrials: number;
}

export interface SamplingRun extends AutoTestRunBase {
  kind: 'sampling';
  sessionId: string;
  sweepDef: SamplingSweepDefinition;
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
    sweepDef: SamplingSweepDefinition;
    maxTrials: number;
    weights: BestConfigWeights;
    repeats: number;
    effectiveConfig?: Record<string, unknown>;
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

  const startSamplingRun = useCallback(({ sessionId, enabledScenarios, sweepDef, maxTrials, weights, repeats, effectiveConfig }: {
    sessionId: string;
    enabledScenarios: EnabledScenarios;
    sweepDef: SamplingSweepDefinition;
    maxTrials: number;
    weights: BestConfigWeights;
    repeats: number;
    effectiveConfig?: Record<string, unknown>;
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
      sweepDef,
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

        // Calibrate context-fill prompts if chat scenario is enabled
        if (enabledScenarios.chat) {
          const contextWindow = extractContextWindow(effectiveConfig);
          if (contextWindow && contextWindow > 0) {
            setRun(prev => prev ? { ...prev, calibrationStatus: 'Calibrating context fill prompts...' } : prev);
            try {
              const ctxFillPrompts = await calibrateContextFillPrompts(sessionId, contextWindow, controller.signal);
              if (ctxFillPrompts.length > 0) {
                const chatIdx = scenarios.findIndex(s => s.id === 'chat');
                if (chatIdx >= 0) {
                  scenarios[chatIdx] = {
                    ...scenarios[chatIdx],
                    prompts: [...scenarios[chatIdx].prompts, ...ctxFillPrompts],
                  };
                }
                setRun(prev => prev ? { ...prev, calibrationStatus: `Calibrated ${ctxFillPrompts.length} context fill levels ✓` } : prev);
              } else {
                setRun(prev => prev ? { ...prev, calibrationStatus: 'Context window too small for fill tests' } : prev);
              }
            } catch {
              setRun(prev => prev ? { ...prev, calibrationStatus: 'Context fill calibration failed — skipping fill tests' } : prev);
            }
          }
        }

        const candidates = generateTrialCandidates(sweepDef, maxTrials);
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
        if (enabledScenarios.chat) scenarios.push(configChatScenario);
        if (enabledScenarios.tool_call) scenarios.push(configToolCallScenario);

        let bestComposite = -Infinity;
        let bestTPS = -Infinity;
        let bestId: string | undefined;

        const activeBaseline: SamplingCandidate = {
          temperature: 0,
          top_p: 1,
          top_k: 1,
          min_p: 0,
          repeat_penalty: 1,
          frequency_penalty: 0,
          presence_penalty: 0,
        };

        for (let i = 0; i < configCandidates.length; i++) {
          if (controller.signal.aborted) break;

          const candidate = configCandidates[i];
          const { 'cache_type': cacheType, 'cache_mode': cacheMode, ...cfgRest } = candidate;
          const apiCfg = {
            ...cfgRest,
            ...(cacheType !== undefined && { 'cache_type_k': cacheType, 'cache_type_v': cacheType }),
            ...(cacheMode !== undefined && {
              'system_prompt_cache': cacheMode === 'spc',
              'incremental_cache': cacheMode === 'imc',
            }),
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

            if (enabledScenarios.tool_call) {
              const probeResult = await probeTemplate(configSessionId, controller.signal);
              if (!probeResult) {
                const idx = activeScenarios.indexOf(configToolCallScenario);
                if (idx !== -1) activeScenarios.splice(idx, 1);
              }
            }

            // Calibrate context-fill prompts for this config's context window
            if (enabledScenarios.chat) {
              const cfgContextWindow = extractContextWindow(resp.effective_config, sessionSeed.base_config);
              if (cfgContextWindow && cfgContextWindow > 0) {
                try {
                  const ctxFillPrompts = await calibrateContextFillPrompts(configSessionId, cfgContextWindow, controller.signal);
                  if (ctxFillPrompts.length > 0) {
                    const chatIdx = activeScenarios.findIndex(s => s.id === 'chat');
                    if (chatIdx >= 0) {
                      activeScenarios[chatIdx] = {
                        ...activeScenarios[chatIdx],
                        prompts: [...activeScenarios[chatIdx].prompts, ...ctxFillPrompts],
                      };
                    }
                  }
                } catch {
                  // Calibration failed; skip fill tests for this config
                }
              }
            }

            const onUpdate = (partial: AutoTestTrialResult) => {
              setRun(prev => {
                if (!prev || prev.kind !== 'config') return prev;
                const trials = [...prev.trials];
                trials[i] = { ...partial, config: candidate };
                return { ...prev, trials };
              });
            };

            const effectiveNSeqMax = (candidate['nseq_max'] as number | undefined) ?? sessionSeed.base_config['nseq_max'] ?? 1;
            const result = await runTrial(configSessionId, activeBaseline, activeScenarios, onUpdate, controller.signal, weights, repeats,
              effectiveNSeqMax > 1 ? { concurrency: effectiveNSeqMax } : undefined,
            );

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
