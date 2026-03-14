import { useMemo } from 'react';
import type { VRAM } from '../../types';
import type { DevicesInfo } from './devices';

// ── MoE detection helpers ───────────────────────────────────────────────────

export function isMoeModel(vram?: VRAM | null, metadata?: Record<string, string>): boolean {
  return vram?.moe?.is_moe === true
    || (!!metadata && Object.keys(metadata).some(k => k.endsWith('.expert_count')));
}

// ── MoE VRAM fit computation ────────────────────────────────────────────────

export type VramFitStatus = 'fits' | 'tight' | 'wont_fit';

export interface VramFitResult {
  status: VramFitStatus | null;
  allGPU: number;
  cpuExperts: number;
}

export interface VramFitOverrides {
  contextWindow?: number;
  slots?: number;
  bytesPerElement?: number;
}

// ── MoE fit context ─────────────────────────────────────────────────────────

export interface MoeFitContext {
  isMoe: boolean;
  fit: VramFitResult | null;
}

export const DEFAULT_COMPUTE_BUFFER_EST_BYTES = 300 * 1024 * 1024;
export const VRAM_FIT_THRESHOLD = 0.95;

export const VRAM_FIT_TEXT: Record<VramFitStatus, string> = {
  fits: '✅ Fits in VRAM',
  tight: '⚠️ Experts won\'t fit — CPU offload recommended',
  wont_fit: '❌ Tight even with CPU experts',
};

// ── MoE mode labels ─────────────────────────────────────────────────────────

export const MOE_STRATEGY_OPTIONS = [
  { value: '', label: '🟢 Recommended' },
  { value: 'experts_cpu', label: '💾 Save GPU Memory — experts on CPU' },
  { value: 'keep_top_n', label: '⚖️ Balanced — keep some experts on GPU' },
  { value: 'experts_gpu', label: '⚡ Maximum Speed — all on GPU' },
  { value: 'custom', label: '🔧 Advanced' },
] as const;

export const MOE_SWEEP_LABELS: Record<string, string> = {
  experts_cpu: '💾 Save GPU Memory',
  keep_top_n: '⚖️ Balanced',
  experts_gpu: '⚡ Maximum Speed',
};

// ── Core fit computation ────────────────────────────────────────────────────

function computeSlotMemory(vramInfo: VRAM, overrides?: VramFitOverrides): number {
  const ctxWindow = overrides?.contextWindow ?? vramInfo.input.context_window;
  const slots = overrides?.slots ?? vramInfo.input.slots;
  const bpe = overrides?.bytesPerElement ?? vramInfo.input.bytes_per_element;

  const kvPerSlot =
    ctxWindow *
    vramInfo.input.block_count *
    vramInfo.input.head_count_kv *
    (vramInfo.input.key_length + vramInfo.input.value_length) *
    bpe;

  return kvPerSlot * slots;
}

export function computeMoeFit(
  vramInfo: VRAM,
  gpuVramBytes: number,
  overrides?: VramFitOverrides,
): VramFitResult {
  const slotMem = computeSlotMemory(vramInfo, overrides);
  const computeEst = vramInfo.compute_buffer_est ?? DEFAULT_COMPUTE_BUFFER_EST_BYTES;

  const allGPU = vramInfo.input.model_size_bytes + slotMem + computeEst;

  const activeOnly =
    vramInfo.weights?.always_active_bytes ?? vramInfo.input.model_size_bytes;
  const cpuExperts = activeOnly + slotMem + computeEst;

  if (gpuVramBytes <= 0) {
    return { status: null, allGPU, cpuExperts };
  }

  if (allGPU <= gpuVramBytes * VRAM_FIT_THRESHOLD) {
    return { status: 'fits', allGPU, cpuExperts };
  }

  if (cpuExperts <= gpuVramBytes * VRAM_FIT_THRESHOLD) {
    return { status: 'tight', allGPU, cpuExperts };
  }

  return { status: 'wont_fit', allGPU, cpuExperts };
}

// ── Per-device fit evaluation ───────────────────────────────────────────────

function evaluateFitStatus(
  allGPU: number,
  cpuExperts: number,
  devices: DevicesInfo,
): VramFitStatus | null {
  const gpuDevices = devices.gpuDevices;

  // Per-device check: when we have individual GPU info, check that the
  // per-device share fits on the smallest GPU. This assumes equal tensor
  // splitting; actual llama.cpp distribution may use proportional splits
  // based on VRAM, so this is a conservative heuristic.
  if (gpuDevices.length > 1) {
    const perDeviceAllGPU = allGPU / gpuDevices.length;
    const perDeviceCpuExperts = cpuExperts / gpuDevices.length;
    const smallestTotal = Math.min(...gpuDevices.map(d => d.total_bytes));

    if (smallestTotal <= 0) return null;

    if (perDeviceAllGPU <= smallestTotal * VRAM_FIT_THRESHOLD) return 'fits';
    if (perDeviceCpuExperts <= smallestTotal * VRAM_FIT_THRESHOLD) return 'tight';
    return 'wont_fit';
  }

  // Single GPU or aggregate check.
  const capacity = devices.gpuVramBytes;
  if (capacity <= 0) return null;

  if (allGPU <= capacity * VRAM_FIT_THRESHOLD) return 'fits';
  if (cpuExperts <= capacity * VRAM_FIT_THRESHOLD) return 'tight';
  return 'wont_fit';
}

export function computeMoeFitWithDevices(
  vramInfo: VRAM,
  devices: DevicesInfo,
  overrides?: VramFitOverrides,
): VramFitResult {
  const slotMem = computeSlotMemory(vramInfo, overrides);
  const computeEst = vramInfo.compute_buffer_est ?? DEFAULT_COMPUTE_BUFFER_EST_BYTES;

  const allGPU = vramInfo.input.model_size_bytes + slotMem + computeEst;

  const activeOnly =
    vramInfo.weights?.always_active_bytes ?? vramInfo.input.model_size_bytes;
  const cpuExperts = activeOnly + slotMem + computeEst;

  const status = evaluateFitStatus(allGPU, cpuExperts, devices);
  return { status, allGPU, cpuExperts };
}

// ── Shared MoE fit hook ─────────────────────────────────────────────────────

export function useMoeFit(
  vram: VRAM | null | undefined,
  metadata: Record<string, string> | undefined,
  devices: DevicesInfo | null,
  overrides?: VramFitOverrides,
): MoeFitContext {
  return useMemo(() => {
    const isMoe = isMoeModel(vram, metadata);
    if (!isMoe || !vram || !devices) {
      return { isMoe, fit: null };
    }
    const fit = computeMoeFitWithDevices(vram, devices, overrides);
    return { isMoe, fit };
  }, [vram, metadata, devices, overrides?.contextWindow, overrides?.slots, overrides?.bytesPerElement]);
}
