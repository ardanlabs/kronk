import { useState } from 'react';

export default function Login({ onAuthenticated }: { onAuthenticated: () => void }) {
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    setError('');
    try {
      const response = await fetch('/admin/api/login', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password }),
      });
      if (response.ok) {
        onAuthenticated();
        return;
      }
      setError(response.status === 401 ? 'Invalid password.' : 'Unable to sign in. Please try again.');
    } catch {
      setError('Unable to reach the server. Please try again.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="auth-screen">
      <form className="auth-card" onSubmit={submit}>
        <img src="/admin/kronk-logo.png" alt="Kronk" />
        <h1>Kronk Admin</h1>
        <p>Sign in to manage this model server.</p>
        <label htmlFor="admin-password">Admin password</label>
        <input id="admin-password" type="password" autoComplete="current-password" autoFocus required value={password} onChange={(event) => setPassword(event.target.value)} />
        {error && <div className="alert alert-error" role="alert">{error}</div>}
        <button className="btn btn-primary" type="submit" disabled={submitting || !password}>
          {submitting ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
    </div>
  );
}
