import { createContext, useContext, useState, useCallback, useRef, type ReactNode } from 'react';
import { api } from '../services/api';
import { useModelList } from './ModelListContext';
import type { PullResponse } from '../types';

export interface DownloadMessage {
  text: string;
  type: 'info' | 'error' | 'success';
}

type DownloadKind = 'model' | 'catalog';

export type DownloadOrigin = 'model-pull' | 'catalog';

export interface DownloadMeta {
  model_id?: string;
  model_urls: string[];
  proj_url?: string;
  fileIndex: number;
  fileTotal: number;
}

export interface DownloadProgress {
  src: string;
  currentBytes: number;
  totalBytes: number;
  mbPerSec: number;
  pct: number;
  startedAtMs: number;
}

interface DownloadState {
  kind: DownloadKind;
  origin: DownloadOrigin;
  modelUrl: string;
  modelUrls?: string[];
  currentIndex?: number;
  catalogId?: string;
  messages: DownloadMessage[];
  status: 'downloading' | 'complete' | 'error';
  meta?: DownloadMeta;
  progress?: DownloadProgress;
}

interface DownloadContextType {
  download: DownloadState | null;
  isDownloading: boolean;
  startDownload: (modelUrl: string, projUrl?: string) => void;
  startBatchDownload: (modelUrls: string[], projUrl?: string) => void;
  startCatalogDownload: (catalogId: string, downloadServer?: string) => void;
  cancelDownload: () => void;
  clearDownload: () => void;
}

const DownloadContext = createContext<DownloadContextType | null>(null);

export function DownloadProvider({ children }: { children: ReactNode }) {
  const { invalidate } = useModelList();
  const [download, setDownload] = useState<DownloadState | null>(null);
  const abortRef = useRef<(() => void) | null>(null);
  const progressStartRef = useRef<{ src: string; startMs: number } | null>(null);
  const lastProgressUpdateRef = useRef<number>(0);

  const ANSI_INLINE = '\r\x1b[K';

  const addMessage = useCallback((text: string, type: DownloadMessage['type']) => {
    setDownload((prev) => {
      if (!prev) return prev;
      return { ...prev, messages: [...prev.messages, { text, type }] };
    });
  }, []);

  const updateLastMessage = useCallback((text: string, type: DownloadMessage['type']) => {
    setDownload((prev) => {
      if (!prev) return prev;
      if (prev.messages.length === 0) {
        return { ...prev, messages: [{ text, type }] };
      }
      const updated = [...prev.messages];
      updated[updated.length - 1] = { text, type };
      return { ...prev, messages: updated };
    });
  }, []);

  const handleProgress = useCallback((data: PullResponse) => {
    if (!data.progress) return;

    const now = Date.now();
    if (now - lastProgressUpdateRef.current < 200) return;
    lastProgressUpdateRef.current = now;

    const p = data.progress;
    const src = p.src || '';

    if (!progressStartRef.current || progressStartRef.current.src !== src) {
      progressStartRef.current = { src, startMs: now };
    }

    const pct = p.total_bytes && p.total_bytes > 0
      ? Math.min(100, (p.current_bytes ?? 0) / p.total_bytes * 100)
      : 0;

    setDownload((prev) => {
      if (!prev) return prev;
      return {
        ...prev,
        progress: {
          src,
          currentBytes: p.current_bytes ?? 0,
          totalBytes: p.total_bytes ?? 0,
          mbPerSec: p.mb_per_sec ?? 0,
          pct,
          startedAtMs: progressStartRef.current!.startMs,
        },
      };
    });
  }, []);

  const pullOne = useCallback((modelUrl: string, projUrl: string | undefined, onComplete: () => void) => {
    abortRef.current = api.pullModel(
      modelUrl,
      projUrl,
      (data: PullResponse) => {
        if (data.meta) {
          setDownload((prev) => {
            if (!prev) return prev;
            const existing = prev.meta;
            if (existing && existing.model_id === data.meta!.model_id) {
              const urls = [...existing.model_urls];
              if (data.meta!.model_url && !urls.includes(data.meta!.model_url)) {
                urls.push(data.meta!.model_url);
              }
              return { ...prev, meta: { ...existing, model_urls: urls, fileIndex: data.meta!.file_index ?? existing.fileIndex, fileTotal: data.meta!.file_total ?? existing.fileTotal } };
            }
            return {
              ...prev,
              meta: {
                model_id: data.meta!.model_id,
                model_urls: data.meta!.model_url ? [data.meta!.model_url] : [],
                proj_url: data.meta!.proj_url || undefined,
                fileIndex: data.meta!.file_index ?? 1,
                fileTotal: data.meta!.file_total ?? 1,
              },
            };
          });
        }
        if (data.progress) {
          handleProgress(data);
          if (data.status) {
            updateLastMessage(data.status, 'info');
          }
          return;
        }
        if (data.status) {
          if (data.status.startsWith(ANSI_INLINE)) {
            const cleanText = data.status.slice(ANSI_INLINE.length);
            updateLastMessage(cleanText, 'info');
          } else {
            addMessage(data.status, 'info');
          }
        }
        if (data.model_file) {
          addMessage(`Model file: ${data.model_file}`, 'info');
        }
      },
      (error: string) => {
        addMessage(error, 'error');
        setDownload((prev) => (prev ? { ...prev, status: 'error' } : prev));
        abortRef.current = null;
      },
      onComplete
    );
  }, [addMessage, updateLastMessage, handleProgress]);

  const startDownload = useCallback((modelUrl: string, projUrl?: string) => {
    if (abortRef.current) {
      return;
    }

    setDownload({
      kind: 'model',
      origin: 'model-pull',
      modelUrl,
      messages: [],
      status: 'downloading',
    });

    progressStartRef.current = null;
    lastProgressUpdateRef.current = 0;

    pullOne(modelUrl, projUrl, () => {
      addMessage('Pull complete!', 'success');
      setDownload((prev) => (prev ? { ...prev, status: 'complete' } : prev));
      abortRef.current = null;
      invalidate();
    });
  }, [pullOne, addMessage, invalidate]);

  const startBatchDownload = useCallback((modelUrls: string[], projUrl?: string) => {
    if (abortRef.current || modelUrls.length === 0) {
      return;
    }

    const total = modelUrls.length;

    setDownload({
      kind: 'model',
      origin: 'model-pull',
      modelUrl: modelUrls[0],
      modelUrls,
      currentIndex: 0,
      messages: [{ text: `Starting pull 1 of ${total}: ${modelUrls[0]}`, type: 'info' }],
      status: 'downloading',
    });

    progressStartRef.current = null;
    lastProgressUpdateRef.current = 0;

    const pullNext = (index: number) => {
      const proj = index === 0 ? projUrl : undefined;
      pullOne(modelUrls[index], proj, () => {
        addMessage(`Pull complete for: ${modelUrls[index]}`, 'success');
        abortRef.current = null;

        const nextIndex = index + 1;
        if (nextIndex < total) {
          addMessage(`Starting pull ${nextIndex + 1} of ${total}: ${modelUrls[nextIndex]}`, 'info');
          setDownload((prev) => (prev ? { ...prev, modelUrl: modelUrls[nextIndex], currentIndex: nextIndex } : prev));
          pullNext(nextIndex);
        } else {
          addMessage('All pulls complete!', 'success');
          setDownload((prev) => (prev ? { ...prev, status: 'complete' } : prev));
          invalidate();
        }
      });
    };

    pullNext(0);
  }, [pullOne, addMessage, invalidate]);

  const startCatalogDownload = useCallback((catalogId: string, downloadServer?: string) => {
    if (abortRef.current) {
      return;
    }

    setDownload({
      kind: 'catalog',
      origin: 'catalog',
      modelUrl: catalogId,
      catalogId,
      messages: [],
      status: 'downloading',
    });

    progressStartRef.current = null;
    lastProgressUpdateRef.current = 0;

    abortRef.current = api.pullCatalogModel(
      catalogId,
      (data: PullResponse) => {
        if (data.meta) {
          setDownload((prev) => {
            if (!prev) return prev;
            const existing = prev.meta;
            if (existing && existing.model_id === data.meta!.model_id) {
              const urls = [...existing.model_urls];
              if (data.meta!.model_url && !urls.includes(data.meta!.model_url)) {
                urls.push(data.meta!.model_url);
              }
              return { ...prev, meta: { ...existing, model_urls: urls, fileIndex: data.meta!.file_index ?? existing.fileIndex, fileTotal: data.meta!.file_total ?? existing.fileTotal } };
            }
            return {
              ...prev,
              meta: {
                model_id: data.meta!.model_id,
                model_urls: data.meta!.model_url ? [data.meta!.model_url] : [],
                proj_url: data.meta!.proj_url || undefined,
                fileIndex: data.meta!.file_index ?? 1,
                fileTotal: data.meta!.file_total ?? 1,
              },
            };
          });
        }
        if (data.progress) {
          handleProgress(data);
          if (data.status) {
            updateLastMessage(data.status, 'info');
          }
          return;
        }
        if (data.status) {
          if (data.status.startsWith(ANSI_INLINE)) {
            const cleanText = data.status.slice(ANSI_INLINE.length);
            updateLastMessage(cleanText, 'info');
          } else {
            addMessage(data.status, 'info');
          }
        }
        if (data.model_file) {
          addMessage(`Model file: ${data.model_file}`, 'info');
        }
      },
      (error: string) => {
        addMessage(error, 'error');
        setDownload((prev) => (prev ? { ...prev, status: 'error' } : prev));
        abortRef.current = null;
      },
      () => {
        addMessage('Pull complete!', 'success');
        setDownload((prev) => (prev ? { ...prev, status: 'complete' } : prev));
        abortRef.current = null;
        invalidate();
      },
      downloadServer
    );
  }, [addMessage, updateLastMessage, handleProgress, invalidate]);

  const cancelDownload = useCallback(() => {
    if (abortRef.current) {
      abortRef.current();
      abortRef.current = null;
    }
    setDownload((prev) => {
      if (!prev) return prev;
      return {
        ...prev,
        messages: [...prev.messages, { text: 'Cancelled', type: 'error' }],
        status: 'error',
      };
    });
  }, []);

  const clearDownload = useCallback(() => {
    if (abortRef.current) {
      abortRef.current();
      abortRef.current = null;
    }
    setDownload(null);
  }, []);

  const isDownloading = download?.status === 'downloading';

  return (
    <DownloadContext.Provider
      value={{ download, isDownloading, startDownload, startBatchDownload, startCatalogDownload, cancelDownload, clearDownload }}
    >
      {children}
    </DownloadContext.Provider>
  );
}

export function useDownload() {
  const context = useContext(DownloadContext);
  if (!context) {
    throw new Error('useDownload must be used within a DownloadProvider');
  }
  return context;
}
