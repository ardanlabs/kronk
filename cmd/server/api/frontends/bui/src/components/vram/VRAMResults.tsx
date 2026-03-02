import KeyValueTable from '../KeyValueTable';
import { formatBytes } from '../../lib/format';
import type { VRAMInput } from '../../types';

interface VRAMResultsProps {
  totalVram: number;
  slotMemory: number;
  kvPerSlot: number;
  kvPerTokenPerLayer: number;
  input: VRAMInput;
}

export default function VRAMResults({
  totalVram,
  slotMemory,
  kvPerSlot,
  kvPerTokenPerLayer,
  input,
}: VRAMResultsProps) {
  const breakdownRows = [
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

  return (
    <div className="vram-results">
      <div className="vram-hero">
        <div className="vram-hero-label">Total VRAM Required</div>
        <div className="vram-hero-value">{formatBytes(totalVram)}</div>
      </div>

      <div className="vram-breakdown">
        <div>
          <h4 className="vram-breakdown-title">Breakdown</h4>
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
