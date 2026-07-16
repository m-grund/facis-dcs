package validation

import (
	"reflect"
	"testing"
)

func TestRequiredSignatureFields(t *testing.T) {
	doc := []byte(`{
		"@type": "dcs:Contract",
		"signatureFields": [
			{"@type": "SignatureField", "@id": "urn:doc:x#SignerOne", "signatoryName": "SignerOne"},
			{"@type": "SignatureField", "@id": "urn:doc:x#SignerTwo", "signatoryName": "SignerTwo"},
			{"@type": "SignatureField", "@id": "urn:doc:x#dup", "signatoryName": "SignerOne"},
			{"@type": "SignatureField", "@id": "urn:doc:x#blank", "signatoryName": "  "}
		]
	}`)
	got := RequiredSignatureFields(doc)
	want := []string{"SignerOne", "SignerTwo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	if got := RequiredSignatureFields([]byte(`{"@type":"dcs:Contract"}`)); len(got) != 0 {
		t.Fatalf("expected no fields for a contract without signatureFields, got %v", got)
	}
	if got := RequiredSignatureFields([]byte(`not json`)); got != nil {
		t.Fatalf("expected nil for unparseable data, got %v", got)
	}
}
