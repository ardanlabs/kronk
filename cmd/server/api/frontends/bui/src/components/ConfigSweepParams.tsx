import React from 'react';
import type { ConfigSweepDefinition } from '../types';
import { PARAM_TOOLTIPS, ParamTooltip } from './ParamTooltips';
import { MOE_SWEEP_LABELS } from '../lib/moe';

export interface ConfigSweepParamsProps {
  configSweepDef: ConfigSweepDefinition;
  setConfigSweepDef: React.Dispatch<React.SetStateAction<ConfigSweepDefinition>>;
  rawNBatch: string;
  setRawNBatch: (v: string) => void;
  rawNUBatch: string;
  setRawNUBatch: (v: string) => void;
  rawContextWindow: string;
  setRawContextWindow: (v: string) => void;
  rawNSeqMax: string;
  setRawNSeqMax: (v: string) => void;
  rawMoeKeepExpertsTopN: string;
  setRawMoeKeepExpertsTopN: (v: string) => void;
  rawOpOffloadMinBatch: string;
  setRawOpOffloadMinBatch: (v: string) => void;
  commitNumericSweep: (raw: string, field: 'nbatch' | 'nubatch' | 'contextWindow' | 'nSeqMax' | 'moeKeepExpertsTopN' | 'opOffloadMinBatch', setRaw: (v: string) => void) => void;
  isMoE?: boolean;
  isRunning: boolean;
  trialCount: number;
}

export default function ConfigSweepParams({
  configSweepDef,
  setConfigSweepDef,
  rawNBatch,
  setRawNBatch,
  rawNUBatch,
  setRawNUBatch,
  rawContextWindow,
  setRawContextWindow,
  rawNSeqMax,
  setRawNSeqMax,
  rawMoeKeepExpertsTopN,
  setRawMoeKeepExpertsTopN,
  rawOpOffloadMinBatch,
  setRawOpOffloadMinBatch,
  commitNumericSweep,
  isMoE,
  isRunning,
  trialCount,
}: ConfigSweepParamsProps) {
  return (
    <div className="playground-autotest-section">
      <p style={{ fontSize: 12, color: 'var(--color-warning-text)', marginBottom: 8 }}>
        ⚠ Each candidate reloads the model. This is slower than sampling sweeps.
      </p>
      <div className="playground-sweep-params">
        <div className="playground-sweep-param">
          <label className="playground-sweep-param-toggle">NBatch{PARAM_TOOLTIPS.nbatch && <ParamTooltip text={PARAM_TOOLTIPS.nbatch} />}</label>
          <input
            type="text"
            className="playground-sweep-param-values"
            value={rawNBatch}
            onChange={(e) => setRawNBatch(e.target.value)}
            onBlur={() => commitNumericSweep(rawNBatch, 'nbatch', setRawNBatch)}
            onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
            placeholder="512, 1024, 2048, 4096"
            disabled={isRunning}
          />
        </div>

        <div className="playground-sweep-param">
          <label className="playground-sweep-param-toggle">NUBatch{PARAM_TOOLTIPS.nubatch && <ParamTooltip text={PARAM_TOOLTIPS.nubatch} />}</label>
          <input
            type="text"
            className="playground-sweep-param-values"
            value={rawNUBatch}
            onChange={(e) => setRawNUBatch(e.target.value)}
            onBlur={() => commitNumericSweep(rawNUBatch, 'nubatch', setRawNUBatch)}
            onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
            placeholder="128, 256, 512, 1024, 2048"
            disabled={isRunning}
          />
        </div>

        <div className="playground-sweep-param">
          <label className="playground-sweep-param-toggle">Context Window{PARAM_TOOLTIPS.contextWindow && <ParamTooltip text={PARAM_TOOLTIPS.contextWindow} />}</label>
          <input
            type="text"
            className="playground-sweep-param-values"
            value={rawContextWindow}
            onChange={(e) => setRawContextWindow(e.target.value)}
            onBlur={() => commitNumericSweep(rawContextWindow, 'contextWindow', setRawContextWindow)}
            onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
            placeholder="2048, 4096, 8192, 16384, 32768"
            disabled={isRunning}
          />
        </div>

        <div className="playground-sweep-param">
          <label className="playground-sweep-param-toggle">NSeqMax{PARAM_TOOLTIPS.nSeqMax && <ParamTooltip text={PARAM_TOOLTIPS.nSeqMax} />}</label>
          <input
            type="text"
            className="playground-sweep-param-values"
            value={rawNSeqMax}
            onChange={(e) => setRawNSeqMax(e.target.value)}
            onBlur={() => commitNumericSweep(rawNSeqMax, 'nSeqMax', setRawNSeqMax)}
            onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
            placeholder="1, 2, 4, 8"
            disabled={isRunning}
          />
        </div>

        <div className="playground-sweep-param">
          <label className="playground-sweep-param-toggle">Flash Attention{PARAM_TOOLTIPS.flashAttention && <ParamTooltip text={PARAM_TOOLTIPS.flashAttention} />}</label>
          <div className="playground-sweep-option-checks">
            {['enabled', 'disabled'].map((val) => (
              <label key={val} className="playground-sweep-option-label">
                <input
                  type="checkbox"
                  checked={configSweepDef.flashAttention.values.includes(val)}
                  onChange={(e) => {
                    setConfigSweepDef(d => {
                      const prev = d.flashAttention.values;
                      const next = e.target.checked ? [...prev, val] : prev.filter(v => v !== val);
                      return { ...d, flashAttention: { ...d.flashAttention, values: next } };
                    });
                  }}
                  disabled={isRunning}
                />
                {val}
              </label>
            ))}
          </div>
        </div>

        <div className="playground-sweep-param">
          <label className="playground-sweep-param-toggle">Cache Type{PARAM_TOOLTIPS.cacheType && <ParamTooltip text={PARAM_TOOLTIPS.cacheType} />}</label>
          <div className="playground-sweep-option-checks">
            {['f16', 'q8_0', 'q4_0'].map((val) => (
              <label key={val} className="playground-sweep-option-label">
                <input
                  type="checkbox"
                  checked={configSweepDef.cacheType.values.includes(val)}
                  onChange={(e) => {
                    setConfigSweepDef(d => {
                      const prev = d.cacheType.values;
                      const next = e.target.checked ? [...prev, val] : prev.filter(v => v !== val);
                      return { ...d, cacheType: { ...d.cacheType, values: next } };
                    });
                  }}
                  disabled={isRunning}
                />
                {val}
              </label>
            ))}
          </div>
        </div>

        <div className="playground-sweep-param">
          <label className="playground-sweep-param-toggle">Cache Mode{PARAM_TOOLTIPS.cacheMode && <ParamTooltip text={PARAM_TOOLTIPS.cacheMode} />}</label>
          <div className="playground-sweep-option-checks">
            {['none', 'spc', 'imc'].map((val) => (
              <label key={val} className="playground-sweep-option-label">
                <input
                  type="checkbox"
                  checked={configSweepDef.cacheMode.values.includes(val)}
                  onChange={(e) => {
                    setConfigSweepDef(d => {
                      const prev = d.cacheMode.values;
                      const next = e.target.checked ? [...prev, val] : prev.filter(v => v !== val);
                      return { ...d, cacheMode: { ...d.cacheMode, values: next } };
                    });
                  }}
                  disabled={isRunning}
                />
                {val === 'none' ? 'None' : val.toUpperCase()}
              </label>
            ))}
          </div>
        </div>

      </div>

      {isMoE && (
        <>
          <h4 style={{ marginTop: 16 }}>MoE Parameters</h4>
          <div className="playground-sweep-params">
            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle">Expert Strategy{PARAM_TOOLTIPS.moeMode && <ParamTooltip text={PARAM_TOOLTIPS.moeMode} />}</label>
              <div className="playground-sweep-option-checks">
                {['experts_cpu', 'keep_top_n', 'experts_gpu'].map((val) => (
                  <label key={val} className="playground-sweep-option-label">
                    <input
                      type="checkbox"
                      checked={configSweepDef.moeMode?.values.includes(val) ?? false}
                      onChange={(e) => {
                        setConfigSweepDef(d => {
                          const prev = d.moeMode?.values ?? [];
                          const next = e.target.checked ? [...prev, val] : prev.filter(v => v !== val);
                          return { ...d, moeMode: { enabled: next.length > 0, values: next } };
                        });
                      }}
                      disabled={isRunning}
                    />
                    {MOE_SWEEP_LABELS[val] ?? val}
                  </label>
                ))}
              </div>
            </div>

            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle">GPU Expert Layers{PARAM_TOOLTIPS.moeKeepExpertsTopN && <ParamTooltip text={PARAM_TOOLTIPS.moeKeepExpertsTopN} />}</label>
              <input
                type="text"
                className="playground-sweep-param-values"
                value={rawMoeKeepExpertsTopN}
                onChange={(e) => setRawMoeKeepExpertsTopN(e.target.value)}
                onBlur={() => commitNumericSweep(rawMoeKeepExpertsTopN, 'moeKeepExpertsTopN', setRawMoeKeepExpertsTopN)}
                onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
                placeholder="0, 4, 8, 16"
                disabled={isRunning}
              />
            </div>

            <div className="playground-sweep-param">
              <label className="playground-sweep-param-toggle">Op Offload Min Batch{PARAM_TOOLTIPS.opOffloadMinBatch && <ParamTooltip text={PARAM_TOOLTIPS.opOffloadMinBatch} />}</label>
              <input
                type="text"
                className="playground-sweep-param-values"
                value={rawOpOffloadMinBatch}
                onChange={(e) => setRawOpOffloadMinBatch(e.target.value)}
                onBlur={() => commitNumericSweep(rawOpOffloadMinBatch, 'opOffloadMinBatch', setRawOpOffloadMinBatch)}
                onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
                placeholder="0, 128, 256, 512"
                disabled={isRunning}
              />
            </div>
          </div>
        </>
      )}

      <p style={{ fontSize: 12, color: 'var(--color-gray-600)', marginTop: 8 }}>Trials: {trialCount}</p>
    </div>
  );
}
