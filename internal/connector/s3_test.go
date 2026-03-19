package connector

import (
	"context"
	"testing"
)

// --- S3PutConnector validation tests ---

func TestS3PutConnector_MissingBucket(t *testing.T) {
	c := &S3PutConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"key":     "test.txt",
		"content": "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}
	if got := err.Error(); got != "s3/put: bucket is required" {
		t.Errorf("error = %q, want %q", got, "s3/put: bucket is required")
	}
}

func TestS3PutConnector_MissingKey(t *testing.T) {
	c := &S3PutConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"bucket":  "my-bucket",
		"content": "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if got := err.Error(); got != "s3/put: key is required" {
		t.Errorf("error = %q, want %q", got, "s3/put: key is required")
	}
}

func TestS3PutConnector_MissingContent(t *testing.T) {
	c := &S3PutConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"bucket": "my-bucket",
		"key":    "test.txt",
	})
	if err == nil {
		t.Fatal("expected error for missing content")
	}
	if got := err.Error(); got != "s3/put: content is required" {
		t.Errorf("error = %q, want %q", got, "s3/put: content is required")
	}
}

func TestS3PutConnector_MissingCredential(t *testing.T) {
	c := &S3PutConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"bucket":  "my-bucket",
		"key":     "test.txt",
		"content": "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestS3PutConnector_IncompleteCredential(t *testing.T) {
	c := &S3PutConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"bucket":  "my-bucket",
		"key":     "test.txt",
		"content": "hello",
		"_credential": map[string]string{
			"access_key": "AKID",
			// missing secret_key
		},
	})
	if err == nil {
		t.Fatal("expected error for incomplete credential")
	}
	if got := err.Error(); got != "s3: access_key and secret_key are required in credential" {
		t.Errorf("error = %q, want credential error", got)
	}
}

// --- S3GetConnector validation tests ---

func TestS3GetConnector_MissingBucket(t *testing.T) {
	c := &S3GetConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"key": "test.txt",
	})
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}
	if got := err.Error(); got != "s3/get: bucket is required" {
		t.Errorf("error = %q, want %q", got, "s3/get: bucket is required")
	}
}

func TestS3GetConnector_MissingKey(t *testing.T) {
	c := &S3GetConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"bucket": "my-bucket",
	})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if got := err.Error(); got != "s3/get: key is required" {
		t.Errorf("error = %q, want %q", got, "s3/get: key is required")
	}
}

func TestS3GetConnector_MissingCredential(t *testing.T) {
	c := &S3GetConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"bucket": "my-bucket",
		"key":    "test.txt",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

// --- S3ListConnector validation tests ---

func TestS3ListConnector_MissingBucket(t *testing.T) {
	c := &S3ListConnector{}
	_, err := c.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}
	if got := err.Error(); got != "s3/list: bucket is required" {
		t.Errorf("error = %q, want %q", got, "s3/list: bucket is required")
	}
}

func TestS3ListConnector_MissingCredential(t *testing.T) {
	c := &S3ListConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"bucket": "my-bucket",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

// --- newS3Client tests ---

func TestNewS3Client_ValidCredentials(t *testing.T) {
	params := map[string]any{
		"_credential": map[string]string{
			"access_key": "AKIAIOSFODNN7EXAMPLE",
			"secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"region":     "us-west-2",
		},
	}

	client, err := newS3Client(params)
	if err != nil {
		t.Fatalf("newS3Client() error: %v", err)
	}
	if client == nil {
		t.Fatal("newS3Client() returned nil client")
	}
	// Credential should be removed from params.
	if _, ok := params["_credential"]; ok {
		t.Error("_credential should be deleted from params after extraction")
	}
}

func TestNewS3Client_DefaultRegion(t *testing.T) {
	params := map[string]any{
		"_credential": map[string]string{
			"access_key": "AKID",
			"secret_key": "SECRET",
		},
	}

	client, err := newS3Client(params)
	if err != nil {
		t.Fatalf("newS3Client() error: %v", err)
	}
	if client == nil {
		t.Fatal("newS3Client() returned nil client")
	}
}

func TestNewS3Client_WithEndpoint(t *testing.T) {
	params := map[string]any{
		"_credential": map[string]string{
			"access_key": "minioadmin",
			"secret_key": "minioadmin",
			"endpoint":   "http://localhost:9000",
		},
	}

	client, err := newS3Client(params)
	if err != nil {
		t.Fatalf("newS3Client() error: %v", err)
	}
	if client == nil {
		t.Fatal("newS3Client() returned nil client")
	}
}

func TestNewS3Client_MissingCredentialMap(t *testing.T) {
	params := map[string]any{}
	_, err := newS3Client(params)
	if err == nil {
		t.Fatal("expected error for missing _credential")
	}
}

func TestNewS3Client_MissingAccessKey(t *testing.T) {
	params := map[string]any{
		"_credential": map[string]string{
			"secret_key": "SECRET",
		},
	}
	_, err := newS3Client(params)
	if err == nil {
		t.Fatal("expected error for missing access_key")
	}
}

func TestNewS3Client_MissingSecretKey(t *testing.T) {
	params := map[string]any{
		"_credential": map[string]string{
			"access_key": "AKID",
		},
	}
	_, err := newS3Client(params)
	if err == nil {
		t.Fatal("expected error for missing secret_key")
	}
}

// --- Registry tests ---

func TestRegistry_S3Connectors(t *testing.T) {
	r := NewRegistry()

	for _, action := range []string{"s3/put", "s3/get", "s3/list"} {
		_, err := r.Get(action)
		if err != nil {
			t.Errorf("Get(%q) error: %v", action, err)
		}
	}
}
