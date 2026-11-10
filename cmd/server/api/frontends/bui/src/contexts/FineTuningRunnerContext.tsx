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

export interface FineTuningActivity {
  kind: 'preparation' | 'training';
  title: string;
  stage: string;
  completed: number;
  total: number;
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

interface FineTuningRunnerValue {
  inputKind: 'dataset' | 'document';
  preparedFile: File | null;
  upload: TrainingUpload | null;
  uploading: boolean;
  submitting: boolean;
  currentRun: TrainingRun | null;
  preparationRun: TrainingRun | null;
  trainingRun: TrainingRun | null;
  error: string;
  watchError: string;
  preparationActivity: FineTuningActivity | null;
  trainingActivity: FineTuningActivity | null;
  chooseInput: (file: File | null, kind: 'dataset' | 'document') => Promise<void>;
  cancelPreparation: () => void;
  startRun: (training: TrainingOptions, preparation?: PreparationOptions) => Promise<boolean>;
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

export function FineTuningRunnerProvider({ children }: { children: ReactNode }) {
  const [inputKind, setInputKind] = useState<'dataset' | 'document'>('dataset');
  const [preparedFile, setPreparedFile] = useState<File | null>(null);
  const [upload, setUpload] = useState<TrainingUpload | null>(null);
  const [uploading, setUploading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [preparationRun, setPreparationRun] = useState<TrainingRun | null>(null);
  const [trainingRun, setTrainingRun] = useState<TrainingRun | null>(null);
  const [selectedRun, setSelectedRun] = useState<'preparation' | 'training'>('preparation');
  const currentRun = selectedRun === 'preparation' ? preparationRun : trainingRun;
  const [error, setError] = useState('');
  const [watchError, setWatchError] = useState('');
  const [watchAttempt, setWatchAttempt] = useState(0);
  const uploadAbort = useRef<AbortController | null>(null);
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

  useEffect(() => () => uploadAbort.current?.abort(), []);

  useEffect(() => {
    const identities: [string, string][] = JSON.parse(watchedRuns);
    if (identities.length === 0) return;
    const controller = new AbortController();
    setWatchError('');
    void (async () => {
      try {
        let revision = '';
        while (!controller.signal.aborted) {
          const page = await api.trainingRuns('', controller.signal, revision);
          const updates = await Promise.all(identities.map(async ([workflowID, runID]) => page.runs.find((item) => item.workflow_id === workflowID && item.run_id === runID)
            ?? await api.trainingRun(workflowID, runID, controller.signal)));
          if (controller.signal.aborted) return;
          const update = (selected: TrainingRun | null) => updates.find((run) => selected?.workflow_id === run.workflow_id && selected.run_id === run.run_id) ?? selected;
          setPreparationRun(update);
          setTrainingRun(update);
          if (updates.every((run) => terminalStatuses.has(run.status))) return;
          revision = page.revision;
        }
      } catch (err) {
        if (!controller.signal.aborted) setWatchError(`Live updates paused: ${err instanceof Error ? err.message : 'Connection failed'}. Select Refresh to retry.`);
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

  const cancelPreparation = () => {
    uploadAbort.current?.abort();
    uploadAbort.current = null;
    setUploading(false);
    setUpload(null);
    setPreparedFile(null);
    setError('');
  };

  const chooseInput = async (file: File | null, kind: 'dataset' | 'document') => {
    cancelPreparation();
    setInputKind(kind);
    setPreparedFile(file);
    if (!file) return;
    const extension = file.name.split('.').pop()?.toLowerCase() ?? '';
    if (!(kind === 'dataset' ? ['jsonl'] : ['pdf', 'docx', 'md', 'txt']).includes(extension)) {
      setError(kind === 'dataset' ? 'Choose a training-ready JSONL dataset.' : 'Choose a PDF, DOCX, Markdown, or text document.');
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

  const startRun = async (training: TrainingOptions, preparation?: PreparationOptions): Promise<boolean> => {
    if (!upload || uploading || submissionPending.current) return false;
    if (inputKind === 'document' ? !!activePreparation : !!trainingJob) return false;
    if (inputKind === 'document' && !preparation?.model) return false;
    submissionPending.current = true;
    setSubmitting(true);
    setError('');
    const submissionKey = `${inputKind}:${upload.id}`;
    const submissionID = runSubmissionIDs.current[submissionKey] ?? `training-${crypto.randomUUID()}`;
    runSubmissionIDs.current[submissionKey] = submissionID;
    try {
      const run = await api.startTrainingRun({
        id: submissionID, model: FINE_TUNING_MODEL_ID, training,
        ...(inputKind === 'document' && preparation
          ? { preparation_only: true, prepare: { ...preparation, dataset_id: upload.id } }
          : { dataset_id: upload.id }),
      });
      delete runSubmissionIDs.current[submissionKey];
      selectRun(run);
      return true;
    } catch (err) {
      setError(err instanceof ApiError && err.status === 409
        ? `${err.message} Refresh datasets if the destination changed, or check Training history if this job was already submitted.`
        : err instanceof Error ? err.message : 'Job submission failed');
      return false;
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
      setWatchError(err instanceof Error ? err.message : 'Could not refresh training job');
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
      inputKind, preparedFile, upload, uploading, submitting, currentRun, preparationRun, trainingRun, error, watchError,
      preparationActivity: uploading ? { kind: 'preparation', title: inputKind === 'document' ? 'Uploading document…' : 'Uploading dataset…', stage: preparedFile?.name ?? '', completed: 0, total: 0 }
        : activePreparation?.stage === 'preparation' ? {
          kind: 'preparation', title: activePreparation.status === 'waiting' ? 'Preparation waiting…' : 'Preparing dataset…',
          stage: activePreparation.step || activePreparation.status,
          completed: activePreparation.progress?.completed ?? 0, total: activePreparation.progress?.total ?? 0,
        } : null,
      trainingActivity: trainingJob ? {
        kind: 'training', title: 'Training model…',
        stage: [trainingJob.stage, trainingJob.step].filter(Boolean).join(' / ') || trainingJob.status,
        completed: trainingJob.progress?.completed ?? 0, total: trainingJob.progress?.total ?? 0,
      } : null,
      chooseInput, cancelPreparation, startRun,
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
