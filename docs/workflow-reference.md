# Workflow Reference

This document describes every field in a Mantle workflow YAML file. For a hands-on introduction, start with the [Getting Started](getting-started.md) guide.

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

Steps are executed in order. Each step invokes a connector action and can optionally include conditional logic, retry policies, and timeouts.

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

### Step Name Rules

Step names follow the same rules as the workflow name: kebab-case, starting with a lowercase letter. Step names must be unique within a workflow -- duplicate names cause a validation error.

Step names matter because you reference step outputs in CEL expressions using `steps.STEP_NAME.output`.

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

## CEL Expressions

Mantle uses [CEL (Common Expression Language)](https://github.com/google/cel-go) for conditional logic and data access between steps. CEL expressions appear in two places:

1. **`if` fields** -- determine whether a step runs
2. **Template strings in `params`** -- reference data from inputs and previous steps using `{{ expression }}` syntax

### Available Variables

| Variable | Description |
|---|---|
| `inputs.NAME` | Value of the input parameter `NAME`. |
| `steps.STEP_NAME.output` | Output of the step named `STEP_NAME`. The structure depends on the connector. |
| `env.NAME` | Value of the environment variable `NAME`. |

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
if: "inputs.verbose == true && steps.fetch-data.output.status_code == 200"
```

String operations:

```yaml
if: "steps.fetch-data.output.body.contains('error') == false"
```

### CEL Type Safety

CEL is a strongly typed language. If you compare values of different types, the expression will fail at evaluation time. For example, `inputs.count > "5"` fails because you are comparing a number to a string.

## Connectors

Connectors define the actions a step can perform. Actions use a `connector/action` naming convention.

### http/request

Makes an HTTP request.

**Params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `method` | string | Yes | HTTP method: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`. |
| `url` | string | Yes | Request URL. |
| `headers` | map | No | HTTP headers as key-value pairs. |
| `body` | any | No | Request body. Objects are JSON-encoded. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `status_code` | number | HTTP response status code. |
| `headers` | map | Response headers. |
| `body` | any | Response body. JSON responses are parsed into objects. |

**Example:**

```yaml
- name: create-item
  action: http/request
  params:
    method: POST
    url: https://api.example.com/items
    headers:
      Authorization: "Bearer {{ env.API_TOKEN }}"
      Content-Type: application/json
    body:
      name: "New Item"
      quantity: 5
```

### ai/completion (coming soon)

Sends a prompt to an LLM and returns the completion.

**Params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `provider` | string | Yes | LLM provider. Currently: `openai`. |
| `model` | string | Yes | Model name (e.g., `gpt-4o`, `gpt-4o-mini`). |
| `prompt` | string | Yes | The prompt to send. |

**Output:**

The output structure depends on the provider and model. Typically includes the completion text and any structured output fields.

**Example:**

```yaml
- name: summarize
  action: ai/completion
  params:
    provider: openai
    model: gpt-4o
    prompt: "Summarize this in 3 bullet points: {{ steps.fetch-data.output.body }}"
```

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
