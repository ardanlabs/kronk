import { useState, useEffect, useRef } from 'react';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import type { CatalogModelsResponse, PullResponse } from '../types';

export default function CatalogPull() {
  const { invalidate } = useModelList();
  const [catalogList, setCatalogList] = useState<CatalogModelsResponse | null>(null);
  const [selectedId, setSelectedId] = useState('');
  const [listLoading, setListLoading] = useState(true);
  const [pulling, setPulling] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [messages, setMessages] = useState<Array<{ text: string; type: 'info' | 'error' | 'success' }>>([]);
  const closeRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    loadCatalogList();
  }, []);

  const loadCatalogList = async () => {
    setListLoading(true);
    try {
      const response = await api.listCatalog();
      setCatalogList(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load catalog');
    } finally {
      setListLoading(false);
    }
  };

  const handlePull = () => {
    if (!selectedId) return;

    setPulling(true);
    setMessages([]);
    setError(null);

    const addMessage = (text: string, type: 'info' | 'error' | 'success') => {
      setMessages((prev) => [...prev, { text, type }]);
    };

    closeRef.current = api.pullCatalogModel(
      selectedId,
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
      (errorMsg: string) => {
        addMessage(errorMsg, 'error');
        setPulling(false);
      },
      () => {
        addMessage('Pull complete!', 'success');
        setPulling(false);
        invalidate();
        loadCatalogList();
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
        <h2>Pull Catalog Model</h2>
        <p>Download a model from the catalog</p>
      </div>

      <div className="card">
        {error && <div className="alert alert-error">{error}</div>}

        {listLoading ? (
          <div className="loading">Loading catalog</div>
        ) : (
          <>
            <div className="form-group">
              <label htmlFor="modelSelect">Select Model</label>
              <select
                id="modelSelect"
                value={selectedId}
                onChange={(e) => setSelectedId(e.target.value)}
                disabled={pulling}
              >
                <option value="">-- Select a model --</option>
                {catalogList?.map((model) => (
                  <option key={model.id} value={model.id}>
                    {model.id} {model.downloaded ? '(downloaded)' : ''}
                  </option>
                ))}
              </select>
            </div>

            <div style={{ display: 'flex', gap: '12px' }}>
              <button
                className="btn btn-primary"
                onClick={handlePull}
                disabled={!selectedId || pulling}
              >
                {pulling ? 'Pulling...' : 'Pull Model'}
              </button>
              {pulling && (
                <button className="btn btn-danger" onClick={handleCancel}>
                  Cancel
                </button>
              )}
            </div>
          </>
        )}

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
