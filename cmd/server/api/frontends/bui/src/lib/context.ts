// Context window helpers — extract native and max context from GGUF metadata.

function fmtCompact(n: number): string {
  if (n >= 1_000_000) return `${+(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${+(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

export interface ContextInfo {
  native: number;
  max: number;
  hasRoPE: boolean;
}

/**
 * Extracts context window info from GGUF metadata.
 * Returns native context length and the practical maximum (4× via YaRN)
 * when the model has RoPE parameters.
 */
export function extractContextInfo(metadata: Record<string, string> | undefined): ContextInfo | null {
  if (!metadata) return null;
  const arch = metadata['general.architecture'] || '';
  if (!arch) return null;

  const ctxStr = metadata[`${arch}.context_length`];
  if (!ctxStr) return null;

  const native = Number(ctxStr);
  if (isNaN(native) || native <= 0) return null;

  const hasRoPE = metadata[`${arch}.rope.dimension_count`] !== undefined
    || metadata[`${arch}.rope.freq_base`] !== undefined;

  return {
    native,
    max: hasRoPE ? native * 4 : native,
    hasRoPE,
  };
}

/**
 * Formats a context info line for tooltips / labels.
 * Example: "Context: 256K (Max ~1M via YaRN)"
 */
export function formatContextHint(info: ContextInfo): string {
  const nativeStr = fmtCompact(info.native);
  if (!info.hasRoPE || info.max <= info.native) {
    return `Context: ${nativeStr}`;
  }
  return `Context: ${nativeStr} (Max ~${fmtCompact(info.max)} via YaRN)`;
}
