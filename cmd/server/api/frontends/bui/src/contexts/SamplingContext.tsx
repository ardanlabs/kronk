import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from 'react';

const SAMPLING_STORAGE_KEY = 'kronk_chat_sampling';

export interface SamplingParams {
  maxTokens: number;
  temperature: number;
  topP: number;
  topK: number;
  minP: number;
  repeatPenalty: number;
  repeatLastN: number;
  dryMultiplier: number;
  dryBase: number;
  dryAllowedLen: number;
  dryPenaltyLast: number;
  xtcProbability: number;
  xtcThreshold: number;
  xtcMinKeep: number;
  enableThinking: string;
  reasoningEffort: string;
  returnPrompt: boolean;
  includeUsage: boolean;
  logprobs: boolean;
  topLogprobs: number;
}

export const defaultSampling: SamplingParams = {
  maxTokens: 2048,
  temperature: 0.8,
  topP: 0.9,
  topK: 40,
  minP: 0,
  repeatPenalty: 1.0,
  repeatLastN: 64,
  dryMultiplier: 1.05,
  dryBase: 1.75,
  dryAllowedLen: 2,
  dryPenaltyLast: 0,
  xtcProbability: 0,
  xtcThreshold: 0.1,
  xtcMinKeep: 1,
  enableThinking: '',
  reasoningEffort: '',
  returnPrompt: false,
  includeUsage: true,
  logprobs: false,
  topLogprobs: 0,
};

interface SamplingContextType {
  sampling: SamplingParams;
  setSampling: (params: Partial<SamplingParams>) => void;
  resetSampling: () => void;
}

const SamplingContext = createContext<SamplingContextType | null>(null);

export function SamplingProvider({ children }: { children: ReactNode }) {
  const [sampling, setSamplingState] = useState<SamplingParams>(() => {
    try {
      const stored = localStorage.getItem(SAMPLING_STORAGE_KEY);
      if (stored) {
        return { ...defaultSampling, ...JSON.parse(stored) };
      }
    } catch {
      // Ignore parse errors
    }
    return defaultSampling;
  });

  useEffect(() => {
    try {
      localStorage.setItem(SAMPLING_STORAGE_KEY, JSON.stringify(sampling));
    } catch {
      // Ignore storage errors
    }
  }, [sampling]);

  const setSampling = useCallback((params: Partial<SamplingParams>) => {
    setSamplingState(prev => ({ ...prev, ...params }));
  }, []);

  const resetSampling = useCallback(() => {
    setSamplingState(defaultSampling);
    localStorage.removeItem(SAMPLING_STORAGE_KEY);
  }, []);

  return (
    <SamplingContext.Provider value={{ sampling, setSampling, resetSampling }}>
      {children}
    </SamplingContext.Provider>
  );
}

export function useSampling() {
  const context = useContext(SamplingContext);
  if (!context) {
    throw new Error('useSampling must be used within a SamplingProvider');
  }
  return context;
}

// Helper to check if a sampling parameter differs from a baseline
export function isChangedFrom<K extends keyof SamplingParams>(
  key: K,
  value: SamplingParams[K],
  baseline: SamplingParams | null
): boolean {
  if (!baseline) return false;
  return value !== baseline[key];
}

// Helper to format a baseline value for display
export function formatBaselineValue<K extends keyof SamplingParams>(
  key: K,
  baseline: SamplingParams | null
): string {
  if (!baseline) return '';
  const val = baseline[key];
  if (typeof val === 'boolean') {
    return val ? 'true' : 'false';
  }
  if (typeof val === 'string') {
    return val === '' ? 'default' : val;
  }
  return String(val);
}

// Basic (non-advanced) sampling parameter keys
const basicSamplingKeys: (keyof SamplingParams)[] = [
  'maxTokens', 'temperature', 'topP', 'topK'
];

// Advanced sampling parameter keys
const advancedSamplingKeys: (keyof SamplingParams)[] = [
  'minP', 'repeatPenalty', 'repeatLastN',
  'dryMultiplier', 'dryBase', 'dryAllowedLen', 'dryPenaltyLast',
  'xtcProbability', 'xtcThreshold', 'xtcMinKeep',
  'enableThinking', 'reasoningEffort'
];

// Check if any sampling parameter differs from baseline
export function hasAnyChange(
  sampling: SamplingParams,
  baseline: SamplingParams | null
): boolean {
  if (!baseline) return false;
  return [...basicSamplingKeys, ...advancedSamplingKeys].some(
    key => sampling[key] !== baseline[key]
  );
}

// Check if any advanced sampling parameter differs from baseline
export function hasAdvancedChange(
  sampling: SamplingParams,
  baseline: SamplingParams | null
): boolean {
  if (!baseline) return false;
  return advancedSamplingKeys.some(key => sampling[key] !== baseline[key]);
}
