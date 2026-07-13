package sign

import (
	"encoding/hex"
	"testing"
)

// TestSignatureAlgorithmMaxLengthHint covers every algorithm family the
// function recognizes, guarding against the regression this function was
// extracted to fix: a Go switch with one case per algorithm name sharing a
// body only runs that body for the last case in the group (no implicit
// fallthrough), so e.g. "SHA1-RSA" and "ECDSA-SHA1" silently added zero
// extra headroom instead of the intended hex.EncodedLen(128).
func TestSignatureAlgorithmMaxLengthHint(t *testing.T) {
	tests := []struct {
		sigAlg string
		want   uint32
	}{
		{"SHA1-RSA", uint32(hex.EncodedLen(128))},
		{"ECDSA-SHA1", uint32(hex.EncodedLen(128))},
		{"DSA-SHA1", uint32(hex.EncodedLen(128))},
		{"SHA256-RSA", uint32(hex.EncodedLen(256))},
		{"ECDSA-SHA256", uint32(hex.EncodedLen(256))},
		{"DSA-SHA256", uint32(hex.EncodedLen(256))},
		{"SHA384-RSA", uint32(hex.EncodedLen(384))},
		{"ECDSA-SHA384", uint32(hex.EncodedLen(384))},
		{"SHA512-RSA", uint32(hex.EncodedLen(512))},
		{"ECDSA-SHA512", uint32(hex.EncodedLen(512))},
		{"unknown-alg", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.sigAlg, func(t *testing.T) {
			got := signatureAlgorithmMaxLengthHint(tt.sigAlg)
			if got != tt.want {
				t.Errorf("signatureAlgorithmMaxLengthHint(%q) = %d, want %d", tt.sigAlg, got, tt.want)
			}
		})
	}
}
