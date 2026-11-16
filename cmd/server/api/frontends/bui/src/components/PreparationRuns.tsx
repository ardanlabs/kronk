import { useEffect, useReducer, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { api, ApiError } from '../services/api';
import { preparationSteps as steps, requestMessage as message, sentence, stepLabel } from '../contexts/FineTuningRunnerContext';
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
export const active = (run: TrainingRun) => !closed.includes(run.status);
export const reviewable = (run: TrainingRun) => run.status === 'waiting' && run.checkpoint?.name === 'preview-review';
function display(run: TrainingRun): Display {
  if (run.status === 'completed' || run.status === 'failed' || run.status === 'stopped') return run.status;
  if (run.status === 'needs_improvement') return 'completed';
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
const examples = (count: number) => `${count} ${count === 1 ? 'example' : 'examples'}`;
export const datasetName = (run: TrainingRun) => run.target_dataset_name ?? run.target_dataset_id ?? 'Preparation run';
// Known failure reasons from the server; other failures are described generically.
const failureReasons: Record<NonNullable<TrainingRun['failure_reason']>, (dataset: string) => string> = {
  'dataset-document-exists': (dataset) => `This document and page range are already in ${dataset}.`,
  'dataset-questions-exist': (dataset) => `Every record repeats a question already in ${dataset}.`,
  'dataset-changed': (dataset) => `${dataset} changed while this run was working: it was edited or removed, or another run saved to it first. Start a new preparation from the dashboard.`,
  'preparation-no-examples': () => 'The model found nothing in this document it could turn into records.',
};
export const failedStep = (run: TrainingRun) => run.stages[0]?.steps.find((step) => step.status === 'failed');
export function Badge({ run }: { run: TrainingRun }) {
  const state = display(run);
  return <span className={`prep-badge prep-badge-${state}`}><span aria-hidden="true">{statuses[state].glyph}</span>{statuses[state].label}</span>;
}
export function Progress({ run }: { run: TrainingRun }) {
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
// Retries reuse their request IDs, and unsent review edits follow the run.
const controlIDs = new Map<string, string>();
const reviewIDs = new Map<string, string>();
const reviewDrafts = new Map<string, TrainingPreviewRecord[]>();

export const runMeta = (run: TrainingRun) => `${run.source_document ?? 'Source document'} · created ${date(run.started_at)} · ${version(run)}`;

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
  const paused = run.control_action === 'pause';
  const ready = state === 'completed' && !!run.prepared_dataset_id && !!run.prepared_dataset_version;
  const eta = remaining(run);
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
    <div className="prep-workspace prep-detail">
      {notice && <p className="prep-notice" role="status">{notice}</p>}
      {actionError && <p className="prep-field-error prep-action-error" role="alert">{actionError}</p>}
      {run.state_stale && active(run) && <p className="prep-warning" role="status">Showing the last saved status{run.observed_at ? ` from ${date(run.observed_at)}` : ''}. The preparation worker is not reporting right now. Pause, Continue, and Stop requests are kept and take effect when it reconnects.</p>}
      <section className={`fine-panel prep-next${ready ? ' dash-done' : ''}`}>{ready && <span className="dash-done-mark" aria-hidden="true">✓</span>}<div><span className="fine-eyebrow">{ready ? 'Dataset ready' : active(run) ? 'Preparation status' : 'Run result'}</span><h3>{state === 'review' ? 'Preview ready for review' : state === 'paused' ? reviewable(run) ? 'Paused at preview review' : `Paused at ${stepLabel(run.step).toLowerCase()}` : state === 'pausing' ? 'Pausing after the current save' : state === 'running' || state === 'recovering' ? stepLabel(run.step) : state === 'queued' ? 'Waiting for a preparation worker' : state === 'completed' ? run.prepared_dataset_version ? `${datasetName(run)} version ${run.prepared_dataset_version} is ready` : 'Finished without a saved version' : state === 'failed' ? `Failed during ${stepLabel(failed?.name ?? run.step).toLowerCase()}` : state === 'stopped' ? 'This run was cancelled' : 'Waiting'}</h3>
        <p className="prep-copy">{state === 'review' ? 'Review and edit your preview before continuing full preparation. Training does not start.' : state === 'paused' ? reviewable(run) ? 'Saved progress is kept. You can still review the preview; after you accept it, the run stays paused until you select Continue.' : 'Saved progress is kept. Continue resumes from it, even after a restart.' : state === 'pausing' ? 'The current unit of work finishes and saves, then the run pauses.' : state === 'recovering' ? 'The worker was lost. The run resumes from its saved progress.' : state === 'running' ? 'This run keeps going while you browse other runs or set up a new one. Leaving this screen never stops it.' : state === 'queued' ? 'The run starts when a worker is free.' : state === 'completed' ? `${run.prepared_dataset_version ? `Version ${run.prepared_dataset_version} has ${records(run.dataset_record_count)}.` : 'No dataset version is available for this run.'}${run.skipped_records ? ` ${records(run.skipped_records)} skipped: the dataset already had ${run.skipped_records === 1 ? 'that question' : 'those questions'}.` : ''} Earlier dataset versions are unchanged.` : state === 'failed' ? run.failure_reason ? `${failureReasons[run.failure_reason](datasetName(run))} Nothing was saved. Earlier dataset versions are unchanged.` : `${failed && failed.attempt > 1 ? `It failed after ${failed.attempt} attempts. ` : ''}Nothing was saved. Earlier dataset versions are unchanged. Check the server logs for the cause.` : 'Nothing from this run was saved. Earlier dataset versions are unchanged.'}</p>
        {eta && <p className="prep-copy"><strong>{eta}.</strong></p>}</div>
        {ready && <div className="dash-done-next">
          <p><strong>Next: train a model with this dataset.</strong> {run.training_ready ? `Configure Training opens with version ${run.prepared_dataset_version} of ${datasetName(run)} selected.` : `Version ${run.prepared_dataset_version} has fewer than 50 records, the training minimum. Add documents to grow it first.`}</p>
          <div className="prep-actions">
            <Link className="btn btn-primary" to={`/fine-tuning?${new URLSearchParams({ view: 'configure', preparation: run.workflow_id, run: run.run_id })}`}>Continue to training →</Link>
            <Link className="btn btn-secondary" to={`/fine-tuning?${new URLSearchParams({ view: 'prepare', dataset: run.prepared_dataset_id! })}`}>Add documents</Link>
            <button className="btn btn-secondary" disabled={busy === 'download'} onClick={() => void download(run)}>{busy === 'download' ? 'Downloading…' : `Download version ${run.prepared_dataset_version}`}</button>
          </div>
        </div>}
        {(state === 'failed' || state === 'stopped') && <div className="prep-actions"><Link className="btn btn-secondary" to="/fine-tuning">Back to dashboard</Link></div>}
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
          ['Date created', date(run.started_at)], ['Updated', date(run.updated_at)], ['Run ID', `${run.workflow_id} · ${run.run_id}`],
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
