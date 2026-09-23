import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  DEFAULT_FINE_TUNING_DATASET_INSTRUCTIONS,
  FINE_TUNING_MODEL_ID,
  MAX_FINE_TUNING_COLLECTION_BYTES,
  MAX_FINE_TUNING_DATASET_INSTRUCTIONS,
  MAX_FINE_TUNING_DOCUMENTS,
  MAX_FINE_TUNING_PREVIEW_RECORDS,
  MAX_FINE_TUNING_UPLOAD_BYTES,
  isTerminalFineTuningStatus,
  preparedFineTuningDatasetError,
  useFineTuningRunner,
} from '../contexts/FineTuningRunnerContext';
import { useModelList } from '../contexts/ModelListContext';
import { api } from '../services/api';
import type {
  FineTuningGoal,
  FineTuningPreparationProgress,
  FineTuningRecipeResponse,
  FineTuningRun,
} from '../types';
import KeyValueTable from './KeyValueTable';
import { FieldLabel, labelWithTip } from './ParamTooltips';

type View = 'configure' | 'run' | 'history' | 'prepare';

function formatDate(value?: string): string {
  if (!value || value.startsWith('0001-')) return '—';
  return new Date(value).toLocaleString();
}

function statusLabel(status: string): string {
  return status.replaceAll('_', ' ');
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

  useEffect(() => {
    if (!active) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [active]);

  const percent = total > 0 ? Math.min(100, Math.round((completed / total) * 100)) : 0;
  const started = Date.parse(startedAt);
  const stopped = active ? now : endedAt ? Date.parse(endedAt) : now;
  const elapsed = Number.isFinite(started) && Number.isFinite(stopped) ? Math.max(0, stopped - started) : 0;

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
  sourceNames = [],
  onCancel,
}: {
  title: string;
  message: string;
  progress: FineTuningPreparationProgress | null;
  startedAt: string;
  cancelLabel: string;
  sourceNames?: string[];
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
      {!!progress?.documents?.length && (
        <div className="fine-document-progress" aria-label="Document extraction progress">
          {progress.documents.map((document, index) => (
            <div className={`fine-document-progress-row ${document.status}`} key={`${document.name}-${index}`}>
              <span className="fine-document-progress-mark" aria-hidden="true">
                {document.status === 'completed' ? '✓' : document.status === 'failed' ? '!' : document.status === 'stopped' ? '×' : index + 1}
              </span>
              <span className="fine-document-progress-name" title={document.name}>{document.name}</span>
              <span className="fine-document-progress-stage">
                {document.cached ? 'Cached extraction' : document.error || document.stage}
                {document.target_facts ? ` · ${document.accepted_facts ?? 0}/${document.target_facts} facts` : ''}
                {document.total_batches ? ` · batch ${document.completed_batches ?? 0}/${document.total_batches}` : ''}
              </span>
            </div>
          ))}
        </div>
      )}
      {!progress?.documents?.length && sourceNames.length > 0 && (
        <div className="fine-document-progress" aria-label="Source documents included in dataset build">
          {sourceNames.map((name, index) => (
            <div className="fine-document-progress-row completed" key={`${name}-${index}`}>
              <span className="fine-document-progress-mark" aria-hidden="true">✓</span>
              <span className="fine-document-progress-name" title={name}>{name}</span>
              <span className="fine-document-progress-stage">Waiting</span>
            </div>
          ))}
        </div>
      )}
      <button className="btn btn-danger" onClick={onCancel}>{cancelLabel}</button>
    </section>
  );
}

function RunStatus({ run, busy, onStop }: { run: FineTuningRun; busy: boolean; onStop: () => void }) {
  const active = !isTerminalFineTuningStatus(run.status);

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
  const [preparationModelIDs, setPreparationModelIDs] = useState<string[]>([]);
  const [preparationModelsLoading, setPreparationModelsLoading] = useState(true);
  const [preparationModelsError, setPreparationModelsError] = useState('');
  const [trainingRecipe, setTrainingRecipe] = useState<FineTuningRecipeResponse | null>(null);
  const [trainingRecipeLoading, setTrainingRecipeLoading] = useState(false);
  const [trainingRecipeError, setTrainingRecipeError] = useState('');
  const {
    sourceFiles,
    preparedFile,
    datasetID,
    sourceName,
    goal,
    preparationModelID,
    previewCount,
    customDatasetInstructions,
    datasetInstructions,
    documentPreviews,
    datasetBuilt,
    currentRun,
    activeRun,
    history,
    busy,
    preparationOperation,
    preparationProgress,
    preparationStartedAt,
    error,
    setSourceFiles,
    setGoal,
    setPreparationModelID,
    setPreviewCount,
    setCustomDatasetInstructions,
    setDatasetInstructions,
    setError,
    choosePreparedDataset,
    preparePreview,
    regenerateDocumentPreview,
    updateRecord,
    removeDocumentPreview,
    buildDataset,
    returnToSourceSetup,
    resetPreparation,
    cancelPreparation,
    startRun,
    stopRun,
    resumeRun,
    selectRun,
    refreshRuns,
  } = useFineTuningRunner();
  const [searchParams, setSearchParams] = useSearchParams();
  const [activeDocumentIndex, setActiveDocumentIndex] = useState(0);
  const [view, setView] = useState<View>(() => {
    const requested = searchParams.get('view');
    if (requested === 'prepare' || requested === 'run') return requested;
    return preparationOperation ? 'prepare' : activeRun ? 'run' : datasetBuilt ? 'configure' : 'prepare';
  });
  const sourceInputRef = useRef<HTMLInputElement>(null);
  const preparedInputRef = useRef<HTMLInputElement>(null);
  const trainingModelInstalled = installedModels.some((model) => model.id === FINE_TUNING_MODEL_ID);

  useEffect(() => {
    void loadModels();
  }, [loadModels]);

  useEffect(() => {
    const requested = searchParams.get('view');
    if (requested !== 'prepare' && requested !== 'run') return;
    setView(requested);
    setSearchParams({}, { replace: true });
  }, [searchParams, setSearchParams]);

  useEffect(() => {
    let cancelled = false;
    api.fineTuningPreparationModels()
      .then((response) => {
        if (!cancelled) setPreparationModelIDs(response.models ?? []);
      })
      .catch((err) => {
        if (!cancelled) setPreparationModelsError(err instanceof Error ? err.message : 'Could not load preparation models');
      })
      .finally(() => {
        if (!cancelled) setPreparationModelsLoading(false);
      });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    if (!preparationModelIDs.includes(preparationModelID)) {
      setPreparationModelID(preparationModelIDs[0] ?? '');
    }
  }, [preparationModelID, preparationModelIDs]);

  useEffect(() => {
    if (!trainingModelInstalled) {
      setTrainingRecipe(null);
      setTrainingRecipeError('');
      setTrainingRecipeLoading(false);
      return;
    }
    let cancelled = false;
    setTrainingRecipeLoading(true);
    setTrainingRecipeError('');
    api.fineTuningRecipe(FINE_TUNING_MODEL_ID)
      .then((recipe) => {
        if (!cancelled) setTrainingRecipe(recipe);
      })
      .catch((err) => {
        if (!cancelled) {
          setTrainingRecipe(null);
          setTrainingRecipeError(err instanceof Error ? err.message : 'Could not load training recipe');
        }
      })
      .finally(() => {
        if (!cancelled) setTrainingRecipeLoading(false);
      });
    return () => { cancelled = true; };
  }, [trainingModelInstalled]);

  useEffect(() => {
    setActiveDocumentIndex((current) => Math.max(0, Math.min(current, documentPreviews.length - 1)));
  }, [documentPreviews.length]);

  const activeDocument = documentPreviews[activeDocumentIndex];
  const stage = preparationOperation === 'build' ? 2 : documentPreviews.length > 0 ? 1 : 0;
  const sourceFileError = useMemo(() => {
    if (sourceFiles.length === 0) return '';
    if (sourceFiles.length > MAX_FINE_TUNING_DOCUMENTS) return `Choose no more than ${MAX_FINE_TUNING_DOCUMENTS} documents.`;
    for (const file of sourceFiles) {
      const extension = file.name.split('.').pop()?.toLowerCase();
      if (extension !== 'pdf' && extension !== 'docx') return `${file.name} must be a PDF or DOCX document.`;
      if (file.size > MAX_FINE_TUNING_UPLOAD_BYTES) return `${file.name} exceeds the 100 MiB limit.`;
    }
    if (sourceFiles.reduce((total, file) => total + file.size, 0) > MAX_FINE_TUNING_COLLECTION_BYTES) return 'The selected documents exceed the 500 MiB combined limit.';
    return '';
  }, [sourceFiles]);

  const preparedFileError = useMemo(() => {
    if (!preparedFile) return '';
    return preparedFineTuningDatasetError(preparedFile);
  }, [preparedFile]);

  const cancel = () => {
    const operation = preparationOperation;
    cancelPreparation();
    if (operation !== 'prepared-upload') setView('prepare');
  };

  const build = async () => {
    if (await buildDataset()) setView('configure');
  };

  const start = async () => {
    if (await startRun()) setView('run');
  };

  const resume = async (id: string) => {
    if (await resumeRun(id)) setView('run');
  };

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
          <button key={item} className={`tab ${view === item ? 'active' : ''}`} disabled={
            preparationOperation !== null
            && (preparationOperation !== 'build' || (item !== 'prepare' && item !== 'history'))
          } onClick={() => {
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
      {preparationModelsError && <div className="fine-error" role="alert">Could not load preparation models from the catalog: {preparationModelsError}</div>}

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
                <button className="btn btn-primary" onClick={resetPreparation}>Change dataset</button>
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
                        <button className="btn btn-danger" onClick={cancel}>Cancel Upload</button>
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
              <div><strong>Gemma 3 1B IT</strong><code>{FINE_TUNING_MODEL_ID}</code></div>
              <span className={`fine-status ${trainingModelInstalled ? 'fine-status-completed' : 'fine-status-stopped'}`}>{modelsLoading ? 'Checking' : trainingModelInstalled ? 'Installed' : 'Not installed'}</span>
            </div>
          </section>

          <section className="fine-panel fine-recipe-panel">
            <div className="fine-panel-heading"><div><span>Validated preset</span><h3>Training Configuration</h3></div></div>
            <p className="fine-panel-copy">This validated preset comes directly from the GoMLX backend. Training uses each example's actual tokenized length up to the model's context limit.</p>
            {trainingRecipeLoading ? (
              <p className="fine-panel-copy">Loading training settings…</p>
            ) : trainingRecipeError ? (
              <div className="fine-inline-error">Could not load training settings: {trainingRecipeError}</div>
            ) : !trainingRecipe ? (
              <p className="fine-panel-copy">Install Gemma 3 1B IT to inspect its complete training recipe.</p>
            ) : (
              <>
                <div className="fine-recipe-columns">
                  <KeyValueTable rows={[
                    { key: 'method', label: labelWithTip('Training Method', 'fineTuningTrainingMethod'), value: trainingRecipe.method === 'lora' ? 'LoRA' : statusLabel(trainingRecipe.method) },
                    { key: 'epochs', label: labelWithTip('Maximum Epochs', 'fineTuningMaximumEpochs'), value: trainingRecipe.maximum_epochs },
                    { key: 'batch-size', label: labelWithTip('Batch Size', 'fineTuningBatchSize'), value: `${trainingRecipe.batch_size} examples` },
                    { key: 'learning-rate', label: labelWithTip('Learning Rate', 'fineTuningLearningRate'), value: trainingRecipe.learning_rate },
                  ]} />
                  <KeyValueTable rows={[
                    { key: 'rank', label: labelWithTip('LoRA Rank', 'fineTuningLoRARank'), value: trainingRecipe.lora.rank },
                    { key: 'sequence-limit', label: labelWithTip('Sequence Limit', 'fineTuningSequenceLimit'), value: `${trainingRecipe.sequence.maximum_tokens.toLocaleString()} tokens · model GGUF` },
                    { key: 'checkpoint', label: labelWithTip('Checkpoint Selection', 'fineTuningCheckpointSelection'), value: trainingRecipe.checkpoint_selection === 'lowest_validation_loss' ? 'Lowest validation loss' : statusLabel(trainingRecipe.checkpoint_selection) },
                    { key: 'output', label: labelWithTip('Output Adapter', 'fineTuningOutputAdapter'), value: `${trainingRecipe.output.precision.toUpperCase()} ${trainingRecipe.output.format.toUpperCase()} adapter` },
                  ]} />
                </div>
                <details className="fine-recipe-details">
                  <summary>Advanced Recipe Details</summary>
                  <div className="fine-recipe-columns">
                    <KeyValueTable rows={[
                      { key: 'optimizer', label: labelWithTip('Optimizer', 'fineTuningOptimizer'), value: trainingRecipe.optimizer === 'adam' ? 'Adaptive Moment Estimation' : statusLabel(trainingRecipe.optimizer) },
                      { key: 'weight-decay', label: labelWithTip('Weight Decay', 'fineTuningWeightDecay'), value: trainingRecipe.weight_decay },
                      { key: 'alpha', label: labelWithTip('LoRA Alpha', 'fineTuningLoRAAlpha'), value: trainingRecipe.lora.alpha },
                      { key: 'scale', label: labelWithTip('LoRA Scale', 'fineTuningLoRAScale'), value: trainingRecipe.lora.scale },
                      { key: 'target-modules', label: labelWithTip('Target Modules', 'fineTuningTargetModules'), value: `${trainingRecipe.lora.target_modules.join(', ')} projections` },
                    ]} />
                    <KeyValueTable rows={[
                      { key: 'base-weights', label: labelWithTip('Base Model Weights', 'fineTuningBaseWeights'), value: trainingRecipe.base_weights === 'frozen' ? 'Frozen' : statusLabel(trainingRecipe.base_weights) },
                      { key: 'loss', label: labelWithTip('Loss', 'fineTuningLoss'), value: trainingRecipe.loss === 'answer_only_cross_entropy' ? 'Answer-only cross-entropy' : statusLabel(trainingRecipe.loss) },
                      { key: 'seed', label: labelWithTip('Shuffle Seed', 'fineTuningShuffleSeed'), value: trainingRecipe.seed },
                      { key: 'oversized', label: labelWithTip('Oversized Examples', 'fineTuningOversizedExamples'), value: trainingRecipe.sequence.overflow === 'reject' ? 'Rejected' : statusLabel(trainingRecipe.sequence.overflow) },
                      { key: 'padding', label: labelWithTip('Batch Padding', 'fineTuningBatchPadding'), value: trainingRecipe.sequence.padding === 'power_of_two' ? 'Power-of-two sequence buckets' : statusLabel(trainingRecipe.sequence.padding) },
                    ]} />
                  </div>
                </details>
              </>
            )}
          </section>

          <section className="fine-panel fine-generation-summary">
            <div>
              <span className={`fine-status ${datasetBuilt && trainingModelInstalled ? 'fine-status-completed' : 'fine-status-stopped'}`}>{datasetBuilt && trainingModelInstalled ? 'Ready' : 'Action needed'}</span>
              <h3>{datasetBuilt && trainingModelInstalled ? 'Ready to Fine-tune' : 'Complete the Training Configuration'}</h3>
              <p>{datasetBuilt && trainingModelInstalled ? `${sourceName || datasetID} will train against Gemma 3 1B IT.` : 'Choose a prepared dataset and install the supported Gemma 3 1B model.'}</p>
            </div>
            <button className="btn btn-primary" disabled={!datasetBuilt || !datasetID || !trainingModelInstalled || busy || !!activeRun} onClick={start}>Start fine-tuning</button>
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
                  <FieldLabel htmlFor="fine-source" tooltipKey="fineTuningSource">Source Documents</FieldLabel>
                  <input ref={sourceInputRef} className="fine-hidden-input" id="fine-source" type="file" multiple accept=".pdf,.docx,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document" onChange={(event) => {
                    const selected = Array.from(event.target.files ?? []);
                    const known = new Set(sourceFiles.map((file) => `${file.name}:${file.size}:${file.lastModified}`));
                    setSourceFiles([...sourceFiles, ...selected.filter((file) => {
                      const key = `${file.name}:${file.size}:${file.lastModified}`;
                      if (known.has(key)) return false;
                      known.add(key);
                      return true;
                    })]);
                    event.currentTarget.value = '';
                  }} />
                  <div className="fine-file-picker">
                    <button type="button" className="btn btn-secondary" onClick={() => sourceInputRef.current?.click()}>{sourceFiles.length ? 'Add Documents' : 'Choose Documents'}</button>
                    <span>{sourceFiles.length ? `${sourceFiles.length} document${sourceFiles.length === 1 ? '' : 's'} selected` : 'No documents chosen'}</span>
                  </div>
                  {!!sourceFiles.length && (
                    <ul className="fine-source-files">
                      {sourceFiles.map((file, index) => (
                        <li key={`${file.name}-${file.size}-${file.lastModified}`}>
                          <span><strong title={file.name}>{file.name}</strong><small>{(file.size / 1024 / 1024).toFixed(1)} MiB</small></span>
                          <button type="button" className="btn btn-small btn-secondary" onClick={() => setSourceFiles(sourceFiles.filter((_, fileIndex) => fileIndex !== index))}>Remove</button>
                        </li>
                      ))}
                    </ul>
                  )}
                  <small>{sourceFiles.length ? `${(sourceFiles.reduce((total, file) => total + file.size, 0) / 1024 / 1024).toFixed(1)} MiB combined` : `PDF or DOCX · up to ${MAX_FINE_TUNING_DOCUMENTS} documents · 100 MiB each`}</small>
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
                  <select id="fine-model" value={preparationModelID} onChange={(event) => setPreparationModelID(event.target.value)} disabled={preparationModelsLoading}>
                    {preparationModelsLoading && <option>Loading models…</option>}
                    {!preparationModelsLoading && preparationModelIDs.length === 0 && <option value="">No compatible catalog models installed</option>}
                    {preparationModelIDs.map((modelID) => <option key={modelID} value={modelID}>{modelID}</option>)}
                  </select>
                </div>
                <div className="form-group">
                  <FieldLabel htmlFor="fine-preview-count" tooltipKey="fineTuningPreviewCount">Preview Examples</FieldLabel>
                  <input id="fine-preview-count" type="number" min={1} max={MAX_FINE_TUNING_PREVIEW_RECORDS} value={previewCount} onChange={(event) => setPreviewCount(Math.min(MAX_FINE_TUNING_PREVIEW_RECORDS, Math.max(1, Number(event.target.value))))} />
                </div>
              </div>
              <div className="fine-prompt-editor">
                <div className="fine-prompt-heading">
                  <div>
                    <h4>Dataset Instructions <span>Optional</span></h4>
                    <p>Describe the audience, topics, question style, and answer detail you want in the prepared examples.</p>
                  </div>
                </div>
                <FieldLabel className="fine-prompt-toggle" tooltipKey="fineTuningDatasetInstructions">
                  <input type="checkbox" checked={customDatasetInstructions} onChange={(event) => setCustomDatasetInstructions(event.target.checked)} />
                  <span>Customize how examples are written</span>
                </FieldLabel>
                {!customDatasetInstructions && (
                  <p className="fine-prompt-default">Using Kronk’s grounded defaults for natural, standalone examples.</p>
                )}
                {customDatasetInstructions && (
                  <div className="form-group fine-prompt-custom">
                    <FieldLabel htmlFor="fine-preparation-instructions">Custom Instructions</FieldLabel>
                    <textarea
                      id="fine-preparation-instructions"
                      rows={4}
                      maxLength={MAX_FINE_TUNING_DATASET_INSTRUCTIONS}
                      value={datasetInstructions}
                      onChange={(event) => setDatasetInstructions(event.target.value)}
                    />
                    <div className="fine-prompt-footer">
                      <span>{datasetInstructions.length} / {MAX_FINE_TUNING_DATASET_INSTRUCTIONS} characters</span>
                      <button type="button" className="btn-link" onClick={() => setDatasetInstructions(DEFAULT_FINE_TUNING_DATASET_INSTRUCTIONS)}>Restore Default Text</button>
                    </div>
                  </div>
                )}
                <details className="fine-prompt-rules">
                  <summary>View Default Instructions and Required Rules</summary>
                  <div className="fine-prompt-rules-content">
                    <strong>Default Instructions</strong>
                    <p className="fine-prompt-default-copy">{DEFAULT_FINE_TUNING_DATASET_INSTRUCTIONS}</p>
                    <strong>Required Rules</strong>
                    <p>These protections always apply, even when custom instructions are used.</p>
                    <ul>
                      <li>Use only facts found in the selected source passage.</li>
                      <li>Keep every question understandable without the original document.</li>
                      <li>Require one directly supported answer and reject outside knowledge.</li>
                      <li>Verify grounding and dataset quality before accepting an example.</li>
                    </ul>
                  </div>
                </details>
              </div>
              {sourceFileError && <div className="fine-inline-error">{sourceFileError}</div>}
              <div className="fine-actions">
                <button className="btn btn-primary" disabled={sourceFiles.length === 0 || !!sourceFileError || !preparationModelID || busy} onClick={preparePreview}>Generate Preview</button>
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
              onCancel={cancel}
            />
          )}

          {stage === 1 && preparationOperation !== 'preview' && activeDocument && (
            <section className="fine-panel fine-review-panel">
              <button
                className="btn btn-small btn-secondary"
                style={{ marginBottom: 12 }}
                disabled={busy}
                onClick={returnToSourceSetup}
              >
                ← Back to Source Setup
              </button>
              <div className="fine-panel-heading">
                <div><span>Step 2 · Document {activeDocumentIndex + 1} of {documentPreviews.length}</span><h3>Review Examples</h3></div>
                <strong>{activeDocument.records.length} Records</strong>
              </div>
              <div className="fine-document-review-nav">
                <button className="btn btn-small btn-secondary" disabled={activeDocumentIndex === 0 || busy} onClick={() => setActiveDocumentIndex((index) => index - 1)}>← Previous</button>
                <div><strong title={activeDocument.name}>{activeDocument.name}</strong><span>Review this document’s examples independently.</span></div>
                <button className="btn btn-small btn-secondary" disabled={activeDocumentIndex === documentPreviews.length - 1 || busy} onClick={() => setActiveDocumentIndex((index) => index + 1)}>Next →</button>
              </div>
              <p className="fine-panel-copy">Edit prompts or targets before building. Evidence stays read-only so each target remains traceable to the source.</p>
              <div className="fine-records">
                {activeDocument.records.map((record, index) => {
                  const result = activeDocument.verification[index];
                  return (
                    <article className="fine-record" key={`${record.fact_id ?? 'record'}-${index}`}>
                      <div className="fine-record-number">{String(index + 1).padStart(2, '0')}</div>
                      <div className="fine-record-fields">
                        <FieldLabel tooltipKey="fineTuningPrompt">{goal === 'question_answer' ? 'Question' : 'Prompt'}<textarea value={record.prompt} onChange={(event) => updateRecord(activeDocument.id, index, 'prompt', event.target.value)} /></FieldLabel>
                        <FieldLabel tooltipKey="fineTuningTarget">{goal === 'question_answer' ? 'Answer' : 'Target'}<textarea value={record.target} onChange={(event) => updateRecord(activeDocument.id, index, 'target', event.target.value)} /></FieldLabel>
                        <details><summary>Source evidence</summary><p>{record.evidence}</p></details>
                      </div>
                      <span className={`fine-verification ${result?.passed ? 'passed' : 'review'}`}>{result?.passed ? 'Verified' : 'Review'}</span>
                    </article>
                  );
                })}
              </div>
              <div className="fine-review-actions">
                <button className="btn btn-secondary" disabled={busy} onClick={() => regenerateDocumentPreview(activeDocument.id)}>Regenerate This Preview</button>
                <button className="btn btn-danger" disabled={busy} onClick={() => removeDocumentPreview(activeDocument.id)}>Remove Document</button>
                <button className="btn btn-primary" disabled={busy || documentPreviews.some((preview) => preview.records.some((record) => !record.prompt.trim() || !record.target.trim()))} onClick={build}>Build Training Dataset</button>
              </div>
            </section>
          )}

          {preparationOperation === 'build' && (
            <PreparationProgress
              title="Building Training Dataset"
              message="Generating and validating one training dataset from all selected documents."
              progress={preparationProgress}
              startedAt={preparationStartedAt}
              cancelLabel="Stop Build"
              sourceNames={documentPreviews.map((document) => document.name)}
              onCancel={cancel}
            />
          )}
        </div>
      )}

      {view === 'run' && (
        <div className="fine-train-view">
          {currentRun && <div className="fine-actions"><button className="btn btn-primary" onClick={() => setView('history')}>Back to History</button></div>}
          {currentRun ? <RunStatus run={currentRun} busy={busy} onStop={stopRun} /> : (
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
                    <td><button className="fine-run-link" onClick={() => { selectRun(run); setView('run'); }}>{run.id}</button></td>
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
