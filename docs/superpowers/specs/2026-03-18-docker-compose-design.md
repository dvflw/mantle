# Design: Docker-Compose for Local Development

> Linear issue: [DVFLW-273](https://linear.app/dvflw/issue/DVFLW-273/docker-composeyml-for-local-development)
> Date: 2026-03-18

## Goal

Set up docker-compose with Postgres for local development, update Makefile with dev workflow targets, and update README with setup instructions.

## Acceptance Criteria

- `docker-compose up` starts Postgres with correct credentials
- Makefile has common operations: migrate, test, build, lint, run
- README documents local dev setup

## Files

- Create: `docker-compose.yml`
- Modify: `Makefile` — add `migrate`, `run`, `dev` targets
- Modify: `README.md` — update Development section
- Modify: `internal/config/config.go` — update default database URL to match docker-compose credentials

## docker-compose.yml

```yaml
services:
  postgres:
    image: postgres:16-alpine
    ports:
      - "5432:5432"
    environment:
      POSTGRES_USER: mantle
      POSTGRES_PASSWORD: mantle
      POSTGRES_DB: mantle
    volumes:
      - pgdata:/var/lib/postgresql/data

volumes:
  pgdata:
```

Postgres 16 Alpine for small image size. Credentials: `mantle/mantle` on database `mantle`. Named volume for data persistence across restarts.

## Config Default Update

Change the default `database.url` from:
```
postgres://localhost:5432/mantle?sslmode=disable
```
to:
```
postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable
```

This matches the docker-compose credentials so `make build && ./mantle` works out of the box against `docker-compose up`.

Update the corresponding test expectations in `internal/config/config_test.go`.

## Makefile Additions

Add to existing Makefile (which already has `build`, `test`, `lint`, `clean`):

- `migrate` — prints "No migrations yet. Run 'mantle init' when available." (placeholder for future use)
- `run` — `go run ./cmd/mantle $(ARGS)` for quick dev iteration without building
- `dev` — `docker-compose up -d` shorthand

## README Updates

Update the Development section to include:
1. Prerequisites (Go, Docker)
2. `docker-compose up -d` to start Postgres
3. `make build` / `make test` / `make lint`

Keep it brief — the README already has most of this, just needs minor cleanup.

## What's NOT Included

- Database migrations (no schema yet — comes with workflow engine issues)
- Health check wait scripts for Postgres readiness
- Additional services (Redis, message queues, etc.)
