# Obscurenv API Guide

Interactive reference: run the backend and open `http://localhost:8080/docs`.
Raw OpenAPI spec: `backend/api-docs/openapi.yaml` (source of truth).

> Building a web dashboard or other client? See
> [`web-client-changes.md`](web-client-changes.md) for what the client side
> must implement (tokens, account settings, project rename, export, activity
> pagination).

## Base URL

The backend defaults to `http://localhost:8080` (port from `PORT` or `ADDR`).
The CLI uses `OBE_API_URL`. Health endpoints:

```sh
curl -i "$OBE_API_URL/healthz"
```

## Getting a token

1. Register (first time):

   ```sh
   curl -X POST "$OBE_API_URL/api/v1/auth/register" \
     -H 'Content-Type: application/json' \
     -d '{"email":"alice@example.com","username":"alice","password":"super-secret"}'
   ```

2. Log in to receive a token:

   ```sh
   curl -X POST "$OBE_API_URL/api/v1/auth/login" \
     -H 'Content-Type: application/json' \
     -d '{"email":"alice@example.com","password":"super-secret","token_name":"cli"}'
   # {"token":"obe_tok_..."}
   ```

The token is an `obe_tok_...` bearer credential. It is returned exactly once;
the backend stores only its hash. Passkey login returns a token the same way.
If you lose it, log in again (or revoke/re-issue from the CLI).

Authenticate all protected endpoints with:

```text
Authorization: Bearer <API_TOKEN>
```

## Zero-knowledge contract

This API stores **only opaque encrypted payloads**. Never send:

- plaintext `.env` values,
- passphrases,
- derived keys or raw encryption keys.

The CLI encrypts locally (Argon2id + AES-256-GCM) and uploads an opaque
envelope. The backend never inspects, parses, decrypts, or transforms
`encrypted_payload` — it treats it as text and stores a new version.

For `POST /api/v1/env/push`, `checksum` must be the lowercase hex SHA-256 of
the raw `encrypted_payload` bytes; otherwise the request is rejected:

```text
sha256(encrypted_payload) == checksum
```

## Errors

Every error returns a non-2xx status with a JSON body:

```json
{ "error": "project not found" }
```

Common status codes:

| Code | Meaning |
|------|---------|
| 400  | Invalid request body or query parameters |
| 401  | Missing/invalid bearer token or bad credentials |
| 404  | Project, environment, user, or passkey not found |
| 409  | Duplicate project slug, email, username, or passkey |
| 503  | Passkey authentication not configured on this instance |
| 500  | Server error |

## Endpoints

All routes under `/api/v1`:

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/auth/register` | – | Create a user |
| POST | `/auth/login` | – | Password login → token |
| POST | `/auth/passkey/login/options` | – | Start passkey login |
| POST | `/auth/passkey/login/finish` | – | Finish passkey login → token |
| GET | `/auth/passkeys` | Bearer | List passkeys |
| DELETE | `/auth/passkeys/{id}` | Bearer | Revoke a passkey |
| POST | `/auth/passkey/register/options` | Bearer | Start passkey registration |
| POST | `/auth/passkey/register/finish` | Bearer | Finish passkey registration |
| GET | `/projects` | Bearer | List projects (`?search=`, `?q=`) |
| POST | `/projects` | Bearer | Create a project |
| GET | `/projects/{slug}` | Bearer | Get project + latest environments |
| DELETE | `/projects/{slug}` | Bearer | Delete project and its environments |
| POST | `/env/push` | Bearer | Store an encrypted environment (new version) |
| GET | `/env/pull` | Bearer | Pull latest (or `?version=N`) environment |
| GET | `/env/list` | Bearer | List environment names for a project |
| GET | `/env/versions` | Bearer | List version history (payloads omitted) |
| DELETE | `/env` | Bearer | Delete an environment and its versions |
| GET | `/user/profile` | Bearer | Get profile |
| PATCH | `/user/profile` | Bearer | Update username |
| GET | `/activity` | Bearer | Recent activity feed |

Passkey `options` responses contain opaque WebAuthn PublicKeyCredential JSON —
pass them straight to the authenticator; keep the `ceremony_id` for the
matching `/finish` call.

## Activity feed

`GET /api/v1/activity` returns the authenticated user's changes, newest first,
scoped to their projects. API-only for now (no CLI command).

Query parameters:

| Param | Default | Description |
|-------|---------|-------------|
| `limit` | 50 | Items to return (1–100) |
| `offset` | 0 | Items to skip |
| `project` | – | Filter by project slug |
| `action` | – | `project.created`, `project.deleted`, `env.pushed`, `env.deleted` |

Response:

```json
{
  "activities": [
    {
      "id": "…",
      "action": "env.pushed",
      "project_slug": "my-app",
      "environment_name": "production",
      "metadata": { "version": 4 },
      "created_at": "2026-08-06T12:00:00Z"
    }
  ],
  "total": 12
}
```

`total` counts all matching rows (ignores `limit`/`offset`). `project_slug`
and `environment_name` are denormalized, so entries survive project or
environment deletion.

## Worked example

Encrypted push, pull, and activity (assumes the CLI produced the envelope):

```sh
TOKEN=$(curl -s -X POST "$OBE_API_URL/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"super-secret","token_name":"docs"}' | jq -r .token)

curl -X POST "$OBE_API_URL/api/v1/projects" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"My App","slug":"my-app"}'

curl -X POST "$OBE_API_URL/api/v1/env/push" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"project_slug\":\"my-app\",\"environment\":\"production\",\"encrypted_payload\":\"$PAYLOAD\",\"checksum\":\"$CHECKSUM\"}"

curl "$OBE_API_URL/api/v1/env/pull?project=my-app&environment=production" \
  -H "Authorization: Bearer $TOKEN"

curl "$OBE_API_URL/api/v1/activity?project=my-app" \
  -H "Authorization: Bearer $TOKEN"
```

Prefer the `obe` CLI for normal operation; the API is for integrations.
