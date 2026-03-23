# Observability Guide

Mantle provides three observability mechanisms: Prometheus metrics, an immutable audit trail, and structured JSON logging. This guide covers how to set up each one, what data is available, and how to integrate with external monitoring systems.

## Prometheus Metrics

When running in server mode (`mantle serve`), Mantle exposes a `/metrics` endpoint in Prometheus exposition format. The endpoint is available on the same address as the API server (default `:8080`).

### Scraping Metrics

Add Mantle to your Prometheus scrape configuration:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'mantle'
    scrape_interval: 15s
    static_configs:
      - targets: ['localhost:8080']
```

Verify the endpoint is working:

```bash
curl http://localhost:8080/metrics
```

### Available Metrics

| Metric | Type | Labels | Description |
|---|---|---|---|
| `mantle_workflow_executions_total` | Counter | `workflow`, `status` | Total number of workflow executions, labeled by workflow name and final status (`completed`, `failed`, `cancelled`). |
| `mantle_step_executions_total` | Counter | `workflow`, `step`, `status` | Total number of step executions, labeled by workflow name, step name, and final status. |
| `mantle_step_duration_seconds` | Histogram | `workflow`, `step`, `action` | Duration of step executions in seconds. Uses default Prometheus histogram buckets. |
| `mantle_connector_duration_seconds` | Histogram | `action` | Duration of connector invocations in seconds, labeled by action name (e.g., `http/request`, `ai/completion`). |
| `mantle_active_executions` | Gauge | -- | Number of currently running workflow executions. |

### Example PromQL Queries

**Workflow execution rate (per minute):**

```promql
rate(mantle_workflow_executions_total[5m]) * 60
```

**Failure rate for a specific workflow:**

```promql
rate(mantle_workflow_executions_total{workflow="daily-report", status="failed"}[1h])
/
rate(mantle_workflow_executions_total{workflow="daily-report"}[1h])
```

**P95 step duration for a workflow:**

```promql
histogram_quantile(0.95, rate(mantle_step_duration_seconds_bucket{workflow="daily-report"}[5m]))
```

**Average connector latency by action type:**

```promql
rate(mantle_connector_duration_seconds_sum[5m])
/
rate(mantle_connector_duration_seconds_count[5m])
```

**Currently active executions:**

```promql
mantle_active_executions
```

**Slowest connectors (P99):**

```promql
histogram_quantile(0.99, sum by (action, le) (rate(mantle_connector_duration_seconds_bucket[5m])))
```

### Grafana Dashboard

A useful Grafana dashboard for Mantle includes these panels:

1. **Execution rate** -- `rate(mantle_workflow_executions_total[5m])` stacked by workflow
2. **Failure rate** -- `rate(mantle_workflow_executions_total{status="failed"}[5m])` with alert threshold
3. **Active executions** -- `mantle_active_executions` as a stat panel
4. **Step duration heatmap** -- `mantle_step_duration_seconds_bucket` as a heatmap
5. **Connector latency** -- `mantle_connector_duration_seconds` by action as a time series

### Alerting Examples

**Alert: workflow failure rate exceeds 10%:**

```yaml
# Prometheus alerting rule
groups:
  - name: mantle
    rules:
      - alert: MantleHighFailureRate
        expr: |
          rate(mantle_workflow_executions_total{status="failed"}[15m])
          /
          rate(mantle_workflow_executions_total[15m])
          > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Workflow {{ $labels.workflow }} failure rate is {{ $value | humanizePercentage }}"
```

**Alert: AI connector latency exceeds 30 seconds:**

```yaml
      - alert: MantleSlowAIConnector
        expr: |
          histogram_quantile(0.95, rate(mantle_connector_duration_seconds_bucket{action="ai/completion"}[5m])) > 30
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "AI connector P95 latency is {{ $value }}s"
```

## Audit Trail

Every state-changing operation in Mantle emits an audit event to the `audit_events` table in Postgres. The table is append-only -- events cannot be updated or deleted. This provides a tamper-resistant record of all activity.

### What Gets Audited

| Action | Trigger | Description |
|---|---|---|
| `workflow.applied` | `mantle apply` | A new workflow version was stored in the database. |
| `workflow.executed` | `mantle run`, cron, webhook, API | A workflow execution started. |
| `step.started` | Engine | A step began executing. |
| `step.completed` | Engine | A step finished successfully. |
| `step.failed` | Engine | A step failed (error is recorded). |
| `step.skipped` | Engine | A step was skipped because its `if` condition evaluated to `false`. |
| `execution.cancelled` | `mantle cancel`, API | An execution was cancelled. |

### Audit Event Structure

Each audit event contains:

| Field | Description |
|---|---|
| `id` | Unique event identifier. |
| `timestamp` | When the event occurred (UTC). |
| `actor` | Who performed the action: `cli`, `engine`, `scheduler`, or a user ID. |
| `action` | The action type (e.g., `workflow.applied`). |
| `resource_type` | The type of resource affected (e.g., `workflow_definition`, `workflow_execution`, `step_execution`). |
| `resource_id` | The identifier of the affected resource. |
| `before_state` | Optional JSON: the state of the resource before the change. |
| `after_state` | Optional JSON: the state of the resource after the change. |
| `metadata` | Optional key-value pairs with additional context. |

### Querying Audit Events

Use the `mantle audit` command to query audit events from the command line:

```bash
# All recent events (default: last 50)
mantle audit

# Filter by action type
mantle audit --action workflow.applied

# Filter by actor
mantle audit --actor cli

# Filter by resource
mantle audit --resource workflow_definition/hello-world

# Filter by time range
mantle audit --since 24h

# Combine filters
mantle audit --action step.failed --since 7d --limit 100
```

Output format:

```
2026-03-18T14:30:00Z  cli           workflow.applied        workflow_definition/hello-world
2026-03-18T14:30:01Z  engine        workflow.executed        workflow_execution/a1b2c3d4
2026-03-18T14:30:01Z  engine        step.started            step_execution/fetch
2026-03-18T14:30:02Z  engine        step.completed          step_execution/fetch
```

### Querying Audit Events via SQL

For advanced queries, you can query the `audit_events` table directly:

```sql
-- Find all failed steps in the last 24 hours
SELECT timestamp, actor, action, resource_type, resource_id
FROM audit_events
WHERE action = 'step.failed'
  AND timestamp >= NOW() - INTERVAL '24 hours'
ORDER BY timestamp DESC;

-- Count events by action type
SELECT action, COUNT(*) as count
FROM audit_events
WHERE timestamp >= NOW() - INTERVAL '7 days'
GROUP BY action
ORDER BY count DESC;
```

### Audit Retention

Mantle does not automatically delete old audit events. In a long-running production deployment, you may want to set up a retention policy. Example using a Postgres cron job (via `pg_cron`):

```sql
-- Delete audit events older than 90 days
DELETE FROM audit_events WHERE timestamp < NOW() - INTERVAL '90 days';
```

Run this as a scheduled task outside of Mantle.

## Structured JSON Logging

In server mode, Mantle emits structured JSON logs to stdout via Go's `slog` package. Each log line is a self-contained JSON object.

### Log Format

```json
{"time":"2026-03-18T14:30:00.000Z","level":"INFO","msg":"server listening","address":":8080"}
{"time":"2026-03-18T14:30:01.000Z","level":"INFO","msg":"cron scheduler started"}
{"time":"2026-03-18T14:30:02.000Z","level":"INFO","msg":"workflow execution started","workflow":"hello-world","execution_id":"abc123"}
{"time":"2026-03-18T14:30:03.000Z","level":"ERROR","msg":"step failed","workflow":"hello-world","step":"fetch","error":"connection refused"}
```

### Log Levels

| Level | Description |
|---|---|
| `debug` | Detailed diagnostic information. Includes connector request/response details and CEL expression evaluation. |
| `info` | Normal operational events. Server startup, workflow executions, cron triggers. |
| `warn` | Abnormal situations that do not prevent operation. Shutdown timeouts, deprecated features. |
| `error` | Errors that caused a step or operation to fail. |

Configure the level with:

```bash
# CLI flag
mantle serve --log-level debug

# Environment variable
export MANTLE_LOG_LEVEL=warn

# Config file
# mantle.yaml
log:
  level: info
```

### Integration with Log Aggregation

The JSON log format integrates directly with standard log aggregation systems.

**Datadog:**

Datadog's agent automatically parses JSON logs from stdout. If running Mantle in a container, configure the Datadog agent to collect container logs:

```yaml
# datadog.yaml (agent config)
logs:
  - type: docker
    service: mantle
    source: go
```

**ELK Stack (Elasticsearch, Logstash, Kibana):**

Configure Filebeat to read Mantle's stdout (typically via container logs or a file redirect):

```yaml
# filebeat.yml
filebeat.inputs:
  - type: container
    paths:
      - /var/log/containers/mantle-*.log
    json.keys_under_root: true
    json.add_error_key: true
```

**Grafana Loki:**

Use Promtail to ship logs from Mantle containers to Loki:

```yaml
# promtail.yaml
scrape_configs:
  - job_name: mantle
    static_configs:
      - targets: [localhost]
        labels:
          job: mantle
          __path__: /var/log/mantle/*.log
    pipeline_stages:
      - json:
          expressions:
            level: level
            msg: msg
      - labels:
          level:
```

**CloudWatch Logs:**

When running on ECS or EKS, stdout logs are automatically captured by the `awslogs` log driver. No additional configuration is needed.

### Correlating Logs with Metrics and Audit Events

All three observability signals share common identifiers:

| Identifier | In Logs | In Metrics | In Audit Events |
|---|---|---|---|
| Workflow name | `workflow` field | `workflow` label | `resource_id` (for workflow resources) |
| Execution ID | `execution_id` field | -- | `resource_id` (for execution resources) |
| Step name | `step` field | `step` label | `resource_id` (for step resources) |

To trace a specific execution:

1. Find the execution in `mantle logs <execution-id>` or `GET /api/v1/executions/{id}`
2. Search structured logs for `execution_id` to see detailed step-level events
3. Query audit events with `mantle audit --resource workflow_execution/<id>` for the audit trail
4. Check Prometheus metrics for timing data with the `workflow` label

## REST API for Execution Queries

In addition to the CLI and Prometheus metrics, you can query execution data through the REST API. This is useful for building custom dashboards or integrating with external monitoring systems.

```bash
# List recent executions
curl -s "http://localhost:8080/api/v1/executions?limit=10" | jq .

# Get details for a specific execution
curl -s "http://localhost:8080/api/v1/executions/abc123-def456" | jq .

# Filter by workflow and status
curl -s "http://localhost:8080/api/v1/executions?workflow=daily-report&status=failed&since=24h" | jq .
```

See the [CLI Reference](cli-reference.md#rest-api) for the full API specification.

## Further Reading

- [CLI Reference](cli-reference.md#mantle-audit) -- `mantle audit` command and flags
- [CLI Reference](cli-reference.md#rest-api) -- REST API endpoint documentation
- [Server Guide](server-guide.md) -- running `mantle serve` with health probes and metrics
- [Concepts](concepts.md#observability) -- architectural overview of the observability stack
- [Configuration](configuration.md) -- log level and server address configuration
