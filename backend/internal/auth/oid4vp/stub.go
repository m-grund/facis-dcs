package oid4vp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"
)

// VerifiedLoginClaims holds subject and roles extracted from a VP (stub or real verifier).
type VerifiedLoginClaims struct {
	SubjectDID     string
	OrganizationID string
	Roles          []string
	RawClaims      json.RawMessage
}

// StubVerifier accepts development presentations when vp_token is "stub".
type StubVerifier struct {
	DefaultSubject string
	DefaultOrg     string
	DefaultRoles   []string
}

func NewStubVerifier() *StubVerifier {
	return &StubVerifier{
		DefaultSubject: "did:jwk:development-user",
		DefaultOrg:     "Acme Corp",
		DefaultRoles: []string{
			string(userrole.TemplateCreator),
			string(userrole.TemplateReviewer),
			string(userrole.TemplateApprover),
			string(userrole.TemplateManager),
			string(userrole.ContractCreator),
			string(userrole.ContractReviewer),
			string(userrole.ContractApprover),
			string(userrole.ContractManager),
			string(userrole.ContractSigner),
			string(userrole.ContractObserver),
			string(userrole.ArchiveManager),
			string(userrole.Auditor),
			string(userrole.SystemAdministrator),
			string(userrole.ComplianceOfficer),
			string(userrole.IntegrationManager),
			string(userrole.ProcessOrchestrator),
			string(userrole.Validator),
		},
	}
}

func (v *StubVerifier) Verify(vpToken string, defaultRoles []string) (*VerifiedLoginClaims, error) {
	token := strings.TrimSpace(vpToken)
	switch token {
	case "stub":
	default:
		if token == "" {
			return nil, fmt.Errorf("vp_token is required (use \"stub\" for development)")
		}
		return nil, fmt.Errorf("unsupported vp_token until Epic 8 verifier is implemented")
	}

	roles := defaultRoles
	if len(roles) == 0 {
		roles = v.DefaultRoles
	}
	if len(roles) == 0 {
		return nil, fmt.Errorf("no roles configured for stub presentation")
	}

	raw, err := json.Marshal(BuildStubPoACredentialClaims(v.DefaultSubject, v.DefaultOrg, roles, time.Now().UTC()))
	if err != nil {
		return nil, err
	}

	return &VerifiedLoginClaims{
		SubjectDID:     v.DefaultSubject,
		OrganizationID: v.DefaultOrg,
		Roles:          roles,
		RawClaims:      raw,
	}, nil
}

func LoadDCQLQuery() (any, error) {
	raw := strings.TrimSpace(os.Getenv("OID4VP_DCQL_QUERY"))
	if raw == "" {
		return DefaultDCQLQuery(), nil
	}
	var q any
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		return nil, fmt.Errorf("invalid OID4VP_DCQL_QUERY JSON: %w", err)
	}
	return q, nil
}
