# Plan: Mantle — Headless AI Workflow Automation Platform

> Source PRD: Mantle project spec (grill-me session, 2026-03-18). Linear project: https://linear.app/dvflw/project/mantle-76a8271f19e8
>
> Revised 2026-03-18 based on backend architect and software architect reviews.

## Architectural decisions

Durable decisions that apply across all phases:

- **Runtime**: Go single binary, Postgres-only (no premature Store interface — code directly against Postgres; if a different backend is needed later, the data layer will be rewritten anyway since access patterns differ fundamentally)
- **Data models**: `WorkflowDefinition` (versioned), `WorkflowExecution`, `StepExecution`, `Credential`, `AuditEvent`, `SharedWorkflow`. Multi-tenancy models (`Team`, `User`, `Role`, `APIKey`) added in Phase 6.
- **Versioning**: `workflow_definitions` has a `version` column; executions pin to the version that started them
- **Execution guarantees**: Checkpoint-and-resume (NOT "exactly-once" — external side effects cannot be guaranteed exactly-once without idempotency keys at the connector level). Steps checkpoint to Postgres after completion; crash recovery resumes from last completed step. Completed steps are not re-executed.
- **Work queue (V1.1)**: `step_executions` rows with status `pending`, claimed via `SELECT ... FOR UPDATE SKIP LOCKED` with a claim/lease/reaper model: claim the row, set status to `running` with a lease timestamp, commit, then execute outside the transaction. A reaper goroutine reclaims steps stuck in `running` beyond the lease timeout.
- **API surface**: REST (JSON). Single-tenant in V1 (no team namespacing). Multi-tenant routing added in Phase 6.
- **CLI commands**: `version`, `init`, `validate`, `plan`, `apply`, `run`, `cancel`, `logs`, `status`, `secrets`, `serve`. Multi-tenancy commands (`teams`, `users`, `roles`) added in Phase 6. `library`, `plugins` added in V1.1.
- **Expression engine**: CEL (google/cel-go). Context exposes `steps.<name>.output`, `inputs.<name>`, `env.<name>`. Secrets are resolved as **opaque handles** by the engine at connector invocation time — they are NOT exposed as raw values in CEL expressions to prevent exfiltration. CEL evaluation has resource limits (max eval time, max output size).
- **Plugin protocol**: gRPC over subprocess (HashiCorp go-plugin model). Connectors implement a `Connector` protobuf service with `Execute`, `Validate`, `Describe` RPCs.
- **Config system**: `mantle.yaml` config file for engine settings (Postgres connection, API listen address, plugin directory, secret backend config). CLI flags and env vars override config file values.
- **Audit event emission**: Defined as an interface from Phase 1. Every state-changing operation emits an audit event. Storage and query API for audit events comes in V1.1, but the emission hooks are built incrementally with each phase.
- **Error handling model**: Steps support retry policies (max attempts, backoff strategy, retryable error types), per-step timeouts, and workflow-level cancellation (`mantle cancel`).
- **License**: BSL/SSPL-style — source available, no commercial resale of forks

## Key design documents needed before implementation

1. **Step claim/execute/lease/reaper lifecycle** — detailed design for SKIP LOCKED work distribution, including poll intervals, backoff, thundering herd mitigation, orphan detection, and lease timeout handling.
2. **AI tool use execution model** — how recursive step execution (LLM calls tool → tool executes → result returns to LLM) interacts with checkpointing, multi-node distribution, and LLM non-determinism. Including: sub-step tracking, recursion limits, tool call result caching for crash recovery.
3. **Secrets security model** — opaque handle resolution, encryption at rest (algorithm, master key management), and policy for preventing secret exfiltration through expressions.

---

## Phase 1: Core Engine & First Demoable Workflow

**User stories**: As a developer, I can write a workflow in YAML, validate it, apply it, run it, and see HTTP requests execute with data passing between steps — end-to-end in under 5 minutes.

### What to build

The complete happy path from YAML to execution. Go project scaffold with CLI framework (Cobra), config system (`mantle.yaml`), Postgres connection and migrations, workflow definition format (YAML + JSON Schema), offline validation, plan/apply lifecycle, sequential step execution with CEL expressions and conditionals, checkpoint-and-resume for crash recovery, HTTP connector, manual trigger, execution logs/status CLI, step retry policies and timeouts, workflow cancellation, health endpoints. Single-tenant (no auth, no teams) — security comes later.

Also: CI pipeline (go test, go vet, golangci-lint), docker-compose.yml for local Postgres, Makefile/Taskfile for common operations, testing infrastructure (unit + integration with testcontainers), audit event emission interface (no storage yet), and example workflows.

### Acceptance criteria

- [ ] `mantle version` prints version info
- [ ] `mantle init` runs Postgres migrations and creates the schema
- [ ] `mantle.yaml` config file for Postgres connection and API listen address
- [ ] docker-compose.yml starts Postgres for local development
- [ ] YAML workflow schema supports: name, description, typed inputs, steps with actions, CEL expressions, trigger declarations
- [ ] `mantle validate workflow.yaml` checks schema conformance offline with line-number errors
- [ ] `mantle plan workflow.yaml` shows diff of what will change
- [ ] `mantle apply workflow.yaml` stores versioned definition; "no changes" when unchanged
- [ ] `mantle run <workflow> [--input key=value]` triggers execution pinned to current version
- [ ] Steps execute sequentially with CEL expression data passing (`steps.<name>.output`, `inputs.<name>`)
- [ ] Conditional steps (`if:`) skip when false, recorded as `skipped`
- [ ] HTTP connector: GET/POST/PUT/DELETE with configurable URL, headers, body, timeout
- [ ] Step-level checkpointing: crash mid-workflow → restart → resumes from last completed step
- [ ] Step retry policies: configurable max attempts, backoff, retryable error types
- [ ] Per-step timeout configuration
- [ ] `mantle cancel <execution-id>` cancels a running workflow
- [ ] `mantle logs <execution-id>` shows step-by-step history with timing/status/outputs
- [ ] `mantle status <execution-id>` shows current state
- [ ] In-flight executions pinned to their original definition version
- [ ] `/healthz` and `/readyz` endpoints for container probes
- [ ] Audit event emission interface defined (events emitted but not yet stored/queryable)
- [ ] CI pipeline: `go test`, `go vet`, `golangci-lint` on every push
- [ ] Integration tests for checkpoint/recovery using testcontainers
- [ ] Example workflow demonstrating multi-step HTTP + CEL chaining

---

## Phase 2: Secrets Management

**User stories**: As a developer, I can securely store API credentials and reference them in workflows so that secrets are never exposed in definition files or logs.

### What to build

Typed credential objects with schema validation per type (e.g., type `openai` requires `api_key`, optional `org_id`). Credentials encrypted at rest in Postgres (AES-256-GCM with configurable master key source). Env var backend as initial secret source. `mantle secrets` CLI for CRUD. Secrets resolved as opaque handles at connector invocation time — NOT available as raw values in CEL. This prevents exfiltration via expressions.

### Acceptance criteria

- [ ] Credential types define required and optional fields with validation
- [ ] Credentials encrypted at rest (AES-256-GCM); master key configurable via env var or config
- [ ] `mantle secrets create --type openai` prompts for required fields, validates schema
- [ ] `mantle secrets list` shows names and types (never values)
- [ ] `mantle secrets delete` removes a credential
- [ ] Env vars (e.g., `MANTLE_SECRET_OPENAI_KEY`) can serve as credential values
- [ ] Secrets referenced in workflow YAML as opaque handles, resolved by engine at connector invocation
- [ ] Secrets never appear in execution logs or step outputs
- [ ] Credential backend abstracted for future cloud secret store support
- [ ] HTTP connector can use credentials for auth headers

---

## Phase 3: AI/LLM Connector

**User stories**: As a developer, I can build AI-powered workflows using my own API keys, with the LLM returning structured data that flows into subsequent steps.

### What to build

AI/LLM connector supporting OpenAI-compatible APIs with text completion and structured output (JSON schema enforcement). Tool use is deferred to V1.1 — it requires a separate design document for recursive step execution. The connector uses the `LLMProvider` Go interface for future multi-provider support.

### Acceptance criteria

- [ ] `action: ai/completion` with `provider`, `model`, `prompt`, `system_prompt` fields
- [ ] Works with OpenAI API and any OpenAI-compatible endpoint (Ollama, vLLM, etc.)
- [ ] `output_schema` field enforces JSON schema on LLM responses
- [ ] Structured output parsed and available as typed data in subsequent steps
- [ ] `LLMProvider` Go interface enables future provider additions
- [ ] Credentials resolved via secrets system (Phase 2)
- [ ] End-to-end workflow: HTTP fetch → AI summarize with structured output → HTTP post

---

## Phase 4: Packaging & Distribution

**User stories**: As a developer or operator, I can install Mantle via a single binary download, Docker, or Helm chart and have it running in minutes.

### What to build

Dockerfile (multi-stage, minimal image), Helm chart for Kubernetes, CI pipeline for Go binary releases across platforms, npm wrapper package. This ships early so people can actually try the product.

### Acceptance criteria

- [ ] Multi-stage Dockerfile: Go build + distroless/alpine runtime
- [ ] `helm install mantle ./charts/mantle` deploys a working instance
- [ ] Helm chart configurable: Postgres connection, resource limits, service account, liveness/readiness probes
- [ ] Go binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
- [ ] CI builds on tag push (GitHub Actions)
- [ ] npm package: `npx mantle` works
- [ ] Quick-start guide: from zero to running workflow in under 5 minutes

---

## Phase 5: Triggers & Server Mode

**User stories**: As a developer, I can configure workflows to run on a schedule or in response to webhooks, with the engine running as a persistent service.

### What to build

The `mantle serve` command that runs the engine as a persistent process (API server + cron scheduler + webhook listener). Cron trigger evaluates schedule definitions and creates executions. Webhook trigger receives HTTP requests and matches to registered workflows. This is an architectural shift from CLI-driven execution to a long-running server.

### Acceptance criteria

- [ ] `mantle serve` starts the engine as a persistent process
- [ ] Workflow YAML supports `trigger: { type: cron, schedule: "*/5 * * * *" }`
- [ ] Workflow YAML supports `trigger: { type: webhook, path: "/hooks/my-workflow" }`
- [ ] Cron-triggered workflows execute on schedule
- [ ] Webhook POST triggers execution; body available as `trigger.payload` in CEL
- [ ] Webhook returns execution ID in response
- [ ] `mantle apply` updates trigger registrations (cron schedules, webhook paths) automatically
- [ ] `mantle plan` shows trigger changes in diff output
- [ ] Graceful shutdown: in-flight executions complete before process exits

---

## Phase 6: Multi-tenancy & RBAC

**User stories**: As an admin, I can create teams, assign users and roles, and enforce access control so that multiple teams share one Mantle installation securely.

### What to build

Retrofit multi-tenancy across all existing features. Teams, users, roles (Admin / Team Owner / Operator), API key authentication. All workflow definitions, executions, and credentials scoped by team. RBAC middleware on all API endpoints. This is intentionally last in V1 — the core product is proven before adding organizational complexity.

### Acceptance criteria

- [ ] `mantle teams create/list/delete` manages teams
- [ ] `mantle users create/list/delete` manages users within teams
- [ ] `mantle roles assign` assigns Admin / Team Owner / Operator roles
- [ ] API key generation scoped to users; keys hashed at rest
- [ ] All API endpoints require valid API key (except health checks)
- [ ] RBAC: Admin has full access; Team Owner scoped to their team; Operator can view logs and trigger runs only
- [ ] 401 for invalid keys, 403 for insufficient permissions
- [ ] All existing features (workflows, secrets, executions) scoped by team
- [ ] Existing single-tenant installations migrate cleanly to multi-tenant schema

---

## Phase 7: Multi-Node Distribution (V1.1)

**User stories**: As an operator, I can run multiple Mantle replicas and workflows distribute across nodes without duplication or loss.

### What to build

Multi-node work distribution using Postgres SKIP LOCKED with the claim/lease/reaper model. Workers claim pending steps, set status to `running` with a lease timestamp (outside the execution transaction), execute, then update status. A reaper goroutine reclaims steps stuck in `running` beyond lease timeout. Requires design doc for claim/execute/lease/reaper lifecycle before implementation.

### Acceptance criteria

- [ ] Design doc: claim/lease/reaper lifecycle, poll intervals, backoff, thundering herd mitigation
- [ ] Workers claim steps via SKIP LOCKED; claim and execute are separate transactions
- [ ] Lease timeout: steps stuck in `running` beyond timeout are reclaimed by reaper
- [ ] No step lost or duplicated under normal operation with 3+ replicas
- [ ] Concurrency tests proving correctness under load with deliberate crashes
- [ ] PgBouncer documented as recommended for 10+ replica deployments
- [ ] `step_executions` table: partial index on `status = 'pending'`, archival strategy for completed executions
- [ ] Step output size limits documented; large payloads reference object storage

---

## Phase 8: AI Tool Use & Parallel Execution (V1.1)

**User stories**: As a developer, I can have LLMs call tools during workflow execution, and I can run independent steps in parallel.

### What to build

Requires design doc before implementation. AI tool use: LLM steps can invoke other actions as tools, with results fed back. Recursive execution model with sub-step tracking, recursion limits, and tool call result caching for crash recovery. Parallel step execution: fan-out/fan-in for independent steps (e.g., fetch from 3 APIs concurrently, then combine results).

### Acceptance criteria

- [ ] Design doc: recursive execution model, sub-step tracking, recursion limits, crash recovery with cached tool results
- [ ] LLM can invoke declared tools; multi-turn tool use works
- [ ] Tool calls logged in step execution record
- [ ] Recursion depth limit enforced
- [ ] Parallel steps: `parallel: true` or explicit fan-out/fan-in syntax in workflow YAML
- [ ] Parallel steps respect dependency declarations
- [ ] Parallel execution works correctly with checkpointing and multi-node distribution

---

## Phase 9: Connector Expansion & gRPC Plugins (V1.1)

**User stories**: As a developer, I can use built-in connectors for common services and write custom connectors in any language.

### What to build

Built-in connectors: Email (IMAP/SMTP), Slack, Postgres (external DB queries), S3. gRPC plugin framework (HashiCorp go-plugin pattern) for third-party connectors. Protobuf service definition, plugin lifecycle management, `mantle plugins` CLI, example plugins in Python and Go.

### Acceptance criteria

- [ ] Email connector: SMTP send, IMAP read/filter
- [ ] Slack connector: send messages, read channel history
- [ ] Postgres connector: parameterized queries, structured results
- [ ] S3 connector: get/put/list (S3-compatible endpoints supported)
- [ ] Protobuf `Connector` service with `Execute`, `Validate`, `Describe` RPCs published
- [ ] Engine spawns plugin subprocesses, communicates over gRPC
- [ ] `mantle plugins list/install/remove` CLI
- [ ] Plugin crash handled gracefully (step fails, engine continues)
- [ ] Example plugins in Python and Go with scaffolding
- [ ] Cross-connector end-to-end: DB query → AI summarize → Slack message

---

## Phase 10: Enterprise Hardening (V1.1)

**User stories**: As a compliance officer, I can audit all actions. As an operator, I can monitor health via Prometheus. As an admin, I can use SSO.

### What to build

Prometheus metrics endpoint, immutable audit trail (storage + query API for events emitted since Phase 1), SSO/OIDC integration, structured JSON logging to stdout, queryable execution logs with filtering.

### Acceptance criteria

- [ ] `/metrics` endpoint: execution counts, step durations, queue depth, connector latency
- [ ] Audit trail stored in append-only table; `mantle audit` CLI with filters
- [ ] Audit log: who, what, when, target, before/after state
- [ ] OIDC configuration via `mantle.yaml`; SSO tokens accepted alongside API keys
- [ ] RBAC identical for SSO and API key auth
- [ ] Structured JSON logs to stdout with execution context
- [ ] `mantle logs` supports filtering by workflow, execution ID, status, time range
- [ ] Execution log data retention / archival policy configurable

---

## Phase 11: Cloud Secret Stores & Shared Workflow Library (V1.1)

**User stories**: As an operator, I can use cloud secret managers. As a team lead, I can share workflow templates across teams.

### What to build

Secret store backends for AWS Secrets Manager, GCP Secret Manager, Azure Key Vault. AWS IAM for Bedrock. Shared workflow library: publish templates, browse, deploy with variable bindings.

### Acceptance criteria

- [ ] AWS Secrets Manager, GCP Secret Manager, Azure Key Vault backends
- [ ] AWS IAM authentication for Bedrock (no explicit API key)
- [ ] Secret rotation picked up without redeployment
- [ ] `mantle library publish/list/deploy` CLI
- [ ] Templates visible to all teams; deployed copies are independent and team-scoped
- [ ] RBAC: Team Owner/Admin can publish and deploy; Operator can browse only
