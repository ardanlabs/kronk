import { useRef, useCallback } from 'react';

export const PARAM_TOOLTIPS: Record<string, string> = {
  // Sampling – Generation
  temperature: 'Scales the probability distribution to control randomness. Lower values (e.g. 0.2) make output more focused and deterministic; higher values (e.g. 1.5) increase variety and creativity. Values ≤ 0 fall back to the model default.',
  top_p: 'Nucleus sampling — keeps the smallest set of top tokens whose cumulative probability is ≥ this value. Lower values (e.g. 0.5) focus on the most likely tokens; 1.0 effectively disables top-p filtering. Works alongside temperature.',
  top_k: 'Limits sampling to the top K most probable tokens at each step. Lower values (e.g. 10) make output more predictable; higher values allow more variety. Values ≤ 0 fall back to the model default (use a very large K to effectively disable).',
  min_p: 'Filters out tokens whose probability is below this fraction of the top token\'s probability. For example, 0.05 removes tokens less than 5% as likely as the best choice. Higher = stricter filtering. 0 disables.',

  // Sampling – Repetition Control
  repeat_penalty: 'Penalizes tokens that appeared in the recent context window (controlled by Repeat Last N). Values above 1.0 discourage repetition; 1.0 means no penalty. Typical range is 1.0–1.3. Too high can make text incoherent.',
  repeat_last_n: 'How many recent tokens to check when applying the repeat penalty. Larger values look further back for repetitions. To disable repetition penalties, set Repeat Penalty to 1.0 instead.',
  frequency_penalty: 'Reduces the likelihood of a token proportional to how many times it has appeared. Positive values discourage overused tokens; negative values encourage them. Common range: -2.0 to 2.0. 0 disables.',
  presence_penalty: 'Applies a flat penalty to any token that has appeared at all, regardless of how often. Positive values encourage the model to use new tokens; negative values favor staying on existing ones. 0 disables.',

  // Sampling – DRY Sampler
  dry_multiplier: 'Strength of the DRY (Don\'t Repeat Yourself) anti-repetition penalty. Higher values more aggressively penalize repeated n-gram patterns. Values ≤ 0 fall back to the model default.',
  dry_base: 'Base for exponential DRY penalty growth. Higher values make the penalty increase faster for longer repeated sequences. Typical values are 1.5–2.0.',
  dry_allowed_length: 'Minimum n-gram length before DRY penalties apply — repeated sequences up to this token length are allowed without penalty. Useful for common short phrases. Higher values are more lenient.',
  dry_penalty_last_n: 'How many recent tokens DRY examines when looking for repeated patterns. Larger values detect repetitions from further back. 0 means use the full context.',

  // Sampling – XTC Sampler
  xtc_probability: 'Chance of enabling XTC (eXtreme Token Culling) for a generation step. When active, XTC removes very high-probability ("obvious") tokens to increase variety. 0 disables XTC entirely, 1 always applies it.',
  xtc_threshold: 'Probability cutoff for XTC culling. When XTC is active, tokens with probability ≥ this threshold are candidates for removal (with safeguards to keep output coherent). Lower thresholds make XTC more aggressive.',
  xtc_min_keep: 'Minimum number of token candidates to keep after XTC culling, preventing over-aggressive filtering. Ensures at least this many choices remain available.',

  // Sampling – Generation limit
  max_tokens: 'Maximum number of tokens (roughly words or word-pieces) to generate. Output may stop earlier on end-of-sequence or when the context window is full. Higher values allow longer answers but take more time.',

  // Sampling – Reasoning
  enable_thinking: 'Toggles "thinking" mode in the prompt template (model-dependent). Some models produce an explicit chain-of-thought section; others may ignore it. Can improve accuracy on complex tasks but increases token usage and latency.',
  reasoning_effort: 'Requested reasoning level (model/provider dependent). Higher effort may produce more thorough reasoning but uses more tokens and time. Models that don\'t support this setting will ignore it.',

  // Config sweep
  nbatch: 'Batch size capacity — maximum tokens processed per forward pass. Larger values speed up prompt evaluation and multi-request batching but increase VRAM usage. Typically keep ≤ context window size.',
  nubatch: 'Micro-batch size for prompt processing. Controls VRAM usage per batch operation. Must be ≤ NBatch. Smaller values reduce peak VRAM usage at the cost of slightly slower processing.',
  contextWindow: 'Maximum number of tokens (input + output combined) the model can handle at once. Larger windows support longer conversations but increase VRAM usage proportionally via the KV cache.',
  nSeqMax: 'Maximum number of concurrent request slots. Each slot handles one user request simultaneously. More slots = better concurrency, but each slot reserves memory for its KV cache.',
  flashAttention: 'Optimized attention algorithm that reduces VRAM usage and can improve speed. "Enabled" forces it on, "Disabled" forces it off, "Auto" lets the server decide based on model compatibility.',
  cacheType: 'KV cache precision. f16 = full precision (best quality), q8_0 = 8-bit quantized (less VRAM, minimal quality loss), q4_0 = 4-bit quantized (most VRAM savings, slight quality trade-off especially at long context).',
  cacheMode: 'Caching strategy. None = clears KV state after each request. SPC (System Prompt Cache) = reuses cached system-prompt state to speed up new conversations. IMC (Incremental Message Cache) = keeps the conversation\'s KV state in a dedicated slot for fast multi-turn follow-ups.',
};

export function ParamTooltip({ text }: { text: string }) {
  const wrapperRef = useRef<HTMLSpanElement>(null);
  const tipRef = useRef<HTMLSpanElement>(null);

  const reposition = useCallback(() => {
    const wrapper = wrapperRef.current;
    const tip = tipRef.current;
    if (!wrapper || !tip) return;
    const iconRect = wrapper.getBoundingClientRect();
    const tipWidth = tip.offsetWidth;
    const tipHeight = tip.offsetHeight;

    // Position above the icon using viewport coordinates (fixed positioning).
    const top = iconRect.top - tipHeight - 8;

    // Align left edge of tooltip with the icon, then clamp to viewport.
    let left = iconRect.left;
    const rightOverflow = left + tipWidth - window.innerWidth + 8;
    if (rightOverflow > 0) {
      left -= rightOverflow;
    }
    if (left < 8) {
      left = 8;
    }
    tip.style.left = `${left}px`;
    tip.style.top = `${top}px`;

    // Position the arrow to point at the icon.
    const arrowLeft = Math.max(10, Math.min(tipWidth - 10, iconRect.left - left + iconRect.width / 2));
    tip.style.setProperty('--arrow-left', `${arrowLeft}px`);
  }, []);

  return (
    <span className="param-tooltip-wrapper" ref={wrapperRef} onMouseEnter={reposition}>
      <span className="param-tooltip-icon">ⓘ</span>
      <span className="param-tooltip-text" ref={tipRef}>{text}</span>
    </span>
  );
}
