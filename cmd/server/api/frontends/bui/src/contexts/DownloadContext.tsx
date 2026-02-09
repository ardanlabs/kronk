import { createContext, useContext, useState, useCallback, useRef, type ReactNode } from 'react';
import { api } from '../services/api';
import { useModelList } from './ModelListContext';
import type { PullResponse } from '../types';

export interface DownloadMessage {
  text: string;
  type: 'info' | 'error' | 'success';
}

type DownloadKind = 'model' | 'catalog';

interface DownloadState {
  kind: DownloadKind;
  modelUrl: string;
  modelUrls?: string[];
  currentIndex?: number;
  catalogId?: string;
  messages: DownloadMessage[];
  status: 'downloading' | 'complete' | 'error';
}

interface DownloadContextType {
  download: DownloadState | null;
  isDownloading: boolean;
  startDownload: (modelUrl: string, projUrl?: string) => void;
  startBatchDownload: (modelUrls: string[], projUrl?: string) => void;
  startCatalogDownload: (catalogId: string) => void;
  cancelDownload: () => void;
  clearDownload: () => void;
}

const DownloadContext = createContext<DownloadContextType | null>(null);

export function DownloadProvider({ children }: { children: ReactNode }) {
  const { invalidate } = useModelList();
  const [download, setDownload] = useState<DownloadState | null>(null);
  const abortRef = useRef<(() => void) | null>(null);

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

  const pullOne = useCallback((modelUrl: string, projUrl: string | undefined, onComplete: () => void) => {
    abortRef.current = api.pullModel(
      modelUrl,
      projUrl,
      (data: PullResponse) => {
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
  }, [addMessage, updateLastMessage]);

  const startDownload = useCallback((modelUrl: string, projUrl?: string) => {
    if (abortRef.current) {
      return;
    }

    setDownload({
      kind: 'model',
      modelUrl,
      messages: [],
      status: 'downloading',
    });

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
      modelUrl: modelUrls[0],
      modelUrls,
      currentIndex: 0,
      messages: [{ text: `Starting pull 1 of ${total}: ${modelUrls[0]}`, type: 'info' }],
      status: 'downloading',
    });

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

  const startCatalogDownload = useCallback((catalogId: string) => {
    if (abortRef.current) {
      return;
    }

    setDownload({
      kind: 'catalog',
      modelUrl: catalogId,
      catalogId,
      messages: [],
      status: 'downloading',
    });

    abortRef.current = api.pullCatalogModel(
      catalogId,
      (data: PullResponse) => {
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
      }
    );
  }, [addMessage, updateLastMessage, invalidate]);

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
