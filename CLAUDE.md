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
mantle repos add <name> --url <url> --credential <cred>  # Register a GitOps source repo
mantle repos list              # List registered repos with last-sync status
mantle repos status <name>     # Show detailed repo status
mantle repos remove <name> -y  # Unregister a repo (requires --yes)
mantle run <wf> --values f.yaml   # Run with a values file (inputs + env overrides)
mantle run <wf> --env <name>      # Run against a stored named environment
mantle plan <wf> --env <name>     # Plan; appends resolved inputs/env with source
mantle serve                  # Start persistent server
```

**Override precedence (highest wins).** Inputs and env vars resolve in separate namespaces:

- **Workflow inputs** (consumed by `inputs.<name>` in CEL): `--input` flags > `--values` file `inputs:` > `--env` named-environment `inputs` > workflow definition `default`
- **Env vars** (consumed by `env.<KEY>` in CEL): `MANTLE_ENV_*` OS vars > `--values` file `env:` > `--env` named-environment `env` > config `env:` section in `mantle.yaml`

## GitOps Config

Register repos in `mantle.yaml` under `git_sync.repos`:

```yaml
git_sync:
  repos:
    - name: acme
      url: https://github.com/acme/workflows.git
      branch: main
      path: /
      poll_interval: 60s
      credential: github-pat   # must reference a secret of type: git
      auto_apply: true
      prune: true
```

Credentials of type `git` accept `token` (for HTTPS), `ssh_key` (for SSH), and optional `username`. At least one of `token` or `ssh_key` is required.

## License

BSL/SSPL-style — source available, no commercial resale of forks.
