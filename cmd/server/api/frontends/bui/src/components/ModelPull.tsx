import { useState } from 'react';
import { useDownload } from '../contexts/DownloadContext';

export default function ModelPull() {
  const { download, isDownloading, startDownload, startBatchDownload, cancelDownload, clearDownload } = useDownload();
  const [modelUrls, setModelUrls] = useState<string[]>(['']);
  const [projUrl, setProjUrl] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const urls = modelUrls.map((u) => u.trim()).filter(Boolean);
    if (urls.length === 0 || isDownloading) return;

    if (urls.length === 1) {
      startDownload(urls[0], projUrl.trim() || undefined);
    } else {
      startBatchDownload(urls, projUrl.trim() || undefined);
    }
  };

  const updateUrl = (index: number, value: string) => {
    setModelUrls((prev) => {
      const updated = [...prev];
      updated[index] = value;
      return updated;
    });
  };

  const addUrlRow = () => {
    setModelUrls((prev) => [...prev, '']);
  };

  const removeUrlRow = (index: number) => {
    setModelUrls((prev) => prev.filter((_, i) => i !== index));
  };

  const hasValidUrl = modelUrls.some((u) => u.trim().length > 0);
  const isComplete = download?.status === 'complete';
  const hasError = download?.status === 'error';

  return (
    <div>
      <div className="page-header">
        <h2>Pull Model</h2>
              <p>Download a model from a URL or shorthand reference. For split models, add multiple URLs and they will be pulled sequentially.</p>
              <p>Shorthand: <code>bartowski/Qwen3-8B-GGUF:Q4_K_M</code> or <code>hf.co/bartowski/Qwen3-8B-GGUF:Q4_K_M</code></p>
              <p>With revision: <code>bartowski/Qwen3-8B-GGUF:Q4_K_M@main</code></p>
              <p>Full URL: <code>https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/Qwen2.5-VL-3B-Instruct-Q4_K_M.gguf</code></p>
      </div>

      <div className="card">
        <form onSubmit={handleSubmit}>
          {modelUrls.map((url, index) => (
            <div className="form-group" key={index}>
              <label htmlFor={`modelUrl-${index}`}>
                {modelUrls.length === 1 ? 'Model URL' : `Model URL ${index + 1}`}
              </label>
              <div style={{ display: 'flex', gap: '8px' }}>
                <input
                  type="text"
                  id={`modelUrl-${index}`}
                  value={url}
                  onChange={(e) => updateUrl(index, e.target.value)}
                  placeholder="owner/repo:Q4_K_M or https://huggingface.co/org/repo/resolve/main/model.gguf"
                  disabled={isDownloading}
                  style={{ flex: 1 }}
                />
                {modelUrls.length > 1 && (
                  <button
                    type="button"
                    className="btn btn-danger"
                    onClick={() => removeUrlRow(index)}
                    disabled={isDownloading}
                    style={{ padding: '8px 12px' }}
                  >
                    ✕
                  </button>
                )}
              </div>
            </div>
          ))}

          <div style={{ marginBottom: '16px' }}>
            <button
              type="button"
              className="btn btn-secondary"
              onClick={addUrlRow}
              disabled={isDownloading}
            >
              + Add URL
            </button>
          </div>

          <div className="form-group">
            <label htmlFor="projUrl">Projection URL (optional, for vision/audio models)</label>
            <input
              type="text"
              id="projUrl"
              value={projUrl}
              onChange={(e) => setProjUrl(e.target.value)}
              placeholder="https://huggingface.co/org/repo/resolve/main/mmproj-model.gguf"
              disabled={isDownloading}
            />
          </div>

          <div style={{ display: 'flex', gap: '12px' }}>
            <button
              className="btn btn-primary"
              type="submit"
              disabled={isDownloading || !hasValidUrl}
            >
              {isDownloading ? 'Downloading...' : modelUrls.filter((u) => u.trim()).length > 1 ? 'Pull Models' : 'Pull Model'}
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
