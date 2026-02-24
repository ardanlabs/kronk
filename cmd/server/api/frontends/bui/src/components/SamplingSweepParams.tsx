import React from 'react';
import type { SamplingSweepDefinition, SamplingConfig } from '../types';
import { PARAM_TOOLTIPS, ParamTooltip } from './ParamTooltips';

export type SamplingNumericKey = 'temperature' | 'top_p' | 'top_k' | 'min_p' | 'repeat_penalty' | 'repeat_last_n' |
  'frequency_penalty' | 'presence_penalty' | 'dry_multiplier' | 'dry_base' | 'dry_allowed_length' |
  'dry_penalty_last_n' | 'xtc_probability' | 'xtc_threshold' | 'xtc_min_keep' | 'max_tokens';

export interface SweepInputTriple {
  min: string;
  max: string;
  step: string;
}

export interface SweepParamRange {
  validMin: number;
  validMax: number;
  defaultStep: number;
}

export const SWEEP_PARAM_RANGES: Record<SamplingNumericKey, SweepParamRange> = {
  temperature: { validMin: 0, validMax: 2, defaultStep: 0.1 },
  top_p: { validMin: 0, validMax: 1, defaultStep: 0.1 },
  top_k: { validMin: 0, validMax: 200, defaultStep: 10 },
  min_p: { validMin: 0, validMax: 1, defaultStep: 0.05 },
  repeat_penalty: { validMin: 0, validMax: 3, defaultStep: 0.1 },
  repeat_last_n: { validMin: 0, validMax: 2048, defaultStep: 64 },
  frequency_penalty: { validMin: -2, validMax: 2, defaultStep: 0.1 },
  presence_penalty: { validMin: -2, validMax: 2, defaultStep: 0.1 },
  dry_multiplier: { validMin: 0, validMax: 5, defaultStep: 0.1 },
  dry_base: { validMin: 1, validMax: 3, defaultStep: 0.25 },
  dry_allowed_length: { validMin: 0, validMax: 100, defaultStep: 1 },
  dry_penalty_last_n: { validMin: 0, validMax: 2048, defaultStep: 64 },
  xtc_probability: { validMin: 0, validMax: 1, defaultStep: 0.1 },
  xtc_threshold: { validMin: 0, validMax: 1, defaultStep: 0.1 },
  xtc_min_keep: { validMin: 1, validMax: 100, defaultStep: 1 },
  max_tokens: { validMin: 1, validMax: 131072, defaultStep: 512 },
};

export interface SamplingSweepParamsProps {
  sweepDef: SamplingSweepDefinition;
  setSweepDef: React.Dispatch<React.SetStateAction<SamplingSweepDefinition>>;
  sweepInputs: Record<SamplingNumericKey, SweepInputTriple>;
  setSweepInputs: React.Dispatch<React.SetStateAction<Record<SamplingNumericKey, SweepInputTriple>>>;
  commitTriple: (key: SamplingNumericKey) => void;
  setSweepDirty: React.Dispatch<React.SetStateAction<boolean>>;
  catalogSampling?: SamplingConfig | null;
  isRunning: boolean;
  trialCount: number;
}

function SweepParamGroup({
  title,
  entries,
  sweepInputs,
  setSweepInputs,
  commitTriple,
  catalogSampling,
  isRunning,
}: {
  title: string;
  entries: [SamplingNumericKey & keyof SamplingConfig, string][];
  sweepInputs: Record<SamplingNumericKey, SweepInputTriple>;
  setSweepInputs: React.Dispatch<React.SetStateAction<Record<SamplingNumericKey, SweepInputTriple>>>;
  commitTriple: (key: SamplingNumericKey) => void;
  catalogSampling?: SamplingConfig | null;
  isRunning: boolean;
}) {
  return (
    <>
      <div className="playground-sweep-group-title">{title}</div>
      {entries.map(([key, label]) => {
        const r = SWEEP_PARAM_RANGES[key];
        const triple = sweepInputs[key];
        return (
          <div className="playground-sweep-param" key={key}>
            <label className="playground-sweep-param-toggle">
              {label}
              {PARAM_TOOLTIPS[key] && <ParamTooltip text={PARAM_TOOLTIPS[key]} />}
              {catalogSampling && catalogSampling[key] !== undefined && (
                <span className="sweep-catalog-hint" title="Default catalog value">(default: {catalogSampling[key]})</span>
              )}
            </label>
            <div className="playground-sweep-param-range">
              <div className="playground-sweep-param-range-field">
                <span className="playground-sweep-param-range-label">Min ({r.validMin})</span>
                <input type="number" value={triple.min} min={r.validMin} max={r.validMax} step={r.defaultStep}
                  onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], min: e.target.value } }))}
                  onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
              </div>
              <div className="playground-sweep-param-range-field">
                <span className="playground-sweep-param-range-label">Max ({r.validMax})</span>
                <input type="number" value={triple.max} min={r.validMin} max={r.validMax} step={r.defaultStep}
                  onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], max: e.target.value } }))}
                  onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
              </div>
              <div className="playground-sweep-param-range-field">
                <span className="playground-sweep-param-range-label">Step ({r.defaultStep})</span>
                <input type="number" value={triple.step} min={0} step={r.defaultStep}
                  onChange={(e) => setSweepInputs(prev => ({ ...prev, [key]: { ...prev[key], step: e.target.value } }))}
                  onBlur={() => commitTriple(key)} onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()} disabled={isRunning} />
              </div>
            </div>
          </div>
        );
      })}
    </>
  );
}

export default function SamplingSweepParams({
  sweepDef,
  setSweepDef,
  sweepInputs,
  setSweepInputs,
  commitTriple,
  setSweepDirty,
  catalogSampling,
  isRunning,
  trialCount,
}: SamplingSweepParamsProps) {
  return (
    <div className="playground-autotest-section">
      <h4>Sampling Sweep Parameters</h4>
      <p style={{ fontSize: 12, color: 'var(--color-gray-600)', marginBottom: 8 }}>
        Set min, max, and step values to define the sweep range. Set min = max for no sweep.
      </p>
      <div className="playground-sweep-params">
        <SweepParamGroup title="Generation" entries={[
          ['temperature', 'Temperature'], ['top_p', 'Top P'], ['top_k', 'Top K'], ['min_p', 'Min P'],
        ]} sweepInputs={sweepInputs} setSweepInputs={setSweepInputs} commitTriple={commitTriple} catalogSampling={catalogSampling} isRunning={isRunning} />

        <SweepParamGroup title="Repetition Control" entries={[
          ['repeat_penalty', 'Repeat Penalty'], ['repeat_last_n', 'Repeat Last N'],
          ['frequency_penalty', 'Frequency Penalty'], ['presence_penalty', 'Presence Penalty'],
        ]} sweepInputs={sweepInputs} setSweepInputs={setSweepInputs} commitTriple={commitTriple} catalogSampling={catalogSampling} isRunning={isRunning} />

        <SweepParamGroup title="DRY Sampler" entries={[
          ['dry_multiplier', 'DRY Multiplier'], ['dry_base', 'DRY Base'],
          ['dry_allowed_length', 'DRY Allowed Length'], ['dry_penalty_last_n', 'DRY Penalty Last N'],
        ]} sweepInputs={sweepInputs} setSweepInputs={setSweepInputs} commitTriple={commitTriple} catalogSampling={catalogSampling} isRunning={isRunning} />

        <SweepParamGroup title="XTC Sampler" entries={[
          ['xtc_probability', 'XTC Probability'], ['xtc_threshold', 'XTC Threshold'], ['xtc_min_keep', 'XTC Min Keep'],
        ]} sweepInputs={sweepInputs} setSweepInputs={setSweepInputs} commitTriple={commitTriple} catalogSampling={catalogSampling} isRunning={isRunning} />

        <div className="playground-sweep-group-title">Reasoning</div>
        <div className="playground-sweep-param">
          <label className="playground-sweep-param-toggle">
            Enable Thinking
            {PARAM_TOOLTIPS.enable_thinking && <ParamTooltip text={PARAM_TOOLTIPS.enable_thinking} />}
            {catalogSampling?.enable_thinking && (
              <span className="sweep-catalog-hint" title="Default catalog value">— {catalogSampling.enable_thinking === 'true' ? 'Enabled' : 'Disabled'}</span>
            )}
          </label>
          <div className="playground-sweep-option-checks">
            {(['true', 'false'] as const).map((val) => (
              <label key={val} className="playground-sweep-option-label">
                <input
                  type="checkbox"
                  checked={sweepDef.enable_thinking.includes(val)}
                  onChange={(e) => {
                    setSweepDef(d => {
                      const prev = d.enable_thinking;
                      const next = e.target.checked ? [...prev, val] : prev.filter(v => v !== val);
                      return { ...d, enable_thinking: next.length > 0 ? next : prev };
                    });
                    setSweepDirty(true);
                  }}
                  disabled={isRunning}
                />
                {val === 'true' ? 'Enabled' : 'Disabled'}
              </label>
            ))}
          </div>
        </div>
        <div className="playground-sweep-param">
          <label className="playground-sweep-param-toggle">
            Reasoning Effort
            {PARAM_TOOLTIPS.reasoning_effort && <ParamTooltip text={PARAM_TOOLTIPS.reasoning_effort} />}
            {catalogSampling?.reasoning_effort && (
              <span className="sweep-catalog-hint" title="Default catalog value">— {catalogSampling.reasoning_effort}</span>
            )}
          </label>
          <div className="playground-sweep-option-checks">
            {(['none', 'minimal', 'low', 'medium', 'high'] as const).map((val) => (
              <label key={val} className="playground-sweep-option-label">
                <input
                  type="checkbox"
                  checked={sweepDef.reasoning_effort.includes(val)}
                  onChange={(e) => {
                    setSweepDef(d => {
                      const prev = d.reasoning_effort;
                      const next = e.target.checked ? [...prev, val] : prev.filter(v => v !== val);
                      return { ...d, reasoning_effort: next.length > 0 ? next : prev };
                    });
                    setSweepDirty(true);
                  }}
                  disabled={isRunning}
                />
                {val.charAt(0).toUpperCase() + val.slice(1)}
              </label>
            ))}
          </div>
        </div>
      </div>
      <p style={{ fontSize: 12, color: 'var(--color-gray-600)', marginTop: 8 }}>Trials: {trialCount}</p>
    </div>
  );
}
