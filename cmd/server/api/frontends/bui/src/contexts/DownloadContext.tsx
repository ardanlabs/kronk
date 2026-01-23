import { createContext, useContext, useState, useCallback, useRef, type ReactNode } from 'react';
import { api } from '../services/api';
import { useModelList } from './ModelListContext';
import type { PullResponse } from '../types';

export interface DownloadMessage {
  text: string;
  type: 'info' | 'error' | 'success';
}

interface DownloadState {
  modelUrl: string;
  messages: DownloadMessage[];
  status: 'downloading' | 'complete' | 'error';
}

interface DownloadContextType {
  download: DownloadState | null;
  isDownloading: boolean;
  startDownload: (modelUrl: string) => void;
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

  const startDownload = useCallback((modelUrl: string) => {
    if (abortRef.current) {
      return;
    }

    setDownload({
      modelUrl,
      messages: [],
      status: 'downloading',
    });

    abortRef.current = api.pullModel(
      modelUrl,
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
      value={{ download, isDownloading, startDownload, cancelDownload, clearDownload }}
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
