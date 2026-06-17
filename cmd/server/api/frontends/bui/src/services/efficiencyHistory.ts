import type { EfficiencyResponse } from '../types';

// A saved efficiency run. The id is generated client-side so the same model and
// prompt can be saved multiple times.
export interface EfficiencyHistoryEntry {
  id: string;
  model: string;
  prompt: string;
  result: EfficiencyResponse;
  savedAt: number;
}

const HISTORY_KEY = 'efficiency:history';
const HISTORY_PING_KEY = 'efficiency:history:updatedAt';
const MAX_ENTRIES = 100;
const UPDATED_EVENT = 'efficiency-history-updated';

function genID(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

// isValidEntry guards against malformed or outdated entries in storage. The
// usage fields are read directly during render, so an entry missing them would
// crash the History tab; validating here lets us drop bad entries instead.
function isValidEntry(e: unknown): e is EfficiencyHistoryEntry {
  if (typeof e !== 'object' || e === null) return false;
  const entry = e as Record<string, unknown>;

  if (typeof entry.id !== 'string' || entry.id.length === 0) return false;
  if (typeof entry.model !== 'string') return false;
  if (typeof entry.prompt !== 'string') return false;
  if (typeof entry.savedAt !== 'number') return false;

  if (typeof entry.result !== 'object' || entry.result === null) return false;
  const usage = (entry.result as Record<string, unknown>).usage;
  if (typeof usage !== 'object' || usage === null) return false;
  const u = usage as Record<string, unknown>;

  return (
    typeof u.prompt_tokens === 'number' &&
    typeof u.completion_tokens === 'number' &&
    typeof u.in_tps === 'number' &&
    typeof u.out_tps === 'number' &&
    typeof u.ttft_ms === 'number' &&
    typeof u.wallclock_ms === 'number'
  );
}

// loadHistory returns all saved runs, newest first. Malformed/outdated entries
// are dropped so a single bad record can't crash the History tab.
export function loadHistory(): EfficiencyHistoryEntry[] {
  try {
    const raw = localStorage.getItem(HISTORY_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(isValidEntry);
  } catch {
    return [];
  }
}

function persist(entries: EfficiencyHistoryEntry[]): void {
  try {
    localStorage.setItem(HISTORY_KEY, JSON.stringify(entries.slice(0, MAX_ENTRIES)));
    // Ping so other tabs' `storage` listeners fire, then notify this tab.
    localStorage.setItem(HISTORY_PING_KEY, String(Date.now()));
  } catch {
    /* storage full or unavailable — ignore */
  }
  window.dispatchEvent(new Event(UPDATED_EVENT));
}

// saveRun snapshots a completed run into history and returns the new entry.
export function saveRun(model: string, prompt: string, result: EfficiencyResponse): EfficiencyHistoryEntry {
  const entry: EfficiencyHistoryEntry = {
    id: genID(),
    model,
    prompt,
    result,
    savedAt: Date.now(),
  };
  const entries = [entry, ...loadHistory()];
  persist(entries);
  return entry;
}

// deleteEntries removes the runs with the given ids.
export function deleteEntries(ids: Set<string>): void {
  persist(loadHistory().filter((e) => !ids.has(e.id)));
}

// subscribe registers a callback fired whenever history changes (this tab or
// another). Returns an unsubscribe function.
export function subscribe(cb: () => void): () => void {
  const onStorage = (e: StorageEvent) => {
    if (e.key === HISTORY_PING_KEY || e.key === HISTORY_KEY) cb();
  };
  window.addEventListener(UPDATED_EVENT, cb);
  window.addEventListener('storage', onStorage);
  return () => {
    window.removeEventListener(UPDATED_EVENT, cb);
    window.removeEventListener('storage', onStorage);
  };
}
