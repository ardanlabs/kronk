import type { DisplayMessage } from '../contexts/ChatContext'
import type { SavedChat } from '../contexts/ChatHistoryContext'

const SESSION_LS_KEY = 'kronk_chat_messages'
const HISTORY_LS_KEY = 'kronk_chat_history'

// ---------------------------------------------------------------------------
// IndexedDB setup
// ---------------------------------------------------------------------------

const DB_NAME = 'kronk:chatHistory'
const DB_VERSION = 1
const STORE_SESSION = 'session'
const STORE_HISTORY = 'history'
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
      if (!db.objectStoreNames.contains(STORE_SESSION)) {
        db.createObjectStore(STORE_SESSION, { keyPath: 'key' })
      }
      if (!db.objectStoreNames.contains(STORE_HISTORY)) {
        const store = db.createObjectStore(STORE_HISTORY, { keyPath: 'id' })
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
let migrationPromise: Promise<void> | null = null

async function migrateFromLocalStorage(db: IDBDatabase): Promise<void> {
  if (migrationDone) return
  if (migrationPromise) return migrationPromise

  migrationPromise = doMigration(db)
  return migrationPromise
}

async function doMigration(db: IDBDatabase): Promise<void> {
  const metaTx = db.transaction(STORE_META, 'readonly')
  const flag = await promisifyRequest(metaTx.objectStore(STORE_META).get('migratedFromLocalStorage'))
  if (flag) {
    migrationDone = true
    return
  }

  // Check if stores already have data (avoid clobbering).
  const countTx = db.transaction([STORE_SESSION, STORE_HISTORY], 'readonly')
  const sessionCount = await promisifyRequest(countTx.objectStore(STORE_SESSION).count())
  const historyCount = await promisifyRequest(countTx.objectStore(STORE_HISTORY).count())
  if (sessionCount > 0 || historyCount > 0) {
    const markTx = db.transaction(STORE_META, 'readwrite')
    markTx.objectStore(STORE_META).put({ key: 'migratedFromLocalStorage', value: true })
    await awaitTx(markTx)
    migrationDone = true
    return
  }

  try {
    const tx = db.transaction([STORE_SESSION, STORE_HISTORY, STORE_META], 'readwrite')
    let migrated = false

    // Migrate session messages.
    const rawMessages = localStorage.getItem(SESSION_LS_KEY)
    if (rawMessages) {
      const parsed = JSON.parse(rawMessages)
      if (Array.isArray(parsed) && parsed.length > 0) {
        tx.objectStore(STORE_SESSION).put({ key: 'messages', value: parsed })
        migrated = true
      }
    }

    // Migrate history entries.
    const rawHistory = localStorage.getItem(HISTORY_LS_KEY)
    if (rawHistory) {
      const parsed = JSON.parse(rawHistory)
      if (Array.isArray(parsed)) {
        const store = tx.objectStore(STORE_HISTORY)
        for (const chat of parsed as SavedChat[]) {
          store.put(chat)
        }
        if (parsed.length > 0) migrated = true
      }
    }

    tx.objectStore(STORE_META).put({ key: 'migratedFromLocalStorage', value: true })
    await awaitTx(tx)

    if (migrated) {
      localStorage.removeItem(SESSION_LS_KEY)
      localStorage.removeItem(HISTORY_LS_KEY)
    }

    migrationDone = true
    return
  } catch {
    // localStorage read failed; continue without migration.
  }

  const markTx2 = db.transaction(STORE_META, 'readwrite')
  markTx2.objectStore(STORE_META).put({ key: 'migratedFromLocalStorage', value: true })
  await awaitTx(markTx2)
  migrationDone = true
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

function loadSessionFromLocalStorage(): DisplayMessage[] {
  try {
    const raw = localStorage.getItem(SESSION_LS_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed as DisplayMessage[]
  } catch {
    return []
  }
}

function saveSessionToLocalStorage(messages: DisplayMessage[]): void {
  try {
    if (messages.length > 0) {
      localStorage.setItem(SESSION_LS_KEY, JSON.stringify(messages))
    } else {
      localStorage.removeItem(SESSION_LS_KEY)
    }
  } catch { /* ignore */ }
}

function loadHistoryFromLocalStorage(): SavedChat[] {
  try {
    const raw = localStorage.getItem(HISTORY_LS_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed as SavedChat[]
  } catch {
    return []
  }
}

function saveHistoryToLocalStorage(history: SavedChat[]): void {
  try {
    if (history.length > 0) {
      localStorage.setItem(HISTORY_LS_KEY, JSON.stringify(history))
    } else {
      localStorage.removeItem(HISTORY_LS_KEY)
    }
  } catch { /* ignore */ }
}

// ---------------------------------------------------------------------------
// Persistence (async IndexedDB with localStorage fallback)
// ---------------------------------------------------------------------------

export async function getSessionMessages(): Promise<DisplayMessage[]> {
  try {
    const db = await getDB()
    const tx = db.transaction(STORE_SESSION, 'readonly')
    const row = await promisifyRequest(tx.objectStore(STORE_SESSION).get('messages')) as { key: string, value: DisplayMessage[] } | undefined
    return row && Array.isArray(row.value) ? row.value : []
  } catch {
    return loadSessionFromLocalStorage()
  }
}

export async function setSessionMessages(messages: DisplayMessage[]): Promise<void> {
  try {
    const db = await getDB()
    const tx = db.transaction(STORE_SESSION, 'readwrite')
    if (messages.length === 0) {
      tx.objectStore(STORE_SESSION).delete('messages')
    } else {
      tx.objectStore(STORE_SESSION).put({ key: 'messages', value: messages })
    }
    await awaitTx(tx)
  } catch {
    saveSessionToLocalStorage(messages)
  }
}

export async function clearSessionMessages(): Promise<void> {
  await setSessionMessages([])
}

export async function getAllHistory(): Promise<SavedChat[]> {
  try {
    const db = await getDB()
    const tx = db.transaction(STORE_HISTORY, 'readonly')
    const idx = tx.objectStore(STORE_HISTORY).index('by_savedAt')
    const all = await promisifyRequest(idx.getAll())
    // Index returns ascending by savedAt; reverse to get newest first.
    return (all as SavedChat[]).reverse()
  } catch {
    return loadHistoryFromLocalStorage()
  }
}

export async function putHistoryChat(chat: SavedChat): Promise<void> {
  try {
    const db = await getDB()
    const tx = db.transaction(STORE_HISTORY, 'readwrite')
    tx.objectStore(STORE_HISTORY).put(chat)
    await awaitTx(tx)
  } catch {
    const history = loadHistoryFromLocalStorage().filter(e => e.id !== chat.id)
    history.unshift(chat)
    saveHistoryToLocalStorage(history)
  }
}

export async function deleteHistoryChats(ids: string[]): Promise<void> {
  if (ids.length === 0) return
  try {
    const db = await getDB()
    const tx = db.transaction(STORE_HISTORY, 'readwrite')
    const store = tx.objectStore(STORE_HISTORY)
    for (const id of ids) {
      store.delete(id)
    }
    await awaitTx(tx)
  } catch {
    const idSet = new Set(ids)
    const history = loadHistoryFromLocalStorage().filter(e => !idSet.has(e.id))
    saveHistoryToLocalStorage(history)
  }
}

export async function clearAllHistory(): Promise<void> {
  try {
    const db = await getDB()
    const tx = db.transaction(STORE_HISTORY, 'readwrite')
    tx.objectStore(STORE_HISTORY).clear()
    await awaitTx(tx)
  } catch {
    try { localStorage.removeItem(HISTORY_LS_KEY) } catch { /* ignore */ }
  }
}
