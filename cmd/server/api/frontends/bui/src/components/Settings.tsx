import { useAuth } from '../contexts/AuthContext';

export default function Settings() {
  const { authenticationRequired } = useAuth();

  return (
    <div>
      <div className="page-header">
        <h2>Session</h2>
        <p>Admin security status</p>
      </div>
      <div className="card">
        <h4 style={{ marginBottom: '12px', color: 'var(--color-page-title)' }}>Authentication</h4>
        <p style={{ color: 'var(--color-success)' }}>
          {authenticationRequired ? '✓ This browser has an authenticated admin session.' : '✓ Authentication is disabled for this server.'}
        </p>
        <p style={{ marginTop: '8px', color: 'var(--color-gray-600)' }}>
          {authenticationRequired
            ? 'Requests are authenticated securely with a server-managed cookie. Use Sign out in the sidebar to end the session.'
            : 'The browser admin and management APIs are available without credentials.'}
        </p>
      </div>
    </div>
  );
}
