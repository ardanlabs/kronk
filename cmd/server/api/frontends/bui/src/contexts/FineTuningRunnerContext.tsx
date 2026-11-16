import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { api, ApiError } from '../services/api';
import type { ContinueTrainingPreview, PreparationOptions, StartPreparedTraining, TrainingOptions, TrainingPreviewRecord, TrainingRun, TrainingUpload } from '../types';

export const FINE_TUNING_MODEL_ID = 'ggml-org/gemma-3-1b-it-Q8_0';

// Initial form values, not model policy.
export const TESTED_TRAINING_OPTIONS: TrainingOptions = {
  maximum_epochs: 1,
  batch_size: 1,
  learning_rate: 0.0002,
  seed: 3407,
  lora_rank: 1,
  lora_alpha: 1,
};

const terminalStatuses = new Set(['completed', 'needs_improvement', 'failed', 'stopped']);
const activeStatuses = ['queued', 'running', 'recovering', 'pause_requested', 'paused', 'waiting'];

// preparationPath is where the dashboard shows a run.
export const preparationPath = (run: TrainingRun) => `/fine-tuning?${new URLSearchParams({ preparation: run.workflow_id, run: run.run_id })}`;

export const preparationSteps: [string, string][] = [['extract', 'Extract document'], ['preview', 'Generate preview'], ['generate', 'Generate records'], ['verify', 'Verify records']];
export const stepLabel = (name?: string) => preparationSteps.find(([key]) => key === name)?.[1] ?? 'Preparation';

// sentence turns a server message, which starts lowercase, into a sentence.
export const sentence = (text: string) => text ? `${text[0].toUpperCase()}${text.slice(1)}${/[.!?]$/.test(text) ? '' : '.'}` : text;

// requestMessage describes a failed request. Fetch failures (TypeError) and empty or
// cut-off responses (SyntaxError) mean the server gave no usable answer.
export const requestMessage = (err: unknown, fallback: string) => err instanceof TypeError || err instanceof SyntaxError ? 'The server could not be reached. Check that it is running.'
  : err instanceof Error && err.message ? sentence(err.message) : fallback;

export interface FineTuningActivity {
  kind: 'preparation' | 'training';
  title: string;
  stage: string;
  completed: number;
  total: number;
  paused?: boolean;
}

interface PreviewSubmission {
  request: ContinueTrainingPreview;
  status: 'sending' | 'uncertain' | 'rejected' | 'accepted';
  error: string;
}

interface TrainingSubmission {
  request: StartPreparedTraining;
  status: 'sending' | 'uncertain' | 'rejected' | 'accepted';
  error: string;
}

interface StopSubmission {
  status: 'sending' | 'requested' | 'failed';
  error: string;
}

// SourceDocument is a document chosen for preparation. It is uploading until
// it has an upload or an error.
export interface SourceDocument {
  key: string;
  file: File;
  upload: TrainingUpload | null;
  error: string;
}

interface FineTuningRunnerValue {
  preparedFile: File | null;
  upload: TrainingUpload | null;
  uploading: boolean;
  documents: SourceDocument[];
  documentsUploading: boolean;
  submitting: boolean;
  currentRun: TrainingRun | null;
  preparationRun: TrainingRun | null;
  trainingRun: TrainingRun | null;
  error: string;
  watchError: string;
  // adopting is true until the page knows whether a run is already in progress.
  adopting: boolean;
  adoptionError: string;
  preparationActivity: FineTuningActivity | null;
  trainingActivity: FineTuningActivity | null;
  chooseInput: (file: File | null) => Promise<void>;
  addDocuments: (files: File[]) => void;
  removeDocument: (key: string) => void;
  cancelPreparation: () => void;
  startRun: (training: TrainingOptions) => Promise<TrainingRun | null>;
  startPreparations: (training: TrainingOptions, prepare: (item: SourceDocument) => PreparationOptions) => Promise<TrainingRun[]>;
  selectRun: (run: TrainingRun) => void;
  refreshRun: (run?: TrainingRun) => Promise<void>;
  previewSubmission: (run: TrainingRun) => PreviewSubmission | undefined;
  continuePreview: (run: TrainingRun, records: TrainingPreviewRecord[]) => Promise<boolean>;
  trainingSubmission: (run: TrainingRun) => TrainingSubmission | undefined;
  startPreparedTraining: (run: TrainingRun) => Promise<boolean>;
  stopSubmission: (run: TrainingRun) => StopSubmission | undefined;
  stopRun: (run: TrainingRun) => Promise<void>;
}

const FineTuningRunnerContext = createContext<FineTuningRunnerValue | null>(null);

// retryDelay waits before the next attempt after a failed request, ending early on abort.
export function retryDelay(signal: AbortSignal, ms = 5000) {
  return new Promise<void>((resolve) => {
    const timer = setTimeout(resolve, ms);
    signal.addEventListener('abort', () => { clearTimeout(timer); resolve(); }, { once: true });
  });
}

export function FineTuningRunnerProvider({ children }: { children: ReactNode }) {
  const [preparedFile, setPreparedFile] = useState<File | null>(null);
  const [upload, setUpload] = useState<TrainingUpload | null>(null);
  const [uploading, setUploading] = useState(false);
  const [documents, setDocuments] = useState<SourceDocument[]>([]);
  const documentsUploading = documents.some((item) => !item.upload && !item.error);
  const [submitting, setSubmitting] = useState(false);
  const [preparationRun, setPreparationRun] = useState<TrainingRun | null>(null);
  const [trainingRun, setTrainingRun] = useState<TrainingRun | null>(null);
  const [selectedRun, setSelectedRun] = useState<'preparation' | 'training'>('preparation');
  const currentRun = selectedRun === 'preparation' ? preparationRun : trainingRun;
  const [error, setError] = useState('');
  const [watchError, setWatchError] = useState('');
  const [watchAttempt, setWatchAttempt] = useState(0);
  const [adopting, setAdopting] = useState(true);
  const [adoptionError, setAdoptionError] = useState('');
  const uploadAbort = useRef<AbortController | null>(null);
  const documentAborts = useRef(new Map<string, AbortController>());
  const submissionPending = useRef(false);
  const runSubmissionIDs = useRef<Record<string, string>>({});
  const previewRequests = useRef<Record<string, PreviewSubmission>>({});
  const [previewSubmissions, setPreviewSubmissions] = useState<Record<string, PreviewSubmission>>({});
  const trainingRequests = useRef<Record<string, TrainingSubmission>>({});
  const [trainingSubmissions, setTrainingSubmissions] = useState<Record<string, TrainingSubmission>>({});
  const stopRequests = useRef<Record<string, StopSubmission>>({});
  const [stopSubmissions, setStopSubmissions] = useState<Record<string, StopSubmission>>({});
  const activePreparation = preparationRun && !terminalStatuses.has(preparationRun.status) ? preparationRun : null;
  const activeTraining = trainingRun && !terminalStatuses.has(trainingRun.status) ? trainingRun : null;
  const trainingJob = activeTraining ?? (activePreparation?.stage === 'training' ? activePreparation : null);
  const watchedRuns = JSON.stringify([activePreparation, activeTraining].filter((run) => run !== null).map((run) => [run.workflow_id, run.run_id]));

  useEffect(() => () => {
    uploadAbort.current?.abort();
    for (const controller of documentAborts.current.values()) controller.abort();
  }, []);

  // A page load starts with no runs in memory. Adopt the newest in-progress runs so the
  // sidebar and the one-preparation-at-a-time rule survive a reload or a new tab.
  // Starting stays blocked until this answers, so it retries until it does.
  useEffect(() => {
    const controller = new AbortController();
    void (async () => {
      while (!controller.signal.aborted) {
        try {
          const page = await api.trainingRuns('', controller.signal, '', { statuses: activeStatuses });
          if (controller.signal.aborted) return;
          const runs = page.runs ?? [];
          const preparation = runs.find((run) => run.stages.some((stage) => stage.name === 'preparation'));
          const training = runs.find((run) => !run.stages.some((stage) => stage.name === 'preparation'));
          if (preparation) setPreparationRun((current) => current ?? preparation);
          if (training) setTrainingRun((current) => current ?? training);
          setAdoptionError('');
          setAdopting(false);
          return;
        } catch (err) {
          if (controller.signal.aborted) return;
          setAdoptionError(requestMessage(err, 'Could not load runs in progress.'));
          await retryDelay(controller.signal);
        }
      }
    })();
    return () => controller.abort();
  }, []);

  useEffect(() => {
    const identities: [string, string][] = JSON.parse(watchedRuns);
    if (identities.length === 0) return;
    const controller = new AbortController();
    setWatchError('');
    void (async () => {
      let revision = '';
      while (!controller.signal.aborted) {
        try {
          const page = await api.trainingRuns('', controller.signal, revision);
          // List rows omit the preview and artifacts, so read each watched run in full.
          const updates = await Promise.all(identities.map(([workflowID, runID]) => api.trainingRun(workflowID, runID, controller.signal)));
          if (controller.signal.aborted) return;
          const update = (selected: TrainingRun | null) => updates.find((run) => selected?.workflow_id === run.workflow_id && selected.run_id === run.run_id) ?? selected;
          setPreparationRun(update);
          setTrainingRun(update);
          setWatchError('');
          if (updates.every((run) => terminalStatuses.has(run.status))) return;
          revision = page.revision;
        } catch (err) {
          if (controller.signal.aborted) return;
          // The server may be restarting; keep retrying until this run is no longer watched.
          setWatchError(`Live updates paused: ${requestMessage(err, 'The connection failed.')} Retrying automatically.`);
          revision = '';
          await retryDelay(controller.signal);
        }
      }
    })();
    return () => controller.abort();
  }, [watchedRuns, watchAttempt]);

  const selectRun = (run: TrainingRun) => {
    setWatchError('');
    if (run.stages.some((stage) => stage.name === 'preparation')) {
      setPreparationRun(run);
      setSelectedRun('preparation');
    } else {
      setTrainingRun(run);
      setSelectedRun('training');
    }
  };

  // cancelPreparation cancels uploads still in progress. Uploaded documents stay chosen.
  const cancelPreparation = () => {
    uploadAbort.current?.abort();
    uploadAbort.current = null;
    setUploading(false);
    setUpload(null);
    setPreparedFile(null);
    for (const controller of documentAborts.current.values()) controller.abort();
    documentAborts.current.clear();
    setDocuments((current) => current.filter((item) => item.upload || item.error));
    setError('');
  };

  // chooseInput replaces the training-ready JSONL dataset.
  const chooseInput = async (file: File | null) => {
    uploadAbort.current?.abort();
    uploadAbort.current = null;
    setUpload(null);
    setError('');
    setPreparedFile(file);
    if (!file) { setUploading(false); return; }
    if (file.name.split('.').pop()?.toLowerCase() !== 'jsonl') {
      setUploading(false);
      setError('Only training-ready JSONL datasets are supported.');
      return;
    }
    const controller = new AbortController();
    uploadAbort.current = controller;
    setUploading(true);
    try {
      const result = await api.uploadTrainingDataset(file, controller.signal);
      if (!controller.signal.aborted) setUpload(result);
    } catch (err) {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'Upload failed');
    } finally {
      if (uploadAbort.current === controller) {
        uploadAbort.current = null;
        setUploading(false);
      }
    }
  };

  // addDocuments uploads source documents for preparation, each on its own.
  const addDocuments = (files: File[]) => {
    const added = files.map((file) => ({ key: crypto.randomUUID(), file, upload: null, error: '' } as SourceDocument));
    setDocuments((current) => [...current, ...added]);
    const update = (key: string, change: Partial<SourceDocument>) => setDocuments((current) => current.map((item) => item.key === key ? { ...item, ...change } : item));
    for (const item of added) {
      if (!['pdf', 'docx', 'md', 'txt'].includes(item.file.name.split('.').pop()?.toLowerCase() ?? '')) {
        update(item.key, { error: 'Only PDF, DOCX, Markdown, and text documents are supported.' });
        continue;
      }
      const controller = new AbortController();
      documentAborts.current.set(item.key, controller);
      api.uploadTrainingDataset(item.file, controller.signal)
        .then((result) => { if (!controller.signal.aborted) update(item.key, { upload: result }); })
        .catch((err) => { if (!controller.signal.aborted) update(item.key, { error: err instanceof Error ? err.message : 'Upload failed' }); })
        .finally(() => { if (documentAborts.current.get(item.key) === controller) documentAborts.current.delete(item.key); });
    }
  };

  const removeDocument = (key: string) => {
    documentAborts.current.get(key)?.abort();
    documentAborts.current.delete(key);
    setDocuments((current) => current.filter((item) => item.key !== key));
  };

  const failure = (err: unknown) => err instanceof ApiError && err.status === 409
    ? `${sentence(err.message)} Refresh datasets if the destination changed, or check the dashboard if this run was already started.`
    : requestMessage(err, 'The run could not be started.');

  // start submits one run. A retry of the same upload reuses its submission ID, so
  // an uncertain earlier request is not started twice.
  const start = async (upload: TrainingUpload, request: (id: string) => Parameters<typeof api.startTrainingRun>[0]) => {
    const submissionID = runSubmissionIDs.current[upload.id] ?? `training-${crypto.randomUUID()}`;
    runSubmissionIDs.current[upload.id] = submissionID;
    const run = await api.startTrainingRun(request(submissionID));
    delete runSubmissionIDs.current[upload.id];
    return run;
  };

  // startRun starts training on the training-ready JSONL dataset.
  const startRun = async (training: TrainingOptions): Promise<TrainingRun | null> => {
    if (!upload || uploading || adopting || submissionPending.current || trainingJob) return null;
    submissionPending.current = true;
    setSubmitting(true);
    setError('');
    try {
      const run = await start(upload, (id) => ({ id, model: FINE_TUNING_MODEL_ID, training, dataset_id: upload.id }));
      // The run owns the upload now; the form starts empty for the next one.
      setUpload(null);
      setPreparedFile(null);
      selectRun(run);
      return run;
    } catch (err) {
      setError(failure(err));
      return null;
    } finally {
      submissionPending.current = false;
      setSubmitting(false);
    }
  };

  // startPreparations starts one preparation run per uploaded document. Runs do not
  // wait for each other: each save adds onto the dataset's newest version. Started
  // documents leave the list; a document that could not start keeps its error.
  const startPreparations = async (training: TrainingOptions, prepare: (item: SourceDocument) => PreparationOptions): Promise<TrainingRun[]> => {
    const ready = documents.filter((item) => item.upload);
    if (!ready.length || submissionPending.current) return [];
    submissionPending.current = true;
    setSubmitting(true);
    setError('');
    try {
      const results = await Promise.all(ready.map(async (item) => {
        try {
          return { item, run: await start(item.upload!, (id) => ({ id, model: FINE_TUNING_MODEL_ID, training, preparation_only: true, prepare: { ...prepare(item), dataset_id: item.upload!.id } })) };
        } catch (err) {
          return { item, error: failure(err) };
        }
      }));
      const started = new Set(results.filter((result) => result.run).map((result) => result.item.key));
      setDocuments((current) => current.filter((item) => !started.has(item.key)).map((item) => {
        const result = results.find((entry) => entry.item.key === item.key);
        return result?.error ? { ...item, error: result.error } : item;
      }));
      const runs = results.flatMap((result) => result.run ? [result.run] : []);
      if (runs[0]) selectRun(runs[0]);
      return runs;
    } finally {
      submissionPending.current = false;
      setSubmitting(false);
    }
  };

  const refreshRun = async (selected = currentRun) => {
    if (!selected) return;
    setWatchError('');
    try {
      const run = await api.trainingRun(selected.workflow_id, selected.run_id);
      const update = (current: TrainingRun | null) => current?.workflow_id === run.workflow_id && current.run_id === run.run_id ? run : current;
      setPreparationRun(update);
      setTrainingRun(update);
      setWatchAttempt((attempt) => attempt + 1);
    } catch (err) {
      setWatchError(requestMessage(err, 'Could not refresh this run.'));
    }
  };

  const stopRun = async (run: TrainingRun): Promise<void> => {
    const key = JSON.stringify([run.workflow_id, run.run_id]);
    const previous = stopRequests.current[key];
    if (terminalStatuses.has(run.status) || previous?.status === 'sending' || previous?.status === 'requested') return;
    const update = (submission: StopSubmission) => {
      // Guard duplicate clicks synchronously and retain state across Back/navigation.
      stopRequests.current = { ...stopRequests.current, [key]: submission };
      setStopSubmissions(stopRequests.current);
    };
    update({ status: 'sending', error: '' });
    try {
      await api.stopTrainingRun(run.workflow_id, run.run_id);
      update({ status: 'requested', error: '' });
      // Cancellation acceptance is not a terminal job state. Polling remains authoritative.
      setWatchAttempt((attempt) => attempt + 1);
    } catch (err) {
      update({ status: 'failed', error: err instanceof Error ? err.message : 'Could not confirm the cancellation request.' });
    }
  };

  function setPreviewSubmission(key: string, submission: PreviewSubmission) {
    // Update the ref synchronously so rapid clicks cannot race React's render.
    previewRequests.current = { ...previewRequests.current, [key]: submission };
    setPreviewSubmissions(previewRequests.current);
  }

  const continuePreview = async (run: TrainingRun, records: TrainingPreviewRecord[]): Promise<boolean> => {
    const key = JSON.stringify([run.workflow_id, run.run_id]);
    const previous = previewRequests.current[key];
    if (previous?.status === 'sending' || previous?.status === 'accepted') return false;
    const retry = previous?.status === 'uncertain';
    if (!retry && (run.state_stale || run.status !== 'waiting' || !run.checkpoint?.id
      || run.checkpoint.name !== 'preview-review' || run.checkpoint.stage !== 'preparation'
      || run.checkpoint.step !== 'preview' || records.length === 0)) return false;

    const request = retry ? previous.request : {
      checkpoint_id: run.checkpoint!.id,
      submission_id: crypto.randomUUID(),
      records: records.map((record) => ({ ...record })),
    };
    setPreviewSubmission(key, { request, status: 'sending', error: '' });
    try {
      const receipt = await api.continueTrainingPreview(run.workflow_id, run.run_id, request);
      if (receipt.checkpoint_id !== request.checkpoint_id || receipt.submission_id !== request.submission_id) {
        throw new Error('The response did not confirm this submission.');
      }
      setPreviewSubmission(key, { request, status: 'accepted', error: '' });
      return true;
    } catch (err) {
      // A rejected retry does not prove that an earlier uncertain attempt failed.
      const rejected = !retry && err instanceof ApiError && err.status >= 400 && err.status < 500 && err.status !== 408;
      setPreviewSubmission(key, {
        request, status: rejected ? 'rejected' : 'uncertain',
        error: err instanceof Error ? err.message : 'Could not confirm acceptance.',
      });
      return false;
    }
  };

  function setTrainingSubmission(key: string, submission: TrainingSubmission) {
    // Keep the click guard synchronous, as with Preview submissions.
    trainingRequests.current = { ...trainingRequests.current, [key]: submission };
    setTrainingSubmissions(trainingRequests.current);
  }

  const startPreparedTraining = async (run: TrainingRun): Promise<boolean> => {
    const key = JSON.stringify([run.workflow_id, run.run_id]);
    const previous = trainingRequests.current[key];
    if (previous?.status === 'sending' || previous?.status === 'accepted') return false;
    const retry = previous?.status === 'uncertain';
    if (!retry && (run.state_stale || run.status !== 'waiting' || run.stage !== 'preparation'
      || !run.checkpoint?.id || run.checkpoint.name !== 'start-training'
      || run.checkpoint.stage !== 'preparation' || !!run.checkpoint.step
      || !run.stages.some((stage) => stage.name === 'preparation' && stage.status === 'completed'))) return false;

    const request = retry ? previous.request : {
      checkpoint_id: run.checkpoint!.id,
      submission_id: crypto.randomUUID(),
    };
    setTrainingSubmission(key, { request, status: 'sending', error: '' });
    try {
      const receipt = await api.startPreparedTraining(run.workflow_id, run.run_id, request);
      if (receipt.checkpoint_id !== request.checkpoint_id || receipt.submission_id !== request.submission_id) {
        throw new Error('The response did not confirm this submission.');
      }
      setTrainingSubmission(key, { request, status: 'accepted', error: '' });
      return true;
    } catch (err) {
      // A rejected retry cannot disprove acceptance of an earlier uncertain request.
      const rejected = !retry && err instanceof ApiError && err.status >= 400 && err.status < 500 && err.status !== 408;
      setTrainingSubmission(key, {
        request, status: rejected ? 'rejected' : 'uncertain',
        error: err instanceof Error ? err.message : 'Could not confirm Training authorization.',
      });
      return false;
    }
  };

  return (
    <FineTuningRunnerContext.Provider value={{
      preparedFile, upload, uploading, documents, documentsUploading, submitting, currentRun, preparationRun, trainingRun, error, watchError, adopting, adoptionError,
      preparationActivity: documentsUploading ? { kind: 'preparation', title: 'Uploading documents…', stage: documents.filter((item) => !item.upload && !item.error).map((item) => item.file.name).join(', '), completed: 0, total: 0 }
        : uploading ? { kind: 'preparation', title: 'Uploading dataset…', stage: preparedFile?.name ?? '', completed: 0, total: 0 }
        : activePreparation?.stage === 'preparation' ? {
          kind: 'preparation', paused: activePreparation.control_action === 'pause',
          title: activePreparation.control_action === 'pause' ? activePreparation.status === 'pause_requested' ? 'Pausing preparation…' : 'Preparation paused'
            : activePreparation.status === 'waiting' ? activePreparation.checkpoint?.name === 'preview-review' ? 'Preview ready for review' : 'Preparation waiting…'
            : activePreparation.status === 'queued' ? 'Preparation queued…' : 'Preparing dataset…',
          stage: activePreparation.status === 'queued' ? 'Waiting for a worker' : stepLabel(activePreparation.step),
          completed: activePreparation.progress?.completed ?? 0, total: activePreparation.progress?.total ?? 0,
        } : null,
      trainingActivity: trainingJob ? {
        kind: 'training', title: 'Training model…',
        stage: [trainingJob.stage, trainingJob.step].filter(Boolean).join(' / ') || trainingJob.status,
        completed: trainingJob.progress?.completed ?? 0, total: trainingJob.progress?.total ?? 0,
      } : null,
      chooseInput, addDocuments, removeDocument, cancelPreparation, startRun, startPreparations,
      selectRun,
      refreshRun,
      previewSubmission: (run) => previewSubmissions[JSON.stringify([run.workflow_id, run.run_id])],
      continuePreview,
      trainingSubmission: (run) => trainingSubmissions[JSON.stringify([run.workflow_id, run.run_id])],
      startPreparedTraining,
      stopSubmission: (run) => stopSubmissions[JSON.stringify([run.workflow_id, run.run_id])],
      stopRun,
    }}>
      {children}
    </FineTuningRunnerContext.Provider>
  );
}

export function useFineTuningRunner() {
  const context = useContext(FineTuningRunnerContext);
  if (!context) throw new Error('useFineTuningRunner must be used within a FineTuningRunnerProvider');
  return context;
}
