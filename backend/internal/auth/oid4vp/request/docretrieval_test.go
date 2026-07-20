package request

import (
	"testing"
	"time"
)

// TestBuildDocumentRetrievalJWTMatchesEUDIShape locks the request object to the
// EUDI walletdriven-signer / eudi-lib-jvm-rqes-csc-kt wire contract, so a real
// EUDI wallet can consume what the DCS publishes.
func TestBuildDocumentRetrievalJWTMatchesEUDIShape(t *testing.T) {
	signer := &captureSigner{}
	_, err := BuildDocumentRetrievalJWT(signer, DocRetrievalParams{
		ClientID:           "dcs-rp",
		ResponseURI:        "https://rp.example/cb",
		Nonce:              "nonce-1",
		ExpiresAt:          time.Now().UTC().Add(time.Minute),
		SignatureQualifier: "eu_eidas_aes",
		DocumentDigests:    []DocumentDigest{{Hash: "abc==", Label: "SignerOne"}},
		DocumentLocations:  []DocumentLocation{{URI: "https://rp.example/doc", Method: DocumentLocationMethod{Type: "public"}}},
	})
	if err != nil {
		t.Fatalf("BuildDocumentRetrievalJWT returned error: %v", err)
	}

	c := signer.claims
	if c["response_type"] != "sign_response" {
		t.Fatalf("response_type must be sign_response, got %v", c["response_type"])
	}
	if c["client_id_scheme"] != "x509_san_dns" {
		t.Fatalf("client_id_scheme must be x509_san_dns, got %v", c["client_id_scheme"])
	}
	if c["response_mode"] != "direct_post" {
		t.Fatalf("response_mode must be direct_post, got %v", c["response_mode"])
	}
	if c["signatureQualifier"] != "eu_eidas_aes" {
		t.Fatalf("signatureQualifier mismatch: %v", c["signatureQualifier"])
	}
	if c["hashAlgorithmOID"] != SHA256OID {
		t.Fatalf("hashAlgorithmOID mismatch: %v", c["hashAlgorithmOID"])
	}

	digests, ok := c["documentDigests"].([]any)
	if !ok || len(digests) != 1 {
		t.Fatalf("documentDigests missing or wrong shape: %T %v", c["documentDigests"], c["documentDigests"])
	}
	digest := digests[0].(map[string]any)
	if digest["hash"] != "abc==" || digest["label"] != "SignerOne" {
		t.Fatalf("document digest members mismatch: %v", digest)
	}

	locations, ok := c["documentLocations"].([]any)
	if !ok || len(locations) != 1 {
		t.Fatalf("documentLocations missing or wrong shape: %T %v", c["documentLocations"], c["documentLocations"])
	}
	location := locations[0].(map[string]any)
	if location["uri"] != "https://rp.example/doc" {
		t.Fatalf("document location uri mismatch: %v", location)
	}
	method, ok := location["method"].(map[string]any)
	if !ok || method["type"] != "public" {
		t.Fatalf("document location method mismatch: %v", location["method"])
	}
}

func TestBuildDocumentRetrievalJWTRejectsMismatchedDigestsAndLocations(t *testing.T) {
	signer := &captureSigner{}
	_, err := BuildDocumentRetrievalJWT(signer, DocRetrievalParams{
		ClientID:           "dcs-rp",
		ResponseURI:        "https://rp.example/cb",
		Nonce:              "nonce-1",
		ExpiresAt:          time.Now().UTC().Add(time.Minute),
		SignatureQualifier: "eu_eidas_aes",
		DocumentDigests:    []DocumentDigest{{Hash: "abc==", Label: "SignerOne"}},
		DocumentLocations:  nil,
	})
	if err == nil {
		t.Fatal("expected error when documentLocations is empty")
	}
}
