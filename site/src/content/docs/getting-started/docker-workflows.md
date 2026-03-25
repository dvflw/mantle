# Docker Workflows

Mantle can run Docker containers as workflow steps using the `docker/run` connector. This lets you use any containerized tool -- data processors, backup utilities, custom scripts -- as part of your automation pipeline.

## Prerequisites

- Docker daemon running on the same host as Mantle (or a remote Docker daemon accessible via TCP)
- A `docker` credential (optional for local socket access)

## Your First Docker Workflow

Create `docker-hello.yaml`:

```yaml
name: docker-hello
description: Run a simple container and capture its output

steps:
  - name: greet
    action: docker/run
    timeout: "30s"
    params:
      image: alpine:latest
      cmd: ["echo", "Hello from Docker!"]
      pull: missing
```

Apply and run:

```bash
mantle validate docker-hello.yaml
mantle apply docker-hello.yaml
mantle run docker-hello
```

The step output includes:

```json
{
  "exit_code": 0,
  "stdout": "Hello from Docker!\n",
  "stderr": ""
}
```

## Passing Data with Stdin

Pipe data into a container using the `stdin` param. This example uses `jq` inside a container to transform JSON:

```yaml
name: docker-transform
description: Transform data using a containerized tool

steps:
  - name: fetch-data
    action: http/request
    params:
      method: GET
      url: "https://jsonplaceholder.typicode.com/users"

  - name: extract-emails
    action: docker/run
    timeout: "30s"
    params:
      image: giantswarm/tiny-tools
      cmd: ["jq", "[.[].email]"]
      stdin: "{{ steps['fetch-data'].output.body }}"
      memory: "128m"
```

The container receives the HTTP response as stdin and outputs the transformed result to stdout.

## Environment Variables

Pass configuration to containers via `env`:

```yaml
- name: process
  action: docker/run
  params:
    image: myorg/processor:latest
    cmd: ["process", "--verbose"]
    env:
      INPUT_FORMAT: "json"
      OUTPUT_FORMAT: "csv"
      MAX_ROWS: "1000"
```

## Resource Limits

Constrain container resources to prevent runaway processes:

```yaml
- name: heavy-computation
  action: docker/run
  timeout: "5m"
  params:
    image: myorg/cruncher:latest
    memory: "512m"
    cpus: 2.0
```

## Branching on Exit Code

Non-zero exit codes do **not** fail the step. This lets you use the exit code for conditional logic:

```yaml
steps:
  - name: check
    action: docker/run
    params:
      image: alpine
      cmd: ["sh", "-c", "test -f /data/ready && echo ready || exit 1"]
      mounts:
        - source: "my-volume"
          target: "/data"
          readonly: true

  - name: on-ready
    action: slack/send
    credential: slack-token
    if: "steps['check'].output.exit_code == 0"
    params:
      channel: "#notifications"
      text: "Data is ready: {{ steps['check'].output.stdout }}"

  - name: on-not-ready
    action: slack/send
    credential: slack-token
    if: "steps['check'].output.exit_code != 0"
    params:
      channel: "#notifications"
      text: "Data not ready yet"
```

## Private Images

For private registries, create credentials and reference them on the step:

```bash
# Docker daemon credential (for local socket, fields are optional)
mantle secrets create my-docker --type docker

# Registry credential for pulling private images
mantle secrets create my-registry --type basic \
  --field username=myuser \
  --field password=mytoken
```

```yaml
- name: run-private
  action: docker/run
  credential: my-docker
  registry_credential: my-registry
  params:
    image: registry.example.com/myorg/processor:latest
    cmd: ["process"]
```

## Volume Mounts

Mount Docker volumes or host paths into containers:

```yaml
- name: backup
  action: docker/run
  params:
    image: alpine
    cmd: ["tar", "-czf", "/output/backup.tar.gz", "-C", "/data", "."]
    mounts:
      - source: "app-data"
        target: "/data"
        readonly: true
      - source: "backup-output"
        target: "/output"
```

## What's Next

- [Workflow Reference: docker/run](/docs/workflow-reference/connectors#dockerrun) -- full parameter reference
- [Secrets Guide](/docs/secrets-guide) -- creating Docker and registry credentials
- [Examples](/docs/examples) -- `docker-volume-backup.yaml` and `docker-data-transform.yaml`
