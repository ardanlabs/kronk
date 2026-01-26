export interface SamplingConfig {
  temperature?: number;
  top_k?: number;
  top_p?: number;
  min_p?: number;
  repeat_penalty?: number;
  repeat_last_n?: number;
  dry_multiplier?: number;
  dry_base?: number;
  dry_allowed_length?: number;
  dry_penalty_last_n?: number;
  xtc_probability?: number;
  xtc_threshold?: number;
  xtc_min_keep?: number;
}

export interface ExtendedConfig {
  context_window?: number;
  nbatch?: number;
  nubatch?: number;
  nthreads?: number;
  nthreads_batch?: number;
  cache_type_k?: string;
  cache_type_v?: string;
  flash_attention?: string;
  nseq_max?: number;
  system_prompt_cache?: boolean;
  first_message_cache?: boolean;
  cache_min_tokens?: number;
  sampling?: SamplingConfig;
}

export interface ListModelDetail {
  id: string;
  object: string;
  created: number;
  owned_by: string;
  model_family: string;
  size: number;
  modified: string;
  validated: boolean;
  extended_config?: ExtendedConfig;
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
  embedding: boolean;
  rerank: boolean;
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
  validated: boolean;
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

export interface ChatContentPartText {
  type: 'text';
  text: string;
}

export interface ChatContentPartImage {
  type: 'image_url';
  image_url: {
    url: string;
  };
}

export interface ChatContentPartAudio {
  type: 'input_audio';
  input_audio: {
    data: string;
    format: string;
  };
}

export type ChatContentPart = ChatContentPartText | ChatContentPartImage | ChatContentPartAudio;

export interface ChatMessage {
  role: 'user' | 'assistant' | 'system';
  content: string | ChatContentPart[];
}

export interface ChatRequest {
  model: string;
  messages: ChatMessage[];
  stream?: boolean;
  max_tokens?: number;
  temperature?: number;
  top_p?: number;
  top_k?: number;
  min_p?: number;
  repeat_penalty?: number;
  repeat_last_n?: number;
  dry_multiplier?: number;
  dry_base?: number;
  dry_allowed_length?: number;
  dry_penalty_last_n?: number;
  xtc_probability?: number;
  xtc_threshold?: number;
  xtc_min_keep?: number;
  enable_thinking?: string;
  reasoning_effort?: string;
  return_prompt?: boolean;
  stream_options?: {
    include_usage?: boolean;
  };
  logprobs?: boolean;
  top_logprobs?: number;
}

export interface ChatToolCallFunction {
  name: string;
  arguments: string;
}

export interface ChatToolCall {
  id: string;
  index: number;
  type: string;
  function: ChatToolCallFunction;
}

export interface ChatDelta {
  role?: string;
  content?: string;
  reasoning?: string;
  tool_calls?: ChatToolCall[];
}

export interface ChatChoice {
  index: number;
  delta: ChatDelta;
  finish_reason: string | null;
}

export interface ChatUsage {
  prompt_tokens: number;
  completion_tokens: number;
  reasoning_tokens: number;
  output_tokens: number;
  tokens_per_second: number;
}

export interface ChatStreamResponse {
  id: string;
  object: string;
  created: number;
  model: string;
  choices: ChatChoice[];
  usage?: ChatUsage;
}

export interface AttachedFile {
  type: 'image' | 'audio';
  name: string;
  dataUrl: string;
}

export interface DisplayMessage {
  role: 'user' | 'assistant';
  content: string;
  reasoning?: string;
  usage?: ChatUsage;
  toolCalls?: ChatToolCall[];
  attachments?: AttachedFile[];
}
