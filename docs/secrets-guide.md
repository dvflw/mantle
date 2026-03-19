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
2. **Environment variable fallback** -- checks for `MANTLE_SECRET_<UPPER_NAME>`

The environment variable name is derived from the credential name by converting to uppercase and replacing hyphens with underscores. For example:

| Credential Name | Env Var Fallback |
|---|---|
| `my-openai` | `MANTLE_SECRET_MY_OPENAI` |
| `my_api_key` | `MANTLE_SECRET_MY_API_KEY` |
| `prod-token` | `MANTLE_SECRET_PROD_TOKEN` |

The env var fallback returns the value as a single `key` field, equivalent to a `generic` credential. This is useful for local development or CI where you do not want to set up the full encryption pipeline.

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
- [Workflow Reference](workflow-reference.md) -- the `credential` field on steps and the AI connector
- [Configuration](configuration.md) -- setting `encryption.key` and `MANTLE_ENCRYPTION_KEY`
- [Concepts](concepts.md) -- how credential resolution fits into the engine architecture
