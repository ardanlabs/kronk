import { createContext, useContext, useState, useCallback, useRef, useEffect, type ReactNode } from 'react';
import type { DisplayMessage } from '../contexts/ChatContext';
import * as chatDb from '../services/chatDb';

export interface HistoryAttachment {
  type: 'image' | 'audio';
  name: string;
}

export interface HistoryMessage {
  role: 'user' | 'assistant';
  content: string;
  reasoning?: string;
  attachments?: HistoryAttachment[];
}

export interface SavedChat {
  id: string;
  title: string;
  model: string;
  savedAt: number;
  messages: HistoryMessage[];
}

interface ChatHistoryContextType {
  history: SavedChat[];
  saveChat: (model: string, messages: DisplayMessage[]) => void;
  deleteChats: (ids: string[]) => void;
  getChat: (id: string) => SavedChat | undefined;
  clearHistory: () => void;
}

const ChatHistoryContext = createContext<ChatHistoryContextType | null>(null);

function generateId(): string {
  const timePart = Date.now().toString(36);
  const randomPart = Math.random().toString(36).substring(2, 6);
  return `${timePart}-${randomPart}`;
}

function generateTitle(messages: DisplayMessage[]): string {
  const firstUserMsg = messages.find((m) => m.role === 'user');
  if (!firstUserMsg) {
    return 'Untitled Chat';
  }

  const content = firstUserMsg.content.trim();
  if (content.length <= 60) {
    return content;
  }

  return content.substring(0, 60) + '...';
}

function stripMessages(messages: DisplayMessage[]): HistoryMessage[] {
  return messages.map((m) => {
    const stripped: HistoryMessage = {
      role: m.role,
      content: m.content,
    };

    if (m.reasoning) {
      stripped.reasoning = m.reasoning;
    }

    if (m.attachments && m.attachments.length > 0) {
      stripped.attachments = m.attachments.map((a) => ({
        type: a.type,
        name: a.name,
      }));
    }

    return stripped;
  });
}

export function ChatHistoryProvider({ children }: { children: ReactNode }) {
  const [history, setHistory] = useState<SavedChat[]>([]);
  const hydratedRef = useRef(false);

  useEffect(() => {
    let cancelled = false;
    chatDb.getAllHistory().then((loaded) => {
      if (cancelled) return;
      setHistory(loaded);
      hydratedRef.current = true;
    });
    return () => { cancelled = true; };
  }, []);

  const saveChat = useCallback((model: string, messages: DisplayMessage[]) => {
    if (messages.length === 0) {
      return;
    }

    const chat: SavedChat = {
      id: generateId(),
      title: generateTitle(messages),
      model,
      savedAt: Date.now(),
      messages: stripMessages(messages),
    };

    setHistory((prev) => [chat, ...prev]);
    void chatDb.putHistoryChat(chat);
  }, []);

  const deleteChats = useCallback((ids: string[]) => {
    const idSet = new Set(ids);
    setHistory((prev) => prev.filter((c) => !idSet.has(c.id)));
    void chatDb.deleteHistoryChats(ids);
  }, []);

  const getChat = useCallback((id: string): SavedChat | undefined => {
    return history.find((c) => c.id === id);
  }, [history]);

  const clearHistory = useCallback(() => {
    setHistory([]);
    void chatDb.clearAllHistory();
  }, []);

  return (
    <ChatHistoryContext.Provider value={{ history, saveChat, deleteChats, getChat, clearHistory }}>
      {children}
    </ChatHistoryContext.Provider>
  );
}

export function useChatHistory() {
  const context = useContext(ChatHistoryContext);
  if (!context) {
    throw new Error('useChatHistory must be used within a ChatHistoryProvider');
  }
  return context;
}
