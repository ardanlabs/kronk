import { useState } from 'react';
import { api } from '../services/api';

export default function SecurityKeyCreate() {
  const [token, setToken] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [newKeyId, setNewKeyId] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token.trim()) return;

    setLoading(true);
    setError(null);
    setNewKeyId(null);
    try {
      const response = await api.createKey(token.trim());
      setNewKeyId(response.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create key');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <div className="page-header">
        <h2>Create Security Key</h2>
        <p>Generate a new security key (requires admin token)</p>
      </div>

      <div className="card">
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label htmlFor="adminToken">Admin Token</label>
            <input
              type="password"
              id="adminToken"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="Enter admin token (KRONK_TOKEN)"
            />
          </div>
          <button className="btn btn-primary" type="submit" disabled={loading || !token.trim()}>
            {loading ? 'Creating...' : 'Create Key'}
          </button>
        </form>
      </div>

      {error && <div className="alert alert-error">{error}</div>}

      {newKeyId && (
        <div className="card">
          <div className="alert alert-success">Key created successfully!</div>
          <div style={{ marginTop: '12px' }}>
            <label style={{ fontWeight: 500, display: 'block', marginBottom: '8px' }}>
              New Key ID
            </label>
            <div className="token-display">{newKeyId}</div>
            <p style={{ marginTop: '8px', fontSize: '13px', color: 'var(--color-gray-600)' }}>
              Store this key securely. It will not be shown again.
            </p>
          </div>
        </div>
      )}
    </div>
  );
}
