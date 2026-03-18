# Mantle

Headless AI workflow automation platform — BYOK, IaC-first, enterprise-grade, open source.

## Project Tracking

- **Linear project:** https://linear.app/dvflw/project/mantle-76a8271f19e8
- **Linear team:** dvflw
- **Issue prefix:** DVFLW
- **Implementation plan:** `plans/mantle.md`

## Tech Stack

- **Language:** Go
- **Database:** Postgres (no ORM — code directly against Postgres, no premature Store interface)
- **CLI framework:** Cobra
- **Expression engine:** CEL (google/cel-go)
- **Plugin protocol:** gRPC over subprocess (HashiCorp go-plugin)
- **Config:** mantle.yaml with CLI flag and env var overrides
- **Testing:** testcontainers for integration tests against real Postgres
- **CI:** GitHub Actions (go test, go vet, golangci-lint)

## Architecture Principles

- **Single binary** — no external runtime dependencies beyond Postgres
- **IaC lifecycle** — `mantle validate` (offline) → `mantle plan` (diff) → `mantle apply` (versioned)
- **Checkpoint-and-resume** — NOT "exactly-once." External side effects cannot be guaranteed exactly-once without idempotency keys. Steps checkpoint to Postgres; crash recovery resumes from last completed step.
- **Secrets as opaque handles** — secrets are resolved by the engine at connector invocation time, never exposed as raw values in CEL expressions
- **Audit from day one** — every state-changing operation emits an audit event via the AuditEmitter interface (no-op in V1, Postgres-backed in V1.1)
- **Single-tenant in V1** — no auth, no teams. Multi-tenancy and RBAC added in Phase 6 as a retrofit.

## Project Structure (target)

```
cmd/mantle/          CLI entrypoint (Cobra commands)
internal/
  config/            Config file loading, env var overrides
  engine/            Step execution loop, checkpoint logic
  workflow/          YAML parser, JSON Schema validation, CEL evaluation
  connector/         Built-in connectors (HTTP, AI)
  secret/            Credential storage, encryption, opaque handle resolution
  api/               REST API server
  audit/             Audit event emission interface
charts/mantle/       Helm chart
plans/               Implementation plans
```

## Key Commands

```bash
# Local dev
docker-compose up -d          # Start Postgres
make migrate                  # Run migrations
make test                     # Unit + integration tests
make lint                     # golangci-lint
make build                    # Build binary

# Mantle CLI
mantle version                # Print version
mantle init                   # Run migrations
mantle validate workflow.yaml # Offline schema validation
mantle plan workflow.yaml     # Diff against applied version
mantle apply workflow.yaml    # Apply versioned definition
mantle run <workflow>         # Manual trigger
mantle cancel <execution-id>  # Cancel running workflow
mantle logs <execution-id>    # View execution logs
mantle status <execution-id>  # View execution state
mantle secrets create         # Create typed credential
mantle serve                  # Start persistent server (Phase 5+)
```

## V1 Phasing

1. **Core Engine & First Demoable Workflow** — scaffold, config, validate/plan/apply/run, HTTP connector, CEL, checkpointing, retry/timeout/cancel, health endpoints, CI, testing
2. **Secrets Management** — typed credentials, AES-256-GCM encryption, opaque handles, env var backend
3. **AI/LLM Connector** — OpenAI-compatible completion + structured output (no tool use until V1.1)
4. **Packaging & Distribution** — Dockerfile, Helm chart, binary releases, npm wrapper
5. **Triggers & Server Mode** — `mantle serve`, cron scheduler, webhook ingestion
6. **Multi-tenancy & RBAC** — teams, users, roles, API keys, team scoping retrofit

## License

BSL/SSPL-style — source available, no commercial resale of forks.
