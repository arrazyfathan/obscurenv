# Obscurenv (`obv`)

Obscurenv is a personal zero-knowledge `.env` cloud storage tool with a Go backend and a Go CLI.

Plaintext `.env` content is encrypted locally by the CLI before upload. The backend stores only opaque encrypted payloads, checksums, versions, users, projects, and token hashes.

## Repository

```text
backend/   Go + Gin REST API
cli/       Go + Cobra CLI
```

## Local Backend

```sh
docker compose up -d postgres
cd backend
go run .
```

Default backend environment:

```sh
DATABASE_URL=postgres://obv:obv@localhost:5432/obv?sslmode=disable
ADDR=:8080
```

## CLI

```sh
cd cli
go build -o ../bin/obv .
../bin/obv login --token obv_tok_example
../bin/obv init -p my-app
../bin/obv push -k "MySecretPassphrase123!" -e development
../bin/obv pull -k "MySecretPassphrase123!" -e development
../bin/obv swap production -k "MySecretPassphrase123!"
../bin/obv run -e staging -k "MySecretPassphrase123!" -- npm start
```

Set `OBV_API_URL` to override the default API URL (`https://localhost:8080`).

For local development against the Docker backend:

```sh
export OBV_API_URL=http://localhost:8080
```

## Validation

From the repository root:

```sh
go test ./backend/... ./cli/...
go vet ./backend/... ./cli/...
```

## Security Notes

- Add `.env` and `.obv.json` to project `.gitignore`.
- Do not reuse weak passphrases across sensitive projects.
- Production deployments must terminate HTTPS/TLS 1.3 before the backend.
