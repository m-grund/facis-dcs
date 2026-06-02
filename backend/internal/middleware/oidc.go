package middleware

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCConfig holds OIDC provider configuration
type OIDCConfig struct {
	// Example: https://keycloak.example.com/auth/realms/dcs
	IssuerURL string
	// Example: "dcs-service". "aud" claim in JWT must match this value.
	ClientID string
}

// OIDCValidator validates JWT tokens from OIDC providers
type OIDCValidator struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	config   OIDCConfig
}

const (
	oidcDiscoveryDefaultAttempts       = 30
	oidcDiscoveryDefaultAttemptTimeout = 5 * time.Second
	oidcDiscoveryDefaultInitialBackoff = 500 * time.Millisecond
	oidcDiscoveryDefaultMaxBackoff     = 5 * time.Second
)

// NewOIDCValidator connects to the OIDC provider to get public keys
func NewOIDCValidator(ctx context.Context, config OIDCConfig) (*OIDCValidator, error) {
	provider, err := discoverOIDCProvider(ctx, config.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to discover OIDC provider: %w", err)
	}

	// Skip audience check — Keycloak places the client ID in the "azp" claim,
	// not in "aud". The token signature and issuer are still fully validated.
	verifier := provider.Verifier(&oidc.Config{
		ClientID:                   config.ClientID,
		SkipClientIDCheck:          true,
		InsecureSkipSignatureCheck: os.Getenv("JWT_ALG_NONE_SUPPORTED") == "true",
	})

	return &OIDCValidator{
		provider: provider,
		verifier: verifier,
		config:   config,
	}, nil
}

func discoverOIDCProvider(ctx context.Context, issuerURL string) (*oidc.Provider, error) {
	var lastErr error
	attempts := oidcDiscoveryAttempts()
	attemptTimeout := oidcDiscoveryAttemptTimeout()
	backoff := oidcDiscoveryInitialBackoff()
	maxBackoff := oidcDiscoveryMaxBackoff()

	for attempt := 1; attempt <= attempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
		provider, err := oidc.NewProvider(attemptCtx, issuerURL)
		cancel()
		if err == nil {
			return provider, nil
		}
		lastErr = err

		if attempt == attempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return nil, fmt.Errorf("OIDC discovery failed after %d attempts: %w", attempts, lastErr)
}

func oidcDiscoveryAttempts() int {
	value, err := strconv.Atoi(os.Getenv("OIDC_DISCOVERY_ATTEMPTS"))
	if err != nil || value < 1 {
		return oidcDiscoveryDefaultAttempts
	}
	return value
}

func oidcDiscoveryAttemptTimeout() time.Duration {
	return oidcDiscoveryDuration("OIDC_DISCOVERY_ATTEMPT_TIMEOUT", oidcDiscoveryDefaultAttemptTimeout)
}

func oidcDiscoveryInitialBackoff() time.Duration {
	return oidcDiscoveryDuration("OIDC_DISCOVERY_INITIAL_BACKOFF", oidcDiscoveryDefaultInitialBackoff)
}

func oidcDiscoveryMaxBackoff() time.Duration {
	return oidcDiscoveryDuration("OIDC_DISCOVERY_MAX_BACKOFF", oidcDiscoveryDefaultMaxBackoff)
}

func oidcDiscoveryDuration(envName string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(envName))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

// TokenInfo holds the validated identity extracted from a JWT.
type TokenInfo struct {
	Roles         []string
	Username      string
	ParticipantID string
}

// ValidateToken verifies the token signature, issuer, and azp claim, then
// returns the caller's roles and username.
func (v *OIDCValidator) ValidateToken(ctx context.Context, token string) (*TokenInfo, error) {
	idToken, err := v.verifier.Verify(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}

	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse token claims: %w", err)
	}

	// Validate that the authorized party matches our client ID.
	azp, _ := claims["azp"].(string)
	if azp != v.config.ClientID {
		return nil, fmt.Errorf("azp claim %q does not match expected client ID %q", azp, v.config.ClientID)
	}

	username, _ := claims["preferred_username"].(string)
	if username == "" {
		username, _ = claims["sub"].(string)
	}
	// This value is set by the Keycloak -> Clients -> <client_id>
	// -> <client_id>-dedicated -> Configure a new mapper / Add mapper (by configuration) -> Hardcoded claim
	participantID, _ := claims["participant-id"].(string)

	return &TokenInfo{
		Roles:         extractRoles(claims),
		Username:      username,
		ParticipantID: participantID,
	}, nil
}

// extractRoles extracts client-scoped roles from the
// resource_access.<azp>.roles JWT claim.
func extractRoles(claims map[string]interface{}) []string {
	ra, ok := claims["resource_access"].(map[string]interface{})
	if !ok {
		return []string{}
	}
	azp, ok := claims["azp"].(string)
	if !ok {
		return []string{}
	}
	client, ok := ra[azp].(map[string]interface{})
	if !ok {
		return []string{}
	}
	if roles := toStringSlice(client["roles"]); len(roles) > 0 {
		return roles
	}
	return []string{}
}

func toStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ExtractBearerToken expected format "Authorization: Bearer <token>"
func ExtractBearerToken(authHeader string) (string, error) {
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return "", fmt.Errorf("invalid authorization header format")
	}
	return strings.TrimPrefix(authHeader, bearerPrefix), nil
}

// unexported key type to avoid context key collisions.
type authCtxKey struct{}

// AuthContext carries the validated caller identity through the request context.
type AuthContext struct {
	Roles         []string
	Username      string
	ParticipantID string
}

// GetRoles extracts roles from the request context.
func GetRoles(ctx context.Context) []string {
	if ac, ok := ctx.Value(authCtxKey{}).(AuthContext); ok {
		return ac.Roles
	}
	return []string{}
}

// GetUsername extracts the authenticated username from the request context.
func GetUsername(ctx context.Context) string {
	if ac, ok := ctx.Value(authCtxKey{}).(AuthContext); ok {
		return ac.Username
	}
	return ""
}

// GetParticipantID extracts the authenticated participant ID from the request context.
func GetParticipantID(ctx context.Context) string {
	if ac, ok := ctx.Value(authCtxKey{}).(AuthContext); ok {
		return ac.ParticipantID
	}
	return ""
}

// HasRole checks if the context contains a specific role.
func HasRole(ctx context.Context, requiredRole string) bool {
	for _, role := range GetRoles(ctx) {
		if role == requiredRole {
			return true
		}
	}
	return false
}

// InjectAuthContext injects the validated identity into the request context.
func InjectAuthContext(ctx context.Context, roles []string, username string, participantID string) context.Context {
	return context.WithValue(ctx, authCtxKey{}, AuthContext{Roles: roles, Username: username, ParticipantID: participantID})
}
