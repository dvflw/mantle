package connector

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// extractK8sCredential pulls server, token, and namespace from the _credential param.
// Namespace defaults to "default" if empty.
// Deletes _credential from params.
func extractK8sCredential(params map[string]any) (server, token, namespace string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", "", "", fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	var cred map[string]string
	switch v := raw.(type) {
	case map[string]string:
		cred = v
	case map[string]any:
		cred = make(map[string]string, len(v))
		for k, val := range v {
			if s, ok := val.(string); ok {
				cred[k] = s
			}
		}
	default:
		return "", "", "", fmt.Errorf("credential is required")
	}

	server = cred["server"]
	if server == "" {
		return "", "", "", fmt.Errorf("credential must contain a 'server' field")
	}
	token = cred["token"]
	if token == "" {
		return "", "", "", fmt.Errorf("credential must contain a 'token' field")
	}
	namespace = cred["namespace"]
	if namespace == "" {
		namespace = "default"
	}
	return server, token, namespace, nil
}

// k8sInsecureClient returns an HTTP client that skips TLS verification.
// This is appropriate for Kubernetes clusters with self-signed certs.
func k8sInsecureClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
}

// k8sResourcePath builds the REST path for a Kubernetes resource.
// Core API (apiVersion="v1"): /api/v1/namespaces/{ns}/{resourceType}/{name}
// Other APIs: /apis/{apiVersion}/namespaces/{ns}/{resourceType}/{name}
func k8sResourcePath(apiVersion, kind, namespace, name string) string {
	resourceType := strings.ToLower(kind) + "s"

	var basePath string
	if apiVersion == "v1" {
		basePath = "/api/v1"
	} else {
		basePath = "/apis/" + apiVersion
	}

	if name != "" {
		return fmt.Sprintf("%s/namespaces/%s/%s/%s", basePath, namespace, resourceType, name)
	}
	return fmt.Sprintf("%s/namespaces/%s/%s", basePath, namespace, resourceType)
}

// k8sDoRequest executes an HTTP request against the Kubernetes API.
func k8sDoRequest(ctx context.Context, client *http.Client, method, url, token string, body []byte) (map[string]any, int, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}

	var result map[string]any
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, resp.StatusCode, fmt.Errorf("parsing response: %w", err)
		}
	}

	return result, resp.StatusCode, nil
}

// K8sApplyConnector applies a Kubernetes resource manifest using server-side apply
// (PATCH with application/apply-patch+yaml and fieldManager=mantle).
// Creates or updates without requiring resourceVersion in the manifest.
// Params: manifest (required — map[string]any Kubernetes resource manifest).
// Output: {"ok": true, "name": "...", "kind": "..."}
type K8sApplyConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *K8sApplyConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	manifest, ok := params["manifest"].(map[string]any)
	if !ok || manifest == nil {
		return nil, fmt.Errorf("k8s/apply: manifest is required")
	}

	server, token, credNamespace, err := extractK8sCredential(params)
	if err != nil {
		return nil, fmt.Errorf("k8s/apply: %w", err)
	}

	baseURL := server
	if c.baseURL != "" {
		baseURL = c.baseURL
	}

	apiVersion, _ := manifest["apiVersion"].(string)
	kind, _ := manifest["kind"].(string)
	if apiVersion == "" || kind == "" {
		return nil, fmt.Errorf("k8s/apply: manifest must contain apiVersion and kind")
	}

	namespace := credNamespace
	if meta, ok := manifest["metadata"].(map[string]any); ok {
		if ns, _ := meta["namespace"].(string); ns != "" {
			namespace = ns
		}
	}

	name := ""
	if meta, ok := manifest["metadata"].(map[string]any); ok {
		name, _ = meta["name"].(string)
	}
	if name == "" {
		return nil, fmt.Errorf("k8s/apply: manifest.metadata.name is required")
	}

	body, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("k8s/apply: marshalling manifest: %w", err)
	}

	client := k8sInsecureClient(c.Client)
	path := k8sResourcePath(apiVersion, kind, namespace, name)
	applyURL := baseURL + path + "?fieldManager=mantle&force=true"

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, applyURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("k8s/apply: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	// Server-side apply content type: creates or updates without requiring resourceVersion.
	req.Header.Set("Content-Type", "application/apply-patch+yaml")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("k8s/apply: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("k8s/apply: server-side apply returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	return map[string]any{
		"ok":   true,
		"name": name,
		"kind": kind,
	}, nil
}

// K8sCreateJobConnector creates a Kubernetes batch/v1 Job.
// Params: name (required), image (required), command (required []string),
//
//	namespace (optional), restart_policy (optional, default "Never"),
//	backoff_limit (optional int, default 0).
//
// Output: parsed job object from Kubernetes API.
type K8sCreateJobConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *K8sCreateJobConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	name, _ := params["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("k8s/create_job: name is required")
	}

	image, _ := params["image"].(string)
	if image == "" {
		return nil, fmt.Errorf("k8s/create_job: image is required")
	}

	var command []string
	switch v := params["command"].(type) {
	case []string:
		command = v
	case []any:
		for _, s := range v {
			if str, ok := s.(string); ok {
				command = append(command, str)
			}
		}
	default:
		return nil, fmt.Errorf("k8s/create_job: command is required")
	}
	if len(command) == 0 {
		return nil, fmt.Errorf("k8s/create_job: command is required")
	}

	server, token, credNamespace, err := extractK8sCredential(params)
	if err != nil {
		return nil, fmt.Errorf("k8s/create_job: %w", err)
	}

	baseURL := server
	if c.baseURL != "" {
		baseURL = c.baseURL
	}

	namespace := credNamespace
	if ns, _ := params["namespace"].(string); ns != "" {
		namespace = ns
	}

	restartPolicy := "Never"
	if rp, _ := params["restart_policy"].(string); rp != "" {
		restartPolicy = rp
	}

	backoffLimit := 0
	if bl, ok := extractInt(params["backoff_limit"]); ok {
		backoffLimit = bl
	}

	jobManifest := map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"backoffLimit": backoffLimit,
			"template": map[string]any{
				"spec": map[string]any{
					"restartPolicy": restartPolicy,
					"containers": []any{
						map[string]any{
							"name":    name,
							"image":   image,
							"command": command,
						},
					},
				},
			},
		},
	}

	body, err := json.Marshal(jobManifest)
	if err != nil {
		return nil, fmt.Errorf("k8s/create_job: marshalling manifest: %w", err)
	}

	client := k8sInsecureClient(c.Client)
	url := fmt.Sprintf("%s/apis/batch/v1/namespaces/%s/jobs", baseURL, namespace)

	result, statusCode, err := k8sDoRequest(ctx, client, http.MethodPost, url, token, body)
	if err != nil {
		return nil, fmt.Errorf("k8s/create_job: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("k8s/create_job: API returned status %d", statusCode)
	}

	return result, nil
}

// K8sGetPodStatusConnector fetches the status of a Kubernetes Pod.
// Params: pod_name (required), namespace (optional, fallback to credential namespace).
// Output: {"name": ..., "phase": ..., "conditions": [...], "container_statuses": [...]}
type K8sGetPodStatusConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *K8sGetPodStatusConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	podName, _ := params["pod_name"].(string)
	if podName == "" {
		return nil, fmt.Errorf("k8s/get_pod_status: pod_name is required")
	}

	server, token, credNamespace, err := extractK8sCredential(params)
	if err != nil {
		return nil, fmt.Errorf("k8s/get_pod_status: %w", err)
	}

	baseURL := server
	if c.baseURL != "" {
		baseURL = c.baseURL
	}

	namespace := credNamespace
	if ns, _ := params["namespace"].(string); ns != "" {
		namespace = ns
	}

	client := k8sInsecureClient(c.Client)
	url := fmt.Sprintf("%s/api/v1/namespaces/%s/pods/%s", baseURL, namespace, podName)

	result, statusCode, err := k8sDoRequest(ctx, client, http.MethodGet, url, token, nil)
	if err != nil {
		return nil, fmt.Errorf("k8s/get_pod_status: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("k8s/get_pod_status: API returned status %d", statusCode)
	}

	// Extract relevant status fields.
	out := map[string]any{
		"name": podName,
	}

	if meta, ok := result["metadata"].(map[string]any); ok {
		if n, _ := meta["name"].(string); n != "" {
			out["name"] = n
		}
	}

	if status, ok := result["status"].(map[string]any); ok {
		out["phase"] = status["phase"]
		out["conditions"] = status["conditions"]
		out["container_statuses"] = status["containerStatuses"]
	}

	return out, nil
}
