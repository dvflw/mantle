# Production Setup

This page covers running Mantle in production with server mode, triggers, and multi-tenancy.

## Server Mode and Triggers

So far you have been running workflows manually with `mantle run`. In production, you start Mantle as a persistent server that supports cron schedules and webhook triggers.

### Define Triggers

Add a `triggers` section to your workflow YAML:

```yaml
name: api-health-check
description: Check API health hourly and on demand

triggers:
  - type: cron
    schedule: "0 * * * *"
  - type: webhook
    path: "/hooks/api-health-check"

steps:
  - name: check-api
    action: http/request
    timeout: "10s"
    retry:
      max_attempts: 3
      backoff: exponential
    params:
      method: GET
      url: https://api.example.com/health

  - name: alert-on-failure
    action: http/request
    if: "steps['check-api'].output.status != 200"
    params:
      method: POST
      url: https://hooks.slack.com/services/T00/B00/xxx
      body:
        text: "API health check failed with status {{ steps['check-api'].output.status }}"
```

Apply the workflow, then start the server:

```bash
mantle apply api-health-check.yaml
mantle serve
```

```
Running migrations...
Migrations complete.
Starting server on :8080
Cron scheduler started (poll interval: 30s)
```

The server runs migrations on startup, starts the HTTP API on `:8080`, and polls for due cron triggers every 30 seconds. The `api-health-check` workflow now runs every hour automatically.

### Trigger a Webhook

Send a POST request to the webhook path:

```bash
curl -X POST http://localhost:8080/hooks/api-health-check \
  -H "Content-Type: application/json" \
  -d '{"reason": "manual check"}'
```

The request body is available as `trigger.payload` in CEL expressions within the workflow.

### REST API

The server also exposes a REST API for programmatic access:

```bash
# Trigger a workflow
curl -s -X POST http://localhost:8080/api/v1/run/api-health-check | jq .
```

```json
{
  "execution_id": "e5f6a7b8-c9d0-1234-efab-345678901234",
  "workflow": "api-health-check",
  "version": 1
}
```

```bash
# Cancel a running execution
curl -s -X POST http://localhost:8080/api/v1/cancel/e5f6a7b8-c9d0-1234-efab-345678901234
```

Health endpoints are available at `/healthz` (liveness) and `/readyz` (readiness, checks database connectivity). See the [Server Guide](/docs/server-guide) for production deployment, Helm chart configuration, and graceful shutdown behavior.

## Multi-Tenancy

Mantle supports teams, users, roles, and API keys for multi-tenant environments.

Create a team, add a user, and generate an API key:

```bash
mantle teams create --name acme-corp
```

```
Created team acme-corp (id: f6a7b8c9-d0e1-2345-fabc-456789012345)
```

```bash
mantle users create --email alice@acme.com --name "Alice Chen" --team acme-corp --role admin
```

```
Created user alice@acme.com (role: admin, team: acme-corp)
```

```bash
mantle users api-key --email alice@acme.com --key-name production
```

```
API Key: mk_a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2

Save this key — it cannot be retrieved again.
Key prefix for reference: mk_a1b2c3
```

Available roles are `admin`, `team_owner`, and `operator`. API keys use the `mk_` prefix and are hashed before storage -- the raw key is only shown once at creation time.

This is a brief overview. Multi-tenancy, role-based access control, and team scoping are covered in detail in the [Authentication Guide](/docs/authentication-guide).

## Next Steps

- **[Server Guide](/docs/server-guide)** -- production deployment, Helm chart, cron and webhook triggers, REST API
- **[Secrets Guide](/docs/secrets-guide)** -- credential types, encryption setup, cloud backends (AWS, GCP, Azure), and key rotation
- **[Concepts](/docs/concepts)** -- architecture, checkpointing, CEL expressions, versioning, connectors, plugins, and observability
- **[Configuration](/docs/configuration)** -- config file, environment variables, cloud backends, and flag precedence
- **[examples/](https://github.com/dvflw/mantle/tree/main/examples)** -- ready-to-run workflow files
