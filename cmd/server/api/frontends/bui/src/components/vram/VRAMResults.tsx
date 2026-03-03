import KeyValueTable from '../KeyValueTable';
import { formatBytes } from '../../lib/format';
import type { VRAMInput, MoEInfo, WeightBreakdown } from '../../types';

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
}: VRAMResultsProps) {
  const isMoE = moe?.is_moe === true && weights != null;

  const breakdownRows = isMoE
    ? [
        { label: 'Always-Active Weights (GPU)', value: formatBytes(weights!.always_active_bytes) },
        {
          label: `Expert Weights — GPU (${expertLayersOnGPU ?? 0} layers)`,
          value: formatBytes((modelWeightsGPU ?? 0) - weights!.always_active_bytes),
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
          </div>
        )}
      </div>

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
    </div>
  );
}
