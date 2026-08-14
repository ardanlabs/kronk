import KeyValueTable from './KeyValueTable';
import type { KVRow } from './KeyValueTable';
import MetadataSection from './MetadataSection';
import { fmtVal } from '../lib/format';
import { labelWithTip, type TooltipKey } from './ParamTooltips';

const fileTypeMap: Record<string, string> = {
  '0': 'All F32',
  '1': 'Mostly F16',
  '2': 'Mostly Q4_0',
  '3': 'Mostly Q4_1',
  '7': 'Mostly Q8_0',
  '8': 'Mostly Q5_0',
  '9': 'Mostly Q5_1',
  '10': 'Mostly Q2_K',
  '11': 'Mostly Q3_K_S',
  '12': 'Mostly Q3_K_M',
  '13': 'Mostly Q3_K_L',
  '14': 'Mostly Q4_K_S',
  '15': 'Mostly Q4_K_M',
  '16': 'Mostly Q5_K_S',
  '17': 'Mostly Q5_K_M',
  '18': 'Mostly Q6_K',
  '19': 'Mostly IQ2_XXS',
  '20': 'Mostly IQ2_XS',
  '21': 'Mostly Q2_K_S',
  '22': 'Mostly IQ3_XS',
  '23': 'Mostly IQ3_XXS',
  '24': 'Mostly IQ1_S',
  '25': 'Mostly IQ4_NL',
  '26': 'Mostly IQ3_S',
  '27': 'Mostly IQ3_M',
  '28': 'Mostly IQ2_S',
  '29': 'Mostly IQ2_M',
  '30': 'Mostly IQ4_XS',
  '31': 'Mostly IQ1_M',
  '32': 'Mostly BF16',
  '36': 'Mostly TQ1_0',
  '37': 'Mostly TQ2_0',
  '38': 'Mostly MXFP4_MOE',
  '39': 'Mostly NVFP4',
  '40': 'Mostly Q1_0',
  '41': 'Mostly Q2_0',
};

function fmtInteger(v: string): string {
  const n = Number(v);
  if (isNaN(n)) return v;
  return n.toLocaleString();
}

function fmtCompact(n: number): string {
  if (n >= 1_000_000) return `${+(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${+(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

function fmtFileType(v: string): string {
  return fileTypeMap[v] || `File Type ${v}`;
}

interface ModelCardProps {
  metadata: Record<string, string>;
  excludeKeys?: string[];
  quantization?: string;
  webPage?: string;
}

export default function ModelCard({ metadata, excludeKeys = [], quantization, webPage }: ModelCardProps) {
  const allExclude = [...excludeKeys, 'tokenizer.chat_template'];

  const hasMetadata = Object.keys(metadata).length > 0;

  if (!hasMetadata && !quantization && !webPage) {
    return (
      <div className="empty-state">
        <p>No metadata available for this model.</p>
      </div>
    );
  }

  const get = (key: string): string | undefined => metadata[key];
  const arch = get('general.architecture') || '';

  type MetadataRowSpec = [label: string, key: string, tooltipKey?: TooltipKey];

  function buildRows(specs: MetadataRowSpec[]): KVRow[] {
    return specs
      .filter(([, key]) => get(key) !== undefined)
      .map(([label, key, tip]) => ({ key, label: tip ? labelWithTip(label, tip) : label, value: fmtVal(get(key)) }));
  }

  // --- Identity ---
  const identitySpecs: MetadataRowSpec[] = [
    ['Name', 'general.name'],
    ['Architecture', 'general.architecture', 'modelArchitecture'],
    ['Size Label', 'general.size_label', 'sizeLabel'],
    ['Tags', 'general.tags'],
  ];
  const identityRows = buildRows(identitySpecs);

  const fileType = get('general.file_type');
  const quantizationName = quantization || (fileType !== undefined ? fmtFileType(fileType) : '');
  if (quantizationName) {
    identityRows.push({ key: 'general.file_type', label: labelWithTip('Quantization', 'quantization'), value: quantizationName });
  }
  const quantizedBy = get('general.quantized_by');
  if (quantizedBy !== undefined) {
    identityRows.push({ key: 'general.quantized_by', label: 'Quantized By', value: fmtVal(quantizedBy) });
  }
  const license = get('general.license') || get('general.license.link');
  if (license !== undefined) {
    identityRows.push({ key: 'general.license', label: 'License', value: fmtVal(license) });
  }

  // --- Architecture ---
  const archSpecs: MetadataRowSpec[] = [
    ['Layers', `${arch}.block_count`, 'blockCount'],
    ['Context Length', `${arch}.context_length`, 'contextLength'],
    ['Embedding Dimension', `${arch}.embedding_length`, 'embeddingDimension'],
    ['Attention Heads (Q)', `${arch}.attention.head_count`, 'attentionHeadsQ'],
    ['Attention Heads (KV)', `${arch}.attention.head_count_kv`, 'headCountKV'],
    ['Key Length', `${arch}.attention.key_length`, 'keyLength'],
    ['Value Length', `${arch}.attention.value_length`, 'valueLength'],
    ['RoPE Dimension', `${arch}.rope.dimension_count`, 'ropeDimension'],
    ['RoPE Freq Base', `${arch}.rope.freq_base`, 'ropeFreqBase'],
    ['Feed Forward Length', `${arch}.feed_forward_length`, 'feedForwardLength'],
  ];
  const archRows = buildRows(archSpecs);

  // Format context length: "262,144 (256K, max ~1M via YaRN)"
  const ctxVal = get(`${arch}.context_length`);
  if (ctxVal !== undefined) {
    const idx = archRows.findIndex(r => r.key === `${arch}.context_length`);
    if (idx >= 0) {
      const ctxNum = Number(ctxVal);
      let display = fmtInteger(ctxVal);
      if (!isNaN(ctxNum) && ctxNum >= 1_000) {
        display += ` (${fmtCompact(ctxNum)}`;
        const hasRoPE = get(`${arch}.rope.dimension_count`) !== undefined || get(`${arch}.rope.freq_base`) !== undefined;
        if (hasRoPE) {
          const maxCtx = ctxNum * 4;
          display += `, max ~${fmtCompact(maxCtx)} via YaRN`;
        }
        display += ')';
      }
      archRows[idx] = { ...archRows[idx], value: display };
    }
  }

  // Format RoPE freq base with commas.
  const ropeVal = get(`${arch}.rope.freq_base`);
  if (ropeVal !== undefined) {
    const idx = archRows.findIndex(r => r.key === `${arch}.rope.freq_base`);
    if (idx >= 0) {
      archRows[idx] = { ...archRows[idx], value: fmtInteger(ropeVal) };
    }
  }

  // --- MoE ---
  const hasMoE = get(`${arch}.expert_count`) !== undefined;
  const moeRows = hasMoE
    ? buildRows([
        ['Expert Count', `${arch}.expert_count`, 'expertCount'],
        ['Experts Used', `${arch}.expert_used_count`, 'activeExperts'],
        ['Expert FFN Length', `${arch}.expert_feed_forward_length`, 'expertFFNLength'],
        ['Shared Expert FFN Length', `${arch}.expert_shared_feed_forward_length`, 'sharedExpertFFNLength'],
      ])
    : [];

  // --- Hybrid / SSM ---
  const hasSSM = get(`${arch}.ssm.inner_size`) !== undefined;
  const ssmRows = hasSSM
    ? buildRows([
        ['SSM Inner Size', `${arch}.ssm.inner_size`, 'ssmInnerSize'],
        ['SSM State Size', `${arch}.ssm.state_size`, 'ssmStateSize'],
        ['SSM Conv Kernel', `${arch}.ssm.conv_kernel`, 'ssmConvKernel'],
        ['SSM Time Step Rank', `${arch}.ssm.time_step_rank`, 'ssmTimeStepRank'],
        ['SSM Group Count', `${arch}.ssm.group_count`, 'ssmGroupCount'],
        ['Full Attention Interval', `${arch}.full_attention_interval`, 'fullAttentionInterval'],
      ])
    : [];

  // --- Tokenizer ---
  const tokenizerRows = buildRows([
    ['Model', 'tokenizer.ggml.model', 'tokenizerModel'],
    ['EOS Token ID', 'tokenizer.ggml.eos_token_id', 'eosTokenId'],
    ['BOS Token ID', 'tokenizer.ggml.bos_token_id', 'bosTokenId'],
    ['Padding Token ID', 'tokenizer.ggml.padding_token_id', 'paddingTokenId'],
  ]);

  const sections: Array<{ title: string; rows: KVRow[] }> = [
    { title: 'Identity', rows: identityRows },
    { title: 'Architecture', rows: archRows },
  ];
  if (moeRows.length > 0) sections.push({ title: 'Mixture of Experts', rows: moeRows });
  if (ssmRows.length > 0) sections.push({ title: 'Hybrid / SSM', rows: ssmRows });
  if (tokenizerRows.length > 0) sections.push({ title: 'Tokenizer', rows: tokenizerRows });

  return (
    <>
      {webPage && (
        <div style={{ marginBottom: '16px' }}>
          <h4 className="meta-section-title" style={{ marginBottom: '8px' }}>Source</h4>
          <p>
            <a href={webPage} target="_blank" rel="noopener noreferrer">
              {webPage}
            </a>
          </p>
        </div>
      )}
      {hasMetadata && (
        <>
          <div className="meta-sections">
            {sections
              .filter(s => s.rows.length > 0)
              .map(s => (
                <section key={s.title} className="meta-section">
                  <div className="meta-section-header">
                    <h4 className="meta-section-title">{s.title}</h4>
                  </div>
                  <KeyValueTable rows={s.rows} />
                </section>
              ))}
          </div>

          <details className="model-card-raw-toggle">
            <summary>Advanced / Raw Metadata</summary>
            <MetadataSection metadata={metadata} excludeKeys={allExclude} />
          </details>
        </>
      )}
    </>
  );
}
