import { useState, useEffect } from 'react';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';

export default function ModelRemove() {
  const { models, loading: listLoading, error: listError, loadModels, invalidate } = useModelList();
  const [selectedModel, setSelectedModel] = useState('');
  const [removing, setRemoving] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  useEffect(() => {
    loadModels();
  }, [loadModels]);

  const handleRemoveClick = () => {
    if (!selectedModel) return;
    setConfirming(true);
  };

  const handleConfirmRemove = async () => {
    if (!selectedModel) return;

    setRemoving(true);
    setConfirming(false);
    setError(null);
    setSuccess(null);
    try {
      await api.removeModel(selectedModel);
      setSuccess(`Model "${selectedModel}" removed successfully`);
      setSelectedModel('');
      invalidate();
      await loadModels();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to remove model');
    } finally {
      setRemoving(false);
    }
  };

  const handleCancelConfirm = () => {
    setConfirming(false);
  };

  return (
    <div>
      <div className="page-header">
        <h2>Remove Model</h2>
        <p>Delete a model from the system</p>
      </div>

      <div className="card">
        {(error || listError) && <div className="alert alert-error">{error || listError}</div>}
        {success && <div className="alert alert-success">{success}</div>}

        {listLoading ? (
          <div className="loading">Loading models</div>
        ) : confirming ? (
          <div>
            <p style={{ marginBottom: '16px' }}>
              Are you sure you want to remove <strong>{selectedModel}</strong>? This action cannot be undone.
            </p>
            <div style={{ display: 'flex', gap: '12px' }}>
              <button className="btn btn-danger" onClick={handleConfirmRemove}>
                Yes, Remove
              </button>
              <button className="btn btn-secondary" onClick={handleCancelConfirm}>
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <>
            <div className="form-group">
              <label htmlFor="modelSelect">Select Model</label>
              <select
                id="modelSelect"
                value={selectedModel}
                onChange={(e) => setSelectedModel(e.target.value)}
                disabled={removing}
              >
                <option value="">-- Select a model --</option>
                {models?.data?.map((model) => (
                  <option key={model.id} value={model.id}>
                    {model.id}
                  </option>
                ))}
              </select>
            </div>

            <div style={{ display: 'flex', gap: '12px' }}>
              <button
                className="btn btn-danger"
                onClick={handleRemoveClick}
                disabled={!selectedModel || removing}
              >
                {removing ? 'Removing...' : 'Remove Model'}
              </button>
              <button
                className="btn btn-secondary"
                onClick={() => {
                  invalidate();
                  loadModels();
                }}
                disabled={listLoading || removing}
              >
                Refresh List
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
