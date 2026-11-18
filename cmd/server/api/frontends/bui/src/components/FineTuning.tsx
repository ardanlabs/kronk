import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Link, Navigate, useSearchParams } from 'react-router-dom';
import { DEFAULT_TRAINING_OPTIONS, requestMessage, useFineTuningRunner } from '../contexts/FineTuningRunnerContext';
import { api, ApiError } from '../services/api';
import type { CatalogModelResponse, PreparationDataset, PreparationOptions, TrainingRun } from '../types';
import { FieldLabel, ParamTooltip } from './ParamTooltips';
import DatasetDashboard from './DatasetDashboard';

// Match the runner's upload lifetime, including navigation away from this page.
// An uncertain create is never repeated; a confirmed destination is reused.
const uploadDestinations = new Map<string, { id?: string; uncertain: boolean }>();
// digits keeps only 0–9, so page fields never hold letters or signs, even when pasted.
const digits = (value: string) => value.replace(/\D/g, '');
const count = (n: number, noun: string) => `${n} ${noun}${n === 1 ? '' : 's'}`;

export default function FineTuning() {
  const [params] = useSearchParams();
  const view = params.get('view');
  const workspaceVisible = params.get('workspace') === 'live' || view === 'prepare';
  const [workspaceVisited, setWorkspaceVisited] = useState(workspaceVisible);
  useEffect(() => { if (workspaceVisible) setWorkspaceVisited(true); }, [workspaceVisible]);
  // History is part of the dashboard now, and every run opens on the dashboard.
  if (view === 'history') return <Navigate replace to="/fine-tuning" />;
  // Training starts from a finished dataset now.
  if (view === 'configure' || view === 'run') return <Navigate replace to="/fine-tuning" />;
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
  const { documents, documentsUploading, submitting, preparationRun, addDocuments, removeDocument: removeSource, cancelPreparation, startPreparations } = useFineTuningRunner();
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
  const preparedInput = useRef<HTMLInputElement>(null);
  const datasetID = searchParams.get('dataset') || '';
  // Document uploads run on their own, so more documents can be chosen meanwhile.
  const busy = submitting || datasetBusy;
  const preparationUnfinished = !!preparationRun && !['completed', 'needs_improvement', 'failed', 'stopped'].includes(preparationRun.status);
  const uploaded = documents.filter((item) => item.upload);
  const uploadFailed = documents.filter((item) => !item.upload && item.error);
  // The same file twice would be prepared twice; the second save would then be rejected.
  const duplicates = new Set(uploaded.filter((item, index) => uploaded.findIndex((other) => other.upload!.sha256 === item.upload!.sha256) !== index).map((item) => item.key));
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
  const ready = uploaded.length > 0 && !documentsUploading && !uploadFailed.length && !duplicates.size && settingsValid && !catalogLoading && !catalogError && preparationModelInstalled && !datasetLoading && !datasetError && !creationUncertain && (datasetID ? selectedDataset?.id === datasetID : !!datasetName.trim());

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

  useEffect(() => {
    let cancelled = false;
    setCatalogLoading(true);
    setCatalogError('');
    api.listCatalog()
      .then((result) => { if (!cancelled) setCatalog(result); })
      .catch((err) => { if (!cancelled) setCatalogError(err instanceof Error ? err.message : 'Could not load preparation models'); })
      .finally(() => { if (!cancelled) setCatalogLoading(false); });
    return () => { cancelled = true; };
  }, [catalogAttempt]);

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
    if (!ready || datasetPending.current) return;
    datasetPending.current = true;
    setDatasetBusy(true);
    try {
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
      const runs = await startPreparations(DEFAULT_TRAINING_OPTIONS, (item) => {
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

  // Every run opens on the dashboard's run screen.
  const showPreparation = (run: TrainingRun, replace = false) => setSearchParams({ preparation: run.workflow_id, run: run.run_id }, { replace });

  return (
    <div className="fine-page fine-preparing">
      <div className="page-header fine-page-header">
        <div>
          <h2>Fine-Tuning</h2>
        </div>
      </div>

      <div className="page-header-with-action prep-heading">
        <div>
          <Link className="prep-back" to="/fine-tuning">← Dashboard</Link>
          <h2>{datasetID ? 'Add documents' : 'New dataset'}</h2>
          <p className="page-description">{datasetID ? `Choose documents to add to ${selectedDataset?.name ?? 'this dataset'}, then generate previews to review.` : 'Choose documents and where their records go, then generate previews to review.'}</p>
        </div>
      </div>
      <div className="fine-setup-wrapper">
        <form className="fine-configure-view" onSubmit={start}>
          <div className="fine-preparation-fields">
          <section className="fine-panel fine-configure-intro" id="fine-source" tabIndex={-1}>
            <div className="fine-panel-heading">
              <div><span>Step 1</span><h3>Source documents</h3></div>
              <span className={`fine-status ${uploaded.length && !documentsUploading ? 'fine-status-completed' : 'fine-status-stopped'}`}>{documentsUploading ? 'Uploading…' : uploaded.length ? count(uploaded.length, 'document') : 'Document needed'}</span>
            </div>
            <p className="fine-panel-copy">Choose one or more PDF, DOCX, Markdown, or text documents to turn into grounded records. Each document is prepared as its own run, with its own preview to review.</p>
            <div className="form-group fine-file-field">
              <FieldLabel htmlFor="fine-prepared-dataset" tooltipKey="fineTuningSource">Source documents (at least one)</FieldLabel>
              <input ref={preparedInput} className="fine-hidden-input" id="fine-prepared-dataset" type="file" multiple accept=".pdf,.docx,.md,.txt" disabled={busy} onChange={(event) => {
                const files = [...(event.target.files ?? [])];
                if (files.length) {
                  setStartNotice('');
                  addDocuments(files);
                }
                event.currentTarget.value = '';
              }} />
            </div>
            <>
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
            </>
          </section>

          <section className="fine-panel" id="fine-destination-section" tabIndex={-1} aria-label="Save to dataset">
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
          </section>

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
                <p id="fine-cap-help" className="fine-action-note">Counts verified records from this document, including reviewed records and the evaluation questions Kronk adds. Rejected records are replaced while passages remain.</p>
                <details className="fine-recipe-details"><summary>What makes a dataset ready for training?</summary><p>Training needs at least 20 training records. Kronk also adds evaluation questions that check the trained model against the base model: test questions for 5% of the training records (16 to 128) and half as many validation questions (at least 8). With a record limit they count toward it, so a new dataset needs a limit of 44 to get both; with less room it trains without that check. Preparation does not start training.</p></details>
              </div>

              <details className="fine-recipe-details">
                <summary>Preparation instructions (optional)</summary>
                <div className="form-group">
                  <FieldLabel htmlFor="fine-preparation-instructions" tooltipKey="fineTuningDatasetInstructions">Instructions</FieldLabel>
                  <textarea id="fine-preparation-instructions" className="form-input" rows={4} value={preparation.instructions} disabled={busy} onChange={(event) => setPreparation((current) => ({ ...current, instructions: event.target.value }))} />
                </div>
              </details>
            </section>

          <section className="fine-panel fine-preview-readiness" id="fine-preview-readiness" tabIndex={-1} aria-label="Preview readiness">
              <h3>{ready && !busy ? `Ready to generate ${previews}` : `Before you generate ${previews}`}</h3>
              {startNotice && <p className="fine-action-note" role="status">{startNotice}</p>}
              <p className="fine-action-note">Review generated examples first. Accept and build dataset continues preparation after review; it does not start training.{uploaded.length > 1 ? ' Each document has its own preview to review, and each saves its own dataset version.' : ''}</p>
              {readinessIssues.length > 0 && <ul className="fine-readiness">
                {readinessIssues.map((issue) => <li key={issue.message}>{issue.field ? <button type="button" className="fine-readiness-link" onClick={() => goToField(issue.field!)}>{issue.message}</button> : issue.message}</li>)}
              </ul>}
            </section>
            <p className="fine-action-note">New records are saved after verification. Existing records are unchanged. No training starts, and Gemma is not required.</p>
          </div>
          <div className="fine-preview-action-bar" aria-label="Generate preview">
              <div aria-live="polite"><strong>{ready && !busy ? `Ready to generate ${previews}` : `${readinessIssues.length} ${readinessIssues.length === 1 ? 'requirement' : 'requirements'} remaining`}</strong>
                {readinessIssues[0] && (readinessIssues[0].field ? <button className="fine-readiness-link" type="button" onClick={() => goToField(readinessIssues[0].field!)}>{readinessIssues[0].message}</button> : <span>{readinessIssues[0].message}</span>)}
              </div>
              <button className="btn btn-primary" type="submit" aria-describedby="fine-preview-readiness" disabled={!ready || busy}>{submitting ? 'Submitting…' : `Generate ${previews}`}</button>
            </div>
        </form>
      </div>
    </div>
  );
}
