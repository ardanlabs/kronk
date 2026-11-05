import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { api } from '../services/api';
import { useFineTuningRunner } from '../contexts/FineTuningRunnerContext';
import type { TrainingEvaluation, TrainingPreviewRecord, TrainingRun as Run, TrainingStep } from '../types';
import { FieldLabel } from './ParamTooltips';

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

export function TrainingJobState({ run, onRefresh }: { run: Run; onRefresh: () => void }) {
  return (
    <section className="fine-run-card" aria-label="Training job state">
      <div className="fine-run-heading">
        <div><span className="fine-eyebrow">Job</span><h3>{run.workflow_id}</h3></div>
        <div className="fine-actions">
          <span className="fine-status">{run.status}</span>
          <button className="btn btn-secondary" onClick={onRefresh}>Refresh</button>
        </div>
      </div>
      <div className="fine-run-facts">
        <span><strong>Run ID</strong>{run.run_id}</span>
        <span><strong>Started</strong>{formatDate(run.started_at)}</span>
        <span><strong>Updated</strong>{formatDate(run.updated_at)}</span>
        {run.closed_at && <span><strong>Closed</strong>{formatDate(run.closed_at)}</span>}
        {run.stage && <span><strong>Current stage</strong>{run.stage}</span>}
        {run.step && <span><strong>Current step</strong>{run.step}</span>}
      </div>
      {run.state_stale && <p role="status" className="fine-action-note">This is the last reported state and may be out of date. Refresh to request the latest state.</p>}
      {run.progress && <p className="fine-action-note">Reported progress · {run.progress.stage} / {run.progress.step}: {run.progress.completed} / {run.progress.total}</p>}

      <ol className="training-job-stages" aria-label="Job stages">
        {run.stages.map((stage) => {
          const previewIndex = stage.name === 'preparation' && run.preview ? stage.steps.findIndex((step) => step.name === 'preview') : -1;
          return (
            <li key={stage.name} className="training-job-stage">
              <div className="fine-panel-heading">
                <div><span>Stage</span><h4>{stage.name}</h4></div>
                <span className="fine-status">{stage.status}</span>
              </div>
              <StepStates caption={`${stage.name} steps`} steps={previewIndex < 0 ? stage.steps : stage.steps.slice(0, previewIndex + 1)} />
              {previewIndex >= 0 && run.preview && (
                <>
                  <PreviewReview
                    key={JSON.stringify([run.workflow_id, run.run_id, run.checkpoint?.id])}
                    run={run}
                    savedRecords={run.preview.records}
                    onRefresh={onRefresh}
                  />
                  {previewIndex + 1 < stage.steps.length && <StepStates caption={`Remaining ${stage.name} steps`} steps={stage.steps.slice(previewIndex + 1)} />}
                </>
              )}
              {stage.steps.length === 0 && <p className="fine-action-note">No steps reported yet.</p>}
              {stage.name === 'preparation' && (
                <StartPreparedTraining key={JSON.stringify([run.workflow_id, run.run_id])} run={run} onRefresh={onRefresh} />
              )}
            </li>
          );
        })}
      </ol>
      {run.stages.length === 0 && <p className="fine-action-note">No stages reported yet.</p>}
      <EvaluationResults key={JSON.stringify([run.workflow_id, run.run_id, run.evaluation_available])} run={run} />
      <AdapterDownload key={JSON.stringify([run.workflow_id, run.run_id, run.adapter_available, run.state_stale])} run={run} />
    </section>
  );
}

function AdapterDownload({ run }: { run: Run }) {
  const request = useRef<AbortController | null>(null);
  const [downloading, setDownloading] = useState(false);
  const [error, setError] = useState('');
  const [sent, setSent] = useState(false);
  useEffect(() => () => request.current?.abort(), []);
  if (!run.adapter_available) return null;

  async function download() {
    if (request.current || run.state_stale) return;
    const controller = new AbortController();
    request.current = controller;
    setDownloading(true);
    setError('');
    setSent(false);
    try {
      const blob = await api.downloadTrainingAdapter(run.workflow_id, run.run_id, controller.signal);
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      try {
        link.href = url;
        link.download = 'adapter.gguf';
        document.body.appendChild(link);
        link.click();
      } finally {
        link.remove();
        URL.revokeObjectURL(url);
      }
      setSent(true);
    } catch (err) {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'Could not download adapter.');
    } finally {
      if (!controller.signal.aborted) setDownloading(false);
      request.current = null;
    }
  }

  return (
    <section className="fine-panel" aria-label="Published Adapter">
      <div className="fine-panel-heading"><div><span>Training · Published artifact</span><h3>Published Adapter</h3></div></div>
      <p className="fine-panel-copy">Download this job's published adapter. The service checks availability again when you download.</p>
      {error && <p className="fine-error" role="alert">Could not download adapter: {error}</p>}
      <div className="fine-actions">
        <button className="btn btn-primary" type="button" disabled={downloading || run.state_stale} onClick={() => void download()}>
          {downloading ? 'Downloading…' : error ? 'Retry Download' : 'Download Adapter'}
        </button>
        <span className="fine-action-note" role="status">
          {run.state_stale ? 'Refresh to confirm current job state before downloading.' : sent ? 'Download sent to your browser.' : downloading ? 'Fetching adapter…' : 'Saves as adapter.gguf.'}
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

function StartPreparedTraining({ run, onRefresh }: { run: Run; onRefresh: () => void }) {
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
  if (!waiting && !submission) return null;

  async function handleStart() {
    if (await startPreparedTraining(run) && mounted.current) onRefresh();
  }

  return (
    <section className="fine-panel" aria-label="Start prepared training">
      <div className="fine-panel-heading">
        <div><span>Preparation complete</span><h3>Dataset Prepared</h3></div>
      </div>
      <p className="fine-panel-copy">Preparation is complete. Start Training uses this job's prepared dataset and the training settings saved when the job was submitted.</p>
      {submission?.error && <p className="fine-error" role="alert">{submission.error}</p>}
      <div className="fine-review-actions">
        <span className="fine-action-note" role="status">
          {accepted ? 'Training authorized. Follow the job state for training progress.'
            : uncertain ? 'Could not confirm authorization. Retry Start Training sends the same submission; it does not create another job.'
              : submitting ? 'Requesting Training authorization…'
                : run.state_stale ? 'Refresh to confirm the current job state before starting Training.'
                  : waiting ? 'Training will not start until you choose Start Training.'
                    : 'Start Training is available only at the completed Preparation checkpoint.'}
        </span>
        <button className="btn btn-primary" type="button" disabled={submitting || accepted || (!uncertain && (!waiting || run.state_stale))} onClick={() => void handleStart()}>
          {submitting ? 'Submitting…' : uncertain ? 'Retry Start Training' : accepted ? 'Training Authorized' : 'Start Training'}
        </button>
      </div>
    </section>
  );
}

function StepStates({ caption, steps }: { caption: string; steps: TrainingStep[] }) {
  return (
    <div className="fine-history-table-wrap">
      <table className="fine-history-table">
        <caption className="training-job-caption">{caption}</caption>
        <thead><tr><th scope="col">Step</th><th scope="col">Status</th><th scope="col">Attempt</th><th scope="col">Started</th><th scope="col">Closed</th></tr></thead>
        <tbody>{steps.map((step) => (
          <tr key={step.name}>
            <th scope="row">{step.name}</th><td>{step.status}</td><td>{step.attempt}</td>
            <td>{formatDate(step.started_at)}</td><td>{formatDate(step.closed_at)}</td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  );
}

// Unsubmitted edits stay local; the provider retains submitted snapshots across
// navigation so an uncertain request can only be retried with identical input.
function PreviewReview({ run, savedRecords, onRefresh }: { run: Run; savedRecords: TrainingPreviewRecord[]; onRefresh: () => void }) {
  const { previewSubmission, continuePreview } = useFineTuningRunner();
  const submission = previewSubmission(run);
  const [draft, setDraft] = useState(() => (submission?.request.records ?? savedRecords).map((record) => ({ ...record })));
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);
  const waiting = run.status === 'waiting' && run.checkpoint?.name === 'preview-review'
    && run.checkpoint.stage === 'preparation' && run.checkpoint.step === 'preview';
  const submitting = submission?.status === 'sending';
  const uncertain = submission?.status === 'uncertain';
  const accepted = submission?.status === 'accepted';
  const editable = waiting && !run.state_stale && (!submission || submission.status === 'rejected');
  const records = submission && submission.status !== 'rejected' ? submission.request.records : waiting ? draft : savedRecords;

  async function handleContinue() {
    if (await continuePreview(run, records) && mounted.current) onRefresh();
  }

  function updateRecord(index: number, field: 'prompt' | 'target', value: string) {
    setDraft((current) => current.map((record, recordIndex) => recordIndex === index ? { ...record, [field]: value } : record));
  }

  return (
    <section className="fine-panel fine-review-panel" aria-label="Review Examples">
      <div className="fine-panel-heading">
        <div><span>Preparation · Preview</span><h3>Review Examples</h3></div>
        <strong>{records.length} Records</strong>
      </div>
      <p className="fine-panel-copy">
        {waiting ? 'Review and edit these examples before continuing preparation of the full dataset. Continue does not start training.' : 'Saved preview examples. Review is read-only because this job is not waiting at Preview.'}
      </p>
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
            : uncertain ? 'Could not confirm acceptance. Retry Continue sends the same submitted examples. Editing is locked until acceptance is confirmed.'
              : submitting ? 'Submitting reviewed examples…'
                : editable ? 'Continue submits these examples and allows preparation to finish. Training remains a separate choice.'
                  : 'Continue is available only while Preview is waiting for review with current job state.'}
        </span>
        <button className="btn btn-primary" type="button" disabled={submitting || accepted || (!uncertain && (!editable || records.length === 0))} onClick={() => void handleContinue()}>
          {submitting ? 'Submitting…' : uncertain ? 'Retry Continue' : accepted ? 'Review Accepted' : 'Continue'}
        </button>
      </div>
    </section>
  );
}
