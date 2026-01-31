export interface SamplingConfig {
  temperature: number;
  top_k: number;
  top_p: number;
  min_p: number;
  max_tokens: number;
  repeat_penalty: number;
  repeat_last_n: number;
  dry_multiplier: number;
  dry_base: number;
  dry_allowed_length: number;
  dry_penalty_last_n: number;
  xtc_probability: number;
  xtc_threshold: number;
  xtc_min_keep: number;
  enable_thinking: string;
  reasoning_effort: string;
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
  sampling?: SamplingConfig;
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

export interface ModelConfig {
  device: string;
  'context-window': number;
  nbatch: number;
  nubatch: number;
  nthreads: number;
  'nthreads-batch': number;
  'cache-type-k': string;
  'cache-type-v': string;
  'use-direct-io': boolean;
  'flash-attention': string;
  'ignore-integrity-check': boolean;
  'nseq-max': number;
  'offload-kqv': boolean | null;
  'op-offload': boolean | null;
  'ngpu-layers': number | null;
  'split-mode': string;
  'system-prompt-cache': boolean;
  'incremental-cache': boolean;
  'cache-min-tokens': number;
  'sampling-parameters': SamplingConfig;

  // YaRN RoPE scaling for extended context windows.
  'rope-scaling-type': string;
  'rope-freq-base': number | null;
  'rope-freq-scale': number | null;
  'yarn-ext-factor': number | null;
  'yarn-attn-factor': number | null;
  'yarn-beta-fast': number | null;
  'yarn-beta-slow': number | null;
  'yarn-orig-ctx': number | null;
}

export interface ModelInfoResponse {
  id: string;
  object: string;
  created: number;
  owned_by: string;
  desc: string;
  size: number;
  has_projection: boolean;
  is_gpt: boolean;
  metadata: Record<string, string>;
  vram?: VRAM;
  model_config?: ModelConfig;
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
  proj: CatalogFile;
}

export interface VRAMInput {
  model_size_bytes: number;
  context_window: number;
  block_count: number;
  head_count_kv: number;
  key_length: number;
  value_length: number;
  bytes_per_element: number;
  slots: number;
  cache_sequences: number;
}

export interface VRAM {
  input: VRAMInput;
  kv_per_token_per_layer: number;
  kv_per_slot: number;
  total_slots: number;
  slot_memory: number;
  total_vram: number;
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
  vram?: VRAM;
  model_config?: ModelConfig;
  model_metadata?: Record<string, string>;
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

export interface VRAMRequest {
  model_url: string;
  context_window: number;
  bytes_per_element: number;
  slots: number;
  cache_sequences: number;
}

export interface VRAMCalculatorResponse {
  input: VRAMInput;
  kv_per_token_per_layer: number;
  kv_per_slot: number;
  total_slots: number;
  slot_memory: number;
  total_vram: number;
}
