package oid4vp

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	_ = ConfigureStatusListVerification("", true)
	os.Exit(m.Run())
}

func TestConfigureStatusListVerification_AllowsXFSCFallbackOutsideProduction(t *testing.T) {
	err := configureStatusListVerification("", true, func(string) string { return "development" })
	require.NoError(t, err)
}

func makeXFSCListBody(bitstring []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(bitstring)
	_ = w.Close()
	body, _ := json.Marshal(map[string]any{
		"tenantId": "default",
		"listId":   1,
		"list":     base64.RawStdEncoding.EncodeToString(buf.Bytes()),
	})
	return body
}

func TestCheckStatusList_IETFStatusList_Active(t *testing.T) {
	bitstring := make([]byte, 125000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Type")), status.XFSCSignedContentType) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, status.IETFStatusListAccept, r.Header.Get("Accept"))
		assert.Empty(t, r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(makeXFSCListBody(bitstring))
	}))
	defer srv.Close()

	claims, err := json.Marshal(map[string]any{
		"status": map[string]any{
			"status_list": map[string]any{
				"uri": srv.URL,
				"idx": 62073,
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, checkStatusList(claims))
}

func TestCheckStatusList_IETFStatusList_Revoked(t *testing.T) {
	idx := uint32(3)
	bitstring := make([]byte, 16)
	bitstring[idx/8] |= 1 << (idx % 8)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Type")), status.XFSCSignedContentType) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, status.IETFStatusListAccept, r.Header.Get("Accept"))
		assert.Empty(t, r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(makeXFSCListBody(bitstring))
	}))
	defer srv.Close()

	claims, err := json.Marshal(map[string]any{
		"status": map[string]any{
			"status_list": map[string]any{
				"uri": srv.URL,
				"idx": idx,
			},
		},
	})
	require.NoError(t, err)

	err = checkStatusList(claims)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revoked")
}
