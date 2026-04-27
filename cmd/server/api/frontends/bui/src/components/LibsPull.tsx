import { useState, useRef, useEffect, useMemo, useCallback } from 'react';
import { api } from '../services/api';
import type {
  VersionResponse,
  LibsCombination,
  LibsBundleTag,
  LibsPeerBundle,
  LibsPeerPullEvent,
} from '../types';
import { FieldLabel, ParamTooltip } from './ParamTooltips';

export default function LibsPull() {
  const [pulling, setPulling] = useState(false);
  const [messages, setMessages] = useState<Array<{ text: string; type: 'info' | 'error' | 'success' }>>([]);
  const [versionInfo, setVersionInfo] = useState<VersionResponse | null>(null);
  const [loadingVersion, setLoadingVersion] = useState(true);
  const [overrideUpgrade, setOverrideUpgrade] = useState(false);
  const [version, setVersion] = useState('');
  const [bundles, setBundles] = useState<LibsBundleTag[]>([]);
  const closeRef = useRef<(() => void) | null>(null);

  const loadBundles = useCallback(async () => {
    try {
      const resp = await api.listLibsInstalls();
      setBundles(resp.bundles ?? []);
    } catch {
      setBundles([]);
    }
  }, []);

  useEffect(() => {
    api
      .getLibsVersion()
      .then(setVersionInfo)
      .catch(() => {})
      .finally(() => setLoadingVersion(false));
    loadBundles();
  }, [loadBundles]);

  const handlePull = () => {
    setPulling(true);
    setMessages([]);
    setVersionInfo(null);

    const addMessage = (text: string, type: 'info' | 'error' | 'success') => {
      setMessages((prev) => [...prev, { text, type }]);
    };

    const useAllowUpgrade = overrideUpgrade ? true : undefined;

    closeRef.current = api.pullLibs(
      (data: VersionResponse) => {
        if (data.status) {
          addMessage(data.status, 'info');
        }
        if (data.current || data.latest) {
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
      },
      { allowUpgrade: useAllowUpgrade, version: version || undefined }
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
        <h2>Manage Libs</h2>
        <p>Download, update, and manage Kronk libraries</p>
      </div>

      <div className="card">
        {loadingVersion ? (
          <p>Loading version info...</p>
        ) : versionInfo ? (
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
              {versionInfo.current && (
                <div className="model-meta-item">
                  <label>Installed Version</label>
                  <span>{versionInfo.current}</span>
                </div>
              )}
              {versionInfo.latest && (
                <div className="model-meta-item">
                  <label>Latest Version</label>
                  <span>{versionInfo.latest}</span>
                </div>
              )}
            </div>
          </div>
        ) : (
          <p style={{ marginBottom: '24px', color: 'var(--color-gray-600)' }}>
            No libs installed yet.
          </p>
        )}

        {versionInfo && (
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '16px' }}>
            <input
              type="checkbox"
              checked={versionInfo.allow_upgrade || overrideUpgrade}
              disabled={versionInfo.allow_upgrade || pulling}
              onChange={(e) => setOverrideUpgrade(e.target.checked)}
              id="allow-upgrade"
            />
            <label htmlFor="allow-upgrade" style={{ fontSize: '14px', cursor: versionInfo.allow_upgrade ? 'default' : 'pointer' }}>
              Allow Upgrade
            </label>
            <ParamTooltip text="Controls this server's library upgrade policy. When enabled, the server tracks the latest llama.cpp release; otherwise it stays on the version currently installed. Independent of the standalone `kronk libs` CLI, which has its own --upgrade flag." />
          </div>
        )}

        <div className="form-group">
          <label htmlFor="version">
            Version (leave empty for latest)
          </label>
          <input
            type="text"
            id="version"
            value={version}
            onChange={(e) => setVersion(e.target.value)}
            disabled={pulling}
            placeholder="e.g. b5540"
            style={{ maxWidth: '200px' }}
          />
        </div>

        <div style={{ display: 'flex', gap: '12px' }}>
          <button className="btn btn-primary" onClick={handlePull} disabled={pulling || (versionInfo !== null && !versionInfo.allow_upgrade && !overrideUpgrade)}>
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

      <InstalledBundlesSection bundles={bundles} onChanged={loadBundles} />

      <LibraryInstallsSection onChanged={loadBundles} />

      <PeerBundleSection installed={bundles} onChanged={loadBundles} />
    </div>
  );
}

function InstalledBundlesSection({ bundles, onChanged }: { bundles: LibsBundleTag[]; onChanged: () => void }) {
  const [error, setError] = useState<string | null>(null);

  const handleRemove = async (b: LibsBundleTag) => {
    if (!confirm(`Remove install ${b.os}/${b.arch}/${b.processor}?`)) return;
    setError(null);
    try {
      await api.removeLibsInstall(b.arch, b.os, b.processor);
      onChanged();
    } catch (err) {
      setError(`Remove failed: ${(err as Error).message}`);
    }
  };

  return (
    <div className="card" style={{ marginTop: 24 }}>
      <h3 style={{ marginTop: 0, marginBottom: 8 }}>Installed Bundles</h3>
      <p style={{ marginBottom: 16, color: 'var(--color-gray-600)', fontSize: 14 }}>
        Library bundles currently installed on disk under the libraries root. To switch
        the active install, set <code>KRONK_LIB_PATH</code> to a bundle's folder and
        restart the server.
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

function LibraryInstallsSection({ onChanged }: { onChanged: () => void }) {
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
    api.getLibsCombinations()
      .then((resp) => setCombinations(resp.combinations ?? []))
      .catch(() => setCombinations([]));
  }, []);

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

    closeRef.current = api.pullLibs(
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
        Install llama.cpp library bundles for any supported (arch, os, processor) combination.
        Each install lives in its own folder under the libraries root. To run Kronk against a
        non-default install, set <code>KRONK_LIB_PATH</code> to that folder and restart the server.
      </p>

      <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', marginBottom: 16 }}>
        <div className="form-group" style={{ minWidth: 160 }}>
          <FieldLabel tooltipKey="bundleOS" htmlFor="bundle-os">OS</FieldLabel>
          <select id="bundle-os" value={os} onChange={(e) => { setOS(e.target.value); setArch(''); setProcessor(''); }} disabled={pulling}>
            <option value="">Select…</option>
            {osOptions.map((v) => <option key={v} value={v}>{v}</option>)}
          </select>
        </div>
        <div className="form-group" style={{ minWidth: 160 }}>
          <FieldLabel tooltipKey="bundleArch" htmlFor="bundle-arch">Architecture</FieldLabel>
          <select id="bundle-arch" value={arch} onChange={(e) => { setArch(e.target.value); setProcessor(''); }} disabled={pulling || !os}>
            <option value="">Select…</option>
            {archOptions.map((v) => <option key={v} value={v}>{v}</option>)}
          </select>
        </div>
        <div className="form-group" style={{ minWidth: 160 }}>
          <FieldLabel tooltipKey="bundleProcessor" htmlFor="bundle-processor">Processor</FieldLabel>
          <select id="bundle-processor" value={processor} onChange={(e) => setProcessor(e.target.value)} disabled={pulling || !arch}>
            <option value="">Select…</option>
            {processorOptions.map((v) => <option key={v} value={v}>{v}</option>)}
          </select>
        </div>
        <div className="form-group" style={{ minWidth: 160 }}>
          <label htmlFor="bundle-version">Version (optional)</label>
          <input
            type="text"
            id="bundle-version"
            value={version}
            onChange={(e) => setVersion(e.target.value)}
            placeholder="latest"
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
            To activate this bundle, set <code>KRONK_LIB_PATH</code> to its folder
            and restart the server. Libraries are not hot-reloaded.
          </div>
          <pre style={{
            margin: 0,
            fontFamily: '"SF Mono", "Monaco", "Inconsolata", "Fira Code", monospace',
            fontSize: 13,
            whiteSpace: 'pre',
            overflowX: 'auto',
          }}>
{`export KRONK_LIB_PATH=~/.kronk/libraries/${activationHint.os}/${activationHint.arch}/${activationHint.processor}
kronk server start`}
          </pre>
        </div>
      )}
    </div>
  );
}

// =============================================================================

const PEER_HOST_STORAGE_KEY = 'kronk_lib_peer_server';

type PeerBundleKey = string;

function bundleKey(b: { os: string; arch: string; processor: string }): PeerBundleKey {
  return `${b.os}|${b.arch}|${b.processor}`;
}

interface PeerPullState {
  status: string;
  bytes?: number;
  bytesTotal?: number;
  mbPerSecond?: number;
  size?: number;
  sha256?: string;
  error?: string;
}

function formatBytes(n: number): string {
  if (!n || n <= 0) return '';
  const mb = n / (1000 * 1000);
  if (mb < 1024) return `${mb.toFixed(1)} MB`;
  return `${(mb / 1024).toFixed(2)} GB`;
}

function PeerBundleSection({ installed, onChanged }: { installed: LibsBundleTag[]; onChanged: () => void }) {
  const [host, setHost] = useState<string>(() => localStorage.getItem(PEER_HOST_STORAGE_KEY) || '');
  const [connecting, setConnecting] = useState(false);
  const [bundles, setBundles] = useState<LibsPeerBundle[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pullStates, setPullStates] = useState<Record<PeerBundleKey, PeerPullState>>({});
  const cancelRefs = useRef<Record<PeerBundleKey, (() => void) | null>>({});

  const installedKeys = useMemo(() => {
    const set = new Set<PeerBundleKey>();
    for (const b of installed) set.add(bundleKey(b));
    return set;
  }, [installed]);

  const updateHost = (value: string) => {
    setHost(value);
    if (value) {
      localStorage.setItem(PEER_HOST_STORAGE_KEY, value);
    } else {
      localStorage.removeItem(PEER_HOST_STORAGE_KEY);
    }
  };

  const handleConnect = async () => {
    const trimmed = host.trim();
    if (!trimmed) return;

    setConnecting(true);
    setError(null);
    setBundles(null);

    try {
      const resp = await api.listPeerLibsBundles(trimmed);
      setBundles(resp.bundles ?? []);
    } catch (err) {
      setError(`Connect failed: ${(err as Error).message}`);
    } finally {
      setConnecting(false);
    }
  };

  const handleDownload = (b: LibsPeerBundle) => {
    const trimmed = host.trim();
    if (!trimmed) return;

    const key = bundleKey(b);

    setPullStates((prev) => ({
      ...prev,
      [key]: { status: 'connecting' },
    }));

    cancelRefs.current[key] = api.pullLibsFromPeer(
      trimmed,
      b.arch,
      b.os,
      b.processor,
      (data: LibsPeerPullEvent) => {
        setPullStates((prev) => ({
          ...prev,
          [key]: {
            status: data.status,
            bytes: data.bytes,
            bytesTotal: data.bytes_total,
            mbPerSecond: data.mb_per_second,
            size: data.size ?? prev[key]?.size,
            sha256: data.sha256 ?? prev[key]?.sha256,
          },
        }));
      },
      (errMsg: string) => {
        setPullStates((prev) => ({
          ...prev,
          [key]: { ...(prev[key] || { status: 'error' }), status: 'error', error: errMsg },
        }));
        cancelRefs.current[key] = null;
      },
      () => {
        setPullStates((prev) => ({
          ...prev,
          [key]: { ...(prev[key] || { status: 'complete' }), status: 'complete' },
        }));
        cancelRefs.current[key] = null;
        onChanged();
      },
    );
  };

  const handleCancel = (b: LibsPeerBundle) => {
    const key = bundleKey(b);
    const cancel = cancelRefs.current[key];
    if (cancel) {
      cancel();
      cancelRefs.current[key] = null;
    }
    setPullStates((prev) => ({
      ...prev,
      [key]: { ...(prev[key] || { status: 'cancelled' }), status: 'cancelled' },
    }));
  };

  return (
    <div className="card" style={{ marginTop: 24 }}>
      <h3 style={{ marginTop: 0, marginBottom: 8 }}>Pull Bundles From Network Peer</h3>
      <p style={{ marginBottom: 16, color: 'var(--color-gray-600)', fontSize: 14 }}>
        Download library bundles from another Kronk server on the local network instead of
        from the upstream llama.cpp release. The peer must be running with the download
        endpoint enabled. Useful in workshops where Internet access is slow or unreliable.
      </p>

      <div style={{ display: 'flex', gap: 12, alignItems: 'flex-end', flexWrap: 'wrap', marginBottom: 16 }}>
        <div className="form-group" style={{ marginBottom: 0, minWidth: 240 }}>
          <FieldLabel tooltipKey="peerLibsHost" htmlFor="peer-libs-host">Peer Server</FieldLabel>
          <input
            id="peer-libs-host"
            type="text"
            value={host}
            onChange={(e) => updateHost(e.target.value)}
            disabled={connecting}
            placeholder="192.168.0.246:11435"
          />
        </div>
        <button
          className="btn btn-primary"
          onClick={handleConnect}
          disabled={connecting || !host.trim()}
        >
          {connecting ? 'Connecting…' : 'Connect'}
          <ParamTooltip tooltipKey="peerLibsConnect" />
        </button>
      </div>

      {error && (
        <div className="status-box" style={{ marginBottom: 16 }}>
          <div className="status-line error">{error}</div>
        </div>
      )}

      {bundles !== null && bundles.length === 0 && (
        <p style={{ color: 'var(--color-gray-600)', fontSize: 14 }}>
          The peer reported no installed bundles.
        </p>
      )}

      {bundles !== null && bundles.length > 0 && (
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr>
              <th style={{ textAlign: 'left', padding: 8 }}>OS</th>
              <th style={{ textAlign: 'left', padding: 8 }}>Architecture</th>
              <th style={{ textAlign: 'left', padding: 8 }}>Processor</th>
              <th style={{ textAlign: 'left', padding: 8 }}>Version</th>
              <th style={{ textAlign: 'left', padding: 8 }}>Size</th>
              <th style={{ textAlign: 'left', padding: 8 }}>Status</th>
              <th style={{ textAlign: 'right', padding: 8 }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {bundles.map((b) => {
              const key = bundleKey(b);
              const state = pullStates[key];
              const isInstalled = installedKeys.has(key);
              const inFlight = state && state.status !== 'complete' && state.status !== 'error' && state.status !== 'cancelled';
              const total = state?.bytesTotal || state?.size || b.size || 0;
              const cur = state?.bytes || 0;
              const pct = total > 0 ? Math.min(100, Math.round((cur / total) * 100)) : 0;

              return (
                <tr key={key}>
                  <td style={{ padding: 8 }}>{b.os}</td>
                  <td style={{ padding: 8 }}>{b.arch}</td>
                  <td style={{ padding: 8 }}>{b.processor}</td>
                  <td style={{ padding: 8 }}>{b.version}</td>
                  <td style={{ padding: 8 }}>{formatBytes(b.size || total)}</td>
                  <td style={{ padding: 8, fontSize: 13 }}>
                    {state ? (
                      <div>
                        <div>
                          {state.status}
                          {state.status === 'downloading' && total > 0 && (
                            <> — {formatBytes(cur)} / {formatBytes(total)} ({pct}%) {state.mbPerSecond ? `@ ${state.mbPerSecond.toFixed(1)} MB/s` : ''}</>
                          )}
                        </div>
                        {state.status === 'downloading' && total > 0 && (
                          <div style={{ marginTop: 4, height: 6, background: 'var(--color-gray-200)', borderRadius: 3, overflow: 'hidden' }}>
                            <div style={{ width: `${pct}%`, height: '100%', background: 'var(--color-primary, #2563eb)', transition: 'width 200ms linear' }} />
                          </div>
                        )}
                        {state.error && (
                          <div className="status-line error" style={{ marginTop: 4 }}>{state.error}</div>
                        )}
                      </div>
                    ) : isInstalled ? (
                      <span style={{ color: 'var(--color-gray-600)' }}>installed</span>
                    ) : (
                      <span style={{ color: 'var(--color-gray-600)' }}>—</span>
                    )}
                  </td>
                  <td style={{ padding: 8, textAlign: 'right' }}>
                    {inFlight ? (
                      <button className="btn btn-danger" onClick={() => handleCancel(b)}>Cancel</button>
                    ) : (
                      <button
                        className="btn btn-primary"
                        onClick={() => handleDownload(b)}
                      >
                        {isInstalled ? 'Re-download' : 'Download'}
                        <ParamTooltip tooltipKey="peerLibsDownload" />
                      </button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}
