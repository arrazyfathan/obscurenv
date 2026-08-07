# Web Client Changes

Guide for the Obscurenv web dashboard/API client. The backend API lives at
`/api/v1`; the full reference is [`api.md`](api.md) and the interactive OpenAPI
spec is served at `/docs` when the backend runs.

Auth note: the CLI and API use long-lived `obe_tok_...` bearer credentials.
This guide covers the recent auth/token changes first, then the rest of the
web-facing surface.

## 1. Auth & tokens (v0.11.0)

### Token rotation on login

`POST /api/v1/auth/login` and passkey login now **rotate** the user's token
instead of appending a new one. Logging in with a `token_name` that already
exists revokes the previous token and issues a fresh one:

- Each `(user, token_name)` keeps **at most one active token**.
- Password login uses the `token_name` you send (default in the CLI:
  `local-cli`).
- Passkey login always uses the fixed name `passkey-web`.

Password login:

```sh
curl -X POST "$OBE_API_URL/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"super-secret","token_name":"web-dashboard"}'
# 200 {"token":"obe_tok_..."}
```

Passkey login returns the same shape after the `/finish` ceremony.

The raw token is shown exactly once; the backend stores only its SHA-256 hash.

### What the web client must handle

- Re-logging in with the same `token_name` immediately revokes any other
  session using that token — those sessions start returning `401`.
- Recommendation: use a **per-device/per-session** token name (e.g.
  `web-<device-or-session-id>`), or treat a re-login as "replace this session".
  On any `401`, clear stored credentials and send the user through login again.
- When the dashboard issues long-lived tokens from a token-management page,
  give each one a distinct name so creating one does not revoke another.

### No expiry at login

`expires_in_days` was removed from the login request body. The field is
ignored if sent, so:

- Remove the expiry input from the **login form**.
- Tokens issued by login never expire (`expires_at` is null).

Explicit token creation still supports expiry — keep the field on the
token-management UI:

```sh
curl -X POST "$OBE_API_URL/api/v1/tokens" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"ci","expires_in_days":30}'
# 201 {"token":"obe_tok_...","id":"...","expires_at":"2026-09-05T00:00:00Z"}
```

## 2. Token management

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/tokens` | List tokens |
| POST | `/api/v1/tokens` | Create a token (optional `expires_in_days`) |
| DELETE | `/api/v1/tokens/{id}` | Revoke a token |

List response:

```json
{
  "tokens": [
    { "id": "…", "name": "ci", "created_at": "2026-08-01T00:00:00Z", "expires_at": null }
  ]
}
```

`expires_at: null` means the token never expires. `expires_at` is an RFC3339
string when set. On the UI, show the `name`, `created_at`, and `expires_at`
(rendering null as "never"), and offer a revoke action.

## 3. Account settings

- `GET /api/v1/user/profile` → `{ "id", "email", "username", "created_at" }`
  (`username` may be null).
- `PATCH /api/v1/user/profile` with `{ "username": "alice" }` → updated
  profile. Username: 3–32 chars, `[a-zA-Z0-9_][a-zA-Z0-9_-]`, lowercased.
  `409` if the username is taken.
- `POST /api/v1/user/password` with
  `{ "current_password", "new_password" }` (new ≥ 8 chars). `400` if the
  current password is wrong.
- `DELETE /api/v1/user` with `{ "confirm": true }` → deletes the account and
  all of its data. Only call with an explicit user confirmation.

## 4. Projects

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/projects` | List projects (`?search=` or `?q=`) |
| GET | `/api/v1/projects/{slug}` | Project detail + latest environments |
| POST | `/api/v1/projects` | Create (`name`, `slug`) |
| PATCH | `/api/v1/projects/{slug}` | Rename (`{ "name" }`; slug unchanged) |
| DELETE | `/api/v1/projects/{slug}` | Delete project and environments |

List supports search across name, slug, and environment name. Each project
includes `environment_count`, `latest_version`, `latest_updated_at`, and an
`environments` array (latest version per environment) for the dashboard view.

## 5. Environments (zero-knowledge)

The backend stores only **opaque encrypted payloads**. Never send plaintext
`.env` values, passphrases, or keys.

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/v1/env/push` | Store a new version |
| GET | `/api/v1/env/pull` | Pull latest (or `?version=N`) |
| GET | `/api/v1/env/list` | List environment names |
| GET | `/api/v1/env/versions` | Version history (payloads omitted) |
| GET | `/api/v1/env/export` | Latest version per environment (payloads included) |
| DELETE | `/api/v1/env` | Delete an environment (`?project=&environment=`) |

Push payload:

```json
{
  "project_slug": "my-app",
  "environment": "production",
  "encrypted_payload": "…opaque…",
  "checksum": "…"
}
```

`checksum` must equal the lowercase hex SHA-256 of the raw
`encrypted_payload` bytes; otherwise the request is rejected with `400`.

## 6. Activity feed

`GET /api/v1/activity` returns the user's changes, newest first.

Query params:

| Param | Default | Description |
|-------|---------|-------------|
| `limit` | 50 | 1–100 |
| `offset` | 0 | Rows to skip |
| `project` | – | Filter by project slug |
| `action` | – | `project.created`, `project.deleted`, `project.renamed`, `env.pushed`, `env.deleted` |
| `from` / `to` | – | RFC3339 time range |
| `cursor` | – | Opaque cursor from `next_cursor` |

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
  "total": 12,
  "next_cursor": "…"
}
```

Paginate with `cursor` when present; `total` counts all matching rows
(ignoring `limit`/`offset`).
