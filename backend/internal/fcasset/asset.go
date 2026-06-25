package fcasset

import (
	"fmt"
	"time"
)

// CredentialsV2Context is the W3C Verifiable Credentials Data Model 2.0 JSON-LD context.
const CredentialsV2Context = "https://www.w3.org/ns/credentials/v2"

// BuildInput carries data required to build an FC /assets JSON-LD payload.
type BuildInput struct {
	TemplateDID  string
	Issuer       string
	ValidFrom    time.Time
	TemplateData map[string]any
}

// BuildPayload assembles the publish payload: VC shell fields first, then template_data.
func BuildPayload(input BuildInput) (map[string]any, error) {
	if input.TemplateDID == "" {
		return nil, fmt.Errorf("template did is empty")
	}

	if input.Issuer == "" {
		return nil, fmt.Errorf("issuer is empty")
	}

	if input.TemplateData == nil {
		return nil, fmt.Errorf("template data is nil")
	}

	envelope := FillMissingJSONLDFields(input.TemplateDID, input.Issuer, input.ValidFrom)

	return ApplyTemplateData(envelope, input.TemplateData), nil
}

// FillMissingJSONLDFields seeds VC envelope fields before template_data is applied.
func FillMissingJSONLDFields(templateDID, issuer string, validFrom time.Time) map[string]any {
	if validFrom.After(time.Now().UTC()) {
		validFrom = time.Now().UTC()
	}

	return map[string]any{
		"type": []string{
			"VerifiableCredential",
			"dcs:ContractTemplate",
		},
		"issuer":    issuer,
		"validFrom": validFrom.UTC().Format(time.RFC3339),
		"credentialSubject": map[string]any{
			"id":   templateDID,
			"type": "dcs:ContractTemplate",
		},
	}
}

// ApplyTemplateData overlays template_data onto the envelope. template_data values win on key conflicts.
func ApplyTemplateData(envelope map[string]any, templateData map[string]any) map[string]any {
	for key, value := range templateData {
		envelope[key] = value
	}
	envelope["@context"] = prependCredentialsV2Context(envelope["@context"])
	delete(envelope, "@type")

	return envelope
}

func prependCredentialsV2Context(templateContext any) any {
	switch typed := templateContext.(type) {
	case nil:
		return []any{CredentialsV2Context}
	case map[string]any:
		return []any{CredentialsV2Context, typed}
	case []any:
		return prependContextIfMissing(CredentialsV2Context, typed)
	case []string:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return prependContextIfMissing(CredentialsV2Context, items)
	default:
		return []any{CredentialsV2Context, typed}
	}
}

func prependContextIfMissing(contextURL string, contexts []any) []any {
	if len(contexts) == 0 {
		return []any{contextURL}
	}

	if contexts[0] == contextURL {
		return contexts
	}

	return append([]any{contextURL}, contexts...)
}
