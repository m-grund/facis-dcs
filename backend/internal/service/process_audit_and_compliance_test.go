package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
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
		{
			name:      "signatures",
			scope:     "signatures",
			scopeName: "signatures",
			component: componenttype.SignatureManagement,
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

func TestBuildAuditReportSummarizesEventsAndFindings(t *testing.T) {
	createdData := json.RawMessage(`{"created_by":"alice"}`)
	findingData := json.RawMessage(`{"ruleId":"FACIS-CONTRACT-POLICY-003","severity":"error","message":"Service availability must satisfy policy minimum.","requirement":"service.sla.availability must be >= 99.9","actualValue":99.5,"expectedValue":99.9,"operator":"gte","path":"service.sla.availability"}`)
	did := "did:example:contract"
	report := buildAuditReport("contracts", "", "auditor", time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC), []*processauditandcompliance.PACAuditResponse{
		{
			Did:       did,
			Component: componenttype.ContractWorkflowEngine.String(),
			AuditTrail: []*processauditandcompliance.PACResourceAuditTrailEntry{
				{ID: 1, Component: componenttype.ContractWorkflowEngine.String(), EventType: "CREATE_CONTRACT", EventData: createdData, Did: &did, CreatedAt: "2026-06-30T10:00:00Z"},
				{ID: 2, Component: componenttype.ContractWorkflowEngine.String(), EventType: "CONTRACT_CONTENT_POLICY_AUDIT_FINDING", EventData: findingData, Did: &did, CreatedAt: "2026-06-30T10:05:00Z"},
			},
		},
	})

	if report.Summary.TotalEvents != 1 || report.Summary.TotalChecks != 1 || report.Summary.Failed != 1 {
		t.Fatalf("unexpected summary: %+v", report.Summary)
	}
	if got := report.Events[0].Actor; got != "alice" {
		t.Fatalf("event actor = %q, want alice", got)
	}
	finding := report.Findings[0]
	if finding.RuleID != "FACIS-CONTRACT-POLICY-003" || finding.Operator != "gte" {
		t.Fatalf("unexpected finding: %+v", finding)
	}
	if finding.ExpectedValue != float64(99.9) || finding.ActualValue != float64(99.5) {
		t.Fatalf("unexpected finding values: actual=%v expected=%v", finding.ActualValue, finding.ExpectedValue)
	}
}

func TestBuildAuditReportFiltersByDID(t *testing.T) {
	keptDID := "did:example:kept"
	skippedDID := "did:example:skipped"
	eventData := json.RawMessage(`{"created_by":"alice"}`)
	responses := []*processauditandcompliance.PACAuditResponse{
		{
			Did:       keptDID,
			Component: componenttype.ContractWorkflowEngine.String(),
			AuditTrail: []*processauditandcompliance.PACResourceAuditTrailEntry{
				{ID: 1, Component: componenttype.ContractWorkflowEngine.String(), EventType: "CREATE_CONTRACT", EventData: eventData, Did: &keptDID, CreatedAt: "2026-06-30T10:00:00Z"},
			},
		},
		{
			Did:       skippedDID,
			Component: componenttype.ContractWorkflowEngine.String(),
			AuditTrail: []*processauditandcompliance.PACResourceAuditTrailEntry{
				{ID: 2, Component: componenttype.ContractWorkflowEngine.String(), EventType: "CREATE_CONTRACT", EventData: eventData, Did: &skippedDID, CreatedAt: "2026-06-30T10:05:00Z"},
			},
		},
	}

	report := buildAuditReport("contracts", keptDID, "auditor", time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC), responses)

	if len(report.Resources) != 1 || report.Resources[0].DID != keptDID {
		t.Fatalf("unexpected resources: %+v", report.Resources)
	}
	if len(report.Events) != 1 || report.Events[0].DID != keptDID {
		t.Fatalf("unexpected events: %+v", report.Events)
	}
}

func TestRenderAuditReportCSVAndPDF(t *testing.T) {
	report := auditReport{
		ReportID:    "pac-report-test",
		Scope:       "contracts",
		GeneratedAt: "2026-06-30T12:00:00Z",
		GeneratedBy: "auditor",
		Summary: auditReportSummary{
			TotalChecks: 1,
			Failed:      1,
		},
		Findings: []auditReportFinding{
			{Timestamp: "2026-06-30T10:05:00Z", DID: "did:example:contract", Component: "CONTRACT_WORKFLOW_ENGINE", EventType: "CONTRACT_CONTENT_POLICY_AUDIT_FINDING", RuleID: "rule,with,comma", Severity: "error", Message: "quoted \"message\"", Requirement: "value must be >= 99.9"},
		},
	}

	csvBytes, err := renderAuditReportCSV(report)
	if err != nil {
		t.Fatalf("render csv: %v", err)
	}
	csvText := string(csvBytes)
	if !strings.Contains(csvText, `"rule,with,comma"`) || !strings.Contains(csvText, `"quoted ""message"""`) {
		t.Fatalf("csv does not contain escaped values: %s", csvText)
	}
	pdfBytes := renderAuditReportPDF(report)
	if !strings.HasPrefix(string(pdfBytes), "%PDF-") {
		t.Fatalf("pdf header missing: %q", string(pdfBytes[:8]))
	}
	if len(pdfBytes) < 100 {
		t.Fatalf("pdf too small: %d", len(pdfBytes))
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
