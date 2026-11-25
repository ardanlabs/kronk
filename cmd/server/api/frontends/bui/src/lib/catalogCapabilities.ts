import type { CatalogCapabilities, ModelSummary } from '../types';

export const BOOLEAN_CAPABILITY_LABELS = {
  images: 'Images',
  audio: 'Audio',
  video: 'Video',
  streaming: 'Streaming',
  reasoning: 'Reasoning',
  tooling: 'Tooling',
  embedding: 'Embedding',
  rerank: 'Rerank',
  decision: 'Decision',
} as const satisfies Partial<Record<keyof CatalogCapabilities, string>>;

export const DRAFT_CAPABILITY_LABELS = {
  mtp: 'MTP',
  dspark: 'DSpark',
  conventional: 'Conventional',
} as const satisfies Record<NonNullable<CatalogCapabilities['draft']>, string>;

export type BooleanCapability = keyof typeof BOOLEAN_CAPABILITY_LABELS;
export type DraftCapability = NonNullable<CatalogCapabilities['draft']>;
export type CapabilityFilter = BooleanCapability | DraftCapability;

export const BOOLEAN_CAPABILITY_KEYS = Object.keys(BOOLEAN_CAPABILITY_LABELS) as BooleanCapability[];
export const DRAFT_CAPABILITY_KEYS = Object.keys(DRAFT_CAPABILITY_LABELS) as DraftCapability[];

export function draftCapability(capabilities?: CatalogCapabilities, mtpSource?: ModelSummary['mtp_source']): DraftCapability | undefined {
  return capabilities?.draft ?? (mtpSource ? 'mtp' : undefined);
}

export function matchesCapability(capabilities: CatalogCapabilities | undefined, mtpSource: ModelSummary['mtp_source'], filter: CapabilityFilter): boolean {
  switch (filter) {
    case 'mtp':
    case 'dspark':
    case 'conventional':
      return draftCapability(capabilities, mtpSource) === filter;
    default:
      return capabilities?.[filter] === true;
  }
}
