import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import { useDownload } from '../contexts/DownloadContext';
import DownloadInfoTable from './DownloadInfoTable';
import DownloadProgressBar from './DownloadProgressBar';
import { usePlayground } from '../contexts/PlaygroundContext';
import type {
  PlaygroundTemplateInfo,
  PlaygroundChatRequest,
  ChatMessage,
  ChatStreamResponse,
  ChatToolCall,
  ModelConfig,
  VRAM,
} from '../types';
import AutomatedTestingPanel from './AutomatedTestingPanel';
import ChatPanel, { type StreamTransport } from './ChatPanel';
import ModelSelector from './ModelSelector';
import PlaygroundHistory from './PlaygroundHistory';
import { autoTestTools } from '../services/autoTestRunner';
import type { SamplingParams } from '../contexts/SamplingContext';
import { PARAM_TOOLTIPS, FieldLabel } from './ParamTooltips';
import { formatBytes } from '../lib/format';
import { extractContextInfo, formatContextHint } from '../lib/context';
import { useDevicesInfo, useMoeFit, MOE_STRATEGY_OPTIONS, VRAM_FIT_TEXT, VRAM_FIT_THRESHOLD } from './vram';
import type { VramFitStatus } from './vram';

const NEW_MODEL_VALUE = '__new__';

const defaultTools = JSON.stringify(autoTestTools, null, 2);

export default function ModelPlayground() {
  const navigate = useNavigate();
  const { models, loadModels } = useModelList();
  const { download, isDownloading, startDownload, cancelDownload, clearDownload } = useDownload();

  // Persistent state from context (survives navigation)
  const {
    session, setSession,
    chatMessages, setChatMessages,
    selectedModel, setSelectedModel,
    playgroundMode, setPlaygroundMode,
    activeTab, setActiveTab,
    systemPrompt, setSystemPrompt,
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
    moeMode, setMoeMode,
    moeKeepTopN, setMoeKeepTopN,
    tensorBuftOverrides, setTensorBuftOverrides,
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

  // Device info
  const devicesInfo = useDevicesInfo();

  // Model metadata (for context info)
  const [modelMetadata, setModelMetadata] = useState<Record<string, string> | undefined>(undefined);
  const contextInfo = extractContextInfo(modelMetadata);

  // MoE state
  const [moeBlockCount, setMoeBlockCount] = useState(0);
  const [modelVramInfo, setModelVramInfo] = useState<VRAM | null>(null);
  const [catalogMoeMode, setCatalogMoeMode] = useState('');
  const moeModeTouchedRef = useRef(false);

  // MoE fit — reacts to contextWindow, nSeqMax, and cache type changes
  const fitBytesPerElement = cacheType === 'q8_0' ? 1 : cacheType === 'q4_0' ? 0.5625 : 2;
  const fitOverrides = useMemo(() => ({ contextWindow, slots: nSeqMax, bytesPerElement: fitBytesPerElement }), [contextWindow, nSeqMax, fitBytesPerElement]);
  const moeFit = useMoeFit(modelVramInfo, modelMetadata, devicesInfo, fitOverrides);
  const isMoE = moeFit.isMoe;
  const vramFitStatus: VramFitStatus | null = moeFit.fit?.status ?? null;
  const vramNeededAllGPU = moeFit.fit?.allGPU ?? 0;
  const vramNeededCPUExperts = moeFit.fit?.cpuExperts ?? 0;

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
          setMoeMode(mc.moe?.mode || '');
          setMoeKeepTopN(mc.moe?.['keep-experts-top-n'] ?? 0);
        }

        // Store metadata for context info.
        setModelMetadata(info.metadata);

        // Store VRAM info — MoE detection and fit are handled by useMoeFit.
        setModelVramInfo(info.vram || null);
        setCatalogMoeMode(mc?.moe?.mode || '');
        moeModeTouchedRef.current = false;

        // Extract block_count for MoE layer slider range.
        if (info.metadata) {
          const blockKey = Object.keys(info.metadata).find(k => k.endsWith('.block_count'));
          setMoeBlockCount(blockKey ? parseInt(info.metadata[blockKey], 10) || 0 : 0);
        }

        setHydratedModelId(selectedModel);
      })
      .catch((err) => {
        if (!cancelled) setSessionError(err.message || 'Failed to load model config');
      })
      .finally(() => { if (!cancelled) setConfigLoading(false); });

    return () => { cancelled = true; };
  }, [selectedModel, hydratedModelId]);

  // Auto-suggest experts_cpu when fit is tight and user hasn't touched the mode.
  useEffect(() => {
    if (vramFitStatus && vramFitStatus !== 'fits' && !catalogMoeMode && !moeModeTouchedRef.current) {
      setMoeMode('experts_cpu');
    }
  }, [vramFitStatus, catalogMoeMode]);

  useEffect(() => {
    return () => {
      toolTestAbortRef.current?.();
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
        config['cache_type_k'] = cacheType || 'f16';
        config['cache_type_v'] = cacheType || 'f16';
      }
      const catalogCacheMode = catalogConfig?.['incremental-cache'] ? 'imc' : catalogConfig?.['system-prompt-cache'] ? 'spc' : 'none';
      if (!catalogConfig || cacheMode !== catalogCacheMode) {
        config['system_prompt_cache'] = cacheMode === 'spc';
        config['incremental_cache'] = cacheMode === 'imc';
      }
      if (moeMode) {
        config['moe_mode'] = moeMode;
        if (moeMode === 'keep_top_n') {
          config['moe_keep_experts_top_n'] = moeKeepTopN;
        }
      }
      if (moeMode === 'custom' && tensorBuftOverrides.length > 0) {
        config['tensor_buft_overrides'] = tensorBuftOverrides;
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
    toolTestAbortRef.current?.();
    toolTestAbortRef.current = null;
    setToolTestRunning(false);

    try {
      await api.deletePlaygroundSession(session.session_id);
      setSession(null);
      setChatMessages([]);
    } catch (err: any) {
      setSessionError(err.message || 'Failed to unload session');
    }
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
              if (tc.type) existing.type = tc.type;
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

  // ── ChatPanel bridge for Basic Chat tab ─────────────────────────────
  const playgroundSampling = useMemo<SamplingParams>(() => ({
    maxTokens,
    temperature,
    topP,
    topK,
    minP,
    presencePenalty,
    repeatPenalty,
    repeatLastN,
    dryMultiplier,
    dryBase,
    dryAllowedLen: dryAllowedLength,
    dryPenaltyLast: dryPenaltyLastN,
    xtcProbability,
    xtcThreshold,
    xtcMinKeep,
    frequencyPenalty,
    enableThinking,
    reasoningEffort,
    returnPrompt: false,
    includeUsage: true,
    logprobs: false,
    topLogprobs: 0,
    grammar: '',
    systemPrompt,
    cacheId: '',
  }), [maxTokens, temperature, topP, topK, minP, presencePenalty, repeatPenalty, repeatLastN,
    dryMultiplier, dryBase, dryAllowedLength, dryPenaltyLastN, xtcProbability, xtcThreshold,
    xtcMinKeep, frequencyPenalty, enableThinking, reasoningEffort, systemPrompt]);

  const setPlaygroundSampling = useCallback((partial: Partial<SamplingParams>) => {
    if (partial.maxTokens !== undefined) setMaxTokens(partial.maxTokens);
    if (partial.temperature !== undefined) setTemperature(partial.temperature);
    if (partial.topP !== undefined) setTopP(partial.topP);
    if (partial.topK !== undefined) setTopK(partial.topK);
    if (partial.minP !== undefined) setMinP(partial.minP);
    if (partial.presencePenalty !== undefined) setPresencePenalty(partial.presencePenalty);
    if (partial.repeatPenalty !== undefined) setRepeatPenalty(partial.repeatPenalty);
    if (partial.repeatLastN !== undefined) setRepeatLastN(partial.repeatLastN);
    if (partial.dryMultiplier !== undefined) setDryMultiplier(partial.dryMultiplier);
    if (partial.dryBase !== undefined) setDryBase(partial.dryBase);
    if (partial.dryAllowedLen !== undefined) setDryAllowedLength(partial.dryAllowedLen);
    if (partial.dryPenaltyLast !== undefined) setDryPenaltyLastN(partial.dryPenaltyLast);
    if (partial.xtcProbability !== undefined) setXtcProbability(partial.xtcProbability);
    if (partial.xtcThreshold !== undefined) setXtcThreshold(partial.xtcThreshold);
    if (partial.xtcMinKeep !== undefined) setXtcMinKeep(partial.xtcMinKeep);
    if (partial.frequencyPenalty !== undefined) setFrequencyPenalty(partial.frequencyPenalty);
    if (partial.enableThinking !== undefined) setEnableThinking(partial.enableThinking as 'true' | 'false');
    if (partial.reasoningEffort !== undefined) setReasoningEffort(partial.reasoningEffort as typeof reasoningEffort);
    if (partial.systemPrompt !== undefined) setSystemPrompt(partial.systemPrompt);
  }, [setSystemPrompt]);

  const playgroundTransport = useCallback<StreamTransport>(({ messages, sampling, onMessage, onError, onComplete }) => {
    if (!session) {
      onError('No session');
      return () => {};
    }
    return api.streamPlaygroundChat(
      {
        session_id: session.session_id,
        messages,
        stream: true,
        stream_options: { include_usage: sampling.includeUsage },
        temperature: sampling.temperature,
        top_p: sampling.topP,
        top_k: sampling.topK,
        min_p: sampling.minP,
        max_tokens: sampling.maxTokens,
        repeat_penalty: sampling.repeatPenalty,
        repeat_last_n: sampling.repeatLastN,
        frequency_penalty: sampling.frequencyPenalty,
        presence_penalty: sampling.presencePenalty,
        dry_multiplier: sampling.dryMultiplier,
        dry_base: sampling.dryBase,
        dry_allowed_length: sampling.dryAllowedLen,
        dry_penalty_last_n: sampling.dryPenaltyLast,
        xtc_probability: sampling.xtcProbability,
        xtc_threshold: sampling.xtcThreshold,
        xtc_min_keep: sampling.xtcMinKeep,
        enable_thinking: (sampling.enableThinking || undefined) as 'true' | 'false' | undefined,
        reasoning_effort: (sampling.reasoningEffort || undefined) as PlaygroundChatRequest['reasoning_effort'],
        grammar: sampling.grammar || undefined,
        return_prompt: sampling.returnPrompt,
        logprobs: sampling.logprobs,
        top_logprobs: sampling.topLogprobs,
      },
      onMessage,
      onError,
      onComplete,
    );
  }, [session]);

  const handlePlaygroundClear = useCallback(() => {
    setChatMessages([]);
  }, [setChatMessages]);

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
              <label>Model</label>
              <ModelSelector
                models={models?.data}
                selectedModel={showPullForm ? NEW_MODEL_VALUE : selectedModel}
                onSelect={(val) => {
                  if (val === NEW_MODEL_VALUE) {
                    setSelectedModel('');
                    setShowPullForm(true);
                  } else {
                    setSelectedModel(val);
                    setShowPullForm(false);
                  }
                }}
                disabled={!!session}
                extraItems={[{ id: NEW_MODEL_VALUE, label: 'New…' }]}
              />
            </div>

            {showPullForm && !session && (
              <div className="playground-pull-form">
                <div className="form-group">
                  <label>HuggingFace Model URL or Shorthand</label>
                  <input
                    type="text"
                    value={hfModelUrl}
                    onChange={(e) => setHfModelUrl(e.target.value)}
                    placeholder="owner/repo:Q4_K_M or org/repo/model.gguf"
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

                {download && download.meta && (
                  <DownloadInfoTable meta={download.meta} />
                )}

                {download && download.progress && isDownloading && (
                  <DownloadProgressBar progress={download.progress} meta={download.meta} />
                )}

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

          {devicesInfo && (
            <div className="playground-system-info">
              {devicesInfo.gpuCount > 0 && (
                <span>
                  {devicesInfo.gpuCount} GPU{devicesInfo.gpuCount > 1 ? 's' : ''}{devicesInfo.gpuType ? ` (${devicesInfo.gpuType})` : ''}
                </span>
              )}
              {devicesInfo.gpuVramBytes > 0 && (
                <span>Total VRAM: {formatBytes(devicesInfo.gpuVramBytes)}</span>
              )}
              {devicesInfo.ramBytes > 0 && (
                <span>System RAM: {formatBytes(devicesInfo.ramBytes)}</span>
              )}
            </div>
          )}

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
          <button
            className={`playground-mode-btn ${playgroundMode === 'history' ? 'active' : ''}`}
            onClick={() => setPlaygroundMode('history')}
          >
            History
          </button>
        </div>

        {playgroundMode === 'automated' && (
          <div className="playground-test" style={{ flex: 1 }}>
            <div className="playground-tab-content">
              <AutomatedTestingPanel
                session={session}
                catalogSampling={catalogConfig?.['sampling-parameters'] ?? null}
                isMoE={isMoE}
                gpuVramBytes={devicesInfo?.gpuVramBytes}
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
                    moe_mode: moeMode || undefined,
                    moe_keep_experts_top_n: moeMode === 'keep_top_n' ? moeKeepTopN : undefined,
                    tensor_buft_overrides: moeMode === 'custom' && tensorBuftOverrides.length > 0 ? tensorBuftOverrides : undefined,
                  },
                }}
              />
            </div>
          </div>
        )}

        {playgroundMode === 'history' && (
          <div className="playground-test" style={{ flex: 1 }}>
            <div className="playground-tab-content">
              <PlaygroundHistory />
            </div>
          </div>
        )}

        {playgroundMode === 'manual' && (
        <>
        {/* Setup Panel */}
        <div className="playground-setup">
          <h3>Setup</h3>

          {isMoE && !session && devicesInfo && (
            <div className="playground-moe-guide">
              <div className="playground-moe-guide-header">
                <span className="playground-moe-guide-icon">🧩</span>
                <span>Mixture of Experts Model Detected</span>
              </div>
              <div className="playground-moe-guide-body">
                <div className="playground-moe-guide-hw">
                  <strong>Your Hardware:</strong> {devicesInfo.gpuType || 'GPU'} — {formatBytes(devicesInfo.gpuVramBytes)} VRAM, {formatBytes(devicesInfo.ramBytes)} RAM
                </div>
                <div className="playground-moe-guide-estimates">
                  <div className="playground-moe-guide-estimate">
                    <span>⚡ All on GPU:</span> <strong>{formatBytes(vramNeededAllGPU)}</strong>
                    {vramFitStatus === 'fits' ? <span className="playground-moe-guide-badge playground-moe-guide-badge--ok">fits</span> : <span className="playground-moe-guide-badge playground-moe-guide-badge--no">won't fit</span>}
                  </div>
                  <div className="playground-moe-guide-estimate">
                    <span>💾 Experts on CPU:</span> <strong>{formatBytes(vramNeededCPUExperts)}</strong>
                    {vramNeededCPUExperts <= devicesInfo.gpuVramBytes * VRAM_FIT_THRESHOLD ? <span className="playground-moe-guide-badge playground-moe-guide-badge--ok">fits</span> : <span className="playground-moe-guide-badge playground-moe-guide-badge--no">tight</span>}
                  </div>
                </div>
                {vramFitStatus && vramFitStatus !== 'fits' && (
                  <button
                    className="btn btn-primary playground-moe-guide-apply"
                    onClick={() => {
                      setMoeMode('experts_cpu');
                      setFlashAttention('enabled');
                      if (nBatch < 4096) setNBatch(4096);
                      if (nUBatch < 512) setNUBatch(512);
                    }}
                    disabled={!!session}
                  >
                    Apply Recommended Settings
                  </button>
                )}
              </div>
            </div>
          )}

          <h4>Configuration</h4>
          <div className="playground-config-grid-fluid">
            <div className="form-group">
              <FieldLabel htmlFor="pg-context-window" tooltipKey="contextWindow">Context Window</FieldLabel>
              <input
                id="pg-context-window"
                type="number"
                value={contextWindow}
                onChange={(e) => setContextWindow(Number(e.target.value))}
                disabled={!!session}
                max={contextInfo?.max}
              />
              {contextInfo && contextInfo.hasRoPE && (
                <div style={{ fontSize: '11px', color: 'var(--color-gray-500)', marginTop: 2 }}>
                  {formatContextHint(contextInfo)}
                </div>
              )}
            </div>
            <div className="form-group">
              <FieldLabel htmlFor="pg-nbatch" tooltipKey="nbatch">NBatch</FieldLabel>
              <input
                id="pg-nbatch"
                type="number"
                value={nBatch}
                onChange={(e) => setNBatch(Number(e.target.value))}
                disabled={!!session}
              />
            </div>
            <div className="form-group">
              <FieldLabel htmlFor="pg-nubatch" tooltipKey="nubatch">NUBatch</FieldLabel>
              <input
                id="pg-nubatch"
                type="number"
                value={nUBatch}
                onChange={(e) => setNUBatch(Number(e.target.value))}
                disabled={!!session}
              />
            </div>
            <div className="form-group">
              <FieldLabel htmlFor="pg-nseqmax" tooltipKey="nSeqMax">NSeqMax</FieldLabel>
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
              <FieldLabel htmlFor="pg-flash-attn" tooltipKey="flashAttention">Flash Attention</FieldLabel>
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
              <FieldLabel htmlFor="pg-cache-type" tooltipKey="cacheType">KV Cache Type</FieldLabel>
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
              <FieldLabel tooltipKey="cacheMode">Cache Mode</FieldLabel>
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
            {isMoE && (
              <div className="form-group">
                <FieldLabel htmlFor="pg-moe-mode" tooltipKey="moeMode">Expert Strategy</FieldLabel>
                <select
                  id="pg-moe-mode"
                  value={moeMode}
                  onChange={(e) => { moeModeTouchedRef.current = true; setMoeMode(e.target.value); }}
                  disabled={!!session}
                >
                  {MOE_STRATEGY_OPTIONS.map(opt => (
                    <option key={opt.value} value={opt.value}>{opt.label}</option>
                  ))}
                </select>
                {vramFitStatus && devicesInfo && (
                  <div className={`playground-vram-fit playground-vram-fit--${vramFitStatus}`}>
                    {VRAM_FIT_TEXT[vramFitStatus]}
                    <span className="playground-vram-fit-detail">
                      {' '}({formatBytes(devicesInfo.gpuVramBytes)} available
                      {vramFitStatus !== 'fits' && `, ${formatBytes(vramNeededCPUExperts)} needed with CPU experts`}
                      {vramFitStatus === 'fits' && `, ${formatBytes(vramNeededAllGPU)} needed`})
                    </span>
                  </div>
                )}
              </div>
            )}
            {isMoE && moeMode === 'keep_top_n' && (
              <div className="form-group">
                <FieldLabel htmlFor="pg-moe-topn" tooltipKey="moeKeepExpertsTopN">GPU Expert Layers ({moeKeepTopN} of {moeBlockCount || '?'} — more = faster, more VRAM)</FieldLabel>
                <input
                  id="pg-moe-topn"
                  type="range"
                  value={moeKeepTopN}
                  onChange={(e) => setMoeKeepTopN(Number(e.target.value))}
                  min={0}
                  max={moeBlockCount || 64}
                  disabled={!!session}
                />
              </div>
            )}
          </div>
          {isMoE && (moeMode === 'experts_cpu' || moeMode === 'keep_top_n') && (
            <div className="playground-moe-tips" style={{ fontSize: '0.85em', color: 'var(--color-text-secondary, #aaa)', padding: '0 0 8px', lineHeight: 1.4 }}>
              💡 {PARAM_TOOLTIPS.moeTipBatch} {PARAM_TOOLTIPS.moeTipFlashAttention}
            </div>
          )}

          {isMoE && moeMode === 'custom' && (
            <div className="playground-moe-custom">
              <div className="form-group">
                <label>Tensor Buffer Overrides</label>
                <p style={{ fontSize: '0.8em', color: 'var(--color-text-secondary, #aaa)', margin: '0 0 8px' }}>
                  Select which tensors to offload to CPU. These are applied as tensor-buft-overrides.
                </p>
              </div>

              <div className="playground-moe-shortcuts">
                <label className="playground-moe-shortcut">
                  <input
                    type="checkbox"
                    checked={tensorBuftOverrides.includes('moe-experts')}
                    onChange={(e) => {
                      setTensorBuftOverrides(prev =>
                        e.target.checked
                          ? [...prev.filter(s => s !== 'moe-experts'), 'moe-experts']
                          : prev.filter(s => s !== 'moe-experts')
                      );
                    }}
                    disabled={!!session}
                  />
                  All expert FFN tensors → CPU
                  <span className="playground-moe-shortcut-hint">moe-experts</span>
                </label>

                <label className="playground-moe-shortcut">
                  <input
                    type="checkbox"
                    checked={tensorBuftOverrides.includes('all-ffn')}
                    onChange={(e) => {
                      setTensorBuftOverrides(prev =>
                        e.target.checked
                          ? [...prev.filter(s => s !== 'all-ffn'), 'all-ffn']
                          : prev.filter(s => s !== 'all-ffn')
                      );
                    }}
                    disabled={!!session}
                  />
                  All FFN tensors → CPU
                  <span className="playground-moe-shortcut-hint">all-ffn</span>
                </label>
              </div>

              {moeBlockCount > 0 && (
                <div className="form-group" style={{ marginTop: 8 }}>
                  <label>Block-Specific Overrides</label>
                  <div style={{ display: 'flex', gap: 6, alignItems: 'center', flexWrap: 'wrap' }}>
                    <select
                      id="pg-moe-block-select"
                      disabled={!!session}
                      defaultValue=""
                      onChange={(e) => {
                        const val = e.target.value;
                        if (!val) return;
                        setTensorBuftOverrides(prev => prev.includes(val) ? prev : [...prev, val]);
                        e.target.value = '';
                      }}
                    >
                      <option value="">Add block override…</option>
                      {Array.from({ length: moeBlockCount }, (_, i) => (
                        <option key={i} value={`moe-experts:block:${i}`}>
                          Block {i} expert tensors → CPU
                        </option>
                      ))}
                      {Array.from({ length: moeBlockCount }, (_, i) => (
                        <option key={`block-${i}`} value={`block:${i}`}>
                          Block {i} (all tensors) → CPU
                        </option>
                      ))}
                    </select>
                  </div>
                </div>
              )}

              {tensorBuftOverrides.length > 0 && (
                <div className="playground-moe-overrides-list">
                  {tensorBuftOverrides.map((override, i) => (
                    <div key={i} className="playground-moe-override-item">
                      <code>{override}</code>
                      <button
                        type="button"
                        className="playground-moe-override-remove"
                        onClick={() => setTensorBuftOverrides(prev => prev.filter((_, j) => j !== i))}
                        disabled={!!session}
                        title="Remove"
                      >
                        ×
                      </button>
                    </div>
                  ))}
                </div>
              )}

              <div className="form-group" style={{ marginTop: 8 }}>
                <label htmlFor="pg-moe-raw-override">Custom Pattern (regex)</label>
                <div style={{ display: 'flex', gap: 6 }}>
                  <input
                    id="pg-moe-raw-override"
                    type="text"
                    placeholder="e.g., blk\.5\.ffn_.*"
                    disabled={!!session}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        const val = (e.target as HTMLInputElement).value.trim();
                        if (val) {
                          setTensorBuftOverrides(prev => prev.includes(val) ? prev : [...prev, val]);
                          (e.target as HTMLInputElement).value = '';
                        }
                      }
                    }}
                  />
                </div>
              </div>
            </div>
          )}

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
                <ChatPanel
                  messages={chatMessages}
                  setMessages={setChatMessages}
                  onClear={handlePlaygroundClear}
                  sampling={playgroundSampling}
                  setSampling={setPlaygroundSampling}
                  modelBaseline={null}
                  transport={playgroundTransport}
                  modelVRAM={modelVramInfo}
                  devicesInfo={devicesInfo}
                  moeFit={moeFit}
                  disabled={!session}
                  disabledPlaceholder="Create a session to start chatting"
                  headerLeft={<h2>Basic Chat</h2>}
                />
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
