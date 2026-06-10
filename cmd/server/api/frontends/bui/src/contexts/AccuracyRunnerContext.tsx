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
import type { AccuracyFunction, AccuracyResponse } from '../types';

export type Mode = 'manual' | 'batch' | 'compare';
export type SortBy = 'line' | 'loc' | 'name';
export type CompareLayout = 'columns' | 'stacked';

// A batch row tracks a single function's run lifecycle.
export interface BatchRow {
  identifier: string;
  line: number;
  loc: number;
  status: 'pending' | 'running' | 'done' | 'error';
  result?: AccuracyResponse;
  error?: string;
}

// A compare cell tracks one model's run of the chosen function.
export interface CompareCell {
  model: string;
  status: 'pending' | 'running' | 'done' | 'error';
  result?: AccuracyResponse;
  error?: string;
}

export const DEFAULT_RANDOM = 5;
export const MAX_RANDOM = 5;
export const MAX_COMPARE = 2;

// isChatModel filters out embedding/rerank models, matching the Chat app.
export function isChatModel(id: string): boolean {
  const lid = id.toLowerCase();
  return !lid.includes('embed') && !lid.includes('rerank');
}

// A compact summary of whatever run is currently in flight, used by the
// sidebar indicator so progress stays visible after navigating away.
export interface AccuracyActiveRun {
  mode: Mode;
  label: string;
  done: number;
  total: number;
}

export interface AccuracyCompletedRun {
  mode: Mode;
  ok: boolean;
  title: string;
  summary: string;
}

interface AccuracyRunnerValue {
  // Shared.
  mode: Mode;
  setMode: (m: Mode) => void;
  selectedModel: string;
  setSelectedModel: (m: string) => void;
  functions: AccuracyFunction[] | null;
  sortBy: SortBy;
  setSortBy: (s: SortBy) => void;
  error: string | null;
  setError: (e: string | null) => void;

  // Manual.
  selectedFn: string;
  setSelectedFn: (f: string) => void;
  running: boolean;
  result: AccuracyResponse | null;
  runManual: () => void;
  stopManual: () => void;
  clearManual: () => void;

  // Batch.
  checked: Set<string>;
  setChecked: (s: Set<string>) => void;
  randomCount: number;
  setRandomCount: (n: number) => void;
  rows: BatchRow[];
  batchRunning: boolean;
  expanded: Set<string>;
  toggle: (id: string) => void;
  clearAll: () => void;
  pickRandom: () => void;
  runBatch: () => void;
  stopBatch: () => void;
  toggleExpand: (id: string) => void;

  // Compare.
  compareModels: Set<string>;
  compareFn: string;
  setCompareFn: (f: string) => void;
  compareCells: CompareCell[];
  comparing: boolean;
  compareLayout: CompareLayout;
  setCompareLayout: (l: CompareLayout) => void;
  toggleCompareModel: (id: string) => void;
  clearCompare: () => void;
  runCompare: () => void;
  stopCompare: () => void;

  // Sidebar indicator helpers.
  isRunning: boolean;
  activeRun: AccuracyActiveRun | null;
  completedRun: AccuracyCompletedRun | null;
  dismissCompleted: () => void;
  stopActive: () => void;
}

const AccuracyRunnerContext = createContext<AccuracyRunnerValue | null>(null);

export function AccuracyRunnerProvider({ children }: { children: ReactNode }) {
  const { models, loadModels } = useModelList();

  const [mode, setMode] = useState<Mode>('manual');
  const [selectedModel, setSelectedModelState] = useState('');

  // Only one accuracy run may be active at a time across all three modes. A
  // synchronous ref lock guards against overlapping runs (cross-mode or rapid
  // double-clicks) more reliably than the async React state flags.
  const activeModeRef = useRef<Mode | null>(null);

  // Completion notification: lingers in the sidebar after a run finishes (so a
  // user who navigated away still sees the result), then auto-dismisses.
  const [completedRun, setCompletedRun] = useState<AccuracyCompletedRun | null>(null);

  function dismissCompleted() {
    setCompletedRun(null);
  }

  useEffect(() => {
    if (!completedRun) return;
    const t = setTimeout(() => setCompletedRun(null), 10000);
    return () => clearTimeout(t);
  }, [completedRun]);

  function beginRun(m: Mode): boolean {
    if (activeModeRef.current) return false;
    activeModeRef.current = m;
    setCompletedRun(null);
    return true;
  }

  function endRun(m: Mode) {
    if (activeModeRef.current === m) activeModeRef.current = null;
  }

  const [functions, setFunctions] = useState<AccuracyFunction[] | null>(null);
  const [sortBy, setSortBy] = useState<SortBy>('line');
  const [error, setError] = useState<string | null>(null);

  // Manual mode state.
  const [selectedFn, setSelectedFn] = useState('');
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<AccuracyResponse | null>(null);
  const manualAbortRef = useRef<AbortController | null>(null);

  // Batch mode state.
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [randomCount, setRandomCount] = useState(DEFAULT_RANDOM);
  const [rows, setRows] = useState<BatchRow[]>([]);
  const [batchRunning, setBatchRunning] = useState(false);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const stopRef = useRef(false);
  const batchAbortRef = useRef<AbortController | null>(null);

  // Compare mode state.
  const [compareModels, setCompareModels] = useState<Set<string>>(new Set());
  const [compareFn, setCompareFn] = useState('');
  const [compareCells, setCompareCells] = useState<CompareCell[]>([]);
  const [comparing, setComparing] = useState(false);
  const [compareLayout, setCompareLayout] = useState<CompareLayout>('columns');
  const compareStopRef = useRef(false);
  const compareAbortRef = useRef<AbortController | null>(null);

  // Ignore model changes while a run is active so an in-flight run's results
  // aren't silently cleared out from under it.
  function setSelectedModel(m: string) {
    if (activeModeRef.current) return;
    setSelectedModelState(m);
  }

  // Load models on mount.
  useEffect(() => {
    loadModels();
  }, [loadModels]);

  // Default to the first chat-capable model (same rule as the Chat app). Never
  // change the selection while a run is active.
  useEffect(() => {
    if (activeModeRef.current) return;
    if (models?.data && models.data.length > 0) {
      const chatModels = models.data.filter((m) => isChatModel(m.id));
      const valid = chatModels.some((m) => m.id === selectedModel);
      if (!valid && chatModels.length > 0) {
        setSelectedModelState(chatModels[0].id);
      }
    }
  }, [models, selectedModel]);

  // Switching models clears stale results. Model changes are blocked while a
  // run is active (see setSelectedModel), so this never interrupts a live run.
  useEffect(() => {
    setResult(null);
    setRows([]);
    setExpanded(new Set());
    setChecked(new Set());
  }, [selectedModel]);

  // Changing the compared function or model set invalidates prior results.
  useEffect(() => {
    setCompareCells([]);
  }, [compareFn, compareModels]);

  // Load the fixed function list on mount.
  useEffect(() => {
    let cancelled = false;
    api
      .listAccuracyFunctions()
      .then((resp) => {
        if (cancelled) return;
        setFunctions(resp.data);
        if (resp.data.length > 0) {
          setSelectedFn(resp.data[0].identifier);
          setCompareFn(resp.data[0].identifier);
        }
      })
      .catch((err) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : 'Failed to read functions');
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // ── Manual run ──

  async function runManual() {
    if (!selectedModel || !selectedFn) return;
    if (!beginRun('manual')) return;
    const controller = new AbortController();
    manualAbortRef.current = controller;
    setRunning(true);
    setError(null);
    setResult(null);
    try {
      const resp = await api.runAccuracy(selectedModel, selectedFn, controller.signal);
      setResult(resp);
      setCompletedRun({
        mode: 'manual',
        ok: true,
        title: 'Accuracy test done',
        summary: `${selectedFn} · ${resp.match_percent.toFixed(0)}% match`,
      });
    } catch (err) {
      // A user-initiated stop aborts the request; don't surface it as an error.
      if (controller.signal.aborted) return;
      setError(err instanceof Error ? err.message : 'Test failed');
      setCompletedRun({
        mode: 'manual',
        ok: false,
        title: 'Accuracy test failed',
        summary: selectedFn,
      });
    } finally {
      if (manualAbortRef.current === controller) manualAbortRef.current = null;
      setRunning(false);
      endRun('manual');
    }
  }

  function stopManual() {
    manualAbortRef.current?.abort();
  }

  function clearManual() {
    setResult(null);
    setError(null);
  }

  // ── Batch selection ──

  function toggle(identifier: string) {
    setChecked((prev) => {
      const next = new Set(prev);
      if (next.has(identifier)) {
        next.delete(identifier);
      } else {
        if (next.size >= MAX_RANDOM) return prev;
        next.add(identifier);
      }
      return next;
    });
  }

  function clearAll() {
    stopRef.current = true;
    setChecked(new Set());
    setRows([]);
    setExpanded(new Set());
  }

  function pickRandom() {
    const all = (functions ?? []).map((f) => f.identifier);
    const n = Math.max(1, Math.min(randomCount, MAX_RANDOM, all.length));
    // Fisher–Yates shuffle, then take the first n.
    const shuffled = [...all];
    for (let i = shuffled.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [shuffled[i], shuffled[j]] = [shuffled[j], shuffled[i]];
    }
    setChecked(new Set(shuffled.slice(0, n)));
  }

  // ── Batch run (sequential, client-side loop) ──

  async function runBatch() {
    if (!selectedModel || checked.size === 0) return;
    if (!beginRun('batch')) return;

    const targets = (functions ?? []).filter((f) => checked.has(f.identifier));

    setBatchRunning(true);
    setError(null);
    setExpanded(new Set());
    stopRef.current = false;

    const initial: BatchRow[] = targets.map((f) => ({
      identifier: f.identifier,
      line: f.line,
      loc: f.loc,
      status: 'pending',
    }));
    setRows(initial);

    let okCount = 0;
    let errCount = 0;
    let matchSum = 0;

    try {
      for (let i = 0; i < targets.length; i++) {
        if (stopRef.current) break;
        const fn = targets[i].identifier;

        setRows((prev) =>
          prev.map((r) => (r.identifier === fn ? { ...r, status: 'running' } : r)),
        );

        const controller = new AbortController();
        batchAbortRef.current = controller;

        try {
          const resp = await api.runAccuracy(selectedModel, fn, controller.signal);
          okCount++;
          matchSum += resp.match_percent;
          setRows((prev) =>
            prev.map((r) =>
              r.identifier === fn ? { ...r, status: 'done', result: resp } : r,
            ),
          );
        } catch (err) {
          // A user-initiated stop aborts the in-flight call; leave the row
          // pending and break out rather than marking it as a failure.
          if (controller.signal.aborted) {
            setRows((prev) =>
              prev.map((r) => (r.identifier === fn ? { ...r, status: 'pending' } : r)),
            );
            break;
          }
          errCount++;
          setRows((prev) =>
            prev.map((r) =>
              r.identifier === fn
                ? { ...r, status: 'error', error: err instanceof Error ? err.message : 'failed' }
                : r,
            ),
          );
        } finally {
          batchAbortRef.current = null;
        }
      }
    } finally {
      setBatchRunning(false);
      endRun('batch');
      // Only notify on a full run, not a user-initiated stop.
      if (!stopRef.current) {
        const total = targets.length;
        const avg = okCount > 0 ? Math.round(matchSum / okCount) : 0;
        setCompletedRun({
          mode: 'batch',
          ok: errCount === 0,
          title: 'Accuracy batch done',
          summary:
            `${okCount}/${total} · avg ${avg}% match` +
            (errCount > 0 ? ` · ${errCount} error${errCount > 1 ? 's' : ''}` : ''),
        });
      }
    }
  }

  function stopBatch() {
    stopRef.current = true;
    batchAbortRef.current?.abort();
  }

  function toggleExpand(identifier: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(identifier)) next.delete(identifier);
      else next.add(identifier);
      return next;
    });
  }

  // ── Compare (one function across multiple models, sequential) ──

  function toggleCompareModel(id: string) {
    setCompareModels((prev) => {
      const next = new Set(prev);
      // Clicking a selected model deselects it.
      if (next.has(id)) {
        next.delete(id);
        return next;
      }
      // Selecting a new model when full drops the earliest-selected one (FIFO),
      // so you can swap without clearing. Set preserves insertion order.
      if (next.size >= MAX_COMPARE) {
        const oldest = next.values().next().value;
        if (oldest !== undefined) next.delete(oldest);
      }
      next.add(id);
      return next;
    });
  }

  function clearCompare() {
    compareStopRef.current = true;
    compareAbortRef.current?.abort();
    setCompareModels(new Set());
    setCompareCells([]);
  }

  async function runCompare() {
    const targets = Array.from(compareModels);
    if (targets.length === 0 || !compareFn) return;
    if (!beginRun('compare')) return;

    setComparing(true);
    setError(null);
    compareStopRef.current = false;

    const initial: CompareCell[] = targets.map((m) => ({ model: m, status: 'pending' }));
    setCompareCells(initial);

    let okCount = 0;
    let errCount = 0;

    try {
      for (let i = 0; i < targets.length; i++) {
        if (compareStopRef.current) break;
        const m = targets[i];

        setCompareCells((prev) =>
          prev.map((c) => (c.model === m ? { ...c, status: 'running' } : c)),
        );

        const controller = new AbortController();
        compareAbortRef.current = controller;

        try {
          const resp = await api.runAccuracy(m, compareFn, controller.signal);
          okCount++;
          setCompareCells((prev) =>
            prev.map((c) => (c.model === m ? { ...c, status: 'done', result: resp } : c)),
          );
        } catch (err) {
          // A user-initiated stop aborts the in-flight call; leave the cell
          // pending and break out rather than marking it as a failure.
          if (controller.signal.aborted) {
            setCompareCells((prev) =>
              prev.map((c) => (c.model === m ? { ...c, status: 'pending' } : c)),
            );
            break;
          }
          errCount++;
          setCompareCells((prev) =>
            prev.map((c) =>
              c.model === m
                ? { ...c, status: 'error', error: err instanceof Error ? err.message : 'failed' }
                : c,
            ),
          );
        } finally {
          compareAbortRef.current = null;
        }
      }
    } finally {
      setComparing(false);
      endRun('compare');
      // Only notify on a full run, not a user-initiated stop.
      if (!compareStopRef.current) {
        setCompletedRun({
          mode: 'compare',
          ok: errCount === 0,
          title: 'Accuracy compare done',
          summary:
            `${okCount}/${targets.length} models` +
            (errCount > 0 ? ` · ${errCount} error${errCount > 1 ? 's' : ''}` : ''),
        });
      }
    }
  }

  function stopCompare() {
    compareStopRef.current = true;
    compareAbortRef.current?.abort();
  }

  // ── Sidebar indicator summary ──

  const isRunning = running || batchRunning || comparing;

  let activeRun: AccuracyActiveRun | null = null;
  if (running) {
    const fnLoc = functions?.find((f) => f.identifier === selectedFn)?.loc;
    const label = fnLoc != null ? `${selectedFn} (${fnLoc} loc)` : selectedFn;
    activeRun = { mode: 'manual', label, done: 0, total: 1 };
  } else if (batchRunning) {
    const done = rows.filter((r) => r.status === 'done' || r.status === 'error').length;
    activeRun = { mode: 'batch', label: `${done}/${rows.length} functions`, done, total: rows.length };
  } else if (comparing) {
    const done = compareCells.filter((c) => c.status === 'done' || c.status === 'error').length;
    activeRun = { mode: 'compare', label: `${done}/${compareCells.length} models`, done, total: compareCells.length };
  }

  function stopActive() {
    if (running) stopManual();
    else if (batchRunning) stopBatch();
    else if (comparing) stopCompare();
  }

  return (
    <AccuracyRunnerContext.Provider
      value={{
        mode,
        setMode,
        selectedModel,
        setSelectedModel,
        functions,
        sortBy,
        setSortBy,
        error,
        setError,
        selectedFn,
        setSelectedFn,
        running,
        result,
        runManual,
        stopManual,
        clearManual,
        checked,
        setChecked,
        randomCount,
        setRandomCount,
        rows,
        batchRunning,
        expanded,
        toggle,
        clearAll,
        pickRandom,
        runBatch,
        stopBatch,
        toggleExpand,
        compareModels,
        compareFn,
        setCompareFn,
        compareCells,
        comparing,
        compareLayout,
        setCompareLayout,
        toggleCompareModel,
        clearCompare,
        runCompare,
        stopCompare,
        isRunning,
        activeRun,
        completedRun,
        dismissCompleted,
        stopActive,
      }}
    >
      {children}
    </AccuracyRunnerContext.Provider>
  );
}

export function useAccuracyRunner() {
  const context = useContext(AccuracyRunnerContext);
  if (!context) {
    throw new Error('useAccuracyRunner must be used within an AccuracyRunnerProvider');
  }
  return context;
}
