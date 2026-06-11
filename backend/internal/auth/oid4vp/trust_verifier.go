package oid4vp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

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

	presentation, err := parseSDJWTPresentation(vpToken)
	if err != nil {
		return nil, err
	}

	credClaims, err := v.verifyCredentialJWT(presentation.CredentialJWT)
	if err != nil {
		return nil, err
	}

	err = v.verifyKBJWT(presentation.KBJWT, presentation.SDHash, ctx)
	if err != nil {
		return nil, err
	}

	roles, err := rolesFromClaims(credClaims)
	if err != nil {
		return nil, err
	}

	iss, _ := credClaims["iss"].(string)
	if strings.TrimSpace(iss) == "" {
		return nil, fmt.Errorf("credential missing iss")
	}

	if !v.trust.issuerTrusted(iss) {
		return nil, fmt.Errorf("issuer %q is not trusted", iss)
	}

	sub, _ := credClaims["sub"].(string)
	if strings.TrimSpace(sub) == "" {
		return nil, fmt.Errorf("credential missing sub")
	}

	organization, _ := credClaims["organization"].(string)
	if strings.TrimSpace(organization) == "" {
		return nil, fmt.Errorf("credential missing organization")
	}

	raw, err := json.Marshal(credClaims)
	if err != nil {
		return nil, err
	}

	return &VerifiedLoginClaims{
		SubjectDID:     strings.TrimSpace(sub),
		ParticipantDID: strings.TrimSpace(organization),
		Roles:          roles,
		RawClaims:      raw,
	}, nil
}

func (v *TrustVerifier) verifyCredentialJWT(token string) (jwt.MapClaims, error) {
	unverified, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("parse credential jwt: %w", err)
	}
	typ, _ := unverified.Header["typ"].(string)
	if strings.TrimSpace(typ) != CredentialJWTTyp {
		return nil, fmt.Errorf("credential jwt typ must be %q", CredentialJWTTyp)
	}

	claims, ok := unverified.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("credential jwt claims are invalid")
	}

	sub, _ := claims["sub"].(string)
	if strings.TrimSpace(sub) == "" {
		return nil, fmt.Errorf("credential jwt missing sub")
	}

	iss, _ := claims["iss"].(string)
	if strings.TrimSpace(iss) == "" {
		return nil, fmt.Errorf("credential jwt missing iss")
	}
	if !v.trust.issuerTrusted(iss) {
		return nil, fmt.Errorf("issuer %q is not trusted", iss)
	}

	vct, _ := claims["vct"].(string)
	if !v.trust.vctAllowed(strings.TrimSpace(vct)) {
		return nil, fmt.Errorf("vct %q is not allowed", vct)
	}

	err = validateTimeClaims(claims)
	if err != nil {
		return nil, err
	}

	jwksRaw, err := v.trust.issuerJWKS(iss)
	if err != nil {
		return nil, err
	}

	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		return keySetKeyFunc(jwksRaw, t)
	}, jwt.WithValidMethods([]string{"ES256"}))
	if err != nil {
		return nil, fmt.Errorf("credential jwt signature: %w", err)
	}

	out, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("credential jwt claims are invalid")
	}
	return out, nil
}

func (v *TrustVerifier) verifyKBJWT(token, expectedSDHash string, ctx PresentationContext) error {
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

	clientID := strings.TrimSpace(ctx.ClientID)
	if clientID == "" {
		return fmt.Errorf("kb jwt verification requires client_id")
	}
	aud, _ := claims["aud"].(string)
	if strings.TrimSpace(aud) != clientID {
		return fmt.Errorf("kb jwt aud mismatch")
	}

	if hash, _ := claims["sd_hash"].(string); hash != "" {
		if expectedSDHash == "" || hash != expectedSDHash {
			return fmt.Errorf("kb jwt sd_hash mismatch")
		}
	}

	err = validateTimeClaims(claims)
	if err != nil {
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
