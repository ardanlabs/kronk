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
          <h3 id="11-what-is-kronk">1.1 What Is Kronk</h3>
          <p>Kronk is a Go SDK and model server for running open-source models on your own hardware. It uses three C++ inference engines:</p>
          <ul>
            <li><a href="https://github.com/ggml-org/llama.cpp">llama.cpp</a> runs GGUF text, vision, embedding, and reranking models.</li>
            <li><a href="https://github.com/ggml-org/whisper.cpp">whisper.cpp</a> runs speech-to-text models through Kronk's <strong>Bucky</strong> subsystem.</li>
            <li><a href="https://github.com/leejet/stable-diffusion.cpp">stable-diffusion.cpp</a> runs image-generation models through Kronk's experimental <strong>Malina</strong> subsystem.</li>
          </ul>
          <p>Kronk provides hardware-accelerated text generation, image understanding, audio transcription, image generation, embeddings, and reranking from Go without requiring Python or a CGO build toolchain. Runtime inference still requires compatible native libraries, which Kronk's library managers can download for supported platforms.</p>
          <h3 id="12-sdk-or-model-server">1.2 SDK or Model Server</h3>
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
          <p>Standalone SDK examples are available in the <a href="https://github.com/ardanlabs/kronk/tree/main/examples"><code>examples</code></a> directory.</p>
          <h3 id="13-capabilities">1.3 Capabilities</h3>
          <p><strong>Inference</strong></p>
          <ul>
            <li><strong>Text generation</strong> — chat completions and responses, including streaming, reasoning, and tool calls.</li>
            <li><strong>Vision</strong> — image understanding with compatible multimodal models.</li>
            <li><strong>Audio transcription</strong> — speech-to-text through Bucky and whisper.cpp. See <a href="https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription">Chapter 18: Bucky</a>.</li>
            <li><strong>Image generation and editing</strong> — text-to-image and image-to-image through experimental Malina and stable-diffusion.cpp. See <a href="https://www.kronkai.com/manual#chapter-19-malina-image-generation">Chapter 19: Malina</a> for server capabilities and additional SDK-only operations.</li>
            <li><strong>Embeddings and reranking</strong> — vector generation and document relevance scoring for search and retrieval systems.</li>
            <li><strong>Structured decisions</strong> — evaluate choice, score, and calibrated yes/no questions against shared application state with supported decision models.</li>
          </ul>
          <p><strong>Performance</strong></p>
          <ul>
            <li><strong>Concurrent processing</strong> — serve multiple requests against a loaded model within configured concurrency and memory limits.</li>
            <li><strong>Message caching</strong> — reuse computed prompt prefixes instead of evaluating them again for every request.</li>
            <li><strong>Extended context</strong> — configure YaRN context extension for compatible models.</li>
            <li><strong>Model pooling</strong> — retain loaded models within per-backend count limits and shared RAM/VRAM budgets, evicting idle models when space is needed. Optional time-to-live expiration can also unload idle models; it is disabled by default.</li>
          </ul>
          <p><strong>Operation</strong></p>
          <ul>
            <li><strong>Model catalog</strong> — discover, download, and configure a curated set of models through the CLI or Browser UI (BUI).</li>
            <li><strong>Client APIs</strong> — use OpenAI-compatible endpoints, the Anthropic-compatible Messages endpoint, or integrations such as OpenWebUI and OpenCode.</li>
            <li><strong>Security</strong> — protect endpoints with JWT authentication, scoped authorization, and rate limits.</li>
            <li><strong>Observability</strong> — collect traces and Prometheus metrics for external observability systems.</li>
          </ul>
          <p>Kronk stores its managed files under <code>~/.kronk/</code> by default. The location is configurable; the official containers use <code>/kronk</code> so the directory can be mounted as persistent storage.</p>
          <p>Hardware support differs by operating system, architecture, inference engine, and release artifact. Kronk can use CPU inference and GPU backends such as Metal, CUDA, Vulkan, and ROCm where a compatible library bundle is available. See <a href="https://www.kronkai.com/manual#chapter-2-installation-quick-start">Chapter 2: Installation & Quick Start</a> for the currently distributed native and container options.</p>
          <p>Memory requirements depend on more than model parameter count. Quantization, context size, KV cache type, batch size, concurrency, and multimodal projections all affect RAM and VRAM use. See <a href="https://www.kronkai.com/manual#chapter-3-model-configuration">Chapter 3: Model Configuration</a> before selecting a large model or context window.</p>
          <h3 id="14-architecture">1.4 Architecture</h3>
          <p>Kronk is layered so applications and the model server use the same inference SDKs. The three engine paths provide different model capabilities and may have different platform support.</p>
          <p><img src="https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/02-kronk-architecture.svg" alt="Kronk architecture: application access, SDKs, bindings, native engines, and models" /></p>
          <p>Embedded applications control model loading and inference directly. The model server provides network access to those capabilities and manages model retention, memory admission, concurrent requests, and waiting requests. It also provides the BUI, security, and operational services. Supported operations differ by model and backend; some SDK operations are not exposed over HTTP.</p>
          <p>For conversational generation, Incremental Message Caching (IMC) can avoid reprocessing a compatible prompt prefix from an earlier request. Chat-completion clients still send the complete conversation; caching does not replace conversation history management. An inactive conversation does not hold an execution slot, but its cached state still consumes storage. Changes to messages, tools, templates, or media can reduce or prevent reuse. See <a href="https://www.kronkai.com/manual#chapter-4-batch-processing">Chapter 4</a> for concurrency and request handling, and <a href="https://www.kronkai.com/manual#chapter-5-message-caching">Chapter 5</a> for cache behavior and limitations.</p>
          <h3 id="15-where-to-go-next">1.5 Where to Go Next</h3>
          <ul>
            <li><strong>Install Kronk and run your first model:</strong> <a href="https://www.kronkai.com/manual#chapter-2-installation-quick-start">Chapter 2: Installation & Quick Start</a></li>
            <li><strong>Choose memory, context, and GPU settings:</strong> <a href="https://www.kronkai.com/manual#chapter-3-model-configuration">Chapter 3: Model Configuration</a></li>
            <li><strong>Connect an editor, agent, or other client:</strong> <a href="https://www.kronkai.com/manual#chapter-14-client-integration">Chapter 14: Client Integration</a></li>
            <li><strong>Run speech-to-text models:</strong> <a href="https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription">Chapter 18: Bucky</a></li>
            <li><strong>Generate or edit images:</strong> <a href="https://www.kronkai.com/manual#chapter-19-malina-image-generation">Chapter 19: Malina</a></li>
          </ul>
          <hr />
          <h2 id="chapter-2-installation-quick-start">Chapter 2: Installation &amp; Quick Start</h2>
          <h3 id="21-native-quick-start">2.1 Native Quick Start</h3>
          <p>This path installs Kronk on the host, downloads a starter model, and starts the model server. The first run requires an internet connection for native libraries and model files. These examples assume the default server settings; an existing installation keeps its saved configuration.</p>
          <p><strong>1. Install the CLI with Homebrew (macOS or Linux)</strong></p>
          <pre className="code-block"><code className="language-shell">{`brew install ardanlabs/kronk/kronk`}</code></pre>
          <p>The fully qualified formula name adds the tap and trusts only the Kronk formula. Do not run <code>brew trust ardanlabs/kronk</code> unless you intend to trust every current and future formula, cask, and external command published by the tap.</p>
          <p>Other installation methods are listed in <a href="#22-choose-an-installation-method">Section 2.2</a>.</p>
          <p><strong>2. Install the native inference libraries</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local`}</code></pre>
          <p><code>--local</code> runs this operation on the current machine without a server. Library, model, and catalog commands that support this flag otherwise use a running server by default.</p>
          <p>Kronk detects the operating system, architecture, and available GPU backend. It selects Metal, CUDA, ROCm, or Vulkan when supported and falls back to CPU.</p>
          <p><strong>3. Download a starter model</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk model pull mradermacher/Qwopus3.5-4B-Coder.Q8_0 --local`}</code></pre>
          <p>This coding model requires about 5.3 GB of disk space, including its multimodal projection. On memory-constrained or CPU-only systems, start with the smaller <code>unsloth/Qwen3-0.6B-Q8_0</code> instead, using that ID in both the pull command and the API examples below. Disk size is not the model's runtime memory requirement.</p>
          <p><strong>4. Start the server in the background</strong></p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --api-host=127.0.0.1:11435 -d`}</code></pre>
          <p>The explicit host keeps this unauthenticated quick-start server reachable only from the local machine. The API listens on <code>http://localhost:11435</code>. The Browser UI (BUI) is enabled by default at <code>http://localhost:11435/admin/</code>.</p>
          <p><strong>5. Try the model</strong></p>
          <p>Open <code>http://localhost:11435/admin/</code>, select <strong>Apps → Chat</strong>, and choose the downloaded model. The first request takes longer because Kronk must load the model into memory.</p>
          <p>You can also call the API directly:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "mradermacher/Qwopus3.5-4B-Coder.Q8_0",
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
                <td>Go 1.27.3 or later for this release</td>
              </tr>
              <tr>
                <td>Container</td>
                <td>Any host supported by the selected Linux image</td>
                <td>Docker or another OCI runtime</td>
              </tr>
              <tr>
                <td>Kubernetes / Helm</td>
                <td>Linux nodes supported by the selected image</td>
                <td>Helm, kubectl, a cluster, and a suitable StorageClass</td>
              </tr>
            </tbody>
          </table>
          <p>Regardless of installation method, model downloads require enough disk space, and inference requires enough RAM or VRAM for the selected model and configuration. Quantization, context size, KV cache type, and concurrency all affect memory use; parameter count alone is not a reliable requirement. See <a href="https://www.kronkai.com/manual#chapter-3-model-configuration">Chapter 3: Model Configuration</a>.</p>
          <h4 id="homebrew">Homebrew</h4>
          <pre className="code-block"><code className="language-shell">{`brew install ardanlabs/kronk/kronk`}</code></pre>
          <p>Homebrew automatically adds the tap and records trust for this formula. This is narrower than <code>brew trust ardanlabs/kronk</code>, which trusts every current and future item in the tap.</p>
          <p>Upgrade an existing installation with:</p>
          <pre className="code-block"><code className="language-shell">{`brew upgrade ardanlabs/kronk/kronk`}</code></pre>
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
          <p>Release images are published to GHCR and Docker Hub. Each image contains the Kronk executable, BUI, ffmpeg, and native llama.cpp and whisper.cpp libraries. Linux amd64 images also include stable-diffusion.cpp libraries for Malina; arm64 images do not. The tag identifies the llama.cpp backend, not a guarantee that every inference engine uses that backend: ROCm images use Vulkan for transcription, and CUDA images provide Vulkan or CPU for image generation.</p>
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
          <p>This command binds the service only to the local machine and persists all state in a named volume:</p>
          <pre className="code-block"><code className="language-shell">{`docker run -d \\
  --name kronk \\
  --restart unless-stopped \\
  -e KRONK_PROCESSOR=cpu \\
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
  -p 127.0.0.1:11435:11435 \\
  -v kronk-data:/kronk \\
  ghcr.io/ardanlabs/kronk:latest-cuda`}</code></pre>
          <p><strong>AMD ROCm container (Linux amd64)</strong></p>
          <p>The host needs a ROCm-compatible GPU and kernel driver, with <code>/dev/kfd</code> and <code>/dev/dri</code> available. Pass the host's numeric device-group IDs so the container's non-root user can access the GPU:</p>
          <pre className="code-block"><code className="language-shell">{`docker run -d \\
  --name kronk \\
  --restart unless-stopped \\
  --device=/dev/kfd --device=/dev/dri \\
  --group-add "$(getent group video | cut -d: -f3)" \\
  --group-add "$(getent group render | cut -d: -f3)" \\
  --security-opt seccomp=unconfined \\
  -p 127.0.0.1:11435:11435 \\
  -v kronk-data:/kronk \\
  ghcr.io/ardanlabs/kronk:latest-rocm`}</code></pre>
          <p>This ROCm example disables Docker's default seccomp filter; apply your deployment's security policy before using it on shared infrastructure.</p>
          <p><strong>Vulkan container (AMD or Intel on Linux)</strong></p>
          <p>Use a compatible host kernel driver and expose the GPU render devices:</p>
          <pre className="code-block"><code className="language-shell">{`docker run -d \\
  --name kronk \\
  --restart unless-stopped \\
  --device=/dev/dri \\
  --group-add "$(getent group video | cut -d: -f3)" \\
  --group-add "$(getent group render | cut -d: -f3)" \\
  -p 127.0.0.1:11435:11435 \\
  -v kronk-data:/kronk \\
  ghcr.io/ardanlabs/kronk:latest-vulkan`}</code></pre>
          <p>For both examples, check <code>getent group video</code> and <code>getent group render</code> first. If your distribution uses different device groups, substitute the numeric GIDs that own the GPU devices on your host. These examples expose all devices under <code>/dev/dri</code>; expose individual render nodes instead when you need to restrict device access.</p>
          <p><strong>Vulkan container (NVIDIA on Linux)</strong></p>
          <p>Use the NVIDIA Container Toolkit and request graphics capabilities in addition to compute and utility access:</p>
          <pre className="code-block"><code className="language-shell">{`docker run -d \\
  --name kronk \\
  --restart unless-stopped \\
  --runtime=nvidia --gpus all \\
  -e NVIDIA_DRIVER_CAPABILITIES=compute,utility,graphics \\
  -p 127.0.0.1:11435:11435 \\
  -v kronk-data:/kronk \\
  ghcr.io/ardanlabs/kronk:latest-vulkan`}</code></pre>
          <p>Run only one of these examples at a time: they reuse the same container name and port. Stop and remove an existing container before switching backends; keep <code>kronk-data</code>. For production authentication, resource limits, and a local Jetson build, see <a href="https://www.kronkai.com/manual#87-container-operations">8.7 Container Operations</a>.</p>
          <p><strong>Docker Compose</strong></p>
          <p>The repository includes Compose files in <a href="https://github.com/ardanlabs/kronk/tree/main/zarf/docker/kronk"><code>zarf/docker/kronk</code></a> that reuse the same <code>kronk-data</code> volume. The default file selects CPU inference and publishes the API (<code>11435</code>) and debug server (<code>11445</code>, including metrics and profiling) on localhost. Remove a <code>kronk</code> container started with <code>docker run</code> first; its models are kept in the volume:</p>
          <pre className="code-block"><code className="language-shell">{`docker rm -f kronk
git clone --depth 1 https://github.com/ardanlabs/kronk.git
cd kronk/zarf/docker/kronk
docker compose up -d`}</code></pre>
          <p>Each backend has its own self-contained file. For a GPU, use the matching one instead of <code>compose.yaml</code>:</p>
          <pre className="code-block"><code className="language-shell">{`docker compose -f compose.cuda.yaml up -d     # NVIDIA, needs the NVIDIA Container Toolkit
docker compose -f compose.rocm.yaml up -d     # AMD with ROCm, needs /dev/kfd and /dev/dri
docker compose -f compose.vulkan.yaml up -d   # AMD or Intel with Vulkan, needs /dev/dri`}</code></pre>
          <p>To use the <code>latest-all</code> image, set the GPU file's image to <code>ghcr.io/ardanlabs/kronk:latest-all</code>, retaining its device access and processor settings.</p>
          <p>Settings, server options such as authentication, GPU details, and upgrades are covered in <a href="https://www.kronkai.com/manual#87-container-operations">8.7 Container Operations</a>.</p>
          <p>The container runs as UID/GID <code>10001</code>. A named volume needs no preparation. If you use a host directory such as <code>/srv/kronk</code>, make it writable by that user before starting the container:</p>
          <pre className="code-block"><code className="language-shell">{`sudo mkdir -p /srv/kronk
sudo chown -R 10001:10001 /srv/kronk`}</code></pre>
          <p>For pinned releases, updates, and remote operation, see <a href="https://www.kronkai.com/manual#87-container-operations">Chapter 8: Model Server</a>.</p>
          <p><strong>Kubernetes with Helm</strong></p>
          <p>From a repository checkout, with your intended Kubernetes context selected:</p>
          <pre className="code-block"><code className="language-shell">{`helm upgrade --install kronk zarf/helm/kronk -n kronk --create-namespace \\
  -f zarf/helm/kronk/examples/cpu.yaml --wait --timeout 20m`}</code></pre>
          <p>This installs one StatefulSet replica, a retained <code>50Gi</code> RWO data claim, and the small <code>unsloth/Qwen3-0.6B-Q8_0</code> starter model. A default StorageClass is required unless you select another class or an existing claim. Unlike the local container quick start, the chart defaults to authenticated access and disables the BUI. See <a href="https://www.kronkai.com/manual#810-kubernetes-with-helm">8.10 Kubernetes with Helm</a> for installation and essential operating warnings, and the <a href="https://github.com/ardanlabs/kronk/blob/main/zarf/helm/kronk/README.md">chart README</a> for configuration, the first chat request, and troubleshooting. Installation is from the local chart only. GPU examples require validation on real nodes with compatible drivers and runtimes.</p>
          <h3 id="24-libraries">2.4 Libraries</h3>
          <p>Kronk needs a native llama.cpp library bundle before it can run GGUF models. By default, the server checks the selected bundle during startup and installs it when needed. To install it explicitly before starting the server, run:</p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local`}</code></pre>
          <p>The server also installs and initializes Bucky and Malina libraries by default where supported. For a server that does not need transcription or image generation, <code>--media-backends-enabled=false</code> skips those backend downloads and initialization. This does not disable vision or audio input supported by a llama.cpp model.</p>
          <h4 id="selecting-a-library-bundle">Selecting a library bundle</h4>
          <p>The default location is:</p>
          <pre className="code-block"><code className="language-text">{`~/.kronk/libraries/<os>/<arch>/<processor>/`}</code></pre>
          <p>Examples include <code>darwin/arm64/metal</code> and <code>linux/amd64/cuda</code>. Multiple bundles can coexist. During automatic selection, Kronk first resolves the preferred processor for the host. A CUDA 13 bundle requires every visible NVIDIA GPU to have compute capability 7.5 or newer. If the visible devices are older, or <code>CUDA_VISIBLE_DEVICES</code> leaves no CUDA device visible, Kronk selects a supported ROCm or Vulkan host runtime when one is available and otherwise selects CPU. An inconclusive NVIDIA probe retains CUDA rather than silently changing the backend.</p>
          <p>After installation, Kronk probes the preferred accelerator bundle before loading it. If that bundle positively reports no accelerator and another installed bundle for the same llama.cpp version positively reports a device, Kronk selects the working installed bundle. This second check does not download another bundle. Startup logs report the preferred processor, selected processor, and reason for either retaining or changing it.</p>
          <p><code>KRONK_ARCH</code> and <code>KRONK_OS</code> select the bundle's target architecture and operating system. Set <code>KRONK_PROCESSOR</code> when automatic backend selection is not appropriate, such as deliberately running CPU inference on a GPU host. An explicit processor or library path disables the server's automatic runtime substitution; setting only the architecture or operating system does not.</p>
          <p>List the supported and installed bundles without a running server:</p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local --list-combinations
kronk libs --local --list-installs`}</code></pre>
          <p>To deliberately select CPU inference, apply the setting to both installation and server startup; setting it for one command does not persist it:</p>
          <pre className="code-block"><code className="language-shell">{`KRONK_PROCESSOR=cpu kronk libs --local
KRONK_PROCESSOR=cpu kronk server start`}</code></pre>
          <p>For server startup, <code>KRONK_LIB_PATH</code> or <code>--lib-path</code> can name an existing versioned Kronk bundle or a user-managed library directory. A non-empty user-managed directory without <code>version.json</code> is treated as read-only: Kronk loads from it but does not install, upgrade, or replace its contents. Changing the selected processor or library path requires a process restart; native libraries are not hot-reloaded.</p>
          <p>Linux CUDA bundles depend on CUDA runtime libraries supplied by the host. A working <code>nvidia-smi</code> confirms the driver but not necessarily the CUDA runtime. If the CUDA backend does not load, inspect <code>libggml-cuda.so</code> with <code>ldd</code> and install the runtime packages appropriate for the CUDA version used by the current bundle. The CUDA container image already includes its matching runtime.</p>
          <h4 id="versions-and-upgrades">Versions and upgrades</h4>
          <p>On a fresh installation, the normal command installs the llama.cpp build pinned for the installed Kronk release and verifies the download against its pinned release manifest. A newer installed build is retained rather than automatically downgraded; use <code>--version</code> to select a particular release. Once verification metadata is cached, subsequent verification can run offline. Older installations without that metadata need an online verification first.</p>
          <p>Prefer the pinned build. <code>--upgrade</code> opts into the latest published llama.cpp build, which may be incompatible with the installed Kronk release:</p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --local --upgrade`}</code></pre>
          <p>Use <code>kronk libs --help</code> for explicit version selection, cross-platform bundle installation, and removal. The compatibility matrix is kept in the repository <a href="https://github.com/ardanlabs/kronk#project-status">README</a>. Native backend environment settings and their restart requirements are covered in <a href="https://www.kronkai.com/manual#native-backend-environment-settings">Chapter 3</a>.</p>
          <h4 id="openvino-preview-bundles">OpenVINO preview bundles</h4>
          <p>Kronk can download llama.cpp OpenVINO bundles for Linux amd64 and Windows amd64. Kronk supports these bundles only on Intel systems. They can be tried to improve inference performance by targeting an Intel CPU, integrated or discrete Intel GPU, or Intel NPU. They do not provide acceleration for AMD or NVIDIA GPUs; use ROCm or Vulkan for AMD GPUs and CUDA for NVIDIA GPUs.</p>
          <p>OpenVINO remains an explicit, preview backend. It is not selected automatically, and performance and compatibility depend on the model, quantization, workload, and target device. Benchmark against Kronk's standard <code>cpu</code> backend when using an Intel CPU, and validate complete serving workloads on the target system before production use. Successfully loading one model is not confirmation that every model or serving feature is supported.</p>
          <p>Two environment variables select the runtime:</p>
          <ul>
            <li><code>KRONK_PROCESSOR=openvino</code> selects the OpenVINO llama.cpp bundle.</li>
            <li><code>GGML_OPENVINO_DEVICE=CPU|GPU|GPU.&lt;index&gt;|NPU</code> selects the device used by OpenVINO within the Intel family. OpenVINO defaults to <code>CPU</code> when this variable is unset, but setting it explicitly makes the intended target clear.</li>
          </ul>
          <p>For example:</p>
          <pre className="code-block"><code className="language-shell">{`# Install the Linux amd64 bundle without making it the automatic default.
kronk libs --local --install \\
  --arch=amd64 \\
  --os=linux \\
  --processor=openvino

# Select the bundle and an Intel GPU for one server process.
KRONK_PROCESSOR=openvino \\
GGML_OPENVINO_DEVICE=GPU \\
kronk server start`}</code></pre>
          <p>The OpenVINO runtime reads <code>GGML_OPENVINO_DEVICE</code> once during initialization; restart Kronk after changing it. GPU and NPU targets require the corresponding Intel host drivers and device permissions. If a requested device is not available, upstream OpenVINO can fall back to CPU, so validate the installation with the opt-in probe from a Kronk source checkout:</p>
          <pre className="code-block"><code className="language-shell">{`make test-openvino                         # CPU
make test-openvino OPENVINO_DEVICE=GPU
make test-openvino OPENVINO_DEVICE=GPU.0
make test-openvino OPENVINO_DEVICE=NPU`}</code></pre>
          <p>The probe requires a source checkout and Linux/Windows amd64. It downloads the pinned bundle and a small diagnostic model, tests inference, and fails if OpenVINO reports a fallback to CPU. Passing it is a smoke test, not validation of every model or serving feature.</p>
          <h4 id="bucky-and-malina-libraries">Bucky and Malina libraries</h4>
          <p>Speech-to-text and image generation use separate bundles. To install them explicitly without a running server:</p>
          <pre className="code-block"><code className="language-shell">{`kronk bucky libs --local
kronk malina libs --local`}</code></pre>
          <p>Platform and backend support differ from llama.cpp. See <a href="https://www.kronkai.com/manual#182-install-whisper-libraries">Chapter 18: Bucky</a> and <a href="https://www.kronkai.com/manual#192-install-stable-diffusion-libraries">Chapter 19: Malina</a> for supported bundles, models, and configuration.</p>
          <h3 id="25-models-and-data-paths">2.5 Models and Data Paths</h3>
          <p>List the starter catalog and download a model directly on the host:</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list --local
kronk model pull unsloth/Qwen3-0.6B-Q8_0 --local`}</code></pre>
          <p>Model sources may be canonical <code>provider/model</code> IDs, Hugging Face URLs, or repository-and-quantization shorthands. Run <code>kronk model pull --help</code> for all accepted forms. To pin both the repository and file, use <code>owner/repo/file.gguf</code> or a Hugging Face <code>blob</code> or <code>resolve</code> URL. Kronk then resolves that exact file only within that repository.</p>
          <p>The default data layout is:</p>
          <pre className="code-block"><code className="language-text">{`~/.kronk/
├── catalog/
│   └── catalog.yaml
├── libraries/
│   └── <os>/<arch>/<processor>/
├── lora/
│   └── [<organization>/]<adapter>.gguf
├── models/
│   ├── model_config.yaml
│   └── <provider>/<family>/<model files>
├── bucky-libraries/
├── bucky-models/
├── malina-libraries/
├── malina-models/
└── keys/`}</code></pre>
          <p>Set <code>KRONK_BASE_PATH</code> or the global <code>--base-path</code> flag to select a different data root. This does not move existing files; migrate them separately when reusing an installation. Use the same root for local management commands and the server. Official containers set it to <code>/kronk</code>.</p>
          <p>The <code>lora</code> directory holds optional, user-provided LoRA adapter GGUF files. Kronk does not download these files. See <a href="https://www.kronkai.com/manual#lora-adapters">Chapter 3 §3.7</a> for placement and configuration.</p>
          <p><code>models/model_config.yaml</code> contains server settings and overrides for language, Bucky, and Malina models, including language-model variants. Kronk creates it when absent and preserves existing edits. Restart the server after changing it. Do not copy configuration values based only on model size; use <a href="https://www.kronkai.com/manual#chapter-3-model-configuration">Chapter 3</a> for context, cache, GPU, and sampling settings.</p>
          <h3 id="26-running-the-server">2.6 Running the Server</h3>
          <p>Start in the foreground:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --api-host=127.0.0.1:11435`}</code></pre>
          <p>Stop a foreground server with Ctrl-C. To run in the background instead:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --api-host=127.0.0.1:11435 -d`}</code></pre>
          <p>The background process writes logs to <code>~/.kronk/kronk.log</code>. Manage it with:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server logs
kronk server stop`}</code></pre>
          <p>If you selected another data root, use the same <code>--base-path</code> or <code>KRONK_BASE_PATH</code> for logs and stop commands. A background launch can return while startup is still in progress; follow the logs and verify the API before sending inference requests.</p>
          <p>The server defaults to <code>127.0.0.1:11435</code> for local-only access. The API and BUI share port <code>11435</code>; the BUI is enabled by default and is served under <code>/admin/</code>. Disable it for a headless server with:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start \\
  --api-host=127.0.0.1:11435 \\
  --web-admin-enabled=false`}</code></pre>
          <p>Peer downloads are separately controlled by <code>KRONK_DOWNLOAD_ENABLED</code>, which defaults to <code>false</code>. It shares this server's models and library bundles with other Kronk servers on the network.</p>
          <p>See <a href="https://www.kronkai.com/manual#chapter-8-model-server">Chapter 8</a> for server flags, model pooling, runtime paths, and deployment operations.</p>
          <h4 id="run-as-a-linux-service-systemd">Run as a Linux Service (systemd)</h4>
          <p>With Kronk installed in a permanent location, run these as your normal user:</p>
          <pre className="code-block"><code className="language-shell">{`kronk systemd install
kronk systemd start
kronk systemd status`}</code></pre>
          <p>The user service uses your existing <code>~/.kronk</code> by default and enables startup at login. Use <code>kronk systemd install --boot</code> instead to enable startup before login and operation after logout (may require authorization). Stop any manually started server before starting the service. Use <code>kronk systemd stop</code> and <code>kronk systemd logs</code> for this process, not the detached-server commands. See <a href="https://www.kronkai.com/manual#88-running-as-a-systemd-service">Chapter 8 §8.8</a> for configuration, upgrades, and removal.</p>
          <h4 id="run-as-a-macos-agent-launchd">Run as a macOS Agent (launchd)</h4>
          <p>With Kronk installed in a permanent location, run these as your normal user:</p>
          <pre className="code-block"><code className="language-shell">{`kronk launchd install
kronk launchd start
kronk launchd status`}</code></pre>
          <p>The LaunchAgent uses your existing <code>~/.kronk</code> by default and enables startup at desktop login. Stop any manually started server before starting the agent. Use <code>kronk launchd stop</code> and <code>kronk launchd logs</code> to manage it. This is a per-user agent, not a system-wide service or a before-login boot service. See <a href="https://www.kronkai.com/manual#89-running-as-a-launchd-agent">Chapter 8 §8.9</a> for configuration, upgrades, logs, and removal.</p>
          <h3 id="27-verify-the-installation">2.7 Verify the Installation</h3>
          <p>Check server liveness:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/liveness`}</code></pre>
          <p>List models visible to the server:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/models`}</code></pre>
          <p>Test the starter model from Section 2.1, or substitute the ID of another downloaded chat model:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "mradermacher/Qwopus3.5-4B-Coder.Q8_0",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 100
  }'`}</code></pre>
          <p>If authentication is enabled, add <code>-H "Authorization: Bearer $KRONK_TOKEN"</code> to protected API requests, using a token with the required permissions. Health endpoints remain public. A successful health check confirms the API is responding, not that a particular model can load and generate; the chat request checks that separately.</p>
          <h3 id="28-security-and-next-steps">2.8 Security and Next Steps</h3>
          <p>The native server defaults to open API access. Do not publish port <code>11435</code> to an untrusted network until authentication and TLS termination are configured. The BUI, management endpoints, and inference endpoints have separate security controls; see <a href="https://www.kronkai.com/manual#chapter-12-security-and-authentication">Chapter 12: Security & Authentication</a> before operating a shared or internet-reachable server.</p>
          <p>Continue with:</p>
          <ul>
            <li><a href="https://www.kronkai.com/manual#chapter-3-model-configuration">Chapter 3: Model Configuration</a> for memory, context, GPU, cache, and sampling settings.</li>
            <li><a href="https://www.kronkai.com/manual#chapter-8-model-server">Chapter 8: Model Server</a> for server operation and deployment.</li>
            <li><a href="https://www.kronkai.com/manual#chapter-12-security-and-authentication">Chapter 12: Security & Authentication</a> before exposing the service.</li>
            <li><a href="https://www.kronkai.com/manual#chapter-14-client-integration">Chapter 14: Client Integration</a> to connect an editor or coding agent.</li>
            <li><a href="https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription">Chapter 18: Bucky</a> for speech-to-text.</li>
            <li><a href="https://www.kronkai.com/manual#chapter-19-malina-image-generation">Chapter 19: Malina</a> for image generation.</li>
          </ul>
          <h3 id="29-cli-reference-and-shell-completion">2.9 CLI Reference and Shell Completion</h3>
          <p>The BUI's <strong>Docs → CLI</strong> reference and <code>kronk --help</code> list commands, flags, declared defaults, and examples. Use this manual for workflows, environment settings, and service configuration. Defaults shown in help are not necessarily the effective server settings: saved configuration and environment variables can supply values for flags you do not set. Chapter 8 describes server defaults and configuration precedence.</p>
          <p>Shell completion is available for Bash, Fish, PowerShell, and Zsh. For the current session:</p>
          <pre className="code-block"><code className="language-shell">{`# Bash
source <(kronk completion bash)

# Fish
kronk completion fish | source

# PowerShell
kronk completion powershell | Out-String | Invoke-Expression

# Zsh (initialize compinit first if your shell has not already done so)
autoload -Uz compinit && compinit
source <(kronk completion zsh)`}</code></pre>
          <p>For future sessions, save Fish output to <code>~/.config/fish/completions/kronk.fish</code>, or add the PowerShell command to your profile. Save Bash output to your installed bash-completion directory (<code>/etc/bash_completion.d/kronk</code> on many Linux systems, or <code>$(brew --prefix)/etc/bash_completion.d/kronk</code> with Homebrew). For Zsh, save the output as <code>_kronk</code> in a directory on <code>$fpath</code>, then initialize completion in your shell startup file. Create the destination directory if needed and use a location your account can write to. <code>kronk completion &lt;shell&gt; --help</code> describes shell-specific options such as omitting descriptions.</p>
          <hr />
          <h2 id="chapter-3-model-configuration">Chapter 3: Model Configuration</h2>
          <p>Kronk analyzes each model and the available hardware before loading it. Most models run well without manual tuning. Use per-model configuration when you need a different context window, more concurrent requests, explicit device placement, or an advanced feature such as speculative decoding.</p>
          <p>This chapter documents model runtime configuration. Server settings such as the listen address, authentication, and the number of models kept in the pool are covered in <a href="https://www.kronkai.com/manual#chapter-8-model-server">Chapter 8</a>.</p>
          <h3 id="31-configuration-file">3.1 Configuration File</h3>
          <p>The model server reads per-model overrides from:</p>
          <pre className="code-block"><code className="language-text">{`~/.kronk/models/model_config.yaml`}</code></pre>
          <p>Kronk creates this file on first use. Version 2 stores language-model overrides under <code>models</code>, Bucky overrides under <code>bucky-models</code>, and Malina overrides under <code>malina-models</code>. Version 1 files containing only <code>kms</code> and <code>models</code> remain supported. Language-model entries use the canonical ID shown by <code>kronk model list</code> or the <code>/v1/models</code> endpoint:</p>
          <pre className="code-block"><code className="language-yaml">{`version: 2
models:
  unsloth/Qwen3-0.6B-Q8_0:
    context-window: 32768
    nseq-max: 2
    admission-timeout: 3m
    queue-depth: 2
    imc-session-capacity: 8

bucky-models:
  tiny:
    nseq-max: 2
    queue-depth: 4

malina-models:
  sd-1.5:
    concurrency: 2
    queue-depth: 2`}</code></pre>
          <p>Files must declare <code>version: 1</code> or <code>version: 2</code>; missing or unsupported versions prevent server startup. Migrate legacy files with top-level model IDs into the <code>models:</code> mapping. The remaining examples in this chapter show individual entries as they appear below <code>models:</code>. Model setting names use kebab-case, such as <code>context-window</code> and <code>nseq-max</code>. Keys nested under <code>sampling-parameters</code> use the API's snake_case names, such as <code>top_p</code>.</p>
          <p>The server reads this file during startup. Restart the server after changing it. To test a different file without replacing the default, run:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --model-config-file=./my-model-config.yaml`}</code></pre>
          <p>You can also set <code>KRONK_POOL_MODEL_CONFIG_FILE</code> to an alternative path. See <a href="https://www.kronkai.com/manual#85-model-configuration-files">Chapter 8 §8.5</a> for model config file management and <a href="https://www.kronkai.com/manual#25-models-and-data-paths">Chapter 2 §2.5</a> for all data paths.</p>
          <p>Inference requests require the canonical <code>provider/modelID</code> shown by <code>/v1/models</code>. Bare model IDs are rejected rather than searched across a list of providers. Bucky entries use the short ID shown by <code>kronk bucky model list</code>, and Malina entries use the bundle ID shown by <code>kronk malina model list</code>.</p>
          <h4 id="model-variants">Model variants</h4>
          <p>A suffix creates another configuration for the same downloaded model:</p>
          <pre className="code-block"><code className="language-yaml">{`models:
  unsloth/Qwen3-0.6B-Q8_0:
    context-window: 32768

  unsloth/Qwen3-0.6B-Q8_0/LONG:
    context-window: 65536`}</code></pre>
          <p>Select the variant by sending the complete name, including <code>/LONG</code>, as the API request's <code>model</code> value. Variants let applications use different runtime settings without keeping duplicate model files.</p>
          <h4 id="other-configuration-surfaces">Other configuration surfaces</h4>
          <p>Applications embedding the SDK can configure model loading directly without using the server's YAML file. Request fields such as <code>temperature</code>, <code>top_p</code>, <code>seed</code>, and <code>max_tokens</code> can override generation behavior for an individual request. A configured seed supplies a repeatable sampling default; an explicit request seed takes precedence. Without either, Kronk generates a random seed, which a matched IMC session can retain across turns. A fixed seed does not guarantee identical output across different backends or execution conditions. Those request fields are documented in <a href="https://www.kronkai.com/manual#chapter-10-request-parameters">Chapter 10</a>.</p>
          <p>The hardware processor (such as <code>cpu</code>, <code>metal</code>, <code>cuda</code>, <code>rocm</code>, or <code>vulkan</code>) selects a native library bundle rather than a per-model setting. Kronk normally detects it automatically. To override detection, apply <code>KRONK_PROCESSOR</code> to both library installation and server startup; a one-command setting does not persist. See <a href="https://www.kronkai.com/manual#24-libraries">Chapter 2 §2.4</a>.</p>
          <h4 id="311-chat-templates-define-the-model-protocol">3.1.1 Chat Templates Define the Model Protocol</h4>
          <p>A chat template is not cosmetic string formatting. It is executable, model-specific protocol logic that converts portable request data—messages, roles, tools, optional media, and supported reasoning controls—into the exact prompt syntax the model learned during training. The model never receives the original message objects.</p>
          <p>The correct template is therefore part of model compatibility, much like the tokenizer. A mismatched template can use the wrong role markers, serialize tool definitions incorrectly, omit media placeholders, or fail to append the cue that tells the model to begin an assistant response. These failures can be subtle: the model may still generate fluent text while instruction following, tool calling, reasoning, or multimodal behavior degrades.</p>
          <p>For normal server model loads, Kronk resolves the template in this order:</p>
          <ol>
            <li>The explicit <code>template</code> path in <code>model_config.yaml</code>.</li>
            <li>A matching Jinja file under the data root's <code>jinja</code> directory, first matching the model name including quantization, then the name without its quantization suffix.</li>
            <li>The <code>tokenizer.chat_template</code> embedded in GGUF metadata.</li>
          </ol>
          <p>Relative <code>template</code> paths resolve under the Kronk data root, not beside the configuration file. An unreadable or empty explicit template prevents model loading. A custom in-memory template supplied by the Playground or an embedded application takes precedence over these file-based choices.</p>
          <p>Prefer the model's supplied template unless you are correcting known-bad metadata or deliberately testing a custom protocol. A template override must remain compatible with the model and with any tool-call output parser. Sampling controls such as <code>temperature</code>, <code>top_p</code>, penalties, and <code>max_tokens</code> do not become prompt text; they govern token selection and stopping after the template has rendered.</p>
          <p><a href="https://www.kronkai.com/manual#45-stage-2-—-prepare-model-work">Chapter 4 §4.5</a> shows this template protocol as part of Stage 2 request preparation, where the portable request becomes the exact prompt that Kronk tokenizes and executes. <a href="https://www.kronkai.com/manual#chapter-5-message-caching">Chapter 5</a> explains why IMC renders the complete conversation both with and without the generation suffix. <a href="https://www.kronkai.com/manual#chapter-9-api-endpoints">Chapter 9</a> documents the portable API message formats that templates consume.</p>
          <h3 id="32-automatic-tuning">3.2 Automatic Tuning</h3>
          <p>The Kronk Model Server runs automatic tuning by default for every model it loads. No <code>auto-tune</code> key is required in <code>model_config.yaml</code>. The server derives a starting configuration from GGUF metadata and the available hardware. This analysis chooses values such as:</p>
          <ul>
            <li>context window;</li>
            <li>KV cache types;</li>
            <li>maximum parallel sequences;</li>
            <li>GPU layer placement;</li>
            <li>Flash Attention mode; and</li>
            <li>multi-GPU split mode.</li>
          </ul>
          <p>Automatic tuning does not select the model weight loading mode. The default <code>auto</code> mode uses mmap when every selected device supports it and otherwise uses ordinary loading. Storage, filesystem support, page-cache reuse, memory-lock limits, and workload lifetime still determine whether an explicit mode is appropriate.</p>
          <p>A concrete override in <code>model_config.yaml</code> constrains the analysis. Explicit <code>context-window</code> and <code>nseq-max</code> values are fixed: automatic tuning includes them in its memory calculation and never silently reduces them. The special cache type <code>auto</code> is treated as unset and therefore does not clear an analyzed <code>f16</code> or <code>q8_0</code> choice. This makes the usual workflow:</p>
          <ol>
            <li>Start with no override and let Kronk analyze the model.</li>
            <li>Use the model normally and monitor memory and latency.</li>
            <li>Override only the setting needed for the workload.</li>
          </ol>
          <p>When <code>context-window</code> and <code>nseq-max</code> are specified but the KV cache types are omitted, automatic tuning uses this priority:</p>
          <ol>
            <li>Preserve the configured context window and concurrency.</li>
            <li>Try <code>f16</code> for both KV caches.</li>
            <li>If <code>f16</code> does not fit, try <code>q8_0</code> at the same context and concurrency.</li>
            <li>If <code>q8_0</code> does not fit, keep the configured values and report that the recommendation does not fit. Automatic tuning does not select a cache type below <code>q8_0</code> and does not reduce the configured context or concurrency.</li>
          </ol>
          <p>This is the recommended server configuration style:</p>
          <pre className="code-block"><code className="language-yaml">{`ornith-ai/Ornith-1.5-35B-Q8_0/AGENT:
  context-window: 262144
  nseq-max: 2`}</code></pre>
          <p>Leave <code>cache-type-k</code> and <code>cache-type-v</code> out unless the workload requires a specific cache format. In this example, Kronk sizes the exact 262144-token, two-sequence configuration with <code>f16</code> first and then <code>q8_0</code> if necessary.</p>
          <p>When context and concurrency are not specified, the balanced analysis limits the selected context to the model's training context and a maximum of 128K tokens. It searches supported context buckets from largest to smallest. At each context it tries <code>f16</code> first and then <code>q8_0</code> before considering a smaller context. The usual minimum candidate is 4K, but a smaller training context is not expanded to 4K. Fit checks use both VRAM and host RAM where applicable; on unified-memory systems, they use the combined footprint. CPU-only analysis can check RAM when a budget is known. Without a known budget, the corresponding memory limit cannot be checked. All recommendations are estimates, not a guarantee that every backend and workload will fit or have identical memory use.</p>
          <p>Automatic tuning estimates a configuration; it does not guarantee a successful load. A configuration that cannot fit the server's memory budget can be rejected before loading. AutoTune does not automatically turn an oversized full-GPU configuration into a CPU/GPU split: configure partial layer or expert placement explicitly when needed.</p>
          <p>Embedded applications must opt into automatic tuning. Explicit application settings constrain the recommendation in the same way as server overrides.</p>
          <h3 id="33-core-runtime-settings">3.3 Core Runtime Settings</h3>
          <h4 id="context-window">Context window</h4>
          <p><code>context-window</code> is the maximum number of tokens available to one sequence. Input and generated tokens both consume this capacity.</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/Qwen3-0.6B-Q8_0:
  context-window: 32768`}</code></pre>
          <p>A larger window increases KV-cache memory and can reduce the number of parallel sequences that fit. It also cannot create model capability that was absent during training. If the requested window exceeds the model's native context, the model may require RoPE scaling; see <a href="https://www.kronkai.com/manual#chapter-7-yarn-extended-context">Chapter 7</a>.</p>
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
          <h4 id="kv-cache-layout">KV Cache Layout</h4>
          <p><code>kv-cache-mode</code> selects how concurrent generation slots allocate KV storage:</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/Qwen3-0.6B-Q8_0:
  context-window: 8192
  nseq-max: 4
  kv-cache-mode: shared`}</code></pre>
          <p>The default, <code>partitioned</code>, gives each slot a dedicated context-window-sized partition. <code>shared</code> uses one aggregate pool. Both request total context capacity of <code>context-window × nseq-max</code>, isolate attention between requests, and keep Kronk's per-request context limit unchanged. Shared mode does not reserve a physical partition for each slot.</p>
          <p>Shared mode can improve ordinary generation batching when active sequence IDs have gaps. It can also increase attention workspace and long-history processing costs. Benchmark your workload rather than assuming it is always faster. This is a model-load setting, not a chat-request parameter; restart/reload with the new configuration.</p>
          <h4 id="flash-attention">Flash Attention</h4>
          <p>Flash Attention can reduce attention memory traffic and improve performance, especially at longer contexts:</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/Qwen3-0.6B-Q8_0:
  flash-attention: auto`}</code></pre>
          <p>Valid values are <code>enabled</code>, <code>disabled</code>, and <code>auto</code>. Automatic tuning uses <code>auto</code> when a GPU is available and disables it for CPU-only analysis. Set an explicit value only for backend compatibility or controlled benchmarking.</p>
          <p>Quantized value caches such as <code>cache-type-v: q8_0</code> or <code>q4_0</code> require <code>flash-attention: auto</code> or <code>enabled</code>; combining them with <code>disabled</code> fails configuration validation. Backend and model support must still be verified.</p>
          <h4 id="sliding-window-attention">Sliding Window Attention</h4>
          <p>Kronk reads the sliding-window size from model metadata. <code>swa-full</code> controls the cache allocation used by models with sliding window attention:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/some-swa-model:
  swa-full: false`}</code></pre>
          <p>When unset, Kronk leaves the choice to llama.cpp. Explicitly setting <code>false</code> uses the compact sliding-window cache, which can save memory but limits context caching and shifting. Setting <code>true</code> requests the full cache at a higher memory cost. This key has no effect on models without SWA metadata.</p>
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
          <h4 id="moe-expert-placement">MoE expert placement</h4>
          <p>For MoE models, <code>moe.mode</code> controls routed expert placement separately from ordinary layer offload:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Value</th>
                <th>Behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>auto</code></td>
                <td>Experts follow normal layer placement; no extra CPU override</td>
              </tr>
              <tr>
                <td><code>experts_cpu</code></td>
                <td>Keep routed expert weights on the CPU while other weights follow layer placement</td>
              </tr>
              <tr>
                <td><code>experts_gpu</code></td>
                <td>No extra CPU override; experts follow layer placement, so full GPU offload is still needed to put all experts on GPU</td>
              </tr>
              <tr>
                <td><code>keep_top_n</code></td>
                <td>Keep routed experts eligible for GPU placement in the highest-numbered N layers; force the remaining expert layers to CPU</td>
              </tr>
              <tr>
                <td><code>custom</code></td>
                <td>Use explicit <code>tensor-buft-overrides</code> for placement</td>
              </tr>
            </tbody>
          </table>
          <p>For example, on a discrete-GPU system with enough host RAM, this keeps the non-expert model weights eligible for GPU offload while placing routed experts in host memory:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/large-moe-model:
  ngpu-layers: 0
  moe:
    mode: experts_cpu`}</code></pre>
          <p>To retain some expert layers on GPU, use <code>mode: keep_top_n</code> with <code>keep-experts-top-n: 4</code>, adjusting the count to fit the available VRAM. Here “top” means the highest-numbered layers, not the most frequently selected experts. An explicit <code>tensor-buft-overrides</code> list takes precedence over <code>moe.mode</code>. These settings redistribute weights; they do not reduce model size or create additional capacity on unified-memory systems. Measure throughput because CPU expert evaluation can become the bottleneck. The optional GPU expert cache is described in Section 3.6.</p>
          <h4 id="model-weight-loading">Model weight loading</h4>
          <p><code>load-mode</code> controls how Kronk reads model weights. Its default is <code>auto</code>:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/some-model:
  load-mode: auto`}</code></pre>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Value</th>
                <th>Behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>auto</code></td>
                <td>Use mmap when every selected device supports it; otherwise use ordinary loading; this is the default</td>
              </tr>
              <tr>
                <td><code>mmap</code></td>
                <td>Force memory-mapped model weights and use the operating system page cache</td>
              </tr>
              <tr>
                <td><code>none</code></td>
                <td>Load weights without mmap, mlock, or direct I/O</td>
              </tr>
              <tr>
                <td><code>mlock</code></td>
                <td>Load weights and request that their pages remain resident in RAM without forcing mmap</td>
              </tr>
              <tr>
                <td><code>mmap+mlock</code></td>
                <td>Memory-map weights and request that their pages remain resident in RAM</td>
              </tr>
              <tr>
                <td><code>direct-io</code></td>
                <td>Bypass the operating system page cache where the platform and filesystem support it</td>
              </tr>
            </tbody>
          </table>
          <p>Use <code>none</code> when mmap is unsuitable or when direct allocation is needed for a measured NUMA placement issue. Use <code>mlock</code> only when the host has enough physical and lockable RAM for the model plus the rest of the workload; process resource limits may prevent all pages from being locked. Use <code>mmap+mlock</code> when the model should be both memory mapped and kept resident. Direct I/O can avoid page-cache pollution for some local-storage and GPU-loading workloads, but it can also make repeated loads slower and is not supported by every filesystem. Benchmark the actual model path before selecting it.</p>
          <p>Kronk applies one load mode to the target model and any separate draft model.</p>
          <p>The former <code>use-mmap</code> and <code>use-direct-io</code> keys are no longer supported. Migrate existing configuration as follows:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Removed configuration</th>
                <th>Replacement</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>use-mmap: true</code></td>
                <td><code>load-mode: mmap</code></td>
              </tr>
              <tr>
                <td><code>use-mmap: false</code></td>
                <td><code>load-mode: none</code></td>
              </tr>
              <tr>
                <td><code>use-direct-io: true</code></td>
                <td><code>load-mode: direct-io</code></td>
              </tr>
            </tbody>
          </table>
          <h4 id="lazy-tensor-reading">Lazy tensor reading</h4>
          <p><code>lazy-mode</code> controls on-demand reading of tensors that llama.cpp marks eligible, such as the per-layer token embedding tables in Qwen4Exp and Gemma4. It is not whole-model lazy loading; models without eligible tensors are unaffected.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Value</th>
                <th>Behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>auto</code></td>
                <td>Lazy-read eligible tensors larger than 4 GiB when all selected devices support mmap; this is the default</td>
              </tr>
              <tr>
                <td><code>off</code></td>
                <td>Read eligible tensors up front like ordinary weights</td>
              </tr>
              <tr>
                <td><code>on</code></td>
                <td>Request lazy reading regardless of tensor size; if platform mmap is unavailable, load fully into RAM and log a warning</td>
              </tr>
            </tbody>
          </table>
          <pre className="code-block"><code className="language-yaml">{`some-provider/some-model:
  lazy-mode: on
  load-mode: none`}</code></pre>
          <p>On Metal, mmap can bring the entire table into RAM through an overlapping GPU mapping even when the log says <code>lazy read enabled</code>. Use <code>load-mode: none</code> to avoid that overlap unless the table is isolated in its own shard. Eligible lazy tensors remain file-mapped even with <code>load-mode: none</code>; the other weights use ordinary loading. Accessed pages may remain cached, and disk reads can reduce inference speed. Verify residency and performance with the actual model.</p>
          <p>The setting applies to the target and any separate or companion draft model. For eligible lazy tables, Kronk's memory estimates and admission reservations use an estimated host-memory working set instead of the full table size.</p>
          <p>Full-table accounting remains when lazy reading is off or uncertain, when <code>auto</code> does not meet its size/device requirements, with mlock, or when Metal mmap may overlap a table that is not alone in its shard. The current device capability check verifies CPU and Metal; other backends conservatively retain the full table in <code>auto</code> mode. Explicit <code>on</code> bypasses the size and device checks, but cannot enable lazy reading on a platform without mmap support. This is not a hard residency cap: clean mapped pages may accumulate or be reclaimed by the operating system. Validate actual memory and latency under representative prompts before relying on the additional admission headroom.</p>
          <h4 id="kv-cache-and-operations">KV cache and operations</h4>
          <p>The KV cache and host tensor operations are offloaded to the GPU by default:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/some-model:
  offload-kqv: false
  op-offload: false`}</code></pre>
          <p><code>offload-kqv: false</code> keeps the KV cache on the CPU. <code>op-offload: false</code> keeps host tensor operations on the CPU. These options can reduce discrete-GPU VRAM pressure at a performance cost. They do not reduce total memory requirements.</p>
          <p><code>GGML_OP_OFFLOAD_MIN_BATCH</code> sets the minimum batch size for offloading supported host operations on applicable GPU backends; the default is 32. It is process-wide, not a per-model setting. Set it before server startup and restart after changing it.</p>
          <p>For multimodal models, <code>proj-on-cpu: true</code> keeps the media projector on the CPU without changing placement of the language model itself. To place the projector on a specific accelerator, set <code>proj-device</code> to a name reported by <code>kronk devices</code> (for example, <code>CUDA1</code> or <code>MTL0</code>). Omit both settings for automatic placement. <code>proj-device</code> cannot be combined with <code>proj-on-cpu: true</code>.</p>
          <h4 id="native-backend-environment-settings">Native backend environment settings</h4>
          <p>These optional llama.cpp settings are process-wide environment variables, not per-model YAML fields. They affect the CUDA backend and applicable ROCm/HIP paths. Leave them unset unless you need to override backend memory behavior.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Variable</th>
                <th>Effect when set to <code>1</code></th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>GGML_CUDA_ENABLE_UNIFIED_MEMORY</code></td>
                <td>Enable managed/unified memory allocation</td>
              </tr>
              <tr>
                <td><code>GGML_CUDA_NO_PINNED</code></td>
                <td>Disable pinned host-memory allocation</td>
              </tr>
              <tr>
                <td><code>GGML_CUDA_REGISTER_HOST</code></td>
                <td>Enable host-memory registration</td>
              </tr>
            </tbody>
          </table>
          <p>Use <code>1</code> to activate these three settings. An empty value, <code>0</code>, <code>true</code>, or <code>yes</code> does not activate them. For example:</p>
          <pre className="code-block"><code className="language-shell">{`GGML_CUDA_ENABLE_UNIFIED_MEMORY=1 kronk server start`}</code></pre>
          <p>Set these variables before backend initialization. Their values are cached on first use, so restart the server after changing them. Do not apply this numeric rule to every native setting: <code>GGML_CUDA_P2P</code> remains presence-based, so even <code>GGML_CUDA_P2P=0</code> enables it; unset it to disable it.</p>
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
                <td>Use legacy row-split parallelism where supported</td>
              </tr>
              <tr>
                <td><code>tensor</code></td>
                <td>Use experimental tensor parallelism where supported</td>
              </tr>
            </tbody>
          </table>
          <p>When the setting is omitted, Kronk selects <code>layer</code>, matching llama.cpp's default and most compatible multi-GPU mode. Layer mode can distribute a single GGUF file across multiple GPUs. <code>row</code> remains available for legacy configurations.</p>
          <p>With AutoTune enabled, Kronk selects <code>tensor</code> when exactly two selected devices use the ROCm backend, the model architecture supports llama.cpp tensor parallelism, Flash Attention is not disabled, and GPU offload is enabled. This allows llama.cpp to use RCCL when available or its two-GPU HIP AllReduce path otherwise. An explicit <code>split-mode</code>, disabled Flash Attention, CPU-only placement, a different backend, or any other GPU count keeps the normal selection behavior. Tensor mode is experimental and requires Flash Attention; set <code>split-mode: layer</code> to opt out.</p>
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
          <p>For text generation, this allows up to four active generation requests. Requests remain isolated, and each has the configured <code>context-window</code> limit. Kronk budgets aggregate context capacity of <code>context-window × nseq-max</code>; <code>kv-cache-mode</code> determines whether KV storage is partitioned or shared. Increasing concurrency can substantially increase memory use.</p>
          <p>For embedding and reranking models, <code>nseq-max</code> limits the number of complete inputs or query-document pairs evaluated together. Their combined token count is also limited by <code>prefill-batch-size</code>. See <a href="https://www.kronkai.com/manual#chapter-4-batch-processing">Chapter 4</a> for request scheduling and the differences between model types.</p>
          <p>Text scheduling, image capacity, and image preprocessing are configured separately:</p>
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
                <td><code>prefill-batch-size</code></td>
                <td><code>2048</code></td>
                <td>Generation prefill contribution per decode iteration, or aggregate embedding/reranking tokens per native sequence batch</td>
              </tr>
              <tr>
                <td><code>physical-batch-size</code></td>
                <td>Unset</td>
                <td>Requested physical compute capacity for generation, independent of the prompt contribution; constrained by image floors, logical capacity, and MTP policy</td>
              </tr>
              <tr>
                <td><code>image-batch-token-capacity</code></td>
                <td><code>0</code> (no additional floor)</td>
                <td>Independent native capacity floor for image chunks; does not change text scheduling or image detail</td>
              </tr>
              <tr>
                <td><code>image-token-budget</code></td>
                <td>Unset (projector default)</td>
                <td>Explicit preprocessing budget for supported projectors; lowering it may reduce image resolution and detail</td>
              </tr>
            </tbody>
          </table>
          <p>For generation models, a larger <code>prefill-batch-size</code> can move a long prompt to generation in fewer decode calls, but requires larger compute buffers and each call runs longer before already-generating slots can run again. A smaller value gives generating slots more frequent scheduling opportunities at the cost of more prefill decode calls.</p>
          <p>For embedding and reranking, <code>prefill-batch-size</code> limits the combined tokens evaluated together, not the token capacity of each input. The batch is constrained independently by <code>nseq-max</code> complete sequences and by <code>prefill-batch-size</code> combined tokens. For example, <code>nseq-max: 8</code> with <code>prefill-batch-size: 2048</code> can evaluate eight 256-token sequences, four 512-token sequences, two 1024-token sequences, or one 2048-token sequence in a single batch. Kronk does not multiply the token capacity by <code>nseq-max</code>; larger aggregate batches require more compute memory. Increase <code>prefill-batch-size</code> explicitly when the expected embedding or reranking workload needs a larger aggregate batch.</p>
          <p>Most deployments should leave the prompt contribution at its default.</p>
          <p>For ordinary causal generation, <code>physical-batch-size</code> lets llama.cpp divide one logical submission into smaller compute batches without changing how many prompt tokens Kronk schedules. For example, <code>prefill-batch-size: 2048</code>, <code>physical-batch-size: 512</code>, and <code>nseq-max: 1</code> allow a prompt contribution to be processed in smaller compute batches. This can enable layer-split multi-GPU pipeline overlap within a submission.</p>
          <p>Explicit positive physical sizes override AutoTune's recommendation. AutoTune initially recommends 512 for ordinary text models using multiple selected GPUs with layer splitting and full GPU offload. Single-GPU and other execution modes retain the legacy default when the setting is omitted. Image capacity and budget floors can raise a request, and logical/native context capacity can cap it. Embedding, reranking, and specialized decision evaluation retain their own sizing policies. Requested and effective capacities are reported separately. Non-causal media chunks still must fit in one physical batch; oversized chunks return an error.</p>
          <p>Kronk adds generation capacity automatically; users do not need to calculate an allowance for active slots or draft tokens. With active MTP, Kronk keeps the work in one physical batch, so <code>physical-batch-size</code> cannot divide that work into smaller compute batches. MTP may therefore require more compute memory. See <a href="https://www.kronkai.com/manual#chapter-6-speculative-decoding-and-mtp">Chapter 6</a> for MTP configuration and limitations.</p>
          <p>Image capacity and an explicit image token budget can increase the required compute capacity without increasing text prefill per iteration.</p>
          <p>There are no model-family image-capacity defaults. Supply a non-negative <code>image-batch-token-capacity</code> when complete non-causal image chunks need more room than text prefill provides. For example, <code>prefill-batch-size: 512</code> and <code>image-batch-token-capacity: 1120</code> reserve room for a complete 1120-token image chunk while keeping text prefill at 512 tokens per iteration. This does not change image preprocessing or detail.</p>
          <p>Only an explicit positive <code>image-token-budget</code> changes the projector's preprocessing budget; neither text prefill nor image capacity silently sets it. Lowering this budget can reduce image resolution and detail. When omitted, Kronk preserves the projector's default. Not all projectors support the option (for example, Gemma 3's fixed-resolution path does not), and resizing alignment can make the final chunk count differ from the budget. Very small budgets may also conflict with a projector's minimum and fail initialization. An explicit budget also contributes to native capacity, so a separate capacity setting is unnecessary unless more room is needed. Increasing capacity does not undo a smaller preprocessing budget.</p>
          <p>Non-causal media chunks must fit completely in one physical decode. Kronk checks actual chunk sizes before native decoding in both cached and uncached paths. An oversized chunk returns an error with the required capacity instead of being split or aborting the process. Increase <code>image-batch-token-capacity</code> and reload the model to process that chunk.</p>
          <p>After loading, inspect the <code>batch-sizing</code> debug logs and Slots diagnostics to compare requested settings with effective runtime capacities. Image requirements, active MTP, and native context limits can change the effective physical capacity.</p>
          <p><img src="https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/08-wait-for-execution-slot.svg" alt="How two requests move from prefill to generation and how non-MTP and MTP batch sizing supports them" /></p>
          <p>Incremental Message Caching is configured separately with <code>incremental-cache</code> and related cache settings. See <a href="https://www.kronkai.com/manual#chapter-5-message-caching">Chapter 5</a> rather than treating cached conversations as dedicated physical slots.</p>
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
          <h4 id="independent-dimensions-of-model-design">Independent dimensions of model design</h4>
          <p>Dense, MoE, and hybrid are not three mutually exclusive model types. Describe a model using independent dimensions:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Dimension</th>
                <th>Choices</th>
                <th>What it describes</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Parameter activation</td>
                <td>Dense or Mixture-of-Experts (MoE)</td>
                <td>Which feed-forward weights compute for a token</td>
              </tr>
              <tr>
                <td>Sequence processing</td>
                <td>Attention-only, recurrent/state-space-only, or hybrid</td>
                <td>How information from earlier tokens is processed</td>
              </tr>
              <tr>
                <td>Architecture</td>
                <td>A specific overall design identifier, such as the GGUF <code>general.architecture</code> value</td>
                <td>The model's concrete layout, operations, and conventions</td>
              </tr>
            </tbody>
          </table>
          <p>An architecture identifier is not another activation or sequence-processing category. It identifies the overall design; inspect that design and its metadata to determine both dimensions rather than treating its name as a third axis of mutually exclusive types.</p>
          <p>The catalog's existing <strong>Type</strong> column reports parameter activation: Dense or MoE. New downloads also store <code>architecture</code> (the GGUF architecture identifier) and <code>sequence_processing</code> (Attention-only, Recurrent, Hybrid, or Unknown) in catalog metadata, without adding list columns. Unknown means the available metadata does not establish the sequence-processing topology. The runtime uses independent MoE and sequence-processing properties for memory planning. A hybrid MoE model combines MoE parameter activation with hybrid sequence processing; neither property replaces the other.</p>
          <p>Dense and Mixture-of-Experts (MoE) describe how each layer's feed-forward weights are activated. A dense layer uses its complete feed-forward network for every token. An MoE layer stores many expert feed-forward networks and uses a learned router to select a small subset independently for each token and layer. All experts remain in memory even though only the selected experts compute.</p>
          <p><strong>Experimental GPU expert cache:</strong> <code>moe.cache-size</code> reserves additional GPU storage for frequently used experts whose original weights remain in host memory. The value is bytes per native context; it is one global budget shared across eligible GPUs, not a budget per GPU or per sequence slot. Unset or zero leaves the llama.cpp default unchanged (currently disabled). Non-MoE models ignore this setting.</p>
          <pre className="code-block"><code className="language-yaml">{`my-moe-model:
  moe:
    mode: experts_cpu
    cache-size: 1073741824 # 1 GiB of additional GPU expert-cache capacity`}</code></pre>
          <p>The cache primarily targets small-batch generation (up to 32 tokens per microbatch when capacity permits). Multiple GPUs are supported; tensor parallelism (<code>split-mode: tensor</code>) is not. Native pipeline parallelism is disabled when caching is requested. A cache too small for the selected experts can fail context initialization. Models without eligible host-resident experts may run without an active cache. Benchmark before assuming a speedup.</p>
          <p>All three BUI calculators (standalone, model details, and catalog details) expose <strong>MoE Expert Cache (GiB)</strong> for MoE models and export <code>cache-size</code> in bytes. The calculator and pool reserve this budget in addition to model weights, KV, and compute memory. On unified-memory systems it increases the shared physical-memory footprint; original host weights are not deducted. Per-GPU allocation is an estimate: actual cache placement follows eligible layers and may use free-memory proportions. Retain backend/host-metadata headroom beyond the cache budget.</p>
          <p>For sequence processing, <strong>attention-only</strong> models use attention to access prior tokens (full, sliding-window, or sparse attention). <strong>Recurrent/state-space-only</strong> models carry information forward in recurrent state without attention layers; they are not automatically hybrid. Here, <strong>hybrid</strong> means a stack combining attention with recurrent/state-space mechanisms. Mixing full and sliding-window attention alone remains attention-only in this classification.</p>
          <p>Each sequence-processing choice can pair with dense feed-forward networks or MoE blocks; not every combination needs to be represented in Kronk's catalog. Qwen 3.6 combines Gated DeltaNet and full-attention layers with MoE; qwen4exp combines Gated DeltaNet and sparse attention with MoE. Every token still traverses every layer. The hybrid stack changes how context is processed, while an MoE router independently changes which feed-forward experts activate.</p>
          <p><img src="https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/03c-dense-moe-and-hybrid-inference.svg" alt="Independent model dimensions: Dense and MoE activation crossed with attention-only, recurrent/state-space-only, and hybrid sequence processing" /></p>
          <p>The estimator follows the model's per-layer attention topology when the GGUF provides it. Full-attention and sliding-window layers can have different KV head counts and dimensions; compact SWA includes its physical-batch headroom, while <code>swa-full: true</code> allocates SWA against the full context. Shared KV layers are not charged twice.</p>
          <p>Hybrid recurrent models use both attention-cache memory and per-sequence recurrent state. Speculative decoding can add state copies, additional context memory, and drafter weights. Use the calculator for the intended speculative configuration; an ordinary transformer KV-cache formula can substantially misestimate these models.</p>
          <p>If a configuration does not fit, consider these changes one at a time:</p>
          <ol>
            <li>Reduce <code>context-window</code>.</li>
            <li>Reduce <code>nseq-max</code>.</li>
            <li>Let automatic tuning use q8_0, or explicitly compare a quantized KV cache.</li>
            <li>On discrete-GPU systems with sufficient host RAM, move the KV cache, model layers, or MoE experts to CPU to reduce VRAM pressure.</li>
            <li>Choose a smaller or more heavily quantized GGUF.</li>
          </ol>
          <h4 id="weight-quantization-versus-kv-cache-quantization">Weight quantization versus KV-cache quantization</h4>
          <p>The quantization in a GGUF filename describes the model's stored weights. It is selected when downloading the model and cannot be changed in <code>model_config.yaml</code>. Lower-bit files generally use less storage and memory, but the quality and speed trade-offs depend on the model, quantizer, and hardware. Parameter count alone is not enough to predict whether a model fits.</p>
          <p><code>cache-type-k</code> and <code>cache-type-v</code> quantize runtime attention state instead. They do not change model weights. Evaluate weight format and KV-cache format as separate choices.</p>
          <h3 id="37-advanced-features">3.7 Advanced Features</h3>
          <h4 id="lora-adapters">LoRA adapters</h4>
          <p>Kronk can apply one or more user-provided, llama.cpp-compatible LoRA or QLoRA adapter GGUF files when it loads a base model. The adapter must be compatible with that base model. An incompatible or invalid adapter prevents the model from loading.</p>
          <p>An <code>id</code> resolves beneath the Kronk data root's <code>lora</code> directory. Do not include the <code>.gguf</code> extension:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/base-model:
  adapters:
    - id: acme/support
    - id: concise
      scale: 0.5`}</code></pre>
          <p>With the default data root, these IDs resolve to:</p>
          <pre className="code-block"><code className="language-text">{`~/.kronk/lora/acme/support.gguf
~/.kronk/lora/concise.gguf`}</code></pre>
          <p>Create the directories as needed and place the files there before loading the model. <code>KRONK_BASE_PATH</code> or <code>--base-path</code> selects the data root containing the <code>lora</code> directory; it does not move existing adapter files.</p>
          <p>To keep an adapter elsewhere, specify its absolute path instead:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/base-model:
  adapters:
    - path: /opt/adapters/support.gguf
      scale: 1.0`}</code></pre>
          <p>Each entry must set exactly one of <code>id</code> or <code>path</code>. The file must exist, be a regular file, and have a <code>.gguf</code> extension. The optional <code>scale</code> must be a finite, non-negative number and defaults to <code>1.0</code>; an explicit <code>0</code> disables that adapter's contribution while keeping the configured set unchanged. Multiple adapters compose additively using their configured scales.</p>
          <p>Adapter files and scales are fixed for the lifetime of the loaded model. After changing them, restart the server or otherwise unload and reload that model. Kronk does not download adapters, resolve them through the model catalog, or accept per-request adapter changes.</p>
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
          <p>Use <code>speculation</code> to select the implementation for a model:</p>
          <pre className="code-block"><code className="language-yaml">{`some-provider/mtp-target-model:
  speculation: disabled # auto, disabled, classic, or mtp`}</code></pre>
          <p><code>auto</code> uses a configured separate drafter first; otherwise it attempts supported MTP and can fall back to target-only generation. <code>disabled</code> runs target-only and does not load draft resources. <code>classic</code> requires <code>draft-model.model-id</code>. <code>mtp</code> requires a supported, compatible embedded head or companion assistant and a native library that provides MTP; explicitly requiring unavailable MTP fails model loading instead of silently disabling it. Do not combine <code>mtp</code> with a separate <code>draft-model.model-id</code>.</p>
          <p>Do not use model names or benchmark results as universal draft-selection rules. Measure acceptance and throughput on the actual workload. See <a href="https://www.kronkai.com/manual#chapter-6-speculative-decoding-and-mtp">Chapter 6</a> for drafter selection, adaptive throttling, observability, and limitations.</p>
          <h4 id="extended-context-with-yarn">Extended context with YaRN</h4>
          <p>Do not add RoPE scaling merely because a large <code>context-window</code> fits in memory. Scaling must match the model and its native training context. Configuration uses <code>rope-scaling-type</code> and the <code>yarn-*</code> keys described in <a href="https://www.kronkai.com/manual#chapter-7-yarn-extended-context">Chapter 7</a>.</p>
          <h4 id="per-model-sampling-defaults">Per-model sampling defaults</h4>
          <p><code>sampling-parameters</code> supplies defaults for requests using one model:</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/Qwen3-0.6B-Q8_0:
  sampling-parameters:
    temperature: 0.7
    top_p: 0.8
    top_k: 20`}</code></pre>
          <p>The nested keys use snake_case because they match request parameter names. Clients can provide request-specific values. See <a href="https://www.kronkai.com/manual#chapter-10-request-parameters">Chapter 10</a> for behavior and the full parameter reference.</p>
          <h4 id="per-model-chat-template-defaults">Per-model chat-template defaults</h4>
          <p><code>chat-template-kwargs</code> supplies model-specific Jinja variables independently from sampling defaults. Request-level <code>chat_template_kwargs</code> override matching model defaults. First-class request parameters such as <code>reasoning_effort</code> remain top-level and are resolved separately:</p>
          <pre className="code-block"><code className="language-yaml">{`unsloth/gemma-4-26B-A4B-it-UD-Q8_K_XL:
  chat-template-kwargs:
    preserve_thinking: true`}</code></pre>
          <p><code>preserve_thinking</code> is template-only. Requests should send it inside <code>chat_template_kwargs</code>; it is not a sampling parameter.</p>
          <p>Only templates that reference a key respond to it. For example, Gemma 4 uses <code>preserve_thinking</code> to retain reasoning from earlier assistant messages that contain tool calls. This can make rendered prompts more stable across user turns, but it increases input-token use and does not change Gemma's separate look-ahead behavior at some assistant/tool boundaries.</p>
          <p><code>MODEL-CONFIG</code> and per-request <code>FINAL-PARAMS</code> logs include the effective template-kwarg keys. Boolean values are shown directly so settings such as <code>preserve_thinking=true</code> can be verified; arbitrary non-boolean values are reported as <code>configured</code> instead of being written to logs.</p>
          <h3 id="38-complete-example-and-key-reference">3.8 Complete Example and Key Reference</h3>
          <p>This example shows the file structure and naming conventions. It is not a recommendation that every model needs these overrides:</p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/models/model_config.yaml

version: 2
kms: {}
models:
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
          <p>Common model-entry keys are summarized below. An omitted hardware-related value is normally supplied by analysis or by the load-time defaults.</p>
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
                <td><code>kv-cache-mode</code></td>
                <td><code>partitioned</code> (default), <code>shared</code></td>
                <td>Dedicated slot partitions or shared aggregate KV capacity</td>
              </tr>
              <tr>
                <td><code>flash-attention</code></td>
                <td><code>enabled</code>, <code>disabled</code>, <code>auto</code></td>
                <td>Attention implementation mode</td>
              </tr>
              <tr>
                <td><code>nseq-max</code></td>
                <td>Positive integer</td>
                <td>Active generation requests or embedding/reranking batch width</td>
              </tr>
              <tr>
                <td><code>admission-timeout</code></td>
                <td>Go duration, default <code>3m</code></td>
                <td>Maximum SDK admission-permit wait; separate from the server's <code>KRONK_WEB_INFERENCE_TIMEOUT</code> (default <code>60m</code>)</td>
              </tr>
              <tr>
                <td><code>queue-depth</code></td>
                <td>Non-negative integer, default <code>2</code></td>
                <td>Admitted request layers: <code>1</code> allows slot occupants only; <code>2</code> adds one waiting layer</td>
              </tr>
              <tr>
                <td><code>imc-session-capacity</code></td>
                <td>Positive integer; derived when omitted</td>
                <td>Maximum reusable IMC conversation identities; built-in storage is RAM</td>
              </tr>
              <tr>
                <td><code>prefill-batch-size</code></td>
                <td>Positive token count, default <code>2048</code></td>
                <td>Generation prefill contribution per decode iteration, or aggregate embedding/reranking tokens per native sequence batch</td>
              </tr>
              <tr>
                <td><code>physical-batch-size</code></td>
                <td>Positive token count up to <code>2147483647</code>; unset by default</td>
                <td>Requested generation compute batch size; AutoTune can choose it, and correctness constraints determine the effective capacity</td>
              </tr>
              <tr>
                <td><code>image-batch-token-capacity</code></td>
                <td>Non-negative token count, default <code>0</code></td>
                <td>Image-capacity floor for native generation batches; does not change image detail</td>
              </tr>
              <tr>
                <td><code>image-token-budget</code></td>
                <td>Positive token count up to <code>2147483647</code>; unset by default</td>
                <td>Explicit projector preprocessing budget; may reduce image detail; also contributes to native capacity</td>
              </tr>
              <tr>
                <td><code>ngpu-layers</code></td>
                <td><code>-1</code>, <code>0</code>, or a positive count</td>
                <td>CPU/GPU layer placement</td>
              </tr>
              <tr>
                <td><code>moe.mode</code></td>
                <td><code>auto</code>, <code>experts_cpu</code>, <code>experts_gpu</code>, <code>keep_top_n</code>, <code>custom</code></td>
                <td>Routed-expert placement; explicit tensor overrides take precedence</td>
              </tr>
              <tr>
                <td><code>moe.keep-experts-top-n</code></td>
                <td>Non-negative layer count</td>
                <td>Required for <code>keep_top_n</code>; highest-numbered expert layers eligible for GPU placement</td>
              </tr>
              <tr>
                <td><code>moe.cache-size</code></td>
                <td>Non-negative byte count</td>
                <td>Additional experimental GPU expert-cache budget</td>
              </tr>
              <tr>
                <td><code>tensor-buft-overrides</code></td>
                <td>CPU-placement patterns</td>
                <td>Advanced tensor placement overriding <code>moe.mode</code></td>
              </tr>
              <tr>
                <td><code>lazy-mode</code></td>
                <td><code>auto</code>, <code>off</code>, <code>on</code></td>
                <td>On-demand reading of eligible tensors; default <code>auto</code></td>
              </tr>
              <tr>
                <td><code>load-mode</code></td>
                <td><code>auto</code>, <code>mmap</code>, <code>none</code>, <code>mlock</code>, <code>mmap+mlock</code>, <code>direct-io</code></td>
                <td>Model weight loading strategy</td>
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
                <td><code>proj-device</code></td>
                <td>Device name</td>
                <td>Place multimodal projector on a specific accelerator</td>
              </tr>
              <tr>
                <td><code>devices</code></td>
                <td>Device-name list</td>
                <td>Devices available to the model</td>
              </tr>
              <tr>
                <td><code>split-mode</code></td>
                <td><code>none</code>, <code>layer</code>, <code>row</code>, <code>tensor</code></td>
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
                <td><code>adapters</code></td>
                <td>List of <code>id</code> or absolute <code>path</code>, plus optional <code>scale</code></td>
                <td>Fixed load-time LoRA adapters</td>
              </tr>
              <tr>
                <td><code>draft-model</code></td>
                <td>Mapping</td>
                <td>Separate drafter or MTP draft-count override</td>
              </tr>
              <tr>
                <td><code>speculation</code></td>
                <td><code>auto</code>, <code>disabled</code>, <code>classic</code>, <code>mtp</code></td>
                <td>Select speculative-decoding implementation</td>
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
                <td><code>chat-template-kwargs</code></td>
                <td>Mapping</td>
                <td>Per-model Jinja template defaults; request values override matching keys</td>
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
          <p>This chapter provides the runtime story for generation: how a request is admitted, turned into model work, scheduled, and executed. Model memory, batch-size configuration, and KV-cache precision are covered in <a href="https://www.kronkai.com/manual#chapter-3-model-configuration">Chapter 3</a>. Message-cache session behavior is covered in <a href="https://www.kronkai.com/manual#chapter-5-message-caching">Chapter 5</a>.</p>
          <h3 id="41-runtime-mental-model">4.1 Runtime Mental Model</h3>
          <p>Concurrency has different meanings for generation and complete-input evaluation:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Workload</th>
                <th><code>nseq-max</code> controls</th>
                <th>Behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Text generation</td>
                <td>Active generation slots</td>
                <td>Requests share model computation while keeping conversation state isolated</td>
              </tr>
              <tr>
                <td>Multimodal generation</td>
                <td>Active generation slots</td>
                <td>The same slot limit, with additional media processing</td>
              </tr>
              <tr>
                <td>Embedding/reranking</td>
                <td>Complete sequences evaluated together</td>
                <td>Inputs from queued requests can share a batch within sequence and token limits</td>
              </tr>
            </tbody>
          </table>
          <p>Multimodal generation includes requests that provide images or audio to a compatible language model. Bucky speech transcription is a separate whisper.cpp service and is not scheduled by this batch engine; see <a href="https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription">Chapter 18</a>.</p>
          <p>Increasing <code>nseq-max</code> allows more work to proceed concurrently. It can improve aggregate throughput when requests overlap, but it also increases memory capacity and gives each request a smaller share of the same compute resources. Higher concurrency can therefore increase individual response latency. There is no universal value that is best for every model, device, and workload.</p>
          <h4 id="generation-slots-and-sequences">Generation slots and sequences</h4>
          <p>For text and multimodal generation, <code>nseq-max</code> sets the number of execution slots. Each slot serves one active request. Requests share the loaded weights but keep prompt state, sampling, and output separate.</p>
          <pre className="code-block"><code className="language-diagram">{`┌───────────────┐       ┌────────────────────────────────────┐
│ Waiting jobs  │──────▶│ Batch engine                       │
└───────────────┘       │                                    │
                        │  Slot 0 ── sequence 0 ── request A │
                        │  Slot 1 ── sequence 1 ── request B │
                        │  Slot 2 ── sequence 2 ── request C │
                        └────────────────┬───────────────────┘
                                         │
                                         ▼
                        ┌──────────────────────────────────┐
                        │ Shared model context and weights │
                        └──────────────────────────────────┘`}</code></pre>
          <p>Sequence IDs isolate attention state, so one request cannot attend to another request's tokens in either KV layout. With the default <code>kv-cache-mode: partitioned</code>, Kronk gives each sequence a fixed physical KV-cache partition. Both layouts request aggregate context capacity of:</p>
          <pre className="code-block"><code className="language-text">{`context-window × nseq-max`}</code></pre>
          <p>In partitioned mode, llama.cpp divides that allocation into <code>nseq-max</code> streams, so every slot gets the full configured <code>context-window</code>; it is never divided by the slot count. An idle slot's partition is not borrowed by another slot. With <code>kv-cache-mode: shared</code>, sequences use one aggregate pool instead, while Kronk enforces the configured context-window limit per request. There is no reserved per-slot partition in shared mode. Increasing <code>nseq-max</code> increases the total capacity Kronk must allocate and budget in both modes.</p>
          <p>Partitioned ordinary generation may require separate native computations when active sequence IDs have gaps. Shared mode avoids that restriction for ordinary KV batching, but can cost more with long concurrent histories. See Chapter 3's KV Cache Layout setting for the tradeoff.</p>
          <p>When a request finishes, its slot becomes available for another waiting job. Scheduling uses the first available slot; jobs do not reserve a particular slot between requests.</p>
          <h3 id="42-the-four-stage-request-lifecycle">4.2 The Four-Stage Request Lifecycle</h3>
          <p>Every generation request passes through the same four lifecycle stages:</p>
          <ol>
            <li><strong>Admit request</strong> — apply the route deadline and acquire SDK admission capacity.</li>
            <li><strong>Prepare request and reserve session</strong> — validate the request, render and tokenize the prompt, and reserve compatible IMC state when eligible.</li>
            <li><strong>Schedule job and wait for slot</strong> — submit prepared work to the batch scheduler and wait for the first inactive execution slot.</li>
            <li><strong>Execute in assigned slot and release resources</strong> — bind state to the slot's sequence, restore or prefill model state, generate output, then clear the sequence and release ownership.</li>
          </ol>
          <p><img src="https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/05-acquire-admission.svg" alt="The four stages of a Kronk generation request" /></p>
          <p>An IMC session and an execution slot are deliberately separate. A session is a reusable conversation and model-state identity selected during Stage 2. A slot is a temporary execution resource assigned during Stage 3 and occupied during Stage 4. A session can therefore be restored into different slots on different requests.</p>
          <h3 id="43-the-generation-inference-lifecycle">4.3 The Generation Inference Lifecycle</h3>
          <p>The four stages describe ownership and waiting. The next view zooms into the generation path: Stage 2 turns portable request objects into an exact prompt plan, Stage 3 schedules that plan, and Stage 4 executes it in one slot.</p>
          <p><img src="https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/04-generation-inference-lifecycle.svg" alt="Generation inference from request preparation through slot execution" /></p>
          <p>This lifecycle is the map for the detailed views below. Chat-template rendering is part of Stage 2. Prefill batching and token generation are parts of Stage 4. IMC and speculative decoding specialize those stages without changing the top-level four-stage request lifecycle.</p>
          <h3 id="44-stage-1-—-admit-the-request">4.4 Stage 1 — Admit the Request</h3>
          <p>The outer Kronk API applies the user-visible admission limit before a request reaches model preparation. For generation, the capacity is:</p>
          <pre className="code-block"><code className="language-text">{`admission capacity = max(nseq-max, 1) × queue-depth`}</code></pre>
          <p><code>queue-depth</code> counts admitted request layers rather than a literal number of waiting requests. A depth of <code>1</code> admits only enough requests to occupy the execution slots, so <code>nseq-max: 2</code> admits two requests. The default depth of <code>2</code> admits one active layer and one additional waiting layer, so the same two-slot model admits four requests. An unset or zero value resolves to <code>2</code>, and negative values are invalid. Embedding and reranking do not use this multiplier; their admission capacity is <code>max(nseq-max, 1)</code>.</p>
          <p>If admission is full, Kronk waits for a permit for at most <code>admission-timeout</code>, a per-model setting that defaults to three minutes. This deadline applies only to the admission wait, so prompt preparation, slot waiting, and generation do <strong>not</strong> inherit a three-minute completion deadline. A caller's own earlier cancellation or deadline still applies throughout the request.</p>
          <p>The admission permit remains held until the request finishes. It therefore bounds the total number of requests that can be preparing, waiting for an execution slot, or generating—not merely the number in the handoff channel.</p>
          <p>At the default generation admission depth, <code>nseq-max: 4</code> permits up to eight requests through the outer admission gate. At most four can occupy execution slots at once; the remainder wait for a slot. Additional callers block at the admission gate until capacity is released.</p>
          <p>The model server applies a total inference timeout of 60 minutes by default. It bounds the entire inference route, including admission, IMC session reservation, execution-slot waiting, and generation. The admission-specific three-minute limit normally expires first in Stage 1. Direct SDK callers should use request cancellation or generation limits such as <code>max_tokens</code>.</p>
          <h3 id="45-stage-2-—-prepare-model-work">4.5 Stage 2 — Prepare Model Work</h3>
          <p>After admission, Kronk validates generation parameters and translates the portable request into exact model work. A model-specific chat template renders messages, roles, tools, media markers, reasoning controls, and the assistant generation cue into the protocol the selected model learned during training.</p>
          <p><img src="https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/06-render-plan-prompt.svg" alt="A chat template translates request context into the selected model's prompt protocol" /></p>
          <p>The rendered prompt—not the original message objects—is tokenized and executed. When IMC is enabled, preparation determines whether a compatible complete cached prefix can be reused. Kronk prefers a matching conversation cache, then a compatible System preload; otherwise it prepares a new cache. Partial overlap alone does not qualify for reuse. Cache planning identifies reusable work; model computation happens after slot assignment. See <a href="https://www.kronkai.com/manual#chapter-5-message-caching">Chapter 5</a> for eligibility and matching rules.</p>
          <h3 id="46-stage-3-—-schedule-the-job">4.6 Stage 3 — Schedule the Job</h3>
          <p>Prepared requests wait for the first available execution slot. <code>queue-depth</code> sets the admitted-request budget described in Stage 1; it is not another queue of that size after preparation. Raising it allows more requests to remain in the system but does not increase execution concurrency. <code>nseq-max</code> remains the maximum number of requests that can occupy slots simultaneously.</p>
          <p>Waiting honors request cancellation. If a request's context is cancelled while waiting for admission, preparing, submitting, waiting for a slot, or generating, the request returns that cancellation. During model shutdown, the engine rejects new submissions and finishes active and pending jobs with a shutdown error.</p>
          <p>The engine does <strong>not</strong> cancel a long-running request merely because another job has waited for a slot. The visible queue wait begins around engine submission and ends when the first inactive slot is assigned. It remains bounded by the route deadline and caller cancellation.</p>
          <h3 id="47-stage-4-—-execute-in-the-slot">4.7 Stage 4 — Execute in the Slot</h3>
          <h4 id="471-bind-and-restore">4.7.1 Bind and Restore</h4>
          <p>A request runs in an available slot. If preparation selected a compatible cached prefix, Kronk restores it before processing the remaining prompt; a cache miss starts with a fresh sequence. Cache identities are not permanently assigned to slots. An expected snapshot that cannot be restored can fail the request; it is not silently treated as a cache miss.</p>
          <p>Cache reuse preserves the prompt's sampling context. Unavailable speculative-decoding state does not require discarding a valid target-model prefix; generation can continue without speculation.</p>
          <h4 id="472-prefill-uncached-work">4.7.2 Prefill Uncached Work</h4>
          <p>Prefill computes the prompt state that is not already reusable from a cache. For ordinary text, one slot retains prefill priority until its prompt is complete, cancelled, or no longer eligible. <code>prefill-batch-size</code>—2048 tokens by default—caps its text contribution per scheduling iteration. A long prompt can therefore delay another new prompt even when both have execution slots.</p>
          <p>Kronk interleaves this work with output generation from ready slots. A new prompt does not normally need to finish all of its prefill before existing responses can continue, but each compute batch still takes time. Larger prefill batches can increase pauses between streamed output tokens. This is a responsiveness tradeoff, not a fixed latency guarantee.</p>
          <p>Eligible text-only, layer-split multi-GPU workloads can overlap prompt computation across GPUs. This includes building or extending a text IMC prefix. Actual overlap depends on the backend; media and speculative workloads should not be assumed to have the same behavior.</p>
          <p>Media requires additional encoding and may require isolated processing rather than sharing a text batch. Supported causal media can share computation when it fits; other media uses a separate decode path. In particular, non-causal image chunks must fit completely in one physical batch. Increase <code>image-batch-token-capacity</code> and reload the model if an oversized chunk is rejected. This capacity setting does not increase image detail; an explicit <code>image-token-budget</code> controls preprocessing on supported projectors.</p>
          <p><img src="https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/09-restore-session-state.svg" alt="The two prefill phases: restore reusable IMC state, then compute missing request state" /></p>
          <p>Kronk automatically includes capacity for generation and speculative work. Operators do not need to calculate extra output rows. Active MTP keeps work in one physical batch, so it cannot use <code>physical-batch-size</code> to split computation into smaller batches. Chapter 3 describes these controls and their memory tradeoffs; Chapter 6 covers speculative decoding.</p>
          <h4 id="473-generate-output">4.7.3 Generate Output</h4>
          <p>Once prompt processing completes, Kronk samples output tokens and continues generation using each request's own history and sampling settings. Ready requests share model computation; one request does not receive another's generated tokens. Streaming clients receive output as it becomes available.</p>
          <p><img src="https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/10-run-generation-inference.svg" alt="Stage 4 ordinary token generation from batched decode through sampling and streaming" /></p>
          <p>The model's end-of-generation signal, parsed completion, and <code>max_tokens</code> end generation normally. Cancellation, context or decode failure, streaming failure, and engine shutdown use the error path. Sampling controls and generation limits are documented in <a href="https://www.kronkai.com/manual#chapter-10-request-parameters">Chapter 10</a>, while endpoint-specific stream framing is documented in <a href="https://www.kronkai.com/manual#chapter-9-api-endpoints">Chapter 9</a>.</p>
          <p>Media input requires specialized encoder and prefill steps, so it is not always combined with text work in one forward pass. Multi-Token Prediction (MTP) also changes how some prefill and verification batches are formed. These special cases preserve the same user-visible slot limit but should not be treated as identical scheduling at the backend level. <a href="https://www.kronkai.com/manual#chapter-6-speculative-decoding-and-mtp">Chapter 6</a> zooms into proposal, verification, acceptance, and state synchronization.</p>
          <h4 id="474-finish-and-release-resources">4.7.4 Finish and Release Resources</h4>
          <p>On completion, cancellation, or failure, Kronk releases the request's execution resources, any IMC reservation it still holds, and its admission permit. The slot's active sequence is cleared before reuse. A cache already saved by the request can remain available after the slot is released; the next request does not inherit the previous request's sampling, parsing, or live sequence state.</p>
          <p>Most users should leave <code>prefill-batch-size</code> at its 2048-token default. Kronk derives the internal batch capacities as described in <a href="https://www.kronkai.com/manual#35-concurrency-and-batching">Chapter 3 §3.5</a>.</p>
          <h3 id="48-embedding-and-reranking">4.8 Embedding and Reranking</h3>
          <p>Embedding and reranking models do not use generation slots. They use sequence batching: complete inputs from one or more queued requests can be evaluated together. This applies to all models identified as embedding or reranking models; it is not limited to an architecture allowlist.</p>
          <pre className="code-block"><code className="language-diagram">{`┌──────────────┐       ┌────────────────────────────────┐
│ Request jobs │──────▶│ Sequence-batch scheduler       │
└──────────────┘       │ seq 0 ── request A, input 0    │
                       │ seq 1 ── request A, input 1    │
                       │ seq 2 ── request B, input 0    │
                       └───────────────┬────────────────┘
                                       ▼
                       ┌───────────────────────────────┐
                       │ One model context and weights │
                       └───────────────────────────────┘`}</code></pre>
          <p>The scheduler keeps each embedding input or reranking query-document pair as a complete sequence. It coalesces already queued requests, fills a native batch up to the <code>nseq-max</code> and token limits, and schedules requests round-robin when one request cannot fit in a single batch. The engine is intentionally separate from generation because it does not own long-lived generation slots, samplers, or streaming state.</p>
          <p>The two limits control independent dimensions of a sequence batch:</p>
          <pre className="code-block"><code className="language-text">{`number of sequences <= nseq-max
sum of sequence tokens <= prefill-batch-size`}</code></pre>
          <p>For these models, <code>prefill-batch-size</code> is an aggregate batch token budget, not a per-sequence allowance. With <code>nseq-max: 8</code> and <code>prefill-batch-size: 2048</code>, one batch can hold eight 256-token sequences or two 1024-token sequences, but not eight 2048-token sequences. Kronk deliberately does not multiply <code>prefill-batch-size</code> by <code>nseq-max</code>; larger batches require more compute memory. Increase <code>prefill-batch-size</code> explicitly when measured workloads benefit from a larger aggregate token budget.</p>
          <p>Raise <code>nseq-max</code> only after measuring throughput and memory for the actual model and workload.</p>
          <h3 id="49-configuration-and-tuning">4.9 Configuration and Tuning</h3>
          <p>Configure concurrency in <code>~/.kronk/models/model_config.yaml</code>:</p>
          <pre className="code-block"><code className="language-yaml">{`version: 2
models:
  mradermacher/Qwopus3.5-4B-Coder.Q8_0:
    context-window: 32768
    nseq-max: 2
    admission-timeout: 3m
    queue-depth: 2`}</code></pre>
          <p>The file is read at server startup. Restart the server after changing it. The entry under <code>models</code> must match the model ID used by requests.</p>
          <p><code>admission-timeout</code> is separate from the model server's <code>KRONK_WEB_INFERENCE_TIMEOUT</code> (default <code>60m</code>): admission timeout only bounds waiting for a permit, while inference timeout bounds admitted request processing at the server/web/CLI layer.</p>
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
          <p>Do not treat weight size plus a hand-calculated KV value as total VRAM. Use the BUI's <strong>Apps → VRAM Calculator</strong> and retain operating headroom. See <a href="https://www.kronkai.com/manual#36-memory-planning-and-quantization">Chapter 3 §3.6</a> for the components that affect an estimate.</p>
          <h3 id="410-interaction-with-message-caching">4.10 Interaction with Message Caching</h3>
          <p>Incremental Message Caching (IMC) keeps reusable conversation state in a logical session, not in a permanently assigned execution slot. Cached state is externalized to a session store between requests. A later request can restore that state into any free slot, extend it, and continue generation.</p>
          <p>While a request executes, its restored or newly prepared prefix occupies the slot's model state. Cache builds and extensions run as scheduled preparation work, in bounded steps, before processing the remaining prompt. Once the stable prefix is ready, Kronk saves it for reuse; exact read-only hits can avoid saving the same state again. Completion clears the live sequence, not the reusable cache. Cached conversations can therefore outnumber execution slots.</p>
          <p>The IMC pool contains:</p>
          <pre className="code-block"><code className="language-text">{`default session capacity = max(nseq-max, 1) × queue-depth`}</code></pre>
          <p>The default session pool matches generation admission capacity. Set <code>imc-session-capacity</code> to a larger explicit value when the measured warm conversation working set warrants it. An explicit value cannot be smaller than generation admission capacity. Therefore, for generation through the Kronk SDK:</p>
          <pre className="code-block"><code className="language-text">{`session capacity ≥ admission capacity`}</code></pre>
          <p>This invariant prevents Kronk from admitting more concurrent generation requests than the IMC planner has session identities available to reserve. It avoids moving an admitted request into preparation when every session is already owned by another admitted request.</p>
          <p>A reservation gives one request exclusive access to a cache entry, including while it waits for a slot. It is not a measure of active generation. A newly built or extended cache may become available to other requests before its owner finishes generating. Exact read-only reuse and some media operations retain the reservation until completion.</p>
          <p>Kronk's built-in cache store uses host RAM, and System preloads always remain in RAM. Embedded applications can supply a custom working-session store; externalizing state does not by itself make it durable across restarts. Cache matching, media reuse, invalidation, and settings are covered in <a href="https://www.kronkai.com/manual#chapter-5-message-caching">Chapter 5</a>.</p>
          <h3 id="411-observing-queue-behavior">4.11 Observing Queue Behavior</h3>
          <p>Kronk records two direct indicators of generation-slot contention:</p>
          <ul>
            <li>the <code>queue-wait</code> trace span, which wraps the submit attempt and subsequent slot wait for successful jobs; and</li>
            <li>the <code>chat_queue_wait_seconds</code> Prometheus histogram, recorded when a slot is assigned.</li>
          </ul>
          <p>For a successful job, timing starts immediately before attempting submission to the batch engine and ends at slot assignment. It does not include time blocked at the outer SDK admission gate or time spent preparing an IMC session before the submit attempt. Compare it with end-to-end request duration and time-to-first-token measurements when diagnosing latency.</p>
          <p>Embedding and reranking expose <code>inference_requests_total</code>, <code>inference_request_duration_seconds</code>, and <code>inference_active_requests</code>, labeled by operation and runtime (currently <code>batchseq</code>). Sequence batching also publishes <code>batchseq_queue_wait_seconds</code>, <code>batchseq_items</code>, and <code>batchseq_batches_total</code>. These distinguish outer request concurrency from the number and width of native batches actually evaluated.</p>
          <p>The <code>inference_*</code> metrics begin after the outer SDK admission permit is acquired. They describe admitted model-layer work and do not count admission wait time or requests that time out or are cancelled before admission.</p>
          <p>Consistently increasing queue-wait time indicates contention for execution capacity; compare request volume, prompt lengths, and generation lengths. Before raising <code>nseq-max</code>, confirm that the device has memory headroom and that aggregate throughput improves under a realistic concurrent load. See <a href="https://www.kronkai.com/manual#chapter-15-observability">Chapter 15</a> for metrics, tracing, and profiling.</p>
          <hr />
          <h2 id="chapter-5-message-caching">Chapter 5: Message Caching</h2>
          <h2 id="51-what-imc-does">5.1 What IMC Does</h2>
          <p>Chapter 4 explains where IMC participates in the generation lifecycle. This chapter explains how Kronk chooses reusable state and preserves it safely.</p>
          <p>Incremental Message Cache (IMC) reduces repeated prompt processing in multi-turn conversations. Without IMC, the model must prefill the complete conversation before generating each response. With IMC, Kronk can restore a previously processed prompt prefix and prefill only the new portion.</p>
          <p>IMC is enabled by default for generation models. It is most useful for:</p>
          <ul>
            <li>Long-running chat and coding-agent conversations</li>
            <li>Tool-calling workflows that append results to the existing history</li>
            <li>Multiple agents or conversation branches sharing one model</li>
            <li>Prompts with expensive media that remains unchanged across follow-up turns</li>
          </ul>
          <p>Chat-completion clients still send the complete conversation on every request, including earlier media. IMC avoids repeated model computation; it does not store a client-visible conversation history or let a later request omit it. Built-in cache state is lost when the model unloads or the server restarts. There is no <code>cache_id</code> contract for choosing a session: Kronk automatically matches compatible rendered prefixes.</p>
          <p>Short, one-shot prompts generally gain little from caching. IMC also performs host-side rendering, tokenization, snapshot, and restore work, so it is not a replacement for choosing an appropriate context window and concurrency level.</p>
          <h3 id="511-quick-semantic-understanding">5.1.1 Quick Semantic Understanding</h3>
          <p>IMC is best understood as <strong>prompt planning backed by reusable model state</strong>. Before assigning a request to an execution slot, Kronk determines the exact rendered prompt, its reusable stable portion, the inference-only tail, the compatible saved state, and the work required to move from that state to the current request.</p>
          <p>IMC retains both the complete reusable prefix and the model state computed for it. Matching the prefix establishes whether reuse is safe; restoring the saved state avoids processing that prefix again. The saved state depends on the model and may include attention, recurrent, and speculative-decoding state.</p>
          <p>This is prompt-oriented rather than message-oriented because the model does not consume message objects directly. It consumes rendered tokens, media embeddings, and positions. Requests with apparently unchanged messages can render differently when tools, thinking settings, templates, media, or other render-affecting inputs change.</p>
          <p>Each model owns two bounded pools with equal capacity:</p>
          <ul>
            <li><strong>Working sessions</strong> retain the latest complete stable prompt states and are the normal exact and append paths.</li>
            <li><strong>System caches</strong> retain immutable, rendered system-only prefixes. Multiple working sessions can preload the same System cache entry.</li>
          </ul>
          <p>System provides a conservative restart point when a chat template retroactively changes historical turns. It does not retain a user-turn boundary or a progressively discovered common prefix.</p>
          <p>Kronk never restores an arbitrary longest common prefix (LCP). It restores only a complete Current or System snapshot whose full token sequence prefixes the new stable rendering. If neither qualifies, Kronk rebuilds from the beginning. The LCP itself is not model state and is not a safe restoration boundary.</p>
          <p>The complete generation lifecycle is introduced in <a href="https://www.kronkai.com/manual#43-the-generation-inference-lifecycle">Chapter 4 §4.3</a>. IMC first searches every available working cache and selects the longest complete match. Only when no working cache matches does it search the model's System cache pool. A System hit is copied into a separately selected working session. If neither tier matches, Kronk selects an empty working session or resets an available least-recently-used one.</p>
          <p>The planning process has five steps.</p>
          <h4 id="step-1-render-the-complete-prompt-with-and-without-the-generation-suffix">Step 1: Render the complete prompt with and without the generation suffix</h4>
          <p>Kronk independently renders the complete conversation in two forms:</p>
          <pre className="code-block"><code className="language-text">{`Stable rendering:           [complete conversation]
Generation-ready rendering: [complete conversation][generation suffix]`}</code></pre>
          <p>The stable rendering disables the generation prompt and becomes the reusable cache target. The generation-ready rendering is the prompt used for inference. Its suffix is normally template material that begins the next assistant turn, such as an assistant header or control tokens; it is not the answer generated by the model.</p>
          <p>After tokenization or media-plan construction, the stable plan must be a strict prefix of the generation-ready plan, and the generation-ready plan must contribute a nonempty tail. For media, that tail must also be text-only. Kronk renders both complete prompts instead of independently rendering a suffix because a chat template can make separators, control tokens, and role formatting depend on the surrounding messages. If the resulting plans are not prefix-compatible, Kronk safely processes the complete generation-ready prompt without IMC reuse.</p>
          <h4 id="step-2-build-a-canonical-token-or-media-plan">Step 2: Build a canonical token or media plan</h4>
          <p>For text, the canonical plan is the complete sequence of rendered token IDs:</p>
          <pre className="code-block"><code className="language-text">{`Stable plan: [A B C D]
Actual plan: [A B C D][G]`}</code></pre>
          <p><code>[G]</code> is the generation-ready tail. It is processed for inference but is not part of the reusable stable snapshot.</p>
          <p>For media, token equality alone is insufficient. Kronk builds an ordered logical plan containing text tokens and media-content identities:</p>
          <pre className="code-block"><code className="language-text">{`[token][token][image digest][token][audio digest][token]`}</code></pre>
          <p>This plan captures media content, count, order, and placement relative to text. It is called canonical because it is the authoritative logical description of what the engine must execute, not merely a normalized version of the client's messages.</p>
          <p>A media item is one logical plan unit, but the multimodal pipeline may expand it into many physical KV cells. Models using M-RoPE can also distinguish the next logical decode position from the physical KV-cell count. IMC therefore keeps the logical plan, logical position, and physical snapshot accounting consistent without pretending that media is an ordinary text token.</p>
          <h4 id="step-3-select-a-complete-safe-prefix-by-tier">Step 3: Select a complete safe prefix by tier</h4>
          <p>Kronk first searches every available working cache and selects the longest complete plan that prefixes the new stable plan. It searches the separate System pool only when no working cache matches:</p>
          <pre className="code-block"><code className="language-text">{`New stable plan: [A B C D E F]

Session 1 Current:  [A B]          -> safe prefix
Session 2 Current:  [A B C D]      -> safe prefix and better
Session 3 Current:  [A B X]        -> not safe
Session 4 Current:  [A B C D E F]  -> exact and best
System cache pool:                 -> not searched; working cache matched

If every Current diverges:
System cache 1:     [A B]          -> safe verified system-only prefix
Empty/LRU session:                 -> destination for the copied state`}</code></pre>
          <p>Choosing the longest compatible working cache minimizes new prefill work. System lookup instead requires an exact match with the independently rendered system-only candidate; it does not choose an arbitrary shorter System prefix. A prefix is safe only when:</p>
          <ul>
            <li>It ends at a complete, committed session boundary.</li>
            <li>The saved plan and snapshot metadata describe the same model state.</li>
            <li>The complete saved plan prefixes the new stable plan.</li>
            <li>The session is available rather than reserved by another request.</li>
            <li>For media, the saved plan remains unchanged. Added media must also pass attention-mode and native-prefix compatibility checks.</li>
          </ul>
          <p>"Longest complete working prefix" does not mean the longest coincidental token overlap. For example, <code>[A B C D]</code> is not shortened and reused for <code>[A B X D]</code>, even though <code>[A B]</code> matches. The LCP is not selected as the boundary. Kronk restores only model state that it explicitly serialized as Current or System. This avoids assuming that an internal KV cut remains valid across template boundaries, media embeddings, M-RoPE positions, hybrid recurrent state, or draft/MTP state.</p>
          <p>When establishing a text session, Kronk independently renders the system-only prefix without a generation cue:</p>
          <pre className="code-block"><code className="language-text">{`[SYSTEM]
   ^ fixed checkpoint candidate`}</code></pre>
          <p>The candidate is accepted only when it is nonempty, cache-worthy, and its complete token sequence exactly prefixes the request's stable rendering. Kronk serializes matching target and compatible draft/MTP state into the model-owned System pool. An exact duplicate refreshes the existing entry. A new prefix uses an empty entry or replaces the least-recently-used entry that is not being restored.</p>
          <h4 id="step-4-select-a-reuse-action-or-rebuild">Step 4: Select a reuse action or rebuild</h4>
          <p>The selected action follows from the stable plan and the best safe session.</p>
          <p><strong>Exact</strong> means the cached stable plan and new stable plan are identical:</p>
          <pre className="code-block"><code className="language-text">{`Cached stable: [A B C D]
New stable:    [A B C D]
Actual:        [A B C D][G]`}</code></pre>
          <p>Kronk restores the snapshot and processes only <code>[G]</code>. Because the stable state did not change, it can usually skip serializing the same snapshot again.</p>
          <p><strong>Append</strong> applies to a text-only stable extension:</p>
          <pre className="code-block"><code className="language-text">{`Cached stable: [A B C D]
New stable:    [A B C D E F]
Actual:        [A B C D E F][G]`}</code></pre>
          <p>Kronk restores <code>[A B C D]</code>, prefills <code>[E F]</code>, snapshots the new reusable state <code>[A B C D E F]</code>, and then processes <code>[G]</code> without adding it to that stable snapshot.</p>
          <p>If Current diverges because a template retroactively changes historical rendering, append can begin from System only when the complete System token sequence prefixes the new stable rendering. Kronk restores that fixed state and prefills everything after it; it never rewinds Current or deletes an arbitrary KV range.</p>
          <p><strong>Anchor</strong> is the media-safe form of append:</p>
          <pre className="code-block"><code className="language-text">{`Cached plan: [A B][image-1][C D]
New plan:    [A B][image-1][C D E F]`}</code></pre>
          <p>The unchanged media-bearing plan acts as an anchor. Kronk restores its snapshot, does not encode <code>image-1</code> again, prefills only <code>[E F]</code>, and stages an advanced snapshot before processing the generation tail. The staged snapshot and its matching metadata are published together only after success, so a failed advance leaves the previous media snapshot authoritative.</p>
          <p>Changing, removing, or reordering an existing media prefix is not an anchor extension:</p>
          <pre className="code-block"><code className="language-text">{`[A B][image-2][C D]             -> rebuild: changed media
[A B][image-1][image-2][C D]    -> rebuild: inserted media before cached C D
[A B][C D][image-1]             -> rebuild: reordered media
[A B][C D]                      -> rebuild: removed media`}</code></pre>
          <p>These outcomes describe comparison with the shown anchor. Because Kronk searches the full available session pool, it can still reuse another session whose complete media plan is compatible with the request.</p>
          <p><strong>Media append</strong> can reuse a complete cached text or media prefix while adding new media after it:</p>
          <pre className="code-block"><code className="language-text">{`Cached plan: [A B][image-1][C D]
New plan:    [A B][image-1][C D E F][image-2]`}</code></pre>
          <p>This requires compatible causal attention and a verified native media prefix. A non-causal cached session or a native-prefix mismatch can require a rebuild. The cached portion avoids repeated model media encoding and prefill, but the complete input may still require host-side media preparation and tokenization.</p>
          <p><strong>Rebuild</strong> means no eligible working cache or exact System checkpoint matches. Kronk selects an empty session or the least recently used available session, resets it, processes the stable plan from the beginning, snapshots the resulting state, and then processes the generation tail. Rebuild is not a request failure; it means only that the request receives no saved-prefill benefit.</p>
          <h4 id="step-5-reserve-the-selected-session">Step 5: Reserve the selected session</h4>
          <p>Planning and execution do not happen at the same instant. A request may wait for an execution slot after selecting a session, so Kronk immediately reserves it for that request:</p>
          <pre className="code-block"><code className="language-text">{`select session -> reserve -> queue -> restore/build stable state

ordinary text append/rebuild or initial media build:
    publish complete stable snapshot -> release -> generate

exact read-only hit or media-anchor advance:
    keep reservation -> generate -> release at completion`}</code></pre>
          <p>Other planners skip reserved sessions. Reservations apply to exact matches, appends, media anchors, empty sessions selected for rebuild, and LRU sessions selected for replacement. Even an exact match needs a reservation: another request must not rebuild or replace its snapshot between selection and restore.</p>
          <p>If every session is reserved, Kronk returns a retryable busy error rather than evicting an active session. The reservation also prevents a partially updated session from becoming visible. Snapshot bytes and metadata must describe the same stable prefix before the session can be selected again.</p>
          <h2 id="52-how-kronk-reuses-a-text-prefix">5.2 How Kronk Reuses a Text Prefix</h2>
          <p>Kronk compares the complete stable token sequence with the Current and fixed System sequences in available sessions. The result is one of three match types:</p>
          <ul>
            <li><strong>Exact</strong> — The new stable sequence is identical to a cached sequence. Kronk restores that session and processes only the generation-ready tail.</li>
            <li><strong>Append</strong> — A cached sequence is a complete prefix of the new stable sequence. Kronk restores it, processes the appended stable tokens, and then processes the generation-ready tail.</li>
            <li><strong>Rebuild</strong> — No complete cached sequence prefixes the new stable sequence. Kronk uses an empty session or replaces the least recently used available session and processes the stable prefix from the beginning.</li>
          </ul>
          <p>Only complete-prefix reuse is allowed. If an earlier message is edited, removed, reordered, or rendered differently, Kronk can reuse System only when that complete fixed state still prefixes the new rendering. Kronk does not turn a coincidental partial token match into cached model state.</p>
          <p>For example:</p>
          <pre className="code-block"><code className="language-text">{`Cached stable tokens: [A B C D]

New stable tokens:    [A B C D]       -> exact
New stable tokens:    [A B C D E F]   -> append E F
New stable tokens:    [A B X D]       -> rebuild`}</code></pre>
          <p>This comparison uses rendered tokens, not only the message objects supplied by the client. Changes to the chat template, tool definitions, thinking options, or other inputs that affect rendering can therefore prevent reuse even when the visible message text appears unchanged.</p>
          <h3 id="521-system-cache-lifecycle-and-template-behavior">5.2.1 System Cache Lifecycle and Template Behavior</h3>
          <p>The preload pool has one role: <strong>System</strong>. Kronk renders the system-only prompt independently and verifies its complete token sequence against the stable rendering. If it is nonempty and at least <code>cache-min-tokens</code>, Kronk retains that model state in a model-owned, RAM-only cache. There is no entry when the request has no cache-worthy system-only prefix.</p>
          <p>The normal lifecycle is:</p>
          <ol>
            <li>Search all working sessions first and use the normal exact or append path when one matches.</li>
            <li>If none matches, search System entries by exact rendered system-token sequence.</li>
            <li>On a System hit, reserve a separate empty or least-recently-used working session, copy the immutable System state into it, prefill the suffix, and publish the complete stable state there.</li>
            <li>On a System miss, rebuild from the beginning and attempt to retain the new System state. Retention failure never fails inference.</li>
            <li>When the equal-capacity System pool is full, replace its least-recently-used available entry. An entry currently being restored cannot be replaced.</li>
          </ol>
          <p>For example:</p>
          <pre className="code-block"><code className="language-text">{`System:  [SYSTEM]                                      (fixed)
Current: [SYSTEM][USER1][ASSISTANT/TOOLS]

Next stable rendering is prefix-compatible:
  restore Current -> append new stable tokens -> replace Current

Next stable rendering retroactively changes an assistant turn:
  reject Current -> restore System -> recompute after System -> replace Current`}</code></pre>
          <p>The exact token lengths depend on the template; the brackets show logical regions, not token counts. The diagram combines normal deterministic growth with the System-only recovery path used when every Current cache diverges.</p>
          <p><img src="https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/07-select-imc-session.svg" alt="IMC cache population and session selection" /></p>
          <p>System never advances to a user, assistant, tool, or computed token boundary. The pool has the same number of entries as the working-session pool, and one immutable entry can preload multiple conversation branches.</p>
          <h2 id="53-sessions-slots-and-snapshots">5.3 Sessions, Slots, and Snapshots</h2>
          <p>An IMC <strong>session</strong> is a reusable conversation identity and its saved model state. An execution <strong>slot</strong> is a lane that can actively run a request. These are deliberately separate:</p>
          <ul>
            <li>By default, Kronk retains <code>max(nseq-max, 1) × queue-depth</code> working sessions and the same number of System cache entries; <code>imc-session-capacity</code> can increase both pool capacities.</li>
            <li>Only <code>nseq-max</code> requests can decode concurrently.</li>
            <li>A session can be restored into any available execution slot; it is not tied permanently to one slot.</li>
            <li>Snapshot bytes and backing storage are allocated lazily as conversations begin using them.</li>
          </ul>
          <p>For example, <code>nseq-max: 2</code> with the default <code>queue-depth: 2</code> provides two concurrent decode slots and four warm IMC session identities. The default working-session pool therefore matches generation admission capacity, and the System pool has four entries as well. Raising <code>nseq-max</code> also adds another full <code>context-window</code> KV stream and its memory cost, so do not raise it solely to retain more conversation branches without considering the effects described in <a href="https://www.kronkai.com/manual#chapter-4-batch-processing">Chapter 4</a>.</p>
          <p>Setting <code>queue-depth: 1</code> with two slots instead provides two working sessions and two System entries. It does not reduce two-request execution concurrency; it prevents additional requests from passing admission until a slot's request finishes.</p>
          <p>An explicit <code>imc-session-capacity</code> must be at least <code>nseq-max × queue-depth</code>. This preserves one reservable session identity for every admitted generation request. Values above that floor retain more completed conversation branches and may reduce LRU rebuilds, at the cost of additional session-store capacity.</p>
          <p>Admission waiting is controlled separately by the per-model <code>admission-timeout</code> setting (default <code>3m</code>). It only bounds the wait for an SDK admission permit. The server's <code>KRONK_WEB_INFERENCE_TIMEOUT</code> (default <code>60m</code>) instead bounds admitted preparation, slot waiting, and inference; neither setting changes IMC session retention.</p>
          <p>Slot assignment and cache preparation are separate steps. Text and media cache builds and extensions run as scheduled work after a request receives a slot. One preparing request retains priority until its reusable prefix is ready, cancelled, or no longer eligible. Work proceeds in bounded steps, so already-generating requests can continue between steps. Media encoding and indivisible image chunks can still take longer; this is not a fixed latency guarantee. Text preparation uses <code>prefill-batch-size</code>, 2048 tokens by default.</p>
          <p>Kronk reserves a session as soon as it selects it for an exact match, append, or rebuild. Other requests cannot select that identity while the reservation is held. If all session identities are reserved, the request returns a busy error and should be retried. Kronk does not evict an active session to make room.</p>
          <p>During a request, Kronk restores the selected snapshot into a free slot. For a new or appended stable prefix, it creates the updated snapshot after processing the stable tokens. The generation-ready tail is then processed without making it part of that reusable stable prefix.</p>
          <p>Repetition penalties and DRY still account for the complete prompt, including the restored portion. Cache reuse does not remove earlier text from sampling history merely because the model computation was skipped.</p>
          <p>Kronk can retain an independently rendered and verified text-only system preload, including matching draft/MTP state when available. These preloads live in a model-owned RAM pool whose capacity equals the working session count. Unlike working snapshots, System entries do not use the configured session store; saved working snapshots already contain the System prefix.</p>
          <p>An exact match may skip rewriting the snapshot when the stable state has not changed. This avoids an unnecessary serialization of the state that was just restored. Exact media-plan reuse can receive the same optimization. This does not change which content is considered part of the cache.</p>
          <p>Snapshots externalize inactive session state from the model's active KV cache. They therefore do not permanently occupy an execution slot or pin their state in accelerator KV memory between requests. They do consume the configured session-store capacity, as described in <a href="#55-configuration-and-storage">Configuration and Storage</a>.</p>
          <h2 id="54-media-requests">5.4 Media Requests</h2>
          <p>IMC supports media processed by Kronk's multimodal pipeline. Instead of relying only on text-token equality, Kronk builds a logical plan containing the ordered text and media inputs.</p>
          <p>Kronk can reuse a media session in three cases:</p>
          <ul>
            <li><strong>Exact plan</strong> — The complete stable media plan is unchanged.</li>
            <li><strong>Text extension from an anchor</strong> — The stored media plan remains unchanged and is followed only by new text. Kronk restores the media state and processes the text extension without encoding the media again.</li>
            <li><strong>Compatible media append</strong> — A complete cached text or media prefix is followed by new media. Kronk reuses the prefix when causal-attention and native-prefix checks permit it, then encodes and processes the added media.</li>
          </ul>
          <p>Kronk rebuilds when earlier media changes or an append is incompatible and no available session has a compatible plan. This conservative rule lets the model-specific multimodal pipeline remain authoritative for media embeddings, token placement, and position handling.</p>
          <p>For example, a user can submit an image and then ask several text-only follow-up questions. The saved media plan acts as an anchor for those turns. Replacing the image cannot extend that anchor; it requires a rebuild unless another session already holds the new plan. Adding media after a complete cached prefix can reuse it when the compatibility checks permit.</p>
          <p>See <a href="https://www.kronkai.com/manual#chapter-11-multimodal-models">Chapter 11</a> for supported media inputs and model requirements.</p>
          <h2 id="55-configuration-and-storage">5.5 Configuration and Storage</h2>
          <p>IMC settings belong under the model ID in <code>~/.kronk/models/model_config.yaml</code>:</p>
          <pre className="code-block"><code className="language-yaml">{`version: 2
models:
  unsloth/Qwen3-1.7B-UD-Q8_K_XL:
    nseq-max: 2
    queue-depth: 2
    incremental-cache: true
    cache-min-tokens: 100
    imc-session-capacity: 8
    session-store-kind: ram`}</code></pre>
          <p>Restart the server after changing the file. This example retains eight working cache identities while admitting up to four requests for two execution slots.</p>
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
                <td>Minimum stable text-token-plan length required for text IMC reuse.</td>
              </tr>
              <tr>
                <td><code>imc-session-capacity</code></td>
                <td>Derived</td>
                <td>Reusable identities; must be at least admission capacity.</td>
              </tr>
              <tr>
                <td><code>session-store-kind</code></td>
                <td><code>ram</code></td>
                <td>Working-session storage; only <code>ram</code> is supported in server configuration.</td>
              </tr>
            </tbody>
          </table>
          <p>When <code>imc-session-capacity</code> is omitted, Kronk derives the capacity from <code>nseq-max</code> and <code>queue-depth</code>. The same admission-capacity floor applies to an explicit value in <code>model_config.yaml</code>.</p>
          <p>The <code>cache-min-tokens</code> setting applies to the stable text-token-plan length. A text request below the threshold still works, but Kronk processes its complete generation-ready prompt without creating or reusing an IMC session. The current media working-plan cache does not apply this threshold; media safety depends on compatible stable and generation-ready plans. Text-only System checkpoints still use the threshold, including those created during media requests. Zero or negative <code>cache-min-tokens</code> values use the default of 100 rather than making every text prompt eligible.</p>
          <p>Set <code>incremental-cache: false</code> if a workload is entirely short-lived or if you need to compare behavior without prompt caching.</p>
          <h3 id="built-in-ram-storage">Built-in RAM storage</h3>
          <p>The built-in <code>ram</code> store keeps snapshots in process memory. It is selected by default when <code>session-store-kind</code> is omitted. Each session buffer grows as needed and retains its peak allocation for reuse. Actual memory use depends on the model, cached conversation lengths, KV data types, and number of sessions that have been used. Budget for peak conversation state across the branches you expect to keep warm, not just the <code>nseq-max</code> requests that can run simultaneously.</p>
          <h3 id="custom-sdk-storage">Custom SDK storage</h3>
          <p>Embedded applications can replace working-session storage. This is an SDK extension, not a server configuration option for disk storage. System preloads still use RAM. Custom storage must preserve complete snapshots, report read and write failures, and provide its own capacity limits and cleanup policy.</p>
          <p>Writing snapshot bytes to disk does not make conversations recoverable after a restart. Durable recovery also needs compatible model and configuration identities, matching cache metadata, validated snapshots, and an application recovery design.</p>
          <p>The <a href="https://github.com/ardanlabs/kronk/tree/main/examples/session-store"><code>examples/session-store</code></a> program demonstrates the SDK extension. It uses temporary files and deletes them when closed; it does not provide startup recovery or durable atomic commits. <strong>Do not use the example as durable session storage.</strong> Consult the SDK storage contract when implementing an alternative backend.</p>
          <p>Some MTP configurations maintain draft-model cached state and saved hidden state in addition to the target model snapshot. Account for this extra storage when sizing memory. See <a href="https://www.kronkai.com/manual#chapter-6-speculative-decoding-and-mtp">Chapter 6</a> for MTP configuration and behavior.</p>
          <h2 id="56-invalidation-and-limitations">5.6 Invalidation and Limitations</h2>
          <p>IMC favors safe reuse over partial recovery. A session is rebuilt when Kronk cannot prove that its complete saved prefix matches the new stable prompt. Common causes include:</p>
          <ul>
            <li>Editing, deleting, or reordering earlier conversation content</li>
            <li>Changing tools or settings that alter the rendered prompt</li>
            <li>Changing, removing, or reordering cached media, or adding incompatible media</li>
            <li>Loading a different model or an incompatible model configuration</li>
          </ul>
          <p>If the stable rendering does not prefix the generation-ready prompt, or a media plan cannot be proven safe, Kronk bypasses IMC and processes the complete request. Examples include media-marker mismatches or a non-text generation tail. A cache bypass is different from rebuilding an eligible stable snapshot.</p>
          <p>An unload or server restart clears the built-in in-memory sessions. Do not rely on IMC sessions surviving model or process lifecycles.</p>
          <p>IMC has several practical costs:</p>
          <ul>
            <li>Planning text reuse requires rendering and tokenizing complete prompts.</li>
            <li>Snapshot and restore operations use host memory bandwidth.</li>
            <li>Edited text rebuilds instead of reusing an arbitrary partial prefix.</li>
            <li>The session pool is finite, so inactive least-recently-used branches can be replaced as new branches arrive.</li>
            <li>MTP can require additional draft-side state. If Kronk restores the target prefix without compatible draft state, it can still use the target cache but disables speculative decoding for that request.</li>
            <li>A corrupt, empty, or partial target snapshot is never treated as a shorter reusable prefix. Kronk invalidates the affected current state and fails that restore so a later request can rebuild safely. An independently retained, compatible System entry remains available; a failed staged media-anchor advance leaves its previously published media snapshot authoritative.</li>
          </ul>
          <p>Evaluate IMC using a representative conversation workload rather than a single prompt benchmark. The benefit grows with reusable prefix length and follow-up frequency.</p>
          <h2 id="57-observability">5.7 Observability</h2>
          <p>IMC planning events identify the selected <code>match_kind</code> (<code>exact</code>, <code>append</code>, or <code>rebuild</code>) and report reusable, extension, stable, and tail token counts. Media planning events also identify <code>media-append</code> and System-preload append decisions. The server emits these through its Info logger; embedded application logging depends on the configured logger. Request-completion events include whether IMC participated and whether a prior snapshot was restored.</p>
          <p>Text preparation emits <code>imc-preparation-queued</code>, <code>imc-preparation-chunk</code>, and <code>imc-cache-ready</code> events. Chunk progress and elapsed time distinguish a large cache build from a request waiting for execution. Media preparation has corresponding media-specific events. Compare these with generation activity and end-to-end request latency when investigating responsiveness.</p>
          <p>The Prometheus counters <code>imc_snapshot_skipped_total</code> and <code>imc_pure_hit_stale_session_total</code> expose exact-hit snapshot skips and rejected stale-session races. A rising rebuild rate usually means clients are changing earlier prompt content, media, tools, or rendering inputs rather than appending to a stable conversation.</p>
          <p>Per-working-entry gauges expose token/allocation values, latest-request input/output/context, peak context, and context-window utilization. The BUI <strong>IMC Sessions</strong> page exposes working sessions and model-owned System caches on separate tabs. These are bounded current-state views, not a history of every conversation transition.</p>
          <p>See <a href="https://www.kronkai.com/manual#chapter-15-observability">Chapter 15</a> for logging, metrics, tracing, and profiling configuration.</p>
          <h2 id="chapter-6-speculative-decoding-and-mtp">Chapter 6: Speculative Decoding and MTP</h2>
          <h3 id="61-what-speculative-decoding-does">6.1 What Speculative Decoding Does</h3>
          <p>Speculative decoding uses a faster drafter to propose several continuation tokens. The target model verifies those proposals together. Accepted proposals reduce the number of target-model passes needed to produce the response; rejected proposals are discarded and the target remains authoritative.</p>
          <p>This optimization does not change the chat, Responses, or SDK request shapes. It can improve generation throughput when the drafter is inexpensive and its proposals agree frequently with the target. It can also reduce performance when draft work is expensive or acceptance is poor. Always measure with the model, sampling settings, hardware, and prompts used in production.</p>
          <p>Kronk supports classic speculative decoding with a separate draft model and Multi-Token Prediction (MTP). MTP uses a prediction head designed for the target rather than a general-purpose smaller language model.</p>
          <p>The target accepts a verified prefix of the proposals and supplies a replacement or bonus token. Rejected proposals do not become authoritative output.</p>
          <p><img src="https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/chapter-06/speculative-decoding.svg" alt="Speculative decoding proposal, target verification, acceptance, and state synchronization" /></p>
          <p>Classic speculative sampling uses the target and draft distributions to decide acceptance and replacement. Kronk's MTP path uses exact token-match verification against the target sampler. Both paths keep the target authoritative and emit an accepted prefix of zero to <code>ndraft</code> candidates followed by a target-derived replacement or bonus token.</p>
          <p>When a request supplies <code>seed</code>, Kronk derives request-local random streams for the target sampler, classic draft sampler, and speculative acceptance and replacement decisions. This prevents concurrent requests from consuming one another's random stream. See <a href="https://www.kronkai.com/manual#chapter-10-request-parameters">Chapter 10</a> for the repeatability contract and its environment constraints.</p>
          <h3 id="62-drafter-sources-and-selection">6.2 Drafter Sources and Selection</h3>
          <p>Kronk can load a drafter from four sources:</p>
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
                <td><strong>Shared-KV companion MTP assistant</strong></td>
                <td>A model-specific assistant GGUF, currently used by Gemma4 models, is discovered with the downloaded target.</td>
                <td>Supports multiple slots</td>
              </tr>
              <tr>
                <td><strong>Own-KV companion MTP head</strong></td>
                <td>A supported MTP GGUF under the repository's <code>MTP/</code> folder, currently used by Qwen3.8-27B, is downloaded with the target.</td>
                <td>Supports multiple slots</td>
              </tr>
              <tr>
                <td><strong>Embedded MTP head</strong></td>
                <td>The target GGUF contains supported head weights, including Qwen3.5, Qwen3.6, Qwen3.8 Flash Next (<code>qwen4exp</code>), and NVIDIA Nemotron 3 Super models.</td>
                <td>Supports multiple slots</td>
              </tr>
            </tbody>
          </table>
          <p>Kronk checks these sources in that order. A <code>draft-model</code> block containing a <code>model-id</code> explicitly selects the classic separate draft and takes precedence over the MTP forms. Without one, Kronk uses a compatible companion MTP file when present, then checks the target for an embedded MTP head. If no source is available, the model runs normally without speculation.</p>
          <p>A <code>draft-model</code> block containing only <code>ndraft</code> is different: it changes the MTP draft ceiling and does not select a classic draft or disable MTP.</p>
          <p>The model-level <code>speculation</code> setting selects the implementation. <code>auto</code> keeps the normal priority order, <code>disabled</code> runs target-only, <code>classic</code> requires a separate draft model, and <code>mtp</code> requires a compatible companion or embedded head. Explicit selection makes it possible to benchmark the same target with MTP enabled and disabled without changing downloaded model files.</p>
          <p>Embedded MTP requires a supported head in the downloaded GGUF files. Qwen3.5-family files that retain MTP metadata after the head weights were stripped are not treated as MTP-capable. Changing <code>ndraft</code> cannot add missing head weights. Recognized metadata alone does not establish runtime support: GLM5 Next currently runs target-only in <code>auto</code> and fails loading with <code>speculation: mtp</code>.</p>
          <p>MTP also requires support from the loaded llama.cpp library. In <code>auto</code> mode, an unavailable MTP API, unsupported recognized runtime, or incompatible embedded head width leaves the model running target-only. With <code>speculation: mtp</code>, these conditions fail model loading instead. Explicit MTP also fails when no compatible source is found or a classic separate draft is configured. Automatic selection does not suppress errors while loading an otherwise selected drafter.</p>
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
          <p>An embedded head requires no companion file. A companion MTP head is an additional model-specific file, but Kronk's catalog and download flow can discover and associate files at the repository root or under <code>MTP/</code> with the target automatically. A version 2 catalog entry can instead provide <code>mtp_url</code> when a compatible companion is hosted in another repository. It is not configured as a classic <code>draft-model</code>.</p>
          <p>Qwen3.8 Flash Next's <code>qwen4exp</code> MTP runtime supports embedded heads and self-contained companion GGUFs through the own-KV runtime. Existing shared-tensor sidecars are not compatible; the companion must contain its own token embeddings and output tensors.</p>
          <p>In <code>auto</code> mode Kronk loads a compatible Qwen4Exp companion when one is available, otherwise it uses a compatible embedded head. The <code>disabled</code> speculation mode continues to run the target without MTP.</p>
          <p>NVIDIA Nemotron 3 Super uses the embedded own-KV MTP runtime for the <code>nemotron_h_moe</code> architecture. Separate Nemotron MTPv2 companion files and Nemotron Puzzle MTP are not supported; configure <code>speculation: disabled</code> to benchmark target-only generation with the same embedded model.</p>
          <p>MTP availability is a property of the downloaded files and the loaded llama.cpp library. Naming a model “MTP” or adding an <code>ndraft</code> override cannot create an MTP head that is not present.</p>
          <h3 id="64-draft-size-and-classic-adaptive-throttling">6.4 Draft Size and Classic Adaptive Throttling</h3>
          <p><code>ndraft</code> is the maximum number of candidates the drafter attempts in one round. Larger values can save more target passes when acceptance remains high, but they also increase wasted draft and verification work when proposals are rejected.</p>
          <p>Defaults are:</p>
          <ul>
            <li><strong>Classic separate draft:</strong> 5</li>
            <li><strong>MTP:</strong> 3</li>
          </ul>
          <p>For MTP, one <strong>round</strong> is a complete speculative cycle: MTP proposes up to <code>ndraft</code> candidates, the target verifies them together, and Kronk emits the accepted prefix followed by one target-selected replacement or bonus token. A round therefore emits between 1 and <code>ndraft + 1</code> tokens; a round is not the same as a token.</p>
          <p>MTP always uses its configured <code>ndraft</code> as the per-round maximum. Kronk does not evaluate acceptance to try lower values, run calibration rounds, or select a different value at runtime. An omitted or zero value uses the MTP default of 3; an explicit positive value remains the ceiling for the loaded model. A round can propose fewer candidates near context or output limits or when drafting reaches an end condition.</p>
          <p>Prompt type, sampling settings, and output structure can change MTP acceptance and the amount of useful work produced by each round. Coding, summarization, prose, and structured output can therefore perform differently at the same <code>ndraft</code>. If a workload performs better with another value, benchmark that representative workload and configure the value explicitly. Compare end-to-end throughput and latency rather than selecting a value from acceptance rate alone.</p>
          <p>Classic separate-draft speculation does adapt its draft size directly from its acceptance EMA:</p>
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
          <p>Classic acceptance EMA also belongs to the execution slot and resets at the start of each request, so acceptance history from an unrelated request does not affect the next one.</p>
          <h3 id="65-configuration">6.5 Configuration</h3>
          <p>Configuration belongs under the target model ID in <code>~/.kronk/models/model_config.yaml</code>.</p>
          <h4 id="651-classic-separate-draft">6.5.1 Classic separate draft</h4>
          <pre className="code-block"><code className="language-yaml">{`version: 1
models:
  some-provider/target-model:
    nseq-max: 1
    draft-model:
      model-id: some-provider/compatible-draft-model
      ndraft: 5`}</code></pre>
          <p>The target and draft must already be downloaded. Kronk resolves the configured draft model ID to its local files. A classic draft with <code>nseq-max</code> greater than 1 is rejected during configuration validation.</p>
          <h4 id="652-mtp-default">6.5.2 MTP default</h4>
          <p>No model configuration is required. Downloading a supported target and its catalog-provided companion files is sufficient for automatic detection.</p>
          <h4 id="653-mtp-draft-count-override">6.5.3 MTP draft-count override</h4>
          <p>To change the MTP draft ceiling, set <code>ndraft</code> without a <code>model-id</code>:</p>
          <pre className="code-block"><code className="language-yaml">{`version: 1
models:
  some-provider/mtp-target-model:
    draft-model:
      ndraft: 6`}</code></pre>
          <p>This form supports multiple slots. A value of 0 or an omitted value uses the MTP default of 3; a negative value is rejected. If neither a compatible companion nor an embedded MTP head is available, the override has no effect.</p>
          <p>See <a href="https://www.kronkai.com/manual#chapter-3-model-configuration">Chapter 3</a> for the complete model configuration format.</p>
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
          <p>Final <code>chat-completion</code> and batch-engine completion logs use <code>draft_acceptance_rate</code>. Both report draft-token counts, accepted-token counts, and draft coverage when a drafter is loaded, including zero counts when speculation did not run. Common MTP disable reasons include:</p>
          <ul>
            <li><code>imc-hit</code> — target cache state was restored without compatible draft state</li>
            <li><code>sync-error</code> — draft state could not be synchronized after target decoding, including media prefill or verification</li>
          </ul>
          <p>Startup events under <code>draft-model</code>, <code>draft-model-mtp</code>, <code>draft-model-mtp-separate</code>, <code>draft-model-mtp-shared</code>, and <code>speculation</code> show which source loaded or why MTP was skipped. See <a href="https://www.kronkai.com/manual#chapter-15-observability">Chapter 15</a> for logging and metrics configuration.</p>
          <h3 id="67-limitations-and-fallbacks">6.7 Limitations and Fallbacks</h3>
          <ul>
            <li><strong>Classic separate drafts require one slot.</strong> Set <code>nseq-max: 1</code> on the target entry.</li>
            <li><strong>Some sampling controls bypass classic speculation.</strong> Requests using grammar, adaptive-p, DRY, frequency/presence penalties, a nondefault repeat penalty, or XTC run target-only. This restriction applies to classic separate drafts, not MTP.</li>
            <li><strong>Tokenizer compatibility remains the user's responsibility.</strong> Kronk rejects unequal vocabulary sizes, but that check cannot establish identical token mappings or templates.</li>
            <li><strong>MTP at nonzero temperature is an approximation.</strong> MTP proposals are greedy, while target verification uses the request's sampler and accepts exact token matches. Sampling parameters still shape output, but this does not provide strict speculative-sampling distribution equivalence.</li>
            <li><strong>MTP can fall back per request.</strong> A synchronization or compatible-state problem disables MTP for the affected request while target-only generation continues. It does not make an incorrect draft token authoritative. A target rollback or restore failure is different: Kronk fails the affected request rather than continuing from ambiguous recurrent state.</li>
            <li><strong>IMC may restore target state without draft state.</strong> Target-prefix reuse remains valid. Shared-target-KV MTP resumes from the restored target state. Own-KV MTP resumes only when Kronk can restore compatible draft state; if that state is absent or invalid, the request runs target-only rather than drafting from inconsistent state. Text System cache entries retain this paired state when it is valid. See <a href="https://www.kronkai.com/manual#chapter-5-message-caching">Chapter 5</a>.</li>
            <li><strong>MTP supports media input, including M-RoPE images.</strong> Compatible draft state can also be retained for IMC reuse. Incompatible draft input widths or failed synchronization fall back to target-only execution for that request. Classic separate drafts still cannot reconstruct media prefixes. See <a href="https://www.kronkai.com/manual#chapter-11-multimodal-models">Chapter 11</a>.</li>
            <li><strong>Drafting consumes resources.</strong> A classic draft loads another model and KV cache. MTP heads and companion assistants also require compute and memory. Hybrid recurrent targets retain current state plus rollback copies sized by the MTP draft ceiling. Automatic detection does not guarantee a performance improvement.</li>
          </ul>
          <p>Implementation details for drafting, verification, state synchronization, and hybrid-model rollback belong in <a href="https://www.kronkai.com/manual#2066-speculative-decoding-and-mtp">Chapter 20</a>.</p>
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
          <p>The <a href="https://huggingface.co/Qwen/Qwen3-8B#processing-long-texts">Qwen3-8B model documentation</a> identifies 32,768 tokens as the native context and reports validation up to 131,072 tokens with YaRN. It recommends matching the scale to the context actually needed. These examples apply to a downloaded Qwen3-8B GGUF; use its local model ID in place of <code>owner/Qwen3-8B-GGUF</code>.</p>
          <p>For a 2× extension to 65,536 tokens:</p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/models/model_config.yaml
version: 1
models:
  owner/Qwen3-8B-GGUF:
    context-window: 65536
    rope-scaling-type: yarn
    rope-freq-scale: 0.5
    yarn-orig-ctx: 32768`}</code></pre>
          <p>For a 4× extension to 131,072 tokens:</p>
          <pre className="code-block"><code className="language-yaml">{`# ~/.kronk/models/model_config.yaml
version: 1
models:
  owner/Qwen3-8B-GGUF:
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
                <td>Leaves the model/native RoPE scaling method unchanged. It does not force scaling off or prevent an oversized context window.</td>
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
          <p>Use the provider's instructions when comparing scaled and unscaled operation. Setting <code>rope-scaling-type: none</code> alone does not disable scaling already declared by the GGUF.</p>
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
          <p>KV-cache capacity grows approximately linearly with the context window. The actual size depends on the model architecture, layer count, KV heads, head dimensions, cache data types, backend, and alignment. With multiple generation slots, aggregate capacity is based on <code>context-window × nseq-max</code>. Shared KV storage does not imply fixed physical partitions per slot; <code>context-window</code> still limits an individual request.</p>
          <p>If a long-context model does not fit, consider:</p>
          <ul>
            <li>reducing <code>nseq-max</code>;</li>
            <li>selecting supported quantized KV-cache types; or</li>
            <li>keeping the KV cache on the CPU with <code>offload-kqv: false</code> to relieve discrete GPU memory, at a likely performance cost. On unified-memory systems this does not remove the cache from the shared physical memory budget.</li>
          </ul>
          <p>Do not rely on fixed memory figures from another model. Use Kronk's hardware analysis and observe actual memory consumption. See <a href="https://www.kronkai.com/manual#chapter-3-model-configuration">Chapter 3</a> for KV-cache configuration and <a href="https://www.kronkai.com/manual#chapter-4-batch-processing">Chapter 4</a> for concurrency effects.</p>
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
          <p>The Kronk model server provides OpenAI-compatible inference APIs and manages downloaded models, native libraries, and loaded model instances. This chapter focuses on operating that server. Installation is covered in <a href="https://www.kronkai.com/manual#chapter-2-installation-quick-start">Chapter 2</a>, while model-level tuning belongs in <a href="https://www.kronkai.com/manual#chapter-3-model-configuration">Chapter 3</a>.</p>
          <h2 id="81-server-lifecycle">8.1 Server Lifecycle</h2>
          <p>Start the server in the foreground:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start`}</code></pre>
          <p>The API listens on <code>127.0.0.1:11435</code> by default and is reachable only from the local machine. To expose it on another interface, set <code>--api-host</code> explicitly, restrict access with a firewall or private network, and configure the authorization described in <a href="https://www.kronkai.com/manual#chapter-12-security-and-authentication">Chapter 12</a>.</p>
          <p>To bind only to the local machine:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --api-host=127.0.0.1:11435`}</code></pre>
          <p>Run the server in the background with:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --detach`}</code></pre>
          <p>Detached mode records the process ID and redirects server output to these paths by default:</p>
          <pre className="code-block"><code className="language-text">{`~/.kronk/kronk.pid
~/.kronk/kronk.log`}</code></pre>
          <p>Setting <code>KRONK_BASE_PATH</code> before starting the detached server selects that root for both files; existing files are not moved.</p>
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
                <td><code>127.0.0.1:11435</code></td>
                <td>Main API bind address</td>
              </tr>
              <tr>
                <td><code>--debug-host</code></td>
                <td><code>KRONK_WEB_DEBUG_HOST</code></td>
                <td><code>127.0.0.1:11445</code></td>
                <td>Metrics and profiling bind address</td>
              </tr>
              <tr>
                <td><code>--inference-timeout</code></td>
                <td><code>KRONK_WEB_INFERENCE_TIMEOUT</code></td>
                <td><code>60m</code></td>
                <td>Total timeout for inference admission, preparation, slot waiting, and generation</td>
              </tr>
              <tr>
                <td><code>--write-timeout</code></td>
                <td><code>KRONK_WEB_WRITE_TIMEOUT</code></td>
                <td><code>61m</code></td>
                <td>HTTP response write timeout; <code>0</code> disables it, otherwise it must exceed the inference timeout</td>
              </tr>
              <tr>
                <td><code>--base-path</code></td>
                <td><code>KRONK_BASE_PATH</code></td>
                <td><code>~/.kronk</code></td>
                <td>Root for Kronk data</td>
              </tr>
              <tr>
                <td><code>--lib-path</code></td>
                <td><code>KRONK_LIB_PATH</code></td>
                <td>Detected bundle under <code>&lt;base&gt;/libraries</code></td>
                <td>Exact llama.cpp library directory</td>
              </tr>
              <tr>
                <td><code>--lib-version</code></td>
                <td><code>KRONK_LIB_VERSION</code></td>
                <td>unset</td>
                <td>llama.cpp release, optionally pinned as <code>VERSION@sha256:&lt;64-hex-digest&gt;</code></td>
              </tr>
              <tr>
                <td><code>--lib-verify-enabled</code></td>
                <td><code>KRONK_LIB_VERIFY_ENABLED</code></td>
                <td><code>true</code></td>
                <td>Verify selected llama.cpp, whisper.cpp, and stable-diffusion.cpp bundles before loading them</td>
              </tr>
              <tr>
                <td><code>--processor</code></td>
                <td><code>KRONK_PROCESSOR</code></td>
                <td>Detected host backend</td>
                <td>Processor bundle such as <code>metal</code>, <code>cuda</code>, <code>rocm</code>, <code>vulkan</code>, or <code>cpu</code></td>
              </tr>
              <tr>
                <td><code>--arch</code></td>
                <td><code>KRONK_ARCH</code></td>
                <td>Host architecture</td>
                <td>Library-bundle architecture override</td>
              </tr>
              <tr>
                <td><code>--os</code></td>
                <td><code>KRONK_OS</code></td>
                <td>Host operating system</td>
                <td>Library-bundle operating-system override</td>
              </tr>
              <tr>
                <td><code>--bucky-lib-path</code></td>
                <td><code>KRONK_BUCKY_LIB_PATH</code></td>
                <td>Detected bundle under <code>&lt;base&gt;/bucky-libraries</code></td>
                <td>Exact whisper.cpp library directory</td>
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
                <td><code>90</code></td>
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
                <td><code>0m</code></td>
                <td>Idle model retention time; <code>0</code> disables idle expiration</td>
              </tr>
              <tr>
                <td><code>--web-admin-enabled</code></td>
                <td><code>KRONK_WEB_ADMIN_ENABLED</code></td>
                <td><code>true</code></td>
                <td>Serve the BUI under <code>/admin/</code></td>
              </tr>
              <tr>
                <td><code>--authorization-mode</code></td>
                <td><code>KRONK_AUTHORIZATION_MODE</code></td>
                <td><code>open</code></td>
                <td>Select the API access policy</td>
              </tr>
              <tr>
                <td><code>--download-enabled</code></td>
                <td><code>KRONK_DOWNLOAD_ENABLED</code></td>
                <td><code>false</code></td>
                <td>Share this server's models and library bundles with other Kronk servers on the network (peer downloads)</td>
              </tr>
              <tr>
                <td><code>--lib-download-enabled</code></td>
                <td><code>KRONK_LIB_DOWNLOAD_ENABLED</code></td>
                <td><code>true</code></td>
                <td>Download or update the llama.cpp library during server startup</td>
              </tr>
              <tr>
                <td><code>--media-backends-enabled</code></td>
                <td><code>KRONK_MEDIA_BACKENDS_ENABLED</code></td>
                <td><code>true</code></td>
                <td>Download and load the Bucky and Malina native backends during server startup</td>
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
                <td><code>0</code></td>
                <td>Enable or disable llama.cpp logging</td>
              </tr>
            </tbody>
          </table>
          <p>Most server configuration flags map to environment variables, but names follow the server's configuration hierarchy rather than a universal text conversion. For example, <code>--budget-percent</code> maps to <code>KRONK_POOL_BUDGET_PERCENT</code>. <code>--detach</code> is a CLI process-control flag and has no environment equivalent.</p>
          <p>Run the following for the complete current list, including HTTP timeouts, CORS, tracing, external authentication, processor selection, and library overrides:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --help`}</code></pre>
          <p>Keep tokens and passwords in protected environment or secret-manager settings, not shared shell scripts. <code>--insecure-logging</code> can expose prompts and model configuration and should be limited to controlled debugging.</p>
          <h3 id="runtime-selection-and-overrides">Runtime selection and overrides</h3>
          <p>With no processor or library-path override, server startup detects a preferred llama.cpp backend and verifies it before native libraries are loaded. CUDA 13 requires compute capability 7.5 or newer on every visible NVIDIA GPU. Older or hidden CUDA devices cause host detection to prefer an available ROCm or Vulkan runtime and then CPU. If compatibility cannot be determined conclusively, Kronk retains the preferred backend.</p>
          <p>Kronk also probes the installed preferred accelerator bundle. It changes to a different installed bundle only when the preferred bundle positively reports no accelerator and a same-version alternative positively reports a device. This probe never installs another bundle. The startup log records the preferred and selected processors and the reason for the decision.</p>
          <p><code>--processor</code> and <code>--lib-path</code>, or their environment equivalents, are strict operator choices and disable automatic fallback. A custom non-empty library directory without <code>version.json</code> is treated as a read-only user-managed build; Kronk does not upgrade or replace it. To use such a build, explicitly disable automatic llama.cpp downloading and managed-bundle verification with <code>--lib-download-enabled=false --lib-verify-enabled=false</code>. Default startup checks otherwise fail because managed installation metadata is absent. <code>--base-path</code> selects the root for managed libraries and other Kronk data; it does not migrate existing data. <code>--lib-path</code> selects the llama.cpp location specifically. Restart after changing native library selection settings.</p>
          <p>Set <code>--lib-download-enabled=false</code> or <code>KRONK_LIB_DOWNLOAD_ENABLED=false</code> to skip the automatic llama.cpp download at startup. Kronk still loads the library selected by <code>--lib-path</code> or <code>KRONK_LIB_PATH</code>.</p>
          <p>Set <code>--media-backends-enabled=false</code> or <code>KRONK_MEDIA_BACKENDS_ENABLED=false</code> to skip automatic Bucky and Malina bundle downloads and native backend initialization. This does not affect llama.cpp download or initialization.</p>
          <p>Library verification is enabled by default and applies to llama.cpp and the enabled Bucky and Malina native backends. Kronk verifies a runtime before its device probe or native initialization. Installation or verification failures normally stop server startup; native initialization failures after verification can leave the affected backend unavailable while the server continues. For llama.cpp releases without per-file digests, Kronk logs that post-install verification is unavailable and can continue. Set <code>--lib-verify-enabled=false</code> or <code>KRONK_LIB_VERIFY_ENABLED=false</code> to disable verification; do this only for libraries you independently trust.</p>
          <p>For an externally pinned llama.cpp check, set <code>--lib-version</code> or <code>KRONK_LIB_VERSION</code> to <code>VERSION@sha256:&lt;64-hex-digest&gt;</code>. The digest identifies Yzma's release manifest, which identifies the platform archives and their installed files. The default llama.cpp and whisper.cpp versions already include their published manifest digests. The builders publish each manifest as a release asset, GitHub records its SHA-256, and Kronk carries that digest as the trust anchor. Yzma and Bucky validate the manifest and selected archive during download; startup verification then hashes the extracted files against the authenticated manifest. Post-install verification requires an install record and per-file hashes in the release manifest.</p>
          <p>Bucky performs separate whisper.cpp runtime selection. Its managed bundles live below <code>&lt;base&gt;/bucky-libraries</code>, and <code>KRONK_BUCKY_LIB_PATH</code> authoritatively selects a different bundle or user-managed build. It does not use <code>--lib-path</code>/<code>KRONK_LIB_PATH</code>. See <a href="https://www.kronkai.com/manual#182-install-whisper-libraries">Chapter 18 §18.2</a> for Bucky's CUDA, Vulkan, and CPU fallback rules.</p>
          <h2 id="84-model-pool-and-resource-budgets">8.4 Model Pool and Resource Budgets</h2>
          <p>Kronk keeps loaded models in memory to avoid paying model-load latency on every request. Three settings govern retention:</p>
          <ul>
            <li><code>budget-percent</code> controls memory admission.</li>
            <li><code>models-in-pool</code> places a count limit on each backend pool.</li>
            <li><code>pool-ttl</code> unloads entries that remain unused past the configured duration. Set it to <code>0</code> to keep loaded models resident regardless of inactivity. This does not prevent eviction caused by the pool-count limit, memory pressure, explicit unload, or server shutdown.</li>
          </ul>
          <p>At the default <code>budget-percent: 90</code>, each discrete GPU receives a 90% budget minus 256 MiB of headroom. Host RAM receives an 85% budget because Kronk reserves an additional five percentage points for the operating system, allocators, and memory not represented in model estimates. Apple Silicon unified memory is accounted as one host-memory pool rather than independent RAM and Metal VRAM.</p>
          <p>Admission uses predicted model, KV-cache, and runtime memory. These predictions are planning estimates, not a guarantee that every backend allocation will succeed. Context size, cache types, sequence count, CPU offload, and model architecture all affect the estimate.</p>
          <p>After a model and its target, draft/MTP, and projection contexts are initialized, Kronk checks the backend's live free-memory report before publishing the model to the pool. CUDA and ROCm loads that no longer retain the configured headroom are unloaded and rejected with an insufficient-memory error. Metal and Vulkan checks are advisory: Metal reports a recommended working-set limit rather than physical unified-memory capacity, while Vulkan drivers without the memory-budget extension can report the entire heap as free. CPU backends do not expose a portable live-memory value and skip this check.</p>
          <p>On multi-GPU systems, Kronk accounts for llama.cpp's model distribution across the selected devices. Automatic splits use available GPUs, while explicit <code>devices</code> and <code>tensor-split</code> configuration control the proportions. Each assigned share must fit within that GPU's individual budget; unused capacity on another card cannot satisfy an over-budget share.</p>
          <p>For a load that can fit the configured budget, Kronk can evict one or more idle entries from the requesting backend's pool to satisfy its count or memory limit. For memory pressure it prefers an entry that frees enough memory without unloading a needlessly large model, then falls back to the coldest idle entry. Models with active streams are not evicted. If no idle entry in that pool can make room, the request returns server busy. A load that exceeds the entire applicable budget is rejected without evicting existing entries; reduce its sizing or change the budget rather than retrying unchanged.</p>
          <p>The LLM, Bucky, and Malina pools share one byte budget, but each has its own count limit and eviction candidates. An idle model in another backend's pool is not automatically evicted to admit the request; explicitly unload it or allow its TTL to expire when necessary. Bucky installation and pool behavior are covered in <a href="https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription">Chapter 18</a>.</p>
          <p>Resource usage and eviction events are available through the logging and metrics described in <a href="https://www.kronkai.com/manual#chapter-15-observability">Chapter 15</a>.</p>
          <h2 id="85-model-configuration-files">8.5 Model Configuration Files</h2>
          <p>The server reads per-model overrides from:</p>
          <pre className="code-block"><code className="language-text">{`~/.kronk/models/model_config.yaml`}</code></pre>
          <p>Kronk seeds the file on first use and preserves edits across upgrades. Entries are merged over hardware-analysis recommendations rather than replacing the entire runtime configuration.</p>
          <p>Version 2 makes this the single configuration file for the server and all model backends. Version 1 files containing only <code>kms</code> and <code>models</code> remain readable. The top-level shape is:</p>
          <pre className="code-block"><code className="language-yaml">{`version: 2
kms:
  web:
    api-host: 127.0.0.1:11435
    debug-host: 127.0.0.1:11445
    read-timeout: 30s
    write-timeout: 61m
    inference-timeout: 60m
    idle-timeout: 1m
    shutdown-timeout: 1m
    cors-allowed-origins: ["*"]
    admin:
      enabled: true
      session-duration: 12h
      password-sha-256: 18511e63760230cd17291273b607e7e13da2a2bb9a1750e0becdac08185a3c11
  auth:
    host: ""
    tls:
      enabled: false
      ca-file: ""
      server-name: ""
    local:
      issuer: kronk project
  authorization:
    mode: open # open, management, authenticated, or full-protected
  mcp:
    enabled: true
    host: ""
    auth-enabled: false
    brave-api-key: ""
  download:
    enabled: false
  tempo:
    host: localhost:4317
    service-name: kronk
    probability: 0.25
  pool:
    budget-percent: 90
    models-in-pool: 10
    ttl: 0m
  base-path: ""
  lib-path: ""
  bucky-lib-path: ""
  malina-lib-path: ""
  lib-version: ""
  lib-download-enabled: true
  lib-verify-enabled: true
  media-backends-enabled: true
  arch: ""
  os: ""
  processor: ""
  allow-upgrade: false
  insecure-logging: false
  hf-token: ""
  llama-log: 0
models:
  owner/model:
    context-window: 8192
    nseq-max: 2
bucky-models:
  tiny:
    nseq-max: 2
malina-models:
  sd-1.5:
    concurrency: 2`}</code></pre>
          <p>The built-in defaults apply when a <code>kms</code> key is omitted. Configuration precedence is built-in defaults, then <code>kms</code> YAML, then <code>KRONK_*</code> environment variables, then explicitly supplied <code>kronk server start</code> flags. The <code>--model-config-file</code> flag and <code>KRONK_POOL_MODEL_CONFIG_FILE</code> environment variable select the YAML file itself and therefore remain outside the file. The default file location is resolved from the command-line/environment base path before YAML is read; setting <code>kms.base-path</code> inside the file does not select a different configuration file. The backend sections are top-level mappings alongside <code>models</code>. Bucky keys are the short IDs shown by <code>kronk bucky model list</code>; Malina keys are bundle IDs shown by <code>kronk malina model list</code>. Restart the server after changing them.</p>
          <p>Automatic tuning is enabled by default in the model server. An explicit <code>context-window</code> or <code>nseq-max</code> in this file is treated as a fixed sizing constraint, not a value the tuner may reduce. When KV cache types are omitted, the server tries <code>f16</code> and then <code>q8_0</code> for that exact context and concurrency; it never automatically quantizes below <code>q8_0</code>. See <a href="https://www.kronkai.com/manual#32-automatic-tuning">Chapter 3 §3.2</a> for the complete selection order and recommended configuration style.</p>
          <p>Use another file without replacing the default:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --model-config-file=./my-model_config.yaml`}</code></pre>
          <p>or:</p>
          <pre className="code-block"><code className="language-shell">{`KRONK_POOL_MODEL_CONFIG_FILE=./my-model_config.yaml kronk server start`}</code></pre>
          <p>The file format, variants, configuration keys, and tuning workflow are documented in <a href="https://www.kronkai.com/manual#chapter-3-model-configuration">Chapter 3</a>. The repository's commented reference file is <code>zarf/kms/model_config.yaml</code>.</p>
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
          <p>Catalog schema version 2 adds the optional <code>mtp_url</code> field. Set it to a fully qualified Hugging Face download URL when a compatible MTP companion lives in a different repository from its target model. When it is absent, Kronk retains the version 1 behavior and builds the companion URL from the entry's provider, family, revision, and <code>mtp_orig</code> fields. Version 1 catalogs remain readable and are stamped as version 2 the next time the current Kronk version rewrites them; their entries do not require a data migration.</p>
          <p>Source specificity controls catalog resolution:</p>
          <ul>
            <li>A canonical <code>provider/modelID</code> selects one provider explicitly.</li>
            <li><code>provider/repo:quantization</code> pins the repository and selects the matching quantization there.</li>
            <li><code>owner/repo/file.gguf</code>, or an equivalent Hugging Face <code>blob</code> or <code>resolve</code> URL, pins both the repository and upstream filename.</li>
            <li>A repository root or tree URL returns its GGUF files for explicit selection rather than choosing one automatically.</li>
          </ul>
          <p>Exact file pins never fall back to a different repository or to another catalog entry with the same quantization suffix. This matters when target and MTP drafter files have similar names: companion discovery remains inside the pinned repository, preventing a cached target, drafter, or unrelated model from being substituted. The resulting canonical ID can still reflect Kronk's normal on-disk rename rules, such as the <code>mtp-</code> prefix for a dedicated MTP repository.</p>
          <p>Use <code>--local</code> for the same operations when the server is stopped. The BUI also provides catalog and model views when enabled; see <a href="https://www.kronkai.com/manual#chapter-13-browser-ui-bui">Chapter 13</a>.</p>
          <h2 id="87-container-operations">8.7 Container Operations</h2>
          <p>Chapter 2 covers image variants and initial container startup. For a persistent deployment, use a versioned image tag and retain <code>/kronk</code> in a volume. This headless example fully protects the API and exposes it only through the host loopback interface:</p>
          <pre className="code-block"><code className="language-shell">{`docker run -d \\
  --name kronk \\
  --restart unless-stopped \\
  -e KRONK_PROCESSOR=cpu \\
  -e KRONK_AUTHORIZATION_MODE=full-protected \\
  -e KRONK_WEB_ADMIN_ENABLED=false \\
  -p 127.0.0.1:11435:11435 \\
  -v kronk-data:/kronk \\
  ghcr.io/ardanlabs/kronk:vX.Y.Z-cpu`}</code></pre>
          <p>Choose the processor-specific tag documented in Chapter 2. Terminate TLS at a reverse proxy or keep the service on a trusted private network. Read <a href="https://www.kronkai.com/manual#chapter-12-security-and-authentication">Chapter 12</a> before exposing an authenticated server remotely.</p>
          <p>The image sets the container API listener to <code>0.0.0.0:11435</code>, peer downloads off, and insecure logging off through environment variables. These override the corresponding YAML settings; host exposure is controlled separately by the published port above.</p>
          <p>On first initialization Kronk creates an administrator token at <code>/kronk/keys/master.jwt</code>. Capture it without printing it or enabling shell tracing:</p>
          <pre className="code-block"><code className="language-shell">{`set +x
KRONK_TOKEN=$(docker exec kronk cat /kronk/keys/master.jwt)`}</code></pre>
          <p>Send <code>Authorization: Bearer $KRONK_TOKEN</code> with protected API requests. Keep the token private and run <code>unset KRONK_TOKEN</code> when finished. This bootstrap also works with the Compose container below; do not disable authentication to obtain a token. See Chapter 12 for token management and scoped credentials.</p>
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
          <p><strong>Container resource limits</strong></p>
          <p>Container limits and Kronk's model budgets serve different purposes. Docker and Kubernetes enforce CPU and host-memory limits; Kronk's <code>budget-percent</code> controls model admission, not a hard limit on the whole server process. Leave room for the server, native libraries, temporary allocations, and other work beyond model weights and KV caches. A model fitting Kronk's estimated budget does not guarantee that the container fits its RAM limit.</p>
          <ul>
            <li>CPU quotas can throttle inference and increase latency. Size thread counts and concurrency for the CPU capacity assigned to the container, not the host's total core count.</li>
            <li>A hard RAM limit can cause an OOM kill rather than a recoverable API error. Reducing context size, concurrent sequences, or resident models can lower memory demand. Inspect container memory usage and restarts under realistic load before rollout.</li>
            <li>CPU/RAM limits do not cap GPU VRAM. Device access such as <code>--gpus all</code> is not a per-container VRAM quota or an exclusive reservation. Account for other GPU workloads and leave VRAM headroom; do not treat <code>budget-percent</code> as isolation from other processes.</li>
            <li>With Docker Desktop, the Linux VM's CPU and memory allocation also bounds what containers can use. Increasing a container limit cannot increase the VM's available resources.</li>
          </ul>
          <p>For example, add <code>--cpus=2 --memory=8g</code> to the <code>docker run</code> command to limit it to two CPUs' worth of execution time and 8 GiB of host memory. These are illustrative limits, not sizing recommendations for every model. For Compose, add the equivalent settings to the same override file used for server settings:</p>
          <pre className="code-block"><code className="language-yaml">{`services:
  kronk:
    cpus: 2.0
    mem_limit: 8g`}</code></pre>
          <p>For Helm, set <code>resources.requests</code> and <code>resources.limits</code> in your values file. Requests guide scheduling; limits constrain runtime use. A GPU device request does not by itself specify a VRAM byte limit. See §8.4 for model budgets and Chapter 3 for context and concurrency tuning.</p>
          <p><strong>NVIDIA Jetson: build locally</strong></p>
          <p>Jetson is not part of the published image matrix: there is no automatically published <code>ghcr.io/ardanlabs/kronk:jetson</code> tag. The optional <code>runtime-jetson</code> target requires a Jetson compatible with JetPack 6 / L4T r36 and NVIDIA Container Runtime. Build natively on that device from a reviewed repository checkout, using a local image name:</p>
          <pre className="code-block"><code className="language-shell">{`docker buildx build --load \\
  --platform linux/arm64 \\
  --build-arg LLAMA_PROCESSORS=cuda \\
  --build-arg BUCKY_PROCESSORS=cuda \\
  --target runtime-jetson \\
  -t kronk-jetson:local \\
  -f zarf/docker/kronk/Dockerfile .

docker run -d \\
  --name kronk \\
  --restart unless-stopped \\
  --runtime=nvidia \\
  -e NVIDIA_DRIVER_CAPABILITIES=compute,utility,graphics \\
  -p 127.0.0.1:11435:11435 \\
  -v kronk-data:/kronk \\
  kronk-jetson:local`}</code></pre>
          <p>Use the authentication settings above for a protected deployment. To upgrade, build again from the desired release checkout, then stop and remove the old container and repeat the run command, retaining <code>kronk-data</code>. Validate inference on the actual device; a successful build or liveness response alone does not verify GPU inference.</p>
          <p><strong>Docker Compose</strong></p>
          <p>The Compose files in <a href="https://github.com/ardanlabs/kronk/tree/main/zarf/docker/kronk"><code>zarf/docker/kronk</code></a> use the same container name (<code>kronk</code>) and volume (<code>kronk-data</code>) as the examples above, so the <code>docker exec</code> and <code>docker logs</code> commands work unchanged. Start with <a href="https://www.kronkai.com/manual#23-container-quick-start">2.3 Container Quick Start</a>. All <code>docker compose</code> commands run from the directory holding the files.</p>
          <p>Without a repository checkout, download only the Compose files. Replace <code>main</code> with the release tag of the image you pin:</p>
          <pre className="code-block"><code className="language-shell">{`mkdir kronk && cd kronk
for f in compose.yaml compose.cuda.yaml compose.rocm.yaml compose.vulkan.yaml; do
  curl -fsSLO "https://raw.githubusercontent.com/ardanlabs/kronk/main/zarf/docker/kronk/$f"
done`}</code></pre>
          <p>These variables configure the Compose files. Set them in the shell or in a <code>.env</code> file in the Compose directory:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Variable</th>
                <th>Default</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>KRONK_IMAGE_VERSION</code></td>
                <td><code>latest</code></td>
                <td><code>latest</code> or a release such as <code>vX.Y.Z</code>; pin a release outside local testing</td>
              </tr>
              <tr>
                <td><code>KRONK_BIND_ADDR</code></td>
                <td><code>127.0.0.1</code></td>
                <td>Host address for ports <code>11435</code> and <code>11445</code></td>
              </tr>
              <tr>
                <td><code>KRONK_DOWNLOAD_ENABLED</code></td>
                <td><code>false</code></td>
                <td>Share models and library bundles with other Kronk servers (peer downloads)</td>
              </tr>
              <tr>
                <td><code>KRONK_RENDER_GID</code> / <code>KRONK_VIDEO_GID</code></td>
                <td><code>render</code> / <code>video</code></td>
                <td>Host group IDs for GPU devices (ROCm, Vulkan)</td>
              </tr>
              <tr>
                <td><code>KRONK_TEMPO_HOST</code></td>
                <td>unset</td>
                <td>OTLP endpoint, e.g. <code>tempo:4317</code></td>
              </tr>
            </tbody>
          </table>
          <p>Only set <code>KRONK_BIND_ADDR=0.0.0.0</code> on a trusted network: port <code>11445</code> serves pprof and metrics without authentication, and Docker's published ports are not blocked by host firewalls such as ufw.</p>
          <p>Do not put server settings in <code>.env</code>. Compose uses <code>.env</code> only for the variables that the Compose files refer to, such as those in the table above. Any other variable in <code>.env</code> never reaches the container, and Compose gives no warning. For example, <code>KRONK_AUTHORIZATION_MODE=full-protected</code> in <code>.env</code> leaves the server unprotected. Pass server settings, such as the authentication settings above, through a <code>compose.override.yaml</code> next to the Compose files:</p>
          <pre className="code-block"><code className="language-yaml">{`services:
  kronk:
    environment:
      - KRONK_AUTHORIZATION_MODE=full-protected
      - KRONK_WEB_ADMIN_ENABLED=false`}</code></pre>
          <p>A plain <code>docker compose up -d</code> loads <code>compose.override.yaml</code> automatically. With <code>-f</code> or <code>COMPOSE_FILE</code>, list it last. To avoid repeating <code>-f</code>, create a <code>.env</code> such as this one, which selects Vulkan, loads the override, and pins the image (the separator line makes it work on Windows too):</p>
          <pre className="code-block"><code className="language-text">{`COMPOSE_PATH_SEPARATOR=:
COMPOSE_FILE=compose.vulkan.yaml:compose.override.yaml
KRONK_IMAGE_VERSION=vX.Y.Z`}</code></pre>
          <p>In a repository checkout, <code>make kronk-up KRONK_GPU=vulkan</code> (or <code>cuda</code>, <code>rocm</code>) starts the same from the repository root and adds the override when it exists.</p>
          <p>The GPU files need these host components:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>File</th>
                <th>Host requirements</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>compose.cuda.yaml</code></td>
                <td>NVIDIA driver and <a href="https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html">NVIDIA Container Toolkit</a></td>
              </tr>
              <tr>
                <td><code>compose.rocm.yaml</code></td>
                <td><code>linux/amd64</code>, ROCm-capable kernel driver, <code>/dev/kfd</code> and <code>/dev/dri</code></td>
              </tr>
              <tr>
                <td><code>compose.vulkan.yaml</code></td>
                <td>Mesa Vulkan drivers, <code>/dev/dri</code></td>
              </tr>
            </tbody>
          </table>
          <ul>
            <li>Check the backend in use with <code>docker compose logs kronk | grep "installing/updating libraries"</code>; the <code>processor</code> field shows it. <code>compose.yaml</code> sets <code>KRONK_PROCESSOR=cpu</code>, because the image's software Vulkan driver would otherwise make detection pick <code>vulkan</code> without a GPU.</li>
            <li>NVIDIA: to use specific cards, replace <code>count: all</code> in <code>compose.cuda.yaml</code> with <code>device_ids: ["0"]</code> (indices or UUIDs from <code>nvidia-smi -L</code>).</li>
            <li>ROCm: <code>HIP_VISIBLE_DEVICES</code> and <code>HSA_OVERRIDE_GFX_VERSION</code> are passed through. For an unlisted GPU that is not detected, try <code>HSA_OVERRIDE_GFX_VERSION=10.3.0</code> (RDNA2) or <code>11.0.0</code> (RDNA3). The ROCm image is about 30 GB unpacked; on integrated GPUs, Vulkan is usually smaller and faster.</li>
            <li>Vulkan: <code>VK_LOADER_DRIVERS_SELECT</code> and <code>MESA_VK_DEVICE_SELECT</code> are passed through to pick one GPU.</li>
            <li>Each file is self-contained and pulls the image for its own backend (<code>-cpu</code>, <code>-cuda</code>, <code>-rocm</code>, <code>-vulkan</code>). To run the <code>all</code> image, change the image tag in the matching GPU file to <code>-all</code>.</li>
            <li>macOS is CPU only; Windows uses <code>compose.cuda.yaml</code> with Docker Desktop and WSL2. Startup errors are covered in <a href="https://www.kronkai.com/manual#172-libraries-and-devices">17.2 Libraries and Devices</a>.</li>
          </ul>
          <p>To upgrade, run from the Compose directory with the same <code>-f</code> files or <code>.env</code>:</p>
          <ul>
            <li>Pinned version: change <code>KRONK_IMAGE_VERSION</code> to the new release, then run <code>docker compose up -d</code>. If you downloaded the Compose files for a release tag, download them again for the new tag first.</li>
            <li><code>latest</code>: run <code>docker compose pull</code>, then <code>docker compose up -d</code>.</li>
          </ul>
          <p><strong>Stop and uninstall without deleting data</strong></p>
          <p>For a container created with <code>docker run</code>:</p>
          <pre className="code-block"><code className="language-shell">{`docker stop kronk
docker rm kronk`}</code></pre>
          <p>For Compose, run from its directory with the same <code>-f</code> files or <code>.env</code> used to start it:</p>
          <pre className="code-block"><code className="language-shell">{`docker compose stop
docker compose down`}</code></pre>
          <p><code>stop</code> leaves the container available to start again; <code>down</code> removes the Compose containers and network. Both retain the named <code>kronk-data</code> volume. Do not use <code>docker compose down --volumes</code> or <code>docker volume rm kronk-data</code> unless you intend to permanently delete models, configuration, and keys. Back up important data before any deliberate volume removal.</p>
          <p>The <code>kronk-net</code> network is shared with the <a href="https://www.kronkai.com/manual#153-bundled-observability-stack">observability stack</a> and can remain after both stacks stop; remove it with <code>docker network rm kronk-net</code>.</p>
          <h2 id="88-running-as-a-systemd-service">8.8 Running as a systemd Service</h2>
          <p>On Linux, <code>kronk systemd</code> installs and manages a systemd <strong>user service</strong>. It runs as your normal user and uses your existing <code>~/.kronk</code> models, libraries, and keys. It is separate from <code>kronk server start --detach</code>: systemd owns the process, restarts it after failures, and stores its logs in the journal. Do not use <code>sudo kronk systemd</code>.</p>
          <p>Install Kronk in a permanent location using <code>go install</code>, Homebrew, or a release binary, then run:</p>
          <pre className="code-block"><code className="language-shell">{`kronk systemd install
kronk systemd start
kronk systemd status
curl http://localhost:11435/v1/liveness`}</code></pre>
          <p>Stop any existing foreground or detached server first to avoid port and database-lock conflicts. Use Ctrl-C for a foreground server or <code>kronk server stop</code> for a detached server. Once systemd manages the server, use <code>kronk systemd stop</code> and <code>kronk systemd logs</code> instead.</p>
          <h3 id="installation-and-startup">Installation and Startup</h3>
          <p>The service definition is embedded in the executable; there is no separate download or shell script. Installation records the absolute path of the running executable and writes <code>~/.config/systemd/user/kronk.service</code>. With <code>XDG_CONFIG_HOME</code> set, that directory replaces <code>~/.config</code> for both the service file and the optional configuration file below. Do not install using <code>go run</code>: its temporary executable will disappear.</p>
          <p>Installation enables startup at login but does not start the server. To start at boot without logging in and keep running after logout, use:</p>
          <pre className="code-block"><code className="language-shell">{`kronk systemd install --boot`}</code></pre>
          <p><code>--boot</code> runs <code>loginctl enable-linger</code> for your account. It may require authorization and affects all your user services. If already installed, use <code>loginctl enable-linger</code> directly, or reinstall with <code>--force --boot</code>. A machine-wide service with a dedicated system account is not installed.</p>
          <h3 id="configuration">Configuration</h3>
          <p>The optional <code>~/.config/kronk/kronk.env</code> contains server environment settings (see §8.3). Create its directory if needed and protect sensitive settings:</p>
          <pre className="code-block"><code className="language-shell">{`mkdir -p ~/.config/kronk
touch ~/.config/kronk/kronk.env
chmod 600 ~/.config/kronk/kronk.env`}</code></pre>
          <p>For example:</p>
          <pre className="code-block"><code className="language-text">{`KRONK_WEB_API_HOST=127.0.0.1:11435
KRONK_AUTHORIZATION_MODE=full-protected
KRONK_WEB_ADMIN_ENABLED=false
# Optional GPU settings; use values appropriate to your host:
# KRONK_PROCESSOR=rocm
# PATH=/opt/rocm/bin:/usr/local/bin:/usr/bin:/bin
# LD_LIBRARY_PATH=/opt/rocm/lib`}</code></pre>
          <p>This is not a shell script: use <code>NAME=value</code>, without <code>export</code>, and absolute paths, not <code>~</code>, <code>$HOME</code>, or <code>$PATH</code> substitutions. The service does not read your shell startup files. GPU detection tools and native libraries must be available in its environment. Configure GPU-specific values here if a working interactive shell depends on them.</p>
          <p><code>--base-path</code> or <code>KRONK_BASE_PATH</code> when installing records a custom data directory in the unit; <code>KRONK_BASE_PATH</code> in <code>kronk.env</code> overrides that value. Other shell settings are not copied into the unit automatically. The service loads the existing <code>model_config.yaml</code>; settings in <code>kronk.env</code> override its server settings. The built-in listener default is <code>127.0.0.1:11435</code>, but existing YAML can change it. The example above explicitly selects a local, protected, headless server. Review listener and authentication settings before starting. The initial administrator token is at <code>&lt;base-path&gt;/keys/master.jwt</code>; see <a href="https://www.kronkai.com/manual#chapter-12-security-and-authentication">Chapter 12</a> before remote exposure or enabling the BUI.</p>
          <h3 id="managing-and-removing-the-service">Managing and Removing the Service</h3>
          <pre className="code-block"><code className="language-shell">{`kronk systemd stop
kronk systemd start
kronk systemd restart
kronk systemd status
kronk systemd logs
kronk systemd uninstall`}</code></pre>
          <p>Restart after editing <code>kronk.env</code> or upgrading the binary at the same path. If the binary moves, run <code>kronk systemd install --force</code> to update its path, then restart. An existing unit is not overwritten without <code>--force</code>. <code>status</code> reports systemd process state, not HTTP readiness; startup can include library downloads. Check <code>/v1/liveness</code> before sending requests.</p>
          <p>Use <code>systemctl --user edit kronk</code> for systemd overrides. The unit allows 150 seconds to stop. If increasing <code>KRONK_WEB_SHUTDOWN_TIMEOUT</code> above its default of one minute, set <code>TimeoutStopSec</code> to at least twice that timeout plus 30 seconds. The unit requires systemd 240 or newer (<code>Type=exec</code>).</p>
          <p>Uninstall stops and disables the service, removes the unit, and reloads systemd. It preserves models, libraries, keys, <code>kronk.env</code>, and systemd drop-ins. It also preserves user lingering because other services may depend on it; use <code>loginctl disable-linger</code> only if none do.</p>
          <h2 id="89-running-as-a-launchd-agent">8.9 Running as a launchd Agent</h2>
          <p>On macOS, <code>kronk launchd</code> installs and manages a per-user <strong>LaunchAgent</strong>. It runs as you, reuses your existing <code>~/.kronk</code> models, libraries, keys, and server configuration, and starts at desktop login. It is not a system-wide LaunchDaemon and does not run before login or continue after logout. Run the commands as your normal user, not with <code>sudo</code>; a macOS desktop login session must be available.</p>
          <p>Install Kronk in a permanent location using <code>go install</code>, Homebrew, or a release binary, then run:</p>
          <pre className="code-block"><code className="language-shell">{`kronk launchd install
kronk launchd start
kronk launchd status
curl http://localhost:11435/v1/liveness`}</code></pre>
          <p>Installation writes the embedded definition to <code>~/Library/LaunchAgents/com.ardanlabs.kronk.plist</code> and enables startup at login, without loading or starting the agent immediately. It records the absolute path used to invoke Kronk, preserving stable symlinks such as Homebrew's <code>/opt/homebrew/bin/kronk</code> (Apple Silicon) or <code>/usr/local/bin/kronk</code> (Intel). Do not install using <code>go run</code>: its temporary executable will disappear.</p>
          <p>Stop any existing foreground server with Ctrl-C, or a detached server with <code>kronk server stop</code>, before starting the agent. The agent runs <code>kronk server start</code> in the foreground; launchd owns its process and logs. Use <code>kronk launchd stop</code> and <code>kronk launchd logs</code> for that process, not the detached-server commands.</p>
          <h3 id="launchagent-configuration">LaunchAgent Configuration</h3>
          <p><code>--base-path</code> or <code>KRONK_BASE_PATH</code> at installation records a custom data directory. Otherwise Kronk uses <code>~/.kronk</code>. Server settings in the existing <code>model_config.yaml</code> continue to apply (see §8.5). To override settings for the agent, edit its plist's <code>EnvironmentVariables</code> dictionary. For example, add these entries inside that dictionary:</p>
          <pre className="code-block"><code className="language-xml">{`<key>KRONK_WEB_API_HOST</key>
<string>127.0.0.1:11435</string>
<key>KRONK_AUTHORIZATION_MODE</key>
<string>full-protected</string>
<key>KRONK_WEB_ADMIN_ENABLED</key>
<string>false</string>`}</code></pre>
          <p>Launchd does not source shell startup files or read the systemd <code>kronk.env</code> file. Other shell settings are not automatically copied at installation. Use literal values and absolute paths, not <code>~</code>, <code>$HOME</code>, or <code>$PATH</code> substitutions. The plist includes a <code>PATH</code> covering both standard Homebrew locations and macOS system tools. Its initial permissions are <code>0600</code>; keep it private if adding sensitive environment values.</p>
          <p>Validate an edited plist, then reload it:</p>
          <pre className="code-block"><code className="language-shell">{`plutil -lint ~/Library/LaunchAgents/com.ardanlabs.kronk.plist
kronk launchd restart`}</code></pre>
          <p>The built-in API listener default is <code>127.0.0.1:11435</code>, but existing YAML can change it. The environment example above overrides YAML to select a local, protected, headless server. Review the effective settings before starting; the initial administrator token is at <code>&lt;base-path&gt;/keys/master.jwt</code>. See <a href="https://www.kronkai.com/manual#chapter-12-security-and-authentication">Chapter 12</a> before remote exposure or enabling the BUI. <code>status</code> shows launchd's agent information, not HTTP readiness; startup may include library downloads. Check <code>/v1/liveness</code> before sending requests.</p>
          <h3 id="launchagent-lifecycle-and-logs">LaunchAgent Lifecycle and Logs</h3>
          <pre className="code-block"><code className="language-shell">{`kronk launchd stop
kronk launchd start
kronk launchd restart
kronk launchd status
kronk launchd logs
kronk launchd uninstall`}</code></pre>
          <p><code>RunAtLoad</code> starts the server when the agent is loaded. <code>KeepAlive</code> restarts it if the process exits, with a 10-second throttle for repeated short-lived runs. <code>stop</code> unloads the agent so it stays stopped for the current login session, until explicitly started again. It starts again at the next desktop login. <code>restart</code> gracefully unloads and reloads the plist, applying configuration changes rather than just killing the process. The plist requests a 150-second <code>ExitTimeOut</code> for graceful HTTP shutdown and model unload, but the effective grace period can depend on macOS. Inspect <code>kronk launchd status</code> for the effective exit timeout. If shutdown can exceed it, reduce <code>KRONK_WEB_SHUTDOWN_TIMEOUT</code> so twice that timeout plus 30 seconds fits the available grace period. Merely increasing <code>ExitTimeOut</code> does not guarantee a longer grace period.</p>
          <p>Both standard output and standard error append to <code>~/Library/Logs/kronk/kronk.log</code>. <code>logs</code> follows that file, including after rotation; Ctrl-C stops following, not the server. Arrange log rotation if running long-term, since launchd does not impose a size limit on this file.</p>
          <p>Restart after upgrading the binary at the same path. If its location changes, reinstall with <code>kronk launchd install --force</code>, then restart. <code>--force</code> replaces the plist and resets manual plist edits; save those edits first. Models, keys, server YAML configuration, and logs are preserved. Uninstall unloads and disables the agent and removes its plist, retaining server data and logs. Any environment settings stored only in the removed plist are removed with it.</p>
          <h2 id="810-kubernetes-with-helm">8.10 Kubernetes with Helm</h2>
          <p>The chart in <code>zarf/helm/kronk</code> runs the published image as one server with a retained data PVC; it does not provide synchronized HA. Requirements are an existing Kubernetes 1.27+ cluster, Helm 3, kubectl, a default StorageClass (or configured storage), and enough node memory/disk for your models. The default claim is <code>50Gi</code> RWO. Image and model downloads require network access.</p>
          <p>From a repository checkout, copy the CPU example to your own values file and install into the intended cluster:</p>
          <pre className="code-block"><code className="language-shell">{`kubectl config current-context
cp zarf/helm/kronk/examples/cpu.yaml my-kronk.yaml
helm upgrade --install kronk zarf/helm/kronk -n kronk --create-namespace \\
  -f my-kronk.yaml --wait --timeout 20m
helm test kronk -n kronk
kubectl port-forward -n kronk service/kronk 11435:11435`}</code></pre>
          <p>Authentication is enabled and the BUI is disabled by default. Port-forwarding provides temporary local access; keep authentication enabled and configure TLS and access restrictions before exposing the API remotely. Optional metrics also expose unauthenticated pprof and must remain restricted to trusted clients.</p>
          <p>GPU deployments require compatible node drivers, device access, and runtime configuration; selecting an image variant does not provision a GPU. CPU smoke tests do not validate GPU inference. Uninstall retains data, but deleting its PVC <strong>or namespace</strong> can destroy models and keys. Back up before maintenance.</p>
          <p>See the <a href="https://github.com/ardanlabs/kronk/blob/main/zarf/helm/kronk/README.md">Helm chart README</a> for the complete first-request walkthrough, backend selection, configuration, troubleshooting, upgrades, and storage/key management. Keep deployment overrides in your own values file rather than editing the chart.</p>
          <h2 id="811-related-administration-guides">8.11 Related Administration Guides</h2>
          <p>Detailed administration is divided by responsibility:</p>
          <ul>
            <li><a href="https://www.kronkai.com/manual#chapter-2-installation-quick-start">Chapter 2</a> — installation, libraries, image variants, and data paths</li>
            <li><a href="https://www.kronkai.com/manual#chapter-3-model-configuration">Chapter 3</a> — per-model runtime settings</li>
            <li><a href="https://www.kronkai.com/manual#chapter-12-security-and-authentication">Chapter 12</a> — authentication, keys, tokens, and remote exposure</li>
            <li><a href="https://www.kronkai.com/manual#chapter-13-browser-ui-bui">Chapter 13</a> — BUI operation and browser login</li>
            <li><a href="https://www.kronkai.com/manual#chapter-15-observability">Chapter 15</a> — logs, health checks, metrics, tracing, and profiling</li>
            <li><a href="https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription">Chapter 18</a> — transcription libraries, models, and pool behavior</li>
          </ul>
          <h2 id="chapter-9-api-endpoints">Chapter 9: API Endpoints</h2>
          <p>Kronk exposes several familiar inference API formats. This chapter describes their wire contracts and the Kronk-specific details needed to use them. See <a href="https://www.kronkai.com/manual#chapter-10-request-parameters">Chapter 10</a> for generation and sampling parameters.</p>
          <h2 id="91-api-conventions">9.1 API Conventions</h2>
          <p>The examples use the default server address, <code>http://localhost:11435</code>. JSON endpoints accept <code>Content-Type: application/json</code>. Streaming endpoints use Server-Sent Events (SSE).</p>
          <p>Inference and model discovery are public in <code>open</code> and <code>management</code> modes. <code>authenticated</code> requires a valid bearer token; <code>full-protected</code> additionally requires the inference endpoint's grant:</p>
          <pre className="code-block"><code className="language-text">{`Authorization: Bearer <token>`}</code></pre>
          <p>Management routes require an administrator token in every mode except <code>open</code>. External auth providers must also be configured to enforce authentication. See <a href="https://www.kronkai.com/manual#chapter-12-security-and-authentication">Chapter 12</a> for token creation, endpoint grants, and rate limits.</p>
          <p>Application errors use a top-level code and message:</p>
          <pre className="code-block"><code className="language-json">{`{
  "code": "invalid_argument",
  "message": "missing model field"
}`}</code></pre>
          <p>The HTTP status reflects the error. Depending on the failure, clients may see statuses such as 400, 401, 403, 404, 409, 429, 500, 501, or 503.</p>
          <p>Errors before streaming begins use the JSON error format and HTTP status above. Once SSE headers have been sent, a generation failure is reported inside the stream and the HTTP status cannot change. Clients must check stream error records rather than treating HTTP 200 as successful completion.</p>
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
                <td><code>/v1/systemone</code></td>
                <td>POST</td>
                <td>Evaluate typed decision questions</td>
              </tr>
              <tr>
                <td><code>/v1/decide</code></td>
                <td>POST</td>
                <td>Alias for <code>/v1/systemone</code></td>
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
                <td><code>/v1/models/&#123;model&#125;</code></td>
                <td>GET</td>
                <td>Retrieve one locally available model</td>
              </tr>
              <tr>
                <td><code>/v1/images/generations</code></td>
                <td>POST</td>
                <td>Generate an image with Malina</td>
              </tr>
              <tr>
                <td><code>/v1/images/edits</code></td>
                <td>POST</td>
                <td>Transform an image with Malina</td>
              </tr>
              <tr>
                <td><code>/v1/images/events</code></td>
                <td>GET</td>
                <td>Stream server-wide Malina progress</td>
              </tr>
              <tr>
                <td><code>/v1/audio/transcriptions</code></td>
                <td>POST</td>
                <td>Transcribe audio with Bucky</td>
              </tr>
              <tr>
                <td><code>/v1/audio/translations</code></td>
                <td>POST</td>
                <td>Translate audio into English with Bucky</td>
              </tr>
            </tbody>
          </table>
          <p>Sections 9.11 through 9.14 inventory the administration, diagnostics, and evaluation endpoints used by the CLI and BUI. Administration endpoints are open when administration authentication is disabled. When it is enabled, they require an administrator token. <code>GET /v1/models</code> and <code>GET /v1/models/&#123;model&#125;</code> instead follow inference authentication and do not require a separate endpoint grant.</p>
          <h2 id="93-chat-completions-and-tool-calls">9.3 Chat Completions and Tool Calls</h2>
          <p><code>POST /v1/chat/completions</code> accepts an OpenAI-style <code>model</code> and <code>messages</code> request:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "unsloth/Qwen3-1.7B-UD-Q8_K_XL",
  "messages": [
    {"role": "system", "content": "Be concise."},
    {"role": "user", "content": "What is the capital of France?"}
  ]
}`}</code></pre>
          <p>A non-streaming response contains one or more <code>choices</code>, an assistant <code>message</code>, a <code>finish_reason</code>, and token <code>usage</code>. Thinking models can also return <code>reasoning_content</code>. Set the top-level <code>enable_thinking</code> boolean to request or suppress thinking when the model and its chat template support that option.</p>
          <p>Kronk currently generates one choice per request. The optional <code>n</code> field may be omitted, <code>null</code>, or <code>1</code>; other values are rejected because multiple choices are not supported.</p>
          <p>Use <code>max_completion_tokens</code> to set the output-token limit. The legacy <code>max_tokens</code> name remains supported; if both are supplied, <code>max_completion_tokens</code> takes precedence. Use <code>stop</code> with a string or an array of up to four strings to end generation when Kronk encounters one of those sequences. The matched sequence is omitted from the response. A custom stop has <code>finish_reason: "stop"</code>; a response that reaches its output-token limit has <code>finish_reason: "length"</code>.</p>
          <p>Set <code>"stream": true</code> to receive chat completion chunks as SSE records:</p>
          <pre className="code-block"><code className="language-text">{`data: {"id":"chatcmpl-...","object":"chat.completion.chunk",...}

data: [DONE]`}</code></pre>
          <p>By default, streaming chunks omit <code>usage</code>. To request usage, set <code>"stream_options": &#123;"include_usage": true&#125;</code>. Each completion chunk then has <code>"usage": null</code>, and Kronk sends one additional chunk before <code>[DONE]</code> with an empty <code>choices</code> array and the final usage totals. This option affects the streaming response; it does not change generation or server-side accounting.</p>
          <p>Compatible tool-call parsers emit an OpenAI-style activity delta as soon as a function name is known. Once parsing completes, Kronk emits the completed arguments in a nonterminal tool-call delta followed by an empty terminal delta with <code>finish_reason: "tool_calls"</code>.</p>
          <p><code>usage.completion_tokens</code> includes all generated tokens, including reasoning, control, and tool-call syntax that a parser may buffer instead of exposing as assistant text. <code>usage.completion_tokens_details.reasoning_tokens</code> reports the reasoning subset. <code>usage.total_tokens</code> is the sum of <code>prompt_tokens</code> and <code>completion_tokens</code>.</p>
          <h3 id="tool-calls">Tool calls</h3>
          <p>Add OpenAI-style function definitions in <code>tools</code> and use <code>"tool_choice": "auto"</code> to let the model select one. Tool calling requires a compatible model, chat template, and output parser; adding <code>tools</code> cannot give an incompatible model tool-calling ability. Use <code>"none"</code> to withhold tools and prevent structured tool-call output, <code>"required"</code> to request a call from a compatible model template, or the OpenAI forced-function object to limit the request to one declared function. Required and forced selection remain subject to the model and template honoring the requested mode.</p>
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
          <p>Execute the function in your application, then append the assistant message and a <code>role: "tool"</code> message containing the result and matching <code>tool_call_id</code>. Send the full conversation in the next request. Tool calls can also stream incrementally. Chat Completions selects a specific function with <code>&#123;"type":"function","function":&#123;"name":"get_weather"&#125;&#125;</code>.</p>
          <h2 id="94-responses-api">9.4 Responses API</h2>
          <p><code>POST /v1/responses</code> accepts <code>input</code> as a string:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "unsloth/Qwen3-1.7B-UD-Q8_K_XL",
  "input": "Explain quantum computing in simple terms."
}`}</code></pre>
          <p>It also accepts an array of input messages for conversations. A non-streaming response places generated messages or function calls in <code>output</code>. Tools use Responses-style tool definitions. <code>tool_choice</code> accepts <code>"none"</code>, <code>"auto"</code>, or <code>"required"</code>. Select a specific function with the Responses form <code>&#123;"type":"function","name":"get_weather"&#125;</code>.</p>
          <p>This is a compatible inference interface, not a stored-response service. Send the full conversation and function-call results in <code>input</code>; <code>previous_response_id</code> does not retrieve earlier turns, and <code>store</code> does not enable response persistence. File and <code>input_file</code> content parts are rejected. Use Kronk's top-level <code>response_format</code> and <code>reasoning_effort</code> controls; the Responses-native <code>text.format</code> and <code>reasoning.effort</code> objects are not translated.</p>
          <p>Use <code>max_output_tokens</code> to set the Responses output limit. <code>max_tokens</code> remains available as a compatibility alias, but <code>max_output_tokens</code> wins when both are present. As a Kronk extension, <code>stop</code> accepts a string or an array of up to four strings and ends generation when one is encountered; the matched sequence is omitted from the output. When the output-token limit is reached, the response has <code>status: "incomplete"</code>, <code>completed_at: null</code>, and:</p>
          <pre className="code-block"><code className="language-json">{`{"incomplete_details": {"reason": "max_output_tokens"}}`}</code></pre>
          <p>Output items have the same <code>incomplete</code> status. Usage reports all generated output tokens in <code>output_tokens</code>, including reasoning and tool-call syntax; <code>output_tokens_details.reasoning_tokens</code> supplies the reasoning subset, and <code>total_tokens</code> includes both input and output.</p>
          <p>With <code>"stream": true</code>, each SSE record has a named event and matching JSON payload. Streaming usage is omitted unless the request sets <code>"stream_options": &#123;"include_usage": true&#125;</code>. A text response commonly includes:</p>
          <pre className="code-block"><code className="language-text">{`event: response.created
data: {"type":"response.created",...}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"...",...}

event: response.completed
data: {"type":"response.completed",...}`}</code></pre>
          <p>Function calls produce corresponding <code>response.function_call_arguments.delta</code> and <code>.done</code> events. A token-limited stream ends with <code>response.incomplete</code> instead of <code>response.completed</code> and its embedded response carries the same incomplete details as a non-streaming response.</p>
          <h2 id="95-anthropic-messages-api">9.5 Anthropic Messages API</h2>
          <p><code>POST /v1/messages</code> provides an Anthropic-style interface. <code>model</code> and a nonzero <code>max_tokens</code> are required:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "unsloth/Qwen3-1.7B-UD-Q8_K_XL",
  "max_tokens": 256,
  "system": "Be concise.",
  "messages": [
    {"role": "user", "content": "What is the capital of France?"}
  ]
}`}</code></pre>
          <p><code>system</code> and message <code>content</code> may be strings or arrays of content blocks. The API supports text, image, <code>tool_use</code>, and <code>tool_result</code> blocks, subject to the selected model's capabilities. Anthropic-style tool definitions use <code>name</code>, <code>description</code>, and <code>input_schema</code>.</p>
          <p>With <code>"stream": true</code>, Kronk emits Anthropic-style named events including <code>message_start</code>, <code>content_block_start</code>, <code>content_block_delta</code>, <code>content_block_stop</code>, <code>message_delta</code>, and <code>message_stop</code>.</p>
          <p>Custom <code>stop_sequences</code> are not supported and are rejected. A request that reaches <code>max_tokens</code> returns <code>stop_reason: "max_tokens"</code>; natural completion uses <code>end_turn</code>, and a completed tool call uses <code>tool_use</code>. Streaming and non-streaming responses use the same mapping.</p>
          <h2 id="96-embeddings">9.6 Embeddings</h2>
          <p><code>POST /v1/embeddings</code> accepts one string or an array of strings:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "Qwen/Qwen3-Embedding-0.6B-Q8_0",
  "input": ["First document", "Second document"]
}`}</code></pre>
          <p>The response contains <code>object</code>, <code>created</code>, <code>model</code>, a <code>data</code> array, and <code>usage</code>. Each data item has an <code>index</code> and an <code>embedding</code> vector. Use an embedding model; models not marked as embedding models are rejected.</p>
          <p>Inputs exceeding the model's effective token limit are rejected by default. Set <code>truncate: true</code> to truncate them; the default <code>truncate_direction: "right"</code> drops the end, while <code>"left"</code> drops the beginning. The effective limit can include batch limits as well as the context window.</p>
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
          <h2 id="98-decisions">9.8 Decisions</h2>
          <p><code>POST /v1/systemone</code> and <code>POST /v1/decide</code> are equivalent. Both evaluate typed questions independently against one shared <code>state</code> value and require a model recognized as a decision model. Use <code>/v1/decide</code> for the descriptive Kronk route name or <code>/v1/systemone</code> for SystemOne compatibility.</p>
          <p>The request contains a model ID, any JSON-compatible shared state, and a nonempty object of named questions:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "chaoliangUNSW/Jev-Style-0.8B-Decision-v3-Q8_0",
  "state": {
    "customer_message": "I was charged twice and need this fixed today.",
    "account_tier": "business"
  },
  "questions": {
    "route": {
      "type": "choice",
      "instructions": "Which team should handle this request?",
      "criteria": {
        "billing": "Payments, invoices, refunds, and duplicate charges",
        "technical_support": "Product bugs and technical problems",
        "sales": "Plans, pricing, and new purchases"
      }
    },
    "urgency": {
      "type": "score",
      "instructions": "How urgent is this request?",
      "criteria": ["not urgent", "normal", "urgent", "critical"]
    },
    "requires_human": {
      "type": "noul",
      "instructions": "Should a human review this request?"
    }
  }
}`}</code></pre>
          <p>Question types have different criteria and response fields:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Type</th>
                <th>Request criteria</th>
                <th>Response</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>choice</code></td>
                <td>Object whose keys are option names and values describe each option</td>
                <td>Selected <code>choice</code>, option <code>probabilities</code>, and <code>confidence</code></td>
              </tr>
              <tr>
                <td><code>score</code></td>
                <td>Ordered array of 2 through 10 levels</td>
                <td>Numeric <code>score</code>, level <code>legend</code>, level <code>probabilities</code>, and <code>confidence</code></td>
              </tr>
              <tr>
                <td><code>noul</code></td>
                <td>Optional object with <code>false</code> and <code>true</code> descriptions</td>
                <td>Calibrated <code>noul</code> probability from 0 through 1</td>
              </tr>
            </tbody>
          </table>
          <p>Object order is part of the decision-model input. Kronk preserves the incoming order of state fields, questions, choice options, instructions, and nested criteria rather than normalizing them through unordered maps. Protocols may apply their model's normalization rules: decision2 sorts structured state keys, while Clef sorts choice options.</p>
          <h4 id="decision-images">Decision images</h4>
          <p>Vision-capable Clef models accept an optional top-level <code>images</code> array of base64 strings or image data URLs, such as <code>data:image/jpeg;base64,...</code>. Remote URLs are not fetched. In the Go SDK, set <code>DecisionRequest.Images</code> to an ordered <code>[][]byte</code> containing encoded JPEG, PNG, GIF, or WebP files.</p>
          <p>All images appear before <code>state</code>, in array order, outside question and option spans. Image-shaped values inside <code>state</code> remain ordinary structured data; Kronk does not extract chat-message attachments from decision state.</p>
          <p>This requires a compatible projector configured through <code>ProjFile</code> and a Clef GGUF whose <code>systemone</code> template supports images. A projection file alone does not enable images for other decision protocols. LFM3-d1-omni supports image and audio inputs as described below; OpenJEV and the remaining protocols stay text-only. Video inputs are unsupported.</p>
          <p>The complete text-plus-image prompt must fit both the context window and one physical batch. Expanded image rows count toward <code>usage.input_tokens</code> and the batch budget. Oversized inputs are rejected, never truncated; increase <code>context-window</code> or <code>prefill-batch-size</code> as indicated by the error. Decoding images also consumes memory proportional to their dimensions, so applications accepting untrusted images should bound payload sizes, dimensions, and count.</p>
          <h4 id="lfm3-d1-omni-inputs">LFM3-d1-omni inputs</h4>
          <p>LFM3-d1-omni models with a compatible projector accept an optional top-level <code>files</code> array containing nonempty base64 strings or image/audio data URLs, such as <code>data:image/png;base64,...</code> and <code>data:audio/wav;base64,...</code>. The existing <code>images</code> field remains image-only. Images appear first, followed by <code>files</code> in array order, before state. Remote URLs are not fetched and video data URLs are rejected. Encoded media formats and protocol/projector support are validated by the SDK.</p>
          <p>A question can use audio instructions instead of text or structured data:</p>
          <pre className="code-block"><code className="language-json">{`{
  "type": "noul",
  "instructions": {
    "type": "input_audio",
    "input_audio": {
      "data": "data:audio/wav;base64,...",
      "format": "wav"
    }
  }
}`}</code></pre>
          <p><code>input_audio.data</code> must contain nonempty base64 audio or an audio base64 data URL. Only an instructions object with the exact <code>type</code> value <code>input_audio</code> triggers audio decoding; other structured instructions and state remain ordinary JSON.</p>
          <h4 id="decision-protocols">Decision protocols</h4>
          <p>Kronk selects the decision protocol from the model's metadata and bundled template. Use a compatible decision model rather than an ordinary chat model. OpenJEV, Jev-Style, Laya/Julia, Lev, Kev, Nimble, and Clef are supported, along with the following protocols:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Protocol</th>
                <th>User-visible behavior</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>pplx-decider</code></td>
                <td>Independent decisions for each question</td>
              </tr>
              <tr>
                <td><code>lfm2-d1</code></td>
                <td>Model-calibrated option probabilities</td>
              </tr>
              <tr>
                <td><code>lfm3-d1</code></td>
                <td>Median score; confidence is maximum answer probability</td>
              </tr>
              <tr>
                <td><code>lfm3-d1-omni</code></td>
                <td>LFM3-d1 semantics with image/audio state and spoken instructions; requires a compatible projector</td>
              </tr>
              <tr>
                <td><code>vllm-decision2</code></td>
                <td>Requires at least two choice options and text, object, or array state</td>
              </tr>
              <tr>
                <td><code>vllm-d3</code></td>
                <td>Allows omitted or empty instructions, using the template's default question</td>
              </tr>
            </tbody>
          </table>
          <p>These protocols apply model-provided temperatures and score-level biases. Decision2 and D3 return normalized entropy concentration as <code>confidence</code> for choice and score questions; <code>noul</code> still returns the true-option probability. Decision2 requires at least two choice options and a state that is text, an object, or an array. D3 permits omitted or empty instructions, using the template's default question; the other protocols require instructions.</p>
          <p>Decision2, D3, and LFM3 d1 evaluate each complete prompt in a single physical batch, regardless of the configured prefill batch size. D3 and LFM3 d1 use one execution sequence even when <code>NSeqMax</code> is larger. Large context windows can therefore increase working memory requirements for these models. LFM3 d1 does not reuse persistent sequence state; independent questions and concurrent requests are serialized through that single execution sequence.</p>
          <p>LFM3 d1 preserves structured JSON state and option descriptions, returns the median score level rather than an expected level, and uses maximum answer probability for choice and score <code>confidence</code>. The <code>lfm3-d1-omni</code> protocol shares these semantics and supports image/audio state and spoken instructions with a compatible projector. All expanded media rows count toward usage and the whole-prompt budget; inputs are never truncated.</p>
          <p>Model-backed compatibility has been exercised for LFM3 d1 text and Omni media decisions, not decision quality. The other four additional protocols still need model-backed validation. Protocol support alone is not a quality guarantee.</p>
          <p>A response is keyed by the caller's question IDs:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "chaoliangUNSW/Jev-Style-0.8B-Decision-v3-Q8_0",
  "answers": {
    "route": {
      "type": "choice",
      "choice": "billing",
      "probabilities": {
        "billing": 0.96,
        "technical_support": 0.03,
        "sales": 0.01
      },
      "confidence": 0.96
    },
    "urgency": {
      "type": "score",
      "score": 2.7,
      "legend": {
        "0": "not urgent",
        "1": "normal",
        "2": "urgent",
        "3": "critical"
      },
      "probabilities": {
        "0": 0.01,
        "1": 0.04,
        "2": 0.2,
        "3": 0.75
      },
      "confidence": 0.75
    },
    "requires_human": {
      "type": "noul",
      "noul": 0.83
    }
  },
  "usage": {
    "input_tokens": 132,
    "output_tokens": 0
  }
}`}</code></pre>
          <p>Decision models read logits directly rather than generating text, so <code>output_tokens</code> is zero. Exact scores and probabilities depend on the model and request. In <code>full-protected</code> mode, both aliases require the same <code>decision</code> endpoint grant.</p>
          <p>Call either route with curl:</p>
          <pre className="code-block"><code className="language-shell">{`make curl-kronk-decide
make curl-kronk-systemone`}</code></pre>
          <h3 id="sdk-examples">SDK examples</h3>
          <p>The bundled SDK reference documents typed decision requests and responses. Runnable examples are in <code>examples/decision</code>, <code>examples/decision-image</code>, and <code>examples/decision-audio</code>. From a source checkout:</p>
          <pre className="code-block"><code className="language-shell">{`make example-decision
make example-decision-image
make example-decision-audio`}</code></pre>
          <p>These examples can download models and projectors into the normal Kronk store and run inference. Media decisions require a compatible decision model, projector, and systemone template; a text-only conversion cannot accept images or audio merely because the request includes media.</p>
          <h2 id="99-tokenization">9.9 Tokenization</h2>
          <p><code>POST /v1/tokenize</code> returns a token <strong>count</strong>, not token IDs:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "unsloth/Qwen3-1.7B-UD-Q8_K_XL",
  "input": "The quick brown fox",
  "apply_template": true,
  "add_generation_prompt": true
}`}</code></pre>
          <p><code>apply_template</code> defaults to <code>false</code>. When enabled, Kronk wraps the input as a user message and includes chat-template overhead in the count. <code>add_generation_prompt</code> controls the assistant prefix when the template is applied and defaults to <code>true</code>.</p>
          <pre className="code-block"><code className="language-json">{`{
  "object": "tokenize",
  "created": 1738857600,
  "model": "unsloth/Qwen3-1.7B-UD-Q8_K_XL",
  "tokens": 11
}`}</code></pre>
          <h2 id="910-models-image-generation-and-audio-transcription">9.10 Models, Image Generation, and Audio Transcription</h2>
          <p><code>GET /v1/models</code> returns an OpenAI-style list of models and configured model extensions available locally. It is not limited to models currently loaded in memory. Each item includes <code>id</code>, <code>object</code>, <code>created</code>, and <code>owned_by</code>. The <code>id</code> uses the canonical <code>provider/modelID</code> form. <code>owned_by</code> comes from model metadata when available and otherwise defaults to <code>kronk</code>.</p>
          <p><code>GET /v1/models/&#123;model&#125;</code> returns the corresponding OpenAI-style model object for one model ID. It returns <code>404 Not Found</code> when the model is not available.</p>
          <p><code>POST /v1/images/generations</code> accepts an OpenAI-style text-to-image request and generates one PNG with a locally installed Malina model bundle:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "sd-1.5",
  "prompt": "A lighthouse during a thunderstorm",
  "size": "512x512",
  "response_format": "b64_json",
  "negative_prompt": "blurry",
  "steps": 20,
  "cfg_scale": 7,
  "seed": -1
}`}</code></pre>
          <p><code>size</code> defaults to <code>512x512</code>. The response contains <code>created</code> and a <code>data</code> array whose single item contains <code>b64_json</code>, <code>seed</code>, <code>width</code>, and <code>height</code>. Only <code>n: 1</code>, <code>response_format: "b64_json"</code>, and PNG output are supported.</p>
          <p><code>POST /v1/images/edits</code> accepts the same controls as multipart form fields and requires an <code>image</code> PNG or JPEG file. The optional <code>strength</code> field controls how far the result may depart from the source and defaults to <code>0.75</code>; its range is greater than zero through <code>1</code>. When <code>size</code> is omitted, Kronk preserves the source aspect ratio, scales dimensions down to at most 1024 pixels per side, and aligns them to multiples of eight. Uploads are limited to 25 MiB. Both image routes support <code>negative_prompt</code>, <code>steps</code>, <code>cfg_scale</code>, and <code>seed</code> and require the <code>image-generations</code> inference permission in <code>full-protected</code> mode.</p>
          <pre className="code-block"><code className="language-sh">{`curl http://localhost:11435/v1/images/edits \\
  -F model=sd-1.5 \\
  -F prompt='A watercolor illustration at sunset' \\
  -F image=@source.png \\
  -F strength=0.6 \\
  -F steps=20`}</code></pre>
          <p><code>GET /v1/images/events</code> is a persistent server-sent event stream for Malina model-loading and image-generation progress. Each event reports <code>scope: "global"</code>, <code>step</code>, <code>steps</code>, <code>percent</code>, and <code>seconds_per_step</code>. The native callback is process-global, so an event describes server-wide Malina activity and is not attributable to the client or request consuming the stream. This route also requires the <code>image-generations</code> inference permission in <code>full-protected</code> mode.</p>
          <p>ControlNet, ADetailer, video generation, and upscaling remain available through the Malina SDK rather than the model-server API.</p>
          <p><code>POST /v1/audio/transcriptions</code> accepts multipart audio uploads and uses the Bucky speech-to-text runtime. <code>POST /v1/audio/translations</code> accepts the same file, model, prompt, temperature, and response-format fields and translates speech from a supported language into English. Their request fields, formats, and administrative operations are documented in <a href="https://www.kronkai.com/manual#1861-request-and-response">Chapter 18</a>.</p>
          <h2 id="911-kronk-administration">9.11 Kronk Administration</h2>
          <p>These routes manage the llama.cpp runtime, local GGUF models, and the personal model catalog. Mutating routes may stream progress or perform network and disk operations. Clients should use the exact <code>/v1/kronk/...</code> prefix; the shorter <code>/v1/libs</code>, <code>/v1/models/pull</code>, and <code>/v1/catalog</code> forms are not aliases.</p>
          <h3 id="libraries">Libraries</h3>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Method and path</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>GET /v1/kronk/libs</code></td>
                <td>Show the active llama.cpp library installation and upgrade state</td>
              </tr>
              <tr>
                <td><code>GET /v1/kronk/libs/integrity</code></td>
                <td>Hash and verify the selected llama.cpp bundle against Yzma's release manifest</td>
              </tr>
              <tr>
                <td><code>GET /v1/kronk/libs/combinations</code></td>
                <td>List supported operating-system, architecture, and processor combinations</td>
              </tr>
              <tr>
                <td><code>GET /v1/kronk/libs/installs</code></td>
                <td>List installed library bundles</td>
              </tr>
              <tr>
                <td><code>POST /v1/kronk/libs/pull</code></td>
                <td>Install a library bundle and stream progress</td>
              </tr>
              <tr>
                <td><code>DELETE /v1/kronk/libs/installs</code></td>
                <td>Remove the bundle selected by <code>arch</code>, <code>os</code>, and <code>processor</code> query parameters</td>
              </tr>
            </tbody>
          </table>
          <p><code>POST /v1/kronk/libs/pull</code> accepts optional <code>arch</code>, <code>os</code>, <code>processor</code>, and <code>version</code> query parameters. Supply <code>arch</code>, <code>os</code>, and <code>processor</code> together for a cross-platform bundle; omit all three to operate on the active platform. A version can be externally pinned as <code>VERSION@sha256:&lt;64-hex-digest&gt;</code>. Yzma checks the raw release-manifest bytes against that digest and then verifies the selected archive before extraction.</p>
          <p><code>GET /v1/kronk/libs/integrity</code> hashes the installed files and compares them with the file digests in Yzma's release manifest. Its optional <code>version</code> query parameter accepts the same pinned syntax, allowing the caller to supply the trusted manifest digest rather than trusting the server's install record or manifest host. The response identifies the backend, version, platform triple, overall result, per-file states, and changed, missing, or unexpected counts. Kronk's default llama.cpp version already carries its published manifest digest, so default downloads and startup verification use the authenticated manifest without an operator-supplied version.</p>
          <p>Every successful runtime-integrity response includes <code>verified_at</code>, <code>bundle_manifest_version</code>, and <code>bundle_digest</code>. The digest is a stable identity for the selected runtime files, independent of archive serialization. The response also exposes per-file verification evidence. When verification fails, the digest and verification time are omitted.</p>
          <p>This digest identifies the installed bytes; it does not establish that they are acceptable. Compare it with an allowlist obtained outside the server's mutable runtime environment. <code>manifest_authenticated</code> separately reports whether the publisher manifest was authenticated by an externally supplied pin.</p>
          <p>Verification requires a Yzma install record and a release manifest containing per-file hashes. A release or upstream asset with archive hashes only returns an error rather than claiming the installed files are verified.</p>
          <h3 id="models">Models</h3>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Method and path</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>GET /v1/kronk/models</code></td>
                <td>List locally installed GGUF models with Kronk metadata</td>
              </tr>
              <tr>
                <td><code>GET /v1/kronk/models/integrity</code></td>
                <td>List local artifact digests and persisted verification evidence without hashing model files</td>
              </tr>
              <tr>
                <td><code>GET /v1/kronk/models/integrity/&#123;model&#125;</code></td>
                <td>Return integrity information for one local model without inspecting other models</td>
              </tr>
              <tr>
                <td><code>GET /v1/kronk/models/&#123;model&#125;</code></td>
                <td>Show detailed metadata and effective configuration for one model</td>
              </tr>
              <tr>
                <td><code>GET /v1/kronk/models/ps</code></td>
                <td>List models currently loaded in the pool</td>
              </tr>
              <tr>
                <td><code>GET /v1/kronk/models/imc-sessions</code></td>
                <td>List active incremental-message-cache sessions</td>
              </tr>
              <tr>
                <td><code>POST /v1/kronk/models/index</code></td>
                <td>Rebuild the local model index</td>
              </tr>
              <tr>
                <td><code>POST /v1/kronk/models/pull</code></td>
                <td>Download a model and optional companion files and stream progress</td>
              </tr>
              <tr>
                <td><code>POST /v1/kronk/models/autotune</code></td>
                <td>Resolve an automatically tuned runtime configuration for a model</td>
              </tr>
              <tr>
                <td><code>POST /v1/kronk/models/vram</code></td>
                <td>Estimate model memory use for the requested runtime settings</td>
              </tr>
              <tr>
                <td><code>POST /v1/kronk/models/unload</code></td>
                <td>Unload a model or playground instance from the pool</td>
              </tr>
              <tr>
                <td><code>DELETE /v1/kronk/models/&#123;model&#125;</code></td>
                <td>Remove a locally installed model</td>
              </tr>
            </tbody>
          </table>
          <p>The native model list and detail routes are distinct from the OpenAI-compatible <code>GET /v1/models</code> and <code>GET /v1/models/&#123;model&#125;</code> routes. Use the native routes for administration metadata and effective Kronk configuration.</p>
          <p><code>GET /v1/kronk/models/integrity</code> returns each indexed physical model with its weight shards, projection, and MTP artifacts. Each artifact includes the local filename and size, its Hugging Face SHA-256 digest when available, and one of these evidence states. For example:</p>
          <pre className="code-block"><code className="language-json">{`{
  "object": "model_integrity.list",
  "data": [
    {
      "id": "Qwen3-VL-30B-Q4_K_M",
      "owned_by": "unsloth",
      "model_family": "Qwen3-VL-30B-GGUF",
      "status": "unavailable",
      "verified": false,
      "artifacts": [
        {
          "role": "weights",
          "filename": "Qwen3-VL-30B-Q4_K_M.gguf",
          "digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
          "size": 18456789012,
          "status": "verified",
          "verified": true,
          "verified_at": "2026-08-07T14:35:09Z"
        },
        {
          "role": "projection",
          "filename": "mmproj-Qwen3-VL-30B-Q4_K_M.gguf",
          "size": 734567890,
          "status": "unavailable",
          "verified": false,
          "reason": "digest_unavailable"
        }
      ]
    }
  ]
}`}</code></pre>
          <p>Use <code>GET /v1/kronk/models/integrity/&#123;model&#125;</code> when only one model is needed. It reads the index and inspects sidecars and filesystem metadata only for that model. The response is the matching model object directly, without the list envelope:</p>
          <pre className="code-block"><code className="language-json">{`{
  "id": "Qwen3-VL-30B-Q4_K_M",
  "owned_by": "unsloth",
  "model_family": "Qwen3-VL-30B-GGUF",
  "status": "verified",
  "verified": true,
  "artifacts": [
    {
      "role": "weights",
      "filename": "Qwen3-VL-30B-Q4_K_M.gguf",
      "digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
      "size": 18456789012,
      "status": "verified",
      "verified": true,
      "verified_at": "2026-08-07T14:35:09Z"
    }
  ]
}`}</code></pre>
          <p>The targeted route returns <code>404 Not Found</code> when the model is not present in the local index. It is preferable to the collection route on installations with a large model inventory when a caller needs only one model.</p>
          <p>The response envelope and model fields are:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Field</th>
                <th>Meaning</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>object</code></td>
                <td>Always <code>model_integrity.list</code>.</td>
              </tr>
              <tr>
                <td><code>data</code></td>
                <td>Models physically present in the local model index. Configured extension aliases are not duplicated here.</td>
              </tr>
              <tr>
                <td><code>id</code></td>
                <td>Kronk model ID from the local index.</td>
              </tr>
              <tr>
                <td><code>owned_by</code></td>
                <td>Provider or organization directory containing the model.</td>
              </tr>
              <tr>
                <td><code>model_family</code></td>
                <td>Model-family directory containing the artifacts.</td>
              </tr>
              <tr>
                <td><code>status</code></td>
                <td>Least trustworthy status among the model's required artifacts: <code>unavailable</code>, then <code>stale</code>, then <code>unverified</code>, then <code>verified</code>.</td>
              </tr>
              <tr>
                <td><code>verified</code></td>
                <td><code>true</code> only when every required artifact has status <code>verified</code>.</td>
              </tr>
              <tr>
                <td><code>artifacts</code></td>
                <td>Physical weight shards followed by an optional projection and optional MTP drafter.</td>
              </tr>
            </tbody>
          </table>
          <p>Each artifact provides:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Field</th>
                <th>Meaning</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>role</code></td>
                <td><code>weights</code>, <code>projection</code>, or <code>mtp</code>. Each weight shard is a separate artifact.</td>
              </tr>
              <tr>
                <td><code>filename</code></td>
                <td>Basename of the local artifact.</td>
              </tr>
              <tr>
                <td><code>digest</code></td>
                <td>Hugging Face Git LFS SHA-256 identity in <code>sha256:&lt;hex&gt;</code> form. Omitted when unavailable.</td>
              </tr>
              <tr>
                <td><code>size</code></td>
                <td>Current local file size in bytes. It is <code>0</code> when an indexed file is absent.</td>
              </tr>
              <tr>
                <td><code>status</code></td>
                <td>Current evidence state described below.</td>
              </tr>
              <tr>
                <td><code>verified</code></td>
                <td>Convenience boolean that is <code>true</code> only for status <code>verified</code>.</td>
              </tr>
              <tr>
                <td><code>verified_at</code></td>
                <td>UTC time of the last successful full-file verification. Omitted when no usable verification record exists.</td>
              </tr>
              <tr>
                <td><code>reason</code></td>
                <td>Machine-readable explanation when applicable: <code>digest_unavailable</code> or <code>file_metadata_changed</code>.</td>
              </tr>
            </tbody>
          </table>
          <p>Artifact statuses mean:</p>
          <ul>
            <li><code>verified</code> — the published digest, persisted verification record, and current file size and modification time agree;</li>
            <li><code>unverified</code> — a published digest exists, but no persisted verification record exists;</li>
            <li><code>stale</code> — persisted verification or current file metadata no longer agrees;</li>
            <li><code>unavailable</code> — the published digest sidecar is missing or malformed.</li>
          </ul>
          <p>This endpoint reads only the model index, small sidecar files, and filesystem metadata. It does not hash model contents or freshly verify their bytes. A <code>verified</code> result means Kronk previously hashed that artifact successfully and the tracked file metadata has not changed since. It does not prove the current bytes through a new content read.</p>
          <h3 id="catalog">Catalog</h3>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Method and path</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>GET /v1/kronk/catalog</code></td>
                <td>List personal catalog entries and local validation state</td>
              </tr>
              <tr>
                <td><code>GET /v1/kronk/catalog/&#123;id...&#125;</code></td>
                <td>Show one catalog entry; the ID may contain slashes</td>
              </tr>
              <tr>
                <td><code>POST /v1/kronk/catalog/lookup</code></td>
                <td>List GGUF files for a HuggingFace repository or URL</td>
              </tr>
              <tr>
                <td><code>POST /v1/kronk/catalog/resolve</code></td>
                <td>Resolve a model source to canonical download files without downloading</td>
              </tr>
              <tr>
                <td><code>POST /v1/kronk/catalog/reconcile</code></td>
                <td>Reconcile catalog metadata with locally installed models</td>
              </tr>
              <tr>
                <td><code>DELETE /v1/kronk/catalog/&#123;id...&#125;</code></td>
                <td>Remove a catalog entry and its downloaded and cached files</td>
              </tr>
            </tbody>
          </table>
          <p><code>POST /v1/kronk/catalog/lookup</code> accepts <code>&#123;"input":"..."&#125;</code>. The resolve route accepts <code>&#123;"source":"..."&#125;</code> and may add successfully resolved metadata to the personal catalog even though it does not download model files.</p>
          <h2 id="912-bucky-and-malina-administration">9.12 Bucky and Malina Administration</h2>
          <p>The Bucky management API mirrors the library and model lifecycle for the whisper.cpp backend:</p>
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
                <td>Show the active whisper.cpp library installation</td>
              </tr>
              <tr>
                <td><code>GET /v1/bucky/libs/integrity</code></td>
                <td>Hash and verify the selected whisper.cpp bundle against Bucky's release manifest</td>
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
                <td>Install a library bundle and stream progress</td>
              </tr>
              <tr>
                <td><code>DELETE /v1/bucky/libs/installs</code></td>
                <td>Remove the selected library bundle</td>
              </tr>
              <tr>
                <td><code>GET /v1/bucky/models</code></td>
                <td>List installed whisper models</td>
              </tr>
              <tr>
                <td><code>GET /v1/bucky/models/catalog</code></td>
                <td>List the bundled short-name model catalog</td>
              </tr>
              <tr>
                <td><code>POST /v1/bucky/models/pull</code></td>
                <td>Download a whisper model and stream progress</td>
              </tr>
              <tr>
                <td><code>GET /v1/bucky/models/&#123;model&#125;/details</code></td>
                <td>Show model header and file details</td>
              </tr>
              <tr>
                <td><code>DELETE /v1/bucky/models/&#123;model&#125;</code></td>
                <td>Remove an installed whisper model</td>
              </tr>
            </tbody>
          </table>
          <p><code>POST /v1/bucky/libs/pull</code> accepts the same optional platform and <code>version</code> query parameters as the Kronk library route. Bucky always verifies the selected archive against its release manifest before extraction. A version in <code>VERSION@sha256:&lt;64-hex-digest&gt;</code> form also authenticates the manifest itself. The default Bucky version already includes its published manifest digest.</p>
          <p><code>GET /v1/bucky/libs/integrity</code> hashes the installed files. With no <code>version</code> query parameter, Kronk uses the version recorded for the active installation; the default installation therefore retains its authenticated manifest pin. The response includes <code>manifest_authenticated</code> and <code>source</code> in addition to the common library integrity fields.</p>
          <p>See <a href="https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription">Chapter 18</a> for installation, model naming, transcription formats, and Bucky-specific runtime behavior.</p>
          <h3 id="malina-models-and-runtime-integrity">Malina Models and Runtime Integrity</h3>
          <p>The Malina management API exposes installed text-to-image bundles and the selected stable-diffusion.cpp runtime identity:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Method and path</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>GET /v1/malina/models</code></td>
                <td>List installed model bundles intended for basic text-to-image generation</td>
              </tr>
              <tr>
                <td><code>GET /v1/malina/libs/integrity</code></td>
                <td>Hash and verify the selected stable-diffusion.cpp bundle against Malina's trusted manifest</td>
              </tr>
            </tbody>
          </table>
          <p>The integrity endpoint accepts an optional <code>version</code> query parameter, including <code>VERSION@sha256:&lt;64-hex-digest&gt;</code>. Its response uses the same canonical bundle identity and per-file evidence as the llama.cpp and whisper.cpp endpoints and reports <code>backend</code> as <code>stable-diffusion</code>.</p>
          <h2 id="913-operations-and-evaluation">9.13 Operations and Evaluation</h2>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Method and path</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>GET /v1/pool/budget</code></td>
                <td>Report shared host and device memory budgets and current reservations</td>
              </tr>
              <tr>
                <td><code>GET /v1/devices</code></td>
                <td>List detected compute devices</td>
              </tr>
              <tr>
                <td><code>GET /v1/diagnose</code></td>
                <td>Return the JSON diagnostic report; use <code>bench=true</code> to include a benchmark</td>
              </tr>
              <tr>
                <td><code>GET /v1/accuracy/functions</code></td>
                <td>List functions available to the code-recall accuracy test</td>
              </tr>
              <tr>
                <td><code>POST /v1/accuracy/test</code></td>
                <td>Run one code-recall comparison using <code>model</code> and <code>function</code></td>
              </tr>
              <tr>
                <td><code>POST /v1/efficiency/run</code></td>
                <td>Warm a model, run one prompt, and report throughput measurements</td>
              </tr>
            </tbody>
          </table>
          <p><code>GET /v1/diagnose</code> also accepts optional <code>model</code> and <code>processor</code> query parameters for its benchmark. The accuracy request body is <code>&#123;"model":"...","function":"..."&#125;</code>. The efficiency request body contains <code>model</code>, <code>prompt</code>, and an optional positive <code>max_tokens</code>, which defaults to</p>
          <ol>
            <li>These evaluation routes can load models and may take several minutes.</li>
          </ol>
          <h2 id="914-security-administration">9.14 Security Administration</h2>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Method and path</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>POST /v1/security/token/create</code></td>
                <td>Create a token with administrator status or endpoint grants and quotas</td>
              </tr>
              <tr>
                <td><code>GET /v1/security/keys</code></td>
                <td>List signing keys</td>
              </tr>
              <tr>
                <td><code>POST /v1/security/keys/add</code></td>
                <td>Create a signing key</td>
              </tr>
              <tr>
                <td><code>POST /v1/security/keys/remove/&#123;keyid&#125;</code></td>
                <td>Remove a non-master signing key and revoke its tokens</td>
              </tr>
            </tbody>
          </table>
          <p>These routes are authorized by the authentication service. In protected modes they require an administrator token. Token creation accepts <code>admin</code>, <code>duration</code>, and an <code>endpoints</code> map of endpoint grants and rate limits. See <a href="https://www.kronkai.com/manual#chapter-12-security-and-authentication">Chapter 12</a> for the request model, key rotation, and the effects of deleting a signing key.</p>
          <h2 id="chapter-10-request-parameters">Chapter 10: Request Parameters</h2>
          <p>This chapter covers generation parameters used by Chat Completions and the Go SDK. Other API formats expose compatible subsets or translate their own field names into these parameters. See <a href="https://www.kronkai.com/manual#chapter-9-api-endpoints">Chapter 9</a> for endpoint-specific request formats and streaming behavior.</p>
          <h2 id="101-scope-and-defaults">10.1 Scope and Defaults</h2>
          <p>The defaults below are Kronk's baseline values. When present, a model's GGUF sampling recommendations take precedence over those baselines. A model configuration can provide different sampling defaults, and a request can override them. The precedence order is request, model configuration, GGUF recommendation, then Kronk baseline. See <a href="https://www.kronkai.com/manual#37-advanced-features">Chapter 3 §3.7</a> for per-model <code>sampling-parameters</code>.</p>
          <p>This precedence has zero-value exceptions. Some zero-valued model configuration fields are treated as unset and fall back to GGUF recommendations or Kronk baselines. Explicit request zeros work for controls such as <code>temperature</code>, <code>top_k</code>, and <code>min_p</code>, but <code>dry_multiplier: 0</code> and <code>xtc_probability: 0</code> currently fall back to the resolved model default rather than disabling an enabled sampler.</p>
          <p>GGUF recommendations are used when present for <code>temperature</code>, <code>top_k</code>, <code>top_p</code>, <code>min_p</code>, <code>repeat_last_n</code>, <code>xtc_probability</code>, and <code>xtc_threshold</code>. Malformed or absent metadata falls back to Kronk's baseline. Model configuration and explicit request values take precedence subject to the zero-value exceptions above.</p>
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
                <td><code>1.0</code></td>
                <td>Rescales token probabilities. Higher values generally increase variation.</td>
              </tr>
              <tr>
                <td><code>top_k</code></td>
                <td>integer</td>
                <td><code>20</code></td>
                <td>Keeps only the K most probable candidates.</td>
              </tr>
              <tr>
                <td><code>top_p</code></td>
                <td>number</td>
                <td><code>0.95</code></td>
                <td>Keeps the smallest candidate set whose cumulative probability reaches P.</td>
              </tr>
              <tr>
                <td><code>min_p</code></td>
                <td>number</td>
                <td><code>0.0</code></td>
                <td>Removes candidates below <code>min_p × probability_of_most_likely_token</code>; <code>0</code> disables it.</td>
              </tr>
              <tr>
                <td><code>seed</code></td>
                <td>integer</td>
                <td>random</td>
                <td>Initializes sampling randomness; values from <code>0</code> through <code>4294967295</code> request repeatable sampling.</td>
              </tr>
            </tbody>
          </table>
          <p>Explicit request values override model-specific sampling defaults, including <code>top_p: 1</code>, which disables nucleus filtering. When <code>temperature</code>, <code>top_k</code>, or <code>top_p</code> is omitted, Kronk uses the configured value, GGUF recommendation, or baseline default in that order. Set <code>temperature: 0</code> explicitly to request greedy generation. Set <code>top_k: 0</code> to disable top-k filtering.</p>
          <p>When <code>seed</code> is omitted, Kronk uses the model's configured <code>sampling-parameters.seed</code> default, or generates a random master seed when no default is configured. A matched Incremental Message Cache session retains that master seed across conversation turns. Resetting or reassigning the session clears it so the next conversation receives a new random seed. Requests without a matched session continue to receive a new random seed. An explicit request seed, including <code>seed: 0</code>, overrides the configured or retained seed and becomes the session's retained seed. Seeded requests do not consume one another's sampling randomness. Repeatability requires the same Kronk and native-library builds, model files, request, sampling settings, backend and devices, speculative mode, and equivalent batching conditions. It is not guaranteed across software versions, backends, quantizations, sampler changes, or different batching schedules.</p>
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
                <td>Number of recent tokens considered by repetition penalties; <code>0</code> disables them and <code>-1</code> uses the full context.</td>
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
                <td>Enables DRY and controls its strength. Baseline disabled; see the zero-value caveat in §10.1.</td>
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
                <td><code>-1</code></td>
                <td>Recent-token window for DRY; <code>0</code> disables it and <code>-1</code> uses the full context.</td>
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
                <td>Probability that XTC runs for a token. Baseline disabled; see the zero-value caveat in §10.1.</td>
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
                <td>Legacy/common maximum output-token field.</td>
              </tr>
              <tr>
                <td><code>max_completion_tokens</code></td>
                <td>integer</td>
                <td>model-dependent</td>
                <td>Chat Completions output limit; takes precedence over <code>max_tokens</code>.</td>
              </tr>
              <tr>
                <td><code>max_output_tokens</code></td>
                <td>integer</td>
                <td>model-dependent</td>
                <td>Responses API output limit; takes precedence over <code>max_tokens</code>.</td>
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
                <td>template default</td>
                <td>Requests a model-specific reasoning level, commonly <code>none</code>, <code>minimal</code>, <code>low</code>, <code>medium</code>, <code>high</code>, or <code>xhigh</code>.</td>
              </tr>
            </tbody>
          </table>
          <p>If neither the request nor model configuration supplies a positive output limit, Kronk uses the model's configured context window. The actual output can be shorter because the prompt and generated text share that window, the model can stop naturally, or another limit can end generation. See Chapter 9 for the limit and termination fields returned by each API format.</p>
          <p>Reasoning controls are model- and template-dependent. Kronk accepts any string for <code>reasoning_effort</code> so newer templates can add levels without requiring a server change. Unsupported models may ignore the value, a parser can normalize it, and a strict template can reject values it does not support.</p>
          <p>When <code>reasoning_effort</code> is omitted from both the request and model configuration, Kronk leaves it undefined so the selected chat template can apply its native default.</p>
          <h2 id="106-structured-output">10.6 Structured Output</h2>
          <p>Kronk can convert JSON Schema to a GBNF grammar and constrain emitted tokens. For OpenAI-compatible clients, prefer <code>response_format</code>:</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "unsloth/Qwen3-1.7B-UD-Q8_K_XL",
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
          <p>Supported <code>response_format.type</code> values are <code>text</code>, <code>json_object</code>, and <code>json_schema</code>. Kronk also accepts a schema directly in the top-level <code>json_schema</code> field and accepts a custom GBNF string in <code>grammar</code>. Use one structured-output mechanism per request. JSON Schema <code>pattern</code> is not supported; Kronk rejects schemas containing it rather than interpreting its regular expression as GBNF.</p>
          <p>When a constraint is present and <code>enable_thinking</code> is omitted, Kronk disables thinking automatically so free-form reasoning does not precede the structured answer. Explicitly enabling thinking overrides that default, but is generally counterproductive for constrained output.</p>
          <p>A grammar restricts which tokens can be emitted; it does not guarantee a complete result. A response cut short by <code>max_tokens</code>, context limits, or cancellation can still contain an incomplete JSON value.</p>
          <h2 id="107-token-log-probabilities">10.7 Token Log Probabilities</h2>
          <p>Set <code>logprobs: true</code> to return the log probability of each generated token. <code>top_logprobs</code> requests likely alternatives and is clamped to the range 0–5. Any positive <code>top_logprobs</code> value implicitly enables <code>logprobs</code>.</p>
          <pre className="code-block"><code className="language-json">{`{
  "model": "unsloth/Qwen3-1.7B-UD-Q8_K_XL",
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
          <p>Multimodal models combine a language model with a media projector that turns images or audio into input the language model can process. They use the Chat Completions endpoint described in <a href="https://www.kronkai.com/manual#chapter-9-api-endpoints">Chapter 9</a>.</p>
          <h2 id="111-models-and-projectors">11.1 Models and Projectors</h2>
          <p>Use the catalog to find models with image or audio capabilities:</p>
          <pre className="code-block"><code className="language-shell">{`kronk catalog list`}</code></pre>
          <p>The <code>MTMD</code> column identifies entries with a multimodal projector. The BUI catalog also provides image, audio, and video capability filters. The live catalog is the source of truth; examples in the seed catalog include:</p>
          <ul>
            <li><code>ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0</code> for images;</li>
            <li><code>ggml-org/Qwen2.5-Omni-3B-Q8_0</code> for images and audio; and</li>
            <li><code>ggml-org/Qwen3-Omni-30B-A3B-Instruct-Q8_0</code> for image, audio, and video-capable model metadata.</li>
          </ul>
          <p>Pulling a catalog model also pulls its companion projector when one is available:</p>
          <pre className="code-block"><code className="language-shell">{`kronk model pull ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0`}</code></pre>
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
          <blockquote><strong>Note:</strong> Despite the <code>image_url</code> field name, Kronk does not download images</blockquote>
          <blockquote>from <code>http://</code> or <code>https://</code> URLs. The client must download the image and send</blockquote>
          <blockquote>it as a base64 data URL or raw base64 value.</blockquote>
          <p>This shell example expands the base64 value before sending the request:</p>
          <pre className="code-block"><code className="language-shell">{`IMAGE_B64=$(base64 < photo.jpg | tr -d '\\n')

curl http://localhost:11435/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  --data-binary @- <<EOF
{
  "model": "ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0",
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
          <p>Use a multimodal chat model when you need conversational questions, summaries, or reasoning about audio. For a dedicated speech-to-text API, use Bucky's <code>POST /v1/audio/transcriptions</code> endpoint described in <a href="https://www.kronkai.com/manual#1861-request-and-response">Chapter 18</a>.</p>
          <h2 id="115-go-sdk-helpers">11.5 Go SDK Helpers</h2>
          <p>Go applications can read media into a byte slice and use:</p>
          <ul>
            <li><code>model.ImageMessage(question, image, format)</code>; or</li>
            <li><code>model.AudioMessage(question, audio, format)</code>.</li>
          </ul>
          <p>These helpers create one user turn with media before text. <code>model.VideoMessage</code> constructs a <code>video_url</code> part, but it does not add video-container decoding; send extracted frames with <code>ImageMessage</code> for the current media path.</p>
          <h2 id="116-configuration-and-resources">11.6 Configuration and Resources</h2>
          <p>Multimodal requests use the same batch engine and concurrency controls as text requests. The projector adds weights and runtime buffers, while image resolution, audio duration, context length, and <code>nseq-max</code> affect resource use. Use the BUI VRAM Calculator rather than adding model, projector, and KV file sizes as a complete memory estimate. See <a href="https://www.kronkai.com/manual#36-memory-planning-and-quantization">Chapter 3 §3.6</a> for memory planning and <a href="https://www.kronkai.com/manual#chapter-4-batch-processing">Chapter 4</a> for concurrency.</p>
          <p>Most deployments should leave <code>prefill-batch-size</code> at its 2048-token default. A multimodal encoder may require an entire media-token chunk to fit in one batch, so lowering <code>prefill-batch-size</code> below that capacity can break media input. <code>proj-on-cpu: true</code> can keep the projector on the CPU when accelerator memory is constrained, at a performance cost. This relieves discrete VRAM but does not remove the projector's memory cost on unified-memory systems.</p>
          <p>Media preparation is scheduled alongside other requests, but an individual image or audio chunk can still monopolize computation while it is encoded or decoded. Do not assume that a text-prefill chunk limit bounds every media operation's latency.</p>
          <h2 id="117-message-caching">11.7 Message Caching</h2>
          <p>Incremental Message Caching can reuse unchanged media state for text-only follow-up turns without repeating model media encoding/prefill for the cached portion. Continue sending the complete conversation, including original media; IMC does not reconstruct omitted inputs. Appending media can also reuse a compatible prefix when native-stream and attention-mode checks permit. Changing, reordering, or removing cached inputs requires another compatible session or rebuilding the prefix. See <a href="https://www.kronkai.com/manual#54-media-requests">Chapter 5 §5.4</a> for the cache behavior and limitations.</p>
          <h2 id="118-limitations">11.8 Limitations</h2>
          <ul>
            <li>Media must be embedded as base64; Kronk does not fetch remote URLs.</li>
            <li>The current path accepts image and audio containers, not video containers.</li>
            <li>The selected model and projector must support the detected modality.</li>
            <li>Image resolution, media count, and audio duration affect latency and memory.</li>
            <li>Model quality and practical media limits vary by model and projector.</li>
            <li>Kronk does not impose fixed media payload, image-dimension, or media-count limits. Image decoding allocates memory in proportion to decoded dimensions, and a request can retain multiple decoded media items at once. Applications that accept media from untrusted sources must validate those resources before passing them to Kronk.</li>
          </ul>
          <hr />
          <p><em>Next: &lt;a href="https://www.kronkai.com/manual#chapter-12-security-and-authentication"&gt;Chapter 12: Security & Authentication&lt;/a&gt;</em></p>
          <h2 id="chapter-12-security-and-authentication">Chapter 12: Security and Authentication</h2>
          <p>Kronk signs JWT bearer tokens with local RSA keys. A token can be an unrestricted administrator credential or a user credential limited to specific inference endpoints and request quotas.</p>
          <h2 id="121-authorization-modes">12.1 Authorization Modes</h2>
          <p><code>KRONK_AUTHORIZATION_MODE</code> selects the API access policy independently of whether Kronk uses its embedded auth service or <code>KRONK_AUTH_HOST</code>:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Value</th>
                <th>Model discovery</th>
                <th>Inference</th>
                <th>Management</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>open</code></td>
                <td>Public</td>
                <td>Public</td>
                <td>Public</td>
              </tr>
              <tr>
                <td><code>management</code></td>
                <td>Public</td>
                <td>Public</td>
                <td>Admin JWT</td>
              </tr>
              <tr>
                <td><code>authenticated</code></td>
                <td>Valid JWT</td>
                <td>Valid JWT without an endpoint grant</td>
                <td>Admin JWT</td>
              </tr>
              <tr>
                <td><code>full-protected</code></td>
                <td>Valid JWT</td>
                <td>JWT with the matching endpoint grant</td>
                <td>Admin JWT</td>
              </tr>
            </tbody>
          </table>
          <p>Health endpoints remain public in every mode. Model discovery consists of <code>GET /v1/models</code> and <code>GET /v1/models/&#123;model&#125;</code>. Native Kronk model, integrity, catalog, device, diagnostic, pool, playground, and security endpoints are management APIs.</p>
          <p>For example:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_AUTHORIZATION_MODE=full-protected
export KRONK_WEB_ADMIN_ENABLED=false
kronk server start`}</code></pre>
          <p>The default authorization mode is <code>open</code>.</p>
          <p><code>KRONK_AUTH_HOST</code> selects the authentication provider. The table assumes that provider enforces authentication. The standalone auth service defaults to <code>AUTH_AUTH_ENABLED=false</code>, which bypasses token validation; set it to <code>true</code> before relying on a protected Kronk mode. Selecting a protected mode on Kronk alone cannot make a disabled external provider enforce tokens. <code>KRONK_WEB_ADMIN_ENABLED</code> independently controls whether the BUI is served. Replace the built-in browser password or disable the BUI before remote use. When using an external auth host with a mode that protects management, disable the BUI because its password login uses the embedded security store.</p>
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
          <pre className="code-block"><code className="language-shell">{`set +x
export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)`}</code></pre>
          <p>Admin tokens bypass endpoint grants and rate limits. The security CLI requires <code>KRONK_TOKEN</code> for key and token commands. In protected modes, the server also verifies that it is an admin token.</p>
          <p>Create a short-lived application token with exact endpoint grants:</p>
          <pre className="code-block"><code className="language-shell">{`kronk security token create \\
  --duration 24h \\
  --endpoints chat-completions,responses,messages`}</code></pre>
          <p><code>--duration</code> uses Go duration syntax such as <code>1h</code>, <code>24h</code>, or <code>720h</code>; it does not accept <code>30d</code>. Every generated token receives a unique subject UUID, so authorization and quotas are per token rather than per named human or account.</p>
          <h2 id="124-endpoint-grants-and-rate-limits">12.4 Endpoint Grants and Rate Limits</h2>
          <p>Endpoint grants and quotas are enforced only in <code>full-protected</code> mode; admin tokens bypass both. In <code>authenticated</code> mode a valid user token can call inference without endpoint grants, and its endpoint quotas are not counted.</p>
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
                <td><code>decision</code></td>
                <td><code>POST /v1/systemone</code> and <code>/v1/decide</code></td>
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
                <td><code>POST /v1/audio/transcriptions</code>, <code>POST /v1/audio/translations</code></td>
              </tr>
              <tr>
                <td><code>image-generations</code></td>
                <td><code>POST /v1/images/generations</code>, <code>POST /v1/images/edits</code>, <code>GET /v1/images/events</code></td>
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
    "model": "unsloth/Qwen3-1.7B-UD-Q8_K_XL",
    "messages": [{"role": "user", "content": "Hello"}]
  }'`}</code></pre>
          <p>With enforcing authentication, Kronk verifies the signature, issuer, expiration, and required admin status before processing a protected request. In <code>full-protected</code>, user requests also require the endpoint grant and available quota. Authentication, missing-grant, and exhausted-quota failures return <code>401 Unauthorized</code>, <code>403 Forbidden</code>, and <code>429 Too Many Requests</code>, respectively. Authentication service failures return <code>500 Internal Server Error</code>, while unavailable external auth services return <code>503 Service Unavailable</code>.</p>
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
          <p>Local mode still requires <code>KRONK_TOKEN</code> containing a valid local admin token. Use it while the server is stopped because the local command opens the same Badger database. The local security CLI currently uses the issuer <code>kronk project</code> rather than reading the server's configured issuer; tokens issued with a custom issuer cannot be administered through this local path. Use server-backed commands with the matching provider instead.</p>
          <h2 id="127-embedded-and-standalone-authentication">12.7 Embedded and Standalone Authentication</h2>
          <p>Kronk uses embedded authentication by default. Relevant server settings are:</p>
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
                <td><code>--authorization-mode</code></td>
                <td><code>KRONK_AUTHORIZATION_MODE</code></td>
                <td>Select <code>open</code>, <code>management</code>, <code>authenticated</code>, or <code>full-protected</code>. Defaults to <code>open</code>.</td>
              </tr>
              <tr>
                <td><code>--auth-issuer</code></td>
                <td><code>KRONK_AUTH_LOCAL_ISSUER</code></td>
                <td>Set the embedded-auth JWT issuer.</td>
              </tr>
              <tr>
                <td><code>--auth-host</code></td>
                <td><code>KRONK_AUTH_HOST</code></td>
                <td>Connect to an external auth service instead.</td>
              </tr>
              <tr>
                <td><code>--auth-tls-enabled</code></td>
                <td><code>KRONK_AUTH_TLS_ENABLED</code></td>
                <td>Use TLS for an external auth connection.</td>
              </tr>
              <tr>
                <td><code>--auth-tls-ca-file</code></td>
                <td><code>KRONK_AUTH_TLS_CA_FILE</code></td>
                <td>Trust the certificates in this PEM CA file instead of the system roots.</td>
              </tr>
              <tr>
                <td><code>--auth-tls-server-name</code></td>
                <td><code>KRONK_AUTH_TLS_SERVER_NAME</code></td>
                <td>Override the TLS certificate server name.</td>
              </tr>
            </tbody>
          </table>
          <p>Setting <code>KRONK_AUTH_HOST</code> skips embedded auth startup. The standalone <code>auth</code> service uses <code>AUTH_AUTH_HOST</code> (default <code>0.0.0.0:6000</code>), <code>AUTH_AUTH_ISSUER</code> (default <code>kronk project</code>), and <code>AUTH_AUTH_ENABLED</code> (default <code>false</code>). Its separate debug listener defaults to <code>0.0.0.0:6010</code>. Bind both listeners explicitly to loopback for local use with <code>AUTH_AUTH_HOST=127.0.0.1:6000</code> and <code>AUTH_WEB_DEBUG_HOST=127.0.0.1:6010</code>, or restrict both through network policy. Enable <code>AUTH_AUTH_ENABLED=true</code> for token enforcement. Set <code>AUTH_AUTH_TLS_ENABLED=true</code>, <code>AUTH_AUTH_TLS_CERT_FILE</code>, and <code>AUTH_AUTH_TLS_KEY_FILE</code> to serve TLS. Configure token issuance and validation consistently at the provider and configure Kronk's connection for the same transport security. External plaintext connections remain supported for compatibility but produce a startup warning.</p>
          <p>The standalone MCP service uses <code>MCP_AUTH_TLS_ENABLED</code>, <code>MCP_AUTH_TLS_CA_FILE</code>, and <code>MCP_AUTH_TLS_SERVER_NAME</code> for its connection to the auth service.</p>
          <p>CLI web mode reads <code>KRONK_WEB_API_HOST</code>, which defaults to <code>localhost:11435</code>.</p>
          <h2 id="128-production-hardening">12.8 Production Hardening</h2>
          <p>Kronk listens on <code>127.0.0.1:11435</code> and serves plain HTTP by default. Before binding Kronk to another interface or allowing traffic outside a trusted host:</p>
          <ul>
            <li>select <code>management</code>, <code>authenticated</code>, or <code>full-protected</code> authorization as appropriate; <code>management</code> leaves inference public, and only <code>full-protected</code> enforces user endpoint grants and quotas;</li>
            <li>replace the built-in browser password or disable the BUI;</li>
            <li>enable token enforcement at an external auth service before relying on it;</li>
            <li>terminate TLS at a trusted reverse proxy and do not expose the API port directly to the public internet;</li>
            <li>enable auth TLS whenever the auth service is reached across an untrusted network, or use an authenticated encrypted service mesh;</li>
            <li>restrict network access with host or cloud firewall rules;</li>
            <li>restrict <code>KRONK_WEB_CORS_ALLOWED_ORIGINS</code> instead of retaining <code>*</code>;</li>
            <li>issue separate, short-lived, least-privilege tokens for each application;</li>
            <li>set quotas based on the workload and monitor authentication failures;</li>
            <li>when an application forwards untrusted media, validate request size, media count, payload size, and decoded image dimensions before forwarding it;</li>
            <li>protect and back up <code>master.pem</code> and <code>master.jwt</code>; and</li>
            <li>rotate non-master signing keys deliberately, accounting for all tokens that a deletion will revoke.</li>
          </ul>
          <hr />
          <p><em>Next: &lt;a href="https://www.kronkai.com/manual#chapter-13-browser-ui-bui"&gt;Chapter 13: Browser UI (BUI)&lt;/a&gt;</em></p>
          <h2 id="chapter-13-browser-ui-bui">Chapter 13: Browser UI (BUI)</h2>
          <p>Kronk includes a Browser UI (BUI) for managing a local server and trying models interactively. It is bundled in the <code>kronk</code> binary, served from the same port as the Web API, and uses the server's <code>/v1</code> endpoints for server operations. Chat history and some preferences are stored locally in the browser; they do not automatically follow you to another browser or machine.</p>
          <p>This chapter describes the main areas of the BUI without cataloging every control. The CLI remains useful for scripting and headless administration; the BUI is not intended to duplicate every CLI command.</p>
          <h3 id="131-accessing-the-bui">13.1 Accessing the BUI</h3>
          <p>The BUI is enabled by default. Start the server and open:</p>
          <pre className="code-block"><code>{`http://localhost:11435/admin/`}</code></pre>
          <p>The address comes from <code>KRONK_WEB_API_HOST</code>, whose default is <code>127.0.0.1:11435</code>. The server root and <code>/admin</code> redirect to <code>/admin/</code> while the BUI is enabled.</p>
          <p>For a headless deployment, disable it with either form:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_WEB_ADMIN_ENABLED=false
kronk server start`}</code></pre>
          <pre className="code-block"><code className="language-shell">{`kronk server start --web-admin-enabled=false`}</code></pre>
          <h3 id="132-capabilities">13.2 Capabilities</h3>
          <p>The sidebar groups related operations by subsystem.</p>
          <h4 id="apps">Apps</h4>
          <ul>
            <li><strong>Chat</strong> provides multi-turn conversations, model selection, system prompts, chat history, and sampling controls.</li>
            <li><strong>Decision</strong> runs a customer-support example against a decision model from the catalog. The model selector shows download readiness and disables evaluation until the selected model is downloaded and validated. Edit the shared customer state, inspect the fixed choice, score, and Noul questions, then review typed answers, probability distributions, token usage, and the raw <code>/v1/decide</code> response.</li>
            <li><strong>VRAM Calculator</strong> estimates model memory requirements from a HuggingFace model without downloading the entire model. A calculator is also available in local model and catalog details. Set the intended context, sequence slots, KV precision and placement, layer/expert offload, devices, and tensor split before comparing the result with available memory. Estimates do not reserve memory or guarantee that a model will load.</li>
            <li><strong>Translator</strong> records or uploads audio for transcription through Bucky. You can select a whisper model, language, and response format and inspect timestamped segments. See <a href="https://www.kronkai.com/manual#185-browser-ui">Chapter 18 §18.5</a>.</li>
            <li><strong>Image Generator</strong> creates an image from a text prompt or transforms an uploaded PNG or JPEG with an installed Malina model. Generation settings include negative prompt, output size, steps, CFG scale, seed, and image-to-image strength. The result includes a download action and image dimensions. A server-wide activity panel displays Malina model-loading and generation progress. See <a href="https://www.kronkai.com/manual#chapter-19-malina-image-generation">Chapter 19</a>.</li>
          </ul>
          <h4 id="system">System</h4>
          <ul>
            <li><strong>Info</strong> reports server, host, device, library, and model diagnostics.</li>
            <li><strong>Running</strong> shows models that are loading or resident in the pool, along with the current resource budget. Models can also be unloaded here.</li>
            <li><strong>Slots</strong> shows live generation slots, request age, prefill/generation status, and progress for diagnosing concurrent workloads. Embedding and reranking models do not have generation slots and are omitted.</li>
            <li><strong>IMC Sessions</strong> has separate tabs for bounded working sessions and model-owned, reusable System prompt caches.</li>
          </ul>
          <h4 id="kronk">Kronk</h4>
          <ul>
            <li><strong>Models</strong> lists local GGUF models and their metadata, effective configuration, sampling defaults, chat templates, and estimated VRAM. Models can be pulled from HuggingFace, copied from another Kronk Model Server (KMS), or removed. Persistent configuration is read from <code>~/.kronk/models/model_config.yaml</code>; the model details are read-only. A pull source that includes <code>owner/repo/file.gguf</code> pins that exact Hugging Face repository and file; a repository-only source opens its GGUF file list for selection.</li>
            <li><strong>Catalog</strong> browses the personal catalog at <code>~/.kronk/catalog/catalog.yaml</code>. You can refresh its on-disk state, inspect entries, pull their files, and remove entries. See Chapter 8 for how the catalog is populated and resolved.</li>
            <li><strong>Libs</strong> downloads and removes llama.cpp bundles for supported operating system, architecture, and processor combinations. Bundles are stored below <code>~/.kronk/libraries/</code>.</li>
          </ul>
          <h4 id="bucky">Bucky</h4>
          <p>Bucky has separate pages for downloading and removing whisper models and managing whisper.cpp library bundles under <code>~/.kronk/bucky-libraries/</code>. See <a href="https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription">Chapter 18</a> for installation and transcription details.</p>
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
          <p>The VRAM calculator's SWA control follows runtime precedence. An explicit calculator value wins; for an installed model, its <code>swa-full</code> model setting is next; otherwise the calculator uses the llama.cpp default shipped with the Kronk release. Leaving the control unset therefore does not mean compact SWA. The result separates model weights, KV or recurrent state, compute buffers, and CPU/GPU placement. It is an estimate rather than a reservation guarantee, so retain operating headroom and use the same settings that will load the model.</p>
          <h4 id="docs">Docs</h4>
          <p>The binary includes an offline documentation snapshot built with that Kronk release:</p>
          <ul>
            <li><strong>Manual</strong> — this manual, with chapter navigation</li>
            <li><strong>SDK</strong> — SDK and model API references with examples</li>
            <li><strong>CLI</strong> — command reference</li>
            <li><strong>Web API</strong> — inference and management endpoint reference</li>
          </ul>
          <h3 id="133-authentication">13.3 Authentication</h3>
          <p>By default, the BUI and management APIs do not require a login. To protect browser administration, select a mode that protects management and configure the SHA-256 digest of the password:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_AUTHORIZATION_MODE=management
export KRONK_WEB_ADMIN_PASSWORD_SHA_256="$(printf '%s' 'choose-a-password' | shasum -a 256 | awk '{print $1}')"
kronk server start`}</code></pre>
          <p>Login creates a 12-hour admin token in an HttpOnly, SameSite cookie by default. The browser cannot read the token, and the server uses it to authenticate the BUI's same-origin <code>/v1</code> requests. Sign out from the sidebar to end the browser session.</p>
          <p>Behind a trusted reverse proxy, preserve the public Host and set or overwrite <code>X-Forwarded-Proto</code> to reflect HTTPS. Login and unsafe cookie-authenticated requests require a matching Origin host. Keep the backend private so clients cannot bypass the proxy or spoof its forwarding headers. Signing out clears the browser cookie; it does not revoke the token itself.</p>
          <p>Set <code>kms.web.admin.session-duration</code> in <code>model_config.yaml</code>, <code>KRONK_WEB_ADMIN_SESSION_DURATION</code>, or the server start flag <code>--web-admin-session-duration</code> to override the lifetime (for example, <code>24h</code>). The duration must be at least one second and applies to both the token and cookie. It is an absolute lifetime from login, not an inactivity timeout; activity does not renew it. Changes apply to new logins, not existing sessions. A longer lifetime reduces interruptions during test sweeps but does not preserve an unfinished run across a page refresh or session expiry.</p>
          <p>Chapter 12 explains the <code>open</code>, <code>management</code>, <code>authenticated</code>, and <code>full-protected</code> modes, including TLS and reverse proxy considerations.</p>
          <h3 id="134-operational-notes">13.4 Operational Notes</h3>
          <ul>
            <li>Downloading a library bundle does not switch the libraries used by the running process. Set <code>KRONK_LIB_PATH</code> or <code>KRONK_BUCKY_LIB_PATH</code> to the selected bundle and restart the server.</li>
            <li>Model details display the effective configuration but do not persist model overrides. Catalog details display catalog metadata, files, templates, and VRAM estimates rather than the effective model configuration. Playground settings, including load mode, apply only to that test session. Edit <code>~/.kronk/models/model_config.yaml</code> and reload the model when changing persistent configuration; see Chapter 3.</li>
            <li>Closing a browser tab does not explicitly delete its playground session. Use <strong>Unload Model</strong> when finished. Otherwise, the model remains subject to the server pool's normal eviction policy and is removed on server restart.</li>
          </ul>
          <hr />
          <p><em>Next: &lt;a href="https://www.kronkai.com/manual#chapter-14-client-integration"&gt;Chapter 14: Client Integration&lt;/a&gt;</em></p>
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
          <h4 id="launch-an-isolated-session">Launch an Isolated Session</h4>
          <p>For a disposable session, use the preferred launch workflow:</p>
          <pre className="code-block"><code className="language-shell">{`kronk model pull mradermacher/Qwopus3.5-4B-Coder.Q8_0 --local
kronk server start
kronk launch opencode mradermacher/Qwopus3.5-4B-Coder.Q8_0`}</code></pre>
          <p>OpenCode uses the model ID exactly as provided, with Kronk responsible for resolving it to the downloaded model and optional configuration profile. The model must already be available from the running Kronk server. Launch never starts a server or downloads models. It connects to <code>localhost:11435</code> by default and verifies the server and selected model through <code>GET /v1/models</code>. Accepted model ID forms are <code>modelName</code>, <code>provider/modelName</code>, and <code>provider/modelName/userName</code>. Use <code>--host</code> when Kronk runs elsewhere:</p>
          <pre className="code-block"><code className="language-shell">{`kronk launch opencode mradermacher/Qwopus3.5-4B-Coder.Q8_0 \\
  --host 192.168.1.10:11435`}</code></pre>
          <p>The server remains independently managed and continues running after OpenCode exits. Its normal <code>model_config.yaml</code> resolution and AutoTune behavior apply.</p>
          <p>OpenCode runs with only the selected model in a temporary workspace and temporary XDG config, data, cache, and state directories. User and project OpenCode configuration is not loaded or changed. Kronk prints the temporary environment path before OpenCode starts and confirms its removal after OpenCode exits. In an interactive terminal, Kronk warns that OpenCode may show a blank screen for up to 20 seconds while it initializes and waits for Enter before handing over the terminal. Arguments after <code>--</code> pass directly to OpenCode:</p>
          <pre className="code-block"><code className="language-shell">{`kronk launch opencode mradermacher/Qwopus3.5-4B-Coder.Q8_0 -- --help`}</code></pre>
          <p>If OpenCode is missing, Kronk displays the official install command and <a href="https://opencode.ai/docs/">OpenCode documentation</a>, asks permission, and installs only after a yes response. To use OpenCode's official preview and confirmed removal workflow, run:</p>
          <pre className="code-block"><code className="language-shell">{`kronk launch opencode uninstall`}</code></pre>
          <p>See the <a href="https://opencode.ai/docs/cli/#uninstall">OpenCode uninstall documentation</a>. The persistent bundle installation below is an advanced alternative for users who want a reusable OpenCode configuration connected to a separately managed Kronk server.</p>
          <p>For protected inference, export a valid application token as <code>KRONK_TOKEN</code> before launching; the isolated configuration uses it for server access. The isolated launch does not connect to Kronk's MCP service. Use the persistent bundle below when that connection is needed. The base ID above avoids requiring a preconfigured <code>/AGENT</code> variant; use a variant only when the server lists it.</p>
          <h4 id="install-opencode">Install OpenCode</h4>
          <p>Install OpenCode with its official installer:</p>
          <pre className="code-block"><code className="language-shell">{`curl -fsSL https://opencode.ai/install | bash`}</code></pre>
          <p>Other installation options are listed at <a href="https://opencode.ai/download">opencode.ai/download</a>. Verify the result with:</p>
          <pre className="code-block"><code className="language-shell">{`opencode --version`}</code></pre>
          <h4 id="install-the-kronk-bundle">Install the Kronk Bundle</h4>
          <p>For advanced, persistent configuration, run this from a Kronk source checkout:</p>
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
          <p>OpenCode's model name must match a model registered in its <code>provider.kronk.models</code> map. Kronk also needs a corresponding model variant in its model configuration. For example, place this entry under the <code>models:</code> mapping in your versioned configuration file:</p>
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
          <p>For example, under the model configuration's <code>models:</code> mapping:</p>
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
          <p>That default suits Kronk running natively on the host. For Kronk started from the Compose files in <a href="https://www.kronkai.com/manual#23-container-quick-start">2.3 Container Quick Start</a>, point OpenWebUI at the container instead:</p>
          <pre className="code-block"><code className="language-shell">{`KRONK_OWU_API_URL=http://kronk:11435/v1 make owu-up`}</code></pre>
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
    if chunk.choices and chunk.choices[0].delta.content:
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
          <p><em>Next: &lt;a href="https://www.kronkai.com/manual#chapter-15-observability"&gt;Chapter 15: Observability&lt;/a&gt;</em></p>
          <h2 id="chapter-15-observability">Chapter 15: Observability</h2>
          <p>Kronk exposes health checks on the Web API and runs a separate debug server for metrics, profiling, and runtime visualization. It can also export traces to an OTLP gRPC collector such as Grafana Tempo. Structured logs are written to standard output.</p>
          <h3 id="151-debug-and-health-endpoints">15.1 Debug and Health Endpoints</h3>
          <h4 id="debug-server">Debug Server</h4>
          <p>The main API binds to <code>127.0.0.1:11435</code> by default. Observability endpoints use a separate server at <code>127.0.0.1:11445</code>:</p>
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
          <blockquote><strong>Security:</strong> The debug server has no authentication. Its built-in default</blockquote>
          <blockquote>is loopback, but YAML, environment settings, or flags can change the listener.</blockquote>
          <blockquote>Profiles and runtime data can reveal sensitive operational details. Keep it</blockquote>
          <blockquote>private or restrict port <code>11445</code> at the network boundary for remote scraping.</blockquote>
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
                <td>Embedding/reranking</td>
                <td><code>inference_requests_total</code>, <code>inference_request_duration_seconds</code>, <code>inference_active_requests</code></td>
              </tr>
              <tr>
                <td>Sequence batching</td>
                <td><code>batchseq_queue_wait_seconds</code>, <code>batchseq_items</code>, <code>batchseq_batches_total</code></td>
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
                <td><code>imc_session_state</code>, <code>imc_session_context_tokens</code>, <code>imc_session_latest_request_context_tokens</code>, <code>imc_session_peak_context_tokens</code>, <code>imc_snapshot_skipped_total</code>, <code>imc_pure_hit_stale_session_total</code></td>
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
          <p>When Prometheus runs in Docker while Kronk runs on the host, use <code>host.docker.internal:11445</code>, as the repository's configuration does. Kronk's debug listener must also be reachable from that container; a loopback-only listener is not portable across container runtimes. Select an appropriate private interface and restrict access rather than exposing profiling publicly.</p>
          <h4 id="promql-examples">PromQL Examples</h4>
          <p>Average end-to-end time to first token by model:</p>
          <pre className="code-block"><code className="language-promql">{`rate(model_request_ttft_seconds_sum[5m])
  / rate(model_request_ttft_seconds_count[5m])`}</code></pre>
          <p>P99 time to first token by model:</p>
          <pre className="code-block"><code className="language-promql">{`histogram_quantile(0.99,
  sum by (le, model_id) (rate(model_request_ttft_seconds_bucket[5m])))`}</code></pre>
          <p>Token throughput by model and kind:</p>
          <pre className="code-block"><code className="language-promql">{`sum by (model_id, kind) (rate(usage_tokens_total[5m]))`}</code></pre>
          <p>Average sequence-batch width by model and operation:</p>
          <pre className="code-block"><code className="language-promql">{`rate(batchseq_items_sum[5m])
  / rate(batchseq_items_count[5m])`}</code></pre>
          <p>Embedding and reranking request metrics include <code>operation</code> (<code>embedding</code> or <code>rerank</code>) and <code>runtime</code> (<code>batchseq</code> or <code>context_pool</code>) labels. Resource-manager reservation metrics already account for both runtime types because they are published for every loaded model, independently of its inference engine. The <code>inference_*</code> request metrics start after SDK admission, so they exclude admission wait and attempts that fail or are cancelled before a permit is acquired.</p>
          <p>Current IMC cache entries are exposed with <code>model_id</code> and <code>entry</code> labels. These gauges report the same bounded scalar snapshot as the BUI <strong>IMC Sessions</strong> page; they do not retain session history. The bundled Grafana dashboard presents the values in an <strong>IMC Sessions</strong> table.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Metric</th>
                <th>Meaning</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>imc_session_state</code></td>
                <td>One-hot entry state. The <code>state</code> label is <code>active</code>, <code>idle</code>, or <code>empty</code>.</td>
              </tr>
              <tr>
                <td><code>imc_session_messages</code></td>
                <td>Complete messages represented by Current.</td>
              </tr>
              <tr>
                <td><code>imc_session_context_tokens</code></td>
                <td>Physical token context represented by Current.</td>
              </tr>
              <tr>
                <td><code>imc_session_allocated_tokens</code></td>
                <td>Largest physical token context retained by Current's backing allocation.</td>
              </tr>
              <tr>
                <td><code>imc_session_latest_request_messages</code></td>
                <td>Messages in the latest completed request tracked by the entry.</td>
              </tr>
              <tr>
                <td><code>imc_session_latest_request_input_tokens</code></td>
                <td>Complete input tokens in that request, including the generation-template tail.</td>
              </tr>
              <tr>
                <td><code>imc_session_latest_request_output_tokens</code></td>
                <td>Reasoning and completion tokens generated by that request.</td>
              </tr>
              <tr>
                <td><code>imc_session_latest_request_context_tokens</code></td>
                <td>Latest request input plus generated output tokens.</td>
              </tr>
              <tr>
                <td><code>imc_session_peak_context_tokens</code></td>
                <td>Largest live context observed across the entry's requests, including generated output.</td>
              </tr>
              <tr>
                <td><code>imc_session_window_tokens</code></td>
                <td>Configured context window.</td>
              </tr>
              <tr>
                <td><code>imc_session_used_percent</code></td>
                <td>Current context divided by the configured window.</td>
              </tr>
              <tr>
                <td><code>imc_session_peak_used_percent</code></td>
                <td>Peak context divided by the configured window.</td>
              </tr>
              <tr>
                <td><code>imc_session_has_media</code></td>
                <td><code>1</code> when the retained Current snapshot includes media; otherwise <code>0</code>.</td>
              </tr>
              <tr>
                <td><code>imc_session_last_used_timestamp_seconds</code></td>
                <td>Current eviction/LRU Unix timestamp; zero when never used.</td>
              </tr>
            </tbody>
          </table>
          <p>System caches are model-owned rather than working-session metrics. Their live state is available on the <strong>System Caches</strong> tab of the BUI <strong>IMC Sessions</strong> page. The Current values can move before the latest-request values update: the former are published during prompt preparation, while the latter describe the most recently completed request. Peak context is an entry lifetime high water mark and can describe an earlier request.</p>
          <h3 id="153-bundled-observability-stack">15.3 Bundled Observability Stack</h3>
          <p>The repository includes a Docker Compose stack containing Grafana, Prometheus, Tempo, Loki, and Alloy. It provisions the data sources and a Kronk dashboard without manual Grafana setup.</p>
          <p>This is a trusted-development stack: Grafana enables anonymous Admin access and publishes port <code>3100</code> without a loopback-only binding. Restrict published ports and configure authentication before allowing access from other machines. Actual reachability depends on Docker, network, and firewall settings.</p>
          <p>Download the pinned images once, start the stack, and open Grafana:</p>
          <pre className="code-block"><code className="language-shell">{`make install-docker
make grafana-up
make grafana-browse`}</code></pre>
          <p>Grafana is served at <code>http://localhost:3100/</code>. Prometheus scrapes the host's Kronk debug server, and Tempo accepts OTLP gRPC traces on port <code>4317</code>.</p>
          <p>When Kronk runs from the Compose file in <code>zarf/docker/kronk/</code> (see <a href="https://www.kronkai.com/manual#23-container-quick-start">2.3 Container Quick Start</a>), both stacks share the <code>kronk-net</code> network. Prometheus also scrapes <code>kronk:11445</code>; the target that is not running shows as <code>DOWN</code>. If you publish the container's ports on <code>0.0.0.0</code> (<code>KRONK_BIND_ADDR</code>), both targets reach the same Kronk; filter on the <code>deployment</code> label (<code>host</code> or <code>container</code>) to avoid counting its metrics twice. To send traces to Tempo, start Kronk with <code>KRONK_TEMPO_HOST=tempo:4317</code>, and point OpenWebUI at the container with <code>KRONK_OWU_API_URL=http://kronk:11435/v1</code>.</p>
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
          <pre className="code-block"><code className="language-shell">{`go tool pprof 'http://localhost:11445/debug/pprof/profile?seconds=30'
go tool pprof 'http://localhost:11445/debug/pprof/heap'
curl 'http://localhost:11445/debug/pprof/goroutine?debug=2'`}</code></pre>
          <p>Start pprof's interactive web interface with:</p>
          <pre className="code-block"><code className="language-shell">{`go tool pprof -http=localhost:8081 \\
  'http://localhost:11445/debug/pprof/profile?seconds=30'`}</code></pre>
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
          <p><em>Next: &lt;a href="https://www.kronkai.com/manual#chapter-16-mcp-service"&gt;Chapter 16: MCP Service&lt;/a&gt;</em></p>
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
          <pre className="code-block"><code className="language-shell">{`export KRONK_MCP_BRAVE_API_KEY="<your-brave-api-key>"
kronk server start`}</code></pre>
          <p>The corresponding CLI option is <code>--mcp-brave-api-key</code>. Disable embedded MCP with <code>--mcp-enabled=false</code> or <code>KRONK_MCP_ENABLED=false</code>. To use a separately managed MCP service, set <code>KRONK_MCP_HOST</code> or pass <code>--mcp-host</code>; a non-empty value only prevents the embedded service from starting. Kronk does not connect or proxy to that address.</p>
          <p>Protect embedded MCP with the existing Kronk JWT system:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --mcp-auth-enabled --authorization-mode=management --web-admin-enabled=false`}</code></pre>
          <p><code>--mcp-auth-enabled</code> requires an admin bearer token on every MCP request; it does not protect REST administration or the BUI. The example separately selects <code>management</code> authorization and disables the BUI. Inference remains public in that mode; use <code>full-protected</code> for endpoint grants and quotas. Configure the MCP client to send <code>Authorization: Bearer &lt;admin-token&gt;</code>. Application tokens with inference endpoint grants are not sufficient for MCP access. Before exposing the model server outside a trusted host, replace the BUI's default <code>kronk</code> password as described in <a href="https://www.kronkai.com/manual#133-authentication">Chapter 13</a> or disable the BUI; otherwise that known password can be exchanged for an admin session.</p>
          <p>Start the standalone service with:</p>
          <pre className="code-block"><code className="language-shell">{`export MCP_MCP_BRAVE_API_KEY="<your-brave-api-key>"
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
          <p>Each tier must identify exactly one block. An absent or ambiguous match returns an error without modifying the file. The replacement may undergo line-ending or trailing-newline adjustment to match the selected tier.</p>
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
          <p>First initialize a session and copy the <code>Mcp-Session-Id</code> response header. These examples assume MCP authentication is enabled and <code>KRONK_TOKEN</code> contains an admin token. For an unprotected local listener, omit the Authorization header:</p>
          <pre className="code-block"><code className="language-shell">{`curl -i http://localhost:9000/mcp \\
  -H 'Content-Type: application/json' \\
  -H 'Accept: application/json, text/event-stream' \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -d '{
    "jsonrpc": "2.0", "id": 1, "method": "initialize",
    "params": {
      "protocolVersion": "2025-03-26",
      "capabilities": {},
      "clientInfo": {"name": "curl-client", "version": "1.0.0"}
    }
  }'`}</code></pre>
          <p>Then send the required initialized notification:</p>
          <pre className="code-block"><code className="language-shell">{`SESSIONID="<session-id-from-response>"
curl http://localhost:9000/mcp \\
  -H 'Content-Type: application/json' \\
  -H 'Accept: application/json, text/event-stream' \\
  -H 'MCP-Protocol-Version: 2025-03-26' \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Mcp-Session-Id: $SESSIONID" \\
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}'`}</code></pre>
          <p>The session can now list and call tools:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:9000/mcp \\
  -H 'Content-Type: application/json' \\
  -H 'Accept: application/json, text/event-stream' \\
  -H 'MCP-Protocol-Version: 2025-03-26' \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Mcp-Session-Id: $SESSIONID" \\
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'

curl http://localhost:9000/mcp \\
  -H 'Content-Type: application/json' \\
  -H 'Accept: application/json, text/event-stream' \\
  -H 'MCP-Protocol-Version: 2025-03-26' \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Mcp-Session-Id: $SESSIONID" \\
  -d '{
    "jsonrpc": "2.0", "id": 3, "method": "tools/call",
    "params": {
      "name": "web_search",
      "arguments": {"query": "what is the Model Context Protocol", "count": 5}
    }
  }'`}</code></pre>
          <p>When MCP authentication is enabled, include <code>Authorization: Bearer &lt;admin-token&gt;</code> in every initialization, notification, tool-listing, tool-call, and session-deletion request.</p>
          <p>If the service restarts, initialize a new session instead of reusing the old ID.</p>
          <hr />
          <p><em>Next: &lt;a href="https://www.kronkai.com/manual#chapter-17-troubleshooting"&gt;Chapter 17: Troubleshooting&lt;/a&gt;</em></p>
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
          <p>Readiness currently returns an empty <code>200 OK</code> when the HTTP service is running. It does not verify libraries, devices, memory, loaded models, or inference. Metrics and profiles are served from the unauthenticated debug server; see <a href="https://www.kronkai.com/manual#151-debug-and-health-endpoints">Chapter 15</a> before exposing that port.</p>
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
          <p><code>kronk diagnose</code> probes every installed bundle independently. When the selected backend reports no GPU but a same-version installed alternative detects one, the report recommends the corresponding <code>KRONK_PROCESSOR</code> override. Restart Kronk after applying it.</p>
          <p>Not every combination is published. Check the live matrix with <code>kronk libs --list-combinations</code>. See <a href="https://www.kronkai.com/manual#24-libraries">Chapter 2: Libraries</a> for installation and path details.</p>
          <h4 id="nvidia-is-visible-but-llamacpp-uses-the-cpu">NVIDIA is visible but llama.cpp uses the CPU</h4>
          <p><code>nvidia-smi</code> proves that the driver is available, but a native CUDA bundle also needs the CUDA runtime libraries against which it was linked. On Linux, find the active library path in <code>kronk diagnose</code>, then inspect the backend for unresolved dependencies:</p>
          <pre className="code-block"><code className="language-shell">{`ldd <lib-path>/libggml-cuda.so | grep -iE 'not found|cudart|cublas'`}</code></pre>
          <p>Install the matching CUDA runtime packages for the bundle and operating system. For containers, use the current <code>latest-cuda</code> image and grant GPU access with <code>--runtime=nvidia --gpus all</code>, or with Compose use <code>compose.cuda.yaml</code> from <a href="https://www.kronkai.com/manual#87-container-operations">8.7 Container Operations</a>; the required runtime libraries are included in that image.</p>
          <h4 id="container-fails-with-`unknown-or-invalid-runtime-name-nvidia`">Container fails with `unknown or invalid runtime name: nvidia`</h4>
          <p>Docker does not know the <code>nvidia</code> runtime that <code>--runtime=nvidia</code> and <code>compose.cuda.yaml</code> request. Install the <a href="https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html">NVIDIA Container Toolkit</a>, register the runtime, and restart Docker:</p>
          <pre className="code-block"><code className="language-shell">{`sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker`}</code></pre>
          <h4 id="permission-denied-on-`devdri`-or-`devkfd`-in-a-container">Permission denied on `/dev/dri` or `/dev/kfd` in a container</h4>
          <p>The container user reaches GPU devices through the <code>render</code> and <code>video</code> groups, which the image maps to GIDs 110 and 44. If the host uses other GIDs, look them up and pass them to Compose:</p>
          <pre className="code-block"><code className="language-shell">{`getent group render video
KRONK_RENDER_GID="<render-GID>" KRONK_VIDEO_GID="<video-GID>" docker compose -f compose.vulkan.yaml up -d`}</code></pre>
          <p>With <code>docker run</code>, pass the numbers to <code>--group-add</code> instead. On hosts with SELinux in enforcing mode, also check the audit log for denied device access.</p>
          <h4 id="a-library-update-introduced-crashes-or-bad-output">A library update introduced crashes or bad output</h4>
          <p>The normal CLI installs Kronk's pinned default. <code>--upgrade</code> and the server's <code>--allow-upgrade=true</code> opt into newer llama.cpp releases. List installed bundles and pin a known-good version when investigating a regression:</p>
          <pre className="code-block"><code className="language-shell">{`kronk libs --list-installs
kronk libs --local --version=b5490
kronk server start --lib-version=b5490`}</code></pre>
          <p>Unset <code>KRONK_LIB_VERSION</code> after the pinned default contains the required fix. Libraries are not hot-reloaded; restart the server after switching <code>KRONK_LIB_PATH</code>.</p>
          <p>Also check native backend environment values. Starting with b11696, <code>GGML_CUDA_ENABLE_UNIFIED_MEMORY</code>, <code>GGML_CUDA_NO_PINNED</code>, and <code>GGML_CUDA_REGISTER_HOST</code> require numeric nonzero values such as <code>1</code>; <code>true</code>, <code>yes</code>, empty values, and <code>0</code> no longer activate them. Restart after changing these settings. See <a href="https://www.kronkai.com/manual#native-backend-environment-settings">Native backend environment settings</a> for details.</p>
          <p>Native kernel changes can alter floating-point rounding without changing Kronk's embedding or sampling algorithms. For example, b11696 changes CUDA/HIP RMSNorm reductions and adds an eligible Metal BF16 Flash Attention path. Exact embeddings, logits, and generated text can differ across native versions, even with a fixed seed. Compare representative output quality and numerical tolerances rather than requiring byte-for-byte equality. Crashes, NaNs, or materially degraded results are not ordinary rounding differences; investigate and report them with the model, backend, library versions, and reproduction steps.</p>
          <h3 id="173-models-catalog-and-storage">17.3 Models, Catalog, and Storage</h3>
          <h4 id="a-downloaded-model-is-not-listed">A downloaded model is not listed</h4>
          <p>Kronk stores data beneath <code>KRONK_BASE_PATH</code>, which defaults to <code>~/.kronk</code>. Inspect the model index rather than relying on a hard-coded directory:</p>
          <pre className="code-block"><code className="language-shell">{`kronk model list --local
kronk model index --local`}</code></pre>
          <p>Indexing scans model files and checks available size and SHA metadata. An arbitrary GGUF without corresponding checksum metadata cannot receive the same integrity validation as a model downloaded by Kronk.</p>
          <h4 id="a-model-is-missing-incomplete-or-corrupt">A model is missing, incomplete, or corrupt</h4>
          <p>Pull it again using any supported source form:</p>
          <pre className="code-block"><code className="language-shell">{`kronk model pull <model-id> --local`}</code></pre>
          <p>For canonical <code>provider/modelID</code> sources, <code>model pull</code> checks the specified provider when unresolved. URLs, exact file pins, and supported quantization shorthands also work. A separate <code>model resolve</code> step is not normally required. Interrupted downloads are resumable; if a file remains invalid, remove that model through Kronk and pull it again:</p>
          <pre className="code-block"><code className="language-shell">{`kronk model remove <model-id> --local
kronk model pull <model-id> --local`}</code></pre>
          <p>For a gated or private Hugging Face repository, provide a read token:</p>
          <pre className="code-block"><code className="language-shell">{`export KRONK_HF_TOKEN=hf_xxx
kronk model pull <model-id> --local`}</code></pre>
          <h4 id="`catalogyaml`-was-hand-edited-and-no-longer-parses">`catalog.yaml` was hand-edited and no longer parses</h4>
          <p>The default catalog is <code>&lt;base&gt;/catalog/catalog.yaml</code>. Restore valid YAML from a backup before running catalog commands; those commands must parse the file and cannot repair malformed YAML. Do not use <code>catalog remove</code> as a syntax-repair tool because it also removes the selected model's downloaded files.</p>
          <p>Catalog administration is covered in <a href="https://www.kronkai.com/manual#86-catalog-operations">Chapter 8</a>.</p>
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
          <p>Use the BUI VRAM Calculator for a model-specific estimate and retain headroom. See <a href="https://www.kronkai.com/manual#36-memory-planning-and-quantization">Chapter 3: Memory Planning</a>.</p>
          <h4 id="`input-tokens-[n]-exceed-context-window-[m]`">`input tokens [N] exceed context window [M]`</h4>
          <p>The rendered prompt is already larger than the configured context. Shorten the conversation or system prompt, or increase <code>context-window</code> if memory permits. Cached prefix tokens still consume context capacity.</p>
          <h4 id="`the-context-window-is-full`">`the context window is full`</h4>
          <p>Input plus generated tokens exhausted the context during inference. Request fewer output tokens, shorten the input, or increase the context. YaRN may extend supported RoPE models, but it is not a generic memory fix; follow <a href="https://www.kronkai.com/manual#chapter-7-yarn-extended-context">Chapter 7</a>.</p>
          <h4 id="slow-inference-or-slow-time-to-first-token">Slow inference or slow time to first token</h4>
          <p>Start with <code>kronk diagnose</code> and confirm that llama.cpp sees the expected GPU. A cold request includes model loading, and a large uncached prompt includes prefill. Partial CPU offload can reduce token throughput. Compare representative requests after the model is warm rather than relying on the first request.</p>
          <p>Model loading defaults to <code>load-mode: auto</code>, which uses mmap when every selected device supports it and otherwise uses ordinary loading. Do not switch to <code>direct-io</code> solely because a GPU is present: its performance depends on the model's actual storage device and filesystem, and bypassing the page cache can make repeated loads slower. <code>mlock</code> stabilizes residency after loading but does not force mmap; use <code>mmap+mlock</code> when both behaviors are required. Both modes require enough lockable RAM and are not general cold-start optimizations. See <a href="https://www.kronkai.com/manual#model-weight-loading">Chapter 3 §3.4</a> before changing the mode.</p>
          <p>Use Chapter 15's request, queue, prefill, TTFT, token-rate, and pool metrics to separate loading, waiting, prompt processing, and generation. IMC-specific diagnosis is below.</p>
          <h3 id="175-requests-and-streaming">17.5 Requests and Streaming</h3>
          <h4 id="`context-deadline-exceeded`">`context deadline exceeded`</h4>
          <p>The source of the deadline matters:</p>
          <ul>
            <li>a client or reverse proxy may cancel first;</li>
            <li>admission waits are capped at three minutes before model processing starts;</li>
            <li>the inference deadline defaults to 60 minutes, covering admission, preparation, slot waiting, and generation;</li>
            <li>the HTTP server defaults to a 30-second read timeout and a 61-minute write timeout.</li>
          </ul>
          <p><code>--read-timeout</code> covers reading the request, not model execution. Increase the inference deadline only for intentionally longer operations, and keep a nonzero write timeout greater than it:</p>
          <pre className="code-block"><code className="language-shell">{`kronk server start --inference-timeout 90m --write-timeout 91m`}</code></pre>
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
  --endpoints chat-completions,decision,embeddings,rerank,responses,messages,tokenize,transcriptions,image-generations`}</code></pre>
          <p>Rate limits use forms such as <code>chat-completions:10000/day</code>. Token creation, key rotation, and production hardening are covered in <a href="https://www.kronkai.com/manual#chapter-12-security-and-authentication">Chapter 12</a>.</p>
          <h3 id="177-imc">17.7 IMC</h3>
          <p>IMC is enabled by default. It externalizes cached session state to the built-in RAM store or, for direct SDK use, a configured custom session store. See <a href="https://www.kronkai.com/manual#chapter-5-message-caching">Chapter 5</a> for its lifecycle and settings.</p>
          <h4 id="every-turn-rebuilds-the-cache">Every turn rebuilds the cache</h4>
          <p>Common causes are changed earlier messages, changed template inputs, a prompt below <code>cache-min-tokens</code>, or cache pressure. Relevant JSON logs use status <code>plan-ready</code>; inspect <code>match_kind</code> (<code>exact</code>, <code>append</code>, <code>anchor</code>, or <code>rebuild</code>), <code>match_reason</code>, and the reusable/extension token counts. Status <code>plan-fallback</code> identifies prompts that cannot use the token planner safely.</p>
          <p>Keep earlier conversation messages stable and use a deterministic template. Increase <code>nseq-max</code> only when additional inference concurrency and its memory cost are both appropriate; IMC maintains more session identities than active decode slots.</p>
          <h4 id="`server-busy-processing-other-requests-try-again-shortly`">`server busy processing other requests, try again shortly`</h4>
          <p>No IMC session was available. Kronk returns this retryable error immediately rather than waiting at the session-planning stage. Wait and retry from the client. If it is frequent, inspect long-running requests and queue/cache metrics before increasing <code>nseq-max</code>.</p>
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
          <p>MCP authentication is disabled by default, and <code>fuzzy_edit</code> has the process's filesystem access. Keep it on loopback unless bearer authentication, TLS, and network restrictions are configured. See <a href="https://www.kronkai.com/manual#chapter-16-mcp-service">Chapter 16</a> for configuration and the complete handshake.</p>
          <h3 id="179-ports-processes-and-permissions">17.9 Ports, Processes, and Permissions</h3>
          <p>Default listeners are <code>11435</code> for the API, <code>11445</code> for model-server debugging, and <code>9000</code> for embedded MCP. Standalone MCP also starts a debug listener on <code>9010</code>. Find a conflicting process before changing ports:</p>
          <pre className="code-block"><code className="language-shell">{`lsof -nP -iTCP:11435 -sTCP:LISTEN
lsof -nP -iTCP:9000 -sTCP:LISTEN`}</code></pre>
          <p>Move the API or debug listener with <code>--api-host</code> or <code>--debug-host</code>. Standalone MCP uses <code>MCP_MCP_HOST</code> and <code>MCP_WEB_DEBUG_HOST</code>.</p>
          <p>Detached mode stores <code>kronk.pid</code> and <code>kronk.log</code> under <code>KRONK_BASE_PATH</code>. If <code>kronk server stop</code> encounters a stale PID, verify that no Kronk process owns the API or debug port before removing only the stale PID file.</p>
          <p>BadgerDB also permits only one model-server process to use the rate-limit database. A lock error means another process owns <code>&lt;base&gt;/badger</code>; stop that process. Do not delete Badger's <code>LOCK</code> file while a server may be running.</p>
          <p>For permission errors, make the selected base path writable by the service user. The server enforces mode <code>0700</code> on <code>&lt;base&gt;/keys</code> and <code>0600</code> on private key files. Avoid recursively making credentials readable by other users.</p>
          <p>For the Linux user service, use <code>kronk systemd status</code> and <code>kronk systemd logs</code>, not the detached-server commands. A failure to connect to the user bus means the systemd user manager is unavailable; run as your normal user in a systemd login session, not with <code>sudo</code>. To run at boot without login, enable lingering as described in <a href="https://www.kronkai.com/manual#88-running-as-a-systemd-service">Chapter 8 §8.8</a>.</p>
          <p>If the service cannot execute Kronk after moving the binary, reinstall with <code>kronk systemd install --force</code>, then restart. If GPU detection differs from your interactive shell, check the service's <code>PATH</code>, native library paths, device permissions, and GPU environment settings in <code>kronk.env</code> (§8.8). After five failed starts within five minutes, fix the journal's reported error, run <code>systemctl --user reset-failed kronk</code>, then <code>kronk systemd start</code>.</p>
          <p>For the macOS LaunchAgent, use <code>kronk launchd status</code> and <code>kronk launchd logs</code>. A failure to find the <code>gui/&lt;uid&gt;</code> domain means a macOS desktop login session is unavailable; this agent is not a headless system service. Run as your normal user, not with <code>sudo</code>. A missing agent after <code>kronk launchd stop</code> is expected: <code>stop</code> unloads it to prevent <code>KeepAlive</code> from restarting it. Use <code>kronk launchd start</code> to load it again.</p>
          <p>If launchd keeps restarting a failing process, stop the agent and inspect <code>~/Library/Logs/kronk/kronk.log</code>. Check the recorded executable path, port conflicts, and the plist's <code>EnvironmentVariables</code>; shell startup files and <code>kronk.env</code> are not read. Validate manual edits with <code>plutil -lint</code> and run <code>kronk launchd restart</code> to reload them. See <a href="https://www.kronkai.com/manual#89-running-as-a-launchd-agent">Chapter 8 §8.9</a>.</p>
          <p>Whisper-specific failures are listed in <a href="https://www.kronkai.com/manual#189-troubleshooting">Chapter 18 §18.9</a>.</p>
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
          <p>For a suspected API-contract problem — constrained decoding, tool-call parsing, speculative decoding, logprobs, the Anthropic or Responses translations, or parameter validation — reproduce it with the adversarial harness from a clone of the repository and attach the relevant part of the report:</p>
          <pre className="code-block"><code className="language-shell">{`make test-adversarial ARGS="--tier=smoke"`}</code></pre>
          <p>Findings land in <code>.tools/adversarial/output/findings.txt</code>, which includes the full request and response of every flagged call. Name individual probe groups to narrow the run, and see <a href="https://www.kronkai.com/manual#2042-server-adversarial-probes">Chapter 20.4.2</a> for tiers, environment variables, and the full output layout.</p>
          <hr />
          <p><em>Next: &lt;a href="https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription"&gt;Chapter 18: Bucky (Audio Transcription)&lt;/a&gt;</em></p>
          <h2 id="chapter-18-bucky-audio-transcription">Chapter 18: Bucky (Audio Transcription)</h2>
          <p>Bucky is Kronk's speech-to-text subsystem. It uses <a href="https://github.com/ggerganov/whisper.cpp"><code>whisper.cpp</code></a> and is available through:</p>
          <ul>
            <li>the <code>/v1/audio/transcriptions</code> and <code>/v1/audio/translations</code> HTTP endpoints;</li>
            <li>the Browser UI (BUI) Translator;</li>
            <li>the <code>kronk bucky</code> management commands; and</li>
            <li>the Go packages under <code>sdk/bucky</code>.</li>
          </ul>
          <p>Using Bucky requires both a compatible whisper.cpp library bundle and a Whisper model. The libraries run the inference engine; the model contains the speech-recognition weights.</p>
          <p>Developer-level package, lifecycle, and test information belongs in <a href="https://www.kronkai.com/manual#chapter-20-developer-guide">Chapter 20: Developer Guide</a>.</p>
          <h3 id="181-overview">18.1 Overview</h3>
          <p>Bucky supports these common workflows:</p>
          <ul>
            <li>transcribe an uploaded or recorded audio file;</li>
            <li>translate speech from a supported language into English;</li>
            <li>return plain text, JSON, SRT, or WebVTT;</li>
            <li>transcribe separate audio channels as separate speakers through the SDK; and</li>
            <li>consume partial and final transcript events from live audio through the SDK.</li>
          </ul>
          <p>The HTTP endpoint follows the OpenAI audio transcription request shape. It is protected by Kronk's <code>transcriptions</code> endpoint grant in <code>full-protected</code> mode. <code>authenticated</code> requires a valid token without grants; <code>open</code> and <code>management</code> leave inference public. See Chapter 12.</p>
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
                <td><code>cpu</code>, <code>cuda</code>, <code>cuda12</code>, <code>cuda13</code>, <code>vulkan</code></td>
              </tr>
              <tr>
                <td>Windows</td>
                <td><code>amd64</code></td>
                <td><code>cpu</code>, <code>cuda</code>, <code>cuda12</code></td>
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

# Install the Linux CUDA 13 bundle alongside CUDA 12.
kronk bucky libs --install --arch=amd64 --os=linux --processor=cuda13

# List or remove installed bundles.
kronk bucky libs --list-installs
kronk bucky libs --remove-install --arch=amd64 --os=linux --processor=cuda`}</code></pre>
          <p>The commands use the running model server by default. Add <code>--local</code> to manage the files directly without a server.</p>
          <p>The BUI's <strong>Whisper Libraries</strong> screen provides the same installation and removal operations when the server is running. Default library installation or verification failures stop startup; install a compatible managed bundle with <code>kronk bucky libs --local</code>, then <strong>restart the server</strong>. If startup reached native initialization but Bucky failed, the server can remain running in degraded mode and the BUI can manage libraries. It does not automatically retry initialization.</p>
          <p>Libraries are installed below <code>~/.kronk/bucky-libraries/</code> by default. Kronk normally treats the detected processor as a preference and verifies that the corresponding whisper.cpp bundle can run on the host. CPU and Metal are retained when published for the platform. CUDA is retained only when <code>nvidia-smi</code> reports a compatible driver and every GPU visible through <code>CUDA_VISIBLE_DEVICES</code> meets the bundle's compute-capability requirement:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Platform</th>
                <th>Minimum reported CUDA version</th>
                <th>Minimum compute capability</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Linux <code>amd64</code></td>
                <td>12.9</td>
                <td>8.6</td>
              </tr>
              <tr>
                <td>Linux <code>arm64</code></td>
                <td>12.9</td>
                <td>8.7</td>
              </tr>
              <tr>
                <td>Windows <code>amd64</code></td>
                <td>12.4</td>
                <td>5.0</td>
              </tr>
            </tbody>
          </table>
          <p><code>cuda</code> and <code>cuda12</code> download the same CUDA 12 bundle into separate processor directories. On Linux, automatic detection with a <code>cuda</code> preference first tries <code>cuda13</code>: it requires a reported CUDA version of at least 13.0, the same GPU capability thresholds above, and installed <code>libcudart.so.13</code> and <code>libcublas.so.13</code>. If those checks fail, it tries CUDA 12. Linux CUDA 12 also requires installed <code>libcudart.so.12</code> and <code>libcublas.so.12</code>. Library discovery uses <code>ldconfig -p</code> and <code>LD_LIBRARY_PATH</code>; stale cache entries do not count. The Linux bundles do not include these NVIDIA user-space libraries. A newer driver alone does not supply them. Both CUDA runtime majors can coexist. Keep Jetson Orin on CUDA 12 unless its JetPack/runtime supports CUDA 13.</p>
          <p>An explicit <code>--install --processor=cuda13</code> downloads the requested bundle without probing the current host, allowing installation for another machine. CUDA 13 archives use <code>cuda-13</code> in their filenames; the Bucky processor option is <code>cuda13</code>, without a hyphen. Windows remains CUDA 12 only.</p>
          <p>If the preferred CUDA or Vulkan runtime is unavailable or incompatible, Linux uses Vulkan when <code>vulkaninfo --summary</code> reports a usable GPU and otherwise uses CPU. Other supported platforms fall back to CPU. Bucky has no separate ROCm artifact, so a shared <code>KRONK_PROCESSOR=rocm</code> preference resolves to Vulkan on a usable Linux Vulkan host and otherwise to CPU. Startup logs report the preferred runtime, selected runtime, and reason.</p>
          <p>To select a specific installed bundle or user-managed build authoritatively, set its directory before starting the server:</p>
          <pre className="code-block"><code className="language-sh">{`export KRONK_BUCKY_LIB_PATH=~/.kronk/bucky-libraries/linux/amd64/cuda`}</code></pre>
          <p>For CUDA 13, use the corresponding <code>linux/amd64/cuda13</code> or <code>linux/arm64/cuda13</code> directory. Prefer this Bucky-specific override rather than setting <code>KRONK_PROCESSOR=cuda13</code> for the whole server: llama.cpp uses different CUDA-major option names.</p>
          <p>A non-empty <code>KRONK_BUCKY_LIB_PATH</code> bypasses automatic compatibility selection. If the directory contains <code>version.json</code>, Kronk adopts its recorded platform metadata unless explicitly overridden. A non-empty existing directory without that file is treated as a read-only user-managed build and is not installed, upgraded, or replaced. Default verified startup requires managed installation metadata and fails for such a build. Independently trusted custom builds require an advanced startup path with managed verification explicitly disabled; this is not routine recovery for a broken installation.</p>
          <p>The default downloader installs the authenticated whisper.cpp version bound to this Bucky dependency, replacing older or arbitrary newer managed bundles rather than assuming numerical version order implies ABI compatibility. An offline installation must match the requested pin and platform. <code>--version</code> and <code>--upgrade</code> explicitly opt out of the default pin. Restart after replacing libraries. If a future upgrade changes the ABI, rebuild or replace user-managed libraries yourself; Kronk never mutates a read-only build. Do not mix ggml files from independently built Whisper and llama.cpp bundles.</p>
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
                <td><code>cpu</code>, <code>metal</code>, <code>cuda</code>, <code>cuda-12</code>, <code>cuda-13</code>, <code>vulkan</code>; <code>rocm</code> maps to Vulkan or CPU</td>
              </tr>
            </tbody>
          </table>
          <p>Only combinations listed by <code>--list-combinations</code> can be installed. Platform variables remain preferences during automatic Bucky selection; use <code>KRONK_BUCKY_LIB_PATH</code> when the selected directory must be exact. Library selection is process-wide, so restart the CLI process or server after changing it.</p>
          <p>The shared environment spellings <code>cuda-12</code> and <code>cuda-13</code> map to Bucky's <code>cuda12</code> and <code>cuda13</code> processors. For Bucky install commands, use the unhyphenated spelling shown by <code>--list-combinations</code>.</p>
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
                <td><code>tiny.en</code></td>
                <td>75 MB</td>
                <td>English only</td>
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
          <p>The catalog also contains <code>silero-vad</code>, an auxiliary voice-activity detection model. It is not a transcription model. SDK streaming uses its built-in energy detector unless the application explicitly selects the Silero model.</p>
          <p>Pull, list, and remove models with:</p>
          <pre className="code-block"><code className="language-sh">{`kronk bucky model pull tiny
kronk bucky model list
kronk bucky model remove tiny`}</code></pre>
          <p>The pull command accepts a catalog name, a GGML filename such as <code>ggml-tiny.bin</code>, or a complete download URL. The catalog name <code>tiny</code> and the filename <code>ggml-tiny.bin</code> identify the same multilingual model. English-only models use the <code>.en</code> suffix, such as <code>base.en</code>.</p>
          <p>Add <code>--local</code> to any model command to operate directly on disk. Installed models are stored below <code>~/.kronk/bucky-models/</code> by default, using filenames such as <code>ggml-tiny.bin</code> and <code>ggml-base.en.bin</code>.</p>
          <h3 id="184-server-configuration">18.4 Server Configuration</h3>
          <p>Start the model server normally after installing the libraries and at least one model. Bucky uses the standard server address, whose default is:</p>
          <pre className="code-block"><code className="language-text">{`http://localhost:11435`}</code></pre>
          <p>The server discovers installed <code>.bin</code> files and loads a model when it is first requested. Configure each model under the top-level <code>bucky-models</code> mapping in <code>~/.kronk/models/model_config.yaml</code>, using the short ID shown by <code>kronk bucky model list</code>:</p>
          <pre className="code-block"><code className="language-yaml">{`version: 2
bucky-models:
  tiny:
    nseq-max: 2
    queue-depth: 4
    admission-timeout: 3m
    nthreads: 0`}</code></pre>
          <p><code>nseq-max</code> controls the number of concurrent operations. Increasing it increases runtime memory without duplicating the model weights. <code>queue-depth</code> adds waiting requests without adding inference capacity. <code>nthreads</code> controls native threads per operation; zero preserves the whisper.cpp default. Restart the server after changing these settings.</p>
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
                <td><code>0</code></td>
                <td>Idle expiration disabled by default; count/memory eviction still applies</td>
              </tr>
              <tr>
                <td>Memory budget</td>
                <td>Shared</td>
                <td>Bucky, language, and Malina models compete for the shared budget</td>
              </tr>
            </tbody>
          </table>
          <p><code>--media-backends-enabled=false</code> / <code>KRONK_MEDIA_BACKENDS_ENABLED=false</code> skips both Bucky and Malina library downloads and initialization. Installing libraries alone does not re-enable inference: enable media backends and restart. Library installation/verification failures normally stop startup; native initialization failures can leave Bucky unavailable while the server continues.</p>
          <p>Server startup constructs the Bucky library manager from <code>KRONK_BASE_PATH</code> and <code>KRONK_BUCKY_LIB_PATH</code>, downloads or verifies the selected bundle, and passes the manager's resolved directory to the whisper runtime. This keeps a moved base path and an explicit Bucky library path consistent across installation and inference. The llama.cpp <code>--lib-path</code>/<code>KRONK_LIB_PATH</code> setting does not select whisper.cpp libraries; use <code>KRONK_BUCKY_LIB_PATH</code> for Bucky.</p>
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
          <h3 id="186-transcriptions-and-translations-api">18.6 Transcriptions and Translations API</h3>
          <h4 id="1861-request-and-response">18.6.1 Request and Response</h4>
          <p>Send a <code>multipart/form-data</code> request to:</p>
          <pre className="code-block"><code className="language-text">{`POST /v1/audio/transcriptions`}</code></pre>
          <p>To translate supported source speech into English, use the OpenAI-compatible translation route instead:</p>
          <pre className="code-block"><code className="language-text">{`POST /v1/audio/translations`}</code></pre>
          <p>The uploaded file is limited to <strong>25 MiB</strong>. Transcription and translation use the shared <code>--inference-timeout</code> / <code>KRONK_WEB_INFERENCE_TIMEOUT</code>, default <code>60m</code>.</p>
          <p>WAV, MP3, and FLAC decode in-process. Browser recordings and formats such as WebM/Opus, MP4/AAC, OGG, and M4A require <code>ffmpeg</code> on the <strong>server's PATH</strong>, including inside its container or service environment. Installing it only on the browser's machine is not sufficient. Restart after fixing PATH or installing ffmpeg if the running process has already failed to find it.</p>
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
                <td>Kronk extension on the transcription route; <code>true</code> translates supported source speech to English</td>
              </tr>
              <tr>
                <td><code>temperature</code></td>
                <td>No</td>
                <td>Initial decoding temperature</td>
              </tr>
              <tr>
                <td><code>temperature_inc</code></td>
                <td>No</td>
                <td>Temperature increase used for fallback decoding</td>
              </tr>
              <tr>
                <td><code>entropy_threshold</code></td>
                <td>No</td>
                <td>Entropy threshold for deciding whether to retry a decode</td>
              </tr>
              <tr>
                <td><code>logprob_threshold</code></td>
                <td>No</td>
                <td>Average log-probability threshold for deciding whether to retry a decode</td>
              </tr>
              <tr>
                <td><code>no_speech_threshold</code></td>
                <td>No</td>
                <td>Probability threshold used to classify a segment as silence</td>
              </tr>
              <tr>
                <td><code>greedy_best_of</code></td>
                <td>No</td>
                <td>Number of candidates considered by greedy sampling</td>
              </tr>
              <tr>
                <td><code>beam_size</code></td>
                <td>No</td>
                <td>Positive beam size that selects beam-search sampling</td>
              </tr>
              <tr>
                <td><code>beam_search_patience</code></td>
                <td>No</td>
                <td>Patience used by beam-search sampling</td>
              </tr>
              <tr>
                <td><code>length_penalty</code></td>
                <td>No</td>
                <td>Decoder length penalty</td>
              </tr>
              <tr>
                <td><code>response_format</code></td>
                <td>No</td>
                <td><code>json</code> (default), <code>verbose_json</code>, <code>text</code>, <code>srt</code>, or <code>vtt</code></td>
              </tr>
              <tr>
                <td><code>timestamp_granularities[]</code></td>
                <td>No</td>
                <td><code>word</code> enables word-level timestamps in <code>verbose_json</code></td>
              </tr>
            </tbody>
          </table>
          <p>Omitted sampling fields retain the defaults supplied by the loaded whisper.cpp library. Sampling fields must contain valid numeric values.</p>
          <p>Example:</p>
          <pre className="code-block"><code className="language-sh">{`curl -X POST http://localhost:11435/v1/audio/transcriptions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -F file=@samples/jfk.wav \\
  -F model=tiny \\
  -F temperature=0 \\
  -F response_format=json`}</code></pre>
          <p>The default JSON response is:</p>
          <pre className="code-block"><code className="language-json">{`{"text":"And so my fellow Americans..."}`}</code></pre>
          <p>The translation route accepts the same options above and always enables source-to-English translation, so it does not require the Kronk-specific <code>translate</code> field. Its default JSON response has the same <code>text</code> shape. A <code>verbose_json</code> translation identifies its task as <code>translate</code> and its output language as <code>english</code>. Prefer this route when verbose output must identify the translation task; the transcription route's <code>translate=true</code> extension retains its transcription task label.</p>
          <p><code>verbose_json</code> adds the detected language, duration, and timestamped segments. When <code>timestamp_granularities[]=word</code> is requested, it also includes a <code>words</code> array whose entries contain <code>word</code>, <code>start</code>, and <code>end</code> fields. Segment <code>tokens</code>, <code>temperature</code>, <code>avg_logprob</code>, and <code>compression_ratio</code> are compatibility placeholders (empty or zero), not measured decoder results. Do not use them as confidence filters. The <code>text</code>, <code>srt</code>, and <code>vtt</code> formats return their corresponding non-JSON media types.</p>
          <p>English-only models (<code>tiny.en</code>, <code>base.en</code>, <code>small.en</code>, and <code>medium.en</code>) only accept an empty language hint or <code>en</code>. Use a multilingual model for other languages or translation.</p>
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
                <td><code>GET /v1/bucky/libs/integrity</code></td>
                <td>Verify the selected library bundle</td>
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
          <h3 id="187-capability-boundaries-and-developer-resources">18.7 Capability Boundaries and Developer Resources</h3>
          <p>HTTP and BUI support uploaded or recorded-file transcription and translation. Channel-separated diarization and continuous partial/final streaming are SDK-only; they are not audio HTTP streaming routes. The Translator records audio before uploading it rather than exposing the SDK's continuous stream.</p>
          <p>Application developers should use the generated Bucky SDK reference and the <code>examples/bucky</code> and <code>examples/bucky-stream</code> programs. The source-checkout targets <code>make example-bucky</code> and <code>make example-bucky-stream</code> run those examples; <code>make example-bucky-stream-vad</code> additionally downloads and selects Silero VAD. These commands can download libraries/models and run inference or capture audio.</p>
          <h3 id="188-languages">18.8 Languages</h3>
          <p>Whisper supports approximately 99 languages. Use its short language codes, such as <code>en</code>, <code>de</code>, <code>fr</code>, <code>es</code>, or <code>zh</code>. An empty language value asks Whisper to auto-detect the language.</p>
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
                <td>Keep the audio file at or below 25 MiB. Split long recordings or re-encode them at a lower bitrate.</td>
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
                <td>Browser recordings fail to decode</td>
                <td>Make ffmpeg available on the server/container PATH and restart after a previous failed lookup.</td>
              </tr>
              <tr>
                <td>Server startup fails during library installation or verification</td>
                <td>Install the compatible managed bundle locally, inspect the verification error, then restart; do not routinely disable verification.</td>
              </tr>
              <tr>
                <td>Libraries exist but audio inference is unavailable</td>
                <td>Check <code>KRONK_MEDIA_BACKENDS_ENABLED</code>, enable it if needed, and restart.</td>
              </tr>
            </tbody>
          </table>
          <hr />
          <p>Next: <a href="https://www.kronkai.com/manual#chapter-19-malina-image-generation">Chapter 19: Malina (Image Generation)</a></p>
          <h2 id="chapter-19-malina-image-generation">Chapter 19: Malina (Image Generation)</h2>
          <p><strong>Experimental:</strong> Malina's public SDK API is subject to change.</p>
          <h3 id="191-overview">19.1 Overview</h3>
          <p>Malina is Kronk's image-generation subsystem, powered by <a href="https://github.com/leejet/stable-diffusion.cpp">stable-diffusion.cpp</a>. It needs both compatible native libraries and a supported model bundle. A bundle can be a single checkpoint or several pipeline components; downloading a language-model GGUF does not supply an image-generation pipeline.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Surface</th>
                <th>Supported workflows</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Model server and BUI</td>
                <td>Text-to-image and image-to-image; PNG results and server-wide progress</td>
              </tr>
              <tr>
                <td>Local CLI</td>
                <td>Library and curated model-bundle installation, listing, and removal</td>
              </tr>
              <tr>
                <td>SDK</td>
                <td>Image generation/editing plus ControlNet, ADetailer, video generation, upscaling, and Motion-JPEG encoding</td>
              </tr>
            </tbody>
          </table>
          <p>The server shares memory admission and retention controls with the language and Bucky backends. Advanced SDK features are not additional HTTP routes.</p>
          <h3 id="192-install-stable-diffusion-libraries">19.2 Install Stable Diffusion Libraries</h3>
          <p>Install the pinned bundle for the current host:</p>
          <pre className="code-block"><code className="language-shell">{`kronk malina libs --local
kronk malina libs --list-combinations --local
kronk malina libs --list-installs --local`}</code></pre>
          <p>The Malina CLI currently requires <code>--local</code>. Server-side management is available through the BUI and <code>/v1/malina/...</code> APIs, not through default CLI web mode. Run <code>kronk malina libs --help</code> for version selection, installation for another platform triple, and removal.</p>
          <p>Libraries normally live under <code>~/.kronk/malina-libraries/&lt;os&gt;/&lt;arch&gt;/&lt;processor&gt;/</code>. Supported combinations for the pinned release include:</p>
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
                <td><code>arm64</code></td>
                <td><code>cpu</code>, <code>metal</code></td>
              </tr>
              <tr>
                <td>Linux</td>
                <td><code>amd64</code></td>
                <td><code>cpu</code>, <code>vulkan</code>, <code>rocm</code></td>
              </tr>
              <tr>
                <td>Windows</td>
                <td><code>amd64</code></td>
                <td><code>cpu</code>, <code>cuda</code>, <code>vulkan</code>, <code>rocm</code></td>
              </tr>
            </tbody>
          </table>
          <p>Use <code>--list-combinations</code> for the installed release's current list. The macOS <code>cpu</code> and <code>metal</code> choices use the same Metal-capable archive; selecting its CPU directory alone does not force CPU execution. This pin has no Linux CUDA or Linux/Windows arm64 bundle.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Variable</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>KRONK_BASE_PATH</code></td>
                <td>Select the data root without migrating existing files</td>
              </tr>
              <tr>
                <td><code>KRONK_MALINA_LIB_PATH</code></td>
                <td>Select an exact stable-diffusion.cpp library directory</td>
              </tr>
              <tr>
                <td><code>MALINA_LIB</code></td>
                <td>Legacy fallback for the Malina library path</td>
              </tr>
              <tr>
                <td><code>KRONK_ARCH</code>, <code>KRONK_OS</code>, <code>KRONK_PROCESSOR</code></td>
                <td>Runtime-selection preferences; compatibility checks can select a fallback</td>
              </tr>
            </tbody>
          </table>
          <p>An explicit library path is authoritative. A nonempty directory without <code>version.json</code> is a read-only user-managed build, not a managed installation. Default verified server startup requires managed metadata and can reject it. Using independently trusted custom builds requires an advanced verification exception; do not disable verification as routine recovery.</p>
          <p>Use the libraries pinned to this Kronk release. Newer native libraries are not necessarily ABI-compatible; upgrade bindings and libraries together. Offline managed installations must match the requested pin and platform. Installation is staged so a failed download does not replace the working bundle. Restart after changing the selected libraries; the process does not hot-reload them.</p>
          <p>With default verification enabled, installation or verification failures stop server startup. Repair the managed bundle locally, then restart. A later native initialization failure can instead leave Malina unavailable while the server continues; BUI repair is useful only if the server actually remains running.</p>
          <h3 id="193-manage-model-bundles">19.3 Manage Model Bundles</h3>
          <p>Use the curated catalog rather than an arbitrary repository layout:</p>
          <pre className="code-block"><code className="language-shell">{`kronk malina model catalog --local
kronk malina model pull sd-1.5 --local
kronk malina model list --local
kronk malina model remove sd-1.5 --local`}</code></pre>
          <p>Models normally live under <code>~/.kronk/malina-models/&lt;bundle&gt;/</code>. The catalog reports component files, licenses, gating, and download sizes. Budget disk for multi-gigabyte downloads and installation staging, not just inference memory. Complete installations are reused on subsequent runs.</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Bundle</th>
                <th>Intended use</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>sd-1.5</code>, <code>sdxl-base-1.0</code></td>
                <td>Single-checkpoint image generation</td>
              </tr>
              <tr>
                <td><code>z-image-turbo</code>, <code>llada-image-turbo</code>, <code>moss-image-sft</code></td>
                <td>Component-based image generation</td>
              </tr>
              <tr>
                <td><code>flux2-klein-4b</code>, <code>flux2-klein-9b</code></td>
                <td>Component-based, license-gated image generation</td>
              </tr>
              <tr>
                <td><code>controlnet-canny-sd1.5</code>, <code>z-image-turbo-controlnet-union</code></td>
                <td>SDK ControlNet workflows</td>
              </tr>
              <tr>
                <td><code>adetailer-face-yolov8n</code></td>
                <td>SDK face refinement</td>
              </tr>
              <tr>
                <td><code>animatediff-sd1.5</code>, <code>wan2.2-s2v-14b</code></td>
                <td>SDK video workflows</td>
              </tr>
              <tr>
                <td><code>realesrgan-x4-anime</code></td>
                <td>SDK upscaling</td>
              </tr>
            </tbody>
          </table>
          <p>For gated FLUX.2 bundles, accept the provider's license and supply a read token through <code>KRONK_HF_TOKEN</code> or <code>HF_TOKEN</code>. Review each model's license for your use; availability in the catalog is not a grant of commercial rights.</p>
          <p>Catalog selection does not change generation defaults. Apply model-specific recommendations explicitly in HTTP fields or the BUI:</p>
          <ul>
            <li><strong>Z-Image-Turbo:</strong> 8 steps and CFG scale 1.</li>
            <li><strong>LLaDA-Image-Turbo:</strong> 4 steps and CFG scale 1. Its distinct native reference-image instruction-editing path is not exposed by Kronk.</li>
            <li><strong>MOSS-Image-SFT:</strong> dimensions divisible by 16 and CFG scale 3. The curated bundle has not been locally inference-validated.</li>
          </ul>
          <p>The server maps the required component roles for supported generation bundles; operators do not need to configure individual component paths.</p>
          <h3 id="194-server-configuration-and-resources">19.4 Server Configuration and Resources</h3>
          <p>Enable media backends and restart after installation. <code>--media-backends-enabled=false</code> / <code>KRONK_MEDIA_BACKENDS_ENABLED=false</code> skips both Malina and Bucky downloads and native initialization. Installing files alone does not override this setting.</p>
          <p>Configure bundles under <code>malina-models</code> in the versioned model configuration:</p>
          <pre className="code-block"><code className="language-yaml">{`version: 2
malina-models:
  sd-1.5:
    concurrency: 1
    queue-depth: 2
    admission-timeout: 3m
    cpu-threads: 0`}</code></pre>
          <p>Merge this mapping into the existing file rather than replacing its other sections. <code>concurrency</code> defaults to 1. Each additional slot loads another native model context and increases RAM/VRAM use substantially. Queue depth adds waiting requests without adding execution capacity; it defaults to 0. Additional callers can wait for admission up to the configured timeout, default three minutes. Restart after changing configuration.</p>
          <p>The shared server pool defaults to ten entries per backend with idle expiry disabled. Memory pressure and count limits can still evict idle Malina models. The three backend pools share a budget but evict only their own idle entries; an idle language model is not automatically unloaded to admit an image model. See <a href="https://www.kronkai.com/manual#84-model-pool-and-resource-budgets">Chapter 8</a>.</p>
          <h3 id="195-browser-ui">19.5 Browser UI</h3>
          <p>Use the library and model-management pages to prepare a compatible installation. In <strong>Image Generator</strong>, select an installed generation model, enter a prompt, and set dimensions, steps, CFG scale, and seed. Upload a PNG/JPEG to select image-to-image and choose strength. The result shows dimensions and can be downloaded as PNG.</p>
          <p>The activity panel reports server-wide loading and generation progress, not only the current browser's request. See Chapter 13 for browser authentication.</p>
          <h3 id="196-image-generation-and-editing-api">19.6 Image Generation and Editing API</h3>
          <p>All image routes follow inference authorization. <code>authenticated</code> requires a valid token; <code>full-protected</code> additionally requires <code>image-generations</code>. Inference is public in <code>open</code> and <code>management</code> modes.</p>
          <p>Create an image with JSON:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/images/generations \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H 'Content-Type: application/json' \\
  -d '{
    "model": "sd-1.5",
    "prompt": "A lighthouse during a thunderstorm",
    "size": "512x512",
    "steps": 20,
    "cfg_scale": 7,
    "seed": 42,
    "response_format": "b64_json"
  }'`}</code></pre>
          <p>Omit Authorization for an unprotected local server. Unknown JSON fields are rejected. Request controls are:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Field</th>
                <th>Default</th>
                <th>Contract</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>model</code></td>
                <td>Required</td>
                <td>Installed generation bundle ID</td>
              </tr>
              <tr>
                <td><code>prompt</code></td>
                <td>Required</td>
                <td>Nonempty text prompt</td>
              </tr>
              <tr>
                <td><code>negative_prompt</code></td>
                <td>Empty</td>
                <td>Undesired features</td>
              </tr>
              <tr>
                <td><code>size</code></td>
                <td><code>512x512</code> for generation</td>
                <td><code>WIDTHxHEIGHT</code>; multiples of 8, each 64–1024, at most 1,048,576 pixels</td>
              </tr>
              <tr>
                <td><code>steps</code></td>
                <td><code>20</code></td>
                <td>1–1,000</td>
              </tr>
              <tr>
                <td><code>cfg_scale</code></td>
                <td><code>7</code></td>
                <td>Positive finite value; follow model recommendations</td>
              </tr>
              <tr>
                <td><code>seed</code></td>
                <td><code>-1</code></td>
                <td><code>-1</code> requests random selection</td>
              </tr>
              <tr>
                <td><code>n</code></td>
                <td><code>1</code></td>
                <td>Only one image is supported</td>
              </tr>
              <tr>
                <td><code>response_format</code></td>
                <td><code>b64_json</code></td>
                <td>Only base64 PNG output is supported; no URL response</td>
              </tr>
            </tbody>
          </table>
          <p>The response has <code>created</code> and a one-element <code>data</code> array containing <code>b64_json</code>, <code>seed</code>, <code>width</code>, and <code>height</code>. Decode <code>b64_json</code> to save the PNG. The returned seed retains the requested value, including <code>-1</code>; it does not expose a resolved random seed. Do not expect identical pixels across model, backend, or native-library changes.</p>
          <p>Transform an image with multipart form fields:</p>
          <pre className="code-block"><code className="language-shell">{`curl http://localhost:11435/v1/images/edits \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -F model=sd-1.5 \\
  -F prompt='A watercolor illustration at sunset' \\
  -F image=@source.png \\
  -F strength=0.6 \\
  -F steps=20`}</code></pre>
          <p>Editing accepts the same generation controls plus required PNG/JPEG <code>image</code> and optional <code>strength</code> (default <code>0.75</code>, greater than zero through 1). Uploads are limited to 25 MiB and decoded images to 32 Mi pixels. Without <code>size</code>, the server preserves aspect ratio, scales down to at most 1024 pixels per side, and rounds dimensions down to multiples of eight. Explicit dimensions resize the source to that size. The response has the same base64 PNG shape.</p>
          <p>Both operations use the server's inference deadline. Cancellation requests native work to stop but can take time to finish; it is not a guarantee of immediate resource release.</p>
          <h3 id="197-progress-and-administration">19.7 Progress and Administration</h3>
          <p><code>GET /v1/images/events</code> is an SSE stream reporting <code>scope: "global"</code>, <code>step</code>, <code>steps</code>, <code>percent</code>, and <code>seconds_per_step</code>. Progress is process-global and not request-correlated; it must not be treated as the completion signal for an individual request. Use the POST response for that result.</p>
          <p>Management APIs require administrator access in every authorization mode except <code>open</code>:</p>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Method and path</th>
                <th>Purpose</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><code>GET /v1/malina/libs</code></td>
                <td>Active native library installation</td>
              </tr>
              <tr>
                <td><code>GET /v1/malina/libs/integrity</code></td>
                <td>Verify the selected library bundle</td>
              </tr>
              <tr>
                <td><code>GET /v1/malina/libs/combinations</code></td>
                <td>Supported platform combinations</td>
              </tr>
              <tr>
                <td><code>GET /v1/malina/libs/installs</code></td>
                <td>Installed bundles</td>
              </tr>
              <tr>
                <td><code>POST /v1/malina/libs/pull</code></td>
                <td>Install libraries</td>
              </tr>
              <tr>
                <td><code>DELETE /v1/malina/libs/installs</code></td>
                <td>Remove an installation</td>
              </tr>
              <tr>
                <td><code>GET /v1/malina/models</code></td>
                <td>Installed model bundles</td>
              </tr>
              <tr>
                <td><code>GET /v1/malina/models/catalog</code></td>
                <td>Curated model catalog</td>
              </tr>
              <tr>
                <td><code>POST /v1/malina/models/pull</code></td>
                <td>Download a bundle</td>
              </tr>
              <tr>
                <td><code>DELETE /v1/malina/models/&#123;model&#125;</code></td>
                <td>Remove a model bundle</td>
              </tr>
            </tbody>
          </table>
          <p>See Chapter 9 for API conventions and Chapter 15 for server logs and metrics.</p>
          <h3 id="198-limitations-and-developer-resources">19.8 Limitations and Developer Resources</h3>
          <p>The server exposes basic text-to-image and image-to-image only. ControlNet, ADetailer, video, upscaling, and AVI encoding remain SDK workflows. A feature in upstream stable-diffusion.cpp does not establish a supported Kronk workflow or a curated, licensed model bundle. The catalog is intentionally bounded.</p>
          <p>Use the generated Malina SDK reference and <code>examples/malina*</code> for application development. Source-checkout targets include <code>make example-malina</code>, <code>make example-malina-img2img</code>, <code>make example-malina-controlnet</code>, <code>make example-malina-adetailer</code>, <code>make example-malina-animatediff</code>, <code>make example-malina-s2v</code>, and <code>make example-malina-upscale</code>. These can download multi-gigabyte model bundles and execute native inference. They are not required steps for operating the HTTP server.</p>
          <h3 id="199-troubleshooting">19.9 Troubleshooting</h3>
          <table className="flags-table">
            <thead>
              <tr>
                <th>Symptom</th>
                <th>Action</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Startup fails during library installation or verification</td>
                <td>Install the pinned managed bundle locally, inspect the error, then restart. Do not routinely disable verification.</td>
              </tr>
              <tr>
                <td>No compatible host bundle</td>
                <td>Check <code>kronk malina libs --list-combinations --local</code>; use a published platform/backend.</td>
              </tr>
              <tr>
                <td>Files exist but image inference is unavailable</td>
                <td>Enable media backends and restart; inspect native initialization errors.</td>
              </tr>
              <tr>
                <td>Custom libraries cannot be upgraded or removed</td>
                <td>Directories without managed metadata are read-only; manage them independently.</td>
              </tr>
              <tr>
                <td>Gated download returns 401/403</td>
                <td>Accept the provider's license and supply a read token privately.</td>
              </tr>
              <tr>
                <td>Invalid generation request</td>
                <td>Check installed bundle ID, prompt, dimensions, steps, CFG scale, and edit strength.</td>
              </tr>
              <tr>
                <td>Black/white images or NaN diagnostics</td>
                <td>Verify the pinned library/model combination and model-specific guidance; report backend, format, and settings rather than changing unrelated defaults.</td>
              </tr>
              <tr>
                <td>Admission timeout or server busy</td>
                <td>Inspect running work and shared memory; reduce concurrency/sizing or unload unused models. Increase waiting only if callers can tolerate it.</td>
              </tr>
              <tr>
                <td>Library path changed but behavior did not</td>
                <td>Restart; native libraries are not hot-reloaded.</td>
              </tr>
            </tbody>
          </table>
          <hr />
          <p>Next: <a href="https://www.kronkai.com/manual#chapter-20-developer-guide">Chapter 20: Developer Guide</a></p>
          <h2 id="chapter-20-developer-guide">Chapter 20: Developer Guide</h2>
          <h3 id="201-how-to-use-this-guide">20.1 How to Use This Guide</h3>
          <p>This chapter is a durable orientation guide for contributors and coding agents. It describes ownership boundaries, lifecycle contracts, and the smallest useful checks for common changes. It intentionally does not narrate every function, reproduce private structures, or freeze today's source layout at the individual-file level.</p>
          <h4 id="2011-source-of-truth-hierarchy">20.1.1 Source-of-truth hierarchy</h4>
          <p>When sources disagree, use this order:</p>
          <ol>
            <li><strong>Applicable &lt;code&gt;AGENTS.md&lt;/code&gt; files.</strong> Read the root instructions and every scoped instruction file governing the path being changed. A deeper file overrides or supplements broader guidance for its subtree.</li>
            <li><strong>Current source and tests.</strong> Interfaces, call sites, focused tests, and generated code establish actual behavior. For generated documentation, the authored input and generator are authoritative over checked-in output from an older generation. Confirm assumptions in code before editing.</li>
            <li><strong>Makefiles and GitHub workflows.</strong> These establish supported build, generation, CI, packaging, and deployment procedures.</li>
            <li><strong>This chapter.</strong> It provides background and navigation, not a replacement for scoped instructions or source inspection.</li>
          </ol>
          <p>Treat names and defaults in this chapter as wayfinding aids. Before relying on an exact flag, timeout, model capability, or API signature, inspect its current owner. Do not expand a task merely to make the repository resemble this overview.</p>
          <h4 id="2012-a-productive-agent-loop">20.1.2 A productive agent loop</h4>
          <p>For most work, the safest loop is:</p>
          <ol>
            <li>Identify the public behavior being changed and its owning package.</li>
            <li>Read that package's scoped instructions, implementation, focused tests, and direct caller. Avoid broad repository scans when a precise search will answer the question.</li>
            <li>Write down the invariants that must remain true: resource ownership, cancellation, mutation semantics, compatibility, generated artifacts, and error translation.</li>
            <li>Make the smallest coherent change in the owner. Do not duplicate policy in a transport or facade when the lower layer already owns it.</li>
            <li>Format and run package-scoped static checks, then focused tests. Regenerate only artifacts derived from changed sources.</li>
            <li>Review the diff for accidental generated-file edits, private data in logs, unrelated formatting, and stale documentation.</li>
          </ol>
          <h3 id="202-task-to-owner-and-verification-map">20.2 Task-to-Owner and Verification Map</h3>
          <p>The following table is a starting point, not permission to skip local instructions. “Focused verification” means the narrowest package or command that exercises the change. Go commands require the environment described in <a href="#209-verification-for-llm-agents">§20.9</a>.</p>
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
                <td>Malina handle or image generation</td>
                <td><code>sdk/malina/</code> and <code>sdk/malina/model/</code></td>
                <td><code>sdk/tools/malina/</code>, examples, Chapter 19 for user behavior</td>
                <td>package unit tests and compile affected examples; model-backed generation is deliberate human work</td>
              </tr>
              <tr>
                <td>Malina libraries/model-bundle tooling</td>
                <td><code>sdk/tools/malina/</code></td>
                <td><code>sdk/malina/</code>, curated bundle catalog, examples</td>
                <td>tooling package tests and catalog drift tests; do not download multi-gigabyte models merely to verify tooling</td>
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
                <td>Helm chart</td>
                <td><code>zarf/helm/kronk/</code></td>
                <td>container entrypoint, <a href="https://www.kronkai.com/manual#810-kubernetes-with-helm">Chapter 8 §8.10</a>, CPU/CUDA examples</td>
                <td><code>zarf/helm/kronk/test.sh</code>; smoke test on an explicitly selected disposable cluster context; no Go tests for chart-only changes</td>
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
          <h3 id="203-repository-ownership-map">20.3 Repository Ownership Map</h3>
          <h4 id="2031-commands-and-server">20.3.1 Commands and server</h4>
          <ul>
            <li><strong>&lt;code&gt;cmd/kronk/&lt;/code&gt;</strong> owns the installed <code>kronk</code> executable, command hierarchy, flags, local-versus-server dispatch, terminal presentation, and process control. Commands should orchestrate reusable managers rather than become a second implementation of catalogs, downloads, inference, or authentication. <code>make install-kronk</code> is simply <code>go install ./cmd/kronk</code>; there are no project build tags required for installation.</li>
            <li><strong>&lt;code&gt;cmd/server/api/&lt;/code&gt;</strong> composes executable services, startup configuration, embedded assets, and tooling binaries. <code>cmd/server/api/services/kronk/main.go</code> is the Kronk service composition root and the BUI embedding owner.</li>
            <li><strong>&lt;code&gt;cmd/server/app/domain/&lt;/code&gt;</strong> owns HTTP domain behavior. Packages such as <code>chatapp</code>, <code>respapp</code>, <code>embedapp</code>, <code>rerankapp</code>, <code>audioapp</code>, <code>toolapp</code>, and <code>playgroundapp</code> register routes and translate HTTP requests to application/SDK calls. Keep protocol validation and response formatting near the domain; keep inference policy in SDKs.</li>
            <li><strong>&lt;code&gt;cmd/server/app/&lt;/code&gt;</strong> also contains application wiring and adapters shared by domains. It is below service startup and above reusable SDK packages.</li>
            <li><strong>&lt;code&gt;cmd/server/foundation/&lt;/code&gt;</strong> owns cross-cutting server infrastructure. <code>web/</code> owns request lifecycle, context, middleware, error/response writing, and transport-level tracing; <code>logger/</code> owns server logging primitives. Domain packages should use these facilities rather than invent parallel conventions.</li>
          </ul>
          <h4 id="2032-language-model-sdk-and-engine">20.3.2 Language-model SDK and engine</h4>
          <ul>
            <li><strong>&lt;code&gt;sdk/kronk/&lt;/code&gt;</strong> is the public language-model handle and API surface. A <code>kronk.Kronk</code> owns <strong>one primary loaded model</strong> and a semaphore governing admission to that handle. Its low-level model may also own draft or MTP resources. The handle exposes chat, streaming, Responses, embeddings, reranking, tokenization, model information, and unload behavior. It is not a model pool.</li>
            <li><strong>&lt;code&gt;sdk/kronk/model/&lt;/code&gt;</strong> owns low-level llama/yzma inference: model/context creation, prompt planning, batch slots, sequence IDs, prefill/decode, media handling, IMC, samplers, parser interfaces, and draft/MTP behavior. Changes here must preserve cross-slot isolation and native-resource cleanup on every exit path.</li>
            <li><strong>&lt;code&gt;sdk/kronk/parsers/&lt;/code&gt;</strong> contains model-family parser plug-ins. Family packages own streaming state machines and extraction/normalization of reasoning and tool calls. The model package owns the registry contract and selection boundary; parser packages should not reach into batch-engine state.</li>
            <li><strong>&lt;code&gt;sdk/kronk/observ/&lt;/code&gt;</strong>, <code>sdk/kronk/kvstorage/</code>, <code>sdk/kronk/vram/</code>, <code>sdk/kronk/gguf/</code>, and <code>sdk/kronk/hf/</code> own their named concerns. Prefer these boundaries to embedding format, storage, or resource calculations in request handlers.</li>
          </ul>
          <h4 id="2033-pools-and-shared-resources">20.3.3 Pools and shared resources</h4>
          <ul>
            <li><strong>&lt;code&gt;sdk/pool/&lt;/code&gt;</strong> is the server-facing application facade over the language, audio, and image typed pools and their shared resource manager. It coordinates backends; it does not replace the public SDK handles.</li>
            <li><strong>&lt;code&gt;sdk/kronk/pool/&lt;/code&gt;</strong> adapts Kronk model discovery, planning, loading, display, and unloading to the generic pool engine.</li>
            <li><strong>&lt;code&gt;sdk/bucky/pool/&lt;/code&gt;</strong> performs the equivalent work for Whisper/Bucky models.</li>
            <li><strong>&lt;code&gt;sdk/malina/pool/&lt;/code&gt;</strong> performs the equivalent work for image-generation bundles.</li>
            <li><strong>&lt;code&gt;sdk/pool/engine/&lt;/code&gt;</strong> owns typed cache mechanics: acquisition coalescing, admission, idle-entry selection, expiry, invalidation, and eviction callbacks. The shared resource manager owns RAM/VRAM reservations and budget decisions across backends. Eviction is therefore budget-aware and constrained by active use and releasable reservations; it is not a simple model-ID LRU.</li>
          </ul>
          <p>The service supplies shared-pool settings to all three typed pools. A typed pool used independently may therefore have different fallback values. Check the composition layer before documenting effective server behavior, and check the typed constructor before documenting standalone behavior.</p>
          <h4 id="2034-bucky-malina-tools-ui-examples-and-deployment">20.3.4 Bucky, Malina, tools, UI, examples, and deployment</h4>
          <ul>
            <li><strong>&lt;code&gt;sdk/bucky/&lt;/code&gt;</strong> is the public concurrent audio handle. <strong>&lt;code&gt;sdk/bucky/model/&lt;/code&gt;</strong> owns whisper context/state operations, decoding, transcription, and stream mechanics.</li>
            <li><strong>&lt;code&gt;sdk/malina/&lt;/code&gt;</strong> is the experimental public image-generation handle. <strong>&lt;code&gt;sdk/malina/model/&lt;/code&gt;</strong> owns stable-diffusion model configuration, native context lifecycle, generation, and Motion-JPEG encoding. The server exposes image generation and editing through Malina's typed pool; other SDK capabilities remain SDK-only.</li>
            <li><strong>&lt;code&gt;sdk/tools/&lt;/code&gt;</strong> owns reusable catalog, downloader, backend, device, diagnostics, library, and model-management operations used by CLI and server tools. Bucky-specific installers and catalogs live below <code>sdk/tools/bucky/</code>; Malina's native-library manager and curated diffusion bundles live below <code>sdk/tools/malina/</code>.</li>
            <li><strong>&lt;code&gt;cmd/server/api/frontends/bui/&lt;/code&gt;</strong> is the React/TypeScript browser application. Component-level conventions are deliberately delegated to the applicable <code>AGENTS.md</code>; this guide does not duplicate them.</li>
            <li><strong>&lt;code&gt;examples/&lt;/code&gt;</strong> is a separate Go module of runnable public-SDK examples and a source for generated documentation. Keep examples public and instructional: do not import internal server implementation to make them convenient.</li>
            <li><strong>&lt;code&gt;.manual/&lt;/code&gt;</strong> contains authored manual chapters. The docs tool converts manuals, public SDK documentation, and examples into BUI components.</li>
            <li><strong>&lt;code&gt;zarf/&lt;/code&gt;</strong>, <code>.github/workflows/</code>, <code>.goreleaser.yaml</code>, and <code>.release/</code> own deployment, reproducibility, and release automation. Runtime image behavior must agree with the Docker workflow and entrypoint, not with an old prose inventory.</li>
          </ul>
          <h3 id="204-developer-setup-and-daily-commands">20.4 Developer Setup and Daily Commands</h3>
          <p>From the repository root, install the CLI with:</p>
          <pre className="code-block"><code className="language-shell">{`go install ./cmd/kronk`}</code></pre>
          <p>The Make target is a convenient alias:</p>
          <pre className="code-block"><code className="language-shell">{`make install-kronk`}</code></pre>
          <p>Configure repository hooks and optional development tooling with:</p>
          <pre className="code-block"><code className="language-shell">{`make setup
make install-gotooling
make install-tooling`}</code></pre>
          <p><code>make setup</code> configures the repository's hook workflow. Tool installation is separate; inspect the Make targets before running them on a platform where package-manager changes are undesirable. The pre-commit hook regenerates the manual and BUI and, when <code>gomod2nix</code> is installed, refreshes <code>zarf/nix/gomod2nix.toml</code>. It stages only the generated BUI source/bundle paths and the Nix dependency file; it does not run <code>git add -A</code> or stage unrelated working-tree changes. Review the resulting staged diff before committing, especially when committing only part of the worktree.</p>
          <p>Common service commands include:</p>
          <pre className="code-block"><code className="language-shell">{`make kronk-server
make kronk-server-detach
make kronk-server-logs
make kronk-server-stop`}</code></pre>
          <p>Native llama and Whisper libraries and test models are large external prerequisites. Use the CLI and Make targets appropriate to the focused test rather than downloading every supported artifact. The Bucky CLI uses <code>--local</code> for direct filesystem work; web/server operation is the default and there is no <code>--web</code> flag.</p>
          <p>OpenVINO validation is deliberately opt-in and never runs in the normal local or GitHub suites. On Linux/Windows amd64, run <code>make test-openvino</code> for the CPU target or set <code>OPENVINO_DEVICE=GPU</code>, <code>GPU.&lt;index&gt;</code>, or <code>NPU</code>. The target uses the <code>openvino_integration</code> build tag, installs the pinned llama.cpp bundle and diagnostic model, and rejects an unavailable device that silently falls back to CPU. See <a href="https://www.kronkai.com/manual#24-libraries">Chapter 2 §2.4</a> for runtime selection and host requirements.</p>
          <h4 id="2041-native-library-compatibility-and-sdk-initialization">20.4.1 Native-library compatibility and SDK initialization</h4>
          <p>The versions in <code>go.mod</code>, <code>sdk/tools/libs</code> defaults, and the README compatibility matrix describe one tested Kronk/Yzma/llama.cpp set. A dependency update is incomplete unless the binding, pinned native build, root and examples modules, generated Nix module data, and focused model tests remain aligned. Do not update Yzma or select a newer llama.cpp build independently merely because it is available upstream.</p>
          <p>Runnable examples should resolve, verify, and initialize the same native runtime. Call <code>kronk.Init</code> after runtime detection and installation but before constructing a Kronk model. Passing the manager's resolved <code>LibsPath</code> is required: calling bare <code>kronk.Init()</code> can repeat default selection and load a different bundle from the one the example just verified. Initialization is process-wide and should occur once.</p>
          <p>Bucky examples follow the same sequence with <code>sdk/tools/bucky/libs</code>, <code>bucky.Init</code>, and <code>bucky.WithLibPath(lib.LibsPath())</code>. Keep library installation separate from model download in both example families so failures identify the correct dependency.</p>
          <p>The exact development toolchain is pinned by <code>.go-version</code>, while <code>go.mod</code> declares the minimum language version. Patch versions may differ, but major and minor must match. The workflow version script enforces this relationship; read both files rather than copying their current values into new documentation.</p>
          <h4 id="2042-server-adversarial-probes">20.4.2 Server adversarial probes</h4>
          <p><code>.tools/adversarial/adversarial.sh</code> is an adversarial probe harness that exercises a running Kronk server's HTTP API: constrained decoding, the MTP/speculative decode path, the Anthropic and Responses translations, logprobs, and parameter validation. It treats "returned 200 and quietly ignored what was asked" as a defect, and a probe that cannot assert an invariant reports <code>NO SIGNAL</code> rather than <code>PASS</code>.</p>
          <pre className="code-block"><code className="language-shell">{`make test-adversarial`}</code></pre>
          <p>The target runs the default <code>deep</code> tier, which needs a real generation-capable model and takes tens of minutes. Pass arguments through <code>ARGS</code>:</p>
          <pre className="code-block"><code className="language-shell">{`make test-adversarial ARGS="--tier=smoke"        # contract probes only, minutes
make test-adversarial ARGS="--tier=all"          # deep plus soak and TTL eviction
make test-adversarial ARGS="stream structured"   # only these probe groups
make test-adversarial ARGS="-l"                  # list groups and their tiers`}</code></pre>
          <p>The script probes an existing server by default. Set <code>SERVER=1</code> to have the script start and stop its own server. <code>HOST</code>, <code>MODEL</code>, <code>THINK</code>, <code>TIMEOUT</code>, <code>SERVER_CMD</code>, and <code>KEEP_ALL</code> are the other tuning knobs; run <code>.tools/adversarial/adversarial.sh -h</code> for the full list and current defaults.</p>
          <p>Results land under <code>.tools/adversarial/output</code> (ignored by Git, overridable with <code>OUT</code>): <code>findings.txt</code> holds a result line per probe, a verdict block, and the full request/response of every flagged call, and <code>kronk-server.log</code> holds the server's own log when the script manages the server. Exit status is <code>0</code> when nothing is flagged, <code>1</code> when there are findings, and <code>2</code> when the run could not proceed.</p>
          <p>This is a deliberate human/integration run, not part of <code>make test</code> and not an agent default — see <a href="#2091-required-go-post-edit-sequence">20.9.1</a>.</p>
          <h5>Triaging the findings</h5>
          <p>A finding is a flagged probe, not a proven defect: the probe itself can be wrong, and the cause may sit in yzma or llama.cpp rather than in Kronk. <code>make test-adversarial</code> prints a triage prompt after every run — including the exit-1 run that produced findings — which hands the report to a coding agent for verification. The authoritative prompt is <code>.tools/adversarial/adversarial-triage.md</code>. Follow it to correlate flagged requests with server logs, check the probe's assumptions, and trace ownership before calling a finding a defect. Report confirmed mechanisms separately from unproven findings.</p>
          <p>The yzma and llama.cpp trees the prompt cites are working checkouts under <code>.extras/</code>, which is untracked. Clone them there before triaging, or drop those two sources from the prompt and accept that a defect below the Kronk boundary can only be localized to the binding call site.</p>
          <h4 id="2043-opencode-coding-benchmark">20.4.3 OpenCode coding benchmark</h4>
          <p><code>.tools/codegen-benchmark</code> is a deliberate coding-agent benchmark against a separately managed Kronk server. It runs the tic-tac-toe prompt from <code>examples/talks/tic-tac-toe.md</code> through <code>opencode run</code>, preserving an isolated OpenCode home and nested Git repository for each attempt. The repository boundary keeps OpenCode inside the attempt directory, where it can compile and repair its program before the harness applies hidden structural tests and scripted gameplay checks.</p>
          <p>The harness warms each model with a minimal <code>hello model</code> chat completion before its attempts, then unloads that model before continuing to the next configured model. Warm-up time and tokens are excluded from the OpenCode benchmark measurements.</p>
          <p>Eligible models come exclusively from <code>zarf/kms/model_config.yaml</code>: every selected ID must be configured there, end in <code>/AGENT</code>, and be returned by the running server's <code>GET /v1/models</code> endpoint. The harness defaults to one attempt per eligible model.</p>
          <pre className="code-block"><code className="language-shell">{`make benchmark-codegen-list
make benchmark-codegen
make benchmark-codegen \\
    CODEGEN_MODEL=ornith-ai/Ornith-1.5-35B-Q8_0/AGENT \\
    CODEGEN_ATTEMPTS=3`}</code></pre>
          <p>Results land directly beneath <code>.tools/codegen-benchmark/output</code>. Each run clears the previous contents, stores every selected model beneath that directory, and writes one aggregate <code>report.md</code> for model comparison. Each attempt retains the generated program, OpenCode state and event stream, and grader result. See <code>.tools/codegen-benchmark/README.md</code> for all runtime knobs. This is not part of <code>make test</code>; starting the model server and running inference remain explicit human actions.</p>
          <h3 id="205-request-and-model-lifecycle">20.5 Request and Model Lifecycle</h3>
          <h4 id="2051-server-request-flow">20.5.1 Server request flow</h4>
          <p>The stable request path is:</p>
          <pre className="code-block"><code className="language-text">{`HTTP request
  -> foundation/web middleware and request context
  -> domain route validation and protocol translation
  -> sdk/pool facade
  -> typed pool acquisition (Kronk, Bucky, or Malina)
  -> one-model public handle admission semaphore
  -> model engine/state execution
  -> SDK result or stream
  -> protocol response`}</code></pre>
          <p>Every arrow is an ownership boundary. Middleware owns transport concerns. Domains own HTTP compatibility. Pools own model residency and resource tickets. Handles own per-model admission and shutdown coordination. Engines own native contexts, sequences, and inference state. Preserve error identities long enough for the domain layer to map capacity, cancellation, validation, and internal failures correctly.</p>
          <h4 id="2052-typed-pool-acquisition-loading-and-eviction">20.5.2 Typed pool acquisition, loading, and eviction</h4>
          <p>An acquisition first checks/coalesces a typed cache entry. A cold load is planned by the backend loader, then reserved against the shared memory manager before expensive native loading. The loaded handle becomes visible only after initialization succeeds. Failed planning or loading must release its reservation and must not publish a partial cache entry. Concurrent acquisition of the same key should share load work rather than multiply memory commitments.</p>
          <p>Item count and available RAM/VRAM are independent admission constraints. Normal admission-driven eviction selects only idle handles; the pool observes each handle's active-operation counter rather than owning a separate request lease. Invalidation also rechecks active use: an active entry is restored rather than unloaded.</p>
          <p>Asynchronous invalidation does not imply that its eviction callback has finished. Synchronous invalidation reports callback errors. When unload fails, the engine restores the entry and retains its reservation rather than pretending that memory was released. Preserve the engine-level distinction between a model that cannot fit the configured budget and temporary pressure where no idle candidate can be evicted; typed/public APIs may translate those errors differently.</p>
          <h4 id="2053-semaphore-lifetime-and-cancellation">20.5.3 Semaphore lifetime and cancellation</h4>
          <p>The <code>Kronk</code> handle's semaphore is admission control around one model. A permit belongs to the operation, not merely to function setup. For a non-streaming call, hold it until the operation returns. For streaming, hold it until the stream is terminal or closed, including cancellation/error cleanup. Releasing at stream construction over-admits; forgetting to release on an early error deadlocks future work. The handle's active-use counter similarly spans the operation so pool eviction cannot unload native resources while work is active.</p>
          <p>Context cancellation must propagate inward to queue waits and inference. The layer that creates a goroutine, stream, native object, or lease owns its shutdown. Do not close caller-owned channels or unload a caller-owned handle. Unload prevents new work, waits for owned active work according to its context, then tears down engine/native resources. Pool shutdown owns handles created by that pool.</p>
          <h4 id="2054-batch-slots-and-sequence-isolation">20.5.4 Batch slots and sequence isolation</h4>
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
          <h4 id="2055-generation-versus-embedding-and-reranking-engines">20.5.5 Generation versus embedding and reranking engines</h4>
          <p>Text generation benefits from shared batched execution and sequence-partitioned KV. Embeddings and reranking never use generation slots. All embedding and reranking models currently use the separate sequence-batch engine, which schedules complete embedding inputs or query-document pairs across sequence IDs on one context. A context-pool fallback remains available for models proven incompatible with that engine, but no architecture is currently excluded. Reranking must not allow one document's state to contaminate the next. Embedding pooling and normalization are model/output concerns, not chat-slot concerns. Do not merge the sequence-batch and generation engines merely to share code; their lifecycle contracts differ.</p>
          <p>Runtime selection belongs to <code>supportsBatchSeq</code>; do not add fallback heuristics in HTTP handlers. Changes to compatibility need native/model-backed evidence, concurrent isolation tests, and a benchmark. Some llama.cpp failures assert instead of returning an error, so retrying a failed native decode is not a safe compatibility probe.</p>
          <h3 id="206-core-inference-invariants">20.6 Core Inference Invariants</h3>
          <h4 id="2061-imc-sessions-slots-and-external-storage">20.6.1 IMC sessions, slots, and external storage</h4>
          <p>Incremental Message Cache (IMC) sessions are cache identities, not execution slots. A stable cache/session identifier allows a conversation prefix to survive movement between batch slots. A slot is short-lived compute capacity; binding a session to a slot would reduce concurrency and make slot reuse unsafe.</p>
          <p>The <code>SessionStore</code> contract externalizes each session's native KV snapshot. RAM and custom implementations differ in storage and I/O, but the model layer owns when a snapshot is read, prepared, committed, reset, and closed. Session metadata—cached tokens, render-sensitive identity/version, and snapshot—must describe the same prefix. Do not update one independently and call the session valid.</p>
          <p>Store slices are borrowed rather than transferred. Treat <code>Bytes</code> results as read-only, and do not retain a <code>Bytes</code> or <code>Prepare</code> result across the next prepare, commit, reset, or close operation. <code>Prepare</code> invalidates the committed length until <code>Commit</code> succeeds; a partial or failed snapshot must never become readable. <code>Len</code> and <code>Cap</code> are metadata operations and must not perform storage I/O. Kronk serializes one store through the session reservation, but stores returned by the same factory may run concurrently. A factory may share a connection pool, quota, or eviction manager only when that shared resource is concurrency-safe.</p>
          <p>Pass operation contexts into cancellable backend calls and preserve storage failures with wrapped errors. Durable implementations must publish complete replacements atomically and validate length, format version, and integrity before returning bytes. They also need expiration or out-of-band garbage collection because process failure can bypass <code>Close</code>. The per-session byte-store contract does not itself provide stable conversation identity or restart recovery.</p>
          <p>The session's <code>reserved</code> state serializes mutation and hides the session from competing selection until metadata and snapshot bytes agree. The reservation begins in prompt planning and survives the queue wait and restore; exact read-only hits keep it through generation, while a successfully published text build/extension can release it after the stable snapshot is committed. Restore only a committed snapshot whose complete token or media plan and expected version still match. Ordinary text build/extension prepares and commits through the session's existing store; if snapshot publication fails, invalidate that current state so later work rebuilds it rather than claiming the old or partial state is valid.</p>
          <p>Text planning compares complete token sequences and searches every working session for exact or append reuse before considering System. Each model owns a separate RAM-only System cache pool with the same capacity as its working-session pool. Entries contain only independently rendered, nonempty, cache-worthy system prefixes whose complete token sequences prefix the stable request. They include target KV and compatible own-KV MTP state and never advance.</p>
          <p>On a System hit, reserve the immutable source during restoration and copy it into a separately selected empty or least-recently-used working session. Multiple working sessions may preload the same System entry. On a miss, publish into an empty System entry or replace the least-recently-used entry that is not being restored. Retention failure must not fail inference. Never trim a working session at an internal token match.</p>
          <h5>Template stability and the System cache pool</h5>
          <p>Prefix-stable templates normally append from a working session. Reasoning-aware or look-ahead templates may retroactively change an earlier assistant or tool turn; a matching System entry then provides a conservative restart point.</p>
          <p>Native sequence snapshots can consume hundreds of megabytes, RAM stores retain peak backing capacity, and own-KV MTP configurations may require matching draft state. Do not create System entries for empty or below-threshold system-only renderings, and do not add user-turn or progressive checkpoint roles.</p>
          <p>Do not attempt prefix sharing by slicing or pointing into the current serialized state buffers. The native interface exposes complete per-sequence size/get/set operations, not partial state serialization, base-plus-delta restoration, copy-on-write KV pages, or immutable-prefix sharing. Hybrid recurrent state makes arbitrary byte slicing especially unsafe. Meaningful shared-prefix storage requires explicit native backend support rather than a different Go buffer layout.</p>
          <p>When investigating another template, correlate <code>match_reason</code>, <code>reusable_tokens</code>, <code>system_boundary_tokens</code>, and <code>full_input_snapshot_messages</code> from <code>plan-ready</code> records with System creation/restore and <code>slot-finished</code> events. Distinguish clean Current appends, System recovery, full rebuilds, and actual restored-token reuse.</p>
          <p>Media-anchor advancement has a stronger replacement contract: it writes a separately staged store and swaps the store plus matching plan/count metadata only after success, so failure leaves the previous media snapshot published. Do not generalize that staged replacement guarantee to every IMC path. A <code>SessionStore</code> implementation must honor the interface's read/prepare/commit/reset lifetime rules and clean up temporary resources; callers must not assume bytes remain stable across the next mutation or modify bytes returned for reading.</p>
          <p>Restored KV and sampler history are separate correctness concerns. Prime sampler penalties and DRY with the complete logical prompt after restore. An own-KV MTP session may publish draft KV only with the matching target snapshot and final hidden row, including media prefixes. Media cache decoding hands off every target batch before another decode or projector encode can replace its borrowed buffers. Own-KV replay preserves projected embeddings and explicit positions and shifts only hidden states; logical continuation comes from mtmd rather than embedding-row count. A media advance retains draft state only after a complete paired snapshot for that prefix. Shared-target-KV MTP derives its resume point from the restored target and must not allocate or restore an independent draft store. A draft-only restore failure permits target-only fallback. Target-side rejection has two distinct outcomes: a stale expected-version mismatch is a retryable busy error and leaves the published session intact, while empty target bytes or a partial native target restore are request-fatal and invalidate the affected current state.</p>
          <p>Snapshot publication failures also differ by path. Ordinary text build/append and initial media build write through the current store; incomplete target serialization invalidates that current cache state, but target generation for the current request can continue. Media-anchor advancement instead uses a replacement store and requires a complete staged snapshot before swapping metadata; decode or snapshot failure fails the request while preserving the old published anchor.</p>
          <h4 id="2062-prompt-plans-text-and-media">20.6.2 Prompt plans: text and media</h4>
          <p>Prompt planning converts normalized messages and parameters into the exact work the engine will execute. The plan, cache identity, token accounting, and decode positions must agree. Text-only plans can compare rendered/tokenized prefixes directly and may take optimized exact-hit paths. Media plans carry more than text tokens: ordered media parts, placeholder/embedding expansion, positions, and render-affecting metadata are part of identity and execution.</p>
          <p>Do not treat a media prompt as text with an attachment ignored by caching. A text prefix match is insufficient if image/audio/video content, ordering, sizing, or model projection changes. Media prefill must align embeddings and positions with the same sequence that receives surrounding text. A media-anchor advance stages replacement state separately, so decode or snapshot failure leaves that anchor's prior published snapshot authoritative. Do not claim the same preservation for an LRU rebuild: prompt planning intentionally resets the selected session before rebuilding it.</p>
          <h4 id="2063-parser-registry-ownership">20.6.3 Parser registry ownership</h4>
          <p>Parser implementations live under <code>sdk/kronk/parsers/</code>, grouped by model family. The registry interface and registration entry point live in <code>sdk/kronk/model/</code>. A parser plug-in supplies factories/state machines for its advertised family and must tolerate stream chunk boundaries: tags, JSON, reasoning delimiters, and tool arguments may span chunks. It must keep request state per parser instance and produce equivalent logical results for streaming and non-streaming input.</p>
          <p>To add or change a parser, edit the family package, update registration/selection only where necessary, and test fragmented as well as complete input. Keep generic JSON repair separate from family recognition. Unknown families need an intentional fallback or error; registration order must not create accidental model-family selection.</p>
          <h4 id="2064-responses-normalization">20.6.4 Responses normalization</h4>
          <p>The Responses API adapts to the chat/inference pipeline in <code>sdk/kronk/response.go</code>. Normalization has a compatibility-sensitive mutation contract:</p>
          <ul>
            <li>Preserve existing <code>messages</code>; they win when already supplied.</li>
            <li>Convert Responses <code>input</code> into messages <strong>only when messages are absent</strong>.</li>
            <li>Normalize Responses item/content/tool forms needed by chat processing.</li>
            <li>Mutate the supplied <code>model.D</code> document map. Callers that require isolation must clone before invoking the Responses path.</li>
          </ul>
          <p>Do not “clean up” this code by always rebuilding messages or silently switching to a copy. Either change breaks callers that combine compatibility fields or inspect the document after normalization. Add tests for existing messages, input-only requests, and observable in-place mutation.</p>
          <h4 id="2065-tracing-and-logging">20.6.5 Tracing and logging</h4>
          <p>Tracing should identify major waits and ownership boundaries: request handling, model acquisition/load, queue wait, prompt/prefill, generation, and unload when relevant. Keep spans concise. Avoid a span per token, duplicated nested timing, giant model-config attribute sets, prompt/media payloads, and unbounded IDs. Propagate the request context instead of creating unrelated roots. Logs and metrics should help distinguish queue, capacity, cancellation, and inference failures without exposing user content unless an explicit insecure-logging mode authorizes it.</p>
          <p>Embedding/reranking metrics distinguish the operation and selected runtime. Sequence- batch queue wait and batch width are engine-level measurements; request duration, active requests, status, and prompt-token usage are operation-level measurements. Resource reservations remain owned by the shared pool/resource manager and must not be duplicated inside either inference engine.</p>
          <h4 id="2066-speculative-decoding-and-mtp">20.6.6 Speculative decoding and MTP</h4>
          <p>Speculative support has three ownership shapes:</p>
          <ol>
            <li><strong>Separate GGUF draft model.</strong> The draft has its own model/context/KV and proposes tokens; the target verifies them. Loading, memory planning, sequence cleanup, and rollback must account for both models.</li>
            <li><strong>Embedded MTP.</strong> A target GGUF exposes an embedded multi-token-prediction head. Model detection and MTP construction belong to the model package; speculative scheduling and verification coordinate with <code>model/internal/speculation</code>.</li>
            <li><strong>Separate-file Gemma4/shared-target-KV MTP.</strong> The MTP component is supplied as a separate file but shares target KV semantics rather than behaving like an ordinary independent draft model. Capabilities, not “has a draft path,” must decide whether draft KV can be trimmed or externalized.</li>
          </ol>
          <p>Across all three, target output is authoritative. Proposal generation cannot expose a token until target verification accepts it or chooses the replacement/bonus token. Position counters, sampled-token history, target KV, draft/MTP state, and streamed output must describe one accepted prefix after every round.</p>
          <p>Verification in a multi-slot batch is explicitly read-before-mutate. First read all target logits/hidden-state rows and decide each slot's accepted prefix while the shared batch outputs are intact. Only then mutate KV, counters, slot buffers, stream output, or MTP mirror state. Mutating one slot during the read phase can invalidate indices or native output needed by another slot.</p>
          <p>Ordinary transformer KV can often remove a rejected suffix. Hybrid recurrent/state- space models cannot assume partial KV deletion restores prior state. Request bounded recurrent rollback through <code>NRsSeq</code> and use it only when the context reports enough effective rollback depth for the speculative round. Otherwise take a full pre-speculation per-sequence snapshot, and on rejection restore it and re-decode exactly the accepted prefix. Preserve captured target hidden-state rows needed to synchronize MTP. For own-KV MTP, rollback removes speculative draft state before mirroring accepted target state. For shared-target-KV Gemma4, do not apply independent-draft rollback to the shared target cache. If rollback, restore, or synchronization fails, fail the affected slot or safely disable MTP rather than retaining ambiguous state.</p>
          <p>Unit-level owners are the batch/speculative files and tests in <code>sdk/kronk/model/</code>. Model-backed MTP suites live in <code>sdk/kronk/tests/mtp</code> and <code>sdk/kronk/tests/gemma4mtp</code>; they are CI/human suites, not commands agents should launch from the forbidden integration-test tree.</p>
          <h3 id="207-server-bui-and-generated-documentation">20.7 Server, BUI, and Generated Documentation</h3>
          <h4 id="2071-routes-middleware-and-domains">20.7.1 Routes, middleware, and domains</h4>
          <p>Route declarations belong with their domain package, normally in <code>route.go</code>. Keep authentication/authorization, tracing, request IDs, panic recovery, and common response behavior in foundation middleware. Domain handlers decode and validate protocol input, select the appropriate application capability, call SDK/facade methods, and encode the protocol result. They should not manipulate native model state or implement pool eviction.</p>
          <p>When adding an endpoint, follow a neighboring domain end to end: registration, middleware order, request model, error mapping, streaming behavior, and service wiring. Test malformed input and cancellation as well as success. A server build catches route composition errors that a leaf-package test may miss.</p>
          <h4 id="2072-bui-ownership-and-embedding">20.7.2 BUI ownership and embedding</h4>
          <p>The BUI lives at <code>cmd/server/api/frontends/bui/</code>. Follow its own package scripts and the applicable component <code>AGENTS.md</code>; component structure and UI conventions change more quickly than this chapter. The production bundle is embedded by <code>cmd/server/api/services/kronk/main.go</code>. Editing TypeScript does not alter the server binary until the frontend is rebuilt and embedded output is rebuilt into Go.</p>
          <p>For frontend changes:</p>
          <pre className="code-block"><code className="language-shell">{`cd cmd/server/api/frontends/bui
npm run build`}</code></pre>
          <p>Then build the server (or the narrow service package) and verify that the expected static bundle is present in the embedding location. Avoid hand-editing minified/static output.</p>
          <h4 id="2073-documentation-generation">20.7.3 Documentation generation</h4>
          <p><code>cmd/server/api/tooling/docs/main.go</code> orchestrates four conceptual pipelines:</p>
          <pre className="code-block"><code className="language-text">{`registered Cobra commands   -> cli-reference.json -> CLI BUI reference and navigation
public SDK Go documentation -> SDK BUI documentation
examples source             -> example BUI documentation
.manual chapter Markdown    -> DocsManual.tsx`}</code></pre>
          <p>Author manual content in <code>.manual/</code>, public API descriptions in Go doc comments, and examples in <code>examples/</code>. The shared CLI tree lives in <code>cmd/kronk/commands/</code>. Author CLI descriptions and short examples in the owning Cobra command's <code>Long</code> and <code>Example</code> fields; usage, flags, types, defaults, and inherited flags come from the command definitions. The generator inspects the tree without executing commands or their authentication/platform hooks. New top-level commands automatically appear in the BUI sidebar. <code>DocsCLI.tsx</code> is the shared renderer, not a per-command reference.</p>
          <p>Keep operational workflows and authoritative environment-variable guidance in the manual rather than duplicating flag tables in React. Generated flag defaults are not necessarily effective server defaults: environment variables and configuration can override them. API reference components remain hand-written.</p>
          <p><code>cli-reference.json</code>, <code>DocsManual.tsx</code>, and generated SDK/example documentation are outputs and must not be hand-edited. The pre-commit hook regenerates docs and builds the BUI. To regenerate explicitly, run:</p>
          <pre className="code-block"><code className="language-shell">{`make kronk-docs`}</code></pre>
          <p>Review generated diffs for malformed Markdown conversion and then run <code>npm run build</code> in the BUI. Finally build the server to check that generated components compile into the embedded bundle. Generation may update more than one documented package; do not discard legitimate generated changes. If the requested scope intentionally excludes generated artifacts, report that regeneration remains pending.</p>
          <h3 id="208-bucky-implementation-map">20.8 Bucky Implementation Map</h3>
          <p>This is an implementation map only. Chapter 18 owns installation, configuration, streaming usage, and API examples.</p>
          <h4 id="2081-owners">20.8.1 Owners</h4>
          <ul>
            <li><strong>&lt;code&gt;sdk/bucky/&lt;/code&gt;</strong> owns initialization and the public <code>Bucky</code> handle. A handle owns one Whisper model and admission/shutdown coordination.</li>
            <li><strong>&lt;code&gt;sdk/bucky/model/&lt;/code&gt;</strong> owns the Whisper context, its pool of model states, audio decode/transcription primitives, language operations, and stream implementation. Model weights/context are shared by the handle while state isolates concurrent work.</li>
            <li><strong>&lt;code&gt;sdk/bucky/pool/&lt;/code&gt;</strong> adapts Bucky model planning, loading, status, unloading, and reservations to the generic typed pool.</li>
            <li><strong>&lt;code&gt;sdk/pool/&lt;/code&gt; and &lt;code&gt;sdk/pool/engine/&lt;/code&gt;</strong> let Bucky and Kronk share one resource budget while retaining backend-specific loaders and handles.</li>
            <li><strong>&lt;code&gt;sdk/tools/bucky/&lt;/code&gt;</strong> owns Whisper shared-library and model catalog/download work.</li>
            <li><strong>&lt;code&gt;cmd/kronk/bucky/&lt;/code&gt;</strong> exposes those tools. Web/server mode is default; <code>--local</code> requests direct local operation.</li>
            <li><strong>&lt;code&gt;cmd/server/app/domain/audioapp/&lt;/code&gt;</strong> owns the OpenAI-compatible transcription route. Administrative library/model routes are in <code>toolapp</code>. Service startup wires the Bucky backend and shared pool.</li>
          </ul>
          <h4 id="2082-lifecycle-invariants">20.8.2 Lifecycle invariants</h4>
          <p><code>Init</code> registers/resolves/loads the backend. Technically, a failed <code>Init</code> can be called again and retry. The current server calls it only during startup, however. Installing missing libraries through CLI or BUI does <strong>not</strong> promise automatic server re-init; restart the server so startup calls <code>Init</code> again.</p>
          <p>A transcription acquires handle capacity and a model state, performs decode/inference, then releases both on every completion path. A streaming session is longer-lived: opening it reserves a state and capacity until its worker exits. <code>Close</code> requests the normal final flush and waits for that exit; a terminal worker error also exits and releases automatically. Callers should still defer the idempotent <code>Close</code>, including when feed/event handling fails. Unload must not destroy the Whisper context while transcriptions or streams remain active.</p>
          <p>The state pool is sized by <code>Config.NSeqMax</code>, which defaults to 1. The admission channel is sized by <code>NSeqMax + QueueDepth</code>; queue depth defaults to 0. Calls beyond that capacity wait until their context is canceled, the configured admission timeout expires, or an operation releases admission capacity. The admission timeout defaults to three minutes and stops applying once capacity is acquired. The states isolate concurrent inference while sharing the handle's model weights and Whisper context.</p>
          <p>The Bucky planner reserves the shared model file size once and a conservative 200 MiB runtime allowance for every configured state. It does not charge <code>QueueDepth</code>, because waiting calls do not allocate a state. Planning and model loading use the same resolved configuration so the reservation scales with the actual <code>NSeqMax</code> used by the handle.</p>
          <p>The audio HTTP handler delegates file decoding and transcription to <code>Bucky.TranscribeFile</code>. It explicitly enforces the 25 MiB upload limit before allowing unbounded work. Keep protocol field validation/format selection in the handler and audio/model mechanics in Bucky.</p>
          <p>Focused tests that exist include unit tests under <code>sdk/bucky/model/</code> and <code>sdk/bucky/ffmpeg/</code>, transcription/pool/stream suites under <code>sdk/bucky/tests/transcribe/</code>, and the server audio API tests under <code>cmd/server/api/services/kronk/tests/</code>. Choose the narrowest test whose native library and model prerequisites are available. Do not duplicate Chapter 18's usage matrix here.</p>
          <h3 id="209-verification-for-llm-agents">20.9 Verification for LLM Agents</h3>
          <h4 id="2091-required-go-post-edit-sequence">20.9.1 Required Go post-edit sequence</h4>
          <p>After changing Go, obey the root instructions and scope work to the changed package. For each changed Go file/package:</p>
          <pre className="code-block"><code className="language-shell">{`go fix ./path/to/changed/package
gofmt -s -w path/to/all-changed.go
go vet ./path/to/changed/package
staticcheck ./path/to/changed/package`}</code></pre>
          <p>Use exact package paths rather than <code>./...</code>. If several packages changed, list those packages explicitly or run commands separately so failures remain attributable. Review <code>go fix</code> output/diff because it may modify additional files; include every resulting Go file in the subsequent <code>gofmt</code> and review.</p>
          <p>Before focused Go tests, set:</p>
          <pre className="code-block"><code className="language-shell">{`export GITHUB_WORKSPACE="$(pwd -P)"`}</code></pre>
          <p><code>GITHUB_WORKSPACE</code> must be the absolute repository root. Then run a package test or a specific test, for example:</p>
          <pre className="code-block"><code className="language-shell">{`go test -count=1 ./sdk/kronk/model
go test -count=1 -run 'TestSpecificBehavior' ./sdk/kronk/parsers/qwen`}</code></pre>
          <p>Agents must <strong>never prescribe or run a full repository test run</strong>, and must <strong>never launch tests from &lt;code&gt;sdk/kronk/tests&lt;/code&gt;</strong>. Those suites require managed libraries/models and belong to CI or deliberate human integration runs. Commands such as <code>make test</code> and <code>make test-adversarial</code> (<a href="#2042-server-adversarial-probes">20.4.2</a>) exist as broad human/CI-maintainer context, but they are not the agent default. Do not use a broad command merely because focused ownership is unclear; inspect the owner.</p>
          <h4 id="2092-choosing-effective-checks">20.9.2 Choosing effective checks</h4>
          <ul>
            <li>Pure logic changes: focused unit test plus package static checks.</li>
            <li>Public API changes: owner tests, direct dependent package build/test, and generated SDK docs when comments/signatures changed.</li>
            <li>Batch/native changes: focused model unit tests first; report model-backed validation as unavailable if prerequisites are absent rather than substituting unrelated tests.</li>
            <li>Pool changes: engine tests plus the affected typed pool's reservation/load tests. Include failure and cancellation, not only warm acquisition.</li>
            <li>Route changes: domain tests and a server build. Use the relevant API test only when its server/model prerequisites are available.</li>
            <li>CLI changes: command package tests, <code>go install ./cmd/kronk</code>, and one safe invocation.</li>
            <li>Bucky changes: package tests; transcription integration only with installed Whisper libraries/model. Streaming changes require close/cancellation coverage.</li>
            <li>Malina changes: package tests and affected example builds. Native generation requires compatible stable-diffusion.cpp libraries and multi-gigabyte model bundles, so keep model-backed runs deliberate and do not make them an unconditional CI requirement.</li>
          </ul>
          <h4 id="2093-markdown-generated-docs-and-bui">20.9.3 Markdown, generated docs, and BUI</h4>
          <p>For a manual-only edit, run formatting/sanity checks appropriate to the task, including <code>git diff --check</code>. <code>make kronk-docs</code> validates the manual conversion pipeline but also changes generated <code>DocsManual</code>; honor any task restriction on edited files. For normal documentation changes, commit source and generated output together, then run the BUI build.</p>
          <p>For BUI changes, install dependencies according to its lockfile/workflow and run:</p>
          <pre className="code-block"><code className="language-shell">{`npm run build`}</code></pre>
          <p>Run it from <code>cmd/server/api/frontends/bui/</code>. For docs or BUI work that affects the production bundle, also build the server to verify the bundle embedded by <code>cmd/server/api/services/kronk/main.go</code> exists and compiles. A successful Vite build alone does not prove the Go binary contains current assets.</p>
          <p>Always report what actually ran, including skipped integration prerequisites. Never claim CI parity from a narrower local test.</p>
          <h3 id="2010-ci-release-containers-and-nix">20.10 CI, Release, Containers, and Nix</h3>
          <h4 id="20101-linux-ci">20.10.1 Linux CI</h4>
          <p><code>.github/workflows/linux.yml</code> is the authoritative Linux pipeline. It currently has four parallel jobs:</p>
          <ul>
            <li><code>static</code>: source/static quality checks;</li>
            <li><code>race</code>: race-enabled focused coverage separated from static checks;</li>
            <li><code>api-tests</code>: server/API integration coverage;</li>
            <li><code>sdk-tests</code>: SDK/model integration coverage.</li>
          </ul>
          <p>The shared setup action is under <code>.github/actions/setup-kronk/</code>. CI model dependencies are declared in <code>.github/test-models.txt</code>; its contents also participate in cache behavior. When a CI test gains a required model, update the manifest with the correct backend and model ID and check the setup action's parser. Keep local human setup in sync where the Make workflow maintains a separate install list.</p>
          <p>The exact CI toolchain comes from <code>.go-version</code>, while <code>go.mod</code> declares the minimum language version. Their major/minor versions must match. Update workflow assumptions and run <code>.github/scripts/check-go-version.sh</code> when changing either.</p>
          <h4 id="20102-release">20.10.2 Release</h4>
          <p>The release workflow, GoReleaser configuration, scripts, and release notes divide responsibility:</p>
          <ul>
            <li><code>.github/workflows/release.yaml</code> owns trigger, permissions, setup, checks, and release execution.</li>
            <li><code>.goreleaser.yaml</code> owns binary/archive packaging and related release products.</li>
            <li><code>.github/scripts/check-version.sh</code> enforces the release identity.</li>
            <li><code>sdk/kronk/kronk.go</code> owns the exported <code>Version</code> constant.</li>
            <li><code>.release/</code> owns maintained release-note/checklist material.</li>
          </ul>
          <p>The release tag must equal <code>v</code> plus the <code>Version</code> constant. Update the constant and release material intentionally before creating the tag; do not bypass the guard. Also confirm the Go major/minor guard, generated docs/BUI, clean tree, and relevant Linux jobs before tagging.</p>
          <h4 id="20103-containers">20.10.3 Containers</h4>
          <p><code>.github/workflows/docker.yml</code> is authoritative for image variants, target/platform matrix, registry publication, attestations, and signing. <code>zarf/docker/</code> owns Dockerfile, runtime configuration, and entrypoint behavior. Native llama and Bucky processor availability can differ by image variant, so inspect the workflow matrix and tooling combination tables before changing an image. Avoid copying a variant table into docs; it becomes stale faster than the workflow.</p>
          <p>For a container change, build the affected target and architecture where practical, exercise entrypoint startup/configuration, and verify expected native libraries. Do not infer publication or signature behavior from a local build; review the workflow.</p>
          <p><strong>Helm chart</strong></p>
          <p><code>zarf/helm/kronk/</code> owns the chart, CPU/CUDA examples, and chart checks. Validate chart changes from the repository root with:</p>
          <pre className="code-block"><code className="language-shell">{`zarf/helm/kronk/test.sh`}</code></pre>
          <p>This requires Helm and Python 3 with PyYAML. Chart-only changes do not require Go tests. For integration, use <code>zarf/helm/kronk/smoke-test.sh</code> only against an explicitly selected disposable Kubernetes context; inspect its usage and required tools first. Do not use your ambient production context. Rendering checks are not proof of cluster startup or GPU inference. GPU examples require real-node validation with compatible hardware, drivers, device plugins, and runtimes; disposable kind smoke tests do not prove GPU inference.</p>
          <p>Keep the single-replica/retained-PVC contract, authenticated defaults, Secret keyset reconciliation, and private metrics Service intact. Update <code>zarf/helm/kronk/README.md</code> when changing operator-facing values; keep <a href="https://www.kronkai.com/manual#810-kubernetes-with-helm">Chapter 8 §8.10</a> focused on installation and essential warnings. Bump the chart <code>version</code> for chart changes; <code>appVersion</code> must reference an already published container image, not an unreleased CLI version. <code>zarf/helm/kronk/Chart.yaml</code> is authoritative for the default app version. Installation uses the local checkout; there is no Helm OCI publication workflow to run.</p>
          <h4 id="20104-nix">20.10.4 Nix</h4>
          <p>The flake at <code>zarf/nix/flake.nix</code> defines how developers/users enter or build the project; generated Go dependency data lives beside it. Entering a development shell runs <code>gomod2nix import</code> from its shell hook and may dirty that generated material. When Go module dependencies change, update the Nix dependency material with the repository's configured command and evaluate/build the relevant entry point. Keep Nix fixes in Nix owners rather than adding environment special cases to Go code.</p>
          <h3 id="2011-change-and-release-checklists">20.11 Change and Release Checklists</h3>
          <h4 id="20111-focused-change-checklist">20.11.1 Focused change checklist</h4>
          <ul>
            <li>[ ] Read all applicable <code>AGENTS.md</code> files.</li>
            <li>[ ] Locate the owning package, direct caller, and focused tests.</li>
            <li>[ ] State lifecycle/mutation/resource invariants before editing.</li>
            <li>[ ] Change the owner rather than duplicating logic in a facade or transport.</li>
            <li>[ ] Preserve cancellation and release behavior on every return path.</li>
            <li>[ ] For Go: run <code>gofmt -s</code>, <code>go fix</code>, <code>go vet</code>, and <code>staticcheck</code> scoped to changed files/packages.</li>
            <li>[ ] Set absolute <code>GITHUB_WORKSPACE</code> for focused tests.</li>
            <li>[ ] Do not run a full repository suite or launch tests from <code>sdk/kronk/tests</code> as an agent.</li>
            <li>[ ] Regenerate docs/BUI/Nix artifacts only when their sources changed and task scope permits it.</li>
            <li>[ ] Run <code>git diff --check</code> and inspect the complete diff for unrelated changes.</li>
            <li>[ ] Report commands, results, skipped prerequisites, and residual uncertainty.</li>
          </ul>
          <h4 id="20112-release-checklist">20.11.2 Release checklist</h4>
          <ul>
            <li>[ ] Choose the release version and update <code>sdk/kronk</code>'s <code>Version</code> constant.</li>
            <li>[ ] Ensure the intended tag is exactly <code>v&lt;Version&gt;</code> and run the version guard.</li>
            <li>[ ] Confirm <code>.go-version</code> and <code>go.mod</code> major/minor agreement and run the Go-version guard.</li>
            <li>[ ] Update release notes/changelog material under the repository's release process.</li>
            <li>[ ] Ensure <code>.github/test-models.txt</code> covers model-backed CI requirements.</li>
            <li>[ ] Regenerate documentation and BUI assets; build the BUI and server embedding.</li>
            <li>[ ] Confirm focused package checks and the four Linux CI jobs are green.</li>
            <li>[ ] Run <code>make test-adversarial</code> against a generation-capable model on a machine that has one, and review <code>.tools/adversarial/output/findings.txt</code>.</li>
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
                <li><a href="#11-what-is-kronk" className={activeSection === '11-what-is-kronk' ? 'active' : ''}>1.1 What Is Kronk</a></li>
                <li><a href="#12-sdk-or-model-server" className={activeSection === '12-sdk-or-model-server' ? 'active' : ''}>1.2 SDK or Model Server</a></li>
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
                <li><a href="#29-cli-reference-and-shell-completion" className={activeSection === '29-cli-reference-and-shell-completion' ? 'active' : ''}>2.9 CLI Reference and Shell Completion</a></li>
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
                <li><a href="#41-runtime-mental-model" className={activeSection === '41-runtime-mental-model' ? 'active' : ''}>4.1 Runtime Mental Model</a></li>
                <li><a href="#42-the-four-stage-request-lifecycle" className={activeSection === '42-the-four-stage-request-lifecycle' ? 'active' : ''}>4.2 The Four-Stage Request Lifecycle</a></li>
                <li><a href="#43-the-generation-inference-lifecycle" className={activeSection === '43-the-generation-inference-lifecycle' ? 'active' : ''}>4.3 The Generation Inference Lifecycle</a></li>
                <li><a href="#44-stage-1-—-admit-the-request" className={activeSection === '44-stage-1-—-admit-the-request' ? 'active' : ''}>4.4 Stage 1 — Admit the Request</a></li>
                <li><a href="#45-stage-2-—-prepare-model-work" className={activeSection === '45-stage-2-—-prepare-model-work' ? 'active' : ''}>4.5 Stage 2 — Prepare Model Work</a></li>
                <li><a href="#46-stage-3-—-schedule-the-job" className={activeSection === '46-stage-3-—-schedule-the-job' ? 'active' : ''}>4.6 Stage 3 — Schedule the Job</a></li>
                <li><a href="#47-stage-4-—-execute-in-the-slot" className={activeSection === '47-stage-4-—-execute-in-the-slot' ? 'active' : ''}>4.7 Stage 4 — Execute in the Slot</a></li>
                <li><a href="#48-embedding-and-reranking" className={activeSection === '48-embedding-and-reranking' ? 'active' : ''}>4.8 Embedding and Reranking</a></li>
                <li><a href="#49-configuration-and-tuning" className={activeSection === '49-configuration-and-tuning' ? 'active' : ''}>4.9 Configuration and Tuning</a></li>
                <li><a href="#410-interaction-with-message-caching" className={activeSection === '410-interaction-with-message-caching' ? 'active' : ''}>4.10 Interaction with Message Caching</a></li>
                <li><a href="#411-observing-queue-behavior" className={activeSection === '411-observing-queue-behavior' ? 'active' : ''}>4.11 Observing Queue Behavior</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-5-message-caching" className={`doc-index-header ${activeSection === 'chapter-5-message-caching' ? 'active' : ''}`}>Chapter 5: Message Caching</a>
            </div>
            <div className="doc-index-section">
              <a href="#51-what-imc-does" className={`doc-index-header ${activeSection === '51-what-imc-does' ? 'active' : ''}`}>5.1 What IMC Does</a>
              <ul>
                <li><a href="#511-quick-semantic-understanding" className={activeSection === '511-quick-semantic-understanding' ? 'active' : ''}>5.1.1 Quick Semantic Understanding</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#52-how-kronk-reuses-a-text-prefix" className={`doc-index-header ${activeSection === '52-how-kronk-reuses-a-text-prefix' ? 'active' : ''}`}>5.2 How Kronk Reuses a Text Prefix</a>
              <ul>
                <li><a href="#521-system-cache-lifecycle-and-template-behavior" className={activeSection === '521-system-cache-lifecycle-and-template-behavior' ? 'active' : ''}>5.2.1 System Cache Lifecycle and Template Behavior</a></li>
              </ul>
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
                <li><a href="#built-in-ram-storage" className={activeSection === 'built-in-ram-storage' ? 'active' : ''}>Built-in RAM storage</a></li>
                <li><a href="#custom-sdk-storage" className={activeSection === 'custom-sdk-storage' ? 'active' : ''}>Custom SDK storage</a></li>
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
                <li><a href="#64-draft-size-and-classic-adaptive-throttling" className={activeSection === '64-draft-size-and-classic-adaptive-throttling' ? 'active' : ''}>6.4 Draft Size and Classic Adaptive Throttling</a></li>
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
              <ul>
                <li><a href="#runtime-selection-and-overrides" className={activeSection === 'runtime-selection-and-overrides' ? 'active' : ''}>Runtime selection and overrides</a></li>
              </ul>
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
              <a href="#88-running-as-a-systemd-service" className={`doc-index-header ${activeSection === '88-running-as-a-systemd-service' ? 'active' : ''}`}>8.8 Running as a systemd Service</a>
              <ul>
                <li><a href="#installation-and-startup" className={activeSection === 'installation-and-startup' ? 'active' : ''}>Installation and Startup</a></li>
                <li><a href="#configuration" className={activeSection === 'configuration' ? 'active' : ''}>Configuration</a></li>
                <li><a href="#managing-and-removing-the-service" className={activeSection === 'managing-and-removing-the-service' ? 'active' : ''}>Managing and Removing the Service</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#89-running-as-a-launchd-agent" className={`doc-index-header ${activeSection === '89-running-as-a-launchd-agent' ? 'active' : ''}`}>8.9 Running as a launchd Agent</a>
              <ul>
                <li><a href="#launchagent-configuration" className={activeSection === 'launchagent-configuration' ? 'active' : ''}>LaunchAgent Configuration</a></li>
                <li><a href="#launchagent-lifecycle-and-logs" className={activeSection === 'launchagent-lifecycle-and-logs' ? 'active' : ''}>LaunchAgent Lifecycle and Logs</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#810-kubernetes-with-helm" className={`doc-index-header ${activeSection === '810-kubernetes-with-helm' ? 'active' : ''}`}>8.10 Kubernetes with Helm</a>
            </div>
            <div className="doc-index-section">
              <a href="#811-related-administration-guides" className={`doc-index-header ${activeSection === '811-related-administration-guides' ? 'active' : ''}`}>8.11 Related Administration Guides</a>
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
              <a href="#98-decisions" className={`doc-index-header ${activeSection === '98-decisions' ? 'active' : ''}`}>9.8 Decisions</a>
              <ul>
                <li><a href="#sdk-examples" className={activeSection === 'sdk-examples' ? 'active' : ''}>SDK examples</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#99-tokenization" className={`doc-index-header ${activeSection === '99-tokenization' ? 'active' : ''}`}>9.9 Tokenization</a>
            </div>
            <div className="doc-index-section">
              <a href="#910-models-image-generation-and-audio-transcription" className={`doc-index-header ${activeSection === '910-models-image-generation-and-audio-transcription' ? 'active' : ''}`}>9.10 Models, Image Generation, and Audio Transcription</a>
            </div>
            <div className="doc-index-section">
              <a href="#911-kronk-administration" className={`doc-index-header ${activeSection === '911-kronk-administration' ? 'active' : ''}`}>9.11 Kronk Administration</a>
              <ul>
                <li><a href="#libraries" className={activeSection === 'libraries' ? 'active' : ''}>Libraries</a></li>
                <li><a href="#models" className={activeSection === 'models' ? 'active' : ''}>Models</a></li>
                <li><a href="#catalog" className={activeSection === 'catalog' ? 'active' : ''}>Catalog</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#912-bucky-and-malina-administration" className={`doc-index-header ${activeSection === '912-bucky-and-malina-administration' ? 'active' : ''}`}>9.12 Bucky and Malina Administration</a>
              <ul>
                <li><a href="#malina-models-and-runtime-integrity" className={activeSection === 'malina-models-and-runtime-integrity' ? 'active' : ''}>Malina Models and Runtime Integrity</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#913-operations-and-evaluation" className={`doc-index-header ${activeSection === '913-operations-and-evaluation' ? 'active' : ''}`}>9.13 Operations and Evaluation</a>
            </div>
            <div className="doc-index-section">
              <a href="#914-security-administration" className={`doc-index-header ${activeSection === '914-security-administration' ? 'active' : ''}`}>9.14 Security Administration</a>
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
              <a href="#121-authorization-modes" className={`doc-index-header ${activeSection === '121-authorization-modes' ? 'active' : ''}`}>12.1 Authorization Modes</a>
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
                <li><a href="#186-transcriptions-and-translations-api" className={activeSection === '186-transcriptions-and-translations-api' ? 'active' : ''}>18.6 Transcriptions and Translations API</a></li>
                <li><a href="#187-capability-boundaries-and-developer-resources" className={activeSection === '187-capability-boundaries-and-developer-resources' ? 'active' : ''}>18.7 Capability Boundaries and Developer Resources</a></li>
                <li><a href="#188-languages" className={activeSection === '188-languages' ? 'active' : ''}>18.8 Languages</a></li>
                <li><a href="#189-troubleshooting" className={activeSection === '189-troubleshooting' ? 'active' : ''}>18.9 Troubleshooting</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-19-malina-image-generation" className={`doc-index-header ${activeSection === 'chapter-19-malina-image-generation' ? 'active' : ''}`}>Chapter 19: Malina (Image Generation)</a>
              <ul>
                <li><a href="#191-overview" className={activeSection === '191-overview' ? 'active' : ''}>19.1 Overview</a></li>
                <li><a href="#192-install-stable-diffusion-libraries" className={activeSection === '192-install-stable-diffusion-libraries' ? 'active' : ''}>19.2 Install Stable Diffusion Libraries</a></li>
                <li><a href="#193-manage-model-bundles" className={activeSection === '193-manage-model-bundles' ? 'active' : ''}>19.3 Manage Model Bundles</a></li>
                <li><a href="#194-server-configuration-and-resources" className={activeSection === '194-server-configuration-and-resources' ? 'active' : ''}>19.4 Server Configuration and Resources</a></li>
                <li><a href="#195-browser-ui" className={activeSection === '195-browser-ui' ? 'active' : ''}>19.5 Browser UI</a></li>
                <li><a href="#196-image-generation-and-editing-api" className={activeSection === '196-image-generation-and-editing-api' ? 'active' : ''}>19.6 Image Generation and Editing API</a></li>
                <li><a href="#197-progress-and-administration" className={activeSection === '197-progress-and-administration' ? 'active' : ''}>19.7 Progress and Administration</a></li>
                <li><a href="#198-limitations-and-developer-resources" className={activeSection === '198-limitations-and-developer-resources' ? 'active' : ''}>19.8 Limitations and Developer Resources</a></li>
                <li><a href="#199-troubleshooting" className={activeSection === '199-troubleshooting' ? 'active' : ''}>19.9 Troubleshooting</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#chapter-20-developer-guide" className={`doc-index-header ${activeSection === 'chapter-20-developer-guide' ? 'active' : ''}`}>Chapter 20: Developer Guide</a>
              <ul>
                <li><a href="#201-how-to-use-this-guide" className={activeSection === '201-how-to-use-this-guide' ? 'active' : ''}>20.1 How to Use This Guide</a></li>
                <li><a href="#202-task-to-owner-and-verification-map" className={activeSection === '202-task-to-owner-and-verification-map' ? 'active' : ''}>20.2 Task-to-Owner and Verification Map</a></li>
                <li><a href="#203-repository-ownership-map" className={activeSection === '203-repository-ownership-map' ? 'active' : ''}>20.3 Repository Ownership Map</a></li>
                <li><a href="#204-developer-setup-and-daily-commands" className={activeSection === '204-developer-setup-and-daily-commands' ? 'active' : ''}>20.4 Developer Setup and Daily Commands</a></li>
                <li><a href="#205-request-and-model-lifecycle" className={activeSection === '205-request-and-model-lifecycle' ? 'active' : ''}>20.5 Request and Model Lifecycle</a></li>
                <li><a href="#206-core-inference-invariants" className={activeSection === '206-core-inference-invariants' ? 'active' : ''}>20.6 Core Inference Invariants</a></li>
                <li><a href="#207-server-bui-and-generated-documentation" className={activeSection === '207-server-bui-and-generated-documentation' ? 'active' : ''}>20.7 Server, BUI, and Generated Documentation</a></li>
                <li><a href="#208-bucky-implementation-map" className={activeSection === '208-bucky-implementation-map' ? 'active' : ''}>20.8 Bucky Implementation Map</a></li>
                <li><a href="#209-verification-for-llm-agents" className={activeSection === '209-verification-for-llm-agents' ? 'active' : ''}>20.9 Verification for LLM Agents</a></li>
                <li><a href="#2010-ci-release-containers-and-nix" className={activeSection === '2010-ci-release-containers-and-nix' ? 'active' : ''}>20.10 CI, Release, Containers, and Nix</a></li>
                <li><a href="#2011-change-and-release-checklists" className={activeSection === '2011-change-and-release-checklists' ? 'active' : ''}>20.11 Change and Release Checklists</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
