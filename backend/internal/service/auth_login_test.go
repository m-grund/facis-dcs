package service

import "testing"

func TestExtractSinglePresentation(t *testing.T) {
	vp, err := extractSinglePresentation(`{"dcs_poa_credential":["a~b~c"]}`, "dcs_poa_credential")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vp != "a~b~c" {
		t.Fatalf("unexpected vp: %q", vp)
	}
}

func TestExtractSinglePresentationMissingQueryID(t *testing.T) {
	_, err := extractSinglePresentation(`{"other":["a~b~c"]}`, "dcs_poa_credential")
	if err == nil {
		t.Fatal("expected error for missing query id")
	}
}

func TestExtractSinglePresentationRejectsMultiplePresentations(t *testing.T) {
	_, err := extractSinglePresentation(`{"dcs_poa_credential":["a","b"]}`, "dcs_poa_credential")
	if err == nil {
		t.Fatal("expected error for multiple presentations")
	}
}

func TestExtractSinglePresentationRejectsNonObject(t *testing.T) {
	_, err := extractSinglePresentation(`"a~b~c"`, "dcs_poa_credential")
	if err == nil {
		t.Fatal("expected error for non-object vp_token")
	}
}

func TestCredentialQueryIDFromDCQL(t *testing.T) {
	id, err := credentialQueryIDFromDCQL(map[string]any{
		"credentials": []any{map[string]any{"id": "query-1"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "query-1" {
		t.Fatalf("unexpected query id: %s", id)
	}
}
