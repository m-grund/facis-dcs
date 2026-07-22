package request

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SHA256OID is the ETSI/NIST object identifier for SHA-256, the hashAlgorithmOID
// the EUDI walletdriven-signer reference advertises for document digests.
const SHA256OID = "2.16.840.1.101.3.4.2.1"

// DocumentDigest names one document the wallet is asked to sign: the base64
// (standard, not URL) hash of the to-be-signed bytes and a human label. The
// field order and encoding mirror the EUDI reference (get_document_digest).
type DocumentDigest struct {
	Hash  string `json:"hash"`
	Label string `json:"label"`
}

// DocumentLocationMethod is how the wallet retrieves the document. "public" is
// an unauthenticated GET (the EUDI reference's only method).
type DocumentLocationMethod struct {
	Type string `json:"type"`
}

// DocumentLocation is where the wallet fetches one to-be-signed document from,
// matching the EUDI reference (get_document_location): {uri, method:{type}}.
type DocumentLocation struct {
	URI    string                 `json:"uri"`
	Method DocumentLocationMethod `json:"method"`
}

// DocRetrievalParams are the parameters of an OID4VP "Document Retrieval"
// request object (EUDI walletdriven-signer). The wallet fetches the documents
// from DocumentLocations, drives its own SCA+QTSP, and posts the signed
// documents back to ResponseURI. The DCS never signs.
type DocRetrievalParams struct {
	ClientID    string
	ResponseURI string
	Nonce       string
	ExpiresAt   time.Time
	// SignatureQualifier is the eIDAS level requested (CSC/rQES vocabulary,
	// e.g. "eu_eidas_aes" for an AES, "eu_eidas_qes" for a QES).
	SignatureQualifier string
	DocumentDigests    []DocumentDigest
	DocumentLocations  []DocumentLocation
}

// BuildDocumentRetrievalJWT creates the signed request object (JAR) a wallet
// consumes to sign the DCS's prepared documents. Its claim set matches the EUDI
// walletdriven-signer reference's generate_request_object: response_type
// "sign_response",
// client_id_scheme "x509_san_dns", response_mode "direct_post", and the
// camelCase documentDigests/documentLocations/hashAlgorithmOID members.
//
// It is NOT yet consumable by a real EUDI wallet. The reference wallet asserts
// three things this request does not satisfy: client_id_scheme "x509_san_dns"
// requires an x5c chain in the JAR header, which we do not attach; client_id
// must be a DNS name; and it must equal the response_uri host. Ours is the
// Hydra client id. Verified against eudi-lib-jvm-rqes-csc-kt — either attach
// x5c with a DNS-named client_id, or declare the pre-registered scheme
// honestly.
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
	qualifier := strings.TrimSpace(params.SignatureQualifier)
	if qualifier == "" {
		return "", fmt.Errorf("signatureQualifier is required")
	}
	if len(params.DocumentDigests) == 0 || len(params.DocumentDigests) != len(params.DocumentLocations) {
		return "", fmt.Errorf("documentDigests and documentLocations must be non-empty and parallel")
	}
	now := time.Now().UTC()
	exp := params.ExpiresAt.UTC()
	if !exp.After(now) {
		return "", fmt.Errorf("request expiry must be in the future")
	}

	digests := make([]any, 0, len(params.DocumentDigests))
	for _, d := range params.DocumentDigests {
		digests = append(digests, map[string]any{"hash": d.Hash, "label": d.Label})
	}
	locations := make([]any, 0, len(params.DocumentLocations))
	for _, l := range params.DocumentLocations {
		locations = append(locations, map[string]any{"uri": l.URI, "method": map[string]any{"type": l.Method.Type}})
	}

	claims := jwt.MapClaims{
		"response_type":      "sign_response",
		"client_id":          clientID,
		"client_id_scheme":   "x509_san_dns",
		"response_mode":      "direct_post",
		"response_uri":       responseURI,
		"nonce":              nonce,
		"signatureQualifier": qualifier,
		"documentDigests":    digests,
		"documentLocations":  locations,
		"hashAlgorithmOID":   SHA256OID,
		"iat":                now.Unix(),
		"exp":                exp.Unix(),
	}

	return signer.SignAuthorizationRequestJWT(claims)
}
