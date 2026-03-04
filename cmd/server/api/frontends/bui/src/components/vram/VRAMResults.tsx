import KeyValueTable from '../KeyValueTable';
import { formatBytes } from '../../lib/format';
import type { VRAMInput, MoEInfo, WeightBreakdown, PerDeviceVRAM } from '../../types';

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
  perDevice?: PerDeviceVRAM[];
  deviceCount?: number;
  systemRAMBytes?: number;
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
  perDevice,
  deviceCount,
  systemRAMBytes,
}: VRAMResultsProps) {
  const isMoE = moe?.is_moe === true && weights != null;

  const breakdownRows = isMoE
    ? [
        { label: 'Always-Active Weights (GPU)', value: formatBytes(weights!.always_active_bytes) },
        {
          label: `Expert Weights — GPU (${expertLayersOnGPU ?? 0} layers)`,
          value: formatBytes(Math.max(0, (modelWeightsGPU ?? 0) - weights!.always_active_bytes)),
        },
        { label: 'Expert Weights — CPU', value: formatBytes(modelWeightsCPU ?? 0) },
        { label: 'KV Cache (Slot Memory)', value: formatBytes(slotMemory) },
        { label: 'Compute Buffer (estimate)', value: `~${formatBytes(computeBufferEst ?? 0)}` },
      ]
    : [
        { label: 'Model Weights', value: formatBytes(input.model_size_bytes) },
        { label: 'Slot Memory (KV Cache)', value: formatBytes(slotMemory) },
        { label: 'KV Per Slot', value: formatBytes(kvPerSlot) },
        { label: 'KV Per Token Per Layer', value: formatBytes(kvPerTokenPerLayer) },
      ];

  const headerRows = [
    { label: 'Model Size', value: formatBytes(input.model_size_bytes) },
    { label: 'Layers (Block Count)', value: String(input.block_count) },
    { label: 'Head Count KV', value: String(input.head_count_kv) },
    { label: 'Key Length', value: String(input.key_length) },
    { label: 'Value Length', value: String(input.value_length) },
  ];

  if (isMoE) {
    headerRows.push(
      { label: 'Expert Count', value: String(moe!.expert_count) },
      { label: 'Active Experts (top-k)', value: String(moe!.expert_used_count) },
    );
    if (moe!.has_shared_experts) {
      headerRows.push({ label: 'Shared Experts', value: 'Yes' });
    }
  }

  return (
    <div className="vram-results">
      <div className="vram-hero">
        <div className="vram-hero-label">Total Estimated VRAM</div>
        <div className="vram-hero-value">{formatBytes(totalVram)}</div>
        {isMoE && (
          <div className="vram-hero-subtitle" style={{ fontSize: '0.85em', opacity: 0.7, marginTop: '4px' }}>
            {formatBytes(modelWeightsCPU ?? 0)} on CPU
            {systemRAMBytes != null && systemRAMBytes > 0 && (
              <span> / {formatBytes(systemRAMBytes)} system RAM</span>
            )}
          </div>
        )}
      </div>

      {isMoE && systemRAMBytes != null && systemRAMBytes > 0 && (modelWeightsCPU ?? 0) > 0 && (
        <div style={{ marginTop: '12px', marginBottom: '12px' }}>
          <h4 className="vram-breakdown-title">System RAM Usage (estimated)</h4>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.85em', marginBottom: '2px' }}>
            <span>Expert weights on CPU</span>
            <span>{formatBytes(modelWeightsCPU ?? 0)} / {formatBytes(systemRAMBytes)}</span>
          </div>
          <div style={{ background: 'var(--color-gray-200)', borderRadius: '4px', height: '20px', overflow: 'hidden', display: 'flex' }}>
            <div
              style={{
                width: `${Math.min(100, ((modelWeightsCPU ?? 0) / systemRAMBytes) * 100)}%`,
                background: (modelWeightsCPU ?? 0) > systemRAMBytes ? '#ef5350' : (modelWeightsCPU ?? 0) > systemRAMBytes * 0.8 ? '#ffa726' : '#66bb6a',
                height: '100%',
                transition: 'width 0.3s ease',
              }}
              title={`CPU expert weights: ${formatBytes(modelWeightsCPU ?? 0)}`}
            />
          </div>
          <div style={{ fontSize: '0.75em', opacity: 0.7, marginTop: '4px' }}>
            {(modelWeightsCPU ?? 0) > systemRAMBytes
              ? '❌ Exceeds available RAM — increase expert layers on GPU or use smaller quantization'
              : (modelWeightsCPU ?? 0) > systemRAMBytes * 0.8
                ? '⚠️ Tight fit — limited headroom for OS and other processes'
                : '✅ Fits comfortably in system RAM'}
          </div>
        </div>
      )}

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

      {isMoE && systemRAMBytes != null && systemRAMBytes > 0 && (modelWeightsCPU ?? 0) > systemRAMBytes && (
        <div className="alert alert-error" style={{ marginTop: '12px', fontSize: '0.85em' }}>
          <strong>Warning:</strong> Expert weights on CPU ({formatBytes(modelWeightsCPU ?? 0)}) exceed available system RAM ({formatBytes(systemRAMBytes)}). Increase expert layers on GPU to move more to VRAM, or use a smaller quantization.
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

      {perDevice && perDevice.length > 1 && (() => {
        const maxBytes = Math.max(...perDevice.map(d => d.totalBytes), 1);
        return (
        <div style={{ marginTop: '16px' }}>
          <h4 className="vram-breakdown-title">Per-GPU VRAM Allocation (estimated)</h4>
          {perDevice.map((dev, i) => {
            const barWidth = (dev.totalBytes / maxBytes) * 100;
            const devTotal = dev.totalBytes || 1;
            return (
              <div key={i} style={{ marginBottom: '8px' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.85em', marginBottom: '2px' }}>
                  <span>{dev.label}</span>
                  <span>{formatBytes(dev.totalBytes)}</span>
                </div>
                <div style={{ background: 'var(--color-gray-200)', borderRadius: '4px', height: '20px', overflow: 'hidden', display: 'flex', width: `${barWidth}%` }}>
                  {dev.weightsBytes > 0 && (
                    <div style={{ width: `${(dev.weightsBytes / devTotal) * 100}%`, background: 'var(--color-blue)', height: '100%' }} title={`Weights: ${formatBytes(dev.weightsBytes)}`} />
                  )}
                  {dev.kvBytes > 0 && (
                    <div style={{ width: `${(dev.kvBytes / devTotal) * 100}%`, background: 'var(--color-orange)', height: '100%' }} title={`KV Cache: ${formatBytes(dev.kvBytes)}`} />
                  )}
                  {dev.computeBytes > 0 && (
                    <div style={{ width: `${(dev.computeBytes / devTotal) * 100}%`, background: '#8b5cf6', height: '100%' }} title={`Compute Buffer: ${formatBytes(dev.computeBytes)}`} />
                  )}
                </div>
              </div>
            );
          })}
          <div style={{ display: 'flex', gap: '12px', fontSize: '0.75em', opacity: 0.7, marginTop: '4px' }}>
            <span>■ Weights</span>
            <span style={{ color: 'var(--color-orange)' }}>■ KV Cache</span>
            <span style={{ color: '#8b5cf6' }}>■ Compute</span>
          </div>
          <div className="alert alert-info" style={{ marginTop: '8px', fontSize: '0.85em' }}>
            <strong>Note:</strong> Per-GPU allocation is estimated based on tensor split proportions. Actual distribution may vary depending on llama.cpp split mode behavior.
          </div>
        </div>
        );
      })()}

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
