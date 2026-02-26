# Chapter 11: Security & Authentication

## Table of Contents

- [11.1 Enabling Authentication](#111-enabling-authentication)
- [11.2 Using the Admin Token](#112-using-the-admin-token)
- [11.3 Key Management](#113-key-management)
- [11.4 Creating User Tokens](#114-creating-user-tokens)
- [11.5 Token Examples](#115-token-examples)
- [11.6 Using Tokens in API Requests](#116-using-tokens-in-api-requests)
- [11.7 Authorization Flow](#117-authorization-flow)
- [11.8 Rate Limiting](#118-rate-limiting)
- [11.9 Configuration Reference](#119-configuration-reference)
- [11.10 Security Best Practices](#1110-security-best-practices)

---



Kronk provides JWT-based authentication and authorization with per-endpoint
rate limiting. When enabled, all API requests require a valid token.

### 11.1 Enabling Authentication

**Start Server with Auth Enabled:**

```shell
kronk server start --auth-enabled
```

Or via environment variable:

```shell
export KRONK_AUTH_ENABLED=true
kronk server start
```

**First-Time Setup:**

On first startup with authentication enabled, Kronk automatically:

1. Creates a `keys/` directory in `~/.kronk/`
2. Generates a master private key (`master.pem`)
3. Creates an admin token (`master.jwt`) valid for 10 years
4. Generates an additional signing key for user tokens

The admin token is stored at `~/.kronk/keys/master.jwt`.

### 11.2 Using the Admin Token

The admin token is required for all security management operations.

**Set the Token:**

```shell
export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)
```

**Admin Capabilities:**

- Create new tokens for users
- Add and remove signing keys
- Access all endpoints without rate limits

### 11.3 Key Management

Private keys sign JWT tokens. Multiple keys allow token rotation without
invalidating all existing tokens.

**List Keys:**

```shell
kronk security key list
```

Output:

```
KEY ID                                  CREATED
master                                  2024-01-15T10:30:00Z
a1b2c3d4-e5f6-7890-abcd-ef1234567890    2024-01-15T10:30:00Z
```

**Create a New Key:**

```shell
kronk security key create
```

This generates a new UUID-named key for signing tokens.

**Delete a Key:**

```shell
kronk security key delete --keyid a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

**Important:** The master key cannot be deleted. Deleting a key invalidates
all tokens signed with that key.

**Local Mode:**

All key commands support `--local` to operate without a running server:

```shell
kronk security key list --local
kronk security key create --local
kronk security key delete --keyid <keyid> --local
```

### 11.4 Creating User Tokens

Create tokens with specific endpoint access and optional rate limits.

**Basic Syntax:**

```shell
kronk security token create \
  --duration <duration> \
  --endpoints <endpoint-list>
```

**Parameters:**

- `--duration` - Token lifetime (e.g., `1h`, `24h`, `720h`, `8760h`)
- `--endpoints` - Comma-separated list of endpoints with optional limits

**Endpoint Format:**

- `endpoint` - Unlimited access (default)
- `endpoint:unlimited` - Unlimited access (explicit)
- `endpoint:limit/window` - Rate limited

**Rate Limit Windows:**

- `day` - Resets daily
- `month` - Resets monthly
- `year` - Resets yearly

**Available Endpoints:**

- `chat-completions` - Chat completions API
- `responses` - Responses API
- `embeddings` - Embeddings API
- `rerank` - Reranking API
- `messages` - Anthropic Messages API

### 11.5 Token Examples

**Unlimited Access to All Endpoints (24 hours):**

```shell
kronk security token create \
  --duration 24h \
  --endpoints chat-completions,embeddings,rerank,responses,messages
```

**Rate-Limited Chat Token (30 days):**

```shell
kronk security token create \
  --duration 720h \
  --endpoints "chat-completions:1000/day,embeddings:500/day"
```

**Monthly Quota Token:**

```shell
kronk security token create \
  --duration 8760h \
  --endpoints "chat-completions:10000/month,embeddings:50000/month"
```

**Mixed Limits:**

```shell
kronk security token create \
  --duration 720h \
  --endpoints "chat-completions:100/day,embeddings:unlimited"
```

**Output:**

```
Token create
  Duration: 720h0m0s
  Endpoints: map[chat-completions:{1000 day} embeddings:{0 unlimited}]
TOKEN:
eyJhbGciOiJSUzI1NiIsImtpZCI6ImExYjJjM2Q0Li4uIiwidHlwIjoiSldUIn0...
```

### 11.6 Using Tokens in API Requests

Pass the token in the `Authorization` header with the `Bearer` prefix.

**curl Example:**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer eyJhbGciOiJS..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

**Environment Variable Pattern:**

```shell
export KRONK_TOKEN="eyJhbGciOiJS..."

curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{...}'
```

**Python Example:**

```python
import openai

client = openai.OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="eyJhbGciOiJS..."  # Your Kronk token
)

response = client.chat.completions.create(
    model="Qwen3-8B-Q8_0",
    messages=[{"role": "user", "content": "Hello"}]
)
```

### 11.7 Authorization Flow

When a request arrives:

1. **Token Extraction** - Bearer token parsed from Authorization header
2. **Signature Verification** - Token signature verified against known keys
3. **Expiration Check** - Token must not be expired
4. **Endpoint Authorization** - Token must include the requested endpoint
5. **Rate Limit Check** - Request counted against endpoint quota
6. **Request Processing** - If all checks pass, request proceeds

**Error Responses:**

- `401 Unauthorized` - Missing, invalid, or expired token
- `403 Forbidden` - Token lacks access to the endpoint
- `429 Too Many Requests` - Rate limit exceeded

### 11.8 Rate Limiting

Rate limits are enforced per token (identified by the token's subject claim).

**How Limits Work:**

- Each token has a unique subject (UUID)
- Requests are counted per endpoint per subject
- Counters reset at window boundaries (day/month/year)

**Limit Storage:**

Rate limit counters are stored in a BadgerDB database at `~/.kronk/badger/`.
Counters persist across server restarts.

**Bypassing Rate Limits:**

Admin tokens (like `master.jwt`) bypass all rate limiting.

### 11.9 Configuration Reference

**Server Flags:**

- `--auth-enabled` - Enable authentication (env: `KRONK_AUTH_ENABLED`)
- `--auth-issuer` - JWT issuer name (env: `KRONK_AUTH_ISSUER`)
- `--auth-host` - External auth service host (env: `KRONK_AUTH_HOST`)

**Environment Variables:**

- `KRONK_TOKEN` - Token for CLI commands and API requests
- `KRONK_WEB_API_HOST` - Server address for CLI web mode
  (default: `localhost:8080`)

### 11.10 Security Best Practices

**Token Management:**

- Store admin tokens securely; treat `master.jwt` like a password
- Create separate tokens for different applications/users
- Use short durations for development tokens
- Rotate keys periodically for production deployments

**Rate Limiting:**

- Set appropriate limits based on expected usage
- Use daily limits for interactive applications
- Use monthly limits for batch processing

**Key Rotation:**

1. Create a new key: `kronk security key create`
2. Issue new tokens using the new key
3. Wait for old tokens to expire
4. Delete the old key: `kronk security key delete --keyid <old-keyid>`

**Production Checklist:**

- Enable authentication: `--auth-enabled`
- Secure the `~/.kronk/keys/` directory (mode 0700)
- Back up `master.pem` and `master.jwt` securely
- Distribute user tokens, never the admin token
- Monitor rate limit usage in logs

---

_Next: [Chapter 12: Browser UI (BUI)](#chapter-12-browser-ui-bui)_
