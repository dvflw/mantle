# Mantle

[![CI](https://github.com/dvflw/mantle/actions/workflows/ci.yml/badge.svg)](https://github.com/dvflw/mantle/actions/workflows/ci.yml)

Headless AI workflow automation platform. Bring your own keys, define workflows as code, run them anywhere.

Mantle gives you an IaC-style lifecycle for AI-powered workflows: **validate** offline, **plan** diffs, **apply** versioned definitions, and **run** with checkpoint-and-resume reliability. Ship a single Go binary, connect to Postgres, and you're running.

## Why Mantle?

- **BYOK (Bring Your Own Keys)** — Use your own API keys for OpenAI, Anthropic, or any OpenAI-compatible endpoint. No vendor lock-in, no proxy fees.
- **IaC Lifecycle** — `validate` → `plan` → `apply` → `run`. Version every workflow definition. Pin executions to the version that started them.
- **Checkpoint & Resume** — Steps checkpoint to Postgres after completion. Crash mid-workflow? Restart picks up from the last completed step.
- **Secrets as Opaque Handles** — Credentials are encrypted at rest and resolved at connector invocation time. They never appear in expressions, logs, or step outputs.
- **Single Binary** — No external runtime dependencies beyond Postgres. Deploy anywhere containers run.
- **Extensible** — Built-in HTTP and AI connectors. gRPC plugin protocol for custom connectors in any language.

## Quick Start

### Prerequisites

- Go 1.22+
- Docker & Docker Compose (for local development)

### Install

```bash
# From source
go install github.com/dvflw/mantle/cmd/mantle@latest

# Or build locally
git clone https://github.com/dvflw/mantle.git
cd mantle
make build
```

### Run

```bash
# Start Postgres
docker-compose up -d

# Initialize the database
mantle init

# Validate a workflow
mantle validate workflow.yaml

# See what will change
mantle plan workflow.yaml

# Apply the workflow definition
mantle apply workflow.yaml

# Run it
mantle run my-workflow --input url=https://api.example.com/data
```

## Example Workflow

```yaml
name: fetch-and-summarize
description: Fetch data from an API and summarize it with an LLM

inputs:
  url:
    type: string
    description: URL to fetch

steps:
  - name: fetch-data
    action: http/request
    params:
      method: GET
      url: "{{ inputs.url }}"

  - name: summarize
    action: ai/completion
    params:
      provider: openai
      model: gpt-4o
      prompt: "Summarize this data: {{ steps.fetch-data.output.body }}"
      output_schema:
        type: object
        properties:
          summary:
            type: string
          key_points:
            type: array
            items:
              type: string

  - name: post-result
    action: http/request
    if: "steps.summarize.output.key_points.size() > 0"
    params:
      method: POST
      url: https://hooks.example.com/results
      body:
        summary: "{{ steps.summarize.output.summary }}"
        points: "{{ steps.summarize.output.key_points }}"
```

## CLI Reference

| Command | Description |
|---------|-------------|
| `mantle version` | Print version info |
| `mantle init` | Run database migrations |
| `mantle validate <file>` | Offline schema validation with line-number errors |
| `mantle plan <file>` | Show diff of what will change |
| `mantle apply <file>` | Store versioned workflow definition |
| `mantle run <workflow>` | Trigger execution (pinned to current version) |
| `mantle cancel <id>` | Cancel a running execution |
| `mantle logs <id>` | View step-by-step execution history |
| `mantle status <id>` | View current execution state |
| `mantle secrets create` | Create an encrypted credential |
| `mantle serve` | Start as a persistent server with triggers |

## Configuration

Mantle reads configuration from `mantle.yaml`, environment variables, and CLI flags (highest precedence).

```yaml
database:
  url: postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable

api:
  address: ":8080"

log:
  level: info
```

Environment variables use the `MANTLE_` prefix:

- `MANTLE_DATABASE_URL`
- `MANTLE_API_ADDRESS`
- `MANTLE_LOG_LEVEL`

## Architecture

```
cmd/mantle/          CLI entrypoint (Cobra)
internal/
  config/            Config loading, env var overrides
  engine/            Step execution, checkpoint logic
  workflow/          YAML parsing, JSON Schema validation, CEL
  connector/         Built-in connectors (HTTP, AI)
  secret/            Credential storage, encryption
  api/               REST API server
  audit/             Audit event emission
```

**Key design decisions:**

- **Postgres only** — No ORM, no premature storage abstractions. Direct SQL queries optimized for the access patterns that matter.
- **CEL expressions** — Google's Common Expression Language for data passing between steps (`steps.<name>.output`, `inputs.<name>`).
- **gRPC plugins** — Custom connectors run as subprocesses communicating over gRPC (HashiCorp go-plugin pattern).
- **Audit from day one** — Every state-changing operation emits an audit event.

## Development

### Prerequisites

- Go 1.22+
- Docker & Docker Compose

### Setup

```bash
# Clone the repo
git clone https://github.com/dvflw/mantle.git
cd mantle

# Start Postgres
docker-compose up -d

# Build
make build

# Verify
./mantle version
```

### Common Commands

```bash
make build      # Build binary with version info
make test       # Run tests
make lint       # Run golangci-lint
make run        # Run without building (go run)
make dev        # Start docker-compose services
make migrate    # Run database migrations (placeholder)
make clean      # Remove built binary
```

## Roadmap

Mantle is being built in phases:

1. **Core Engine** — CLI, config, validate/plan/apply/run, HTTP connector, CEL, checkpointing, retry/timeout
2. **Secrets** — Typed credentials, AES-256-GCM encryption, opaque handles
3. **AI Connector** — OpenAI-compatible completion + structured output
4. **Packaging** — Dockerfile, Helm chart, binary releases
5. **Triggers** — Cron schedules, webhook ingestion, `mantle serve`
6. **Multi-tenancy** — Teams, users, roles, RBAC

## License

BSL/SSPL-style — source available, no commercial resale of forks.
