## ROADMAP

### BUGS / ISSUES

### MODEL SERVER / TOOLING

- Add tests to the tooling package
- Solidfy the auth system
  - CLI tooling to create tokens
  - Provide Auth at the endpoint level (completion/embeddings)
  - Rate limiting
- Apply OTEL Spans to critical areas beyond start/stop request
- Maintain stats at a model level

### API

- Implement the Charmbracelt Interface
  https://github.com/charmbracelet/fantasy/pull/92#issuecomment-3636479873

### FRONTEND

- Maybe a Kronk model server BUI
  - Need local DB, maybe duck or postgres lite (CGO considerations)
  - Show loaded models
  - Show model stats
  - Tools support
  - Create tokens (need admin user)
