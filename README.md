# Obscurenv (`obv`)

Obscurenv is a self-hosted, zero-knowledge encrypted `.env` storage system.

You run the backend API and PostgreSQL database on infrastructure you control, then use the `obv` CLI from your projects to push, pull, swap, and run environments. The CLI encrypts `.env` content locally before upload, so the server only stores opaque encrypted payloads and metadata.

The backend never receives plaintext `.env` values, passphrases, derived keys, or raw encryption keys.

## What You Host

Obscurenv has two parts:

- `backend/`: Go REST API using Gin and PostgreSQL.
- `cli/`: Go Cobra CLI named `obv`.

The hosted backend stores:

- Users.
- Hashed API tokens.
- Projects.
- Environment names.
- Encrypted payload versions.
- Plaintext checksums for local change/conflict detection.

The backend does not decrypt, inspect, parse, transform, or validate the encrypted `.env` payload.

## Current Status

This repository is an MVP foundation for a self-hosted Obscurenv server.

Implemented:

- User registration and login.
- API token authentication.
- Project creation.
- Encrypted environment push and pull.
- Environment listing.
- Environment swapping.
- In-memory command execution with `obv run`.
- Dockerfile for the backend.
- Docker Compose stack for local backend + PostgreSQL.

Not production complete yet:

- No packaged release binaries.
- No managed hosted service.
- No hidden passphrase prompt; commands currently accept `-k`.
- Limited `.env` parser support.
- Basic test coverage only.
- No conflict resolution flow beyond storing checksums.

If you expose the backend outside localhost, put it behind HTTPS.

## Requirements

- Go 1.24 or newer.
- PostgreSQL 16 or newer.
- `curl` for API examples.

Optional:

- Docker Compose, for running the self-hosted stack locally.
- Any server or container platform that can run a Go HTTP service.
- Managed PostgreSQL, such as Supabase, Neon, RDS, or a PostgreSQL instance you operate.

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
├── .env.example
├── docker-compose.yml
├── go.work
└── README.md
```

## Self-Hosted Quick Start

The fastest way to run your own Obscurenv server is Docker Compose.

```sh
docker compose up -d
```

This starts:

- PostgreSQL on `localhost:5432`.
- Obscurenv backend API on `localhost:8080`.

The backend runs database migrations automatically on startup.

Install the CLI:

```sh
./install.sh
```

The installer builds `cli/` and copies `obv` to `~/.local/bin` by default. If that directory is not on your `PATH`, add it for zsh on macOS:

```sh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

Point the CLI at your self-hosted server:

```sh
export OBV_API_URL=http://localhost:8080
```

If `OBV_API_URL` is not set, the CLI currently defaults to:

```text
https://localhost:8080
```

## Install The CLI

From the repository root:

```sh
./install.sh
```

By default this installs:

```text
~/.local/bin/obv
```

To install somewhere else:

```sh
./install.sh --install-dir /usr/local/bin
```

You can also set `INSTALL_DIR`:

```sh
INSTALL_DIR="$HOME/bin" ./install.sh
```

Verify the install:

```sh
obv version
```

For development, build a repo-local versioned binary with Make:

```sh
make build
./bin/obv version
```

## Make Commands

Common development commands are available from the repository root:

```sh
make help
make build
make install
make package
make release
make test
make vet
make check
make backend
make up
make down
```

Useful targets:

- `make build`: builds the CLI to `./bin/obv`.
- `make install`: builds and installs `obv` using `./install.sh`.
- `make package`: builds and writes a versioned tarball to `./dist`.
- `make release`: runs tests and vet, then writes a versioned tarball to `./dist`.
- `make test`: runs `go test ./backend/... ./cli/...`.
- `make vet`: runs `go vet ./backend/... ./cli/...`.
- `make check`: runs tests and vet.
- `make backend`: runs the backend locally with `DATABASE_URL` and `ADDR`.
- `make up`: starts PostgreSQL and backend with Docker Compose.
- `make down`: stops Docker Compose services.

You can override paths and local backend settings:

```sh
INSTALL_DIR="$HOME/bin" make install
BACKEND_ADDR=:9090 make backend
DATABASE_URL="postgres://user:password@localhost:5432/obv?sslmode=disable" make backend
```

## CLI Versioning

The CLI uses SemVer. The source of truth is:

```text
VERSION
```

Current version:

```sh
make version
```

The build embeds:

- `version`: from `VERSION`.
- `commit`: current Git commit short SHA.
- `built`: UTC build timestamp.

Check a binary:

```sh
obv version
obv --version
```

Example output:

```text
obv version 0.1.0
commit: da4759e
built: 2026-07-31T06:48:51Z
```

To cut a release:

```sh
printf '0.1.1\n' > VERSION
make release
git add VERSION Makefile install.sh README.md cli/cmd
git commit -m "Release obv v0.1.1"
git tag v0.1.1
```

This creates a package like:

```text
dist/obv_0.1.1_darwin_arm64.tar.gz
```

## Server Configuration

Example local environment variables are provided in `.env.example`:

```sh
cp .env.example .env
```

The backend uses:

```sh
DATABASE_URL="postgres://obv:obv@localhost:5432/obv?sslmode=disable"
ADDR=":8080"
```

On Vercel, the platform provides `PORT` automatically. Obscurenv uses `ADDR` when set, otherwise it listens on `PORT`, otherwise it falls back to `:8080`.

The CLI uses:

```sh
OBV_API_URL="http://localhost:8080"
```

For Docker Compose, these are already configured in `docker-compose.yml`:

```text
DATABASE_URL=postgres://obv:obv@postgres:5432/obv?sslmode=disable
ADDR=:8080
POSTGRES_USER=obv
POSTGRES_PASSWORD=obv
POSTGRES_DB=obv
```

For a real server, set `DATABASE_URL` to your production PostgreSQL connection string and expose `ADDR` behind a reverse proxy or load balancer with HTTPS.

Managed PostgreSQL examples:

```sh
DATABASE_URL="postgresql://postgres:[PASSWORD]@db.[PROJECT_REF].supabase.co:5432/postgres?sslmode=require"
```

```sh
DATABASE_URL="postgres://user:password@host:5432/obv?sslmode=require"
```

## Run The Backend Without Docker

Start PostgreSQL first, then run:

```sh
cp .env.example .env
cd backend
DATABASE_URL="postgres://obv:obv@localhost:5432/obv?sslmode=disable" ADDR=:8080 go run .
```

The API will be available at:

```text
http://localhost:8080
```

## First-Time Setup

### 1. Register A User

```sh
curl -X POST "$OBV_API_URL/api/v1/auth/register" \
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
curl -X POST "$OBV_API_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"securepassword","token_name":"local-cli"}'
```

Expected response:

```json
{
  "token": "obv_tok_..."
}
```

For shell examples:

```sh
TOKEN="$(curl -s -X POST "$OBV_API_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"securepassword","token_name":"local-cli"}' \
  | sed -E 's/.*"token":"([^"]+)".*/\1/')"
```

### 3. Store The Token Locally

```sh
obv login --token "$TOKEN"
```

This writes:

```text
~/.obv/credentials.json
```

The token authenticates the CLI to your self-hosted backend. It is not an encryption passphrase.

### 4. Create A Project

```sh
curl -X POST "$OBV_API_URL/api/v1/projects" \
  -H "Authorization: Bearer $TOKEN" \
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

### 5. Link A Local Folder

Run this inside the folder where your `.env` file lives:

```sh
obv init -p my-app
```

This creates `.obv.json`:

```json
{
  "project_slug": "my-app",
  "active_environment": "development"
}
```

## Common CLI Workflows

### Push Local `.env`

```sh
obv push -k "MySecretPassphrase123!" -e development
```

What happens:

- Reads local `.env`.
- Calculates SHA-256 checksum of plaintext.
- Encrypts plaintext locally with Argon2id + AES-256-GCM.
- Uploads only encrypted payload and checksum.
- Backend stores a new version.

### Pull Remote `.env`

```sh
obv pull -k "MySecretPassphrase123!" -e development
```

What happens:

- Downloads the latest encrypted payload.
- Decrypts locally.
- Writes `.env` only after decryption succeeds.

If the passphrase is wrong, `.env` is not modified.

### List Environments

```sh
obv env ls
```

Example output:

```text
development
production
staging
```

### Swap Active Environment

```sh
obv swap production -k "MySecretPassphrase123!"
```

What happens:

- Pushes the current `.env` as the current active environment.
- Updates `.obv.json` to `production`.
- Pulls the latest `production` payload.
- Replaces local `.env` only after decrypting successfully.

### Run A Command Without Writing `.env`

```sh
obv run -e staging -k "MySecretPassphrase123!" -- npm start
```

What happens:

- Downloads encrypted `staging` payload.
- Decrypts it in memory.
- Parses `KEY=VALUE` lines.
- Starts the child command with those variables injected into `os/exec.Cmd.Env`.
- Does not write decrypted content to `.env`.

Simple test:

```sh
obv run -e development -k "MySecretPassphrase123!" -- printenv SECRET
```

## Full Local Example

From the repository root, with the backend running:

```sh
export OBV_API_URL=http://localhost:8080

./install.sh

curl -X POST "$OBV_API_URL/api/v1/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"securepassword"}'

TOKEN="$(curl -s -X POST "$OBV_API_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"securepassword","token_name":"local-cli"}' \
  | sed -E 's/.*"token":"([^"]+)".*/\1/')"

obv login --token "$TOKEN"

curl -X POST "$OBV_API_URL/api/v1/projects" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"My App","slug":"my-app"}'

obv init -p my-app
printf 'DATABASE_URL=postgres://localhost\nSECRET=local-secret\n' > .env

obv push -k "MySecretPassphrase123!" -e development
obv env ls
obv pull -k "MySecretPassphrase123!" -e development
obv run -e development -k "MySecretPassphrase123!" -- printenv SECRET
```

## Deploy On Your Own Server

Build and run the backend container:

```sh
docker build -t obv-backend ./backend
docker run -p 8080:8080 \
  -e ADDR=:8080 \
  -e DATABASE_URL="postgres://user:password@db-host:5432/obv?sslmode=require" \
  obv-backend
```

Recommended production shape:

```text
Client projects -> obv CLI -> HTTPS reverse proxy -> Obscurenv backend -> PostgreSQL
```

Operational notes:

- Terminate TLS before the backend or run it behind a platform that provides HTTPS.
- Keep PostgreSQL private to the backend.
- Use a strong database password.
- Back up PostgreSQL regularly; encrypted payloads are still your environment history.
- Rotate API tokens if credentials are exposed.
- Do not log request bodies at your proxy or platform layer.
- Treat `DATABASE_URL` as a secret.

After deployment, point every CLI user at the server:

```sh
export OBV_API_URL=https://obv.example.com
```

Then users can register, login, create projects, and sync encrypted environments against your self-hosted instance.

## Deploy Backend To Vercel

Vercel can run the Go backend, but it will not run the PostgreSQL service from `docker-compose.yml`. Use an external PostgreSQL database such as Supabase, Neon, RDS, or another Postgres server you operate.

Recommended Vercel setup:

```text
Vercel project root: backend
Framework preset: Other
Build command: default
Output directory: default
Install command: default
```

Set this Vercel environment variable:

```text
DATABASE_URL=postgres://user:password@host:5432/obv?sslmode=require
```

Do not set `ADDR` on Vercel unless you have a specific reason. Vercel provides `PORT`, and the backend will listen on it automatically.

Deploy flow:

1. Push this repository to GitHub, GitLab, or Bitbucket.
2. Create a new Vercel project from the repository.
3. Set the root directory to `backend`.
4. Add `DATABASE_URL` in Vercel Project Settings.
5. Deploy.
6. Copy the production deployment URL.

Point the CLI at the deployed backend:

```sh
export OBV_API_URL=https://your-obscurenv-backend.vercel.app
```

Smoke test the deployed API:

```sh
curl -X POST "$OBV_API_URL/api/v1/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"securepassword"}'
```

For serverless-style deployments, prefer a pooled PostgreSQL connection string when your database provider offers one. Vercel can scale multiple instances, and each instance may open database connections.

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

This file should not be committed unless you intentionally want the project slug shared with the repository.

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

CLI credentials may be stored locally. Encryption passphrases must not be persisted.

## Security Model

- `.env` plaintext is read only by the CLI.
- Passphrases are used only by the CLI.
- Argon2id derives a 32-byte AES key from the passphrase and a random salt.
- AES-256-GCM encrypts and authenticates the payload.
- The encrypted payload contains a versioned JSON envelope with salt and ciphertext.
- The backend stores the encrypted payload as opaque text.
- API tokens are hashed before database storage.
- Wrong passphrase causes AES-GCM authentication failure and prevents `.env` overwrite.
- `obv run` injects decrypted variables through process environment only and does not write decrypted content to disk.

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

The backend is not running, PostgreSQL is not running, or `OBV_API_URL`/`DATABASE_URL` is wrong.

Check that the API is reachable. A `401` response is expected here unless you include a valid token:

```sh
curl -i "$OBV_API_URL/api/v1/env/list?project=my-app"
```

Check local PostgreSQL:

```sh
psql -h 127.0.0.1 -p 5432 -d postgres -c 'select current_database(), current_user;'
```

### `project not found`

Create the project first:

```sh
curl -X POST "$OBV_API_URL/api/v1/projects" \
  -H "Authorization: Bearer obv_tok_..." \
  -H "Content-Type: application/json" \
  -d '{"name":"My App","slug":"my-app"}'
```

Then link the local folder:

```sh
obv init -p my-app
```

### `decrypt failed; .env was not modified`

The passphrase is wrong, the encrypted payload is corrupted, or the payload was encrypted with a different passphrase.

Use the same passphrase that was used for `obv push`.

### CLI tries `https://localhost:8080`

Set the API URL for your self-hosted server:

```sh
export OBV_API_URL=http://localhost:8080
```

or:

```sh
export OBV_API_URL=https://obv.example.com
```

### `user already exists`

Use the existing user and login again:

```sh
curl -X POST "$OBV_API_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"securepassword","token_name":"local-cli"}'
```
