import type { CatalogSummary } from '../types';
import { formatBytes } from '../lib/format';
import KeyValueTable from './KeyValueTable';
import { labelWithTip } from './ParamTooltips';

type DraftInfo = Partial<Pick<CatalogSummary, 'draft' | 'has_draft' | 'has_mtp' | 'mtp_source'>>;

export function draftLabel(info: DraftInfo): string {
  if (info.draft) {
    const method = info.draft.method;
    if (method === 'mtp') return 'MTP companion';
    if (method === 'dspark') return 'DSpark companion';
    if (method === 'conventional') return 'Conventional companion';
    return 'Companion (method unspecified)';
  }
  if (info.mtp_source === 'embedded') return 'MTP embedded';
  if (info.mtp_source === 'companion') return 'MTP companion';
  if (info.has_draft) return 'Companion (method unspecified)';
  if (info.has_mtp) return 'MTP (source unspecified)';
  return 'None';
}

interface DraftDetailsProps extends DraftInfo {
  url?: string;
  size?: number;
}

export default function DraftDetails({ url, size, ...info }: DraftDetailsProps) {
  const draft = info.draft;
  const sourceURL = url || draft?.url;
  const bytes = size ?? draft?.size;

  return (
    <div>
      <KeyValueTable rows={[
        { label: labelWithTip('Draft', 'catalogDraft'), value: draftLabel(info) },
        { label: labelWithTip('Draft method', 'catalogDraftMethod'), value: draft?.method || (info.mtp_source || info.has_mtp ? 'mtp' : 'Not specified') },
        { label: labelWithTip('Draft source', 'catalogDraftSource'), value: draft?.original || 'Not specified' },
        { label: labelWithTip('Draft file', 'catalogDraftFile'), value: draft?.file || 'Not specified' },
        { label: labelWithTip('Draft URL', 'catalogDraftURL'), value: sourceURL ? <code style={{ overflowWrap: 'anywhere' }}>{sourceURL}</code> : 'Not specified' },
        { label: labelWithTip('Draft size', 'catalogDraftSize'), value: bytes != null && bytes > 0 ? formatBytes(bytes) : 'Unknown' },
      ]} />
      {draft?.method === 'dspark' && <p>DSpark companion download only. DSpark inference is not supported.</p>}
    </div>
  );
}
