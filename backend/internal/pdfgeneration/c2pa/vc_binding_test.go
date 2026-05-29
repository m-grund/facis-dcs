package c2pa

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureSigner struct {
	lastUnsigned json.RawMessage
}

func (s *captureSigner) CreateCredential(_ context.Context, unsignedVC json.RawMessage) (json.RawMessage, error) {
	s.lastUnsigned = append(json.RawMessage{}, unsignedVC...)
	return json.RawMessage(`{"proof":{"type":"Ed25519Signature2020"}}`), nil
}

func TestIssueLifecycleVC_IncludesInlineJSONLDContextAndSubjectID(t *testing.T) {
	effectiveAt := time.Date(2026, 5, 29, 16, 0, 0, 0, time.UTC)
	assertion := NewLifecycleAssertion(
		"did:web:example.org:contracts:abc123",
		"f00dbabe",
		"",
		"",
		"active",
		"approved",
		"did:web:example.org:issuer",
		"",
		"",
		effectiveAt,
	)

	signer := &captureSigner{}
	_, _, err := IssueLifecycleVC(context.Background(), signer, "did:web:example.org:issuer", assertion)
	require.NoError(t, err)
	require.NotEmpty(t, signer.lastUnsigned)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(signer.lastUnsigned, &doc))

	ctxList, ok := doc["@context"].([]interface{})
	require.True(t, ok)
	require.Len(t, ctxList, 3)
	assert.Equal(t, "https://w3id.org/security/suites/ed25519-2020/v1", ctxList[1])

	inlineCtx, ok := ctxList[2].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "https://w3id.org/facis/dcs#", inlineCtx["dcs"])
	assert.Equal(t, "dcs:contractId", inlineCtx["contract_id"])
	assert.Equal(t, "dcs:fileHash", inlineCtx["file_hash"])
	assert.Equal(t, "dcs:status", inlineCtx["status"])
	assert.Equal(t, "dcs:reason", inlineCtx["reason"])

	effectiveAtTerm, ok := inlineCtx["effective_at"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "dcs:effectiveAt", effectiveAtTerm["@id"])
	assert.Equal(t, "http://www.w3.org/2001/XMLSchema#dateTime", effectiveAtTerm["@type"])

	subject, ok := doc["credentialSubject"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, assertion.ContractID, subject["id"])
	assert.Equal(t, assertion.ContractID, subject["contract_id"])
}

func TestIssueLifecycleVC_NormalizesNonURISubjectID(t *testing.T) {
	effectiveAt := time.Date(2026, 5, 29, 16, 0, 0, 0, time.UTC)
	assertion := NewLifecycleAssertion(
		"contract-123", // not an absolute URI
		"f00dbabe",
		"",
		"",
		"active",
		"approved",
		"did:web:example.org:issuer",
		"",
		"",
		effectiveAt,
	)

	signer := &captureSigner{}
	_, _, err := IssueLifecycleVC(context.Background(), signer, "did:web:example.org:issuer", assertion)
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(signer.lastUnsigned, &doc))
	subject, ok := doc["credentialSubject"].(map[string]interface{})
	require.True(t, ok)

	subjectID, ok := subject["id"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(subjectID, "urn:dcs:subject:"))
	assert.Equal(t, assertion.ContractID, subject["contract_id"])
}
