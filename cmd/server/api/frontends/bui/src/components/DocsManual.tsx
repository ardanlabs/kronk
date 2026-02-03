import { useState, useEffect } from 'react';
import { useLocation } from 'react-router-dom';

export default function DocsManual() {
  const [activeSection, setActiveSection] = useState('');
  const location = useLocation();

  useEffect(() => {
    if (location.hash) {
      const id = location.hash.slice(1);
      const element = document.getElementById(id);
      if (element) {
        setTimeout(() => {
          element.scrollIntoView({ behavior: 'smooth' });
        }, 100);
      }
    }
  }, [location.hash]);

  useEffect(() => {
    const handleScroll = () => {
      const sections = document.querySelectorAll('.manual-content h2, .manual-content h3');
      let current = '';
      sections.forEach((section) => {
        const rect = section.getBoundingClientRect();
        if (rect.top <= 100) {
          current = section.id;
        }
      });
      setActiveSection(current);
    };

    window.addEventListener('scroll', handleScroll);
    return () => window.removeEventListener('scroll', handleScroll);
  }, []);

  return (
    <div>
      <div className="page-header">
        <h2>Kronk Manual</h2>
        <p>Complete documentation for the Kronk Model Server</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content manual-content">
          <h1 id="kronk-model-server-user-manual">Kronk Model Server User Manual</h1>
          <h2 id="table-of-contents">Table of Contents</h2>
          <ol>
            <li><a href="#chapter-1-introduction">Introduction</a></li>
            <li><a href="#chapter-2-installation--quick-start">Installation & Quick Start</a></li>
            <li><a href="#chapter-3-model-configuration">Model Configuration</a></li>
            <li><a href="#chapter-4-batch-processing">Batch Processing</a></li>
            <li><a href="#chapter-5-message-caching">Message Caching</a></li>
            <li><a href="#chapter-6-yarn-extended-context">YaRN Extended Context</a></li>
            <li><a href="#chapter-7-model-server">Model Server</a></li>
            <li><a href="#chapter-8-api-endpoints">API Endpoints</a></li>
            <li><a href="#chapter-9-multi-modal-models">Multi-Modal Models</a></li>
            <li><a href="#chapter-10-security--authentication">Security & Authentication</a></li>
            <li><a href="#chapter-11-browser-ui-bui">Browser UI (BUI)</a></li>
            <li><a href="#chapter-12-client-integration">Client Integration</a></li>
            <li><a href="#chapter-13-observability">Observability</a></li>
            <li><a href="#chapter-14-troubleshooting">Troubleshooting</a></li>
            <li><a href="#chapter-15-developer-guide">Developer Guide</a></li>
          </ol>
          <hr />
          <h2 id="chapter-1:-introduction">Chapter 1: Introduction</h2>
          <h3 id="11-what-is-kronk-model-server">1.1 What is Kronk Model Server</h3>
          <p>Kronk Model Server (KMS) is an OpenAI and Anthropic compatible model server for running local inference with open-source GGUF models. Built on top of llama.cpp via the <a href="https://github.com/hybridgroup/yzma">yzma</a> Go bindings, Kronk provides hardware-accelerated inference for text generation, vision, audio, embeddings, and reranking.</p>
          <p>The server exposes a REST API that is compatible with:</p>
          <ul>
            <li>OpenAI client libraries</li>
            <li>OpenWebUI</li>
            <li>Agents that can be configured to work with local models</li>
            <li>Any OpenAI-compatible client</li>
          </ul>
          <h3 id="12-key-features">1.2 Key Features</h3>
          <p><strong>Model Types</strong></p>
          <ul>
            <li><strong>Text Generation</strong> - Chat completions and streaming responses with reasoning support</li>
            <li><strong>Vision</strong> - Image understanding and analysis</li>
            <li><strong>Audio</strong> - Speech-to-text and audio understanding</li>
            <li><strong>Embeddings</strong> - Vector embeddings for semantic search and RAG</li>
            <li><strong>Reranking</strong> - Document relevance scoring</li>
          </ul>
          <p><strong>Performance</strong></p>
          <ul>
            <li><strong>Batch Processing</strong> - Process multiple requests concurrently with shared KV cache</li>
            <li><strong>Message Caching</strong> - System prompt and incremental message caching to reduce redundant computation</li>
            <li><strong>YaRN Context Extension</strong> - Extend context windows 2-4x beyond native training length</li>
            <li><strong>Model Pooling</strong> - Keep models loaded in memory with configurable TTL</li>
          </ul>
          <p><strong>Operations</strong></p>
          <ul>
            <li><strong>Catalog System</strong> - Curated collection of verified models with one-command downloads</li>
            <li><strong>Browser UI (BUI)</strong> - Web interface for model management, downloads, and configuration</li>
            <li><strong>Authentication</strong> - JWT-based security with key management and endpoint authorization</li>
            <li><strong>Observability</strong> - Tempo tracing integration and debug endpoints</li>
          </ul>
          <h3 id="13-supported-platforms-and-hardware">1.3 Supported Platforms and Hardware</h3>
          <p>Kronk supports full hardware acceleration across major platforms:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>OS</th>
                <th>CPU</th>
                <th>GPU</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Linux</td>
                <td>amd64, arm64</td>
                <td>CUDA, Vulkan, HIP, ROCm, SYCL</td>
              </tr>
              <tr>
                <td>macOS</td>
                <td>arm64</td>
                <td>Metal</td>
              </tr>
              <tr>
                <td>Windows</td>
                <td>amd64</td>
                <td>CUDA, Vulkan, HIP, SYCL, OpenCL</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Hardware Requirements</strong></p>
          <ul>
            <li>Minimum 8GB RAM for small models (1-3B parameters)</li>
            <li>16GB+ RAM recommended for medium models (7-8B parameters)</li>
            <li>32GB+ RAM or dedicated GPU VRAM for large models (30B+ parameters)</li>
            <li>GPU with Metal, CUDA, or Vulkan support recommended for optimal performance</li>
          </ul>
          <h3 id="14-architecture-overview">1.4 Architecture Overview</h3>
          <pre className="code-block"><code>{`┌────────────────────────────────────────────────────────────────────┐
│                         Kronk Model Server                         │
├────────────────────────────────────────────────────────────────────┤
│                     REST API (OpenAI Compatible)                   │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │
│  │   Chat   │ │ Response │ │  Embed   │ │  Rerank  │ │   Msgs   │  │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘  │
├───────┼────────────┼────────────┼────────────┼────────────┼────────┤
│       └────────────┴──────────┬─┴────────────┴────────────┘        │
│                               ▼                                    │
│      ┌─────────────────────────────────────────────────────┐       │
│      │              Kronk SDK (Model Pool)                 │       │
│      │  ┌─────────┐  ┌─────────┐  ┌─────────┐              │       │
│      │  │ Model A │  │ Model B │  │ Model C │  (cached)    │       │
│      │  └────┬────┘  └────┬────┘  └────┬────┘              │       │
│      └───────┼────────────┼────────────┼───────────────────┘       │
├──────────────┼────────────┼────────────┼───────────────────────────│
│          .   └────────────┴─────┬──────┘                           │
│                                 ▼                                  │
│      ┌─────────────────────────────────────────────────────┐       │
│      │         yzma (llama.cpp Go Bindings)                │       │
│      └─────────────────────────────────────────────────────┘       │
├────────────────────────────────────────────────────────────────────┤
│        Hardware Acceleration: Metal │ CUDA │ Vulkan │ CPU          │
└────────────────────────────────────────────────────────────────────┘`}</code></pre>
          <p><strong>Request Flow</strong></p>
          <ol>
            <li>Client sends request to REST API endpoint</li>
            <li>Server routes to appropriate handler (chat, embed, rerank)</li>
            <li>Model is acquired from pool (or loaded if not cached)</li>
            <li>For text models with batch processing enabled, requests queue into batch slots</li>
            <li>Message caching checks for reusable KV state from previous requests</li>
            <li>Inference runs with hardware acceleration</li>
            <li>Response streams back to client (for streaming requests)</li>
            <li>Model returns to pool for reuse</li>
          </ol>
          <hr />
          <h2 id="chapter-2:-installation-quick-start">Chapter 2: Installation &amp; Quick Start</h2>
          <h3 id="21-prerequisites">2.1 Prerequisites</h3>
          <p><strong>Required</strong></p>
          <ul>
            <li>Go 1.25 or later</li>
            <li>Internet connection (for downloading libraries and models)</li>
          </ul>
          <p><strong>Recommended</strong></p>
          <ul>
            <li>GPU with Metal (macOS), CUDA (NVIDIA), or Vulkan support</li>
            <li>16GB+ system RAM (96GB+ Recommended)</li>
          </ul>
          <h3 id="22-installing-the-cli">2.2 Installing the CLI</h3>
          <p>Install Kronk using Go:</p>
          <pre className="code-block"><code className="language-shell">{`go install github.com/ardanlabs/kronk/cmd/kronk@latest`}</code></pre>
          <p>Verify the installation:</p>
          <pre className="code-block"><code className="language-shell">{`kronk --help`}</code></pre>
          <p>You should see output listing available commands:</p>
          <pre className="code-block"><code>{`Kronk CLI - A tool for managing Kronk models

Usage:
  kronk [command]

Available Commands:
  catalog     Manage model catalog
  libs        Install or upgrade llama.cpp libraries
  model       Manage models
  run         Run a model directly for quick testing
  security    Manage security keys and tokens
  server      Manage Kronk model server
  help        Help about any command`}</code></pre>
          <h3 id="23-installing-libraries">2.3 Installing Libraries</h3>
          <p>Before running inference, you need the llama.cpp libraries for your platform. Kronk auto-detects your hardware and downloads the appropriate binaries.</p>
          <p><strong>Option A: Via the Server (Recommended)</strong></p>
          <p>Start the server and use the BUI to download libraries:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start`}</code></pre>
          <p>Open http://localhost:8080 in your browser and navigate to the Libraries page.</p>
          <p><strong>Option B: Via CLI</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local`}</code></pre>
          <p>This downloads libraries to <code>~/.kronk/libraries/</code> using auto-detected settings.</p>
          <p><strong>Environment Variables for Library Installation</strong></p>
          <pre className="code-block"><code>{`KRONK_LIB_PATH  - Library directory (default: \`~/.kronk/libraries\`)
KRONK_PROCESSOR - \`cpu\`, \`cuda\`, \`metal\`, or \`vulkan\` (default: \`cpu\`)
KRONK_ARCH      - Architecture override: \`amd64\`, \`arm64\`
KRONK_OS        - OS override: \`linux\`, \`darwin\`, \`windows\``}</code></pre>
          <p><strong>Example: Install CUDA Libraries</strong></p>
          <pre className="code-block"><code className="language-shell">{`KRONK_PROCESSOR=cuda kronk libs --local`}</code></pre>
          <h3 id="24-downloading-your-first-model">2.4 Downloading Your First Model</h3>
          <p>Kronk provides a curated catalog of verified models. List available models:</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list --local`}</code></pre>
          <p>Output:</p>
          <pre className="code-block"><code>{`CATALOG              MODEL ID                            PULLED   ENDPOINT
Audio-Text-to-Text   Qwen2-Audio-7B.Q8_0                 no       chat_completion
Embedding            embeddinggemma-300m-qat-Q8_0        no       embeddings
Image-Text-to-Text   gemma-3-4b-it-q4_0                  no       chat_completion
Text-Generation      Qwen3-8B-Q8_0                       no       chat_completion
Text-Generation      Llama-3.3-70B-Instruct-Q8_0         no       chat_completion
...`}</code></pre>
          <p>Download a model (recommended starter: Qwen3-8B):</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog pull Qwen3-8B-Q8_0 --local`}</code></pre>
          <p>Models are stored in <code>~/.kronk/models/</code> by default.</p>
          <h3 id="25-starting-the-server">2.5 Starting the Server</h3>
          <p>Start the Kronk Model Server:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start`}</code></pre>
          <p>The server starts on <code>http://localhost:8080</code> by default. You'll see output like:</p>
          <pre className="code-block"><code>{`Kronk Model Server started
API: http://localhost:8080
BUI: http://localhost:8080`}</code></pre>
          <p><strong>Running in Background</strong></p>
          <p>To run the server as a background process:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start -d`}</code></pre>
          <p><strong>Stopping the Server</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server stop`}</code></pre>
          <h3 id="26-verifying-the-installation">2.6 Verifying the Installation</h3>
          <p><strong>Test via curl</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/models`}</code></pre>
          <p>You should see a list of available models.</p>
          <p><strong>Test Chat Completion</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 100
  }'`}</code></pre>
          <p><strong>Test via BUI</strong></p>
          <p>Open http://localhost:8080 in your browser. The Browser UI provides:</p>
          <ul>
            <li>Model management and downloads</li>
            <li>Library installation</li>
            <li>Server configuration</li>
            <li>Security key management</li>
          </ul>
          <h3 id="27-quick-start-summary">2.7 Quick Start Summary</h3>
          <pre className="code-block"><code className="language-shell">{`# 1. Install Kronk
go install github.com/ardanlabs/kronk/cmd/kronk@latest

# 2. Start the server (auto-installs libraries on first run)
kronk server start

# 3. Open BUI and download a model
open http://localhost:8080

# 4. Or download via CLI
kronk catalog pull Qwen3-8B-Q8_0 --local

# 5. Test the API
curl http://localhost:8080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{"model": "Qwen3-8B-Q8_0", "messages": [{"role": "user", "content": "Hello!"}]}'`}</code></pre>
          <hr />
          <h2 id="chapter-3:-model-configuration">Chapter 3: Model Configuration</h2>
          <p>Model configuration controls how Kronk loads and runs inference. Configuration</p>
          <p>can be set via model config files, catalog templates, or programmatically</p>
          <p>through the SDK.</p>
          <h3 id="31-basic-configuration">3.1 Basic Configuration</h3>
          <p><strong>Context Window</strong></p>
          <p>The context window defines the maximum number of tokens the model can process</p>
          <p>in a single request. This includes both the input prompt and generated output.</p>
          <pre className="code-block"><code className="language-yaml">{`context_window: 8192 # Default: 8192 tokens`}</code></pre>
          <p>Larger context windows require more VRAM. A rough estimate:</p>
          <ul>
            <li><code>8K context</code>: ~2GB additional VRAM</li>
            <li><code>32K context</code>: ~8GB additional VRAM</li>
            <li><code>128K context</code>: ~32GB additional VRAM (requires YaRN scaling)</li>
          </ul>
          <p><strong>Batch Size Configuration</strong></p>
          <p>Two parameters control how tokens are processed:</p>
          <ul>
            <li><code>n_batch</code> - Maximum tokens in a single forward pass (default: 2048)</li>
            <li><code>n_ubatch</code> - Physical batch size for prompt processing (default: 512)</li>
          </ul>
          <pre className="code-block"><code className="language-yaml">{`n_batch: 2048 # Logical batch size
n_ubatch: 512 # Physical batch size (must be ≤ n_batch)`}</code></pre>
          <p><strong>Recommended settings by workload:</strong></p>
          <ul>
            <li>Interactive chat (single user): <code>n_batch=512-1024</code>, <code>n_ubatch=512</code></li>
            <li>Long prompts/RAG: <code>n_batch=2048-4096</code>, <code>n_ubatch=512-1024</code></li>
            <li>Batch inference (multiple prompts): <code>n_batch=2048-4096</code>, <code>n_ubatch=512</code></li>
            <li>Low VRAM (&lt;8GB): <code>n_batch=512</code>, <code>n_ubatch=256-512</code></li>
            <li>High VRAM (24GB+): <code>n_batch=4096+</code>, <code>n_ubatch=1024+</code></li>
          </ul>
          <h3 id="32-sampling-parameters">3.2 Sampling Parameters</h3>
          <p>Sampling parameters control the randomness and quality of generated text.</p>
          <p>These are set per-request in the API call.</p>
          <p><strong>Temperature</strong></p>
          <p>Controls randomness. Lower values produce more deterministic output.</p>
          <pre className="code-block"><code className="language-json">{`{
  "temperature": 0.8
}`}</code></pre>
          <ul>
            <li><code>0.0-0.3</code> - Focused, deterministic (good for code, factual Q&A)</li>
            <li><code>0.5-0.8</code> - Balanced (good for general chat)</li>
            <li><code>0.9-1.2</code> - Creative (good for storytelling, brainstorming)</li>
          </ul>
          <p><strong>Top-K and Top-P</strong></p>
          <p>Limit the token selection pool:</p>
          <pre className="code-block"><code className="language-json">{`{
  "top_k": 40,
  "top_p": 0.9
}`}</code></pre>
          <ul>
            <li><code>top_k</code> - Consider only the K most probable tokens (default: 40)</li>
            <li><code>top_p</code> - Consider tokens until cumulative probability reaches P (default: 0.9)</li>
          </ul>
          <p><strong>Repetition Control</strong></p>
          <p>Reduce repetitive output:</p>
          <pre className="code-block"><code className="language-json">{`{
  "repeat_penalty": 1.1,
  "repeat_last_n": 64
}`}</code></pre>
          <ul>
            <li><code>repeat_penalty</code> - Penalty for repeated tokens (1.0 = off, 1.1 = mild)</li>
            <li><code>repeat_last_n</code> - How many recent tokens to check (default: 64)</li>
          </ul>
          <p><strong>DRY Sampler (Don't Repeat Yourself)</strong></p>
          <p>Advanced n-gram repetition penalty:</p>
          <pre className="code-block"><code className="language-json">{`{
  "dry_multiplier": 1.05,
  "dry_base": 1.75,
  "dry_allowed_length": 2
}`}</code></pre>
          <p><strong>Max Tokens</strong></p>
          <p>Limit the response length:</p>
          <pre className="code-block"><code className="language-json">{`{
  "max_tokens": 2048
}`}</code></pre>
          <h3 id="33-gpu-configuration">3.3 GPU Configuration</h3>
          <p><strong>Layer Offloading</strong></p>
          <p>Control how many model layers run on GPU:</p>
          <pre className="code-block"><code className="language-yaml">{`n_gpu_layers: 0      # 0 = all layers on GPU (default)
n_gpu_layers: -1     # All layers on CPU
n_gpu_layers: 20     # First 20 layers on GPU`}</code></pre>
          <p><strong>KV Cache Location</strong></p>
          <p>The KV cache stores attention state and can consume significant VRAM:</p>
          <pre className="code-block"><code className="language-yaml">{`offload_kqv: true    # KV cache on GPU (default, faster)
offload_kqv: false   # KV cache on CPU (saves VRAM, slower)`}</code></pre>
          <p><strong>Tensor Operations Offload</strong></p>
          <p>Control where tensor computations run:</p>
          <pre className="code-block"><code className="language-yaml">{`op_offload: true     # Tensor ops on GPU (default)
op_offload: false    # Tensor ops on CPU`}</code></pre>
          <p>Use <code>op_offload: false</code> when you need to run the model on CPU but want to</p>
          <p>keep some layers on GPU for memory.</p>
          <p><strong>Multi-GPU Split Mode</strong></p>
          <p>For systems with multiple GPUs:</p>
          <pre className="code-block"><code className="language-yaml">{`split_mode: none     # Single GPU (default)
split_mode: layer    # Split layers across GPUs
split_mode: row      # Tensor parallelism (best for MoE models)`}</code></pre>
          <p>Use <code>row</code> for Mixture of Experts models like Qwen3-MoE, Mixtral, or DeepSeek.</p>
          <p><strong>Configuration Reference</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Field</th>
                <th>YAML Key</th>
                <th>Values</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>NGpuLayers</td>
                <td><code>n_gpu_layers</code></td>
                <td>0, -1, N</td>
                <td>0</td>
                <td>Layers on GPU (0=all, -1=none)</td>
              </tr>
              <tr>
                <td>OffloadKQV</td>
                <td><code>offload_kqv</code></td>
                <td>true/false</td>
                <td>true</td>
                <td>KV cache on GPU</td>
              </tr>
              <tr>
                <td>OpOffload</td>
                <td><code>op_offload</code></td>
                <td>true/false</td>
                <td>true</td>
                <td>Tensor ops on GPU</td>
              </tr>
              <tr>
                <td>SplitMode</td>
                <td><code>split_mode</code></td>
                <td>none/layer/row</td>
                <td>none</td>
                <td>Multi-GPU distribution</td>
              </tr>
            </tbody>
          </table>
          <h3 id="34-kv-cache-quantization">3.4 KV Cache Quantization</h3>
          <p>Reduce VRAM usage by quantizing the KV cache:</p>
          <pre className="code-block"><code className="language-yaml">{`cache_type_k: q8_0 # Key cache precision
cache_type_v: q8_0 # Value cache precision`}</code></pre>
          <p><strong>Available types:</strong></p>
          <ul>
            <li><code>f16</code> - Half precision (default, best quality)</li>
            <li><code>q8_0</code> - 8-bit quantization (good balance)</li>
            <li><code>q4_0</code> - 4-bit quantization (aggressive, may affect quality)</li>
            <li><code>bf16</code> - Brain float 16 (for supported hardware)</li>
          </ul>
          <p><strong>VRAM savings with Q8_0 cache:</strong></p>
          <ul>
            <li>8K context: ~25% reduction</li>
            <li>32K context: ~25% reduction</li>
            <li>Larger contexts benefit proportionally</li>
          </ul>
          <h3 id="35-flash-attention">3.5 Flash Attention</h3>
          <p>Flash Attention optimizes memory usage and speeds up attention computation:</p>
          <pre className="code-block"><code className="language-yaml">{`flash_attention: enabled   # Default: enabled
flash_attention: disabled  # Disable if causing issues
flash_attention: auto      # Let llama.cpp decide`}</code></pre>
          <p>Flash Attention is particularly beneficial for large context windows.</p>
          <h3 id="36-parallel-inference-nseqmax">3.6 Parallel Inference (NSeqMax)</h3>
          <p><code>NSeqMax</code> controls concurrent request handling, but behaves differently based</p>
          <p>on model type:</p>
          <p><strong>Text Models (Chat/Completion)</strong></p>
          <p>For text models, <code>NSeqMax</code> controls batch parallelism within a single model:</p>
          <pre className="code-block"><code className="language-yaml">{`n_seq_max: 4 # Process up to 4 requests concurrently`}</code></pre>
          <p>Multiple requests share the model context and KV cache, with each request</p>
          <p>getting an isolated sequence partition.</p>
          <p><strong>Sequential Models (Embed/Rerank/Vision/Audio)</strong></p>
          <p>For sequential models, <code>NSeqMax</code> creates multiple model instances:</p>
          <pre className="code-block"><code className="language-yaml">{`n_seq_max: 2 # Create 2 model instances in pool`}</code></pre>
          <p>Each instance handles one request at a time, but multiple instances allow</p>
          <p>concurrent processing.</p>
          <h3 id="37-vram-estimation">3.7 VRAM Estimation</h3>
          <p>Rough VRAM requirements for common configurations:</p>
          <p><strong>Model Size (Q8_0 quantization)</strong></p>
          <ul>
            <li>1-3B parameters: 2-4 GB</li>
            <li>7-8B parameters: 8-10 GB</li>
            <li>13B parameters: 14-16 GB</li>
            <li>30B parameters: 32-36 GB</li>
            <li>70B parameters: 72-80 GB</li>
          </ul>
          <p><strong>Additional VRAM for Context</strong></p>
          <p>Per 1K tokens of context (with F16 KV cache):</p>
          <ul>
            <li>7B model: ~50 MB</li>
            <li>13B model: ~80 MB</li>
            <li>70B model: ~200 MB</li>
          </ul>
          <p><strong>Example: Qwen3-8B with 32K context</strong></p>
          <pre className="code-block"><code>{`Model weights (Q8_0):     ~8.5 GB
KV cache (32K, F16):      ~1.6 GB
Overhead:                 ~0.5 GB
─────────────────────────────────
Total:                    ~10.6 GB`}</code></pre>
          <p>With Q8_0 KV cache quantization, the KV cache drops to ~0.8 GB.</p>
          <h3 id="38-model-config-file-example">3.8 Model Config File Example</h3>
          <p>Create a YAML config file for custom model settings:</p>
          <pre className="code-block"><code className="language-yaml">{`# model-config.yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 32768
    n_batch: 2048
    n_ubatch: 512
    n_seq_max: 2
    cache_type_k: q8_0
    cache_type_v: q8_0
    flash_attention: enabled
    system_prompt_cache: true

  Llama-3.3-70B-Instruct-Q8_0:
    context_window: 8192
    n_gpu_layers: 0
    split_mode: row
    offload_kqv: true`}</code></pre>
          <p>Start the server with custom config:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --model-config-file=model-config.yaml`}</code></pre>
          <h3 id="39-model-specific-tuning">3.9 Model-Specific Tuning</h3>
          <p>Different model architectures have specific optimization requirements.</p>
          <p><strong>Vision and Audio Models</strong></p>
          <p>Keep <code>n_ubatch</code> high for efficient media token processing:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen2.5-VL-3B-Instruct-Q8_0:
    n_batch: 2048
    n_ubatch: 2048 # High for image/audio token batches
    n_seq_max: 2 # Creates 2 model instances in pool`}</code></pre>
          <p>Vision models process image tiles as large token batches. Low <code>n_ubatch</code></p>
          <p>values cause multiple decode passes per image, significantly slowing</p>
          <p>inference.</p>
          <p><strong>Mixture of Experts (MoE) Models</strong></p>
          <p>Use row-based tensor parallelism for multi-GPU setups:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-MoE-30B-A3B-Q8_0:
    split_mode: row # Best for MoE architecture
    cache_type_k: q8_0 # Be cautious with aggressive quantization
    cache_type_v: q8_0`}</code></pre>
          <p>MoE models can be sensitive to aggressive KV cache quantization. If you</p>
          <p>notice quality degradation, try <code>f16</code> cache types.</p>
          <p><strong>Embedding Models</strong></p>
          <p>Optimize batch size for your typical input lengths:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  embeddinggemma-300m-qat-Q8_0:
    n_batch: 8192 # Can equal context_window
    n_ubatch: 512 # Align with typical sliding window
    n_seq_max: 4 # 4 model instances for concurrency`}</code></pre>
          <p>Embedding models process complete inputs in a single pass, so larger</p>
          <p><code>n_batch</code> values improve throughput.</p>
          <hr />
          <h2 id="chapter-4:-batch-processing">Chapter 4: Batch Processing</h2>
          <p>Batch processing allows Kronk to handle multiple concurrent requests</p>
          <p>efficiently by sharing model resources. This chapter explains the architecture</p>
          <p>and how to optimize for your workload.</p>
          <h3 id="41-architecture-overview">4.1 Architecture Overview</h3>
          <p>When <code>NSeqMax &gt; 1</code> for text models, Kronk creates a batch engine that</p>
          <p>processes multiple requests in parallel within a single model instance.</p>
          <pre className="code-block"><code>{`                    ┌───────────────────────────────────┐
   Request 1 ──────▶│                                   │
                    │          Request Queue            │
   Request 2 ──────▶│      (capacity: NSeqMax × 2)      │
                    │                                   │
   Request 3 ──────▶│                                   │
                    └────────────────┬──────────────────┘
                                     │
                                     ▼
                    ┌───────────────────────────────────┐
                    │           Batch Engine            │
                    │                                   │
                    │  ┌─────────┐  ┌─────────┐         │
                    │  │ Slot 0  │  │ Slot 1  │  ...    │
                    │  │ seqID=0 │  │ seqID=1 │         │
                    │  └────┬────┘  └────┬────┘    ┬    │
                    │       │            │         │    │
                    │       └─────────┬──┴─────────┘    │
                    │                 ▼                 │
                    │          Shared KV Cache          │
                    │  ┌─────────────────────────────┐  │
                    │  │ seq 0 │ seq 1 │ seq 2 │ ... │  │
                    │  └─────────────────────────────┘  │
                    └───────────────────────────────────┘
                                      │
                                      ▼
                    ┌───────────────────────────────────┐
                    │        llama.cpp Backend          │
                    │         (GPU/CPU Inference)       │
                    └───────────────────────────────────┘`}</code></pre>
          <h3 id="42-slots-and-sequences">4.2 Slots and Sequences</h3>
          <p><strong>Slots</strong> are processing units that handle individual requests. Each slot</p>
          <p>tracks its state: prompt tokens, decode position, sampler, and response</p>
          <p>channel.</p>
          <p><strong>Sequences</strong> are isolated partitions in the shared KV cache. Each slot is</p>
          <p>assigned a unique sequence ID, ensuring requests don't interfere with each</p>
          <p>other's attention state.</p>
          <pre className="code-block"><code>{`NSeqMax = 4 (without caching)

Slot 0  →  seqID = 0  →  KV cache partition 0
Slot 1  →  seqID = 1  →  KV cache partition 1
Slot 2  →  seqID = 2  →  KV cache partition 2
Slot 3  →  seqID = 3  →  KV cache partition 3`}</code></pre>
          <p>When caching is enabled, sequence 0 is reserved for cached content:</p>
          <pre className="code-block"><code>{`NSeqMax = 2 (with System Prompt Cache)

Cache   →  seqID = 0  →  Cached system prompt KV state
Slot 0  →  seqID = 1  →  KV cache partition 1
Slot 1  →  seqID = 2  →  KV cache partition 2`}</code></pre>
          <h3 id="43-request-flow">4.3 Request Flow</h3>
          <ol>
            <li><strong>Queue</strong>: Request enters the queue (backpressure if full)</li>
            <li><strong>Assign</strong>: Available slot picks up the request</li>
            <li><strong>Clear</strong>: Slot clears its sequence partition</li>
            <li><strong>Cache Check</strong>: If caching enabled, copy cached KV state to slot's sequence</li>
            <li><strong>Prefill</strong>: Tokenize and process prompt tokens</li>
            <li><strong>Decode</strong>: Generate tokens one at a time, streaming to client</li>
            <li><strong>Complete</strong>: Clear sequence, slot becomes available</li>
          </ol>
          <h3 id="44-configuring-batch-processing">4.4 Configuring Batch Processing</h3>
          <p><strong>Enable Batch Processing</strong></p>
          <p>Set <code>NSeqMax &gt; 1</code> in your model config:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    n_seq_max: 4 # 4 concurrent requests`}</code></pre>
          <p><strong>Queue Depth</strong></p>
          <p>The request queue holds <code>NSeqMax × 2</code> requests. With <code>NSeqMax=4</code>, up to 8</p>
          <p>requests can queue while 4 are actively processing.</p>
          <p><strong>Memory Considerations</strong></p>
          <p>Each slot needs its own KV cache partition. With 4 slots and 8K context:</p>
          <pre className="code-block"><code>{`KV cache per slot:  ~200 MB (for 8B model with F16)
Total KV cache:     ~800 MB (4 slots × 200 MB)`}</code></pre>
          <p><strong>Caching Memory Overhead</strong></p>
          <p>When message caching is enabled, additional sequences are reserved:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>SPC</th>
                <th>IMC</th>
                <th>MaxCacheSessions</th>
                <th>Reserved Seqs</th>
                <th>Slot Start</th>
                <th>Memory Overhead</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>off</td>
                <td>off</td>
                <td>-</td>
                <td>0</td>
                <td>seq 0</td>
                <td>none</td>
              </tr>
              <tr>
                <td>on</td>
                <td>off</td>
                <td>1</td>
                <td>1</td>
                <td>seq 1</td>
                <td>+1 context window</td>
              </tr>
              <tr>
                <td>on</td>
                <td>off</td>
                <td>4</td>
                <td>4</td>
                <td>seq 4</td>
                <td>+4 context windows</td>
              </tr>
              <tr>
                <td>off</td>
                <td>on</td>
                <td>1</td>
                <td>1</td>
                <td>seq 1</td>
                <td>+1 context window</td>
              </tr>
              <tr>
                <td>off</td>
                <td>on</td>
                <td>4</td>
                <td>4</td>
                <td>seq 4</td>
                <td>+4 context windows</td>
              </tr>
            </tbody>
          </table>
          <p>Example with <code>max-cache-sessions=3</code> and <code>n_seq_max=2</code>:</p>
          <pre className="code-block"><code>{`seq 0: user-1 cache (IMC)
seq 1: user-2 cache (IMC)
seq 2: user-3 cache (IMC)
seq 3: slot[0] inference
seq 4: slot[1] inference`}</code></pre>
          <p>Each cache sequence requires one full context window of KV memory.</p>
          <h3 id="45-batch-vs-sequential-models">4.5 Batch vs Sequential Models</h3>
          <p>The batch engine is only used for <strong>text-only</strong> requests. Other model types</p>
          <p>use sequential processing with model pooling:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Model Type</th>
                <th>NSeqMax Behavior</th>
                <th>Concurrency Method</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Text (chat, completion)</td>
                <td>Batch parallelism</td>
                <td>Shared model, multiple slots</td>
              </tr>
              <tr>
                <td>Embedding</td>
                <td>Model pool</td>
                <td>Multiple model instances</td>
              </tr>
              <tr>
                <td>Reranking</td>
                <td>Model pool</td>
                <td>Multiple model instances</td>
              </tr>
              <tr>
                <td>Vision</td>
                <td>Model pool</td>
                <td>Multiple model instances</td>
              </tr>
              <tr>
                <td>Audio</td>
                <td>Model pool</td>
                <td>Multiple model instances</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Why Vision/Audio Can't Batch</strong></p>
          <p>Media models require exclusive model context for processing image/audio</p>
          <p>tokens through a separate projector pipeline. Each request needs its own</p>
          <p>context for media embedding.</p>
          <h3 id="46-performance-tuning">4.6 Performance Tuning</h3>
          <p><strong>Throughput vs Latency</strong></p>
          <ul>
            <li>Higher <code>NSeqMax</code>: Better throughput, potentially higher per-request latency</li>
            <li>Lower <code>NSeqMax</code>: Lower latency, less concurrent capacity</li>
          </ul>
          <p><strong>Recommended Settings</strong></p>
          <ul>
            <li>Single user, interactive: <code>n_seq_max: 1-2</code></li>
            <li>Multi-user API server: <code>n_seq_max: 4-8</code></li>
            <li>High-throughput batch jobs: <code>n_seq_max: 8-16</code></li>
          </ul>
          <p><strong>Monitoring</strong></p>
          <p>Watch for queue backpressure. If requests consistently queue, consider:</p>
          <ol>
            <li>Increasing <code>NSeqMax</code> (if VRAM allows)</li>
            <li>Reducing <code>context_window</code> to fit more slots</li>
            <li>Using KV cache quantization (<code>cache_type_k/v: q8_0</code>)</li>
          </ol>
          <h3 id="47-example-configuration">4.7 Example Configuration</h3>
          <p>High-throughput server configuration:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    context_window: 8192
    n_seq_max: 8
    n_batch: 2048
    n_ubatch: 512
    cache_type_k: q8_0
    cache_type_v: q8_0
    system_prompt_cache: true`}</code></pre>
          <p>This configuration:</p>
          <ul>
            <li>Handles 8 concurrent requests</li>
            <li>Uses quantized KV cache to reduce memory</li>
            <li>Caches system prompt for faster prefill</li>
          </ul>
          <hr />
          <h2 id="chapter-5:-message-caching">Chapter 5: Message Caching</h2>
          <p>Message caching reduces redundant computation by storing and reusing KV cache</p>
          <p>state from previous requests. Kronk provides two caching modes optimized for</p>
          <p>different use cases.</p>
          <h3 id="51-overview">5.1 Overview</h3>
          <p>When processing a chat request, the model must compute attention for every</p>
          <p>token in the conversation. For long conversations or repeated system prompts,</p>
          <p>this becomes wasteful—the same tokens are reprocessed on every request.</p>
          <p>Message caching stores the computed KV state and copies it to new requests,</p>
          <p>skipping the prefill phase for cached tokens.</p>
          <pre className="code-block"><code>{`Without Caching:
┌─────────────────────────────────────────────────────┐
│ System Prompt │ Message 1 │ Message 2 │ New Message │
│   (prefill)   │ (prefill) │ (prefill) │  (prefill)  │
└─────────────────────────────────────────────────────┘
                                              ↓
                                         Generate

With Caching:
┌─────────────────────────────────────────────────────┐
│ System Prompt │ Message 1 │ Message 2 │ New Message │
│   (cached)    │ (cached)  │ (cached)  │  (prefill)  │
└─────────────────────────────────────────────────────┘
                                              ↓
                                         Generate`}</code></pre>
          <h3 id="52-system-prompt-cache-spc">5.2 System Prompt Cache (SPC)</h3>
          <p>System Prompt Cache stores the KV state of the first system message and</p>
          <p>reuses it across all requests with the same system prompt.</p>
          <p><strong>Best for:</strong></p>
          <ul>
            <li>Models with inconsistent templates (GPT-OSS, GLM)</li>
            <li>OpenWebUI and similar chat interfaces</li>
            <li>Applications with a consistent system prompt</li>
            <li>Single-user or shared system prompt scenarios</li>
          </ul>
          <p><strong>Enable SPC:</strong></p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    system_prompt_cache: true`}</code></pre>
          <p><strong>How It Works:</strong></p>
          <ol>
            <li>First request: System prompt is processed and cached in sequence 0</li>
            <li>Subsequent requests: Cached KV state is copied to the slot's sequence</li>
            <li>Only the new messages need prefill processing</li>
          </ol>
          <p><strong>Cache Invalidation:</strong></p>
          <p>The cache is automatically invalidated when:</p>
          <ul>
            <li>The system prompt content changes</li>
            <li>The system prompt role changes</li>
            <li>The server restarts</li>
          </ul>
          <h3 id="53-incremental-message-cache-imc">5.3 Incremental Message Cache (IMC)</h3>
          <p>Incremental Message Cache is designed for agentic workflows where</p>
          <p>conversations grow monotonically. It caches all messages except the last</p>
          <p>one and extends the cache incrementally on each turn.</p>
          <p><strong>Requires:</strong> Models with consistent templates where the same messages always</p>
          <p>produce identical templated output regardless of conversation length. Models</p>
          <p>like QWEN and Llama have consistent templates. Models like GPT-OSS and GLM</p>
          <p>inject tool calls in ways that change earlier message rendering, making them</p>
          <p>incompatible with IMC (use SPC instead).</p>
          <p><strong>Best for:</strong></p>
          <ul>
            <li>Models with consistent templates (QWEN, Llama)</li>
            <li>AI coding agents (Cline, OpenCode, Aider)</li>
            <li>Long-running agent conversations</li>
            <li>Any workflow where messages are appended, not edited</li>
          </ul>
          <p><strong>Enable IMC:</strong></p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    incremental_cache: true
    max_cache_sessions: 4 # Support 4 concurrent users
    cache_min_tokens: 100 # Minimum tokens before caching`}</code></pre>
          <p><strong>How It Works:</strong></p>
          <p>First request (2 messages: system + user):</p>
          <pre className="code-block"><code>{`Messages: [system, user]
Cache:    [system]           ← Cache all except last
Prefill:  [user + gen_prompt]`}</code></pre>
          <p>Second request (4 messages):</p>
          <pre className="code-block"><code>{`Messages: [system, user, assistant, user2]
Cache:    [system, user, assistant]  ← Extend cache
Prefill:  [user2 + gen_prompt]`}</code></pre>
          <p>Third request (6 messages):</p>
          <pre className="code-block"><code>{`Messages: [system, user, assistant, user2, assistant2, user3]
Cache:    [system, user, assistant, user2, assistant2]  ← Extend
Prefill:  [user3 + gen_prompt]`}</code></pre>
          <h3 id="54-multi-user-caching">5.4 Multi-User Caching</h3>
          <p>Both SPC and IMC support multiple concurrent users, each with their own cache sequence.</p>
          <p>Users are identified by the <code>cache_id</code> parameter in requests.</p>
          <p><strong>Configuration:</strong></p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    # For SPC or IMC - both use cache_id for multi-user support
    system_prompt_cache: true # OR incremental_cache: true
    max_cache_sessions: 4 # 4 concurrent user caches`}</code></pre>
          <p><strong>Passing Cache ID:</strong></p>
          <p>Via HTTP header:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "KRONK_CACHE_ID: user-123" \\
  -d '{"model": "Qwen3-8B-Q8_0", "messages": [...]}'`}</code></pre>
          <p>Or in the request body:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen3-8B-Q8_0",
  "cache_id": "user-123",
  "messages": [...]
}`}</code></pre>
          <p><strong>Sequence Allocation:</strong></p>
          <p>With <code>max_cache_sessions=3</code> and <code>n_seq_max=2</code>:</p>
          <pre className="code-block"><code>{`seq 0: user-1 cache
seq 1: user-2 cache
seq 2: user-3 cache
seq 3: slot[0] inference
seq 4: slot[1] inference`}</code></pre>
          <p>If all cache slots are in use, new sessions bypass IMC gracefully.</p>
          <h3 id="55-spc-vs-imc">5.5 SPC vs IMC</h3>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Feature</th>
                <th>System Prompt Cache</th>
                <th>Incremental Message Cache</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Caches</td>
                <td>System prompt only</td>
                <td>All messages except last</td>
              </tr>
              <tr>
                <td>Extends</td>
                <td>No</td>
                <td>Yes, incrementally</td>
              </tr>
              <tr>
                <td>Multi-user</td>
                <td>Per-user cache (dedicated sequences)</td>
                <td>Per-user cache (dedicated sequences)</td>
              </tr>
              <tr>
                <td>Best for</td>
                <td>Chat UIs, inconsistent templates</td>
                <td>Agentic workflows, consistent templates</td>
              </tr>
              <tr>
                <td>Memory</td>
                <td>N extra sequences (max_cache_sessions)</td>
                <td>N extra sequences (max_cache_sessions)</td>
              </tr>
              <tr>
                <td>Template req</td>
                <td>Any</td>
                <td>Consistent templates only</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Important:</strong> SPC and IMC are mutually exclusive. Choose based on your</p>
          <p>model's template behavior:</p>
          <ul>
            <li><strong>Consistent templates (QWEN, Llama):</strong> Use IMC for maximum cache efficiency</li>
            <li><strong>Inconsistent templates (GPT-OSS, GLM):</strong> Use SPC only</li>
          </ul>
          <h3 id="56-cache-invalidation">5.6 Cache Invalidation</h3>
          <p><strong>SPC Invalidation:</strong></p>
          <ul>
            <li>System prompt content changes → cache rebuilt</li>
            <li>System prompt hash mismatch → cache rebuilt</li>
          </ul>
          <p><strong>IMC Invalidation:</strong></p>
          <ul>
            <li>Message prefix hash mismatch → cache rebuilt from scratch</li>
            <li>User starts new conversation → new cache created</li>
            <li>Earlier message edited → cache rebuilt</li>
            <li><code>cache_id</code> not provided → falls back to "default" ID (problematic for multi-user)</li>
          </ul>
          <p><strong>Automatic Invalidation:</strong></p>
          <p>Caches are cleared when:</p>
          <ul>
            <li>Model is unloaded</li>
            <li>Server restarts</li>
          </ul>
          <h3 id="57-configuration-reference">5.7 Configuration Reference</h3>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    # System Prompt Cache
    system_prompt_cache: true

    # OR Incremental Message Cache (mutually exclusive)
    incremental_cache: true
    max_cache_sessions: 4

    # Shared settings
    cache_min_tokens: 100 # Don't cache if < 100 tokens`}</code></pre>
          <p><strong>cache_min_tokens</strong></p>
          <p>Minimum token count before caching activates. Short prompts don't benefit</p>
          <p>from caching because copy overhead exceeds prefill savings.</p>
          <p>Default: 100 tokens</p>
          <h3 id="58-context-window-auto-scaling-imc-only">5.8 Context Window Auto-Scaling (IMC Only)</h3>
          <p>When IMC is enabled, Kronk automatically scales the internal context window</p>
          <p>to ensure each slot gets the full configured context size. This auto-scaling</p>
          <p>does not apply to SPC since it only caches the system prompt (typically small).</p>
          <p><strong>Why This Is Needed:</strong></p>
          <p>IMC caches the full conversation history. The KV cache is shared across all</p>
          <p>sequences, so without auto-scaling, IMC would reduce the effective context</p>
          <p>per slot:</p>
          <pre className="code-block"><code>{`Without auto-scaling (broken):
  context-window: 128k
  IMC with 1 session → 2 sequences → 64k effective per slot ❌

With auto-scaling (Kronk's behavior):
  context-window: 128k
  IMC with 1 session → internal NCtx = 256k → 128k effective per slot ✓`}</code></pre>
          <p><strong>Formula:</strong></p>
          <pre className="code-block"><code>{`Internal NCtx = context-window × (nseq-max + max-cache-sessions)`}</code></pre>
          <p><strong>Example:</strong></p>
          <pre className="code-block"><code className="language-yaml">{`Qwen3-8B-Q8_0/IMC:
  context-window: 32768 # User wants 32k per slot
  nseq-max: 1
  incremental-cache: true
  max-cache-sessions: 2

# Internal calculation:
# total_seqs = 1 (nseq-max) + 2 (cache sessions) = 3
# Internal NCtx = 32768 × 3 = 98304
# Each slot gets full 32k context ✓`}</code></pre>
          <p><strong>VRAM Impact:</strong></p>
          <p>Auto-scaling increases KV cache memory proportionally. Plan VRAM accordingly:</p>
          <pre className="code-block"><code>{`32k context, IMC with 2 sessions, F16 cache:
  Internal NCtx = 32k × 3 = 96k
  KV cache = ~2.4 GB (instead of 800 MB without caching)`}</code></pre>
          <h3 id="59-performance-and-limitations">5.9 Performance and Limitations</h3>
          <p><strong>Prefill Time Savings:</strong></p>
          <p>For a 2000-token cached prefix:</p>
          <ul>
            <li>Without cache: ~200ms prefill (varies by hardware)</li>
            <li>With cache: ~5ms copy + ~20ms for new tokens</li>
          </ul>
          <p><strong>Memory Overhead:</strong></p>
          <p>Each cache sequence requires one context window worth of KV cache memory:</p>
          <pre className="code-block"><code>{`8K context, F16 cache:    ~200 MB per cache sequence
8K context, Q8_0 cache:   ~100 MB per cache sequence
32K context, F16 cache:   ~800 MB per cache sequence`}</code></pre>
          <p><strong>IMC Limitations:</strong></p>
          <ul>
            <li>Text-only requests (vision/audio models use sequential path)</li>
            <li>Requires deterministic Jinja templates (no timestamps or random values)</li>
            <li>Conversations must grow monotonically (append-only)</li>
            <li>Editing earlier messages triggers full cache rebuild</li>
            <li>When all <code>max_cache_sessions</code> slots are in use, new sessions bypass IMC</li>
          </ul>
          <hr />
          <h2 id="chapter-6:-yarn-extended-context">Chapter 6: YaRN Extended Context</h2>
          <p>YaRN (Yet another RoPE extensioN) allows models to handle context windows</p>
          <p>beyond their native training length. This is essential for long documents,</p>
          <p>extended conversations, and complex agentic workflows.</p>
          <h3 id="61-understanding-context-extension">6.1 Understanding Context Extension</h3>
          <p>Language models are trained with a fixed context length (e.g., 8K, 32K tokens).</p>
          <p>RoPE (Rotary Position Embedding) encodes position information, but naive</p>
          <p>extension beyond training length causes quality degradation.</p>
          <p>YaRN applies frequency-dependent interpolation with attention scaling to</p>
          <p>maintain quality at extended lengths.</p>
          <pre className="code-block"><code>{`Native Context:     32K tokens (training length)
Extended Context:   131K tokens (4x extension with YaRN)`}</code></pre>
          <h3 id="62-when-to-use-yarn">6.2 When to Use YaRN</h3>
          <p><strong>Good candidates for YaRN:</strong></p>
          <ul>
            <li>Qwen3 models (trained at 32K, support 131K with YaRN)</li>
            <li>Llama models with RoPE scaling support</li>
            <li>Any model where you need 2-4x the native context</li>
          </ul>
          <p><strong>When NOT to use YaRN:</strong></p>
          <ul>
            <li>If native context is sufficient for your use case</li>
            <li>Extensions beyond 4x (quality degrades significantly)</li>
            <li>Models without RoPE (older architectures)</li>
          </ul>
          <h3 id="63-configuration">6.3 Configuration</h3>
          <p><strong>Basic YaRN Setup:</strong></p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    context_window: 131072 # Extended context (131K)
    rope_scaling: yarn # Enable YaRN`}</code></pre>
          <p>That's often all you need—Kronk auto-calculates the other YaRN parameters</p>
          <p>from the context extension ratio.</p>
          <p><strong>Full Configuration (Advanced):</strong></p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    context_window: 131072
    rope_scaling: yarn
    rope_freq_base: 1000000 # Model-specific (Qwen3 uses 1M)
    rope_freq_scale: null # Auto-calculate
    yarn_ext_factor: null # Auto-calculate
    yarn_attn_factor: 1.0 # Attention scaling
    yarn_beta_fast: 32.0 # Low correction dimension
    yarn_beta_slow: 1.0 # High correction dimension
    yarn_orig_ctx: 32768 # Original training context`}</code></pre>
          <h3 id="64-scaling-types">6.4 Scaling Types</h3>
          <p>Kronk supports three RoPE scaling methods:</p>
          <p><strong>None (Default)</strong></p>
          <pre className="code-block"><code className="language-yaml">{`rope_scaling: none`}</code></pre>
          <p>Uses native context length. No scaling applied.</p>
          <p><strong>Linear</strong></p>
          <pre className="code-block"><code className="language-yaml">{`rope_scaling: linear`}</code></pre>
          <p>Simple linear interpolation. Works but quality degrades faster than YaRN</p>
          <p>at high extension ratios.</p>
          <p><strong>YaRN (Recommended)</strong></p>
          <pre className="code-block"><code className="language-yaml">{`rope_scaling: yarn`}</code></pre>
          <p>Frequency-dependent interpolation with attention scaling. Maintains quality</p>
          <p>better at 2-4x extensions.</p>
          <h3 id="65-parameter-reference">6.5 Parameter Reference</h3>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Parameter</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>rope_scaling</code></td>
                <td>none</td>
                <td>Scaling method: <code>none</code>, <code>linear</code>, <code>yarn</code></td>
              </tr>
              <tr>
                <td><code>rope_freq_base</code></td>
                <td>model default</td>
                <td>Base frequency (10000 for Llama, 1000000 for Qwen3)</td>
              </tr>
              <tr>
                <td><code>rope_freq_scale</code></td>
                <td>auto</td>
                <td>Frequency scaling factor</td>
              </tr>
              <tr>
                <td><code>yarn_ext_factor</code></td>
                <td>auto</td>
                <td>Extrapolation mix factor (0 = disable)</td>
              </tr>
              <tr>
                <td><code>yarn_attn_factor</code></td>
                <td>1.0</td>
                <td>Attention magnitude scaling</td>
              </tr>
              <tr>
                <td><code>yarn_beta_fast</code></td>
                <td>32.0</td>
                <td>Low correction dimension</td>
              </tr>
              <tr>
                <td><code>yarn_beta_slow</code></td>
                <td>1.0</td>
                <td>High correction dimension</td>
              </tr>
              <tr>
                <td><code>yarn_orig_ctx</code></td>
                <td>model metadata</td>
                <td>Original training context size</td>
              </tr>
            </tbody>
          </table>
          <h3 id="66-model-specific-examples">6.6 Model-Specific Examples</h3>
          <p><strong>Qwen3 (32K → 131K)</strong></p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    context_window: 131072
    rope_scaling: yarn`}</code></pre>
          <p>Qwen3 models are specifically designed to support 131K context with YaRN.</p>
          <p>The default parameters work well.</p>
          <p><strong>Llama 3 (8K → 32K)</strong></p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Llama-3-8B-Q8_0:
    context_window: 32768
    rope_scaling: yarn
    rope_freq_base: 10000`}</code></pre>
          <p>4x extension from 8K to 32K is within the recommended range.</p>
          <h3 id="67-memory-impact">6.7 Memory Impact</h3>
          <p>Extended context significantly increases memory requirements:</p>
          <pre className="code-block"><code>{`Qwen3-8B with F16 KV cache:

32K context:   ~1.6 GB KV cache
64K context:   ~3.2 GB KV cache
131K context:  ~6.5 GB KV cache`}</code></pre>
          <p><strong>Mitigation strategies:</strong></p>
          <ol>
            <li>Use KV cache quantization:</li>
          </ol>
          <pre className="code-block"><code className="language-yaml">{`cache_type_k: q8_0
cache_type_v: q8_0`}</code></pre>
          <ol>
            <li>Reduce batch parallelism:</li>
          </ol>
          <pre className="code-block"><code className="language-yaml">{`n_seq_max: 1 # Fewer concurrent requests`}</code></pre>
          <ol>
            <li>Keep KV cache on CPU (slower but saves VRAM):</li>
          </ol>
          <pre className="code-block"><code className="language-yaml">{`offload_kqv: false`}</code></pre>
          <h3 id="68-quality-considerations">6.8 Quality Considerations</h3>
          <p><strong>Extension ratio guidelines:</strong></p>
          <ul>
            <li>2x extension: Minimal quality loss</li>
            <li>3x extension: Slight degradation, usually acceptable</li>
            <li>4x extension: Noticeable but often usable</li>
            <li>&gt; 4x extension: Not recommended</li>
          </ul>
          <p><strong>Testing your configuration:</strong></p>
          <ol>
            <li>Start with a known-good prompt at native context</li>
            <li>Extend to your target length</li>
            <li>Compare output quality</li>
            <li>Adjust if needed (reduce extension or try different parameters)</li>
          </ol>
          <h3 id="69-example:-long-document-processing">6.9 Example: Long Document Processing</h3>
          <p>Configuration for processing long documents:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    context_window: 65536 # 64K context
    rope_scaling: yarn
    n_batch: 4096 # Larger batch for long prompts
    n_ubatch: 1024
    cache_type_k: q8_0
    cache_type_v: q8_0
    n_seq_max: 1 # Single request (memory intensive)`}</code></pre>
          <p>This configuration can process documents up to ~50K tokens while leaving</p>
          <p>room for generation.</p>
          <hr />
          <h2 id="chapter-7:-model-server">Chapter 7: Model Server</h2>
          <p>The Kronk Model Server provides an OpenAI-compatible REST API for inference.</p>
          <p>This chapter covers server configuration, management, and the catalog system.</p>
          <p><strong>CLI Modes: Web vs Local</strong></p>
          <p>Most CLI commands communicate with a running server by default:</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list              # Talks to server at localhost:8080
kronk catalog pull Qwen3-8B-Q8_0  # Downloads via server`}</code></pre>
          <p>Add <code>--local</code> to run commands directly without a server:</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list --local        # Direct file access
kronk catalog pull Qwen3-8B-Q8_0 --local
kronk libs --local`}</code></pre>
          <p>Use <code>--local</code> when:</p>
          <ul>
            <li>The server isn't running yet</li>
            <li>You're setting up on the same machine where the server will run</li>
            <li>You prefer direct file operations</li>
          </ul>
          <p>Use web mode (no flag) when:</p>
          <ul>
            <li>The server is running</li>
            <li>You want progress streaming in the BUI</li>
            <li>You're managing a remote server via <code>KRONK_WEB_API_HOST</code></li>
          </ul>
          <p><strong>Environment Variables</strong></p>
          <p>Every command-line flag has a corresponding environment variable. The naming</p>
          <p>convention is <code>KRONK_</code> followed by the flag name in uppercase with hyphens</p>
          <p>replaced by underscores:</p>
          <pre className="code-block"><code>{`--api-host        →  KRONK_WEB_API_HOST
--models-in-cache →  KRONK_MODELS_IN_CACHE
--cache-ttl       →  KRONK_CACHE_TTL
--processor       →  KRONK_PROCESSOR
--hf-token        →  KRONK_HF_TOKEN`}</code></pre>
          <p>Environment variables are useful for:</p>
          <ul>
            <li>Configuration in Docker/Kubernetes deployments</li>
            <li>Setting defaults without repeating flags</li>
            <li>Keeping secrets out of command history</li>
          </ul>
          <h3 id="71-starting-the-server">7.1 Starting the Server</h3>
          <p><strong>Install the CLI</strong> (if not already installed)</p>
          <pre className="code-block"><code className="language-shell">{`go install github.com/ardanlabs/kronk/cmd/kronk@latest`}</code></pre>
          <p><strong>Basic Start</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start`}</code></pre>
          <p>The server starts on <code>http://localhost:8080</code> by default.</p>
          <p><strong>Background Mode</strong></p>
          <p>Run the server as a background process:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start -d`}</code></pre>
          <p><strong>Custom Host/Port</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --api-host=0.0.0.0:9000`}</code></pre>
          <h3 id="72-stopping-the-server">7.2 Stopping the Server</h3>
          <pre className="code-block"><code className="language-shell">{`kronk server stop`}</code></pre>
          <h3 id="73-server-configuration">7.3 Server Configuration</h3>
          <p>Configuration can be set via command-line flags or environment variables.</p>
          <p><strong>Web Settings</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --api-host=localhost:8080 \\
  --debug-host=localhost:8090 \\
  --read-timeout=30s \\
  --write-timeout=15m \\
  --idle-timeout=1m \\
  --shutdown-timeout=1m \\
  --cors-allowed-origins=http://localhost:3000`}</code></pre>
          <p><strong>Environment Variables</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Variable</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>KRONK_WEB_API_HOST</code></td>
                <td>API host address (default: localhost:8080)</td>
              </tr>
              <tr>
                <td><code>KRONK_WEB_DEBUG_HOST</code></td>
                <td>Debug host address</td>
              </tr>
              <tr>
                <td><code>KRONK_WEB_READ_TIMEOUT</code></td>
                <td>HTTP read timeout</td>
              </tr>
              <tr>
                <td><code>KRONK_WEB_WRITE_TIMEOUT</code></td>
                <td>HTTP write timeout</td>
              </tr>
            </tbody>
          </table>
          <h3 id="74-model-caching">7.4 Model Caching</h3>
          <p>The server maintains a pool of loaded models to avoid reload latency.</p>
          <p><strong>Configuration</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --models-in-cache=3 \\
  --cache-ttl=5m`}</code></pre>
          <ul>
            <li><code>models-in-cache</code> - Maximum models kept loaded (default: 3)</li>
            <li><code>cache-ttl</code> - How long unused models stay loaded (default: 5m)</li>
          </ul>
          <p>When a new model is requested and the cache is full, the least recently</p>
          <p>used model is unloaded.</p>
          <h3 id="75-model-config-files">7.5 Model Config Files</h3>
          <p>Create a YAML file to configure model-specific settings:</p>
          <pre className="code-block"><code className="language-yaml">{`# model-config.yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 32768
    n_seq_max: 4
    cache_type_k: q8_0
    cache_type_v: q8_0
    system_prompt_cache: true

  Llama-3.3-70B-Instruct-Q8_0:
    context_window: 8192
    n_gpu_layers: 0
    split_mode: row

  embeddinggemma-300m-qat-Q8_0:
    n_seq_max: 2`}</code></pre>
          <p>Start with the config file:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --model-config-file=model-config.yaml`}</code></pre>
          <p>Or via environment variable:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_CATALOG_MODEL_CONFIG_FILE=/path/to/model-config.yaml
kronk server start`}</code></pre>
          <p><strong>Project Reference Configuration</strong></p>
          <p>The Kronk repository includes a comprehensive reference configuration with</p>
          <p>recommended settings for various models and use cases:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_CATALOG_MODEL_CONFIG_FILE=<clone_path>/zarf/kms/model_config.yaml
kronk server start`}</code></pre>
          <p>This file includes:</p>
          <ul>
            <li>Optimized configurations for coding agents (Cline, OpenCode)</li>
            <li>YaRN extended context examples</li>
            <li>SPC and IMC variants for different caching strategies</li>
            <li>Vision and audio model settings</li>
            <li>Detailed comments explaining each configuration option</li>
          </ul>
          <p>Review <code>zarf/kms/model_config.yaml</code> for examples of YAML anchors, cache</p>
          <p>configurations, and model-specific tuning.</p>
          <h3 id="76-catalog-system">7.6 Catalog System</h3>
          <p>The catalog provides a curated list of verified models with preconfigured</p>
          <p>settings.</p>
          <p><strong>List Available Models</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list`}</code></pre>
          <p>Output:</p>
          <pre className="code-block"><code>{`CATALOG              MODEL ID                         PULLED  ENDPOINT
Audio-Text-to-Text   Qwen2-Audio-7B.Q8_0              no      chat_completion
Embedding            embeddinggemma-300m-qat-Q8_0     no      embeddings
Image-Text-to-Text   gemma-3-4b-it-q4_0               no      chat_completion
Text-Generation      Qwen3-8B-Q8_0                    yes     chat_completion
Text-Generation      Llama-3.3-70B-Instruct-Q8_0      no      chat_completion`}</code></pre>
          <p><strong>Filter by Category</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list --filter-category=Embedding`}</code></pre>
          <p><strong>Pull a Model</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog pull Qwen3-8B-Q8_0`}</code></pre>
          <p><strong>Show Model Details</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog show Qwen3-8B-Q8_0`}</code></pre>
          <p><strong>Update Catalog</strong></p>
          <p>_Note: We don't have a server version of this yet._</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog update --local`}</code></pre>
          <h3 id="77-custom-catalog-repository">7.7 Custom Catalog Repository</h3>
          <p>Use a custom catalog repository:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --catalog-github-repo=https://github.com/myorg/my-catalog`}</code></pre>
          <h3 id="78-templates">7.8 Templates</h3>
          <p>Templates define chat formatting (Jinja templates) for different models.</p>
          <p>Kronk downloads templates automatically from the offical templates repository.</p>
          <p>https://github.com/ardanlabs/kronk_catalogs</p>
          <p>You don't need this unless you want to maintain your own repository.</p>
          <p><strong>Custom Templates Repository</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --templates-github-repo=https://github.com/myorg/my-templates`}</code></pre>
          <p>Templates are cached in <code>~/.kronk/templates/</code> by default.</p>
          <h3 id="79-runtime-settings">7.9 Runtime Settings</h3>
          <p><strong>Processor Selection</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --processor=cuda    # NVIDIA GPU
kronk server start --processor=metal   # Apple Silicon
kronk server start --processor=vulkan  # Cross-platform GPU
kronk server start --processor=cpu     # CPU only`}</code></pre>
          <p><strong>Library Path</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --lib-path=/custom/path/to/libraries \\
  --lib-version=b7406`}</code></pre>
          <p><strong>Hugging Face Token</strong></p>
          <p>For gated models requiring authentication:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --hf-token=hf_xxxxx`}</code></pre>
          <p>Or via environment variable:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_HF_TOKEN=hf_xxxxx
kronk server start`}</code></pre>
          <h3 id="710-logging">7.10 Logging</h3>
          <p><strong>llama.cpp Logging</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --llama-log=1    # Enable llama.cpp logs
kronk server start --llama-log=0    # Disable (default)`}</code></pre>
          <p><strong>Insecure Logging</strong></p>
          <p>Enable logging of message content (for debugging only):</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --insecure-logging=true`}</code></pre>
          <p><strong>Warning:</strong> This logs sensitive data. Never use in production.</p>
          <p><strong>View Server Logs</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server logs`}</code></pre>
          <h3 id="711-data-paths">7.11 Data Paths</h3>
          <p>Default data locations:</p>
          <pre className="code-block"><code>{`~/.kronk/
├── libraries/     # llama.cpp libraries
├── models/        # Downloaded models
├── templates/     # Chat templates
└── catalog/       # Catalog cache`}</code></pre>
          <p><strong>Custom Base Path</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --base-path=/data/kronk`}</code></pre>
          <h3 id="712-complete-example">7.12 Complete Example</h3>
          <p>Production-ready server configuration:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --api-host=0.0.0.0:8080 \\
  --models-in-cache=2 \\
  --cache-ttl=10m \\
  --model-config-file=/etc/kronk/models.yaml \\
  --processor=cuda \\
  --auth-enabled=true \\
  -d`}</code></pre>
          <p>With model config:</p>
          <pre className="code-block"><code className="language-yaml">{`# /etc/kronk/models.yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 32768
    n_seq_max: 4
    cache_type_k: q8_0
    cache_type_v: q8_0
    incremental_cache: true
    max_cache_sessions: 8`}</code></pre>
          <hr />
          <h2 id="chapter-8:-api-endpoints">Chapter 8: API Endpoints</h2>
          <p>Kronk provides an OpenAI-compatible REST API. This chapter documents the</p>
          <p>available endpoints and their usage.</p>
          <h3 id="81-endpoint-overview">8.1 Endpoint Overview</h3>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Endpoint</th>
                <th>Method</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>/v1/chat/completions</code></td>
                <td>POST</td>
                <td>Chat completions (streaming/non-streaming)</td>
              </tr>
              <tr>
                <td><code>/v1/responses</code></td>
                <td>POST</td>
                <td>OpenAI Responses API format</td>
              </tr>
              <tr>
                <td><code>/v1/messages</code></td>
                <td>POST</td>
                <td>Anthropic API format</td>
              </tr>
              <tr>
                <td><code>/v1/embeddings</code></td>
                <td>POST</td>
                <td>Generate embeddings</td>
              </tr>
              <tr>
                <td><code>/v1/rerank</code></td>
                <td>POST</td>
                <td>Rerank documents</td>
              </tr>
              <tr>
                <td><code>/v1/models</code></td>
                <td>GET</td>
                <td>List available models</td>
              </tr>
            </tbody>
          </table>
          <h3 id="82-chat-completions">8.2 Chat Completions</h3>
          <p>Generate chat responses using the familiar OpenAI format.</p>
          <p><strong>Endpoint:</strong> <code>POST /v1/chat/completions</code></p>
          <p><strong>Basic Request:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "What is the capital of France?"}
    ]
  }'`}</code></pre>
          <p><strong>Request Parameters:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen3-8B-Q8_0",
  "messages": [
    { "role": "system", "content": "System prompt" },
    { "role": "user", "content": "User message" },
    { "role": "assistant", "content": "Previous response" },
    { "role": "user", "content": "Follow-up question" }
  ],
  "temperature": 0.8,
  "top_p": 0.9,
  "top_k": 40,
  "max_tokens": 2048,
  "stream": true
}`}</code></pre>
          <p><strong>Streaming Response:</strong></p>
          <p>With <code>"stream": true</code>, responses are sent as Server-Sent Events:</p>
          <pre className="code-block"><code>{`data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk",...}

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk",...}

data: [DONE]`}</code></pre>
          <p><strong>Non-Streaming Response:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "Qwen3-8B-Q8_0",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "The capital of France is Paris."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 25,
    "completion_tokens": 8,
    "total_tokens": 33
  }
}`}</code></pre>
          <p><strong>Reasoning Models:</strong></p>
          <p>For models with thinking/reasoning support (like Qwen3):</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen3-8B-Q8_0",
  "messages": [...],
  "enable_thinking": true
}`}</code></pre>
          <p>The response includes <code>reasoning_content</code> in the message.</p>
          <p>To disable thinking:</p>
          <pre className="code-block"><code className="language-json">{`{
  "enable_thinking": false
}`}</code></pre>
          <h3 id="83-responses-api">8.3 Responses API</h3>
          <p>OpenAI's newer Responses API format, used by some clients.</p>
          <p><strong>Endpoint:</strong> <code>POST /v1/responses</code></p>
          <p><strong>Request:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/responses \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "input": "Explain quantum computing in simple terms."
  }'`}</code></pre>
          <p>The <code>input</code> field can be a string or an array of message objects.</p>
          <p><strong>Streaming Events:</strong></p>
          <p>The Responses API uses a different event format:</p>
          <pre className="code-block"><code>{`event: response.created
data: {"type":"response.created",...}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"The",...}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":" answer",...}

event: response.completed
data: {"type":"response.completed",...}`}</code></pre>
          <h3 id="84-embeddings">8.4 Embeddings</h3>
          <p>Generate vector embeddings for text.</p>
          <p><strong>Endpoint:</strong> <code>POST /v1/embeddings</code></p>
          <p><strong>Request:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/embeddings \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "embeddinggemma-300m-qat-Q8_0",
    "input": "The quick brown fox jumps over the lazy dog."
  }'`}</code></pre>
          <p><strong>Multiple Inputs:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "embeddinggemma-300m-qat-Q8_0",
  "input": [
    "First document to embed.",
    "Second document to embed.",
    "Third document to embed."
  ]
}`}</code></pre>
          <p><strong>Response:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [0.123, -0.456, 0.789, ...]
    }
  ],
  "model": "embeddinggemma-300m-qat-Q8_0",
  "usage": {
    "prompt_tokens": 10,
    "total_tokens": 10
  }
}`}</code></pre>
          <h3 id="85-reranking">8.5 Reranking</h3>
          <p>Score and reorder documents by relevance to a query.</p>
          <p><strong>Endpoint:</strong> <code>POST /v1/rerank</code></p>
          <p><strong>Request:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/rerank \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "bge-reranker-v2-m3-Q8_0",
    "query": "What is machine learning?",
    "documents": [
      "Machine learning is a subset of artificial intelligence.",
      "The weather today is sunny.",
      "Deep learning uses neural networks.",
      "I like pizza."
    ],
    "top_n": 2
  }'`}</code></pre>
          <p><strong>Response:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "object": "list",
  "results": [
    {
      "index": 0,
      "relevance_score": 0.95,
      "document": "Machine learning is a subset of artificial intelligence."
    },
    {
      "index": 2,
      "relevance_score": 0.82,
      "document": "Deep learning uses neural networks."
    }
  ],
  "model": "bge-reranker-v2-m3-Q8_0",
  "usage": {
    "prompt_tokens": 45,
    "total_tokens": 45
  }
}`}</code></pre>
          <h3 id="86-tool-calling-function-calling">8.6 Tool Calling (Function Calling)</h3>
          <p>Kronk supports OpenAI-compatible tool calling, allowing models to request</p>
          <p>function executions that you handle in your application.</p>
          <p><strong>Request with Tools:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [
      {"role": "user", "content": "What is the weather in Paris?"}
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get current weather for a location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {
                "type": "string",
                "description": "City name"
              }
            },
            "required": ["location"]
          }
        }
      }
    ],
    "tool_choice": "auto"
  }'`}</code></pre>
          <p><strong>Tool Choice Options:</strong></p>
          <ul>
            <li><code>"auto"</code> - Model decides whether to call tools (default)</li>
            <li><code>"none"</code> - Never call tools</li>
            <li><code>&#123;"type": "function", "function": &#123;"name": "get_weather"&#125;&#125;</code> - Force specific tool</li>
          </ul>
          <p><strong>Response with Tool Calls:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "id": "chatcmpl-xxx",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": null,
        "tool_calls": [
          {
            "id": "call_abc123",
            "type": "function",
            "function": {
              "name": "get_weather",
              "arguments": "{\\"location\\": \\"Paris\\"}"
            }
          }
        ]
      },
      "finish_reason": "tool_calls"
    }
  ]
}`}</code></pre>
          <p><strong>Handling Tool Results:</strong></p>
          <p>After executing the tool, send the result back:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen3-8B-Q8_0",
  "messages": [
    { "role": "user", "content": "What is the weather in Paris?" },
    {
      "role": "assistant",
      "content": null,
      "tool_calls": [
        {
          "id": "call_abc123",
          "type": "function",
          "function": {
            "name": "get_weather",
            "arguments": "{\\"location\\": \\"Paris\\"}"
          }
        }
      ]
    },
    {
      "role": "tool",
      "tool_call_id": "call_abc123",
      "content": "{\\"temperature\\": 18, \\"condition\\": \\"sunny\\"}"
    }
  ]
}`}</code></pre>
          <p><strong>Streaming with Tool Calls:</strong></p>
          <p>Tool call arguments stream incrementally:</p>
          <pre className="code-block"><code>{`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\\"loc"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ation\\":"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":" \\"Paris\\"}"}}]}}]}`}</code></pre>
          <h3 id="87-logprobs-token-probabilities">8.7 Logprobs (Token Probabilities)</h3>
          <p>Request log probabilities for generated tokens to understand model confidence</p>
          <p>or implement custom sampling strategies.</p>
          <p><strong>Request Parameters:</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Parameter</th>
                <th>Type</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>logprobs</code></td>
                <td>bool</td>
                <td>false</td>
                <td>Return log probability for each token</td>
              </tr>
              <tr>
                <td><code>top_logprobs</code></td>
                <td>int</td>
                <td>0</td>
                <td>Number of top alternatives (0-5)</td>
              </tr>
            </tbody>
          </table>
          <p>Setting <code>top_logprobs &gt; 0</code> implicitly enables <code>logprobs</code>.</p>
          <p><strong>Request:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [
      {"role": "user", "content": "What is 2+2?"}
    ],
    "logprobs": true,
    "top_logprobs": 3,
    "max_tokens": 10
  }'`}</code></pre>
          <p><strong>Response with Logprobs:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "id": "chatcmpl-xxx",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "4"
      },
      "logprobs": {
        "content": [
          {
            "token": "4",
            "logprob": -0.0012,
            "bytes": [52],
            "top_logprobs": [
              { "token": "4", "logprob": -0.0012, "bytes": [52] },
              { "token": "The", "logprob": -6.82, "bytes": [84, 104, 101] },
              {
                "token": "Four",
                "logprob": -7.15,
                "bytes": [70, 111, 117, 114]
              }
            ]
          }
        ]
      },
      "finish_reason": "stop"
    }
  ]
}`}</code></pre>
          <p><strong>Response Structure:</strong></p>
          <ul>
            <li><code>logprobs.content[]</code> - Array of per-token probability data</li>
            <li><code>token</code> - The generated token string</li>
            <li><code>logprob</code> - Log probability (always ≤ 0; closer to 0 = higher confidence)</li>
            <li><code>bytes</code> - UTF-8 byte representation of the token</li>
            <li><code>top_logprobs[]</code> - Alternative tokens with their probabilities</li>
          </ul>
          <p><strong>Streaming Behavior:</strong></p>
          <ul>
            <li><strong>Streaming</strong>: Logprobs sent in each delta chunk</li>
            <li><strong>Non-streaming</strong>: All logprobs in final response</li>
          </ul>
          <p><strong>Use Cases:</strong></p>
          <ul>
            <li>Confidence scoring for model outputs</li>
            <li>Detecting hallucinations (low probability sequences)</li>
            <li>Custom rejection sampling</li>
            <li>Token-level analysis for debugging</li>
          </ul>
          <h3 id="88-models-list">8.8 Models List</h3>
          <p>Get available models.</p>
          <p><strong>Endpoint:</strong> <code>GET /v1/models</code></p>
          <p><strong>Request:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/models`}</code></pre>
          <p><strong>Response:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "object": "list",
  "data": [
    {
      "id": "Qwen3-8B-Q8_0",
      "object": "model",
      "owned_by": "kronk"
    },
    {
      "id": "embeddinggemma-300m-qat-Q8_0",
      "object": "model",
      "owned_by": "kronk"
    }
  ]
}`}</code></pre>
          <h3 id="89-using-cache-id-with-api-requests">8.9 Using Cache ID with API Requests</h3>
          <p>To use multi-user caching (SPC or IMC), pass the session ID via header:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "KRONK_CACHE_ID: user-123" \\
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [...]
  }'`}</code></pre>
          <p>Or in the request body:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen3-8B-Q8_0",
  "cache_id": "user-123",
  "messages": [...]
}`}</code></pre>
          <p>The <code>cache_id</code> is used by both System Prompt Cache (SPC) and Incremental Message Cache (IMC).</p>
          <p>Each unique <code>cache_id</code> gets its own dedicated cache sequence, up to <code>max_cache_sessions</code>.</p>
          <h3 id="810-authentication">8.10 Authentication</h3>
          <p>When authentication is enabled, include the token in requests:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer your-token-here" \\
  -d '{...}'`}</code></pre>
          <p>See <a href="#chapter-10-security--authentication">Chapter 10: Security & Authentication</a></p>
          <p>for details on token management.</p>
          <h3 id="811-error-responses">8.11 Error Responses</h3>
          <p>Errors follow a standard format:</p>
          <pre className="code-block"><code className="language-json">{`{
  "error": {
    "code": "invalid_argument",
    "message": "missing model field"
  }
}`}</code></pre>
          <p><strong>Common Error Codes:</strong></p>
          <ul>
            <li><code>invalid_argument</code> - Missing or invalid request parameters</li>
            <li><code>not_found</code> - Model not found</li>
            <li><code>internal</code> - Server error during processing</li>
            <li><code>unauthenticated</code> - Missing or invalid authentication token</li>
          </ul>
          <hr />
          <h2 id="chapter-9:-multi-modal-models">Chapter 9: Multi-Modal Models</h2>
          <p>Kronk supports vision and audio models that can process images, video frames,</p>
          <p>and audio alongside text. This chapter covers how to use these models.</p>
          <h3 id="91-overview">9.1 Overview</h3>
          <p>Multi-modal models combine a language model with a media projector that</p>
          <p>converts images or audio into tokens the model can understand.</p>
          <p><strong>Supported Media Types:</strong></p>
          <ul>
            <li><strong>Vision</strong>: JPEG, PNG, GIF images</li>
            <li><strong>Audio</strong>: WAV audio files</li>
          </ul>
          <p><strong>Available Models (from catalog):</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list --filter-category=Image
kronk catalog list --filter-category=Audio`}</code></pre>
          <p>Example models:</p>
          <ul>
            <li><code>Qwen2.5-VL-3B-Instruct-Q8_0</code> - Vision model</li>
            <li><code>gemma-3-4b-it-q4_0</code> - Vision model</li>
            <li><code>Qwen2-Audio-7B.Q8_0</code> - Audio model</li>
          </ul>
          <h3 id="92-vision-models">9.2 Vision Models</h3>
          <p>Vision models analyze images and answer questions about their content.</p>
          <p><strong>Download a Vision Model:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog pull Qwen2.5-VL-3B-Instruct-Q8_0`}</code></pre>
          <p><strong>API Request with Image (OpenAI Format):</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen2.5-VL-3B-Instruct-Q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "text",
            "text": "What do you see in this image?"
          },
          {
            "type": "image_url",
            "image_url": {
              "url": "data:image/jpeg;base64,/9j/4AAQSkZJRg..."
            }
          }
        ]
      }
    ]
  }'`}</code></pre>
          <p><strong>Content Array Structure:</strong></p>
          <p>The <code>content</code> field is an array of content parts:</p>
          <pre className="code-block"><code className="language-json">{`{
  "content": [
    { "type": "text", "text": "Describe this image" },
    {
      "type": "image_url",
      "image_url": { "url": "data:image/jpeg;base64,..." }
    }
  ]
}`}</code></pre>
          <p><strong>Supported image_url Formats:</strong></p>
          <ul>
            <li>Base64 data URL: <code>data:image/jpeg;base64,/9j/4AAQSkZJRg...</code></li>
            <li>Base64 data URL: <code>data:image/png;base64,iVBORw0KGgo...</code></li>
          </ul>
          <h3 id="93-audio-models">9.3 Audio Models</h3>
          <p>Audio models transcribe and understand spoken content.</p>
          <p><strong>Download an Audio Model:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog pull Qwen2-Audio-7B.Q8_0`}</code></pre>
          <p><strong>API Request with Audio:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen2-Audio-7B.Q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "text",
            "text": "Transcribe this audio and summarize what is said."
          },
          {
            "type": "input_audio",
            "input_audio": {
              "data": "UklGRi...",
              "format": "wav"
            }
          }
        ]
      }
    ]
  }'`}</code></pre>
          <p><strong>Audio Format:</strong></p>
          <ul>
            <li><code>data</code> - Base64-encoded audio data</li>
            <li><code>format</code> - Audio format (currently <code>wav</code> supported)</li>
          </ul>
          <h3 id="94-plain-base64-format">9.4 Plain Base64 Format</h3>
          <p>For simpler integrations, Kronk also accepts plain base64 as the message</p>
          <p>content (without the structured OpenAI format):</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen2.5-VL-3B-Instruct-Q8_0",
  "messages": [
    {
      "role": "user",
      "content": "/9j/4AAQSkZJRgABAQEASABIAAD..."
    }
  ]
}`}</code></pre>
          <p>Kronk auto-detects the media type from the binary header:</p>
          <ul>
            <li>JPEG: starts with <code>FF D8 FF</code></li>
            <li>PNG: starts with <code>89 50 4E 47</code></li>
            <li>WAV: starts with <code>RIFF</code></li>
          </ul>
          <h3 id="95-configuration-for-multi-modal-models">9.5 Configuration for Multi-Modal Models</h3>
          <p>Vision and audio models have specific configuration requirements:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen2.5-VL-3B-Instruct-Q8_0:
    n_ubatch: 2048 # Higher for image token processing
    n_seq_max: 2 # Creates 2 model instances (pooled)
    context_window: 8192`}</code></pre>
          <p><strong>Key Considerations:</strong></p>
          <ul>
            <li><code>n_ubatch</code> should be high (≥2048) for efficient image/audio token processing</li>
            <li><code>n_seq_max</code> creates model instances in a pool (not batch parallelism)</li>
            <li>Each request needs exclusive model context for media embedding</li>
          </ul>
          <h3 id="96-memory-requirements">9.6 Memory Requirements</h3>
          <p>Vision and audio models require additional memory for the projector:</p>
          <p><strong>Vision Model Example (Qwen2.5-VL-3B):</strong></p>
          <pre className="code-block"><code>{`Model weights:     ~3.5 GB
Projector:         ~0.5 GB
KV cache (8K):     ~0.4 GB
─────────────────────────
Total:             ~4.4 GB`}</code></pre>
          <p><strong>Audio Model Example (Qwen2-Audio-7B):</strong></p>
          <pre className="code-block"><code>{`Model weights:     ~8 GB
Projector:         ~0.8 GB
KV cache (8K):     ~0.6 GB
─────────────────────────
Total:             ~9.4 GB`}</code></pre>
          <h3 id="97-limitations">9.7 Limitations</h3>
          <ul>
            <li>Vision/audio models cannot use batch processing (sequential only)</li>
            <li>Each request gets exclusive model context</li>
            <li>Message caching (SPC/IMC) not supported for media requests</li>
            <li>Processing time varies with image resolution and audio duration</li>
          </ul>
          <h3 id="98-example:-image-analysis">9.8 Example: Image Analysis</h3>
          <p>Complete example analyzing an image:</p>
          <pre className="code-block"><code className="language-shell">{`# Encode image to base64
IMAGE_B64=$(base64 -i photo.jpg)

# Send request
curl http://localhost:8080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen2.5-VL-3B-Instruct-Q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Describe this image in detail."},
          {
            "type": "image_url",
            "image_url": {"url": "data:image/jpeg;base64,\${IMAGE_B64}"}
          }
        ]
      }
    ],
    "max_tokens": 1024
  }'`}</code></pre>
          <h3 id="99-example:-audio-transcription">9.9 Example: Audio Transcription</h3>
          <p>Complete example transcribing audio:</p>
          <pre className="code-block"><code className="language-shell">{`# Encode audio to base64
AUDIO_B64=$(base64 -i recording.wav)

# Send request
curl http://localhost:8080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen2-Audio-7B.Q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Transcribe this audio."},
          {
            "type": "input_audio",
            "input_audio": {"data": "\${AUDIO_B64}", "format": "wav"}
          }
        ]
      }
    ],
    "max_tokens": 2048
  }'`}</code></pre>
          <hr />
          <p>_Next: <a href="#chapter-10-security--authentication">Chapter 10: Security & Authentication</a>_</p>
          <h2 id="chapter-10:-security-authentication">Chapter 10: Security &amp; Authentication</h2>
          <p>Kronk provides JWT-based authentication and authorization with per-endpoint</p>
          <p>rate limiting. When enabled, all API requests require a valid token.</p>
          <h3 id="101-enabling-authentication">10.1 Enabling Authentication</h3>
          <p><strong>Start Server with Auth Enabled:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --auth-enabled`}</code></pre>
          <p>Or via environment variable:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_AUTH_ENABLED=true
kronk server start`}</code></pre>
          <p><strong>First-Time Setup:</strong></p>
          <p>On first startup with authentication enabled, Kronk automatically:</p>
          <ol>
            <li>Creates a <code>keys/</code> directory in <code>~/.kronk/</code></li>
            <li>Generates a master private key (<code>master.pem</code>)</li>
            <li>Creates an admin token (<code>master.jwt</code>) valid for 10 years</li>
            <li>Generates an additional signing key for user tokens</li>
          </ol>
          <p>The admin token is stored at <code>~/.kronk/keys/master.jwt</code>.</p>
          <h3 id="102-using-the-admin-token">10.2 Using the Admin Token</h3>
          <p>The admin token is required for all security management operations.</p>
          <p><strong>Set the Token:</strong></p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)`}</code></pre>
          <p><strong>Admin Capabilities:</strong></p>
          <ul>
            <li>Create new tokens for users</li>
            <li>Add and remove signing keys</li>
            <li>Access all endpoints without rate limits</li>
          </ul>
          <h3 id="103-key-management">10.3 Key Management</h3>
          <p>Private keys sign JWT tokens. Multiple keys allow token rotation without</p>
          <p>invalidating all existing tokens.</p>
          <p><strong>List Keys:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk security key list`}</code></pre>
          <p>Output:</p>
          <pre className="code-block"><code>{`KEY ID                                  CREATED
master                                  2024-01-15T10:30:00Z
a1b2c3d4-e5f6-7890-abcd-ef1234567890    2024-01-15T10:30:00Z`}</code></pre>
          <p><strong>Create a New Key:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk security key create`}</code></pre>
          <p>This generates a new UUID-named key for signing tokens.</p>
          <p><strong>Delete a Key:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk security key delete --keyid a1b2c3d4-e5f6-7890-abcd-ef1234567890`}</code></pre>
          <p><strong>Important:</strong> The master key cannot be deleted. Deleting a key invalidates</p>
          <p>all tokens signed with that key.</p>
          <p><strong>Local Mode:</strong></p>
          <p>All key commands support <code>--local</code> to operate without a running server:</p>
          <pre className="code-block"><code className="language-shell">{`kronk security key list --local
kronk security key create --local
kronk security key delete --keyid <keyid> --local`}</code></pre>
          <h3 id="104-creating-user-tokens">10.4 Creating User Tokens</h3>
          <p>Create tokens with specific endpoint access and optional rate limits.</p>
          <p><strong>Basic Syntax:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk security token create \\
  --duration <duration> \\
  --endpoints <endpoint-list>`}</code></pre>
          <p><strong>Parameters:</strong></p>
          <ul>
            <li><code>--duration</code> - Token lifetime (e.g., <code>1h</code>, <code>24h</code>, <code>720h</code>, <code>8760h</code>)</li>
            <li><code>--endpoints</code> - Comma-separated list of endpoints with optional limits</li>
          </ul>
          <p><strong>Endpoint Format:</strong></p>
          <ul>
            <li><code>endpoint</code> - Unlimited access (default)</li>
            <li><code>endpoint:unlimited</code> - Unlimited access (explicit)</li>
            <li><code>endpoint:limit/window</code> - Rate limited</li>
          </ul>
          <p><strong>Rate Limit Windows:</strong></p>
          <ul>
            <li><code>day</code> - Resets daily</li>
            <li><code>month</code> - Resets monthly</li>
            <li><code>year</code> - Resets yearly</li>
          </ul>
          <p><strong>Available Endpoints:</strong></p>
          <ul>
            <li><code>chat-completions</code> - Chat completions API</li>
            <li><code>responses</code> - Responses API</li>
            <li><code>embeddings</code> - Embeddings API</li>
            <li><code>rerank</code> - Reranking API</li>
            <li><code>messages</code> - Anthropic Messages API</li>
          </ul>
          <h3 id="105-token-examples">10.5 Token Examples</h3>
          <p><strong>Unlimited Access to All Endpoints (24 hours):</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk security token create \\
  --duration 24h \\
  --endpoints chat-completions,embeddings,rerank,responses,messages`}</code></pre>
          <p><strong>Rate-Limited Chat Token (30 days):</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk security token create \\
  --duration 720h \\
  --endpoints "chat-completions:1000/day,embeddings:500/day"`}</code></pre>
          <p><strong>Monthly Quota Token:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk security token create \\
  --duration 8760h \\
  --endpoints "chat-completions:10000/month,embeddings:50000/month"`}</code></pre>
          <p><strong>Mixed Limits:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk security token create \\
  --duration 720h \\
  --endpoints "chat-completions:100/day,embeddings:unlimited"`}</code></pre>
          <p><strong>Output:</strong></p>
          <pre className="code-block"><code>{`Token create
  Duration: 720h0m0s
  Endpoints: map[chat-completions:{1000 day} embeddings:{0 unlimited}]
TOKEN:
eyJhbGciOiJSUzI1NiIsImtpZCI6ImExYjJjM2Q0Li4uIiwidHlwIjoiSldUIn0...`}</code></pre>
          <h3 id="106-using-tokens-in-api-requests">10.6 Using Tokens in API Requests</h3>
          <p>Pass the token in the <code>Authorization</code> header with the <code>Bearer</code> prefix.</p>
          <p><strong>curl Example:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer eyJhbGciOiJS..." \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [{"role": "user", "content": "Hello"}]
  }'`}</code></pre>
          <p><strong>Environment Variable Pattern:</strong></p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_TOKEN="eyJhbGciOiJS..."

curl http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{...}'`}</code></pre>
          <p><strong>Python Example:</strong></p>
          <pre className="code-block"><code className="language-python">{`import openai

client = openai.OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="eyJhbGciOiJS..."  # Your Kronk token
)

response = client.chat.completions.create(
    model="Qwen3-8B-Q8_0",
    messages=[{"role": "user", "content": "Hello"}]
)`}</code></pre>
          <h3 id="107-authorization-flow">10.7 Authorization Flow</h3>
          <p>When a request arrives:</p>
          <ol>
            <li><strong>Token Extraction</strong> - Bearer token parsed from Authorization header</li>
            <li><strong>Signature Verification</strong> - Token signature verified against known keys</li>
            <li><strong>Expiration Check</strong> - Token must not be expired</li>
            <li><strong>Endpoint Authorization</strong> - Token must include the requested endpoint</li>
            <li><strong>Rate Limit Check</strong> - Request counted against endpoint quota</li>
            <li><strong>Request Processing</strong> - If all checks pass, request proceeds</li>
          </ol>
          <p><strong>Error Responses:</strong></p>
          <ul>
            <li><code>401 Unauthorized</code> - Missing, invalid, or expired token</li>
            <li><code>403 Forbidden</code> - Token lacks access to the endpoint</li>
            <li><code>429 Too Many Requests</code> - Rate limit exceeded</li>
          </ul>
          <h3 id="108-rate-limiting">10.8 Rate Limiting</h3>
          <p>Rate limits are enforced per token (identified by the token's subject claim).</p>
          <p><strong>How Limits Work:</strong></p>
          <ul>
            <li>Each token has a unique subject (UUID)</li>
            <li>Requests are counted per endpoint per subject</li>
            <li>Counters reset at window boundaries (day/month/year)</li>
          </ul>
          <p><strong>Limit Storage:</strong></p>
          <p>Rate limit counters are stored in a BadgerDB database at <code>~/.kronk/badger/</code>.</p>
          <p>Counters persist across server restarts.</p>
          <p><strong>Bypassing Rate Limits:</strong></p>
          <p>Admin tokens (like <code>master.jwt</code>) bypass all rate limiting.</p>
          <h3 id="109-configuration-reference">10.9 Configuration Reference</h3>
          <p><strong>Server Flags:</strong></p>
          <ul>
            <li><code>--auth-enabled</code> - Enable authentication (env: <code>KRONK_AUTH_ENABLED</code>)</li>
            <li><code>--auth-issuer</code> - JWT issuer name (env: <code>KRONK_AUTH_ISSUER</code>)</li>
            <li><code>--auth-host</code> - External auth service host (env: <code>KRONK_AUTH_HOST</code>)</li>
          </ul>
          <p><strong>Environment Variables:</strong></p>
          <ul>
            <li><code>KRONK_TOKEN</code> - Token for CLI commands and API requests</li>
            <li><code>KRONK_WEB_API_HOST</code> - Server address for CLI web mode</li>
          </ul>
          <p>  (default: <code>localhost:8080</code>)</p>
          <h3 id="1010-security-best-practices">10.10 Security Best Practices</h3>
          <p><strong>Token Management:</strong></p>
          <ul>
            <li>Store admin tokens securely; treat <code>master.jwt</code> like a password</li>
            <li>Create separate tokens for different applications/users</li>
            <li>Use short durations for development tokens</li>
            <li>Rotate keys periodically for production deployments</li>
          </ul>
          <p><strong>Rate Limiting:</strong></p>
          <ul>
            <li>Set appropriate limits based on expected usage</li>
            <li>Use daily limits for interactive applications</li>
            <li>Use monthly limits for batch processing</li>
          </ul>
          <p><strong>Key Rotation:</strong></p>
          <ol>
            <li>Create a new key: <code>kronk security key create</code></li>
            <li>Issue new tokens using the new key</li>
            <li>Wait for old tokens to expire</li>
            <li>Delete the old key: <code>kronk security key delete --keyid &lt;old-keyid&gt;</code></li>
          </ol>
          <p><strong>Production Checklist:</strong></p>
          <ul>
            <li>Enable authentication: <code>--auth-enabled</code></li>
            <li>Secure the <code>~/.kronk/keys/</code> directory (mode 0700)</li>
            <li>Back up <code>master.pem</code> and <code>master.jwt</code> securely</li>
            <li>Distribute user tokens, never the admin token</li>
            <li>Monitor rate limit usage in logs</li>
          </ul>
          <hr />
          <p>_Next: <a href="#chapter-11-browser-ui-bui">Chapter 11: Browser UI (BUI)</a>_</p>
          <h2 id="chapter-11:-browser-ui-bui">Chapter 11: Browser UI (BUI)</h2>
          <p>Kronk includes a web-based interface for managing models, libraries,</p>
          <p>security, and server configuration without using the command line.</p>
          <h3 id="111-accessing-the-bui">11.1 Accessing the BUI</h3>
          <p>The BUI is served from the same port as the API.</p>
          <p><strong>Open in Browser:</strong></p>
          <pre className="code-block"><code>{`http://localhost:8080`}</code></pre>
          <p>The BUI automatically loads when you navigate to the server root.</p>
          <h3 id="112-downloading-libraries">11.2 Downloading Libraries</h3>
          <p>Before running inference, you need the llama.cpp libraries.</p>
          <p><strong>Steps:</strong></p>
          <ol>
            <li>Navigate to the <strong>Libraries</strong> page from the menu</li>
            <li>Click <strong>Pull Libraries</strong></li>
            <li>Wait for the download to complete</li>
          </ol>
          <p>The BUI auto-detects your platform (OS, architecture, GPU) and downloads</p>
          <p>the appropriate binaries to <code>~/.kronk/libraries/</code>.</p>
          <p><strong>Override Detection:</strong></p>
          <p>If auto-detection is incorrect, you can specify:</p>
          <ul>
            <li>Processor type (CPU, CUDA, Metal, Vulkan)</li>
            <li>Architecture (amd64, arm64)</li>
            <li>Operating system</li>
          </ul>
          <h3 id="113-downloading-models">11.3 Downloading Models</h3>
          <p><strong>Browse the Catalog:</strong></p>
          <ol>
            <li>Navigate to the <strong>Catalog</strong> page</li>
            <li>Browse available models by category:</li>
          </ol>
          <p>   - Text-Generation</p>
          <p>   - Image-Text-to-Text (Vision)</p>
          <p>   - Audio-Text-to-Text</p>
          <p>   - Embedding</p>
          <p>   - Reranking</p>
          <ol>
            <li>Click <strong>Pull</strong> next to a model to download it</li>
          </ol>
          <p><strong>Monitor Progress:</strong></p>
          <p>The BUI shows real-time download progress including:</p>
          <ul>
            <li>Download percentage</li>
            <li>Transfer speed</li>
            <li>Estimated time remaining</li>
          </ul>
          <p><strong>View Pulled Models:</strong></p>
          <p>Navigate to the <strong>Models</strong> page to see all downloaded models and their</p>
          <p>status.</p>
          <h3 id="114-managing-keys-and-tokens">11.4 Managing Keys and Tokens</h3>
          <p>When authentication is enabled, use the BUI to manage security.</p>
          <p><strong>Keys Page:</strong></p>
          <ul>
            <li>View all signing keys with their IDs and creation dates</li>
            <li>Create new signing keys</li>
            <li>Delete keys (except master key)</li>
          </ul>
          <p><strong>Tokens Page:</strong></p>
          <ul>
            <li>Generate new tokens with specific:</li>
          </ul>
          <p>  - Duration (hours, days)</p>
          <p>  - Endpoint access (chat-completions, embeddings, etc.)</p>
          <p>  - Rate limits (requests per day/month/year)</p>
          <ul>
            <li>Copy generated tokens to clipboard</li>
          </ul>
          <p><strong>Note:</strong> You must provide an admin token in the BUI settings to access</p>
          <p>security management features.</p>
          <h3 id="115-other-screens">11.5 Other Screens</h3>
          <p><strong>Dashboard:</strong></p>
          <p>Overview of server status, loaded models, and system information.</p>
          <p><strong>Documentation:</strong></p>
          <p>Built-in SDK and CLI documentation accessible from the menu:</p>
          <ul>
            <li>SDK API reference</li>
            <li>CLI command reference</li>
            <li>Example code</li>
          </ul>
          <p><strong>Settings:</strong></p>
          <p>Configure BUI preferences:</p>
          <ul>
            <li>API token for authenticated requests</li>
            <li>Theme preferences</li>
          </ul>
          <hr />
          <p>_Next: <a href="#chapter-12-client-integration">Chapter 12: Client Integration</a>_</p>
          <h2 id="chapter-12:-client-integration">Chapter 12: Client Integration</h2>
          <p>Kronk's OpenAI-compatible API works with popular AI clients and tools.</p>
          <h3 id="121-openwebui">12.1 OpenWebUI</h3>
          <p>OpenWebUI is a self-hosted chat interface that works with Kronk.</p>
          <p><strong>Configure OpenWebUI:</strong></p>
          <ol>
            <li>Open OpenWebUI settings</li>
            <li>Navigate to Connections → OpenAI API</li>
            <li>Set the base URL:</li>
          </ol>
          <pre className="code-block"><code>{`http://localhost:8080/v1`}</code></pre>
          <ol>
            <li>Set API key to your Kronk token (or any value (123) if auth is disabled)</li>
            <li>Save and refresh models</li>
          </ol>
          <p><strong>Features that work:</strong></p>
          <ul>
            <li>Chat completions with streaming</li>
            <li>Model selection from available models</li>
            <li>System prompts</li>
            <li>Conversation history</li>
          </ul>
          <h3 id="122-cline">12.2 Cline</h3>
          <p>Cline is a VS Code extension for AI-assisted coding.</p>
          <p><strong>Configure Cline for Kronk:</strong></p>
          <ol>
            <li>Open VS Code settings</li>
            <li>Search for "Cline"</li>
            <li>Set API Provider to "OpenAI Compatible"</li>
            <li>Configure:</li>
          </ol>
          <pre className="code-block"><code>{`Base URL: http://localhost:8080/v1
API Key: <your-kronk-token> or 123 for anything
Model: Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL/IMC`}</code></pre>
          <p><strong>Recommended Model Settings:</strong></p>
          <p>For coding tasks, configure your model with:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
    Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL:
    &base_Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL
    context-window: 131072
    nbatch: 2048
    nubatch: 512
    cache-type-k: q8_0
    cache-type-v: q8_0
    flash-attention: enabled
    nseq-max: 2
    insecure-logging: true
    sampling-parameters:
        temperature: 0.7
        top_p: 0.8
        top_k: 20

    Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL/IMC:
    <<: *base_Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL
    nseq-max: 1
    incremental-cache: true
    max-cache-sessions: 1`}</code></pre>
          <p>IMC is especially beneficial for Cline's iterative coding workflow.</p>
          <p>_Note: Don't use R1 Message formats when using KMS._</p>
          <h3 id="124-python-openai-sdk">12.4 Python OpenAI SDK</h3>
          <p>Use the official OpenAI Python library with Kronk.</p>
          <p><strong>Installation:</strong></p>
          <pre className="code-block"><code className="language-shell">{`pip install openai`}</code></pre>
          <p><strong>Usage:</strong></p>
          <pre className="code-block"><code className="language-python">{`from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="your-kronk-token"  # Or any string if auth disabled
)

response = client.chat.completions.create(
    model="Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL/IMC",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Hello!"}
    ],
    stream=True
)

for chunk in response:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")`}</code></pre>
          <h3 id="125-curl-and-http-clients">12.5 curl and HTTP Clients</h3>
          <p>Any HTTP client can call Kronk's REST API directly.</p>
          <p><strong>Basic Request:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -d '{
    "model": "Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'`}</code></pre>
          <p><strong>Streaming Response:</strong></p>
          <p>Streaming responses use Server-Sent Events (SSE) format:</p>
          <pre className="code-block"><code>{`data: {"id":"...","choices":[{"delta":{"content":"Hello"}}],...}

data: {"id":"...","choices":[{"delta":{"content":"!"}}],...}

data: [DONE]`}</code></pre>
          <h3 id="126-langchain">12.6 LangChain</h3>
          <p>Use LangChain with Kronk via the OpenAI integration.</p>
          <p><strong>Installation:</strong></p>
          <pre className="code-block"><code className="language-shell">{`pip install langchain-openai`}</code></pre>
          <p><strong>Usage:</strong></p>
          <pre className="code-block"><code className="language-python">{`from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="http://localhost:8080/v1",
    api_key="your-kronk-token",
    model="Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL",
    streaming=True
)

response = llm.invoke("Explain quantum computing briefly.")
print(response.content)`}</code></pre>
          <hr />
          <p>_Next: <a href="#chapter-13-observability">Chapter 13: Observability</a>_</p>
          <h2 id="chapter-13:-observability">Chapter 13: Observability</h2>
          <p>Kronk provides comprehensive observability through distributed tracing,</p>
          <p>Prometheus metrics, pprof profiling, and real-time visualizations.</p>
          <h3 id="131-debug-server">13.1 Debug Server</h3>
          <p>Kronk runs a separate debug server for observability endpoints, isolated</p>
          <p>from the main API for security.</p>
          <p><strong>Default Ports:</strong></p>
          <ul>
            <li>Main API: <code>localhost:8080</code></li>
            <li>Debug server: <code>localhost:8090</code></li>
          </ul>
          <p><strong>Configure Debug Host:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --debug-host localhost:9090`}</code></pre>
          <p>Or via environment variable:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_DEBUG_HOST=localhost:9090
kronk server start`}</code></pre>
          <h3 id="132-debug-endpoints">13.2 Debug Endpoints</h3>
          <p>The debug server exposes these endpoints:</p>
          <p><strong>Prometheus Metrics:</strong></p>
          <pre className="code-block"><code>{`http://localhost:8090/metrics`}</code></pre>
          <p><strong>pprof Profiling:</strong></p>
          <ul>
            <li><code>http://localhost:8090/debug/pprof/</code> - Index page</li>
            <li><code>http://localhost:8090/debug/pprof/profile</code> - CPU profile</li>
            <li><code>http://localhost:8090/debug/pprof/heap</code> - Heap profile</li>
            <li><code>http://localhost:8090/debug/pprof/goroutine</code> - Goroutine stacks</li>
            <li><code>http://localhost:8090/debug/pprof/trace</code> - Execution trace</li>
          </ul>
          <p><strong>Statsviz (Real-time Visualizations):</strong></p>
          <pre className="code-block"><code>{`http://localhost:8090/debug/statsviz`}</code></pre>
          <p>Provides live charts for memory, goroutines, GC, and more.</p>
          <h3 id="133-health-check-endpoints">13.3 Health Check Endpoints</h3>
          <p>Available on the main API port (no authentication required):</p>
          <p><strong>Liveness Check:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/liveness`}</code></pre>
          <p>Response:</p>
          <pre className="code-block"><code className="language-json">{`{
  "status": "up",
  "build": "v1.0.0",
  "host": "hostname",
  "GOMAXPROCS": 8
}`}</code></pre>
          <p><strong>Readiness Check:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/readiness`}</code></pre>
          <p>Returns 200 OK when the server is ready to accept requests.</p>
          <h3 id="134-prometheus-metrics">13.4 Prometheus Metrics</h3>
          <p>Kronk exposes detailed inference metrics in Prometheus format.</p>
          <p><strong>Fetch Metrics:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8090/metrics`}</code></pre>
          <p><strong>Available Metrics:</strong></p>
          <p>System metrics:</p>
          <ul>
            <li><code>goroutines</code> - Current goroutine count</li>
            <li><code>requests</code> - Total request count</li>
            <li><code>errors</code> - Total error count</li>
            <li><code>panics</code> - Total panic count</li>
          </ul>
          <p>Model loading (in seconds):</p>
          <ul>
            <li><code>model_load_avg</code>, <code>model_load_min</code>, <code>model_load_max</code></li>
            <li><code>model_load_proj_avg</code>, <code>model_load_proj_min</code>, <code>model_load_proj_max</code></li>
          </ul>
          <p>Inference timing (in seconds):</p>
          <ul>
            <li><code>model_prompt_creation_avg</code>, <code>_min</code>, <code>_max</code></li>
            <li><code>model_prefill_nonmedia_avg</code>, <code>_min</code>, <code>_max</code></li>
            <li><code>model_prefill_media_avg</code>, <code>_min</code>, <code>_max</code></li>
            <li><code>model_ttft_avg</code>, <code>_min</code>, <code>_max</code> (time to first token)</li>
          </ul>
          <p>Token usage:</p>
          <ul>
            <li><code>usage_prompt_tokens_avg</code>, <code>_min</code>, <code>_max</code></li>
            <li><code>usage_reasoning_tokens_avg</code>, <code>_min</code>, <code>_max</code></li>
            <li><code>usage_completion_tokens_avg</code>, <code>_min</code>, <code>_max</code></li>
            <li><code>usage_output_tokens_avg</code>, <code>_min</code>, <code>_max</code></li>
            <li><code>usage_total_tokens_avg</code>, <code>_min</code>, <code>_max</code></li>
            <li><code>usage_tokens_per_second_avg</code>, <code>_min</code>, <code>_max</code></li>
          </ul>
          <h3 id="135-prometheus-integration">13.5 Prometheus Integration</h3>
          <p><strong>Example Prometheus Configuration:</strong></p>
          <pre className="code-block"><code className="language-yaml">{`# prometheus.yml
scrape_configs:
  - job_name: "kronk"
    static_configs:
      - targets: ["localhost:8090"]
    scrape_interval: 15s`}</code></pre>
          <p><strong>Grafana Dashboard Query Examples:</strong></p>
          <p>Time to first token:</p>
          <pre className="code-block"><code className="language-promql">{`model_ttft_avg`}</code></pre>
          <p>Tokens per second throughput:</p>
          <pre className="code-block"><code className="language-promql">{`usage_tokens_per_second_avg`}</code></pre>
          <p>Request rate:</p>
          <pre className="code-block"><code className="language-promql">{`rate(requests[5m])`}</code></pre>
          <p>Error rate:</p>
          <pre className="code-block"><code className="language-promql">{`rate(errors[5m]) / rate(requests[5m])`}</code></pre>
          <h3 id="136-distributed-tracing-with-tempo">13.6 Distributed Tracing with Tempo</h3>
          <p>Kronk supports OpenTelemetry tracing with Grafana Tempo integration.</p>
          <p><strong>Enable Tracing:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --tempo-host localhost:4317 \\
  --tempo-service-name kronk \\
  --tempo-probability 0.25`}</code></pre>
          <p>Or via environment variables:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_TEMPO_HOST=localhost:4317
export KRONK_TEMPO_SERVICE_NAME=kronk
export KRONK_TEMPO_PROBABILITY=0.25
kronk server start`}</code></pre>
          <p><strong>Configuration Options:</strong></p>
          <ul>
            <li><code>--tempo-host</code> - Tempo collector address (OTLP gRPC endpoint)</li>
            <li><code>--tempo-service-name</code> - Service name in traces (default: <code>kronk</code>)</li>
            <li><code>--tempo-probability</code> - Sampling probability 0.0-1.0 (default: <code>0.25</code>)</li>
          </ul>
          <p><strong>Sampling Probability:</strong></p>
          <ul>
            <li><code>1.0</code> - Trace every request (development only)</li>
            <li><code>0.25</code> - Trace 25% of requests (recommended for production)</li>
            <li><code>0.05</code> - Trace 5% of requests (high-traffic production)</li>
          </ul>
          <p><strong>Excluded Routes:</strong></p>
          <p>Health check endpoints are automatically excluded from tracing:</p>
          <ul>
            <li><code>/v1/liveness</code></li>
            <li><code>/v1/readiness</code></li>
          </ul>
          <h3 id="137-tracing-architecture">13.7 Tracing Architecture</h3>
          <p><strong>Request Flow with Tracing:</strong></p>
          <pre className="code-block"><code>{`Client Request
      │
      ▼
┌─────────────────────────────┐
│  Kronk Server               │
│  ┌───────────────────────┐  │
│  │ Inject Trace Context  │  │
│  │ (trace_id, span_id)   │  │
│  └───────────┬───────────┘  │
│              ▼              │
│  ┌───────────────────────┐  │
│  │ Handler Span          │  │
│  │ (chat, embed, etc.)   │  │
│  └───────────┬───────────┘  │
│              ▼              │
│  ┌───────────────────────┐  │
│  │ Inference Span        │  │
│  │ (model operations)    │  │
│  └───────────────────────┘  │
└─────────────────────────────┘
      │
      ▼
   Tempo Collector (OTLP gRPC)
      │
      ▼
   Grafana (Visualization)`}</code></pre>
          <p><strong>What Gets Traced:</strong></p>
          <ul>
            <li>HTTP request handling</li>
            <li>Model acquisition from pool</li>
            <li>Prefill and generation phases</li>
            <li>Token streaming</li>
          </ul>
          <h3 id="138-tempo-setup-with-docker">13.8 Tempo Setup with Docker</h3>
          <p><strong>Run Tempo Locally:</strong></p>
          <pre className="code-block"><code className="language-shell">{`docker run -d --name tempo \\
  -p 3200:3200 \\
  -p 4317:4317 \\
  grafana/tempo:latest \\
  -config.file=/etc/tempo/tempo.yaml`}</code></pre>
          <p><strong>Run Grafana:</strong></p>
          <pre className="code-block"><code className="language-shell">{`docker run -d --name grafana \\
  -p 3000:3000 \\
  grafana/grafana:latest`}</code></pre>
          <p><strong>Configure Grafana:</strong></p>
          <ol>
            <li>Open http://localhost:3000 (admin/admin)</li>
            <li>Add data source → Tempo</li>
            <li>Set URL: <code>http://tempo:3200</code></li>
            <li>Save and explore traces</li>
          </ol>
          <h3 id="139-pprof-profiling">13.9 pprof Profiling</h3>
          <p>Use Go's pprof tools for performance analysis.</p>
          <p><strong>Capture CPU Profile (30 seconds):</strong></p>
          <pre className="code-block"><code className="language-shell">{`go tool pprof http://localhost:8090/debug/pprof/profile?seconds=30`}</code></pre>
          <p><strong>Capture Heap Profile:</strong></p>
          <pre className="code-block"><code className="language-shell">{`go tool pprof http://localhost:8090/debug/pprof/heap`}</code></pre>
          <p><strong>View Goroutine Stacks:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8090/debug/pprof/goroutine?debug=2`}</code></pre>
          <p><strong>Generate Flame Graph:</strong></p>
          <pre className="code-block"><code className="language-shell">{`go tool pprof -http=:8081 \\
  http://localhost:8090/debug/pprof/profile?seconds=30`}</code></pre>
          <p>Opens interactive web UI with flame graph visualization.</p>
          <h3 id="1310-statsviz-real-time-monitoring">13.10 Statsviz Real-Time Monitoring</h3>
          <p>Statsviz provides live runtime visualizations in your browser.</p>
          <p><strong>Access Statsviz:</strong></p>
          <pre className="code-block"><code>{`http://localhost:8090/debug/statsviz`}</code></pre>
          <p><strong>Available Charts:</strong></p>
          <ul>
            <li>Heap size and allocations</li>
            <li>Goroutine count</li>
            <li>GC pause times</li>
            <li>CPU scheduler latency</li>
            <li>Memory by size class</li>
          </ul>
          <p>Useful for real-time monitoring during load testing or debugging</p>
          <p>memory issues.</p>
          <h3 id="1311-logging">13.11 Logging</h3>
          <p>Kronk logs structured JSON to stdout by default.</p>
          <p><strong>Log Levels:</strong></p>
          <p>Logs include context like trace IDs, request details, and timing.</p>
          <p><strong>Insecure Logging:</strong></p>
          <p>For debugging, enable verbose logging that includes message content:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --insecure-logging`}</code></pre>
          <p><strong>Warning:</strong> Insecure logging exposes user prompts and model responses.</p>
          <p>Never enable in production.</p>
          <p><strong>Environment Variable:</strong></p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_INSECURE_LOGGING=true`}</code></pre>
          <h3 id="1312-configuration-reference">13.12 Configuration Reference</h3>
          <p><strong>Debug Server:</strong></p>
          <ul>
            <li><code>--debug-host</code> - Debug server address (env: <code>KRONK_DEBUG_HOST</code>,</li>
          </ul>
          <p>  default: <code>localhost:8090</code>)</p>
          <p><strong>Tracing:</strong></p>
          <ul>
            <li><code>--tempo-host</code> - Tempo collector address (env: <code>KRONK_TEMPO_HOST</code>,</li>
          </ul>
          <p>  default: <code>localhost:4317</code>)</p>
          <ul>
            <li><code>--tempo-service-name</code> - Service name (env: <code>KRONK_TEMPO_SERVICE_NAME</code>,</li>
          </ul>
          <p>  default: <code>kronk</code>)</p>
          <ul>
            <li><code>--tempo-probability</code> - Sampling rate 0.0-1.0</li>
          </ul>
          <p>  (env: <code>KRONK_TEMPO_PROBABILITY</code>, default: <code>0.25</code>)</p>
          <p><strong>Logging:</strong></p>
          <ul>
            <li><code>--insecure-logging</code> - Log message content</li>
          </ul>
          <p>  (env: <code>KRONK_INSECURE_LOGGING</code>, default: <code>false</code>)</p>
          <ul>
            <li><code>--llama-log</code> - llama.cpp log level, 0=off, 1=on</li>
          </ul>
          <p>  (env: <code>KRONK_LLAMA_LOG</code>, default: <code>1</code>)</p>
          <hr />
          <p>_Next: <a href="#chapter-14-troubleshooting">Chapter 14: Troubleshooting</a>_</p>
          <h2 id="chapter-14:-troubleshooting">Chapter 14: Troubleshooting</h2>
          <p>This chapter covers common issues, their causes, and solutions.</p>
          <h3 id="141-library-issues">14.1 Library Issues</h3>
          <p><strong>Error: "unable to load library"</strong></p>
          <p>The llama.cpp libraries are missing or incompatible.</p>
          <p><strong>Solution:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local`}</code></pre>
          <p>Or download via the BUI Libraries page.</p>
          <p><strong>Error: "unknown device"</strong></p>
          <p>The specified GPU device is not available.</p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Wrong <code>--device</code> flag (e.g., <code>cuda</code> on a Mac)</li>
            <li>GPU drivers not installed</li>
            <li>Library mismatch (CPU library with GPU device setting)</li>
          </ul>
          <p><strong>Solution:</strong></p>
          <p>Check your hardware and install matching libraries:</p>
          <pre className="code-block"><code className="language-shell">{`# For Mac with Apple Silicon
KRONK_PROCESSOR=metal kronk libs --local

# For NVIDIA GPU
KRONK_PROCESSOR=cuda kronk libs --local

# For CPU only
KRONK_PROCESSOR=cpu kronk libs --local`}</code></pre>
          <h3 id="142-model-loading-failures">14.2 Model Loading Failures</h3>
          <p><strong>Error: "unable to load model"</strong></p>
          <p>The model file is missing, corrupted, or incompatible.</p>
          <p><strong>Check model exists:</strong></p>
          <pre className="code-block"><code className="language-shell">{`ls ~/.kronk/models/`}</code></pre>
          <p><strong>Re-download the model:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog pull <model-name> --local`}</code></pre>
          <p><strong>Verify model integrity:</strong></p>
          <p>By default, Kronk skips integrity checks. To force verification:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --ignore-integrity-check=false`}</code></pre>
          <p><strong>Error: "failed to retrieve model template"</strong></p>
          <p>The model's chat template is missing.</p>
          <p><strong>Solution:</strong></p>
          <p>Ensure templates are downloaded:</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog pull-templates --local`}</code></pre>
          <h3 id="143-memory-errors">14.3 Memory Errors</h3>
          <p><strong>Error: "unable to init context" or "unable to get memory"</strong></p>
          <p>Insufficient memory for the model configuration.</p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Context window too large</li>
            <li>Too many batch slots</li>
            <li>Model too large for available RAM/VRAM</li>
          </ul>
          <p><strong>Solutions:</strong></p>
          <p>Reduce context window:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    context_window: 8192 # Reduce from 32768`}</code></pre>
          <p>Reduce batch parallelism:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    n_seq_max: 1 # Single request at a time`}</code></pre>
          <p>Use quantized KV cache:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    cache-type-k: q8_0 # Saves ~50% KV cache memory
    cache-type-v: q8_0`}</code></pre>
          <p><strong>Error: "context window is full"</strong></p>
          <p>The request plus context exceeds the configured context window.</p>
          <p><strong>Solutions:</strong></p>
          <ul>
            <li>Reduce input size (fewer messages or shorter prompts)</li>
            <li>Increase <code>context_window</code> in model config</li>
            <li>Enable YaRN for extended context (see Chapter 6)</li>
          </ul>
          <h3 id="144-request-timeouts">14.4 Request Timeouts</h3>
          <p><strong>Error: "context deadline exceeded"</strong></p>
          <p>The request took longer than the configured timeout.</p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Model too slow for the request size</li>
            <li>Large prefill with many tokens</li>
            <li>Server under heavy load</li>
          </ul>
          <p><strong>Solutions:</strong></p>
          <p>Increase HTTP timeouts:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --read-timeout 5m \\
  --write-timeout 30m`}</code></pre>
          <p>Or via environment variables:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_READ_TIMEOUT=5m
export KRONK_WRITE_TIMEOUT=30m`}</code></pre>
          <h3 id="145-authentication-errors">14.5 Authentication Errors</h3>
          <p><strong>Error: "unauthorized: no authorization header"</strong></p>
          <p>Authentication is enabled but no token was provided.</p>
          <p><strong>Solution:</strong></p>
          <p>Include the Authorization header:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $(cat ~/.kronk/keys/master.jwt)" \\
  -H "Content-Type: application/json" \\
  -d '{...}'`}</code></pre>
          <p><strong>Error: "invalid token"</strong></p>
          <p>The token is malformed, expired, or signed with an unknown key.</p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Token has expired (check <code>--duration</code> when created)</li>
            <li>Signing key was deleted</li>
            <li>Token is corrupted</li>
          </ul>
          <p><strong>Solution:</strong></p>
          <p>Create a new token:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)
kronk security token create \\
  --duration 720h \\
  --endpoints chat-completions,embeddings`}</code></pre>
          <p><strong>Error: "endpoint not authorized"</strong></p>
          <p>The token doesn't include the requested endpoint.</p>
          <p><strong>Solution:</strong></p>
          <p>Create a new token with the required endpoints:</p>
          <pre className="code-block"><code className="language-shell">{`kronk security token create \\
  --duration 720h \\
  --endpoints chat-completions,embeddings,rerank,responses,messages`}</code></pre>
          <p><strong>Error: "rate limit exceeded"</strong></p>
          <p>The token has exceeded its rate limit.</p>
          <p><strong>Solution:</strong></p>
          <p>Wait for the rate limit window to reset, or create a new token with</p>
          <p>higher limits:</p>
          <pre className="code-block"><code className="language-shell">{`kronk security token create \\
  --duration 720h \\
  --endpoints "chat-completions:10000/day"`}</code></pre>
          <h3 id="146-streaming-issues">14.6 Streaming Issues</h3>
          <p><strong>Problem: Streaming stops mid-response</strong></p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Client disconnected</li>
            <li>Request timeout</li>
            <li>Model generated stop token</li>
          </ul>
          <p><strong>Check server logs:</strong></p>
          <pre className="code-block"><code className="language-shell">{`# Look for errors in server output
kronk server start  # Run in foreground to see logs`}</code></pre>
          <p><strong>Problem: SSE events not parsing correctly</strong></p>
          <p>Ensure your client handles Server-Sent Events format:</p>
          <pre className="code-block"><code>{`data: {"id":"...","choices":[...]}\\n\\n`}</code></pre>
          <p>Each event is prefixed with <code>data: </code> and ends with two newlines.</p>
          <h3 id="147-performance-issues">14.7 Performance Issues</h3>
          <p><strong>Problem: Slow time to first token (TTFT)</strong></p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Large system prompt not cached</li>
            <li>No message caching enabled</li>
            <li>Cold model load</li>
          </ul>
          <p><strong>Solutions:</strong></p>
          <p>Enable system prompt caching:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    system_prompt_cache: true`}</code></pre>
          <p>Or enable incremental message cache for agents:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    incremental_cache: true
    max_cache_sessions: 4`}</code></pre>
          <p><strong>Problem: Slow token generation (tokens/second)</strong></p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>CPU inference instead of GPU</li>
            <li>Insufficient GPU layers</li>
            <li>Large model for available hardware</li>
          </ul>
          <p><strong>Solutions:</strong></p>
          <p>Check GPU is being used:</p>
          <pre className="code-block"><code className="language-shell">{`# On macOS, check Metal usage
sudo powermetrics --samplers gpu_power

# On Linux with NVIDIA
nvidia-smi`}</code></pre>
          <p>Increase GPU layers:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  Qwen3-8B-Q8_0:
    gpu_layers: 99 # Offload all layers to GPU`}</code></pre>
          <h3 id="148-viewing-logs">14.8 Viewing Logs</h3>
          <p><strong>Run server in foreground:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start`}</code></pre>
          <p>All logs print to stdout with structured JSON format.</p>
          <p><strong>Enable verbose logging:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --insecure-logging`}</code></pre>
          <p>This logs full message content (never use in production).</p>
          <p><strong>Enable llama.cpp logging:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --llama-log 1`}</code></pre>
          <p>Shows low-level inference engine messages.</p>
          <p><strong>Disable llama.cpp logging:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --llama-log 0`}</code></pre>
          <h3 id="149-common-error-messages">14.9 Common Error Messages</h3>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Error</th>
                <th>Cause</th>
                <th>Solution</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>Init() not called</code></td>
                <td>Missing initialization</td>
                <td>Call <code>kronk.Init()</code></td>
              </tr>
              <tr>
                <td><code>unknown device</code></td>
                <td>Invalid GPU setting</td>
                <td>Check <code>--device</code> flag</td>
              </tr>
              <tr>
                <td><code>context deadline</code></td>
                <td>Request timeout</td>
                <td>Increase timeouts</td>
              </tr>
              <tr>
                <td><code>unable to load model</code></td>
                <td>Missing/corrupt model</td>
                <td>Re-download model</td>
              </tr>
              <tr>
                <td><code>no authorization</code></td>
                <td>Missing token</td>
                <td>Add Bearer token</td>
              </tr>
              <tr>
                <td><code>rate limit exceeded</code></td>
                <td>Quota exhausted</td>
                <td>Wait or increase limit</td>
              </tr>
              <tr>
                <td><code>context window full</code></td>
                <td>Input too large</td>
                <td>Reduce input size</td>
              </tr>
              <tr>
                <td><code>NBatch overflow</code></td>
                <td>Batch too large</td>
                <td>Reduce <code>n_batch</code></td>
              </tr>
            </tbody>
          </table>
          <h3 id="1410-getting-help">14.10 Getting Help</h3>
          <p><strong>Check server status:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/liveness`}</code></pre>
          <p><strong>List loaded models:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8080/v1/models`}</code></pre>
          <p><strong>Check metrics:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8090/metrics`}</code></pre>
          <p><strong>View goroutine stacks (for hangs):</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8090/debug/pprof/goroutine?debug=2`}</code></pre>
          <p><strong>Report issues:</strong></p>
          <p>Include the following when reporting bugs:</p>
          <ul>
            <li>Kronk version (<code>kronk --version</code>)</li>
            <li>Operating system and architecture</li>
            <li>GPU type and driver version</li>
            <li>Model name and configuration</li>
            <li>Full error message and stack trace</li>
            <li>Steps to reproduce</li>
          </ul>
          <hr />
          <p>_Next: <a href="#chapter-15-developer-guide">Chapter 15: Developer Guide</a>_</p>
          <h2 id="chapter-15:-developer-guide">Chapter 15: Developer Guide</h2>
          <p>This chapter covers development workflows, build commands, and code</p>
          <p>conventions for contributors to the Kronk project.</p>
          <h3 id="151-quick-reference">15.1 Quick Reference</h3>
          <p>Here is a quick chart of some of the more imporant make commands.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Task</th>
                <th>Command</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Install CLI</td>
                <td><code>make install-kronk</code>.</td>
              </tr>
              <tr>
                <td>Run all tests</td>
                <td><code>make test</code></td>
              </tr>
              <tr>
                <td>Single test</td>
                <td><code>go test -v -count=1 -run TestName ./sdk/kronk/...</code></td>
              </tr>
              <tr>
                <td>Run server</td>
                <td><code>make kronk-server</code></td>
              </tr>
              <tr>
                <td>Build BUI</td>
                <td><code>make bui-build</code></td>
              </tr>
              <tr>
                <td>Generate docs</td>
                <td><code>make kronk-docs</code></td>
              </tr>
              <tr>
                <td>Tidy modules</td>
                <td><code>make tidy</code></td>
              </tr>
              <tr>
                <td>Update deps</td>
                <td><code>make deps-upgrade</code></td>
              </tr>
              <tr>
                <td>Lint</td>
                <td><code>staticcheck ./...</code></td>
              </tr>
              <tr>
                <td>Developer setup</td>
                <td><code>make setup</code> (configures git hooks)</td>
              </tr>
            </tbody>
          </table>
          <h3 id="152-build-test-commands">15.2 Build &amp; Test Commands</h3>
          <p><strong>Install CLI locally:</strong></p>
          <pre className="code-block"><code className="language-shell">{`go install ./cmd/kronk`}</code></pre>
          <p><strong>Run all tests:</strong></p>
          <pre className="code-block"><code className="language-shell">{`make test`}</code></pre>
          <p>Tests require prerequisites and environment variables:</p>
          <pre className="code-block"><code className="language-shell">{`# Install dependencies first
make install-libraries install-models

# Set required environment variables
export RUN_IN_PARALLEL=yes
export GITHUB_WORKSPACE=/path/to/kronk  # project root

# Run from project root directory
make test`}</code></pre>
          <p><strong>Run a single test:</strong></p>
          <pre className="code-block"><code className="language-shell">{`go test -v -count=1 -run TestName ./sdk/kronk/...`}</code></pre>
          <h3 id="153-developer-setup">15.3 Developer Setup</h3>
          <p>Configure git hooks for automatic pre-commit checks:</p>
          <pre className="code-block"><code className="language-shell">{`make setup`}</code></pre>
          <p>This enables a pre-commit hook that automatically runs:</p>
          <ul>
            <li><code>make kronk-docs</code> - Regenerates documentation</li>
            <li><code>make bui-build</code> - Rebuilds the BUI frontend</li>
          </ul>
          <h3 id="154-project-architecture">15.4 Project Architecture</h3>
          <p><strong>Directory Structure:</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Directory</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>cmd/kronk/</code></td>
                <td>CLI tool (subcommands: catalog, libs, model, run, security, server)</td>
              </tr>
              <tr>
                <td><code>cmd/server/</code></td>
                <td>OpenAI-compatible model server (gRPC + HTTP) with BUI frontend</td>
              </tr>
              <tr>
                <td><code>cmd/server/api/tooling/docs/</code></td>
                <td>Documentation generator for BUI (SDK and CLI docs)</td>
              </tr>
              <tr>
                <td><code>sdk/kronk/</code></td>
                <td>Core API: model loading, chat, embeddings, cache, metrics</td>
              </tr>
              <tr>
                <td><code>sdk/kronk/model/</code></td>
                <td>Core inference and caching engine</td>
              </tr>
              <tr>
                <td><code>sdk/kronk/observ/</code></td>
                <td>Observability packages (metrics/, otel/)</td>
              </tr>
              <tr>
                <td><code>sdk/tools/</code></td>
                <td>Support for libs, models, catalogs, templates, and defaults</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Core Technology:</strong></p>
          <p>Kronk uses <a href="https://github.com/hybridgroup/yzma">yzma</a> (llama.cpp Go bindings)</p>
          <p>for local inference with GGUF models.</p>
          <h3 id="155-bui-frontend-development">15.5 BUI Frontend Development</h3>
          <p>The Browser UI is a React application located at:</p>
          <pre className="code-block"><code>{`cmd/server/api/frontends/bui/src/`}</code></pre>
          <p><strong>Directory Structure:</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Directory/File</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>components/</code></td>
                <td>React components (pages and UI elements)</td>
              </tr>
              <tr>
                <td><code>contexts/</code></td>
                <td>React context providers for shared state</td>
              </tr>
              <tr>
                <td><code>services/</code></td>
                <td>API client (<code>api.ts</code>)</td>
              </tr>
              <tr>
                <td><code>types/</code></td>
                <td>TypeScript type definitions</td>
              </tr>
              <tr>
                <td><code>App.tsx</code></td>
                <td>Main app with routing configuration</td>
              </tr>
              <tr>
                <td><code>index.css</code></td>
                <td>Global styles (CSS variables, component styles)</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Routing:</strong></p>
          <p>Uses <code>react-router-dom</code> with <code>BrowserRouter</code>. Routes are defined in</p>
          <p><code>routeMap</code> in <code>App.tsx</code>.</p>
          <p><strong>Adding New Pages:</strong></p>
          <ol>
            <li>Create component in <code>components/</code> (e.g., <code>DocsSDKKronk.tsx</code>)</li>
            <li>Add page type to <code>Page</code> union in <code>App.tsx</code></li>
            <li>Add route path to <code>routeMap</code> in <code>App.tsx</code></li>
            <li>Add <code>&lt;Route&gt;</code> element in <code>App.tsx</code></li>
            <li>Add <code>&lt;Link&gt;</code> entry to menu in <code>components/Layout.tsx</code></li>
          </ol>
          <p><strong>Menu Structure (&lt;code&gt;Layout.tsx&lt;/code&gt;):</strong></p>
          <p>Uses <code>MenuCategory[]</code> with properties:</p>
          <ul>
            <li><code>id</code> - Unique identifier</li>
            <li><code>label</code> - Display text</li>
            <li><code>items</code> - Array of leaf pages</li>
            <li><code>subcategories</code> - Nested menu categories</li>
          </ul>
          <p><strong>State Management:</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Context</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>TokenContext</code></td>
                <td>Stores API token in localStorage (key: <code>kronk_token</code>)</td>
              </tr>
              <tr>
                <td><code>ModelListContext</code></td>
                <td>Caches model list data with invalidation support</td>
              </tr>
            </tbody>
          </table>
          <p>Access via hooks: <code>useToken()</code>, <code>useModelList()</code></p>
          <p><strong>API Service (&lt;code&gt;services/api.ts&lt;/code&gt;):</strong></p>
          <ul>
            <li><code>ApiService</code> class with methods for all endpoints</li>
            <li>Streaming support for pull operations (models, catalog, libs)</li>
            <li>Auth-required endpoints accept token parameter</li>
          </ul>
          <p><strong>Styling Conventions:</strong></p>
          <ul>
            <li>CSS variables defined in <code>:root</code> (colors: <code>--color-orange</code>, <code>--color-blue</code>, etc.)</li>
            <li>Common classes: <code>.card</code>, <code>.btn</code>, <code>.btn-primary</code>, <code>.form-group</code>, <code>.alert</code>, <code>.table-container</code></li>
            <li>No CSS modules or styled-components; use global CSS classes</li>
          </ul>
          <p><strong>Documentation Generation:</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Type</th>
                <th>Generator Location</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>SDK docs</td>
                <td><code>cmd/server/api/tooling/docs/sdk/</code> (uses <code>go doc</code> output)</td>
              </tr>
              <tr>
                <td>CLI docs</td>
                <td><code>cmd/server/api/tooling/docs/cli/</code> (from command definitions)</td>
              </tr>
              <tr>
                <td>Examples</td>
                <td>Auto-generated from <code>examples/</code> directory</td>
              </tr>
            </tbody>
          </table>
          <p>Generate all documentation:</p>
          <pre className="code-block"><code className="language-shell">{`go run ./cmd/server/api/tooling/docs -pkg=all`}</code></pre>
          <h3 id="156-code-style-guidelines">15.6 Code Style Guidelines</h3>
          <p><strong>Package Comments:</strong></p>
          <pre className="code-block"><code className="language-go">{`// Package kronk provides the core inference API.`}</code></pre>
          <p><strong>Error Handling:</strong></p>
          <pre className="code-block"><code className="language-go">{`// Wrap errors with lowercase context prefix
return fmt.Errorf("loading model: %w", err)

// Declare package-level sentinel errors
var ErrModelNotFound = errors.New("model not found")`}</code></pre>
          <p><strong>Struct Design:</strong></p>
          <ul>
            <li>Use unexported fields with exported types</li>
            <li>Use <code>Config</code> pattern for constructors</li>
          </ul>
          <pre className="code-block"><code className="language-go">{`type Config struct {
    Host string
    Port int
}

func New(cfg Config) *Server {
    // ...
}`}</code></pre>
          <p><strong>Testing:</strong></p>
          <p>Disable CGO in tests:</p>
          <pre className="code-block"><code className="language-shell">{`CGO_ENABLED=0 go test ./...`}</code></pre>
          <p><strong>Import Order (goimports):</strong></p>
          <ol>
            <li>Standard library</li>
            <li>External packages</li>
            <li>Internal packages</li>
          </ol>
          <p><strong>Control Flow:</strong></p>
          <ul>
            <li>Avoid <code>else</code> and <code>else if</code> clauses</li>
            <li>Prefer <code>switch</code> statements or early returns</li>
          </ul>
          <pre className="code-block"><code className="language-go">{`// Preferred: early return
if err != nil {
    return err
}
// continue with main logic

// Preferred: switch over if-else chains
switch state {
case "active":
    // ...

case "pending":
    // ...

default:
    // ...
}`}</code></pre>
          <h3 id="157-sdk-internals">15.7 SDK Internals</h3>
          <p>This section documents implementation details for developers working on</p>
          <p>the Kronk SDK packages.</p>
          <h4 id="1571-package-structure">15.7.1 Package Structure</h4>
          <p><strong>sdk/kronk/</strong> - Core API package:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>File</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>acquire.go</code></td>
                <td>Model pool acquire/release</td>
              </tr>
              <tr>
                <td><code>chat.go</code></td>
                <td>Chat completion API</td>
              </tr>
              <tr>
                <td><code>concurrency.go</code></td>
                <td>Generic streaming utilities</td>
              </tr>
              <tr>
                <td><code>embedding.go</code></td>
                <td>Embedding API</td>
              </tr>
              <tr>
                <td><code>init.go</code></td>
                <td>Initialization and configuration</td>
              </tr>
              <tr>
                <td><code>kronk.go</code></td>
                <td>Main Kronk type, model pool management</td>
              </tr>
              <tr>
                <td><code>rerank.go</code></td>
                <td>Reranking API</td>
              </tr>
              <tr>
                <td><code>response.go</code></td>
                <td>OpenAI Responses API streaming</td>
              </tr>
            </tbody>
          </table>
          <p><strong>sdk/kronk/model/</strong> - Low-level inference:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>File</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>batch.go</code></td>
                <td>Batch engine for parallel text inference</td>
              </tr>
              <tr>
                <td><code>caching.go</code></td>
                <td>System prompt and IMC cache management</td>
              </tr>
              <tr>
                <td><code>chat.go</code></td>
                <td>Chat inference loop, batch vs sequential routing</td>
              </tr>
              <tr>
                <td><code>config.go</code></td>
                <td>Model configuration (GPU, cache, batching)</td>
              </tr>
              <tr>
                <td><code>embed.go</code></td>
                <td>Embedding inference</td>
              </tr>
              <tr>
                <td><code>logprobs.go</code></td>
                <td>Token log probability extraction</td>
              </tr>
              <tr>
                <td><code>media.go</code></td>
                <td>Vision/audio media processing</td>
              </tr>
              <tr>
                <td><code>model.go</code></td>
                <td>Model type, context management, lifecycle</td>
              </tr>
              <tr>
                <td><code>models.go</code></td>
                <td>OpenAI-compatible types (ChatMessage, ToolCall, etc.)</td>
              </tr>
              <tr>
                <td><code>params.go</code></td>
                <td>Sampling parameters</td>
              </tr>
              <tr>
                <td><code>processor.go</code></td>
                <td>Template-specific token processors</td>
              </tr>
              <tr>
                <td><code>prompts.go</code></td>
                <td>Prompt formatting</td>
              </tr>
              <tr>
                <td><code>rerank.go</code></td>
                <td>Reranking inference</td>
              </tr>
            </tbody>
          </table>
          <h4 id="1572-streaming-architecture">15.7.2 Streaming Architecture</h4>
          <p><strong>Response Streaming Pattern</strong> (<code>response.go</code>, <code>concurrency.go</code>):</p>
          <ul>
            <li>Uses <code>streamingWith[T, U]</code> generic function for 1:N event transformation</li>
            <li><code>streamProcessor</code> has three phases: <code>Start()</code>, <code>Process(chunk)</code>, <code>Complete(lastChunk)</code></li>
            <li><code>streamState</code> struct maintains response ID, sequence numbers, aggregated usage</li>
            <li>SSE format: <code>event: &lt;type&gt;\ndata: &lt;json&gt;\n\n</code></li>
          </ul>
          <p><strong>FinishReason Handling:</strong></p>
          <ul>
            <li><code>FinishReasonPtr *string</code> field with <code>FinishReason()</code> accessor</li>
            <li>Constants: <code>FinishReasonStop="stop"</code>, <code>FinishReasonTool="tool_calls"</code>, <code>FinishReasonError="error"</code></li>
            <li>When <code>FinishReasonPtr != nil</code>, skip text/reasoning deltas (they duplicate previous content)</li>
            <li>Always process tool calls even with FinishReason set (may only arrive in final chunk)</li>
          </ul>
          <h4 id="1573-model-pool-strategy">15.7.3 Model Pool Strategy</h4>
          <p><code>NSeqMax</code> behaves differently depending on model type:</p>
          <p><strong>Sequential Models</strong> (embed, rerank, vision/audio):</p>
          <ul>
            <li><code>NSeqMax</code> controls the number of model instances in the pool</li>
            <li>Each instance handles one request at a time (single-flight)</li>
            <li>Pooled via <code>krn.pool</code> channel for concurrent request handling</li>
          </ul>
          <p><strong>Text Inference Models</strong> (chat, completion):</p>
          <ul>
            <li><code>NSeqMax</code> controls batch parallelism within a single model instance</li>
            <li>Only one <code>model.Model</code> instance is created</li>
            <li>Semaphore capacity = <code>NSeqMax * queueDepth</code> (default queueDepth=2)</li>
          </ul>
          <p><strong>Detection Logic</strong> (<code>kronk.go</code>):</p>
          <pre className="code-block"><code className="language-go">{`isSingleFlight := cfg.ProjFile != ""  // Vision/audio projector
if mi.IsEmbedModel || mi.IsRerankModel {
    isSingleFlight = true
}`}</code></pre>
          <h4 id="1574-model-acquirerelease-cleanup">15.7.4 Model Acquire/Release &amp; Cleanup</h4>
          <p><strong>Two-Stage Acquisition</strong> (<code>acquire.go</code>):</p>
          <ol>
            <li><strong>Backpressure slot</strong>: Acquire semaphore slot (limits total in-flight requests)</li>
            <li><strong>Model instance</strong>: If pooled, acquire specific model from pool channel</li>
          </ol>
          <p><strong>Cleanup Flow:</strong></p>
          <ol>
            <li><code>streaming()</code> acquires model, defers <code>releaseModel()</code> in wrapper goroutine</li>
            <li><code>ChatStreaming</code> defers <code>m.resetContext()</code> before any processing</li>
            <li>When generation completes, <code>resetContext()</code> runs first:</li>
          </ol>
          <p>   - <code>llama.Synchronize(m.lctx)</code> - waits for GPU operations</p>
          <p>   - <code>llama.MemoryClear(mem, true)</code> - clears KV cache</p>
          <ol>
            <li>Channel closes, wrapper exits, <code>releaseModel()</code> runs</li>
            <li>Model returns to pool in clean state</li>
          </ol>
          <p><strong>Key invariant:</strong> <code>resetContext()</code> always runs before model release due to defer ordering.</p>
          <h4 id="1575-batch-engine-internals">15.7.5 Batch Engine Internals</h4>
          <p><strong>ChatStreaming Decision Logic</strong> (<code>chat.go</code>):</p>
          <p>The <code>submitToBatchEngine()</code> function decides the processing path:</p>
          <pre className="code-block"><code className="language-go">{`// submitToBatchEngine returns false if batch not available.
if m.batch == nil || object != ObjectChatText {
    return false
}
// Submit job to batch engine...
return true`}</code></pre>
          <p>If <code>submitToBatchEngine()</code> returns false, the sequential path is used:</p>
          <pre className="code-block"><code className="language-go">{`if m.submitToBatchEngine(...) {
    batching = true
    return
}
m.sequentialChatRequest(...)`}</code></pre>
          <p><strong>Batch Engine Architecture</strong> (<code>batch.go</code>):</p>
          <ul>
            <li><code>batchEngine</code> manages <code>nSlots</code> parallel <code>slot</code> structs</li>
            <li>Each slot tracks: <code>seqID</code>, prompt tokens, decode state, sampler, response channel, logprobs, prefill state</li>
            <li>Signal-based wake pattern: <code>wakeCh chan struct&#123;&#125;</code> (buffered size 1) wakes immediately on new requests</li>
            <li>Polling intervals: 100µs (active slots generating), 5ms (idle, no active slots)</li>
          </ul>
          <p><strong>Slots vs Sequences:</strong></p>
          <ul>
            <li><code>slot.id</code> = slot index (for logging)</li>
            <li><code>slot.seqID</code> = llama.cpp sequence ID (determines KV cache partition)</li>
            <li><code>slot.seqIDs</code> = pre-allocated slice for efficient <code>batchAdd</code> calls</li>
          </ul>
          <p>Sequences are isolated partitions in the shared KV cache memory. Slot seqIDs</p>
          <p>are offset when caching is enabled (both SPC and IMC use seqs 0 to</p>
          <p>MaxCacheSessions-1).</p>
          <h4 id="1576-context-pooling">15.7.6 Context Pooling</h4>
          <ul>
            <li><code>llama.Context</code> is created once in <code>NewModel</code> and reused across requests</li>
            <li>Call <code>resetContext()</code> between requests to clear KV cache</li>
            <li>Avoids Vulkan memory fragmentation from repeated context alloc/dealloc</li>
          </ul>
          <h4 id="1577-imc-implementation-details">15.7.7 IMC Implementation Details</h4>
          <p><strong>Critical Implementation Details:</strong></p>
          <ol>
            <li><strong>Extension tokenization must use &lt;code&gt;special=true&lt;/code&gt;</strong>: Use <code>llama.Tokenize(vocab, extension, false, true)</code> to ensure ChatML tokens like <code>&lt;|im_start|&gt;</code> are recognized.</li>
          </ol>
          <ol>
            <li><strong>Prefix mismatch detection</strong>: Use <code>strings.HasPrefix(fullPrompt, prefixPrompt)</code> to detect Jinja template nondeterminism.</li>
          </ol>
          <ol>
            <li><strong>&lt;code&gt;add_generation_prompt=false&lt;/code&gt; for cached prefixes</strong>: Creates valid prefix for extension. Generation prompt added only for final suffix.</li>
          </ol>
          <p><strong>IMC Algorithm:</strong></p>
          <ol>
            <li>First request (cache empty): Cache <code>messages[0:len-1]</code>, generate from last message</li>
            <li>Subsequent requests (prefix match): Extend cache with <code>messages[cachedCount:len-1]</code></li>
            <li>New thread (prefix mismatch): Rebuild cache from scratch</li>
          </ol>
          <p><strong>IMC Session State:</strong></p>
          <pre className="code-block"><code className="language-go">{`type imcSession struct {
    hash      string      // Hash of all cached messages
    tokens    int         // Total tokens in cache
    msgCount  int         // Number of messages cached
    promptLen int         // Length of templated prefix
    seqID     llama.SeqId // Assigned cache sequence ID
    lastUsed  time.Time   // For future eviction
}`}</code></pre>
          <h4 id="1578-tool-call-internals">15.7.8 Tool Call Internals</h4>
          <p><strong>chatMessage Unmarshaling</strong> (<code>models.go</code>):</p>
          <ul>
            <li><code>Content</code> can be <code>nil</code> for assistant messages with tool_calls</li>
            <li>Handle <code>len(app.Content) == 0 || string(app.Content) == "null"</code> as valid empty content</li>
          </ul>
          <p><strong>ToolCallArguments Type:</strong></p>
          <ul>
            <li>Custom type that marshals to JSON string (OpenAI spec)</li>
            <li>Unmarshals from either string or object for non-compliant clients</li>
          </ul>
          <h4 id="1579-logprobs-implementation">15.7.9 Logprobs Implementation</h4>
          <p><strong>Implementation</strong> (<code>logprobs.go</code>):</p>
          <ul>
            <li><code>extractLogprobs()</code>: Retrieves logits via <code>llama.GetLogitsIth()</code></li>
            <li><code>logSoftmax()</code>: Numerically stable log-softmax using log-sum-exp trick</li>
            <li><code>getTopKLogprobs()</code>: Uses min-heap for efficient O(n log k) top-k extraction</li>
          </ul>
          <p><strong>Critical:</strong> Logprobs must be extracted <strong>before</strong> <code>llama.SamplerAccept()</code> is called.</p>
          <h3 id="158-api-handler-notes">15.8 API Handler Notes</h3>
          <p><strong>Input Format Conversion</strong> (<code>cmd/server/app/domain/</code>):</p>
          <p>Both streaming and non-streaming Response APIs must call</p>
          <p><code>convertInputToMessages(d)</code> to handle the OpenAI Responses <code>input</code> field</p>
          <p>format.</p>
          <h3 id="159-reference-threads">15.9 Reference Threads</h3>
          <p>See <code>THREADS.md</code> for important past conversations and decisions worth</p>
          <p>preserving.</p>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#table-of-contents" className={`doc-index-header ${activeSection === 'table-of-contents' ? 'active' : ''}`}>Table of Contents</a>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-1:-introduction" className={`doc-index-header ${activeSection === 'chapter-1:-introduction' ? 'active' : ''}`}>Chapter 1: Introduction</a>
              <ul>
                <li><a href="#11-what-is-kronk-model-server" className={activeSection === '11-what-is-kronk-model-server' ? 'active' : ''}>1.1 What is Kronk Model Server</a></li>
                <li><a href="#12-key-features" className={activeSection === '12-key-features' ? 'active' : ''}>1.2 Key Features</a></li>
                <li><a href="#13-supported-platforms-and-hardware" className={activeSection === '13-supported-platforms-and-hardware' ? 'active' : ''}>1.3 Supported Platforms and Hardware</a></li>
                <li><a href="#14-architecture-overview" className={activeSection === '14-architecture-overview' ? 'active' : ''}>1.4 Architecture Overview</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-2:-installation-quick-start" className={`doc-index-header ${activeSection === 'chapter-2:-installation-quick-start' ? 'active' : ''}`}>Chapter 2: Installation &amp; Quick Start</a>
              <ul>
                <li><a href="#21-prerequisites" className={activeSection === '21-prerequisites' ? 'active' : ''}>2.1 Prerequisites</a></li>
                <li><a href="#22-installing-the-cli" className={activeSection === '22-installing-the-cli' ? 'active' : ''}>2.2 Installing the CLI</a></li>
                <li><a href="#23-installing-libraries" className={activeSection === '23-installing-libraries' ? 'active' : ''}>2.3 Installing Libraries</a></li>
                <li><a href="#24-downloading-your-first-model" className={activeSection === '24-downloading-your-first-model' ? 'active' : ''}>2.4 Downloading Your First Model</a></li>
                <li><a href="#25-starting-the-server" className={activeSection === '25-starting-the-server' ? 'active' : ''}>2.5 Starting the Server</a></li>
                <li><a href="#26-verifying-the-installation" className={activeSection === '26-verifying-the-installation' ? 'active' : ''}>2.6 Verifying the Installation</a></li>
                <li><a href="#27-quick-start-summary" className={activeSection === '27-quick-start-summary' ? 'active' : ''}>2.7 Quick Start Summary</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-3:-model-configuration" className={`doc-index-header ${activeSection === 'chapter-3:-model-configuration' ? 'active' : ''}`}>Chapter 3: Model Configuration</a>
              <ul>
                <li><a href="#31-basic-configuration" className={activeSection === '31-basic-configuration' ? 'active' : ''}>3.1 Basic Configuration</a></li>
                <li><a href="#32-sampling-parameters" className={activeSection === '32-sampling-parameters' ? 'active' : ''}>3.2 Sampling Parameters</a></li>
                <li><a href="#33-gpu-configuration" className={activeSection === '33-gpu-configuration' ? 'active' : ''}>3.3 GPU Configuration</a></li>
                <li><a href="#34-kv-cache-quantization" className={activeSection === '34-kv-cache-quantization' ? 'active' : ''}>3.4 KV Cache Quantization</a></li>
                <li><a href="#35-flash-attention" className={activeSection === '35-flash-attention' ? 'active' : ''}>3.5 Flash Attention</a></li>
                <li><a href="#36-parallel-inference-nseqmax" className={activeSection === '36-parallel-inference-nseqmax' ? 'active' : ''}>3.6 Parallel Inference (NSeqMax)</a></li>
                <li><a href="#37-vram-estimation" className={activeSection === '37-vram-estimation' ? 'active' : ''}>3.7 VRAM Estimation</a></li>
                <li><a href="#38-model-config-file-example" className={activeSection === '38-model-config-file-example' ? 'active' : ''}>3.8 Model Config File Example</a></li>
                <li><a href="#39-model-specific-tuning" className={activeSection === '39-model-specific-tuning' ? 'active' : ''}>3.9 Model-Specific Tuning</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-4:-batch-processing" className={`doc-index-header ${activeSection === 'chapter-4:-batch-processing' ? 'active' : ''}`}>Chapter 4: Batch Processing</a>
              <ul>
                <li><a href="#41-architecture-overview" className={activeSection === '41-architecture-overview' ? 'active' : ''}>4.1 Architecture Overview</a></li>
                <li><a href="#42-slots-and-sequences" className={activeSection === '42-slots-and-sequences' ? 'active' : ''}>4.2 Slots and Sequences</a></li>
                <li><a href="#43-request-flow" className={activeSection === '43-request-flow' ? 'active' : ''}>4.3 Request Flow</a></li>
                <li><a href="#44-configuring-batch-processing" className={activeSection === '44-configuring-batch-processing' ? 'active' : ''}>4.4 Configuring Batch Processing</a></li>
                <li><a href="#45-batch-vs-sequential-models" className={activeSection === '45-batch-vs-sequential-models' ? 'active' : ''}>4.5 Batch vs Sequential Models</a></li>
                <li><a href="#46-performance-tuning" className={activeSection === '46-performance-tuning' ? 'active' : ''}>4.6 Performance Tuning</a></li>
                <li><a href="#47-example-configuration" className={activeSection === '47-example-configuration' ? 'active' : ''}>4.7 Example Configuration</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-5:-message-caching" className={`doc-index-header ${activeSection === 'chapter-5:-message-caching' ? 'active' : ''}`}>Chapter 5: Message Caching</a>
              <ul>
                <li><a href="#51-overview" className={activeSection === '51-overview' ? 'active' : ''}>5.1 Overview</a></li>
                <li><a href="#52-system-prompt-cache-spc" className={activeSection === '52-system-prompt-cache-spc' ? 'active' : ''}>5.2 System Prompt Cache (SPC)</a></li>
                <li><a href="#53-incremental-message-cache-imc" className={activeSection === '53-incremental-message-cache-imc' ? 'active' : ''}>5.3 Incremental Message Cache (IMC)</a></li>
                <li><a href="#54-multi-user-caching" className={activeSection === '54-multi-user-caching' ? 'active' : ''}>5.4 Multi-User Caching</a></li>
                <li><a href="#55-spc-vs-imc" className={activeSection === '55-spc-vs-imc' ? 'active' : ''}>5.5 SPC vs IMC</a></li>
                <li><a href="#56-cache-invalidation" className={activeSection === '56-cache-invalidation' ? 'active' : ''}>5.6 Cache Invalidation</a></li>
                <li><a href="#57-configuration-reference" className={activeSection === '57-configuration-reference' ? 'active' : ''}>5.7 Configuration Reference</a></li>
                <li><a href="#58-context-window-auto-scaling-imc-only" className={activeSection === '58-context-window-auto-scaling-imc-only' ? 'active' : ''}>5.8 Context Window Auto-Scaling (IMC Only)</a></li>
                <li><a href="#59-performance-and-limitations" className={activeSection === '59-performance-and-limitations' ? 'active' : ''}>5.9 Performance and Limitations</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-6:-yarn-extended-context" className={`doc-index-header ${activeSection === 'chapter-6:-yarn-extended-context' ? 'active' : ''}`}>Chapter 6: YaRN Extended Context</a>
              <ul>
                <li><a href="#61-understanding-context-extension" className={activeSection === '61-understanding-context-extension' ? 'active' : ''}>6.1 Understanding Context Extension</a></li>
                <li><a href="#62-when-to-use-yarn" className={activeSection === '62-when-to-use-yarn' ? 'active' : ''}>6.2 When to Use YaRN</a></li>
                <li><a href="#63-configuration" className={activeSection === '63-configuration' ? 'active' : ''}>6.3 Configuration</a></li>
                <li><a href="#64-scaling-types" className={activeSection === '64-scaling-types' ? 'active' : ''}>6.4 Scaling Types</a></li>
                <li><a href="#65-parameter-reference" className={activeSection === '65-parameter-reference' ? 'active' : ''}>6.5 Parameter Reference</a></li>
                <li><a href="#66-model-specific-examples" className={activeSection === '66-model-specific-examples' ? 'active' : ''}>6.6 Model-Specific Examples</a></li>
                <li><a href="#67-memory-impact" className={activeSection === '67-memory-impact' ? 'active' : ''}>6.7 Memory Impact</a></li>
                <li><a href="#68-quality-considerations" className={activeSection === '68-quality-considerations' ? 'active' : ''}>6.8 Quality Considerations</a></li>
                <li><a href="#69-example:-long-document-processing" className={activeSection === '69-example:-long-document-processing' ? 'active' : ''}>6.9 Example: Long Document Processing</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-7:-model-server" className={`doc-index-header ${activeSection === 'chapter-7:-model-server' ? 'active' : ''}`}>Chapter 7: Model Server</a>
              <ul>
                <li><a href="#71-starting-the-server" className={activeSection === '71-starting-the-server' ? 'active' : ''}>7.1 Starting the Server</a></li>
                <li><a href="#72-stopping-the-server" className={activeSection === '72-stopping-the-server' ? 'active' : ''}>7.2 Stopping the Server</a></li>
                <li><a href="#73-server-configuration" className={activeSection === '73-server-configuration' ? 'active' : ''}>7.3 Server Configuration</a></li>
                <li><a href="#74-model-caching" className={activeSection === '74-model-caching' ? 'active' : ''}>7.4 Model Caching</a></li>
                <li><a href="#75-model-config-files" className={activeSection === '75-model-config-files' ? 'active' : ''}>7.5 Model Config Files</a></li>
                <li><a href="#76-catalog-system" className={activeSection === '76-catalog-system' ? 'active' : ''}>7.6 Catalog System</a></li>
                <li><a href="#77-custom-catalog-repository" className={activeSection === '77-custom-catalog-repository' ? 'active' : ''}>7.7 Custom Catalog Repository</a></li>
                <li><a href="#78-templates" className={activeSection === '78-templates' ? 'active' : ''}>7.8 Templates</a></li>
                <li><a href="#79-runtime-settings" className={activeSection === '79-runtime-settings' ? 'active' : ''}>7.9 Runtime Settings</a></li>
                <li><a href="#710-logging" className={activeSection === '710-logging' ? 'active' : ''}>7.10 Logging</a></li>
                <li><a href="#711-data-paths" className={activeSection === '711-data-paths' ? 'active' : ''}>7.11 Data Paths</a></li>
                <li><a href="#712-complete-example" className={activeSection === '712-complete-example' ? 'active' : ''}>7.12 Complete Example</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-8:-api-endpoints" className={`doc-index-header ${activeSection === 'chapter-8:-api-endpoints' ? 'active' : ''}`}>Chapter 8: API Endpoints</a>
              <ul>
                <li><a href="#81-endpoint-overview" className={activeSection === '81-endpoint-overview' ? 'active' : ''}>8.1 Endpoint Overview</a></li>
                <li><a href="#82-chat-completions" className={activeSection === '82-chat-completions' ? 'active' : ''}>8.2 Chat Completions</a></li>
                <li><a href="#83-responses-api" className={activeSection === '83-responses-api' ? 'active' : ''}>8.3 Responses API</a></li>
                <li><a href="#84-embeddings" className={activeSection === '84-embeddings' ? 'active' : ''}>8.4 Embeddings</a></li>
                <li><a href="#85-reranking" className={activeSection === '85-reranking' ? 'active' : ''}>8.5 Reranking</a></li>
                <li><a href="#86-tool-calling-function-calling" className={activeSection === '86-tool-calling-function-calling' ? 'active' : ''}>8.6 Tool Calling (Function Calling)</a></li>
                <li><a href="#87-logprobs-token-probabilities" className={activeSection === '87-logprobs-token-probabilities' ? 'active' : ''}>8.7 Logprobs (Token Probabilities)</a></li>
                <li><a href="#88-models-list" className={activeSection === '88-models-list' ? 'active' : ''}>8.8 Models List</a></li>
                <li><a href="#89-using-cache-id-with-api-requests" className={activeSection === '89-using-cache-id-with-api-requests' ? 'active' : ''}>8.9 Using Cache ID with API Requests</a></li>
                <li><a href="#810-authentication" className={activeSection === '810-authentication' ? 'active' : ''}>8.10 Authentication</a></li>
                <li><a href="#811-error-responses" className={activeSection === '811-error-responses' ? 'active' : ''}>8.11 Error Responses</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-9:-multi-modal-models" className={`doc-index-header ${activeSection === 'chapter-9:-multi-modal-models' ? 'active' : ''}`}>Chapter 9: Multi-Modal Models</a>
              <ul>
                <li><a href="#91-overview" className={activeSection === '91-overview' ? 'active' : ''}>9.1 Overview</a></li>
                <li><a href="#92-vision-models" className={activeSection === '92-vision-models' ? 'active' : ''}>9.2 Vision Models</a></li>
                <li><a href="#93-audio-models" className={activeSection === '93-audio-models' ? 'active' : ''}>9.3 Audio Models</a></li>
                <li><a href="#94-plain-base64-format" className={activeSection === '94-plain-base64-format' ? 'active' : ''}>9.4 Plain Base64 Format</a></li>
                <li><a href="#95-configuration-for-multi-modal-models" className={activeSection === '95-configuration-for-multi-modal-models' ? 'active' : ''}>9.5 Configuration for Multi-Modal Models</a></li>
                <li><a href="#96-memory-requirements" className={activeSection === '96-memory-requirements' ? 'active' : ''}>9.6 Memory Requirements</a></li>
                <li><a href="#97-limitations" className={activeSection === '97-limitations' ? 'active' : ''}>9.7 Limitations</a></li>
                <li><a href="#98-example:-image-analysis" className={activeSection === '98-example:-image-analysis' ? 'active' : ''}>9.8 Example: Image Analysis</a></li>
                <li><a href="#99-example:-audio-transcription" className={activeSection === '99-example:-audio-transcription' ? 'active' : ''}>9.9 Example: Audio Transcription</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-10:-security-authentication" className={`doc-index-header ${activeSection === 'chapter-10:-security-authentication' ? 'active' : ''}`}>Chapter 10: Security &amp; Authentication</a>
              <ul>
                <li><a href="#101-enabling-authentication" className={activeSection === '101-enabling-authentication' ? 'active' : ''}>10.1 Enabling Authentication</a></li>
                <li><a href="#102-using-the-admin-token" className={activeSection === '102-using-the-admin-token' ? 'active' : ''}>10.2 Using the Admin Token</a></li>
                <li><a href="#103-key-management" className={activeSection === '103-key-management' ? 'active' : ''}>10.3 Key Management</a></li>
                <li><a href="#104-creating-user-tokens" className={activeSection === '104-creating-user-tokens' ? 'active' : ''}>10.4 Creating User Tokens</a></li>
                <li><a href="#105-token-examples" className={activeSection === '105-token-examples' ? 'active' : ''}>10.5 Token Examples</a></li>
                <li><a href="#106-using-tokens-in-api-requests" className={activeSection === '106-using-tokens-in-api-requests' ? 'active' : ''}>10.6 Using Tokens in API Requests</a></li>
                <li><a href="#107-authorization-flow" className={activeSection === '107-authorization-flow' ? 'active' : ''}>10.7 Authorization Flow</a></li>
                <li><a href="#108-rate-limiting" className={activeSection === '108-rate-limiting' ? 'active' : ''}>10.8 Rate Limiting</a></li>
                <li><a href="#109-configuration-reference" className={activeSection === '109-configuration-reference' ? 'active' : ''}>10.9 Configuration Reference</a></li>
                <li><a href="#1010-security-best-practices" className={activeSection === '1010-security-best-practices' ? 'active' : ''}>10.10 Security Best Practices</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-11:-browser-ui-bui" className={`doc-index-header ${activeSection === 'chapter-11:-browser-ui-bui' ? 'active' : ''}`}>Chapter 11: Browser UI (BUI)</a>
              <ul>
                <li><a href="#111-accessing-the-bui" className={activeSection === '111-accessing-the-bui' ? 'active' : ''}>11.1 Accessing the BUI</a></li>
                <li><a href="#112-downloading-libraries" className={activeSection === '112-downloading-libraries' ? 'active' : ''}>11.2 Downloading Libraries</a></li>
                <li><a href="#113-downloading-models" className={activeSection === '113-downloading-models' ? 'active' : ''}>11.3 Downloading Models</a></li>
                <li><a href="#114-managing-keys-and-tokens" className={activeSection === '114-managing-keys-and-tokens' ? 'active' : ''}>11.4 Managing Keys and Tokens</a></li>
                <li><a href="#115-other-screens" className={activeSection === '115-other-screens' ? 'active' : ''}>11.5 Other Screens</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-12:-client-integration" className={`doc-index-header ${activeSection === 'chapter-12:-client-integration' ? 'active' : ''}`}>Chapter 12: Client Integration</a>
              <ul>
                <li><a href="#121-openwebui" className={activeSection === '121-openwebui' ? 'active' : ''}>12.1 OpenWebUI</a></li>
                <li><a href="#122-cline" className={activeSection === '122-cline' ? 'active' : ''}>12.2 Cline</a></li>
                <li><a href="#124-python-openai-sdk" className={activeSection === '124-python-openai-sdk' ? 'active' : ''}>12.4 Python OpenAI SDK</a></li>
                <li><a href="#125-curl-and-http-clients" className={activeSection === '125-curl-and-http-clients' ? 'active' : ''}>12.5 curl and HTTP Clients</a></li>
                <li><a href="#126-langchain" className={activeSection === '126-langchain' ? 'active' : ''}>12.6 LangChain</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-13:-observability" className={`doc-index-header ${activeSection === 'chapter-13:-observability' ? 'active' : ''}`}>Chapter 13: Observability</a>
              <ul>
                <li><a href="#131-debug-server" className={activeSection === '131-debug-server' ? 'active' : ''}>13.1 Debug Server</a></li>
                <li><a href="#132-debug-endpoints" className={activeSection === '132-debug-endpoints' ? 'active' : ''}>13.2 Debug Endpoints</a></li>
                <li><a href="#133-health-check-endpoints" className={activeSection === '133-health-check-endpoints' ? 'active' : ''}>13.3 Health Check Endpoints</a></li>
                <li><a href="#134-prometheus-metrics" className={activeSection === '134-prometheus-metrics' ? 'active' : ''}>13.4 Prometheus Metrics</a></li>
                <li><a href="#135-prometheus-integration" className={activeSection === '135-prometheus-integration' ? 'active' : ''}>13.5 Prometheus Integration</a></li>
                <li><a href="#136-distributed-tracing-with-tempo" className={activeSection === '136-distributed-tracing-with-tempo' ? 'active' : ''}>13.6 Distributed Tracing with Tempo</a></li>
                <li><a href="#137-tracing-architecture" className={activeSection === '137-tracing-architecture' ? 'active' : ''}>13.7 Tracing Architecture</a></li>
                <li><a href="#138-tempo-setup-with-docker" className={activeSection === '138-tempo-setup-with-docker' ? 'active' : ''}>13.8 Tempo Setup with Docker</a></li>
                <li><a href="#139-pprof-profiling" className={activeSection === '139-pprof-profiling' ? 'active' : ''}>13.9 pprof Profiling</a></li>
                <li><a href="#1310-statsviz-real-time-monitoring" className={activeSection === '1310-statsviz-real-time-monitoring' ? 'active' : ''}>13.10 Statsviz Real-Time Monitoring</a></li>
                <li><a href="#1311-logging" className={activeSection === '1311-logging' ? 'active' : ''}>13.11 Logging</a></li>
                <li><a href="#1312-configuration-reference" className={activeSection === '1312-configuration-reference' ? 'active' : ''}>13.12 Configuration Reference</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-14:-troubleshooting" className={`doc-index-header ${activeSection === 'chapter-14:-troubleshooting' ? 'active' : ''}`}>Chapter 14: Troubleshooting</a>
              <ul>
                <li><a href="#141-library-issues" className={activeSection === '141-library-issues' ? 'active' : ''}>14.1 Library Issues</a></li>
                <li><a href="#142-model-loading-failures" className={activeSection === '142-model-loading-failures' ? 'active' : ''}>14.2 Model Loading Failures</a></li>
                <li><a href="#143-memory-errors" className={activeSection === '143-memory-errors' ? 'active' : ''}>14.3 Memory Errors</a></li>
                <li><a href="#144-request-timeouts" className={activeSection === '144-request-timeouts' ? 'active' : ''}>14.4 Request Timeouts</a></li>
                <li><a href="#145-authentication-errors" className={activeSection === '145-authentication-errors' ? 'active' : ''}>14.5 Authentication Errors</a></li>
                <li><a href="#146-streaming-issues" className={activeSection === '146-streaming-issues' ? 'active' : ''}>14.6 Streaming Issues</a></li>
                <li><a href="#147-performance-issues" className={activeSection === '147-performance-issues' ? 'active' : ''}>14.7 Performance Issues</a></li>
                <li><a href="#148-viewing-logs" className={activeSection === '148-viewing-logs' ? 'active' : ''}>14.8 Viewing Logs</a></li>
                <li><a href="#149-common-error-messages" className={activeSection === '149-common-error-messages' ? 'active' : ''}>14.9 Common Error Messages</a></li>
                <li><a href="#1410-getting-help" className={activeSection === '1410-getting-help' ? 'active' : ''}>14.10 Getting Help</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-15:-developer-guide" className={`doc-index-header ${activeSection === 'chapter-15:-developer-guide' ? 'active' : ''}`}>Chapter 15: Developer Guide</a>
              <ul>
                <li><a href="#151-quick-reference" className={activeSection === '151-quick-reference' ? 'active' : ''}>15.1 Quick Reference</a></li>
                <li><a href="#152-build-test-commands" className={activeSection === '152-build-test-commands' ? 'active' : ''}>15.2 Build &amp; Test Commands</a></li>
                <li><a href="#153-developer-setup" className={activeSection === '153-developer-setup' ? 'active' : ''}>15.3 Developer Setup</a></li>
                <li><a href="#154-project-architecture" className={activeSection === '154-project-architecture' ? 'active' : ''}>15.4 Project Architecture</a></li>
                <li><a href="#155-bui-frontend-development" className={activeSection === '155-bui-frontend-development' ? 'active' : ''}>15.5 BUI Frontend Development</a></li>
                <li><a href="#156-code-style-guidelines" className={activeSection === '156-code-style-guidelines' ? 'active' : ''}>15.6 Code Style Guidelines</a></li>
                <li><a href="#157-sdk-internals" className={activeSection === '157-sdk-internals' ? 'active' : ''}>15.7 SDK Internals</a></li>
                <li><a href="#158-api-handler-notes" className={activeSection === '158-api-handler-notes' ? 'active' : ''}>15.8 API Handler Notes</a></li>
                <li><a href="#159-reference-threads" className={activeSection === '159-reference-threads' ? 'active' : ''}>15.9 Reference Threads</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
