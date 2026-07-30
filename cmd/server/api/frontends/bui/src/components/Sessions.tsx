import { useCallback, useEffect, useState } from 'react';
import { api } from '../services/api';
import type {
  SessionPageResponse,
  SessionState,
  SessionSummary,
  SessionSummaryResponse,
} from '../types';

const PAGE_SIZE = 10;

function formatNumber(value: number): string {
  return value.toLocaleString();
}

function formatPercent(value: number): string {
  return `${(value * 100).toFixed(1)}%`;
}

function formatDate(value?: string): string {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString();
}

function sessionUtilization(session: SessionSummary): string {
  if (session.context_window <= 0) return '—';
  return formatPercent(session.peak_context / session.context_window);
}

interface SessionTableProps {
  title: string;
  state: SessionState;
  page: SessionPageResponse | null;
  offset: number;
  total: number;
  onOffsetChange: (offset: number) => void;
}

function SessionTable({ title, state, page, offset, total, onOffsetChange }: SessionTableProps) {
  const sessions = page?.sessions ?? [];
  const first = sessions.length === 0 ? 0 : offset + 1;
  const last = sessions.length === 0 ? 0 : Math.min(offset + sessions.length, total);

  return (
    <div className="card session-table-card">
      <div className="session-table-heading">
        <h4>{title} ({formatNumber(total)})</h4>
        <span>{first}–{last} of {formatNumber(total)}</span>
      </div>
      {sessions.length > 0 ? (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Model</th>
                <th>Session</th>
                <th style={{ textAlign: 'right' }}>Requests</th>
                {state !== 'completed' && <th style={{ textAlign: 'right' }}>Current Context</th>}
                <th style={{ textAlign: 'right' }}>Peak Context</th>
                <th style={{ textAlign: 'right' }}>Window</th>
                <th style={{ textAlign: 'right' }}>Utilization</th>
                <th style={{ textAlign: 'right' }}>Processed</th>
                <th>Last Active</th>
                {state === 'completed' && <th>Completed</th>}
              </tr>
            </thead>
            <tbody>
              {sessions.map((session) => (
                <tr key={`${session.model_id}:${session.session_id}`}>
                  <td>{session.model_id}</td>
                  <td className="session-id-cell" title={session.session_id}>{session.session_id}</td>
                  <td style={{ textAlign: 'right' }}>{formatNumber(session.request_count)}</td>
                  {state !== 'completed' && (
                    <td style={{ textAlign: 'right' }}>{formatNumber(session.current_context ?? 0)}</td>
                  )}
                  <td style={{ textAlign: 'right' }}>{formatNumber(session.peak_context)}</td>
                  <td style={{ textAlign: 'right' }}>{formatNumber(session.context_window)}</td>
                  <td style={{ textAlign: 'right' }}>
                    {sessionUtilization(session)}
                    {session.context_full && <span className="session-context-full">Full</span>}
                  </td>
                  <td style={{ textAlign: 'right' }}>{formatNumber(session.total_processed_tokens ?? 0)}</td>
                  <td style={{ whiteSpace: 'nowrap' }}>{formatDate(session.last_active_at)}</td>
                  {state === 'completed' && <td style={{ whiteSpace: 'nowrap' }}>{formatDate(session.ended_at)}</td>}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="session-empty">No {title.toLowerCase()} sessions</div>
      )}
      <div className="session-pagination">
        <button
          className="btn btn-secondary btn-sm"
          disabled={offset === 0}
          onClick={() => onOffsetChange(Math.max(0, offset - PAGE_SIZE))}
        >
          Previous
        </button>
        <button
          className="btn btn-secondary btn-sm"
          disabled={!page?.has_more}
          onClick={() => onOffsetChange(page?.next_offset ?? offset + PAGE_SIZE)}
        >
          Next
        </button>
      </div>
    </div>
  );
}

function Summary({ summary }: { summary: SessionSummaryResponse }) {
  const percentileKeys = ['p50', 'p90', 'p95', 'p99', 'max'] as const;

  return (
    <div className="session-summary">
      <div className="session-summary-counts">
        <span><strong>{formatNumber(summary.active)}</strong> Active</span>
        <span><strong>{formatNumber(summary.idle)}</strong> Idle</span>
        <span><strong>{formatNumber(summary.completed)}</strong> Completed</span>
        <span><strong>{formatNumber(summary.total)}</strong> Total</span>
      </div>
      <table>
        <thead>
          <tr>
            <th></th>
            {percentileKeys.map((key) => <th key={key}>{key.toUpperCase()}</th>)}
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>Context tokens</td>
            {percentileKeys.map((key) => <td key={key}>{formatNumber(summary.context_tokens[key])}</td>)}
          </tr>
          <tr>
            <td>Utilization</td>
            {percentileKeys.map((key) => <td key={key}>{formatPercent(summary.utilization[key])}</td>)}
          </tr>
        </tbody>
      </table>
    </div>
  );
}

export default function Sessions() {
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [summary, setSummary] = useState<SessionSummaryResponse | null>(null);
  const [pages, setPages] = useState<Record<SessionState, SessionPageResponse | null>>({
    active: null,
    idle: null,
    completed: null,
  });
  const [offsets, setOffsets] = useState<Record<SessionState, number>>({
    active: 0,
    idle: 0,
    completed: 0,
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    setError(null);
    try {
      const status = await api.getSessionStatus();
      setEnabled(status.enabled);
      if (!status.enabled) return;

      const [nextSummary, active, idle, completed] = await Promise.all([
        api.getSessionSummary(),
        api.listSessions('active', { limit: PAGE_SIZE, offset: offsets.active }),
        api.listSessions('idle', { limit: PAGE_SIZE, offset: offsets.idle }),
        api.listSessions('completed', { limit: PAGE_SIZE, offset: offsets.completed }),
      ]);
      setSummary(nextSummary);
      setPages({ active, idle, completed });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sessions');
    } finally {
      if (!silent) setLoading(false);
    }
  }, [offsets]);

  useEffect(() => {
    load();
    const id = window.setInterval(() => load(true), 5000);
    return () => window.clearInterval(id);
  }, [load]);

  const setOffset = (state: SessionState, offset: number) => {
    setOffsets((current) => ({ ...current, [state]: offset }));
  };

  return (
    <section className="sessions-section">
      <div className="sessions-header">
        <div>
          <h3>Sessions</h3>
          <p>System-wide model context usage across current and completed sessions.</p>
        </div>
        {enabled && summary && <Summary summary={summary} />}
      </div>

      {loading && <div className="card loading">Loading sessions</div>}
      {error && <div className="alert alert-error">{error}</div>}
      {!loading && !error && enabled === false && (
        <div className="card session-empty">
          Session observability is disabled. Enable it in the server configuration to collect context usage.
        </div>
      )}
      {!loading && !error && enabled && summary && (
        <>
          <SessionTable
            title="Active"
            state="active"
            page={pages.active}
            offset={offsets.active}
            total={summary.active}
            onOffsetChange={(offset) => setOffset('active', offset)}
          />
          <SessionTable
            title="Idle"
            state="idle"
            page={pages.idle}
            offset={offsets.idle}
            total={summary.idle}
            onOffsetChange={(offset) => setOffset('idle', offset)}
          />
          <SessionTable
            title="Completed"
            state="completed"
            page={pages.completed}
            offset={offsets.completed}
            total={summary.completed}
            onOffsetChange={(offset) => setOffset('completed', offset)}
          />
        </>
      )}
    </section>
  );
}
