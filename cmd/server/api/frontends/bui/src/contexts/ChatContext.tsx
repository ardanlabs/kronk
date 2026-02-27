import { createContext, useContext, useState, useCallback, useEffect, useRef, type ReactNode } from 'react';
import type { ChatUsage, ChatToolCall } from '../types';
import * as chatDb from '../services/chatDb';

interface AttachedFile {
  type: 'image' | 'audio';
  name: string;
  dataUrl: string;
}

export interface DisplayMessage {
  role: 'user' | 'assistant';
  content: string;
  reasoning?: string;
  usage?: ChatUsage;
  toolCalls?: ChatToolCall[];
  attachments?: AttachedFile[];
  originalContent?: string;
}

interface ChatContextType {
  messages: DisplayMessage[];
  setMessages: React.Dispatch<React.SetStateAction<DisplayMessage[]>>;
  clearMessages: () => void;
}

const ChatContext = createContext<ChatContextType | null>(null);

export function ChatProvider({ children }: { children: ReactNode }) {
  const [messages, setMessagesState] = useState<DisplayMessage[]>([]);
  const hydratedRef = useRef(false);

  const persistChainRef = useRef(Promise.resolve());

  useEffect(() => {
    let cancelled = false
    chatDb.getSessionMessages().then((loaded) => {
      if (cancelled) return
      setMessagesState((prev) => (prev.length === 0 ? loaded : prev))
      hydratedRef.current = true
    })
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    if (!hydratedRef.current) return
    const t = window.setTimeout(() => {
      persistChainRef.current = persistChainRef.current
        .then(() => chatDb.setSessionMessages(messages))
        .catch(() => {})
    }, 250)
    return () => window.clearTimeout(t)
  }, [messages]);

  const setMessages: React.Dispatch<React.SetStateAction<DisplayMessage[]>> = useCallback((action) => {
    setMessagesState(action);
  }, []);

  const clearMessages = useCallback(() => {
    setMessagesState([]);
    void chatDb.clearSessionMessages();
  }, []);

  return (
    <ChatContext.Provider value={{ messages, setMessages, clearMessages }}>
      {children}
    </ChatContext.Provider>
  );
}

export function useChatMessages() {
  const context = useContext(ChatContext);
  if (!context) {
    throw new Error('useChatMessages must be used within a ChatProvider');
  }
  return context;
}
