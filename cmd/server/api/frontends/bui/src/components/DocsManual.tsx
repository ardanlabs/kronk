import { useState, useEffect } from 'react';
import { useLocation } from 'react-router-dom';

export default function DocsManual() {
  const [activeSection, setActiveSection] = useState('');
  const location = useLocation();

  useEffect(() => {
    if (location.hash) {
      const id = location.hash.slice(1);
      const scrollToElement = () => {
        const element = document.getElementById(id);
        const container = document.querySelector('.main-content');
        if (element && container) {
          const containerRect = container.getBoundingClientRect();
          const elementRect = element.getBoundingClientRect();
          const offset = elementRect.top - containerRect.top + container.scrollTop;
          container.scrollTo({ top: offset - 20, behavior: 'smooth' });
        }
      };
      requestAnimationFrame(scrollToElement);
    }
  }, [location.key]);

  useEffect(() => {
    const container = document.querySelector('.main-content');
    if (!container) return;

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

    container.addEventListener('scroll', handleScroll);
    return () => container.removeEventListener('scroll', handleScroll);
  }, []);

  useEffect(() => {
    if (activeSection) {
      const activeLink = document.querySelector('.doc-sidebar a[href="#' + activeSection + '"]');
      if (activeLink) {
        const sidebar = document.querySelector('.doc-sidebar');
        if (sidebar) {
          const sidebarRect = sidebar.getBoundingClientRect();
          const linkRect = activeLink.getBoundingClientRect();
          const offset = linkRect.top - sidebarRect.top + sidebar.scrollTop - 20;
          sidebar.scrollTo({ top: offset, behavior: 'smooth' });
        }
      }
    }
  }, [activeSection]);

  return (
    <div>
      <div className="page-header">
        <h2>Kronk Manual</h2>
        <p>Complete documentation for the Kronk Model Server</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content manual-content">
          <h2 id="chapter-1-introduction">Chapter 1: Introduction</h2>
          <h3 id="11-what-is-kronk">1.1 What is Kronk</h3>
          <p>Kronk is a Go SDK and Model Server for running local inference with open-source GGUF models. Built on top of llama.cpp via the <a href="https://github.com/hybridgroup/yzma">yzma</a> Go bindings (a non-CGO FFI layer), Kronk provides hardware-accelerated inference for text generation, vision, audio, embeddings, and reranking. Kronk is being designed to be your personal engine for running open source models locally.</p>
          <p><strong>The SDK is the foundation.</strong></p>
          <p>The Kronk Model Server is built entirely on top of the SDK — we "dog food" our own library. Everything the model server can do is available to you as a SDK developer to help you write your own applications.</p>
          <p><strong>You don't need a model server.</strong></p>
          <p>The real power of Kronk is that you can embed model inference directly into your Go applications. Load models, run inference, manage caching, and handle concurrent requests — all without running the models in a separate server process. The <a href="sdk/examples">examples</a> directory demonstrates building standalone applications with the SDK.</p>
          <p><strong>The Model Server is optional.</strong></p>
          <p>When you do need an model server (for web UIs, multi-client access, or OpenAI-compatible endpoints), the Kronk Model Server provides:</p>
          <ul>
            <li>OpenAI and Anthropic compatible REST APIs</li>
            <li>OpenWebUI integration</li>
            <li>Agent and tool support for local models</li>
            <li>Any OpenAI-compatible client</li>
          </ul>
          <h3 id="12-key-features">1.2 Key Features</h3>
          <p><strong>Model Types</strong></p>
          <ul>
            <li><strong>Text Generation</strong> - Chat completions and streaming responses with reasoning support.</li>
            <li><strong>Vision</strong> - Image understanding and analysis.</li>
            <li><strong>Audio</strong> - Speech-to-text and audio understanding.</li>
            <li><strong>Embeddings</strong> - Vector embeddings for semantic search and RAG.</li>
            <li><strong>Reranking</strong> - Document relevance scoring.</li>
          </ul>
          <p><strong>Performance</strong></p>
          <ul>
            <li><strong>Batch Processing</strong> - Process multiple requests concurrently within a set of partitioned KV cache sequences.</li>
            <li><strong>Message Caching</strong> - System prompt and incremental message caching to reduce redundant computation.</li>
            <li><strong>YaRN Context Extension</strong> - Extend context windows 2-4x beyond native training length.</li>
            <li><strong>Model Pooling</strong> - Keep a number of models loaded in memory with configurable TTL.</li>
          </ul>
          <p><strong>Operations</strong></p>
          <ul>
            <li><strong>Catalog System</strong> - Your personal catalog of downloaded models, seeded with a starter list and managed via the CLI and BUI.</li>
            <li><strong>Browser UI (BUI)</strong> - Web interface for model management, downloads, and configuration.</li>
            <li><strong>Authentication</strong> - JWT-based security with key management, endpoint authorization and rate limiting.</li>
            <li><strong>Observability</strong> - Tracing and metrics integration with Grafana support.</li>
            <li><strong>Local Storage</strong> - Everything Kronk manages — catalog, downloaded models, llama.cpp libraries, and per-model configuration — lives under <code>~/.kronk/</code>.</li>
          </ul>
          <h3 id="13-supported-platforms-and-hardware">1.3 Supported Platforms and Hardware</h3>
          <p>Kronk supports full hardware acceleration across major platforms:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th><strong>OS</strong></th>
                <th><strong>CPU</strong></th>
                <th><strong>GPU</strong></th>
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
          <p>Kronk is designed as a layered architecture where the SDK provides all core functionality and the Model Server is one application built on top of it.</p>
          <p><img src="https://github.com/ardanlabs/kronk/blob/main/images/design/sdk.png?raw=true" alt="Kronk SDK Architecture" /></p>
          <p><strong>Layer Breakdown:</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Layer</th>
                <th>Component</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><strong>Application</strong></td>
                <td>Kronk Model Server</td>
                <td>REST API server (or your own app)</td>
              </tr>
              <tr>
                <td><strong>SDK Tools</strong></td>
                <td>Models, Libs, Downloader, Devices</td>
                <td>High-level APIs for common tasks</td>
              </tr>
              <tr>
                <td><strong>SDK Core</strong></td>
                <td>Kronk SDK API, Model SDK API</td>
                <td>Model loading, inference, pooling, caching</td>
              </tr>
              <tr>
                <td><strong>Bindings</strong></td>
                <td>yzma (non-CGO FFI via purego)</td>
                <td>Go bindings to llama.cpp without CGO</td>
              </tr>
              <tr>
                <td><strong>Engine</strong></td>
                <td>llama.cpp</td>
                <td>Hardware-accelerated inference</td>
              </tr>
              <tr>
                <td><strong>Hardware</strong></td>
                <td>Metal, CUDA, Vulkan, CPU</td>
                <td>GPU/CPU acceleration</td>
              </tr>
            </tbody>
          </table>
          <p>Your application sits at the same level as the Kronk Model Server. You have access to the exact same SDK APIs. Whether you're building a CLI tool, a web service, an embedded system, or a desktop app — you get the full power of local model inference without any server overhead.</p>
          <p><strong>SDK vs Server Usage:</strong></p>
          <pre className="code-block"><code className="language-go">{`// Direct SDK usage - no server needed
cfg := model.Config{
    ModelFiles: modelPath.ModelFiles,
    CacheTypeK: model.GGMLTypeQ8_0,
    CacheTypeV: model.GGMLTypeQ8_0,
}

krn, _ := kronk.New(cfg)
defer krn.Unload(ctx)

ch, _ := krn.ChatStreaming(ctx, model.D{
    "messages":   model.DocumentArray(model.TextMessage(model.RoleUser, "Hello")),
    "max_tokens": 2048,
})

for resp := range ch {
    fmt.Print(resp.Choice[0].Delta.Content)
}`}</code></pre>
          <pre className="code-block"><code className="language-shell">{`# Or use the Model Server for OpenAI-compatible API.
# Server-side per-model tuning lives in ~/.kronk/model_config.yaml.
kronk server start
curl http://localhost:11435/v1/chat/completions -d '{"model":"Qwen3-0.6B-Q8_0","messages":[...]}'`}</code></pre>
          <hr />
          <h2 id="chapter-2-installation-quick-start">Chapter 2: Installation &amp; Quick Start</h2>
          <h3 id="21-prerequisites">2.1 Prerequisites</h3>
          <p><strong>Required</strong></p>
          <ul>
            <li>Go 1.26 or later</li>
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
          <pre className="code-block"><code>{`KRONK
Local LLM inference with hardware acceleration

USAGE
  kronk [command]

COMMANDS
  server    Start/stop the model server
  model     Manage local models (list, pull, remove, show, ps)
  catalog   Browse and manage the model catalog (list, show, remove)
  libs      Install/upgrade llama.cpp libraries
  security  Manage API keys and JWT tokens
  run       Run a model directly for interactive chat (no server needed)

QUICK START
  # List entries in the catalog
  kronk catalog list --local

  # Download a model (e.g., Qwen3-8B)
  kronk model pull Qwen3-0.6B-Q8_0 --local

  # Start the server (runs on http://localhost:11435)
  kronk server start

  # Open the Browser UI
  open http://localhost:11435

FEATURES
  • Text, Vision, Audio, Embeddings, Reranking
  • Metal, CUDA, ROCm, Vulkan, CPU acceleration
  • Batch processing, message caching, YaRN context extension
  • Model pooling, catalog system, browser UI
  • MCP service, security, observability

MODES
  Web mode (default)  - Communicates with running server at localhost:11435
  Local mode (--local) - Direct file operations without server

ENVIRONMENT
  KRONK_BASE_PATH, KRONK_PROCESSOR, KRONK_LIB_VERSION
  KRONK_HF_TOKEN, KRONK_WEB_API_HOST, KRONK_TOKEN

FOR MORE
  kronk <command> --help    Get help for a command
  See AGENTS.md for documentation

Usage:
  kronk [flags]
  kronk [command]

Available Commands:
  catalog     Browse and manage the model catalog (list, show, remove)
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  libs        Install or upgrade llama.cpp libraries
  model       Manage local models (index, list, pull, remove, show, ps)
  run         Run an interactive chat session with a model
  security    Manage API security (keys and tokens)
  server      Start, stop, and manage the Kronk model server

Flags:
      --base-path string   Base path for kronk data (models, libraries, catalog, model_config)
  -h, --help               help for kronk
  -v, --version            version for kronk

Use "kronk [command] --help" for more information about a command.`}</code></pre>
          <h3 id="23-installing-libraries">2.3 Installing Libraries</h3>
          <p>Before running inference, you need the llama.cpp libraries for your machine. Kronk auto-detects your hardware and downloads the appropriate binaries.</p>
          <p><strong>Option A: Via the Server</strong></p>
          <p>Start the server and use the BUI to download libraries:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start`}</code></pre>
          <p>Open http://localhost:11435 in your browser and navigate to the Libraries page.</p>
          <p><strong>Option B: Via CLI</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local`}</code></pre>
          <p>This downloads the <strong>well-known default version</strong> of llama.cpp baked into the SDK and installs it under <code>~/.kronk/libraries/&lt;os&gt;/&lt;arch&gt;/&lt;processor&gt;/</code> using auto-detected settings (for example <code>~/.kronk/libraries/darwin/arm64/metal/</code>). Each <code>(arch, os, processor)</code> triple lives in its own folder so multiple bundles can coexist on the same machine.</p>
          <p>To track and install the <strong>latest</strong> llama.cpp release instead of the default version, opt in with <code>--upgrade</code>:</p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local --upgrade`}</code></pre>
          <blockquote>The standalone CLI defaults to the pinned default version so reinstalls</blockquote>
          <blockquote>are reproducible. The model server takes the opposite default</blockquote>
          <blockquote>(<code>--allow-upgrade=true</code>) so a long-running server picks up upstream</blockquote>
          <blockquote>fixes; see Chapter 7 §7.3 for that flag.</blockquote>
          <p><strong>Pinning a Specific Library Version</strong></p>
          <p>Sometimes there are breaking changes to llama.cpp that require a matching version of yzma and Kronk. To ensure stability, you can install a specific library version:</p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --version=b8864 --local`}</code></pre>
          <p>Or via environment variable:</p>
          <pre className="code-block"><code className="language-shell">{`KRONK_LIB_VERSION=b8864 kronk libs --local`}</code></pre>
          <p>Here are the known compatible versions:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>llama.cpp</th>
                <th>yzma</th>
                <th>kronk</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>b8864</td>
                <td>v1.12.0</td>
                <td>1.23.1</td>
              </tr>
              <tr>
                <td>b8865+</td>
                <td>v1.13.0</td>
                <td>1.23.2</td>
              </tr>
            </tbody>
          </table>
          <p>If you experience unexpected behavior after a library upgrade, pin the version that matches your installed Kronk release using the table above.</p>
          <p><strong>Environment Variables for Library Installation</strong></p>
          <pre className="code-block"><code>{`KRONK_LIB_PATH  - Library directory. See "KRONK_LIB_PATH semantics" below.
KRONK_PROCESSOR - \`cpu\`, \`cuda\`, \`metal\`, \`rocm\`, or \`vulkan\` (default: \`cpu\`)
KRONK_ARCH      - Architecture override: \`amd64\`, \`arm64\`
KRONK_OS        - OS override: \`linux\`, \`darwin\`, \`windows\``}</code></pre>
          <p><strong>KRONK_LIB_PATH semantics</strong></p>
          <p><code>KRONK_LIB_PATH</code> is interpreted in one of three ways:</p>
          <ol>
            <li><em>Unset</em> — the runtime resolves <code>&lt;base&gt;/libraries/&lt;os&gt;/&lt;arch&gt;/&lt;processor&gt;/</code> based on the detected (or <code>KRONK_*</code>-overridden) triple.</li>
            <li><em>Points at a directory containing a &lt;code&gt;version.json&lt;/code&gt;</em> — used as-is. This is the form to set when you want to switch the active install to a previously-downloaded triple folder. Example: ```shell export KRONK_LIB_PATH=~/.kronk/libraries/linux/amd64/cuda ```</li>
            <li><em>Points at a non-empty directory without a &lt;code&gt;version.json&lt;/code&gt;</em> — treated as a user-managed read-only build. Kronk will load libraries from it but never write to it; mutating CLI/HTTP operations against it return an error.</li>
          </ol>
          <p>Switching the active install requires a server restart; libraries are not hot-reloaded.</p>
          <p><strong>Example: Install CUDA Libraries</strong></p>
          <pre className="code-block"><code className="language-shell">{`KRONK_PROCESSOR=cuda kronk libs --local`}</code></pre>
          <p><strong>Installing for Another Triple</strong></p>
          <p>You can also install a bundle for a triple other than the current machine's detected one — useful for prepping a shared filesystem or a target host. The install lands in its own folder under the libraries root and does not touch the active install:</p>
          <pre className="code-block"><code className="language-shell">{`# List every supported (arch, os, processor) combination
kronk libs --list-combinations

# Install the Linux/CUDA bundle alongside whatever is already active
kronk libs --install --arch=amd64 --os=linux --processor=cuda --local

# List installed bundles
kronk libs --list-installs

# Remove an install
kronk libs --remove-install --arch=amd64 --os=linux --processor=cuda --local`}</code></pre>
          <p>In web mode (the default — no <code>--local</code>) the same commands are dispatched through the running server. Activate any installed bundle by exporting <code>KRONK_LIB_PATH</code> to its folder and restarting the server.</p>
          <h3 id="24-downloading-your-first-model">2.4 Downloading Your First Model</h3>
          <p>Kronk maintains your <strong>personal catalog</strong> at <code>~/.kronk/catalog.yaml</code>. On first run it is seeded from an embedded starter list so you have something to choose from immediately; the catalog grows as you pull more models or resolve new IDs against HuggingFace.</p>
          <p>List entries in the catalog:</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list --local`}</code></pre>
          <p>Output:</p>
          <pre className="code-block"><code>{`VAL   MODEL ID                                            PROVIDER    FAMILY                              ARCH      MTMD   SIZE
✓     ggml-org/embeddinggemma-300m-qat-Q8_0               ggml-org    embeddinggemma-300m-qat-q8_0-GGUF   bert      -      329.0 MB
✓     unsloth/Qwen3-0.6B-Q8_0                             unsloth     Qwen3-0.6B-GGUF                     qwen3     -      699.0 MB
✗     bartowski/cerebras_Qwen3-Coder-REAP-25B-A3B-Q8_0    bartowski   Qwen3-Coder-REAP-25B-A3B-GGUF       qwen3moe  -      26.5 GB
✗     unsloth/LFM2.5-VL-1.6B-Q8_0                         unsloth     LFM2.5-VL-1.6B-GGUF                 lfm2      ✓      1.7 GB`}</code></pre>
          <p>The <code>VAL</code> column shows whether the model files have been downloaded and validated locally; <code>MTMD</code> indicates a multimodal projection (mmproj) is present.</p>
          <p>Download a model (recommended starter: Qwen3-0.6B-Q8_0):</p>
          <pre className="code-block"><code className="language-shell">{`kronk model pull Qwen3-0.6B-Q8_0 --local`}</code></pre>
          <p>Models are stored in <code>~/.kronk/models/&lt;provider&gt;/&lt;family&gt;/</code> by default. After the pull completes the catalog entry is updated with the resolved provider, family, revision, and file sizes so subsequent lookups don't need to hit HuggingFace.</p>
          <h3 id="25-starting-the-server">2.5 Starting the Server</h3>
          <p>Start the Kronk Model Server:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start`}</code></pre>
          <p>The server starts on <code>http://localhost:11435</code> by default. You'll see output like:</p>
          <pre className="code-block"><code>{`Kronk Model Server started
API: http://localhost:11435
BUI: http://localhost:11435`}</code></pre>
          <p><strong>Running in Background</strong></p>
          <p>To run the server as a background process:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start -d`}</code></pre>
          <p><strong>Stopping the Server</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server stop`}</code></pre>
          <h3 id="26-model-configuration-file">2.6 Model Configuration File</h3>
          <p>When Kronk starts the server for the first time, it automatically installs a default <code>model_config.yaml</code> file in the <code>~/.kronk/</code> directory. This file controls how each model behaves when loaded by the server — context window size, batch processing, caching, sampling parameters, and more.</p>
          <p><strong>How It Works</strong></p>
          <p>The default configuration is embedded inside the Kronk CLI binary. On first server start, if <code>~/.kronk/model_config.yaml</code> does not already exist, Kronk writes the embedded default to that path. Once the file exists, Kronk never overwrites it — your edits are preserved across upgrades.</p>
          <p>The server logs the path it's using on startup:</p>
          <pre className="code-block"><code>{`startup  status=model config  path=/Users/you/.kronk/model_config.yaml`}</code></pre>
          <p><strong>File Structure</strong></p>
          <p>The file is a YAML document where each top-level key is a model ID (or a model ID with a config variant suffix). Under each key you set the configuration options for that model. Here's a simplified example:</p>
          <pre className="code-block"><code className="language-yaml">{`Qwen/Qwen3-8B-Q8_0:
  context-window: 32768
  sampling-parameters:
    temperature: 0.7
    top_p: 0.8
    top_k: 20

unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M/AGENT:
  context-window: 131072
  nseq-max: 2
  sampling-parameters:
    temperature: 1.0
    top_k: 64
    top_p: 0.95

Qwen/Qwen3-8B-Q8_0/YARN:
  context-window: 131072
  rope-scaling-type: yarn
  yarn-orig-ctx: 32768`}</code></pre>
          <p>The <code>/YARN</code> suffix is a <strong>config variant</strong> — it lets you define multiple configurations for the same model. When making an API request, use the full variant name (e.g., <code>Qwen/Qwen3-8B-Q8_0/YARN</code>) as the <code>model</code> field to select that configuration.</p>
          <p><strong>Available Options</strong></p>
          <p>The file includes a commented reference at the top listing every option. Here are the most commonly used:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Option</th>
                <th>Description</th>
                <th>Default</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>context-window</code></td>
                <td>Max tokens the model can process per request</td>
                <td>8192</td>
              </tr>
              <tr>
                <td><code>ngpu-layers</code></td>
                <td>GPU layers to offload (0 = all, -1 = none)</td>
                <td>0</td>
              </tr>
              <tr>
                <td><code>flash-attention</code></td>
                <td>Flash Attention mode: <code>enabled</code>, <code>disabled</code>, <code>auto</code></td>
                <td>auto</td>
              </tr>
              <tr>
                <td><code>incremental-cache</code></td>
                <td>Enable IMC for agentic workflows</td>
                <td>true</td>
              </tr>
              <tr>
                <td><code>nseq-max</code></td>
                <td>Max parallel sequences for batched inference</td>
                <td>0</td>
              </tr>
              <tr>
                <td><code>nbatch</code></td>
                <td>Logical batch size</td>
                <td>2048</td>
              </tr>
              <tr>
                <td><code>nubatch</code></td>
                <td>Physical batch size for prompt ingestion</td>
                <td>512</td>
              </tr>
              <tr>
                <td><code>cache-type-k</code></td>
                <td>KV cache key quantization: <code>f16</code>, <code>q8_0</code>, <code>q4_0</code>, etc.</td>
                <td>—</td>
              </tr>
              <tr>
                <td><code>cache-type-v</code></td>
                <td>KV cache value quantization</td>
                <td>—</td>
              </tr>
              <tr>
                <td><code>sampling-parameters</code></td>
                <td>Nested block for temperature, top_p, top_k, min_p</td>
                <td>—</td>
              </tr>
            </tbody>
          </table>
          <p>For the complete list of options and detailed explanations, see <a href="chapter-03-model-configuration.md">Chapter 3: Model Configuration</a>.</p>
          <p><strong>Editing the File</strong></p>
          <p>Open the file in any text editor:</p>
          <pre className="code-block"><code className="language-shell">{`# macOS
open ~/.kronk/model_config.yaml

# Linux
nano ~/.kronk/model_config.yaml`}</code></pre>
          <p>After editing, restart the server to apply changes:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server stop
kronk server start`}</code></pre>
          <p><strong>Configuration Priority</strong></p>
          <p>When the server loads a model, configuration is resolved through two layers (plus sampling defaults):</p>
          <ol>
            <li><strong>Analysis defaults</strong> — Hardware-aware values inferred from the GGUF metadata and the local devices (context window, batch sizes, cache types, flash attention, GPU layers).</li>
            <li><strong>&lt;code&gt;model_config.yaml&lt;/code&gt; overrides</strong> — Your per-model overrides merged on top of the analysis defaults. Anything you set here wins.</li>
            <li><strong>Sampling defaults</strong> — Any zero-valued sampling fields are filled in from the SDK's built-in sampling defaults so the model always has a complete sampler configuration.</li>
          </ol>
          <p>The catalog itself is <strong>not</strong> part of this layering — it is a resolution cache (provider, family, revision, files) and not a source of tuning knobs. All tuning lives in <code>model_config.yaml</code> (or in <code>model.Config</code> when you're embedding the SDK directly).</p>
          <p><strong>Tips</strong></p>
          <ul>
            <li>The key is the canonical model id — <code>provider/modelID</code> (for example <code>unsloth/Qwen3-0.6B-Q8_0</code>) or a variant such as <code>unsloth/Qwen3-0.6B-Q8_0/IMC</code> — not a file name.</li>
            <li>Use YAML anchors (<code>&name</code> and <code>&lt;&lt;: *name</code>) to share common settings between variants. The default file includes examples of this pattern.</li>
            <li>The <code>--model-config</code> server flag lets you point to an alternative config file for testing without modifying your main one.</li>
          </ul>
          <h3 id="27-verifying-the-installation">2.7 Verifying the Installation</h3>
          <p><strong>Test via curl</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/models`}</code></pre>
          <p>You should see a list of available models.</p>
          <p><strong>Test Chat Completion</strong></p>
          <p><em>Note: It might take a few seconds the first time you call this because the model needs to be loaded into memory first.</em></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen3-0.6B-Q8_0",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 100
  }'`}</code></pre>
          <p><strong>Test via BUI</strong></p>
          <p>Open <code>http://localhost:11435</code> in your browser and navigate to the <code>Apps/Chat</code> app. Select the model you want to try and chat away.</p>
          <h3 id="28-quick-start-summary">2.8 Quick Start Summary</h3>
          <pre className="code-block"><code className="language-shell">{`# 1. Install Kronk
go install github.com/ardanlabs/kronk/cmd/kronk@latest

# 2. Start the server (auto-installs libraries on first run)
kronk server start

# 3. Open BUI and download a model
open http://localhost:11435

# 4. Download via the BUI Catalog/List screen or use this CLI call
kronk model pull Qwen3-0.6B-Q8_0 --local

# 5. Test the API using this curl call or the BUI App/Chat screen
curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{"model": "Qwen3-0.6B-Q8_0", "messages": [{"role": "user", "content": "Hello!"}]}'`}</code></pre>
          <h3 id="29-nixos-setup">2.9 NixOS Setup</h3>
          <p>NixOS does not follow the Filesystem Hierarchy Standard (FHS), so shared libraries and binaries cannot be found in standard paths like <code>/usr/lib</code>. Kronk requires llama.cpp shared libraries at runtime, which means on NixOS you need to provide them through Nix rather than using the built-in <code>kronk libs</code> downloader.</p>
          <p>A <code>flake.nix</code> is provided in <code>zarf/nix/</code> with dev shells for development and build packages for producing a standalone <code>kronk</code> binary, each per GPU backend.</p>
          <p><strong>Prerequisites</strong></p>
          <ul>
            <li>NixOS or Nix package manager with flakes enabled</li>
            <li>A supported GPU (Vulkan or CUDA), or CPU-only mode</li>
          </ul>
          <p><strong>Available Dev Shells</strong></p>
          <p>The flake provides multiple shells, one per GPU backend:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Command</th>
                <th>Backend</th>
                <th>GPU Required</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>nix develop ./zarf/nix</code></td>
                <td>CPU</td>
                <td>None</td>
              </tr>
              <tr>
                <td><code>nix develop ./zarf/nix#cpu</code></td>
                <td>CPU</td>
                <td>None</td>
              </tr>
              <tr>
                <td><code>nix develop ./zarf/nix#vulkan</code></td>
                <td>Vulkan</td>
                <td>Vulkan-capable GPU</td>
              </tr>
              <tr>
                <td><code>nix develop ./zarf/nix#cuda</code></td>
                <td>CUDA</td>
                <td>NVIDIA GPU with CUDA</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Building the Kronk CLI</strong></p>
          <p>The flake also provides build packages that produce a wrapped <code>kronk</code> binary with the correct llama.cpp backend and runtime libraries baked in:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Command</th>
                <th>Backend</th>
                <th>GPU Required</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>nix build ./zarf/nix</code></td>
                <td>CPU</td>
                <td>None</td>
              </tr>
              <tr>
                <td><code>nix build ./zarf/nix#cpu</code></td>
                <td>CPU</td>
                <td>None</td>
              </tr>
              <tr>
                <td><code>nix build ./zarf/nix#vulkan</code></td>
                <td>Vulkan</td>
                <td>Vulkan-capable GPU</td>
              </tr>
              <tr>
                <td><code>nix build ./zarf/nix#cuda</code></td>
                <td>CUDA</td>
                <td>NVIDIA GPU with CUDA</td>
              </tr>
            </tbody>
          </table>
          <p>The Go binary is built once with <code>CGO_ENABLED=0</code>, then wrapped per backend so that <code>KRONK_LIB_PATH</code>, <code>KRONK_ALLOW_UPGRADE</code>, and <code>LD_LIBRARY_PATH</code> are set automatically. No dev shell is required to run the resulting binary.</p>
          <p><strong>Note:</strong> The <code>vendorHash</code> in the flake must be updated whenever <code>go.mod</code> or <code>go.sum</code> changes. Build with a fake hash and Nix will report the correct one.</p>
          <p><strong>Environment Variables</strong></p>
          <p>All shells and built packages automatically set the following:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Variable</th>
                <th>Value</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>KRONK_LIB_PATH</code></td>
                <td>Nix store path to the selected llama.cpp</td>
                <td>Points Kronk to the Nix-managed llama.cpp libraries</td>
              </tr>
              <tr>
                <td><code>KRONK_ALLOW_UPGRADE</code></td>
                <td><code>false</code></td>
                <td>Prevents Kronk from attempting to download libraries</td>
              </tr>
              <tr>
                <td><code>LD_LIBRARY_PATH</code></td>
                <td>Includes <code>libffi</code> and <code>libstdc++</code></td>
                <td>Required for FFI runtime linking</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Important:</strong> Because <code>KRONK_ALLOW_UPGRADE</code> is set to <code>false</code>, the <code>kronk libs</code> command will not attempt to download or overwrite libraries. Library updates are managed through <code>nix flake update</code> instead.</p>
          <p><strong>Troubleshooting</strong></p>
          <ul>
            <li><strong>Library not found errors:</strong> Ensure you are inside the <code>nix develop</code> shell or using a <code>nix build</code> output. The required <code>LD_LIBRARY_PATH</code> and <code>KRONK_LIB_PATH</code> are only set within the shell or the wrapped binary.</li>
            <li><strong>Vulkan not detected:</strong> Verify your GPU drivers are installed at the NixOS system level (<code>hardware.opengl.enable = true</code> and appropriate driver packages in your NixOS configuration).</li>
            <li><strong>Go version mismatch:</strong> The flake pins a specific Go version. If Kronk requires a newer version, update the <code>go_1_26</code> package reference in <code>flake.nix</code>.</li>
            <li><strong>vendorHash mismatch:</strong> After updating Go dependencies, rebuild with a fake hash (e.g. <code>lib.fakeHash</code>) and Nix will print the correct <code>vendorHash</code>.</li>
          </ul>
          <hr />
          <h2 id="chapter-3-model-configuration">Chapter 3: Model Configuration</h2>
          <p>Model configuration controls how Kronk configures models to run inference. There are exactly two ways to configure a model:</p>
          <ol>
            <li><strong>&lt;code&gt;~/.kronk/model_config.yaml&lt;/code&gt;</strong> — per-model overrides keyed by model id, used by the model server and any tool that goes through <code>models.KronkResolvedConfig</code>.</li>
            <li><strong>&lt;code&gt;model.Config&lt;/code&gt; (Go struct)</strong> — passed directly to <code>kronk.New(cfg)</code> when you embed the SDK in your own application.</li>
          </ol>
          <p>The catalog (<code>~/.kronk/catalog.yaml</code>) is <strong>not</strong> a source of tuning knobs — it is a resolution cache (provider, family, revision, files). All tuning described in this chapter lives in <code>model_config.yaml</code> or <code>model.Config</code>.</p>
          <h3 id="31-basic-configuration">3.1 Basic Configuration</h3>
          <p>For most models you will want to touch these basic settings. There are many more which will be presented later. Each model has GGUF metadata that Kronk can read for defaults like setting the context window size when not provided. Kronk also has default settings for things like <code>temperature</code> and <code>top_p</code> when not provided.</p>
          <h4 id="context-window">Context Window</h4>
          <p>The context window is the maximum number of tokens the KV cache can hold, and it's consumed by both input tokens (system prompt, user messages) and output tokens (model responses). Once the cumulative tokens from all inputs and outputs in a session reach the context window limit, the model can't process more without some form of truncation, sliding window, or cache eviction.</p>
          <pre className="code-block"><code className="language-yaml">{`context_window: 8192 # This represent 8192 tokens.`}</code></pre>
          <p><em>Note: A common rule of thumb is that 1 token ≈ 0.75 words (or roughly 4 characters in English). So an 8K context window can handle approximately 6,000 words of combined input and output.</em></p>
          <p>Larger context windows require more VRAM for the KV cache. The actual cost depends on the model's layer count, number of KV heads, head dimension, KV cache data type (f16, q8_0, q4_0), and the number of parallel sequences (slots).</p>
          <p>As a rough guide for a single slot for a given model size:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Context Size</th>
                <th>~7B (q8_0)</th>
                <th>~7B (f16)</th>
                <th>~70B (q8_0)</th>
                <th>~70B (f16)</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>8K</td>
                <td>~0.5 GB</td>
                <td>~1 GB</td>
                <td>~1.25 GB</td>
                <td>~2.5 GB</td>
              </tr>
              <tr>
                <td>32K</td>
                <td>~2 GB</td>
                <td>~4 GB</td>
                <td>~5 GB</td>
                <td>~10 GB</td>
              </tr>
              <tr>
                <td>128K</td>
                <td>~8 GB</td>
                <td>~16 GB</td>
                <td>~20 GB</td>
                <td>~40 GB</td>
              </tr>
            </tbody>
          </table>
          <p>Using q4_0 KV cache quantization can reduce costs further to roughly ¼ of f16.</p>
          <p>128K context typically requires YaRN scaling for models not natively trained at that length.</p>
          <p><em>Note: YaRN is a way to extend the natural size of context windows for small models. Kronk supports YaRN and talked about in Chapter 6.</em></p>
          <h4 id="batch-size-configuration">Batch Size Configuration</h4>
          <p>When you send a prompt to a model, the model doesn't process all your input tokens at once. It breaks them into smaller chunks and processes each chunk through the GPU in a series of steps called forward passes. These two parameters control the size of those chunks:</p>
          <ul>
            <li><code>n_batch</code> - Maximum tokens per decode call (kronk default: 2048)</li>
            <li><code>n_ubatch</code> - GPU compute chunk size within each decode call (kronk default: 512)</li>
          </ul>
          <p><strong>&lt;code&gt;n_batch&lt;/code&gt; is the capacity of the work tray</strong> — the maximum number of tokens you can load onto the tray before handing it to the GPU. When the batch engine is running multiple slots in parallel (NSeqMax &gt; 1), all their tokens share this tray.</p>
          <p><strong>&lt;code&gt;n_ubatch&lt;/code&gt; is the GPU's bite size</strong> — when the tray arrives at the GPU, it doesn't process all the tokens at once. It chews through them in <code>n_ubatch</code>-sized bites. This is a hardware optimization: different GPUs have different optimal bite sizes based on their memory architecture.</p>
          <p><strong>&lt;code&gt;n_ubatch&lt;/code&gt; also controls fair sharing of the tray.</strong> When multiple slots need prefill, the batch engine uses <code>n_ubatch</code> as the round-robin chunk size. It pulls up to <code>n_ubatch</code> tokens from slot 0, then up to <code>n_ubatch</code> from slot 1, then slot 2, and so on — cycling through until the tray is full. This prevents one slot's large prefill from starving the others.</p>
          <p>The flow works like this:</p>
          <ol>
            <li>Add generation tokens from all active slots (1 token each — always fits)</li>
            <li>Round-robin prefill: pull <code>n_ubatch</code> tokens from each prefilling slot in turn until the tray reaches <code>n_batch</code> capacity</li>
            <li>Hand tray to the GPU</li>
            <li>GPU processes the tray in <code>n_ubatch</code>-sized bites</li>
          </ol>
          <p>For example, with 4 prefilling slots, <code>n_batch: 4096</code>, and <code>n_ubatch: 512</code>, each round pulls 512 tokens from S0, then S1, then S2, then S3, then back to S0 — giving each slot 1024 tokens per tray instead of one slot consuming all 4096.</p>
          <p>For example, if you send a 4096-token prompt with <code>n_batch: 2048</code> and <code>n_ubatch: 512</code>, the prompt is split into 2 decode calls of 2048 tokens each. Within each call, the GPU processes 512 tokens at a time — so each call runs 4 compute passes internally. Larger values mean faster prompt processing but use more VRAM. The <code>n_ubatch</code> value must always be less than or equal to <code>n_batch</code>.</p>
          <pre className="code-block"><code className="language-yaml">{`n_batch: 2048 # Work tray capacity (must be ≥ n_ubatch)
n_ubatch: 512 # GPU bite size (must be ≤ n_batch)`}</code></pre>
          <h4 id="recommended-settings-by-workload">Recommended settings by workload</h4>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Workload</th>
                <th>n_batch</th>
                <th>n_ubatch</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Interactive chat (single user)</td>
                <td>512-1024</td>
                <td>512</td>
              </tr>
              <tr>
                <td>Long prompts/RAG</td>
                <td>2048-4096</td>
                <td>512-1024</td>
              </tr>
              <tr>
                <td>Batch inference (multiple prompts)</td>
                <td>2048-4096</td>
                <td>512</td>
              </tr>
              <tr>
                <td>Low VRAM (&lt;8GB)</td>
                <td>512</td>
                <td>256-512</td>
              </tr>
              <tr>
                <td>High VRAM (24GB+)</td>
                <td>4096+</td>
                <td>1024+</td>
              </tr>
            </tbody>
          </table>
          <h3 id="32-processor-selection">3.2 Processor Selection</h3>
          <p>The <strong>processor</strong> determines which hardware backend Kronk uses for inference: CPU, CUDA, Metal, ROCm, or Vulkan. Each processor corresponds to a different build of the llama.cpp shared libraries, so the processor must be resolved <strong>before</strong> libraries are downloaded. Once the wrong libraries are installed, switching processors requires re-downloading them.</p>
          <p>This means processor selection happens early — before <code>libs.New()</code> in the SDK, and before <code>kronk libs</code> or any server startup on the CLI. Each install lands in its own per-triple folder under the libraries root (<code>&lt;base&gt;/libraries/&lt;os&gt;/&lt;arch&gt;/&lt;processor&gt;/</code>), so multiple processor bundles can coexist; switch active install at runtime by exporting <code>KRONK_LIB_PATH</code> to that folder and restarting the server. Everything downstream (model loading, layer offloading, KV cache placement) depends on having the correct libraries for your hardware.</p>
          <h4 id="how-processor-selection-works">How Processor Selection Works</h4>
          <p>Kronk resolves the processor through a two-step priority:</p>
          <ol>
            <li><strong>Environment variable</strong> — If <code>KRONK_PROCESSOR</code> is set (e.g., <code>cpu</code>, <code>cuda</code>, <code>metal</code>, <code>vulkan</code>, <code>rocm</code>), that value is used directly. This gives you explicit control and overrides all auto-detection.</li>
            <li><strong>Auto-detection</strong> — If <code>KRONK_PROCESSOR</code> is not set, Kronk calls <code>DetectGPU()</code> to probe your system for available GPU hardware and selects the best processor automatically.</li>
          </ol>
          <pre className="code-block"><code>{`KRONK_PROCESSOR set?
  ├─ Yes → Use that value
  └─ No  → DetectGPU()
              ├─ CUDA found?   → cuda
              ├─ ROCm found?   → rocm   (Linux only)
              ├─ Vulkan found? → vulkan
              └─ Nothing found → cpu`}</code></pre>
          <p>Auto-detection was introduced in release v1.21.5 so that Kronk selects the best available GPU automatically rather than silently defaulting to CPU. Because hardware configurations vary widely, auto-detection ensures GPU acceleration is enabled when supported — users who need a specific backend can always override via <code>KRONK_PROCESSOR</code>.</p>
          <p>For SDK users, <code>defaults.Processor("")</code> calls <code>DetectGPU()</code> internally. This must be called before library initialization:</p>
          <pre className="code-block"><code className="language-go">{`lbs, err := libs.New(
    libs.WithVersion(defaults.LibVersion("")),
    libs.WithProcessor(defaults.Processor("")),
)`}</code></pre>
          <h4 id="platform-detection-details">Platform Detection Details</h4>
          <p>Detection varies by platform because each operating system exposes GPU information differently.</p>
          <p><strong>macOS (Darwin)</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Architecture</th>
                <th>Result</th>
                <th>Reason</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>ARM64 (Apple Silicon)</td>
                <td><code>metal</code></td>
                <td>Native Metal support via unified memory</td>
              </tr>
              <tr>
                <td>AMD64 (Intel Mac)</td>
                <td><code>cpu</code></td>
                <td>The x64 macOS <code>cpu</code> binary already includes Metal support</td>
              </tr>
            </tbody>
          </table>
          <p>On macOS, GPU detection is straightforward. Apple Silicon machines always use Metal. Intel Macs return <code>cpu</code> because yzma's precompiled Metal libraries are ARM64-only — but the x64 <code>cpu</code> binary already includes Metal acceleration, so Intel Macs still get GPU support through the CPU processor selection.</p>
          <p><strong>Windows</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Priority</th>
                <th>Check</th>
                <th>Processor</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>1</td>
                <td><code>nvidia-smi</code> found</td>
                <td><code>cuda</code></td>
              </tr>
              <tr>
                <td>2</td>
                <td><code>vulkaninfo</code> or <code>vulkan-1.dll</code> present</td>
                <td><code>vulkan</code></td>
              </tr>
              <tr>
                <td>3</td>
                <td>None</td>
                <td><code>cpu</code></td>
              </tr>
            </tbody>
          </table>
          <p><strong>Linux</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Priority</th>
                <th>Check</th>
                <th>Processor</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>1</td>
                <td><code>nvidia-smi</code> found</td>
                <td><code>cuda</code></td>
              </tr>
              <tr>
                <td>2</td>
                <td><code>rocminfo</code> found</td>
                <td><code>rocm</code></td>
              </tr>
              <tr>
                <td>3</td>
                <td><code>vulkaninfo --summary</code> succeeds</td>
                <td><code>vulkan</code></td>
              </tr>
              <tr>
                <td>4</td>
                <td>None</td>
                <td><code>cpu</code></td>
              </tr>
            </tbody>
          </table>
          <h4 id="supported-processors">Supported Processors</h4>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Processor</th>
                <th>Hardware</th>
                <th>Platforms</th>
                <th>Notes</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>metal</code></td>
                <td>Apple Silicon (M1/M2/M3/M4)</td>
                <td>macOS</td>
                <td>Unified memory — CPU and GPU share RAM</td>
              </tr>
              <tr>
                <td><code>cuda</code></td>
                <td>NVIDIA discrete GPUs</td>
                <td>Windows, Linux</td>
                <td>Requires NVIDIA drivers with <code>nvidia-smi</code></td>
              </tr>
              <tr>
                <td><code>rocm</code></td>
                <td>AMD discrete GPUs</td>
                <td>Linux</td>
                <td>Requires ROCm runtime with <code>rocminfo</code></td>
              </tr>
              <tr>
                <td><code>vulkan</code></td>
                <td>Cross-platform GPUs</td>
                <td>Windows, Linux</td>
                <td>Intel, AMD, NVIDIA — including integrated GPUs</td>
              </tr>
              <tr>
                <td><code>cpu</code></td>
                <td>Any</td>
                <td>All</td>
                <td>No GPU acceleration, uses system RAM only</td>
              </tr>
            </tbody>
          </table>
          <h4 id="integrated-gpus-igpus">Integrated GPUs (iGPUs)</h4>
          <p>Machines without a discrete GPU but with an integrated GPU (Intel UHD/Iris, AMD APU) will auto-detect as <code>vulkan</code> if Vulkan drivers are installed.</p>
          <p>_<strong>Warning:</strong> On systems with low RAM (8GB or less) or older integrated GPUs, Vulkan may perform worse than CPU-only inference. Integrated GPUs share system RAM with the CPU, and the overhead of GPU dispatch may outweigh any acceleration benefit. If you suspect this applies to your hardware, benchmark both options and override with <code>KRONK_PROCESSOR=cpu</code> if CPU performs better._</p>
          <h3 id="33-gpu-configuration">3.3 GPU Configuration</h3>
          <p>A model is made up of layers, and each layer contains the weights (numbers) the model learned during training. When you run inference, the model processes your input through these layers one at a time. The key performance question is: where do those layers live — on the GPU or the CPU?</p>
          <p>GPUs are dramatically faster at the math required for inference, but they have limited memory (VRAM). If your model doesn't fit entirely in VRAM, you can split the work: keep some layers on the GPU for speed and let the rest run on the CPU. This section covers how to control that split and other GPU-related settings.</p>
          <h4 id="layer-offloading">Layer Offloading</h4>
          <p>A typical model might have anywhere from 28 to 80+ layers depending on its size. For example, a 7B parameter model usually has around 32 layers, while a 70B model might have 80. Each layer you place on the GPU runs significantly faster, but consumes VRAM. If your GPU doesn't have enough VRAM to hold every layer, you can choose how many to offload — the rest will run on the CPU, which is slower but has access to your full system RAM.</p>
          <p>The goal is to put as many layers on the GPU as your VRAM allows. If you run out of VRAM, lower this number until the model fits.</p>
          <p>Control how many model layers run on GPU:</p>
          <pre className="code-block"><code className="language-yaml">{`n_gpu_layers: 0      # 0 = all layers on GPU (default)
n_gpu_layers: -1     # All layers on CPU
n_gpu_layers: 20     # First 20 layers on GPU`}</code></pre>
          <p><em>Note: On Apple Silicon (Metal), the CPU and GPU share the same unified memory pool, so there is no separate VRAM. All layers run on the GPU by default and this setting does not need to be configured. Layer offloading applies to discrete GPU systems (NVIDIA CUDA, Vulkan).</em></p>
          <h4 id="kv-cache-location">KV Cache Location</h4>
          <p>As the model processes your conversation, it builds up a cache of intermediate calculations called the KV (Key-Value) cache. Think of it as the model's short-term memory — it stores what the model has already "read" so it doesn't have to reprocess the entire conversation for every new token it generates. The longer the conversation, the larger this cache grows.</p>
          <p>By default the KV cache lives on the GPU for speed, but it can consume a significant amount of VRAM — especially with large context windows or multiple concurrent requests. If you're running low on VRAM, moving the KV cache to the CPU frees up GPU memory at the cost of slower inference.</p>
          <p>Control where the KV cache is stored:</p>
          <pre className="code-block"><code className="language-yaml">{`offload_kqv: true    # KV cache on GPU (default, faster)
offload_kqv: false   # KV cache on CPU (saves VRAM, slower)`}</code></pre>
          <p><em>Note: On Apple Silicon (Metal), the CPU and GPU share unified memory, so this setting has no practical effect. KV cache location applies to discrete GPU systems (NVIDIA CUDA, Vulkan).</em></p>
          <h4 id="tensor-operations-offload">Tensor Operations Offload</h4>
          <p>Beyond the model layers and KV cache, there are additional math operations (called tensor operations) that happen during inference — things like matrix multiplications and attention score calculations. These operations are separate from the layer weights themselves and can independently be placed on the GPU or CPU. By default they run on the GPU, but if VRAM is tight you can move them to the CPU while still keeping your model layers on the GPU.</p>
          <p>Control where these tensor computations run:</p>
          <pre className="code-block"><code className="language-yaml">{`op_offload: true     # Tensor ops on GPU (default)
op_offload: false    # Tensor ops on CPU`}</code></pre>
          <p><em>Note: On Apple Silicon (Metal), the CPU and GPU share unified memory, so this setting has no practical effect. Offloading applies to discrete GPU systems (NVIDIA CUDA, Vulkan).</em></p>
          <h4 id="multi-gpu-split-mode">Multi-GPU Split Mode</h4>
          <p>If you have more than one GPU in your system, you can spread a model across them. This is useful when a model is too large to fit in a single GPU's VRAM. There are two strategies,<code>layer mode</code> and <code>row mode</code>.</p>
          <p><code>layer mode</code> assigns entire layers to different GPUs (simple and works well for most models).</p>
          <p><code>row mode</code> splits individual tensor operations across GPUs in parallel (better for Mixture of Experts models like Qwen3-MoE, Mixtral, or DeepSeek where different "experts" can run simultaneously on different GPUs).</p>
          <p>Control how the model is distributed across GPUs:</p>
          <pre className="code-block"><code className="language-yaml">{`split_mode: none     # Single GPU (default)
split_mode: layer    # Split layers across GPUs
split_mode: row      # Tensor parallelism (best for MoE models)`}</code></pre>
          <p><em>Note: Use this setting for Mixture of Experts models like Qwen3-MoE, Mixtral, or DeepSeek.</em></p>
          <h4 id="configuration-reference">Configuration Reference</h4>
          <p>Here is a chart for all these GPU settings. These only apply to discrete GPU systems (NVIDIA CUDA, Vulkan). On Apple Silicon, the CPU and GPU share unified memory and these settings can be ignored.</p>
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
          <p>As discussed in the previous section, the KV cache is the model's short-term memory of your conversation. By default it stores values in half precision (f16), which gives the best accuracy but uses the most VRAM. Quantization reduces the precision of those stored values — using fewer bits to represent each number. It's a trade-off: you lose a small amount of accuracy in exchange for meaningful VRAM savings. For most use cases, <code>q8_0</code> (8-bit) gives nearly identical output quality while cutting KV cache memory by about 25%. More aggressive options like <code>q4_0</code> save even more but can start to affect generation quality.</p>
          <p>Control the precision of the key and value caches independently:</p>
          <pre className="code-block"><code className="language-yaml">{`cache_type_k: q8_0 # Key cache precision
cache_type_v: q8_0 # Value cache precision`}</code></pre>
          <h4 id="available-types">Available types</h4>
          <ul>
            <li><code>f16</code> - Half precision (default, best quality)</li>
            <li><code>q8_0</code> - 8-bit quantization (good balance)</li>
            <li><code>q4_0</code> - 4-bit quantization (aggressive, may affect quality)</li>
            <li><code>bf16</code> - Brain float 16 (for supported hardware)</li>
          </ul>
          <h4 id="when-to-use-f16-vs-q8_0">When to use f16 vs q8_0</h4>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Consideration</th>
                <th>f16 (default)</th>
                <th>q8_0</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>VRAM usage</td>
                <td>Higher</td>
                <td>~50% less for KV cache</td>
              </tr>
              <tr>
                <td>Output quality</td>
                <td>Best possible</td>
                <td>Nearly identical for most tasks</td>
              </tr>
              <tr>
                <td>MoE models</td>
                <td>Recommended — routing is sensitive to precision</td>
                <td>May degrade expert routing decisions</td>
              </tr>
              <tr>
                <td>Dense models</td>
                <td>Safe but uses more memory</td>
                <td>Recommended — minimal quality loss with good VRAM savings</td>
              </tr>
              <tr>
                <td>Long-context (64K+)</td>
                <td>Safer — avoids compounding</td>
                <td>Small precision errors can</td>
              </tr>
              <tr>
                <td></td>
                <td>precision errors</td>
                <td>accumulate over long sequences</td>
              </tr>
            </tbody>
          </table>
          <p>Start with <code>q8_0</code> for dense models. Use <code>f16</code> for MoE models or if you notice quality issues (incoherent outputs, reasoning failures).</p>
          <h4 id="example-moe-model-with-f16-cache">Example: MoE Model with F16 Cache</h4>
          <pre className="code-block"><code className="language-yaml">{`models:
  # MoE models benefit from f16 cache for routing accuracy
  Qwen3.5-35B-A3B-Q8_0:
    context_window: 32768
    cache_type_k: f16 # Preserve routing precision
    cache_type_v: f16
    split_mode: row # Best for MoE multi-GPU

  # Dense models can often use q8_0 cache without issues
  Qwen3-8B-Q8_0:
    context_window: 32768
    cache_type_k: q8_0
    cache_type_v: q8_0`}</code></pre>
          <p><strong>Recommendation:</strong> If you notice quality degradation (incoherent outputs, reasoning failures, or code bugs) with quantized cache, try <code>f16</code> first before adjusting other parameters. The VRAM cost is typically 25-50% more for the cache, but the quality improvement for sensitive workloads is substantial.</p>
          <h3 id="35-flash-attention">3.5 Flash Attention</h3>
          <p>Attention is the core mechanism that lets a model figure out which parts of your input are relevant to each other. For example, in the sentence "The cat sat on the mat because it was tired," attention is how the model connects "it" back to "the cat." The standard attention algorithm needs to hold a large matrix of scores in memory — one score for every pair of tokens in your input. As context windows grow, this matrix grows quadratically and can become both slow and memory-hungry.</p>
          <p>Flash Attention is an optimized implementation that computes the same result but processes the matrix in small tiles that fit in the GPU's fast on-chip memory (SRAM) instead of slower VRAM. The result is lower memory usage and faster computation — especially noticeable with large context windows (32K+). It's enabled by default and should rarely need to be changed.</p>
          <p>Control whether Flash Attention is used:</p>
          <pre className="code-block"><code className="language-yaml">{`flash_attention: enabled   # Default: enabled
flash_attention: disabled  # Disable if causing issues
flash_attention: auto      # Let llama.cpp decide`}</code></pre>
          <p>_Note: Hybrid models (those combining attention and recurrent layers, such as Qwen3.5-35B-A3B) do not support flash attention. Kronk automatically disables it for these models. Additionally, quantized KV caches (<code>q8_0</code>, <code>q4_0</code>) require flash attention to function — so when flash attention is disabled for hybrid models, Kronk also forces the KV cache type to f16. These overrides happen regardless of your configuration settings._</p>
          <h3 id="36-sliding-window-attention-swa">3.6 Sliding Window Attention (SWA)</h3>
          <p>Some models use a hybrid attention pattern that interleaves sliding window attention (SWA) layers with full global attention layers. In SWA layers, each token only attends to a small local window of recent tokens (e.g., 1024 tokens) rather than the entire context. The global attention layers still see everything, which keeps the model coherent over long contexts while the SWA layers provide efficient local processing.</p>
          <p>Models that use sliding window attention include:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Model</th>
                <th>SWA Window</th>
                <th>Architecture</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Gemma 4 26B-A4B</td>
                <td>1024</td>
                <td>MoE</td>
              </tr>
              <tr>
                <td>Gemma 4 31B</td>
                <td>1024</td>
                <td>Dense</td>
              </tr>
              <tr>
                <td>Gemma 4 E2B / E4B</td>
                <td>512</td>
                <td>Dense</td>
              </tr>
              <tr>
                <td>Gemma 3 (all sizes)</td>
                <td>1024</td>
                <td>Dense</td>
              </tr>
            </tbody>
          </table>
          <p>Kronk automatically detects sliding window metadata from the GGUF file — you don't need to configure the window size. By default, llama.cpp allocates a compact KV cache for SWA layers (sized to the window), which saves significant VRAM compared to allocating the full context window for every layer. However, this compact cache prevents advanced operations like context shifting and full prefix caching on SWA layers.</p>
          <h4 id="swa-full-cache-mode">SWA Full Cache Mode</h4>
          <p>When accuracy is more important than memory savings, you can force SWA layers to use the full context window for their KV cache:</p>
          <pre className="code-block"><code className="language-yaml">{`swa_full: true    # Full-size KV cache for SWA layers (more VRAM, better accuracy)
swa_full: false   # Compact SWA cache (default, less VRAM)`}</code></pre>
          <p>When <code>swa_full</code> is enabled, SWA layers allocate the same KV cache size as global attention layers. This preserves all cached context for SWA layers and enables full context shifting and prefix caching, but increases VRAM usage proportionally.</p>
          <h4 id="vram-impact">VRAM Impact</h4>
          <p>The VRAM difference depends on what fraction of layers use SWA. For example, Gemma 4 26B-A4B has 30 layers with a pattern where roughly 5/6 of attention layers are SWA. With a 32K context window and f16 KV cache:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Setting</th>
                <th>SWA Layer Cache</th>
                <th>Approximate KV Savings</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>swa_full: false</code></td>
                <td>1024 tokens</td>
                <td>~40-50% less KV VRAM</td>
              </tr>
              <tr>
                <td><code>swa_full: true</code></td>
                <td>32768 tokens</td>
                <td>None (full allocation)</td>
              </tr>
            </tbody>
          </table>
          <p>_Note: Not all models use sliding window attention. Dense models (Llama, Qwen3, Mistral-large), hybrid models (Qwen3.5/3.6), and most MoE models (Qwen3-MoE, DeepSeek) use full attention on all layers. The <code>swa_full</code> setting has no effect on these models._</p>
          <h3 id="37-parallel-inference-nseqmax">3.7 Parallel Inference (NSeqMax)</h3>
          <p>When multiple users (or applications) send requests to the same model at the same time, the model needs a way to handle them concurrently. That's what <code>NSeqMax</code> controls — it determines how many requests the model can process in parallel.</p>
          <p>Behind the scenes, when a model is loaded, Kronk creates one processing slot for each unit of <code>n_seq_max</code> (e.g., <code>n_seq_max: 4</code> creates four slots). Each slot gets its own isolated partition in the KV cache (the model's short-term memory from earlier sections). All slots share the same model weights and GPU, but each one maintains its own conversation state independently.</p>
          <p>Consider what happens when <code>n_seq_max</code> is set to 1 and two requests arrive. The first request is assigned to the only available slot and begins generating tokens. The second request has no slot available, so it waits in a queue. Once the first request finishes and the slot is released, the second request is assigned to that slot and begins processing. With a single slot, requests are handled one at a time.</p>
          <p>When <code>n_seq_max</code> is set to 4, then four requests can each be assigned a slot at the same time and generate tokens simultaneously. Kronk combines the next token from each active slot into a single batch and sends that batch through the GPU in one forward pass — so the GPU processes all four tokens together rather than one at a time. That's where we get some performance optimization.</p>
          <p>The trade-off is VRAM. Each slot reserves its full KV cache partition when the model loads, whether or not it's actively handling a request. Setting <code>n_seq_max: 4</code> means four KV cache partitions are allocated upfront. If each partition costs 3 GB, that's 12 GB of VRAM just for the cache — on top of the model weights. More slots means more concurrency but more VRAM.</p>
          <p>Control how many requests can be processed in parallel:</p>
          <pre className="code-block"><code className="language-yaml">{`n_seq_max: 4 # Process up to 4 requests concurrently`}</code></pre>
          <h4 id="how-caching-strategy-affects-slot-behavior">How Caching Strategy Affects Slot Behavior</h4>
          <p>Enabling a <a href="#chapter-5-message-caching">caching strategy</a> does not add any extra memory to the system — caching works within the KV cache already allocated to each slot. The difference between strategies is what happens to the data in the KV cache between requests:</p>
          <p><strong>No Caching</strong> — The simplest mode. When a request finishes, the slot's KV cache is cleared. The next request that lands in that slot starts from scratch, processing the full prompt from the beginning. Every request pays the full cost of prompt processing regardless of how similar it is to a previous one.</p>
          <p><strong>IMC (Incremental Message Cache)</strong> — Designed for single-user, multi-turn conversations. IMC maintains logical sessions that cache the conversation history. All sessions (text and media) externalize their cached KV state to RAM after each request and restore it into any available slot on the next request — slots are not dedicated to conversations. When the user sends a new message, only the new tokens need to be processed — the model doesn't re-read the entire conversation. This gives the best performance for chat and agentic applications.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Mode</th>
                <th>Session Lifetime</th>
                <th>Best For</th>
                <th>Cache Strategy</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Off</td>
                <td>Cleared after request</td>
                <td>Stateless</td>
                <td>None</td>
              </tr>
              <tr>
                <td>IMC</td>
                <td>Persists across requests</td>
                <td>Single-user</td>
                <td>Conversation cached in session; externalizes to RAM between requests</td>
              </tr>
            </tbody>
          </table>
          <h4 id="embedding-and-reranking-models">Embedding and Reranking Models</h4>
          <p>Embedding and reranking models work differently. Instead of slots sharing a single context, <code>NSeqMax</code> creates a pool of independent contexts. When a request contains multiple inputs (for example, 100 sentences to embed), those inputs are spread across the pool contexts and processed in parallel. Model weights are shared, but each context has its own KV cache memory.</p>
          <h3 id="38-understanding-gguf-quantization">3.8 Understanding GGUF Quantization</h3>
          <p>GGUF models come in various quantization formats that trade off between file size, VRAM usage, and output quality. Understanding these formats helps you choose the right model variant for your hardware and use case.</p>
          <h4 id="what-is-quantization?">What is Quantization?</h4>
          <p>Quantization reduces model precision from the original 16-bit or 32-bit floating-point weights to lower bit representations. This dramatically decreases:</p>
          <ul>
            <li><strong>File size</strong> - A 7B model can go from ~14GB (FP16) to ~3GB (Q4)</li>
            <li><strong>VRAM usage</strong> - More aggressive quantization allows larger models on limited hardware</li>
            <li><strong>Inference speed</strong> - Smaller models load faster and may run faster on memory-constrained systems</li>
          </ul>
          <p>The tradeoff is <strong>quality degradation</strong> - lower precision means less accurate representations of the original weights, which can affect output coherence, reasoning ability and factual accuracy.</p>
          <h4 id="what-are-k-quants?">What are K-Quants?</h4>
          <p>K-quants (introduced by llama.cpp) use <strong>per-block scaling</strong> with importance weighting. Instead of applying uniform quantization across all weights, K-quants:</p>
          <ol>
            <li>Divide weights into small blocks (typically 32 or 256 values)</li>
            <li>Calculate optimal scale factors per block</li>
            <li>Preserve more precision for important weights</li>
          </ol>
          <p>This produces better quality than naive quantization at the same bit rate. K-quant variants include size suffixes:</p>
          <ul>
            <li><strong>S</strong> (Small) - Smallest file size, lowest quality within that bit level</li>
            <li><strong>M</strong> (Medium) - Balanced size and quality</li>
            <li><strong>L</strong> (Large) - Larger file, better quality</li>
          </ul>
          <h4 id="standard-quantization-formats">Standard Quantization Formats</h4>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Format</th>
                <th>Bits/Weight</th>
                <th>Quality</th>
                <th>VRAM (7B Model)</th>
                <th>Use Case</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><strong>Q4_0</strong></td>
                <td>4.5</td>
                <td>Low</td>
                <td>~4 GB</td>
                <td>Maximum compression, quality loss noticeable</td>
              </tr>
              <tr>
                <td><strong>Q4_1</strong></td>
                <td>5.0</td>
                <td>Low-Med</td>
                <td>~4.3 GB</td>
                <td>Slightly better than Q4_0</td>
              </tr>
              <tr>
                <td><strong>Q4_K_S</strong></td>
                <td>4.5</td>
                <td>Medium</td>
                <td>~4 GB</td>
                <td>K-quant, good balance for limited VRAM</td>
              </tr>
              <tr>
                <td><strong>Q4_K_M</strong></td>
                <td>4.8</td>
                <td>Medium</td>
                <td>~4.5 GB</td>
                <td>K-quant, recommended 4-bit option</td>
              </tr>
              <tr>
                <td><strong>Q5_K_S</strong></td>
                <td>5.5</td>
                <td>Medium-High</td>
                <td>~5 GB</td>
                <td>Good quality, moderate size</td>
              </tr>
              <tr>
                <td><strong>Q5_K_M</strong></td>
                <td>5.7</td>
                <td>High</td>
                <td>~5.3 GB</td>
                <td>Recommended for most users</td>
              </tr>
              <tr>
                <td><strong>Q6_K</strong></td>
                <td>6.5</td>
                <td>High</td>
                <td>~6 GB</td>
                <td>Near-original quality</td>
              </tr>
              <tr>
                <td><strong>Q8_0</strong></td>
                <td>8.5</td>
                <td>Highest</td>
                <td>~8 GB</td>
                <td>Best quality, largest size</td>
              </tr>
            </tbody>
          </table>
          <h4 id="iq-importance-matrix-quantization">IQ (Importance Matrix) Quantization</h4>
          <p>IQ formats use <strong>learned importance matrices</strong> to determine which weights matter most. They achieve extreme compression with minimal quality loss by:</p>
          <ol>
            <li>Analyzing weight importance during quantization</li>
            <li>Allocating more bits to critical weights</li>
            <li>Aggressively compressing less important weights</li>
          </ol>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Format</th>
                <th>Bits/Weight</th>
                <th>Quality</th>
                <th>Use Case</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><strong>IQ1_S</strong></td>
                <td>~1.5</td>
                <td>Very Low</td>
                <td>Extreme compression, experimental</td>
              </tr>
              <tr>
                <td><strong>IQ1_M</strong></td>
                <td>~1.75</td>
                <td>Low</td>
                <td>Extreme compression, experimental</td>
              </tr>
              <tr>
                <td><strong>IQ2_XXS</strong></td>
                <td>~2.0</td>
                <td>Low</td>
                <td>Ultra-low VRAM situations</td>
              </tr>
              <tr>
                <td><strong>IQ2_XS</strong></td>
                <td>~2.3</td>
                <td>Low-Med</td>
                <td>Very constrained hardware</td>
              </tr>
              <tr>
                <td><strong>IQ2_S</strong></td>
                <td>~2.5</td>
                <td>Medium</td>
                <td>Constrained hardware</td>
              </tr>
              <tr>
                <td><strong>IQ3_XXS</strong></td>
                <td>~3.0</td>
                <td>Medium</td>
                <td>Good balance for low VRAM</td>
              </tr>
              <tr>
                <td><strong>IQ3_XS</strong></td>
                <td>~3.3</td>
                <td>Medium-High</td>
                <td>Better quality low-bit option</td>
              </tr>
              <tr>
                <td><strong>IQ4_XS</strong></td>
                <td>~4.0</td>
                <td>High</td>
                <td>Alternative to Q4_K variants</td>
              </tr>
            </tbody>
          </table>
          <h4 id="ud-ultra-dynamic-quantization">UD (Ultra-Dynamic) Quantization</h4>
          <p>UD quantization applies <strong>different precision levels per layer</strong>. Neural network layers have varying sensitivity to quantization:</p>
          <ul>
            <li>Early layers (embeddings, first attention blocks) - More sensitive</li>
            <li>Middle layers - Moderately sensitive</li>
            <li>Later layers - Often more tolerant of compression</li>
          </ul>
          <p>UD variants analyze each layer and assign optimal bit depths, achieving better quality than uniform quantization at similar average bits per weight.</p>
          <p>Common UD naming: <code>UD-Q5_K_XL</code> means Ultra-Dynamic with Q5 K-quant base, XL quality tier.</p>
          <h3 id="39-choosing-the-right-quantization">3.9 Choosing the Right Quantization</h3>
          <p>The right quantization depends on how much VRAM you have and what quality you need.</p>
          <h4 id="by-available-vram">By Available VRAM</h4>
          <table className="flags-table">
            <thead>
              <tr>
                <th>VRAM</th>
                <th>7B Model</th>
                <th>13B Model</th>
                <th>30B Model</th>
                <th>70B Model</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>6 GB</td>
                <td>Q4_K_M</td>
                <td>IQ3_XXS</td>
                <td>-</td>
                <td>-</td>
              </tr>
              <tr>
                <td>8 GB</td>
                <td>Q6_K</td>
                <td>Q4_K_M</td>
                <td>IQ2_XXS</td>
                <td>-</td>
              </tr>
              <tr>
                <td>12 GB</td>
                <td>Q8_0</td>
                <td>Q5_K_M</td>
                <td>IQ3_XXS</td>
                <td>-</td>
              </tr>
              <tr>
                <td>16 GB</td>
                <td>Q8_0</td>
                <td>Q8_0</td>
                <td>Q4_K_M</td>
                <td>-</td>
              </tr>
              <tr>
                <td>24 GB</td>
                <td>Q8_0</td>
                <td>Q8_0</td>
                <td>Q6_K</td>
                <td>IQ3_XXS</td>
              </tr>
              <tr>
                <td>48 GB</td>
                <td>Q8_0</td>
                <td>Q8_0</td>
                <td>Q8_0</td>
                <td>Q4_K_M</td>
              </tr>
              <tr>
                <td>64 GB+</td>
                <td>Q8_0</td>
                <td>Q8_0</td>
                <td>Q8_0</td>
                <td>Q6_K/Q8_0</td>
              </tr>
            </tbody>
          </table>
          <h4 id="by-use-case">By Use Case</h4>
          <ul>
            <li><strong>Production/Quality-Critical</strong>: Q8_0 or Q6_K - Minimal quality loss</li>
            <li><strong>General Use</strong>: Q5_K_M - Best balance of quality and efficiency</li>
            <li><strong>VRAM-Constrained</strong>: Q4_K_M - Good quality at low VRAM cost</li>
            <li><strong>Experimental/Testing</strong>: IQ3_XXS or IQ2_XS - Run larger models on limited hardware</li>
          </ul>
          <h4 id="quality-guidelines">Quality Guidelines</h4>
          <ol>
            <li><strong>Start with Q5_K_M</strong> - It's the sweet spot for most use cases</li>
            <li><strong>Use Q8_0 for reasoning-heavy tasks</strong> - Math, code, complex logic benefit from higher precision</li>
            <li><strong>Q4_K_M is the floor</strong> - Below this, quality degrades noticeably for most models</li>
            <li><strong>IQ formats are specialized</strong> - Great for running models that wouldn't otherwise fit, but expect some quality loss</li>
            <li><strong>Larger models at lower quant often beat smaller models at higher quant</strong> - A 70B Q4 may outperform a 7B Q8</li>
          </ol>
          <h4 id="example-configuration">Example Configuration</h4>
          <pre className="code-block"><code className="language-yaml">{`models:
  # Quality-focused: Q8_0 for a model that fits in VRAM
  Qwen3-8B-Q8_0:
    context_window: 32768
    cache_type_k: q8_0
    cache_type_v: q8_0

  # VRAM-constrained: Q4_K_M to fit larger model
  Llama-3.3-70B-Instruct-Q4_K_M:
    context_window: 8192
    split_mode: row
    n_gpu_layers: 0`}</code></pre>
          <h3 id="310-vram-estimation">3.10 VRAM Estimation</h3>
          <p>Before loading a model, you need to know whether it will fit in your GPU's memory. VRAM usage comes from two things: the model weights (fixed cost determined by the model you chose) and the KV cache (variable cost determined by your configuration choices from the previous sections — context window size, number of slots, and cache precision). If the total exceeds your available VRAM, the model either won't load or will partially fall back to the CPU, which significantly slows inference. This section walks through how to estimate the total.</p>
          <h4 id="model-weights-+-kv-cache">Model Weights + KV Cache</h4>
          <p>Model weights are the learned numerical parameters (billions of floating-point values) that encode the model's knowledge and reasoning ability — they represent the fixed cost of loading a model into memory. Model weights are determined by the GGUF file size (e.g., ~8GB for a 7B Q8_0 model). The KV cache is the variable cost you control through configuration. Together they determine total VRAM usage:</p>
          <p>Total VRAM = Model Weights + KV Cache.</p>
          <h4 id="model-weights-q8_0-quantization">Model Weights (Q8_0 quantization)</h4>
          <p>The following table provides rough VRAM estimates for model weights at Q8_0 quantization, grouped by parameter count.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Parameters</th>
                <th>VRAM</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>1-3B</td>
                <td>2-4 GB</td>
              </tr>
              <tr>
                <td>7-8B</td>
                <td>8-10 GB</td>
              </tr>
              <tr>
                <td>13B</td>
                <td>14-16 GB</td>
              </tr>
              <tr>
                <td>30B</td>
                <td>32-36 GB</td>
              </tr>
              <tr>
                <td>70B</td>
                <td>72-80 GB</td>
              </tr>
            </tbody>
          </table>
          <h4 id="slots-and-sequences">Slots and Sequences</h4>
          <p>A slot is a processing unit that handles one request at a time. Each slot is assigned a unique sequence ID that maps to an isolated partition in the shared KV cache. The mapping is always 1:1 in Kronk.</p>
          <pre className="code-block"><code>{`NSeqMax = 4 (set via n_seq_max in model config)

Slot 0  →  Sequence 0  →  KV cache partition 0
Slot 1  →  Sequence 1  →  KV cache partition 1
Slot 2  →  Sequence 2  →  KV cache partition 2
Slot 3  →  Sequence 3  →  KV cache partition 3`}</code></pre>
          <p>Remember as shared previously, <code>NSeqMax</code> controls how many slots (and sequences) are created. More slots means more concurrent requests, but each slot reserves its own KV cache partition in VRAM whether or not it is actively used.</p>
          <h4 id="what-affects-kv-cache-memory-per-sequence">What Affects KV Cache Memory Per Sequence</h4>
          <p>Each sequence's KV cache partition size is determined by three factors:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Factor</th>
                <th>Config Key</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Context Window</td>
                <td><code>n_ctx</code></td>
                <td>Maximum tokens the sequence can hold. Memory scales linearly — 32K context uses 4× the memory of 8K.</td>
              </tr>
              <tr>
                <td>Number of Layers</td>
                <td><code>block_count</code></td>
                <td>Every transformer layer stores its own key and value tensors per token. A 70B model (80 layers) uses ~2.5× more per-token memory than a 7B model (32 layers).</td>
              </tr>
              <tr>
                <td>KV Cache Precision</td>
                <td><code>bytes_per_element</code></td>
                <td>Data type for cached keys and values: <code>f16</code> = 2 bytes/element (default, best quality), <code>q8_0</code> = 1 byte/element (50% VRAM savings, good quality).</td>
              </tr>
            </tbody>
          </table>
          <p>The remaining values (<code>head_count_kv</code>, <code>key_length</code>, <code>value_length</code>) are baked into the model itself and cannot be changed — Kronk reads them automatically from the GGUF file.</p>
          <p>The formula:</p>
          <pre className="code-block"><code>{`KV_Per_Token_Per_Layer = head_count_kv × (key_length + value_length) × bytes_per_element
KV_Per_Sequence        = n_ctx × n_layers × KV_Per_Token_Per_Layer`}</code></pre>
          <h4 id="what-affects-total-kv-cache-slot-memory">What Affects Total KV Cache (Slot Memory)</h4>
          <p>Total KV cache (Slot Memory) is the per-sequence cost multiplied by the number of slots:</p>
          <pre className="code-block"><code>{`Slot_Memory = NSeqMax × KV_Per_Sequence
Total_VRAM  = Model_Weights + Slot_Memory`}</code></pre>
          <p>Memory is statically allocated upfront when the model loads. All slots reserve their full KV cache partition regardless of whether they are actively processing a request.</p>
          <h4 id="example-real-model-calculation">Example: Real Model Calculation</h4>
          <pre className="code-block"><code>{`Model                   : Qwen3.5-35B-A3B-Q8_0
Model Weights           : 36.0 GB
Context Window (n_ctx)  : 131,072 (128K)
Bytes Per Element       : 1 (q8_0)
block_count (n_layers)  : 48
attention.head_count_kv : 4
attention.key_length    : 128
attention.value_length  : 128

Step 1 — Per-token-per-layer cost:

  KV_Per_Token_Per_Layer = 4 × (128 + 128) × 1 = 1,024 bytes

Step 2 — Per-sequence cost:

  KV_Per_Sequence = 131,072 × 48 × 1,024 = ~6.4 GB

Step 3 — Total KV cache (NSeqMax = 2):

  Slot_Memory = 2 × 6.4 GB = ~12.8 GB

Step 4 — Total VRAM:

  Total_VRAM = 36.0 GB + 12.8 GB = ~48.8 GB`}</code></pre>
          <h3 id="311-model-specific-tuning">3.11 Model-Specific Tuning</h3>
          <p>The previous sections covered general configuration that applies to all models. However, different model architectures — Dense, Mixture of Experts (MoE), and Hybrid — each have their own characteristics that benefit from specific tuning. Vision and audio models also need adjusted batch settings because they process media as large token batches. This section provides recommended configurations for each model type so you can get the best performance out of the box.</p>
          <h4 id="dense-models">Dense Models</h4>
          <p>Dense models are the most common architecture. Every parameter participates in every token, producing sequential memory access patterns that saturate bandwidth efficiently. No special configuration is needed — the defaults from the previous sections apply directly.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Do</th>
                <th>Don't</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Use <code>q8_0</code> KV cache to save VRAM</td>
                <td>Use <code>f16</code> cache unless you need maximum quality</td>
              </tr>
              <tr>
                <td>Start with default <code>n_batch</code> / <code>n_ubatch</code></td>
                <td>Over-tune batch settings without benchmarking</td>
              </tr>
              <tr>
                <td>Use flash attention when available</td>
                <td>Disable flash attention without reason</td>
              </tr>
            </tbody>
          </table>
          <h4 id="moe-models">MoE Models</h4>
          <p>MoE (Mixture of Experts) models have many total parameters but only activate a small subset per token. For example, <code>Qwen3-Coder-30B-A3B</code> has 30B total parameters but only 3B are active per token (that's what "A3B" means).</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Do</th>
                <th>Don't</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Use <code>split_mode: row</code> for multi-GPU</td>
                <td>Use <code>split_mode: layer</code> — it doesn't suit MoE routing</td>
              </tr>
              <tr>
                <td>Start with <code>q8_0</code> KV cache, fall back to <code>f16</code> if quality drops</td>
                <td>Assume <code>q8_0</code> cache works for all MoE models</td>
              </tr>
              <tr>
                <td>Prefer dense Q4 over MoE Q8 on Apple Silicon</td>
                <td>Assume fewer active parameters means faster inference</td>
              </tr>
            </tbody>
          </table>
          <p><em>Note: On unified memory systems (like Apple Silicon), inference speed depends on memory bandwidth, not compute. MoE models create scattered memory access patterns (jumping between expert weights) that underutilize bandwidth compared to the sequential access of dense models. A dense model at Q4 may outperform a larger MoE at Q8 on Apple Silicon — fewer total bytes moved and a more efficient access pattern.</em></p>
          <h4 id="hybrid-models">Hybrid Models</h4>
          <p>Hybrid models mix traditional attention layers with recurrent layers (DeltaNet or SSM/Mamba). Like dense models, every parameter participates in every token. Kronk detects hybrid models automatically at load time.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Do</th>
                <th>Don't</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Use <code>f16</code> for both <code>cache_type_k</code> and <code>cache_type_v</code></td>
                <td>Use <code>q8_0</code> cache — it's incompatible with recurrent layers</td>
              </tr>
              <tr>
                <td>Enable <code>incremental_cache</code> for conversation workloads</td>
                <td>Manually enable flash attention — Kronk disables it for you</td>
              </tr>
              <tr>
                <td>Budget extra VRAM for the larger f16 KV cache</td>
                <td>Expect the same KV cache savings as dense models</td>
              </tr>
            </tbody>
          </table>
          <p>See <a href="#imc-hybrid">IMC Hybrid</a> for details on how caching works with recurrent state.</p>
          <h4 id="vision-and-audio-models">Vision and Audio Models</h4>
          <p>Vision and audio models process media (image tiles, audio frames) as large token batches. Low <code>n_ubatch</code> values force multiple decode passes per image or audio clip, significantly slowing inference.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Do</th>
                <th>Don't</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Set <code>n_ubatch</code> high (2048+) to match media token volume</td>
                <td>Leave <code>n_ubatch</code> at low defaults — it causes multiple decode passes</td>
              </tr>
              <tr>
                <td>Match <code>n_batch</code> to <code>n_ubatch</code> for media workloads</td>
                <td>Set <code>n_seq_max</code> high — media processing is memory-intensive</td>
              </tr>
            </tbody>
          </table>
          <h4 id="embedding-models">Embedding Models</h4>
          <p>Embedding models process complete inputs in a single pass rather than generating tokens one at a time.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Do</th>
                <th>Don't</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Set <code>n_batch</code> high (up to <code>context_window</code>)</td>
                <td>Use small <code>n_batch</code> — it limits throughput</td>
              </tr>
              <tr>
                <td>Use multiple slots (<code>n_seq_max</code>) for concurrency</td>
                <td>Over-allocate slots beyond your request volume</td>
              </tr>
            </tbody>
          </table>
          <h4 id="swa-models">SWA Models</h4>
          <p>Models with sliding window attention (Gemma 4, Gemma 3) interleave local SWA layers with global attention layers. By default, SWA layers use a compact KV cache sized to the sliding window (e.g., 1024 tokens), which saves significant VRAM. Enable <code>swa_full</code> when accuracy matters more than memory.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Do</th>
                <th>Don't</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Enable <code>swa_full: true</code> when accuracy is the priority</td>
                <td>Enable <code>swa_full</code> on models without SWA — it has no effect</td>
              </tr>
              <tr>
                <td>Use <code>f16</code> KV cache for MoE SWA models (e.g., Gemma 4)</td>
                <td>Assume <code>swa_full</code> is needed — test without it first</td>
              </tr>
              <tr>
                <td>Budget extra VRAM when <code>swa_full</code> is enabled</td>
                <td>Use large context windows + <code>swa_full</code> without checking VRAM</td>
              </tr>
            </tbody>
          </table>
          <pre className="code-block"><code className="language-yaml">{`# Example: Gemma 4 26B-A4B with full SWA cache
unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M:
  context-window: 32768
  swa-full: true
  incremental-cache: true`}</code></pre>
          <h3 id="312-speculative-decoding">3.12 Speculative Decoding</h3>
          <p>Speculative decoding uses a small, fast "draft" model to predict candidate tokens, then verifies them against the full "target" model in a single forward pass. When the draft model's predictions match the target's, multiple tokens are accepted per decode step — improving throughput without changing output quality. The output distribution is mathematically guaranteed to match the target model exactly, regardless of draft quality (Leviathan et al., 2023).</p>
          <h4 id="how-it-works">How It Works</h4>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Step</th>
                <th>What Happens</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>1. Draft</td>
                <td>The draft model generates N candidate tokens (default 5)</td>
              </tr>
              <tr>
                <td>2. Batch</td>
                <td>All candidates plus the last accepted token are decoded by the target model in one forward pass</td>
              </tr>
              <tr>
                <td>3. Verify</td>
                <td>Each candidate is accepted with probability <code>min(1, p_target / q_draft)</code></td>
              </tr>
              <tr>
                <td>4. Reject</td>
                <td>On rejection, a corrective token is sampled from the target and remaining candidates are discarded</td>
              </tr>
              <tr>
                <td>5. Bonus</td>
                <td>If all candidates are accepted, a bonus token is sampled from the target</td>
              </tr>
            </tbody>
          </table>
          <p>The speedup depends on the draft model's acceptance rate. Higher acceptance means more tokens per forward pass.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Factor</th>
                <th>Effect on Acceptance Rate</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Draft model quality</td>
                <td>Larger, more capable drafts produce better predictions. Q4 of the same architecture tends to outperform a much smaller model.</td>
              </tr>
              <tr>
                <td>Temperature</td>
                <td>Lower temperatures yield higher acceptance. At 0.8, expect ~30% of steps to accept zero draft tokens.</td>
              </tr>
              <tr>
                <td>Task type</td>
                <td>Predictable text (boilerplate, common patterns) accepts more often than creative or reasoning-heavy output.</td>
              </tr>
            </tbody>
          </table>
          <h4 id="requirements">Requirements</h4>
          <ul>
            <li>Draft and target models must share the <strong>same vocabulary</strong> (same tokenizer)</li>
            <li><code>n_seq_max</code> must be <code>1</code> (single-slot mode only)</li>
            <li>The draft model must be downloaded and available locally</li>
            <li>Only text generation is supported (not vision/audio)</li>
          </ul>
          <h4 id="configuration">Configuration</h4>
          <p>Speculative decoding is configured via the <code>draft-model</code> block under a target model entry in <code>model_config.yaml</code> (or via <code>model.Config.DraftModel</code> when you embed the SDK directly):</p>
          <pre className="code-block"><code className="language-yaml">{`# In ~/.kronk/model_config.yaml
Qwen/Qwen3-8B-Q8_0:
  context-window: 32768
  nbatch: 2048
  nubatch: 512
  cache-type-k: q8_0
  cache-type-v: q8_0
  nseq-max: 1
  incremental-cache: true
  draft-model:
    model-id: unsloth/Qwen3-0.6B-Q8_0  # Draft model ID (must be downloaded)
    ndraft: 5                          # Candidates per step (default: 5)
    ngpu-layers: 0                     # GPU layers (0=all, -1=none)
    devices: []                        # Pin to specific GPUs (e.g., ["CUDA0"])`}</code></pre>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Field</th>
                <th>YAML Key</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>ModelID</td>
                <td><code>model-id</code></td>
                <td>(none)</td>
                <td>Draft model ID (must be downloaded)</td>
              </tr>
              <tr>
                <td>NDraft</td>
                <td><code>ndraft</code></td>
                <td>5</td>
                <td>Number of candidate tokens per step</td>
              </tr>
              <tr>
                <td>NGpuLayers</td>
                <td><code>ngpu-layers</code></td>
                <td>0 (all)</td>
                <td>GPU layers for draft model</td>
              </tr>
              <tr>
                <td>Devices</td>
                <td><code>devices</code></td>
                <td><code>[]</code></td>
                <td>Pin draft model to specific GPUs (by name)</td>
              </tr>
              <tr>
                <td>MainGPU</td>
                <td><code>main-gpu</code></td>
                <td>(auto)</td>
                <td>Main GPU index for the draft model</td>
              </tr>
              <tr>
                <td>TensorSplit</td>
                <td><code>tensor-split</code></td>
                <td><code>[]</code></td>
                <td>Tensor split ratios across draft GPUs</td>
              </tr>
            </tbody>
          </table>
          <h4 id="draft-model-selection">Draft Model Selection</h4>
          <p>Choose a draft model that shares the same tokenizer family as the target. A quantized version of the same architecture at lower precision works well:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Target Model</th>
                <th>Recommended Draft</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Qwen3-8B-Q8_0</td>
                <td>Qwen3-0.6B-Q8_0</td>
              </tr>
              <tr>
                <td>Qwen3.5-35B-A3B-Q8_K_XL</td>
                <td>Qwen3.5-35B-A3B-UD-Q2_K_XL</td>
              </tr>
            </tbody>
          </table>
          <p>The second example uses the same MoE architecture at lower quantization, which shares more of the target's weight structure and produces higher acceptance rates than a smaller dense model.</p>
          <h4 id="performance-characteristics">Performance Characteristics</h4>
          <p>Speculative decoding helps most when the target model is large relative to the draft. For dense models where the target is already fast (e.g., 8B at 33+ TPS), the overhead of running a draft model may not provide a net speedup. MoE models with large parameter counts but sparse activation (e.g., 30B-A3B) are better candidates, but only when using a high-quality draft.</p>
          <p>The <code>ndraft</code> parameter controls how many candidates to generate. Higher values increase the potential speedup but also increase wasted work when predictions are rejected. The default of 5 is a good starting point; tune based on your observed acceptance rates.</p>
          <h3 id="313-sampling-parameters">3.13 Sampling Parameters</h3>
          <p>Sampling parameters control the randomness and quality of generated text. These are set per-request in the API call.</p>
          <p>For most models you will want to touch these basic sampling parameters. There are <a href="chapter-09-request-parameters.md">many more</a> which will be presented later.</p>
          <h4 id="temperature">Temperature</h4>
          <p>Temperature controls how "random" the model's output is. At each step, the model produces a probability distribution over all possible next tokens. Temperature scales those probabilities — lower values sharpen the distribution (making the top choice dominant), higher values flatten it (giving lower-ranked tokens a better chance of being selected).</p>
          <pre className="code-block"><code className="language-json">{`{
  "temperature": 0.8
}`}</code></pre>
          <ul>
            <li><code>0.0 - 0.3</code> - Focused, deterministic (good for code, factual Q&A)</li>
            <li><code>0.5 - 0.8</code> - Balanced (good for general chat)</li>
            <li><code>0.9 - 1.2</code> - Creative (good for storytelling, brainstorming)</li>
          </ul>
          <h4 id="top-k-and-top-p">Top-K and Top-P</h4>
          <p>After temperature adjusts the probability distribution, Top-K and Top-P narrow the pool of tokens the model can choose from. They work together — Top-K sets a hard cap on how many tokens to consider, and Top-P trims that pool further by cumulative probability.</p>
          <pre className="code-block"><code className="language-json">{`{
  "top_k": 40,
  "top_p": 0.9
}`}</code></pre>
          <ul>
            <li><code>top_k</code> - Consider only the K most probable tokens (default: 40). Lower values make output more focused, higher values allow more variety.</li>
            <li><code>top_p</code> - After Top-K filtering, keep only enough tokens so their combined probability reaches P (default: 0.9). This removes the long tail of unlikely tokens while adapting to how confident the model is at each step.</li>
          </ul>
          <h4 id="repetition-control">Repetition Control</h4>
          <p>Models sometimes get stuck in loops, repeating the same word, phrase, or sentence pattern. Repetition penalty discourages this by reducing the probability of tokens that have already appeared in recent output.</p>
          <pre className="code-block"><code className="language-json">{`{
  "repeat_penalty": 1.1,
  "repeat_last_n": 64
}`}</code></pre>
          <ul>
            <li><code>repeat_penalty</code> - How strongly to penalize repeated tokens. <code>1.0</code> means no penalty, <code>1.1</code> is a mild penalty that works well for most use cases. Values above <code>1.3</code> can start to make output sound unnatural.</li>
            <li><code>repeat_last_n</code> - How many recent tokens to check for repeats (default: 64). A larger window catches longer repeated patterns but may over-penalize common words like "the" or "is."</li>
          </ul>
          <h4 id="dry-sampler-dont-repeat-yourself">DRY Sampler (Don't Repeat Yourself)</h4>
          <p>DRY is a more targeted approach to repetition than <code>repeat_penalty</code>. Instead of penalizing individual repeated tokens, it detects repeated n-gram sequences (multi-word patterns) and penalizes them with an exponentially increasing cost. This catches structural repetition — like repeated sentences or paragraphs — while leaving single-word reuse alone.</p>
          <pre className="code-block"><code className="language-json">{`{
  "dry_multiplier": 1.05,
  "dry_base": 1.75,
  "dry_allowed_length": 2
}`}</code></pre>
          <ul>
            <li><code>dry_multiplier</code> - Strength of the penalty. <code>0</code> disables DRY. Start with <code>1.05</code> and increase if you still see repeated patterns.</li>
            <li><code>dry_base</code> - Controls how fast the penalty grows for longer repeated sequences. Higher values penalize longer repeats more aggressively.</li>
            <li><code>dry_allowed_length</code> - N-grams up to this length are allowed to repeat without penalty (default: 2). This prevents penalizing common short phrases like "of the" or "it is."</li>
          </ul>
          <h4 id="max-tokens">Max Tokens</h4>
          <p>Controls the maximum number of tokens the model will generate in a single response. Once the limit is reached, generation stops. This is useful for controlling costs, response time, and preventing runaway output.</p>
          <pre className="code-block"><code className="language-json">{`{
  "max_tokens": 2048
}`}</code></pre>
          <p>If not set, the model will generate until it produces a stop token or reaches the context window limit.</p>
          <h3 id="314-model-config-file-example">3.14 Model Config File Example</h3>
          <p><code>~/.kronk/model_config.yaml</code> is a flat map keyed by canonical model id (<code>provider/modelID</code>, optionally with a <code>/variant</code> suffix). Each entry's fields use kebab-case YAML keys that map 1:1 to <code>model.Config</code>:</p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/model_config.yaml

Qwen/Qwen3-8B-Q8_0:
  context-window: 32768
  nbatch: 2048
  nubatch: 512
  nseq-max: 2
  cache-type-k: q8_0
  cache-type-v: q8_0
  flash-attention: enabled
  incremental-cache: true
  sampling-parameters:
    temperature: 0.7
    top_p: 0.8
    top_k: 20

Qwen/Qwen3-8B-Q8_0/YARN:
  context-window: 131072
  rope-scaling-type: yarn
  yarn-orig-ctx: 32768

unsloth/Ministral-3-14B-Instruct-2512-Q4_0:
  context-window: 8192
  ngpu-layers: 0
  split-mode: row
  offload-kqv: true`}</code></pre>
          <p>The <code>/YARN</code> suffix is a <strong>config variant</strong> — multiple entries can target the same on-disk model and be selected per request by passing the full variant name as the <code>model</code> field.</p>
          <p>Kronk seeds this file from an embedded default on first server start; your edits are preserved across upgrades. To point at an alternative file for testing, use the <code>--model-config-file</code> server flag:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --model-config-file=./my-test-config.yaml`}</code></pre>
          <hr />
          <h2 id="chapter-4-batch-processing">Chapter 4: Batch Processing</h2>
          <p>Batch processing allows Kronk to handle multiple concurrent requests efficiently by sharing model resources. This chapter explains the architecture and how to optimize for your workload.</p>
          <h3 id="41-architecture-overview">4.1 Architecture Overview</h3>
          <p>For text inference models (including vision/audio), Kronk always creates a batch engine with <code>NSeqMax</code> slots (defaulting to 1). <code>NSeqMax</code> controls how many sequences are processed in parallel within a single model instance.</p>
          <pre className="code-block"><code>{`                    ┌───────────────────────────────────┐
    Request 1 ─────▶│                                   │
                    │          Request Queue            │   Incoming requests are buffered.
    Request 2 ─────▶│      (capacity: NSeqMax × 2)      │   R3 waits because all slots are
                    │                                   │   occupied (NSeqMax=2).
Request 3 (WAIT) ──▶│                                   │
                    └────────────────┬──────────────────┘
                                     │
                                     ▼
                    ┌───────────────────────────────────┐
                    │            Batch Engine           │
                    │                                   │
                    │  ┌───────────┐    ┌───────────┐   │
                    │  │  Slot 0   │    │  Slot 1   │   │   Each request is assigned to a slot.
                    │  │   (R1)    │    │   (R2)    │   │   The slot tracks prompt tokens,
                    │  │  seqID=0  │    │  seqID=1  │   │   decode position, and sampler state.
                    │  └─────┬─────┘    └─────┬─────┘   │
                    │        │                │         │
                    │        ▼                ▼         │
                    │  ┌───────────┐    ┌───────────┐   │
                    │  │ KV Cache  │    │ KV Cache  │   │   Each slot writes to its own KV cache
                    │  │   (R1)    │    │   (R2)    │   │   partition, isolated by sequence ID.
                    │  │   seq0    │    │   seq1    │   │   Requests never share attention state.
                    │  └─────┬─────┘    └─────┬─────┘   │
                    │        │                │         │
                    │        └───────┬────────┘         │
                    │                ▼                  │
                    │        ┌────────────────┐         │   Tokens from all active slots are
                    │        │  Decode Loop   │         │   collected into a single batch using
                    │        │(parallel batch)│         │   round-robin n_ubatch-sized chunks
                    │        └───────┬────────┘         │   and decoded together each iteration.
                    └────────────────┼──────────────────┘
                                     │
                                     ▼
                    ┌───────────────────────────────────┐   llama.cpp processes the full batch
                    │         llama.cpp Backend         │   on the GPU, computing all sequences
                    │        (GPU/CPU Inference)        │   in parallel in one forward pass.
                    └───────────────────────────────────┘`}</code></pre>
          <h3 id="42-slots-and-sequences">4.2 Slots and Sequences</h3>
          <p>The batch engine divides its capacity into slots and sequences. Together they provide the mechanism for processing multiple requests concurrently while keeping each request's data isolated inside the shared KV cache.</p>
          <p><strong>Slots</strong> are processing units that handle individual requests. Each slot tracks its own state: prompt tokens, decode position, sampler, and response channel.</p>
          <p><strong>Sequences</strong> are isolated partitions in the shared KV cache. Each slot is assigned a unique sequence ID, ensuring requests don't interfere with each other's attention state.</p>
          <p>The slot/sequence layout is the same for all caching strategies in Kronk:</p>
          <pre className="code-block"><code>{`NSeqMax = 4

Slot 0  →  seqID = 0  →  KV cache partition 0
Slot 1  →  seqID = 1  →  KV cache partition 1
Slot 2  →  seqID = 2  →  KV cache partition 2
Slot 3  →  seqID = 3  →  KV cache partition 3`}</code></pre>
          <p>How a slot uses its sequence depends on the caching strategy. Without caching, the sequence is cleared between requests. With IMC, all sessions (text and media) externalize their cached KV state to RAM after each request and restore it into any available slot on the next request. See <a href="#37-parallel-inference-nseqmax">Section 3.7</a> for details on how each caching strategy affects slot behavior.</p>
          <h3 id="43-request-flow">4.3 Request Flow</h3>
          <p>Each request moves through the batch engine in the following stages:</p>
          <ol>
            <li><strong>Queue</strong>: Request enters the queue (backpressure if full)</li>
            <li><strong>Assign</strong>: Available slot picks up the request</li>
            <li><strong>Cache Setup</strong>: Prepare the slot's sequence based on caching strategy:
              <ul>
                <li>Clear the sequence (no caching)</li>
                <li>IMC: restore cached KV from RAM, extend or rebuild</li>
              </ul>
            </li>
            <li><strong>Prefill</strong>: Tokenize and process remaining prompt tokens (round-robin across slots in <code>n_ubatch</code>-sized chunks to prevent starvation)</li>
            <li><strong>Decode</strong>: Generate tokens one at a time, streaming to client</li>
            <li><strong>Complete</strong>: Release the slot:
              <ul>
                <li>Clear the entire sequence (no caching)</li>
                <li>IMC (all model types): clear the entire VRAM sequence (cached prefix already snapshotted to RAM)</li>
              </ul>
            </li>
          </ol>
          <h3 id="44-configuring-batch-processing">4.4 Configuring Batch Processing</h3>
          <p>Batch processing is controlled primarily through the model configuration. The key setting is <code>NSeqMax</code>, which determines how many slots the batch engine creates and therefore how many requests can be processed in parallel. Increasing <code>NSeqMax</code> improves concurrency but requires proportionally more KV cache memory, so it's important to balance throughput against available VRAM.</p>
          <h4 id="enable-batch-processing">Enable Batch Processing</h4>
          <p>By default, the batch engine runs with a single slot (<code>NSeqMax=1</code>). To enable parallel request processing, set <code>nseq-max &gt; 1</code> in <code>~/.kronk/model_config.yaml</code>:</p>
          <pre className="code-block"><code className="language-yaml">{`Qwen/Qwen3-8B-Q8_0:
  nseq-max: 4 # 4 concurrent requests`}</code></pre>
          <h4 id="queue-depth">Queue Depth</h4>
          <p>A bounded request queue sits in front of the batch engine to absorb bursts of incoming requests without rejecting them immediately.</p>
          <p>The request queue holds <code>NSeqMax × 2</code> requests by default. With <code>NSeqMax=4</code>, up to 8 requests can be in-flight: 4 actively processing in slots and 4 waiting in the queue. This multiplier is configurable via <code>WithQueueDepth</code> when using the SDK:</p>
          <pre className="code-block"><code className="language-go">{`krn, err := kronk.New(ctx, cfg, kronk.WithQueueDepth(3))`}</code></pre>
          <p>When all slots and queue positions are occupied, new requests block until a slot becomes available or the request's context is cancelled. If a queued request waits longer than <code>CacheSlotTimeout</code> (default: 30 seconds), the engine preempts the longest-running slot — cancelling that in-flight request with a "preempted by queued request" error — and assigns the slot to the waiting request. If the engine is shutting down, queued requests receive an immediate error. This backpressure and preemption mechanism prevents any single request from starving others indefinitely.</p>
          <h4 id="memory-and-caching">Memory and Caching</h4>
          <p>Adding slots increases throughput but costs memory. Each additional slot allocates its own KV cache partition proportional to the full context window.</p>
          <p>Each slot reserves its own KV cache partition, so increasing <code>NSeqMax</code> increases VRAM usage proportionally. IMC does not add extra sequences. For details on how slot memory is allocated and how to estimate total VRAM, see <a href="chapter-03-model-configuration.md#37-parallel-inference-nseqmax">Section 3.7 — Parallel Inference (NSeqMax)</a> and <a href="chapter-03-model-configuration.md#310-vram-estimation">Section 3.10 — VRAM Estimation</a>.</p>
          <h3 id="45-concurrency-by-model-type">4.5 Concurrency by Model Type</h3>
          <p>Not all model types achieve concurrency the same way. Text inference models (including vision and audio) use the batch engine described in the previous sections, where multiple slots share a single model context and their tokens are combined into one decode call. Embedding and reranking models take a different approach — they create a pool of independent contexts that each process requests separately. The table below summarizes the distinction, and the diagrams that follow show the request flow for each approach.</p>
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
                <td>Vision/Audio</td>
                <td>Batch parallelism</td>
                <td>Shared model, multiple slots</td>
              </tr>
              <tr>
                <td>Embedding</td>
                <td>Context pool</td>
                <td>Shared weights, multiple contexts</td>
              </tr>
              <tr>
                <td>Reranking</td>
                <td>Context pool</td>
                <td>Shared weights, multiple contexts</td>
              </tr>
            </tbody>
          </table>
          <h4 id="embeddingrerank-request-flow-nseqmax=4">Embedding/Rerank Request Flow (NSeqMax=4)</h4>
          <p>Embedding and reranking models don't use the batch engine. Instead, Kronk creates a pool of independent contexts — one per <code>NSeqMax</code> slot. When a request arrives, it acquires a context from the pool, processes its inputs, and releases the context back. If all contexts are in use, the request blocks until one becomes available. The following diagram shows this flow:</p>
          <pre className="code-block"><code>{`                    ┌──────────────────────────────────┐
   Request 1 ──────▶│                                  │   Requests acquire a context from the
                    │           Context Pool           │   pool. If all contexts are in use,
   Request 2 ──────▶│       (capacity: NSeqMax)        │   the request blocks until one is
                    │                                  │   released.
Request 3 (WAIT) ──▶│                                  │
                    └────────────────┬─────────────────┘
                                     │
                                     ▼
                    ┌──────────────────────────────────┐
                    │     Independent Contexts         │
                    │                                  │
                    │  ┌───────────┐    ┌───────────┐  │   Each context has its own KV cache.
                    │  │ Context 0 │    │ Context 1 │  │   Unlike the batch engine, there is
                    │  │   (R1)    │    │   (R2)    │  │   no shared state between contexts.
                    │  └─────┬─────┘    └─────┬─────┘  │
                    │        │                │        │
                    │        ▼                ▼        │
                    │  ┌───────────┐    ┌───────────┐  │   Each request runs its own decode
                    │  │  Decode   │    │  Decode   │  │   call independently. Efficiency
                    │  │   (R1)    │    │   (R2)    │  │   comes from sharing model weights,
                    │  └─────┬─────┘    └─────┬─────┘  │   not from batching work together.
                    │        │                │        │
                    └────────┼────────────────┼────────┘
                             │                │
                             ▼                ▼
                    ┌──────────────────────────────────┐   llama.cpp processes each context
                    │         llama.cpp Backend        │   separately on the GPU. Model weights
                    │        (GPU/CPU Inference)       │   are shared, only KV cache is per-ctx.
                    └──────────────────────────────────┘`}</code></pre>
          <p>Unlike the batch engine, each request runs its own separate decode call — there is no combining of work across requests. The efficiency comes from sharing the model weights across all contexts, so only the KV cache memory is duplicated.</p>
          <h3 id="46-performance-tuning">4.6 Performance Tuning</h3>
          <p>The right <code>NSeqMax</code> value depends on your workload. More slots increase throughput by serving more requests in parallel, but each additional slot shares the same GPU, so individual requests may take slightly longer to complete. The goal is to find the balance point where you have enough concurrency for your users without saturating the GPU or running out of VRAM.</p>
          <p><strong>Throughput vs Latency</strong></p>
          <ul>
            <li>Higher <code>NSeqMax</code>: Better throughput, potentially higher per-request latency</li>
            <li>Lower <code>NSeqMax</code>: Lower latency, less concurrent capacity</li>
          </ul>
          <p><strong>Recommended Settings</strong></p>
          <ul>
            <li>Single user, interactive: <code>nseq-max: 1-2</code></li>
            <li>Multi-user API server: <code>nseq-max: 4-8</code></li>
            <li>High-throughput batch jobs: <code>nseq-max: 8-16</code></li>
          </ul>
          <p><strong>Monitoring</strong></p>
          <p>Use request tracing to watch for long <code>queue-wait</code> spans, which indicate requests are waiting for an available slot. If you see consistently long queue waits, consider:</p>
          <ol>
            <li>Increasing <code>nseq-max</code> (if VRAM allows)</li>
            <li>Reducing <code>context-window</code> to fit more slots</li>
            <li>Using KV cache quantization (<code>cache-type-k</code>/<code>cache-type-v: q8_0</code>)</li>
          </ol>
          <p>See <a href="#chapter-14-observability">Chapter 14: Observability</a> for details on tracing and metrics.</p>
          <h3 id="47-example-configuration">4.7 Example Configuration</h3>
          <p>The following config shows a high-throughput setup that balances concurrency, memory, and caching for a multi-user API server:</p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/model_config.yaml
Qwen/Qwen3-8B-Q8_0:
  context-window: 8192
  nseq-max: 8
  nbatch: 2048
  nubatch: 512
  cache-type-k: q8_0
  cache-type-v: q8_0
  incremental-cache: true`}</code></pre>
          <p>This configuration handles 8 concurrent requests, uses quantized KV cache to reduce memory, and caches conversations incrementally for faster prefill. Here is the VRAM estimate (see <a href="chapter-03-model-configuration.md#310-vram-estimation">Section 3.10 — VRAM Estimation</a> for the full formula):</p>
          <pre className="code-block"><code>{`Model                   : Qwen3-8B-Q8_0
Model Weights           : ~9 GB
Context Window (n_ctx)  : 8,192
Bytes Per Element       : 1 (q8_0)
block_count (n_layers)  : 36
attention.head_count_kv : 8
attention.key_length    : 128
attention.value_length  : 128

Step 1 — Per-token-per-layer cost:

  KV_Per_Token_Per_Layer = 8 × (128 + 128) × 1 = 2,048 bytes

Step 2 — Per-sequence cost:

  KV_Per_Sequence = 8,192 × 36 × 2,048 = ~0.6 GB

Step 3 — Total KV cache (NSeqMax = 8):

  Slot_Memory = 8 × 0.6 GB = ~4.8 GB

Step 4 — Total VRAM:

  Total_VRAM = 9.0 GB + 4.8 GB = ~13.8 GB`}</code></pre>
          <h3 id="48-imc-slot-scheduling">4.8 IMC Slot Scheduling</h3>
          <p>When IMC is enabled, the batch engine uses a scheduling algorithm to assign requests to slots. This section explains how IMC scheduling works and the mechanisms that prevent requests from stalling.</p>
          <h4 id="normal-scheduling-no-caching">Normal Scheduling (No Caching)</h4>
          <p>Without IMC, the algorithm assigns the next queued request to any available slot. If all slots are busy, the request stays in the queue until a slot finishes. This is simple and works well because requests have no slot affinity.</p>
          <h4 id="imc-scheduling">IMC Scheduling</h4>
          <p>All IMC requests have no slot affinity — cached KV state is externalized to RAM and can be restored into any available slot. These requests are scheduled identically to non-IMC requests (first available slot).</p>
          <h4 id="slot-preemption">Slot Preemption</h4>
          <p>If all slots are busy when a queued job needs to be assigned, and the job waits longer than <code>CacheSlotTimeout</code> seconds (default: 30), the algorithm triggers preemption. This is a safety mechanism for pathologically long generations.</p>
          <p>Preemption uses a two-phase approach for safety:</p>
          <ol>
            <li><strong>Schedule</strong> — The algorithm marks the victim slot for preemption and defers the waiting job. No slot state is modified yet.</li>
            <li><strong>Execute</strong> — At the top of the next processing loop iteration, after the batch is cleared but before any tokens are added, the victim slot is finished with a preemption error. This ordering is critical — the victim slot must have no tokens in the current batch, otherwise cleaning up its KV state could corrupt a subsequent decode.</li>
          </ol>
          <p>The preempted request receives an error response and the client can retry. The waiting job is then assigned to the freed slot. The longest-running slot is preempted.</p>
          <h4 id="cacheslottimeout">CacheSlotTimeout</h4>
          <p>The <code>cache_slot_timeout</code> setting (default: 30 seconds) controls two distinct timeout scenarios in the IMC scheduling path:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Scenario</th>
                <th>Phase</th>
                <th>What Happens at Timeout</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Wait for slot available</td>
                <td>Before batch queue</td>
                <td>Error returned: "server busy"</td>
              </tr>
              <tr>
                <td>Queued job waiting</td>
                <td>Inside batch queue</td>
                <td>Longest-running slot preempted, job assigned</td>
              </tr>
            </tbody>
          </table>
          <pre className="code-block"><code>{`                          CacheSlotTimeout (30s)
                          ┌──────────────────────────────────────┐
                          │                                      │
    ┌─────────────────────┼──────────────────┐   ┌───────────────┼──────────────┐
    │  Before Batch Queue │                  │   │ Inside Batch  │              │
    │                     │                  │   │ Queue         │              │
    │  All slots have     │                  │   │  All slots    │              │
    │  cache builds       ▼                  │   │  are busy     ▼              │
    │  in-flight     ──► Error               │   │  generating ──► Preempt      │
    │                     "server busy"      │   │                victim slot   │
    └────────────────────────────────────────┘   └──────────────────────────────┘`}</code></pre>
          <p>The first scenario fires before the job enters the batch engine — it blocks during cache preparation when all IMC sessions have pending cache builds in-flight. The second scenario fires inside the batch engine — the job is already queued but all slots are actively generating tokens for other requests.</p>
          <p><strong>Important:</strong> The preemption timeout is measured from when the job enters the batch engine queue, not from when the HTTP request arrived. Time spent waiting for cache builds does not count against the preemption budget. This prevents false preemptions when a request waits for a long cache build before entering the queue.</p>
          <h4 id="debugging-imc-scheduling">Debugging IMC Scheduling</h4>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Log Message</th>
                <th>Meaning</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>all slots pending, waiting for slot</code></td>
                <td>Waiting for a cache build to finish (timeout 1)</td>
              </tr>
              <tr>
                <td><code>slot became available, retrying</code></td>
                <td>A cache build finished, retrying slot scan</td>
              </tr>
              <tr>
                <td><code>server busy</code></td>
                <td>Wait for slot timed out (timeout 1)</td>
              </tr>
              <tr>
                <td><code>preempting-slot</code></td>
                <td>Preemption scheduled (timeout 2, shows wait time)</td>
              </tr>
              <tr>
                <td><code>preempted by queued request</code></td>
                <td>Victim slot finished with preemption error</td>
              </tr>
              <tr>
                <td><code>slot-finished</code> (after preemption)</td>
                <td>Victim cleaned up, slot available for deferred job</td>
              </tr>
            </tbody>
          </table>
          <h3 id="49-model-types-and-state-management">4.9 Model Types and State Management</h3>
          <p>Kronk supports three model architectures. The model type is detected automatically at load time and affects how the batch engine manages sequence state. The caching system's session matching and cache building are the same for all model types — the difference is in the batch engine's cleanup behavior after a request completes.</p>
          <p><strong>All IMC sessions</strong> (text and media) use the same lifecycle for all model types: the cached prefix is snapshotted to RAM (via <code>StateSeqGetData</code>) during slot initialization, and the entire VRAM sequence is cleared after the request completes. The next request restores the cached state from RAM into any available slot. <code>StateSeqGetData</code> captures raw KV bytes regardless of whether they originated from text tokens or media embeddings. For Hybrid models, <code>StateSeqGetData</code> captures both KV cache and recurrent state (DeltaNet/SSM), so the unified snapshot/restore path naturally handles them.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Model Type</th>
                <th>Architecture</th>
                <th>IMC Cleanup</th>
                <th>Detection</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Dense</td>
                <td>Standard transformer</td>
                <td>Full clear (snapshot)</td>
                <td>Default (not MoE, not Hybrid)</td>
              </tr>
              <tr>
                <td>MoE</td>
                <td>Mixture of Experts</td>
                <td>Full clear (snapshot)</td>
                <td>GGUF <code>expert_count</code> metadata</td>
              </tr>
              <tr>
                <td>Hybrid</td>
                <td>Attention + Recurrent (DeltaNet/SSM)</td>
                <td>Full clear (snapshot)</td>
                <td><code>llama.ModelIsHybrid</code></td>
              </tr>
            </tbody>
          </table>
          <h4 id="snapshot-to-ram-all-model-types">Snapshot to RAM (All Model Types)</h4>
          <p>All IMC sessions use the same snapshot/restore approach regardless of model type or content type (text or media):</p>
          <ol>
            <li><strong>Snapshot</strong>: After the IMC cache is built or extended but before suffix tokens are decoded, the engine captures the full sequence state (KV cache and recurrent hidden state for Hybrid models) into a byte buffer in RAM via <code>StateSeqGetData</code>.</li>
            <li><strong>Clear</strong>: After the request completes, the entire VRAM sequence is cleared. The cached prefix lives in the session's RAM buffer.</li>
            <li><strong>Restore</strong>: On the next request, the cached state is restored from RAM into any available slot via <code>StateSeqSetData</code>.</li>
          </ol>
          <pre className="code-block"><code>{`IMC (all types, all content): Snapshot to RAM → Clear VRAM → Restore into any slot`}</code></pre>
          <p>The only difference is that when a new media message appears in the conversation, the cache is rebuilt through the mtmd pipeline (projection model encodes image/audio into embeddings).</p>
          <p>The snapshot/restore is a memory copy operation, typically 10-30ms depending on conversation size.</p>
          <h4 id="partial-prefix-rebuilds-hybrid">Partial Prefix Rebuilds (Hybrid)</h4>
          <p>Partial prefix matches are more expensive for hybrid models because the recurrent state must be rebuilt from the beginning.</p>
          <p>When a request matches a partial token prefix (the token prefix fallback path), Dense/MoE models trim from the divergence point and re-decode only the new tokens. Hybrid models cannot do partial trims, so the engine performs a full sequence clear and re-decodes the entire cached token sequence from position 0. This is more expensive but guarantees the recurrent state is built correctly.</p>
          <h4 id="moe-performance-characteristics">MoE Performance Characteristics</h4>
          <p>While MoE models share the same state management as Dense, their architecture introduces unique performance trade-offs worth understanding.</p>
          <p>MoE models use the same state management as Dense (snapshot/restore), but have different performance profiles that affect configuration:</p>
          <ul>
            <li>Lower tokens/sec than comparably-sized dense models on Apple Silicon due to scattered memory access patterns from expert routing</li>
            <li>Sensitive to aggressive KV cache quantization — use <code>f16</code> cache types if quality degrades with <code>q8_0</code></li>
            <li>Use <code>split_mode: row</code> for multi-GPU setups to enable expert-parallel execution</li>
          </ul>
          <h4 id="hybrid-constraints">Hybrid Constraints</h4>
          <p>Hybrid models have hard requirements that Kronk enforces at load time.</p>
          <ul>
            <li>KV cache must use <code>f16</code> — quantized cache types (e.g., <code>q8_0</code>) are incompatible with recurrent layers</li>
            <li>Flash attention is automatically disabled</li>
          </ul>
          <h4 id="hybrid-guardrails">Hybrid Guardrails</h4>
          <p>Kronk protects against corrupted state by automatically recovering when snapshot operations fail.</p>
          <p>If a snapshot restore fails, Kronk clears the session's IMC metadata so the session is not reused with a corrupted state. The next request for that session triggers a full cache rebuild from scratch.</p>
          <h3 id="410-debugging-state-management">4.10 Debugging State Management</h3>
          <p>Use these log messages to diagnose how the batch engine is managing KV cache state between requests. Snapshot/restore is used by all IMC sessions (to externalize KV state to RAM). These messages are especially useful when restore failures trigger expensive full rebuilds.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Log Message</th>
                <th>Meaning</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>imc-hybrid-snapshot</code></td>
                <td>State captured after cache build (shows snapshot_bytes)</td>
              </tr>
              <tr>
                <td><code>imc-hybrid-snapshot-failed</code></td>
                <td>StateSeqGetData returned 0 bytes</td>
              </tr>
              <tr>
                <td><code>imc-hybrid-restore</code></td>
                <td>Snapshot restored after request (shows restored_bytes)</td>
              </tr>
              <tr>
                <td><code>imc-hybrid-restore-failed</code></td>
                <td>StateSeqSetData failed, slot metadata cleared</td>
              </tr>
              <tr>
                <td><code>imc-hybrid-no-snapshot</code></td>
                <td>No snapshot available, full clear + metadata invalidation</td>
              </tr>
              <tr>
                <td><code>imc-hybrid-rebuild</code></td>
                <td>Partial prefix: full clear + re-decode from position 0</td>
              </tr>
              <tr>
                <td><code>imc-hybrid-trim-rebuild</code></td>
                <td>Trim-only prefix: full clear + re-decode truncated sequence</td>
              </tr>
            </tbody>
          </table>
          <hr />
          <h2 id="chapter-5-message-caching">Chapter 5: Message Caching</h2>
          <p>Message caching reduces redundant computation by storing and reusing KV cache state from previous requests.</p>
          <h3 id="51-overview">5.1 Overview</h3>
          <p>When processing a chat request, the model must compute attention for every token in the conversation. Without caching, the entire prompt is prefilled on every request — even tokens the model has already seen.</p>
          <p><em>Note: Prefill is the phase where the model processes all input tokens (system prompt, conversation history, and the new message) before it begins generating a response. This is the most computationally expensive part of a request, and its cost grows with the number of input tokens.</em></p>
          <p>Kronk provides the Incremental Message Cache (IMC) to reduce redundant prefill work. <strong>IMC is enabled by default for all models.</strong> IMC maintains logical sessions — one per conversation branch — and caches the full message history so only the new message needs to be prefilled. All sessions (text and media) externalize their cached KV state to RAM after each request and restore it into any available slot on the next request. <code>StateSeqGetData</code> captures the raw KV bytes regardless of whether they originated from text tokens or media embeddings.</p>
          <pre className="code-block"><code>{`No Caching:
┌─────────────────────────────────────────────────────┐
│ System Prompt │ Message 1 │ Message 2 │ New Message │
│   (prefill)   │ (prefill) │ (prefill) │  (prefill)  │
└─────────────────────────────────────────────────────┘
                                              ↓
                                           Generate

IMC (Incremental Message Cache):
┌─────────────────────────────────────────────────────┐
│ System Prompt │ Message 1 │ Message 2 │ New Message │
│   (cached)    │ (cached)  │ (cached)  │  (prefill)  │
└─────────────────────────────────────────────────────┘
                                              ↓
                                           Generate`}</code></pre>
          <h3 id="52-incremental-message-cache-imc">5.2 Incremental Message Cache (IMC)</h3>
          <p>Incremental Message Cache is designed for agentic workflows. It caches all messages except the last one and extends the cache incrementally on each turn. When a client or agent mutates the conversation history, IMC uses a two-tier hash to preserve the system prompt KV state and only rebuild the conversation body.</p>
          <p><strong>Key Terminology:</strong></p>
          <ul>
            <li><strong>Session</strong> — logical IMC conversation branch with its own metadata (hash, token count, message index). Decoupled from physical slots.</li>
            <li><strong>Slot</strong> — physical batch-engine execution lane. Any session (text or media) can run on any available slot.</li>
            <li><strong>Sequence / seqID</strong> — llama.cpp KV cache partition attached to the active slot during request processing.</li>
          </ul>
          <h4 id="two-tier-hash-design">Two-Tier Hash Design</h4>
          <p>IMC tracks two independent hashes per session:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Tier</th>
                <th>What It Covers</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Tier 1</td>
                <td>System prompt (<code>messages[0]</code> when role=system)</td>
                <td>Preserved across conversation edits</td>
              </tr>
              <tr>
                <td>Tier 2</td>
                <td>All cached messages (<code>messages[0:N]</code>)</td>
                <td>Detects any change in the conversation</td>
              </tr>
            </tbody>
          </table>
          <p>When a request arrives, IMC first checks the full prefix hash (Tier 2). If it matches, the cache is extended as normal. If the full hash mismatches but the system prompt hash (Tier 1) still matches, IMC keeps the system prompt KV in place and only re-decodes the conversation body after it. This is the most common mutation scenario — the client edits conversation history while keeping the same system prompt.</p>
          <pre className="code-block"><code>{`Normal append (full hash match):
┌─────────────────────────────────────────────────────────┐
│ System Prompt │ Msg 1  │ Msg 2  │ Msg 3  │  New Message │
│   (cached)    │(cached)│(cached)│(cached)│  (prefill)   │
└─────────────────────────────────────────────────────────┘

Conversation edit (sys prompt hash match, full hash mismatch):
┌─────────────────────────────────────────────────────────────────┐
│ System Prompt │ Msg 1'    │ Msg 2'    │ Msg 3'    │ New Message │
│   (cached)    │(re-decode)│(re-decode)│(re-decode)│(prefill)    │
└─────────────────────────────────────────────────────────────────┘
   ↑ kept in KV     ↑ trimmed and rebuilt from sys prompt boundary`}</code></pre>
          <p><strong>How IMC Detects Changes:</strong></p>
          <p>IMC uses a cascading match algorithm. It always tries the fastest path first and automatically falls back to slower-but-more-resilient strategies when the fast path fails:</p>
          <ol>
            <li><strong>Hash match</strong> — Hash the incoming message prefix and compare against each session's stored hash. Instant, zero-tokenization overhead. This is the common case when the conversation grows normally (messages appended, nothing edited).</li>
            <li><strong>System prompt preservation</strong> — If the full hash mismatches but the system prompt hash (Tier 1) still matches, keep the system prompt KV in place and re-decode only the conversation body. This handles the common case where the client edits or drops messages while keeping the same system prompt.</li>
            <li><strong>Token prefix fallback</strong> — If no hash matches at all, tokenize the incoming messages and compare element-by-element against cached sessions to find the longest common prefix. Trim the divergent suffix and decode only the new tokens. This salvages 70-80% of cached tokens when templates, tool call formatting, or client behavior causes token-level differences even though the conversation is logically the same.</li>
            <li><strong>Full rebuild</strong> — No usable match found. Pick an empty session or evict the LRU session and build the cache from scratch.</li>
          </ol>
          <p>The matching algorithm is independent of the model type (Dense, MoE, Hybrid). What changes per model type is how the batch engine manages state between requests — see <a href="#49-model-types-and-state-management">Section 4.9</a>.</p>
          <p><strong>IMC is Best for:</strong></p>
          <ul>
            <li>AI coding agents</li>
            <li>Long-running agent conversations</li>
            <li>Agentic workflows where conversations grow or are edited</li>
            <li>Sub-agent architectures with multiple concurrent agents</li>
          </ul>
          <p><strong>Enable IMC:</strong></p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/model_config.yaml
Qwen/Qwen3-8B-Q8_0:
  incremental-cache: true
  cache-min-tokens: 100 # Minimum tokens before caching (default)`}</code></pre>
          <h4 id="multi-session-architecture">Multi-Session Architecture</h4>
          <p>All <code>NSeqMax</code> sessions are available for IMC. Each session independently tracks its own conversation branch — its own message hash, system prompt hash, token count, and message index. Sub-agents are routed to different sessions via hash matching, allowing them to maintain independent caches.</p>
          <p>Each session externalizes its cached KV state to RAM after the request completes. On the next request, the cached state is restored into any available slot — sessions are not pinned to specific slots. This means all slots are equally eligible for any session, maximizing slot utilization. <code>StateSeqGetData</code> captures raw KV bytes regardless of whether they originated from text tokens or media embeddings.</p>
          <p>With <code>nseq-max: 3</code>, three sub-agents can each have their own cached conversation branch. Without multi-session IMC, every sub-agent request would cause a prefix mismatch and rebuild the cache from scratch because different sub-agents send different system prompts and conversation content.</p>
          <p><strong>Important:</strong> Set <code>nseq-max</code> to at least the number of concurrent sub-agents your agent framework spawns. If <code>nseq-max</code> is smaller than the number of sub-agents, cache thrashing can occur — each new sub-agent evicts a session, and when the evicted sub-agent returns, it evicts another. Every request triggers a full rebuild from scratch, eliminating the caching benefit entirely. With unified KV cache, all slots share the same <code>n_ctx</code> pool, so adding more slots does not multiply VRAM usage. However, more sessions means more cached conversations competing for the shared pool. KV pressure eviction automatically clears stale sessions when space gets tight — see <a href="#kv-pressure-eviction">KV Pressure Eviction</a>.</p>
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
          <p>Fourth request (conversation edited — assistant response removed):</p>
          <pre className="code-block"><code>{`Messages: [system, user, user3]
Cache:    [system]                   ← System prompt KV preserved
Rebuild:  [user, user3]              ← Only conversation body re-decoded
Prefill:  [user3 + gen_prompt]`}</code></pre>
          <h4 id="session-selection-algorithm">Session Selection Algorithm</h4>
          <p>When a request arrives, IMC scans all sessions to find the best match. The algorithm has five steps, tried in order. After a session is selected, the batch engine assigns the request to the first available slot. The session's KV state is restored from RAM into the assigned slot.</p>
          <ol>
            <li><strong>Scan all sessions</strong> — For each session: stored hash Track the session as a system-prompt-match candidate if it does.
              <ul>
                <li>Skip sessions with a build in-flight (pending flag set)</li>
                <li>Skip empty sessions (track them as fallback candidates)</li>
                <li>Skip sessions with more cached messages than the request has total</li>
                <li>Hash <code>messages[:session.cachedMsgCount]</code> and compare to the session's</li>
                <li>On mismatch: check if the system prompt hash (Tier 1) still matches.</li>
                <li>Track mismatched sessions as eviction candidates</li>
              </ul>
            </li>
            <li><strong>KV pressure eviction</strong> — When a matching session is found and the total KV usage across all sessions exceeds the context window, evict mismatched sessions (largest first) to reclaim space. Sessions with externalized <code>kvState</code> do not count against VRAM KV pressure because their VRAM sequences are already cleared. See <a href="#kv-pressure-eviction">KV Pressure Eviction</a> for details.</li>
            <li><strong>On full match</strong> — Pick the session with the best prefix coverage (most cached messages). If the request has new messages to cache, extend the session's cache. If the messages are identical, it's a pure cache hit.</li>
            <li><strong>System prompt preservation (two-tier hash)</strong> — No full match, but a session has the same system prompt cached. Keep the system prompt KV in place, trim everything after the system prompt token boundary, and re-template and re-decode only the conversation body. Before preserving, IMC verifies the system prompt token boundary is consistent after re-templating — if the template produces a different token count for the system prompt, it falls back to a full rebuild.</li>
            <li><strong>Token prefix fallback</strong> — Tokenize the incoming messages and compare the resulting token sequence element-by-element against each non-empty session's stored <code>cachedTokens</code>. Pick the session with the longest common prefix that meets <code>cache-min-tokens</code>. Trim the KV cache from the divergence point and decode only the new tokens from there forward. See <a href="#token-prefix-fallback">Token Prefix Fallback</a> for details.</li>
            <li><strong>No match at all</strong> — Pick an empty session if one exists, otherwise evict the least-recently-used (LRU) session and rebuild from scratch.</li>
          </ol>
          <p><strong>Concurrent Build Protection:</strong></p>
          <p>When two requests arrive simultaneously and both need to build a cache from scratch, a race condition could cause both to pick the same empty session. IMC prevents this with a pending flag: when a session begins a deferred cache build, it is marked pending. Concurrent scanners skip pending sessions, so the second request picks a different session. The pending flag is cleared after the cache decode completes (or on error).</p>
          <p><strong>Decode Failure Recovery:</strong></p>
          <p>If a cache decode fails at any point (extend, rebuild, trim, or media build), IMC clears the entire KV sequence and resets the session metadata. This ensures the slot never advertises cached content that doesn't exist in the KV cache.</p>
          <h4 id="kv-pressure-eviction">KV Pressure Eviction</h4>
          <p>With <code>nseq-max &gt; 1</code>, Kronk enables a unified KV cache (<code>KVUnified=1</code>) so that all sequences share the full <code>n_ctx</code> pool. Any single sequence can grow up to the full context window, but the <strong>total</strong> KV usage across all sequences cannot exceed <code>n_ctx</code>.</p>
          <p>All sessions externalize their KV state to RAM after each request and clear their VRAM sequence, so they do not contribute to VRAM KV pressure between requests. However, during active processing, a session's restored KV does consume VRAM cells until the request completes and the state is externalized again.</p>
          <p><strong>Example:</strong> With <code>nseq-max: 3</code> and <code>context-window: 131072</code>:</p>
          <pre className="code-block"><code>{`Session 0: 854 tokens    (stale media — 2 cached messages, hash mismatch)
Session 1: 46,541 tokens (stale media — 17 cached messages, hash mismatch)
Session 2: 86,682 tokens (active media — 49 cached messages, hash match)
Total VRAM-resident: 134,077 tokens > 131,072 → context window full!`}</code></pre>
          <p>Without KV pressure eviction, the next decode would fail with "context window is full" even though the active conversation only uses ~87k of the 131k window.</p>
          <p><strong>How It Works:</strong></p>
          <p>After the session scan finds a matching session (Step 1), IMC checks whether the projected total KV usage across all sessions exceeds the context window. If it does, mismatched sessions are evicted largest-first until the total fits:</p>
          <ol>
            <li>Sum <code>totalTokensCached</code> across all non-empty, non-pending sessions (sessions with externalized <code>kvState</code> are excluded since their VRAM is already freed)</li>
            <li>If the sum exceeds <code>context-window</code>, sort mismatched sessions by token count (descending)</li>
            <li>Evict sessions one at a time — clear the KV sequence (<code>MemorySeqRm</code>) and reset the session metadata — until the projected total is within bounds</li>
          </ol>
          <p>In the example above, evicting Session 1 (46,541 tokens) brings the total to 87,536 — well within the 131,072 limit. Session 0 (854 tokens) may or may not need eviction depending on the remaining headroom.</p>
          <p><strong>Key Points:</strong></p>
          <ul>
            <li>Eviction only targets <strong>mismatched</strong> sessions — the active session and any other matching sessions are never evicted</li>
            <li>Pending sessions (with a build in-flight) are never evicted</li>
            <li>Sessions with externalized <code>kvState</code> do not count toward VRAM pressure and are not eviction candidates (their VRAM is already freed)</li>
            <li>Evicted sessions become empty and are available for future cache builds</li>
            <li>The eviction check runs before the extend/hit path, so the active session always has room to grow</li>
            <li>No configuration needed — eviction triggers automatically when KV pressure is detected</li>
          </ul>
          <h4 id="token-prefix-fallback">Token Prefix Fallback</h4>
          <p>When hash matching fails — whether because the client edited messages, a template produced slightly different tokens, or the agent didn't send exactly the same conversation — IMC falls back to token-level prefix matching to salvage as much of the cached KV state as possible.</p>
          <p><strong>When it activates:</strong> Automatically when no hash match and no system prompt match is found during the session scan (Step 5 of the <a href="#session-selection-algorithm">Session Selection Algorithm</a>). IMC compares the actual cached token arrays against the incoming request's tokens. Only candidates with compatible message counts are considered — the request must have at least as many messages as the session cached.</p>
          <p><strong>How it works:</strong></p>
          <p>IMC tokenizes the incoming messages and compares them element-by-element against each non-empty session's stored token sequence to find the longest common prefix.</p>
          <pre className="code-block"><code>{`Cached tokens:   [T1, T2, T3, T4, T5, T6, T7, T8]
Incoming tokens: [T1, T2, T3, T4, T5, T9, T10, T11, T12]
                                       ↑
                              Divergence point (pos 5)

Common prefix: 5 tokens (salvaged from KV cache)
Trimmed:       3 tokens (T6-T8 removed from KV cache)
New decode:    4 tokens (T9-T12, from divergence point forward)`}</code></pre>
          <p>If the common prefix meets the <code>cache-min-tokens</code> threshold, IMC:</p>
          <ol>
            <li>Reserves the matching session (marks it pending)</li>
            <li>Trims the divergent suffix from the KV cache</li>
            <li>Decodes only the new tokens from the divergence point forward</li>
            <li>Updates the session's hash and cached token sequence</li>
          </ol>
          <p>Once the partial rebuild completes, subsequent requests in the same conversation use normal hash-based extending.</p>
          <p>Real-world testing showed 77-80% cache salvage rates. Instead of decoding ~8400 tokens from scratch, the system kept ~6800 cached and only decoded ~1600.</p>
          <p><strong>Debugging token prefix fallback:</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Log Message</th>
                <th>Meaning</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>no slot matched, trying token prefix match</code></td>
                <td>Hash match failed, entering token comparison</td>
              </tr>
              <tr>
                <td><code>slot[N] common-prefix X/Y tokens (Z% salvageable)</code></td>
                <td>Per-slot comparison result</td>
              </tr>
              <tr>
                <td><code>token prefix match found</code></td>
                <td>Usable prefix found, will trim and extend</td>
              </tr>
              <tr>
                <td><code>imc-trim-prefix</code></td>
                <td>KV cache trim in progress (shows cached_tokens, trim_pos)</td>
              </tr>
              <tr>
                <td><code>imc-partial-rebuilt</code></td>
                <td>Rebuild complete (shows total_cached, salvaged_prefix, salvaged_pct)</td>
              </tr>
              <tr>
                <td><code>no usable token prefix match</code></td>
                <td>All prefixes below <code>cache-min-tokens</code>, falling back to empty/LRU slot</td>
              </tr>
            </tbody>
          </table>
          <h4 id="model-type-interactions">Model Type Interactions</h4>
          <p>The IMC matching algorithm is the same for all model types (Dense, MoE, Hybrid). Only the batch engine's state management differs. See <a href="#49-model-types-and-state-management">Section 4.9</a> for how each model type manages state between requests.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Model Type</th>
                <th>State Management</th>
                <th>Configuration Notes</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Dense</td>
                <td>Snapshot/Restore</td>
                <td>No special requirements</td>
              </tr>
              <tr>
                <td>MoE</td>
                <td>Snapshot/Restore</td>
                <td>f16 cache, split-mode: row</td>
              </tr>
              <tr>
                <td>Hybrid</td>
                <td>Snapshot/Restore</td>
                <td>f16 cache required, no flash attn</td>
              </tr>
            </tbody>
          </table>
          <p><strong>MoE Configuration:</strong></p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/model_config.yaml
unsloth/Qwen3.6-35B-A3B-UD-Q4_K_M:
  incremental-cache: true
  split-mode: row     # Best for MoE architecture
  cache-type-k: f16   # Safer for MoE routing accuracy
  cache-type-v: f16`}</code></pre>
          <p><strong>Hybrid Configuration:</strong></p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/model_config.yaml
unsloth/LFM2-700M-Q8_0:
  incremental-cache: true
  cache-type-k: f16   # Required for hybrid models
  cache-type-v: f16   # Required for hybrid models`}</code></pre>
          <h3 id="53-single-user-caching">5.3 Single-User Caching</h3>
          <p>IMC is designed for single-user use. All <code>NSeqMax</code> sessions are available, with each session independently tracking its own conversation branch via hash matching. All sessions can run on any available slot. This design is optimized for agentic workflows where multiple sub-agents send independent conversations (different system prompts, different message histories).</p>
          <h3 id="54-when-to-use-imc">5.4 When to Use IMC</h3>
          <p>IMC caches the entire conversation history and uses hash matching with automatic token prefix fallback when changes are detected. It is best suited for:</p>
          <ul>
            <li><strong>Agentic workflows</strong> — hash matching handles the common case, and token prefix fallback automatically salvages 70-80% of cached tokens when changes are detected</li>
            <li><strong>AI coding agents</strong> — long-running conversations with growing context</li>
            <li><strong>Sub-agent architectures</strong> — each sub-agent gets its own session via hash matching, maintaining independent caches</li>
          </ul>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Feature</th>
                <th>Behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Caches</td>
                <td>All messages except last</td>
              </tr>
              <tr>
                <td>Extends</td>
                <td>Yes, incrementally</td>
              </tr>
              <tr>
                <td>Sessions</td>
                <td>All sessions available, single-user</td>
              </tr>
              <tr>
                <td>Slot routing</td>
                <td>Any available slot (all sessions)</td>
              </tr>
              <tr>
                <td>Sub-agents</td>
                <td>Each gets own session via hash matching</td>
              </tr>
              <tr>
                <td>Best for</td>
                <td>Agentic workflows</td>
              </tr>
              <tr>
                <td>VRAM</td>
                <td>Unified <code>n_ctx</code> pool, not multiplied by <code>nseq-max</code></td>
              </tr>
              <tr>
                <td>RAM</td>
                <td>One externalized KV snapshot per session between requests</td>
              </tr>
            </tbody>
          </table>
          <h3 id="55-cache-invalidation">5.5 Cache Invalidation</h3>
          <p>Cached state doesn't last forever. Kronk uses hash comparisons to detect when cached tokens no longer match the incoming request, and automatically rebuilds the cache when a mismatch is found. Understanding what triggers invalidation helps you avoid unexpected prefill costs.</p>
          <p><strong>IMC Invalidation:</strong></p>
          <ul>
            <li>Message prefix hash mismatch with same system prompt → system prompt KV preserved, conversation body trimmed and re-decoded (Step 4 of the session selection algorithm)</li>
            <li>Message prefix hash mismatch with no system prompt match → token prefix fallback attempted (see <a href="#token-prefix-fallback">Token Prefix Fallback</a>). If a common prefix ≥ <code>cache-min-tokens</code> is found, only the divergent suffix is trimmed and rebuilt. Otherwise, cache is rebuilt from scratch.</li>
            <li>System prompt changed → full cache rebuild from scratch</li>
            <li>Conversation shrinks (client dropped messages or reasoning blocks) → system prompt preserved if unchanged, conversation body re-decoded</li>
          </ul>
          <p><strong>Automatic Invalidation:</strong></p>
          <p>Caches are cleared when:</p>
          <ul>
            <li>Model is unloaded</li>
            <li>Server restarts</li>
          </ul>
          <h3 id="56-configuration-reference">5.6 Configuration Reference</h3>
          <p>IMC is enabled by default for all models. No configuration is needed to use it. To disable IMC for a specific model, set <code>incremental-cache: false</code> in your <code>model_config.yaml</code>:</p>
          <pre className="code-block"><code className="language-yaml">{`Qwen/Qwen3-8B-Q8_0:
  incremental-cache: false   # Disable IMC for this model`}</code></pre>
          <p>You can also tune the minimum cache threshold:</p>
          <pre className="code-block"><code className="language-yaml">{`Qwen/Qwen3-8B-Q8_0:
  cache-min-tokens: 100   # Don't cache if < 100 tokens (default: 100)`}</code></pre>
          <p><strong>cache-min-tokens</strong></p>
          <p>Minimum common prefix length required for token-level partial prefix matching. If no session's cached tokens share at least this many tokens with the incoming request, the fallback is skipped and the cache is rebuilt from scratch.</p>
          <p>Default: 100 tokens</p>
          <h3 id="57-performance-and-limitations">5.7 Performance and Limitations</h3>
          <p>IMC improves request latency by skipping redundant prefill work. It delivers large savings for multi-turn conversations but imposes restrictions on template behavior and session management.</p>
          <p><strong>IMC Prefill Savings:</strong></p>
          <p>For a 2000-token cached conversation prefix:</p>
          <ul>
            <li>Without cache: ~200ms prefill (varies by hardware)</li>
            <li>With IMC: ~5ms for new tokens only</li>
          </ul>
          <p>Cache extensions (adding new messages to an existing cached prefix) are especially fast because only the delta tokens are decoded. In production logs, sequential extensions typically take ~3ms each.</p>
          <p><strong>IMC Memory Overhead:</strong></p>
          <p>IMC adds no extra VRAM beyond what the context window already requires. With <code>nseq-max &gt; 1</code>, Kronk enables a unified KV cache where all sequences share the full <code>n_ctx</code> pool. The total KV cache size is determined by <code>context-window</code>, not multiplied by the number of sessions:</p>
          <pre className="code-block"><code>{`131K context, nseq-max=3, IMC (unified KV cache):
  Total KV cache: ~3.2 GB (8B model, F16)
  Any single slot can use up to the full 131K tokens
  Total across all slots cannot exceed 131K tokens`}</code></pre>
          <p>Sessions do not pin their prefix KV in VRAM between requests — the cached prefix is snapshotted to RAM and the VRAM sequence is cleared. This means sessions consume <strong>RAM</strong> (one KV snapshot per session) but no VRAM KV cells between requests. The RAM cost varies by conversation length and model size.</p>
          <p>KV pressure eviction only considers sessions whose cached KV is still resident in VRAM (sessions without an externalized <code>kvState</code>). Sessions with externalized state are excluded from VRAM pressure calculations.</p>
          <p><strong>IMC Token Prefix Fallback Performance:</strong></p>
          <p>When IMC falls back to token-level prefix matching, there is a one-time cost to tokenize the incoming messages for comparison. This is typically fast (&lt; 5ms for most conversations). The savings from salvaging 70-80% of the cached tokens far outweigh this cost compared to a full rebuild.</p>
          <p><strong>IMC with Vision/Audio Models:</strong></p>
          <p>IMC fully supports vision and audio models (models configured with a projection file). Text-only requests are cached normally. When a message containing media (image, video, or audio) appears in the conversation history, IMC caches the entire conversation — including the media embeddings — in the KV cache. The image or audio is encoded through the projection model once. After the request, the entire cached prefix (text + media KV) is snapshotted to RAM and restored on the next request — media is never re-encoded unless the cache is rebuilt from scratch. Text-only follow-up messages extend the cache without re-encoding the media.</p>
          <p>For example, in a conversation like:</p>
          <pre className="code-block"><code>{`Request 1 (image request):
[system]       →  cached by IMC (text tokens)
[user + image] →  cached by IMC (text + image embeddings via mtmd pipeline)
[user]         →  prefill (generation target)

Request 2 (text follow-up about the image):
[system]       →  restored from RAM (no re-encode)
[user + image] →  restored from RAM (image KV preserved, no re-encode)
[assistant]    →  extended (new text tokens decoded into cache)
[user]         →  prefill (generation target)

Request 3 (unrelated text question):
[system]       →  restored from RAM
[user + image] →  restored from RAM (image KV preserved)
[assistant]    →  restored from RAM
[user]         →  extended (new text tokens decoded into cache)
[assistant]    →  extended
[user]         →  prefill (generation target)

Request 4 (back to asking about the image):
[system]       →  restored from RAM
[user + image] →  restored from RAM (image KV preserved, no re-encode)
[assistant]    →  restored from RAM
[user]         →  restored from RAM
[assistant]    →  restored from RAM
[user]         →  extended (new text tokens decoded into cache)
[assistant]    →  extended
[user]         →  prefill (generation target)`}</code></pre>
          <p>When an image appears mid-conversation (after text-only messages), IMC preserves the existing text cache and extends it with media instead of rebuilding from scratch:</p>
          <pre className="code-block"><code>{`Text-only conversation, then image appears mid-conversation:

Requests 1–3 (text-only):
[system]       →  cached by IMC (text tokens)
[user]         →  cached / extended normally
[assistant]    →  cached / extended normally
...            →  conversation grows, all text cached incrementally

Request 4 (image appears mid-conversation):
[system]       →  cached (text tokens skipped via imcMediaSkipTextTokens)
[earlier msgs] →  cached (text tokens skipped)
[asst + user]  →  media extend from text (new text decoded from skip point)
[user + image] →  media extend from text (image encoded through projection model)
[user]         →  prefill (generation target)

Request 5 (text follow-up about the image):
[all prior]    →  restored from RAM (image KV preserved, no re-encode)
[assistant]    →  extended (text tokens only, no image re-encode)
[user]         →  prefill (generation target)`}</code></pre>
          <p><strong>How media caching works internally:</strong></p>
          <ol>
            <li>When <code>buildIMCCacheFromScratch</code> detects media content, it defers the build to <code>startSlot</code> where the mtmd pipeline (projection model) is available. The cache result carries <code>imcMediaBuild: true</code>.</li>
            <li>When media first appears in a conversation that started text-only, <code>extendIMCTextCacheWithMedia</code> preserves the existing text prefix in the KV cache. It sets <code>imcMediaSkipTextTokens</code> to the number of already-cached text tokens, so <code>decodeMediaIntoCache</code> skips them and only decodes the new text plus media embeddings. This avoids re-decoding potentially tens of thousands of cached text tokens when an image is first introduced mid-conversation.</li>
            <li><code>decodeMediaIntoCache</code> processes the prompt as interleaved chunks — text chunks are tokenized and decoded normally, while image/audio chunks are encoded through the projection model and their embeddings are decoded into the KV cache. When <code>imcMediaSkipTextTokens</code> is set, the first text chunk is partially skipped (only tokens beyond the skip point are decoded). For models using M-RoPE (e.g., Qwen2.5-VL), 2D spatial positions are assigned to image tokens.</li>
            <li>The session tracks <code>mediaKVCounts</code> — the number of KV positions consumed by each media chunk. This is needed because media embeddings occupy a different number of KV positions than the text marker tokens they replace in the tokenized prompt.</li>
            <li>On text-only follow-ups, <code>extendIMCMediaSlotWithText</code> uses the <code>mediaKVCounts</code> to compute the correct offset between text token indices and KV positions, then decodes only the new text tokens at the right position — no image re-encoding occurs.</li>
            <li>If a new message being added contains media (a second image, for example), <code>rebuildIMCWithMedia</code> triggers a full rebuild through the mtmd pipeline.</li>
            <li>Token prefix matching is skipped when the incoming request contains media messages, since the tokenization path would mutate media content and corrupt downstream processing.</li>
          </ol>
          <p><strong>IMC Limitations:</strong></p>
          <ul>
            <li>Editing earlier messages requires a partial rebuild (system prompt KV is preserved when the system prompt hasn't changed; conversation body is re-decoded)</li>
            <li>Changing the system prompt triggers a full cache rebuild</li>
            <li>Designed for single-user use</li>
            <li>Max concurrent conversation branches = NSeqMax; when all sessions are occupied, the least-recently-used session is evicted</li>
            <li>Cache hits include a RAM→VRAM restore step (typically 10-30ms depending on conversation size)</li>
            <li>When a new media message appears in the conversation, the cache is rebuilt through the mtmd pipeline (projection model encodes image/audio into embeddings)</li>
          </ul>
          <hr />
          <h2 id="chapter-6-yarn-extended-context">Chapter 6: YaRN Extended Context</h2>
          <p>YaRN (Yet another RoPE extensioN) allows models to handle context windows beyond their native training length. This is essential for long documents, extended conversations, and complex agentic workflows.</p>
          <h3 id="61-understanding-context-extension">6.1 Understanding Context Extension</h3>
          <p>Language models are trained with a fixed context length (e.g., 8K, 32K tokens). RoPE (Rotary Position Embedding) encodes position information, but naive extension beyond training length causes quality degradation.</p>
          <p>YaRN applies frequency-dependent interpolation with attention scaling to maintain quality at extended lengths.</p>
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
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/model_config.yaml
Qwen/Qwen3-8B-Q8_0:
  context-window: 131072    # Extended context (131K)
  rope-scaling-type: yarn   # Enable YaRN`}</code></pre>
          <p>That's often all you need—Kronk auto-calculates the other YaRN parameters from the context extension ratio.</p>
          <p><strong>Full Configuration (Advanced):</strong></p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/model_config.yaml
Qwen/Qwen3-8B-Q8_0:
  context-window: 131072
  rope-scaling-type: yarn
  rope-freq-base: 1000000   # Model-specific (Qwen3 uses 1M)
  rope-freq-scale: null     # Auto-calculate
  yarn-ext-factor: null     # Auto-calculate
  yarn-attn-factor: 1.0     # Attention scaling
  yarn-beta-fast: 32.0      # Low correction dimension
  yarn-beta-slow: 1.0       # High correction dimension
  yarn-orig-ctx: 32768      # Original training context`}</code></pre>
          <h3 id="64-scaling-types">6.4 Scaling Types</h3>
          <p>Kronk supports three RoPE scaling methods:</p>
          <p><strong>None (Default)</strong></p>
          <pre className="code-block"><code className="language-yaml">{`rope-scaling-type: none`}</code></pre>
          <p>Uses native context length. No scaling applied.</p>
          <p><strong>Linear</strong></p>
          <pre className="code-block"><code className="language-yaml">{`rope-scaling-type: linear`}</code></pre>
          <p>Simple linear interpolation. Works but quality degrades faster than YaRN at high extension ratios.</p>
          <p><strong>YaRN (Recommended)</strong></p>
          <pre className="code-block"><code className="language-yaml">{`rope-scaling-type: yarn`}</code></pre>
          <p>Frequency-dependent interpolation with attention scaling. Maintains quality better at 2-4x extensions.</p>
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
                <td><code>rope-scaling-type</code></td>
                <td>none</td>
                <td>Scaling method: <code>none</code>, <code>linear</code>, <code>yarn</code></td>
              </tr>
              <tr>
                <td><code>rope-freq-base</code></td>
                <td>model default</td>
                <td>Base frequency (10000 for Llama, 1000000 for Qwen3)</td>
              </tr>
              <tr>
                <td><code>rope-freq-scale</code></td>
                <td>auto</td>
                <td>Frequency scaling factor</td>
              </tr>
              <tr>
                <td><code>yarn-ext-factor</code></td>
                <td>auto</td>
                <td>Extrapolation mix factor (0 = disable)</td>
              </tr>
              <tr>
                <td><code>yarn-attn-factor</code></td>
                <td>1.0</td>
                <td>Attention magnitude scaling</td>
              </tr>
              <tr>
                <td><code>yarn-beta-fast</code></td>
                <td>32.0</td>
                <td>Low correction dimension</td>
              </tr>
              <tr>
                <td><code>yarn-beta-slow</code></td>
                <td>1.0</td>
                <td>High correction dimension</td>
              </tr>
              <tr>
                <td><code>yarn-orig-ctx</code></td>
                <td>model metadata</td>
                <td>Original training context size</td>
              </tr>
            </tbody>
          </table>
          <h3 id="66-model-specific-examples">6.6 Model-Specific Examples</h3>
          <p><strong>Qwen3 (32K → 131K)</strong></p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/model_config.yaml
Qwen/Qwen3-8B-Q8_0:
  context-window: 131072
  rope-scaling-type: yarn`}</code></pre>
          <p>Qwen3 models are specifically designed to support 131K context with YaRN. The default parameters work well.</p>
          <p><strong>Llama 3 (8K → 32K)</strong></p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/model_config.yaml
unsloth/Ministral-3-14B-Instruct-2512-Q4_0:
  context-window: 32768
  rope-scaling-type: yarn
  rope-freq-base: 10000`}</code></pre>
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
          <pre className="code-block"><code className="language-yaml">{`cache-type-k: q8_0
cache-type-v: q8_0`}</code></pre>
          <ol>
            <li>Reduce batch parallelism:</li>
          </ol>
          <pre className="code-block"><code className="language-yaml">{`nseq-max: 1 # Fewer concurrent requests`}</code></pre>
          <ol>
            <li>Keep KV cache on CPU (slower but saves VRAM):</li>
          </ol>
          <pre className="code-block"><code className="language-yaml">{`offload-kqv: false`}</code></pre>
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
          <h3 id="69-example-long-document-processing">6.9 Example: Long Document Processing</h3>
          <p>Configuration for processing long documents:</p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/model_config.yaml
Qwen/Qwen3-8B-Q8_0:
  context-window: 65536      # 64K context
  rope-scaling-type: yarn
  nbatch: 4096               # Larger batch for long prompts
  nubatch: 1024
  cache-type-k: q8_0
  cache-type-v: q8_0
  nseq-max: 1                # Single request (memory intensive)`}</code></pre>
          <p>This configuration can process documents up to ~50K tokens while leaving room for generation.</p>
          <hr />
          <h2 id="chapter-7-model-server">Chapter 7: Model Server</h2>
          <p>The Kronk Model Server provides an OpenAI-compatible REST API for inference. This chapter covers server configuration, management, and the catalog system.</p>
          <p><strong>CLI Modes: Web vs Local</strong></p>
          <p>Most CLI commands communicate with a running server by default:</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list                  # Talks to server at localhost:11435
kronk model pull Qwen3-0.6B-Q8_0    # Downloads via server`}</code></pre>
          <p>Add <code>--local</code> to run commands directly without a server:</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list --local          # Direct file access
kronk model pull Qwen3-0.6B-Q8_0 --local
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
          <p>Every command-line flag has a corresponding environment variable. The naming convention is <code>KRONK_</code> followed by the flag name in uppercase with hyphens replaced by underscores:</p>
          <pre className="code-block"><code>{`--api-host        →  KRONK_WEB_API_HOST
--models-in-cache →  KRONK_CACHE_MODELS_IN_CACHE
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
          <p>The server starts on <code>http://localhost:11435</code> by default.</p>
          <p><strong>Background Mode</strong></p>
          <p>Run the server as a background process:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start -d`}</code></pre>
          <p><strong>Custom Host/Port</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --api-host=0.0.0.0:9000`}</code></pre>
          <h3 id="72-stopping-the-server">7.2 Stopping the Server</h3>
          <pre className="code-block"><code className="language-shell">{`kronk server stop`}</code></pre>
          <h3 id="73-server-configuration">7.3 Server Configuration</h3>
          <p>Configuration can be set via command-line flags or environment variables. Every flag has a corresponding environment variable using the <code>KRONK_</code> prefix with underscores replacing hyphens.</p>
          <p><strong>Web Settings</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Flag</th>
                <th>Environment Variable</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>--api-host</code></td>
                <td><code>KRONK_WEB_API_HOST</code></td>
                <td><code>localhost:11435</code></td>
                <td>API host address</td>
              </tr>
              <tr>
                <td><code>--debug-host</code></td>
                <td><code>KRONK_WEB_DEBUG_HOST</code></td>
                <td><code>localhost:8090</code></td>
                <td>Debug/pprof host address</td>
              </tr>
              <tr>
                <td><code>--read-timeout</code></td>
                <td><code>KRONK_WEB_READ_TIMEOUT</code></td>
                <td><code>30s</code></td>
                <td>HTTP read timeout</td>
              </tr>
              <tr>
                <td><code>--write-timeout</code></td>
                <td><code>KRONK_WEB_WRITE_TIMEOUT</code></td>
                <td><code>15m</code></td>
                <td>HTTP write timeout</td>
              </tr>
              <tr>
                <td><code>--idle-timeout</code></td>
                <td><code>KRONK_WEB_IDLE_TIMEOUT</code></td>
                <td><code>1m</code></td>
                <td>HTTP idle timeout</td>
              </tr>
              <tr>
                <td><code>--shutdown-timeout</code></td>
                <td><code>KRONK_WEB_SHUTDOWN_TIMEOUT</code></td>
                <td><code>1m</code></td>
                <td>Graceful shutdown timeout</td>
              </tr>
              <tr>
                <td><code>--cors-allowed-origins</code></td>
                <td><code>KRONK_WEB_CORS_ALLOWED_ORIGINS</code></td>
                <td><code>*</code></td>
                <td>Comma-separated CORS origins</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Authentication Settings</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Flag</th>
                <th>Environment Variable</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>--auth-host</code></td>
                <td><code>KRONK_AUTH_HOST</code></td>
                <td><em>(empty)</em></td>
                <td>External auth service host. Leave empty to use local auth</td>
              </tr>
              <tr>
                <td><code>--auth-enabled</code></td>
                <td><code>KRONK_AUTH_LOCAL_ENABLED</code></td>
                <td><code>false</code></td>
                <td>Enable local JWT authentication</td>
              </tr>
              <tr>
                <td><code>--auth-issuer</code></td>
                <td><code>KRONK_AUTH_LOCAL_ISSUER</code></td>
                <td><code>kronk project</code></td>
                <td>Issuer name for local JWT tokens</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Tracing Settings (Tempo)</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Flag</th>
                <th>Environment Variable</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>--tempo-host</code></td>
                <td><code>KRONK_TEMPO_HOST</code></td>
                <td><code>localhost:4317</code></td>
                <td>OpenTelemetry collector host</td>
              </tr>
              <tr>
                <td><code>--tempo-service-name</code></td>
                <td><code>KRONK_TEMPO_SERVICE_NAME</code></td>
                <td><code>kronk</code></td>
                <td>Service name for traces</td>
              </tr>
              <tr>
                <td><code>--tempo-probability</code></td>
                <td><code>KRONK_TEMPO_PROBABILITY</code></td>
                <td><code>0.25</code></td>
                <td>Trace sampling probability (0.0-1.0)</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Cache & Model Configuration Settings</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Flag</th>
                <th>Environment Variable</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>--model-config-file</code></td>
                <td><code>KRONK_CACHE_MODEL_CONFIG_FILE</code></td>
                <td><code>&lt;base&gt;/model_config.yaml</code></td>
                <td>Path to per-model configuration overrides. Defaults to the file under your <code>--base-path</code>.</td>
              </tr>
              <tr>
                <td><code>--model-instances</code></td>
                <td><code>KRONK_CACHE_MODEL_INSTANCES</code></td>
                <td><code>1</code></td>
                <td>Maximum loaded instances of any single model id</td>
              </tr>
              <tr>
                <td><code>--models-in-cache</code></td>
                <td><code>KRONK_CACHE_MODELS_IN_CACHE</code></td>
                <td><code>2</code></td>
                <td>Maximum distinct models kept loaded in memory</td>
              </tr>
              <tr>
                <td><code>--cache-ttl</code></td>
                <td><code>KRONK_CACHE_TTL</code></td>
                <td><code>20m</code></td>
                <td>How long an unused model stays loaded</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Runtime Settings</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Flag</th>
                <th>Environment Variable</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>--base-path</code></td>
                <td><code>KRONK_BASE_PATH</code></td>
                <td><code>~/.kronk</code></td>
                <td>Base directory for all Kronk data</td>
              </tr>
              <tr>
                <td><code>--lib-path</code></td>
                <td><code>KRONK_LIB_PATH</code></td>
                <td><em>(empty)</em></td>
                <td>Override path Kronk loads llama.cpp libraries from. Empty resolves the default per-triple folder under the libraries root (<code>&lt;base&gt;/libraries/&lt;os&gt;/&lt;arch&gt;/&lt;processor&gt;/</code>). A directory containing a <code>version.json</code> is used as-is. A non-empty directory without a <code>version.json</code> is treated as a read-only user-managed build. See chapter 2.3 for full semantics.</td>
              </tr>
              <tr>
                <td><code>--lib-version</code></td>
                <td><code>KRONK_LIB_VERSION</code></td>
                <td><em>(empty)</em></td>
                <td>Specific llama library version</td>
              </tr>
              <tr>
                <td><code>--arch</code></td>
                <td><code>KRONK_ARCH</code></td>
                <td><em>(auto)</em></td>
                <td>Architecture override (<code>amd64</code>, <code>arm64</code>)</td>
              </tr>
              <tr>
                <td><code>--os</code></td>
                <td><code>KRONK_OS</code></td>
                <td><em>(auto)</em></td>
                <td>OS override (<code>linux</code>, <code>darwin</code>, <code>windows</code>)</td>
              </tr>
              <tr>
                <td><code>--processor</code></td>
                <td><code>KRONK_PROCESSOR</code></td>
                <td><em>(auto)</em></td>
                <td>Processor type (<code>cpu</code>, <code>metal</code>, <code>cuda</code>, <code>rocm</code>, <code>vulkan</code>)</td>
              </tr>
              <tr>
                <td><code>--hf-token</code></td>
                <td><code>KRONK_HF_TOKEN</code></td>
                <td><em>(empty)</em></td>
                <td>Hugging Face API token for gated models</td>
              </tr>
              <tr>
                <td><code>--allow-upgrade</code></td>
                <td><code>KRONK_ALLOW_UPGRADE</code></td>
                <td><code>true</code></td>
                <td>Allow automatic library upgrades to the latest llama.cpp release. The server defaults to <code>true</code> so a long-running server tracks upstream fixes. The standalone <code>kronk libs</code> CLI defaults to <code>false</code> (installs the well-known default version) and opts in via <code>--upgrade</code>.</td>
              </tr>
              <tr>
                <td><code>--llama-log</code></td>
                <td><code>KRONK_LLAMA_LOG</code></td>
                <td><code>1</code></td>
                <td>Llama log level (0=off, 1=on)</td>
              </tr>
              <tr>
                <td><code>--insecure-logging</code></td>
                <td><code>KRONK_INSECURE_LOGGING</code></td>
                <td><code>false</code></td>
                <td>Log sensitive data (messages, model config)</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Example</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --api-host=0.0.0.0:11435 \\
  --models-in-cache=5 \\
  --cache-ttl=30m \\
  --model-config-file=./model_config.yaml \\
  --hf-token=hf_xxxxx`}</code></pre>
          <h3 id="74-model-caching">7.4 Model Caching</h3>
          <p>The server maintains a pool of loaded models to avoid reload latency.</p>
          <p><strong>Configuration</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --models-in-cache=3 \\
  --cache-ttl=20m`}</code></pre>
          <ul>
            <li><code>models-in-cache</code> - Maximum distinct models kept loaded (default: 2)</li>
            <li><code>model-instances</code> - Maximum loaded instances of any single model id (default: 1)</li>
            <li><code>cache-ttl</code> - How long an unused model stays loaded (default: 20m)</li>
          </ul>
          <p>When a new model is requested and the cache is full, the least recently used model is unloaded.</p>
          <h3 id="75-model-config-files">7.5 Model Config Files</h3>
          <p>The server reads per-model overrides from <code>~/.kronk/model_config.yaml</code> by default. Kronk seeds this file from an embedded default on first server start; your edits are preserved across upgrades.</p>
          <p>The file is a flat map keyed by canonical model id (<code>provider/modelID</code>, optionally with a <code>/variant</code> suffix). Each entry's keys map 1:1 to <code>model.Config</code> and use kebab-case:</p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/model_config.yaml

unsloth/Qwen3-0.6B-Q8_0:
  context-window: 32768
  nseq-max: 4
  cache-type-k: q8_0
  cache-type-v: q8_0
  incremental-cache: true

unsloth/Ministral-3-14B-Instruct-2512-Q4_0:
  context-window: 8192
  ngpu-layers: 0
  split-mode: row

ggml-org/embeddinggemma-300m-qat-Q8_0:
  nseq-max: 2`}</code></pre>
          <p>To point at an alternative file (for testing without modifying your main one):</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --model-config-file=./my-test-config.yaml`}</code></pre>
          <p>Or via environment variable:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_CACHE_MODEL_CONFIG_FILE=/path/to/model_config.yaml
kronk server start`}</code></pre>
          <p><strong>Project Reference Configuration</strong></p>
          <p>The Kronk repository includes a comprehensive reference configuration with recommended settings for various models and use cases at <code>zarf/kms/model_config.yaml</code>. It includes:</p>
          <ul>
            <li>Optimized configurations for coding agents (Cline, OpenCode)</li>
            <li>YaRN extended context examples</li>
            <li>IMC configuration for message caching</li>
            <li>Vision and audio model settings</li>
            <li>Detailed comments explaining each configuration option</li>
            <li>Examples of YAML anchors for sharing common settings between variants</li>
          </ul>
          <h3 id="76-catalog-system">7.6 Catalog System</h3>
          <p>The catalog (<code>~/.kronk/catalog.yaml</code>) is your <strong>personal</strong> catalog of models. On first run Kronk seeds it from an embedded starter list so you have something to choose from immediately; the catalog grows as you pull models or resolve new IDs against HuggingFace.</p>
          <p>Each entry is a resolution cache — provider, family (HF repo), revision, file list, sizes, optional MMProj projection, and detected capabilities. Templates come from the GGUF metadata of the downloaded model itself and are not stored here.</p>
          <p><strong>List entries in the catalog</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list`}</code></pre>
          <p>Output (real columns):</p>
          <pre className="code-block"><code>{`VAL   MODEL ID                                       PROVIDER   FAMILY                              ARCH      MTMD   SIZE
✓     ggml-org/embeddinggemma-300m-qat-Q8_0          ggml-org   embeddinggemma-300m-qat-q8_0-GGUF   bert      -      329.0 MB
✓     unsloth/Qwen3-0.6B-Q8_0                        unsloth    Qwen3-0.6B-GGUF                     qwen3     -      699.0 MB
✗     unsloth/Ministral-3-14B-Instruct-2512-Q4_0     unsloth    Ministral-3-14B-Instruct-2512-GGUF  llama     -      8.0 GB`}</code></pre>
          <p><code>VAL</code> indicates whether the model files have been downloaded and validated locally; <code>MTMD</code> indicates a multimodal projection (mmproj) is present.</p>
          <p><strong>Show catalog entry details</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog show unsloth/Qwen3-0.6B-Q8_0`}</code></pre>
          <p><strong>Pull (download) a model</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk model pull unsloth/Qwen3-0.6B-Q8_0`}</code></pre>
          <p>After the pull completes, the catalog entry is enriched with the resolved provider, family, revision, and file sizes so subsequent lookups don't need to hit HuggingFace.</p>
          <p><strong>Remove a catalog entry</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog remove unsloth/Qwen3-0.6B-Q8_0   # also removes downloaded files`}</code></pre>
          <p>The same operations are available in the BUI's Catalog and Model views.</p>
          <h3 id="77-runtime-settings">7.7 Runtime Settings</h3>
          <p><strong>Processor Selection</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --processor=cuda    # NVIDIA GPU
kronk server start --processor=metal   # Apple Silicon
kronk server start --processor=vulkan  # Cross-platform GPU
kronk server start --processor=rocm    # AMD GPU (ROCm/HIP)
kronk server start --processor=cpu     # CPU only`}</code></pre>
          <p><strong>Library Path and Version Pinning</strong></p>
          <p>You can point to a custom library directory and pin a specific llama.cpp version for stability:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --lib-path=/custom/path/to/libraries \\
  --lib-version=b8864`}</code></pre>
          <p>Or via environment variable:</p>
          <pre className="code-block"><code className="language-shell">{`KRONK_LIB_VERSION=b8864 kronk server start`}</code></pre>
          <p>Breaking changes in llama.cpp can cause incompatibilities with yzma and Kronk. Use <code>--lib-version</code> (or <code>KRONK_LIB_VERSION</code>) to lock the server to a known-good version:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>llama.cpp</th>
                <th>yzma</th>
                <th>kronk</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>b8864</td>
                <td>v1.12.0</td>
                <td>1.23.1</td>
              </tr>
              <tr>
                <td>b8865+</td>
                <td>v1.13.0</td>
                <td>1.23.2</td>
              </tr>
            </tbody>
          </table>
          <p>If you set <code>--allow-upgrade=false</code>, automatic library upgrades are disabled and the server will only use the version you have installed.</p>
          <p><strong>Hugging Face Token</strong></p>
          <p>For gated models requiring authentication:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --hf-token=hf_xxxxx`}</code></pre>
          <p>Or via environment variable:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_HF_TOKEN=hf_xxxxx
kronk server start`}</code></pre>
          <h3 id="78-logging">7.8 Logging</h3>
          <p><strong>llama.cpp Logging</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --llama-log=1    # Enable llama.cpp logs
kronk server start --llama-log=0    # Disable (default)`}</code></pre>
          <p><strong>Insecure Logging</strong></p>
          <p>Enable logging of message content (for debugging only):</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --insecure-logging=true`}</code></pre>
          <p><strong>Warning:</strong> This logs sensitive data. Never use in production.</p>
          <p><strong>View Server Logs</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server logs`}</code></pre>
          <h3 id="79-data-paths">7.9 Data Paths</h3>
          <p>Default data locations:</p>
          <pre className="code-block"><code>{`~/.kronk/
├── catalog.yaml                        # Personal catalog (resolution cache + provider list)
├── model_config.yaml                   # Per-model configuration overrides
├── libraries/                          # llama.cpp libraries (one folder per triple)
│   └── <os>/<arch>/<processor>/        # e.g. darwin/arm64/metal/, linux/amd64/cuda/
│       ├── libllama.so / .dylib / .dll
│       └── version.json
└── models/                             # Downloaded model files
    ├── .index.yaml                     # Local file index (validated state per model)
    └── <provider>/<family>/<file>.gguf`}</code></pre>
          <p>Each <code>(arch, os, processor)</code> library install lives in its own folder. The runtime loads the folder for the detected triple by default; set <code>KRONK_LIB_PATH</code> to a different triple folder (and restart) to switch active install. See chapter 2.3 for <code>KRONK_LIB_PATH</code> semantics and the install-management commands.</p>
          <p><strong>Custom Base Path</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --base-path=/data/kronk`}</code></pre>
          <p><code>--base-path</code> shifts every file above to live under the new root.</p>
          <h3 id="710-complete-example">7.10 Complete Example</h3>
          <p>Production-ready server configuration:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --api-host=0.0.0.0:11435 \\
  --models-in-cache=2 \\
  --cache-ttl=20m \\
  --model-config-file=/etc/kronk/model_config.yaml \\
  --processor=cuda \\
  --auth-enabled=true \\
  -d`}</code></pre>
          <p>With model config:</p>
          <pre className="code-block"><code className="language-yaml">{`# /etc/kronk/model_config.yaml

unsloth/Qwen3-0.6B-Q8_0:
  context-window: 32768
  nseq-max: 4
  cache-type-k: q8_0
  cache-type-v: q8_0
  incremental-cache: true`}</code></pre>
          <hr />
          <h2 id="chapter-8-api-endpoints">Chapter 8: API Endpoints</h2>
          <p>Kronk provides an OpenAI-compatible REST API. This chapter documents the available endpoints and their usage.</p>
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
                <td><code>/v1/tokenize</code></td>
                <td>POST</td>
                <td>Tokenize text input</td>
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
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen/Qwen3-8B-Q8_0",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "What is the capital of France?"}
    ]
  }'`}</code></pre>
          <p><strong>Request Parameters:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen/Qwen3-8B-Q8_0",
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
  "model": "Qwen/Qwen3-8B-Q8_0",
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
  "model": "Qwen/Qwen3-8B-Q8_0",
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
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/responses \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen/Qwen3-8B-Q8_0",
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
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/embeddings \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "ggml-org/embeddinggemma-300m-qat-Q8_0",
    "input": "The quick brown fox jumps over the lazy dog."
  }'`}</code></pre>
          <p><strong>Multiple Inputs:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "ggml-org/embeddinggemma-300m-qat-Q8_0",
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
  "model": "ggml-org/embeddinggemma-300m-qat-Q8_0",
  "usage": {
    "prompt_tokens": 10,
    "total_tokens": 10
  }
}`}</code></pre>
          <h3 id="85-reranking">8.5 Reranking</h3>
          <p>Score and reorder documents by relevance to a query.</p>
          <p><strong>Endpoint:</strong> <code>POST /v1/rerank</code></p>
          <p><strong>Request:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/rerank \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpustack/bge-reranker-v2-m3-Q8_0",
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
  "model": "gpustack/bge-reranker-v2-m3-Q8_0",
  "usage": {
    "prompt_tokens": 45,
    "total_tokens": 45
  }
}`}</code></pre>
          <h3 id="86-tokenize">8.6 Tokenize</h3>
          <p>Get the token count for a text input. Works with any model type.</p>
          <p><strong>Endpoint:</strong> <code>POST /v1/tokenize</code></p>
          <p><strong>Parameters:</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Field</th>
                <th>Type</th>
                <th>Required</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>model</code></td>
                <td><code>string</code></td>
                <td>Yes</td>
                <td>Model ID (e.g., <code>Qwen/Qwen3-8B-Q8_0</code>). Works with any model type.</td>
              </tr>
              <tr>
                <td><code>input</code></td>
                <td><code>string</code></td>
                <td>Yes</td>
                <td>The text to tokenize.</td>
              </tr>
              <tr>
                <td><code>apply_template</code></td>
                <td><code>boolean</code></td>
                <td>No</td>
                <td>If true, wraps the input as a user message and applies the model's chat template before tokenizing. The count includes template overhead. Defaults to false.</td>
              </tr>
              <tr>
                <td><code>add_generation_prompt</code></td>
                <td><code>boolean</code></td>
                <td>No</td>
                <td>When <code>apply_template</code> is true, controls whether the assistant role prefix is appended to the prompt. Defaults to true.</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Request (raw text):</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/tokenize \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen/Qwen3-8B-Q8_0",
    "input": "The quick brown fox jumps over the lazy dog"
  }'`}</code></pre>
          <p><strong>Request (with template):</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/tokenize \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen/Qwen3-8B-Q8_0",
    "input": "The quick brown fox jumps over the lazy dog",
    "apply_template": true
  }'`}</code></pre>
          <p><strong>Response:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "object": "tokenize",
  "created": 1738857600,
  "model": "Qwen/Qwen3-8B-Q8_0",
  "tokens": 11
}`}</code></pre>
          <p>When <code>apply_template</code> is true, the token count will be higher than raw text because it includes template overhead (role markers, separators, and the generation prompt).</p>
          <h3 id="87-tool-calling-function-calling">8.7 Tool Calling (Function Calling)</h3>
          <p>Kronk supports OpenAI-compatible tool calling, allowing models to request function executions that you handle in your application.</p>
          <p><strong>Request with Tools:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen/Qwen3-8B-Q8_0",
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
  "model": "Qwen/Qwen3-8B-Q8_0",
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
          <h3 id="88-models-list">8.8 Models List</h3>
          <p>Get available models.</p>
          <p><strong>Endpoint:</strong> <code>GET /v1/models</code></p>
          <p><strong>Request:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/models`}</code></pre>
          <p><strong>Response:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "object": "list",
  "data": [
    {
      "id": "Qwen/Qwen3-8B-Q8_0",
      "object": "model",
      "owned_by": "kronk"
    },
    {
      "id": "ggml-org/embeddinggemma-300m-qat-Q8_0",
      "object": "model",
      "owned_by": "kronk"
    }
  ]
}`}</code></pre>
          <h3 id="89-authentication">8.9 Authentication</h3>
          <p>When authentication is enabled, include the token in requests:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer your-token-here" \\
  -d '{...}'`}</code></pre>
          <p>See <a href="#chapter-11-security--authentication">Chapter 11: Security & Authentication</a> for details on token management.</p>
          <h3 id="810-error-responses">8.10 Error Responses</h3>
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
          <h2 id="chapter-9-request-parameters">Chapter 9: Request Parameters</h2>
          <p>This chapter documents the request parameters available for controlling model output through both the SDK and REST API.</p>
          <h3 id="91-sampling-parameters">9.1 Sampling Parameters</h3>
          <p>These parameters control the randomness and diversity of generated text.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Parameter</th>
                <th>JSON Key</th>
                <th>Type</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Temperature</td>
                <td><code>temperature</code></td>
                <td>float32</td>
                <td>0.8</td>
                <td>Controls randomness of output. Higher values produce more varied text, lower values more deterministic.</td>
              </tr>
              <tr>
                <td>Top-K</td>
                <td><code>top_k</code></td>
                <td>int32</td>
                <td>40</td>
                <td>Limits token pool to K most probable tokens before sampling.</td>
              </tr>
              <tr>
                <td>Top-P</td>
                <td><code>top_p</code></td>
                <td>float32</td>
                <td>0.9</td>
                <td>Nucleus sampling threshold. Only tokens with cumulative probability ≤ top_p are considered.</td>
              </tr>
              <tr>
                <td>Min-P</td>
                <td><code>min_p</code></td>
                <td>float32</td>
                <td>0.0</td>
                <td>Dynamic sampling threshold. Tokens with probability &lt; min_p × max_probability are excluded.</td>
              </tr>
            </tbody>
          </table>
          <h3 id="92-repetition-control">9.2 Repetition Control</h3>
          <p>These parameters help prevent repetitive output.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Parameter</th>
                <th>JSON Key</th>
                <th>Type</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Repeat Penalty</td>
                <td><code>repeat_penalty</code></td>
                <td>float32</td>
                <td>1.0</td>
                <td>Penalty multiplier for repeated tokens. Values &gt; 1.0 discourage repetition.</td>
              </tr>
              <tr>
                <td>Repeat Last N</td>
                <td><code>repeat_last_n</code></td>
                <td>int32</td>
                <td>64</td>
                <td>Window size for repetition check. Only the last N tokens are considered.</td>
              </tr>
            </tbody>
          </table>
          <p><strong>DRY Parameters (Don't Repeat Yourself):</strong></p>
          <p>DRY penalizes n-gram repetitions to prevent the model from repeating phrases.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Parameter</th>
                <th>JSON Key</th>
                <th>Type</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>DRY Multiplier</td>
                <td><code>dry_multiplier</code></td>
                <td>float32</td>
                <td>1.05</td>
                <td>N-gram repetition penalty strength. Higher values penalize repetition more.</td>
              </tr>
              <tr>
                <td>DRY Base</td>
                <td><code>dry_base</code></td>
                <td>float32</td>
                <td>1.75</td>
                <td>Exponential penalty base for longer n-grams.</td>
              </tr>
              <tr>
                <td>DRY Allowed Length</td>
                <td><code>dry_allowed_length</code></td>
                <td>int32</td>
                <td>2</td>
                <td>Minimum n-gram length to consider for penalties.</td>
              </tr>
              <tr>
                <td>DRY Penalty Last N</td>
                <td><code>dry_penalty_last_n</code></td>
                <td>int32</td>
                <td>0</td>
                <td>Number of recent tokens to consider for DRY. 0 means all tokens.</td>
              </tr>
            </tbody>
          </table>
          <h3 id="93-advanced-sampling">9.3 Advanced Sampling</h3>
          <p><strong>XTC (eXtreme Token Culling):</strong></p>
          <p>XTC probabilistically removes high-probability tokens to increase diversity.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Parameter</th>
                <th>JSON Key</th>
                <th>Type</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>XTC Probability</td>
                <td><code>xtc_probability</code></td>
                <td>float32</td>
                <td>0.0</td>
                <td>Probability of activating XTC on each token. 0 disables XTC.</td>
              </tr>
              <tr>
                <td>XTC Threshold</td>
                <td><code>xtc_threshold</code></td>
                <td>float32</td>
                <td>0.1</td>
                <td>Probability threshold for token culling.</td>
              </tr>
              <tr>
                <td>XTC Min Keep</td>
                <td><code>xtc_min_keep</code></td>
                <td>uint32</td>
                <td>1</td>
                <td>Minimum number of tokens to keep after culling.</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Adaptive-P:</strong></p>
          <p>Adaptive-P dynamically adjusts the sampling threshold based on output probability.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Parameter</th>
                <th>JSON Key</th>
                <th>Type</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Adaptive-P Target</td>
                <td><code>adaptive_p_target</code></td>
                <td>float32</td>
                <td>0.0</td>
                <td>Target probability threshold. 0 disables adaptive sampling.</td>
              </tr>
              <tr>
                <td>Adaptive-P Decay</td>
                <td><code>adaptive_p_decay</code></td>
                <td>float32</td>
                <td>0.0</td>
                <td>Speed of threshold adjustment toward target.</td>
              </tr>
            </tbody>
          </table>
          <h3 id="94-generation-control">9.4 Generation Control</h3>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Parameter</th>
                <th>JSON Key</th>
                <th>Type</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Max Tokens</td>
                <td><code>max_tokens</code></td>
                <td>int</td>
                <td>4096</td>
                <td>Maximum tokens to generate.</td>
              </tr>
              <tr>
                <td>Enable Thinking</td>
                <td><code>enable_thinking</code></td>
                <td>string</td>
                <td>"true"</td>
                <td>Enable model thinking/reasoning mode. Set to "false" for direct responses.</td>
              </tr>
              <tr>
                <td>Reasoning Effort</td>
                <td><code>reasoning_effort</code></td>
                <td>string</td>
                <td>"medium"</td>
                <td>GPT reasoning level: none, minimal, low, medium, high.</td>
              </tr>
              <tr>
                <td>Stream</td>
                <td><code>stream</code></td>
                <td>bool</td>
                <td>false</td>
                <td>Stream response chunks via SSE.</td>
              </tr>
              <tr>
                <td>Include Usage</td>
                <td><code>include_usage</code></td>
                <td>bool</td>
                <td>true</td>
                <td>Include token usage statistics in streaming responses.</td>
              </tr>
            </tbody>
          </table>
          <h3 id="95-grammar-constrained-output">9.5 Grammar Constrained Output</h3>
          <p>Grammars force the model to only produce tokens that match a specified pattern, guaranteeing structured output.</p>
          <p><strong>Built-in Presets:</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Preset</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>GrammarJSON</code></td>
                <td>Valid JSON objects or arrays</td>
              </tr>
              <tr>
                <td><code>GrammarJSONObject</code></td>
                <td>JSON objects only</td>
              </tr>
              <tr>
                <td><code>GrammarJSONArray</code></td>
                <td>JSON arrays only</td>
              </tr>
              <tr>
                <td><code>GrammarBoolean</code></td>
                <td>"true" or "false"</td>
              </tr>
              <tr>
                <td><code>GrammarYesNo</code></td>
                <td>"yes" or "no"</td>
              </tr>
              <tr>
                <td><code>GrammarInteger</code></td>
                <td>Integer values</td>
              </tr>
              <tr>
                <td><code>GrammarNumber</code></td>
                <td>Numeric values (int or float)</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Using Grammar Presets (SDK):</strong></p>
          <pre className="code-block"><code className="language-go">{`d := model.D{
    "messages": model.DocumentArray(
        model.TextMessage(model.RoleUser, "List 3 languages in JSON"),
    ),
    "grammar": model.GrammarJSONObject,
}`}</code></pre>
          <p><strong>Using Grammar via API:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen/Qwen3-8B-Q8_0",
    "messages": [{"role": "user", "content": "List 3 languages in JSON"}],
    "grammar": "root ::= object\\nvalue ::= object | array | string | number | \\"true\\" | \\"false\\" | \\"null\\"\\nobject ::= \\"{\\" ws ( string \\":\\" ws value (\\",\\" ws string \\":\\" ws value)* )? ws \\"}\\"\\narray ::= \\"[\\" ws ( value (\\",\\" ws value)* )? ws \\"]\\"\\nstring ::= \\"\\\\\\"\\" ([^\\"\\\\\\\\] | \\"\\\\\\\\\\" [\\"\\\\\\\\bfnrt/] | \\"\\\\\\\\u\\" [0-9a-fA-F]{4})* \\"\\\\\\"\\"\\nnumber ::= \\"-\\"? (\\"0\\" | [1-9][0-9]*) (\\".\\" [0-9]+)? ([eE] [+-]? [0-9]+)?\\nws ::= [ \\\\t\\\\n\\\\r]*"
  }'`}</code></pre>
          <p><strong>JSON Schema Auto-Conversion:</strong></p>
          <pre className="code-block"><code className="language-go">{`schema := model.D{
    "type": "object",
    "properties": model.D{
        "name": model.D{"type": "string"},
        "year": model.D{"type": "integer"},
    },
    "required": []string{"name", "year"},
}

d := model.D{
    "messages": model.DocumentArray(...),
    "json_schema": schema,
    "enable_thinking": false,
}`}</code></pre>
          <p>Via API with <code>json_schema</code> field:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen/Qwen3-8B-Q8_0",
  "messages": [...],
  "json_schema": {
    "type": "object",
    "properties": {
      "name": {"type": "string"},
      "year": {"type": "integer"}
    },
    "required": ["name", "year"]
  },
  "enable_thinking": false
}`}</code></pre>
          <p><strong>Custom GBNF Grammars:</strong></p>
          <pre className="code-block"><code className="language-go">{`sentimentGrammar := \`root ::= sentiment
sentiment ::= "positive" | "negative" | "neutral"\`

d := model.D{
    "messages": model.DocumentArray(...),
    "grammar": sentimentGrammar,
    "enable_thinking": false,
}`}</code></pre>
          <p><strong>Important:</strong> When using grammar constraints, set <code>enable_thinking: false</code> because the grammar applies from the first output token.</p>
          <h3 id="96-logprobs-token-probabilities">9.6 Logprobs (Token Probabilities)</h3>
          <p>Request log probabilities for generated tokens to understand model confidence or implement custom sampling strategies.</p>
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
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen/Qwen3-8B-Q8_0",
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
          <h3 id="97-parameter-reference">9.7 Parameter Reference</h3>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Parameter</th>
                <th>JSON Key</th>
                <th>Type</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Temperature</td>
                <td><code>temperature</code></td>
                <td>float32</td>
                <td>0.8</td>
                <td>Controls randomness of output</td>
              </tr>
              <tr>
                <td>Top-K</td>
                <td><code>top_k</code></td>
                <td>int32</td>
                <td>40</td>
                <td>Limits token pool to K most probable</td>
              </tr>
              <tr>
                <td>Top-P</td>
                <td><code>top_p</code></td>
                <td>float32</td>
                <td>0.9</td>
                <td>Nucleus sampling threshold</td>
              </tr>
              <tr>
                <td>Min-P</td>
                <td><code>min_p</code></td>
                <td>float32</td>
                <td>0.0</td>
                <td>Dynamic sampling threshold</td>
              </tr>
              <tr>
                <td>Max Tokens</td>
                <td><code>max_tokens</code></td>
                <td>int</td>
                <td>4096</td>
                <td>Maximum tokens to generate</td>
              </tr>
              <tr>
                <td>Repeat Penalty</td>
                <td><code>repeat_penalty</code></td>
                <td>float32</td>
                <td>1.0</td>
                <td>Penalty for repeated tokens</td>
              </tr>
              <tr>
                <td>Repeat Last N</td>
                <td><code>repeat_last_n</code></td>
                <td>int32</td>
                <td>64</td>
                <td>Window for repetition check</td>
              </tr>
              <tr>
                <td>DRY Multiplier</td>
                <td><code>dry_multiplier</code></td>
                <td>float32</td>
                <td>1.05</td>
                <td>N-gram repetition penalty</td>
              </tr>
              <tr>
                <td>DRY Base</td>
                <td><code>dry_base</code></td>
                <td>float32</td>
                <td>1.75</td>
                <td>Exponential penalty base</td>
              </tr>
              <tr>
                <td>DRY Allowed Length</td>
                <td><code>dry_allowed_length</code></td>
                <td>int32</td>
                <td>2</td>
                <td>Min n-gram length for DRY</td>
              </tr>
              <tr>
                <td>DRY Penalty Last N</td>
                <td><code>dry_penalty_last_n</code></td>
                <td>int32</td>
                <td>0</td>
                <td>Recent tokens for DRY (0=all)</td>
              </tr>
              <tr>
                <td>XTC Probability</td>
                <td><code>xtc_probability</code></td>
                <td>float32</td>
                <td>0.0</td>
                <td>XTC activation probability</td>
              </tr>
              <tr>
                <td>XTC Threshold</td>
                <td><code>xtc_threshold</code></td>
                <td>float32</td>
                <td>0.1</td>
                <td>XTC probability threshold</td>
              </tr>
              <tr>
                <td>XTC Min Keep</td>
                <td><code>xtc_min_keep</code></td>
                <td>uint32</td>
                <td>1</td>
                <td>Min tokens after XTC</td>
              </tr>
              <tr>
                <td>Adaptive-P Target</td>
                <td><code>adaptive_p_target</code></td>
                <td>float32</td>
                <td>0.0</td>
                <td>Adaptive sampling target</td>
              </tr>
              <tr>
                <td>Adaptive-P Decay</td>
                <td><code>adaptive_p_decay</code></td>
                <td>float32</td>
                <td>0.0</td>
                <td>Adaptive adjustment speed</td>
              </tr>
              <tr>
                <td>Enable Thinking</td>
                <td><code>enable_thinking</code></td>
                <td>string</td>
                <td>"true"</td>
                <td>Enable model thinking</td>
              </tr>
              <tr>
                <td>Reasoning Effort</td>
                <td><code>reasoning_effort</code></td>
                <td>string</td>
                <td>"medium"</td>
                <td>GPT reasoning level</td>
              </tr>
              <tr>
                <td>Grammar</td>
                <td><code>grammar</code></td>
                <td>string</td>
                <td>""</td>
                <td>GBNF grammar constraint</td>
              </tr>
              <tr>
                <td>Logprobs</td>
                <td><code>logprobs</code></td>
                <td>bool</td>
                <td>false</td>
                <td>Return token probabilities</td>
              </tr>
              <tr>
                <td>Top Logprobs</td>
                <td><code>top_logprobs</code></td>
                <td>int</td>
                <td>0</td>
                <td>Number of top alternatives</td>
              </tr>
              <tr>
                <td>Stream</td>
                <td><code>stream</code></td>
                <td>bool</td>
                <td>false</td>
                <td>Stream response</td>
              </tr>
              <tr>
                <td>Include Usage</td>
                <td><code>include_usage</code></td>
                <td>bool</td>
                <td>true</td>
                <td>Include usage in streaming</td>
              </tr>
              <tr>
                <td>Return Prompt</td>
                <td><code>return_prompt</code></td>
                <td>bool</td>
                <td>false</td>
                <td>Include prompt in response</td>
              </tr>
            </tbody>
          </table>
          <hr />
          <h2 id="chapter-10-multi-modal-models">Chapter 10: Multi-Modal Models</h2>
          <p>Kronk supports vision and audio models that can process images, video frames, and audio alongside text. This chapter covers how to use these models.</p>
          <h3 id="101-overview">10.1 Overview</h3>
          <p>Multi-modal models combine a language model with a media projector that converts images or audio into tokens the model can understand.</p>
          <p><strong>Supported Media Types:</strong></p>
          <ul>
            <li><strong>Vision</strong>: JPEG, PNG, GIF images</li>
            <li><strong>Audio</strong>: WAV audio files</li>
          </ul>
          <p><strong>Available Models (from catalog):</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list`}</code></pre>
          <p>The <code>MTMD</code> column marks entries that ship with a multi-modal projector. To filter by capability (images, audio, etc.) use the BUI catalog view, which exposes capability filters in the sidebar.</p>
          <p>Example models from the seed catalog:</p>
          <ul>
            <li><code>unsloth/LFM2.5-VL-1.6B-Q8_0</code> - Vision model</li>
            <li><code>unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M</code> - Vision model</li>
            <li><code>mradermacher/Qwen2-Audio-7B.Q8_0</code> - Audio model</li>
            <li><code>ggml-org/Qwen3-Omni-30B-A3B-Instruct-Q8_0</code> - Vision + audio + video</li>
          </ul>
          <h3 id="102-vision-models">10.2 Vision Models</h3>
          <p>Vision models analyze images and answer questions about their content.</p>
          <p><strong>Download a Vision Model:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk model pull unsloth/LFM2.5-VL-1.6B-Q8_0`}</code></pre>
          <p><strong>API Request with Image (OpenAI Format):</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "unsloth/LFM2.5-VL-1.6B-Q8_0",
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
          <h3 id="103-audio-models">10.3 Audio Models</h3>
          <p>Audio models transcribe and understand spoken content.</p>
          <p><strong>Download an Audio Model:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk model pull mradermacher/Qwen2-Audio-7B.Q8_0`}</code></pre>
          <p><strong>API Request with Audio:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "mradermacher/Qwen2-Audio-7B.Q8_0",
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
          <h3 id="104-plain-base64-format">10.4 Plain Base64 Format</h3>
          <p>For simpler integrations, Kronk also accepts plain base64 as the message content (without the structured OpenAI format):</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "unsloth/LFM2.5-VL-1.6B-Q8_0",
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
          <h3 id="105-configuration-for-multi-modal-models">10.5 Configuration for Multi-Modal Models</h3>
          <p>Vision and audio models have specific configuration requirements:</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/LFM2.5-VL-1.6B-Q8_0:
  nubatch: 2048 # Higher for image token processing
  nseq-max: 2 # Process up to 2 requests concurrently
  context-window: 8192`}</code></pre>
          <p><strong>Key Considerations:</strong></p>
          <ul>
            <li><code>nubatch</code> should be high (≥2048) for efficient image/audio token processing</li>
            <li><code>nseq-max</code> controls batch parallelism (multiple slots in shared context)</li>
            <li>Vision/audio models use the same batch engine as text models</li>
          </ul>
          <h3 id="106-memory-requirements">10.6 Memory Requirements</h3>
          <p>Vision and audio models require additional memory for the projector:</p>
          <p><strong>Vision Model Example (unsloth/LFM2.5-VL-1.6B-Q8_0):</strong></p>
          <pre className="code-block"><code>{`Model weights:     ~1.2 GB
Projector:         ~0.8 GB
KV cache (8K):     ~0.4 GB
─────────────────────────
Total:             ~2.4 GB`}</code></pre>
          <p><strong>Audio Model Example (mradermacher/Qwen2-Audio-7B.Q8_0):</strong></p>
          <pre className="code-block"><code>{`Model weights:     ~8 GB
Projector:         ~0.7 GB
KV cache (8K):     ~0.6 GB
─────────────────────────
Total:             ~9.3 GB`}</code></pre>
          <h3 id="107-imc-and-multi-modal-caching">10.7 IMC and Multi-Modal Caching</h3>
          <p>IMC fully supports vision and audio models. Media embeddings (images, audio) are cached in the KV cache alongside text tokens. After each request, the entire cached prefix — including media embeddings — is snapshotted to RAM via <code>StateSeqGetData</code> and the VRAM sequence is cleared. On the next request, the cached state is restored from RAM into any available slot, just like text-only sessions. Media is never re-encoded through the projection model unless the conversation cache is rebuilt from scratch.</p>
          <p>For example, in a multi-turn vision conversation:</p>
          <ol>
            <li><strong>First request</strong> (image + question): The image is encoded through the projection model and decoded into the KV cache alongside text tokens. After generation, the entire cached prefix (text + media KV) is snapshotted to RAM.</li>
            <li><strong>Follow-up requests</strong> (text-only): The cached state is restored from RAM into any available slot. Only new text tokens are decoded — the image embeddings are preserved in the restored KV state without re-encoding.</li>
            <li><strong>New image in conversation</strong>: If a new message contains media, IMC triggers a full rebuild through the mtmd pipeline to re-encode all media.</li>
          </ol>
          <p>See <a href="chapter-05-message-caching.md">Chapter 5: Message Caching</a> for full details on IMC's caching algorithm.</p>
          <h3 id="108-limitations">10.8 Limitations</h3>
          <ul>
            <li>Processing time varies with image resolution and audio duration</li>
          </ul>
          <h3 id="109-example-image-analysis">10.9 Example: Image Analysis</h3>
          <p>Complete example analyzing an image:</p>
          <pre className="code-block"><code className="language-shell">{`# Encode image to base64
IMAGE_B64=$(base64 -i photo.jpg)

# Send request
curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "unsloth/LFM2.5-VL-1.6B-Q8_0",
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
          <h3 id="1010-example-audio-transcription">10.10 Example: Audio Transcription</h3>
          <p>Complete example transcribing audio:</p>
          <pre className="code-block"><code className="language-shell">{`# Encode audio to base64
AUDIO_B64=$(base64 -i recording.wav)

# Send request
curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "mradermacher/Qwen2-Audio-7B.Q8_0",
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
          <p><em>Next: &lt;a href="#chapter-11-security--authentication"&gt;Chapter 11: Security & Authentication&lt;/a&gt;</em></p>
          <h2 id="chapter-11-security-authentication">Chapter 11: Security &amp; Authentication</h2>
          <p>Kronk provides JWT-based authentication and authorization with per-endpoint rate limiting. When enabled, all API requests require a valid token.</p>
          <h3 id="111-enabling-authentication">11.1 Enabling Authentication</h3>
          <p><strong>Start Server with Auth Enabled:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --auth-enabled`}</code></pre>
          <p>Or via environment variable:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_AUTH_LOCAL_ENABLED=true
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
          <h3 id="112-using-the-admin-token">11.2 Using the Admin Token</h3>
          <p>The admin token is required for all security management operations.</p>
          <p><strong>Set the Token:</strong></p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)`}</code></pre>
          <p><strong>Admin Capabilities:</strong></p>
          <ul>
            <li>Create new tokens for users</li>
            <li>Add and remove signing keys</li>
            <li>Access all endpoints without rate limits</li>
          </ul>
          <h3 id="113-key-management">11.3 Key Management</h3>
          <p>Private keys sign JWT tokens. Multiple keys allow token rotation without invalidating all existing tokens.</p>
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
          <p><strong>Important:</strong> The master key cannot be deleted. Deleting a key invalidates all tokens signed with that key.</p>
          <p><strong>Local Mode:</strong></p>
          <p>All key commands support <code>--local</code> to operate without a running server:</p>
          <pre className="code-block"><code className="language-shell">{`kronk security key list --local
kronk security key create --local
kronk security key delete --keyid <keyid> --local`}</code></pre>
          <h3 id="114-creating-user-tokens">11.4 Creating User Tokens</h3>
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
          <h3 id="115-token-examples">11.5 Token Examples</h3>
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
          <h3 id="116-using-tokens-in-api-requests">11.6 Using Tokens in API Requests</h3>
          <p>Pass the token in the <code>Authorization</code> header with the <code>Bearer</code> prefix.</p>
          <p><strong>curl Example:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Authorization: Bearer eyJhbGciOiJS..." \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen/Qwen3-8B-Q8_0",
    "messages": [{"role": "user", "content": "Hello"}]
  }'`}</code></pre>
          <p><strong>Environment Variable Pattern:</strong></p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_TOKEN="eyJhbGciOiJS..."

curl http://localhost:11435/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{...}'`}</code></pre>
          <p><strong>Python Example:</strong></p>
          <pre className="code-block"><code className="language-python">{`import openai

client = openai.OpenAI(
    base_url="http://localhost:11435/v1",
    api_key="eyJhbGciOiJS..."  # Your Kronk token
)

response = client.chat.completions.create(
    model="Qwen/Qwen3-8B-Q8_0",
    messages=[{"role": "user", "content": "Hello"}]
)`}</code></pre>
          <h3 id="117-authorization-flow">11.7 Authorization Flow</h3>
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
          <h3 id="118-rate-limiting">11.8 Rate Limiting</h3>
          <p>Rate limits are enforced per token (identified by the token's subject claim).</p>
          <p><strong>How Limits Work:</strong></p>
          <ul>
            <li>Each token has a unique subject (UUID)</li>
            <li>Requests are counted per endpoint per subject</li>
            <li>Counters reset at window boundaries (day/month/year)</li>
          </ul>
          <p><strong>Limit Storage:</strong></p>
          <p>Rate limit counters are stored in a BadgerDB database at <code>~/.kronk/badger/</code>. Counters persist across server restarts.</p>
          <p><strong>Bypassing Rate Limits:</strong></p>
          <p>Admin tokens (like <code>master.jwt</code>) bypass all rate limiting.</p>
          <h3 id="119-configuration-reference">11.9 Configuration Reference</h3>
          <p><strong>Deployment Modes:</strong></p>
          <p>Auth can run in two modes:</p>
          <ol>
            <li><strong>Embedded (default)</strong> — The auth service runs in-process inside <code>kronk server</code> over an in-memory listener. This is what <code>kronk server start --auth-enabled</code> uses. Configured via the <code>--auth-enabled</code> / <code>--auth-issuer</code> flags and the <code>KRONK_AUTH_LOCAL_*</code> env vars.</li>
            <li><strong>Standalone</strong> — A separate <code>auth</code> binary listens on its own host; <code>kronk server</code> connects to it via <code>--auth-host</code> (<code>KRONK_AUTH_HOST</code>). The standalone service has its own <code>AUTH_AUTH_*</code> env-var prefix (e.g. <code>AUTH_AUTH_HOST</code>, <code>AUTH_AUTH_ISSUER</code>, <code>AUTH_AUTH_ENABLED</code>). Setting <code>KRONK_AUTH_HOST</code> on the kronk server skips the embedded auth startup entirely.</li>
          </ol>
          <p><strong>Server Flags (kronk server):</strong></p>
          <ul>
            <li><code>--auth-enabled</code> - Enable embedded local auth (env: <code>KRONK_AUTH_LOCAL_ENABLED</code>)</li>
            <li><code>--auth-issuer</code> - JWT issuer name for the embedded auth (env: <code>KRONK_AUTH_LOCAL_ISSUER</code>)</li>
            <li><code>--auth-host</code> - Host of an external standalone auth service (env: <code>KRONK_AUTH_HOST</code>)</li>
          </ul>
          <p><strong>Standalone Auth Service Env Vars:</strong></p>
          <ul>
            <li><code>AUTH_AUTH_HOST</code> - Listen address (default <code>localhost:6000</code>)</li>
            <li><code>AUTH_AUTH_ISSUER</code> - JWT issuer name (default <code>kronk project</code>)</li>
            <li><code>AUTH_AUTH_ENABLED</code> - Enable auth enforcement (default <code>false</code>)</li>
          </ul>
          <p><strong>Environment Variables (CLI / clients):</strong></p>
          <ul>
            <li><code>KRONK_TOKEN</code> - Token for CLI commands and API requests</li>
            <li><code>KRONK_WEB_API_HOST</code> - Server address for CLI web mode (default: <code>localhost:11435</code>)</li>
          </ul>
          <h3 id="1110-security-best-practices">11.10 Security Best Practices</h3>
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
          <p><em>Next: &lt;a href="#chapter-12-browser-ui-bui"&gt;Chapter 12: Browser UI (BUI)&lt;/a&gt;</em></p>
          <h2 id="chapter-12-browser-ui-bui">Chapter 12: Browser UI (BUI)</h2>
          <p>Kronk ships with a built-in Browser UI (BUI) served from the same port as the API. It is a thin client over the Web API and exposes the same operations the CLI provides — pulling libraries and models, browsing the catalog, managing tokens, and running interactive experiments against a loaded model. This chapter is a high-level guide to what the BUI offers; it intentionally does not enumerate every tab, filter, or button so that the documentation stays accurate as the UI evolves.</p>
          <h3 id="121-accessing-the-bui">12.1 Accessing the BUI</h3>
          <p>The BUI loads automatically when you open the server root in a browser:</p>
          <pre className="code-block"><code>{`http://localhost:11435`}</code></pre>
          <p>It is bundled inside the <code>kronk</code> binary and served from the same address configured by <code>KRONK_WEB_API_HOST</code> (default <code>0.0.0.0:11435</code>).</p>
          <h3 id="122-sidebar-layout">12.2 Sidebar Layout</h3>
          <p>Navigation is grouped into the following top-level sections in the sidebar:</p>
          <ul>
            <li><strong>Home</strong> — landing page with a project banner and feature overview</li>
            <li><strong>Models</strong> — local model file management</li>
            <li><strong>Catalog</strong> — personal catalog browsing</li>
            <li><strong>Libraries</strong> — llama.cpp library installs</li>
            <li><strong>Apps</strong> — interactive tools (Chat, Playground, VRAM Calculator)</li>
            <li><strong>Security</strong> — keys and tokens (relevant when auth is enabled)</li>
            <li><strong>Docs</strong> — bundled documentation (Manual, SDK, CLI, Web API)</li>
            <li><strong>Settings</strong> — BUI preferences and the admin token</li>
          </ul>
          <h3 id="123-what-the-bui-provides">12.3 What the BUI Provides</h3>
          <h4 id="models">Models</h4>
          <p>The Models area lists every model file under <code>~/.kronk/models/</code> along with the currently running models and a page for pulling new ones by HuggingFace URL or canonical model id.</p>
          <p>It mirrors the CLI surface: <code>kronk model list</code>, <code>kronk model ps</code>, <code>kronk model pull</code>, <code>kronk model show</code>, and <code>kronk model remove</code>. Per- model details (configuration, sampling, template, GGUF metadata) are read-only views; persistent overrides live in <code>~/.kronk/model_config.yaml</code> (see Chapter 3).</p>
          <h4 id="catalog">Catalog</h4>
          <p>The Catalog area browses entries in <code>~/.kronk/catalog.yaml</code> — your <strong>personal</strong> catalog, seeded on first run from an embedded starter list and grown as you pull or resolve new models against HuggingFace.</p>
          <p>It mirrors <code>kronk catalog list</code>, <code>kronk catalog show</code>, and <code>kronk catalog remove</code>, plus model pulling via <code>kronk model pull</code>. There is no curated upstream catalog; Chapter 7 covers the catalog model in detail.</p>
          <h4 id="libraries">Libraries</h4>
          <p>The Libraries area downloads and manages llama.cpp shared libraries under <code>~/.kronk/libraries/&lt;os&gt;/&lt;arch&gt;/&lt;processor&gt;/</code>. The active install used at runtime is selected via <code>KRONK_LIB_PATH</code>; the BUI can stage additional <code>(arch, os, processor)</code> bundles for other targets but does not hot-reload the active install. See Chapter 2 and the <code>kronk libs</code> CLI for the same operations.</p>
          <h4 id="apps">Apps</h4>
          <p>Three interactive tools live under <strong>Apps</strong>:</p>
          <ul>
            <li><strong>Chat</strong> — a multi-turn chat interface with model selection, system prompt, and full sampling controls. Useful for ad-hoc conversations against any loaded model.</li>
            <li><strong>Model Playground</strong> — an interactive bench for exercising a model under specific configuration (context window, batch sizes, cache mode, sampling parameters) and for running automated sweeps. It lets you load a session, send chat messages, inspect rendered prompts, and probe tool-calling behaviour against a configurable set of tool definitions.</li>
            <li><strong>VRAM Calculator</strong> — a standalone estimator for the VRAM a model will consume given a chosen context window, slot count, KV cache precision, and other parameters. The same calculator is embedded in per-model detail views.</li>
          </ul>
          <h4 id="security">Security</h4>
          <p>When authentication is enabled (Chapter 11), the Security area lets you list, create, and delete signing keys and create user tokens with chosen durations, endpoint scopes, and rate limits. These pages require an admin token configured under Settings; with auth disabled they remain accessible but are not meaningful.</p>
          <h4 id="docs">Docs</h4>
          <p>The Docs area embeds the full Kronk documentation set so it is available offline next to the running server:</p>
          <ul>
            <li><strong>Manual</strong> — this manual, with chapter navigation</li>
            <li><strong>SDK</strong> — Kronk SDK and Model API references plus usage examples</li>
            <li><strong>CLI</strong> — reference for <code>kronk</code> subcommands (catalog, libs, model, run, security, server)</li>
            <li><strong>Web API</strong> — reference for the HTTP endpoints (Chat, Messages, Responses, Embeddings, Rerank, Tokenize, Tools)</li>
          </ul>
          <h4 id="settings">Settings</h4>
          <p>Settings holds BUI-level preferences, including the API token used by the BUI when calling the Web API. Set this when running with <code>--auth-enabled</code> so the BUI can reach security-protected endpoints.</p>
          <h3 id="124-authentication">12.4 Authentication</h3>
          <p>The BUI talks to the same <code>/v1</code> API as any other client. When <code>--auth-enabled</code> is set on <code>kronk server start</code>, every BUI call must carry a valid bearer token — configure it under <strong>Settings</strong>. With auth disabled the BUI works without configuration. See Chapter 11 for key and token management.</p>
          <h3 id="125-notes-on-live-state">12.5 Notes on Live State</h3>
          <p>A few things the BUI deliberately does not do:</p>
          <ul>
            <li>It does not switch the active llama.cpp install in-process. Changing <code>KRONK_LIB_PATH</code> requires a server restart.</li>
            <li>It does not edit <code>~/.kronk/model_config.yaml</code> from the model pages. Persistent configuration changes are made by editing that file directly (see Chapter 3); the BUI's per-model views are read-only.</li>
            <li>The Playground's loaded session is held in server memory; closing the browser tab does not unload the model. Use <strong>Unload Session</strong> before adjusting model configuration.</li>
          </ul>
          <hr />
          <p><em>Next: &lt;a href="#chapter-13-client-integration"&gt;Chapter 13: Client Integration&lt;/a&gt;</em></p>
          <h2 id="chapter-13-client-integration">Chapter 13: Client Integration</h2>
          <p>Kronk's OpenAI-compatible API works with popular AI clients, coding agents, and tools. This chapter covers configuration for the CLI-style coding agents that talk to Kronk, plus a few general-purpose clients.</p>
          <p>Reference configuration files for each agent are provided in the <code>.agents/</code> directory at the project root. These files are ready to copy into each agent's CLI config directory.</p>
          <pre className="code-block"><code>{`.agents/
├── cline/       # Cline (~/.cline)
├── goose/       # Goose (~/.config/goose)
├── kilo/        # Kilo Code (~/.config/kilo)
└── opencode/    # OpenCode (~/.config/opencode)`}</code></pre>
          <h3 id="131-coding-agent-model-configuration">13.1 Coding Agent Model Configuration</h3>
          <p>All coding agents share the same Kronk server and model configuration. The model is configured in <code>model_config.yaml</code> (or the catalog) with an <code>/AGENT</code> suffix that the agent references as its model name.</p>
          <p><strong>Recommended Configuration:</strong></p>
          <pre className="code-block"><code className="language-yaml">{`Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT:
  context-window: 131072
  nseq-max: 2
  incremental-cache: true
  sampling-parameters:
    temperature: 0.6
    top_k: 20
    top_p: 0.95`}</code></pre>
          <p>Another model that works well for coding:</p>
          <pre className="code-block"><code className="language-yaml">{`gemma-4-26B-A4B-it-UD-Q4_K_M/AGENT:
  context-window: 131072
  nseq-max: 2
  incremental-cache: true
  sampling-parameters:
    temperature: 1.0
    top_k: 64
    top_p: 0.95`}</code></pre>
          <p>See <code>zarf/kms/model_config.yaml</code> for additional pre-configured examples.</p>
          <p><strong>Why these settings matter:</strong></p>
          <ul>
            <li><strong>&lt;code&gt;incremental-cache: true&lt;/code&gt;</strong> — IMC caches the conversation prefix in RAM between requests, so only the new message needs prefilling on each turn. This is essential for iterative coding workflows where conversations grow to tens of thousands of tokens.</li>
            <li><strong>&lt;code&gt;nseq-max: 2&lt;/code&gt;</strong> — Two sessions allow the agent's main conversation and a sub-agent to run concurrently without evicting each other's cache.</li>
            <li><strong>&lt;code&gt;context-window: 131072&lt;/code&gt;</strong> — Large context windows are important for coding agents that accumulate tool results, file contents, and long conversations.</li>
          </ul>
          <p><strong>MCP Service:</strong></p>
          <p>The Kronk MCP service provides tools (like <code>web_search</code>) to coding agents. It starts automatically with the Kronk server on <code>http://localhost:9000/mcp</code>. All agent configs below reference this endpoint.</p>
          <h3 id="132-cline">13.2 Cline</h3>
          <p><a href="https://cline.bot">Cline</a> is a coding agent that stores its state under <code>~/.cline/</code>.</p>
          <p><strong>Installation:</strong></p>
          <p>Copy the MCP settings file from <code>.agents/cline/</code> into Cline's settings directory:</p>
          <pre className="code-block"><code className="language-bash">{`cp .agents/cline/cline_mcp_settings.json \\
   ~/.cline/data/settings/cline_mcp_settings.json`}</code></pre>
          <p>This registers Kronk's MCP service so Cline can discover the <code>web_search</code> and other tools served from <code>http://localhost:9000/mcp</code>.</p>
          <p><strong>Connection settings:</strong></p>
          <p>Point Cline at the Kronk Web API:</p>
          <pre className="code-block"><code>{`Base URL: http://localhost:11435/v1
API Key: <your-kronk-token> or 123 if auth is disabled
Model:   Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT`}</code></pre>
          <p>The <code>.agents/cline/globalState.json</code> file is included as a reference for which fields Cline expects (model id, base URL, auto-approval settings). It is not meant to be copied wholesale — Cline manages this file itself.</p>
          <p>Reference files: <code>.agents/cline/</code></p>
          <h3 id="133-kilo-code">13.3 Kilo Code</h3>
          <p><a href="https://kilocode.ai">Kilo Code</a> is a coding agent that reads its configuration from <code>~/.config/kilo/</code>.</p>
          <p><strong>Installation:</strong></p>
          <p>Copy the config files from <code>.agents/kilo/</code> to Kilo's config directory:</p>
          <pre className="code-block"><code className="language-bash">{`cp .agents/kilo/agent.md  ~/.config/kilo/agent.md
cp .agents/kilo/kilo.json ~/.config/kilo/kilo.json`}</code></pre>
          <p>The <code>kilo.json</code> configures Kronk as a custom provider with model definitions and MCP settings. The <code>agent.md</code> file provides custom instructions that tell the model to use Kronk's <code>kronk_fuzzy_edit</code> MCP tool for file edits.</p>
          <p><strong>Key settings in &lt;code&gt;kilo.json&lt;/code&gt;:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT",
  "provider": {
    "kronk": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "http://localhost:11435/v1",
        "apiKey": "123"
      }
    }
  },
  "mcp": {
    "Kronk": {
      "type": "remote",
      "url": "http://localhost:9000/mcp"
    }
  }
}`}</code></pre>
          <p>_Note: Kilo prefixes MCP tool names with the server name (e.g., <code>Kronk</code> server → <code>Kronk_fuzzy_edit</code>). If you see tool name mismatches, check the MCP server key in <code>kilo.json</code>._</p>
          <p>Reference files: <code>.agents/kilo/</code></p>
          <h3 id="134-opencode">13.4 OpenCode</h3>
          <p><a href="https://opencode.ai">OpenCode</a> is a terminal-based coding agent.</p>
          <p><strong>Installation:</strong></p>
          <p>Copy the config files from <code>.agents/opencode/</code> to your OpenCode config directory:</p>
          <pre className="code-block"><code className="language-bash">{`cp .agents/opencode/agent.md       ~/.config/opencode/agent.md
cp .agents/opencode/auth.json      ~/.config/opencode/auth.json
cp .agents/opencode/opencode.jsonc ~/.config/opencode/opencode.jsonc`}</code></pre>
          <p>The <code>opencode.jsonc</code> configures Kronk as a custom provider. The <code>agent.md</code> file provides custom instructions that tell the model to use Kronk's <code>kronk_fuzzy_edit</code> MCP tool for file edits. The <code>auth.json</code> file provides a placeholder API key for local use.</p>
          <p><strong>Key settings in &lt;code&gt;opencode.jsonc&lt;/code&gt;:</strong></p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "kronk/Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT",
  "provider": {
    "kronk": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "http://127.0.0.1:11435/v1"
      }
    }
  },
  "mcp": {
    "kronk": {
      "type": "remote",
      "url": "http://localhost:9000/mcp"
    }
  }
}`}</code></pre>
          <p>_Note: OpenCode prefixes MCP tool names with the server name in lowercase (e.g., <code>kronk</code> server → <code>kronk_fuzzy_edit</code>)._</p>
          <p>Reference files: <code>.agents/opencode/</code></p>
          <h3 id="135-goose">13.5 Goose</h3>
          <p><a href="https://block.github.io/goose/">Goose</a> is a terminal-based AI agent from Block.</p>
          <p><strong>Installation:</strong></p>
          <p>Copy the config from <code>.agents/goose/</code> to Goose's config directory:</p>
          <pre className="code-block"><code className="language-bash">{`cp .agents/goose/config.yaml       ~/.config/goose/config.yaml
cp .agents/goose/custom_kronk.json ~/.config/goose/custom_providers/custom_kronk.json`}</code></pre>
          <p><strong>Key settings in &lt;code&gt;config.yaml&lt;/code&gt;:</strong></p>
          <pre className="code-block"><code className="language-yaml">{`GOOSE_PROVIDER: kronk
GOOSE_MODEL: Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT`}</code></pre>
          <p>The <code>custom_kronk.json</code> file configures the Kronk provider connection.</p>
          <p>Reference files: <code>.agents/goose/</code></p>
          <h3 id="136-openwebui">13.6 OpenWebUI</h3>
          <p>OpenWebUI is a self-hosted chat interface that works with Kronk.</p>
          <p><strong>Configure OpenWebUI:</strong></p>
          <ol>
            <li>Open OpenWebUI settings</li>
            <li>Navigate to Connections → OpenAI API</li>
            <li>Set the base URL:</li>
          </ol>
          <pre className="code-block"><code>{`http://localhost:11435/v1`}</code></pre>
          <ol>
            <li>Set API key to your Kronk token (or any value if auth is disabled)</li>
            <li>Save and refresh models</li>
          </ol>
          <p><strong>Features that work:</strong></p>
          <ul>
            <li>Chat completions with streaming</li>
            <li>Model selection from available models</li>
            <li>System prompts</li>
            <li>Conversation history</li>
          </ul>
          <h3 id="137-python-openai-sdk">13.7 Python OpenAI SDK</h3>
          <p>Use the official OpenAI Python library with Kronk.</p>
          <p><strong>Installation:</strong></p>
          <pre className="code-block"><code className="language-shell">{`pip install openai`}</code></pre>
          <p><strong>Usage:</strong></p>
          <pre className="code-block"><code className="language-python">{`from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:11435/v1",
    api_key="your-kronk-token"  # Or any string if auth disabled
)

response = client.chat.completions.create(
    model="Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Hello!"}
    ],
    stream=True
)

for chunk in response:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")`}</code></pre>
          <h3 id="138-curl-and-http-clients">13.8 curl and HTTP Clients</h3>
          <p>Any HTTP client can call Kronk's REST API directly.</p>
          <p><strong>Basic Request:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -d '{
    "model": "Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'`}</code></pre>
          <p><strong>Streaming Response:</strong></p>
          <p>Streaming responses use Server-Sent Events (SSE) format:</p>
          <pre className="code-block"><code>{`data: {"id":"...","choices":[{"delta":{"content":"Hello"}}],...}

data: {"id":"...","choices":[{"delta":{"content":"!"}}],...}

data: [DONE]`}</code></pre>
          <h3 id="139-langchain">13.9 LangChain</h3>
          <p>Use LangChain with Kronk via the OpenAI integration.</p>
          <p><strong>Installation:</strong></p>
          <pre className="code-block"><code className="language-shell">{`pip install langchain-openai`}</code></pre>
          <p><strong>Usage:</strong></p>
          <pre className="code-block"><code className="language-python">{`from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="http://localhost:11435/v1",
    api_key="your-kronk-token",
    model="Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT",
    streaming=True
)

response = llm.invoke("Explain quantum computing briefly.")
print(response.content)`}</code></pre>
          <hr />
          <p><em>Next: &lt;a href="chapter-14-observability.md"&gt;Chapter 14: Observability&lt;/a&gt;</em></p>
          <h2 id="chapter-14-observability">Chapter 14: Observability</h2>
          <p>Kronk provides comprehensive observability through distributed tracing, Prometheus metrics, pprof profiling, and real-time visualizations.</p>
          <h3 id="141-debug-server">14.1 Debug Server</h3>
          <p>Kronk runs a separate debug server for observability endpoints, isolated from the main API for security.</p>
          <p><strong>Default Ports:</strong></p>
          <ul>
            <li>Main API: <code>localhost:11435</code></li>
            <li>Debug server: <code>localhost:8090</code></li>
          </ul>
          <p><strong>Configure Debug Host:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --debug-host localhost:9090`}</code></pre>
          <p>Or via environment variable:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_WEB_DEBUG_HOST=localhost:9090
kronk server start`}</code></pre>
          <h3 id="142-debug-endpoints">14.2 Debug Endpoints</h3>
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
          <h3 id="143-health-check-endpoints">14.3 Health Check Endpoints</h3>
          <p>Available on the main API port (no authentication required):</p>
          <p><strong>Liveness Check:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/liveness`}</code></pre>
          <p>Response:</p>
          <pre className="code-block"><code className="language-json">{`{
  "status": "up",
  "build": "v1.0.0",
  "host": "hostname",
  "GOMAXPROCS": 8
}`}</code></pre>
          <p><strong>Readiness Check:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/readiness`}</code></pre>
          <p>Returns 200 OK when the server is ready to accept requests.</p>
          <h3 id="144-prometheus-metrics">14.4 Prometheus Metrics</h3>
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
            <li><code>model_prefill_avg</code>, <code>_min</code>, <code>_max</code></li>
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
          <h3 id="145-prometheus-integration">14.5 Prometheus Integration</h3>
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
          <h3 id="146-distributed-tracing-with-tempo">14.6 Distributed Tracing with Tempo</h3>
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
          <h3 id="147-tracing-architecture">14.7 Tracing Architecture</h3>
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
          <h3 id="148-tempo-setup-with-docker">14.8 Tempo Setup with Docker</h3>
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
          <h3 id="149-pprof-profiling">14.9 pprof Profiling</h3>
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
          <h3 id="1410-statsviz-real-time-monitoring">14.10 Statsviz Real-Time Monitoring</h3>
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
          <p>Useful for real-time monitoring during load testing or debugging memory issues.</p>
          <h3 id="1411-logging">14.11 Logging</h3>
          <p>Kronk logs structured JSON to stdout by default.</p>
          <p><strong>Log Levels:</strong></p>
          <p>Logs include context like trace IDs, request details, and timing.</p>
          <p><strong>Insecure Logging:</strong></p>
          <p>For debugging, enable verbose logging that includes message content:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --insecure-logging`}</code></pre>
          <p><strong>Warning:</strong> Insecure logging exposes user prompts and model responses. Never enable in production.</p>
          <p><strong>Environment Variable:</strong></p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_INSECURE_LOGGING=true`}</code></pre>
          <h3 id="1412-configuration-reference">14.12 Configuration Reference</h3>
          <p><strong>Debug Server:</strong></p>
          <ul>
            <li><code>--debug-host</code> - Debug server address (env: <code>KRONK_WEB_DEBUG_HOST</code>, default: <code>0.0.0.0:8090</code>)</li>
          </ul>
          <p><strong>Tracing:</strong></p>
          <ul>
            <li><code>--tempo-host</code> - Tempo collector address (env: <code>KRONK_TEMPO_HOST</code>, default: <code>localhost:4317</code>)</li>
            <li><code>--tempo-service-name</code> - Service name (env: <code>KRONK_TEMPO_SERVICE_NAME</code>, default: <code>kronk</code>)</li>
            <li><code>--tempo-probability</code> - Sampling rate 0.0-1.0 (env: <code>KRONK_TEMPO_PROBABILITY</code>, default: <code>0.25</code>)</li>
          </ul>
          <p><strong>Logging:</strong></p>
          <ul>
            <li><code>--insecure-logging</code> - Log message content (env: <code>KRONK_INSECURE_LOGGING</code>, default: <code>false</code>)</li>
            <li><code>--llama-log</code> - llama.cpp log level, 0=off, 1=on (env: <code>KRONK_LLAMA_LOG</code>, default: <code>1</code>)</li>
          </ul>
          <hr />
          <p><em>Next: &lt;a href="#chapter-15-mcp-service"&gt;Chapter 15: MCP Service&lt;/a&gt;</em></p>
          <h2 id="chapter-15-mcp-service">Chapter 15: MCP Service</h2>
          <p>Kronk includes a built-in <a href="https://modelcontextprotocol.io/">Model Context Protocol (MCP)</a> service that exposes tools to MCP-compatible clients. Two tools are provided today:</p>
          <ul>
            <li><strong>&lt;code&gt;web_search&lt;/code&gt;</strong> — Powered by the <a href="https://brave.com/search/api/">Brave Search API</a>.</li>
            <li><strong>&lt;code&gt;fuzzy_edit&lt;/code&gt;</strong> — A tiered-fuzzy-matching file edit tool that is more reliable than the built-in <code>edit</code> tools used by most coding agents.</li>
          </ul>
          <p>MCP is an open standard that lets AI agents call external tools over a simple JSON-RPC protocol. By running the MCP service, any MCP-compatible client (Cline, Kilo Code, OpenCode, Goose, Cursor, etc.) can discover and invoke tools served by Kronk.</p>
          <h3 id="151-architecture">15.1 Architecture</h3>
          <p>The MCP service can run in two modes:</p>
          <p><strong>Embedded (default)</strong> — When the Kronk model server starts and no external MCP host is configured (<code>--mcp-host</code> is empty), it automatically starts an embedded MCP server on <code>localhost:9000</code>. No extra process is needed.</p>
          <p><strong>Standalone</strong> — Run the MCP service as its own process for independent scaling or when you don't need the full model server:</p>
          <pre className="code-block"><code className="language-shell">{`make mcp-server`}</code></pre>
          <p>Or directly:</p>
          <pre className="code-block"><code className="language-shell">{`go run cmd/server/api/services/mcp/main.go`}</code></pre>
          <p>Both modes serve the same MCP protocol on the same default port (<code>9000</code>).</p>
          <h3 id="152-prerequisites">15.2 Prerequisites</h3>
          <p>The <code>web_search</code> tool requires a Brave Search API key. Get a free key at <a href="https://brave.com/search/api/">https://brave.com/search/api/</a>.</p>
          <p>The <code>fuzzy_edit</code> tool needs no external credentials — it operates on the local filesystem and is available as soon as the MCP service starts.</p>
          <h3 id="153-configuration">15.3 Configuration</h3>
          <p><strong>Environment Variables:</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Variable</th>
                <th>Description</th>
                <th>Default</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>MCP_MCP_HOST</code></td>
                <td>MCP listen address (standalone mode)</td>
                <td><code>localhost:9000</code></td>
              </tr>
              <tr>
                <td><code>MCP_MCP_BRAVEAPIKEY</code></td>
                <td>Brave Search API key (standalone mode)</td>
                <td>—</td>
              </tr>
              <tr>
                <td><code>KRONK_MCP_HOST</code></td>
                <td>External MCP host (empty = embedded mode)</td>
                <td>—</td>
              </tr>
              <tr>
                <td><code>KRONK_MCP_BRAVEAPIKEY</code></td>
                <td>Brave Search API key (embedded mode)</td>
                <td>—</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Embedded mode</strong> — Pass the Brave API key when starting the Kronk server:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_MCP_BRAVEAPIKEY=<your-brave-api-key>
kronk server start`}</code></pre>
          <p>The embedded MCP server will start automatically on <code>localhost:9000</code>.</p>
          <p><strong>Standalone mode</strong> — Start the MCP service as a separate process:</p>
          <pre className="code-block"><code className="language-shell">{`export MCP_MCP_BRAVEAPIKEY=<your-brave-api-key>
make mcp-server`}</code></pre>
          <h3 id="154-available-tools">15.4 Available Tools</h3>
          <h4 id="web_search">web_search</h4>
          <p>Performs a web search and returns a list of relevant web pages with titles, URLs, and descriptions.</p>
          <p><strong>Parameters:</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Parameter</th>
                <th>Type</th>
                <th>Required</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>query</code></td>
                <td>string</td>
                <td>Yes</td>
                <td>Search query</td>
              </tr>
              <tr>
                <td><code>count</code></td>
                <td>int</td>
                <td>No</td>
                <td>Number of results to return (default 10, max 20)</td>
              </tr>
              <tr>
                <td><code>country</code></td>
                <td>string</td>
                <td>No</td>
                <td>Country code for search context (e.g. <code>US</code>, <code>GB</code>, <code>DE</code>)</td>
              </tr>
              <tr>
                <td><code>freshness</code></td>
                <td>string</td>
                <td>No</td>
                <td>Filter by freshness: <code>pd</code> (past day), <code>pw</code> (past week), <code>pm</code> (past month), <code>py</code> (past year)</td>
              </tr>
              <tr>
                <td><code>safesearch</code></td>
                <td>string</td>
                <td>No</td>
                <td>Safe search filter: <code>off</code>, <code>moderate</code>, <code>strict</code> (default <code>moderate</code>)</td>
              </tr>
            </tbody>
          </table>
          <h4 id="fuzzy_edit">fuzzy_edit</h4>
          <p>Edits a file by replacing one occurrence of <code>old_string</code> with <code>new_string</code>. Useful for coding agents whose built-in edit tools are brittle around whitespace or line endings.</p>
          <p>The tool tries three matching strategies in order and stops at the first that produces exactly one match:</p>
          <ol>
            <li><strong>Exact</strong> — byte-for-byte <code>strings.Replace</code>.</li>
            <li><strong>Line-ending normalized</strong> — folds <code>\r\n</code> → <code>\n</code> on both sides, then exact-matches; preserves the file's original line endings on write.</li>
            <li><strong>Indentation insensitive</strong> — strips leading whitespace per line for comparison; replacement text is inserted as-is.</li>
          </ol>
          <p>If no tier yields exactly one match, the call returns an error and the file is not modified.</p>
          <p><strong>Parameters:</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Parameter</th>
                <th>Type</th>
                <th>Required</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>file_path</code></td>
                <td>string</td>
                <td>Yes</td>
                <td>Absolute path to the file to edit</td>
              </tr>
              <tr>
                <td><code>old_string</code></td>
                <td>string</td>
                <td>Yes</td>
                <td>Text to find in the file (fuzzy whitespace matching is applied)</td>
              </tr>
              <tr>
                <td><code>new_string</code></td>
                <td>string</td>
                <td>Yes</td>
                <td>Replacement text</td>
              </tr>
            </tbody>
          </table>
          <h3 id="155-client-configuration">15.5 Client Configuration</h3>
          <p>The MCP service uses the Streamable HTTP transport. Configure your MCP-compatible client to connect to <code>http://localhost:9000/mcp</code>.</p>
          <h4 id="cline">Cline</h4>
          <p>Add the following to your Cline MCP settings (<code>~/.cline/data/settings/cline_mcp_settings.json</code>):</p>
          <pre className="code-block"><code className="language-json">{`{
  "mcpServers": {
    "Kronk": {
      "autoApprove": ["web_search", "fuzzy_edit"],
      "disabled": false,
      "timeout": 60,
      "type": "streamableHttp",
      "url": "http://localhost:9000/mcp"
    }
  }
}`}</code></pre>
          <h4 id="kilo-code">Kilo Code</h4>
          <p>Add the following to your Kilo Code MCP settings (inside <code>~/.config/kilo/kilo.json</code>):</p>
          <pre className="code-block"><code className="language-json">{`{
  "mcpServers": {
    "Kronk": {
      "type": "streamable-http",
      "url": "http://localhost:9000/mcp",
      "disabled": false,
      "alwaysAllow": ["web_search", "fuzzy_edit"],
      "timeout": 60
    }
  }
}`}</code></pre>
          <p>Kilo prefixes MCP tool names with the server key, so the tools are exposed to the model as <code>Kronk_web_search</code> and <code>Kronk_fuzzy_edit</code>.</p>
          <h4 id="opencode">OpenCode</h4>
          <p>Inside <code>~/.config/opencode/opencode.jsonc</code>:</p>
          <pre className="code-block"><code className="language-jsonc">{`{
  "mcp": {
    "kronk": {
      "type": "remote",
      "url": "http://localhost:9000/mcp"
    }
  }
}`}</code></pre>
          <p>OpenCode lowercases the server prefix, so the tools are exposed as <code>kronk_web_search</code> and <code>kronk_fuzzy_edit</code>.</p>
          <h4 id="goose">Goose</h4>
          <p>Goose discovers tools from any MCP server it is configured against. Add an entry to your Goose configuration that points to <code>http://localhost:9000/mcp</code> using the streamable-HTTP transport. Both <code>web_search</code> and <code>fuzzy_edit</code> will appear in the tool list once the connection is established.</p>
          <h3 id="156-testing-with-curl">15.6 Testing with curl</h3>
          <p>You can test the MCP service manually using curl. See the makefile targets for convenience commands.</p>
          <p><strong>Initialize a session:</strong></p>
          <pre className="code-block"><code className="language-shell">{`make curl-mcp-init`}</code></pre>
          <p>This returns the <code>Mcp-Session-Id</code> header needed for subsequent requests.</p>
          <p><strong>List available tools:</strong></p>
          <pre className="code-block"><code className="language-shell">{`make curl-mcp-tools-list SESSIONID=<session-id>`}</code></pre>
          <p><strong>Call web_search:</strong></p>
          <pre className="code-block"><code className="language-shell">{`make curl-mcp-web-search SESSIONID=<session-id>`}</code></pre>
          <hr />
          <p><em>Next: &lt;a href="#chapter-16-troubleshooting"&gt;Chapter 16: Troubleshooting&lt;/a&gt;</em></p>
          <h2 id="chapter-16-troubleshooting">Chapter 16: Troubleshooting</h2>
          <p>This chapter covers common issues, their causes, and solutions.</p>
          <h3 id="161-library-issues">16.1 Library Issues</h3>
          <p><strong>Error: "unable to load library"</strong></p>
          <p>The llama.cpp shared libraries are missing or incompatible with your hardware.</p>
          <p><strong>Solution:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local`}</code></pre>
          <p>Or download via the BUI Libraries page.</p>
          <p>Kronk auto-detects your GPU hardware and selects the correct library bundle. If auto-detection fails, set the processor explicitly:</p>
          <pre className="code-block"><code className="language-shell">{`# For Mac with Apple Silicon
KRONK_PROCESSOR=metal kronk libs --local

# For NVIDIA GPU
KRONK_PROCESSOR=cuda kronk libs --local

# For AMD GPU (ROCm, Linux only)
KRONK_PROCESSOR=rocm kronk libs --local

# For Vulkan (cross-platform, including iGPUs)
KRONK_PROCESSOR=vulkan kronk libs --local

# For CPU only
KRONK_PROCESSOR=cpu kronk libs --local`}</code></pre>
          <p>See <a href="chapter-03-model-configuration.md#32-processor-selection">Chapter 3: Processor Selection</a> for details on how auto-detection works on each platform.</p>
          <p><strong>Problem: New library version causes crashes or bad output</strong></p>
          <p>The standalone <code>kronk libs</code> CLI installs the well-known default version of llama.cpp by default, which is conservative and changes only when the Kronk release bumps it. The model server (<code>kronk server start</code>) defaults to <code>--allow-upgrade=true</code> and tracks the latest llama.cpp release, so a long-running server can pick up a regression — crashes during model loading, decode errors, or degraded output quality. When this happens, pin the library to a known-good version using <code>KRONK_LIB_VERSION</code> (or <code>--version</code> on the CLI).</p>
          <p><strong>Pin to a specific version:</strong></p>
          <pre className="code-block"><code className="language-shell">{`# Install a specific version
kronk libs --version=b5490 --local

# Or use the environment variable
KRONK_LIB_VERSION=b5490 kronk libs --local`}</code></pre>
          <p><strong>Start the server with a pinned version:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --lib-version=b5490`}</code></pre>
          <p>Or set it globally so both <code>kronk libs</code> and <code>kronk server start</code> use the same version:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_LIB_VERSION=b5490
kronk libs --local
kronk server start`}</code></pre>
          <p><strong>Check your current installed version:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --version`}</code></pre>
          <p>This shows the installed version, architecture, OS, processor, and the latest available version. The CLI will only upgrade past the installed version when you pass <code>--upgrade</code>; otherwise it sticks to the well-known default version (or whatever is on disk if it is already newer).</p>
          <p><strong>When to pin:</strong> Pin whenever a new llama.cpp release breaks something you depend on. Unset <code>KRONK_LIB_VERSION</code> once the upstream fix is released to resume tracking either the default version (CLI) or latest (server with <code>--allow-upgrade=true</code>).</p>
          <p>See <a href="chapter-02-installation.md#23-installing-libraries">Chapter 2: Installing Libraries</a> for the full compatibility matrix.</p>
          <p><strong>Error: "unknown device"</strong></p>
          <p>The specified GPU device is not recognized by the loaded library.</p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Wrong processor for your hardware (e.g., <code>cuda</code> library on a Mac)</li>
            <li>GPU drivers not installed or outdated</li>
            <li>Library/processor mismatch (CPU library loaded but GPU device requested)</li>
          </ul>
          <p><strong>Solution:</strong></p>
          <p>Verify your processor and re-download libraries:</p>
          <pre className="code-block"><code className="language-shell">{`# Check what Kronk detects
kronk devices

# Re-install matching libraries
kronk libs --local`}</code></pre>
          <p><strong>Problem: "unable to load library" pointing at the wrong folder</strong></p>
          <p>Library bundles now live at <code>&lt;base&gt;/libraries/&lt;os&gt;/&lt;arch&gt;/&lt;processor&gt;/</code>, one folder per <code>(arch, os, processor)</code> triple. If <code>dlopen</code> reports a path like <code>&lt;base&gt;/libraries/libllama.dylib</code> (libraries directly under the root), you have an installation from before the per-triple layout. The SDK migrates the legacy layout into the correct triple folder automatically on first call to <code>libs.New()</code>/<code>libs.Path()</code>. If migration fails, just re-run:</p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local`}</code></pre>
          <p>The new install lands at <code>&lt;base&gt;/libraries/&lt;os&gt;/&lt;arch&gt;/&lt;processor&gt;/</code> and the runtime resolves to the same folder.</p>
          <p><strong>Problem: Server is loading the wrong install</strong></p>
          <p>To switch the active install (for example to a previously downloaded CUDA or CPU bundle), point <code>KRONK_LIB_PATH</code> at its triple folder and restart the server. Libraries are not hot-reloaded.</p>
          <pre className="code-block"><code className="language-shell">{`# List installed bundles
kronk libs --list-installs

# Switch active install
export KRONK_LIB_PATH=~/.kronk/libraries/linux/amd64/cuda
kronk server start`}</code></pre>
          <p>If <code>KRONK_LIB_PATH</code> points at a directory containing <code>version.json</code>, Kronk uses it as-is. If it points at a non-empty directory without a <code>version.json</code>, Kronk treats it as a read-only user-managed build and will refuse mutating operations against it (errors will mention <code>read-only</code> or <code>ErrReadOnly</code>).</p>
          <h3 id="162-model-loading-failures">16.2 Model Loading Failures</h3>
          <p><strong>Error: "unable to load model"</strong></p>
          <p>The model file is missing, corrupted, or incompatible.</p>
          <p><strong>Check model exists:</strong></p>
          <pre className="code-block"><code className="language-shell">{`ls ~/.kronk/models/`}</code></pre>
          <p><strong>Re-download the model:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk model pull <model-id> --local`}</code></pre>
          <p><strong>Problem: Model exists but server says "model not found"</strong></p>
          <p>The model files are on disk but Kronk can't find them. This happens when the model index (<code>.index.yaml</code>) is out of sync — for example after manually moving model files, a failed download, or removing a model outside of Kronk.</p>
          <p><strong>Solution — rebuild the model index:</strong></p>
          <pre className="code-block"><code className="language-shell">{`# With the server running (triggers re-index via API)
kronk model index

# Without the server (rebuilds index directly on disk)
kronk model index --local`}</code></pre>
          <p>This scans <code>~/.kronk/models/</code>, validates each GGUF file, and rebuilds the <code>.index.yaml</code> that Kronk uses for fast model lookups. You can also trigger a rebuild from the BUI Models page.</p>
          <p><strong>When to rebuild the index:</strong></p>
          <ul>
            <li>Model files were moved or renamed manually</li>
            <li>A download was interrupted and left partial files</li>
            <li><code>kronk model list</code> doesn't show a model you know is downloaded</li>
            <li>After deleting model files outside of <code>kronk model remove</code></li>
          </ul>
          <h3 id="163-memory-errors">16.3 Memory Errors</h3>
          <p><strong>Error: "unable to init context" or "unable to get memory"</strong></p>
          <p>Insufficient memory for the model plus its KV cache at the configured context window size.</p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Context window too large for available VRAM/RAM</li>
            <li>Too many parallel sequences (<code>nseq-max</code>)</li>
            <li>Model weights don't fit in available memory</li>
          </ul>
          <p><strong>Solutions:</strong></p>
          <p>Reduce context window:</p>
          <pre className="code-block"><code className="language-yaml">{`Qwen/Qwen3-8B-Q8_0:
  context-window: 8192 # Reduce from 32768`}</code></pre>
          <p>Reduce parallel sequences:</p>
          <pre className="code-block"><code className="language-yaml">{`Qwen/Qwen3-8B-Q8_0:
  nseq-max: 1 # Single request at a time`}</code></pre>
          <p>Use quantized KV cache:</p>
          <pre className="code-block"><code className="language-yaml">{`Qwen/Qwen3-8B-Q8_0:
  cache-type-k: q8_0 # ~50% less KV cache memory vs f16
  cache-type-v: q8_0`}</code></pre>
          <p>See <a href="chapter-03-model-configuration.md#39-vram-estimation">Chapter 3: VRAM Estimation</a> for how to calculate whether a model fits in your hardware.</p>
          <p><strong>Error: "the context window is full"</strong></p>
          <p>The total token count (input + cached + generated) exceeds the configured context window during inference.</p>
          <p><strong>Solutions:</strong></p>
          <ul>
            <li>Reduce input size (fewer messages, shorter prompts)</li>
            <li>Increase <code>context-window</code> in model config (requires more VRAM)</li>
            <li>Enable YaRN for extended context (see <a href="chapter-06-yarn-extended-context.md">Chapter 6</a>)</li>
          </ul>
          <p><strong>Error: "input tokens [N] exceed context window [M]"</strong></p>
          <p>The prompt itself (after tokenization) is larger than the context window, before any generation can begin.</p>
          <p><strong>Solutions:</strong></p>
          <ul>
            <li>Shorten the prompt or system message</li>
            <li>Increase <code>context-window</code></li>
            <li>If using IMC, the cached prefix counts toward the limit</li>
          </ul>
          <h3 id="164-request-timeouts">16.4 Request Timeouts</h3>
          <p><strong>Error: "context deadline exceeded"</strong></p>
          <p>The request took longer than the configured HTTP timeout.</p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Large prefill with many input tokens</li>
            <li>Server under heavy load with all slots busy</li>
            <li>Model too slow for the requested output length</li>
          </ul>
          <p><strong>Solutions:</strong></p>
          <p>Increase HTTP timeouts:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --read-timeout 5m \\
  --write-timeout 30m`}</code></pre>
          <p>Or via environment variables:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_WEB_READ_TIMEOUT=5m
export KRONK_WEB_WRITE_TIMEOUT=30m`}</code></pre>
          <p><strong>Error: "server busy processing other requests, try again shortly"</strong></p>
          <p>All IMC sessions have pending cache builds in-flight, or the slot preemption timeout was reached.</p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>All sessions are busy building caches simultaneously</li>
            <li>A long-running request is occupying the slot pool</li>
          </ul>
          <p><strong>Solutions:</strong></p>
          <ul>
            <li>Wait and retry the request — the error is transient</li>
            <li>Increase <code>nseq-max</code> to allow more concurrent sessions</li>
            <li>Increase <code>cache-slot-timeout</code> (default: 30 seconds) if requests need more time</li>
          </ul>
          <h3 id="165-authentication-errors">16.5 Authentication Errors</h3>
          <p><strong>Error: "unauthorized: no authorization header"</strong></p>
          <p>Authentication is enabled but no token was provided.</p>
          <p><strong>Solution:</strong></p>
          <p>Include the Authorization header:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Authorization: Bearer $(cat ~/.kronk/keys/master.jwt)" \\
  -H "Content-Type: application/json" \\
  -d '{...}'`}</code></pre>
          <p><strong>Error: "invalid token"</strong></p>
          <p>The token is malformed, expired, or signed with an unknown key.</p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Token has expired (check <code>--duration</code> when created)</li>
            <li>Signing key was deleted or rotated</li>
            <li>Token is truncated or corrupted</li>
          </ul>
          <p><strong>Solution:</strong></p>
          <p>Create a new token:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)
kronk security token create \\
  --duration 720h \\
  --endpoints chat-completions,embeddings`}</code></pre>
          <p><strong>Error: "endpoint not authorized"</strong></p>
          <p>The token doesn't include the requested endpoint in its allowed list.</p>
          <p><strong>Solution:</strong></p>
          <p>Create a new token with the required endpoints:</p>
          <pre className="code-block"><code className="language-shell">{`kronk security token create \\
  --duration 720h \\
  --endpoints chat-completions,embeddings,rerank,responses,messages`}</code></pre>
          <p><strong>Error: "rate limit exceeded"</strong></p>
          <p>The token has exceeded its configured rate limit.</p>
          <p><strong>Solution:</strong></p>
          <p>Wait for the rate limit window to reset, or create a new token with higher limits:</p>
          <pre className="code-block"><code className="language-shell">{`kronk security token create \\
  --duration 720h \\
  --endpoints "chat-completions:10000/day"`}</code></pre>
          <h3 id="166-streaming-issues">16.6 Streaming Issues</h3>
          <p><strong>Problem: Streaming stops mid-response</strong></p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Client disconnected (network timeout, browser tab closed)</li>
            <li>HTTP write timeout reached on the server</li>
            <li>Model generated an end-of-generation token (normal completion)</li>
          </ul>
          <p><strong>Solutions:</strong></p>
          <ul>
            <li>Check if the response includes a <code>finish_reason</code> — if it does, the model stopped normally</li>
            <li>Increase <code>--write-timeout</code> if large responses are being cut off</li>
            <li>Run the server in foreground to see logs:</li>
          </ul>
          <pre className="code-block"><code className="language-shell">{`kronk server start  # Logs print to stdout`}</code></pre>
          <p><strong>Problem: SSE events not parsing correctly</strong></p>
          <p>Ensure your client handles Server-Sent Events (SSE) format. Each event is prefixed with <code>data: </code> and terminated by two newlines:</p>
          <pre className="code-block"><code>{`data: {"id":"...","choices":[{"delta":{"content":"Hello"}}],...}\\n\\n
data: [DONE]\\n\\n`}</code></pre>
          <h3 id="167-performance-issues">16.7 Performance Issues</h3>
          <p><strong>Problem: Slow time to first token (TTFT)</strong></p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Large conversation prefix being re-processed from scratch</li>
            <li>IMC not enabled (every request re-processes the full prompt)</li>
            <li>Cold model load on first request</li>
          </ul>
          <p><strong>Solutions:</strong></p>
          <p>Enable IMC to cache the conversation prefix:</p>
          <pre className="code-block"><code className="language-yaml">{`Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT:
  incremental-cache: true`}</code></pre>
          <p>With IMC, only the new message is prefilled — cached tokens are restored from RAM in ~10-30ms regardless of conversation length.</p>
          <p><strong>Problem: Slow token generation (tokens/second)</strong></p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Running on CPU instead of GPU</li>
            <li>Model too large for available VRAM (partial CPU offload)</li>
            <li>MoE model on Apple Silicon (scattered memory access patterns)</li>
          </ul>
          <p><strong>Solutions:</strong></p>
          <p>Check GPU is being used:</p>
          <pre className="code-block"><code className="language-shell">{`# On macOS, check Metal usage
sudo powermetrics --samplers gpu_power

# On Linux with NVIDIA
nvidia-smi`}</code></pre>
          <p>Ensure all layers are on GPU (default):</p>
          <pre className="code-block"><code className="language-yaml">{`Qwen/Qwen3-8B-Q8_0:
  ngpu-layers: 0 # 0 = all layers on GPU (default)`}</code></pre>
          <p>For MoE models on Apple Silicon, consider a dense model at lower quantization — the sequential memory access pattern is faster than MoE's scattered expert routing (see <a href="chapter-03-model-configuration.md#310-model-specific-tuning">Chapter 3: Model-Specific Tuning</a>).</p>
          <h3 id="168-imc-caching-issues">16.8 IMC Caching Issues</h3>
          <p><strong>Problem: Every request triggers a full cache rebuild</strong></p>
          <p><strong>Causes:</strong></p>
          <ul>
            <li>Client is modifying earlier messages between requests</li>
            <li>Non-deterministic Jinja template producing different tokens for the same messages</li>
            <li><code>nseq-max</code> too low for the number of concurrent sub-agents (cache thrashing)</li>
          </ul>
          <p><strong>Diagnosis:</strong></p>
          <p>Look for these log patterns:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Log Message</th>
                <th>Meaning</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>session[N] mismatch</code></td>
                <td>Hash changed — messages were modified</td>
              </tr>
              <tr>
                <td><code>sys-prompt-match</code></td>
                <td>System prompt preserved, conversation rebuilt</td>
              </tr>
              <tr>
                <td><code>token prefix match found</code></td>
                <td>Partial prefix salvaged via token comparison</td>
              </tr>
              <tr>
                <td><code>no usable token prefix match</code></td>
                <td>No salvageable prefix, full rebuild required</td>
              </tr>
              <tr>
                <td><code>kv-pressure-evict</code></td>
                <td>Stale session evicted to free KV space</td>
              </tr>
              <tr>
                <td><code>all sessions pending, waiting</code></td>
                <td>All sessions busy, request is waiting</td>
              </tr>
              <tr>
                <td><code>imc-restore-start</code> / <code>imc-restore-done</code></td>
                <td>KV state being restored from RAM</td>
              </tr>
              <tr>
                <td><code>imc-snapshot-start</code> / <code>imc-snapshot-done</code></td>
                <td>KV state being snapshotted to RAM</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Solutions:</strong></p>
          <ul>
            <li>Increase <code>nseq-max</code> to match the number of concurrent sub-agents</li>
            <li>Check if the client is modifying conversation history between requests</li>
            <li>If using a non-deterministic template, IMC falls back to token prefix matching automatically — this is expected behavior</li>
          </ul>
          <p><strong>Problem: IMC restore fails</strong></p>
          <p><strong>Error:</strong> <code>imc restore failed for seq N</code></p>
          <p>The RAM-to-VRAM restore (<code>StateSeqSetData</code>) failed for a session.</p>
          <p><strong>Cause:</strong> Usually indicates the KV cache memory could not be allocated (VRAM pressure from other sessions or models).</p>
          <p><strong>Solution:</strong> The session is automatically reset and the next request triggers a full rebuild. If this happens frequently, reduce <code>nseq-max</code> or <code>context-window</code> to lower VRAM pressure.</p>
          <h3 id="169-viewing-logs">16.9 Viewing Logs</h3>
          <p><strong>Run server in foreground:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start`}</code></pre>
          <p>All logs print to stdout with structured key-value format.</p>
          <p><strong>Enable verbose logging:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --insecure-logging`}</code></pre>
          <p>This logs full message content including prompts and responses. Never use in production — it exposes sensitive conversation data.</p>
          <p><strong>Enable llama.cpp logging:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --llama-log 1`}</code></pre>
          <p>Shows low-level inference engine messages from llama.cpp. Useful for debugging GPU issues, memory allocation failures, and decode errors.</p>
          <p><strong>Disable llama.cpp logging:</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --llama-log 0`}</code></pre>
          <h3 id="1610-common-error-messages">16.10 Common Error Messages</h3>
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
                <td><code>unable to load library</code></td>
                <td>Missing llama.cpp libraries</td>
                <td><code>kronk libs --local</code></td>
              </tr>
              <tr>
                <td><code>unknown device</code></td>
                <td>Wrong processor for hardware</td>
                <td>Check <code>kronk devices</code>, re-install libs</td>
              </tr>
              <tr>
                <td><code>unable to load model</code></td>
                <td>Missing or corrupt model file</td>
                <td>Re-download with <code>kronk model pull</code></td>
              </tr>
              <tr>
                <td><code>unable to init context</code></td>
                <td>Insufficient VRAM/RAM</td>
                <td>Reduce context-window or nseq-max</td>
              </tr>
              <tr>
                <td><code>input tokens [N] exceed context window [M]</code></td>
                <td>Prompt too large</td>
                <td>Shorten prompt or increase context</td>
              </tr>
              <tr>
                <td><code>the context window is full</code></td>
                <td>KV cache exhausted during decode</td>
                <td>Reduce input size or increase context</td>
              </tr>
              <tr>
                <td><code>context deadline exceeded</code></td>
                <td>HTTP timeout reached</td>
                <td>Increase <code>--write-timeout</code></td>
              </tr>
              <tr>
                <td><code>server busy processing other requests</code></td>
                <td>All IMC sessions busy</td>
                <td>Retry, or increase nseq-max</td>
              </tr>
              <tr>
                <td><code>no authorization header</code></td>
                <td>Missing auth token</td>
                <td>Add <code>Authorization: Bearer &lt;token&gt;</code></td>
              </tr>
              <tr>
                <td><code>invalid token</code></td>
                <td>Expired or malformed JWT</td>
                <td>Create a new token</td>
              </tr>
              <tr>
                <td><code>endpoint not authorized</code></td>
                <td>Token missing endpoint scope</td>
                <td>Create token with correct endpoints</td>
              </tr>
              <tr>
                <td><code>rate limit exceeded</code></td>
                <td>Quota exhausted</td>
                <td>Wait for reset or increase limit</td>
              </tr>
              <tr>
                <td><code>engine shutting down</code></td>
                <td>Server is stopping</td>
                <td>Wait for shutdown, restart server</td>
              </tr>
              <tr>
                <td><code>huggingface 401 / 403</code></td>
                <td>Gated/private repo or rate limit</td>
                <td>Set <code>KRONK_HF_TOKEN</code> env var</td>
              </tr>
              <tr>
                <td><code>model doesn't support embedding</code></td>
                <td>Wrong model for endpoint</td>
                <td>Use an embedding model</td>
              </tr>
              <tr>
                <td><code>model doesn't support reranking</code></td>
                <td>Wrong model for endpoint</td>
                <td>Use a reranking model</td>
              </tr>
              <tr>
                <td><code>imc restore failed</code></td>
                <td>RAM→VRAM restore failed</td>
                <td>Auto-recovers; reduce VRAM pressure</td>
              </tr>
              <tr>
                <td><code>imc extend stale</code></td>
                <td>Concurrent cache modification</td>
                <td>Auto-retries; transient</td>
              </tr>
            </tbody>
          </table>
          <h3 id="1611-catalog-model-pull-issues">16.11 Catalog &amp; Model Pull Issues</h3>
          <p><strong>Error: &lt;code&gt;huggingface 401&lt;/code&gt; / &lt;code&gt;403&lt;/code&gt; during &lt;code&gt;kronk model pull&lt;/code&gt;</strong></p>
          <p>The repo is gated/private or the request was throttled.</p>
          <p><strong>Solution:</strong> export a HuggingFace token before pulling. The token must have read access to any gated repos you intend to use:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_HF_TOKEN=hf_xxx
kronk model pull <provider/model-id>`}</code></pre>
          <p><strong>Problem: &lt;code&gt;kronk model pull &lt;id&gt;&lt;/code&gt; says "model not found in catalog"</strong></p>
          <p>The id has not been resolved against HuggingFace yet. Use <code>model resolve</code> to look it up and seed the local catalog:</p>
          <pre className="code-block"><code className="language-shell">{`kronk model resolve https://huggingface.co/<owner>/<repo>
kronk model pull <provider/model-id>`}</code></pre>
          <p><strong>Problem: &lt;code&gt;catalog.yaml&lt;/code&gt; was hand-edited and Kronk now refuses to start</strong></p>
          <p>The catalog file is YAML keyed by canonical id with strict typing. Run <code>kronk catalog list --local</code> to validate it; remove the broken entry with <code>kronk catalog remove &lt;id&gt; --local</code> and re-add it via <code>kronk model resolve</code>.</p>
          <p><strong>Problem: pull failed mid-download, &lt;code&gt;kronk model list&lt;/code&gt; shows the model as invalid</strong></p>
          <p>Partial files are left behind under <code>~/.kronk/models/&lt;provider&gt;/&lt;family&gt;/</code>. Remove the model and re-pull:</p>
          <pre className="code-block"><code className="language-shell">{`kronk model remove <provider/model-id> --local
kronk model pull <provider/model-id>
kronk model index --local`}</code></pre>
          <h3 id="1612-mcp-service-issues">16.12 MCP Service Issues</h3>
          <p><strong>Error: &lt;code&gt;/mcp&lt;/code&gt; returns 404</strong></p>
          <p>The MCP endpoint is <code>http://localhost:9000/mcp</code> (no trailing slash). The client must use the Streamable HTTP transport — Cline calls it <code>streamableHttp</code>, Kilo Code uses <code>streamable-http</code>, and OpenCode/Goose both spell it <code>remote</code>.</p>
          <p><strong>Error: &lt;code&gt;web_search&lt;/code&gt; reports "missing Brave API key"</strong></p>
          <p>The Brave Search key is unset. Provide it before starting the server:</p>
          <pre className="code-block"><code className="language-shell">{`# Embedded mode (kronk server start)
export KRONK_MCP_BRAVEAPIKEY=<your-brave-api-key>

# Standalone (make mcp-server)
export MCP_MCP_BRAVEAPIKEY=<your-brave-api-key>`}</code></pre>
          <p><strong>Problem: model can't find the tool ("unknown tool kronk_fuzzy_edit")</strong></p>
          <p>Each MCP-aware client prefixes tool names with the server key. Check that the prefix matches the key you used in the client config:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Client</th>
                <th>Server key in config</th>
                <th>Tool names exposed</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Cline</td>
                <td><code>Kronk</code></td>
                <td><code>web_search</code>, <code>fuzzy_edit</code></td>
              </tr>
              <tr>
                <td>Kilo</td>
                <td><code>Kronk</code></td>
                <td><code>Kronk_web_search</code>, <code>Kronk_fuzzy_edit</code></td>
              </tr>
              <tr>
                <td>OpenCode</td>
                <td><code>kronk</code></td>
                <td><code>kronk_web_search</code>, <code>kronk_fuzzy_edit</code></td>
              </tr>
              <tr>
                <td>Goose</td>
                <td>(lowercase)</td>
                <td><code>kronk_web_search</code>, <code>kronk_fuzzy_edit</code></td>
              </tr>
            </tbody>
          </table>
          <p><strong>Error: &lt;code&gt;fuzzy_edit&lt;/code&gt; returns "old_string not found in file (even with fuzzy matching)"</strong></p>
          <p>The search snippet is either absent from the file or matches more than once. Tighten the snippet to a unique block of lines, or break the edit into a smaller anchor that appears exactly once.</p>
          <p><strong>Problem: embedded MCP server is not starting</strong></p>
          <p>Setting <code>KRONK_MCP_HOST</code> to a non-empty value tells <code>kronk server</code> to defer to an external MCP host instead of starting its own. Unset it (or run the standalone service via <code>make mcp-server</code>) if you want the embedded mode back.</p>
          <h3 id="1613-port-conflicts-filesystem">16.13 Port Conflicts &amp; Filesystem</h3>
          <p><strong>Error: &lt;code&gt;bind: address already in use&lt;/code&gt;</strong></p>
          <p>Another process is already listening on the port Kronk is trying to bind. Default ports are <code>11435</code> (API), <code>8090</code> (debug), and <code>9000</code> (MCP).</p>
          <p><strong>Solutions:</strong></p>
          <pre className="code-block"><code className="language-shell">{`# Find the offending process
lsof -i :11435

# Or move Kronk to a different port
kronk server start --api-host 0.0.0.0:21435`}</code></pre>
          <p><strong>Error: &lt;code&gt;permission denied&lt;/code&gt; reading or writing &lt;code&gt;~/.kronk/&lt;/code&gt;</strong></p>
          <p>Ensure your user owns the kronk base directory. Auth in particular expects <code>~/.kronk/keys/</code> to be <code>0700</code>:</p>
          <pre className="code-block"><code className="language-shell">{`chmod -R u+rwX ~/.kronk
chmod 700 ~/.kronk/keys`}</code></pre>
          <p><strong>Error: &lt;code&gt;lock file already exists&lt;/code&gt; from BadgerDB (&lt;code&gt;~/.kronk/badger/LOCK&lt;/code&gt;)</strong></p>
          <p>Only one Kronk process may hold the rate-limit DB at a time. Confirm no other server is running, then remove the lock:</p>
          <pre className="code-block"><code className="language-shell">{`ps aux | grep "kronk server"
rm ~/.kronk/badger/LOCK   # only if no process is using it`}</code></pre>
          <p><strong>Problem: &lt;code&gt;kronk server start -d&lt;/code&gt; says "already running" but no process exists</strong></p>
          <p>The PID file is stale. Remove it and start again:</p>
          <pre className="code-block"><code className="language-shell">{`rm ~/.kronk/kronk.pid
kronk server start -d`}</code></pre>
          <p><strong>Problem: model pull fails with "no space left on device"</strong></p>
          <p>The models directory is full. Free space by removing unused models:</p>
          <pre className="code-block"><code className="language-shell">{`kronk model list --local
kronk model remove <provider/model-id> --local`}</code></pre>
          <p>Models live under <code>~/.kronk/models/</code>; check available space with <code>df -h ~/.kronk/models</code>.</p>
          <h3 id="1614-getting-help">16.14 Getting Help</h3>
          <p><strong>Check server liveness:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/liveness`}</code></pre>
          <p><strong>Check server readiness (model loaded):</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/readiness`}</code></pre>
          <p><strong>List loaded models:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/models`}</code></pre>
          <p><strong>Check Prometheus metrics:</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8090/metrics`}</code></pre>
          <p><strong>View goroutine stacks (for hangs):</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8090/debug/pprof/goroutine?debug=2`}</code></pre>
          <p><strong>CPU profile (for slow inference):</strong></p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:8090/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof`}</code></pre>
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
          <p><em>Next: &lt;a href="chapter-17-developer-guide.md"&gt;Chapter 17: Developer Guide&lt;/a&gt;</em></p>
          <h2 id="chapter-17-developer-guide">Chapter 17: Developer Guide</h2>
          <p>This chapter covers development workflows, build commands, and code conventions for contributors to the Kronk project.</p>
          <h3 id="171-quick-reference">17.1 Quick Reference</h3>
          <p>Here is a quick chart of some of the more important make commands.</p>
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
                <td><code>make install-kronk</code></td>
              </tr>
              <tr>
                <td>Run all tests</td>
                <td><code>make test</code> (requires env vars below)</td>
              </tr>
              <tr>
                <td>Test prereqs</td>
                <td><code>export RUN_IN_PARALLEL=yes; export GITHUB_WORKSPACE=$(pwd)</code></td>
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
                <td>Run server (bg)</td>
                <td><code>make kronk-server-detach</code></td>
              </tr>
              <tr>
                <td>Tail server log</td>
                <td><code>make kronk-server-logs</code></td>
              </tr>
              <tr>
                <td>Stop server</td>
                <td><code>make kronk-server-stop</code></td>
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
                <td>Post-edit checks</td>
                <td><code>gofmt -s -w &lt;files&gt; && go vet ./... && staticcheck ./...</code></td>
              </tr>
              <tr>
                <td>Developer setup</td>
                <td><code>make setup</code> (configures git hooks)</td>
              </tr>
            </tbody>
          </table>
          <h3 id="172-build-test-commands">17.2 Build &amp; Test Commands</h3>
          <p><strong>Install CLI locally:</strong></p>
          <pre className="code-block"><code className="language-shell">{`make install-kronk`}</code></pre>
          <p>This is the canonical install path used everywhere in the docs. Internally it runs <code>go install ./cmd/kronk</code> with the project's build tags.</p>
          <p><strong>Run all tests:</strong></p>
          <pre className="code-block"><code className="language-shell">{`# Set required environment variables (project root must be absolute)
export RUN_IN_PARALLEL=yes
export GITHUB_WORKSPACE=$(pwd)

# Run from project root directory
make test`}</code></pre>
          <p><code>make test</code> expands to <code>test-only + lint + vuln-check + diff</code>. The <code>test-only</code> target depends on <code>install-libraries</code> and <code>install-test-models</code>, so the llama.cpp libraries and test GGUF models are downloaded automatically the first time you run it.</p>
          <p>For fast iteration (skip <code>lint</code>, <code>vuln-check</code>, and <code>diff</code>):</p>
          <pre className="code-block"><code className="language-shell">{`make test-only`}</code></pre>
          <p><strong>Run a single test:</strong></p>
          <pre className="code-block"><code className="language-shell">{`go test -v -count=1 -run TestName ./sdk/kronk/...`}</code></pre>
          <p>The path can target any package (e.g. <code>./cmd/...</code> or a specific subpackage); <code>./sdk/kronk/...</code> is just where most inference tests live.</p>
          <h3 id="173-developer-setup">17.3 Developer Setup</h3>
          <p>Configure git hooks for automatic pre-commit checks:</p>
          <pre className="code-block"><code className="language-shell">{`make setup`}</code></pre>
          <p>This enables a pre-commit hook that automatically runs:</p>
          <ul>
            <li><code>make kronk-docs</code> - Regenerates documentation</li>
            <li><code>make bui-build</code> - Rebuilds the BUI frontend</li>
            <li><code>gomod2nix --dir . --outdir zarf/nix</code> - Regenerates the Nix lock file (only if <code>gomod2nix</code> is on <code>PATH</code>)</li>
            <li><code>git add -A</code> - Stages all generated changes so they land in the same commit</li>
          </ul>
          <p><strong>Toolchain dependencies:</strong></p>
          <p><code>make setup</code> only configures git hooks. The lint/vuln/codegen toolchain is installed separately:</p>
          <pre className="code-block"><code className="language-shell">{`make install-gotooling   # staticcheck, govulncheck, protoc-gen-go(-grpc), gomod2nix
make install-tooling     # brew: protobuf, grpcurl, node (only needed for codegen / BUI work)`}</code></pre>
          <p>A fresh checkout that skips <code>install-gotooling</code> will fail the <code>lint</code> and <code>vuln-check</code> steps of <code>make test</code> and the post-edit checks listed in §17.1.</p>
          <h3 id="174-project-architecture">17.4 Project Architecture</h3>
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
                <td>CLI tool (subcommands: <code>catalog</code>, <code>client</code>, <code>devices</code>, <code>libs</code>, <code>model</code>, <code>run</code>, <code>security</code>, <code>server</code>)</td>
              </tr>
              <tr>
                <td><code>cmd/server/</code></td>
                <td>OpenAI-compatible HTTP model server with embedded BUI. Internal gRPC over <code>bufconn</code> for the embedded auth service.</td>
              </tr>
              <tr>
                <td><code>cmd/server/api/</code></td>
                <td>Transport layer: HTTP services, BUI frontend embed, tooling (docs generator, logfmt)</td>
              </tr>
              <tr>
                <td><code>cmd/server/app/</code></td>
                <td>Application layer: <code>domain/</code> (auth, mcp, model, etc. handlers) + <code>sdk/</code> (cache, mux, security wiring)</td>
              </tr>
              <tr>
                <td><code>cmd/server/foundation/</code></td>
                <td>Cross-cutting infra: <code>logger/</code>, <code>web/</code> (request lifecycle, tracing helpers)</td>
              </tr>
              <tr>
                <td><code>cmd/server/api/tooling/docs/</code></td>
                <td>Documentation generator (SDK godoc + CLI command tree → BUI markdown)</td>
              </tr>
              <tr>
                <td><code>sdk/kronk/</code></td>
                <td>Public SDK API surface (<code>init.go</code>, <code>chat.go</code>, <code>embedding.go</code>, <code>rerank.go</code>, <code>tokenize.go</code>, concurrency)</td>
              </tr>
              <tr>
                <td><code>sdk/kronk/model/</code></td>
                <td>Core inference and caching engine (batch, slots, IMC, sampler, prefill/decode)</td>
              </tr>
              <tr>
                <td><code>sdk/kronk/observ/</code></td>
                <td>Observability packages: <code>metrics/</code>, <code>otel/</code></td>
              </tr>
              <tr>
                <td><code>sdk/tools/</code></td>
                <td>CLI tooling support: <code>defaults/</code>, <code>devices/</code>, <code>downloader/</code>, <code>github/</code>, <code>libs/</code>, <code>models/</code></td>
              </tr>
              <tr>
                <td><code>examples/</code></td>
                <td>Standalone module (own <code>go.mod</code>) with runnable SDK examples — source for auto-generated example docs</td>
              </tr>
              <tr>
                <td><code>zarf/</code></td>
                <td>Deployment assets: <code>docker/</code>, <code>kms/</code> (model config samples), <code>nix/</code> (gomod2nix lock)</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Core Technology:</strong></p>
          <p>Kronk uses <a href="https://github.com/hybridgroup/yzma">yzma</a> (llama.cpp Go bindings) for local inference with GGUF models.</p>
          <h3 id="175-bui-frontend-development">17.5 BUI Frontend Development</h3>
          <p>The Browser UI is a React application located at:</p>
          <pre className="code-block"><code>{`cmd/server/api/frontends/bui/src/`}</code></pre>
          <p><strong>Directory Structure (&lt;code&gt;src/&lt;/code&gt;):</strong></p>
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
                <td><code>hooks/</code></td>
                <td>Reusable hooks (e.g. <code>useDraftSession.ts</code>)</td>
              </tr>
              <tr>
                <td><code>lib/</code></td>
                <td>Shared TypeScript utilities (<code>context.ts</code>, <code>format.ts</code>)</td>
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
                <td><code>main.tsx</code></td>
                <td>Vite entry point — mounts <code>&lt;App&gt;</code> into <code>index.html</code></td>
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
          <p>The BUI project root (<code>cmd/server/api/frontends/bui/</code>) also contains <code>index.html</code>, <code>package.json</code>, <code>vite.config.ts</code>, and <code>tsconfig.json</code>.</p>
          <p><strong>Build & Embed:</strong></p>
          <p>The BUI is a Vite + React + TypeScript app. The build output is <strong>embedded into the Go binary</strong> at compile time via <code>//go:embed static</code> in <a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/cmd/server/api/services/kronk/kronk.go#L37-L38"><code>cmd/server/api/services/kronk/kronk.go</code></a>. Editing source under <code>bui/src/</code> has no runtime effect until the bundle is rebuilt and the server is recompiled.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Workflow</th>
                <th>Command</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>HMR dev server</td>
                <td><code>npm run dev</code> (from <code>cmd/server/api/frontends/bui/</code>)</td>
              </tr>
              <tr>
                <td>Production build</td>
                <td><code>make bui-build</code> (or <code>npm run build</code>)</td>
              </tr>
              <tr>
                <td>Build + embed + run</td>
                <td><code>make kronk-server-build</code></td>
              </tr>
            </tbody>
          </table>
          <p><strong>Component Conventions:</strong></p>
          <p>Tooltip and form-label conventions are governed by <a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/cmd/server/api/frontends/bui/src/components/AGENTS.md"><code>bui/src/components/AGENTS.md</code></a>. All parameter explanations live in <code>PARAM_TOOLTIPS</code> and are accessed through the type-safe <code>TooltipKey</code>, <code>FieldLabel</code>, and <code>labelWithTip</code> helpers in <code>ParamTooltips.tsx</code>. Any new form field must add a <code>PARAM_TOOLTIPS</code> entry.</p>
          <p><strong>Routing:</strong></p>
          <p>Uses <code>react-router-dom</code> with <code>BrowserRouter</code>. Routes are defined in <code>routeMap</code> in <code>App.tsx</code>.</p>
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
            <li>Streaming support for pull operations (models and libraries)</li>
            <li>Auth-required endpoints accept token parameter</li>
          </ul>
          <p>The catalog is local and personal — there is no <code>catalog pull</code> stream.</p>
          <p><strong>Styling Conventions:</strong></p>
          <ul>
            <li>CSS variables defined in <code>:root</code> (colors: <code>--color-orange</code>, <code>--color-blue</code>, etc.)</li>
            <li>Common classes: <code>.card</code>, <code>.btn</code>, <code>.btn-primary</code>, <code>.form-group</code>, <code>.alert</code>, <code>.table-container</code></li>
            <li>No CSS modules or styled-components; use global CSS classes</li>
          </ul>
          <p><strong>Documentation Generation:</strong></p>
          <p>The single binary at <code>cmd/server/api/tooling/docs/main.go</code> runs three pipelines in order: SDK godoc → examples → manual chapters.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Pipeline</th>
                <th>Generator Location</th>
                <th>Source</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>SDK godoc</td>
                <td><code>cmd/server/api/tooling/docs/sdk/gofmt/</code></td>
                <td><code>go doc</code> output for <code>sdk/...</code></td>
              </tr>
              <tr>
                <td>Examples</td>
                <td><code>cmd/server/api/tooling/docs/sdk/examples/</code></td>
                <td><code>examples/</code> module</td>
              </tr>
              <tr>
                <td>Manual chapters</td>
                <td><code>cmd/server/api/tooling/docs/manual/</code></td>
                <td><code>.manual/chapter-*.md</code> files</td>
              </tr>
            </tbody>
          </table>
          <p>The manual chapters pipeline is what turns this very file into BUI React components — edits here flow into the BUI on the next <code>make kronk-docs</code>.</p>
          <p>Generate all documentation:</p>
          <pre className="code-block"><code className="language-shell">{`make kronk-docs
# equivalent to: go run cmd/server/api/tooling/docs/*.go`}</code></pre>
          <p>The generator takes no flags; it always rebuilds all three pipelines.</p>
          <h3 id="176-code-style-guidelines">17.6 Code Style Guidelines</h3>
          <p><strong>Package Comments:</strong></p>
          <pre className="code-block"><code className="language-go">{`// Package kronk provides the core inference API.`}</code></pre>
          <p><strong>Error Handling:</strong></p>
          <pre className="code-block"><code className="language-go">{`// Wrap errors with lowercase context prefix
return fmt.Errorf("loading model: %w", err)

// Declare package-level sentinel errors
var ErrModelNotFound = errors.New("model not found")`}</code></pre>
          <p><strong>Struct Design:</strong></p>
          <ul>
            <li>Consumer-facing structs (<code>Config</code>, <code>Result</code>, request/response DTOs) expose exported fields the caller fills in.</li>
            <li>Internal state structs keep fields unexported and surface behavior through methods on the exported type.</li>
            <li>Use the <code>Config</code> pattern for constructors.</li>
          </ul>
          <pre className="code-block"><code className="language-go">{`type Config struct {
    Host string
    Port int
}

func New(cfg Config) *Server {
    // ...
}`}</code></pre>
          <p><strong>Testing:</strong></p>
          <p>Tests link against yzma (llama.cpp Go bindings), so they require CGO and the installed libraries plus test models. Always go through <code>make test</code> (or <code>make test-only</code> for fast iteration) per §17.2. Never run with <code>CGO_ENABLED=0</code> — inference packages will fail to compile.</p>
          <p><strong>Post-edit Checks (per &lt;a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/AGENTS.md"&gt;AGENTS.md&lt;/a&gt;):</strong></p>
          <p>After modifying any <code>.go</code> file, run on the changed files / package:</p>
          <pre className="code-block"><code className="language-shell">{`gofmt -s -w <changed files>
go vet ./...
staticcheck ./...
go fix <changed package>`}</code></pre>
          <p>These are non-negotiable; they are also what <code>make test</code>'s <code>lint</code> step runs.</p>
          <p><strong>Comment Conventions:</strong></p>
          <ul>
            <li>Doc comments are full sentences ending with a period.</li>
            <li>Package doc lives on the file that declares the package's main type or entry point (e.g. <code>package kronk</code> doc on <code>kronk.go</code>).</li>
            <li>Use a block separator between logical sections in larger files:</li>
          </ul>
          <pre className="code-block"><code className="language-go">{`// =============================================================================
// Tiered Replace
// =============================================================================`}</code></pre>
          <p>See <a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/cmd/server/app/domain/mcpapp/fuzzyedit.go#L58-L60">fuzzyedit.go</a> for the canonical pattern.</p>
          <p><strong>Import Order:</strong></p>
          <p>Imports are grouped into three blocks separated by blank lines and sorted by <code>goimports -local github.com/ardanlabs/kronk</code>:</p>
          <ol>
            <li>Standard library</li>
            <li>External packages</li>
            <li>Internal packages (<code>github.com/ardanlabs/kronk/...</code>)</li>
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
          <h3 id="177-sdk-internals">17.7 SDK Internals</h3>
          <p>This section documents implementation details for developers working on the Kronk SDK packages.</p>
          <h4 id="1771-package-structure">17.7.1 Package Structure</h4>
          <p><strong>sdk/kronk/</strong> - Public SDK API surface:</p>
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
                <td>Model pool acquire/release (top-level wrapper)</td>
              </tr>
              <tr>
                <td><code>chat.go</code></td>
                <td>Chat completion API</td>
              </tr>
              <tr>
                <td><code>concurrency.go</code></td>
                <td>Generic streaming utilities, semaphore-based backpressure</td>
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
                <td>Main <code>Kronk</code> type, model pool management</td>
              </tr>
              <tr>
                <td><code>logger.go</code></td>
                <td>Streaming response logger (used when insecure-logging on)</td>
              </tr>
              <tr>
                <td><code>rerank.go</code></td>
                <td>Reranking API</td>
              </tr>
              <tr>
                <td><code>response.go</code></td>
                <td>OpenAI Responses API streaming</td>
              </tr>
              <tr>
                <td><code>tokenize.go</code></td>
                <td>Token-count API</td>
              </tr>
            </tbody>
          </table>
          <p><strong>sdk/kronk/model/</strong> - Low-level inference engine.</p>
          <p>The package is large (30+ files); files are grouped here by concern.</p>
          <p><em>Core types and lifecycle:</em></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>File</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>model.go</code></td>
                <td><code>Model</code> type, llama context management, lifecycle</td>
              </tr>
              <tr>
                <td><code>config.go</code></td>
                <td>Model configuration (GPU, cache, batching, YaRN, etc.)</td>
              </tr>
              <tr>
                <td><code>params.go</code></td>
                <td>Sampling parameters</td>
              </tr>
              <tr>
                <td><code>models.go</code></td>
                <td>OpenAI-compatible types (<code>ChatMessage</code>, <code>ToolCall</code>, etc.)</td>
              </tr>
              <tr>
                <td><code>chat.go</code></td>
                <td>Chat inference entry point, request validation, batch routing</td>
              </tr>
              <tr>
                <td><code>embed.go</code></td>
                <td>Embedding inference</td>
              </tr>
              <tr>
                <td><code>rerank.go</code></td>
                <td>Reranking inference</td>
              </tr>
              <tr>
                <td><code>prompts.go</code></td>
                <td>Jinja2 chat template application</td>
              </tr>
              <tr>
                <td><code>tokenize.go</code></td>
                <td>Token-count helper</td>
              </tr>
              <tr>
                <td><code>media.go</code></td>
                <td>Vision/audio media detection and conversion</td>
              </tr>
              <tr>
                <td><code>check.go</code></td>
                <td>Model file SHA validation</td>
              </tr>
              <tr>
                <td><code>logging.go</code></td>
                <td>Streaming response logger (mirrors final response for log)</td>
              </tr>
              <tr>
                <td><code>yzma.go</code></td>
                <td>Workarounds for yzma FFI issues not yet fixed upstream</td>
              </tr>
            </tbody>
          </table>
          <p><em>Batch engine</em> (<code>batch_*.go</code>):</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>File</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>batch_engine.go</code></td>
                <td><code>batchEngine</code> — parallel inference slots, request queue, wake loop</td>
              </tr>
              <tr>
                <td><code>batch_schedule.go</code></td>
                <td>Slot assignment (first-available for all sessions)</td>
              </tr>
              <tr>
                <td><code>batch_slot.go</code></td>
                <td><code>chatJob</code> and slot state structures</td>
              </tr>
              <tr>
                <td><code>batch_slot_start.go</code></td>
                <td>Slot init: KV restore from RAM, sampler build, prefix snapshot</td>
              </tr>
              <tr>
                <td><code>batch_decode.go</code></td>
                <td>MTMD batch decode helpers</td>
              </tr>
              <tr>
                <td><code>batch_prefill_text.go</code></td>
                <td>Round-robin token prefill across slots</td>
              </tr>
              <tr>
                <td><code>batch_prefill_media.go</code></td>
                <td>Vision/audio chunk prefill (interleaves embeddings with text)</td>
              </tr>
              <tr>
                <td><code>batch_tokens.go</code></td>
                <td>Per-token sampling and pipeline (logprobs, EOG, classify, stream)</td>
              </tr>
              <tr>
                <td><code>batch_utf8.go</code></td>
                <td>Partial-codepoint buffering across token boundaries</td>
              </tr>
              <tr>
                <td><code>batch_finish.go</code></td>
                <td>Request completion, metrics, KV cleanup per model type</td>
              </tr>
              <tr>
                <td><code>batch_errors.go</code></td>
                <td>Slot cancellation / shutdown error helpers</td>
              </tr>
              <tr>
                <td><code>batch_shutdown.go</code></td>
                <td>Drain active slots and pending jobs on shutdown</td>
              </tr>
              <tr>
                <td><code>batch_speculative.go</code></td>
                <td>Speculative decoding (draft model prefill + verify)</td>
              </tr>
            </tbody>
          </table>
          <p><em>Caching:</em></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>File</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>caching.go</code></td>
                <td>Cache orchestration and routing (<code>cacheResult</code>)</td>
              </tr>
              <tr>
                <td><code>caching_imc.go</code></td>
                <td>IMC session matching, two-tier hash scan, prefix trim/extend</td>
              </tr>
              <tr>
                <td><code>caching_imc_media.go</code></td>
                <td>IMC media cache build/extend (vision/audio)</td>
              </tr>
            </tbody>
          </table>
          <p><em>Sampling and grammar:</em></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>File</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>grammar.go</code></td>
                <td>JSON Schema → GBNF conversion for grammar-constrained output</td>
              </tr>
              <tr>
                <td><code>logprobs.go</code></td>
                <td>Top-k token log probability extraction</td>
              </tr>
              <tr>
                <td><code>speculative_sparse.go</code></td>
                <td>Sparse candidate sampling for speculative decode verification</td>
              </tr>
            </tbody>
          </table>
          <p><em>Content processors</em> (per-template tool/reasoning classification):</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>File</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>processor.go</code></td>
                <td>Processor interface, content classifier state machine</td>
              </tr>
              <tr>
                <td><code>processor_parse.go</code></td>
                <td>Router: dispatches to model-specific tool parser</td>
              </tr>
              <tr>
                <td><code>processor_gemma.go</code></td>
                <td>Gemma4-style tool calls</td>
              </tr>
              <tr>
                <td><code>processor_glm.go</code></td>
                <td>GLM <code>&lt;arg_key&gt;</code>/<code>&lt;arg_value&gt;</code> tag tool calls</td>
              </tr>
              <tr>
                <td><code>processor_gpt.go</code></td>
                <td>GPT-OSS Harmony tool calls</td>
              </tr>
              <tr>
                <td><code>processor_qwen.go</code></td>
                <td>Qwen3-Coder XML-like tool tags</td>
              </tr>
              <tr>
                <td><code>processor_mistral.go</code></td>
                <td>Mistral / Devstral tool calls</td>
              </tr>
              <tr>
                <td><code>processor_json.go</code></td>
                <td>Standard JSON tool calls</td>
              </tr>
            </tbody>
          </table>
          <p><em>Misc:</em></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>File</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>pool.go</code></td>
                <td><code>contextPool</code> — parallel llama contexts for embedding/rerank workloads</td>
              </tr>
            </tbody>
          </table>
          <h4 id="1772-streaming-architecture">17.7.2 Streaming Architecture</h4>
          <p><strong>Two streaming primitives</strong> (<code>concurrency.go</code>):</p>
          <ul>
            <li><code>streaming[T]</code> — 1:1 relay. Used by <code>ChatStreaming</code> to forward <code>model.ChatResponse</code> chunks straight to the caller.</li>
            <li><code>streamingWith[T, U]</code> — 1:N event transformation. Used by <code>ResponseStreaming</code> to fan out a single upstream chunk into multiple SSE event types.</li>
          </ul>
          <p>Both acquire the model on entry and release it from a <code>defer</code> when the user-facing channel closes (see §17.7.4) — the lifecycle is not hand-rolled in the per-API files.</p>
          <p><strong>Response Streaming Pattern</strong> (<code>response.go</code>, <code>concurrency.go</code>):</p>
          <ul>
            <li><code>streamProcessor</code> has three phases: <code>Start()</code>, <code>Process(chunk)</code>, <code>Complete(lastChunk)</code></li>
            <li>Phase flow: <code>Start</code> runs once before the upstream channel opens, <code>Process</code> runs once per upstream chunk, <code>Complete</code> runs once after the upstream channel closes and receives the last chunk seen.</li>
            <li><code>streamState</code> struct maintains response ID, sequence numbers, aggregated usage</li>
            <li>SSE format: <code>event: &lt;type&gt;\ndata: &lt;json&gt;\n\n</code></li>
          </ul>
          <p><strong>FinishReason Handling:</strong></p>
          <ul>
            <li><code>FinishReasonPtr *string</code> field with <code>FinishReason()</code> accessor</li>
            <li>Only three constants exist (<code>models.go</code>): <code>FinishReasonStop="stop"</code>, <code>FinishReasonTool="tool_calls"</code>, <code>FinishReasonError="error"</code>. There is no <code>"length"</code> — a <code>max_tokens</code> cap is reported as <code>FinishReasonStop</code>, unlike the OpenAI API.</li>
            <li>When <code>FinishReasonPtr != nil</code>, skip text/reasoning deltas (they duplicate previous content)</li>
            <li>Always process tool calls even with FinishReason set (may only arrive in final chunk)</li>
          </ul>
          <h4 id="1773-concurrency-strategy">17.7.3 Concurrency Strategy</h4>
          <p>All concurrent requests on a single <code>Kronk</code> block on one semaphore; its capacity is fixed at <code>New()</code> time and depends on the model class. <code>acquireModel()</code> is the gate (see §17.7.4).</p>
          <p><code>NSeqMax</code> is the <code>nseq-max</code> knob from <code>model_config.yaml</code>, and behaves differently depending on model type:</p>
          <p><strong>Embedding and Reranking Models</strong>:</p>
          <ul>
            <li><code>NSeqMax</code> controls the internal context pool size (see §17.7.6)</li>
            <li>Model weights are shared, only KV cache memory is multiplied</li>
            <li>Inputs within a request are partitioned across pool contexts for parallel processing</li>
            <li>Semaphore capacity = <code>NSeqMax</code></li>
          </ul>
          <p><strong>Text Inference Models</strong> (chat, completion, vision, audio):</p>
          <ul>
            <li><code>NSeqMax</code> controls batch parallelism within the batch engine — the number of concurrent slots (see §17.7.5)</li>
            <li>Only one <code>model.Model</code> instance is created with multiple slots</li>
            <li>Semaphore capacity = <code>NSeqMax * queueDepth</code> (default <code>queueDepth=2</code>)</li>
            <li>Why ×2: with <code>queueDepth=2</code>, one request can sit on the batch engine's request queue while another is in prefill/decode, smoothing throughput across acquire → prefill → decode → release. Increase to absorb bursty load; decrease to bound queued memory.</li>
          </ul>
          <p><strong>Detection Logic</strong> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/kronk.go#L63-L83">kronk.go:63-83</a>):</p>
          <pre className="code-block"><code className="language-go">{`queueDepth := cfg.QueueDepth()
if queueDepth == 0 {
    queueDepth = 2
}

var semCapacity int

switch {
case mi.IsEmbedModel || mi.IsRerankModel:
    semCapacity = max(cfg.NSeqMax(), 1)
default:
    semCapacity = max(cfg.NSeqMax(), 1) * queueDepth
}`}</code></pre>
          <h4 id="1774-model-acquirerelease-cleanup">17.7.4 Model Acquire/Release &amp; Cleanup</h4>
          <p>The wrappers in <code>concurrency.go</code> (<code>streaming</code> / <code>streamingWith</code>, see §17.7.2) are what call <code>acquireModel</code> and <code>releaseModel</code> — per-API files do not call them directly.</p>
          <p><strong>Acquisition</strong> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/acquire.go">acquire.go</a>):</p>
          <ol>
            <li><strong>Shutdown check</strong>: take <code>shutdown.Lock()</code>; if <code>shutdownFlag</code> is set, return <code>"acquire-model: kronk has been unloaded"</code>.</li>
            <li><strong>Active stream accounting</strong>: increment the <code>activeStreams</code> atomic counter while still holding the shutdown lock (the unload path waits on this counter to drain).</li>
            <li><strong>Backpressure slot</strong>: block on the semaphore (<code>krn.sem &lt;- struct&#123;&#125;&#123;&#125;</code>), respecting <code>ctx.Done()</code> — on context cancellation, decrement <code>activeStreams</code> and return <code>ctx.Err()</code>.</li>
            <li><strong>Return model</strong>: return the single <code>*model.Model</code> (<code>krn.model</code>).</li>
          </ol>
          <p><strong>Release</strong> (<code>acquire.go</code>):</p>
          <ol>
            <li>Drain a slot from the semaphore (<code>&lt;-krn.sem</code>).</li>
            <li>Decrement <code>activeStreams</code>.</li>
          </ol>
          <p><strong>Cleanup Flow:</strong></p>
          <p>The KV-cache cleanup path depends on whether the request goes through the batch engine. The decision is captured by the local <code>batching</code> flag in <a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/chat.go">chat.go</a>.</p>
          <p><em>Batched path</em> (text inference via IMC — the normal route for chat):</p>
          <ul>
            <li>The <code>m.resetContext()</code> defer in <code>chat.go</code> is gated on <code>!batching</code> and is skipped.</li>
            <li>Per-slot KV cleanup happens inside the batch engine in <a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_finish.go">batch_finish.go</a>, which clears each slot's sequence and frees per-request resources.</li>
            <li><code>releaseModel()</code> then runs from the wrapper <code>defer</code> in <code>concurrency.go</code> after the user-facing channel closes.</li>
          </ul>
          <p><em>Non-batched path</em> (e.g. some embed/rerank entrypoints, non-IMC media flows):</p>
          <ul>
            <li><code>chat.go</code> registers <code>defer m.resetContext()</code> after <code>validateAndCloneDocument</code> and <code>prepareContext</code> succeed (not "before any processing").</li>
            <li><code>resetContext()</code> calls <code>llama.Synchronize(m.lctx)</code> then <code>llama.MemoryClear(mem, true)</code> for each model memory.</li>
            <li><code>releaseModel()</code> runs after that, from the wrapper <code>defer</code>.</li>
          </ul>
          <p><strong>Key invariant:</strong> the semaphore guarantees the model is never released while a request is in flight — <code>releaseModel()</code> is only called from the streaming wrapper's <code>defer</code>, which fires after the user-facing channel closes.</p>
          <h4 id="1775-batch-engine-internals">17.7.5 Batch Engine Internals</h4>
          <p><strong>ChatStreaming Decision Logic</strong> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/chat.go#L258-L315">chat.go:258-315</a>):</p>
          <p>All chat requests — text and media (<code>ObjectChatText</code> and <code>ObjectChatMedia</code>) — flow through <code>submitToBatchEngine</code>. It builds a <code>chatJob</code> (carrying request data plus the resolved IMC fields from <code>cacheResult</code>) and unconditionally calls <code>m.batch.submit(&job)</code>. Returns:</p>
          <ul>
            <li><code>true</code> on successful submit; the caller sets <code>batching = true</code> in <code>chat.go</code> so the non-batched cleanup defer is skipped (see §17.7.4).</li>
            <li><code>false</code> only on submit error. The error has already been streamed to the caller via <code>sendChatError</code> and any IMC pending reservation cleared.</li>
          </ul>
          <p>There is no longer an <code>m.batch == nil || object != ObjectChatText</code> early return — vision/audio also runs through the batch engine.</p>
          <p><strong>Batch Engine Architecture</strong> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_engine.go">batch_engine.go</a>):</p>
          <ul>
            <li><code>batchEngine</code> manages <code>nSlots</code> parallel <code>*slot</code> structs (slot type lives in <a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot.go">batch_slot.go</a>)</li>
            <li>Constructed via <code>newBatchEngine(m, nSlots)</code> during model setup; <code>nSlots = NSeqMax</code> (see §17.7.3)</li>
            <li>Queueing: <code>requestQ chan <em>chatJob&lt;/code&gt; (buffered &lt;code&gt;nSlots</em>2</code>) is the public inbox; <code>pendingJobs</code> holds jobs already dequeued but unable to start because no slot was free, and is checked before reading <code>requestQ</code> again</li>
            <li>Each slot tracks: <code>seqID</code>, prompt tokens, decode state, sampler, response channel, logprobs, prefill state</li>
            <li>Signal-based wake: <code>wakeCh chan struct&#123;&#125;</code> (buffered size 1) is poked on every successful submit, eliminating up-to-1ms scheduling latency on request pickup</li>
            <li>Polling intervals when no wake signal arrives: 100µs (active slots generating), 5ms (idle, no active slots)</li>
          </ul>
          <p><strong>Slots, Sequences, and Sessions:</strong></p>
          <ul>
            <li><code>slot.id</code> = slot index (batch-engine execution lane)</li>
            <li><code>slot.seqID</code> = llama.cpp sequence ID (KV cache partition for the active slot)</li>
            <li><code>slot.seqIDs</code> = pre-allocated slice for efficient <code>batchAdd</code> calls</li>
            <li><code>imcSession</code> = logical cached conversation branch (hash, tokens, KV state)</li>
          </ul>
          <p>Sequences are isolated partitions in the shared KV cache memory. Slot seqIDs always start at 0. IMC sessions are decoupled from slots: session state is externalized to RAM after each request and restored into any available slot on the next request via <code>StateSeqSetData</code>. <code>StateSeqGetData</code> captures raw KV bytes regardless of whether they originated from text tokens or media embeddings. Full IMC lifecycle is detailed in §17.7.7.</p>
          <h4 id="1776-context-pooling">17.7.6 Context Pooling</h4>
          <p>Kronk uses two distinct context strategies depending on the workload.</p>
          <p><strong>Text inference: single shared context.</strong></p>
          <ul>
            <li>One <code>llama.Context</code> is created in <code>NewModel</code> and reused across requests.</li>
            <li>KV cleanup splits by path (see §17.7.4):</li>
          </ul>
          <p>- Non-batched path → <code>resetContext()</code> runs <code>llama.Synchronize(m.lctx)</code> then <code>llama.MemoryClear(mem, true)</code>. - Batched path (text/IMC) → per-slot cleanup happens in <a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_finish.go">batch_finish.go</a>; <code>resetContext()</code> is skipped.</p>
          <ul>
            <li>Reusing a single context avoids GPU memory fragmentation on all backends (CUDA, Metal, Vulkan, ROCm) caused by repeated context alloc/free.</li>
          </ul>
          <p><strong>Embedding & rerank: &lt;code&gt;contextPool&lt;/code&gt;</strong> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/pool.go">pool.go</a>):</p>
          <ul>
            <li><code>newContextPool(model, ctxParams, log, n)</code> creates <code>n = NSeqMax</code> parallel <code>llama.Context</code> instances. All share the same <code>llama.Model</code> (weights), so only KV cache memory is multiplied per context.</li>
            <li>Available context indices are tracked via a buffered <code>avail chan int</code>; callers acquire by receiving from the channel and release by sending the index back.</li>
            <li>Inputs within a single embed/rerank request are partitioned across pool contexts for parallel processing — this is the concurrency semantic documented in §17.7.3 for embedding/reranking models.</li>
          </ul>
          <h4 id="1777-imc-implementation-details">17.7.7 IMC Implementation Details</h4>
          <p><strong>Key Functions:</strong></p>
          <p>The four entry points an agent will grep for live in <a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/caching_imc.go">caching_imc.go</a> and <a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/caching_imc_media.go">caching_imc_media.go</a>:</p>
          <ul>
            <li><code>processIMC</code> — session selection / strategy dispatch</li>
            <li><code>extendIMCCache</code> — append new messages to a matched session</li>
            <li><code>buildIMCCacheFromScratch</code> — fresh build (no usable prefix)</li>
            <li><code>rebuildIMCFromPartialPrefix</code> — salvage a token-prefix overlap</li>
          </ul>
          <p><strong>Critical Implementation Details:</strong></p>
          <ol>
            <li><strong>Extension tokenization must use &lt;code&gt;special=true&lt;/code&gt;</strong>: <code>llama.Tokenize(m.vocab, extension, m.addBOSToken, true)</code> — the <code>true</code> in the 4th arg ensures ChatML tokens like <code>&lt;|im_start|&gt;</code> are recognized. The <code>addBOS</code> arg uses the model's <code>addBOSToken</code> setting, not a hardcoded value.</li>
            <li><strong>Prefix mismatch detection</strong>: Use <code>strings.HasPrefix(fullPrompt, prefixPrompt)</code> to detect Jinja template nondeterminism.</li>
            <li><strong>&lt;code&gt;add_generation_prompt=false&lt;/code&gt; for cached prefixes</strong>: Creates valid prefix for extension. Generation prompt added only for final suffix.</li>
          </ol>
          <p><strong>IMC Algorithm — 5 strategies</strong> (per <a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/caching_imc.go#L54-L68">caching_imc.go:54-68</a>):</p>
          <p><code>processIMC</code> snapshots all sessions and picks one of:</p>
          <ol>
            <li><strong>Pure cache hit</strong> — <code>cachedMsgCount == len(messages)-1</code>. Nothing to decode beyond the suffix; KV state is already correct.</li>
            <li><strong>Hash-prefix extend</strong> — a session's <code>cachedMsgsHash</code> matches the prefix hash of the incoming messages → extend with <code>messages[cachedMsgCount : len-1]</code>.</li>
            <li><strong>System-prompt preserve</strong> — only the system prompt hash matches. The sys prompt KV is preserved; the conversation body is rebuilt fresh on top of it.</li>
            <li><strong>Token-prefix fallback</strong> — no hash match, but a session's <code>cachedTokens</code> shares a leading run with the incoming prompt's tokens. Trim to the common prefix, rebuild the rest (<code>rebuildIMCFromPartialPrefix</code>).</li>
            <li><strong>Rebuild from scratch</strong> — no usable overlap. Pick an empty session, or evict the LRU session by <code>lastUsed</code>, and call <code>buildIMCCacheFromScratch</code>.</li>
          </ol>
          <p><strong>Multi-user IMC:</strong></p>
          <p>Each of the <code>NSeqMax</code> sessions is an independent conversation branch. Concurrent users and sub-agents land in different sessions via hash matching, so they don't trample each other's caches. When all sessions are full, the LRU session is evicted on <code>lastUsed</code>.</p>
          <p><strong>Text vs Media IMC:</strong></p>
          <ul>
            <li><strong>Text sessions</strong> externalize KV to RAM via <code>StateSeqGetData</code> after each request and restore into any free slot via <code>StateSeqSetData</code> on the next request. Sessions migrate freely between slots.</li>
            <li><strong>Media sessions</strong> (vision/audio) stay <strong>slot-dedicated</strong>: image/audio embeddings cannot be externalized through <code>StateSeqGetData/SetData</code>, so the session is bound to a fixed slot for its lifetime. Media-specific build/extend logic lives in <a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/caching_imc_media.go">caching_imc_media.go</a>; the <code>hasMedia</code>, <code>useMRoPE</code>, and <code>mediaKVCounts</code> fields track media state.</li>
          </ul>
          <p><strong>IMC Lifecycle (All Sessions):</strong></p>
          <ol>
            <li><code>processIMC()</code> scans <strong>sessions</strong> (not slots) for a hash match</li>
            <li><code>fillSlots()</code> assigns the job to the <strong>first available slot</strong></li>
            <li><code>startSlot()</code> restores cached KV from RAM via <code>StateSeqSetData</code></li>
            <li>Cache is extended/rebuilt as needed, then snapshotted back to RAM via <code>StateSeqGetData</code></li>
            <li>Suffix tokens are decoded and generation runs</li>
            <li><code>finishSlot()</code> clears the full VRAM sequence (cached prefix already lives in RAM)</li>
          </ol>
          <p><strong>IMC Session State</strong> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/model.go#L39-L55">model.go:39-55</a>):</p>
          <pre className="code-block"><code className="language-go">{`type imcSession struct {
    slotID            int           // Slot index (transitional: removed in Phase 4)
    seqID             llama.SeqId   // KV cache sequence ID (transitional: removed in Phase 4)
    cachedMsgsHash    string        // Hash of all cached messages
    cachedTokens      []llama.Token // Full token sequence in KV cache
    totalTokensCached int           // Total KV positions cached
    cachedMsgCount    int           // Number of messages cached
    kvState           []byte        // Externalized KV state (RAM buffer)
    kvStateBytes      int           // Size of kvState in bytes
    lastUsed          time.Time     // Last access time (for LRU eviction)
    pending           bool          // True when build/extend in-flight (transitional: removed in Phase 4)
    hasMedia          bool          // True if cached content includes media
    useMRoPE          bool          // True if cached media used M-RoPE
    mediaKVCounts     []int         // KV positions per media chunk
    sysPromptHash     string        // Hash of system prompt message
    sysPromptTokens   int           // Token count of system prompt
}`}</code></pre>
          <p>The fields marked <code>transitional</code> (<code>slotID</code>, <code>seqID</code>, <code>pending</code>) are scheduled for removal in an upcoming Phase 4 refactor that fully decouples sessions from slots.</p>
          <h4 id="1778-tool-call-internals">17.7.8 Tool Call Internals</h4>
          <p><strong>Processor state machine</strong> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/processor.go">processor.go</a>):</p>
          <p>A per-slot <code>processor</code> classifies streaming output token-by-token into one of four statuses (<code>processor.go:10-15</code>):</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Constant</th>
                <th>Value</th>
                <th>Meaning</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>statusNone</code></td>
                <td>0</td>
                <td>Initial / between segments</td>
              </tr>
              <tr>
                <td><code>statusReasoning</code></td>
                <td>1</td>
                <td>Inside <code>&lt;think&gt;</code> (or model equivalent)</td>
              </tr>
              <tr>
                <td><code>statusCompletion</code></td>
                <td>2</td>
                <td>Regular response text</td>
              </tr>
              <tr>
                <td><code>statusTooling</code></td>
                <td>3</td>
                <td>Inside a tool call</td>
              </tr>
            </tbody>
          </table>
          <p>While in <code>statusTooling</code>, tokens are appended to <code>toolCallBuf</code> and not streamed to the caller as completion text. When the tool-call segment closes, the buffered content is handed to <code>parseToolCall</code> for structured extraction.</p>
          <p><strong>Tool call format dispatch</strong> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/processor_parse.go">processor_parse.go</a>):</p>
          <p><code>parseToolCall</code> routes accumulated tool-call content to a model-specific parser based on its format:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>File</th>
                <th>Format</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>processor_json.go</code></td>
                <td>Standard JSON <code>&#123;"name":..., "arguments":...&#125;</code></td>
              </tr>
              <tr>
                <td><code>processor_qwen.go</code></td>
                <td>Qwen3-Coder XML-like <code>&lt;tool_call&gt;</code> / <code>&lt;function=...&gt;</code> tags</td>
              </tr>
              <tr>
                <td><code>processor_gpt.go</code></td>
                <td>GPT-OSS Harmony (`&lt;\</td>
                <td>channel\</td>
                <td>&gt;commentary to=NAME&lt;\</td>
                <td>constrain\</td>
                <td>&gt;...`)</td>
              </tr>
              <tr>
                <td><code>processor_glm.go</code></td>
                <td>GLM <code>&lt;arg_key&gt;</code> / <code>&lt;arg_value&gt;</code> pairs</td>
              </tr>
              <tr>
                <td><code>processor_gemma.go</code></td>
                <td>Gemma4 tool calls</td>
              </tr>
              <tr>
                <td><code>processor_mistral.go</code></td>
                <td>Mistral / Devstral tool calls</td>
              </tr>
            </tbody>
          </table>
          <p><strong>Split-token tag handling:</strong></p>
          <p>Some models (Qwen3-Coder variants) emit a bare <code>&lt;function=...&gt;</code> without the <code>&lt;tool_call&gt;</code> wrapper, and the tag itself can be tokenized across multiple tokens (e.g. <code>&lt;</code>, <code>function</code>, <code>=</code>). The processor uses <code>pendingTagBuf</code> / <code>inPendingTag</code> to accumulate fragments until the tag is complete or disproven.</p>
          <p><strong>GPT-OSS Harmony channel handling:</strong></p>
          <p>For GPT-OSS, the processor accumulates the channel name (<code>channelBuf</code>) and watches for <code>&lt;|constrain|&gt;</code> (<code>awaitingConstrain</code>). The function name is extracted from <code>to=NAME</code> in the channel and stored in <code>toolFuncName</code> for later assembly into a structured <code>ResponseToolCall</code>.</p>
          <p><strong>Tool call ID:</strong></p>
          <p>IDs are generated by <code>newToolCallID()</code> as <code>"call_" + uuid.NewString()</code> — stable contract for the OpenAI-compatible response wire format.</p>
          <p><strong>chatMessage unmarshaling</strong> (<code>models.go</code>):</p>
          <ul>
            <li><code>Content</code> can be <code>nil</code> for assistant messages with <code>tool_calls</code>.</li>
            <li>Handle <code>len(app.Content) == 0 || string(app.Content) == "null"</code> as valid empty content.</li>
          </ul>
          <p><strong>ToolCallArguments type:</strong></p>
          <ul>
            <li>Custom type that marshals to a JSON string (OpenAI spec).</li>
            <li>Unmarshals from either a string or an object for non-compliant clients.</li>
          </ul>
          <h4 id="1779-logprobs-implementation">17.7.9 Logprobs Implementation</h4>
          <p><strong>Implementation</strong> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/logprobs.go">logprobs.go</a>):</p>
          <ul>
            <li><code>extractLogprobs(lctx, vocab, sampledToken, iBatch, topK, buf)</code>: retrieves logits via <code>llama.GetLogitsIth(lctx, iBatch, nVocab)</code> and converts them to log probabilities. <code>iBatch</code> identifies which slot's logits row to read when multiple slots are batched in one forward pass (see §17.7.5). <code>buf</code> is a pre-allocated byte buffer reused across tokens to avoid per-token allocations during top-K decoding.</li>
            <li><code>logSoftmax()</code>: numerically stable log-softmax using the log-sum-exp trick.</li>
            <li><code>getTopKLogprobs()</code>: <code>container/heap</code> min-heap for O(n log k) top-k extraction.</li>
          </ul>
          <p><strong>Gating:</strong></p>
          <p>The extraction path runs only when <code>params.TopLogprobs &gt; 0</code>. With logprobs disabled, this work is skipped entirely on the hot path.</p>
          <p><strong>Critical ordering — extract before &lt;code&gt;llama.SamplerAccept&lt;/code&gt;</strong> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_tokens.go#L38-L57">batch_tokens.go:38, 57</a>):</p>
          <p><code>SamplerAccept</code> mutates sampler state (repetition history, penalty buffers, dry/xtc state). Reading logits after acceptance would no longer reflect the probability landscape for the token we're trying to score.</p>
          <p>This corresponds to step 12.1 of the request flow in §17.11.</p>
          <h3 id="178-responses-api-normalization">17.8 Responses API Normalization</h3>
          <p>The SDK's <a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/response.go">response.go</a> exposes the OpenAI Responses API on top of the same <code>model.Chat</code> engine that serves Chat Completions. To do that, the input document is normalized into a Chat-Completions-style <code>messages</code> array before <code>model.Chat</code> / <code>model.ChatStreaming</code> is invoked.</p>
          <p><strong>Owner</strong>: the SDK methods <code>Response</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/response.go#L140">line 140</a>) and <code>ResponseStreaming</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/response.go#L163">line 163</a>) call <code>convertInputToMessages(d)</code> themselves as the first step. Callers (CLI, MCP, server handlers) do not need to invoke it.</p>
          <p><strong>Why it exists</strong>: lets the SDK accept both the Chat-style <code>messages</code> payload and the Responses-style <code>input</code> payload (string, message list, or function-call/output items).</p>
          <p><strong>Normalization helpers</strong> (all in <code>response.go</code>):</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Function</th>
                <th>Responsibility</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>convertInputToMessages</code></td>
                <td>Top-level entrypoint; orchestrates the helpers below</td>
              </tr>
              <tr>
                <td><code>inputToMessages</code></td>
                <td>Turns the <code>input</code> field (string / messages / items) into <code>[]model.D</code></td>
              </tr>
              <tr>
                <td><code>normalizeResponsesItems</code></td>
                <td>Maps Responses items (<code>function_call</code>, <code>function_call_output</code>, …) to Chat messages; groups consecutive <code>function_call</code>s into one assistant message with multiple <code>tool_calls</code></td>
              </tr>
              <tr>
                <td><code>normalizeResponsesContent</code></td>
                <td>Translates Responses content parts (<code>input_text</code>, <code>output_text</code>, …) inside existing messages into Chat-format content</td>
              </tr>
              <tr>
                <td><code>normalizeTools</code></td>
                <td>Converts Responses' flat tool definitions into the Chat-Completions <code>&#123; "function": &#123;...&#125; &#125;</code> shape</td>
              </tr>
              <tr>
                <td><code>injectInstructions</code></td>
                <td>Promotes the Responses <code>instructions</code> field into a leading <code>role:"system"</code> message</td>
              </tr>
              <tr>
                <td><code>extractInputParams</code></td>
                <td>Pulls Responses-only parameters (e.g. <code>Instructions</code>) into <code>inputParams</code> for downstream handling</td>
              </tr>
              <tr>
                <td><code>extractTools</code></td>
                <td>Reads the (already-normalized) tool list out of the document</td>
              </tr>
            </tbody>
          </table>
          <h3 id="179-goroutine-budget">17.9 Goroutine Budget</h3>
          <p>A running Kronk server typically shows ~25 baseline goroutines before any requests arrive. When requests are active, expect roughly 3-5 additional goroutines per in-flight request. For example, 3 concurrent requests for the same model will show ~40 goroutines total. This is normal.</p>
          <p><strong>Baseline goroutines (~25, always running):</strong></p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Source</th>
                <th>Goroutines</th>
                <th>Location</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Go runtime (GC, finalizer, netpoller, etc.)</td>
                <td>~4-6</td>
                <td>runtime internals</td>
              </tr>
              <tr>
                <td>API <code>http.Server</code> (listener + idle conns)</td>
                <td>~3</td>
                <td><code>cmd/server/api/services/kronk/kronk.go</code></td>
              </tr>
              <tr>
                <td>Debug <code>http.Server</code> (pprof, metrics, statsviz)</td>
                <td>~3</td>
                <td><code>cmd/server/api/services/kronk/kronk.go</code></td>
              </tr>
              <tr>
                <td><code>statsviz.Register</code> (websocket handler)</td>
                <td>~2</td>
                <td><code>cmd/server/app/sdk/debug/debug.go</code></td>
              </tr>
              <tr>
                <td>gRPC auth server (<code>gs.Serve</code>)</td>
                <td>~2-3</td>
                <td><code>cmd/server/app/domain/authapp/start.go</code></td>
              </tr>
              <tr>
                <td>Embedded MCP <code>http.Server</code> (listener + conns)</td>
                <td>~2-3</td>
                <td><code>cmd/server/app/domain/mcpapp/start.go</code></td>
              </tr>
              <tr>
                <td>OTEL background collector probe</td>
                <td>1</td>
                <td><code>sdk/kronk/observ/otel/otel.go</code></td>
              </tr>
              <tr>
                <td><code>otelhttp.NewHandler</code> internals</td>
                <td>~1-2</td>
                <td><code>cmd/server/foundation/web/web.go</code></td>
              </tr>
              <tr>
                <td>Batch engine <code>processLoop</code></td>
                <td>1</td>
                <td><code>sdk/kronk/model/batch_engine.go</code></td>
              </tr>
            </tbody>
          </table>
          <p>The baseline assumes embedded auth and embedded MCP — the defaults when <code>KRONK_AUTH_HOST</code> and <code>KRONK_MCP_HOST</code> are unset. Pointing either at an external service removes its row from the table.</p>
          <p><strong>Per-request goroutines (~3-5 each):</strong></p>
          <p>The floor is three: the <code>http.Server</code> connection handler, the SDK wrapper goroutine (<code>streaming</code> or <code>streamingWith</code>), and the <code>ChatStreaming</code>/<code>ResponseStreaming</code> request goroutine. <code>wrapChannelForLogging</code> adds one more when <code>InsecureLogging</code> is enabled.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Source</th>
                <th>Location</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>http.Server</code> connection handler</td>
                <td>Go stdlib</td>
              </tr>
              <tr>
                <td><code>ChatStreaming</code> request goroutine</td>
                <td><code>sdk/kronk/model/chat.go</code></td>
              </tr>
              <tr>
                <td><code>streaming()</code> wrapper goroutine (Chat / Embedding paths)</td>
                <td><code>sdk/kronk/concurrency.go</code></td>
              </tr>
              <tr>
                <td><code>streamingWith()</code> wrapper goroutine (ResponseStreaming path)</td>
                <td><code>sdk/kronk/concurrency.go</code></td>
              </tr>
              <tr>
                <td><code>wrapChannelForLogging</code> (only when <code>InsecureLogging</code> is on)</td>
                <td><code>sdk/kronk/model/chat.go</code></td>
              </tr>
            </tbody>
          </table>
          <p>The goroutine metric is a point-in-time snapshot from <code>runtime.NumGoroutine()</code> captured every 10th request by the metrics middleware. It includes everything in the process, including Go runtime internals. After active requests complete, the count drops back to the baseline.</p>
          <h3 id="1710-request-tracing-spans">17.10 Request Tracing Spans</h3>
          <p>Each chat completion request produces the following trace hierarchy. <code>prepare-request</code>, <code>queue-wait</code>, and <code>process-request</code> are sibling spans under the request's root context — none is a child of another.</p>
          <pre className="code-block"><code>{`POST /v1/chat/completions
├── prepare-request                          Validation, caching, prompt creation
│   ├── process-cache                        Cache lookup/update (IMC, when enabled)
│   │   ├── cache-tokenize-imc-prefix-match  Token-prefix fallback (§17.7.7 strategy 4)
│   │   ├── cache-tokenize-imc-extend        Hash-prefix extend (strategy 2)
│   │   ├── cache-tokenize-imc-sysprompt-preserve  System-prompt preserve (strategy 3)
│   │   ├── cache-tokenize-imc-scratch       Rebuild from scratch (strategy 5)
│   │   ├── cache-tokenize-imc-media-text-extend  Media-IMC text extend
│   │   └── cache-decode                     KV-fill decode for build/extend
│   └── create-prompt                        Jinja template application
│
├── queue-wait                               Job sits in requestQ until a slot picks it up
│
└── process-request                          Batch engine slot processing
    ├── prefill                              Tokenize + KV fill (ends at first token)
    └── token-generation                     Decode loop producing output tokens`}</code></pre>
          <p><strong>Phase 1: prepare-request</strong> runs in the <code>ChatStreaming</code> request goroutine. It validates the document, processes the IMC cache, and creates the prompt via the Jinja template. When caching is enabled, <code>process-cache</code> and its <code>cache-tokenize-imc-*</code> and <code>cache-decode</code> children appear here. Only the tokenize variant matching the strategy chosen by <code>processIMC</code> is emitted on any given request.</p>
          <p><strong>queue-wait</strong> is its own top-level span (not an attribute). It is started at the very end of <code>prepare-request</code> in the request goroutine and ended by <code>startSlot</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot_start.go#L36-L39">batch_slot_start.go:36-39</a>) when the batch engine picks the job up off <code>requestQ</code>. Its duration is the gap between <code>prepare-request</code> ending and <code>process-request</code> starting.</p>
          <p><strong>Phase 2: process-request</strong> runs in the batch engine's <code>processLoop</code> goroutine. The <code>prefill</code> span covers tokenization and KV cache filling. Time to first token (TTFT) is measured from prefill start to the first output token. The <code>token-generation</code> span covers the decode loop that produces output tokens.</p>
          <p>Additional spans that may appear at the top level:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Span</th>
                <th>When</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>model-file-load-time</code></td>
                <td>First request for a model</td>
                <td>Loading the GGUF model file</td>
              </tr>
              <tr>
                <td><code>proj-file-load-time</code></td>
                <td>Vision/audio requests</td>
                <td>Loading the multimodal projection file</td>
              </tr>
              <tr>
                <td><code>imc-media-cache-build</code></td>
                <td>Vision/audio IMC builds</td>
                <td>Media-IMC cache build (separate from the text path)</td>
              </tr>
            </tbody>
          </table>
          <h3 id="1711-inference-code-path">17.11 Inference Code Path</h3>
          <p>This section traces a <code>ChatStreaming</code> request end-to-end. Each step has a high-level description followed by a <strong>Code:</strong> sub-block listing the function calls and file locations the agent will navigate.</p>
          <h4 id="step-1-receive-the-request">Step 1: Receive the Request</h4>
          <p>The caller provides a document containing messages and sampling parameters. The SDK validates that the request's context has a deadline to prevent unbounded processing.</p>
          <p><strong>Code:</strong></p>
          <ul>
            <li><code>Kronk.ChatStreaming</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/chat.go">sdk/kronk/chat.go</a>) — validates the context deadline and wraps <code>Model.ChatStreaming</code> in a closure.</li>
          </ul>
          <h4 id="step-2-acquire-the-model">Step 2: Acquire the Model</h4>
          <p>The kronk-level semaphore controls how many requests can be in-flight at once. The request blocks here until a slot opens up, providing backpressure when the system is under load. See §17.7.4 for the full acquire/release contract.</p>
          <p><strong>Code:</strong></p>
          <ul>
            <li><code>streaming()</code> / <code>streamingWith()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/concurrency.go">sdk/kronk/concurrency.go</a>) calls <code>acquireModel()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/acquire.go">sdk/kronk/acquire.go</a>) — checks the shutdown flag, increments the kronk-level <code>krn.activeStreams</code>, and blocks on the semaphore (with <code>ctx.Done</code> cancellation).</li>
            <li>The wrapper goroutine <code>defer</code>s <code>releaseModel()</code> and <code>close(ch)</code>.</li>
          </ul>
          <h4 id="step-3-validate-the-document">Step 3: Validate the Document</h4>
          <p>The request document is validated to ensure it contains properly structured messages. Sampling parameters (temperature, top_p, top_k, min_p, max_tokens, grammar, etc.) are extracted and resolved against model defaults. The document is shallow-cloned so downstream processing can modify it without affecting the caller.</p>
          <p><strong>Code:</strong></p>
          <ul>
            <li><code>Model.ChatStreaming</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/chat.go">model/chat.go</a>) creates the response channel, wraps it with <code>wrapChannelForLogging</code> if <code>InsecureLogging</code> is on, increments the <strong>model-level</strong> <code>m.activeStreams</code> (a separate counter from the kronk-level one in Step 2 — both are waited on independently during unload), and spawns the request goroutine with the <code>prepare-request</code> span (§17.10).</li>
            <li><code>validateAndCloneDocument()</code> (<code>model/chat.go</code>) — validates the <code>messages</code> field, calls <code>parseParams()</code> to extract sampling parameters, shallow-clones the document.</li>
          </ul>
          <h4 id="step-4-prepare-the-context">Step 4: Prepare the Context</h4>
          <p>The system determines whether this is a text-only or media (vision/audio) request:</p>
          <ul>
            <li><strong>Text</strong>: multi-part content arrays are flattened into plain strings.</li>
            <li><strong>Media</strong>: the projection model is loaded; media content (images or audio) is detected and converted into raw bytes for the encoder pipeline.</li>
          </ul>
          <p><strong>Code:</strong></p>
          <ul>
            <li><code>prepareContext()</code> (<code>model/chat.go</code>) returns <code>ObjectChatText</code> or <code>ObjectChatMedia</code>.</li>
          </ul>
          <p>- Text path: <code>prepareTextContext()</code>. - Media path: <code>prepareMediaContext()</code> — loads the projection file via <code>mtmd.InitFromFile()</code> and converts the OpenAI media format to byte slices.</p>
          <h4 id="step-5-process-the-cache">Step 5: Process the Cache</h4>
          <p>If caching is enabled, the system checks whether any portion of the conversation is already in the KV cache to avoid redundant computation. The IMC algorithm picks one of <strong>5 strategies</strong> — see §17.7.7 for the full decision tree (pure cache hit, hash-prefix extend, system-prompt preserve, token-prefix fallback, or rebuild from scratch with LRU eviction).</p>
          <p>Tool response messages are also enriched with their originating function names so templates can render tool results correctly.</p>
          <p><strong>Code:</strong></p>
          <ul>
            <li><code>prepareCacheAndPrompt()</code> (<code>model/chat.go</code>):</li>
          </ul>
          <p>- <code>injectToolResponseNames()</code> — adds <code>name</code>/<code>tool_call_name</code> to <code>role:"tool"</code> messages by matching <code>tool_call_id</code>. - <code>processCache()</code> (<code>model/caching.go</code>) → <code>processIMC()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/caching_imc.go">model/caching_imc.go</a>) — runs the §17.7.7 5-strategy selection across all <code>NSeqMax</code> sessions, tokenizes any extension tokens, and sets the <code>pending</code> flag on the chosen session.</p>
          <h4 id="step-6-apply-the-chat-template">Step 6: Apply the Chat Template</h4>
          <p>The remaining (non-cached) messages are run through the model's Jinja2 chat template. This converts the structured message array into the exact prompt string the model expects, including any special tokens, role markers, and tool definitions. For media requests, raw media bytes are returned alongside the text prompt.</p>
          <p><strong>Code:</strong></p>
          <ul>
            <li><code>createPrompt()</code> → <code>applyRequestJinjaTemplate()</code> (<code>model/chat.go</code>) — returns the prompt string plus media byte slices.</li>
          </ul>
          <h4 id="step-7-submit-to-the-batch-engine">Step 7: Submit to the Batch Engine</h4>
          <p>The fully prepared request — prompt string, media bytes, sampling parameters, and cache state — is packaged into a job and placed on the batch engine's request queue. A wake signal is sent so the batch engine picks it up immediately rather than waiting for its next poll cycle.</p>
          <p><strong>Code:</strong></p>
          <ul>
            <li><code>submitToBatchEngine()</code> (<code>model/chat.go</code>) — builds the <code>chatJob</code> struct (request data, cache state, IMC fields) and calls <code>batch.submit()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_engine.go">model/batch_engine.go</a>), which pushes onto <code>requestQ</code> and pokes <code>wakeCh</code>. The <code>queue-wait</code> span (§17.10) starts here.</li>
          </ul>
          <h4 id="step-8-assign-to-a-slot">Step 8: Assign to a Slot</h4>
          <p>The batch engine's processing loop wakes up and checks for pending work. It dequeues the job and assigns it to the first available processing slot. All IMC sessions (text and media) use first-available slot assignment. If all slots are busy, the longest-running slot is preempted after a configurable timeout (<code>cache-slot-timeout</code>, see §17.7.5).</p>
          <p><strong>Code:</strong></p>
          <ul>
            <li><code>processLoop()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_engine.go">model/batch_engine.go</a>) — signal-based wake on <code>wakeCh</code> (§17.7.5); polls at 100µs when active, 5ms when idle.</li>
            <li><code>processBatch()</code> clears the batch buffer, runs any pending slot preemption, prefills the draft model for speculative slots, adds 1 generation token per active slot, then continues text prefill via round-robin <code>addPrefillChunk()</code> and media prefill via <code>addPrefillMediaChunk()</code>.</li>
            <li><code>fillSlots()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_schedule.go">model/batch_schedule.go</a>) dequeues the job and assigns it to a slot.</li>
          </ul>
          <h4 id="step-9-initialize-the-slot">Step 9: Initialize the Slot</h4>
          <p>The assigned slot is prepared for this request:</p>
          <ol>
            <li><strong>Restore cached KV state</strong>: For IMC, the session's externalized KV state is restored from RAM into the slot's sequence via <code>StateSeqSetData</code>. Extension tokens are then decoded, or the sequence is cleared and rebuilt.</li>
            <li><strong>Build the sampler</strong>: A sampler chain is constructed from the request's sampling parameters (temperature, top_k, top_p, min_p, repetition penalties, etc.). If grammar-constrained output is requested, a separate grammar sampler is also created.</li>
            <li><strong>Snapshot cached prefix</strong>: For IMC, after cache build/extend but before suffix tokens are decoded, the cached prefix KV state is snapshotted to RAM via <code>StateSeqGetData</code>. This captures the reusable prefix for the next request.</li>
            <li><strong>Tokenize the prompt</strong>: The prompt string is converted into a sequence of token IDs. Only the non-cached portion of the prompt needs tokenization.</li>
            <li><strong>Context window check</strong>: The total token count (cached + new) is verified against the model's context window limit.</li>
          </ol>
          <p><strong>Code:</strong></p>
          <ul>
            <li><code>startSlot()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot_start.go">model/batch_slot_start.go</a>) — resets the slot, ends the <code>queue-wait</code> span, and starts the <code>process-request</code> and <code>prefill</code> spans (§17.10).</li>
            <li><code>toSampler()</code> builds the llama.cpp sampler chain (temperature, top_k, top_p, min_p, repetition penalties, DRY, XTC, mirostat); a separate grammar sampler is created if requested.</li>
            <li>IMC KV restore: <code>StateSeqSetData</code> from <code>session.kvState</code>, then <code>decodeTokensIntoCache()</code> for extend, or <code>MemorySeqRm</code> for rebuild, or partial trim.</li>
            <li>IMC KV snapshot: <code>StateSeqGetData</code> into <code>session.kvState</code> after build/extend.</li>
            <li><code>llama.Tokenize(m.vocab, prompt, m.addBOSToken, true)</code> — <code>special=true</code> ensures ChatML markers are recognized (§17.7.7).</li>
            <li>Draft prompt assembly for speculative decoding.</li>
            <li>First chunk added via <code>addPrefillChunk()</code>.</li>
          </ul>
          <h4 id="step-10-prefill-kv-cache-fill">Step 10: Prefill (KV Cache Fill)</h4>
          <p>The prompt tokens are fed through the model in chunks to build up the KV cache — this is the "prefill" phase. Tokens are added to a batch buffer up to the configured batch size limit, then a GPU forward pass (decode) is executed. When multiple slots are active, tokens are allocated round-robin across slots so no single request can starve others. This repeats until all prompt tokens have been processed.</p>
          <p>For media requests, image or audio embeddings are interleaved with text tokens and decoded through the model's multimodal pipeline.</p>
          <p><strong>Code:</strong></p>
          <ul>
            <li><code>addPrefillChunk()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_prefill_text.go">model/batch_prefill_text.go</a>) — adds tokens up to the <code>NBatch</code> cap; round-robin chunk size is <code>NUBatch</code>.</li>
            <li>Each token: <code>batch.Add(token, position, seqIDs, isLast)</code>.</li>
            <li><code>llama.Decode(lctx, batch)</code> — GPU forward pass, fills KV cache.</li>
            <li><code>llama.Synchronize(lctx)</code> — waits for GPU completion.</li>
            <li>Repeats until all prefill tokens are consumed.</li>
            <li>Media path: <code>addPrefillMediaChunk()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_prefill_media.go">model/batch_prefill_media.go</a>) interleaves embeddings with text tokens.</li>
          </ul>
          <h4 id="step-11-token-generation-decode-loop">Step 11: Token Generation (Decode Loop)</h4>
          <p>Once prefill is complete, the model enters the decode loop — generating one output token per iteration:</p>
          <ol>
            <li><strong>Forward pass</strong>: The most recently sampled token is added to the batch and decoded through the model. With multiple active slots, all their tokens are batched together in a single forward pass for efficiency.</li>
            <li><strong>Sampling</strong>: The model's output logits are processed through the sampler chain to select the next token. If grammar constraints are active, the sampler respects the grammar rules.</li>
            <li><strong>Speculative decoding</strong> (optional): A smaller draft model generates candidate tokens ahead of the main model. These drafts are verified in a single batch forward pass, accepting correct predictions and rejecting mismatches. This can significantly increase tokens per second.</li>
          </ol>
          <p><strong>Code:</strong></p>
          <ul>
            <li>Back in <code>processBatch</code> (<code>model/batch_engine.go</code>), for each active slot with <code>prefillDone=true</code>: <code>batch.Add(sampled, nPast, seqIDs, true)</code> then <code>llama.Decode()</code>.</li>
            <li>Speculative path: <code>generateDraftTokens()</code> → batch in draft + sampled → <code>verifySpeculativeTokens()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_speculative.go">model/batch_speculative.go</a>).</li>
            <li><code>processSlotToken()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_tokens.go">model/batch_tokens.go</a>) samples via <code>llama.SamplerSample(sampler, lctx, iBatch)</code> or, when grammar is active, <code>grammarSampler.SampleWithGrammar(lctx, sampler, iBatch)</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/grammar.go">model/grammar.go</a>).</li>
          </ul>
          <h4 id="step-12-process-each-token">Step 12: Process Each Token</h4>
          <p>Each sampled token goes through a processing pipeline:</p>
          <ol>
            <li><strong>Logprobs extraction</strong>: If requested, token log-probabilities are extracted from the model's logits before the sampler state is updated (§17.7.9 explains why the order matters).</li>
            <li><strong>End-of-generation check</strong>: If the token is an EOG (end-of-generation) token, generation stops and the request moves to the finish phase.</li>
            <li><strong>UTF-8 assembly</strong>: Tokens are converted to text bytes. Since a single Unicode character can span multiple tokens, partial bytes are buffered until a complete codepoint is available.</li>
            <li><strong>Content classification</strong>: A state machine categorizes the output into reasoning (think tags), completion (regular response), or tool call content. This determines how the text is accumulated and streamed.</li>
            <li><strong>Token counting</strong>: Each generated token is counted as either a reasoning token or a completion token for usage reporting.</li>
            <li><strong>Max tokens check</strong>: If the output token count reaches the requested limit, generation stops.</li>
            <li><strong>Stream to client</strong>: For non-tool content, each complete text fragment is sent as an SSE delta event through the response channel.</li>
          </ol>
          <p><strong>Code:</strong></p>
          <ul>
            <li><code>handleSampledToken()</code> (<code>model/batch_tokens.go</code>) drives the pipeline:</li>
          </ul>
          <p>- <code>extractLogprobs()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/logprobs.go">model/logprobs.go</a>) — <code>llama.GetLogitsIth</code> + log-softmax + top-k heap. Runs <strong>before</strong> <code>llama.SamplerAccept()</code> (§17.7.9). - <code>llama.SamplerAccept()</code> (and grammar accept). - <code>llama.VocabIsEOG()</code> → if true, jump to <code>finishSlot()</code>. - <code>llama.TokenToPiece()</code> → buffer partial multi-byte codepoints, then <code>extractCompleteUTF8()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_utf8.go">model/batch_utf8.go</a>). - First token: records <code>prefillDone=true</code>, computes TTFT, ends the <code>prefill</code> span, starts the <code>token-generation</code> span. - Processor classification: <code>stepGPT()</code> for GPT-OSS, <code>stepStandard()</code> for everything else (§17.7.8). Classifies content into reasoning / completion / tooling and detects tool-call markers. - Counter increment: <code>reasonTokens</code> or <code>completionTokens</code>. - Max-tokens check: if <code>outputTokens &gt;= maxTokens</code>, <code>finishSlot()</code>. - Accumulate into <code>finalContent</code> / <code>finalReasoning</code> / <code>finalTooling</code>. - <code>sendDeltaResponse()</code> for non-tool content (tool content is buffered until parse).</p>
          <h4 id="step-13-finish-the-request">Step 13: Finish the Request</h4>
          <p>When generation ends (EOG token, max tokens, or error), the request is finalized:</p>
          <ol>
            <li><strong>Flush remaining text</strong>: Any buffered UTF-8 bytes are flushed into the final response accumulators.</li>
            <li><strong>Parse tool calls</strong>: If the model generated tool call content, it is parsed into structured function calls with validated JSON arguments.</li>
            <li><strong>Calculate metrics</strong>: Tokens per second (TPS), time to first token (TTFT), and draft acceptance rates are computed.</li>
            <li><strong>Send final response</strong>: The complete response — including content, reasoning, tool calls, logprobs, and usage statistics — is sent through the response channel.</li>
            <li><strong>Clean up the KV cache</strong>: conversation prefix was already snapshotted to RAM during slot initialization and will be restored on the next request.
              <ul>
                <li>IMC (all model types): the entire VRAM sequence is cleared. The cached</li>
                <li>Without caching, the entire sequence is cleared.</li>
              </ul>
            </li>
            <li><strong>Free resources</strong>: The sampler, grammar sampler, and any multimodal resources (bitmaps, projection context) are freed.</li>
          </ol>
          <p><strong>Code:</strong></p>
          <ul>
            <li><code>finishSlot()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_finish.go">model/batch_finish.go</a>):</li>
          </ul>
          <p>- Flushes the UTF-8 buffer. - Tool-call parse: GPT-OSS path calls <code>parseGPTToolCall()</code> directly (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_finish.go#L177">batch_finish.go:177</a>); all other models route through <code>parseToolCall</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/processor_parse.go">model/processor_parse.go</a>) — see §17.7.8. - Metrics: TPS = <code>(outputTokens - 1) / elapsed</code>, TTFT, draft-acceptance rate. - <code>sendFinalResponse()</code> with usage, content, reasoning, tool calls, logprobs. - KV cleanup: <code>MemorySeqRm(mem, seqID, -1, -1)</code> clears the full VRAM sequence; the IMC prefix is already in RAM from Step 9. - Frees the sampler, grammar sampler, MTMD bitmaps/chunks, and <code>mtmdCtx</code>. - Closes the job channel; the <code>streaming()</code> wrapper drains and closes the caller's channel. - Decrements the <strong>model-level</strong> <code>m.activeStreams</code>.</p>
          <h4 id="step-14-release-the-model">Step 14: Release the Model</h4>
          <p>The response channel is closed, signaling to the caller that streaming is complete. The kronk-level semaphore slot is released, allowing the next queued request to begin processing.</p>
          <p><strong>Code:</strong></p>
          <ul>
            <li><code>releaseModel()</code> (<a href="file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/acquire.go">sdk/kronk/acquire.go</a>) — drains a slot from the kronk-level semaphore (<code>&lt;-krn.sem</code>) and decrements <code>krn.activeStreams</code> (the kronk-level counter; the model-level one was already decremented in Step 13).</li>
          </ul>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#chapter-1-introduction" className={`doc-index-header ${activeSection === 'chapter-1-introduction' ? 'active' : ''}`}>Chapter 1: Introduction</a>
              <ul>
                <li><a href="#11-what-is-kronk" className={activeSection === '11-what-is-kronk' ? 'active' : ''}>1.1 What is Kronk</a></li>
                <li><a href="#12-key-features" className={activeSection === '12-key-features' ? 'active' : ''}>1.2 Key Features</a></li>
                <li><a href="#13-supported-platforms-and-hardware" className={activeSection === '13-supported-platforms-and-hardware' ? 'active' : ''}>1.3 Supported Platforms and Hardware</a></li>
                <li><a href="#14-architecture-overview" className={activeSection === '14-architecture-overview' ? 'active' : ''}>1.4 Architecture Overview</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-2-installation-quick-start" className={`doc-index-header ${activeSection === 'chapter-2-installation-quick-start' ? 'active' : ''}`}>Chapter 2: Installation &amp; Quick Start</a>
              <ul>
                <li><a href="#21-prerequisites" className={activeSection === '21-prerequisites' ? 'active' : ''}>2.1 Prerequisites</a></li>
                <li><a href="#22-installing-the-cli" className={activeSection === '22-installing-the-cli' ? 'active' : ''}>2.2 Installing the CLI</a></li>
                <li><a href="#23-installing-libraries" className={activeSection === '23-installing-libraries' ? 'active' : ''}>2.3 Installing Libraries</a></li>
                <li><a href="#24-downloading-your-first-model" className={activeSection === '24-downloading-your-first-model' ? 'active' : ''}>2.4 Downloading Your First Model</a></li>
                <li><a href="#25-starting-the-server" className={activeSection === '25-starting-the-server' ? 'active' : ''}>2.5 Starting the Server</a></li>
                <li><a href="#26-model-configuration-file" className={activeSection === '26-model-configuration-file' ? 'active' : ''}>2.6 Model Configuration File</a></li>
                <li><a href="#27-verifying-the-installation" className={activeSection === '27-verifying-the-installation' ? 'active' : ''}>2.7 Verifying the Installation</a></li>
                <li><a href="#28-quick-start-summary" className={activeSection === '28-quick-start-summary' ? 'active' : ''}>2.8 Quick Start Summary</a></li>
                <li><a href="#29-nixos-setup" className={activeSection === '29-nixos-setup' ? 'active' : ''}>2.9 NixOS Setup</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-3-model-configuration" className={`doc-index-header ${activeSection === 'chapter-3-model-configuration' ? 'active' : ''}`}>Chapter 3: Model Configuration</a>
              <ul>
                <li><a href="#31-basic-configuration" className={activeSection === '31-basic-configuration' ? 'active' : ''}>3.1 Basic Configuration</a></li>
                <li><a href="#32-processor-selection" className={activeSection === '32-processor-selection' ? 'active' : ''}>3.2 Processor Selection</a></li>
                <li><a href="#33-gpu-configuration" className={activeSection === '33-gpu-configuration' ? 'active' : ''}>3.3 GPU Configuration</a></li>
                <li><a href="#34-kv-cache-quantization" className={activeSection === '34-kv-cache-quantization' ? 'active' : ''}>3.4 KV Cache Quantization</a></li>
                <li><a href="#35-flash-attention" className={activeSection === '35-flash-attention' ? 'active' : ''}>3.5 Flash Attention</a></li>
                <li><a href="#36-sliding-window-attention-swa" className={activeSection === '36-sliding-window-attention-swa' ? 'active' : ''}>3.6 Sliding Window Attention (SWA)</a></li>
                <li><a href="#37-parallel-inference-nseqmax" className={activeSection === '37-parallel-inference-nseqmax' ? 'active' : ''}>3.7 Parallel Inference (NSeqMax)</a></li>
                <li><a href="#38-understanding-gguf-quantization" className={activeSection === '38-understanding-gguf-quantization' ? 'active' : ''}>3.8 Understanding GGUF Quantization</a></li>
                <li><a href="#39-choosing-the-right-quantization" className={activeSection === '39-choosing-the-right-quantization' ? 'active' : ''}>3.9 Choosing the Right Quantization</a></li>
                <li><a href="#310-vram-estimation" className={activeSection === '310-vram-estimation' ? 'active' : ''}>3.10 VRAM Estimation</a></li>
                <li><a href="#311-model-specific-tuning" className={activeSection === '311-model-specific-tuning' ? 'active' : ''}>3.11 Model-Specific Tuning</a></li>
                <li><a href="#312-speculative-decoding" className={activeSection === '312-speculative-decoding' ? 'active' : ''}>3.12 Speculative Decoding</a></li>
                <li><a href="#313-sampling-parameters" className={activeSection === '313-sampling-parameters' ? 'active' : ''}>3.13 Sampling Parameters</a></li>
                <li><a href="#314-model-config-file-example" className={activeSection === '314-model-config-file-example' ? 'active' : ''}>3.14 Model Config File Example</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-4-batch-processing" className={`doc-index-header ${activeSection === 'chapter-4-batch-processing' ? 'active' : ''}`}>Chapter 4: Batch Processing</a>
              <ul>
                <li><a href="#41-architecture-overview" className={activeSection === '41-architecture-overview' ? 'active' : ''}>4.1 Architecture Overview</a></li>
                <li><a href="#42-slots-and-sequences" className={activeSection === '42-slots-and-sequences' ? 'active' : ''}>4.2 Slots and Sequences</a></li>
                <li><a href="#43-request-flow" className={activeSection === '43-request-flow' ? 'active' : ''}>4.3 Request Flow</a></li>
                <li><a href="#44-configuring-batch-processing" className={activeSection === '44-configuring-batch-processing' ? 'active' : ''}>4.4 Configuring Batch Processing</a></li>
                <li><a href="#45-concurrency-by-model-type" className={activeSection === '45-concurrency-by-model-type' ? 'active' : ''}>4.5 Concurrency by Model Type</a></li>
                <li><a href="#46-performance-tuning" className={activeSection === '46-performance-tuning' ? 'active' : ''}>4.6 Performance Tuning</a></li>
                <li><a href="#47-example-configuration" className={activeSection === '47-example-configuration' ? 'active' : ''}>4.7 Example Configuration</a></li>
                <li><a href="#48-imc-slot-scheduling" className={activeSection === '48-imc-slot-scheduling' ? 'active' : ''}>4.8 IMC Slot Scheduling</a></li>
                <li><a href="#49-model-types-and-state-management" className={activeSection === '49-model-types-and-state-management' ? 'active' : ''}>4.9 Model Types and State Management</a></li>
                <li><a href="#410-debugging-state-management" className={activeSection === '410-debugging-state-management' ? 'active' : ''}>4.10 Debugging State Management</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-5-message-caching" className={`doc-index-header ${activeSection === 'chapter-5-message-caching' ? 'active' : ''}`}>Chapter 5: Message Caching</a>
              <ul>
                <li><a href="#51-overview" className={activeSection === '51-overview' ? 'active' : ''}>5.1 Overview</a></li>
                <li><a href="#52-incremental-message-cache-imc" className={activeSection === '52-incremental-message-cache-imc' ? 'active' : ''}>5.2 Incremental Message Cache (IMC)</a></li>
                <li><a href="#53-single-user-caching" className={activeSection === '53-single-user-caching' ? 'active' : ''}>5.3 Single-User Caching</a></li>
                <li><a href="#54-when-to-use-imc" className={activeSection === '54-when-to-use-imc' ? 'active' : ''}>5.4 When to Use IMC</a></li>
                <li><a href="#55-cache-invalidation" className={activeSection === '55-cache-invalidation' ? 'active' : ''}>5.5 Cache Invalidation</a></li>
                <li><a href="#56-configuration-reference" className={activeSection === '56-configuration-reference' ? 'active' : ''}>5.6 Configuration Reference</a></li>
                <li><a href="#57-performance-and-limitations" className={activeSection === '57-performance-and-limitations' ? 'active' : ''}>5.7 Performance and Limitations</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-6-yarn-extended-context" className={`doc-index-header ${activeSection === 'chapter-6-yarn-extended-context' ? 'active' : ''}`}>Chapter 6: YaRN Extended Context</a>
              <ul>
                <li><a href="#61-understanding-context-extension" className={activeSection === '61-understanding-context-extension' ? 'active' : ''}>6.1 Understanding Context Extension</a></li>
                <li><a href="#62-when-to-use-yarn" className={activeSection === '62-when-to-use-yarn' ? 'active' : ''}>6.2 When to Use YaRN</a></li>
                <li><a href="#63-configuration" className={activeSection === '63-configuration' ? 'active' : ''}>6.3 Configuration</a></li>
                <li><a href="#64-scaling-types" className={activeSection === '64-scaling-types' ? 'active' : ''}>6.4 Scaling Types</a></li>
                <li><a href="#65-parameter-reference" className={activeSection === '65-parameter-reference' ? 'active' : ''}>6.5 Parameter Reference</a></li>
                <li><a href="#66-model-specific-examples" className={activeSection === '66-model-specific-examples' ? 'active' : ''}>6.6 Model-Specific Examples</a></li>
                <li><a href="#67-memory-impact" className={activeSection === '67-memory-impact' ? 'active' : ''}>6.7 Memory Impact</a></li>
                <li><a href="#68-quality-considerations" className={activeSection === '68-quality-considerations' ? 'active' : ''}>6.8 Quality Considerations</a></li>
                <li><a href="#69-example-long-document-processing" className={activeSection === '69-example-long-document-processing' ? 'active' : ''}>6.9 Example: Long Document Processing</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-7-model-server" className={`doc-index-header ${activeSection === 'chapter-7-model-server' ? 'active' : ''}`}>Chapter 7: Model Server</a>
              <ul>
                <li><a href="#71-starting-the-server" className={activeSection === '71-starting-the-server' ? 'active' : ''}>7.1 Starting the Server</a></li>
                <li><a href="#72-stopping-the-server" className={activeSection === '72-stopping-the-server' ? 'active' : ''}>7.2 Stopping the Server</a></li>
                <li><a href="#73-server-configuration" className={activeSection === '73-server-configuration' ? 'active' : ''}>7.3 Server Configuration</a></li>
                <li><a href="#74-model-caching" className={activeSection === '74-model-caching' ? 'active' : ''}>7.4 Model Caching</a></li>
                <li><a href="#75-model-config-files" className={activeSection === '75-model-config-files' ? 'active' : ''}>7.5 Model Config Files</a></li>
                <li><a href="#76-catalog-system" className={activeSection === '76-catalog-system' ? 'active' : ''}>7.6 Catalog System</a></li>
                <li><a href="#77-runtime-settings" className={activeSection === '77-runtime-settings' ? 'active' : ''}>7.7 Runtime Settings</a></li>
                <li><a href="#78-logging" className={activeSection === '78-logging' ? 'active' : ''}>7.8 Logging</a></li>
                <li><a href="#79-data-paths" className={activeSection === '79-data-paths' ? 'active' : ''}>7.9 Data Paths</a></li>
                <li><a href="#710-complete-example" className={activeSection === '710-complete-example' ? 'active' : ''}>7.10 Complete Example</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-8-api-endpoints" className={`doc-index-header ${activeSection === 'chapter-8-api-endpoints' ? 'active' : ''}`}>Chapter 8: API Endpoints</a>
              <ul>
                <li><a href="#81-endpoint-overview" className={activeSection === '81-endpoint-overview' ? 'active' : ''}>8.1 Endpoint Overview</a></li>
                <li><a href="#82-chat-completions" className={activeSection === '82-chat-completions' ? 'active' : ''}>8.2 Chat Completions</a></li>
                <li><a href="#83-responses-api" className={activeSection === '83-responses-api' ? 'active' : ''}>8.3 Responses API</a></li>
                <li><a href="#84-embeddings" className={activeSection === '84-embeddings' ? 'active' : ''}>8.4 Embeddings</a></li>
                <li><a href="#85-reranking" className={activeSection === '85-reranking' ? 'active' : ''}>8.5 Reranking</a></li>
                <li><a href="#86-tokenize" className={activeSection === '86-tokenize' ? 'active' : ''}>8.6 Tokenize</a></li>
                <li><a href="#87-tool-calling-function-calling" className={activeSection === '87-tool-calling-function-calling' ? 'active' : ''}>8.7 Tool Calling (Function Calling)</a></li>
                <li><a href="#88-models-list" className={activeSection === '88-models-list' ? 'active' : ''}>8.8 Models List</a></li>
                <li><a href="#89-authentication" className={activeSection === '89-authentication' ? 'active' : ''}>8.9 Authentication</a></li>
                <li><a href="#810-error-responses" className={activeSection === '810-error-responses' ? 'active' : ''}>8.10 Error Responses</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-9-request-parameters" className={`doc-index-header ${activeSection === 'chapter-9-request-parameters' ? 'active' : ''}`}>Chapter 9: Request Parameters</a>
              <ul>
                <li><a href="#91-sampling-parameters" className={activeSection === '91-sampling-parameters' ? 'active' : ''}>9.1 Sampling Parameters</a></li>
                <li><a href="#92-repetition-control" className={activeSection === '92-repetition-control' ? 'active' : ''}>9.2 Repetition Control</a></li>
                <li><a href="#93-advanced-sampling" className={activeSection === '93-advanced-sampling' ? 'active' : ''}>9.3 Advanced Sampling</a></li>
                <li><a href="#94-generation-control" className={activeSection === '94-generation-control' ? 'active' : ''}>9.4 Generation Control</a></li>
                <li><a href="#95-grammar-constrained-output" className={activeSection === '95-grammar-constrained-output' ? 'active' : ''}>9.5 Grammar Constrained Output</a></li>
                <li><a href="#96-logprobs-token-probabilities" className={activeSection === '96-logprobs-token-probabilities' ? 'active' : ''}>9.6 Logprobs (Token Probabilities)</a></li>
                <li><a href="#97-parameter-reference" className={activeSection === '97-parameter-reference' ? 'active' : ''}>9.7 Parameter Reference</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-10-multi-modal-models" className={`doc-index-header ${activeSection === 'chapter-10-multi-modal-models' ? 'active' : ''}`}>Chapter 10: Multi-Modal Models</a>
              <ul>
                <li><a href="#101-overview" className={activeSection === '101-overview' ? 'active' : ''}>10.1 Overview</a></li>
                <li><a href="#102-vision-models" className={activeSection === '102-vision-models' ? 'active' : ''}>10.2 Vision Models</a></li>
                <li><a href="#103-audio-models" className={activeSection === '103-audio-models' ? 'active' : ''}>10.3 Audio Models</a></li>
                <li><a href="#104-plain-base64-format" className={activeSection === '104-plain-base64-format' ? 'active' : ''}>10.4 Plain Base64 Format</a></li>
                <li><a href="#105-configuration-for-multi-modal-models" className={activeSection === '105-configuration-for-multi-modal-models' ? 'active' : ''}>10.5 Configuration for Multi-Modal Models</a></li>
                <li><a href="#106-memory-requirements" className={activeSection === '106-memory-requirements' ? 'active' : ''}>10.6 Memory Requirements</a></li>
                <li><a href="#107-imc-and-multi-modal-caching" className={activeSection === '107-imc-and-multi-modal-caching' ? 'active' : ''}>10.7 IMC and Multi-Modal Caching</a></li>
                <li><a href="#108-limitations" className={activeSection === '108-limitations' ? 'active' : ''}>10.8 Limitations</a></li>
                <li><a href="#109-example-image-analysis" className={activeSection === '109-example-image-analysis' ? 'active' : ''}>10.9 Example: Image Analysis</a></li>
                <li><a href="#1010-example-audio-transcription" className={activeSection === '1010-example-audio-transcription' ? 'active' : ''}>10.10 Example: Audio Transcription</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-11-security-authentication" className={`doc-index-header ${activeSection === 'chapter-11-security-authentication' ? 'active' : ''}`}>Chapter 11: Security &amp; Authentication</a>
              <ul>
                <li><a href="#111-enabling-authentication" className={activeSection === '111-enabling-authentication' ? 'active' : ''}>11.1 Enabling Authentication</a></li>
                <li><a href="#112-using-the-admin-token" className={activeSection === '112-using-the-admin-token' ? 'active' : ''}>11.2 Using the Admin Token</a></li>
                <li><a href="#113-key-management" className={activeSection === '113-key-management' ? 'active' : ''}>11.3 Key Management</a></li>
                <li><a href="#114-creating-user-tokens" className={activeSection === '114-creating-user-tokens' ? 'active' : ''}>11.4 Creating User Tokens</a></li>
                <li><a href="#115-token-examples" className={activeSection === '115-token-examples' ? 'active' : ''}>11.5 Token Examples</a></li>
                <li><a href="#116-using-tokens-in-api-requests" className={activeSection === '116-using-tokens-in-api-requests' ? 'active' : ''}>11.6 Using Tokens in API Requests</a></li>
                <li><a href="#117-authorization-flow" className={activeSection === '117-authorization-flow' ? 'active' : ''}>11.7 Authorization Flow</a></li>
                <li><a href="#118-rate-limiting" className={activeSection === '118-rate-limiting' ? 'active' : ''}>11.8 Rate Limiting</a></li>
                <li><a href="#119-configuration-reference" className={activeSection === '119-configuration-reference' ? 'active' : ''}>11.9 Configuration Reference</a></li>
                <li><a href="#1110-security-best-practices" className={activeSection === '1110-security-best-practices' ? 'active' : ''}>11.10 Security Best Practices</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-12-browser-ui-bui" className={`doc-index-header ${activeSection === 'chapter-12-browser-ui-bui' ? 'active' : ''}`}>Chapter 12: Browser UI (BUI)</a>
              <ul>
                <li><a href="#121-accessing-the-bui" className={activeSection === '121-accessing-the-bui' ? 'active' : ''}>12.1 Accessing the BUI</a></li>
                <li><a href="#122-sidebar-layout" className={activeSection === '122-sidebar-layout' ? 'active' : ''}>12.2 Sidebar Layout</a></li>
                <li><a href="#123-what-the-bui-provides" className={activeSection === '123-what-the-bui-provides' ? 'active' : ''}>12.3 What the BUI Provides</a></li>
                <li><a href="#124-authentication" className={activeSection === '124-authentication' ? 'active' : ''}>12.4 Authentication</a></li>
                <li><a href="#125-notes-on-live-state" className={activeSection === '125-notes-on-live-state' ? 'active' : ''}>12.5 Notes on Live State</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-13-client-integration" className={`doc-index-header ${activeSection === 'chapter-13-client-integration' ? 'active' : ''}`}>Chapter 13: Client Integration</a>
              <ul>
                <li><a href="#131-coding-agent-model-configuration" className={activeSection === '131-coding-agent-model-configuration' ? 'active' : ''}>13.1 Coding Agent Model Configuration</a></li>
                <li><a href="#132-cline" className={activeSection === '132-cline' ? 'active' : ''}>13.2 Cline</a></li>
                <li><a href="#133-kilo-code" className={activeSection === '133-kilo-code' ? 'active' : ''}>13.3 Kilo Code</a></li>
                <li><a href="#134-opencode" className={activeSection === '134-opencode' ? 'active' : ''}>13.4 OpenCode</a></li>
                <li><a href="#135-goose" className={activeSection === '135-goose' ? 'active' : ''}>13.5 Goose</a></li>
                <li><a href="#136-openwebui" className={activeSection === '136-openwebui' ? 'active' : ''}>13.6 OpenWebUI</a></li>
                <li><a href="#137-python-openai-sdk" className={activeSection === '137-python-openai-sdk' ? 'active' : ''}>13.7 Python OpenAI SDK</a></li>
                <li><a href="#138-curl-and-http-clients" className={activeSection === '138-curl-and-http-clients' ? 'active' : ''}>13.8 curl and HTTP Clients</a></li>
                <li><a href="#139-langchain" className={activeSection === '139-langchain' ? 'active' : ''}>13.9 LangChain</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-14-observability" className={`doc-index-header ${activeSection === 'chapter-14-observability' ? 'active' : ''}`}>Chapter 14: Observability</a>
              <ul>
                <li><a href="#141-debug-server" className={activeSection === '141-debug-server' ? 'active' : ''}>14.1 Debug Server</a></li>
                <li><a href="#142-debug-endpoints" className={activeSection === '142-debug-endpoints' ? 'active' : ''}>14.2 Debug Endpoints</a></li>
                <li><a href="#143-health-check-endpoints" className={activeSection === '143-health-check-endpoints' ? 'active' : ''}>14.3 Health Check Endpoints</a></li>
                <li><a href="#144-prometheus-metrics" className={activeSection === '144-prometheus-metrics' ? 'active' : ''}>14.4 Prometheus Metrics</a></li>
                <li><a href="#145-prometheus-integration" className={activeSection === '145-prometheus-integration' ? 'active' : ''}>14.5 Prometheus Integration</a></li>
                <li><a href="#146-distributed-tracing-with-tempo" className={activeSection === '146-distributed-tracing-with-tempo' ? 'active' : ''}>14.6 Distributed Tracing with Tempo</a></li>
                <li><a href="#147-tracing-architecture" className={activeSection === '147-tracing-architecture' ? 'active' : ''}>14.7 Tracing Architecture</a></li>
                <li><a href="#148-tempo-setup-with-docker" className={activeSection === '148-tempo-setup-with-docker' ? 'active' : ''}>14.8 Tempo Setup with Docker</a></li>
                <li><a href="#149-pprof-profiling" className={activeSection === '149-pprof-profiling' ? 'active' : ''}>14.9 pprof Profiling</a></li>
                <li><a href="#1410-statsviz-real-time-monitoring" className={activeSection === '1410-statsviz-real-time-monitoring' ? 'active' : ''}>14.10 Statsviz Real-Time Monitoring</a></li>
                <li><a href="#1411-logging" className={activeSection === '1411-logging' ? 'active' : ''}>14.11 Logging</a></li>
                <li><a href="#1412-configuration-reference" className={activeSection === '1412-configuration-reference' ? 'active' : ''}>14.12 Configuration Reference</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-15-mcp-service" className={`doc-index-header ${activeSection === 'chapter-15-mcp-service' ? 'active' : ''}`}>Chapter 15: MCP Service</a>
              <ul>
                <li><a href="#151-architecture" className={activeSection === '151-architecture' ? 'active' : ''}>15.1 Architecture</a></li>
                <li><a href="#152-prerequisites" className={activeSection === '152-prerequisites' ? 'active' : ''}>15.2 Prerequisites</a></li>
                <li><a href="#153-configuration" className={activeSection === '153-configuration' ? 'active' : ''}>15.3 Configuration</a></li>
                <li><a href="#154-available-tools" className={activeSection === '154-available-tools' ? 'active' : ''}>15.4 Available Tools</a></li>
                <li><a href="#155-client-configuration" className={activeSection === '155-client-configuration' ? 'active' : ''}>15.5 Client Configuration</a></li>
                <li><a href="#156-testing-with-curl" className={activeSection === '156-testing-with-curl' ? 'active' : ''}>15.6 Testing with curl</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-16-troubleshooting" className={`doc-index-header ${activeSection === 'chapter-16-troubleshooting' ? 'active' : ''}`}>Chapter 16: Troubleshooting</a>
              <ul>
                <li><a href="#161-library-issues" className={activeSection === '161-library-issues' ? 'active' : ''}>16.1 Library Issues</a></li>
                <li><a href="#162-model-loading-failures" className={activeSection === '162-model-loading-failures' ? 'active' : ''}>16.2 Model Loading Failures</a></li>
                <li><a href="#163-memory-errors" className={activeSection === '163-memory-errors' ? 'active' : ''}>16.3 Memory Errors</a></li>
                <li><a href="#164-request-timeouts" className={activeSection === '164-request-timeouts' ? 'active' : ''}>16.4 Request Timeouts</a></li>
                <li><a href="#165-authentication-errors" className={activeSection === '165-authentication-errors' ? 'active' : ''}>16.5 Authentication Errors</a></li>
                <li><a href="#166-streaming-issues" className={activeSection === '166-streaming-issues' ? 'active' : ''}>16.6 Streaming Issues</a></li>
                <li><a href="#167-performance-issues" className={activeSection === '167-performance-issues' ? 'active' : ''}>16.7 Performance Issues</a></li>
                <li><a href="#168-imc-caching-issues" className={activeSection === '168-imc-caching-issues' ? 'active' : ''}>16.8 IMC Caching Issues</a></li>
                <li><a href="#169-viewing-logs" className={activeSection === '169-viewing-logs' ? 'active' : ''}>16.9 Viewing Logs</a></li>
                <li><a href="#1610-common-error-messages" className={activeSection === '1610-common-error-messages' ? 'active' : ''}>16.10 Common Error Messages</a></li>
                <li><a href="#1611-catalog-model-pull-issues" className={activeSection === '1611-catalog-model-pull-issues' ? 'active' : ''}>16.11 Catalog &amp; Model Pull Issues</a></li>
                <li><a href="#1612-mcp-service-issues" className={activeSection === '1612-mcp-service-issues' ? 'active' : ''}>16.12 MCP Service Issues</a></li>
                <li><a href="#1613-port-conflicts-filesystem" className={activeSection === '1613-port-conflicts-filesystem' ? 'active' : ''}>16.13 Port Conflicts &amp; Filesystem</a></li>
                <li><a href="#1614-getting-help" className={activeSection === '1614-getting-help' ? 'active' : ''}>16.14 Getting Help</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-17-developer-guide" className={`doc-index-header ${activeSection === 'chapter-17-developer-guide' ? 'active' : ''}`}>Chapter 17: Developer Guide</a>
              <ul>
                <li><a href="#171-quick-reference" className={activeSection === '171-quick-reference' ? 'active' : ''}>17.1 Quick Reference</a></li>
                <li><a href="#172-build-test-commands" className={activeSection === '172-build-test-commands' ? 'active' : ''}>17.2 Build &amp; Test Commands</a></li>
                <li><a href="#173-developer-setup" className={activeSection === '173-developer-setup' ? 'active' : ''}>17.3 Developer Setup</a></li>
                <li><a href="#174-project-architecture" className={activeSection === '174-project-architecture' ? 'active' : ''}>17.4 Project Architecture</a></li>
                <li><a href="#175-bui-frontend-development" className={activeSection === '175-bui-frontend-development' ? 'active' : ''}>17.5 BUI Frontend Development</a></li>
                <li><a href="#176-code-style-guidelines" className={activeSection === '176-code-style-guidelines' ? 'active' : ''}>17.6 Code Style Guidelines</a></li>
                <li><a href="#177-sdk-internals" className={activeSection === '177-sdk-internals' ? 'active' : ''}>17.7 SDK Internals</a></li>
                <li><a href="#178-responses-api-normalization" className={activeSection === '178-responses-api-normalization' ? 'active' : ''}>17.8 Responses API Normalization</a></li>
                <li><a href="#179-goroutine-budget" className={activeSection === '179-goroutine-budget' ? 'active' : ''}>17.9 Goroutine Budget</a></li>
                <li><a href="#1710-request-tracing-spans" className={activeSection === '1710-request-tracing-spans' ? 'active' : ''}>17.10 Request Tracing Spans</a></li>
                <li><a href="#1711-inference-code-path" className={activeSection === '1711-inference-code-path' ? 'active' : ''}>17.11 Inference Code Path</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
