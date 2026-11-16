import { useEffect, useReducer, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { api, ApiError } from '../services/api';
import { preparationPath, preparationSteps as steps, requestMessage as message, retryDelay, sentence, stepLabel, useFineTuningRunner } from '../contexts/FineTuningRunnerContext';
import type { PreparationOptions, TrainingPreviewRecord, TrainingRun } from '../types';
import { FieldLabel } from './ParamTooltips';
import { duration } from './TrainingRun';
import './PreparationRuns.css';

const goals: [PreparationOptions['goal'], string][] = [['question_answer', 'Question answering'], ['classification', 'Classification'], ['numeric_prediction', 'Numeric prediction'], ['text_generation', 'Text generation']];
const goalLabel = (goal?: string) => goals.find(([key]) => key === goal)?.[1] ?? goal ?? '—';

type Display = 'queued' | 'running' | 'recovering' | 'pausing' | 'paused' | 'review' | 'waiting' | 'completed' | 'failed' | 'stopped';
const statuses: Record<Display, { label: string; glyph: string }> = {
  queued: { label: 'Queued', glyph: '○' }, running: { label: 'Running', glyph: '●' }, recovering: { label: 'Recovering', glyph: '↻' },
  pausing: { label: 'Pausing', glyph: '◑' }, paused: { label: 'Paused', glyph: '‖' }, review: { label: 'Waiting for review', glyph: '◐' },
  waiting: { label: 'Waiting', glyph: '◐' }, failed: { label: 'Failed', glyph: '!' }, stopped: { label: 'Stopped', glyph: '■' }, completed: { label: 'Completed', glyph: '✓' },
};
const closed = ['completed', 'failed', 'stopped', 'needs_improvement'];
const active = (run: TrainingRun) => !closed.includes(run.status);
const reviewable = (run: TrainingRun) => run.status === 'waiting' && run.checkpoint?.name === 'preview-review';
function display(run: TrainingRun): Display {
  if (run.status === 'completed' || run.status === 'failed' || run.status === 'stopped') return run.status;
  if (run.status === 'needs_improvement') return 'completed';
  // A paused run waiting at a checkpoint still reports waiting; the control says it is paused.
  if (run.control_action === 'pause') return run.status === 'pause_requested' ? 'pausing' : 'paused';
  if (run.status === 'waiting') return reviewable(run) ? 'review' : 'waiting';
  if (run.status === 'recovering' || run.status === 'running') return run.status;
  return 'queued';
}
// Filters run in SQL before paging, so each tab is a server-side status set.
const filters: [string, string, string[]][] = [
  ['all', 'All runs', []], ['active', 'In progress', ['queued', 'running', 'recovering', 'pause_requested', 'paused', 'waiting']],
  ['review', 'Needs review', ['waiting']], ['finished', 'Closed', ['completed', 'failed', 'stopped']],
];
const runKey = (run: TrainingRun) => `${run.workflow_id}/${run.run_id}`;
const date = (value?: string) => value ? new Date(value).toLocaleString([], { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' }) : '—';
function relative(value?: string) {
  if (!value) return '—';
  const minutes = Math.max(0, Math.round((Date.now() - Date.parse(value)) / 60000));
  const days = Math.round(minutes / 1440);
  return minutes < 1 ? 'Just now' : minutes < 60 ? `${minutes} min ago` : minutes < 1440 ? `${Math.round(minutes / 60)} hr ago` : `${days} ${days === 1 ? 'day' : 'days'} ago`;
}
function version(run: TrainingRun) {
  if (run.prepared_dataset_version) return `Saved as version ${run.prepared_dataset_version}`;
  if (!run.base_version) return active(run) ? 'Adds a new version' : 'Nothing saved';
  return active(run) ? `Builds on version ${run.base_version}` : `Nothing saved · version ${run.base_version} unchanged`;
}
const records = (count: number) => `${count} ${count === 1 ? 'record' : 'records'}`;
const examples = (count: number) => `${count} ${count === 1 ? 'example' : 'examples'}`;
const datasetName = (run: TrainingRun) => run.target_dataset_name ?? run.target_dataset_id ?? 'Preparation run';
// Known failure reasons from the server; other failures are described generically.
const failureReasons: Record<NonNullable<TrainingRun['failure_reason']>, (dataset: string) => string> = {
  'dataset-document-exists': (dataset) => `This document and page range are already in ${dataset}.`,
  'dataset-questions-exist': (dataset) => `Every record repeats a question already in ${dataset}.`,
  'dataset-changed': (dataset) => `${dataset} changed while this run was working: it was edited or removed, or another run saved to it first. Start a new preparation from Preparation runs.`,
  'preparation-no-examples': () => 'The model found nothing in this document it could turn into records.',
};
const failedStep = (run: TrainingRun) => run.stages[0]?.steps.find((step) => step.status === 'failed');
export function Badge({ run }: { run: TrainingRun }) {
  const state = display(run);
  return <span className={`prep-badge prep-badge-${state}`}><span aria-hidden="true">{statuses[state].glyph}</span>{statuses[state].label}</span>;
}
function Progress({ run }: { run: TrainingRun }) {
  const progress = run.progress;
  if (!progress?.total) return null;
  const percent = Math.min(100, Math.floor(progress.completed / progress.total * 100));
  const cap = progress.step === 'generate' ? run.settings?.max_questions : 0;
  return <><span className="prep-sub">{progress.completed} of {progress.total}{cap ? ` · stops as soon as ${records(cap)} are saved, which can be before this count finishes` : ''}</span><div className="prep-track" role="progressbar" aria-label={`${stepLabel(progress.step)} progress`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}><span style={{ width: `${percent}%` }} /></div></>;
}
// StepTiming shows how long the current step has run and the worker's estimate of what is left in it.
function StepTiming({ run }: { run: TrainingRun }) {
  const running = run.status === 'running' && !run.state_stale;
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (!running) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [running]);
  if (!running) return null;
  const started = run.stages[0]?.steps.find((step) => step.name === run.step)?.started_at;
  const elapsed = started ? now - Date.parse(started) : NaN;
  const progress = run.progress?.step === run.step ? run.progress : undefined;
  const remaining = progress?.remaining_seconds && progress.completed < progress.total ? progress.remaining_seconds * 1000 : 0;
  return <div className="fine-job-timing">
    <div><span>Step elapsed</span><strong>{Number.isFinite(elapsed) && elapsed >= 0 ? elapsed < 60000 ? 'Less than a minute' : duration(elapsed) : 'Not started'}</strong></div>
    <div><span>Estimated step remaining</span><strong>{remaining ? `About ${duration(remaining)}` : progress?.total && progress.completed === progress.total ? 'Finishing up…' : 'Estimating as progress arrives…'}</strong></div>
  </div>;
}
function StepSummary({ run }: { run: TrainingRun }) {
  const state = display(run);
  if (state === 'queued') return <><span className="prep-main">Waiting for a worker</span><span className="prep-sub">Not started</span></>;
  if (state === 'review') return <><span className="prep-main">Preview ready</span><span className="prep-sub">Your review is needed</span></>;
  if (state === 'completed') return <><span className="prep-main">Finished</span><span className="prep-sub">{run.dataset_available ? `${records(run.dataset_record_count)} in version ${run.prepared_dataset_version}` : 'No saved version'}</span></>;
  if (state === 'failed') return <><span className="prep-main">Failed during {stepLabel(failedStep(run)?.name ?? run.step).toLowerCase()}</span><span className="prep-sub">See run details</span></>;
  if (state === 'stopped') return <><span className="prep-main">Stopped</span><span className="prep-sub">Cancelled · cannot resume</span></>;
  return <><span className="prep-main">{stepLabel(run.step)}</span>{state === 'paused' ? <span className="prep-sub">Paused · saved progress kept</span> : state === 'recovering' ? <span className="prep-sub">Resuming from saved progress</span> : <Progress run={run} />}</>;
}

export default function PreparationRuns({ visible: workspaceVisible }: { visible: boolean }) {
  const [params, setParams] = useSearchParams();
  // While a preparation is in progress, Prepare Dataset shows it.
  const { preparationRun } = useFineTuningRunner();
  const current = preparationRun && active(preparationRun) && preparationRun.stage !== 'training' ? preparationRun : null;
  const [filter, setFilter] = useState('all');
  const [list, setList] = useState<{ runs: TrainingRun[]; next: string } | null>(null);
  const [listError, setListError] = useState('');
  const [listAttempt, setListAttempt] = useState(0);
  const [loadingMore, setLoadingMore] = useState(false);
  const [detail, setDetail] = useState<TrainingRun | null>(null);
  const [detailError, setDetailError] = useState<{ key: string; text: string; missing: boolean } | null>(null);
  const [detailAttempt, setDetailAttempt] = useState(0);
  const heading = useRef<HTMLHeadingElement>(null);
  const selectedKey = params.get('preparation') && params.get('run') ? `${params.get('preparation')}/${params.get('run')}` : '';
  // Until details load, the table row stands in (without preview or settings).
  const run = detail && runKey(detail) === selectedKey ? detail : list?.runs.find((item) => runKey(item) === selectedKey);
  const missing = detailError?.key === selectedKey && detailError.missing;
  const statusSet = filters.find(([key]) => key === filter)![2];

  function navigate(target?: TrainingRun | 'new') {
    // New preparations, and runs still in progress, use the Prepare Dataset tab.
    if (target === 'new') setParams({ view: 'prepare' });
    else if (target && active(target)) setParams({ view: 'prepare', preparation: target.workflow_id, run: target.run_id, ...(reviewable(target) && { review: '1' }) });
    else setParams(target ? { preparation: target.workflow_id, run: target.run_id } : {});
  }

  // The first page long-polls: the server answers when the filtered page changes.
  useEffect(() => {
    if (!workspaceVisible) return;
    const controller = new AbortController();
    void (async () => {
      let revision = '';
      setListError('');
      while (!controller.signal.aborted) {
        try {
          const page = await api.trainingRuns('', controller.signal, revision, { mode: 'preparation', statuses: statusSet });
          if (controller.signal.aborted) return;
          setListError('');
          if (page.revision !== revision) {
            revision = page.revision;
            setList({ runs: page.runs ?? [], next: page.next_page_token ?? '' });
          }
        } catch (err) {
          if (controller.signal.aborted) return;
          // The server may be restarting; keep showing the last list and retry.
          setListError(message(err, 'Could not load preparation runs.'));
          revision = '';
          await retryDelay(controller.signal);
        }
      }
    })();
    return () => controller.abort();
  }, [filter, workspaceVisible, listAttempt]);

  useEffect(() => {
    if (!selectedKey || !workspaceVisible) return;
    const [workflowID, runID] = [params.get('preparation')!, params.get('run')!];
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const load = async () => {
      try {
        const next = await api.trainingRun(workflowID, runID, controller.signal);
        if (controller.signal.aborted) return;
        setDetail(next);
        setDetailError(null);
        if (active(next)) timer = setTimeout(load, 3000);
      } catch (err) {
        if (controller.signal.aborted) return;
        const missing = err instanceof ApiError && err.status === 404;
        setDetailError({ key: selectedKey, text: message(err, 'Could not load this run.'), missing });
        if (!missing) timer = setTimeout(load, 5000);
      }
    };
    void load();
    return () => { controller.abort(); clearTimeout(timer); };
  }, [selectedKey, workspaceVisible, detailAttempt]);

  // A run still in progress lives in Prepare Dataset; an older link to its details goes there.
  const runActive = !!run && active(run);
  useEffect(() => {
    if (workspaceVisible && run && runActive) setParams({ view: 'prepare', preparation: run.workflow_id, run: run.run_id }, { replace: true });
  }, [workspaceVisible, selectedKey, runActive]);

  useEffect(() => {
    if (workspaceVisible) heading.current?.focus();
  }, [selectedKey, workspaceVisible]);

  function refresh() {
    setListAttempt((attempt) => attempt + 1);
    setDetailAttempt((attempt) => attempt + 1);
  }
  async function loadMore() {
    if (!list?.next) return;
    setLoadingMore(true);
    try {
      const page = await api.trainingRuns(list.next, undefined, '', { mode: 'preparation', statuses: statusSet });
      setList((current) => current && { runs: [...current.runs, ...page.runs.filter((item) => !current.runs.some((known) => runKey(known) === runKey(item)))], next: page.next_page_token ?? '' });
    } catch (err) {
      setListError(message(err, 'Could not load more runs.'));
    } finally {
      setLoadingMore(false);
    }
  }

  const visibleRuns = list?.runs ?? [];
  const actionLabel = (item: TrainingRun) => reviewable(item) ? 'Review preview' : 'View details';

  return <div className="fine-page prep-workspace">
    <div className="page-header fine-page-header"><div><h2>Fine-Tuning</h2></div></div>
    <nav className="tabs fine-tabs" aria-label="Fine-tuning views">
      <Link className="tab active" aria-current="page" to="/fine-tuning">Preparation runs</Link>
      <Link className="tab" to={current ? preparationPath(current) : '/fine-tuning?view=prepare'}>Prepare Dataset</Link>
      <Link className="tab" to="/fine-tuning?view=configure">Configure Training</Link>
      <Link className="tab" to="/fine-tuning?view=history">History</Link>
    </nav>
    {selectedKey && <button className="prep-back" onClick={() => navigate()}>← Preparation runs</button>}
    <div className="page-header-with-action prep-heading">
      <div><h2 ref={heading} tabIndex={-1}>{run ? datasetName(run) : selectedKey ? 'Preparation run' : 'Preparation runs'}</h2><p className="page-description">{run ? runMeta(run) : 'Each row is one run that turns a document into verified records for a dataset. Select a run to see its details.'}</p></div>
      <div className="prep-actions">{run && <Badge run={run} />}<button className="btn btn-secondary" onClick={refresh}>Refresh</button>{!selectedKey && <button className="btn btn-primary" onClick={() => navigate('new')}>+ New preparation</button>}</div>
    </div>
    {!selectedKey && <>
      <div className="tabs prep-filters" role="group" aria-label="Filter runs" aria-describedby="prep-filter-help">{filters.map(([key, label]) => <button key={key} className={`tab ${filter === key ? 'active' : ''}`} aria-pressed={filter === key} onClick={() => { if (key !== filter) { setList(null); setFilter(key); } }}>{label}</button>)}</div>
      <p id="prep-filter-help" className="prep-copy prep-filter-help">In progress includes queued, running, paused, and waiting for review. Closed includes completed, failed, and stopped.</p>
      {listError && <div className="prep-load-error" role="alert"><span>Could not load preparation runs: {listError} Retrying automatically.</span><button className="btn btn-secondary" onClick={() => setListAttempt((attempt) => attempt + 1)}>Try again</button></div>}
      {!list && !listError && <p className="prep-copy" role="status">Loading preparation runs…</p>}
      {list && <div className="catalog-table-wrap prep-table-wrap"><table className="catalog-table prep-table"><caption className="prep-sr-only">Preparation runs</caption><thead><tr>{['Dataset · run', 'Source document', 'Status', 'Step', 'Updated', 'Next action'].map((label) => <th key={label} scope="col">{label}</th>)}</tr></thead><tbody>
        {visibleRuns.map((item) => <tr key={runKey(item)}>
          <td><Link className="prep-run-link" to={active(item) ? preparationPath(item) : `?preparation=${encodeURIComponent(item.workflow_id)}&run=${encodeURIComponent(item.run_id)}`}>{datasetName(item)}</Link><span className="prep-sub prep-run-id">{item.workflow_id}</span><span className="prep-sub">{version(item)}</span></td>
          <td data-label="Source document"><strong className="prep-main">{item.source_document ?? '—'}</strong></td>
          <td data-label="Status"><Badge run={item} />{item.state_stale && active(item) && <span className="prep-sub prep-stale-tag">Last known status</span>}</td>
          <td data-label="Step"><StepSummary run={item} /></td>
          <td data-label="Updated"><span title={date(item.updated_at ?? item.started_at)}>{relative(item.updated_at ?? item.started_at)}</span><span className="prep-sub">Started {date(item.started_at)}</span></td>
          <td className="prep-row-action"><button className={`btn ${reviewable(item) ? 'btn-primary' : 'btn-secondary'}`} aria-label={`${actionLabel(item)} · ${datasetName(item)} · ${item.workflow_id}`} onClick={() => navigate(item)}>{actionLabel(item)}</button></td>
        </tr>)}
        {!visibleRuns.length && <tr><td colSpan={6}><div className="empty-state">{filter === 'all' ? <><h3>No preparation runs yet</h3><p>Start one with New preparation.</p></> : <><h3>No runs match this filter</h3><p>Choose All runs to see every preparation run.</p></>}</div></td></tr>}
      </tbody></table></div>}
      {list?.next && <button className="btn btn-secondary prep-load-more" disabled={loadingMore} onClick={() => void loadMore()}>{loadingMore ? 'Loading…' : 'Load more runs'}</button>}
    </>}
    {selectedKey && current && runKey(current) !== selectedKey && <div className="prep-active-note"><strong>Another run is in progress.</strong> Opening other screens does not cancel it. <button className="prep-inline-link" onClick={() => navigate(current)}>View that run</button></div>}
    {selectedKey && !run && (missing ? <section className="fine-panel"><h3>Run not found</h3><p>This preparation run does not exist or is no longer available.</p><button className="btn btn-secondary" onClick={() => navigate()}>Return to preparation runs</button></section>
      : detailError?.key === selectedKey ? <div className="prep-load-error" role="alert"><span>Could not load this run: {detailError.text}{detailError.missing ? '' : ' Retrying automatically.'}</span><button className="btn btn-secondary" onClick={() => setDetailAttempt((attempt) => attempt + 1)}>Try again</button></div>
      : <p className="prep-copy" role="status">Loading run…</p>)}
    {run && <>
      {detailError?.key === selectedKey && !missing && <p className="prep-warning" role="status">Could not refresh this run: {detailError.text} Retrying automatically. <button className="prep-inline-link" onClick={() => setDetailAttempt((attempt) => attempt + 1)}>Try again</button></p>}
      <PreparationRunView key={selectedKey} run={run} visible={workspaceVisible} onRefresh={refresh} />
    </>}
  </div>;
}

// Shared by both tabs: retries reuse their request IDs, and unsent review edits follow the run.
const controlIDs = new Map<string, string>();
const reviewIDs = new Map<string, string>();
const reviewDrafts = new Map<string, TrainingPreviewRecord[]>();

export const runMeta = (run: TrainingRun) => `${run.source_document ?? 'Source document'} · started ${date(run.started_at)} · ${version(run)}`;

// PreparationRunView shows one preparation run with its controls and preview
// review. Preparation runs and Prepare Dataset both render it.
export function PreparationRunView({ run, visible, onRefresh, heading, openReview, onReviewOpened }: {
  run: TrainingRun; visible: boolean; onRefresh: () => void; heading?: boolean; openReview?: boolean; onReviewOpened?: () => void;
}) {
  const [notice, setNotice] = useState('');
  const [actionError, setActionError] = useState('');
  const [busy, setBusy] = useState('');
  const [confirmStop, setConfirmStop] = useState(false);
  const [review, setReview] = useState(false);
  const [reviewError, setReviewError] = useState('');
  const [, rerender] = useReducer((count: number) => count + 1, 0);
  const dialog = useRef<HTMLDialogElement>(null);
  const reviewTrigger = useRef<HTMLButtonElement>(null);
  const draft = reviewDrafts.get(runKey(run));
  const setDraft = (next?: TrainingPreviewRecord[]) => {
    if (next) reviewDrafts.set(runKey(run), next);
    else reviewDrafts.delete(runKey(run));
    rerender();
  };
  const reviewOpen = review && !!run.preview;
  useEffect(() => {
    if (openReview && run.preview) { setReview(true); onReviewOpened?.(); }
  }, [openReview, !!run.preview]);
  useEffect(() => {
    if (reviewOpen && visible) dialog.current?.showModal();
    else dialog.current?.close();
  }, [reviewOpen, visible]);
  const state = display(run);
  // A request notice is replaced by the run's own state once that changes.
  const shownState = useRef(state);
  useEffect(() => {
    if (shownState.current !== state) setNotice('');
    shownState.current = state;
  }, [state]);
  const paused = run.control_action === 'pause';
  const settings = run.settings;
  const failed = failedStep(run);

  // Retries of an unanswered request reuse its ID so the server can deduplicate it.
  async function act(name: string, request: () => Promise<unknown>, success: string, ids?: Map<string, string>, key?: string) {
    setBusy(name);
    setActionError('');
    setNotice('');
    try {
      await request();
      if (key) ids?.delete(key);
      setNotice(success);
      return true;
    } catch (err) {
      if (key && err instanceof ApiError && err.status >= 400 && err.status < 500) ids?.delete(key);
      setActionError(err instanceof ApiError && err.status === 409 ? `${sentence(err.message)} The run changed; its latest state is shown.` : message(err, 'The request did not finish. Check the run state before trying again.'));
      return false;
    } finally {
      setBusy('');
      onRefresh();
    }
  }
  function control(target: TrainingRun, action: 'pause' | 'resume') {
    const key = `${runKey(target)}:${action}:${target.control_revision}`;
    const id = controlIDs.get(key) ?? `control-${crypto.randomUUID()}`;
    controlIDs.set(key, id);
    const body = { request_id: id, expected_revision: target.control_revision };
    return act(action, () => action === 'pause' ? api.pausePreparation(target.workflow_id, target.run_id, body) : api.continuePreparation(target.workflow_id, target.run_id, body),
      action === 'pause' ? 'Pause requested. The run pauses after it saves its current progress.' : 'Continue requested. The run resumes from its saved progress.', controlIDs, key);
  }
  async function download(target: TrainingRun) {
    if (!target.prepared_dataset_id || !target.prepared_dataset_version) return;
    const id = target.prepared_dataset_id, saved = target.prepared_dataset_version;
    await act('download', async () => {
      const blob = await api.downloadPreparationDataset(id, saved);
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${datasetName(target).replace(/[^a-zA-Z0-9._-]/g, '_')}-v${saved}.jsonl`;
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    }, `Downloaded version ${saved}.`);
  }
  const reviewRecords = draft ?? run.preview?.records ?? [];
  const fieldLabel = (field: 'prompt' | 'target') => settings?.goal === 'question_answer' || !settings ? field === 'prompt' ? 'Question' : 'Answer' : field === 'prompt' ? 'Input' : 'Expected output';
  const validReview = reviewRecords.length > 0 && reviewRecords.every((record) => record.prompt.trim() && record.target.trim());
  function closeReview() {
    setReview(false);
    setReviewError('');
    dialog.current?.close();
    reviewTrigger.current?.focus();
  }
  async function acceptReview() {
    if (!run.checkpoint || !validReview) return;
    const checkpoint = run.checkpoint.id;
    const id = reviewIDs.get(checkpoint) ?? `review-${crypto.randomUUID()}`;
    reviewIDs.set(checkpoint, id);
    setBusy('review');
    setReviewError('');
    try {
      await api.continueTrainingPreview(run.workflow_id, run.run_id, { checkpoint_id: checkpoint, submission_id: id, records: reviewRecords });
      setDraft(undefined);
      closeReview();
      setNotice('Review accepted. Preparation continues; training does not start.');
    } catch (err) {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) reviewIDs.delete(checkpoint);
      setReviewError(message(err, 'The review was not confirmed. Try again; a repeated send is recognized.'));
    } finally {
      setBusy('');
      onRefresh();
    }
  }

  return <>
    {heading && <Link className="prep-back" to="/fine-tuning">← Preparation runs</Link>}
    {heading && <div className="page-header-with-action prep-heading"><div><h3>{datasetName(run)}</h3><p className="page-description">{runMeta(run)}</p></div><div className="prep-actions"><Badge run={run} /><button className="btn btn-secondary" onClick={onRefresh}>Refresh</button></div></div>}
    <div className="prep-workspace prep-detail">
      {notice && <p className="prep-notice" role="status">{notice}</p>}
      {actionError && <p className="prep-field-error prep-action-error" role="alert">{actionError}</p>}
      {run.state_stale && active(run) && <p className="prep-warning" role="status">Showing the last saved status{run.observed_at ? ` from ${date(run.observed_at)}` : ''}. The preparation worker is not reporting right now. Pause, Continue, and Stop requests are kept and take effect when it reconnects.</p>}
      <section className="fine-panel prep-next"><div><span className="fine-eyebrow">{active(run) ? 'Preparation status' : 'Run result'}</span><h3>{state === 'review' ? 'Preview ready for review' : state === 'paused' ? reviewable(run) ? 'Paused at preview review' : `Paused at ${stepLabel(run.step).toLowerCase()}` : state === 'pausing' ? 'Pausing after the current save' : state === 'running' || state === 'recovering' ? stepLabel(run.step) : state === 'queued' ? 'Waiting for a preparation worker' : state === 'completed' ? run.prepared_dataset_version ? `Saved to ${datasetName(run)} as version ${run.prepared_dataset_version}` : 'Finished without a saved version' : state === 'failed' ? `Failed during ${stepLabel(failed?.name ?? run.step).toLowerCase()}` : state === 'stopped' ? 'This run was cancelled' : 'Waiting'}</h3>
        <p className="prep-copy">{state === 'review' ? 'Review and edit your preview before continuing full preparation. Training does not start.' : state === 'paused' ? reviewable(run) ? 'Saved progress is kept. You can still review the preview; after you accept it, the run stays paused until you select Continue.' : 'Saved progress is kept. Continue resumes from it, even after a restart.' : state === 'pausing' ? 'The current unit of work finishes and saves, then the run pauses.' : state === 'recovering' ? 'The worker was lost. The run resumes from its saved progress.' : state === 'running' ? 'This run keeps going while you browse other runs or set up a new one. Leaving this screen never stops it.' : state === 'queued' ? 'The run starts when a worker is free.' : state === 'completed' ? `${run.prepared_dataset_version ? `Version ${run.prepared_dataset_version} has ${records(run.dataset_record_count)}.` : 'No dataset version is available for this run.'}${run.skipped_records ? ` ${records(run.skipped_records)} skipped: the dataset already had ${run.skipped_records === 1 ? 'that question' : 'those questions'}.` : ''} Earlier dataset versions are unchanged.` : state === 'failed' ? run.failure_reason ? `${failureReasons[run.failure_reason](datasetName(run))} Nothing was saved. Earlier dataset versions are unchanged.` : `${failed && failed.attempt > 1 ? `It failed after ${failed.attempt} attempts. ` : ''}Nothing was saved. Earlier dataset versions are unchanged. Check the server logs for the cause.` : 'Nothing from this run was saved. Earlier dataset versions are unchanged.'}</p></div>
        {state === 'completed' && run.prepared_dataset_id && !!run.prepared_dataset_version && <button className="btn btn-secondary" disabled={busy === 'download'} onClick={() => void download(run)}>{busy === 'download' ? 'Downloading…' : `Download version ${run.prepared_dataset_version}`}</button>}
        {active(run) && <div className="prep-stop">
          {paused ? <button className="btn btn-primary" disabled={!!busy} onClick={() => void control(run, 'resume')}>{busy === 'resume' ? 'Sending…' : 'Continue preparation'}</button>
            : <button className="btn btn-secondary" disabled={!!busy} onClick={() => void control(run, 'pause')}>{busy === 'pause' ? 'Sending…' : 'Pause'}</button>}
          <button className="btn btn-secondary" disabled={!!busy} onClick={() => setConfirmStop(true)}>Stop preparation…</button>
          <span className="prep-copy">Pause keeps saved progress. Stop cancels this run; saved versions stay unchanged.</span>
        </div>}
        {confirmStop && active(run) && <div className="prep-confirm" role="group" aria-label="Confirm cancellation"><p><strong>Stop this preparation?</strong> Stop means cancellation, not pause. This run cannot be resumed; saved dataset versions stay unchanged.</p><div className="prep-actions"><button className="btn btn-danger" disabled={!!busy} onClick={() => { setConfirmStop(false); void act('stop', () => api.stopTrainingRun(run.workflow_id, run.run_id), 'Stop requested. The run closes after it cancels.'); }}>Stop run</button><button className="btn btn-secondary" onClick={() => setConfirmStop(false)}>Keep this run</button></div></div>}
      </section>
      {reviewable(run) && <section className="fine-panel prep-review-callout"><div><h3>Review {run.preview?.records.length === 1 ? '1 preview example' : `${run.preview?.records.length ?? ''} preview examples`}</h3><p className="prep-copy">Check each example and its source evidence before building the dataset.{draft ? ' You have unsent edits.' : ''}</p></div><button ref={reviewTrigger} className="btn btn-primary" disabled={!run.preview} onClick={() => setReview(true)}>Review preview</button></section>}
      <section className="fine-panel prep-step-summary" aria-label="Preparation progress">
        <ol>{steps.map(([name, label]) => {
          const step = run.stages[0]?.steps.find((item) => item.name === name);
          const stepState = step?.status === 'completed' ? 'completed' : step?.status === 'failed' ? 'failed' : step?.status === 'running' || step?.status === 'recovering' || (run.step === name && active(run) && state !== 'queued') ? 'running' : 'pending';
          const current = run.step === name && active(run) && state !== 'queued';
          return <li key={name} className={`fine-step-item fine-step-${stepState}`} aria-current={current ? 'step' : undefined}><span className="fine-step-marker" aria-hidden="true">{stepState === 'completed' ? '✓' : stepState === 'failed' ? '!' : steps.findIndex(([key]) => key === name) + 1}</span><div><strong>{label}</strong><span>{current && state === 'review' ? 'Waiting for your review' : current && paused ? 'Paused' : step && step.attempt > 1 && stepState !== 'completed' ? `${stepState} · attempt ${step.attempt}` : stepState}</span></div></li>;
        })}</ol>
        {run.progress?.total ? <div className="prep-detail-progress"><Progress run={run} /><p className="prep-copy">{stepLabel(run.progress.step)} only — not overall completion.</p></div> : null}
        <StepTiming run={run} />
      </section>
      <details className="fine-panel prep-saved-preview"><summary>Preview examples · {run.preview?.records.length ?? 0}{draft ? ' · unsent edits' : ''}</summary>
        {!reviewable(run) && !!run.preview?.records.length && <button ref={reviewTrigger} className="btn btn-secondary" onClick={() => setReview(true)}>View preview</button>}
        <div className="prep-records">{(draft ?? run.preview?.records ?? []).map((record, index, all) => <article className="prep-qa" key={index}><span className="prep-sub">Example {index + 1} of {all.length}</span><strong>{record.prompt}</strong><p>{record.target}</p><details><summary>Source evidence</summary><p>{record.evidence}</p></details></article>)}</div>
        {!run.preview?.records.length && <p className="prep-copy">{run.preview ? 'No preview examples were saved.' : 'The preview is shown once the run is readable.'}</p>}
        {run.preview?.warnings?.map((warning) => <p key={warning} className="prep-warning">{warning}</p>)}
      </details>
      <details className="fine-panel prep-setup"><summary>Run setup and timing</summary>
        {!settings && <p className="prep-copy">Settings are shown while the preparation worker is reachable.</p>}
        <dl className="prep-facts">{([
          ['Dataset', datasetName(run)], ['Base version', run.base_version ? `Version ${run.base_version}` : '—'], ['Saved version', run.prepared_dataset_version ? `Version ${run.prepared_dataset_version}` : 'Nothing saved'], ['Source document', run.source_document ?? '—'],
          ...(settings ? [['Page range', settings.page_start ? `Pages ${settings.page_start}–${settings.page_end}` : 'Entire document'], ['Record limit', settings.max_questions || 'Unlimited'], ['Preparation model', settings.model], ['Goal', goalLabel(settings.goal)], ['Preview examples requested', settings.preview_records ?? '—'], ['Instructions', settings.instructions || 'None']] : []),
          ['Started', date(run.started_at)], ['Updated', date(run.updated_at)], ['Run ID', `${run.workflow_id} · ${run.run_id}`],
        ] as [string, string | number][]).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
      </details>
    </div>
    <dialog ref={dialog} className="prep-review-dialog" onCancel={(event) => { event.preventDefault(); closeReview(); }} aria-labelledby="prep-review-title" onKeyDown={(event) => {
      if (event.key !== 'Tab') return;
      const controls = [...event.currentTarget.querySelectorAll<HTMLElement>('button:not([disabled]), textarea, summary, [tabindex="0"]')].filter((element) => element.getClientRects().length > 0);
      const first = controls[0], last = controls.at(-1);
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    }}>
      {reviewOpen && <div className="prep-review-body">
        <header className="prep-review-header"><div><span className="prep-sub">{datasetName(run)} · {examples(reviewRecords.length)}</span><h3 id="prep-review-title">Review preview examples</h3></div><button autoFocus className="btn btn-secondary" onClick={closeReview}>Close preview</button></header>
        <div className="prep-review-content">
          <p className="prep-copy">{reviewable(run) ? 'Check each example against its source. Edits stay here until accepted; reloading discards them.' : 'Read-only preview. This run is not waiting for review.'}</p>
          <div className="prep-records">{reviewRecords.map((record, index) => <article className="prep-qa" key={index}>
            <h4>Example {index + 1} of {reviewRecords.length}</h4>
            {(['prompt', 'target'] as const).map((field) => <div className="form-group" key={field}>
              <FieldLabel htmlFor={`prep-${field}-${index}`} tooltipKey={field === 'prompt' ? 'fineTuningPrompt' : 'fineTuningTarget'}>{fieldLabel(field)}</FieldLabel>
              <textarea id={`prep-${field}-${index}`} className="form-input" required readOnly={!reviewable(run) || busy === 'review'} aria-invalid={!record[field].trim()} aria-describedby={!record[field].trim() ? `prep-${field}-${index}-error` : undefined} value={record[field]} onChange={(event) => { const next = reviewRecords.map((item) => ({ ...item })); next[index][field] = event.target.value; setDraft(next); }} />
              {!record[field].trim() && <p id={`prep-${field}-${index}-error`} className="prep-field-error">Enter a nonblank {fieldLabel(field).toLowerCase()}.</p>}
            </div>)}
            <details><summary>Source evidence</summary><p>{record.evidence}</p></details>
          </article>)}</div>
        </div>
        <footer className="prep-review-footer">
          <span className={reviewError ? 'prep-field-error' : 'prep-copy'} role="status">{reviewError || (!validReview ? `Complete each ${fieldLabel('prompt').toLowerCase()} and ${fieldLabel('target').toLowerCase()} to continue.` : draft ? 'Unsent edits · closing keeps your changes' : reviewable(run) ? 'Accepting continues preparation, not training.' : 'Read-only examples')}</span>
          {reviewable(run) && <button className="btn btn-primary" disabled={!validReview || busy === 'review'} onClick={() => void acceptReview()}>{busy === 'review' ? 'Sending…' : 'Accept and build dataset'}</button>}
        </footer>
      </div>}
    </dialog>
  </>;
}
