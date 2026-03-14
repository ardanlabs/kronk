import { useState, useEffect, useRef } from 'react';
import { useSearchParams } from 'react-router-dom';
import { api } from '../services/api';
import type { CatalogModelResponse, ModelConfig, SamplingConfig } from '../types';
import { labelWithTip, type TooltipKey } from './ParamTooltips';

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
  architecture: string;
  ggufArch: string;
  parameters: string;
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
    devices: string;
    mainGpu: number | null;
    tensorSplit: string;
    autoFitVram: boolean;
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
    draftModelId: string;
    draftNDraft: number | null;
    draftNGpuLayers: number | null;
    draftDevice: string;
    moeMode: string;
    moeKeepTopN: number | null;
    useMMap: boolean | null;
    numa: string;
    opOffloadMinBatch: number | null;
  };
  sampling: {
    temperature: number | null;
    topK: number | null;
    topP: number | null;
    minP: number | null;
    presencePenalty: number | null;
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
    frequencyPenalty: number | null;
    enableThinking: string;
    reasoningEffort: string;
    grammar: string;
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
  architecture: '',
  ggufArch: '',
  parameters: '',
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
    devices: '',
    mainGpu: null,
    tensorSplit: '',
    autoFitVram: false,
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
    draftModelId: '',
    draftNDraft: null,
    draftNGpuLayers: null,
    draftDevice: '',
    moeMode: '',
    moeKeepTopN: null,
    useMMap: null,
    numa: '',
    opOffloadMinBatch: null,
  },
  sampling: {
    temperature: null,
    topK: null,
    topP: null,
    minP: null,
    presencePenalty: null,
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
    frequencyPenalty: null,
    enableThinking: '',
    reasoningEffort: '',
    grammar: '',
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
    architecture: resp.architecture || '',
    ggufArch: resp.gguf_arch || '',
    parameters: resp.parameters || '',
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
      devices: mc?.['devices']?.join(',') ?? '',
      mainGpu: mc?.['main-gpu'] ?? null,
      tensorSplit: mc?.['tensor-split']?.join(',') ?? '',
      autoFitVram: mc?.['auto-fit-vram'] ?? false,
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
      draftModelId: mc?.['draft-model']?.['model-id'] || '',
      draftNDraft: mc?.['draft-model']?.ndraft ?? null,
      draftNGpuLayers: mc?.['draft-model']?.['ngpu-layers'] ?? null,
      draftDevice: mc?.['draft-model']?.device || '',
      moeMode: mc?.moe?.mode || '',
      moeKeepTopN: mc?.moe?.['keep-experts-top-n'] ?? null,
      useMMap: mc?.['use-mmap'] ?? null,
      numa: mc?.numa || '',
      opOffloadMinBatch: mc?.['op-offload-min-batch'] ?? null,
    },
    sampling: {
      temperature: sp?.temperature ?? null,
      topK: sp?.top_k ?? null,
      topP: sp?.top_p ?? null,
      minP: sp?.min_p ?? null,
      presencePenalty: sp?.presence_penalty ?? null,
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
      frequencyPenalty: sp?.frequency_penalty ?? null,
      enableThinking: sp?.enable_thinking || '',
      reasoningEffort: sp?.reasoning_effort || '',
      grammar: sp?.grammar || '',
    },
  };
}

function defaultPlaceholder(defaultValue: number | undefined): string {
  if (defaultValue === undefined || defaultValue === 0) return '';
  return `default: ${defaultValue}`;
}

function NullableNumInput({ label, value, step, defaultValue, tooltipKey, onChange }: { label: string; value: number | null; step?: string; defaultValue?: number; tooltipKey?: TooltipKey; onChange: (v: number | null) => void }) {
  return (
    <div>
      <label style={labelStyle}>{tooltipKey ? labelWithTip(label, tooltipKey) : label}</label>
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

function TriStateSelect({ label, value, tooltipKey, onChange }: { label: string; value: boolean | null; tooltipKey?: TooltipKey; onChange: (v: boolean | null) => void }) {
  const strVal = value === null ? '' : value ? 'true' : 'false';
  return (
    <div>
      <label style={labelStyle}>{tooltipKey ? labelWithTip(label, tooltipKey) : label}</label>
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
  const [templateFiles, setTemplateFiles] = useState<string[]>([]);
  const [grammarFiles, setGrammarFiles] = useState<string[]>([]);
  const pendingVramConfigRef = useRef<Record<string, unknown> | null>(null);

  useEffect(() => {
    api.listCatalogFiles().then(setCatalogFiles).catch(() => {});
    api.getCatalogRepoPath().then((r) => setRepoPath(r.repo_path)).catch(() => {});
    api.listTemplates().then((r) => setTemplateFiles(r.files || [])).catch(() => {});
    api.listGrammars().then((r) => setGrammarFiles(r.files || [])).catch(() => {});
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

  useEffect(() => {
    if (searchParams.get('source') === 'playground') {
      const draftStr = sessionStorage.getItem('kronk_catalog_draft');
      if (draftStr) {
        try {
          const draft = JSON.parse(draftStr);
          setForm((prev) => ({
            ...prev,
            id: draft.id || prev.id,
            template: draft.template || prev.template,
            capabilities: {
              ...prev.capabilities,
              streaming: draft.capabilities?.streaming ?? prev.capabilities.streaming,
              tooling: draft.capabilities?.tooling ?? prev.capabilities.tooling,
            },
            config: {
              ...prev.config,
              contextWindow: draft.config?.['context-window'] ?? prev.config.contextWindow,
              nbatch: draft.config?.nbatch ?? prev.config.nbatch,
              nubatch: draft.config?.nubatch ?? prev.config.nubatch,
              nseqMax: draft.config?.['nseq-max'] ?? prev.config.nseqMax,
              flashAttention: draft.config?.['flash-attention'] ?? prev.config.flashAttention,
              cacheTypeK: draft.config?.['cache-type-k'] ?? prev.config.cacheTypeK,
              cacheTypeV: draft.config?.['cache-type-v'] ?? prev.config.cacheTypeV,
              systemPromptCache: draft.config?.['system-prompt-cache'] ?? prev.config.systemPromptCache,
            },
          }));
          sessionStorage.removeItem('kronk_catalog_draft');
        } catch {
          // Ignore invalid draft
        }
      }
    }
  }, [searchParams]);

  const vramImportedRef = useRef(false);
  useEffect(() => {
    if (searchParams.get('source') !== 'vram') return;
    if (vramImportedRef.current) return;

    const draftStr = sessionStorage.getItem('kronk_catalog_draft');
    if (!draftStr) return;

    let draft: { source?: string; modelUrl?: string; config?: Record<string, unknown> };
    try {
      draft = JSON.parse(draftStr);
    } catch {
      return;
    }
    if (draft.source !== 'vram' || !draft.modelUrl) return;

    vramImportedRef.current = true;

    const vramConfig = draft.config ?? {};
    const rawUrl = draft.modelUrl;

    // Parse modelUrl to derive lookup input and file-matching hints.
    const isUrl = /^https?:\/\//i.test(rawUrl);
    let lookupInput: string;
    let selectedFilename: string | undefined;
    let quantizationHint: string | undefined;

    if (isUrl) {
      // Full URL: extract owner/repo and optionally the specific .gguf filename.
      const hfUrlMatch = rawUrl.match(/^https?:\/\/huggingface\.co\/([^/]+\/[^/]+)(?:\/(?:resolve|blob)\/[^/]+\/(.+\.gguf))?/i);
      if (hfUrlMatch) {
        lookupInput = hfUrlMatch[1];
        if (hfUrlMatch[2]) {
          selectedFilename = hfUrlMatch[2];
        }
      } else {
        // Unrecognized URL format — pass directly to backend which knows how to resolve it.
        lookupInput = rawUrl;
      }
    } else {
      // Shorthand: owner/repo:TAG → split on colon for quantization hint.
      const colonIdx = rawUrl.indexOf(':');
      if (colonIdx > 0) {
        lookupInput = rawUrl.substring(0, colonIdx);
        quantizationHint = rawUrl.substring(colonIdx + 1);
      } else {
        lookupInput = rawUrl;
      }
    }

    setHfInput(lookupInput);
    setHfLoading(true);
    setHfError(null);
    setRepoFiles([]);
    setShowConfig(true);
    pendingVramConfigRef.current = vramConfig;

    const mergeVramConfig = (prev: CatalogFormData): CatalogFormData => ({
      ...prev,
      config: {
        ...prev.config,
        contextWindow: (vramConfig.contextWindow as number) ?? prev.config.contextWindow,
        nseqMax: (vramConfig.nseqMax as number) ?? prev.config.nseqMax,
        cacheTypeK: (vramConfig.cacheTypeK as string) || prev.config.cacheTypeK,
        cacheTypeV: (vramConfig.cacheTypeV as string) || prev.config.cacheTypeV,
        flashAttention: (vramConfig.flashAttention as string) || prev.config.flashAttention,
        ngpuLayers: (vramConfig.ngpuLayers as number | null) ?? prev.config.ngpuLayers,
        offloadKQV: vramConfig.offloadKQV != null ? (vramConfig.offloadKQV as boolean | null) : prev.config.offloadKQV,
        splitMode: (vramConfig.splitMode as string) || prev.config.splitMode,
        tensorSplit: (vramConfig.tensorSplit as string) || prev.config.tensorSplit,
        moeMode: (vramConfig.moeMode as string) || prev.config.moeMode,
        moeKeepTopN: (vramConfig.moeKeepTopN as number | null) ?? prev.config.moeKeepTopN,
      },
    });

    const doFileLookup = async (repoBase: string, filename: string) => {
      const fullInput = `${repoBase}/${filename}`;
      setHfInput(fullInput);
      try {
        const result = await api.lookupHuggingFace(fullInput);
        const populated = populateFromResponse(result.model);
        setForm((prev) => mergeVramConfig({
          ...prev,
          ...populated,
          catalogFile: prev.catalogFile,
          newCatalogFile: prev.newCatalogFile,
        }));
        setRepoFiles([]);
      } catch (err) {
        setHfError(err instanceof Error ? err.message : 'File lookup failed');
      } finally {
        setHfLoading(false);
      }
    };

    (async () => {
      try {
        const result = await api.lookupHuggingFace(lookupInput);
        const populated = populateFromResponse(result.model);

        // Check if repo_files came back (repo-level lookup without specific file).
        const files: HFRepoFile[] = result.repo_files ?? [];

        if (files.length > 0) {
          // Try to auto-select the right file.
          let match: string | undefined;

          if (selectedFilename) {
            const found = files.find((f) => f.filename === selectedFilename);
            if (found) match = found.filename;
          }

          if (!match && quantizationHint) {
            const hint = quantizationHint.toLowerCase();
            const candidates = files.filter((f) =>
              f.filename.toLowerCase().endsWith('.gguf') &&
              f.filename.toLowerCase().includes(hint.toLowerCase()),
            );
            if (candidates.length === 1) {
              match = candidates[0].filename;
            }
          }

          if (match) {
            await doFileLookup(lookupInput, match);
            sessionStorage.removeItem('kronk_catalog_draft');
            return;
          }

          // No unambiguous match — show repo files for manual selection,
          // but still apply the VRAM config overlay.
          setForm((prev) => mergeVramConfig({
            ...prev,
            ...populated,
            catalogFile: prev.catalogFile,
            newCatalogFile: prev.newCatalogFile,
          }));
          setRepoFiles(files);
          setHfLoading(false);
        } else {
          // Direct file match (no repo_files) — apply populated + VRAM config.
          setForm((prev) => mergeVramConfig({
            ...prev,
            ...populated,
            catalogFile: prev.catalogFile,
            newCatalogFile: prev.newCatalogFile,
          }));
          setHfLoading(false);
        }
        sessionStorage.removeItem('kronk_catalog_draft');
      } catch (err) {
        setHfError(err instanceof Error ? err.message : 'Lookup failed');
        setHfLoading(false);
      }
    })();
  }, [searchParams]);

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
      const vc = pendingVramConfigRef.current;
      setForm((prev) => {
        const merged = {
          ...prev,
          ...populated,
          catalogFile: prev.catalogFile,
          newCatalogFile: prev.newCatalogFile,
        };
        if (!vc) return merged;
        return {
          ...merged,
          config: {
            ...merged.config,
            contextWindow: (vc.contextWindow as number) ?? merged.config.contextWindow,
            nseqMax: (vc.nseqMax as number) ?? merged.config.nseqMax,
            cacheTypeK: (vc.cacheTypeK as string) || merged.config.cacheTypeK,
            cacheTypeV: (vc.cacheTypeV as string) || merged.config.cacheTypeV,
            flashAttention: (vc.flashAttention as string) || merged.config.flashAttention,
            ngpuLayers: (vc.ngpuLayers as number | null) ?? merged.config.ngpuLayers,
            offloadKQV: vc.offloadKQV != null ? (vc.offloadKQV as boolean | null) : merged.config.offloadKQV,
            splitMode: (vc.splitMode as string) || merged.config.splitMode,
            tensorSplit: (vc.tensorSplit as string) || merged.config.tensorSplit,
            moeMode: (vc.moeMode as string) || merged.config.moeMode,
            moeKeepTopN: (vc.moeKeepTopN as number | null) ?? merged.config.moeKeepTopN,
          },
        };
      });
      pendingVramConfigRef.current = null;
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
        architecture: form.architecture,
        gguf_arch: form.ggufArch,
        parameters: form.parameters,
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
          'split-mode': form.config.splitMode || null,
          'devices': form.config.devices ? form.config.devices.split(',').map(s => s.trim()).filter(Boolean) : null,
          'main-gpu': form.config.mainGpu,
          'tensor-split': form.config.tensorSplit ? form.config.tensorSplit.split(',').map(s => parseFloat(s.trim())).filter(n => !isNaN(n)) : null,
          'tensor-buft-overrides': null,
          'auto-fit-vram': form.config.autoFitVram,
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
          ...(form.config.draftModelId ? {
            'draft-model': {
              'model-id': form.config.draftModelId,
              ndraft: form.config.draftNDraft ?? 0,
              'ngpu-layers': form.config.draftNGpuLayers,
              device: form.config.draftDevice || undefined,
            },
          } : {}),
          ...(form.config.moeMode ? {
            moe: {
              mode: form.config.moeMode,
              ...(form.config.moeMode === 'keep_top_n' && form.config.moeKeepTopN != null ? {
                'keep-experts-top-n': form.config.moeKeepTopN,
              } : {}),
            },
          } : {}),
          'use-mmap': form.config.useMMap,
          numa: form.config.numa || null,
          'op-offload-min-batch': form.config.opOffloadMinBatch ?? 0,
          'sampling-parameters': {
            temperature: form.sampling.temperature ?? 0,
            top_k: form.sampling.topK ?? 0,
            top_p: form.sampling.topP ?? 0,
            min_p: form.sampling.minP ?? 0,
            presence_penalty: form.sampling.presencePenalty ?? 0,
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
            frequency_penalty: form.sampling.frequencyPenalty ?? 0,
            enable_thinking: (form.sampling.enableThinking || '') as SamplingConfig['enable_thinking'],
            reasoning_effort: (form.sampling.reasoningEffort || '') as SamplingConfig['reasoning_effort'],
            grammar: form.sampling.grammar ?? '',
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
            <label style={labelStyle}>Architecture</label>
            <select value={form.architecture} onChange={(e) => setForm({ ...form, architecture: e.target.value })} style={inputStyle}>
              <option value="">not set</option>
              <option value="Dense">Dense</option>
              <option value="MoE">MoE</option>
              <option value="Hybrid">Hybrid</option>
            </select>
          </div>
          <div>
            <label style={labelStyle}>GGUF Arch</label>
            <input type="text" value={form.ggufArch} onChange={(e) => setForm({ ...form, ggufArch: e.target.value })} style={inputStyle} placeholder="e.g. llama, qwen2moe" />
          </div>
          <div>
            <label style={labelStyle}>Parameters</label>
            <input type="text" value={form.parameters} onChange={(e) => setForm({ ...form, parameters: e.target.value })} style={inputStyle} placeholder="e.g. 8B, 70B, 0.6B" />
          </div>
          <div>
            <label style={labelStyle}>Web Page</label>
            <input type="text" value={form.webPage} onChange={(e) => setForm({ ...form, webPage: e.target.value })} style={inputStyle} />
          </div>
          <div>
            <label style={labelStyle}>Template</label>
            <select value={form.template} onChange={(e) => setForm({ ...form, template: e.target.value })} style={inputStyle}>
              <option value="">not set</option>
              {templateFiles.map((f) => (
                <option key={f} value={f}>{f}</option>
              ))}
            </select>
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
              <input type="text" value={file.size} onChange={(e) => updateFile(idx, 'size', e.target.value)} style={inputStyle} placeholder="8.71 GB" />
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
              <NullableNumInput label="Batch Size (nbatch)" tooltipKey="nbatch" value={form.config.nbatch} defaultValue={rc?.nbatch} onChange={(v) => setConfig({ nbatch: v })} />
              <NullableNumInput label="Micro Batch Size (nubatch)" tooltipKey="nubatch" value={form.config.nubatch} defaultValue={rc?.nubatch} onChange={(v) => setConfig({ nubatch: v })} />
              <div>
                <label style={labelStyle}>{labelWithTip('Cache Type K', 'cacheTypeK')}</label>
                <select value={form.config.cacheTypeK} onChange={(e) => setConfig({ cacheTypeK: e.target.value })} style={inputStyle}>
                  <option value="">not set</option>
                  <option value="f16">f16</option>
                  <option value="bf16">bf16</option>
                  <option value="q8_0">q8_0</option>
                  <option value="q4_0">q4_0</option>
                </select>
              </div>
              <div>
                <label style={labelStyle}>{labelWithTip('Cache Type V', 'cacheTypeV')}</label>
                <select value={form.config.cacheTypeV} onChange={(e) => setConfig({ cacheTypeV: e.target.value })} style={inputStyle}>
                  <option value="">not set</option>
                  <option value="f16">f16</option>
                  <option value="bf16">bf16</option>
                  <option value="q8_0">q8_0</option>
                  <option value="q4_0">q4_0</option>
                </select>
              </div>
              <NullableNumInput label="Context Window" tooltipKey="contextWindow" value={form.config.contextWindow} defaultValue={rc?.['context-window']} onChange={(v) => setConfig({ contextWindow: v })} />
              <NullableNumInput label="Max Sequences (nseq-max)" tooltipKey="nSeqMax" value={form.config.nseqMax} defaultValue={rc?.['nseq-max']} onChange={(v) => setConfig({ nseqMax: v })} />
              <TriStateSelect label="System Prompt Cache" tooltipKey="systemPromptCache" value={form.config.systemPromptCache} onChange={(v) => setConfig({ systemPromptCache: v })} />
              <TriStateSelect label="Incremental Cache" tooltipKey="incrementalCache" value={form.config.incrementalCache} onChange={(v) => setConfig({ incrementalCache: v })} />
              <NullableNumInput label="Batch Threads (nthreads-batch)" tooltipKey="nthreadsBatch" value={form.config.nthreadsBatch} defaultValue={rc?.['nthreads-batch']} onChange={(v) => setConfig({ nthreadsBatch: v })} />
              <NullableNumInput label="Cache Min Tokens" tooltipKey="cacheMinTokens" value={form.config.cacheMinTokens} defaultValue={rc?.['cache-min-tokens']} onChange={(v) => setConfig({ cacheMinTokens: v })} />
              <div>
                <label style={labelStyle}>{labelWithTip('Device', 'device')}</label>
                <select value={form.config.device} onChange={(e) => setConfig({ device: e.target.value })} style={inputStyle}>
                  <option value="">not set</option>
                  <option value="cpu">cpu</option>
                  <option value="cuda">cuda</option>
                  <option value="metal">metal</option>
                  <option value="vulkan">vulkan</option>
                  <option value="rocm">rocm</option>
                </select>
              </div>
              <TriStateSelect label="Direct I/O" tooltipKey="useDirectIO" value={form.config.useDirectIO} onChange={(v) => setConfig({ useDirectIO: v })} />
              <div>
                <label style={labelStyle}>{labelWithTip('Flash Attention', 'flashAttention')}</label>
                <select value={form.config.flashAttention} onChange={(e) => setConfig({ flashAttention: e.target.value })} style={inputStyle}>
                  <option value="">not set</option>
                  <option value="enabled">enabled</option>
                  <option value="disabled">disabled</option>
                </select>
              </div>
              <NullableNumInput label="GPU Layers (ngpu-layers)" tooltipKey="ngpuLayers" value={form.config.ngpuLayers} defaultValue={rc?.['ngpu-layers'] ?? undefined} onChange={(v) => setConfig({ ngpuLayers: v === null ? null : Math.round(v) })} />
              <TriStateSelect label="Ignore Integrity Check" tooltipKey="ignoreIntegrityCheck" value={form.config.ignoreIntegrityCheck} onChange={(v) => setConfig({ ignoreIntegrityCheck: v })} />
              <TriStateSelect label="Offload KQV" tooltipKey="offloadKQV" value={form.config.offloadKQV} onChange={(v) => setConfig({ offloadKQV: v })} />
              <TriStateSelect label="Op Offload" tooltipKey="opOffload" value={form.config.opOffload} onChange={(v) => setConfig({ opOffload: v })} />
              <div>
                <label style={labelStyle}>{labelWithTip('Split Mode', 'splitMode')}</label>
                <select value={form.config.splitMode} onChange={(e) => setConfig({ splitMode: e.target.value })} style={inputStyle}>
                  <option value="">not set</option>
                  <option value="none">none</option>
                  <option value="layer">layer</option>
                  <option value="row">row</option>
                </select>
              </div>
              <NullableNumInput label="Main GPU" tooltipKey="mainGpu" value={form.config.mainGpu} onChange={(v) => setConfig({ mainGpu: v === null ? null : Math.round(v) })} />
              <div>
                <label style={labelStyle}>{labelWithTip('Devices', 'devices')}</label>
                <input type="text" value={form.config.devices} onChange={(e) => setConfig({ devices: e.target.value })} style={inputStyle} placeholder="e.g., CUDA0,CUDA1" />
              </div>
              <div>
                <label style={labelStyle}>{labelWithTip('Tensor Split', 'tensorSplit')}</label>
                <input type="text" value={form.config.tensorSplit} onChange={(e) => setConfig({ tensorSplit: e.target.value })} style={inputStyle} placeholder="e.g., 0.6,0.4" />
              </div>
              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', marginTop: '8px' }}>
                <input type="checkbox" checked={form.config.autoFitVram} onChange={(e) => setConfig({ autoFitVram: e.target.checked })} />
                <span style={{ fontSize: '14px', fontWeight: 600, color: 'var(--color-gray-700)' }}>{labelWithTip('Auto-Fit VRAM', 'autoFitVram')}</span>
              </label>
              <NullableNumInput label="Threads (nthreads)" tooltipKey="nthreads" value={form.config.nthreads} defaultValue={rc?.nthreads} onChange={(v) => setConfig({ nthreads: v })} />
            </div>

            {/* YaRN / RoPE Scaling */}
            <div style={{ marginTop: '20px' }}>
              <h4 style={{ marginBottom: '12px', fontSize: '14px', fontWeight: 600, color: 'var(--color-gray-700)' }}>RoPE / YaRN Scaling</h4>
              <div style={gridStyle}>
                <NullableNumInput label="RoPE Freq Base" tooltipKey="ropeFreqBase" value={form.config.ropeFreqBase} step="0.01" defaultValue={rc?.['rope-freq-base'] ?? undefined} onChange={(v) => setConfig({ ropeFreqBase: v })} />
                <NullableNumInput label="RoPE Freq Scale" tooltipKey="ropeFreqScale" value={form.config.ropeFreqScale} step="0.01" defaultValue={rc?.['rope-freq-scale'] ?? undefined} onChange={(v) => setConfig({ ropeFreqScale: v })} />
                <div>
                  <label style={labelStyle}>{labelWithTip('RoPE Scaling Type', 'ropeScaling')}</label>
                  <select value={form.config.ropeScaling} onChange={(e) => setConfig({ ropeScaling: e.target.value })} style={inputStyle}>
                    <option value="">none</option>
                    <option value="linear">linear</option>
                    <option value="yarn">yarn</option>
                  </select>
                </div>
                <NullableNumInput label="YaRN Attn Factor" tooltipKey="yarnAttnFactor" value={form.config.yarnAttnFactor} step="0.01" defaultValue={rc?.['yarn-attn-factor'] ?? undefined} onChange={(v) => setConfig({ yarnAttnFactor: v })} />
                <NullableNumInput label="YaRN Beta Fast" tooltipKey="yarnBetaFast" value={form.config.yarnBetaFast} step="0.01" defaultValue={rc?.['yarn-beta-fast'] ?? undefined} onChange={(v) => setConfig({ yarnBetaFast: v })} />
                <NullableNumInput label="YaRN Beta Slow" tooltipKey="yarnBetaSlow" value={form.config.yarnBetaSlow} step="0.01" defaultValue={rc?.['yarn-beta-slow'] ?? undefined} onChange={(v) => setConfig({ yarnBetaSlow: v })} />
                <NullableNumInput label="YaRN Ext Factor" tooltipKey="yarnExtFactor" value={form.config.yarnExtFactor} step="0.01" defaultValue={rc?.['yarn-ext-factor'] ?? undefined} onChange={(v) => setConfig({ yarnExtFactor: v })} />
                <NullableNumInput label="YaRN Original Context" tooltipKey="yarnOrigCtx" value={form.config.yarnOrigCtx} defaultValue={rc?.['yarn-orig-ctx'] ?? undefined} onChange={(v) => setConfig({ yarnOrigCtx: v === null ? null : Math.round(v) })} />
              </div>
            </div>

            {/* Speculative Decoding (Draft Model) */}
            <div style={{ marginTop: '20px' }}>
              <h4 style={{ marginBottom: '12px', fontSize: '14px', fontWeight: 600, color: 'var(--color-gray-700)' }}>Speculative Decoding</h4>
              <div style={gridStyle}>
                <div>
                  <label style={labelStyle}>{labelWithTip('Draft Model ID', 'draftModel')}</label>
                  <input
                    type="text"
                    value={form.config.draftModelId}
                    onChange={(e) => setConfig({ draftModelId: e.target.value })}
                    style={inputStyle}
                    placeholder="e.g., Qwen3-0.6B-Q8_0"
                  />
                </div>
                <NullableNumInput label="Draft Tokens (ndraft)" tooltipKey="draftTokens" value={form.config.draftNDraft} onChange={(v) => setConfig({ draftNDraft: v === null ? null : Math.round(v) })} />
                <NullableNumInput label="Draft GPU Layers" tooltipKey="draftGpuLayers" value={form.config.draftNGpuLayers} onChange={(v) => setConfig({ draftNGpuLayers: v === null ? null : Math.round(v) })} />
                <div>
                  <label style={labelStyle}>{labelWithTip('Draft Device', 'draftDevice')}</label>
                  <input
                    type="text"
                    value={form.config.draftDevice}
                    onChange={(e) => setConfig({ draftDevice: e.target.value })}
                    style={inputStyle}
                    placeholder="e.g., GPU1"
                  />
                </div>
              </div>
            </div>

            {/* MoE Configuration */}
            <div style={{ marginTop: '20px' }}>
              <h4 style={{ marginBottom: '12px', fontSize: '14px', fontWeight: 600, color: 'var(--color-gray-700)' }}>MoE (Expert Placement)</h4>
              <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', marginBottom: '12px' }}>
                <button
                  type="button"
                  className="btn btn-secondary"
                  style={{ fontSize: '0.85em', padding: '6px 12px' }}
                  title="Experts on CPU, flash attention, large batches — best for GPUs with 24-48GB VRAM"
                  onClick={() => setConfig({
                    moeMode: 'experts_cpu',
                    flashAttention: 'enabled',
                    nbatch: 4096,
                    nubatch: 512,
                  })}
                >
                  💾 CPU Experts (24-48GB GPU)
                </button>
                <button
                  type="button"
                  className="btn btn-secondary"
                  style={{ fontSize: '0.85em', padding: '6px 12px' }}
                  title="All experts on GPU with flash attention — requires very large VRAM (often 80GB+ for 70B-class MoE at large context)"
                  onClick={() => setConfig({
                    moeMode: 'experts_gpu',
                    flashAttention: 'enabled',
                  })}
                >
                  ⚡ Full GPU (80GB+)
                </button>
                <button
                  type="button"
                  className="btn btn-secondary"
                  style={{ fontSize: '0.85em', padding: '6px 12px' }}
                  title="Row split for multi-GPU, experts on CPU, flash attention, large batches"
                  onClick={() => setConfig({
                    moeMode: 'experts_cpu',
                    splitMode: 'row',
                    flashAttention: 'enabled',
                    nbatch: 4096,
                    nubatch: 512,
                  })}
                >
                  🖥️ Multi-GPU
                </button>
              </div>
              <div style={gridStyle}>
                <div>
                  <label style={labelStyle}>{labelWithTip('Expert Strategy', 'moeMode')}</label>
                  <select value={form.config.moeMode} onChange={(e) => setConfig({ moeMode: e.target.value })} style={inputStyle}>
                    <option value="">not set (auto)</option>
                    <option value="experts_cpu">💾 Save GPU Memory — experts on CPU</option>
                    <option value="keep_top_n">⚖️ Balanced — keep top N on GPU</option>
                    <option value="experts_gpu">⚡ Maximum Speed — all on GPU</option>
                    <option value="custom">🔧 Advanced (tensor overrides)</option>
                  </select>
                </div>
                {form.config.moeMode === 'keep_top_n' && (
                  <NullableNumInput label="Expert Layers on GPU (top N)" tooltipKey="moeKeepExpertsTopN" value={form.config.moeKeepTopN} onChange={(v) => setConfig({ moeKeepTopN: v === null ? null : Math.round(v) })} />
                )}
              </div>
            </div>

            {/* NUMA / mmap */}
            <div style={{ marginTop: '20px' }}>
              <h4 style={{ marginBottom: '12px', fontSize: '14px', fontWeight: 600, color: 'var(--color-gray-700)' }}>Advanced System Tuning</h4>
              <div style={gridStyle}>
                <TriStateSelect label="Use mmap" tooltipKey="useMMap" value={form.config.useMMap} onChange={(v) => setConfig({ useMMap: v })} />
                <div>
                  <label style={labelStyle}>{labelWithTip('NUMA Strategy', 'numa')}</label>
                  <select value={form.config.numa} onChange={(e) => setConfig({ numa: e.target.value })} style={inputStyle}>
                    <option value="">disabled</option>
                    <option value="distribute">distribute</option>
                    <option value="isolate">isolate</option>
                    <option value="numactl">numactl</option>
                    <option value="mirror">mirror</option>
                  </select>
                </div>
                <NullableNumInput label="Op Offload Min Batch" tooltipKey="opOffloadMinBatch" value={form.config.opOffloadMinBatch} defaultValue={rc?.['op-offload-min-batch'] ?? undefined} onChange={(v) => setConfig({ opOffloadMinBatch: v === null ? null : Math.round(v) })} />
              </div>
              <div style={{ marginTop: '12px', padding: '12px', background: 'var(--color-gray-100)', borderRadius: '6px', fontSize: '0.85em' }}>
                <strong>💡 MoE Performance Tips:</strong>
                <ul style={{ margin: '4px 0 8px', paddingLeft: '20px', lineHeight: '1.6' }}>
                  <li>For multi-CPU server systems running experts on CPU: set <strong>Use mmap = No</strong> and <strong>NUMA = distribute</strong> for best memory bandwidth</li>
                  <li>Without <code>numa: distribute</code>, cross-socket memory access can cause significant bandwidth collapse</li>
                  <li>Without disabling mmap, tensor data is memory-mapped and may not be placed on the optimal NUMA node</li>
                </ul>
                <strong>Op-Offload Min Batch:</strong>
                <p style={{ margin: '4px 0', lineHeight: '1.6' }}>
                  Controls the minimum batch size for offloading host tensor operations to GPU during prompt processing.
                  Default is 32 tokens. For large MoE models with many CPU weights, values of 200–500+ may improve prompt ingestion speed.
                  Configure via the field above or in <code>model_config.yaml</code> as <code>op-offload-min-batch</code>.
                </p>
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
            <NullableNumInput label="DRY Allowed Length" tooltipKey="dry_allowed_length" value={form.sampling.dryAllowedLen} defaultValue={rsp?.dry_allowed_length} onChange={(v) => setSampling({ dryAllowedLen: v })} />
            <NullableNumInput label="DRY Base" tooltipKey="dry_base" value={form.sampling.dryBase} step="0.01" defaultValue={rsp?.dry_base} onChange={(v) => setSampling({ dryBase: v })} />
            <NullableNumInput label="DRY Multiplier" tooltipKey="dry_multiplier" value={form.sampling.dryMultiplier} step="0.01" defaultValue={rsp?.dry_multiplier} onChange={(v) => setSampling({ dryMultiplier: v })} />
            <NullableNumInput label="DRY Penalty Last N" tooltipKey="dry_penalty_last_n" value={form.sampling.dryPenaltyLast} defaultValue={rsp?.dry_penalty_last_n} onChange={(v) => setSampling({ dryPenaltyLast: v })} />
            <div>
              <label style={labelStyle}>{labelWithTip('Enable Thinking', 'enable_thinking')}</label>
              <select value={form.sampling.enableThinking} onChange={(e) => setSampling({ enableThinking: e.target.value })} style={inputStyle}>
                <option value="">not set</option>
                <option value="on">on</option>
                <option value="off">off</option>
              </select>
            </div>
            <NullableNumInput label="Max Tokens" tooltipKey="max_tokens" value={form.sampling.maxTokens} defaultValue={rsp?.max_tokens} onChange={(v) => setSampling({ maxTokens: v })} />
            <NullableNumInput label="Min P" tooltipKey="min_p" value={form.sampling.minP} step="0.01" defaultValue={rsp?.min_p} onChange={(v) => setSampling({ minP: v })} />
            <div>
              <label style={labelStyle}>{labelWithTip('Reasoning Effort', 'reasoning_effort')}</label>
              <select value={form.sampling.reasoningEffort} onChange={(e) => setSampling({ reasoningEffort: e.target.value })} style={inputStyle}>
                <option value="">not set</option>
                <option value="low">low</option>
                <option value="medium">medium</option>
                <option value="high">high</option>
              </select>
            </div>
            <div>
              <label style={labelStyle}>{labelWithTip('Grammar', 'grammar')}</label>
              <select value={form.sampling.grammar} onChange={(e) => setSampling({ grammar: e.target.value })} style={inputStyle}>
                <option value="">empty</option>
                {grammarFiles.map((f) => (
                  <option key={f} value={f}>{f}</option>
                ))}
              </select>
            </div>
            <NullableNumInput label="Repeat Last N" tooltipKey="repeat_last_n" value={form.sampling.repeatLastN} defaultValue={rsp?.repeat_last_n} onChange={(v) => setSampling({ repeatLastN: v })} />
            <NullableNumInput label="Repeat Penalty" tooltipKey="repeat_penalty" value={form.sampling.repeatPenalty} step="0.01" defaultValue={rsp?.repeat_penalty} onChange={(v) => setSampling({ repeatPenalty: v })} />
            <NullableNumInput label="Frequency Penalty" tooltipKey="frequency_penalty" value={form.sampling.frequencyPenalty} step="0.01" defaultValue={rsp?.frequency_penalty} onChange={(v) => setSampling({ frequencyPenalty: v })} />
            <NullableNumInput label="Presence Penalty" tooltipKey="presence_penalty" value={form.sampling.presencePenalty} step="0.01" defaultValue={rsp?.presence_penalty} onChange={(v) => setSampling({ presencePenalty: v })} />
            <NullableNumInput label="Temperature" tooltipKey="temperature" value={form.sampling.temperature} step="0.01" defaultValue={rsp?.temperature} onChange={(v) => setSampling({ temperature: v })} />
            <NullableNumInput label="Top K" tooltipKey="top_k" value={form.sampling.topK} defaultValue={rsp?.top_k} onChange={(v) => setSampling({ topK: v })} />
            <NullableNumInput label="Top P" tooltipKey="top_p" value={form.sampling.topP} step="0.01" defaultValue={rsp?.top_p} onChange={(v) => setSampling({ topP: v })} />
            <NullableNumInput label="XTC Min Keep" tooltipKey="xtc_min_keep" value={form.sampling.xtcMinKeep} defaultValue={rsp?.xtc_min_keep} onChange={(v) => setSampling({ xtcMinKeep: v })} />
            <NullableNumInput label="XTC Probability" tooltipKey="xtc_probability" value={form.sampling.xtcProbability} step="0.01" defaultValue={rsp?.xtc_probability} onChange={(v) => setSampling({ xtcProbability: v })} />
            <NullableNumInput label="XTC Threshold" tooltipKey="xtc_threshold" value={form.sampling.xtcThreshold} step="0.01" defaultValue={rsp?.xtc_threshold} onChange={(v) => setSampling({ xtcThreshold: v })} />
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
            <button className="btn btn-primary" onClick={handlePublish} disabled={publishing} style={{ background: 'var(--color-success-dark)' }}>
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
            style={saveMsg.type === 'success' ? { marginTop: '8px', background: 'var(--color-success-bg)', color: 'var(--color-success-dark)', padding: '12px', borderRadius: '6px' } : { marginTop: '8px' }}
          >
            {saveMsg.text}
          </div>
        )}
      </div>
    </div>
  );
}
