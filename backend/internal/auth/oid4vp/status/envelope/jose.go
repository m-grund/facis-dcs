package envelope

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type VerifiedJWT struct {
	Header map[string]any
	Claims map[string]any
	Raw    []byte
}

func VerifyES256JWT(raw []byte, resolveKey func(issuer string, token *jwt.Token) (*ecdsa.PublicKey, error)) (VerifiedJWT, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"ES256"}))
	token, err := parser.Parse(string(raw), func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodES256.Alg() {
			return nil, fmt.Errorf("unsupported JWS algorithm %q", t.Method.Alg())
		}
		claims, ok := t.Claims.(jwt.MapClaims)
		if !ok {
			return nil, fmt.Errorf("invalid jwt claims")
		}
		issuer := tokenIssuer(claims)
		if issuer == "" {
			return nil, fmt.Errorf("jwt missing iss or issuer claim")
		}
		return resolveKey(issuer, t)
	})
	if err != nil {
		return VerifiedJWT{}, fmt.Errorf("jwt verification failed: %w", err)
	}
	if !token.Valid {
		return VerifiedJWT{}, fmt.Errorf("jwt is not valid")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return VerifiedJWT{}, fmt.Errorf("invalid jwt claims")
	}

	header := make(map[string]any, len(token.Header))
	for k, v := range token.Header {
		header[k] = v
	}

	claimMap := make(map[string]any, len(claims))
	for k, v := range claims {
		claimMap[k] = v
	}

	return VerifiedJWT{
		Header: header,
		Claims: claimMap,
		Raw:    raw,
	}, nil
}

func JWTPayloadSegment(raw []byte) ([]byte, error) {
	parts := strings.Split(strings.TrimSpace(string(raw)), ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("not a compact JWT")
	}
	return base64.RawURLEncoding.DecodeString(parts[1])
}

func ClaimsFromJWT(raw []byte) (map[string]any, error) {
	payload, err := JWTPayloadSegment(raw)
	if err != nil {
		return nil, err
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func JWTHeader(raw []byte) (map[string]any, error) {
	parts := strings.Split(strings.TrimSpace(string(raw)), ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("not a compact JWT")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	var header map[string]any
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, err
	}
	return header, nil
}

func NormalizeContentType(contentType string) string {
	return strings.TrimSpace(strings.Split(contentType, ";")[0])
}

func tokenIssuer(claims jwt.MapClaims) string {
	if iss, ok := claims["iss"].(string); ok && strings.TrimSpace(iss) != "" {
		return strings.TrimSpace(iss)
	}
	if issuer, ok := claims["issuer"].(string); ok && strings.TrimSpace(issuer) != "" {
		return strings.TrimSpace(issuer)
	}
	return ""
}
