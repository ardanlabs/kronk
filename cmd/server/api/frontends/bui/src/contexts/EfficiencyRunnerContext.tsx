import {
  createContext,
  useContext,
  useState,
  useRef,
  useEffect,
  type ReactNode,
} from 'react';
import { api } from '../services/api';
import { useModelList } from './ModelListContext';
import type { EfficiencyResponse } from '../types';

// Maximum models that can be selected for a comparison. Bump this to allow more
// columns; everything else is array/Set-backed and scales automatically.
export const MAX_MODELS = 2;

export const DEFAULT_MAX_TOKENS = 512;

export type RunStatus = 'idle' | 'loading' | 'running' | 'done' | 'error';

// RunState tracks one model's run lifecycle in the Manual tab.
export interface RunState {
  status: RunStatus;
  result?: EfficiencyResponse;
  error?: string;
}

// isChatModel filters out embedding/rerank models, matching the Chat app.
export function isChatModel(id: string): boolean {
  const lid = id.toLowerCase();
  return !lid.includes('embed') && !lid.includes('rerank');
}

// A compact summary of the run currently in flight, for the sidebar indicator.
export interface EfficiencyActiveRun {
  model: string;
  status: RunStatus;
  done: number;
  total: number;
}

export interface EfficiencyCompletedRun {
  ok: boolean;
  title: string;
  summary: string;
}

interface EfficiencyRunnerValue {
  // Selection + inputs.
  selectedModels: Set<string>;
  toggleModel: (id: string) => void;
  prompt: string;
  setPrompt: (p: string) => void;
  maxTokens: number;
  setMaxTokens: (n: number) => void;
  clear: () => void;

  // Per-model run state.
  runs: Map<string, RunState>;
  runModel: (id: string) => void;
  runAll: () => void;
  stopActive: () => void;

  // Sidebar indicator.
  isRunning: boolean;
  activeRun: EfficiencyActiveRun | null;
  completedRun: EfficiencyCompletedRun | null;
  dismissCompleted: () => void;
}

const EfficiencyRunnerContext = createContext<EfficiencyRunnerValue | null>(null);

export function EfficiencyRunnerProvider({ children }: { children: ReactNode }) {
  const { loadModels } = useModelList();

  const [selectedModels, setSelectedModels] = useState<Set<string>>(new Set());
  const [prompt, setPrompt] = useState('');
  const [maxTokens, setMaxTokens] = useState(DEFAULT_MAX_TOKENS);
  const [runs, setRuns] = useState<Map<string, RunState>>(new Map());
  const [completedRun, setCompletedRun] = useState<EfficiencyCompletedRun | null>(null);

  // Only one run may be active at a time. The ref is the synchronous lock
  // (checked/set in the same tick to block overlapping runs); runningModel is
  // the state mirror that the UI reads, so the sidebar indicator can't drift
  // from the ref. This matches the Accuracy/AutoTest runner contexts.
  const runningRef = useRef<string | null>(null);
  const [runningModel, setRunningModel] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const stopAllRef = useRef(false);

  // Models that have completed at least one run this session are "warm" — their
  // next run shows "running" rather than "loading".
  const warmedRef = useRef<Set<string>>(new Set());

  // Load models on mount.
  useEffect(() => {
    loadModels();
  }, [loadModels]);

  // Auto-dismiss the completion notice.
  useEffect(() => {
    if (!completedRun) return;
    const t = setTimeout(() => setCompletedRun(null), 10000);
    return () => clearTimeout(t);
  }, [completedRun]);

  function dismissCompleted() {
    setCompletedRun(null);
  }

  function toggleModel(id: string) {
    // Don't change selection while a run is active.
    if (runningRef.current) return;
    setSelectedModels((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
        return next;
      }
      // FIFO: dropping the earliest-selected when full so you can swap freely.
      if (next.size >= MAX_MODELS) {
        const oldest = next.values().next().value;
        if (oldest !== undefined) next.delete(oldest);
      }
      next.add(id);
      return next;
    });
    // Clear that model's stale result when its selection changes.
    setRuns((prev) => {
      const next = new Map(prev);
      next.delete(id);
      return next;
    });
  }

  function clear() {
    if (runningRef.current) return;
    setPrompt('');
    setRuns(new Map());
  }

  // setRunState updates one model's run entry.
  function setRunState(id: string, state: RunState) {
    setRuns((prev) => {
      const next = new Map(prev);
      next.set(id, state);
      return next;
    });
  }

  // runOne performs a single timed run. Caller owns the lock.
  async function runOne(id: string): Promise<boolean> {
    if (!prompt.trim()) return false;

    runningRef.current = id;
    setRunningModel(id);
    const cold = !warmedRef.current.has(id);
    setRunState(id, { status: cold ? 'loading' : 'running' });

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      const result = await api.runEfficiency(id, prompt, maxTokens, controller.signal);
      warmedRef.current.add(id);
      setRunState(id, { status: 'done', result });
      return true;
    } catch (err) {
      if (controller.signal.aborted) {
        setRunState(id, { status: 'idle' });
        return false;
      }
      setRunState(id, { status: 'error', error: err instanceof Error ? err.message : 'failed' });
      return false;
    } finally {
      abortRef.current = null;
      runningRef.current = null;
      setRunningModel(null);
    }
  }

  function runModel(id: string) {
    if (runningRef.current) return;
    if (!selectedModels.has(id)) return;
    void runOne(id);
  }

  async function runAll() {
    if (runningRef.current) return;
    const targets = Array.from(selectedModels);
    if (targets.length === 0 || !prompt.trim()) return;

    stopAllRef.current = false;
    setCompletedRun(null);

    let ok = 0;
    let errCount = 0;
    for (const id of targets) {
      if (stopAllRef.current) break;
      const success = await runOne(id);
      if (success) ok++;
      else if (!stopAllRef.current) errCount++;
    }

    if (!stopAllRef.current) {
      setCompletedRun({
        ok: errCount === 0,
        title: 'Efficiency run done',
        summary:
          `${ok}/${targets.length} models` +
          (errCount > 0 ? ` · ${errCount} error${errCount > 1 ? 's' : ''}` : ''),
      });
    }
  }

  function stopActive() {
    stopAllRef.current = true;
    abortRef.current?.abort();
  }

  // Sidebar indicator summary, derived from state (not the ref) so it stays in
  // sync with what React renders.
  const isRunning = runningModel !== null;
  let activeRun: EfficiencyActiveRun | null = null;
  if (runningModel) {
    const state = runs.get(runningModel);
    const total = selectedModels.size;
    const done = Array.from(runs.values()).filter((r) => r.status === 'done' || r.status === 'error').length;
    activeRun = { model: runningModel, status: state?.status ?? 'running', done, total };
  }

  return (
    <EfficiencyRunnerContext.Provider
      value={{
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
        activeRun,
        completedRun,
        dismissCompleted,
      }}
    >
      {children}
    </EfficiencyRunnerContext.Provider>
  );
}

export function useEfficiencyRunner() {
  const context = useContext(EfficiencyRunnerContext);
  if (!context) {
    throw new Error('useEfficiencyRunner must be used within an EfficiencyRunnerProvider');
  }
  return context;
}
