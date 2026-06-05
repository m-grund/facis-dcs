package oid4vp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const vpFormat = "dc+sd-jwt"

// vpEnvelope is the VP token shape with separate credential and KB-JWT parts.
type vpEnvelope struct {
	Format        string `json:"format"`
	CredentialJWT string `json:"credential_jwt"`
	KBJWT         string `json:"kb_jwt"`
}

// TrustVerifier validates wallet presentations against trust.json issuers.
type TrustVerifier struct {
	trust    *TrustConfig
	resolver DIDResolver
}

func NewTrustVerifier(trust *TrustConfig) *TrustVerifier {
	return &TrustVerifier{
		trust:    trust,
		resolver: NewUniversalResolverFromEnv(),
	}
}

func (v *TrustVerifier) Verify(vpToken string, ctx PresentationContext) (*VerifiedLoginClaims, error) {
	if v == nil || v.trust == nil {
		return nil, fmt.Errorf("trust verifier is not configured")
	}
	env, err := parseVPEnvelope(vpToken)
	if err != nil {
		return nil, err
	}
	if env.Format != vpFormat {
		return nil, fmt.Errorf("unsupported vp format %q", env.Format)
	}

	credClaims, err := v.verifyCredentialJWT(env.CredentialJWT)
	if err != nil {
		return nil, err
	}
	if err := v.verifyKBJWT(env.KBJWT, env.CredentialJWT, ctx); err != nil {
		return nil, err
	}

	roles, err := rolesFromClaims(credClaims)
	if err != nil {
		return nil, err
	}
	org, _ := credClaims["organization"].(string)
	sub, _ := credClaims["sub"].(string)
	if strings.TrimSpace(sub) == "" {
		return nil, fmt.Errorf("credential missing sub")
	}
	if !v.trust.organizationAllowed(org) {
		return nil, fmt.Errorf("organization %q is not allowed", org)
	}

	raw, err := json.Marshal(credClaims)
	if err != nil {
		return nil, err
	}
	return &VerifiedLoginClaims{
		SubjectDID:     strings.TrimSpace(sub),
		OrganizationID: strings.TrimSpace(org),
		Roles:          roles,
		RawClaims:      raw,
	}, nil
}

func parseVPEnvelope(vpToken string) (*vpEnvelope, error) {
	raw := strings.TrimSpace(vpToken)
	if raw == "" {
		return nil, fmt.Errorf("vp_token is required")
	}
	if !strings.HasPrefix(raw, "{") {
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("vp_token is not JSON or base64url JSON")
		}
		raw = string(decoded)
	}
	var env vpEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return nil, fmt.Errorf("invalid vp envelope: %w", err)
	}
	if strings.TrimSpace(env.CredentialJWT) == "" || strings.TrimSpace(env.KBJWT) == "" {
		return nil, fmt.Errorf("vp envelope missing credential_jwt or kb_jwt")
	}
	return &env, nil
}

func (v *TrustVerifier) verifyCredentialJWT(token string) (jwt.MapClaims, error) {
	unverified, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("parse credential jwt: %w", err)
	}
	claims, ok := unverified.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("credential jwt claims are invalid")
	}
	iss, _ := claims["iss"].(string)
	iss = strings.TrimSpace(iss)
	if iss == "" {
		return nil, fmt.Errorf("credential jwt missing iss")
	}
	if !v.trust.issuerTrusted(iss) {
		return nil, fmt.Errorf("issuer %q is not trusted", iss)
	}
	vct, _ := claims["vct"].(string)
	if !v.trust.vctAllowed(strings.TrimSpace(vct)) {
		return nil, fmt.Errorf("vct %q is not allowed", vct)
	}
	if err := validateTimeClaims(claims); err != nil {
		return nil, err
	}

	jwksRaw, err := v.trust.issuerJWKS(iss)
	if err != nil {
		return nil, err
	}
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		return keySetKeyFunc(jwksRaw, t)
	}, jwt.WithValidMethods([]string{"ES256", "RS256"}))
	if err != nil {
		return nil, fmt.Errorf("credential jwt signature: %w", err)
	}
	out, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("credential jwt claims are invalid")
	}
	return out, nil
}

func (v *TrustVerifier) verifyKBJWT(token, credentialJWT string, ctx PresentationContext) error {
	unverified, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return fmt.Errorf("parse kb jwt: %w", err)
	}
	claims, ok := unverified.Claims.(jwt.MapClaims)
	if !ok {
		return fmt.Errorf("kb jwt claims are invalid")
	}
	nonce, _ := claims["nonce"].(string)
	if strings.TrimSpace(ctx.Nonce) == "" || nonce != ctx.Nonce {
		return fmt.Errorf("kb jwt nonce mismatch")
	}
	if hash, _ := claims["sd_hash"].(string); hash != "" {
		expected := sha256Base64URL(credentialJWT)
		if hash != expected {
			return fmt.Errorf("kb jwt sd_hash mismatch")
		}
	}
	if err := validateTimeClaims(claims); err != nil {
		return err
	}

	sub, _ := claims["sub"].(string)
	if strings.TrimSpace(sub) == "" {
		return fmt.Errorf("kb jwt missing sub")
	}
	resolveCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		return holderKeyFunc(resolveCtx, v.resolver, sub, t)
	}, jwt.WithValidMethods([]string{"ES256"}))
	if err != nil {
		return fmt.Errorf("kb jwt signature: %w", err)
	}
	if _, ok := parsed.Claims.(jwt.MapClaims); !ok {
		return fmt.Errorf("kb jwt claims are invalid")
	}
	return nil
}

func rolesFromClaims(claims jwt.MapClaims) ([]string, error) {
	raw, ok := claims["roles"]
	if !ok {
		return nil, fmt.Errorf("credential missing roles")
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("credential roles must be an array")
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("credential roles must be strings")
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("credential roles are empty")
	}
	return out, nil
}

func validateTimeClaims(claims jwt.MapClaims) error {
	now := time.Now().UTC()
	if expVal, ok := claims["exp"]; ok {
		exp, err := numericDate(expVal)
		if err != nil {
			return err
		}
		if now.After(exp) {
			return fmt.Errorf("token expired")
		}
	}
	if nbfVal, ok := claims["nbf"]; ok {
		nbf, err := numericDate(nbfVal)
		if err != nil {
			return err
		}
		if now.Before(nbf) {
			return fmt.Errorf("token not yet valid")
		}
	}
	return nil
}

func numericDate(v any) (time.Time, error) {
	switch t := v.(type) {
	case float64:
		return time.Unix(int64(t), 0).UTC(), nil
	case json.Number:
		sec, err := t.Int64()
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(sec, 0).UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("invalid numeric date")
	}
}

func sha256Base64URL(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
