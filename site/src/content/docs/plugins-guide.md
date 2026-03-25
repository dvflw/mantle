# Plugins Guide

Plugins extend Mantle with third-party connector actions. A plugin is an executable binary that communicates with the engine over a JSON stdin/stdout protocol. Plugins run as subprocesses -- they cannot access the engine's memory, database, or internal state directly.

This guide covers what plugins are, how to write one, and how to install and manage them.

## When to Use a Plugin

Use a plugin when you need a connector action that Mantle does not provide out of the box. The built-in connectors cover HTTP, AI/LLM, Slack, Postgres, Email, S3, and Docker. For anything else -- a proprietary API, a custom data transformation, a niche SaaS integration -- write a plugin.

Plugins are the right choice when:

- You need to call an API that requires custom authentication or request formatting
- You want to reuse a connector across multiple Mantle installations
- You need to keep proprietary integration logic separate from the open-source core

## How Plugins Work

When a workflow step references a plugin action, the engine:

1. Looks up the plugin binary in the `.mantle/plugins/` directory
2. Spawns it as a subprocess
3. Writes a JSON request to the plugin's stdin
4. Reads the JSON response from the plugin's stdout
5. Terminates the subprocess

Each step execution spawns a fresh process. There is no persistent connection or shared state between invocations.

```
Engine                        Plugin Process
  |                                 |
  |-- spawn subprocess ------------>|
  |-- write JSON to stdin --------->|
  |                                 |-- parse input
  |                                 |-- execute action
  |                                 |-- write JSON to stdout
  |<-- read JSON from stdout -------|
  |-- subprocess exits ------------>|
```

The plugin has a 60-second timeout by default. If it does not produce output within that window, the engine kills the process and fails the step.

## The JSON Protocol

### Input (stdin)

The engine writes a single JSON object to the plugin's stdin:

```json
{
  "action": "my-plugin/fetch-data",
  "params": {
    "url": "https://api.example.com/data",
    "limit": 100
  },
  "credential": {
    "api_key": "sk-abc123"
  }
}
```

| Field | Type | Description |
|---|---|---|
| `action` | string | The full action name from the workflow step (e.g., `my-plugin/fetch-data`). |
| `params` | object | The `params` map from the workflow step, with CEL expressions already evaluated. |
| `credential` | object | Decrypted credential fields, if the step has a `credential` reference. Empty object if no credential. |

### Output (stdout)

The plugin writes a single JSON object to stdout:

```json
{
  "result": "success",
  "items": [
    {"id": 1, "name": "Item A"},
    {"id": 2, "name": "Item B"}
  ]
}
```

The output object becomes the step's `output` in subsequent CEL expressions. For example, `steps['my-step'].output.items[0].name` evaluates to `"Item A"`.

### Errors

If the plugin encounters an error, it should write a message to stderr and exit with a non-zero exit code. The engine captures stderr and reports it as the step error:

```
step.failed: plugin "my-plugin" failed: API returned 403 Forbidden
```

Do not write error details to stdout -- the engine only parses stdout as the output object.

## Writing a Plugin

A plugin can be written in any language. The only requirements are:

1. It is an executable binary (or a script with a shebang line)
2. It reads JSON from stdin
3. It writes JSON to stdout
4. It exits with code 0 on success or non-zero on failure

### Example: Python Plugin

This minimal plugin fetches data from a custom API:

```python
#!/usr/bin/env python3
"""my-api-connector — a Mantle plugin for the Example API."""

import json
import sys
import urllib.request

def main():
    # Read input from stdin.
    raw = sys.stdin.read()
    request = json.loads(raw)

    action = request["action"]
    params = request["params"]
    credential = request.get("credential", {})

    if action != "example-api/fetch":
        print(f"Unknown action: {action}", file=sys.stderr)
        sys.exit(1)

    # Build the API request.
    url = params["url"]
    api_key = credential.get("api_key", "")

    req = urllib.request.Request(url)
    req.add_header("Authorization", f"Bearer {api_key}")

    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            body = json.loads(resp.read())
    except Exception as e:
        print(f"Request failed: {e}", file=sys.stderr)
        sys.exit(1)

    # Write output to stdout.
    output = {
        "status": resp.status,
        "data": body,
    }
    json.dump(output, sys.stdout)

if __name__ == "__main__":
    main()
```

Make it executable:

```bash
chmod +x my-api-connector
```

### Example: Go Plugin

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
)

type Input struct {
    Action     string            `json:"action"`
    Params     map[string]any    `json:"params"`
    Credential map[string]string `json:"credential"`
}

func main() {
    var input Input
    if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
        fmt.Fprintf(os.Stderr, "failed to parse input: %s\n", err)
        os.Exit(1)
    }

    // Implement your connector logic here.
    output := map[string]any{
        "result": "ok",
        "action": input.Action,
    }

    if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
        fmt.Fprintf(os.Stderr, "failed to write output: %s\n", err)
        os.Exit(1)
    }
}
```

Build and install:

```bash
go build -o my-go-connector .
mantle plugins install ./my-go-connector
```

### Example: Shell Script Plugin

For quick prototyping, a shell script works:

```bash
#!/bin/bash
# Read the full JSON input.
INPUT=$(cat)

# Extract fields with jq.
ACTION=$(echo "$INPUT" | jq -r '.action')
URL=$(echo "$INPUT" | jq -r '.params.url')

# Do the work.
RESPONSE=$(curl -sf "$URL")
if [ $? -ne 0 ]; then
    echo "HTTP request failed" >&2
    exit 1
fi

# Write JSON output.
echo "{\"body\": $RESPONSE}"
```

## Installing and Managing Plugins

### Install

Copy a plugin binary into the plugin directory:

```bash
mantle plugins install ./path/to/my-plugin
```

This copies the file to `.mantle/plugins/my-plugin`. The plugin name is derived from the filename.

### List

See all installed plugins:

```bash
mantle plugins list
```

### Remove

Remove a plugin by name:

```bash
mantle plugins remove my-plugin
```

This deletes the binary from the plugin directory.

### Plugin Directory

Plugins are stored in `.mantle/plugins/` relative to the current working directory. The directory is created automatically when you install the first plugin.

```
.mantle/
  plugins/
    my-api-connector
    my-go-connector
    data-transformer
```

## Using a Plugin in a Workflow

Reference the plugin action in a step's `action` field. The action name is `<plugin-name>/<action>`:

```yaml
name: custom-integration
steps:
  - name: fetch-external-data
    action: example-api/fetch
    credential: my-api-key
    timeout: "30s"
    params:
      url: "https://api.example.com/data"
      limit: 100

  - name: process-data
    action: ai/completion
    credential: my-openai
    params:
      model: gpt-4o
      prompt: "Summarize: {{ steps['fetch-external-data'].output.data }}"
```

Plugins work with all standard step features: `if` conditions, `retry`, `timeout`, and `credential` resolution.

## The Protobuf Specification

The formal plugin contract is defined in `proto/connector.proto`. While the current implementation uses JSON stdin/stdout, the protobuf definition serves as the specification for a future gRPC-based protocol.

The service defines three RPCs:

```protobuf
service Connector {
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);
  rpc Validate(ValidateRequest) returns (ValidateResponse);
  rpc Describe(DescribeRequest) returns (DescribeResponse);
}
```

- **Execute** -- runs the action with parameters and credentials. This is the only RPC that the JSON protocol currently implements.
- **Validate** -- checks whether parameters are valid without executing. Planned for a future version.
- **Describe** -- returns metadata about the plugin's supported actions. Planned for a future version.

## Best Practices

- **Keep plugins stateless.** Each invocation is a fresh process. Do not rely on files, environment variables, or other side effects from previous runs.
- **Validate input early.** Check for required params and credential fields before doing any work. Exit with a clear error message on stderr.
- **Set timeouts on external calls.** The engine applies a 60-second timeout to the subprocess, but your plugin should set its own timeouts on network requests to fail gracefully.
- **Test with stdin/stdout directly.** You can test a plugin without Mantle by piping JSON:

```bash
echo '{"action":"my-plugin/fetch","params":{"url":"https://example.com"},"credential":{}}' | ./my-plugin
```

- **Keep output small.** The entire stdout output is stored as the step's output in the database checkpoint. Avoid returning megabytes of data -- filter and summarize in the plugin.

## Further Reading

- [CLI Reference](cli-reference.md#mantle-plugins) -- `mantle plugins` command documentation
- [Workflow Reference](workflow-reference.md#connectors) -- connector actions and the `action` field
- [Concepts](concepts.md#plugin-system) -- architectural overview of the plugin system
