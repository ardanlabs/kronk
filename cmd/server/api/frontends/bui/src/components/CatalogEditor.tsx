import { useState, useEffect } from 'react';
import { useSearchParams } from 'react-router-dom';
import { api } from '../services/api';
import type { CatalogModelResponse, ModelConfig } from '../types';

interface FileEntry {
  url: string;
  size: string;
}

interface CatalogFormData {
  id: string;
  category: string;
  catalogFile: string;
  newCatalogFile: string;
  ownedBy: string;
  modelFamily: string;
  webPage: string;
  template: string;
  gatedModel: boolean;
  files: FileEntry[];
  projUrl: string;
  projSize: string;
  capabilities: {
    endpoint: string;
    images: boolean;
    audio: boolean;
    video: boolean;
    streaming: boolean;
    reasoning: boolean;
    tooling: boolean;
    embedding: boolean;
    rerank: boolean;
  };
  description: string;
  collections: string;
  created: string;
  config: {
    device: string;
    contextWindow: number | null;
    nbatch: number | null;
    nubatch: number | null;
    nthreads: number | null;
    nthreadsBatch: number | null;
    cacheTypeK: string;
    cacheTypeV: string;
    useDirectIO: boolean | null;
    flashAttention: string;
    ignoreIntegrityCheck: boolean | null;
    nseqMax: number | null;
    offloadKQV: boolean | null;
    opOffload: boolean | null;
    ngpuLayers: number | null;
    splitMode: string;
    systemPromptCache: boolean | null;
    incrementalCache: boolean | null;
    cacheMinTokens: number | null;
    ropeScaling: string;
    ropeFreqBase: number | null;
    ropeFreqScale: number | null;
    yarnExtFactor: number | null;
    yarnAttnFactor: number | null;
    yarnBetaFast: number | null;
    yarnBetaSlow: number | null;
    yarnOrigCtx: number | null;
  };
  sampling: {
    temperature: number | null;
    topK: number | null;
    topP: number | null;
    minP: number | null;
    maxTokens: number | null;
    repeatPenalty: number | null;
    repeatLastN: number | null;
    dryMultiplier: number | null;
    dryBase: number | null;
    dryAllowedLen: number | null;
    dryPenaltyLast: number | null;
    xtcProbability: number | null;
    xtcThreshold: number | null;
    xtcMinKeep: number | null;
    enableThinking: string;
    reasoningEffort: string;
  };
}

interface CatalogFileInfo {
  name: string;
  model_count: number;
}

interface HFRepoFile {
  filename: string;
  size: number;
  size_str: string;
}

const defaultForm: CatalogFormData = {
  id: '',
  category: 'Text-Generation',
  catalogFile: '',
  newCatalogFile: '',
  ownedBy: '',
  modelFamily: '',
  webPage: '',
  template: '',
  gatedModel: false,
  files: [{ url: '', size: '' }],
  projUrl: '',
  projSize: '',
  capabilities: {
    endpoint: 'chat_completion',
    images: false,
    audio: false,
    video: false,
    streaming: true,
    reasoning: false,
    tooling: false,
    embedding: false,
    rerank: false,
  },
  description: '',
  collections: '',
  created: new Date().toISOString().split('T')[0],
  config: {
    device: '',
    contextWindow: null,
    nbatch: null,
    nubatch: null,
    nthreads: null,
    nthreadsBatch: null,
    cacheTypeK: '',
    cacheTypeV: '',
    useDirectIO: null,
    flashAttention: '',
    ignoreIntegrityCheck: null,
    nseqMax: null,
    offloadKQV: null,
    opOffload: null,
    ngpuLayers: null,
    splitMode: '',
    systemPromptCache: null,
    incrementalCache: null,
    cacheMinTokens: null,
    ropeScaling: '',
    ropeFreqBase: null,
    ropeFreqScale: null,
    yarnExtFactor: null,
    yarnAttnFactor: null,
    yarnBetaFast: null,
    yarnBetaSlow: null,
    yarnOrigCtx: null,
  },
  sampling: {
    temperature: null,
    topK: null,
    topP: null,
    minP: null,
    maxTokens: null,
    repeatPenalty: null,
    repeatLastN: null,
    dryMultiplier: null,
    dryBase: null,
    dryAllowedLen: null,
    dryPenaltyLast: null,
    xtcProbability: null,
    xtcThreshold: null,
    xtcMinKeep: null,
    enableThinking: '',
    reasoningEffort: '',
  },
};

const inputStyle: React.CSSProperties = {
  width: '100%',
  padding: '8px',
  borderRadius: '4px',
  border: '1px solid var(--color-gray-300)',
  background: 'var(--color-gray-50)',
  color: 'var(--color-gray-900)',
  fontSize: '14px',
  boxSizing: 'border-box',
};

const labelStyle: React.CSSProperties = {
  fontWeight: 600,
  fontSize: '14px',
  color: 'var(--color-gray-700)',
  marginBottom: '4px',
  display: 'block',
};

const gridStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '1fr 1fr',
  gap: '12px',
};

const sectionStyle: React.CSSProperties = {
  marginTop: '24px',
};

function populateFromResponse(resp: CatalogModelResponse): CatalogFormData {
  const created = resp.metadata?.created
    ? new Date(resp.metadata.created).toISOString().split('T')[0]
    : new Date().toISOString().split('T')[0];

  const files = resp.files?.model?.length
    ? resp.files.model.map((f) => ({ url: f.url, size: f.size }))
    : [{ url: '', size: '' }];

  const mc = resp.base_config || resp.model_config;
  const sp = mc?.['sampling-parameters'];

  return {
    id: resp.id || '',
    category: resp.category || 'Text-Generation',
    catalogFile: resp.catalog_file || '',
    newCatalogFile: '',
    ownedBy: resp.owned_by || '',
    modelFamily: resp.model_family || '',
    webPage: resp.web_page || '',
    template: resp.template || '',
    gatedModel: resp.gated_model || false,
    files,
    projUrl: resp.files?.proj?.url || '',
    projSize: resp.files?.proj?.size || '',
    capabilities: {
      endpoint: resp.capabilities?.endpoint || 'chat_completion',
      images: resp.capabilities?.images || false,
      audio: resp.capabilities?.audio || false,
      video: resp.capabilities?.video || false,
      streaming: resp.capabilities?.streaming || false,
      reasoning: resp.capabilities?.reasoning || false,
      tooling: resp.capabilities?.tooling || false,
      embedding: resp.capabilities?.embedding || false,
      rerank: resp.capabilities?.rerank || false,
    },
    description: resp.metadata?.description || '',
    collections: resp.metadata?.collections || '',
    created,
    config: {
      device: mc?.device || '',
      contextWindow: mc?.['context-window'] ?? null,
      nbatch: mc?.nbatch ?? null,
      nubatch: mc?.nubatch ?? null,
      nthreads: mc?.nthreads ?? null,
      nthreadsBatch: mc?.['nthreads-batch'] ?? null,
      cacheTypeK: mc?.['cache-type-k'] || '',
      cacheTypeV: mc?.['cache-type-v'] || '',
      useDirectIO: mc?.['use-direct-io'] ?? null,
      flashAttention: mc?.['flash-attention'] || '',
      ignoreIntegrityCheck: mc?.['ignore-integrity-check'] ?? null,
      nseqMax: mc?.['nseq-max'] ?? null,
      offloadKQV: mc?.['offload-kqv'] ?? null,
      opOffload: mc?.['op-offload'] ?? null,
      ngpuLayers: mc?.['ngpu-layers'] ?? null,
      splitMode: mc?.['split-mode'] || '',
      systemPromptCache: mc?.['system-prompt-cache'] ?? null,
      incrementalCache: mc?.['incremental-cache'] ?? null,
      cacheMinTokens: mc?.['cache-min-tokens'] ?? null,
      ropeScaling: mc?.['rope-scaling-type'] || '',
      ropeFreqBase: mc?.['rope-freq-base'] ?? null,
      ropeFreqScale: mc?.['rope-freq-scale'] ?? null,
      yarnExtFactor: mc?.['yarn-ext-factor'] ?? null,
      yarnAttnFactor: mc?.['yarn-attn-factor'] ?? null,
      yarnBetaFast: mc?.['yarn-beta-fast'] ?? null,
      yarnBetaSlow: mc?.['yarn-beta-slow'] ?? null,
      yarnOrigCtx: mc?.['yarn-orig-ctx'] ?? null,
    },
    sampling: {
      temperature: sp?.temperature ?? null,
      topK: sp?.top_k ?? null,
      topP: sp?.top_p ?? null,
      minP: sp?.min_p ?? null,
      maxTokens: sp?.max_tokens ?? null,
      repeatPenalty: sp?.repeat_penalty ?? null,
      repeatLastN: sp?.repeat_last_n ?? null,
      dryMultiplier: sp?.dry_multiplier ?? null,
      dryBase: sp?.dry_base ?? null,
      dryAllowedLen: sp?.dry_allowed_length ?? null,
      dryPenaltyLast: sp?.dry_penalty_last_n ?? null,
      xtcProbability: sp?.xtc_probability ?? null,
      xtcThreshold: sp?.xtc_threshold ?? null,
      xtcMinKeep: sp?.xtc_min_keep ?? null,
      enableThinking: sp?.enable_thinking || '',
      reasoningEffort: sp?.reasoning_effort || '',
    },
  };
}

function defaultPlaceholder(defaultValue: number | undefined): string {
  if (defaultValue === undefined || defaultValue === 0) return '';
  return `default: ${defaultValue}`;
}

function NullableNumInput({ label, value, step, defaultValue, onChange }: { label: string; value: number | null; step?: string; defaultValue?: number; onChange: (v: number | null) => void }) {
  return (
    <div>
      <label style={labelStyle}>{label}</label>
      <input
        type="number"
        step={step}
        value={value ?? ''}
        onChange={(e) => onChange(e.target.value === '' ? null : parseFloat(e.target.value) || 0)}
        style={inputStyle}
        placeholder={defaultPlaceholder(defaultValue) || 'not set'}
      />
    </div>
  );
}

function TriStateSelect({ label, value, onChange }: { label: string; value: boolean | null; onChange: (v: boolean | null) => void }) {
  const strVal = value === null ? '' : value ? 'true' : 'false';
  return (
    <div>
      <label style={labelStyle}>{label}</label>
      <select value={strVal} onChange={(e) => onChange(e.target.value === '' ? null : e.target.value === 'true')} style={inputStyle}>
        <option value="">not set</option>
        <option value="true">Yes</option>
        <option value="false">No</option>
      </select>
    </div>
  );
}

export default function CatalogEditor() {
  const [searchParams] = useSearchParams();
  const editId = searchParams.get('id');

  const [form, setForm] = useState<CatalogFormData>({ ...defaultForm });
  const [resolvedConfig, setResolvedConfig] = useState<ModelConfig | null>(null);
  const [catalogFiles, setCatalogFiles] = useState<CatalogFileInfo[]>([]);
  const [hfInput, setHfInput] = useState('');
  const [hfLoading, setHfLoading] = useState(false);
  const [hfError, setHfError] = useState<string | null>(null);
  const [repoFiles, setRepoFiles] = useState<HFRepoFile[]>([]);
  const [saving, setSaving] = useState(false);
  const [saveMsg, setSaveMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [editLoading, setEditLoading] = useState(false);
  const [showConfig, setShowConfig] = useState(false);
  const [showSampling, setShowSampling] = useState(false);
  const [repoPath, setRepoPath] = useState('');
  const [publishing, setPublishing] = useState(false);

  useEffect(() => {
    api.listCatalogFiles().then(setCatalogFiles).catch(() => {});
    api.getCatalogRepoPath().then((r) => setRepoPath(r.repo_path)).catch(() => {});
  }, []);

  useEffect(() => {
    if (!editId) return;
    setEditLoading(true);
    api.showCatalogModel(editId)
      .then((resp) => {
        setForm(populateFromResponse(resp));
        if (resp.model_config) {
          setResolvedConfig(resp.model_config);
        }
      })
      .catch(() => {})
      .finally(() => setEditLoading(false));
  }, [editId]);

  const handleLookup = async () => {
    if (!hfInput.trim()) return;
    setHfLoading(true);
    setHfError(null);
    setRepoFiles([]);

    try {
      const result = await api.lookupHuggingFace(hfInput.trim());
      const populated = populateFromResponse(result.model);
      setForm((prev) => ({
        ...prev,
        ...populated,
        catalogFile: prev.catalogFile,
        newCatalogFile: prev.newCatalogFile,
      }));

      if (result.repo_files?.length) {
        setRepoFiles(result.repo_files);
      }
    } catch (err) {
      setHfError(err instanceof Error ? err.message : 'Lookup failed');
    } finally {
      setHfLoading(false);
    }
  };

  const handleFileSelect = async (filename: string) => {
    const parts = hfInput.trim().replace('https://huggingface.co/', '').split('/');
    const owner = parts[0];
    const repo = parts[1];
    const fullInput = `${owner}/${repo}/${filename}`;
    setHfInput(fullInput);
    setHfLoading(true);
    setHfError(null);

    try {
      const result = await api.lookupHuggingFace(fullInput);
      const populated = populateFromResponse(result.model);
      setForm((prev) => ({
        ...prev,
        ...populated,
        catalogFile: prev.catalogFile,
        newCatalogFile: prev.newCatalogFile,
      }));
      setRepoFiles([]);
    } catch (err) {
      setHfError(err instanceof Error ? err.message : 'Lookup failed');
    } finally {
      setHfLoading(false);
    }
  };

  const handleSave = async () => {
    if (!form.id) {
      setSaveMsg({ type: 'error', text: 'Model ID is required' });
      return;
    }

    const catalogFile = form.catalogFile === '__new__'
      ? form.newCatalogFile
      : form.catalogFile;

    if (!catalogFile) {
      setSaveMsg({ type: 'error', text: 'Catalog file is required' });
      return;
    }

    setSaving(true);
    setSaveMsg(null);

    try {
      await api.saveCatalogModel({
        id: form.id,
        category: form.category,
        owned_by: form.ownedBy,
        model_family: form.modelFamily,
        web_page: form.webPage,
        gated_model: form.gatedModel,
        template: form.template,
        files: {
          model: form.files.filter((f) => f.url),
          proj: { url: form.projUrl, size: form.projSize },
        },
        capabilities: form.capabilities,
        metadata: {
          created: form.created ? new Date(form.created).toISOString() : new Date().toISOString(),
          collections: form.collections,
          description: form.description,
        },
        config: {
          device: form.config.device,
          'context-window': form.config.contextWindow ?? 0,
          nbatch: form.config.nbatch ?? 0,
          nubatch: form.config.nubatch ?? 0,
          nthreads: form.config.nthreads ?? 0,
          'nthreads-batch': form.config.nthreadsBatch ?? 0,
          'cache-type-k': form.config.cacheTypeK,
          'cache-type-v': form.config.cacheTypeV,
          'use-direct-io': form.config.useDirectIO ?? false,
          'flash-attention': form.config.flashAttention,
          'ignore-integrity-check': form.config.ignoreIntegrityCheck ?? false,
          'nseq-max': form.config.nseqMax ?? 0,
          'offload-kqv': form.config.offloadKQV,
          'op-offload': form.config.opOffload,
          'ngpu-layers': form.config.ngpuLayers,
          'split-mode': form.config.splitMode,
          'system-prompt-cache': form.config.systemPromptCache ?? false,
          'incremental-cache': form.config.incrementalCache ?? false,
          'cache-min-tokens': form.config.cacheMinTokens ?? 0,
          'rope-scaling-type': form.config.ropeScaling,
          'rope-freq-base': form.config.ropeFreqBase,
          'rope-freq-scale': form.config.ropeFreqScale,
          'yarn-ext-factor': form.config.yarnExtFactor,
          'yarn-attn-factor': form.config.yarnAttnFactor,
          'yarn-beta-fast': form.config.yarnBetaFast,
          'yarn-beta-slow': form.config.yarnBetaSlow,
          'yarn-orig-ctx': form.config.yarnOrigCtx,
          'sampling-parameters': {
            temperature: form.sampling.temperature ?? 0,
            top_k: form.sampling.topK ?? 0,
            top_p: form.sampling.topP ?? 0,
            min_p: form.sampling.minP ?? 0,
            max_tokens: form.sampling.maxTokens ?? 0,
            repeat_penalty: form.sampling.repeatPenalty ?? 0,
            repeat_last_n: form.sampling.repeatLastN ?? 0,
            dry_multiplier: form.sampling.dryMultiplier ?? 0,
            dry_base: form.sampling.dryBase ?? 0,
            dry_allowed_length: form.sampling.dryAllowedLen ?? 0,
            dry_penalty_last_n: form.sampling.dryPenaltyLast ?? 0,
            xtc_probability: form.sampling.xtcProbability ?? 0,
            xtc_threshold: form.sampling.xtcThreshold ?? 0,
            xtc_min_keep: form.sampling.xtcMinKeep ?? 0,
            enable_thinking: form.sampling.enableThinking,
            reasoning_effort: form.sampling.reasoningEffort,
          },
        },
        catalog_file: catalogFile,
      });
      setSaveMsg({ type: 'success', text: `Model "${form.id}" saved to ${catalogFile}` });
      api.listCatalogFiles().then(setCatalogFiles).catch(() => {});
    } catch (err) {
      setSaveMsg({ type: 'error', text: err instanceof Error ? err.message : 'Save failed' });
    } finally {
      setSaving(false);
    }
  };

  const handlePublish = async () => {
    const catalogFile = form.catalogFile === '__new__'
      ? form.newCatalogFile
      : form.catalogFile;

    if (!catalogFile) {
      setSaveMsg({ type: 'error', text: 'Catalog file is required to publish' });
      return;
    }

    setPublishing(true);
    setSaveMsg(null);

    try {
      await api.publishCatalogModel(catalogFile);
      setSaveMsg({ type: 'success', text: `Published "${catalogFile}" to repo` });
    } catch (err) {
      setSaveMsg({ type: 'error', text: err instanceof Error ? err.message : 'Publish failed' });
    } finally {
      setPublishing(false);
    }
  };

  const updateFile = (index: number, field: 'url' | 'size', value: string) => {
    const updated = [...form.files];
    updated[index] = { ...updated[index], [field]: value };
    setForm({ ...form, files: updated });
  };

  const addFile = () => {
    setForm({ ...form, files: [...form.files, { url: '', size: '' }] });
  };

  const removeFile = (index: number) => {
    if (form.files.length <= 1) return;
    setForm({ ...form, files: form.files.filter((_, i) => i !== index) });
  };

  const setConfig = (updates: Partial<CatalogFormData['config']>) => {
    setForm({ ...form, config: { ...form.config, ...updates } });
  };

  const setSampling = (updates: Partial<CatalogFormData['sampling']>) => {
    setForm({ ...form, sampling: { ...form.sampling, ...updates } });
  };

  const rc = resolvedConfig;
  const rsp = rc?.['sampling-parameters'];

  if (editLoading) {
    return (
      <div>
        <div className="page-header">
          <h2>Catalog Editor</h2>
        </div>
        <div className="card">
          <div className="loading">Loading model details</div>
        </div>
      </div>
    );
  }

  return (
    <div>
      <div className="page-header">
        <h2>{editId ? `Edit: ${editId}` : 'Catalog Editor'}</h2>
        <p>Create or edit catalog entries. Use HuggingFace lookup to auto-populate fields.</p>
      </div>

      {/* HuggingFace Import */}
      <div className="card">
        <h3 style={{ marginBottom: '12px' }}>HuggingFace Import</h3>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'flex-end' }}>
          <div style={{ flex: 1 }}>
            <label style={labelStyle}>Model URL or Path</label>
            <input
              type="text"
              value={hfInput}
              onChange={(e) => setHfInput(e.target.value)}
              placeholder="e.g., unsloth/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf or full HuggingFace URL"
              style={inputStyle}
              onKeyDown={(e) => e.key === 'Enter' && handleLookup()}
            />
          </div>
          <button className="btn btn-primary" onClick={handleLookup} disabled={hfLoading || !hfInput.trim()}>
            {hfLoading ? 'Looking up...' : 'Lookup'}
          </button>
        </div>

        {hfError && <div className="alert alert-error" style={{ marginTop: '8px' }}>{hfError}</div>}

        {repoFiles.length > 0 && (
          <div style={{ marginTop: '12px' }}>
            <label style={labelStyle}>Select a GGUF file from this repository</label>
            <div style={{ maxHeight: '300px', overflowY: 'auto', border: '1px solid var(--color-gray-300)', borderRadius: '4px' }}>
              <table style={{ width: '100%' }}>
                <thead>
                  <tr>
                    <th style={{ textAlign: 'left', padding: '6px 8px' }}>File</th>
                    <th style={{ textAlign: 'right', padding: '6px 8px' }}>Size</th>
                    <th style={{ width: '80px' }}></th>
                  </tr>
                </thead>
                <tbody>
                  {repoFiles.map((f) => (
                    <tr key={f.filename} style={{ cursor: 'pointer' }} onClick={() => handleFileSelect(f.filename)}>
                      <td style={{ padding: '6px 8px', fontSize: '13px' }}>{f.filename}</td>
                      <td style={{ padding: '6px 8px', textAlign: 'right', fontSize: '13px' }}>{f.size_str}</td>
                      <td style={{ padding: '6px 8px' }}>
                        <button className="btn btn-secondary" style={{ padding: '2px 8px', fontSize: '12px' }}>
                          Select
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>

      {/* Model Identity */}
      <div className="card" style={sectionStyle}>
        <h3 style={{ marginBottom: '12px' }}>Model Identity</h3>
        <div style={gridStyle}>
          <div>
            <label style={labelStyle}>ID *</label>
            <input type="text" value={form.id} onChange={(e) => setForm({ ...form, id: e.target.value })} style={inputStyle} />
          </div>
          <div>
            <label style={labelStyle}>Category</label>
            <select value={form.category} onChange={(e) => setForm({ ...form, category: e.target.value })} style={inputStyle}>
              <option value="Text-Generation">Text-Generation</option>
              <option value="Embedding">Embedding</option>
              <option value="Image-Text-to-Text">Image-Text-to-Text</option>
              <option value="Audio-Text-to-Text">Audio-Text-to-Text</option>
              <option value="Rerank">Rerank</option>
            </select>
          </div>
          <div>
            <label style={labelStyle}>Catalog File *</label>
            <select
              value={form.catalogFile}
              onChange={(e) => setForm({ ...form, catalogFile: e.target.value })}
              style={inputStyle}
            >
              <option value="">-- Select --</option>
              {catalogFiles.map((f) => (
                <option key={f.name} value={f.name}>{f.name} ({f.model_count} models)</option>
              ))}
              <option value="__new__">New catalog file...</option>
            </select>
          </div>
          {form.catalogFile === '__new__' && (
            <div>
              <label style={labelStyle}>New File Name</label>
              <input
                type="text"
                value={form.newCatalogFile}
                onChange={(e) => setForm({ ...form, newCatalogFile: e.target.value })}
                placeholder="e.g., text_generation"
                style={inputStyle}
              />
            </div>
          )}
          <div>
            <label style={labelStyle}>Owned By</label>
            <input type="text" value={form.ownedBy} onChange={(e) => setForm({ ...form, ownedBy: e.target.value })} style={inputStyle} />
          </div>
          <div>
            <label style={labelStyle}>Model Family</label>
            <input type="text" value={form.modelFamily} onChange={(e) => setForm({ ...form, modelFamily: e.target.value })} style={inputStyle} />
          </div>
          <div>
            <label style={labelStyle}>Web Page</label>
            <input type="text" value={form.webPage} onChange={(e) => setForm({ ...form, webPage: e.target.value })} style={inputStyle} />
          </div>
          <div>
            <label style={labelStyle}>Template</label>
            <input type="text" value={form.template} onChange={(e) => setForm({ ...form, template: e.target.value })} style={inputStyle} placeholder="e.g., qwen3.jinja" />
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', paddingTop: '24px' }}>
            <input
              type="checkbox"
              checked={form.gatedModel}
              onChange={(e) => setForm({ ...form, gatedModel: e.target.checked })}
              id="gated-model"
            />
            <label htmlFor="gated-model" style={{ ...labelStyle, marginBottom: 0 }}>Gated Model</label>
          </div>
        </div>
      </div>

      {/* Files */}
      <div className="card" style={sectionStyle}>
        <h3 style={{ marginBottom: '12px' }}>Files</h3>
        <label style={{ ...labelStyle, marginBottom: '8px' }}>Model Files</label>
        {form.files.map((file, idx) => (
          <div key={idx} style={{ display: 'flex', gap: '8px', marginBottom: '8px', alignItems: 'flex-end' }}>
            <div style={{ flex: 3 }}>
              {idx === 0 && <label style={{ fontSize: '12px', color: 'var(--color-gray-500)' }}>URL</label>}
              <input type="text" value={file.url} onChange={(e) => updateFile(idx, 'url', e.target.value)} style={inputStyle} placeholder="owner/repo/file.gguf" />
            </div>
            <div style={{ flex: 1 }}>
              {idx === 0 && <label style={{ fontSize: '12px', color: 'var(--color-gray-500)' }}>Size</label>}
              <input type="text" value={file.size} onChange={(e) => updateFile(idx, 'size', e.target.value)} style={inputStyle} placeholder="8.71 GiB" />
            </div>
            <button className="btn btn-danger" onClick={() => removeFile(idx)} disabled={form.files.length <= 1} style={{ padding: '8px 12px' }} title="Remove this file entry">
              ✕
            </button>
          </div>
        ))}
        <button className="btn btn-secondary" onClick={addFile} style={{ marginTop: '4px' }}>
          + Add File
        </button>

        <div style={{ ...gridStyle, marginTop: '16px' }}>
          <div>
            <label style={labelStyle}>Projection File URL (optional)</label>
            <input type="text" value={form.projUrl} onChange={(e) => setForm({ ...form, projUrl: e.target.value })} style={inputStyle} />
          </div>
          <div>
            <label style={labelStyle}>Projection File Size</label>
            <input type="text" value={form.projSize} onChange={(e) => setForm({ ...form, projSize: e.target.value })} style={inputStyle} />
          </div>
        </div>
      </div>

      {/* Capabilities */}
      <div className="card" style={sectionStyle}>
        <h3 style={{ marginBottom: '12px' }}>Capabilities</h3>
        <div style={{ marginBottom: '12px' }}>
          <label style={labelStyle}>Endpoint</label>
          <select
            value={form.capabilities.endpoint}
            onChange={(e) => setForm({ ...form, capabilities: { ...form.capabilities, endpoint: e.target.value } })}
            style={{ ...inputStyle, maxWidth: '300px' }}
          >
            <option value="chat_completion">chat_completion</option>
            <option value="embeddings">embeddings</option>
            <option value="rerank">rerank</option>
          </select>
        </div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '16px' }}>
          {(['images', 'audio', 'video', 'streaming', 'reasoning', 'tooling', 'embedding', 'rerank'] as const).map((cap) => (
            <label key={cap} style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={form.capabilities[cap]}
                onChange={(e) => setForm({ ...form, capabilities: { ...form.capabilities, [cap]: e.target.checked } })}
              />
              <span style={{ fontSize: '14px', textTransform: 'capitalize' }}>{cap}</span>
            </label>
          ))}
        </div>
      </div>

      {/* Metadata */}
      <div className="card" style={sectionStyle}>
        <h3 style={{ marginBottom: '12px' }}>Metadata</h3>
        <div style={{ marginBottom: '12px' }}>
          <label style={labelStyle}>Description</label>
          <textarea
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
            rows={3}
            style={{ ...inputStyle, resize: 'vertical' }}
          />
        </div>
        <div style={gridStyle}>
          <div>
            <label style={labelStyle}>Collections</label>
            <input type="text" value={form.collections} onChange={(e) => setForm({ ...form, collections: e.target.value })} style={inputStyle} placeholder="collections/owner" />
          </div>
          <div>
            <label style={labelStyle}>Created</label>
            <input type="date" value={form.created} onChange={(e) => setForm({ ...form, created: e.target.value })} style={inputStyle} />
          </div>
        </div>
      </div>

      {/* Config */}
      <div className="card" style={sectionStyle}>
        <h3
          onClick={() => setShowConfig(!showConfig)}
          style={{ cursor: 'pointer', userSelect: 'none', marginBottom: showConfig ? '12px' : 0 }}
        >
          {showConfig ? '▼' : '▶'} Configuration
        </h3>
        {showConfig && (
          <>
            <div style={gridStyle}>
              <NullableNumInput label="Batch Size (nbatch)" value={form.config.nbatch} defaultValue={rc?.nbatch} onChange={(v) => setConfig({ nbatch: v })} />
              <NullableNumInput label="Batch Threads (nthreads-batch)" value={form.config.nthreadsBatch} defaultValue={rc?.['nthreads-batch']} onChange={(v) => setConfig({ nthreadsBatch: v })} />
              <NullableNumInput label="Cache Min Tokens" value={form.config.cacheMinTokens} defaultValue={rc?.['cache-min-tokens']} onChange={(v) => setConfig({ cacheMinTokens: v })} />
              <div>
                <label style={labelStyle}>Cache Type K</label>
                <select value={form.config.cacheTypeK} onChange={(e) => setConfig({ cacheTypeK: e.target.value })} style={inputStyle}>
                  <option value="">not set</option>
                  <option value="f16">f16</option>
                  <option value="bf16">bf16</option>
                  <option value="q8_0">q8_0</option>
                  <option value="q4_0">q4_0</option>
                </select>
              </div>
              <div>
                <label style={labelStyle}>Cache Type V</label>
                <select value={form.config.cacheTypeV} onChange={(e) => setConfig({ cacheTypeV: e.target.value })} style={inputStyle}>
                  <option value="">not set</option>
                  <option value="f16">f16</option>
                  <option value="bf16">bf16</option>
                  <option value="q8_0">q8_0</option>
                  <option value="q4_0">q4_0</option>
                </select>
              </div>
              <NullableNumInput label="Context Window" value={form.config.contextWindow} defaultValue={rc?.['context-window']} onChange={(v) => setConfig({ contextWindow: v })} />
              <div>
                <label style={labelStyle}>Device</label>
                <select value={form.config.device} onChange={(e) => setConfig({ device: e.target.value })} style={inputStyle}>
                  <option value="">not set</option>
                  <option value="cpu">cpu</option>
                  <option value="cuda">cuda</option>
                  <option value="metal">metal</option>
                  <option value="vulkan">vulkan</option>
                </select>
              </div>
              <TriStateSelect label="Direct I/O" value={form.config.useDirectIO} onChange={(v) => setConfig({ useDirectIO: v })} />
              <div>
                <label style={labelStyle}>Flash Attention</label>
                <select value={form.config.flashAttention} onChange={(e) => setConfig({ flashAttention: e.target.value })} style={inputStyle}>
                  <option value="">not set</option>
                  <option value="enabled">enabled</option>
                  <option value="disabled">disabled</option>
                </select>
              </div>
              <NullableNumInput label="GPU Layers (ngpu-layers)" value={form.config.ngpuLayers} defaultValue={rc?.['ngpu-layers'] ?? undefined} onChange={(v) => setConfig({ ngpuLayers: v === null ? null : Math.round(v) })} />
              <TriStateSelect label="Ignore Integrity Check" value={form.config.ignoreIntegrityCheck} onChange={(v) => setConfig({ ignoreIntegrityCheck: v })} />
              <TriStateSelect label="Incremental Cache" value={form.config.incrementalCache} onChange={(v) => setConfig({ incrementalCache: v })} />
              <NullableNumInput label="Max Sequences (nseq-max)" value={form.config.nseqMax} defaultValue={rc?.['nseq-max']} onChange={(v) => setConfig({ nseqMax: v })} />
              <NullableNumInput label="Micro Batch Size (nubatch)" value={form.config.nubatch} defaultValue={rc?.nubatch} onChange={(v) => setConfig({ nubatch: v })} />
              <TriStateSelect label="Offload KQV" value={form.config.offloadKQV} onChange={(v) => setConfig({ offloadKQV: v })} />
              <TriStateSelect label="Op Offload" value={form.config.opOffload} onChange={(v) => setConfig({ opOffload: v })} />
              <div>
                <label style={labelStyle}>Split Mode</label>
                <select value={form.config.splitMode} onChange={(e) => setConfig({ splitMode: e.target.value })} style={inputStyle}>
                  <option value="">not set</option>
                  <option value="none">none</option>
                  <option value="layer">layer</option>
                  <option value="row">row</option>
                </select>
              </div>
              <TriStateSelect label="System Prompt Cache" value={form.config.systemPromptCache} onChange={(v) => setConfig({ systemPromptCache: v })} />
              <NullableNumInput label="Threads (nthreads)" value={form.config.nthreads} defaultValue={rc?.nthreads} onChange={(v) => setConfig({ nthreads: v })} />
            </div>

            {/* YaRN / RoPE Scaling */}
            <div style={{ marginTop: '20px' }}>
              <h4 style={{ marginBottom: '12px', fontSize: '14px', fontWeight: 600, color: 'var(--color-gray-700)' }}>RoPE / YaRN Scaling</h4>
              <div style={gridStyle}>
                <NullableNumInput label="RoPE Freq Base" value={form.config.ropeFreqBase} step="0.01" defaultValue={rc?.['rope-freq-base'] ?? undefined} onChange={(v) => setConfig({ ropeFreqBase: v })} />
                <NullableNumInput label="RoPE Freq Scale" value={form.config.ropeFreqScale} step="0.01" defaultValue={rc?.['rope-freq-scale'] ?? undefined} onChange={(v) => setConfig({ ropeFreqScale: v })} />
                <div>
                  <label style={labelStyle}>RoPE Scaling Type</label>
                  <select value={form.config.ropeScaling} onChange={(e) => setConfig({ ropeScaling: e.target.value })} style={inputStyle}>
                    <option value="">none</option>
                    <option value="linear">linear</option>
                    <option value="yarn">yarn</option>
                  </select>
                </div>
                <NullableNumInput label="YaRN Attn Factor" value={form.config.yarnAttnFactor} step="0.01" defaultValue={rc?.['yarn-attn-factor'] ?? undefined} onChange={(v) => setConfig({ yarnAttnFactor: v })} />
                <NullableNumInput label="YaRN Beta Fast" value={form.config.yarnBetaFast} step="0.01" defaultValue={rc?.['yarn-beta-fast'] ?? undefined} onChange={(v) => setConfig({ yarnBetaFast: v })} />
                <NullableNumInput label="YaRN Beta Slow" value={form.config.yarnBetaSlow} step="0.01" defaultValue={rc?.['yarn-beta-slow'] ?? undefined} onChange={(v) => setConfig({ yarnBetaSlow: v })} />
                <NullableNumInput label="YaRN Ext Factor" value={form.config.yarnExtFactor} step="0.01" defaultValue={rc?.['yarn-ext-factor'] ?? undefined} onChange={(v) => setConfig({ yarnExtFactor: v })} />
                <NullableNumInput label="YaRN Original Context" value={form.config.yarnOrigCtx} defaultValue={rc?.['yarn-orig-ctx'] ?? undefined} onChange={(v) => setConfig({ yarnOrigCtx: v === null ? null : Math.round(v) })} />
              </div>
            </div>
          </>
        )}
      </div>

      {/* Sampling */}
      <div className="card" style={sectionStyle}>
        <h3
          onClick={() => setShowSampling(!showSampling)}
          style={{ cursor: 'pointer', userSelect: 'none', marginBottom: showSampling ? '12px' : 0 }}
        >
          {showSampling ? '▼' : '▶'} Sampling Parameters
        </h3>
        {showSampling && (
          <div style={gridStyle}>
            <NullableNumInput label="DRY Allowed Length" value={form.sampling.dryAllowedLen} defaultValue={rsp?.dry_allowed_length} onChange={(v) => setSampling({ dryAllowedLen: v })} />
            <NullableNumInput label="DRY Base" value={form.sampling.dryBase} step="0.01" defaultValue={rsp?.dry_base} onChange={(v) => setSampling({ dryBase: v })} />
            <NullableNumInput label="DRY Multiplier" value={form.sampling.dryMultiplier} step="0.01" defaultValue={rsp?.dry_multiplier} onChange={(v) => setSampling({ dryMultiplier: v })} />
            <NullableNumInput label="DRY Penalty Last N" value={form.sampling.dryPenaltyLast} defaultValue={rsp?.dry_penalty_last_n} onChange={(v) => setSampling({ dryPenaltyLast: v })} />
            <div>
              <label style={labelStyle}>Enable Thinking</label>
              <select value={form.sampling.enableThinking} onChange={(e) => setSampling({ enableThinking: e.target.value })} style={inputStyle}>
                <option value="">not set</option>
                <option value="on">on</option>
                <option value="off">off</option>
              </select>
            </div>
            <NullableNumInput label="Max Tokens" value={form.sampling.maxTokens} defaultValue={rsp?.max_tokens} onChange={(v) => setSampling({ maxTokens: v })} />
            <NullableNumInput label="Min P" value={form.sampling.minP} step="0.01" defaultValue={rsp?.min_p} onChange={(v) => setSampling({ minP: v })} />
            <div>
              <label style={labelStyle}>Reasoning Effort</label>
              <select value={form.sampling.reasoningEffort} onChange={(e) => setSampling({ reasoningEffort: e.target.value })} style={inputStyle}>
                <option value="">not set</option>
                <option value="low">low</option>
                <option value="medium">medium</option>
                <option value="high">high</option>
              </select>
            </div>
            <NullableNumInput label="Repeat Last N" value={form.sampling.repeatLastN} defaultValue={rsp?.repeat_last_n} onChange={(v) => setSampling({ repeatLastN: v })} />
            <NullableNumInput label="Repeat Penalty" value={form.sampling.repeatPenalty} step="0.01" defaultValue={rsp?.repeat_penalty} onChange={(v) => setSampling({ repeatPenalty: v })} />
            <NullableNumInput label="Temperature" value={form.sampling.temperature} step="0.01" defaultValue={rsp?.temperature} onChange={(v) => setSampling({ temperature: v })} />
            <NullableNumInput label="Top K" value={form.sampling.topK} defaultValue={rsp?.top_k} onChange={(v) => setSampling({ topK: v })} />
            <NullableNumInput label="Top P" value={form.sampling.topP} step="0.01" defaultValue={rsp?.top_p} onChange={(v) => setSampling({ topP: v })} />
            <NullableNumInput label="XTC Min Keep" value={form.sampling.xtcMinKeep} defaultValue={rsp?.xtc_min_keep} onChange={(v) => setSampling({ xtcMinKeep: v })} />
            <NullableNumInput label="XTC Probability" value={form.sampling.xtcProbability} step="0.01" defaultValue={rsp?.xtc_probability} onChange={(v) => setSampling({ xtcProbability: v })} />
            <NullableNumInput label="XTC Threshold" value={form.sampling.xtcThreshold} step="0.01" defaultValue={rsp?.xtc_threshold} onChange={(v) => setSampling({ xtcThreshold: v })} />
          </div>
        )}
      </div>

      {/* Actions */}
      <div className="card" style={sectionStyle}>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
          <button className="btn btn-primary" onClick={handleSave} disabled={saving}>
            {saving ? 'Saving...' : 'Save to Catalog'}
          </button>
          {repoPath && (
            <button className="btn btn-primary" onClick={handlePublish} disabled={publishing} style={{ background: '#2e7d32' }}>
              {publishing ? 'Publishing...' : 'Publish to Repo'}
            </button>
          )}
          <button className="btn btn-secondary" onClick={() => { setForm({ ...defaultForm }); setResolvedConfig(null); setHfInput(''); setRepoFiles([]); setSaveMsg(null); }}>
            Reset
          </button>
        </div>
        {saveMsg && (
          <div
            className={saveMsg.type === 'error' ? 'alert alert-error' : 'alert'}
            style={saveMsg.type === 'success' ? { marginTop: '8px', background: '#e8f5e9', color: '#2e7d32', padding: '12px', borderRadius: '6px' } : { marginTop: '8px' }}
          >
            {saveMsg.text}
          </div>
        )}
      </div>
    </div>
  );
}
