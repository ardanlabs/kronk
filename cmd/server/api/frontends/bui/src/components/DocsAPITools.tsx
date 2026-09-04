import type { ReactNode } from 'react';

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
  details?: ReactNode;
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
      { method: 'GET', path: '/v1/kronk/libs/integrity', description: 'Hash and verify the selected library bundle.', auth: 'Admin' },
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
      { method: 'GET', path: '/v1/kronk/models/integrity', description: 'List local artifact digests and persisted verification evidence without hashing model files.', auth: 'Admin' },
      { method: 'GET', path: '/v1/kronk/models/integrity/{model}', description: 'Return integrity information for one model without inspecting other models.', auth: 'Admin' },
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
    details: (
      <>
        <h4><code>GET /v1/kronk/models/integrity</code> response</h4>
        <p>
          Returns physical models from the local index, including every weight shard and optional projection and MTP artifacts.
          Configured extension aliases are not duplicated. The request reads only the index, sidecars, and filesystem metadata; it
          does not hash model contents.
        </p>
        <pre className="code-block"><code>{`{
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

        <h4><code>GET /v1/kronk/models/integrity/{'{model}'}</code> response</h4>
        <p>
          Use the targeted route when only one model is needed, especially on installations with a large model inventory. It inspects
          only that model and returns the model object directly, without the <code>object</code> and <code>data</code> list envelope. A
          missing model returns <code>404 Not Found</code>.
        </p>
        <pre className="code-block"><code>{`{
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

        <h4>Model fields</h4>
        <table className="flags-table">
          <thead><tr><th>Field</th><th>Meaning</th></tr></thead>
          <tbody>
            <tr><td><code>object</code></td><td>Always <code>model_integrity.list</code>.</td></tr>
            <tr><td><code>data</code></td><td>Physical models present in the local index.</td></tr>
            <tr><td><code>id</code></td><td>Kronk model ID from the local index.</td></tr>
            <tr><td><code>owned_by</code></td><td>Provider or organization directory containing the model.</td></tr>
            <tr><td><code>model_family</code></td><td>Model-family directory containing the artifacts.</td></tr>
            <tr><td><code>status</code></td><td>Least trustworthy artifact status: unavailable, stale, unverified, then verified.</td></tr>
            <tr><td><code>verified</code></td><td>True only when every required artifact is verified.</td></tr>
            <tr><td><code>artifacts</code></td><td>Weight shards followed by an optional projection and optional MTP drafter.</td></tr>
          </tbody>
        </table>

        <h4>Artifact fields</h4>
        <table className="flags-table">
          <thead><tr><th>Field</th><th>Meaning</th></tr></thead>
          <tbody>
            <tr><td><code>role</code></td><td><code>weights</code>, <code>projection</code>, or <code>mtp</code>. Every weight shard is separate.</td></tr>
            <tr><td><code>filename</code></td><td>Basename of the local artifact.</td></tr>
            <tr><td><code>digest</code></td><td>Hugging Face Git LFS identity as <code>sha256:&lt;hex&gt;</code>; omitted when unavailable.</td></tr>
            <tr><td><code>size</code></td><td>Current local size in bytes; zero when the indexed file is absent.</td></tr>
            <tr><td><code>status</code></td><td><code>verified</code>, <code>unverified</code>, <code>stale</code>, or <code>unavailable</code>.</td></tr>
            <tr><td><code>verified</code></td><td>Convenience boolean that is true only for status <code>verified</code>.</td></tr>
            <tr><td><code>verified_at</code></td><td>UTC time of the last successful full-file verification; omitted without a usable record.</td></tr>
            <tr><td><code>reason</code></td><td>When applicable, <code>digest_unavailable</code> or <code>file_metadata_changed</code>.</td></tr>
          </tbody>
        </table>

        <p>
          <strong>Status meaning:</strong> <code>verified</code> means the published digest, persisted verification record, and current
          size and modification time agree. <code>unverified</code> means a digest exists without a verification record. <code>stale</code>
          means verification evidence or file metadata changed. <code>unavailable</code> means the digest sidecar is absent or malformed.
          A verified result records an earlier full hash; this GET does not freshly prove the current bytes.
        </p>
      </>
    ),
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
      { method: 'GET', path: '/v1/bucky/libs/integrity', description: 'Hash and verify the selected library bundle.', auth: 'Admin' },
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
              {group.details}
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
