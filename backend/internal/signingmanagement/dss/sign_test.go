package dss

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSignRunsTheRQESTwoCallFlow proves the client performs the CSC/rQES shape:
// getDataToSign computes the DTBS, the external HashSigner (backend PKCS#11 in
// the demonstrator, a wallet-unlocked QTSP in prod) signs it, and signDocument
// embeds the signature — the DSS server never sees the private key. Verified for
// both PAdES and JAdES.
func TestSignRunsTheRQESTwoCallFlow(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fmt   Format
		level string
	}{
		{"pades", FormatPAdES, "PAdES_BASELINE_T"},
		{"jades", FormatJAdES, "JAdES_BASELINE_B"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dtbs := []byte("data-to-be-signed-" + tc.name)
			signed := []byte("signed-document-" + tc.name)
			var sawLevel, sawSignatureValue, sawAlgorithm string
			var sawFormatPackaging string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				raw, _ := io.ReadAll(r.Body)
				require.NoError(t, json.Unmarshal(raw, &body))
				params, _ := body["parameters"].(map[string]any)
				sawLevel, _ = params["signatureLevel"].(string)
				sawFormatPackaging, _ = params["signaturePackaging"].(string)
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/getDataToSign"):
					_ = json.NewEncoder(w).Encode(map[string]string{"bytes": base64.StdEncoding.EncodeToString(dtbs)})
				case strings.HasSuffix(r.URL.Path, "/signDocument"):
					sv, _ := body["signatureValue"].(map[string]any)
					sawSignatureValue, _ = sv["value"].(string)
					sawAlgorithm, _ = sv["algorithm"].(string)
					_ = json.NewEncoder(w).Encode(map[string]string{"bytes": base64.StdEncoding.EncodeToString(signed)})
				default:
					t.Fatalf("unexpected DSS path %q", r.URL.Path)
				}
			}))
			defer srv.Close()

			var signedDTBS []byte
			signer := func(_ context.Context, data []byte) ([]byte, string, error) {
				signedDTBS = data
				return []byte("sigval"), "ECDSA_SHA256", nil
			}

			out, err := New(srv.URL).Sign(context.Background(), []byte("document"), "contract."+tc.name,
				SignParams{Format: tc.fmt, SignatureLevel: tc.level, SigningCertificate: "cert", SignatureFieldID: "SignerOne"}, signer)
			require.NoError(t, err)

			require.Equal(t, signed, out, "returns the DSS-embedded signed document")
			require.Equal(t, dtbs, signedDTBS, "the external signer signs exactly the DSS data-to-be-signed")
			require.Equal(t, tc.level, sawLevel, "the requested signature level reaches DSS")
			require.Equal(t, base64.StdEncoding.EncodeToString([]byte("sigval")), sawSignatureValue)
			require.Equal(t, "ECDSA_SHA256", sawAlgorithm)
			require.NotEmpty(t, sawFormatPackaging, "a signaturePackaging is set for the format")
		})
	}
}

func TestSignRequiresBaseURL(t *testing.T) {
	_, err := New("").Sign(context.Background(), []byte("doc"), "n", SignParams{Format: FormatPAdES}, nil)
	require.Error(t, err)
}
