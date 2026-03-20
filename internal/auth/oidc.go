package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCClaims holds the validated claims extracted from an OIDC JWT.
type OIDCClaims struct {
	Subject       string
	Email         string
	EmailVerified bool
}

// OIDCValidator validates OIDC JWTs using provider discovery and JWKS.
type OIDCValidator struct {
	verifier       *oidc.IDTokenVerifier
	allowedDomains []string
}

// NewOIDCValidator creates a validator using OIDC discovery.
// JWKS is cached internally by go-oidc with automatic refresh on unknown kid.
func NewOIDCValidator(ctx context.Context, issuerURL, clientID, audience string, allowedDomains []string) (*OIDCValidator, error) {
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery for %s: %w", issuerURL, err)
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: audience,
	})

	return &OIDCValidator{
		verifier:       verifier,
		allowedDomains: allowedDomains,
	}, nil
}

// ValidateToken verifies the JWT signature, expiry, issuer, and audience,
// then extracts and validates the email claim.
func (v *OIDCValidator) ValidateToken(ctx context.Context, rawToken string) (*OIDCClaims, error) {
	idToken, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("extracting claims: %w", err)
	}

	if claims.Email == "" {
		return nil, fmt.Errorf("token missing email claim")
	}
	if !claims.EmailVerified {
		return nil, fmt.Errorf("email %q is not verified", claims.Email)
	}

	if err := v.checkDomain(claims.Email); err != nil {
		return nil, err
	}

	return &OIDCClaims{
		Subject:       idToken.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
	}, nil
}

func (v *OIDCValidator) checkDomain(email string) error {
	if len(v.allowedDomains) == 0 {
		return nil
	}

	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid email format: %q", email)
	}
	domain := strings.ToLower(parts[1])

	for _, allowed := range v.allowedDomains {
		if strings.ToLower(allowed) == domain {
			return nil
		}
	}
	return fmt.Errorf("email domain %q not in allowed domains", domain)
}
