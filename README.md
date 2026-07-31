# Obscurenv (`obv`)

Self-hosted, zero-knowledge encrypted `.env` storage.

The CLI encrypts `.env` locally before upload. The backend stores only opaque encrypted payloads and metadata; it never receives plaintext env values, passphrases, derived keys, or raw encryption keys.

## Components

- `backend/`: Go REST API using Gin and PostgreSQL.
- `cli/`: Go Cobra CLI named `obv`.

## Requirements

- Go 1.24+
- PostgreSQL 16+
- Optional: Docker Compose for local backend + PostgreSQL

## Quick Start

Start the local backend:

```sh
docker compose up -d
```

Install the CLI:

```sh
./install.sh
```

If `~/.local/bin` is not on your `PATH`:

```sh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

Run the interactive setup:

```sh
obv login --register
obv init
```

Then push a local `.env`:

```sh
printf 'DATABASE_URL=postgres://localhost\nSECRET=local-secret\n' > .env
obv push
```

`obv push`, `obv pull`, `obv swap`, and `obv run` prompt for the encryption passphrase when `-k` is omitted. Passphrases are never stored.

## Common Commands

```sh
obv login --register
obv init
obv push
obv pull
obv env ls
obv swap production
obv run -- npm start
obv version
```

Script-friendly examples:

```sh
obv login --api-url https://obv.example.com --email you@example.com
obv login --token "$TOKEN" --api-url https://obv.example.com
obv init --project my-app --env development --create
obv push --env development --key "$OBV_PASSPHRASE"
obv run --env staging --key "$OBV_PASSPHRASE" -- npm start
```

## Configuration

Example local env file:

```sh
cp .env.example .env
```

Runtime variables:

```sh
DATABASE_URL=postgres://obv:obv@localhost:5432/obv?sslmode=disable
ADDR=:8080
PORT=
OBV_API_URL=http://localhost:8080
```

Precedence:

- Backend listens on `ADDR`, then `PORT`, then `:8080`.
- CLI uses `OBV_API_URL`; if unset, it uses the API URL saved by `obv login`; otherwise it falls back to `https://localhost:8080`.

Local files:

- `.obv.json`: project config created by `obv init`.
- `~/.obv/credentials.json`: API token and API URL created by `obv login`.
- `.env`: plaintext env file; do not commit it.

Recommended `.gitignore` for projects using Obscurenv:

```gitignore
.env
.obv.json
```

## Development

Use Make from the repository root:

```sh
make help
make build
make install
make test
make vet
make check
make backend
make up
make down
```

Useful targets:

- `make build`: build `./bin/obv`.
- `make install`: build and install with `./install.sh`.
- `make test`: run backend and CLI tests.
- `make vet`: run `go vet`.
- `make check`: run tests and vet.
- `make backend`: run backend locally.
- `make up`: start Docker Compose services.
- `make down`: stop Docker Compose services.

Override local settings:

```sh
INSTALL_DIR="$HOME/bin" make install
BACKEND_ADDR=:9090 make backend
DATABASE_URL="postgres://user:password@localhost:5432/obv?sslmode=disable" make backend
```

## Versioning

The CLI uses SemVer. The source of truth is:

```text
VERSION
```

Check the current version:

```sh
make version
obv version
obv --version
```

Build metadata includes version, Git commit, and UTC build time.

Cut a release:

```sh
printf '0.2.1\n' > VERSION
make release
git add VERSION Makefile install.sh README.md cli/cmd
git commit -m "Release obv v0.2.1"
git tag v0.2.1
```

Release artifacts are written to `dist/`, for example:

```text
dist/obv_0.2.1_darwin_arm64.tar.gz
```

## Backend Deployment

Docker:

```sh
docker build -t obv-backend ./backend
docker run -p 8080:8080 \
  -e ADDR=:8080 \
  -e DATABASE_URL="postgres://user:password@db-host:5432/obv?sslmode=require" \
  obv-backend
```

Vercel:

- Set project root to `backend`.
- Use an external PostgreSQL database.
- Set `DATABASE_URL`.
- Do not set `ADDR` unless needed; Vercel provides `PORT`.

After deployment:

```sh
obv login --api-url https://your-obscurenv-backend.example.com
```

Production notes:

- Put the backend behind HTTPS.
- Keep PostgreSQL private.
- Treat `DATABASE_URL` as a secret.
- Do not log request bodies.
- Back up PostgreSQL; encrypted payloads are still your env history.

## API

Protected endpoints require:

```text
Authorization: Bearer <API_TOKEN>
```

Endpoints:

```text
POST /api/v1/auth/register
POST /api/v1/auth/login
POST /api/v1/projects
POST /api/v1/env/push
GET  /api/v1/env/pull?project=my-app&environment=development
GET  /api/v1/env/list?project=my-app
```

Use the CLI for normal operation. API calls should send only encrypted payloads, never plaintext env values or passphrases.

## Security Model

- Plaintext `.env` content is read only by the CLI.
- Passphrases are used only by the CLI and are never persisted.
- Encryption uses Argon2id and AES-256-GCM.
- Backend stores encrypted payloads as opaque text.
- API tokens are hashed in the database.
- Decrypt failure exits without modifying the existing `.env`.
- `obv run` injects variables through process environment and does not write decrypted content to disk.

## Troubleshooting

Check the API:

```sh
curl -i "$OBV_API_URL/healthz"
```

Common fixes:

- `connection refused`: backend, PostgreSQL, `OBV_API_URL`, or `DATABASE_URL` is wrong.
- `project not found`: run `obv init --create` or create the project on the server.
- `decrypt failed; .env was not modified`: use the same passphrase used for `obv push`.
- CLI tries `https://localhost:8080`: run `obv login --api-url <url>` or set `OBV_API_URL`.
