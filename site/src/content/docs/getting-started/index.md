# Getting Started

After completing this guide, you will have Mantle installed, a Postgres database running, and your first workflow executing -- all in under five minutes.

## What is Mantle?

Mantle is a headless AI workflow automation platform. You define workflows as YAML, deploy them through an infrastructure-as-code lifecycle (validate, plan, apply), and execute them against a Postgres-backed engine. It ships as a single Go binary -- bring your own API keys, bring your own database, no hosted runtime required.

## Prerequisites

You need the following installed on your machine:

- **Go 1.25+** -- [install instructions](https://go.dev/doc/install)
- **Docker and Docker Compose** -- [install instructions](https://docs.docker.com/get-docker/)
- **Make** -- included on macOS and most Linux distributions

Verify your setup:

```bash
go version    # go1.25 or later
docker --version
```

## Install and Start (< 2 minutes)

Clone the repository, start Postgres, build the binary, and run migrations:

```bash
git clone https://github.com/dvflw/mantle.git && cd mantle
docker compose up -d
make build
./mantle init
```

The `docker compose up -d` command starts Postgres 16 on `localhost:5432` with user `mantle`, password `mantle`, and database `mantle`. The `make build` command produces a single `mantle` binary in the project root. The `mantle init` command creates all required database tables.

The default database URL uses `sslmode=disable`, which is correct for local development with the provided Docker Compose setup. For production, always use `sslmode=require` or `sslmode=verify-full`:

```bash
export MANTLE_DATABASE_URL="postgres://mantle:secret@db.example.com:5432/mantle?sslmode=require"
```

See [Configuration](/configuration) for all database options.

You should see:

```
Running migrations...
Migrations complete.
```

Optionally, move the binary onto your PATH:

```bash
sudo mv mantle /usr/local/bin/
```

Verify it works:

```bash
mantle version
# mantle v0.1.0 (791fa83, built 2026-03-18T00:00:00Z)
```

## Your First Workflow (< 3 minutes)

The `examples/` directory includes several ready-to-run workflows. Start with the simplest one -- a single HTTP GET request.

Look at `examples/hello-world.yaml`:

```yaml
name: hello-world
description: Fetch a random fact from a public API — the simplest possible Mantle workflow

steps:
  - name: fetch
    action: http/request
    params:
      method: GET
      url: "https://jsonplaceholder.typicode.com/posts/1"
```

This workflow has one step: it sends a GET request to the JSONPlaceholder API and returns the response.

### Step 1: Validate

Check the workflow for structural errors. This runs offline -- no database connection required:

```bash
mantle validate examples/hello-world.yaml
```

Output:

```
hello-world.yaml: valid
```

If there are errors, Mantle reports them with file, line, and column numbers:

```
bad-workflow.yaml:1:1: error: name must match ^[a-z][a-z0-9-]*$ (name)
```

### Step 2: Apply

Store the workflow definition as a new immutable version in the database:

```bash
mantle apply examples/hello-world.yaml
```

Output:

```
Applied hello-world version 1
```

Every time you edit a workflow and re-apply, Mantle creates a new version. If the content has not changed, it tells you:

```
No changes — hello-world is already at version 1
```

You can also preview what will change before applying:

```bash
mantle plan examples/hello-world.yaml
```

```
No changes — hello-world is at version 1
```

### Step 3: Run

Execute the workflow by name:

```bash
mantle run hello-world
```

Output:

```
Running hello-world (version 1)...
Execution a1b2c3d4-e5f6-7890-abcd-ef1234567890: completed
  fetch: completed
```

### Step 4: View Logs

Inspect the execution with the execution ID from the previous step:

```bash
mantle logs a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

Output:

```
Execution: a1b2c3d4-e5f6-7890-abcd-ef1234567890
Workflow:  hello-world (version 1)
Status:    completed
Started:   2026-03-18T14:30:00Z
Completed: 2026-03-18T14:30:01Z
Duration:  1.042s

Steps:
  fetch           completed (1.0s)
```

If a step fails, the error appears below the step name:

```
Steps:
  fetch           failed (0.5s)
    error: http/request: GET https://jsonplaceholder.typicode.com/posts/1: connection refused
```

You can also get a quick status summary with `mantle status <execution-id>`.

## Next Steps

- [Data Passing](/docs/getting-started/data-passing) -- CEL expressions, conditional execution, step chaining
- [AI Workflows](/docs/getting-started/ai-workflows) -- AI connector, structured output, credentials
- [Production](/docs/getting-started/production) -- server mode, triggers, multi-tenancy
- [Workflow Reference](/docs/workflow-reference) -- complete YAML schema
- [CLI Reference](/docs/cli-reference) -- every command and flag
