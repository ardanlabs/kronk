import { useState, useEffect } from 'react';
import type { DownloadProgress, DownloadMeta } from '../contexts/DownloadContext';

function formatBytes(bytes: number): string {
  if (bytes < 1000 * 1000) return `${(bytes / 1000).toFixed(0)} KB`;
  if (bytes < 1000 * 1000 * 1000) return `${(bytes / (1000 * 1000)).toFixed(1)} MB`;
  return `${(bytes / (1000 * 1000 * 1000)).toFixed(2)} GB`;
}

function formatEta(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`;
  const m = Math.floor(seconds / 60);
  const s = Math.round(seconds % 60);
  if (m < 60) return `${m}m ${s}s`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

interface Props {
  progress: DownloadProgress;
  meta?: DownloadMeta;
}

export default function DownloadProgressBar({ progress, meta }: Props) {
  const [, setTick] = useState(0);

  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 1000);
    return () => clearInterval(id);
  }, []);

  const { currentBytes, totalBytes, mbPerSec, pct, startedAtMs } = progress;
  const elapsedSec = (Date.now() - startedAtMs) / 1000;

  let etaLabel = '';
  if (elapsedSec >= 5 && totalBytes > 0 && currentBytes < totalBytes && mbPerSec > 0) {
    const remainingBytes = totalBytes - currentBytes;
    const etaSec = remainingBytes / (mbPerSec * 1000 * 1000);
    etaLabel = ` · ETA ${formatEta(etaSec)}`;
  }

  const fileName = progress.src.split('/').pop() || progress.src;
  const isSplit = meta && meta.fileTotal > 1;
  const hasTotal = totalBytes > 0;
  const pctDisplay = hasTotal ? `${pct.toFixed(1)}%` : '';
  const sizeLabel = hasTotal
    ? `${formatBytes(currentBytes)} / ${formatBytes(totalBytes)}`
    : `${formatBytes(currentBytes)}`;
  const speedLabel = mbPerSec > 0 ? `${mbPerSec.toFixed(1)} MB/s` : '';

  const label = [pctDisplay, sizeLabel, speedLabel].filter(Boolean).join(' · ') + etaLabel;
  const barPct = hasTotal ? pct : 0;
  const showInside = barPct >= 45;

  return (
    <div className="download-progress">
      <div className="download-progress-text">
        <span>Downloading {fileName}{isSplit ? ` (file ${meta.fileIndex} of ${meta.fileTotal})` : ''}</span>
      </div>
      <div className="playground-autotest-progress-bar">
        <div
          className="playground-autotest-progress-fill"
          style={{ width: `${Math.max(barPct, 1)}%` }}
        >
          {showInside && <span className="playground-autotest-progress-label">{label}</span>}
        </div>
        {!showInside && <span className="playground-autotest-progress-label-outside">{label}</span>}
      </div>
    </div>
  );
}
