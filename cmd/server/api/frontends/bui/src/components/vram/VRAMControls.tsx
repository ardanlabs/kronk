import { CONTEXT_WINDOW_OPTIONS, BYTES_PER_ELEMENT_OPTIONS, SLOT_OPTIONS } from './constants';

interface VRAMControlsProps {
  contextWindow: number;
  onContextWindowChange: (v: number) => void;
  bytesPerElement: number;
  onBytesPerElementChange: (v: number) => void;
  slots: number;
  onSlotsChange: (v: number) => void;
  variant?: 'form' | 'compact';
}

export default function VRAMControls({
  contextWindow, onContextWindowChange,
  bytesPerElement, onBytesPerElementChange,
  slots, onSlotsChange,
  variant = 'form',
}: VRAMControlsProps) {
  if (variant === 'compact') {
    return (
      <div className="controls-row">
        <div className="control-field">
          <label htmlFor="vram-compact-ctx">Context Window</label>
          <select
            id="vram-compact-ctx"
            value={contextWindow}
            onChange={(e) => onContextWindowChange(Number(e.target.value))}
            className="form-select"
          >
            {CONTEXT_WINDOW_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label} ({opt.value.toLocaleString()} tokens)
              </option>
            ))}
          </select>
        </div>
        <div className="control-field">
          <label htmlFor="vram-compact-bpe">Cache Type</label>
          <select
            id="vram-compact-bpe"
            value={bytesPerElement}
            onChange={(e) => onBytesPerElementChange(Number(e.target.value))}
            className="form-select"
          >
            {BYTES_PER_ELEMENT_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>
        <div className="control-field">
          <label htmlFor="vram-compact-slots">Slots</label>
          <select
            id="vram-compact-slots"
            value={slots}
            onChange={(e) => onSlotsChange(Number(e.target.value))}
            className="form-select"
          >
            {SLOT_OPTIONS.map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
        </div>
      </div>
    );
  }

  return (
    <>
      <div className="form-group">
        <label htmlFor="vram-contextWindow">Context Window</label>
        <select
          id="vram-contextWindow"
          value={contextWindow}
          onChange={(e) => onContextWindowChange(Number(e.target.value))}
          className="form-select"
        >
          {CONTEXT_WINDOW_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label} ({opt.value.toLocaleString()} tokens)
            </option>
          ))}
        </select>
      </div>
      <div className="form-group">
        <label htmlFor="vram-bytesPerElement">Cache Type (Bytes Per Element)</label>
        <select
          id="vram-bytesPerElement"
          value={bytesPerElement}
          onChange={(e) => onBytesPerElementChange(Number(e.target.value))}
          className="form-select"
        >
          {BYTES_PER_ELEMENT_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>
      <div className="form-group">
        <label htmlFor="vram-slots">Slots (Concurrent Sequences)</label>
        <select
          id="vram-slots"
          value={slots}
          onChange={(e) => onSlotsChange(Number(e.target.value))}
          className="form-select"
        >
          {SLOT_OPTIONS.map((s) => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>
      </div>
    </>
  );
}
