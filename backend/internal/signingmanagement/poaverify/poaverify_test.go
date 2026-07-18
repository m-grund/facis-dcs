package poaverify

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp"
)

func jwtWith(payload map[string]any) string {
	header, _ := json.Marshal(map[string]any{"alg": "ES256", "typ": "dc+sd-jwt"})
	body, _ := json.Marshal(payload)
	enc := base64.RawURLEncoding.EncodeToString
	return enc(header) + "." + enc(body) + ".sig"
}

func presentation(issuerPayload map[string]any) string {
	return jwtWith(issuerPayload) + "~" + jwtWith(map[string]any{"nonce": "n"})
}

func TestVerifyRejectsMalformedToken(t *testing.T) {
	if _, _, err := Verify("not-a-presentation"); err == nil {
		t.Fatal("expected error for token without a KB-JWT segment")
	}
}

func TestVerifyRejectsNonPoACredential(t *testing.T) {
	_, _, err := Verify(presentation(map[string]any{"vct": "urn:example:pid", "sub": "did:jwk:x"}))
	if err == nil || !strings.Contains(err.Error(), "not a PoA") {
		t.Fatalf("expected non-PoA rejection, got %v", err)
	}
}

func TestVerifyAcceptsPoAVCTThroughToHolderBinding(t *testing.T) {
	// A PoA VCT passes the credential-type gate and proceeds to holder-binding
	// verification, which fails here only because the crafted credential carries
	// no cnf.jwk — proving the vct check is not what rejects a genuine PoA.
	_, _, err := Verify(presentation(map[string]any{"vct": oid4vp.PoAVCT, "sub": "did:jwk:x"}))
	if err == nil || strings.Contains(err.Error(), "not a PoA") {
		t.Fatalf("PoA vct must pass the type gate, got %v", err)
	}
}
