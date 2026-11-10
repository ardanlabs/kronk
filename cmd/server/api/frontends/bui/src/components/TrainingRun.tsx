import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { api } from '../services/api';
import { useFineTuningRunner } from '../contexts/FineTuningRunnerContext';
import type { TrainingEvaluation, TrainingPreviewRecord, TrainingRun as Run, TrainingStage, TrainingStep } from '../types';
import { FieldLabel } from './ParamTooltips';
import { PreparedDatasetResult } from './PreparationDatasetHistory';

const previewDrafts = new Map<string, TrainingPreviewRecord[]>();

function formatDate(value?: string): string {
  return !value || value.startsWith('0001-') ? '—' : new Date(value).toLocaleString();
}

export default function TrainingRun() {
  const [params, setParams] = useSearchParams();
  const workflowID = params.get('workflow_id') ?? '';
  const runID = params.get('run_id') ?? '';
  const [workflowInput, setWorkflowInput] = useState(workflowID);
  const [runInput, setRunInput] = useState(runID);
  const [run, setRun] = useState<Run | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [refresh, setRefresh] = useState(0);

  useEffect(() => {
    setWorkflowInput(workflowID);
    setRunInput(runID);
    setRun(null);
    setError('');
    setLoading(false);
    if (!workflowID || !runID) return;

    const controller = new AbortController();
    setLoading(true);
    api.trainingRun(workflowID, runID, controller.signal)
      .then((result) => { if (!controller.signal.aborted) setRun(result); })
      .catch((err) => {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'Could not load training job.');
      })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [workflowID, runID, refresh]);

  function openJob(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!workflowInput.trim() || !runInput.trim()) return;
    if (workflowInput.trim() === workflowID && runInput.trim() === runID) {
      setRefresh((value) => value + 1);
      return;
    }
    setParams({ workflow_id: workflowInput.trim(), run_id: runInput.trim() });
  }

  return (
    <div className="fine-page training-job-page">
      <div className="page-header fine-page-header">
        <div>
          <span className="fine-eyebrow">Fine-tuning · Job state</span>
          <h2>Training Job</h2>
          <p>View the job, its stages, and each step as reported by the training service.</p>
        </div>
        <Link className="btn btn-secondary" to="/fine-tuning">Back to Fine-Tuning</Link>
      </div>

      <section className="fine-panel" aria-label="Find training job">
        <form onSubmit={openJob}>
          <div className="fine-form-grid">
            <div className="form-group">
              <FieldLabel htmlFor="training-workflow-id" tooltipKey="trainingWorkflowID">Workflow ID</FieldLabel>
              <input id="training-workflow-id" className="form-input" required value={workflowInput} onChange={(event) => setWorkflowInput(event.target.value)} />
            </div>
            <div className="form-group">
              <FieldLabel htmlFor="training-run-id" tooltipKey="trainingRunID">Run ID</FieldLabel>
              <input id="training-run-id" className="form-input" required value={runInput} onChange={(event) => setRunInput(event.target.value)} />
            </div>
          </div>
          <div className="fine-actions">
            <button className="btn btn-primary" type="submit">View Job</button>
            <span className="fine-action-note">Use the two IDs returned when the job was submitted.</span>
          </div>
        </form>
      </section>

      {(!workflowID || !runID) && <p role="status">Enter both IDs to view a training job.</p>}
      {loading && <p role="status">Loading training job…</p>}
      {error && <div className="fine-error" role="alert">Could not load training job: {error} Check the IDs and that the training service is enabled, then select View Job to retry.</div>}

      {run && <TrainingJobState run={run} onRefresh={() => setRefresh((value) => value + 1)} />}
    </div>
  );
}

export function TrainingJobState({ run, onRefresh, view = 'all' }: { run: Run; onRefresh: () => void; view?: 'all' | 'preparation' | 'training' }) {
  const { stopSubmission, stopRun } = useFineTuningRunner();
  const stop = stopSubmission(run);
  const terminal = ['completed', 'needs_improvement', 'failed', 'stopped'].includes(run.status);
  const controlStage = run.stage || run.stages[0]?.name;
  const canStop = !terminal && (view === 'all' || controlStage === view);
  const stopping = stop?.status === 'sending' || stop?.status === 'requested';
  const stopLabel = controlStage === 'preparation' ? 'Preparation' : 'Training';
  const stages = run.stages.filter((stage) => view === 'all' || stage.name === view);
  const currentStage = stages.find((stage) => stage.name === run.stage)
    ?? stages.find((stage) => stage.status !== 'completed') ?? stages.at(-1);
  return (
    <section className="fine-job-view" aria-label={view === 'preparation' ? 'Preparation job state' : 'Training job state'}>
      <div className="fine-run-heading">
        <div><span className="fine-eyebrow">{view === 'preparation' ? 'Document preparation' : 'Model training'}</span><h3>{view === 'preparation' ? 'Preparation progress' : 'Training progress'}</h3></div>
        <div className="fine-actions">
          <span className="fine-status">{run.status}</span>
          <button className="btn btn-secondary" onClick={onRefresh}>Refresh</button>
          {canStop && <button className="btn btn-danger" type="button" disabled={stopping} onClick={() => {
            if (window.confirm(`Stop ${stopLabel}? This requests cancellation of this job. Existing saved dataset versions are unchanged. A stopped job cannot be resumed from this screen.`)) void stopRun(run);
          }}>{stop?.status === 'sending' ? 'Requesting cancellation…' : stop?.status === 'requested' ? 'Cancellation requested' : `Stop ${stopLabel}`}</button>}
        </div>
      </div>
      {!terminal && stopping && <p role="status" className="fine-action-note">{stop.status === 'sending' ? 'Requesting cancellation for this job…' : 'Cancellation requested. The job may still be running; progress is preserved until the service reports a terminal state.'}</p>}
      {!terminal && stop?.error && <p role="alert" className="fine-error">Could not confirm cancellation: {stop.error} The job may still be running. Retry Stop {stopLabel} to request cancellation for this job.</p>}
      {run.state_stale && <p role="status" className="fine-action-note">This is the last reported state and may be out of date. Refresh to request the latest state.</p>}
      {run.status === 'queued' && !run.stage && <p role="status" className="fine-action-note">Your job was submitted and is waiting for a training worker. This server runs one reserved job at a time, including Preparation. It will start when the worker becomes available; Stop cancels this queued request.</p>}
      {currentStage ? <JobProgress key={`${run.workflow_id}/${run.run_id}/${currentStage.name}`} run={run} stage={currentStage} /> : <p role="status">No steps reported yet.</p>}
      {run.prepared_dataset_id
        ? <PreparedDatasetResult key={`${run.prepared_dataset_id}:${run.prepared_dataset_version}`} id={run.prepared_dataset_id} version={run.prepared_dataset_version} />
        : <ArtifactDownload key={JSON.stringify([run.workflow_id, run.run_id, run.dataset_available, run.state_stale])} run={run} artifact="dataset" />}
      {view !== 'training' && run.preview && <PreviewReview key={JSON.stringify([run.workflow_id, run.run_id, run.checkpoint?.id])} run={run} savedRecords={run.preview.records} onRefresh={onRefresh} />}
      {view !== 'preparation' && <StartPreparedTraining run={run} onRefresh={onRefresh} />}
      {stages.filter((stage) => stage !== currentStage).map((stage) => <section className="fine-panel" key={stage.name}><StepStates caption={`${stage.name} steps`} steps={stage.steps} /></section>)}
      <details className="fine-panel fine-job-details">
        <summary>Job details</summary>
        <div className="fine-run-facts">
          <span><strong>Workflow ID</strong>{run.workflow_id}</span>
          <span><strong>Run ID</strong>{run.run_id}</span>
          <span><strong>Started</strong>{formatDate(run.started_at)}</span>
          <span><strong>Updated</strong>{formatDate(run.updated_at)}</span>
          {run.closed_at && <span><strong>Closed</strong>{formatDate(run.closed_at)}</span>}
        </div>
      </details>
      {view !== 'preparation' && <>
        <EvaluationResults key={JSON.stringify([run.workflow_id, run.run_id, run.evaluation_available])} run={run} />
        <ArtifactDownload key={JSON.stringify([run.workflow_id, run.run_id, run.adapter_available, run.state_stale])} run={run} artifact="adapter" />
      </>}
    </section>
  );
}

const stepLabels: Record<string, string> = {
  extract: 'Extract document', preview: 'Generate preview', generate: 'Generate examples', verify: 'Verify examples',
  split: 'Split dataset', train: 'Train adapter', evaluate: 'Evaluate adapter', publish: 'Publish adapter',
};

function duration(milliseconds: number): string {
  const minutes = Math.max(1, Math.ceil(milliseconds / 60000));
  return minutes < 60 ? `${minutes} min` : `${Math.floor(minutes / 60)} hr ${minutes % 60} min`;
}

function JobProgress({ run, stage }: { run: Run; stage: TrainingStage }) {
  const [now, setNow] = useState(Date.now);
  const step = stage.steps.find((item) => item.name === run.step && stage.name === run.stage)
    ?? stage.steps.find((item) => item.status === 'failed' || item.status === 'running');
  const complete = stage.status === 'completed';
  const waiting = run.status === 'waiting' && run.stage === stage.name;
  const running = run.status === 'running' && stage.status === 'running' && !run.state_stale;
  const progress = !complete && run.progress?.stage === stage.name && run.progress.step === step?.name ? run.progress : undefined;
  useEffect(() => {
    if (!running || waiting) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [running, waiting]);

  const percentage = progress && progress.total > 0 ? Math.max(0, Math.min(100, Math.floor(progress.completed / progress.total * 100))) : undefined;
  const elapsedStart = step?.started_at;
  const elapsedEnd = running ? now : Date.parse(step?.closed_at ?? run.updated_at ?? '');
  const elapsed = elapsedStart ? elapsedEnd - Date.parse(elapsedStart) : NaN;
  const remaining = running && !waiting && progress?.remaining_seconds && progress.completed < progress.total ? progress.remaining_seconds * 1000 : undefined;
  const title = complete ? `${stage.name === 'preparation' ? 'Preparation' : 'Training'} complete`
    : waiting ? run.checkpoint?.name === 'preview-review' ? 'Preview ready for review' : 'Ready to start training'
      : step ? stepLabels[step.name] ?? step.name : `Waiting to start ${stage.name}`;

  return (
    <section className="fine-panel fine-job-progress" aria-label={`${stage.name} progress`}>
      <div className="fine-panel-heading">
        <div><span>{complete ? 'Finished' : waiting ? 'Your next action' : 'Current step'}</span><h3>{title}</h3></div>
        <span className={`fine-status fine-status-${stage.status}`}>{stage.status.replaceAll('_', ' ')}</span>
      </div>
      {stage.status === 'failed' && <p className="fine-error" role="alert">This step failed. Saved progress remains attached to this job.</p>}
      {waiting && <p className="fine-panel-copy">{run.checkpoint?.name === 'preview-review' ? 'Open the preview to review your examples before building the full dataset.' : 'Preparation is finished. Training starts only when you authorize it.'}</p>}
      {progress && !waiting && <>
        <div className="fine-job-count"><strong>{progress.completed.toLocaleString()} <span>/ {progress.total.toLocaleString()}</span></strong><span>{percentage === undefined ? 'In progress' : `${percentage}%`}</span></div>
        {percentage !== undefined && <div className="fine-progress-track" role="progressbar" aria-label={`${stepLabels[progress.step] ?? progress.step} progress`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percentage}><span style={{ width: `${percentage}%` }} /></div>}
      </>}
      {!complete && !waiting && <div className="fine-job-timing">
        <div><span>Step elapsed</span><strong>{Number.isFinite(elapsed) && elapsed >= 0 ? elapsed < 60000 ? 'Less than a minute' : duration(elapsed) : 'Not started'}</strong></div>
        <div><span>Estimated step remaining</span><strong>{run.state_stale ? 'Unavailable · state is out of date' : !running ? 'Unavailable · not running' : remaining ? `About ${duration(remaining)}` : progress?.completed === progress?.total && progress?.total ? 'Finishing up…' : 'Estimating as progress arrives…'}</strong></div>
      </div>}
      {!complete && !waiting && <p className="fine-action-note">{progress ? 'Counts apply to this step, not the final accepted dataset size. ' : ''}Time estimates use worker progress retained in Temporal heartbeats and survive page reloads. Later steps and review waits are not included; retries and changing model speed can affect the estimate.</p>}
      <StepStates caption={`${stage.name === 'preparation' ? 'Preparation' : 'Training'} steps`} steps={stage.steps} />
    </section>
  );
}

function ArtifactDownload({ run, artifact }: { run: Run; artifact: 'adapter' | 'dataset' }) {
  const request = useRef<AbortController | null>(null);
  const [downloading, setDownloading] = useState(false);
  const [error, setError] = useState('');
  const [sent, setSent] = useState(false);
  useEffect(() => () => request.current?.abort(), []);
  const dataset = artifact === 'dataset';
  const title = dataset ? 'Prepared Dataset' : 'Published Adapter';
  const filename = dataset ? 'dataset.jsonl' : 'adapter.gguf';
  if (!(dataset ? run.dataset_available : run.adapter_available)) return null;

  async function download() {
    if (request.current || run.state_stale) return;
    const controller = new AbortController();
    request.current = controller;
    setDownloading(true);
    setError('');
    setSent(false);
    try {
      const blob = await api.downloadTrainingArtifact(run.workflow_id, run.run_id, artifact, controller.signal);
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      try {
        link.href = url;
        link.download = filename;
        document.body.appendChild(link);
        link.click();
      } finally {
        link.remove();
        URL.revokeObjectURL(url);
      }
      setSent(true);
    } catch (err) {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : `Could not download ${artifact}.`);
    } finally {
      if (!controller.signal.aborted) setDownloading(false);
      request.current = null;
    }
  }

  return (
    <section className="fine-panel" aria-label={title}>
      <div className="fine-panel-heading"><div><span>{dataset ? 'Preparation · Completed artifact' : 'Training · Published artifact'}</span><h3>{title}</h3></div></div>
      <p className="fine-panel-copy">{dataset ? `${run.dataset_record_count} verified examples. Download your dataset as JSONL, including small test datasets that are not ready for training.` : "Download this job's published adapter. The service checks availability again when you download."}</p>
      {error && <p className="fine-error" role="alert">Could not download {artifact}: {error}</p>}
      <div className="fine-actions">
        <button className="btn btn-primary" type="button" disabled={downloading || run.state_stale} onClick={() => void download()}>
          {downloading ? 'Downloading…' : error ? 'Retry Download' : dataset ? 'Download Dataset' : 'Download Adapter'}
        </button>
        <span className="fine-action-note" role="status">
          {run.state_stale ? 'Refresh to confirm current job state before downloading.' : sent ? 'Download sent to your browser.' : downloading ? `Fetching ${artifact}…` : `Saves as ${filename}.`}
        </span>
      </div>
    </section>
  );
}

function EvaluationResults({ run }: { run: Run }) {
  const [report, setReport] = useState<TrainingEvaluation | null>(null);
  const [error, setError] = useState('');
  const [retry, setRetry] = useState(0);

  useEffect(() => {
    if (!run.evaluation_available) return;
    const controller = new AbortController();
    setReport(null);
    setError('');
    api.trainingEvaluation(run.workflow_id, run.run_id, controller.signal)
      .then((result) => { if (!controller.signal.aborted) setReport(result); })
      .catch((err) => {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'Could not load evaluation results.');
      });
    return () => controller.abort();
  }, [run.workflow_id, run.run_id, run.evaluation_available, retry]);

  return (
    <section className="fine-panel training-evaluation" aria-label="Evaluation Results">
      <div className="fine-panel-heading">
        <div><span>Training · Evaluation</span><h3>Evaluation Results</h3></div>
        {report && <span className="fine-status">{report.accepted ? 'Accepted' : 'Needs improvement'}</span>}
      </div>
      {!run.evaluation_available ? (
        <p role="status">{run.status === 'failed' ? 'This job failed without an evaluation report. No evaluation scores are available.' : 'No evaluation report is available yet. Refresh the job to check again.'}</p>
      ) : error ? (
        <>
          <p className="fine-error" role="alert">Could not load evaluation results: {error}</p>
          <button className="btn btn-secondary" type="button" onClick={() => { setError(''); setRetry((value) => value + 1); }}>Retry Evaluation</button>
        </>
      ) : !report ? <p role="status">Loading evaluation results…</p> : (
        <>
          <p className="fine-panel-copy">Scores use normalized exact match (case and whitespace), not semantic or manual factual grading. Acceptance is the saved evaluation decision; it does not mean an adapter has been published or is available to download.</p>
          <div className="fine-run-facts">
            <span><strong>Base correct</strong>{report.base_correct} / {report.total}</span>
            <span><strong>Adapter correct</strong>{report.adapter_correct} / {report.total}</span>
          </div>
          <div className="fine-records">
            {report.answers.map((answer, index) => (
              <article className="fine-record" key={index}>
                <div className="fine-record-number">{String(index + 1).padStart(2, '0')}</div>
                <div className="fine-record-fields">
                  <details>
                    <summary>{answer.prompt}</summary>
                    <p><strong>Expected:</strong> {answer.expected}</p>
                    <p><strong>Base answer:</strong> {answer.base_answer}</p>
                    <p><strong>Adapter answer:</strong> {answer.adapter_answer}</p>
                  </details>
                </div>
              </article>
            ))}
          </div>
          {report.answers.length === 0 && <p>No answer details were saved in this report.</p>}
        </>
      )}
    </section>
  );
}

export function StartPreparedTraining({ run, onRefresh, onStarted }: { run: Run; onRefresh: () => void; onStarted?: () => void }) {
  const { trainingSubmission, startPreparedTraining } = useFineTuningRunner();
  const submission = trainingSubmission(run);
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);
  const waiting = run.status === 'waiting' && run.stage === 'preparation' && !!run.checkpoint?.id
    && run.checkpoint.name === 'start-training' && run.checkpoint.stage === 'preparation' && !run.checkpoint.step
    && run.stages.some((stage) => stage.name === 'preparation' && stage.status === 'completed');
  const submitting = submission?.status === 'sending';
  const uncertain = submission?.status === 'uncertain';
  const accepted = submission?.status === 'accepted';
  if (run.preparation_only || (!waiting && !submission)) return null;

  async function handleStart() {
    if (await startPreparedTraining(run) && mounted.current) {
      onRefresh();
      onStarted?.();
    }
  }

  return (
    <section className="fine-panel" aria-label="Start prepared training">
      <div className="fine-panel-heading">
        <div><span>Preparation complete</span><h3>Dataset Prepared</h3></div>
      </div>
      <p className="fine-panel-copy">Preparation is complete. Start Training uses this job's prepared dataset and the training settings saved when the job was submitted.</p>
      {!run.training_ready && <p className="fine-action-note" role="status">This dataset has {run.dataset_record_count} examples and is not training-ready. Training needs at least 20 training, 10 validation, and 20 protected test examples, with sufficient distinct facts and valid splits. Small capped datasets are for preparation tests; download this dataset or prepare a larger one to train.</p>}
      {submission?.error && <p className="fine-error" role="alert">{submission.error}</p>}
      <div className="fine-review-actions">
        <span className="fine-action-note" role="status">
          {accepted ? 'Training authorized. Follow the job state for training progress.'
            : uncertain ? 'Could not confirm authorization. Retry Start Training sends the same submission; it does not create another job.'
              : submitting ? 'Requesting Training authorization…'
                : run.state_stale ? 'Refresh to confirm the current job state before starting Training.'
                  : !run.training_ready ? 'Start Training is unavailable for this dataset.'
                    : waiting ? 'Training will not start until you choose Start Training.'
                    : 'Start Training is available only at the completed Preparation checkpoint.'}
        </span>
        <button className="btn btn-primary" type="button" disabled={submitting || accepted || (!uncertain && (!waiting || run.state_stale || !run.training_ready))} onClick={() => void handleStart()}>
          {submitting ? 'Submitting…' : uncertain ? 'Retry Start Training' : accepted ? 'Training Authorized' : 'Start Training'}
        </button>
      </div>
    </section>
  );
}

function StepStates({ caption, steps }: { caption: string; steps: TrainingStep[] }) {
  return (
    <div className="fine-step-plan">
      <h4>{caption}</h4>
      <ol aria-label={caption}>
        {steps.map((step, index) => (
          <li key={step.name} className={`fine-step-item fine-step-${step.status}`} aria-current={step.status === 'running' ? 'step' : undefined}>
            <span className="fine-step-marker" aria-hidden="true">{step.status === 'completed' ? '✓' : step.status === 'failed' ? '!' : index + 1}</span>
            <div><strong>{stepLabels[step.name] ?? step.name}</strong><span>{step.status.replaceAll('_', ' ') || 'Pending'}{step.attempt > 1 ? ` · Attempt ${step.attempt}` : ''}</span></div>
            {step.closed_at && <span className="fine-step-time">{formatDate(step.closed_at)}</span>}
          </li>
        ))}
      </ol>
      {steps.length === 0 && <p className="fine-action-note">No steps reported yet.</p>}
    </div>
  );
}

// Keep unsubmitted edits across in-app navigation. Submitted snapshots belong
// to the provider so uncertain retries cannot change their input.
function PreviewReview({ run, savedRecords, onRefresh }: { run: Run; savedRecords: TrainingPreviewRecord[]; onRefresh: () => void }) {
  const { previewSubmission, continuePreview } = useFineTuningRunner();
  const submission = previewSubmission(run);
  const draftKey = JSON.stringify([run.workflow_id, run.run_id, run.checkpoint?.id]);
  const [draft, setDraft] = useState(() => (submission?.request.records ?? previewDrafts.get(draftKey) ?? savedRecords).map((record) => ({ ...record })));
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);
  const waiting = run.status === 'waiting' && run.checkpoint?.name === 'preview-review'
    && run.checkpoint.stage === 'preparation' && run.checkpoint.step === 'preview';
  const [open, setOpen] = useState(waiting);
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => { if (waiting) setOpen(true); }, [waiting]);
  useEffect(() => {
    const element = dialog.current;
    if (!element || !open) return;
    element.showModal();
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => { element.close(); document.body.style.overflow = previousOverflow; };
  }, [open]);
  const submitting = submission?.status === 'sending';
  const uncertain = submission?.status === 'uncertain';
  const accepted = submission?.status === 'accepted';
  const editable = waiting && !run.state_stale && (!submission || submission.status === 'rejected');
  const records = submission && submission.status !== 'rejected' ? submission.request.records : waiting ? draft : savedRecords;

  async function handleContinue() {
    if (await continuePreview(run, records)) {
      previewDrafts.delete(draftKey);
      if (mounted.current) { setOpen(false); onRefresh(); }
    }
  }

  function updateRecord(index: number, field: 'prompt' | 'target', value: string) {
    const updated = draft.map((record, recordIndex) => recordIndex === index ? { ...record, [field]: value } : record);
    previewDrafts.set(draftKey, updated);
    setDraft(updated);
  }

  return (
    <>
    <section className="fine-panel fine-preview-launcher" aria-label="Dataset preview">
      <div><h3>{waiting ? 'Review your preview' : 'Saved preview'}</h3><p>{records.length} {records.length === 1 ? 'example' : 'examples'} · {waiting ? 'Review before building the full dataset.' : 'View the examples saved with this job.'}</p></div>
      <button className="btn btn-primary" type="button" onClick={() => setOpen(true)}>{waiting ? 'Review Preview' : 'View Preview'}</button>
    </section>
    <dialog ref={dialog} className="fine-preview-dialog" aria-labelledby="fine-preview-title" onCancel={() => setOpen(false)} onClose={(event) => { if (!event.currentTarget.open) setOpen(false); }} onClick={(event) => { if (event.target === event.currentTarget) setOpen(false); }} onKeyDown={(event) => {
      if (event.key !== 'Tab') return;
      const controls = [...event.currentTarget.querySelectorAll<HTMLElement>('button:not([disabled]), textarea:not([disabled]), summary, [tabindex="0"]')].filter((element) => element.getClientRects().length > 0);
      const first = controls[0], last = controls.at(-1);
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    }}>
    <section className="fine-panel fine-review-panel" aria-label="Review Examples">
      <div className="fine-panel-heading">
        <div><span>Dataset preview · {records.length} {records.length === 1 ? 'record' : 'records'}</span><h3 id="fine-preview-title">Review Examples</h3></div>
        <button className="btn btn-secondary" type="button" autoFocus onClick={() => setOpen(false)}>Close preview</button>
      </div>
      <p className="fine-panel-copy">
        {waiting ? 'Review and edit these examples before building the full dataset. Build Training Dataset does not start training.' : 'Saved preview examples. Review is read-only because this job is not waiting at Preview.'}
      </p>
      {editable && <p className="fine-action-note">Closing this preview does not stop preparation. Your edits stay in this browser session as you move between screens; submit them before reloading or closing this browser tab.</p>}
      {run.preview?.warnings && run.preview.warnings.length > 0 && (
        <ul aria-label="Preparation warnings">{run.preview.warnings.map((warning, index) => <li key={index}>{warning}</li>)}</ul>
      )}
      <div className="fine-records">
        {records.map((record, index) => {
          const saved = savedRecords[index];
          const edited = record.prompt !== saved?.prompt || record.target !== saved?.target;
          return (
            <article className="fine-record" key={index}>
              <div className="fine-record-number">{String(index + 1).padStart(2, '0')}</div>
              <div className="fine-record-fields">
                <FieldLabel tooltipKey="fineTuningPrompt">Prompt<textarea readOnly={!editable} value={record.prompt} onChange={(event) => updateRecord(index, 'prompt', event.target.value)} /></FieldLabel>
                <FieldLabel tooltipKey="fineTuningTarget">Target<textarea readOnly={!editable} value={record.target} onChange={(event) => updateRecord(index, 'target', event.target.value)} /></FieldLabel>
                <details><summary>Source evidence</summary><p>{record.evidence}</p></details>
              </div>
              <span className="fine-verification review">{accepted ? 'Accepted' : submitting || uncertain ? 'Submitted' : edited ? 'Edited' : waiting ? 'Review' : 'Saved'}</span>
            </article>
          );
        })}
      </div>
      {records.length === 0 && <p role="status">No preview examples are available.</p>}
      {submission?.error && <p className="fine-error" role="alert">{submission.error}</p>}
      <div className="fine-review-actions">
        <span className="fine-action-note" role="status">
          {accepted ? 'Review accepted. Follow the job state for preparation progress.'
            : uncertain ? 'Could not confirm acceptance. Retry Build Training Dataset sends the same submitted examples. Editing is locked until acceptance is confirmed.'
              : submitting ? 'Submitting reviewed examples…'
                : editable ? 'Build Training Dataset submits these examples and continues full-dataset preparation. Training remains a separate choice.'
                  : 'Build Training Dataset is available only while Preview is waiting for review with current job state.'}
        </span>
        <button className="btn btn-primary" type="button" disabled={submitting || accepted || (!uncertain && (!editable || records.length === 0))} onClick={() => void handleContinue()}>
          {submitting ? 'Submitting…' : uncertain ? 'Retry Build Training Dataset' : accepted ? 'Review Accepted' : 'Build Training Dataset'}
        </button>
      </div>
    </section>
    </dialog>
    </>
  );
}
