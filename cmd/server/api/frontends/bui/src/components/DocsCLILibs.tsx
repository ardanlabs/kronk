export default function DocsCLILibs() {
  return (
    <div>
      <div className="page-header">
        <h2>libs</h2>
        <p>Install or upgrade llama.cpp libraries.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk libs [flags]</code>
            </pre>
            <p>
              Kronk requires llama.cpp shared libraries for runtime inference.
              By default the command installs the{' '}
              <strong>well-known default version</strong> of llama.cpp baked
              into the SDK; pass <code>--upgrade</code> to track the latest
              published release. The command auto-detects your system
              architecture (amd64/arm64), operating system
              (linux/darwin/windows), and processor type
              (cpu/metal/cuda/rocm/vulkan).
            </p>
            <h5>Hardware Backends</h5>
            <ul>
              <li><code>cpu</code> — CPU-only inference (works on all systems)</li>
              <li><code>metal</code> — Apple Silicon GPU acceleration (macOS)</li>
              <li><code>cuda</code> — NVIDIA GPU acceleration</li>
              <li><code>rocm</code> — AMD GPU acceleration</li>
              <li><code>vulkan</code> — Cross-platform GPU acceleration</li>
            </ul>
          </div>

          <div className="card" id="flags">
            <h3>Flags</h3>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Flag</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>--local</code></td>
                  <td>Run without the model server (direct download)</td>
                </tr>
                <tr>
                  <td><code>--upgrade</code></td>
                  <td>Track the latest llama.cpp release instead of the well-known default version (default: <code>false</code>)</td>
                </tr>
                <tr>
                  <td><code>--version &lt;string&gt;</code></td>
                  <td>Download a specific llama.cpp version instead of the default (e.g. <code>b5540</code>). See <a href="https://github.com/ggml-org/llama.cpp/releases" target="_blank" rel="noopener noreferrer">available releases</a>. An explicit version overrides both the default and <code>--upgrade</code>.</td>
                </tr>
                <tr>
                  <td><code>--install</code></td>
                  <td>Install for the supplied <code>--arch</code>/<code>--os</code>/<code>--processor</code> triple (lands in its own folder under the libraries root)</td>
                </tr>
                <tr>
                  <td><code>--arch &lt;string&gt;</code></td>
                  <td>Architecture for triple-aware install operations: <code>amd64</code>, <code>arm64</code></td>
                </tr>
                <tr>
                  <td><code>--os &lt;string&gt;</code></td>
                  <td>Operating system for triple-aware install operations: <code>linux</code>, <code>bookworm</code>, <code>trixie</code>, <code>darwin</code>, <code>windows</code></td>
                </tr>
                <tr>
                  <td><code>--processor &lt;string&gt;</code></td>
                  <td>Processor for triple-aware install operations: <code>cpu</code>, <code>cuda</code>, <code>metal</code>, <code>rocm</code>, <code>vulkan</code></td>
                </tr>
                <tr>
                  <td><code>--list-combinations</code></td>
                  <td>List supported (arch, os, processor) combinations and exit</td>
                </tr>
                <tr>
                  <td><code>--list-installs</code></td>
                  <td>List installed library bundles under the libraries root and exit</td>
                </tr>
                <tr>
                  <td><code>--remove-install</code></td>
                  <td>Remove the install matching <code>--arch</code>/<code>--os</code>/<code>--processor</code></td>
                </tr>
                <tr>
                  <td><code>--base-path &lt;string&gt;</code></td>
                  <td>Base path for kronk data (models, libraries, catalog, model_config) — persistent global flag</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="env">
            <h3>Environment Variables</h3>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Variable</th>
                  <th>Default</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>KRONK_TOKEN</code></td>
                  <td></td>
                  <td>Authentication token for the kronk server (required when auth enabled)</td>
                </tr>
                <tr>
                  <td><code>KRONK_WEB_API_HOST</code></td>
                  <td>localhost:11435</td>
                  <td>IP Address for the kronk server (web mode)</td>
                </tr>
                <tr>
                  <td><code>KRONK_BASE_PATH</code></td>
                  <td>$HOME/kronk</td>
                  <td>Base path for kronk data directories (local mode)</td>
                </tr>
                <tr>
                  <td><code>KRONK_ARCH</code></td>
                  <td>auto-detected</td>
                  <td>Architecture override (local mode): <code>amd64</code>, <code>arm64</code></td>
                </tr>
                <tr>
                  <td><code>KRONK_OS</code></td>
                  <td>auto-detected</td>
                  <td>Operating system override (local mode): <code>linux</code>, <code>darwin</code>, <code>windows</code></td>
                </tr>
                <tr>
                  <td><code>KRONK_PROCESSOR</code></td>
                  <td>auto-detected</td>
                  <td>Hardware backend override (local mode): <code>cpu</code>, <code>cuda</code>, <code>metal</code>, <code>rocm</code>, <code>vulkan</code></td>
                </tr>
                <tr>
                  <td><code>KRONK_LIB_PATH</code></td>
                  <td>$HOME/kronk/libraries</td>
                  <td>Library directory path (local mode). Set to switch between previously installed bundles.</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="examples">
            <h3>Examples</h3>
            <pre className="code-block">
              <code>{`# Install the well-known default version using the server
kronk libs

# Install the well-known default version locally
kronk libs --local

# Track and install the latest llama.cpp release
kronk libs --local --upgrade

# Install a specific version
kronk libs --local --version=b7406

# Install CUDA libraries explicitly via env override
KRONK_PROCESSOR=cuda kronk libs --local

# List supported (arch, os, processor) combinations
kronk libs --list-combinations

# Install a Linux/CUDA bundle alongside the active install
kronk libs --install --arch=amd64 --os=linux --processor=cuda

# List installed library bundles
kronk libs --list-installs

# Remove an install
kronk libs --remove-install --arch=amd64 --os=linux --processor=cuda

# Switch to a previously installed bundle
export KRONK_LIB_PATH=~/.kronk/libraries/linux/amd64/cuda`}</code>
            </pre>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#usage" className="doc-index-header">Usage</a>
            </div>
            <div className="doc-index-section">
              <a href="#flags" className="doc-index-header">Flags</a>
            </div>
            <div className="doc-index-section">
              <a href="#env" className="doc-index-header">Environment Variables</a>
            </div>
            <div className="doc-index-section">
              <a href="#examples" className="doc-index-header">Examples</a>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
