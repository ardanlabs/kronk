import { useState } from 'react';
import { useDownload } from '../contexts/DownloadContext';

export default function ModelPull() {
  const { download, isDownloading, startDownload, cancelDownload, clearDownload } = useDownload();
  const [modelUrl, setModelUrl] = useState('');
  const [projUrl, setProjUrl] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!modelUrl.trim() || isDownloading) return;
    startDownload(modelUrl.trim(), projUrl.trim() || undefined);
  };

  const isComplete = download?.status === 'complete';
  const hasError = download?.status === 'error';

  return (
    <div>
      <div className="page-header">
        <h2>Pull Model</h2>
              <p>Download a model from a URL</p>
              <p>Example: ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/Qwen2.5-VL-3B-Instruct-Q4_K_M.gguf</p>
              <p>Example: https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/Qwen2.5-VL-3B-Instruct-Q4_K_M.gguf</p>
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
              placeholder="org/repo/model.gguf"
              disabled={isDownloading}
            />
          </div>

          <div className="form-group">
            <label htmlFor="projUrl">Projection URL (optional, for vision/audio models)</label>
            <input
              type="text"
              id="projUrl"
              value={projUrl}
              onChange={(e) => setProjUrl(e.target.value)}
              placeholder="org/repo/mmproj-model.gguf"
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
