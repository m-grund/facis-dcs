package service

import (
	"testing"

	"digital-contracting-service/internal/base/datatype/componenttype"
)

func TestResolveAuditScopeMapsUIScopes(t *testing.T) {
	tests := []struct {
		name       string
		scope      string
		scopeName  string
		component  componenttype.ComponentType
		template   bool
		contract   bool
		archive    bool
		provenance bool
	}{
		{
			name:       "templates",
			scope:      "templates",
			scopeName:  "templates",
			component:  componenttype.ContractTemplateRepo,
			template:   true,
			provenance: true,
		},
		{
			name:      "contracts",
			scope:     "contracts",
			scopeName: "contracts",
			component: componenttype.ContractWorkflowEngine,
			contract:  true,
		},
		{
			name:      "archive",
			scope:     "archive",
			scopeName: "archive",
			component: componenttype.ContractStorageArchive,
			archive:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAuditScope(tt.scope)
			if err != nil {
				t.Fatalf("resolveAuditScope(%q) returned error: %v", tt.scope, err)
			}
			if got.component != tt.component {
				t.Fatalf("component = %s, want %s", got.component, tt.component)
			}
			if got.scopeName != tt.scopeName {
				t.Fatalf("scopeName = %s, want %s", got.scopeName, tt.scopeName)
			}
			if got.includeTemplatePolicyTrail != tt.template {
				t.Fatalf("includeTemplatePolicyTrail = %t, want %t", got.includeTemplatePolicyTrail, tt.template)
			}
			if got.includeTemplateProvenanceTrail != tt.provenance {
				t.Fatalf("includeTemplateProvenanceTrail = %t, want %t", got.includeTemplateProvenanceTrail, tt.provenance)
			}
			if got.includeContractContentTrail != tt.contract {
				t.Fatalf("includeContractContentTrail = %t, want %t", got.includeContractContentTrail, tt.contract)
			}
			if got.includeArchiveTrail != tt.archive {
				t.Fatalf("includeArchiveTrail = %t, want %t", got.includeArchiveTrail, tt.archive)
			}
		})
	}
}

func TestResolveAuditScopeAcceptsComponentTypes(t *testing.T) {
	tests := []struct {
		name      string
		scope     string
		component componenttype.ComponentType
	}{
		{name: "template component", scope: "CONTRACT_TEMPLATE_REPOSITORY", component: componenttype.ContractTemplateRepo},
		{name: "workflow component lower case", scope: "contract_workflow_engine", component: componenttype.ContractWorkflowEngine},
		{name: "signature component", scope: "SIGNATURE_MANAGEMENT", component: componenttype.SignatureManagement},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAuditScope(tt.scope)
			if err != nil {
				t.Fatalf("resolveAuditScope(%q) returned error: %v", tt.scope, err)
			}
			if got.component != tt.component {
				t.Fatalf("component = %s, want %s", got.component, tt.component)
			}
		})
	}
}

func TestResolveAuditScopeRejectsUnknownScope(t *testing.T) {
	if _, err := resolveAuditScope("unknown"); err == nil {
		t.Fatal("resolveAuditScope returned nil error for unknown scope")
	}
}

func TestValidateAuditScopeDependencies(t *testing.T) {
	service := &processAuditAndCompliancesrvc{}

	if err := service.validateAuditScopeDependencies(templateAuditScopeConfig()); err == nil {
		t.Fatal("validateAuditScopeDependencies returned nil error for missing template scope dependency")
	}
	if err := service.validateAuditScopeDependencies(contractAuditScopeConfig()); err == nil {
		t.Fatal("validateAuditScopeDependencies returned nil error for missing contract scope dependency")
	}
	if err := service.validateAuditScopeDependencies(auditScopeConfig{component: componenttype.SignatureManagement}); err != nil {
		t.Fatalf("validateAuditScopeDependencies returned error for scope without dependency requirement: %v", err)
	}
}
