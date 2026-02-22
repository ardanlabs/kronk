export interface SamplingConfig {
  temperature: number;
  top_k: number;
  top_p: number;
  min_p: number;
  presence_penalty: number;
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
  frequency_penalty: number;
  enable_thinking: 'true' | 'false';
  reasoning_effort: 'none' | 'minimal' | 'low' | 'medium' | 'high';
  grammar: string;
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
  vram_total: number;
  slot_memory: number;
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

  // Speculative decoding (draft model).
  'draft-model'?: {
    'model-id': string;
    ndraft: number;
    'ngpu-layers': number | null;
    device?: string;
  };
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
}

export interface VRAM {
  input: VRAMInput;
  kv_per_token_per_layer: number;
  kv_per_slot: number;
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
  base_config?: ModelConfig;
  model_metadata?: Record<string, string>;
  downloaded: boolean;
  gated_model: boolean;
  validated: boolean;
  catalog_file?: string;
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
  allow_upgrade: boolean;
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
  tool_calls?: ChatToolCall[];
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
  presence_penalty?: number;
  repeat_penalty?: number;
  repeat_last_n?: number;
  dry_multiplier?: number;
  dry_base?: number;
  dry_allowed_length?: number;
  dry_penalty_last_n?: number;
  xtc_probability?: number;
  xtc_threshold?: number;
  xtc_min_keep?: number;
  frequency_penalty?: number;
  enable_thinking?: string;
  reasoning_effort?: string;
  return_prompt?: boolean;
  stream_options?: {
    include_usage?: boolean;
  };
  logprobs?: boolean;
  top_logprobs?: number;
  grammar?: string;
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
  reasoning_content?: string;
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
  time_to_first_token_ms?: number;
}

export interface ChatStreamResponse {
  id: string;
  object: string;
  created: number;
  model: string;
  choices: ChatChoice[];
  usage?: ChatUsage;
}

export interface HFRepoFile {
  filename: string;
  size: number;
  size_str: string;
}

export interface HFLookupResponse {
  model: CatalogModelResponse;
  repo_files: HFRepoFile[];
}

export interface SaveCatalogRequest {
  id: string;
  category: string;
  owned_by: string;
  model_family: string;
  web_page: string;
  gated_model: boolean;
  template: string;
  files: CatalogFiles;
  capabilities: CatalogCapabilities;
  metadata: {
    created: string;
    collections: string;
    description: string;
  };
  config?: ModelConfig;
  catalog_file: string;
}

export interface SaveCatalogResponse {
  status: string;
  id: string;
}

export interface CatalogFileInfo {
  name: string;
  model_count: number;
}

export interface VRAMRequest {
  model_url: string;
  context_window: number;
  bytes_per_element: number;
  slots: number;
}

export interface VRAMCalculatorResponse {
  input: VRAMInput;
  kv_per_token_per_layer: number;
  kv_per_slot: number;
  slot_memory: number;
  total_vram: number;
}

export interface HFRepoFile {
  filename: string;
  size: number;
  size_str: string;
}

export interface HFLookupResponse {
  model: CatalogModelResponse;
  repo_files: HFRepoFile[];
}

export interface SaveCatalogRequest {
  id: string;
  category: string;
  owned_by: string;
  model_family: string;
  web_page: string;
  gated_model: boolean;
  template: string;
  files: CatalogFiles;
  capabilities: CatalogCapabilities;
  metadata: CatalogMetadata;
  config?: ModelConfig;
  catalog_file: string;
}

export interface SaveCatalogResponse {
  status: string;
  id: string;
}

export interface CatalogFileInfo {
  name: string;
  model_count: number;
}

export type CatalogFilesListResponse = CatalogFileInfo[];

export interface PublishCatalogRequest {
  catalog_file: string;
}

export interface PublishCatalogResponse {
  status: string;
}

export interface RepoPathResponse {
  repo_path: string;
}

export interface ChatToolDefinition {
  type: 'function';
  function: {
    name: string;
    description?: string;
    parameters?: Record<string, unknown>;
  };
}

// =============================================================================
// Playground Types

export interface PlaygroundSessionRequest {
  model_id: string;
  template_mode: 'builtin' | 'custom';
  template_name?: string;
  template_script?: string;
  config: PlaygroundModelConfig;
}

export interface PlaygroundModelConfig {
  'context_window'?: number;
  nbatch?: number;
  nubatch?: number;
  'nseq_max'?: number;
  'flash_attention'?: string;
  'cache_type_k'?: string;
  'cache_type_v'?: string;
  'ngpu_layers'?: number | null;
  'system_prompt_cache'?: boolean;
  'incremental_cache'?: boolean;
  'split_mode'?: string;
  'rope_scaling_type'?: string;
  'rope_freq_base'?: number | null;
  'rope_freq_scale'?: number | null;
  'yarn_ext_factor'?: number | null;
  'yarn_attn_factor'?: number | null;
  'yarn_beta_fast'?: number | null;
  'yarn_beta_slow'?: number | null;
  'yarn_orig_ctx'?: number | null;
}

export interface PlaygroundSessionResponse {
  session_id: string;
  status: string;
  effective_config: Record<string, unknown>;
}

export interface PlaygroundTemplateInfo {
  name: string;
  size: number;
}

export interface PlaygroundTemplateListResponse {
  templates: PlaygroundTemplateInfo[];
}

export interface PlaygroundTemplateResponse {
  name: string;
  script: string;
}

export interface PlaygroundChatRequest {
  session_id: string;
  messages: ChatMessage[];
  tools?: ChatToolDefinition[];
  stream?: boolean;
  return_prompt?: boolean;
  temperature?: number;
  top_k?: number;
  top_p?: number;
  min_p?: number;
  max_tokens?: number;
  presence_penalty?: number;
  repeat_penalty?: number;
  repeat_last_n?: number;
  dry_multiplier?: number;
  dry_base?: number;
  dry_allowed_length?: number;
  dry_penalty_last_n?: number;
  xtc_probability?: number;
  xtc_threshold?: number;
  xtc_min_keep?: number;
  frequency_penalty?: number;
  enable_thinking?: 'true' | 'false';
  reasoning_effort?: 'none' | 'minimal' | 'low' | 'medium' | 'high';
  grammar?: string;
  stream_options?: { include_usage?: boolean };
  logprobs?: boolean;
  top_logprobs?: number;
  adaptive_p_target?: number;
  adaptive_p_decay?: number;
}

// Automated Testing Types

export type AutoTestScenarioID = 'chat' | 'tool_call';

export type AutoTestTrialStatus = 'queued' | 'running' | 'completed' | 'failed' | 'cancelled';

export type AutoTestRunnerState = 'idle' | 'repairing_template' | 'running_trials' | 'completed' | 'cancelled' | 'error';

export type ContextFillRatio = '20%' | '50%' | '80%';

export interface AutoTestPromptDef {
  id: string;
  messages: ChatMessage[];
  tools?: ChatToolDefinition[];
  max_tokens?: number;
  expected?: { type: 'regex' | 'exact' | 'tool_call' | 'no_tool_call'; value?: string };
  contextFill?: { ratio: number; label: ContextFillRatio };
  includeInScore?: boolean;
}

export interface AutoTestScenario {
  id: AutoTestScenarioID;
  name: string;
  systemPrompt?: string;
  prompts: AutoTestPromptDef[];
}

export interface SamplingCandidate {
  temperature?: number;
  top_p?: number;
  top_k?: number;
  min_p?: number;
  max_tokens?: number;
  repeat_penalty?: number;
  repeat_last_n?: number;
  frequency_penalty?: number;
  presence_penalty?: number;
  dry_multiplier?: number;
  dry_base?: number;
  dry_allowed_length?: number;
  dry_penalty_last_n?: number;
  xtc_probability?: number;
  xtc_threshold?: number;
  xtc_min_keep?: number;
  adaptive_p_target?: number;
  adaptive_p_decay?: number;
  enable_thinking?: 'true' | 'false';
  reasoning_effort?: 'none' | 'minimal' | 'low' | 'medium' | 'high';
  grammar?: string;
}

export interface AutoTestPromptResult {
  promptId: string;
  assistantText: string;
  toolCalls: ChatToolCall[];
  usage?: ChatUsage;
  score: number;
  notes?: string[];
}

export interface AutoTestScenarioResult {
  scenarioId: AutoTestScenarioID;
  promptResults: AutoTestPromptResult[];
  score: number;
  avgTPS?: number;
  avgTTFT?: number;
  avgTPSByFill?: Record<ContextFillRatio, number>;
  avgTTFTByFill?: Record<ContextFillRatio, number>;
  promptTokensByFill?: Record<ContextFillRatio, number>;
}

export interface AutoTestActivePrompt {
  scenarioId: AutoTestScenarioID;
  promptId: string;
  promptIndex: number;
  repeatIndex?: number;
  repeats?: number;
  preview?: string;
  startedAt?: string;
}

export interface AutoTestTrialResult {
  id: string;
  status: AutoTestTrialStatus;
  candidate: SamplingCandidate;
  startedAt?: string;
  finishedAt?: string;
  scenarioResults: AutoTestScenarioResult[];
  totalScore?: number;
  avgTPS?: number;
  avgTTFT?: number;
  avgTPSByFill?: Record<ContextFillRatio, number>;
  activePrompts?: AutoTestActivePrompt[];
}

// Config Sweep Types

export type AutoTestSweepMode = 'sampling' | 'config';

export interface SweepParamValues {
  enabled: boolean;
  values: number[];
}

export interface SweepStringValues {
  enabled: boolean;
  values: string[];
}

export interface ConfigSweepDefinition {
  nbatch: SweepParamValues;
  nubatch: SweepParamValues;
  contextWindow: SweepParamValues;
  nSeqMax: SweepParamValues;
  flashAttention: SweepStringValues;
  cacheType: SweepStringValues;
  cacheMode: SweepStringValues;
}

export interface SamplingSweepDefinition {
  temperature: number[];
  top_p: number[];
  top_k: number[];
  min_p: number[];
  repeat_penalty: number[];
  repeat_last_n: number[];
  frequency_penalty: number[];
  presence_penalty: number[];
  dry_multiplier: number[];
  dry_base: number[];
  dry_allowed_length: number[];
  dry_penalty_last_n: number[];
  xtc_probability: number[];
  xtc_threshold: number[];
  xtc_min_keep: number[];
  max_tokens: number[];
  enable_thinking: string[];
  reasoning_effort: string[];
}

export interface BestConfigWeights {
  chatScore: number;
  toolScore: number;
  totalScore: number;
  avgTPS: number;
  avgTTFT: number;
}

export interface ConfigCandidate {
  'context_window'?: number;
  nbatch?: number;
  nubatch?: number;
  'nseq_max'?: number;
  'flash_attention'?: string;
  'cache_type'?: string;
  'cache_mode'?: string;
}

export interface ModelCaps {
  isHybrid?: boolean;
  isGPT?: boolean;
}

export interface AutoTestSessionSeed {
  model_id: string;
  template_mode: 'builtin' | 'custom';
  template_name?: string;
  template_script?: string;
  base_config: PlaygroundModelConfig;
}
