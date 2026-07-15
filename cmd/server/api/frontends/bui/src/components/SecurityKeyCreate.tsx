import { useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../services/api';
import { useToken } from '../contexts/TokenContext';

export default function SecurityKeyCreate() {
  const { token: storedToken } = useToken();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!storedToken) return;

    setLoading(true);
    setError(null);
    setCreated(false);
    try {
      await api.createKey(storedToken);
      setCreated(true);
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

      {!storedToken && (
        <div className="alert alert-error">
          No API token configured. <Link to="/settings">Configure your token in Settings</Link>
        </div>
      )}

      {storedToken && (
        <div className="card">
          <form onSubmit={handleSubmit}>
            <button className="btn btn-primary" type="submit" disabled={loading}>
              {loading ? 'Creating...' : 'Create Key'}
            </button>
          </form>
        </div>
      )}

      {error && <div className="alert alert-error">{error}</div>}

      {created && <div className="alert alert-success">Key created successfully!</div>}
    </div>
  );
}
