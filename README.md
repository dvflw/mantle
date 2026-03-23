# Mantle

[![CI](https://github.com/dvflw/mantle/actions/workflows/ci.yml/badge.svg)](https://github.com/dvflw/mantle/actions/workflows/ci.yml)
[![Go 1.25+](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: BSL 1.1](https://img.shields.io/badge/License-BSL_1.1-blue)](LICENSE)

**Define AI workflows as YAML. Deploy like infrastructure.**

Terraform for workflow automation, with native AI. Single binary. Postgres only. Bring your own keys.

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
    credential: my-openai-key
    params:
      model: gpt-4o
      prompt: "Summarize: {{ steps['fetch-data'].output.body }}"
      output_schema:
        type: object
        properties:
          summary:
            type: string
          key_points:
            type: array
            items:
              type: string
```

## Why Mantle?

- **IaC Lifecycle** -- `validate` / `plan` / `apply` / `run`. Version every workflow definition. Pin executions to immutable versions. Diff before you deploy.
- **Single Binary** -- One Go binary, one Postgres database. No message queues, no worker fleets, no cluster topology.
- **BYOK (Bring Your Own Keys)** -- Your API keys live in your database, encrypted with your encryption key. Mantle never proxies through a hosted service. OpenAI, Anthropic, Bedrock, Azure, or self-hosted.
- **AI Tool Use** -- Multi-turn function calling out of the box. The LLM requests tools, Mantle executes them via connectors, feeds results back. Crash recovery included.
- **Checkpoint and Resume** -- Steps checkpoint to Postgres after completion. Crash mid-workflow? Restart picks up from the last completed step.
- **Secrets as Opaque Handles** -- Credentials are AES-256-GCM encrypted at rest and resolved at connector invocation time. Never exposed in expressions, logs, or step outputs.

[How Mantle compares to Temporal, n8n, LangChain, Airflow, and others.](/docs/comparison)

## Quick Start

```bash
# 1. Install
go install github.com/dvflw/mantle/cmd/mantle@latest

# 2. Start Postgres and initialize
docker-compose up -d
mantle init

# 3. Apply your first workflow
mantle apply examples/hello-world.yaml

# 4. Run it
mantle run hello-world
```

17 example workflows are included in [`examples/`](examples/). See the [Getting Started guide](/docs/getting-started) for a full walkthrough.

## Connectors

9 built-in connector actions ship with the binary:

| Connector | Actions | Description |
|-----------|---------|-------------|
| HTTP | `http/request` | REST APIs, webhooks, any HTTP endpoint |
| AI (OpenAI) | `ai/completion` | Chat completions, structured output, tool use |
| AI (Bedrock) | `ai/completion` | AWS Bedrock models with region routing |
| Slack | `slack/send`, `slack/history` | Send messages, read channel history |
| Email | `email/send` | Send via SMTP, plaintext and HTML |
| Postgres | `postgres/query` | Parameterized SQL against external databases |
| S3 | `s3/put`, `s3/get`, `s3/list` | Put, get, list objects (S3-compatible) |

Need something else? Write a [plugin](/docs/plugins). Any executable that reads JSON from stdin and writes JSON to stdout -- Python, Rust, Node, Bash.

## CLI Reference

### Workflow Lifecycle

| Command | Description |
|---------|-------------|
| `mantle validate <file>` | Offline schema validation with line-number errors |
| `mantle plan <file>` | Show diff of what will change |
| `mantle apply <file>` | Store versioned workflow definition |
| `mantle run <workflow>` | Trigger execution (pinned to current version) |
| `mantle cancel <id>` | Cancel a running execution |
| `mantle logs <id>` | View step-by-step execution history |
| `mantle status <id>` | View current execution state |

### Server and Triggers

| Command | Description |
|---------|-------------|
| `mantle serve` | Start persistent server with cron and webhook triggers |

### Authentication and Secrets

| Command | Description |
|---------|-------------|
| `mantle secrets create` | Create an encrypted credential |
| `mantle secrets list` | List stored credentials |
| `mantle secrets delete <name>` | Delete a credential |
| `mantle login` | Authenticate via API key, auth code PKCE, or device flow |
| `mantle logout` | Clear cached credentials |

### Administration

| Command | Description |
|---------|-------------|
| `mantle init` | Run database migrations |
| `mantle migrate [status\|down]` | Migration management (status, rollback) |
| `mantle teams` | Manage teams |
| `mantle users` | Manage users |
| `mantle roles assign` | Assign roles (admin, team_owner, operator) |
| `mantle audit` | View audit log |
| `mantle plugins` | Manage plugins |
| `mantle library` | Shared workflow library |
| `mantle cleanup` | Clean up old executions and artifacts |
| `mantle version` | Print version info |

See the [CLI Reference](/docs/cli-reference) for full usage and flags.

## Configuration

Mantle reads configuration from `mantle.yaml`, environment variables, and CLI flags. A config file is optional -- sensible defaults work with the docker-compose setup out of the box.

```yaml
database:
  url: postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable

api:
  address: ":8080"

log:
  level: info
```

Override precedence (highest to lowest): CLI flags > environment variables (`MANTLE_` prefix) > config file > defaults.

## Architecture

```
cmd/mantle/          CLI entrypoint (Cobra)
internal/
  cli/               Command definitions
  config/            Config loading, env var overrides
  engine/            Step execution, checkpoint logic, DAG scheduler
  workflow/          YAML parsing, JSON Schema validation, CEL
  connector/         Built-in connectors (HTTP, AI, Slack, Email, Postgres, S3)
  plugin/            JSON stdin/stdout plugin protocol
  secret/            Credential storage, AES-256-GCM encryption, cloud backends
  auth/              API key and OIDC/SSO authentication, RBAC
  api/               REST API server
  server/            Server mode, cron scheduler, webhook ingestion
  db/                Database migrations and queries
  cel/               CEL expression evaluation
  library/           Shared workflow library
  metrics/           Prometheus metrics
  logging/           Structured logging
  audit/             Audit event emission
  version/           Build version info
```

Key design decisions:

- **Postgres only** -- No ORM, no premature storage abstractions. Direct SQL queries.
- **CEL expressions** -- Google's Common Expression Language for data passing between steps.
- **JSON plugins** -- Custom connectors run as subprocesses communicating via JSON over stdin/stdout.
- **Audit from day one** -- Every state-changing operation emits an audit event.

## Development

```bash
git clone https://github.com/dvflw/mantle.git
cd mantle
docker-compose up -d
make build
./mantle version
```

```bash
make build      # Build binary
make test       # Run tests
make lint       # Run golangci-lint
make run        # Run without building (go run)
make dev        # Start docker-compose services
make migrate    # Run database migrations
make clean      # Remove built binary
```

## Contributing

Contributions are welcome. Please open an issue first to discuss what you want to change. See the [Contributing guide](/docs/contributing) for details.

## License

Source-available under the [Business Source License 1.1](LICENSE). Production use is permitted; commercial resale as a workflow-as-a-service is not. Converts to Apache 2.0 on 2030-03-22.

---

*Built with YAML and conviction.*
