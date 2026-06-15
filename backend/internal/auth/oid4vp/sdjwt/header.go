package sdjwt

import (
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// CredentialTyp is the JWT typ for issued dc+sd-jwt credentials.
const CredentialTyp = "dc+sd-jwt"

// KBJWTTyp is the JWT typ for holder key-binding JWTs.
const KBJWTTyp = "kb+jwt"

func validateCredentialHeader(token *jwt.Token) error {
	typ, _ := token.Header["typ"].(string)

	if strings.TrimSpace(typ) != CredentialTyp {
		return fmt.Errorf("credential jwt typ must be %q", CredentialTyp)
	}

	return nil
}

func validateKBHeader(token *jwt.Token) error {
	typ, _ := token.Header["typ"].(string)

	if strings.TrimSpace(typ) != KBJWTTyp {
		return fmt.Errorf("kb jwt typ must be %q", KBJWTTyp)
	}

	return nil
}
