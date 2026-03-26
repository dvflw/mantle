# Workflow Reference

This document describes the top-level fields and step configuration in a Mantle workflow YAML file. For connector-specific parameters, see [Connectors](/docs/workflow-reference/connectors). For AI tool use, see [Tool Use](/docs/workflow-reference/tools). For a hands-on introduction, start with the [Getting Started](/docs/getting-started) guide.

## Complete Example

```yaml
name: fetch-and-summarize
description: Fetch data from an API and summarize it with an LLM

inputs:
  url:
    type: string
    description: URL to fetch
  max_retries:
    type: number
    description: Maximum number of retries for the HTTP request

triggers:
  - type: cron
    schedule: "0 * * * *"
  - type: webhook
    path: "/hooks/fetch-and-summarize"

steps:
  - name: fetch-data
    action: http/request
    timeout: 30s
    retry:
      max_attempts: 3
      backoff: exponential
    params:
      method: GET
      url: "{{ inputs.url }}"

  - name: summarize
    action: ai/completion
    timeout: 60s
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

## Top-Level Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Unique identifier for the workflow. Must be kebab-case: lowercase letters, digits, and hyphens. Pattern: `^[a-z][a-z0-9-]*$`. |
| `description` | string | No | Human-readable description of what the workflow does. |
| `inputs` | map | No | Input parameters the workflow accepts at runtime. |
| `triggers` | list | No | Automatic triggers that start the workflow. See [Triggers](#triggers). |
| `steps` | list | Yes | Ordered list of steps to execute. At least one step is required. |

### Name Rules

The workflow name is the primary identifier used across `validate`, `apply`, `plan`, and `run`. It must:

- Start with a lowercase letter
- Contain only lowercase letters (`a-z`), digits (`0-9`), and hyphens (`-`)
- Not start or end with a hyphen

Valid examples: `fetch-data`, `my-workflow-v2`, `a1`

Invalid examples: `Fetch-Data`, `fetch_data`, `-fetch`, `123abc`

## Inputs

Inputs define the parameters a workflow accepts when triggered. Each input is a key-value pair in the `inputs` map.

```yaml
inputs:
  url:
    type: string
    description: URL to fetch
  verbose:
    type: boolean
    description: Enable verbose output
  max_items:
    type: number
    description: Maximum number of items to process
```

### Input Fields

| Field | Type | Required | Description |
|---|---|---|---|
| (key) | string | Yes | Input parameter name. Must be snake_case: lowercase letters, digits, and underscores. Pattern: `^[a-z][a-z0-9_]*$`. |
| `type` | string | No | Data type. One of: `string`, `number`, `boolean`. |
| `description` | string | No | Human-readable description. |

### Input Name Rules

Input names use snake_case (underscores), not kebab-case (hyphens). This is intentional -- input names appear in CEL expressions where hyphens would be interpreted as subtraction.

Valid: `url`, `max_retries`, `api_key`

Invalid: `URL`, `max-retries`, `apiKey`, `123abc`

## Steps

Steps are the building blocks of a workflow. Each step invokes a connector action and can optionally include conditional logic, retry policies, timeouts, and explicit dependencies. Steps without dependencies run concurrently; use `depends_on` to declare explicit ordering. See [Parallel Execution](#parallel-execution).

```yaml
steps:
  - name: fetch-data
    action: http/request
    timeout: 30s
    retry:
      max_attempts: 3
      backoff: exponential
    if: "inputs.url != ''"
    params:
      method: GET
      url: "{{ inputs.url }}"
```

### Step Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Unique name within the workflow. Must be kebab-case: `^[a-z][a-z0-9-]*$`. |
| `action` | string | Yes | Connector action to invoke, in `connector/action` format. |
| `params` | map | No | Parameters passed to the connector action. Structure depends on the action. |
| `if` | string | No | CEL expression. The step runs only if this evaluates to `true`. |
| `retry` | object | No | Retry policy for this step. See [Retry Policy](#retry-policy). |
| `timeout` | string | No | Maximum duration for the step. Uses Go duration format (e.g., `30s`, `5m`, `1h`). |
| `credential` | string | No | Name of a stored credential to inject into this step. See [Secrets Guide](/docs/secrets-guide). |
| `depends_on` | list of strings | No | Declares explicit dependencies on other steps for parallel execution. See [Parallel Execution](#parallel-execution). |
| `continue_on_error` | boolean | No | When `true`, workflow execution continues even if this step fails after exhausting retries. Default is `false`. See [Error Handling](#error-handling). |

### Step Name Rules

Step names follow the same rules as the workflow name: kebab-case, starting with a lowercase letter. Step names must be unique within a workflow -- duplicate names cause a validation error.

Step names matter because you reference step outputs in CEL expressions using `steps.STEP_NAME.output`.

**Note on hyphenated step names in CEL:** When a step name contains hyphens (e.g., `fetch-data`), you can use dot notation in template strings (`{{ steps.fetch-data.output.body }}`), but in `if` expressions you must use bracket notation: `steps['fetch-data'].output.body`. This is because CEL interprets hyphens as subtraction in expression context.

### Parallel Execution

By default, Mantle builds a directed acyclic graph (DAG) from your steps and runs steps concurrently when their dependencies allow it. You control ordering with `depends_on` and through implicit dependencies detected from CEL expressions.

**How dependencies are resolved:**

- **Explicit dependencies** -- list step names in `depends_on` to declare that a step must wait for those steps to complete before it can start.
- **Implicit dependencies** -- Mantle analyzes CEL expressions in `params` and `if` fields. If a step references `steps.fetch-data.output`, the engine automatically adds `fetch-data` as a dependency. You do not need to list implicit dependencies in `depends_on`.
- **Skipped steps count as resolved** -- if a step is skipped (its `if` condition evaluated to `false`), downstream steps that depend on it are unblocked and can proceed.

**Fan-out/fan-in example:**

```yaml
name: fan-out-fan-in
description: Run two API calls in parallel, then merge results

steps:
  - name: fetch-users
    action: http/request
    params:
      method: GET
      url: https://api.example.com/users

  - name: fetch-orders
    action: http/request
    params:
      method: GET
      url: https://api.example.com/orders

  - name: merge-results
    action: ai/completion
    credential: openai
    depends_on:
      - fetch-users
      - fetch-orders
    params:
      model: gpt-4o
      prompt: >
        Correlate these users and orders:
        Users: {{ steps['fetch-users'].output.body }}
        Orders: {{ steps['fetch-orders'].output.body }}
```

In this workflow, `fetch-users` and `fetch-orders` have no dependencies on each other, so they run concurrently. The `merge-results` step declares both as explicit dependencies via `depends_on` and waits for both to complete before it starts.

## Retry Policy

The retry policy controls what happens when a step fails.

```yaml
retry:
  max_attempts: 3
  backoff: exponential
```

| Field | Type | Required | Description |
|---|---|---|---|
| `max_attempts` | integer | Yes | Maximum number of attempts. Must be greater than 0. |
| `backoff` | string | No | Backoff strategy between retries. One of: `fixed`, `exponential`. |

If `backoff` is omitted and `retry` is present, the default behavior depends on the engine implementation.

## Timeout

The `timeout` field accepts Go duration strings. These consist of a number followed by a unit suffix:

| Unit | Suffix | Example |
|---|---|---|
| Milliseconds | `ms` | `500ms` |
| Seconds | `s` | `30s` |
| Minutes | `m` | `5m` |
| Hours | `h` | `1h` |

You can combine units: `1m30s` means one minute and thirty seconds.

The timeout must be a positive duration. `0s` and negative values are invalid.

## Error Handling

By default, if a step fails after exhausting its retry policy, the workflow stops and the entire execution is marked as failed. The `continue_on_error` field changes this behavior.

### continue_on_error

When `continue_on_error: true`, the step's failure does not stop the workflow. Instead, the error is captured and made available to downstream steps via `steps['step-name'].error`. This allows workflows to implement custom error handling, logging, or recovery logic.

```yaml
steps:
  - name: backup
    action: s3/put
    continue_on_error: true
    timeout: "5m"
    params:
      bucket: my-backups
      key: "data.csv"
      content: "{{ steps.fetch.output.body }}"

  - name: notify-on-failure
    action: slack/send
    credential: slack-token
    if: "steps['backup'].error != null"
    params:
      channel: "#ops-alerts"
      text: "Backup failed: {{ steps['backup'].error }}"
```

### Available Error Fields

- **`steps['name'].error`** — `null` for successful or skipped steps; a string error message for failed steps. The field is always present in the CEL context, but is only practically reachable on a step that has `continue_on_error: true` — without that flag, a step failure halts the workflow before any downstream step can inspect the error.
- **`steps['name'].output`** — Partial output available from the failed step if the connector provided it. Structure depends on the connector.

### Example: Fallback Pattern

Use `continue_on_error` with conditional steps to implement fallback logic:

```yaml
steps:
  - name: try-primary-api
    action: http/request
    continue_on_error: true
    params:
      method: GET
      url: https://primary-api.example.com/data

  - name: try-backup-api
    action: http/request
    if: "steps['try-primary-api'].error != null"
    params:
      method: GET
      url: https://backup-api.example.com/data

  - name: process-data
    action: http/request
    params:
      method: POST
      url: https://processor.example.com/process
      # Use output from whichever API succeeded
      body: "{{ steps['try-primary-api'].error == null ? steps['try-primary-api'].output.body : steps['try-backup-api'].output.body }}"
```

## CEL Expressions

Mantle uses [CEL (Common Expression Language)](https://cel.dev) for conditional logic and data access between steps. See the [Expressions guide](/docs/concepts/expressions) for practical examples. CEL expressions appear in two places:

1. **`if` fields** -- determine whether a step runs
2. **Template strings in `params`** -- reference data from inputs and previous steps using `{{ expression }}` syntax

### Available Variables

| Variable | Description |
|---|---|
| `inputs.NAME` | Value of the input parameter `NAME`. |
| `steps.STEP_NAME.output` | Output of the step named `STEP_NAME`. The structure depends on the connector. |
| `env.NAME` | Value of the environment variable `NAME`. |
| `trigger.payload` | Request body from a webhook trigger, parsed as JSON. Only available for webhook-triggered executions. |

### Expression Examples

Reference an input:

```yaml
url: "{{ inputs.url }}"
```

Reference a previous step's output:

```yaml
prompt: "Summarize: {{ steps.fetch-data.output.body }}"
```

Conditional execution based on step output:

```yaml
if: "steps.summarize.output.key_points.size() > 0"
```

Boolean logic:

```yaml
if: "inputs.verbose == true && steps.fetch-data.output.status == 200"
```

String operations:

```yaml
if: "steps.fetch-data.output.body.contains('error') == false"
```

### CEL Type Safety

CEL is a strongly typed language. If you compare values of different types, the expression will fail at evaluation time. For example, `inputs.count > "5"` fails because you are comparing a number to a string.

## Triggers

Triggers define how a workflow is started automatically when Mantle runs in server mode (`mantle serve`). A workflow can have zero, one, or multiple triggers.

```yaml
triggers:
  - type: cron
    schedule: "*/5 * * * *"
  - type: webhook
    path: "/hooks/my-workflow"
```

Triggers are optional. Without them, the workflow can still be executed manually with `mantle run` or via the REST API (`POST /api/v1/run/{workflow}`).

### Trigger Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `type` | string | Yes | Trigger type. One of: `cron`, `webhook`, `email`. |
| `schedule` | string | Cron only | Cron expression defining the schedule. Required when `type` is `cron`. |
| `path` | string | Webhook only | URL path for the webhook endpoint. Required when `type` is `webhook`. |
| `mailbox` | string | Email only | Credential name for the email account (IMAP-compatible). Required when `type` is `email`. |
| `folder` | string | Email only | Folder to monitor (e.g., `INBOX`). Default: `INBOX`. |
| `filter` | string | Email only | Filter messages: `all`, `unseen`, `recent`, `flagged`. Default: `unseen`. |
| `poll_interval` | string | Email only | How often to check for new messages (e.g., `30s`, `5m`). Default: `60s`. |

### Trigger Lifecycle

Triggers are managed through the standard IaC lifecycle. When you run `mantle apply`:

- **New triggers** in the YAML are registered with the server
- **Changed triggers** (e.g., updated cron schedule) are updated
- **Removed triggers** (deleted from the YAML) are deregistered

You do not manage triggers separately. The workflow definition is the single source of truth.

## Validation Rules Summary

Mantle validates the following rules when you run `mantle validate` or `mantle apply`:

| Rule | Error Message |
|---|---|
| Workflow name is required | `name is required` |
| Workflow name must be kebab-case | `name must match ^[a-z][a-z0-9-]*$` |
| At least one step is required | `at least one step is required` |
| Input names must be snake_case | `input name must match ^[a-z][a-z0-9_]*$` |
| Input types must be valid | `type must be one of: string, number, boolean` |
| Step names are required | `step name is required` |
| Step names must be kebab-case | `step name must match ^[a-z][a-z0-9-]*$` |
| Step names must be unique | `duplicate step name "NAME"` |
| Step actions are required | `step action is required` |
| Retry max_attempts must be > 0 | `max_attempts must be greater than 0` |
| Retry backoff must be valid | `backoff must be one of: fixed, exponential` |
| Timeout must be a valid duration | `invalid duration: ...` |
| Timeout must be positive | `timeout must be a positive duration` |
| Dependency cycle detected | `cycle detected in step dependencies` |
| `depends_on` references undefined step | `references undefined step "NAME"` |

Validation errors include line and column numbers when available, formatted as:

```
workflow.yaml:3:1: error: step name must match ^[a-z][a-z0-9-]*$ (steps[0].name)
```

## Minimal Valid Workflow

The smallest valid workflow contains a name and one step with an action:

```yaml
name: hello
steps:
  - name: greet
    action: http/request
    params:
      method: GET
      url: https://httpbin.org/get
```
