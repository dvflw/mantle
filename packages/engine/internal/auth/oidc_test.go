package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestOIDCValidator_ValidToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}

	srv := mockOIDCServer(t, key)
	ctx := context.Background()

	validator, err := NewOIDCValidator(ctx, srv.URL, "test-client", "mantle", []string{"example.com"})
	if err != nil {
		t.Fatalf("creating validator: %v", err)
	}

	now := time.Now()
	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"sub":            "user-123",
		"aud":            "mantle",
		"exp":            now.Add(time.Hour).Unix(),
		"iat":            now.Unix(),
		"email":          "alice@example.com",
		"email_verified": true,
	})

	claims, err := validator.ValidateToken(ctx, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if claims.Subject != "user-123" {
		t.Errorf("subject = %q, want %q", claims.Subject, "user-123")
	}
	if claims.Email != "alice@example.com" {
		t.Errorf("email = %q, want %q", claims.Email, "alice@example.com")
	}
	if !claims.EmailVerified {
		t.Error("email_verified = false, want true")
	}
}

func TestOIDCValidator_UnverifiedEmail(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}

	srv := mockOIDCServer(t, key)
	ctx := context.Background()

	validator, err := NewOIDCValidator(ctx, srv.URL, "test-client", "mantle", nil)
	if err != nil {
		t.Fatalf("creating validator: %v", err)
	}

	now := time.Now()
	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"sub":            "user-456",
		"aud":            "mantle",
		"exp":            now.Add(time.Hour).Unix(),
		"iat":            now.Unix(),
		"email":          "bob@example.com",
		"email_verified": false,
	})

	_, err = validator.ValidateToken(ctx, token)
	if err == nil {
		t.Fatal("expected error for unverified email, got nil")
	}
}

func TestOIDCValidator_DisallowedDomain(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}

	srv := mockOIDCServer(t, key)
	ctx := context.Background()

	validator, err := NewOIDCValidator(ctx, srv.URL, "test-client", "mantle", []string{"corp.com"})
	if err != nil {
		t.Fatalf("creating validator: %v", err)
	}

	now := time.Now()
	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"sub":            "user-789",
		"aud":            "mantle",
		"exp":            now.Add(time.Hour).Unix(),
		"iat":            now.Unix(),
		"email":          "eve@evil.com",
		"email_verified": true,
	})

	_, err = validator.ValidateToken(ctx, token)
	if err == nil {
		t.Fatal("expected error for disallowed domain, got nil")
	}
}

func TestOIDCValidator_ExpiredToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}

	srv := mockOIDCServer(t, key)
	ctx := context.Background()

	validator, err := NewOIDCValidator(ctx, srv.URL, "test-client", "mantle", nil)
	if err != nil {
		t.Fatalf("creating validator: %v", err)
	}

	past := time.Now().Add(-2 * time.Hour)
	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"sub":            "user-expired",
		"aud":            "mantle",
		"exp":            past.Add(time.Hour).Unix(), // expired 1 hour ago
		"iat":            past.Unix(),
		"email":          "expired@example.com",
		"email_verified": true,
	})

	_, err = validator.ValidateToken(ctx, token)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestOIDCValidator_NoDomainRestriction(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}

	srv := mockOIDCServer(t, key)
	ctx := context.Background()

	// Empty allowed domains means all domains are accepted.
	validator, err := NewOIDCValidator(ctx, srv.URL, "test-client", "mantle", nil)
	if err != nil {
		t.Fatalf("creating validator: %v", err)
	}

	now := time.Now()
	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"sub":            "user-any",
		"aud":            "mantle",
		"exp":            now.Add(time.Hour).Unix(),
		"iat":            now.Unix(),
		"email":          "anyone@anydomain.org",
		"email_verified": true,
	})

	claims, err := validator.ValidateToken(ctx, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if claims.Email != "anyone@anydomain.org" {
		t.Errorf("email = %q, want %q", claims.Email, "anyone@anydomain.org")
	}
}
