# AGENTS.md

## Project
Obscurenv (`obe`) is a zero-knowledge encrypted `.env` storage product.

The repository is split into:

- `backend/`: Go REST API using Gin and PostgreSQL.
- `cli/`: Go Cobra CLI for encryption, sync, swapping, and in-memory command execution.

## Security Rules
- Never send plaintext `.env` values to the backend.
- Never send passphrases, derived keys, or raw encryption keys to the backend.
- The backend stores only opaque encrypted payloads and metadata.
- API tokens must be stored hashed in the database.
- CLI credentials may be stored locally, but encryption passphrases must not be persisted.
- On decrypt failure, the CLI must exit without modifying the existing local `.env`.
- Do not log secrets, plaintext env values, tokens, passphrases, or decrypted payloads.

## Backend Agent Guidelines
- Use Go, Gin, `database/sql`, and PostgreSQL.
- Keep handlers thin: parse request, authorize user, call storage/service logic, return JSON.
- All authenticated endpoints must resolve the user from `Authorization: Bearer <API_TOKEN>`.
- Validate request bodies and return consistent JSON errors.
- Use UUID v4 primary keys.
- Scope all project and env queries by authenticated `user_id`.
- Treat `encrypted_payload` as opaque text; do not inspect, parse, decrypt, or transform it.
- `env_versions.version` must increment per project and environment.
- `GET /api/v1/env/pull` must return the latest version unless version support is explicitly added later.

## CLI Agent Guidelines
- Use Cobra for commands.
- Keep crypto code isolated under `cli/pkg/crypto`.
- Keep API calls isolated from command handlers.
- Read `.obe.json` from the current project directory.
- Read API credentials from `~/.obe/credentials.json`.
- Do not overwrite `.env` until decryption and validation have completed successfully.
- `obe run` must inject variables through `os/exec.Cmd.Env` and must not write decrypted content to disk.
- Prefer clear, actionable CLI errors with non-zero exit codes.

## Test Expectations
- Add unit tests for crypto round-trip and wrong-passphrase failure.
- Add backend handler tests for auth, push, pull, and list.
- Add CLI tests for env parsing, config loading, and safe `.env` overwrite behavior.
- Include at least one integration path covering push then pull using encrypted payloads.

## Commands
Expected validation commands once implemented:

```sh
go test ./backend/... ./cli/...
go vet ./backend/... ./cli/...
```
