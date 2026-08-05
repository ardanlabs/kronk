import type { VRAM } from '../../types';

// ── MoE display helpers ─────────────────────────────────────────────────────

export function isMoeModel(vram?: VRAM | null, metadata?: Record<string, string>): boolean {
  return vram?.moe?.is_moe === true
    || (!!metadata && Object.keys(metadata).some(k => k.endsWith('.expert_count')));
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
