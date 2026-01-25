export default function DocsConfigModels() {
  return (
    <div>
      <div className="page-header">
        <h2>Model Configuration</h2>
        <p>Configuration options and suggested settings for running models with the Kronk Model Server.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="config-options">
            <h3>Configuration Options</h3>
            <p>These options can be set per-model in the <code>model_config.yaml</code> file located in the KMS data directory.</p>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Option</th>
                  <th>Default</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>context-window</code></td>
                  <td>8192</td>
                  <td>Max tokens model can process at once</td>
                </tr>
                <tr>
                  <td><code>nbatch</code></td>
                  <td>2048</td>
                  <td>Logical batch size for forward passes</td>
                </tr>
                <tr>
                  <td><code>nubatch</code></td>
                  <td>512</td>
                  <td>Physical batch size for prompt ingestion</td>
                </tr>
                <tr>
                  <td><code>nthreads</code></td>
                  <td>0</td>
                  <td>Threads for generation (0 = llama.cpp default)</td>
                </tr>
                <tr>
                  <td><code>nthreads-batch</code></td>
                  <td>0</td>
                  <td>Threads for batch processing (0 = llama.cpp default)</td>
                </tr>
                <tr>
                  <td><code>cache-type-k</code></td>
                  <td>auto</td>
                  <td>KV cache key type: f32, f16, q8_0, q4_0, bf16, auto</td>
                </tr>
                <tr>
                  <td><code>cache-type-v</code></td>
                  <td>auto</td>
                  <td>KV cache value type: f32, f16, q8_0, q4_0, bf16, auto</td>
                </tr>
                <tr>
                  <td><code>flash-attention</code></td>
                  <td>enabled</td>
                  <td>Flash Attention mode: enabled, disabled, auto</td>
                </tr>
                <tr>
                  <td><code>device</code></td>
                  <td>""</td>
                  <td>Device to use (run <code>llama-bench --list-devices</code>)</td>
                </tr>
                <tr>
                  <td><code>nseq-max</code></td>
                  <td>0</td>
                  <td>Max parallel sequences for batched inference (0 = default)</td>
                </tr>
                <tr>
                  <td><code>offload-kqv</code></td>
                  <td>true</td>
                  <td>Offload KV cache to GPU (false = keep on CPU)</td>
                </tr>
                <tr>
                  <td><code>op-offload</code></td>
                  <td>true</td>
                  <td>Offload tensor operations to GPU (false = keep on CPU)</td>
                </tr>
                <tr>
                  <td><code>ngpu-layers</code></td>
                  <td>0</td>
                  <td>GPU layers to offload (0 = all, -1 = none, N = specific count)</td>
                </tr>
                <tr>
                  <td><code>split-mode</code></td>
                  <td>row</td>
                  <td>Multi-GPU split: none, layer, row (row recommended for MoE models)</td>
                </tr>
                <tr>
                  <td><code>system-prompt-cache</code></td>
                  <td>false</td>
                  <td>Cache system prompt KV state for reuse across requests</td>
                </tr>
                <tr>
                  <td><code>first-message-cache</code></td>
                  <td>false</td>
                  <td>Cache first user message KV state (for clients like Cline)</td>
                </tr>
                <tr>
                  <td><code>cache-min-tokens</code></td>
                  <td>100</td>
                  <td>Min tokens before caching (applies to both cache types)</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="quantization">
            <h3>Model Quantization Guide</h3>
            <p>These suffixes define how a Large Language Model (LLM) is quantized—compressed from its original size to fit into computer memory (RAM/VRAM). The primary difference lies in the balance between model size, inference speed, and output accuracy.</p>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Suffix</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>F16</code></td>
                  <td>Unquantized, maximum quality, largest size</td>
                </tr>
                <tr>
                  <td><code>Q8_0</code></td>
                  <td>8-bit, near-lossless compression, very high quality (best for local)</td>
                </tr>
                <tr>
                  <td><code>K_XL</code></td>
                  <td>K-quant (smart) method, high efficiency, better accuracy than standard _0 at similar sizes</td>
                </tr>
              </tbody>
            </table>
            <p>FP8/F16 or even FP4 are more for modern NVIDIA cards, defining the precision of floating point numbers. All operations are accelerated by the GPU.</p>
            <p>Most modern GPUs have hardware support for FP16. Other formats like FP8 (Q8) or FP4 may be emulated. If you have hardware support for FP8, it will be roughly double the speed of FP16.</p>
          </div>

          <div className="card" id="slot-memory">
            <h3>Slot Memory Cost Formula</h3>
            <p>KV memory per slot = n_ctx × (K + V bytes) × n_layers</p>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Model</th>
                  <th>Context</th>
                  <th>Memory per Slot</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td>7B</td>
                  <td>8K</td>
                  <td>~537 MB</td>
                </tr>
                <tr>
                  <td>70B</td>
                  <td>8K</td>
                  <td>~1.3 GB</td>
                </tr>
              </tbody>
            </table>
            <p><strong>Key finding:</strong> Memory is statically allocated upfront when the model loads, based on n_ctx × n_seq_max. Reserving slots consumes memory whether or not they're actually used.</p>
            <p><strong>Advice:</strong> Reserving 2 dedicated slots (one for system prompt cache, one for user message cache) would add +537MB (7B) to +1.3GB (70B) overhead. Since the slots would rarely both be used simultaneously, you'd be "paying for memory you don't benefit from."</p>
          </div>

          <div className="card" id="usage-notes">
            <h3>Configuration Notes</h3>
            <p>If you want to use a model with an Agent, use these settings:</p>
            <pre className="code-block">
              <code>{`nseq-max: 1
first-message-cache: true`}</code>
            </pre>
            <p>If you want to use a Chat application like OpenWebUI, use these settings:</p>
            <pre className="code-block">
              <code>{`system-prompt-cache: true`}</code>
            </pre>
            <p><strong>Cline:</strong> Works great with <code>cerebras_qwen3-coder-reap-25b-a3b-q8_0</code></p>
            <p><strong>Claude Code:</strong> Needs a model that handles tool calling well with a decent context window. The GPT models have not performed well. No ideal model has been found yet.</p>
          </div>

          <div className="card" id="suggested-settings">
            <h3>Suggested Settings</h3>
            <p>Pre-configured model settings for common use cases.</p>

            <div className="doc-section" id="suggested-settings--coding-agents">
              <h4>Coding Agents (Cline / Claude Code)</h4>
              <p className="doc-description">Models configured for use with coding agents.</p>
              <h5>GLM-4.7-Flash-UD-Q8_K_XL</h5>
              <p>Good model, works well with Cline, faster than alternatives.</p>
              <pre className="code-block">
                <code>{`GLM-4.7-Flash-UD-Q8_K_XL:
  context-window: 131072
  nbatch: 2048
  nubatch: 512
  cache-type-k: q8_0
  cache-type-v: q8_0
  flash-attention: enabled
  nseq-max: 1
  first-message-cache: true`}</code>
              </pre>

              <h5>Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL</h5>
              <p>Good model, works well with Cline.</p>
              <pre className="code-block">
                <code>{`Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL:
  context-window: 131072
  nbatch: 2048
  nubatch: 512
  cache-type-k: q8_0
  cache-type-v: q8_0
  flash-attention: enabled
  nseq-max: 1
  first-message-cache: true`}</code>
              </pre>

              <h5>cerebras_qwen3-coder-reap-25b-a3b-q8_0</h5>
              <p>Decent model, works ok with Cline.</p>
              <pre className="code-block">
                <code>{`cerebras_qwen3-coder-reap-25b-a3b-q8_0:
  context-window: 131072
  nbatch: 2048
  nubatch: 512
  cache-type-k: q8_0
  cache-type-v: q8_0
  flash-attention: enabled
  nseq-max: 1
  first-message-cache: true`}</code>
              </pre>

              <h5>Qwen3-Coder-30B-A3B-Instruct-Q8_0</h5>
              <p>Decent model, works ok with Cline.</p>
              <pre className="code-block">
                <code>{`Qwen3-Coder-30B-A3B-Instruct-Q8_0:
  context-window: 131072
  nbatch: 2048
  nubatch: 512
  cache-type-k: q8_0
  cache-type-v: q8_0
  nseq-max: 1
  first-message-cache: true`}</code>
              </pre>
            </div>

            <div className="doc-section" id="suggested-settings--reasoning-models">
              <h4>Reasoning Models (with Tool Support)</h4>
              <p className="doc-description">Good reasoning models with tooling support. Not recommended for agents.</p>

              <h5>gpt-oss-120b-F16</h5>
              <pre className="code-block">
                <code>{`gpt-oss-120b-F16:
  context-window: 131072
  nbatch: 2048
  nubatch: 512
  cache-type-k: q8_0
  cache-type-v: q8_0
  flash-attention: enabled
  nseq-max: 2`}</code>
              </pre>

              <h5>gpt-oss-20b-Q8_0</h5>
              <pre className="code-block">
                <code>{`gpt-oss-20b-Q8_0:
  context-window: 98304
  nbatch: 2048
  nubatch: 512
  cache-type-k: q8_0
  cache-type-v: q8_0
  flash-attention: enabled
  nseq-max: 2`}</code>
              </pre>

              <h5>Qwen3-8B-Q8_0</h5>
              <p>Great model but small context window.</p>
              <pre className="code-block">
                <code>{`Qwen3-8B-Q8_0:
  context-window: 40960
  nbatch: 2048
  nubatch: 512
  cache-type-k: q8_0
  cache-type-v: q8_0
  flash-attention: enabled
  nseq-max: 2`}</code>
              </pre>
            </div>

            <div className="doc-section" id="suggested-settings--non-reasoning-models">
              <h4>Non-Reasoning Models (with Tool Support)</h4>
              <p className="doc-description">Models with tooling support but no reasoning. Not recommended for agents.</p>

              <h5>GLM-4.7-Flash-Q8_0</h5>
              <p>Runs slow, not extensively tested.</p>
              <pre className="code-block">
                <code>{`GLM-4.7-Flash-Q8_0:
  context-window: 131072
  nbatch: 2048
  nubatch: 512
  cache-type-k: q8_0
  cache-type-v: q8_0
  flash-attention: enabled
  nseq-max: 2`}</code>
              </pre>
            </div>

            <div className="doc-section" id="suggested-settings--vision-models">
              <h4>Vision Models</h4>
              <h5>Qwen2.5-VL-3B-Instruct-Q8_0</h5>
              <p>Vision model that works great.</p>
              <pre className="code-block">
                <code>{`Qwen2.5-VL-3B-Instruct-Q8_0:
  context-window: 8192
  nbatch: 2048
  nubatch: 2048
  cache-type-k: q8_0
  cache-type-v: q8_0
  flash-attention: enabled`}</code>
              </pre>
            </div>

            <div className="doc-section" id="suggested-settings--audio-models">
              <h4>Audio Models</h4>
              <h5>Qwen2-Audio-7B.Q8_0</h5>
              <p>Audio model that works great.</p>
              <pre className="code-block">
                <code>{`Qwen2-Audio-7B.Q8_0:
  context-window: 8192
  nbatch: 2048
  nubatch: 512
  cache-type-k: q8_0
  cache-type-v: q8_0
  flash-attention: enabled`}</code>
              </pre>
            </div>

            <div className="doc-section" id="suggested-settings--embedding-models">
              <h4>Embedding Models</h4>
              <h5>embeddinggemma-300m-qat-Q8_0</h5>
              <p>Embedding model that works great.</p>
              <pre className="code-block">
                <code>{`embeddinggemma-300m-qat-Q8_0:
  context-window: 2048
  nbatch: 2048
  nubatch: 512
  cache-type-k: q8_0
  cache-type-v: q8_0
  flash-attention: enabled`}</code>
              </pre>
            </div>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#config-options" className="doc-index-header">Configuration Options</a>
            </div>
            <div className="doc-index-section">
              <a href="#quantization" className="doc-index-header">Model Quantization Guide</a>
            </div>
            <div className="doc-index-section">
              <a href="#slot-memory" className="doc-index-header">Slot Memory Cost Formula</a>
            </div>
            <div className="doc-index-section">
              <a href="#usage-notes" className="doc-index-header">Configuration Notes</a>
            </div>
            <div className="doc-index-section">
              <a href="#suggested-settings" className="doc-index-header">Suggested Settings</a>
              <ul>
                <li><a href="#suggested-settings--coding-agents">Coding Agents</a></li>
                <li><a href="#suggested-settings--reasoning-models">Reasoning Models</a></li>
                <li><a href="#suggested-settings--non-reasoning-models">Non-Reasoning Models</a></li>
                <li><a href="#suggested-settings--vision-models">Vision Models</a></li>
                <li><a href="#suggested-settings--audio-models">Audio Models</a></li>
                <li><a href="#suggested-settings--embedding-models">Embedding Models</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
