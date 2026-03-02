import { VRAM_FORMULA_CONTENT } from './constants';

interface VRAMFormulaModalProps {
  onClose: () => void;
}

export default function VRAMFormulaModal({ onClose }: VRAMFormulaModalProps) {
  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content modal-large" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>VRAM Calculation Formula</h3>
          <button
            className="modal-close"
            onClick={onClose}
            aria-label="Close"
          >
            ×
          </button>
        </div>
        <div className="modal-body">
          <pre className="vram-formula-content">{VRAM_FORMULA_CONTENT}</pre>
        </div>
      </div>
    </div>
  );
}
