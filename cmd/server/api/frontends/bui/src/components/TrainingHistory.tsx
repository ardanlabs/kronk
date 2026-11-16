import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../services/api';
import type { TrainingRun, TrainingRunsResponse } from '../types';

export default function TrainingHistory({ onSelect }: { onSelect: (run: TrainingRun) => void }) {
  const [pages, setPages] = useState<TrainingRunsResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const request = useRef<AbortController | null>(null);
  const watch = useRef<AbortController | null>(null);
  const runs = pages.flatMap((page) => page.runs).filter((run) => !run.preparation_only);
  const title = 'Training History';
  const nextPageToken = pages.at(-1)?.next_page_token;
  const loaded = pages.length > 0;

  const load = useCallback(async (token = '', pageCount = 1) => {
    request.current?.abort();
    watch.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setError('');
    try {
      const updated: TrainingRunsResponse[] = [];
      let cursor = token;
      for (let i = 0; i < pageCount; i++) {
        const page = await api.trainingRuns(cursor, controller.signal);
        if (controller.signal.aborted) return;
        updated.push(page);
        if (!page.next_page_token) break;
        cursor = page.next_page_token;
      }
      setPages((previous) => token ? [...previous, ...updated] : updated);
    } catch (err) {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'Could not load history.');
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
    return () => request.current?.abort();
  }, [load]);

  useEffect(() => {
    if (pages.length === 0) return;
    const controller = new AbortController();
    watch.current = controller;
    // Watch every displayed page, including older jobs loaded with Load more.
    pages.forEach((page, index) => {
      const token = index === 0 ? '' : pages[index - 1].next_page_token ?? '';
      void (async () => {
        try {
          while (!controller.signal.aborted) {
            const update = await api.trainingRuns(token, controller.signal, page.revision);
            if (controller.signal.aborted) return;
            if (update.revision !== page.revision) {
              // Rebuild the loaded prefix so insertions/deletions cannot mix cursors.
              controller.abort();
              void load('', pages.length);
              return;
            }
          }
        } catch (err) {
          if (!controller.signal.aborted) {
            controller.abort();
            setError(`Live updates paused: ${err instanceof Error ? err.message : 'Connection failed'}`);
          }
        }
      })();
    });
    return () => controller.abort();
  }, [pages, load]);

  return (
    <section className="fine-panel fine-history" aria-label={title} aria-busy={loading}>
      <div className="fine-panel-heading">
        <div><span>Archive</span><h3>{title}</h3></div>
        <button className="btn btn-primary" disabled={loading} onClick={() => void load('', Math.max(1, pages.length))}>Refresh</button>
      </div>
      {error && <div className="fine-error" role="alert">Could not load training history: {error}. {loaded ? 'The displayed jobs are from the last successful request. ' : ''}Select Refresh to retry.</div>}
      {loading && <p role="status">Loading training history…</p>}
      {loaded && runs.length === 0 && !loading && !error && <div className="fine-history-empty"><strong>No saved runs</strong><span>Training jobs will appear here after they are submitted.</span></div>}
      {runs.length > 0 && (
        <div className="fine-history-table-wrap">
          <table className="fine-history-table">
            <thead><tr><th scope="col">Run</th><th scope="col">Status</th><th scope="col">Started</th></tr></thead>
            <tbody>{runs.map((run) => (
              <tr key={`${run.workflow_id}:${run.run_id}`}>
                <td><button className="btn-link" onClick={() => onSelect(run)}>{run.workflow_id}</button></td>
                <td><span className={`fine-status fine-status-${run.status}`}>{run.status.replaceAll('_', ' ')}</span></td>
                <td>{!run.started_at || run.started_at.startsWith('0001-') ? '—' : new Date(run.started_at).toLocaleString()}</td>
              </tr>
            ))}</tbody>
          </table>
        </div>
      )}
      {nextPageToken && <div className="fine-actions fine-actions-end"><button className="btn btn-primary" disabled={loading} onClick={() => void load(nextPageToken)}>Load more</button></div>}
    </section>
  );
}
