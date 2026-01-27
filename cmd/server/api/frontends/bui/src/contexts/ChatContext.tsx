import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from 'react';
import type { ChatUsage, ChatToolCall } from '../types';

const CHAT_STORAGE_KEY = 'kronk_chat_messages';

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
}

interface ChatContextType {
  messages: DisplayMessage[];
  setMessages: React.Dispatch<React.SetStateAction<DisplayMessage[]>>;
  clearMessages: () => void;
}

const ChatContext = createContext<ChatContextType | null>(null);

export function ChatProvider({ children }: { children: ReactNode }) {
  const [messages, setMessagesState] = useState<DisplayMessage[]>(() => {
    try {
      const stored = localStorage.getItem(CHAT_STORAGE_KEY);
      return stored ? JSON.parse(stored) : [];
    } catch {
      return [];
    }
  });

  useEffect(() => {
    try {
      if (messages.length > 0) {
        localStorage.setItem(CHAT_STORAGE_KEY, JSON.stringify(messages));
      } else {
        localStorage.removeItem(CHAT_STORAGE_KEY);
      }
    } catch {
      // Ignore storage errors
    }
  }, [messages]);

  const setMessages: React.Dispatch<React.SetStateAction<DisplayMessage[]>> = useCallback((action) => {
    setMessagesState(action);
  }, []);

  const clearMessages = useCallback(() => {
    setMessagesState([]);
    localStorage.removeItem(CHAT_STORAGE_KEY);
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
