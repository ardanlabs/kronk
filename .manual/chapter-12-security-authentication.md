# Chapter 12: Security and Authentication

## Table of Contents

- [12.1 Authorization Modes](#121-authorization-modes)
- [12.2 Initial Credentials](#122-initial-credentials)
- [12.3 Admin and User Tokens](#123-admin-and-user-tokens)
- [12.4 Endpoint Grants and Rate Limits](#124-endpoint-grants-and-rate-limits)
- [12.5 Using a Token](#125-using-a-token)
- [12.6 Key Rotation and Revocation](#126-key-rotation-and-revocation)
- [12.7 Embedded and Standalone Authentication](#127-embedded-and-standalone-authentication)
- [12.8 Production Hardening](#128-production-hardening)

---

Kronk signs JWT bearer tokens with local RSA keys. A token can be an
unrestricted administrator credential or a user credential limited to
specific inference endpoints and request quotas.

## 12.1 Authorization Modes

`KRONK_AUTHORIZATION_MODE` selects the API access policy independently of
whether Kronk uses its embedded auth service or `KRONK_AUTH_HOST`:

| Value | Model discovery | Inference | Management |
| ----- | --------------- | --------- | ---------- |
| `open` | Public | Public | Public |
| `management` | Public | Public | Admin JWT |
| `authenticated` | Valid JWT | Valid JWT without an endpoint grant | Admin JWT |
| `full-protected` | Valid JWT | JWT with the matching endpoint grant | Admin JWT |

Health endpoints remain public in every mode. Model discovery consists of
`GET /v1/models` and `GET /v1/models/{model}`. Native Kronk model, integrity,
catalog, device, diagnostic, pool, playground, and security endpoints are
management APIs.

For example:

```shell
export KRONK_AUTHORIZATION_MODE=full-protected
kronk server start
```

When `KRONK_AUTHORIZATION_MODE` is set, it overrides the legacy
`KRONK_AUTH_LOCAL_ENABLED` and `KRONK_AUTH_ADMIN_ENABLED` settings. When the new
mode is unset, those settings retain their existing behavior for compatibility.
This allows deployments to migrate before the legacy settings are deprecated.

`KRONK_AUTH_HOST` selects the authentication provider; it does not change the
rights in this table. `KRONK_WEB_ADMIN_ENABLED` independently controls whether
the BUI is served. When using an external auth host with a mode that protects
management, disable the BUI because its password login uses the embedded
security store.

## 12.2 Initial Credentials

When the embedded security store initializes for the first time, Kronk creates:

- `~/.kronk/keys/master.pem`, the master private key;
- `~/.kronk/keys/master.jwt`, an admin token valid for ten years; and
- an additional UUID-named signing key used for subsequently created tokens.

This initialization occurs even in open mode when Kronk starts its embedded
auth service. The key directory is set to mode `0700`, and private keys and the
master token are set to `0600`.

Treat both master files as recovery credentials. Keep secure backups and never
distribute them to applications. Changing the configured JWT issuer causes
existing tokens to fail issuer validation.

## 12.3 Admin and User Tokens

Load the initial admin token for CLI administration:

```shell
export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)
```

Admin tokens bypass endpoint grants and rate limits. The security CLI requires
`KRONK_TOKEN` for key and token commands. In protected modes, the server also
verifies that it is an admin token.

Create a short-lived application token with exact endpoint grants:

```shell
kronk security token create \
  --duration 24h \
  --endpoints chat-completions,responses,messages
```

`--duration` uses Go duration syntax such as `1h`, `24h`, or `720h`; it does
not accept `30d`. Every generated token receives a unique subject UUID, so
authorization and quotas are per token rather than per named human or account.

## 12.4 Endpoint Grants and Rate Limits

The grant names used by inference middleware are:

| Grant | Endpoint |
| ----- | -------- |
| `chat-completions` | `POST /v1/chat/completions` |
| `responses` | `POST /v1/responses` |
| `messages` | `POST /v1/messages` |
| `embeddings` | `POST /v1/embeddings` |
| `rerank` | `POST /v1/rerank` and `/v1/reranking` |
| `tokenize` | `POST /v1/tokenize` |
| `transcriptions` | `POST /v1/audio/transcriptions` |

Grant names are not validated when a token is created. Use the names above
exactly; a typo produces a valid token with an unusable grant.

Each `--endpoints` entry has one of these forms:

```text
endpoint
endpoint:unlimited
endpoint:limit/window
```

An entry without a suffix is unlimited. Rate windows are `day`, `month`, and
`year`, measured at UTC calendar boundaries. For example:

```shell
kronk security token create \
  --duration 720h \
  --endpoints "chat-completions:1000/day,embeddings:500/month,responses:unlimited"
```

Kronk counts admitted requests by token subject and endpoint. Counters are
stored in `~/.kronk/badger/`, survive server restarts, and expire after their
current window. Admin tokens do not use these counters.

## 12.5 Using a Token

Send a token using the bearer authorization scheme:

```shell
export KRONK_TOKEN="<application-token>"

curl http://localhost:11435/v1/chat/completions \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen/Qwen3-8B-Q8_0",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

Kronk verifies the signature, issuer, expiration, required admin status or
endpoint grant, and quota before processing a protected request. Authentication,
missing-grant, and exhausted-quota failures return `401 Unauthorized`,
`403 Forbidden`, and `429 Too Many Requests`, respectively. Authentication
service failures return `500 Internal Server Error`, while unavailable external
auth services return `503 Service Unavailable`.

## 12.6 Key Rotation and Revocation

Security commands use the running server by default:

```shell
kronk security key list
kronk security key create
kronk security key delete --keyid "$KEY_ID"
```

`key create` generates a UUID-named private key. The newest key becomes the
signing key for subsequently created tokens, while older public keys continue
to verify existing tokens.

Kronk does not maintain an individual token revocation list. Deleting a key
immediately invalidates every token signed by that key. Rotate safely by:

1. Creating a new key.
2. Issuing replacement tokens, which use the new key.
3. Waiting for old tokens to expire or confirming they are no longer used.
4. Deleting the old non-master key.

The master key cannot be deleted through the security API. Keep it as the
administrative recovery key.

Add `--local` to operate directly on the local key store without the server:

```shell
kronk security key list --local
```

Local mode still requires `KRONK_TOKEN` containing a valid local admin token.
Use it while the server is stopped because the local command opens the same
Badger database.

## 12.7 Embedded and Standalone Authentication

By default, `kronk server` runs the auth service in-process over an in-memory
listener. Relevant server settings are:

| Flag | Environment variable | Purpose |
| ---- | -------------------- | ------- |
| — | `KRONK_AUTHORIZATION_MODE` | Select `open`, `management`, `authenticated`, or `full-protected`; overrides the legacy authorization settings. |
| `--auth-enabled` | `KRONK_AUTH_LOCAL_ENABLED` | Legacy setting that protects inference and administration. |
| `--admin-auth-enabled` | `KRONK_AUTH_ADMIN_ENABLED` | Legacy setting that protects administration only. |
| `--auth-issuer` | `KRONK_AUTH_LOCAL_ISSUER` | Set the expected JWT issuer. |
| `--auth-host` | `KRONK_AUTH_HOST` | Connect to an external auth service instead. |
| `--auth-tls-enabled` | `KRONK_AUTH_TLS_ENABLED` | Use TLS for an external auth connection. |
| `--auth-tls-ca-file` | `KRONK_AUTH_TLS_CA_FILE` | Trust the certificates in this PEM CA file instead of the system roots. |
| `--auth-tls-server-name` | `KRONK_AUTH_TLS_SERVER_NAME` | Override the TLS certificate server name. |

Setting `KRONK_AUTH_HOST` skips embedded auth startup. The standalone `auth`
service uses `AUTH_AUTH_HOST` (default `localhost:6000`), `AUTH_AUTH_ISSUER`
(default `kronk project`), and `AUTH_AUTH_ENABLED`. Set
`AUTH_AUTH_TLS_ENABLED=true`, `AUTH_AUTH_TLS_CERT_FILE`, and
`AUTH_AUTH_TLS_KEY_FILE` to serve TLS. The Kronk client and auth service must
agree on issuer, protection policy, and transport security. External plaintext
connections remain supported for compatibility but produce a startup warning.

The standalone MCP service uses `MCP_AUTH_TLS_ENABLED`,
`MCP_AUTH_TLS_CA_FILE`, and `MCP_AUTH_TLS_SERVER_NAME` for its connection to the
auth service.

CLI web mode reads `KRONK_WEB_API_HOST`, which defaults to
`localhost:11435`.

## 12.8 Production Hardening

Kronk listens on `0.0.0.0:11435` and serves plain HTTP by default. For any
traffic outside a trusted host:

- enable full or admin authentication as appropriate;
- terminate TLS at a trusted reverse proxy and do not expose the API port
  directly to the public internet;
- enable auth TLS whenever the auth service is reached across an untrusted
  network, or use an authenticated encrypted service mesh;
- restrict network access with host or cloud firewall rules;
- restrict `KRONK_WEB_CORS_ALLOWED_ORIGINS` instead of retaining `*`;
- issue separate, short-lived, least-privilege tokens for each application;
- set quotas based on the workload and monitor authentication failures;
- protect and back up `master.pem` and `master.jwt`; and
- rotate non-master signing keys deliberately, accounting for all tokens that
  a deletion will revoke.

---

_Next: [Chapter 13: Browser UI (BUI)](https://www.kronkai.com/manual#chapter-13-browser-ui-bui)_
