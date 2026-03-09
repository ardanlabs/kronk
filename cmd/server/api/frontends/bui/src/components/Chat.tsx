import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import { useChatMessages, type DisplayMessage } from '../contexts/ChatContext';
import { useSampling, defaultSampling, type SamplingParams } from '../contexts/SamplingContext';
import ChatHistoryPanel from './ChatHistoryPanel';
import ChatPanel, { type StreamTransport } from './ChatPanel';
import { useChatHistory, type HistoryMessage } from '../contexts/ChatHistoryContext';
import type { SamplingConfig, ListModelDetail, VRAM } from '../types';

const HISTORY_ENABLED_KEY = 'kronk_chat_history_enabled';

export default function Chat() {
  const { models, loading: modelsLoading, loadModels } = useModelList();
  const { messages, setMessages, clearMessages } = useChatMessages();
  const { sampling, setSampling } = useSampling();
  const { saveChat } = useChatHistory();
  const [selectedModel, setSelectedModel] = useState<string>(() => {
    return localStorage.getItem('kronk_chat_model') || '';
  });
  const [showHistory, setShowHistory] = useState(false);
  const [historyEnabled, setHistoryEnabled] = useState<boolean>(() => {
    const stored = localStorage.getItem(HISTORY_ENABLED_KEY);
    return stored === null ? true : stored === 'true';
  });


  const [extendedModels, setExtendedModels] = useState<ListModelDetail[]>([]);
  const [isMoE, setIsMoE] = useState(false);
  const [modelVRAM, setModelVRAM] = useState<VRAM | null>(null);
  const [devicesInfo, setDevicesInfo] = useState<{ gpuCount: number; gpuType: string; gpuVramBytes: number; ramBytes: number } | null>(null);
  const [vramFitStatus, setVramFitStatus] = useState<'fits' | 'tight' | 'wont_fit' | null>(null);
  const [vramNeededAllGPU, setVramNeededAllGPU] = useState(0);
  const [vramNeededCPUExperts, setVramNeededCPUExperts] = useState(0);
  const [modelBaseline, setModelBaseline] = useState<SamplingParams | null>(null);
  const prevModelRef = useRef<string | null>(null);
  const selectedModelDraftId = useMemo(() => {
    if (!selectedModel || !models?.data) return undefined;
    const m = models.data.find((m) => m.id === selectedModel);
    return m?.draft_model_id;
  }, [selectedModel, models]);

  const toggleHistoryEnabled = useCallback(() => {
    setHistoryEnabled((prev) => {
      const next = !prev;
      try { localStorage.setItem(HISTORY_ENABLED_KEY, String(next)); } catch { /* ignore */ }
      return next;
    });
  }, []);

  const toSamplingParams = useCallback((modelSampling: SamplingConfig): SamplingParams => {
    return {
      temperature: modelSampling.temperature ?? defaultSampling.temperature,
      topK: modelSampling.top_k ?? defaultSampling.topK,
      topP: modelSampling.top_p ?? defaultSampling.topP,
      minP: modelSampling.min_p ?? defaultSampling.minP,
      presencePenalty: modelSampling.presence_penalty ?? defaultSampling.presencePenalty,
      maxTokens: modelSampling.max_tokens ?? defaultSampling.maxTokens,
      repeatPenalty: modelSampling.repeat_penalty ?? defaultSampling.repeatPenalty,
      repeatLastN: modelSampling.repeat_last_n ?? defaultSampling.repeatLastN,
      dryMultiplier: modelSampling.dry_multiplier ?? defaultSampling.dryMultiplier,
      dryBase: modelSampling.dry_base ?? defaultSampling.dryBase,
      dryAllowedLen: modelSampling.dry_allowed_length ?? defaultSampling.dryAllowedLen,
      dryPenaltyLast: modelSampling.dry_penalty_last_n ?? defaultSampling.dryPenaltyLast,
      xtcProbability: modelSampling.xtc_probability ?? defaultSampling.xtcProbability,
      xtcThreshold: modelSampling.xtc_threshold ?? defaultSampling.xtcThreshold,
      xtcMinKeep: modelSampling.xtc_min_keep ?? defaultSampling.xtcMinKeep,
      frequencyPenalty: modelSampling.frequency_penalty ?? defaultSampling.frequencyPenalty,
      enableThinking: modelSampling.enable_thinking ?? defaultSampling.enableThinking,
      reasoningEffort: modelSampling.reasoning_effort ?? defaultSampling.reasoningEffort,
      returnPrompt: defaultSampling.returnPrompt,
      includeUsage: defaultSampling.includeUsage,
      logprobs: defaultSampling.logprobs,
      topLogprobs: defaultSampling.topLogprobs,
      grammar: modelSampling.grammar ?? defaultSampling.grammar,
      systemPrompt: defaultSampling.systemPrompt,
      cacheId: defaultSampling.cacheId,
    };
  }, []);

  const applySamplingConfig = useCallback((modelSampling: SamplingConfig | undefined) => {
    if (modelSampling) {
      const params = toSamplingParams(modelSampling);
      const grammarVal = params.grammar;

      if (grammarVal && grammarVal.endsWith('.grm')) {
        api.getGrammarContent(grammarVal)
          .then(res => {
            setSampling({ ...params, grammar: res.content });
            setModelBaseline({ ...params, grammar: res.content });
          })
          .catch(() => {
            setSampling({ ...params, grammar: '' });
            setModelBaseline({ ...params, grammar: '' });
          });
      } else if (grammarVal) {
        setSampling(params);
        setModelBaseline(params);
      } else {
        setSampling(params);
        setModelBaseline(params);
      }
    } else {
      setModelBaseline(null);
    }
  }, [setSampling, toSamplingParams]);

  useEffect(() => {
    loadModels();
    api.listModelsExtended()
      .then((response) => {
        if (response?.data) {
          setExtendedModels(response.data);
        }
      })
      .catch(() => {});
  }, [loadModels]);

  useEffect(() => {
    if (models?.data && models.data.length > 0) {
      const chatModels = models.data.filter((m) => {
        const id = m.id.toLowerCase();
        return !id.includes('embed') && !id.includes('rerank');
      });
      const isCurrentValid = chatModels.some((m) => m.id === selectedModel);
      if (!isCurrentValid && chatModels.length > 0) {
        setSelectedModel(chatModels[0].id);
      }
    }
  }, [models, selectedModel]);

  useEffect(() => {
    if (selectedModel && extendedModels.length > 0) {
      localStorage.setItem('kronk_chat_model', selectedModel);
      if (prevModelRef.current !== selectedModel) {
        const modelDetail = extendedModels.find((m) => m.id === selectedModel);
        applySamplingConfig(modelDetail?.sampling);
      }
      prevModelRef.current = selectedModel;
    }
  }, [selectedModel, extendedModels, applySamplingConfig]);

  useEffect(() => {
    if (!selectedModel) {
      setIsMoE(false);
      setModelVRAM(null);
      return;
    }
    let cancelled = false;
    api.showModel(selectedModel)
      .then((info) => {
        if (cancelled) return;
        const modelIsMoE = info.vram?.moe?.is_moe === true
          || (info.metadata && Object.keys(info.metadata).some(k => k.endsWith('.expert_count')));
        setIsMoE(modelIsMoE);
        setModelVRAM(info.vram || null);
      })
      .catch(() => {
        if (!cancelled) {
          setIsMoE(false);
          setModelVRAM(null);
        }
      });
    return () => { cancelled = true; };
  }, [selectedModel]);

  useEffect(() => {
    let cancelled = false;
    api.getDevices()
      .then((resp) => {
        if (cancelled) return;
        const gpuDevices = resp.devices.filter(d => d.type !== 'cpu' && d.type !== 'unknown');
        const gpuType = gpuDevices.length > 0
          ? gpuDevices[0].type.replace('gpu_', '').replace('cuda', 'CUDA').replace('metal', 'Metal').replace('rocm', 'ROCm').replace('vulkan', 'Vulkan')
          : '';
        setDevicesInfo({ gpuCount: resp.gpu_count, gpuType, gpuVramBytes: resp.gpu_total_bytes, ramBytes: resp.system_ram_bytes });
      })
      .catch(() => {});
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    if (!isMoE || !modelVRAM || !devicesInfo) {
      setVramFitStatus(null);
      setVramNeededAllGPU(0);
      setVramNeededCPUExperts(0);
      return;
    }
    const gpuVram = devicesInfo.gpuVramBytes;
    const vramInfo = modelVRAM;
    const kvPerSlot = vramInfo.input.context_window * vramInfo.input.block_count *
      vramInfo.input.head_count_kv * (vramInfo.input.key_length + vramInfo.input.value_length) * vramInfo.input.bytes_per_element;
    const slotMem = kvPerSlot * vramInfo.input.slots;
    const computeEst = vramInfo.compute_buffer_est ?? 300 * 1024 * 1024;
    const allGPU = vramInfo.input.model_size_bytes + slotMem + computeEst;
    setVramNeededAllGPU(allGPU);
    const activeOnly = vramInfo.weights?.always_active_bytes ?? vramInfo.input.model_size_bytes;
    const cpuExperts = activeOnly + slotMem + computeEst;
    setVramNeededCPUExperts(cpuExperts);
    if (gpuVram <= 0) return;
    if (allGPU <= gpuVram * 0.95) {
      setVramFitStatus('fits');
    } else if (cpuExperts <= gpuVram * 0.95) {
      setVramFitStatus('tight');
    } else {
      setVramFitStatus('wont_fit');
    }
  }, [isMoE, modelVRAM, devicesInfo]);

  const transport = useCallback<StreamTransport>(({ messages, sampling, onMessage, onError, onComplete }) => {
    return api.streamChat(
      {
        model: selectedModel,
        messages,
        max_tokens: sampling.maxTokens,
        temperature: sampling.temperature,
        top_p: sampling.topP,
        top_k: sampling.topK,
        min_p: sampling.minP,
        presence_penalty: sampling.presencePenalty,
        repeat_penalty: sampling.repeatPenalty,
        repeat_last_n: sampling.repeatLastN,
        dry_multiplier: sampling.dryMultiplier,
        dry_base: sampling.dryBase,
        dry_allowed_length: sampling.dryAllowedLen,
        dry_penalty_last_n: sampling.dryPenaltyLast,
        xtc_probability: sampling.xtcProbability,
        xtc_threshold: sampling.xtcThreshold,
        xtc_min_keep: sampling.xtcMinKeep,
        frequency_penalty: sampling.frequencyPenalty,
        enable_thinking: sampling.enableThinking || undefined,
        reasoning_effort: sampling.reasoningEffort || undefined,
        return_prompt: sampling.returnPrompt,
        stream_options: {
          include_usage: sampling.includeUsage,
        },
        logprobs: sampling.logprobs,
        top_logprobs: sampling.topLogprobs,
        grammar: sampling.grammar || undefined,
      },
      onMessage,
      onError,
      onComplete,
      sampling.cacheId || undefined,
    );
  }, [selectedModel]);

  const handleClear = useCallback(() => {
    if (historyEnabled && messages.length > 0) {
      saveChat(selectedModel, messages);
    }
    clearMessages();
  }, [historyEnabled, messages, selectedModel, saveChat, clearMessages]);

  const handleLoadFromHistory = useCallback((historyMessages: HistoryMessage[]) => {
    const displayMessages: DisplayMessage[] = historyMessages.map((m) => ({
      role: m.role,
      content: m.content,
      reasoning: m.reasoning,
    }));
    setMessages(displayMessages);
  }, [setMessages]);

  const handleModelChange = useCallback((e: React.ChangeEvent<HTMLSelectElement>) => {
    const newModel = e.target.value;
    if (messages.length > 0) {
      if (historyEnabled) {
        saveChat(selectedModel, messages);
      }
      clearMessages();
    }
    setSelectedModel(newModel);
  }, [messages, historyEnabled, selectedModel, saveChat, clearMessages]);

  return (
    <div className="chat-layout">
      <ChatHistoryPanel
        isOpen={showHistory}
        onClose={() => setShowHistory(false)}
        onLoadChat={handleLoadFromHistory}
      />
      <ChatPanel
        messages={messages}
        setMessages={setMessages}
        onClear={handleClear}
        sampling={sampling}
        setSampling={setSampling}
        modelBaseline={modelBaseline}
        transport={transport}
        isMoE={isMoE}
        modelVRAM={modelVRAM}
        devicesInfo={devicesInfo}
        vramFitStatus={vramFitStatus}
        vramNeededAllGPU={vramNeededAllGPU}
        vramNeededCPUExperts={vramNeededCPUExperts}
        disabled={!selectedModel}
        headerLeft={
          <>
            <h2>Chat</h2>
            <select
              value={selectedModel}
              onChange={handleModelChange}
              disabled={modelsLoading}
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
                  {model.draft_model_id ? '⚡ ' : ''}{model.id}
                </option>
              ))}
            </select>
            {selectedModelDraftId && (
              <span className="chat-draft-label" title="Draft model for speculative decoding">
                ⚡ {selectedModelDraftId}
              </span>
            )}
          </>
        }
        headerRight={
          <>
            <button
              type="button"
              className={`chat-history-toggle ${historyEnabled ? 'chat-history-toggle-on' : 'chat-history-toggle-off'}`}
              onClick={toggleHistoryEnabled}
              title={historyEnabled ? 'Chat history auto-save is on' : 'Chat history auto-save is off'}
            >
              Chat History: {historyEnabled ? 'ON' : 'OFF'}
            </button>
            <button
              className="btn btn-secondary btn-sm"
              onClick={() => setShowHistory(!showHistory)}
            >
              History
            </button>
          </>
        }
      />
    </div>
  );
}
