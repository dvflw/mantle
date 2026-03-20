package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/dvflw/mantle/internal/config"
	"github.com/spf13/cobra"
)

func newLoginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Mantle",
		Long:  "Authenticate using OIDC (auth code + PKCE or device flow) or cache an API key.",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiKey, _ := cmd.Flags().GetBool("api-key")
			device, _ := cmd.Flags().GetBool("device")

			if apiKey {
				return loginAPIKey(cmd)
			}

			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("failed to load configuration")
			}

			if cfg.Auth.OIDC.IssuerURL == "" {
				return fmt.Errorf("OIDC issuer URL not configured — set auth.oidc.issuer_url in mantle.yaml or MANTLE_AUTH_OIDC_ISSUER_URL")
			}
			if cfg.Auth.OIDC.ClientID == "" {
				return fmt.Errorf("OIDC client ID not configured — set auth.oidc.client_id in mantle.yaml or MANTLE_AUTH_OIDC_CLIENT_ID")
			}

			if device {
				return loginDeviceFlow(cmd, cfg)
			}
			return loginAuthCodePKCE(cmd, cfg)
		},
	}

	cmd.Flags().Bool("api-key", false, "authenticate by caching an API key")
	cmd.Flags().Bool("device", false, "use device authorization flow (for headless environments)")

	return cmd
}

func loginAPIKey(cmd *cobra.Command) error {
	fmt.Fprint(cmd.OutOrStdout(), "Enter API key: ")

	var key string
	if _, err := fmt.Fscanln(cmd.InOrStdin(), &key); err != nil {
		return fmt.Errorf("reading API key: %w", err)
	}

	key = strings.TrimSpace(key)
	if !strings.HasPrefix(key, "mk_") {
		return fmt.Errorf("invalid API key: must start with 'mk_' prefix")
	}

	cred := &Credentials{
		Type:   CredTypeAPIKey,
		APIKey: key,
	}

	path := DefaultCredentialsPath()
	if err := SaveCredentials(path, cred); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "API key saved to %s\n", path)
	return nil
}

func loginAuthCodePKCE(cmd *cobra.Command, cfg *config.Config) error {
	ctx := cmd.Context()

	provider, err := oidc.NewProvider(ctx, cfg.Auth.OIDC.IssuerURL)
	if err != nil {
		return fmt.Errorf("discovering OIDC provider: %w", err)
	}

	// Start local listener on a random port before building the oauth2 config.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("starting local callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	oauth2Cfg := &oauth2.Config{
		ClientID:     cfg.Auth.OIDC.ClientID,
		ClientSecret: cfg.Auth.OIDC.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  fmt.Sprintf("http://localhost:%d/callback", port),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile", "offline_access"},
	}

	// Generate PKCE verifier and state.
	verifier := oauth2.GenerateVerifier()
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		listener.Close()
		return fmt.Errorf("generating state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	authURL := oauth2Cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))

	// Channel to receive the authorization code or error.
	type callbackResult struct {
		code string
		err  error
	}
	resultCh := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			resultCh <- callbackResult{err: fmt.Errorf("state mismatch")}
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			desc := r.URL.Query().Get("error_description")
			resultCh <- callbackResult{err: fmt.Errorf("authorization error: %s — %s", errParam, desc)}
			http.Error(w, "Authorization failed", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			resultCh <- callbackResult{err: fmt.Errorf("no authorization code in callback")}
			http.Error(w, "Missing code", http.StatusBadRequest)
			return
		}
		resultCh <- callbackResult{code: code}
		fmt.Fprint(w, "<html><body><h1>Login successful!</h1><p>You may close this window.</p></body></html>")
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			resultCh <- callbackResult{err: fmt.Errorf("callback server error: %w", err)}
		}
	}()
	defer server.Shutdown(context.Background())

	fmt.Fprintf(cmd.OutOrStdout(), "Open this URL to authenticate:\n\n  %s\n\n", authURL)
	fmt.Fprintln(cmd.OutOrStdout(), "Waiting for callback...")

	openBrowser(authURL)

	// Wait for callback.
	result := <-resultCh
	if result.err != nil {
		return result.err
	}

	// Exchange code for token.
	token, err := oauth2Cfg.Exchange(ctx, result.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return fmt.Errorf("exchanging code for token: %w", err)
	}

	cred := &Credentials{
		Type:         CredTypeOIDC,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
	}

	path := DefaultCredentialsPath()
	if err := SaveCredentials(path, cred); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Login successful! Credentials saved to %s\n", path)
	return nil
}

func loginDeviceFlow(cmd *cobra.Command, cfg *config.Config) error {
	ctx := cmd.Context()

	provider, err := oidc.NewProvider(ctx, cfg.Auth.OIDC.IssuerURL)
	if err != nil {
		return fmt.Errorf("discovering OIDC provider: %w", err)
	}

	// Extract device authorization endpoint from provider claims.
	var claims struct {
		DeviceAuthURL string `json:"device_authorization_endpoint"`
	}
	if err := provider.Claims(&claims); err != nil {
		return fmt.Errorf("reading provider claims: %w", err)
	}
	if claims.DeviceAuthURL == "" {
		return fmt.Errorf("OIDC provider does not support the device authorization flow")
	}

	oauth2Cfg := &oauth2.Config{
		ClientID:     cfg.Auth.OIDC.ClientID,
		ClientSecret: cfg.Auth.OIDC.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:       provider.Endpoint().AuthURL,
			TokenURL:      provider.Endpoint().TokenURL,
			DeviceAuthURL: claims.DeviceAuthURL,
		},
		Scopes: []string{oidc.ScopeOpenID, "email", "profile", "offline_access"},
	}

	resp, err := oauth2Cfg.DeviceAuth(ctx)
	if err != nil {
		return fmt.Errorf("requesting device authorization: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "To authenticate, visit:\n\n  %s\n\n", resp.VerificationURI)
	fmt.Fprintf(cmd.OutOrStdout(), "And enter code: %s\n\n", resp.UserCode)
	if resp.VerificationURIComplete != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Or open directly:\n\n  %s\n\n", resp.VerificationURIComplete)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Waiting for authorization...")

	token, err := oauth2Cfg.DeviceAccessToken(ctx, resp)
	if err != nil {
		return fmt.Errorf("polling for device token: %w", err)
	}

	cred := &Credentials{
		Type:         CredTypeOIDC,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
	}

	path := DefaultCredentialsPath()
	if err := SaveCredentials(path, cred); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Login successful! Credentials saved to %s\n", path)
	return nil
}

func openBrowser(url string) {
	cmd := browserCommand(url)
	_ = cmd.Start()
}

func newLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove cached credentials",
		Long:  "Delete the credentials file at ~/.mantle/credentials.",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := DefaultCredentialsPath()
			if err := DeleteCredentials(path); err != nil {
				return fmt.Errorf("removing credentials: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Credentials removed from %s\n", path)
			return nil
		},
	}
}
