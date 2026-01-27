import { useState, useEffect, useRef, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import { useChatMessages, type DisplayMessage } from '../contexts/ChatContext';
import { useSampling, defaultSampling, isChangedFrom, formatBaselineValue, hasAnyChange, hasAdvancedChange, type SamplingParams } from '../contexts/SamplingContext';
import CodeBlock from './CodeBlock';
import type { ChatMessage, ChatUsage, ChatToolCall, ChatContentPart, SamplingConfig, ListModelDetail } from '../types';

interface AttachedFile {
  type: 'image' | 'audio';
  name: string;
  dataUrl: string; // data:mime;base64,...
}

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
  const { sampling, setSampling } = useSampling();
  const [selectedModel, setSelectedModel] = useState<string>(() => {
    return localStorage.getItem('kronk_chat_model') || '';
  });
  const [input, setInput] = useState('');
  const [isStreaming, setIsStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [attachedFiles, setAttachedFiles] = useState<AttachedFile[]>([]);
  const [showAdvanced, setShowAdvanced] = useState(false);

  // Extended model configs with sampling parameters
  const [extendedModels, setExtendedModels] = useState<ListModelDetail[]>([]);
  
  // Baseline sampling config from the selected model's /models endpoint
  const [modelBaseline, setModelBaseline] = useState<SamplingParams | null>(null);
  
  // Track previous model to only apply config on actual model change
  const prevModelRef = useRef<string | null>(null);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const abortRef = useRef<(() => void) | null>(null);

  // Convert API sampling config to SamplingParams
  const toSamplingParams = useCallback((modelSampling: SamplingConfig): SamplingParams => {
    return {
      temperature: modelSampling.temperature || defaultSampling.temperature,
      topK: modelSampling.top_k || defaultSampling.topK,
      topP: modelSampling.top_p || defaultSampling.topP,
      minP: modelSampling.min_p || defaultSampling.minP,
      maxTokens: modelSampling.max_tokens || defaultSampling.maxTokens,
      repeatPenalty: modelSampling.repeat_penalty || defaultSampling.repeatPenalty,
      repeatLastN: modelSampling.repeat_last_n || defaultSampling.repeatLastN,
      dryMultiplier: modelSampling.dry_multiplier || defaultSampling.dryMultiplier,
      dryBase: modelSampling.dry_base || defaultSampling.dryBase,
      dryAllowedLen: modelSampling.dry_allowed_length || defaultSampling.dryAllowedLen,
      dryPenaltyLast: modelSampling.dry_penalty_last_n ?? defaultSampling.dryPenaltyLast,
      xtcProbability: modelSampling.xtc_probability ?? defaultSampling.xtcProbability,
      xtcThreshold: modelSampling.xtc_threshold || defaultSampling.xtcThreshold,
      xtcMinKeep: modelSampling.xtc_min_keep || defaultSampling.xtcMinKeep,
      enableThinking: modelSampling.enable_thinking || defaultSampling.enableThinking,
      reasoningEffort: modelSampling.reasoning_effort || defaultSampling.reasoningEffort,
      returnPrompt: defaultSampling.returnPrompt,
      includeUsage: defaultSampling.includeUsage,
      logprobs: defaultSampling.logprobs,
      topLogprobs: defaultSampling.topLogprobs,
    };
  }, []);

  // Apply sampling config for a model and set baseline for comparison
  const applySamplingConfig = useCallback((modelSampling: SamplingConfig | undefined) => {
    if (modelSampling) {
      const params = toSamplingParams(modelSampling);
      setSampling(params);
      setModelBaseline(params);
    } else {
      setModelBaseline(null);
    }
  }, [setSampling, toSamplingParams]);

  useEffect(() => {
    loadModels();
    // Also fetch extended models for sampling configs
    api.listModelsExtended()
      .then((response) => {
        if (response?.data) {
          setExtendedModels(response.data);
        }
      })
      .catch(() => {
        // Ignore errors, fall back to defaults
      });
  }, [loadModels]);

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

  // Save selected model to localStorage and apply sampling config only on model change
  useEffect(() => {
    if (selectedModel) {
      localStorage.setItem('kronk_chat_model', selectedModel);
      // Only apply model sampling config when the user actually changes models
      if (prevModelRef.current !== null && prevModelRef.current !== selectedModel) {
        const modelDetail = extendedModels.find((m) => m.id === selectedModel);
        applySamplingConfig(modelDetail?.sampling);
      }
      prevModelRef.current = selectedModel;
    }
  }, [selectedModel, extendedModels, applySamplingConfig]);

  // Set baseline for initial model load (without overwriting user's sampling values)
  useEffect(() => {
    if (selectedModel && extendedModels.length > 0 && modelBaseline === null) {
      const modelDetail = extendedModels.find((m) => m.id === selectedModel);
      if (modelDetail?.sampling) {
        setModelBaseline(toSamplingParams(modelDetail.sampling));
      }
    }
  }, [selectedModel, extendedModels, modelBaseline, toSamplingParams]);

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
        max_tokens: sampling.maxTokens,
        temperature: sampling.temperature,
        top_p: sampling.topP,
        top_k: sampling.topK,
        min_p: sampling.minP,
        repeat_penalty: sampling.repeatPenalty,
        repeat_last_n: sampling.repeatLastN,
        dry_multiplier: sampling.dryMultiplier,
        dry_base: sampling.dryBase,
        dry_allowed_length: sampling.dryAllowedLen,
        dry_penalty_last_n: sampling.dryPenaltyLast,
        xtc_probability: sampling.xtcProbability,
        xtc_threshold: sampling.xtcThreshold,
        xtc_min_keep: sampling.xtcMinKeep,
        enable_thinking: sampling.enableThinking || undefined,
        reasoning_effort: sampling.reasoningEffort || undefined,
        return_prompt: sampling.returnPrompt,
        stream_options: {
          include_usage: sampling.includeUsage,
        },
        logprobs: sampling.logprobs,
        top_logprobs: sampling.topLogprobs,
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
            onChange={(e) => setSelectedModel(e.target.value)}
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
              .map((model) => (
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
        <div className="chat-settings">
          <div className={`chat-setting ${isChangedFrom('maxTokens', sampling.maxTokens, modelBaseline) ? 'chat-setting-changed' : ''}`}>
            <label>
              Max Tokens
              {isChangedFrom('maxTokens', sampling.maxTokens, modelBaseline) && (
                <span className="chat-setting-default" title={`Default: ${formatBaselineValue('maxTokens', modelBaseline)}`}>●</span>
              )}
            </label>
            <input
              type="number"
              value={sampling.maxTokens}
              onChange={(e) => setSampling({ maxTokens: Number(e.target.value) })}
              min={1}
              max={32768}
            />
          </div>
          <div className={`chat-setting ${isChangedFrom('temperature', sampling.temperature, modelBaseline) ? 'chat-setting-changed' : ''}`}>
            <label>
              Temperature
              {isChangedFrom('temperature', sampling.temperature, modelBaseline) && (
                <span className="chat-setting-default" title={`Default: ${formatBaselineValue('temperature', modelBaseline)}`}>●</span>
              )}
            </label>
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
            <label>
              Top P
              {isChangedFrom('topP', sampling.topP, modelBaseline) && (
                <span className="chat-setting-default" title={`Default: ${formatBaselineValue('topP', modelBaseline)}`}>●</span>
              )}
            </label>
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
            <label>
              Top K
              {isChangedFrom('topK', sampling.topK, modelBaseline) && (
                <span className="chat-setting-default" title={`Default: ${formatBaselineValue('topK', modelBaseline)}`}>●</span>
              )}
            </label>
            <input
              type="number"
              value={sampling.topK}
              onChange={(e) => setSampling({ topK: Number(e.target.value) })}
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

          {showAdvanced && (
            <div className="chat-advanced-settings">
                <div className={`chat-setting ${isChangedFrom('minP', sampling.minP, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <label>
                    Min P
                    {isChangedFrom('minP', sampling.minP, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('minP', modelBaseline)}`}>●</span>
                    )}
                  </label>
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
                  <label>
                    Repeat Penalty
                    {isChangedFrom('repeatPenalty', sampling.repeatPenalty, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('repeatPenalty', modelBaseline)}`}>●</span>
                    )}
                  </label>
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
                  <label>
                    Repeat Last N
                    {isChangedFrom('repeatLastN', sampling.repeatLastN, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('repeatLastN', modelBaseline)}`}>●</span>
                    )}
                  </label>
                  <input
                    type="number"
                    value={sampling.repeatLastN}
                    onChange={(e) => setSampling({ repeatLastN: Number(e.target.value) })}
                    min={-1}
                    max={512}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('dryMultiplier', sampling.dryMultiplier, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <label>
                    DRY Multiplier
                    {isChangedFrom('dryMultiplier', sampling.dryMultiplier, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('dryMultiplier', modelBaseline)}`}>●</span>
                    )}
                  </label>
                  <input
                    type="number"
                    value={sampling.dryMultiplier}
                    onChange={(e) => setSampling({ dryMultiplier: Number(e.target.value) })}
                    min={0}
                    step={0.1}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('dryBase', sampling.dryBase, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <label>
                    DRY Base
                    {isChangedFrom('dryBase', sampling.dryBase, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('dryBase', modelBaseline)}`}>●</span>
                    )}
                  </label>
                  <input
                    type="number"
                    value={sampling.dryBase}
                    onChange={(e) => setSampling({ dryBase: Number(e.target.value) })}
                    min={1}
                    step={0.05}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('dryAllowedLen', sampling.dryAllowedLen, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <label>
                    DRY Allowed Length
                    {isChangedFrom('dryAllowedLen', sampling.dryAllowedLen, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('dryAllowedLen', modelBaseline)}`}>●</span>
                    )}
                  </label>
                  <input
                    type="number"
                    value={sampling.dryAllowedLen}
                    onChange={(e) => setSampling({ dryAllowedLen: Number(e.target.value) })}
                    min={0}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('dryPenaltyLast', sampling.dryPenaltyLast, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <label>
                    DRY Penalty Last N
                    {isChangedFrom('dryPenaltyLast', sampling.dryPenaltyLast, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('dryPenaltyLast', modelBaseline)}`}>●</span>
                    )}
                  </label>
                  <input
                    type="number"
                    value={sampling.dryPenaltyLast}
                    onChange={(e) => setSampling({ dryPenaltyLast: Number(e.target.value) })}
                    min={-1}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('xtcProbability', sampling.xtcProbability, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <label>
                    XTC Probability
                    {isChangedFrom('xtcProbability', sampling.xtcProbability, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('xtcProbability', modelBaseline)}`}>●</span>
                    )}
                  </label>
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
                  <label>
                    XTC Threshold
                    {isChangedFrom('xtcThreshold', sampling.xtcThreshold, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('xtcThreshold', modelBaseline)}`}>●</span>
                    )}
                  </label>
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
                  <label>
                    XTC Min Keep
                    {isChangedFrom('xtcMinKeep', sampling.xtcMinKeep, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('xtcMinKeep', modelBaseline)}`}>●</span>
                    )}
                  </label>
                  <input
                    type="number"
                    value={sampling.xtcMinKeep}
                    onChange={(e) => setSampling({ xtcMinKeep: Number(e.target.value) })}
                    min={1}
                  />
                </div>
                <div className={`chat-setting ${isChangedFrom('enableThinking', sampling.enableThinking, modelBaseline) ? 'chat-setting-changed' : ''}`}>
                  <label>
                    Enable Thinking
                    {isChangedFrom('enableThinking', sampling.enableThinking, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('enableThinking', modelBaseline)}`}>●</span>
                    )}
                  </label>
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
                  <label>
                    Reasoning Effort
                    {isChangedFrom('reasoningEffort', sampling.reasoningEffort, modelBaseline) && (
                      <span className="chat-setting-default" title={`Default: ${formatBaselineValue('reasoningEffort', modelBaseline)}`}>●</span>
                    )}
                  </label>
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
