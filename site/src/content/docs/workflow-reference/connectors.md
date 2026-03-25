# Connector Reference

Connectors define the actions a step can perform. Actions use a `connector/action` naming convention. For AI tool use (function calling), see [Tool Use](/docs/workflow-reference/tools).

## http/request

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

## ai/completion

Sends a prompt to an OpenAI-compatible chat completion API and returns the result. Requires a credential with an API key -- see the [Secrets Guide](/docs/secrets-guide) for setup.

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
| `tools` | list | No | Tool declarations for function calling. See [Tool Use](/docs/workflow-reference/tools). |
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

**Authentication:** The AI connector reads the credential's `api_key` field (or `token` or `key` as fallbacks) and sends it as a Bearer token. If the credential includes an `org_id` field, it is sent as the `OpenAI-Organization` header. See the [Secrets Guide](/docs/secrets-guide) for how to create an `openai`-type credential.

## slack/send

Sends a message to a Slack channel via the [chat.postMessage](https://api.slack.com/methods/chat.postMessage) API. Requires a credential with a Slack Bot User OAuth Token.

**Params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `channel` | string | Yes | Slack channel — either a channel ID (e.g., `C01234ABCDE`) or a channel name with `#` prefix (e.g., `#general`). Channel IDs are preferred for reliability. |
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

## slack/history

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

## postgres/query

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

## email/send

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

## s3/put

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

## s3/get

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

## s3/list

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

## S3 Authentication

All S3 connectors (`s3/put`, `s3/get`, `s3/list`) read the following fields from the credential:

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

## docker/run

Runs a Docker container to completion and captures its output. The container is created, started, waited on, and optionally removed. Non-zero exit codes do not constitute a step failure — use `if` conditions to branch on exit code.

**Params:**

| Param | Type | Required | Default | Description |
|---|---|---|---|---|
| `image` | string | Yes | — | Container image (e.g., `alpine:latest`) |
| `cmd` | array | No | — | Command and arguments |
| `env` | object | No | — | Environment variables |
| `stdin` | string | No | — | Data piped to container stdin |
| `mounts` | array | No | — | Volume/bind mounts (each with `source`, `target`, `readonly`) |
| `network` | string | No | `bridge` | Docker network mode (`bridge` or `none`) |
| `pull` | string | No | `missing` | Image pull policy: `always`, `missing`, `never` |
| `memory` | string | No | — | Memory limit (e.g., `512m`, `1g`) |
| `cpus` | number | No | — | CPU limit (e.g., `1.5`) |
| `remove` | boolean | No | `true` | Remove container after completion |

**Output:**

| Field | Type | Description |
|---|---|---|
| `exit_code` | integer | Container exit code |
| `stdout` | string | Container stdout (capped at 10MB) |
| `stderr` | string | Container stderr (capped at 10MB) |

**Authentication:** The Docker connector uses a `docker` credential type for daemon access. All fields are optional — an empty credential connects to the local Docker socket. For private images, use `registry_credential` with a `basic` credential type. Note that `registry_credential` is a **step-level field** (alongside `credential`), not a param.

**Security:** Containers run with all Linux capabilities dropped (`CAP_DROP ALL`), `no-new-privileges`, and a PID limit. Only `bridge` and `none` network modes are permitted.

**Example:**

```yaml
- name: process-data
  action: docker/run
  credential: my-docker
  registry_credential: my-registry
  timeout: "2m"
  params:
    image: myorg/processor:latest
    cmd: ["process", "--format", "json"]
    stdin: "{{ steps['fetch-data'].output.body }}"
    memory: "512m"
    cpus: 1.0
```
