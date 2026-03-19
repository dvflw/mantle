# Server Guide

This guide covers running Mantle as a persistent server with automatic triggers, the REST API, and production deployment. For the YAML trigger syntax, see the [Workflow Reference](workflow-reference.md#triggers). For the `mantle serve` command flags, see the [CLI Reference](cli-reference.md#mantle-serve).

## Starting the Server

Run `mantle serve` to start Mantle as a long-running process:

```bash
mantle serve
```

The server:

1. Runs all pending database migrations
2. Starts the HTTP API, webhook listener, and health endpoints on the configured address (default `:8080`)
3. Starts the cron scheduler, polling every 30 seconds for due triggers

The server keeps running until you stop it with `SIGTERM` or `SIGINT` (Ctrl+C).

### Custom Listen Address

Use the `--api-address` flag or `MANTLE_API_ADDRESS` environment variable to change the listen address:

```bash
mantle serve --api-address :9090
```

See [Configuration](configuration.md) for all configuration options.

## Setting Up Cron Triggers

A cron trigger executes a workflow on a recurring schedule. Add a `triggers` section to your workflow YAML:

```yaml
name: daily-report
description: Generate and email a daily summary report

triggers:
  - type: cron
    schedule: "0 9 * * 1-5"

steps:
  - name: fetch-metrics
    action: http/request
    params:
      method: GET
      url: https://api.internal.com/metrics/daily

  - name: summarize
    action: ai/completion
    credential: my-openai
    params:
      model: gpt-4o
      prompt: "Summarize these metrics into 5 bullet points: {{ steps.fetch-metrics.output.body }}"
```

Apply the workflow to register the trigger:

```bash
mantle apply daily-report.yaml
# Applied daily-report version 1
```

The cron scheduler picks up the trigger the next time it polls (within 30 seconds). The workflow runs every weekday at 9 AM.

### Cron Expression Syntax

The `schedule` field uses standard 5-field cron syntax:

```
┌───────────── minute (0-59)
│ ┌───────────── hour (0-23)
│ │ ┌───────────── day of month (1-31)
│ │ │ ┌───────────── month (1-12)
│ │ │ │ ┌───────────── day of week (0-6, Sunday=0)
│ │ │ │ │
* * * * *
```

Common patterns:

| Schedule | Expression |
|---|---|
| Every minute | `* * * * *` |
| Every 5 minutes | `*/5 * * * *` |
| Every hour on the hour | `0 * * * *` |
| Daily at midnight | `0 0 * * *` |
| Weekdays at 9 AM | `0 9 * * 1-5` |
| First of every month at noon | `0 12 1 * *` |
| Every 15 minutes during business hours | `*/15 9-17 * * 1-5` |

### Updating a Cron Schedule

Edit the `schedule` field in your YAML and re-apply:

```bash
# Change from every 5 minutes to every 15 minutes
mantle apply daily-report.yaml
# Applied daily-report version 2
```

The scheduler picks up the updated schedule on the next poll cycle.

### Removing a Cron Trigger

Delete the `triggers` section from the YAML (or remove the specific trigger entry) and re-apply:

```bash
mantle apply daily-report.yaml
# Applied daily-report version 3
```

The trigger is deregistered. The workflow is still available for manual execution with `mantle run` or the REST API.

## Setting Up Webhook Triggers

A webhook trigger executes a workflow when an HTTP POST request arrives at a configured path. The request body is available as `trigger.payload` in CEL expressions.

```yaml
name: deploy-notifier
description: Post a Slack notification when a deploy completes

triggers:
  - type: webhook
    path: "/hooks/deploy-notifier"

steps:
  - name: notify-slack
    action: http/request
    params:
      method: POST
      url: https://hooks.slack.com/services/T00/B00/xxx
      body:
        text: "Deployed {{ trigger.payload.repo }}@{{ trigger.payload.sha }} to {{ trigger.payload.environment }}"
```

Apply the workflow:

```bash
mantle apply deploy-notifier.yaml
# Applied deploy-notifier version 1
```

Trigger it from your CI pipeline or any HTTP client:

```bash
curl -X POST http://localhost:8080/hooks/deploy-notifier \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "my-app",
    "sha": "abc1234",
    "environment": "production"
  }'
```

The server starts a new execution and the full JSON body is available as `trigger.payload`.

### Accessing Webhook Payload Data

The `trigger.payload` variable contains the parsed JSON body. Access nested fields with dot notation in template strings or bracket notation in `if` expressions:

```yaml
# In template strings (params):
url: "{{ trigger.payload.callback_url }}"
prompt: "Analyze this event: {{ trigger.payload }}"

# In if expressions:
if: "trigger.payload.action == 'opened'"
```

## Calling the REST API

The server exposes a REST API for triggering and managing executions programmatically.

### Trigger a Workflow

```
POST /api/v1/run/{workflow}
```

Starts a new execution of the named workflow using the latest applied version.

**Request:**

```bash
curl -s -X POST http://localhost:8080/api/v1/run/fetch-and-summarize | jq .
```

**Response:**

```json
{
  "execution_id": "abc123-def456",
  "workflow": "fetch-and-summarize",
  "version": 2
}
```

### Cancel an Execution

```
POST /api/v1/cancel/{execution}
```

Cancels a running or pending execution. Equivalent to `mantle cancel`.

**Request:**

```bash
curl -s -X POST http://localhost:8080/api/v1/cancel/abc123-def456
```

### Health Endpoints

| Endpoint | Purpose | Returns 200 When |
|---|---|---|
| `GET /healthz` | Liveness probe | The process is running |
| `GET /readyz` | Readiness probe | The database connection is healthy and migrations are applied |

```bash
curl http://localhost:8080/healthz
# 200 OK

curl http://localhost:8080/readyz
# 200 OK (when database is connected)
```

## Graceful Shutdown

When the server receives `SIGTERM` or `SIGINT`:

1. The HTTP server stops accepting new connections
2. In-flight HTTP requests are allowed to complete
3. Running workflow executions finish their current step and checkpoint
4. The process exits with code 0

This makes Mantle safe to run behind Kubernetes, systemd, or any process manager that sends `SIGTERM` on shutdown.

## Production Deployment

### Using the Helm Chart

The recommended way to run Mantle in production is with the included Helm chart. It configures health probes, resource limits, and environment variables:

```bash
helm install mantle charts/mantle \
  --set database.url="postgres://mantle:secret@db.internal:5432/mantle?sslmode=require" \
  --set encryption.key="your-hex-key"
```

The Helm chart configures:

- Liveness probe pointing to `/healthz`
- Readiness probe pointing to `/readyz`
- `SIGTERM` as the termination signal (aligns with Mantle's graceful shutdown)

### Health Probes

Configure your load balancer or orchestrator to use the health endpoints:

| Probe | Endpoint | Recommended Interval |
|---|---|---|
| Liveness | `GET /healthz` | 10s |
| Readiness | `GET /readyz` | 5s |

The readiness probe returns a non-200 status when the database connection is lost, which causes the load balancer to stop routing traffic to the unhealthy instance.

### Environment Variables

In production, pass configuration through environment variables rather than config files:

```bash
export MANTLE_DATABASE_URL="postgres://mantle:secret@db.internal:5432/mantle?sslmode=require"
export MANTLE_API_ADDRESS=":8080"
export MANTLE_ENCRYPTION_KEY="your-64-char-hex-key"
export MANTLE_LOG_LEVEL="warn"
mantle serve
```

See [Configuration](configuration.md) for the full list of environment variables.

### Migrations

The server runs migrations automatically on startup. You do not need a separate `mantle init` step in your deployment pipeline. This is safe to run with multiple replicas -- migrations use database-level locking to prevent conflicts.

## Example: Workflow with Both Cron and Webhook Triggers

This workflow monitors an API endpoint hourly and can also be triggered on demand by an external system:

```yaml
name: api-health-check
description: Check API health and alert on failures

triggers:
  - type: cron
    schedule: "0 * * * *"
  - type: webhook
    path: "/hooks/api-health-check"

steps:
  - name: check-api
    action: http/request
    timeout: 10s
    retry:
      max_attempts: 3
      backoff: exponential
    params:
      method: GET
      url: https://api.example.com/health

  - name: alert-on-failure
    action: http/request
    if: "steps['check-api'].output.status_code != 200"
    params:
      method: POST
      url: https://hooks.slack.com/services/T00/B00/xxx
      body:
        text: "API health check failed with status {{ steps.check-api.output.status_code }}"
```

Apply the workflow and start the server:

```bash
mantle apply api-health-check.yaml
mantle serve
```

The workflow runs every hour automatically. You can also trigger it immediately:

```bash
# Via the REST API
curl -X POST http://localhost:8080/api/v1/run/api-health-check

# Via the webhook endpoint
curl -X POST http://localhost:8080/hooks/api-health-check
```

## Further Reading

- [Workflow Reference](workflow-reference.md#triggers) -- trigger YAML syntax
- [CLI Reference](cli-reference.md#mantle-serve) -- `mantle serve` command flags
- [Configuration](configuration.md) -- all configuration options
- [Concepts](concepts.md#triggers-and-server-mode) -- architectural overview of triggers
