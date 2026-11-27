import type { DraftMetadata, ModelSummary as Summary, SamplingConfig } from '../types';
import { DRAFT_CAPABILITY_LABELS, draftCapability } from '../lib/catalogCapabilities';
import KeyValueTable from './KeyValueTable';
import { labelWithTip, type TooltipKey } from './ParamTooltips';

interface ModelSummaryProps {
  id: string;
  provider: string;
  family?: string;
  size: string;
  metadata: Record<string, string>;
  summary?: Summary;
  draft?: DraftMetadata;
}

const SAMPLING_ROWS: [string, keyof SamplingConfig, string, TooltipKey][] = [
  ['Temperature', 'temperature', 'temp', 'temperature'],
  ['Top K', 'top_k', 'top_k', 'top_k'],
  ['Top P', 'top_p', 'top_p', 'top_p'],
];

function tokens(value: string | number | null | undefined): string {
  const n = Number(value);
  return value != null && value !== '' && Number.isFinite(n) && n > 0
    ? `${n.toLocaleString()} tokens` : 'Unknown';
}

function samplingValue(value: unknown, absent: string): string {
  if (value == null || value === '') return absent;
  const n = Number(value);
  return Number.isFinite(n) ? n.toLocaleString(undefined, { maximumFractionDigits: 6 }) : 'Invalid metadata';
}

export default function ModelSummary({ id, provider, family, size, metadata, summary, draft: companion }: ModelSummaryProps) {
  const arch = metadata['general.architecture'];
  const caps = summary?.capabilities;
  const nativeContext = arch ? metadata[`${arch}.context_length`] : undefined;
  const mtpSource = summary?.mtp_source;
  const draft = draftCapability(caps, mtpSource);
  const draftSource = companion?.file ? 'Companion'
    : draft === 'mtp' && mtpSource ? (mtpSource === 'embedded' ? 'Embedded' : 'Companion')
    : caps ? 'Not declared' : 'Unknown';
  const yarn = summary?.rope_scaling === 'yarn';
  const capabilityRows: [string, boolean | undefined, TooltipKey][] = [
    ['Text', caps ? caps.endpoint !== 'speech' : undefined, 'summaryText'],
    ['Images', caps?.images, 'summaryImages'],
    ['Audio', caps?.audio, 'summaryAudio'],
    ['Video', caps?.video, 'summaryVideo'],
    ['Tool calling', caps?.tooling, 'summaryTooling'],
    ['Reasoning', caps?.reasoning, 'summaryReasoning'],
  ];

  return (
    <div className="model-summary">
      <h3>Summary</h3>
      <p className="model-summary-name">
        <strong>{id}</strong>
        {metadata['general.name'] && <span>{metadata['general.name']}</span>}
      </p>
      <div className="model-summary-capabilities" aria-label="Model capabilities">
        {capabilityRows.map(([label, supported, tip]) => (
          <span key={label} className="model-summary-capability">
            {labelWithTip(label, tip)}
            <span className={`badge ${supported == null ? '' : supported ? 'badge-yes' : 'badge-no'}`}>
              {supported == null ? 'Unknown' : supported ? 'Yes' : 'No'}
            </span>
          </span>
        ))}
      </div>
      <div className="model-summary-grid">
        <section>
          <h4 className="meta-section-title">Context &amp; speculation</h4>
          <KeyValueTable rows={[
            { label: labelWithTip('Trained context', 'contextLength'), value: tokens(nativeContext) },
            { label: labelWithTip('Configured context', 'summaryContext'), value: tokens(summary?.context_window) },
            { label: labelWithTip('YaRN extension', 'summaryYarn'), value: yarn
              ? `Configured to ${tokens(summary?.context_window)}${summary?.yarn_orig_ctx ? ` (original: ${tokens(summary.yarn_orig_ctx)})` : ''}`
              : summary?.rope_scaling || 'Not specified' },
            { label: labelWithTip('Draft capability', 'draftCapability'), value: draft ? DRAFT_CAPABILITY_LABELS[draft] : caps ? 'None' : 'Unknown' },
            { label: labelWithTip('Draft source', 'summaryDraftSource'), value: draftSource },
            { label: labelWithTip('Speculation mode', 'summarySpeculation'), value: summary?.speculation || 'Unknown' },
            ...(summary?.classic_draft ? [{ label: 'Classic draft', value: summary.classic_draft }] : []),
          ]} />
        </section>
        <section>
          <h4 className="meta-section-title">Model</h4>
          <KeyValueTable rows={[
            { label: 'Provider', value: provider || 'Unknown' },
            { label: labelWithTip('Family / GGUF architecture', 'modelArchitecture'), value: [family, arch].filter(Boolean).join(' / ') || 'Unknown' },
            { label: labelWithTip('Parameter activation', 'summaryActivation'), value: summary?.model_type || 'Unknown' },
            { label: labelWithTip('Sequence processing', 'summarySequence'), value: summary?.sequence_processing || 'Unknown' },
            { label: labelWithTip('Parameters', 'sizeLabel'), value: summary?.parameters || metadata['general.size_label'] || 'Unknown' },
            { label: labelWithTip('Layers', 'blockCount'), value: (arch && metadata[`${arch}.block_count`]) || 'Unknown' },
            { label: labelWithTip('Quantization', 'quantization'), value: summary?.quantization || 'Unknown' },
            { label: labelWithTip('File size', 'modelSize'), value: size },
            ...(summary?.model_type === 'MoE' ? [
              { label: labelWithTip('Total experts', 'expertCount'), value: metadata[`${arch}.expert_count`] || 'Unknown' },
              { label: labelWithTip('Active experts', 'activeExperts'), value: metadata[`${arch}.expert_used_count`] || 'Unknown' },
            ] : []),
          ]} />
        </section>
      </div>
      <section className="model-summary-sampling">
        <h4 className="meta-section-title">Sampling</h4>
        <p>Effective defaults include configuration overrides, GGUF recommendations, and Kronk baselines. Requests can override them.</p>
        {!summary?.sampling && <p>Effective defaults are unavailable until the model is downloaded and its configuration can be resolved.</p>}
        <table className="model-summary-sampling-table">
          <thead><tr><th>Parameter</th><th>GGUF recommendation</th><th>Effective default</th></tr></thead>
          <tbody>
            {SAMPLING_ROWS.map(([label, key, metadataKey, tip]) => (
              <tr key={key}>
                <th scope="row">{labelWithTip(label, tip)}</th>
                <td>{samplingValue(metadata[`general.sampling.${metadataKey}`], 'Not specified')}</td>
                <td>{samplingValue(summary?.sampling?.[key], 'Unknown')}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </div>
  );
}
