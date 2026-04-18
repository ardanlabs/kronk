import { createContext, useContext, useState, useCallback, type ReactNode } from 'react';
import { api } from '../services/api';
import type { ListModelInfoResponse } from '../types';

interface ModelListContextType {
  models: ListModelInfoResponse | null;
  loading: boolean;
  error: string | null;
  loadModels: () => Promise<void>;
  invalidate: () => void;
}

const ModelListContext = createContext<ModelListContextType | null>(null);

export function ModelListProvider({ children }: { children: ReactNode }) {
  const [models, setModels] = useState<ListModelInfoResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loaded, setLoaded] = useState(false);

  const loadModels = useCallback(async () => {
    if (loaded && models) return;
    
    setLoading(true);
    setError(null);
    try {
      const response = await api.listModels();
      setModels(response);
      setLoaded(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load models');
    } finally {
      setLoading(false);
    }
  }, [loaded, models]);

  const invalidate = useCallback(() => {
    setLoaded(false);
    setModels(null);
  }, []);

  return (
    <ModelListContext.Provider value={{ models, loading, error, loadModels, invalidate }}>
      {children}
    </ModelListContext.Provider>
  );
}

export function useModelList() {
  const context = useContext(ModelListContext);
  if (!context) {
    throw new Error('useModelList must be used within a ModelListProvider');
  }
  return context;
}
