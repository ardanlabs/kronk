import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react';
import Login from '../components/Login';
import { SESSION_EXPIRED_EVENT } from '../services/api';

interface AuthContextType {
  authenticationRequired: boolean;
  logout: () => Promise<void>;
}

interface SessionResponse {
  authenticated: boolean;
  authentication_required: boolean;
}

const AuthContext = createContext<AuthContextType | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<'loading' | 'authenticated' | 'anonymous'>('loading');
  const [authenticationRequired, setAuthenticationRequired] = useState(true);

  const checkSession = useCallback(async () => {
    try {
      const response = await fetch('/admin/api/session', { credentials: 'same-origin' });
      const session = await response.json() as SessionResponse;
      setAuthenticationRequired(session.authentication_required);
      setStatus(response.ok && session.authenticated ? 'authenticated' : 'anonymous');
    } catch {
      setStatus('anonymous');
    }
  }, []);

  useEffect(() => {
    localStorage.removeItem('kronk_token');
    void checkSession();
    const expire = () => setStatus('anonymous');
    window.addEventListener(SESSION_EXPIRED_EVENT, expire);
    return () => window.removeEventListener(SESSION_EXPIRED_EVENT, expire);
  }, [checkSession]);

  const logout = useCallback(async () => {
    try {
      await fetch('/admin/api/logout', { method: 'POST', credentials: 'same-origin' });
    } finally {
      setStatus('anonymous');
    }
  }, []);

  if (status === 'loading') {
    return <div className="auth-screen"><div className="auth-card auth-loading">Loading Kronk…</div></div>;
  }

  if (status === 'anonymous') {
    return <Login onAuthenticated={() => setStatus('authenticated')} />;
  }

  return <AuthContext.Provider value={{ authenticationRequired, logout }}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) throw new Error('useAuth must be used within an AuthProvider');
  return context;
}
