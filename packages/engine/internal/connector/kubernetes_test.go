package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testK8sCredential() map[string]string {
	return map[string]string{
		"server":    "https://k8s.example.com",
		"token":     "test-token",
		"namespace": "default",
	}
}

// --- K8sApplyConnector ---

func TestK8sApplyConnector_ServerSideApply(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/apply-patch+yaml" {
			t.Errorf("unexpected Content-Type: %s", r.Header.Get("Content-Type"))
		}
		if r.URL.Query().Get("fieldManager") != "mantle" {
			t.Errorf("expected fieldManager=mantle, got %q", r.URL.Query().Get("fieldManager"))
		}
		if r.URL.Query().Get("force") != "true" {
			t.Errorf("expected force=true, got %q", r.URL.Query().Get("force"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := &K8sApplyConnector{Client: ts.Client(), baseURL: ts.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": testK8sCredential(),
		"manifest": map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "my-config", "namespace": "default"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["ok"] != true {
		t.Errorf("expected ok=true, got %v", out["ok"])
	}
}

func TestK8sApplyConnector_MissingManifest(t *testing.T) {
	c := &K8sApplyConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": testK8sCredential(),
	})
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestK8sApplyConnector_MissingMetadataName(t *testing.T) {
	c := &K8sApplyConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": testK8sCredential(),
		"manifest": map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{},
		},
	})
	if err == nil {
		t.Fatal("expected error for missing metadata.name")
	}
}

// --- K8sCreateJobConnector ---

func TestK8sCreateJobConnector_CreatesJob(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !containsStr(r.URL.Path, "/apis/batch/v1/namespaces/default/jobs") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		resp := map[string]any{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]any{
				"name":      "my-job",
				"namespace": "default",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	c := &K8sCreateJobConnector{
		Client:  ts.Client(),
		baseURL: ts.URL,
	}
	result, err := c.Execute(t.Context(), map[string]any{
		"name":        "my-job",
		"image":       "ubuntu:22.04",
		"command":     []string{"echo", "hello"},
		"_credential": testK8sCredential(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if kind, _ := result["kind"].(string); kind != "Job" {
		t.Errorf("expected kind=Job, got %q", kind)
	}
}

func TestK8sCreateJobConnector_MissingName(t *testing.T) {
	c := &K8sCreateJobConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"image":       "ubuntu:22.04",
		"command":     []string{"echo", "hello"},
		"_credential": testK8sCredential(),
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestK8sCreateJobConnector_MissingImage(t *testing.T) {
	c := &K8sCreateJobConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"name":        "my-job",
		"command":     []string{"echo", "hello"},
		"_credential": testK8sCredential(),
	})
	if err == nil {
		t.Fatal("expected error for missing image")
	}
}

func TestK8sCreateJobConnector_MissingCommand(t *testing.T) {
	c := &K8sCreateJobConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"name":        "my-job",
		"image":       "ubuntu:22.04",
		"_credential": testK8sCredential(),
	})
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestK8sCreateJobConnector_MissingCredential(t *testing.T) {
	c := &K8sCreateJobConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"name":    "my-job",
		"image":   "ubuntu:22.04",
		"command": []string{"echo", "hello"},
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

// --- K8sGetPodStatusConnector ---

func TestK8sGetPodStatusConnector_GetsPodStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !containsStr(r.URL.Path, "/api/v1/namespaces/default/pods/my-pod") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]any{
			"metadata": map[string]any{"name": "my-pod"},
			"status": map[string]any{
				"phase": "Running",
				"conditions": []any{
					map[string]any{"type": "Ready", "status": "True"},
				},
				"containerStatuses": []any{
					map[string]any{"name": "main", "ready": true},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	c := &K8sGetPodStatusConnector{
		Client:  ts.Client(),
		baseURL: ts.URL,
	}
	result, err := c.Execute(t.Context(), map[string]any{
		"pod_name":    "my-pod",
		"_credential": testK8sCredential(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if phase, _ := result["phase"].(string); phase != "Running" {
		t.Errorf("expected phase=Running, got %q", phase)
	}
}

func TestK8sGetPodStatusConnector_MissingPodName(t *testing.T) {
	c := &K8sGetPodStatusConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": testK8sCredential(),
	})
	if err == nil {
		t.Fatal("expected error for missing pod_name")
	}
}

// --- Registry ---

func TestRegistry_K8sConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{
		"k8s/apply",
		"k8s/create_job",
		"k8s/get_pod_status",
	} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}

func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}
