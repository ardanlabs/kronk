import { useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../services/api';
import { useToken } from '../contexts/TokenContext';
import { RateLimit, RateWindow } from '../types';

const AVAILABLE_ENDPOINTS = [
  { label: '/v1/chat/completions', value: 'chat-completions' },
  { label: '/v1/embeddings', value: 'embeddings' },
];

const RATE_WINDOWS: { label: string; value: RateWindow }[] = [
  { label: 'Unlimited', value: 'unlimited' },
  { label: 'Per Day', value: 'day' },
  { label: 'Per Month', value: 'month' },
  { label: 'Per Year', value: 'year' },
];

interface EndpointConfig {
  enabled: boolean;
  limit: number;
  window: RateWindow;
}

type EndpointConfigs = Record<string, EndpointConfig>;

const defaultEndpointConfig = (): EndpointConfig => ({
  enabled: false,
  limit: 1000,
  window: 'unlimited',
});

export default function SecurityTokenCreate() {
  const { token: storedToken } = useToken();
  const [isAdmin, setIsAdmin] = useState(false);
  const [endpointConfigs, setEndpointConfigs] = useState<EndpointConfigs>(() => {
    const configs: EndpointConfigs = {};
    AVAILABLE_ENDPOINTS.forEach((ep) => {
      configs[ep.value] = defaultEndpointConfig();
    });
    return configs;
  });
  const [duration, setDuration] = useState('24');
  const [durationUnit, setDurationUnit] = useState<'h' | 'd' | 'M' | 'y'>('h');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [newToken, setNewToken] = useState<string | null>(null);

  const updateEndpointConfig = (
    endpoint: string,
    updates: Partial<EndpointConfig>
  ) => {
    setEndpointConfigs((prev) => ({
      ...prev,
      [endpoint]: { ...prev[endpoint], ...updates },
    }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!storedToken) return;

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

    const endpoints: Record<string, RateLimit> = {};
    Object.entries(endpointConfigs).forEach(([name, config]) => {
      if (config.enabled) {
        endpoints[name] = {
          limit: config.window === 'unlimited' ? 0 : config.limit,
          window: config.window,
        };
      }
    });

    try {
      const response = await api.createToken(storedToken, {
        admin: isAdmin,
        endpoints,
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

      {!storedToken && (
        <div className="alert alert-error">
          No API token configured. <Link to="/settings">Configure your token in Settings</Link>
        </div>
      )}

      {storedToken && (
        <div className="card">
          <form onSubmit={handleSubmit}>
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
              <label>Endpoints &amp; Rate Limits</label>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {AVAILABLE_ENDPOINTS.map((endpoint) => {
                  const config = endpointConfigs[endpoint.value];
                  return (
                    <div
                      key={endpoint.value}
                      style={{
                        padding: '12px',
                        background: config.enabled
                          ? 'rgba(240, 181, 49, 0.1)'
                          : 'var(--color-gray-100)',
                        borderRadius: '6px',
                        border: config.enabled
                          ? '1px solid rgba(240, 181, 49, 0.3)'
                          : '1px solid transparent',
                      }}
                    >
                      <label
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          cursor: 'pointer',
                          fontWeight: 500,
                        }}
                      >
                        <input
                          type="checkbox"
                          checked={config.enabled}
                          onChange={(e) =>
                            updateEndpointConfig(endpoint.value, {
                              enabled: e.target.checked,
                            })
                          }
                          style={{ marginRight: '8px' }}
                        />
                        {endpoint.label}
                      </label>

                      {config.enabled && (
                        <div
                          style={{
                            display: 'flex',
                            gap: '12px',
                            marginTop: '10px',
                            paddingLeft: '24px',
                          }}
                        >
                          <div style={{ flex: 1 }}>
                            <label
                              style={{
                                fontSize: '12px',
                                color: 'var(--color-gray-600)',
                                display: 'block',
                                marginBottom: '4px',
                              }}
                            >
                              Rate Limit
                            </label>
                            <select
                              value={config.window}
                              onChange={(e) =>
                                updateEndpointConfig(endpoint.value, {
                                  window: e.target.value as RateWindow,
                                })
                              }
                              style={{ width: '100%' }}
                            >
                              {RATE_WINDOWS.map((w) => (
                                <option key={w.value} value={w.value}>
                                  {w.label}
                                </option>
                              ))}
                            </select>
                          </div>

                          {config.window !== 'unlimited' && (
                            <div style={{ flex: 1 }}>
                              <label
                                style={{
                                  fontSize: '12px',
                                  color: 'var(--color-gray-600)',
                                  display: 'block',
                                  marginBottom: '4px',
                                }}
                              >
                                Max Requests
                              </label>
                              <input
                                type="number"
                                value={config.limit}
                                onChange={(e) =>
                                  updateEndpointConfig(endpoint.value, {
                                    limit: parseInt(e.target.value) || 0,
                                  })
                                }
                                min="1"
                                style={{ width: '100%' }}
                              />
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  );
                })}
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
              disabled={loading}
            >
              {loading ? 'Creating...' : 'Create Token'}
            </button>
          </form>
        </div>
      )}

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
