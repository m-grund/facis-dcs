package oid4vp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// TrustConfig is the verifier trust anchor loaded from trust.dev.json (OID4VP_TRUST_DATA_PATH).
// It records which credential types and issuer DIDs are accepted, and bundles their JWKS.
// JWT public-key resolution for issuer signatures is in sdjwt/keys.go.
type TrustConfig struct {
	VCTs    []string                 `json:"vcts"`
	Issuers map[string]TrustedIssuer `json:"issuers"`
}

// TrustedIssuer holds verification keys for one issuer DID entry in trust configuration.
type TrustedIssuer struct {
	JWKS json.RawMessage `json:"jwks"`
}

// LoadTrustConfig reads trust data from a JSON file (ConfigMap mount).
func LoadTrustConfig(path string) (*TrustConfig, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("trust config path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read trust config %q: %w", path, err)
	}

	var cfg TrustConfig
	err = json.Unmarshal(data, &cfg)

	if err != nil {
		return nil, fmt.Errorf("parse trust config %q: %w", path, err)
	}

	if len(cfg.VCTs) == 0 {
		return nil, fmt.Errorf("trust config %q: vcts is required", path)
	}

	if len(cfg.Issuers) == 0 {
		return nil, fmt.Errorf("trust config %q: issuers is required", path)
	}

	return &cfg, nil
}

// LoadTrustConfigFromEnv loads trust data from OID4VP_TRUST_DATA_PATH.
func LoadTrustConfigFromEnv() (*TrustConfig, error) {
	return LoadTrustConfig(os.Getenv("OID4VP_TRUST_DATA_PATH"))
}

func (c *TrustConfig) IssuerTrusted(iss string) bool {
	if c == nil {
		return false
	}
	_, ok := c.Issuers[strings.TrimSpace(iss)]

	return ok
}

func (c *TrustConfig) VCTAllowed(vct string) bool {
	if c == nil {
		return false
	}

	vct = strings.TrimSpace(vct)

	for _, allowed := range c.VCTs {
		if vct == allowed {
			return true
		}
	}

	return false
}

func (c *TrustConfig) IssuerJWKS(iss string) (json.RawMessage, error) {
	entry, ok := c.Issuers[strings.TrimSpace(iss)]
	if !ok {
		return nil, fmt.Errorf("issuer %q is not trusted", iss)
	}

	if len(entry.JWKS) == 0 {
		return nil, fmt.Errorf("issuer %q has no jwks", iss)
	}

	return entry.JWKS, nil
}
