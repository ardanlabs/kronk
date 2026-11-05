import { useEffect, useRef, useState, type FormEvent } from 'react';
import { useSearchParams } from 'react-router-dom';
import { FINE_TUNING_MODEL_ID, TESTED_TRAINING_OPTIONS, useFineTuningRunner } from '../contexts/FineTuningRunnerContext';
import { useModelList } from '../contexts/ModelListContext';
import { api } from '../services/api';
import type { CatalogModelResponse, PreparationOptions, TrainingOptions } from '../types';
import { FieldLabel } from './ParamTooltips';
import TrainingHistory from './TrainingHistory';
import { TrainingJobState } from './TrainingRun';

type View = 'configure' | 'run' | 'history';

export default function FineTuning() {
  const { models, loading: modelsLoading, error: modelsError, loadModels } = useModelList();
  const {
    inputKind, preparedFile, upload, uploading, submitting, currentRun, error, watchError, preparationActivity, trainingActivity,
    chooseInput, cancelPreparation, startRun, selectRun, refreshRun,
  } = useFineTuningRunner();
  const [training, setTraining] = useState<TrainingOptions>(TESTED_TRAINING_OPTIONS);
  const [preparation, setPreparation] = useState<PreparationOptions>({ model: '', goal: 'question_answer', instructions: '' });
  const [catalog, setCatalog] = useState<CatalogModelResponse[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(false);
  const [catalogError, setCatalogError] = useState('');
  const [catalogAttempt, setCatalogAttempt] = useState(0);
  const [view, setView] = useState<View>('configure');
  const [searchParams, setSearchParams] = useSearchParams();
  const preparedInput = useRef<HTMLInputElement>(null);
  const busy = uploading || submitting;
  const preparing = inputKind === 'document';
  const modelInstalled = !!models?.data.some((model) => model.id === FINE_TUNING_MODEL_ID);
  const preparationModels = catalog.filter((model) => model.capabilities?.endpoint === 'chat_completion');
  const preparationModelInstalled = preparationModels.some((model) => model.id === preparation.model && model.downloaded);
  const ready = !!upload && modelInstalled && (!preparing || (!catalogLoading && !catalogError && preparationModelInstalled));

  useEffect(() => { void loadModels(); }, [loadModels]);

  useEffect(() => {
    if (!preparing) return;
    let cancelled = false;
    setCatalogLoading(true);
    setCatalogError('');
    api.listCatalog()
      .then((result) => { if (!cancelled) setCatalog(result); })
      .catch((err) => { if (!cancelled) setCatalogError(err instanceof Error ? err.message : 'Could not load preparation models'); })
      .finally(() => { if (!cancelled) setCatalogLoading(false); });
    return () => { cancelled = true; };
  }, [preparing, catalogAttempt]);

  useEffect(() => {
    const requested = searchParams.get('view');
    if (requested !== 'configure' && requested !== 'run' && requested !== 'history') return;
    setView(requested);
    setSearchParams({}, { replace: true });
  }, [searchParams, setSearchParams]);

  const start = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!ready) return;
    if (await startRun(training, preparing ? preparation : undefined)) setView('run');
  };

  const updateTraining = (key: keyof TrainingOptions, value: number) => {
    setTraining((current) => ({ ...current, [key]: value }));
  };

  return (
    <div className="fine-page">
      <div className="page-header fine-page-header">
        <div>
          <span className="fine-eyebrow">Local adaptation workbench</span>
          <h2>Fine-Tuning</h2>
          <p>Prepare a document or upload a prepared dataset, then follow the job’s stages and steps.</p>
        </div>
      </div>

      <div className="tabs fine-tabs" role="tablist" aria-label="Fine-tuning views">
        {(['configure', 'run', 'history'] as View[]).map((item) => (
          <button key={item} className={`tab ${view === item ? 'active' : ''}`} onClick={() => setView(item)}>
            {item === 'configure' ? 'Configure Training' : item === 'run' ? 'Training Progress' : 'History'}
          </button>
        ))}
      </div>

      {error && <div className="fine-error" role="alert">{error}</div>}
      {modelsError && <div className="fine-error" role="alert">Could not load installed models: {modelsError}</div>}

      {view === 'configure' && (
        <form className="fine-configure-view" onSubmit={start}>
          <section className="fine-panel fine-configure-intro">
            <div className="form-group">
              <FieldLabel htmlFor="fine-input-kind" tooltipKey="fineTuningInputKind">Starting Material</FieldLabel>
              <select id="fine-input-kind" className="form-input" value={inputKind} disabled={submitting} onChange={(event) => void chooseInput(null, event.target.value as 'dataset' | 'document')}>
                <option value="dataset">Prepared JSONL dataset</option>
                <option value="document">Prepare documents</option>
              </select>
            </div>
            <div className="fine-panel-heading">
              <div><span>Required</span><h3>{preparing ? 'Choose Source Document' : 'Choose Training Dataset'}</h3></div>
              <span className={`fine-status ${upload ? 'fine-status-completed' : 'fine-status-stopped'}`}>{upload ? 'Uploaded' : preparing ? 'Document needed' : 'Dataset needed'}</span>
            </div>
            <p className="fine-panel-copy">{preparing ? 'Choose a PDF, DOCX, Markdown, or text document. Review examples in Preview before continuing preparation of the full dataset.' : 'Choose a training-ready JSONL file. The training service validates its contents when you select Start.'}</p>
            <div className="form-group fine-file-field">
              <FieldLabel htmlFor="fine-prepared-dataset" tooltipKey={preparing ? 'fineTuningSource' : 'fineTuningDataset'}>{preparing ? 'Source Document' : 'Training-Ready JSONL'}</FieldLabel>
              <input ref={preparedInput} className="fine-hidden-input" id="fine-prepared-dataset" type="file" accept={preparing ? '.pdf,.docx,.md,.txt' : '.jsonl,application/x-ndjson'} disabled={submitting} onChange={(event) => {
                void chooseInput(event.target.files?.[0] ?? null, inputKind);
                event.currentTarget.value = '';
              }} />
            </div>
            {upload ? (
              <div className="fine-dataset-ready">
                <div><strong>{upload.name}</strong><span>Stored · {(upload.size_bytes / 1024).toFixed(1)} KiB · {preparing ? 'not yet prepared' : 'not yet validated for training'}</span></div>
                <button className="btn btn-secondary" type="button" disabled={submitting} onClick={() => void chooseInput(null, inputKind)}>{preparing ? 'Change document' : 'Change dataset'}</button>
              </div>
            ) : (
              <div className="fine-actions">
                {uploading ? (
                  <>
                    <p role="status">Uploading {preparedFile?.name}…</p>
                    <button className="btn btn-danger" type="button" onClick={cancelPreparation}>Cancel Upload</button>
                  </>
                ) : (
                  <button className="btn btn-primary" type="button" disabled={submitting} onClick={() => preparedInput.current?.click()}>{preparing ? 'Choose Document' : 'Choose JSONL Dataset'}</button>
                )}
              </div>
            )}
            {preparing && <p className="fine-action-note">Preparation finishes before you choose whether to enter Training. PDF/DOCX require a configured server-side document extractor; text and Markdown can be read directly.</p>}
          </section>

          {preparing && (
            <section className="fine-panel" aria-label="Preparation settings">
              <div className="fine-panel-heading"><div><span>Preparation</span><h3>Prepare Examples</h3></div></div>
              <p className="fine-panel-copy">Choose a catalog model to generate and verify source-grounded examples. This is separate from the Gemma base model used for adapter training.</p>
              {catalogError && <div className="fine-error" role="alert">Could not load preparation models: {catalogError} <button className="btn btn-secondary" type="button" onClick={() => setCatalogAttempt((attempt) => attempt + 1)}>Retry Catalog</button></div>}
              {catalogLoading && <p role="status">Loading preparation models…</p>}
              {!catalogLoading && !catalogError && !preparationModels.some((model) => model.downloaded) && <p role="status">No installed preparation models are available. Install a chat model from the Catalog, then select Refresh Catalog.</p>}
              <div className="form-group">
                <FieldLabel htmlFor="fine-preparation-model" tooltipKey="fineTuningPreparationModel">Preparation Model</FieldLabel>
                <select id="fine-preparation-model" className="form-input" required value={preparation.model} disabled={busy || catalogLoading || !!catalogError} onChange={(event) => setPreparation((current) => ({ ...current, model: event.target.value }))}>
                  <option value="">Select a preparation model</option>
                  {preparationModels.map((model) => <option key={model.id} value={model.id} disabled={!model.downloaded}>{model.id} · {model.downloaded ? 'Installed' : 'Not installed'}</option>)}
                </select>
                <button className="btn btn-secondary" type="button" disabled={busy || catalogLoading} onClick={() => setCatalogAttempt((attempt) => attempt + 1)}>Refresh Catalog</button>
              </div>
              <div className="form-group">
                <FieldLabel htmlFor="fine-preparation-goal" tooltipKey="fineTuningGoal">Preparation Goal</FieldLabel>
                <select id="fine-preparation-goal" className="form-input" value={preparation.goal} disabled={busy} onChange={(event) => setPreparation((current) => ({ ...current, goal: event.target.value as PreparationOptions['goal'] }))}>
                  <option value="question_answer">Question answering</option>
                  <option value="classification">Classification</option>
                  <option value="numeric_prediction">Numeric prediction</option>
                  <option value="text_generation">Text generation</option>
                </select>
              </div>
              <div className="form-group">
                <FieldLabel htmlFor="fine-preparation-instructions" tooltipKey="fineTuningDatasetInstructions">Preparation Instructions (optional)</FieldLabel>
                <textarea id="fine-preparation-instructions" className="form-input" rows={4} value={preparation.instructions} disabled={busy} onChange={(event) => setPreparation((current) => ({ ...current, instructions: event.target.value }))} />
              </div>
            </section>
          )}

          <section className="fine-panel">
            <div className="fine-panel-heading"><div><span>Required</span><h3>Base Model</h3></div></div>
            <p className="fine-panel-copy">Gemma 270M is the only model exercised end-to-end so far. Adapter quality is evaluated separately for every job.</p>
            <div className="fine-supported-model">
              <div><strong>Gemma 3 270M IT · Q8_0</strong><code>{FINE_TUNING_MODEL_ID}</code></div>
              <span className={`fine-status ${modelInstalled ? 'fine-status-completed' : 'fine-status-stopped'}`}>{modelsLoading ? 'Checking' : modelInstalled ? 'Installed' : 'Not installed'}</span>
            </div>
          </section>

          <section className="fine-panel fine-recipe-panel">
            <div className="fine-panel-heading"><div><span>Settings</span><h3>Training Configuration</h3></div></div>
            <p className="fine-panel-copy">Starting values match the tested Gemma 270M run. The training service validates submitted settings.{preparing && ' These settings are stored with the job; Training still requires a separate choice after the full dataset is prepared.'}</p>
            <div className="fine-recipe-columns">
              <div className="form-group">
                <FieldLabel htmlFor="fine-maximum-epochs" tooltipKey="fineTuningMaximumEpochs">Maximum Epochs</FieldLabel>
                <input id="fine-maximum-epochs" className="form-input" type="number" required step={1} value={training.maximum_epochs} disabled={busy} onChange={(event) => updateTraining('maximum_epochs', event.target.valueAsNumber)} />
              </div>
              <div className="form-group">
                <FieldLabel htmlFor="fine-learning-rate" tooltipKey="fineTuningLearningRate">Learning Rate</FieldLabel>
                <input id="fine-learning-rate" className="form-input" type="number" required step="any" value={training.learning_rate} disabled={busy} onChange={(event) => updateTraining('learning_rate', event.target.valueAsNumber)} />
              </div>
              <div className="form-group">
                <FieldLabel htmlFor="fine-batch-size" tooltipKey="fineTuningBatchSize">Batch Size</FieldLabel>
                <input id="fine-batch-size" className="form-input" type="number" required step={1} value={training.batch_size} disabled={busy} onChange={(event) => updateTraining('batch_size', event.target.valueAsNumber)} />
              </div>
            </div>
            <details className="fine-recipe-details">
              <summary>Advanced Settings</summary>
              <div className="fine-recipe-columns">
                <div className="form-group">
                  <FieldLabel htmlFor="fine-lora-rank" tooltipKey="fineTuningLoRARank">LoRA Rank</FieldLabel>
                  <input id="fine-lora-rank" className="form-input" type="number" required step={1} value={training.lora_rank} disabled={busy} onChange={(event) => updateTraining('lora_rank', event.target.valueAsNumber)} />
                </div>
                <div className="form-group">
                  <FieldLabel htmlFor="fine-lora-alpha" tooltipKey="fineTuningLoRAAlpha">LoRA Alpha</FieldLabel>
                  <input id="fine-lora-alpha" className="form-input" type="number" required step="any" value={training.lora_alpha} disabled={busy} onChange={(event) => updateTraining('lora_alpha', event.target.valueAsNumber)} />
                </div>
                <div className="form-group">
                  <FieldLabel htmlFor="fine-random-seed" tooltipKey="fineTuningShuffleSeed">Random Seed</FieldLabel>
                  <input id="fine-random-seed" className="form-input" type="number" required step={1} value={training.seed} disabled={busy} onChange={(event) => updateTraining('seed', event.target.valueAsNumber)} />
                </div>
              </div>
              <div className="fine-actions"><button className="btn btn-secondary" type="button" disabled={busy} onClick={() => setTraining(TESTED_TRAINING_OPTIONS)}>Reset to Tested Settings</button></div>
            </details>
          </section>

          <section className="fine-panel fine-generation-summary">
            <div><h3>{ready ? 'Ready to Submit' : preparing ? 'Complete the Preparation Configuration' : 'Complete the Training Configuration'}</h3><p>{preparing ? 'Start Preparation extracts the document and creates examples for Preview. Continue completes the dataset; Training remains a separate choice.' : upload ? `${upload.name} will be submitted for Gemma 3 270M training.` : 'Choose a prepared JSONL dataset and install the tested Gemma 270M model.'}</p></div>
            <button className="btn btn-primary" type="submit" disabled={!ready || busy || !!preparationActivity || !!trainingActivity}>{submitting ? 'Submitting…' : preparing ? 'Start Preparation' : 'Start Training'}</button>
          </section>
        </form>
      )}

      {view === 'run' && (
        <div className="fine-train-view">
          {watchError && <div className="fine-error" role="alert">{watchError}</div>}
          {currentRun ? (
            <>
              <TrainingJobState run={currentRun} onRefresh={() => void refreshRun()} />
              <p className="fine-action-note">Stop and Resume are not connected yet. Job state is reported by the training service.</p>
            </>
          ) : (
            <section className="fine-empty-run"><div><h3>No Training Job Selected</h3><p>Start training or select a job from History.</p></div><button className="btn btn-primary" onClick={() => setView('history')}>View History</button></section>
          )}
        </div>
      )}
      {view === 'history' && <TrainingHistory onSelect={(run) => { selectRun(run); setView('run'); }} />}
    </div>
  );
}
