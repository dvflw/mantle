# Server Guide

This guide covers running Mantle as a persistent server with automatic triggers and production deployment. For the YAML trigger syntax, see the [Workflow Reference](/docs/workflow-reference). For the `mantle serve` command flags, see the [CLI Reference](/docs/cli-reference/server-commands).

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

See [Configuration](/docs/configuration) for all configuration options.

## Graceful Shutdown

When the server receives `SIGTERM` or `SIGINT`:

1. The HTTP server stops accepting new connections
2. In-flight HTTP requests are allowed to complete
3. Running workflow executions finish their current step and checkpoint
4. The process exits with code 0

This makes Mantle safe to run behind Kubernetes, systemd, or any process manager that sends `SIGTERM` on shutdown.

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
    if: "steps['check-api'].output.status != 200"
    params:
      method: POST
      url: https://hooks.slack.com/services/T00/B00/xxx
      body:
        text: "API health check failed with status {{ steps['check-api'].output.status }}"
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

- [Triggers](/docs/server-guide/triggers) -- cron and webhook configuration
- [REST API](/docs/server-guide/api) -- API endpoints reference
- [Operations](/docs/server-guide/operations) -- backup, deployment, monitoring
