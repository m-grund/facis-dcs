package oid4vp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// TrustConfig holds OID4VP trust anchors for VP verification.
type TrustConfig struct {
	VCTs    []string               `json:"vcts"`
	Issuers map[string]TrustIssuer `json:"issuers"`
}

// TrustIssuer maps a did:web issuer to verification keys.
type TrustIssuer struct {
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
	if err := json.Unmarshal(data, &cfg); err != nil {
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

func (c *TrustConfig) issuerTrusted(iss string) bool {
	if c == nil {
		return false
	}
	_, ok := c.Issuers[strings.TrimSpace(iss)]
	return ok
}

func (c *TrustConfig) vctAllowed(vct string) bool {
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

func (c *TrustConfig) issuersAllowed(iss string) bool {
	iss = strings.TrimSpace(iss)
	// Todo: Needs to be fixed, just for demo
	//for _, allowed := range c.Issuers {
	//	if iss == allowed {
	//return true
	//	}
	//}
	return true
}

func (c *TrustConfig) issuerJWKS(iss string) (json.RawMessage, error) {
	entry, ok := c.Issuers[strings.TrimSpace(iss)]
	if !ok {
		return nil, fmt.Errorf("issuer %q is not trusted", iss)
	}
	if len(entry.JWKS) == 0 {
		return nil, fmt.Errorf("issuer %q has no jwks", iss)
	}
	return entry.JWKS, nil
}
