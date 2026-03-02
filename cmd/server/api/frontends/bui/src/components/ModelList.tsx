import { useState, useEffect, useRef } from 'react';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import type { ModelInfoResponse } from '../types';
import { formatBytes, fmtNum, fmtVal } from '../lib/format';
import KeyValueTable from './KeyValueTable';
import MetadataSection from './MetadataSection';
import CodeBlock from './CodeBlock';
import ModelSelector from './ModelSelector';
import { VRAMFormulaModal, VRAMControls, VRAMResults, calculateVRAM } from './vram';

type ModelListSection = 'config' | 'sampling' | 'metadata' | 'template' | 'vram';

const SECTION_LABELS: Record<ModelListSection, string> = {
  config: 'Model Configuration',
  sampling: 'Sampling Parameters',
  metadata: 'Metadata',
  template: 'Template',
  vram: 'VRAM Calculator',
};

export default function ModelList() {
  const { models, loading, error, loadModels, invalidate } = useModelList();
  const [selectedModelId, setSelectedModelId] = useState<string | null>(null);
  const [modelInfo, setModelInfo] = useState<ModelInfoResponse | null>(null);
  const [infoLoading, setInfoLoading] = useState(false);
  const [infoError, setInfoError] = useState<string | null>(null);
  const [activeSection, setActiveSection] = useState<ModelListSection>('config');

  const [rebuildingIndex, setRebuildingIndex] = useState(false);
  const [rebuildError, setRebuildError] = useState<string | null>(null);
  const [rebuildSuccess, setRebuildSuccess] = useState(false);

  const [confirmingRemove, setConfirmingRemove] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [removeError, setRemoveError] = useState<string | null>(null);
  const [removeSuccess, setRemoveSuccess] = useState<string | null>(null);

  // VRAM calculator local state
  const [vramCtx, setVramCtx] = useState(8192);
  const [vramBytes, setVramBytes] = useState(1);
  const [vramSlots, setVramSlots] = useState(2);
  const [showLearnMore, setShowLearnMore] = useState(false);

  // Timeout refs for cleanup
  const rebuildTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const removeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (rebuildTimerRef.current) clearTimeout(rebuildTimerRef.current);
      if (removeTimerRef.current) clearTimeout(removeTimerRef.current);
    };
  }, []);

  useEffect(() => {
    loadModels();
  }, [loadModels]);

  // Fetch model info when selection changes
  useEffect(() => {
    if (!selectedModelId) {
      setModelInfo(null);
      setInfoError(null);
      return;
    }

    let cancelled = false;
    setInfoLoading(true);
    setInfoError(null);
    setModelInfo(null);

    api.showModel(selectedModelId)
      .then((resp) => { if (!cancelled) setModelInfo(resp); })
      .catch((err) => { if (!cancelled) setInfoError(err?.message ?? 'Failed to load model info'); })
      .finally(() => { if (!cancelled) setInfoLoading(false); });

    return () => { cancelled = true; };
  }, [selectedModelId]);

  // Seed VRAM calculator from model info
  const vramInputRef = modelInfo?.vram?.input;
  useEffect(() => {
    if (vramInputRef) {
      setVramCtx(vramInputRef.context_window);
      setVramBytes(vramInputRef.bytes_per_element);
      setVramSlots(vramInputRef.slots);
    }
  }, [vramInputRef]);

  const handleModelSelect = (id: string) => {
    setSelectedModelId(id || null);
    setActiveSection('config');
    setConfirmingRemove(false);
    setRemoveError(null);
    setRemoveSuccess(null);
  };

  const handleRebuildIndex = async () => {
    setRebuildingIndex(true);
    setRebuildError(null);
    setRebuildSuccess(false);
    try {
      await api.rebuildModelIndex();
      invalidate();
      loadModels();
      setSelectedModelId(null);
      setModelInfo(null);
      setRebuildSuccess(true);
      rebuildTimerRef.current = setTimeout(() => setRebuildSuccess(false), 3000);
    } catch (err) {
      setRebuildError(err instanceof Error ? err.message : 'Failed to rebuild index');
    } finally {
      setRebuildingIndex(false);
    }
  };

  const handleRemoveClick = () => {
    if (!selectedModelId) return;
    setConfirmingRemove(true);
  };

  const handleConfirmRemove = async () => {
    if (!selectedModelId) return;

    setRemoving(true);
    setConfirmingRemove(false);
    setRemoveError(null);
    setRemoveSuccess(null);

    try {
      await api.removeModel(selectedModelId);
      setRemoveSuccess(`Model "${selectedModelId}" removed successfully`);
      setSelectedModelId(null);
      setModelInfo(null);
      invalidate();
      await loadModels();
      removeTimerRef.current = setTimeout(() => setRemoveSuccess(null), 3000);
    } catch (err) {
      setRemoveError(err instanceof Error ? err.message : 'Failed to remove model');
    } finally {
      setRemoving(false);
    }
  };

  const handleCancelRemove = () => {
    setConfirmingRemove(false);
  };

  // Compute VRAM locally from model header data
  const vramInput = modelInfo?.vram?.input;
  const vramResult = vramInput
    ? calculateVRAM({ ...vramInput, context_window: vramCtx, bytes_per_element: vramBytes, slots: vramSlots })
    : null;

  // Look up validated flag from list data
  const selectedListModel = models?.data?.find((m) => m.id === selectedModelId);

  const sections = Object.keys(SECTION_LABELS) as ModelListSection[];

  return (
    <div>
      <div className="page-header">
        <h2>Models</h2>
        <p>Select a model to view its configuration and details.</p>
      </div>

      {error && <div className="alert alert-error">{error}</div>}
      {removeError && <div className="alert alert-error">{removeError}</div>}
      {removeSuccess && <div className="alert alert-success">{removeSuccess}</div>}
      {rebuildError && <div className="alert alert-error">{rebuildError}</div>}
      {rebuildSuccess && <div className="alert alert-success">Index rebuilt successfully</div>}

      <div className="playground-layout">
        {/* Left Sidebar */}
        <div className="playground-mode-selector">
          <div className="playground-model-config">
            <div className="form-group">
              <label>Model</label>
              <ModelSelector
                models={models?.data}
                selectedModel={selectedModelId}
                onSelect={handleModelSelect}
                disabled={loading}
              />
            </div>
          </div>

          {sections.map((section) => (
            <button
              key={section}
              className={`playground-mode-btn ${activeSection === section ? 'active' : ''}`}
              onClick={() => setActiveSection(section)}
              disabled={!selectedModelId}
            >
              {SECTION_LABELS[section]}
            </button>
          ))}

          <div style={{ marginTop: '16px', display: 'flex', flexDirection: 'column', gap: '8px' }}>
            <button
              className="btn btn-secondary"
              onClick={() => {
                invalidate();
                loadModels();
              }}
              disabled={loading}
            >
              Refresh
            </button>
            <button
              className="btn btn-secondary"
              onClick={handleRebuildIndex}
              disabled={rebuildingIndex || loading}
            >
              {rebuildingIndex ? 'Rebuilding...' : 'Rebuild Index'}
            </button>
            {selectedModelId && !confirmingRemove && (
              <button
                className="btn btn-danger"
                onClick={handleRemoveClick}
                disabled={removing}
              >
                Remove Model
              </button>
            )}
            {selectedModelId && confirmingRemove && (
              <>
                <button className="btn btn-danger" onClick={handleConfirmRemove} disabled={removing}>
                  {removing ? 'Removing...' : 'Yes, Remove'}
                </button>
                <button className="btn btn-secondary" onClick={handleCancelRemove} disabled={removing}>
                  Cancel
                </button>
              </>
            )}
          </div>
        </div>

        {/* Main Content */}
        <div className="playground-test" style={{ flex: 1 }}>
          <div className="playground-tab-content" style={{ overflow: 'auto'}}>
            {loading && <div className="loading">Loading models</div>}

            {!selectedModelId && !loading && (
              <div className="empty-state">
                <h3>Select a model</h3>
                <p>Choose a model from the dropdown to view its details.</p>
              </div>
            )}

            {infoLoading && (
              <div className="loading">Loading model details</div>
            )}

            {infoError && <div className="alert alert-error">{infoError}</div>}

            {/* Model Configuration Section */}
            {modelInfo && !infoLoading && activeSection === 'config' && (
              <div>
                <h3 style={{ marginBottom: '16px' }}>{selectedModelId ?? modelInfo.id}</h3>

                {modelInfo.desc && (
                  <div style={{ marginBottom: '16px' }}>
                    <p>{modelInfo.desc}</p>
                  </div>
                )}

                <KeyValueTable rows={[
                  { key: 'owner', label: 'Owner', value: modelInfo.owned_by },
                  { key: 'size', label: 'Size', value: formatBytes(modelInfo.size) },
                  { key: 'created', label: 'Created', value: new Date(modelInfo.created).toLocaleString() },
                  { key: 'projection', label: 'Has Projection', value: <span className={`badge ${modelInfo.has_projection ? 'badge-yes' : 'badge-no'}`}>{modelInfo.has_projection ? 'Yes' : 'No'}</span> },
                  { key: 'gpt', label: 'Is GPT', value: <span className={`badge ${modelInfo.is_gpt ? 'badge-yes' : 'badge-no'}`}>{modelInfo.is_gpt ? 'Yes' : 'No'}</span> },
                  ...(selectedListModel ? [{ key: 'validated' as const, label: 'Validated', value: <span style={{ color: selectedListModel.validated ? 'inherit' : 'var(--color-error)' }}>{selectedListModel.validated ? '✓' : '✗'}</span> }] : []),
                ]} />

                {modelInfo.model_config && (
                  <div style={{ marginTop: '24px' }}>
                    <h4 className="meta-section-title" style={{ marginBottom: '8px' }}>Configuration</h4>
                    <KeyValueTable rows={[
                      { key: 'device', label: 'Device', value: modelInfo.model_config.device || 'default' },
                      { key: 'ctx', label: 'Context Window', value: fmtVal(modelInfo.model_config['context-window']) },
                      { key: 'nbatch', label: 'Batch Size', value: fmtVal(modelInfo.model_config.nbatch) },
                      { key: 'nubatch', label: 'Micro Batch Size', value: fmtVal(modelInfo.model_config.nubatch) },
                      { key: 'nthreads', label: 'Threads', value: fmtVal(modelInfo.model_config.nthreads) },
                      { key: 'nthreads-batch', label: 'Batch Threads', value: fmtVal(modelInfo.model_config['nthreads-batch']) },
                      { key: 'cache-k', label: 'Cache Type K', value: modelInfo.model_config['cache-type-k'] || 'default' },
                      { key: 'cache-v', label: 'Cache Type V', value: modelInfo.model_config['cache-type-v'] || 'default' },
                      { key: 'flash', label: 'Flash Attention', value: modelInfo.model_config['flash-attention'] || 'default' },
                      { key: 'nseq', label: 'Max Sequences', value: fmtVal(modelInfo.model_config['nseq-max']) },
                      { key: 'ngpu', label: 'GPU Layers', value: fmtVal(modelInfo.model_config['ngpu-layers'] ?? 'auto') },
                      { key: 'split', label: 'Split Mode', value: modelInfo.model_config['split-mode'] || 'default' },
                      { key: 'spc', label: 'System Prompt Cache', value: <span className={`badge ${modelInfo.model_config['system-prompt-cache'] ? 'badge-yes' : 'badge-no'}`}>{modelInfo.model_config['system-prompt-cache'] ? 'Yes' : 'No'}</span> },
                      { key: 'imc', label: 'Incremental Cache', value: <span className={`badge ${modelInfo.model_config['incremental-cache'] ? 'badge-yes' : 'badge-no'}`}>{modelInfo.model_config['incremental-cache'] ? 'Yes' : 'No'}</span> },
                      ...(!!modelInfo.model_config['rope-scaling-type'] && modelInfo.model_config['rope-scaling-type'] !== 'none' ? [
                        { key: 'rope-scaling', label: 'RoPE Scaling', value: modelInfo.model_config['rope-scaling-type'] },
                        { key: 'yarn-orig', label: 'YaRN Original Context', value: fmtVal(modelInfo.model_config['yarn-orig-ctx'] ?? 'auto') },
                        ...(modelInfo.model_config['rope-freq-base'] != null ? [{ key: 'rope-freq', label: 'RoPE Freq Base', value: fmtVal(modelInfo.model_config['rope-freq-base']) }] : []),
                        ...(modelInfo.model_config['yarn-ext-factor'] != null ? [{ key: 'yarn-ext', label: 'YaRN Ext Factor', value: fmtVal(modelInfo.model_config['yarn-ext-factor']) }] : []),
                        ...(modelInfo.model_config['yarn-attn-factor'] != null ? [{ key: 'yarn-attn', label: 'YaRN Attn Factor', value: fmtVal(modelInfo.model_config['yarn-attn-factor']) }] : []),
                      ] : []),
                      ...(modelInfo.model_config['draft-model'] ? [
                        { key: 'draft-model', label: 'Draft Model', value: modelInfo.model_config['draft-model']['model-id'] },
                        { key: 'draft-tokens', label: 'Draft Tokens', value: fmtVal(modelInfo.model_config['draft-model'].ndraft) },
                      ] : []),
                    ]} />
                  </div>
                )}
              </div>
            )}

            {/* Sampling Parameters Section */}
            {modelInfo && !infoLoading && activeSection === 'sampling' && (
              <div>
                <h3 style={{ marginBottom: '16px' }}>Sampling Parameters</h3>
                {modelInfo.model_config?.['sampling-parameters'] ? (() => {
                  const sp = modelInfo.model_config['sampling-parameters'];
                  return (
                    <KeyValueTable rows={[
                      { key: 'temperature', label: 'Temperature', value: fmtNum(sp.temperature) },
                      { key: 'top_k', label: 'Top K', value: fmtVal(sp.top_k) },
                      { key: 'top_p', label: 'Top P', value: fmtNum(sp.top_p) },
                      { key: 'min_p', label: 'Min P', value: fmtNum(sp.min_p) },
                      { key: 'max_tokens', label: 'Max Tokens', value: fmtVal(sp.max_tokens) },
                      { key: 'repeat_penalty', label: 'Repeat Penalty', value: fmtNum(sp.repeat_penalty) },
                      { key: 'repeat_last_n', label: 'Repeat Last N', value: fmtVal(sp.repeat_last_n) },
                      { key: 'freq_penalty', label: 'Frequency Penalty', value: fmtNum(sp.frequency_penalty) },
                      { key: 'pres_penalty', label: 'Presence Penalty', value: fmtNum(sp.presence_penalty) },
                      { key: 'dry_mult', label: 'DRY Multiplier', value: fmtVal(sp.dry_multiplier) },
                      { key: 'dry_base', label: 'DRY Base', value: fmtVal(sp.dry_base) },
                      { key: 'dry_len', label: 'DRY Allowed Length', value: fmtVal(sp.dry_allowed_length) },
                      { key: 'dry_last', label: 'DRY Penalty Last N', value: fmtVal(sp.dry_penalty_last_n) },
                      { key: 'xtc_prob', label: 'XTC Probability', value: fmtVal(sp.xtc_probability) },
                      { key: 'xtc_thresh', label: 'XTC Threshold', value: fmtVal(sp.xtc_threshold) },
                      { key: 'xtc_keep', label: 'XTC Min Keep', value: fmtVal(sp.xtc_min_keep) },
                      { key: 'thinking', label: 'Enable Thinking', value: fmtVal(sp.enable_thinking ?? 'default') },
                      { key: 'reasoning', label: 'Reasoning Effort', value: fmtVal(sp.reasoning_effort ?? 'default') },
                      ...(sp.grammar ? [{ key: 'grammar', label: 'Grammar', value: sp.grammar }] : []),
                    ]} />
                  );
                })() : (
                  <div className="empty-state">
                    <p>No sampling parameters configured for this model.</p>
                  </div>
                )}
              </div>
            )}

            {/* Metadata Section */}
            {modelInfo && !infoLoading && activeSection === 'metadata' && (
              <div>
                <h3 style={{ marginBottom: '16px' }}>Metadata</h3>
                {modelInfo.metadata && Object.keys(modelInfo.metadata).filter(k => k !== 'tokenizer.chat_template').length > 0 ? (
                  <MetadataSection
                    metadata={modelInfo.metadata}
                    excludeKeys={['tokenizer.chat_template']}
                  />
                ) : (
                  <div className="empty-state">
                    <p>No metadata available for this model.</p>
                  </div>
                )}
              </div>
            )}

            {/* Template Section */}
            {modelInfo && !infoLoading && activeSection === 'template' && (
              <div>
                <h3 style={{ marginBottom: '16px' }}>Chat Template</h3>
                {modelInfo.metadata?.['tokenizer.chat_template'] ? (
                  <CodeBlock
                    code={modelInfo.metadata['tokenizer.chat_template']}
                    language="django"
                  />
                ) : (
                  <div className="empty-state">
                    <p>No chat template found in metadata.</p>
                  </div>
                )}
              </div>
            )}

            {/* VRAM Calculator Section */}
            {modelInfo && !infoLoading && activeSection === 'vram' && (
              <div>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
                  <h3>VRAM Calculator</h3>
                  <button
                    type="button"
                    className="btn btn-secondary"
                    onClick={() => setShowLearnMore(true)}
                  >
                    Learn More
                  </button>
                </div>

                {showLearnMore && <VRAMFormulaModal onClose={() => setShowLearnMore(false)} />}

                {vramInput ? (
                  <>
                    <p style={{ fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: '16px' }}>
                      Computed locally from GGUF header. Adjust parameters below to see how they affect VRAM.
                    </p>

                    <div style={{ marginBottom: '24px' }}>
                      <VRAMControls
                        contextWindow={vramCtx}
                        onContextWindowChange={setVramCtx}
                        bytesPerElement={vramBytes}
                        onBytesPerElementChange={setVramBytes}
                        slots={vramSlots}
                        onSlotsChange={setVramSlots}
                        variant="compact"
                      />
                    </div>

                    <VRAMResults
                      totalVram={vramResult!.totalVram}
                      slotMemory={vramResult!.slotMemory}
                      kvPerSlot={vramResult!.kvPerSlot}
                      kvPerTokenPerLayer={vramResult!.kvPerTokenPerLayer}
                      input={{ ...vramInput!, context_window: vramCtx, bytes_per_element: vramBytes, slots: vramSlots }}
                    />
                  </>
                ) : (
                  <div className="empty-state">
                    <p>No VRAM data available for this model.</p>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
