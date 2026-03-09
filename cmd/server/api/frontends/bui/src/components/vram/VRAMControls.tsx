import { useState } from 'react';
import { CONTEXT_WINDOW_OPTIONS, BYTES_PER_ELEMENT_OPTIONS, SLOT_OPTIONS } from './constants';
import { PARAM_TOOLTIPS, ParamTooltip } from '../ParamTooltips';
import type { ContextInfo } from '../../lib/context';
import { formatContextHint } from '../../lib/context';

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
  kvCacheOnCPU?: boolean;
  onKvCacheOnCPUChange?: (v: boolean) => void;
  deviceCount?: number;
  onDeviceCountChange?: (v: number) => void;
  tensorSplit?: string;
  onTensorSplitChange?: (v: string) => void;
  contextInfo?: ContextInfo | null;
}

export default function VRAMControls({
  contextWindow, onContextWindowChange,
  bytesPerElement, onBytesPerElementChange,
  slots, onSlotsChange,
  variant = 'form',
  maxDeviceCount,
  isMoE, blockCount,
  expertLayersOnGPU, onExpertLayersOnGPUChange,
  kvCacheOnCPU, onKvCacheOnCPUChange,
  deviceCount, onDeviceCountChange,
  tensorSplit, onTensorSplitChange,
  contextInfo,
}: VRAMControlsProps) {
  const [compactAdvancedOpen, setCompactAdvancedOpen] = useState(false);

  if (variant === 'compact') {
    return (
      <div>
        <div className="controls-row">
          <div className="control-field">
            <label htmlFor="vram-compact-ctx">
              Context Window<ParamTooltip text={PARAM_TOOLTIPS.contextWindow} />
              {contextInfo && contextInfo.hasRoPE && (
                <span style={{ fontSize: '11px', fontWeight: 400, color: 'var(--color-gray-500)', display: 'block', textTransform: 'none', letterSpacing: 'normal' }}>
                  {formatContextHint(contextInfo)}
                </span>
              )}
            </label>
            <select
              id="vram-compact-ctx"
              value={contextWindow}
              onChange={(e) => onContextWindowChange(Number(e.target.value))}
              className="form-select"
            >
              {CONTEXT_WINDOW_OPTIONS.filter(opt => !contextInfo || opt.value <= contextInfo.max).map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label} ({opt.value.toLocaleString()} tokens)
                </option>
              ))}
            </select>
          </div>
          <div className="control-field">
            <label htmlFor="vram-compact-bpe">Cache Type<ParamTooltip text={PARAM_TOOLTIPS.cacheType} /></label>
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
            <label htmlFor="vram-compact-slots">Slots<ParamTooltip text={PARAM_TOOLTIPS.nSeqMax} /></label>
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
          <CompactAdvancedToggle
            open={compactAdvancedOpen}
            onToggle={() => setCompactAdvancedOpen(!compactAdvancedOpen)}
          />
        </div>
        {compactAdvancedOpen && (
          <CompactAdvancedContent
            isMoE={isMoE}
            blockCount={blockCount}
            expertLayersOnGPU={expertLayersOnGPU}
            onExpertLayersOnGPUChange={onExpertLayersOnGPUChange}
            kvCacheOnCPU={kvCacheOnCPU}
            onKvCacheOnCPUChange={onKvCacheOnCPUChange}
            maxDeviceCount={maxDeviceCount}
            deviceCount={deviceCount}
            onDeviceCountChange={onDeviceCountChange}
            tensorSplit={tensorSplit}
            onTensorSplitChange={onTensorSplitChange}
          />
        )}
      </div>
    );
  }

  return (
    <div className="playground-sweep-params">
      <div className="playground-sweep-param">
        <label className="playground-sweep-param-toggle" htmlFor="vram-contextWindow">Context Window<ParamTooltip text={PARAM_TOOLTIPS.contextWindow} /></label>
        <select
          id="vram-contextWindow"
          value={contextWindow}
          onChange={(e) => onContextWindowChange(Number(e.target.value))}
          className="playground-sweep-param-values"
        >
          {CONTEXT_WINDOW_OPTIONS.filter(opt => !contextInfo || opt.value <= contextInfo.max).map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label} ({opt.value.toLocaleString()} tokens)
            </option>
          ))}
        </select>
        {contextInfo && contextInfo.hasRoPE && (
          <div style={{ fontSize: '11px', color: 'var(--color-gray-500)', marginTop: 2 }}>
            {formatContextHint(contextInfo)}
          </div>
        )}
      </div>

      <div className="playground-sweep-param">
        <label className="playground-sweep-param-toggle" htmlFor="vram-bytesPerElement">Cache Type<ParamTooltip text={PARAM_TOOLTIPS.cacheType} /></label>
        <select
          id="vram-bytesPerElement"
          value={bytesPerElement}
          onChange={(e) => onBytesPerElementChange(Number(e.target.value))}
          className="playground-sweep-param-values"
        >
          {BYTES_PER_ELEMENT_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>

      <div className="playground-sweep-param">
        <label className="playground-sweep-param-toggle" htmlFor="vram-slots">Slots<ParamTooltip text={PARAM_TOOLTIPS.nSeqMax} /></label>
        <select
          id="vram-slots"
          value={slots}
          onChange={(e) => onSlotsChange(Number(e.target.value))}
          className="playground-sweep-param-values"
        >
          {SLOT_OPTIONS.map((s) => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>
      </div>

      <AdvancedSection
        isMoE={isMoE}
        blockCount={blockCount}
        expertLayersOnGPU={expertLayersOnGPU}
        onExpertLayersOnGPUChange={onExpertLayersOnGPUChange}
        kvCacheOnCPU={kvCacheOnCPU}
        onKvCacheOnCPUChange={onKvCacheOnCPUChange}
        maxDeviceCount={maxDeviceCount}
        deviceCount={deviceCount}
        onDeviceCountChange={onDeviceCountChange}
        tensorSplit={tensorSplit}
        onTensorSplitChange={onTensorSplitChange}
      />
    </div>
  );
}

interface AdvancedSectionProps {
  isMoE?: boolean;
  blockCount?: number;
  expertLayersOnGPU?: number;
  onExpertLayersOnGPUChange?: (v: number) => void;
  kvCacheOnCPU?: boolean;
  onKvCacheOnCPUChange?: (v: boolean) => void;
  maxDeviceCount?: number;
  deviceCount?: number;
  onDeviceCountChange?: (v: number) => void;
  tensorSplit?: string;
  onTensorSplitChange?: (v: string) => void;
}

function AdvancedSection({
  isMoE, blockCount, expertLayersOnGPU, onExpertLayersOnGPUChange,
  kvCacheOnCPU, onKvCacheOnCPUChange,
  maxDeviceCount, deviceCount, onDeviceCountChange,
  tensorSplit, onTensorSplitChange,
}: AdvancedSectionProps) {
  const [open, setOpen] = useState(false);

  return (
    <div style={{ gridColumn: '1 / -1', padding: '10px' }}>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        style={{
          background: 'none',
          border: 'none',
          padding: 0,
          cursor: 'pointer',
          fontSize: '13px',
          color: 'var(--color-text-secondary)',
          display: 'flex',
          alignItems: 'center',
          gap: '4px',
        }}
      >
        <span style={{ display: 'inline-block', transition: 'transform 0.2s', transform: open ? 'rotate(90deg)' : 'rotate(0deg)' }}>▶</span>
        Advanced
      </button>
      {open && (
        <div style={{ marginTop: '8px', display: 'flex', flexWrap: 'wrap', gap: '12px', alignItems: 'stretch' }}>
          <div className="playground-sweep-param">
            <label className="playground-sweep-param-toggle" htmlFor="vram-kvCpu" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <input
                id="vram-kvCpu"
                type="checkbox"
                checked={kvCacheOnCPU ?? false}
                onChange={(e) => onKvCacheOnCPUChange?.(e.target.checked)}
              />
              KV Cache on CPU<ParamTooltip text={PARAM_TOOLTIPS.kvCacheOnCPU} />
            </label>
            <div style={{ fontSize: '11px', color: 'var(--color-gray-500)', marginTop: 2 }}>
              Offload KV cache to system RAM to reduce VRAM usage
            </div>
          </div>

          <div className="playground-sweep-param">
            <label className="playground-sweep-param-toggle" htmlFor="vram-deviceCount">GPU Count<ParamTooltip text={PARAM_TOOLTIPS.gpuCount} /></label>
            <select
              id="vram-deviceCount"
              value={deviceCount ?? 1}
              onChange={(e) => onDeviceCountChange?.(Number(e.target.value))}
              className="playground-sweep-param-values"
            >
              {gpuCountOptions(maxDeviceCount).map(n => (
                <option key={n} value={n}>{n} GPU{n > 1 ? 's' : ''}</option>
              ))}
            </select>
          </div>

          {(deviceCount ?? 1) > 1 && (
            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle" htmlFor="vram-tensorSplit">
                Tensor Split (proportions, e.g. 0.6,0.4)<ParamTooltip text={PARAM_TOOLTIPS.tensorSplit} />
              </label>
              <input
                id="vram-tensorSplit"
                type="text"
                value={tensorSplit ?? ''}
                onChange={(e) => onTensorSplitChange?.(e.target.value)}
                className="playground-sweep-param-values"
                placeholder="empty = equal split"
              />
              <div style={{ fontSize: '11px', color: 'var(--color-gray-500)', marginTop: 2 }}>
                Leave empty for equal distribution across GPUs
              </div>
            </div>
          )}

          {isMoE && blockCount != null && blockCount > 0 && (
            <div className="playground-sweep-param" style={{ width: '100%' }}>
              <label className="playground-sweep-param-toggle" htmlFor="vram-expertLayers">
                Expert Layers on GPU ({expertLayersOnGPU ?? 0} of {blockCount})<ParamTooltip text={PARAM_TOOLTIPS.expertLayersOnGPU} />
              </label>
              <input
                id="vram-expertLayers"
                type="range"
                min={0}
                max={blockCount}
                value={expertLayersOnGPU ?? 0}
                onChange={(e) => onExpertLayersOnGPUChange?.(Number(e.target.value))}
                style={{ width: '100%', marginTop: 6 }}
              />
              <div style={{ fontSize: '11px', color: 'var(--color-gray-500)', marginTop: 2 }}>
                {expertLayersOnGPU === 0
                  ? 'All experts on CPU (recommended for limited VRAM)'
                  : expertLayersOnGPU === blockCount
                    ? 'All experts on GPU (requires full VRAM)'
                    : `Top ${expertLayersOnGPU} layers on GPU, rest on CPU`}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function CompactAdvancedToggle({ open, onToggle }: { open: boolean; onToggle: () => void }) {
  return (
    <div style={{ display: 'flex', alignItems: 'flex-end', flexShrink: 0, paddingBottom: '8px' }}>
      <button
        type="button"
        onClick={onToggle}
        style={{
          background: 'none',
          border: 'none',
          padding: 0,
          cursor: 'pointer',
          fontSize: '13px',
          color: 'var(--color-text-secondary)',
          display: 'flex',
          alignItems: 'center',
          gap: '4px',
          whiteSpace: 'nowrap',
        }}
      >
        <span style={{ display: 'inline-block', transition: 'transform 0.2s', transform: open ? 'rotate(90deg)' : 'rotate(0deg)' }}>▶</span>
        Advanced
      </button>
    </div>
  );
}

function CompactAdvancedContent({
  isMoE, blockCount, expertLayersOnGPU, onExpertLayersOnGPUChange,
  kvCacheOnCPU, onKvCacheOnCPUChange,
  maxDeviceCount, deviceCount, onDeviceCountChange,
  tensorSplit, onTensorSplitChange,
}: AdvancedSectionProps) {
  return (
    <div style={{ marginTop: '8px', display: 'flex', flexWrap: 'wrap', gap: '12px' }}>
      {isMoE && blockCount != null && blockCount > 0 && (
        <div className="control-field" style={{ width: '100%' }}>
          <label htmlFor="vram-compact-expertLayers">
            Expert Layers GPU ({expertLayersOnGPU ?? 0}/{blockCount})<ParamTooltip text={PARAM_TOOLTIPS.expertLayersOnGPU} />
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
        <label htmlFor="vram-compact-kvCpu" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <input
            id="vram-compact-kvCpu"
            type="checkbox"
            checked={kvCacheOnCPU ?? false}
            onChange={(e) => onKvCacheOnCPUChange?.(e.target.checked)}
          />
          KV Cache on CPU<ParamTooltip text={PARAM_TOOLTIPS.kvCacheOnCPU} />
        </label>
      </div>
      <div className="control-field">
        <label htmlFor="vram-compact-deviceCount">GPU Count<ParamTooltip text={PARAM_TOOLTIPS.gpuCount} /></label>
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
          <label htmlFor="vram-compact-tensorSplit">Tensor Split<ParamTooltip text={PARAM_TOOLTIPS.tensorSplit} /></label>
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
