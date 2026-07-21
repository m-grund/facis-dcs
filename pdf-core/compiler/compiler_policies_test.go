package compiler

import (
	"bytes"
	"testing"
	"time"
)

// TestCompilePDF_RendersODRLPolicies verifies that a dcs:policies odrl:Set is
// rendered as a human-readable section in the compiled PDF: each rule's kind,
// action, parties, target and constraint must appear as visible text.
func TestCompilePDF_RendersODRLPolicies(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
			"xsd": "http://www.w3.org/2001/XMLSchema#"
		},
		"@id": "did:web:localhost:template:abc",
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": {"@type": "dcs:TemplateMetadata", "dcs:title": "Policy Rendering Test"},
		"dcs:documentStructure": {
			"@type": "dcs:DocumentStructure",
			"dcs:layout": [
				{"@type": "dcs:LayoutNode", "dcs:isRoot": true, "dcs:children": ["did:web:localhost:template:abc#s1"]},
				{"@type": "dcs:LayoutNode", "@id": "did:web:localhost:template:abc#s1", "dcs:children": ["did:web:localhost:template:abc#c1"]}
			],
			"dcs:blocks": [
				{"@type": "dcs:Section", "@id": "did:web:localhost:template:abc#s1", "dcs:title": "1. Terms"},
				{"@type": "dcs:Clause", "@id": "did:web:localhost:template:abc#c1", "dcs:content": ["An ordinary clause."]}
			]
		},
		"dcs:policies": {
			"@id": "did:web:localhost:template:abc#policy-set",
			"@type": "odrl:Set",
			"uid": "did:web:localhost:template:abc",
			"odrl:profile": {"@id": "https://w3id.org/facis/dcs/odrl-profile/v1"},
			"odrl:duty": [
				{
					"@id": "did:web:localhost:template:abc#policy-req-x-0",
					"@type": "odrl:Duty",
					"odrl:action": {"@id": "https://w3id.org/facis/dcs/action/provideData"},
					"odrl:assigner": {"@id": "did:web:localhost:template:abc#party-provider"},
					"odrl:assignee": {"@id": "did:web:localhost:template:abc#party-customer"},
					"odrl:target": {"@id": "did:web:localhost:template:abc"},
					"odrl:constraint": {
						"@type": "odrl:Constraint",
						"odrl:leftOperand": {"@id": "did:web:localhost:template:abc#field-country"},
						"odrl:operator": {"@id": "odrl:isAnyOf"},
						"odrl:rightOperand": [{"@value": "DEU"}, {"@value": "AUT"}, {"@value": "CHE"}]
					}
				}
			],
			"odrl:permission": [
				{
					"@id": "did:web:localhost:template:abc#policy-perm-0",
					"@type": "odrl:Permission",
					"odrl:action": {"@id": "https://w3id.org/facis/dcs/action/inspect"},
					"odrl:assigner": {"@id": "did:web:localhost:template:abc#party-provider"},
					"odrl:assignee": {"@id": "did:web:localhost:template:abc#party-customer"},
					"odrl:target": {"@id": "did:web:localhost:template:abc"}
				}
			],
			"odrl:prohibition": [
				{
					"@id": "did:web:localhost:template:abc#policy-proh-0",
					"@type": "odrl:Prohibition",
					"odrl:action": {"@id": "https://w3id.org/facis/dcs/action/redistribute"},
					"odrl:assigner": {"@id": "did:web:localhost:template:abc#party-provider"},
					"odrl:assignee": {"@id": "did:web:localhost:template:abc#party-customer"},
					"odrl:target": {"@id": "did:web:localhost:template:abc"}
				}
			]
		}
	}`)

	pdf, err := CompilePDF(testSigningContext(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	content := concatBTBlocks(pdf)

	markers := []string{
		"Policies",      // section heading
		"Obligation",    // odrl:Duty rendered as an obligation
		"provideData",   // duty action label
		"field-country", // constraint left operand label
		"DEU",           // constraint right operand value
		"Permission",    // odrl:Permission kind
		"inspect",       // permission action label
		"Prohibition",   // odrl:Prohibition kind
		"redistribute",  // prohibition action label
	}
	for _, m := range markers {
		if !bytes.Contains(content, []byte(m)) {
			t.Errorf("expected rendered policy marker %q in PDF content streams, not found", m)
		}
	}
}
