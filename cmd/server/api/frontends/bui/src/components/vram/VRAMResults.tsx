import { useState, type ReactNode } from 'react';
import KeyValueTable from '../KeyValueTable';
import { formatBytes } from '../../lib/format';
import { PARAM_TOOLTIPS, ParamTooltip } from '../ParamTooltips';
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
}: VRAMResultsProps) {
  const isMoE = moe?.is_moe === true && weights != null;
  const kvOnCPU = kvCacheOnCPU ?? false;
  const kvCacheLocation = kvOnCPU ? 'System RAM' : 'GPU';

  const breakdownRows = isMoE
    ? [
        { label: <>Always-Active Weights (GPU)<ParamTooltip text={PARAM_TOOLTIPS.alwaysActiveWeights} /></>, value: formatBytes(weights!.always_active_bytes) },
        {
          label: <>Expert Weights — GPU ({expertLayersOnGPU ?? 0} layers)<ParamTooltip text={PARAM_TOOLTIPS.expertWeightsGPU} /></>,
          value: formatBytes(Math.max(0, (modelWeightsGPU ?? 0) - weights!.always_active_bytes)),
        },
        { label: <>Expert Weights — CPU<ParamTooltip text={PARAM_TOOLTIPS.expertWeightsCPU} /></>, value: formatBytes(modelWeightsCPU ?? 0) },
        { label: <>KV Cache ({kvCacheLocation})<ParamTooltip text={PARAM_TOOLTIPS.kvCache} /></>, value: formatBytes(slotMemory) },
        { label: <>Compute Buffer (estimate)<ParamTooltip text={PARAM_TOOLTIPS.computeBuffer} /></>, value: `~${formatBytes(computeBufferEst ?? 0)}` },
      ]
    : [
        { label: <>Model Weights<ParamTooltip text={PARAM_TOOLTIPS.modelWeights} /></>, value: formatBytes(input.model_size_bytes) },
        { label: <>KV Cache ({kvCacheLocation})<ParamTooltip text={PARAM_TOOLTIPS.kvCache} /></>, value: formatBytes(slotMemory) },
        { label: <>KV Per Slot<ParamTooltip text={PARAM_TOOLTIPS.kvPerSlot} /></>, value: formatBytes(kvPerSlot) },
        { label: <>KV Per Token Per Layer<ParamTooltip text={PARAM_TOOLTIPS.kvPerTokenPerLayer} /></>, value: formatBytes(kvPerTokenPerLayer) },
        { label: <>Compute Buffer (estimate)<ParamTooltip text={PARAM_TOOLTIPS.computeBuffer} /></>, value: `~${formatBytes(computeBufferEst ?? 0)}` },
      ];

  const headerRows: { label: ReactNode; value: string }[] = [
    { label: <>Model Size<ParamTooltip text={PARAM_TOOLTIPS.modelSize} /></>, value: formatBytes(input.model_size_bytes) },
    { label: <>Layers (Block Count)<ParamTooltip text={PARAM_TOOLTIPS.blockCount} /></>, value: String(input.block_count) },
    { label: <>Head Count KV<ParamTooltip text={PARAM_TOOLTIPS.headCountKV} /></>, value: String(input.head_count_kv) },
    { label: <>Key Length<ParamTooltip text={PARAM_TOOLTIPS.keyLength} /></>, value: String(input.key_length) },
    { label: <>Value Length<ParamTooltip text={PARAM_TOOLTIPS.valueLength} /></>, value: String(input.value_length) },
  ];

  if (isMoE) {
    headerRows.push(
      { label: <>Expert Count<ParamTooltip text={PARAM_TOOLTIPS.expertCount} /></>, value: String(moe!.expert_count) },
      { label: <>Active Experts (top-k)<ParamTooltip text={PARAM_TOOLTIPS.activeExperts} /></>, value: String(moe!.expert_used_count) },
    );
    if (moe!.has_shared_experts) {
      headerRows.push({ label: <>Shared Experts<ParamTooltip text={PARAM_TOOLTIPS.sharedExperts} /></>, value: 'Yes' });
    }
  }

  const systemRamUsed = (totalSystemRamEst ?? (modelWeightsCPU ?? 0) + (kvCpuBytes ?? 0));
  const showSystemRAM = systemRamUsed > 0;

  return (
    <div className="vram-results">
      <div className="vram-hero" style={{ display: 'flex', gap: '32px', flexWrap: 'wrap' }}>
        <div style={{ flex: 1, minWidth: '180px' }}>
          <div className="vram-hero-label">Total Estimated VRAM<ParamTooltip text={PARAM_TOOLTIPS.totalEstimatedVRAM} /></div>
          <div className="vram-hero-value">
            {formatBytes(totalVram)}
            {gpuTotalBytes != null && gpuTotalBytes > 0 && (
              <span style={{ fontSize: '0.55em', opacity: 0.5 }}> / {formatBytes(gpuTotalBytes)}</span>
            )}
          </div>
        </div>
        {showSystemRAM && systemRAMBytes != null && systemRAMBytes > 0 && (
          <div style={{ minWidth: '180px' }}>
            <div className="vram-hero-label">Total Estimated System RAM<ParamTooltip text={PARAM_TOOLTIPS.totalEstimatedSystemRAM} /></div>
            <div className="vram-hero-value">
              {formatBytes(systemRamUsed)}
              <span style={{ fontSize: '0.55em', opacity: 0.5 }}> / {formatBytes(systemRAMBytes)}</span>
            </div>
          </div>
        )}
      </div>

      {perDevice && perDevice.length > 1 && (() => {
        return (
        <div style={{ marginTop: '16px' }}>
          <h4 className="vram-breakdown-title">Per-GPU VRAM Allocation (estimated)</h4>
          {perDevice.map((dev, i) => {
            const gpuCapacity = gpuDevices?.[i]?.total_bytes ?? 0;
            const barMax = Math.max(1, gpuCapacity > 0 ? gpuCapacity : dev.totalBytes);
            const freeBytes = Math.max(0, barMax - dev.totalBytes);
            const overcommit = dev.totalBytes > barMax && gpuCapacity > 0;
            return (
              <div key={i} style={{ marginBottom: '8px' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.85em', marginBottom: '2px' }}>
                  <span>{dev.label}</span>
                  <span>
                    {formatBytes(dev.totalBytes)}
                    {gpuCapacity > 0 && <span style={{ opacity: 0.6 }}> / {formatBytes(gpuCapacity)}</span>}
                  </span>
                </div>
                <div style={{ background: 'var(--color-gray-200)', borderRadius: '4px', height: '20px', overflow: 'hidden', display: 'flex' }}>
                  {dev.weightsBytes > 0 && (
                    <div style={{ width: `${(dev.weightsBytes / barMax) * 100}%`, background: 'var(--color-blue)', height: '100%' }} title={`Weights: ${formatBytes(dev.weightsBytes)}`} />
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
                    ⚠ Exceeds GPU capacity by {formatBytes(dev.totalBytes - gpuCapacity)}
                  </div>
                )}
              </div>
            );
          })}
          <div style={{ display: 'flex', gap: '12px', fontSize: '0.75em', opacity: 0.7, marginTop: '4px' }}>
            <span>■ Weights</span>
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

      {showSystemRAM && systemRAMBytes != null && systemRAMBytes > 0 && (
        <div style={{ marginTop: '12px', marginBottom: '12px' }}>
          <h4 className="vram-breakdown-title">System RAM Usage (estimated)</h4>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.85em', marginBottom: '2px' }}>
            <span>
              {(modelWeightsCPU ?? 0) > 0 && (kvCpuBytes ?? 0) > 0
                ? 'Expert weights + KV cache on CPU'
                : (kvCpuBytes ?? 0) > 0
                  ? 'KV cache on CPU'
                  : 'Expert weights on CPU'}
            </span>
            <span>{formatBytes(systemRamUsed)} / {formatBytes(systemRAMBytes)}</span>
          </div>
          {(() => {
            const barMax = Math.max(1, Math.max(systemRAMBytes, systemRamUsed));
            const overcommit = systemRamUsed > systemRAMBytes;
            const freeBytes = Math.max(0, systemRAMBytes - systemRamUsed);
            return (
              <div style={{ background: 'var(--color-gray-200)', borderRadius: '4px', height: '20px', overflow: 'hidden', display: 'flex' }}>
                {(modelWeightsCPU ?? 0) > 0 && (
                  <div
                    style={{
                      width: `${((modelWeightsCPU ?? 0) / barMax) * 100}%`,
                      background: 'var(--color-blue)',
                      height: '100%',
                    }}
                    title={`Expert weights: ${formatBytes(modelWeightsCPU ?? 0)}`}
                  />
                )}
                {(kvCpuBytes ?? 0) > 0 && (
                  <div
                    style={{
                      width: `${((kvCpuBytes ?? 0) / barMax) * 100}%`,
                      background: 'var(--color-orange)',
                      height: '100%',
                    }}
                    title={`KV cache: ${formatBytes(kvCpuBytes ?? 0)}`}
                  />
                )}
                {freeBytes > 0 && !overcommit && (
                  <div style={{ flex: 1, background: '#66bb6a', height: '100%' }} title={`Free: ${formatBytes(freeBytes)}`} />
                )}
              </div>
            );
          })()}
          {(modelWeightsCPU ?? 0) > 0 && (kvCpuBytes ?? 0) > 0 && (
            <div style={{ display: 'flex', gap: '12px', fontSize: '0.75em', opacity: 0.7, marginTop: '4px' }}>
              <span>■ Expert Weights</span>
              <span style={{ color: 'var(--color-orange)' }}>■ KV Cache</span>
            </div>
          )}
          <div style={{ fontSize: '0.75em', opacity: 0.7, marginTop: '4px' }}>
            {systemRamUsed > systemRAMBytes
              ? '❌ Exceeds available RAM — reduce context window, increase expert layers on GPU, or use smaller quantization'
              : systemRamUsed > systemRAMBytes * 0.8
                ? '⚠️ Tight fit — limited headroom for OS and other processes'
                : '✅ Fits comfortably in system RAM'}
          </div>
        </div>
      )}

      {kvOnCPU && (
        <div className="alert alert-info" style={{ marginTop: '12px', fontSize: '0.85em' }}>
          <strong>KV Cache on CPU:</strong>
          <ul style={{ margin: '4px 0', paddingLeft: '20px' }}>
            <li><strong>Discrete GPUs (CUDA/ROCm/Vulkan):</strong> Expect significantly lower tokens/sec during generation due to PCIe bandwidth bottleneck — often 2-5× slower</li>
            <li><strong>Apple Silicon (Metal):</strong> Impact is much smaller due to unified memory — no PCIe transfer needed</li>
            <li>Consider KV cache quantization (q8_0) as a less costly alternative to reduce VRAM usage</li>
          </ul>
        </div>
      )}

      <CatalogConfigSection
        input={input}
        isMoE={isMoE}
        expertLayersOnGPU={expertLayersOnGPU}
        kvCacheOnCPU={kvOnCPU}
        deviceCount={deviceCount}
        tensorSplit={tensorSplit}
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

      {showSystemRAM && systemRAMBytes != null && systemRAMBytes > 0 && systemRamUsed > systemRAMBytes && (
        <div className="alert alert-error" style={{ marginTop: '12px', fontSize: '0.85em' }}>
          <strong>Warning:</strong> Estimated system RAM usage ({formatBytes(systemRamUsed)}) exceeds available RAM ({formatBytes(systemRAMBytes)}).
          {kvOnCPU && ' Disable KV Cache on CPU, reduce context window, or use KV cache quantization.'}
          {isMoE && ' Increase expert layers on GPU or use a smaller quantization.'}
        </div>
      )}

      {isMoE && (
        <div className="alert alert-info" style={{ marginTop: '12px', fontSize: '0.85em' }}>
          <strong>MoE Tips:</strong>
          <ul style={{ margin: '4px 0', paddingLeft: '20px' }}>
            <li>For MoE with CPU experts, NBatch/NUBatch ≥ 4096 is recommended</li>
            <li>Flash Attention is strongly recommended for MoE models</li>
            <li>Larger NUBatch increases compute buffer VRAM usage</li>
          </ul>
        </div>
      )}

      {(deviceCount ?? 1) > 1 && isMoE && (
        <div className="alert alert-info" style={{ marginTop: '12px', fontSize: '0.85em' }}>
          <strong>MoE Multi-GPU Tips:</strong>
          <ul style={{ margin: '4px 0', paddingLeft: '20px' }}>
            <li>For MoE models, <strong>split-mode: row</strong> (tensor parallelism) is recommended</li>
            <li>If experts are on CPU with 2+ GPUs, <strong>split-mode: layer</strong> gives simpler behavior unless chasing throughput</li>
            <li>MainGPU should be the GPU with highest PCIe bandwidth for prompt processing offload</li>
          </ul>
        </div>
      )}
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

function buildCatalogYAML(
  input: VRAMInput,
  isMoE: boolean,
  expertLayersOnGPU?: number,
  kvCacheOnCPU?: boolean,
  deviceCount?: number,
  tensorSplit?: string,
): string {
  const lines: string[] = [];
  lines.push('model-name/variant:');
  lines.push(`  context-window: ${input.context_window}`);
  lines.push(`  nseq-max: ${input.slots}`);

  const cacheType = cacheTypeName(input.bytes_per_element);
  lines.push(`  cache-type-k: ${cacheType}`);
  lines.push(`  cache-type-v: ${cacheType}`);

  lines.push('  flash-attention: enabled');

  if (kvCacheOnCPU) {
    lines.push('  offload-kqv: false');
  }

  const gpuCount = deviceCount ?? 1;

  if (isMoE) {
    const layers = expertLayersOnGPU ?? 0;
    const allOnGPU = layers >= input.block_count;
    if (!allOnGPU) {
      lines.push('  moe:');
      if (layers > 0) {
        lines.push('    mode: keep_top_n');
        lines.push(`    keep-experts-top-n: ${layers}`);
      } else {
        lines.push('    mode: experts_cpu');
      }
    }
  }

  if (gpuCount > 1) {
    const nums = tensorSplit
      ?.split(',')
      .map(s => parseFloat(s.trim()))
      .filter(n => !isNaN(n)) ?? [];
    if (nums.length > 0) {
      lines.push(`  tensor-split: [${nums.join(', ')}]`);
    }
    if (isMoE) {
      lines.push('  split-mode: row');
    }
  }

  return lines.join('\n');
}

function CatalogConfigSection({ input, isMoE, expertLayersOnGPU, kvCacheOnCPU, deviceCount, tensorSplit }: {
  input: VRAMInput;
  isMoE: boolean;
  expertLayersOnGPU?: number;
  kvCacheOnCPU?: boolean;
  deviceCount?: number;
  tensorSplit?: string;
}) {
  const [open, setOpen] = useState(false);
  const yaml = buildCatalogYAML(input, isMoE, expertLayersOnGPU, kvCacheOnCPU, deviceCount, tensorSplit);

  return (
    <div style={{ marginTop: '0px', padding: '0px 12px 25px 0px', background: 'var(--color-gray-50)', borderRadius: '6px' }}>
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
