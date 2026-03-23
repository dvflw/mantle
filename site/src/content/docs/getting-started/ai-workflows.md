# AI Workflows

Mantle includes a built-in AI connector that supports OpenAI-compatible APIs. Before using it, you need to store your API key as an encrypted credential.

## Set Up Credentials

Generate an encryption key and export it:

```bash
export MANTLE_ENCRYPTION_KEY=$(openssl rand -hex 32)
```

Store your OpenAI API key:

```bash
mantle secrets create --name openai --type openai --field api_key=sk-proj-your-key-here
```

```
Created credential "openai" (type: openai)
```

The credential is encrypted at rest with AES-256-GCM. The raw API key is never stored in plaintext, never exposed in logs, and never available in CEL expressions. See the [Secrets Guide](/docs/secrets-guide) for credential types and the full security model.

## AI Completion Step

Here is a workflow that fetches a webpage and uses an LLM to extract structured data (from `examples/ai-structured-extraction.yaml`):

```yaml
name: ai-structured-extraction
description: >
  Fetch a webpage and use an LLM with output_schema to extract structured data
  (title, author, key topics). Demonstrates enforcing JSON structure from AI output.

inputs:
  url:
    type: string
    description: URL of the page to fetch and extract data from

steps:
  - name: fetch-page
    action: http/request
    timeout: "15s"
    retry:
      max_attempts: 2
      backoff: exponential
    params:
      method: GET
      url: "{{ inputs.url }}"

  - name: extract-metadata
    action: ai/completion
    credential: openai
    params:
      model: gpt-4o
      system_prompt: >
        You are a structured data extraction engine. Given raw page content,
        extract the requested fields accurately. If a field cannot be determined,
        use null or an empty value as appropriate.
      prompt: >
        Extract the following metadata from this page content:

        {{ steps['fetch-page'].output.body }}
      output_schema:
        type: object
        properties:
          title:
            type: string
          author:
            type: string
          key_topics:
            type: array
            items:
              type: string
        required:
          - title
          - author
          - key_topics
        additionalProperties: false
```

The `credential: openai` field tells the engine to resolve the `openai` credential you created earlier. The `output_schema` field enforces structured JSON output from the model -- the response is guaranteed to match the schema.

Apply and run it:

```bash
mantle apply examples/ai-structured-extraction.yaml
mantle run ai-structured-extraction --input url=https://example.com
```

```
Running ai-structured-extraction (version 1)...
Execution d4e5f6a7-b8c9-0123-defa-234567890123: completed
  fetch-page: completed
  extract-metadata: completed
```

## Key AI Connector Details

| Field | Description |
|---|---|
| `action` | `ai/completion` for chat completions |
| `credential` | Name of a stored credential (type `openai`) |
| `model` | Model name (e.g., `gpt-4o`, `gpt-4o-mini`) |
| `prompt` | User message -- supports CEL template expressions |
| `system_prompt` | Optional system message to set model behavior |
| `output_schema` | Optional JSON Schema -- enforces structured output |

See the [Workflow Reference](/docs/workflow-reference/connectors) for the complete AI connector specification, including tool use, custom base URLs, and AWS Bedrock support.
