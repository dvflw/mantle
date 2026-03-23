package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
)

func TestVerifyWebhookSignature_Valid(t *testing.T) {
	body := []byte(`{"action":"push","ref":"refs/heads/main"}`)
	secret := "my-webhook-secret"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	headers := http.Header{}
	headers.Set("X-Hub-Signature-256", sig)

	if !verifyWebhookSignature(body, secret, headers) {
		t.Error("valid signature was rejected")
	}
}

func TestVerifyWebhookSignature_Invalid(t *testing.T) {
	body := []byte(`{"action":"push"}`)
	secret := "my-webhook-secret"

	headers := http.Header{}
	headers.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString([]byte("wrong-signature-value-1234567890")))

	if verifyWebhookSignature(body, secret, headers) {
		t.Error("invalid signature was accepted")
	}
}

func TestVerifyWebhookSignature_MissingHeader(t *testing.T) {
	body := []byte(`{"action":"push"}`)
	secret := "my-webhook-secret"
	headers := http.Header{} // no signature header

	if verifyWebhookSignature(body, secret, headers) {
		t.Error("missing signature header should return false")
	}
}

func TestVerifyWebhookSignature_EmptySecret(t *testing.T) {
	// When the trigger has no secret, verifyWebhookSignature is not called
	// (the caller skips it). But if called with an empty secret, the HMAC
	// is computed with an empty key. The signature header must still be present
	// and match; an empty secret does not mean "skip verification."
	body := []byte(`{"action":"push"}`)

	// Compute HMAC with empty key.
	mac := hmac.New(sha256.New, []byte(""))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	headers := http.Header{}
	headers.Set("X-Hub-Signature-256", validSig)

	if !verifyWebhookSignature(body, "", headers) {
		t.Error("valid signature with empty secret was rejected")
	}

	// Without the header, it should still fail.
	emptyHeaders := http.Header{}
	if verifyWebhookSignature(body, "", emptyHeaders) {
		t.Error("missing header with empty secret should return false")
	}
}

func TestVerifyWebhookSignature_AlternativeHeader(t *testing.T) {
	body := []byte(`test-body`)
	secret := "alt-secret"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	headers := http.Header{}
	headers.Set("X-Signature-256", sig)

	if !verifyWebhookSignature(body, secret, headers) {
		t.Error("valid X-Signature-256 header was rejected")
	}
}
