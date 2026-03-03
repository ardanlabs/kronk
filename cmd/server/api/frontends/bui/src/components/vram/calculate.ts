import type { VRAMInput, WeightBreakdown, MoEInfo } from '../../types';

export interface VRAMResult {
  kvPerTokenPerLayer: number;
  kvPerSlot: number;
  slotMemory: number;
  totalVram: number;
  modelWeightsGPU: number;
  modelWeightsCPU: number;
  computeBufferEst: number;
}

export function calculateVRAM(
  input: VRAMInput,
  weights?: WeightBreakdown | null,
  moe?: MoEInfo | null,
  expertLayersOnGPU?: number,
): VRAMResult {
  const kvPerTokenPerLayer = input.head_count_kv * (input.key_length + input.value_length) * input.bytes_per_element;
  const kvPerSlot = input.context_window * input.block_count * kvPerTokenPerLayer;
  const slotMemory = input.slots * kvPerSlot;

  let modelWeightsGPU: number;
  let modelWeightsCPU: number;

  if (weights && moe?.is_moe) {
    const alwaysActiveGPU = weights.always_active_bytes;
    let expertsGPU = 0;

    const layersOnGPU = expertLayersOnGPU ?? 0;
    if (layersOnGPU > 0 && weights.expert_bytes_by_layer?.length > 0) {
      const blockCount = weights.expert_bytes_by_layer.length;
      const startLayer = Math.max(0, blockCount - layersOnGPU);
      for (let i = startLayer; i < blockCount; i++) {
        expertsGPU += weights.expert_bytes_by_layer[i];
      }
    }

    modelWeightsGPU = alwaysActiveGPU + expertsGPU;
    modelWeightsCPU = weights.expert_bytes_total - expertsGPU;
  } else {
    modelWeightsGPU = input.model_size_bytes;
    modelWeightsCPU = 0;
  }

  // Compute buffer estimate (heuristic).
  const baseBuffer = input.model_size_bytes > 50 * 1024 * 1024 * 1024
    ? 512 * 1024 * 1024
    : 256 * 1024 * 1024;

  let embeddingComponent = 0;
  const embLen = input.embedding_length ?? 0;
  if (embLen > 0) {
    embeddingComponent = 8 * 512 * embLen * 4;
  }

  const computeBufferEst = Math.round((baseBuffer + embeddingComponent) * 1.1);

  const totalVram = modelWeightsGPU + slotMemory + computeBufferEst;

  return { kvPerTokenPerLayer, kvPerSlot, slotMemory, totalVram, modelWeightsGPU, modelWeightsCPU, computeBufferEst };
}
