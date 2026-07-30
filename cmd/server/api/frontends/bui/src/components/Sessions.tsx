import { useCallback, useEffect, useMemo, useState } from 'react';
import { api } from '../services/api';
import type { SessionSummary, SessionsResponse } from '../types';

type SortKey =
  | 'model_id'
  | 'session_id'
  | 'state'
  | 'request_count'
  | 'current_context'
  | 'peak_context'
  | 'context_window'
  | 'utilization'
  | 'total_processed_tokens'
  | 'last_active_at';

type SortDirection = 'asc' | 'desc';

function formatNumber(value: number): string {
  return value.toLocaleString();
}

function formatPercent(value: number): string {
  return `${(value * 100).toFixed(1)}%`;
}

function formatDate(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString();
}

function utilization(session: SessionSummary): number {
  if (session.context_window <= 0) return 0;
  return session.peak_context / session.context_window;
}

function sortValue(session: SessionSummary, key: SortKey): string | number {
  switch (key) {
    case 'utilization':
      return utilization(session);
    case 'last_active_at':
      return new Date(session.last_active_at).getTime();
    case 'current_context':
    case 'total_processed_tokens':
      return session[key] ?? 0;
    default:
      return session[key];
  }
}

function compare(a: string | number, b: string | number): number {
  if (typeof a === 'number' && typeof b === 'number') return a - b;
  return String(a).localeCompare(String(b));
}

interface SortHeadingProps {
  label: string;
  column: SortKey;
  sortKey: SortKey;
  direction: SortDirection;
  numeric?: boolean;
  onSort: (column: SortKey) => void;
}

function SortHeading({ label, column, sortKey, direction, numeric, onSort }: SortHeadingProps) {
  const active = sortKey === column;
  return (
    <th
      className="sortable-th"
      style={numeric ? { textAlign: 'right' } : undefined}
      onClick={() => onSort(column)}
      aria-sort={active ? (direction === 'asc' ? 'ascending' : 'descending') : 'none'}
    >
      {label}{active ? (direction === 'asc' ? ' ↑' : ' ↓') : ''}
    </th>
  );
}

export default function Sessions() {
  const [data, setData] = useState<SessionsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sortKey, setSortKey] = useState<SortKey>('peak_context');
  const [direction, setDirection] = useState<SortDirection>('desc');

  const load = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    setError(null);
    try {
      setData(await api.listSessions());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sessions');
    } finally {
      if (!silent) setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
    const id = window.setInterval(() => load(true), 5000);
    return () => window.clearInterval(id);
  }, [load]);

  const sessions = useMemo(() => {
    const sorted = [...(data?.sessions ?? [])];
    sorted.sort((a, b) => {
      const result = compare(sortValue(a, sortKey), sortValue(b, sortKey));
      return direction === 'asc' ? result : -result;
    });
    return sorted;
  }, [data, direction, sortKey]);

  const handleSort = (column: SortKey) => {
    if (column === sortKey) {
      setDirection((current) => current === 'asc' ? 'desc' : 'asc');
      return;
    }
    setSortKey(column);
    setDirection(column === 'model_id' || column === 'session_id' || column === 'state' ? 'asc' : 'desc');
  };

  return (
    <section className="sessions-section">
      <div className="page-header-with-action sessions-header">
        <div>
          <h2>Sessions</h2>
          <p className="page-description">Latest system-wide context usage held in memory for this server process.</p>
        </div>
        <button className="btn btn-primary" onClick={() => load()} disabled={loading}>Refresh</button>
      </div>

      {loading && <div className="card loading">Loading sessions</div>}
      {error && <div className="alert alert-error">{error}</div>}
      {!loading && !error && data && !data.enabled && (
        <div className="card session-empty">
          Session observability is disabled. Enable it in the server configuration to collect context usage.
        </div>
      )}
      {!loading && !error && data?.enabled && (
        <div className="card session-table-card">
          <div className="session-table-heading">
            <h3>All Sessions ({formatNumber(sessions.length)})</h3>
          </div>
          {sessions.length > 0 ? (
            <div className="table-container">
              <table>
                <thead>
                  <tr>
                    <SortHeading label="Model" column="model_id" sortKey={sortKey} direction={direction} onSort={handleSort} />
                    <SortHeading label="Session" column="session_id" sortKey={sortKey} direction={direction} onSort={handleSort} />
                    <SortHeading label="State" column="state" sortKey={sortKey} direction={direction} onSort={handleSort} />
                    <SortHeading label="Requests" column="request_count" sortKey={sortKey} direction={direction} numeric onSort={handleSort} />
                    <SortHeading label="Current Context" column="current_context" sortKey={sortKey} direction={direction} numeric onSort={handleSort} />
                    <SortHeading label="Peak Context" column="peak_context" sortKey={sortKey} direction={direction} numeric onSort={handleSort} />
                    <SortHeading label="Window" column="context_window" sortKey={sortKey} direction={direction} numeric onSort={handleSort} />
                    <SortHeading label="Utilization" column="utilization" sortKey={sortKey} direction={direction} numeric onSort={handleSort} />
                    <SortHeading label="Processed" column="total_processed_tokens" sortKey={sortKey} direction={direction} numeric onSort={handleSort} />
                    <SortHeading label="Last Active" column="last_active_at" sortKey={sortKey} direction={direction} onSort={handleSort} />
                  </tr>
                </thead>
                <tbody>
                  {sessions.map((session) => (
                    <tr key={`${session.model_id}:${session.session_id}`}>
                      <td>{session.model_id}</td>
                      <td className="session-id-cell" title={session.session_id}>{session.session_id}</td>
                      <td><span className={`session-state session-state-${session.state}`}>{session.state}</span></td>
                      <td style={{ textAlign: 'right' }}>{formatNumber(session.request_count)}</td>
                      <td style={{ textAlign: 'right' }}>{formatNumber(session.current_context ?? 0)}</td>
                      <td style={{ textAlign: 'right' }}>{formatNumber(session.peak_context)}</td>
                      <td style={{ textAlign: 'right' }}>{formatNumber(session.context_window)}</td>
                      <td style={{ textAlign: 'right' }}>
                        {formatPercent(utilization(session))}
                        {session.context_full && <span className="session-context-full">Full</span>}
                      </td>
                      <td style={{ textAlign: 'right' }}>{formatNumber(session.total_processed_tokens ?? 0)}</td>
                      <td style={{ whiteSpace: 'nowrap' }}>{formatDate(session.last_active_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="session-empty">No sessions have been observed by this server process.</div>
          )}
        </div>
      )}
    </section>
  );
}
