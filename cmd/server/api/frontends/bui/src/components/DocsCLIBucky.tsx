export default function DocsCLIBucky() {
  return (
    <div>
      <div className="page-header">
        <h2>bucky</h2>
        <p>Whisper (whisper.cpp) backend: libs and model management.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk bucky &lt;command&gt; [flags]</code>
            </pre>
            <p>
              The <code>bucky</code> sub-command tree targets the whisper.cpp
              runtime (audio transcription). It mirrors the top-level llama
              verbs: use it to install the whisper shared libraries and to
              download / manage whisper GGML models. Whisper has no chat or
              generation surface, so there is no <code>bucky run</code> verb.
            </p>
            <p>
              Every bucky verb accepts a <code>--local</code> flag. The default
              web mode talks to the model server's{' '}
              <code>/v1/bucky/libs/...</code> and{' '}
              <code>/v1/bucky/models/...</code> endpoints; local mode runs
              in-process without a server.
            </p>
            <h5>Commands</h5>
            <ul>
              <li><code>libs</code> — install or upgrade whisper.cpp libraries</li>
              <li><code>model catalog</code> — list the bundled catalog of well-known whisper models</li>
              <li><code>model list</code> — list installed whisper models</li>
              <li><code>model pull</code> — download a whisper model by short name or URL</li>
              <li><code>model remove</code> — remove an installed whisper model from disk</li>
            </ul>
          </div>

          <div className="card" id="cmd-libs">
            <h3>libs — Install or upgrade whisper.cpp libraries</h3>
            <pre className="code-block">
              <code>kronk bucky libs [flags]</code>
            </pre>
            <p className="doc-description">
              Downloads and installs the whisper.cpp library bundle for your
              hardware platform under the bucky libraries root (default:{' '}
              <code>~/.kronk/bucky-libraries/</code>). Auto-detects
              architecture (amd64/arm64), OS (linux/darwin/windows), and
              processor (cpu/cuda/metal/vulkan).
            </p>
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
                  <td>Track the latest whisper.cpp release instead of the well-known default version (default: <code>false</code>)</td>
                </tr>
                <tr>
                  <td><code>--version &lt;string&gt;</code></td>
                  <td>Download a specific whisper.cpp version instead of the default (e.g. <code>v1.9.1</code>)</td>
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
                  <td>Operating system for triple-aware install operations: <code>linux</code>, <code>darwin</code>, <code>windows</code></td>
                </tr>
                <tr>
                  <td><code>--processor &lt;string&gt;</code></td>
                  <td>Processor for triple-aware install operations: <code>cpu</code>, <code>cuda</code>, <code>metal</code>, <code>vulkan</code></td>
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
            <h5>Example</h5>
            <pre className="code-block">
              <code>{`# Install the default whisper.cpp libraries for the current host
kronk bucky libs

# Install a Linux/CUDA bundle alongside the active install
kronk bucky libs --install --arch=amd64 --os=linux --processor=cuda

# List installed library bundles
kronk bucky libs --list-installs`}</code>
            </pre>
          </div>

          <div className="card" id="cmd-model">
            <h3>model — Manage local whisper models</h3>
            <pre className="code-block">
              <code>kronk bucky model &lt;command&gt; [flags]</code>
            </pre>
            <p>
              Whisper models are single <code>.bin</code> files stored flat
              under the bucky models root (default:{' '}
              <code>~/.kronk/bucky-models/</code>). The short name strips the{' '}
              <code>ggml-</code> prefix and <code>.bin</code> suffix.
            </p>

            <div className="doc-section" id="cmd-model-catalog">
              <h4>catalog</h4>
              <p className="doc-description">List the bundled catalog of well-known whisper models (short name, size, download URL).</p>
              <pre className="code-block">
                <code>kronk bucky model catalog [flags]</code>
              </pre>
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
                    <td>Run without the model server</td>
                  </tr>
                  <tr>
                    <td><code>--base-path &lt;string&gt;</code></td>
                    <td>Base path for kronk data — persistent global flag</td>
                  </tr>
                </tbody>
              </table>
            </div>

            <div className="doc-section" id="cmd-model-list">
              <h4>list</h4>
              <p className="doc-description">List installed whisper models found under the bucky models root.</p>
              <pre className="code-block">
                <code>kronk bucky model list [flags]</code>
              </pre>
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
                    <td>Run without the model server</td>
                  </tr>
                  <tr>
                    <td><code>--base-path &lt;string&gt;</code></td>
                    <td>Base path for kronk data — persistent global flag</td>
                  </tr>
                </tbody>
              </table>
            </div>

            <div className="doc-section" id="cmd-model-pull">
              <h4>pull</h4>
              <p className="doc-description">
                Download a whisper model from the bundled catalog or a URL. The
                argument may be a short catalog name (<code>tiny</code>,{' '}
                <code>base.en</code>, <code>large-v3</code>), a full ggml
                filename (<code>ggml-tiny.bin</code>), or a fully qualified URL.
              </p>
              <pre className="code-block">
                <code>kronk bucky model pull &lt;MODEL&gt; [flags]</code>
              </pre>
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
                    <td><code>--base-path &lt;string&gt;</code></td>
                    <td>Base path for kronk data — persistent global flag</td>
                  </tr>
                </tbody>
              </table>
            </div>

            <div className="doc-section" id="cmd-model-remove">
              <h4>remove</h4>
              <p className="doc-description">
                Remove an installed whisper model from disk. The argument is the
                short name, the full ggml filename, or the bare basename.
              </p>
              <pre className="code-block">
                <code>kronk bucky model remove &lt;MODEL&gt; [flags]</code>
              </pre>
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
                    <td>Run without the model server</td>
                  </tr>
                  <tr>
                    <td><code>--base-path &lt;string&gt;</code></td>
                    <td>Base path for kronk data — persistent global flag</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# List the bundled catalog
kronk bucky model catalog

# Download the tiny English whisper model
kronk bucky model pull ggml-tiny.bin

# List installed models
kronk bucky model list

# Remove a model
kronk bucky model remove ggml-tiny.bin`}</code>
              </pre>
            </div>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#usage" className="doc-index-header">Usage</a>
            </div>
            <div className="doc-index-section">
              <a href="#cmd-libs" className="doc-index-header">libs</a>
            </div>
            <div className="doc-index-section">
              <a href="#cmd-model" className="doc-index-header">model</a>
              <ul>
                <li><a href="#cmd-model-catalog">catalog</a></li>
                <li><a href="#cmd-model-list">list</a></li>
                <li><a href="#cmd-model-pull">pull</a></li>
                <li><a href="#cmd-model-remove">remove</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
