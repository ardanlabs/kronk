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
  tokenizer_fingerprint: string;
  size: number;
  modified: string;
  validated: boolean;
  has_projection: boolean;
  sampling?: SamplingConfig;
  draft_model_id?: string;
}

export interface ListModelInfoResponse {
  object: string;
  data: ListModelDetail[];
}

export interface ModelDetail {
  id: string;
  backend: string;
  owned_by: string;
  model_family: string;
  size: number;
  vram_total: number;
  kv_cache: number;
  slots: number;
  expires_at: string;
  active_streams: number;
  status: string;
}

export type ModelDetailsResponse = ModelDetail[];

export type IMCSessionState = 'active' | 'idle' | 'empty';

export interface IMCSessionDetail {
  model_id: string;
  id: number;
  state: IMCSessionState;
  context: number;
  allocated: number;
  total_allocated: number;
  snapshot_bytes: number;
  peak_context: number;
  messages: number;
  input_messages: number;
  input_tokens: number;
  output_tokens: number;
  context_window: number;
  last_used: string;
  has_media: boolean;
}

export type IMCSessionsResponse = IMCSessionDetail[];

export interface IMCSystemCacheDetail {
  model_id: string;
  id: number;
  tokens: number;
  allocated: number;
  snapshot_bytes: number;
  restore_count: number;
  active_restores: number;
  last_used: string;
}

export type IMCSystemCachesResponse = IMCSystemCacheDetail[];

export interface BatchGenerationContribution {
  slot_id: number;
  rows: number;
  mode: string;
}

export interface BatchSlotDetail {
  id: number;
  phase: 'idle' | 'starting' | 'imc-restore' | 'prefill-imc' | 'prefill' | 'media-prefill' | 'generation';
  request_id: string;
  request_age_ms: number;
  prefill_owner: boolean;
  prompt_tokens: number;
  prefilled_tokens: number;
  prefill_remaining: number;
  generated_tokens: number;
  past_tokens: number;
  generation_mode: string;
  generation_rows: number;
  imc_prepared_tokens: number;
  imc_total_tokens: number;
  imc_preparation_remaining: number;
}

export interface BatchEngineDetail {
  model_id: string;
  iteration: number;
  prefill_batch_size: number;
  nbatch: number;
  nubatch: number;
  mtp: boolean;
  ndraft: number;
  queued_requests: number;
  pending_requests: number;
  prefill_selector_start: number;
  prefill_selector_selected: number;
  prefill_selector_next: number;
  eligible_prefill_slots: number[];
  imc_selector_start: number;
  imc_selector_selected: number;
  imc_selector_next: number;
  eligible_imc_slots: number[];
  generation_rows: number;
  prefill_rows: number;
  total_rows: number;
  generation_contributions: BatchGenerationContribution[];
  slots: BatchSlotDetail[];
}

export type BatchEngineSlotsResponse = BatchEngineDetail[];

export interface DeviceBudget {
  index: number;
  name: string;
  type: string;
  total_bytes: number;
  budget_bytes: number;
  used_bytes: number;
}

export interface ReservationDevice {
  index: number;
  name: string;
  bytes: number;
}

export interface Reservation {
  key: string;
  vram_bytes: number;
  ram_bytes: number;
  per: ReservationDevice[];
}

export interface PoolBudgetResponse {
  budget_percent: number;
  headroom_bytes: number;
  unified_memory: boolean;
  ram_total: number;
  ram_budget: number;
  ram_used: number;
  devices: DeviceBudget[];
  reservations: Reservation[];
}

export type ModelLoadMode = 'auto' | 'mmap' | 'none' | 'mlock' | 'mmap+mlock' | 'direct-io';

export interface ModelConfig {
  'context-window': number;
  'prefill-batch-size': number;
  nthreads: number;
  'nthreads-batch': number;
  'cache-type-k': string;
  'cache-type-v': string;
  'load-mode': ModelLoadMode;
  'flash-attention': string;
  'nseq-max': number;
  'queue-depth': number;
  'admission-capacity': number;
  'imc-session-capacity': number;
  'offload-kqv': boolean | null;
  'op-offload': boolean | null;
  'op-offload-min-batch'?: number;
  'proj-on-cpu': boolean | null;
  'ngpu-layers': number | null;
  'split-mode': string | null;
  'tensor-split': number[] | null;
  'tensor-buft-overrides': string[] | null;
  'main-gpu': number | null;
  'devices': string[] | null;

  'swa-full': boolean | null;
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

  // MoE configuration for expert placement.
  moe?: {
    mode: string;
    'keep-experts-top-n'?: number | null;
  };

  // NUMA configuration for multi-socket systems.
  numa?: string | null;

  // Speculative decoding (draft model).
  'draft-model'?: {
    'model-id': string;
    ndraft: number;
    'ngpu-layers': number | null;
    devices?: string[];
    'main-gpu'?: number | null;
    'tensor-split'?: number[] | null;
  };
}

export interface ModelInfoResponse {
  id: string;
  object: string;
  created: number;
  owned_by: string;
  desc: string;
  size: number;
  file_type: number;
  quantization: string;
  has_projection: boolean;
  web_page?: string;
  template: string;
  metadata: Record<string, string>;
  vram?: VRAM;
  model_config?: ModelConfig;
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
  size: number;
}

export interface CatalogFiles {
  model: CatalogFile[];
  proj: CatalogFile;
  mtp: CatalogFile;
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
  sliding_window?: number;
  sliding_window_layers?: number;
  embedding_length?: number;
  moe?: MoEInfo;
  weights?: WeightBreakdown;
  gpu_layers: number;
  expert_layers_on_gpu: number;
  kv_cache_on_cpu?: boolean;
  swa_full: boolean;
}

export interface MoEInfo {
  is_moe: boolean;
  expert_count: number;
  expert_used_count: number;
  has_shared_experts: boolean;
}

export interface WeightBreakdown {
  total_bytes: number;
  always_active_bytes: number;
  expert_bytes_total: number;
  expert_bytes_by_layer: number[];
}

export interface VRAM {
  input: VRAMInput;
  kv_per_token_per_layer: number;
  kv_per_slot: number;
  slot_memory: number;
  total_vram: number;
  moe?: MoEInfo;
  weights?: WeightBreakdown;
  model_weights_gpu?: number;
  model_weights_cpu?: number;
  compute_buffer_est?: number;
  always_active_gpu_bytes?: number;
  always_active_cpu_bytes?: number;
  expert_gpu_bytes?: number;
  expert_cpu_bytes?: number;
  kv_vram_bytes?: number;
  kv_cpu_bytes?: number;
  total_system_ram_est?: number;
  unified_footprint: number;
  fit_assessment?: FitAssessment;
  per_device?: PerDeviceVRAM[];
}

export type FitStatus = 'unknown' | 'fits' | 'tight' | 'does_not_fit';

export interface CapacityAssessment {
  required_bytes: number;
  capacity_bytes: number;
  headroom_bytes: number;
  status: FitStatus;
}

export interface FitAssessment {
  fits: boolean;
  status: FitStatus;
  gpu: CapacityAssessment;
  system_ram: CapacityAssessment;
  unified: CapacityAssessment;
}

export interface PerDeviceVRAM {
  label: string;
  weights_bytes: number;
  kv_bytes: number;
  compute_bytes: number;
  total_bytes: number;
}

// CatalogSummary is the per-entry shape returned by GET /v1/catalog. It
// is cheap to compute on the server (catalog.yaml + local index, no GGUF
// reads). model_type and capabilities are persisted on the catalog entry
// itself (populated when the entry was added or refreshed) so the list
// page can filter without paying GGUF I/O on every call.
export interface CatalogSummary {
  id: string;
  owned_by: string;
  model_family: string;
  revision: string;
  web_page: string;
  total_size: string;
  total_size_bytes: number;
  has_projection: boolean;
  has_mtp?: boolean;
  downloaded: boolean;
  validated: boolean;
  model_type?: string;
  capabilities?: CatalogCapabilities;
}

// CatalogModelResponse is the per-entry payload returned by the catalog
// endpoints. The list endpoint (GET /v1/catalog) populates only the
// CatalogSummary fields; the detail endpoint (GET /v1/catalog/{id})
// additionally fills in the GGUF-derived fields (gguf_arch, parameters,
// template, files, model_metadata).
//
// Fields that are not produced by the new backend (description,
// collections, gated_model, etc.) remain optional so the editor compiles
// until it is rewritten in a follow-up.
export interface CatalogModelResponse extends CatalogSummary {
  gguf_arch?: string;
  parameters?: string;
  parameter_count?: number;
  template?: string;
  files?: CatalogFiles;
  model_metadata?: Record<string, string>;

  // Legacy fields — not returned by the new backend.
  category?: string;
  architecture?: string;
  metadata?: {
    created?: string;
    collections?: string;
    description?: string;
  };
  gated_model?: boolean;
  vram?: VRAM;
  swa_full_default?: boolean;
  model_config?: ModelConfig;
  base_config?: ModelConfig;
  catalog_file?: string;
}

export type CatalogModelsResponse = CatalogModelResponse[];

export interface KeyResponse {
  id: string;
  created: string;
}

export type KeysResponse = KeyResponse[];

export interface PullMeta {
  model_url?: string;
  proj_url?: string;
  model_id?: string;
  file_index?: number;
  file_total?: number;
}

export interface PullProgress {
  src?: string;
  current_bytes?: number;
  total_bytes?: number;
  mb_per_sec?: number;
  complete?: boolean;
}

export interface PullResponse {
  status: string;
  model_file?: string;
  model_files?: string[];
  proj_file?: string;
  mtp_file?: string;
  downloaded?: boolean;
  meta?: PullMeta;
  progress?: PullProgress;
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

export interface LibsCombination {
  arch: string;
  os: string;
  processor: string;
}

export interface LibsCombinationsResponse {
  combinations: LibsCombination[];
}

export interface LibsBundleTag {
  version: string;
  arch: string;
  os: string;
  processor: string;
}

export interface LibsBundleListResponse {
  bundles: LibsBundleTag[];
}

export interface LibsBundleActionResponse {
  status: string;
  arch: string;
  os: string;
  processor: string;
}

export interface LibsPeerBundle {
  arch: string;
  os: string;
  processor: string;
  version: string;
  size?: number;
  sha256?: string;
}

export interface LibsPeerBundleListResponse {
  bundles: LibsPeerBundle[];
}

export interface LibsPeerPullEvent {
  status: string;
  arch?: string;
  os?: string;
  processor?: string;
  version?: string;
  bytes?: number;
  bytes_total?: number;
  mb_per_second?: number;
  size?: number;
  sha256?: string;
  error?: string;
}

export interface PeerModelDetail {
  id: string;
  owned_by: string;
  model_family: string;
  size: number;
  validated: boolean;
  has_projection: boolean;
}

export interface PeerModelListResponse {
  models: PeerModelDetail[];
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
  prompt_tokens_details: {
    cached_tokens: number;
  };
  completion_tokens: number;
  completion_tokens_details?: {
    reasoning_tokens: number;
  };
  total_tokens: number;
  tokens_per_second: number;
  time_to_first_token_ms?: number;
  draft_tokens?: number;
  draft_accepted_tokens?: number;
  draft_acceptance_rate?: number;
  // Fraction of completion_tokens emitted via speculation (rounds + accepted drafts).
  // Together with draft_acceptance_rate this distinguishes "MTP ran the whole
  // request at high acceptance" from "MTP ran for a few rounds then was disabled
  // and the rest was target-only".
  draft_coverage?: number;
  // Empty when MTP stayed enabled. Otherwise one of: "imc-hit",
  // "hybrid-restore", "mirror-error".
  draft_disable_reason?: string;
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
  repo_files: HFRepoFile[];
}

export interface ResolveSourceResponse {
  canonical_id: string;
  provider: string;
  family: string;
  revision: string;
  download_urls: string[];
  download_proj?: string;
  download_mtp?: string;
  from_cache: boolean;
  from_local: boolean;
  installed: boolean;
  // repo_files is populated only when the input identified a repository
  // without selecting a specific file. The other fields above are then
  // empty (except provider and family) and the caller should let the
  // user pick a file from this list.
  repo_files?: HFRepoFile[];
}

export interface AutoTuneRequest {
  model_id?: string;
  catalog_id?: string;
}

export interface AutoTuneRecommendation {
  name: string;
  context_window: number;
  nseq_max: number;
  cache_type_k: string;
  cache_type_v: string;
  flash_attention: string;
  split_mode: string;
  ngpu_layers: number;
  estimated_vram_bytes: number;
  fits: boolean;
  reason?: string;
}

export interface AutoTuneResponse {
  recommended: AutoTuneRecommendation;
  profiles?: AutoTuneRecommendation[];
  warnings?: string[];
}

export interface VRAMRequest {
  model_url?: string;
  model_urls?: string[];
  model_id?: string;
  context_window: number;
  bytes_per_element: number;
  slots: number;
  gpu_layers?: number;
  expert_layers_on_gpu?: number;
  kv_cache_on_cpu?: boolean;
  swa_full?: boolean;
  device_count?: number;
  tensor_split?: number[];
  auto_fit?: boolean;
  gpu_free_bytes?: number[];
  gpu_capacity_bytes?: number;
  system_ram_bytes?: number;
  unified_memory?: boolean;
}

export interface VRAMCalculatorResponse extends VRAM {
  auto_fit_succeeded?: boolean;
  repo_files?: HFRepoFile[];
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
  'prefill_batch_size'?: number;
  'nseq_max'?: number;
  'flash_attention'?: string;
  'cache_type_k'?: string;
  'cache_type_v'?: string;
  'load_mode'?: ModelLoadMode;
  'ngpu_layers'?: number | null;
  'incremental_cache'?: boolean;
  'split_mode'?: string;
  'devices'?: string[] | null;
  'main_gpu'?: number | null;
  'tensor_split'?: number[] | null;
  'rope_scaling_type'?: string;
  'rope_freq_base'?: number | null;
  'rope_freq_scale'?: number | null;
  'yarn_ext_factor'?: number | null;
  'yarn_attn_factor'?: number | null;
  'yarn_beta_fast'?: number | null;
  'yarn_beta_slow'?: number | null;
  'yarn_orig_ctx'?: number | null;
  'moe_mode'?: string;
  'moe_keep_experts_top_n'?: number | null;
  'tensor_buft_overrides'?: string[];
  'op_offload_min_batch'?: number | null;
  'draft_model_id'?: string;
  'draft_ndraft'?: number;
}

export interface PlaygroundSessionResponse {
  session_id: string;
  cache_key?: string;
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

export type AutoTestTrialStatus = 'queued' | 'running' | 'completed' | 'failed' | 'cancelled' | 'skipped';

export type AutoTestRunnerState = 'idle' | 'repairing_template' | 'running_trials' | 'completed' | 'cancelled' | 'error';

export type ContextFillRatio = '0%' | '20%' | '50%' | '80%';

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

export interface AutoTestLogEntry {
  timestamp: string;
  message: string;
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
  avgTTFTByFill?: Record<ContextFillRatio, number>;
  activePrompts?: AutoTestActivePrompt[];
  logEntries?: AutoTestLogEntry[];
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
  prefillBatchSize: SweepParamValues;
  contextWindow: SweepParamValues;
  nSeqMax: SweepParamValues;
  flashAttention: SweepStringValues;
  cacheType: SweepStringValues;
  cacheMode: SweepStringValues;
  moeMode?: SweepStringValues;
  moeKeepExpertsTopN?: SweepParamValues;
  opOffloadMinBatch?: SweepParamValues;
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
  tps0: number;
  tps20: number;
  tps50: number;
  tps80: number;
  ttft0: number;
  ttft20: number;
  ttft50: number;
  ttft80: number;
}

export interface ConfigCandidate {
  'context_window'?: number;
  'prefill_batch_size'?: number;
  'nseq_max'?: number;
  'flash_attention'?: string;
  'cache_type'?: string;
  'cache_mode'?: string;
  'moe_mode'?: string;
  'moe_keep_experts_top_n'?: number;
  'op_offload_min_batch'?: number;
}

export interface ModelCaps {
  isHybrid?: boolean;
}

export interface AutoTestSessionSeed {
  model_id: string;
  template_mode: 'builtin' | 'custom';
  template_name?: string;
  template_script?: string;
  base_config: PlaygroundModelConfig;
}

export interface DeviceInfo {
  index: number;
  name: string;
  type: 'cpu' | 'gpu_cuda' | 'gpu_metal' | 'gpu_rocm' | 'gpu_vulkan' | 'unknown';
  free_bytes: number;
  total_bytes: number;
}

export interface DevicesResponse {
  devices: DeviceInfo[];
  gpu_count: number;
  gpu_total_bytes: number;
  supports_gpu_offload: boolean;
  max_devices: number;
  system_ram_bytes: number;
  unified_memory: boolean;
}

// =============================================================================
// Diagnose (host diagnostic report)

export interface DiagnoseVersions {
  kronk: string;
  yzma: string;
}

export interface DiagnoseCommand {
  cmd: string;
  output: string;
  err?: string;
}

export interface DiagnoseSystem {
  os: string;
  arch: string;
  numCPU: number;
  cpuModel: string;
  ramBytes: number;
  commands?: DiagnoseCommand[];
}

export interface DiagnoseDevice {
  id: string;
  name: string;
  vramTotalMiB: number;
  vramFreeMiB: number;
}

export interface DiagnoseBackend {
  processor: string;
  version: string;
  binDir: string;
  build: string;
  devices: DiagnoseDevice[];
  commands?: DiagnoseCommand[];
}

export interface DiagnoseLlama {
  installed: boolean;
  root: string;
  backends: DiagnoseBackend[];
}

export interface DiagnoseEngine {
  probed: boolean;
  loaded: boolean;
  processor: string;
  libPath: string;
  error?: string;
}

export interface DiagnoseBench {
  processor: string;
  model: string;
  commands?: DiagnoseCommand[];
}

export interface DiagnoseHint {
  severity: string;
  message: string;
  remedy?: string;
}

export interface DiagnoseResponse {
  versions: DiagnoseVersions;
  system: DiagnoseSystem;
  llama: DiagnoseLlama;
  engine: DiagnoseEngine;
  bench: DiagnoseBench;
  hints?: DiagnoseHint[];
}

// =============================================================================
// Bucky (whisper) models

export interface BuckyCatalogEntry {
  id: string;
  url: string;
  size: string;
  notes: string;
}

export interface BuckyCatalogResponse {
  models: BuckyCatalogEntry[];
}

export interface BuckyModelEntry {
  id: string;
  path: string;
  size: number;
  modified: string;
}

export interface BuckyModelsResponse {
  models: BuckyModelEntry[];
}

export interface BuckyModelActionResponse {
  status: string;
  id: string;
}

export interface BuckyModelDetails {
  id: string;
  model_type: string;
  is_multilingual: boolean;
  quantization: string;
  qnt_version: number;
  n_vocab: number;
  n_audio_ctx: number;
  n_audio_state: number;
  n_audio_head: number;
  n_audio_layer: number;
  n_text_ctx: number;
  n_text_state: number;
  n_text_head: number;
  n_text_layer: number;
  n_mels: number;
}

// =============================================================================
// Audio transcription / translation

export interface TranscriptionSegment {
  id: number;
  start: number;
  end: number;
  text: string;
  no_speech_prob: number;
}

export interface TranscriptionResponse {
  task: string;
  language: string;
  duration: number;
  text: string;
  segments: TranscriptionSegment[];
}

// =============================================================================
// Accuracy app — model code-recall comparison

export interface AccuracyFunction {
  num: number;
  line: number;
  loc: number;
  identifier: string;
}

export interface AccuracyFunctionsResponse {
  object: string;
  data: AccuracyFunction[];
}

export interface AccuracyDiffLine {
  op: 'context' | 'del' | 'add';
  text: string;
}

export interface AccuracyUsage {
  prompt_tokens: number;
  completion_tokens: number;
  tokens_per_second: number;
}

export interface AccuracyResponse {
  model: string;
  function: string;
  line: number;
  match_percent: number;
  want: string;
  got: string;
  diff: AccuracyDiffLine[];
  usage: AccuracyUsage;
}

// =============================================================================
// Efficiency app — model throughput comparison

export interface EfficiencyUsage {
  prompt_tokens: number;
  completion_tokens: number;
  in_tps: number;
  out_tps: number;
  ttft_ms: number;
  wallclock_ms: number;
}

export interface EfficiencyResponse {
  model: string;
  prompt: string;
  output: string;
  usage: EfficiencyUsage;
  ran_at: number;
}
