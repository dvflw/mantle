# Bedrock Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add AWS Bedrock as a second AI provider behind a `LLMProvider` interface, with shared AWS config, IAM-based auth, and full tool-use parity.

**Architecture:** Extract a `LLMProvider` interface from the existing OpenAI-specific `AIConnector`. The `AIConnector` becomes a dispatcher that routes to the correct provider based on the `provider` step param. A shared `NewAWSConfig` helper builds AWS SDK configs for both Bedrock and S3 connectors. The Bedrock provider uses the Converse API for completions, structured output, and tool use.

**Tech Stack:** `github.com/aws/aws-sdk-go-v2` (already in go.mod), `github.com/aws/aws-sdk-go-v2/service/bedrockruntime` (new), `github.com/coreos/go-oidc/v3` (existing)

**Linear Issue:** DVFLW-269

---

## File Structure

| File | Responsibility |
|------|---------------|
| **Create:** `internal/connector/provider.go` | `LLMProvider` interface, `ChatRequest`/`ChatResponse`/`ChatMessage`/`ChatTool`/`ChatUsage` types (reuses existing `ToolCall`/`ToolFunction` from `ai.go`) |
| **Create:** `internal/connector/provider_openai.go` | OpenAI provider — extracted from current `ai.go` |
| **Create:** `internal/connector/provider_openai_test.go` | OpenAI provider unit tests |
| **Create:** `internal/connector/provider_bedrock.go` | Bedrock provider — Converse API implementation |
| **Create:** `internal/connector/provider_bedrock_test.go` | Bedrock provider unit tests with mocked AWS SDK |
| **Create:** `internal/connector/awsclient.go` | `NewAWSConfig` shared helper — credential chain + region resolution |
| **Create:** `internal/connector/awsclient_test.go` | AWS config builder tests |
| **Modify:** `internal/connector/ai.go` | Refactor to dispatcher — route by `provider` param to `LLMProvider` |
| **Modify:** `internal/connector/ai_test.go` | Update tests for dispatcher + provider routing |
| **Modify:** `internal/connector/s3.go` | Replace `newS3Client` with shared `NewAWSConfig` |
| **Modify:** `internal/connector/tools.go` | Update `AIExecuteFunc` to work with normalized types (minimal change) |
| **Modify:** `internal/secret/types.go` | Add `aws` credential type |
| **Modify:** `internal/config/config.go` | Add `CloudConfig` with `AWSConfig`, `GCPConfig`, `AzureConfig` |
| **Modify:** `internal/engine/tooluse.go` | Convert OpenAI-specific tool format to provider-agnostic format |

**Important:** Tasks 1-3 are the foundation (types, config, AWS helper). Tasks 4-5 extract OpenAI into the provider interface. Task 6 adds Bedrock. Task 7 migrates S3. Task 8 updates the engine integration. Tasks must be executed sequentially.

---

### Task 1: Provider interface and common types

**Files:**
- Create: `internal/connector/provider.go`

- [ ] **Step 1: Create the provider interface and types**

```go
package connector

import "context"

// LLMProvider implements a specific AI provider's chat completion API.
// Each provider translates between these normalized types and its native format.
type LLMProvider interface {
	ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
}

// ChatRequest is the provider-agnostic request format.
type ChatRequest struct {
	Model        string
	Messages     []ChatMessage
	Tools        []ChatTool
	OutputSchema map[string]any
	MaxTokens    int
	Credential   map[string]string // resolved credential fields
}

// ChatMessage represents a single message in the conversation.
// Valid Role values: "system", "user", "assistant", "tool".
// For "tool" messages (tool results), set ToolCallID to match the original call.
// Note: "tool" role maps to ConversationRoleUser in Bedrock (tool results are user messages).
type ChatMessage struct {
	Role       string     // "system", "user", "assistant", "tool"
	Content    string
	ToolCalls  []ToolCall // assistant messages with function calls (reuses existing ToolCall type from ai.go)
	ToolCallID string     // tool result messages — matches ToolCall.ID
}

// ChatTool defines a tool the model can invoke.
type ChatTool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// ChatResponse is the provider-agnostic response format.
type ChatResponse struct {
	Text         string
	ToolCalls    []ToolCall
	FinishReason string // "stop" or "tool_calls"
	Usage        ChatUsage
	Model        string
}

// ChatUsage tracks token consumption.
type ChatUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/connector/...`
Expected: PASS (new file, no consumers yet)

- [ ] **Step 3: Commit**

```bash
git add internal/connector/provider.go
git commit -m "feat(ai): add LLMProvider interface and common chat types"
```

---

### Task 2: Cloud config and AWS credential type

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/secret/types.go`

- [ ] **Step 1: Write config test**

Add to `internal/config/config_test.go` or create `internal/config/config_cloud_test.go`:

```go
func TestLoadCloudConfig(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "mantle.yaml")
	err := os.WriteFile(cfgFile, []byte(`
aws:
  region: us-west-2
gcp:
  region: us-central1
azure:
  region: eastus
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cmd := newTestCommand()
	cmd.SetArgs([]string{"--config", cfgFile})
	_ = cmd.Execute()
	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AWS.Region != "us-west-2" {
		t.Errorf("aws.region = %q, want %q", cfg.AWS.Region, "us-west-2")
	}
	if cfg.GCP.Region != "us-central1" {
		t.Errorf("gcp.region = %q, want %q", cfg.GCP.Region, "us-central1")
	}
	if cfg.Azure.Region != "eastus" {
		t.Errorf("azure.region = %q, want %q", cfg.Azure.Region, "eastus")
	}
}
```

- [ ] **Step 2: Run test — verify it fails**

Run: `go test ./internal/config/ -run TestLoadCloudConfig -v`
Expected: FAIL — `cfg.AWS` does not exist

- [ ] **Step 3: Add cloud config structs**

In `internal/config/config.go`, add to `Config`:

```go
type Config struct {
	// ... existing fields ...
	AWS   AWSConfig   `mapstructure:"aws"`
	GCP   GCPConfig   `mapstructure:"gcp"`
	Azure AzureConfig `mapstructure:"azure"`
}

type AWSConfig struct {
	Region string `mapstructure:"region"`
}

type GCPConfig struct {
	Region string `mapstructure:"region"`
}

type AzureConfig struct {
	Region string `mapstructure:"region"`
}
```

Add env var bindings in `Load()`:

```go
_ = v.BindEnv("aws.region", "MANTLE_AWS_REGION")
_ = v.BindEnv("gcp.region", "MANTLE_GCP_REGION")
_ = v.BindEnv("azure.region", "MANTLE_AZURE_REGION")
```

- [ ] **Step 4: Run config test — verify it passes**

Run: `go test ./internal/config/ -run TestLoadCloudConfig -v`
Expected: PASS

- [ ] **Step 5: Add `aws` credential type**

In `internal/secret/types.go`, add to `builtinTypes`:

```go
"aws": {
	Name: "aws",
	Fields: []FieldDef{
		{Name: "access_key_id", Required: true},
		{Name: "secret_access_key", Required: true},
		{Name: "region", Required: false},
		{Name: "session_token", Required: false},
	},
},
```

Update the error message in `GetType` to include `aws` in the available types list.

- [ ] **Step 6: Verify build and commit**

```bash
go build ./...
git add internal/config/config.go internal/config/config_cloud_test.go internal/secret/types.go
git commit -m "feat: add cloud provider config (aws/gcp/azure) and aws credential type"
```

---

### Task 3: Shared AWS config builder

**Files:**
- Create: `internal/connector/awsclient.go`
- Create: `internal/connector/awsclient_test.go`

- [ ] **Step 1: Write failing tests**

```go
package connector

import (
	"context"
	"os"
	"testing"
)

func TestNewAWSConfig_ExplicitCredentials(t *testing.T) {
	cred := map[string]string{
		"access_key_id":     "AKIATEST",
		"secret_access_key": "secret123",
		"region":            "eu-west-1",
	}
	cfg, err := NewAWSConfig(context.Background(), cred, "us-east-1")
	if err != nil {
		t.Fatalf("NewAWSConfig: %v", err)
	}
	if cfg.Region != "eu-west-1" {
		t.Errorf("region = %q, want %q (credential should override default)", cfg.Region, "eu-west-1")
	}
	// Verify credentials are static, not from chain
	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if creds.AccessKeyID != "AKIATEST" {
		t.Errorf("access_key = %q, want AKIATEST", creds.AccessKeyID)
	}
}

func TestNewAWSConfig_DefaultRegionFallback(t *testing.T) {
	cred := map[string]string{
		"access_key_id":     "AKIATEST",
		"secret_access_key": "secret123",
	}
	cfg, err := NewAWSConfig(context.Background(), cred, "ap-southeast-1")
	if err != nil {
		t.Fatalf("NewAWSConfig: %v", err)
	}
	if cfg.Region != "ap-southeast-1" {
		t.Errorf("region = %q, want %q (should use default)", cfg.Region, "ap-southeast-1")
	}
}

func TestNewAWSConfig_SessionToken(t *testing.T) {
	cred := map[string]string{
		"access_key_id":     "AKIATEST",
		"secret_access_key": "secret123",
		"session_token":     "FwoGZX...",
		"region":            "us-east-1",
	}
	cfg, err := NewAWSConfig(context.Background(), cred, "")
	if err != nil {
		t.Fatalf("NewAWSConfig: %v", err)
	}
	creds, _ := cfg.Credentials.Retrieve(context.Background())
	if creds.SessionToken != "FwoGZX..." {
		t.Errorf("session_token not set")
	}
}

func TestNewAWSConfig_NilCredential_UsesDefaultChain(t *testing.T) {
	// Unset AWS env vars to ensure clean test
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_REGION", "us-west-2")

	cfg, err := NewAWSConfig(context.Background(), nil, "us-east-1")
	if err != nil {
		t.Fatalf("NewAWSConfig: %v", err)
	}
	// When no credential provided, should use default chain
	// Region falls back: nil credential has no region → default "us-east-1"
	if cfg.Region != "us-east-1" {
		t.Errorf("region = %q, want us-east-1", cfg.Region)
	}
}

func TestNewAWSConfig_EnvRegionFallback(t *testing.T) {
	t.Setenv("AWS_REGION", "eu-north-1")
	cfg, err := NewAWSConfig(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("NewAWSConfig: %v", err)
	}
	// Empty default + no credential region → should pick up AWS_REGION from SDK default config
	// The SDK's LoadDefaultConfig respects AWS_REGION
	if cfg.Region == "" {
		t.Error("region should not be empty when AWS_REGION is set")
	}
}
```

- [ ] **Step 2: Run tests — verify fail**

Run: `go test ./internal/connector/ -run TestNewAWSConfig -v`
Expected: FAIL — `NewAWSConfig` undefined

- [ ] **Step 3: Implement the shared AWS config builder**

Create `internal/connector/awsclient.go`:

```go
package connector

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// NewAWSConfig builds an aws.Config using the provided credential map and default region.
//
// Credential resolution:
//   - If credential is non-nil and contains access_key_id + secret_access_key,
//     uses static credentials. Optional session_token for temporary creds.
//   - If credential is nil, uses the AWS SDK default credential chain
//     (env vars → config file → IRSA → instance metadata).
//
// Region resolution:
//  1. credential["region"] (if present)
//  2. defaultRegion parameter (from mantle.yaml aws.region or step param)
//  3. AWS SDK default (AWS_REGION env var, config file, etc.)
func NewAWSConfig(ctx context.Context, credential map[string]string, defaultRegion string) (aws.Config, error) {
	var opts []func(*awsconfig.LoadOptions) error

	// Credential: explicit static or SDK default chain
	if credential != nil {
		accessKey := credential["access_key_id"]
		secretKey := credential["secret_access_key"]
		if accessKey != "" && secretKey != "" {
			sessionToken := credential["session_token"]
			opts = append(opts, awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(accessKey, secretKey, sessionToken),
			))
		}
	}

	// Region: credential > default > SDK chain
	region := ""
	if credential != nil {
		region = credential["region"]
	}
	if region == "" {
		region = defaultRegion
	}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("loading AWS config: %w", err)
	}

	if cfg.Region == "" {
		return aws.Config{}, fmt.Errorf("AWS region is required: set via credential, aws.region config, or AWS_REGION env var")
	}

	return cfg, nil
}
```

- [ ] **Step 4: Run tests — verify pass**

Run: `go test ./internal/connector/ -run TestNewAWSConfig -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/connector/awsclient.go internal/connector/awsclient_test.go
git commit -m "feat: add shared AWS config builder with credential chain and region resolution"
```

---

### Task 4: Extract OpenAI into LLMProvider

**Files:**
- Create: `internal/connector/provider_openai.go`
- Create: `internal/connector/provider_openai_test.go`
- Modify: `internal/connector/ai.go`

- [ ] **Step 1: Create OpenAI provider**

Create `internal/connector/provider_openai.go`. Extract the HTTP call logic from `ai.go` into an `OpenAIProvider` that implements `LLMProvider`:

```go
package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIProvider implements LLMProvider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	Client  *http.Client
	BaseURL string // defaults to "https://api.openai.com/v1"
}

func (p *OpenAIProvider) ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Build OpenAI messages from ChatMessage
	messages := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		msg := map[string]any{"role": m.Role, "content": m.Content}
		if len(m.ToolCalls) > 0 {
			msg["tool_calls"] = m.ToolCalls
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		messages = append(messages, msg)
	}

	body := map[string]any{
		"model":    req.Model,
		"messages": messages,
	}

	if len(req.Tools) > 0 {
		openaiTools := make([]map[string]any, len(req.Tools))
		for i, t := range req.Tools {
			openaiTools[i] = map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.InputSchema,
				},
			}
		}
		body["tools"] = openaiTools
	}

	if req.OutputSchema != nil {
		body["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "response",
				"strict": true,
				"schema": req.OutputSchema,
			},
		}
	}

	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}

	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("openai: creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Auth from credential
	if cred := req.Credential; cred != nil {
		switch {
		case cred["api_key"] != "":
			httpReq.Header.Set("Authorization", "Bearer "+cred["api_key"])
		case cred["token"] != "":
			httpReq.Header.Set("Authorization", "Bearer "+cred["token"])
		case cred["key"] != "":
			httpReq.Header.Set("Authorization", "Bearer "+cred["key"])
		}
		if orgID := cred["org_id"]; orgID != "" {
			httpReq.Header.Set("OpenAI-Organization", orgID)
		}
	}

	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("openai: reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openai: API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var apiResp chatCompletionResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("openai: parsing response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices returned")
	}

	choice := apiResp.Choices[0]
	chatResp := &ChatResponse{
		Model: apiResp.Model,
		Usage: ChatUsage{
			PromptTokens:     apiResp.Usage.PromptTokens,
			CompletionTokens: apiResp.Usage.CompletionTokens,
			TotalTokens:      apiResp.Usage.TotalTokens,
		},
	}

	if len(choice.Message.ToolCalls) > 0 {
		chatResp.ToolCalls = choice.Message.ToolCalls
		chatResp.FinishReason = "tool_calls"
	} else {
		chatResp.Text = choice.Message.Content
		chatResp.FinishReason = "stop"
	}

	return chatResp, nil
}
```

Note: The `chatCompletionResponse`, `chatMessage`, `ToolCall`, `ToolFunction` types stay in `ai.go` for now since they're used by both the provider and the dispatcher.

- [ ] **Step 2: Write OpenAI provider tests**

Create `internal/connector/provider_openai_test.go` with a mock HTTP server that returns canned OpenAI responses. Test:
- Basic completion (text response)
- Tool calls response
- Structured output request format
- Credential auth header setting
- API error handling

Follow the existing test pattern in `internal/connector/ai_test.go`.

- [ ] **Step 3: Refactor `ai.go` to dispatch by provider**

Refactor `AIConnector.Execute` to:
1. Extract `provider` from params (default: `"openai"`)
2. Build a `ChatRequest` from the raw params
3. Dispatch to the appropriate `LLMProvider`
4. Convert `ChatResponse` back to the `map[string]any` output format the engine expects

```go
func (c *AIConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	model, _ := params["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("ai/completion: model is required")
	}

	providerName, _ := params["provider"].(string)
	if providerName == "" {
		providerName = "openai"
	}

	provider, err := c.getProvider(providerName, params)
	if err != nil {
		return nil, err
	}

	req, err := c.buildChatRequest(params)
	if err != nil {
		return nil, err
	}

	resp, err := provider.ChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ai/completion [%s]: %w", providerName, err)
	}

	return c.chatResponseToOutput(resp), nil
}
```

Add `getProvider`, `buildChatRequest`, and `chatResponseToOutput` helper methods. `getProvider` returns `&OpenAIProvider{...}` for `"openai"`, error for unknown. Bedrock will be added in Task 6.

The `AIConnector` struct gains optional fields for provider configuration:
```go
type AIConnector struct {
	Client         *http.Client
	AWSConfigFunc  func(ctx context.Context, cred map[string]string, defaultRegion string) (aws.Config, error)
	DefaultRegion  string // from mantle.yaml aws.region
}
```

- [ ] **Step 4: Update existing AI tests**

Modify `internal/connector/ai_test.go` — existing tests should still pass since `provider` defaults to `"openai"`. Add a test for explicit `provider: "openai"` and one for unknown provider returning an error.

- [ ] **Step 5: Run all connector tests**

Run: `go test ./internal/connector/ -v -count=1`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/connector/provider_openai.go internal/connector/provider_openai_test.go internal/connector/ai.go internal/connector/ai_test.go
git commit -m "refactor(ai): extract OpenAI into LLMProvider, add provider dispatcher"
```

---

### Task 5: Update ToolLoop for provider-agnostic types

**Files:**
- Modify: `internal/connector/tools.go`
- Modify: `internal/engine/tooluse.go`

- [ ] **Step 1: Update ToolLoop to use ChatMessage for internal message tracking**

The `ToolLoop` currently builds raw `map[string]any` messages in OpenAI format. Since the dispatcher now handles format conversion, the ToolLoop should continue working with `map[string]any` params (the dispatcher converts). Minimal changes needed — verify the `_messages` passthrough still works with the refactored dispatcher.

The key change: the `tool_calls` in the response are now `[]ToolCall` from the `ChatResponse` (via `chatResponseToOutput`), which is the same type as before. Verify this works.

- [ ] **Step 2: Update engine tooluse.go**

The `executeToolUseStep` currently builds OpenAI-format tools:
```go
openaiTools[i] = map[string]any{"type": "function", "function": ...}
```

This should now build provider-agnostic `ChatTool` format instead, and pass them via a new `_chat_tools` param (or let the dispatcher convert). The simplest approach: keep passing `_tools` in OpenAI format, and have each provider convert from that. This minimizes changes.

Actually, since the dispatcher's `buildChatRequest` will read `_tools` and convert to `[]ChatTool`, the engine can keep building `_tools` in the same format. Verify this works end-to-end.

- [ ] **Step 3: Run all tests**

Run: `go test ./internal/connector/... ./internal/engine/... -v -count=1 -timeout 120s`
Expected: All PASS

- [ ] **Step 4: Commit if any changes were needed**

```bash
git add internal/connector/tools.go internal/engine/tooluse.go
git commit -m "refactor: update tool loop for provider-agnostic dispatch"
```

---

### Task 6: Bedrock provider

**Files:**
- Create: `internal/connector/provider_bedrock.go`
- Create: `internal/connector/provider_bedrock_test.go`
- Modify: `internal/connector/ai.go` (add bedrock to `getProvider`)

- [ ] **Step 1: Add bedrockruntime dependency**

Run: `go get github.com/aws/aws-sdk-go-v2/service/bedrockruntime@latest && go mod tidy`

- [ ] **Step 2: Write failing tests with mocked Bedrock client**

Create `internal/connector/provider_bedrock_test.go`. Define a mock interface matching the Bedrock SDK's `Converse` method:

```go
package connector

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// mockBedrockClient implements the BedrockConverseAPI interface for testing.
type mockBedrockClient struct {
	Response *bedrockruntime.ConverseOutput
	Err      error
}

func (m *mockBedrockClient) Converse(ctx context.Context, input *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	return m.Response, m.Err
}
```

Write tests:
- `TestBedrockProvider_BasicCompletion` — mock returns text, verify ChatResponse
- `TestBedrockProvider_ToolCalls` — mock returns tool use, verify ToolCalls in response
- `TestBedrockProvider_StructuredOutput` — verify system prompt includes schema instruction
- `TestBedrockProvider_ThrottlingError` — verify error is marked retryable
- `TestBedrockProvider_AccessDenied` — verify error is non-retryable
- `TestBedrockProvider_InferenceProfileARN` — verify ARN passed as modelId

- [ ] **Step 3: Run tests — verify fail**

Run: `go test ./internal/connector/ -run TestBedrockProvider -v`
Expected: FAIL — `BedrockProvider` undefined

- [ ] **Step 4: Implement Bedrock provider**

Create `internal/connector/provider_bedrock.go`:

```go
package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	smithydocument "github.com/aws/smithy-go/document"
)

// BedrockConverseAPI is the subset of the Bedrock client used by BedrockProvider.
// Defined as an interface for testability.
type BedrockConverseAPI interface {
	Converse(ctx context.Context, input *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
}

// BedrockProvider implements LLMProvider using AWS Bedrock's Converse API.
type BedrockProvider struct {
	Client BedrockConverseAPI
}

func (p *BedrockProvider) ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Convert ChatMessages to Bedrock messages
	var messages []brtypes.Message
	var system []brtypes.SystemContentBlock
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			system = append(system, &brtypes.SystemContentBlockMemberText{Value: m.Content})
		case "user":
			messages = append(messages, brtypes.Message{
				Role:    brtypes.ConversationRoleUser,
				Content: []brtypes.ContentBlock{&brtypes.ContentBlockMemberText{Value: m.Content}},
			})
		case "assistant":
			if len(m.ToolCalls) > 0 {
				var content []brtypes.ContentBlock
				for _, tc := range m.ToolCalls {
					var inputDoc map[string]any
					json.Unmarshal([]byte(tc.Function.Arguments), &inputDoc)
					content = append(content, &brtypes.ContentBlockMemberToolUse{
						Value: brtypes.ToolUseBlock{
							ToolUseId: aws.String(tc.ID),
							Name:      aws.String(tc.Function.Name),
							Input:     mustDocument(inputDoc),
						},
					})
				}
				messages = append(messages, brtypes.Message{
					Role:    brtypes.ConversationRoleAssistant,
					Content: content,
				})
			} else {
				messages = append(messages, brtypes.Message{
					Role:    brtypes.ConversationRoleAssistant,
					Content: []brtypes.ContentBlock{&brtypes.ContentBlockMemberText{Value: m.Content}},
				})
			}
		case "tool":
			messages = append(messages, brtypes.Message{
				Role: brtypes.ConversationRoleUser,
				Content: []brtypes.ContentBlock{&brtypes.ContentBlockMemberToolResult{
					Value: brtypes.ToolResultBlock{
						ToolUseId: aws.String(m.ToolCallID),
						Content:   []brtypes.ToolResultContentBlock{&brtypes.ToolResultContentBlockMemberText{Value: m.Content}},
					},
				}},
			})
		}
	}

	// Build Converse input
	input := &bedrockruntime.ConverseInput{
		ModelId:  aws.String(req.Model),
		Messages: messages,
	}
	if len(system) > 0 {
		input.System = system
	}

	// Convert tools
	if len(req.Tools) > 0 {
		var toolConfigs []brtypes.Tool
		for _, t := range req.Tools {
			toolConfigs = append(toolConfigs, &brtypes.ToolMemberToolSpec{
				Value: brtypes.ToolSpecification{
					Name:        aws.String(t.Name),
					Description: aws.String(t.Description),
					InputSchema: &brtypes.ToolInputSchemaMemberJson{Value: mustDocument(t.InputSchema)},
				},
			})
		}
		input.ToolConfig = &brtypes.ToolConfiguration{Tools: toolConfigs}
	}

	// Structured output: instruct via system prompt (Bedrock doesn't have native json_schema mode)
	if req.OutputSchema != nil {
		schemaJSON, _ := json.Marshal(req.OutputSchema)
		system = append(system, &brtypes.SystemContentBlockMemberText{
			Value: fmt.Sprintf("You must respond with valid JSON matching this schema: %s", schemaJSON),
		})
		input.System = system
	}

	if req.MaxTokens > 0 {
		input.InferenceConfig = &brtypes.InferenceConfiguration{
			MaxTokens: aws.Int32(int32(req.MaxTokens)),
		}
	}

	// Call Bedrock
	output, err := p.Client.Converse(ctx, input)
	if err != nil {
		return nil, classifyBedrockError(err)
	}

	// Parse response
	resp := &ChatResponse{
		Model: req.Model,
	}

	if output.Usage != nil {
		resp.Usage = ChatUsage{
			PromptTokens:     int(aws.ToInt32(output.Usage.InputTokens)),
			CompletionTokens: int(aws.ToInt32(output.Usage.OutputTokens)),
			TotalTokens:      int(aws.ToInt32(output.Usage.InputTokens)) + int(aws.ToInt32(output.Usage.OutputTokens)),
		}
	}

	// Safely extract the output message (type assertion with check)
	outputMsg, ok := output.Output.(*brtypes.ConverseOutputMemberMessage)
	if !ok {
		return nil, fmt.Errorf("bedrock: unexpected output type %T", output.Output)
	}

	if output.StopReason == brtypes.StopReasonToolUse {
		resp.FinishReason = "tool_calls"
		for _, block := range outputMsg.Value.Content {
			if tu, ok := block.(*brtypes.ContentBlockMemberToolUse); ok {
				argsJSON, _ := json.Marshal(tu.Value.Input)
				resp.ToolCalls = append(resp.ToolCalls, ToolCall{
					ID:   aws.ToString(tu.Value.ToolUseId),
					Type: "function",
					Function: ToolFunction{
						Name:      aws.ToString(tu.Value.Name),
						Arguments: string(argsJSON),
					},
				})
			}
		}
	} else {
		resp.FinishReason = "stop"
		for _, block := range outputMsg.Value.Content {
			if text, ok := block.(*brtypes.ContentBlockMemberText); ok {
				resp.Text = text.Value
			}
		}
	}

	return resp, nil
}

// classifyBedrockError wraps AWS errors with retryable/non-retryable classification.
// Uses smithy error code extraction when available, falls back to string matching.
func classifyBedrockError(err error) error {
	// Try AWS SDK error code extraction first (preferred)
	var apiErr interface{ ErrorCode() string }
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "ThrottlingException", "ModelTimeoutException",
			"ServiceUnavailableException", "ModelNotReadyException":
			return &RetryableError{Err: err}
		}
	}

	// Fallback to string matching for wrapped errors
	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, "ThrottlingException"),
		strings.Contains(errMsg, "ModelTimeoutException"),
		strings.Contains(errMsg, "ServiceUnavailableException"):
		return &RetryableError{Err: err}
	default:
		return fmt.Errorf("bedrock: %w", err)
	}
}

// RetryableError signals to the engine that this error is transient and the step can be retried.
// The engine's step execution loop should check errors.As(err, &RetryableError{}) to determine
// if a step failure is retryable. If the engine doesn't already check for this type, add the check
// in internal/engine/engine.go's step execution error handling (near the retry policy logic).
type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string { return e.Err.Error() }
func (e *RetryableError) Unwrap() error { return e.Err }

// mustDocument converts a map to a Bedrock document type using smithy-go.
func mustDocument(m map[string]any) smithydocument.Interface {
	return smithydocument.NewLazyDocument(m)
}
```

**Note:** The exact Bedrock SDK document type handling depends on the SDK version. The implementer should check the actual `bedrockruntime` package for how `document.Interface` works — it may accept `map[string]any` directly or require `smithydocument.NewLazyDocument()`.

- [ ] **Step 5: Wire Bedrock into the dispatcher**

In `internal/connector/ai.go`, update `getProvider`:

```go
func (c *AIConnector) getProvider(name string, params map[string]any) (LLMProvider, error) {
	switch name {
	case "openai":
		baseURL, _ := params["base_url"].(string)
		return &OpenAIProvider{Client: c.Client, BaseURL: baseURL}, nil
	case "bedrock":
		cred, _ := params["_credential"].(map[string]string)
		region, _ := params["region"].(string)
		defaultRegion := c.DefaultRegion
		if region != "" {
			defaultRegion = region
		}
		awsCfg, err := c.AWSConfigFunc(context.Background(), cred, defaultRegion)
		if err != nil {
			return nil, fmt.Errorf("ai/completion [bedrock]: %w", err)
		}
		client := bedrockruntime.NewFromConfig(awsCfg)
		return &BedrockProvider{Client: client}, nil
	default:
		return nil, fmt.Errorf("ai/completion: unknown provider %q (available: openai, bedrock)", name)
	}
}
```

- [ ] **Step 6: Run all tests**

Run: `go test ./internal/connector/ -v -count=1`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/connector/provider_bedrock.go internal/connector/provider_bedrock_test.go internal/connector/ai.go go.mod go.sum
git commit -m "feat(ai): add Bedrock provider with Converse API, tool use, and IAM auth"
```

---

### Task 7: Migrate S3 to shared AWS config

**Files:**
- Modify: `internal/connector/s3.go`

- [ ] **Step 1: Replace `newS3Client` with `NewAWSConfig`**

Refactor `newS3Client` in `s3.go` to use the shared helper.

**Important:** Existing S3 credentials use field names `access_key` and `secret_key`, but the new `aws` credential type uses `access_key_id` and `secret_access_key`. The `NewAWSConfig` helper accepts the new names. To maintain backward compatibility with existing S3 credentials, normalize the field names before passing to `NewAWSConfig`:

```go
func newS3Client(ctx context.Context, params map[string]any, defaultRegion string) (*s3.Client, error) {
	cred, _ := params["_credential"].(map[string]string)
	delete(params, "_credential")

	// Normalize legacy S3 credential field names to standard AWS names.
	if cred != nil {
		if _, ok := cred["access_key_id"]; !ok {
			if v := cred["access_key"]; v != "" {
				cred["access_key_id"] = v
			}
		}
		if _, ok := cred["secret_access_key"]; !ok {
			if v := cred["secret_key"]; v != "" {
				cred["secret_access_key"] = v
			}
		}
	}

	awsCfg, err := NewAWSConfig(ctx, cred, defaultRegion)
	if err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}

	opts := []func(*s3.Options){}

	// Support custom endpoint for S3-compatible services (MinIO, etc.)
	if cred != nil {
		if endpoint := cred["endpoint"]; endpoint != "" {
			opts = append(opts, func(o *s3.Options) {
				o.BaseEndpoint = aws.String(endpoint)
				o.UsePathStyle = true
			})
		}
	}

	client := s3.NewFromConfig(awsCfg, opts...)
	return client, nil
}
```

Update all callers of `newS3Client` to pass `ctx` and `defaultRegion` (empty string is fine — the AWS config builder handles fallback).

- [ ] **Step 2: Run S3 tests**

Run: `go test ./internal/connector/ -run TestS3 -v`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add internal/connector/s3.go
git commit -m "refactor(s3): migrate to shared AWS config builder"
```

---

### Task 8: Wire default region into engine

**Files:**
- Modify: `internal/cli/serve.go`
- Modify: `internal/connector/connector.go`

- [ ] **Step 1: Pass AWS default region to AIConnector**

In `internal/connector/connector.go`, update `NewRegistry` to accept config or update `AIConnector` initialization. The simplest approach: `NewRegistry` stays as-is, and `serve.go` sets the default region on the connector after creation:

In `internal/cli/serve.go`, after creating the engine:
```go
// Set AWS default region for AI and S3 connectors.
if cfg.AWS.Region != "" {
	if aiConn, err := eng.Registry.Get("ai/completion"); err == nil {
		if ai, ok := aiConn.(*connector.AIConnector); ok {
			ai.DefaultRegion = cfg.AWS.Region
			ai.AWSConfigFunc = connector.NewAWSConfig
		}
	}
}
```

- [ ] **Step 2: Run full build and tests**

```bash
go build ./...
go test ./internal/connector/... ./internal/engine/... -v -count=1 -timeout 120s
```

- [ ] **Step 3: Commit**

```bash
git add internal/cli/serve.go internal/connector/connector.go
git commit -m "feat: wire AWS default region from config into AI and S3 connectors"
```

---

## Summary

| Task | Description | Deps |
|------|-------------|------|
| 1 | LLMProvider interface + common types | None |
| 2 | Cloud config + `aws` credential type | None |
| 3 | Shared AWS config builder | None |
| 4 | Extract OpenAI into LLMProvider + dispatcher | 1 |
| 5 | Update ToolLoop for provider-agnostic dispatch | 4 |
| 6 | Bedrock provider implementation | 1, 3 |
| 7 | Migrate S3 to shared AWS config | 3 |
| 8 | Wire default region into engine | 2, 4, 7 |

Tasks 1, 2, 3 are independent foundations. Task 4 depends on 1. Tasks 5-8 build on the foundation. The critical path is: 1 → 4 → 5 → 6 → 8.
