import { useEffect, useState } from 'react';
import { api } from '../services/api';
import type { IMCSessionsResponse } from '../types';
import { labelWithTip, PARAM_TOOLTIPS, type TooltipKey } from './ParamTooltips';

type SortField = 'model_id' | 'id' | 'state' | 'messages' | 'context' | 'allocated' | 'reusable_tokens' | 'checkpoint_allocated' | 'peak_context' | 'utilization' | 'last_used' | 'has_media';

const STATE_ORDER = { active: 0, idle: 1, empty: 2 } as const;
const ALL_MODELS = '';

const FIELD_GUIDE: ReadonlyArray<{ label: string; tooltipKey: TooltipKey }> = [
  { label: 'Model', tooltipKey: 'imcModelID' },
  { label: 'Cache Entry', tooltipKey: 'imcSessionID' },
  { label: 'State', tooltipKey: 'imcState' },
  { label: 'Current Messages', tooltipKey: 'imcMessages' },
  { label: 'Current Tokens', tooltipKey: 'imcContext' },
  { label: 'Current Allocated', tooltipKey: 'imcAllocated' },
  { label: 'Fallback Tokens', tooltipKey: 'imcFallbackTokens' },
  { label: 'Fallback Allocated', tooltipKey: 'imcCheckpointAllocated' },
  { label: 'Peak Context', tooltipKey: 'imcPeakContext' },
  { label: 'Peak Used', tooltipKey: 'imcUtilization' },
  { label: 'Media', tooltipKey: 'imcMedia' },
  { label: 'Last Used', tooltipKey: 'imcLastUsed' },
];

function formatDate(dateStr: string): string {
  if (!dateStr || dateStr.startsWith('0001-01-01')) return '—';
  const date = new Date(dateStr);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleString();
}

function utilization(peakContext: number, contextWindow: number): string {
  if (contextWindow <= 0) return '0%';
  return `${((peakContext / contextWindow) * 100).toFixed(1)}%`;
}

function modelTabLabel(modelID: string): string {
  return modelID.split('/').at(-1) || modelID;
}

export default function IMCSessions() {
  const [data, setData] = useState<IMCSessionsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showFieldGuide, setShowFieldGuide] = useState(false);
  const [selectedModel, setSelectedModel] = useState(ALL_MODELS);
  const [sortField, setSortField] = useState<SortField>('utilization');
  const [sortAscending, setSortAscending] = useState(false);

  const loadSessions = async (silent = false) => {
    if (!silent) setLoading(true);
    setError(null);
    try {
      const sessions = await api.listIMCSessions();
      setData(sessions);
      setSelectedModel((current) => (
        current === ALL_MODELS || sessions.some((session) => session.model_id === current)
          ? current
          : ALL_MODELS
      ));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load IMC sessions');
    } finally {
      if (!silent) setLoading(false);
    }
  };

  useEffect(() => {
    loadSessions();

    const id = window.setInterval(() => {
      loadSessions(true);
    }, 5000);

    return () => window.clearInterval(id);
  }, []);

  useEffect(() => {
    if (!showFieldGuide) return;

    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setShowFieldGuide(false);
    };
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [showFieldGuide]);

  const handleSort = (field: SortField) => {
    if (field === sortField) {
      setSortAscending((ascending) => !ascending);
      return;
    }

    setSortField(field);
    setSortAscending(true);
  };

  const sortIndicator = (field: SortField) => (
    <span className="catalog-table-sort-indicator">
      {sortField === field ? (sortAscending ? ' ▲' : ' ▼') : ''}
    </span>
  );

  const modelIDs = [...new Set((data ?? []).map((session) => session.model_id))]
    .sort((a, b) => a.localeCompare(b));

  const visibleData = selectedModel === ALL_MODELS
    ? data
    : data?.filter((session) => session.model_id === selectedModel);

  const sortedData = visibleData ? [...visibleData].sort((a, b) => {
    let comparison = 0;
    switch (sortField) {
      case 'model_id':
        comparison = a.model_id.localeCompare(b.model_id);
        break;
      case 'id':
        comparison = a.id - b.id;
        break;
      case 'state':
        comparison = STATE_ORDER[a.state] - STATE_ORDER[b.state];
        break;
      case 'messages':
        comparison = a.messages - b.messages;
        break;
      case 'context':
        comparison = a.context - b.context;
        break;
      case 'allocated':
        comparison = a.allocated - b.allocated;
        break;
      case 'reusable_tokens':
        comparison = a.reusable_tokens - b.reusable_tokens;
        break;
      case 'checkpoint_allocated':
        comparison = a.checkpoint_allocated - b.checkpoint_allocated;
        break;
      case 'peak_context':
        comparison = a.peak_context - b.peak_context;
        break;
      case 'utilization':
        comparison = (a.context_window > 0 ? a.peak_context / a.context_window : 0)
          - (b.context_window > 0 ? b.peak_context / b.context_window : 0);
        break;
      case 'last_used':
        comparison = Date.parse(a.last_used) - Date.parse(b.last_used);
        break;
      case 'has_media':
        comparison = Number(a.has_media) - Number(b.has_media);
        break;
    }

    if (comparison === 0) {
      comparison = a.model_id.localeCompare(b.model_id) || a.id - b.id;
    }
    return sortAscending ? comparison : -comparison;
  }) : null;

  return (
    <div>
      <div className="page-header-with-action">
        <div>
          <h2>IMC Sessions</h2>
          <p className="page-description">
            Current working and fallback cache snapshots for loaded models
          </p>
        </div>
        <div style={{ display: 'flex', gap: '8px' }}>
          <button className="btn btn-secondary" onClick={() => setShowFieldGuide(true)}>
            Field Guide
          </button>
          <button className="btn btn-primary" onClick={() => loadSessions()} disabled={loading}>
            Refresh
          </button>
        </div>
      </div>

      {showFieldGuide && (
        <div className="modal-overlay" role="dialog" aria-modal="true" aria-labelledby="imc-field-guide-title" onClick={() => setShowFieldGuide(false)}>
          <div className="modal-content" onClick={(event) => event.stopPropagation()}>
            <div className="modal-header">
              <h3 id="imc-field-guide-title">IMC Session Field Guide</h3>
              <button className="modal-close" onClick={() => setShowFieldGuide(false)} aria-label="Close field guide">
                ×
              </button>
            </div>
            <div className="modal-body">
              <p style={{ marginTop: 0 }}>
                Each loaded model owns an independent bounded set of cache entries. Unloading a model removes its entries.
              </p>
              <dl style={{ margin: '24px 0 0', display: 'grid', gap: '20px' }}>
                {FIELD_GUIDE.map(({ label, tooltipKey }) => (
                  <div key={tooltipKey}>
                    <dt><strong>{label}</strong></dt>
                    <dd style={{ margin: '4px 0 0' }}>{PARAM_TOOLTIPS[tooltipKey]}</dd>
                  </div>
                ))}
              </dl>
            </div>
          </div>
        </div>
      )}

      {error && <div className="alert alert-error">{error}</div>}

      {data && data.length > 0 && (
        <div className="tabs imc-model-tabs" role="tablist" aria-label="IMC sessions by model">
          <button
            type="button"
            role="tab"
            aria-selected={selectedModel === ALL_MODELS}
            className={`tab ${selectedModel === ALL_MODELS ? 'active' : ''}`}
            onClick={() => setSelectedModel(ALL_MODELS)}
          >
            All
          </button>
          {modelIDs.map((modelID) => (
            <button
              key={modelID}
              type="button"
              role="tab"
              aria-selected={selectedModel === modelID}
              className={`tab imc-model-tab ${selectedModel === modelID ? 'active' : ''}`}
              title={modelID}
              onClick={() => setSelectedModel(modelID)}
            >
              <span className="imc-model-tab-label">{modelTabLabel(modelID)}</span>
            </button>
          ))}
        </div>
      )}

      <div className="card">
        {loading && !data ? (
          <div className="loading">Loading IMC sessions...</div>
        ) : sortedData && sortedData.length > 0 ? (
          <div style={{ overflowX: 'auto' }}>
            <table className="imc-sessions-table">
              <thead>
                <tr className="imc-table-groups">
                  <th colSpan={3} className="imc-table-group imc-table-group-session">Session</th>
                  <th colSpan={3} className="imc-table-group imc-table-group-current">
                    Current Working Cache
                  </th>
                  <th colSpan={2} className="imc-table-group imc-table-group-fallback">Fallback Cache</th>
                  <th colSpan={2} className="imc-table-group imc-table-group-capacity">Capacity / Usage</th>
                  <th colSpan={2} className="imc-table-group imc-table-group-details">Details</th>
                </tr>
                <tr className="imc-table-columns">
                  <th onClick={() => handleSort('model_id')} className="catalog-table-sortable">
                    {labelWithTip('Model', 'imcModelID')}{sortIndicator('model_id')}
                  </th>
                  <th onClick={() => handleSort('id')} className="catalog-table-sortable">
                    {labelWithTip('Cache Entry', 'imcSessionID')}{sortIndicator('id')}
                  </th>
                  <th onClick={() => handleSort('state')} className="catalog-table-sortable">
                    {labelWithTip('State', 'imcState')}{sortIndicator('state')}
                  </th>
                  <th onClick={() => handleSort('messages')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Messages', 'imcMessages')}{sortIndicator('messages')}
                  </th>
                  <th onClick={() => handleSort('context')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Tokens', 'imcContext')}{sortIndicator('context')}
                  </th>
                  <th onClick={() => handleSort('allocated')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Allocated', 'imcAllocated')}{sortIndicator('allocated')}
                  </th>
                  <th onClick={() => handleSort('reusable_tokens')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Tokens', 'imcFallbackTokens')}{sortIndicator('reusable_tokens')}
                  </th>
                  <th onClick={() => handleSort('checkpoint_allocated')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Allocated', 'imcCheckpointAllocated')}{sortIndicator('checkpoint_allocated')}
                  </th>
                  <th onClick={() => handleSort('peak_context')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Peak Context', 'imcPeakContext')}{sortIndicator('peak_context')}
                  </th>
                  <th onClick={() => handleSort('utilization')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Peak Used', 'imcUtilization')}{sortIndicator('utilization')}
                  </th>
                  <th onClick={() => handleSort('has_media')} className="catalog-table-sortable">
                    {labelWithTip('Media', 'imcMedia')}{sortIndicator('has_media')}
                  </th>
                  <th onClick={() => handleSort('last_used')} className="catalog-table-sortable">
                    {labelWithTip('Last Used', 'imcLastUsed')}{sortIndicator('last_used')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {sortedData.map((session) => (
                  <tr key={`${session.model_id}-${session.id}`}>
                    <td>{session.model_id}</td>
                    <td>{session.id}</td>
                    <td><span className={`badge badge-${session.state}`}>{session.state}</span></td>
                    <td className="imc-current-cell" style={{ textAlign: 'right' }}>{session.messages.toLocaleString()}</td>
                    <td className="imc-current-cell" style={{ textAlign: 'right' }}>{session.context.toLocaleString()}</td>
                    <td className="imc-current-cell" style={{ textAlign: 'right' }}>{session.allocated.toLocaleString()}</td>
                    <td className="imc-fallback-cell" style={{ textAlign: 'right' }}>{session.reusable_tokens > 0 ? session.reusable_tokens.toLocaleString() : '—'}</td>
                    <td className="imc-fallback-cell" style={{ textAlign: 'right' }}>{session.checkpoint_allocated > 0 ? session.checkpoint_allocated.toLocaleString() : '—'}</td>
                    <td className="imc-capacity-cell" style={{ textAlign: 'right' }}>{session.peak_context.toLocaleString()}</td>
                    <td className="imc-capacity-cell" style={{ textAlign: 'right' }}>{utilization(session.peak_context, session.context_window)}</td>
                    <td><span className={`badge badge-${session.has_media ? 'yes' : 'no'}`}>{session.has_media ? 'yes' : 'no'}</span></td>
                    <td style={{ whiteSpace: 'nowrap' }}>{formatDate(session.last_used)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="empty-state">
            <h3>No IMC sessions</h3>
            <p>Entries will appear when an IMC-enabled model is loaded</p>
          </div>
        )}
      </div>
    </div>
  );
}
