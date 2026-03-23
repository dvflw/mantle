# REST API Reference

The server exposes a REST API for triggering and managing executions programmatically. All endpoints return JSON.

## Trigger a Workflow

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

## Cancel an Execution

```
POST /api/v1/cancel/{execution}
```

Cancels a running or pending execution. Equivalent to `mantle cancel`.

**Request:**

```bash
curl -s -X POST http://localhost:8080/api/v1/cancel/abc123-def456
```

## List Executions

```
GET /api/v1/executions
```

Returns a list of recent executions with optional filtering. Supports query parameters: `workflow`, `status`, `since`, `limit`.

```bash
curl -s "http://localhost:8080/api/v1/executions?workflow=daily-report&status=completed&limit=10" | jq .
```

```json
{
  "executions": [
    {
      "id": "abc123-def456",
      "workflow": "daily-report",
      "version": 2,
      "status": "completed",
      "started_at": "2026-03-18T09:00:00Z",
      "completed_at": "2026-03-18T09:00:15Z"
    }
  ]
}
```

## Get Execution Details

```
GET /api/v1/executions/{id}
```

Returns a single execution with step-level detail.

```bash
curl -s http://localhost:8080/api/v1/executions/abc123-def456 | jq .
```

```json
{
  "id": "abc123-def456",
  "workflow": "daily-report",
  "version": 2,
  "status": "completed",
  "started_at": "2026-03-18T09:00:00Z",
  "completed_at": "2026-03-18T09:00:15Z",
  "steps": [
    {
      "name": "fetch-metrics",
      "status": "completed",
      "started_at": "2026-03-18T09:00:00Z",
      "completed_at": "2026-03-18T09:00:05Z"
    },
    {
      "name": "summarize",
      "status": "completed",
      "started_at": "2026-03-18T09:00:05Z",
      "completed_at": "2026-03-18T09:00:15Z"
    }
  ]
}
```

## Prometheus Metrics

```
GET /metrics
```

Returns Prometheus metrics in exposition format. Scrape this endpoint with Prometheus, Grafana Agent, or any compatible collector. See the [Observability Guide](/docs/observability-guide) for metric names and example PromQL queries.

## Health Endpoints

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
