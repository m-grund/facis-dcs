package sdjwt

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// Presentation is a dc+sd-jwt presentation (issuer JWT, disclosures, holder KB-JWT).
type Presentation struct {
	IssuerJWT   string
	Disclosures []string
	KBJWT       string
	SDHash      string
}

// ParsePresentation splits a compact SD-JWT+KB presentation string.
func ParsePresentation(vpToken string) (*Presentation, error) {
	rawParts := strings.Split(strings.TrimSpace(vpToken), "~")
	if len(rawParts) == 0 || strings.TrimSpace(rawParts[0]) == "" {
		return nil, fmt.Errorf("vp_token is required")
	}

	issuerJWT := strings.TrimSpace(rawParts[0])
	if !strings.HasPrefix(issuerJWT, "eyJ") {
		return nil, fmt.Errorf("sd-jwt presentation must start with issuer jwt")
	}

	remainder := rawParts[1:]
	var kbJWT string

	if len(remainder) > 0 && remainder[len(remainder)-1] == "" {
		remainder = remainder[:len(remainder)-1]
	} else if len(remainder) > 0 && looksLikeJWT(remainder[len(remainder)-1]) {
		kbJWT = strings.TrimSpace(remainder[len(remainder)-1])
		remainder = remainder[:len(remainder)-1]
	}

	disclosures := make([]string, 0, len(remainder))
	for _, part := range remainder {
		part = strings.TrimSpace(part)
		if part != "" {
			disclosures = append(disclosures, part)
		}
	}

	if kbJWT == "" {
		return nil, fmt.Errorf("sd-jwt presentation is missing kb-jwt")
	}

	if !strings.HasPrefix(kbJWT, "eyJ") {
		return nil, fmt.Errorf("kb-jwt must be a compact jwt")
	}

	return &Presentation{
		IssuerJWT:   issuerJWT,
		Disclosures: disclosures,
		KBJWT:       kbJWT,
		SDHash:      SDHash(issuerJWT, disclosures),
	}, nil
}

func looksLikeJWT(value string) bool {
	value = strings.TrimSpace(value)

	return strings.HasPrefix(value, "eyJ") && strings.Count(value, ".") >= 2
}

func presentationBodyForSDHash(issuerJWT string, disclosures []string) string {
	body := issuerJWT + "~"
	for _, disclosure := range disclosures {
		body += disclosure + "~"
	}

	return body
}

// SDHash returns the RFC 9901 sd_hash for the given issuer JWT and disclosures.
func SDHash(issuerJWT string, disclosures []string) string {
	return sha256Base64URL(presentationBodyForSDHash(issuerJWT, disclosures))
}

func sha256Base64URL(value string) string {
	sum := sha256.Sum256([]byte(value))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}
