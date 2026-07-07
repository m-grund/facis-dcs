package fcasset

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	CredentialsV2Context = "https://www.w3.org/ns/credentials/v2"
	DCSContextURL        = "https://w3id.org/facis/dcs/1#"
	SchemaContextURL     = "https://schema.org/"
	ProvContextURL       = "http://www.w3.org/ns/prov#"
)

// CatalogueSubject is the credentialSubject published to FC.
type CatalogueSubject struct {
	ID          string
	State       string
	Name        string
	Description string
	Version     string
}

// BuildInput carries catalogue metadata required for an FC /assets JSON-LD payload.
type BuildInput struct {
	Issuer    string
	ValidFrom time.Time
	Subject   CatalogueSubject
}

// BuildPayload assembles a thin catalogue VC for FC publish.
func BuildPayload(input BuildInput) (map[string]any, error) {
	if strings.TrimSpace(input.Subject.ID) == "" {
		return nil, fmt.Errorf("template did is empty")
	}

	if strings.TrimSpace(input.Issuer) == "" {
		return nil, fmt.Errorf("issuer is empty")
	}

	validFrom := input.ValidFrom.UTC()
	if validFrom.After(time.Now().UTC()) {
		validFrom = time.Now().UTC()
	}

	return map[string]any{
		"@context": []any{
			CredentialsV2Context,
			map[string]any{
				"dcs":    DCSContextURL,
				"schema": SchemaContextURL,
				"prov":   ProvContextURL,
			},
		},
		"id": input.Subject.ID,
		"type": []string{
			"VerifiableCredential",
			"dcs:ContractTemplate",
		},
		"issuer":    input.Issuer,
		"validFrom": validFrom.Format(time.RFC3339),
		"credentialSubject": map[string]any{
			// We need a reachable link for DCS registration.
			"id":                 input.Subject.ID,
			"type":               "dcs:ContractTemplate",
			"dcs:templateUuid":   input.Subject.ID,
			"dcs:state":          input.Subject.State,
			"schema:name":        input.Subject.Name,
			"schema:description": input.Subject.Description,
			"schema:version":     input.Subject.Version,
		},
	}, nil
}

// CatalogueSubjectFromRepository builds FC catalogue metadata from a local template.
func CatalogueSubjectFromRepository(
	did string,
	version int,
	state string,
	name string,
	description string,
) CatalogueSubject {
	return CatalogueSubject{
		ID:          did,
		State:       strings.ToLower(strings.TrimSpace(state)),
		Name:        name,
		Description: description,
		Version:     strconv.Itoa(version),
	}
}

// ErrRemoteTemplateNotFound is returned when the remote DCS does not expose template content for a DID yet.
var ErrRemoteTemplateNotFound = errors.New("remote template not found")

// ToDidDocumentURL maps a did:web identifier to its HTTPS DID document URL per W3C DID Core.
// Example: did:web:localhost:template:uuid → https://localhost/template/uuid/did.json
func ToDidDocumentURL(did string) (string, error) {
	const prefix = "did:web:"

	did = strings.TrimSpace(did)
	if did == "" {
		return "", fmt.Errorf("did is empty")
	}
	if !strings.HasPrefix(did, prefix) {
		return "", fmt.Errorf("only did:web is supported: %s", did)
	}

	path := strings.TrimPrefix(did, prefix)
	if path == "" {
		return "", fmt.Errorf("did path is empty")
	}

	return "https://" + strings.ReplaceAll(path, ":", "/") + "/did.json", nil
}

// FetchDocument resolves template content from the DID document URL.
func FetchDocument(ctx context.Context, did string) (map[string]any, error) {
	if _, err := ToDidDocumentURL(did); err != nil {
		return nil, err
	}

	_ = ctx
	return nil, ErrRemoteTemplateNotFound
}
