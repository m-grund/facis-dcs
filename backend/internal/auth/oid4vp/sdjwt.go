package oid4vp

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// sdjwtPresentation is a flat dc+sd-jwt presentation (issuer credential JWT + holder KB-JWT).
type sdjwtPresentation struct {
	Presentation  string
	CredentialJWT string
	KBJWT         string
	SDHash        string
}

func parseSDJWTPresentation(vpToken string) (*sdjwtPresentation, error) {
	raw := strings.TrimSpace(vpToken)
	if raw == "" {
		return nil, fmt.Errorf("vp_token is required")
	}

	if strings.HasPrefix(raw, "{") {
		return nil, fmt.Errorf("vp_token must be an SD-JWT presentation (issuer-jwt~kb-jwt), not JSON")
	}

	parts := strings.Split(raw, "~")
	if len(parts) < 2 {
		return nil, fmt.Errorf("sd-jwt presentation must contain issuer jwt and kb-jwt separated by ~")
	}

	credentialJWT := strings.TrimSpace(parts[0])
	kbJWT := strings.TrimSpace(parts[len(parts)-1])

	if credentialJWT == "" || kbJWT == "" {
		return nil, fmt.Errorf("sd-jwt presentation is missing issuer jwt or kb-jwt")
	}

	if !strings.HasPrefix(credentialJWT, "eyJ") || !strings.HasPrefix(kbJWT, "eyJ") {
		return nil, fmt.Errorf("sd-jwt presentation parts must be compact jwts")
	}

	presentation := strings.Join(parts[:len(parts)-1], "~")

	return &sdjwtPresentation{
		Presentation:  presentation,
		CredentialJWT: credentialJWT,
		KBJWT:         kbJWT,
		SDHash:        sha256Base64URL(presentation),
	}, nil
}

func sha256Base64URL(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
