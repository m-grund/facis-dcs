package provenance

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// StatusListPublisher publishes contract status to a status list service (DCS-OR-C2PA-005).
// This integrates with XFSC's OCM-W Status List Service to maintain a verifiable
// status list (Status List 2021/2023 format) with ≤ 5 minute update latency.
type StatusListPublisher interface {
	// PublishStatus updates the contract status in the status list.
	// Returns the status list URI and any error.
	PublishStatus(
		ctx context.Context,
		contractID string,
		status string, // "active", "suspended", "terminated", "expired", etc.
		reason string,
		effectiveAt time.Time,
	) (statusListURI string, err error)

	// RevokeStatus marks a contract as revoked in the status list.
	RevokeStatus(ctx context.Context, contractID string) (statusListURI string, err error)
}

// listSize is the number of entries in a standard 16 KB bitstring status list (2^17).
const listSize = 131072

// defaultListID is the list used for contract revocation (1-indexed).
const defaultListID = 1

// OCMWStatusListPublisher is a client for the XFSC statuslist-service.
// It calls POST /v1/tenants/{tenantID}/status/revoke/{listID}/{index} to revoke entries.
// The status list VC is available at GET /v1/tenants/{tenantID}/status/{listID}.
//
// Indices are derived deterministically from the contractID SHA-256 so no
// per-contract allocation table is required.
type OCMWStatusListPublisher struct {
	// ServiceURL is the statuslist-service root endpoint (e.g., http://statuslist:8080).
	ServiceURL string

	// IssuerDID is the issuer DID that owns the status list.
	IssuerDID string

	// TenantID is the tenant identifier in the statuslist-service path (default "default").
	TenantID string

	client *http.Client
}

// NewOCMWStatusListPublisher creates a status list publisher that calls the
// XFSC statuslist-service HTTP API.  tenantID may be empty, defaulting to "default".
func NewOCMWStatusListPublisher(serviceURL, issuerDID, tenantID string) *OCMWStatusListPublisher {
	if tenantID == "" {
		tenantID = "default"
	}
	return &OCMWStatusListPublisher{
		ServiceURL: serviceURL,
		IssuerDID:  issuerDID,
		TenantID:   tenantID,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// statusListURI returns the URL at which the status list VC can be fetched.
func (p *OCMWStatusListPublisher) statusListURI() string {
	return fmt.Sprintf("%s/v1/tenants/%s/status/%d", p.ServiceURL, p.TenantID, defaultListID)
}

// StatusListIndex returns the bitstring position for contractID.
// Uses the first 4 bytes of SHA-256(contractID) modulo listSize so the
// index is deterministic without requiring a per-contract allocation table.
func StatusListIndex(contractID string) uint32 {
	h := sha256.Sum256([]byte(contractID))
	return binary.BigEndian.Uint32(h[:4]) % listSize
}

// revokeResponse is the JSON shape returned by the statuslist-service revoke endpoint.
type revokeResponse struct {
	TenantID string `json:"tenantId"`
	ListID   int    `json:"listId"`
	Index    int    `json:"index"`
	Status   string `json:"status"`
}

// setRevoked calls POST /{tenantID}/status/{listID}/revoke/{index}.
// ServiceURL must be non-empty; an empty URL is a hard failure (DCS hard-failure policy).
func (p *OCMWStatusListPublisher) setRevoked(ctx context.Context, contractID string) error {
	if p.ServiceURL == "" {
		return fmt.Errorf("status list ServiceURL must not be empty: required for revocation of %s", contractID)
	}
	index := StatusListIndex(contractID)
	url := fmt.Sprintf("%s/v1/tenants/%s/status/%d/revoke/%d", p.ServiceURL, p.TenantID, defaultListID, index)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(nil))
	if err != nil {
		return fmt.Errorf("build revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("could not close response body:", err)
		}
	}(resp.Body)

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("statuslist-service revoke returned %d: %s", resp.StatusCode, body)
	}

	var r revokeResponse
	if err := json.Unmarshal(body, &r); err == nil {
		_ = r // parsed for logging; ignore unmarshal errors on unexpected shapes
	}
	return nil
}

// PublishStatus updates the contract status in the XFSC status list (DCS-OR-C2PA-005).
// Terminal states (terminated, expired, replaced, suspended) set the revocation bit.
// Comparison is case-insensitive so CWE UPPERCASE states (TERMINATED, EXPIRED, …)
// are handled correctly alongside the SRS lowercase vocabulary.
// Active/draft/amended states are the default (not revoked) and require no HTTP call.
func (p *OCMWStatusListPublisher) PublishStatus(
	ctx context.Context,
	contractID string,
	status string,
	reason string,
	effectiveAt time.Time,
) (statusListURI string, err error) {
	switch strings.ToLower(status) {
	case "terminated", "expired", "replaced", "suspended":
		if err := p.setRevoked(ctx, contractID); err != nil {
			return "", fmt.Errorf("publish status %s for %s: %w", status, contractID, err)
		}
	}
	// active, draft, approved, amended — default state = not revoked, no action required.
	return p.statusListURI(), nil
}

// statusListResponse is the JSON shape actually returned by the deployed XFSC
// statuslist-service for GET /v1/tenants/{tenant}/status/{listId}:
//
//	{"list": "<base64, gzip-compressed bitstring>", "listId": 1, "tenantId": "default"}
//
// This is NOT a W3C VC (no credentialSubject wrapper).
type statusListResponse struct {
	List string `json:"list"`
}

// QueryStatusListStatus fetches the status list at statusListCredential and returns
// "revoked" if the entry at index is set, "active" otherwise (DCS-OR-C2PA-006).
//
// The XFSC statuslist-service (deployment/helm/charts/statuslist-service) returns a
// plain {"list": "...", "listId": ..., "tenantId": "..."} JSON object rather than a
// W3C VC; "list" is a base64-encoded, gzip-compressed bitstring. Bit packing
// follows the IETF Token Status List / XFSC convention (LSB-first), matching the
// parsing already established for status list checks in internal/auth/oid4vp.
func QueryStatusListStatus(ctx context.Context, client *http.Client, statusListCredential string, index uint32) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusListCredential, nil)
	if err != nil {
		return "", fmt.Errorf("build status list request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", statusListCredential, err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("could not close response body:", err)
		}
	}(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status list service returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read status list response: %w", err)
	}

	var sl statusListResponse
	if err := json.Unmarshal(body, &sl); err != nil {
		return "", fmt.Errorf("parse status list response: %w", err)
	}

	encoded := strings.TrimSpace(sl.List)
	if encoded == "" {
		return "", fmt.Errorf("status list response has no list field")
	}

	bitstring, err := decodeAndDecompressStatusList(encoded)
	if err != nil {
		return "", err
	}

	byteIdx := index / 8
	if int(byteIdx) >= len(bitstring) {
		return "", fmt.Errorf("index %d out of range for bitstring of %d bytes", index, len(bitstring))
	}
	// IETF Token Status List / XFSC statuslist-service convention: LSB-first —
	// bit N is at bit (N%8) of byte N/8.
	bitIdx := uint(index % 8)
	if bitstring[byteIdx]&(1<<bitIdx) != 0 {
		return "revoked", nil
	}
	return "active", nil
}

// decodeAndDecompressStatusList base64-decodes encoded (accepting both padded/
// unpadded and standard/url-safe alphabets, since deployments have been
// observed to disagree on this detail) and gzip-decompresses the result
// (the XFSC statuslist-service's only compression format).
func decodeAndDecompressStatusList(encoded string) ([]byte, error) {
	compressed, err := decodeStatusListBase64(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode status list: %w", err)
	}

	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("create gzip reader for bitstring: %w", err)
	}
	defer func(r io.ReadCloser) {
		if err := r.Close(); err != nil {
			log.Printf("close gzip reader for bitstring: %v", err)
		}
	}(r)
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("decompress gzip bitstring: %w", err)
	}
	return out, nil
}

// decodeStatusListBase64 tries the base64 variants seen across StatusList2021
// (base64url, unpadded) and the XFSC statuslist-service (standard, padded).
func decodeStatusListBase64(s string) ([]byte, error) {
	var lastErr error
	for _, enc := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		} else {
			lastErr = err
		}
	}
	return nil, lastErr
}

// RevokeStatus marks the contract as revoked in the status list (DCS-OR-C2PA-005).
func (p *OCMWStatusListPublisher) RevokeStatus(ctx context.Context, contractID string) (statusListURI string, err error) {
	if err := p.setRevoked(ctx, contractID); err != nil {
		return "", fmt.Errorf("revoke %s: %w", contractID, err)
	}
	return p.statusListURI(), nil
}

// ExtractCredentialStatusFields parses statusListCredential and statusListIndex
// from the credentialStatus object embedded in vcBytes.
func ExtractCredentialStatusFields(vcBytes []byte) (statusListCredential string, index uint32, ok bool) {
	var vcObj map[string]interface{}
	if err := json.Unmarshal(vcBytes, &vcObj); err != nil {
		return "", 0, false
	}
	csRaw, exists := vcObj["credentialStatus"]
	if !exists {
		return "", 0, false
	}
	cs, ok := csRaw.(map[string]interface{})
	if !ok {
		return "", 0, false
	}
	cred, _ := cs["statusListCredential"].(string)
	indexStr, _ := cs["statusListIndex"].(string)
	if cred == "" || indexStr == "" {
		return "", 0, false
	}
	idx, err := strconv.ParseUint(indexStr, 10, 32)
	if err != nil {
		return "", 0, false
	}
	return cred, uint32(idx), true
}

// ExtractStatusListURI extracts the credentialStatus.id from the VC JSON.
func ExtractStatusListURI(vcBytes []byte) string {
	var vcObj map[string]interface{}
	if err := json.Unmarshal(vcBytes, &vcObj); err != nil {
		return ""
	}
	credStatusRaw, ok := vcObj["credentialStatus"]
	if !ok {
		return ""
	}
	credStatusObj, ok := credStatusRaw.(map[string]interface{})
	if !ok {
		return ""
	}
	uri, _ := credStatusObj["id"].(string)
	return uri
}
