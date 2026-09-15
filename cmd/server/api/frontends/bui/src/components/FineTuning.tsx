import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useModelList } from '../contexts/ModelListContext';
import { api } from '../services/api';
import type {
  FineTuningGoal,
  FineTuningPreparationProgress,
  FineTuningRecord,
  FineTuningRun,
  FineTuningVerification,
} from '../types';
import { FieldLabel } from './ParamTooltips';

type View = 'configure' | 'run' | 'history' | 'prepare';
type PreparationOperation = 'prepared-upload' | 'preview' | 'build';

const MAX_UPLOAD_BYTES = 100 * 1024 * 1024;
const MAX_PREVIEW_RECORDS = 5;
const TRAINING_MODEL_ID = 'ggml-org/gemma-3-1b-it-Q8_0';
const terminalStatuses = new Set(['completed', 'needs_improvement', 'failed', 'stopped']);

function formatDate(value?: string): string {
  if (!value || value.startsWith('0001-')) return '—';
  return new Date(value).toLocaleString();
}

function statusLabel(status: string): string {
  return status.replaceAll('_', ' ');
}

function preparedDatasetError(file: File): string {
  if (file.name.split('.').pop()?.toLowerCase() !== 'jsonl') return 'Choose a training-ready JSONL dataset.';
  if (file.size > MAX_UPLOAD_BYTES) return 'The selected file exceeds the 100 MiB limit.';
  return '';
}

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === 'AbortError';
}

function formatDuration(milliseconds: number): string {
  const seconds = Math.max(0, Math.ceil(milliseconds / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) return `${minutes}m ${remainingSeconds}s`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ${minutes % 60}m`;
}

function ProgressMetrics({
  stage,
  completed,
  total,
  startedAt,
  endedAt,
  active,
  acceptedRecords,
}: {
  stage: string;
  completed: number;
  total: number;
  startedAt: string;
  endedAt?: string;
  active: boolean;
  acceptedRecords?: number;
}) {
  const [now, setNow] = useState(Date.now());
  const samples = useRef<Array<{ completed: number; at: number }>>([]);

  useEffect(() => {
    if (!active) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [active]);

  useEffect(() => {
    if (!active || completed <= 0 || total <= 0) return;
    const last = samples.current.at(-1);
    if (last && completed <= last.completed) return;
    samples.current.push({ completed, at: Date.now() });
    if (samples.current.length > 6) samples.current.shift();
  }, [active, completed, total]);

  const percent = total > 0 ? Math.min(100, Math.round((completed / total) * 100)) : 0;
  const started = Date.parse(startedAt);
  const stopped = active ? now : endedAt ? Date.parse(endedAt) : now;
  const elapsed = Number.isFinite(started) && Number.isFinite(stopped) ? Math.max(0, stopped - started) : 0;
  let remaining = active ? 'Estimating…' : '—';
  const first = samples.current[0];
  const last = samples.current.at(-1);
  if (active && first && last && last.completed > first.completed && last.at > first.at && completed < total) {
    const millisecondsPerUnit = (last.at - first.at) / (last.completed - first.completed);
    remaining = formatDuration(millisecondsPerUnit * (total - completed));
  } else if (active && total > 0 && completed >= total) {
    remaining = 'Finishing…';
  }

  return (
    <>
      <div className="fine-progress-track" role="progressbar" aria-label={`${percent}% complete`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}>
        <span style={{ width: `${percent}%` }} />
      </div>
      <div className="fine-run-facts">
        <span><strong>Current Stage</strong>{statusLabel(stage)}</span>
        <span><strong>Completed / Total</strong>{total > 0 ? `${completed} / ${total}` : `${completed} / —`}</span>
        <span><strong>Percentage</strong>{total > 0 ? `${percent}%` : 'Estimating…'}</span>
        <span><strong>Elapsed Time</strong>{formatDuration(elapsed)}</span>
        <span><strong>Estimated Remaining</strong>{remaining}</span>
        {!!acceptedRecords && <span><strong>Verified Records</strong>{acceptedRecords}</span>}
      </div>
    </>
  );
}

function PreparationProgress({
  title,
  message,
  progress,
  startedAt,
  cancelLabel,
  onCancel,
}: {
  title: string;
  message: string;
  progress: FineTuningPreparationProgress | null;
  startedAt: string;
  cancelLabel: string;
  onCancel: () => void;
}) {
  const stage = progress?.stage || message;
  const completed = progress?.completed ?? 0;
  const total = progress?.total ?? 0;
  return (
    <section className="fine-panel fine-preparation-progress" aria-live="polite">
      <div>
        <span className="fine-status fine-status-preparing">In Progress</span>
        <h3>{title}</h3>
        <p>{message}</p>
      </div>
      <ProgressMetrics
        key={progress?.operation ?? title}
        stage={stage}
        completed={completed}
        total={total}
        startedAt={progress?.started_at ?? startedAt}
        active
        acceptedRecords={progress?.accepted_records}
      />
      <button className="btn btn-danger" onClick={onCancel}>{cancelLabel}</button>
    </section>
  );
}

function RunStatus({ run, busy, onStop }: { run: FineTuningRun; busy: boolean; onStop: () => void }) {
  const active = !terminalStatuses.has(run.status);

  return (
    <section className="fine-run-card" aria-live="polite">
      <div className="fine-run-heading">
        <div>
          <span className={`fine-status fine-status-${run.status}`}>{statusLabel(run.status)}</span>
          <h3>{run.progress.message || 'Fine-tuning Run'}</h3>
        </div>
        <code>{run.id}</code>
      </div>
      <ProgressMetrics
        key={`${run.id}:${run.progress.stage}`}
        stage={run.progress.stage || run.status}
        completed={run.progress.completed}
        total={run.progress.total}
        startedAt={run.started_at}
        endedAt={run.finished_at || run.updated_at}
        active={active}
      />
      <div className="fine-run-facts">
        <span><strong>Dataset</strong>{run.dataset_id}</span>
        <span><strong>Model</strong>{run.model}</span>
        <span><strong>Final Adapter</strong>{run.has_adapter ? 'adapter-f32.gguf' : '—'}</span>
      </div>
      {run.error && <div className="fine-error">{run.error}</div>}
      {active && <div className="fine-actions fine-actions-end"><button className="btn btn-danger" disabled={busy} onClick={onStop}>Stop Run</button></div>}
    </section>
  );
}

export default function FineTuning() {
  const { models, loading: modelsLoading, error: modelsError, loadModels } = useModelList();
  const installedModels = useMemo(() => models?.data ?? [], [models]);
  const [configuredModelIDs, setConfiguredModelIDs] = useState<string[]>([]);
  const [configuredModelsLoading, setConfiguredModelsLoading] = useState(true);
  const [configuredModelsError, setConfiguredModelsError] = useState('');
  const [view, setView] = useState<View>('prepare');
  const [sourceFile, setSourceFile] = useState<File | null>(null);
  const [preparedFile, setPreparedFile] = useState<File | null>(null);
  const [datasetID, setDatasetID] = useState('');
  const [sourceName, setSourceName] = useState('');
  const [goal, setGoal] = useState<FineTuningGoal>('question_answer');
  const [preparationModelID, setPreparationModelID] = useState('');
  const [previewCount, setPreviewCount] = useState(MAX_PREVIEW_RECORDS);
  const [records, setRecords] = useState<FineTuningRecord[]>([]);
  const [verifiedRecords, setVerifiedRecords] = useState<FineTuningRecord[]>([]);
  const [verification, setVerification] = useState<FineTuningVerification[]>([]);
  const [datasetBuilt, setDatasetBuilt] = useState(false);
  const [currentRun, setCurrentRun] = useState<FineTuningRun | null>(null);
  const [history, setHistory] = useState<FineTuningRun[]>([]);
  const [busy, setBusy] = useState(false);
  const [preparationOperation, setPreparationOperation] = useState<PreparationOperation | null>(null);
  const [preparationProgress, setPreparationProgress] = useState<FineTuningPreparationProgress | null>(null);
  const [preparationStartedAt, setPreparationStartedAt] = useState('');
  const [error, setError] = useState('');
  const preparedInputRef = useRef<HTMLInputElement>(null);
  const preparationAbortRef = useRef<AbortController | null>(null);
  const trainingModelInstalled = installedModels.some((model) => model.id === TRAINING_MODEL_ID);
  const preparationModels = useMemo(() => {
    const configured = new Set(configuredModelIDs);
    return installedModels.filter((model) => configured.has(model.id));
  }, [configuredModelIDs, installedModels]);

  useEffect(() => {
    void loadModels();
  }, [loadModels]);

  useEffect(() => {
    let cancelled = false;
    api.fineTuningPreparationModels()
      .then((response) => {
        if (!cancelled) setConfiguredModelIDs(response.models ?? []);
      })
      .catch((err) => {
        if (!cancelled) setConfiguredModelsError(err instanceof Error ? err.message : 'Could not load configured models');
      })
      .finally(() => {
        if (!cancelled) setConfiguredModelsLoading(false);
      });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    if (!preparationModels.some((model) => model.id === preparationModelID)) {
      setPreparationModelID(preparationModels[0]?.id ?? '');
    }
  }, [preparationModelID, preparationModels]);

  const refreshRuns = useCallback(async (showError = false) => {
    try {
      const [current, saved] = await Promise.all([
        api.currentFineTuningRun().catch(() => null),
        api.fineTuningHistory(),
      ]);
      setCurrentRun(current);
      setHistory(saved.runs ?? []);
    } catch (err) {
      if (showError) setError(err instanceof Error ? err.message : 'Could not load fine-tuning runs');
    }
  }, []);

  useEffect(() => {
    void refreshRuns(false);
  }, [refreshRuns]);

  useEffect(() => () => preparationAbortRef.current?.abort(), []);

  useEffect(() => {
    if (!currentRun || terminalStatuses.has(currentRun.status)) return;
    const timer = window.setInterval(() => void refreshRuns(false), 2000);
    return () => window.clearInterval(timer);
  }, [currentRun, refreshRuns]);

  useEffect(() => {
    if (!preparationOperation) return;
    let cancelled = false;
    const refresh = async () => {
      try {
        const progress = await api.currentFineTuningPreparation();
        const matches = preparationOperation === 'preview'
          ? progress.operation === 'extraction' || progress.operation === 'preview'
          : progress.operation === preparationOperation;
        if (!cancelled && matches) setPreparationProgress(progress);
      } catch {
        // The operation may still be parsing its request before progress exists.
      }
    };
    void refresh();
    const timer = window.setInterval(() => void refresh(), 1000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [preparationOperation]);

  const stage = preparationOperation === 'build' ? 2 : records.length > 0 ? 1 : 0;
  const sourceFileError = useMemo(() => {
    if (!sourceFile) return '';
    const extension = sourceFile.name.split('.').pop()?.toLowerCase();
    if (extension !== 'pdf' && extension !== 'docx') return 'Choose a PDF or DOCX document.';
    if (sourceFile.size > MAX_UPLOAD_BYTES) return 'The selected file exceeds the 100 MiB limit.';
    return '';
  }, [sourceFile]);

  const preparedFileError = useMemo(() => {
    if (!preparedFile) return '';
    return preparedDatasetError(preparedFile);
  }, [preparedFile]);

  const runAction = async (action: () => Promise<void>) => {
    setBusy(true);
    setError('');
    try {
      await action();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'The request failed');
    } finally {
      setBusy(false);
    }
  };

  const runPreparationAction = async (
    operation: PreparationOperation,
    action: (signal: AbortSignal) => Promise<void>,
  ) => {
    const controller = new AbortController();
    preparationAbortRef.current = controller;
    setPreparationOperation(operation);
    setPreparationProgress(null);
    setPreparationStartedAt(new Date().toISOString());
    setBusy(true);
    setError('');
    try {
      await action(controller.signal);
    } catch (err) {
      if (!isAbortError(err)) setError(err instanceof Error ? err.message : 'The request failed');
    } finally {
      if (preparationAbortRef.current === controller) {
        preparationAbortRef.current = null;
        setPreparationOperation(null);
        setBusy(false);
      }
    }
  };

  const cancelPreparation = () => {
    const operation = preparationOperation;
    void api.stopFineTuningPreparation().catch(() => undefined);
    preparationAbortRef.current?.abort();
    setPreparationOperation(null);
    setPreparationProgress(null);
    setBusy(false);
    setError('');
    if (operation === 'prepared-upload') {
      setPreparedFile(null);
      return;
    }
    setRecords([]);
    setVerifiedRecords([]);
    setVerification([]);
    setDatasetBuilt(false);
    setView('prepare');
  };

  const uploadPrepared = (selectedFile: File) => runPreparationAction('prepared-upload', async (signal) => {
    if (preparedDatasetError(selectedFile)) return;
    const dataset = await api.uploadFineTuningDataset(selectedFile, signal);
    setDatasetID(dataset.dataset_id);
    setSourceName(dataset.source_name);
    setRecords([]);
    setVerifiedRecords([]);
    setVerification([]);
    setDatasetBuilt(true);
  });

  const choosePreparedDataset = (selectedFile: File | null) => {
    setPreparedFile(selectedFile);
    if (selectedFile && !preparedDatasetError(selectedFile)) void uploadPrepared(selectedFile);
  };

  const preparePreview = () => runPreparationAction('preview', async (signal) => {
    if (!sourceFile || sourceFileError || !preparationModelID) return;
    const dataset = datasetID
      ? { dataset_id: datasetID, source_name: sourceName }
      : await api.uploadFineTuningDataset(sourceFile, signal);
    setDatasetID(dataset.dataset_id);
    setSourceName(dataset.source_name);
    const result = await api.previewFineTuningDataset({
      dataset_id: dataset.dataset_id,
      goal,
      model_id: preparationModelID,
      record_count: previewCount,
    }, signal);
    setRecords(result.records);
    setVerifiedRecords(result.records.map((record) => ({ ...record })));
    setVerification(result.verification);
    setDatasetBuilt(false);
  });

  const updateRecord = (index: number, field: 'prompt' | 'target', value: string) => {
    setRecords((current) => current.map((record, recordIndex) =>
      recordIndex === index ? { ...record, [field]: value } : record));
  };

  const build = () => runPreparationAction('build', async (signal) => {
    await api.buildFineTuningDataset({
      dataset_id: datasetID,
      goal,
      model_id: preparationModelID,
      reviewed_records: records,
      verified_preview_records: verifiedRecords,
    }, signal);
    setDatasetBuilt(true);
    setView('configure');
  });

  const start = () => runAction(async () => {
    const run = await api.startFineTuningRun(datasetID, TRAINING_MODEL_ID);
    setCurrentRun(run);
    setView('run');
    await refreshRuns(false);
  });

  const stop = () => runAction(async () => {
    await api.stopFineTuningRun();
    await refreshRuns(true);
  });

  const resume = (id: string) => runAction(async () => {
    const run = await api.resumeFineTuningRun(id);
    setCurrentRun(run);
    setView('run');
    await refreshRuns(false);
  });

  return (
    <div className="fine-page">
      <div className="page-header fine-page-header">
        <div>
          <span className="fine-eyebrow">Local adaptation workbench</span>
          <h2>Fine-Tuning</h2>
          <p>Configure a local training run from a prepared dataset, follow its progress, and keep the resulting adapter history.</p>
        </div>
      </div>

      <div className="tabs fine-tabs" role="tablist" aria-label="Fine-tuning views">
        {(['prepare', 'configure', 'run', 'history'] as View[]).map((item) => (
          <button key={item} className={`tab ${view === item ? 'active' : ''}`} disabled={preparationOperation !== null} onClick={() => {
            setView(item);
            setError('');
            if (item === 'history') void refreshRuns(true);
          }}>
            {item === 'prepare' ? 'Prepare Dataset' : item === 'configure' ? 'Configure Training' : item === 'run' ? 'Training Progress' : 'History'}
            {item === 'history' && history.length > 0 && <span className="fine-tab-count">{history.length}</span>}
          </button>
        ))}
      </div>

      {error && <div className="fine-error" role="alert">{error}</div>}
      {modelsError && <div className="fine-error" role="alert">Could not load installed models: {modelsError}</div>}
      {configuredModelsError && <div className="fine-error" role="alert">Could not load models from the active configuration: {configuredModelsError}</div>}

      {view === 'configure' && (
        <div className="fine-configure-view">
          <section className="fine-panel fine-configure-intro">
            <div className="fine-panel-heading">
              <div><span>Required</span><h3>Choose Training Dataset</h3></div>
              <span className={`fine-status ${datasetBuilt ? 'fine-status-completed' : 'fine-status-stopped'}`}>{datasetBuilt ? 'Ready' : 'Dataset needed'}</span>
            </div>
            {datasetBuilt ? (
              <div className="fine-dataset-ready">
                <div><strong>{sourceName || datasetID}</strong><span>Validated and stored for local fine-tuning</span></div>
                <button className="btn btn-primary" onClick={() => {
                  setDatasetBuilt(false);
                  setDatasetID('');
                  setSourceName('');
                  setSourceFile(null);
                  setPreparedFile(null);
                  setRecords([]);
                  setVerifiedRecords([]);
                  setVerification([]);
                }}>Change dataset</button>
              </div>
            ) : (
              <>
                <p className="fine-panel-copy">Upload a training-ready JSONL file, or prepare one from a raw PDF or DOCX.</p>
                <div className="fine-dataset-choice">
                  <div className="form-group fine-file-field">
                    <FieldLabel htmlFor="fine-prepared-dataset" tooltipKey="fineTuningDataset">Training-Ready JSONL</FieldLabel>
                    <input ref={preparedInputRef} className="fine-hidden-input" id="fine-prepared-dataset" type="file" accept=".jsonl,application/x-ndjson" onChange={(event) => {
                      choosePreparedDataset(event.target.files?.[0] ?? null);
                      event.currentTarget.value = '';
                    }} />
                    <small>{preparedFile ? `${preparedFile.name} · ${(preparedFile.size / 1024 / 1024).toFixed(1)} MiB` : 'Training-ready JSONL · 100 MiB maximum'}</small>
                  </div>
                  <div className="fine-actions">
                    {preparationOperation === 'prepared-upload' ? (
                      <div className="fine-upload-progress" aria-live="polite">
                        <ProgressMetrics
                          key={preparationProgress?.operation ?? 'prepared-upload'}
                          stage={preparationProgress?.stage ?? 'Uploading prepared dataset'}
                          completed={preparationProgress?.completed ?? 0}
                          total={preparationProgress?.total ?? 0}
                          startedAt={preparationProgress?.started_at ?? preparationStartedAt}
                          active
                        />
                        <button className="btn btn-danger" onClick={cancelPreparation}>Cancel Upload</button>
                      </div>
                    ) : (
                      <>
                        <button className="btn btn-primary" disabled={busy} onClick={() => preparedInputRef.current?.click()}>Choose JSONL Dataset</button>
                        <span className="fine-choice-separator">or</span>
                        <button className="btn btn-primary" disabled={busy} onClick={() => setView('prepare')}>Prepare Raw Dataset</button>
                      </>
                    )}
                  </div>
                </div>
                {preparedFileError && <div className="fine-inline-error">{preparedFileError}</div>}
              </>
            )}
          </section>

          <section className="fine-panel">
            <div className="fine-panel-heading"><div><span>Required</span><h3>Base Model</h3></div></div>
            <p className="fine-panel-copy">Fine-tuning currently uses the single validated Gemma 3 1B recipe.</p>
            <div className="fine-supported-model">
              <div><strong>Gemma 3 1B IT</strong><code>{TRAINING_MODEL_ID}</code></div>
              <span className={`fine-status ${trainingModelInstalled ? 'fine-status-completed' : 'fine-status-stopped'}`}>{modelsLoading ? 'Checking' : trainingModelInstalled ? 'Installed' : 'Not installed'}</span>
            </div>
          </section>

          <section className="fine-panel fine-recipe-panel">
            <div className="fine-panel-heading"><div><span>Validated preset</span><h3>Training Configuration</h3></div><strong>LoRA</strong></div>
            <p className="fine-panel-copy">Moxie chooses the validated recipe for the selected model. These are safeguards, not extra inputs.</p>
            <div className="fine-recipe-grid">
              <span><strong>Method</strong>LoRA</span>
              <span><strong>Selection</strong>Early stopping</span>
              <span><strong>Context</strong>512 tokens</span>
              <span><strong>Output</strong>F32 GGUF adapter</span>
            </div>
          </section>

          <section className="fine-panel fine-generation-summary">
            <div>
              <span className={`fine-status ${datasetBuilt && trainingModelInstalled ? 'fine-status-completed' : 'fine-status-stopped'}`}>{datasetBuilt && trainingModelInstalled ? 'Ready' : 'Action needed'}</span>
              <h3>{datasetBuilt && trainingModelInstalled ? 'Ready to Fine-tune' : 'Complete the Training Configuration'}</h3>
              <p>{datasetBuilt && trainingModelInstalled ? `${sourceName || datasetID} will train against Gemma 3 1B IT.` : 'Choose a prepared dataset and install the supported Gemma 3 1B model.'}</p>
            </div>
            <button className="btn btn-primary" disabled={!datasetBuilt || !datasetID || !trainingModelInstalled || busy || (!!currentRun && !terminalStatuses.has(currentRun.status))} onClick={start}>Start fine-tuning</button>
          </section>
        </div>
      )}

      {view === 'prepare' && (
        <div className="fine-workbench">
          {stage === 0 && preparationOperation !== 'preview' && (
            <section className="fine-prepare-skip">
              <div><strong>Already have a prepared dataset?</strong><span>Skip preparation and configure fine-tuning directly.</span></div>
              <button className="btn btn-primary" onClick={() => setView('configure')}>Go Straight to Configure Training</button>
            </section>
          )}
          <ol className="fine-stage-rail" aria-label="Dataset preparation stages">
            {['Extract Source', 'Review Examples', 'Build Dataset'].map((label, index) => (
              <li key={label} className={stage > index ? 'complete' : stage === index ? 'active' : ''}>
                <span>{stage > index ? '✓' : index + 1}</span><strong>{label}</strong>
              </li>
            ))}
          </ol>

          {stage === 0 && preparationOperation !== 'preview' && (
            <section className="fine-panel">
              <div className="fine-panel-heading">
                <div><span>Step 1</span><h3>Prepare Grounded Examples</h3></div>
                {datasetID && <code>{datasetID}</code>}
              </div>
              <div className="fine-form-grid">
                <div className="form-group fine-file-field">
                  <FieldLabel htmlFor="fine-source" tooltipKey="fineTuningSource">Source Document</FieldLabel>
                  <input id="fine-source" type="file" accept=".pdf,.docx,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document" onChange={(event) => {
                    setSourceFile(event.target.files?.[0] ?? null);
                    setDatasetID('');
                    setSourceName('');
                    setDatasetBuilt(false);
                  }} />
                  <small>{sourceFile ? `${sourceFile.name} · ${(sourceFile.size / 1024 / 1024).toFixed(1)} MiB` : 'PDF or DOCX · 100 MiB maximum'}</small>
                </div>
                <div className="form-group">
                  <FieldLabel htmlFor="fine-goal" tooltipKey="fineTuningGoal">Training Goal</FieldLabel>
                  <select id="fine-goal" value={goal} onChange={(event) => setGoal(event.target.value as FineTuningGoal)}>
                    <option value="question_answer">Question answering</option>
                    <option value="classification">Classification</option>
                    <option value="numeric_prediction">Numeric prediction</option>
                    <option value="text_generation">Text generation</option>
                  </select>
                </div>
                <div className="form-group">
                  <FieldLabel htmlFor="fine-model" tooltipKey="fineTuningModel">Model</FieldLabel>
                  <select id="fine-model" value={preparationModelID} onChange={(event) => setPreparationModelID(event.target.value)} disabled={modelsLoading || configuredModelsLoading}>
                    {(modelsLoading || configuredModelsLoading) && <option>Loading models…</option>}
                    {!modelsLoading && !configuredModelsLoading && configuredModelIDs.length === 0 && <option value="">No models in model config</option>}
                    {!modelsLoading && !configuredModelsLoading && configuredModelIDs.length > 0 && preparationModels.length === 0 && <option value="">No configured models installed</option>}
                    {preparationModels.map((model) => <option key={model.id} value={model.id}>{model.id}</option>)}
                  </select>
                </div>
                <div className="form-group">
                  <FieldLabel htmlFor="fine-preview-count" tooltipKey="fineTuningPreviewCount">Preview Examples</FieldLabel>
                  <input id="fine-preview-count" type="number" min={1} max={MAX_PREVIEW_RECORDS} value={previewCount} onChange={(event) => setPreviewCount(Math.min(MAX_PREVIEW_RECORDS, Math.max(1, Number(event.target.value))))} />
                </div>
              </div>
              {sourceFileError && <div className="fine-inline-error">{sourceFileError}</div>}
              <div className="fine-actions">
                <button className="btn btn-primary" disabled={!sourceFile || !!sourceFileError || !preparationModelID || busy} onClick={preparePreview}>Generate Preview</button>
                {sourceName && <span className="fine-source-ready">Source Ready · {sourceName}</span>}
              </div>
            </section>
          )}

          {preparationOperation === 'preview' && (
            <PreparationProgress
              title="Preparing Dataset Preview"
              message="Extracting the source and generating up to five grounded examples for review."
              progress={preparationProgress}
              startedAt={preparationStartedAt}
              cancelLabel="Cancel Preparation"
              onCancel={cancelPreparation}
            />
          )}

          {stage === 1 && (
            <section className="fine-panel fine-review-panel">
              <div className="fine-panel-heading"><div><span>Step 2</span><h3>Review Examples</h3></div><strong>{records.length} Records</strong></div>
              <p className="fine-panel-copy">Edit prompts or targets before building. Evidence stays read-only so each target remains traceable to the source.</p>
              <div className="fine-records">
                {records.map((record, index) => {
                  const result = verification[index];
                  return (
                    <article className="fine-record" key={`${record.fact_id ?? 'record'}-${index}`}>
                      <div className="fine-record-number">{String(index + 1).padStart(2, '0')}</div>
                      <div className="fine-record-fields">
                        <FieldLabel tooltipKey="fineTuningPrompt">Prompt<textarea value={record.prompt} onChange={(event) => updateRecord(index, 'prompt', event.target.value)} /></FieldLabel>
                        <FieldLabel tooltipKey="fineTuningTarget">Target<textarea value={record.target} onChange={(event) => updateRecord(index, 'target', event.target.value)} /></FieldLabel>
                        <details><summary>Source evidence</summary><p>{record.evidence}</p></details>
                      </div>
                      <span className={`fine-verification ${result?.passed ? 'passed' : 'review'}`}>{result?.passed ? 'Verified' : 'Review'}</span>
                    </article>
                  );
                })}
              </div>
              <div className="fine-actions fine-actions-end"><button className="btn btn-primary" disabled={busy || records.some((record) => !record.prompt.trim() || !record.target.trim())} onClick={build}>Build Training Dataset</button></div>
            </section>
          )}

          {preparationOperation === 'build' && (
            <PreparationProgress
              title="Building Training Dataset"
              message="Generating and validating the complete dataset from your reviewed examples."
              progress={preparationProgress}
              startedAt={preparationStartedAt}
              cancelLabel="Stop Build"
              onCancel={cancelPreparation}
            />
          )}
        </div>
      )}

      {view === 'run' && (
        <div className="fine-train-view">
          {currentRun && <div className="fine-actions"><button className="btn btn-primary" onClick={() => setView('history')}>Back to History</button></div>}
          {currentRun ? <RunStatus run={currentRun} busy={busy} onStop={stop} /> : (
            <section className="fine-empty-run">
              <div className="fine-empty-symbol">→</div>
              <div><h3>No Training Run Yet</h3><p>Configure a prepared dataset and base model, then start fine-tuning.</p></div>
              <button className="btn btn-primary fine-empty-action" onClick={() => setView('configure')}>Configure fine-tuning</button>
            </section>
          )}
        </div>
      )}

      {view === 'history' && (
        <section className="fine-panel fine-history">
          <div className="fine-panel-heading">
            <div><span>Archive</span><h3>Training History</h3></div>
            <button className="btn btn-primary" disabled={busy} onClick={() => void refreshRuns(true)}>Refresh</button>
          </div>
          {history.length === 0 ? <div className="fine-history-empty"><strong>No saved runs</strong><span>Completed, stopped, and recoverable runs will appear here.</span></div> : (
            <div className="fine-history-table-wrap">
              <table className="fine-history-table">
                <thead><tr><th>Run</th><th>Status</th><th>Started</th><th /></tr></thead>
                <tbody>{history.map((run) => (
                  <tr key={run.id}>
                    <td><button className="fine-run-link" onClick={() => { setCurrentRun(run); setView('run'); }}>{run.id}</button></td>
                    <td><span className={`fine-status fine-status-${run.status}`}>{statusLabel(run.status)}</span></td>
                    <td>{formatDate(run.started_at)}</td>
                    <td>{run.resumable && <button className="btn btn-primary" disabled={busy} onClick={() => resume(run.id)}>Resume</button>}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </section>
      )}
    </div>
  );
}
