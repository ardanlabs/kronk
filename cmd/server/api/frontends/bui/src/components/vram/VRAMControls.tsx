import { CONTEXT_WINDOW_OPTIONS, BYTES_PER_ELEMENT_OPTIONS, SLOT_OPTIONS } from './constants';

const GPU_COUNT_FALLBACK = [1, 2, 4, 8];

function gpuCountOptions(maxDeviceCount?: number): number[] {
  if (maxDeviceCount != null && maxDeviceCount > 0) {
    return Array.from({ length: maxDeviceCount }, (_, i) => i + 1);
  }
  return GPU_COUNT_FALLBACK;
}

interface VRAMControlsProps {
  contextWindow: number;
  onContextWindowChange: (v: number) => void;
  bytesPerElement: number;
  onBytesPerElementChange: (v: number) => void;
  slots: number;
  onSlotsChange: (v: number) => void;
  variant?: 'form' | 'compact';
  maxDeviceCount?: number;
  isMoE?: boolean;
  blockCount?: number;
  expertLayersOnGPU?: number;
  onExpertLayersOnGPUChange?: (v: number) => void;
  deviceCount?: number;
  onDeviceCountChange?: (v: number) => void;
  tensorSplit?: string;
  onTensorSplitChange?: (v: string) => void;
}

export default function VRAMControls({
  contextWindow, onContextWindowChange,
  bytesPerElement, onBytesPerElementChange,
  slots, onSlotsChange,
  variant = 'form',
  maxDeviceCount,
  isMoE, blockCount,
  expertLayersOnGPU, onExpertLayersOnGPUChange,
  deviceCount, onDeviceCountChange,
  tensorSplit, onTensorSplitChange,
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
        {isMoE && blockCount != null && blockCount > 0 && (
          <div className="control-field">
            <label htmlFor="vram-compact-expertLayers">
              Expert Layers GPU ({expertLayersOnGPU ?? 0}/{blockCount})
            </label>
            <input
              id="vram-compact-expertLayers"
              type="range"
              min={0}
              max={blockCount}
              value={expertLayersOnGPU ?? 0}
              onChange={(e) => onExpertLayersOnGPUChange?.(Number(e.target.value))}
              className="form-range"
            />
          </div>
        )}
        <div className="control-field">
          <label htmlFor="vram-compact-deviceCount">GPU Count</label>
          <select
            id="vram-compact-deviceCount"
            value={deviceCount ?? 1}
            onChange={(e) => onDeviceCountChange?.(Number(e.target.value))}
            className="form-select"
          >
            {gpuCountOptions(maxDeviceCount).map(n => (
              <option key={n} value={n}>{n} GPU{n > 1 ? 's' : ''}</option>
            ))}
          </select>
        </div>
        {(deviceCount ?? 1) > 1 && (
          <div className="control-field">
            <label htmlFor="vram-compact-tensorSplit">Tensor Split</label>
            <input
              id="vram-compact-tensorSplit"
              type="text"
              value={tensorSplit ?? ''}
              onChange={(e) => onTensorSplitChange?.(e.target.value)}
              className="form-input"
              placeholder="e.g. 0.6,0.4"
            />
          </div>
        )}
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
      {isMoE && blockCount != null && blockCount > 0 && (
        <div className="form-group">
          <label htmlFor="vram-expertLayers">
            Expert Layers on GPU ({expertLayersOnGPU ?? 0} of {blockCount})
          </label>
          <input
            id="vram-expertLayers"
            type="range"
            min={0}
            max={blockCount}
            value={expertLayersOnGPU ?? 0}
            onChange={(e) => onExpertLayersOnGPUChange?.(Number(e.target.value))}
            className="form-range"
          />
          <div style={{ fontSize: '0.85em', opacity: 0.7 }}>
            {expertLayersOnGPU === 0
              ? 'All experts on CPU (recommended for limited VRAM)'
              : expertLayersOnGPU === blockCount
                ? 'All experts on GPU (requires full VRAM)'
                : `Top ${expertLayersOnGPU} layers on GPU, rest on CPU`}
          </div>
        </div>
      )}

      {/* Multi-GPU Configuration */}
      <div className="form-group">
        <label htmlFor="vram-deviceCount">GPU Count</label>
        <select
          id="vram-deviceCount"
          value={deviceCount ?? 1}
          onChange={(e) => onDeviceCountChange?.(Number(e.target.value))}
          className="form-select"
        >
          {gpuCountOptions(maxDeviceCount).map(n => (
            <option key={n} value={n}>{n} GPU{n > 1 ? 's' : ''}</option>
          ))}
        </select>
      </div>
      {(deviceCount ?? 1) > 1 && (
        <div className="form-group">
          <label htmlFor="vram-tensorSplit">
            Tensor Split (proportions, e.g. 0.6,0.4)
          </label>
          <input
            id="vram-tensorSplit"
            type="text"
            value={tensorSplit ?? ''}
            onChange={(e) => onTensorSplitChange?.(e.target.value)}
            className="form-input"
            placeholder="empty = equal split"
          />
          <div style={{ fontSize: '0.85em', opacity: 0.7, marginTop: '2px' }}>
            Leave empty for equal distribution across GPUs
          </div>
        </div>
      )}
    </>
  );
}
