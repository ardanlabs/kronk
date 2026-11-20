import { useEffect, useReducer, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { api, ApiError } from '../services/api';
import { preparationSteps as steps, requestMessage as message, sentence, stepLabel, trainingStepLabels } from '../contexts/FineTuningRunnerContext';
import type { PreparationDataset, PreparationOptions, TrainingPreviewRecord, TrainingRun } from '../types';
import { FieldLabel } from './ParamTooltips';
import { DatasetTrainings, duration, setupPath, useDatasetTrainings } from './TrainingRun';
import { failureText, runIssues, RunIssues } from './RunIssues';
import './PreparationRuns.css';

const goals: [PreparationOptions['goal'], string][] = [['question_answer', 'Question answering'], ['classification', 'Classification'], ['numeric_prediction', 'Numeric prediction'], ['text_generation', 'Text generation']];
const goalLabel = (goal?: string) => goals.find(([key]) => key === goal)?.[1] ?? goal ?? '—';

type Display = 'queued' | 'running' | 'recovering' | 'pausing' | 'paused' | 'review' | 'waiting' | 'completed' | 'needs_improvement' | 'failed' | 'stopped';
const statuses: Record<Display, { label: string; glyph: string }> = {
  queued: { label: 'Queued', glyph: '○' }, running: { label: 'Running', glyph: '●' }, recovering: { label: 'Recovering', glyph: '↻' },
  pausing: { label: 'Pausing', glyph: '◑' }, paused: { label: 'Paused', glyph: '‖' }, review: { label: 'Waiting for review', glyph: '◐' },
  waiting: { label: 'Waiting', glyph: '◐' }, failed: { label: 'Failed', glyph: '×' }, stopped: { label: 'Stopped', glyph: '■' }, completed: { label: 'Completed', glyph: '✓' },
  needs_improvement: { label: 'Not accepted', glyph: '' },
};
const closed = ['completed', 'failed', 'stopped', 'needs_improvement'];
export const active = (run: TrainingRun) => !closed.includes(run.status);
export const reviewable = (run: TrainingRun) => run.status === 'waiting' && run.checkpoint?.name === 'preview-review';
function display(run: TrainingRun): Display {
  if (run.status === 'needs_improvement' || (run.status === 'completed' && run.evaluation_result === 'needs_improvement')) return 'needs_improvement';
  if (run.status === 'completed' || run.status === 'failed' || run.status === 'stopped') return run.status;
  // A paused run waiting at a checkpoint still reports waiting; the control says it is paused.
  if (run.control_action === 'pause') return run.status === 'pause_requested' ? 'pausing' : 'paused';
  if (run.status === 'waiting') return reviewable(run) ? 'review' : 'waiting';
  if (run.status === 'recovering' || run.status === 'running') return run.status;
  return 'queued';
}
export const runKey = (run: TrainingRun) => `${run.workflow_id}/${run.run_id}`;
export const date = (value?: string) => value ? new Date(value).toLocaleString([], { month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' }) : '—';
export function version(run: TrainingRun) {
  if (run.prepared_dataset_version) return `Saved as version ${run.prepared_dataset_version}`;
  if (!run.base_version) return active(run) ? 'Adds a new version' : 'Nothing saved';
  return active(run) ? `Builds on version ${run.base_version}` : `Nothing saved · version ${run.base_version} unchanged`;
}
export const records = (count: number) => `${count} ${count === 1 ? 'record' : 'records'}`;
// previewWarning rewrites the developer wording earlier runs saved for left-out document parts.
const previewWarning = (warning: string) => {
  const count = Number(/^(\d+) (?:structural\/resource )?source exclusions(?: recorded)?; (?:see|inspect) source_exclusions$/.exec(warning)?.[1]);
  if (!count) return warning;
  return `${count === 1 ? '1 part of the document was' : `${count} parts of the document were`} left out, such as page headers and footers, tables, pictures, or passages too long to use. Records come from the rest of the text.`;
};
const examples = (count: number) => `${count} ${count === 1 ? 'example' : 'examples'}`;
export const datasetName = (run: TrainingRun) => run.target_dataset_name ?? run.target_dataset_id ?? 'Preparation run';
export const failedStep = (run: TrainingRun) => run.stages.flatMap((stage) => stage.steps).find((step) => step.status === 'failed');
// DetailTabs is the tabbed details panel under a run's card; arrow keys move between tabs.
export function DetailTabs({ tabs, shown, onSelect, children }: { tabs: [string, string][]; shown: string; onSelect: (key: string) => void; children: React.ReactNode }) {
  return <section className="fine-panel run-details" aria-label="Run details">
    <div className="run-details-tabs" role="tablist" aria-label="Run details" onKeyDown={(event) => {
      if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') return;
      const at = tabs.findIndex(([key]) => key === shown);
      const next = tabs[(at + (event.key === 'ArrowRight' ? 1 : tabs.length - 1)) % tabs.length][0];
      onSelect(next);
      document.getElementById(`run-tab-${next}`)?.focus();
    }}>
      {tabs.map(([key, label]) => <button key={key} id={`run-tab-${key}`} className={`run-details-tab${shown === key ? ' active' : ''}`} role="tab" type="button" aria-selected={shown === key} aria-controls="run-tab-panel" tabIndex={shown === key ? 0 : -1} onClick={() => onSelect(key)}>{label}</button>)}
    </div>
    <div id="run-tab-panel" role="tabpanel" aria-labelledby={`run-tab-${shown}`}>{children}</div>
  </section>;
}

export function Badge({ run }: { run: TrainingRun }) {
  const state = display(run);
  return <span className={`prep-badge prep-badge-${state}`}>{statuses[state].glyph && <span aria-hidden="true">{statuses[state].glyph}</span>}{statuses[state].label}</span>;
}
export function Progress({ run }: { run: TrainingRun }) {
  const progress = run.progress;
  const read = pagesRead(run);
  if (read) return <span className="prep-sub">{read} pages read</span>;
  if (!progress?.total) return null;
  const percent = Math.min(100, Math.floor(progress.completed / progress.total * 100));
  const cap = progress.step === 'generate' ? run.settings?.max_questions : 0;
  return <><span className="prep-sub">{progress.completed} of {progress.total}{progress.step === 'extract' ? ' pages read' : ''}{cap ? ` · stops as soon as ${records(cap)} are saved, which can be before this count finishes` : ''}</span><div className="prep-track" role="progressbar" aria-label={`${stepLabel(progress.step)} progress`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}><span style={{ width: `${percent}%` }} /></div></>;
}
export type StepState = 'completed' | 'running' | 'review' | 'paused' | 'failed' | 'stopped' | 'pending';
export const stepStateLabels: Record<StepState, string> = { completed: 'Done', running: 'Running', review: 'Waiting for you', paused: 'Paused', failed: 'Failed', stopped: 'Stopped', pending: 'Not started' };
export const stepGlyphs: Record<StepState, string> = { completed: '✓', running: '●', review: '●', paused: '‖', failed: '×', stopped: '■', pending: '○' };
const clock = (value?: string) => value ? new Date(value).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' }) : '—';
const span = (from?: string, to?: string) => from && to ? duration(Date.parse(to) - Date.parse(from)) : '';
const pages = (start?: number, end?: number) => start ? `${start}–${end}` : 'Entire document';
// timeline gives each preparation step its state from the run's step records and current position.
function timeline(run: TrainingRun, state: Display) {
  const done = (name: string) => run.stages[0]?.steps.find((item) => item.name === name)?.status === 'completed';
  // A closed run's step can be one it already finished; it stopped at the next unfinished one.
  const from = Math.max(0, steps.findIndex(([name]) => name === run.step));
  const at = state === 'failed' || state === 'stopped' ? steps.slice(from).find(([name]) => !done(name))?.[0] ?? run.step : run.step;
  return steps.map(([name, label]) => {
    const step = run.stages[0]?.steps.find((item) => item.name === name);
    const current = at === name && state !== 'queued';
    let stepState: StepState = 'pending';
    if (step?.status === 'completed') stepState = 'completed';
    else if (step?.status === 'failed' || (current && state === 'failed')) stepState = 'failed';
    else if (current && state === 'stopped') stepState = 'stopped';
    else if (current && active(run)) stepState = state === 'review' ? 'review' : state === 'paused' || state === 'pausing' ? 'paused' : 'running';
    return { name, label, state: stepState, step };
  });
}
function stepDetail(run: TrainingRun, item: ReturnType<typeof timeline>[number]) {
  const step = item.step;
  switch (item.state) {
    case 'completed': return step?.closed_at ? `Finished ${clock(step.closed_at)}${step.started_at ? ` · took ${span(step.started_at, step.closed_at)}` : ''}` : 'Done';
    case 'failed': return `Failed at ${clock(step?.closed_at ?? run.closed_at)}${step && step.attempt > 1 ? ` after ${step.attempt} attempts` : ''}`;
    case 'stopped': return `Stopped at ${clock(run.closed_at)} by a cancel request`;
    case 'pending': return 'Not started';
  }
  return step?.started_at ? `Started ${clock(step.started_at)}` : 'Started';
}
// useNow ticks every second while a run is in progress so its timers stay current.
export function useNow(ticking: boolean) {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (!ticking) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [ticking]);
  return now;
}
const stepCopy: Record<string, string> = {
  extract: 'The text is being read out of your document.',
  preview: 'A few example records are being drafted for you to review.',
  generate: 'The model is writing records from your document. Leaving this screen never stops it.',
  verify: 'Each record is being checked against its source. Records that fail are dropped.',
};
const progressUnits: Record<string, string> = { extract: 'pages read', generate: 'sections', verify: 'records checked' };

// RunActivity records only updates observed on this screen, not a durable log.
export function RunActivity({ run, now }: { run: TrainingRun; now: number }) {
  const progress = run.progress?.stage === run.stage && run.progress?.step === run.step ? run.progress : undefined;
  const label = run.stage === 'training' ? trainingStepLabels[run.step ?? ''] ?? 'Training' : stepLabel(run.step);
  const text = run.status === 'running' ? progress?.message || (run.step === 'extract' ? 'Waiting for document extraction results. Page counts update after a conversion batch finishes.' : 'In progress.')
    : statuses[display(run)].label;
  const units = progress?.message === 'Generating additional records to reach the requested count' ? 'records' : progress?.message === 'Generating evaluation questions' ? 'evaluation questions' : progressUnits[progress?.step ?? ''] ?? 'work units completed';
  const counts = progress && progress.completed > 0 && progress.message !== 'Source extraction complete' ? `${progress.completed}${progress.total > 0 ? ` of ${progress.total}` : ''} ${units}` : '';
  const activity = `${run.state_stale ? 'Last known: ' : ''}${label}: ${text}${counts ? ` · ${counts}` : ''}`;
  const stamp = progress?.updated_at || run.updated_at || run.started_at;
  const [entries, setEntries] = useState<{ id: number; text: string; time: string }[]>([]);
  useEffect(() => {
    setEntries((current) => current[0]?.text === activity ? current : [{ id: (current[0]?.id ?? 0) + 1, text: activity, time: stamp }, ...current.slice(0, 29)]);
  }, [activity, stamp]);
  const stage = run.stages.find((item) => item.name === run.stage);
  const step = stage?.steps.find((item) => item.name === run.step);
  const heartbeat = step?.last_heartbeat_at;
  const age = (value: string) => timeLeft(Math.max(0, now - Date.parse(value)));
  return <section className="fine-panel run-activity" aria-label="Run activity">
    <h3>Activity</h3>
    <p className="prep-copy">{active(run) ? 'Checks for updates every 5 seconds while this run is open.' : 'This run has ended.'} Recent updates observed on this screen; not a complete log.</p>
    {active(run) && <p className="run-activity-health">{heartbeat ? `Worker heartbeat ${age(heartbeat)} ago.` : 'No worker heartbeat reported yet.'} {progress?.updated_at ? `Last work update ${age(progress.updated_at)} ago.` : 'No work-update timestamp available.'} A heartbeat reports worker liveness, not completed work.</p>}
    <ol className="run-activity-list" role="log" aria-label="Observed run activity" aria-live="polite" aria-relevant="additions">
      {entries.map((entry) => <li key={entry.id}><time dateTime={entry.time} title={entry.time}>{new Date(entry.time).toLocaleTimeString()}</time><span>{entry.text}</span></li>)}
    </ol>
  </section>;
}

// pagesRead is how many pages extraction has read of a whole document, whose page count is not known until the end.
export const pagesRead = (run: TrainingRun) => run.status === 'running' && run.step === 'extract' && run.progress?.step === 'extract' && !run.progress.total ? run.progress.completed : 0;
// timeLeft writes a span with its units, such as "4 min 16 sec" or "1 hr 5 min".
export function timeLeft(milliseconds: number) {
  const total = Math.max(0, Math.ceil(milliseconds / 1000));
  const h = Math.floor(total / 3600), m = Math.floor(total / 60) % 60, s = total % 60;
  return h ? `${h} hr ${m} min` : m ? `${m} min ${s} sec` : `${s} sec`;
}
// Retries reuse their request IDs, and unsent review edits follow the run.
const controlIDs = new Map<string, string>();
const reviewIDs = new Map<string, string>();
const reviewDrafts = new Map<string, TrainingPreviewRecord[]>();

export const runMeta = (run: TrainingRun) => `${run.source_document ?? 'Source document'} · created ${date(run.started_at)}`;

// remaining is the worker's estimate of the time left in the current step.
export function remaining(run: TrainingRun) {
  const progress = run.progress?.step === run.step ? run.progress : undefined;
  const seconds = run.status === 'running' && !run.state_stale && progress?.remaining_seconds && progress.completed < progress.total ? progress.remaining_seconds : 0;
  return seconds ? `About ${Math.max(1, Math.round(seconds / 60))} min left in this step` : '';
}

// PreparationRunView shows one preparation run with its controls and preview
// review. A finished dataset leads on to training or to adding documents.
export function PreparationRunView({ run, visible, onRefresh }: { run: TrainingRun; visible: boolean; onRefresh: () => void }) {
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
  const paused = active(run) && run.control_action === 'pause';
  const ready = state === 'completed' && !!run.prepared_dataset_id && !!run.prepared_dataset_version;
  const settings = run.settings;
  const now = useNow(active(run) && visible);
  // The worker's estimate counts down here between updates, from when it arrived.
  const live = run.status === 'running' && !run.state_stale && run.progress?.step === run.step ? run.progress : undefined;
  const estimate = live && live.completed < live.total ? live.remaining_seconds ?? 0 : 0;
  const [estimateAt, setEstimateAt] = useState(Date.now);
  useEffect(() => setEstimateAt(Date.now()), [estimate]);
  // The training page's back link opens the Training runs tab.
  const [params] = useSearchParams();
  const [detailTab, setDetailTab] = useState(params.get('details') ?? '');
  const previousStatus = useRef(run.status);
  useEffect(() => {
    if (previousStatus.current !== run.status && run.status === 'failed') setDetailTab('issues');
    previousStatus.current = run.status;
  }, [run.status]);
  const [dataset, setDataset] = useState<PreparationDataset | null>(null);
  const [datasetError, setDatasetError] = useState('');
  const datasetID = run.prepared_dataset_id ?? run.target_dataset_id;
  const datasetVersion = run.prepared_dataset_version ?? run.base_version ?? 0;
  const trainings = useDatasetTrainings(datasetID, datasetVersion, visible);

  // The opened run owns the snapshot shown here, not later additions to the dataset.
  useEffect(() => {
    if (!visible || !datasetID) return;
    const controller = new AbortController();
    setDataset(null);
    setDatasetError('');
    api.preparationDataset(datasetID, controller.signal, datasetVersion).then((next) => {
      if (controller.signal.aborted) return;
      if (datasetVersion && next.version !== datasetVersion) { setDatasetError('Could not load this run’s dataset version.'); return; }
      setDataset(next);
    }, (err) => {
      if (!controller.signal.aborted) setDatasetError(message(err, 'Could not load this dataset.'));
    });
    return () => controller.abort();
  }, [visible, datasetID, run.status, datasetVersion]);

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
  const reviewRecords = draft ?? run.preview?.records ?? [];
  const fieldLabel = (field: 'prompt' | 'target') => settings?.goal === 'question_answer' || !settings ? field === 'prompt' ? 'Question' : 'Answer' : field === 'prompt' ? 'Input' : 'Expected output';
  const validReview = reviewRecords.length > 0 && reviewRecords.every((record) => record.prompt.trim() && record.target.trim());
  function closeReview() {
    setReview(false);
    setReviewError('');
    dialog.current?.close();
    (reviewTrigger.current ?? document.getElementById('prep-view-preview'))?.focus();
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

  const tl = timeline(run, state);
  const stoppedAt = tl.find((item) => item.state === 'failed' || item.state === 'stopped')?.name;
  const index = Math.max(0, steps.findIndex(([name]) => name === (stoppedAt ?? run.step))) + 1;
  const stepProgress = state === 'running' && run.progress?.total && run.progress.step === run.step ? run.progress : undefined;
  const percent = stepProgress ? Math.min(100, Math.floor(stepProgress.completed / stepProgress.total * 100)) : null;
  const cap = stepProgress?.step === 'generate' ? settings?.max_questions : 0;
  const finishedRun = state === 'failed' || state === 'stopped';
  const left = estimate ? estimate * 1000 - Math.max(0, now - estimateAt) : 0;
  const read = state === 'running' ? pagesRead(run) : 0;
  const timing = state !== 'running' ? '' : read ? `${read} pages read` : left > 0 ? `About ${timeLeft(left)} left` : estimate || (stepProgress && stepProgress.completed >= stepProgress.total) ? 'Finishing up…' : 'Estimating time left…';
  const sincePhrase = (value?: string) => value ? duration(now - Date.parse(value)) : '—';
  const eyebrow = state === 'queued' ? 'Queued' : state === 'failed' ? `Failed · step ${index} of 4` : state === 'stopped' ? `Stopped · step ${index} of 4`
    : state === 'review' ? `Step ${index} of 4 · waiting for you` : state === 'paused' ? `Paused · step ${index} of 4` : state === 'pausing' ? `Pausing · step ${index} of 4` : `Step ${index} of 4`;
  const title = state === 'queued' ? 'Waiting for a worker' : state === 'review' || (state === 'paused' && reviewable(run)) ? 'Review the preview' : state === 'waiting' ? 'Waiting' : stepLabel(stoppedAt ?? run.step);
  const copy = state === 'queued' ? 'The run starts on its own when a preparation worker is free.'
    : state === 'review' ? 'The preview is ready. Check and edit the examples, then accept them to build the full dataset. Nothing else happens until you do.'
    : state === 'paused' ? reviewable(run) ? 'Saved progress is kept. You can still review the preview; after you accept it, the run stays paused until you select Continue.' : 'Saved progress is kept. Continue picks up where it stopped, even after a restart.'
    : state === 'pausing' ? 'The current unit of work finishes and saves, then the run pauses.'
    : state === 'recovering' ? 'The worker was lost. The run resumes from its saved progress.'
    : state === 'failed' ? `${failureText(run)} See issues for details.`
    : state === 'stopped' ? 'Stopped by you. This run cannot be resumed.'
    : state === 'waiting' ? 'The run is waiting at a checkpoint.' : stepCopy[run.step ?? ''] ?? 'This run keeps going while you browse. Leaving this screen never stops it.';
  const ring = state === 'review' ? 'Your turn' : state === 'queued' ? 'Queued' : state === 'paused' ? 'Paused' : state === 'pausing' ? 'Pausing' : state === 'recovering' ? 'Recovering' : state === 'failed' ? '×' : state === 'stopped' ? '■' : 'Working';
  const nextVersion = run.base_version ? `Will add version ${run.base_version + 1}` : 'Will create version 1';
  const facts = active(run)
    ? [state === 'queued' ? `Queued ${sincePhrase(run.started_at)} ago` : paused ? `Started ${sincePhrase(run.started_at)} ago` : `Running for ${sincePhrase(run.started_at)}`, nextVersion, 'Records are saved after the last step']
    : [`Ran for ${span(run.started_at, run.closed_at) || '—'}`, run.base_version ? `Version ${run.base_version} unchanged` : 'No version created', 'No records saved'];

  // The dataset's documents, with this run's document marked as added or being added.
  const runPages = (doc: { name: string; page_start: number; page_end: number }) => doc.name === run.source_document && (!settings || ((doc.page_start || 0) === (settings.page_start || 0) && (doc.page_end || 0) === (settings.page_end || 0)));
  const added = ready ? dataset?.documents.find(runPages) : undefined;
  const docRows = [...(dataset?.documents ?? []).map((doc) => ({ ...doc, note: doc === added ? 'added' : '' })),
    ...(active(run) && run.source_document ? [{ id: 'this-run', name: run.source_document, record_count: -1, page_start: settings?.page_start ?? 0, page_end: settings?.page_end ?? 0, note: 'adding' }] : [])];
  const issues = runIssues(run);
  const openIssues = () => { setDetailTab('issues'); document.getElementById('run-tab-issues')?.focus(); };
  const tabs: [string, string][] = [
    ...(issues.length ? [['issues', 'Issues'] as [string, string]] : []),
    ['dataset', dataset ? `Dataset (${docRows.length} ${docRows.length === 1 ? 'document' : 'documents'})` : 'Dataset'],
    ...(active(run) ? [] : [['steps', 'Step history'] as [string, string]]),
    ['setup', 'Details'],
    ...(datasetID ? [['trainings', `Training runs${trainings.runs.length ? ` (${trainings.runs.length})` : ''}`] as [string, string]] : []),
  ];
  const shownTab = tabs.some(([key]) => key === detailTab) ? detailTab : state === 'failed' ? 'issues' : finishedRun ? 'steps' : 'dataset';
  const reviewButton = (primary: boolean) => <button ref={reviewTrigger} className={`btn ${primary ? 'btn-primary hero-action' : 'btn-secondary'}`} disabled={!run.preview} onClick={() => setReview(true)}>Review preview</button>;
  // A run that saved nothing can still lead to training when its dataset's saved versions are trainable.
  const trainable = finishedRun && !!dataset?.version && dataset.training_ready;
  // The run's own flag was saved when it finished; the dataset reports the current rule for the same version.
  const versionReady = dataset && dataset.version === run.prepared_dataset_version ? dataset.training_ready : run.training_ready;
  // Training stays with the opened run's snapshot.
  const trainVersion = datasetVersion;
  const trainPath = datasetID && trainVersion ? setupPath(datasetID, trainVersion, { preparation: run.workflow_id, run: run.run_id }) : '/fine-tuning';
  const evaluation = dataset ? Math.min(dataset.validation_questions, dataset.validation_needed) + Math.min(dataset.test_questions, dataset.test_needed) : 0;
  const evaluationNeeded = dataset ? dataset.validation_needed + dataset.test_needed : 0;
  const continueButton = <button className="btn btn-primary hero-action" disabled={!!busy} onClick={() => void control(run, 'resume')}>{busy === 'resume' ? 'Sending…' : 'Continue preparation'}</button>;

  return <>
    <div className="prep-detail prep-run-detail">
      {notice && <p className="prep-notice" role="status">{notice}</p>}
      {actionError && <p className="prep-field-error prep-action-error" role="alert">{actionError}</p>}
      {run.state_stale && active(run) && <p className="prep-warning" role="status">Showing the last saved status{run.observed_at ? ` from ${date(run.observed_at)}` : ''}. The preparation worker is not reporting right now. Pause, Continue, and Stop requests are kept and take effect when it reconnects.</p>}
      {state === 'completed' ? <section className="fine-panel done-card" aria-labelledby="run-card-title">
        <div className="prep-done-header">
          <div className="done-top"><span className="dash-done-mark" aria-hidden="true">✓</span><div>
            <span className="fine-eyebrow">Completed</span>
            <h3 id="run-card-title" tabIndex={-1}>{ready ? `${datasetName(run)} version ${run.prepared_dataset_version}` : 'Finished without a saved version'}</h3>
            <p className="prep-copy">Finished {date(run.closed_at)}{run.closed_at ? ` · took ${span(run.started_at, run.closed_at)}` : ''} · earlier versions are unchanged.</p>
            {issues.length > 0 && <button className="prep-inline-link run-issues-link" type="button" onClick={openIssues}>View issues</button>}
          </div></div>
          {datasetID && <button className="btn btn-secondary" type="button" onClick={() => { setDetailTab('trainings'); document.getElementById('run-tab-trainings')?.focus(); }}>Training runs</button>}
        </div>
        {ready && <dl className="done-inline-stats">
          <div><dt>Records in version</dt><dd>{run.dataset_record_count}<small>{added ? `${added.record_count} added by this run` : 'across all documents'}</small></dd></div>
          <div><dt>Skipped as duplicates</dt><dd>{run.skipped_records ?? 0}<small>already in the dataset</small></dd></div>
          <div><dt>Version</dt><dd>{run.prepared_dataset_version}<small>{run.base_version ? `was ${run.base_version}` : 'new dataset'}</small></dd></div>
          <div><dt>Ready to train</dt><dd>{versionReady ? 'Yes' : 'Not yet'}<small>{versionReady ? 'has the 20 training records it needs' : 'needs 20 training records'}</small></dd></div>
        </dl>}
      </section> : <section className={`fine-panel live-hero live-hero-${state}`} aria-labelledby="run-card-title">
        <div className="live-gauge">
          <div className={`live-ring${percent === null ? state === 'running' || state === 'recovering' ? ' live-ring-spin' : ' live-ring-idle' : ''}`} style={{ '--pct': percent ?? 0 } as React.CSSProperties} aria-hidden="true">
            {percent !== null ? <><span className="ring-num">{percent}%</span><span className="ring-text">of this step</span></> : <span className="ring-text">{ring}</span>}
          </div>
          {timing && <p className="live-time">{timing}</p>}
        </div>
        <div className="live-main">
          <span className="fine-eyebrow">{eyebrow}</span>
          <h3 id="run-card-title" tabIndex={-1}>{title}</h3>
          <p className="prep-copy">{copy}</p>
          {state !== 'failed' && issues.length > 0 && <button className="prep-inline-link run-issues-link" type="button" onClick={openIssues}>View issues</button>}
          {trainable && <p className="prep-copy"><strong>{dataset.name} version {dataset.version} has {records(dataset.record_count)} and can still be used for training.</strong></p>}
          {stepProgress && <p className="live-count"><strong>{stepProgress.completed} of {stepProgress.total}</strong> {progressUnits[stepProgress.step] ?? ''}{cap ? ` · stops once ${records(cap)} are saved` : ''}</p>}
          {reviewable(run) && draft && <p className="prep-copy">You have unsent edits.</p>}
          {(reviewable(run) || paused || (finishedRun && trainable)) && <div className="prep-actions">
            {paused ? <>{continueButton}{reviewable(run) && reviewButton(false)}</> : reviewable(run) ? reviewButton(true)
              : <Link className="btn btn-primary run-train" to={trainPath}>Continue to training →</Link>}
          </div>}
        </div>
        <div className="live-steps"><ol className="step-chips" aria-label="Steps">{tl.map((item) => <li key={item.name} className={`step-chip step-chip-${item.state}`} aria-current={['running', 'review', 'paused'].includes(item.state) ? 'step' : undefined}><span aria-hidden="true">{stepGlyphs[item.state]}</span>{item.label}<span className="prep-sr-only">: {stepStateLabels[item.state].toLowerCase()}</span></li>)}</ol></div>
        <div className="live-foot">
          <p>{facts.join(' · ')}</p>
          {active(run) && <div className="live-controls">
            {!paused && <button className="btn btn-secondary btn-compact" disabled={!!busy} title="Pause after the current save; progress is kept" onClick={() => void control(run, 'pause')}><span aria-hidden="true">‖</span>{busy === 'pause' ? 'Sending…' : 'Pause'}</button>}
            <button className="btn btn-secondary btn-compact btn-quiet-danger" disabled={!!busy} title="Cancel this run; it cannot be resumed" onClick={() => setConfirmStop(true)}><span aria-hidden="true">■</span>Stop preparation…</button>
          </div>}
        </div>
        {confirmStop && active(run) && <div className="prep-confirm live-confirm" role="group" aria-label="Confirm cancellation"><p><strong>Stop this preparation?</strong> Stop means cancellation, not pause. This run cannot be resumed; saved dataset versions stay unchanged.</p><div className="prep-actions"><button className="btn btn-danger" disabled={!!busy} onClick={() => { setConfirmStop(false); void act('stop', () => api.stopTrainingRun(run.workflow_id, run.run_id), 'Stop requested. The run closes after it cancels.'); }}>Stop run</button><button className="btn btn-secondary" onClick={() => setConfirmStop(false)}>Keep this run</button></div></div>}
      </section>}
      <RunActivity key={runKey(run)} run={run} now={now} />
      <DetailTabs tabs={tabs} shown={shownTab} onSelect={setDetailTab}>
          {shownTab === 'issues' && <RunIssues key={runKey(run)} run={run} issues={issues} />}
          {shownTab === 'dataset' && (!datasetID ? <p className="prep-copy">This run has no dataset.</p> : datasetError && !dataset ? <p className="prep-warning" role="alert">Could not load the dataset: {datasetError}</p> : !dataset ? <p className="prep-copy" role="status">Loading the dataset…</p> : <>
            <div className="ds-summary">
              <div><span className="fine-eyebrow">Dataset</span><h4>{dataset.name}</h4><p className="prep-copy">{dataset.version ? `Version ${dataset.version} · ${records(dataset.record_count)}` : 'No saved version yet'}{active(run) ? ` · this run ${run.base_version ? `will add version ${run.base_version + 1}` : 'will create version 1'}` : ''}</p></div>
              <dl className="ds-stats">
                <div><dt>Documents</dt><dd>{dataset.documents.length}</dd></div>
                <div><dt>Training records</dt><dd>{dataset.training_records}</dd></div>
                <div><dt>Evaluation questions</dt><dd>{evaluationNeeded ? `${evaluation} of ${evaluationNeeded}` : '—'}</dd></div>
                <div><dt>Versions</dt><dd>{dataset.versions.length || '—'}</dd></div>
                <div><dt>Training</dt><dd className={dataset.training_ready ? 'ds-ok' : ''}>{dataset.training_ready ? '✓ Ready' : 'Not ready'}{dataset.training_ready && evaluationNeeded > 0 && evaluation < evaluationNeeded && <small>without evaluation</small>}</dd></div>
              </dl>
            </div>
            <div className="ds-documents-heading">
              <h4>Source documents</h4>
              <Link className="btn btn-secondary" to={`/fine-tuning?${new URLSearchParams({ view: 'prepare', dataset: datasetID })}`}>Add documents</Link>
            </div>
            <div className="catalog-table-wrap prep-table-wrap"><table className="catalog-table prep-table ds-table">
              <caption className="prep-sr-only">Documents in {dataset.name}</caption>
              <thead><tr><th scope="col">Source document</th><th scope="col">Pages</th><th scope="col">Records</th><th scope="col"><span className="prep-sr-only">Note</span></th></tr></thead>
              <tbody>
                {docRows.map((doc) => <tr key={doc.id} className={doc.note ? 'ds-this-run' : undefined}>
                  <td><span className="prep-main">{doc.name}</span></td>
                  <td data-label="Pages">{pages(doc.page_start, doc.page_end)}</td>
                  <td data-label="Records">{doc.record_count < 0 ? '—' : doc.record_count}</td>
                  <td data-label={doc.note ? 'Note' : undefined}>{doc.note === 'added' ? <span className="ds-tag">Added by this run</span> : doc.note === 'adding' ? <span className="ds-tag ds-tag-pending">Being added by this run</span> : null}</td>
                </tr>)}
                {!docRows.length && <tr><td colSpan={4}><div className="empty-state"><h3>No documents yet</h3><p>Add documents to build this dataset.</p></div></td></tr>}
              </tbody>
            </table></div>
            <p className="prep-copy">Each document’s records stay in every later version unless you remove the document. Removing a document creates a new version; older versions are kept.</p>
          </>)}
          {shownTab === 'trainings' && <DatasetTrainings runs={trainings.runs} error={trainings.error} loading={trainings.loading} trainPath={trainPath} trainingReady={!!trainVersion && !!(ready ? versionReady : dataset?.training_ready)} />}
          {shownTab === 'steps' && <ol className="run-timeline">{tl.map((item, at) => <li key={item.name} className={`run-step run-step-${item.state} fine-step-${item.state === 'stopped' ? 'failed' : item.state === 'review' || item.state === 'paused' ? 'running' : item.state}`}>
            <span className="fine-step-marker" aria-hidden="true">{item.state === 'pending' ? at + 1 : stepGlyphs[item.state]}</span>
            <div className="run-step-body"><div className="run-step-title"><strong>{item.label}</strong><span className="run-step-state">{stepStateLabels[item.state]}</span></div><span className="prep-sub">{stepDetail(run, item)}</span></div>
          </li>)}</ol>}
          {shownTab === 'setup' && <>
            {!settings && <p className="prep-copy">Settings are shown while the preparation worker is reachable.</p>}
            <dl className="prep-facts">{([
              ['Dataset', datasetName(run)], ['Base version', run.base_version ? `Version ${run.base_version}` : '—'], ['Saved version', run.prepared_dataset_version ? `Version ${run.prepared_dataset_version}` : 'Nothing saved'], ['Source document', run.source_document ?? '—'],
              ...(settings ? [['Page range', settings.page_start ? `Pages ${settings.page_start}–${settings.page_end}` : 'Entire document'], ['Record limit', settings.max_questions || 'Unlimited'], ['Preparation model', settings.model], ['Goal', goalLabel(settings.goal)], ['Instructions', settings.instructions || 'None']] : []),
              ...(run.preview?.records.length ? [['Preview examples', <button id="prep-view-preview" className="prep-inline-link" type="button" onClick={() => setReview(true)}>View {examples(run.preview.records.length)}</button>]] : []),
              ['Date created', date(run.started_at)], ['Updated', date(run.updated_at)], ['Run ID', `${run.workflow_id} · ${run.run_id}`],
            ] as [string, React.ReactNode][]).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
          </>}
      </DetailTabs>
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
          {run.preview?.warnings?.map((warning) => <p key={warning} className="prep-warning">{previewWarning(warning)}</p>)}
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
