import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { useMatch } from 'react-router-dom';
import { api, ApiError } from '../services/api';
import type { PreparationOptions, TrainingOptions, TrainingRun, TrainingUpload } from '../types';
import { useAuth } from './AuthContext';

export const FINE_TUNING_MODEL_ID = 'ggml-org/gemma-3-1b-it-Q8_0';

// Initial form values, not model policy.
// DEFAULT_TRAINING_OPTIONS are the server's training defaults.
export const DEFAULT_TRAINING_OPTIONS: TrainingOptions = {
  maximum_epochs: 10,
  batch_size: 2,
  learning_rate: 0.0002,
  seed: 3407,
  lora_rank: 8,
  lora_alpha: 8,
};

const terminalStatuses = new Set(['completed', 'needs_improvement', 'failed', 'stopped']);
const activeStatuses = ['queued', 'running', 'recovering', 'pause_requested', 'paused', 'waiting'];

// Keep cancellation distinct from a resumable stop. A pause intent alone does not
// mean the worker has saved its progress and acknowledged the stop.
export function runStopState(run: TrainingRun): 'stopping' | 'stopped' | null {
  if (terminalStatuses.has(run.status) || run.control_action !== 'pause') return null;
  return run.control_revision > 0 && (run.control_acknowledged_revision ?? 0) >= run.control_revision ? 'stopped' : 'stopping';
}

// preparationPath is where the dashboard shows a run.
export const preparationPath = (run: TrainingRun) => `/fine-tuning?${new URLSearchParams({ preparation: run.workflow_id, run: run.run_id })}`;
export const trainingPath = (run: TrainingRun) => `/fine-tuning?${new URLSearchParams({ training: run.workflow_id, run: run.run_id })}`;

export const preparationSteps: [string, string][] = [['extract', 'Extract document'], ['preview', 'Generate preview'], ['generate', 'Generate records'], ['verify', 'Verify records']];
export const stepLabel = (name?: string) => preparationSteps.find(([key]) => key === name)?.[1] ?? 'Preparation';
export const trainingStepLabels: Record<string, string> = { split: 'Split dataset', train: 'Train adapter', evaluate: 'Evaluate adapter', publish: 'Publish adapter' };

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
  // path opens the run's page.
  path?: string;
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
  documents: SourceDocument[];
  documentsUploading: boolean;
  submitting: boolean;
  preparationRun: TrainingRun | null;
  error: string;
  // adopting is true until the page knows whether a run is already in progress.
  adopting: boolean;
  adoptionError: string;
  preparationActivity: FineTuningActivity | null;
  trainingActivity: FineTuningActivity | null;
  addDocuments: (files: File[]) => void;
  removeDocument: (key: string) => void;
  cancelPreparation: () => void;
  startDatasetTraining: (training: TrainingOptions, dataset: { id: string; version: number }) => Promise<TrainingRun | null>;
  startPreparations: (training: TrainingOptions, prepare: (item: SourceDocument) => PreparationOptions) => Promise<TrainingRun[]>;
  selectRun: (run: TrainingRun) => void;
  refreshRuns: () => void;
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
  const { moxieEnabled } = useAuth();
  const matchesFineTuningPage = useMatch('/fine-tuning') !== null;
  const onFineTuningPage = moxieEnabled && matchesFineTuningPage;
  const [documents, setDocuments] = useState<SourceDocument[]>([]);
  const documentsUploading = documents.some((item) => !item.upload && !item.error);
  const [submitting, setSubmitting] = useState(false);
  const [preparationRun, setPreparationRun] = useState<TrainingRun | null>(null);
  const [trainingRun, setTrainingRun] = useState<TrainingRun | null>(null);
  const [error, setError] = useState('');
  const [adopting, setAdopting] = useState(true);
  const [adoptionError, setAdoptionError] = useState('');
  const [refreshAttempt, setRefreshAttempt] = useState(0);
  const documentAborts = useRef(new Map<string, AbortController>());
  const submissionPending = useRef(false);
  const runSubmissionIDs = useRef<Record<string, string>>({});
  const activePreparation = preparationRun && !terminalStatuses.has(preparationRun.status) ? preparationRun : null;
  const activeTraining = trainingRun && !terminalStatuses.has(trainingRun.status) ? trainingRun : null;
  const trainingJob = activeTraining ?? (activePreparation?.stage === 'training' ? activePreparation : null);

  useEffect(() => () => {
    for (const controller of documentAborts.current.values()) controller.abort();
  }, []);

  // Discover in-progress runs when entering Fine-Tuning. Keep form/run state across
  // navigation, but do not make background requests while another page is open.
  // Starting stays blocked until this answers; a failed request needs manual refresh.
  useEffect(() => {
    if (!onFineTuningPage) return;
    const controller = new AbortController();
    setAdopting(true);
    setAdoptionError('');
    void (async () => {
      try {
        const page = await api.trainingRuns('', controller.signal, '', { statuses: activeStatuses });
        if (controller.signal.aborted) return;
        const runs = page.runs ?? [];
        const preparation = runs.find((run) => run.stages.some((stage) => stage.name === 'preparation'));
        const training = runs.find((run) => !run.stages.some((stage) => stage.name === 'preparation'));
        setPreparationRun(preparation ?? null);
        setTrainingRun(training ?? null);
        setAdoptionError('');
        setAdopting(false);
      } catch (err) {
        if (controller.signal.aborted) return;
        setAdoptionError(requestMessage(err, 'Could not load runs in progress.'));
      }
    })();
    return () => controller.abort();
  }, [onFineTuningPage, refreshAttempt]);

  // Detail screens own refreshing; share their results without a second watcher.
  const selectRun = useCallback((run: TrainingRun) => {
    if (run.stages.some((stage) => stage.name === 'preparation')) {
      setPreparationRun(run);
    } else {
      setTrainingRun(run);
    }
  }, []);

  // cancelPreparation cancels uploads still in progress. Uploaded documents stay chosen.
  const cancelPreparation = () => {
    for (const controller of documentAborts.current.values()) controller.abort();
    documentAborts.current.clear();
    setDocuments((current) => current.filter((item) => item.upload || item.error));
    setError('');
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
  const start = async (key: string, request: (id: string) => Parameters<typeof api.startTrainingRun>[0]) => {
    const submissionID = runSubmissionIDs.current[key] ?? `training-${crypto.randomUUID()}`;
    runSubmissionIDs.current[key] = submissionID;
    const run = await api.startTrainingRun(request(submissionID));
    delete runSubmissionIDs.current[key];
    return run;
  };

  // startDatasetTraining starts training on a saved dataset version. A retry for the
  // same version reuses its submission ID, so an uncertain request is not started twice.
  const startDatasetTraining = async (training: TrainingOptions, dataset: { id: string; version: number }): Promise<TrainingRun | null> => {
    if (submissionPending.current || trainingJob) return null;
    submissionPending.current = true;
    setSubmitting(true);
    setError('');
    try {
      const run = await start(`${dataset.id}@${dataset.version}`, (id) => ({ id, model: FINE_TUNING_MODEL_ID, training, saved_dataset: dataset }));
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
          return { item, run: await start(item.upload!.id, (id) => ({ id, model: FINE_TUNING_MODEL_ID, training, preparation_only: true, prepare: { ...prepare(item), dataset_id: item.upload!.id } })) };
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

  return (
    <FineTuningRunnerContext.Provider value={{
      documents, documentsUploading, submitting, preparationRun, error, adopting, adoptionError,
      preparationActivity: documentsUploading ? { kind: 'preparation', title: 'Uploading documents…', stage: documents.filter((item) => !item.upload && !item.error).map((item) => item.file.name).join(', '), completed: 0, total: 0 }
        : activePreparation?.stage === 'preparation' ? {
          kind: 'preparation', paused: runStopState(activePreparation) === 'stopped',
          title: runStopState(activePreparation) ? runStopState(activePreparation) === 'stopping' ? 'Stopping preparation…' : 'Preparation stopped'
            : activePreparation.status === 'waiting' ? activePreparation.checkpoint?.name === 'preview-review' ? 'Preview ready for review' : 'Preparation waiting…'
            : activePreparation.status === 'queued' ? 'Preparation queued…' : 'Preparing dataset…',
          stage: activePreparation.status === 'queued' ? 'Waiting for a worker'
            : activePreparation.step === 'extract' && activePreparation.progress?.completed && !activePreparation.progress.total ? `${stepLabel(activePreparation.step)} · ${activePreparation.progress.completed} pages read`
            : stepLabel(activePreparation.step),
          completed: activePreparation.progress?.completed ?? 0, total: activePreparation.progress?.total ?? 0,
        } : null,
      trainingActivity: trainingJob ? {
        kind: 'training', paused: runStopState(trainingJob) === 'stopped', title: runStopState(trainingJob) === 'stopping' ? 'Stopping training…' : runStopState(trainingJob) === 'stopped' ? 'Training stopped' : trainingJob.status === 'queued' ? 'Training queued…' : 'Training…', path: trainingPath(trainingJob),
        stage: trainingJob.status === 'queued' ? 'Waiting for a worker' : trainingStepLabels[trainingJob.step ?? ''] ?? 'Training',
        completed: trainingJob.progress?.completed ?? 0, total: trainingJob.progress?.total ?? 0,
      } : null,
      addDocuments, removeDocument, cancelPreparation, startDatasetTraining, startPreparations,
      selectRun, refreshRuns: () => setRefreshAttempt((value) => value + 1),
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
