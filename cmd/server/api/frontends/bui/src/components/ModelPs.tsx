import { useState, useEffect } from 'react';
import { api } from '../services/api';
import type { ModelDetailsResponse, PoolBudgetResponse } from '../types';
import { formatBytes } from '../lib/format';
import { labelWithTip, ParamTooltip } from './ParamTooltips';

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString();
}

function pct(used: number, budget: number): string {
  if (budget <= 0) return '0%';
  return `${((used / budget) * 100).toFixed(1)}%`;
}

export default function ModelPs() {
  const [data, setData] = useState<ModelDetailsResponse | null>(null);
  const [budget, setBudget] = useState<PoolBudgetResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [unloading, setUnloading] = useState<string | null>(null);

  useEffect(() => {
    loadAll();
  }, []);

  const loadAll = async () => {
    setLoading(true);
    setError(null);
    try {
      const [modelsResp, budgetResp] = await Promise.all([
        api.listRunningModels(),
        api.getPoolBudget(),
      ]);
      setData(modelsResp);
      setBudget(budgetResp);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load running models');
    } finally {
      setLoading(false);
    }
  };

  const handleUnload = async (modelId: string) => {
    if (!confirm(`Unload model "${modelId}"?`)) return;
    setUnloading(modelId);
    setError(null);
    try {
      await api.unloadModel(modelId);
      await loadAll();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to unload model');
    } finally {
      setUnloading(null);
    }
  };

  return (
    <div>
      <div className="page-header">
        <h2>Running Models</h2>
        <p>Models currently loaded in cache and the pool's resource budget</p>
      </div>

      {budget && (
        <div className="card">
          <h3 style={{ marginTop: 0 }}>
            Resource Budget — {budget.budget_percent}% of detected hardware
            {' '}
            <ParamTooltip tooltipKey="budgetPercent" />
          </h3>
          <p style={{ marginTop: 0, marginBottom: 16, color: 'var(--text-muted, #888)' }}>
            Headroom per GPU: {formatBytes(budget.headroom_bytes)}
            {' '}
            <ParamTooltip tooltipKey="budgetHeadroom" />
          </p>

          {budget.unified_memory && (
            <p style={{ marginTop: 0, marginBottom: 16, color: 'var(--text-muted, #888)' }}>
              <em>
                Apple Silicon unified memory detected. The GPU (Metal) and CPU
                share a single physical pool, so the resman tracks just one
                budget under <strong>Unified Memory</strong> below to avoid
                double-counting the same bytes against two budgets.
              </em>
            </p>
          )}

          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Device</th>
                  <th>Type</th>
                  <th style={{ textAlign: 'right' }}>{labelWithTip('Total', 'budgetDeviceTotal')}</th>
                  <th style={{ textAlign: 'right' }}>{labelWithTip('Budget', 'budgetDeviceBudget')}</th>
                  <th style={{ textAlign: 'right' }}>{labelWithTip('Used', 'budgetDeviceUsed')}</th>
                  <th style={{ textAlign: 'right' }}>{labelWithTip('% of Budget', 'budgetDevicePctOfBudget')}</th>
                  <th style={{ textAlign: 'right' }}>{labelWithTip('Free in Budget', 'budgetDeviceFree')}</th>
                </tr>
              </thead>
              <tbody>
                {budget.devices.map((d) => (
                  <tr key={`${d.index}-${d.name}`}>
                    <td>{d.name}</td>
                    <td>{d.type}</td>
                    <td style={{ textAlign: 'right' }}>{formatBytes(d.total_bytes)}</td>
                    <td style={{ textAlign: 'right' }}>{formatBytes(d.budget_bytes)}</td>
                    <td style={{ textAlign: 'right' }}>{formatBytes(d.used_bytes)}</td>
                    <td style={{ textAlign: 'right' }}>{pct(d.used_bytes, d.budget_bytes)}</td>
                    <td style={{ textAlign: 'right' }}>{formatBytes(Math.max(d.budget_bytes - d.used_bytes, 0))}</td>
                  </tr>
                ))}
                <tr>
                  <td>{budget.unified_memory ? 'Unified Memory' : 'System RAM'}</td>
                  <td>{budget.unified_memory ? 'metal+ram' : 'ram'}</td>
                  <td style={{ textAlign: 'right' }}>{budget.ram_total > 0 ? formatBytes(budget.ram_total) : '—'}</td>
                  <td style={{ textAlign: 'right' }}>{formatBytes(budget.ram_budget)}</td>
                  <td style={{ textAlign: 'right' }}>{formatBytes(budget.ram_used)}</td>
                  <td style={{ textAlign: 'right' }}>{pct(budget.ram_used, budget.ram_budget)}</td>
                  <td style={{ textAlign: 'right' }}>{formatBytes(Math.max(budget.ram_budget - budget.ram_used, 0))}</td>
                </tr>
              </tbody>
            </table>
          </div>

          {budget.reservations.length > 0 && (
            <>
              <h4 style={{ marginTop: 24, marginBottom: 12 }}>
                Active Reservations ({budget.reservations.length})
              </h4>
              <div className="table-container">
                <table>
                  <thead>
                    <tr>
                      <th>{labelWithTip('Key', 'budgetReservationKey')}</th>
                      <th style={{ textAlign: 'right' }}>{labelWithTip('Total', 'budgetReservationTotal')}</th>
                      <th style={{ textAlign: 'right' }}>{labelWithTip('VRAM', 'budgetReservationVRAM')}</th>
                      <th style={{ textAlign: 'right' }}>{labelWithTip('System', 'budgetReservationSystem')}</th>
                      <th>{labelWithTip('Per-Device', 'budgetReservationPerDevice')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {budget.reservations.map((r) => (
                      <tr key={r.key}>
                        <td style={{ whiteSpace: 'nowrap' }}>{r.key}</td>
                        <td style={{ textAlign: 'right', fontWeight: 'bold', whiteSpace: 'nowrap' }}>
                          {formatBytes(r.vram_bytes + r.ram_bytes)}
                        </td>
                        <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>{formatBytes(r.vram_bytes)}</td>
                        <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>{formatBytes(r.ram_bytes)}</td>
                        <td>
                          {r.per.length === 0
                            ? '—'
                            : r.per.map((p) => `${p.name}: ${formatBytes(p.bytes)}`).join(', ')}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </div>
      )}

      <div className="card">
        {loading && <div className="loading">Loading running models</div>}

        {error && <div className="alert alert-error">{error}</div>}

        {!loading && !error && data && (
          <div className="table-container">
            {data.length > 0 ? (
              <table>
                <thead>
                  <tr>
                    <th>{labelWithTip('ID', 'runningModelID')}</th>
                    <th>{labelWithTip('Owner', 'runningModelOwner')}</th>
                    <th>{labelWithTip('Family', 'runningModelFamily')}</th>
                    <th style={{ textAlign: 'right' }}>{labelWithTip('Size', 'runningModelSize')}</th>
                    <th style={{ textAlign: 'right' }}>{labelWithTip('VRAM Total', 'runningModelVRAMTotal')}</th>
                    <th style={{ textAlign: 'right' }}>{labelWithTip('KV Cache', 'runningModelKVCache')}</th>
                    <th style={{ textAlign: 'right' }}>{labelWithTip('Slots', 'runningModelSlots')}</th>
                    <th>{labelWithTip('Expires At', 'runningModelExpiresAt')}</th>
                    <th>{labelWithTip('Active Streams', 'runningModelActiveStreams')}</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {data.map((model) => (
                    <tr key={model.id}>
                      <td style={{ whiteSpace: 'nowrap' }}>{model.id}</td>
                      <td>{model.owned_by}</td>
                      <td style={{ whiteSpace: 'nowrap' }}>{model.model_family}</td>
                      <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>{formatBytes(model.size)}</td>
                      <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>{formatBytes(model.vram_total)}</td>
                      <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>{formatBytes(model.kv_cache)}</td>
                      <td style={{ textAlign: 'right' }}>{model.slots}</td>
                      <td style={{ whiteSpace: 'nowrap' }}>{formatDate(model.expires_at)}</td>
                      <td>{model.active_streams}</td>
                      <td>
                        <button
                          className="btn btn-danger btn-sm"
                          onClick={() => handleUnload(model.id)}
                          disabled={unloading === model.id || model.active_streams > 0}
                          title={model.active_streams > 0 ? 'Cannot unload while streams are active' : 'Unload model from cache'}
                        >
                          {unloading === model.id ? 'Unloading…' : 'Unload'}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
                {data.length > 1 && (
                  <tfoot>
                    <tr>
                      <td colSpan={4} style={{ textAlign: 'right', fontWeight: 'bold' }}>Total:</td>
                      <td style={{ textAlign: 'right', fontWeight: 'bold' }}>{formatBytes(data.reduce((sum, m) => sum + m.vram_total, 0))}</td>
                      <td style={{ textAlign: 'right', fontWeight: 'bold' }}>{formatBytes(data.reduce((sum, m) => sum + m.kv_cache, 0))}</td>
                      <td style={{ textAlign: 'right', fontWeight: 'bold' }}>{data.reduce((sum, m) => sum + m.slots, 0)}</td>
                      <td colSpan={3}></td>
                    </tr>
                  </tfoot>
                )}
              </table>
            ) : (
              <div className="empty-state">
                <h3>No running models</h3>
                <p>Models will appear here when loaded into cache</p>
              </div>
            )}
          </div>
        )}

        <div style={{ marginTop: '16px' }}>
          <button className="btn btn-secondary" onClick={loadAll} disabled={loading}>
            Refresh
          </button>
        </div>
      </div>
    </div>
  );
}
