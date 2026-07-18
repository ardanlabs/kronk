import { useState } from 'react';
import { api } from '../services/api';

export default function SecurityKeyDelete() {
  const [keyId, setKeyId] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!keyId.trim()) return;

    setLoading(true);
    setError(null);
    setSuccess(null);
    try {
      await api.deleteKey(keyId.trim());
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
        <p>Remove a security key using your admin session</p>
      </div>

        <div className="card">
          {error && <div className="alert alert-error">{error}</div>}
          {success && <div className="alert alert-success">{success}</div>}

          <form onSubmit={handleSubmit}>
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
              disabled={loading || !keyId.trim()}
            >
              {loading ? 'Deleting...' : 'Delete Key'}
            </button>
          </form>
        </div>
    </div>
  );
}
