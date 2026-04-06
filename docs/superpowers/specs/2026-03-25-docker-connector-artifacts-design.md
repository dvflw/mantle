# Docker Connector & Artifact System Design

## Overview

Add a Docker connector (`docker/run`) for running containers to completion as workflow steps, and an artifact system for passing large files between steps without loading them into engine memory. Covers GitHub issues #12 (Docker Volume Backup) and #13 (Run Docker Container for Step).

## Docker Connector

### Action: `docker/run`

Runs a container to completion, captures exit code, stdout, and stderr. No long-running container management — run-to-completion only.

Uses the official Moby Go client (`github.com/docker/docker/client`) to talk to the Docker daemon via socket or TCP.

### Params

| Param | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `image` | string | yes | — | Container image |
| `cmd` | []string | no | — | Command and arguments |
| `env` | map[string]string | no | — | Environment variables |
| `stdin` | string | no | — | Data piped to container stdin |
| `mounts` | []mount | no | — | Volume/bind mounts |
| `network` | string | no | `"bridge"` | Docker network mode |
| `pull` | string | no | `"missing"` | Pull policy: `always`, `missing`, `never` |
| `memory` | string | no | — | Memory limit (e.g., `"512m"`) |
| `cpus` | float | no | — | CPU limit |
| `remove` | bool | no | `true` | Remove container on completion |

Each mount has: `source` (volume name or host path), `target` (container path), `readonly` (bool, optional).

### Output

```json
{
  "exit_code": 0,
  "stdout": "...",
  "stderr": "..."
}
```

stdout and stderr are capped at 10MB (`DefaultMaxResponseBytes`). No truncation flag — this limit is documented. The connector logs a warning when truncation occurs.

### Exit Code Semantics

A non-zero exit code does NOT constitute a step failure. The step always succeeds (unless the Docker API itself errors), and the exit code is informational in the output. Workflow authors use `if` conditions to branch on exit code. This allows subsequent steps (e.g., failure notifications) to execute regardless of the container's exit code.

### Credentials

**Docker daemon** — new `docker` credential type:

| Field | Required | Description |
|-------|----------|-------------|
| `host` | no | Defaults to `unix:///var/run/docker.sock` |
| `ca_cert` | no | TLS CA certificate |
| `client_cert` | no | TLS client certificate |
| `client_key` | no | TLS client key |

All fields optional. An empty credential means "local socket, no TLS." For remote TCP with TLS, populate all four.

**Registry auth** — uses existing `basic` credential type via `registry_credential` param on the step. Tokens go in the password field (this is the standard Docker registry API pattern).

```yaml
- name: run-private-image
  action: docker/run
  credential: my-docker
  registry_credential: my-registry
  params:
    image: myorg/processor:latest
    cmd: ["process"]
```

### Container Lifecycle

1. Pull image (per pull policy; authenticate with `registry_credential` if provided)
2. Create container with params (env, cmd, mounts, network, resource limits)
3. If `stdin` is set, attach and pipe it
4. If step declares `artifacts`, mount scratch dir at `/mantle/artifacts`
5. Start container, wait for exit
6. Capture stdout, stderr, exit code
7. If step declares `artifacts`, persist declared files to tmp storage (fail step if declared artifact path is missing)
8. Remove container (if `remove: true`)

### Timeout and Kill Chain

The step-level `timeout` controls the overall execution time. When the context is cancelled (timeout or workflow cancellation):

1. Connector calls `ContainerStop` with a 10-second grace period (SIGTERM)
2. If the container does not exit within the grace period, Docker sends SIGKILL
3. stdout/stderr captured up to the point of termination
4. Step output includes whatever was captured; exit code reflects the signal

## Artifact System

### Purpose

Allow steps to produce large files that subsequent steps can reference without loading into engine memory. Any connector can produce artifacts, not just Docker.

### Declaration

Workflow authors explicitly declare artifacts on a step:

```yaml
steps:
  - name: backup-volume
    action: docker/run
    params:
      image: alpine
      cmd: ["tar", "-czf", "/mantle/artifacts/backup.tar.gz", "-C", "/data", "."]
      mounts:
        - source: "my-volume"
          target: "/data"
          readonly: true
    artifacts:
      - path: /mantle/artifacts/backup.tar.gz
        name: backup-archive
```

### Engine Behavior

1. Step declares `artifacts` → engine creates a scratch dir, passes the path via context (using `WithArtifactsDir(ctx, path)`, following the established `WithExecutionID` pattern)
2. For `docker/run`: connector reads the artifacts dir from context and mounts it at `/mantle/artifacts` inside the container
3. For other connectors: connector reads the artifacts dir from context and writes files there
4. After step completes, engine persists declared files from scratch dir to tmp storage. If a declared artifact path is missing from the scratch dir, the step fails with an error.
5. Metadata written to `execution_artifacts` table
6. Artifacts cleaned up after retention TTL expires

### CEL Access

```
artifacts['backup-archive'].url    # S3 URI or local path
artifacts['backup-archive'].size   # bytes
artifacts['backup-archive'].name   # "backup-archive"
```

### Validation

`mantle validate` rejects duplicate artifact names across steps within a workflow. Artifact names must be unique per workflow definition.

### Artifact Access Scoping

Artifacts are available to any step that executes after the producing step completes. If a step references an artifact from a step that was skipped (via `if` condition), this is a runtime error. The engine checks artifact availability before executing a step that references artifacts in its params.

### Template Syntax Note

In the example workflows, `{{ }}` in `params` values denotes string interpolation (CEL expressions embedded in strings). The `if` field uses bare CEL expressions without delimiters. This is the existing convention in the codebase.

### Artifact-Aware Connectors

Connectors that consume artifacts (e.g., `s3/put`) need to detect when a param value is an artifact URL and stream from it instead of treating it as literal content. This avoids loading large files into memory.

## Database Schema

### `execution_artifacts` Table

```sql
CREATE TABLE execution_artifacts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id UUID NOT NULL REFERENCES workflow_executions(id),
    step_name    TEXT NOT NULL,
    name         TEXT NOT NULL,
    url          TEXT NOT NULL,
    size         BIGINT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (execution_id, name)
);
```

The unique constraint on `(execution_id, name)` enforces artifact name uniqueness at the database level.

## Tmp Storage Configuration

### Purpose

System-level config for where artifacts are persisted between steps. Supports distributed deployments (S3) and local dev (filesystem).

### Config in `mantle.yaml`

```yaml
# S3 (production / multi-pod)
tmp:
  type: s3
  bucket: mantle-tmp
  prefix: tmp/
  retention: 24h

# Filesystem (local dev / single node)
tmp:
  type: filesystem
  path: /tmp/mantle-scratch
  retention: 24h
```

### Namespacing

Files are stored at: `tmp/{workflow-name}/{execution-id}/{artifact-name}`

### Retention

- Default TTL applies to both successful and failed executions
- Configurable: operator can change duration or disable auto-cleanup
- Engine runs a reaper to clean up expired artifacts
- If no tmp storage is configured and a workflow declares artifacts, `mantle validate` warns and `mantle apply` rejects

## Example Workflows

### Docker Volume Backup (Issue #12)

```yaml
name: docker-volume-backup
description: >
  Back up a Docker volume to S3 on a daily schedule.
  Compresses the volume contents, uploads to S3, and
  sends a Slack alert on success or failure.

triggers:
  - type: cron
    schedule: "0 2 * * *"

steps:
  - name: backup-volume
    action: docker/run
    credential: my-docker
    timeout: "10m"
    params:
      image: alpine
      cmd: ["tar", "-czf", "/mantle/artifacts/backup.tar.gz", "-C", "/data", "."]
      mounts:
        - source: "my-app-data"
          target: "/data"
          readonly: true
      memory: "256m"
      remove: true
    artifacts:
      - path: /mantle/artifacts/backup.tar.gz
        name: backup-archive

  - name: upload-to-s3
    action: s3/put
    credential: aws-prod
    timeout: "5m"
    params:
      bucket: my-backups
      key: "volumes/my-app-data/{{ date }}/backup.tar.gz"
      content: "{{ artifacts['backup-archive'].url }}"
      content_type: "application/gzip"

  - name: notify-success
    action: slack/send
    credential: slack-token
    if: "steps['upload-to-s3'].output.size > 0"
    params:
      channel: "#ops-alerts"
      text: "Volume backup completed — {{ artifacts['backup-archive'].size }} bytes uploaded to s3://my-backups/volumes/my-app-data/"

  - name: notify-failure
    action: slack/send
    credential: slack-token
    if: "steps['upload-to-s3'].error != null"
    params:
      channel: "#ops-alerts"
      text: "Volume backup FAILED — {{ steps['upload-to-s3'].error }}"
```

### Data Processing with Docker (Issue #13)

```yaml
name: docker-data-transform
description: >
  Query a database for recent records, process them through
  a containerized tool, and post the results to Slack.
  Demonstrates the docker/run connector with stdin/stdout
  data passing.

steps:
  - name: fetch-records
    action: postgres/query
    credential: my-db
    timeout: "15s"
    params:
      query: "SELECT id, name, status, updated_at FROM tasks WHERE updated_at > now() - interval '24 hours' ORDER BY updated_at DESC"

  - name: transform-data
    action: docker/run
    credential: my-docker
    timeout: "30s"
    params:
      image: giantswarm/tiny-tools
      cmd: ["jq", "[.[] | {id, name, status}]"]
      stdin: "{{ steps['fetch-records'].output.json }}"
      memory: "128m"

  - name: post-results
    action: slack/send
    credential: slack-token
    if: "steps['transform-data'].output.exit_code == 0"
    params:
      channel: "#daily-report"
      text: "Daily task summary:\n{{ steps['transform-data'].output.stdout }}"
```

## Go Type Changes

### New fields on `Step` struct (`internal/workflow/workflow.go`)

```go
type Step struct {
	Name               string           `yaml:"name"`
	Action             string           `yaml:"action"`
	Params             map[string]any   `yaml:"params"`
	If                 string           `yaml:"if"`
	Retry              *RetryPolicy     `yaml:"retry"`
	Timeout            string           `yaml:"timeout"`
	Credential         string           `yaml:"credential"`
	RegistryCredential string           `yaml:"registry_credential"` // NEW: for docker/run private image pulls
	DependsOn          []string         `yaml:"depends_on"`
	Artifacts          []ArtifactDecl   `yaml:"artifacts"`           // NEW: artifact declarations
}
```

### New types

```go
// ArtifactDecl declares a file that a step will produce.
type ArtifactDecl struct {
	Path string `yaml:"path"` // path inside the artifacts dir
	Name string `yaml:"name"` // unique name for CEL reference
}

// ArtifactRef is the runtime representation available in CEL expressions.
type ArtifactRef struct {
	Name string `json:"name"`
	URL  string `json:"url"`  // S3 URI or local filesystem path
	Size int64  `json:"size"`
}
```

### New context helpers (`internal/engine/`)

```go
func WithArtifactsDir(ctx context.Context, dir string) context.Context
func ArtifactsDirFromContext(ctx context.Context) string
```

## Security Considerations

Docker socket access (`unix:///var/run/docker.sock`) grants effectively root-level access to the host. For V1:

- The `docker` credential type controls which daemon a workflow connects to
- Resource limits (`memory`, `cpus`) constrain container resource usage
- The `remove: true` default prevents container accumulation

Future considerations (out of scope for this design):
- Image allowlists to restrict which images can be pulled
- Preventing `--privileged` mode (not exposed as a param, so blocked by default)
- Mount source restrictions to prevent mounting sensitive host paths
- Network policy enforcement

## Homepage Update

Add the Docker connector to the connectors section in `site/src/components/Connectors.astro`. Update the count from "7 connectors with 9 actions" to "8 connectors with 10 actions."

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Docker SDK | Moby Go client | Official, supports socket + TCP + TLS, API version negotiation |
| Container lifecycle | Run-to-completion only | Long-running containers handled via HTTP/plugin connectors |
| Input mechanisms | env, cmd, stdin, mounts | Covers all common data-passing patterns |
| Output mechanisms | exit_code, stdout, stderr | File output handled via artifact system |
| stdout/stderr cap | 10MB | Matches `DefaultMaxResponseBytes`, prevents OOM |
| Artifact declaration | Explicit on step | IaC-first philosophy, self-documenting, no magic |
| Artifact storage | Separate `execution_artifacts` table | Clean querying and cleanup, no output bloat |
| Artifact CEL access | `artifacts['name']` (top-level) | Short, clean; uniqueness enforced at validation |
| Artifact connector interface | Context value (`WithArtifactsDir`) | Follows established `WithExecutionID` pattern, no param pollution |
| Tmp storage | System-level config in `mantle.yaml` | One config for all workflows, S3 or filesystem |
| Tmp namespacing | `tmp/{workflow}/{execution}/{artifact}` | Isolated per execution, easy cleanup |
| Tmp retention | TTL-based, configurable | Useful for both debugging and auditing |
| Artifact scope | Single execution | Clean lifecycle, no cross-execution dependencies |
| Registry auth | Existing `basic` credential type | Industry-standard pattern, tokens go in password field |
| Large file transfer | Artifact URLs, not content | Prevents loading multi-GB files into engine memory |
