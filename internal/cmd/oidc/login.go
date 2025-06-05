package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"go.funccloud.dev/fcp/internal/config"
	"golang.org/x/oauth2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	clientauthv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/yaml"
)

var (
	oidcLoginLong = templates.LongDesc(i18n.T(`
		Perform OIDC login for Kubernetes authentication.

		This command initiates an OpenID Connect (OIDC) authentication flow
		and saves the resulting session for use with Kubernetes clusters.
		The session, including email and group claims from the ID token,
		is saved to ~/.fcp/session.yaml.
		The command then outputs a Kubernetes ExecCredential JSON to stdout.

		The command starts a local callback server, opens your default browser
		to the OIDC provider's authorization page, and waits for the callback
		to complete the authentication flow.`))

	oidcLoginExample = templates.Examples(i18n.T(`
		# Login using OIDC with default settings
		fcp oidc login

		# Login with a specific issuer URL
		fcp oidc login --issuer-url https://accounts.google.com

		# Login with custom client ID
		fcp oidc login --client-id my-client-id

		# Login with custom redirect URI and port
		fcp oidc login --redirect-uri http://localhost:9090/callback --port 9090

		# Login with custom scopes
		fcp oidc login --scopes openid,profile,email,groups`))
)

// OIDCLoginOptions contains the options for OIDC login
type OIDCLoginOptions struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
	Port         int
	Timeout      time.Duration

	genericiooptions.IOStreams
}

// OIDCSession represents the saved OIDC session
type OIDCSession struct {
	AccessToken   string    `json:"access_token" yaml:"access_token"`
	RefreshToken  string    `json:"refresh_token,omitempty" yaml:"refresh_token,omitempty"`
	IDToken       string    `json:"id_token" yaml:"id_token"`
	TokenType     string    `json:"token_type" yaml:"token_type"`
	ExpiresAt     time.Time `json:"expires_at" yaml:"expires_at"` // Access Token expiry
	IssuerURL     string    `json:"issuer_url" yaml:"issuer_url"`
	ClientID      string    `json:"client_id" yaml:"client_id"`
	ClientSecret  string    `json:"client_secret,omitempty" yaml:"client_secret,omitempty"`
	CreatedAt     time.Time `json:"created_at" yaml:"created_at"`
	Email         string    `json:"email,omitempty" yaml:"email,omitempty"`
	Groups        []string  `json:"groups,omitempty" yaml:"groups,omitempty"`
	IDTokenExpiry time.Time `json:"id_token_expiry,omitempty" yaml:"id_token_expiry,omitempty"` // ID Token expiry
}

// NewCmdOIDCLogin creates the oidc login command
func NewCmdOIDCLogin(f cmdutil.Factory, ioStreams genericiooptions.IOStreams) *cobra.Command {
	o := &OIDCLoginOptions{
		IssuerURL:   "https://accounts.google.com",
		ClientID:    "123456789012-abc123def456.apps.googleusercontent.com",
		RedirectURI: "http://localhost:8080/callback",
		Scopes:      []string{"openid", "profile", "email", "groups"},
		Port:        8080,
		Timeout:     5 * time.Minute,
		IOStreams:   ioStreams,
	}

	cmd := &cobra.Command{
		Use:                   "login",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Perform OIDC login"),
		Long:                  oidcLoginLong,
		Example:               oidcLoginExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Complete(f, cmd, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			return o.Run(cmd.Context())
		},
	}

	cmd.Flags().StringVar(&o.IssuerURL, "issuer-url", o.IssuerURL, "The OIDC issuer URL")
	cmd.Flags().StringVar(&o.ClientID, "client-id", o.ClientID, "The OIDC client ID")
	cmd.Flags().StringVar(&o.ClientSecret, "client-secret", "", "The OIDC client secret (if required)")
	cmd.Flags().StringVar(&o.RedirectURI, "redirect-uri", o.RedirectURI, "The redirect URI for OIDC callback")
	cmd.Flags().StringSliceVar(&o.Scopes, "scopes", o.Scopes, "The OIDC scopes to request")
	cmd.Flags().IntVar(&o.Port, "port", o.Port, "The local port to use for the callback server")
	cmd.Flags().DurationVar(&o.Timeout, "timeout", o.Timeout, "Timeout for the authentication flow")

	return cmd
}

// Complete fills in any details not provided in flags
func (o *OIDCLoginOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	// Update redirect URI to match the port if it was changed
	if cmd.Flags().Changed("port") && !cmd.Flags().Changed("redirect-uri") {
		o.RedirectURI = fmt.Sprintf("http://localhost:%d/callback", o.Port)
	}
	return nil
}

// Validate ensures the options are valid
func (o *OIDCLoginOptions) Validate() error {
	if o.IssuerURL == "" {
		return fmt.Errorf("issuer-url is required")
	}
	if o.ClientID == "" {
		return fmt.Errorf("client-id is required")
	}
	if o.ClientSecret == "" {
		return fmt.Errorf("client-secret is required")
	}
	if o.RedirectURI == "" {
		return fmt.Errorf("redirect-uri is required")
	}
	if o.Port <= 0 || o.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if len(o.Scopes) == 0 {
		return fmt.Errorf("at least one scope is required")
	}
	return nil
}

// loadAndValidateSession attempts to load an existing session.
// Returns the session if it's valid and not expired.
// Returns nil if no session exists, it's expired, or invalid.
// Returns an error only for issues like file read/parse problems.
func (o *OIDCLoginOptions) loadAndValidateSession(ctx context.Context) (*OIDCSession, error) {
	configDir := config.GetConfigDir()
	sessionPath := filepath.Join(configDir, "session.yaml")

	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		return nil, nil // No session file is not an error, just no session.
	}

	data, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file %s: %w", sessionPath, err)
	}

	var session OIDCSession
	if err := yaml.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session file %s: %w", sessionPath, err)
	}

	if session.IDToken == "" || session.IssuerURL == "" || session.ClientID == "" {
		return nil, fmt.Errorf("session file %s is incomplete (missing ID token, issuer, or client ID)", sessionPath)
	}

	provider, err := oidc.NewProvider(ctx, session.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider for session (issuer: %s): %w", session.IssuerURL, err)
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: session.ClientID})
	verifiedIDToken, err := verifier.Verify(ctx, session.IDToken)
	if err != nil {
		return nil, fmt.Errorf("existing session ID token is invalid or expired: %w", err)
	}

	if time.Now().After(verifiedIDToken.Expiry) {
		return nil, fmt.Errorf(
			"existing session ID token has expired (checked at %v, expires at %v)",
			time.Now(), verifiedIDToken.Expiry,
		)
	}

	session.IDTokenExpiry = verifiedIDToken.Expiry
	claims := struct {
		Email  string   `json:"email"`
		Groups []string `json:"groups"`
	}{}
	if err := verifiedIDToken.Claims(&claims); err == nil {
		session.Email = claims.Email
		session.Groups = claims.Groups
	} // Not fatal if claims can't be extracted, but ID token itself is valid.
	return &session, nil
}

// handleExistingSession attempts to use an existing valid session and outputs ExecCredential if successful.
// Returns true if an existing session was used, false otherwise.
func (o *OIDCLoginOptions) handleExistingSession(_ context.Context, existingSession *OIDCSession) (bool, error) {
	if existingSession != nil {
		ec := clientauthv1.ExecCredential{
			TypeMeta: metav1.TypeMeta{
				APIVersion: clientauthv1.SchemeGroupVersion.String(),
				Kind:       "ExecCredential",
			},
			Status: &clientauthv1.ExecCredentialStatus{
				Token:               existingSession.IDToken,
				ExpirationTimestamp: &metav1.Time{Time: existingSession.IDTokenExpiry},
			},
		}
		ecBytes, marshalErr := yaml.YAMLToJSON([]byte(mustYamlMarshal(ec)))
		if marshalErr != nil {
			_, _ = fmt.Fprintf(o.IOStreams.ErrOut, "Failed to marshal exec credential for existing session: %v\n", marshalErr)
			return false, fmt.Errorf("failed to marshal exec credential for existing session: %w", marshalErr)
		}
		_, _ = fmt.Fprintln(o.IOStreams.Out, string(ecBytes))
		_, _ = fmt.Fprintf(
			o.IOStreams.ErrOut,
			"Using existing valid OIDC session. Email: %s. Groups: %v. Expires: %s.\n",
			existingSession.Email,
			existingSession.Groups,
			existingSession.IDTokenExpiry.Format(time.RFC3339),
		)
		return true, nil
	}
	msg := "No valid existing session found or session expired. " +
		"Proceeding with new OIDC login.\n"
	_, _ = o.IOStreams.ErrOut.Write([]byte(msg))
	return false, nil
}

// performNewOIDCLogin handles the full OIDC authentication flow.
func (o *OIDCLoginOptions) performNewOIDCLogin(ctx context.Context) (*OIDCSession, error) {
	provider, err := oidc.NewProvider(ctx, o.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	oauth2Config := oauth2.Config{
		ClientID:     o.ClientID,
		ClientSecret: o.ClientSecret,
		RedirectURL:  o.RedirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       o.Scopes,
	}

	state, err := generateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	tokenChan := make(chan *oauth2.Token, 1)

	server := &http.Server{
		Addr: fmt.Sprintf(":%d", o.Port),
	}

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if err := o.handleOAuth2Callback(ctx, w, r, &oauth2Config, state, tokenChan); err != nil {
			return
		}
	})

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errMsg := fmt.Sprintf("failed to start callback server: %v", err)
			_, _ = o.IOStreams.ErrOut.Write([]byte(errMsg))
		}
	}()

	time.Sleep(100 * time.Millisecond)
	authURL := oauth2Config.AuthCodeURL(state, oauth2.AccessTypeOffline)

	if err := browser.OpenURL(authURL); err != nil {
		_, _ = fmt.Fprintf(o.IOStreams.ErrOut, "Failed to open browser automatically: %v\n", err)
	}

	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	var (
		token  *oauth2.Token
		email  string
		groups []string
	)
	select {
	case token = <-tokenChan:
		// Success: extract ID token, email, groups
		idToken := ""
		if idTokenRaw, ok := token.Extra("id_token").(string); ok {
			idToken = idTokenRaw
		}
		// Parse ID token for claims
		verifier := provider.Verifier(&oidc.Config{ClientID: o.ClientID})
		idTok, err := verifier.Verify(ctx, idToken)
		if err == nil {
			claims := struct {
				Email  string   `json:"email"`
				Groups []string `json:"groups"`
			}{}
			_ = idTok.Claims(&claims)
			email = claims.Email
			groups = claims.Groups
		}
		// Output ExecCredential with authenticated true
		ec := clientauthv1.ExecCredential{
			TypeMeta: metav1.TypeMeta{
				APIVersion: clientauthv1.SchemeGroupVersion.String(),
				Kind:       "ExecCredential",
			},
			Status: &clientauthv1.ExecCredentialStatus{
				Token:               token.Extra("id_token").(string),
				ExpirationTimestamp: &metav1.Time{Time: token.Expiry},
			},
		}
		ecBytes, err := yaml.YAMLToJSON([]byte(mustYamlMarshal(ec)))
		if err != nil {
			_, _ = fmt.Fprintf(o.IOStreams.ErrOut, "Failed to marshal exec credential: %v\n", err)
		} else {
			_, _ = fmt.Fprintln(o.IOStreams.Out, string(ecBytes))
		}
		_, _ = fmt.Fprintf(o.IOStreams.ErrOut, "Authenticated: true, Username (email): %s, Groups: %v\n", email, groups)
		if err := server.Shutdown(context.Background()); err != nil {
			_, _ = fmt.Fprintf(o.IOStreams.ErrOut, "Failed to shutdown server: %v\n", err)
		}
		return &OIDCSession{
			AccessToken:   token.AccessToken,
			RefreshToken:  token.RefreshToken,
			IDToken:       idToken,
			TokenType:     token.TokenType,
			ExpiresAt:     token.Expiry,
			IssuerURL:     o.IssuerURL,
			ClientID:      o.ClientID,
			CreatedAt:     time.Now(),
			Email:         email,
			Groups:        groups,
			IDTokenExpiry: idTok.Expiry,
		}, nil
	case <-ctx.Done():
		if shutdownErr := server.Shutdown(context.Background()); shutdownErr != nil {
			_, _ = fmt.Fprintf(o.IOStreams.ErrOut, "Failed to shutdown server: %v\n", shutdownErr)
		}
		return nil, fmt.Errorf("authentication timed out after %v", o.Timeout)
	}
}

// Run executes the OIDC login flow
func (o *OIDCLoginOptions) Run(ctx context.Context) error {
	execCredUnauth := func() {
		ec := clientauthv1.ExecCredential{
			TypeMeta: metav1.TypeMeta{
				APIVersion: clientauthv1.SchemeGroupVersion.String(),
				Kind:       "ExecCredential",
			},
			Status: &clientauthv1.ExecCredentialStatus{
				Token:               "",
				ExpirationTimestamp: nil,
			},
		}
		ecBytes, _ := json.Marshal(ec)
		_, _ = fmt.Fprintln(o.IOStreams.Out, string(ecBytes))
	}

	// Attempt to load and use existing valid session
	existingSession, loadErr := o.loadAndValidateSession(ctx)
	if loadErr != nil {
		errMsg := fmt.Sprintf("Error loading existing session: %v. Proceeding with new login.\n", loadErr)
		_, _ = o.IOStreams.ErrOut.Write([]byte(errMsg))
		// Always output unauthenticated ExecCredential on error
		execCredUnauth()
		return nil
	}

	usedExisting, err := o.handleExistingSession(ctx, existingSession)
	if err != nil {
		errMsg := fmt.Sprintf("%v\n", err)
		_, _ = o.IOStreams.ErrOut.Write([]byte(errMsg))
		execCredUnauth()
		return err // Error during marshalling or outputting existing session
	}
	if usedExisting {
		return nil // Successfully used existing session
	}

	// Perform new OIDC login (this outputs ExecCredential directly)
	newSession, err := o.performNewOIDCLogin(ctx)
	if err != nil {
		execCredUnauth()
		return nil
	}

	// Save the new session (no further ExecCredential output)
	if err := o.saveSession(newSession); err != nil {
		_, _ = fmt.Fprintf(o.IOStreams.ErrOut, "failed to save new session: %v\n", err)
		// Still output authenticated ExecCredential for the session just obtained
		return nil
	}

	return nil
}

// handleOAuth2Callback processes the OAuth2 callback
func (o *OIDCLoginOptions) handleOAuth2Callback(ctx context.Context, w http.ResponseWriter, r *http.Request,
	oauth2Config *oauth2.Config, expectedState string, tokenChan chan *oauth2.Token) error {

	// Check for error parameter first
	if errorParam := r.URL.Query().Get("error"); errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		http.Error(w, "Authentication failed", http.StatusBadRequest)
		return fmt.Errorf("authentication failed: %s - %s", errorParam, errorDesc)
	}

	// Verify state
	state := r.URL.Query().Get("state")
	if state != expectedState {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return fmt.Errorf("invalid state parameter")
	}

	// Get authorization code
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return fmt.Errorf("missing authorization code")
	}

	// Exchange code for tokens
	token, err := oauth2Config.Exchange(ctx, code)
	if err != nil {
		http.Error(w, "Failed to exchange code for tokens", http.StatusInternalServerError)
		return fmt.Errorf("failed to exchange code for tokens: %w", err)
	}

	// Send success response
	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprint(w, `
		<!DOCTYPE html>
		<html>
		<head>
			<title>FCP Authentication Complete</title>
			<style>
				body { 
					font-family: Arial, sans-serif; 
					text-align: center; 
					padding: 50px; 
					background-color: #f5f5f5; 
				}
				.container { 
					background-color: white; 
					padding: 30px; 
					border-radius: 10px; 
					box-shadow: 0 2px 10px rgba(0,0,0,0.1); 
					max-width: 500px; 
					margin: 0 auto; 
				}
				.success { color: #28a745; font-size: 24px; margin-bottom: 20px; }
				.details { color: #666; font-size: 14px; }
			</style>
		</head>
		<body>
			<div class="container">
				<div class="success">✓ Authentication Successful!</div>
				<p>Your OIDC authentication has been completed successfully.</p>
				<p class="details">You can close this window and return to your terminal.</p>
			</div>
		</body>
		</html>
	`)

	tokenChan <- token
	return nil
}

// saveSession saves the OIDC session to file
func (o *OIDCLoginOptions) saveSession(session *OIDCSession) error {
	configDir := config.GetConfigDir()

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	sessionPath := filepath.Join(configDir, "session.yaml")

	// Marshal to YAML
	data, err := yaml.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Write to file with restricted permissions
	if err := os.WriteFile(sessionPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// generateRandomString generates a random string for state parameter
func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(bytes)
	if len(encoded) > length {
		return encoded[:length], nil
	}
	return encoded, nil
}

// Helper for YAML marshaling
func mustYamlMarshal(obj interface{}) string {
	b, err := yaml.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return string(b)
}
