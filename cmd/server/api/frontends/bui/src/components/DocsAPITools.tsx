type Method = 'GET' | 'POST' | 'DELETE';

type Endpoint = {
  method: Method;
  path: string;
  description: string;
  auth: 'Inference' | 'Admin';
};

type EndpointGroup = {
  id: string;
  title: string;
  description: string;
  endpoints: Endpoint[];
};

const endpointGroups: EndpointGroup[] = [
  {
    id: 'model-discovery',
    title: 'OpenAI Model Discovery',
    description: 'OpenAI-compatible model discovery for inference clients.',
    endpoints: [
      { method: 'GET', path: '/v1/models', description: 'List locally available models and configured model extensions.', auth: 'Inference' },
      { method: 'GET', path: '/v1/models/{model}', description: 'Retrieve one locally available model by ID.', auth: 'Inference' },
    ],
  },
  {
    id: 'kronk-libraries',
    title: 'Kronk Libraries',
    description: 'Manage llama.cpp runtime library bundles.',
    endpoints: [
      { method: 'GET', path: '/v1/kronk/libs', description: 'Show the active library installation and upgrade state.', auth: 'Admin' },
      { method: 'GET', path: '/v1/kronk/libs/combinations', description: 'List supported operating-system, architecture, and processor combinations.', auth: 'Admin' },
      { method: 'GET', path: '/v1/kronk/libs/installs', description: 'List installed library bundles.', auth: 'Admin' },
      { method: 'POST', path: '/v1/kronk/libs/pull', description: 'Install a library bundle and stream progress.', auth: 'Admin' },
      { method: 'DELETE', path: '/v1/kronk/libs/installs', description: 'Remove the bundle selected by arch, os, and processor query parameters.', auth: 'Admin' },
    ],
  },
  {
    id: 'kronk-models',
    title: 'Kronk Models',
    description: 'Manage local GGUF models, runtime configuration, and loaded instances.',
    endpoints: [
      { method: 'GET', path: '/v1/kronk/models', description: 'List locally installed GGUF models with Kronk metadata.', auth: 'Admin' },
      { method: 'GET', path: '/v1/kronk/models/', description: 'Return the missing-model validation error for an empty model ID.', auth: 'Admin' },
      { method: 'GET', path: '/v1/kronk/models/{model}', description: 'Show metadata and effective configuration for one model.', auth: 'Admin' },
      { method: 'GET', path: '/v1/kronk/models/ps', description: 'List models currently loaded in the pool.', auth: 'Admin' },
      { method: 'GET', path: '/v1/kronk/models/imc-sessions', description: 'List active incremental-message-cache sessions.', auth: 'Admin' },
      { method: 'POST', path: '/v1/kronk/models/index', description: 'Rebuild the local model index.', auth: 'Admin' },
      { method: 'POST', path: '/v1/kronk/models/pull', description: 'Download a model and optional companion files and stream progress.', auth: 'Admin' },
      { method: 'POST', path: '/v1/kronk/models/autotune', description: 'Resolve an automatically tuned runtime configuration.', auth: 'Admin' },
      { method: 'POST', path: '/v1/kronk/models/vram', description: 'Estimate model memory use for requested runtime settings.', auth: 'Admin' },
      { method: 'POST', path: '/v1/kronk/models/unload', description: 'Unload a model or playground instance from the pool.', auth: 'Admin' },
      { method: 'DELETE', path: '/v1/kronk/models/{model}', description: 'Remove a locally installed model.', auth: 'Admin' },
    ],
  },
  {
    id: 'kronk-catalog',
    title: 'Kronk Catalog',
    description: 'Browse, resolve, reconcile, and remove personal catalog entries.',
    endpoints: [
      { method: 'GET', path: '/v1/kronk/catalog', description: 'List catalog entries and local validation state.', auth: 'Admin' },
      { method: 'GET', path: '/v1/kronk/catalog/{id...}', description: 'Show one catalog entry; the ID may contain slashes.', auth: 'Admin' },
      { method: 'POST', path: '/v1/kronk/catalog/lookup', description: 'List GGUF files for a HuggingFace repository or URL.', auth: 'Admin' },
      { method: 'POST', path: '/v1/kronk/catalog/resolve', description: 'Resolve a source to canonical download files without downloading.', auth: 'Admin' },
      { method: 'POST', path: '/v1/kronk/catalog/reconcile', description: 'Reconcile catalog metadata with locally installed models.', auth: 'Admin' },
      { method: 'DELETE', path: '/v1/kronk/catalog/{id...}', description: 'Remove an entry and its downloaded and cached files.', auth: 'Admin' },
    ],
  },
  {
    id: 'bucky-libraries',
    title: 'Bucky Libraries',
    description: 'Manage whisper.cpp runtime library bundles.',
    endpoints: [
      { method: 'GET', path: '/v1/bucky/libs', description: 'Show the active library installation.', auth: 'Admin' },
      { method: 'GET', path: '/v1/bucky/libs/combinations', description: 'List supported platform combinations.', auth: 'Admin' },
      { method: 'GET', path: '/v1/bucky/libs/installs', description: 'List installed library bundles.', auth: 'Admin' },
      { method: 'POST', path: '/v1/bucky/libs/pull', description: 'Install a library bundle and stream progress.', auth: 'Admin' },
      { method: 'DELETE', path: '/v1/bucky/libs/installs', description: 'Remove the selected library bundle.', auth: 'Admin' },
    ],
  },
  {
    id: 'bucky-models',
    title: 'Bucky Models',
    description: 'Manage local whisper models and the bundled short-name catalog.',
    endpoints: [
      { method: 'GET', path: '/v1/bucky/models', description: 'List installed whisper models.', auth: 'Admin' },
      { method: 'GET', path: '/v1/bucky/models/catalog', description: 'List the bundled short-name model catalog.', auth: 'Admin' },
      { method: 'POST', path: '/v1/bucky/models/pull', description: 'Download a whisper model and stream progress.', auth: 'Admin' },
      { method: 'GET', path: '/v1/bucky/models/{model}/details', description: 'Show model header and file details.', auth: 'Admin' },
      { method: 'DELETE', path: '/v1/bucky/models/{model}', description: 'Remove an installed whisper model.', auth: 'Admin' },
    ],
  },
  {
    id: 'operations',
    title: 'Operations',
    description: 'Inspect resources, hardware, and server diagnostics.',
    endpoints: [
      { method: 'GET', path: '/v1/pool/budget', description: 'Report memory budgets and current reservations.', auth: 'Admin' },
      { method: 'GET', path: '/v1/devices', description: 'List detected compute devices.', auth: 'Admin' },
      { method: 'GET', path: '/v1/diagnose', description: 'Return the JSON diagnostic report; bench=true includes a benchmark.', auth: 'Admin' },
    ],
  },
  {
    id: 'evaluation',
    title: 'Evaluation',
    description: 'Run the BUI code-recall and throughput evaluations.',
    endpoints: [
      { method: 'GET', path: '/v1/accuracy/functions', description: 'List functions available to the code-recall test.', auth: 'Admin' },
      { method: 'POST', path: '/v1/accuracy/test', description: 'Run one code-recall comparison using model and function.', auth: 'Admin' },
      { method: 'POST', path: '/v1/efficiency/run', description: 'Warm a model, run one prompt, and report throughput.', auth: 'Admin' },
    ],
  },
  {
    id: 'security',
    title: 'Security',
    description: 'Create tokens and manage authentication signing keys.',
    endpoints: [
      { method: 'POST', path: '/v1/security/token/create', description: 'Create a token with grants, quotas, and optional administrator status.', auth: 'Admin' },
      { method: 'GET', path: '/v1/security/keys', description: 'List signing keys.', auth: 'Admin' },
      { method: 'POST', path: '/v1/security/keys/add', description: 'Create a signing key.', auth: 'Admin' },
      { method: 'POST', path: '/v1/security/keys/remove/{keyid}', description: 'Remove a non-master signing key and revoke its tokens.', auth: 'Admin' },
    ],
  },
];

function methodClass(method: Method) {
  return `method-${method.toLowerCase()}`;
}

export default function DocsAPITools() {
  return (
    <div>
      <div className="page-header">
        <h2>Tools API</h2>
        <p>Complete reference for model discovery, administration, diagnostics, evaluation, and security endpoints.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="overview">
            <h3>Overview</h3>
            <p>Paths below include the complete <code>/v1</code> prefix. The default base URL is <code>http://localhost:11435</code>.</p>
            <p><strong>Inference</strong> routes require a valid user or administrator token only when full authentication is enabled. They do not require a separate model-discovery grant.</p>
            <p><strong>Admin</strong> routes require an administrator token when administration authentication is enabled and are open otherwise.</p>
            <pre className="code-block">
              <code>Authorization: Bearer YOUR_TOKEN</code>
            </pre>
            <p>Endpoints that accept request bodies use <code>application/json</code>. Library and model pulls stream progress. See Manual Chapter 9 for request notes and links to the security and Bucky guides.</p>
          </div>

          {endpointGroups.map((group) => (
            <div className="card" id={group.id} key={group.id}>
              <h3>{group.title}</h3>
              <p>{group.description}</p>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Method</th>
                    <th>Path</th>
                    <th>Authentication</th>
                    <th>Purpose</th>
                  </tr>
                </thead>
                <tbody>
                  {group.endpoints.map((endpoint) => (
                    <tr key={`${endpoint.method}-${endpoint.path}`}>
                      <td><span className={methodClass(endpoint.method)}>{endpoint.method}</span></td>
                      <td><code>{endpoint.path}</code></td>
                      <td>{endpoint.auth}</td>
                      <td>{endpoint.description}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))}
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#overview" className="doc-index-header">Overview</a>
            </div>
            {endpointGroups.map((group) => (
              <div className="doc-index-section" key={group.id}>
                <a href={`#${group.id}`} className="doc-index-header">{group.title}</a>
              </div>
            ))}
          </div>
        </nav>
      </div>
    </div>
  );
}
