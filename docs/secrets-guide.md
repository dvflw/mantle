# Secrets Management Guide

Mantle includes a built-in secrets system for storing API keys, tokens, and other credentials. Credentials are encrypted at rest and resolved at connector invocation time -- they are never exposed in CEL expressions, workflow logs, or step outputs.

This guide covers setting up encryption, creating and managing credentials, and using them in workflows.

## Why Secrets Exist

Hardcoding API keys in workflow YAML files is a security risk. Anyone with access to the file sees the raw key. Mantle solves this with typed credentials that are:

- **Encrypted at rest** using AES-256-GCM
- **Resolved at runtime** inside the connector, not in the expression engine
- **Never logged** -- credential values do not appear in execution logs or step outputs

You reference credentials by name in your workflow YAML. The engine resolves the name to decrypted field values at the moment a connector needs them.

## Setting Up the Encryption Key

The secrets system requires a 32-byte hex-encoded encryption key. You configure it in one of two ways:

**Environment variable (recommended for production):**

```bash
export MANTLE_ENCRYPTION_KEY="your-64-char-hex-string-here"
```

**Config file:**

```yaml
# mantle.yaml
encryption:
  key: "your-64-char-hex-string-here"
```

The encryption key is required for all `mantle secrets` commands. Other Mantle commands (like `validate`, `apply`, and `run`) work without it -- you only need the key when creating, listing, deleting, or rotating credentials, or when running workflows that reference credentials.

### Generating a Key

Generate a cryptographically secure 32-byte key with `openssl`:

```bash
openssl rand -hex 32
```

This outputs a 64-character hex string suitable for `MANTLE_ENCRYPTION_KEY`.

Alternatively, the `mantle secrets rotate-key` command auto-generates a new key if you do not provide one (see [Key Rotation](#key-rotation) below).

## Creating Credentials

Use `mantle secrets create` to store a new credential:

```bash
mantle secrets create --name my-openai --type openai --field api_key=sk-proj-abc123
```

The `--field` flag is repeatable. Pass one `--field key=value` per credential field.

**Example with multiple fields:**

```bash
mantle secrets create --name my-openai --type openai \
  --field api_key=sk-proj-abc123 \
  --field org_id=org-xyz789
```

**Example with basic auth:**

```bash
mantle secrets create --name my-api --type basic \
  --field username=admin \
  --field password=s3cret
```

On success, you see:

```
Created credential "my-openai" (type: openai)
```

## Credential Types

Each credential type has a defined set of fields. Mantle validates that all required fields are present when you create a credential.

| Type | Required Fields | Optional Fields | Use Case |
|---|---|---|---|
| `generic` | `key` | -- | General-purpose API key or secret value |
| `bearer` | `token` | -- | Bearer token authentication |
| `openai` | `api_key` | `org_id` | OpenAI API access |
| `basic` | `username`, `password` | -- | HTTP Basic authentication |

If you pass a field name that does not belong to the credential type, it is silently ignored. If you omit a required field, you get an error:

```
Error: field "api_key" is required for credential type "openai"
```

## Listing Credentials

List all stored credentials with `mantle secrets list`:

```bash
$ mantle secrets list
NAME        TYPE    CREATED
my-openai   openai  2026-03-18 14:30:00
my-api      basic   2026-03-18 14:35:00
```

This command shows name, type, and creation date. It never displays decrypted values.

If no credentials exist:

```
(no credentials)
```

## Deleting Credentials

Delete a credential by name:

```bash
$ mantle secrets delete --name my-openai
Deleted credential "my-openai"
```

Deleting a credential that is referenced by a workflow does not affect stored workflow definitions. The workflow fails at execution time if the credential cannot be resolved.

## Using Credentials in Workflows

Reference a stored credential by adding the `credential` field to a step:

```yaml
steps:
  - name: call-openai
    action: ai/completion
    credential: my-openai
    params:
      model: gpt-4o
      prompt: "Summarize this text: {{ steps.fetch-data.output.body }}"
```

When the engine reaches this step, it resolves `my-openai` to its decrypted field values and passes them to the connector. For the `openai` credential type, the AI connector uses the `api_key` field for the `Authorization` header and the `org_id` field (if present) for the `OpenAI-Organization` header.

The `credential` field works on any step, not just AI steps. For example, you can use a `bearer` credential with an HTTP request:

```yaml
steps:
  - name: fetch-data
    action: http/request
    credential: my-api-token
    params:
      method: GET
      url: https://api.example.com/data
```

### Credential Resolution Order

When a step references a credential, the engine resolves it in this order:

1. **Postgres credentials table** -- looks up the credential by name and decrypts it
2. **Cloud secret backends** -- tries each configured backend in registration order (AWS, GCP, Azure)
3. **Environment variable fallback** -- checks for `MANTLE_SECRET_<UPPER_NAME>`

The first source to return a value wins. If the credential is not found in any source, the command fails with an error listing all sources that were checked.

The environment variable name is derived from the credential name by converting to uppercase and replacing hyphens with underscores. For example:

| Credential Name | Env Var Fallback |
|---|---|
| `my-openai` | `MANTLE_SECRET_MY_OPENAI` |
| `my_api_key` | `MANTLE_SECRET_MY_API_KEY` |
| `prod-token` | `MANTLE_SECRET_PROD_TOKEN` |

The env var fallback returns the value as a single `key` field, equivalent to a `generic` credential. This is useful for local development or CI where you do not want to set up the full encryption pipeline.

## Cloud Secret Backends

Mantle can resolve credentials from external cloud secret stores in addition to the local Postgres table. This lets you centralize secret management across your infrastructure.

All cloud backends share the same behavior: if the secret value in the cloud store is valid JSON that decodes to a flat `map[string]string`, those key-value pairs are returned directly as credential fields. If the value is an opaque string, it is returned as `{"key": "<value>"}`, equivalent to a `generic` credential.

### AWS Secrets Manager

Mantle resolves secrets from AWS Secrets Manager using the standard AWS SDK credential chain (environment variables, shared config, EC2/ECS instance profile).

**IAM permissions required:**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "secretsmanager:GetSecretValue",
      "Resource": "arn:aws:secretsmanager:*:*:secret:*"
    }
  ]
}
```

Scope the `Resource` field to limit access to specific secrets.

**Secret naming:** The credential name in your workflow maps 1:1 to the secret name in AWS Secrets Manager. If your workflow step references `credential: my-openai`, Mantle calls `GetSecretValue` with `SecretId: "my-openai"`.

**Storing structured credentials in AWS:**

Store the secret as a JSON object to provide multiple fields:

```bash
aws secretsmanager create-secret \
  --name my-openai \
  --secret-string '{"api_key":"sk-proj-abc123","org_id":"org-xyz789"}'
```

The connector receives the parsed fields `api_key` and `org_id`, the same as a Postgres-stored `openai` credential.

**Storing simple values:**

```bash
aws secretsmanager create-secret \
  --name my-api-key \
  --secret-string "sk-proj-abc123"
```

This is returned as `{"key": "sk-proj-abc123"}`.

**Setup:** Configure the AWS region through the standard `AWS_REGION` or `AWS_DEFAULT_REGION` environment variables. The backend uses `config.LoadDefaultConfig`, so all standard AWS credential sources are supported.

### GCP Secret Manager

Mantle resolves secrets from GCP Secret Manager using Application Default Credentials (ADC).

**IAM permissions required:**

The service account or user needs the `secretmanager.versions.access` permission on the target secrets. The simplest way is to grant the `roles/secretmanager.secretAccessor` role:

```bash
gcloud secrets add-iam-policy-binding my-openai \
  --member="serviceAccount:mantle@my-project.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"
```

**Secret naming:** The credential name maps to the secret name in GCP. Mantle constructs the resource path as `projects/{project_id}/secrets/{name}/versions/latest`, always fetching the latest version.

**GCP project ID:** The backend requires a GCP project ID. Configure this through the `MANTLE_GCP_PROJECT` environment variable or the ADC project setting.

**Storing credentials in GCP:**

```bash
echo -n '{"api_key":"sk-proj-abc123","org_id":"org-xyz789"}' | \
  gcloud secrets create my-openai --data-file=-
```

**Authentication setup:** Configure ADC through one of these methods:

- `gcloud auth application-default login` (local development)
- Workload Identity on GKE
- Service account key file (set `GOOGLE_APPLICATION_CREDENTIALS`)

### Azure Key Vault

Mantle resolves secrets from Azure Key Vault using the DefaultAzureCredential chain (environment variables, managed identity, Azure CLI, etc.).

**Required permissions:** The identity needs the `Get` secret permission on the Key Vault. Assign it through Azure RBAC or Key Vault access policies:

```bash
az keyvault set-policy --name my-vault \
  --object-id <principal-id> \
  --secret-permissions get
```

**Vault URL:** Configure the Key Vault URL through the `MANTLE_AZURE_VAULT_URL` environment variable (e.g., `https://my-vault.vault.azure.net/`).

**Secret naming:** The credential name maps 1:1 to the secret name in Key Vault. Mantle always fetches the latest version.

**Storing credentials in Azure:**

```bash
az keyvault secret set --vault-name my-vault \
  --name my-openai \
  --value '{"api_key":"sk-proj-abc123","org_id":"org-xyz789"}'
```

**Authentication setup:** Configure DefaultAzureCredential through one of these methods:

- `az login` (local development)
- Managed Identity on Azure VMs, App Service, or AKS
- Environment variables: `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_CLIENT_SECRET`

### Resolution Order with Cloud Backends

When cloud backends are configured, the full resolution order is:

```
1. Postgres store (if encryption key is configured)
2. AWS Secrets Manager (if AWS credentials are available)
3. GCP Secret Manager (if GCP project is configured)
4. Azure Key Vault (if vault URL is configured)
5. Environment variable fallback (MANTLE_SECRET_<NAME>)
```

Cloud backends are tried in the order they are registered. The engine falls through to the next source on any error (not found, permission denied, network timeout). This means a credential stored in both Postgres and AWS resolves from Postgres because it is checked first.

**When to use cloud backends vs. Postgres:**

| Scenario | Recommendation |
|---|---|
| Single-instance deployment | Postgres store is simplest |
| Secrets shared across multiple services | Cloud backend (centralized management) |
| Compliance requirement for a specific vault | Cloud backend |
| Local development | Environment variable fallback |

## Key Rotation

If your encryption key is compromised or your security policy requires periodic rotation, use `mantle secrets rotate-key` to re-encrypt all credentials with a new key:

**Auto-generate a new key:**

```bash
$ mantle secrets rotate-key
Re-encrypted 3 credential(s).
New key: a1b2c3d4e5f6...
Update MANTLE_ENCRYPTION_KEY to the new key and restart.
```

**Provide a specific new key:**

```bash
$ mantle secrets rotate-key --new-key "your-new-64-char-hex-string"
Re-encrypted 3 credential(s).
New key: your-new-64-char-hex-string
Update MANTLE_ENCRYPTION_KEY to the new key and restart.
```

After rotating, you must update `MANTLE_ENCRYPTION_KEY` (or `encryption.key` in `mantle.yaml`) to the new key before running any other secrets commands.

The rotation command decrypts all credentials with the current key and re-encrypts them with the new key in a single operation.

## Security Model

- **Encryption algorithm**: AES-256-GCM (authenticated encryption with associated data)
- **Key storage**: The encryption key is not stored in the database. You manage it through environment variables or the config file.
- **At rest**: All credential field values are encrypted before being written to Postgres. The credential name and type are stored in plaintext for listing and lookup.
- **In expressions**: Credentials are never available in CEL expressions. You cannot reference `steps.my-step.credential` or access decrypted values through template interpolation. The credential is resolved inside the connector implementation.
- **In logs**: Credential values never appear in execution logs, step outputs, or error messages.

## Further Reading

- [CLI Reference](cli-reference.md) -- full flag documentation for all secrets commands
- [Workflow Reference](workflow-reference.md) -- the `credential` field on steps and all connectors
- [Configuration](configuration.md) -- setting `encryption.key`, `MANTLE_ENCRYPTION_KEY`, and cloud backend env vars
- [Concepts](concepts.md) -- how credential resolution fits into the engine architecture
