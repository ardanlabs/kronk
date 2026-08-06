import { useEffect, useState } from 'react';
import { api } from '../services/api';
import type { IMCSessionsResponse } from '../types';
import { labelWithTip } from './ParamTooltips';

type SortField = 'model_id' | 'id' | 'state' | 'input_messages' | 'input_tokens' | 'reusable_messages' | 'reusable_tokens' | 'output_tokens' | 'request_total' | 'request_utilization' | 'total_allocated' | 'peak_context' | 'context_window' | 'utilization' | 'last_used' | 'has_media';

const STATE_ORDER = { active: 0, idle: 1, empty: 2 } as const;
const ALL_MODELS = '';

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

function requestTotal(inputTokens: number, outputTokens: number): number {
  return inputTokens + outputTokens;
}

function modelTabLabel(modelID: string): string {
  return modelID.split('/').at(-1) || modelID;
}

export default function IMCSessions() {
  const [data, setData] = useState<IMCSessionsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
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
      case 'input_messages':
        comparison = a.input_messages - b.input_messages;
        break;
      case 'input_tokens':
        comparison = a.input_tokens - b.input_tokens;
        break;
      case 'reusable_messages':
        comparison = a.reusable_messages - b.reusable_messages;
        break;
      case 'reusable_tokens':
        comparison = a.reusable_tokens - b.reusable_tokens;
        break;
      case 'output_tokens':
        comparison = a.output_tokens - b.output_tokens;
        break;
      case 'request_total':
        comparison = requestTotal(a.input_tokens, a.output_tokens) - requestTotal(b.input_tokens, b.output_tokens);
        break;
      case 'request_utilization':
        comparison = (a.context_window > 0 ? requestTotal(a.input_tokens, a.output_tokens) / a.context_window : 0)
          - (b.context_window > 0 ? requestTotal(b.input_tokens, b.output_tokens) / b.context_window : 0);
        break;
      case 'total_allocated':
        comparison = a.total_allocated - b.total_allocated;
        break;
      case 'peak_context':
        comparison = a.peak_context - b.peak_context;
        break;
      case 'context_window':
        comparison = a.context_window - b.context_window;
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
            Current bounded Incremental Message Cache entries for loaded models
          </p>
          <p className="page-description" style={{ marginTop: 6 }}>
            Each loaded model owns an independent set of entries. Unloading a model removes its entries.{' '}
            <strong>Active</strong> means the entry is reserved while being restored or updated.{' '}
            <strong>Idle</strong> means it contains a published snapshot ready for reuse.{' '}
            <strong>Empty</strong> means it has no cached snapshot.
          </p>
          <p className="page-description" style={{ marginTop: 6 }}>
            <strong>Total Allocated</strong> is the high-water context for the session, including generated output, and does not decrease when the session is reused with a smaller context.{' '}
            <strong>Request Used</strong> shows the latest input plus output against the window.{' '}
            <strong>Peak Used</strong> shows the largest execution depth observed for the session.
          </p>
        </div>
        <button className="btn btn-primary" onClick={() => loadSessions()} disabled={loading}>
          Refresh
        </button>
      </div>

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
            <table>
              <thead>
                <tr>
                  <th onClick={() => handleSort('model_id')} className="catalog-table-sortable">
                    {labelWithTip('Model', 'imcModelID')}{sortIndicator('model_id')}
                  </th>
                  <th onClick={() => handleSort('id')} className="catalog-table-sortable">
                    {labelWithTip('Cache Entry', 'imcSessionID')}{sortIndicator('id')}
                  </th>
                  <th onClick={() => handleSort('state')} className="catalog-table-sortable">
                    {labelWithTip('State', 'imcState')}{sortIndicator('state')}
                  </th>
                  <th onClick={() => handleSort('input_messages')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Input Msgs', 'imcInputMessages')}{sortIndicator('input_messages')}
                  </th>
                  <th onClick={() => handleSort('input_tokens')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Input Tokens', 'imcInputTokens')}{sortIndicator('input_tokens')}
                  </th>
                  <th onClick={() => handleSort('output_tokens')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Output Tokens', 'imcOutputTokens')}{sortIndicator('output_tokens')}
                  </th>
                  <th onClick={() => handleSort('request_total')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Request Total', 'imcRequestTotal')}{sortIndicator('request_total')}
                  </th>
                  <th onClick={() => handleSort('request_utilization')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Request Used', 'imcRequestUtilization')}{sortIndicator('request_utilization')}
                  </th>
                  <th onClick={() => handleSort('reusable_messages')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Reusable Msgs', 'imcReusableMessages')}{sortIndicator('reusable_messages')}
                  </th>
                  <th onClick={() => handleSort('reusable_tokens')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Reusable Tokens', 'imcReusableTokens')}{sortIndicator('reusable_tokens')}
                  </th>
                  <th onClick={() => handleSort('total_allocated')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Total Allocated', 'imcTotalAllocated')}{sortIndicator('total_allocated')}
                  </th>
                  <th onClick={() => handleSort('peak_context')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Peak Context', 'imcPeakContext')}{sortIndicator('peak_context')}
                  </th>
                  <th onClick={() => handleSort('utilization')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Peak Used', 'imcUtilization')}{sortIndicator('utilization')}
                  </th>
                  <th onClick={() => handleSort('context_window')} className="catalog-table-sortable" style={{ textAlign: 'right' }}>
                    {labelWithTip('Window', 'imcContextWindow')}{sortIndicator('context_window')}
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
                    <td style={{ textAlign: 'right' }}>{session.input_messages.toLocaleString()}</td>
                    <td style={{ textAlign: 'right' }}>{session.input_tokens.toLocaleString()}</td>
                    <td style={{ textAlign: 'right' }}>{session.output_tokens.toLocaleString()}</td>
                    <td style={{ textAlign: 'right' }}>{requestTotal(session.input_tokens, session.output_tokens).toLocaleString()}</td>
                    <td style={{ textAlign: 'right' }}>{utilization(requestTotal(session.input_tokens, session.output_tokens), session.context_window)}</td>
                    <td style={{ textAlign: 'right' }}>{session.reusable_tokens > 0 && session.reusable_messages === 0 ? '—' : session.reusable_messages.toLocaleString()}</td>
                    <td style={{ textAlign: 'right' }}>{session.reusable_tokens.toLocaleString()}</td>
                    <td style={{ textAlign: 'right' }}>{session.total_allocated.toLocaleString()}</td>
                    <td style={{ textAlign: 'right' }}>{session.peak_context.toLocaleString()}</td>
                    <td style={{ textAlign: 'right' }}>{utilization(session.peak_context, session.context_window)}</td>
                    <td style={{ textAlign: 'right' }}>{session.context_window.toLocaleString()}</td>
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
