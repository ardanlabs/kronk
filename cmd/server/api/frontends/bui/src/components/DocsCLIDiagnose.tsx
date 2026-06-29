export default function DocsCLIDiagnose() {
  return (
    <div>
      <div className="page-header">
        <h2>diagnose</h2>
        <p>Inspect the host environment for debugging.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk diagnose [flags]</code>
            </pre>
            <p>
              Inspect the host environment and report information useful for
              debugging "Kronk doesn't work" or "the model is slow" problems.
              The report includes component versions (Kronk, yzma), host /
              hardware details, the installed llama.cpp builds and the compute
              devices each one sees, actionable hints, and a small{' '}
              <code>llama-bench</code> run.
            </p>
            <p>
              By default this command is <strong>inspect-only and never
              downloads</strong>: it reports on the llama.cpp libraries and
              model already installed. Use <code>--install</code> to download
              anything missing.
            </p>
            <h5>Report Sections</h5>
            <ul>
              <li><strong>Versions</strong> — Kronk and yzma versions</li>
              <li><strong>System</strong> — OS, arch, CPU, RAM, and raw host command output</li>
              <li><strong>llama.cpp</strong> — each installed backend (cpu/cuda/rocm/vulkan/metal) and the devices it detects</li>
              <li><strong>Hints</strong> — detected problems with a one-line remedy (e.g. GPU render nodes not accessible on Linux)</li>
              <li><strong>Benchmark</strong> — <code>llama-bench</code> throughput (prompt-processing and token-generation), unless skipped</li>
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
                  <td><code>--format &lt;string&gt;</code></td>
                  <td>Output format: <code>text</code>, <code>json</code>, or <code>yaml</code> (default: <code>text</code>)</td>
                </tr>
                <tr>
                  <td><code>--install</code></td>
                  <td>Download missing llama.cpp libraries and the benchmark model (default: <code>false</code>)</td>
                </tr>
                <tr>
                  <td><code>--no-bench</code></td>
                  <td>Skip the <code>llama-bench</code> step (the slowest part)</td>
                </tr>
                <tr>
                  <td><code>--model &lt;string&gt;</code></td>
                  <td>Model source or local <code>.gguf</code> path to benchmark (e.g. <code>unsloth/Qwen3-8B-Q8_0</code>)</td>
                </tr>
                <tr>
                  <td><code>--processor &lt;string&gt;</code></td>
                  <td>Processor to benchmark: <code>cpu</code>, <code>cuda</code>, <code>metal</code>, or <code>vulkan</code> (default: <code>KRONK_PROCESSOR</code> or auto-detect). Affects the benchmark only; the engine section always reflects the real server. With <code>cpu</code> the benchmark runs CPU-only even from a GPU library bundle.</td>
                </tr>
                <tr>
                  <td><code>--base-path &lt;string&gt;</code></td>
                  <td>Base path for kronk data (models, libraries, catalog, model_config) — persistent global flag</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="examples">
            <h3>Examples</h3>
            <pre className="code-block">
              <code>{`# Human-readable report (default, inspect-only)
kronk diagnose

# Download missing llama.cpp libraries / benchmark model, then report
kronk diagnose --install

# Machine-readable report (paste into a bug report)
kronk diagnose --format json
kronk diagnose --format yaml

# Skip the benchmark (faster)
kronk diagnose --no-bench

# Benchmark a specific model (source or local .gguf path)
kronk diagnose --model unsloth/Qwen3-8B-Q8_0

# Benchmark on a specific processor (e.g. force CPU on a GPU machine)
kronk diagnose --processor cpu`}</code>
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
              <a href="#examples" className="doc-index-header">Examples</a>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
