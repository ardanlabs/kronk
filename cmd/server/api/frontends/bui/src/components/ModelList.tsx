import { useState, useEffect, useRef } from 'react';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import type { ModelInfoResponse, ListModelDetail } from '../types';
import { formatBytes, fmtNum, fmtVal } from '../lib/format';
import KeyValueTable from './KeyValueTable';
import MetadataSection from './MetadataSection';
import CodeBlock from './CodeBlock';
import { VRAMFormulaModal, VRAMControls, VRAMResults, useVRAMState } from './vram';

type ModelListSection = 'config' | 'sampling' | 'metadata' | 'template' | 'vram';

const SECTION_LABELS: Record<ModelListSection, string> = {
  config: 'Model Configuration',
  sampling: 'Sampling Parameters',
  metadata: 'Metadata',
  template: 'Template',
  vram: 'VRAM Calculator',
};

type SortField = 'id' | 'owner' | 'family' | 'size' | 'modified';

function getSortValue(model: ListModelDetail, field: SortField): string | number {
  switch (field) {
    case 'id': return model.id.toLowerCase();
    case 'owner': return (model.owned_by || '').toLowerCase();
    case 'family': return (model.model_family || '').toLowerCase();
    case 'size': return model.size;
    case 'modified': return new Date(model.modified).getTime();
    default: return '';
  }
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString();
}

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

  // Sort state
  const [sortField, setSortField] = useState<SortField>('id');
  const [sortAsc, setSortAsc] = useState(true);

  // VRAM calculator state (shared hook)
  const vramServerResponse = modelInfo?.vram ?? null;
  const { controlsProps: vramControls, resultsProps: vramResults } = useVRAMState({
    initialContextWindow: 8192,
    initialBytesPerElement: 1,
    serverResponse: vramServerResponse,
  });
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

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortAsc(!sortAsc);
    } else {
      setSortField(field);
      setSortAsc(true);
    }
  };

  const handleRowClick = (id: string) => {
    if (selectedModelId === id) {
      setSelectedModelId(null);
      setModelInfo(null);
      setConfirmingRemove(false);
      return;
    }
    setSelectedModelId(id);
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

  // Sort models
  const allModels = models?.data ?? [];
  const mainModels = allModels.filter((m) => !m.id.includes('/'));
  const extensionModels = allModels.filter((m) => m.id.includes('/'));

  const sortedModels = [...mainModels].sort((a, b) => {
    const va = getSortValue(a, sortField);
    const vb = getSortValue(b, sortField);
    const dir = sortAsc ? 1 : -1;
    let result: number;
    if (typeof va === 'number' && typeof vb === 'number') {
      result = (va - vb) * dir;
    } else {
      result = String(va).localeCompare(String(vb)) * dir;
    }
    if (result !== 0 || sortField === 'size') return result;
    return (a.size - b.size);
  });

  return (
    <div>
      <div className="page-header">
        <h2>Models</h2>
        <p>List of all models available in the system. Click a model to view details.</p>
      </div>

      {error && <div className="alert alert-error">{error}</div>}
      {removeError && <div className="alert alert-error">{removeError}</div>}
      {removeSuccess && <div className="alert alert-success">{removeSuccess}</div>}
      {rebuildError && <div className="alert alert-error">{rebuildError}</div>}
      {rebuildSuccess && <div className="alert alert-success">Index rebuilt successfully</div>}

      <div className="catalog-main-content">
        {loading && <div className="loading">Loading models</div>}

        {!loading && !error && models && (
          <div className="catalog-table-wrap">
            {allModels.length > 0 ? (
              <table className="catalog-table">
                <thead>
                  <tr>
                    <th style={{ width: '40px', textAlign: 'center' }} title="Validated">✓</th>
                    {([
                      ['id', 'Model ID'],
                      ['owner', 'Owner'],
                      ['family', 'Family'],
                      ['size', 'Size'],
                      ['modified', 'Modified'],
                    ] as const).map(([field, label]) => (
                      <th key={field} onClick={() => handleSort(field)} className="catalog-table-sortable">
                        {label}
                        <span className="catalog-table-sort-indicator">
                          {sortField === field ? (sortAsc ? ' ▲' : ' ▼') : ''}
                        </span>
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {sortedModels.map((model) => {
                    const extensions = extensionModels.filter((ext) => ext.id.startsWith(model.id + '/'));
                    const isParentSelected = selectedModelId === model.id;
                    const isExtensionSelected = selectedModelId?.startsWith(model.id + '/');
                    const showExtensions = isParentSelected || isExtensionSelected;
                    return (
                      <>{/* keyed fragment not needed; keys on tr */}
                        <tr
                          key={model.id}
                          className={selectedModelId === model.id ? 'active' : ''}
                          onClick={() => handleRowClick(model.id)}
                        >
                          <td style={{ textAlign: 'center', color: model.validated ? 'inherit' : 'var(--color-error)' }}>{model.validated ? '✓' : '✗'}</td>
                          <td><span className="catalog-table-cell-ellipsis">{model.id}</span></td>
                          <td>{model.owned_by || '-'}</td>
                          <td>{model.model_family || '-'}</td>
                          <td>{formatBytes(model.size)}</td>
                          <td>{formatDate(model.modified)}</td>
                        </tr>
                        {showExtensions && extensions.map((ext) => (
                          <tr
                            key={ext.id}
                            className={selectedModelId === ext.id ? 'active' : ''}
                            onClick={() => handleRowClick(ext.id)}
                          >
                            <td></td>
                            <td style={{ paddingLeft: '24px' }}><span className="catalog-table-cell-ellipsis">↳ {ext.id}</span></td>
                            <td></td>
                            <td>Extension Model</td>
                            <td></td>
                            <td></td>
                          </tr>
                        ))}
                      </>
                    );
                  })}
                </tbody>
              </table>
            ) : (
              <div className="empty-state">
                <h3>No models found</h3>
                <p>Pull a model to get started</p>
              </div>
            )}
          </div>
        )}

        <div style={{ marginTop: '16px', display: 'flex', gap: '8px' }}>
          <button
            className="btn btn-secondary"
            onClick={() => {
              invalidate();
              loadModels();
              setSelectedModelId(null);
              setModelInfo(null);
              setConfirmingRemove(false);
              setRemoveError(null);
              setRemoveSuccess(null);
              setInfoError(null);
              setRebuildError(null);
              setRebuildSuccess(false);
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

        {infoError && <div className="alert alert-error" style={{ marginTop: '16px' }}>{infoError}</div>}

        {infoLoading && (
          <div style={{ marginTop: '16px' }}>
            <div className="loading">Loading model details</div>
          </div>
        )}

        {selectedModelId && modelInfo && !infoLoading && (
          <div style={{ marginTop: '16px', borderTop: '1px solid var(--color-gray-200)', paddingTop: '16px' }}>
            <div className="tabs">
              {(Object.keys(SECTION_LABELS) as ModelListSection[]).map(section => (
                <button
                  key={section}
                  className={`tab ${activeSection === section ? 'active' : ''}`}
                  onClick={() => setActiveSection(section)}
                >
                  {SECTION_LABELS[section]}
                </button>
              ))}
            </div>

            {/* Model Configuration Section */}
            {activeSection === 'config' && (
              <div>
                <h3 style={{ marginBottom: '16px' }}>{selectedModelId}</h3>

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
                  { key: 'validated', label: 'Validated', value: (() => { const m = allModels.find((m) => m.id === selectedModelId); return m ? <span style={{ color: m.validated ? 'inherit' : 'var(--color-error)' }}>{m.validated ? '✓' : '✗'}</span> : '-'; })() },
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
            {activeSection === 'sampling' && (
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
            {activeSection === 'metadata' && (
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
            {activeSection === 'template' && (
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
            {activeSection === 'vram' && (
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

                {vramResults ? (
                  <>
                    <p style={{ fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: '16px' }}>
                      Computed locally from GGUF header. Adjust parameters below to see how they affect VRAM.
                    </p>

                    <div style={{ marginBottom: '24px' }}>
                      <VRAMControls
                        {...vramControls}
                        variant="compact"
                      />
                    </div>

                    <VRAMResults
                      totalVram={vramResults.vramResult.totalVram}
                      slotMemory={vramResults.vramResult.slotMemory}
                      kvPerSlot={vramResults.vramResult.kvPerSlot}
                      kvPerTokenPerLayer={vramResults.vramResult.kvPerTokenPerLayer}
                      input={vramResults.input}
                      moe={vramResults.moe}
                      weights={vramResults.weights}
                      modelWeightsGPU={vramResults.vramResult.modelWeightsGPU}
                      modelWeightsCPU={vramResults.vramResult.modelWeightsCPU}
                      computeBufferEst={vramResults.vramResult.computeBufferEst}
                      expertLayersOnGPU={vramResults.expertLayersOnGPU}
                      kvCacheOnCPU={vramControls.kvCacheOnCPU}
                      kvCpuBytes={vramResults.vramResult.kvCpuBytes}
                      totalSystemRamEst={vramResults.vramResult.totalSystemRamEst}
                      perDevice={vramResults.perDevice}
                      deviceCount={vramResults.deviceCount}
                      systemRAMBytes={vramResults.systemRAMBytes}
                      gpuTotalBytes={vramResults.gpuTotalBytes}
                      gpuDevices={vramResults.gpuDevices}
                      tensorSplit={vramResults.tensorSplit}
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
        )}
      </div>
    </div>
  );
}
