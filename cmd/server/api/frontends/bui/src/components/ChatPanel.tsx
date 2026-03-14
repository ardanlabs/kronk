import { useState, useEffect, useRef, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { api } from '../services/api';
import type { DisplayMessage } from '../contexts/ChatContext';
import { isChangedFrom, formatBaselineValue, hasAnyChange, hasAdvancedChange, type SamplingParams } from '../contexts/SamplingContext';
import CodeBlock from './CodeBlock';
import { PARAM_TOOLTIPS, FieldLabel } from './ParamTooltips';
import { formatBytes } from '../lib/format';
import type { ChatMessage, ChatUsage, ChatToolCall, ChatContentPart, ChatStreamResponse, VRAM } from '../types';
import { VRAM_FIT_TEXT } from './vram';
import type { DevicesInfo, MoeFitContext } from './vram';

// ── Types ──────────────────────────────────────────────────────────────

interface AttachedFile {
  type: 'image' | 'audio';
  name: string;
  dataUrl: string;
}

export type StreamTransport = (args: {
  messages: ChatMessage[];
  sampling: SamplingParams;
  onMessage: (data: ChatStreamResponse) => void;
  onError: (error: string) => void;
  onComplete: () => void;
}) => (() => void); // returns abort function

export interface ChatPanelProps {
  messages: DisplayMessage[];
  setMessages: React.Dispatch<React.SetStateAction<DisplayMessage[]>>;
  onClear: () => void;

  sampling: SamplingParams;
  setSampling: (params: Partial<SamplingParams>) => void;
  modelBaseline: SamplingParams | null;

  transport: StreamTransport;

  modelVRAM?: VRAM | null;
  devicesInfo?: DevicesInfo | null;
  moeFit?: MoeFitContext;

  disabled?: boolean;
  disabledPlaceholder?: string;
  headerLeft?: React.ReactNode;
  headerRight?: React.ReactNode;
}

// ── Markdown helpers ───────────────────────────────────────────────────

function preprocessContent(content: string): string {
  const openFences = (content.match(/```/g) || []).length;
  if (openFences % 2 !== 0) {
    return content + '\n```';
  }
  return content;
}

const markdownComponents = {
  code({ node, className, children, ...props }: any) {
    const match = /language-(\w+)/.exec(className || '');
    const isInline = !match && !className;
    if (isInline) {
      return <code className="inline-code" {...props}>{children}</code>;
    }
    const language = match ? match[1] : 'text';
    const codeString = String(children).replace(/\n$/, '');
    return (
      <div className="chat-code-block-wrapper">
        <CodeBlock code={codeString} language={language} collapsible={true} />
      </div>
    );
  },
  h1: ({ children }: any) => <h1 className="markdown-h1">{children}</h1>,
  h2: ({ children }: any) => <h2 className="markdown-h2">{children}</h2>,
  h3: ({ children }: any) => <h3 className="markdown-h3">{children}</h3>,
  ul: ({ children }: any) => <ul className="markdown-list">{children}</ul>,
  ol: ({ children }: any) => <ol className="markdown-list markdown-list-ordered">{children}</ol>,
  li: ({ children }: any) => <li className="markdown-list-item">{children}</li>,
  p: ({ children }: any) => <p className="markdown-paragraph">{children}</p>,
  strong: ({ children }: any) => <strong className="markdown-bold">{children}</strong>,
  em: ({ children }: any) => <em className="markdown-italic">{children}</em>,
  blockquote: ({ children }: any) => <blockquote className="markdown-blockquote">{children}</blockquote>,
  a: ({ href, children }: any) => <a href={href} className="markdown-link" target="_blank" rel="noopener noreferrer">{children}</a>,
};

function renderContent(content: string): JSX.Element {
  const processedContent = preprocessContent(content);
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={markdownComponents}
    >
      {processedContent}
    </ReactMarkdown>
  );
}

// ── Component ──────────────────────────────────────────────────────────

export default function ChatPanel({
  messages,
  setMessages,
  onClear,
  sampling,
  setSampling,
  modelBaseline,
  transport,
  modelVRAM,
  devicesInfo,
  moeFit,
  disabled = false,
  disabledPlaceholder,
  headerLeft,
  headerRight,
}: ChatPanelProps) {
  const [input, setInput] = useState('');
  const [isStreaming, setIsStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [attachedFiles, setAttachedFiles] = useState<AttachedFile[]>([]);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [editContent, setEditContent] = useState('');
  const [grammarFiles, setGrammarFiles] = useState<string[]>([]);
  const [grammarMode, setGrammarMode] = useState<'none' | 'preset' | 'custom'>('none');
  const [selectedPreset, setSelectedPreset] = useState('');

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const abortRef = useRef<(() => void) | null>(null);
  const autoScrollRef = useRef(true);
  const programmaticScrollRef = useRef(false);
  const userInteractedRef = useRef(false);
  const [showScrollBtn, setShowScrollBtn] = useState(false);

  useEffect(() => {
    api.listGrammars()
      .then(res => setGrammarFiles(res.files || []))
      .catch(() => setGrammarFiles([]));
  }, []);

  const loadPresetContent = useCallback((name: string) => {
    setSelectedPreset(name);
    api.getGrammarContent(name)
      .then(res => setSampling({ grammar: res.content }))
      .catch(() => setSampling({ grammar: '' }));
  }, [setSampling]);

  // ── Scroll logic ───────────────────────────────────────────────────

  const handleWheel = useCallback((e: React.WheelEvent) => {
    if (e.deltaY < 0) {
      const el = messagesContainerRef.current;
      if (!el) return;
      if (el.scrollHeight > el.clientHeight) {
        userInteractedRef.current = true;
        autoScrollRef.current = false;
        setShowScrollBtn(true);
      }
    }
  }, []);

  const handleMessagesScroll = useCallback(() => {
    if (programmaticScrollRef.current || !userInteractedRef.current) return;
    const el = messagesContainerRef.current;
    if (!el) return;
    const threshold = 100;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < threshold;
    if (atBottom) {
      autoScrollRef.current = true;
      setShowScrollBtn(false);
    }
  }, []);

  const scrollToBottom = useCallback(() => {
    const el = messagesContainerRef.current;
    if (!el) return;
    programmaticScrollRef.current = true;
    el.scrollTop = el.scrollHeight;
    autoScrollRef.current = true;
    setShowScrollBtn(false);
    requestAnimationFrame(() => {
      programmaticScrollRef.current = false;
    });
  }, []);

  useEffect(() => {
    if (autoScrollRef.current) {
      const el = messagesContainerRef.current;
      if (!el) return;
      programmaticScrollRef.current = true;
      el.scrollTop = el.scrollHeight;
      requestAnimationFrame(() => {
        programmaticScrollRef.current = false;
      });
    }
  }, [messages]);

  useEffect(() => {
    if (!isStreaming && !disabled) {
      inputRef.current?.focus();
    }
  }, [isStreaming, disabled]);

  // Abort streaming on unmount.
  useEffect(() => () => { abortRef.current?.(); }, []);

  // Abort if parent disables while streaming (e.g. session unload).
  useEffect(() => {
    if (disabled && isStreaming) {
      abortRef.current?.();
      abortRef.current = null;
      setIsStreaming(false);
    }
  }, [disabled, isStreaming]);

  // ── Message content building ───────────────────────────────────────

  const buildMessageContent = useCallback((text: string, files: AttachedFile[]): string | ChatContentPart[] => {
    if (files.length === 0) {
      return text;
    }
    const parts: ChatContentPart[] = [];
    if (text) {
      parts.push({ type: 'text', text });
    }
    for (const file of files) {
      if (file.type === 'image') {
        parts.push({
          type: 'image_url',
          image_url: { url: file.dataUrl },
        });
      } else if (file.type === 'audio') {
        const match = file.dataUrl.match(/^data:audio\/(\w+);base64,(.+)$/);
        if (match) {
          parts.push({
            type: 'input_audio',
            input_audio: { data: match[2], format: match[1] },
          });
        }
      }
    }
    return parts;
  }, []);

  // ── Streaming ──────────────────────────────────────────────────────

  const streamResponse = useCallback((chatMessages: ChatMessage[]) => {
    setError(null);
    setIsStreaming(true);

    let currentContent = '';
    let currentReasoning = '';
    let lastUsage: ChatUsage | undefined;
    let currentToolCalls: ChatToolCall[] = [];

    setMessages(prev => [...prev, { role: 'assistant', content: '', reasoning: '' }]);

    abortRef.current = transport({
      messages: chatMessages,
      sampling,
      onMessage: (data) => {
        const choice = data.choices?.[0];
        if (choice?.delta?.content) {
          currentContent += choice.delta.content;
        }
        if (choice?.delta?.reasoning_content) {
          currentReasoning += choice.delta.reasoning_content;
        }
        if (choice?.delta?.tool_calls && choice.delta.tool_calls.length > 0) {
          for (const tc of choice.delta.tool_calls) {
            const idx = tc.index ?? (currentToolCalls.length > 0 ? currentToolCalls[currentToolCalls.length - 1].index : 0);
            const existing = currentToolCalls.find(c => c.index === idx);
            if (existing) {
              if (tc.id && !existing.id) existing.id = tc.id;
              if (tc.function?.name && !existing.function.name) existing.function.name = tc.function.name;
              if (tc.function?.arguments) existing.function.arguments += tc.function.arguments;
            } else {
              currentToolCalls.push({
                id: tc.id || '',
                index: idx,
                type: tc.type || 'function',
                function: {
                  name: tc.function?.name || '',
                  arguments: tc.function?.arguments || '',
                },
              });
            }
          }
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
      onError: (err) => {
        setError(err);
        setIsStreaming(false);
      },
      onComplete: () => {
        setIsStreaming(false);
      },
    });
  }, [sampling, setMessages, transport]);

  // ── Handlers ───────────────────────────────────────────────────────

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if ((!input.trim() && attachedFiles.length === 0) || disabled || isStreaming) return;

    const userMessage: DisplayMessage = {
      role: 'user',
      content: input.trim(),
      attachments: attachedFiles.length > 0 ? [...attachedFiles] : undefined,
    };
    setMessages(prev => [...prev, userMessage]);
    setInput('');
    setAttachedFiles([]);

    const chatMessages: ChatMessage[] = [
      ...(sampling.systemPrompt ? [{ role: 'system' as const, content: sampling.systemPrompt }] : []),
      ...messages.map(m => ({
        role: m.role,
        content: m.attachments ? buildMessageContent(m.content, m.attachments) : m.content,
      })),
      { role: 'user' as const, content: buildMessageContent(input.trim(), attachedFiles) },
    ];

    streamResponse(chatMessages);
  };

  const handleEditStart = (index: number) => {
    setEditingIndex(index);
    setEditContent(messages[index].content);
  };

  const handleEditCancel = () => {
    setEditingIndex(null);
    setEditContent('');
  };

  const handleEditSave = (index: number) => {
    setMessages(prev => prev.map((m, i) =>
      i === index ? { ...m, content: editContent, originalContent: m.originalContent ?? m.content } : m
    ));
    setEditingIndex(null);
    setEditContent('');
  };

  const handleEditUndo = (index: number) => {
    setMessages(prev => prev.map((m, i) =>
      i === index && m.originalContent != null ? { ...m, content: m.originalContent, originalContent: undefined } : m
    ));
  };

  const handleEditSend = (index: number) => {
    if (disabled || isStreaming) return;

    const updatedMessages = messages.slice(0, index + 1).map((m, i) => {
      if (i === index) {
        return { ...m, content: editContent };
      }
      return m;
    });

    setMessages(updatedMessages);
    setEditingIndex(null);
    setEditContent('');

    const chatMessages: ChatMessage[] = [
      ...(sampling.systemPrompt ? [{ role: 'system' as const, content: sampling.systemPrompt }] : []),
      ...updatedMessages.map(m => ({
        role: m.role,
        content: m.attachments ? buildMessageContent(m.content, m.attachments) : m.content,
      })),
    ];

    streamResponse(chatMessages);
  };

  const handleStop = () => {
    if (abortRef.current) {
      abortRef.current();
      abortRef.current = null;
      setIsStreaming(false);
    }
  };

  const handleClear = () => {
    if (abortRef.current) {
      abortRef.current();
      abortRef.current = null;
      setIsStreaming(false);
    }
    onClear();
    setError(null);
    setAttachedFiles([]);
  };

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (!files) return;
    Array.from(files).forEach(file => {
      const reader = new FileReader();
      reader.onload = () => {
        const dataUrl = reader.result as string;
        const fileType: 'image' | 'audio' = file.type.startsWith('image/') ? 'image' : 'audio';
        setAttachedFiles(prev => [...prev, { type: fileType, name: file.name, dataUrl }]);
      };
      reader.readAsDataURL(file);
    });
    e.target.value = '';
  };

  const removeAttachment = (index: number) => {
    setAttachedFiles(prev => prev.filter((_, i) => i !== index));
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit(e);
    }
  };

  // ── Render ─────────────────────────────────────────────────────────

  return (
    <div className="chat-container">
      <div className="chat-header">
        <div className="chat-header-left">
          {headerLeft}
        </div>
        <div className="chat-header-right">
          {headerRight}
          <button
            className="btn btn-secondary btn-sm"
            onClick={() => setShowSettings(!showSettings)}
          >
            Settings
            {hasAnyChange(sampling, modelBaseline) && (
              <span className="chat-setting-default">●</span>
            )}
          </button>
          <button
            className="btn btn-secondary btn-sm"
            onClick={handleClear}
            disabled={isStreaming || messages.length === 0}
          >
            Clear chat
          </button>
        </div>
      </div>

      {showSettings && (
        <>
        <div className="chat-system-prompt">
          <label>System Prompt</label>
          <textarea
            value={sampling.systemPrompt}
            onChange={(e) => setSampling({ systemPrompt: e.target.value })}
            placeholder="Enter a system prompt to set the model's behavior..."
            className="chat-system-prompt-input"
            rows={3}
          />
        </div>
        <div className="chat-settings">
          <div className={`chat-setting ${isChangedFrom('maxTokens', sampling.maxTokens, modelBaseline) ? 'chat-setting-changed' : ''}`}>
            <FieldLabel tooltipKey="max_tokens" after={isChangedFrom('maxTokens', sampling.maxTokens, modelBaseline) && (
                <span className="chat-setting-default" title={`Default: ${formatBaselineValue('maxTokens', modelBaseline)}`}>●</span>
              )}>
              Max Tokens
            </FieldLabel>
            <input
              type="number"
              value={sampling.maxTokens}
              onChange={(e) => setSampling({ maxTokens: Number(e.target.value) })}
              min={1}
              max={32768}
            />
          </div>
          <div className={`chat-setting ${isChangedFrom('temperature', sampling.temperature, modelBaseline) ? 'chat-setting-changed' : ''}`}>
            <FieldLabel tooltipKey="temperature" after={isChangedFrom('temperature', sampling.temperature, modelBaseline) && (
                <span className="chat-setting-default" title={`Default: ${formatBaselineValue('temperature', modelBaseline)}`}>●</span>
              )}>
              Temperature
            </FieldLabel>
            <input
              type="number"
              value={sampling.temperature}
              onChange={(e) => setSampling({ temperature: Number(e.target.value) })}
              min={0}
              max={2}
              step={0.1}
            />
          </div>
          <div className={`chat-setting ${isChangedFrom('topP', sampling.topP, modelBaseline) ? 'chat-setting-changed' : ''}`}>
            <FieldLabel tooltipKey="top_p" after={isChangedFrom('topP', sampling.topP, modelBaseline) && (
                <span className="chat-setting-default" title={`Default: ${formatBaselineValue('topP', modelBaseline)}`}>●</span>
              )}>
              Top P
            </FieldLabel>
            <input
              type="number"
              value={sampling.topP}
              onChange={(e) => setSampling({ topP: Number(e.target.value) })}
              min={0}
              max={1}
              step={0.05}
            />
          </div>
          <div className={`chat-setting ${isChangedFrom('topK', sampling.topK, modelBaseline) ? 'chat-setting-changed' : ''}`}>
            <FieldLabel tooltipKey="top_k" after={isChangedFrom('topK', sampling.topK, modelBaseline) && (
                <span className="chat-setting-default" title={`Default: ${formatBaselineValue('topK', modelBaseline)}`}>●</span>
              )}>
              Top K
            </FieldLabel>
            <input
              type="number"
              value={sampling.topK}
              onChange={(e) => setSampling({ topK: Number(e.target.value) })}
              min={1}
              max={100}
            />
          </div>
          <div className="chat-setting">
            <label>Cache ID</label>
            <input
              type="text"
              value={sampling.cacheId}
              onChange={(e) => setSampling({ cacheId: e.target.value })}
              placeholder="e.g. user-123"
            />
          </div>
          <div className="chat-setting chat-setting-grammar">
            <label>Grammar</label>
            <select
              value={grammarMode}
              onChange={(e) => {
                const mode = e.target.value as 'none' | 'preset' | 'custom';
                setGrammarMode(mode);
                if (mode === 'none') {
                  setSampling({ grammar: '' });
                  setSelectedPreset('');
                } else if (mode === 'preset' && grammarFiles.length > 0) {
                  loadPresetContent(grammarFiles[0]);
                } else if (mode === 'custom') {
                  setSampling({ grammar: '' });
                  setSelectedPreset('');
                }
              }}
            >
              <option value="none">None</option>
              <option value="preset">Preset</option>
              <option value="custom">Custom</option>
            </select>
          </div>
          {grammarMode === 'preset' && (
            <div className="chat-setting chat-setting-grammar-preset">
              <label>Preset</label>
              <select
                value={selectedPreset}
                onChange={(e) => loadPresetContent(e.target.value)}
              >
                {grammarFiles.map((f) => (
                  <option key={f} value={f}>{f.replace('.grm', '')}</option>
                ))}
              </select>
            </div>
          )}
          <div className="chat-setting chat-setting-button">
            <button
              type="button"
              className="chat-advanced-toggle"
              onClick={() => setShowAdvanced(!showAdvanced)}
            >
              Advanced {showAdvanced ? '▲' : '▼'}
              {hasAdvancedChange(sampling, modelBaseline) && (
                <span className="chat-setting-default">●</span>
              )}
            </button>
          </div>
          <div className="chat-setting chat-setting-button">
            {hasAnyChange(sampling, modelBaseline) && modelBaseline && (
              <button
                type="button"
                className="chat-reset-defaults"
                onClick={() => setSampling(modelBaseline)}
                title="Reset all sampling values to model defaults"
              >
                Reset to default
              </button>
            )}
          </div>
        </div>
        {(grammarMode === 'preset' || grammarMode === 'custom') && (
          <div className="chat-grammar-editor">
            <textarea
              value={sampling.grammar}
              onChange={(e) => setSampling({ grammar: e.target.value })}
              rows={4}
              placeholder="Enter GBNF grammar..."
              className="chat-system-prompt-input"
              style={{ fontFamily: 'monospace', fontSize: '12px' }}
            />
          </div>
        )}
        {moeFit?.isMoe && modelVRAM && (
          <div className="chat-settings" style={{ flexDirection: 'column', gap: '8px', padding: '12px', background: 'var(--color-gray-50)', borderRadius: '6px', margin: '0 0 8px' }}>
            <div style={{ fontWeight: 600, fontSize: '14px', display: 'flex', alignItems: 'center', gap: '6px' }}>
              🧩 MoE Model
              {modelVRAM.moe && (
                <span style={{ fontWeight: 400, fontSize: '12px', color: 'var(--color-text-secondary)' }}>
                  ({modelVRAM.moe.expert_count} experts, top-{modelVRAM.moe.expert_used_count} active{modelVRAM.moe.has_shared_experts ? ', shared experts' : ''})
                </span>
              )}
            </div>
            {modelVRAM.weights && (
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '4px 16px', fontSize: '12px', color: 'var(--color-text-secondary)' }}>
                <span>Always-active weights (GPU):</span>
                <span style={{ fontWeight: 500 }}>{formatBytes(modelVRAM.weights.always_active_bytes)}</span>
                {(modelVRAM.model_weights_gpu ?? 0) > 0 && (
                  <>
                    <span>Expert weights (GPU):</span>
                    <span style={{ fontWeight: 500 }}>{formatBytes((modelVRAM.model_weights_gpu ?? 0) - modelVRAM.weights.always_active_bytes)}</span>
                  </>
                )}
                {(modelVRAM.model_weights_cpu ?? 0) > 0 && (
                  <>
                    <span>Expert weights (CPU):</span>
                    <span style={{ fontWeight: 500 }}>{formatBytes(modelVRAM.model_weights_cpu ?? 0)}</span>
                  </>
                )}
                <span>KV cache:</span>
                <span style={{ fontWeight: 500 }}>{formatBytes(modelVRAM.slot_memory)}</span>
                {(modelVRAM.compute_buffer_est ?? 0) > 0 && (
                  <>
                    <span>Compute buffer (est.):</span>
                    <span style={{ fontWeight: 500 }}>~{formatBytes(modelVRAM.compute_buffer_est ?? 0)}</span>
                  </>
                )}
                <span style={{ fontWeight: 600 }}>Total estimated VRAM:</span>
                <span style={{ fontWeight: 600 }}>{formatBytes(modelVRAM.total_vram)}</span>
              </div>
            )}
            {!modelVRAM.weights && modelVRAM.total_vram > 0 && (
              <div style={{ fontSize: '12px', color: 'var(--color-text-secondary)' }}>
                Total VRAM: {formatBytes(modelVRAM.total_vram)}
              </div>
            )}
            {moeFit?.fit?.status && devicesInfo && (
              <div className={`playground-vram-fit playground-vram-fit--${moeFit.fit.status}`}>
                {VRAM_FIT_TEXT[moeFit.fit.status]}
                <span className="playground-vram-fit-detail">
                  {' '}({formatBytes(devicesInfo.gpuVramBytes)} available
                  {moeFit.fit.status !== 'fits' && `, ${formatBytes(moeFit.fit.cpuExperts)} needed with CPU experts`}
                  {moeFit.fit.status === 'fits' && `, ${formatBytes(moeFit.fit.allGPU)} needed`})
                </span>
              </div>
            )}
            <div style={{ fontSize: '11px', color: 'var(--color-text-secondary)', lineHeight: 1.4, marginTop: '4px' }}>
              💡 {PARAM_TOOLTIPS.moeTipBatch} {PARAM_TOOLTIPS.moeTipFlashAttention}
            </div>
          </div>
        )}
        {showAdvanced && (
          <div className="chat-settings">
            <div className="chat-advanced-settings">
                <div className={`chat-setting ${isChangedFrom('minP', sampling.minP, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="min_p" after={isChangedFrom('minP', sampling.minP, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('minP', modelBaseline)}`}>●</span>
                    )}>
                    Min P
                  </FieldLabel>
                  <input
                    type="number"
                    value={sampling.minP}
                    onChange={(e) => setSampling({ minP: Number(e.target.value) })}
                    min={0}
                    max={1}
                    step={0.01}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('repeatPenalty', sampling.repeatPenalty, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="repeat_penalty" after={isChangedFrom('repeatPenalty', sampling.repeatPenalty, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('repeatPenalty', modelBaseline)}`}>●</span>
                    )}>
                    Repeat Penalty
                  </FieldLabel>
                  <input
                    type="number"
                    value={sampling.repeatPenalty}
                    onChange={(e) => setSampling({ repeatPenalty: Number(e.target.value) })}
                    min={0}
                    max={2}
                    step={0.1}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('repeatLastN', sampling.repeatLastN, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="repeat_last_n" after={isChangedFrom('repeatLastN', sampling.repeatLastN, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('repeatLastN', modelBaseline)}`}>●</span>
                    )}>
                    Repeat Last N
                  </FieldLabel>
                  <input
                    type="number"
                    value={sampling.repeatLastN}
                    onChange={(e) => setSampling({ repeatLastN: Number(e.target.value) })}
                    min={-1}
                    max={512}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('frequencyPenalty', sampling.frequencyPenalty, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="frequency_penalty" after={isChangedFrom('frequencyPenalty', sampling.frequencyPenalty, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('frequencyPenalty', modelBaseline)}`}>●</span>
                    )}>
                    Frequency Penalty
                  </FieldLabel>
                  <input
                    type="number"
                    value={sampling.frequencyPenalty}
                    onChange={(e) => setSampling({ frequencyPenalty: Number(e.target.value) })}
                    min={0}
                    max={2}
                    step={0.1}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('presencePenalty', sampling.presencePenalty, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="presence_penalty" after={isChangedFrom('presencePenalty', sampling.presencePenalty, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('presencePenalty', modelBaseline)}`}>●</span>
                    )}>
                    Presence Penalty
                  </FieldLabel>
                  <input
                    type="number"
                    value={sampling.presencePenalty}
                    onChange={(e) => setSampling({ presencePenalty: Number(e.target.value) })}
                    min={0}
                    max={2}
                    step={0.1}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('dryMultiplier', sampling.dryMultiplier, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="dry_multiplier" after={isChangedFrom('dryMultiplier', sampling.dryMultiplier, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('dryMultiplier', modelBaseline)}`}>●</span>
                    )}>
                    DRY Multiplier
                  </FieldLabel>
                  <input
                    type="number"
                    value={sampling.dryMultiplier}
                    onChange={(e) => setSampling({ dryMultiplier: Number(e.target.value) })}
                    min={0}
                    step={0.1}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('dryBase', sampling.dryBase, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="dry_base" after={isChangedFrom('dryBase', sampling.dryBase, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('dryBase', modelBaseline)}`}>●</span>
                    )}>
                    DRY Base
                  </FieldLabel>
                  <input
                    type="number"
                    value={sampling.dryBase}
                    onChange={(e) => setSampling({ dryBase: Number(e.target.value) })}
                    min={1}
                    step={0.05}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('dryAllowedLen', sampling.dryAllowedLen, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="dry_allowed_length" after={isChangedFrom('dryAllowedLen', sampling.dryAllowedLen, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('dryAllowedLen', modelBaseline)}`}>●</span>
                    )}>
                    DRY Allowed Length
                  </FieldLabel>
                  <input
                    type="number"
                    value={sampling.dryAllowedLen}
                    onChange={(e) => setSampling({ dryAllowedLen: Number(e.target.value) })}
                    min={0}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('dryPenaltyLast', sampling.dryPenaltyLast, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="dry_penalty_last_n" after={isChangedFrom('dryPenaltyLast', sampling.dryPenaltyLast, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('dryPenaltyLast', modelBaseline)}`}>●</span>
                    )}>
                    DRY Penalty Last N
                  </FieldLabel>
                  <input
                    type="number"
                    value={sampling.dryPenaltyLast}
                    onChange={(e) => setSampling({ dryPenaltyLast: Number(e.target.value) })}
                    min={-1}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('xtcProbability', sampling.xtcProbability, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="xtc_probability" after={isChangedFrom('xtcProbability', sampling.xtcProbability, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('xtcProbability', modelBaseline)}`}>●</span>
                    )}>
                    XTC Probability
                  </FieldLabel>
                  <input
                    type="number"
                    value={sampling.xtcProbability}
                    onChange={(e) => setSampling({ xtcProbability: Number(e.target.value) })}
                    min={0}
                    max={1}
                    step={0.01}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('xtcThreshold', sampling.xtcThreshold, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="xtc_threshold" after={isChangedFrom('xtcThreshold', sampling.xtcThreshold, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('xtcThreshold', modelBaseline)}`}>●</span>
                    )}>
                    XTC Threshold
                  </FieldLabel>
                  <input
                    type="number"
                    value={sampling.xtcThreshold}
                    onChange={(e) => setSampling({ xtcThreshold: Number(e.target.value) })}
                    min={0}
                    max={1}
                    step={0.01}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('xtcMinKeep', sampling.xtcMinKeep, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="xtc_min_keep" after={isChangedFrom('xtcMinKeep', sampling.xtcMinKeep, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('xtcMinKeep', modelBaseline)}`}>●</span>
                    )}>
                    XTC Min Keep
                  </FieldLabel>
                  <input
                    type="number"
                    value={sampling.xtcMinKeep}
                    onChange={(e) => setSampling({ xtcMinKeep: Number(e.target.value) })}
                    min={1}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('enableThinking', sampling.enableThinking, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="enable_thinking" after={isChangedFrom('enableThinking', sampling.enableThinking, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('enableThinking', modelBaseline)}`}>●</span>
                    )}>
                    Enable Thinking
                  </FieldLabel>
                  <select
                    value={sampling.enableThinking}
                    onChange={(e) => setSampling({ enableThinking: e.target.value })}
                  >
                    <option value="">Default</option>
                    <option value="true">Enabled</option>
                    <option value="false">Disabled</option>
                  </select>
                </div>
                <div className={`chat-setting ${isChangedFrom('reasoningEffort', sampling.reasoningEffort, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <FieldLabel tooltipKey="reasoning_effort" after={isChangedFrom('reasoningEffort', sampling.reasoningEffort, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('reasoningEffort', modelBaseline)}`}>●</span>
                    )}>
                    Reasoning Effort
                  </FieldLabel>
                  <select
                    value={sampling.reasoningEffort}
                    onChange={(e) => setSampling({ reasoningEffort: e.target.value })}
                  >
                    <option value="">Default</option>
                    <option value="low">Low</option>
                    <option value="medium">Medium</option>
                    <option value="high">High</option>
                  </select>
                </div>
                <div className={`chat-setting ${isChangedFrom('topLogprobs', sampling.topLogprobs, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <label>
                    Top Logprobs
                    {isChangedFrom('topLogprobs', sampling.topLogprobs, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('topLogprobs', modelBaseline)}`}>●</span>
                    )}
                  </label>
                  <input
                    type="number"
                    value={sampling.topLogprobs}
                    onChange={(e) => setSampling({ topLogprobs: Number(e.target.value) })}
                    min={0}
                    max={20}
                  />
                </div>
                <div className={`chat-setting chat-setting-checkbox ${isChangedFrom('returnPrompt', sampling.returnPrompt, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <label>
                    <input
                      type="checkbox"
                      checked={sampling.returnPrompt}
                      onChange={(e) => setSampling({ returnPrompt: e.target.checked })}
                    />
                    Return Prompt
                    {isChangedFrom('returnPrompt', sampling.returnPrompt, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('returnPrompt', modelBaseline)}`}>●</span>
                    )}
                  </label>
                </div>
                <div className={`chat-setting chat-setting-checkbox ${isChangedFrom('includeUsage', sampling.includeUsage, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <label>
                    <input
                      type="checkbox"
                      checked={sampling.includeUsage}
                      onChange={(e) => setSampling({ includeUsage: e.target.checked })}
                    />
                    Include Usage
                    {isChangedFrom('includeUsage', sampling.includeUsage, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('includeUsage', modelBaseline)}`}>●</span>
                    )}
                  </label>
                </div>
                <div className={`chat-setting chat-setting-checkbox ${isChangedFrom('logprobs', sampling.logprobs, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <label>
                    <input
                      type="checkbox"
                      checked={sampling.logprobs}
                      onChange={(e) => setSampling({ logprobs: e.target.checked })}
                    />
                    Logprobs
                    {isChangedFrom('logprobs', sampling.logprobs, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('logprobs', modelBaseline)}`}>●</span>
                    )}
                  </label>
                </div>
            </div>
          </div>
        )}
        </>
      )}

      {error && <div className="alert alert-error">{error}</div>}

      <div className="chat-messages-wrapper">
      <div className="chat-messages" ref={messagesContainerRef} onScroll={handleMessagesScroll} onWheel={handleWheel}>
        {messages.length === 0 && (
          <div className="chat-empty">
            <p>{disabledPlaceholder || 'Select a model and start chatting'}</p>
            <p className="chat-empty-hint">Type a message below to begin</p>
          </div>
        )}
        {messages.map((msg, idx) => (
          <div key={idx} className={`chat-message chat-message-${msg.role}`}>
            <div className="chat-message-header">
              <span>{msg.role === 'user' ? 'USER' : 'MODEL'}</span>
              {!isStreaming && editingIndex === null && (
                <span className="chat-header-actions">
                  {msg.originalContent != null && (
                    <button
                      className="chat-edit-btn"
                      onClick={() => handleEditUndo(idx)}
                      title="Revert to original message"
                    >
                      Undo
                    </button>
                  )}
                  <button
                    className="chat-edit-btn"
                    onClick={() => handleEditStart(idx)}
                    title="Edit message"
                  >
                    Edit
                  </button>
                </span>
              )}
            </div>
            {msg.attachments && msg.attachments.length > 0 && (
              <div className="chat-message-attachments">
                {msg.attachments.map((att, i) => (
                  <div key={i} className="chat-attachment-preview">
                    {att.type === 'image' ? (
                      <img src={att.dataUrl} alt={att.name} className="chat-attachment-image" />
                    ) : (
                      <div className="chat-attachment-audio">
                        <span>🔊 {att.name}</span>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
            {msg.reasoning && (
              <details className="chat-message-reasoning" open={isStreaming && idx === messages.length - 1}>
                <summary className="chat-reasoning-summary">
                  Reasoning ({msg.reasoning.length.toLocaleString()} chars)
                </summary>
                <div className="chat-reasoning-content">
                  {renderContent(msg.reasoning)}
                </div>
              </details>
            )}
            <div className="chat-message-content">
              {editingIndex === idx ? (
                <div className="chat-edit-area">
                  <textarea
                    className="chat-edit-textarea"
                    value={editContent}
                    onChange={(e) => setEditContent(e.target.value)}
                    rows={Math.max(4, editContent.split('\n').length)}
                    autoFocus
                  />
                  <div className="chat-edit-actions">
                    <button
                      className="btn btn-primary btn-sm"
                      onClick={() => msg.role === 'assistant' ? handleEditSave(idx) : handleEditSend(idx)}
                      disabled={!editContent.trim()}
                    >
                      {msg.role === 'assistant' ? 'Save' : 'Send'}
                    </button>
                    <button
                      className="btn btn-secondary btn-sm"
                      onClick={handleEditCancel}
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              ) : (
                msg.content ? renderContent(msg.content) : (isStreaming && idx === messages.length - 1 ? '...' : '')
              )}
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
                {(msg.usage.draft_tokens ?? 0) > 0 && (
                  <> | Draft Model Acceptance Rate: {((msg.usage.draft_acceptance_rate ?? (msg.usage.draft_accepted_tokens ?? 0) / (msg.usage.draft_tokens ?? 1)) * 100).toFixed(2)}%</>
                )}
                {(msg.usage.time_to_first_token_ms ?? 0) > 0 && (
                  <> | TTFT: {msg.usage.time_to_first_token_ms!.toFixed(0)}ms</>
                )}
              </div>
            )}
          </div>
        ))}
        <div ref={messagesEndRef} />
      </div>
      {showScrollBtn && (
        <button className="chat-scroll-bottom-btn" onClick={scrollToBottom} title="Scroll to bottom">
          ↓
        </button>
      )}
      </div>

      <form onSubmit={handleSubmit} className="chat-input-form">
        <div className="chat-input-row">
          {attachedFiles.length > 0 && (
            <div className="chat-attachments-bar">
              {attachedFiles.map((file, idx) => (
                <div key={idx} className="chat-attachment-chip">
                  {file.type === 'image' ? (
                    <img src={file.dataUrl} alt={file.name} className="chat-attachment-chip-image" />
                  ) : (
                    <span className="chat-attachment-chip-audio">🔊</span>
                  )}
                  <span className="chat-attachment-chip-name">{file.name}</span>
                  <button
                    type="button"
                    className="chat-attachment-chip-remove"
                    onClick={() => removeAttachment(idx)}
                  >
                    ×
                  </button>
                </div>
              ))}
            </div>
          )}
          <textarea
            ref={inputRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={disabled ? (disabledPlaceholder || 'Chat is disabled') : 'Type your message... (Enter to send, Shift+Enter for new line)'}
            disabled={isStreaming || disabled}
            className="chat-input"
            rows={3}
          />
          <div className="chat-input-buttons">
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*,audio/*"
              multiple
              onChange={handleFileSelect}
              style={{ display: 'none' }}
            />
            <button
              type="button"
              className="btn btn-secondary chat-attach-btn"
              onClick={() => fileInputRef.current?.click()}
              disabled={isStreaming || disabled}
            >
              📎
            </button>
            {isStreaming ? (
              <button type="button" className="btn btn-danger" onClick={handleStop}>
                Stop
              </button>
            ) : (
              <button
                type="submit"
                className="btn btn-primary"
                disabled={(!input.trim() && attachedFiles.length === 0) || disabled}
              >
                Send
              </button>
            )}
          </div>
        </div>
      </form>
    </div>
  );
}
