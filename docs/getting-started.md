# Getting Started

After completing this guide, you will have Mantle installed, a Postgres database running, and your first workflow validated and applied -- all in about five minutes.

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

## Install Mantle

Clone the repository and build the binary:

```bash
git clone https://github.com/dvflw/mantle.git
cd mantle
make build
```

This produces a single `mantle` binary in the project root. You can move it anywhere on your `PATH`:

```bash
sudo mv mantle /usr/local/bin/
```

Verify it works:

```bash
mantle version
# mantle dev (abc32ad, built 2026-03-18T00:00:00Z)
```

## Start Postgres

Mantle stores workflow definitions and execution state in Postgres. The included `docker-compose.yml` starts a Postgres 16 instance with sensible defaults:

```bash
docker-compose up -d
```

This starts Postgres on `localhost:5432` with:
- **User:** `mantle`
- **Password:** `mantle`
- **Database:** `mantle`

The default database URL is `postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable`. You do not need to configure anything if you use the provided docker-compose file.

## Initialize the Database

Run migrations to create the required tables:

```bash
mantle init
```

Expected output:

```
Running migrations...
Migrations complete.
```

You can verify migration status at any time:

```bash
mantle migrate status
```

## Write Your First Workflow

Create a file called `workflow.yaml`:

```yaml
name: fetch-and-summarize
description: Fetch data from an API and summarize it with an LLM

inputs:
  url:
    type: string
    description: URL to fetch

steps:
  - name: fetch-data
    action: http/request
    params:
      method: GET
      url: "{{ inputs.url }}"

  - name: summarize
    action: ai/completion
    params:
      provider: openai
      model: gpt-4o
      prompt: "Summarize this data: {{ steps.fetch-data.output.body }}"

  - name: post-result
    action: http/request
    if: "steps.summarize.output.key_points.size() > 0"
    params:
      method: POST
      url: https://hooks.example.com/results
      body:
        summary: "{{ steps.summarize.output.summary }}"
```

See the [Workflow Reference](workflow-reference.md) for a complete description of every field.

## The IaC Lifecycle: Validate, Plan, Apply

Mantle treats workflow definitions like infrastructure code. You go through a validate-plan-apply cycle to deploy changes.

### Step 1: Validate

Check your workflow for structural errors offline. No database connection required:

```bash
mantle validate workflow.yaml
```

If the workflow is valid, you see:

```
workflow.yaml: valid
```

If there are errors, Mantle reports them with file, line, and column numbers:

```
workflow.yaml:1:1: error: name must match ^[a-z][a-z0-9-]*$ (name)
```

### Step 2: Plan (coming soon)

The `plan` command will diff your local workflow file against the version stored in the database and show you what will change before you apply it. This command is not yet implemented.

### Step 3: Apply

Store the workflow definition as a new immutable version in the database:

```bash
mantle apply workflow.yaml
```

On first apply:

```
Applied fetch-and-summarize version 1
```

If you apply again without changes:

```
No changes — fetch-and-summarize is already at version 1
```

Edit the workflow and apply again, and Mantle creates version 2. Every version is preserved -- you can never lose a previous definition.

## Run a Workflow (coming soon)

The `run` command will execute a workflow by name. This command is not yet implemented:

```bash
# Coming soon
mantle run fetch-and-summarize --input url=https://api.example.com/data
```

## Next Steps

- [Workflow Reference](workflow-reference.md) -- complete YAML schema documentation
- [CLI Reference](cli-reference.md) -- every command, flag, and example
- [Configuration](configuration.md) -- config file, environment variables, and flag precedence
- [Concepts](concepts.md) -- architecture, checkpointing, CEL expressions, and connectors
