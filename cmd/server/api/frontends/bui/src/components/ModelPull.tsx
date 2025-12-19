import { useState, useRef } from 'react';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import type { PullResponse } from '../types';

export default function ModelPull() {
  const { invalidate } = useModelList();
  const [modelUrl, setModelUrl] = useState('');
  const [projUrl, setProjUrl] = useState('');
  const [pulling, setPulling] = useState(false);
  const [messages, setMessages] = useState<Array<{ text: string; type: 'info' | 'error' | 'success' }>>([]);
  const closeRef = useRef<(() => void) | null>(null);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!modelUrl.trim()) return;

    setPulling(true);
    setMessages([]);

    const addMessage = (text: string, type: 'info' | 'error' | 'success') => {
      setMessages((prev) => [...prev, { text, type }]);
    };

    closeRef.current = api.pullModel(
      modelUrl.trim(),
      projUrl.trim(),
      (data: PullResponse) => {
        if (data.status) {
          addMessage(data.status, 'info');
        }
        if (data.model_file) {
          addMessage(`Model file: ${data.model_file}`, 'info');
        }
        if (data.proj_file) {
          addMessage(`Projection file: ${data.proj_file}`, 'info');
        }
      },
      (error: string) => {
        addMessage(error, 'error');
        setPulling(false);
      },
      () => {
        addMessage('Pull complete!', 'success');
        setPulling(false);
        invalidate();
      }
    );
  };

  const handleCancel = () => {
    if (closeRef.current) {
      closeRef.current();
      closeRef.current = null;
    }
    setPulling(false);
    setMessages((prev) => [...prev, { text: 'Cancelled', type: 'error' }]);
  };

  return (
    <div>
      <div className="page-header">
        <h2>Pull Model</h2>
        <p>Download a model from a URL</p>
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
              placeholder="https://huggingface.co/..."
              disabled={pulling}
            />
          </div>
          <div className="form-group">
            <label htmlFor="projUrl">Projection URL (optional)</label>
            <input
              type="text"
              id="projUrl"
              value={projUrl}
              onChange={(e) => setProjUrl(e.target.value)}
              placeholder="https://huggingface.co/..."
              disabled={pulling}
            />
          </div>
          <div style={{ display: 'flex', gap: '12px' }}>
            <button className="btn btn-primary" type="submit" disabled={pulling || !modelUrl.trim()}>
              {pulling ? 'Pulling...' : 'Pull Model'}
            </button>
            {pulling && (
              <button className="btn btn-danger" type="button" onClick={handleCancel}>
                Cancel
              </button>
            )}
          </div>
        </form>

        {messages.length > 0 && (
          <div className="status-box">
            {messages.map((msg, idx) => (
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
