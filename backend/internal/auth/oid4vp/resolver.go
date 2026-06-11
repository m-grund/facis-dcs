package oid4vp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultDIDResolverURL = "https://dev.uniresolver.io"

// DIDResolver resolves a DID to the holder's public JWK.
type DIDResolver interface {
	ResolvePublicJWK(ctx context.Context, did string) (*jwkKey, error)
}

// UniversalResolver resolves DIDs via the Universal Resolver HTTP API.
type UniversalResolver struct {
	baseURL string
	client  *http.Client
}

func NewUniversalResolverFromEnv() *UniversalResolver {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OID4VP_DID_RESOLVER_URL")), "/")
	if baseURL == "" {
		baseURL = defaultDIDResolverURL
	}
	return &UniversalResolver{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (r *UniversalResolver) ResolvePublicJWK(ctx context.Context, did string) (*jwkKey, error) {
	if r == nil {
		return nil, fmt.Errorf("did resolver is not configured")
	}
	did = strings.TrimSpace(did)
	if did == "" {
		return nil, fmt.Errorf("did is required")
	}

	endpoint, err := url.Parse(r.baseURL + "/1.0/identifiers/" + url.PathEscape(did))
	if err != nil {
		return nil, fmt.Errorf("build resolver url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build resolver request: %w", err)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("resolve did %q: %w", did, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read resolver response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resolve did %q: resolver returned %s: %s", did, resp.Status, strings.TrimSpace(string(body)))
	}

	methods, err := verificationMethodsFromResolverBody(body)
	if err != nil {
		return nil, err
	}

	for _, method := range methods {
		if len(method.PublicKeyJwk) == 0 {
			continue
		}
		var key jwkKey
		if err := json.Unmarshal(method.PublicKeyJwk, &key); err != nil {
			continue
		}
		if key.Kty != "EC" || key.Crv != "P-256" || key.X == "" || key.Y == "" {
			continue
		}
		return &key, nil
	}
	return nil, fmt.Errorf("resolve did %q: no P-256 EC public key in did document", did)
}

type resolverResponse struct {
	DIDDocument           resolverDIDDocument `json:"didDocument"`
	DIDResolutionMetadata struct {
		Error string `json:"error"`
	} `json:"didResolutionMetadata"`
}

type resolverDIDDocument struct {
	VerificationMethod []resolverVerificationMethod `json:"verificationMethod"`
}

type resolverVerificationMethod struct {
	Type         string          `json:"type"`
	PublicKeyJwk json.RawMessage `json:"publicKeyJwk"`
}

func verificationMethodsFromResolverBody(body []byte) ([]resolverVerificationMethod, error) {
	var wrapped resolverResponse
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("parse resolver response: %w", err)
	}
	if errMsg := strings.TrimSpace(wrapped.DIDResolutionMetadata.Error); errMsg != "" {
		return nil, fmt.Errorf("%s", errMsg)
	}
	if len(wrapped.DIDDocument.VerificationMethod) > 0 {
		return wrapped.DIDDocument.VerificationMethod, nil
	}

	var raw resolverDIDDocument
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse resolver did document: %w", err)
	}
	return raw.VerificationMethod, nil
}
