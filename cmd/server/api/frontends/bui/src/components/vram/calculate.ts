import type { VRAMInput } from '../../types';

export interface VRAMResult {
  kvPerTokenPerLayer: number;
  kvPerSlot: number;
  slotMemory: number;
  totalVram: number;
}

export function calculateVRAM(input: VRAMInput): VRAMResult {
  const kvPerTokenPerLayer = input.head_count_kv * (input.key_length + input.value_length) * input.bytes_per_element;
  const kvPerSlot = input.context_window * input.block_count * kvPerTokenPerLayer;
  const slotMemory = input.slots * kvPerSlot;
  const totalVram = input.model_size_bytes + slotMemory;

  return { kvPerTokenPerLayer, kvPerSlot, slotMemory, totalVram };
}
