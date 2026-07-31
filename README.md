# Obscurenv (`obv`)

Obscurenv is a personal zero-knowledge `.env` cloud storage tool.

It has two parts:

- `backend/`: Go REST API using Gin and PostgreSQL.
- `cli/`: Go CLI named `obv` using Cobra.

The CLI encrypts `.env` content locally before upload. The backend stores only opaque encrypted payloads, checksums, versions, users, projects, and API token hashes. The backend never receives plaintext `.env` values or encryption passphrases.

## Current Status

This repository is an MVP foundation. It is usable locally for register/login, project creation, encrypted push/pull, environment listing, swapping, and in-memory command execution.

Not production complete yet:

- No hosted deployment config.
- No hidden passphrase prompt; commands currently accept `-k`.
- Limited `.env` parser support.
- Basic test coverage only.
- No conflict resolution flow beyond storing checksums.

## Requirements

- Go 1.24 or newer.
- PostgreSQL 16 or newer.
- `curl` for API examples.

Optional:

- Docker Compose, if you want to run PostgreSQL in Docker.
- Supabase, if you want a managed PostgreSQL database.

## Repository Layout

```text
.
├── backend/
│   ├── main.go
│   ├── db/
│   ├── handlers/
│   ├── middleware/
│   └── models/
├── cli/
│   ├── main.go
│   ├── cmd/
│   └── pkg/
│       ├── api/
│       └── crypto/
├── AGENTS.md
├── docker-compose.yml
├── go.work
└── README.md
```

## How The Flow Works

1. Create a user on the backend.
2. Login to get an API token.
3. Store the token locally with `obv login`.
4. Create a project on the backend.
5. Link a local folder to that project with `obv init`.
6. Create a local `.env`.
7. Run `obv push` to encrypt and upload it.
8. Run `obv pull`, `obv swap`, or `obv run` when needed.

## Start PostgreSQL

Choose one database option.

### Option A: Docker PostgreSQL

```sh
docker compose up -d postgres
```

Use this backend database URL:

```sh
postgres://obv:obv@localhost:5432/obv?sslmode=disable
```

### Option B: Postgres.app

Start Postgres.app, then check it:

```sh
psql -h 127.0.0.1 -p 5432 -d postgres -c 'select current_database(), current_user;'
```

Use this backend database URL if your local user is `macintosh`:

```sh
postgres://macintosh@127.0.0.1:5432/postgres?sslmode=disable
```

### Option C: Supabase PostgreSQL

Use the Supabase database connection string, not the Supabase REST API URL.

Direct connection example:

```sh
postgresql://postgres:[PASSWORD]@db.[PROJECT_REF].supabase.co:5432/postgres?sslmode=require
```

Session pooler example:

```sh
postgres://postgres.[PROJECT_REF]:[PASSWORD]@aws-[REGION].pooler.supabase.com:5432/postgres?sslmode=require
```

Use the session pooler if your deploy environment or network cannot reach Supabase over IPv6.

## Run The Backend

From the repository root:

```sh
cd backend
DATABASE_URL="postgres://macintosh@127.0.0.1:5432/postgres?sslmode=disable" ADDR=:8080 go run .
```

For Docker PostgreSQL:

```sh
cd backend
DATABASE_URL="postgres://obv:obv@localhost:5432/obv?sslmode=disable" ADDR=:8080 go run .
```

The backend runs migrations automatically on startup. It creates:

- `users`
- `api_tokens`
- `projects`
- `env_versions`

The API is available at:

```text
http://localhost:8080
```

## Build The CLI

Open another terminal from the repository root:

```sh
cd cli
go build -o ../bin/obv .
```

Set the CLI to use your local backend:

```sh
export OBV_API_URL=http://localhost:8080
```

If `OBV_API_URL` is not set, the CLI defaults to:

```text
https://localhost:8080
```

## First-Time Setup

### 1. Register A User

```sh
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"securepassword"}'
```

Expected response:

```json
{
  "message": "User registered successfully"
}
```

### 2. Login And Get A Token

```sh
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"securepassword","token_name":"local-cli"}'
```

Expected response:

```json
{
  "token": "obv_tok_..."
}
```

### 3. Store The Token Locally

From the repository root:

```sh
./bin/obv login --token obv_tok_...
```

This writes:

```text
~/.obv/credentials.json
```

### 4. Create A Project

```sh
curl -X POST http://localhost:8080/api/v1/projects \
  -H "Authorization: Bearer obv_tok_..." \
  -H "Content-Type: application/json" \
  -d '{"name":"My App","slug":"my-app"}'
```

Expected response:

```json
{
  "id": "uuid...",
  "slug": "my-app"
}
```

### 5. Link Your Local Folder

Run this inside the folder where your `.env` file lives:

```sh
/path/to/obscurenv/bin/obv init -p my-app
```

This creates:

```json
{
  "project_slug": "my-app",
  "active_environment": "development"
}
```

The file is named `.obv.json`.

## Common CLI Workflows

### Push Local `.env` To Cloud

```sh
./bin/obv push -k "MySecretPassphrase123!" -e development
```

What happens:

- Reads local `.env`.
- Calculates SHA-256 checksum of plaintext.
- Encrypts plaintext locally with Argon2id + AES-256-GCM.
- Uploads only encrypted payload and checksum.
- Backend stores a new version.

### Pull Remote `.env` To Disk

```sh
./bin/obv pull -k "MySecretPassphrase123!" -e development
```

What happens:

- Downloads latest encrypted payload.
- Decrypts locally.
- Writes `.env` only after decryption succeeds.

If the passphrase is wrong, `.env` is not modified.

### List Remote Environments

```sh
./bin/obv env ls
```

Example output:

```text
development
production
staging
```

### Swap Active Environment

```sh
./bin/obv swap production -k "MySecretPassphrase123!"
```

What happens:

- Pushes the current `.env` as the current active environment.
- Updates `.obv.json` to `production`.
- Pulls the latest `production` payload.
- Replaces local `.env` only after decrypting successfully.

### Run A Command Without Writing `.env`

```sh
./bin/obv run -e staging -k "MySecretPassphrase123!" -- npm start
```

What happens:

- Downloads encrypted `staging` payload.
- Decrypts it in memory.
- Parses `KEY=VALUE` lines.
- Starts the child command with those variables injected into `cmd.Env`.
- Does not write decrypted content to `.env`.

Simple test:

```sh
./bin/obv run -e development -k "MySecretPassphrase123!" -- printenv SECRET
```

## Full Local Example

From repository root, with backend already running:

```sh
export OBV_API_URL=http://localhost:8080

cd cli
go build -o ../bin/obv .
cd ..

curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"securepassword"}'

TOKEN="$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"securepassword","token_name":"local-cli"}' \
  | sed -E 's/.*"token":"([^"]+)".*/\1/')"

./bin/obv login --token "$TOKEN"

curl -X POST http://localhost:8080/api/v1/projects \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"My App","slug":"my-app"}'

./bin/obv init -p my-app
printf 'DATABASE_URL=postgres://localhost\nSECRET=local-secret\n' > .env

./bin/obv push -k "MySecretPassphrase123!" -e development
./bin/obv env ls
./bin/obv pull -k "MySecretPassphrase123!" -e development
./bin/obv run -e development -k "MySecretPassphrase123!" -- printenv SECRET
```

## API Reference

All protected endpoints require:

```text
Authorization: Bearer <API_TOKEN>
```

### Register

```http
POST /api/v1/auth/register
```

```json
{
  "email": "user@example.com",
  "password": "securepassword"
}
```

### Login

```http
POST /api/v1/auth/login
```

```json
{
  "email": "user@example.com",
  "password": "securepassword",
  "token_name": "macbook-cli"
}
```

### Create Project

```http
POST /api/v1/projects
```

```json
{
  "name": "My App",
  "slug": "my-app"
}
```

### Push Environment

```http
POST /api/v1/env/push
```

```json
{
  "project_slug": "my-app",
  "environment": "development",
  "encrypted_payload": "{\"version\":1,\"kdf\":\"argon2id\",\"salt\":\"...\",\"ciphertext\":\"...\"}",
  "checksum": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
}
```

### Pull Environment

```http
GET /api/v1/env/pull?project=my-app&environment=development
```

### List Environments

```http
GET /api/v1/env/list?project=my-app
```

## Configuration Files

### Local Project Config

`.obv.json` is created by `obv init`:

```json
{
  "project_slug": "my-app",
  "active_environment": "development"
}
```

This file should not be committed unless you intentionally want the project slug shared with the repo.

### Local CLI Credentials

`obv login` writes:

```text
~/.obv/credentials.json
```

Example:

```json
{
  "token": "obv_tok_..."
}
```

## Security Model

- `.env` plaintext is read only by the CLI.
- Passphrase is used only by the CLI.
- Argon2id derives a 32-byte AES key from the passphrase and a random salt.
- AES-256-GCM encrypts and authenticates the payload.
- The encrypted payload contains a versioned JSON envelope with salt and ciphertext.
- The backend stores the encrypted payload as opaque text.
- API tokens are hashed before database storage.
- Wrong passphrase causes AES-GCM authentication failure and prevents `.env` overwrite.

Recommended `.gitignore` entries for projects using Obscurenv:

```gitignore
.env
.obv.json
```

## Validation

From the repository root:

```sh
go test ./backend/... ./cli/...
go vet ./backend/... ./cli/...
```

## Troubleshooting

### `connection refused` from backend

PostgreSQL is not running or `DATABASE_URL` is wrong.

Check local Postgres:

```sh
psql -h 127.0.0.1 -p 5432 -d postgres -c 'select current_database(), current_user;'
```

### `project not found`

Create the project first:

```sh
curl -X POST http://localhost:8080/api/v1/projects \
  -H "Authorization: Bearer obv_tok_..." \
  -H "Content-Type: application/json" \
  -d '{"name":"My App","slug":"my-app"}'
```

Then link the local folder:

```sh
./bin/obv init -p my-app
```

### `decrypt failed; .env was not modified`

The passphrase is wrong, the encrypted payload is corrupted, or the payload was encrypted with a different passphrase.

Use the same passphrase that was used for `obv push`.

### CLI tries `https://localhost:8080`

Set the local API URL:

```sh
export OBV_API_URL=http://localhost:8080
```

### `user already exists`

Use the existing user and login again:

```sh
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"securepassword","token_name":"local-cli"}'
```

## Deployment Recommendation

For a free personal deployment:

```text
Backend API: Koyeb free web service
Database: Supabase free Postgres
CLI: local binary
```

Set `DATABASE_URL` on the hosted backend to your Supabase Postgres connection string with `sslmode=require`.

Production deployments must expose the backend over HTTPS.
