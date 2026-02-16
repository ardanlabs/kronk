import { useState, useRef, useEffect } from 'react';
import { api } from '../services/api';
import type { VersionResponse } from '../types';

export default function LibsPull() {
  const [pulling, setPulling] = useState(false);
  const [messages, setMessages] = useState<Array<{ text: string; type: 'info' | 'error' | 'success' }>>([]);
  const [versionInfo, setVersionInfo] = useState<VersionResponse | null>(null);
  const [loadingVersion, setLoadingVersion] = useState(true);
  const [overrideUpgrade, setOverrideUpgrade] = useState(false);
  const [version, setVersion] = useState('');
  const closeRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    api
      .getLibsVersion()
      .then(setVersionInfo)
      .catch(() => {})
      .finally(() => setLoadingVersion(false));
  }, []);

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
      useAllowUpgrade,
      version || undefined
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
        <h2>Pull/Update Libs</h2>
        <p>Download or update the Kronk libraries</p>
      </div>

      <div className="card">
        {loadingVersion ? (
          <p>Loading version info...</p>
        ) : versionInfo ? (
          <div style={{ marginBottom: '24px' }}>
            <h4 style={{ marginBottom: '12px' }}>Current Version</h4>
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
          </div>
        )}

        <div style={{ marginBottom: '16px' }}>
          <label htmlFor="version" style={{ fontSize: '14px', display: 'block', marginBottom: '4px' }}>
            Version (leave empty for latest)
          </label>
          <input
            type="text"
            id="version"
            value={version}
            onChange={(e) => setVersion(e.target.value)}
            disabled={pulling}
            placeholder="e.g. b5540"
            style={{ width: '200px', padding: '6px 8px', fontSize: '14px' }}
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
    </div>
  );
}
