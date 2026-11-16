import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { FINE_TUNING_MODEL_ID, TESTED_TRAINING_OPTIONS, preparationPath, useFineTuningRunner } from '../contexts/FineTuningRunnerContext';
import { useModelList } from '../contexts/ModelListContext';
import { api, ApiError } from '../services/api';
import type { CatalogModelResponse, PreparationDataset, PreparationOptions, TrainingOptions, TrainingRun } from '../types';
import { FieldLabel } from './ParamTooltips';
import PreparationDatasetHistory from './PreparationDatasetHistory';
import TrainingHistory from './TrainingHistory';
import { StartPreparedTraining, TrainingJobState } from './TrainingRun';
import PreparationRuns, { PreparationRunView } from './PreparationRuns';

type View = 'prepare' | 'configure' | 'history';

// Match the runner's upload lifetime, including navigation away from this page.
// An uncertain create is never repeated; a confirmed destination is reused.
const uploadDestinations = new Map<string, { id?: string; uncertain: boolean }>();

export default function FineTuning() {
  const [params] = useSearchParams();
  const view = params.get('view');
  const workspaceVisible = params.get('workspace') === 'live' || view === 'prepare' || view === 'configure' || view === 'history' || view === 'run';
  const [workspaceVisited, setWorkspaceVisited] = useState(workspaceVisible);
  useEffect(() => { if (workspaceVisible) setWorkspaceVisited(true); }, [workspaceVisible]);
  return <>
    <div hidden={workspaceVisible}><PreparationRuns visible={!workspaceVisible} /></div>
    <div hidden={!workspaceVisible}>
      {(workspaceVisible || workspaceVisited) && <FineTuningWorkspace visible={workspaceVisible} />}
    </div>
  </>;
}

function FineTuningWorkspace({ visible }: { visible: boolean }) {
  const { models, loading: modelsLoading, error: modelsError, loadModels } = useModelList();
  const {
    inputKind, preparedFile, upload, uploading, submitting, trainingRun, preparationRun, error, watchError, trainingActivity,
    chooseInput, cancelPreparation, startRun, selectRun, refreshRun,
  } = useFineTuningRunner();
  const [training, setTraining] = useState<TrainingOptions>(TESTED_TRAINING_OPTIONS);
  const [preparation, setPreparation] = useState<PreparationOptions>({ model: '', goal: 'question_answer', instructions: '', preview_records: 3 });
  const [maxQuestions, setMaxQuestions] = useState('');
  const [limitExamples, setLimitExamples] = useState(false);
  const [usePageRange, setUsePageRange] = useState(false);
  const [pageStart, setPageStart] = useState('');
  const [pageEnd, setPageEnd] = useState('');
  const [catalog, setCatalog] = useState<CatalogModelResponse[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(false);
  const [catalogError, setCatalogError] = useState('');
  const [catalogAttempt, setCatalogAttempt] = useState(0);
  const [inspectedPreparation, setInspectedPreparation] = useState<TrainingRun | null>(null);
  const [inspectionError, setInspectionError] = useState('');
  const [runLoadError, setRunLoadError] = useState('');
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
  const requestedView = searchParams.get('view');
  const view: View = requestedView === 'history' ? 'history' : requestedView === 'configure' || requestedView === 'run' ? 'configure' : 'prepare';
  const setView = (next: View, replace = false) => setSearchParams({ view: next }, { replace });
  const preparedInput = useRef<HTMLInputElement>(null);
  const busy = uploading || submitting || datasetBusy;
  const preparing = view === 'prepare';
  // Prepare Dataset shows the run named in the URL, or the form when the URL names none.
  // The hidden workspace ignores the URL, which then belongs to Preparation runs.
  const requestedWorkflow = visible && preparing ? searchParams.get('preparation') : null;
  const requestedRun = visible && preparing ? searchParams.get('run') : null;
  const requestedKey = requestedWorkflow && requestedRun ? `${requestedWorkflow}/${requestedRun}` : '';
  const keyOf = (run: TrainingRun | null) => run ? `${run.workflow_id}/${run.run_id}` : '';
  const displayedPreparation = !requestedKey ? null
    : requestedKey === keyOf(preparationRun) ? preparationRun
    : requestedKey === keyOf(inspectedPreparation) ? inspectedPreparation : null;
  const showSourceSetup = !requestedKey;
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
  const positiveInteger = (value: string) => /^\d+$/.test(value) && Number.isSafeInteger(Number(value)) && Number(value) > 0;
  const capError = limitExamples && !positiveInteger(maxQuestions) ? 'Enter a positive whole number, or choose Unlimited.' : '';
  const pageStartError = pdfSource && usePageRange && !positiveInteger(pageStart) ? 'Enter a positive whole-number first page.' : '';
  const pageEndError = pdfSource && usePageRange && (!positiveInteger(pageEnd) || (!pageStartError && Number(pageEnd) < Number(pageStart))) ? 'Enter a positive whole-number last page at or after the first page.' : '';
  const settingsValid = !capError && !pageStartError && !pageEndError;
  const ready = matchingUpload && !!upload && (preparing
    ? settingsValid && !catalogLoading && !catalogError && preparationModelInstalled && !datasetLoading && !datasetError && !creationUncertain && (!!selectedDataset || (!datasetID && !!datasetName.trim()))
    : modelInstalled);

  useEffect(() => {
    if (inputKind !== 'document' || !upload) return;
    const destination = uploadDestinations.get(upload.id);
    if (destination?.id) setDatasetID(destination.id);
    setCreationUncertain(destination?.uncertain ?? false);
  }, [inputKind, upload]);

  useEffect(() => {
    if (!visible || !preparing || !showSourceSetup) return;
    const controller = new AbortController();
    setDatasetLoading(true);
    setDatasetError('');
    api.preparationDatasets(controller.signal).then((result) => { if (!controller.signal.aborted) setDatasets(result.datasets ?? []); })
      .catch((err) => { if (!controller.signal.aborted) setDatasetError(err instanceof Error ? err.message : 'Could not load datasets.'); })
      .finally(() => { if (!controller.signal.aborted) setDatasetLoading(false); });
    return () => controller.abort();
  }, [visible, preparing, showSourceSetup, datasetAttempt]);

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
  if (!matchingUpload || !upload) readinessIssues.push({ message: uploading ? 'Wait for the document upload.' : 'Choose a source document.', field: 'fine-source' });
  if (datasetLoading) readinessIssues.push({ message: 'Wait for datasets to load.' });
  else if (datasetError || creationUncertain) readinessIssues.push({ message: 'Review the dataset destination.', field: 'fine-destination-section' });
  else if (datasetID ? !selectedDataset : !datasetName.trim()) readinessIssues.push({ message: datasetID ? 'Choose an available dataset.' : 'Name the new dataset.', field: datasetID ? 'fine-destination' : 'fine-dataset-name' });
  if (catalogLoading) readinessIssues.push({ message: 'Wait for preparation models to load.' });
  else if (catalogError || !preparationModelInstalled) readinessIssues.push({ message: catalogError ? 'Retry the preparation model catalog.' : 'Select a preparation model.', field: 'fine-model-section' });
  if (capError) readinessIssues.push({ message: capError, field: 'fine-max-questions' });
  if (pageStartError) readinessIssues.push({ message: pageStartError, field: 'fine-page-start' });
  if (pageEndError) readinessIssues.push({ message: pageEndError, field: 'fine-page-end' });
  if (preparationPending) readinessIssues.push({ message: 'Wait for the current preparation to finish.' });
  if (busy && !uploading) readinessIssues.push({ message: 'Wait for the current operation to finish.' });
  const goToField = (id: string) => {
    const field = document.getElementById(id);
    const scroller = field?.closest('.fine-preparation-fields');
    if (field && scroller) scroller.scrollTo({ top: scroller.scrollTop + field.getBoundingClientRect().top - scroller.getBoundingClientRect().top - (scroller.clientHeight - field.clientHeight) / 2 });
    field?.focus({ preventScroll: true });
  };

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
      const run = await startRun(training, preparing ? {
        ...preparation, target_dataset_id: destination!.id, base_version: destination!.version,
        max_questions: limitExamples ? Number(maxQuestions) : 0,
        ...(pdfSource && usePageRange ? { page_start: Number(pageStart), page_end: Number(pageEnd) } : {}),
      } : undefined);
      if (run && preparing) showPreparation(run, true);
      if (run) setTrainingSetup(false);
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
    setView('prepare');
  };

  const finished = (run: TrainingRun) => ['completed', 'needs_improvement', 'failed', 'stopped'].includes(run.status);

  // showRun makes a run the current one, so it is watched live. A finished run opened
  // while another preparation is active is only inspected, so the active one stays current.
  const showRun = (run: TrainingRun) => {
    const inspect = preparationActive && finished(run) && keyOf(run) !== keyOf(preparationRun);
    if (inspect) {
      setInspectedPreparation(run);
    } else {
      setInspectedPreparation(null);
      selectRun(run);
    }
    setInspectionError('');
    // History rows omit the preview and artifacts; load the full run.
    if (!inspect) { void refreshRun(run); return; }
    api.trainingRun(run.workflow_id, run.run_id)
      .then((updated) => setInspectedPreparation((current) => current?.workflow_id === updated.workflow_id && current.run_id === updated.run_id ? updated : current))
      .catch((err) => setInspectionError(err instanceof Error ? err.message : 'Could not load this preparation.'));
  };

  const showPreparation = (run: TrainingRun, replace = false) =>
    setSearchParams({ view: 'prepare', preparation: run.workflow_id, run: run.run_id }, { replace });

  const refreshPreparation = async (run: TrainingRun) => {
    if (!inspectedPreparation || run !== inspectedPreparation) { await refreshRun(run); return; }
    setInspectionError('');
    try {
      const updated = await api.trainingRun(inspectedPreparation.workflow_id, inspectedPreparation.run_id);
      setInspectedPreparation((current) => current?.workflow_id === updated.workflow_id && current.run_id === updated.run_id ? updated : current);
    } catch (err) {
      setInspectionError(err instanceof Error ? err.message : 'Could not refresh this preparation.');
    }
  };

  // A reload or link names a run the page does not hold yet; load it.
  useEffect(() => {
    setRunLoadError('');
    if (!requestedKey) return;
    if (requestedKey === keyOf(preparationRun)) { setInspectedPreparation(null); return; }
    if (displayedPreparation) return;
    const controller = new AbortController();
    api.trainingRun(requestedWorkflow!, requestedRun!, controller.signal)
      .then((run) => { if (!controller.signal.aborted) showRun(run); })
      .catch((err) => {
        if (controller.signal.aborted) return;
        setRunLoadError(err instanceof ApiError && err.status === 404 ? 'This preparation run is no longer available.' : err instanceof Error ? err.message : 'Could not load this preparation run.');
      });
    return () => controller.abort();
  }, [requestedKey]);

  const updateTraining = (key: keyof TrainingOptions, value: number) => {
    setTraining((current) => ({ ...current, [key]: value }));
  };

  return (
    <div className={`fine-page${preparing && showSourceSetup ? ' fine-preparing' : ''}`}>
      <div className="page-header fine-page-header">
        <div>
          <h2>Fine-Tuning</h2>
        </div>
      </div>

      <nav className="tabs fine-tabs" aria-label="Fine-tuning views">
        <Link className="tab" to="/fine-tuning">Preparation runs</Link>
        {(['prepare', 'configure', 'history'] as View[]).map((item) => (
          <Link key={item} aria-current={visible && view === item ? 'page' : undefined} className={`tab ${view === item ? 'active' : ''}`} to={item === 'prepare' && preparationActive && preparationRun ? preparationPath(preparationRun) : `/fine-tuning?view=${item}`}>
            {item === 'prepare' ? 'Prepare Dataset' : item === 'configure' ? 'Configure Training' : 'History'}
          </Link>
        ))}
      </nav>

      {error && !(preparing && showSourceSetup && inputKind === 'document' && !upload) && <div className="fine-error" role="alert">{error}</div>}
      {modelsError && <div className="fine-error" role="alert">Could not load installed models: {modelsError}</div>}
      {requestedKey && !displayedPreparation && (runLoadError
        ? <div className="fine-error" role="alert">{runLoadError} <Link to="/fine-tuning">Return to preparation runs</Link></div>
        : <p className="prep-copy" role="status">Loading run…</p>)}

      {preparationRun && preparationActive && displayedPreparation !== preparationRun && (
        <section className="fine-panel fine-current-preparation" aria-label="Current preparation">
          <div className="fine-panel-heading">
            <div><span>Preparation in progress</span><h3>Your preparation has not been canceled</h3></div>
            <span className={`fine-status fine-status-${preparationRun.status}`}>{preparationRun.control_action === 'pause' ? 'paused' : preparationRun.status.replaceAll('_', ' ')}</span>
          </div>
          <p className="fine-panel-copy">Opening another dataset setup or browsing History does not cancel this run. You can configure another dataset, but must wait for the current preparation to finish before starting it.</p>
          {preparationRun.status === 'waiting' && preparationRun.control_action !== 'pause' && <p className="fine-action-note">This run is waiting for your decision. Return to the process screen to continue.</p>}
          {(preparationRun.state_stale || watchError) && <p className="fine-error" role="alert">{watchError || 'This status may be out of date.'} Open the current preparation and select Refresh to check its latest status.</p>}
          <div className="fine-actions"><button className="btn btn-primary" type="button" onClick={() => showPreparation(preparationRun)}>View current preparation</button></div>
        </section>
      )}

      {((view === 'prepare' && showSourceSetup) || (view === 'configure' && showTrainingSetup)) && (
        <div className="fine-setup-wrapper">
        {!preparing && preparationRun && !preparationActive && <p className="fine-action-note fine-independent-training">Train with a prepared dataset independently of your document job. <button className="btn btn-secondary" type="button" onClick={() => showPreparation(preparationRun)}>View Document Preparation</button></p>}
        <form className="fine-configure-view" onSubmit={start}>
          <div className="fine-preparation-fields">
          <section className="fine-panel fine-configure-intro" id="fine-source" tabIndex={-1}>
            <div className="fine-panel-heading">
              <div><span>{preparing ? 'Step 1' : 'Required'}</span><h3>{preparing ? 'Source document' : 'Choose Training Dataset'}</h3></div>
              <span className={`fine-status ${matchingUpload && upload ? 'fine-status-completed' : 'fine-status-stopped'}`}>{matchingUpload && upload ? 'Uploaded' : preparing ? 'Document needed' : 'Dataset needed'}</span>
            </div>
            <p className="fine-panel-copy">{preparing ? 'Choose a PDF, DOCX, Markdown, or text document to turn into grounded records.' : 'Choose a training-ready JSONL file. The training service validates its contents when you select Start.'}</p>
            <div className="form-group fine-file-field">
              <FieldLabel htmlFor="fine-prepared-dataset" tooltipKey={preparing ? 'fineTuningSource' : 'fineTuningDataset'}>{preparing ? 'Source document (required)' : 'Training-Ready JSONL'}</FieldLabel>
              <input ref={preparedInput} className="fine-hidden-input" id="fine-prepared-dataset" type="file" accept={preparing ? '.pdf,.docx,.md,.txt' : '.jsonl,application/x-ndjson'} disabled={busy} onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) {
                  if (preparing) {
                    setUsePageRange(false);
                    setPageStart('');
                    setPageEnd('');
                  }
                  void chooseInput(file, preparing ? 'document' : 'dataset');
                }
                event.currentTarget.value = '';
              }} />
            </div>
            {matchingUpload && upload ? (
              <div className="fine-dataset-ready">
                <div><strong>{upload.name}</strong><span>Stored · {(upload.size_bytes / 1024).toFixed(1)} KiB · {preparing ? 'not yet prepared' : 'not yet validated for training'}</span></div>
                <button className="btn btn-secondary" type="button" disabled={busy} onClick={() => preparing ? preparedInput.current?.click() : void chooseInput(null, inputKind)}>{preparing ? 'Change document' : 'Change dataset'}</button>
              </div>
            ) : (
              <div className="fine-actions">
                {matchingUpload && uploading ? (
                  <>
                    <p role="status">Uploading {preparedFile?.name}…</p>
                    <button className="btn btn-danger" type="button" onClick={cancelPreparation}>Cancel Upload</button>
                  </>
                ) : (
                  <button className="btn btn-primary" type="button" disabled={busy} onClick={() => preparedInput.current?.click()}>{preparing ? 'Choose document' : 'Choose JSONL Dataset'}</button>
                )}
              </div>
            )}
            {preparing && inputKind === 'document' && !upload && error && <p className="fine-error" role="alert">Could not upload {preparedFile?.name || 'this document'}: {error} Choose a document to try again.</p>}
            {preparing && <p className="fine-action-note">PDF/DOCX need a server-side document extractor. Text and Markdown can be read directly.</p>}
            {preparing && pdfSource && <div className="form-group fine-source-scope">
              <FieldLabel htmlFor="fine-pdf-scope" tooltipKey="fineTuningPageStart">PDF pages</FieldLabel>
              <select id="fine-pdf-scope" className="form-input" value={usePageRange ? 'range' : 'whole'} disabled={busy} onChange={(event) => setUsePageRange(event.target.value === 'range')}>
                <option value="whole">Whole PDF</option>
                <option value="range">Page range</option>
              </select>
              {usePageRange && <div className="fine-form-grid">
                <div className="form-group">
                  <FieldLabel htmlFor="fine-page-start" tooltipKey="fineTuningPageStart">First page (required)</FieldLabel>
                  <input id="fine-page-start" className="form-input" type="number" min={1} step={1} required value={pageStart} disabled={busy} aria-invalid={!!pageStartError} aria-describedby={`fine-page-help${pageStartError ? ' fine-page-start-error' : ''}`} onChange={(event) => setPageStart(event.target.value)} />
                  {pageStartError && <p id="fine-page-start-error" className="fine-error">{pageStartError}</p>}
                </div>
                <div className="form-group">
                  <FieldLabel htmlFor="fine-page-end" tooltipKey="fineTuningPageEnd">Last page, inclusive (required)</FieldLabel>
                  <input id="fine-page-end" className="form-input" type="number" min={Math.max(1, Number(pageStart) || 1)} step={1} required value={pageEnd} disabled={busy} aria-invalid={!!pageEndError} aria-describedby={`fine-page-help${pageEndError ? ' fine-page-end-error' : ''}`} onChange={(event) => setPageEnd(event.target.value)} />
                  {pageEndError && <p id="fine-page-end-error" className="fine-error">{pageEndError}</p>}
                </div>
              </div>}
              <p id="fine-page-help" className="fine-action-note">Use PDF viewer page numbers, not printed labels. The original file is unchanged.</p>
            </div>}
          </section>

          {preparing && <section className="fine-panel" id="fine-destination-section" tabIndex={-1} aria-label="Save to dataset">
              <div className="fine-panel-heading"><div><span>Step 2</span><h3>Save to dataset</h3></div></div>
              <div className="fine-preparation-options">
              <div className="form-group">
                <FieldLabel htmlFor="fine-destination" tooltipKey="fineTuningDestination">Save records to</FieldLabel>
                <select id="fine-destination" className="form-input" value={datasetID} disabled={busy || datasetLoading} aria-describedby="fine-destination-feedback" onChange={(event) => { setDatasetID(event.target.value); setRemoval(null); if (event.target.value) setCreationUncertain(false); }}>
                  <option value="">Create a new dataset</option>
                  {datasets.map((dataset) => <option key={dataset.id} value={dataset.id}>{dataset.name} · {dataset.documents.length} documents · {dataset.record_count} records</option>)}
                </select>
                <p id="fine-destination-feedback" className="fine-action-note">{datasetID && !selectedDataset && !datasetLoading ? 'This dataset is unavailable. Refresh datasets or choose another destination.' : 'Add this document to an existing dataset or create a new one.'}</p>
              </div>
              {!datasetID && <div className="form-group">
                <FieldLabel htmlFor="fine-dataset-name" tooltipKey="fineTuningDatasetName">Dataset name (required)</FieldLabel>
                <input id="fine-dataset-name" className="form-input" required value={datasetName} disabled={busy || creationUncertain} aria-invalid={!!datasetName && !datasetName.trim()} aria-describedby="fine-name-feedback" placeholder="Give this dataset a name" onChange={(event) => setDatasetName(event.target.value)} />
                <p id="fine-name-feedback" className="fine-action-note">{datasetName.trim() ? 'The dataset is created when you generate the preview.' : 'Enter a dataset name to enable Generate preview.'}</p>
              </div>}
              </div>
              {datasetLoading && <p role="status">Loading datasets…</p>}
              {datasetError && <p className="fine-error" role="alert">{datasetError}</p>}
              {creationUncertain && <p className="fine-action-note" role="alert">Dataset creation could not be confirmed. Refresh datasets and select the dataset that was created before submitting again. A new dataset will not be created automatically.</p>}
              <div className="fine-actions"><button className="btn btn-secondary" type="button" disabled={busy || datasetLoading} onClick={() => { setRemoval(null); setDatasetAttempt((value) => value + 1); }}>Refresh datasets</button></div>
              {selectedDataset && <div className="fine-recipe-details">
                <p className="fine-action-note"><strong>{selectedDataset.record_count} existing records stay as they are.</strong> Only the selected document will be prepared. Version {selectedDataset.version} is retained.</p>
                <details className="fine-document-management"><summary>Manage existing documents ({selectedDataset.documents.length})</summary>
                  <p className="fine-action-note">Optional: remove documents and their records from a future dataset version. This is separate from preparing your new source.</p>
                  {selectedDataset.documents.map((doc) => <div className="fine-dataset-ready" key={doc.id}>
                    <div><strong>{doc.name}</strong><span>{doc.page_start > 0 ? `Pages ${doc.page_start}–${doc.page_end}` : 'Entire document'} · {doc.record_count} records</span></div>
                    <button className="btn btn-secondary" type="button" disabled={busy || preparationPending} onClick={() => setRemoval(doc)}>Remove from dataset</button>
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
              <h3>{preparationPending ? 'Current preparation must finish first' : ready && !busy ? 'Ready to generate preview' : 'Before you generate preview'}</h3>
              <p className="fine-action-note">Review generated examples first. Build dataset continues preparation after review; it does not start training.</p>
              {readinessIssues.length > 0 && <ul className="fine-readiness">
                {readinessIssues.map((issue) => <li key={issue.message}>{issue.field ? <button type="button" className="fine-readiness-link" onClick={() => goToField(issue.field!)}>{issue.message}</button> : issue.message}</li>)}
              </ul>}
            </section>}
            {preparing && <p className="fine-action-note">New records are saved after verification. Existing records are unchanged. No training starts, and Gemma is not required.</p>}
          </div>
          {preparing ? <div className="fine-preview-action-bar" aria-label="Generate preview">
              <div aria-live="polite"><strong>{ready && !busy && !preparationPending ? 'Ready to generate preview' : `${readinessIssues.length} ${readinessIssues.length === 1 ? 'requirement' : 'requirements'} remaining`}</strong>
                {readinessIssues[0] && (readinessIssues[0].field ? <button className="fine-readiness-link" type="button" onClick={() => goToField(readinessIssues[0].field!)}>{readinessIssues[0].message}</button> : <span>{readinessIssues[0].message}</span>)}
              </div>
              <button className="btn btn-primary" type="submit" aria-describedby="fine-preview-readiness" disabled={!ready || busy || preparationPending}>{submitting ? 'Submitting…' : 'Generate preview'}</button>
            </div> : <section className="fine-panel fine-generation-summary">
            <div><h3>{ready ? 'Ready to Submit' : 'Complete the Training Configuration'}</h3><p>{matchingUpload && upload ? `${upload.name} will be submitted for Gemma 3 1B training.` : 'Choose a prepared JSONL dataset and install the Gemma 3 1B model.'}</p></div>
            <button className="btn btn-primary" type="submit" disabled={!ready || busy || !!trainingActivity}>{submitting ? 'Submitting…' : 'Start Training'}</button>
          </section>}
        </form>
        </div>
      )}

      {inspectedPreparation && ((view === 'prepare' && displayedPreparation === inspectedPreparation) || (view === 'configure' && !showTrainingSetup && trainingJob === inspectedPreparation)) && <p className="fine-action-note">You are viewing an earlier run. Your current preparation is tracked separately above.</p>}

      {view === 'prepare' && displayedPreparation && (
        <>
          {watchError && <div className="fine-error" role="alert">{watchError}</div>}
          {inspectionError && <div className="fine-error" role="alert">{inspectionError}</div>}
          {displayedPreparation.preparation_only
            ? <PreparationRunView key={keyOf(displayedPreparation)} heading run={displayedPreparation} visible={visible} onRefresh={() => void refreshPreparation(displayedPreparation)}
              openReview={searchParams.get('review') === '1'} onReviewOpened={() => showPreparation(displayedPreparation, true)} />
            : visible && <TrainingJobState run={displayedPreparation} onRefresh={() => void refreshPreparation(displayedPreparation)} view="preparation" />}
          {!displayedPreparation.preparation_only && <StartPreparedTraining run={displayedPreparation} onRefresh={() => void refreshPreparation(displayedPreparation)} onStarted={() => { selectRun(displayedPreparation); setTrainingSetup(false); setView('configure'); }} />}
        </>
      )}
      {view === 'configure' && !showTrainingSetup && trainingJob && (
        <div className="fine-train-view">
          {watchError && <div className="fine-error" role="alert">{watchError}</div>}
          {inspectionError && <div className="fine-error" role="alert">{inspectionError}</div>}
          {visible && <TrainingJobState run={trainingJob} onRefresh={() => void refreshPreparation(trainingJob)} view="training" />}
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
          <TrainingHistory preparationOnly onSelect={(run) => finished(run) ? setSearchParams({ preparation: run.workflow_id, run: run.run_id }) : showPreparation(run)} />
        </> : <TrainingHistory onSelect={(run) => {
          if (run.stages.some((stage) => stage.name === 'preparation') && run.stage !== 'training') {
            showPreparation(run);
          } else if (run.stages.some((stage) => stage.name === 'preparation')) {
            showRun(run);
            setTrainingSetup(false);
            setView('configure');
          } else {
            setInspectedPreparation(null);
            selectRun(run);
            void refreshRun(run);
            setTrainingSetup(false);
            setView('configure');
          }
        }} />}
      </>}
    </div>
  );
}
