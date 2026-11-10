import { useEffect, useRef, useState, type FormEvent } from 'react';
import { useSearchParams } from 'react-router-dom';
import { FINE_TUNING_MODEL_ID, TESTED_TRAINING_OPTIONS, useFineTuningRunner } from '../contexts/FineTuningRunnerContext';
import { useModelList } from '../contexts/ModelListContext';
import { api, ApiError } from '../services/api';
import type { CatalogModelResponse, PreparationDataset, PreparationOptions, TrainingOptions, TrainingRun } from '../types';
import { FieldLabel } from './ParamTooltips';
import PreparationDatasetHistory from './PreparationDatasetHistory';
import TrainingHistory from './TrainingHistory';
import { StartPreparedTraining, TrainingJobState } from './TrainingRun';

type View = 'prepare' | 'configure' | 'history';

// Match the runner's upload lifetime, including navigation away from this page.
// An uncertain create is never repeated; a confirmed destination is reused.
const uploadDestinations = new Map<string, { id?: string; uncertain: boolean }>();

export default function FineTuning() {
  const { models, loading: modelsLoading, error: modelsError, loadModels } = useModelList();
  const {
    inputKind, preparedFile, upload, uploading, submitting, trainingRun, preparationRun, error, watchError, trainingActivity,
    chooseInput, cancelPreparation, startRun, selectRun, refreshRun,
  } = useFineTuningRunner();
  const [training, setTraining] = useState<TrainingOptions>(TESTED_TRAINING_OPTIONS);
  const [preparation, setPreparation] = useState<PreparationOptions>({ model: '', goal: 'question_answer', instructions: '', preview_records: 3 });
  const [maxQuestions, setMaxQuestions] = useState('0');
  const [pageStart, setPageStart] = useState('');
  const [pageEnd, setPageEnd] = useState('');
  const [catalog, setCatalog] = useState<CatalogModelResponse[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(false);
  const [catalogError, setCatalogError] = useState('');
  const [catalogAttempt, setCatalogAttempt] = useState(0);
  const [view, setView] = useState<View>('prepare');
  const [sourceSetup, setSourceSetup] = useState(false);
  const [inspectedPreparation, setInspectedPreparation] = useState<TrainingRun | null>(null);
  const [inspectionError, setInspectionError] = useState('');
  const [trainingSetup, setTrainingSetup] = useState(false);
  const [historyTab, setHistoryTab] = useState<'prepare' | 'training'>('prepare');
  const [datasets, setDatasets] = useState<PreparationDataset[]>([]);
  const [datasetID, setDatasetID] = useState('');
  const [datasetName, setDatasetName] = useState('');
  const [datasetLoading, setDatasetLoading] = useState(false);
  const [datasetBusy, setDatasetBusy] = useState(false);
  const [datasetError, setDatasetError] = useState('');
  const [datasetAttempt, setDatasetAttempt] = useState(0);
  const [creationUncertain, setCreationUncertain] = useState(false);
  const [removal, setRemoval] = useState<PreparationDataset['documents'][number] | null>(null);
  const datasetPending = useRef(false);
  const selectedDataset = datasets.find((dataset) => dataset.id === datasetID);
  const [searchParams, setSearchParams] = useSearchParams();
  const preparedInput = useRef<HTMLInputElement>(null);
  const busy = uploading || submitting || datasetBusy;
  const preparing = view === 'prepare';
  const displayedPreparation = inspectedPreparation ?? preparationRun;
  const showSourceSetup = !displayedPreparation || sourceSetup;
  const preparationPending = !!preparationRun && !['completed', 'needs_improvement', 'failed', 'stopped'].includes(preparationRun.status);
  const preparationActive = preparationPending && preparationRun?.stage !== 'training';
  const trainingJob = (inspectedPreparation?.stage === 'training' ? inspectedPreparation : null) ?? trainingRun ?? (preparationRun?.stage === 'training' ? preparationRun : null);
  const showTrainingSetup = !trainingJob || trainingSetup;
  const matchingUpload = inputKind === (preparing ? 'document' : 'dataset');
  const pdfSource = matchingUpload && upload?.format === '.pdf';
  const modelInstalled = !!models?.data.some((model) => model.id === FINE_TUNING_MODEL_ID);
  const preparationModels = catalog.filter((model) => model.downloaded && model.validated
    && model.capabilities?.endpoint === 'chat_completion'
    && !model.capabilities.images && !model.capabilities.audio && !model.capabilities.video
    && !model.capabilities.embedding && !model.capabilities.rerank && !model.capabilities.decision);
  const preparationModelInstalled = preparationModels.some((model) => model.id === preparation.model);
  const ready = matchingUpload && !!upload && (preparing
    ? !catalogLoading && !catalogError && preparationModelInstalled && !datasetLoading && !datasetError && !creationUncertain && (!!selectedDataset || (!datasetID && !!datasetName.trim()))
    : modelInstalled);

  useEffect(() => {
    if (inputKind !== 'document' || !upload) return;
    const destination = uploadDestinations.get(upload.id);
    if (destination?.id) setDatasetID(destination.id);
    setCreationUncertain(destination?.uncertain ?? false);
  }, [inputKind, upload]);

  useEffect(() => {
    if (!preparing || !showSourceSetup) return;
    const controller = new AbortController();
    setDatasetLoading(true);
    setDatasetError('');
    api.preparationDatasets(controller.signal).then((result) => { if (!controller.signal.aborted) setDatasets(result.datasets ?? []); })
      .catch((err) => { if (!controller.signal.aborted) setDatasetError(err instanceof Error ? err.message : 'Could not load datasets.'); })
      .finally(() => { if (!controller.signal.aborted) setDatasetLoading(false); });
    return () => controller.abort();
  }, [preparing, showSourceSetup, datasetAttempt]);

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
    if (requested === 'prepare') { setSourceSetup(false); setInspectedPreparation(null); setInspectionError(''); }
    setSearchParams({}, { replace: true });
  }, [searchParams, setSearchParams]);

  const start = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!ready || datasetPending.current || (preparing && preparationPending)) return;
    datasetPending.current = true;
    setDatasetBusy(true);
    try {
      let destination = selectedDataset;
      const previous = upload ? uploadDestinations.get(upload.id) : undefined;
      if (preparing && !destination && previous?.id) destination = await api.preparationDataset(previous.id);
      if (preparing && !destination && previous?.uncertain) {
        setCreationUncertain(true);
        throw new Error('Refresh datasets and select the created dataset before submitting again.');
      }
      if (preparing && !destination) {
        // Persist the destination before submitting the job. A job retry reuses it.
        uploadDestinations.set(upload!.id, { uncertain: true });
        try {
          destination = await api.createPreparationDataset(datasetName.trim());
        } catch (err) {
          const rejected = err instanceof ApiError && err.status >= 400 && err.status < 500 && err.status !== 408;
          if (!rejected) setCreationUncertain(true);
          else uploadDestinations.delete(upload!.id);
          throw err;
        }
        setDatasets((current) => [...current, destination!]);
        setDatasetID(destination.id);
      }
      if (preparing) uploadDestinations.set(upload!.id, { id: destination!.id, uncertain: false });
      if (await startRun(training, preparing ? {
        ...preparation, target_dataset_id: destination!.id, base_version: destination!.version,
        max_questions: Number(maxQuestions),
        ...(pdfSource && pageStart && pageEnd ? { page_start: Number(pageStart), page_end: Number(pageEnd) } : {}),
      } : undefined)) {
        setSourceSetup(false);
        setTrainingSetup(false);
      }
    } catch (err) {
      setDatasetError(err instanceof Error ? err.message : 'Could not create dataset.');
    } finally {
      datasetPending.current = false;
      setDatasetBusy(false);
    }
  };

  const removeDocument = async () => {
    if (!selectedDataset || !removal || datasetPending.current) return;
    datasetPending.current = true;
    setDatasetBusy(true);
    setDatasetError('');
    try {
      const updated = await api.removePreparationDocument(selectedDataset.id, removal.id, selectedDataset.version);
      setDatasets((current) => current.map((dataset) => dataset.id === updated.id ? updated : dataset));
      setRemoval(null);
    } catch (err) {
      setDatasetError(`${err instanceof Error ? err.message : 'Could not confirm removal.'} Refresh datasets before trying again.`);
      setRemoval(null);
    } finally {
      datasetPending.current = false;
      setDatasetBusy(false);
    }
  };

  const openPreparation = (id: string) => {
    setInspectedPreparation(null);
    setInspectionError('');
    if (!id) {
      void chooseInput(null, 'document');
      setDatasetName('');
    }
    setDatasetID(id);
    setRemoval(null);
    setDatasetAttempt((value) => value + 1);
    setSourceSetup(true);
    setView('prepare');
  };

  const openPreparationRun = (run: TrainingRun) => {
    if (preparationActive && preparationRun && (run.workflow_id !== preparationRun.workflow_id || run.run_id !== preparationRun.run_id)) {
      setInspectedPreparation(run);
    } else {
      setInspectedPreparation(null);
      selectRun(run);
    }
    setInspectionError('');
    setSourceSetup(false);
    setView('prepare');
  };

  const refreshPreparation = async () => {
    if (!inspectedPreparation) { await refreshRun(preparationRun ?? undefined); return; }
    setInspectionError('');
    try {
      const updated = await api.trainingRun(inspectedPreparation.workflow_id, inspectedPreparation.run_id);
      setInspectedPreparation((current) => current?.workflow_id === updated.workflow_id && current.run_id === updated.run_id ? updated : current);
    } catch (err) {
      setInspectionError(err instanceof Error ? err.message : 'Could not refresh this preparation.');
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
          <button key={item} role="tab" aria-selected={view === item} className={`tab ${view === item ? 'active' : ''}`} onClick={() => { setView(item); setInspectedPreparation(null); setInspectionError(''); if (item === 'prepare') setSourceSetup(false); }}>
            {item === 'prepare' ? 'Prepare Dataset' : item === 'configure' ? 'Configure Training' : 'History'}
          </button>
        ))}
      </div>

      {error && <div className="fine-error" role="alert">{error}</div>}
      {modelsError && <div className="fine-error" role="alert">Could not load installed models: {modelsError}</div>}

      {preparationRun && (preparationActive || sourceSetup || inspectedPreparation) && (view !== 'prepare' || showSourceSetup || inspectedPreparation) && (
        <section className="fine-panel fine-current-preparation" aria-label="Current preparation">
          <div className="fine-panel-heading">
            <div><span>{preparationActive ? 'Preparation in progress' : 'Preparation result'}</span><h3>{preparationActive ? 'Your preparation has not been canceled' : 'Your previous preparation is still available'}</h3></div>
            <span className={`fine-status fine-status-${preparationRun.status}`}>{preparationRun.status.replaceAll('_', ' ')}</span>
          </div>
          <p className="fine-panel-copy">{preparationActive ? 'Opening another dataset setup or browsing History does not cancel this run. You can configure another dataset, but must wait for the current preparation to finish before starting it.' : 'Opening another dataset setup did not cancel this run. Return to its process screen to see the final status and any saved dataset.'}</p>
          {preparationRun.status === 'waiting' && <p className="fine-action-note">This run is waiting for your decision. Return to the process screen to continue.</p>}
          {(preparationRun.state_stale || watchError) && <p className="fine-error" role="alert">{watchError || 'This status may be out of date.'} Open the current preparation and select Refresh to check its latest status.</p>}
          <div className="fine-actions"><button className="btn btn-primary" type="button" onClick={() => { setInspectedPreparation(null); setInspectionError(''); setSourceSetup(false); setView('prepare'); }}>{preparationActive ? 'View current preparation' : 'View previous preparation'}</button></div>
        </section>
      )}

      {((view === 'prepare' && showSourceSetup) || (view === 'configure' && showTrainingSetup)) && (
        <div>
        {!preparing && preparationRun && <p className="fine-action-note fine-independent-training">Train with a prepared dataset independently of your document job. <button className="btn btn-secondary" type="button" onClick={() => { setSourceSetup(false); setInspectedPreparation(null); setInspectionError(''); setView('prepare'); }}>View Document Preparation</button></p>}
        <form className="fine-configure-view" onSubmit={start}>
          <section className="fine-panel fine-configure-intro">
            <div className="fine-panel-heading">
              <div><span>{preparing ? 'Source document' : 'Required'}</span><h3>{preparing ? 'Prepare Grounded Examples' : 'Choose Training Dataset'}</h3></div>
              <span className={`fine-status ${matchingUpload && upload ? 'fine-status-completed' : 'fine-status-stopped'}`}>{matchingUpload && upload ? 'Uploaded' : preparing ? 'Document needed' : 'Dataset needed'}</span>
            </div>
            <p className="fine-panel-copy">{preparing ? 'Choose a PDF, DOCX, Markdown, or text document. Review examples in Preview before continuing preparation of the full dataset.' : 'Choose a training-ready JSONL file. The training service validates its contents when you select Start.'}</p>
            {preparing && <>
              <div className="form-group">
                <FieldLabel htmlFor="fine-destination" tooltipKey="fineTuningDestination">Save examples to</FieldLabel>
                <select id="fine-destination" className="form-input" value={datasetID} disabled={busy || datasetLoading} onChange={(event) => { setDatasetID(event.target.value); setRemoval(null); if (event.target.value) setCreationUncertain(false); }}>
                  <option value="">Create a new dataset</option>
                  {datasets.map((dataset) => <option key={dataset.id} value={dataset.id}>{dataset.name} · {dataset.documents.length} documents · {dataset.record_count} examples</option>)}
                </select>
              </div>
              {!datasetID && <div className="form-group">
                <FieldLabel htmlFor="fine-dataset-name" tooltipKey="fineTuningDatasetName">Dataset name</FieldLabel>
                <input id="fine-dataset-name" className="form-input" required value={datasetName} disabled={busy || creationUncertain} placeholder="Give this dataset a name" onChange={(event) => setDatasetName(event.target.value)} />
              </div>}
              {datasetLoading && <p role="status">Loading datasets…</p>}
              {datasetError && <p className="fine-error" role="alert">{datasetError}</p>}
              {creationUncertain && <p className="fine-action-note" role="alert">Dataset creation could not be confirmed. Refresh datasets and select the dataset that was created before submitting again. A new dataset will not be created automatically.</p>}
              <div className="fine-actions"><button className="btn btn-secondary" type="button" disabled={busy || datasetLoading} onClick={() => { setRemoval(null); setDatasetAttempt((value) => value + 1); }}>Refresh datasets</button></div>
              {selectedDataset && <div className="fine-recipe-details">
                <p className="fine-action-note"><strong>{selectedDataset.record_count} existing examples stay as they are.</strong> Only the document you add below will be prepared. Version {selectedDataset.version} is retained.</p>
                <details><summary>View {selectedDataset.documents.length} existing documents</summary>
                  {selectedDataset.documents.map((doc) => <div className="fine-dataset-ready" key={doc.id}>
                    <div><strong>{doc.name}</strong><span>{doc.page_start > 0 ? `Pages ${doc.page_start}–${doc.page_end}` : 'Entire document'} · {doc.record_count} examples</span></div>
                    <button className="btn btn-secondary" type="button" disabled={busy || preparationPending} onClick={() => setRemoval(doc)}>Remove from dataset</button>
                  </div>)}
                </details>
                {removal && <div className="fine-error" role="alert">
                  <strong>Remove {removal.name}?</strong>
                  <p>Its {removal.record_count} examples will be removed from the next version of {selectedDataset.name}. Original files, previous dataset versions, and existing adapters are retained unchanged.</p>
                  <div className="fine-actions">
                    <button className="btn btn-danger" type="button" disabled={busy} onClick={() => void removeDocument()}>Remove document &amp; examples</button>
                    <button className="btn btn-secondary" type="button" disabled={busy} onClick={() => setRemoval(null)}>Cancel</button>
                  </div>
                </div>}
              </div>}
            </>}
            <div className="form-group fine-file-field">
              <FieldLabel htmlFor="fine-prepared-dataset" tooltipKey={preparing ? 'fineTuningSource' : 'fineTuningDataset'}>{preparing ? 'Source Document' : 'Training-Ready JSONL'}</FieldLabel>
              <input ref={preparedInput} className="fine-hidden-input" id="fine-prepared-dataset" type="file" accept={preparing ? '.pdf,.docx,.md,.txt' : '.jsonl,application/x-ndjson'} disabled={busy} onChange={(event) => {
                void chooseInput(event.target.files?.[0] ?? null, preparing ? 'document' : 'dataset');
                event.currentTarget.value = '';
              }} />
            </div>
            {matchingUpload && upload ? (
              <div className="fine-dataset-ready">
                <div><strong>{upload.name}</strong><span>Stored · {(upload.size_bytes / 1024).toFixed(1)} KiB · {preparing ? 'not yet prepared' : 'not yet validated for training'}</span></div>
                <button className="btn btn-secondary" type="button" disabled={busy} onClick={() => void chooseInput(null, inputKind)}>{preparing ? 'Change document' : 'Change dataset'}</button>
              </div>
            ) : (
              <div className="fine-actions">
                {matchingUpload && uploading ? (
                  <>
                    <p role="status">Uploading {preparedFile?.name}…</p>
                    <button className="btn btn-danger" type="button" onClick={cancelPreparation}>Cancel Upload</button>
                  </>
                ) : (
                  <button className="btn btn-primary" type="button" disabled={busy} onClick={() => preparedInput.current?.click()}>{preparing ? 'Choose Document' : 'Choose JSONL Dataset'}</button>
                )}
              </div>
            )}
            {preparing && <p className="fine-action-note">Preparation finishes before you choose whether to enter Training. PDF/DOCX require a configured server-side document extractor; text and Markdown can be read directly.</p>}
          </section>

          {preparing && (
            <section className="fine-panel" aria-label="Preparation settings">
              <div className="fine-panel-heading"><div><span>Preparation</span><h3>Preparation Settings</h3></div></div>
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
                <FieldLabel htmlFor="fine-max-questions" tooltipKey="fineTuningMaxQuestions">Maximum questions</FieldLabel>
                <input id="fine-max-questions" className="form-input" type="number" min={0} step={1} required value={maxQuestions} disabled={busy} onChange={(event) => setMaxQuestions(event.target.value)} />
                <p className="fine-action-note">0 = unlimited. Applies only to this document, not existing dataset examples. Includes reviewed examples and evaluation questions; fewer may survive verification.</p>
                <p className="fine-action-note">Training needs at least 50 usable examples: 20 training, 10 validation, and 20 protected test examples, with valid fact coverage and splits. A cap of 20 tests preparation only.</p>
              </div>
              {pdfSource && <div className="form-group">
                <div className="fine-form-grid">
                  <div className="form-group">
                    <FieldLabel htmlFor="fine-page-start" tooltipKey="fineTuningPageStart">First page (optional)</FieldLabel>
                    <input id="fine-page-start" className="form-input" type="number" min={1} step={1} required={!!pageEnd} value={pageStart} disabled={busy} placeholder="Whole PDF" onChange={(event) => setPageStart(event.target.value)} />
                  </div>
                  <div className="form-group">
                    <FieldLabel htmlFor="fine-page-end" tooltipKey="fineTuningPageEnd">Last page (inclusive)</FieldLabel>
                    <input id="fine-page-end" className="form-input" type="number" min={Math.max(1, Number(pageStart) || 1)} step={1} required={!!pageStart} value={pageEnd} disabled={busy} placeholder="Whole PDF" onChange={(event) => setPageEnd(event.target.value)} />
                  </div>
                </div>
                <p className="fine-action-note">Leave both blank to process the whole PDF. To process one chapter, enter its first and last PDF viewer page numbers (not printed page labels). Only those pages are extracted and used for examples; your original file is unchanged.</p>
              </div>}
              <details className="fine-recipe-details">
                <summary>Preparation Instructions (optional)</summary>
                <div className="form-group">
                  <FieldLabel htmlFor="fine-preparation-instructions" tooltipKey="fineTuningDatasetInstructions">Instructions</FieldLabel>
                  <textarea id="fine-preparation-instructions" className="form-input" rows={4} value={preparation.instructions} disabled={busy} onChange={(event) => setPreparation((current) => ({ ...current, instructions: event.target.value }))} />
                </div>
              </details>
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
            <p className="fine-panel-copy">The training service validates these settings when you start a standalone training run.</p>
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
            <div><h3>{preparing && preparationPending ? 'Current job must finish first' : ready ? 'Ready to Submit' : preparing ? 'Complete the Preparation Configuration' : 'Complete the Training Configuration'}</h3><p>{preparing ? 'Generate Preview extracts the document and creates examples for review. Build Training Dataset then continues full-dataset preparation; Training remains a separate choice.' : matchingUpload && upload ? `${upload.name} will be submitted for Gemma 3 1B training.` : 'Choose a prepared JSONL dataset and install the Gemma 3 1B model.'}</p></div>
            <button className="btn btn-primary" type="submit" disabled={!ready || busy || (preparing ? preparationPending : !!trainingActivity)}>{submitting ? 'Submitting…' : preparing ? 'Generate Preview' : 'Start Training'}</button>
          </section>
          {preparing && preparationPending && <p className="fine-action-note">Generate Preview is unavailable until your current job finishes. Configuring this dataset does not stop that job.</p>}
          {preparing && <p className="fine-action-note">New examples are saved to the selected dataset after verification. Existing examples are unchanged. No training starts during preparation, and Gemma is not required.</p>}
        </form>
        </div>
      )}

      {inspectedPreparation && ((view === 'prepare' && !sourceSetup) || (view === 'configure' && !showTrainingSetup)) && <p className="fine-action-note">You are viewing a different run from History. Your current preparation is tracked separately above.</p>}

      {view === 'prepare' && displayedPreparation && !sourceSetup && (
        <>
          {watchError && <div className="fine-error" role="alert">{watchError}</div>}
          {inspectionError && <div className="fine-error" role="alert">{inspectionError}</div>}
          <TrainingJobState run={displayedPreparation} onRefresh={() => void refreshPreparation()} view="preparation" />
          {!displayedPreparation.preparation_only && <StartPreparedTraining run={displayedPreparation} onRefresh={() => void refreshPreparation()} onStarted={() => { selectRun(displayedPreparation); setTrainingSetup(false); setView('configure'); }} />}
          {['completed', 'needs_improvement', 'failed', 'stopped'].includes(displayedPreparation.status) && (
            <button className="btn btn-secondary" type="button" disabled={busy} onClick={() => { void chooseInput(null, 'document'); openPreparation(displayedPreparation.prepared_dataset_id ?? datasetID); }}>Prepare Another Document</button>
          )}
        </>
      )}
      {view === 'configure' && !showTrainingSetup && trainingJob && (
        <div className="fine-train-view">
          {watchError && <div className="fine-error" role="alert">{watchError}</div>}
          {inspectionError && <div className="fine-error" role="alert">{inspectionError}</div>}
          <TrainingJobState run={trainingJob} onRefresh={() => { if (inspectedPreparation) void refreshPreparation(); else void refreshRun(trainingJob); }} view="training" />
          {['completed', 'needs_improvement', 'failed', 'stopped'].includes(trainingJob.status) && <button className="btn btn-secondary" type="button" disabled={busy} onClick={() => { void chooseInput(null, 'dataset'); setTrainingSetup(true); }}>Configure Another Training</button>}
        </div>
      )}
      {view === 'history' && <>
        <div className="tabs" role="tablist" aria-label="History views">
          <button className={`tab ${historyTab === 'prepare' ? 'active' : ''}`} role="tab" aria-selected={historyTab === 'prepare'} onClick={() => setHistoryTab('prepare')}>Prepare Dataset</button>
          <button className={`tab ${historyTab === 'training' ? 'active' : ''}`} role="tab" aria-selected={historyTab === 'training'} onClick={() => setHistoryTab('training')}>Training</button>
        </div>
        {historyTab === 'prepare' ? <>
          <PreparationDatasetHistory onOpen={openPreparation} />
          <TrainingHistory preparationOnly onSelect={openPreparationRun} />
        </> : <TrainingHistory onSelect={(run) => {
          if (run.stages.some((stage) => stage.name === 'preparation')) {
            openPreparationRun(run);
            if (run.stage === 'training') { setTrainingSetup(false); setView('configure'); }
          } else {
            setInspectedPreparation(null);
            selectRun(run);
            setTrainingSetup(false);
            setView('configure');
          }
        }} />}
      </>}
    </div>
  );
}
