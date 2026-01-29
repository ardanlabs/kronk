import { useState } from 'react';
import { useDownload } from '../contexts/DownloadContext';

export default function ModelPull() {
  const { download, isDownloading, startDownload, cancelDownload, clearDownload } = useDownload();
  const [modelUrl, setModelUrl] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!modelUrl.trim() || isDownloading) return;
    startDownload(modelUrl.trim());
  };

  const isComplete = download?.status === 'complete';
  const hasError = download?.status === 'error';

  return (
    <div>
      <div className="page-header">
        <h2>Pull Model</h2>
              <p>Download a model from a URL</p>
              <p>Example: Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf</p>
      </div>

      <div className="card">
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label htmlFor="modelUrl">Model URL</label>
            <input
              type="text"
              id="modelUrl"
              value={modelUrl}
              onChange={(e) => setModelUrl(e.target.value)}
              placeholder="..."
              disabled={isDownloading}
            />
          </div>

          <div style={{ display: 'flex', gap: '12px' }}>
            <button
              className="btn btn-primary"
              type="submit"
              disabled={isDownloading || !modelUrl.trim()}
            >
              {isDownloading ? 'Downloading...' : 'Pull Model'}
            </button>
            {isDownloading && (
              <button className="btn btn-danger" type="button" onClick={cancelDownload}>
                Cancel
              </button>
            )}
            {(isComplete || hasError) && (
              <button className="btn" type="button" onClick={clearDownload}>
                Clear
              </button>
            )}
          </div>
        </form>

        {download && download.messages.length > 0 && (
          <div className="status-box">
            {download.messages.map((msg, idx) => (
              <div key={idx} className={`status-line ${msg.type}`}>
                {msg.text}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
