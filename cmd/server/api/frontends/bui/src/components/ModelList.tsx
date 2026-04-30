import { useState, useEffect, useRef } from 'react';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import type { ModelInfoResponse, ListModelDetail } from '../types';
import { formatBytes, fmtNum, fmtVal } from '../lib/format';
import { labelWithTip } from './ParamTooltips';
import { extractContextInfo } from '../lib/context';
import KeyValueTable from './KeyValueTable';
import ModelCard from './ModelCard';
import CodeBlock from './CodeBlock';
import { VRAMFormulaModal, VRAMCalculatorPanel, useVRAMState } from './vram';

type ModelListSection = 'model-card' | 'draft-card' | 'config' | 'sampling' | 'template' | 'vram';

const SECTION_LABELS: Record<ModelListSection, string> = {
  'model-card': 'Model Card',
  'draft-card': 'Draft Model Card',
  config: 'Model Configuration',
  sampling: 'Sampling Parameters',
  template: 'Template',
  vram: 'VRAM Calculator',
};

type SortField = 'id' | 'owner' | 'family' | 'projection' | 'size' | 'modified';

function getSortValue(model: ListModelDetail, field: SortField): string | number {
  switch (field) {
    case 'id': return model.id.toLowerCase();
    case 'owner': return (model.owned_by || '').toLowerCase();
    case 'family': return (model.model_family || '').toLowerCase();
    case 'projection': return model.has_projection ? 1 : 0;
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
  const [activeSection, setActiveSection] = useState<ModelListSection>('model-card');

  const [draftModelInfo, setDraftModelInfo] = useState<ModelInfoResponse | null>(null);
  const [draftInfoLoading, setDraftInfoLoading] = useState(false);

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
    modelId: selectedModelId ?? undefined,
  });
  const contextInfo = extractContextInfo(modelInfo?.metadata);
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

  // Derive draft model presence from the detail config (fetched per-model).
  const allModels = models?.data ?? [];
  const draftModelConfig = modelInfo?.model_config?.['draft-model'];
  const draftModelId = draftModelConfig?.['model-id'] ?? null;
  const hasDraftModel = !!draftModelConfig;

  // Fetch draft model info when draft-card tab is selected
  useEffect(() => {
    if (activeSection !== 'draft-card' || !draftModelId) {
      setDraftModelInfo(null);
      return;
    }

    let cancelled = false;
    setDraftInfoLoading(true);
    setDraftModelInfo(null);

    api.showModel(draftModelId)
      .then((resp) => { if (!cancelled) setDraftModelInfo(resp); })
      .catch(() => { /* draft model info is best-effort */ })
      .finally(() => { if (!cancelled) setDraftInfoLoading(false); });

    return () => { cancelled = true; };
  }, [activeSection, draftModelId]);

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
    setActiveSection('model-card');
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
      <div className="page-header-with-action">
        <div>
          <h2>Models</h2>
          <p className="page-description">List of all models available in the system. Click a model to view details.</p>
        </div>
        <div style={{ display: 'flex', gap: '8px' }}>
          <button
            className="btn btn-primary"
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
            className="btn btn-primary"
            onClick={handleRebuildIndex}
            disabled={rebuildingIndex || loading}
          >
            {rebuildingIndex ? 'Rebuilding...' : 'Rebuild Index'}
          </button>
          {selectedModelId && !confirmingRemove && (
            <button
              className="btn btn-primary"
              onClick={handleRemoveClick}
              disabled={removing}
            >
              Remove Model
            </button>
          )}
          {selectedModelId && confirmingRemove && (
            <>
              <button className="btn btn-primary" onClick={handleConfirmRemove} disabled={removing}>
                {removing ? 'Removing...' : 'Yes, Remove'}
              </button>
              <button className="btn btn-primary" onClick={handleCancelRemove} disabled={removing}>
                Cancel
              </button>
            </>
          )}
        </div>
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
                    <th style={{ width: '40px', textAlign: 'center' }} title="Configuration and template confirmed working with the Kronk catalog">VAL</th>
                    {([
                      ['id', 'Model ID'],
                      ['owner', 'Provider'],
                      ['family', 'Family'],
                    ] as const).map(([field, label]) => (
                      <th key={field} onClick={() => handleSort(field)} className="catalog-table-sortable">
                        {label}
                        <span className="catalog-table-sort-indicator">
                          {sortField === field ? (sortAsc ? ' ▲' : ' ▼') : ''}
                        </span>
                      </th>
                    ))}
                    <th style={{ textAlign: 'center' }} onClick={() => handleSort('projection')} className="catalog-table-sortable" title="Multimodal projection file present">
                      MTMD
                      <span className="catalog-table-sort-indicator">
                        {sortField === 'projection' ? (sortAsc ? ' ▲' : ' ▼') : ''}
                      </span>
                    </th>
                    {([
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
                          <td style={{ textAlign: 'center' }}>{model.has_projection ? '✓' : ''}</td>
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

        {infoError && <div className="alert alert-error" style={{ marginTop: '16px' }}>{infoError}</div>}

        {infoLoading && (
          <div style={{ marginTop: '16px' }}>
            <div className="loading">Loading model details</div>
          </div>
        )}

        {selectedModelId && modelInfo && !infoLoading && (
          <div style={{ marginTop: '16px', borderTop: '1px solid var(--color-gray-200)', paddingTop: '16px' }}>
            <div className="tabs">
              {(Object.keys(SECTION_LABELS) as ModelListSection[]).filter(
                section => section !== 'draft-card' || hasDraftModel,
              ).map(section => (
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
                  { key: 'owner', label: 'Provider', value: modelInfo.owned_by },
                  { key: 'size', label: 'Size', value: formatBytes(modelInfo.size) },
                  { key: 'created', label: 'Created', value: new Date(modelInfo.created).toLocaleString() },
                  { key: 'projection', label: labelWithTip('Has Projection', 'hasProjection'), value: <span className={`badge ${modelInfo.has_projection ? 'badge-yes' : 'badge-no'}`}>{modelInfo.has_projection ? 'Yes' : 'No'}</span> },
                  { key: 'gpt', label: labelWithTip('Is GPT', 'isGPT'), value: <span className={`badge ${modelInfo.is_gpt ? 'badge-yes' : 'badge-no'}`}>{modelInfo.is_gpt ? 'Yes' : 'No'}</span> },
                  { key: 'validated', label: labelWithTip('Validated', 'validated'), value: (() => { const m = allModels.find((m) => m.id === selectedModelId); return m ? <span style={{ color: m.validated ? 'inherit' : 'var(--color-error)' }}>{m.validated ? '✓' : '✗'}</span> : '-'; })() },
                ]} />

                {modelInfo.model_config && (
                  <div style={{ marginTop: '24px' }}>
                    <h4 className="meta-section-title" style={{ marginBottom: '8px' }}>Configuration</h4>
                    <KeyValueTable rows={(() => {
                      const mc = modelInfo.model_config;
                      const boolBadge = (v: boolean | null | undefined) => <span className={`badge ${v ? 'badge-yes' : 'badge-no'}`}>{v ? 'Yes' : 'No'}</span>;
                      return [

                        { key: 'cache-min-tokens', label: labelWithTip('cache-min-tokens', 'cacheMinTokens'), value: fmtVal(mc['cache-min-tokens']) },
                        { key: 'cache-type-k', label: labelWithTip('cache-type-k', 'cacheTypeK'), value: mc['cache-type-k'] || 'default' },
                        { key: 'cache-type-v', label: labelWithTip('cache-type-v', 'cacheTypeV'), value: mc['cache-type-v'] || 'default' },
                        { key: 'context-window', label: labelWithTip('context-window', 'contextWindow'), value: fmtVal(mc['context-window']) },
                        { key: 'devices', label: labelWithTip('devices', 'devices'), value: mc.devices?.join(', ') || '—' },
                        { key: 'draft-model.devices', label: labelWithTip('draft-model.devices', 'draftDevice'), value: mc['draft-model']?.devices?.join(', ') || '—' },
                        { key: 'draft-model.main-gpu', label: labelWithTip('draft-model.main-gpu', 'draftMainGpu'), value: fmtVal(mc['draft-model']?.['main-gpu']) },
                        { key: 'draft-model.model-id', label: labelWithTip('draft-model.model-id', 'draftModel'), value: mc['draft-model']?.['model-id'] || '—' },
                        { key: 'draft-model.ndraft', label: labelWithTip('draft-model.ndraft', 'draftTokens'), value: fmtVal(mc['draft-model']?.ndraft) },
                        { key: 'draft-model.ngpu-layers', label: labelWithTip('draft-model.ngpu-layers', 'draftGpuLayers'), value: fmtVal(mc['draft-model']?.['ngpu-layers']) },
                        { key: 'draft-model.tensor-split', label: labelWithTip('draft-model.tensor-split', 'draftTensorSplit'), value: mc['draft-model']?.['tensor-split']?.join(', ') || '—' },
                        { key: 'flash-attention', label: labelWithTip('flash-attention', 'flashAttention'), value: mc['flash-attention'] || 'default' },

                        { key: 'incremental-cache', label: labelWithTip('incremental-cache', 'incrementalCache'), value: boolBadge(mc['incremental-cache']) },
                        { key: 'main-gpu', label: labelWithTip('main-gpu', 'mainGpu'), value: fmtVal(mc['main-gpu']) },
                        { key: 'moe.keep-experts-top-n', label: labelWithTip('moe.keep-experts-top-n', 'moeKeepExpertsTopN'), value: fmtVal(mc.moe?.['keep-experts-top-n']) },
                        { key: 'moe.mode', label: labelWithTip('moe.mode', 'moeMode'), value: mc.moe?.mode || '—' },
                        { key: 'nbatch', label: labelWithTip('nbatch', 'nbatch'), value: fmtVal(mc.nbatch) },
                        { key: 'ngpu-layers', label: labelWithTip('ngpu-layers', 'ngpuLayers'), value: fmtVal(mc['ngpu-layers'] ?? 'auto') },
                        { key: 'nseq-max', label: labelWithTip('nseq-max', 'nSeqMax'), value: fmtVal(mc['nseq-max']) },
                        { key: 'nthreads', label: labelWithTip('nthreads', 'nthreads'), value: fmtVal(mc.nthreads) },
                        { key: 'nthreads-batch', label: labelWithTip('nthreads-batch', 'nthreadsBatch'), value: fmtVal(mc['nthreads-batch']) },
                        { key: 'nubatch', label: labelWithTip('nubatch', 'nubatch'), value: fmtVal(mc.nubatch) },
                        { key: 'numa', label: labelWithTip('numa', 'numa'), value: mc.numa || '—' },
                        { key: 'offload-kqv', label: labelWithTip('offload-kqv', 'offloadKQV'), value: fmtVal(mc['offload-kqv']) },
                        { key: 'op-offload', label: labelWithTip('op-offload', 'opOffload'), value: fmtVal(mc['op-offload']) },
                        { key: 'op-offload-min-batch', label: labelWithTip('op-offload-min-batch', 'opOffloadMinBatch'), value: fmtVal(mc['op-offload-min-batch']) },
                        { key: 'rope-freq-base', label: labelWithTip('rope-freq-base', 'ropeFreqBase'), value: fmtVal(mc['rope-freq-base']) },
                        { key: 'rope-freq-scale', label: labelWithTip('rope-freq-scale', 'ropeFreqScale'), value: fmtVal(mc['rope-freq-scale']) },
                        { key: 'rope-scaling-type', label: labelWithTip('rope-scaling-type', 'ropeScaling'), value: mc['rope-scaling-type'] || '—' },
                        { key: 'split-mode', label: labelWithTip('split-mode', 'splitMode'), value: mc['split-mode'] || 'default' },
                        { key: 'swa-full', label: labelWithTip('swa-full', 'swaFull'), value: mc['swa-full'] == null ? <span className="badge badge-yes">Yes (default)</span> : boolBadge(mc['swa-full']) },
                        { key: 'tensor-buft-overrides', label: labelWithTip('tensor-buft-overrides', 'tensorBuftOverrides'), value: mc['tensor-buft-overrides']?.join(', ') || '—' },
                        { key: 'tensor-split', label: labelWithTip('tensor-split', 'tensorSplit'), value: mc['tensor-split']?.join(', ') || '—' },
                        { key: 'use-direct-io', label: labelWithTip('use-direct-io', 'useDirectIO'), value: boolBadge(mc['use-direct-io']) },
                        { key: 'use-mmap', label: labelWithTip('use-mmap', 'useMMap'), value: fmtVal(mc['use-mmap']) },
                        { key: 'yarn-attn-factor', label: labelWithTip('yarn-attn-factor', 'yarnAttnFactor'), value: fmtVal(mc['yarn-attn-factor']) },
                        { key: 'yarn-beta-fast', label: labelWithTip('yarn-beta-fast', 'yarnBetaFast'), value: fmtVal(mc['yarn-beta-fast']) },
                        { key: 'yarn-beta-slow', label: labelWithTip('yarn-beta-slow', 'yarnBetaSlow'), value: fmtVal(mc['yarn-beta-slow']) },
                        { key: 'yarn-ext-factor', label: labelWithTip('yarn-ext-factor', 'yarnExtFactor'), value: fmtVal(mc['yarn-ext-factor']) },
                        { key: 'yarn-orig-ctx', label: labelWithTip('yarn-orig-ctx', 'yarnOrigCtx'), value: fmtVal(mc['yarn-orig-ctx']) },
                      ];
                    })()} />
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
                      { key: 'dry_allowed_length', label: labelWithTip('dry_allowed_length', 'dry_allowed_length'), value: fmtVal(sp.dry_allowed_length) },
                      { key: 'dry_base', label: labelWithTip('dry_base', 'dry_base'), value: fmtVal(sp.dry_base) },
                      { key: 'dry_multiplier', label: labelWithTip('dry_multiplier', 'dry_multiplier'), value: fmtVal(sp.dry_multiplier) },
                      { key: 'dry_penalty_last_n', label: labelWithTip('dry_penalty_last_n', 'dry_penalty_last_n'), value: fmtVal(sp.dry_penalty_last_n) },
                      { key: 'enable_thinking', label: labelWithTip('enable_thinking', 'enable_thinking'), value: fmtVal(sp.enable_thinking ?? '—') },
                      { key: 'frequency_penalty', label: labelWithTip('frequency_penalty', 'frequency_penalty'), value: fmtNum(sp.frequency_penalty) },
                      { key: 'grammar', label: labelWithTip('grammar', 'grammar'), value: fmtVal(sp.grammar || '—') },
                      { key: 'max_tokens', label: labelWithTip('max_tokens', 'max_tokens'), value: fmtVal(sp.max_tokens) },
                      { key: 'min_p', label: labelWithTip('min_p', 'min_p'), value: fmtNum(sp.min_p) },
                      { key: 'presence_penalty', label: labelWithTip('presence_penalty', 'presence_penalty'), value: fmtNum(sp.presence_penalty) },
                      { key: 'reasoning_effort', label: labelWithTip('reasoning_effort', 'reasoning_effort'), value: fmtVal(sp.reasoning_effort ?? '—') },
                      { key: 'repeat_last_n', label: labelWithTip('repeat_last_n', 'repeat_last_n'), value: fmtVal(sp.repeat_last_n) },
                      { key: 'repeat_penalty', label: labelWithTip('repeat_penalty', 'repeat_penalty'), value: fmtNum(sp.repeat_penalty) },
                      { key: 'temperature', label: labelWithTip('temperature', 'temperature'), value: fmtNum(sp.temperature) },
                      { key: 'top_k', label: labelWithTip('top_k', 'top_k'), value: fmtVal(sp.top_k) },
                      { key: 'top_p', label: labelWithTip('top_p', 'top_p'), value: fmtNum(sp.top_p) },
                      { key: 'xtc_min_keep', label: labelWithTip('xtc_min_keep', 'xtc_min_keep'), value: fmtVal(sp.xtc_min_keep) },
                      { key: 'xtc_probability', label: labelWithTip('xtc_probability', 'xtc_probability'), value: fmtVal(sp.xtc_probability) },
                      { key: 'xtc_threshold', label: labelWithTip('xtc_threshold', 'xtc_threshold'), value: fmtVal(sp.xtc_threshold) },
                    ]} />
                  );
                })() : (
                  <div className="empty-state">
                    <p>No sampling parameters configured for this model.</p>
                  </div>
                )}
              </div>
            )}

            {/* Model Card Section */}
            {activeSection === 'model-card' && (
              <div>
                <h3 style={{ marginBottom: '16px' }}>Model Card</h3>
                <ModelCard metadata={modelInfo.metadata ?? {}} webPage={modelInfo.web_page} />
              </div>
            )}

            {/* Draft Model Card Section */}
            {activeSection === 'draft-card' && hasDraftModel && (
              <div>
                <h3 style={{ marginBottom: '16px' }}>Draft Model Card</h3>

                <div style={{ marginBottom: '24px' }}>
                  <h4 className="meta-section-title" style={{ marginBottom: '8px' }}>Draft Settings</h4>
                  <KeyValueTable rows={[
                    { key: 'draft-id', label: labelWithTip('Model ID', 'draftModel'), value: draftModelId ?? '-' },
                    ...(draftModelConfig ? [
                      { key: 'draft-ndraft', label: labelWithTip('Draft Tokens', 'draftTokens'), value: fmtVal(draftModelConfig.ndraft) },
                      ...(draftModelConfig['ngpu-layers'] != null ? [{ key: 'draft-ngpu', label: labelWithTip('GPU Layers', 'ngpuLayers'), value: fmtVal(draftModelConfig['ngpu-layers']) }] : []),
                      ...(draftModelConfig.devices?.length ? [{ key: 'draft-devices', label: labelWithTip('Devices', 'devices'), value: draftModelConfig.devices.join(', ') }] : []),
                    ] : []),
                  ]} />
                </div>

                {draftInfoLoading && <div className="loading">Loading draft model details</div>}

                {draftModelInfo && (
                  <div style={{ marginBottom: '24px' }}>
                    <h4 className="meta-section-title" style={{ marginBottom: '8px' }}>Model Details</h4>
                    <KeyValueTable rows={[
                      { key: 'draft-owner', label: 'Provider', value: draftModelInfo.owned_by },
                      { key: 'draft-size', label: 'Size', value: formatBytes(draftModelInfo.size) },
                    ]} />
                  </div>
                )}

                {draftModelInfo ? (
                  <ModelCard metadata={draftModelInfo.metadata ?? {}} webPage={draftModelInfo.web_page} />
                ) : (
                  !draftInfoLoading && (
                    <div className="empty-state">
                      <p>No metadata available for the draft model.</p>
                    </div>
                  )
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

                    <VRAMCalculatorPanel
                      controlsProps={vramControls}
                      resultsProps={vramResults}
                      variant="compact"
                      contextInfo={contextInfo}
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
