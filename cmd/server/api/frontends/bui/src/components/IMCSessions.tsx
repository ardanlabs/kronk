import { useEffect, useState } from 'react';
import { api } from '../services/api';
import type { IMCSessionDetail, IMCSessionsResponse, IMCSystemCacheDetail, IMCSystemCachesResponse } from '../types';
import { formatBytes, formatModelID } from '../lib/format';
import { labelWithTip, PARAM_TOOLTIPS, type TooltipKey } from './ParamTooltips';

type View = 'current' | 'system';
type CurrentSort = 'model_id' | 'id' | 'state' | 'messages' | 'context' | 'allocated' | 'snapshot_bytes' | 'input_tokens' | 'output_tokens' | 'request_context' | 'peak_context' | 'utilization' | 'last_used' | 'has_media';
type SystemSort = 'model_id' | 'id' | 'tokens' | 'allocated' | 'snapshot_bytes' | 'restore_count' | 'active_restores' | 'last_used';

const ALL_MODELS = '';
const STATE_ORDER = { active: 0, idle: 1, empty: 2 } as const;
const CURRENT_GUIDE: ReadonlyArray<[string, TooltipKey]> = [
  ['Model', 'imcModelID'], ['Cache Entry', 'imcSessionID'], ['State', 'imcState'],
  ['Current Messages', 'imcMessages'], ['Current Tokens', 'imcContext'], ['Current Allocated', 'imcAllocated'],
  ['Latest Request Input', 'imcInputTokens'], ['Latest Request Output', 'imcOutputTokens'],
  ['Latest Request Context', 'imcRequestTotal'], ['Peak Context', 'imcPeakContext'], ['Peak Memory', 'imcSnapshotBytes'],
  ['Peak Used', 'imcUtilization'], ['Media', 'imcMedia'], ['Last Used', 'imcLastUsed'],
];
const SYSTEM_GUIDE: ReadonlyArray<[string, TooltipKey]> = [
  ['Model', 'imcModelID'], ['Cache Entry', 'imcSystemCacheID'], ['System Tokens', 'imcSystemContext'],
  ['Allocated', 'imcSystemAllocated'], ['Snapshot Memory', 'imcSystemSnapshotBytes'], ['Restores', 'imcSystemRestores'],
  ['Active Restores', 'imcSystemActiveRestores'], ['Last Used', 'imcLastUsed'],
];
const VIEW_STORAGE_KEY = 'kronk-imc-view';
const MODEL_STORAGE_KEY = 'kronk-imc-model';

function storedView(): View {
  try {
    return sessionStorage.getItem(VIEW_STORAGE_KEY) === 'system' ? 'system' : 'current';
  } catch {
    return 'current';
  }
}

function storedModel(): string {
  try {
    return sessionStorage.getItem(MODEL_STORAGE_KEY) ?? ALL_MODELS;
  } catch {
    return ALL_MODELS;
  }
}

function formatDate(value: string): string {
  if (!value || value.startsWith('0001-01-01')) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString();
}

function utilization(peak: number, window: number): string {
  return window > 0 ? `${((peak / window) * 100).toFixed(1)}%` : '0%';
}

export default function IMCSessions() {
  const [sessions, setSessions] = useState<IMCSessionsResponse | null>(null);
  const [systemCaches, setSystemCaches] = useState<IMCSystemCachesResponse | null>(null);
  const [view, setView] = useState<View>(storedView);
  const [selectedModel, setSelectedModel] = useState(storedModel);
  const [currentSort, setCurrentSort] = useState<CurrentSort>('last_used');
  const [currentAscending, setCurrentAscending] = useState(false);
  const [systemSort, setSystemSort] = useState<SystemSort>('last_used');
  const [systemAscending, setSystemAscending] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showFieldGuide, setShowFieldGuide] = useState(false);

  const load = async (silent = false) => {
    if (!silent) setLoading(true);
    setError(null);
    try {
      const [nextSessions, nextSystemCaches] = await Promise.all([
        api.listIMCSessions(), api.listIMCSystemCaches(),
      ]);
      setSessions(nextSessions);
      setSystemCaches(nextSystemCaches);
      const models = new Set([...nextSessions, ...nextSystemCaches].map((entry) => entry.model_id));
      setSelectedModel((current) => current === ALL_MODELS || models.has(current) ? current : ALL_MODELS);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load IMC caches');
    } finally {
      if (!silent) setLoading(false);
    }
  };

  useEffect(() => {
    load();
    const id = window.setInterval(() => load(true), 5000);
    return () => window.clearInterval(id);
  }, []);

  useEffect(() => {
    try { sessionStorage.setItem(VIEW_STORAGE_KEY, view); } catch { /* Storage may be unavailable. */ }
  }, [view]);

  useEffect(() => {
    try { sessionStorage.setItem(MODEL_STORAGE_KEY, selectedModel); } catch { /* Storage may be unavailable. */ }
  }, [selectedModel]);

  useEffect(() => {
    if (!showFieldGuide) return;
    const handleKey = (event: KeyboardEvent) => event.key === 'Escape' && setShowFieldGuide(false);
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [showFieldGuide]);

  const sortCurrent = (field: CurrentSort) => {
    if (field === currentSort) setCurrentAscending((ascending) => !ascending);
    else { setCurrentSort(field); setCurrentAscending(true); }
  };
  const sortSystem = (field: SystemSort) => {
    if (field === systemSort) setSystemAscending((ascending) => !ascending);
    else { setSystemSort(field); setSystemAscending(true); }
  };
  const currentIndicator = (field: CurrentSort) => currentSort === field ? (currentAscending ? ' ▲' : ' ▼') : '';
  const systemIndicator = (field: SystemSort) => systemSort === field ? (systemAscending ? ' ▲' : ' ▼') : '';

  const modelIDs = [...new Set([...(sessions ?? []), ...(systemCaches ?? [])].map((entry) => entry.model_id))]
    .sort((a, b) => a.localeCompare(b));
  const filteredSessions = (sessions ?? []).filter((entry) => selectedModel === ALL_MODELS || entry.model_id === selectedModel);
  const filteredSystemCaches = (systemCaches ?? []).filter((entry) => selectedModel === ALL_MODELS || entry.model_id === selectedModel);

  const sortedSessions = filteredSessions.sort((a, b) => {
    let comparison: number;
    switch (currentSort) {
      case 'model_id': comparison = a.model_id.localeCompare(b.model_id); break;
      case 'id': comparison = a.id - b.id; break;
      case 'state': comparison = STATE_ORDER[a.state] - STATE_ORDER[b.state]; break;
      case 'messages': comparison = a.messages - b.messages; break;
      case 'context': comparison = a.context - b.context; break;
      case 'allocated': comparison = a.allocated - b.allocated; break;
      case 'snapshot_bytes': comparison = a.snapshot_bytes - b.snapshot_bytes; break;
      case 'input_tokens': comparison = a.input_tokens - b.input_tokens; break;
      case 'output_tokens': comparison = a.output_tokens - b.output_tokens; break;
      case 'request_context': comparison = a.input_tokens + a.output_tokens - b.input_tokens - b.output_tokens; break;
      case 'peak_context': comparison = a.peak_context - b.peak_context; break;
      case 'utilization': comparison = ratio(a) - ratio(b); break;
      case 'has_media': comparison = Number(a.has_media) - Number(b.has_media); break;
      case 'last_used': comparison = Date.parse(a.last_used) - Date.parse(b.last_used); break;
    }
    comparison ||= a.model_id.localeCompare(b.model_id) || a.id - b.id;
    return currentAscending ? comparison : -comparison;
  });

  const sortedSystemCaches = filteredSystemCaches.sort((a, b) => {
    let comparison: number;
    switch (systemSort) {
      case 'model_id': comparison = a.model_id.localeCompare(b.model_id); break;
      case 'id': comparison = a.id - b.id; break;
      case 'tokens': comparison = a.tokens - b.tokens; break;
      case 'allocated': comparison = a.allocated - b.allocated; break;
      case 'snapshot_bytes': comparison = a.snapshot_bytes - b.snapshot_bytes; break;
      case 'restore_count': comparison = a.restore_count - b.restore_count; break;
      case 'active_restores': comparison = a.active_restores - b.active_restores; break;
      case 'last_used': comparison = Date.parse(a.last_used) - Date.parse(b.last_used); break;
    }
    comparison ||= a.model_id.localeCompare(b.model_id) || a.id - b.id;
    return systemAscending ? comparison : -comparison;
  });

  const guide = view === 'current' ? CURRENT_GUIDE : SYSTEM_GUIDE;

  return (
    <div>
      <div className="page-header-with-action">
        <div><h2>IMC Sessions</h2><p className="page-description">Working sessions and reusable System prompt caches for loaded models</p></div>
        <div style={{ display: 'flex', gap: '8px' }}>
          <button className="btn btn-secondary" onClick={() => setShowFieldGuide(true)}>Field Guide</button>
          <button className="btn btn-primary" onClick={() => load()} disabled={loading}>Refresh</button>
        </div>
      </div>

      <div className="tabs" role="tablist" aria-label="IMC cache type">
        <button type="button" role="tab" aria-selected={view === 'current'} className={`tab ${view === 'current' ? 'active' : ''}`} onClick={() => setView('current')}>Working Sessions</button>
        <button type="button" role="tab" aria-selected={view === 'system'} className={`tab ${view === 'system' ? 'active' : ''}`} onClick={() => setView('system')}>System Caches</button>
      </div>

      {showFieldGuide && (
        <div className="modal-overlay" role="dialog" aria-modal="true" aria-labelledby="imc-field-guide-title" onClick={() => setShowFieldGuide(false)}>
          <div className="modal-content" onClick={(event) => event.stopPropagation()}>
            <div className="modal-header"><h3 id="imc-field-guide-title">IMC {view === 'current' ? 'Working Session' : 'System Cache'} Field Guide</h3><button className="modal-close" onClick={() => setShowFieldGuide(false)} aria-label="Close field guide">×</button></div>
            <div className="modal-body"><dl style={{ margin: 0, display: 'grid', gap: '20px' }}>{guide.map(([label, key]) => <div key={key}><dt><strong>{label}</strong></dt><dd style={{ margin: '4px 0 0' }}>{PARAM_TOOLTIPS[key]}</dd></div>)}</dl></div>
          </div>
        </div>
      )}

      {error && <div className="alert alert-error">{error}</div>}
      {modelIDs.length > 0 && (
        <div className="tabs imc-model-tabs" role="tablist" aria-label="IMC caches by model">
          <button type="button" role="tab" aria-selected={selectedModel === ALL_MODELS} className={`tab ${selectedModel === ALL_MODELS ? 'active' : ''}`} onClick={() => setSelectedModel(ALL_MODELS)}>All</button>
          {modelIDs.map((modelID) => <button key={modelID} type="button" role="tab" aria-selected={selectedModel === modelID} className={`tab imc-model-tab ${selectedModel === modelID ? 'active' : ''}`} title={modelID} onClick={() => setSelectedModel(modelID)}><span className="imc-model-tab-label">{formatModelID(modelID)}</span></button>)}
        </div>
      )}

      <div className="card">
        {loading && !sessions ? <div className="loading">Loading IMC caches...</div>
          : view === 'current' ? <CurrentTable data={sortedSessions} sort={sortCurrent} indicator={currentIndicator} />
            : <SystemTable data={sortedSystemCaches} sort={sortSystem} indicator={systemIndicator} />}
      </div>
    </div>
  );
}

function ratio(session: IMCSessionDetail): number {
  return session.context_window > 0 ? session.peak_context / session.context_window : 0;
}

function CurrentTable({ data, sort, indicator }: { data: IMCSessionDetail[]; sort: (field: CurrentSort) => void; indicator: (field: CurrentSort) => string }) {
  if (data.length === 0) return <div className="empty-state"><h3>No IMC working sessions</h3><p>Entries appear when an IMC-enabled model is loaded</p></div>;
  const head = (label: string, tip: TooltipKey, field: CurrentSort, right = false) => <th onClick={() => sort(field)} className="catalog-table-sortable" style={right ? { textAlign: 'right' } : undefined}>{labelWithTip(label, tip)}<span className="catalog-table-sort-indicator">{indicator(field)}</span></th>;
  return <div style={{ overflowX: 'auto' }}><table className="imc-sessions-table"><thead>
    <tr className="imc-table-groups"><th colSpan={3} className="imc-table-group imc-table-group-session">Session</th><th colSpan={3} className="imc-table-group imc-table-group-request">Latest Request</th><th colSpan={3} className="imc-table-group imc-table-group-current">Working Cache</th><th colSpan={3} className="imc-table-group imc-table-group-capacity">Peak / Usage</th><th colSpan={2} className="imc-table-group imc-table-group-details">Details</th></tr>
    <tr className="imc-table-columns">{head('Model', 'imcModelID', 'model_id')}{head('Cache Entry', 'imcSessionID', 'id')}{head('State', 'imcState', 'state')}{head('Input', 'imcInputTokens', 'input_tokens', true)}{head('Output', 'imcOutputTokens', 'output_tokens', true)}{head('Context', 'imcRequestTotal', 'request_context', true)}{head('Messages', 'imcMessages', 'messages', true)}{head('Tokens', 'imcContext', 'context', true)}{head('Allocated', 'imcAllocated', 'allocated', true)}{head('Peak Context', 'imcPeakContext', 'peak_context', true)}{head('Peak Memory', 'imcSnapshotBytes', 'snapshot_bytes', true)}{head('Peak Used', 'imcUtilization', 'utilization', true)}{head('Media', 'imcMedia', 'has_media')}{head('Last Used', 'imcLastUsed', 'last_used')}</tr>
  </thead><tbody>{data.map((s) => <tr key={`${s.model_id}-${s.id}`}><td>{s.model_id}</td><td>{s.id}</td><td><span className={`badge badge-${s.state}`}>{s.state}</span></td><td className="imc-request-cell" style={{ textAlign: 'right' }}>{s.input_tokens.toLocaleString()}</td><td className="imc-request-cell" style={{ textAlign: 'right' }}>{s.output_tokens.toLocaleString()}</td><td className="imc-request-cell" style={{ textAlign: 'right' }}>{(s.input_tokens + s.output_tokens).toLocaleString()}</td><td className="imc-current-cell" style={{ textAlign: 'right' }}>{s.messages.toLocaleString()}</td><td className="imc-current-cell" style={{ textAlign: 'right' }}>{s.context.toLocaleString()}</td><td className="imc-current-cell" style={{ textAlign: 'right' }}>{s.allocated.toLocaleString()}</td><td className="imc-capacity-cell" style={{ textAlign: 'right' }}>{s.peak_context.toLocaleString()}</td><td className="imc-capacity-cell" style={{ textAlign: 'right' }}>{formatBytes(s.snapshot_bytes)}</td><td className="imc-capacity-cell" style={{ textAlign: 'right' }}>{utilization(s.peak_context, s.context_window)}</td><td><span className={`badge badge-${s.has_media ? 'yes' : 'no'}`}>{s.has_media ? 'yes' : 'no'}</span></td><td style={{ whiteSpace: 'nowrap' }}>{formatDate(s.last_used)}</td></tr>)}</tbody></table></div>;
}

function SystemTable({ data, sort, indicator }: { data: IMCSystemCacheDetail[]; sort: (field: SystemSort) => void; indicator: (field: SystemSort) => string }) {
  if (data.length === 0) return <div className="empty-state"><h3>No System caches</h3><p>Entries appear after requests with System prompts</p></div>;
  const head = (label: string, tip: TooltipKey, field: SystemSort, right = false) => <th onClick={() => sort(field)} className="catalog-table-sortable" style={right ? { textAlign: 'right' } : undefined}>{labelWithTip(label, tip)}<span className="catalog-table-sort-indicator">{indicator(field)}</span></th>;
  return <div style={{ overflowX: 'auto' }}><table className="imc-sessions-table"><thead><tr>{head('Model', 'imcModelID', 'model_id')}{head('Cache Entry', 'imcSystemCacheID', 'id')}{head('System Tokens', 'imcSystemContext', 'tokens', true)}{head('Allocated', 'imcSystemAllocated', 'allocated', true)}{head('Snapshot Memory', 'imcSystemSnapshotBytes', 'snapshot_bytes', true)}{head('Restores', 'imcSystemRestores', 'restore_count', true)}{head('Active Restores', 'imcSystemActiveRestores', 'active_restores', true)}{head('Last Used', 'imcLastUsed', 'last_used')}</tr></thead><tbody>{data.map((cache) => <tr key={`${cache.model_id}-${cache.id}`}><td>{cache.model_id}</td><td>{cache.id}</td><td className="imc-system-cell" style={{ textAlign: 'right' }}>{cache.tokens.toLocaleString()}</td><td className="imc-system-cell" style={{ textAlign: 'right' }}>{cache.allocated.toLocaleString()}</td><td className="imc-system-cell" style={{ textAlign: 'right' }}>{formatBytes(cache.snapshot_bytes)}</td><td style={{ textAlign: 'right' }}>{cache.restore_count.toLocaleString()}</td><td style={{ textAlign: 'right' }}>{cache.active_restores.toLocaleString()}</td><td style={{ whiteSpace: 'nowrap' }}>{formatDate(cache.last_used)}</td></tr>)}</tbody></table></div>;
}
