import { useState } from 'react';
import { useToken } from '../contexts/TokenContext';

export default function Settings() {
  const { token, setToken, clearToken, hasToken } = useToken();
  const [inputToken, setInputToken] = useState(token);
  const [showToken, setShowToken] = useState(false);
  const [saved, setSaved] = useState(false);

  const handleSave = (e: React.FormEvent) => {
    e.preventDefault();
    setToken(inputToken.trim());
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  const handleClear = () => {
    clearToken();
    setInputToken('');
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  return (
    <div>
      <div className="page-header">
        <h2>Settings</h2>
        <p>Configure your API token for authenticated requests</p>
      </div>

      <div className="card">
        <form onSubmit={handleSave}>
          <div className="form-group">
            <label htmlFor="apiToken">API Token</label>
            <div style={{ display: 'flex', gap: '8px' }}>
              <input
                type={showToken ? 'text' : 'password'}
                id="apiToken"
                value={inputToken}
                onChange={(e) => setInputToken(e.target.value)}
                placeholder="Enter your KRONK_TOKEN"
                style={{ flex: 1 }}
              />
              <button
                type="button"
                className="btn btn-secondary"
                onClick={() => setShowToken(!showToken)}
              >
                {showToken ? 'Hide' : 'Show'}
              </button>
            </div>
            <p style={{ fontSize: '12px', color: 'var(--color-gray-600)', marginTop: '8px' }}>
              This token will be stored in your browser and used for all API requests that require authentication.
            </p>
          </div>
          <div style={{ display: 'flex', gap: '12px' }}>
            <button className="btn btn-primary" type="submit">
              Save Token
            </button>
            {hasToken && (
              <button className="btn btn-danger" type="button" onClick={handleClear}>
                Clear Token
              </button>
            )}
          </div>
        </form>
      </div>

      {saved && <div className="alert alert-success">Token settings saved</div>}

      <div className="card">
        <h4 style={{ marginBottom: '12px', color: 'var(--color-page-title)' }}>Token Status</h4>
        <p style={{ color: hasToken ? 'var(--color-success)' : 'var(--color-gray-600)' }}>
          {hasToken ? '✓ Token is configured' : '○ No token configured'}
        </p>
      </div>
    </div>
  );
}
