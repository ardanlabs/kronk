import { Link, useParams } from 'react-router-dom';
import reference from './cli-reference.json';

export default function DocsCLI() {
  const { '*': group = '' } = useParams();
  const path = group ? `kronk ${group}` : 'kronk';
  const command = reference.find((entry) => entry.path === path);
  const entries = reference.filter((entry) => entry.path === path || (group && entry.path.startsWith(`${path} `)));
  const anchor = (entryPath: string) => entryPath === path ? 'usage' : `cmd-${entryPath.slice(path.length + 1).replaceAll(' ', '-')}`;

  if (!command) {
    return <div className="card"><h2>Command not found</h2><Link to="/docs/cli">CLI reference</Link></div>;
  }

  return (
    <div>
      <div className="page-header">
        <h2>{command.path}</h2>
        <p>{command.short}</p>
      </div>
      <p className="doc-description">
        Generated from the CLI command definitions. Defaults below are flag defaults;
        environment variables and server configuration may supply effective values.
        See the <Link to="/docs/manual">manual</Link> for operational workflows and environment settings.
      </p>
      <div className="doc-layout">
        <div className="doc-content">
          {!group && (
            <div className="card">
              <h3>Commands</h3>
              <ul>
                {reference.filter((entry) => entry.path === `kronk ${entry.name}`).map((entry) => (
                  <li key={entry.path}><Link to={`/docs/cli/${entry.name}`}>{entry.name}</Link> — {entry.short}</li>
                ))}
              </ul>
            </div>
          )}
          {entries.map((entry) => (
            <div className="card" id={anchor(entry.path)} key={entry.path}>
              <h3>{entry.path}</h3>
              <p className="doc-description" style={{ whiteSpace: 'pre-wrap' }}>{entry.description || entry.short}</p>
              <h4>Usage</h4>
              <pre className="code-block"><code>{entry.usage}</code></pre>
              {entry.example && <><h4>Examples</h4><pre className="code-block"><code>{entry.example}</code></pre></>}
              {entry.flags.length > 0 && (
                <>
                  <h4>Flags</h4>
                  <div className="table-container" tabIndex={0} role="region" aria-label={`${entry.path} flags`}>
                    <table className="flags-table" style={{ minWidth: 560 }}>
                      <thead><tr><th>Flag</th><th>Type</th><th>Default</th><th>Description</th></tr></thead>
                      <tbody>
                        {entry.flags.map((flag) => (
                          <tr key={flag.name}>
                            <td><code style={{ whiteSpace: 'nowrap' }}>{flag.shorthand ? `-${flag.shorthand}, ` : ''}--{flag.name}</code>{flag.inherited && <small> (inherited)</small>}</td>
                            <td><code>{flag.type}</code></td>
                            <td><code>{flag.default === '' ? '""' : flag.default}</code></td>
                            <td>{flag.usage}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </>
              )}
            </div>
          ))}
        </div>
        <nav className="doc-sidebar" aria-label="CLI command index">
          <div className="doc-sidebar-content">
            {entries.map((entry) => (
              <div className="doc-index-section" key={entry.path}>
                <a href={`#${anchor(entry.path)}`} className="doc-index-header">{entry.path}</a>
              </div>
            ))}
          </div>
        </nav>
      </div>
    </div>
  );
}
