package request

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// DocumentDigest names one document the wallet is asked to sign: a human label
// and the base64url hash of the to-be-signed bytes (OID4VP "Document Retrieval").
type DocumentDigest struct {
	Label string `json:"label"`
	Hash  string `json:"hash"`
}

// DocRetrievalParams are the OID4VP Document-Retrieval request-object parameters:
// the wallet fetches the documents from Locations, drives its own SCA+QTSP, and
// posts the signed documents back to ResponseURI. The DCS never signs.
type DocRetrievalParams struct {
	ClientID        string
	ResponseURI     string
	Nonce           string
	State           string
	ExpiresAt       time.Time
	DocumentDigests []DocumentDigest
	// DocumentLocations are the URLs the wallet fetches the to-be-signed
	// documents from (parallel to DocumentDigests).
	DocumentLocations []string
	// DCQLQuery requests the PoA/PID presentation the QTSP's SAM needs to
	// authorize the signature (presented alongside the signing consent).
	DCQLQuery any
}

// BuildDocumentRetrievalJWT creates the signed request object a wallet consumes
// to sign the DCS's prepared documents. client_id_scheme is x509_san_dns so a
// production EUDI wallet can authenticate the request against the DCS's
// DNS-bound certificate; a bespoke shape here would break that swap.
func BuildDocumentRetrievalJWT(signer Signer, params DocRetrievalParams) (string, error) {
	if signer == nil {
		return "", fmt.Errorf("request signer is not configured")
	}
	clientID := strings.TrimSpace(params.ClientID)
	if clientID == "" {
		return "", fmt.Errorf("client_id is required")
	}
	responseURI := strings.TrimSpace(params.ResponseURI)
	if responseURI == "" {
		return "", fmt.Errorf("response_uri is required")
	}
	nonce := strings.TrimSpace(params.Nonce)
	if nonce == "" {
		return "", fmt.Errorf("nonce is required")
	}
	if len(params.DocumentDigests) == 0 || len(params.DocumentDigests) != len(params.DocumentLocations) {
		return "", fmt.Errorf("document_digests and document_locations must be non-empty and parallel")
	}
	now := time.Now().UTC()
	exp := params.ExpiresAt.UTC()
	if !exp.After(now) {
		return "", fmt.Errorf("request expiry must be in the future")
	}

	digests := make([]any, 0, len(params.DocumentDigests))
	for _, d := range params.DocumentDigests {
		digests = append(digests, map[string]any{"label": d.Label, "hash": d.Hash})
	}

	claims := jwt.MapClaims{
		"iss":                clientID,
		"client_id":          clientID,
		"client_id_scheme":   "x509_san_dns",
		"response_type":      "vp_token",
		"response_mode":      "direct_post",
		"response_uri":       responseURI,
		"nonce":              nonce,
		"state":              strings.TrimSpace(params.State),
		"document_digests":   digests,
		"document_locations": params.DocumentLocations,
		"iat":                now.Unix(),
		"exp":                exp.Unix(),
	}
	if params.DCQLQuery != nil {
		dcqlJSON, err := json.Marshal(params.DCQLQuery)
		if err != nil {
			return "", fmt.Errorf("marshal dcql_query: %w", err)
		}
		var dcql any
		if err := json.Unmarshal(dcqlJSON, &dcql); err != nil {
			return "", fmt.Errorf("decode dcql_query: %w", err)
		}
		claims["dcql_query"] = dcql
	}

	return signer.SignAuthorizationRequestJWT(claims)
}
