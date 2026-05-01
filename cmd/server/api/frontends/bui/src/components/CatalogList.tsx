import { useState, useEffect, useMemo, useCallback, useRef, type ReactNode } from 'react';
import { api } from '../services/api';
import { useDownload } from '../contexts/DownloadContext';
import type { CatalogModelResponse, CatalogModelsResponse, CatalogCapabilities, VRAMCalculatorResponse } from '../types';
import { ParamTooltip, labelWithTip } from './ParamTooltips';
import { extractContextInfo } from '../lib/context';
import { formatBytes } from '../lib/format';
import ResizablePanel from './ResizablePanel';
import KeyValueTable from './KeyValueTable';
import ModelCard from './ModelCard';
import CodeBlock from './CodeBlock';
import DownloadInfoTable from './DownloadInfoTable';
import DownloadProgressBar from './DownloadProgressBar';
import { VRAMFormulaModal, VRAMCalculatorPanel, useVRAMState } from './vram';

type DetailSection = 'model-card' | 'catalog' | 'template' | 'vram' | 'pull';

const SECTION_LABELS: Record<DetailSection, string> = {
  'model-card': 'Model Card',
  catalog: 'Catalog',
  template: 'Template',
  vram: 'VRAM Calculator',
  pull: 'Pull',
};


// ---------------------------------------------------------------------------
// Size slider helpers (logarithmic scale)
// ---------------------------------------------------------------------------

const MB = 1024 * 1024;
const GB = 1024 * 1024 * 1024;
const TB = 1024 * 1024 * 1024 * 1024;
const SIZE_MAX_BYTES = 4 * TB; // 4 TiB
const SLIDER_STEPS = 1000;

function sliderToBytes(pos: number): number {
  if (pos <= 0) return 0;
  if (pos >= SLIDER_STEPS) return SIZE_MAX_BYTES;
  const logMin = Math.log(MB);
  const logMax = Math.log(SIZE_MAX_BYTES);
  return Math.exp(logMin + (pos / SLIDER_STEPS) * (logMax - logMin));
}

function bytesToSlider(bytes: number): number {
  if (bytes <= 0) return 0;
  if (bytes >= SIZE_MAX_BYTES) return SLIDER_STEPS;
  const logMin = Math.log(MB);
  const logMax = Math.log(SIZE_MAX_BYTES);
  return Math.round(((Math.log(Math.max(bytes, MB)) - logMin) / (logMax - logMin)) * SLIDER_STEPS);
}

type SizeUnit = 'MB' | 'GB' | 'TB';

const UNIT_MULT: Record<SizeUnit, number> = { MB, GB, TB };

function bytesToBestUnit(bytes: number): { value: string; unit: SizeUnit } {
  if (bytes >= TB) return { value: (bytes / TB).toFixed(1), unit: 'TB' };
  if (bytes >= GB) return { value: (bytes / GB).toFixed(1), unit: 'GB' };
  return { value: Math.round(bytes / MB).toString(), unit: 'MB' };
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function toggleSet<T>(set: Set<T>, value: T): Set<T> {
  const next = new Set(set);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}

const CAPABILITY_KEYS: (keyof CatalogCapabilities)[] = [
  'images', 'audio', 'video', 'streaming', 'reasoning', 'tooling', 'embedding', 'rerank',
];

const CAPABILITY_LABELS: Record<string, string> = {
  images: 'Images',
  audio: 'Audio',
  video: 'Video',
  streaming: 'Streaming',
  reasoning: 'Reasoning',
  tooling: 'Tooling',
  embedding: 'Embedding',
  rerank: 'Rerank',
};

// ---------------------------------------------------------------------------
// FilterSection component
// ---------------------------------------------------------------------------

function FilterSection({
  title, expanded, onToggle, children,
}: {
  title: string;
  expanded: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  return (
    <div className="catalog-filter-section">
      <div className="catalog-filter-section-header" onClick={onToggle}>
        <h4>{title}</h4>
        <span className={`chevron ${expanded ? 'expanded' : ''}`}>▶</span>
      </div>
      {expanded && children}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export default function CatalogList() {
  const { download, startCatalogDownload, cancelDownload } = useDownload();
  const [data, setData] = useState<CatalogModelsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [modelInfo, setModelInfo] = useState<CatalogModelResponse | null>(null);
  const [infoLoading, setInfoLoading] = useState(false);
  const [infoError, setInfoError] = useState<string | null>(null);

  const [activeSection, setActiveSection] = useState<DetailSection>('model-card');

  // Sort state for catalog table
  type SortField = 'id' | 'owned_by' | 'model_family' | 'model_type' | 'has_projection' | 'total_size_bytes' | 'validated';
  const [sortField, setSortField] = useState<SortField>('id');
  const [sortAsc, setSortAsc] = useState(true);

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortAsc(!sortAsc);
    } else {
      setSortField(field);
      setSortAsc(true);
    }
  };

  // Remove-from-catalog state
  const [confirmingRemove, setConfirmingRemove] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [removeError, setRemoveError] = useState<string | null>(null);
  const [removeSuccess, setRemoveSuccess] = useState<string | null>(null);
  const removeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (removeTimerRef.current) clearTimeout(removeTimerRef.current);
    };
  }, []);

  // VRAM calculator state (shared hook, wired after effectiveVram is computed below)
  const [showLearnMore, setShowLearnMore] = useState(false);

  // Remote VRAM data for non-downloaded models
  const [remoteVram, setRemoteVram] = useState<Record<string, VRAMCalculatorResponse>>({});
  const [remoteVramLoading, setRemoteVramLoading] = useState<string | null>(null);
  const [remoteVramError, setRemoteVramError] = useState<string | null>(null);

  const [downloadServer, setDownloadServer] = useState<string>(() => {
    return localStorage.getItem('kronk_download_server') || '';
  });

  const handleDownloadServerChange = (value: string) => {
    setDownloadServer(value);
    if (value) {
      localStorage.setItem('kronk_download_server', value);
    } else {
      localStorage.removeItem('kronk_download_server');
    }
  };

  // ── Filter state ──────────────────────────────────────────────────────────

  const [searchText, setSearchText] = useState('');
  const [selectedOwners, setSelectedOwners] = useState<Set<string>>(new Set());
  const [selectedFamilies, setSelectedFamilies] = useState<Set<string>>(new Set());
  const [selectedModelTypes, setSelectedModelTypes] = useState<Set<string>>(new Set());
  const [selectedCapabilities, setSelectedCapabilities] = useState<Set<string>>(new Set());

  const [sizeMinVal, setSizeMinVal] = useState('0');
  const [sizeMinUnit, setSizeMinUnit] = useState<SizeUnit>('MB');
  const [sizeMaxVal, setSizeMaxVal] = useState('4');
  const [sizeMaxUnit, setSizeMaxUnit] = useState<SizeUnit>('TB');

  const [downloadedFilter, setDownloadedFilter] = useState<'all' | 'yes' | 'no'>('all');
  const [validatedFilter, setValidatedFilter] = useState<'all' | 'yes' | 'no'>('all');

  const [expandedSections, setExpandedSections] = useState<Set<string>>(
    new Set(['architecture', 'capabilities', 'size']),
  );

  const toggleSection = (s: string) => setExpandedSections(prev => toggleSet(prev, s));

  // Compute bytes from text inputs (blank/invalid max → no upper bound)
  const parsedMin = parseFloat(sizeMinVal);
  const parsedMax = parseFloat(sizeMaxVal);
  const rawMinBytes = Number.isFinite(parsedMin) ? parsedMin * UNIT_MULT[sizeMinUnit] : 0;
  const rawMaxBytes = Number.isFinite(parsedMax) ? parsedMax * UNIT_MULT[sizeMaxUnit] : SIZE_MAX_BYTES;
  const sizeMinBytes = Math.min(rawMinBytes, rawMaxBytes);
  const sizeMaxBytes = Math.max(rawMinBytes, rawMaxBytes);
  const sliderMin = bytesToSlider(sizeMinBytes);
  const sliderMax = bytesToSlider(sizeMaxBytes);

  const handleSliderMin = (pos: number) => {
    const clamped = Math.min(pos, bytesToSlider(sizeMaxBytes));
    const bytes = sliderToBytes(clamped);
    const display = bytesToBestUnit(bytes);
    setSizeMinVal(display.value);
    setSizeMinUnit(display.unit);
  };

  const handleSliderMax = (pos: number) => {
    const clamped = Math.max(pos, bytesToSlider(sizeMinBytes));
    const bytes = sliderToBytes(clamped);
    const display = bytesToBestUnit(bytes);
    setSizeMaxVal(display.value);
    setSizeMaxUnit(display.unit);
  };

  // ── Distinct filter values ────────────────────────────────────────────────

  const distinctValues = useMemo(() => {
    if (!data) return { owners: [] as string[], families: [] as string[] };

    const isStr = (s: string | undefined): s is string => Boolean(s);

    return {
      owners: [...new Set(data.map(m => m.owned_by).filter(isStr))].sort(),
      families: [...new Set(data.map(m => m.model_family).filter(isStr))].sort(),
    };
  }, [data]);

  // ── Filtered data ─────────────────────────────────────────────────────────

  const filteredData = useMemo(() => {
    if (!data) return [];
    return data.filter(model => {
      if (searchText && !model.id.toLowerCase().includes(searchText.toLowerCase())) return false;
      if (selectedOwners.size > 0 && !selectedOwners.has(model.owned_by)) return false;
      if (selectedFamilies.size > 0 && !selectedFamilies.has(model.model_family)) return false;
      if (selectedModelTypes.size > 0 && (!model.model_type || !selectedModelTypes.has(model.model_type))) return false;
      if (model.total_size_bytes < sizeMinBytes || model.total_size_bytes > sizeMaxBytes) return false;
      if (downloadedFilter === 'yes' && !model.downloaded) return false;
      if (downloadedFilter === 'no' && model.downloaded) return false;
      if (validatedFilter === 'yes' && !model.validated) return false;
      if (validatedFilter === 'no' && model.validated) return false;
      for (const cap of selectedCapabilities) {
        const caps = model.capabilities as unknown as Record<string, boolean | string> | undefined;
        if (!caps || !caps[cap]) return false;
      }
      return true;
    });
  }, [data, searchText, selectedOwners, selectedFamilies, selectedModelTypes, sizeMinBytes, sizeMaxBytes, downloadedFilter, validatedFilter, selectedCapabilities]);

  const sortedData = useMemo(() => {
    const sorted = [...filteredData];
    sorted.sort((a, b) => {
      let cmp = 0;
      switch (sortField) {
        case 'id': cmp = a.id.localeCompare(b.id); break;
        case 'owned_by': cmp = (a.owned_by || '').localeCompare(b.owned_by || ''); break;
        case 'model_family': cmp = (a.model_family || '').localeCompare(b.model_family || ''); break;
        case 'model_type': cmp = (a.model_type || '').localeCompare(b.model_type || ''); break;
        case 'has_projection': cmp = (a.has_projection ? 1 : 0) - (b.has_projection ? 1 : 0); break;
        case 'total_size_bytes': cmp = (a.total_size_bytes || 0) - (b.total_size_bytes || 0); break;
        case 'validated': cmp = (a.validated ? 1 : 0) - (b.validated ? 1 : 0); break;
      }
      return sortAsc ? cmp : -cmp;
    });
    return sorted;
  }, [filteredData, sortField, sortAsc]);

  const hasActiveFilters = searchText !== '' ||
    selectedOwners.size > 0 || selectedFamilies.size > 0 ||
    selectedModelTypes.size > 0 || selectedCapabilities.size > 0 ||
    sizeMinBytes > 0 || sizeMaxBytes < SIZE_MAX_BYTES ||
    downloadedFilter !== 'all' || validatedFilter !== 'all';

  // Clear detail panel when filters change and selected model is no longer visible
  useEffect(() => {
    if (selectedId && !filteredData.some(m => m.id === selectedId)) {
      setSelectedId(null);
      setModelInfo(null);
      setInfoError(null);
      setActiveSection('model-card');
    }
  }, [filteredData, selectedId]);

  const clearAllFilters = () => {
    setSearchText('');
    setSelectedOwners(new Set());
    setSelectedFamilies(new Set());
    setSelectedModelTypes(new Set());
    setSelectedCapabilities(new Set());
    setSizeMinVal('0');
    setSizeMinUnit('MB');
    setSizeMaxVal('4');
    setSizeMaxUnit('TB');
    setDownloadedFilter('all');
    setValidatedFilter('all');
  };

  // ── Download state ────────────────────────────────────────────────────────

  const isCatalogDownload = download?.kind === 'catalog' && download.catalogId === selectedId;
  const pulling = isCatalogDownload ? download.status === 'downloading' : false;
  const pullMessages = isCatalogDownload ? download.messages : [];
  const hasCatalogPullActivity = download?.kind === 'catalog' && download.catalogId === selectedId && (download.status === 'downloading' || download.messages.length > 0);

  // ── Data loading ──────────────────────────────────────────────────────────

  useEffect(() => {
    loadCatalog();
  }, []);

  useEffect(() => {
    if (download?.kind === 'catalog' && download.catalogId && download.status === 'downloading') {
      setSelectedId(download.catalogId);
      setActiveSection('pull');
    }
  }, [download?.status]);

  // Refresh catalog list and re-select the model after a pull completes.
  useEffect(() => {
    if (download?.kind === 'catalog' && download.status === 'complete' && download.catalogId) {
      const completedId = download.catalogId;
      (async () => {
        try {
          const response = await api.listCatalog();
          setData(response);
          setSelectedId(completedId);

          const info = await api.showCatalogModel(completedId);
          setModelInfo(info);
        } catch {
          // Ignore refresh errors; user can manually refresh.
        }
      })();
    }
  }, [download?.status]);

  const loadCatalog = async () => {
    setLoading(true);
    setError(null);
    setSelectedId(null);
    setModelInfo(null);
    setInfoError(null);
    setActiveSection('model-card');
    try {
      // Reconcile first so any new on-disk models or schema-version
      // upgrades are picked up before listing. Best-effort: when
      // reconcile fails (e.g. no admin auth) we still try to list.
      try {
        await api.reconcileCatalog();
      } catch {
        // ignore — listing still works on stale data.
      }

      const response = await api.listCatalog();
      setData(response);

      // Restore selection if a catalog download is active or has results.
      if (download?.kind === 'catalog' && download.catalogId) {
        setSelectedId(download.catalogId);
        setActiveSection('pull');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load catalog');
    } finally {
      setLoading(false);
    }
  };

  const handleRowClick = async (id: string) => {
    setConfirmingRemove(false);
    setRemoveError(null);

    if (selectedId === id) {
      setSelectedId(null);
      setModelInfo(null);
      setActiveSection('model-card');
      return;
    }

    setSelectedId(id);
    setActiveSection('model-card');
    setInfoLoading(true);
    setInfoError(null);
    setModelInfo(null);

    try {
      const response = await api.showCatalogModel(id);
      setModelInfo(response);
    } catch (err) {
      setInfoError(err instanceof Error ? err.message : 'Failed to load model info');
    } finally {
      setInfoLoading(false);
    }
  };

  // Fetch remote VRAM data for non-downloaded models
  const fetchRemoteVram = useCallback(async (id: string, modelUrl: string, signal?: { cancelled: boolean }) => {
    setRemoteVramLoading(id);
    setRemoteVramError(null);
    try {
      const resp = await api.calculateVRAM({
        model_url: modelUrl,
        context_window: 8192,
        bytes_per_element: 1,
        slots: 2,
      });
      if (signal?.cancelled) return;
      setRemoteVram(prev => ({ ...prev, [id]: resp }));
    } catch (err) {
      if (signal?.cancelled) return;
      setRemoteVramError(err instanceof Error ? err.message : 'Failed to calculate VRAM');
    } finally {
      if (!signal?.cancelled) {
        setRemoteVramLoading(curr => curr === id ? null : curr);
      }
    }
  }, []);

  // Auto-fetch when VRAM tab is opened for a non-downloaded model
  useEffect(() => {
    if (activeSection !== 'vram' || !modelInfo) return;
    if (modelInfo.vram) return;
    if (remoteVram[modelInfo.id]) return;

    const modelUrl = modelInfo.files?.model?.[0]?.url;
    if (!modelUrl) return;

    const signal = { cancelled: false };
    fetchRemoteVram(modelInfo.id, modelUrl, signal);
    return () => { signal.cancelled = true; };
  }, [activeSection, modelInfo?.id, fetchRemoteVram]);

  // Clear remote error when switching models
  useEffect(() => {
    setRemoteVramError(null);
  }, [modelInfo?.id]);

  // VRAM calculator: derive effective response and wire hook
  const effectiveVram = modelInfo?.vram ?? (modelInfo ? remoteVram[modelInfo.id] : undefined);
  const catalogModelUrl = modelInfo?.files?.model?.[0]?.url;
  const { controlsProps: vramControls, resultsProps: vramResults } = useVRAMState({
    initialContextWindow: 8192,
    initialBytesPerElement: 1,
    serverResponse: effectiveVram ?? null,
    modelId: modelInfo?.vram ? modelInfo.id : undefined,
    modelUrl: !modelInfo?.vram ? catalogModelUrl : undefined,
  });
  const contextInfo = extractContextInfo(modelInfo?.model_metadata);
  const handlePull = () => {
    if (!selectedId) return;
    startCatalogDownload(selectedId, downloadServer || undefined);
    setActiveSection('pull');
  };

  const handleCancelPull = () => {
    cancelDownload();
  };

  const handleRemoveClick = () => {
    if (!selectedId) return;
    setConfirmingRemove(true);
  };

  const handleConfirmRemove = async () => {
    if (!selectedId) return;

    const removedId = selectedId;
    setRemoving(true);
    setConfirmingRemove(false);
    setRemoveError(null);
    setRemoveSuccess(null);

    try {
      await api.removeCatalogModel(removedId);
      setRemoveSuccess(`Catalog entry "${removedId}" removed successfully`);
      setSelectedId(null);
      setModelInfo(null);
      setActiveSection('model-card');
      await loadCatalog();
      removeTimerRef.current = setTimeout(() => setRemoveSuccess(null), 3000);
    } catch (err) {
      setRemoveError(err instanceof Error ? err.message : 'Failed to remove catalog entry');
    } finally {
      setRemoving(false);
    }
  };

  const handleCancelRemove = () => {
    setConfirmingRemove(false);
  };

  const isDownloaded = data?.find((m) => m.id === selectedId)?.downloaded ?? false;

  // ── Render ────────────────────────────────────────────────────────────────

  return (
    <div>
      <div className="page-header-with-action">
        <div>
          <h2>Catalog</h2>
          <p className="page-description">Browse available models in the catalog. Click a model to view details.</p>
        </div>
        <div style={{ display: 'flex', gap: '8px' }}>
          <button
            className="btn btn-primary"
            onClick={() => {
              loadCatalog();
              setSelectedId(null);
              setModelInfo(null);
              setActiveSection('model-card');
              setInfoError(null);
              setConfirmingRemove(false);
              setRemoveError(null);
              setRemoveSuccess(null);
            }}
            disabled={loading}
          >
            Refresh
          </button>
          {selectedId && !confirmingRemove && (
            <button
              className="btn btn-primary"
              onClick={handleRemoveClick}
              disabled={removing}
            >
              Remove Model
            </button>
          )}
          {selectedId && confirmingRemove && (
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

      <div className="playground-layout">
        {/* ── Filter Sidebar ────────────────────────────────────────────── */}
        <ResizablePanel defaultWidth={340} minWidth={240} maxWidth={600} storageKey="catalog-sidebar-width" className="catalog-filter-sidebar-panel">
          {/* Search */}
          <div className="catalog-filter-section">
            <input
              type="text"
              placeholder="Search models..."
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              style={{
                width: '100%', padding: '8px 10px',
                border: '1px solid var(--color-gray-300)', borderRadius: '4px',
                fontSize: '14px', background: 'var(--color-white)',
              }}
            />
          </div>

          {/* Provider */}
          {distinctValues.owners.length > 0 && (
            <FilterSection title="Provider" expanded={expandedSections.has('owner')} onToggle={() => toggleSection('owner')}>
              <div className="catalog-filter-options">
                {distinctValues.owners.map(owner => (
                  <label key={owner}>
                    <input
                      type="checkbox"
                      checked={selectedOwners.has(owner)}
                      onChange={() => setSelectedOwners(prev => toggleSet(prev, owner))}
                    />
                    {owner}
                  </label>
                ))}
              </div>
            </FilterSection>
          )}

          {/* Model Family */}
          {distinctValues.families.length > 0 && (
            <FilterSection title="Family" expanded={expandedSections.has('family')} onToggle={() => toggleSection('family')}>
              <div className="catalog-filter-options">
                {distinctValues.families.map(fam => (
                  <label key={fam}>
                    <input
                      type="checkbox"
                      checked={selectedFamilies.has(fam)}
                      onChange={() => setSelectedFamilies(prev => toggleSet(prev, fam))}
                    />
                    {fam}
                  </label>
                ))}
              </div>
            </FilterSection>
          )}

          {/* Architecture (Dense/MoE/Hybrid) */}
          <FilterSection title="Architecture" expanded={expandedSections.has('architecture')} onToggle={() => toggleSection('architecture')}>
            <div className="catalog-filter-options">
              {(['Dense', 'MoE', 'Hybrid'] as const).map(t => (
                <label key={t}>
                  <input
                    type="checkbox"
                    checked={selectedModelTypes.has(t)}
                    onChange={() => setSelectedModelTypes(prev => toggleSet(prev, t))}
                  />
                  {t}
                </label>
              ))}
            </div>
          </FilterSection>

          {/* Capabilities */}
          <FilterSection title="Capabilities" expanded={expandedSections.has('capabilities')} onToggle={() => toggleSection('capabilities')}>
            <div className="catalog-filter-options">
              {CAPABILITY_KEYS.map(cap => (
                <label key={cap}>
                  <input
                    type="checkbox"
                    checked={selectedCapabilities.has(cap)}
                    onChange={() => setSelectedCapabilities(prev => toggleSet(prev, cap))}
                  />
                  {CAPABILITY_LABELS[cap]}
                </label>
              ))}
            </div>
          </FilterSection>

          {/* Size */}
          <FilterSection title="Size" expanded={expandedSections.has('size')} onToggle={() => toggleSection('size')}>
            <div className="catalog-range-row">
              <label>Min</label>
              <input
                type="number"
                min="0"
                step="any"
                value={sizeMinVal}
                onChange={(e) => setSizeMinVal(e.target.value)}
              />
              <select value={sizeMinUnit} onChange={(e) => setSizeMinUnit(e.target.value as SizeUnit)}>
                <option value="MB">MB</option>
                <option value="GB">GB</option>
                <option value="TB">TB</option>
              </select>
            </div>
            <div className="catalog-range-row">
              <label>Max</label>
              <input
                type="number"
                min="0"
                step="any"
                value={sizeMaxVal}
                onChange={(e) => setSizeMaxVal(e.target.value)}
              />
              <select value={sizeMaxUnit} onChange={(e) => setSizeMaxUnit(e.target.value as SizeUnit)}>
                <option value="MB">MB</option>
                <option value="GB">GB</option>
                <option value="TB">TB</option>
              </select>
            </div>
            <div className="catalog-dual-range">
              <div className="catalog-dual-range-track">
                <div
                  className="catalog-dual-range-fill"
                  style={{
                    left: `${(sliderMin / SLIDER_STEPS) * 100}%`,
                    right: `${100 - (sliderMax / SLIDER_STEPS) * 100}%`,
                  }}
                />
              </div>
              <input
                type="range"
                min={0}
                max={SLIDER_STEPS}
                value={sliderMin}
                onChange={(e) => handleSliderMin(Number(e.target.value))}
              />
              <input
                type="range"
                min={0}
                max={SLIDER_STEPS}
                value={sliderMax}
                onChange={(e) => handleSliderMax(Number(e.target.value))}
              />
            </div>
          </FilterSection>

          {/* Downloaded */}
          <FilterSection title="Downloaded" expanded={expandedSections.has('downloaded')} onToggle={() => toggleSection('downloaded')}>
            <div className="catalog-filter-radio">
              {(['all', 'yes', 'no'] as const).map(v => (
                <button
                  key={v}
                  className={downloadedFilter === v ? 'active' : ''}
                  onClick={() => setDownloadedFilter(v)}
                >
                  {v === 'all' ? 'All' : v === 'yes' ? 'Yes' : 'No'}
                </button>
              ))}
            </div>
          </FilterSection>

          {/* Validated */}
          <FilterSection title="Validated" expanded={expandedSections.has('validated')} onToggle={() => toggleSection('validated')}>
            <div className="catalog-filter-radio">
              {(['all', 'yes', 'no'] as const).map(v => (
                <button
                  key={v}
                  className={validatedFilter === v ? 'active' : ''}
                  onClick={() => setValidatedFilter(v)}
                >
                  {v === 'all' ? 'All' : v === 'yes' ? 'Yes' : 'No'}
                </button>
              ))}
            </div>
          </FilterSection>

          {/* Footer: match count + clear + actions */}
          <div className="catalog-filter-footer">
            <div className="catalog-match-count">
              {data ? `${filteredData.length} / ${data.length} models` : ''}
            </div>
            {hasActiveFilters && (
              <button className="btn btn-secondary" onClick={clearAllFilters} style={{ fontSize: '12px', padding: '6px 10px' }}>
                Clear Filters
              </button>
            )}
          </div>

        </ResizablePanel>

        {/* ── Main Content ──────────────────────────────────────────────── */}
        <div className="catalog-main-content">
            {loading && <div className="loading">Loading catalog</div>}

            {error && <div className="alert alert-error">{error}</div>}

            {!loading && !error && data && (
              <div className="catalog-table-wrap">
                <table className="catalog-table">
                  <thead>
                    <tr>
                      <th style={{ width: '40px', textAlign: 'center' }} onClick={() => handleSort('validated')} className="catalog-table-sortable" title="Configuration and template confirmed working with the Kronk catalog">
                        VAL
                        <span className="catalog-table-sort-indicator">
                          {sortField === 'validated' ? (sortAsc ? ' ▲' : ' ▼') : ''}
                        </span>
                      </th>
                      {([
                        ['id', 'Model ID'],
                        ['owned_by', 'Provider'],
                        ['model_family', 'Family'],
                      ] as const).map(([field, label]) => (
                        <th key={field} onClick={() => handleSort(field)} className="catalog-table-sortable">
                          {label}
                          <span className="catalog-table-sort-indicator">
                            {sortField === field ? (sortAsc ? ' ▲' : ' ▼') : ''}
                          </span>
                        </th>
                      ))}
                      <th onClick={() => handleSort('model_type')} className="catalog-table-sortable" title="Architecture class (Dense, MoE, or Hybrid) derived from GGUF metadata">
                        Arch
                        <span className="catalog-table-sort-indicator">
                          {sortField === 'model_type' ? (sortAsc ? ' ▲' : ' ▼') : ''}
                        </span>
                      </th>
                      <th style={{ textAlign: 'center' }} onClick={() => handleSort('has_projection')} className="catalog-table-sortable" title="Multimodal projection file present">
                        MTMD
                        <span className="catalog-table-sort-indicator">
                          {sortField === 'has_projection' ? (sortAsc ? ' ▲' : ' ▼') : ''}
                        </span>
                      </th>
                      <th onClick={() => handleSort('total_size_bytes')} className="catalog-table-sortable">
                        Size
                        <span className="catalog-table-sort-indicator">
                          {sortField === 'total_size_bytes' ? (sortAsc ? ' ▲' : ' ▼') : ''}
                        </span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {sortedData.map((model) => (
                      <tr
                        key={model.id}
                        className={selectedId === model.id ? 'active' : ''}
                        onClick={() => handleRowClick(model.id)}
                      >
                        <td style={{ textAlign: 'center', color: model.validated ? 'inherit' : 'var(--color-error)' }}>{model.validated ? '✓' : '✗'}</td>
                        <td><span className="catalog-table-cell-ellipsis">{model.id}</span></td>
                        <td>{model.owned_by || '-'}</td>
                        <td>{model.model_family || '-'}</td>
                        <td>{model.model_type || '-'}</td>
                        <td style={{ textAlign: 'center' }}>{model.has_projection ? '✓' : ''}</td>
                        <td>{model.total_size || '-'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {filteredData.length === 0 && (
                  <div className="empty-state">
                    <h3>{hasActiveFilters ? 'No matching models' : 'No catalog entries'}</h3>
                    <p>{hasActiveFilters ? 'Try adjusting your filters' : 'The catalog is empty'}</p>
                  </div>
                )}
              </div>
            )}

            {removeError && <div className="alert alert-error" style={{ marginTop: '16px' }}>{removeError}</div>}
            {removeSuccess && <div className="alert alert-success" style={{ marginTop: '16px' }}>{removeSuccess}</div>}

            {infoError && <div className="alert alert-error" style={{ marginTop: '16px' }}>{infoError}</div>}

            {infoLoading && (
              <div style={{ marginTop: '16px' }}>
                <div className="loading">Loading model details</div>
              </div>
            )}

            {selectedId && !infoLoading && (modelInfo || hasCatalogPullActivity) && (
              <div style={{ marginTop: '16px', borderTop: '1px solid var(--color-gray-200)', paddingTop: '16px' }}>
                <div className="tabs">
                  {(Object.keys(SECTION_LABELS) as DetailSection[]).map(section => (
                    <button
                      key={section}
                      className={`tab ${activeSection === section ? 'active' : ''}`}
                      onClick={() => setActiveSection(section)}
                      disabled={false}
                    >
                      {SECTION_LABELS[section]}
                    </button>
                  ))}
                </div>

                {/* Catalog Section */}
                {activeSection === 'catalog' && modelInfo && (
                  <div>
                    <h3 style={{ marginBottom: '16px' }}>{modelInfo.id}</h3>

                    {modelInfo.metadata?.description && (
                      <div style={{ marginBottom: '16px' }}>
                        <p>{modelInfo.metadata.description}</p>
                      </div>
                    )}

                    {modelInfo.web_page && (
                      <div style={{ marginBottom: '16px' }}>
                        <h4 className="meta-section-title" style={{ marginBottom: '8px' }}>Web Page</h4>
                        <p>
                          <a href={modelInfo.web_page} target="_blank" rel="noopener noreferrer">
                            {modelInfo.web_page}
                          </a>
                        </p>
                      </div>
                    )}

                    <div style={{ marginBottom: '16px' }}>
                      <h4 className="meta-section-title" style={{ marginBottom: '8px' }}>Files</h4>
                      <KeyValueTable rows={[
                        { key: 'model-url', label: 'Model URL', value: (modelInfo.files?.model?.length ?? 0) > 0 ? modelInfo.files!.model.map((file, idx) => <div key={idx}>{file.url} {file.size ? `(${formatBytes(file.size)})` : ''}</div>) : '-' },
                        { key: 'proj-url', label: 'Projection URL', value: modelInfo.files?.proj?.url ? <div>{modelInfo.files.proj.url} {modelInfo.files.proj.size ? `(${formatBytes(modelInfo.files.proj.size)})` : ''}</div> : '-' },
                      ]} />
                    </div>

                    <KeyValueTable rows={[
                      { key: 'owner', label: 'Provider', value: modelInfo.owned_by },
                      { key: 'family', label: 'Family', value: modelInfo.model_family },
                      { key: 'arch', label: 'Architecture', value: modelInfo.model_type || '-' },
                      { key: 'gguf-arch', label: 'GGUF Arch', value: modelInfo.gguf_arch || '-' },
                      { key: 'params', label: 'Parameters', value: modelInfo.parameters || '-' },
                      { key: 'size', label: 'Size', value: modelInfo.total_size || '-' },
                      { key: 'downloaded', label: 'Downloaded', value: <span className={`badge ${modelInfo.downloaded ? 'badge-yes' : 'badge-no'}`}>{modelInfo.downloaded ? 'Yes' : 'No'}</span> },
                      { key: 'validated', label: labelWithTip('Validated', 'validated'), value: <span style={{ color: modelInfo.validated ? 'inherit' : 'var(--color-error)' }}>{modelInfo.validated ? '✓' : '✗'}</span> },
                      { key: 'endpoint', label: 'Endpoint', value: modelInfo.capabilities?.endpoint || '-' },
                      { key: 'template', label: 'Template', value: modelInfo.template || '-' },
                    ]} />

                    <div style={{ marginTop: '24px' }}>
                      <h4 className="meta-section-title" style={{ marginBottom: '8px' }}>Capabilities</h4>
                      <KeyValueTable rows={CAPABILITY_KEYS.map(cap => {
                        const on = Boolean((modelInfo.capabilities as unknown as Record<string, boolean> | undefined)?.[cap]);
                        return {
                          key: cap,
                          label: CAPABILITY_LABELS[cap],
                          value: <span className={`badge ${on ? 'badge-yes' : 'badge-no'}`}>{on ? 'Yes' : 'No'}</span>,
                        };
                      })} />
                    </div>

                  </div>
                )}

                {/* Model Card Section */}
                {activeSection === 'model-card' && modelInfo && (
                  <div>
                    <h3 style={{ marginBottom: '16px' }}>Model Card</h3>
                    <ModelCard metadata={modelInfo.model_metadata ?? {}} webPage={modelInfo.web_page} />
                  </div>
                )}

                {/* Template Section */}
                {activeSection === 'template' && modelInfo && (
                  <div>
                    <h3 style={{ marginBottom: '16px' }}>Chat Template</h3>
                    {modelInfo.model_metadata?.['tokenizer.chat_template'] ? (
                      <CodeBlock
                        code={modelInfo.model_metadata['tokenizer.chat_template']}
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
                {activeSection === 'vram' && modelInfo && (
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

                    {remoteVramLoading === modelInfo.id ? (
                      <div className="vram-loading-banner">
                        <span className="vram-loading-spinner" />
                        <span>Fetching model header (up to 16 MB)…</span>
                      </div>
                    ) : remoteVramError && !vramResults ? (
                      <div className="empty-state">
                        <p>{remoteVramError}</p>
                        <button
                          type="button"
                          className="btn btn-secondary"
                          style={{ marginTop: '8px' }}
                          onClick={() => {
                            const modelUrl = modelInfo.files?.model?.[0]?.url;
                            if (modelUrl) fetchRemoteVram(modelInfo.id, modelUrl);
                          }}
                        >
                          Retry
                        </button>
                      </div>
                    ) : vramResults ? (
                      <>
                        <p style={{ fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: '16px' }}>
                          {modelInfo.vram
                            ? 'Computed locally from GGUF header. Adjust parameters below to see how they affect VRAM.'
                            : 'Computed from remote GGUF header. Adjust parameters below to see how they affect VRAM.'}
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

                {/* Pull Section */}
                {activeSection === 'pull' && (
                  <div>
                    <h3 style={{ marginBottom: '16px' }}>Pull: {selectedId}</h3>

                    <div style={{ display: 'flex', gap: '8px', alignItems: 'flex-end', flexWrap: 'wrap', marginBottom: '16px' }}>
                      <div className="form-group" style={{ marginBottom: 0 }}>
                        <label htmlFor="download-server" style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '12px', fontWeight: 600, color: 'var(--color-gray-600)', marginBottom: '4px' }}>
                          Download Server
                          <ParamTooltip text="Optional: specify a Kronk server on your local network that already has the model files. The pull will download from that server instead of HuggingFace, which is much faster." />
                        </label>
                        <input
                          id="download-server"
                          type="text"
                          className="form-control"
                          placeholder="192.168.0.246:11435"
                          value={downloadServer}
                          onChange={(e) => handleDownloadServerChange(e.target.value)}
                          disabled={pulling}
                          style={{ width: '220px', padding: '6px 8px', border: '1px solid var(--color-gray-300)', borderRadius: '4px', fontSize: '13px' }}
                        />
                      </div>
                      <button
                        className="btn btn-primary"
                        onClick={handlePull}
                        disabled={pulling || isDownloaded}
                      >
                        {pulling ? 'Pulling...' : isDownloaded ? 'Already Downloaded' : 'Pull Model'}
                      </button>
                      {pulling && (
                        <button className="btn btn-danger" onClick={handleCancelPull}>
                          Cancel
                        </button>
                      )}
                    </div>

                    {isCatalogDownload && download.meta && (
                      <DownloadInfoTable meta={download.meta} />
                    )}

                    {isCatalogDownload && download.progress && pulling && (
                      <DownloadProgressBar progress={download.progress} meta={download.meta} />
                    )}

                    {pullMessages.length > 0 && (
                      <div className="status-box">
                        {pullMessages.map((msg, idx) => (
                          <div key={idx} className={`status-line ${msg.type}`}>
                            {msg.text}
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
            )}
        </div>
      </div>
    </div>
  );
}
