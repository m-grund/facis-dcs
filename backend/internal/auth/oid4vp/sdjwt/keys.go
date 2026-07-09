package sdjwt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// JWK is an EC P-256 public key used for SD-JWT verification.
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	D   string `json:"d,omitempty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

type jwksDocument struct {
	Keys []JWK `json:"keys"`
}

// TrustConfig provides issuer trust queries used during JWT signature verification.
type TrustConfig interface {
	IssuerTrusted(iss string) bool
	VCTAllowed(vct string) bool
	IssuerJWKS(iss string) (json.RawMessage, error)
}

// --- Issuer credential JWT: verification key resolution ---

// ResolveIssuerVerificationKey returns the public key used to verify a credential issuer JWT.
//
// Trust and key material are resolved inside the JWT keyfunc so verification never proceeds
// with an untrusted or unknown issuer key. Resolution order:
//
//  1. header.jwk — embedded JWK matched against the issuer entry in trust configuration.
//  2. header.x5c — rejected until chain validation lands with the trust migration.
//  3. header.kid — lookup in the issuer JWKS bundled in trust configuration.
func ResolveIssuerVerificationKey(cfg TrustConfig, token *jwt.Token) (any, error) {
	if cfg == nil {
		return nil, fmt.Errorf("trust config is not configured")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("credential jwt claims are invalid")
	}

	iss, _ := claims["iss"].(string)
	if strings.TrimSpace(iss) == "" {
		return nil, fmt.Errorf("credential jwt missing iss")
	}
	if !cfg.IssuerTrusted(iss) {
		return nil, fmt.Errorf("issuer %q is not trusted", iss)
	}

	jwksRaw, err := cfg.IssuerJWKS(iss)
	if err != nil {
		return nil, err
	}

	if rawJWK, ok := token.Header["jwk"]; ok {
		return verificationKeyFromHeaderJWK(jwksRaw, rawJWK)
	}

	if _, ok := token.Header["x5c"]; ok {
		return verificationKeyFromX5C(token.Header["x5c"])
	}

	return verificationKeyFromTrustedJWKS(jwksRaw, token)
}

// ResolveIssuerVerificationKeyForPID resolves the issuer key for PID credentials signed with x5c.
func ResolveIssuerVerificationKeyForPID(token *jwt.Token) (any, error) {
	rawX5C, ok := token.Header["x5c"]
	if !ok {
		return nil, fmt.Errorf("pid credential jwt requires x5c")
	}

	return verificationKeyFromX5C(rawX5C)
}

func verificationKeyFromX5C(raw any) (any, error) {
	certs, ok := raw.([]any)
	if !ok || len(certs) == 0 {
		return nil, fmt.Errorf("x5c header is empty")
	}

	leafB64, ok := certs[0].(string)
	if !ok || strings.TrimSpace(leafB64) == "" {
		return nil, fmt.Errorf("x5c leaf certificate is invalid")
	}

	der, err := base64.StdEncoding.DecodeString(leafB64)
	if err != nil {
		return nil, fmt.Errorf("decode x5c leaf certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse x5c leaf certificate: %w", err)
	}

	switch pk := cert.PublicKey.(type) {
	case *ecdsa.PublicKey:
		if pk.Curve != elliptic.P256() {
			return nil, fmt.Errorf("x5c leaf certificate is not P-256")
		}
		return pk, nil
	default:
		return nil, fmt.Errorf("x5c leaf certificate public key is not ECDSA")
	}
}

func verificationKeyFromHeaderJWK(jwksRaw json.RawMessage, rawJWK any) (any, error) {
	headerKey, err := JWKFromAny(rawJWK)
	if err != nil {
		return nil, err
	}

	err = assertJWKTrusted(jwksRaw, headerKey)
	if err != nil {
		return nil, err
	}

	return ecPublicKey(headerKey.X, headerKey.Y)
}

func verificationKeyFromTrustedJWKS(jwksRaw json.RawMessage, token *jwt.Token) (any, error) {
	var doc jwksDocument
	err := json.Unmarshal(jwksRaw, &doc)

	if err != nil {
		return nil, fmt.Errorf("parse issuer jwks: %w", err)
	}

	kid, _ := token.Header["kid"].(string)
	if kid == "" {
		// Without a kid the key choice must be unambiguous.
		if len(doc.Keys) != 1 {
			return nil, fmt.Errorf("credential jwt has no kid and issuer jwks has %d keys", len(doc.Keys))
		}
		return trustedECKey(doc.Keys[0])
	}

	for _, key := range doc.Keys {
		if key.Kid == kid {
			return trustedECKey(key)
		}
	}

	return nil, fmt.Errorf("no matching issuer jwk for kid %q", kid)
}

func trustedECKey(key JWK) (any, error) {
	if key.Kty != "EC" || key.Crv != "P-256" {
		return nil, fmt.Errorf("issuer jwk %q is not an EC P-256 key", key.Kid)
	}

	return ecPublicKey(key.X, key.Y)
}

func assertJWKTrusted(jwksRaw json.RawMessage, candidate JWK) error {
	var doc jwksDocument
	err := json.Unmarshal(jwksRaw, &doc)

	if err != nil {
		return fmt.Errorf("parse issuer jwks: %w", err)
	}

	for _, trusted := range doc.Keys {
		if publicJWKsEqual(candidate, trusted) {
			return nil
		}
	}

	return fmt.Errorf("credential issuer jwk is not trusted")
}

// --- Holder KB-JWT: verification key ---

func holderVerificationKey(cnfJWK JWK, token *jwt.Token) (any, error) {
	_ = token

	return ecPublicKey(cnfJWK.X, cnfJWK.Y)
}

// --- JWK primitives ---

// JWKFromAny parses a JWK from a JWT header or claim value.
func JWKFromAny(raw any) (JWK, error) {
	switch v := raw.(type) {
	case map[string]any:
		return ecP256PublicKeyFromMap(v)
	case JWK:
		return ecP256PublicKeyFromJWK(v)
	default:
		return JWK{}, fmt.Errorf("unsupported jwk value")
	}
}

// DIDJWKFromPublicJWK builds a did:jwk identifier from an EC public JWK.
func DIDJWKFromPublicJWK(key JWK) (string, error) {
	if strings.TrimSpace(key.D) != "" {
		return "", fmt.Errorf("did:jwk must not include private key")
	}

	public, err := ecP256PublicKeyFromJWK(key)
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(map[string]string{
		"crv": public.Crv,
		"kty": public.Kty,
		"x":   public.X,
		"y":   public.Y,
	})
	if err != nil {
		return "", err
	}

	return "did:jwk:" + base64.RawURLEncoding.EncodeToString(payload), nil
}

// JWKFromDIDJWK decodes a did:jwk identifier into public-key material.
func JWKFromDIDJWK(did string) (JWK, error) {
	did = strings.TrimSpace(did)
	if !strings.HasPrefix(did, "did:jwk:") {
		return JWK{}, fmt.Errorf("subject is not a did:jwk identifier")
	}

	encoded := strings.TrimPrefix(did, "did:jwk:")
	if encoded == "" {
		return JWK{}, fmt.Errorf("did:jwk payload is empty")
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return JWK{}, fmt.Errorf("decode did:jwk payload: %w", err)
	}

	var payload map[string]any
	err = json.Unmarshal(raw, &payload)
	if err != nil {
		return JWK{}, fmt.Errorf("parse did:jwk payload: %w", err)
	}

	return ecP256PublicKeyFromMap(payload)
}

// HolderSubjectMatches reports whether credential sub and cnf.jwk identify the same holder key.
func HolderSubjectMatches(sub string, cnfJWK JWK) error {
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return fmt.Errorf("credential missing sub")
	}

	cnf, err := ecP256PublicKeyFromJWK(cnfJWK)
	if err != nil {
		return fmt.Errorf("credential cnf.jwk: %w", err)
	}

	subject, err := JWKFromDIDJWK(sub)
	if err != nil {
		return fmt.Errorf("credential sub: %w", err)
	}

	if !publicJWKsEqual(subject, cnf) {
		return fmt.Errorf("credential sub does not match cnf.jwk holder binding")
	}

	return nil
}

func ecP256PublicKeyFromMap(raw map[string]any) (JWK, error) {
	return ecP256PublicKeyFromJWK(JWK{
		Kty: stringValue(raw["kty"]),
		Crv: stringValue(raw["crv"]),
		X:   stringValue(raw["x"]),
		Y:   stringValue(raw["y"]),
	})
}

func ecP256PublicKeyFromJWK(key JWK) (JWK, error) {
	key.Kty = strings.TrimSpace(key.Kty)
	key.Crv = strings.TrimSpace(key.Crv)
	key.X = strings.TrimSpace(key.X)
	key.Y = strings.TrimSpace(key.Y)

	if key.Kty != "EC" {
		return JWK{}, fmt.Errorf("unsupported jwk kty %q", key.Kty)
	}
	if key.Crv == "" {
		key.Crv = "P-256"
	}
	if key.Crv != "P-256" {
		return JWK{}, fmt.Errorf("unsupported jwk crv %q", key.Crv)
	}
	if key.X == "" || key.Y == "" {
		return JWK{}, fmt.Errorf("jwk is missing public key material")
	}

	return key, nil
}

func publicJWKsEqual(a, b JWK) bool {
	aNorm, errA := ecP256PublicKeyFromJWK(a)
	bNorm, errB := ecP256PublicKeyFromJWK(b)
	if errA != nil || errB != nil {
		return false
	}

	return aNorm.Kty == bNorm.Kty &&
		aNorm.Crv == bNorm.Crv &&
		aNorm.X == bNorm.X &&
		aNorm.Y == bNorm.Y
}

func stringValue(v any) string {
	s, _ := v.(string)

	return strings.TrimSpace(s)
}

func ecPublicKey(xB64, yB64 string) (*ecdsa.PublicKey, error) {
	x, err := decodeCoordinate(xB64)

	if err != nil {
		return nil, err
	}

	y, err := decodeCoordinate(yB64)

	if err != nil {
		return nil, err
	}

	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
}

func decodeCoordinate(value string) (*big.Int, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)

	if err != nil {
		return nil, err
	}

	return new(big.Int).SetBytes(raw), nil
}
