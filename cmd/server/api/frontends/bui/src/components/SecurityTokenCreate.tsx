import { useState } from 'react';
import { api } from '../services/api';

const AVAILABLE_ENDPOINTS = [
  { label: '/v1/chat/completions', value: 'chat-completions' },
  { label: '/v1/embeddings', value: 'embeddings' },
];

export default function SecurityTokenCreate() {
  const [adminToken, setAdminToken] = useState('');
  const [userName, setUserName] = useState('');
  const [isAdmin, setIsAdmin] = useState(false);
  const [selectedEndpoints, setSelectedEndpoints] = useState<string[]>([]);
  const [duration, setDuration] = useState('24');
  const [durationUnit, setDurationUnit] = useState<'h' | 'd' | 'M' | 'y'>('h');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [newToken, setNewToken] = useState<string | null>(null);

  const toggleEndpoint = (value: string) => {
    setSelectedEndpoints((prev) =>
      prev.includes(value)
        ? prev.filter((e) => e !== value)
        : [...prev, value]
    );
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!adminToken.trim() || !userName.trim()) return;

    setLoading(true);
    setError(null);
    setNewToken(null);

    const durationValue = parseInt(duration);
    let durationNs: number;
    switch (durationUnit) {
      case 'h':
        durationNs = durationValue * 60 * 60 * 1e9;
        break;
      case 'd':
        durationNs = durationValue * 24 * 60 * 60 * 1e9;
        break;
      case 'M':
        durationNs = durationValue * 30 * 24 * 60 * 60 * 1e9;
        break;
      case 'y':
        durationNs = durationValue * 365 * 24 * 60 * 60 * 1e9;
        break;
    }

    try {
      const response = await api.createToken(adminToken.trim(), {
        user_name: userName.trim(),
        admin: isAdmin,
        endpoints: selectedEndpoints,
        duration: durationNs,
      });
      setNewToken(response.token);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create token');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <div className="page-header">
        <h2>Create Token</h2>
        <p>Generate a new authentication token</p>
      </div>

      <div className="card">
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label htmlFor="adminToken">Admin Token</label>
            <input
              type="password"
              id="adminToken"
              value={adminToken}
              onChange={(e) => setAdminToken(e.target.value)}
              placeholder="Enter admin token (KRONK_TOKEN)"
            />
          </div>

          <div className="form-group">
            <label htmlFor="userName">Username</label>
            <input
              type="text"
              id="userName"
              value={userName}
              onChange={(e) => setUserName(e.target.value)}
              placeholder="Enter username for the token"
            />
          </div>

          <div className="form-group">
            <label>
              <input
                type="checkbox"
                checked={isAdmin}
                onChange={(e) => setIsAdmin(e.target.checked)}
              />
              Admin privileges
            </label>
          </div>

          <div className="form-group">
            <label>Endpoints</label>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px' }}>
              {AVAILABLE_ENDPOINTS.map((endpoint) => (
                <label
                  key={endpoint.value}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    padding: '6px 12px',
                    background: selectedEndpoints.includes(endpoint.value)
                      ? 'rgba(240, 181, 49, 0.2)'
                      : 'var(--color-gray-100)',
                    borderRadius: '4px',
                    cursor: 'pointer',
                    fontSize: '13px',
                  }}
                >
                  <input
                    type="checkbox"
                    checked={selectedEndpoints.includes(endpoint.value)}
                    onChange={() => toggleEndpoint(endpoint.value)}
                    style={{ marginRight: '6px' }}
                  />
                  {endpoint.label}
                </label>
              ))}
            </div>
          </div>

          <div className="form-row">
            <div className="form-group">
              <label htmlFor="duration">Duration</label>
              <input
                type="number"
                id="duration"
                value={duration}
                onChange={(e) => setDuration(e.target.value)}
                min="1"
              />
            </div>
            <div className="form-group">
              <label htmlFor="durationUnit">Unit</label>
              <select
                id="durationUnit"
                value={durationUnit}
                onChange={(e) => setDurationUnit(e.target.value as 'h' | 'd' | 'M' | 'y')}
              >
                <option value="h">Hours</option>
                <option value="d">Days</option>
                <option value="M">Months</option>
                <option value="y">Years</option>
              </select>
            </div>
          </div>

          <button
            className="btn btn-primary"
            type="submit"
            disabled={loading || !adminToken.trim() || !userName.trim()}
          >
            {loading ? 'Creating...' : 'Create Token'}
          </button>
        </form>
      </div>

      {error && <div className="alert alert-error">{error}</div>}

      {newToken && (
        <div className="card">
          <div className="alert alert-success">Token created successfully!</div>
          <div style={{ marginTop: '12px' }}>
            <label style={{ fontWeight: 500, display: 'block', marginBottom: '8px' }}>
              Token
            </label>
            <div className="token-display">{newToken}</div>
            <p style={{ marginTop: '8px', fontSize: '13px', color: 'var(--color-gray-600)' }}>
              Store this token securely. It will not be shown again.
            </p>
          </div>
        </div>
      )}
    </div>
  );
}
