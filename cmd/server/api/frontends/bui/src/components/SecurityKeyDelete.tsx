import { useState } from 'react';
import { api } from '../services/api';

export default function SecurityKeyDelete() {
  const [token, setToken] = useState('');
  const [keyId, setKeyId] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token.trim() || !keyId.trim()) return;

    setLoading(true);
    setError(null);
    setSuccess(null);
    try {
      await api.deleteKey(token.trim(), keyId.trim());
      setSuccess(`Key "${keyId}" deleted successfully`);
      setKeyId('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete key');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <div className="page-header">
        <h2>Delete Security Key</h2>
        <p>Remove a security key (requires admin token)</p>
      </div>

      <div className="card">
        {error && <div className="alert alert-error">{error}</div>}
        {success && <div className="alert alert-success">{success}</div>}

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
          <div className="form-group">
            <label htmlFor="keyId">Key ID</label>
            <input
              type="text"
              id="keyId"
              value={keyId}
              onChange={(e) => setKeyId(e.target.value)}
              placeholder="Enter key ID to delete"
            />
          </div>
          <button
            className="btn btn-danger"
            type="submit"
            disabled={loading || !token.trim() || !keyId.trim()}
          >
            {loading ? 'Deleting...' : 'Delete Key'}
          </button>
        </form>
      </div>
    </div>
  );
}
