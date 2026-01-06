export interface ListModelDetail {
  id: string;
  object: string;
  created: number;
  owned_by: string;
  model_family: string;
  size: number;
  modified: string;
}

export interface ListModelInfoResponse {
  object: string;
  data: ListModelDetail[];
}

export interface ModelDetail {
  id: string;
  owned_by: string;
  model_family: string;
  size: number;
  expires_at: string;
  active_streams: number;
}

export type ModelDetailsResponse = ModelDetail[];

export interface ModelInfoResponse {
  id: string;
  object: string;
  created: number;
  owned_by: string;
  desc: string;
  size: number;
  has_projection: boolean;
  has_encoder: boolean;
  has_decoder: boolean;
  is_recurrent: boolean;
  is_hybrid: boolean;
  is_gpt: boolean;
  metadata: Record<string, string>;
}

export interface CatalogMetadata {
  created: string;
  collections: string;
  description: string;
}

export interface CatalogCapabilities {
  endpoint: string;
  images: boolean;
  audio: boolean;
  video: boolean;
  streaming: boolean;
  reasoning: boolean;
  tooling: boolean;
}

export interface CatalogFile {
  url: string;
  size: string;
}

export interface CatalogFiles {
  model: CatalogFile[];
  proj: CatalogFile[];
}

export interface CatalogModelResponse {
  id: string;
  category: string;
  owned_by: string;
  model_family: string;
  web_page: string;
  template: string;
  files: CatalogFiles;
  capabilities: CatalogCapabilities;
  metadata: CatalogMetadata;
  downloaded: boolean;
  gated_model: boolean;
}

export type CatalogModelsResponse = CatalogModelResponse[];

export interface KeyResponse {
  id: string;
  created: string;
}

export type KeysResponse = KeyResponse[];

export interface PullResponse {
  status: string;
  model_file?: string;
  model_files?: string[];
  downloaded?: boolean;
}

export interface AsyncPullResponse {
  session_id: string;
}

export interface VersionResponse {
  status: string;
  arch?: string;
  os?: string;
  processor?: string;
  latest?: string;
  current?: string;
}

export type RateWindow = 'day' | 'month' | 'year' | 'unlimited';

export interface RateLimit {
  limit: number;
  window: RateWindow;
}

export interface TokenRequest {
  admin: boolean;
  endpoints: Record<string, RateLimit>;
  duration: number;
}

export interface TokenResponse {
  token: string;
}

export interface ApiError {
  error: {
    message: string;
  };
}
