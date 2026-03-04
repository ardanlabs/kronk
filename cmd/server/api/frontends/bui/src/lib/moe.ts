import type { VRAM } from '../types';

// ── VRAM fit computation ────────────────────────────────────────────────────

export type VramFitStatus = 'fits' | 'tight' | 'wont_fit';

export interface DevicesInfo {
  gpuCount: number;
  gpuType: string;
  gpuVramBytes: number;
  ramBytes: number;
}

export interface VramFitResult {
  status: VramFitStatus | null;
  allGPU: number;
  cpuExperts: number;
}

export interface VramFitOverrides {
  contextWindow?: number;
  slots?: number;
}

export function computeMoeVramFit(vramInfo: VRAM, gpuVramBytes: number, overrides?: VramFitOverrides): VramFitResult {
  const ctxWindow = overrides?.contextWindow ?? vramInfo.input.context_window;
  const slots = overrides?.slots ?? vramInfo.input.slots;
  const kvPerSlot = ctxWindow * vramInfo.input.block_count *
    vramInfo.input.head_count_kv * (vramInfo.input.key_length + vramInfo.input.value_length) * vramInfo.input.bytes_per_element;
  const slotMem = kvPerSlot * slots;
  const computeEst = vramInfo.compute_buffer_est ?? 300 * 1024 * 1024;
  const allGPU = vramInfo.input.model_size_bytes + slotMem + computeEst;

  const activeOnly = vramInfo.weights?.always_active_bytes ?? vramInfo.input.model_size_bytes;
  const cpuExperts = activeOnly + slotMem + computeEst;

  if (gpuVramBytes <= 0) {
    return { status: null, allGPU, cpuExperts };
  }

  let status: VramFitStatus;
  if (allGPU <= gpuVramBytes * 0.95) {
    status = 'fits';
  } else if (cpuExperts <= gpuVramBytes * 0.95) {
    status = 'tight';
  } else {
    status = 'wont_fit';
  }

  return { status, allGPU, cpuExperts };
}

// ── Shared device info parsing ──────────────────────────────────────────────

export function parseDevicesInfo(resp: { devices: Array<{ type: string }>; gpu_count: number; gpu_total_bytes: number; system_ram_bytes: number }): DevicesInfo {
  const gpuDevices = resp.devices.filter(d => d.type !== 'cpu' && d.type !== 'unknown');
  const gpuType = gpuDevices.length > 0
    ? gpuDevices[0].type.replace('gpu_', '').replace('cuda', 'CUDA').replace('metal', 'Metal').replace('rocm', 'ROCm').replace('vulkan', 'Vulkan')
    : '';
  return { gpuCount: resp.gpu_count, gpuType, gpuVramBytes: resp.gpu_total_bytes, ramBytes: resp.system_ram_bytes };
}

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

// ── Fit status display helpers ──────────────────────────────────────────────

export const VRAM_FIT_TEXT: Record<VramFitStatus, string> = {
  fits: '✅ Fits in VRAM',
  tight: '⚠️ Experts won\'t fit — CPU offload recommended',
  wont_fit: '❌ Tight even with CPU experts',
};
