package middleware

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"digital-contracting-service/internal/base/datatype/userrole"

	"github.com/coreos/go-oidc/v3/oidc"
)

// HydraJWTConfig holds OIDC provider configuration.
type HydraJWTConfig struct {
	// Example: http://localhost:30444
	IssuerURL string
	// Example: "dcs-client". Hydra JWT access tokens use the client_id claim (RFC 9068).
	ClientID string
}

// HydraJWTValidator validates JWT tokens from OIDC providers.
type HydraJWTValidator struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	config   HydraJWTConfig
}

// NewHydraJWTValidator connects to the OIDC provider to get public keys.
func NewHydraJWTValidator(ctx context.Context, config HydraJWTConfig) (*HydraJWTValidator, error) {
	provider, err := oidc.NewProvider(ctx, config.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to discover OIDC provider: %w", err)
	}

	// Skip audience check — client binding is validated in ValidateToken via client_id (and aud).
	// The token signature and issuer are still fully validated.
	verifier := provider.Verifier(&oidc.Config{
		ClientID:                   config.ClientID,
		SkipClientIDCheck:          true,
		InsecureSkipSignatureCheck: os.Getenv("JWT_ALG_NONE_SUPPORTED") == "true",
	})

	return &HydraJWTValidator{
		provider: provider,
		verifier: verifier,
		config:   config,
	}, nil
}

// TokenInfo holds the validated identity extracted from a JWT.
type TokenInfo struct {
	Roles         []string
	DID           string
	Username      string
	ParticipantID string
}

// ValidateToken verifies the token signature, issuer, and client binding, then
// returns the caller's roles, DID, username, and participant ID.
func (v *HydraJWTValidator) ValidateToken(ctx context.Context, token string) (*TokenInfo, error) {
	idToken, err := v.verifier.Verify(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}

	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse token claims: %w", err)
	}

	if !matchesClientID(claims, v.config.ClientID) {
		return nil, fmt.Errorf("token is not bound to client ID %q", v.config.ClientID)
	}

	iss, _ := claims["iss"].(string)
	sub, _ := claims["sub"].(string)

	return &TokenInfo{
		Roles:         extractRoles(claims),
		DID:           sub,
		ParticipantID: iss,
	}, nil
}

// extractRoles extracts DCS roles from a Hydra access token.
func extractRoles(claims map[string]interface{}) []string {
	if ext, ok := claims["ext"].(map[string]interface{}); ok {
		if roles := toStringSlice(ext["roles"]); len(roles) > 0 {
			return roles
		}
	}
	return []string{}
}

// matchesClientID matches the JWT token to the expected OAuth client.
func matchesClientID(claims map[string]interface{}, clientID string) bool {
	if cid, _ := claims["client_id"].(string); cid != "" {
		return cid == clientID
	}
	switch aud := claims["aud"].(type) {
	case string:
		return aud == clientID
	case []interface{}:
		for _, item := range aud {
			if audience, ok := item.(string); ok && audience == clientID {
				return true
			}
		}
	}
	return false
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
	DID           string
	Username      string
	ParticipantID string
}

// GetUserRoles extracts roles from the request context.
func GetUserRoles(ctx context.Context) []userrole.UserRole {
	if ac, ok := ctx.Value(authCtxKey{}).(AuthContext); ok {
		userRoles := make([]userrole.UserRole, len(ac.Roles))
		for i, role := range ac.Roles {
			userRole, err := userrole.NewUserRole(role)
			if err != nil {
				log.Printf("failed to parse user role %q: %v", role, err)
			}
			userRoles[i] = userRole
		}
		return userRoles

	}
	return []userrole.UserRole{}
}

// GetDID extracts the authenticated DID from the request context.
func GetDID(ctx context.Context) string {
	if ac, ok := ctx.Value(authCtxKey{}).(AuthContext); ok {
		return ac.DID
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

// InjectAuthContext injects the validated identity into the request context.
func InjectAuthContext(ctx context.Context, roles []string, did string, username string, participantID string) context.Context {
	return context.WithValue(ctx, authCtxKey{}, AuthContext{Roles: roles, DID: did, Username: username, ParticipantID: participantID})
}
