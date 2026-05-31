package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// blobRedirectTransport redirects all requests to a test server by replacing
// the scheme+host while preserving path and query.
type blobRedirectTransport struct {
	targetScheme string
	targetHost   string
}

func newBlobRedirectTransport(serverURL string) *blobRedirectTransport {
	u, _ := url.Parse(serverURL)
	return &blobRedirectTransport{
		targetScheme: u.Scheme,
		targetHost:   u.Host,
	}
}

func (rt *blobRedirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = rt.targetScheme
	cloned.URL.Host = rt.targetHost
	cloned.Host = rt.targetHost
	return http.DefaultTransport.RoundTrip(cloned)
}

func TestAzureBlobUploadConnector_UploadsBlob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.Header.Get("x-ms-blob-type") != "BlockBlob" {
			t.Errorf("expected x-ms-blob-type=BlockBlob, got %s", r.Header.Get("x-ms-blob-type"))
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := &AzureBlobUploadConnector{
		Client: &http.Client{Transport: newBlobRedirectTransport(srv.URL)},
	}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"account":   "storageacct",
			"container": "mycontainer",
			"sas_token": "sv=2020-01-01&sig=abc",
		},
		"blob_name": "hello.txt",
		"content":   "Hello Mantle!",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["ok"] != true {
		t.Errorf("expected ok=true, got %v", out["ok"])
	}
	if out["blob_name"] != "hello.txt" {
		t.Errorf("expected blob_name=hello.txt, got %v", out["blob_name"])
	}
}

func TestAzureBlobUploadConnector_MissingBlobName(t *testing.T) {
	c := &AzureBlobUploadConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"account":   "storageacct",
			"container": "mycontainer",
		},
		"content": "data",
	})
	if err == nil {
		t.Fatal("expected error for missing blob_name")
	}
}

func TestAzureBlobUploadConnector_MissingCredential(t *testing.T) {
	c := &AzureBlobUploadConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"blob_name": "hello.txt",
		"content":   "data",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestAzureBlobUploadConnector_MissingAccount(t *testing.T) {
	c := &AzureBlobUploadConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"container": "mycontainer",
		},
		"blob_name": "hello.txt",
		"content":   "data",
	})
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

func TestAzureBlobDownloadConnector_DownloadsBlob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("blob content here"))
	}))
	defer srv.Close()

	c := &AzureBlobDownloadConnector{
		Client: &http.Client{Transport: newBlobRedirectTransport(srv.URL)},
	}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"account":   "storageacct",
			"container": "mycontainer",
			"sas_token": "sv=2020-01-01&sig=abc",
		},
		"blob_name": "hello.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["content"] != "blob content here" {
		t.Errorf("expected content='blob content here', got %v", out["content"])
	}
	if out["content_type"] != "text/plain" {
		t.Errorf("expected content_type=text/plain, got %v", out["content_type"])
	}
}

func TestAzureBlobDownloadConnector_MissingCredential(t *testing.T) {
	c := &AzureBlobDownloadConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"blob_name": "hello.txt",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestAzureInvokeFunctionConnector_Invokes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Query().Get("code") != "funckey123" {
			t.Errorf("expected code=funckey123, got %s", r.URL.Query().Get("code"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "invoked"})
	}))
	defer srv.Close()

	c := &AzureInvokeFunctionConnector{}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"function_url": srv.URL + "/api/MyFunc",
			"function_key": "funckey123",
		},
		"body": `{"input":"data"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["status"] != "invoked" {
		t.Errorf("expected status=invoked, got %v", out["status"])
	}
}

func TestAzureInvokeFunctionConnector_MissingCredential(t *testing.T) {
	c := &AzureInvokeFunctionConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"body": `{}`,
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestAzureInvokeFunctionConnector_MissingFunctionURL(t *testing.T) {
	c := &AzureInvokeFunctionConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"function_key": "key",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing function_url")
	}
}

func TestRegistry_AzureConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"azure/blob_upload", "azure/blob_download", "azure/invoke_function"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
