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
          <h3 id="11-what-is-kronk?">1.1 What Is Kronk?</h3>
          <p>Kronk is a Go SDK and model server for running open-source models on your own hardware. It uses two C++ inference engines:</p>
          <ul>
            <li><a href="https://github.com/ggml-org/llama.cpp">llama.cpp</a> runs GGUF text, vision, embedding, and reranking models.</li>
            <li><a href="https://github.com/ggml-org/whisper.cpp">whisper.cpp</a> runs speech-to-text models through Kronk's <strong>Bucky</strong> subsystem.</li>
          </ul>
          <p>Kronk provides hardware-accelerated text generation, image understanding, audio transcription, embeddings, and reranking from Go without requiring Python or a CGO build toolchain.</p>
          <h3 id="12-sdk-or-model-server?">1.2 SDK or Model Server?</h3>
          <p>The model server's inference functionality is built on the same public SDK that is available to Go applications. You can either embed inference directly in an application or run the server for network access.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Use the SDK when you need...</th>
                <th>Use the Model Server when you need...</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Inference inside a Go process</td>
                <td>HTTP APIs for one or more clients</td>
              </tr>
              <tr>
                <td>Direct control over model loading and lifetime</td>
                <td>OpenAI-compatible APIs and an Anthropic-compatible Messages API</td>
              </tr>
              <tr>
                <td>No separate server process</td>
                <td>A browser interface for managing and testing models</td>
              </tr>
              <tr>
                <td>Application-specific caching and concurrency</td>
                <td>Authentication, authorization, and rate limiting</td>
              </tr>
            </tbody>
          </table>
          <p>Standalone SDK examples are available in the <a href="../examples"><code>examples</code></a> directory.</p>
          <h3 id="13-capabilities">1.3 Capabilities</h3>
          <p><strong>Inference</strong></p>
          <ul>
            <li><strong>Text generation</strong> — chat completions and responses, including streaming, reasoning, and tool calls.</li>
            <li><strong>Vision</strong> — image understanding with compatible multimodal models.</li>
            <li><strong>Audio transcription</strong> — speech-to-text through Bucky and whisper.cpp. See <a href="chapter-18-bucky.md">Chapter 18: Bucky</a>.</li>
            <li><strong>Embeddings and reranking</strong> — vector generation and document relevance scoring for search and retrieval systems.</li>
          </ul>
          <p><strong>Performance</strong></p>
          <ul>
            <li><strong>Concurrent processing</strong> — partition a model's KV cache into sequences so it can process multiple requests.</li>
            <li><strong>Message caching</strong> — reuse computed prompt prefixes instead of evaluating them again for every request.</li>
            <li><strong>Extended context</strong> — configure YaRN context extension for compatible models.</li>
            <li><strong>Model pooling</strong> — keep a configurable number of models loaded and evict inactive models after a time-to-live period.</li>
          </ul>
          <p><strong>Operation</strong></p>
          <ul>
            <li><strong>Model catalog</strong> — discover, download, and configure a curated set of models through the CLI or Browser UI (BUI).</li>
            <li><strong>Client APIs</strong> — use OpenAI-compatible endpoints, the Anthropic-compatible Messages endpoint, or integrations such as OpenWebUI and OpenCode.</li>
            <li><strong>Security</strong> — protect endpoints with JWT authentication, scoped authorization, and rate limits.</li>
            <li><strong>Observability</strong> — collect traces and Prometheus metrics for external observability systems.</li>
          </ul>
          <p>Kronk stores its managed files under <code>~/.kronk/</code> by default. The location is configurable; the official containers use <code>/kronk</code> so the directory can be mounted as persistent storage.</p>
          <p>Hardware support differs by operating system, architecture, inference engine, and release artifact. Kronk can use CPU inference and GPU backends such as Metal, CUDA, Vulkan, and ROCm where a compatible library bundle is available. See <a href="chapter-02-installation.md">Chapter 2: Installation & Quick Start</a> for the currently distributed native and container options.</p>
          <p>Memory requirements depend on more than model parameter count. Quantization, context size, KV cache type, batch size, concurrency, and multimodal projections all affect RAM and VRAM use. See <a href="chapter-03-model-configuration.md">Chapter 3: Model Configuration</a> before selecting a large model or context window.</p>
          <h3 id="14-architecture">1.4 Architecture</h3>
          <p>Kronk is layered so applications and the model server use the same inference SDKs. The two engine paths provide different model capabilities and may have different platform support.</p>
          <pre className="code-block"><code className="language-diagram">{`Your Go Application                 Kronk Model Server
        |                                   |
        +----------------+------------------+
                         |
              +----------+----------+
              |                     |
          Kronk SDK             Bucky SDK
    text, vision, embedding,      speech-to-text
           reranking                  |
              |                       |
            yzma                Bucky bindings
              |                       |
          llama.cpp               whisper.cpp
              +-----------+-----------+
                          |
              CPU / Metal / CUDA / Vulkan / ROCm`}</code></pre>
          <p>The SDK layer owns model loading, inference, caching, and concurrency. The model server adds HTTP transport, model pooling, the BUI, security, and operational services. Your application can use the SDK without starting the model server.</p>
          <h3 id="15-where-to-go-next">1.5 Where to Go Next</h3>
          <ul>
            <li><strong>Install Kronk and run your first model:</strong> <a href="chapter-02-installation.md">Chapter 2: Installation & Quick Start</a></li>
            <li><strong>Choose memory, context, and GPU settings:</strong> <a href="chapter-03-model-configuration.md">Chapter 3: Model Configuration</a></li>
            <li><strong>Connect an editor, agent, or other client:</strong> <a href="chapter-14-client-integration.md">Chapter 14: Client Integration</a></li>
            <li><strong>Run speech-to-text models:</strong> <a href="chapter-18-bucky.md">Chapter 18: Bucky</a></li>
          </ul>
          <hr />
          <h2 id="chapter-2-installation-quick-start">Chapter 2: Installation &amp; Quick Start</h2>
          <h3 id="21-native-quick-start">2.1 Native Quick Start</h3>
          <p>This path installs Kronk on the host, downloads a starter model, and starts the model server. The first run requires an internet connection for native libraries and model files.</p>
          <p><strong>1. Install the CLI with Homebrew (macOS or Linux)</strong></p>
          <pre className="code-block"><code className="language-shell">{`brew tap ardanlabs/kronk
brew trust ardanlabs/kronk
brew install kronk`}</code></pre>
          <p>Other installation methods are listed in <a href="#22-choose-an-installation-method">Section 2.2</a>.</p>
          <p><strong>2. Install the native inference libraries</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local`}</code></pre>
          <p>Kronk detects the operating system, architecture, and available GPU backend. It selects Metal, CUDA, ROCm, or Vulkan when supported and falls back to CPU.</p>
          <p><strong>3. Download a starter model</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk model pull mradermacher/Qwopus3.5-4B-Coder.Q8_0 --local`}</code></pre>
          <p>This coding model requires about 5.3 GB of disk space, including its multimodal projection. On memory-constrained or CPU-only systems, start with the smaller <code>unsloth/Qwen3-0.6B-Q8_0</code> instead.</p>
          <p><strong>4. Start the server in the background</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --api-host=127.0.0.1:11435 -d`}</code></pre>
          <p>The explicit host keeps this unauthenticated quick-start server reachable only from the local machine. The API listens on <code>http://localhost:11435</code>. The Browser UI (BUI) is enabled by default at <code>http://localhost:11435/admin/</code>.</p>
          <p><strong>5. Try the model</strong></p>
          <p>Open <code>http://localhost:11435/admin/</code>, select <strong>Apps → Chat</strong>, and choose the downloaded model. The first request takes longer because Kronk must load the model into memory.</p>
          <p>You can also call the API directly:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "mradermacher/Qwopus3.5-4B-Coder.Q8_0/AGENT",
    "messages": [{"role": "user", "content": "Write a Go function that reverses a string."}],
    "max_tokens": 256
  }'`}</code></pre>
          <p>Stop the background server with:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server stop`}</code></pre>
          <h3 id="22-choose-an-installation-method">2.2 Choose an Installation Method</h3>
          <p>The prerequisites depend on how you install Kronk:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Method</th>
                <th>Platforms</th>
                <th>Prerequisite</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Homebrew</td>
                <td>macOS and Linux</td>
                <td>Homebrew</td>
              </tr>
              <tr>
                <td>Release archive</td>
                <td>Supported release platforms</td>
                <td>None beyond the host OS</td>
              </tr>
              <tr>
                <td><code>go install</code></td>
                <td>Platforms supported by the Go toolchain and Kronk libraries</td>
                <td>Go 1.26 or later</td>
              </tr>
              <tr>
                <td>Container</td>
                <td>Any host supported by the selected Linux image</td>
                <td>Docker or another OCI runtime</td>
              </tr>
            </tbody>
          </table>
          <p>Regardless of installation method, model downloads require enough disk space, and inference requires enough RAM or VRAM for the selected model and configuration. Quantization, context size, KV cache type, and concurrency all affect memory use; parameter count alone is not a reliable requirement. See <a href="chapter-03-model-configuration.md">Chapter 3: Model Configuration</a>.</p>
          <h4 id="homebrew">Homebrew</h4>
          <pre className="code-block"><code className="language-shell">{`brew tap ardanlabs/kronk
brew trust ardanlabs/kronk
brew install kronk`}</code></pre>
          <p>Upgrade an existing installation with:</p>
          <pre className="code-block"><code className="language-shell">{`brew upgrade kronk`}</code></pre>
          <h4 id="release-archive">Release archive</h4>
          <p>Download the archive for your operating system and architecture from the <a href="https://github.com/ardanlabs/kronk/releases">Kronk releases page</a>, extract the <code>kronk</code> executable, and place it on your <code>PATH</code>.</p>
          <h4 id="go-toolchain">Go toolchain</h4>
          <pre className="code-block"><code className="language-shell">{`go install github.com/ardanlabs/kronk/cmd/kronk@latest`}</code></pre>
          <p>Ensure the Go binary directory (normally <code>$(go env GOPATH)/bin</code>) is on your <code>PATH</code>.</p>
          <h4 id="confirm-the-cli-is-installed">Confirm the CLI is installed</h4>
          <pre className="code-block"><code className="language-shell">{`kronk --version
kronk --help`}</code></pre>
          <p>Use <code>kronk &lt;command&gt; --help</code> for the current command and flag reference rather than relying on copied CLI output.</p>
          <h3 id="23-container-quick-start">2.3 Container Quick Start</h3>
          <p>Release images are published to GHCR and Docker Hub. Each image contains the Kronk executable, BUI, ffmpeg, and the native llama.cpp and whisper.cpp libraries for its processor variant.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Tag</th>
                <th>Intended hardware</th>
                <th>Published platforms</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>latest</code> or <code>latest-cpu</code></td>
                <td>CPU inference</td>
                <td><code>linux/amd64</code>, <code>linux/arm64</code></td>
              </tr>
              <tr>
                <td><code>latest-cuda</code></td>
                <td>NVIDIA GPU</td>
                <td><code>linux/amd64</code>, <code>linux/arm64</code></td>
              </tr>
              <tr>
                <td><code>latest-vulkan</code></td>
                <td>AMD, NVIDIA, or Intel through Vulkan</td>
                <td><code>linux/amd64</code>, <code>linux/arm64</code></td>
              </tr>
              <tr>
                <td><code>latest-rocm</code></td>
                <td>AMD GPU through ROCm</td>
                <td><code>linux/amd64</code></td>
              </tr>
              <tr>
                <td><code>latest-all</code></td>
                <td>All backends built for the target architecture</td>
                <td><code>linux/amd64</code>, <code>linux/arm64</code></td>
              </tr>
            </tbody>
          </table>
          <p>Versioned tags use the form <code>vX.Y.Z-&lt;variant&gt;</code>. Pin a versioned tag for a reproducible deployment; <code>latest-*</code> tags move when a release is published.</p>
          <p><strong>Local CPU container</strong></p>
          <p>This command binds the service only to the local machine, enables BUI model downloads, and persists all state in a named volume:</p>
          <pre className="code-block"><code className="language-shell">{`docker run -d \\
  --name kronk \\
  --restart unless-stopped \\
  -e KRONK_DOWNLOAD_ENABLED=true \\
  -p 127.0.0.1:11435:11435 \\
  -v kronk-data:/kronk \\
  ghcr.io/ardanlabs/kronk:latest`}</code></pre>
          <p>Open <code>http://localhost:11435/admin/</code>, download a model from the catalog, and use <strong>Apps → Chat</strong> to test it.</p>
          <p><strong>NVIDIA container</strong></p>
          <p>Install the <a href="https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html">NVIDIA Container Toolkit</a>, then add GPU access and use the CUDA image:</p>
          <pre className="code-block"><code className="language-shell">{`docker run -d \\
  --name kronk \\
  --restart unless-stopped \\
  --runtime=nvidia --gpus all \\
  -e KRONK_DOWNLOAD_ENABLED=true \\
  -p 127.0.0.1:11435:11435 \\
  -v kronk-data:/kronk \\
  ghcr.io/ardanlabs/kronk:latest-cuda`}</code></pre>
          <p>AMD ROCm and Vulkan require host-specific device access. The tested command lines and compatibility notes are maintained in the header of the <a href="../zarf/docker/kronk/Dockerfile"><code>Dockerfile</code></a>.</p>
          <p>The container runs as UID/GID <code>10001</code>. A named volume needs no preparation. If you use a host directory such as <code>/srv/kronk</code>, make it writable by that user before starting the container:</p>
          <pre className="code-block"><code className="language-shell">{`sudo mkdir -p /srv/kronk
sudo chown -R 10001:10001 /srv/kronk`}</code></pre>
          <p>For pinned releases, updates, and remote operation, see <a href="chapter-08-model-server.md#87-container-operations">Chapter 8: Model Server</a>.</p>
          <h3 id="24-libraries">2.4 Libraries</h3>
          <p>Kronk needs a native llama.cpp library bundle before it can run GGUF models. The server checks the selected bundle during startup and installs it when needed. To install it explicitly before starting the server, run:</p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local`}</code></pre>
          <p>The default location is:</p>
          <pre className="code-block"><code className="language-text">{`~/.kronk/libraries/<os>/<arch>/<processor>/`}</code></pre>
          <p>Examples include <code>darwin/arm64/metal</code> and <code>linux/amd64/cuda</code>. Multiple bundles can coexist. Kronk selects the folder matching the detected host unless <code>KRONK_ARCH</code>, <code>KRONK_OS</code>, <code>KRONK_PROCESSOR</code>, or <code>KRONK_LIB_PATH</code> overrides it.</p>
          <p>Useful commands:</p>
          <pre className="code-block"><code className="language-shell">{`# Show supported bundles.
kronk libs --list-combinations

# Show installed bundles.
kronk libs --list-installs

# Install a particular published version.
kronk libs --local --version=<llama.cpp-build>

# Explicitly select CPU instead of an available GPU.
KRONK_PROCESSOR=cpu kronk libs --local`}</code></pre>
          <p>The normal command installs the llama.cpp version selected for the installed Kronk release. <code>--upgrade</code> opts into the latest published llama.cpp build, which may introduce upstream compatibility changes:</p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local --upgrade`}</code></pre>
          <p>Use <code>kronk libs --help</code> for cross-platform bundle installation and removal. Changing the active library path requires a server restart; libraries are not hot-reloaded.</p>
          <p>Linux CUDA bundles depend on CUDA runtime libraries supplied by the host. A working <code>nvidia-smi</code> confirms the driver but not necessarily the CUDA runtime. If the CUDA backend does not load, inspect <code>libggml-cuda.so</code> with <code>ldd</code> and install the runtime packages appropriate for the CUDA version used by the current bundle. The CUDA container image already includes its matching runtime.</p>
          <p>Speech-to-text uses a separate whisper.cpp bundle:</p>
          <pre className="code-block"><code className="language-shell">{`kronk bucky libs --local`}</code></pre>
          <p>See <a href="chapter-18-bucky.md#182-installation-libraries">Chapter 18: Bucky</a> for Bucky platforms, models, and configuration.</p>
          <h3 id="25-models-and-data-paths">2.5 Models and Data Paths</h3>
          <p>List the starter catalog and download a model directly on the host:</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list --local
kronk model pull unsloth/Qwen3-0.6B-Q8_0 --local`}</code></pre>
          <p>Model sources may be bare IDs, canonical <code>provider/model</code> IDs, Hugging Face URLs, or repository-and-quantization shorthands. Run <code>kronk model pull --help</code> for all accepted forms.</p>
          <p>The default data layout is:</p>
          <pre className="code-block"><code className="language-text">{`~/.kronk/
├── catalog/
│   └── catalog.yaml
├── libraries/
│   └── <os>/<arch>/<processor>/
├── models/
│   ├── model_config.yaml
│   ├── .index.yaml
│   └── <provider>/<family>/<model files>
├── bucky-libraries/
├── bucky-models/
└── keys/`}</code></pre>
          <p>Older installations that stored <code>catalog.yaml</code> or <code>model_config.yaml</code> directly under <code>~/.kronk/</code> are migrated automatically when the new location is first used.</p>
          <p>Set <code>KRONK_BASE_PATH</code> or the global <code>--base-path</code> flag to move the entire data root. Official containers set it to <code>/kronk</code>.</p>
          <p>The model configuration file contains per-model and per-variant overrides. Do not copy configuration values based only on model size; use <a href="chapter-03-model-configuration.md">Chapter 3</a> for context, cache, GPU, and sampling settings.</p>
          <h3 id="26-running-the-server">2.6 Running the Server</h3>
          <p>Start in the foreground:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --api-host=127.0.0.1:11435`}</code></pre>
          <p>Start in the background:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --api-host=127.0.0.1:11435 -d`}</code></pre>
          <p>The background process writes logs to <code>~/.kronk/kronk.log</code>. Manage it with:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server logs
kronk server stop`}</code></pre>
          <p>The server's configured default is <code>0.0.0.0:11435</code>, which listens on every network interface. The commands above deliberately use <code>127.0.0.1</code> for local operation. The API and BUI share port <code>11435</code>; the BUI is enabled by default and is served under <code>/admin/</code>. Disable it for a headless server with:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --api-host=127.0.0.1:11435 \\
  --web-admin-enabled=false`}</code></pre>
          <p>Server-side downloading is separately controlled by <code>KRONK_DOWNLOAD_ENABLED</code> and defaults to <code>false</code>. Local CLI commands using <code>--local</code> are not affected by this setting.</p>
          <p>See <a href="chapter-08-model-server.md">Chapter 8</a> for server flags, model pooling, runtime paths, and deployment operations.</p>
          <h3 id="27-verify-the-installation">2.7 Verify the Installation</h3>
          <p>Check server liveness:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/liveness`}</code></pre>
          <p>List models visible to the server:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/models`}</code></pre>
          <p>Test a downloaded model:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "unsloth/Qwen3-0.6B-Q8_0",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 100
  }'`}</code></pre>
          <p>If authentication is enabled, add <code>-H "Authorization: Bearer $KRONK_TOKEN"</code> to API requests.</p>
          <h3 id="28-security-and-next-steps">2.8 Security and Next Steps</h3>
          <p>Authentication is disabled by default. Do not publish port <code>11435</code> to an untrusted network until authentication and TLS termination are configured. The BUI, management endpoints, and inference endpoints have separate security controls; see <a href="chapter-12-security-authentication.md">Chapter 12: Security & Authentication</a> before operating a shared or internet-reachable server.</p>
          <p>Continue with:</p>
          <ul>
            <li><a href="chapter-03-model-configuration.md">Chapter 3: Model Configuration</a> for memory, context, GPU, cache, and sampling settings.</li>
            <li><a href="chapter-08-model-server.md">Chapter 8: Model Server</a> for server operation and deployment.</li>
            <li><a href="chapter-12-security-authentication.md">Chapter 12: Security & Authentication</a> before exposing the service.</li>
            <li><a href="chapter-14-client-integration.md">Chapter 14: Client Integration</a> to connect an editor or coding agent.</li>
            <li><a href="chapter-18-bucky.md">Chapter 18: Bucky</a> for speech-to-text.</li>
          </ul>
          <hr />
          <h2 id="chapter-3-model-configuration">Chapter 3: Model Configuration</h2>
          <p>Kronk analyzes each model and the available hardware before loading it. Most models run well without manual tuning. Use per-model configuration when you need a different context window, more concurrent requests, explicit device placement, or an advanced feature such as speculative decoding.</p>
          <p>This chapter documents model runtime configuration. Server settings such as the listen address, authentication, and the number of models kept in the pool are covered in <a href="chapter-08-model-server.md">Chapter 8</a>.</p>
          <h3 id="31-configuration-file">3.1 Configuration File</h3>
          <p>The model server reads per-model overrides from:</p>
          <pre className="code-block"><code className="language-text">{`~/.kronk/models/model_config.yaml`}</code></pre>
          <p>Kronk creates this file on first use. The file is a flat YAML map keyed by the canonical model ID. Use the same ID shown by <code>kronk model list</code> or the <code>/v1/models</code> endpoint:</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/Qwen3-0.6B-Q8_0:
  context-window: 32768
  nseq-max: 2`}</code></pre>
          <p>Do not add a <code>models:</code> wrapper. Top-level setting names use kebab-case, such as <code>context-window</code> and <code>nseq-max</code>. Keys nested under <code>sampling-parameters</code> use the API's snake_case names, such as <code>top_p</code>.</p>
          <p>The server reads this file during startup. Restart the server after changing it. To test a different file without replacing the default, run:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --model-config-file=./my-model-config.yaml`}</code></pre>
          <p>You can also set <code>KRONK_POOL_MODEL_CONFIG_FILE</code> to an alternative path. See <a href="chapter-08-model-server.md#85-model-configuration-files">Chapter 8 §8.5</a> for model config file management and <a href="chapter-02-installation.md#25-models-and-data-paths">Chapter 2 §2.5</a> for all data paths.</p>
          <h4 id="model-variants">Model variants</h4>
          <p>A suffix creates another configuration for the same downloaded model:</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/Qwen3-0.6B-Q8_0:
  context-window: 32768

unsloth/Qwen3-0.6B-Q8_0/LONG:
  context-window: 65536`}</code></pre>
          <p>Select the variant by sending the complete name, including <code>/LONG</code>, as the API request's <code>model</code> value. Variants let applications use different runtime settings without keeping duplicate model files.</p>
          <h4 id="other-configuration-surfaces">Other configuration surfaces</h4>
          <p>Applications embedding the Go SDK can construct a <code>model.Config</code> directly. Request fields such as <code>temperature</code>, <code>top_p</code>, and <code>max_tokens</code> can override generation behavior for an individual request. Those request fields are documented in <a href="chapter-10-request-parameters.md">Chapter 10</a>.</p>
          <p>The hardware processor (<code>cpu</code>, <code>metal</code>, <code>cuda</code>, <code>rocm</code>, or <code>vulkan</code>) selects a native library bundle rather than a per-model setting. Kronk detects it during library installation. Set <code>KRONK_PROCESSOR</code> before installing libraries only when you need to override detection; see <a href="chapter-02-installation.md#24-libraries">Chapter 2 §2.4</a>.</p>
          <h3 id="32-automatic-tuning">3.2 Automatic Tuning</h3>
          <p>The model server derives a starting configuration from GGUF metadata and the available hardware. This analysis chooses values such as:</p>
          <ul>
            <li>context window;</li>
            <li>KV cache types;</li>
            <li>maximum parallel sequences;</li>
            <li>GPU layer placement;</li>
            <li>Flash Attention mode; and</li>
            <li>multi-GPU split mode.</li>
          </ul>
          <p>A concrete override in <code>model_config.yaml</code> replaces the analyzed value. The special cache type <code>auto</code> is treated as unset and therefore does not clear an analyzed <code>f16</code> or <code>q8_0</code> choice. This makes the usual workflow:</p>
          <ol>
            <li>Start with no override and let Kronk analyze the model.</li>
            <li>Use the model normally and monitor memory and latency.</li>
            <li>Override only the setting needed for the workload.</li>
          </ol>
          <p>The balanced analysis limits the selected context to the model's training context and a maximum of 128K tokens. It estimates the largest supported context bucket that fits its GPU budget, with a 4K minimum recommendation. It starts with an f16 KV cache and tries q8_0 if the minimum f16 configuration does not fit. CPU-only analysis and systems without a known GPU budget cannot perform the same fit check. All recommendations are estimates, not a guarantee that every backend and workload will fit or have identical memory use.</p>
          <p>In the Go SDK, the same analysis is opt-in through <code>WithAutoTune</code>. It is not applied when an application uses the low-level <code>model</code> package directly. Explicit SDK options still take precedence over analyzed values.</p>
          <h3 id="33-core-runtime-settings">3.3 Core Runtime Settings</h3>
          <h4 id="context-window">Context window</h4>
          <p><code>context-window</code> is the maximum number of tokens available to one sequence. Input and generated tokens both consume this capacity.</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/Qwen3-0.6B-Q8_0:
  context-window: 32768`}</code></pre>
          <p>A larger window increases KV-cache memory and can reduce the number of parallel sequences that fit. It also cannot create model capability that was absent during training. If the requested window exceeds the model's native context, the model may require RoPE scaling; see <a href="chapter-07-yarn-extended-context.md">Chapter 7</a>.</p>
          <h4 id="kv-cache-types">KV cache types</h4>
          <p>The KV cache stores attention state for tokens already processed. Configure the key and value caches independently:</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/Qwen3-0.6B-Q8_0:
  cache-type-k: q8_0
  cache-type-v: q8_0`}</code></pre>
          <p>Common choices are:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Value</th>
                <th>Meaning</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>f16</code></td>
                <td>Higher precision and larger cache</td>
              </tr>
              <tr>
                <td><code>q8_0</code></td>
                <td>Smaller quantized cache</td>
              </tr>
              <tr>
                <td><code>q4_0</code></td>
                <td>More aggressive compression</td>
              </tr>
            </tbody>
          </table>
          <p>The actual memory reduction includes block and alignment overhead, so it is not an exact ratio for every model and backend. Start with automatic tuning. If an explicit quantized cache changes output quality, compare the same workload with <code>f16</code> before changing unrelated settings.</p>
          <h4 id="flash-attention">Flash Attention</h4>
          <p>Flash Attention can reduce attention memory traffic and improve performance, especially at longer contexts:</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/Qwen3-0.6B-Q8_0:
  flash-attention: auto`}</code></pre>
          <p>Valid values are <code>enabled</code>, <code>disabled</code>, and <code>auto</code>. Automatic tuning uses <code>auto</code> when a GPU is available and disables it for CPU-only analysis. Set an explicit value only for backend compatibility or controlled benchmarking.</p>
          <h4 id="sliding-window-attention">Sliding Window Attention</h4>
          <p>Kronk reads the sliding-window size from model metadata. <code>swa-full</code> controls the cache allocation used by models with sliding window attention:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/some-swa-model:
  swa-full: false`}</code></pre>
          <p>When unset, llama.cpp currently uses a full-size SWA cache. Explicitly setting <code>false</code> uses the compact sliding-window cache, which can save memory but limits context caching and shifting. Setting <code>true</code> preserves the full cache at a higher memory cost. This key has no effect on models without SWA metadata.</p>
          <h3 id="34-gpu-and-memory-placement">3.4 GPU and Memory Placement</h3>
          <p>Automatic tuning normally places model layers and operations. Use these settings when a model does not fit or when a multi-GPU deployment needs an explicit layout.</p>
          <h4 id="model-layers">Model layers</h4>
          <p><code>ngpu-layers</code> controls how many model layers are offloaded to the GPU:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/some-model:
  ngpu-layers: 20`}</code></pre>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Value</th>
                <th>Behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>0</code></td>
                <td>Offload all layers to the GPU</td>
              </tr>
              <tr>
                <td><code>-1</code></td>
                <td>Keep all layers on the CPU</td>
              </tr>
              <tr>
                <td>Positive integer</td>
                <td>Offload that many layers</td>
              </tr>
            </tbody>
          </table>
          <p>Partial offload can make a model fit in limited VRAM, but CPU-resident layers usually reduce inference speed. On unified-memory systems, CPU and GPU do not have separate memory pools, although placement can still affect performance.</p>
          <h4 id="kv-cache-and-operations">KV cache and operations</h4>
          <p>The KV cache and host tensor operations are offloaded to the GPU by default:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/some-model:
  offload-kqv: false
  op-offload: false`}</code></pre>
          <p><code>offload-kqv: false</code> keeps the KV cache on the CPU. <code>op-offload: false</code> keeps host tensor operations on the CPU. These options can reduce discrete-GPU VRAM pressure at a performance cost. They do not reduce total memory requirements.</p>
          <p>For multimodal models, <code>proj-on-cpu: true</code> keeps the media projector on the CPU without changing placement of the language model itself.</p>
          <h4 id="multiple-gpus">Multiple GPUs</h4>
          <p><code>split-mode</code> accepts:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Value</th>
                <th>Behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>none</code></td>
                <td>Use one GPU</td>
              </tr>
              <tr>
                <td><code>layer</code></td>
                <td>Distribute whole layers across GPUs</td>
              </tr>
              <tr>
                <td><code>row</code></td>
                <td>Split tensor rows across GPUs</td>
              </tr>
            </tbody>
          </table>
          <p>When the setting is omitted, Kronk selects <code>row</code> when more than one GPU is present and <code>layer</code> otherwise. This is a hardware-derived default, not a rule that one mode is always fastest for a particular model architecture.</p>
          <p>For explicit placement, <code>devices</code> names the devices and <code>tensor-split</code> gives their proportional shares:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/some-model:
  devices: [CUDA0, CUDA1]
  split-mode: layer
  tensor-split: [0.6, 0.4]`}</code></pre>
          <p>The number of <code>tensor-split</code> values must match the number of devices. Omit the split to let the backend derive it from available memory. <code>main-gpu</code> selects the primary device when <code>split-mode</code> is <code>none</code>.</p>
          <h3 id="35-concurrency-and-batching">3.5 Concurrency and Batching</h3>
          <p><code>nseq-max</code> controls model concurrency:</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/Qwen3-0.6B-Q8_0:
  nseq-max: 4`}</code></pre>
          <p>For text generation, this creates up to four batch-engine slots. Their sequence state is isolated, while the text engine uses a unified KV pool with total capacity based on <code>context-window × nseq-max</code>. Idle slots do not own permanent fixed partitions, but increasing <code>nseq-max</code> still increases the capacity Kronk must budget and can substantially increase memory use.</p>
          <p>Embedding and reranking models use <code>nseq-max</code> to size a pool of independent contexts rather than text-generation slots. See <a href="chapter-04-batch-processing.md">Chapter 4</a> for request scheduling and the differences between model types.</p>
          <p>Two settings control prompt batching:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Key</th>
                <th>Load-time default</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>nubatch</code></td>
                <td><code>2048</code>; <code>4096</code> with MoE expert CPU offload</td>
                <td>Physical compute chunk size</td>
              </tr>
              <tr>
                <td><code>nbatch</code></td>
                <td><code>nubatch × nseq-max</code></td>
                <td>Maximum logical decode batch</td>
              </tr>
            </tbody>
          </table>
          <p>Most deployments should leave both unset. Larger values can improve prompt throughput but require larger compute buffers. <code>nubatch</code> must not exceed <code>nbatch</code>. Multimodal encoders may require an entire media token chunk to fit in one <code>nubatch</code>, so do not lower it for a multimodal model without testing media input.</p>
          <p>Incremental Message Caching is configured separately with <code>incremental-cache</code> and related cache settings. See <a href="chapter-05-message-caching.md">Chapter 5</a> rather than treating cached conversations as dedicated physical slots.</p>
          <h3 id="36-memory-planning-and-quantization">3.6 Memory Planning and Quantization</h3>
          <p>Model memory is not just the GGUF file size plus a simple KV-cache formula. Depending on the model and backend, memory use can include:</p>
          <ul>
            <li>model weights placed on each device;</li>
            <li>KV cache or recurrent state;</li>
            <li>compute and output buffers;</li>
            <li>multimodal projector weights and buffers;</li>
            <li>speculative drafter weights and state; and</li>
            <li>backend allocations and safety margins.</li>
          </ul>
          <p>Context length, <code>nseq-max</code>, cache precision, layer placement, SWA, and model architecture all affect the result. Use <strong>Apps → VRAM Calculator</strong> in the BUI to inspect the GGUF metadata and estimate a specific configuration. Treat the result as planning guidance and retain headroom for the backend and other processes.</p>
          <p>If a configuration does not fit, consider these changes one at a time:</p>
          <ol>
            <li>Reduce <code>context-window</code>.</li>
            <li>Reduce <code>nseq-max</code>.</li>
            <li>Let automatic tuning use q8_0, or explicitly compare a quantized KV cache.</li>
            <li>Move the KV cache or some model layers to CPU.</li>
            <li>Choose a smaller or more heavily quantized GGUF.</li>
          </ol>
          <h4 id="weight-quantization-versus-kv-cache-quantization">Weight quantization versus KV-cache quantization</h4>
          <p>The quantization in a GGUF filename describes the model's stored weights. It is selected when downloading the model and cannot be changed in <code>model_config.yaml</code>. Lower-bit files generally use less storage and memory, but the quality and speed trade-offs depend on the model, quantizer, and hardware. Parameter count alone is not enough to predict whether a model fits.</p>
          <p><code>cache-type-k</code> and <code>cache-type-v</code> quantize runtime attention state instead. They do not change model weights. Evaluate weight format and KV-cache format as separate choices.</p>
          <h3 id="37-advanced-features">3.7 Advanced Features</h3>
          <h4 id="speculative-decoding-and-mtp">Speculative decoding and MTP</h4>
          <p>Kronk supports a separate draft GGUF and Multi-Token Prediction (MTP). MTP may be embedded in the target GGUF or supplied as a model-specific companion file that Kronk's catalog and download flow associates with the target. A separate classic draft must already be downloaded, must have a compatible vocabulary, and requires <code>nseq-max: 1</code>:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/target-model:
  nseq-max: 1
  draft-model:
    model-id: some-provider/compatible-draft-model
    ndraft: 5`}</code></pre>
          <p>MTP is detected automatically from the downloaded target and its companion files. To override only its starting draft-token count, omit <code>model-id</code>:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/mtp-target-model:
  draft-model:
    ndraft: 6`}</code></pre>
          <p>Do not use model names or benchmark results as universal draft-selection rules. Measure acceptance and throughput on the actual workload. See <a href="chapter-06-speculative-decoding-mtp.md">Chapter 6</a> for drafter selection, adaptive throttling, observability, and limitations.</p>
          <h4 id="extended-context-with-yarn">Extended context with YaRN</h4>
          <p>Do not add RoPE scaling merely because a large <code>context-window</code> fits in memory. Scaling must match the model and its native training context. Configuration uses <code>rope-scaling-type</code> and the <code>yarn-*</code> keys described in <a href="chapter-07-yarn-extended-context.md">Chapter 7</a>.</p>
          <h4 id="per-model-sampling-defaults">Per-model sampling defaults</h4>
          <p><code>sampling-parameters</code> supplies defaults for requests using one model:</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/Qwen3-0.6B-Q8_0:
  sampling-parameters:
    temperature: 0.7
    top_p: 0.8
    top_k: 20`}</code></pre>
          <p>The nested keys use snake_case because they match request parameter names. Clients can provide request-specific values. See <a href="chapter-10-request-parameters.md">Chapter 10</a> for behavior and the full parameter reference.</p>
          <h3 id="38-complete-example-and-key-reference">3.8 Complete Example and Key Reference</h3>
          <p>This example shows the file structure and naming conventions. It is not a recommendation that every model needs these overrides:</p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/models/model_config.yaml

unsloth/Qwen3-0.6B-Q8_0:
  context-window: 32768
  nseq-max: 2
  cache-type-k: q8_0
  cache-type-v: q8_0
  flash-attention: auto
  incremental-cache: true
  sampling-parameters:
    temperature: 0.7
    top_p: 0.8

unsloth/Qwen3-0.6B-Q8_0/LONG:
  context-window: 65536
  nseq-max: 1

some-provider/large-model:
  context-window: 16384
  ngpu-layers: 20
  offload-kqv: false`}</code></pre>
          <p>Common top-level keys are summarized below. An omitted hardware-related value is normally supplied by analysis or by the load-time defaults.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Key</th>
                <th>Values</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>context-window</code></td>
                <td>Positive token count</td>
                <td>Per-sequence context capacity</td>
              </tr>
              <tr>
                <td><code>cache-type-k</code>, <code>cache-type-v</code></td>
                <td><code>f16</code>, <code>q8_0</code>, <code>q4_0</code>, and supported GGML types</td>
                <td>KV-cache precision</td>
              </tr>
              <tr>
                <td><code>flash-attention</code></td>
                <td><code>enabled</code>, <code>disabled</code>, <code>auto</code></td>
                <td>Attention implementation mode</td>
              </tr>
              <tr>
                <td><code>nseq-max</code></td>
                <td>Positive integer</td>
                <td>Parallel sequences or context-pool size</td>
              </tr>
              <tr>
                <td><code>nubatch</code>, <code>nbatch</code></td>
                <td>Positive token counts</td>
                <td>Physical and logical batch sizes</td>
              </tr>
              <tr>
                <td><code>ngpu-layers</code></td>
                <td><code>-1</code>, <code>0</code>, or a positive count</td>
                <td>CPU/GPU layer placement</td>
              </tr>
              <tr>
                <td><code>offload-kqv</code></td>
                <td>Boolean</td>
                <td>Place KV cache on GPU when true</td>
              </tr>
              <tr>
                <td><code>op-offload</code></td>
                <td>Boolean</td>
                <td>Place host tensor operations on GPU when true</td>
              </tr>
              <tr>
                <td><code>proj-on-cpu</code></td>
                <td>Boolean</td>
                <td>Keep multimodal projector on CPU</td>
              </tr>
              <tr>
                <td><code>devices</code></td>
                <td>Device-name list</td>
                <td>Devices available to the model</td>
              </tr>
              <tr>
                <td><code>split-mode</code></td>
                <td><code>none</code>, <code>layer</code>, <code>row</code></td>
                <td>Multi-GPU distribution mode</td>
              </tr>
              <tr>
                <td><code>main-gpu</code></td>
                <td>Device index</td>
                <td>Primary device in single-GPU mode</td>
              </tr>
              <tr>
                <td><code>tensor-split</code></td>
                <td>Numeric share list</td>
                <td>Proportional multi-GPU placement</td>
              </tr>
              <tr>
                <td><code>swa-full</code></td>
                <td>Boolean</td>
                <td>Full or compact SWA cache</td>
              </tr>
              <tr>
                <td><code>incremental-cache</code></td>
                <td>Boolean</td>
                <td>Incremental Message Cache</td>
              </tr>
              <tr>
                <td><code>draft-model</code></td>
                <td>Mapping</td>
                <td>Separate drafter or MTP draft-count override</td>
              </tr>
              <tr>
                <td><code>rope-scaling-type</code></td>
                <td>Supported scaling mode</td>
                <td>Extended-context scaling</td>
              </tr>
              <tr>
                <td><code>sampling-parameters</code></td>
                <td>Mapping</td>
                <td>Per-model generation defaults</td>
              </tr>
              <tr>
                <td><code>template</code></td>
                <td>File path</td>
                <td>Override the model's chat template</td>
              </tr>
            </tbody>
          </table>
          <p>Prefer the automatic values until a measured workload gives you a reason to override them. Change one setting at a time so memory, quality, and throughput effects remain attributable.</p>
          <hr />
          <h2 id="chapter-4-batch-processing">Chapter 4: Batch Processing</h2>
          <p>Kronk can process requests concurrently while sharing one loaded copy of a model's weights. The <code>nseq-max</code> model setting controls how much concurrency a model instance provides, but its exact behavior depends on the model's task.</p>
          <p>This chapter covers user-visible scheduling and configuration. Model memory, batch sizes, and KV-cache precision are covered in <a href="chapter-03-model-configuration.md">Chapter 3</a>. Message-cache session behavior is covered in <a href="chapter-05-message-caching.md">Chapter 5</a>.</p>
          <h3 id="41-concurrency-at-a-glance">4.1 Concurrency at a Glance</h3>
          <p>Kronk uses two concurrency designs:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Workload</th>
                <th><code>nseq-max</code> controls</th>
                <th>Execution design</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Text generation</td>
                <td>Active generation slots</td>
                <td>One model context and a shared batch engine</td>
              </tr>
              <tr>
                <td>Multimodal generation</td>
                <td>Active generation slots</td>
                <td>The same batch engine, with specialized media prefill</td>
              </tr>
              <tr>
                <td>Embedding</td>
                <td>Independent contexts</td>
                <td>Context pool with shared model weights</td>
              </tr>
              <tr>
                <td>Reranking</td>
                <td>Independent contexts</td>
                <td>Context pool with shared model weights</td>
              </tr>
            </tbody>
          </table>
          <p>Multimodal generation includes requests that provide images or audio to a compatible language model. Bucky speech transcription is a separate whisper.cpp service and is not scheduled by this batch engine; see <a href="chapter-18-bucky.md">Chapter 18</a>.</p>
          <p>Increasing <code>nseq-max</code> allows more work to proceed concurrently. It can improve aggregate throughput when requests overlap, but it also increases memory capacity and gives each request a smaller share of the same compute resources. Higher concurrency can therefore increase individual response latency. There is no universal value that is best for every model, device, and workload.</p>
          <h3 id="42-generation-slots-and-sequences">4.2 Generation Slots and Sequences</h3>
          <p>For text and multimodal generation, the batch engine creates <code>nseq-max</code> execution slots. A slot tracks one active request's prompt position, sampler, streaming response, and sequence ID.</p>
          <pre className="code-block"><code className="language-diagram">{`┌───────────────┐       ┌──────────────────────────────────┐
│ Waiting jobs  │──────▶│ Batch engine                     │
└───────────────┘       │                                  │
                        │  Slot 0 ── sequence 0 ── request A│
                        │  Slot 1 ── sequence 1 ── request B│
                        │  Slot 2 ── sequence 2 ── request C│
                        └────────────────┬─────────────────┘
                                         │
                                         ▼
                        ┌──────────────────────────────────┐
                        │ Shared model context and weights │
                        └──────────────────────────────────┘`}</code></pre>
          <p>Sequence IDs isolate attention state, so one request cannot attend to another request's tokens. They are not fixed physical KV-cache partitions. With more than one sequence, Kronk enables a unified KV pool whose total capacity is based on:</p>
          <pre className="code-block"><code className="language-text">{`context-window × nseq-max`}</code></pre>
          <p>Each slot is limited to one <code>context-window</code>, while unused capacity remains available to active sequences. Idle slots do not permanently own a slice of the pool. Even so, increasing <code>nseq-max</code> increases the total capacity Kronk must allocate and budget.</p>
          <p>When a request finishes, its slot becomes available for another waiting job. Scheduling uses the first available slot; jobs do not reserve a particular slot between requests.</p>
          <h3 id="43-admission-waiting-and-cancellation">4.3 Admission, Waiting, and Cancellation</h3>
          <p>The outer Kronk API applies the user-visible admission limit before a request reaches the model. For generation, its default capacity is <code>nseq-max × queue-depth</code>, where the default queue depth is 2.</p>
          <p>Internally, the batch engine receives admitted jobs through a bounded handoff channel and drains them into its pending-job list until slots become available. The channel is not a second user-visible queue budget. The direct Go SDK option <code>model.WithQueueDepth(n)</code> changes the outer admission multiplier; it does not resize that internal handoff channel. Embedding and reranking use an admission capacity of <code>nseq-max</code> rather than the queue-depth multiplier.</p>
          <p>At the default generation admission depth, <code>nseq-max: 4</code> permits up to eight requests through the outer admission gate. At most four can occupy execution slots at once; the remainder wait for a slot. Additional callers block at the admission gate until capacity is released.</p>
          <p>Waiting honors request cancellation. If a request's context is cancelled before admission or while submitting to the engine, the request returns that cancellation. During model shutdown, the engine rejects new submissions and finishes active and pending jobs with a shutdown error.</p>
          <p>The engine does <strong>not</strong> cancel a long-running request merely because another job has waited for a slot. Applications that require a maximum generation time should use request cancellation, server timeouts, or generation limits such as <code>max_tokens</code>.</p>
          <h3 id="44-prompt-and-token-scheduling">4.4 Prompt and Token Scheduling</h3>
          <p>Generation work moves through these stages:</p>
          <ol>
            <li>Prepare the request and plan any reusable cached state.</li>
            <li>Submit the job and wait for an execution slot.</li>
            <li>Restore or build cached state and tokenize or prefill remaining input.</li>
            <li>Generate and stream output tokens.</li>
            <li>Clear the active sequence and release the slot.</li>
          </ol>
          <p>Some preparation and IMC tokenization occurs before submission. Ordinary non-cached tokenization can occur when the slot starts. The exact boundary is an implementation detail; the visible queue wait begins around engine submission and ends when a slot is assigned.</p>
          <p>For ordinary text prefill, active slots contribute prompt tokens in round-robin chunks of up to <code>nubatch</code> tokens until the shared <code>nbatch</code> capacity is reached. This prevents one large prompt from consuming every prefill pass while other slots wait. Generated tokens from active slots can be processed in the same shared decode loop.</p>
          <p>Media input requires specialized encoder and prefill steps, so it is not always combined with text work in one forward pass. Multi-Token Prediction (MTP) also changes how some prefill and verification batches are formed. These special cases preserve the same user-visible slot limit but should not be treated as identical scheduling at the backend level.</p>
          <p>Most users should leave <code>nbatch</code> and <code>nubatch</code> unset. Kronk derives their load-time values as described in <a href="chapter-03-model-configuration.md#35-concurrency-and-batching">Chapter 3 §3.5</a>.</p>
          <h3 id="45-embedding-and-reranking">4.5 Embedding and Reranking</h3>
          <p>Embedding and reranking models do not use generation slots. Kronk creates a pool of <code>nseq-max</code> independent model contexts that share the model weights.</p>
          <pre className="code-block"><code className="language-diagram">{`┌──────────┐       ┌──────────────────────────────┐
│ Requests │──────▶│ Context pool                │
└──────────┘       │  Context 0 ── request A     │
                   │  Context 1 ── request B     │
                   │  Context 2 ── available     │
                   └──────────────────────────────┘`}</code></pre>
          <p>Each admitted request acquires one context, performs its work independently, and returns the context to the pool. If every context is busy, another request waits until one is released or its context is cancelled. Work from separate contexts is not combined into the generation engine's shared token batch.</p>
          <p>Additional contexts require memory even though model weights are shared. Raise <code>nseq-max</code> only when concurrent embedding or reranking traffic benefits from the extra contexts.</p>
          <h3 id="46-configuration-and-tuning">4.6 Configuration and Tuning</h3>
          <p>Configure concurrency in <code>~/.kronk/models/model_config.yaml</code>:</p>
          <pre className="code-block"><code className="language-yaml">{`mradermacher/Qwopus3.5-4B-Coder.Q8_0:
  context-window: 32768
  nseq-max: 2`}</code></pre>
          <p>The file is read at server startup. Restart the server after changing it. The top-level key must match the model ID used by requests.</p>
          <p>Tune from a measured baseline rather than a generic slot recommendation:</p>
          <ol>
            <li>Start with automatic tuning or <code>nseq-max: 1</code> for a controlled baseline.</li>
            <li>Run the expected number and shape of concurrent requests.</li>
            <li>Measure aggregate throughput, time to first token, queue wait, and memory.</li>
            <li>Increase <code>nseq-max</code> one step at a time while throughput improves acceptably.</li>
            <li>Stop when memory pressure, queueing, or per-request latency becomes worse than the workload can tolerate.</li>
          </ol>
          <p>If requests spend too long waiting for slots, possible responses include:</p>
          <ul>
            <li>increase <code>nseq-max</code> if the model and device have sufficient memory;</li>
            <li>reduce <code>context-window</code> when the workload does not need it;</li>
            <li>evaluate a smaller KV-cache type or a smaller model; or</li>
            <li>distribute traffic across more model-server instances.</li>
          </ul>
          <p>Do not treat weight size plus a hand-calculated KV value as total VRAM. Use the BUI's <strong>Apps → VRAM Calculator</strong> and retain operating headroom. See <a href="chapter-03-model-configuration.md#36-memory-planning-and-quantization">Chapter 3 §3.6</a> for the components that affect an estimate.</p>
          <h3 id="47-interaction-with-message-caching">4.7 Interaction with Message Caching</h3>
          <p>Incremental Message Caching (IMC) keeps reusable conversation state in a logical session, not in a permanently assigned execution slot. Cached state is externalized to a session store between requests. A later request can restore that state into any free slot, extend it, and continue generation.</p>
          <p>While a request is active, its restored or newly built state consumes cells in the unified KV pool. Kronk normally snapshots a built or extended stable prefix during slot startup, before generating the request's suffix. Exact read-only hits can skip a redundant snapshot. Completion clears the slot's active sequence. This allows the number of cached conversation identities to differ from the number of concurrent execution slots.</p>
          <p>If every IMC session has work pending, current token-based planning returns a server-busy error rather than preempting a generation already running in a batch slot.</p>
          <p>Session matching, RAM and disk stores, media caching, invalidation, and cache settings are documented in <a href="chapter-05-message-caching.md">Chapter 5</a>.</p>
          <h3 id="48-observing-queue-behavior">4.8 Observing Queue Behavior</h3>
          <p>Kronk records two direct indicators of generation-slot contention:</p>
          <ul>
            <li>the <code>queue-wait</code> trace span, which wraps the submit attempt and subsequent slot wait for successful jobs; and</li>
            <li>the <code>chat_queue_wait_seconds</code> Prometheus histogram, recorded when a slot is assigned.</li>
          </ul>
          <p>For a successful job, timing starts immediately before attempting submission to the batch engine and ends at slot assignment. It does not include time blocked at the outer SDK admission gate or time spent preparing an IMC session before the submit attempt. Compare it with end-to-end request duration and time-to-first-token measurements when diagnosing latency.</p>
          <p>Consistently increasing queue-wait time means requests are arriving faster than slots complete them. Before raising <code>nseq-max</code>, confirm that the device has memory headroom and that aggregate throughput improves under a realistic concurrent load. See <a href="chapter-15-observability.md">Chapter 15</a> for metrics, tracing, and profiling.</p>
          <hr />
          <h2 id="chapter-5-message-caching">Chapter 5: Message Caching</h2>
          <h2 id="51-what-imc-does">5.1 What IMC Does</h2>
          <p>Incremental Message Cache (IMC) reduces repeated prompt processing in multi-turn conversations. Without IMC, the model must prefill the complete conversation before generating each response. With IMC, Kronk can restore a previously processed prompt prefix and prefill only the new portion.</p>
          <p>IMC is enabled by default for generation models. It is most useful for:</p>
          <ul>
            <li>Long-running chat and coding-agent conversations</li>
            <li>Tool-calling workflows that append results to the existing history</li>
            <li>Multiple agents or conversation branches sharing one model</li>
            <li>Prompts with expensive media that remains unchanged across follow-up turns</li>
          </ul>
          <p>Short, one-shot prompts generally gain little from caching. IMC also performs host-side rendering, tokenization, snapshot, and restore work, so it is not a replacement for choosing an appropriate context window and concurrency level.</p>
          <p>For text requests, Kronk creates two complete prompt renderings:</p>
          <ol>
            <li>A <strong>stable rendering</strong> without the generation prompt. This is the reusable prefix stored by IMC.</li>
            <li>A <strong>generation-ready rendering</strong> used for inference. This includes a nonempty tail after the stable prefix.</li>
          </ol>
          <p>The stable tokens must be a prefix of the generation-ready tokens. This lets Kronk reuse a complete, template-valid conversation rather than rendering an independent suffix that might have different template semantics.</p>
          <p>The <code>cache-min-tokens</code> setting controls the minimum stable-render token length required to create or reuse an IMC session. Its default is 100. Requests below the threshold still work, but Kronk processes the complete generation-ready prompt without IMC.</p>
          <h2 id="52-how-kronk-reuses-a-text-prefix">5.2 How Kronk Reuses a Text Prefix</h2>
          <p>Kronk compares the complete stable token sequence with sequences retained by existing sessions. The result is one of three match types:</p>
          <ul>
            <li><strong>Exact</strong> — The new stable sequence is identical to a cached sequence. Kronk restores that session and processes only the generation-ready tail.</li>
            <li><strong>Append</strong> — A cached sequence is a complete prefix of the new stable sequence. Kronk restores it, processes the appended stable tokens, and then processes the generation-ready tail.</li>
            <li><strong>Rebuild</strong> — No complete cached sequence prefixes the new stable sequence. Kronk uses an empty session or replaces the least recently used available session and processes the stable prefix from the beginning.</li>
          </ul>
          <p>Only complete-prefix reuse is allowed. If an earlier message is edited, removed, reordered, or rendered differently, Kronk rebuilds the prefix. It does not trim an existing session at an internal point and attempt to salvage the tokens before the divergence.</p>
          <p>For example:</p>
          <pre className="code-block"><code className="language-text">{`Cached stable tokens: [A B C D]

New stable tokens:    [A B C D]       -> exact
New stable tokens:    [A B C D E F]   -> append E F
New stable tokens:    [A B X D]       -> rebuild`}</code></pre>
          <p>This comparison uses rendered tokens, not only the message objects supplied by the client. Changes to the chat template, tool definitions, thinking options, or other inputs that affect rendering can therefore prevent reuse even when the visible message text appears unchanged.</p>
          <h2 id="53-sessions-slots-and-snapshots">5.3 Sessions, Slots, and Snapshots</h2>
          <p>An IMC <strong>session</strong> is a reusable conversation identity and its saved model state. An execution <strong>slot</strong> is a lane that can actively run a request. These are deliberately separate:</p>
          <ul>
            <li>Kronk retains up to <code>nseq-max × 3</code> IMC sessions.</li>
            <li>Only <code>nseq-max</code> requests can decode concurrently.</li>
            <li>A session can be restored into any available execution slot; it is not tied permanently to one slot.</li>
            <li>Session storage is allocated lazily as conversations begin using it.</li>
          </ul>
          <p>For example, <code>nseq-max: 2</code> provides two concurrent decode slots and up to six warm IMC session identities. Raising <code>nseq-max</code> also increases the unified KV cache capacity and its memory cost, so do not raise it solely to retain more conversation branches without considering the effects described in <a href="chapter-04-batch-processing.md">Chapter 4</a>.</p>
          <p>Kronk reserves a session as soon as it selects it for an exact match, append, or rebuild. Other requests cannot select that identity while the reservation is pending. If all session identities are pending, the request returns a busy error and should be retried. Kronk does not evict an active session to make room.</p>
          <p>During a request, Kronk restores the selected snapshot into a free slot. For a new or appended stable prefix, it creates the updated snapshot after processing the stable tokens. The generation-ready tail is then processed without making it part of that reusable stable prefix.</p>
          <p>An exact match may skip rewriting the snapshot when the stable state has not changed. This avoids an unnecessary serialization of the state that was just restored. Exact media-plan reuse can receive the same optimization. These are implementation optimizations; they do not change which content is considered part of the cache.</p>
          <p>Snapshots externalize inactive session state from the model's active KV cache. They therefore do not permanently occupy an execution slot or pin their state in accelerator KV memory between requests. They do consume host or disk storage, as described in <a href="#55-configuration-and-storage">Configuration and Storage</a>.</p>
          <h2 id="54-media-requests">5.4 Media Requests</h2>
          <p>IMC supports media processed by Kronk's multimodal pipeline. Instead of relying only on text-token equality, Kronk builds a logical plan containing the ordered text and media inputs.</p>
          <p>Kronk can reuse a media session in two cases:</p>
          <ul>
            <li><strong>Exact plan</strong> — The complete stable media plan is unchanged.</li>
            <li><strong>Text extension from an anchor</strong> — The stored media plan remains unchanged and is followed only by new text. Kronk restores the media state and processes the text extension without encoding the media again.</li>
          </ul>
          <p>Kronk rebuilds the stable plan when media is changed, reordered, removed, or newly appended. This conservative rule lets the model-specific multimodal pipeline remain authoritative for media embeddings, token placement, and position handling.</p>
          <p>For example, a user can submit an image and then ask several text-only follow-up questions. The saved media plan acts as an anchor for those turns. Replacing the image or adding another one requires a rebuild.</p>
          <p>See <a href="chapter-11-multi-modal-models.md">Chapter 11</a> for supported media inputs and model requirements.</p>
          <h2 id="55-configuration-and-storage">5.5 Configuration and Storage</h2>
          <p>IMC settings belong under the model ID in <code>~/.kronk/models/model_config.yaml</code>:</p>
          <pre className="code-block"><code className="language-yaml">{`Qwen/Qwen3-8B-Q8_0:
  incremental-cache: true
  cache-min-tokens: 100
  session-store-kind: ram`}</code></pre>
          <p>The relevant settings are:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Setting</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>incremental-cache</code></td>
                <td><code>true</code></td>
                <td>Enables IMC for the model.</td>
              </tr>
              <tr>
                <td><code>cache-min-tokens</code></td>
                <td><code>100</code></td>
                <td>Minimum stable-render length required to create or reuse a session.</td>
              </tr>
              <tr>
                <td><code>session-store-kind</code></td>
                <td><code>ram</code></td>
                <td>Stores inactive session snapshots in <code>ram</code> or on <code>disk</code>.</td>
              </tr>
              <tr>
                <td><code>session-store-dir</code></td>
                <td>None</td>
                <td>Existing writable directory required by the <code>disk</code> store.</td>
              </tr>
            </tbody>
          </table>
          <p>Set <code>incremental-cache: false</code> if a workload is entirely short-lived or if you need to compare behavior without prompt caching.</p>
          <h3 id="ram-storage">RAM storage</h3>
          <p>The default <code>ram</code> store keeps snapshots in process memory. Each session buffer grows as needed and retains its peak allocation for reuse. Actual memory use depends on the model, cached conversation lengths, KV data types, and number of sessions that have been used. Budget for peak conversation state across the branches you expect to keep warm, not just the <code>nseq-max</code> requests that can run simultaneously.</p>
          <h3 id="disk-storage">Disk storage</h3>
          <p>To place inactive snapshots on disk:</p>
          <pre className="code-block"><code className="language-yaml">{`Qwen/Qwen3-8B-Q8_0:
  incremental-cache: true
  session-store-kind: disk
  session-store-dir: /var/lib/kronk/sessions`}</code></pre>
          <p>The directory must already exist and be writable by the Kronk process. Kronk creates a temporary file for each used session and removes it during a normal model unload. Files can remain after a process crash, so use a dedicated directory and arrange cleanup appropriate for your deployment.</p>
          <p>Disk storage changes where inactive snapshots are retained, but it does not eliminate snapshot-sized RAM usage. Snapshot and restore operations require memory buffers, and a session can retain buffers sized to its largest state. Disk also adds I/O latency. Measure both memory and request latency with your model and storage device before relying on it as a capacity solution.</p>
          <p>Some MTP configurations maintain draft-model cached state and saved hidden state in addition to the target model snapshot. Account for this extra storage when sizing memory. See <a href="chapter-06-speculative-decoding-mtp.md">Chapter 6</a> for MTP configuration and behavior.</p>
          <h2 id="56-invalidation-and-limitations">5.6 Invalidation and Limitations</h2>
          <p>IMC favors safe reuse over partial recovery. A session is rebuilt when Kronk cannot prove that its complete saved prefix matches the new stable prompt. Common causes include:</p>
          <ul>
            <li>Editing, deleting, or reordering earlier conversation content</li>
            <li>Changing tools or settings that alter the rendered prompt</li>
            <li>Changing, adding, removing, or reordering media</li>
            <li>Loading a different model or an incompatible model configuration</li>
            <li>Producing a stable rendering that is not a prefix of the generation-ready rendering</li>
          </ul>
          <p>An unload or server restart clears in-memory sessions. The disk store is an inactive snapshot backend, not a persistent conversation database; do not rely on IMC sessions surviving model or process lifecycles.</p>
          <p>IMC has several practical costs:</p>
          <ul>
            <li>Planning text reuse requires rendering and tokenizing complete prompts.</li>
            <li>Snapshot and restore operations use host memory bandwidth and, for disk storage, filesystem I/O.</li>
            <li>Edited text rebuilds instead of reusing an arbitrary partial prefix.</li>
            <li>The session pool is finite, so inactive least-recently-used branches can be replaced as new branches arrive.</li>
            <li>MTP can require additional draft-side state. If Kronk restores the target prefix without compatible draft state, it can still use the target cache but disables speculative decoding for that request.</li>
          </ul>
          <p>Evaluate IMC using a representative conversation workload rather than a single prompt benchmark. The benefit grows with reusable prefix length and follow-up frequency.</p>
          <h2 id="57-observability">5.7 Observability</h2>
          <p>At debug log level, IMC planning events identify the selected <code>match_kind</code> (<code>exact</code>, <code>append</code>, or <code>rebuild</code>) and report reusable, extension, stable, and tail token counts. Media planning events similarly identify exact, anchor, and rebuild decisions. Request-completion events include whether IMC participated and whether a prior snapshot was restored.</p>
          <p>The Prometheus counters <code>imc_snapshot_skipped_total</code> and <code>imc_pure_hit_stale_session_total</code> expose exact-hit snapshot skips and rejected stale-session races. A rising rebuild rate usually means clients are changing earlier prompt content, media, tools, or rendering inputs rather than appending to a stable conversation.</p>
          <p>See <a href="chapter-15-observability.md">Chapter 15</a> for logging, metrics, tracing, and profiling configuration.</p>
          <h2 id="chapter-6-speculative-decoding-and-mtp">Chapter 6: Speculative Decoding and MTP</h2>
          <h3 id="61-what-speculative-decoding-does">6.1 What Speculative Decoding Does</h3>
          <p>Speculative decoding uses a faster drafter to propose several continuation tokens. The target model verifies those proposals together. Accepted proposals reduce the number of target-model passes needed to produce the response; rejected proposals are discarded and the target remains authoritative.</p>
          <p>This optimization does not change the chat, Responses, or SDK request shapes. It can improve generation throughput when the drafter is inexpensive and its proposals agree frequently with the target. It can also reduce performance when draft work is expensive or acceptance is poor. Always measure with the model, sampling settings, hardware, and prompts used in production.</p>
          <p>Kronk supports classic speculative decoding with a separate draft model and Multi-Token Prediction (MTP). MTP uses a prediction head designed for the target rather than a general-purpose smaller language model.</p>
          <h3 id="62-drafter-sources-and-selection">6.2 Drafter Sources and Selection</h3>
          <p>Kronk can load a drafter from three sources:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Source</th>
                <th>How it is supplied</th>
                <th>Slots</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><strong>Classic separate draft</strong></td>
                <td>A <code>draft-model</code> configuration names another compatible GGUF.</td>
                <td>Requires <code>nseq-max: 1</code></td>
              </tr>
              <tr>
                <td><strong>Companion MTP assistant</strong></td>
                <td>A model-specific assistant GGUF, currently used by Gemma4 models, is discovered with the downloaded target.</td>
                <td>Supports multiple slots</td>
              </tr>
              <tr>
                <td><strong>Embedded MTP head</strong></td>
                <td>The target GGUF contains supported <code>nextn_predict_layers</code> metadata, currently used by Qwen3.5/Qwen3.6 models.</td>
                <td>Supports multiple slots</td>
              </tr>
            </tbody>
          </table>
          <p>Kronk checks these sources in that order. A <code>draft-model</code> block containing a <code>model-id</code> explicitly selects the classic separate draft and takes precedence over either MTP form. Without one, Kronk uses a compatible companion MTP file when present, then checks the target for an embedded MTP head. If no source is available, the model runs normally without speculation.</p>
          <p>A <code>draft-model</code> block containing only <code>ndraft</code> is different: it changes the MTP draft ceiling and does not select a classic draft or disable MTP.</p>
          <p>MTP also requires support from the loaded llama.cpp library. When a model advertises MTP but the required API is unavailable, Kronk reports that MTP was disabled at model load and serves the model without speculation.</p>
          <h3 id="63-choosing-a-drafter">6.3 Choosing a Drafter</h3>
          <h4 id="631-classic-separate-draft">6.3.1 Classic separate draft</h4>
          <p>Use a classic separate draft when the target has no supported MTP source or when you have already measured a compatible draft that performs well for your workload.</p>
          <p>The draft must use the same tokenizer and token IDs as the target. Kronk checks that their vocabulary sizes match, but equal sizes alone cannot prove complete tokenizer compatibility. Select models documented as compatible and test them together. Similar names, parameter counts, or model families are not sufficient evidence by themselves.</p>
          <p>A separate draft has additional model and KV-cache memory costs. It also limits the target entry to one execution slot. It is therefore a poor fit when:</p>
          <ul>
            <li>The target is already fast enough that drafting overhead dominates</li>
            <li>The workload needs <code>nseq-max</code> greater than 1</li>
            <li>No demonstrably tokenizer-compatible draft is available</li>
            <li>Measured throughput or latency is worse with drafting enabled</li>
          </ul>
          <p>Do not select a draft using a universal model-pair or quantization rule. Acceptance and cost can change substantially with sampling parameters and task type.</p>
          <h4 id="632-mtp">6.3.2 MTP</h4>
          <p>MTP is normally the simpler choice when the downloaded model provides a supported embedded or companion head. It is architecture-matched to its target, supports multiple execution slots, and does not require a <code>model-id</code> in the <code>draft-model</code> configuration.</p>
          <p>An embedded head requires no companion file. A companion MTP assistant is an additional model-specific file, but Kronk's catalog and download flow can discover and associate it with the target automatically. It is not configured as a classic <code>draft-model</code>.</p>
          <p>MTP availability is a property of the downloaded files and the loaded llama.cpp library. Naming a model “MTP” or adding an <code>ndraft</code> override cannot create an MTP head that is not present.</p>
          <h3 id="64-draft-size-and-adaptive-throttling">6.4 Draft Size and Adaptive Throttling</h3>
          <p><code>ndraft</code> is the maximum number of candidates the drafter attempts in one round. Larger values can save more target passes when acceptance remains high, but they also increase wasted draft and verification work when proposals are rejected.</p>
          <p>Defaults are:</p>
          <ul>
            <li><strong>Classic separate draft:</strong> 5</li>
            <li><strong>MTP:</strong> 2</li>
          </ul>
          <p>Kronk adapts the actual draft count independently for each execution slot. It tracks an exponential moving average (EMA) of recent acceptance and chooses the next round's size from the configured ceiling:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Acceptance EMA</th>
                <th>Next draft size</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Below 0.30</td>
                <td>Usually 0</td>
              </tr>
              <tr>
                <td>0.30 to below 0.50</td>
                <td>At most 1</td>
              </tr>
              <tr>
                <td>0.50 to below 0.70</td>
                <td>At most 2</td>
              </tr>
              <tr>
                <td>0.70 to below 0.85</td>
                <td>At most 3</td>
              </tr>
              <tr>
                <td>0.85 or higher</td>
                <td>Configured ceiling</td>
              </tr>
            </tbody>
          </table>
          <p>When the EMA is below 0.30, Kronk normally bypasses speculation. It performs a one-token recovery probe every 32 fully throttled rounds so a slot can detect that its workload has become predictable again.</p>
          <p>The EMA belongs to the execution slot and persists across requests assigned to that slot. A request can therefore begin with a reduced draft size after prior requests on the same slot had poor acceptance. This is expected adaptive behavior, not evidence that the configured ceiling was ignored.</p>
          <h3 id="65-configuration">6.5 Configuration</h3>
          <p>Configuration belongs under the target model ID in <code>~/.kronk/models/model_config.yaml</code>.</p>
          <h4 id="651-classic-separate-draft">6.5.1 Classic separate draft</h4>
          <pre className="code-block"><code className="language-yaml">{`some-provider/target-model:
  nseq-max: 1
  draft-model:
    model-id: some-provider/compatible-draft-model
    ndraft: 5`}</code></pre>
          <p>The target and draft must already be downloaded. Kronk resolves the configured draft model ID to its local files. A classic draft with <code>nseq-max</code> greater than 1 is rejected during configuration validation.</p>
          <h4 id="652-mtp-default">6.5.2 MTP default</h4>
          <p>No model configuration is required. Downloading a supported target and its catalog-provided companion files is sufficient for automatic detection.</p>
          <h4 id="653-mtp-draft-count-override">6.5.3 MTP draft-count override</h4>
          <p>To change the MTP ceiling, set <code>ndraft</code> without a <code>model-id</code>:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/mtp-target-model:
  draft-model:
    ndraft: 6`}</code></pre>
          <p>This form supports multiple slots. A value of 0 or an omitted value uses the MTP default of 2; a negative value is rejected. If neither a compatible companion nor an embedded MTP head is available, the override has no effect. The adaptive throttle can still select fewer candidates than the configured value.</p>
          <p>See <a href="chapter-03-model-configuration.md">Chapter 3</a> for the complete model configuration format.</p>
          <h3 id="66-measuring-the-result">6.6 Measuring the Result</h3>
          <p>Do not use acceptance rate alone to decide whether speculation helps. Review acceptance, coverage, throughput, latency, and resource use together.</p>
          <p>The response <code>usage</code> object can include:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Field</th>
                <th>Meaning</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>draft_tokens</code></td>
                <td>Candidate tokens proposed during the request</td>
              </tr>
              <tr>
                <td><code>draft_accepted_tokens</code></td>
                <td>Proposed tokens accepted by the target</td>
              </tr>
              <tr>
                <td><code>draft_acceptance_rate</code></td>
                <td>Accepted candidates divided by proposed candidates</td>
              </tr>
              <tr>
                <td><code>draft_coverage</code></td>
                <td>Fraction of output positions produced through speculative rounds</td>
              </tr>
              <tr>
                <td><code>draft_disable_reason</code></td>
                <td>Why MTP fell back to target-only execution during the request</td>
              </tr>
            </tbody>
          </table>
          <p>These fields use <code>omitempty</code>; zero-valued fields may be absent from JSON. A high acceptance rate with low coverage can mean speculation ran for only a small part of the response. Conversely, a moderate acceptance rate can still help when drafting is much cheaper than target decoding. Compare end-to-end latency or tokens per second against the same workload with no drafter.</p>
          <p>Final <code>chat-completion</code> logs use <code>acceptance_rate</code>, while batch-engine completion logs use <code>draft_acceptance_rate</code>. Both also report draft and accepted-token counts when a drafter is loaded. Common MTP disable reasons include:</p>
          <ul>
            <li><code>imc-hit</code> — target cache state was restored without compatible draft state</li>
            <li><code>media-mrope</code> — MTP is not enabled for that M-RoPE media request</li>
            <li><code>sync-error</code> — draft state could not be synchronized after verification</li>
          </ul>
          <p>Startup events under <code>draft-model</code>, <code>draft-model-mtp</code>, and <code>draft-model-mtp-shared</code> show which source loaded or why MTP was skipped. See <a href="chapter-15-observability.md">Chapter 15</a> for logging and metrics configuration.</p>
          <h3 id="67-limitations-and-fallbacks">6.7 Limitations and Fallbacks</h3>
          <ul>
            <li><strong>Classic separate drafts require one slot.</strong> Set <code>nseq-max: 1</code> on the target entry.</li>
            <li><strong>Tokenizer compatibility remains the user's responsibility.</strong> Kronk rejects unequal vocabulary sizes, but that check cannot establish identical token mappings or templates.</li>
            <li><strong>MTP at nonzero temperature is an approximation.</strong> MTP proposals are greedy, while target verification uses the request's sampler and accepts exact token matches. Sampling parameters still shape output, but this does not provide strict speculative-sampling distribution equivalence.</li>
            <li><strong>MTP can fall back per request.</strong> A synchronization or compatible-state problem disables MTP for the affected request while target-only generation continues. It does not make an incorrect draft token authoritative.</li>
            <li><strong>IMC may restore target state without draft state.</strong> Target-prefix reuse remains valid, but own-KV MTP runs target-only for that request when its draft snapshot is absent or cannot be restored. See <a href="chapter-05-message-caching.md">Chapter 5</a>.</li>
            <li><strong>Media support is conservative.</strong> Media projection and media prefill run on the target. Unsupported own-KV media combinations and all M-RoPE media requests run without MTP, although target IMC can remain active. See <a href="chapter-11-multi-modal-models.md">Chapter 11</a>.</li>
            <li><strong>Drafting consumes resources.</strong> A classic draft loads another model and KV cache. MTP heads and companion assistants also require compute and memory. Automatic detection does not guarantee a performance improvement.</li>
          </ul>
          <p>Implementation details for drafting, verification, state synchronization, and hybrid-model rollback belong in <a href="chapter-19-developer-guide.md#1912-mtp-internals">Chapter 19</a>.</p>
          <h2 id="chapter-7-yarn-extended-context">Chapter 7: YaRN Extended Context</h2>
          <h2 id="71-context-size-and-rope-scaling">7.1 Context Size and RoPE Scaling</h2>
          <p><code>context-window</code> sets the token capacity available to one sequence. Input, chat-template tokens, and generated output all consume this capacity.</p>
          <p>Increasing that value allocates more KV-cache space, but it does not by itself give the model reliable long-context behavior. Models using Rotary Position Embeddings (RoPE) may require a scaling method when the requested context exceeds the length for which the model was trained.</p>
          <p>YaRN is one RoPE scaling method. It applies different interpolation behavior across RoPE frequencies and adjusts attention scaling. Whether it works, and which factor to use, depends on the specific model. Do not enable it solely because a larger KV cache fits in memory.</p>
          <h2 id="72-when-to-use-yarn">7.2 When to Use YaRN</h2>
          <p>Use YaRN only when all of the following are true:</p>
          <ul>
            <li>The model's documentation explicitly supports YaRN or compatible RoPE scaling.</li>
            <li>The required input plus output exceeds the model's native context.</li>
            <li>The documented extension factor covers the context you need.</li>
            <li>The larger KV cache fits while leaving enough memory for the model and active requests.</li>
            <li>Representative long-context tests produce acceptable results.</li>
          </ul>
          <p>Avoid YaRN when the workload fits within the native context. llama.cpp uses static YaRN: the configured scale applies at short positions too and can reduce quality on shorter prompts. Do not assume that one model family's settings are valid for another model, even when both use RoPE.</p>
          <h2 id="73-qwen3-configuration">7.3 Qwen3 Configuration</h2>
          <p>The Qwen3-8B model documentation identifies 32,768 tokens as the native context and reports validation up to 131,072 tokens with YaRN. It recommends matching the scale to the context actually needed.</p>
          <p>For a 2× extension to 65,536 tokens:</p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/models/model_config.yaml
Qwen/Qwen3-8B-Q8_0:
  context-window: 65536
  rope-scaling-type: yarn
  rope-freq-scale: 0.5
  yarn-orig-ctx: 32768`}</code></pre>
          <p>For a 4× extension to 131,072 tokens:</p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/models/model_config.yaml
Qwen/Qwen3-8B-Q8_0:
  context-window: 131072
  rope-scaling-type: yarn
  rope-freq-scale: 0.25
  yarn-orig-ctx: 32768`}</code></pre>
          <p><code>rope-freq-scale</code> is the raw frequency multiplier, so it is the reciprocal of the extension factor:</p>
          <pre className="code-block"><code className="language-text">{`rope-freq-scale = native context / configured context`}</code></pre>
          <p>For this model, <code>0.5</code> represents a 2× extension and <code>0.25</code> represents a 4× extension. This is equivalent to llama.cpp's <code>--rope-scale 2</code> and <code>--rope-scale 4</code>, respectively.</p>
          <p>Kronk does not derive <code>rope-freq-scale</code> from <code>context-window</code>. If the setting is omitted, llama.cpp uses scaling metadata from the GGUF. That is correct only when the downloaded GGUF already contains the intended scale. Follow the model or GGUF provider's documentation rather than copying the Qwen3 values to an unrelated model.</p>
          <h2 id="74-scaling-types-and-parameters">7.4 Scaling Types and Parameters</h2>
          <p>Kronk accepts three <code>rope-scaling-type</code> values:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Value</th>
                <th>Behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>none</code></td>
                <td>Applies no RoPE scaling. It does not prevent a separately configured oversized context window.</td>
              </tr>
              <tr>
                <td><code>linear</code></td>
                <td>Applies the same interpolation scale across RoPE frequencies. Use only when the model documentation calls for it.</td>
              </tr>
              <tr>
                <td><code>yarn</code></td>
                <td>Applies YaRN frequency-dependent interpolation and attention scaling.</td>
              </tr>
            </tbody>
          </table>
          <p>The available controls are:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Setting</th>
                <th>Meaning when explicitly configured</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>rope-freq-base</code></td>
                <td>Overrides the model's RoPE base frequency. Normally leave this to GGUF metadata.</td>
              </tr>
              <tr>
                <td><code>rope-freq-scale</code></td>
                <td>Raw frequency multiplier; an extension factor of \(N\) uses \(1/N\) when the model's instructions require explicit scaling.</td>
              </tr>
              <tr>
                <td><code>yarn-orig-ctx</code></td>
                <td>Original context length used by the YaRN calculation.</td>
              </tr>
              <tr>
                <td><code>yarn-ext-factor</code></td>
                <td>Mix between interpolation and extrapolation.</td>
              </tr>
              <tr>
                <td><code>yarn-attn-factor</code></td>
                <td>Attention magnitude scaling.</td>
              </tr>
              <tr>
                <td><code>yarn-beta-fast</code></td>
                <td>YaRN low correction dimension.</td>
              </tr>
              <tr>
                <td><code>yarn-beta-slow</code></td>
                <td>YaRN high correction dimension.</td>
              </tr>
            </tbody>
          </table>
          <p>When these settings are omitted, Kronk leaves the llama.cpp or GGUF defaults in place. An omitted or YAML <code>null</code> value does not ask Kronk to calculate a value from the context ratio. Override the advanced YaRN factors only when the model provider supplies values or controlled evaluation shows they are needed.</p>
          <h2 id="75-memory-and-concurrency">7.5 Memory and Concurrency</h2>
          <p>KV-cache capacity grows approximately linearly with the context window. The actual size depends on the model architecture, layer count, KV heads, head dimensions, cache data types, backend, and alignment. With multiple generation slots, the unified KV pool has capacity based on <code>context-window × nseq-max</code>.</p>
          <p>If a long-context model does not fit, consider:</p>
          <ul>
            <li>reducing <code>nseq-max</code>;</li>
            <li>selecting supported quantized KV-cache types; or</li>
            <li>keeping the KV cache on the CPU with <code>offload-kqv: false</code>, at a likely performance cost.</li>
          </ul>
          <p>Do not rely on fixed memory figures from another model. Use Kronk's hardware analysis and observe actual memory consumption. See <a href="chapter-03-model-configuration.md">Chapter 3</a> for KV-cache configuration and <a href="chapter-04-batch-processing.md">Chapter 4</a> for concurrency effects.</p>
          <h2 id="76-validate-quality">7.6 Validate Quality</h2>
          <p>An accepted configuration is not proof that the model can use the entire context reliably. Test the intended model and GGUF with representative data:</p>
          <ol>
            <li>Establish baseline quality within the native context.</li>
            <li>Test retrieval and reasoning at several positions near the target length.</li>
            <li>Reserve enough context for the expected generated output.</li>
            <li>Compare short-prompt quality with scaling enabled and disabled.</li>
            <li>Reduce the extension factor if quality or performance is unacceptable.</li>
          </ol>
          <p>Prefer the smallest context and scale that satisfy the workload. For Qwen3-8B, use the native context when average requests remain within 32,768 tokens, a 2× configuration for workloads around 65,536, and the documented 4× configuration only when requests genuinely require it.</p>
          <h2 id="chapter-8-model-server">Chapter 8: Model Server</h2>
          <p>The Kronk model server provides OpenAI-compatible inference APIs and manages downloaded models, native libraries, and loaded model instances. This chapter focuses on operating that server. Installation is covered in <a href="chapter-02-installation.md">Chapter 2</a>, while model-level tuning belongs in <a href="chapter-03-model-configuration.md">Chapter 3</a>.</p>
          <h2 id="81-server-lifecycle">8.1 Server Lifecycle</h2>
          <p>Start the server in the foreground:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start`}</code></pre>
          <p>The API listens on <code>0.0.0.0:11435</code> by default. <code>localhost:11435</code> works for a client on the same machine, but the server is bound to all network interfaces. Authentication is disabled by default. Before using an untrusted network, bind to loopback, restrict access with a firewall or private network, or enable the authentication described in <a href="chapter-12-security-authentication.md">Chapter 12</a>.</p>
          <p>To bind only to the local machine:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --api-host=127.0.0.1:11435`}</code></pre>
          <p>Run the server in the background with:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --detach`}</code></pre>
          <p>Detached mode records the process ID and redirects server output to these paths by default:</p>
          <pre className="code-block"><code className="language-text">{`~/.kronk/kronk.pid
~/.kronk/kronk.log`}</code></pre>
          <p>Setting <code>KRONK_BASE_PATH</code> before starting the detached server moves both files under that root.</p>
          <p>Use these commands for a detached server:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server logs
kronk server stop`}</code></pre>
          <p><code>server logs</code> follows the detached log file, and <code>server stop</code> signals the process recorded in the PID file. A foreground server logs to its terminal and should be stopped with the normal terminal or service-manager signal instead.</p>
          <p>The server handles <code>SIGINT</code> and <code>SIGTERM</code> and allows in-flight work to stop within the configured shutdown timeout.</p>
          <h2 id="82-local-and-server-backed-cli-commands">8.2 Local and Server-Backed CLI Commands</h2>
          <p>Model and catalog commands use the running server by default:</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list
kronk model pull unsloth/Qwen3-0.6B-Q8_0`}</code></pre>
          <p>The client connects to <code>localhost:11435</code> unless <code>KRONK_WEB_API_HOST</code> or the corresponding host flag selects another server.</p>
          <p>Add <code>--local</code> to operate directly on files and libraries without contacting a server:</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list --local
kronk model pull unsloth/Qwen3-0.6B-Q8_0 --local
kronk libs --local`}</code></pre>
          <p>Local mode is useful for initial setup, offline administration, and installing models into a stopped server's data directory. Use server-backed mode when administering a remote host or when browser progress needs to observe the operation.</p>
          <h2 id="83-essential-server-configuration">8.3 Essential Server Configuration</h2>
          <p>Common settings can be supplied as flags or environment variables:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Flag</th>
                <th>Environment variable</th>
                <th>Effective default</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>--api-host</code></td>
                <td><code>KRONK_WEB_API_HOST</code></td>
                <td><code>0.0.0.0:11435</code></td>
                <td>Main API bind address</td>
              </tr>
              <tr>
                <td><code>--debug-host</code></td>
                <td><code>KRONK_WEB_DEBUG_HOST</code></td>
                <td><code>0.0.0.0:11445</code></td>
                <td>Metrics and profiling bind address</td>
              </tr>
              <tr>
                <td><code>--base-path</code></td>
                <td><code>KRONK_BASE_PATH</code></td>
                <td><code>~/.kronk</code></td>
                <td>Root for Kronk data</td>
              </tr>
              <tr>
                <td><code>--model-config-file</code></td>
                <td><code>KRONK_POOL_MODEL_CONFIG_FILE</code></td>
                <td><code>&lt;base&gt;/models/model_config.yaml</code></td>
                <td>Per-model overrides</td>
              </tr>
              <tr>
                <td><code>--budget-percent</code></td>
                <td><code>KRONK_POOL_BUDGET_PERCENT</code></td>
                <td><code>80</code></td>
                <td>Memory-budget input for loaded models</td>
              </tr>
              <tr>
                <td><code>--models-in-pool</code></td>
                <td><code>KRONK_POOL_MODELS_IN_POOL</code></td>
                <td><code>10</code></td>
                <td>Maximum loaded entries in each model pool</td>
              </tr>
              <tr>
                <td><code>--pool-ttl</code></td>
                <td><code>KRONK_POOL_TTL</code></td>
                <td><code>20m</code></td>
                <td>Idle model retention time</td>
              </tr>
              <tr>
                <td><code>--web-admin-enabled</code></td>
                <td><code>KRONK_WEB_ADMIN_ENABLED</code></td>
                <td><code>true</code></td>
                <td>Serve the BUI under <code>/admin/</code></td>
              </tr>
              <tr>
                <td><code>--auth-enabled</code></td>
                <td><code>KRONK_AUTH_LOCAL_ENABLED</code></td>
                <td><code>false</code></td>
                <td>Protect inference and administration with local authentication</td>
              </tr>
              <tr>
                <td><code>--admin-auth-enabled</code></td>
                <td><code>KRONK_AUTH_ADMIN_ENABLED</code></td>
                <td><code>false</code></td>
                <td>Protect administration without requiring inference authentication</td>
              </tr>
              <tr>
                <td><code>--allow-upgrade</code></td>
                <td><code>KRONK_ALLOW_UPGRADE</code></td>
                <td><code>false</code></td>
                <td>Opt in to automatic native-library upgrades</td>
              </tr>
              <tr>
                <td><code>--llama-log</code></td>
                <td><code>KRONK_LLAMA_LOG</code></td>
                <td><code>1</code></td>
                <td>Enable or disable llama.cpp logging</td>
              </tr>
            </tbody>
          </table>
          <p>Most server configuration flags map to environment variables, but names follow the server's configuration hierarchy rather than a universal text conversion. For example, <code>--budget-percent</code> maps to <code>KRONK_POOL_BUDGET_PERCENT</code>. <code>--detach</code> is a CLI process-control flag and has no environment equivalent.</p>
          <p>Run the following for the complete current list, including HTTP timeouts, CORS, tracing, external authentication, processor selection, and library overrides:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --help`}</code></pre>
          <p>Keep tokens and passwords in protected environment or secret-manager settings, not shared shell scripts. <code>--insecure-logging</code> can expose prompts and model configuration and should be limited to controlled debugging.</p>
          <h2 id="84-model-pool-and-resource-budgets">8.4 Model Pool and Resource Budgets</h2>
          <p>Kronk keeps loaded models in memory to avoid paying model-load latency on every request. Three settings govern retention:</p>
          <ul>
            <li><code>budget-percent</code> controls memory admission.</li>
            <li><code>models-in-pool</code> places a count limit on each backend pool.</li>
            <li><code>pool-ttl</code> unloads entries that remain unused past the configured duration.</li>
          </ul>
          <p>At the default <code>budget-percent: 80</code>, each discrete GPU receives an 80% budget minus 256 MiB of headroom. Host RAM receives a 75% budget because Kronk reserves an additional five percentage points for the operating system, allocators, and memory not represented in model estimates. Apple Silicon unified memory is accounted as one host-memory pool rather than independent RAM and Metal VRAM.</p>
          <p>Admission uses predicted model, KV-cache, and runtime memory. These predictions are planning estimates, not a guarantee that every backend allocation will succeed. Context size, cache types, sequence count, CPU offload, and model architecture all affect the estimate.</p>
          <p>On multi-GPU systems, Kronk accounts for llama.cpp's model distribution across the selected devices. Automatic splits use available GPUs, while explicit <code>devices</code> and <code>tensor-split</code> configuration control the proportions. Each assigned share must fit within that GPU's individual budget; unused capacity on another card cannot satisfy an over-budget share.</p>
          <p>When a new load exceeds the count or memory budget, Kronk evicts an idle model. For memory pressure it prefers an idle entry that frees enough memory without unloading a needlessly large model, then falls back to the coldest idle entry. Models with active streams are not evicted. If no idle entry can make room, the request returns a server-busy error and the client should retry later.</p>
          <p>The Bucky and LLM pools share the same byte budget, so transcription and language-model loads can compete for memory. Bucky installation and pool behavior are covered in <a href="chapter-18-bucky.md">Chapter 18</a>.</p>
          <p>Resource usage and eviction events are available through the logging and metrics described in <a href="chapter-15-observability.md">Chapter 15</a>.</p>
          <h2 id="85-model-configuration-files">8.5 Model Configuration Files</h2>
          <p>The server reads per-model overrides from:</p>
          <pre className="code-block"><code className="language-text">{`~/.kronk/models/model_config.yaml`}</code></pre>
          <p>Kronk seeds the file on first use and preserves edits across upgrades. Entries are merged over hardware-analysis recommendations rather than replacing the entire runtime configuration.</p>
          <p>Use another file without replacing the default:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --model-config-file=./my-model_config.yaml`}</code></pre>
          <p>or:</p>
          <pre className="code-block"><code className="language-shell">{`KRONK_POOL_MODEL_CONFIG_FILE=./my-model_config.yaml kronk server start`}</code></pre>
          <p>The file format, variants, configuration keys, and tuning workflow are documented in <a href="chapter-03-model-configuration.md">Chapter 3</a>. The repository's commented reference file is <code>zarf/kms/model_config.yaml</code>.</p>
          <h2 id="86-catalog-operations">8.6 Catalog Operations</h2>
          <p>The personal model catalog is stored at <code>~/.kronk/catalog/catalog.yaml</code>. Kronk seeds it with a starter catalog and adds resolved model information as models are discovered or downloaded.</p>
          <p>Common operations are:</p>
          <pre className="code-block"><code className="language-shell">{`# List catalog entries and local validation state.
kronk catalog list

# Inspect one entry.
kronk catalog show unsloth/Qwen3-0.6B-Q8_0

# Download a model and reconcile its catalog metadata.
kronk model pull unsloth/Qwen3-0.6B-Q8_0

# Remove the catalog entry and its downloaded files.
kronk catalog remove unsloth/Qwen3-0.6B-Q8_0`}</code></pre>
          <p>Catalog entries identify the provider, source family, revision, files, sizes, and detected capabilities. Chat templates come from downloaded GGUF metadata and are not stored as catalog configuration.</p>
          <p>Use <code>--local</code> for the same operations when the server is stopped. The BUI also provides catalog and model views when enabled; see <a href="chapter-13-browser-ui.md">Chapter 13</a>.</p>
          <h2 id="87-container-operations">8.7 Container Operations</h2>
          <p>Chapter 2 covers image variants and initial container startup. For a persistent deployment, use a versioned image tag and retain <code>/kronk</code> in a volume. This headless example enables local authentication and exposes the API only through the host loopback interface:</p>
          <pre className="code-block"><code className="language-shell">{`docker run -d \\
  --name kronk \\
  --restart unless-stopped \\
  -e KRONK_AUTH_LOCAL_ENABLED=true \\
  -e KRONK_WEB_ADMIN_ENABLED=false \\
  -p 127.0.0.1:11435:11435 \\
  -v kronk-data:/kronk \\
  ghcr.io/ardanlabs/kronk:vX.Y.Z-cpu`}</code></pre>
          <p>Choose the processor-specific tag documented in Chapter 2. Terminate TLS at a reverse proxy or keep the service on a trusted private network. Read <a href="chapter-12-security-authentication.md">Chapter 12</a> before exposing an authenticated server remotely.</p>
          <p>Install and inspect models directly in the persistent volume without enabling browser downloads:</p>
          <pre className="code-block"><code className="language-shell">{`docker exec kronk kronk model pull unsloth/Qwen3-0.6B-Q8_0 --local
docker exec kronk kronk catalog list --local`}</code></pre>
          <p>Inspect the running container with:</p>
          <pre className="code-block"><code className="language-shell">{`docker logs -f kronk
docker exec kronk kronk --version
curl http://localhost:11435/v1/liveness`}</code></pre>
          <p>To update a pinned image, pull the new tag and recreate the container with the same volume and settings:</p>
          <pre className="code-block"><code className="language-shell">{`docker pull ghcr.io/ardanlabs/kronk:vX.Y.Z-cpu
docker stop kronk
docker rm kronk
# Repeat the docker run command with the new versioned tag.`}</code></pre>
          <p>Models, configuration, catalog state, and authentication keys remain in the named volume. Removing <code>kronk-data</code> permanently deletes that state and is not part of a normal image update.</p>
          <h2 id="88-related-administration-guides">8.8 Related Administration Guides</h2>
          <p>Detailed administration is divided by responsibility:</p>
          <ul>
            <li><a href="chapter-02-installation.md">Chapter 2</a> — installation, libraries, image variants, and data paths</li>
            <li><a href="chapter-03-model-configuration.md">Chapter 3</a> — per-model runtime settings</li>
            <li><a href="chapter-12-security-authentication.md">Chapter 12</a> — authentication, keys, tokens, and remote exposure</li>
            <li><a href="chapter-13-browser-ui.md">Chapter 13</a> — BUI operation and browser login</li>
            <li><a href="chapter-15-observability.md">Chapter 15</a> — logs, health checks, metrics, tracing, and profiling</li>
            <li><a href="chapter-18-bucky.md">Chapter 18</a> — transcription libraries, models, and pool behavior</li>
          </ul>
          <h2 id="chapter-9-api-endpoints">Chapter 9: API Endpoints</h2>
          <p>Kronk exposes several familiar inference API formats. This chapter describes their wire contracts and the Kronk-specific details needed to use them. See <a href="chapter-10-request-parameters.md">Chapter 10</a> for generation and sampling parameters.</p>
          <h2 id="91-api-conventions">9.1 API Conventions</h2>
          <p>The examples use the default server address, <code>http://localhost:11435</code>. JSON endpoints accept <code>Content-Type: application/json</code>. Streaming endpoints use Server-Sent Events (SSE).</p>
          <p>When server authentication is enabled, inference requests require a bearer token with access to the requested endpoint:</p>
          <pre className="code-block"><code className="language-text">{`Authorization: Bearer <token>`}</code></pre>
          <p>Authentication is bypassed only when the server is configured with authentication disabled. See <a href="chapter-12-security-authentication.md">Chapter 12</a> for token creation, endpoint grants, and rate limits.</p>
          <p>Application errors use a top-level code and message:</p>
          <pre className="code-block"><code className="language-json">{`{
  "code": "invalid_argument",
  "message": "missing model field"
}`}</code></pre>
          <p>The HTTP status reflects the error. Depending on the failure, clients may see statuses such as 400, 401, 403, 404, 409, 429, 500, 501, or 503.</p>
          <h2 id="92-endpoint-overview">9.2 Endpoint Overview</h2>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Endpoint</th>
                <th>Method</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>/v1/chat/completions</code></td>
                <td>POST</td>
                <td>OpenAI-style chat completions</td>
              </tr>
              <tr>
                <td><code>/v1/responses</code></td>
                <td>POST</td>
                <td>OpenAI Responses API</td>
              </tr>
              <tr>
                <td><code>/v1/messages</code></td>
                <td>POST</td>
                <td>Anthropic Messages API</td>
              </tr>
              <tr>
                <td><code>/v1/embeddings</code></td>
                <td>POST</td>
                <td>Text embeddings</td>
              </tr>
              <tr>
                <td><code>/v1/rerank</code></td>
                <td>POST</td>
                <td>Document reranking</td>
              </tr>
              <tr>
                <td><code>/v1/reranking</code></td>
                <td>POST</td>
                <td>Alias for <code>/v1/rerank</code></td>
              </tr>
              <tr>
                <td><code>/v1/tokenize</code></td>
                <td>POST</td>
                <td>Count tokens for text</td>
              </tr>
              <tr>
                <td><code>/v1/models</code></td>
                <td>GET</td>
                <td>List locally available models</td>
              </tr>
              <tr>
                <td><code>/v1/audio/transcriptions</code></td>
                <td>POST</td>
                <td>Transcribe audio with Bucky</td>
              </tr>
            </tbody>
          </table>
          <h2 id="93-chat-completions-and-tool-calls">9.3 Chat Completions and Tool Calls</h2>
          <p><code>POST /v1/chat/completions</code> accepts an OpenAI-style <code>model</code> and <code>messages</code> request:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen/Qwen3-8B-Q8_0",
  "messages": [
    {"role": "system", "content": "Be concise."},
    {"role": "user", "content": "What is the capital of France?"}
  ]
}`}</code></pre>
          <p>A non-streaming response contains one or more <code>choices</code>, an assistant <code>message</code>, a <code>finish_reason</code>, and token <code>usage</code>. Thinking models can also return <code>reasoning_content</code>. Set the top-level <code>enable_thinking</code> boolean to request or suppress thinking when the model and its chat template support that option.</p>
          <p>Set <code>"stream": true</code> to receive chat completion chunks as SSE records:</p>
          <pre className="code-block"><code className="language-text">{`data: {"id":"chatcmpl-...","object":"chat.completion.chunk",...}

data: [DONE]`}</code></pre>
          <h3 id="tool-calls">Tool calls</h3>
          <p>Add OpenAI-style function definitions in <code>tools</code> and use <code>"tool_choice": "auto"</code> to let the model select one. Tool calling requires a compatible model, chat template, and output parser; adding <code>tools</code> cannot give an incompatible model tool-calling ability.</p>
          <p>When a tool is selected, the assistant message contains <code>tool_calls</code> and uses an empty string for <code>content</code>:</p>
          <pre className="code-block"><code className="language-json">{`{
  "role": "assistant",
  "content": "",
  "tool_calls": [
    {
      "id": "call_abc123",
      "type": "function",
      "function": {
        "name": "get_weather",
        "arguments": "{\\"location\\":\\"Paris\\"}"
      }
    }
  ]
}`}</code></pre>
          <p>Execute the function in your application, then append the assistant message and a <code>role: "tool"</code> message containing the result and matching <code>tool_call_id</code>. Send the full conversation in the next request. Tool calls can also stream incrementally. Forced-function object forms are not portable across all model templates, so verify them with the model you deploy.</p>
          <h2 id="94-responses-api">9.4 Responses API</h2>
          <p><code>POST /v1/responses</code> accepts <code>input</code> as a string:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen/Qwen3-8B-Q8_0",
  "input": "Explain quantum computing in simple terms."
}`}</code></pre>
          <p>It also accepts an array of input messages for conversations. A non-streaming response places generated messages or function calls in <code>output</code>. Tools use Responses-style tool definitions; <code>tool_choice</code> is a string such as <code>"auto"</code>.</p>
          <p>With <code>"stream": true</code>, each SSE record has a named event and matching JSON payload. A text response commonly includes:</p>
          <pre className="code-block"><code className="language-text">{`event: response.created
data: {"type":"response.created",...}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"...",...}

event: response.completed
data: {"type":"response.completed",...}`}</code></pre>
          <p>Function calls produce corresponding <code>response.function_call_arguments.delta</code> and <code>.done</code> events.</p>
          <h2 id="95-anthropic-messages-api">9.5 Anthropic Messages API</h2>
          <p><code>POST /v1/messages</code> provides an Anthropic-style interface. <code>model</code> and a nonzero <code>max_tokens</code> are required:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen/Qwen3-8B-Q8_0",
  "max_tokens": 256,
  "system": "Be concise.",
  "messages": [
    {"role": "user", "content": "What is the capital of France?"}
  ]
}`}</code></pre>
          <p><code>system</code> and message <code>content</code> may be strings or arrays of content blocks. The API supports text, image, <code>tool_use</code>, and <code>tool_result</code> blocks, subject to the selected model's capabilities. Anthropic-style tool definitions use <code>name</code>, <code>description</code>, and <code>input_schema</code>.</p>
          <p>With <code>"stream": true</code>, Kronk emits Anthropic-style named events including <code>message_start</code>, <code>content_block_start</code>, <code>content_block_delta</code>, <code>content_block_stop</code>, <code>message_delta</code>, and <code>message_stop</code>.</p>
          <h2 id="96-embeddings">9.6 Embeddings</h2>
          <p><code>POST /v1/embeddings</code> accepts one string or an array of strings:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "ggml-org/embeddinggemma-300m-qat-Q8_0",
  "input": ["First document", "Second document"]
}`}</code></pre>
          <p>The response contains <code>object</code>, <code>created</code>, <code>model</code>, a <code>data</code> array, and <code>usage</code>. Each data item has an <code>index</code> and an <code>embedding</code> vector. Use an embedding model; ordinary text-generation models do not provide useful embedding behavior.</p>
          <h2 id="97-reranking">9.7 Reranking</h2>
          <p><code>POST /v1/rerank</code> and <code>POST /v1/reranking</code> are equivalent. Supply a reranker model, a query, and a nonempty string array:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "gpustack/bge-reranker-v2-m3-Q8_0",
  "query": "What is machine learning?",
  "documents": [
    "Machine learning is a branch of artificial intelligence.",
    "The weather is sunny."
  ],
  "top_n": 1,
  "return_documents": true
}`}</code></pre>
          <p>Results are sorted by descending relevance and returned in <code>data</code>, not <code>results</code>:</p>
          <pre className="code-block"><code className="language-json">{`{
  "object": "list",
  "created": 1738857600,
  "model": "gpustack/bge-reranker-v2-m3-Q8_0",
  "data": [
    {"index": 0, "relevance_score": 0.91, "document": "Machine learning is a branch of artificial intelligence."}
  ],
  "usage": {"prompt_tokens": 24, "total_tokens": 24}
}`}</code></pre>
          <p>Documents are omitted from results by default. Set <code>return_documents</code> to <code>true</code> when the response should include their text. <code>top_n</code> defaults to all documents.</p>
          <h2 id="98-tokenization">9.8 Tokenization</h2>
          <p><code>POST /v1/tokenize</code> returns a token <strong>count</strong>, not token IDs:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen/Qwen3-8B-Q8_0",
  "input": "The quick brown fox",
  "apply_template": true,
  "add_generation_prompt": true
}`}</code></pre>
          <p><code>apply_template</code> defaults to <code>false</code>. When enabled, Kronk wraps the input as a user message and includes chat-template overhead in the count. <code>add_generation_prompt</code> controls the assistant prefix when the template is applied and defaults to <code>true</code>.</p>
          <pre className="code-block"><code className="language-json">{`{
  "object": "tokenize",
  "created": 1738857600,
  "model": "Qwen/Qwen3-8B-Q8_0",
  "tokens": 11
}`}</code></pre>
          <h2 id="99-models-and-audio-transcription">9.9 Models and Audio Transcription</h2>
          <p><code>GET /v1/models</code> returns an OpenAI-style list of models and configured model extensions available locally. It is not limited to models currently loaded in memory. Each item includes <code>id</code>, <code>object</code>, <code>created</code>, and <code>owned_by</code>. <code>owned_by</code> comes from model metadata when available and otherwise defaults to <code>kronk</code>.</p>
          <p><code>POST /v1/audio/transcriptions</code> accepts multipart audio uploads and uses the Bucky speech-to-text runtime. Its request fields, formats, and administrative operations are documented in <a href="chapter-18-bucky.md#1871-post-v1audiotranscriptions">Chapter 18</a>.</p>
          <h2 id="chapter-10-request-parameters">Chapter 10: Request Parameters</h2>
          <p>This chapter covers generation parameters used by Chat Completions and the Go SDK. Other API formats expose compatible subsets or translate their own field names into these parameters. See <a href="chapter-09-api-endpoints.md">Chapter 9</a> for endpoint-specific request formats and streaming behavior.</p>
          <h2 id="101-scope-and-defaults">10.1 Scope and Defaults</h2>
          <p>The defaults below are Kronk's baseline values. A model configuration can provide different sampling defaults, and a request can override them. See <a href="chapter-03-model-configuration.md#37-advanced-features">Chapter 3 §3.7</a> for per-model <code>sampling-parameters</code>.</p>
          <p>JSON requests use <code>number</code>, <code>integer</code>, <code>boolean</code>, and <code>string</code> values. The Go SDK accepts the corresponding Go values in <code>model.D</code>.</p>
          <p>Avoid changing several samplers at once. Start with the model's defaults, change one parameter, and evaluate the result against representative prompts. Parameters that improve creative prose can reduce the reliability of JSON and tool calls.</p>
          <h2 id="102-core-sampling">10.2 Core Sampling</h2>
          <p>These parameters control how Kronk selects the next token:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>JSON key</th>
                <th>Type</th>
                <th>Baseline</th>
                <th>Behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>temperature</code></td>
                <td>number</td>
                <td><code>0.8</code></td>
                <td>Rescales token probabilities. Higher values generally increase variation.</td>
              </tr>
              <tr>
                <td><code>top_k</code></td>
                <td>integer</td>
                <td><code>40</code></td>
                <td>Keeps only the K most probable candidates.</td>
              </tr>
              <tr>
                <td><code>top_p</code></td>
                <td>number</td>
                <td><code>0.9</code></td>
                <td>Keeps the smallest candidate set whose cumulative probability reaches P.</td>
              </tr>
              <tr>
                <td><code>min_p</code></td>
                <td>number</td>
                <td><code>0.0</code></td>
                <td>Removes candidates below <code>min_p × probability_of_most_likely_token</code>; <code>0</code> disables it.</td>
              </tr>
            </tbody>
          </table>
          <p>Request values <code>top_p: 0</code> and <code>top_p: 1</code> are treated as unset so clients that send those common defaults do not override model-specific tuning. A model configuration can still set <code>top_p: 1</code> explicitly. Nonpositive values for <code>temperature</code> and <code>top_k</code> also resolve to configured or baseline defaults; <code>temperature: 0</code> is therefore not a deterministic-mode switch in Kronk.</p>
          <h2 id="103-repetition-control">10.3 Repetition Control</h2>
          <p>Kronk supports both token penalties and DRY n-gram penalties:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>JSON key</th>
                <th>Type</th>
                <th>Baseline</th>
                <th>Behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>repeat_penalty</code></td>
                <td>number</td>
                <td><code>1.0</code></td>
                <td>Multiplies penalties for tokens seen in the recent window; <code>1.0</code> disables it.</td>
              </tr>
              <tr>
                <td><code>repeat_last_n</code></td>
                <td>integer</td>
                <td><code>64</code></td>
                <td>Number of recent tokens considered by repetition penalties.</td>
              </tr>
              <tr>
                <td><code>frequency_penalty</code></td>
                <td>number</td>
                <td><code>0.0</code></td>
                <td>Penalizes tokens in proportion to how often they appeared.</td>
              </tr>
              <tr>
                <td><code>presence_penalty</code></td>
                <td>number</td>
                <td><code>0.0</code></td>
                <td>Applies a flat penalty to tokens that appeared at least once.</td>
              </tr>
              <tr>
                <td><code>dry_multiplier</code></td>
                <td>number</td>
                <td><code>0.0</code></td>
                <td>Enables DRY and controls its strength; <code>0</code> disables it.</td>
              </tr>
              <tr>
                <td><code>dry_base</code></td>
                <td>number</td>
                <td><code>1.75</code></td>
                <td>Exponential penalty growth for longer repeated sequences.</td>
              </tr>
              <tr>
                <td><code>dry_allowed_length</code></td>
                <td>integer</td>
                <td><code>2</code></td>
                <td>Minimum repeated sequence length before DRY applies.</td>
              </tr>
              <tr>
                <td><code>dry_penalty_last_n</code></td>
                <td>integer</td>
                <td><code>0</code></td>
                <td>Recent-token window for DRY; <code>0</code> uses the full context.</td>
              </tr>
            </tbody>
          </table>
          <p>The repetition and DRY samplers are disabled by default because penalties can also suppress structural tokens needed by tool-call and JSON formats. Enable them only after testing the selected model and template.</p>
          <h2 id="104-advanced-sampling">10.4 Advanced Sampling</h2>
          <p>XTC probabilistically removes likely candidates to increase diversity:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>JSON key</th>
                <th>Type</th>
                <th>Baseline</th>
                <th>Behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>xtc_probability</code></td>
                <td>number</td>
                <td><code>0.0</code></td>
                <td>Probability that XTC runs for a token; <code>0</code> disables it.</td>
              </tr>
              <tr>
                <td><code>xtc_threshold</code></td>
                <td>number</td>
                <td><code>0.1</code></td>
                <td>Probability threshold used when culling candidates.</td>
              </tr>
              <tr>
                <td><code>xtc_min_keep</code></td>
                <td>integer</td>
                <td><code>1</code></td>
                <td>Minimum candidates retained by XTC.</td>
              </tr>
            </tbody>
          </table>
          <p>Adaptive-P dynamically adjusts a probability threshold as generation continues:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>JSON key</th>
                <th>Type</th>
                <th>Baseline</th>
                <th>Behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>adaptive_p_target</code></td>
                <td>number</td>
                <td><code>0.0</code></td>
                <td>Target probability; values above <code>0</code> enable Adaptive-P.</td>
              </tr>
              <tr>
                <td><code>adaptive_p_decay</code></td>
                <td>number</td>
                <td><code>0.0</code></td>
                <td>Controls how quickly the adaptive state changes.</td>
              </tr>
            </tbody>
          </table>
          <p>These samplers are specialized controls. Leave them disabled unless you can measure an improvement for a specific workload.</p>
          <h2 id="105-generation-and-reasoning">10.5 Generation and Reasoning</h2>
          <table className="flags-table">
            <thead>
              <tr>
                <th>JSON key</th>
                <th>Type</th>
                <th>Baseline</th>
                <th>Behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>max_tokens</code></td>
                <td>integer</td>
                <td>model-dependent</td>
                <td>Maximum output tokens requested.</td>
              </tr>
              <tr>
                <td><code>enable_thinking</code></td>
                <td>boolean</td>
                <td><code>true</code></td>
                <td>Requests thinking from models and templates that support it.</td>
              </tr>
              <tr>
                <td><code>reasoning_effort</code></td>
                <td>string</td>
                <td><code>medium</code></td>
                <td>Requests <code>none</code>, <code>minimal</code>, <code>low</code>, <code>medium</code>, or <code>high</code> effort from supported reasoning templates.</td>
              </tr>
              <tr>
                <td><code>return_prompt</code></td>
                <td>boolean</td>
                <td><code>false</code></td>
                <td>Includes the rendered prompt in the final Chat Completions response.</td>
              </tr>
            </tbody>
          </table>
          <p>If neither the request nor model configuration supplies a positive <code>max_tokens</code>, Kronk uses the model's configured context window. The actual output can be shorter because the prompt and generated text share that window, the model can stop naturally, or another limit can end generation.</p>
          <p>Reasoning controls are model- and template-dependent. Unsupported models may ignore them. A parser can also normalize <code>reasoning_effort</code> to values accepted by its template; for example, a template that supports only <code>none</code> and <code>high</code> cannot honor every intermediate value.</p>
          <h2 id="106-structured-output">10.6 Structured Output</h2>
          <p>Kronk can convert JSON Schema to a GBNF grammar and constrain emitted tokens. For OpenAI-compatible clients, prefer <code>response_format</code>:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen/Qwen3-8B-Q8_0",
  "messages": [
    {"role": "user", "content": "Return a language and its year of creation."}
  ],
  "response_format": {
    "type": "json_schema",
    "json_schema": {
      "name": "language",
      "schema": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "year": {"type": "integer"}
        },
        "required": ["name", "year"]
      }
    }
  }
}`}</code></pre>
          <p>Supported <code>response_format.type</code> values are <code>text</code>, <code>json_object</code>, and <code>json_schema</code>. Kronk also accepts a schema directly in the top-level <code>json_schema</code> field and accepts a custom GBNF string in <code>grammar</code>. Use one structured-output mechanism per request.</p>
          <p>When a constraint is present and <code>enable_thinking</code> is omitted, Kronk disables thinking automatically so free-form reasoning does not precede the structured answer. Explicitly enabling thinking overrides that default, but is generally counterproductive for constrained output.</p>
          <p>A grammar restricts which tokens can be emitted; it does not guarantee a complete result. A response cut short by <code>max_tokens</code>, context limits, or cancellation can still contain an incomplete JSON value.</p>
          <h2 id="107-token-log-probabilities">10.7 Token Log Probabilities</h2>
          <p>Set <code>logprobs: true</code> to return the log probability of each generated token. <code>top_logprobs</code> requests likely alternatives and is clamped to the range 0–5. Any positive <code>top_logprobs</code> value implicitly enables <code>logprobs</code>.</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen/Qwen3-8B-Q8_0",
  "messages": [
    {"role": "user", "content": "What is 2 + 2?"}
  ],
  "logprobs": true,
  "top_logprobs": 3,
  "max_tokens": 10
}`}</code></pre>
          <p>Each entry in <code>choices[].logprobs.content</code> contains the generated <code>token</code>, its <code>logprob</code>, its UTF-8 <code>bytes</code>, and up to <code>top_logprobs</code> alternatives. Values closer to zero were more probable under that generation step, but they are not proof of factual correctness.</p>
          <p>Streaming responses attach logprob data to individual delta chunks. Non-streaming responses collect the entries in the final choice. This data is useful for token-level diagnostics and comparative scoring; it does not alter sampling after generation has occurred.</p>
          <h2 id="chapter-11-multimodal-models">Chapter 11: Multimodal Models</h2>
          <p>Multimodal models combine a language model with a media projector that turns images or audio into input the language model can process. They use the Chat Completions endpoint described in <a href="chapter-09-api-endpoints.md">Chapter 9</a>.</p>
          <h2 id="111-models-and-projectors">11.1 Models and Projectors</h2>
          <p>Use the catalog to find models with image or audio capabilities:</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list`}</code></pre>
          <p>The <code>MTMD</code> column identifies entries with a multimodal projector. The BUI catalog also provides image, audio, and video capability filters. The live catalog is the source of truth; examples in the seed catalog include:</p>
          <ul>
            <li><code>unsloth/LFM2.5-VL-1.6B-Q8_0</code> for images;</li>
            <li><code>ggml-org/Qwen2.5-Omni-3B-Q8_0</code> for images and audio; and</li>
            <li><code>ggml-org/Qwen3-Omni-30B-A3B-Instruct-Q8_0</code> for image, audio, and video-capable model metadata.</li>
          </ul>
          <p>Pulling a catalog model also pulls its companion projector when one is available:</p>
          <pre className="code-block"><code className="language-shell">{`kronk model pull unsloth/LFM2.5-VL-1.6B-Q8_0`}</code></pre>
          <p>The model and projector capabilities must match the submitted media. A model without a projector rejects media, as does a projector without support for the detected image or audio type.</p>
          <h2 id="112-supported-inputs">11.2 Supported Inputs</h2>
          <p>Kronk recognizes media from its decoded file signature rather than trusting a declared MIME type or extension:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Media</th>
                <th>Recognized containers</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Images</td>
                <td>JPEG, PNG, GIF, WebP</td>
              </tr>
              <tr>
                <td>Audio</td>
                <td>WAV, MP3, Ogg, FLAC</td>
              </tr>
            </tbody>
          </table>
          <p>For REST requests, prefer an ordered content array containing text and one or more media parts. Media values can be base64 data URLs or raw base64. Despite the <code>image_url</code> and <code>video_url</code> field names, Kronk does not fetch <code>http://</code> or <code>https://</code> URLs.</p>
          <p>Kronk also recognizes a plain base64 string used as the entire message <code>content</code>. That legacy form is less useful because it cannot place text and media together in one ordered content array.</p>
          <p>Actual video containers such as MP4 and WebM are not decoded by the current media path. For video analysis, extract frames and send them as supported images in the intended order.</p>
          <h2 id="113-sending-an-image">11.3 Sending an Image</h2>
          <p>Place media before the question unless the selected model documents another order. Several multimodal templates were trained with the media token first, and Kronk preserves the order of all content parts.</p>
          <p>This shell example expands the base64 value before sending the request:</p>
          <pre className="code-block"><code className="language-shell">{`IMAGE_B64=$(base64 < photo.jpg | tr -d '\\n')

curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  --data-binary @- <<EOF
{
  "model": "unsloth/LFM2.5-VL-1.6B-Q8_0",
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "image_url",
          "image_url": {"url": "data:image/jpeg;base64,\${IMAGE_B64}"}
        },
        {"type": "text", "text": "Describe this image."}
      ]
    }
  ],
  "max_tokens": 1024
}
EOF`}</code></pre>
          <p>An ordered content array can contain multiple images and text parts. Whether a particular number or resolution of images works well depends on the selected model and projector.</p>
          <h2 id="114-sending-audio">11.4 Sending Audio</h2>
          <p>Audio uses an <code>input_audio</code> content part in the same ordered array:</p>
          <pre className="code-block"><code className="language-json">{`{
  "type": "input_audio",
  "input_audio": {
    "data": "data:audio/wav;base64,UklGRi...",
    "format": "wav"
  }
}`}</code></pre>
          <p>Put this part before the text question. The <code>format</code> field is accepted for client compatibility, but Kronk currently determines the actual format from the decoded bytes rather than this value.</p>
          <p>Use a multimodal chat model when you need conversational questions, summaries, or reasoning about audio. For a dedicated speech-to-text API, use Bucky's <code>POST /v1/audio/transcriptions</code> endpoint described in <a href="chapter-18-bucky.md#1871-post-v1audiotranscriptions">Chapter 18</a>.</p>
          <h2 id="115-go-sdk-helpers">11.5 Go SDK Helpers</h2>
          <p>Go applications can read media into a byte slice and use:</p>
          <ul>
            <li><code>model.ImageMessage(question, image, format)</code>; or</li>
            <li><code>model.AudioMessage(question, audio, format)</code>.</li>
          </ul>
          <p>These helpers create one user turn with media before text. <code>model.VideoMessage</code> constructs a <code>video_url</code> part, but it does not add video-container decoding; send extracted frames with <code>ImageMessage</code> for the current media path.</p>
          <h2 id="116-configuration-and-resources">11.6 Configuration and Resources</h2>
          <p>Multimodal requests use the same batch engine and concurrency controls as text requests. The projector adds weights and runtime buffers, while image resolution, audio duration, context length, and <code>nseq-max</code> affect resource use. Use the BUI VRAM Calculator rather than adding model, projector, and KV file sizes as a complete memory estimate. See <a href="chapter-03-model-configuration.md#36-memory-planning-and-quantization">Chapter 3 §3.6</a> for memory planning and <a href="chapter-04-batch-processing.md">Chapter 4</a> for concurrency.</p>
          <p>Most deployments should leave <code>nubatch</code> unset. Its normal default is 2048, but MoE expert CPU offload can raise it to 4096. A multimodal encoder may require an entire media-token chunk to fit in one physical batch, so lowering <code>nubatch</code> can break media input. <code>proj-on-cpu: true</code> can keep the projector on the CPU when accelerator memory is constrained, at a performance cost.</p>
          <h2 id="117-message-caching">11.7 Message Caching</h2>
          <p>Incremental Message Caching can reuse unchanged media state for text-only follow-up turns without encoding the media again. Changing, reordering, removing, or appending media rebuilds the stable media plan through the multimodal pipeline. See <a href="chapter-05-message-caching.md#54-media-requests">Chapter 5 §5.4</a> for the cache behavior and limitations.</p>
          <h2 id="118-limitations">11.8 Limitations</h2>
          <ul>
            <li>Media must be embedded as base64; Kronk does not fetch remote URLs.</li>
            <li>The current path accepts image and audio containers, not video containers.</li>
            <li>The selected model and projector must support the detected modality.</li>
            <li>Image resolution, media count, and audio duration affect latency and memory.</li>
            <li>Model quality and practical media limits vary by model and projector.</li>
          </ul>
          <hr />
          <p><em>Next: &lt;a href="chapter-12-security-authentication.md"&gt;Chapter 12: Security & Authentication&lt;/a&gt;</em></p>
          <h2 id="chapter-12-security-and-authentication">Chapter 12: Security and Authentication</h2>
          <p>Kronk signs JWT bearer tokens with local RSA keys. A token can be an unrestricted administrator credential or a user credential limited to specific inference endpoints and request quotas.</p>
          <h2 id="121-authentication-modes">12.1 Authentication Modes</h2>
          <p>Inference and administrative protection are configured separately:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Mode</th>
                <th>Inference auth</th>
                <th>Admin auth</th>
                <th>Effect</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Open</td>
                <td>Off</td>
                <td>Off</td>
                <td>Inference and management APIs are open.</td>
              </tr>
              <tr>
                <td>Admin-only</td>
                <td>Off</td>
                <td>On</td>
                <td>Inference is open; management, playground, BUI login, and security APIs require an admin token.</td>
              </tr>
              <tr>
                <td>Fully protected</td>
                <td>On</td>
                <td>On automatically</td>
                <td>Inference requires a valid scoped token, and administrative APIs require an admin token.</td>
              </tr>
            </tbody>
          </table>
          <p>Start a fully protected server with:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --auth-enabled`}</code></pre>
          <p>To leave inference open while protecting administration:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --admin-auth-enabled`}</code></pre>
          <p>The equivalent environment variables are:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_AUTH_LOCAL_ENABLED=true
export KRONK_AUTH_ADMIN_ENABLED=true
kronk server start`}</code></pre>
          <p>Setting <code>KRONK_AUTH_LOCAL_ENABLED=true</code> always enables admin authentication as well. <code>GET /v1/models</code> follows inference authentication: it requires a valid token in fully protected mode but has no separate <code>models</code> endpoint grant.</p>
          <h2 id="122-initial-credentials">12.2 Initial Credentials</h2>
          <p>When the embedded security store initializes for the first time, Kronk creates:</p>
          <ul>
            <li><code>~/.kronk/keys/master.pem</code>, the master private key;</li>
            <li><code>~/.kronk/keys/master.jwt</code>, an admin token valid for ten years; and</li>
            <li>an additional UUID-named signing key used for subsequently created tokens.</li>
          </ul>
          <p>This initialization occurs even in open mode when Kronk starts its embedded auth service. The key directory is set to mode <code>0700</code>, and private keys and the master token are set to <code>0600</code>.</p>
          <p>Treat both master files as recovery credentials. Keep secure backups and never distribute them to applications. Changing the configured JWT issuer causes existing tokens to fail issuer validation.</p>
          <h2 id="123-admin-and-user-tokens">12.3 Admin and User Tokens</h2>
          <p>Load the initial admin token for CLI administration:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)`}</code></pre>
          <p>Admin tokens bypass endpoint grants and rate limits. The security CLI requires <code>KRONK_TOKEN</code> for key and token commands. In protected modes, the server also verifies that it is an admin token.</p>
          <p>Create a short-lived application token with exact endpoint grants:</p>
          <pre className="code-block"><code className="language-shell">{`kronk security token create \\
  --duration 24h \\
  --endpoints chat-completions,responses,messages`}</code></pre>
          <p><code>--duration</code> uses Go duration syntax such as <code>1h</code>, <code>24h</code>, or <code>720h</code>; it does not accept <code>30d</code>. Every generated token receives a unique subject UUID, so authorization and quotas are per token rather than per named human or account.</p>
          <h2 id="124-endpoint-grants-and-rate-limits">12.4 Endpoint Grants and Rate Limits</h2>
          <p>The grant names used by inference middleware are:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Grant</th>
                <th>Endpoint</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>chat-completions</code></td>
                <td><code>POST /v1/chat/completions</code></td>
              </tr>
              <tr>
                <td><code>responses</code></td>
                <td><code>POST /v1/responses</code></td>
              </tr>
              <tr>
                <td><code>messages</code></td>
                <td><code>POST /v1/messages</code></td>
              </tr>
              <tr>
                <td><code>embeddings</code></td>
                <td><code>POST /v1/embeddings</code></td>
              </tr>
              <tr>
                <td><code>rerank</code></td>
                <td><code>POST /v1/rerank</code> and <code>/v1/reranking</code></td>
              </tr>
              <tr>
                <td><code>tokenize</code></td>
                <td><code>POST /v1/tokenize</code></td>
              </tr>
              <tr>
                <td><code>transcriptions</code></td>
                <td><code>POST /v1/audio/transcriptions</code></td>
              </tr>
            </tbody>
          </table>
          <p>Grant names are not validated when a token is created. Use the names above exactly; a typo produces a valid token with an unusable grant.</p>
          <p>Each <code>--endpoints</code> entry has one of these forms:</p>
          <pre className="code-block"><code className="language-text">{`endpoint
endpoint:unlimited
endpoint:limit/window`}</code></pre>
          <p>An entry without a suffix is unlimited. Rate windows are <code>day</code>, <code>month</code>, and <code>year</code>, measured at UTC calendar boundaries. For example:</p>
          <pre className="code-block"><code className="language-shell">{`kronk security token create \\
  --duration 720h \\
  --endpoints "chat-completions:1000/day,embeddings:500/month,responses:unlimited"`}</code></pre>
          <p>Kronk counts admitted requests by token subject and endpoint. Counters are stored in <code>~/.kronk/badger/</code>, survive server restarts, and expire after their current window. Admin tokens do not use these counters.</p>
          <h2 id="125-using-a-token">12.5 Using a Token</h2>
          <p>Send a token using the bearer authorization scheme:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_TOKEN="<application-token>"

curl http://localhost:11435/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen/Qwen3-8B-Q8_0",
    "messages": [{"role": "user", "content": "Hello"}]
  }'`}</code></pre>
          <p>Kronk verifies the signature, issuer, expiration, required admin status or endpoint grant, and quota before processing a protected request. Authentication, missing-grant, and exhausted-quota failures currently all cross the auth boundary as <code>401 Unauthorized</code>; clients should not expect distinct 403 or 429 responses from this path.</p>
          <h2 id="126-key-rotation-and-revocation">12.6 Key Rotation and Revocation</h2>
          <p>Security commands use the running server by default:</p>
          <pre className="code-block"><code className="language-shell">{`kronk security key list
kronk security key create
kronk security key delete --keyid "$KEY_ID"`}</code></pre>
          <p><code>key create</code> generates a UUID-named private key. The newest key becomes the signing key for subsequently created tokens, while older public keys continue to verify existing tokens.</p>
          <p>Kronk does not maintain an individual token revocation list. Deleting a key immediately invalidates every token signed by that key. Rotate safely by:</p>
          <ol>
            <li>Creating a new key.</li>
            <li>Issuing replacement tokens, which use the new key.</li>
            <li>Waiting for old tokens to expire or confirming they are no longer used.</li>
            <li>Deleting the old non-master key.</li>
          </ol>
          <p>The master key cannot be deleted through the security API. Keep it as the administrative recovery key.</p>
          <p>Add <code>--local</code> to operate directly on the local key store without the server:</p>
          <pre className="code-block"><code className="language-shell">{`kronk security key list --local`}</code></pre>
          <p>Local mode still requires <code>KRONK_TOKEN</code> containing a valid local admin token. Use it while the server is stopped because the local command opens the same Badger database.</p>
          <h2 id="127-embedded-and-standalone-authentication">12.7 Embedded and Standalone Authentication</h2>
          <p>By default, <code>kronk server</code> runs the auth service in-process over an in-memory listener. Relevant server settings are:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Flag</th>
                <th>Environment variable</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>--auth-enabled</code></td>
                <td><code>KRONK_AUTH_LOCAL_ENABLED</code></td>
                <td>Protect inference and administration.</td>
              </tr>
              <tr>
                <td><code>--admin-auth-enabled</code></td>
                <td><code>KRONK_AUTH_ADMIN_ENABLED</code></td>
                <td>Protect administration only.</td>
              </tr>
              <tr>
                <td><code>--auth-issuer</code></td>
                <td><code>KRONK_AUTH_LOCAL_ISSUER</code></td>
                <td>Set the expected JWT issuer.</td>
              </tr>
              <tr>
                <td><code>--auth-host</code></td>
                <td><code>KRONK_AUTH_HOST</code></td>
                <td>Connect to an external auth service instead.</td>
              </tr>
            </tbody>
          </table>
          <p>Setting <code>KRONK_AUTH_HOST</code> skips embedded auth startup. The standalone <code>auth</code> service uses <code>AUTH_AUTH_HOST</code> (default <code>localhost:6000</code>), <code>AUTH_AUTH_ISSUER</code> (default <code>kronk project</code>), and <code>AUTH_AUTH_ENABLED</code>. The server and auth service must agree on issuer and protection policy.</p>
          <p>CLI web mode reads <code>KRONK_WEB_API_HOST</code>, which defaults to <code>localhost:11435</code>.</p>
          <h2 id="128-production-hardening">12.8 Production Hardening</h2>
          <p>Kronk listens on <code>0.0.0.0:11435</code> and serves plain HTTP by default. For any traffic outside a trusted host:</p>
          <ul>
            <li>enable full or admin authentication as appropriate;</li>
            <li>terminate TLS at a trusted reverse proxy and do not expose the API port directly to the public internet;</li>
            <li>restrict network access with host or cloud firewall rules;</li>
            <li>restrict <code>KRONK_WEB_CORS_ALLOWED_ORIGINS</code> instead of retaining <code>*</code>;</li>
            <li>issue separate, short-lived, least-privilege tokens for each application;</li>
            <li>set quotas based on the workload and monitor authentication failures;</li>
            <li>protect and back up <code>master.pem</code> and <code>master.jwt</code>; and</li>
            <li>rotate non-master signing keys deliberately, accounting for all tokens that a deletion will revoke.</li>
          </ul>
          <hr />
          <p><em>Next: &lt;a href="chapter-13-browser-ui.md"&gt;Chapter 13: Browser UI (BUI)&lt;/a&gt;</em></p>
          <h2 id="chapter-13-browser-ui-bui">Chapter 13: Browser UI (BUI)</h2>
          <p>Kronk includes a Browser UI (BUI) for managing a local server and trying models interactively. It is bundled in the <code>kronk</code> binary, served from the same port as the Web API, and uses the server's <code>/v1</code> endpoints rather than maintaining separate state.</p>
          <p>This chapter describes the main areas of the BUI without cataloging every control. The CLI remains useful for scripting and headless administration; the BUI is not intended to duplicate every CLI command.</p>
          <h3 id="131-accessing-the-bui">13.1 Accessing the BUI</h3>
          <p>The BUI is enabled by default. Start the server and open:</p>
          <pre className="code-block"><code>{`http://localhost:11435/admin/`}</code></pre>
          <p>The address comes from <code>KRONK_WEB_API_HOST</code>, whose default is <code>0.0.0.0:11435</code>. The server root and <code>/admin</code> redirect to <code>/admin/</code> while the BUI is enabled.</p>
          <p>For a headless deployment, disable it with either form:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_WEB_ADMIN_ENABLED=false
kronk server start`}</code></pre>
          <pre className="code-block"><code className="language-shell">{`kronk server start --web-admin-enabled=false`}</code></pre>
          <h3 id="132-capabilities">13.2 Capabilities</h3>
          <p>The sidebar groups related operations by subsystem.</p>
          <h4 id="apps">Apps</h4>
          <ul>
            <li><strong>Chat</strong> provides multi-turn conversations, model selection, system prompts, chat history, and sampling controls.</li>
            <li><strong>VRAM Calculator</strong> estimates model memory requirements from a HuggingFace model without downloading the entire model. A calculator is also available in local model and catalog details.</li>
            <li><strong>Translator</strong> records or uploads audio for transcription through Bucky. You can select a whisper model, language, and response format and inspect timestamped segments. See <a href="chapter-18-bucky.md#186-bui-usage">Chapter 18 §18.6</a>.</li>
          </ul>
          <h4 id="system">System</h4>
          <ul>
            <li><strong>Info</strong> reports server, host, device, library, and model diagnostics.</li>
            <li><strong>Running</strong> shows models that are loading or resident in the pool, along with the current resource budget. Models can also be unloaded here.</li>
          </ul>
          <h4 id="kronk">Kronk</h4>
          <ul>
            <li><strong>Models</strong> lists local GGUF models and their metadata, effective configuration, sampling defaults, chat templates, and estimated VRAM. Models can be pulled from HuggingFace, copied from another Kronk Model Server (KMS), or removed. Persistent configuration is read from <code>~/.kronk/models/model_config.yaml</code>; the model details are read-only.</li>
            <li><strong>Catalog</strong> browses the personal catalog at <code>~/.kronk/catalog/catalog.yaml</code>. You can refresh its on-disk state, inspect entries, pull their files, and remove entries. See Chapter 8 for how the catalog is populated and resolved.</li>
            <li><strong>Libs</strong> downloads and removes llama.cpp bundles for supported operating system, architecture, and processor combinations. Bundles are stored below <code>~/.kronk/libraries/</code>.</li>
          </ul>
          <h4 id="bucky">Bucky</h4>
          <p>Bucky has separate pages for downloading and removing whisper models and managing whisper.cpp library bundles under <code>~/.kronk/bucky-libraries/</code>. See <a href="chapter-18-bucky.md">Chapter 18</a> for installation and transcription details.</p>
          <h4 id="security">Security</h4>
          <p>Security pages list, create, and delete signing keys and create user tokens. Token controls include duration, endpoint grants, and rate limits. The <strong>Session</strong> page reports whether browser administration authentication is enabled and whether the browser has an authenticated admin session.</p>
          <p>These tools remain available in open mode. This lets you prepare keys and tokens before enabling authentication, but anyone who can reach an open server can also use its management APIs. See Chapter 12 before exposing the server beyond a trusted machine or network.</p>
          <h4 id="testing">Testing</h4>
          <p>Testing provides several model evaluation workflows:</p>
          <ul>
            <li><strong>Accuracy</strong> compares a model's reproduction of source functions with the actual source, individually, in batches, or across models.</li>
            <li><strong>Efficiency</strong> compares generation throughput across selected models.</li>
            <li><strong>Basic</strong> exercises chat, prompt rendering, and tool calling against a model loaded with a chosen runtime configuration.</li>
            <li><strong>Sampling</strong> runs automated sampling-parameter sweeps.</li>
            <li><strong>Configuration</strong> runs automated runtime-configuration sweeps.</li>
          </ul>
          <p>The Basic, Sampling, and Configuration tools create server-side playground sessions. Their configuration applies to that test session; it does not edit the persistent model configuration file.</p>
          <h4 id="docs">Docs</h4>
          <p>The binary includes an offline documentation snapshot built with that Kronk release:</p>
          <ul>
            <li><strong>Manual</strong> — this manual, with chapter navigation</li>
            <li><strong>SDK</strong> — SDK and model API references with examples</li>
            <li><strong>CLI</strong> — command reference</li>
            <li><strong>Web API</strong> — inference and management endpoint reference</li>
          </ul>
          <h3 id="133-authentication">13.3 Authentication</h3>
          <p>By default, the BUI and management APIs do not require a login. To protect browser administration, enable admin authentication and configure the SHA-256 digest of the password:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_AUTH_ADMIN_ENABLED=true
export KRONK_WEB_ADMIN_PASSWORD_SHA256="$(printf '%s' 'choose-a-password' | shasum -a 256 | awk '{print $1}')"
kronk server start`}</code></pre>
          <p>Login creates a one-hour admin token in an HttpOnly, SameSite cookie. The browser cannot read the token, and the server uses it to authenticate the BUI's same-origin <code>/v1</code> requests. Sign out from the sidebar to end the browser session.</p>
          <p>General authentication also enables admin authentication. Chapter 12 explains the open, admin-only, and fully protected modes, including TLS and reverse proxy considerations.</p>
          <h3 id="134-operational-notes">13.4 Operational Notes</h3>
          <ul>
            <li>Downloading a library bundle does not switch the libraries used by the running process. Set <code>KRONK_LIB_PATH</code> or <code>KRONK_BUCKY_LIB_PATH</code> to the selected bundle and restart the server.</li>
            <li>Model and catalog detail pages display configuration but do not persist model overrides. Edit <code>~/.kronk/models/model_config.yaml</code> and reload the model when changing persistent configuration; see Chapter 3.</li>
            <li>Closing a browser tab does not explicitly delete its playground session. Use <strong>Unload Model</strong> when finished. Otherwise, the model remains subject to the server pool's normal eviction policy and is removed on server restart.</li>
          </ul>
          <hr />
          <p><em>Next: &lt;a href="chapter-14-client-integration.md"&gt;Chapter 14: Client Integration&lt;/a&gt;</em></p>
          <h2 id="chapter-14-client-integration">Chapter 14: Client Integration</h2>
          <p>Kronk's OpenAI-compatible API works with clients that let you configure a base URL and API key. This chapter covers the OpenCode configuration shipped with the repository and representative setups for OpenWebUI, the OpenAI Python SDK, curl, and LangChain.</p>
          <h3 id="141-connection-settings">14.1 Connection Settings</h3>
          <p>Most clients need three values:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Setting</th>
                <th>Value</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Base URL</td>
                <td><code>http://localhost:11435/v1</code></td>
              </tr>
              <tr>
                <td>API key</td>
                <td>Any non-empty value in open mode; a valid user token in protected mode</td>
              </tr>
              <tr>
                <td>Model</td>
                <td>An ID returned by <code>GET /v1/models</code></td>
              </tr>
            </tbody>
          </table>
          <p>List the model IDs available to inference clients:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/models`}</code></pre>
          <p>When inference authentication is enabled, add <code>-H "Authorization: Bearer $KRONK_TOKEN"</code> and use a user token whose endpoint grants allow the API you are calling. See Chapter 12 for token creation and Chapter 9 for the supported endpoints.</p>
          <p>If the client runs in a container, <code>localhost</code> refers to the container rather than the host. Use the container runtime's host address, such as <code>host.docker.internal</code>, or place both services on the same container network.</p>
          <h3 id="142-opencode">14.2 OpenCode</h3>
          <p>OpenCode is the coding agent for which this repository ships a ready-to-use configuration. It registers Kronk as an OpenAI-compatible provider and connects OpenCode to Kronk's MCP service.</p>
          <h4 id="install-opencode">Install OpenCode</h4>
          <p>Install OpenCode with its official installer:</p>
          <pre className="code-block"><code className="language-shell">{`curl -fsSL https://opencode.ai/install | bash`}</code></pre>
          <p>Other installation options are listed at <a href="https://opencode.ai/download">opencode.ai/download</a>. Verify the result with:</p>
          <pre className="code-block"><code className="language-shell">{`opencode --version`}</code></pre>
          <h4 id="install-the-kronk-bundle">Install the Kronk Bundle</h4>
          <p>From a Kronk source checkout, run:</p>
          <pre className="code-block"><code className="language-shell">{`make agents-default-opencode`}</code></pre>
          <blockquote><strong>Warning:</strong> This target is intended to install the repository's complete</blockquote>
          <blockquote>OpenCode setup. It overwrites <code>opencode.jsonc</code>, <code>tui.jsonc</code>, <code>auth.json</code>, and</blockquote>
          <blockquote><code>AGENTS.md</code> in <code>~/.config/opencode/</code>, then replaces that directory's</blockquote>
          <blockquote><code>skills/</code> tree. Back up or merge an existing configuration first.</blockquote>
          <p>The installed files provide:</p>
          <ul>
            <li>the Kronk provider at <code>http://127.0.0.1:11435/v1</code>;</li>
            <li>registered coding models and a default model;</li>
            <li>direct access to the Kronk and gopls MCP servers;</li>
            <li>local API-key credentials;</li>
            <li>project instructions and the <code>kronk-mcp</code> and <code>writing-go</code> skills; and</li>
            <li>terminal UI preferences.</li>
          </ul>
          <p>The Kronk MCP service starts with the model server and listens at <code>http://localhost:9000/mcp</code> by default. Its <code>web_search</code> tool requires a Brave Search API key; <code>fuzzy_edit</code> does not. See Chapter 16 for configuration.</p>
          <h4 id="configure-a-coding-model">Configure a Coding Model</h4>
          <p>OpenCode's model name must match a model registered in its <code>provider.kronk.models</code> map. Kronk also needs a corresponding model variant in its model configuration. The shipped default is:</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT:
  context-window: 131072
  nseq-max: 2
  sampling-parameters:
    temperature: 0.6
    top_k: 20
    top_p: 0.95`}</code></pre>
          <p><code>/AGENT</code> is a configuration variant: it reuses the downloaded base model while applying settings intended for coding-agent workloads. The base model must already be present in Kronk's model directory.</p>
          <p>Installed servers use <code>~/.kronk/models/model_config.yaml</code> by default. The repository's development server instead points to <code>zarf/kms/model_config.yaml</code>, which already contains the models registered by the shipped OpenCode bundle. Restart the server after changing either file.</p>
          <p>Incremental Message Caching is enabled by default and is useful for growing agent conversations. <code>nseq-max: 2</code> allows two concurrent sequences, while the large context window accommodates accumulated messages and tool results. See Chapters 4 and 5 before changing those settings.</p>
          <h4 id="change-the-opencode-model">Change the OpenCode Model</h4>
          <p>To use another model:</p>
          <ol>
            <li>Download its base model into Kronk.</li>
            <li>Add an <code>/AGENT</code> variant to the active model configuration file.</li>
            <li>Register the same variant under <code>provider.kronk.models</code> in <code>~/.config/opencode/opencode.jsonc</code>.</li>
            <li>Set the top-level <code>model</code> field to <code>&lt;provider&gt;/&lt;model-id&gt;</code> or select the registered model from OpenCode.</li>
          </ol>
          <p>For example:</p>
          <pre className="code-block"><code className="language-yaml">{`organization/my-coding-model-Q8_0/AGENT:
  context-window: 131072
  nseq-max: 2
  sampling-parameters:
    temperature: 0.6
    top_k: 20
    top_p: 0.95`}</code></pre>
          <pre className="code-block"><code className="language-jsonc">{`"provider": {
  "kronk": {
    "models": {
      "organization/my-coding-model-Q8_0/AGENT": {
        "name": "My Coding Model",
        "limit": { "context": 131072, "output": 65536 }
      }
    }
  }
},
"model": "kronk/organization/my-coding-model-Q8_0/AGENT"`}</code></pre>
          <h4 id="use-a-protected-server">Use a Protected Server</h4>
          <p>The shipped <code>auth.json</code> uses <code>kronk</code> as a placeholder API key. That is enough when inference authentication is disabled. For a protected server, replace the <code>key</code> value under <code>kronk</code> in <code>~/.config/opencode/auth.json</code> with a valid user token that permits the endpoints OpenCode calls. Do not use the master admin token as an application credential.</p>
          <h3 id="143-openwebui">14.3 OpenWebUI</h3>
          <p>OpenWebUI is a self-hosted chat interface that works with Kronk.</p>
          <p>For OpenWebUI running directly on the host, configure an OpenAI connection with:</p>
          <ul>
            <li><strong>URL:</strong> <code>http://localhost:11435/v1</code></li>
            <li><strong>API key:</strong> any non-empty value in open mode, or a valid Kronk user token</li>
          </ul>
          <p>The repository also includes a Docker Compose service preconfigured to reach Kronk through <code>host.docker.internal</code>:</p>
          <pre className="code-block"><code className="language-shell">{`make owu-up
make owu-browse`}</code></pre>
          <p>OpenWebUI discovers available models through <code>GET /v1/models</code> and supports streaming chat, system prompts, model selection, and conversation history.</p>
          <h3 id="144-python-openai-sdk">14.4 Python OpenAI SDK</h3>
          <p>Use the official OpenAI Python library with Kronk.</p>
          <pre className="code-block"><code className="language-shell">{`pip install openai`}</code></pre>
          <p>Replace <code>&lt;model-id-from-v1-models&gt;</code> below with an ID reported by the server.</p>
          <pre className="code-block"><code className="language-python">{`import os

from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:11435/v1",
    api_key=os.getenv("KRONK_TOKEN", "kronk"),
)

response = client.chat.completions.create(
    model="<model-id-from-v1-models>",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Hello!"},
    ],
    stream=True,
)

for chunk in response:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")`}</code></pre>
          <h3 id="145-curl-and-other-http-clients">14.5 curl and Other HTTP Clients</h3>
          <p>Any HTTP client can call Kronk's REST API directly.</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -d '{
    "model": "<model-id-from-v1-models>",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'`}</code></pre>
          <p>Omit the authorization header in open mode. Streaming responses use Server-Sent Events; Chapter 9 documents endpoint behavior and Chapter 18 covers audio transcription.</p>
          <h3 id="146-langchain">14.6 LangChain</h3>
          <p>Use LangChain with Kronk via the OpenAI integration.</p>
          <pre className="code-block"><code className="language-shell">{`pip install langchain-openai`}</code></pre>
          <pre className="code-block"><code className="language-python">{`import os

from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="http://localhost:11435/v1",
    api_key=os.getenv("KRONK_TOKEN", "kronk"),
    model="<model-id-from-v1-models>",
    streaming=True,
)

response = llm.invoke("Explain quantum computing briefly.")
print(response.content)`}</code></pre>
          <hr />
          <p><em>Next: &lt;a href="chapter-15-observability.md"&gt;Chapter 15: Observability&lt;/a&gt;</em></p>
          <h2 id="chapter-15-observability">Chapter 15: Observability</h2>
          <p>Kronk exposes health checks on the Web API and runs a separate debug server for metrics, profiling, and runtime visualization. It can also export traces to an OTLP gRPC collector such as Grafana Tempo. Structured logs are written to standard output.</p>
          <h3 id="151-debug-and-health-endpoints">15.1 Debug and Health Endpoints</h3>
          <h4 id="debug-server">Debug Server</h4>
          <p>The main API binds to <code>0.0.0.0:11435</code> by default. Observability endpoints use a separate server at <code>0.0.0.0:11445</code>:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Path</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>/metrics</code></td>
                <td>Prometheus metrics</td>
              </tr>
              <tr>
                <td><code>/debug/pprof/</code></td>
                <td>Go profile index</td>
              </tr>
              <tr>
                <td><code>/debug/pprof/profile</code></td>
                <td>CPU profile</td>
              </tr>
              <tr>
                <td><code>/debug/pprof/heap</code></td>
                <td>Heap profile</td>
              </tr>
              <tr>
                <td><code>/debug/pprof/goroutine</code></td>
                <td>Goroutine profile</td>
              </tr>
              <tr>
                <td><code>/debug/pprof/trace</code></td>
                <td>Go execution trace</td>
              </tr>
              <tr>
                <td><code>/debug/statsviz</code></td>
                <td>Live Go runtime charts</td>
              </tr>
            </tbody>
          </table>
          <blockquote><strong>Security:</strong> The debug server has no authentication and its default address</blockquote>
          <blockquote>listens on every interface. Profiles and runtime data can reveal sensitive</blockquote>
          <blockquote>operational details. Bind it to loopback or restrict port <code>11445</code> at the</blockquote>
          <blockquote>network boundary unless remote scraping is required.</blockquote>
          <p>To bind it to loopback:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --debug-host localhost:11445`}</code></pre>
          <p>The equivalent environment variable is <code>KRONK_WEB_DEBUG_HOST=localhost:11445</code>.</p>
          <h4 id="health-checks">Health Checks</h4>
          <p>The unauthenticated health routes are served from the main API port:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/liveness
curl http://localhost:11435/v1/readiness`}</code></pre>
          <p>Liveness returns JSON containing <code>status</code>, <code>build</code>, <code>host</code>, and <code>GOMAXPROCS</code>. Readiness currently returns an empty <code>200 OK</code> when the HTTP service is running. It does not validate model files, inference libraries, devices, available memory, or a loaded model.</p>
          <h3 id="152-prometheus-metrics">15.2 Prometheus Metrics</h3>
          <p>Fetch the current metric inventory and its <code>HELP</code> descriptions from:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11445/metrics`}</code></pre>
          <p>The endpoint includes Go <code>go_<em>&lt;/code&gt; metrics, process &lt;code&gt;process_</em></code> metrics, and Kronk metrics. The following groups are useful starting points; <code>/metrics</code> is the authoritative list.</p>
          <h4 id="metric-groups">Metric Groups</h4>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Area</th>
                <th>Representative metrics</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>HTTP</td>
                <td><code>requests</code>, <code>errors</code>, <code>panics</code>, <code>goroutines</code></td>
              </tr>
              <tr>
                <td>Model loading</td>
                <td><code>model_load_seconds</code>, <code>model_load_proj_seconds</code></td>
              </tr>
              <tr>
                <td>Inference latency</td>
                <td><code>model_prompt_creation_seconds</code>, <code>model_prefill_seconds</code>, <code>model_prefill_ttft_seconds</code>, <code>model_request_ttft_seconds</code></td>
              </tr>
              <tr>
                <td>Requests</td>
                <td><code>chat_requests_total</code>, <code>chat_errors_total</code>, <code>chat_request_duration_seconds</code>, <code>chat_queue_wait_seconds</code></td>
              </tr>
              <tr>
                <td>Tokens</td>
                <td><code>usage_tokens_total</code>, <code>usage_tokens_per_second</code></td>
              </tr>
              <tr>
                <td>Model memory</td>
                <td><code>vram_total_bytes</code>, <code>vram_slot_memory_bytes</code></td>
              </tr>
              <tr>
                <td>Pool</td>
                <td><code>pool_acquire_total</code>, <code>pool_evictions_total</code>, <code>pool_items_in_pool</code>, <code>pool_max_items_in_pool</code>, <code>pool_active_streams</code>, <code>pool_inflight_loads</code></td>
              </tr>
              <tr>
                <td>Resource manager</td>
                <td><code>resman_ram_used_bytes</code>, <code>resman_device_used_bytes</code>, <code>resman_reservation_bytes</code>, <code>resman_reserve_rejections_total</code></td>
              </tr>
              <tr>
                <td>IMC</td>
                <td><code>imc_snapshot_skipped_total</code>, <code>imc_pure_hit_stale_session_total</code></td>
              </tr>
            </tbody>
          </table>
          <p>Most model and request metrics have a <code>model_id</code> label. Histograms expose <code>_bucket</code>, <code>_sum</code>, and <code>_count</code> series. Counters such as <code>usage_tokens_total</code> should normally be queried with <code>rate()</code> or <code>increase()</code>.</p>
          <p>For an external Prometheus process on the same host:</p>
          <pre className="code-block"><code className="language-yaml">{`scrape_configs:
  - job_name: "kronk"
    static_configs:
      - targets: ["localhost:11445"]
    scrape_interval: 15s`}</code></pre>
          <p>When Prometheus runs in Docker while Kronk runs on the host, use <code>host.docker.internal:11445</code>, as the repository's configuration does.</p>
          <h4 id="promql-examples">PromQL Examples</h4>
          <p>Average end-to-end time to first token by model:</p>
          <pre className="code-block"><code className="language-promql">{`rate(model_request_ttft_seconds_sum[5m])
  / rate(model_request_ttft_seconds_count[5m])`}</code></pre>
          <p>P99 time to first token by model:</p>
          <pre className="code-block"><code className="language-promql">{`histogram_quantile(0.99,
  sum by (le, model_id) (rate(model_request_ttft_seconds_bucket[5m])))`}</code></pre>
          <p>Token throughput by model and kind:</p>
          <pre className="code-block"><code className="language-promql">{`sum by (model_id, kind) (rate(usage_tokens_total[5m]))`}</code></pre>
          <h3 id="153-bundled-observability-stack">15.3 Bundled Observability Stack</h3>
          <p>The repository includes a Docker Compose stack containing Grafana, Prometheus, Tempo, Loki, and Promtail. It provisions the data sources and a Kronk dashboard without manual Grafana setup.</p>
          <p>Download the pinned images once, start the stack, and open Grafana:</p>
          <pre className="code-block"><code className="language-shell">{`make install-docker
make grafana-up
make grafana-browse`}</code></pre>
          <p>Grafana is served at <code>http://localhost:3100/</code>. Prometheus scrapes the host's Kronk debug server, and Tempo accepts OTLP gRPC traces on port <code>4317</code>.</p>
          <p>Stop the stack with:</p>
          <pre className="code-block"><code className="language-shell">{`make grafana-down`}</code></pre>
          <h3 id="154-opentelemetry-tracing">15.4 OpenTelemetry Tracing</h3>
          <p>Kronk exports OpenTelemetry traces over unencrypted OTLP gRPC. The defaults are:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Setting</th>
                <th>Flag</th>
                <th>Environment variable</th>
                <th>Default</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Collector</td>
                <td><code>--tempo-host</code></td>
                <td><code>KRONK_TEMPO_HOST</code></td>
                <td><code>localhost:4317</code></td>
              </tr>
              <tr>
                <td>Service</td>
                <td><code>--tempo-service-name</code></td>
                <td><code>KRONK_TEMPO_SERVICE_NAME</code></td>
                <td><code>kronk</code></td>
              </tr>
              <tr>
                <td>Sampling</td>
                <td><code>--tempo-probability</code></td>
                <td><code>KRONK_TEMPO_PROBABILITY</code></td>
                <td><code>0.25</code></td>
              </tr>
            </tbody>
          </table>
          <p>No collector is required for startup. Until one is reachable, spans are non-recording and Kronk probes the configured address every 60 seconds. A later successful connection activates tracing without restarting the server.</p>
          <p>For a remote collector or a different sampling rate:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --tempo-host otel-collector.example.com:4317 \\
  --tempo-probability 0.05`}</code></pre>
          <p>The HTTP layer accepts W3C Trace Context headers and creates request and route spans. Inference adds spans for request preparation, cache processing, queueing, prefill, token generation, and selected model operations. Liveness and readiness routes are excluded from sampling.</p>
          <p>Set the probability to <code>1.0</code> when every trace is needed for focused debugging, or <code>0.0</code> to disable sampling. Choose a lower nonzero value for sustained production traffic based on its volume and storage budget.</p>
          <h3 id="155-profiling-and-runtime-visualization">15.5 Profiling and Runtime Visualization</h3>
          <p>Use Go's pprof tooling against the debug server. For example:</p>
          <pre className="code-block"><code className="language-shell">{`go tool pprof http://localhost:11445/debug/pprof/profile?seconds=30
go tool pprof http://localhost:11445/debug/pprof/heap
curl http://localhost:11445/debug/pprof/goroutine?debug=2`}</code></pre>
          <p>Start pprof's interactive web interface with:</p>
          <pre className="code-block"><code className="language-shell">{`go tool pprof -http=localhost:8081 \\
  http://localhost:11445/debug/pprof/profile?seconds=30`}</code></pre>
          <p>Statsviz displays live heap, allocation, goroutine, garbage collection, and scheduler charts at:</p>
          <pre className="code-block"><code className="language-text">{`http://localhost:11445/debug/statsviz`}</code></pre>
          <p>The same unauthenticated-access warning as the rest of the debug server applies to pprof and Statsviz.</p>
          <h3 id="156-logging">15.6 Logging</h3>
          <p>Kronk logs structured JSON to stdout by default.</p>
          <p>Log records include the service name, source location, severity, and a trace ID. A generated request ID is used when a sampled OpenTelemetry trace ID is not available, so logs remain correlatable even without a collector.</p>
          <p>Sensitive prompts, responses, and detailed model configuration are omitted by default. They can be included temporarily for local debugging:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --insecure-logging`}</code></pre>
          <p>The environment equivalent is <code>KRONK_INSECURE_LOGGING=true</code>. Do not enable this on production systems or when logs are sent to a shared collector.</p>
          <hr />
          <p><em>Next: &lt;a href="chapter-16-mcp-service.md"&gt;Chapter 16: MCP Service&lt;/a&gt;</em></p>
          <h2 id="chapter-16-mcp-service">Chapter 16: MCP Service</h2>
          <p>Kronk includes a <a href="https://modelcontextprotocol.io/">Model Context Protocol (MCP)</a> service with two tools:</p>
          <ul>
            <li><strong>&lt;code&gt;web_search&lt;/code&gt;</strong> searches the web through the <a href="https://brave.com/search/api/">Brave Search API</a>.</li>
            <li><strong>&lt;code&gt;fuzzy_edit&lt;/code&gt;</strong> replaces a uniquely matching block of text in a local file.</li>
          </ul>
          <p>MCP clients discover and invoke these tools through Streamable HTTP. The repository includes a ready-to-use OpenCode configuration, described in Chapter 14.</p>
          <h3 id="161-architecture-and-security">16.1 Architecture and Security</h3>
          <p>The service can run in either of these modes:</p>
          <ul>
            <li><strong>Embedded (default):</strong> <code>kronk server start</code> listens on <code>localhost:9000</code> when <code>KRONK_MCP_ENABLED</code> is true and <code>KRONK_MCP_HOST</code> is empty.</li>
            <li><strong>Standalone:</strong> <code>make mcp-server</code> runs the MCP service without the model server. It listens on <code>localhost:9000</code> and starts a debug server on <code>localhost:9010</code> by default.</li>
          </ul>
          <p>The standalone service can also be run directly:</p>
          <pre className="code-block"><code className="language-shell">{`go run cmd/server/api/services/mcp/main.go`}</code></pre>
          <p><code>fuzzy_edit</code> can read and overwrite any file accessible to the service process when given its absolute path. Keep MCP and debug endpoints bound to loopback by default. MCP bearer authentication can be enabled as described below, but it does not provide TLS or protect the separate debug endpoint. If remote access is required, also use TLS, firewall the listeners, and keep debug endpoints private.</p>
          <p>MCP sessions and replay data are stored in process memory. A restart invalidates existing session IDs. Compliant clients reinitialize after the server responds to a stale session with HTTP 404.</p>
          <h3 id="162-prerequisites">16.2 Prerequisites</h3>
          <p><code>web_search</code> requires a Brave Search API key. Obtain one from the <a href="https://brave.com/search/api/">Brave Search API</a> site. Search queries are sent to Brave and are also included in Kronk's structured logs.</p>
          <p><code>fuzzy_edit</code> requires no external credentials. It operates with the same filesystem permissions as the Kronk process.</p>
          <h3 id="163-configuration">16.3 Configuration</h3>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Variable</th>
                <th>Purpose</th>
                <th>Default</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>KRONK_MCP_ENABLED</code></td>
                <td>Enable the embedded MCP listener</td>
                <td><code>true</code></td>
              </tr>
              <tr>
                <td><code>KRONK_MCP_AUTH_ENABLED</code></td>
                <td>Require a Kronk admin bearer token for embedded MCP</td>
                <td><code>false</code></td>
              </tr>
              <tr>
                <td><code>KRONK_MCP_BRAVE_API_KEY</code></td>
                <td>Brave key for embedded mode</td>
                <td>—</td>
              </tr>
              <tr>
                <td><code>KRONK_MCP_HOST</code></td>
                <td>Non-empty value disables embedded MCP</td>
                <td>—</td>
              </tr>
              <tr>
                <td><code>MCP_MCP_AUTH_ENABLED</code></td>
                <td>Require admin bearer authentication for standalone MCP</td>
                <td><code>false</code></td>
              </tr>
              <tr>
                <td><code>MCP_MCP_BRAVE_API_KEY</code></td>
                <td>Brave key for standalone mode</td>
                <td>—</td>
              </tr>
              <tr>
                <td><code>MCP_MCP_HOST</code></td>
                <td>Standalone MCP listen address</td>
                <td><code>localhost:9000</code></td>
              </tr>
              <tr>
                <td><code>MCP_AUTH_HOST</code></td>
                <td>Auth gRPC service used by protected standalone MCP</td>
                <td>—</td>
              </tr>
              <tr>
                <td><code>MCP_WEB_DEBUG_HOST</code></td>
                <td>Standalone debug listen address</td>
                <td><code>localhost:9010</code></td>
              </tr>
            </tbody>
          </table>
          <p>Start the model server with embedded MCP:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_MCP_BRAVE_API_KEY=<your-brave-api-key>
kronk server start`}</code></pre>
          <p>The corresponding CLI option is <code>--mcp-brave-api-key</code>. Disable embedded MCP with <code>--mcp-enabled=false</code> or <code>KRONK_MCP_ENABLED=false</code>. To use a separately managed MCP service, set <code>KRONK_MCP_HOST</code> or pass <code>--mcp-host</code>; a non-empty value only prevents the embedded service from starting. Kronk does not connect or proxy to that address.</p>
          <p>Protect embedded MCP with the existing Kronk JWT system:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --mcp-auth-enabled`}</code></pre>
          <p>This requires an admin bearer token on every MCP request and also enables administrative authentication for the REST API and BUI. Configure the MCP client to send <code>Authorization: Bearer &lt;admin-token&gt;</code>. Application tokens with inference endpoint grants are not sufficient for MCP access. Before exposing the model server outside a trusted host, replace the BUI's default <code>kronk</code> password as described in <a href="chapter-13-browser-ui.md#133-authentication-and-session-behavior">Chapter 13</a> or disable the BUI; otherwise that known password can be exchanged for an admin session.</p>
          <p>Start the standalone service with:</p>
          <pre className="code-block"><code className="language-shell">{`export MCP_MCP_BRAVE_API_KEY=<your-brave-api-key>
make mcp-server`}</code></pre>
          <p>To protect a standalone MCP listener, connect it to an auth service that has authentication enabled:</p>
          <pre className="code-block"><code className="language-shell">{`export MCP_MCP_AUTH_ENABLED=true
export MCP_AUTH_HOST=localhost:6000
make mcp-server`}</code></pre>
          <p>The standalone MCP service then requires an admin token issued by that auth service. Startup fails if MCP authentication is enabled without <code>MCP_AUTH_HOST</code>.</p>
          <h3 id="164-available-tools">16.4 Available Tools</h3>
          <h4 id="web_search">web_search</h4>
          <p>Returns matching page titles, URLs, and descriptions as plain text.</p>
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
                <td>Result count; default 10, maximum 20</td>
              </tr>
              <tr>
                <td><code>country</code></td>
                <td>string</td>
                <td>No</td>
                <td>Search country code, such as <code>US</code>, <code>GB</code>, or <code>DE</code></td>
              </tr>
              <tr>
                <td><code>freshness</code></td>
                <td>string</td>
                <td>No</td>
                <td><code>pd</code>, <code>pw</code>, <code>pm</code>, or <code>py</code> for the past day, week, month, or year</td>
              </tr>
              <tr>
                <td><code>safesearch</code></td>
                <td>string</td>
                <td>No</td>
                <td><code>off</code>, <code>moderate</code>, or <code>strict</code>; Brave defaults to <code>moderate</code></td>
              </tr>
            </tbody>
          </table>
          <h4 id="fuzzy_edit">fuzzy_edit</h4>
          <p>Replaces one occurrence of <code>old_string</code> with <code>new_string</code> in an existing file. Use it as a fallback when the client's normal exact-match edit fails because of whitespace or line-ending differences.</p>
          <p>The tool tries these matching tiers in order:</p>
          <ol>
            <li>Exact text.</li>
            <li>Text after normalizing CRLF and LF line endings.</li>
            <li>Lines compared without surrounding whitespace.</li>
          </ol>
          <p>Each tier must identify exactly one block. An absent or ambiguous match returns an error without modifying the file. The replacement is inserted as provided.</p>
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
                <td>Absolute path to the existing file</td>
              </tr>
              <tr>
                <td><code>old_string</code></td>
                <td>string</td>
                <td>Yes</td>
                <td>Unique text to replace</td>
              </tr>
              <tr>
                <td><code>new_string</code></td>
                <td>string</td>
                <td>Yes</td>
                <td>Replacement text</td>
              </tr>
            </tbody>
          </table>
          <h3 id="165-client-configuration">16.5 Client Configuration</h3>
          <p>Configure an MCP-compatible client to use the Streamable HTTP endpoint:</p>
          <pre className="code-block"><code className="language-text">{`http://localhost:9000/mcp`}</code></pre>
          <h4 id="opencode">OpenCode</h4>
          <p>Install the repository's complete OpenCode bundle with <code>make agents-default-opencode</code>; review its overwrite warning in Chapter 14 first. The installed configuration includes:</p>
          <pre className="code-block"><code className="language-jsonc">{`{
  "mcp": {
    "kronk": {
      "type": "remote",
      "url": "http://localhost:9000/mcp"
    }
  }
}`}</code></pre>
          <p>With the shipped <code>kronk</code> server key, OpenCode exposes the tools as <code>kronk_web_search</code> and <code>kronk_fuzzy_edit</code>.</p>
          <h3 id="166-testing-with-curl">16.6 Testing with curl</h3>
          <p>The makefile provides commands for a complete stateful MCP handshake. First, initialize a session and copy the <code>Mcp-Session-Id</code> response header:</p>
          <pre className="code-block"><code className="language-shell">{`make curl-mcp-init`}</code></pre>
          <p>Then send the required initialized notification:</p>
          <pre className="code-block"><code className="language-shell">{`make curl-mcp-initialized SESSIONID=<session-id>`}</code></pre>
          <p>The session can now list and call tools:</p>
          <pre className="code-block"><code className="language-shell">{`make curl-mcp-tools-list SESSIONID=<session-id>
make curl-mcp-web-search SESSIONID=<session-id>`}</code></pre>
          <p>When MCP authentication is enabled, include <code>Authorization: Bearer &lt;admin-token&gt;</code> in every initialization, notification, tool-listing, tool-call, and session-deletion request.</p>
          <p>If the service restarts, initialize a new session instead of reusing the old ID.</p>
          <hr />
          <p><em>Next: &lt;a href="chapter-17-troubleshooting.md"&gt;Chapter 17: Troubleshooting&lt;/a&gt;</em></p>
          <h2 id="chapter-17-troubleshooting">Chapter 17: Troubleshooting</h2>
          <p>This chapter is a symptom-first guide to common failures. For configuration details, follow the links to the chapter that owns that subsystem.</p>
          <h3 id="171-start-with-diagnostics">17.1 Start with Diagnostics</h3>
          <p>Run the built-in diagnostic before changing configuration:</p>
          <pre className="code-block"><code className="language-shell">{`kronk diagnose`}</code></pre>
          <p>It reports Kronk and yzma versions, host hardware, the active llama.cpp installation, detected compute devices, and a small benchmark. It does not download anything unless <code>--install</code> is supplied. Useful variants are:</p>
          <pre className="code-block"><code className="language-shell">{`kronk diagnose --no-bench
kronk diagnose --format json
kronk diagnose --format yaml`}</code></pre>
          <p>Run the server in the foreground to see JSON logs on stdout:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start`}</code></pre>
          <p>For a server started with <code>-d</code>, follow its log file with:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server logs`}</code></pre>
          <p><code>--insecure-logging</code> includes prompts, responses, and detailed model configuration. Use it only for local diagnosis because the output can contain sensitive data. <code>--llama-log 1</code> enables lower-level llama.cpp messages; <code>--llama-log 0</code> disables them.</p>
          <p>The main API exposes two unauthenticated process health checks:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/liveness
curl -i http://localhost:11435/v1/readiness`}</code></pre>
          <p>Readiness currently returns an empty <code>200 OK</code> when the HTTP service is running. It does not verify libraries, devices, memory, loaded models, or inference. Metrics and profiles are served from the unauthenticated debug server; see <a href="chapter-15-observability.md#151-debug-and-health-endpoints">Chapter 15</a> before exposing that port.</p>
          <h3 id="172-libraries-and-devices">17.2 Libraries and Devices</h3>
          <h4 id="`unable-to-load-library`">`unable to load library`</h4>
          <p>Install the bundle selected for the current operating system, architecture, and processor:</p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local
kronk devices`}</code></pre>
          <p>If detection selected the wrong backend, install explicitly:</p>
          <pre className="code-block"><code className="language-shell">{`KRONK_PROCESSOR=metal kronk libs --local
KRONK_PROCESSOR=cuda kronk libs --local
KRONK_PROCESSOR=rocm kronk libs --local
KRONK_PROCESSOR=vulkan kronk libs --local
KRONK_PROCESSOR=cpu kronk libs --local`}</code></pre>
          <p>Not every combination is published. Check the live matrix with <code>kronk libs --list-combinations</code>. See <a href="chapter-02-installation.md#24-libraries">Chapter 2: Libraries</a> for installation and path details.</p>
          <h4 id="nvidia-is-visible-but-llamacpp-uses-the-cpu">NVIDIA is visible but llama.cpp uses the CPU</h4>
          <p><code>nvidia-smi</code> proves that the driver is available, but a native CUDA bundle also needs the CUDA runtime libraries against which it was linked. On Linux, find the active library path in <code>kronk diagnose</code>, then inspect the backend for unresolved dependencies:</p>
          <pre className="code-block"><code className="language-shell">{`ldd <lib-path>/libggml-cuda.so | grep -iE 'not found|cudart|cublas'`}</code></pre>
          <p>Install the matching CUDA runtime packages for the bundle and operating system. For containers, use the current <code>latest-cuda</code> image and grant GPU access with <code>--runtime=nvidia --gpus all</code>; the required runtime libraries are included in that image.</p>
          <h4 id="a-library-update-introduced-crashes-or-bad-output">A library update introduced crashes or bad output</h4>
          <p>The normal CLI installs Kronk's pinned default. <code>--upgrade</code> and the server's <code>--allow-upgrade=true</code> opt into newer llama.cpp releases. List installed bundles and pin a known-good version when investigating a regression:</p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --list-installs
kronk libs --local --version=b5490
kronk server start --lib-version=b5490`}</code></pre>
          <p>Unset <code>KRONK_LIB_VERSION</code> after the pinned default contains the required fix. Libraries are not hot-reloaded; restart the server after switching <code>KRONK_LIB_PATH</code>.</p>
          <h3 id="173-models-catalog-and-storage">17.3 Models, Catalog, and Storage</h3>
          <h4 id="a-downloaded-model-is-not-listed">A downloaded model is not listed</h4>
          <p>Kronk stores data beneath <code>KRONK_BASE_PATH</code>, which defaults to <code>~/.kronk</code>. Inspect the model index rather than relying on a hard-coded directory:</p>
          <pre className="code-block"><code className="language-shell">{`kronk model list --local
kronk model index --local`}</code></pre>
          <p>Indexing scans model files and checks available size and SHA metadata. An arbitrary GGUF without corresponding checksum metadata cannot receive the same integrity validation as a model downloaded by Kronk.</p>
          <h4 id="a-model-is-missing-incomplete-or-corrupt">A model is missing, incomplete, or corrupt</h4>
          <p>Pull it again using any supported source form:</p>
          <pre className="code-block"><code className="language-shell">{`kronk model pull <model-id> --local`}</code></pre>
          <p><code>model pull</code> checks the catalog and automatically walks configured providers when an ID has not been resolved before. A separate <code>model resolve</code> step is not normally required. Interrupted downloads are resumable; if a file remains invalid, remove that model through Kronk and pull it again:</p>
          <pre className="code-block"><code className="language-shell">{`kronk model remove <model-id> --local
kronk model pull <model-id> --local`}</code></pre>
          <p>For a gated or private Hugging Face repository, provide a read token:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_HF_TOKEN=hf_xxx
kronk model pull <model-id> --local`}</code></pre>
          <h4 id="`catalogyaml`-was-hand-edited-and-no-longer-parses">`catalog.yaml` was hand-edited and no longer parses</h4>
          <p>The default catalog is <code>&lt;base&gt;/catalog/catalog.yaml</code>. Restore valid YAML from a backup before running catalog commands; those commands must parse the file and cannot repair malformed YAML. Do not use <code>catalog remove</code> as a syntax-repair tool because it also removes the selected model's downloaded files.</p>
          <p>Catalog administration is covered in <a href="chapter-08-model-server.md#86-catalog-operations">Chapter 8</a>.</p>
          <h3 id="174-memory-and-performance">17.4 Memory and Performance</h3>
          <h4 id="`unable-to-init-context`-or-`unable-to-get-memory`">`unable to init context` or `unable to get memory`</h4>
          <p>The model, runtime buffers, and configured context do not fit. Change one variable at a time:</p>
          <ol>
            <li>Reduce <code>context-window</code>.</li>
            <li>Reduce <code>nseq-max</code>.</li>
            <li>Use a quantized KV cache such as <code>q8_0</code>.</li>
            <li>Move KV state or model layers to CPU.</li>
            <li>Choose a smaller or more heavily quantized GGUF.</li>
          </ol>
          <p>Use the BUI VRAM Calculator for a model-specific estimate and retain headroom. See <a href="chapter-03-model-configuration.md#36-memory-planning-and-quantization">Chapter 3: Memory Planning</a>.</p>
          <h4 id="`input-tokens-[n]-exceed-context-window-[m]`">`input tokens [N] exceed context window [M]`</h4>
          <p>The rendered prompt is already larger than the configured context. Shorten the conversation or system prompt, or increase <code>context-window</code> if memory permits. Cached prefix tokens still consume context capacity.</p>
          <h4 id="`the-context-window-is-full`">`the context window is full`</h4>
          <p>Input plus generated tokens exhausted the context during inference. Request fewer output tokens, shorten the input, or increase the context. YaRN may extend supported RoPE models, but it is not a generic memory fix; follow <a href="chapter-07-yarn-extended-context.md">Chapter 7</a>.</p>
          <h4 id="slow-inference-or-slow-time-to-first-token">Slow inference or slow time to first token</h4>
          <p>Start with <code>kronk diagnose</code> and confirm that llama.cpp sees the expected GPU. A cold request includes model loading, and a large uncached prompt includes prefill. Partial CPU offload can reduce token throughput. Compare representative requests after the model is warm rather than relying on the first request.</p>
          <p>Use Chapter 15's request, queue, prefill, TTFT, token-rate, and pool metrics to separate loading, waiting, prompt processing, and generation. IMC-specific diagnosis is below.</p>
          <h3 id="175-requests-and-streaming">17.5 Requests and Streaming</h3>
          <h4 id="`context-deadline-exceeded`">`context deadline exceeded`</h4>
          <p>The source of the deadline matters:</p>
          <ul>
            <li>a client or reverse proxy may cancel first;</li>
            <li>chat handlers impose a 180-minute request context deadline;</li>
            <li>the HTTP server defaults to a 30-second read timeout and a 60-minute write timeout.</li>
          </ul>
          <p><code>--read-timeout</code> covers reading the request, not model execution. Increase <code>--write-timeout</code> only when a long response is being cut off by the server:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --write-timeout 90m`}</code></pre>
          <p>Check client and proxy timeouts separately. Large prompts and queued requests should be diagnosed with the timing metrics rather than masking them with a larger HTTP read timeout.</p>
          <h4 id="a-stream-stops-or-does-not-parse">A stream stops or does not parse</h4>
          <p>OpenAI-compatible chat streaming uses SSE records of the form:</p>
          <pre className="code-block"><code className="language-text">{`data: {"id":"...","choices":[...]}

data: [DONE]`}</code></pre>
          <p>Kronk sends an SSE comment every 15 seconds as a keepalive. Clients must ignore comment lines and handle normal <code>finish_reason</code> values. <code>/v1/messages</code> uses named <code>event:</code> records instead of the OpenAI chat format.</p>
          <p>A missing <code>[DONE]</code> commonly means the client disconnected, a proxy timed out, or the server encountered an error. Correlate the request with its <code>trace_id</code> in the JSON logs.</p>
          <h3 id="176-authentication">17.6 Authentication</h3>
          <p>HTTP clients receive a generic authentication failure. The server's JSON log contains the specific cause, commonly one of these:</p>
          <ul>
            <li>no authorization header — add a bearer token;</li>
            <li><code>invalid token:</code> — the JWT is malformed, expired, or signed by an unknown key;</li>
            <li><code>not authorized:</code> — the token lacks the endpoint grant;</li>
            <li><code>rate limit exceeded:</code> — the grant's current window is exhausted.</li>
          </ul>
          <p>Use the generated master token for administration on a default local setup:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)`}</code></pre>
          <p>Create a replacement user token with only the required grants:</p>
          <pre className="code-block"><code className="language-shell">{`kronk security token create \\
  --duration 720h \\
  --endpoints chat-completions,embeddings,rerank,responses,messages,tokenize,transcriptions`}</code></pre>
          <p>Rate limits use forms such as <code>chat-completions:10000/day</code>. Token creation, key rotation, and production hardening are covered in <a href="chapter-12-security-authentication.md">Chapter 12</a>.</p>
          <h3 id="177-imc">17.7 IMC</h3>
          <p>IMC is enabled by default. It externalizes cached session state to RAM by default or to the configured disk session store. See <a href="chapter-05-message-caching.md">Chapter 5</a> for its lifecycle and settings.</p>
          <h4 id="every-turn-rebuilds-the-cache">Every turn rebuilds the cache</h4>
          <p>Common causes are changed earlier messages, changed template inputs, a prompt below <code>cache-min-tokens</code>, or cache pressure. Relevant JSON log statuses include <code>session[N] mismatch</code>, <code>sys-prompt-match</code>, <code>token prefix match found</code>, <code>no usable token prefix match</code>, and <code>kv-pressure-evict</code>.</p>
          <p>Keep earlier conversation messages stable and use a deterministic template. Increase <code>nseq-max</code> only when additional inference concurrency and its memory cost are both appropriate; IMC maintains more session identities than active decode slots.</p>
          <h4 id="`server-busy-processing-other-requests-try-again-shortly`">`server busy processing other requests, try again shortly`</h4>
          <p>No IMC session was available. Depending on the planning path, Kronk may return this immediately or after waiting up to <code>cache-slot-timeout</code>. It is a transient request failure: wait and retry from the client. If it is frequent, inspect long-running requests and queue/cache metrics before increasing <code>nseq-max</code>. Increasing <code>cache-slot-timeout</code> affects only paths that wait for a session.</p>
          <h4 id="`imc-restore-failed`-or-`imc-extend-stale`">`imc restore failed` or `imc extend stale`</h4>
          <p>The current request fails. Retry it from the client; the server does not automatically repeat the request. Repeated restore failures warrant checking memory pressure, the session-store configuration, and nearby low-level errors. Reducing <code>context-window</code> or concurrency can lower memory pressure.</p>
          <h3 id="178-mcp">17.8 MCP</h3>
          <p>The endpoint is <code>http://localhost:9000/mcp</code> without a trailing slash and uses Streamable HTTP. Common failures are:</p>
          <ul>
            <li><strong>404 after a server restart:</strong> discard the stale in-memory session ID and initialize a new MCP session.</li>
            <li><strong>Brave authentication failure:</strong> set <code>KRONK_MCP_BRAVE_API_KEY</code> for embedded mode or <code>MCP_MCP_BRAVE_API_KEY</code> for standalone mode before startup.</li>
            <li><strong>Unknown &lt;code&gt;kronk_fuzzy_edit&lt;/code&gt;:</strong> with the shipped OpenCode configuration, the exposed names are <code>kronk_fuzzy_edit</code> and <code>kronk_web_search</code>.</li>
            <li><strong>&lt;code&gt;old_string not found&lt;/code&gt;:</strong> read the current file and provide one unique block; the same error also covers ambiguous matches.</li>
            <li><strong>Embedded server absent:</strong> <code>KRONK_MCP_ENABLED=false</code> or a non-empty <code>KRONK_MCP_HOST</code> disables it. The host setting does not configure a proxy or client connection.</li>
            <li><strong>401 Unauthorized:</strong> when MCP authentication is enabled, send the same Kronk admin bearer token on every request, including session initialization and notifications. Inference-scoped application tokens are not accepted.</li>
          </ul>
          <p>MCP authentication is disabled by default, and <code>fuzzy_edit</code> has the process's filesystem access. Keep it on loopback unless bearer authentication, TLS, and network restrictions are configured. See <a href="chapter-16-mcp-service.md">Chapter 16</a> for configuration and the complete handshake.</p>
          <h3 id="179-ports-processes-and-permissions">17.9 Ports, Processes, and Permissions</h3>
          <p>Default listeners are <code>11435</code> for the API, <code>11445</code> for model-server debugging, and <code>9000</code> for embedded MCP. Standalone MCP also starts a debug listener on <code>9010</code>. Find a conflicting process before changing ports:</p>
          <pre className="code-block"><code className="language-shell">{`lsof -nP -iTCP:11435 -sTCP:LISTEN
lsof -nP -iTCP:9000 -sTCP:LISTEN`}</code></pre>
          <p>Move the API or debug listener with <code>--api-host</code> or <code>--debug-host</code>. Standalone MCP uses <code>MCP_MCP_HOST</code> and <code>MCP_WEB_DEBUG_HOST</code>.</p>
          <p>Detached mode stores <code>kronk.pid</code> and <code>kronk.log</code> under <code>KRONK_BASE_PATH</code>. If <code>kronk server stop</code> encounters a stale PID, verify that no Kronk process owns the API or debug port before removing only the stale PID file.</p>
          <p>BadgerDB also permits only one model-server process to use the rate-limit database. A lock error means another process owns <code>&lt;base&gt;/badger</code>; stop that process. Do not delete Badger's <code>LOCK</code> file while a server may be running.</p>
          <p>For permission errors, make the selected base path writable by the service user. The server enforces mode <code>0700</code> on <code>&lt;base&gt;/keys</code> and <code>0600</code> on private key files. Avoid recursively making credentials readable by other users.</p>
          <p>Whisper-specific failures are listed in <a href="chapter-18-bucky.md#1811-troubleshooting">Chapter 18 §18.11</a>.</p>
          <h3 id="1710-reporting-a-problem">17.10 Reporting a Problem</h3>
          <p>Include:</p>
          <ul>
            <li><code>kronk diagnose --format json</code> output, or <code>--no-bench</code> if benchmarking fails;</li>
            <li>the Kronk version and relevant JSON log records;</li>
            <li>operating system, architecture, GPU, and driver/runtime versions;</li>
            <li>model ID and non-default configuration;</li>
            <li>the complete error text and reproducible steps; and</li>
            <li>whether the failure occurs on the first request, after warmup, or only under concurrency.</li>
          </ul>
          <p>Remove tokens, prompts, responses, filesystem secrets, and other sensitive values before sharing diagnostic output.</p>
          <hr />
          <p><em>Next: &lt;a href="chapter-18-bucky.md"&gt;Chapter 18: Bucky (Audio Transcription)&lt;/a&gt;</em></p>
          <h2 id="chapter-18-bucky-audio-transcription">Chapter 18: Bucky (Audio Transcription)</h2>
          <p>Bucky is Kronk's speech-to-text subsystem. It uses <a href="https://github.com/ggerganov/whisper.cpp"><code>whisper.cpp</code></a> and is available through:</p>
          <ul>
            <li>the <code>/v1/audio/transcriptions</code> HTTP endpoint;</li>
            <li>the Browser UI (BUI) Translator;</li>
            <li>the <code>kronk bucky</code> management commands; and</li>
            <li>the Go packages under <code>sdk/bucky</code>.</li>
          </ul>
          <p>Using Bucky requires both a compatible whisper.cpp library bundle and a Whisper model. The libraries run the inference engine; the model contains the speech-recognition weights.</p>
          <p>Developer-level package, lifecycle, and test information belongs in <a href="chapter-19-developer-guide.md">Chapter 19: Developer Guide</a>.</p>
          <h3 id="181-overview">18.1 Overview</h3>
          <p>Bucky supports these common workflows:</p>
          <ul>
            <li>transcribe an uploaded or recorded audio file;</li>
            <li>translate speech from a supported language into English;</li>
            <li>return plain text, JSON, SRT, or WebVTT;</li>
            <li>transcribe separate audio channels as separate speakers through the SDK; and</li>
            <li>consume partial and final transcript events from live audio through the SDK.</li>
          </ul>
          <p>The HTTP endpoint follows the OpenAI audio transcription request shape. It is protected by Kronk's <code>transcriptions</code> authentication permission when server authentication is enabled.</p>
          <p>Whisper models use GGML <code>.bin</code> files and are separate from the GGUF models used by Kronk's language-model backend. Bucky models and language models share the server's memory budget and pool controls.</p>
          <h3 id="182-install-whisper-libraries">18.2 Install Whisper Libraries</h3>
          <p>Install the default library bundle for the current host:</p>
          <pre className="code-block"><code className="language-sh">{`kronk bucky libs`}</code></pre>
          <p>The supported bundles are:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Operating system</th>
                <th>Architecture</th>
                <th>Processors</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>macOS</td>
                <td><code>amd64</code>, <code>arm64</code></td>
                <td><code>cpu</code>, <code>metal</code></td>
              </tr>
              <tr>
                <td>Linux</td>
                <td><code>amd64</code>, <code>arm64</code></td>
                <td><code>cpu</code>, <code>cuda</code>, <code>vulkan</code></td>
              </tr>
              <tr>
                <td>Windows</td>
                <td><code>amd64</code></td>
                <td><code>cpu</code>, <code>cuda</code></td>
              </tr>
            </tbody>
          </table>
          <p>Use the CLI as the current source of truth for available combinations:</p>
          <pre className="code-block"><code className="language-sh">{`kronk bucky libs --list-combinations`}</code></pre>
          <p>Other useful operations are:</p>
          <pre className="code-block"><code className="language-sh">{`# Install a particular version for the current host.
kronk bucky libs --version=v1.7.0

# Install another bundle alongside the active one.
kronk bucky libs --install --arch=amd64 --os=linux --processor=cuda

# List or remove installed bundles.
kronk bucky libs --list-installs
kronk bucky libs --remove-install --arch=amd64 --os=linux --processor=cuda`}</code></pre>
          <p>The commands use the running model server by default. Add <code>--local</code> to manage the files directly without a server.</p>
          <p>The BUI's <strong>Whisper Libraries</strong> screen provides the same installation and removal operations. If Bucky failed to initialize because its libraries were missing, install a compatible bundle and <strong>restart the server</strong>. The running server does not automatically retry Bucky initialization.</p>
          <p>Libraries are installed below <code>~/.kronk/bucky-libraries/</code> by default. Kronk normally selects the bundle for the current platform. To select a specific installed bundle, set its directory before starting the server:</p>
          <pre className="code-block"><code className="language-sh">{`export KRONK_BUCKY_LIB_PATH=~/.kronk/bucky-libraries/linux/amd64/cuda`}</code></pre>
          <p>The library tools also recognize these platform overrides:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Variable</th>
                <th>Values</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>KRONK_ARCH</code></td>
                <td><code>amd64</code>, <code>arm64</code></td>
              </tr>
              <tr>
                <td><code>KRONK_OS</code></td>
                <td><code>linux</code>, <code>darwin</code>, <code>windows</code></td>
              </tr>
              <tr>
                <td><code>KRONK_PROCESSOR</code></td>
                <td><code>cpu</code>, <code>metal</code>, <code>cuda</code>, <code>vulkan</code></td>
              </tr>
            </tbody>
          </table>
          <p>Only combinations listed by <code>--list-combinations</code> can be installed.</p>
          <h3 id="183-manage-models">18.3 Manage Models</h3>
          <p>List the bundled model catalog:</p>
          <pre className="code-block"><code className="language-sh">{`kronk bucky model catalog`}</code></pre>
          <p>The transcription models currently included in that catalog are:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Model</th>
                <th>Approximate size</th>
                <th>Language support</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>tiny</code></td>
                <td>75 MB</td>
                <td>Multilingual; fastest, lowest accuracy</td>
              </tr>
              <tr>
                <td><code>base</code></td>
                <td>142 MB</td>
                <td>Multilingual; fast</td>
              </tr>
              <tr>
                <td><code>base.en</code></td>
                <td>142 MB</td>
                <td>English only</td>
              </tr>
              <tr>
                <td><code>small</code></td>
                <td>466 MB</td>
                <td>Multilingual; balanced</td>
              </tr>
              <tr>
                <td><code>small.en</code></td>
                <td>466 MB</td>
                <td>English only</td>
              </tr>
              <tr>
                <td><code>medium</code></td>
                <td>1.5 GB</td>
                <td>Multilingual; accurate</td>
              </tr>
              <tr>
                <td><code>medium.en</code></td>
                <td>1.5 GB</td>
                <td>English only</td>
              </tr>
              <tr>
                <td><code>large-v3</code></td>
                <td>2.9 GB</td>
                <td>Multilingual; highest accuracy</td>
              </tr>
              <tr>
                <td><code>large-v3-turbo</code></td>
                <td>1.5 GB</td>
                <td>Multilingual; faster large-model variant</td>
              </tr>
            </tbody>
          </table>
          <p>The catalog also contains <code>silero-vad</code>, an auxiliary voice-activity detection model. It is not a transcription model and is not required by the SDK's built-in streaming silence detection.</p>
          <p>Pull, list, and remove models with:</p>
          <pre className="code-block"><code className="language-sh">{`kronk bucky model pull tiny
kronk bucky model list
kronk bucky model remove tiny`}</code></pre>
          <p>The pull command accepts a catalog name, a GGML filename such as <code>ggml-tiny.bin</code>, or a complete download URL. The catalog name <code>tiny</code> and the filename <code>ggml-tiny.bin</code> identify the same multilingual model. English-only models use the <code>.en</code> suffix, such as <code>base.en</code>.</p>
          <p>Add <code>--local</code> to any model command to operate directly on disk. Installed models are stored below <code>~/.kronk/bucky-models/</code> by default, using filenames such as <code>ggml-tiny.bin</code> and <code>ggml-base.en.bin</code>.</p>
          <h3 id="184-server-configuration">18.4 Server Configuration</h3>
          <p>Start the model server normally after installing the libraries and at least one model. Bucky uses the standard server address, whose default is:</p>
          <pre className="code-block"><code className="language-text">{`http://localhost:11435`}</code></pre>
          <p>Whisper models do not use Kronk's per-model YAML configuration. The server discovers installed <code>.bin</code> files and loads a model when it is first requested.</p>
          <p>Bucky uses the server's shared pool settings:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Setting</th>
                <th>Server default</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Models in pool</td>
                <td><code>10</code></td>
                <td>Maximum cached models</td>
              </tr>
              <tr>
                <td>Pool TTL</td>
                <td><code>20m</code></td>
                <td>How long an idle model remains loaded</td>
              </tr>
              <tr>
                <td>Memory budget</td>
                <td>Shared</td>
                <td>Bucky and language models compete for the same host memory budget</td>
              </tr>
            </tbody>
          </table>
          <p>If the server starts without usable Whisper libraries, it logs that Bucky is running in degraded mode. Library and model management remain available, but transcription cannot work until the libraries are installed and the server is restarted.</p>
          <h3 id="185-browser-ui">18.5 Browser UI</h3>
          <p>The BUI provides three Bucky-related areas:</p>
          <ol>
            <li><strong>Whisper Libraries</strong> installs and removes compatible library bundles.</li>
            <li><strong>Whisper Models</strong> browses the catalog and manages downloaded models.</li>
            <li><strong>Translator</strong> records or uploads audio and displays its transcript.</li>
          </ol>
          <p>In Translator:</p>
          <ol>
            <li>Select an installed model.</li>
            <li>Upload an audio file or record from the microphone.</li>
            <li>Leave the source language on <strong>Auto-detect</strong>, or select a language hint.</li>
            <li>Optionally enable translation to English or provide a decoder prompt.</li>
            <li>Select <strong>Transcribe</strong> or <strong>Translate</strong>.</li>
          </ol>
          <p>Translator requests <code>verbose_json</code> and displays the text and segment timing. It does not expose every field or response format supported by the HTTP API. Use the API directly when you need plain text, SRT, WebVTT, or explicit timestamp options.</p>
          <h3 id="186-transcriptions-api">18.6 Transcriptions API</h3>
          <h4 id="1861-request-and-response">18.6.1 Request and Response</h4>
          <p>Send a <code>multipart/form-data</code> request to:</p>
          <pre className="code-block"><code className="language-text">{`POST /v1/audio/transcriptions`}</code></pre>
          <p>The uploaded file is limited to <strong>25 MB</strong>. Each transcription has a 30-minute server deadline.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Field</th>
                <th>Required</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>file</code></td>
                <td>Yes</td>
                <td>Audio file to decode and transcribe</td>
              </tr>
              <tr>
                <td><code>model</code></td>
                <td>Yes</td>
                <td>Installed model ID, such as <code>tiny</code> or <code>base.en</code></td>
              </tr>
              <tr>
                <td><code>language</code></td>
                <td>No</td>
                <td>Whisper short language code such as <code>en</code>, <code>de</code>, or <code>fr</code>; empty means auto-detect</td>
              </tr>
              <tr>
                <td><code>prompt</code></td>
                <td>No</td>
                <td>Text that biases the initial decoder output</td>
              </tr>
              <tr>
                <td><code>translate</code></td>
                <td>No</td>
                <td><code>true</code> translates supported source speech to English</td>
              </tr>
              <tr>
                <td><code>response_format</code></td>
                <td>No</td>
                <td><code>json</code> (default), <code>verbose_json</code>, <code>text</code>, <code>srt</code>, or <code>vtt</code></td>
              </tr>
              <tr>
                <td><code>timestamp_granularities[]</code></td>
                <td>No</td>
                <td><code>word</code> is accepted; word data is not yet available and returns an empty <code>words</code> array in <code>verbose_json</code></td>
              </tr>
            </tbody>
          </table>
          <p>Example:</p>
          <pre className="code-block"><code className="language-sh">{`curl -X POST http://localhost:11435/v1/audio/transcriptions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -F file=@samples/jfk.wav \\
  -F model=tiny \\
  -F response_format=json`}</code></pre>
          <p>The default JSON response is:</p>
          <pre className="code-block"><code className="language-json">{`{"text":"And so my fellow Americans..."}`}</code></pre>
          <p><code>verbose_json</code> adds the detected language, duration, and timestamped segments. The <code>text</code>, <code>srt</code>, and <code>vtt</code> formats return their corresponding non-JSON media types.</p>
          <p>English-only models (<code>base.en</code>, <code>small.en</code>, and <code>medium.en</code>) only accept an empty language hint or <code>en</code>. Use a multilingual model for other languages or translation.</p>
          <h4 id="1862-bucky-management-endpoints">18.6.2 Bucky Management Endpoints</h4>
          <p>The CLI and BUI use these management routes:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Method and path</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>GET /v1/bucky/libs</code></td>
                <td>Show the active library installation</td>
              </tr>
              <tr>
                <td><code>GET /v1/bucky/libs/combinations</code></td>
                <td>List supported platform combinations</td>
              </tr>
              <tr>
                <td><code>GET /v1/bucky/libs/installs</code></td>
                <td>List installed library bundles</td>
              </tr>
              <tr>
                <td><code>POST /v1/bucky/libs/pull</code></td>
                <td>Install a library bundle</td>
              </tr>
              <tr>
                <td><code>DELETE /v1/bucky/libs/installs</code></td>
                <td>Remove a library bundle</td>
              </tr>
              <tr>
                <td><code>GET /v1/bucky/models</code></td>
                <td>List installed models</td>
              </tr>
              <tr>
                <td><code>GET /v1/bucky/models/catalog</code></td>
                <td>List the bundled catalog</td>
              </tr>
              <tr>
                <td><code>POST /v1/bucky/models/pull</code></td>
                <td>Download a model</td>
              </tr>
              <tr>
                <td><code>GET /v1/bucky/models/&#123;model&#125;/details</code></td>
                <td>Show model header and file details</td>
              </tr>
              <tr>
                <td><code>DELETE /v1/bucky/models/&#123;model&#125;</code></td>
                <td>Remove a model</td>
              </tr>
            </tbody>
          </table>
          <p>Mutating management routes require administrator authorization when authentication is enabled.</p>
          <h3 id="187-go-sdk">18.7 Go SDK</h3>
          <p>The Go SDK supports batch transcription, channel-separated diarization, and live streaming. See <a href="../examples/bucky/main.go"><code>examples/bucky/main.go</code></a> for a complete program that installs the current-host libraries, downloads a model, initializes Bucky, and handles errors.</p>
          <h4 id="1871-batch-transcription">18.7.1 Batch Transcription</h4>
          <p>After calling <code>bucky.Init</code> and constructing a <code>*bucky.Bucky</code> with the path to an installed model, transcribe an audio reader directly:</p>
          <pre className="code-block"><code className="language-go">{`f, err := os.Open("samples/jfk.wav")
if err != nil {
    return err
}
defer f.Close()

tr, err := b.TranscribeFile(ctx, f, model.WithLanguage("en"))
if err != nil {
    return err
}

fmt.Println(tr.Text)`}</code></pre>
          <p>Use <code>Transcribe</code> instead when the audio is already decoded to 16 kHz mono <code>[]float32</code>. Options include language, translation, initial prompt, beam size, thread count, and no-speech or log-probability thresholds. Consult the Go API documentation for the complete option list.</p>
          <h4 id="1872-channel-separated-diarization">18.7.2 Channel-Separated Diarization</h4>
          <p><code>TranscribeChannelsFile</code> treats each source channel as a separate speaker and merges their timestamped segments:</p>
          <pre className="code-block"><code className="language-go">{`d, err := b.TranscribeChannelsFile(ctx, f, model.WithLanguage("en"))
if err != nil {
    return err
}

for _, seg := range d.Segments {
    fmt.Printf("[%dms] speaker %d: %s\\n", seg.StartMs, seg.Channel, seg.Text)
}`}</code></pre>
          <p>This works best with native multichannel formats such as WAV or FLAC. Formats decoded through ffmpeg may be downmixed and therefore produce only one speaker channel.</p>
          <h4 id="1873-streaming-transcription">18.7.3 Streaming Transcription</h4>
          <p>Use <code>NewStream</code> when audio arrives over time. A stream emits tentative partials and committed finals:</p>
          <pre className="code-block"><code className="language-go">{`stream, err := b.NewStream(ctx, model.WithStreamLanguage("en"))
if err != nil {
    return err
}
defer stream.Close()

go func() {
    for ev := range stream.Events() {
        switch ev.Kind {
        case model.EventPartial:
            // Replace the currently displayed partial with ev.Text.
        case model.EventFinal:
            // Append ev.Text to the permanent transcript.
        case model.EventError:
            log.Println(ev.Err)
        }
    }
}()

for samples := range incomingAudio {
    if err := stream.Feed(ctx, samples); err != nil {
        return err
    }
}

return stream.Close()`}</code></pre>
          <p><code>EventPartial</code> contains the complete pending hypothesis, not a text delta, and may be dropped when the event consumer falls behind. <code>EventFinal</code> is the text to append and is not dropped.</p>
          <p><code>Feed</code> accepts normalized 16 kHz mono <code>[]float32</code>. For raw microphone data, <code>FeedPCM</code> accepts little-endian <code>int16</code> or <code>float32</code> PCM and performs downmixing and resampling:</p>
          <pre className="code-block"><code className="language-go">{`format := model.AudioFormat{
    SampleRate: 48000,
    Channels:   2,
    Sample:     model.Int16LE,
}

if err := stream.FeedPCM(ctx, rawPCM, format); err != nil {
    return err
}`}</code></pre>
          <p>Call <code>Feed</code> or <code>FeedPCM</code> from one producer goroutine. Both methods apply backpressure when their input queue is full. Real-time capture callbacks must not block; the <a href="../examples/bucky-stream/main.go"><code>examples/bucky-stream</code></a> program therefore uses an intermediate channel and deliberately drops capture buffers if its pump falls behind.</p>
          <p>Important stream defaults are:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Behavior</th>
                <th>Default</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Partial update</td>
                <td>Every 1,000 ms</td>
              </tr>
              <tr>
                <td>Forced final</td>
                <td>Every 6,000 ms without a pause</td>
              </tr>
              <tr>
                <td>Maximum utterance</td>
                <td>25,000 ms</td>
              </tr>
              <tr>
                <td>Silence detection</td>
                <td>Enabled</td>
              </tr>
              <tr>
                <td>Prompt carryover</td>
                <td>Enabled</td>
              </tr>
            </tbody>
          </table>
          <p>Options such as <code>WithPartialEveryMs</code>, <code>WithCommitEveryMs</code>, <code>WithVAD</code>, and <code>WithPromptCarryover</code> change these behaviors. A negative partial interval disables partial events.</p>
          <p><code>Reset</code> starts a new logical session while keeping the stream open. By default it flushes pending audio and restarts timestamps at zero. After an <code>EventError</code>, close the failed stream and open a new one instead of resetting it.</p>
          <p>Always close a stream. An open stream reserves SDK inference capacity and can prevent model unloading. SDK users that need concurrent streams can configure <code>model.WithNSeqMax</code> when creating the Bucky handle; this is an SDK setting, not a server configuration field.</p>
          <h3 id="188-languages">18.8 Languages</h3>
          <p>Whisper supports approximately 99 languages. Use its short language codes, such as <code>en</code>, <code>de</code>, <code>fr</code>, <code>es</code>, or <code>zh</code>. An empty language value asks Whisper to auto-detect the language.</p>
          <p>The SDK exposes <code>bucky.LangID</code>, <code>bucky.LangStr</code>, and <code>bucky.LangMaxID</code> for enumerating and converting the codes known to the loaded whisper.cpp library.</p>
          <p>Use a multilingual model (<code>tiny</code>, <code>base</code>, <code>small</code>, <code>medium</code>, <code>large-v3</code>, or <code>large-v3-turbo</code>) for non-English speech. Models ending in <code>.en</code> are English only. Whisper translation converts supported source speech to English; it does not translate into arbitrary target languages.</p>
          <h3 id="189-troubleshooting">18.9 Troubleshooting</h3>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Symptom</th>
                <th>Action</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Server logs <code>bucky init failed, running in degraded mode</code></td>
                <td>Install a library bundle compatible with the host, then restart the server.</td>
              </tr>
              <tr>
                <td>Transcription reports an unknown model</td>
                <td>Run <code>kronk bucky model list</code>; pull the required model if it is absent.</td>
              </tr>
              <tr>
                <td>An English-only model rejects the language</td>
                <td>Use <code>en</code>, omit the hint, or switch to a multilingual model.</td>
              </tr>
              <tr>
                <td>The upload is rejected for its size</td>
                <td>Keep the audio file at or below 25 MB. Split long recordings or re-encode them at a lower bitrate.</td>
              </tr>
              <tr>
                <td>Audio decodes to no samples</td>
                <td>The file may be corrupt or unsupported. Re-encode it as a 16 kHz mono WAV and retry.</td>
              </tr>
              <tr>
                <td>GPU inference is unexpectedly slow</td>
                <td>Check <code>KRONK_BUCKY_LIB_PATH</code> and the active bundle. A CPU bundle runs without GPU acceleration.</td>
              </tr>
              <tr>
                <td>A new SDK stream blocks or times out</td>
                <td>Another stream is holding all configured SDK stream capacity. Close idle streams or create the handle with a larger <code>NSeqMax</code>.</td>
              </tr>
              <tr>
                <td>Streaming emits finals but no partials</td>
                <td>Check that <code>WithPartialEveryMs</code> was not given a negative value and that audio is arriving continuously.</td>
              </tr>
              <tr>
                <td>Streaming repeats words at boundaries</td>
                <td>Keep silence detection enabled and avoid overly short forced-final intervals.</td>
              </tr>
              <tr>
                <td>Whisper diagnostic output appears in the terminal</td>
                <td>Do not select <code>bucky.LogNormal</code>; the default <code>LogSilent</code> suppresses whisper.cpp diagnostics.</td>
              </tr>
            </tbody>
          </table>
          <hr />
          <p><em>Next: &lt;a href="chapter-19-developer-guide.md"&gt;Chapter 19: Developer Guide&lt;/a&gt;</em></p>
          <h2 id="chapter-19-developer-guide">Chapter 19: Developer Guide</h2>
          <h3 id="191-how-to-use-this-guide">19.1 How to Use This Guide</h3>
          <p>This chapter is a durable orientation guide for contributors and coding agents. It describes ownership boundaries, lifecycle contracts, and the smallest useful checks for common changes. It intentionally does not narrate every function, reproduce private structures, or freeze today's source layout at the individual-file level.</p>
          <h4 id="1911-source-of-truth-hierarchy">19.1.1 Source-of-truth hierarchy</h4>
          <p>When sources disagree, use this order:</p>
          <ol>
            <li><strong>Applicable &lt;code&gt;AGENTS.md&lt;/code&gt; files.</strong> Read the root instructions and every scoped instruction file governing the path being changed. A deeper file overrides or supplements broader guidance for its subtree.</li>
            <li><strong>Current source and tests.</strong> Interfaces, call sites, focused tests, and generated code establish actual behavior. For generated documentation, the authored input and generator are authoritative over checked-in output from an older generation. Confirm assumptions in code before editing.</li>
            <li><strong>Makefiles and GitHub workflows.</strong> These establish supported build, generation, CI, packaging, and deployment procedures.</li>
            <li><strong>This chapter.</strong> It provides background and navigation, not a replacement for scoped instructions or source inspection.</li>
          </ol>
          <p>Treat names and defaults in this chapter as wayfinding aids. Before relying on an exact flag, timeout, model capability, or API signature, inspect its current owner. Do not expand a task merely to make the repository resemble this overview.</p>
          <h4 id="1912-a-productive-agent-loop">19.1.2 A productive agent loop</h4>
          <p>For most work, the safest loop is:</p>
          <ol>
            <li>Identify the public behavior being changed and its owning package.</li>
            <li>Read that package's scoped instructions, implementation, focused tests, and direct caller. Avoid broad repository scans when a precise search will answer the question.</li>
            <li>Write down the invariants that must remain true: resource ownership, cancellation, mutation semantics, compatibility, generated artifacts, and error translation.</li>
            <li>Make the smallest coherent change in the owner. Do not duplicate policy in a transport or facade when the lower layer already owns it.</li>
            <li>Format and run package-scoped static checks, then focused tests. Regenerate only artifacts derived from changed sources.</li>
            <li>Review the diff for accidental generated-file edits, private data in logs, unrelated formatting, and stale documentation.</li>
          </ol>
          <h3 id="192-task-to-owner-and-verification-map">19.2 Task-to-Owner and Verification Map</h3>
          <p>The following table is a starting point, not permission to skip local instructions. “Focused verification” means the narrowest package or command that exercises the change. Go commands require the environment described in <a href="#199-verification-for-llm-agents">§19.9</a>.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Task</th>
                <th>Primary owner</th>
                <th>Read adjacent</th>
                <th>Focused verification</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>CLI command, flag, or output</td>
                <td><code>cmd/kronk/</code> (command wiring is under its command tree)</td>
                <td>corresponding <code>sdk/tools/</code> manager and server tool route when local/web modes share behavior</td>
                <td><code>go test</code> for changed command package; <code>go install ./cmd/kronk</code>; invoke the one command with harmless arguments</td>
              </tr>
              <tr>
                <td>HTTP endpoint or response shape</td>
                <td><code>cmd/server/app/domain/&lt;domain&gt;app/</code></td>
                <td>that domain's <code>route.go</code>, <code>cmd/server/foundation/web/</code>, public SDK method, and service composition</td>
                <td>domain package tests; relevant service API test; build <code>./cmd/server/...</code></td>
              </tr>
              <tr>
                <td>Server startup or dependency wiring</td>
                <td><code>cmd/server/api/services/</code></td>
                <td><code>cmd/server/app/</code>, config tests, embedded assets</td>
                <td>service package tests and <code>go build ./cmd/server/...</code></td>
              </tr>
              <tr>
                <td>Middleware, request context, errors, or tracing</td>
                <td><code>cmd/server/foundation/web/</code></td>
                <td>all route registration using the middleware</td>
                <td>foundation tests plus one affected domain test</td>
              </tr>
              <tr>
                <td>Public language-model SDK behavior</td>
                <td><code>sdk/kronk/</code></td>
                <td><code>sdk/kronk/model/</code>, examples, generated SDK docs</td>
                <td>focused <code>sdk/kronk</code> tests and compile an affected example when useful</td>
              </tr>
              <tr>
                <td>Batch scheduling, KV state, IMC, media, sampling, or speculative inference</td>
                <td><code>sdk/kronk/model/</code></td>
                <td>yzma boundary, <code>sdk/kronk/tests/</code> suite definitions, observability</td>
                <td>focused unit tests in <code>sdk/kronk/model</code>; model-backed integration suites are human/CI work</td>
              </tr>
              <tr>
                <td>Tool/reasoning parser</td>
                <td><code>sdk/kronk/parsers/&lt;family&gt;/</code> and registry contract in <code>sdk/kronk/model/</code></td>
                <td>parser registration and model-family selection</td>
                <td>parser-family tests and registry tests</td>
              </tr>
              <tr>
                <td>Multi-model loading or eviction</td>
                <td>typed owners <code>sdk/kronk/pool/</code>, <code>sdk/bucky/pool/</code>; shared mechanics <code>sdk/pool/engine/</code>; app facade <code>sdk/pool/</code></td>
                <td>resource-manager APIs and server composition</td>
                <td>engine eviction tests, typed-pool tests, failed-load and budget tests</td>
              </tr>
              <tr>
                <td>Resource accounting</td>
                <td>shared resource manager used through <code>sdk/pool/engine/</code></td>
                <td>each typed loader's plan/display/unload methods</td>
                <td>synthetic budget/reservation tests; never rely on host memory alone</td>
              </tr>
              <tr>
                <td>Bucky handle, transcription, or stream</td>
                <td><code>sdk/bucky/</code> and <code>sdk/bucky/model/</code></td>
                <td><code>sdk/bucky/pool/</code>, audio route, Chapter 18 for user behavior</td>
                <td>package unit tests and focused <code>sdk/bucky/tests/transcribe</code> tests where dependencies are available</td>
              </tr>
              <tr>
                <td>Bucky libraries/models tooling</td>
                <td><code>sdk/tools/bucky/</code></td>
                <td><code>cmd/kronk/bucky/</code>, server <code>toolapp</code> routes</td>
                <td>tooling package tests and a harmless <code>--local</code> listing command</td>
              </tr>
              <tr>
                <td>General library/model/catalog/device tooling</td>
                <td><code>sdk/tools/</code></td>
                <td>local CLI and <code>toolapp</code> web wrappers</td>
                <td>changed package tests; compare local and default web behavior where both exist</td>
              </tr>
              <tr>
                <td>BUI page or API client</td>
                <td><code>cmd/server/api/frontends/bui/</code></td>
                <td>scoped component instructions and matching HTTP route</td>
                <td><code>npm run build</code> from the BUI directory; server embedding check</td>
              </tr>
              <tr>
                <td>Manual source</td>
                <td><code>.manual/</code></td>
                <td>docs manual generator and generated <code>DocsManual</code></td>
                <td><code>make kronk-docs</code>, then BUI build</td>
              </tr>
              <tr>
                <td>SDK/example docs generation</td>
                <td><code>cmd/server/api/tooling/docs/</code> plus source Go docs and <code>examples/</code></td>
                <td>generated BUI docs components</td>
                <td><code>make kronk-docs</code>; inspect generated diff; BUI build</td>
              </tr>
              <tr>
                <td>Linux CI</td>
                <td><code>.github/workflows/linux.yml</code>, <code>.github/actions/setup-kronk/</code>, <code>.github/test-models.txt</code></td>
                <td>make targets and version-check scripts</td>
                <td>syntax/action review; run changed scripts locally; package checks represented by changed job</td>
              </tr>
              <tr>
                <td>Release</td>
                <td><code>.github/workflows/release.yaml</code>, <code>.goreleaser.yaml</code>, <code>.release/</code>, version scripts</td>
                <td><code>sdk/kronk</code> version constant and tag convention</td>
                <td>version scripts and GoReleaser snapshot/check when appropriate</td>
              </tr>
              <tr>
                <td>Container image</td>
                <td><code>.github/workflows/docker.yml</code>, <code>zarf/docker/</code></td>
                <td>entrypoint, native-library combinations, release tags</td>
                <td>build the affected target/variant; workflow is authority for matrix and signing</td>
              </tr>
              <tr>
                <td>Nix development/package data</td>
                <td><code>zarf/nix/flake.nix</code></td>
                <td>Go module files and setup hook</td>
                <td>evaluate/build the relevant Nix entry point; regenerate gomod2nix data when dependencies change</td>
              </tr>
            </tbody>
          </table>
          <p>Tests close to an owner are usually more diagnostic than a repository-wide command. If a change crosses rows, verify each changed contract rather than choosing only the largest command.</p>
          <h3 id="193-repository-ownership-map">19.3 Repository Ownership Map</h3>
          <h4 id="1931-commands-and-server">19.3.1 Commands and server</h4>
          <ul>
            <li><strong>&lt;code&gt;cmd/kronk/&lt;/code&gt;</strong> owns the installed <code>kronk</code> executable, command hierarchy, flags, local-versus-server dispatch, terminal presentation, and process control. Commands should orchestrate reusable managers rather than become a second implementation of catalogs, downloads, inference, or authentication. <code>make install-kronk</code> is simply <code>go install ./cmd/kronk</code>; there are no project build tags required for installation.</li>
            <li><strong>&lt;code&gt;cmd/server/api/&lt;/code&gt;</strong> composes executable services, startup configuration, embedded assets, and tooling binaries. <code>cmd/server/api/services/kronk/main.go</code> is the Kronk service composition root and the BUI embedding owner.</li>
            <li><strong>&lt;code&gt;cmd/server/app/domain/&lt;/code&gt;</strong> owns HTTP domain behavior. Packages such as <code>chatapp</code>, <code>respapp</code>, <code>embedapp</code>, <code>rerankapp</code>, <code>audioapp</code>, <code>toolapp</code>, and <code>playgroundapp</code> register routes and translate HTTP requests to application/SDK calls. Keep protocol validation and response formatting near the domain; keep inference policy in SDKs.</li>
            <li><strong>&lt;code&gt;cmd/server/app/&lt;/code&gt;</strong> also contains application wiring and adapters shared by domains. It is below service startup and above reusable SDK packages.</li>
            <li><strong>&lt;code&gt;cmd/server/foundation/&lt;/code&gt;</strong> owns cross-cutting server infrastructure. <code>web/</code> owns request lifecycle, context, middleware, error/response writing, and transport-level tracing; <code>logger/</code> owns server logging primitives. Domain packages should use these facilities rather than invent parallel conventions.</li>
          </ul>
          <h4 id="1932-language-model-sdk-and-engine">19.3.2 Language-model SDK and engine</h4>
          <ul>
            <li><strong>&lt;code&gt;sdk/kronk/&lt;/code&gt;</strong> is the public language-model handle and API surface. A <code>kronk.Kronk</code> owns <strong>one primary loaded model</strong> and a semaphore governing admission to that handle. Its low-level model may also own draft or MTP resources. The handle exposes chat, streaming, Responses, embeddings, reranking, tokenization, model information, and unload behavior. It is not a model pool.</li>
            <li><strong>&lt;code&gt;sdk/kronk/model/&lt;/code&gt;</strong> owns low-level llama/yzma inference: model/context creation, prompt planning, batch slots, sequence IDs, prefill/decode, media handling, IMC, samplers, parser interfaces, and draft/MTP behavior. Changes here must preserve cross-slot isolation and native-resource cleanup on every exit path.</li>
            <li><strong>&lt;code&gt;sdk/kronk/parsers/&lt;/code&gt;</strong> contains model-family parser plug-ins. Family packages own streaming state machines and extraction/normalization of reasoning and tool calls. The model package owns the registry contract and selection boundary; parser packages should not reach into batch-engine state.</li>
            <li><strong>&lt;code&gt;sdk/kronk/observ/&lt;/code&gt;</strong>, <code>sdk/kronk/kvstorage/</code>, <code>sdk/kronk/vram/</code>, <code>sdk/kronk/gguf/</code>, and <code>sdk/kronk/hf/</code> own their named concerns. Prefer these boundaries to embedding format, storage, or resource calculations in request handlers.</li>
          </ul>
          <h4 id="1933-pools-and-shared-resources">19.3.3 Pools and shared resources</h4>
          <ul>
            <li><strong>&lt;code&gt;sdk/pool/&lt;/code&gt;</strong> is the server-facing application facade over the language and audio typed pools and their shared resource manager. It coordinates backends; it does not replace either SDK handle.</li>
            <li><strong>&lt;code&gt;sdk/kronk/pool/&lt;/code&gt;</strong> adapts Kronk model discovery, planning, loading, display, and unloading to the generic pool engine.</li>
            <li><strong>&lt;code&gt;sdk/bucky/pool/&lt;/code&gt;</strong> performs the equivalent work for Whisper/Bucky models.</li>
            <li><strong>&lt;code&gt;sdk/pool/engine/&lt;/code&gt;</strong> owns typed cache mechanics: acquisition coalescing, admission, idle-entry selection, expiry, invalidation, and eviction callbacks. The shared resource manager owns RAM/VRAM reservations and budget decisions across backends. Eviction is therefore budget-aware and constrained by active use and releasable reservations; it is not a simple model-ID LRU.</li>
          </ul>
          <p>The service supplies shared-pool settings to both typed pools. A typed pool used independently may therefore have different fallback values. Check the composition layer before documenting effective server behavior, and check the typed constructor before documenting standalone behavior.</p>
          <h4 id="1934-bucky-tools-ui-examples-and-deployment">19.3.4 Bucky, tools, UI, examples, and deployment</h4>
          <ul>
            <li><strong>&lt;code&gt;sdk/bucky/&lt;/code&gt;</strong> is the public concurrent audio handle. <strong>&lt;code&gt;sdk/bucky/model/&lt;/code&gt;</strong> owns whisper context/state operations, decoding, transcription, and stream mechanics.</li>
            <li><strong>&lt;code&gt;sdk/tools/&lt;/code&gt;</strong> owns reusable catalog, downloader, backend, device, diagnostics, library, and model-management operations used by CLI and server tools. Bucky-specific installers and catalogs live below <code>sdk/tools/bucky/</code>.</li>
            <li><strong>&lt;code&gt;cmd/server/api/frontends/bui/&lt;/code&gt;</strong> is the React/TypeScript browser application. Component-level conventions are deliberately delegated to the applicable <code>AGENTS.md</code>; this guide does not duplicate them.</li>
            <li><strong>&lt;code&gt;examples/&lt;/code&gt;</strong> is a separate Go module of runnable public-SDK examples and a source for generated documentation. Keep examples public and instructional: do not import internal server implementation to make them convenient.</li>
            <li><strong>&lt;code&gt;.manual/&lt;/code&gt;</strong> contains authored manual chapters. The docs tool converts manuals, public SDK documentation, and examples into BUI components.</li>
            <li><strong>&lt;code&gt;zarf/&lt;/code&gt;</strong>, <code>.github/workflows/</code>, <code>.goreleaser.yaml</code>, and <code>.release/</code> own deployment, reproducibility, and release automation. Runtime image behavior must agree with the Docker workflow and entrypoint, not with an old prose inventory.</li>
          </ul>
          <h3 id="194-developer-setup-and-daily-commands">19.4 Developer Setup and Daily Commands</h3>
          <p>From the repository root, install the CLI with:</p>
          <pre className="code-block"><code className="language-shell">{`go install ./cmd/kronk`}</code></pre>
          <p>The Make target is a convenient alias:</p>
          <pre className="code-block"><code className="language-shell">{`make install-kronk`}</code></pre>
          <p>Configure repository hooks and optional development tooling with:</p>
          <pre className="code-block"><code className="language-shell">{`make setup
make install-gotooling
make install-tooling`}</code></pre>
          <p><code>make setup</code> configures the repository's hook workflow. Tool installation is separate; inspect the Make targets before running them on a platform where package-manager changes are undesirable. Common service commands include:</p>
          <pre className="code-block"><code className="language-shell">{`make kronk-server
make kronk-server-detach
make kronk-server-logs
make kronk-server-stop`}</code></pre>
          <p>Native llama and Whisper libraries and test models are large external prerequisites. Use the CLI and Make targets appropriate to the focused test rather than downloading every supported artifact. The Bucky CLI uses <code>--local</code> for direct filesystem work; web/server operation is the default and there is no <code>--web</code> flag.</p>
          <p>The exact development toolchain is pinned by <code>.go-version</code>, while <code>go.mod</code> declares the minimum language version. Patch versions may differ, but major and minor must match. The workflow version script enforces this relationship; read both files rather than copying their current values into new documentation.</p>
          <h3 id="195-request-and-model-lifecycle">19.5 Request and Model Lifecycle</h3>
          <h4 id="1951-server-request-flow">19.5.1 Server request flow</h4>
          <p>The stable request path is:</p>
          <pre className="code-block"><code className="language-text">{`HTTP request
  -> foundation/web middleware and request context
  -> domain route validation and protocol translation
  -> sdk/pool facade
  -> typed pool acquisition (Kronk or Bucky)
  -> one-model public handle admission semaphore
  -> model engine/state execution
  -> SDK result or stream
  -> protocol response`}</code></pre>
          <p>Every arrow is an ownership boundary. Middleware owns transport concerns. Domains own HTTP compatibility. Pools own model residency and resource tickets. Handles own per-model admission and shutdown coordination. Engines own native contexts, sequences, and inference state. Preserve error identities long enough for the domain layer to map capacity, cancellation, validation, and internal failures correctly.</p>
          <h4 id="1952-typed-pool-acquisition-loading-and-eviction">19.5.2 Typed pool acquisition, loading, and eviction</h4>
          <p>An acquisition first checks/coalesces a typed cache entry. A cold load is planned by the backend loader, then reserved against the shared memory manager before expensive native loading. The loaded handle becomes visible only after initialization succeeds. Failed planning or loading must release its reservation and must not publish a partial cache entry. Concurrent acquisition of the same key should share load work rather than multiply memory commitments.</p>
          <p>Item count and available RAM/VRAM are independent admission constraints. Normal admission-driven eviction selects only idle handles; the pool observes each handle's active-operation counter rather than owning a separate request lease. Explicit invalidation and shutdown may remove an active entry and rely on the handle's <code>Unload</code> method to drain active work according to its context.</p>
          <p>Asynchronous invalidation does not imply that its eviction callback has finished. Synchronous invalidation waits for callback and reservation-ticket completion, but it does not prove native unload succeeded: the callback can report an unload error and still release the reservation. Preserve the engine-level distinction between a model that cannot fit the configured budget and temporary pressure where no idle candidate can be evicted; typed/public APIs may translate those errors differently.</p>
          <h4 id="1953-semaphore-lifetime-and-cancellation">19.5.3 Semaphore lifetime and cancellation</h4>
          <p>The <code>Kronk</code> handle's semaphore is admission control around one model. A permit belongs to the operation, not merely to function setup. For a non-streaming call, hold it until the operation returns. For streaming, hold it until the stream is terminal or closed, including cancellation/error cleanup. Releasing at stream construction over-admits; forgetting to release on an early error deadlocks future work. The pool's active-use lease similarly spans the entire externally visible operation so eviction cannot unload native resources while output is still being consumed.</p>
          <p>Context cancellation must propagate inward to queue waits and inference. The layer that creates a goroutine, stream, native object, or lease owns its shutdown. Do not close caller-owned channels or unload a caller-owned handle. Unload prevents new work, waits for owned active work according to its context, then tears down engine/native resources. Pool shutdown owns handles created by that pool.</p>
          <h4 id="1954-batch-slots-and-sequence-isolation">19.5.4 Batch slots and sequence isolation</h4>
          <p>Text generation uses a batch engine. A slot is an execution reservation and mutable per-request state; its sequence ID partitions KV/cache operations in the shared native context. Scheduling several slots into one decode is safe only if every token, logit index, sampler, parser, cancellation flag, speculative buffer, media position, and KV operation remains associated with the correct slot/sequence.</p>
          <p>The main invariants are:</p>
          <ul>
            <li>A slot has one active job and one sequence identity at a time.</li>
            <li>Batch construction must preserve token-to-sequence and output-index mappings.</li>
            <li>Completion/cancellation removes or resets only the finishing sequence's state.</li>
            <li>Slot reuse starts from an intentionally clean state; no parser, sampler, media, speculative, or error state may leak to the next job.</li>
            <li>A blocked or cancelled caller must not strand a slot or semaphore permit.</li>
            <li>Native decode failure is attributed to affected jobs and followed by deterministic cleanup; it must not silently publish partly advanced session state.</li>
          </ul>
          <h4 id="1955-text-versus-embedding-and-reranking-contexts">19.5.5 Text versus embedding and reranking contexts</h4>
          <p>Text generation benefits from shared batched execution and sequence-partitioned KV. Embeddings and reranking use a different context strategy: they acquire a context for the operation and perform their own decode/clear cycle. Reranking evaluates query and documents without allowing one document's KV state to contaminate the next. Embedding pooling and normalization are model/output concerns, not chat-slot concerns. Do not force these paths through text batching merely to share code; share only primitives whose lifecycle contracts match.</p>
          <h3 id="196-core-inference-invariants">19.6 Core Inference Invariants</h3>
          <h4 id="1961-imc-sessions-slots-and-external-storage">19.6.1 IMC sessions, slots, and external storage</h4>
          <p>Incremental Message Cache (IMC) sessions are cache identities, not execution slots. A stable cache/session identifier allows a conversation prefix to survive movement between batch slots. A slot is short-lived compute capacity; binding a session to a slot would reduce concurrency and make slot reuse unsafe.</p>
          <p>The <code>SessionStore</code> contract externalizes each session's native KV snapshot. RAM and disk implementations differ in storage and I/O, but the model layer owns when a snapshot is read, prepared, committed, reset, and closed. Session metadata—cached tokens, render-sensitive identity/version, and snapshot—must describe the same prefix. Do not update one independently and call the session valid.</p>
          <p>The session's reservation and <code>pending</code> state serialize mutation and hide the session from competing selection until metadata and snapshot bytes agree. Restore only a committed snapshot whose token/prompt identity still matches. Ordinary text build/extension prepares and commits through the session's existing store; if snapshot publication fails, invalidate that session so later work rebuilds it rather than claiming the old or partial state is valid.</p>
          <p>Media-anchor advancement has a stronger replacement contract: it writes a separately staged store and swaps the store plus matching plan/count metadata only after success, so failure leaves the previous media snapshot published. Do not generalize that staged replacement guarantee to every IMC path. A <code>SessionStore</code> implementation must honor the interface's read/prepare/commit/reset lifetime rules and clean up temporary resources; callers must not assume bytes remain stable across the next mutation.</p>
          <h4 id="1962-prompt-plans-text-and-media">19.6.2 Prompt plans: text and media</h4>
          <p>Prompt planning converts normalized messages and parameters into the exact work the engine will execute. The plan, cache identity, token accounting, and decode positions must agree. Text-only plans can compare rendered/tokenized prefixes directly and may take optimized exact-hit paths. Media plans carry more than text tokens: ordered media parts, placeholder/embedding expansion, positions, and render-affecting metadata are part of identity and execution.</p>
          <p>Do not treat a media prompt as text with an attachment ignored by caching. A text prefix match is insufficient if image/audio/video content, ordering, sizing, or model projection changes. Media prefill must align embeddings and positions with the same sequence that receives surrounding text. When prompt construction or media decode fails, the prior valid IMC snapshot remains authoritative.</p>
          <h4 id="1963-parser-registry-ownership">19.6.3 Parser registry ownership</h4>
          <p>Parser implementations live under <code>sdk/kronk/parsers/</code>, grouped by model family. The registry interface and registration entry point live in <code>sdk/kronk/model/</code>. A parser plug-in supplies factories/state machines for its advertised family and must tolerate stream chunk boundaries: tags, JSON, reasoning delimiters, and tool arguments may span chunks. It must keep request state per parser instance and produce equivalent logical results for streaming and non-streaming input.</p>
          <p>To add or change a parser, edit the family package, update registration/selection only where necessary, and test fragmented as well as complete input. Keep generic JSON repair separate from family recognition. Unknown families need an intentional fallback or error; registration order must not create accidental model-family selection.</p>
          <h4 id="1964-responses-normalization">19.6.4 Responses normalization</h4>
          <p>The Responses API adapts to the chat/inference pipeline in <code>sdk/kronk/response.go</code>. Normalization has a compatibility-sensitive mutation contract:</p>
          <ul>
            <li>Preserve existing <code>messages</code>; they win when already supplied.</li>
            <li>Convert Responses <code>input</code> into messages <strong>only when messages are absent</strong>.</li>
            <li>Normalize Responses item/content/tool forms needed by chat processing.</li>
            <li>Mutate the supplied <code>model.D</code> document map. Callers that require isolation must clone before invoking the Responses path.</li>
          </ul>
          <p>Do not “clean up” this code by always rebuilding messages or silently switching to a copy. Either change breaks callers that combine compatibility fields or inspect the document after normalization. Add tests for existing messages, input-only requests, and observable in-place mutation.</p>
          <h4 id="1965-tracing-and-logging">19.6.5 Tracing and logging</h4>
          <p>Tracing should identify major waits and ownership boundaries: request handling, model acquisition/load, queue wait, prompt/prefill, generation, and unload when relevant. Keep spans concise. Avoid a span per token, duplicated nested timing, giant model-config attribute sets, prompt/media payloads, and unbounded IDs. Propagate the request context instead of creating unrelated roots. Logs and metrics should help distinguish queue, capacity, cancellation, and inference failures without exposing user content unless an explicit insecure-logging mode authorizes it.</p>
          <h4 id="1966-speculative-decoding-and-mtp">19.6.6 Speculative decoding and MTP</h4>
          <p>Speculative support has three ownership shapes:</p>
          <ol>
            <li><strong>Separate GGUF draft model.</strong> The draft has its own model/context/KV and proposes tokens; the target verifies them. Loading, memory planning, sequence cleanup, and rollback must account for both models.</li>
            <li><strong>Embedded MTP.</strong> A target GGUF exposes an embedded multi-token-prediction head. Model detection and MTP construction are owned by <code>draft_mtp.go</code>/<code>batch_mtp.go</code>, while generic proposal verification and reconciliation remain in <code>batch_speculative.go</code>.</li>
            <li><strong>Separate-file Gemma4/shared-target-KV MTP.</strong> The MTP component is supplied as a separate file but shares target KV semantics rather than behaving like an ordinary independent draft model. Capabilities, not “has a draft path,” must decide whether draft KV can be trimmed or externalized.</li>
          </ol>
          <p>Across all three, target output is authoritative. Proposal generation cannot expose a token until target verification accepts it or chooses the replacement/bonus token. Position counters, sampled-token history, target KV, draft/MTP state, and streamed output must describe one accepted prefix after every round.</p>
          <p>Verification in a multi-slot batch is explicitly read-before-mutate. First read all target logits/hidden-state rows and decide each slot's accepted prefix while the shared batch outputs are intact. Only then mutate KV, counters, slot buffers, stream output, or MTP mirror state. Mutating one slot during the read phase can invalidate indices or native output needed by another slot.</p>
          <p>Ordinary transformer KV can often remove a rejected suffix. Hybrid recurrent/state- space models cannot assume partial KV deletion restores prior state. Take the required pre-speculation per-sequence snapshot, and on rejection restore it and re-decode exactly the accepted prefix. Preserve captured target hidden-state rows needed to synchronize MTP. For own-KV MTP, rollback removes speculative draft state before mirroring accepted target state. For shared-target-KV Gemma4, do not apply independent-draft rollback to the shared target cache. If synchronization fails, safely disable MTP for that request and continue target-only rather than retaining ambiguous draft state.</p>
          <p>Unit-level owners are the batch/speculative files and tests in <code>sdk/kronk/model/</code>. Model-backed MTP suites live in <code>sdk/kronk/tests/mtp</code> and <code>sdk/kronk/tests/gemma4mtp</code>; they are CI/human suites, not commands agents should launch from the forbidden integration-test tree.</p>
          <h3 id="197-server-bui-and-generated-documentation">19.7 Server, BUI, and Generated Documentation</h3>
          <h4 id="1971-routes-middleware-and-domains">19.7.1 Routes, middleware, and domains</h4>
          <p>Route declarations belong with their domain package, normally in <code>route.go</code>. Keep authentication/authorization, tracing, request IDs, panic recovery, and common response behavior in foundation middleware. Domain handlers decode and validate protocol input, select the appropriate application capability, call SDK/facade methods, and encode the protocol result. They should not manipulate native model state or implement pool eviction.</p>
          <p>When adding an endpoint, follow a neighboring domain end to end: registration, middleware order, request model, error mapping, streaming behavior, and service wiring. Test malformed input and cancellation as well as success. A server build catches route composition errors that a leaf-package test may miss.</p>
          <h4 id="1972-bui-ownership-and-embedding">19.7.2 BUI ownership and embedding</h4>
          <p>The BUI lives at <code>cmd/server/api/frontends/bui/</code>. Follow its own package scripts and the applicable component <code>AGENTS.md</code>; component structure and UI conventions change more quickly than this chapter. The production bundle is embedded by <code>cmd/server/api/services/kronk/main.go</code>. Editing TypeScript does not alter the server binary until the frontend is rebuilt and embedded output is rebuilt into Go.</p>
          <p>For frontend changes:</p>
          <pre className="code-block"><code className="language-shell">{`cd cmd/server/api/frontends/bui
npm run build`}</code></pre>
          <p>Then build the server (or the narrow service package) and verify that the expected static bundle is present in the embedding location. Avoid hand-editing minified/static output.</p>
          <h4 id="1973-documentation-generation">19.7.3 Documentation generation</h4>
          <p><code>cmd/server/api/tooling/docs/main.go</code> orchestrates three conceptual pipelines:</p>
          <pre className="code-block"><code className="language-text">{`public SDK Go documentation -> SDK BUI documentation
examples source             -> example BUI documentation
.manual chapter Markdown    -> DocsManual.tsx`}</code></pre>
          <p>Author manual content in <code>.manual/</code>, public API descriptions in Go doc comments, and examples in <code>examples/</code>. <code>DocsManual.tsx</code> and generated SDK/example documentation are outputs and must not be hand-edited. Run:</p>
          <pre className="code-block"><code className="language-shell">{`make kronk-docs`}</code></pre>
          <p>Review generated diffs for malformed Markdown conversion and then run <code>npm run build</code> in the BUI. Finally build the server to check that generated components compile into the embedded bundle. Generation may update more than one documented package; do not discard legitimate generated changes. If the requested scope intentionally excludes generated artifacts, report that regeneration remains pending.</p>
          <h3 id="198-bucky-implementation-map">19.8 Bucky Implementation Map</h3>
          <p>This is an implementation map only. Chapter 18 owns installation, configuration, streaming usage, and API examples.</p>
          <h4 id="1981-owners">19.8.1 Owners</h4>
          <ul>
            <li><strong>&lt;code&gt;sdk/bucky/&lt;/code&gt;</strong> owns initialization and the public <code>Bucky</code> handle. A handle owns one Whisper model and admission/shutdown coordination.</li>
            <li><strong>&lt;code&gt;sdk/bucky/model/&lt;/code&gt;</strong> owns the Whisper context, its pool of model states, audio decode/transcription primitives, language operations, and stream implementation. Model weights/context are shared by the handle while state isolates concurrent work.</li>
            <li><strong>&lt;code&gt;sdk/bucky/pool/&lt;/code&gt;</strong> adapts Bucky model planning, loading, status, unloading, and reservations to the generic typed pool.</li>
            <li><strong>&lt;code&gt;sdk/pool/&lt;/code&gt; and &lt;code&gt;sdk/pool/engine/&lt;/code&gt;</strong> let Bucky and Kronk share one resource budget while retaining backend-specific loaders and handles.</li>
            <li><strong>&lt;code&gt;sdk/tools/bucky/&lt;/code&gt;</strong> owns Whisper shared-library and model catalog/download work.</li>
            <li><strong>&lt;code&gt;cmd/kronk/bucky/&lt;/code&gt;</strong> exposes those tools. Web/server mode is default; <code>--local</code> requests direct local operation.</li>
            <li><strong>&lt;code&gt;cmd/server/app/domain/audioapp/&lt;/code&gt;</strong> owns the OpenAI-compatible transcription route. Administrative library/model routes are in <code>toolapp</code>. Service startup wires the Bucky backend and shared pool.</li>
          </ul>
          <h4 id="1982-lifecycle-invariants">19.8.2 Lifecycle invariants</h4>
          <p><code>Init</code> registers/resolves/loads the backend. Technically, a failed <code>Init</code> can be called again and retry. The current server calls it only during startup, however. Installing missing libraries through CLI or BUI does <strong>not</strong> promise automatic server re-init; restart the server so startup calls <code>Init</code> again.</p>
          <p>A transcription acquires handle capacity and a model state, performs decode/inference, then releases both on every completion path. A streaming session is longer-lived: opening it reserves a state and capacity until its worker exits. <code>Close</code> requests the normal final flush and waits for that exit; a terminal worker error also exits and releases automatically. Callers should still defer the idempotent <code>Close</code>, including when feed/event handling fails. Unload must not destroy the Whisper context while transcriptions or streams remain active.</p>
          <p>The audio HTTP handler delegates file decoding and transcription to <code>Bucky.TranscribeFile</code>. It explicitly enforces the 25 MB upload limit before allowing unbounded work. Keep protocol field validation/format selection in the handler and audio/model mechanics in Bucky.</p>
          <p>Focused tests that exist include unit tests under <code>sdk/bucky/model/</code> and <code>sdk/bucky/ffmpeg/</code>, transcription/pool/stream suites under <code>sdk/bucky/tests/transcribe/</code>, and the server audio API tests under <code>cmd/server/api/services/kronk/tests/</code>. Choose the narrowest test whose native library and model prerequisites are available. Do not duplicate Chapter 18's usage matrix here.</p>
          <h3 id="199-verification-for-llm-agents">19.9 Verification for LLM Agents</h3>
          <h4 id="1991-required-go-post-edit-sequence">19.9.1 Required Go post-edit sequence</h4>
          <p>After changing Go, obey the root instructions and scope work to the changed package. For each changed Go file/package:</p>
          <pre className="code-block"><code className="language-shell">{`go fix ./path/to/changed/package
gofmt -s -w path/to/all-changed.go
go vet ./path/to/changed/package
staticcheck ./path/to/changed/package`}</code></pre>
          <p>Use exact package paths rather than <code>./...</code>. If several packages changed, list those packages explicitly or run commands separately so failures remain attributable. Review <code>go fix</code> output/diff because it may modify additional files; include every resulting Go file in the subsequent <code>gofmt</code> and review.</p>
          <p>Before focused Go tests, set:</p>
          <pre className="code-block"><code className="language-shell">{`export RUN_IN_PARALLEL=yes
export GITHUB_WORKSPACE="$(pwd -P)"`}</code></pre>
          <p><code>GITHUB_WORKSPACE</code> must be the absolute repository root. Then run a package test or a specific test, for example:</p>
          <pre className="code-block"><code className="language-shell">{`go test -count=1 ./sdk/kronk/model
go test -count=1 -run 'TestSpecificBehavior' ./sdk/kronk/parsers/qwen`}</code></pre>
          <p>Agents must <strong>never prescribe or run a full repository test run</strong>, and must <strong>never launch tests from &lt;code&gt;sdk/kronk/tests&lt;/code&gt;</strong>. Those suites require managed libraries/models and belong to CI or deliberate human integration runs. Commands such as <code>make test</code> exist as broad human/CI-maintainer context, but they are not the agent default. Do not use a broad command merely because focused ownership is unclear; inspect the owner.</p>
          <h4 id="1992-choosing-effective-checks">19.9.2 Choosing effective checks</h4>
          <ul>
            <li>Pure logic changes: focused unit test plus package static checks.</li>
            <li>Public API changes: owner tests, direct dependent package build/test, and generated SDK docs when comments/signatures changed.</li>
            <li>Batch/native changes: focused model unit tests first; report model-backed validation as unavailable if prerequisites are absent rather than substituting unrelated tests.</li>
            <li>Pool changes: engine tests plus the affected typed pool's reservation/load tests. Include failure and cancellation, not only warm acquisition.</li>
            <li>Route changes: domain tests and a server build. Use the relevant API test only when its server/model prerequisites are available.</li>
            <li>CLI changes: command package tests, <code>go install ./cmd/kronk</code>, and one safe invocation.</li>
            <li>Bucky changes: package tests; transcription integration only with installed Whisper libraries/model. Streaming changes require close/cancellation coverage.</li>
          </ul>
          <h4 id="1993-markdown-generated-docs-and-bui">19.9.3 Markdown, generated docs, and BUI</h4>
          <p>For a manual-only edit, run formatting/sanity checks appropriate to the task, including <code>git diff --check</code>. <code>make kronk-docs</code> validates the manual conversion pipeline but also changes generated <code>DocsManual</code>; honor any task restriction on edited files. For normal documentation changes, commit source and generated output together, then run the BUI build.</p>
          <p>For BUI changes, install dependencies according to its lockfile/workflow and run:</p>
          <pre className="code-block"><code className="language-shell">{`npm run build`}</code></pre>
          <p>Run it from <code>cmd/server/api/frontends/bui/</code>. For docs or BUI work that affects the production bundle, also build the server to verify the bundle embedded by <code>cmd/server/api/services/kronk/main.go</code> exists and compiles. A successful Vite build alone does not prove the Go binary contains current assets.</p>
          <p>Always report what actually ran, including skipped integration prerequisites. Never claim CI parity from a narrower local test.</p>
          <h3 id="1910-ci-release-containers-and-nix">19.10 CI, Release, Containers, and Nix</h3>
          <h4 id="19101-linux-ci">19.10.1 Linux CI</h4>
          <p><code>.github/workflows/linux.yml</code> is the authoritative Linux pipeline. It currently has four parallel jobs:</p>
          <ul>
            <li><code>static</code>: source/static quality checks;</li>
            <li><code>race</code>: race-enabled focused coverage separated from static checks;</li>
            <li><code>api-tests</code>: server/API integration coverage;</li>
            <li><code>sdk-tests</code>: SDK/model integration coverage.</li>
          </ul>
          <p>The shared setup action is under <code>.github/actions/setup-kronk/</code>. CI model dependencies are declared in <code>.github/test-models.txt</code>; its contents also participate in cache behavior. When a CI test gains a required model, update the manifest with the correct backend and model ID and check the setup action's parser. Keep local human setup in sync where the Make workflow maintains a separate install list.</p>
          <p>The exact CI toolchain comes from <code>.go-version</code>, while <code>go.mod</code> declares the minimum language version. Their major/minor versions must match. Update workflow assumptions and run <code>.github/scripts/check-go-version.sh</code> when changing either.</p>
          <h4 id="19102-release">19.10.2 Release</h4>
          <p>The release workflow, GoReleaser configuration, scripts, and release notes divide responsibility:</p>
          <ul>
            <li><code>.github/workflows/release.yaml</code> owns trigger, permissions, setup, checks, and release execution.</li>
            <li><code>.goreleaser.yaml</code> owns binary/archive packaging and related release products.</li>
            <li><code>.github/scripts/check-version.sh</code> enforces the release identity.</li>
            <li><code>sdk/kronk/kronk.go</code> owns the exported <code>Version</code> constant.</li>
            <li><code>.release/</code> owns maintained release-note/checklist material.</li>
          </ul>
          <p>The release tag must equal <code>v</code> plus the <code>Version</code> constant. Update the constant and release material intentionally before creating the tag; do not bypass the guard. Also confirm the Go major/minor guard, generated docs/BUI, clean tree, and relevant Linux jobs before tagging.</p>
          <h4 id="19103-containers">19.10.3 Containers</h4>
          <p><code>.github/workflows/docker.yml</code> is authoritative for image variants, target/platform matrix, registry publication, attestations, and signing. <code>zarf/docker/</code> owns Dockerfile, runtime configuration, and entrypoint behavior. Native llama and Bucky processor availability can differ by image variant, so inspect the workflow matrix and tooling combination tables before changing an image. Avoid copying a variant table into docs; it becomes stale faster than the workflow.</p>
          <p>For a container change, build the affected target and architecture where practical, exercise entrypoint startup/configuration, and verify expected native libraries. Do not infer publication or signature behavior from a local build; review the workflow.</p>
          <h4 id="19104-nix">19.10.4 Nix</h4>
          <p>The flake at <code>zarf/nix/flake.nix</code> defines how developers/users enter or build the project; generated Go dependency data lives beside it. Entering a development shell runs <code>gomod2nix import</code> from its shell hook and may dirty that generated material. When Go module dependencies change, update the Nix dependency material with the repository's configured command and evaluate/build the relevant entry point. Keep Nix fixes in Nix owners rather than adding environment special cases to Go code.</p>
          <h3 id="1911-change-and-release-checklists">19.11 Change and Release Checklists</h3>
          <h4 id="19111-focused-change-checklist">19.11.1 Focused change checklist</h4>
          <ul>
            <li>[ ] Read all applicable <code>AGENTS.md</code> files.</li>
            <li>[ ] Locate the owning package, direct caller, and focused tests.</li>
            <li>[ ] State lifecycle/mutation/resource invariants before editing.</li>
            <li>[ ] Change the owner rather than duplicating logic in a facade or transport.</li>
            <li>[ ] Preserve cancellation and release behavior on every return path.</li>
            <li>[ ] For Go: run <code>gofmt -s</code>, <code>go fix</code>, <code>go vet</code>, and <code>staticcheck</code> scoped to changed files/packages.</li>
            <li>[ ] Set <code>RUN_IN_PARALLEL=yes</code> and absolute <code>GITHUB_WORKSPACE</code> for focused tests.</li>
            <li>[ ] Do not run a full repository suite or launch tests from <code>sdk/kronk/tests</code> as an agent.</li>
            <li>[ ] Regenerate docs/BUI/Nix artifacts only when their sources changed and task scope permits it.</li>
            <li>[ ] Run <code>git diff --check</code> and inspect the complete diff for unrelated changes.</li>
            <li>[ ] Report commands, results, skipped prerequisites, and residual uncertainty.</li>
          </ul>
          <h4 id="19112-release-checklist">19.11.2 Release checklist</h4>
          <ul>
            <li>[ ] Choose the release version and update <code>sdk/kronk</code>'s <code>Version</code> constant.</li>
            <li>[ ] Ensure the intended tag is exactly <code>v&lt;Version&gt;</code> and run the version guard.</li>
            <li>[ ] Confirm <code>.go-version</code> and <code>go.mod</code> major/minor agreement and run the Go-version guard.</li>
            <li>[ ] Update release notes/changelog material under the repository's release process.</li>
            <li>[ ] Ensure <code>.github/test-models.txt</code> covers model-backed CI requirements.</li>
            <li>[ ] Regenerate documentation and BUI assets; build the BUI and server embedding.</li>
            <li>[ ] Confirm focused package checks and the four Linux CI jobs are green.</li>
            <li>[ ] Review <code>.github/workflows/docker.yml</code> for the intended image variants and publication/signing behavior.</li>
            <li>[ ] Review Nix dependency outputs if Go dependencies changed.</li>
            <li>[ ] Verify GoReleaser configuration with an appropriate non-publishing check or snapshot.</li>
            <li>[ ] Confirm the release commit is clean, then create/push the guarded tag through the maintainer release process.</li>
          </ul>
          <p>The purpose of these lists is to protect ownership and lifecycle contracts, not to turn every patch into a release. Use the focused list for ordinary work and reserve broad integration/release machinery for humans and CI with the required models, native libraries, credentials, and platforms.</p>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#chapter-1-introduction" className={`doc-index-header ${activeSection === 'chapter-1-introduction' ? 'active' : ''}`}>Chapter 1: Introduction</a>
              <ul>
                <li><a href="#11-what-is-kronk?" className={activeSection === '11-what-is-kronk?' ? 'active' : ''}>1.1 What Is Kronk?</a></li>
                <li><a href="#12-sdk-or-model-server?" className={activeSection === '12-sdk-or-model-server?' ? 'active' : ''}>1.2 SDK or Model Server?</a></li>
                <li><a href="#13-capabilities" className={activeSection === '13-capabilities' ? 'active' : ''}>1.3 Capabilities</a></li>
                <li><a href="#14-architecture" className={activeSection === '14-architecture' ? 'active' : ''}>1.4 Architecture</a></li>
                <li><a href="#15-where-to-go-next" className={activeSection === '15-where-to-go-next' ? 'active' : ''}>1.5 Where to Go Next</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-2-installation-quick-start" className={`doc-index-header ${activeSection === 'chapter-2-installation-quick-start' ? 'active' : ''}`}>Chapter 2: Installation &amp; Quick Start</a>
              <ul>
                <li><a href="#21-native-quick-start" className={activeSection === '21-native-quick-start' ? 'active' : ''}>2.1 Native Quick Start</a></li>
                <li><a href="#22-choose-an-installation-method" className={activeSection === '22-choose-an-installation-method' ? 'active' : ''}>2.2 Choose an Installation Method</a></li>
                <li><a href="#23-container-quick-start" className={activeSection === '23-container-quick-start' ? 'active' : ''}>2.3 Container Quick Start</a></li>
                <li><a href="#24-libraries" className={activeSection === '24-libraries' ? 'active' : ''}>2.4 Libraries</a></li>
                <li><a href="#25-models-and-data-paths" className={activeSection === '25-models-and-data-paths' ? 'active' : ''}>2.5 Models and Data Paths</a></li>
                <li><a href="#26-running-the-server" className={activeSection === '26-running-the-server' ? 'active' : ''}>2.6 Running the Server</a></li>
                <li><a href="#27-verify-the-installation" className={activeSection === '27-verify-the-installation' ? 'active' : ''}>2.7 Verify the Installation</a></li>
                <li><a href="#28-security-and-next-steps" className={activeSection === '28-security-and-next-steps' ? 'active' : ''}>2.8 Security and Next Steps</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-3-model-configuration" className={`doc-index-header ${activeSection === 'chapter-3-model-configuration' ? 'active' : ''}`}>Chapter 3: Model Configuration</a>
              <ul>
                <li><a href="#31-configuration-file" className={activeSection === '31-configuration-file' ? 'active' : ''}>3.1 Configuration File</a></li>
                <li><a href="#32-automatic-tuning" className={activeSection === '32-automatic-tuning' ? 'active' : ''}>3.2 Automatic Tuning</a></li>
                <li><a href="#33-core-runtime-settings" className={activeSection === '33-core-runtime-settings' ? 'active' : ''}>3.3 Core Runtime Settings</a></li>
                <li><a href="#34-gpu-and-memory-placement" className={activeSection === '34-gpu-and-memory-placement' ? 'active' : ''}>3.4 GPU and Memory Placement</a></li>
                <li><a href="#35-concurrency-and-batching" className={activeSection === '35-concurrency-and-batching' ? 'active' : ''}>3.5 Concurrency and Batching</a></li>
                <li><a href="#36-memory-planning-and-quantization" className={activeSection === '36-memory-planning-and-quantization' ? 'active' : ''}>3.6 Memory Planning and Quantization</a></li>
                <li><a href="#37-advanced-features" className={activeSection === '37-advanced-features' ? 'active' : ''}>3.7 Advanced Features</a></li>
                <li><a href="#38-complete-example-and-key-reference" className={activeSection === '38-complete-example-and-key-reference' ? 'active' : ''}>3.8 Complete Example and Key Reference</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-4-batch-processing" className={`doc-index-header ${activeSection === 'chapter-4-batch-processing' ? 'active' : ''}`}>Chapter 4: Batch Processing</a>
              <ul>
                <li><a href="#41-concurrency-at-a-glance" className={activeSection === '41-concurrency-at-a-glance' ? 'active' : ''}>4.1 Concurrency at a Glance</a></li>
                <li><a href="#42-generation-slots-and-sequences" className={activeSection === '42-generation-slots-and-sequences' ? 'active' : ''}>4.2 Generation Slots and Sequences</a></li>
                <li><a href="#43-admission-waiting-and-cancellation" className={activeSection === '43-admission-waiting-and-cancellation' ? 'active' : ''}>4.3 Admission, Waiting, and Cancellation</a></li>
                <li><a href="#44-prompt-and-token-scheduling" className={activeSection === '44-prompt-and-token-scheduling' ? 'active' : ''}>4.4 Prompt and Token Scheduling</a></li>
                <li><a href="#45-embedding-and-reranking" className={activeSection === '45-embedding-and-reranking' ? 'active' : ''}>4.5 Embedding and Reranking</a></li>
                <li><a href="#46-configuration-and-tuning" className={activeSection === '46-configuration-and-tuning' ? 'active' : ''}>4.6 Configuration and Tuning</a></li>
                <li><a href="#47-interaction-with-message-caching" className={activeSection === '47-interaction-with-message-caching' ? 'active' : ''}>4.7 Interaction with Message Caching</a></li>
                <li><a href="#48-observing-queue-behavior" className={activeSection === '48-observing-queue-behavior' ? 'active' : ''}>4.8 Observing Queue Behavior</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-5-message-caching" className={`doc-index-header ${activeSection === 'chapter-5-message-caching' ? 'active' : ''}`}>Chapter 5: Message Caching</a>
            </div>
            <div className="doc-index-section">
              <a href="#51-what-imc-does" className={`doc-index-header ${activeSection === '51-what-imc-does' ? 'active' : ''}`}>5.1 What IMC Does</a>
            </div>
            <div className="doc-index-section">
              <a href="#52-how-kronk-reuses-a-text-prefix" className={`doc-index-header ${activeSection === '52-how-kronk-reuses-a-text-prefix' ? 'active' : ''}`}>5.2 How Kronk Reuses a Text Prefix</a>
            </div>
            <div className="doc-index-section">
              <a href="#53-sessions-slots-and-snapshots" className={`doc-index-header ${activeSection === '53-sessions-slots-and-snapshots' ? 'active' : ''}`}>5.3 Sessions, Slots, and Snapshots</a>
            </div>
            <div className="doc-index-section">
              <a href="#54-media-requests" className={`doc-index-header ${activeSection === '54-media-requests' ? 'active' : ''}`}>5.4 Media Requests</a>
            </div>
            <div className="doc-index-section">
              <a href="#55-configuration-and-storage" className={`doc-index-header ${activeSection === '55-configuration-and-storage' ? 'active' : ''}`}>5.5 Configuration and Storage</a>
              <ul>
                <li><a href="#ram-storage" className={activeSection === 'ram-storage' ? 'active' : ''}>RAM storage</a></li>
                <li><a href="#disk-storage" className={activeSection === 'disk-storage' ? 'active' : ''}>Disk storage</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#56-invalidation-and-limitations" className={`doc-index-header ${activeSection === '56-invalidation-and-limitations' ? 'active' : ''}`}>5.6 Invalidation and Limitations</a>
            </div>
            <div className="doc-index-section">
              <a href="#57-observability" className={`doc-index-header ${activeSection === '57-observability' ? 'active' : ''}`}>5.7 Observability</a>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-6-speculative-decoding-and-mtp" className={`doc-index-header ${activeSection === 'chapter-6-speculative-decoding-and-mtp' ? 'active' : ''}`}>Chapter 6: Speculative Decoding and MTP</a>
              <ul>
                <li><a href="#61-what-speculative-decoding-does" className={activeSection === '61-what-speculative-decoding-does' ? 'active' : ''}>6.1 What Speculative Decoding Does</a></li>
                <li><a href="#62-drafter-sources-and-selection" className={activeSection === '62-drafter-sources-and-selection' ? 'active' : ''}>6.2 Drafter Sources and Selection</a></li>
                <li><a href="#63-choosing-a-drafter" className={activeSection === '63-choosing-a-drafter' ? 'active' : ''}>6.3 Choosing a Drafter</a></li>
                <li><a href="#64-draft-size-and-adaptive-throttling" className={activeSection === '64-draft-size-and-adaptive-throttling' ? 'active' : ''}>6.4 Draft Size and Adaptive Throttling</a></li>
                <li><a href="#65-configuration" className={activeSection === '65-configuration' ? 'active' : ''}>6.5 Configuration</a></li>
                <li><a href="#66-measuring-the-result" className={activeSection === '66-measuring-the-result' ? 'active' : ''}>6.6 Measuring the Result</a></li>
                <li><a href="#67-limitations-and-fallbacks" className={activeSection === '67-limitations-and-fallbacks' ? 'active' : ''}>6.7 Limitations and Fallbacks</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-7-yarn-extended-context" className={`doc-index-header ${activeSection === 'chapter-7-yarn-extended-context' ? 'active' : ''}`}>Chapter 7: YaRN Extended Context</a>
            </div>
            <div className="doc-index-section">
              <a href="#71-context-size-and-rope-scaling" className={`doc-index-header ${activeSection === '71-context-size-and-rope-scaling' ? 'active' : ''}`}>7.1 Context Size and RoPE Scaling</a>
            </div>
            <div className="doc-index-section">
              <a href="#72-when-to-use-yarn" className={`doc-index-header ${activeSection === '72-when-to-use-yarn' ? 'active' : ''}`}>7.2 When to Use YaRN</a>
            </div>
            <div className="doc-index-section">
              <a href="#73-qwen3-configuration" className={`doc-index-header ${activeSection === '73-qwen3-configuration' ? 'active' : ''}`}>7.3 Qwen3 Configuration</a>
            </div>
            <div className="doc-index-section">
              <a href="#74-scaling-types-and-parameters" className={`doc-index-header ${activeSection === '74-scaling-types-and-parameters' ? 'active' : ''}`}>7.4 Scaling Types and Parameters</a>
            </div>
            <div className="doc-index-section">
              <a href="#75-memory-and-concurrency" className={`doc-index-header ${activeSection === '75-memory-and-concurrency' ? 'active' : ''}`}>7.5 Memory and Concurrency</a>
            </div>
            <div className="doc-index-section">
              <a href="#76-validate-quality" className={`doc-index-header ${activeSection === '76-validate-quality' ? 'active' : ''}`}>7.6 Validate Quality</a>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-8-model-server" className={`doc-index-header ${activeSection === 'chapter-8-model-server' ? 'active' : ''}`}>Chapter 8: Model Server</a>
            </div>
            <div className="doc-index-section">
              <a href="#81-server-lifecycle" className={`doc-index-header ${activeSection === '81-server-lifecycle' ? 'active' : ''}`}>8.1 Server Lifecycle</a>
            </div>
            <div className="doc-index-section">
              <a href="#82-local-and-server-backed-cli-commands" className={`doc-index-header ${activeSection === '82-local-and-server-backed-cli-commands' ? 'active' : ''}`}>8.2 Local and Server-Backed CLI Commands</a>
            </div>
            <div className="doc-index-section">
              <a href="#83-essential-server-configuration" className={`doc-index-header ${activeSection === '83-essential-server-configuration' ? 'active' : ''}`}>8.3 Essential Server Configuration</a>
            </div>
            <div className="doc-index-section">
              <a href="#84-model-pool-and-resource-budgets" className={`doc-index-header ${activeSection === '84-model-pool-and-resource-budgets' ? 'active' : ''}`}>8.4 Model Pool and Resource Budgets</a>
            </div>
            <div className="doc-index-section">
              <a href="#85-model-configuration-files" className={`doc-index-header ${activeSection === '85-model-configuration-files' ? 'active' : ''}`}>8.5 Model Configuration Files</a>
            </div>
            <div className="doc-index-section">
              <a href="#86-catalog-operations" className={`doc-index-header ${activeSection === '86-catalog-operations' ? 'active' : ''}`}>8.6 Catalog Operations</a>
            </div>
            <div className="doc-index-section">
              <a href="#87-container-operations" className={`doc-index-header ${activeSection === '87-container-operations' ? 'active' : ''}`}>8.7 Container Operations</a>
            </div>
            <div className="doc-index-section">
              <a href="#88-related-administration-guides" className={`doc-index-header ${activeSection === '88-related-administration-guides' ? 'active' : ''}`}>8.8 Related Administration Guides</a>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-9-api-endpoints" className={`doc-index-header ${activeSection === 'chapter-9-api-endpoints' ? 'active' : ''}`}>Chapter 9: API Endpoints</a>
            </div>
            <div className="doc-index-section">
              <a href="#91-api-conventions" className={`doc-index-header ${activeSection === '91-api-conventions' ? 'active' : ''}`}>9.1 API Conventions</a>
            </div>
            <div className="doc-index-section">
              <a href="#92-endpoint-overview" className={`doc-index-header ${activeSection === '92-endpoint-overview' ? 'active' : ''}`}>9.2 Endpoint Overview</a>
            </div>
            <div className="doc-index-section">
              <a href="#93-chat-completions-and-tool-calls" className={`doc-index-header ${activeSection === '93-chat-completions-and-tool-calls' ? 'active' : ''}`}>9.3 Chat Completions and Tool Calls</a>
              <ul>
                <li><a href="#tool-calls" className={activeSection === 'tool-calls' ? 'active' : ''}>Tool calls</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#94-responses-api" className={`doc-index-header ${activeSection === '94-responses-api' ? 'active' : ''}`}>9.4 Responses API</a>
            </div>
            <div className="doc-index-section">
              <a href="#95-anthropic-messages-api" className={`doc-index-header ${activeSection === '95-anthropic-messages-api' ? 'active' : ''}`}>9.5 Anthropic Messages API</a>
            </div>
            <div className="doc-index-section">
              <a href="#96-embeddings" className={`doc-index-header ${activeSection === '96-embeddings' ? 'active' : ''}`}>9.6 Embeddings</a>
            </div>
            <div className="doc-index-section">
              <a href="#97-reranking" className={`doc-index-header ${activeSection === '97-reranking' ? 'active' : ''}`}>9.7 Reranking</a>
            </div>
            <div className="doc-index-section">
              <a href="#98-tokenization" className={`doc-index-header ${activeSection === '98-tokenization' ? 'active' : ''}`}>9.8 Tokenization</a>
            </div>
            <div className="doc-index-section">
              <a href="#99-models-and-audio-transcription" className={`doc-index-header ${activeSection === '99-models-and-audio-transcription' ? 'active' : ''}`}>9.9 Models and Audio Transcription</a>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-10-request-parameters" className={`doc-index-header ${activeSection === 'chapter-10-request-parameters' ? 'active' : ''}`}>Chapter 10: Request Parameters</a>
            </div>
            <div className="doc-index-section">
              <a href="#101-scope-and-defaults" className={`doc-index-header ${activeSection === '101-scope-and-defaults' ? 'active' : ''}`}>10.1 Scope and Defaults</a>
            </div>
            <div className="doc-index-section">
              <a href="#102-core-sampling" className={`doc-index-header ${activeSection === '102-core-sampling' ? 'active' : ''}`}>10.2 Core Sampling</a>
            </div>
            <div className="doc-index-section">
              <a href="#103-repetition-control" className={`doc-index-header ${activeSection === '103-repetition-control' ? 'active' : ''}`}>10.3 Repetition Control</a>
            </div>
            <div className="doc-index-section">
              <a href="#104-advanced-sampling" className={`doc-index-header ${activeSection === '104-advanced-sampling' ? 'active' : ''}`}>10.4 Advanced Sampling</a>
            </div>
            <div className="doc-index-section">
              <a href="#105-generation-and-reasoning" className={`doc-index-header ${activeSection === '105-generation-and-reasoning' ? 'active' : ''}`}>10.5 Generation and Reasoning</a>
            </div>
            <div className="doc-index-section">
              <a href="#106-structured-output" className={`doc-index-header ${activeSection === '106-structured-output' ? 'active' : ''}`}>10.6 Structured Output</a>
            </div>
            <div className="doc-index-section">
              <a href="#107-token-log-probabilities" className={`doc-index-header ${activeSection === '107-token-log-probabilities' ? 'active' : ''}`}>10.7 Token Log Probabilities</a>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-11-multimodal-models" className={`doc-index-header ${activeSection === 'chapter-11-multimodal-models' ? 'active' : ''}`}>Chapter 11: Multimodal Models</a>
            </div>
            <div className="doc-index-section">
              <a href="#111-models-and-projectors" className={`doc-index-header ${activeSection === '111-models-and-projectors' ? 'active' : ''}`}>11.1 Models and Projectors</a>
            </div>
            <div className="doc-index-section">
              <a href="#112-supported-inputs" className={`doc-index-header ${activeSection === '112-supported-inputs' ? 'active' : ''}`}>11.2 Supported Inputs</a>
            </div>
            <div className="doc-index-section">
              <a href="#113-sending-an-image" className={`doc-index-header ${activeSection === '113-sending-an-image' ? 'active' : ''}`}>11.3 Sending an Image</a>
            </div>
            <div className="doc-index-section">
              <a href="#114-sending-audio" className={`doc-index-header ${activeSection === '114-sending-audio' ? 'active' : ''}`}>11.4 Sending Audio</a>
            </div>
            <div className="doc-index-section">
              <a href="#115-go-sdk-helpers" className={`doc-index-header ${activeSection === '115-go-sdk-helpers' ? 'active' : ''}`}>11.5 Go SDK Helpers</a>
            </div>
            <div className="doc-index-section">
              <a href="#116-configuration-and-resources" className={`doc-index-header ${activeSection === '116-configuration-and-resources' ? 'active' : ''}`}>11.6 Configuration and Resources</a>
            </div>
            <div className="doc-index-section">
              <a href="#117-message-caching" className={`doc-index-header ${activeSection === '117-message-caching' ? 'active' : ''}`}>11.7 Message Caching</a>
            </div>
            <div className="doc-index-section">
              <a href="#118-limitations" className={`doc-index-header ${activeSection === '118-limitations' ? 'active' : ''}`}>11.8 Limitations</a>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-12-security-and-authentication" className={`doc-index-header ${activeSection === 'chapter-12-security-and-authentication' ? 'active' : ''}`}>Chapter 12: Security and Authentication</a>
            </div>
            <div className="doc-index-section">
              <a href="#121-authentication-modes" className={`doc-index-header ${activeSection === '121-authentication-modes' ? 'active' : ''}`}>12.1 Authentication Modes</a>
            </div>
            <div className="doc-index-section">
              <a href="#122-initial-credentials" className={`doc-index-header ${activeSection === '122-initial-credentials' ? 'active' : ''}`}>12.2 Initial Credentials</a>
            </div>
            <div className="doc-index-section">
              <a href="#123-admin-and-user-tokens" className={`doc-index-header ${activeSection === '123-admin-and-user-tokens' ? 'active' : ''}`}>12.3 Admin and User Tokens</a>
            </div>
            <div className="doc-index-section">
              <a href="#124-endpoint-grants-and-rate-limits" className={`doc-index-header ${activeSection === '124-endpoint-grants-and-rate-limits' ? 'active' : ''}`}>12.4 Endpoint Grants and Rate Limits</a>
            </div>
            <div className="doc-index-section">
              <a href="#125-using-a-token" className={`doc-index-header ${activeSection === '125-using-a-token' ? 'active' : ''}`}>12.5 Using a Token</a>
            </div>
            <div className="doc-index-section">
              <a href="#126-key-rotation-and-revocation" className={`doc-index-header ${activeSection === '126-key-rotation-and-revocation' ? 'active' : ''}`}>12.6 Key Rotation and Revocation</a>
            </div>
            <div className="doc-index-section">
              <a href="#127-embedded-and-standalone-authentication" className={`doc-index-header ${activeSection === '127-embedded-and-standalone-authentication' ? 'active' : ''}`}>12.7 Embedded and Standalone Authentication</a>
            </div>
            <div className="doc-index-section">
              <a href="#128-production-hardening" className={`doc-index-header ${activeSection === '128-production-hardening' ? 'active' : ''}`}>12.8 Production Hardening</a>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-13-browser-ui-bui" className={`doc-index-header ${activeSection === 'chapter-13-browser-ui-bui' ? 'active' : ''}`}>Chapter 13: Browser UI (BUI)</a>
              <ul>
                <li><a href="#131-accessing-the-bui" className={activeSection === '131-accessing-the-bui' ? 'active' : ''}>13.1 Accessing the BUI</a></li>
                <li><a href="#132-capabilities" className={activeSection === '132-capabilities' ? 'active' : ''}>13.2 Capabilities</a></li>
                <li><a href="#133-authentication" className={activeSection === '133-authentication' ? 'active' : ''}>13.3 Authentication</a></li>
                <li><a href="#134-operational-notes" className={activeSection === '134-operational-notes' ? 'active' : ''}>13.4 Operational Notes</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-14-client-integration" className={`doc-index-header ${activeSection === 'chapter-14-client-integration' ? 'active' : ''}`}>Chapter 14: Client Integration</a>
              <ul>
                <li><a href="#141-connection-settings" className={activeSection === '141-connection-settings' ? 'active' : ''}>14.1 Connection Settings</a></li>
                <li><a href="#142-opencode" className={activeSection === '142-opencode' ? 'active' : ''}>14.2 OpenCode</a></li>
                <li><a href="#143-openwebui" className={activeSection === '143-openwebui' ? 'active' : ''}>14.3 OpenWebUI</a></li>
                <li><a href="#144-python-openai-sdk" className={activeSection === '144-python-openai-sdk' ? 'active' : ''}>14.4 Python OpenAI SDK</a></li>
                <li><a href="#145-curl-and-other-http-clients" className={activeSection === '145-curl-and-other-http-clients' ? 'active' : ''}>14.5 curl and Other HTTP Clients</a></li>
                <li><a href="#146-langchain" className={activeSection === '146-langchain' ? 'active' : ''}>14.6 LangChain</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-15-observability" className={`doc-index-header ${activeSection === 'chapter-15-observability' ? 'active' : ''}`}>Chapter 15: Observability</a>
              <ul>
                <li><a href="#151-debug-and-health-endpoints" className={activeSection === '151-debug-and-health-endpoints' ? 'active' : ''}>15.1 Debug and Health Endpoints</a></li>
                <li><a href="#152-prometheus-metrics" className={activeSection === '152-prometheus-metrics' ? 'active' : ''}>15.2 Prometheus Metrics</a></li>
                <li><a href="#153-bundled-observability-stack" className={activeSection === '153-bundled-observability-stack' ? 'active' : ''}>15.3 Bundled Observability Stack</a></li>
                <li><a href="#154-opentelemetry-tracing" className={activeSection === '154-opentelemetry-tracing' ? 'active' : ''}>15.4 OpenTelemetry Tracing</a></li>
                <li><a href="#155-profiling-and-runtime-visualization" className={activeSection === '155-profiling-and-runtime-visualization' ? 'active' : ''}>15.5 Profiling and Runtime Visualization</a></li>
                <li><a href="#156-logging" className={activeSection === '156-logging' ? 'active' : ''}>15.6 Logging</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-16-mcp-service" className={`doc-index-header ${activeSection === 'chapter-16-mcp-service' ? 'active' : ''}`}>Chapter 16: MCP Service</a>
              <ul>
                <li><a href="#161-architecture-and-security" className={activeSection === '161-architecture-and-security' ? 'active' : ''}>16.1 Architecture and Security</a></li>
                <li><a href="#162-prerequisites" className={activeSection === '162-prerequisites' ? 'active' : ''}>16.2 Prerequisites</a></li>
                <li><a href="#163-configuration" className={activeSection === '163-configuration' ? 'active' : ''}>16.3 Configuration</a></li>
                <li><a href="#164-available-tools" className={activeSection === '164-available-tools' ? 'active' : ''}>16.4 Available Tools</a></li>
                <li><a href="#165-client-configuration" className={activeSection === '165-client-configuration' ? 'active' : ''}>16.5 Client Configuration</a></li>
                <li><a href="#166-testing-with-curl" className={activeSection === '166-testing-with-curl' ? 'active' : ''}>16.6 Testing with curl</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-17-troubleshooting" className={`doc-index-header ${activeSection === 'chapter-17-troubleshooting' ? 'active' : ''}`}>Chapter 17: Troubleshooting</a>
              <ul>
                <li><a href="#171-start-with-diagnostics" className={activeSection === '171-start-with-diagnostics' ? 'active' : ''}>17.1 Start with Diagnostics</a></li>
                <li><a href="#172-libraries-and-devices" className={activeSection === '172-libraries-and-devices' ? 'active' : ''}>17.2 Libraries and Devices</a></li>
                <li><a href="#173-models-catalog-and-storage" className={activeSection === '173-models-catalog-and-storage' ? 'active' : ''}>17.3 Models, Catalog, and Storage</a></li>
                <li><a href="#174-memory-and-performance" className={activeSection === '174-memory-and-performance' ? 'active' : ''}>17.4 Memory and Performance</a></li>
                <li><a href="#175-requests-and-streaming" className={activeSection === '175-requests-and-streaming' ? 'active' : ''}>17.5 Requests and Streaming</a></li>
                <li><a href="#176-authentication" className={activeSection === '176-authentication' ? 'active' : ''}>17.6 Authentication</a></li>
                <li><a href="#177-imc" className={activeSection === '177-imc' ? 'active' : ''}>17.7 IMC</a></li>
                <li><a href="#178-mcp" className={activeSection === '178-mcp' ? 'active' : ''}>17.8 MCP</a></li>
                <li><a href="#179-ports-processes-and-permissions" className={activeSection === '179-ports-processes-and-permissions' ? 'active' : ''}>17.9 Ports, Processes, and Permissions</a></li>
                <li><a href="#1710-reporting-a-problem" className={activeSection === '1710-reporting-a-problem' ? 'active' : ''}>17.10 Reporting a Problem</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-18-bucky-audio-transcription" className={`doc-index-header ${activeSection === 'chapter-18-bucky-audio-transcription' ? 'active' : ''}`}>Chapter 18: Bucky (Audio Transcription)</a>
              <ul>
                <li><a href="#181-overview" className={activeSection === '181-overview' ? 'active' : ''}>18.1 Overview</a></li>
                <li><a href="#182-install-whisper-libraries" className={activeSection === '182-install-whisper-libraries' ? 'active' : ''}>18.2 Install Whisper Libraries</a></li>
                <li><a href="#183-manage-models" className={activeSection === '183-manage-models' ? 'active' : ''}>18.3 Manage Models</a></li>
                <li><a href="#184-server-configuration" className={activeSection === '184-server-configuration' ? 'active' : ''}>18.4 Server Configuration</a></li>
                <li><a href="#185-browser-ui" className={activeSection === '185-browser-ui' ? 'active' : ''}>18.5 Browser UI</a></li>
                <li><a href="#186-transcriptions-api" className={activeSection === '186-transcriptions-api' ? 'active' : ''}>18.6 Transcriptions API</a></li>
                <li><a href="#187-go-sdk" className={activeSection === '187-go-sdk' ? 'active' : ''}>18.7 Go SDK</a></li>
                <li><a href="#188-languages" className={activeSection === '188-languages' ? 'active' : ''}>18.8 Languages</a></li>
                <li><a href="#189-troubleshooting" className={activeSection === '189-troubleshooting' ? 'active' : ''}>18.9 Troubleshooting</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-19-developer-guide" className={`doc-index-header ${activeSection === 'chapter-19-developer-guide' ? 'active' : ''}`}>Chapter 19: Developer Guide</a>
              <ul>
                <li><a href="#191-how-to-use-this-guide" className={activeSection === '191-how-to-use-this-guide' ? 'active' : ''}>19.1 How to Use This Guide</a></li>
                <li><a href="#192-task-to-owner-and-verification-map" className={activeSection === '192-task-to-owner-and-verification-map' ? 'active' : ''}>19.2 Task-to-Owner and Verification Map</a></li>
                <li><a href="#193-repository-ownership-map" className={activeSection === '193-repository-ownership-map' ? 'active' : ''}>19.3 Repository Ownership Map</a></li>
                <li><a href="#194-developer-setup-and-daily-commands" className={activeSection === '194-developer-setup-and-daily-commands' ? 'active' : ''}>19.4 Developer Setup and Daily Commands</a></li>
                <li><a href="#195-request-and-model-lifecycle" className={activeSection === '195-request-and-model-lifecycle' ? 'active' : ''}>19.5 Request and Model Lifecycle</a></li>
                <li><a href="#196-core-inference-invariants" className={activeSection === '196-core-inference-invariants' ? 'active' : ''}>19.6 Core Inference Invariants</a></li>
                <li><a href="#197-server-bui-and-generated-documentation" className={activeSection === '197-server-bui-and-generated-documentation' ? 'active' : ''}>19.7 Server, BUI, and Generated Documentation</a></li>
                <li><a href="#198-bucky-implementation-map" className={activeSection === '198-bucky-implementation-map' ? 'active' : ''}>19.8 Bucky Implementation Map</a></li>
                <li><a href="#199-verification-for-llm-agents" className={activeSection === '199-verification-for-llm-agents' ? 'active' : ''}>19.9 Verification for LLM Agents</a></li>
                <li><a href="#1910-ci-release-containers-and-nix" className={activeSection === '1910-ci-release-containers-and-nix' ? 'active' : ''}>19.10 CI, Release, Containers, and Nix</a></li>
                <li><a href="#1911-change-and-release-checklists" className={activeSection === '1911-change-and-release-checklists' ? 'active' : ''}>19.11 Change and Release Checklists</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
