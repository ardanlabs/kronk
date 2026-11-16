import { useEffect, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { requestMessage, retryDelay, stepLabel } from '../contexts/FineTuningRunnerContext';
import { api, ApiError } from '../services/api';
import type { TrainingRun } from '../types';
import { active, Badge, datasetName, date, failedStep, PreparationRunView, Progress, records, remaining, reviewable, runKey, runMeta, version } from './PreparationRuns';
import './PreparationRuns.css';
import './DatasetDashboard.css';

// The Dataset Dashboard lists preparation runs ten per page and opens each on its
// own screen. A finished dataset leads on to training.

const PAGE_SIZE = 10;
// The status filter runs on the server before paging.
const filters: [string, string, string[]][] = [
  ['all', 'All runs', []], ['active', 'In progress', ['queued', 'running', 'recovering', 'pause_requested', 'paused', 'waiting']],
  ['review', 'Needs review', ['waiting']], ['finished', 'Closed', ['completed', 'failed', 'stopped']],
];

function StepCell({ run }: { run: TrainingRun }) {
  const eta = remaining(run);
  if (run.status === 'completed' || run.status === 'needs_improvement') return run.prepared_dataset_version
    ? <><span className="prep-main dash-done-text"><span aria-hidden="true">✓ </span>Dataset ready</span><span className="prep-sub">{records(run.dataset_record_count)} in version {run.prepared_dataset_version} · {run.training_ready ? 'ready for training' : 'below the 50-record training minimum'}</span></>
    : <><span className="prep-main">Finished</span><span className="prep-sub">No saved version</span></>;
  if (run.status === 'failed') return <><span className="prep-main">Failed during {stepLabel(failedStep(run)?.name ?? run.step).toLowerCase()}</span><span className="prep-sub">Open for details</span></>;
  if (run.status === 'stopped') return <><span className="prep-main">Stopped</span><span className="prep-sub">Cancelled · cannot resume</span></>;
  if (run.control_action === 'pause') return <><span className="prep-main">{stepLabel(run.step)}</span><span className="prep-sub">{run.status === 'pause_requested' ? 'Pausing after the current save' : 'Paused · saved progress kept'}</span></>;
  if (reviewable(run)) return <><span className="prep-main">Preview ready</span><span className="prep-sub">Your review is needed</span></>;
  if (run.status === 'queued') return <><span className="prep-main">Waiting for a worker</span><span className="prep-sub">Not started</span></>;
  return <><span className="prep-main">{stepLabel(run.step)}</span>{run.status === 'recovering' ? <span className="prep-sub">Resuming from saved progress</span> : <Progress run={run} />}{eta && <span className="dash-eta">{eta}</span>}</>;
}

function Tabs() {
  return <nav className="tabs fine-tabs" aria-label="Fine-tuning views">
    <Link className="tab active" aria-current="page" to="/fine-tuning">Dashboard</Link>
    <Link className="tab" to="/fine-tuning?view=configure">Configure Training</Link>
  </nav>;
}

export default function DatasetDashboard({ visible }: { visible: boolean }) {
  const [params] = useSearchParams();
  const selected = params.get('preparation') && params.get('run') ? { workflow: params.get('preparation')!, run: params.get('run')! } : null;
  return <div className="fine-page prep-workspace">
    <div className="page-header fine-page-header"><div><h2>Fine-Tuning</h2></div></div>
    <Tabs />
    {selected ? <RunScreen key={`${selected.workflow}/${selected.run}`} workflowID={selected.workflow} runID={selected.run} visible={visible} /> : <RunList visible={visible} />}
  </div>;
}

function RunList({ visible }: { visible: boolean }) {
  const [filter, setFilter] = useState('all');
  const [newestFirst, setNewestFirst] = useState(true);
  // tokens[i] is the page token for page i; page 0 has none.
  const [tokens, setTokens] = useState(['']);
  const [page, setPage] = useState(0);
  const [list, setList] = useState<{ runs: TrainingRun[]; next: string } | null>(null);
  const [error, setError] = useState('');
  const [attempt, setAttempt] = useState(0);
  const heading = useRef<HTMLHeadingElement>(null);
  const shown = useRef(false);
  const statuses = filters.find(([key]) => key === filter)![2];
  const restart = () => { setTokens(['']); setPage(0); setList(null); };

  // The shown page long-polls: the server answers when that page changes.
  useEffect(() => {
    if (!visible) return;
    const controller = new AbortController();
    void (async () => {
      let revision = '';
      setError('');
      while (!controller.signal.aborted) {
        try {
          const result = await api.trainingRuns(tokens[page], controller.signal, revision, { mode: 'preparation', statuses, pageSize: PAGE_SIZE, order: newestFirst ? 'newest' : 'oldest' });
          if (controller.signal.aborted) return;
          setError('');
          if (result.revision !== revision) {
            revision = result.revision;
            setList({ runs: result.runs ?? [], next: result.next_page_token ?? '' });
          }
        } catch (err) {
          if (controller.signal.aborted) return;
          // A page token from before the list changed is rejected; start over from page 1.
          if (err instanceof ApiError && err.status === 400 && page > 0) { restart(); return; }
          setError(requestMessage(err, 'Could not load preparation runs.'));
          revision = '';
          await retryDelay(controller.signal);
        }
      }
    })();
    return () => controller.abort();
  }, [visible, filter, newestFirst, page, attempt]);

  // Move focus to the heading when the page changes, but not on first load.
  useEffect(() => {
    if (visible && shown.current) heading.current?.focus();
    shown.current = true;
  }, [visible, page]);

  const rows = list?.runs ?? [];
  const first = page * PAGE_SIZE + 1;

  return <>
    <div className="page-header-with-action prep-heading">
      <div><h2 ref={heading} tabIndex={-1}>Preparation runs</h2><p className="page-description">Each row is one run that turns documents into verified records for a dataset. Open a run to follow it or see its result.</p></div>
      <div className="prep-actions"><button className="btn btn-secondary" type="button" onClick={() => setAttempt((value) => value + 1)}>Refresh</button><Link className="btn btn-primary" to="/fine-tuning?view=prepare">+ New dataset</Link></div>
    </div>
    <div className="dash-filter">
      <label htmlFor="dash-filter">Sort by</label>
      <select id="dash-filter" value={filter} onChange={(event) => { setFilter(event.target.value); restart(); }}>
        {filters.map(([key, label]) => <option key={key} value={key}>{label}</option>)}
      </select>
    </div>
    {error && <div className="prep-load-error" role="alert"><span>Could not load preparation runs: {error} Retrying automatically.</span><button className="btn btn-secondary" type="button" onClick={() => setAttempt((value) => value + 1)}>Try again</button></div>}
    {!list && !error && <p className="prep-copy" role="status">Loading preparation runs…</p>}
    {list && <div className="catalog-table-wrap prep-table-wrap"><table className="catalog-table prep-table dash-table">
      <caption className="prep-sr-only">Preparation runs, page {page + 1}</caption>
      <thead><tr><th scope="col">Dataset</th><th scope="col">Status</th><th scope="col">Step</th><th scope="col" aria-sort={newestFirst ? 'descending' : 'ascending'}><button className="dash-sort-header" type="button" onClick={() => { setNewestFirst(!newestFirst); restart(); }}>Date created<span className="efficiency-sort-ind active" aria-hidden="true">{newestFirst ? '▼' : '▲'}</span><span className="prep-sr-only">, {newestFirst ? 'newest first' : 'oldest first'}. Select to reverse.</span></button></th><th scope="col"><span className="prep-sr-only">Open run</span></th></tr></thead>
      <tbody>
        {rows.map((run) => {
          const path = `/fine-tuning?${new URLSearchParams({ preparation: run.workflow_id, run: run.run_id })}`;
          return <tr key={runKey(run)}>
            <td><Link className="prep-run-link" to={path}>{datasetName(run)}</Link><span className="prep-sub">{version(run)}</span></td>
            <td data-label="Status"><Badge run={run} />{run.state_stale && active(run) && <span className="prep-sub prep-stale-tag">Last known status</span>}</td>
            <td data-label="Step"><StepCell run={run} /></td>
            <td data-label="Date created">{date(run.started_at)}</td>
            <td className="prep-row-action"><Link className={`btn ${reviewable(run) ? 'btn-primary' : 'btn-secondary'}`} aria-label={`Open ${datasetName(run)}, created ${date(run.started_at)}`} to={path}>Open</Link></td>
          </tr>;
        })}
        {!rows.length && <tr><td colSpan={5}><div className="empty-state">{filter === 'all' ? <><h3>No preparation runs yet</h3><p>Choose + New dataset to start one.</p></> : <><h3>No runs match this filter</h3><p>Choose All runs to see every preparation run.</p></>}</div></td></tr>}
      </tbody>
    </table></div>}
    {list && (page > 0 || list.next) && <nav className="dash-pager" aria-label="Pages">
      <span>{rows.length ? `Showing ${first}–${first + rows.length - 1}` : 'No runs'} · page {page + 1}</span>
      <div className="prep-actions">
        <button className="btn btn-secondary" type="button" disabled={page === 0} onClick={() => { setList(null); setPage(page - 1); }}>← Previous</button>
        <button className="btn btn-secondary" type="button" disabled={!list.next} onClick={() => { setTokens([...tokens.slice(0, page + 1), list.next]); setList(null); setPage(page + 1); }}>Next →</button>
      </div>
    </nav>}
  </>;
}

function RunScreen({ workflowID, runID, visible }: { workflowID: string; runID: string; visible: boolean }) {
  const [run, setRun] = useState<TrainingRun | null>(null);
  const [error, setError] = useState<{ text: string; missing: boolean } | null>(null);
  const [attempt, setAttempt] = useState(0);
  const heading = useRef<HTMLHeadingElement>(null);

  // Runs in progress refresh every few seconds; finished runs load once.
  useEffect(() => {
    if (!visible) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const load = async () => {
      try {
        const next = await api.trainingRun(workflowID, runID, controller.signal);
        if (controller.signal.aborted) return;
        setRun(next);
        setError(null);
        if (active(next)) timer = setTimeout(load, 3000);
      } catch (err) {
        if (controller.signal.aborted) return;
        const missing = err instanceof ApiError && err.status === 404;
        setError({ text: requestMessage(err, 'Could not load this run.'), missing });
        if (!missing) timer = setTimeout(load, 5000);
      }
    };
    void load();
    return () => { controller.abort(); clearTimeout(timer); };
  }, [visible, workflowID, runID, attempt]);

  useEffect(() => {
    if (visible) heading.current?.focus();
  }, [visible, !!run]);

  const refresh = () => setAttempt((value) => value + 1);
  return <>
    <Link className="prep-back" to="/fine-tuning">← Dashboard</Link>
    {run ? <>
      <div className="page-header-with-action prep-heading">
        <div><h2 ref={heading} tabIndex={-1}>{datasetName(run)}</h2><p className="page-description">{runMeta(run)}</p></div>
        <div className="prep-actions"><Badge run={run} /><button className="btn btn-secondary" type="button" onClick={refresh}>Refresh</button></div>
      </div>
      {error && <p className="prep-warning" role="status">Could not refresh this run: {error.text} Retrying automatically. <button className="prep-inline-link" type="button" onClick={refresh}>Try again</button></p>}
      <PreparationRunView run={run} visible={visible} onRefresh={refresh} />
    </> : error?.missing ? <section className="fine-panel"><h2 ref={heading} tabIndex={-1}>Run not found</h2><p>This preparation run does not exist or is no longer available.</p><Link className="btn btn-secondary" to="/fine-tuning">Back to dashboard</Link></section>
      : error ? <div className="prep-load-error" role="alert"><span>Could not load this run: {error.text} Retrying automatically.</span><button className="btn btn-secondary" type="button" onClick={refresh}>Try again</button></div>
      : <p className="prep-copy" role="status">Loading run…</p>}
  </>;
}
