import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Link, Navigate, useSearchParams } from 'react-router-dom';
import { FINE_TUNING_MODEL_ID, TESTED_TRAINING_OPTIONS, requestMessage, useFineTuningRunner } from '../contexts/FineTuningRunnerContext';
import { useModelList } from '../contexts/ModelListContext';
import { api, ApiError } from '../services/api';
import type { CatalogModelResponse, PreparationDataset, PreparationOptions, TrainingOptions, TrainingRun } from '../types';
import { FieldLabel, ParamTooltip } from './ParamTooltips';
import DatasetDashboard from './DatasetDashboard';
import { TrainingJobState } from './TrainingRun';

type View = 'prepare' | 'configure';

// Match the runner's upload lifetime, including navigation away from this page.
// An uncertain create is never repeated; a confirmed destination is reused.
const uploadDestinations = new Map<string, { id?: string; uncertain: boolean }>();
// digits keeps only 0–9, so page fields never hold letters or signs, even when pasted.
const digits = (value: string) => value.replace(/\D/g, '');
const count = (n: number, noun: string) => `${n} ${noun}${n === 1 ? '' : 's'}`;
const sourceFileName = (run: TrainingRun) => `${(run.target_dataset_name ?? 'dataset').replace(/[^a-zA-Z0-9._-]/g, '_')}-v${run.prepared_dataset_version}.jsonl`;

export default function FineTuning() {
  const [params] = useSearchParams();
  const view = params.get('view');
  const workspaceVisible = params.get('workspace') === 'live' || view === 'prepare' || view === 'configure' || view === 'run';
  const [workspaceVisited, setWorkspaceVisited] = useState(workspaceVisible);
  useEffect(() => { if (workspaceVisible) setWorkspaceVisited(true); }, [workspaceVisible]);
  // History is part of the dashboard now, and every run opens on the dashboard.
  if (view === 'history') return <Navigate replace to="/fine-tuning" />;
  if (view === 'prepare' && params.get('preparation') && params.get('run')) {
    const run = new URLSearchParams({ preparation: params.get('preparation')!, run: params.get('run')! });
    return <Navigate replace to={`/fine-tuning?${run}`} />;
  }
  return <>
    <div hidden={workspaceVisible}><DatasetDashboard visible={!workspaceVisible} /></div>
    <div hidden={!workspaceVisible}>
      {(workspaceVisible || workspaceVisited) && <FineTuningWorkspace visible={workspaceVisible} />}
    </div>
  </>;
}

function FineTuningWorkspace({ visible }: { visible: boolean }) {
  const { models, loading: modelsLoading, error: modelsError, loadModels } = useModelList();
  const {
    preparedFile, upload, uploading, documents, documentsUploading, submitting, trainingRun, preparationRun, error, watchError, trainingActivity, adopting,
    chooseInput, addDocuments, removeDocument: removeSource, cancelPreparation, startRun, startPreparations, refreshRun,
  } = useFineTuningRunner();
  const [training, setTraining] = useState<TrainingOptions>(TESTED_TRAINING_OPTIONS);
  const [preparation, setPreparation] = useState<PreparationOptions>({ model: '', goal: 'question_answer', instructions: '', preview_records: 3 });
  const [maxQuestions, setMaxQuestions] = useState('');
  const [limitExamples, setLimitExamples] = useState(false);
  // Page ranges for PDF documents, by document key. Each PDF may cover different pages.
  const [ranges, setRanges] = useState<Record<string, { use: boolean; start: string; end: string }>>({});
  const [startNotice, setStartNotice] = useState('');
  const [catalog, setCatalog] = useState<CatalogModelResponse[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(false);
  const [catalogError, setCatalogError] = useState('');
  const [catalogAttempt, setCatalogAttempt] = useState(0);
  const [trainingSetup, setTrainingSetup] = useState(false);
  // Add documents opens this form for one existing dataset; otherwise it creates a new one.
  const [selectedDataset, setSelectedDataset] = useState<PreparationDataset | null>(null);
  const [datasetName, setDatasetName] = useState('');
  const [datasetLoading, setDatasetLoading] = useState(false);
  const [datasetBusy, setDatasetBusy] = useState(false);
  const [datasetError, setDatasetError] = useState('');
  const [datasetAttempt, setDatasetAttempt] = useState(0);
  const [creationUncertain, setCreationUncertain] = useState(false);
  const [removal, setRemoval] = useState<PreparationDataset['documents'][number] | null>(null);
  const datasetPending = useRef(false);
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedView = searchParams.get('view');
  const view: View = requestedView === 'configure' || requestedView === 'run' ? 'configure' : 'prepare';
  const preparedInput = useRef<HTMLInputElement>(null);
  const preparing = view === 'prepare';
  const datasetID = (preparing && searchParams.get('dataset')) || '';
  // Document uploads run on their own, so more documents can be chosen meanwhile.
  const busy = submitting || datasetBusy || (!preparing && uploading);
  const preparationUnfinished = !!preparationRun && !['completed', 'needs_improvement', 'failed', 'stopped'].includes(preparationRun.status);
  const preparationActive = preparationUnfinished && preparationRun?.stage !== 'training';
  // Preparations run side by side; training waits for an unfinished preparation.
  const startBlocked = !preparing && (preparationUnfinished || adopting);
  const trainingJob = trainingRun ?? (preparationRun?.stage === 'training' ? preparationRun : null);
  const showTrainingSetup = !trainingJob || trainingSetup;
  const uploaded = documents.filter((item) => item.upload);
  const uploadFailed = documents.filter((item) => !item.upload && item.error);
  // The same file twice would be prepared twice; the second save would then be rejected.
  const duplicates = new Set(uploaded.filter((item, index) => uploaded.findIndex((other) => other.upload!.sha256 === item.upload!.sha256) !== index).map((item) => item.key));
  const modelInstalled = !!models?.data.some((model) => model.id === FINE_TUNING_MODEL_ID);
  const preparationModels = catalog.filter((model) => model.downloaded && model.validated
    && model.capabilities?.endpoint === 'chat_completion'
    && !model.capabilities.images && !model.capabilities.audio && !model.capabilities.video
    && !model.capabilities.embedding && !model.capabilities.rerank && !model.capabilities.decision);
  const preparationModelInstalled = preparationModels.some((model) => model.id === preparation.model);
  const positiveInteger = (value: string) => /^\d+$/.test(value) && Number.isSafeInteger(Number(value)) && Number(value) > 0;
  const capError = limitExamples && !positiveInteger(maxQuestions) ? 'Enter a positive whole number, or choose Unlimited.' : '';
  const rangeErrors = (key: string) => {
    const range = ranges[key];
    if (!range?.use) return { start: '', end: '' };
    const start = positiveInteger(range.start) ? '' : 'Enter a positive whole-number first page.';
    return { start, end: !positiveInteger(range.end) || (!start && Number(range.end) < Number(range.start)) ? 'Enter a positive whole-number last page at or after the first page.' : '' };
  };
  const pdfs = uploaded.filter((item) => item.upload!.format === '.pdf');
  const pageIssues = pdfs.flatMap((item) => {
    const errors = rangeErrors(item.key);
    const index = documents.indexOf(item);
    return [errors.start && { message: `${item.file.name}: ${errors.start}`, field: `fine-page-start-${index}` }, errors.end && { message: `${item.file.name}: ${errors.end}`, field: `fine-page-end-${index}` }].filter((issue) => !!issue);
  });
  const settingsValid = !capError && !pageIssues.length;
  const ready = preparing
    ? uploaded.length > 0 && !documentsUploading && !uploadFailed.length && !duplicates.size && settingsValid && !catalogLoading && !catalogError && preparationModelInstalled && !datasetLoading && !datasetError && !creationUncertain && (datasetID ? selectedDataset?.id === datasetID : !!datasetName.trim())
    : !!upload && modelInstalled;

  const uploadIDs = uploaded.map((item) => item.upload!.id).join();
  useEffect(() => {
    const destination = uploaded.map((item) => uploadDestinations.get(item.upload!.id)).find(Boolean);
    setCreationUncertain(destination?.uncertain ?? false);
  }, [uploadIDs]);

  useEffect(() => {
    setSelectedDataset(null);
    if (!visible || !datasetID) return;
    const controller = new AbortController();
    setDatasetLoading(true);
    setDatasetError('');
    api.preparationDataset(datasetID, controller.signal).then((result) => { if (!controller.signal.aborted) setSelectedDataset(result); })
      .catch((err) => { if (!controller.signal.aborted) setDatasetError(requestMessage(err, 'Could not load this dataset.')); })
      .finally(() => { if (!controller.signal.aborted) setDatasetLoading(false); });
    return () => controller.abort();
  }, [visible, datasetID, datasetAttempt]);

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

  const readinessIssues: { message: string; field?: string }[] = [];
  if (documentsUploading) readinessIssues.push({ message: 'Wait for the document uploads.', field: 'fine-source' });
  else if (!uploaded.length) readinessIssues.push({ message: 'Choose at least one source document.', field: 'fine-source' });
  if (uploadFailed.length) readinessIssues.push({ message: 'Remove documents that could not upload.', field: 'fine-source' });
  if (duplicates.size) readinessIssues.push({ message: 'Remove the duplicate document.', field: 'fine-source' });
  if (datasetLoading) readinessIssues.push({ message: 'Wait for the dataset to load.' });
  else if (datasetError || creationUncertain || (datasetID && !selectedDataset)) readinessIssues.push({ message: 'Review the dataset.', field: 'fine-destination-section' });
  else if (!datasetID && !datasetName.trim()) readinessIssues.push({ message: 'Name the new dataset.', field: 'fine-dataset-name' });
  if (catalogLoading) readinessIssues.push({ message: 'Wait for preparation models to load.' });
  else if (catalogError || !preparationModelInstalled) readinessIssues.push({ message: catalogError ? 'Retry the preparation model catalog.' : 'Select a preparation model.', field: 'fine-model-section' });
  if (capError) readinessIssues.push({ message: capError, field: 'fine-max-questions' });
  readinessIssues.push(...pageIssues);
  if (busy) readinessIssues.push({ message: 'Wait for the current operation to finish.' });
  const previews = uploaded.length > 1 ? `${uploaded.length} previews` : 'preview';
  const goToField = (id: string) => {
    const field = document.getElementById(id);
    const scroller = field?.closest('.fine-preparation-fields');
    if (field && scroller) scroller.scrollTo({ top: scroller.scrollTop + field.getBoundingClientRect().top - scroller.getBoundingClientRect().top - (scroller.clientHeight - field.clientHeight) / 2 });
    field?.focus({ preventScroll: true });
  };

  const start = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!ready || datasetPending.current || startBlocked) return;
    datasetPending.current = true;
    setDatasetBusy(true);
    try {
      if (!preparing) {
        if (await startRun(training)) setTrainingSetup(false);
        return;
      }
      // One destination serves every document. It is created once, even across retries.
      const ids = uploaded.map((item) => item.upload!.id);
      let destination = selectedDataset;
      const previous = ids.map((id) => uploadDestinations.get(id)).find(Boolean);
      if (!destination && previous?.id) destination = await api.preparationDataset(previous.id);
      if (!destination && previous?.uncertain) {
        setCreationUncertain(true);
        throw new Error('Refresh datasets and select the created dataset before submitting again.');
      }
      if (!destination) {
        // Persist the destination before submitting the runs. A retry reuses it.
        for (const id of ids) uploadDestinations.set(id, { uncertain: true });
        try {
          destination = await api.createPreparationDataset(datasetName.trim());
        } catch (err) {
          const rejected = err instanceof ApiError && err.status >= 400 && err.status < 500 && err.status !== 408;
          if (!rejected) setCreationUncertain(true);
          else for (const id of ids) uploadDestinations.delete(id);
          throw err;
        }
      }
      for (const id of ids) uploadDestinations.set(id, { id: destination.id, uncertain: false });
      // Each document becomes its own run. All start from the same version; each
      // save adds onto the newest version, so the runs do not conflict.
      const target = destination;
      const runs = await startPreparations(training, (item) => {
        const range = ranges[item.key];
        return {
          ...preparation, target_dataset_id: target.id, base_version: target.version,
          max_questions: limitExamples ? Number(maxQuestions) : 0,
          ...(item.upload!.format === '.pdf' && range?.use ? { page_start: Number(range.start), page_end: Number(range.end) } : {}),
        };
      });
      setStartNotice(runs.length && runs.length < ids.length ? `${count(runs.length, 'preparation')} started and ${runs.length === 1 ? 'is' : 'are'} on the dashboard. Fix or remove the documents below, then generate again.` : '');
      if (runs.length === ids.length) {
        setRanges({});
        if (runs.length === 1) showPreparation(runs[0], true);
        else setSearchParams({}, { replace: true });
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
      setSelectedDataset(updated);
      setRemoval(null);
    } catch (err) {
      setDatasetError(`${err instanceof Error ? err.message : 'Could not confirm removal.'} Refresh the dataset before trying again.`);
      setRemoval(null);
    } finally {
      datasetPending.current = false;
      setDatasetBusy(false);
    }
  };

  // Continue to training names a finished run; its saved dataset version becomes
  // the training dataset, downloaded and uploaded as training-ready JSONL.
  const sourceWorkflow = view === 'configure' ? searchParams.get('preparation') : null;
  const sourceRun = view === 'configure' ? searchParams.get('run') : null;
  const sourceKey = sourceWorkflow && sourceRun ? `${sourceWorkflow}/${sourceRun}` : '';
  const [source, setSource] = useState<TrainingRun | null>(null);
  const [sourceError, setSourceError] = useState('');
  const [sourceAttempt, setSourceAttempt] = useState(0);
  const loadedSource = useRef('');
  useEffect(() => {
    if (!visible || !sourceKey || loadedSource.current === sourceKey) return;
    loadedSource.current = sourceKey;
    setSource(null);
    setSourceError('');
    void (async () => {
      try {
        const run = await api.trainingRun(sourceWorkflow!, sourceRun!);
        setSource(run);
        if (!run.prepared_dataset_id || !run.prepared_dataset_version) throw new Error('This run has no saved dataset version.');
        const blob = await api.downloadPreparationDataset(run.prepared_dataset_id, run.prepared_dataset_version);
        await chooseInput(new File([blob], sourceFileName(run), { type: 'application/x-ndjson' }));
      } catch (err) {
        loadedSource.current = '';
        setSourceError(requestMessage(err, 'Could not load this dataset for training.'));
      }
    })();
  }, [visible, sourceKey, sourceAttempt]);

  // Every run opens on the dashboard's run screen.
  const showPreparation = (run: TrainingRun, replace = false) => setSearchParams({ preparation: run.workflow_id, run: run.run_id }, { replace });

  const updateTraining = (key: keyof TrainingOptions, value: number) => {
    setTraining((current) => ({ ...current, [key]: value }));
  };

  return (
    <div className={`fine-page${preparing ? ' fine-preparing' : ''}`}>
      <div className="page-header fine-page-header">
        <div>
          <h2>Fine-Tuning</h2>
        </div>
      </div>

      <nav className="tabs fine-tabs" aria-label="Fine-tuning views">
        <Link className={`tab ${preparing ? 'active' : ''}`} to="/fine-tuning">Dashboard</Link>
        <Link aria-current={visible && view === 'configure' ? 'page' : undefined} className={`tab ${view === 'configure' ? 'active' : ''}`} to="/fine-tuning?view=configure">Configure Training</Link>
      </nav>

      {preparing && <div className="page-header-with-action prep-heading">
        <div>
          <Link className="prep-back" to="/fine-tuning">← Dashboard</Link>
          <h2>{datasetID ? 'Add documents' : 'New dataset'}</h2>
          <p className="page-description">{datasetID ? `Choose documents to add to ${selectedDataset?.name ?? 'this dataset'}, then generate previews to review.` : 'Choose documents and where their records go, then generate previews to review.'}</p>
        </div>
      </div>}
      {view === 'configure' && showTrainingSetup && sourceKey && <section className="fine-panel dash-training-source" aria-label="Training dataset">
        <Link className="prep-back" to={`/fine-tuning?${new URLSearchParams({ preparation: sourceWorkflow!, run: sourceRun! })}`}>← {source ? source.target_dataset_name ?? 'Dataset' : 'Back to the dataset'}</Link>
        <span className="fine-eyebrow">Training dataset</span>
        {source?.prepared_dataset_id && source.prepared_dataset_version ? <>
          <h3>{source.target_dataset_name ?? source.prepared_dataset_id} · version {source.prepared_dataset_version} · {count(source.dataset_record_count, 'record')}</h3>
          <p className="prep-copy">{source.training_ready ? 'This dataset meets the 50-record minimum for training.' : 'This dataset has fewer than 50 records, the training minimum. Add documents to it before training.'} {upload?.name === sourceFileName(source) ? 'It is loaded as the training dataset below.' : 'Loading it as the training dataset…'}</p>
        </> : !sourceError && <p className="prep-copy" role="status">Loading the dataset…</p>}
        {sourceError && <p className="fine-error" role="alert">{sourceError} <button className="btn btn-secondary" type="button" onClick={() => { loadedSource.current = ''; setSourceAttempt((value) => value + 1); }}>Try again</button></p>}
      </section>}

      {error && view === 'configure' && <div className="fine-error" role="alert">{error}</div>}
      {modelsError && <div className="fine-error" role="alert">Could not load installed models: {modelsError}</div>}
      {(preparing || showTrainingSetup) && (
        <div className="fine-setup-wrapper">
        {!preparing && preparationRun && !preparationActive && <p className="fine-action-note fine-independent-training">Train with a prepared dataset independently of your document job. <button className="btn btn-secondary" type="button" onClick={() => showPreparation(preparationRun)}>View Document Preparation</button></p>}
        <form className="fine-configure-view" onSubmit={start}>
          <div className="fine-preparation-fields">
          <section className="fine-panel fine-configure-intro" id="fine-source" tabIndex={-1}>
            <div className="fine-panel-heading">
              <div><span>{preparing ? 'Step 1' : 'Required'}</span><h3>{preparing ? 'Source documents' : 'Choose Training Dataset'}</h3></div>
              <span className={`fine-status ${(preparing ? uploaded.length && !documentsUploading : upload) ? 'fine-status-completed' : 'fine-status-stopped'}`}>{preparing ? documentsUploading ? 'Uploading…' : uploaded.length ? count(uploaded.length, 'document') : 'Document needed' : upload ? 'Uploaded' : uploading ? 'Uploading…' : 'Dataset needed'}</span>
            </div>
            <p className="fine-panel-copy">{preparing ? 'Choose one or more PDF, DOCX, Markdown, or text documents to turn into grounded records. Each document is prepared as its own run, with its own preview to review.' : 'Choose a training-ready JSONL file. The training service validates its contents when you select Start.'}</p>
            <div className="form-group fine-file-field">
              <FieldLabel htmlFor="fine-prepared-dataset" tooltipKey={preparing ? 'fineTuningSource' : 'fineTuningDataset'}>{preparing ? 'Source documents (at least one)' : 'Training-Ready JSONL'}</FieldLabel>
              <input ref={preparedInput} className="fine-hidden-input" id="fine-prepared-dataset" type="file" multiple={preparing} accept={preparing ? '.pdf,.docx,.md,.txt' : '.jsonl,application/x-ndjson'} disabled={busy} onChange={(event) => {
                const files = [...(event.target.files ?? [])];
                if (files.length) {
                  setStartNotice('');
                  if (preparing) addDocuments(files);
                  else void chooseInput(files[0]);
                }
                event.currentTarget.value = '';
              }} />
            </div>
            {preparing ? <>
              {documents.length > 0 && <ul className="fine-doc-list" aria-label="Source documents">
                {documents.map((item, index) => {
                  const range = ranges[item.key] ?? { use: false, start: '', end: '' };
                  const setRange = (change: Partial<typeof range>) => setRanges((current) => ({ ...current, [item.key]: { ...range, ...change } }));
                  const errors = rangeErrors(item.key);
                  const pdf = item.upload?.format === '.pdf';
                  const state = item.upload ? 'stored' : item.error ? 'failed' : 'uploading';
                  return <li key={item.key} className="fine-doc">
                    <div className="fine-doc-head">
                      <div><strong>{item.file.name}</strong><span className={`fine-doc-state fine-doc-${state}`}>{item.upload ? `Stored · ${(item.upload.size_bytes / 1024).toFixed(1)} KiB` : item.error ? 'Not uploaded' : 'Uploading…'}</span></div>
                      <button className="btn btn-secondary" type="button" disabled={busy} aria-label={`Remove ${item.file.name}`} onClick={() => { removeSource(item.key); setRanges(({ [item.key]: _, ...rest }) => rest); }}>Remove</button>
                    </div>
                    {item.upload && <div className="fine-doc-pages" role="group" aria-label={`Pages of ${item.file.name}`}>
                      <span className="fine-doc-pages-label">Pages<ParamTooltip tooltipKey="fineTuningPageStart" /></span>
                      {pdf ? <>
                        <div className="fine-doc-toggle">
                          <label><input type="radio" name={`fine-pages-${index}`} checked={!range.use} disabled={busy} onChange={() => setRange({ use: false })} />All pages</label>
                          <label><input type="radio" name={`fine-pages-${index}`} checked={range.use} disabled={busy} onChange={() => setRange({ use: true })} />Page range</label>
                        </div>
                        {range.use && <span className="fine-doc-range">
                          <input id={`fine-page-start-${index}`} className="form-input" type="text" inputMode="numeric" pattern="[0-9]*" autoComplete="off" required placeholder="First" aria-label={`First page of ${item.file.name}`} value={range.start} disabled={busy} aria-invalid={!!errors.start} onChange={(event) => setRange({ start: digits(event.target.value) })} />
                          <span aria-hidden="true">to</span>
                          <input id={`fine-page-end-${index}`} className="form-input" type="text" inputMode="numeric" pattern="[0-9]*" autoComplete="off" required placeholder="Last" aria-label={`Last page of ${item.file.name}, inclusive`} value={range.end} disabled={busy} aria-invalid={!!errors.end} onChange={(event) => setRange({ end: digits(event.target.value) })} />
                        </span>}
                      </> : <span className="fine-doc-muted">Whole document · page ranges apply to PDFs</span>}
                    </div>}
                    {(errors.start || errors.end) && <p className="fine-error">{errors.start || errors.end}</p>}
                    {item.error && <p className="fine-error" role="alert">{item.upload ? `Could not start a preparation for this document: ${item.error}` : `Could not upload this document: ${item.error} Remove it and choose another.`}</p>}
                    {duplicates.has(item.key) && <p className="fine-error" role="alert">This document is already in the list. Remove one copy.</p>}
                  </li>;
                })}
              </ul>}
              {pdfs.length > 0 && <p className="fine-action-note">Use PDF viewer page numbers, not printed labels. The original files are unchanged.</p>}
              <div className="fine-actions">
                <button className={`btn ${documents.length ? 'btn-secondary' : 'btn-primary'}`} type="button" disabled={busy} onClick={() => preparedInput.current?.click()}>{documents.length ? 'Add another document' : 'Choose documents'}</button>
                {documentsUploading && <button className="btn btn-danger" type="button" onClick={cancelPreparation}>Cancel uploads</button>}
              </div>
              <p className="fine-action-note">PDF/DOCX need a server-side document extractor. Text and Markdown can be read directly.</p>
            </> : upload ? (
              <div className="fine-dataset-ready">
                <div><strong>{upload.name}</strong><span>Stored · {(upload.size_bytes / 1024).toFixed(1)} KiB · not yet validated for training</span></div>
                <button className="btn btn-secondary" type="button" disabled={busy} onClick={() => void chooseInput(null)}>Change dataset</button>
              </div>
            ) : (
              <div className="fine-actions">
                {uploading ? (
                  <>
                    <p role="status">Uploading {preparedFile?.name}…</p>
                    <button className="btn btn-danger" type="button" onClick={cancelPreparation}>Cancel Upload</button>
                  </>
                ) : (
                  <button className="btn btn-primary" type="button" disabled={busy} onClick={() => preparedInput.current?.click()}>Choose JSONL Dataset</button>
                )}
              </div>
            )}
          </section>

          {preparing && <section className="fine-panel" id="fine-destination-section" tabIndex={-1} aria-label="Save to dataset">
              <div className="fine-panel-heading"><div><span>Step 2</span><h3>{datasetID ? 'Adding to dataset' : 'Name your dataset'}</h3></div></div>
              {!datasetID && <div className="form-group">
                <FieldLabel htmlFor="fine-dataset-name" tooltipKey="fineTuningDatasetName">Dataset name (required)</FieldLabel>
                <input id="fine-dataset-name" className="form-input" required value={datasetName} disabled={busy || creationUncertain} aria-invalid={!!datasetName && !datasetName.trim()} aria-describedby="fine-name-feedback" placeholder="Give this dataset a name" onChange={(event) => setDatasetName(event.target.value)} />
                <p id="fine-name-feedback" className="fine-action-note">{datasetName.trim() ? 'The dataset is created when you generate the preview. To add to an existing dataset, open it from the dashboard and choose Add documents.' : 'Enter a dataset name to generate previews.'}</p>
              </div>}
              {datasetID && selectedDataset && <div className="fine-dataset-ready">
                <div><strong>{selectedDataset.name}</strong><span>{count(selectedDataset.documents.length, 'document')} · {count(selectedDataset.record_count, 'record')} · version {selectedDataset.version}</span></div>
              </div>}
              {datasetLoading && <p role="status">Loading dataset…</p>}
              {datasetError && <p className="fine-error" role="alert">{datasetError}</p>}
              {datasetID && !datasetLoading && !datasetError && !selectedDataset && <p className="fine-error" role="alert">This dataset is unavailable. <Link to="/fine-tuning">Return to the dashboard</Link></p>}
              {creationUncertain && <p className="fine-action-note" role="alert">Dataset creation could not be confirmed. Check the dashboard; if the dataset was created, open it and choose Add documents. A new dataset will not be created automatically.</p>}
              {datasetID && <div className="fine-actions"><button className="btn btn-secondary" type="button" disabled={busy || datasetLoading} onClick={() => { setRemoval(null); setDatasetAttempt((value) => value + 1); }}>Refresh dataset</button></div>}
              {selectedDataset && <div className="fine-recipe-details">
                <p className="fine-action-note"><strong>{selectedDataset.record_count} existing records stay as they are.</strong> Only the documents above are prepared, and each saves its records as a new version. Version {selectedDataset.version} stays available.</p>
                <details className="fine-document-management"><summary>Manage existing documents ({selectedDataset.documents.length})</summary>
                  <p className="fine-action-note">Optional: remove documents and their records from a future dataset version. This is separate from preparing your new source.</p>
                  {selectedDataset.documents.map((doc) => <div className="fine-dataset-ready" key={doc.id}>
                    <div><strong>{doc.name}</strong><span>{doc.page_start > 0 ? `Pages ${doc.page_start}–${doc.page_end}` : 'Entire document'} · {doc.record_count} records</span></div>
                    <button className="btn btn-secondary" type="button" disabled={busy || preparationUnfinished} onClick={() => setRemoval(doc)}>Remove from dataset</button>
                  </div>)}
                </details>
                {removal && <div className="fine-error" role="alert">
                  <strong>Remove {removal.name}?</strong>
                  <p>Its {removal.record_count} records will be removed from the next version of {selectedDataset.name}. Original files, previous dataset versions, and existing adapters are retained unchanged.</p>
                  <div className="fine-actions">
                    <button className="btn btn-danger" type="button" disabled={busy} onClick={() => void removeDocument()}>Remove document &amp; records</button>
                    <button className="btn btn-secondary" type="button" disabled={busy} onClick={() => setRemoval(null)}>Cancel</button>
                  </div>
                </div>}
              </div>}
          </section>}

          {preparing && (
            <section className="fine-panel fine-preparation-settings" aria-label="Preparation settings">
              <div className="fine-panel-heading"><div><span>Step 3</span><h3>Preparation settings</h3></div></div>
              <p className="fine-panel-copy">Choose an installed model to generate and verify records.</p>
              {catalogError && <div className="fine-error" role="alert">Could not load preparation models: {catalogError} <button className="btn btn-secondary" type="button" onClick={() => setCatalogAttempt((attempt) => attempt + 1)}>Retry Catalog</button></div>}
              {catalogLoading && <p role="status">Loading preparation models…</p>}
              {!catalogLoading && !catalogError && preparationModels.length === 0 && <p role="status">No installed, validated text-only chat models were reported by the Catalog. Image/audio/video, embedding, reranking, and decision models are excluded. Check model installation and validation in the Catalog, then select Refresh Catalog.</p>}
              <div className="form-group" id="fine-model-section" tabIndex={-1}>
                <FieldLabel htmlFor="fine-preparation-model" tooltipKey="fineTuningPreparationModel">Preparation model (required)</FieldLabel>
                <div className="fine-model-picker">
                <select id="fine-preparation-model" className="form-input" required value={preparationModelInstalled ? preparation.model : ''} disabled={busy || catalogLoading || !!catalogError} aria-describedby="fine-model-feedback" onChange={(event) => setPreparation((current) => ({ ...current, model: event.target.value }))}>
                  <option value="">Select a preparation model</option>
                  {preparationModels.map((model) => <option key={model.id} value={model.id}>{model.id} · Installed</option>)}
                </select>
                <button className="btn btn-secondary" type="button" disabled={busy || catalogLoading} onClick={() => setCatalogAttempt((attempt) => attempt + 1)}>Refresh Catalog</button>
                </div>
                <p id="fine-model-feedback" className="fine-action-note">{preparationModelInstalled ? 'This model generates records; it does not select the training base model.' : 'Select an installed, validated text-only chat model to enable Generate preview.'}</p>
              </div>
              <div className="fine-preparation-options">
              <div className="form-group">
                <FieldLabel htmlFor="fine-preparation-goal" tooltipKey="fineTuningGoal">Preparation goal</FieldLabel>
                <select id="fine-preparation-goal" className="form-input" value={preparation.goal} disabled={busy} onChange={(event) => setPreparation((current) => ({ ...current, goal: event.target.value as PreparationOptions['goal'] }))}>
                  <option value="question_answer">Question answering</option>
                  <option value="classification">Classification</option>
                  <option value="numeric_prediction">Numeric prediction</option>
                  <option value="text_generation">Text generation</option>
                </select>
              </div>
              <div className="form-group fine-preview-count">
                <FieldLabel htmlFor="fine-preview-count" tooltipKey="fineTuningPreviewCount">Preview examples</FieldLabel>
                <select id="fine-preview-count" className="form-input" value={preparation.preview_records} disabled={busy} onChange={(event) => setPreparation((current) => ({ ...current, preview_records: Number(event.target.value) }))}>
                  {[1, 2, 3, 4, 5].map((count) => <option key={count} value={count}>{count} {count === 1 ? 'example' : 'examples'}</option>)}
                </select>
              </div>
              </div>
              <div className="form-group">
                <FieldLabel htmlFor="fine-example-limit" tooltipKey="fineTuningMaxQuestions">Record limit (optional)</FieldLabel>
                <select id="fine-example-limit" className="form-input" value={limitExamples ? 'limit' : 'unlimited'} disabled={busy} onChange={(event) => setLimitExamples(event.target.value === 'limit')}>
                  <option value="unlimited">Unlimited</option>
                  <option value="limit">Limit records</option>
                </select>
                {limitExamples && <>
                  <FieldLabel htmlFor="fine-max-questions" tooltipKey="fineTuningMaxQuestions">Maximum records (required)</FieldLabel>
                  <input id="fine-max-questions" className="form-input" type="number" min={1} step={1} required value={maxQuestions} disabled={busy} aria-invalid={!!capError} aria-describedby={`fine-cap-help${capError ? ' fine-cap-error' : ''}`} onChange={(event) => setMaxQuestions(event.target.value)} />
                  {capError && <p id="fine-cap-error" className="fine-error">{capError}</p>}
                </>}
                <p id="fine-cap-help" className="fine-action-note">Applies only to this document, including preview and evaluation records. Verification may yield fewer records.</p>
                <details className="fine-recipe-details"><summary>What makes a dataset ready for training?</summary><p>Training needs at least 50 usable records: 20 training, 10 validation, and 20 protected test records, with valid fact coverage and splits. A cap of 20 tests preparation only. Preparation does not start training.</p></details>
              </div>

              <details className="fine-recipe-details">
                <summary>Preparation instructions (optional)</summary>
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

          {preparing && <section className="fine-panel fine-preview-readiness" id="fine-preview-readiness" tabIndex={-1} aria-label="Preview readiness">
              <h3>{ready && !busy ? `Ready to generate ${previews}` : `Before you generate ${previews}`}</h3>
              {startNotice && <p className="fine-action-note" role="status">{startNotice}</p>}
              <p className="fine-action-note">Review generated examples first. Accept and build dataset continues preparation after review; it does not start training.{uploaded.length > 1 ? ' Each document has its own preview to review, and each saves its own dataset version.' : ''}</p>
              {readinessIssues.length > 0 && <ul className="fine-readiness">
                {readinessIssues.map((issue) => <li key={issue.message}>{issue.field ? <button type="button" className="fine-readiness-link" onClick={() => goToField(issue.field!)}>{issue.message}</button> : issue.message}</li>)}
              </ul>}
            </section>}
            {preparing && <p className="fine-action-note">New records are saved after verification. Existing records are unchanged. No training starts, and Gemma is not required.</p>}
          </div>
          {preparing ? <div className="fine-preview-action-bar" aria-label="Generate preview">
              <div aria-live="polite"><strong>{ready && !busy && !startBlocked ? `Ready to generate ${previews}` : `${readinessIssues.length} ${readinessIssues.length === 1 ? 'requirement' : 'requirements'} remaining`}</strong>
                {readinessIssues[0] && (readinessIssues[0].field ? <button className="fine-readiness-link" type="button" onClick={() => goToField(readinessIssues[0].field!)}>{readinessIssues[0].message}</button> : <span>{readinessIssues[0].message}</span>)}
              </div>
              <button className="btn btn-primary" type="submit" aria-describedby="fine-preview-readiness" disabled={!ready || busy || startBlocked}>{submitting ? 'Submitting…' : `Generate ${previews}`}</button>
            </div> : <section className="fine-panel fine-generation-summary">
            <div><h3>{ready ? 'Ready to Submit' : 'Complete the Training Configuration'}</h3><p>{upload ? `${upload.name} will be submitted for Gemma 3 1B training.` : 'Choose a prepared JSONL dataset and install the Gemma 3 1B model.'}</p></div>
            <button className="btn btn-primary" type="submit" disabled={!ready || busy || !!trainingActivity || adopting}>{submitting ? 'Submitting…' : 'Start Training'}</button>
          </section>}
        </form>
        </div>
      )}

      {view === 'configure' && !showTrainingSetup && trainingJob && (
        <div className="fine-train-view">
          {watchError && <div className="fine-error" role="alert">{watchError}</div>}
          {visible && <TrainingJobState run={trainingJob} onRefresh={() => void refreshRun(trainingJob)} view="training" />}
          {['completed', 'needs_improvement', 'failed', 'stopped'].includes(trainingJob.status) && <button className="btn btn-secondary" type="button" disabled={busy} onClick={() => { void chooseInput(null); setTrainingSetup(true); }}>Configure Another Training</button>}
        </div>
      )}
    </div>
  );
}
