package compiler

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mustCreateTestCertChainAndKey(t *testing.T) (string, string, ed25519.PrivateKey) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "DCS-PDF-CORE test signer",
			Organization: []string{"DCS-PDF-CORE"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	chainPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))
	return keyPEM, chainPEM, priv
}

func TestLoadSigningMaterialFromEnv_InlinePEM(t *testing.T) {
	keyPEM, chainPEM, priv := mustCreateTestCertChainAndKey(t)
	env := map[string]string{
		envSignerKeyPEM: keyPEM,
		envX5ChainPEM:   chainPEM,
	}

	material, err := loadSigningMaterialFromEnv(func(k string) string { return env[k] }, os.ReadFile)
	if err != nil {
		t.Fatalf("loadSigningMaterialFromEnv() error = %v", err)
	}
	if len(material.certChainDER) != 1 {
		t.Fatalf("cert chain length = %d, want 1", len(material.certChainDER))
	}
	if string(material.signer) != string(priv) {
		t.Fatalf("signer key did not match configured key")
	}
}

func TestLoadSigningMaterialFromEnv_FilePEM(t *testing.T) {
	keyPEM, chainPEM, _ := mustCreateTestCertChainAndKey(t)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "signer-key.pem")
	chainPath := filepath.Join(dir, "x5chain.pem")
	if err := os.WriteFile(keyPath, []byte(keyPEM), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := os.WriteFile(chainPath, []byte(chainPEM), 0o644); err != nil {
		t.Fatalf("write chain: %v", err)
	}

	env := map[string]string{
		envSignerKeyPEMFile: keyPath,
		envX5ChainPEMFile:   chainPath,
	}
	material, err := loadSigningMaterialFromEnv(func(k string) string { return env[k] }, os.ReadFile)
	if err != nil {
		t.Fatalf("loadSigningMaterialFromEnv() error = %v", err)
	}
	if len(material.certChainDER) != 1 {
		t.Fatalf("cert chain length = %d, want 1", len(material.certChainDER))
	}
}

func TestLoadSigningMaterialFromEnv_RequireExternalMissing(t *testing.T) {
	env := map[string]string{
		envRequireExternalSigningMaterial: "true",
	}

	_, err := loadSigningMaterialFromEnv(func(k string) string { return env[k] }, os.ReadFile)
	if err == nil {
		t.Fatalf("expected error when external signing material is required")
	}
}
