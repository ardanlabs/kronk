import { useState, useEffect } from 'react';
import { api } from '../services/api';
import type { DiagnoseResponse, DiagnoseCommand } from '../types';
import { formatBytes } from '../lib/format';
import CodeBlock from './CodeBlock';

type ViewMode = 'summary' | 'json';

// BENCH_MODEL mirrors defaultModelSource in sdk/tools/diagnose/diagnose.go — the
// standard model the benchmark runs so throughput is comparable across machines.
const BENCH_MODEL = 'unsloth/Qwen3-0.6B-Q8_0';

function formatMiB(mib: number): string {
  if (!mib) return '—';
  return formatBytes(mib * 1024 * 1024);
}

function CommandOutput({ commands, open }: { commands?: DiagnoseCommand[]; open?: boolean }) {
  if (!commands || commands.length === 0) return null;

  return (
    <div style={{ marginTop: 12 }}>
      {commands.map((c, i) => (
        <details key={`${c.cmd}-${i}`} open={open} style={{ marginBottom: 12 }}>
          <summary
            style={{
              cursor: 'pointer',
              fontFamily: '"SF Mono", "Monaco", "Inconsolata", "Fira Code", monospace',
              fontSize: '14px',
              color: 'var(--color-gray-900)',
              padding: '4px 0',
            }}
          >
            $ {c.cmd}
          </summary>
          {c.err && (
            <p style={{ color: 'var(--color-error)', margin: '8px 0 0', fontSize: '13px' }}>
              error: {c.err}
            </p>
          )}
          <pre className="code-block" style={{ marginTop: 8, whiteSpace: 'pre' }}>
            {c.output || '(no output)'}
          </pre>
        </details>
      ))}
    </div>
  );
}

export default function Diagnose() {
  const [data, setData] = useState<DiagnoseResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [view, setView] = useState<ViewMode>('summary');
  const [benchmarking, setBenchmarking] = useState(false);
  const [benchRan, setBenchRan] = useState(false);
  const [benchProcessor, setBenchProcessor] = useState('');

  useEffect(() => {
    load();
  }, []);

  const load = async () => {
    setLoading(true);
    setError(null);
    setBenchRan(false);
    try {
      const resp = await api.getDiagnose();
      setData(resp);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load diagnostics');
    } finally {
      setLoading(false);
    }
  };

  // runBenchmark re-collects the report with the benchmark enabled. It is a
  // separate, explicit action because the benchmark loads a model and runs
  // llama-bench, which takes several seconds — too slow for the initial load.
  const runBenchmark = async () => {
    setBenchmarking(true);
    setError(null);
    try {
      const resp = await api.getDiagnose(true, benchProcessor);
      setData(resp);
      setBenchRan(true);
      setView('summary');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to run benchmark');
    } finally {
      setBenchmarking(false);
    }
  };

  const reportJSON = data ? JSON.stringify(data, null, 2) : '';

  // benchProcessorOptions lists the processors the user can benchmark: every
  // installed backend plus cpu (always selectable, since any bundle can run
  // CPU-only via -ngl 0 — this is how a GPU-only install still benchmarks cpu).
  const benchProcessorOptions = Array.from(
    new Set([...(data?.llama.backends ?? []).map((b) => b.processor), 'cpu'])
  );

  return (
    <div>
      <div className="page-header-with-action">
        <div>
          <h2>Info</h2>
          <p className="page-description">
            Host environment diagnostics — versions, system, and the llama.cpp
            backends and devices Kronk can see. Use <strong>Run benchmark</strong>{' '}
            to measure inference throughput; it loads a model and takes a few
            seconds.
          </p>
        </div>
        <button className="btn btn-primary" onClick={load} disabled={loading || benchmarking}>
          Refresh
        </button>
      </div>

      {data && (
        <div className="tabs">
          <button
            className={`tab ${view === 'summary' ? 'active' : ''}`}
            onClick={() => setView('summary')}
          >
            Summary
          </button>
          <button
            className={`tab ${view === 'json' ? 'active' : ''}`}
            onClick={() => setView('json')}
          >
            JSON
          </button>
        </div>
      )}

      {error && <div className="error-message">{error}</div>}
      {loading && !data && <p>Collecting diagnostics…</p>}

      {data && view === 'json' && (
        <div className="diagnose-json">
          <CodeBlock code={reportJSON} language="json" />
        </div>
      )}

      {data && view === 'summary' && (
        <>
          {data.hints && data.hints.length > 0 && (
            <div className="card" style={{ borderLeft: '4px solid var(--color-warning-border)' }}>
              <h3 style={{ marginTop: 0 }}>Hints</h3>
              {data.hints.map((h, i) => (
                <div key={i} style={{ marginBottom: 12 }}>
                  <p style={{ margin: 0 }}>
                    <strong style={{ textTransform: 'uppercase' }}>[{h.severity}]</strong>{' '}
                    {h.message}
                  </p>
                  {h.remedy && (
                    <pre className="code-block" style={{ marginTop: 6, whiteSpace: 'pre-wrap' }}>
                      {h.remedy}
                    </pre>
                  )}
                </div>
              ))}
            </div>
          )}

          <div className="card">
            <h3 style={{ marginTop: 0 }}>Versions</h3>
            <div className="table-container">
              <table>
                <tbody>
                  <tr>
                    <td>Kronk</td>
                    <td>{data.versions.kronk || '—'}</td>
                  </tr>
                  <tr>
                    <td>yzma</td>
                    <td>{data.versions.yzma || '—'}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div className="card">
            <h3 style={{ marginTop: 0 }}>System</h3>
            <div className="table-container">
              <table>
                <tbody>
                  <tr>
                    <td>OS</td>
                    <td>{data.system.os}</td>
                  </tr>
                  <tr>
                    <td>Arch</td>
                    <td>{data.system.arch}</td>
                  </tr>
                  <tr>
                    <td>CPU</td>
                    <td>{data.system.cpuModel || '—'}</td>
                  </tr>
                  <tr>
                    <td>CPUs</td>
                    <td>{data.system.numCPU}</td>
                  </tr>
                  <tr>
                    <td>RAM</td>
                    <td>{data.system.ramBytes ? formatBytes(data.system.ramBytes) : '—'}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <CommandOutput commands={data.system.commands} />
          </div>

          <div className="card">
            <h3 style={{ marginTop: 0 }}>llama.cpp</h3>
            {!data.llama.installed ? (
              <p style={{ color: 'var(--color-text-secondary)' }}>
                No llama.cpp libraries installed for this machine.
              </p>
            ) : (
              <>
                <p style={{ marginTop: 0, color: 'var(--color-text-secondary)' }}>
                  Root: <code>{data.llama.root}</code>
                </p>
                {(data.llama.backends ?? []).map((b) => (
                  <div key={b.processor} style={{ marginBottom: 24 }}>
                    <h4 style={{ marginBottom: 8 }}>{b.processor}</h4>
                    <div className="table-container" style={{ marginBottom: 12 }}>
                      <table>
                        <tbody>
                          <tr>
                            <td>binDir</td>
                            <td><code>{b.binDir}</code></td>
                          </tr>
                          <tr>
                            <td>version</td>
                            <td>{b.version || '—'}</td>
                          </tr>
                          <tr>
                            <td>build</td>
                            <td>{b.build || '—'}</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                    {(b.devices ?? []).length === 0 ? (
                      <p style={{ color: 'var(--color-text-secondary)', margin: '4px 0' }}>
                        No devices detected.
                      </p>
                    ) : (
                      <div className="table-container">
                        <table>
                          <thead>
                            <tr>
                              <th>ID</th>
                              <th>Device</th>
                              <th style={{ textAlign: 'right' }}>VRAM Total</th>
                              <th style={{ textAlign: 'right' }}>VRAM Free</th>
                            </tr>
                          </thead>
                          <tbody>
                            {(b.devices ?? []).map((d) => (
                              <tr key={d.id}>
                                <td>{d.id}</td>
                                <td>{d.name}</td>
                                <td style={{ textAlign: 'right' }}>{formatMiB(d.vramTotalMiB)}</td>
                                <td style={{ textAlign: 'right' }}>{formatMiB(d.vramFreeMiB)}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
                    <CommandOutput commands={b.commands} />
                  </div>
                ))}
              </>
            )}
          </div>

          {data.engine?.probed && (
            <div
              className="card"
              style={
                data.engine.loaded
                  ? undefined
                  : { borderLeft: '4px solid var(--color-error)' }
              }
            >
              <h3 style={{ marginTop: 0 }}>Engine</h3>
              {data.engine.loaded ? (
                <p style={{ marginTop: 0, color: 'var(--color-text-secondary)' }}>
                  Loaded — Kronk can load models and run inference.
                </p>
              ) : (
                <p style={{ marginTop: 0, color: 'var(--color-error)' }}>
                  <strong>
                    Degraded — Kronk could not load its llama.cpp libraries in-process,
                    so model loading and inference are unavailable.
                  </strong>
                </p>
              )}
              <div className="table-container">
                <table>
                  <tbody>
                    <tr>
                      <td>Loaded</td>
                      <td>{data.engine.loaded ? '✓ true' : '✗ false'}</td>
                    </tr>
                    <tr>
                      <td>Processor</td>
                      <td>{data.engine.processor || '—'}</td>
                    </tr>
                    <tr>
                      <td>Library path</td>
                      <td><code>{data.engine.libPath}</code></td>
                    </tr>
                  </tbody>
                </table>
              </div>
              {data.engine.error && (
                <pre className="code-block" style={{ marginTop: 12, whiteSpace: 'pre-wrap' }}>
                  {data.engine.error}
                </pre>
              )}
            </div>
          )}

          <div className="card">
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                gap: 12,
              }}
            >
              <h3 style={{ margin: 0 }}>Benchmark</h3>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <span className="bench-processor">
                  <label htmlFor="bench-processor">Processor:</label>
                  <select
                    id="bench-processor"
                    value={benchProcessor}
                    onChange={(e) => setBenchProcessor(e.target.value)}
                    disabled={benchmarking || loading}
                  >
                    <option value="">Auto</option>
                    {benchProcessorOptions.map((p) => (
                      <option key={p} value={p}>
                        {p}
                      </option>
                    ))}
                  </select>
                </span>
                <button
                  className="btn btn-primary"
                  onClick={runBenchmark}
                  disabled={benchmarking || loading || !data.llama.installed}
                >
                  {benchmarking ? 'Running benchmark…' : 'Run benchmark'}
                </button>
              </div>
            </div>
            {benchmarking ? (
              <p style={{ color: 'var(--color-text-secondary)' }}>
                Running benchmark — loading the model and measuring throughput…
              </p>
            ) : data.bench.model ? (
              <>
                <div className="table-container">
                  <table>
                    <tbody>
                      <tr>
                        <td>Backend</td>
                        <td>{data.bench.processor || '—'}</td>
                      </tr>
                      <tr>
                        <td>Model</td>
                        <td>{data.bench.model}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <CommandOutput commands={data.bench.commands} open />
              </>
            ) : benchRan ? (
              <div style={{ color: 'var(--color-warning-text)' }}>
                <p style={{ marginTop: 0 }}>
                  Benchmark skipped — the benchmark model <code>{BENCH_MODEL}</code>{' '}
                  is not installed, so there was nothing to run. Install it from the
                  CLI, then click <strong>Run benchmark</strong> again:
                </p>
                <pre className="code-block" style={{ whiteSpace: 'pre-wrap' }}>
                  {`kronk model pull ${BENCH_MODEL}`}
                </pre>
                <p style={{ marginBottom: 0 }}>
                  Or fetch it automatically with <code>kronk diagnose --install</code>.
                </p>
              </div>
            ) : (
              <p style={{ color: 'var(--color-text-secondary)' }}>
                No benchmark run yet. Click <strong>Run benchmark</strong> to measure
                inference throughput — prompt-processing and token-generation speed.
                Requires a benchmark model to be installed.
              </p>
            )}
          </div>
        </>
      )}
    </div>
  );
}
