import { useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import KeyValueTable from '../KeyValueTable';
import { formatBytes } from '../../lib/format';
import { labelWithTip } from '../ParamTooltips';
import type { VRAMInput, MoEInfo, WeightBreakdown, PerDeviceVRAM, DeviceInfo } from '../../types';

interface VRAMResultsProps {
  totalVram: number;
  slotMemory: number;
  kvPerSlot: number;
  kvPerTokenPerLayer: number;
  input: VRAMInput;
  moe?: MoEInfo | null;
  weights?: WeightBreakdown | null;
  modelWeightsGPU?: number;
  modelWeightsCPU?: number;
  computeBufferEst?: number;
  alwaysActiveGPUBytes?: number;
  alwaysActiveCPUBytes?: number;
  expertGPUBytes?: number;
  expertCPUBytes?: number;
  gpuLayers?: number;
  expertLayersOnGPU?: number;
  kvCacheOnCPU?: boolean;
  kvCpuBytes?: number;
  totalSystemRamEst?: number;
  perDevice?: PerDeviceVRAM[];
  deviceCount?: number;
  systemRAMBytes?: number;
  gpuTotalBytes?: number;
  gpuDevices?: DeviceInfo[];
  tensorSplit?: string;
  isHardwareOverridden?: boolean;
  modelUrl?: string;
}

export default function VRAMResults({
  totalVram,
  slotMemory,
  kvPerSlot,
  kvPerTokenPerLayer,
  input,
  moe,
  weights,
  modelWeightsGPU,
  modelWeightsCPU,
  computeBufferEst,
  alwaysActiveGPUBytes,
  alwaysActiveCPUBytes,
  expertGPUBytes,
  expertCPUBytes,
  gpuLayers,
  expertLayersOnGPU,
  kvCacheOnCPU,
  kvCpuBytes,
  totalSystemRamEst,
  perDevice,
  deviceCount,
  systemRAMBytes,
  gpuTotalBytes,
  gpuDevices,
  tensorSplit,
  isHardwareOverridden,
  modelUrl,
}: VRAMResultsProps) {
  const isMoE = moe?.is_moe === true && weights != null;
  const kvOnCPU = kvCacheOnCPU ?? false;
  const kvCacheLocation = kvOnCPU ? 'System RAM' : 'GPU';
  const isPartialGPU = gpuLayers != null && gpuLayers < input.block_count;

  // On unified memory systems (e.g. Apple Silicon) the GPU may not report
  // dedicated VRAM. Fall back to system RAM as the capacity indicator since
  // GPU and system RAM share the same physical memory pool.
  const effectiveGpuCapacity = (gpuTotalBytes != null && gpuTotalBytes > 0)
    ? gpuTotalBytes
    : (systemRAMBytes ?? 0);

  let breakdownRows: { label: ReactNode; value: string }[];
  if (isMoE) {
    breakdownRows = [
      { label: labelWithTip('Always-Active Weights (GPU)', 'alwaysActiveWeights'), value: formatBytes(alwaysActiveGPUBytes ?? 0) },
      ...(alwaysActiveCPUBytes != null && alwaysActiveCPUBytes > 0 ? [{ label: labelWithTip('Always-Active Weights (CPU)', 'alwaysActiveWeights'), value: formatBytes(alwaysActiveCPUBytes) }] : []),
      {
        label: labelWithTip(`Expert Weights — GPU (${expertLayersOnGPU ?? 0} layers)`, 'expertWeightsGPU'),
        value: formatBytes(expertGPUBytes ?? 0),
      },
      { label: labelWithTip('Expert Weights — CPU', 'expertWeightsCPU'), value: formatBytes(expertCPUBytes ?? 0) },
      { label: labelWithTip(`KV Cache (${kvCacheLocation})`, 'kvCache'), value: formatBytes(slotMemory) },
      { label: labelWithTip('Compute Buffer (estimate)', 'computeBuffer'), value: `~${formatBytes(computeBufferEst ?? 0)}` },
    ];
  } else if (isPartialGPU) {
    breakdownRows = [
      { label: labelWithTip(`Weights on GPU (${gpuLayers} of ${input.block_count} layers)`, 'gpuLayers'), value: formatBytes(modelWeightsGPU ?? 0) },
      { label: labelWithTip(`Weights on CPU (${input.block_count - gpuLayers!} layers)`, 'gpuLayers'), value: formatBytes(modelWeightsCPU ?? 0) },
      { label: labelWithTip(`KV Cache (${kvCacheLocation})`, 'kvCache'), value: formatBytes(slotMemory) },
      { label: labelWithTip('KV Per Slot', 'kvPerSlot'), value: formatBytes(kvPerSlot) },
      { label: labelWithTip('KV Per Token Per Layer', 'kvPerTokenPerLayer'), value: formatBytes(kvPerTokenPerLayer) },
      { label: labelWithTip('Compute Buffer (estimate)', 'computeBuffer'), value: `~${formatBytes(computeBufferEst ?? 0)}` },
    ];
  } else {
    breakdownRows = [
      { label: labelWithTip('Model Weights', 'modelWeights'), value: formatBytes(input.model_size_bytes) },
      { label: labelWithTip(`KV Cache (${kvCacheLocation})`, 'kvCache'), value: formatBytes(slotMemory) },
      { label: labelWithTip('KV Per Slot', 'kvPerSlot'), value: formatBytes(kvPerSlot) },
      { label: labelWithTip('KV Per Token Per Layer', 'kvPerTokenPerLayer'), value: formatBytes(kvPerTokenPerLayer) },
      { label: labelWithTip('Compute Buffer (estimate)', 'computeBuffer'), value: `~${formatBytes(computeBufferEst ?? 0)}` },
    ];
  }

  const headerRows: { label: ReactNode; value: string }[] = [
    { label: labelWithTip('Model Size', 'modelSize'), value: formatBytes(input.model_size_bytes) },
    { label: labelWithTip('Layers (Block Count)', 'blockCount'), value: String(input.block_count) },
    { label: labelWithTip('Head Count KV', 'headCountKV'), value: String(input.head_count_kv) },
    { label: labelWithTip('Key Length', 'keyLength'), value: String(input.key_length) },
    { label: labelWithTip('Value Length', 'valueLength'), value: String(input.value_length) },
  ];

  if (isMoE) {
    headerRows.push(
      { label: labelWithTip('Expert Count', 'expertCount'), value: String(moe!.expert_count) },
      { label: labelWithTip('Active Experts (top-k)', 'activeExperts'), value: String(moe!.expert_used_count) },
    );
    if (moe!.has_shared_experts) {
      headerRows.push({ label: labelWithTip('Shared Experts', 'sharedExperts'), value: 'Yes' });
    }
  }

  const systemRamUsed = (totalSystemRamEst ?? (modelWeightsCPU ?? 0) + (kvCpuBytes ?? 0));
  const showSystemRAM = systemRamUsed > 0;

  return (
    <div className="vram-results">
      <div className="vram-hero" style={{ display: 'flex', gap: '32px', flexWrap: 'wrap' }}>
        <div style={{ flex: 1, minWidth: '180px' }}>
          <div className="vram-hero-label">{labelWithTip('Total Estimated VRAM', 'totalEstimatedVRAM')}</div>
          <div className="vram-hero-value">
            {formatBytes(totalVram)}
            {effectiveGpuCapacity > 0 && (
              <span style={{ fontSize: '0.55em', opacity: 0.5 }}> / {formatBytes(effectiveGpuCapacity)}</span>
            )}
          </div>
        </div>
        {showSystemRAM && systemRAMBytes != null && systemRAMBytes > 0 && (
          <div style={{ minWidth: '180px' }}>
            <div className="vram-hero-label">{labelWithTip('Total Estimated System RAM', 'totalEstimatedSystemRAM')}</div>
            <div className="vram-hero-value">
              {formatBytes(systemRamUsed)}
              <span style={{ fontSize: '0.55em', opacity: 0.5 }}> / {formatBytes(systemRAMBytes)}</span>
            </div>
          </div>
        )}
      </div>

      {(() => {
        const hasGpuInfo = effectiveGpuCapacity > 0;
        const hasRamInfo = systemRAMBytes != null && systemRAMBytes > 0;
        if (!hasGpuInfo && !hasRamInfo) return null;

        const gpuExceeds = hasGpuInfo && totalVram > effectiveGpuCapacity;
        const gpuTight = hasGpuInfo && !gpuExceeds && totalVram > effectiveGpuCapacity * 0.8;
        const gpuOk = hasGpuInfo && !gpuExceeds && !gpuTight;

        const ramExceeds = hasRamInfo && showSystemRAM && systemRamUsed > systemRAMBytes;
        const ramTight = hasRamInfo && showSystemRAM && !ramExceeds && systemRamUsed > systemRAMBytes * 0.8;
        const ramOk = !showSystemRAM || (hasRamInfo && !ramExceeds && !ramTight);

        const hasConcerns = gpuTight || ramTight;

        const hwLabel = isHardwareOverridden ? 'the selected hardware configuration' : 'this Kronk model server';
        let icon: string;
        let summary: string;
        if (gpuExceeds && ramExceeds) {
          icon = '❌';
          summary = `This model will NOT run on ${hwLabel} — exceeds both GPU VRAM and system RAM`;
        } else if (gpuExceeds) {
          icon = '❌';
          summary = `This model will NOT run on ${hwLabel} — exceeds available GPU VRAM`;
        } else if (ramExceeds) {
          icon = '❌';
          summary = `This model will NOT run on ${hwLabel} — exceeds available system RAM`;
        } else if (hasConcerns) {
          icon = '⚠️';
          summary = `This model will run on ${hwLabel} but it's a tight fit`;
        } else {
          icon = '✅';
          summary = `This model will run on ${hwLabel} with these settings`;
        }

        const details: string[] = [];
        if (hasGpuInfo) {
          if (gpuExceeds) details.push(`GPU VRAM: ${formatBytes(totalVram)} needed, ${formatBytes(effectiveGpuCapacity)} available`);
          else if (gpuTight) details.push(`GPU VRAM: limited headroom (${formatBytes(effectiveGpuCapacity - totalVram)} free)`);
          else if (gpuOk) details.push(`GPU VRAM: ${formatBytes(effectiveGpuCapacity - totalVram)} free`);
        }
        if (hasRamInfo && showSystemRAM) {
          if (ramExceeds) details.push(`System RAM: ${formatBytes(systemRamUsed)} needed, ${formatBytes(systemRAMBytes)} available`);
          else if (ramTight) details.push(`System RAM: limited headroom (${formatBytes(systemRAMBytes - systemRamUsed)} free)`);
          else if (ramOk) details.push(`System RAM: ${formatBytes(systemRAMBytes - systemRamUsed)} free`);
        }

        const bgColor = gpuExceeds || ramExceeds
          ? 'var(--color-error-bg, rgba(239, 83, 80, 0.1))'
          : hasConcerns
            ? 'var(--color-warning-bg, rgba(255, 167, 38, 0.1))'
            : 'var(--color-success-bg, rgba(102, 187, 106, 0.1))';
        const borderColor = gpuExceeds || ramExceeds
          ? 'var(--color-error, #ef5350)'
          : hasConcerns
            ? 'var(--color-warning, #ffa726)'
            : 'var(--color-success, #66bb6a)';

        return (
          <div style={{
            marginTop: '12px',
            marginBottom: '16px',
            padding: '12px 16px',
            background: bgColor,
            borderLeft: `3px solid ${borderColor}`,
            borderRadius: '4px',
            fontSize: '0.9em',
          }}>
            <div style={{ fontWeight: 600 }}>{icon} {summary}</div>
            {details.length > 0 && (
              <div style={{ marginTop: '4px', fontSize: '0.9em', opacity: 0.8 }}>
                {details.join(' · ')}
              </div>
            )}
          </div>
        );
      })()}

      {perDevice && perDevice.length >= 1 && (() => {
        return (
        <div style={{ marginTop: '16px' }}>
          <h4 className="vram-breakdown-title">Per-GPU VRAM Allocation (estimated)</h4>
          {perDevice.map((dev, i) => {
            const reportedCapacity = gpuDevices?.[i]?.total_bytes ?? 0;
            const perDeviceCapacity = reportedCapacity > 0
              ? reportedCapacity
              : (perDevice.length === 1 ? effectiveGpuCapacity : Math.floor(effectiveGpuCapacity / perDevice.length));
            const barMax = Math.max(1, perDeviceCapacity > 0 ? perDeviceCapacity : dev.totalBytes);
            const freeBytes = Math.max(0, barMax - dev.totalBytes);
            const overcommit = dev.totalBytes > barMax && perDeviceCapacity > 0;
            return (
              <div key={i} style={{ marginBottom: '8px' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.85em', marginBottom: '2px' }}>
                  <span>{dev.label}</span>
                  <span>
                    {formatBytes(dev.totalBytes)}
                    {perDeviceCapacity > 0 && <span style={{ opacity: 0.6 }}> / {formatBytes(perDeviceCapacity)}</span>}
                  </span>
                </div>
                <div style={{ background: 'var(--color-gray-200)', borderRadius: '4px', height: '20px', overflow: 'hidden', display: 'flex' }}>
                  {dev.weightsBytes > 0 && (
                    <div style={{ width: `${(dev.weightsBytes / barMax) * 100}%`, background: 'var(--color-primary)', height: '100%' }} title={`Weights: ${formatBytes(dev.weightsBytes)}`} />
                  )}
                  {dev.kvBytes > 0 && (
                    <div style={{ width: `${(dev.kvBytes / barMax) * 100}%`, background: 'var(--color-orange)', height: '100%' }} title={`KV Cache: ${formatBytes(dev.kvBytes)}`} />
                  )}
                  {dev.computeBytes > 0 && (
                    <div style={{ width: `${(dev.computeBytes / barMax) * 100}%`, background: '#8b5cf6', height: '100%' }} title={`Compute Buffer: ${formatBytes(dev.computeBytes)}`} />
                  )}
                  {freeBytes > 0 && !overcommit && (
                    <div style={{ flex: 1, background: '#66bb6a', height: '100%' }} title={`Free: ${formatBytes(freeBytes)}`} />
                  )}
                </div>
                {overcommit && (
                  <div style={{ fontSize: '0.75em', color: '#ef5350', marginTop: '2px' }}>
                    ⚠ Exceeds GPU capacity by {formatBytes(dev.totalBytes - perDeviceCapacity)}
                  </div>
                )}
              </div>
            );
          })}
          <div style={{ display: 'flex', gap: '12px', fontSize: '0.75em', opacity: 0.7, marginTop: '4px' }}>
            <span style={{ color: 'var(--color-primary)' }}>■ Weights</span>
            <span style={{ color: 'var(--color-orange)' }}>■ KV Cache</span>
            <span style={{ color: '#8b5cf6' }}>■ Compute</span>
            <span style={{ color: '#66bb6a' }}>■ Free</span>
          </div>
          <div className="alert alert-info" style={{ marginTop: '8px', fontSize: '0.85em' }}>
            <strong>Note:</strong> Per-GPU allocation is estimated based on tensor split proportions. Actual distribution may vary depending on llama.cpp split mode behavior.
          </div>
        </div>
        );
      })()}

      <CatalogConfigSection
        input={input}
        isMoE={isMoE}
        gpuLayers={gpuLayers}
        expertLayersOnGPU={expertLayersOnGPU}
        kvCacheOnCPU={kvOnCPU}
        deviceCount={deviceCount}
        tensorSplit={tensorSplit}
        modelUrl={modelUrl}
      />

      <div className="vram-breakdown">
        <div>
          <h4 className="vram-breakdown-title">
            {isMoE ? 'MoE VRAM Breakdown' : 'Breakdown'}
          </h4>
          <KeyValueTable rows={breakdownRows} />
        </div>
        <div>
          <h4 className="vram-breakdown-title">Model Header</h4>
          <KeyValueTable rows={headerRows} />
        </div>
      </div>

    </div>
  );
}

// ── Catalog Config Section ──────────────────────────────────────────────────

function cacheTypeName(bytesPerElement: number): string {
  switch (bytesPerElement) {
    case 4: return 'f32';
    case 2: return 'f16';
    case 1: return 'q8_0';
    default: return 'f16';
  }
}

export interface VramComputedConfig {
  contextWindow: number;
  nseqMax: number;
  cacheTypeK: string;
  cacheTypeV: string;
  flashAttention: string;
  ngpuLayers: number | null;
  offloadKQV: boolean | null;
  splitMode: string;
  tensorSplit: string;
  moeMode: string;
  moeKeepTopN: number | null;
}

function buildComputedCatalogConfig(
  input: VRAMInput,
  isMoE: boolean,
  gpuLayers?: number,
  expertLayersOnGPU?: number,
  kvCacheOnCPU?: boolean,
  deviceCount?: number,
  tensorSplit?: string,
): VramComputedConfig {
  const cacheType = cacheTypeName(input.bytes_per_element);
  const gpuCount = deviceCount ?? 1;
  const allLayersOnGPU = gpuLayers == null || gpuLayers >= input.block_count;

  let ngpuLayers: number | null = null;
  if (gpuLayers != null && gpuLayers < input.block_count) {
    ngpuLayers = gpuLayers === 0 ? -1 : gpuLayers;
  }

  let moeMode = '';
  let moeKeepTopN: number | null = null;
  if (isMoE && allLayersOnGPU) {
    const layers = expertLayersOnGPU ?? 0;
    const allExpertsOnGPU = layers >= input.block_count;
    if (!allExpertsOnGPU) {
      if (layers > 0) {
        moeMode = 'keep_top_n';
        moeKeepTopN = layers;
      } else {
        moeMode = 'experts_cpu';
      }
    }
  }

  let splitMode = '';
  let effectiveTensorSplit = '';
  if (gpuCount > 1) {
    const nums = tensorSplit
      ?.split(',')
      .map(s => parseFloat(s.trim()))
      .filter(n => !isNaN(n)) ?? [];
    if (nums.length > 0) {
      effectiveTensorSplit = nums.join(', ');
    }
    if (isMoE) {
      splitMode = 'row';
    }
  }

  return {
    contextWindow: input.context_window,
    nseqMax: input.slots,
    cacheTypeK: cacheType,
    cacheTypeV: cacheType,
    flashAttention: 'enabled',
    ngpuLayers,
    offloadKQV: kvCacheOnCPU ? false : null,
    splitMode,
    tensorSplit: effectiveTensorSplit,
    moeMode,
    moeKeepTopN,
  };
}

function configToYAML(config: VramComputedConfig): string {
  const lines: string[] = [];
  lines.push('model-name/variant:');
  lines.push(`  context-window: ${config.contextWindow}`);
  lines.push(`  nseq-max: ${config.nseqMax}`);
  lines.push(`  cache-type-k: ${config.cacheTypeK}`);
  lines.push(`  cache-type-v: ${config.cacheTypeV}`);
  lines.push(`  flash-attention: ${config.flashAttention}`);

  if (config.ngpuLayers != null) {
    lines.push(`  ngpu-layers: ${config.ngpuLayers}`);
  }

  if (config.offloadKQV === false) {
    lines.push('  offload-kqv: false');
  }

  if (config.moeMode) {
    lines.push('  moe:');
    lines.push(`    mode: ${config.moeMode}`);
    if (config.moeMode === 'keep_top_n' && config.moeKeepTopN != null) {
      lines.push(`    keep-experts-top-n: ${config.moeKeepTopN}`);
    }
  }

  if (config.tensorSplit) {
    lines.push(`  tensor-split: [${config.tensorSplit}]`);
  }
  if (config.splitMode) {
    lines.push(`  split-mode: ${config.splitMode}`);
  }

  return lines.join('\n');
}

export interface VramCatalogDraft {
  source: 'vram';
  modelUrl: string;
  config: VramComputedConfig;
}

function CatalogConfigSection({ input, isMoE, gpuLayers, expertLayersOnGPU, kvCacheOnCPU, deviceCount, tensorSplit, modelUrl }: {
  input: VRAMInput;
  isMoE: boolean;
  gpuLayers?: number;
  expertLayersOnGPU?: number;
  kvCacheOnCPU?: boolean;
  deviceCount?: number;
  tensorSplit?: string;
  modelUrl?: string;
}) {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const config = buildComputedCatalogConfig(input, isMoE, gpuLayers, expertLayersOnGPU, kvCacheOnCPU, deviceCount, tensorSplit);
  const yaml = configToYAML(config);

  const handleExport = () => {
    const url = modelUrl?.trim();
    if (!url) return;

    const draft: VramCatalogDraft = {
      source: 'vram',
      modelUrl: url,
      config,
    };

    sessionStorage.setItem('kronk_catalog_draft', JSON.stringify(draft));
    navigate('/catalog/editor?source=vram');
  };

  return (
    <div style={{ marginTop: '0px', padding: '0px 12px 25px 0px', background: 'var(--color-gray-50)', borderRadius: '6px' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
        <button
          type="button"
          onClick={() => setOpen(!open)}
          aria-expanded={open}
          aria-controls="computed-catalog-config"
          style={{
            background: 'none',
            border: 'none',
            padding: 0,
            cursor: 'pointer',
            fontSize: '14px',
            fontWeight: 600,
            color: 'var(--color-text)',
            display: 'flex',
            alignItems: 'center',
            gap: '6px',
          }}
        >
          <span style={{ display: 'inline-block', transition: 'transform 0.2s', transform: open ? 'rotate(90deg)' : 'rotate(0deg)', fontSize: '12px' }}>▶</span>
          Computed Catalog Configuration
        </button>
        {modelUrl?.trim() && (
          <button
            type="button"
            className="btn btn-secondary"
            style={{ fontSize: '0.8em', padding: '4px 10px' }}
            onClick={handleExport}
          >
            Export to Catalog Editor
          </button>
        )}
      </div>
      {open && (
        <pre id="computed-catalog-config" style={{
          marginTop: '8px',
          padding: '12px',
          background: 'var(--color-gray-100)',
          borderRadius: '6px',
          fontSize: '0.85em',
          overflow: 'auto',
          whiteSpace: 'pre',
        }}>
          {yaml}
        </pre>
      )}
    </div>
  );
}
