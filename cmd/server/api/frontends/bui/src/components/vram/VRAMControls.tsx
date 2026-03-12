import { useState } from 'react';
import { CONTEXT_WINDOW_OPTIONS, BYTES_PER_ELEMENT_OPTIONS, SLOT_OPTIONS } from './constants';
import { PARAM_TOOLTIPS, ParamTooltip } from '../ParamTooltips';
import { formatBytes } from '../../lib/format';
import type { ContextInfo } from '../../lib/context';
import { formatContextHint } from '../../lib/context';

type OffloadStrategy = 'layer' | 'expert';

// ── GPU Layers slider (Layer Offloading) ───────────────────────────────────

function GpuLayersSlider({
  blockCount,
  gpuLayers,
  onGpuLayersChange,
  variant = 'form',
}: {
  blockCount: number;
  gpuLayers?: number;
  onGpuLayersChange?: (v: number) => void;
  variant?: 'form' | 'compact';
}) {
  const layers = gpuLayers ?? blockCount;

  const label = variant === 'compact'
    ? `Layers on GPU (${layers}/${blockCount})`
    : `Layers on GPU (${layers} of ${blockCount})`;

  const hint = layers === 0
    ? 'All layers on CPU (slowest, saves most VRAM)'
    : layers >= blockCount
      ? 'All layers on GPU (fastest)'
      : `${layers} layers on GPU, ${blockCount - layers} on CPU — every token pays a penalty for CPU layers`;

  if (variant === 'compact') {
    return (
      <div className="control-field" style={{ width: '100%' }}>
        <label htmlFor="vram-compact-gpuLayers">
          {label}<ParamTooltip text={PARAM_TOOLTIPS.gpuLayers} />
        </label>
        <input
          id="vram-compact-gpuLayers"
          type="range"
          min={0}
          max={blockCount}
          value={layers}
          onChange={(e) => onGpuLayersChange?.(Number(e.target.value))}
          className="form-range"
        />
        <div style={{ fontSize: '11px', color: 'var(--color-gray-500)', marginTop: 2 }}>{hint}</div>
      </div>
    );
  }

  return (
    <div className="playground-sweep-param" style={{ width: '100%' }}>
      <label className="playground-sweep-param-toggle" htmlFor="vram-gpuLayers">
        {label}<ParamTooltip text={PARAM_TOOLTIPS.gpuLayers} />
      </label>
      <input
        id="vram-gpuLayers"
        type="range"
        min={0}
        max={blockCount}
        value={layers}
        onChange={(e) => onGpuLayersChange?.(Number(e.target.value))}
        style={{ width: '100%', marginTop: 6 }}
      />
      <div style={{ fontSize: '11px', color: 'var(--color-gray-500)', marginTop: 2 }}>{hint}</div>
    </div>
  );
}

// ── Expert Layers slider (Expert Offloading, MoE only) ─────────────────────

function ExpertLayersSlider({
  blockCount,
  expertLayersOnGPU,
  onExpertLayersOnGPUChange,
  variant = 'form',
}: {
  blockCount: number;
  expertLayersOnGPU?: number;
  onExpertLayersOnGPUChange?: (v: number) => void;
  variant?: 'form' | 'compact';
}) {
  const layers = expertLayersOnGPU ?? 0;

  const label = variant === 'compact'
    ? `Experts on GPU (${layers}/${blockCount} layers)`
    : `Experts on GPU (${layers} of ${blockCount} layers)`;

  const hint = layers === 0
    ? 'All expert weights on CPU (always-active weights remain on GPU)'
    : layers === blockCount
      ? 'All expert weights on GPU'
      : `Expert weights from top ${layers} layers on GPU, rest on CPU — always-active weights remain on GPU`;

  if (variant === 'compact') {
    return (
      <div className="control-field" style={{ width: '100%' }}>
        <label htmlFor="vram-compact-expertLayers">
          {label}<ParamTooltip text={PARAM_TOOLTIPS.expertLayersOnGPU} />
        </label>
        <input
          id="vram-compact-expertLayers"
          type="range"
          min={0}
          max={blockCount}
          value={layers}
          onChange={(e) => onExpertLayersOnGPUChange?.(Number(e.target.value))}
          className="form-range"
        />
        <div style={{ fontSize: '11px', color: 'var(--color-gray-500)', marginTop: 2 }}>{hint}</div>
      </div>
    );
  }

  return (
    <div className="playground-sweep-param" style={{ width: '100%' }}>
      <label className="playground-sweep-param-toggle" htmlFor="vram-expertLayers">
        {label}<ParamTooltip text={PARAM_TOOLTIPS.expertLayersOnGPU} />
      </label>
      <input
        id="vram-expertLayers"
        type="range"
        min={0}
        max={blockCount}
        value={layers}
        onChange={(e) => onExpertLayersOnGPUChange?.(Number(e.target.value))}
        style={{ width: '100%', marginTop: 6 }}
      />
      <div style={{ fontSize: '11px', color: 'var(--color-gray-500)', marginTop: 2 }}>{hint}</div>
    </div>
  );
}

// ── Offload Strategy selector (MoE only) ───────────────────────────────────

function OffloadStrategySelector({
  strategy,
  onStrategyChange,
  variant = 'form',
  disableExpert = false,
}: {
  strategy: OffloadStrategy;
  onStrategyChange: (s: OffloadStrategy) => void;
  variant?: 'form' | 'compact';
  disableExpert?: boolean;
}) {
  const btnStyle = (active: boolean, disabled = false): React.CSSProperties => ({
    flex: 1,
    padding: '8px 16px',
    fontSize: '13px',
    fontWeight: 500,
    border: active ? '1px solid var(--color-primary)' : '1px solid var(--color-gray-300)',
    background: active ? 'var(--color-primary)' : 'var(--color-gray-100)',
    color: active ? '#fff' : 'var(--color-text)',
    cursor: disabled ? 'not-allowed' : 'pointer',
    opacity: disabled ? 0.4 : 1,
    transition: 'background 0.15s, color 0.15s, border-color 0.15s',
  });

  const description = strategy === 'layer'
    ? 'Move entire layers to CPU. Every token pays a penalty for each CPU layer.'
    : 'Keep always-active weights on GPU, move only expert weights to CPU. Less painful.';

  const wrapperClass = variant === 'compact' ? 'control-field' : 'playground-sweep-param';
  const labelClass = variant === 'compact' ? undefined : 'playground-sweep-param-toggle';

  return (
    <div className={wrapperClass} style={{ width: '100%' }}>
      <label className={labelClass}>
        Offload Strategy<ParamTooltip text="MoE models support two offloading strategies. Layer Offloading moves entire layers to CPU. Expert Offloading keeps the always-active weights on GPU and only moves expert weights to CPU." />
      </label>
      <div style={{ display: 'flex', marginTop: 4 }}>
        <button
          type="button"
          style={{ ...btnStyle(strategy === 'layer'), borderRadius: '4px 0 0 4px' }}
          onClick={() => onStrategyChange('layer')}
        >
          Layer Offloading
        </button>
        <button
          type="button"
          style={{ ...btnStyle(strategy === 'expert', disableExpert), borderRadius: '0 4px 4px 0', borderLeft: 'none' }}
          onClick={() => !disableExpert && onStrategyChange('expert')}
          disabled={disableExpert}
        >
          Expert Offloading{disableExpert ? ' (MoE only)' : ''}
        </button>
      </div>
      <div style={{ fontSize: '11px', color: 'var(--color-gray-500)', marginTop: 4 }}>
        {description}
      </div>
    </div>
  );
}

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
  gpuLayers?: number;
  onGpuLayersChange?: (v: number) => void;
  expertLayersOnGPU?: number;
  onExpertLayersOnGPUChange?: (v: number) => void;
  kvCacheOnCPU?: boolean;
  onKvCacheOnCPUChange?: (v: boolean) => void;
  deviceCount?: number;
  onDeviceCountChange?: (v: number) => void;
  tensorSplit?: string;
  onTensorSplitChange?: (v: string) => void;
  contextInfo?: ContextInfo | null;
  modelSizeBytes?: number;
  modelWeightsCPU?: number;
  kvCpuBytes?: number;
  totalSystemRamEst?: number;
  systemRAMBytes?: number;
}

export default function VRAMControls({
  contextWindow, onContextWindowChange,
  bytesPerElement, onBytesPerElementChange,
  slots, onSlotsChange,
  variant = 'form',
  maxDeviceCount,
  isMoE, blockCount,
  gpuLayers, onGpuLayersChange,
  expertLayersOnGPU, onExpertLayersOnGPUChange,
  kvCacheOnCPU, onKvCacheOnCPUChange,
  deviceCount, onDeviceCountChange,
  tensorSplit, onTensorSplitChange,
  contextInfo,
  modelSizeBytes, modelWeightsCPU, kvCpuBytes, totalSystemRamEst, systemRAMBytes,
}: VRAMControlsProps) {
  const [compactAdvancedOpen, setCompactAdvancedOpen] = useState(false);
  const [offloadStrategy, setOffloadStrategy] = useState<OffloadStrategy>('layer');

  // When strategy changes, sync the hidden values so the calculation is correct.
  const handleStrategyChange = (s: OffloadStrategy) => {
    setOffloadStrategy(s);
    if (blockCount == null || blockCount <= 0) return;
    if (s === 'layer') {
      // Layer offloading: expert layers follow GPU layers.
      onExpertLayersOnGPUChange?.(gpuLayers ?? blockCount);
    } else {
      // Expert offloading: all layers stay on GPU.
      onGpuLayersChange?.(blockCount);
    }
  };

  // For layer offloading, keep expert layers synced with GPU layers.
  const handleLayerOffloadGpuChange = (v: number) => {
    onGpuLayersChange?.(v);
    onExpertLayersOnGPUChange?.(v);
  };

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
        {blockCount != null && blockCount > 0 && (
          <div style={{ marginTop: '8px' }}>
            <OffloadStrategySelector
              strategy={offloadStrategy}
              onStrategyChange={handleStrategyChange}
              variant="compact"
              disableExpert={!isMoE}
            />
            <div style={{ marginTop: '8px' }}>
              {offloadStrategy === 'layer' || !isMoE ? (
                <GpuLayersSlider
                  blockCount={blockCount}
                  gpuLayers={gpuLayers}
                  onGpuLayersChange={isMoE ? handleLayerOffloadGpuChange : onGpuLayersChange}
                  variant="compact"
                />
              ) : (
                <ExpertLayersSlider
                  blockCount={blockCount}
                  expertLayersOnGPU={expertLayersOnGPU}
                  onExpertLayersOnGPUChange={onExpertLayersOnGPUChange}
                  variant="compact"
                />
              )}
            </div>
          </div>
        )}
        {compactAdvancedOpen && (
          <CompactAdvancedContent
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

      {blockCount != null && blockCount > 0 && (
        <div style={{ gridColumn: '1 / -1' }}>
          <OffloadStrategySelector
            strategy={offloadStrategy}
            onStrategyChange={handleStrategyChange}
            variant="form"
            disableExpert={!isMoE}
          />
        </div>
      )}

      {blockCount != null && blockCount > 0 && (
        <div style={{ gridColumn: '1 / -1' }}>
          {offloadStrategy === 'layer' || !isMoE ? (
            <GpuLayersSlider
              blockCount={blockCount}
              gpuLayers={gpuLayers}
              onGpuLayersChange={isMoE ? handleLayerOffloadGpuChange : onGpuLayersChange}
              variant="form"
            />
          ) : (
            <ExpertLayersSlider
              blockCount={blockCount}
              expertLayersOnGPU={expertLayersOnGPU}
              onExpertLayersOnGPUChange={onExpertLayersOnGPUChange}
              variant="form"
            />
          )}
        </div>
      )}

      {modelSizeBytes != null && modelSizeBytes > 0 && (
        <CpuOffloadBar
          modelSizeBytes={modelSizeBytes}
          modelWeightsCPU={modelWeightsCPU ?? 0}
          kvCpuBytes={kvCpuBytes ?? 0}
          totalSystemRamEst={totalSystemRamEst ?? 0}
          systemRAMBytes={systemRAMBytes}
        />
      )}

    </div>
  );
}
interface AdvancedSectionProps {
  kvCacheOnCPU?: boolean;
  onKvCacheOnCPUChange?: (v: boolean) => void;
  maxDeviceCount?: number;
  deviceCount?: number;
  onDeviceCountChange?: (v: number) => void;
  tensorSplit?: string;
  onTensorSplitChange?: (v: string) => void;
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
  kvCacheOnCPU, onKvCacheOnCPUChange,
  maxDeviceCount, deviceCount, onDeviceCountChange,
  tensorSplit, onTensorSplitChange,
}: AdvancedSectionProps) {
  return (
    <div style={{ marginTop: '8px', display: 'flex', flexWrap: 'wrap', gap: '12px' }}>
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

function CpuOffloadBar({
  modelSizeBytes,
  modelWeightsCPU,
  kvCpuBytes,
  totalSystemRamEst,
  systemRAMBytes,
}: {
  modelSizeBytes: number;
  modelWeightsCPU: number;
  kvCpuBytes: number;
  totalSystemRamEst: number;
  systemRAMBytes?: number;
}) {
  const systemRamUsed = totalSystemRamEst || (modelWeightsCPU + kvCpuBytes);
  const offloadPct = modelSizeBytes > 0 ? (modelWeightsCPU / modelSizeBytes) * 100 : 0;
  const pctLabel = `${Math.round(offloadPct)}% offloaded`;
  const barMax = Math.max(1, modelSizeBytes);

  return (
    <div style={{ gridColumn: '1 / -1', marginTop: '4px', marginBottom: '4px' }}>
      <h4 className="vram-breakdown-title">
        Model Weights on CPU
        {systemRamUsed <= 0 && <span style={{ fontWeight: 400, opacity: 0.7 }}> — Everything on GPU</span>}
      </h4>
      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.85em', marginBottom: '2px' }}>
        <span>
          {modelWeightsCPU > 0
            ? `${pctLabel} — ${formatBytes(modelWeightsCPU)} on CPU`
            : pctLabel}
        </span>
        <span>{formatBytes(systemRamUsed)}{systemRAMBytes != null && systemRAMBytes > 0 ? ` / ${formatBytes(systemRAMBytes)}` : ''}</span>
      </div>
      <div style={{ background: 'var(--color-gray-300)', borderRadius: '4px', height: '20px', overflow: 'hidden', display: 'flex' }}>
        {modelWeightsCPU > 0 && (
          <div
            style={{
              width: `${(modelWeightsCPU / barMax) * 100}%`,
              background: 'var(--color-primary)',
              height: '100%',
            }}
            title={`Model weights: ${formatBytes(modelWeightsCPU)}`}
          />
        )}
        {kvCpuBytes > 0 && (
          <div
            style={{
              width: `${(kvCpuBytes / barMax) * 100}%`,
              background: 'var(--color-orange)',
              height: '100%',
            }}
            title={`KV cache: ${formatBytes(kvCpuBytes)}`}
          />
        )}
      </div>
      {modelWeightsCPU > 0 && kvCpuBytes > 0 && (
        <div style={{ display: 'flex', gap: '12px', fontSize: '0.75em', opacity: 0.7, marginTop: '4px' }}>
          <span style={{ color: 'var(--color-primary)' }}>■ Weights</span>
          <span style={{ color: 'var(--color-orange)' }}>■ KV Cache</span>
        </div>
      )}
    </div>
  );
}
