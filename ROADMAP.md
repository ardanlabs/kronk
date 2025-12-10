## ROADMAP

### MODEL SERVER / TOOLING

- Add and finish the PS command
- There is a setting that allows the model to return multiple
  tool calls. Parallel bool json:"parallel"
- Solidfy the auth system
  - CLI tooling to create tokens
  - Provide Auth at the endpoint level (completion/embeddings)
  - Rate limiting
- Apply OTEL Spans to critical areas beyond start/stop request
- Maintain stats at a model level

### FRONTEND

- Maybe a Kronk model server BUI
  - Need local DB, maybe duck or postgres lite (CGO considerations)
  - Show loaded models
  - Show model stats
  - Tools support
  - Create tokens (need admin user)
