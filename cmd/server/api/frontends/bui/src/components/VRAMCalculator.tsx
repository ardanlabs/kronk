import { useState, useRef, useCallback, useMemo } from 'react';
import { api } from '../services/api';
import { useToken } from '../contexts/TokenContext';
import type { VRAMCalculatorResponse, HFRepoFile } from '../types';
import { VRAMCalculatorPanel, useVRAMState } from './vram';

/** Regex matching the "-NNNNN-of-NNNNN" suffix on split GGUF files. */
const splitSuffixRe = /-\d+-of-\d+\.gguf$/i;

/** Extract the filename portion from a HuggingFace URL or short-form path. */
function extractFilename(url: string): string {
  const trimmed = url.trim();

  // Full URL: .../resolve/main/path/file.gguf or .../blob/main/path/file.gguf
  for (const marker of ['/resolve/main/', '/blob/main/']) {
    const idx = trimmed.indexOf(marker);
    if (idx >= 0) return trimmed.slice(idx + marker.length);
  }

  // Short form: owner/repo/file.gguf → skip first two segments.
  const stripped = trimmed
    .replace(/^https?:\/\/huggingface\.co\//, '')
    .replace(/^https?:\/\/hf\.co\//, '')
    .replace(/^huggingface\.co\//, '')
    .replace(/^hf\.co\//, '');
  const parts = stripped.split('/');
  if (parts.length > 2) return parts.slice(2).join('/');

  return '';
}

/** Build a model URL from owner/repo extracted from the original URL plus a filename. */
function buildModelUrl(originalUrl: string, filename: string): string {
  const trimmed = originalUrl.trim();

  // If the original was a full HuggingFace URL, reconstruct with the new filename.
  for (const marker of ['/resolve/main/', '/blob/main/']) {
    const idx = trimmed.indexOf(marker);
    if (idx >= 0) return trimmed.slice(0, idx) + '/resolve/main/' + filename;
  }

  // Strip any known prefixes to get owner/repo.
  const stripped = trimmed
    .replace(/^https?:\/\/huggingface\.co\//, '')
    .replace(/^https?:\/\/hf\.co\//, '')
    .replace(/^huggingface\.co\//, '')
    .replace(/^hf\.co\//, '');
  const parts = stripped.split('/');
  if (parts.length >= 2) {
    return parts[0] + '/' + parts[1] + '/' + filename;
  }

  return filename;
}

/** A display row in the file selector — either a single file or a collapsed split folder. */
interface DisplayRow {
  /** Display name shown in the table. */
  label: string;
  /** The filename used for selection (first shard for splits, full path for singles). */
  filename: string;
  /** Human-readable total size. */
  sizeStr: string;
  /** True when this row represents a multi-file split model. */
  isSplit: boolean;
  /** Number of split parts (1 for non-split). */
  parts: number;
  /** For split models, the folder path within the repo (e.g. "Model-Q8_0"). */
  folderPath: string;
}

/** Collapse split model shards into a single folder row, keeping singles as-is. */
function collapseRepoFiles(files: HFRepoFile[]): DisplayRow[] {
  const splitGroups = new Map<string, { files: HFRepoFile[]; dir: string }>();
  const singles: HFRepoFile[] = [];

  for (const f of files) {
    if (f.filename.toLowerCase().includes('mmproj')) continue;
    if (splitSuffixRe.test(f.filename)) {
      // Derive the group key by stripping the split suffix: "Model-Q8-00001-of-00002.gguf" → "Model-Q8"
      const base = f.filename.replace(/-\d+-of-\d+\.gguf$/i, '');
      const existing = splitGroups.get(base);
      if (existing) {
        existing.files.push(f);
      } else {
        // Use the directory portion (if any) as the folder label.
        const slashIdx = f.filename.lastIndexOf('/');
        const dir = slashIdx >= 0 ? f.filename.slice(0, slashIdx) : '';
        splitGroups.set(base, { files: [f], dir });
      }
    } else {
      singles.push(f);
    }
  }

  const rows: DisplayRow[] = [];

  for (const [base, group] of splitGroups) {
    const totalSize = group.files.reduce((sum, f) => sum + f.size, 0);
    // Use the directory if files live in a subfolder, otherwise derive from base name.
    const folder = group.dir || base.split('/').pop() || base;
    // Sort shards so we always pick the first one as the representative.
    group.files.sort((a, b) => a.filename.localeCompare(b.filename));
    rows.push({
      label: `📁 ${folder} (${group.files.length} parts)`,
      filename: group.files[0].filename,
      sizeStr: formatTotalSize(totalSize),
      isSplit: true,
      parts: group.files.length,
      folderPath: folder,
    });
  }

  for (const f of singles) {
    rows.push({
      label: f.filename,
      filename: f.filename,
      sizeStr: f.size_str,
      isSplit: false,
      parts: 1,
      folderPath: '',
    });
  }

  return rows;
}

function formatTotalSize(bytes: number): string {
  const gb = 1000 * 1000 * 1000;
  const mb = 1000 * 1000;
  if (bytes >= gb) return `${(bytes / gb).toFixed(1)} GB`;
  if (bytes >= mb) return `${(bytes / mb).toFixed(1)} MB`;
  return `${(bytes / 1000).toFixed(1)} KB`;
}

export default function VRAMCalculator() {
  const { token } = useToken();
  const [modelUrl, setModelUrl] = useState('');
  const [loading, setLoading] = useState(false);
  const [lookingUp, setLookingUp] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<VRAMCalculatorResponse | null>(null);
  const [calculatedModelUrl, setCalculatedModelUrl] = useState('');
  const [repoFiles, setRepoFiles] = useState<HFRepoFile[]>([]);
  const [selectedFilename, setSelectedFilename] = useState('');
  const [lookupUrl, setLookupUrl] = useState('');
  const cachedKeyRef = useRef('');

  const { controlsProps, resultsProps } = useVRAMState({ serverResponse: result, enableHardwareOverrides: true });

  const displayRows = useMemo(() => collapseRepoFiles(repoFiles), [repoFiles]);

  const handleLookup = useCallback(async () => {
    const trimmed = modelUrl.trim();
    if (!trimmed) {
      setError('Please enter a model URL');
      return;
    }

    setLookingUp(true);
    setError(null);
    setRepoFiles([]);
    setSelectedFilename('');
    setResult(null);
    setCalculatedModelUrl('');
    cachedKeyRef.current = '';

    try {
      const lookupResult = await api.lookupHuggingFace(trimmed);
      const files: HFRepoFile[] = lookupResult.repo_files ?? [];
      setRepoFiles(files);
      setLookupUrl(trimmed);

      // Pre-select the file if the URL pointed to a specific model.
      const inputFilename = extractFilename(trimmed);
      if (inputFilename && files.some((f) => f.filename === inputFilename)) {
        setSelectedFilename(inputFilename);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Lookup failed');
    } finally {
      setLookingUp(false);
    }
  }, [modelUrl]);

  const performCalculation = useCallback(async (urlOverride?: string) => {
    let calcUrl: string;
    if (urlOverride) {
      calcUrl = urlOverride;
    } else if (selectedFilename) {
      // For split models, send a folder URL so the backend sums all shards.
      const selectedRow = displayRows.find((r) => r.filename === selectedFilename);
      if (selectedRow?.isSplit && selectedRow.folderPath) {
        calcUrl = buildModelUrl(lookupUrl || modelUrl, selectedRow.folderPath);
      } else {
        calcUrl = buildModelUrl(lookupUrl || modelUrl, selectedFilename);
      }
    } else {
      calcUrl = modelUrl.trim();
    }

    if (!calcUrl) {
      setError('Please select a model file');
      return;
    }

    const cacheKey = [
      calcUrl,
      controlsProps.contextWindow,
      controlsProps.bytesPerElement,
      controlsProps.slots,
      controlsProps.gpuLayers,
      controlsProps.expertLayersOnGPU,
      controlsProps.kvCacheOnCPU,
      controlsProps.deviceCount,
      controlsProps.tensorSplit,
    ].join('|');
    if (cacheKey === cachedKeyRef.current && result) {
      return;
    }

    setLoading(true);
    setError(null);
    setResult(null);

    try {
      const response = await api.calculateVRAM(
        {
          model_url: calcUrl,
          context_window: controlsProps.contextWindow,
          bytes_per_element: controlsProps.bytesPerElement,
          slots: controlsProps.slots,
        },
        token || undefined
      );
      setResult(response);
      setCalculatedModelUrl(calcUrl);
      cachedKeyRef.current = cacheKey;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to calculate VRAM');
      cachedKeyRef.current = '';
    } finally {
      setLoading(false);
    }
  }, [modelUrl, lookupUrl, selectedFilename, displayRows, controlsProps, token, result]);

  const handleCalculate = async (e: React.FormEvent) => {
    e.preventDefault();

    // If no lookup was done yet, do lookup + auto-calculate if a specific file was provided.
    if (repoFiles.length === 0) {
      const trimmed = modelUrl.trim();
      if (!trimmed) {
        setError('Please enter a model URL');
        return;
      }

      setLookingUp(true);
      setError(null);

      try {
        const lookupResult = await api.lookupHuggingFace(trimmed);
        const files: HFRepoFile[] = lookupResult.repo_files ?? [];
        setRepoFiles(files);
        setLookupUrl(trimmed);

        const inputFilename = extractFilename(trimmed);
        if (inputFilename && files.some((f) => f.filename === inputFilename)) {
          setSelectedFilename(inputFilename);
          setLookingUp(false);
          await performCalculation(trimmed);
          return;
        }

        // No specific file — show the list for selection.
        if (files.length > 0) {
          setLookingUp(false);
          return;
        }

        // No files found, try direct calculation.
        setLookingUp(false);
        await performCalculation(trimmed);
      } catch {
        setLookingUp(false);
        await performCalculation(trimmed);
      }
      return;
    }

    await performCalculation();
  };

  const handleFileSelect = (filename: string) => {
    setSelectedFilename(filename);
    cachedKeyRef.current = '';
    setResult(null);
  };

  return (
    <div className="page">
      <div className="page-header-with-action">
        <div>
          <h2>VRAM Calculator</h2>
          <p className="page-description">
            Calculate VRAM requirements for a model from HuggingFace. Only the model header is fetched, not the entire file.
          </p>
        </div>
        <a
          href="https://www.kronkai.com/blog/understanding-the-kronk-vram-calculator"
          target="_blank"
          rel="noopener noreferrer"
          className="btn btn-secondary"
        >
          Learn More
        </a>
      </div>

      <form onSubmit={handleCalculate} className="form-card">
        <div className="form-group">
                  <label htmlFor="modelUrl">                    
                    Ex. <code>bartowski/Qwen3-8B-GGUF:Q4_K_M</code> (shorthand)<br/>
                    Ex. <code>https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf</code><br/>
                    Ex. <code>https://huggingface.co/unsloth/Qwen3.5-35B-A3B-GGUF/tree/main</code><br/><br/>
                    Model URL, shorthand, or folder for split models
                  </label>
          <div style={{ display: 'flex', gap: '8px' }}>
            <input
              id="modelUrl"
              type="text"
              value={modelUrl}
              onChange={(e) => {
                setModelUrl(e.target.value);
                // Reset file list when URL changes so user can re-lookup.
                if (repoFiles.length > 0) {
                  setRepoFiles([]);
                  setSelectedFilename('');
                  setResult(null);
                  cachedKeyRef.current = '';
                }
              }}
              placeholder="bartowski/Qwen3-8B-GGUF:Q4_K_M"
              className="form-input"
              style={{ flex: 1 }}
            />
            <button
              type="button"
              className="btn btn-primary"
              disabled={lookingUp}
              onClick={handleLookup}
            >
              {lookingUp ? 'Looking up…' : 'Lookup'}
            </button>
          </div>
          <small className="form-hint">
            Enter a shorthand (owner/repo:TAG), full HuggingFace URL, or folder URL for split models
          </small>
        </div>

        {displayRows.length > 0 && (
          <div className="form-group">
            <label htmlFor="modelFileSelect" style={{ fontWeight: 600 }}>
              Select a model file
            </label>
            <select
              id="modelFileSelect"
              value={selectedFilename}
              onChange={(e) => handleFileSelect(e.target.value)}
              className="form-input"
            >
              <option value="">— Choose a file —</option>
              {displayRows.map((row) => (
                <option key={row.filename} value={row.filename}>
                  {row.label} ({row.sizeStr})
                </option>
              ))}
            </select>
          </div>
        )}

        <VRAMCalculatorPanel
          controlsProps={controlsProps}
          resultsProps={resultsProps}
          variant="form"
          hideResults
        />

        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '16px' }}>
          <button type="submit" className="btn btn-primary" disabled={loading || lookingUp || !selectedFilename}>
            {loading ? 'Calculating...' : 'Calculate VRAM'}
          </button>
        </div>
      </form>

      {(loading || lookingUp) && (
        <div className="vram-loading-banner">
          <span className="vram-loading-spinner" />
          <span>{lookingUp ? 'Looking up repository…' : 'Fetching model header (up to 16 MB)…'}</span>
        </div>
      )}

      {error && <div className="alert alert-error">{error}</div>}

      {resultsProps && (
        <VRAMCalculatorPanel
          controlsProps={controlsProps}
          resultsProps={resultsProps}
          hideControls
          modelUrl={calculatedModelUrl}
        />
      )}
    </div>
  );
}
