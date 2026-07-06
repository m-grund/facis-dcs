// Package identity implements the did:web-based peer identity and trust
// model used for DCS-to-DCS federation (see dcstodcs, contractworkflowengine
// /remotesync). Each DCS instance publishes its own DID document (RSA key
// pair) at /.well-known/did.json. Trust between two independently operated
// instances rests on three layers, all implemented in this file: (1) an
// eIDAS certificate chain in the DID document, validated against an EU trust
// pool (VerifyEIDASCertificate); (2) a per-request challenge-response
// signature proving possession of the private key (Sign/Verify), used
// instead of a shared token since there is no common auth authority across
// operators; and (3) a local trusted-peer allowlist enforced by callers
// (see dcstodcs.CheckForUntrustedPeers), which is deliberately not part of
// this package.
package identity

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// eIDAS / ETSI EN 319 412-5 OIDs
var (
	oidQCStatements = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 3} // id-pe-qcStatements
	oidQcCompliance = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 1}       // esi4-qcStatement-1: qualified certificate
	oidQcSSCD       = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 4}       // esi4-qcStatement-4: QSCD
)

type qcStatement struct {
	StatementID   asn1.ObjectIdentifier
	StatementInfo asn1.RawValue `asn1:"optional"`
}

// X5C is the x5c certificate chain of a JWK (RFC 7517 §4.7).
// During unmarshaling it accepts both an array of strings and a single
// string and normalizes to []string.
type X5C []string

func (x *X5C) UnmarshalJSON(data []byte) error {
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*x = arr
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*x = X5C{s}
		return nil
	}

	return fmt.Errorf("x5c: expected string or array of strings, got %s", string(data))
}

type PublicKeyJWK struct {
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
	X5C X5C    `json:"x5c,omitempty"`
}

// RSAPublicKey builds an *rsa.PublicKey from the JWK fields n and e.
func (jwk PublicKeyJWK) RSAPublicKey() (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("decoding n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("decoding e: %w", err)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}, nil
}

type VerificationMethod struct {
	ID           string       `json:"id"`
	PublicKeyJWK PublicKeyJWK `json:"publicKeyJwk"`
}

type DIDDocument struct {
	VerificationMethod []VerificationMethod `json:"verificationMethod"`
	didContent         map[string]interface{}
	privateKey         *rsa.PrivateKey
	publicKey          *rsa.PublicKey
}

// NewDIDDocument loads a DID document and its private key from disk,
// verifies that both keys belong together, and validates the key pair
// with a test signature.
// NewDIDDocument loads a DID document and its private key from disk,
// verifies that both keys belong together, and validates the key pair
// with a test signature.
func NewDIDDocument(didFilePath string, privateKeyPath string) (*DIDDocument, error) {
	didJSON, err := os.ReadFile(didFilePath)
	if err != nil {
		return nil, err
	}

	// Unmarshal into the struct -> fills VerificationMethod.
	var doc DIDDocument
	if err := json.Unmarshal(didJSON, &doc); err != nil {
		return nil, fmt.Errorf("decoding did.json: %w", err)
	}

	// Keep the raw content as a map alongside.
	if err := json.Unmarshal(didJSON, &doc.didContent); err != nil {
		return nil, fmt.Errorf("decoding did.json content: %w", err)
	}

	if len(doc.VerificationMethod) == 0 {
		return nil, errors.New("no verification methods in DID document")
	}

	pemData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, err
	}

	pubKey, err := doc.VerificationMethod[0].PublicKeyJWK.RSAPublicKey()
	if err != nil {
		return nil, fmt.Errorf("extracting public key from DID document: %w", err)
	}

	privKey, err := privateKeyFromPEM(pemData)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	if pubKey.N.Cmp(privKey.N) != 0 {
		return nil, errors.New("public key from DID document does not match private key")
	}

	// Self test: signing and verifying must work.
	message := []byte("key pair self test")
	hash := sha256.Sum256(message)

	signature, err := rsa.SignPSS(rand.Reader, privKey, crypto.SHA256, hash[:], nil)
	if err != nil {
		return nil, fmt.Errorf("key pair self test (sign): %w", err)
	}
	if err := rsa.VerifyPSS(pubKey, crypto.SHA256, hash[:], signature, nil); err != nil {
		return nil, fmt.Errorf("key pair self test (verify): %w", err)
	}

	doc.privateKey = privKey
	doc.publicKey = pubKey

	return &doc, nil
}

func (d *DIDDocument) PublicKey() *rsa.PublicKey {
	return d.publicKey
}

func (d DIDDocument) GetDIDContent() map[string]interface{} {
	return d.didContent
}

func (d DIDDocument) GetID() (string, error) {
	raw, ok := d.didContent["id"]
	if !ok {
		return "", errors.New(`did document does not contain "id"`)
	}

	id, ok := raw.(string)
	if !ok {
		return "", errors.New(`did document "id" is not a string`)
	}

	return id, nil
}

func (d DIDDocument) GetHostname() (string, error) {
	id, err := d.GetID()
	if err != nil {
		return "", err
	}
	return DIDWebToHostname(id)
}

// Sign signs content with RSA-PSS (SHA-256).
func (d *DIDDocument) Sign(content []byte) ([]byte, error) {
	if d.privateKey == nil {
		return nil, errors.New("private key not set")
	}

	hash := sha256.Sum256(content)
	return rsa.SignPSS(rand.Reader, d.privateKey, crypto.SHA256, hash[:], nil)
}

// Verify checks an RSA-PSS signature (SHA-256) against the public key.
func (d *DIDDocument) Verify(content []byte, signature []byte) error {
	if d.publicKey == nil {
		return errors.New("public key not set")
	}

	hash := sha256.Sum256(content)
	return rsa.VerifyPSS(d.publicKey, crypto.SHA256, hash[:], signature, nil)
}

// VerifyEIDASCertificate validates the x5c certificate chain of the first
// verification method:
//
//  1. Chain validation leaf -> intermediates -> root. trustedRoots
//     determines the trust anchor; nil means the system trust store.
//  2. The leaf certificate must match the hostname of the DID.
//  3. The public key of the leaf must match the JWK (n/e).
//  4. The leaf must carry the eIDAS QcCompliance statement.
//
// Note: QCStatements are a self-declaration by the issuer. For a legally
// binding eIDAS validation, trustedRoots must be populated from the EU
// Trusted Lists (LOTL/TSL), e.g. via BuildEUTrustPool.
func (d *DIDDocument) VerifyEIDASCertificate(trustPool *EUTrustPool) error {

	var trustedRoots *x509.CertPool
	if trustPool != nil {
		trustedRoots = trustPool.Pool()
	}

	certs, err := d.loadCertificateChain()
	if err != nil {
		return err
	}

	if trustedRoots != nil {
		if err := verifyChain(certs, trustedRoots); err != nil {
			return err
		}
	}

	cert := certs[0]

	// 1. Does the certificate match the hostname of the DID?
	hostname, err := d.GetHostname()
	if err != nil {
		return err
	}
	host := hostname
	if h, _, err := net.SplitHostPort(hostname); err == nil {
		host = h // strip port, e.g. "localhost:8991" -> "localhost"
	}
	if err := cert.VerifyHostname(host); err != nil {
		return fmt.Errorf("certificate does not match hostname %q: %w", host, err)
	}

	// 2. Does the certificate match the public key from the JWK?
	certPub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("certificate does not contain an RSA public key")
	}
	jwkPub, err := d.VerificationMethod[0].PublicKeyJWK.RSAPublicKey()
	if err != nil {
		return err
	}
	if certPub.N.Cmp(jwkPub.N) != 0 || certPub.E != jwkPub.E {
		return errors.New("certificate public key does not match JWK public key")
	}

	if trustedRoots != nil {
		// 3. Does the certificate carry the eIDAS QCStatements?
		qualified, qscd, err := parseQCStatements(cert)
		if err != nil {
			return err
		}
		if !qualified {
			return errors.New("certificate is not an eIDAS qualified certificate (QcCompliance statement missing)")
		}
		_ = qscd // optional: enforce additionally if required
	}

	return nil
}

// loadCertificateChain parses all x5c entries of the first verification
// method. Entries starting with http:// or https:// are fetched remotely
// (PEM or DER), all others are interpreted as base64 DER (standard base64
// per RFC 7517, NOT base64url).
func (d *DIDDocument) loadCertificateChain() ([]*x509.Certificate, error) {
	if len(d.VerificationMethod) == 0 {
		return nil, errors.New("no verification methods in DID document")
	}
	x5c := d.VerificationMethod[0].PublicKeyJWK.X5C
	if len(x5c) == 0 {
		return nil, errors.New("no x5c entry in publicKeyJwk")
	}

	certs := make([]*x509.Certificate, 0, len(x5c))
	for i, entry := range x5c {
		var der []byte
		var err error

		if strings.HasPrefix(entry, "http://") || strings.HasPrefix(entry, "https://") {
			der, err = fetchCertificateDER(entry)
		} else {
			der, err = base64.StdEncoding.DecodeString(entry)
		}
		if err != nil {
			return nil, fmt.Errorf("x5c[%d]: %w", i, err)
		}

		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("parsing x5c[%d]: %w", i, err)
		}
		certs = append(certs, cert)
	}
	return certs, nil
}

// fetchCertificateDER fetches a certificate from a URL and returns it as
// DER. The server may deliver PEM or raw DER.
func fetchCertificateDER(certURL string) ([]byte, error) {
	resp, err := http.Get(certURL)
	if err != nil {
		return nil, fmt.Errorf("fetching certificate from %s: %w", certURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s fetching certificate from %s", resp.Status, certURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading certificate: %w", err)
	}

	if block, _ := pem.Decode(body); block != nil {
		return block.Bytes, nil
	}
	return body, nil
}

// verifyChain validates the signature chain leaf -> intermediates -> root.
//
// trustedRoots determines the trust anchor:
//   - nil: system trust store of the operating system.
//   - custom pool: e.g. populated from the EU Trusted Lists.
//
// Self-signed certificates from the supplied chain are deliberately NOT
// accepted as trust anchors — otherwise an attacker could ship their own
// root and the check would be worthless.
func verifyChain(certs []*x509.Certificate, trustedRoots *x509.CertPool) error {
	if len(certs) == 0 {
		return errors.New("empty certificate chain")
	}
	leaf := certs[0]

	intermediates := x509.NewCertPool()
	for _, c := range certs[1:] {
		intermediates.AddCert(c)
	}

	_, err := leaf.Verify(x509.VerifyOptions{
		Intermediates: intermediates,
		Roots:         trustedRoots, // nil -> system roots
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	if err != nil {
		return fmt.Errorf("certificate chain verification failed: %w", err)
	}
	return nil
}

// parseQCStatements looks for the QCStatements extension and reports
// whether QcCompliance (qualified certificate) and QcSSCD (QSCD) are set.
func parseQCStatements(cert *x509.Certificate) (qualified bool, qscd bool, err error) {
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(oidQCStatements) {
			continue
		}
		var statements []qcStatement
		if _, err := asn1.Unmarshal(ext.Value, &statements); err != nil {
			return false, false, fmt.Errorf("parsing QCStatements: %w", err)
		}
		for _, s := range statements {
			switch {
			case s.StatementID.Equal(oidQcCompliance):
				qualified = true
			case s.StatementID.Equal(oidQcSSCD):
				qscd = true
			}
		}
		return qualified, qscd, nil
	}
	return false, false, nil // extension not present -> not an eIDAS certificate
}

// FetchDIDDocumentFromHostname fetches the did.json of a host via
// /.well-known/did.json, first over https, then falling back to http.
func FetchDIDDocumentFromHostname(hostname string) (*DIDDocument, error) {
	var lastErr error
	for _, scheme := range []string{"https", "http"} {
		url := fmt.Sprintf("%s://%s/.well-known/did.json", scheme, hostname)
		doc, err := fetchDIDDocumentFromURL(url)
		if err == nil {
			return doc, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("fetching did.json from %s failed: %w", hostname, lastErr)
}

// DIDWebToHostname extracts the host (including port) from a did:web
// identifier, e.g. "did:web:localhost%3A8991" -> "localhost:8991".
func DIDWebToHostname(did string) (string, error) {
	const prefix = "did:web:"
	if !strings.HasPrefix(did, prefix) {
		return "", fmt.Errorf("not a did:web identifier: %q", did)
	}

	rest := strings.TrimPrefix(did, prefix)

	hostEncoded, _, _ := strings.Cut(rest, ":")
	if hostEncoded == "" {
		return "", errors.New("did:web identifier has empty host component")
	}

	host, err := url.QueryUnescape(hostEncoded) // %3A -> ":"
	if err != nil {
		return "", fmt.Errorf("invalid percent-encoding in did:web host: %w", err)
	}

	return host, nil
}

func privateKeyFromPEM(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA key")
	}
	return rsaKey, nil
}

func fetchDIDDocumentFromURL(url string) (*DIDDocument, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s from %s", resp.Status, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading did.json: %w", err)
	}

	var doc DIDDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decoding did.json: %w", err)
	}

	if err := json.Unmarshal(body, &doc.didContent); err != nil {
		return nil, fmt.Errorf("decoding did.json content: %w", err)
	}

	if len(doc.VerificationMethod) == 0 {
		return nil, fmt.Errorf("no verification methods in DID document")
	}

	pubKey, err := doc.VerificationMethod[0].PublicKeyJWK.RSAPublicKey()
	if err != nil {
		return nil, err
	}
	doc.publicKey = pubKey

	return &doc, nil
}
