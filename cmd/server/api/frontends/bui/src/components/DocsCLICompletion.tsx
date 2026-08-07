export default function DocsCLICompletion() {
  return (
    <div>
      <div className="page-header">
        <h2>completion</h2>
        <p>Generate shell-completion scripts for the Kronk CLI.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk completion &lt;bash|fish|powershell|zsh&gt;</code>
            </pre>
          </div>

          <div className="card" id="shells">
            <h3>Shells</h3>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Command</th>
                  <th>Install for future sessions</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>kronk completion bash</code></td>
                  <td><code>kronk completion bash &gt; /etc/bash_completion.d/kronk</code> on Linux, or write to <code>$(brew --prefix)/etc/bash_completion.d/kronk</code> on macOS</td>
                </tr>
                <tr>
                  <td><code>kronk completion fish</code></td>
                  <td><code>kronk completion fish &gt; ~/.config/fish/completions/kronk.fish</code></td>
                </tr>
                <tr>
                  <td><code>kronk completion powershell</code></td>
                  <td>Add the generated script to the PowerShell profile</td>
                </tr>
                <tr>
                  <td><code>kronk completion zsh</code></td>
                  <td><code>kronk completion zsh &gt; &quot;$&#123;fpath[1]&#125;/_kronk&quot;</code> on Linux, or write to the Homebrew zsh site-functions directory on macOS</td>
                </tr>
              </tbody>
            </table>
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
                  <td><code>--no-descriptions</code></td>
                  <td>Omit command and flag descriptions from the generated completion script</td>
                </tr>
                <tr>
                  <td><code>--base-path &lt;string&gt;</code></td>
                  <td>Base path for Kronk data — persistent global flag</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="examples">
            <h3>Current-session Examples</h3>
            <pre className="code-block">
              <code>{`# Bash
source <(kronk completion bash)

# Fish
kronk completion fish | source

# PowerShell
kronk completion powershell | Out-String | Invoke-Expression

# Zsh
source <(kronk completion zsh)`}</code>
            </pre>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section"><a href="#usage" className="doc-index-header">Usage</a></div>
            <div className="doc-index-section"><a href="#shells" className="doc-index-header">Shells</a></div>
            <div className="doc-index-section"><a href="#flags" className="doc-index-header">Flags</a></div>
            <div className="doc-index-section"><a href="#examples" className="doc-index-header">Examples</a></div>
          </div>
        </nav>
      </div>
    </div>
  );
}
