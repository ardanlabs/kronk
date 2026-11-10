import { useEffect, useRef, useState, type FormEvent } from 'react';
import { useSearchParams } from 'react-router-dom';
import { FINE_TUNING_MODEL_ID, TESTED_TRAINING_OPTIONS, useFineTuningRunner } from '../contexts/FineTuningRunnerContext';
import { useModelList } from '../contexts/ModelListContext';
import { api } from '../services/api';
import type { CatalogModelResponse, PreparationOptions, TrainingOptions } from '../types';
import { FieldLabel } from './ParamTooltips';
import TrainingHistory from './TrainingHistory';
import { StartPreparedTraining, TrainingJobState } from './TrainingRun';

type View = 'prepare' | 'configure' | 'history';

export default function FineTuning() {
  const { models, loading: modelsLoading, error: modelsError, loadModels } = useModelList();
  const {
    inputKind, preparedFile, upload, uploading, submitting, trainingRun, preparationRun, error, watchError, preparationActivity, trainingActivity,
    chooseInput, cancelPreparation, startRun, selectRun, refreshRun,
  } = useFineTuningRunner();
  const [training, setTraining] = useState<TrainingOptions>(TESTED_TRAINING_OPTIONS);
  const [preparation, setPreparation] = useState<PreparationOptions>({ model: '', goal: 'question_answer', instructions: '', preview_records: 3 });
  const [catalog, setCatalog] = useState<CatalogModelResponse[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(false);
  const [catalogError, setCatalogError] = useState('');
  const [catalogAttempt, setCatalogAttempt] = useState(0);
  const [view, setView] = useState<View>('prepare');
  const [sourceSetup, setSourceSetup] = useState(false);
  const [trainingSetup, setTrainingSetup] = useState(false);
  const [searchParams, setSearchParams] = useSearchParams();
  const preparedInput = useRef<HTMLInputElement>(null);
  const busy = uploading || submitting;
  const preparing = view === 'prepare';
  const showSourceSetup = !preparationRun || sourceSetup;
  const trainingJob = trainingRun ?? (preparationRun?.stage === 'training' ? preparationRun : null);
  const showTrainingSetup = !trainingJob || trainingSetup;
  const matchingUpload = inputKind === (preparing ? 'document' : 'dataset');
  const modelInstalled = !!models?.data.some((model) => model.id === FINE_TUNING_MODEL_ID);
  const preparationModels = catalog.filter((model) => model.downloaded && model.validated
    && model.capabilities?.endpoint === 'chat_completion'
    && !model.capabilities.images && !model.capabilities.audio && !model.capabilities.video
    && !model.capabilities.embedding && !model.capabilities.rerank && !model.capabilities.decision);
  const preparationModelInstalled = preparationModels.some((model) => model.id === preparation.model);
  const ready = matchingUpload && !!upload && modelInstalled && (!preparing || (!catalogLoading && !catalogError && preparationModelInstalled));

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
    if (requested !== 'prepare' && requested !== 'configure' && requested !== 'run' && requested !== 'history') return;
    setView(requested === 'run' ? 'configure' : requested);
    setSearchParams({}, { replace: true });
  }, [searchParams, setSearchParams]);

  const start = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!ready) return;
    if (await startRun(training, preparing ? preparation : undefined)) {
      setSourceSetup(false);
      setTrainingSetup(false);
    }
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
        {(['prepare', 'configure', 'history'] as View[]).map((item) => (
          <button key={item} role="tab" aria-selected={view === item} className={`tab ${view === item ? 'active' : ''}`} onClick={() => setView(item)}>
            {item === 'prepare' ? 'Prepare Dataset' : item === 'configure' ? 'Configure Training' : 'History'}
          </button>
        ))}
      </div>

      {error && <div className="fine-error" role="alert">{error}</div>}
      {modelsError && <div className="fine-error" role="alert">Could not load installed models: {modelsError}</div>}

      {((view === 'prepare' && showSourceSetup) || (view === 'configure' && showTrainingSetup)) && (
        <div>
        {!preparing && preparationRun && <p className="fine-action-note fine-independent-training">Train with a prepared dataset independently of your document job. <button className="btn btn-secondary" type="button" onClick={() => setView('prepare')}>View Document Preparation</button></p>}
        <form className="fine-configure-view" onSubmit={start}>
          <section className="fine-panel fine-configure-intro">
            <div className="fine-panel-heading">
              <div><span>{preparing ? 'Source document' : 'Required'}</span><h3>{preparing ? 'Prepare Grounded Examples' : 'Choose Training Dataset'}</h3></div>
              <span className={`fine-status ${matchingUpload && upload ? 'fine-status-completed' : 'fine-status-stopped'}`}>{matchingUpload && upload ? 'Uploaded' : preparing ? 'Document needed' : 'Dataset needed'}</span>
            </div>
            <p className="fine-panel-copy">{preparing ? 'Choose a PDF, DOCX, Markdown, or text document. Review examples in Preview before continuing preparation of the full dataset.' : 'Choose a training-ready JSONL file. The training service validates its contents when you select Start.'}</p>
            <div className="form-group fine-file-field">
              <FieldLabel htmlFor="fine-prepared-dataset" tooltipKey={preparing ? 'fineTuningSource' : 'fineTuningDataset'}>{preparing ? 'Source Document' : 'Training-Ready JSONL'}</FieldLabel>
              <input ref={preparedInput} className="fine-hidden-input" id="fine-prepared-dataset" type="file" accept={preparing ? '.pdf,.docx,.md,.txt' : '.jsonl,application/x-ndjson'} disabled={submitting} onChange={(event) => {
                void chooseInput(event.target.files?.[0] ?? null, preparing ? 'document' : 'dataset');
                event.currentTarget.value = '';
              }} />
            </div>
            {matchingUpload && upload ? (
              <div className="fine-dataset-ready">
                <div><strong>{upload.name}</strong><span>Stored · {(upload.size_bytes / 1024).toFixed(1)} KiB · {preparing ? 'not yet prepared' : 'not yet validated for training'}</span></div>
                <button className="btn btn-secondary" type="button" disabled={submitting} onClick={() => void chooseInput(null, inputKind)}>{preparing ? 'Change document' : 'Change dataset'}</button>
              </div>
            ) : (
              <div className="fine-actions">
                {matchingUpload && uploading ? (
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
              {!catalogLoading && !catalogError && preparationModels.length === 0 && <p role="status">No installed, validated text-only chat models were reported by the Catalog. Image/audio/video, embedding, reranking, and decision models are excluded. Check model installation and validation in the Catalog, then select Refresh Catalog.</p>}
              <div className="form-group">
                <FieldLabel htmlFor="fine-preparation-model" tooltipKey="fineTuningPreparationModel">Preparation Model</FieldLabel>
                <div className="fine-model-picker">
                <select id="fine-preparation-model" className="form-input" required value={preparationModelInstalled ? preparation.model : ''} disabled={busy || catalogLoading || !!catalogError} onChange={(event) => setPreparation((current) => ({ ...current, model: event.target.value }))}>
                  <option value="">Select a preparation model</option>
                  {preparationModels.map((model) => <option key={model.id} value={model.id}>{model.id} · Installed</option>)}
                </select>
                <button className="btn btn-secondary" type="button" disabled={busy || catalogLoading} onClick={() => setCatalogAttempt((attempt) => attempt + 1)}>Refresh Catalog</button>
                </div>
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
              <div className="form-group fine-preview-count">
                <FieldLabel htmlFor="fine-preview-count" tooltipKey="fineTuningPreviewCount">Preview records</FieldLabel>
                <select id="fine-preview-count" className="form-input" value={preparation.preview_records} disabled={busy} onChange={(event) => setPreparation((current) => ({ ...current, preview_records: Number(event.target.value) }))}>
                  {[1, 2, 3, 4, 5].map((count) => <option key={count} value={count}>{count} {count === 1 ? 'record' : 'records'}</option>)}
                </select>
              </div>
              <div className="form-group">
                <FieldLabel htmlFor="fine-preparation-instructions" tooltipKey="fineTuningDatasetInstructions">Preparation Instructions (optional)</FieldLabel>
                <textarea id="fine-preparation-instructions" className="form-input" rows={4} value={preparation.instructions} disabled={busy} onChange={(event) => setPreparation((current) => ({ ...current, instructions: event.target.value }))} />
              </div>
            </section>
          )}

          {!preparing && <>
          <section className="fine-panel">
            <div className="fine-panel-heading"><div><span>Required</span><h3>Base Model</h3></div></div>
            <p className="fine-panel-copy">Uses the same base model as the earlier successful run. Adapter quality is evaluated separately for every job.</p>
            <div className="fine-supported-model">
              <div><strong>Gemma 3 1B IT · Q8_0</strong><code>{FINE_TUNING_MODEL_ID}</code></div>
              <span className={`fine-status ${modelInstalled ? 'fine-status-completed' : 'fine-status-stopped'}`}>{modelsLoading ? 'Checking' : modelInstalled ? 'Installed' : 'Not installed'}</span>
            </div>
          </section>

          <section className="fine-panel fine-recipe-panel">
            <div className="fine-panel-heading"><div><span>Settings</span><h3>Training Configuration</h3></div></div>
            <p className="fine-panel-copy">The training service validates submitted settings. For document preparation, these settings are saved when Preparation starts.</p>
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
              <div className="fine-actions"><button className="btn btn-secondary" type="button" disabled={busy} onClick={() => setTraining(TESTED_TRAINING_OPTIONS)}>Reset Training Settings</button></div>
            </details>
          </section>
          </>}

          <section className="fine-panel fine-generation-summary">
            <div><h3>{ready ? 'Ready to Submit' : preparing ? 'Complete the Preparation Configuration' : 'Complete the Training Configuration'}</h3><p>{preparing ? 'Generate Preview extracts the document and creates examples for review. Build Training Dataset then continues full-dataset preparation; Training remains a separate choice.' : matchingUpload && upload ? `${upload.name} will be submitted for Gemma 3 1B training.` : 'Choose a prepared JSONL dataset and install the Gemma 3 1B model.'}</p></div>
            <button className="btn btn-primary" type="submit" disabled={!ready || busy || (preparing ? !!preparationActivity : !!trainingActivity)}>{submitting ? 'Submitting…' : preparing ? 'Generate Preview' : 'Start Training'}</button>
          </section>
          {preparing && <p className="fine-action-note">Training will use the settings currently selected in Configure Training. Those settings are saved when Preparation starts. <button className="btn btn-secondary" type="button" onClick={() => setView('configure')}>Configure Training</button>{!modelInstalled && ' Install the Gemma 3 1B model before starting Preparation.'}</p>}
        </form>
        </div>
      )}

      {view === 'prepare' && preparationRun && !sourceSetup && (
        <>
          {watchError && <div className="fine-error" role="alert">{watchError}</div>}
          <TrainingJobState run={preparationRun} onRefresh={() => void refreshRun(preparationRun)} view="preparation" />
          <StartPreparedTraining run={preparationRun} onRefresh={() => void refreshRun(preparationRun)} onStarted={() => { selectRun(preparationRun); setTrainingSetup(false); setView('configure'); }} />
          {['completed', 'needs_improvement', 'failed', 'stopped'].includes(preparationRun.status) && (
            <button className="btn btn-secondary" type="button" disabled={busy} onClick={() => { void chooseInput(null, 'document'); setSourceSetup(true); }}>Prepare Another Document</button>
          )}
        </>
      )}
      {view === 'configure' && !showTrainingSetup && trainingJob && (
        <div className="fine-train-view">
          {watchError && <div className="fine-error" role="alert">{watchError}</div>}
          <TrainingJobState run={trainingJob} onRefresh={() => void refreshRun(trainingJob)} view="training" />
          {['completed', 'needs_improvement', 'failed', 'stopped'].includes(trainingJob.status) && <button className="btn btn-secondary" type="button" disabled={busy} onClick={() => { void chooseInput(null, 'dataset'); setTrainingSetup(true); }}>Configure Another Training</button>}
        </div>
      )}
      {view === 'history' && <TrainingHistory onSelect={(run) => { selectRun(run); setSourceSetup(false); setTrainingSetup(false); setView(run.stage === 'preparation' || (!run.stage && run.stages.some((stage) => stage.name === 'preparation')) ? 'prepare' : 'configure'); }} />}
    </div>
  );
}
