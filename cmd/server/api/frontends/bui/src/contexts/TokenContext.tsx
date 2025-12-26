import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from 'react';

const TOKEN_STORAGE_KEY = 'kronk_token';

interface TokenContextType {
  token: string;
  setToken: (token: string) => void;
  clearToken: () => void;
  hasToken: boolean;
}

const TokenContext = createContext<TokenContextType | null>(null);

export function TokenProvider({ children }: { children: ReactNode }) {
  const [token, setTokenState] = useState<string>(() => {
    return localStorage.getItem(TOKEN_STORAGE_KEY) || '';
  });

  useEffect(() => {
    if (token) {
      localStorage.setItem(TOKEN_STORAGE_KEY, token);
    } else {
      localStorage.removeItem(TOKEN_STORAGE_KEY);
    }
  }, [token]);

  const setToken = useCallback((newToken: string) => {
    setTokenState(newToken);
  }, []);

  const clearToken = useCallback(() => {
    setTokenState('');
  }, []);

  return (
    <TokenContext.Provider value={{ token, setToken, clearToken, hasToken: !!token }}>
      {children}
    </TokenContext.Provider>
  );
}

export function useToken() {
  const context = useContext(TokenContext);
  if (!context) {
    throw new Error('useToken must be used within a TokenProvider');
  }
  return context;
}
