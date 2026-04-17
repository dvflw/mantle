# Mantle

Headless AI workflow automation platform — BYOK, IaC-first, enterprise-grade, open source.

## Project Tracking

- **Issues & milestones:** GitHub Issues (https://github.com/dvflw/mantle/issues)
- **Implementation plans:** `docs/superpowers/plans/`

## Monorepo Structure

```
packages/
  engine/              Go binary (cmd/, internal/, go.mod)
  site/                Astro documentation site + examples
  helm-chart/          Helm chart
  proto/               Protobuf definitions
```

**Engine development happens in `packages/engine/`.** Run `go test ./...` and `go build ./cmd/mantle` from that directory. The root `go.work` file enables `go` commands from the repo root via workspace resolution.

**Build tools:** Nx for task orchestration (`bunx nx run engine:test`), changesets for versioning, bun as package manager.

## Tech Stack

- **Language:** Go
- **Database:** Postgres (no ORM — code directly against Postgres, no premature Store interface)
- **CLI framework:** Cobra
- **Expression engine:** CEL (google/cel-go)
- **Plugin protocol:** gRPC over subprocess (HashiCorp go-plugin)
- **Config:** mantle.yaml with CLI flag and env var overrides
- **Testing:** testcontainers for integration tests against real Postgres
- **CI:** GitHub Actions (per-package workflows with path filters)
- **Site:** Astro (static site generator)
- **Monorepo:** Nx + changesets + bun

## Architecture Principles

- **Single binary** — no external runtime dependencies beyond Postgres
- **IaC lifecycle** — `mantle validate` (offline) → `mantle plan` (diff) → `mantle apply` (versioned)
- **Checkpoint-and-resume** — NOT "exactly-once." External side effects cannot be guaranteed exactly-once without idempotency keys. Steps checkpoint to Postgres; crash recovery resumes from last completed step.
- **Secrets as opaque handles** — secrets are resolved by the engine at connector invocation time, never exposed as raw values in CEL expressions
- **Audit from day one** — every state-changing operation emits an audit event via the AuditEmitter interface

## Key Commands

```bash
# From repo root (delegates to engine)
make build                    # Build binary
make test                     # Unit + integration tests
make lint                     # golangci-lint

# From packages/engine/
cd packages/engine
docker compose up -d          # Start Postgres + MinIO
make migrate                  # Run migrations
go test ./...                 # Run tests directly

# Nx orchestration
bunx nx run engine:test
bunx nx run engine:build
bunx nx run-many --target=build --all

# Mantle CLI
mantle version                # Print version
mantle init                   # Run migrations
mantle validate workflow.yaml # Offline schema validation
mantle plan workflow.yaml     # Diff against applied version
mantle apply workflow.yaml    # Apply versioned definition
mantle run <workflow>         # Manual trigger
mantle cancel <execution-id>   # Cancel running workflow
mantle retry <execution-id>    # Retry from failed step
mantle rollback <workflow>     # Rollback to previous version
mantle logs <execution-id>    # View execution logs
mantle status <execution-id>  # View execution state
mantle secrets create         # Create typed credential
mantle secrets rotate-key     # Re-encrypt credentials with new key
mantle env create <name>      # Create a named environment from a values file
mantle env update <name>      # Replace inputs/env on an existing environment
mantle env list               # List named environments
mantle env get <name>         # Show environment details (env values redacted; --reveal to unredact, audited)
mantle env delete <name> -y   # Delete a named environment (requires --yes)
mantle run <wf> --values f.yaml   # Run with a values file (inputs + env overrides)
mantle run <wf> --env <name>      # Run against a stored named environment
mantle plan <wf> --env <name>     # Plan; appends resolved inputs/env with source
mantle serve                  # Start persistent server
```

**Override precedence (highest wins):**
`MANTLE_ENV_*` OS vars > `--input` flags > `--values` file > `--env` named environment > workflow `default` values > config `env:` section

## License

BSL/SSPL-style — source available, no commercial resale of forks.
