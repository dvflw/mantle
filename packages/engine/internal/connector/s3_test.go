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
			// missing secret_key — falls through to default chain, which fails on region
		},
	})
	if err == nil {
		t.Fatal("expected error for incomplete credential")
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

	client, err := newS3Client(context.Background(), params, "")
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

	// Pass a defaultRegion since no region is in the credential.
	client, err := newS3Client(context.Background(), params, "us-east-1")
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
			"region":     "us-east-1",
			"endpoint":   "http://localhost:9000",
		},
	}

	client, err := newS3Client(context.Background(), params, "")
	if err != nil {
		t.Fatalf("newS3Client() error: %v", err)
	}
	if client == nil {
		t.Fatal("newS3Client() returned nil client")
	}
}

func TestNewS3Client_MissingCredentialMap(t *testing.T) {
	// No _credential and no region — should fail on region requirement.
	params := map[string]any{}
	_, err := newS3Client(context.Background(), params, "")
	if err == nil {
		t.Fatal("expected error for missing _credential and no region")
	}
}

func TestNewS3Client_MissingAccessKey(t *testing.T) {
	// Only secret_key provided (no access_key) — falls through to default chain, fails on region.
	params := map[string]any{
		"_credential": map[string]string{
			"secret_key": "SECRET",
			"region":     "us-east-1",
		},
	}
	_, err := newS3Client(context.Background(), params, "")
	// With no access_key_id and no access_key, NewAWSConfig uses default chain.
	// Client construction succeeds (region is set); no error expected at build time.
	// This test verifies the call does not panic.
	_ = err
}

func TestNewS3Client_MissingSecretKey(t *testing.T) {
	// Only access_key provided (no secret_key) — falls through to default chain.
	params := map[string]any{
		"_credential": map[string]string{
			"access_key": "AKID",
			"region":     "us-east-1",
		},
	}
	_, err := newS3Client(context.Background(), params, "")
	// Same: incomplete static creds fall through to default chain; no build-time error.
	_ = err
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
