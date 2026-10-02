import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { api } from '../services/api';
import type {
  FineTuningDocumentPreview,
  FineTuningGoal,
  FineTuningPreparationProgress,
  FineTuningRecord,
  FineTuningRun,
  FineTuningTrainingConfig,
} from '../types';

export type FineTuningPreparationOperation = 'prepared-upload' | 'preview' | 'build';

export const MAX_FINE_TUNING_UPLOAD_BYTES = 100 * 1024 * 1024;
export const MAX_FINE_TUNING_DOCUMENTS = 10;
export const MAX_FINE_TUNING_COLLECTION_BYTES = 500 * 1024 * 1024;
export const MAX_FINE_TUNING_PREVIEW_RECORDS = 5;
export const MAX_FINE_TUNING_DATASET_INSTRUCTIONS = 2000;
export const FINE_TUNING_MODEL_ID = 'ggml-org/gemma-3-1b-it-Q8_0';
export const DEFAULT_FINE_TUNING_DATASET_INSTRUCTIONS = 'Create useful, unambiguous training examples from the supplied source material. Write natural, standalone questions and concise answers. Focus on the most important instructional facts, and avoid questions about document navigation, section numbers, or incidental details.';

const terminalStatuses = new Set(['completed', 'needs_improvement', 'failed', 'stopped']);

export function isTerminalFineTuningStatus(status: string): boolean {
  return terminalStatuses.has(status);
}

export function preparedFineTuningDatasetError(file: File): string {
  if (file.name.split('.').pop()?.toLowerCase() !== 'jsonl') return 'Choose a training-ready JSONL dataset.';
  if (file.size > MAX_FINE_TUNING_UPLOAD_BYTES) return 'The selected file exceeds the 100 MiB limit.';
  return '';
}

export function fineTuningRecordReviewError(goal: FineTuningGoal, records: FineTuningRecord[], index: number): string {
  const record = records[index];
  const prompt = record.prompt.trim();
  const field = goal === 'question_answer' ? 'Question' : 'Prompt';
  if (prompt.length < 3) return `${field} is required.`;
  if (!record.target.trim()) return `${goal === 'question_answer' ? 'Answer' : 'Target'} is required.`;
  if (!record.evidence.trim()) return 'Source evidence is unavailable.';
  if (records.some((candidate, candidateIndex) => candidateIndex !== index && candidate.prompt.trim().toLocaleLowerCase() === prompt.toLocaleLowerCase())) {
    return `${field} duplicates another reviewed record.`;
  }
  return '';
}

export interface FineTuningActivity {
  kind: 'preparation' | 'training';
  title: string;
  stage: string;
  completed: number;
  total: number;
}

interface FineTuningRunnerValue {
  sourceFiles: File[];
  preparedFile: File | null;
  datasetID: string;
  sourceName: string;
  goal: FineTuningGoal;
  preparationModelID: string;
  previewCount: number;
  customDatasetInstructions: boolean;
  datasetInstructions: string;
  documentPreviews: FineTuningDocumentPreview[];
  datasetBuilt: boolean;
  currentRun: FineTuningRun | null;
  activeRun: FineTuningRun | null;
  history: FineTuningRun[];
  busy: boolean;
  preparationOperation: FineTuningPreparationOperation | null;
  preparationProgress: FineTuningPreparationProgress | null;
  preparationStartedAt: string;
  error: string;
  preparationActivity: FineTuningActivity | null;
  trainingActivity: FineTuningActivity | null;
  setSourceFiles: (files: File[]) => void;
  setGoal: (goal: FineTuningGoal) => void;
  setPreparationModelID: (id: string) => void;
  setPreviewCount: (count: number) => void;
  setCustomDatasetInstructions: (enabled: boolean) => void;
  setDatasetInstructions: (instructions: string) => void;
  setError: (error: string) => void;
  choosePreparedDataset: (file: File | null) => void;
  preparePreview: () => void;
  regenerateDocumentPreview: (documentID: string) => void;
  updateRecord: (documentID: string, index: number, field: 'prompt' | 'target', value: string) => void;
  removeDocumentPreview: (documentID: string) => void;
  buildDataset: () => Promise<boolean>;
  returnToSourceSetup: () => void;
  resetPreparation: () => void;
  cancelPreparation: () => void;
  startRun: (training: FineTuningTrainingConfig) => Promise<boolean>;
  stopRun: () => void;
  resumeRun: (id: string) => Promise<boolean>;
  selectRun: (run: FineTuningRun) => void;
  refreshRuns: (showError?: boolean) => Promise<void>;
}

const FineTuningRunnerContext = createContext<FineTuningRunnerValue | null>(null);

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === 'AbortError';
}

export function FineTuningRunnerProvider({ children }: { children: ReactNode }) {
  const [sourceFiles, setSourceFilesState] = useState<File[]>([]);
  const [preparedFile, setPreparedFile] = useState<File | null>(null);
  const [datasetID, setDatasetID] = useState('');
  const [sourceName, setSourceName] = useState('');
  const [goal, setGoal] = useState<FineTuningGoal>('question_answer');
  const [preparationModelID, setPreparationModelID] = useState('');
  const [previewCount, setPreviewCount] = useState(MAX_FINE_TUNING_PREVIEW_RECORDS);
  const [customDatasetInstructions, setCustomDatasetInstructions] = useState(false);
  const [datasetInstructions, setDatasetInstructions] = useState(DEFAULT_FINE_TUNING_DATASET_INSTRUCTIONS);
  const [documentPreviews, setDocumentPreviews] = useState<FineTuningDocumentPreview[]>([]);
  const [datasetBuilt, setDatasetBuilt] = useState(false);
  const [currentRun, setCurrentRun] = useState<FineTuningRun | null>(null);
  const [trackedRun, setTrackedRun] = useState<FineTuningRun | null>(null);
  const [history, setHistory] = useState<FineTuningRun[]>([]);
  const [busy, setBusy] = useState(false);
  const [preparationOperation, setPreparationOperation] = useState<FineTuningPreparationOperation | null>(null);
  const [preparationProgress, setPreparationProgress] = useState<FineTuningPreparationProgress | null>(null);
  const [preparationStartedAt, setPreparationStartedAt] = useState('');
  const [error, setError] = useState('');
  const preparationAbortRef = useRef<AbortController | null>(null);
  const activeTrainingRunRef = useRef('');

  const resetPreparation = useCallback(() => {
    setSourceFilesState([]);
    setPreparedFile(null);
    setDatasetID('');
    setSourceName('');
    setDocumentPreviews([]);
    setDatasetBuilt(false);
    setCustomDatasetInstructions(false);
    setDatasetInstructions(DEFAULT_FINE_TUNING_DATASET_INSTRUCTIONS);
  }, []);

  const refreshRuns = useCallback(async (showError = false) => {
    try {
      const [latest, saved] = await Promise.all([
        api.currentFineTuningRun().catch(() => null),
        api.fineTuningHistory(),
      ]);
      setTrackedRun(latest);
      setCurrentRun((selected) => {
        if (!latest) return selected && isTerminalFineTuningStatus(selected.status) ? selected : null;
        if (!selected || selected.id === latest.id || !isTerminalFineTuningStatus(selected.status)) return latest;
        return selected;
      });
      setHistory(saved.runs ?? []);
    } catch (err) {
      if (showError) setError(err instanceof Error ? err.message : 'Could not load fine-tuning runs');
    }
  }, []);

  useEffect(() => {
    void refreshRuns(false);
  }, [refreshRuns]);

  useEffect(() => () => preparationAbortRef.current?.abort(), []);

  useEffect(() => {
    if (!trackedRun || isTerminalFineTuningStatus(trackedRun.status)) return;
    activeTrainingRunRef.current = trackedRun.id;
    const timer = window.setInterval(() => void refreshRuns(false), 2000);
    return () => window.clearInterval(timer);
  }, [trackedRun, refreshRuns]);

  useEffect(() => {
    if (
      !trackedRun
      || trackedRun.id !== activeTrainingRunRef.current
      || trackedRun.status !== 'completed'
      || !trackedRun.has_adapter
    ) return;

    activeTrainingRunRef.current = '';
    resetPreparation();
  }, [trackedRun, resetPreparation]);

  useEffect(() => {
    if (!preparationOperation) return;
    let cancelled = false;
    const refresh = async () => {
      try {
        const progress = await api.currentFineTuningPreparation();
        const matches = preparationOperation === 'preview'
          ? progress.operation === 'extraction' || progress.operation === 'preview'
          : progress.operation === preparationOperation;
        if (!cancelled && matches) setPreparationProgress(progress);
      } catch {
        // The operation may still be parsing its request before progress exists.
      }
    };
    void refresh();
    const timer = window.setInterval(() => void refresh(), 1000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [preparationOperation]);

  const runAction = async (action: () => Promise<void>): Promise<boolean> => {
    setBusy(true);
    setError('');
    try {
      await action();
      return true;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'The request failed');
      return false;
    } finally {
      setBusy(false);
    }
  };

  const runPreparationAction = async (
    operation: FineTuningPreparationOperation,
    action: (signal: AbortSignal) => Promise<void>,
  ): Promise<boolean> => {
    const controller = new AbortController();
    preparationAbortRef.current = controller;
    setPreparationOperation(operation);
    setPreparationProgress(null);
    setPreparationStartedAt(new Date().toISOString());
    setBusy(true);
    setError('');
    try {
      await action(controller.signal);
      return true;
    } catch (err) {
      if (!isAbortError(err)) setError(err instanceof Error ? err.message : 'The request failed');
      return false;
    } finally {
      if (preparationAbortRef.current === controller) {
        preparationAbortRef.current = null;
        setPreparationOperation(null);
        setBusy(false);
      }
    }
  };

  const setSourceFiles = (files: File[]) => {
    setSourceFilesState(files);
    setDatasetID('');
    setSourceName('');
    setDatasetBuilt(false);
  };

  const choosePreparedDataset = (file: File | null) => {
    setPreparedFile(file);
    if (!file || preparedFineTuningDatasetError(file)) return;
    void runPreparationAction('prepared-upload', async (signal) => {
      const dataset = await api.uploadFineTuningDataset(file, signal);
      setDatasetID(dataset.dataset_id);
      setSourceName(dataset.source_name);
      setDocumentPreviews([]);
      setDatasetBuilt(true);
    });
  };

  const preparePreview = () => {
    void runPreparationAction('preview', async (signal) => {
      if (sourceFiles.length === 0 || !preparationModelID) return;
      const dataset = datasetID
        ? { dataset_id: datasetID, source_name: sourceName }
        : await api.uploadFineTuningDataset(sourceFiles, signal);
      setDatasetID(dataset.dataset_id);
      const names = dataset.source_names?.length ? dataset.source_names : [dataset.source_name];
      setSourceName(names.length === 1 ? names[0] : `${names.length} documents`);
      const result = await api.previewFineTuningDataset({
        dataset_id: dataset.dataset_id,
        goal,
        model_id: preparationModelID,
        record_count: previewCount,
        dataset_instructions: customDatasetInstructions ? datasetInstructions.trim() : '',
      }, signal);
      const previews = result.documents?.length ? result.documents : [{
        id: dataset.documents?.[0]?.id ?? '001',
        name: dataset.documents?.[0]?.name ?? dataset.source_name,
        records: result.records,
        verification: result.verification,
        generation_attempts: result.generation_attempts,
        verification_attempts: result.verification_attempts,
        prompt_tokens: result.prompt_tokens,
        completion_tokens: result.completion_tokens,
      }];
      setDocumentPreviews(previews.map((preview) => ({
        ...preview,
        records: preview.records.map((record) => ({ ...record })),
        verified_records: preview.records.map((record) => ({ ...record })),
      })));
      setDatasetBuilt(false);
    });
  };

  const regenerateDocumentPreview = (documentID: string) => {
    void runPreparationAction('preview', async (signal) => {
      const result = await api.previewFineTuningDataset({
        dataset_id: datasetID,
        goal,
        model_id: preparationModelID,
        record_count: previewCount,
        dataset_instructions: customDatasetInstructions ? datasetInstructions.trim() : '',
        document_ids: [documentID],
      }, signal);
      const replacement = result.documents?.[0];
      if (!replacement) throw new Error('The server did not return the regenerated document preview');
      setDocumentPreviews((current) => current.map((preview) => preview.id === documentID ? {
        ...replacement,
        records: replacement.records.map((record) => ({ ...record })),
        verified_records: replacement.records.map((record) => ({ ...record })),
      } : preview));
    });
  };

  const updateRecord = (documentID: string, index: number, field: 'prompt' | 'target', value: string) => {
    setError('');
    setDocumentPreviews((current) => current.map((preview) => preview.id === documentID ? {
      ...preview,
      records: preview.records.map((record, recordIndex) =>
        recordIndex === index ? { ...record, [field]: value } : record),
    } : preview));
  };

  const removeDocumentPreview = (documentID: string) => {
    setDocumentPreviews((current) => current.filter((preview) => preview.id !== documentID));
  };

  const returnToSourceSetup = () => {
    setDocumentPreviews([]);
    setDatasetBuilt(false);
    setError('');
  };

  const buildDataset = async (): Promise<boolean> => {
    for (const preview of documentPreviews) {
      const invalidIndex = preview.records.findIndex((_, index) => fineTuningRecordReviewError(goal, preview.records, index) !== '');
      if (invalidIndex >= 0) {
        setError(`${preview.name} · Record ${invalidIndex + 1}: ${fineTuningRecordReviewError(goal, preview.records, invalidIndex)}`);
        return false;
      }
    }
    return runPreparationAction('build', async (signal) => {
      await api.buildFineTuningDataset({
        dataset_id: datasetID,
        goal,
        model_id: preparationModelID,
        dataset_instructions: customDatasetInstructions ? datasetInstructions.trim() : '',
        documents: documentPreviews.map((preview) => ({
          id: preview.id,
          reviewed_records: preview.records,
          verified_preview_records: preview.verified_records,
        })),
      }, signal);
      const includedNames = documentPreviews.map((preview) => preview.name);
      setSourceName(includedNames.length === 1 ? includedNames[0] : `${includedNames.length} documents`);
      setDatasetBuilt(true);
    });
  };

  const cancelPreparation = () => {
    const operation = preparationOperation;
    void api.stopFineTuningPreparation().catch(() => undefined);
    preparationAbortRef.current?.abort();
    preparationAbortRef.current = null;
    setPreparationOperation(null);
    setPreparationProgress(null);
    setBusy(false);
    setError('');
    if (operation === 'prepared-upload') {
      setPreparedFile(null);
      return;
    }
    setDatasetBuilt(false);
  };

  const startRun = (training: FineTuningTrainingConfig) => runAction(async () => {
    const run = await api.startFineTuningRun(datasetID, FINE_TUNING_MODEL_ID, training);
    activeTrainingRunRef.current = run.id;
    setTrackedRun(run);
    setCurrentRun(run);
    await refreshRuns(false);
  });

  const stopRun = () => {
    void runAction(async () => {
      await api.stopFineTuningRun();
      await refreshRuns(true);
    });
  };

  const resumeRun = (id: string) => runAction(async () => {
    const run = await api.resumeFineTuningRun(id);
    activeTrainingRunRef.current = run.id;
    setTrackedRun(run);
    setCurrentRun(run);
    await refreshRuns(false);
  });

  const activeRun = trackedRun && !isTerminalFineTuningStatus(trackedRun.status) ? trackedRun : null;
  const preparationActivity = useMemo<FineTuningActivity | null>(() => {
    if (!preparationOperation) return null;
    return {
      kind: 'preparation',
      title: preparationOperation === 'build' ? 'Building dataset…' : preparationOperation === 'prepared-upload' ? 'Uploading dataset…' : 'Preparing dataset…',
      stage: preparationProgress?.operation === 'extraction' ? 'Documents' : preparationProgress?.stage ?? 'Starting…',
      completed: preparationProgress?.completed ?? 0,
      total: preparationProgress?.total ?? 0,
    };
  }, [preparationOperation, preparationProgress]);
  const trainingActivity = useMemo<FineTuningActivity | null>(() => {
    if (!activeRun) return null;
    return {
      kind: 'training',
      title: 'Training model…',
      stage: activeRun.progress.message || activeRun.progress.stage || 'Starting…',
      completed: activeRun.progress.completed,
      total: activeRun.progress.total,
    };
  }, [activeRun]);

  return (
    <FineTuningRunnerContext.Provider value={{
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
      preparationActivity,
      trainingActivity,
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
      selectRun: setCurrentRun,
      refreshRuns,
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
