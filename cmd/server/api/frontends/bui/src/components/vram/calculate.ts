import type { VRAMInput, WeightBreakdown, MoEInfo, PerDeviceVRAM } from '../../types';

function clampGpuLayers(gpuLayers: number | undefined, blockCount: number): number {
  if (!Number.isFinite(blockCount) || blockCount <= 0) return 0;
  return Math.max(0, Math.min(blockCount, gpuLayers ?? blockCount));
}

function splitByGpuLayers(totalBytes: number, gpuLayers: number, blockCount: number): { gpu: number; cpu: number } {
  if (blockCount <= 0) return { gpu: totalBytes, cpu: 0 };
  const gpu = Math.round((gpuLayers / blockCount) * totalBytes);
  return { gpu, cpu: Math.max(0, totalBytes - gpu) };
}

export interface VRAMResult {
  kvPerTokenPerLayer: number;
  kvPerSlot: number;
  slotMemory: number;
  kvVramBytes: number;
  kvCpuBytes: number;
  totalVram: number;
  totalSystemRamEst: number;
  modelWeightsGPU: number;
  modelWeightsCPU: number;
  computeBufferEst: number;
  /** For MoE: always-active (non-expert) bytes on GPU. */
  alwaysActiveGPUBytes: number;
  /** For MoE: always-active (non-expert) bytes on CPU. */
  alwaysActiveCPUBytes: number;
  /** For MoE: expert bytes on GPU. */
  expertGPUBytes: number;
  /** For MoE: expert bytes on CPU. */
  expertCPUBytes: number;
}

export interface CalculateVRAMOptions {
  weights?: WeightBreakdown | null;
  moe?: MoEInfo | null;
  gpuLayers?: number;
  expertLayersOnGPU?: number;
  kvCacheOnCPU?: boolean;
}

export function calculateVRAM(
  input: VRAMInput,
  opts: CalculateVRAMOptions = {},
): VRAMResult {
  const { weights, moe, expertLayersOnGPU, kvCacheOnCPU = false } = opts;

  const kvPerTokenPerLayer = input.head_count_kv * (input.key_length + input.value_length) * input.bytes_per_element;
  const kvPerSlot = input.context_window * input.block_count * kvPerTokenPerLayer;
  const slotMemory = input.slots * kvPerSlot;

  let modelWeightsGPU: number;
  let modelWeightsCPU: number;
  let alwaysActiveGPUBytes = 0;
  let alwaysActiveCPUBytes = 0;
  let expertGPUBytes = 0;
  let expertCPUBytes = 0;

  if (weights && moe?.is_moe) {
    const clamped = clampGpuLayers(opts.gpuLayers, input.block_count);
    if (clamped >= input.block_count) {
      alwaysActiveGPUBytes = weights.always_active_bytes;
      alwaysActiveCPUBytes = 0;
    } else {
      const split = splitByGpuLayers(weights.always_active_bytes, clamped, input.block_count);
      alwaysActiveGPUBytes = split.gpu;
      alwaysActiveCPUBytes = split.cpu;
    }

    const layersOnGPU = expertLayersOnGPU ?? 0;
    if (layersOnGPU > 0 && weights.expert_bytes_by_layer?.length > 0) {
      const blockCount = weights.expert_bytes_by_layer.length;
      const startLayer = Math.max(0, blockCount - layersOnGPU);
      for (let i = startLayer; i < blockCount; i++) {
        expertGPUBytes += weights.expert_bytes_by_layer[i];
      }
    }
    expertCPUBytes = Math.max(0, weights.expert_bytes_total - expertGPUBytes);

    modelWeightsGPU = alwaysActiveGPUBytes + expertGPUBytes;
    modelWeightsCPU = alwaysActiveCPUBytes + expertCPUBytes;
  } else {
    const clamped = clampGpuLayers(opts.gpuLayers, input.block_count);
    if (clamped >= input.block_count) {
      modelWeightsGPU = input.model_size_bytes;
      modelWeightsCPU = 0;
    } else {
      const split = splitByGpuLayers(input.model_size_bytes, clamped, input.block_count);
      modelWeightsGPU = split.gpu;
      modelWeightsCPU = split.cpu;
    }
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

  const total = baseBuffer + embeddingComponent;
  const computeBufferEst = total + Math.floor(total / 10);

  const kvVramBytes = kvCacheOnCPU ? 0 : slotMemory;
  const kvCpuBytes = kvCacheOnCPU ? slotMemory : 0;
  const totalVram = modelWeightsGPU + kvVramBytes + computeBufferEst;
  const totalSystemRamEst = modelWeightsCPU + kvCpuBytes;

  return { kvPerTokenPerLayer, kvPerSlot, slotMemory, kvVramBytes, kvCpuBytes, totalVram, totalSystemRamEst, modelWeightsGPU, modelWeightsCPU, computeBufferEst, alwaysActiveGPUBytes, alwaysActiveCPUBytes, expertGPUBytes, expertCPUBytes };
}

export function calculatePerDeviceVRAM(
  modelWeightsGPU: number,
  slotMemory: number,
  computeBufferEst: number,
  deviceCount: number,
  tensorSplit: number[],
  deviceLabels?: string[],
  mainGpuIndex = 0,
): PerDeviceVRAM[] {
  if (deviceCount <= 1) {
    return [{
      label: deviceLabels?.[0] ?? 'GPU 0 (main)',
      weightsBytes: modelWeightsGPU,
      kvBytes: slotMemory,
      computeBytes: computeBufferEst,
      totalBytes: modelWeightsGPU + slotMemory + computeBufferEst,
    }];
  }

  let fractions: number[];
  if (tensorSplit.length === deviceCount) {
    const sanitized = tensorSplit.map(v => (Number.isFinite(v) && v >= 0) ? v : 0);
    const sum = sanitized.reduce((a, b) => a + b, 0);
    fractions = sum > 0 ? sanitized.map(v => v / sum) : Array(deviceCount).fill(1 / deviceCount);
  } else {
    fractions = Array(deviceCount).fill(1 / deviceCount);
  }

  const out: PerDeviceVRAM[] = [];
  let wRemaining = modelWeightsGPU;
  let kvRemaining = slotMemory;

  for (let i = 0; i < deviceCount; i++) {
    const isLast = i === deviceCount - 1;
    const w = isLast ? wRemaining : Math.floor(modelWeightsGPU * fractions[i]);
    const kv = isLast ? kvRemaining : Math.floor(slotMemory * fractions[i]);
    wRemaining -= w;
    kvRemaining -= kv;

    const comp = i === mainGpuIndex ? computeBufferEst : 0;
    const isMain = i === mainGpuIndex;
    out.push({
      label: deviceLabels?.[i] ?? `GPU ${i}${isMain ? ' (main)' : ''}`,
      weightsBytes: w,
      kvBytes: kv,
      computeBytes: comp,
      totalBytes: w + kv + comp,
    });
  }

  return out;
}
