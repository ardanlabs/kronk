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

function DatasetDownload({ dataset, initialVersion }: { dataset: PreparationDataset; initialVersion?: number }) {
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
