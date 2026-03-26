# Server Commands

## mantle serve

Start Mantle as a persistent server with an HTTP API, cron scheduler, and webhook listener. This is the primary way to run Mantle in production.

```
Usage:
  mantle serve [flags]
```

**Flags:**

| Flag | Env Var | Default | Description |
|---|---|---|---|
| `--api-address` | `MANTLE_API_ADDRESS` | `:8080` | Listen address for the HTTP server. |

**Behavior:**

When you run `mantle serve`, Mantle:

1. Runs all pending database migrations automatically
2. Starts the HTTP API on the configured address
3. Starts the cron scheduler, polling every 30 seconds for due triggers
4. Registers webhook listener endpoints for all applied workflows with webhook triggers
5. Serves health endpoints at `/healthz` and `/readyz`

**Example:**

```bash
$ mantle serve
Running migrations...
Migrations complete.
Starting server on :8080
Cron scheduler started (poll interval: 30s)
```

**Custom address:**

```bash
$ mantle serve --api-address :9090
Starting server on :9090
```

**Graceful shutdown:**

Mantle shuts down gracefully on `SIGTERM` or `SIGINT`. When the signal is received:

1. The HTTP server stops accepting new connections
2. In-flight requests are allowed to complete
3. Running workflow executions finish their current step and checkpoint
4. The process exits with code 0

```bash
$ mantle serve
Starting server on :8080
^C
Shutting down gracefully...
Server stopped.
```

**REST API endpoints:**

The server exposes these API endpoints:

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/run/{workflow}` | Trigger a workflow execution. Returns the execution ID, workflow name, and version. |
| `POST` | `/api/v1/cancel/{execution}` | Cancel a running execution. |
| `POST` | `/hooks/<path>` | Webhook trigger endpoint. The request body is available as `trigger.payload` in the workflow's inputs. |
| `GET` | `/healthz` | Liveness probe. Returns 200 when the process is running. |
| `GET` | `/readyz` | Readiness probe. Returns 200 when the database connection is healthy and migrations are applied. |

**Example -- trigger a workflow via the API:**

```bash
$ curl -s -X POST http://localhost:8080/api/v1/run/fetch-and-summarize | jq .
{
  "execution_id": "abc123-def456",
  "workflow": "fetch-and-summarize",
  "version": 2
}
```

**Example -- cancel an execution via the API:**

```bash
$ curl -s -X POST http://localhost:8080/api/v1/cancel/abc123-def456
```

**Example -- trigger a webhook:**

```bash
$ curl -s -X POST http://localhost:8080/hooks/my-workflow \
    -H "Content-Type: application/json" \
    -d '{"event": "deploy", "repo": "my-app"}'
```

Requires a database connection. See the [Server Guide](/docs/server-guide) for production deployment guidance.

---

## REST API

The `mantle serve` command exposes a REST API for programmatic access to Mantle. All endpoints return JSON.

### Endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/run/{workflow}` | Trigger a workflow execution. |
| `POST` | `/api/v1/cancel/{execution}` | Cancel a running execution. |
| `GET` | `/api/v1/executions` | List executions with optional filters. |
| `GET` | `/api/v1/executions/{id}` | Get execution details with step information. |
| `POST` | `/hooks/{path}` | Webhook trigger endpoint. |
| `GET` | `/healthz` | Liveness probe. |
| `GET` | `/readyz` | Readiness probe (checks database connectivity). |
| `GET` | `/metrics` | Prometheus metrics endpoint. |

### POST /api/v1/run/{workflow}

Triggers a new execution of the named workflow using the latest applied version.

```bash
curl -s -X POST http://localhost:8080/api/v1/run/fetch-and-summarize | jq .
```

```json
{
  "execution_id": "abc123-def456",
  "workflow": "fetch-and-summarize",
  "version": 2
}
```

### POST /api/v1/cancel/{execution}

Cancels a running or pending execution.

```bash
curl -s -X POST http://localhost:8080/api/v1/cancel/abc123-def456
```

```json
{
  "execution_id": "abc123-def456",
  "status": "cancelled"
}
```

### GET /api/v1/executions

Lists recent executions. Supports query parameters for filtering.

**Query Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `workflow` | string | Filter by workflow name. |
| `status` | string | Filter by status: `pending`, `running`, `completed`, `failed`, `cancelled`. |
| `since` | string | Show executions within this duration (e.g., `1h`, `24h`, `7d`). |
| `limit` | number | Maximum results. Default: `20`. |

```bash
curl -s "http://localhost:8080/api/v1/executions?workflow=hello-world&status=completed&limit=5" | jq .
```

```json
{
  "executions": [
    {
      "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "workflow": "hello-world",
      "version": 1,
      "status": "completed",
      "started_at": "2026-03-18T14:30:00Z",
      "completed_at": "2026-03-18T14:30:01Z"
    }
  ]
}
```

### GET /api/v1/executions/{id}

Returns detailed information about a single execution, including all step results.

```bash
curl -s http://localhost:8080/api/v1/executions/a1b2c3d4-e5f6-7890-abcd-ef1234567890 | jq .
```

```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "workflow": "hello-world",
  "version": 1,
  "status": "completed",
  "started_at": "2026-03-18T14:30:00Z",
  "completed_at": "2026-03-18T14:30:01Z",
  "steps": [
    {
      "name": "fetch",
      "status": "completed",
      "started_at": "2026-03-18T14:30:00Z",
      "completed_at": "2026-03-18T14:30:01Z"
    }
  ]
}
```
