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
| `credential` | string | No | Name of a stored credential to inject into this step. See [Secrets Guide](secrets-guide.md). |
| `depends_on` | list of strings | No | Declares explicit dependencies on other steps for parallel execution. See [Parallel Execution](#parallel-execution). |

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
| `status` | number | HTTP response status code. |
| `headers` | map | Response headers. |
| `body` | string | Raw response body as a string. |
| `json` | any | Parsed response body. Only present when the response is valid JSON. |

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

### ai/completion

Sends a prompt to an OpenAI-compatible chat completion API and returns the result. Requires a credential with an API key -- see the [Secrets Guide](secrets-guide.md) for setup.

**Params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `provider` | string | No | AI provider to use: `openai` (default) or `bedrock`. |
| `model` | string | Yes | Model name (e.g., `gpt-4o`, `gpt-4o-mini`, `anthropic.claude-3-sonnet-20240229-v1:0`). |
| `prompt` | string | Yes | The user prompt to send. |
| `region` | string | No | AWS region for the Bedrock provider (e.g., `us-east-1`). Only used when `provider` is `bedrock`. |
| `system_prompt` | string | No | System message prepended to the conversation. |
| `output_schema` | object | No | JSON Schema for structured output. When set, the model returns JSON conforming to this schema. |
| `base_url` | string | No | Override the API base URL. Defaults to `https://api.openai.com/v1`. Use this for OpenAI-compatible providers like Azure, Ollama, or local models. |
| `tools` | list | No | Tool declarations for function calling. See [Tool Declarations](#tool-declarations). |
| `max_tool_rounds` | integer | No | Maximum number of LLM-tool interaction rounds. Default: `10` (from `engine.default_max_tool_rounds`). |
| `max_tool_calls_per_round` | integer | No | Maximum number of tool calls the LLM can make in a single round. Default: `10` (from `engine.default_max_tool_calls_per_round`). |

**Output:**

| Field | Type | Description |
|---|---|---|
| `text` | string | The raw completion text returned by the model. |
| `json` | any | If the response is valid JSON (e.g., from structured output), the parsed object. Only present when the response parses as JSON. |
| `tool_calls` | list | Tool invocations requested by the model. Each item has `id`, `type`, and `function` (with `name` and `arguments`). Only present when the model requests tool calls in the final response. |
| `finish_reason` | string | Why the model stopped generating. `stop` for normal text completion, `tool_calls` when the model requested tool invocations. |
| `model` | string | The model name as reported by the API. |
| `usage.prompt_tokens` | number | Number of tokens in the prompt. |
| `usage.completion_tokens` | number | Number of tokens in the completion. |
| `usage.total_tokens` | number | Total tokens used. |

**Example -- basic completion:**

```yaml
- name: summarize
  action: ai/completion
  credential: my-openai
  params:
    model: gpt-4o
    prompt: "Summarize this in 3 bullet points: {{ steps.fetch-data.output.body }}"
```

**Example -- with system prompt and structured output:**

```yaml
- name: extract-entities
  action: ai/completion
  credential: my-openai
  timeout: 60s
  params:
    model: gpt-4o
    system_prompt: "You are a data extraction assistant. Always respond with valid JSON."
    prompt: "Extract all person names and companies from: {{ steps.fetch-data.output.body }}"
    output_schema:
      type: object
      properties:
        people:
          type: array
          items:
            type: string
        companies:
          type: array
          items:
            type: string
      required:
        - people
        - companies
      additionalProperties: false
```

The structured output is available as `steps.extract-entities.output.json.people` and `steps.extract-entities.output.json.companies` in subsequent steps.

**Example -- custom base URL (Ollama):**

```yaml
- name: local-completion
  action: ai/completion
  params:
    model: llama3
    base_url: http://localhost:11434/v1
    prompt: "Explain this error: {{ steps.fetch-logs.output.body }}"
```

**Example -- AWS Bedrock:**

```yaml
- name: summarize
  action: ai/completion
  credential: aws-bedrock-creds
  params:
    provider: bedrock
    model: anthropic.claude-3-sonnet-20240229-v1:0
    region: us-east-1
    prompt: "Summarize: {{ steps.fetch.output.body }}"
```

When running on AWS infrastructure with an IAM role attached (IRSA, instance profile, etc.), the `credential` field can be omitted -- the Bedrock provider uses the standard AWS credential chain automatically.

**Authentication:** The AI connector reads the credential's `api_key` field (or `token` or `key` as fallbacks) and sends it as a Bearer token. If the credential includes an `org_id` field, it is sent as the `OpenAI-Organization` header. See the [Secrets Guide](secrets-guide.md) for how to create an `openai`-type credential.

#### Tool Declarations

Tools let the LLM call back into Mantle connectors during a completion. When you declare `tools` on an `ai/completion` step, the engine runs a multi-turn loop: it sends the prompt to the LLM, the LLM may request tool calls, the engine executes those calls using connector actions, feeds the results back to the LLM, and repeats until the LLM produces a final text response or the configured limits are reached.

Each tool in the `tools` list has the following schema:

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Tool name exposed to the LLM. |
| `description` | string | No | Human-readable description of what the tool does. Helps the LLM decide when to use it. |
| `input_schema` | object | No | JSON Schema describing the tool's input parameters. |
| `action` | string | Yes | Connector action to invoke when the LLM calls this tool (e.g., `http/request`, `postgres/query`). |
| `params` | map | No | Static parameters merged with the LLM-provided arguments when the tool is invoked. |

**Example -- tool use with web search:**

```yaml
- name: research-assistant
  action: ai/completion
  credential: openai
  params:
    model: gpt-4o
    prompt: "Find the current population of Seattle and summarize the top 3 industries."
    max_tool_rounds: 5
    tools:
      - name: web_search
        description: "Search the web for current information"
        input_schema:
          type: object
          properties:
            query:
              type: string
              description: "Search query"
          required:
            - query
        action: http/request
        params:
          method: GET
          url: "https://api.search.example.com/search"
```

The LLM sees `web_search` as an available function. When it decides to call `web_search(query="Seattle population 2026")`, the engine executes the `http/request` action with the merged parameters and returns the result to the LLM. This continues for up to `max_tool_rounds` rounds.

If the LLM exhausts all rounds without producing a final text response, the engine makes one last call asking the LLM to summarize with the information gathered so far.

### slack/send

Sends a message to a Slack channel via the [chat.postMessage](https://api.slack.com/methods/chat.postMessage) API. Requires a credential with a Slack Bot User OAuth Token.

**Params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `channel` | string | Yes | Slack channel ID (e.g., `C01234ABCDE`). Use the channel ID, not the channel name. |
| `text` | string | Yes | Message text. Supports Slack [mrkdwn](https://api.slack.com/reference/surfaces/formatting) formatting. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `ok` | boolean | `true` if the message was sent successfully. |
| `ts` | string | Slack message timestamp. Use this to reference the message in follow-up API calls. |
| `channel` | string | The channel ID where the message was posted. |

**Example:**

```yaml
- name: notify-team
  action: slack/send
  credential: slack-bot
  params:
    channel: "C01234ABCDE"
    text: "Deployment complete: {{ steps.deploy.output.body }}"
```

**Authentication:** The Slack connector reads the credential's `token` field and sends it as a Bearer token. Create a credential of type `bearer` with a `token` field containing your Slack Bot User OAuth Token:

```bash
mantle secrets create --name slack-bot --type bearer --field token=xoxb-your-bot-token
```

### slack/history

Reads recent messages from a Slack channel via the [conversations.history](https://api.slack.com/methods/conversations.history) API.

**Params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `channel` | string | Yes | Slack channel ID (e.g., `C01234ABCDE`). |
| `limit` | number | No | Maximum number of messages to return. Default: `10`. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `ok` | boolean | `true` if the request was successful. |
| `messages` | list | Array of message objects. Each message contains fields like `text`, `user`, `ts`, and `type`. |

**Example:**

```yaml
- name: read-channel
  action: slack/history
  credential: slack-bot
  params:
    channel: "C01234ABCDE"
    limit: 5

- name: summarize-messages
  action: ai/completion
  credential: my-openai
  params:
    model: gpt-4o
    prompt: "Summarize these Slack messages: {{ steps['read-channel'].output.messages }}"
```

### postgres/query

Executes a parameterized SQL query against an external Postgres database. The connector opens a connection per step execution and closes it afterward. Supports both read queries (`SELECT`, `WITH`) and write statements (`INSERT`, `UPDATE`, `DELETE`).

**Params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `query` | string | Yes | SQL query to execute. Use `$1`, `$2`, etc. for parameterized values. |
| `args` | list | No | Ordered list of values to substitute into the parameterized query. |

**Output (SELECT/WITH queries):**

| Field | Type | Description |
|---|---|---|
| `rows` | list | Array of row objects, each mapping column names to values. Empty array if no rows match. |
| `row_count` | number | Number of rows returned. |

**Output (INSERT/UPDATE/DELETE statements):**

| Field | Type | Description |
|---|---|---|
| `rows_affected` | number | Number of rows affected by the statement. |

**Example -- read query:**

```yaml
- name: fetch-users
  action: postgres/query
  credential: my-database
  params:
    query: "SELECT id, email FROM users WHERE active = $1 LIMIT $2"
    args:
      - true
      - 100
```

**Example -- write statement:**

```yaml
- name: update-status
  action: postgres/query
  credential: my-database
  params:
    query: "UPDATE orders SET status = $1 WHERE id = $2"
    args:
      - "shipped"
      - "{{ steps['create-order'].output.json.order_id }}"
```

**Authentication:** The Postgres connector reads the database connection URL from the credential's `url` field (or `key` as a fallback). Create a credential with the full Postgres connection string:

```bash
mantle secrets create --name my-database --type generic --field url=postgres://user:pass@host:5432/dbname?sslmode=require
```

### email/send

Sends an email via SMTP. Supports plaintext and HTML content.

**Params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `to` | string or list | Yes | Recipient email address(es). A single string or a list of strings. |
| `from` | string | Yes | Sender email address. |
| `subject` | string | Yes | Email subject line. |
| `body` | string | Yes | Email body content. |
| `html` | boolean | No | Set to `true` to send the body as HTML. Default: `false` (plaintext). |
| `smtp_host` | string | No | SMTP server hostname. Can also be provided via credential. |
| `smtp_port` | string | No | SMTP server port. Default: `587`. Can also be provided via credential. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `sent` | boolean | `true` if the email was sent successfully. |
| `to` | string | Comma-separated list of recipient addresses. |
| `subject` | string | The subject line that was sent. |

**Example:**

```yaml
- name: send-report
  action: email/send
  credential: smtp-creds
  params:
    to:
      - "alice@example.com"
      - "bob@example.com"
    from: "reports@example.com"
    subject: "Daily Report — {{ steps.generate.output.json.date }}"
    body: "{{ steps.generate.output.json.html_report }}"
    html: true
```

**Authentication:** The email connector reads `username`, `password`, `host`, and `port` from the credential. If `host` or `port` are not in the credential, they fall back to the `smtp_host` and `smtp_port` params. Create a `basic` credential with SMTP fields:

```bash
mantle secrets create --name smtp-creds --type basic \
  --field username=apikey \
  --field password=SG.your-sendgrid-key \
  --field host=smtp.sendgrid.net \
  --field port=587
```

### s3/put

Uploads an object to an S3-compatible storage bucket.

**Params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `bucket` | string | Yes | S3 bucket name. |
| `key` | string | Yes | Object key (path) within the bucket. |
| `content` | string | Yes | Object content as a string. |
| `content_type` | string | No | MIME type for the object. Default: `application/octet-stream`. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `bucket` | string | The bucket the object was uploaded to. |
| `key` | string | The object key. |
| `size` | number | Size of the uploaded content in bytes. |

**Example:**

```yaml
- name: upload-report
  action: s3/put
  credential: aws-s3
  params:
    bucket: "my-reports"
    key: "reports/{{ steps.generate.output.json.date }}.json"
    content: "{{ steps.generate.output.json.report }}"
    content_type: "application/json"
```

### s3/get

Downloads an object from an S3-compatible storage bucket.

**Params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `bucket` | string | Yes | S3 bucket name. |
| `key` | string | Yes | Object key (path) within the bucket. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `bucket` | string | The bucket the object was downloaded from. |
| `key` | string | The object key. |
| `content` | string | Object content as a string. |
| `size` | number | Size of the downloaded content in bytes. |
| `content_type` | string | MIME type of the object as reported by S3. |

**Example:**

```yaml
- name: download-config
  action: s3/get
  credential: aws-s3
  params:
    bucket: "my-configs"
    key: "app/config.json"
```

### s3/list

Lists objects in an S3-compatible storage bucket, with optional prefix filtering.

**Params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `bucket` | string | Yes | S3 bucket name. |
| `prefix` | string | No | Filter results to keys that start with this prefix. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `bucket` | string | The bucket that was listed. |
| `objects` | list | Array of objects. Each object has `key` (string), `size` (number), and `last_modified` (string, RFC 3339). |

**Example:**

```yaml
- name: list-reports
  action: s3/list
  credential: aws-s3
  params:
    bucket: "my-reports"
    prefix: "reports/2026/"
```

**S3 Authentication:** All S3 connectors (`s3/put`, `s3/get`, `s3/list`) read the following fields from the credential:

| Field | Required | Description |
|---|---|---|
| `access_key` | Yes | AWS access key ID. |
| `secret_key` | Yes | AWS secret access key. |
| `region` | No | AWS region. Default: `us-east-1`. |
| `endpoint` | No | Custom S3 endpoint URL. Use this for S3-compatible services like MinIO, DigitalOcean Spaces, or Backblaze B2. |

Create a credential for S3:

```bash
mantle secrets create --name aws-s3 --type generic \
  --field access_key=AKIA... \
  --field secret_key=wJalr... \
  --field region=us-west-2
```

For S3-compatible services, add an `endpoint` field:

```bash
mantle secrets create --name minio --type generic \
  --field access_key=minioadmin \
  --field secret_key=minioadmin \
  --field endpoint=http://localhost:9000
```

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

### Trigger Lifecycle

Triggers are managed through the standard IaC lifecycle. When you run `mantle apply`:

- **New triggers** in the YAML are registered with the server
- **Changed triggers** (e.g., updated cron schedule) are updated
- **Removed triggers** (deleted from the YAML) are deregistered

You do not manage triggers separately. The workflow definition is the single source of truth.

### Trigger Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `type` | string | Yes | Trigger type. One of: `cron`, `webhook`. |
| `schedule` | string | Cron only | Cron expression defining the schedule. Required when `type` is `cron`. |
| `path` | string | Webhook only | URL path for the webhook endpoint. Required when `type` is `webhook`. |

### Cron Triggers

Cron triggers execute the workflow on a recurring schedule. The `schedule` field uses standard 5-field cron syntax:

```
┌───────────── minute (0-59)
│ ┌───────────── hour (0-23)
│ │ ┌───────────── day of month (1-31)
│ │ │ ┌───────────── month (1-12)
│ │ │ │ ┌───────────── day of week (0-6, Sunday=0)
│ │ │ │ │
* * * * *
```

**Supported syntax:**

| Syntax | Meaning | Example |
|---|---|---|
| `*` | Every value | `* * * * *` -- every minute |
| `*/N` | Every N intervals | `*/5 * * * *` -- every 5 minutes |
| `N-M` | Range from N to M | `0 9-17 * * *` -- every hour from 9 AM to 5 PM |
| `N,M,O` | Comma-separated list | `0 0 1,15 * *` -- 1st and 15th of the month |

**Examples:**

```yaml
# Every 5 minutes
triggers:
  - type: cron
    schedule: "*/5 * * * *"

# Daily at midnight
triggers:
  - type: cron
    schedule: "0 0 * * *"

# Weekdays at 9 AM
triggers:
  - type: cron
    schedule: "0 9 * * 1-5"

# Every hour on the hour
triggers:
  - type: cron
    schedule: "0 * * * *"
```

The cron scheduler polls every 30 seconds. Executions may start up to 30 seconds after the scheduled time.

### Webhook Triggers

Webhook triggers execute the workflow when an HTTP POST request is received at the configured path. The request body is available inside the workflow as `trigger.payload`.

```yaml
triggers:
  - type: webhook
    path: "/hooks/deploy-notifier"
```

When the server receives a POST to `/hooks/deploy-notifier`, it starts a new execution. The full request body is parsed as JSON and made available through the `trigger.payload` variable in CEL expressions:

```yaml
name: deploy-notifier
triggers:
  - type: webhook
    path: "/hooks/deploy-notifier"

steps:
  - name: notify
    action: http/request
    params:
      method: POST
      url: https://hooks.slack.com/services/T00/B00/xxx
      body:
        text: "Deployed {{ trigger.payload.repo }} to {{ trigger.payload.environment }}"
```

Triggering the webhook:

```bash
curl -X POST http://localhost:8080/hooks/deploy-notifier \
  -H "Content-Type: application/json" \
  -d '{"repo": "my-app", "environment": "production"}'
```

### Multiple Triggers

A workflow can have multiple triggers of different types. Each trigger independently starts a new execution:

```yaml
triggers:
  - type: cron
    schedule: "0 * * * *"
  - type: webhook
    path: "/hooks/my-workflow"
```

This workflow runs every hour on the hour via cron, and can also be triggered on demand via a webhook POST.

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
