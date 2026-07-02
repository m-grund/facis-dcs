package base

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
	oidQcCompliance = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 1}       // esi4-qcStatement-1: qualifiziertes Zertifikat
	oidQcSSCD       = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 4}       // esi4-qcStatement-4: QSCD
)

type qcStatement struct {
	StatementID   asn1.ObjectIdentifier
	StatementInfo asn1.RawValue `asn1:"optional"`
}

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

func NewDIDDocument(didFilePath string, privateKeyPath string) (*DIDDocument, error) {
	didJSON, err := os.ReadFile(didFilePath)
	if err != nil {
		return nil, err
	}

	var didContent map[string]interface{}
	err = json.Unmarshal(didJSON, &didContent)
	if err != nil {
		return nil, err
	}

	pemData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, err
	}

	pubKey, err := publicKeyFromDID(didJSON)
	if err != nil {
		panic(err)
	}

	privKey, err := privateKeyFromPEM(pemData)
	if err != nil {
		panic(err)
	}

	if pubKey.N.Cmp(privKey.PublicKey.N) != 0 {
		panic("Public key from DID does not match private key!")
	}
	fmt.Println("Keys match ✓")

	message := []byte("hello world")
	hash := sha256.Sum256(message)

	signature, err := rsa.SignPSS(rand.Reader, privKey, crypto.SHA256, hash[:], nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Signature: %s\n", base64.RawURLEncoding.EncodeToString(signature))

	err = rsa.VerifyPSS(pubKey, crypto.SHA256, hash[:], signature, nil)
	if err != nil {
		panic("verification failed: " + err.Error())
	}
	fmt.Println("Signature verified ✓")

	return &DIDDocument{
		didContent: didContent,
		privateKey: privKey,
		publicKey:  pubKey,
	}, nil
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

	return raw.(string), nil
}

func (d DIDDocument) GetHostname() (string, error) {
	id, err := d.GetID()
	if err != nil {
		return "", err
	}
	return DIDWebToHostname(id)
}

func (d *DIDDocument) Sign(content []byte) ([]byte, error) {
	if d.privateKey == nil {
		return nil, errors.New("private key not set")
	}

	hash := sha256.Sum256(content)
	return rsa.SignPSS(rand.Reader, d.privateKey, crypto.SHA256, hash[:], nil)
}

func (d *DIDDocument) Verify(content []byte, signature []byte) error {
	if d.publicKey == nil {
		return errors.New("public key not set")
	}

	hash := sha256.Sum256(content)
	return rsa.VerifyPSS(d.publicKey, crypto.SHA256, hash[:], signature, nil)
}

func (d *DIDDocument) VerifyEIDASCertificate() error {
	cert, err := d.loadCertificate()
	if err != nil {
		return err
	}

	hostname, err := d.GetHostname()
	if err != nil {
		return err
	}
	host := hostname
	if h, _, err := net.SplitHostPort(hostname); err == nil {
		host = h
	}
	if err := cert.VerifyHostname(host); err != nil {
		return fmt.Errorf("certificate does not match hostname %q: %w", host, err)
	}

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

	qualified, qscd, err := parseQCStatements(cert)
	if err != nil {
		return err
	}
	if !qualified {
		return errors.New("certificate is not an eIDAS qualified certificate (QcCompliance statement missing)")
	}
	_ = qscd

	return nil
}

func (d *DIDDocument) loadCertificate() (*x509.Certificate, error) {
	if len(d.VerificationMethod) == 0 {
		return nil, errors.New("no verification methods in DID document")
	}
	x5c := d.VerificationMethod[0].PublicKeyJWK.X5C
	if len(x5c) == 0 {
		return nil, errors.New("no x5c entry in publicKeyJwk")
	}
	entry := x5c[0]

	var der []byte
	if strings.HasPrefix(entry, "http://") || strings.HasPrefix(entry, "https://") {
		resp, err := http.Get(entry)
		if err != nil {
			return nil, fmt.Errorf("fetching certificate from %s: %w", entry, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status %s fetching certificate from %s", resp.Status, entry)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading certificate: %w", err)
		}

		if block, _ := pem.Decode(body); block != nil {
			der = block.Bytes
		} else {
			der = body
		}
	} else {
		// x5c ist standard Base64 (RFC 7517), NICHT base64url
		var err error
		der, err = base64.StdEncoding.DecodeString(entry)
		if err != nil {
			return nil, fmt.Errorf("decoding x5c certificate: %w", err)
		}
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}
	return cert, nil
}

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
	return false, false, nil // Extension nicht vorhanden -> kein eIDAS-Zertifikat
}

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

func HostnameToDIDWeb(host string) string {
	return "did:web:" + strings.ReplaceAll(host, ":", "%3A")
}

func publicKeyFromDID(didJSON []byte) (*rsa.PublicKey, error) {
	var doc DIDDocument
	if err := json.Unmarshal(didJSON, &doc); err != nil {
		return nil, err
	}

	jwk := doc.VerificationMethod[0].PublicKeyJWK

	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("decoding n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("decoding e: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())

	return &rsa.PublicKey{N: n, E: e}, nil
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
	defer resp.Body.Close()

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

// UUID v4
func generateUUID() (string, error) {
	uuid := make([]byte, 16)
	_, err := rand.Read(uuid)
	if err != nil {
		return "", err
	}

	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}

func GenerateID() (*string, error) {
	uuid, err := generateUUID()
	if err != nil {
		return nil, err
	}

	return &uuid, nil
}
