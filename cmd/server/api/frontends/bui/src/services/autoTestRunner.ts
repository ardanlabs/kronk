import { api } from './api'
import type {
  ChatStreamResponse,
  ChatToolCall,
  ChatToolDefinition,
  ChatUsage,
  AutoTestPromptDef,
  AutoTestScenario,
  AutoTestScenarioID,
  AutoTestScenarioResult,
  AutoTestPromptResult,
  AutoTestTrialResult,
  SamplingCandidate,
  SamplingSweepDefinition,
  ConfigSweepDefinition,
  ConfigCandidate,
  PlaygroundModelConfig,
  BestConfigWeights,
  ContextFillRatio,
} from '../types'

/** Standard tool definitions used for automated testing. */
export const autoTestTools: ChatToolDefinition[] = [
  {
    type: 'function',
    function: {
      name: 'get_weather',
      description: 'Get current weather for a city',
      parameters: {
        type: 'object',
        properties: {
          location: { type: 'string', description: 'City name' },
          unit: { type: 'string', enum: ['celsius', 'fahrenheit'] },
        },
        required: ['location'],
      },
    },
  },
  {
    type: 'function',
    function: {
      name: 'add',
      description: 'Add two numbers',
      parameters: {
        type: 'object',
        properties: {
          a: { type: 'number' },
          b: { type: 'number' },
        },
        required: ['a', 'b'],
      },
    },
  },
  {
    type: 'function',
    function: {
      name: 'search_products',
      description: 'Search for products by name or category',
      parameters: {
        type: 'object',
        properties: {
          query: { type: 'string', description: 'Search query' },
          category: { type: 'string', description: 'Product category' },
          max_results: { type: 'number', description: 'Maximum number of results to return' },
        },
        required: ['query'],
      },
    },
  },
  {
    type: 'function',
    function: {
      name: 'send_email',
      description: 'Send an email to a recipient',
      parameters: {
        type: 'object',
        properties: {
          to: { type: 'string', description: 'Recipient email address' },
          subject: { type: 'string', description: 'Email subject line' },
          body: { type: 'string', description: 'Email body content' },
        },
        required: ['to', 'subject', 'body'],
      },
    },
  },
  {
    type: 'function',
    function: {
      name: 'get_stock_price',
      description: 'Get the current stock price for a ticker symbol',
      parameters: {
        type: 'object',
        properties: {
          symbol: { type: 'string', description: 'Stock ticker symbol (e.g. AAPL)' },
        },
        required: ['symbol'],
      },
    },
  },
  {
    type: 'function',
    function: {
      name: 'convert_currency',
      description: 'Convert an amount from one currency to another',
      parameters: {
        type: 'object',
        properties: {
          amount: { type: 'number', description: 'Amount to convert' },
          from: { type: 'string', description: 'Source currency code (e.g. USD)' },
          to: { type: 'string', description: 'Target currency code (e.g. EUR)' },
        },
        required: ['amount', 'from', 'to'],
      },
    },
  },
  {
    type: 'function',
    function: {
      name: 'create_calendar_event',
      description: 'Create a calendar event',
      parameters: {
        type: 'object',
        properties: {
          title: { type: 'string', description: 'Event title' },
          date: { type: 'string', description: 'Event date in YYYY-MM-DD format' },
          time: { type: 'string', description: 'Event time in HH:MM format' },
          duration_minutes: { type: 'number', description: 'Duration in minutes' },
        },
        required: ['title', 'date', 'time'],
      },
    },
  },
  {
    type: 'function',
    function: {
      name: 'translate_text',
      description: 'Translate text from one language to another',
      parameters: {
        type: 'object',
        properties: {
          text: { type: 'string', description: 'Text to translate' },
          source_language: { type: 'string', description: 'Source language code (e.g. en)' },
          target_language: { type: 'string', description: 'Target language code (e.g. fr)' },
        },
        required: ['text', 'target_language'],
      },
    },
  },
  {
    type: 'function',
    function: {
      name: 'get_directions',
      description: 'Get driving directions between two locations',
      parameters: {
        type: 'object',
        properties: {
          origin: { type: 'string', description: 'Starting location' },
          destination: { type: 'string', description: 'Destination location' },
          mode: { type: 'string', enum: ['driving', 'walking', 'transit'], description: 'Travel mode' },
        },
        required: ['origin', 'destination'],
      },
    },
  },
  {
    type: 'function',
    function: {
      name: 'set_reminder',
      description: 'Set a reminder for a specific time',
      parameters: {
        type: 'object',
        properties: {
          message: { type: 'string', description: 'Reminder message' },
          datetime: { type: 'string', description: 'When to trigger the reminder (ISO 8601)' },
        },
        required: ['message', 'datetime'],
      },
    },
  },
]

/** System prompt used across all cache-aware scenarios. */
const cacheSystemPrompt = 'You are a helpful assistant. You answer concisely and accurately. When asked for numbers, respond with only the number. When asked for lists, use numbered format.';

/** Chat scenario — tests basic text generation quality. */
export const chatScenario: AutoTestScenario = {
  id: 'chat',
  name: 'Chat Quality',
  prompts: [
    {
      id: 'math-multiply',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is 17 * 19? Answer with only the number.' },
      ],
      expected: { type: 'exact', value: '323' },
    },
    {
      id: 'list-benefits',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'List exactly 3 benefits of exercise. Use a numbered list.' },
      ],
      expected: { type: 'regex', value: '^\\s*1[.)]\\s+.+\\n\\s*2[.)]\\s+.+\\n\\s*3[.)]\\s+.+' },
    },
    {
      id: 'translate-french',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "Translate 'Good morning' to French. Answer with only the translation." },
      ],
      expected: { type: 'regex', value: 'bonjour' },
    },
    // Multi-turn prompts: each extends the previous conversation to exercise
    // IMC incremental caching. The system prompt is shared with the single-turn
    // prompts above so SPC benefits from the same cached KV state.
    {
      id: 'multi-turn-2',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is the capital of France?' },
        { role: 'assistant', content: 'Paris' },
        { role: 'user', content: 'What is the population of that city? Answer with only the approximate number.' },
      ],
      expected: { type: 'regex', value: '\\d' },
    },
    {
      id: 'multi-turn-4',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is the capital of France?' },
        { role: 'assistant', content: 'Paris' },
        { role: 'user', content: 'What is the population of that city? Answer with only the approximate number.' },
        { role: 'assistant', content: 'Approximately 2.1 million in the city proper.' },
        { role: 'user', content: 'Name exactly 3 famous landmarks there. Use a numbered list.' },
      ],
      expected: { type: 'regex', value: '1[.)]' },
    },
    {
      id: 'multi-turn-6',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is the capital of France?' },
        { role: 'assistant', content: 'Paris' },
        { role: 'user', content: 'What is the population of that city? Answer with only the approximate number.' },
        { role: 'assistant', content: 'Approximately 2.1 million in the city proper.' },
        { role: 'user', content: 'Name exactly 3 famous landmarks there. Use a numbered list.' },
        { role: 'assistant', content: '1. Eiffel Tower\n2. Louvre Museum\n3. Notre-Dame Cathedral' },
        { role: 'user', content: 'When was the first one built? Answer with only the year.' },
      ],
      expected: { type: 'regex', value: '18(87|89)' },
    },
    {
      id: 'math-addition',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is 456 + 789? Answer with only the number.' },
      ],
      expected: { type: 'exact', value: '1245' },
    },
    {
      id: 'math-division',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is 144 / 12? Answer with only the number.' },
      ],
      expected: { type: 'exact', value: '12' },
    },
    {
      id: 'translate-spanish',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "Translate 'Thank you very much' to Spanish. Answer with only the translation." },
      ],
      expected: { type: 'regex', value: '[Mm]uchas\\s+[Gg]racias' },
    },
    {
      id: 'country-capital',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is the capital of Japan? Answer with only the city name.' },
      ],
      expected: { type: 'exact', value: 'Tokyo' },
    },
    {
      id: 'list-planets',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'List the 4 inner planets of our solar system. Use a numbered list.' },
      ],
      expected: { type: 'regex', value: '1[.)].*[Mm]ercury' },
    },
    {
      id: 'acronym',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What does the acronym HTTP stand for? Answer with only the full form.' },
      ],
      expected: { type: 'regex', value: '[Hh]yper[Tt]ext\\s+[Tt]ransfer\\s+[Pp]rotocol' },
    },
    {
      id: 'multi-turn-8',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is the capital of France?' },
        { role: 'assistant', content: 'Paris' },
        { role: 'user', content: 'What is the population of that city? Answer with only the approximate number.' },
        { role: 'assistant', content: 'Approximately 2.1 million in the city proper.' },
        { role: 'user', content: 'Name exactly 3 famous landmarks there. Use a numbered list.' },
        { role: 'assistant', content: '1. Eiffel Tower\n2. Louvre Museum\n3. Notre-Dame Cathedral' },
        { role: 'user', content: 'When was the first one built? Answer with only the year.' },
        { role: 'assistant', content: '1889' },
        { role: 'user', content: 'How tall is it in meters? Answer with only the number.' },
      ],
      expected: { type: 'regex', value: '3(00|30)' },
    },
    {
      id: 'multi-turn-10',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is the capital of France?' },
        { role: 'assistant', content: 'Paris' },
        { role: 'user', content: 'What is the population of that city? Answer with only the approximate number.' },
        { role: 'assistant', content: 'Approximately 2.1 million in the city proper.' },
        { role: 'user', content: 'Name exactly 3 famous landmarks there. Use a numbered list.' },
        { role: 'assistant', content: '1. Eiffel Tower\n2. Louvre Museum\n3. Notre-Dame Cathedral' },
        { role: 'user', content: 'When was the first one built? Answer with only the year.' },
        { role: 'assistant', content: '1889' },
        { role: 'user', content: 'How tall is it in meters? Answer with only the number.' },
        { role: 'assistant', content: '330' },
        { role: 'user', content: 'Who designed it? Answer with only the name.' },
      ],
      expected: { type: 'regex', value: '[Ee]iffel' },
    },
    {
      id: 'math-square-root',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is the square root of 256? Answer with only the number.' },
      ],
      expected: { type: 'exact', value: '16' },
    },
  ],
}

/**
 * Background text blocks used to fill the context window with meaningful content.
 * Each block is approximately 200-300 words of real-ish technical content that
 * an LLM can reason about when asked follow-up questions.
 */
const contextFillBlocks = [
  `The distributed caching layer uses a consistent hashing ring with 256 virtual nodes per physical server. When a node joins or leaves the cluster, only K/N keys need to be redistributed, where K is the total number of keys and N is the number of nodes. The system maintains a replication factor of 3, meaning each key-value pair is stored on three consecutive nodes in the ring. Write operations require acknowledgment from at least two replicas before returning success to the client. Read operations can be configured for either strong consistency (read from all replicas and return the latest version) or eventual consistency (read from the nearest replica). The cache eviction policy uses a combination of LRU and TTL-based expiration. Each entry has a maximum TTL of 24 hours, but frequently accessed entries are kept in a hot tier with faster access times. The system processes approximately 50,000 requests per second per node with a p99 latency of 2.3 milliseconds.`,

  `The machine learning pipeline ingests training data from three sources: user interaction logs, content metadata databases, and external knowledge graphs. The preprocessing stage normalizes text using Unicode NFKC normalization, removes HTML entities, and applies language-specific tokenization. For English text, the system uses a SentencePiece BPE tokenizer with a vocabulary of 32,000 tokens. The model architecture is a transformer with 24 layers, 16 attention heads, and a hidden dimension of 1024. Training uses AdamW optimizer with a learning rate of 3e-4, weight decay of 0.01, and a cosine learning rate schedule with 1000 warmup steps. The batch size is 256 sequences of 512 tokens each. Training runs on 8 A100 GPUs using data parallelism with gradient accumulation over 4 steps. A full training run takes approximately 72 hours and produces a model checkpoint of 1.2 GB.`,

  `The API gateway handles authentication using JWT tokens with RS256 signing. Each token contains claims for user ID, organization ID, role, and a list of permitted scopes. Tokens expire after 1 hour and can be refreshed using a separate refresh token that expires after 30 days. Rate limiting is implemented at three levels: per-IP (1000 requests/minute), per-user (5000 requests/minute), and per-organization (50000 requests/minute). The gateway routes requests to backend services using a weighted round-robin algorithm with health checking. If a backend service fails three consecutive health checks (sent every 10 seconds), it is removed from the rotation until it passes two consecutive checks. The gateway logs all requests with a unique trace ID that propagates through all downstream services. Request and response bodies are logged only for non-production environments. The system handles approximately 10 million API calls per day across 150 different endpoint paths.`,

  `The database schema uses a multi-tenant architecture with row-level security. Each tenant's data is isolated using a tenant_id column present on every table. The primary tables are: users (12 columns, ~2M rows per tenant), documents (18 columns, ~500K rows per tenant), and events (8 columns, ~50M rows per tenant). Indexes are maintained on all foreign keys and commonly queried columns. The events table uses a time-series partitioning scheme with monthly partitions, and partitions older than 12 months are automatically moved to cold storage. The database runs on PostgreSQL 16 with connection pooling via PgBouncer configured for 200 server connections and 2000 client connections. Queries are optimized using materialized views for common aggregation patterns, which are refreshed every 15 minutes. The backup strategy uses continuous WAL archiving to S3 with a recovery point objective of 5 minutes and full base backups taken daily.`,

  `The monitoring system collects metrics from all services using a pull-based model with 15-second scrape intervals. Metrics are stored in a time-series database with 90-day retention at full resolution and 2-year retention at 5-minute aggregation. The system tracks four golden signals for each service: latency (histogram with buckets at 10ms, 50ms, 100ms, 250ms, 500ms, 1s, 5s), traffic (requests per second by endpoint and status code), errors (error rate as a percentage of total requests), and saturation (CPU, memory, disk I/O, and network utilization). Alerts are configured using multi-window multi-burn-rate SLO-based rules. For example, the API gateway has an SLO of 99.9% availability measured over a 30-day window, with fast-burn alerts (14.4x burn rate over 1 hour) triggering pages and slow-burn alerts (3x burn rate over 6 hours) creating tickets. Dashboard visualizations use Grafana with templated dashboards that automatically discover new services and endpoints.`,

  `The CI/CD pipeline consists of five stages: lint, test, build, deploy-staging, and deploy-production. The lint stage runs ESLint, Prettier, and type checking in parallel, completing in approximately 45 seconds. The test stage runs unit tests (Jest with ~4000 test cases, ~3 minutes), integration tests (against Docker Compose services, ~8 minutes), and end-to-end tests (Playwright against a staging-like environment, ~12 minutes). The build stage creates Docker images using multi-stage builds, produces SBOM manifests, and runs Trivy vulnerability scanning. Images are pushed to a private ECR registry with immutable tags based on the Git commit SHA. Deployment uses a progressive delivery model: staging receives every commit to the main branch automatically, while production deployments require manual approval and use a canary strategy that routes 5% of traffic to the new version for 30 minutes before proceeding to full rollout. Rollbacks are automated if the error rate exceeds 1% during the canary phase.`,

  `The search infrastructure uses an inverted index with BM25 scoring enhanced by a learned re-ranking model. Documents are processed through a pipeline that extracts text, splits into passages of approximately 200 tokens, generates embedding vectors using a bi-encoder model, and builds the inverted index. The index supports prefix matching, fuzzy matching with edit distance up to 2, and phrase queries using positional information. At query time, an initial retrieval phase uses the inverted index to find the top 1000 candidate passages by BM25 score. A re-ranking phase then applies a cross-encoder model to score each candidate against the query, producing the final top 20 results. The entire query pipeline completes in under 200 milliseconds for 95% of queries. The index is updated incrementally using a write-ahead log that batches updates every 5 seconds. The system handles approximately 500 queries per second with a corpus of 100 million passages across 10 shards.`,

  `The event streaming platform processes messages using a partitioned log architecture. Events are published to topics, each divided into 12 partitions for parallel processing. Producers use a hash-based partitioning strategy on the event key to ensure ordering within a partition. Consumers form groups where each partition is assigned to exactly one consumer in the group. The system guarantees at-least-once delivery with exactly-once processing semantics achieved through idempotent consumers and transactional outbox patterns. Message retention is configured per topic: user events are retained for 30 days, system events for 7 days, and audit events for 365 days. The platform handles approximately 2 million events per second with an average end-to-end latency of 15 milliseconds. Schema evolution is managed using a schema registry that enforces backward compatibility. Each message includes a schema ID in its header, and consumers automatically deserialize using the correct schema version.`,

  `The content delivery network caches static and dynamic content across 45 points of presence worldwide. Static assets (images, CSS, JavaScript, fonts) are cached with a TTL of 30 days and use content-based hashing in filenames for cache busting. Dynamic content (API responses, personalized pages) uses surrogate keys for targeted cache invalidation. When a product is updated, the system publishes an invalidation event containing the product's surrogate keys, and all edge nodes purge matching entries within 500 milliseconds. The CDN uses HTTP/2 push for critical assets and supports QUIC/HTTP3 for improved performance on mobile networks. TLS termination occurs at the edge using ECDSA certificates with OCSP stapling. The system serves approximately 500 TB of data per month with a cache hit ratio of 94% for static content and 72% for dynamic content. Origin shield nodes reduce origin load by consolidating cache misses from multiple edge nodes in the same region.`,

  `The identity and access management system supports multiple authentication methods: password-based, SSO via SAML 2.0 and OIDC, passwordless via WebAuthn/FIDO2, and API keys for service-to-service communication. Password policies enforce minimum 12 characters with complexity requirements and check against a database of 600 million compromised passwords. Multi-factor authentication supports TOTP, SMS (as fallback only), and hardware security keys. The authorization model uses a hybrid RBAC/ABAC approach. Base permissions are defined through roles (admin, editor, viewer, custom), while fine-grained access rules use attribute-based policies that evaluate request context (time of day, IP geolocation, device trust score) and resource attributes (classification level, owner, creation date). Policy decisions are cached for 5 minutes and re-evaluated on any privilege change. The system processes approximately 100,000 authentication requests and 2 million authorization decisions per minute. Session management uses encrypted, httpOnly, SameSite=Strict cookies with a 15-minute idle timeout and 12-hour absolute timeout.`,
];

/** Final user prompts for context-fill tests. These ask the LLM to reason about
 *  the background content in a meaningful way without requiring a predetermined answer. */
const contextFillQuestions = [
  'Based on the background information provided, identify the three most critical architectural decisions and explain how they impact system reliability and performance. Structure your response with clear headings.',
  'Analyze the background material and describe what would happen if the system needed to scale to 10x its current traffic. What components would become bottlenecks first? Provide a prioritized list.',
  'From the technical details shared above, create a concise executive summary highlighting the key metrics, design patterns, and trade-offs. Focus on what a new team member would need to understand first.',
];

/**
 * Builds a context-fill prompt whose conversation history targets a specific
 * fraction of the context window. The fill is achieved by including multiple
 * "background note" user/assistant message pairs.
 *
 * @param ratio - Target fill ratio (0.2, 0.5, or 0.8)
 * @param label - Human-readable label ('20%', '50%', '80%')
 * @param targetTokens - Approximate number of prompt tokens to aim for
 * @param charMultiplier - Characters-per-token estimate (adjusted by calibration)
 */
export function buildContextFillPrompt(
  ratio: number,
  label: ContextFillRatio,
  targetTokens: number,
  charMultiplier: number = 3.5,
): AutoTestPromptDef {
  const targetChars = Math.floor(targetTokens * charMultiplier);

  // Reserve chars for system prompt + final question + overhead
  const systemText = `${cacheSystemPrompt} Use the background notes provided in the conversation to inform your answer.`;
  const questionIdx = ratio <= 0.25 ? 0 : ratio <= 0.55 ? 1 : 2;
  const finalQuestion = contextFillQuestions[questionIdx];
  const overhead = systemText.length + finalQuestion.length + 500; // 500 for role tags, formatting
  const fillChars = Math.max(200, targetChars - overhead);

  // Build background message pairs from the block pool
  const messages: { role: 'system' | 'user' | 'assistant'; content: string }[] = [
    { role: 'system', content: systemText },
  ];

  let charsAdded = 0;
  let blockIdx = 0;
  const totalBlocks = contextFillBlocks.length;

  while (charsAdded < fillChars) {
    const block = contextFillBlocks[blockIdx % totalBlocks];
    const remaining = fillChars - charsAdded;

    // If the full block fits, add it whole; otherwise trim
    const text = remaining >= block.length
      ? block
      : block.slice(0, remaining);

    messages.push(
      { role: 'user', content: `Background notes section ${blockIdx + 1}:\n${text}` },
      { role: 'assistant', content: 'Acknowledged. I have processed these background notes. Please continue or ask your question.' },
    );

    charsAdded += text.length;
    blockIdx++;

    // Safety: cap based on target size to avoid infinite loops
    if (blockIdx > Math.ceil(fillChars / 200) + totalBlocks) break;
  }

  // Final user question
  messages.push({ role: 'user', content: finalQuestion });

  return {
    id: `ctxfill-${label.replace('%', '')}`,
    messages,
    max_tokens: 512,
    expected: undefined,
    contextFill: { ratio, label },
    includeInScore: false,
  };
}

/**
 * Calibrates context-fill prompts by sending a cheap request (max_tokens=1) and
 * adjusting the character multiplier based on actual token counts from the server.
 * Returns calibrated prompts for each fill ratio.
 */
export async function calibrateContextFillPrompts(
  sessionId: string,
  contextWindow: number,
  signal?: AbortSignal,
): Promise<AutoTestPromptDef[]> {
  const ratios: Array<{ ratio: number; label: ContextFillRatio }> = [
    { ratio: 0.20, label: '20%' },
    { ratio: 0.50, label: '50%' },
    { ratio: 0.80, label: '80%' },
  ];

  const calibrated: AutoTestPromptDef[] = [];
  const safetyMargin = 256;

  for (const { ratio, label } of ratios) {
    const targetTokens = Math.floor(contextWindow * ratio) - safetyMargin;
    if (targetTokens < 100) {
      // Context window too small for this ratio
      continue;
    }

    let charMultiplier = 3.5; // Initial estimate: ~3.5 chars per token
    let bestPrompt = buildContextFillPrompt(ratio, label, targetTokens, charMultiplier);

    // Up to 3 calibration iterations
    for (let iter = 0; iter < 3; iter++) {
      if (signal?.aborted) break;

      try {
        const result = await runSinglePrompt(
          sessionId,
          { ...bestPrompt, max_tokens: 1 },
          {},
          signal,
        );

        const actualPromptTokens = result.usage?.prompt_tokens;
        if (!actualPromptTokens || actualPromptTokens <= 0) break;

        const actualFill = actualPromptTokens / contextWindow;
        const tolerance = 0.03; // ±3%

        if (Math.abs(actualFill - ratio) <= tolerance) {
          break; // Close enough
        }

        // Adjust multiplier proportionally
        charMultiplier = charMultiplier * (targetTokens / actualPromptTokens);
        bestPrompt = buildContextFillPrompt(ratio, label, targetTokens, charMultiplier);
      } catch {
        break; // Use best effort
      }
    }

    // Restore the real max_tokens
    bestPrompt.max_tokens = 512;
    calibrated.push(bestPrompt);
  }

  return calibrated;
}

/**
 * Extracts the context window size from effective config or base config.
 */
export function extractContextWindow(
  effectiveConfig?: Record<string, unknown>,
  baseConfig?: PlaygroundModelConfig,
): number | undefined {
  if (effectiveConfig) {
    const cw = effectiveConfig['context_window'] ?? effectiveConfig['context-window'];
    if (typeof cw === 'number' && cw > 0) return cw;
  }
  if (baseConfig) {
    const cw = baseConfig['context_window'];
    if (typeof cw === 'number' && cw > 0) return cw;
  }
  return undefined;
}

/** Tool call scenario — tests tool/function calling capability. */
export const toolCallScenario: AutoTestScenario = {
  id: 'tool_call',
  name: 'Tool Calling',
  prompts: [
    {
      id: 'weather-tool',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Boston?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'add-tool',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is 15 + 28?' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // Multi-turn tool calling: prior conversation cached by IMC, only the
    // last user message triggers a new tool call.
    {
      id: 'multi-turn-tool',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Boston?" },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc1', index: 0, type: 'function', function: { name: 'get_weather', arguments: '{"location":"Boston"}' } }] },
        { role: 'user', content: "Now check the weather in London too." },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'weather-tokyo',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather like in Tokyo right now?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'weather-celsius',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "Tell me the weather in Paris in celsius." },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'add-large',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Calculate 1234 + 5678 for me.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'add-decimals',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is 3.14 + 2.72?' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'search-laptop',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Search for laptops under $1000.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'search-headphones',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Find me wireless headphones in the electronics category.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'send-email',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Send an email to alice@example.com with subject "Meeting Tomorrow" and body "Let\'s meet at 3pm."' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'stock-price-aapl',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's Apple's current stock price?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'stock-price-tsla',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Check the stock price of Tesla.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'convert-usd-eur',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Convert 100 USD to EUR.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'convert-gbp-jpy',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'How much is 500 British pounds in Japanese yen?' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'calendar-meeting',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Schedule a team meeting for 2025-03-15 at 10:00 for 60 minutes.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'calendar-lunch',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Create a lunch event on 2025-04-01 at 12:30.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'translate-hello-spanish',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "Translate 'Hello, how are you?' to Spanish." },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'translate-goodbye-german',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "Translate 'Goodbye and thank you' to German." },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'directions-sf-la',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Get me driving directions from San Francisco to Los Angeles.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'directions-walking',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'How do I walk from Central Park to Times Square?' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'reminder-dentist',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Remind me about my dentist appointment at 2025-03-20T09:00:00.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'reminder-groceries',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Set a reminder to buy groceries tomorrow at 5pm.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // Multi-turn tool calling with different tools
    {
      id: 'multi-turn-tool-add',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is 10 + 20?' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2', index: 0, type: 'function', function: { name: 'add', arguments: '{"a":10,"b":20}' } }] },
        { role: 'user', content: 'Now add 30 + 40.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'multi-turn-tool-weather-3',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Boston?" },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc3', index: 0, type: 'function', function: { name: 'get_weather', arguments: '{"location":"Boston"}' } }] },
        { role: 'user', content: "Now check the weather in London too." },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc4', index: 0, type: 'function', function: { name: 'get_weather', arguments: '{"location":"London"}' } }] },
        { role: 'user', content: "And what about Sydney?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'multi-turn-tool-stock-follow',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's Apple's stock price?" },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc5', index: 0, type: 'function', function: { name: 'get_stock_price', arguments: '{"symbol":"AAPL"}' } }] },
        { role: 'user', content: 'Now check Google too.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'search-books',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Search for science fiction books, show me the top 5 results.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'directions-transit',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'How do I get from Brooklyn to Manhattan by transit?' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- New single-turn prompts exercising optional params & broader tool coverage ---
    {
      id: 'tc2-weather-fahrenheit-nyc',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in New York in fahrenheit?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-weather-berlin-celsius',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Get the weather in Berlin in celsius.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-add-negative',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is -15 + 42?' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-add-small',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Add 7 + 13.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-search-kitchen-max3',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Search for "coffee mug" in category kitchen and return at most 3 results.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-search-usbc-max1',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Search for USB-C cable and limit to 1 result.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-send-email-brief',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Email bob@example.com with subject "Status" and body "All tests passed."' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-stock-msft',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Get the stock price for MSFT.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-convert-eur-usd',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Convert 49.99 EUR to USD.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-calendar-duration-30',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Create a calendar event titled "1:1" on 2025-05-20 at 09:30 for 30 minutes.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-translate-en-fr',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Translate "Where is the train station?" from English to French.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-directions-transit-mode',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Get transit directions from "Union Square, San Francisco" to "SFO Airport".' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- New moderate multi-turn prompts (2-3 tool calls; cross-tool sequences) ---
    {
      id: 'tc2-mt-weather-unit-followup',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Boston?" },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2w1', index: 0, type: 'function', function: { name: 'get_weather', arguments: '{"location":"Boston"}' } }] },
        { role: 'user', content: 'Same city, but in fahrenheit.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-mt-search-then-email',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Search for running shoes.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2se1', index: 0, type: 'function', function: { name: 'search_products', arguments: '{"query":"running shoes"}' } }] },
        { role: 'user', content: 'Email those results to alice@example.com with subject "Shoe Options" and body "Here are the running shoes I found."' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-mt-stock-then-reminder',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's Apple's stock price?" },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2sr1', index: 0, type: 'function', function: { name: 'get_stock_price', arguments: '{"symbol":"AAPL"}' } }] },
        { role: 'user', content: 'Set a reminder at 2025-06-01T09:00:00 to check again.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-mt-currency-then-calendar',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Convert 100 USD to EUR.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2cc1', index: 0, type: 'function', function: { name: 'convert_currency', arguments: '{"amount":100,"from":"USD","to":"EUR"}' } }] },
        { role: 'user', content: 'Schedule a budget review for 2025-07-10 at 14:00 for 45 minutes.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-mt-directions-then-weather',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Get me driving directions from Seattle to Portland.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2dw1', index: 0, type: 'function', function: { name: 'get_directions', arguments: '{"origin":"Seattle","destination":"Portland"}' } }] },
        { role: 'user', content: "What's the weather in Portland in celsius?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-mt-translate-then-email',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Translate "Meeting postponed to next week" to Spanish.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2te1', index: 0, type: 'function', function: { name: 'translate_text', arguments: '{"text":"Meeting postponed to next week","target_language":"es"}' } }] },
        { role: 'user', content: 'Send that translation to carlos@example.com with subject "Update" and body "Reunión pospuesta para la próxima semana."' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-mt-reminder-reschedule',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Set a reminder for "Submit report" at 2025-04-10T17:00:00.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2rr1', index: 0, type: 'function', function: { name: 'set_reminder', arguments: '{"message":"Submit report","datetime":"2025-04-10T17:00:00"}' } }] },
        { role: 'user', content: 'Actually set it for 2025-04-11T10:00:00 instead.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-mt-add-then-convert',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is 19.99 + 5.00?' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2ac1', index: 0, type: 'function', function: { name: 'add', arguments: '{"a":19.99,"b":5.00}' } }] },
        { role: 'assistant', content: 'The total is 24.99 USD.' },
        { role: 'user', content: 'Convert 24.99 USD to GBP.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-mt-calendar-then-directions',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Create a calendar event titled "Dinner at Luigi\'s" on 2025-08-15 at 19:00.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2cd1', index: 0, type: 'function', function: { name: 'create_calendar_event', arguments: '{"title":"Dinner at Luigi\'s","date":"2025-08-15","time":"19:00"}' } }] },
        { role: 'user', content: 'Get me walking directions from home to Luigi\'s Restaurant.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- Deep chain multi-turn prompts (4+ tool-call turns; multi-tool sequences) ---
    {
      id: 'tc2-mt-trip-planner',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Rome in celsius?" },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2tp1', index: 0, type: 'function', function: { name: 'get_weather', arguments: '{"location":"Rome","unit":"celsius"}' } }] },
        { role: 'user', content: 'Search for travel umbrella in category travel, max 3 results.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2tp2', index: 0, type: 'function', function: { name: 'search_products', arguments: '{"query":"travel umbrella","category":"travel","max_results":3}' } }] },
        { role: 'user', content: 'Convert 200 USD to EUR.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2tp3', index: 0, type: 'function', function: { name: 'convert_currency', arguments: '{"amount":200,"from":"USD","to":"EUR"}' } }] },
        { role: 'user', content: 'Create a calendar event titled "Flight to Rome" on 2025-09-01 at 08:00 for 30 minutes.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2tp4', index: 0, type: 'function', function: { name: 'create_calendar_event', arguments: '{"title":"Flight to Rome","date":"2025-09-01","time":"08:00","duration_minutes":30}' } }] },
        { role: 'user', content: 'Email me the trip checklist at traveler@example.com with subject "Rome Trip Checklist" and body "Umbrella, euros, and flight booked."' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-mt-meeting-runner',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Create a calendar event titled "Client Meeting" on 2025-06-15 at 14:00 for 60 minutes.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2mr1', index: 0, type: 'function', function: { name: 'create_calendar_event', arguments: '{"title":"Client Meeting","date":"2025-06-15","time":"14:00","duration_minutes":60}' } }] },
        { role: 'user', content: 'Get driving directions from "123 Main St" to "456 Oak Ave".' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2mr2', index: 0, type: 'function', function: { name: 'get_directions', arguments: '{"origin":"123 Main St","destination":"456 Oak Ave","mode":"driving"}' } }] },
        { role: 'user', content: "What's the weather at 456 Oak Ave in fahrenheit?" },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2mr3', index: 0, type: 'function', function: { name: 'get_weather', arguments: '{"location":"456 Oak Ave","unit":"fahrenheit"}' } }] },
        { role: 'user', content: 'Translate "Running 10 minutes late" to Spanish.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2mr4', index: 0, type: 'function', function: { name: 'translate_text', arguments: '{"text":"Running 10 minutes late","target_language":"es"}' } }] },
        { role: 'user', content: 'Send an email to client@example.com with subject "On My Way" and body "Llego 10 minutos tarde."' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-mt-shopping-budget',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Search for headphones in electronics, max 5 results.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2sb1', index: 0, type: 'function', function: { name: 'search_products', arguments: '{"query":"headphones","category":"electronics","max_results":5}' } }] },
        { role: 'user', content: 'Add 79.99 + 12.50.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2sb2', index: 0, type: 'function', function: { name: 'add', arguments: '{"a":79.99,"b":12.50}' } }] },
        { role: 'user', content: 'Convert 92.49 USD to GBP.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2sb3', index: 0, type: 'function', function: { name: 'convert_currency', arguments: '{"amount":92.49,"from":"USD","to":"GBP"}' } }] },
        { role: 'user', content: 'Set a reminder at 2025-07-01T12:00:00 to check for price drops.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2sb4', index: 0, type: 'function', function: { name: 'set_reminder', arguments: '{"message":"Check for price drops","datetime":"2025-07-01T12:00:00"}' } }] },
        { role: 'user', content: 'Email the budget summary to finance@example.com with subject "Headphone Budget" and body "Total: 92.49 USD / ~73 GBP."' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-mt-multicity-itinerary',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Seattle?" },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2mi1', index: 0, type: 'function', function: { name: 'get_weather', arguments: '{"location":"Seattle"}' } }] },
        { role: 'user', content: "What's the weather in Vancouver in celsius?" },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2mi2', index: 0, type: 'function', function: { name: 'get_weather', arguments: '{"location":"Vancouver","unit":"celsius"}' } }] },
        { role: 'user', content: 'Get transit directions from Seattle to Vancouver.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2mi3', index: 0, type: 'function', function: { name: 'get_directions', arguments: '{"origin":"Seattle","destination":"Vancouver","mode":"transit"}' } }] },
        { role: 'user', content: 'Convert 100 USD to CAD.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2mi4', index: 0, type: 'function', function: { name: 'convert_currency', arguments: '{"amount":100,"from":"USD","to":"CAD"}' } }] },
        { role: 'user', content: 'Create a calendar event titled "Cross-border day trip" on 2025-08-20 at 07:00 for 480 minutes.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc2-mt-language-lunch-flow',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Translate "I would like a table for two, please" to Italian.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2ll1', index: 0, type: 'function', function: { name: 'translate_text', arguments: '{"text":"I would like a table for two, please","target_language":"it"}' } }] },
        { role: 'user', content: 'Send that phrase to host@example.com with subject "Reservation Request" and body "Vorrei un tavolo per due, per favore."' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2ll2', index: 0, type: 'function', function: { name: 'send_email', arguments: '{"to":"host@example.com","subject":"Reservation Request","body":"Vorrei un tavolo per due, per favore."}' } }] },
        { role: 'user', content: 'Create a calendar event titled "Lunch at Trattoria" on 2025-09-05 at 12:30.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2ll3', index: 0, type: 'function', function: { name: 'create_calendar_event', arguments: '{"title":"Lunch at Trattoria","date":"2025-09-05","time":"12:30"}' } }] },
        { role: 'user', content: 'Get walking directions from "Hotel Bella Vista" to "Trattoria Roma".' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2ll4', index: 0, type: 'function', function: { name: 'get_directions', arguments: '{"origin":"Hotel Bella Vista","destination":"Trattoria Roma","mode":"walking"}' } }] },
        { role: 'user', content: 'Set a reminder at 2025-09-05T12:00:00 to leave for lunch.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- Parallel / multiple tool calls in a single response ---
    {
      id: 'tc3-parallel-weather-2cities',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Boston and London?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-parallel-stocks-2',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Check the current stock prices for AAPL and TSLA.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-parallel-weather-stock',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Paris and what's the current price of AAPL?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-parallel-search-convert',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Search for "wireless headphones" and also convert 199.99 USD to EUR.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-parallel-calendar-reminder',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Create a calendar event titled "Dentist" on 2026-03-10 at 09:00 for 30 minutes, and set a reminder for 2026-03-10T08:30:00 saying "Leave for dentist".' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-parallel-convert-2',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Convert 100 USD to EUR and 500 GBP to JPY.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-parallel-directions-weather',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Get driving directions from San Francisco to San Jose, and tell me the weather in San Jose.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- Unicode / special character / edge cases in arguments ---
    {
      id: 'tc3-weather-unicode',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in São Paulo?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-weather-unicode-umlaut',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Zürich?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-stock-dot-symbol',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Get the stock price for BRK.B.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-email-plus-address',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Send an email to alice+test@example.com with subject "Test" and body "Hello from the test suite."' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-translate-quotes',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Translate the phrase \'He said "hello" and left\' to French.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-calendar-leapday',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Create a calendar event titled "Leap Day Party" on 2028-02-29 at 20:00.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-reminder-timezone',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Set a reminder "Standup" for 2026-02-23T09:30:00-05:00.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-convert-large-amount',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Convert 1000000 USD to JPY.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-directions-unicode',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Get walking directions from "Café de Flore" to "Musée d\'Orsay".' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- Self-correction prompts (user changes mind mid-prompt) ---
    {
      id: 'tc3-correction-location',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in London—actually make that Dublin." },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-correction-currency',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Convert 100 USD to EUR—sorry, convert 100 USD to GBP instead.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-correction-destination',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Get driving directions from Boston to New York—wait, to Philadelphia.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- Fill multi-turn final-tool coverage gaps ---
    {
      id: 'tc3-mt-final-search',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Tokyo?" },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc3fs1', index: 0, type: 'function', function: { name: 'get_weather', arguments: '{"location":"Tokyo"}' } }] },
        { role: 'user', content: 'Search for "Japanese tea set" in category kitchen, max 3 results.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-mt-final-translate',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Get walking directions from "Piazza Navona" to "Colosseum".' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc3ft1', index: 0, type: 'function', function: { name: 'get_directions', arguments: '{"origin":"Piazza Navona","destination":"Colosseum","mode":"walking"}' } }] },
        { role: 'user', content: 'Translate "How far is the Colosseum?" from English to Italian.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- Indirect phrasing that still requires tool calls ---
    {
      id: 'tc3-stock-by-company',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What is Microsoft's stock price?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-reminder-indirect',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'At 2026-04-01T12:00:00 remind me to "renew subscription".' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- Multi-turn with scripted prior tools not yet used as priors ---
    {
      id: 'tc3-mt-prior-convert-then-weather',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Convert 10 USD to EUR.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc3pcw1', index: 0, type: 'function', function: { name: 'convert_currency', arguments: '{"amount":10,"from":"USD","to":"EUR"}' } }] },
        { role: 'user', content: "Now what's the weather in Berlin?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-mt-prior-email-then-translate',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Send an email to bob@example.com with subject "Hi" and body "Test message."' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc3pet1', index: 0, type: 'function', function: { name: 'send_email', arguments: '{"to":"bob@example.com","subject":"Hi","body":"Test message."}' } }] },
        { role: 'user', content: 'Now translate "Good night" to French.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-mt-prior-calendar-then-stock',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Create a calendar event titled "Review" on 2026-01-15 at 11:00.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc3pcs1', index: 0, type: 'function', function: { name: 'create_calendar_event', arguments: '{"title":"Review","date":"2026-01-15","time":"11:00"}' } }] },
        { role: 'user', content: "What's the stock price for NVDA?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-mt-prior-reminder-then-search',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Set a reminder "Buy gift" for 2026-05-01T10:00:00.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc3prs1', index: 0, type: 'function', function: { name: 'set_reminder', arguments: '{"message":"Buy gift","datetime":"2026-05-01T10:00:00"}' } }] },
        { role: 'user', content: 'Search for "birthday gift ideas" with max 5 results.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-mt-prior-translate-then-add',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Translate "Invoice total" to German.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc3pta1', index: 0, type: 'function', function: { name: 'translate_text', arguments: '{"text":"Invoice total","target_language":"de"}' } }] },
        { role: 'user', content: 'Now add 450.00 + 67.50.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'tc3-mt-prior-directions-then-reminder',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Get transit directions from "Grand Central" to "JFK Airport".' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc3pdr1', index: 0, type: 'function', function: { name: 'get_directions', arguments: '{"origin":"Grand Central","destination":"JFK Airport","mode":"transit"}' } }] },
        { role: 'user', content: 'Set a reminder at 2026-06-15T06:00:00 to leave for the airport.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // ===================================================================
    // Negative tests — tools are available but the model should NOT call any
    // ===================================================================
    {
      id: 'neg-simple-arithmetic',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's 2 + 2? Answer with only the number." },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call', value: '4' },
    },
    {
      id: 'neg-general-knowledge',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is the capital of Japan? Answer with only the city name.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call', value: '[Tt]okyo' },
    },
    {
      id: 'neg-creative-writing',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Write a haiku about snow.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'neg-explain-tool',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What does the convert_currency tool do? Explain in one sentence.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'neg-forbid-tools',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Do not call any tools. Just tell me what 15 + 28 is.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call', value: '43' },
    },
    {
      id: 'neg-no-matching-tool',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Summarize the plot of Romeo and Juliet in 2 sentences.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'neg-greeting',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Hello! How are you today?' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'neg-opinion',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What are the pros and cons of remote work? Be brief.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'neg-define-word',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Define the word "ephemeral" in one sentence.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'neg-count-letters',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'How many letters are in the word "banana"? Answer with only the number.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call', value: '6' },
    },
    // ===================================================================
    // Clarification tests — required tool inputs are missing, model should
    // ask for details rather than calling a tool with guessed/empty args
    // ===================================================================
    {
      id: 'clar-weather-no-city',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather right now?" },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'clar-stock-no-symbol',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the current stock price?" },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'clar-convert-no-source',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Convert 100 to EUR.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'clar-translate-no-target',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Translate "Good morning".' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'clar-email-no-content',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Send an email to alice@example.com.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'clar-calendar-no-datetime',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Create a calendar event titled "Team sync".' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'clar-directions-no-origin',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'How do I get to the airport?' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'clar-reminder-no-time',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Remind me to pay rent.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'clar-add-one-number',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Add 42.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'clar-search-empty',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Search for products.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    // ===================================================================
    // Ambiguity tests — multiple tools could apply, model should clarify
    // ===================================================================
    {
      id: 'ambig-apple-stock-or-product',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the price of Apple?" },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'ambig-tesla-stock-or-search',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Find Tesla for me.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'ambig-translate-and-send-no-recipient',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Translate "Hello" to French and email it.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'ambig-book-meeting-vague',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Book a meeting with Alice.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'ambig-check-price-vague',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Check the price for me.' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
    {
      id: 'ambig-send-message-no-channel',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Send a message to Bob saying "See you later".' },
      ],
      tools: autoTestTools,
      expected: { type: 'no_tool_call' },
    },
  ],
}

/**
 * Config sweep chat scenario — focuses on instruction following, simple math,
 * and format compliance rather than factual knowledge. Every prompt here should
 * be trivially correct for any model/quantization so the score reflects
 * hardware config quality (throughput, latency) not model capability.
 */
export const configChatScenario: AutoTestScenario = {
  id: 'chat',
  name: 'Chat Quality',
  prompts: [
    {
      id: 'repeat-word',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Repeat the following word exactly: elephant' },
      ],
      expected: { type: 'regex', value: '[Ee]lephant' },
    },
    {
      id: 'repeat-phrase',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Repeat this phrase exactly: the quick brown fox' },
      ],
      expected: { type: 'regex', value: '[Tt]he quick brown fox' },
    },
    {
      id: 'list-3-colors',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'List exactly 3 colors. Use a numbered list.' },
      ],
      expected: { type: 'regex', value: '1[.)]\\s+.+\\n\\s*2[.)]\\s+.+\\n\\s*3[.)]' },
    },
    {
      id: 'list-3-animals',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'List exactly 3 animals. Use a numbered list.' },
      ],
      expected: { type: 'regex', value: '1[.)]\\s+.+\\n\\s*2[.)]\\s+.+\\n\\s*3[.)]' },
    },
    {
      id: 'uppercase',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Write the word "hello" in all uppercase letters. Answer with only the word.' },
      ],
      expected: { type: 'regex', value: 'HELLO' },
    },
    {
      id: 'lowercase',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Write the word "WORLD" in all lowercase letters. Answer with only the word.' },
      ],
      expected: { type: 'regex', value: 'world' },
    },
    {
      id: 'yes-no',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Is the sky blue? Answer with only yes or no.' },
      ],
      expected: { type: 'regex', value: '[Yy]es' },
    },
    {
      id: 'count-items',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'How many items are in this list: red, blue, green? Answer with only the number.' },
      ],
      expected: { type: 'regex', value: '3' },
    },
    {
      id: 'first-letter',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is the first letter of the word "banana"? Answer with only the letter.' },
      ],
      expected: { type: 'regex', value: '[Bb]' },
    },
    {
      id: 'in-context-fact',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Remember this fact: The zorgblatt capital is Plindovia. What is the capital of zorgblatt? Answer with only the name.' },
      ],
      expected: { type: 'regex', value: '[Pp]lindovia' },
    },
    // Code generation prompts — produce longer structured output that
    // exercises throughput measurement and context window filling.
    {
      id: 'codegen-python-fn',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Write a Python function called `fizzbuzz` that takes an integer n and returns a list of strings from 1 to n where multiples of 3 are "Fizz", multiples of 5 are "Buzz", and multiples of both are "FizzBuzz".' },
      ],
      max_tokens: 512,
      expected: { type: 'regex', value: 'def\\s+fizzbuzz' },
      includeInScore: false,
    },
    {
      id: 'codegen-js-class',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Write a JavaScript class called `Stack` with push, pop, peek, and isEmpty methods. Include JSDoc comments for each method.' },
      ],
      max_tokens: 512,
      expected: { type: 'regex', value: 'class\\s+Stack' },
      includeInScore: false,
    },
    {
      id: 'codegen-go-struct',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Write a Go struct called `Config` with fields for Host (string), Port (int), and Timeout (time.Duration). Include a NewConfig constructor function and a String() method that formats it as "host:port".' },
      ],
      max_tokens: 512,
      expected: { type: 'regex', value: 'type\\s+Config\\s+struct' },
      includeInScore: false,
    },
    {
      id: 'codegen-python-sort',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Write a Python implementation of merge sort. Include a main function that sorts the list [38, 27, 43, 3, 9, 82, 10] and prints the result.' },
      ],
      max_tokens: 512,
      expected: { type: 'regex', value: 'def\\s+merge' },
      includeInScore: false,
    },
    {
      id: 'codegen-rust-enum',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Write a Rust enum called `Shape` with variants Circle(f64), Rectangle(f64, f64), and Triangle(f64, f64, f64). Implement a method `area()` that returns the area for each variant.' },
      ],
      max_tokens: 512,
      expected: { type: 'regex', value: 'enum\\s+Shape' },
      includeInScore: false,
    },
    // Multi-turn prompts that exercise IMC caching using pure instruction
    // following — no math or factual knowledge required.
    {
      id: 'multi-turn-2',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Name a fruit. Answer with only one word.' },
        { role: 'assistant', content: 'Apple' },
        { role: 'user', content: 'Now name a vegetable. Answer with only one word.' },
      ],
      expected: { type: 'regex', value: '\\w+' },
    },
    {
      id: 'multi-turn-4',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Name a fruit. Answer with only one word.' },
        { role: 'assistant', content: 'Apple' },
        { role: 'user', content: 'Now name a vegetable. Answer with only one word.' },
        { role: 'assistant', content: 'Carrot' },
        { role: 'user', content: 'Now name a color. Answer with only one word.' },
      ],
      expected: { type: 'regex', value: '\\w+' },
    },
    {
      id: 'multi-turn-6',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Name a fruit. Answer with only one word.' },
        { role: 'assistant', content: 'Apple' },
        { role: 'user', content: 'Now name a vegetable. Answer with only one word.' },
        { role: 'assistant', content: 'Carrot' },
        { role: 'user', content: 'Now name a color. Answer with only one word.' },
        { role: 'assistant', content: 'Blue' },
        { role: 'user', content: 'List the three things you named. Use a numbered list.' },
      ],
      expected: { type: 'regex', value: '1[.)]' },
    },
    {
      id: 'multi-turn-8',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Name a fruit. Answer with only one word.' },
        { role: 'assistant', content: 'Apple' },
        { role: 'user', content: 'Now name a vegetable. Answer with only one word.' },
        { role: 'assistant', content: 'Carrot' },
        { role: 'user', content: 'Now name a color. Answer with only one word.' },
        { role: 'assistant', content: 'Blue' },
        { role: 'user', content: 'List the three things you named. Use a numbered list.' },
        { role: 'assistant', content: '1. Apple\n2. Carrot\n3. Blue' },
        { role: 'user', content: 'Which of those is a fruit? Answer with only the word.' },
      ],
      expected: { type: 'regex', value: '[Aa]pple' },
    },
    {
      id: 'multi-turn-10',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Name a fruit. Answer with only one word.' },
        { role: 'assistant', content: 'Apple' },
        { role: 'user', content: 'Now name a vegetable. Answer with only one word.' },
        { role: 'assistant', content: 'Carrot' },
        { role: 'user', content: 'Now name a color. Answer with only one word.' },
        { role: 'assistant', content: 'Blue' },
        { role: 'user', content: 'List the three things you named. Use a numbered list.' },
        { role: 'assistant', content: '1. Apple\n2. Carrot\n3. Blue' },
        { role: 'user', content: 'Which of those is a fruit? Answer with only the word.' },
        { role: 'assistant', content: 'Apple' },
        { role: 'user', content: 'Write that word in all uppercase. Answer with only the word.' },
      ],
      expected: { type: 'regex', value: 'APPLE' },
    },
  ],
}

/**
 * Config sweep tool call scenario — uses a subset of tool call prompts
 * that any model should handle. The multi-turn prompts use the same
 * scripted pattern but avoid factual follow-ups.
 */
export const configToolCallScenario: AutoTestScenario = {
  id: 'tool_call',
  name: 'Tool Calling',
  prompts: [
    {
      id: 'weather-tool',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Boston?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'add-tool',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is 15 + 28?' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'weather-tokyo',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather like in Tokyo right now?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'add-large',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Calculate 1234 + 5678 for me.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'search-laptop',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Search for laptops under $1000.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'send-email',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Send an email to alice@example.com with subject "Meeting Tomorrow" and body "Let\'s meet at 3pm."' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'stock-price-aapl',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's Apple's current stock price?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'convert-usd-eur',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Convert 100 USD to EUR.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'calendar-meeting',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Schedule a team meeting for 2025-03-15 at 10:00 for 60 minutes.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'reminder-dentist',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Remind me about my dentist appointment at 2025-03-20T09:00:00.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // Multi-turn tool calls with scripted prior turns
    {
      id: 'multi-turn-tool',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Boston?" },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc1', index: 0, type: 'function', function: { name: 'get_weather', arguments: '{"location":"Boston"}' } }] },
        { role: 'user', content: "Now check the weather in London too." },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'multi-turn-tool-add',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is 10 + 20?' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'tc2', index: 0, type: 'function', function: { name: 'add', arguments: '{"a":10,"b":20}' } }] },
        { role: 'user', content: 'Now add 30 + 40.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- New single-turn prompts (simple/universal) ---
    {
      id: 'ctc2-weather-la',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Los Angeles?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'ctc2-weather-miami-fahrenheit',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Miami in fahrenheit?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'ctc2-add-zero',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is 0 + 0?' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'ctc2-add-negative',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Compute -3 + 11.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'ctc2-search-coffee',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Search for coffee.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'ctc2-search-pen-max2',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Search for pen and limit to 2 results.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'ctc2-stock-msft',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Get stock price for MSFT.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'ctc2-convert-1-usd-eur',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Convert 1 USD to EUR.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'ctc2-translate-hello-fr',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Translate "hello" to French.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- New multi-turn prompts (scripted, still easy) ---
    {
      id: 'ctc2-mt-weather-two-cities',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Boston?" },
        { role: 'assistant', content: '', tool_calls: [{ id: 'ctc2w1', index: 0, type: 'function', function: { name: 'get_weather', arguments: '{"location":"Boston"}' } }] },
        { role: 'user', content: 'Now check the weather in Chicago.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'ctc2-mt-add-followup',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'What is 1 + 2?' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'ctc2a1', index: 0, type: 'function', function: { name: 'add', arguments: '{"a":1,"b":2}' } }] },
        { role: 'user', content: 'Now add 3 + 4.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'ctc2-mt-translate-email-reminder',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Translate "hello" to Spanish.' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'ctc2ter1', index: 0, type: 'function', function: { name: 'translate_text', arguments: '{"text":"hello","target_language":"es"}' } }] },
        { role: 'user', content: 'Send that to test@example.com with subject "Greeting" and body "hola".' },
        { role: 'assistant', content: '', tool_calls: [{ id: 'ctc2ter2', index: 0, type: 'function', function: { name: 'send_email', arguments: '{"to":"test@example.com","subject":"Greeting","body":"hola"}' } }] },
        { role: 'user', content: 'Set a reminder at 2025-01-01T00:00:00 to follow up.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- Parallel tool calls (simple/deterministic) ---
    {
      id: 'ctc3-parallel-weather-2',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in Boston and Tokyo?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'ctc3-parallel-add-stock',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "Add 5 + 10, and get the stock price for AAPL." },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- Edge cases (simple/universal) ---
    {
      id: 'ctc3-weather-unicode',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in São Paulo?" },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    {
      id: 'ctc3-convert-large',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: 'Convert 1000000 USD to JPY.' },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
    // --- Self-correction (simple) ---
    {
      id: 'ctc3-correction-city',
      messages: [
        { role: 'system', content: cacheSystemPrompt },
        { role: 'user', content: "What's the weather in London—actually make that Dublin." },
      ],
      tools: autoTestTools,
      expected: { type: 'tool_call' },
    },
  ],
}

/** Generates trial candidates with expanded parameter grids, truncated to maxTrials. */
export function generateTrialCandidates(
  sweepDef: SamplingSweepDefinition,
  maxTrials: number = Infinity,
): SamplingCandidate[] {
  const safeMax = maxTrials === Infinity
    ? Infinity
    : Number.isFinite(maxTrials) ? Math.max(1, Math.floor(maxTrials)) : 25

  // Baseline uses first value of each param's range
  const base: SamplingCandidate = {
    temperature: sweepDef.temperature[0] ?? 0.8,
    top_p: sweepDef.top_p[0] ?? 0.9,
    top_k: sweepDef.top_k[0] ?? 40,
    min_p: sweepDef.min_p[0] ?? 0,
    repeat_penalty: sweepDef.repeat_penalty[0] ?? 1.0,
    repeat_last_n: sweepDef.repeat_last_n[0] ?? 64,
    frequency_penalty: sweepDef.frequency_penalty[0] ?? 0.0,
    presence_penalty: sweepDef.presence_penalty[0] ?? 0.0,
    dry_multiplier: sweepDef.dry_multiplier[0] ?? 1.05,
    dry_base: sweepDef.dry_base[0] ?? 1.75,
    dry_allowed_length: sweepDef.dry_allowed_length[0] ?? 2,
    dry_penalty_last_n: sweepDef.dry_penalty_last_n[0] ?? 0,
    xtc_probability: sweepDef.xtc_probability[0] ?? 0.0,
    xtc_threshold: sweepDef.xtc_threshold[0] ?? 0.1,
    xtc_min_keep: sweepDef.xtc_min_keep[0] ?? 1,
    max_tokens: sweepDef.max_tokens[0] ?? 4096,
    enable_thinking: (sweepDef.enable_thinking[0] ?? 'true') as 'true' | 'false',
    reasoning_effort: (sweepDef.reasoning_effort[0] ?? 'medium') as SamplingCandidate['reasoning_effort'],
  }

  // Normalize floats to 3 decimal places for stable comparison and dedup
  const norm = (n: number | undefined) =>
    n !== undefined && Number.isFinite(n) ? Math.round(n * 1000) / 1000 : n

  const seen = new Set<string>()
  const candidates: SamplingCandidate[] = []

  const keyOf = (c: SamplingCandidate) =>
    `t=${norm(c.temperature)}|p=${norm(c.top_p)}|k=${c.top_k}|m=${norm(c.min_p)}|rp=${norm(c.repeat_penalty)}|rn=${c.repeat_last_n}|fp=${norm(c.frequency_penalty)}|pp=${norm(c.presence_penalty)}|dm=${norm(c.dry_multiplier)}|db=${norm(c.dry_base)}|da=${c.dry_allowed_length}|dp=${c.dry_penalty_last_n}|xp=${norm(c.xtc_probability)}|xt=${norm(c.xtc_threshold)}|xk=${c.xtc_min_keep}|mt=${c.max_tokens}|et=${c.enable_thinking}|re=${c.reasoning_effort}`

  const add = (c: SamplingCandidate) => {
    if (safeMax !== Infinity && candidates.length >= safeMax) return
    const k = keyOf(c)
    if (seen.has(k)) return
    seen.add(k)
    candidates.push(c)
  }

  const approxEq = (a: number | undefined, b: number) =>
    a !== undefined && Math.abs(a - b) < 0.001

  // 1) Baseline always first
  add({ ...base })
  if (safeMax <= 1) return candidates

  // Sort each grid by distance from baseline value (closest first for early coverage)
  const sortByDistance = (vals: number[], center: number) =>
    [...vals].sort((a, b) => Math.abs(a - center) - Math.abs(b - center))

  // Build OAT lists from sweep definition ranges
  type NumericKey = 'temperature' | 'top_p' | 'top_k' | 'min_p' | 'repeat_penalty' | 'repeat_last_n' |
    'frequency_penalty' | 'presence_penalty' | 'dry_multiplier' | 'dry_base' | 'dry_allowed_length' |
    'dry_penalty_last_n' | 'xtc_probability' | 'xtc_threshold' | 'xtc_min_keep' | 'max_tokens'

  const numericAxes: Array<{ key: NumericKey; values: number[] }> = [
    { key: 'temperature', values: sweepDef.temperature },
    { key: 'top_p', values: sweepDef.top_p },
    { key: 'top_k', values: sweepDef.top_k },
    { key: 'min_p', values: sweepDef.min_p },
    { key: 'repeat_penalty', values: sweepDef.repeat_penalty },
    { key: 'repeat_last_n', values: sweepDef.repeat_last_n },
    { key: 'frequency_penalty', values: sweepDef.frequency_penalty },
    { key: 'presence_penalty', values: sweepDef.presence_penalty },
    { key: 'dry_multiplier', values: sweepDef.dry_multiplier },
    { key: 'dry_base', values: sweepDef.dry_base },
    { key: 'dry_allowed_length', values: sweepDef.dry_allowed_length },
    { key: 'dry_penalty_last_n', values: sweepDef.dry_penalty_last_n },
    { key: 'xtc_probability', values: sweepDef.xtc_probability },
    { key: 'xtc_threshold', values: sweepDef.xtc_threshold },
    { key: 'xtc_min_keep', values: sweepDef.xtc_min_keep },
    { key: 'max_tokens', values: sweepDef.max_tokens },
  ]

  const oatLists: SamplingCandidate[][] = numericAxes
    .filter(axis => axis.values.length > 1)
    .map(axis => {
      const baseVal = (base[axis.key] as number) ?? 0
      return sortByDistance(
        axis.values.filter(v => !approxEq(baseVal, v)),
        baseVal,
      ).map(v => ({ ...base, [axis.key]: v }))
    })

  // String param axes (enable_thinking, reasoning_effort)
  if (sweepDef.enable_thinking.length > 1) {
    oatLists.push(
      sweepDef.enable_thinking
        .filter(v => v !== base.enable_thinking)
        .map(v => ({ ...base, enable_thinking: v as 'true' | 'false' })),
    )
  }
  if (sweepDef.reasoning_effort.length > 1) {
    oatLists.push(
      sweepDef.reasoning_effort
        .filter(v => v !== base.reasoning_effort)
        .map(v => ({ ...base, reasoning_effort: v as SamplingCandidate['reasoning_effort'] })),
    )
  }

  // 2) Round-robin OAT interleave across parameters
  for (let i = 0; safeMax === Infinity || candidates.length < safeMax; i++) {
    let addedAny = false
    for (const list of oatLists) {
      if (i < list.length) {
        add(list[i])
        addedAny = true
        if (candidates.length >= safeMax) break
      }
    }
    if (!addedAny) break
  }

  // 3) Pairwise corner combos for the 4 primary params
  const tVals = sweepDef.temperature.length > 0 ? sweepDef.temperature : [base.temperature ?? 0.8]
  const pVals = sweepDef.top_p.length > 0 ? sweepDef.top_p : [base.top_p ?? 0.9]
  const kVals = sweepDef.top_k.length > 0 ? sweepDef.top_k : [base.top_k ?? 40]
  const mVals = sweepDef.min_p.length > 0 ? sweepDef.min_p : [base.min_p ?? 0]

  if (tVals.length > 1 || pVals.length > 1 || kVals.length > 1 || mVals.length > 1) {
    const tLow = Math.min(...tVals)
    const tHigh = Math.max(...tVals)
    const pLow = Math.min(...pVals)
    const pHigh = Math.max(...pVals)
    const kLow = Math.min(...kVals)
    const kHigh = Math.max(...kVals)
    const mHigh = Math.max(...mVals)

    const corners: SamplingCandidate[] = [
      { ...base, temperature: tLow, top_p: pLow },
      { ...base, temperature: tLow, top_p: pHigh },
      { ...base, temperature: tHigh, top_p: pLow },
      { ...base, temperature: tHigh, top_p: pHigh },
      { ...base, temperature: tLow, top_k: kLow },
      { ...base, temperature: tLow, top_k: kHigh },
      { ...base, temperature: tHigh, top_k: kLow },
      { ...base, temperature: tHigh, top_k: kHigh },
      { ...base, top_p: pLow, top_k: kLow },
      { ...base, top_p: pLow, top_k: kHigh },
      { ...base, top_p: pHigh, top_k: kLow },
      { ...base, top_p: pHigh, top_k: kHigh },
      { ...base, min_p: mHigh, temperature: tLow },
      { ...base, min_p: mHigh, temperature: tHigh },
      { ...base, min_p: mHigh, top_p: pLow },
      { ...base, min_p: mHigh, top_p: pHigh },
    ]
    corners.forEach(add)
  }

  return candidates
}

/** Default sampling sweep ranges for each parameter. */
export const defaultSamplingSweepDef: SamplingSweepDefinition = {
  temperature: [0.8],
  top_p: [0.9],
  top_k: [40],
  min_p: [0],
  repeat_penalty: [1.0],
  repeat_last_n: [64],
  frequency_penalty: [0.0],
  presence_penalty: [0.0],
  dry_multiplier: [1.05],
  dry_base: [1.75],
  dry_allowed_length: [2],
  dry_penalty_last_n: [0],
  xtc_probability: [0.0],
  xtc_threshold: [0.1],
  xtc_min_keep: [1],
  max_tokens: [4096],
  enable_thinking: ['true'],
  reasoning_effort: ['medium'],
}

/** Default config sweep grids for each parameter. */
export const defaultConfigSweepDef: ConfigSweepDefinition = {
  nbatch: { enabled: true, values: [512, 1024, 2048, 4096] },
  nubatch: { enabled: true, values: [128, 256, 512, 1024, 2048] },
  contextWindow: { enabled: true, values: [2048, 4096, 8192, 16384, 32768] },
  nSeqMax: { enabled: true, values: [1, 2, 4, 8] },
  flashAttention: { enabled: true, values: ['auto', 'enabled', 'disabled'] },
  cacheType: { enabled: true, values: ['f16', 'q8_0', 'q4_0'] },
  cacheMode: { enabled: true, values: ['none', 'spc', 'imc'] },
}

/** Generates config candidates as a full cross-product of all enabled parameter values. */
export function generateConfigCandidates(
  baseConfig: PlaygroundModelConfig,
  def: ConfigSweepDefinition,
): ConfigCandidate[] {
  const baseline: ConfigCandidate = {
    'context_window': baseConfig['context_window'],
    nbatch: baseConfig.nbatch,
    nubatch: baseConfig.nubatch,
    'nseq_max': baseConfig['nseq_max'],
  }

  const paramAxes: Array<{ configKey: keyof ConfigCandidate; values: number[] }> = []

  if (def.nbatch.enabled && def.nbatch.values.length > 0) {
    paramAxes.push({ configKey: 'nbatch', values: def.nbatch.values })
  }
  if (def.nubatch.enabled && def.nubatch.values.length > 0) {
    paramAxes.push({ configKey: 'nubatch', values: def.nubatch.values })
  }
  if (def.contextWindow.enabled && def.contextWindow.values.length > 0) {
    paramAxes.push({ configKey: 'context_window', values: def.contextWindow.values })
  }
  if (def.nSeqMax.enabled && def.nSeqMax.values.length > 0) {
    paramAxes.push({ configKey: 'nseq_max', values: def.nSeqMax.values })
  }

  // String/boolean axes for mixed-type cross-product
  type AnyAxis = { configKey: string; values: (number | string | boolean)[] };
  const allAxes: AnyAxis[] = [...paramAxes];

  if (def.flashAttention.enabled && def.flashAttention.values.length > 0) {
    allAxes.push({ configKey: 'flash_attention', values: def.flashAttention.values });
  }
  if (def.cacheType.enabled && def.cacheType.values.length > 0) {
    allAxes.push({ configKey: 'cache_type', values: def.cacheType.values });
  }
  if (def.cacheMode.enabled && def.cacheMode.values.length > 0) {
    allAxes.push({ configKey: 'cache_mode', values: def.cacheMode.values });
  }

  if (allAxes.length === 0) {
    return [{ ...baseline }]
  }

  // Build the full cross-product of all enabled parameter values.
  let combos: ConfigCandidate[] = [{ ...baseline }]

  for (const axis of allAxes) {
    const next: ConfigCandidate[] = []
    for (const combo of combos) {
      for (const v of axis.values) {
        next.push({ ...combo, [axis.configKey]: v })
      }
    }
    combos = next
  }

  // Dedup (in case values arrays contain duplicates).
  const seen = new Set<string>()
  const candidates: ConfigCandidate[] = []

  const keyOf = (c: ConfigCandidate) =>
    `cw=${c['context_window']}|nb=${c.nbatch}|nub=${c.nubatch}|ns=${c['nseq_max']}|fa=${c['flash_attention']}|ct=${c['cache_type']}|cm=${c['cache_mode']}`

  for (const c of combos) {
    const k = keyOf(c)
    if (seen.has(k)) continue
    // Skip invalid: effective nubatch must not exceed effective nbatch.
    const effNBatch = c.nbatch ?? baseConfig.nbatch
    const effNUBatch = c.nubatch ?? baseConfig.nubatch
    if (effNBatch !== undefined && effNUBatch !== undefined &&
        Number.isFinite(effNBatch) && Number.isFinite(effNUBatch) &&
        effNUBatch > effNBatch) continue
    seen.add(k)
    candidates.push(c)
  }

  return candidates
}

/** Scores chat text output against an expected value. */
export function scoreChat(
  text: string,
  expected: AutoTestPromptDef['expected'],
): { score: number; notes: string[] } {
  const notes: string[] = []
  let score = 0

  if (!expected) {
    return { score: 0, notes: ['No expected value defined'] }
  }

  switch (expected.type) {
    case 'exact': {
      const trimmed = text.trim().toLowerCase()
      const target = (expected.value ?? '').trim().toLowerCase()
      if (trimmed === target) {
        score = 100
      } else if (trimmed.includes(target)) {
        score = 50
        notes.push(`Partial match: response contains "${expected.value}" but has extra text`)
      } else {
        score = 0
        notes.push(`Expected "${expected.value}", got "${text.trim().slice(0, 100)}"`)
      }
      break
    }
    case 'regex': {
      const re = new RegExp(expected.value ?? '', 'im')
      if (re.test(text)) {
        score = 100
      } else {
        score = 0
        notes.push(`Regex /${expected.value}/ did not match`)
      }
      break
    }
    case 'tool_call':
    case 'no_tool_call': {
      score = 0
      notes.push(`${expected.type} expected type not applicable for chat scoring`)
      break
    }
  }

  if (text.length > 2000) {
    score = Math.max(0, score - 10)
    notes.push('Penalized: response exceeds 2000 characters')
  }

  return { score, notes }
}

/** Scores tool call output against declared tools. */
export function scoreToolCall(
  toolCalls: ChatToolCall[],
  tools: ChatToolDefinition[],
): { score: number; notes: string[] } {
  const notes: string[] = []

  if (!toolCalls || toolCalls.length === 0) {
    return { score: 0, notes: ['No tool calls emitted'] }
  }

  let score = 100

  const declaredNames = new Set(
    tools
      .filter((t) => t.type === 'function' && t.function?.name)
      .map((t) => t.function.name),
  )

  for (const tc of toolCalls) {
    if (!declaredNames.has(tc.function.name)) {
      score -= 40
      notes.push(`Tool "${tc.function.name}" not in declared tools`)
    }

    try {
      const args = JSON.parse(tc.function.arguments)
      const toolDef = tools.find(
        (t) => t.type === 'function' && t.function?.name === tc.function.name,
      )
      const params = toolDef?.function?.parameters as Record<string, unknown> | undefined
      if (params?.required && Array.isArray(params.required)) {
        const required: string[] = params.required
        for (const field of required) {
          if (!(field in args)) {
            score -= 20
            notes.push(`Missing required field "${field}" in ${tc.function.name}`)
          }
        }
      }
    } catch {
      score -= 30
      notes.push(`Arguments for "${tc.function.name}" are not valid JSON`)
    }
  }

  return { score: Math.max(0, Math.min(100, score)), notes }
}

/** Scores a prompt where no tool call should have been emitted.
 *  If an optional regex `value` is provided, the assistant text must also match. */
export function scoreNoToolCall(
  toolCalls: ChatToolCall[],
  text: string,
  expected: AutoTestPromptDef['expected'],
): { score: number; notes: string[] } {
  const notes: string[] = []

  if (toolCalls && toolCalls.length > 0) {
    const names = toolCalls.map(tc => tc.function?.name).filter(Boolean).join(', ')
    notes.push(`Unexpected tool call(s): ${names}`)
    return { score: 0, notes }
  }

  let score = 100

  if (text.trim().length === 0) {
    score -= 20
    notes.push('No text response produced')
  }

  if (expected?.value) {
    const re = new RegExp(expected.value, 'im')
    if (!re.test(text)) {
      score -= 30
      notes.push(`Expected text matching /${expected.value}/ but got "${text.trim().slice(0, 100)}"`)
    }
  }

  return { score: Math.max(0, score), notes }
}

/** Runs a single prompt against the playground session and scores the result. */
export function runSinglePrompt(
  sessionId: string,
  prompt: AutoTestPromptDef,
  candidate: SamplingCandidate,
  signal?: AbortSignal,
): Promise<AutoTestPromptResult> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      return reject(new DOMException('Aborted', 'AbortError'))
    }

    let settled = false
    let fullContent = ''
    const collectedToolCalls: ChatToolCall[] = []
    let usage: ChatUsage | undefined

    const onAbort = () => {
      if (settled) return
      settled = true
      abortStream()
      reject(new DOMException('Aborted', 'AbortError'))
    }

    const abortStream = api.streamPlaygroundChat(
      {
        session_id: sessionId,
        messages: prompt.messages,
        tools: prompt.tools,
        stream: true,
        temperature: candidate.temperature,
        top_p: candidate.top_p,
        top_k: candidate.top_k,
        min_p: candidate.min_p,
        max_tokens: candidate.max_tokens ?? prompt.max_tokens,
        repeat_penalty: candidate.repeat_penalty,
        repeat_last_n: candidate.repeat_last_n,
        frequency_penalty: candidate.frequency_penalty,
        presence_penalty: candidate.presence_penalty,
        dry_multiplier: candidate.dry_multiplier,
        dry_base: candidate.dry_base,
        dry_allowed_length: candidate.dry_allowed_length,
        dry_penalty_last_n: candidate.dry_penalty_last_n,
        xtc_probability: candidate.xtc_probability,
        xtc_threshold: candidate.xtc_threshold,
        xtc_min_keep: candidate.xtc_min_keep,
        adaptive_p_target: candidate.adaptive_p_target,
        adaptive_p_decay: candidate.adaptive_p_decay,
        enable_thinking: candidate.enable_thinking,
        reasoning_effort: candidate.reasoning_effort,
        grammar: candidate.grammar,
        stream_options: { include_usage: true },
      },
      (data: ChatStreamResponse) => {
        const choice = data.choices?.[0]
        if (choice?.delta?.content) {
          fullContent += choice.delta.content
        }
        if (choice?.delta?.tool_calls) {
          for (const tc of choice.delta.tool_calls) {
            const existing = collectedToolCalls.find(c => c.index === tc.index)
            if (existing) {
              if (tc.function?.arguments) {
                existing.function.arguments += tc.function.arguments
              }
            } else {
              collectedToolCalls.push({
                id: tc.id || '',
                index: tc.index,
                type: tc.type || 'function',
                function: {
                  name: tc.function?.name || '',
                  arguments: tc.function?.arguments || '',
                },
              })
            }
          }
        }
        if (data.usage) {
          usage = data.usage
        }
      },
      (error: string) => {
        if (settled) return
        settled = true
        signal?.removeEventListener('abort', onAbort)
        reject(new Error(error))
      },
      () => {
        if (settled) return
        settled = true
        signal?.removeEventListener('abort', onAbort)

        let scored: { score: number; notes: string[] }

        if (prompt.expected?.type === 'tool_call') {
          scored = scoreToolCall(collectedToolCalls, prompt.tools ?? [])
        } else if (prompt.expected?.type === 'no_tool_call') {
          scored = scoreNoToolCall(collectedToolCalls, fullContent, prompt.expected)
        } else {
          scored = scoreChat(fullContent, prompt.expected)
        }

        resolve({
          promptId: prompt.id,
          assistantText: fullContent,
          toolCalls: collectedToolCalls,
          usage,
          score: scored.score,
          notes: scored.notes.length > 0 ? scored.notes : undefined,
        })
      },
    )

    signal?.addEventListener('abort', onAbort, { once: true })
  })
}

/** Probes whether the current session/template supports tool calling. */
export async function probeTemplate(sessionId: string, signal?: AbortSignal): Promise<boolean> {
  const prompt: AutoTestPromptDef = {
    id: 'probe-tool',
    messages: [
      { role: 'system', content: cacheSystemPrompt },
      { role: 'user', content: "What's the weather in Boston?" },
    ],
    tools: autoTestTools,
    expected: { type: 'tool_call' },
  }

  try {
    const result = await runSinglePrompt(sessionId, prompt, {}, signal)
    return result.toolCalls.length > 0 && result.score > 0
  } catch {
    return false
  }
}

export const defaultBestConfigWeights: BestConfigWeights = {
  chatScore: 0.4,
  toolScore: 0.6,
  totalScore: 0,
  avgTPS: 0,
  avgTTFT: 0,
}

/**
 * Compute a composite score for a trial using the provided weights.
 * Only metrics that are actually present on the trial contribute; missing
 * metrics (e.g. disabled scenario, undefined TPS/TTFT) are skipped so they
 * don't drag the score down.
 * For avgTTFT lower is better so we invert it: score = max(0, 100 - ttft_ms/10).
 */
export function computeCompositeScore(trial: AutoTestTrialResult, weights: BestConfigWeights): number {
  const chat = trial.scenarioResults.find(r => r.scenarioId === 'chat')?.score
  const tool = trial.scenarioResults.find(r => r.scenarioId === 'tool_call')?.score
  const total = trial.totalScore
  const avgTPS = trial.avgTPS
  const avgTTFT = trial.avgTTFT

  const parts: Array<{ w: number; v: number }> = []

  if (chat !== undefined) parts.push({ w: weights.chatScore, v: chat })
  if (tool !== undefined) parts.push({ w: weights.toolScore, v: tool })
  if (total !== undefined) parts.push({ w: weights.totalScore, v: total })
  if (avgTPS !== undefined) parts.push({ w: weights.avgTPS, v: Math.min(100, Math.max(0, avgTPS)) })
  if (avgTTFT !== undefined) parts.push({ w: weights.avgTTFT, v: Math.max(0, 100 - avgTTFT / 10) })

  const denom = parts.reduce((s, p) => s + p.w, 0)
  if (denom <= 0) return total ?? chat ?? tool ?? 0

  return Math.round(parts.reduce((s, p) => s + p.w * p.v, 0) / denom * 100) / 100
}

/** Options for controlling concurrent prompt execution within a trial. */
export interface RunTrialOptions {
  concurrency?: number;
}

const maxConcurrencyCap = 6

/** Runs up to `limit` async tasks concurrently, preserving result order. */
async function mapLimit<T, R>(
  items: readonly T[],
  limit: number,
  worker: (item: T, index: number) => Promise<R>,
  signal: AbortSignal,
): Promise<R[]> {
  const results = new Array<R>(items.length)
  let next = 0

  const runners = Array.from({ length: Math.min(limit, items.length) }, async () => {
    while (!signal.aborted) {
      const i = next++
      if (i >= items.length) break
      results[i] = await worker(items[i], i)
    }
  })

  await Promise.all(runners)
  return results
}

/** Builds a scenario result snapshot from accumulated prompt results and metrics. */
function buildScenarioResult(
  scenarioId: AutoTestScenarioID,
  prompts: AutoTestPromptDef[],
  promptResults: AutoTestPromptResult[],
  totalGenTokens: number,
  totalGenSeconds: number,
  totalTTFT: number,
  ttftCount: number,
  fillGenTokens: Partial<Record<ContextFillRatio, number>>,
  fillGenSeconds: Partial<Record<ContextFillRatio, number>>,
  fillTTFT: Partial<Record<ContextFillRatio, number>>,
  fillTTFTCount: Partial<Record<ContextFillRatio, number>>,
  actualFills: Partial<Record<ContextFillRatio, number>>,
): AutoTestScenarioResult {
  const excludeIds = new Set(prompts.filter(p => p.includeInScore === false).map(p => p.id))
  const scorablePrompts = promptResults.filter(r => !excludeIds.has(r.promptId))
  const scenarioScore = scorablePrompts.length > 0
    ? Math.round(scorablePrompts.reduce((sum, r) => sum + r.score, 0) / scorablePrompts.length * 100) / 100
    : 0

  const avgTPSByFill: Record<string, number> = {}
  const avgTTFTByFill: Record<string, number> = {}
  const promptTokensByFill: Record<string, number> = {}
  for (const label of ['20%', '50%', '80%'] as ContextFillRatio[]) {
    const gt = fillGenTokens[label]
    const gs = fillGenSeconds[label]
    if (gt && gs && gs > 0) {
      avgTPSByFill[label] = gt / gs
    }
    const ft = fillTTFT[label]
    const fc = fillTTFTCount[label]
    if (ft && fc && fc > 0) {
      avgTTFTByFill[label] = ft / fc
    }
    if (actualFills[label]) {
      promptTokensByFill[label] = actualFills[label]!
    }
  }

  return {
    scenarioId,
    promptResults: [...promptResults],
    score: scenarioScore,
    avgTPS: totalGenSeconds > 0 ? totalGenTokens / totalGenSeconds : undefined,
    avgTTFT: ttftCount > 0 ? totalTTFT / ttftCount : undefined,
    ...(Object.keys(avgTPSByFill).length > 0 && { avgTPSByFill: avgTPSByFill as Record<ContextFillRatio, number> }),
    ...(Object.keys(avgTTFTByFill).length > 0 && { avgTTFTByFill: avgTTFTByFill as Record<ContextFillRatio, number> }),
    ...(Object.keys(promptTokensByFill).length > 0 && { promptTokensByFill: promptTokensByFill as Record<ContextFillRatio, number> }),
  }
}

/** Updates (or appends) a scenario result on the trial and fires onUpdate. */
function upsertScenarioResult(
  result: AutoTestTrialResult,
  scenarioResult: AutoTestScenarioResult,
  onUpdate: (result: AutoTestTrialResult) => void,
) {
  const existingIdx = result.scenarioResults.findIndex(s => s.scenarioId === scenarioResult.scenarioId)
  if (existingIdx >= 0) {
    result.scenarioResults[existingIdx] = scenarioResult
  } else {
    result.scenarioResults.push(scenarioResult)
  }
  onUpdate({ ...result, scenarioResults: [...result.scenarioResults] })
}

/** Accumulates TPS/TTFT metrics from a prompt result.
 *  When includeInOverall is true, the metrics contribute to headline TPS/TTFT.
 *  Context-fill metrics are always tracked in their per-fill buckets. */
function accumulateMetrics(
  pr: AutoTestPromptResult,
  prompt: AutoTestPromptDef,
  acc: {
    genTokens: number; genSeconds: number; ttft: number; ttftCount: number;
    fillGenTokens: Partial<Record<ContextFillRatio, number>>;
    fillGenSeconds: Partial<Record<ContextFillRatio, number>>;
    fillTTFT: Partial<Record<ContextFillRatio, number>>;
    fillTTFTCount: Partial<Record<ContextFillRatio, number>>;
    actualFills: Partial<Record<ContextFillRatio, number>>;
  },
  includeInOverall: boolean,
) {
  const tps = pr.usage?.tokens_per_second
  const out = pr.usage?.output_tokens

  if (includeInOverall && tps && tps > 0 && out !== undefined) {
    const gen = Math.max(0, out - 1)
    if (gen > 0) {
      acc.genTokens += gen
      acc.genSeconds += gen / tps
    }
  }
  if (includeInOverall && pr.usage?.time_to_first_token_ms && pr.usage.time_to_first_token_ms > 0) {
    acc.ttft += pr.usage.time_to_first_token_ms
    acc.ttftCount++
  }

  if (prompt.contextFill) {
    const fillLabel = prompt.contextFill.label
    if (tps && tps > 0 && out !== undefined) {
      const gen = Math.max(0, out - 1)
      if (gen > 0) {
        acc.fillGenTokens[fillLabel] = (acc.fillGenTokens[fillLabel] ?? 0) + gen
        acc.fillGenSeconds[fillLabel] = (acc.fillGenSeconds[fillLabel] ?? 0) + gen / tps
      }
    }
    if (pr.usage?.time_to_first_token_ms && pr.usage.time_to_first_token_ms > 0) {
      acc.fillTTFT[fillLabel] = (acc.fillTTFT[fillLabel] ?? 0) + pr.usage.time_to_first_token_ms
      acc.fillTTFTCount[fillLabel] = (acc.fillTTFTCount[fillLabel] ?? 0) + 1
    }
    if (pr.usage?.prompt_tokens) {
      acc.actualFills[fillLabel] = pr.usage.prompt_tokens
    }
  }
}

/** Runs a scenario's prompts sequentially (original behavior for concurrency=1). */
async function runScenarioSequential(
  sessionId: string,
  candidate: SamplingCandidate,
  scenario: AutoTestScenario,
  safeRepeats: number,
  result: AutoTestTrialResult,
  onUpdate: (result: AutoTestTrialResult) => void,
  signal: AbortSignal,
) {
  const promptResults: AutoTestPromptResult[] = []
  let totalGenTokens = 0
  let totalGenSeconds = 0
  let totalTTFT = 0
  let ttftCount = 0

  const fillGenTokens: Partial<Record<ContextFillRatio, number>> = {}
  const fillGenSeconds: Partial<Record<ContextFillRatio, number>> = {}
  const fillTTFT: Partial<Record<ContextFillRatio, number>> = {}
  const fillTTFTCount: Partial<Record<ContextFillRatio, number>> = {}
  const actualFills: Partial<Record<ContextFillRatio, number>> = {}

  for (let pi = 0; pi < scenario.prompts.length; pi++) {
    const prompt = scenario.prompts[pi]
    if (signal.aborted) {
      result.status = 'cancelled'
      onUpdate({ ...result })
      return
    }

    const isWarmup = pi === 0

    const repeatScores: number[] = []
    const repeatNotes: string[] = []
    let bestAssistantText = ''
    let bestToolCalls: ChatToolCall[] = []
    let repeatGenTokens = 0
    let repeatGenSeconds = 0
    let repeatTTFT = 0
    let repeatTTFTCount = 0
    let lastUsage: ChatUsage | undefined
    let hadAbort = false
    let hadError = false
    let errorMessage = ''

    for (let r = 0; r < safeRepeats; r++) {
      if (signal.aborted) {
        hadAbort = true
        break
      }

      try {
        const pr = await runSinglePrompt(sessionId, prompt, candidate, signal)
        repeatScores.push(pr.score)
        if (pr.notes) repeatNotes.push(...pr.notes)
        if (r === 0) {
          bestAssistantText = pr.assistantText
          bestToolCalls = pr.toolCalls
        }
        lastUsage = pr.usage

        if (!isWarmup) {
          const tps = pr.usage?.tokens_per_second
          const out = pr.usage?.output_tokens
          if (tps && tps > 0 && out !== undefined) {
            const gen = Math.max(0, out - 1)
            if (gen > 0) {
              repeatGenTokens += gen
              repeatGenSeconds += gen / tps
            }
          }
          if (pr.usage?.time_to_first_token_ms && pr.usage.time_to_first_token_ms > 0) {
            repeatTTFT += pr.usage.time_to_first_token_ms
            repeatTTFTCount++
          }
        }

        if (prompt.contextFill) {
          const fillLabel = prompt.contextFill.label
          const tps = pr.usage?.tokens_per_second
          const out = pr.usage?.output_tokens
          if (tps && tps > 0 && out !== undefined) {
            const gen = Math.max(0, out - 1)
            if (gen > 0) {
              fillGenTokens[fillLabel] = (fillGenTokens[fillLabel] ?? 0) + gen
              fillGenSeconds[fillLabel] = (fillGenSeconds[fillLabel] ?? 0) + gen / tps
            }
          }
          if (pr.usage?.time_to_first_token_ms && pr.usage.time_to_first_token_ms > 0) {
            fillTTFT[fillLabel] = (fillTTFT[fillLabel] ?? 0) + pr.usage.time_to_first_token_ms
            fillTTFTCount[fillLabel] = (fillTTFTCount[fillLabel] ?? 0) + 1
          }
          if (pr.usage?.prompt_tokens) {
            actualFills[fillLabel] = pr.usage.prompt_tokens
          }
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === 'AbortError') {
          hadAbort = true
          break
        }
        repeatScores.push(0)
        errorMessage = `Error: ${err instanceof Error ? err.message : String(err)}`
        hadError = true
      }
    }

    if (hadAbort) {
      result.status = 'cancelled'
      onUpdate({ ...result })
      return
    }

    const avgScore = repeatScores.length > 0
      ? Math.round(repeatScores.reduce((s, v) => s + v, 0) / repeatScores.length * 100) / 100
      : 0

    const dedupNotes = repeatNotes.length > 0 ? [...new Set(repeatNotes)] : undefined

    promptResults.push({
      promptId: prompt.id,
      assistantText: bestAssistantText,
      toolCalls: bestToolCalls,
      usage: lastUsage,
      score: avgScore,
      notes: hadError && !dedupNotes ? [errorMessage] : dedupNotes,
    })

    const isFill = !!prompt.contextFill
    if (!isWarmup && !isFill) {
      totalGenTokens += repeatGenTokens
      totalGenSeconds += repeatGenSeconds
      totalTTFT += repeatTTFT
      ttftCount += repeatTTFTCount
    }

    const scenarioResult = buildScenarioResult(
      scenario.id, scenario.prompts, promptResults,
      totalGenTokens, totalGenSeconds, totalTTFT, ttftCount,
      fillGenTokens, fillGenSeconds, fillTTFT, fillTTFTCount, actualFills,
    )
    upsertScenarioResult(result, scenarioResult, onUpdate)
  }
}

/** Runs a scenario's prompts concurrently to exercise multi-slot (NSeqMax > 1) configs.
 *  Uses wall-clock elapsed time to compute effective batch TPS, which reflects
 *  the true concurrent throughput advantage of higher NSeqMax values. */
async function runScenarioConcurrent(
  sessionId: string,
  candidate: SamplingCandidate,
  scenario: AutoTestScenario,
  safeRepeats: number,
  concurrency: number,
  result: AutoTestTrialResult,
  onUpdate: (result: AutoTestTrialResult) => void,
  signal: AbortSignal,
) {
  const promptResults: AutoTestPromptResult[] = new Array(scenario.prompts.length)
  const acc = {
    genTokens: 0, genSeconds: 0, ttft: 0, ttftCount: 0,
    fillGenTokens: {} as Partial<Record<ContextFillRatio, number>>,
    fillGenSeconds: {} as Partial<Record<ContextFillRatio, number>>,
    fillTTFT: {} as Partial<Record<ContextFillRatio, number>>,
    fillTTFTCount: {} as Partial<Record<ContextFillRatio, number>>,
    actualFills: {} as Partial<Record<ContextFillRatio, number>>,
  }

  const batchStartMs = performance.now()

  await mapLimit(
    scenario.prompts,
    concurrency,
    async (prompt, pi) => {
      if (signal.aborted) return

      const repeatScores: number[] = []
      const repeatNotes: string[] = []
      let bestAssistantText = ''
      let bestToolCalls: ChatToolCall[] = []
      let lastUsage: ChatUsage | undefined
      let hadError = false
      let errorMessage = ''

      for (let r = 0; r < safeRepeats; r++) {
        if (signal.aborted) break

        try {
          const pr = await runSinglePrompt(sessionId, prompt, candidate, signal)
          repeatScores.push(pr.score)
          if (pr.notes) repeatNotes.push(...pr.notes)
          if (r === 0) {
            bestAssistantText = pr.assistantText
            bestToolCalls = pr.toolCalls
          }
          lastUsage = pr.usage

          const includeInOverall = pi !== 0 && !prompt.contextFill
          accumulateMetrics(pr, prompt, acc, includeInOverall)
        } catch (err) {
          if (err instanceof DOMException && err.name === 'AbortError') break
          repeatScores.push(0)
          errorMessage = `Error: ${err instanceof Error ? err.message : String(err)}`
          hadError = true
        }
      }

      const avgScore = repeatScores.length > 0
        ? repeatScores.reduce((s, v) => s + v, 0) / repeatScores.length
        : 0

      const dedupNotes = repeatNotes.length > 0 ? [...new Set(repeatNotes)] : undefined

      promptResults[pi] = {
        promptId: prompt.id,
        assistantText: bestAssistantText,
        toolCalls: bestToolCalls,
        usage: lastUsage,
        score: avgScore,
        notes: hadError && !dedupNotes ? [errorMessage] : dedupNotes,
      }

      // Update UI as each prompt completes
      const completedResults = promptResults.filter(Boolean)
      const partialScenario = buildScenarioResult(
        scenario.id, scenario.prompts, completedResults,
        acc.genTokens, acc.genSeconds, acc.ttft, acc.ttftCount,
        acc.fillGenTokens, acc.fillGenSeconds, acc.fillTTFT, acc.fillTTFTCount, acc.actualFills,
      )
      upsertScenarioResult(result, partialScenario, onUpdate)
    },
    signal,
  )

  if (signal.aborted) {
    result.status = 'cancelled'
    onUpdate({ ...result })
    return
  }

  // Compute effective batch TPS using wall-clock time for the concurrent batch.
  // This reflects the real throughput advantage of higher NSeqMax values.
  // acc.genTokens tracks total generated tokens across all repeats (excluding
  // context-fill and warmup prompts) for an apples-to-apples comparison with
  // the sequential path.
  const batchElapsedSeconds = (performance.now() - batchStartMs) / 1000
  const effectiveTPS = batchElapsedSeconds > 0 && acc.genTokens > 0
    ? acc.genTokens / batchElapsedSeconds
    : undefined

  const finalResults = promptResults.filter(Boolean)
  const finalScenario = buildScenarioResult(
    scenario.id, scenario.prompts, finalResults,
    acc.genTokens, acc.genSeconds, acc.ttft, acc.ttftCount,
    acc.fillGenTokens, acc.fillGenSeconds, acc.fillTTFT, acc.fillTTFTCount, acc.actualFills,
  )

  // Override avgTPS with effective batch TPS for concurrent runs
  if (effectiveTPS !== undefined) {
    finalScenario.avgTPS = effectiveTPS
  }

  upsertScenarioResult(result, finalScenario, onUpdate)
}

/** Runs a full trial for one sampling candidate across all scenarios.
 *  Each prompt is repeated `repeats` times (default 3) and results are averaged.
 *  When options.concurrency > 1, prompts within each scenario are executed
 *  concurrently (up to the given limit) to exercise multi-slot (NSeqMax) configs. */
export async function runTrial(
  sessionId: string,
  candidate: SamplingCandidate,
  scenarios: AutoTestScenario[],
  onUpdate: (result: AutoTestTrialResult) => void,
  signal: AbortSignal,
  weights?: BestConfigWeights,
  repeats: number = 3,
  options?: RunTrialOptions,
): Promise<AutoTestTrialResult> {
  const trialId = `trial-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  const safeRepeats = Math.max(1, Math.floor(repeats))
  const concurrency = Math.min(Math.max(1, options?.concurrency ?? 1), maxConcurrencyCap)

  const result: AutoTestTrialResult = {
    id: trialId,
    status: 'running',
    candidate,
    startedAt: new Date().toISOString(),
    scenarioResults: [],
  }

  onUpdate({ ...result })

  for (const scenario of scenarios) {
    if (signal.aborted) {
      result.status = 'cancelled'
      onUpdate({ ...result })
      return result
    }

    // When running concurrently, execute a single warm-up request first so
    // that SPC/IMC caches are primed before the measured batch.
    if (concurrency > 1 && scenario.prompts.length > 0) {
      try {
        await runSinglePrompt(sessionId, scenario.prompts[0], { ...candidate, max_tokens: 1 }, signal)
      } catch (err) {
        if (err instanceof DOMException && err.name === 'AbortError') {
          result.status = 'cancelled'
          onUpdate({ ...result })
          return result
        }
      }
    }

    if (concurrency > 1) {
      await runScenarioConcurrent(sessionId, candidate, scenario, safeRepeats, concurrency, result, onUpdate, signal)
    } else {
      await runScenarioSequential(sessionId, candidate, scenario, safeRepeats, result, onUpdate, signal)
    }

    if (result.status === 'cancelled') return result
  }

  const metricsByScenario = new Map<AutoTestScenarioID, { score: number; avgTPS?: number; avgTTFT?: number }>()

  for (const sr of result.scenarioResults) {
    metricsByScenario.set(sr.scenarioId, { score: sr.score, avgTPS: sr.avgTPS, avgTTFT: sr.avgTTFT })
  }

  const toolMetrics = metricsByScenario.get('tool_call')
  const chatMetrics = metricsByScenario.get('chat')

  const w = weights ?? defaultBestConfigWeights
  if (toolMetrics && chatMetrics) {
    const chatW = w.chatScore
    const toolW = w.toolScore
    const denom = chatW + toolW
    const raw = denom > 0 ? (toolW * toolMetrics.score + chatW * chatMetrics.score) / denom : (toolMetrics.score + chatMetrics.score) / 2
    result.totalScore = Math.round(raw * 100) / 100
  } else if (toolMetrics) {
    result.totalScore = toolMetrics.score
  } else if (chatMetrics) {
    result.totalScore = chatMetrics.score
  }

  // Use chat scenario metrics for TPS/TTFT; fall back to tool_call if chat is not enabled.
  const tpsSource = chatMetrics ?? toolMetrics
  result.avgTPS = tpsSource?.avgTPS
  result.avgTTFT = tpsSource?.avgTTFT

  // Propagate per-fill TPS from chat scenario
  const chatResult = result.scenarioResults.find(r => r.scenarioId === 'chat')
  if (chatResult?.avgTPSByFill && Object.keys(chatResult.avgTPSByFill).length > 0) {
    result.avgTPSByFill = chatResult.avgTPSByFill
  }

  result.finishedAt = new Date().toISOString()
  result.status = 'completed'

  onUpdate({ ...result })
  return result
}
