import { useState, useEffect, useRef, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import { useChatMessages } from '../contexts/ChatContext';
import CodeBlock from './CodeBlock';
import type { ChatMessage, ChatUsage, ChatToolCall, ChatContentPart, AttachedFile, DisplayMessage, ListModelDetail } from '../types';

// Pre-process content to handle in-progress code blocks during streaming
function preprocessContent(content: string): string {
  // Check if there's an unclosed code block at the end
  const openFences = (content.match(/```/g) || []).length;
  if (openFences % 2 !== 0) {
    // Odd number of ```, meaning there's an unclosed block - close it for rendering
    return content + '\n```';
  }
  return content;
}

// Custom components for ReactMarkdown
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
  // Style other markdown elements
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

export default function Chat() {
  const { models, loading: modelsLoading, loadModels } = useModelList();
  const { messages, setMessages, clearMessages } = useChatMessages();
  const [selectedModel, setSelectedModel] = useState<string>(() => {
    return localStorage.getItem('kronk_chat_model') || '';
  });
  const [input, setInput] = useState('');
  const [isStreaming, setIsStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [extendedModels, setExtendedModels] = useState<ListModelDetail[]>([]);
  
  const loadSetting = <T,>(key: string, defaultValue: T): T => {
    const stored = localStorage.getItem(`kronk_chat_${key}`);
    if (stored === null) return defaultValue;
    try {
      return JSON.parse(stored) as T;
    } catch {
      return defaultValue;
    }
  };

  const [maxTokens, setMaxTokens] = useState(() => loadSetting('maxTokens', 2048));
  const [temperature, setTemperature] = useState(() => loadSetting('temperature', 0.8));
  const [topP, setTopP] = useState(() => loadSetting('topP', 0.9));
  const [topK, setTopK] = useState(() => loadSetting('topK', 40));
  const [attachedFiles, setAttachedFiles] = useState<AttachedFile[]>([]);

  const [showAdvanced, setShowAdvanced] = useState(false);
  const [minP, setMinP] = useState(() => loadSetting('minP', 0));
  const [repeatPenalty, setRepeatPenalty] = useState(() => loadSetting('repeatPenalty', 1.1));
  const [repeatLastN, setRepeatLastN] = useState(() => loadSetting('repeatLastN', 64));
  const [dryMultiplier, setDryMultiplier] = useState(() => loadSetting('dryMultiplier', 0));
  const [dryBase, setDryBase] = useState(() => loadSetting('dryBase', 1.75));
  const [dryAllowedLen, setDryAllowedLen] = useState(() => loadSetting('dryAllowedLen', 2));
  const [dryPenaltyLast, setDryPenaltyLast] = useState(() => loadSetting('dryPenaltyLast', 0));
  const [xtcProbability, setXtcProbability] = useState(() => loadSetting('xtcProbability', 0));
  const [xtcThreshold, setXtcThreshold] = useState(() => loadSetting('xtcThreshold', 0.1));
  const [xtcMinKeep, setXtcMinKeep] = useState(() => loadSetting('xtcMinKeep', 1));
  const [enableThinking, setEnableThinking] = useState(() => loadSetting('enableThinking', ''));
  const [reasoningEffort, setReasoningEffort] = useState(() => loadSetting('reasoningEffort', ''));
  const [returnPrompt, setReturnPrompt] = useState(() => loadSetting('returnPrompt', false));
  const [includeUsage, setIncludeUsage] = useState(() => loadSetting('includeUsage', true));
  const [logprobs, setLogprobs] = useState(() => loadSetting('logprobs', false));
  const [topLogprobs, setTopLogprobs] = useState(() => loadSetting('topLogprobs', 0));

  useEffect(() => {
    const settings = {
      maxTokens, temperature, topP, topK, minP, repeatPenalty, repeatLastN,
      dryMultiplier, dryBase, dryAllowedLen, dryPenaltyLast,
      xtcProbability, xtcThreshold, xtcMinKeep,
      enableThinking, reasoningEffort, returnPrompt, includeUsage, logprobs, topLogprobs,
    };
    for (const [key, value] of Object.entries(settings)) {
      localStorage.setItem(`kronk_chat_${key}`, JSON.stringify(value));
    }
  }, [
    maxTokens, temperature, topP, topK, minP, repeatPenalty, repeatLastN,
    dryMultiplier, dryBase, dryAllowedLen, dryPenaltyLast,
    xtcProbability, xtcThreshold, xtcMinKeep,
    enableThinking, reasoningEffort, returnPrompt, includeUsage, logprobs, topLogprobs,
  ]);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const abortRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    loadModels();
  }, [loadModels]);

  // Load models with extended config for sampling defaults
  useEffect(() => {
    const loadExtendedModels = async () => {
      try {
        const result = await api.listModelsWithConfig();
        setExtendedModels(result.data || []);
      } catch {
        // Silently fail - we'll use defaults
      }
    };
    loadExtendedModels();
  }, []);

  // Apply model defaults when model selection changes
  const applyModelDefaults = useCallback((modelId: string) => {
    const model = extendedModels.find(m => m.id === modelId);
    if (!model?.extended_config?.sampling) return;

    const s = model.extended_config.sampling;
    if (s.temperature !== undefined) setTemperature(s.temperature);
    if (s.top_k !== undefined) setTopK(s.top_k);
    if (s.top_p !== undefined) setTopP(s.top_p);
    if (s.min_p !== undefined) setMinP(s.min_p);
    if (s.repeat_penalty !== undefined) setRepeatPenalty(s.repeat_penalty);
    if (s.repeat_last_n !== undefined) setRepeatLastN(s.repeat_last_n);
    if (s.dry_multiplier !== undefined) setDryMultiplier(s.dry_multiplier);
    if (s.dry_base !== undefined) setDryBase(s.dry_base);
    if (s.dry_allowed_length !== undefined) setDryAllowedLen(s.dry_allowed_length);
    if (s.dry_penalty_last_n !== undefined) setDryPenaltyLast(s.dry_penalty_last_n);
    if (s.xtc_probability !== undefined) setXtcProbability(s.xtc_probability);
    if (s.xtc_threshold !== undefined) setXtcThreshold(s.xtc_threshold);
    if (s.xtc_min_keep !== undefined) setXtcMinKeep(s.xtc_min_keep);

    // Also apply max_tokens from context_window if available
    if (model.extended_config.context_window) {
      setMaxTokens(Math.min(model.extended_config.context_window, maxTokens || 2048));
    }
  }, [extendedModels, maxTokens]);

  // Apply defaults when extendedModels loads (handles initial page load race condition)
  useEffect(() => {
    if (selectedModel && extendedModels.length > 0) {
      applyModelDefaults(selectedModel);
    }
  }, [extendedModels, selectedModel, applyModelDefaults]);

  useEffect(() => {
    if (models?.data && models.data.length > 0) {
      const chatModels = models.data.filter((m) => {
        const id = m.id.toLowerCase();
        return !id.includes('embed') && !id.includes('rerank');
      });
      // Check if current selection is valid, otherwise pick first available
      const isCurrentValid = chatModels.some((m) => m.id === selectedModel);
      if (!isCurrentValid && chatModels.length > 0) {
        setSelectedModel(chatModels[0].id);
      }
    }
  }, [models, selectedModel]);

  // Save selected model to localStorage and apply model defaults
  useEffect(() => {
    if (selectedModel) {
      localStorage.setItem('kronk_chat_model', selectedModel);
      applyModelDefaults(selectedModel);
    }
  }, [selectedModel, applyModelDefaults]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  // Focus input when streaming ends
  useEffect(() => {
    if (!isStreaming && selectedModel) {
      inputRef.current?.focus();
    }
  }, [isStreaming, selectedModel]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if ((!input.trim() && attachedFiles.length === 0) || !selectedModel || isStreaming) return;

    const userMessage: DisplayMessage = { 
      role: 'user', 
      content: input.trim(),
      attachments: attachedFiles.length > 0 ? [...attachedFiles] : undefined,
    };
    setMessages(prev => [...prev, userMessage]);
    setInput('');
    setAttachedFiles([]);
    setError(null);
    setIsStreaming(true);

    // Build content for the new message with attachments
    const buildMessageContent = (text: string, files: AttachedFile[]): string | ChatContentPart[] => {
      if (files.length === 0) {
        return text;
      }
      
      const parts: ChatContentPart[] = [];
      
      // Add text part if present
      if (text) {
        parts.push({ type: 'text', text });
      }
      
      // Add file parts
      for (const file of files) {
        if (file.type === 'image') {
          parts.push({
            type: 'image_url',
            image_url: { url: file.dataUrl },
          });
        } else if (file.type === 'audio') {
          // Extract base64 data and format from data URL
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
    };

    const chatMessages: ChatMessage[] = [
      ...messages.map(m => ({ 
        role: m.role, 
        content: m.attachments ? buildMessageContent(m.content, m.attachments) : m.content 
      })),
      { role: 'user' as const, content: buildMessageContent(input.trim(), attachedFiles) }
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
        min_p: minP,
        repeat_penalty: repeatPenalty,
        repeat_last_n: repeatLastN,
        dry_multiplier: dryMultiplier,
        dry_base: dryBase,
        dry_allowed_length: dryAllowedLen,
        dry_penalty_last_n: dryPenaltyLast,
        xtc_probability: xtcProbability,
        xtc_threshold: xtcThreshold,
        xtc_min_keep: xtcMinKeep,
        enable_thinking: enableThinking || undefined,
        reasoning_effort: reasoningEffort || undefined,
        return_prompt: returnPrompt,
        stream_options: {
          include_usage: includeUsage,
        },
        logprobs,
        top_logprobs: topLogprobs,
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
    clearMessages();
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
        setAttachedFiles(prev => [...prev, {
          type: fileType,
          name: file.name,
          dataUrl,
        }]);
      };
      reader.readAsDataURL(file);
    });

    // Reset input so same file can be selected again
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

  return (
    <div className="chat-container">
      <div className="chat-header">
        <div className="chat-header-left">
          <h2>Run</h2>
          <select
            value={selectedModel}
            onChange={(e) => {
              const newModel = e.target.value;
              setSelectedModel(newModel);
              applyModelDefaults(newModel);
            }}
            disabled={modelsLoading || isStreaming}
            className="chat-model-select"
          >
            {modelsLoading && <option>Loading models...</option>}
            {!modelsLoading && models?.data?.length === 0 && (
              <option>No models available</option>
            )}
            {models?.data
              ?.filter((model) => {
                const id = model.id.toLowerCase();
                return !id.includes('embed') && !id.includes('rerank');
              })
              .map((model) => {
                let displayName = model.id;
                if (displayName.endsWith('/SPC')) {
                  displayName = displayName.slice(0, -4) + ' - with System Prompt Cache';
                } else if (displayName.endsWith('/FMC')) {
                  displayName = displayName.slice(0, -4) + ' - with First Message Cache';
                }
                return (
                  <option key={model.id} value={model.id}>
                    {displayName}
                  </option>
                );
              })}
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

          <div className="chat-setting chat-setting-button">
            <label>&nbsp;</label>
            <button
              type="button"
              className="chat-advanced-toggle"
              onClick={() => setShowAdvanced(!showAdvanced)}
            >
              Advanced {showAdvanced ? '▲' : '▼'}
            </button>
          </div>

          {showAdvanced && (
            <div className="chat-advanced-settings">
                <div className="chat-setting">
                  <label>Min P</label>
                  <input
                    type="number"
                    value={minP}
                    onChange={(e) => setMinP(Number(e.target.value))}
                    min={0}
                    max={1}
                    step={0.01}
                  />
                </div>
                <div className="chat-setting">
                  <label>Repeat Penalty</label>
                  <input
                    type="number"
                    value={repeatPenalty}
                    onChange={(e) => setRepeatPenalty(Number(e.target.value))}
                    min={0}
                    max={2}
                    step={0.1}
                  />
                </div>
                <div className="chat-setting">
                  <label>Repeat Last N</label>
                  <input
                    type="number"
                    value={repeatLastN}
                    onChange={(e) => setRepeatLastN(Number(e.target.value))}
                    min={-1}
                    max={512}
                  />
                </div>
                <div className="chat-setting">
                  <label>DRY Multiplier</label>
                  <input
                    type="number"
                    value={dryMultiplier}
                    onChange={(e) => setDryMultiplier(Number(e.target.value))}
                    min={0}
                    step={0.1}
                  />
                </div>
                <div className="chat-setting">
                  <label>DRY Base</label>
                  <input
                    type="number"
                    value={dryBase}
                    onChange={(e) => setDryBase(Number(e.target.value))}
                    min={1}
                    step={0.05}
                  />
                </div>
                <div className="chat-setting">
                  <label>DRY Allowed Length</label>
                  <input
                    type="number"
                    value={dryAllowedLen}
                    onChange={(e) => setDryAllowedLen(Number(e.target.value))}
                    min={0}
                  />
                </div>
                <div className="chat-setting">
                  <label>DRY Penalty Last N</label>
                  <input
                    type="number"
                    value={dryPenaltyLast}
                    onChange={(e) => setDryPenaltyLast(Number(e.target.value))}
                    min={-1}
                  />
                </div>
                <div className="chat-setting">
                  <label>XTC Probability</label>
                  <input
                    type="number"
                    value={xtcProbability}
                    onChange={(e) => setXtcProbability(Number(e.target.value))}
                    min={0}
                    max={1}
                    step={0.01}
                  />
                </div>
                <div className="chat-setting">
                  <label>XTC Threshold</label>
                  <input
                    type="number"
                    value={xtcThreshold}
                    onChange={(e) => setXtcThreshold(Number(e.target.value))}
                    min={0}
                    max={1}
                    step={0.01}
                  />
                </div>
                <div className="chat-setting">
                  <label>XTC Min Keep</label>
                  <input
                    type="number"
                    value={xtcMinKeep}
                    onChange={(e) => setXtcMinKeep(Number(e.target.value))}
                    min={1}
                  />
                </div>
                <div className="chat-setting">
                  <label>Enable Thinking</label>
                  <select
                    value={enableThinking}
                    onChange={(e) => setEnableThinking(e.target.value)}
                  >
                    <option value="">Default</option>
                    <option value="true">Enabled</option>
                    <option value="false">Disabled</option>
                  </select>
                </div>
                <div className="chat-setting">
                  <label>Reasoning Effort</label>
                  <select
                    value={reasoningEffort}
                    onChange={(e) => setReasoningEffort(e.target.value)}
                  >
                    <option value="">Default</option>
                    <option value="low">Low</option>
                    <option value="medium">Medium</option>
                    <option value="high">High</option>
                  </select>
                </div>
                <div className="chat-setting">
                  <label>Top Logprobs</label>
                  <input
                    type="number"
                    value={topLogprobs}
                    onChange={(e) => setTopLogprobs(Number(e.target.value))}
                    min={0}
                    max={20}
                  />
                </div>
                <div className="chat-setting chat-setting-checkbox">
                  <label>
                    <input
                      type="checkbox"
                      checked={returnPrompt}
                      onChange={(e) => setReturnPrompt(e.target.checked)}
                    />
                    Return Prompt
                  </label>
                </div>
                <div className="chat-setting chat-setting-checkbox">
                  <label>
                    <input
                      type="checkbox"
                      checked={includeUsage}
                      onChange={(e) => setIncludeUsage(e.target.checked)}
                    />
                    Include Usage
                  </label>
                </div>
                <div className="chat-setting chat-setting-checkbox">
                  <label>
                    <input
                      type="checkbox"
                      checked={logprobs}
                      onChange={(e) => setLogprobs(e.target.checked)}
                    />
                    Logprobs
                  </label>
                </div>
            </div>
          )}
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
            placeholder="Type your message... (Enter to send, Shift+Enter for new line)"
            disabled={isStreaming || !selectedModel}
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
              disabled={isStreaming || !selectedModel}
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
                disabled={(!input.trim() && attachedFiles.length === 0) || !selectedModel}
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
