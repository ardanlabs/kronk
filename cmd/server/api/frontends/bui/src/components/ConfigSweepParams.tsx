import React from 'react';
import type { ConfigSweepDefinition } from '../types';
import { PARAM_TOOLTIPS, ParamTooltip } from './ParamTooltips';

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
  commitNumericSweep: (raw: string, field: 'nbatch' | 'nubatch' | 'contextWindow' | 'nSeqMax', setRaw: (v: string) => void) => void;
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
  commitNumericSweep,
  isRunning,
  trialCount,
}: ConfigSweepParamsProps) {
  return (
    <div className="playground-autotest-section">
      <h4>Config Parameters</h4>
      <p style={{ fontSize: 12, color: '#6d4c00', marginBottom: 8 }}>
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
      <p style={{ fontSize: 12, color: 'var(--color-gray-600)', marginTop: 8 }}>Trials: {trialCount}</p>
    </div>
  );
}
