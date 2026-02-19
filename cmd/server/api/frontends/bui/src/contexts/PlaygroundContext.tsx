import { createContext, useContext, useState, useMemo, type ReactNode } from 'react';
import type { PlaygroundSessionResponse } from '../types';

interface PlaygroundMessage {
  role: 'user' | 'assistant' | 'system';
  content: string;
}

interface PlaygroundState {
  // Session
  session: PlaygroundSessionResponse | null;
  setSession: React.Dispatch<React.SetStateAction<PlaygroundSessionResponse | null>>;

  // Chat messages
  chatMessages: PlaygroundMessage[];
  setChatMessages: React.Dispatch<React.SetStateAction<PlaygroundMessage[]>>;

  // Selected model (needed to know which model the session belongs to)
  selectedModel: string;
  setSelectedModel: React.Dispatch<React.SetStateAction<string>>;

  // Playground mode (top-level section)
  playgroundMode: 'automated' | 'manual';
  setPlaygroundMode: React.Dispatch<React.SetStateAction<'automated' | 'manual'>>;

  // Active tab (within manual mode)
  activeTab: 'chat' | 'tools' | 'inspector';
  setActiveTab: React.Dispatch<React.SetStateAction<'chat' | 'tools' | 'inspector'>>;

  // System prompt
  systemPrompt: string;
  setSystemPrompt: React.Dispatch<React.SetStateAction<string>>;

  // TPS
  lastTPS: number | null;
  setLastTPS: React.Dispatch<React.SetStateAction<number | null>>;

  // Config state (locked after session creation)
  templateMode: 'builtin' | 'custom';
  setTemplateMode: React.Dispatch<React.SetStateAction<'builtin' | 'custom'>>;
  selectedTemplate: string;
  setSelectedTemplate: React.Dispatch<React.SetStateAction<string>>;
  customScript: string;
  setCustomScript: React.Dispatch<React.SetStateAction<string>>;
  contextWindow: number;
  setContextWindow: React.Dispatch<React.SetStateAction<number>>;
  nBatch: number;
  setNBatch: React.Dispatch<React.SetStateAction<number>>;
  nUBatch: number;
  setNUBatch: React.Dispatch<React.SetStateAction<number>>;
  nSeqMax: number;
  setNSeqMax: React.Dispatch<React.SetStateAction<number>>;
  flashAttention: string;
  setFlashAttention: React.Dispatch<React.SetStateAction<string>>;
  cacheType: string;
  setCacheType: React.Dispatch<React.SetStateAction<string>>;
  systemPromptCache: boolean;
  setSystemPromptCache: React.Dispatch<React.SetStateAction<boolean>>;

  // Tracks which model's catalog config has been applied to avoid
  // re-clobbering user edits on component remount.
  hydratedModelId: string;
  setHydratedModelId: React.Dispatch<React.SetStateAction<string>>;
}

const PlaygroundContext = createContext<PlaygroundState | null>(null);

export function PlaygroundProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<PlaygroundSessionResponse | null>(null);
  const [chatMessages, setChatMessages] = useState<PlaygroundMessage[]>([]);
  const [selectedModel, setSelectedModel] = useState('');
  const [playgroundMode, setPlaygroundMode] = useState<'automated' | 'manual'>('automated');
  const [activeTab, setActiveTab] = useState<'chat' | 'tools' | 'inspector'>('chat');
  const [systemPrompt, setSystemPrompt] = useState('You are a helpful assistant.');
  const [lastTPS, setLastTPS] = useState<number | null>(null);
  const [templateMode, setTemplateMode] = useState<'builtin' | 'custom'>('builtin');
  const [selectedTemplate, setSelectedTemplate] = useState('');
  const [customScript, setCustomScript] = useState('');
  const [contextWindow, setContextWindow] = useState(8192);
  const [nBatch, setNBatch] = useState(2048);
  const [nUBatch, setNUBatch] = useState(512);
  const [nSeqMax, setNSeqMax] = useState(1);
  const [flashAttention, setFlashAttention] = useState('auto');
  const [cacheType, setCacheType] = useState('');
  const [systemPromptCache, setSystemPromptCache] = useState(false);
  const [hydratedModelId, setHydratedModelId] = useState('');

  const value = useMemo<PlaygroundState>(() => ({
    session, setSession,
    chatMessages, setChatMessages,
    selectedModel, setSelectedModel,
    playgroundMode, setPlaygroundMode,
    activeTab, setActiveTab,
    systemPrompt, setSystemPrompt,
    lastTPS, setLastTPS,
    templateMode, setTemplateMode,
    selectedTemplate, setSelectedTemplate,
    customScript, setCustomScript,
    contextWindow, setContextWindow,
    nBatch, setNBatch,
    nUBatch, setNUBatch,
    nSeqMax, setNSeqMax,
    flashAttention, setFlashAttention,
    cacheType, setCacheType,
    systemPromptCache, setSystemPromptCache,
    hydratedModelId, setHydratedModelId,
  }), [
    session, chatMessages, selectedModel, playgroundMode, activeTab, systemPrompt, lastTPS,
    templateMode, selectedTemplate, customScript, contextWindow, nBatch, nUBatch,
    nSeqMax, flashAttention, cacheType, systemPromptCache, hydratedModelId,
  ]);

  return (
    <PlaygroundContext.Provider value={value}>
      {children}
    </PlaygroundContext.Provider>
  );
}

export function usePlayground() {
  const context = useContext(PlaygroundContext);
  if (!context) {
    throw new Error('usePlayground must be used within a PlaygroundProvider');
  }
  return context;
}
