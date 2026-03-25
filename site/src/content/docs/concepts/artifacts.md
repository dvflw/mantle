# Artifacts

Artifacts are large files produced by workflow steps that need to be consumed by downstream steps. Instead of passing file contents through the engine's memory (which would fail for multi-gigabyte files), Mantle persists artifacts to temporary storage and passes references between steps.

## The Problem

Step outputs are stored as JSON in Postgres and passed through CEL expressions. This works well for structured data, but breaks down for large binary files like backups, archives, or generated reports. Loading a 2GB tar file into a JSON column is not viable.

## How Artifacts Work

1. A step **declares** which files it will produce using the `artifacts` field
2. The engine creates a scratch directory and makes it available to the connector
3. For `docker/run`, the scratch directory is mounted at `/mantle/artifacts` inside the container
4. After the step completes, the engine persists declared files to **tmp storage** (S3 or local filesystem)
5. Downstream steps reference artifacts via `artifacts['name']` in CEL expressions
6. After the workflow execution completes, artifacts are cleaned up based on a retention policy

## Declaring Artifacts

Add an `artifacts` list to any step:

```yaml
steps:
  - name: generate-report
    action: docker/run
    params:
      image: myorg/reporter:latest
      cmd: ["generate", "--output", "/mantle/artifacts/report.pdf"]
    artifacts:
      - path: /mantle/artifacts/report.pdf
        name: monthly-report
```

Each artifact has:
- **`path`** -- where the file is written inside the scratch directory
- **`name`** -- a unique identifier used to reference the artifact in downstream steps

Artifact names must be unique across all steps in a workflow. `mantle validate` enforces this.

## Referencing Artifacts

Downstream steps access artifacts via the `artifacts` CEL variable:

```yaml
  - name: upload-report
    action: s3/put
    params:
      bucket: reports
      key: "monthly/{{ artifacts['monthly-report'].name }}.pdf"
      content: "{{ artifacts['monthly-report'].url }}"
```

Each artifact reference provides:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | The artifact's declared name |
| `url` | string | S3 URI or local filesystem path to the persisted file |
| `size` | integer | File size in bytes |

Artifact URLs are **references**, not content. Connectors that consume artifacts (like `s3/put`) stream from the URL rather than loading the file into memory.

## Tmp Storage Configuration

Artifacts require tmp storage to be configured in `mantle.yaml`:

```yaml
# S3 (recommended for multi-node deployments)
tmp:
  type: s3
  bucket: mantle-tmp
  prefix: tmp/
  retention: 24h

# Local filesystem (single-node or development)
tmp:
  type: filesystem
  path: /var/lib/mantle/tmp
  retention: 24h
```

If a workflow declares artifacts but no tmp storage is configured, `mantle validate` warns and `mantle apply` rejects the workflow.

### Storage Layout

Files are stored at: `{prefix}/{workflow-name}/{execution-id}/{artifact-name}/{filename}`

Each execution gets its own namespace, so artifacts from concurrent executions never collide.

### Retention

Artifacts from both successful and failed executions are retained for the configured duration. This is useful for:
- **Debugging** -- inspect artifacts from failed or unexpected workflows
- **Auditing** -- verify outputs during the early stages of deploying a new workflow

Set `retention` to an empty string to disable auto-cleanup (artifacts persist until manually deleted).

## Artifact Scope

Artifacts are scoped to a single workflow execution:
- A step can only access artifacts produced by steps that have already completed in the same execution
- Referencing an artifact from a step that was skipped (via `if` condition) is a runtime error
- Artifacts are not shared across executions -- each run is fully isolated

## Any Connector Can Produce Artifacts

While `docker/run` is the most common artifact producer (the scratch directory is mounted at `/mantle/artifacts`), any connector can produce artifacts. When a step declares artifacts, the engine provides a scratch directory path via context. Connector implementations check for this path and write files there.

## Limitations

- Artifact paths must be directly under `/mantle/artifacts/` (no subdirectories)
- stdout/stderr from `docker/run` are capped at 10MB and returned as regular step output, not artifacts
- Artifacts add storage costs and I/O overhead -- use them only for files too large for step output
