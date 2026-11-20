import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { api, ApiError } from '../services/api';
import { FINE_TUNING_MODEL_ID, DEFAULT_TRAINING_OPTIONS, requestMessage, retryDelay, trainingPath, trainingStepLabels, useFineTuningRunner } from '../contexts/FineTuningRunnerContext';
import { useModelList } from '../contexts/ModelListContext';
import type { PreparationDataset, TrainingOptions, TrainingRun as Run } from '../types';
import { FieldLabel } from './ParamTooltips';
import { active, Badge, date, DetailTabs, failedStep, records, runKey, stepGlyphs, stepStateLabels, timeLeft, useNow, type StepState } from './PreparationRuns';
import { EvaluationChecks, failureText, notAccepted, runIssues, RunIssues, useEvaluation } from './RunIssues';

// Training runs start from a saved dataset version and belong to that dataset: the
// dataset's page lists them, the setup page starts one, and each run has its own page.

export function duration(milliseconds: number): string {
  if (milliseconds < 60000) return 'less than 1 min';
  const minutes = Math.max(1, Math.ceil(milliseconds / 60000));
  return minutes < 60 ? `${minutes} min` : `${Math.floor(minutes / 60)} hr ${minutes % 60} min`;
}

const trainingSteps = ['split', 'train', 'evaluate', 'publish'];
const stepCopy: Record<string, string> = {
  split: 'The dataset is being split into training records and evaluation questions.',
  train: 'The adapter is learning from the training records. Leaving this screen never stops it.',
  evaluate: 'The adapter and the base model answer the evaluation questions so they can be compared.',
  publish: 'The adapter is being saved so it can be downloaded.',
};
const baseModelName = 'Gemma 3 1B IT · Q8_0';
const trainingControlIDs = new Map<string, string>();

export const trainedOn = (run: Run) => run.training_dataset_name ?? run.training_dataset_id ?? 'Uploaded dataset';
const span = (from?: string, to?: string) => from && to ? duration(Date.parse(to) - Date.parse(from)) : '—';

type Result = 'accepted' | 'not_compared' | 'needs_improvement';
const results: Record<Result, { label: string; detail: string; glyph: string }> = {
  accepted: { label: 'Accepted', detail: 'Beat the base model', glyph: '✓' },
  not_compared: { label: 'Not compared', detail: 'No evaluation questions', glyph: '–' },
  needs_improvement: { label: 'Needs improvement', detail: 'Under 70% correct or no gain', glyph: '!' },
};
// result is a finished training's evaluation outcome; runs still going or closed early have none.
export function result(run: Run): Result | null {
  if (run.status === 'needs_improvement') return 'needs_improvement';
  if (run.status !== 'completed') return null;
  return run.evaluation_result ?? 'accepted';
}

// setupPath opens the training setup for a dataset version; back says where its back link goes.
export const setupPath = (dataset: string, version: number, back: Record<string, string> = {}) =>
  `/fine-tuning?${new URLSearchParams({ view: 'train', dataset, version: String(version), ...back })}`;
const addDocumentsPath = (dataset: string) => `/fine-tuning?${new URLSearchParams({ view: 'prepare', dataset })}`;

// liveProgress is the training step's progress while the worker reports it.
function liveProgress(run: Run) {
  const progress = run.progress;
  return ['running', 'pause_requested', 'paused'].includes(run.status) && !run.state_stale && progress?.total && progress.step === run.step ? progress : undefined;
}

function TrainingStepCell({ run }: { run: Run }) {
  const progress = liveProgress(run);
  const outcome = result(run);
  if (outcome) return <><span className={`prep-main${outcome === 'accepted' ? ' dash-done-text' : ''}`}>{outcome === 'needs_improvement' ? 'Adapter trained' : <><span aria-hidden="true">✓ </span>Adapter ready</>}</span><span className="prep-sub">Took {span(run.started_at, run.closed_at)}</span></>;
  if (run.status === 'failed') return <><span className="prep-main">Failed during {(trainingStepLabels[failedStep(run)?.name ?? run.step ?? ''] ?? 'training').toLowerCase()}</span><span className="prep-sub">Open for the reason</span></>;
  if (run.status === 'stopped') return <><span className="prep-main">Stopped</span><span className="prep-sub">Cancelled · cannot resume</span></>;
  if (run.status === 'paused' || run.status === 'pause_requested') return <><span className="prep-main">{run.status === 'paused' ? 'Paused' : 'Pausing'}</span><span className="prep-sub">{trainingStepLabels[run.step ?? ''] ?? 'Training'}</span></>;
  if (run.status === 'queued') return <><span className="prep-main">Waiting for a worker</span><span className="prep-sub">Not started</span></>;
  const percent = progress ? Math.min(100, Math.floor(progress.completed / progress.total * 100)) : null;
  return <>
    <span className="prep-main">{trainingStepLabels[run.step ?? ''] ?? 'Training'}</span>
    {percent !== null && <div className="prep-track" role="progressbar" aria-label="Training progress" aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}><span style={{ width: `${percent}%` }} /></div>}
    {progress?.remaining_seconds ? <span className="dash-eta">{timeLeft(progress.remaining_seconds * 1000)} left</span> : null}
  </>;
}

function ResultCell({ run }: { run: Run }) {
  const outcome = result(run);
  if (!outcome) return <span className="prep-sub">{active(run) ? 'After evaluation' : '—'}</span>;
  return <><span className={`train-result train-result-${outcome}`}><span aria-hidden="true">{results[outcome].glyph}</span>{results[outcome].label}</span><span className="prep-sub">{results[outcome].detail}</span></>;
}

// trainingLabel is a training's status in a few words, such as "Training · 35% · 4 min left"
// while it runs and "Trained · Accepted" once it finishes.
export function trainingLabel(run: Run) {
  const outcome = result(run);
  if (outcome) return `Trained · ${results[outcome].label}`;
  if (run.status === 'failed') return 'Training failed';
  if (run.status === 'stopped') return 'Training stopped';
  if (run.status === 'paused') return 'Training paused';
  if (run.status === 'pause_requested') return 'Training pausing';
  if (run.status === 'queued') return 'Training queued';
  const progress = liveProgress(run);
  if (!progress) return 'Training';
  const left = progress.remaining_seconds ? ` · ${timeLeft(progress.remaining_seconds * 1000)} left` : '';
  return `Training · ${Math.min(100, Math.floor(progress.completed / progress.total * 100))}%${left}`;
}

// useDatasetTrainings loads one dataset version's training runs, newest first, and keeps them
// current while one is in progress.
export function useDatasetTrainings(datasetID: string | undefined, version: number, visible: boolean) {
  const [runs, setRuns] = useState<Run[]>([]);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    if (!visible || !datasetID) return;
    const controller = new AbortController();
    setRuns([]);
    setLoading(true);
    setError('');
    void (async () => {
      while (!controller.signal.aborted) {
        try {
          const matching: Run[] = [];
          let token = '';
          do {
            const page = await api.trainingRuns(token, controller.signal, '', { mode: 'training', targetDatasetID: datasetID, pageSize: 50, order: 'newest' });
            matching.push(...(page.runs ?? []).filter((run) => version > 0 && run.training_dataset_id === datasetID && run.training_dataset_version === version));
            token = page.next_page_token ?? '';
          } while (token && !controller.signal.aborted);
          if (controller.signal.aborted) return;
          setRuns(matching);
          setError('');
          setLoading(false);
          if (!matching.some(active)) return;
        } catch (err) {
          if (controller.signal.aborted) return;
          setError(requestMessage(err, 'Could not load trainings.'));
          setLoading(false);
        }
        await retryDelay(controller.signal);
      }
    })();
    return () => controller.abort();
  }, [visible, datasetID, version]);
  return { runs, error, loading };
}

// DatasetTrainings is the Training runs tab of a dataset's page: every training of
// the dataset, newest first, 10 per page like the dashboard.
export function DatasetTrainings({ runs, error, loading, trainPath, trainingReady }: { runs: Run[]; error: string; loading: boolean; trainPath: string; trainingReady: boolean }) {
  const [page, setPage] = useState(0);
  const pages = Math.max(1, Math.ceil(runs.length / 10));
  const shown = Math.min(page, pages - 1);
  const rows = runs.slice(shown * 10, shown * 10 + 10);
  const running = runs.some(active);
  const actionLabel = runs.length ? 'Retrain' : 'Start training';
  const blocked = running ? 'Wait for the current training run to finish before starting another.' : !trainingReady ? 'This version needs at least 20 training records. Add documents in the Dataset tab to prepare more.' : '';
  const action = blocked ? <button className="btn btn-primary" type="button" disabled>{actionLabel}</button>
    : <Link className="btn btn-primary" to={trainPath}>{actionLabel}</Link>;
  return <>
    {error && <p className="prep-warning" role="status">Could not refresh trainings: {error} Retrying automatically.</p>}
    {loading && <p className="prep-copy" role="status">Loading training runs…</p>}
    {!loading && runs.length > 0 && <div className="ds-training-heading">
      <p className="prep-copy">{blocked || 'Retraining starts a new run from this dataset version. Earlier runs are kept.'}</p>
      {action}
    </div>}
    {runs.length > 0 && <div className="catalog-table-wrap prep-table-wrap"><table className="catalog-table prep-table dash-table train-table">
      <caption className="prep-sr-only">Training runs of this dataset</caption>
      <thead><tr><th scope="col">Version</th><th scope="col">Status</th><th scope="col">Step</th><th scope="col">Result</th><th scope="col">Date created</th><th scope="col"><span className="prep-sr-only">Training details</span></th></tr></thead>
      <tbody>
        {rows.map((run) => <tr key={runKey(run)}>
          <td><Link className="prep-run-link" to={trainingPath(run)}>{run.training_dataset_version ? `Version ${run.training_dataset_version}` : 'Version —'}</Link><span className="prep-sub">{baseModelName}</span></td>
          <td data-label="Status"><Badge run={run} />{run.state_stale && active(run) && <span className="prep-sub prep-stale-tag">Last known status</span>}</td>
          <td data-label="Step"><TrainingStepCell run={run} /></td>
          <td data-label="Result"><ResultCell run={run} /></td>
          <td data-label="Date created">{date(run.started_at)}</td>
          <td className="prep-row-action"><Link className="btn btn-secondary" aria-label={`Details of training on version ${run.training_dataset_version ?? ''}, created ${date(run.started_at)}`} to={trainingPath(run)}>Details</Link></td>
        </tr>)}
      </tbody>
    </table></div>}
    {pages > 1 && <nav className="dash-pager" aria-label="Pages">
      <span>Showing {shown * 10 + 1}–{shown * 10 + rows.length} of {runs.length} · page {shown + 1} of {pages}</span>
      <div className="prep-actions">
        <button className="btn btn-secondary" type="button" disabled={shown === 0} onClick={() => setPage(shown - 1)}>← Previous</button>
        <button className="btn btn-secondary" type="button" disabled={shown === pages - 1} onClick={() => setPage(shown + 1)}>Next →</button>
      </div>
    </nav>}
    {!loading && !runs.length && !error && <div className="empty-state ds-training-empty"><h3>No training runs yet</h3><p>{blocked || 'Train a model with this dataset. Review the settings before starting.'}</p>{action}</div>}
  </>;
}

const settingFields: [keyof TrainingOptions, string, 'fineTuningMaximumEpochs' | 'fineTuningLearningRate' | 'fineTuningBatchSize' | 'fineTuningLoRARank' | 'fineTuningLoRAAlpha' | 'fineTuningShuffleSeed', string][] = [
  ['maximum_epochs', 'Maximum epochs', 'fineTuningMaximumEpochs', '1'], ['learning_rate', 'Learning rate', 'fineTuningLearningRate', 'any'], ['batch_size', 'Batch size', 'fineTuningBatchSize', '1'],
  ['lora_rank', 'LoRA rank', 'fineTuningLoRARank', '1'], ['lora_alpha', 'LoRA alpha', 'fineTuningLoRAAlpha', 'any'], ['seed', 'Random seed', 'fineTuningShuffleSeed', '1'],
];

// TrainingSetup starts a training run on one saved dataset version.
export function TrainingSetup({ visible }: { visible: boolean }) {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const { models, loading: modelsLoading, loadModels } = useModelList();
  const { submitting, error, trainingActivity, adopting, adoptionError, refreshRuns, startDatasetTraining } = useFineTuningRunner();
  const datasetID = params.get('dataset') ?? '';
  const version = Number(params.get('version'));
  const [dataset, setDataset] = useState<PreparationDataset | null>(null);
  const [loadError, setLoadError] = useState('');
  const [attempt, setAttempt] = useState(0);
  const [training, setTraining] = useState<TrainingOptions>(DEFAULT_TRAINING_OPTIONS);
  const heading = useRef<HTMLHeadingElement>(null);

  useEffect(() => { void loadModels(); }, [loadModels]);
  useEffect(() => {
    if (!visible || !datasetID) return;
    const controller = new AbortController();
    setLoadError('');
    setDataset(null);
    api.preparationDataset(datasetID, controller.signal, version).then((next) => {
      if (controller.signal.aborted) return;
      if (next.version !== version) { setLoadError('Could not load this dataset version.'); return; }
      setDataset(next);
    }, (err) => {
      if (!controller.signal.aborted) setLoadError(requestMessage(err, 'Could not load this dataset.'));
    });
    return () => controller.abort();
  }, [visible, datasetID, version, attempt]);
  useEffect(() => { if (visible) heading.current?.focus(); }, [visible]);

  const back = params.get('preparation') && params.get('run') ? { to: `/fine-tuning?${new URLSearchParams({ preparation: params.get('preparation')!, run: params.get('run')!, details: 'trainings' })}`, label: `${dataset?.name ?? 'Dataset'} · training runs` }
    : params.get('training') && params.get('run') ? { to: `/fine-tuning?${new URLSearchParams({ training: params.get('training')!, run: params.get('run')! })}`, label: 'Training run' }
      : { to: '/fine-tuning', label: 'Dashboard' };
  // Only the requested snapshot can supply training counts and readiness.
  const latest = !!dataset && dataset.version === version;
  const evaluation = dataset ? Math.min(dataset.validation_questions, dataset.validation_needed) + Math.min(dataset.test_questions, dataset.test_needed) : 0;
  const evaluationNeeded = dataset ? dataset.validation_needed + dataset.test_needed : 0;
  const modelInstalled = !!models?.data.some((model) => model.id === FINE_TUNING_MODEL_ID);
  const invalidSetting = settingFields.find(([key]) => !Number.isFinite(training[key]) || training[key] <= 0);
  const issues: { message: string; link?: ReactNode }[] = [];
  if (!datasetID || !Number.isInteger(version) || version < 1) issues.push({ message: 'Open training from a dataset with a saved version.', link: <Link to="/fine-tuning">Go to the dashboard</Link> });
  else if (!dataset) issues.push({ message: loadError ? 'The dataset could not be loaded.' : 'Wait for the dataset to load.' });
  else if (latest && !dataset.training_ready) issues.push({ message: `This version has ${records(dataset.training_records)} for training; training needs 20.`, link: <Link to={addDocumentsPath(dataset.id)}>Add documents</Link> });
  if (!modelsLoading && !modelInstalled) issues.push({ message: `Install ${baseModelName} from the Catalog.`, link: <Link to="/catalog">Open the Catalog</Link> });
  if (invalidSetting) issues.push({ message: `Enter a positive ${invalidSetting[1].toLowerCase()}.` });
  if (trainingActivity?.path) issues.push({ message: 'Another training is already queued or running; one training runs at a time.', link: <Link to={trainingActivity.path}>Open it</Link> });
  if (adoptionError) issues.push({ message: `Could not check for trainings in progress: ${adoptionError}`, link: <button className="prep-inline-link" type="button" onClick={refreshRuns}>Refresh</button> });
  else if (adopting) issues.push({ message: 'Checking for trainings in progress…' });
  const ready = !issues.length && !submitting;

  const start = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!ready || !dataset) return;
    const run = await startDatasetTraining(training, { id: dataset.id, version });
    if (run) navigate(trainingPath(run));
  };

  return <>
    <Link className="prep-back" to={back.to}>← {back.label}</Link>
    <div className="page-header-with-action prep-heading">
      <div><h2 ref={heading} tabIndex={-1}>Train with {dataset?.name ?? 'this dataset'}</h2><p className="page-description">Train an adapter from this dataset version. Training runs on its own and does not change the dataset.</p></div>
    </div>
    <div className="fine-setup-wrapper">
      <form className="fine-configure-view" onSubmit={start}>
        <div className="fine-preparation-fields">
          <section className="fine-panel" aria-labelledby="train-dataset-title">
            <div className="fine-panel-heading"><div><span>Training dataset</span><h3 id="train-dataset-title">{dataset ? `${dataset.name} · version ${version}` : 'Dataset'}</h3></div>{dataset && latest && <span className={`fine-status ${dataset.training_ready ? 'fine-status-completed' : 'fine-status-stopped'}`}>{dataset.training_ready ? '✓ Ready for training' : 'Not ready'}</span>}</div>
            {loadError && !dataset && <p className="fine-error" role="alert">Could not load the dataset: {loadError} <button className="btn btn-secondary" type="button" onClick={() => setAttempt((value) => value + 1)}>Try again</button></p>}
            {!dataset && !loadError && <p className="prep-copy" role="status">Loading the dataset…</p>}
            {dataset && <>
              {latest ? <p className="fine-panel-copy">{records(dataset.training_records)} for training from {dataset.documents.length} {dataset.documents.length === 1 ? 'document' : 'documents'} · {evaluationNeeded ? `${evaluation} of ${evaluationNeeded} evaluation questions` : 'no evaluation questions'}</p>
                : <p className="fine-panel-copy">Training uses version {version} as it was saved. The latest version is {dataset.version}.</p>}
              {latest && evaluationNeeded > 0 && evaluation < evaluationNeeded && <p className="fine-action-note">Without all its evaluation questions, the adapter is trained without being compared with the base model. Add documents to the dataset to add them.</p>}
              <div className="fine-actions"><Link className="btn btn-secondary" to={back.to}>View dataset</Link></div>
            </>}
          </section>

          <section className="fine-panel train-settings" aria-labelledby="train-settings-title">
            <div className="fine-panel-heading"><div><span>Settings</span><h3 id="train-settings-title">Base model and settings</h3></div></div>
            <div className="fine-supported-model">
              <div><strong>{baseModelName}</strong><code>{FINE_TUNING_MODEL_ID}</code></div>
              <span className={`fine-status ${modelInstalled ? 'fine-status-completed' : 'fine-status-stopped'}`}>{modelsLoading ? 'Checking' : modelInstalled ? 'Installed' : 'Not installed'}</span>
            </div>
            {(() => {
              const field = ([key, label, tooltip, step]: typeof settingFields[number]) => <div className="form-group" key={key}>
                <FieldLabel htmlFor={`train-${key}`} tooltipKey={tooltip}>{label}</FieldLabel>
                <input id={`train-${key}`} className="form-input" type="number" required min={0} step={step} value={Number.isNaN(training[key]) ? '' : training[key]} disabled={submitting} onChange={(event) => setTraining((current) => ({ ...current, [key]: event.target.valueAsNumber }))} />
              </div>;
              return <>
                <div className="fine-recipe-columns">{settingFields.slice(0, 3).map(field)}</div>
                <details className="fine-recipe-details">
                  <summary>Advanced settings</summary>
                  <div className="fine-recipe-columns">{settingFields.slice(3).map(field)}</div>
                  <div className="fine-actions"><button className="btn btn-secondary" type="button" disabled={submitting} onClick={() => setTraining(DEFAULT_TRAINING_OPTIONS)}>Reset to defaults</button><span className="fine-action-note">These are the server’s defaults.</span></div>
                </details>
              </>;
            })()}
          </section>
          {error && <p className="fine-error" role="alert">Could not start training: {error}</p>}
        </div>
        <div className="fine-preview-action-bar" aria-label="Start training">
          <div aria-live="polite">
            <strong>{ready ? 'Ready to start training' : issues.length ? `${issues.length} ${issues.length === 1 ? 'requirement' : 'requirements'} remaining` : 'Starting…'}</strong>
            {issues[0] ? <span>{issues[0].message} {issues[0].link}</span> : <span>Trains on {dataset?.name} version {version}{latest ? ` (${records(dataset.training_records)})` : ''}. You can leave this screen; training keeps running.</span>}
          </div>
          <button className="btn btn-primary" type="submit" disabled={!ready}>{submitting ? 'Starting…' : 'Start training'}</button>
        </div>
      </form>
    </div>
  </>;
}

// TrainingRunScreen is one training run's page: live progress, then its result.
export function TrainingRunScreen({ workflowID, runID, visible }: { workflowID: string; runID: string; visible: boolean }) {
  const [run, setRun] = useState<Run | null>(null);
  const [error, setError] = useState<{ text: string; missing: boolean } | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [source, setSource] = useState<Run | null>(null);
  const heading = useRef<HTMLHeadingElement>(null);

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
        if (active(next)) timer = setTimeout(load, next.status === 'running' ? 1000 : 3000);
      } catch (err) {
        if (controller.signal.aborted) return;
        const missing = err instanceof ApiError && err.status === 404;
        setError({ text: requestMessage(err, 'Could not load this training run.'), missing });
        if (!missing) timer = setTimeout(load, 5000);
      }
    };
    void load();
    return () => { controller.abort(); clearTimeout(timer); };
  }, [visible, workflowID, runID, attempt]);

  // Back goes to the dataset's Training runs tab, on its newest preparation run's page.
  const datasetID = run?.training_dataset_id;
  useEffect(() => {
    if (!visible || !datasetID) return;
    const controller = new AbortController();
    api.trainingRuns('', controller.signal, '', { mode: 'preparation', targetDatasetID: datasetID, pageSize: 1 })
      .then((page) => setSource(page.runs?.[0] ?? null), () => {});
    return () => controller.abort();
  }, [visible, datasetID]);

  useEffect(() => { if (visible) (run ? document.getElementById('train-card-title') : heading.current)?.focus(); }, [visible, !!run]);

  const refresh = () => setAttempt((value) => value + 1);
  return <>
    <Link className="prep-back" to={source ? `/fine-tuning?${new URLSearchParams({ preparation: source.workflow_id, run: source.run_id, details: 'trainings' })}` : '/fine-tuning'}>← {source && run ? `${trainedOn(run)} · training runs` : 'Dashboard'}</Link>
    {run ? <>
      {error && <p className="prep-warning" role="status">Could not refresh this run: {error.text} Retrying automatically. <button className="prep-inline-link" type="button" onClick={refresh}>Try again</button></p>}
      <TrainingRunView key={runKey(run)} run={run} visible={visible} onRefresh={refresh} />
    </> : error?.missing ? <section className="fine-panel"><h2 ref={heading} tabIndex={-1}>Training run not found</h2><p>This training run does not exist or is no longer available.</p><Link className="btn btn-secondary" to="/fine-tuning">Back to the dashboard</Link></section>
      : error ? <div className="prep-load-error" role="alert"><span>Could not load this training run: {error.text} Retrying automatically.</span><button className="btn btn-secondary" type="button" onClick={refresh}>Try again</button></div>
        : <p className="prep-copy" role="status">Loading training run…</p>}
  </>;
}

function TrainingRunView({ run, visible, onRefresh }: { run: Run; visible: boolean; onRefresh: () => void }) {
  const now = useNow(active(run) && visible);
  const stage = run.stages.find((item) => item.name === 'training') ?? run.stages.at(-1);
  const steps = stage?.steps ?? [];
  const outcome = result(run);
  const progress = liveProgress(run);
  const percent = progress ? Math.min(100, Math.floor(progress.completed / progress.total * 100)) : null;
  const index = Math.max(0, trainingSteps.indexOf(run.step ?? '')) + 1;
  const settings = run.training_settings;
  const [detailTab, setDetailTab] = useState('');
  const [busy, setBusy] = useState('');
  const [notice, setNotice] = useState('');
  const [actionError, setActionError] = useState('');
  const [confirmStop, setConfirmStop] = useState(false);
  const [requestedRevision, setRequestedRevision] = useState(0);
  const [stopRequested, setStopRequested] = useState(false);
  const paused = run.status === 'paused';
  const pausing = run.status === 'pause_requested';
  const disabled = !!busy || run.state_stale || requestedRevision > run.control_revision || stopRequested;
  useEffect(() => { setNotice(''); }, [run.status, run.control_revision]);

  async function control(action: 'pause' | 'resume') {
    if (disabled) return;
    const key = `${runKey(run)}:${action}:${run.control_revision}`;
    const id = trainingControlIDs.get(key) ?? `control-${crypto.randomUUID()}`;
    trainingControlIDs.set(key, id);
    setBusy(action);
    setActionError('');
    try {
      const receipt = await api.controlTraining(run.workflow_id, run.run_id, action, { request_id: id, expected_revision: run.control_revision });
      trainingControlIDs.delete(key);
      setRequestedRevision(receipt.revision);
      setNotice(action === 'pause' ? 'Pause requested. Waiting for the current operation to finish.' : 'Resume requested. Waiting for the worker.');
    } catch (err) {
      if (err instanceof ApiError && (err.status === 400 || err.status === 409)) trainingControlIDs.delete(key);
      setActionError(err instanceof ApiError && err.status === 409 ? 'The run changed. Refreshing its status before you try again.' : requestMessage(err, 'The request could not be confirmed. Check the run status before retrying.'));
    } finally {
      setBusy('');
      onRefresh();
    }
  }

  async function stop() {
    if (disabled) return;
    setBusy('stop');
    setActionError('');
    try {
      await api.stopTrainingRun(run.workflow_id, run.run_id);
      setStopRequested(true);
      setConfirmStop(false);
      setNotice('Stop requested. Waiting for the worker to cancel this run.');
    } catch (err) {
      setActionError(requestMessage(err, 'Stop could not be confirmed. Check the run status before retrying.'));
    } finally {
      setBusy('');
      onRefresh();
    }
  }
  const previousStatus = useRef(run.status);
  useEffect(() => {
    if (previousStatus.current !== run.status && (run.status === 'failed' || notAccepted(run))) setDetailTab('issues');
    previousStatus.current = run.status;
  }, [run.status, run.evaluation_result]);
  const issues = runIssues(run);
  const evaluation = useEvaluation(run);
  const stepState = (name: string): StepState => {
    const status = steps.find((item) => item.name === name)?.status;
    if (status === 'completed' || status === 'failed') return status;
    if (name !== run.step) return 'pending';
    return run.status === 'stopped' ? 'stopped' : paused ? 'paused' : run.status === 'running' || pausing ? 'running' : 'pending';
  };
  const chips = <ol className="step-chips" aria-label="Steps">{trainingSteps.map((name) => {
    const state = stepState(name);
    return <li key={name} className={`step-chip step-chip-${state}`} aria-current={state === 'running' || state === 'paused' ? 'step' : undefined}><span aria-hidden="true">{stepGlyphs[state]}</span>{trainingStepLabels[name]}<span className="prep-sr-only">: {stepStateLabels[state].toLowerCase()}</span></li>;
  })}</ol>;

  let card: ReactNode;
  if (outcome) {
    const titles: Record<Result, string> = { accepted: 'The adapter beat the base model', not_compared: 'The adapter is trained', needs_improvement: 'The adapter did not meet the acceptance requirements' };
    const copies: Record<Result, string> = {
      accepted: 'It answered at least 70% of the evaluation questions correctly, more than the base model.',
      not_compared: 'This dataset had no evaluation questions, so the adapter was not compared with the base model.',
      needs_improvement: 'Training finished. The failure log shows which acceptance checks were not met.',
    };
    card = <section className={`fine-panel done-card train-done train-done-${outcome}`} aria-labelledby="train-card-title">
      <div className="done-top"><span className="dash-done-mark" aria-hidden="true">{results[outcome].glyph}</span><div>
        <span className="fine-eyebrow">{outcome === 'needs_improvement' ? 'Not accepted' : results[outcome].label}</span>
        <h3 id="train-card-title" tabIndex={-1}>{titles[outcome]}</h3>
        <p className="prep-copy">{copies[outcome]}</p>
      </div></div>
      <dl className="done-inline-stats">
        <div><dt>Trained on</dt><dd>{run.training_dataset_version ? `v${run.training_dataset_version}` : '—'}<small>{trainedOn(run)}</small></dd></div>
        <div><dt>Took</dt><dd>{span(run.started_at, run.closed_at)}<small>finished {date(run.closed_at)}</small></dd></div>
        <div><dt>Epochs</dt><dd>{settings ? `Up to ${settings.maximum_epochs}` : '—'}<small>best epoch kept</small></dd></div>
        <div><dt>Base model</dt><dd>Gemma 3 1B<small>Q8_0</small></dd></div>
      </dl>
    </section>;
  } else if (!active(run)) {
    const failed = run.status === 'failed';
    const step = failedStep(run)?.name ?? run.step;
    card = <section className={`fine-panel live-hero live-hero-${run.status}`} aria-labelledby="train-card-title">
      <div className="live-gauge">
        <div className="live-ring live-ring-idle" aria-hidden="true"><span className="ring-text">{failed ? '×' : '■'}</span></div>
      </div>
      <div className="live-main">
        <span className="fine-eyebrow">{failed ? 'Failed' : 'Stopped'} · step {Math.max(0, trainingSteps.indexOf(step ?? '')) + 1} of {trainingSteps.length}</span>
        <h3 id="train-card-title" tabIndex={-1}>{trainingStepLabels[step ?? ''] ?? 'Training'}</h3>
        <p className="prep-copy">{failed ? failureText(run) || 'An unexpected error stopped this run. See Issues for the available troubleshooting details.' : 'This training was cancelled and cannot be resumed.'} No adapter was saved; the dataset is unchanged.</p>
      </div>
      <div className="live-steps">{chips}</div>
      <div className="live-foot"><p>{[`Ran for ${span(run.started_at, run.closed_at)}`, `${trainedOn(run)}${run.training_dataset_version ? ` v${run.training_dataset_version}` : ''}`, baseModelName].join(' · ')}</p></div>
    </section>;
  } else {
    const queued = run.status === 'queued';
    const remainingSeconds = progress?.remaining_seconds;
    const timing = run.status !== 'running' ? '' : remainingSeconds ? `About ${timeLeft(remainingSeconds * 1000)} left` : progress && progress.completed >= progress.total ? 'Finishing up…' : 'Estimating time left…';
    card = <section className={`fine-panel live-hero live-hero-${paused ? 'paused' : pausing ? 'pausing' : queued ? 'queued' : 'running'}`} aria-labelledby="train-card-title">
      <div className="live-gauge">
        <div className={`live-ring${percent === null ? queued || paused ? ' live-ring-idle' : ' live-ring-spin' : ''}`} style={{ '--pct': percent ?? 0 } as React.CSSProperties} aria-hidden="true">
          {percent !== null ? <><span className="ring-num">{percent}%</span><span className="ring-text">of this step</span></> : <span className="ring-text">{paused ? 'Paused' : queued ? 'Queued' : 'Working'}</span>}
        </div>
        {timing && <p className="live-time">{timing}</p>}
      </div>
      <div className="live-main">
        <span className="fine-eyebrow">{paused ? 'Paused' : pausing ? 'Pausing' : queued ? 'Queued' : `Step ${index} of ${trainingSteps.length}`}</span>
        <h3 id="train-card-title" tabIndex={-1}>{queued ? 'Waiting for a worker' : trainingStepLabels[run.step ?? ''] ?? 'Training'}</h3>
        <p className="prep-copy">{paused ? 'Training is paused. Resume continues this run without restarting the current step. Keep the server running: the worker and model memory stay reserved, and run timeouts still apply.' : pausing ? 'Pause requested. The current batch or operation finishes before the run pauses; it has not paused yet.' : queued ? 'Training starts on its own when the earlier training finishes. One training runs at a time; preparations do not hold it up.' : stepCopy[run.step ?? ''] ?? 'This training keeps going while you browse.'}</p>
        {progress && <p className="live-count"><strong>{progress.completed} of {progress.total}</strong> {run.step === 'train' ? 'training steps' : run.step === 'evaluate' ? 'answers · the base model answers every question, then the adapter' : ''}{settings && run.step === 'train' ? ` · up to ${settings.maximum_epochs} ${settings.maximum_epochs === 1 ? 'epoch' : 'epochs'}` : ''}</p>}
      </div>
      <div className="live-steps">{chips}</div>
      <div className="live-foot">
        <p>{[queued ? `Queued ${duration(now - Date.parse(run.started_at))} ago` : `Started ${duration(now - Date.parse(run.started_at))} ago`, `${trainedOn(run)}${run.training_dataset_version ? ` v${run.training_dataset_version}` : ''}`, baseModelName].join(' · ')}</p>
        <div className="live-controls">
          {run.control_stage === 'training' && <button className="btn btn-secondary btn-compact" type="button" disabled={disabled} title={paused || pausing ? 'Continue this run without restarting optimization' : 'Pause between batches or operations; model memory stays reserved'} onClick={() => void control(paused || pausing ? 'resume' : 'pause')}>
            {busy === 'pause' || busy === 'resume' ? 'Sending…' : paused || pausing ? 'Resume' : 'Pause'}
          </button>}
          <button className="btn btn-secondary btn-compact btn-quiet-danger" type="button" disabled={disabled} onClick={() => setConfirmStop(true)}>Stop training…</button>
        </div>
      </div>
      {confirmStop && <div className="prep-confirm live-confirm" role="group" aria-label="Confirm training cancellation">
        <p><strong>Stop this training?</strong> This cancels the run, not pauses it. It cannot be resumed. The dataset and earlier adapters stay unchanged.</p>
        <div className="prep-actions"><button className="btn btn-danger" type="button" disabled={disabled} onClick={() => void stop()}>{busy === 'stop' ? 'Stopping…' : 'Stop run'}</button><button className="btn btn-secondary" type="button" disabled={!!busy} onClick={() => setConfirmStop(false)}>Keep this run</button></div>
      </div>}
    </section>;
  }

  // The same details panel as a preparation run: the evaluation first once there is one.
  const tabs: [string, string][] = [
    ...(issues.length ? [['issues', 'Issues'] as [string, string]] : []),
    ...(run.evaluation_available ? [['evaluation', 'Evaluation'] as [string, string]] : []),
    ...(active(run) ? [] : [['steps', 'Step history'] as [string, string]]),
    ['settings', 'Training settings'],
  ];
  const shownTab = tabs.some(([key]) => key === detailTab) ? detailTab : issues.length && (run.status === 'failed' || notAccepted(run)) ? 'issues' : run.evaluation_available && outcome && outcome !== 'not_compared' ? 'evaluation' : active(run) ? 'settings' : 'steps';
  return <div className="prep-detail prep-run-detail">
    {run.state_stale && active(run) && <p className="prep-warning" role="status">Showing the last saved status{run.observed_at ? ` from ${date(run.observed_at)}` : ''}. The training worker is not reporting right now.</p>}
    {active(run) && notice && <p className="prep-warning" role="status">{notice}</p>}
    {active(run) && actionError && <p className="fine-error" role="alert">{actionError}</p>}
    {card}
    <DetailTabs tabs={tabs} shown={shownTab} onSelect={setDetailTab}>
      {shownTab === 'issues' && <RunIssues key={runKey(run)} run={run} issues={issues} evaluation={evaluation} />}
      {shownTab === 'evaluation' && <EvaluationResults evaluation={evaluation} />}
      {shownTab === 'steps' && <ol className="run-timeline">{trainingSteps.map((name, at) => {
        const state = stepState(name);
        const step = steps.find((item) => item.name === name);
        const detail = state === 'completed' ? `Took ${span(step?.started_at, step?.closed_at)}` : state === 'failed' ? `Failed ${date(step?.closed_at ?? run.closed_at)}${step && step.attempt > 1 ? ` after ${step.attempt} attempts` : ''}` : state === 'stopped' ? `Stopped ${date(run.closed_at)}` : 'Not started';
        return <li key={name} className={`run-step run-step-${state} fine-step-${state === 'stopped' ? 'failed' : state}`}>
          <span className="fine-step-marker" aria-hidden="true">{state === 'pending' ? at + 1 : stepGlyphs[state]}</span>
          <div className="run-step-body"><div className="run-step-title"><strong>{trainingStepLabels[name]}</strong><span className="run-step-state">{stepStateLabels[state]}</span></div><span className="prep-sub">{detail}</span></div>
        </li>;
      })}</ol>}
      {shownTab === 'settings' && <dl className="prep-facts">{([
        ['Dataset', trainedOn(run)], ['Version', run.training_dataset_version ? `Version ${run.training_dataset_version}` : '—'], ['Base model', settings?.model ?? FINE_TUNING_MODEL_ID],
        ...settingFields.map(([key, label]) => [label, settings ? settings[key] : '—']),
        ['Date created', date(run.started_at)], ['Updated', date(run.updated_at)], ['Run ID', `${run.workflow_id} · ${run.run_id}`],
      ] as [string, string | number][]).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>}
    </DetailTabs>
  </div>;
}

function EvaluationResults({ evaluation }: { evaluation: ReturnType<typeof useEvaluation> }) {
  const { report, error, retry } = evaluation;
  if (error) return <>
    <p className="fine-error" role="alert">Could not load evaluation results: {error}</p>
    <button className="btn btn-secondary" type="button" onClick={retry}>Try again</button>
  </>;
  if (!report) return <p className="prep-copy" role="status">Loading evaluation results…</p>;
  if (report.total === 0) return <p className="prep-copy">This dataset had no evaluation questions, so the adapter was trained without comparing it with the base model. Add documents to the dataset to get evaluation questions for the next training.</p>;
  return <><dl className="ds-stats">
    <div><dt>Base model correct</dt><dd>{report.base_correct} / {report.total}</dd></div>
    <div><dt>Adapter correct</dt><dd className={report.accepted ? 'ds-ok' : ''}>{report.adapter_correct} / {report.total}</dd></div>
    <div><dt>Result</dt><dd>{report.accepted ? 'Accepted' : 'Needs improvement'}</dd></div>
  </dl><EvaluationChecks report={report} />
    <div className="run-evaluation-answers"><h3>Evaluation answers</h3><p className="prep-copy">Compare the expected answer with the base model and adapter. Older reports may not include individual pass results.</p>{report.answers.map((answer, index) => <details className="run-evaluation-answer" key={index}>
      <summary>{answer.adapter_passed === undefined ? '' : answer.adapter_passed ? '✓ Passed · ' : '× Did not pass · '}{answer.prompt}</summary>
      <dl><div><dt>Expected answer</dt><dd>{answer.expected}</dd></div><div><dt>Base model</dt><dd>{answer.base_answer || '(Empty answer)'}</dd></div><div><dt>Adapter</dt><dd>{answer.adapter_answer || '(Empty answer)'}</dd></div></dl>
    </details>)}</div></>;
}
