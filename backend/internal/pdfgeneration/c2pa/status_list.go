package c2pa

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"crypto/sha256"
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

// deriveIndex returns a stable bitstring index for contractID.
// Uses the first 4 bytes of SHA-256(contractID) modulo listSize so the
// index is deterministic without requiring a per-contract allocation table.
func deriveIndex(contractID string) uint32 {
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
func (p *OCMWStatusListPublisher) setRevoked(ctx context.Context, contractID string) error {
	if p.ServiceURL == "" {
		// No service configured — silently skip (non-blocking for offline environments).
		return nil
	}
	index := deriveIndex(contractID)
	url := fmt.Sprintf("%s/v1/tenants/%s/status/revoke/%d/%d", p.ServiceURL, p.TenantID, defaultListID, index)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(nil))
	if err != nil {
		return fmt.Errorf("build revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

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
// Active/draft/amended states are the default (not revoked) and require no HTTP call.
func (p *OCMWStatusListPublisher) PublishStatus(
	ctx context.Context,
	contractID string,
	status string,
	reason string,
	effectiveAt time.Time,
) (statusListURI string, err error) {
	switch status {
	case "terminated", "expired", "replaced", "suspended":
		if err := p.setRevoked(ctx, contractID); err != nil {
			return "", fmt.Errorf("publish status %s for %s: %w", status, contractID, err)
		}
	}
	// active, draft, approved, amended — default state = not revoked, no action required.
	return p.statusListURI(), nil
}

// RevokeStatus marks the contract as revoked in the status list (DCS-OR-C2PA-005).
func (p *OCMWStatusListPublisher) RevokeStatus(ctx context.Context, contractID string) (statusListURI string, err error) {
	if err := p.setRevoked(ctx, contractID); err != nil {
		return "", fmt.Errorf("revoke %s: %w", contractID, err)
	}
	return p.statusListURI(), nil
}
