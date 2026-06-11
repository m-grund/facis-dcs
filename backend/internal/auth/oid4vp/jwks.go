package oid4vp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type jwksDocument struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	D   string `json:"d,omitempty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

func keySetKeyFunc(jwksRaw json.RawMessage, token *jwt.Token) (any, error) {
	var doc jwksDocument
	err := json.Unmarshal(jwksRaw, &doc)
	if err != nil {
		return nil, fmt.Errorf("parse issuer jwks: %w", err)
	}

	kid, _ := token.Header["kid"].(string)
	for _, key := range doc.Keys {
		if kid != "" && key.Kid != kid {
			continue
		}
		if key.Kty == "EC" && key.Crv == "P-256" {
			return ecPublicKey(key.X, key.Y)
		}
	}

	return nil, fmt.Errorf("no matching issuer jwk for kid %q", kid)
}

func holderKeyFunc(ctx context.Context, resolver DIDResolver, holderDID string, token *jwt.Token) (any, error) {
	_ = token
	holderDID = strings.TrimSpace(holderDID)
	if strings.HasPrefix(holderDID, "did:jwk:") {
		key, err := jwkFromDIDJWK(holderDID)
		if err != nil {
			return nil, err
		}
		return ecPublicKey(key.X, key.Y)
	}

	if resolver == nil {
		return nil, fmt.Errorf("did resolver is not configured")
	}

	pub, err := resolver.ResolvePublicJWK(ctx, holderDID)
	if err != nil {
		return nil, err
	}

	return ecPublicKey(pub.X, pub.Y)
}

func jwkFromDIDJWK(did string) (*jwkKey, error) {
	payload := strings.TrimPrefix(strings.TrimSpace(did), "did:jwk:")
	if payload == did {
		return nil, fmt.Errorf("not a did:jwk identifier")
	}

	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("decode did:jwk payload: %w", err)
	}

	var key jwkKey
	err = json.Unmarshal(raw, &key)
	if err != nil {
		return nil, fmt.Errorf("parse did:jwk public key: %w", err)
	}

	if key.Kty == "" || key.X == "" {
		return nil, fmt.Errorf("did:jwk payload is missing public key material")
	}

	return &key, nil
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

	n := new(big.Int).SetBytes(raw)
	return n, nil
}

// DIDJWKFromPublicJWK builds a did:jwk identifier from an EC public JWK.
func DIDJWKFromPublicJWK(key jwkKey) (string, error) {
	if key.D != "" {
		return "", fmt.Errorf("did:jwk must not include private key")
	}

	payload, err := json.Marshal(map[string]string{
		"crv": key.Crv,
		"kty": key.Kty,
		"x":   key.X,
		"y":   key.Y,
	})

	if err != nil {
		return "", err
	}

	return "did:jwk:" + base64.RawURLEncoding.EncodeToString(payload), nil
}
