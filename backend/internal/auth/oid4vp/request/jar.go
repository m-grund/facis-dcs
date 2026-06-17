package request

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Signer signs OpenID4VP authorization request JWTs.
type Signer interface {
	SignAuthorizationRequestJWT(claims jwt.MapClaims) (string, error)
}

// Params are the OpenID4VP parameters encoded in the signed request JWT.
type Params struct {
	ClientID    string
	ResponseURI string
	State       string
	Nonce       string
	ExpiresAt   time.Time
	DCQLQuery   any
}

// BuildJWT creates a signed OpenID4VP authorization request object (JAR).
func BuildJWT(signer Signer, params Params) (string, error) {
	if signer == nil {
		return "", fmt.Errorf("request signer is not configured (set VAULT_ADDR and VAULT_TOKEN)")
	}

	clientID := strings.TrimSpace(params.ClientID)
	if clientID == "" {
		return "", fmt.Errorf("client_id is required")
	}

	responseURI := strings.TrimSpace(params.ResponseURI)
	if responseURI == "" {
		return "", fmt.Errorf("response_uri is required")
	}

	state := strings.TrimSpace(params.State)
	if state == "" {
		return "", fmt.Errorf("state is required")
	}

	nonce := strings.TrimSpace(params.Nonce)
	if nonce == "" {
		return "", fmt.Errorf("nonce is required")
	}

	if params.DCQLQuery == nil {
		return "", fmt.Errorf("dcql_query is required")
	}

	now := time.Now().UTC()
	exp := params.ExpiresAt.UTC()
	if exp.IsZero() || !exp.After(now) {
		exp = now.Add(5 * time.Minute)
	}

	dcqlJSON, err := json.Marshal(params.DCQLQuery)
	if err != nil {
		return "", fmt.Errorf("marshal dcql_query: %w", err)
	}
	var dcql any
	err = json.Unmarshal(dcqlJSON, &dcql)
	if err != nil {
		return "", fmt.Errorf("decode dcql_query: %w", err)
	}

	claims := jwt.MapClaims{
		"iss":           clientID,
		"client_id":     clientID,
		"response_type": "vp_token",
		"response_mode": "direct_post",
		"response_uri":  responseURI,
		"state":         state,
		"nonce":         nonce,
		"dcql_query":    dcql,
		"iat":           now.Unix(),
		"exp":           exp.Unix(),
	}
	return signer.SignAuthorizationRequestJWT(claims)
}
