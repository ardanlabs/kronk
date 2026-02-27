import type { AutoTestRun } from '../contexts/AutoTestRunnerContext'
import type { AutoTestSweepMode } from '../types'

export interface AutoTestHistoryEntry {
  version: 1
  id: string
  savedAt: string
  completedAt?: string
  modelId?: string
  sweepMode: AutoTestSweepMode
  run: AutoTestRun
}

const HISTORY_KEY = 'playground:autoTestHistory:v1'
const HISTORY_PING_KEY = 'playground:autoTestHistory:updatedAt'
const MAX_ENTRIES = 500

export { HISTORY_KEY, MAX_ENTRIES }

// ---------------------------------------------------------------------------
// IndexedDB setup
// ---------------------------------------------------------------------------

const DB_NAME = 'playground:autoTestHistory'
const DB_VERSION = 1
const STORE_ENTRIES = 'entries'
const STORE_META = 'meta'

function promisifyRequest<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}

function awaitTx(tx: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
    tx.onabort = () => reject(tx.error)
  })
}

let dbPromise: Promise<IDBDatabase> | null = null

function openDB(): Promise<IDBDatabase> {
  if (dbPromise) return dbPromise
  dbPromise = new Promise<IDBDatabase>((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onupgradeneeded = () => {
      const db = req.result
      if (!db.objectStoreNames.contains(STORE_ENTRIES)) {
        const store = db.createObjectStore(STORE_ENTRIES, { keyPath: 'id' })
        store.createIndex('by_savedAt', 'savedAt')
      }
      if (!db.objectStoreNames.contains(STORE_META)) {
        db.createObjectStore(STORE_META, { keyPath: 'key' })
      }
    }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => {
      dbPromise = null
      reject(req.error)
    }
  })
  return dbPromise
}

// ---------------------------------------------------------------------------
// localStorage migration (runs once)
// ---------------------------------------------------------------------------

let migrationDone = false

async function migrateFromLocalStorage(db: IDBDatabase): Promise<void> {
  if (migrationDone) return

  const metaTx = db.transaction(STORE_META, 'readonly')
  const flag = await promisifyRequest(metaTx.objectStore(STORE_META).get('migratedFromLocalStorage'))
  if (flag) {
    migrationDone = true
    return
  }

  // Check if entries store already has data (avoid clobbering).
  const countTx = db.transaction(STORE_ENTRIES, 'readonly')
  const count = await promisifyRequest(countTx.objectStore(STORE_ENTRIES).count())
  if (count > 0) {
    const markTx = db.transaction(STORE_META, 'readwrite')
    markTx.objectStore(STORE_META).put({ key: 'migratedFromLocalStorage', value: true })
    await awaitTx(markTx)
    migrationDone = true
    return
  }

  try {
    const raw = localStorage.getItem(HISTORY_KEY)
    if (raw) {
      const parsed = JSON.parse(raw)
      if (Array.isArray(parsed)) {
        const valid = parsed.filter(isValidEntry) as AutoTestHistoryEntry[]
        if (valid.length > 0) {
          const tx = db.transaction([STORE_ENTRIES, STORE_META], 'readwrite')
          const store = tx.objectStore(STORE_ENTRIES)
          for (const entry of valid) {
            store.put(entry)
          }
          tx.objectStore(STORE_META).put({ key: 'migratedFromLocalStorage', value: true })
          await awaitTx(tx)
          localStorage.removeItem(HISTORY_KEY)
          migrationDone = true
          return
        }
      }
    }
  } catch {
    // localStorage read failed; continue without migration.
  }

  const markTx2 = db.transaction(STORE_META, 'readwrite')
  markTx2.objectStore(STORE_META).put({ key: 'migratedFromLocalStorage', value: true })
  await awaitTx(markTx2)
  migrationDone = true
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function isValidEntry(e: unknown): e is AutoTestHistoryEntry {
  if (typeof e !== 'object' || e === null) return false
  const entry = e as Record<string, unknown>
  if (entry.version !== 1) return false
  if (typeof entry.id !== 'string' || entry.id.length === 0) return false
  if (typeof entry.savedAt !== 'string' || !Number.isFinite(Date.parse(entry.savedAt))) return false
  if (entry.sweepMode !== 'sampling' && entry.sweepMode !== 'config') return false
  if (typeof entry.run !== 'object' || entry.run === null) return false
  const run = entry.run as Record<string, unknown>
  if (!Array.isArray(run.trials)) return false
  return true
}

function pruneRunForHistory(run: AutoTestRun): AutoTestRun {
  const clone = structuredClone(run)
  for (const trial of clone.trials) {
    trial.activePrompts = undefined
    trial.logEntries = undefined
  }
  return clone
}

function deriveCompletedAt(run: AutoTestRun): string | undefined {
  let max: string | undefined
  for (const trial of run.trials) {
    if (trial.finishedAt && (!max || trial.finishedAt > max)) {
      max = trial.finishedAt
    }
  }
  return max ?? run.runStartedAt
}

// ---------------------------------------------------------------------------
// Pruning (enforce MAX_ENTRIES)
// ---------------------------------------------------------------------------

async function pruneToMax(db: IDBDatabase): Promise<void> {
  const tx = db.transaction(STORE_ENTRIES, 'readwrite')
  const idx = tx.objectStore(STORE_ENTRIES).index('by_savedAt')
  // savedAt ascending — oldest entries first.
  const allKeys = await promisifyRequest(idx.getAllKeys())
  if (allKeys.length <= MAX_ENTRIES) return
  // Delete oldest entries (those at the beginning of the ascending list).
  const toDelete = allKeys.slice(0, allKeys.length - MAX_ENTRIES)
  const store = tx.objectStore(STORE_ENTRIES)
  for (const key of toDelete) {
    store.delete(key)
  }
  await awaitTx(tx)
}

// ---------------------------------------------------------------------------
// Event dispatch
// ---------------------------------------------------------------------------

function dispatchUpdate() {
  window.dispatchEvent(new Event('autotest-history-updated'))
  // Ping localStorage so cross-tab `storage` listeners fire.
  try {
    localStorage.setItem(HISTORY_PING_KEY, String(Date.now()))
  } catch {
    // Ignore quota errors on the ping key.
  }
}

// ---------------------------------------------------------------------------
// Async DB accessor (with migration)
// ---------------------------------------------------------------------------

async function getDB(): Promise<IDBDatabase> {
  const db = await openDB()
  await migrateFromLocalStorage(db)
  return db
}

// ---------------------------------------------------------------------------
// localStorage fallback helpers
// ---------------------------------------------------------------------------

function loadFromLocalStorage(): AutoTestHistoryEntry[] {
  try {
    const raw = localStorage.getItem(HISTORY_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter(isValidEntry) as AutoTestHistoryEntry[]
  } catch {
    return []
  }
}

function saveToLocalStorage(entries: AutoTestHistoryEntry[]): void {
  try {
    localStorage.setItem(HISTORY_KEY, JSON.stringify(entries.slice(0, MAX_ENTRIES)))
  } catch { /* ignore */ }
}

// ---------------------------------------------------------------------------
// Persistence (async IndexedDB with localStorage fallback)
// ---------------------------------------------------------------------------

export async function loadAutoTestHistory(): Promise<AutoTestHistoryEntry[]> {
  try {
    const db = await getDB()
    const tx = db.transaction(STORE_ENTRIES, 'readonly')
    const idx = tx.objectStore(STORE_ENTRIES).index('by_savedAt')
    const all = await promisifyRequest(idx.getAll())
    // Index returns ascending by savedAt; reverse to get newest first.
    return all.filter(isValidEntry).reverse()
  } catch {
    return loadFromLocalStorage()
  }
}

export async function saveCompletedRun(run: AutoTestRun): Promise<void> {
  const pruned = pruneRunForHistory(run)
  const modelId = run.kind === 'sampling'
    ? run.sessionSeed?.model_id
    : run.sessionSeed.model_id

  const entry: AutoTestHistoryEntry = {
    version: 1,
    id: run.runId,
    savedAt: new Date().toISOString(),
    completedAt: deriveCompletedAt(run),
    modelId,
    sweepMode: run.kind,
    run: pruned,
  }

  try {
    const db = await getDB()
    const tx = db.transaction(STORE_ENTRIES, 'readwrite')
    tx.objectStore(STORE_ENTRIES).put(entry)
    await awaitTx(tx)
    await pruneToMax(db)
  } catch {
    const history = loadFromLocalStorage().filter((e) => e.id !== run.runId)
    history.unshift(entry)
    saveToLocalStorage(history)
  }
  dispatchUpdate()
}

export async function updateHistoryEntry(
  id: string,
  updater: (entry: AutoTestHistoryEntry) => AutoTestHistoryEntry,
): Promise<void> {
  try {
    const db = await getDB()
    const readTx = db.transaction(STORE_ENTRIES, 'readonly')
    const existing = await promisifyRequest(readTx.objectStore(STORE_ENTRIES).get(id))
    if (!existing) return

    const updated = updater(existing)
    const writeTx = db.transaction(STORE_ENTRIES, 'readwrite')
    writeTx.objectStore(STORE_ENTRIES).put(updated)
    await awaitTx(writeTx)
  } catch {
    const history = loadFromLocalStorage()
    const idx = history.findIndex(e => e.id === id)
    if (idx >= 0) {
      history[idx] = updater(history[idx])
      saveToLocalStorage(history)
    }
  }
  dispatchUpdate()
}

export async function deleteHistoryEntry(id: string): Promise<void> {
  try {
    const db = await getDB()
    const tx = db.transaction(STORE_ENTRIES, 'readwrite')
    tx.objectStore(STORE_ENTRIES).delete(id)
    await awaitTx(tx)
  } catch {
    const history = loadFromLocalStorage().filter(e => e.id !== id)
    saveToLocalStorage(history)
  }
  dispatchUpdate()
}

export async function deleteHistoryEntries(ids: string[]): Promise<void> {
  if (ids.length === 0) return
  try {
    const db = await getDB()
    const tx = db.transaction(STORE_ENTRIES, 'readwrite')
    const store = tx.objectStore(STORE_ENTRIES)
    for (const id of ids) {
      store.delete(id)
    }
    await awaitTx(tx)
  } catch {
    const idSet = new Set(ids)
    const history = loadFromLocalStorage().filter(e => !idSet.has(e.id))
    saveToLocalStorage(history)
  }
  dispatchUpdate()
}

export async function clearAutoTestHistory(): Promise<void> {
  try {
    const db = await getDB()
    const tx = db.transaction(STORE_ENTRIES, 'readwrite')
    tx.objectStore(STORE_ENTRIES).clear()
    await awaitTx(tx)
  } catch {
    try { localStorage.removeItem(HISTORY_KEY) } catch { /* ignore */ }
  }
  dispatchUpdate()
}

// ---------------------------------------------------------------------------
// Export / Import
// ---------------------------------------------------------------------------

export interface AutoTestHistoryExport {
  schemaVersion: 1
  exportedAt: string
  entries: AutoTestHistoryEntry[]
}

export async function exportAutoTestHistoryJSON(): Promise<Blob> {
  const entries = await loadAutoTestHistory()
  const envelope: AutoTestHistoryExport = {
    schemaVersion: 1,
    exportedAt: new Date().toISOString(),
    entries,
  }
  return new Blob([JSON.stringify(envelope, null, 2)], { type: 'application/json' })
}

export async function importAutoTestHistoryJSON(
  text: string,
): Promise<{ imported: number; skipped: number }> {
  let envelope: unknown
  try {
    envelope = JSON.parse(text)
  } catch {
    throw new Error('Invalid JSON file')
  }

  if (
    typeof envelope !== 'object' || envelope === null ||
    !('schemaVersion' in envelope) ||
    (envelope as AutoTestHistoryExport).schemaVersion !== 1 ||
    !('entries' in envelope) ||
    !Array.isArray((envelope as AutoTestHistoryExport).entries)
  ) {
    throw new Error('Invalid export file format')
  }

  const candidates = (envelope as AutoTestHistoryExport).entries

  // Validate and deduplicate within the file (keep newest savedAt per id).
  const deduped = new Map<string, AutoTestHistoryEntry>()
  for (const c of candidates) {
    if (!isValidEntry(c)) continue
    // Prune ephemeral fields from imported runs.
    const pruned: AutoTestHistoryEntry = { ...c, run: pruneRunForHistory(c.run) }
    const existing = deduped.get(pruned.id)
    if (!existing || pruned.savedAt > existing.savedAt) {
      deduped.set(pruned.id, pruned)
    }
  }

  const valid = Array.from(deduped.values())
  if (valid.length === 0) {
    return { imported: 0, skipped: candidates.length }
  }

  let imported = 0
  let skipped = 0

  try {
    const db = await getDB()

    const readTx = db.transaction(STORE_ENTRIES, 'readonly')
    const existingKeys = new Set(
      await promisifyRequest(readTx.objectStore(STORE_ENTRIES).getAllKeys()) as string[],
    )

    const writeTx = db.transaction(STORE_ENTRIES, 'readwrite')
    const store = writeTx.objectStore(STORE_ENTRIES)
    for (const entry of valid) {
      if (existingKeys.has(entry.id)) {
        const existingEntry = await promisifyRequest(store.get(entry.id)) as AutoTestHistoryEntry
        if (existingEntry && entry.savedAt > existingEntry.savedAt) {
          store.put(entry)
          imported++
        } else {
          skipped++
        }
      } else {
        store.put(entry)
        imported++
      }
    }
    await awaitTx(writeTx)
    await pruneToMax(db)
  } catch {
    // Fallback: merge into localStorage.
    const history = loadFromLocalStorage()
    const existingIds = new Map(history.map(e => [e.id, e]))
    for (const entry of valid) {
      const ex = existingIds.get(entry.id)
      if (ex) {
        if (entry.savedAt > ex.savedAt) {
          existingIds.set(entry.id, entry)
          imported++
        } else {
          skipped++
        }
      } else {
        existingIds.set(entry.id, entry)
        imported++
      }
    }
    saveToLocalStorage(Array.from(existingIds.values()))
  }

  dispatchUpdate()
  return { imported, skipped }
}
