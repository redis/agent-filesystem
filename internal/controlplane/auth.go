package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	clerk "github.com/clerk/clerk-sdk-go/v2"
	clerkjwks "github.com/clerk/clerk-sdk-go/v2/jwks"
	clerkjwt "github.com/clerk/clerk-sdk-go/v2/jwt"
	clerkuser "github.com/clerk/clerk-sdk-go/v2/user"
)

type AuthMode string

const (
	AuthModeNone          AuthMode = "none"
	AuthModeTrustedHeader AuthMode = "trusted-header"
	AuthModeClerk         AuthMode = "clerk"
)

const (
	authModeEnvVar                      = "AFS_AUTH_MODE"
	authTrustedUserHeaderEnvVar         = "AFS_AUTH_TRUSTED_USER_HEADER"
	authTrustedNameHeaderEnvVar         = "AFS_AUTH_TRUSTED_NAME_HEADER"
	authTrustedEmailHeaderEnvVar        = "AFS_AUTH_TRUSTED_EMAIL_HEADER"
	authTrustedGroupsHeaderEnvVar       = "AFS_AUTH_TRUSTED_GROUPS_HEADER"
	authClerkSecretKeyEnvVar            = "CLERK_SECRET_KEY"
	authClerkPublishableKeyEnvVar       = "NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY"
	authClerkPublishableKeyLegacyEnvVar = "CLERK_PUBLISHABLE_KEY"
	authClerkJWTKeyEnvVar               = "CLERK_JWT_KEY"
	authClerkProxyURLEnvVar             = "CLERK_PROXY_URL"
	clerkSessionCookieName              = "__session"
	clerkJWKCacheTTL                    = time.Hour
)

var ErrUnauthorized = fmt.Errorf("authentication required")

type AuthConfig struct {
	Mode                AuthMode
	TrustedUserHeader   string
	TrustedNameHeader   string
	TrustedEmailHeader  string
	TrustedGroupsHeader string
	ClerkSecretKey      string
	ClerkPublishableKey string
	ClerkJWTKey         string
	ClerkProxyURL       string
}

type AuthIdentity struct {
	Subject           string
	Name              string
	Email             string
	Groups            []string
	Provider          string
	TokenID           string
	// Scope carries the MCP-token scope on token-authenticated requests:
	//   "workspace:<workspace-id>" for workspace-bound tokens,
	//   "control-plane"            for user-scoped control-plane tokens,
	//   ""                         for non-MCP auth paths.
	Scope             string
	ScopedDatabaseID  string
	ScopedWorkspaceID string
	ScopedWorkspace   string
	MCPProfile        string
	Readonly          bool
}

type authRuntimeConfigResponse struct {
	Mode                string           `json:"mode"`
	Enabled             bool             `json:"enabled"`
	Provider            string           `json:"provider"`
	SignInRequired      bool             `json:"sign_in_required"`
	Authenticated       bool             `json:"authenticated"`
	ProductMode         string           `json:"product_mode"`
	ClerkPublishableKey string           `json:"clerk_publishable_key,omitempty"`
	User                *authRuntimeUser `json:"user,omitempty"`
}

type authRuntimeUser struct {
	Subject string   `json:"subject"`
	Name    string   `json:"name,omitempty"`
	Email   string   `json:"email,omitempty"`
	Groups  []string `json:"groups,omitempty"`
}

type authContextKey string

const authIdentityContextKey authContextKey = "afs-auth-identity"

type cachedClerkJWK struct {
	key       *clerk.JSONWebKey
	expiresAt time.Time
}

type AuthHandler struct {
	cfg               AuthConfig
	clerkJWKSClient   *clerkjwks.Client
	clerkStaticJWTKey *clerk.JSONWebKey
	clerkJWKCacheMu   sync.Mutex
	clerkJWKCache     map[string]cachedClerkJWK
	clerkAuthenticate func(*http.Request) (*AuthIdentity, error)
	clerkDeleteUser   func(context.Context, string) error
	cliAuthenticate   func(context.Context, string) (*AuthIdentity, error)
	mcpAuthenticate   func(context.Context, string) (*AuthIdentity, error)
}

func DefaultAuthConfig() AuthConfig {
	return AuthConfig{
		Mode:                AuthModeNone,
		TrustedUserHeader:   "X-Forwarded-User",
		TrustedNameHeader:   "X-Forwarded-Name",
		TrustedEmailHeader:  "X-Forwarded-Email",
		TrustedGroupsHeader: "X-Forwarded-Groups",
	}
}

func LoadAuthConfigFromEnv() (AuthConfig, error) {
	cfg := DefaultAuthConfig()
	rawMode := strings.TrimSpace(os.Getenv(authModeEnvVar))
	if rawMode == "" {
		return cfg, nil
	}

	switch AuthMode(strings.ToLower(rawMode)) {
	case AuthModeNone:
		cfg.Mode = AuthModeNone
	case AuthModeTrustedHeader:
		cfg.Mode = AuthModeTrustedHeader
	case AuthModeClerk:
		cfg.Mode = AuthModeClerk
	default:
		return AuthConfig{}, fmt.Errorf("unsupported auth mode %q", rawMode)
	}

	if value := strings.TrimSpace(os.Getenv(authTrustedUserHeaderEnvVar)); value != "" {
		cfg.TrustedUserHeader = value
	}
	if value := strings.TrimSpace(os.Getenv(authTrustedNameHeaderEnvVar)); value != "" {
		cfg.TrustedNameHeader = value
	}
	if value := strings.TrimSpace(os.Getenv(authTrustedEmailHeaderEnvVar)); value != "" {
		cfg.TrustedEmailHeader = value
	}
	if value := strings.TrimSpace(os.Getenv(authTrustedGroupsHeaderEnvVar)); value != "" {
		cfg.TrustedGroupsHeader = value
	}
	cfg.ClerkSecretKey = strings.TrimSpace(os.Getenv(authClerkSecretKeyEnvVar))
	cfg.ClerkPublishableKey = strings.TrimSpace(os.Getenv(authClerkPublishableKeyEnvVar))
	if cfg.ClerkPublishableKey == "" {
		cfg.ClerkPublishableKey = strings.TrimSpace(os.Getenv(authClerkPublishableKeyLegacyEnvVar))
	}
	cfg.ClerkJWTKey = strings.TrimSpace(os.Getenv(authClerkJWTKeyEnvVar))
	cfg.ClerkProxyURL = strings.TrimSpace(os.Getenv(authClerkProxyURLEnvVar))

	if cfg.Mode == AuthModeTrustedHeader && strings.TrimSpace(cfg.TrustedUserHeader) == "" {
		return AuthConfig{}, fmt.Errorf("%s must not be empty when %s=%s", authTrustedUserHeaderEnvVar, authModeEnvVar, AuthModeTrustedHeader)
	}
	if cfg.Mode == AuthModeClerk {
		if cfg.ClerkPublishableKey == "" {
			return AuthConfig{}, fmt.Errorf("%s must not be empty when %s=%s", authClerkPublishableKeyEnvVar, authModeEnvVar, AuthModeClerk)
		}
		if cfg.ClerkSecretKey == "" && cfg.ClerkJWTKey == "" {
			return AuthConfig{}, fmt.Errorf("%s or %s must be set when %s=%s", authClerkSecretKeyEnvVar, authClerkJWTKeyEnvVar, authModeEnvVar, AuthModeClerk)
		}
	}

	return cfg, nil
}

func NewAuthHandler(cfg AuthConfig) (*AuthHandler, error) {
	handler := &AuthHandler{
		cfg:           cfg,
		clerkJWKCache: make(map[string]cachedClerkJWK),
	}

	switch cfg.Mode {
	case "", AuthModeNone:
		handler.cfg.Mode = AuthModeNone
	case AuthModeTrustedHeader:
		if strings.TrimSpace(cfg.TrustedUserHeader) == "" {
			return nil, fmt.Errorf("trusted-header auth requires a user header")
		}
	case AuthModeClerk:
		if strings.TrimSpace(cfg.ClerkPublishableKey) == "" {
			return nil, fmt.Errorf("clerk auth requires a publishable key")
		}
		if strings.TrimSpace(cfg.ClerkJWTKey) != "" {
			jwk, err := clerk.JSONWebKeyFromPEM(cfg.ClerkJWTKey)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", authClerkJWTKeyEnvVar, err)
			}
			handler.clerkStaticJWTKey = jwk
		}
		if strings.TrimSpace(cfg.ClerkSecretKey) != "" {
			clerkConfig := &clerk.ClientConfig{}
			clerkConfig.Key = clerk.String(cfg.ClerkSecretKey)
			handler.clerkJWKSClient = clerkjwks.NewClient(clerkConfig)
			clerkUsers := clerkuser.NewClient(clerkConfig)
			handler.clerkDeleteUser = func(ctx context.Context, subject string) error {
				_, err := clerkUsers.Delete(ctx, strings.TrimSpace(subject))
				return err
			}
		}
		if handler.clerkStaticJWTKey == nil && handler.clerkJWKSClient == nil {
			return nil, fmt.Errorf("clerk auth requires a secret key or jwt key")
		}
		handler.clerkAuthenticate = handler.authenticateClerk
	default:
		return nil, fmt.Errorf("unsupported auth mode %q", cfg.Mode)
	}
	return handler, nil
}

func NewNoAuthHandler() *AuthHandler {
	handler, err := NewAuthHandler(DefaultAuthConfig())
	if err != nil {
		panic(err)
	}
	return handler
}

func LoadAuthHandlerFromEnv() (*AuthHandler, error) {
	cfg, err := LoadAuthConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return NewAuthHandler(cfg)
}

func (a *AuthHandler) AttachMCPTokenAuthenticator(authenticate func(context.Context, string) (*AuthIdentity, error)) {
	if a == nil {
		return
	}
	a.mcpAuthenticate = authenticate
}

func (a *AuthHandler) AttachCLITokenAuthenticator(authenticate func(context.Context, string) (*AuthIdentity, error)) {
	if a == nil {
		return
	}
	a.cliAuthenticate = authenticate
}

func (a *AuthHandler) Middleware(next http.Handler) http.Handler {
	if a == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || isPublicAuthPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		identity, err := a.authenticate(r)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}
		if identity == nil {
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), authIdentityContextKey, *identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *AuthHandler) RuntimeConfig(r *http.Request) authRuntimeConfigResponse {
	response := authRuntimeConfigResponse{
		Mode:                string(a.mode()),
		Enabled:             a != nil && a.mode() != AuthModeNone,
		Provider:            string(a.mode()),
		SignInRequired:      a != nil && a.mode() != AuthModeNone,
		ProductMode:         ProductModeFromEnv(),
		ClerkPublishableKey: a.clerkPublishableKey(),
	}
	if identity, ok := AuthIdentityFromContext(r.Context()); ok {
		response.Authenticated = true
		response.User = &authRuntimeUser{
			Subject: identity.Subject,
			Name:    identity.Name,
			Email:   identity.Email,
			Groups:  append([]string(nil), identity.Groups...),
		}
		return response
	}
	if a != nil && a.mode() != AuthModeNone {
		if identity, err := a.authenticate(r); err == nil && identity != nil {
			response.Authenticated = true
			response.User = &authRuntimeUser{
				Subject: identity.Subject,
				Name:    identity.Name,
				Email:   identity.Email,
				Groups:  append([]string(nil), identity.Groups...),
			}
			return response
		}
	}
	if a == nil || a.mode() == AuthModeNone {
		response.Authenticated = true
	}
	return response
}

func (a *AuthHandler) mode() AuthMode {
	if a == nil {
		return AuthModeNone
	}
	if a.cfg.Mode == "" {
		return AuthModeNone
	}
	return a.cfg.Mode
}

func (a *AuthHandler) clerkPublishableKey() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.cfg.ClerkPublishableKey)
}

func (a *AuthHandler) CanDeleteIdentity() bool {
	return a != nil && a.mode() == AuthModeClerk && a.clerkDeleteUser != nil
}

func (a *AuthHandler) DeleteCurrentIdentity(ctx context.Context) error {
	if !a.CanDeleteIdentity() {
		return fmt.Errorf("account deletion requires Clerk account authentication")
	}
	identity, ok := AuthIdentityFromContext(ctx)
	if !ok || strings.TrimSpace(identity.Subject) == "" {
		return ErrUnauthorized
	}
	return a.clerkDeleteUser(ctx, identity.Subject)
}

func (a *AuthHandler) authenticate(r *http.Request) (*AuthIdentity, error) {
	if bearer, ok := bearerTokenFromRequest(r); ok {
		if isMCPTokenAuthPath(r.URL.Path) {
			if a.mcpAuthenticate == nil {
				return nil, ErrUnauthorized
			}
			identity, err := a.mcpAuthenticate(r.Context(), bearer)
			if err != nil {
				return nil, ErrUnauthorized
			}
			return identity, nil
		}
		if a.cliAuthenticate != nil {
			identity, err := a.cliAuthenticate(r.Context(), bearer)
			if err != nil {
				return nil, ErrUnauthorized
			}
			return identity, nil
		}
	}
	if a == nil || a.mode() == AuthModeNone {
		return nil, nil
	}

	switch a.cfg.Mode {
	case AuthModeTrustedHeader:
		subject := strings.TrimSpace(r.Header.Get(a.cfg.TrustedUserHeader))
		if subject == "" {
			return nil, ErrUnauthorized
		}
		identity := &AuthIdentity{
			Subject:  subject,
			Name:     strings.TrimSpace(r.Header.Get(a.cfg.TrustedNameHeader)),
			Email:    strings.TrimSpace(r.Header.Get(a.cfg.TrustedEmailHeader)),
			Groups:   splitHeaderValues(r.Header.Get(a.cfg.TrustedGroupsHeader)),
			Provider: string(AuthModeTrustedHeader),
		}
		return identity, nil
	case AuthModeClerk:
		if a.clerkAuthenticate == nil {
			return nil, fmt.Errorf("clerk auth is not initialized")
		}
		return a.clerkAuthenticate(r)
	default:
		return nil, fmt.Errorf("unsupported auth mode %q", a.cfg.Mode)
	}
}

func (a *AuthHandler) authenticateClerk(r *http.Request) (*AuthIdentity, error) {
	token := clerkSessionTokenFromRequest(r)
	if token == "" {
		return nil, ErrUnauthorized
	}
	claims, err := a.verifyClerkSessionToken(r.Context(), token)
	if err != nil {
		return nil, ErrUnauthorized
	}
	subject := strings.TrimSpace(claims.Subject)
	if subject == "" {
		return nil, ErrUnauthorized
	}
	return &AuthIdentity{
		Subject:  subject,
		Provider: string(AuthModeClerk),
	}, nil
}

func (a *AuthHandler) verifyClerkSessionToken(ctx context.Context, token string) (*clerk.SessionClaims, error) {
	params := &clerkjwt.VerifyParams{
		Token: token,
	}
	if strings.TrimSpace(a.cfg.ClerkProxyURL) != "" {
		params.ProxyURL = clerk.String(strings.TrimSpace(a.cfg.ClerkProxyURL))
	}
	if a.clerkStaticJWTKey != nil {
		params.JWK = a.clerkStaticJWTKey
		return clerkjwt.Verify(ctx, params)
	}

	decoded, err := clerkjwt.Decode(ctx, &clerkjwt.DecodeParams{Token: token})
	if err != nil {
		return nil, err
	}
	jwk, err := a.lookupClerkJWK(ctx, decoded.KeyID)
	if err != nil {
		return nil, err
	}
	params.JWK = jwk
	return clerkjwt.Verify(ctx, params)
}

func (a *AuthHandler) lookupClerkJWK(ctx context.Context, keyID string) (*clerk.JSONWebKey, error) {
	if strings.TrimSpace(keyID) == "" {
		return nil, fmt.Errorf("missing Clerk token key id")
	}

	now := time.Now().UTC()
	a.clerkJWKCacheMu.Lock()
	cached, ok := a.clerkJWKCache[keyID]
	if ok && cached.key != nil && now.Before(cached.expiresAt) {
		a.clerkJWKCacheMu.Unlock()
		return cached.key, nil
	}
	a.clerkJWKCacheMu.Unlock()

	if a.clerkJWKSClient == nil {
		return nil, fmt.Errorf("clerk jwks client is unavailable")
	}
	jwk, err := clerkjwt.GetJSONWebKey(ctx, &clerkjwt.GetJSONWebKeyParams{
		KeyID:      keyID,
		JWKSClient: a.clerkJWKSClient,
	})
	if err != nil {
		return nil, err
	}

	a.clerkJWKCacheMu.Lock()
	a.clerkJWKCache[keyID] = cachedClerkJWK{
		key:       jwk,
		expiresAt: now.Add(clerkJWKCacheTTL),
	}
	a.clerkJWKCacheMu.Unlock()
	return jwk, nil
}

func AuthIdentityFromContext(ctx context.Context) (AuthIdentity, bool) {
	if ctx == nil {
		return AuthIdentity{}, false
	}
	identity, ok := ctx.Value(authIdentityContextKey).(AuthIdentity)
	return identity, ok
}

func isPublicAuthPath(path string) bool {
	switch path {
	case "/healthz", "/v1/auth/config", "/v1/catalog/health", "/v1/auth/exchange", "/v1/cli", "/v1/version", "/install.sh":
		return true
	default:
		return false
	}
}

func clerkSessionTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	cookie, err := r.Cookie(clerkSessionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func bearerTokenFromRequest(r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if authorization == "" || !strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
		return "", false
	}
	token := strings.TrimSpace(authorization[len("Bearer "):])
	if token == "" {
		return "", false
	}
	return token, true
}

func isMCPTokenAuthPath(path string) bool {
	path = strings.TrimSpace(path)
	return path == "/mcp" || strings.HasPrefix(path, "/mcp/")
}

func splitHeaderValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}
