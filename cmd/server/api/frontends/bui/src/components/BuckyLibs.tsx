import { useState, useRef, useEffect, useMemo, useCallback } from 'react';
import { api } from '../services/api';
import type {
  VersionResponse,
  LibsCombination,
  LibsBundleTag,
} from '../types';
import { FieldLabel } from './ParamTooltips';

interface LibsManagerBackend {
  id: string;
  engine: string;
  backend: string;
  environmentVariable: string;
  rootFolder: string;
  getVersion: () => Promise<VersionResponse>;
  getCombinations: () => Promise<{ combinations: LibsCombination[] }>;
  listInstalls: () => Promise<{ bundles: LibsBundleTag[] }>;
  removeInstall: (arch: string, os: string, processor: string) => Promise<unknown>;
  pull: (
    onMessage: (data: VersionResponse) => void,
    onError: (error: string) => void,
    onComplete: () => void,
    opts?: { version?: string; arch?: string; os?: string; processor?: string },
  ) => () => void;
}

const buckyBackend: LibsManagerBackend = {
  id: 'bucky',
  engine: 'Whisper.cpp',
  backend: 'Bucky',
  environmentVariable: 'KRONK_BUCKY_LIB_PATH',
  rootFolder: 'bucky-libraries',
  getVersion: () => api.getBuckyLibsVersion(),
  getCombinations: () => api.getBuckyLibsCombinations(),
  listInstalls: () => api.listBuckyLibsInstalls(),
  removeInstall: (arch, os, processor) => api.removeBuckyLibsInstall(arch, os, processor),
  pull: (onMessage, onError, onComplete, opts) => api.pullBuckyLibs(onMessage, onError, onComplete, opts),
};

export default function BuckyLibs() {
  return <LibsManager backend={buckyBackend} />;
}

export function LibsManager({ backend }: { backend: LibsManagerBackend }) {
  const [pulling, setPulling] = useState(false);
  const [messages, setMessages] = useState<Array<{ text: string; type: 'info' | 'error' | 'success' }>>([]);
  const [versionInfo, setVersionInfo] = useState<VersionResponse | null>(null);
  const [loadingVersion, setLoadingVersion] = useState(true);
  const [version, setVersion] = useState('');
  const [bundles, setBundles] = useState<LibsBundleTag[]>([]);
  const closeRef = useRef<(() => void) | null>(null);

  const loadBundles = useCallback(async () => {
    try {
      const resp = await backend.listInstalls();
      setBundles(resp.bundles ?? []);
    } catch {
      setBundles([]);
    }
  }, [backend]);

  useEffect(() => {
    backend
      .getVersion()
      .then(setVersionInfo)
      .catch(() => {})
      .finally(() => setLoadingVersion(false));
    loadBundles();
  }, [backend, loadBundles]);

  const handlePull = () => {
    setPulling(true);
    setMessages([]);
    setVersionInfo(null);

    const addMessage = (text: string, type: 'info' | 'error' | 'success') => {
      setMessages((prev) => [...prev, { text, type }]);
    };

    closeRef.current = backend.pull(
      (data: VersionResponse) => {
        if (data.status) {
          addMessage(data.status, 'info');
        }
        if (data.current) {
          setVersionInfo(data);
        }
      },
      (error: string) => {
        addMessage(error, 'error');
        setPulling(false);
      },
      () => {
        addMessage('Libs update complete!', 'success');
        setPulling(false);
        loadBundles();
      },
      { version: version || undefined },
    );
  };

  const handleCancel = () => {
    if (closeRef.current) {
      closeRef.current();
      closeRef.current = null;
    }
    setPulling(false);
    setMessages((prev) => [...prev, { text: 'Cancelled', type: 'error' }]);
  };

  return (
    <div>
      <div className="page-header">
        <h2>{backend.engine} Libs</h2>
        <p>Download, update, and manage {backend.engine} libraries</p>
      </div>

      <div className="card">
        {loadingVersion ? (
          <p>Loading version info...</p>
        ) : versionInfo && versionInfo.current ? (
          <div style={{ marginBottom: '24px' }}>
            <h4 style={{ marginTop: 0, marginBottom: '12px' }}>Current Version</h4>
            <div className="model-meta">
              {versionInfo.arch && (
                <div className="model-meta-item">
                  <label>Architecture</label>
                  <span>{versionInfo.arch}</span>
                </div>
              )}
              {versionInfo.os && (
                <div className="model-meta-item">
                  <label>OS</label>
                  <span>{versionInfo.os}</span>
                </div>
              )}
              {versionInfo.processor && (
                <div className="model-meta-item">
                  <label>Processor</label>
                  <span>{versionInfo.processor}</span>
                </div>
              )}
              <div className="model-meta-item">
                <label>Installed Version</label>
                <span>{versionInfo.current}</span>
              </div>
            </div>
          </div>
        ) : (
          <p style={{ marginBottom: '24px', color: 'var(--color-gray-600)' }}>
            No {backend.backend} libs installed yet for the active triple.
          </p>
        )}

        <div className="form-group">
          <FieldLabel tooltipKey="bundleVersion" htmlFor={`${backend.id}-version`}>
            Version (leave empty for default)
          </FieldLabel>
          <input
            type="text"
            id={`${backend.id}-version`}
            value={version}
            onChange={(e) => setVersion(e.target.value)}
            disabled={pulling}
            placeholder="e.g. v1.7.6"
            style={{ maxWidth: '200px' }}
          />
        </div>

        <div style={{ display: 'flex', gap: '12px' }}>
          <button className="btn btn-primary" onClick={handlePull} disabled={pulling}>
            {pulling ? 'Updating...' : 'Pull/Update Libs'}
          </button>
          {pulling && (
            <button className="btn btn-danger" onClick={handleCancel}>
              Cancel
            </button>
          )}
        </div>

        {messages.length > 0 && (
          <div className="status-box">
            {messages.map((msg, idx) => (
              <div key={idx} className={`status-line ${msg.type}`}>
                {msg.text}
              </div>
            ))}
          </div>
        )}
      </div>

      <InstalledBundlesSection backend={backend} bundles={bundles} onChanged={loadBundles} />

      <LibraryInstallsSection backend={backend} onChanged={loadBundles} />
    </div>
  );
}

function InstalledBundlesSection({ backend, bundles, onChanged }: { backend: LibsManagerBackend; bundles: LibsBundleTag[]; onChanged: () => void }) {
  const [error, setError] = useState<string | null>(null);

  const handleRemove = async (b: LibsBundleTag) => {
    if (!confirm(`Remove install ${b.os}/${b.arch}/${b.processor}?`)) return;
    setError(null);
    try {
      await backend.removeInstall(b.arch, b.os, b.processor);
      onChanged();
    } catch (err) {
      setError(`Remove failed: ${(err as Error).message}`);
    }
  };

  return (
    <div className="card" style={{ marginTop: 24 }}>
      <h3 style={{ marginTop: 0, marginBottom: 8 }}>Installed Bundles</h3>
      <p style={{ marginBottom: 16, color: 'var(--color-gray-600)', fontSize: 14 }}>
        {backend.engine} library bundles currently installed on disk under the {backend.backend} libraries
        root. To switch the active install, set <code>{backend.environmentVariable}</code> to a
        bundle's folder and restart the server.
      </p>

      {error && (
        <div className="status-box" style={{ marginBottom: 16 }}>
          <div className="status-line error">{error}</div>
        </div>
      )}

      {bundles.length === 0 ? (
        <p style={{ color: 'var(--color-gray-600)', fontSize: 14 }}>No installs found.</p>
      ) : (
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr>
              <th style={{ textAlign: 'left', padding: 8 }}>OS</th>
              <th style={{ textAlign: 'left', padding: 8 }}>Architecture</th>
              <th style={{ textAlign: 'left', padding: 8 }}>Processor</th>
              <th style={{ textAlign: 'left', padding: 8 }}>Version</th>
              <th style={{ textAlign: 'right', padding: 8 }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {bundles.map((b) => (
              <tr key={`${b.os}-${b.arch}-${b.processor}`}>
                <td style={{ padding: 8 }}>{b.os}</td>
                <td style={{ padding: 8 }}>{b.arch}</td>
                <td style={{ padding: 8 }}>{b.processor}</td>
                <td style={{ padding: 8 }}>{b.version}</td>
                <td style={{ padding: 8, textAlign: 'right' }}>
                  <button className="btn btn-danger" onClick={() => handleRemove(b)}>
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function LibraryInstallsSection({ backend, onChanged }: { backend: LibsManagerBackend; onChanged: () => void }) {
  const [combinations, setCombinations] = useState<LibsCombination[]>([]);
  const [arch, setArch] = useState('');
  const [os, setOS] = useState('');
  const [processor, setProcessor] = useState('');
  const [version, setVersion] = useState('');
  const [pulling, setPulling] = useState(false);
  const [messages, setMessages] = useState<Array<{ text: string; type: 'info' | 'error' | 'success' }>>([]);
  const [activationHint, setActivationHint] = useState<{ os: string; arch: string; processor: string } | null>(null);
  const closeRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    backend.getCombinations()
      .then((resp) => setCombinations(resp.combinations ?? []))
      .catch(() => setCombinations([]));
  }, [backend]);

  // Filter the dropdowns so users can only pick valid (os, arch, processor) triples.
  const osOptions = useMemo(() => Array.from(new Set(combinations.map((c) => c.os))).sort(), [combinations]);
  const archOptions = useMemo(() => {
    const filtered = combinations.filter((c) => !os || c.os === os);
    return Array.from(new Set(filtered.map((c) => c.arch))).sort();
  }, [combinations, os]);
  const processorOptions = useMemo(() => {
    const filtered = combinations.filter((c) => (!os || c.os === os) && (!arch || c.arch === arch));
    return Array.from(new Set(filtered.map((c) => c.processor))).sort();
  }, [combinations, os, arch]);

  const tripleSelected = arch && os && processor;

  const addMessage = (text: string, type: 'info' | 'error' | 'success') => {
    setMessages((prev) => [...prev, { text, type }]);
  };

  const handlePull = () => {
    if (!tripleSelected) return;
    setPulling(true);
    setMessages([]);
    setActivationHint(null);

    closeRef.current = backend.pull(
      (data: VersionResponse) => {
        if (data.status) addMessage(data.status, 'info');
      },
      (err: string) => {
        addMessage(err, 'error');
        setPulling(false);
      },
      () => {
        addMessage('Bundle download complete!', 'success');
        setActivationHint({ os, arch, processor });
        setPulling(false);
        onChanged();
      },
      { arch, os, processor, version: version || undefined },
    );
  };

  const handleCancel = () => {
    if (closeRef.current) {
      closeRef.current();
      closeRef.current = null;
    }
    setPulling(false);
    addMessage('Cancelled', 'error');
  };

  return (
    <div className="card" style={{ marginTop: 24 }}>
      <h3 style={{ marginTop: 0, marginBottom: 8 }}>Library Installs</h3>
      <p style={{ marginBottom: 16, color: 'var(--color-gray-600)', fontSize: 14 }}>
        Install {backend.engine} library bundles for any supported (arch, os, processor) combination.
        Each install lives in its own folder under the {backend.backend} libraries root. To run Kronk against
        a non-default install, set <code>{backend.environmentVariable}</code> to that folder and restart
        the server.
      </p>

      <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', marginBottom: 16 }}>
        <div className="form-group" style={{ minWidth: 160 }}>
          <FieldLabel tooltipKey="bundleOS" htmlFor={`${backend.id}-bundle-os`}>OS</FieldLabel>
          <select id={`${backend.id}-bundle-os`} value={os} onChange={(e) => { setOS(e.target.value); setArch(''); setProcessor(''); }} disabled={pulling}>
            <option value="">Select…</option>
            {osOptions.map((v) => <option key={v} value={v}>{v}</option>)}
          </select>
        </div>
        <div className="form-group" style={{ minWidth: 160 }}>
          <FieldLabel tooltipKey="bundleArch" htmlFor={`${backend.id}-bundle-arch`}>Architecture</FieldLabel>
          <select id={`${backend.id}-bundle-arch`} value={arch} onChange={(e) => { setArch(e.target.value); setProcessor(''); }} disabled={pulling || !os}>
            <option value="">Select…</option>
            {archOptions.map((v) => <option key={v} value={v}>{v}</option>)}
          </select>
        </div>
        <div className="form-group" style={{ minWidth: 160 }}>
          <FieldLabel tooltipKey="bundleProcessor" htmlFor={`${backend.id}-bundle-processor`}>Processor</FieldLabel>
          <select id={`${backend.id}-bundle-processor`} value={processor} onChange={(e) => setProcessor(e.target.value)} disabled={pulling || !arch}>
            <option value="">Select…</option>
            {processorOptions.map((v) => <option key={v} value={v}>{v}</option>)}
          </select>
        </div>
        <div className="form-group" style={{ minWidth: 160 }}>
          <FieldLabel tooltipKey="bundleVersion" htmlFor={`${backend.id}-bundle-version`}>Version (optional)</FieldLabel>
          <input
            type="text"
            id={`${backend.id}-bundle-version`}
            value={version}
            onChange={(e) => setVersion(e.target.value)}
            placeholder="default"
            disabled={pulling}
          />
        </div>
      </div>

      <div style={{ display: 'flex', gap: 12, marginBottom: 16 }}>
        <button className="btn btn-primary" onClick={handlePull} disabled={pulling || !tripleSelected}>
          {pulling ? 'Downloading…' : 'Download Bundle'}
        </button>
        {pulling && (
          <button className="btn btn-danger" onClick={handleCancel}>Cancel</button>
        )}
      </div>

      {messages.length > 0 && (
        <div className="status-box" style={{ marginBottom: 16 }}>
          {messages.map((msg, idx) => (
            <div key={idx} className={`status-line ${msg.type}`}>{msg.text}</div>
          ))}
        </div>
      )}

      {activationHint && (
        <div className="status-box" style={{ marginTop: 8 }}>
          <div className="status-line info" style={{ marginBottom: 8 }}>
            To activate this bundle, set <code>{backend.environmentVariable}</code> to its folder
            and restart the server. Libraries are not hot-reloaded.
          </div>
          <pre style={{
            margin: 0,
            fontFamily: '"SF Mono", "Monaco", "Inconsolata", "Fira Code", monospace',
            fontSize: 13,
            whiteSpace: 'pre',
            overflowX: 'auto',
          }}>
{`export ${backend.environmentVariable}=~/.kronk/${backend.rootFolder}/${activationHint.os}/${activationHint.arch}/${activationHint.processor}
kronk server start`}
          </pre>
        </div>
      )}
    </div>
  );
}
