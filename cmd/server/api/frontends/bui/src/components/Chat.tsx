import { useState, useEffect, useRef } from 'react';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import type { ChatMessage, ChatUsage, ChatToolCall } from '../types';

interface DisplayMessage {
  role: 'user' | 'assistant';
  content: string;
  reasoning?: string;
  usage?: ChatUsage;
  toolCalls?: ChatToolCall[];
}

function highlightCode(code: string, lang: string): string {
  const keywords: Record<string, string[]> = {
    go: ['func', 'return', 'if', 'else', 'for', 'range', 'switch', 'case', 'default', 'break', 'continue', 'package', 'import', 'var', 'const', 'type', 'struct', 'interface', 'map', 'chan', 'go', 'defer', 'select', 'nil', 'true', 'false', 'make', 'new', 'len', 'cap', 'append', 'copy', 'delete', 'error'],
    javascript: ['function', 'return', 'if', 'else', 'for', 'while', 'switch', 'case', 'default', 'break', 'continue', 'var', 'let', 'const', 'class', 'extends', 'import', 'export', 'from', 'async', 'await', 'try', 'catch', 'throw', 'new', 'this', 'null', 'undefined', 'true', 'false', 'typeof', 'instanceof'],
    typescript: ['function', 'return', 'if', 'else', 'for', 'while', 'switch', 'case', 'default', 'break', 'continue', 'var', 'let', 'const', 'class', 'extends', 'import', 'export', 'from', 'async', 'await', 'try', 'catch', 'throw', 'new', 'this', 'null', 'undefined', 'true', 'false', 'typeof', 'instanceof', 'interface', 'type', 'enum', 'implements', 'private', 'public', 'protected', 'readonly'],
    python: ['def', 'return', 'if', 'elif', 'else', 'for', 'while', 'break', 'continue', 'class', 'import', 'from', 'as', 'try', 'except', 'raise', 'with', 'lambda', 'None', 'True', 'False', 'and', 'or', 'not', 'in', 'is', 'pass', 'yield', 'async', 'await'],
    rust: ['fn', 'return', 'if', 'else', 'for', 'while', 'loop', 'match', 'break', 'continue', 'let', 'mut', 'const', 'struct', 'enum', 'impl', 'trait', 'use', 'mod', 'pub', 'self', 'Self', 'true', 'false', 'Some', 'None', 'Ok', 'Err', 'async', 'await', 'move', 'where'],
    sql: ['SELECT', 'FROM', 'WHERE', 'INSERT', 'UPDATE', 'DELETE', 'CREATE', 'DROP', 'ALTER', 'TABLE', 'INDEX', 'JOIN', 'LEFT', 'RIGHT', 'INNER', 'OUTER', 'ON', 'AND', 'OR', 'NOT', 'NULL', 'ORDER', 'BY', 'GROUP', 'HAVING', 'LIMIT', 'OFFSET', 'AS', 'DISTINCT', 'COUNT', 'SUM', 'AVG', 'MAX', 'MIN'],
  };

  const langKeywords = keywords[lang.toLowerCase()] || keywords['go'] || [];
  
  let escaped = code
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');

  escaped = escaped.replace(/(\/\/.*$|#.*$)/gm, '<span class="code-comment">$1</span>');
  escaped = escaped.replace(/(\/\*[\s\S]*?\*\/)/g, '<span class="code-comment">$1</span>');
  escaped = escaped.replace(/("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|`(?:[^`\\]|\\.)*`)/g, '<span class="code-string">$1</span>');
  escaped = escaped.replace(/\b(\d+\.?\d*)\b/g, '<span class="code-number">$1</span>');
  
  if (langKeywords.length > 0) {
    const keywordPattern = new RegExp(`\\b(${langKeywords.join('|')})\\b`, 'g');
    escaped = escaped.replace(keywordPattern, '<span class="code-keyword">$1</span>');
  }

  escaped = escaped.replace(/\b([A-Z][a-zA-Z0-9]*)\b/g, '<span class="code-type">$1</span>');
  escaped = escaped.replace(/\b([a-z_][a-zA-Z0-9_]*)\s*\(/g, '<span class="code-function">$1</span>(');

  return escaped;
}

function renderContent(content: string): JSX.Element[] {
  const parts: JSX.Element[] = [];
  const codeBlockRegex = /```(\w*)\n?([\s\S]*?)```/g;
  let lastIndex = 0;
  let match;
  let key = 0;

  while ((match = codeBlockRegex.exec(content)) !== null) {
    if (match.index > lastIndex) {
      const text = content.slice(lastIndex, match.index);
      parts.push(<span key={key++}>{renderInlineCode(text)}</span>);
    }

    const lang = match[1] || 'text';
    const code = match[2].trim();
    const highlighted = highlightCode(code, lang);

    parts.push(
      <div key={key++}>
        <div className="chat-code-header">
          <span className="chat-code-lang">{lang}</span>
        </div>
        <pre><code dangerouslySetInnerHTML={{ __html: highlighted }} /></pre>
      </div>
    );

    lastIndex = match.index + match[0].length;
  }

  if (lastIndex < content.length) {
    const text = content.slice(lastIndex);
    parts.push(<span key={key++}>{renderInlineCode(text)}</span>);
  }

  return parts;
}

function renderInlineCode(text: string): JSX.Element[] {
  const parts: JSX.Element[] = [];
  const inlineCodeRegex = /`([^`]+)`/g;
  let lastIndex = 0;
  let match;
  let key = 0;

  while ((match = inlineCodeRegex.exec(text)) !== null) {
    if (match.index > lastIndex) {
      parts.push(<span key={key++}>{text.slice(lastIndex, match.index)}</span>);
    }
    parts.push(<code key={key++}>{match[1]}</code>);
    lastIndex = match.index + match[0].length;
  }

  if (lastIndex < text.length) {
    parts.push(<span key={key++}>{text.slice(lastIndex)}</span>);
  }

  return parts;
}

export default function Chat() {
  const { models, loading: modelsLoading, loadModels } = useModelList();
  const [selectedModel, setSelectedModel] = useState<string>('');
  const [messages, setMessages] = useState<DisplayMessage[]>([]);
  const [input, setInput] = useState('');
  const [isStreaming, setIsStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  
  const [maxTokens, setMaxTokens] = useState(2048);
  const [temperature, setTemperature] = useState(0.7);
  const [topP, setTopP] = useState(0.9);
  const [topK, setTopK] = useState(40);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    loadModels();
  }, [loadModels]);

  useEffect(() => {
    if (models?.data && models.data.length > 0 && !selectedModel) {
      setSelectedModel(models.data[0].id);
    }
  }, [models, selectedModel]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || !selectedModel || isStreaming) return;

    const userMessage: DisplayMessage = { role: 'user', content: input.trim() };
    setMessages(prev => [...prev, userMessage]);
    setInput('');
    setError(null);
    setIsStreaming(true);

    const chatMessages: ChatMessage[] = [
      ...messages.map(m => ({ role: m.role, content: m.content })),
      { role: 'user' as const, content: input.trim() }
    ];

    let currentContent = '';
    let currentReasoning = '';
    let lastUsage: ChatUsage | undefined;
    let currentToolCalls: ChatToolCall[] = [];

    setMessages(prev => [...prev, { role: 'assistant', content: '', reasoning: '' }]);

    abortRef.current = api.streamChat(
      {
        model: selectedModel,
        messages: chatMessages,
        max_tokens: maxTokens,
        temperature,
        top_p: topP,
        top_k: topK,
      },
      (data) => {
        const choice = data.choices?.[0];
        if (choice?.delta?.content) {
          currentContent += choice.delta.content;
        }
        if (choice?.delta?.reasoning) {
          currentReasoning += choice.delta.reasoning;
        }
        if (choice?.delta?.tool_calls && choice.delta.tool_calls.length > 0) {
          currentToolCalls = [...currentToolCalls, ...choice.delta.tool_calls];
        }
        if (data.usage) {
          lastUsage = data.usage;
        }

        setMessages(prev => {
          const updated = [...prev];
          updated[updated.length - 1] = {
            role: 'assistant',
            content: currentContent,
            reasoning: currentReasoning,
            usage: lastUsage,
            toolCalls: currentToolCalls.length ? currentToolCalls : undefined,
          };
          return updated;
        });
      },
      (err) => {
        setError(err);
        setIsStreaming(false);
      },
      () => {
        setIsStreaming(false);
      }
    );
  };

  const handleStop = () => {
    if (abortRef.current) {
      abortRef.current();
      abortRef.current = null;
      setIsStreaming(false);
    }
  };

  const handleClear = () => {
    setMessages([]);
    setError(null);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit(e);
    }
  };

  return (
    <div className="chat-container">
      <div className="chat-header">
        <div className="chat-header-left">
          <h2>Run</h2>
          <select
            value={selectedModel}
            onChange={(e) => setSelectedModel(e.target.value)}
            disabled={modelsLoading || isStreaming}
            className="chat-model-select"
          >
            {modelsLoading && <option>Loading models...</option>}
            {!modelsLoading && models?.data?.length === 0 && (
              <option>No models available</option>
            )}
            {models?.data?.map((model) => (
              <option key={model.id} value={model.id}>
                {model.id}
              </option>
            ))}
          </select>
        </div>
        <div className="chat-header-right">
          <button
            className="btn btn-secondary btn-sm"
            onClick={() => setShowSettings(!showSettings)}
          >
            Settings
          </button>
          <button
            className="btn btn-secondary btn-sm"
            onClick={handleClear}
            disabled={isStreaming || messages.length === 0}
          >
            Clear
          </button>
        </div>
      </div>

      {showSettings && (
        <div className="chat-settings">
          <div className="chat-setting">
            <label>Max Tokens</label>
            <input
              type="number"
              value={maxTokens}
              onChange={(e) => setMaxTokens(Number(e.target.value))}
              min={1}
              max={32768}
            />
          </div>
          <div className="chat-setting">
            <label>Temperature</label>
            <input
              type="number"
              value={temperature}
              onChange={(e) => setTemperature(Number(e.target.value))}
              min={0}
              max={2}
              step={0.1}
            />
          </div>
          <div className="chat-setting">
            <label>Top P</label>
            <input
              type="number"
              value={topP}
              onChange={(e) => setTopP(Number(e.target.value))}
              min={0}
              max={1}
              step={0.05}
            />
          </div>
          <div className="chat-setting">
            <label>Top K</label>
            <input
              type="number"
              value={topK}
              onChange={(e) => setTopK(Number(e.target.value))}
              min={1}
              max={100}
            />
          </div>
        </div>
      )}

      {error && <div className="alert alert-error">{error}</div>}

      <div className="chat-messages">
        {messages.length === 0 && (
          <div className="chat-empty">
            <p>Select a model and start chatting</p>
            <p className="chat-empty-hint">Type a message below to begin</p>
          </div>
        )}
        {messages.map((msg, idx) => (
          <div key={idx} className={`chat-message chat-message-${msg.role}`}>
            <div className="chat-message-header">
              {msg.role === 'user' ? 'USER' : 'MODEL'}
            </div>
            {msg.reasoning && (
              <div className="chat-message-reasoning">{msg.reasoning}</div>
            )}
            <div className="chat-message-content">
              {msg.content ? renderContent(msg.content) : (isStreaming && idx === messages.length - 1 ? '...' : '')}
            </div>
            {msg.toolCalls && msg.toolCalls.length > 0 && (
              <div className="chat-message-tool-calls">
                {msg.toolCalls.map((tc) => (
                  <div key={tc.id} className="chat-tool-call">
                    Tool call {tc.id}: {tc.function.name}({tc.function.arguments})
                  </div>
                ))}
              </div>
            )}
            {msg.usage && (
              <div className="chat-message-usage">
                Input: {msg.usage.prompt_tokens} | 
                Reasoning: {msg.usage.reasoning_tokens} | 
                Completion: {msg.usage.completion_tokens} | 
                Output: {msg.usage.output_tokens} | 
                TPS: {msg.usage.tokens_per_second.toFixed(2)}
              </div>
            )}
          </div>
        ))}
        <div ref={messagesEndRef} />
      </div>

      <form onSubmit={handleSubmit} className="chat-input-form">
        <textarea
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Type your message... (Enter to send, Shift+Enter for new line)"
          disabled={isStreaming || !selectedModel}
          className="chat-input"
          rows={3}
        />
        <div className="chat-input-actions">
          {isStreaming ? (
            <button type="button" className="btn btn-danger" onClick={handleStop}>
              Stop
            </button>
          ) : (
            <button
              type="submit"
              className="btn btn-primary"
              disabled={!input.trim() || !selectedModel}
            >
              Send
            </button>
          )}
        </div>
      </form>
    </div>
  );
}
