package sdjwt

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

const defaultSDAlg = "sha-256"

// MergeDisclosedClaims merges selectively disclosed claims into issuer-signed payload claims.
func MergeDisclosedClaims(issuerClaims jwt.MapClaims, disclosures []string) (jwt.MapClaims, error) {
	out := make(jwt.MapClaims, len(issuerClaims)+len(disclosures))

	for k, v := range issuerClaims {
		out[k] = v
	}

	delete(out, "_sd")
	delete(out, "_sd_alg")

	for _, encoded := range disclosures {
		arr, err := decodeDisclosure(encoded)
		if err != nil {
			return nil, err
		}
		claimName, ok := arr[1].(string)
		if !ok || strings.TrimSpace(claimName) == "" {
			return nil, fmt.Errorf("disclosure claim name must be a non-empty string")
		}
		out[claimName] = arr[2]
	}

	return out, nil
}

// VerifyDisclosures checks that each disclosure digest is listed in the credential _sd array.
func VerifyDisclosures(issuerClaims jwt.MapClaims, disclosures []string) error {
	sdAlg, _ := issuerClaims["_sd_alg"].(string)

	if strings.TrimSpace(sdAlg) == "" {
		sdAlg = defaultSDAlg
	}

	if sdAlg != defaultSDAlg {
		return fmt.Errorf("unsupported _sd_alg %q", sdAlg)
	}

	rawSD, ok := issuerClaims["_sd"]

	if !ok {
		return fmt.Errorf("credential missing _sd")
	}

	sdHashes, err := stringSliceFromAny(rawSD)

	if err != nil {
		return err
	}

	if len(sdHashes) == 0 {
		return fmt.Errorf("credential _sd is empty")
	}

	seen := make(map[string]struct{}, len(disclosures))
	for _, encoded := range disclosures {
		digest := disclosureDigest(encoded)
		if !containsString(sdHashes, digest) {
			return fmt.Errorf("disclosure digest is not listed in credential _sd")
		}
		if _, dup := seen[digest]; dup {
			return fmt.Errorf("duplicate disclosure digest")
		}
		seen[digest] = struct{}{}
	}

	return nil
}

func disclosureDigest(encodedDisclosure string) string {
	return sha256Base64URL(encodedDisclosure)
}

func decodeDisclosure(encoded string) ([]any, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)

	if err != nil {
		return nil, fmt.Errorf("decode disclosure: %w", err)
	}

	var arr []any
	err = json.Unmarshal(raw, &arr)

	if err != nil {
		return nil, fmt.Errorf("parse disclosure json: %w", err)
	}

	if len(arr) != 3 {
		return nil, fmt.Errorf("property disclosure must be a three-element array")
	}

	return arr, nil
}

func stringSliceFromAny(raw any) ([]string, error) {
	arr, ok := raw.([]any)

	if !ok {
		return nil, fmt.Errorf("expected json array")
	}
	out := make([]string, 0, len(arr))

	for _, item := range arr {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("expected string array")
		}
		out = append(out, s)
	}

	return out, nil
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}

	return false
}
