import { useState, useEffect, useRef, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import { useDownload } from '../contexts/DownloadContext';
import type {
  PlaygroundTemplateInfo,
  PlaygroundSessionResponse,
  ChatMessage,
  ChatStreamResponse,
  ChatToolCall,
  ModelConfig,
} from '../types';
import AutomatedTestingPanel from './AutomatedTestingPanel';

type PlaygroundTab = 'chat' | 'tools' | 'inspector' | 'autotest';

const NEW_MODEL_VALUE = '__new__';

const defaultTools = JSON.stringify([
  {
    type: 'function',
    function: {
      name: 'get_weather',
      description: 'Get current weather for a city',
      parameters: {
        type: 'object',
        properties: {
          location: { type: 'string', description: 'City name' },
          unit: { type: 'string', enum: ['celsius', 'fahrenheit'] },
        },
        required: ['location'],
      },
    },
  },
  {
    type: 'function',
    function: {
      name: 'add',
      description: 'Add two numbers',
      parameters: {
        type: 'object',
        properties: {
          a: { type: 'number' },
          b: { type: 'number' },
        },
        required: ['a', 'b'],
      },
    },
  },
], null, 2);

interface PlaygroundMessage {
  role: 'user' | 'assistant' | 'system';
  content: string;
}

export default function ModelPlayground() {
  const navigate = useNavigate();
  const { models, loadModels } = useModelList();
  const { download, isDownloading, startDownload, cancelDownload, clearDownload } = useDownload();
  const messagesEndRef = useRef<HTMLDivElement>(null);

  // Setup state
  const [selectedModel, setSelectedModel] = useState('');
  const [templateMode, setTemplateMode] = useState<'builtin' | 'custom'>('builtin');
  const [templates, setTemplates] = useState<PlaygroundTemplateInfo[]>([]);
  const [selectedTemplate, setSelectedTemplate] = useState('');
  const [customScript, setCustomScript] = useState('');
  const [contextWindow, setContextWindow] = useState(8192);
  const [nBatch, setNBatch] = useState(2048);
  const [nUBatch, setNUBatch] = useState(512);
  const [nSeqMax, setNSeqMax] = useState(1);
  const [flashAttention, setFlashAttention] = useState('auto');
  const [cacheType, setCacheType] = useState('');
  const [systemPromptCache, setSystemPromptCache] = useState(false);

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
  const [enableThinking, setEnableThinking] = useState('true');
  const [reasoningEffort, setReasoningEffort] = useState('medium');

  // TPS tracking
  const [lastTPS, setLastTPS] = useState<number | null>(null);

  // Catalog config state
  const [catalogConfig, setCatalogConfig] = useState<ModelConfig | null>(null);
  const [configLoading, setConfigLoading] = useState(false);

  // Session state
  const [session, setSession] = useState<PlaygroundSessionResponse | null>(null);
  const [sessionLoading, setSessionLoading] = useState(false);
  const [sessionError, setSessionError] = useState('');

  // Chat state
  const [activeTab, setActiveTab] = useState<PlaygroundTab>('chat');
  const [systemPrompt, setSystemPrompt] = useState('You are a helpful assistant.');
  const [chatMessages, setChatMessages] = useState<PlaygroundMessage[]>([]);
  const [userInput, setUserInput] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [streamAbort, setStreamAbort] = useState<(() => void) | null>(null);

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

  useEffect(() => {
    loadModels();
    loadTemplates();
  }, []);

  useEffect(() => {
    if (!selectedModel || selectedModel === NEW_MODEL_VALUE) {
      setCatalogConfig(null);
      return;
    }

    setConfigLoading(true);
    api.showModel(selectedModel)
      .then((info) => {
        const mc = info.model_config;
        if (mc) {
          setCatalogConfig(mc);
          setContextWindow(mc['context-window'] || 8192);
          setNBatch(mc.nbatch || 2048);
          setNUBatch(mc.nubatch || 512);
          setNSeqMax(mc['nseq-max'] || 1);
          setFlashAttention(mc['flash-attention'] || 'enabled');
          setCacheType(mc['cache-type-k'] || mc['cache-type-v'] || '');
          setSystemPromptCache(mc['system-prompt-cache'] || false);
        }
      })
      .catch(() => {
        // Model info may not be available; keep current defaults.
      })
      .finally(() => setConfigLoading(false));
  }, [selectedModel]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [chatMessages]);

  const loadTemplates = async () => {
    try {
      const list = await api.listPlaygroundTemplates();
      setTemplates(list);
    } catch {
      // Templates may not be available yet
    }
  };

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
        config['context-window'] = contextWindow;
      }
      if (!catalogConfig || nBatch !== (catalogConfig.nbatch || 2048)) {
        config['nbatch'] = nBatch;
      }
      if (!catalogConfig || nUBatch !== (catalogConfig.nubatch || 512)) {
        config['nubatch'] = nUBatch;
      }
      if (!catalogConfig || nSeqMax !== (catalogConfig['nseq-max'] || 1)) {
        config['nseq-max'] = nSeqMax;
      }
      if (!catalogConfig || flashAttention !== (catalogConfig['flash-attention'] || 'enabled')) {
        config['flash-attention'] = flashAttention;
      }
      if (!catalogConfig || cacheType !== (catalogConfig['cache-type-k'] || '')) {
        if (cacheType) {
          config['cache-type-k'] = cacheType;
          config['cache-type-v'] = cacheType;
        }
      }
      if (!catalogConfig || systemPromptCache !== (catalogConfig['system-prompt-cache'] || false)) {
        config['system-prompt-cache'] = systemPromptCache;
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

    try {
      await api.deletePlaygroundSession(session.session_id);
      setSession(null);
      setChatMessages([]);
    } catch (err: any) {
      setSessionError(err.message || 'Failed to unload session');
    }
  };

  const handleSendMessage = useCallback(() => {
    if (!session || !userInput.trim() || streaming) return;

    const messages: ChatMessage[] = [];
    if (systemPrompt.trim()) {
      messages.push({ role: 'system', content: systemPrompt });
    }
    for (const msg of chatMessages) {
      messages.push({ role: msg.role, content: msg.content });
    }
    messages.push({ role: 'user', content: userInput });

    const newMessages: PlaygroundMessage[] = [
      ...chatMessages,
      { role: 'user', content: userInput },
    ];
    setChatMessages(newMessages);
    setUserInput('');
    setStreaming(true);

    let assistantContent = '';

    const abort = api.streamPlaygroundChat(
      {
        session_id: session.session_id,
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
          const updatedContent = assistantContent;
          setChatMessages(() => [
            ...newMessages,
            { role: 'assistant', content: updatedContent },
          ]);
        }
        if (data.usage?.tokens_per_second) {
          setLastTPS(data.usage.tokens_per_second);
        }
      },
      (error: string) => {
        setChatMessages(() => [
          ...newMessages,
          { role: 'assistant', content: `Error: ${error}` },
        ]);
        setStreaming(false);
      },
      () => {
        setStreaming(false);
      }
    );

    setStreamAbort(() => abort);
  }, [session, userInput, streaming, systemPrompt, chatMessages, temperature, topP, topK, minP, maxTokens, repeatPenalty, repeatLastN, frequencyPenalty, presencePenalty, dryMultiplier, dryBase, dryAllowedLength, dryPenaltyLastN, xtcProbability, xtcThreshold, xtcMinKeep, enableThinking, reasoningEffort]);

  const handleStopStreaming = () => {
    streamAbort?.();
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

    api.streamPlaygroundChat(
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
        setToolResult(`Error: ${error}`);
        setToolTestRunning(false);
      },
      () => {
        setToolResult(fullContent);
        if (collectedToolCalls.length > 0) {
          setToolCalls([...collectedToolCalls]);
        }
        setToolTestRunning(false);
      }
    );
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
        'system-prompt-cache': systemPromptCache,
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
        {/* Setup Panel */}
        <div className="playground-setup">
          <h3>Setup</h3>

          <div className="form-group">
            <label>Model</label>
            <select
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
            <label>Template Mode</label>
            <select
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
              <label>Template</label>
              <select
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
              <label>Template Script</label>
              <textarea
                value={customScript}
                onChange={(e) => setCustomScript(e.target.value)}
                disabled={!!session}
                rows={8}
                className="playground-textarea"
                placeholder="Paste Jinja template..."
              />
            </div>
          )}

          <h4>Configuration</h4>
          <div className="playground-config-grid-fluid">
            <div className="form-group">
              <label>Context Window</label>
              <input
                type="number"
                value={contextWindow}
                onChange={(e) => setContextWindow(Number(e.target.value))}
                disabled={!!session}
              />
            </div>
            <div className="form-group">
              <label>NBatch</label>
              <input
                type="number"
                value={nBatch}
                onChange={(e) => setNBatch(Number(e.target.value))}
                disabled={!!session}
              />
            </div>
            <div className="form-group">
              <label>NUBatch</label>
              <input
                type="number"
                value={nUBatch}
                onChange={(e) => setNUBatch(Number(e.target.value))}
                disabled={!!session}
              />
            </div>
            <div className="form-group">
              <label>NSeqMax</label>
              <input
                type="number"
                value={nSeqMax}
                onChange={(e) => setNSeqMax(Number(e.target.value))}
                min={1}
                disabled={!!session}
              />
            </div>
            <div className="form-group">
              <label>Flash Attention</label>
              <select
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
              <label>KV Cache Type</label>
              <select
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
            <div className="form-group checkbox-group">
              <label>
                <input
                  type="checkbox"
                  checked={systemPromptCache}
                  onChange={(e) => setSystemPromptCache(e.target.checked)}
                  disabled={!!session}
                />
                System Prompt Cache
              </label>
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
          <div className="playground-tabs">
            <button
              className={`playground-tab ${activeTab === 'chat' ? 'active' : ''}`}
              onClick={() => setActiveTab('chat')}
            >
              Basic Chat
            </button>
            <button
              className={`playground-tab ${activeTab === 'tools' ? 'active' : ''}`}
              onClick={() => setActiveTab('tools')}
            >
              Tool Calling Test
            </button>
            <button
              className={`playground-tab ${activeTab === 'inspector' ? 'active' : ''}`}
              onClick={() => setActiveTab('inspector')}
            >
              Prompt Inspector
            </button>
            <button
              className={`playground-tab ${activeTab === 'autotest' ? 'active' : ''}`}
              onClick={() => setActiveTab('autotest')}
            >
              Automated Testing
            </button>
          </div>

          <div className="playground-tab-content">
            {activeTab === 'chat' && (
              <div className="playground-chat">
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
                      <label>Temperature</label>
                      <input type="number" value={temperature} onChange={(e) => setTemperature(Number(e.target.value))} step={0.1} min={0} />
                    </div>
                    <div className="form-group">
                      <label>Top P</label>
                      <input type="number" value={topP} onChange={(e) => setTopP(Number(e.target.value))} step={0.05} min={0} max={1} />
                    </div>
                    <div className="form-group">
                      <label>Top K</label>
                      <input type="number" value={topK} onChange={(e) => setTopK(Math.floor(Number(e.target.value)))} step={1} min={0} />
                    </div>
                    <div className="form-group">
                      <label>Min P</label>
                      <input type="number" value={minP} onChange={(e) => setMinP(Number(e.target.value))} step={0.01} min={0} max={1} />
                    </div>
                    <div className="form-group">
                      <label>Max Tokens</label>
                      <input type="number" value={maxTokens} onChange={(e) => setMaxTokens(Math.floor(Number(e.target.value)))} step={256} min={1} />
                    </div>
                  </div>

                  <h5 className="playground-param-group-title">Repetition Control</h5>
                  <div className="playground-config-grid-fluid">
                    <div className="form-group">
                      <label>Repeat Penalty</label>
                      <input type="number" value={repeatPenalty} onChange={(e) => setRepeatPenalty(Number(e.target.value))} step={0.1} min={0} />
                    </div>
                    <div className="form-group">
                      <label>Repeat Last N</label>
                      <input type="number" value={repeatLastN} onChange={(e) => setRepeatLastN(Math.floor(Number(e.target.value)))} step={1} min={0} />
                    </div>
                    <div className="form-group">
                      <label>Frequency Penalty</label>
                      <input type="number" value={frequencyPenalty} onChange={(e) => setFrequencyPenalty(Number(e.target.value))} step={0.1} min={0} />
                    </div>
                    <div className="form-group">
                      <label>Presence Penalty</label>
                      <input type="number" value={presencePenalty} onChange={(e) => setPresencePenalty(Number(e.target.value))} step={0.1} min={0} />
                    </div>
                  </div>

                  <h5 className="playground-param-group-title">DRY Sampler</h5>
                  <div className="playground-config-grid-fluid">
                    <div className="form-group">
                      <label>DRY Multiplier</label>
                      <input type="number" value={dryMultiplier} onChange={(e) => setDryMultiplier(Number(e.target.value))} step={0.05} min={0} />
                    </div>
                    <div className="form-group">
                      <label>DRY Base</label>
                      <input type="number" value={dryBase} onChange={(e) => setDryBase(Number(e.target.value))} step={0.05} min={0} />
                    </div>
                    <div className="form-group">
                      <label>DRY Allowed Length</label>
                      <input type="number" value={dryAllowedLength} onChange={(e) => setDryAllowedLength(Math.floor(Number(e.target.value)))} step={1} min={0} />
                    </div>
                    <div className="form-group">
                      <label>DRY Penalty Last N</label>
                      <input type="number" value={dryPenaltyLastN} onChange={(e) => setDryPenaltyLastN(Math.floor(Number(e.target.value)))} step={1} min={0} />
                    </div>
                  </div>

                  <h5 className="playground-param-group-title">XTC Sampler</h5>
                  <div className="playground-config-grid-fluid">
                    <div className="form-group">
                      <label>XTC Probability</label>
                      <input type="number" value={xtcProbability} onChange={(e) => setXtcProbability(Number(e.target.value))} step={0.01} min={0} max={1} />
                    </div>
                    <div className="form-group">
                      <label>XTC Threshold</label>
                      <input type="number" value={xtcThreshold} onChange={(e) => setXtcThreshold(Number(e.target.value))} step={0.01} min={0} max={1} />
                    </div>
                    <div className="form-group">
                      <label>XTC Min Keep</label>
                      <input type="number" value={xtcMinKeep} onChange={(e) => setXtcMinKeep(Math.floor(Number(e.target.value)))} step={1} min={1} />
                    </div>
                  </div>

                  <h5 className="playground-param-group-title">Reasoning</h5>
                  <div className="playground-config-grid-fluid">
                    <div className="form-group">
                      <label>Enable Thinking</label>
                      <select value={enableThinking} onChange={(e) => setEnableThinking(e.target.value)}>
                        <option value="true">Enabled</option>
                        <option value="false">Disabled</option>
                      </select>
                    </div>
                    <div className="form-group">
                      <label>Reasoning Effort</label>
                      <select value={reasoningEffort} onChange={(e) => setReasoningEffort(e.target.value)}>
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
                    <div key={i} className={`playground-message playground-message-${msg.role}`}>
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
              <div className="playground-tools">
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
              <div className="playground-inspector">
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

            {activeTab === 'autotest' && (
              <AutomatedTestingPanel
                session={session}
                sessionSeed={{
                  model_id: selectedModel,
                  template_mode: templateMode,
                  template_name: templateMode === 'builtin' ? selectedTemplate : undefined,
                  template_script: templateMode === 'custom' ? customScript : undefined,
                  base_config: {
                    'context-window': contextWindow,
                    nbatch: nBatch,
                    nubatch: nUBatch,
                    'nseq-max': nSeqMax,
                    'flash-attention': flashAttention,
                    'cache-type-k': cacheType || undefined,
                    'cache-type-v': cacheType || undefined,
                    'system-prompt-cache': systemPromptCache,
                  },
                }}
              />
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
