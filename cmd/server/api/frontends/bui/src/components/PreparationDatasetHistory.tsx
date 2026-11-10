import { useEffect, useState } from 'react';
import { api } from '../services/api';
import type { PreparationDataset } from '../types';
import { FieldLabel } from './ParamTooltips';

export function PreparedDatasetResult({ id, version }: { id: string; version?: number }) {
  const [dataset, setDataset] = useState<PreparationDataset | null>(null);
  const [error, setError] = useState('');
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setError('');
    api.preparationDataset(id, controller.signal).then((result) => { if (!controller.signal.aborted) setDataset(result); })
      .catch((err) => { if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'Could not load saved dataset.'); });
    return () => controller.abort();
  }, [id, attempt]);
  return <section className="fine-panel" aria-label="Saved preparation dataset">
    <div className="fine-panel-heading"><div><span>Preparation complete</span><h3>{dataset?.name ?? 'Dataset saved'}</h3></div></div>
    <p className="fine-action-note">{version === undefined ? 'Dataset saved.' : `Saved as version ${version}.`} Previous versions and adapters are unchanged. Download the dataset to use it in Configure Training.</p>
    {dataset && <DatasetDownload key={`${dataset.id}:${version}`} dataset={dataset} initialVersion={version} />}
    {error && <p className="fine-error" role="alert">{error} <button className="btn btn-secondary" type="button" onClick={() => setAttempt((value) => value + 1)}>Refresh dataset</button></p>}
    {!dataset && !error && <p role="status">Loading saved dataset…</p>}
  </section>;
}

export function DatasetDownload({ dataset, initialVersion }: { dataset: PreparationDataset; initialVersion?: number }) {
  const [version, setVersion] = useState(initialVersion ?? dataset.version);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function download() {
    setBusy(true);
    setError('');
    try {
      const blob = await api.downloadPreparationDataset(dataset.id, version);
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${dataset.name.replace(/[^a-zA-Z0-9._-]/g, '_')}-v${version}.jsonl`;
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not download dataset.');
    } finally {
      setBusy(false);
    }
  }
  return <div className="fine-dataset-download">
    <div className="form-group">
      <FieldLabel htmlFor={`dataset-version-${dataset.id}`} tooltipKey="fineTuningDatasetVersion">Dataset version</FieldLabel>
      <select id={`dataset-version-${dataset.id}`} className="form-input" value={version} disabled={busy} onChange={(event) => setVersion(Number(event.target.value))}>
        {[...new Set([dataset.version, ...(dataset.versions ?? [])])].sort((a, b) => b - a).map((item) => <option key={item} value={item}>Version {item}{item === dataset.version ? ' · Latest' : ''}</option>)}
      </select>
    </div>
    <button className="btn btn-secondary" type="button" disabled={busy} onClick={() => void download()}>{busy ? 'Downloading…' : 'Download Dataset'}</button>
    {error && <p className="fine-error" role="alert">{error}</p>}
  </div>;
}

export default function PreparationDatasetHistory({ onOpen }: { onOpen: (id: string) => void }) {
  const [datasets, setDatasets] = useState<PreparationDataset[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError('');
    api.preparationDatasets(controller.signal).then((result) => { if (!controller.signal.aborted) setDatasets(result.datasets ?? []); })
      .catch((err) => { if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'Could not load datasets.'); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [attempt]);
  return <section className="fine-panel fine-history" aria-label="Prepared datasets" aria-busy={loading}>
    <div className="fine-panel-heading">
      <div><span>Preparation</span><h3>Prepared datasets</h3></div>
      <div className="fine-actions">
        <button className="btn btn-secondary" type="button" disabled={loading} onClick={() => setAttempt((value) => value + 1)}>Refresh</button>
        <button className="btn btn-primary" type="button" onClick={() => onOpen('')}>New dataset</button>
      </div>
    </div>
    {loading && <p role="status">Loading datasets…</p>}
    {error && <p className="fine-error" role="alert">{error} Select Refresh to retry.</p>}
    {!loading && !error && datasets.length === 0 && <p>No prepared datasets yet. Create a dataset and add your first document.</p>}
    <div className="fine-dataset-collection">
    {datasets.map((dataset) => <article className="fine-panel fine-dataset-card" key={`${dataset.id}:${dataset.version}`}>
      <div className="fine-panel-heading">
        <div><h3>{dataset.name}</h3><p className="fine-action-note">{dataset.documents.length} documents · {dataset.record_count} examples · Version {dataset.version}</p></div>
        <span className={`fine-status ${dataset.training_ready ? 'fine-status-completed' : 'fine-status-waiting'}`}>{dataset.training_ready ? 'Training ready' : 'Not training ready'}</span>
      </div>
      <p className="fine-action-note">Updated {new Date(dataset.updated_at).toLocaleString()}</p>
      <details className="fine-recipe-details"><summary>View existing documents</summary><ul>{dataset.documents.map((doc) => <li key={doc.id}>{doc.name} · {doc.record_count} examples{doc.page_start > 0 ? ` · Pages ${doc.page_start}–${doc.page_end}` : ''}</li>)}</ul></details>
      <DatasetDownload dataset={dataset} />
      <div className="fine-actions"><button className="btn btn-primary" type="button" onClick={() => onOpen(dataset.id)}>Open Preparation</button></div>
    </article>)}
    </div>
  </section>;
}
