# Security Model

This page covers how Mantle handles secrets, credential resolution, and data residency.

## Secrets and Credential Resolution

Mantle treats secrets (API keys, tokens, credentials) as opaque handles that are resolved at connector invocation time. You never put raw secret values in workflow YAML. Instead, you create a named credential with `mantle secrets create` and reference it by name in your workflow step's `credential` field.

### Credential Types

Each credential has a type that defines its schema:

| Type | Fields | Use Case |
|---|---|---|
| `generic` | `key` (required) | General-purpose API key |
| `bearer` | `token` (required) | Bearer token authentication |
| `openai` | `api_key` (required), `org_id` (optional) | OpenAI API access |
| `basic` | `username` (required), `password` (required) | HTTP Basic authentication |

Types enforce that the right fields are present when you create a credential, reducing misconfiguration errors at runtime.

### How Credential Resolution Works

When the engine reaches a step with a `credential` field, it resolves the credential name before invoking the connector:

1. **Postgres lookup** -- the engine queries the credentials table, decrypts the stored fields using AES-256-GCM, and passes them to the connector.
2. **Environment variable fallback** -- if the credential is not found in Postgres, the engine checks for an environment variable named `MANTLE_SECRET_<UPPER_NAME>` (hyphens are replaced with underscores). The env var value is returned as a single `key` field, equivalent to a `generic` credential.

The resolved credential fields are injected directly into the connector as an internal `_credential` parameter. They are never visible in CEL expressions, step outputs, or execution logs.

### Security Properties

- **Encrypted at rest** -- credential field values are encrypted with AES-256-GCM before being written to Postgres. The encryption key is not stored in the database.
- **Never in expressions** -- you cannot reference `credential` data in CEL templates or `if` conditions. The credential is resolved inside the connector, not in the expression engine.
- **Never in logs** -- credential values do not appear in execution logs, step outputs, or error messages.
- **Typed validation** -- creating a credential validates that all required fields for the type are present.

For the full operational guide, see the [Secrets Guide](/docs/secrets-guide).

## Data Residency

Mantle is a self-hosted platform. You control where your data lives by choosing where to deploy Postgres and the Mantle binary.

### Where Data Resides

All workflow data -- inputs, outputs, step checkpoints, encrypted credentials, and audit events -- is stored in the Postgres database. There is no external data store, no telemetry sent to Anthropic or any third party, and no cloud dependency unless you configure one (e.g., cloud secret backends).

Data residency is determined entirely by where you host Postgres. Deploy Postgres in the EU, and all Mantle data resides in the EU.

### BYOK and Credential Storage

Mantle's Bring Your Own Key (BYOK) model means your API keys and credentials are stored in YOUR database, encrypted with YOUR encryption key. They are not sent to a third-party service for storage or management. This is a fundamental difference from SaaS platforms that hold your credentials on their infrastructure.

### AI Connector and Cross-Border Data Flow

While Mantle itself keeps all data in your Postgres instance, the AI connector sends prompts and receives responses from external LLM provider APIs. These API calls cross network boundaries and may cross geographic borders:

| Provider | Default Endpoint | Data Location |
|---|---|---|
| OpenAI | `https://api.openai.com/v1` | US-based (unless using Azure OpenAI regional endpoints) |
| Azure OpenAI | Configured via `base_url` | Region-specific, controlled by your Azure deployment |
| AWS Bedrock | Configured via `region` param or `aws.region` config | Region-specific (e.g., `eu-west-1`, `us-east-1`) |
| Self-hosted (Ollama, vLLM) | Configured via `base_url` | Wherever you host it |

### Restricting AI Endpoints

Use `engine.allowed_base_urls` in your configuration to restrict which AI API endpoints can be called. This prevents workflow authors from accidentally or intentionally sending data to unapproved regions or providers:

```yaml
# mantle.yaml
engine:
  allowed_base_urls:
    - "https://bedrock-runtime.eu-west-1.amazonaws.com"
    - "https://my-internal-llm.corp.example.com"
```

Any `ai/completion` step that specifies a `base_url` not on this list is rejected at validation time.

### EU Compliance Example

To keep all data within the EU:

1. **Deploy Postgres in an EU region** (e.g., AWS `eu-west-1`, GCP `europe-west1`, or an EU-based self-hosted server)
2. **Use EU-region AI endpoints** -- AWS Bedrock in `eu-west-1`, Azure OpenAI in `westeurope`, or a self-hosted model in your EU infrastructure
3. **Restrict endpoints** with `engine.allowed_base_urls` to prevent calls to US-based APIs
4. **Deploy the Mantle binary in the same EU region** to avoid cross-border traffic between the application and the database

## Script Injection

The `browser/run` connector concatenates the `script` param into a wrapper template. If the script value contains CEL expressions that resolve untrusted external data (trigger bodies, webhook payloads), structural injection is possible inside the container sandbox.

**Mitigation:** Pass untrusted data via the `env` param, not the `script` field. Environment variables are treated as string values, not executable code.

## Error String Leakage

When `continue_on_error: true` is used, the `steps.<name>.error` field contains the raw error message from the failed connector. These strings may include infrastructure details:

- Database hostnames and connection strings
- S3 bucket names and object paths
- Docker image names and container IDs
- IMAP/SMTP server banners
- File system paths

**Mitigation:** Do not forward raw error strings to external notification channels. Use conditional expressions that provide user-friendly messages, and rely on `mantle logs` for detailed diagnostics.
