package base

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strings"
)

type PublicKeyJWK struct {
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
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

	pubKey, err := PublicKeyFromDID(didJSON)
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
	return didWebToHostname(id)
}

func didWebToHostname(did string) (string, error) {
	const prefix = "did:web:"
	if !strings.HasPrefix(did, prefix) {
		return "", fmt.Errorf("not a did:web identifier: %q", did)
	}

	rest := strings.TrimPrefix(did, prefix)

	// Alles nach dem ersten ":" wären Pfad-Komponenten (did:web:host:path:to:res),
	// die uns hier nicht interessieren - wir wollen nur den Host-Teil.
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
	return strings.ReplaceAll(hostEncoded, "%3A", ":"), nil
}

func PublicKeyFromDID(didJSON []byte) (*rsa.PublicKey, error) {
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
