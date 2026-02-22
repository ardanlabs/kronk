import { useState, useEffect, useRef, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import { useDownload } from '../contexts/DownloadContext';
import { usePlayground } from '../contexts/PlaygroundContext';
import type {
  PlaygroundTemplateInfo,
  ChatMessage,
  ChatStreamResponse,
  ChatToolCall,
  ModelConfig,
} from '../types';
import AutomatedTestingPanel from './AutomatedTestingPanel';
import { autoTestTools } from '../services/autoTestRunner';
import { PARAM_TOOLTIPS, ParamTooltip } from './ParamTooltips';

const NEW_MODEL_VALUE = '__new__';

const defaultTools = JSON.stringify(autoTestTools, null, 2);

export default function ModelPlayground() {
  const navigate = useNavigate();
  const { models, loadModels } = useModelList();
  const { download, isDownloading, startDownload, cancelDownload, clearDownload } = useDownload();
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const contentBufferRef = useRef('');
  const throttleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const messageKeyCounterRef = useRef(0);
  const messageKeysRef = useRef<number[]>([]);

  // Persistent state from context (survives navigation)
  const {
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
    cacheMode, setCacheMode,
    hydratedModelId, setHydratedModelId,
  } = usePlayground();

  // Local-only state (OK to reset on navigation)
  const [templates, setTemplates] = useState<PlaygroundTemplateInfo[]>([]);

  // Sampling parameters state
  const [temperature, setTemperature] = useState(0.8);
  const [topP, setTopP] = useState(0.9);
  const [topK, setTopK] = useState(40);
  const [minP, setMinP] = useState(0.0);
  const [maxTokens, setMaxTokens] = useState(4096);
  const [repeatPenalty, setRepeatPenalty] = useState(1.0);
  const [repeatLastN, setRepeatLastN] = useState(64);
  const [frequencyPenalty, setFrequencyPenalty] = useState(0.0);
  const [presencePenalty, setPresencePenalty] = useState(0.0);
  const [dryMultiplier, setDryMultiplier] = useState(1.05);
  const [dryBase, setDryBase] = useState(1.75);
  const [dryAllowedLength, setDryAllowedLength] = useState(2);
  const [dryPenaltyLastN, setDryPenaltyLastN] = useState(0);
  const [xtcProbability, setXtcProbability] = useState(0.0);
  const [xtcThreshold, setXtcThreshold] = useState(0.1);
  const [xtcMinKeep, setXtcMinKeep] = useState(1);
  const [enableThinking, setEnableThinking] = useState<'true' | 'false'>('true');
  const [reasoningEffort, setReasoningEffort] = useState<'none' | 'minimal' | 'low' | 'medium' | 'high'>('medium');

  // Catalog config state
  const [catalogConfig, setCatalogConfig] = useState<ModelConfig | null>(null);
  const [configLoading, setConfigLoading] = useState(false);

  // Session loading state
  const [sessionLoading, setSessionLoading] = useState(false);
  const [sessionError, setSessionError] = useState('');

  // Chat input state
  const [userInput, setUserInput] = useState('');
  const [streaming, setStreaming] = useState(false);
  const streamAbortRef = useRef<(() => void) | null>(null);
  const warmupAbortRef = useRef<(() => void) | null>(null);
  const toolTestAbortRef = useRef<(() => void) | null>(null);
  const sessionRef = useRef(session);
  sessionRef.current = session;

  // HuggingFace pull state
  const [showPullForm, setShowPullForm] = useState(false);
  const [hfModelUrl, setHfModelUrl] = useState('');
  const [hfProjUrl, setHfProjUrl] = useState('');
  const [showProjUrl, setShowProjUrl] = useState(false);
  const prePullModelIdsRef = useRef<Set<string>>(new Set());
  const pendingAutoSelectRef = useRef(false);
  const expectedFilenameRef = useRef('');

  // Tool test state
  const [toolDefs, setToolDefs] = useState(defaultTools);
  const [toolPrompt, setToolPrompt] = useState("What's the weather in Boston? Use the get_weather tool.");
  const [toolResult, setToolResult] = useState<string>('');
  const [toolCalls, setToolCalls] = useState<ChatToolCall[]>([]);
  const [toolTestRunning, setToolTestRunning] = useState(false);

  // Inspector state
  const [inspectorPrompt, setInspectorPrompt] = useState('Hello, how are you?');
  const [renderedPrompt, setRenderedPrompt] = useState('');
  const [inspectorRunning, setInspectorRunning] = useState(false);

  const loadTemplates = useCallback(async () => {
    try {
      const list = await api.listPlaygroundTemplates();
      setTemplates(list);
    } catch {
      // Templates may not be available yet
    }
  }, []);

  useEffect(() => {
    loadModels();
    loadTemplates();
  }, [loadModels, loadTemplates]);

  useEffect(() => {
    if (!selectedModel || selectedModel === NEW_MODEL_VALUE) {
      setCatalogConfig(null);
      return;
    }

    if (hydratedModelId === selectedModel) return;

    let cancelled = false;
    setConfigLoading(true);
    api.showModel(selectedModel)
      .then((info) => {
        if (cancelled) return;
        const mc = info.model_config;
        if (mc) {
          setCatalogConfig(mc);
          setContextWindow(mc['context-window'] || 8192);
          setNBatch(mc.nbatch || 2048);
          setNUBatch(mc.nubatch || 512);
          setNSeqMax(mc['nseq-max'] || 1);
          setFlashAttention(mc['flash-attention'] || 'enabled');
          setCacheType(mc['cache-type-k'] || mc['cache-type-v'] || '');
          setCacheMode(mc['incremental-cache'] ? 'imc' : mc['system-prompt-cache'] ? 'spc' : 'none');
        }
        setHydratedModelId(selectedModel);
      })
      .catch((err) => {
        if (!cancelled) setSessionError(err.message || 'Failed to load model config');
      })
      .finally(() => { if (!cancelled) setConfigLoading(false); });

    return () => { cancelled = true; };
  }, [selectedModel, hydratedModelId]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: streaming ? 'auto' : 'smooth' });
  }, [chatMessages, streaming]);

  useEffect(() => {
    return () => {
      streamAbortRef.current?.();
      warmupAbortRef.current?.();
      toolTestAbortRef.current?.();
      if (throttleTimerRef.current) {
        clearTimeout(throttleTimerRef.current);
      }
    };
  }, []);

  const handlePullModel = () => {
    const url = hfModelUrl.trim();
    if (!url || isDownloading || session) return;

    prePullModelIdsRef.current = new Set(models?.data?.map((m) => m.id) || []);
    expectedFilenameRef.current = url.split('/').pop() || '';
    pendingAutoSelectRef.current = true;
    startDownload(url, hfProjUrl.trim() || undefined);
  };

  useEffect(() => {
    if (!pendingAutoSelectRef.current) return;

    if (download?.status === 'error') {
      pendingAutoSelectRef.current = false;
      return;
    }

    if (download?.status !== 'complete') return;

    const before = prePullModelIdsRef.current;
    const all = models?.data ?? [];
    const added = all.filter((m) => !before.has(m.id));
    const filename = expectedFilenameRef.current;

    const chosen =
      added.find((m) => filename && m.id.includes(filename)) ??
      added.find((m) => !m.id.includes('mmproj') && !m.id.includes('proj')) ??
      added[0] ??
      all.find((m) => filename && m.id.includes(filename));

    pendingAutoSelectRef.current = false;

    if (chosen) {
      setSelectedModel(chosen.id);
      setShowPullForm(false);
      setHfModelUrl('');
      setHfProjUrl('');
      setShowProjUrl(false);
    }

    clearDownload();
  }, [models, download?.status]);

  const handleCreateSession = async () => {
    if (!selectedModel) return;

    if (nUBatch > nBatch) {
      setSessionError(`nubatch (${nUBatch}) must not exceed nbatch (${nBatch})`);
      return;
    }

    setSessionLoading(true);
    setSessionError('');

    try {
      // Build config with only user-changed values.
      const config: Record<string, any> = {};

      if (!catalogConfig || contextWindow !== (catalogConfig['context-window'] || 8192)) {
        config['context_window'] = contextWindow;
      }
      if (!catalogConfig || nBatch !== (catalogConfig.nbatch || 2048)) {
        config['nbatch'] = nBatch;
      }
      if (!catalogConfig || nUBatch !== (catalogConfig.nubatch || 512)) {
        config['nubatch'] = nUBatch;
      }
      if (!catalogConfig || nSeqMax !== (catalogConfig['nseq-max'] || 1)) {
        config['nseq_max'] = nSeqMax;
      }
      if (!catalogConfig || flashAttention !== (catalogConfig['flash-attention'] || 'enabled')) {
        config['flash_attention'] = flashAttention;
      }
      if (!catalogConfig || cacheType !== (catalogConfig['cache-type-k'] || '')) {
        if (cacheType) {
          config['cache_type_k'] = cacheType;
          config['cache_type_v'] = cacheType;
        }
      }
      const catalogCacheMode = catalogConfig?.['incremental-cache'] ? 'imc' : catalogConfig?.['system-prompt-cache'] ? 'spc' : 'none';
      if (!catalogConfig || cacheMode !== catalogCacheMode) {
        config['system_prompt_cache'] = cacheMode === 'spc';
        config['incremental_cache'] = cacheMode === 'imc';
      }

      const resp = await api.createPlaygroundSession({
        model_id: selectedModel,
        template_mode: templateMode,
        template_name: templateMode === 'builtin' ? selectedTemplate : undefined,
        template_script: templateMode === 'custom' ? customScript : undefined,
        config: config as any,
      });
      setSession(resp);
      setChatMessages([]);
    } catch (err: any) {
      setSessionError(err.message || 'Failed to create session');
    } finally {
      setSessionLoading(false);
    }
  };

  const handleUnloadSession = async () => {
    if (!session) return;

    // Abort any active streams first
    streamAbortRef.current?.();
    streamAbortRef.current = null;
    warmupAbortRef.current?.();
    warmupAbortRef.current = null;
    toolTestAbortRef.current?.();
    toolTestAbortRef.current = null;
    setToolTestRunning(false);
    if (throttleTimerRef.current) {
      clearTimeout(throttleTimerRef.current);
      throttleTimerRef.current = null;
    }
    setStreaming(false);

    try {
      await api.deletePlaygroundSession(session.session_id);
      setSession(null);
      setChatMessages([]);
    } catch (err: any) {
      setSessionError(err.message || 'Failed to unload session');
    }
  };

  // Run a silent warmup request that is fully consumed but not displayed.
  const runWarmup = useCallback((sessionId: string): Promise<void> => {
    return new Promise((resolve, reject) => {
      const warmupMessages: ChatMessage[] = [
        { role: 'user', content: 'hello, model!' },
      ];

      const abort = api.streamPlaygroundChat(
        {
          session_id: sessionId,
          messages: warmupMessages,
          stream: true,
          max_tokens: 32,
          temperature,
          top_p: topP,
          top_k: topK,
          min_p: minP,
        },
        () => {}, // consume tokens silently
        (error: string) => { warmupAbortRef.current = null; reject(new Error(error)); },
        () => { warmupAbortRef.current = null; resolve(); },
      );
      warmupAbortRef.current = abort;
    });
  }, [temperature, topP, topK, minP]);

  const handleSendMessage = useCallback(async () => {
    if (!session || !userInput.trim() || streaming) return;

    const input = userInput.trim();
    const prevMessages = chatMessages;
    const sessionId = session.session_id;

    setUserInput('');
    setStreaming(true);
    setLastTPS(null);

    // Show the user message and a "warming up" placeholder.
    setChatMessages(prev => [
      ...prev,
      { role: 'user', content: input },
      { role: 'assistant', content: cacheMode === 'imc' ? '' : '⏳ Warming up model...' },
    ]);

    // Warmup: send a throwaway message so the model is hot (skip for IMC to avoid cache corruption).
    if (cacheMode !== 'imc') {
      warmupAbortRef.current?.();
      warmupAbortRef.current = null;
      try {
        await runWarmup(sessionId);
      } catch {
        // Warmup failure is non-fatal; continue with the real request.
      }

      await new Promise(r => setTimeout(r, 1000));
    }

    // Guard: if session changed during warmup, bail out.
    if (!sessionRef.current || sessionRef.current.session_id !== sessionId) {
      setStreaming(false);
      return;
    }

    // Build the real message list (excluding the warmup placeholder).
    const messages: ChatMessage[] = [];
    if (systemPrompt.trim()) {
      messages.push({ role: 'system', content: systemPrompt });
    }
    // Use captured snapshot from before we appended the user+placeholder.
    for (const msg of prevMessages) {
      messages.push({ role: msg.role, content: msg.content });
    }
    messages.push({ role: 'user', content: input });

    // Replace the warmup placeholder with an empty assistant message.
    setChatMessages(prev => {
      const updated = [...prev];
      updated[updated.length - 1] = { role: 'assistant', content: '' };
      return updated;
    });

    contentBufferRef.current = '';
    if (throttleTimerRef.current) {
      clearTimeout(throttleTimerRef.current);
      throttleTimerRef.current = null;
    }

    let assistantContent = '';

    const abort = api.streamPlaygroundChat(
      {
        session_id: sessionId,
        messages,
        stream: true,
        stream_options: { include_usage: true },
        temperature,
        top_p: topP,
        top_k: topK,
        min_p: minP,
        max_tokens: maxTokens,
        repeat_penalty: repeatPenalty,
        repeat_last_n: repeatLastN,
        frequency_penalty: frequencyPenalty,
        presence_penalty: presencePenalty,
        dry_multiplier: dryMultiplier,
        dry_base: dryBase,
        dry_allowed_length: dryAllowedLength,
        dry_penalty_last_n: dryPenaltyLastN,
        xtc_probability: xtcProbability,
        xtc_threshold: xtcThreshold,
        xtc_min_keep: xtcMinKeep,
        enable_thinking: enableThinking,
        reasoning_effort: reasoningEffort,
      },
      (data: ChatStreamResponse) => {
        const delta = data.choices?.[0]?.delta;
        if (delta?.content) {
          assistantContent += delta.content;
          contentBufferRef.current = assistantContent;

          if (!throttleTimerRef.current) {
            throttleTimerRef.current = setTimeout(() => {
              throttleTimerRef.current = null;
              const buffered = contentBufferRef.current;
              setChatMessages(prev => {
                const updated = [...prev];
                updated[updated.length - 1] = { role: 'assistant', content: buffered };
                return updated;
              });
            }, 50);
          }
        }
        if (data.usage?.tokens_per_second) {
          setLastTPS(data.usage.tokens_per_second);
        }
      },
      (error: string) => {
        if (throttleTimerRef.current) {
          clearTimeout(throttleTimerRef.current);
          throttleTimerRef.current = null;
        }
        setChatMessages(prev => {
          const updated = [...prev];
          updated[updated.length - 1] = { role: 'assistant', content: `Error: ${error}` };
          return updated;
        });
        streamAbortRef.current = null;
        setStreaming(false);
      },
      () => {
        if (throttleTimerRef.current) {
          clearTimeout(throttleTimerRef.current);
          throttleTimerRef.current = null;
        }
        const finalContent = contentBufferRef.current;
        if (finalContent) {
          setChatMessages(prev => {
            const updated = [...prev];
            updated[updated.length - 1] = { role: 'assistant', content: finalContent };
            return updated;
          });
        }
        streamAbortRef.current = null;
        setStreaming(false);
      }
    );

    streamAbortRef.current = abort;
  }, [session, userInput, streaming, systemPrompt, chatMessages, cacheMode, temperature, topP, topK, minP, maxTokens, repeatPenalty, repeatLastN, frequencyPenalty, presencePenalty, dryMultiplier, dryBase, dryAllowedLength, dryPenaltyLastN, xtcProbability, xtcThreshold, xtcMinKeep, enableThinking, reasoningEffort, runWarmup]);

  const handleStopStreaming = () => {
    streamAbortRef.current?.();
    streamAbortRef.current = null;
    warmupAbortRef.current?.();
    warmupAbortRef.current = null;
    if (throttleTimerRef.current) {
      clearTimeout(throttleTimerRef.current);
      throttleTimerRef.current = null;
    }
    const finalContent = contentBufferRef.current;
    if (finalContent) {
      setChatMessages(prev => {
        const updated = [...prev];
        if (updated.length > 0) {
          updated[updated.length - 1] = { role: 'assistant', content: finalContent };
        }
        return updated;
      });
    }
    setStreaming(false);
  };

  const handleToolTest = useCallback(() => {
    if (!session || toolTestRunning) return;

    setToolTestRunning(true);
    setToolResult('');
    setToolCalls([]);

    let tools: any[];
    try {
      tools = JSON.parse(toolDefs);
    } catch {
      setToolResult('Invalid JSON in tool definitions');
      setToolTestRunning(false);
      return;
    }

    const messages: ChatMessage[] = [
      { role: 'user', content: toolPrompt },
    ];

    let fullContent = '';
    let collectedToolCalls: ChatToolCall[] = [];

    const abort = api.streamPlaygroundChat(
      {
        session_id: session.session_id,
        messages,
        tools,
        stream: true,
      },
      (data: ChatStreamResponse) => {
        const choice = data.choices?.[0];
        if (choice?.delta?.content) {
          fullContent += choice.delta.content;
        }
        if (choice?.delta?.tool_calls) {
          for (const tc of choice.delta.tool_calls) {
            const existing = collectedToolCalls.find(c => c.index === tc.index);
            if (existing) {
              if (tc.id && !existing.id) existing.id = tc.id;
              if (tc.type && existing.type === 'function') existing.type = tc.type;
              if (tc.function?.name && !existing.function.name) existing.function.name = tc.function.name;
              if (tc.function?.arguments) {
                existing.function.arguments += tc.function.arguments;
              }
            } else {
              collectedToolCalls.push({
                id: tc.id || '',
                index: tc.index,
                type: tc.type || 'function',
                function: {
                  name: tc.function?.name || '',
                  arguments: tc.function?.arguments || '',
                },
              });
            }
          }
        }
        if (choice?.finish_reason === 'tool_calls') {
          setToolCalls([...collectedToolCalls]);
        }
      },
      (error: string) => {
        toolTestAbortRef.current = null;
        setToolResult(`Error: ${error}`);
        setToolTestRunning(false);
      },
      () => {
        toolTestAbortRef.current = null;
        setToolResult(fullContent);
        if (collectedToolCalls.length > 0) {
          setToolCalls([...collectedToolCalls]);
        }
        setToolTestRunning(false);
      }
    );
    toolTestAbortRef.current = abort;
  }, [session, toolTestRunning, toolDefs, toolPrompt]);

  const handleInspector = useCallback(() => {
    if (!session || inspectorRunning) return;

    setInspectorRunning(true);
    setRenderedPrompt('');

    const messages: ChatMessage[] = [
      { role: 'user', content: inspectorPrompt },
    ];

    if (systemPrompt.trim()) {
      messages.unshift({ role: 'system', content: systemPrompt });
    }

    let prompt = '';

    api.streamPlaygroundChat(
      {
        session_id: session.session_id,
        messages,
        stream: true,
        return_prompt: true,
        max_tokens: 1,
      },
      (data: any) => {
        if (data.prompt) {
          prompt = data.prompt;
        }
      },
      (error: string) => {
        setRenderedPrompt(`Error: ${error}`);
        setInspectorRunning(false);
      },
      () => {
        setRenderedPrompt(prompt || '(No prompt returned — prompt may appear in final response)');
        setInspectorRunning(false);
      }
    );
  }, [session, inspectorRunning, inspectorPrompt, systemPrompt]);

  const handleExportToCatalog = () => {
    if (!session) return;

    const draft = {
      id: selectedModel,
      template: templateMode === 'builtin' ? selectedTemplate : '',
      template_script: templateMode === 'custom' ? customScript : '',
      config: {
        'context-window': contextWindow,
        nbatch: nBatch,
        nubatch: nUBatch,
        'nseq-max': nSeqMax,
        'flash-attention': flashAttention,
        'cache-type-k': cacheType,
        'cache-type-v': cacheType,
        'system-prompt-cache': cacheMode === 'spc',
        'incremental-cache': cacheMode === 'imc',
      },
      capabilities: {
        streaming: true,
        tooling: toolCalls.length > 0,
      },
    };

    sessionStorage.setItem('kronk_catalog_draft', JSON.stringify(draft));
    navigate('/catalog/editor?source=playground');
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSendMessage();
    }
  };

  // Sync stable keys with message list length (append-only).
  while (messageKeysRef.current.length < chatMessages.length) {
    messageKeysRef.current.push(++messageKeyCounterRef.current);
  }
  messageKeysRef.current.length = chatMessages.length;

  return (
    <div className="playground-container">
      <div className="playground-header">
        <h2>Model Playground</h2>
        {session && (
          <button className="btn btn-secondary" onClick={handleExportToCatalog}>
            Export to Catalog Editor
          </button>
        )}
      </div>

      <div className="playground-layout">
        {/* Left Sidebar: Model Config + Mode Selector */}
        <div className="playground-mode-selector">
          <div className="playground-model-config">
            <div className="form-group">
              <label htmlFor="pg-model">Model</label>
              <select
                id="pg-model"
                value={showPullForm ? NEW_MODEL_VALUE : selectedModel}
                onChange={(e) => {
                  const val = e.target.value;
                  if (val === NEW_MODEL_VALUE) {
                    setSelectedModel('');
                    setShowPullForm(true);
                  } else {
                    setSelectedModel(val);
                    setShowPullForm(false);
                  }
                }}
                disabled={!!session}
              >
                <option value="">Select a model...</option>
                {models?.data?.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.id}
                  </option>
                ))}
                <option value={NEW_MODEL_VALUE}>New…</option>
              </select>
            </div>

            {showPullForm && !session && (
              <div className="playground-pull-form">
                <div className="form-group">
                  <label>HuggingFace Model URL</label>
                  <input
                    type="text"
                    value={hfModelUrl}
                    onChange={(e) => setHfModelUrl(e.target.value)}
                    placeholder="org/repo/model.gguf"
                    disabled={isDownloading}
                  />
                </div>

                <button
                  type="button"
                  className="btn btn-secondary btn-small playground-pull-toggle"
                  onClick={() => setShowProjUrl((v) => !v)}
                  disabled={isDownloading}
                >
                  {showProjUrl ? '− Hide projection URL' : '+ Projection URL (optional)'}
                </button>

                {showProjUrl && (
                  <div className="form-group">
                    <label>Projection URL (vision/audio models)</label>
                    <input
                      type="text"
                      value={hfProjUrl}
                      onChange={(e) => setHfProjUrl(e.target.value)}
                      placeholder="org/repo/mmproj.gguf"
                      disabled={isDownloading}
                    />
                  </div>
                )}

                <div className="playground-pull-actions">
                  <button
                    className="btn btn-primary"
                    type="button"
                    onClick={handlePullModel}
                    disabled={isDownloading || !hfModelUrl.trim()}
                  >
                    {isDownloading ? 'Pulling…' : 'Pull'}
                  </button>
                  {isDownloading && (
                    <button className="btn btn-danger" type="button" onClick={cancelDownload}>
                      Cancel
                    </button>
                  )}
                  {download && download.status !== 'downloading' && (
                    <button className="btn" type="button" onClick={clearDownload}>
                      Clear
                    </button>
                  )}
                </div>

                {download && download.messages.length > 0 && (
                  <div className="status-box playground-pull-status">
                    {download.messages.map((msg, idx) => (
                      <div key={idx} className={`status-line ${msg.type}`}>
                        {msg.text}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            <div className="form-group">
              <label htmlFor="pg-template-mode">Template Mode</label>
              <select
                id="pg-template-mode"
                value={templateMode}
                onChange={(e) => setTemplateMode(e.target.value as 'builtin' | 'custom')}
                disabled={!!session}
              >
                <option value="builtin">Builtin</option>
                <option value="custom">Custom</option>
              </select>
            </div>

            {templateMode === 'builtin' ? (
              <div className="form-group">
                <label htmlFor="pg-template">Template</label>
                <select
                  id="pg-template"
                  value={selectedTemplate}
                  onChange={(e) => setSelectedTemplate(e.target.value)}
                  disabled={!!session}
                >
                  <option value="">Auto (from catalog)</option>
                  {templates.map((t) => (
                    <option key={t.name} value={t.name}>
                      {t.name}
                    </option>
                  ))}
                </select>
              </div>
            ) : (
              <div className="form-group">
                <label htmlFor="pg-template-script">Template Script</label>
                <textarea
                  id="pg-template-script"
                  value={customScript}
                  onChange={(e) => setCustomScript(e.target.value)}
                  disabled={!!session}
                  rows={8}
                  className="playground-textarea"
                  placeholder="Paste Jinja template..."
                />
              </div>
            )}
          </div>

          <button
            className={`playground-mode-btn ${playgroundMode === 'automated' ? 'active' : ''}`}
            onClick={() => setPlaygroundMode('automated')}
          >
            Automated Mode
          </button>
          <button
            className={`playground-mode-btn ${playgroundMode === 'manual' ? 'active' : ''}`}
            onClick={() => setPlaygroundMode('manual')}
          >
            Manual Mode
          </button>
        </div>

        {playgroundMode === 'automated' && (
          <div className="playground-test" style={{ flex: 1 }}>
            <div className="playground-tab-content">
              <AutomatedTestingPanel
                session={session}
                catalogSampling={catalogConfig?.['sampling-parameters'] ?? null}
                sessionSeed={{
                  model_id: selectedModel,
                  template_mode: templateMode,
                  template_name: templateMode === 'builtin' ? selectedTemplate : undefined,
                  template_script: templateMode === 'custom' ? customScript : undefined,
                  base_config: {
                    context_window: contextWindow,
                    nbatch: nBatch,
                    nubatch: nUBatch,
                    nseq_max: nSeqMax,
                    flash_attention: flashAttention,
                    cache_type_k: cacheType || undefined,
                    cache_type_v: cacheType || undefined,
                    system_prompt_cache: cacheMode === 'spc',
                    incremental_cache: cacheMode === 'imc',
                  },
                }}
              />
            </div>
          </div>
        )}

        {playgroundMode === 'manual' && (
        <>
        {/* Setup Panel */}
        <div className="playground-setup">
          <h3>Setup</h3>

          <h4>Configuration</h4>
          <div className="playground-config-grid-fluid">
            <div className="form-group">
              <label htmlFor="pg-context-window">Context Window{PARAM_TOOLTIPS.contextWindow && <ParamTooltip text={PARAM_TOOLTIPS.contextWindow} />}</label>
              <input
                id="pg-context-window"
                type="number"
                value={contextWindow}
                onChange={(e) => setContextWindow(Number(e.target.value))}
                disabled={!!session}
              />
            </div>
            <div className="form-group">
              <label htmlFor="pg-nbatch">NBatch{PARAM_TOOLTIPS.nbatch && <ParamTooltip text={PARAM_TOOLTIPS.nbatch} />}</label>
              <input
                id="pg-nbatch"
                type="number"
                value={nBatch}
                onChange={(e) => setNBatch(Number(e.target.value))}
                disabled={!!session}
              />
            </div>
            <div className="form-group">
              <label htmlFor="pg-nubatch">NUBatch{PARAM_TOOLTIPS.nubatch && <ParamTooltip text={PARAM_TOOLTIPS.nubatch} />}</label>
              <input
                id="pg-nubatch"
                type="number"
                value={nUBatch}
                onChange={(e) => setNUBatch(Number(e.target.value))}
                disabled={!!session}
              />
            </div>
            <div className="form-group">
              <label htmlFor="pg-nseqmax">NSeqMax{PARAM_TOOLTIPS.nSeqMax && <ParamTooltip text={PARAM_TOOLTIPS.nSeqMax} />}</label>
              <input
                id="pg-nseqmax"
                type="number"
                value={nSeqMax}
                onChange={(e) => setNSeqMax(Number(e.target.value))}
                min={1}
                disabled={!!session}
              />
            </div>
            <div className="form-group">
              <label htmlFor="pg-flash-attn">Flash Attention{PARAM_TOOLTIPS.flashAttention && <ParamTooltip text={PARAM_TOOLTIPS.flashAttention} />}</label>
              <select
                id="pg-flash-attn"
                value={flashAttention}
                onChange={(e) => setFlashAttention(e.target.value)}
                disabled={!!session}
              >
                <option value="auto">Auto</option>
                <option value="enabled">Enabled</option>
                <option value="disabled">Disabled</option>
              </select>
            </div>
            <div className="form-group">
              <label htmlFor="pg-cache-type">KV Cache Type{PARAM_TOOLTIPS.cacheType && <ParamTooltip text={PARAM_TOOLTIPS.cacheType} />}</label>
              <select
                id="pg-cache-type"
                value={cacheType}
                onChange={(e) => setCacheType(e.target.value)}
                disabled={!!session}
              >
                <option value="">Default (f16)</option>
                <option value="f16">f16</option>
                <option value="q8_0">q8_0</option>
                <option value="q4_0">q4_0</option>
              </select>
            </div>
            <div className="form-group">
              <label>Cache Mode{PARAM_TOOLTIPS.cacheMode && <ParamTooltip text={PARAM_TOOLTIPS.cacheMode} />}</label>
              <select
                value={cacheMode}
                onChange={(e) => setCacheMode(e.target.value)}
                disabled={!!session}
              >
                <option value="none">None</option>
                <option value="spc">SPC (System Prompt)</option>
                <option value="imc">IMC (Incremental)</option>
              </select>
            </div>
          </div>

          <div className="playground-session-controls">
            {!session ? (
              <button
                className="btn btn-primary"
                onClick={handleCreateSession}
                disabled={!selectedModel || sessionLoading || configLoading}
              >
                {sessionLoading ? 'Loading Model...' : configLoading ? 'Loading Config...' : 'Create Session'}
              </button>
            ) : (
              <button className="btn btn-danger" onClick={handleUnloadSession}>
                Unload Session
              </button>
            )}
          </div>

          {sessionError && <div className="playground-error">{sessionError}</div>}

          {session && (
            <div className="playground-session-info">
              <strong>Session:</strong> {session.session_id}
              <br />
              <strong>Status:</strong> {session.status}
              {session.effective_config && (
                <div className="playground-effective-config">
                  <strong>Effective Config:</strong>
                  <div className="playground-config-grid">
                    {Object.entries(session.effective_config).map(([key, value]) => (
                      <div key={key} className="playground-config-item">
                        <span className="playground-config-key">{key}:</span>{' '}
                        <span className="playground-config-value">{String(value)}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Test Panel */}
        <div className="playground-test">
          <div className="playground-tabs" role="tablist">
            <button
              role="tab"
              id="tab-chat"
              aria-selected={activeTab === 'chat'}
              aria-controls="tabpanel-chat"
              className={`playground-tab ${activeTab === 'chat' ? 'active' : ''}`}
              onClick={() => setActiveTab('chat')}
            >
              Basic Chat
            </button>
            <button
              role="tab"
              id="tab-tools"
              aria-selected={activeTab === 'tools'}
              aria-controls="tabpanel-tools"
              className={`playground-tab ${activeTab === 'tools' ? 'active' : ''}`}
              onClick={() => setActiveTab('tools')}
            >
              Tool Calling Test
            </button>
            <button
              role="tab"
              id="tab-inspector"
              aria-selected={activeTab === 'inspector'}
              aria-controls="tabpanel-inspector"
              className={`playground-tab ${activeTab === 'inspector' ? 'active' : ''}`}
              onClick={() => setActiveTab('inspector')}
            >
              Prompt Inspector
            </button>
          </div>

          <div className="playground-tab-content">
            {activeTab === 'chat' && (
              <div role="tabpanel" id="tabpanel-chat" aria-labelledby="tab-chat" className="playground-chat">
                <details className="playground-sampling-params">
                  <summary>Chat Parameters</summary>

                  <h5 className="playground-param-group-title">System Prompt</h5>
                  <div className="form-group">
                    <textarea
                        value={systemPrompt}
                        onChange={(e) => setSystemPrompt(e.target.value)}
                        rows={2}
                        className="playground-textarea"
                    />
                  </div>

                  <h5 className="playground-param-group-title">Generation</h5>
                  <div className="playground-config-grid-fluid">
                    <div className="form-group">
                      <label htmlFor="pg-temperature">Temperature{PARAM_TOOLTIPS.temperature && <ParamTooltip text={PARAM_TOOLTIPS.temperature} />}</label>
                      <input id="pg-temperature" type="number" value={temperature} onChange={(e) => setTemperature(Number(e.target.value))} step={0.1} min={0} />
                    </div>
                    <div className="form-group">
                      <label htmlFor="pg-top-p">Top P{PARAM_TOOLTIPS.top_p && <ParamTooltip text={PARAM_TOOLTIPS.top_p} />}</label>
                      <input id="pg-top-p" type="number" value={topP} onChange={(e) => setTopP(Number(e.target.value))} step={0.05} min={0} max={1} />
                    </div>
                    <div className="form-group">
                      <label htmlFor="pg-top-k">Top K{PARAM_TOOLTIPS.top_k && <ParamTooltip text={PARAM_TOOLTIPS.top_k} />}</label>
                      <input id="pg-top-k" type="number" value={topK} onChange={(e) => setTopK(Math.floor(Number(e.target.value)))} step={1} min={0} />
                    </div>
                    <div className="form-group">
                      <label htmlFor="pg-min-p">Min P{PARAM_TOOLTIPS.min_p && <ParamTooltip text={PARAM_TOOLTIPS.min_p} />}</label>
                      <input id="pg-min-p" type="number" value={minP} onChange={(e) => setMinP(Number(e.target.value))} step={0.01} min={0} max={1} />
                    </div>
                    <div className="form-group">
                      <label htmlFor="pg-max-tokens">Max Tokens{PARAM_TOOLTIPS.max_tokens && <ParamTooltip text={PARAM_TOOLTIPS.max_tokens} />}</label>
                      <input id="pg-max-tokens" type="number" value={maxTokens} onChange={(e) => setMaxTokens(Math.floor(Number(e.target.value)))} step={256} min={1} />
                    </div>
                  </div>

                  <h5 className="playground-param-group-title">Repetition Control</h5>
                  <div className="playground-config-grid-fluid">
                    <div className="form-group">
                      <label>Repeat Penalty{PARAM_TOOLTIPS.repeat_penalty && <ParamTooltip text={PARAM_TOOLTIPS.repeat_penalty} />}</label>
                      <input type="number" value={repeatPenalty} onChange={(e) => setRepeatPenalty(Number(e.target.value))} step={0.1} min={0} />
                    </div>
                    <div className="form-group">
                      <label>Repeat Last N{PARAM_TOOLTIPS.repeat_last_n && <ParamTooltip text={PARAM_TOOLTIPS.repeat_last_n} />}</label>
                      <input type="number" value={repeatLastN} onChange={(e) => setRepeatLastN(Math.floor(Number(e.target.value)))} step={1} min={0} />
                    </div>
                    <div className="form-group">
                      <label>Frequency Penalty{PARAM_TOOLTIPS.frequency_penalty && <ParamTooltip text={PARAM_TOOLTIPS.frequency_penalty} />}</label>
                      <input type="number" value={frequencyPenalty} onChange={(e) => setFrequencyPenalty(Number(e.target.value))} step={0.1} min={0} />
                    </div>
                    <div className="form-group">
                      <label>Presence Penalty{PARAM_TOOLTIPS.presence_penalty && <ParamTooltip text={PARAM_TOOLTIPS.presence_penalty} />}</label>
                      <input type="number" value={presencePenalty} onChange={(e) => setPresencePenalty(Number(e.target.value))} step={0.1} min={0} />
                    </div>
                  </div>

                  <h5 className="playground-param-group-title">DRY Sampler</h5>
                  <div className="playground-config-grid-fluid">
                    <div className="form-group">
                      <label>DRY Multiplier{PARAM_TOOLTIPS.dry_multiplier && <ParamTooltip text={PARAM_TOOLTIPS.dry_multiplier} />}</label>
                      <input type="number" value={dryMultiplier} onChange={(e) => setDryMultiplier(Number(e.target.value))} step={0.05} min={0} />
                    </div>
                    <div className="form-group">
                      <label>DRY Base{PARAM_TOOLTIPS.dry_base && <ParamTooltip text={PARAM_TOOLTIPS.dry_base} />}</label>
                      <input type="number" value={dryBase} onChange={(e) => setDryBase(Number(e.target.value))} step={0.05} min={0} />
                    </div>
                    <div className="form-group">
                      <label>DRY Allowed Length{PARAM_TOOLTIPS.dry_allowed_length && <ParamTooltip text={PARAM_TOOLTIPS.dry_allowed_length} />}</label>
                      <input type="number" value={dryAllowedLength} onChange={(e) => setDryAllowedLength(Math.floor(Number(e.target.value)))} step={1} min={0} />
                    </div>
                    <div className="form-group">
                      <label>DRY Penalty Last N{PARAM_TOOLTIPS.dry_penalty_last_n && <ParamTooltip text={PARAM_TOOLTIPS.dry_penalty_last_n} />}</label>
                      <input type="number" value={dryPenaltyLastN} onChange={(e) => setDryPenaltyLastN(Math.floor(Number(e.target.value)))} step={1} min={0} />
                    </div>
                  </div>

                  <h5 className="playground-param-group-title">XTC Sampler</h5>
                  <div className="playground-config-grid-fluid">
                    <div className="form-group">
                      <label>XTC Probability{PARAM_TOOLTIPS.xtc_probability && <ParamTooltip text={PARAM_TOOLTIPS.xtc_probability} />}</label>
                      <input type="number" value={xtcProbability} onChange={(e) => setXtcProbability(Number(e.target.value))} step={0.01} min={0} max={1} />
                    </div>
                    <div className="form-group">
                      <label>XTC Threshold{PARAM_TOOLTIPS.xtc_threshold && <ParamTooltip text={PARAM_TOOLTIPS.xtc_threshold} />}</label>
                      <input type="number" value={xtcThreshold} onChange={(e) => setXtcThreshold(Number(e.target.value))} step={0.01} min={0} max={1} />
                    </div>
                    <div className="form-group">
                      <label>XTC Min Keep{PARAM_TOOLTIPS.xtc_min_keep && <ParamTooltip text={PARAM_TOOLTIPS.xtc_min_keep} />}</label>
                      <input type="number" value={xtcMinKeep} onChange={(e) => setXtcMinKeep(Math.floor(Number(e.target.value)))} step={1} min={1} />
                    </div>
                  </div>

                  <h5 className="playground-param-group-title">Reasoning</h5>
                  <div className="playground-config-grid-fluid">
                    <div className="form-group">
                      <label htmlFor="pg-enable-thinking">Enable Thinking{PARAM_TOOLTIPS.enable_thinking && <ParamTooltip text={PARAM_TOOLTIPS.enable_thinking} />}</label>
                      <select id="pg-enable-thinking" value={enableThinking} onChange={(e) => setEnableThinking(e.target.value as 'true' | 'false')}>
                        <option value="true">Enabled</option>
                        <option value="false">Disabled</option>
                      </select>
                    </div>
                    <div className="form-group">
                      <label htmlFor="pg-reasoning-effort">Reasoning Effort{PARAM_TOOLTIPS.reasoning_effort && <ParamTooltip text={PARAM_TOOLTIPS.reasoning_effort} />}</label>
                      <select id="pg-reasoning-effort" value={reasoningEffort} onChange={(e) => setReasoningEffort(e.target.value as typeof reasoningEffort)}>
                        <option value="none">None</option>
                        <option value="minimal">Minimal</option>
                        <option value="low">Low</option>
                        <option value="medium">Medium</option>
                        <option value="high">High</option>
                      </select>
                    </div>
                  </div>
                </details>

                <div className="playground-messages">
                  {chatMessages.map((msg, i) => (
                    <div key={messageKeysRef.current[i]} className={`playground-message playground-message-${msg.role}`}>
                      <div className="playground-message-role">{msg.role}</div>
                      <div className="playground-message-content">{msg.content}</div>
                    </div>
                  ))}
                  <div ref={messagesEndRef} />
                </div>

                <div className="playground-input-row">
                  <textarea
                    value={userInput}
                    onChange={(e) => setUserInput(e.target.value)}
                    onKeyDown={handleKeyDown}
                    placeholder={session ? 'Type a message...' : 'Create a session first'}
                    disabled={!session || streaming}
                    rows={2}
                    className="playground-textarea"
                  />
                  {streaming ? (
                    <button className="btn btn-danger" onClick={handleStopStreaming}>
                      Stop
                    </button>
                  ) : (
                    <button
                      className="btn btn-primary"
                      onClick={handleSendMessage}
                      disabled={!session || !userInput.trim()}
                    >
                      Send
                    </button>
                  )}
                  {lastTPS !== null && (
                    <span style={{ fontSize: 12, opacity: 0.7, marginLeft: 8, whiteSpace: 'nowrap' }}>
                      {lastTPS.toFixed(1)} TPS
                    </span>
                  )}
                </div>
              </div>
            )}

            {activeTab === 'tools' && (
              <div role="tabpanel" id="tabpanel-tools" aria-labelledby="tab-tools" className="playground-tools">
                <div className="form-group">
                  <label>Tool Definitions (JSON)</label>
                  <textarea
                    value={toolDefs}
                    onChange={(e) => setToolDefs(e.target.value)}
                    rows={12}
                    className="playground-textarea monospace"
                  />
                </div>

                <div className="form-group">
                  <label>Test Prompt</label>
                  <input
                    type="text"
                    value={toolPrompt}
                    onChange={(e) => setToolPrompt(e.target.value)}
                  />
                </div>

                <button
                  className="btn btn-primary"
                  onClick={handleToolTest}
                  disabled={!session || toolTestRunning}
                >
                  {toolTestRunning ? 'Running...' : 'Run Test'}
                </button>

                {(toolCalls.length > 0 || toolResult) && (
                  <div className="playground-tool-results">
                    <h4>Results</h4>
                    {toolCalls.length > 0 ? (
                      <div className="playground-tool-pass">
                        <span className="playground-badge success">PASS</span>
                        Model emitted {toolCalls.length} tool call(s)
                        {toolCalls.map((tc, i) => (
                          <div key={i} className="playground-tool-call">
                            <strong>{tc.function.name}</strong>
                            <pre>{tc.function.arguments}</pre>
                            {tc.id && <small>ID: {tc.id}</small>}
                          </div>
                        ))}
                      </div>
                    ) : (
                      <div className="playground-tool-fail">
                        <span className="playground-badge fail">NO TOOL CALLS</span>
                        <pre>{toolResult}</pre>
                      </div>
                    )}
                  </div>
                )}
              </div>
            )}

            {activeTab === 'inspector' && (
              <div role="tabpanel" id="tabpanel-inspector" aria-labelledby="tab-inspector" className="playground-inspector">
                <div className="form-group">
                  <label>Test Message</label>
                  <input
                    type="text"
                    value={inspectorPrompt}
                    onChange={(e) => setInspectorPrompt(e.target.value)}
                  />
                </div>

                <button
                  className="btn btn-primary"
                  onClick={handleInspector}
                  disabled={!session || inspectorRunning}
                >
                  {inspectorRunning ? 'Rendering...' : 'Render Prompt'}
                </button>

                {renderedPrompt && (
                  <div className="playground-rendered-prompt">
                    <div className="playground-prompt-header">
                      <h4>Rendered Prompt</h4>
                      <button
                        className="btn btn-secondary btn-small"
                        onClick={() => navigator.clipboard.writeText(renderedPrompt)}
                      >
                        Copy
                      </button>
                    </div>
                    <pre className="playground-prompt-text">{renderedPrompt}</pre>
                  </div>
                )}
              </div>
            )}

          </div>
        </div>
        </>
        )}
      </div>
    </div>
  );
}
