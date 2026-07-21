package command

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"
	db "digital-contracting-service/internal/signingmanagement/db"

	"github.com/stretchr/testify/require"
)

// TestSealAgreementStampsPartyFunctions proves the seal tags both signatories
// with their ODRL party function (§4.3.7/4.3.8): the offeror/creator is the
// contractingParty, the accepting counterparty is the contractedParty.
func TestSealAgreementStampsPartyFunctions(t *testing.T) {
	doc := map[string]any{
		"@id":          "urn:contract:1",
		"dcs:policies": map[string]any{"@type": "odrl:Offer"},
		"dcs:parties": []any{
			map[string]any{"@id": "did:web:origin", "@type": "dcs:CompanyParty", "dcs:role": "assigner"},
			map[string]any{"@id": "urn:contract:1#party-assignee", "@type": "dcs:CompanyParty"},
		},
	}
	raw, err := datatype.NewJSON(doc)
	require.NoError(t, err)

	sealed, err := sealAgreementForSigning(raw, &db.Responsible{Creator: "did:web:origin"}, "did:web:signer", "did:web:signer")
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(sealed, &out))

	require.Equal(t, "odrl:Agreement", out["dcs:policies"].(map[string]any)["@type"])

	functions := map[string]string{}
	poa := map[string]string{}
	for _, rawNode := range out["dcs:parties"].([]any) {
		node := rawNode.(map[string]any)
		if fn, ok := node["odrl:function"].(map[string]any); ok {
			functions[node["@id"].(string)] = fn["@id"].(string)
		}
		if p, ok := node["dcs:hasPowerOfAttorney"].(map[string]any); ok {
			poa[node["@id"].(string)] = p["@id"].(string)
		}
	}
	require.Equal(t, "odrl:contractingParty", functions["did:web:origin"], "offeror is the contracting party")
	require.Equal(t, "odrl:contractedParty", functions["did:web:signer"], "counterparty is the contracted party")
	require.Equal(t, "did:web:signer", poa["did:web:signer"], "the signing party carries its Power of Attorney organization")
}
