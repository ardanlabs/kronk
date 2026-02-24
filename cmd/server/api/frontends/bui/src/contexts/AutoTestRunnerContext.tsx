import { createContext, useContext, useState, useCallback, useRef, useEffect, type ReactNode } from 'react';
import type {
  AutoTestRunnerState,
  AutoTestTrialResult,
  AutoTestLogEntry,
  SamplingCandidate,
  SamplingSweepDefinition,
  AutoTestScenario,
  ConfigSweepDefinition,
  ConfigCandidate,
  ModelCaps,
  AutoTestSessionSeed,
  BestConfigWeights,
} from '../types';
import {
  chatScenario,
  toolCallScenario,
  configPerfScenario,
  generateTrialCandidates,
  generateConfigCandidates,
  probeTemplate,
  runTrial,
  computeCompositeScore,
  calibrateContextFillPrompts,
  extractContextWindow,
  TRIAL_PAUSE_MS,
} from '../services/autoTestRunner';
import { api } from '../services/api';

function mergeLogEntries(
  prevLogs: AutoTestLogEntry[],
  contextLogs: AutoTestLogEntry[],
  runnerLogs?: AutoTestLogEntry[],
): AutoTestLogEntry[] {
  const all = [...prevLogs, ...contextLogs, ...(runnerLogs ?? [])];
  const seen = new Set<string>();
  return all.filter(e => {
    const k = `${e.timestamp}|${e.message}`;
    if (seen.has(k)) return false;
    seen.add(k);
    return true;
  });
}

function abortableSleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) { reject(new DOMException('Aborted', 'AbortError')); return; }
    const timer = setTimeout(() => { signal.removeEventListener('abort', onAbort); resolve(); }, ms);
    function onAbort() { clearTimeout(timer); reject(new DOMException('Aborted', 'AbortError')); }
    signal.addEventListener('abort', onAbort, { once: true });
  });
}

async function runTrialLoop<T extends AutoTestTrialResult>(params: {
  signal: AbortSignal;
  weights: BestConfigWeights;
  isStale: () => boolean;
  getNextQueuedTrialId: () => string | undefined;
  /** Atomically claim a queued trial (queued→running). Returns the trial's
   *  current index, or -1 if the trial was already skipped/started. */
  claimTrial: (trialId: string) => number;
  executeTrial: (trialId: string, idx: number) => Promise<T | null>;
  onTrialResult: (trialId: string, idx: number, result: T, bestTrialId: string | undefined) => void;
}): Promise<string | undefined> {
  let bestComposite = -Infinity;
  let bestTPS = -Infinity;
  let bestId: string | undefined;
  let hasRunOne = false;

  for (;;) {
    if (params.signal.aborted || params.isStale()) break;

    // Sleep *before* selecting the next trial so that reorder/skip actions
    // taken during the pause are visible to getNextQueuedTrialId().
    if (hasRunOne) {
      try { await abortableSleep(TRIAL_PAUSE_MS, params.signal); }
      catch (e) {
        if (e instanceof DOMException && e.name === 'AbortError') break;
        throw e;
      }
    }
    if (params.signal.aborted || params.isStale()) break;

    const trialId = params.getNextQueuedTrialId();
    if (trialId === undefined) break;

    // Atomically claim the trial (queued→running).  If it was skipped or
    // already started between selection and now, claimTrial returns -1.
    const idx = params.claimTrial(trialId);
    if (idx < 0) continue;

    hasRunOne = true;

    if (params.signal.aborted || params.isStale()) break;

    const result = await params.executeTrial(trialId, idx);
    if (!result || params.isStale()) continue;

    const composite = computeCompositeScore(result, params.weights);
    const resultTPS = result.avgTPS ?? -Infinity;
    const isBetter = composite > bestComposite + 1e-6
      || (Math.abs(composite - bestComposite) <= 1e-6 && resultTPS > bestTPS);
    if (isBetter) {
      bestComposite = composite;
      bestTPS = resultTPS;
      bestId = result.id;
    }

    params.onTrialResult(trialId, idx, result, bestId);
  }

  return bestId;
}

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
  runStartedAt?: string;
  repeats: number;
  weights: BestConfigWeights;
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
    sessionId?: string;
    sessionSeed?: AutoTestSessionSeed;
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
  moveQueuedTrial(args: { trialId: string; direction: 'up' | 'down' }): void;
  reorderQueuedTrial(args: { trialId: string; targetId: string }): void;
  skipTrial(args: { trialId: string }): void;
  unskipTrial(args: { trialId: string }): void;
}

const AutoTestRunnerContext = createContext<AutoTestRunnerContextType | null>(null);

export function AutoTestRunnerProvider({ children }: { children: ReactNode }) {
  const [run, setRunRaw] = useState<AutoTestRun | null>(null);
  const runRef = useRef<AutoTestRun | null>(null);

  // Update runRef synchronously so the async trial loop always sees the latest
  // state.  We read from runRef.current (the authoritative "prev"), compute
  // the next value, write it back, and then tell React to catch up.  This
  // avoids the React 18 batching problem where setState updater functions are
  // deferred to the next render.
  const setRun = useCallback(
    (updater: AutoTestRun | null | ((prev: AutoTestRun | null) => AutoTestRun | null)) => {
      const prev = runRef.current;
      const next = typeof updater === 'function' ? updater(prev) : updater;
      runRef.current = next;
      setRunRaw(next);
    },
    [],
  );
  // Belt-and-suspenders: keep runRef eventually consistent with React state
  // in case any code path bypasses setRun.
  useEffect(() => { runRef.current = run; }, [run]);

  const abortControllerRef = useRef<AbortController | null>(null);
  const currentConfigSessionRef = useRef<string | null>(null);

  const isRunning = run?.status === 'repairing_template' || run?.status === 'running_trials';

  const currentSamplingSessionRef = useRef<string | null>(null);
  const runTokenRef = useRef(0);

  useEffect(() => {
    return () => {
      abortControllerRef.current?.abort();
      const cfgSid = currentConfigSessionRef.current;
      if (cfgSid) api.deletePlaygroundSession(cfgSid).catch(() => {});
      currentConfigSessionRef.current = null;
      const sampSid = currentSamplingSessionRef.current;
      if (sampSid) api.deletePlaygroundSession(sampSid).catch(() => {});
      currentSamplingSessionRef.current = null;
    };
  }, []);

  const startSamplingRun = useCallback(({ sessionId, sessionSeed, enabledScenarios, sweepDef, maxTrials, weights, repeats, effectiveConfig }: {
    sessionId?: string;
    sessionSeed?: AutoTestSessionSeed;
    enabledScenarios: EnabledScenarios;
    sweepDef: SamplingSweepDefinition;
    maxTrials: number;
    weights: BestConfigWeights;
    repeats: number;
    effectiveConfig?: Record<string, unknown>;
  }) => {
    if (abortControllerRef.current) return;

    const token = ++runTokenRef.current;
    const isStale = () => runTokenRef.current !== token;

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
      sessionId: sessionId ?? '',
      sweepDef,
      maxTrials,
      trials: [],
      repeats,
      weights,
    });

    (async () => {
      try {
        let activeSessionId = sessionId ?? '';
        let activeEffectiveConfig = effectiveConfig;

        // Auto-create session if none provided.
        if (!activeSessionId && sessionSeed?.model_id) {
          setRun(prev => prev && !isStale() ? { ...prev, templateRepairStatus: 'Creating session…' } : prev);
          const resp = await api.createPlaygroundSession({
            model_id: sessionSeed.model_id,
            template_mode: sessionSeed.template_mode,
            template_name: sessionSeed.template_name,
            template_script: sessionSeed.template_script,
            config: sessionSeed.base_config ?? {},
          });
          activeSessionId = resp.session_id;
          activeEffectiveConfig = resp.effective_config;
          currentSamplingSessionRef.current = activeSessionId;

          if (controller.signal.aborted || isStale()) {
            api.deletePlaygroundSession(activeSessionId).catch(() => {});
            currentSamplingSessionRef.current = null;
            return;
          }

          setRun(prev => prev && !isStale() ? { ...prev, sessionId: activeSessionId, templateRepairStatus: 'Session created ✓' } : prev);
        }

        if (!activeSessionId) {
          if (!isStale()) setRun(prev => prev ? { ...prev, status: 'error', errorMessage: 'No session available' } : prev);
          abortControllerRef.current = null;
          return;
        }

        const scenarios: AutoTestScenario[] = [];
        if (enabledScenarios.chat) scenarios.push(chatScenario);
        if (enabledScenarios.tool_call) scenarios.push(toolCallScenario);

        if (enabledScenarios.tool_call) {
          setRun(prev => prev && !isStale() ? { ...prev, templateRepairStatus: 'Probing template for tool calling compatibility...' } : prev);
          const probeResult = await probeTemplate(activeSessionId, controller.signal);
          if (isStale()) return;
          if (probeResult) {
            setRun(prev => prev && !isStale() ? { ...prev, templateRepairStatus: 'Template OK ✓' } : prev);
          } else {
            setRun(prev => prev && !isStale() ? { ...prev, templateRepairStatus: 'Template probe failed — running chat-only tests' } : prev);
            const idx = scenarios.findIndex(s => s.id === 'tool_call');
            if (idx !== -1) scenarios.splice(idx, 1);
          }
        }

        if (scenarios.length === 0) {
          if (!isStale()) setRun(prev => prev ? { ...prev, status: 'error', errorMessage: 'No runnable scenarios — template probe failed and chat is disabled' } : prev);
          abortControllerRef.current = null;
          return;
        }

        // Calibrate context-fill prompts if chat scenario is enabled
        if (enabledScenarios.chat) {
          const contextWindow = extractContextWindow(activeEffectiveConfig);
          if (contextWindow && contextWindow > 0) {
            setRun(prev => prev && !isStale() ? { ...prev, calibrationStatus: 'Calibrating context fill prompts...' } : prev);
            try {
              const ctxFillPrompts = await calibrateContextFillPrompts(activeSessionId, contextWindow, controller.signal);
              if (isStale()) return;
              if (ctxFillPrompts.length > 0) {
                const chatIdx = scenarios.findIndex(s => s.id === 'chat');
                if (chatIdx >= 0) {
                  scenarios[chatIdx] = {
                    ...scenarios[chatIdx],
                    prompts: [...scenarios[chatIdx].prompts, ...ctxFillPrompts],
                  };
                }
                setRun(prev => prev && !isStale() ? { ...prev, calibrationStatus: `Calibrated ${ctxFillPrompts.length} context fill levels ✓` } : prev);
              } else {
                setRun(prev => prev && !isStale() ? { ...prev, calibrationStatus: 'Context window too small for fill tests' } : prev);
              }
            } catch {
              if (isStale()) return;
              setRun(prev => prev && !isStale() ? { ...prev, calibrationStatus: 'Context fill calibration failed — skipping fill tests' } : prev);
            }
          }
        }

        const candidates = generateTrialCandidates(sweepDef, maxTrials);
        const queuedTrials: AutoTestTrialResult[] = candidates.map((c, idx) => ({
          id: `${runId}-trial-${idx}`,
          status: 'queued' as const,
          candidate: c,
          scenarioResults: [],
        }));
        setRun(prev => prev && !isStale() ? { ...prev, status: 'running_trials', runStartedAt: new Date().toISOString(), totalTrials: candidates.length, currentTrialIndex: 0, trials: queuedTrials } : prev);

        await runTrialLoop<AutoTestTrialResult>({
          signal: controller.signal,
          weights,
          isStale,
          getNextQueuedTrialId: () => {
            const r = runRef.current;
            if (!r || r.status !== 'running_trials') return undefined;
            return r.trials.find(t => t.status === 'queued')?.id;
          },
          claimTrial: (trialId) => {
            const trialStartedAt = new Date().toISOString();
            let claimedIndex = -1;
            setRun(prev => {
              if (!prev || prev.kind !== 'sampling' || isStale()) return prev;
              const i = prev.trials.findIndex(t => t.id === trialId);
              if (i < 0 || prev.trials[i].status !== 'queued') return prev;
              const trials = [...prev.trials];
              trials[i] = { ...trials[i], status: 'running', startedAt: trialStartedAt };
              claimedIndex = i;
              return { ...prev, trials };
            });
            return claimedIndex;
          },
          executeTrial: async (trialId) => {
            const r = runRef.current;
            const trial = r?.trials.find(t => t.id === trialId);
            if (!trial) return null;
            const candidate = trial.candidate;
            const onUpdate = (partial: AutoTestTrialResult) => {
              setRun(prev => {
                if (!prev || prev.kind !== 'sampling' || isStale()) return prev;
                const ti = prev.trials.findIndex(t => t.id === trialId);
                if (ti < 0) return prev;
                const trials = [...prev.trials];
                const prevLogs = trials[ti].logEntries ?? [];
                const mergedLogs = mergeLogEntries(prevLogs, [], partial.logEntries);
                trials[ti] = { ...trials[ti], ...partial, logEntries: mergedLogs };
                return { ...prev, trials };
              });
            };
            return await runTrial(activeSessionId, candidate, scenarios, onUpdate, controller.signal, weights, repeats);
          },
          onTrialResult: (trialId, _idx, result, bestTrialId) => {
            setRun(prev => {
              if (!prev || prev.kind !== 'sampling' || isStale()) return prev;
              const i = prev.trials.findIndex(t => t.id === trialId);
              if (i < 0) return prev;
              const trials = [...prev.trials];
              trials[i] = result;
              return { ...prev, trials, currentTrialIndex: i + 1, bestTrialId };
            });
          },
        });

        if (!isStale()) {
          setRun(prev => prev ? { ...prev, status: controller.signal.aborted ? 'cancelled' : 'completed' } : prev);
        }
        abortControllerRef.current = null;
      } catch (err: any) {
        if (isStale()) return;
        if (err instanceof DOMException && err.name === 'AbortError') {
          setRun(prev => prev ? { ...prev, status: 'cancelled' } : prev);
          abortControllerRef.current = null;
        } else {
          setRun(prev => prev ? { ...prev, errorMessage: err.message || 'Automated testing failed', status: 'error' } : prev);
          abortControllerRef.current = null;
        }
      } finally {
        const sid = currentSamplingSessionRef.current;
        if (sid) {
          api.deletePlaygroundSession(sid).catch(() => {});
          currentSamplingSessionRef.current = null;
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

    const token = ++runTokenRef.current;
    const isStale = () => runTokenRef.current !== token;

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
      repeats,
      weights,
    });

    (async () => {
      try {
        // Probe baseline session to detect model capabilities.
        let modelCaps: ModelCaps = {};
        let probeSessionId: string | null = null;
        try {
          const probeResp = await api.createPlaygroundSession({
            model_id: sessionSeed.model_id,
            template_mode: sessionSeed.template_mode,
            template_name: sessionSeed.template_name,
            template_script: sessionSeed.template_script,
            config: {},
          });
          probeSessionId = probeResp.session_id;
          modelCaps = {
            isHybrid: probeResp.effective_config?.['is_hybrid_model'] === true,
            isGPT: probeResp.effective_config?.['is_gpt_model'] === true,
          };
        } catch {
          // Probe failed; proceed without model caps filtering.
        } finally {
          if (probeSessionId) await api.deletePlaygroundSession(probeSessionId).catch(() => {});
        }

        if (isStale()) return;

        const configCandidates = generateConfigCandidates(sessionSeed.base_config, configSweepDef, modelCaps);

        if (configCandidates.length === 0) {
          if (!isStale()) setRun(prev => prev ? { ...prev, errorMessage: 'No valid config candidates generated (check sweep values; hybrid models require f16 cache type and disable flash attention)', status: 'error' } : prev);
          abortControllerRef.current = null;
          return;
        }

        const scenarios: AutoTestScenario[] = [configPerfScenario];

        const activeBaseline: SamplingCandidate = {
          temperature: 0,
          top_p: 1,
          top_k: 1,
          min_p: 0,
          repeat_penalty: 1,
          frequency_penalty: 0,
          presence_penalty: 0,
        };

        const queuedConfigTrials: ConfigTrialResult[] = configCandidates.map((c, idx) => ({
          id: `${runId}-trial-${idx}`,
          status: 'queued' as const,
          candidate: activeBaseline,
          scenarioResults: [],
          config: c,
        }));
        setRun(prev => prev && !isStale() ? { ...prev, status: 'running_trials', runStartedAt: new Date().toISOString(), totalTrials: configCandidates.length, currentTrialIndex: 0, trials: queuedConfigTrials } : prev);

        await runTrialLoop<ConfigTrialResult>({
          signal: controller.signal,
          weights,
          isStale,
          getNextQueuedTrialId: () => {
            const r = runRef.current;
            if (!r || r.kind !== 'config' || r.status !== 'running_trials') return undefined;
            return r.trials.find(t => t.status === 'queued')?.id;
          },
          claimTrial: (trialId) => {
            const trialStartedAt = new Date().toISOString();
            let claimedIndex = -1;
            setRun(prev => {
              if (!prev || prev.kind !== 'config' || isStale()) return prev;
              const i = prev.trials.findIndex(t => t.id === trialId);
              if (i < 0 || prev.trials[i].status !== 'queued') return prev;
              const trials = [...prev.trials];
              trials[i] = { ...trials[i], status: 'running', startedAt: trialStartedAt, logEntries: [] };
              claimedIndex = i;
              return { ...prev, trials };
            });
            return claimedIndex;
          },
          executeTrial: async (trialId) => {
            const r = runRef.current;
            const trial = r?.trials.find(t => t.id === trialId) as ConfigTrialResult | undefined;
            if (!trial?.config) return null;
            const candidate = trial.config;
            const trialLogs: AutoTestLogEntry[] = [];
            const addTrialLog = (message: string) => {
              const entry: AutoTestLogEntry = { timestamp: new Date().toISOString(), message };
              trialLogs.push(entry);
              setRun(prev => {
                if (!prev || prev.kind !== 'config' || isStale()) return prev;
                const ti = prev.trials.findIndex(t => t.id === trialId);
                if (ti < 0) return prev;
                const trials = [...prev.trials];
                trials[ti] = { ...trials[ti], logEntries: [...trialLogs] };
                return { ...prev, trials };
              });
            };

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
              addTrialLog('Creating session with config overrides…');
              const resp = await api.createPlaygroundSession({
                model_id: sessionSeed.model_id,
                template_mode: sessionSeed.template_mode,
                template_name: sessionSeed.template_name,
                template_script: sessionSeed.template_script,
                config: { ...sessionSeed.base_config, ...apiCfg },
              });
              configSessionId = resp.session_id;
              currentConfigSessionRef.current = configSessionId;

              if (controller.signal.aborted || isStale()) {
                throw new DOMException('Aborted', 'AbortError');
              }

              addTrialLog('Session created ✓');

              const activeScenarios = [...scenarios];

              if (activeScenarios.length === 0) {
                throw new Error('No runnable scenarios for this config — template probe failed and chat is disabled');
              }

              // Calibrate context-fill prompts for this config's context window
              if (enabledScenarios.chat) {
                const cfgContextWindow = extractContextWindow(resp.effective_config, sessionSeed.base_config);
                if (cfgContextWindow && cfgContextWindow > 0) {
                  addTrialLog(`Calibrating context fill prompts (ctx=${cfgContextWindow})…`);
                  try {
                    const ctxFillPrompts = await calibrateContextFillPrompts(configSessionId, cfgContextWindow, controller.signal);
                    if (isStale()) throw new DOMException('Aborted', 'AbortError');
                    if (ctxFillPrompts.length > 0) {
                      const chatIdx = activeScenarios.findIndex(s => s.id === 'chat');
                      if (chatIdx >= 0) {
                        activeScenarios[chatIdx] = {
                          ...activeScenarios[chatIdx],
                          prompts: [...activeScenarios[chatIdx].prompts, ...ctxFillPrompts],
                        };
                      }
                      addTrialLog(`Calibrated ${ctxFillPrompts.length} fill levels ✓`);
                    } else {
                      addTrialLog('Context window too small for fill tests');
                    }
                  } catch {
                    addTrialLog('Context fill calibration failed — skipping');
                  }
                }
              }

              addTrialLog('Running scenarios…');

              const onUpdate = (partial: AutoTestTrialResult) => {
                setRun(prev => {
                  if (!prev || prev.kind !== 'config' || isStale()) return prev;
                  const ti = prev.trials.findIndex(t => t.id === trialId);
                  if (ti < 0) return prev;
                  const trials = [...prev.trials];
                  const prevLogs = trials[ti].logEntries ?? [];
                  const mergedLogs = mergeLogEntries(prevLogs, trialLogs, partial.logEntries);
                  trials[ti] = { ...trials[ti], ...partial, config: candidate, logEntries: mergedLogs };
                  return { ...prev, trials };
                });
              };

              const effectiveNSeqMax = (candidate['nseq_max'] as number | undefined) ?? sessionSeed.base_config['nseq_max'] ?? 1;
              const result = await runTrial(configSessionId, activeBaseline, activeScenarios, onUpdate, controller.signal, weights, repeats,
                effectiveNSeqMax > 1 ? { concurrency: effectiveNSeqMax } : undefined,
              );

              return {
                ...result,
                config: candidate,
                logEntries: mergeLogEntries([], trialLogs, result.logEntries),
              } as ConfigTrialResult;
            } catch (innerErr: any) {
              if (innerErr instanceof DOMException && innerErr.name === 'AbortError') {
                throw innerErr;
              }

              const errorMessage = innerErr instanceof Error ? innerErr.message : String(innerErr);

              if (!isStale()) {
                setRun(prev => {
                  if (!prev || prev.kind !== 'config') return prev;
                  const ti = prev.trials.findIndex(t => t.id === trialId);
                  if (ti < 0) return prev;
                  const trials = [...prev.trials];
                  const prevTrial = trials[ti];
                  trials[ti] = {
                    ...prevTrial,
                    id: prevTrial?.id ?? trialId,
                    status: 'failed',
                    startedAt: prevTrial?.startedAt ?? new Date().toISOString(),
                    finishedAt: new Date().toISOString(),
                    scenarioResults: [],
                    totalScore: undefined,
                    avgTPS: undefined,
                    config: candidate,
                    error: errorMessage,
                  };
                  return { ...prev, trials, currentTrialIndex: ti + 1 };
                });
              }
              return null;
            } finally {
              if (configSessionId) {
                await api.deletePlaygroundSession(configSessionId).catch(() => {});
                if (currentConfigSessionRef.current === configSessionId) {
                  currentConfigSessionRef.current = null;
                }
              }
            }
          },
          onTrialResult: (trialId, _idx, result, bestTrialId) => {
            setRun(prev => {
              if (!prev || prev.kind !== 'config' || isStale()) return prev;
              const i = prev.trials.findIndex(t => t.id === trialId);
              if (i < 0) return prev;
              const trials = [...prev.trials];
              trials[i] = result;
              return { ...prev, trials, currentTrialIndex: i + 1, bestTrialId };
            });
          },
        });

        if (!isStale()) {
          setRun(prev => prev ? { ...prev, status: controller.signal.aborted ? 'cancelled' : 'completed' } : prev);
        }
        abortControllerRef.current = null;
      } catch (err: any) {
        if (isStale()) return;
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
    ++runTokenRef.current;
    abortControllerRef.current?.abort();
    abortControllerRef.current = null;
    const cfgSid = currentConfigSessionRef.current;
    if (cfgSid) {
      api.deletePlaygroundSession(cfgSid).catch(() => {});
      currentConfigSessionRef.current = null;
    }
    const sampSid = currentSamplingSessionRef.current;
    if (sampSid) {
      api.deletePlaygroundSession(sampSid).catch(() => {});
      currentSamplingSessionRef.current = null;
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
        if (trial.status === 'skipped') continue;
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

  const moveQueuedTrial = useCallback(({ trialId, direction }: { trialId: string; direction: 'up' | 'down' }) => {
    setRun(prev => {
      if (!prev || prev.status !== 'running_trials') return prev;
      const idx = prev.trials.findIndex(t => t.id === trialId);
      if (idx < 0 || prev.trials[idx].status !== 'queued') return prev;
      const swapIdx = direction === 'up' ? idx - 1 : idx + 1;
      if (swapIdx < 0 || swapIdx >= prev.trials.length) return prev;
      if (prev.trials[swapIdx].status !== 'queued') return prev;
      const trials = [...prev.trials];
      [trials[idx], trials[swapIdx]] = [trials[swapIdx], trials[idx]];
      return { ...prev, trials };
    });
  }, []);

  const reorderQueuedTrial = useCallback(({ trialId, targetId }: { trialId: string; targetId: string }) => {
    if (trialId === targetId) return;
    setRun(prev => {
      if (!prev || prev.status !== 'running_trials') return prev;
      const fromIdx = prev.trials.findIndex(t => t.id === trialId);
      const toIdx = prev.trials.findIndex(t => t.id === targetId);
      if (fromIdx < 0 || toIdx < 0) return prev;
      if (prev.trials[fromIdx].status !== 'queued') return prev;
      const trials = [...prev.trials];
      const [moved] = trials.splice(fromIdx, 1);
      trials.splice(toIdx, 0, moved);
      return { ...prev, trials };
    });
  }, []);

  const skipTrial = useCallback(({ trialId }: { trialId: string }) => {
    setRun(prev => {
      if (!prev || prev.status !== 'running_trials') return prev;
      const idx = prev.trials.findIndex(t => t.id === trialId);
      if (idx < 0 || prev.trials[idx].status !== 'queued') return prev;
      const trials = [...prev.trials] as typeof prev.trials;
      trials[idx] = { ...trials[idx], status: 'skipped', finishedAt: new Date().toISOString() };
      return { ...prev, trials };
    });
  }, []);

  const unskipTrial = useCallback(({ trialId }: { trialId: string }) => {
    setRun(prev => {
      if (!prev || prev.status !== 'running_trials') return prev;
      const idx = prev.trials.findIndex(t => t.id === trialId);
      if (idx < 0 || prev.trials[idx].status !== 'skipped') return prev;
      const trials = [...prev.trials] as typeof prev.trials;
      trials[idx] = { ...trials[idx], status: 'queued', finishedAt: undefined };
      return { ...prev, trials };
    });
  }, []);

  return (
    <AutoTestRunnerContext.Provider value={{ run, isRunning, startSamplingRun, startConfigRun, stopRun, clearRun, reevaluateBestTrial, moveQueuedTrial, reorderQueuedTrial, skipTrial, unskipTrial }}>
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
