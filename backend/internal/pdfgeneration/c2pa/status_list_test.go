package c2pa

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeStatusListVC builds a minimal StatusList2021Credential JSON with a
// bitstring of bitstringLen bytes where the bit at setIndex is 1 if revoked=true.
func makeStatusListVC(bitstringLen int, setIndex uint32, revoked bool) []byte {
	bitstring := make([]byte, bitstringLen)
	if revoked {
		byteIdx := setIndex / 8
		bitIdx := uint(7 - (setIndex % 8))
		bitstring[byteIdx] |= 1 << bitIdx
	}

	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, _ = w.Write(bitstring)
	_ = w.Close()
	encoded := base64.RawURLEncoding.EncodeToString(buf.Bytes())

	vc := map[string]interface{}{
		"@context": []string{"https://www.w3.org/2018/credentials/v1"},
		"type":     []string{"VerifiableCredential", "StatusList2021Credential"},
		"credentialSubject": map[string]interface{}{
			"type":          "StatusList2021",
			"statusPurpose": "revocation",
			"encodedList":   encoded,
		},
	}
	b, _ := json.Marshal(vc)
	return b
}

// TestOCMWStatusListPublisher_PublishStatus_TerminalStatesCallRevoke verifies
// that all terminal states — including the uppercase forms emitted by the CWE
// (DCS-OR-C2PA-005 Gap 1) — trigger a revocation POST to the status list service.
func TestOCMWStatusListPublisher_PublishStatus_TerminalStatesCallRevoke(t *testing.T) {
	for _, state := range []string{
		"terminated", "TERMINATED",
		"expired", "EXPIRED",
		"replaced", "REPLACED",
		"suspended", "SUSPENDED",
	} {
		t.Run(state, func(t *testing.T) {
			revokeCalled := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/revoke/") {
					revokeCalled = true
					w.WriteHeader(http.StatusOK)
					_, err := fmt.Fprintf(w, `{"tenantId":"default","listId":1,"index":0,"status":"revoked"}`)
					if err != nil {
						log.Println("could not write response:", err)
					}
					return
				}
				http.NotFound(w, r)
			}))
			defer srv.Close()

			p := NewOCMWStatusListPublisher(srv.URL, "did:example:issuer", "default")
			uri, err := p.PublishStatus(context.Background(), "did:example:contract1", state, "test reason", time.Now())
			require.NoError(t, err, "state %q should not error", state)
			assert.True(t, revokeCalled, "state %q must POST to /revoke/ endpoint", state)
			assert.Contains(t, uri, "/v1/tenants/")
		})
	}
}

// TestOCMWStatusListPublisher_PublishStatus_NonTerminalStatesDoNotRevoke verifies
// that active, draft, and amended states do NOT call the revoke endpoint.
func TestOCMWStatusListPublisher_PublishStatus_NonTerminalStatesDoNotRevoke(t *testing.T) {
	for _, state := range []string{"active", "draft", "amended", "ACTIVE", "DRAFT", "AMENDED"} {
		t.Run(state, func(t *testing.T) {
			revokeCalled := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/revoke/") {
					revokeCalled = true
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			p := NewOCMWStatusListPublisher(srv.URL, "did:example:issuer", "default")
			_, err := p.PublishStatus(context.Background(), "did:example:contract1", state, "", time.Now())
			require.NoError(t, err)
			assert.False(t, revokeCalled, "state %q must NOT call /revoke/ endpoint", state)
		})
	}
}

// TestOCMWStatusListPublisher_RevokeStatus_CallsCorrectPath verifies the
// revoke endpoint path and response parsing.
func TestOCMWStatusListPublisher_RevokeStatus_CallsCorrectPath(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
		_, err := fmt.Fprintf(w, `{"tenantId":"default","listId":1,"index":42,"status":"revoked"}`)
		if err != nil {
			log.Println("could not write response:", err)
		}
	}))
	defer srv.Close()

	p := NewOCMWStatusListPublisher(srv.URL, "did:example:issuer", "default")
	uri, err := p.RevokeStatus(context.Background(), "did:example:contractX")
	require.NoError(t, err)
	assert.Contains(t, capturedPath, "/v1/tenants/default/status/revoke/1/", "revoke path must contain tenant and list ID")
	assert.Contains(t, uri, "/v1/tenants/default/status/1", "returned URI must point to status list endpoint")
}

// TestOCMWStatusListPublisher_RevokeStatus_PropagatesHTTPError verifies that
// a non-2xx response from the status list service propagates as an error
// (consistent with hard-fail policy for required external deps).
func TestOCMWStatusListPublisher_RevokeStatus_PropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewOCMWStatusListPublisher(srv.URL, "did:example:issuer", "default")
	_, err := p.RevokeStatus(context.Background(), "did:example:contract1")
	require.Error(t, err, "HTTP 500 from status list must propagate as error")
	assert.Contains(t, err.Error(), "statuslist-service revoke returned 500")
}

// TestOCMWStatusListPublisher_EmptyServiceURL_SkipsRevoke verifies that
// a publisher with no URL configured silently skips revocation without error.
// This supports offline / dev environments where the status list is optional.
func TestOCMWStatusListPublisher_EmptyServiceURL_SkipsRevoke(t *testing.T) {
	p := NewOCMWStatusListPublisher("", "did:example:issuer", "")
	_, err := p.PublishStatus(context.Background(), "did:example:c1", "terminated", "", time.Now())
	require.NoError(t, err, "empty ServiceURL must silently skip — no error for offline environments")
}

// TestOCMWStatusListPublisher_DefaultTenant verifies that an empty tenantID
// defaults to "default" in the endpoint path.
func TestOCMWStatusListPublisher_DefaultTenant(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, err := fmt.Fprintf(w, `{"status":"revoked"}`)
		if err != nil {
			log.Printf("could not write response: %v", err)
		}
	}))
	defer srv.Close()

	p := NewOCMWStatusListPublisher(srv.URL, "did:example:issuer", "") // empty tenant
	_, err := p.RevokeStatus(context.Background(), "contract-abc")
	require.NoError(t, err)
	assert.Contains(t, capturedPath, "/default/", "empty tenantID must default to 'default'")
}

// TestDeriveIndex_DeterministicAndInBounds verifies that the same contractID
// always produces the same index and that the index is within listSize bounds.
func TestStatusListIndex_DeterministicAndInBounds(t *testing.T) {
	id := "did:example:contract123"
	idx1 := StatusListIndex(id)
	idx2 := StatusListIndex(id)
	assert.Equal(t, idx1, idx2, "StatusListIndex must be deterministic for the same input")
	assert.Less(t, idx1, uint32(listSize), "index must be within [0, listSize)")
}

// TestStatusListIndex_DifferentIDsDifferentIndices is a collision-sanity check.
func TestStatusListIndex_DifferentIDsDifferentIndices(t *testing.T) {
	idx1 := StatusListIndex("did:example:contract-a")
	idx2 := StatusListIndex("did:example:contract-b")
	assert.NotEqual(t, idx1, idx2, "distinct contract IDs should map to distinct indices")
}

// TestStatusListURI_Format verifies the URI returned by statusListURI matches
// the expected XFSC statuslist-service path format.
func TestStatusListURI_Format(t *testing.T) {
	p := NewOCMWStatusListPublisher("http://statuslist:8080", "did:example:issuer", "acme")
	uri := p.statusListURI()
	assert.Equal(t, "http://statuslist:8080/v1/tenants/acme/status/1", uri)
}

// TestQueryStatusListStatus_ActiveBitNotSet verifies "active" is returned when
// the bitstring bit at the contract's index is 0.
func TestQueryStatusListStatus_ActiveBitNotSet(t *testing.T) {
	contractID := "did:example:contract-active"
	idx := StatusListIndex(contractID)

	vcBody := makeStatusListVC(int(listSize/8), idx, false /* not revoked */)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(vcBody)
		if err != nil {
			log.Printf("could not write response: %v", err)
		}
	}))
	defer srv.Close()

	status, err := QueryStatusListStatus(context.Background(), srv.Client(), srv.URL, idx)
	require.NoError(t, err)
	assert.Equal(t, "active", status)
}

// TestQueryStatusListStatus_RevokedBitSet verifies "revoked" is returned when
// the bit at the contract's index is 1.
func TestQueryStatusListStatus_RevokedBitSet(t *testing.T) {
	contractID := "did:example:contract-revoked"
	idx := StatusListIndex(contractID)

	vcBody := makeStatusListVC(int(listSize/8), idx, true /* revoked */)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(vcBody)
		if err != nil {
			log.Printf("could not write response: %v", err)
		}
	}))
	defer srv.Close()

	status, err := QueryStatusListStatus(context.Background(), srv.Client(), srv.URL, idx)
	require.NoError(t, err)
	assert.Equal(t, "revoked", status)
}

// TestQueryStatusListStatus_HTTPErrorPropagates verifies that a non-200 response
// from the status list service is returned as an error (hard-fail policy).
func TestQueryStatusListStatus_HTTPErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := QueryStatusListStatus(context.Background(), srv.Client(), srv.URL, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

// TestQueryStatusListStatus_MissingEncodedList verifies that a VC without
// encodedList in credentialSubject returns an error.
func TestQueryStatusListStatus_MissingEncodedList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`{"credentialSubject":{}}`))
		if err != nil {
			log.Printf("could not write response: %v", err)
		}
	}))
	defer srv.Close()

	_, err := QueryStatusListStatus(context.Background(), srv.Client(), srv.URL, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encodedList absent")
}
