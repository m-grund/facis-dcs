package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"
)

// startTestSigningServer generates a P-256 key + self-signed leaf, writes the
// leaf as an x5chain PEM, starts an HTTP server mirroring the backend's
// POST /internal/c2pa/sign (deterministic ES256 over the posted Sig_structure),
// and points the pdf-core signing env at it. pdf-core holds no key material and
// delegates every C2PA signature to this endpoint (DCS-IR-HI-01).
func startTestSigningServer(dir string) (*httptest.Server, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "DCS-PDF-CORE test c2pa signer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return nil, err
	}
	chainPath := filepath.Join(dir, "x5chain.pem")
	if err := os.WriteFile(chainPath, certPEM(der), 0o644); err != nil {
		return nil, err
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SigStructure string `json:"sig_structure"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sigStructure, err := base64.StdEncoding.DecodeString(req.SigStructure)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sig := deterministicES256(key, sigStructure)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"signature": base64.StdEncoding.EncodeToString(sig),
		})
	}))

	_ = os.Setenv("DCS_PDF_CORE_C2PA_SIGNING_ENDPOINT", srv.URL)
	_ = os.Setenv("DCS_PDF_CORE_C2PA_X5CHAIN_PEM_FILE", chainPath)
	return srv, nil
}

func setupTestSigning() (*httptest.Server, error) {
	dir, err := os.MkdirTemp("", "dcs-c2pa-test")
	if err != nil {
		return nil, err
	}
	return startTestSigningServer(dir)
}

func certPEM(der []byte) []byte {
	const header = "-----BEGIN CERTIFICATE-----\n"
	const footer = "-----END CERTIFICATE-----\n"
	b := base64.StdEncoding.EncodeToString(der)
	var out []byte
	out = append(out, header...)
	for len(b) > 64 {
		out = append(out, b[:64]...)
		out = append(out, '\n')
		b = b[64:]
	}
	out = append(out, b...)
	out = append(out, '\n')
	out = append(out, footer...)
	return out
}

// deterministicES256 signs message with RFC 6979 deterministic ECDSA over P-256
// and returns the 64-byte r||s encoding, so repeated compilations are byte-stable.
func deterministicES256(priv *ecdsa.PrivateKey, message []byte) []byte {
	digest := sha256.Sum256(message)
	r, s := signRFC6979(priv, digest[:])
	out := make([]byte, 64)
	r.FillBytes(out[:32])
	s.FillBytes(out[32:])
	return out
}

func signRFC6979(priv *ecdsa.PrivateKey, hash []byte) (*big.Int, *big.Int) {
	n := priv.Curve.Params().N
	d := priv.D
	e := hashToInt(hash, priv.Curve)
	for k := rfc6979Nonce(n, d, hash); ; k = k.Add(k, big.NewInt(1)) {
		if k.Sign() <= 0 || k.Cmp(n) >= 0 {
			continue
		}
		rx, _ := priv.Curve.ScalarBaseMult(k.Bytes())
		r := new(big.Int).Mod(rx, n)
		if r.Sign() == 0 {
			continue
		}
		kInv := new(big.Int).ModInverse(k, n)
		s := new(big.Int).Mul(r, d)
		s.Add(s, e)
		s.Mul(s, kInv)
		s.Mod(s, n)
		if s.Sign() == 0 {
			continue
		}
		return r, s
	}
}

func hashToInt(hash []byte, curve elliptic.Curve) *big.Int {
	orderBits := curve.Params().N.BitLen()
	orderBytes := (orderBits + 7) / 8
	if len(hash) > orderBytes {
		hash = hash[:orderBytes]
	}
	ret := new(big.Int).SetBytes(hash)
	if excess := len(hash)*8 - orderBits; excess > 0 {
		ret.Rsh(ret, uint(excess))
	}
	return ret
}

func rfc6979Nonce(n, d *big.Int, hash []byte) *big.Int {
	seed := append(d.Bytes(), hash...)
	sum := sha256.Sum256(seed)
	return new(big.Int).Mod(new(big.Int).SetBytes(sum[:]), n)
}
