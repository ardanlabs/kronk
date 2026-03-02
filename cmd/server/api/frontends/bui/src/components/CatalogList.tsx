import { useState, useEffect, useMemo, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../services/api';
import { useDownload } from '../contexts/DownloadContext';
import type { CatalogModelResponse, CatalogModelsResponse, CatalogCapabilities } from '../types';
import { ParamTooltip } from './ParamTooltips';
import { fmtNum, fmtVal } from '../lib/format';
import ModelSelector from './ModelSelector';
import KeyValueTable from './KeyValueTable';
import MetadataSection from './MetadataSection';
import CodeBlock from './CodeBlock';
import { VRAMFormulaModal, VRAMControls, VRAMResults, calculateVRAM } from './vram';

type DetailSection = 'catalog' | 'config' | 'sampling' | 'metadata' | 'template' | 'vram' | 'pull';

const SECTION_LABELS: Record<DetailSection, string> = {
  catalog: 'Catalog',
  config: 'Configuration',
  sampling: 'Sampling',
  metadata: 'Metadata',
  template: 'Template',
  vram: 'VRAM Calculator',
  pull: 'Pull Output',
};


// ---------------------------------------------------------------------------
// Size slider helpers (logarithmic scale)
// ---------------------------------------------------------------------------

const MB = 1024 * 1024;
const GB = 1024 * 1024 * 1024;
const TB = 1024 * 1024 * 1024 * 1024;
const SIZE_MAX_BYTES = 4 * TB; // 4 TiB
const SLIDER_STEPS = 1000;

// Parameter count range (100M to 1T parameters)
const PARAM_MIN = 1e8;   // 100M
const PARAM_MAX = 1e12;  // 1T

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

function sliderToParams(pos: number): number {
  if (pos <= 0) return 0;
  if (pos >= SLIDER_STEPS) return PARAM_MAX;
  const logMin = Math.log(PARAM_MIN);
  const logMax = Math.log(PARAM_MAX);
  return Math.exp(logMin + (pos / SLIDER_STEPS) * (logMax - logMin));
}

function paramsToSlider(count: number): number {
  if (count <= 0) return 0;
  if (count >= PARAM_MAX) return SLIDER_STEPS;
  const logMin = Math.log(PARAM_MIN);
  const logMax = Math.log(PARAM_MAX);
  return Math.round(((Math.log(Math.max(count, PARAM_MIN)) - logMin) / (logMax - logMin)) * SLIDER_STEPS);
}

type ParamUnit = 'M' | 'B';

const PARAM_UNIT_MULT: Record<ParamUnit, number> = { M: 1e6, B: 1e9 };

function paramsToBestUnit(count: number): { value: string; unit: ParamUnit } {
  if (count >= 1e9) return { value: (count / 1e9).toFixed(1), unit: 'B' };
  return { value: Math.round(count / 1e6).toString(), unit: 'M' };
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
  const navigate = useNavigate();
  const { download, startCatalogDownload, cancelDownload } = useDownload();
  const [data, setData] = useState<CatalogModelsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [modelInfo, setModelInfo] = useState<CatalogModelResponse | null>(null);
  const [infoLoading, setInfoLoading] = useState(false);
  const [infoError, setInfoError] = useState<string | null>(null);

  const [activeSection, setActiveSection] = useState<DetailSection>('catalog');

  // VRAM calculator state
  const [vramCtx, setVramCtx] = useState(8192);
  const [vramBytes, setVramBytes] = useState(1);
  const [vramSlots, setVramSlots] = useState(2);
  const [showLearnMore, setShowLearnMore] = useState(false);

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
  const [selectedCategories, setSelectedCategories] = useState<Set<string>>(new Set());
  const [selectedOwners, setSelectedOwners] = useState<Set<string>>(new Set());
  const [selectedArchitectures, setSelectedArchitectures] = useState<Set<string>>(new Set());
  const [selectedFamilies, setSelectedFamilies] = useState<Set<string>>(new Set());

  const [sizeMinVal, setSizeMinVal] = useState('0');
  const [sizeMinUnit, setSizeMinUnit] = useState<SizeUnit>('MB');
  const [sizeMaxVal, setSizeMaxVal] = useState('4');
  const [sizeMaxUnit, setSizeMaxUnit] = useState<SizeUnit>('TB');

  const [paramMinVal, setParamMinVal] = useState('0');
  const [paramMinUnit, setParamMinUnit] = useState<ParamUnit>('M');
  const [paramMaxVal, setParamMaxVal] = useState('1000');
  const [paramMaxUnit, setParamMaxUnit] = useState<ParamUnit>('B');

  const [downloadedFilter, setDownloadedFilter] = useState<'all' | 'yes' | 'no'>('all');
  const [validatedFilter, setValidatedFilter] = useState<'all' | 'yes' | 'no'>('all');
  const [selectedCapabilities, setSelectedCapabilities] = useState<Set<string>>(new Set());

  const [expandedSections, setExpandedSections] = useState<Set<string>>(
    new Set(['category', 'size', 'capabilities']),
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

  // Compute param count from text inputs
  const parsedParamMin = parseFloat(paramMinVal);
  const parsedParamMax = parseFloat(paramMaxVal);
  const rawParamMin = Number.isFinite(parsedParamMin) ? parsedParamMin * PARAM_UNIT_MULT[paramMinUnit] : 0;
  const rawParamMax = Number.isFinite(parsedParamMax) ? parsedParamMax * PARAM_UNIT_MULT[paramMaxUnit] : PARAM_MAX;
  const paramMin = Math.min(rawParamMin, rawParamMax);
  const paramMax = Math.max(rawParamMin, rawParamMax);
  const paramSliderMin = paramsToSlider(paramMin);
  const paramSliderMax = paramsToSlider(paramMax);

  const handleParamSliderMin = (pos: number) => {
    const clamped = Math.min(pos, paramsToSlider(paramMax));
    const count = sliderToParams(clamped);
    const display = paramsToBestUnit(count);
    setParamMinVal(display.value);
    setParamMinUnit(display.unit);
  };

  const handleParamSliderMax = (pos: number) => {
    const clamped = Math.max(pos, paramsToSlider(paramMin));
    const count = sliderToParams(clamped);
    const display = paramsToBestUnit(count);
    setParamMaxVal(display.value);
    setParamMaxUnit(display.unit);
  };

  // ── Distinct filter values ────────────────────────────────────────────────

  const distinctValues = useMemo(() => {
    if (!data) return { categories: [], owners: [], architectures: [], families: [] };
    return {
      categories: [...new Set(data.map(m => m.category).filter(Boolean))].sort(),
      owners: [...new Set(data.map(m => m.owned_by).filter(Boolean))].sort(),
      architectures: [...new Set(data.map(m => m.architecture).filter(Boolean))].sort(),
      families: [...new Set(data.map(m => m.model_family).filter(Boolean))].sort(),
    };
  }, [data]);

  // ── Filtered data ─────────────────────────────────────────────────────────

  const filteredData = useMemo(() => {
    if (!data) return [];
    return data.filter(model => {
      if (searchText && !model.id.toLowerCase().includes(searchText.toLowerCase())) return false;
      if (selectedCategories.size > 0 && !selectedCategories.has(model.category)) return false;
      if (selectedOwners.size > 0 && !selectedOwners.has(model.owned_by)) return false;
      if (selectedArchitectures.size > 0) {
        if (!model.architecture || !selectedArchitectures.has(model.architecture)) return false;
      }
      if (selectedFamilies.size > 0 && !selectedFamilies.has(model.model_family)) return false;
      if (model.total_size_bytes < sizeMinBytes || model.total_size_bytes > sizeMaxBytes) return false;
      if (paramMin > 0 || paramMax < PARAM_MAX) {
        const pc = model.parameter_count || 0;
        if (pc > 0 && (pc < paramMin || pc > paramMax)) return false;
        if (pc === 0 && paramMin > 0) return false;
      }
      if (downloadedFilter === 'yes' && !model.downloaded) return false;
      if (downloadedFilter === 'no' && model.downloaded) return false;
      if (validatedFilter === 'yes' && !model.validated) return false;
      if (validatedFilter === 'no' && model.validated) return false;
      for (const cap of selectedCapabilities) {
        if (!(model.capabilities as unknown as Record<string, boolean>)[cap]) return false;
      }
      return true;
    });
  }, [data, searchText, selectedCategories, selectedOwners, selectedArchitectures, selectedFamilies, sizeMinBytes, sizeMaxBytes, paramMin, paramMax, downloadedFilter, validatedFilter, selectedCapabilities]);

  const hasActiveFilters = searchText !== '' ||
    selectedCategories.size > 0 || selectedOwners.size > 0 ||
    selectedArchitectures.size > 0 || selectedFamilies.size > 0 ||
    sizeMinBytes > 0 || sizeMaxBytes < SIZE_MAX_BYTES ||
    paramMin > 0 || paramMax < PARAM_MAX ||
    downloadedFilter !== 'all' || validatedFilter !== 'all' ||
    selectedCapabilities.size > 0;

  const clearAllFilters = () => {
    setSearchText('');
    setSelectedCategories(new Set());
    setSelectedOwners(new Set());
    setSelectedArchitectures(new Set());
    setSelectedFamilies(new Set());
    setSizeMinVal('0');
    setSizeMinUnit('MB');
    setSizeMaxVal('4');
    setSizeMaxUnit('TB');
    setParamMinVal('0');
    setParamMinUnit('M');
    setParamMaxVal('1000');
    setParamMaxUnit('B');
    setDownloadedFilter('all');
    setValidatedFilter('all');
    setSelectedCapabilities(new Set());
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
    if (download?.kind === 'catalog' && download.catalogId) {
      if (download.status === 'downloading') {
        setSelectedId(download.catalogId);
        setActiveSection('pull');
      }
      if (download.status === 'complete') {
        loadCatalog();
      }
    }
  }, [download?.status]);

  const loadCatalog = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await api.listCatalog();
      setData(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load catalog');
    } finally {
      setLoading(false);
    }
  };

  const handleRowClick = async (id: string) => {
    if (selectedId === id) {
      setSelectedId(null);
      setModelInfo(null);
      setActiveSection('catalog');
      return;
    }

    setSelectedId(id);
    setActiveSection('catalog');
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

  // Seed VRAM calculator from model info
  const vramInputRef = modelInfo?.vram?.input;
  useEffect(() => {
    if (vramInputRef) {
      setVramCtx(vramInputRef.context_window);
      setVramBytes(vramInputRef.bytes_per_element);
      setVramSlots(vramInputRef.slots);
    }
  }, [vramInputRef]);

  // Compute VRAM locally from model header data
  const vramInput = modelInfo?.vram?.input;
  const vramResult = vramInput
    ? calculateVRAM({ ...vramInput, context_window: vramCtx, bytes_per_element: vramBytes, slots: vramSlots })
    : null;

  const handlePull = () => {
    if (!selectedId) return;
    startCatalogDownload(selectedId, downloadServer || undefined);
    setActiveSection('pull');
  };

  const handleCancelPull = () => {
    cancelDownload();
  };

  const isDownloaded = data?.find((m) => m.id === selectedId)?.downloaded ?? false;

  // ── Render ────────────────────────────────────────────────────────────────

  return (
    <div>
      <div className="page-header">
        <h2>Catalog</h2>
        <p>Browse available models in the catalog. Click a model to view details.</p>
      </div>

      <div className="playground-layout">
        {/* ── Filter Sidebar ────────────────────────────────────────────── */}
        <div className="catalog-filter-sidebar">
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

          {/* Category */}
          {distinctValues.categories.length > 0 && (
            <FilterSection title="Category" expanded={expandedSections.has('category')} onToggle={() => toggleSection('category')}>
              <div className="catalog-filter-options">
                {distinctValues.categories.map(cat => (
                  <label key={cat}>
                    <input
                      type="checkbox"
                      checked={selectedCategories.has(cat)}
                      onChange={() => setSelectedCategories(prev => toggleSet(prev, cat))}
                    />
                    {cat}
                  </label>
                ))}
              </div>
            </FilterSection>
          )}

          {/* Owner */}
          {distinctValues.owners.length > 0 && (
            <FilterSection title="Owner" expanded={expandedSections.has('owner')} onToggle={() => toggleSection('owner')}>
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

          {/* Architecture */}
          {distinctValues.architectures.length > 0 && (
            <FilterSection title="Architecture" expanded={expandedSections.has('architecture')} onToggle={() => toggleSection('architecture')}>
              <div className="catalog-filter-options">
                {distinctValues.architectures.map(arch => (
                  <label key={arch}>
                    <input
                      type="checkbox"
                      checked={selectedArchitectures.has(arch)}
                      onChange={() => setSelectedArchitectures(prev => toggleSet(prev, arch))}
                    />
                    {arch}
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

          {/* Parameters */}
          <FilterSection title="Parameters" expanded={expandedSections.has('parameters')} onToggle={() => toggleSection('parameters')}>
            <div className="catalog-range-row">
              <label>Min</label>
              <input
                type="number"
                min="0"
                step="any"
                value={paramMinVal}
                onChange={(e) => setParamMinVal(e.target.value)}
              />
              <select value={paramMinUnit} onChange={(e) => setParamMinUnit(e.target.value as ParamUnit)}>
                <option value="M">M</option>
                <option value="B">B</option>
              </select>
            </div>
            <div className="catalog-range-row">
              <label>Max</label>
              <input
                type="number"
                min="0"
                step="any"
                value={paramMaxVal}
                onChange={(e) => setParamMaxVal(e.target.value)}
              />
              <select value={paramMaxUnit} onChange={(e) => setParamMaxUnit(e.target.value as ParamUnit)}>
                <option value="M">M</option>
                <option value="B">B</option>
              </select>
            </div>
            <div className="catalog-dual-range">
              <div className="catalog-dual-range-track">
                <div
                  className="catalog-dual-range-fill"
                  style={{
                    left: `${(paramSliderMin / SLIDER_STEPS) * 100}%`,
                    right: `${100 - (paramSliderMax / SLIDER_STEPS) * 100}%`,
                  }}
                />
              </div>
              <input
                type="range"
                min={0}
                max={SLIDER_STEPS}
                value={paramSliderMin}
                onChange={(e) => handleParamSliderMin(Number(e.target.value))}
              />
              <input
                type="range"
                min={0}
                max={SLIDER_STEPS}
                value={paramSliderMax}
                onChange={(e) => handleParamSliderMax(Number(e.target.value))}
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

          <div className="catalog-filter-actions">
            <button
              className="btn btn-secondary"
              onClick={() => {
                loadCatalog();
                setSelectedId(null);
                setModelInfo(null);
                setActiveSection('catalog');
                setInfoError(null);
              }}
              disabled={loading}
            >
              Refresh
            </button>
            {selectedId && (
              <>
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label htmlFor="download-server" style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '12px', fontWeight: 600, color: 'var(--color-gray-600)', marginBottom: '4px' }}>
                    Download Server
                    <ParamTooltip text="Optional: specify a Kronk server on your local network that already has the model files. The pull will download from that server instead of HuggingFace, which is much faster." />
                  </label>
                  <input
                    id="download-server"
                    type="text"
                    className="form-control"
                    placeholder="192.168.0.246:8080"
                    value={downloadServer}
                    onChange={(e) => handleDownloadServerChange(e.target.value)}
                    disabled={pulling}
                    style={{ width: '100%', padding: '6px 8px', border: '1px solid var(--color-gray-300)', borderRadius: '4px', fontSize: '13px' }}
                  />
                </div>
                <button
                  className="btn btn-primary"
                  onClick={handlePull}
                  disabled={pulling || isDownloaded}
                >
                  {pulling ? 'Pulling...' : isDownloaded ? 'Already Downloaded' : 'Pull Model'}
                </button>
                <button
                  className="btn btn-secondary"
                  onClick={() => navigate(`/catalog/editor?id=${encodeURIComponent(selectedId)}`)}
                >
                  Edit
                </button>
              </>
            )}
            {pulling && (
              <button className="btn btn-danger" onClick={handleCancelPull}>
                Cancel
              </button>
            )}
          </div>
        </div>

        {/* ── Main Content ──────────────────────────────────────────────── */}
        <div className="playground-test" style={{ flex: 1 }}>
          <div className="playground-tab-content" style={{ overflow: 'auto' }}>
            {loading && <div className="loading">Loading catalog</div>}

            {error && <div className="alert alert-error">{error}</div>}

            {!loading && !error && data && (
              <div className="catalog-model-selector">
                <ModelSelector
                  models={filteredData}
                  selectedModel={selectedId}
                  onSelect={handleRowClick}
                  disabled={loading}
                />
                {filteredData.length === 0 && (
                  <div className="empty-state">
                    <h3>{hasActiveFilters ? 'No matching models' : 'No catalog entries'}</h3>
                    <p>{hasActiveFilters ? 'Try adjusting your filters' : 'The catalog is empty'}</p>
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

            {selectedId && !infoLoading && (modelInfo || hasCatalogPullActivity) && (
              <div style={{ marginTop: '16px', borderTop: '1px solid var(--color-gray-200)', paddingTop: '16px' }}>
                <div className="tabs">
                  {(Object.keys(SECTION_LABELS) as DetailSection[]).map(section => (
                    <button
                      key={section}
                      className={`tab ${activeSection === section ? 'active' : ''}`}
                      onClick={() => setActiveSection(section)}
                      disabled={section === 'pull' && pullMessages.length === 0 && !pulling}
                    >
                      {SECTION_LABELS[section]}
                    </button>
                  ))}
                </div>

                {/* Catalog Section */}
                {activeSection === 'catalog' && modelInfo && (
                  <div>
                    <h3 style={{ marginBottom: '16px' }}>{modelInfo.id}</h3>

                    {modelInfo.metadata.description && (
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
                        { key: 'model-url', label: 'Model URL', value: modelInfo.files.model.length > 0 ? modelInfo.files.model.map((file, idx) => <div key={idx}>{file.url} {file.size && `(${file.size})`}</div>) : '-' },
                        { key: 'proj-url', label: 'Projection URL', value: modelInfo.files.proj.url ? <div>{modelInfo.files.proj.url} {modelInfo.files.proj.size && `(${modelInfo.files.proj.size})`}</div> : '-' },
                      ]} />
                    </div>

                    <div style={{ marginBottom: '24px' }}>
                      <h4 className="meta-section-title" style={{ marginBottom: '8px' }}>Catalog Metadata</h4>
                      <KeyValueTable rows={[
                        { key: 'created', label: 'Created', value: new Date(modelInfo.metadata.created).toLocaleString() },
                        { key: 'collections', label: 'Collections', value: modelInfo.metadata.collections || '-' },
                      ]} />
                    </div>

                    <KeyValueTable rows={[
                      { key: 'category', label: 'Category', value: modelInfo.category },
                      { key: 'owner', label: 'Owner', value: modelInfo.owned_by },
                      { key: 'family', label: 'Family', value: modelInfo.model_family },
                      { key: 'arch', label: 'Architecture', value: modelInfo.architecture || '-' },
                      { key: 'gguf', label: 'GGUF Arch', value: modelInfo.gguf_arch || '-' },
                      { key: 'params', label: 'Parameters', value: modelInfo.parameters || '-' },
                      { key: 'size', label: 'Size', value: modelInfo.total_size || '-' },
                      { key: 'downloaded', label: 'Downloaded', value: <span className={`badge ${modelInfo.downloaded ? 'badge-yes' : 'badge-no'}`}>{modelInfo.downloaded ? 'Yes' : 'No'}</span> },
                      { key: 'gated', label: 'Gated Model', value: <span className={`badge ${modelInfo.gated_model ? 'badge-yes' : 'badge-no'}`}>{modelInfo.gated_model ? 'Yes' : 'No'}</span> },
                      { key: 'validated', label: 'Validated', value: <span style={{ color: modelInfo.validated ? 'inherit' : 'var(--color-error)' }}>{modelInfo.validated ? '✓' : '✗'}</span> },
                      { key: 'endpoint', label: 'Endpoint', value: modelInfo.capabilities.endpoint },
                      { key: 'template', label: 'Template', value: modelInfo.template || '-' },
                    ]} />

                    <div style={{ marginTop: '24px' }}>
                      <h4 className="meta-section-title" style={{ marginBottom: '8px' }}>Capabilities</h4>
                      <KeyValueTable rows={CAPABILITY_KEYS.map(cap => ({
                        key: cap,
                        label: CAPABILITY_LABELS[cap],
                        value: <span className={`badge ${modelInfo.capabilities[cap] ? 'badge-yes' : 'badge-no'}`}>{modelInfo.capabilities[cap] ? 'Yes' : 'No'}</span>,
                      }))} />
                    </div>

                  </div>
                )}

                {/* Configuration Section */}
                {activeSection === 'config' && modelInfo && (
                  <div>
                    <h3 style={{ marginBottom: '16px' }}>Model Configuration</h3>
                    {modelInfo.model_config ? (
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
                    ) : (
                      <div className="empty-state">
                        <p>No configuration available for this model.</p>
                      </div>
                    )}
                  </div>
                )}

                {/* Sampling Parameters Section */}
                {activeSection === 'sampling' && modelInfo && (
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
                {activeSection === 'metadata' && modelInfo && (
                  <div>
                    <h3 style={{ marginBottom: '16px' }}>Metadata</h3>
                    {modelInfo.model_metadata && Object.keys(modelInfo.model_metadata).filter(k => k !== 'tokenizer.chat_template').length > 0 ? (
                      <MetadataSection
                        metadata={modelInfo.model_metadata}
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
                {activeSection === 'template' && modelInfo && (
                  <div>
                    <h3 style={{ marginBottom: '16px' }}>Template</h3>
                    <KeyValueTable rows={[
                      { key: 'name', label: 'Template Name', value: modelInfo.template || '-' },
                    ]} />
                    <div style={{ marginTop: '16px' }}>
                      <h4 className="meta-section-title" style={{ marginBottom: '8px' }}>Chat Template</h4>
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

                {/* Pull Output Section */}
                {activeSection === 'pull' && (
                  <div>
                    <h3 style={{ marginBottom: '16px' }}>Pull Output: {selectedId}</h3>
                    {pullMessages.length > 0 ? (
                      <div className="status-box">
                        {pullMessages.map((msg, idx) => (
                          <div key={idx} className={`status-line ${msg.type}`}>
                            {msg.text}
                          </div>
                        ))}
                      </div>
                    ) : (
                      <p>No pull output yet.</p>
                    )}
                    {pulling && (
                      <button
                        className="btn btn-danger"
                        onClick={handleCancelPull}
                        style={{ marginTop: '16px' }}
                      >
                        Cancel
                      </button>
                    )}
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
